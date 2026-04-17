# Architecture

## Overview

The factory-orchestrator is a single Go binary that polls GitHub repos for issues tagged `factory:ready`, runs them through a phased LLM pipeline, and posts results back as issue comments or PRs. It runs in a k3s cluster alongside an Ollama instance with GPU access.

## Execution layers

Following [fullsend](https://github.com/fullsend-ai/fullsend) patterns, the system separates concerns into layers:

- **Dispatch** (`cmd/main.go`) — poll loop, repo iteration, label state machine
- **Infrastructure** (`github/`, `ollama/`, `gemini/`) — API clients, authentication, credential management
- **Sandbox** (`sandbox/`) — input sanitization before anything enters agent context
- **Harness** (`harness/`) — assembles phase context from repo docs and prior phase outputs
- **Runtime** (`agents/`) — LLM prompts and response parsing, no business logic

## Package layout

```
cmd/           Entry point, poll loop, config loading, issue processing
  main.go      Signal handling, ticker, pollAllRepos → processIssue
  config.go    JSON config with env var overrides for credentials

github/        GitHub REST API client
  client.go    Issues, labels, comments, file content (TokenSource interface)
  app_auth.go  GitHub App JWT generation + installation token exchange
  readiness.go Repo capability gate (required files check)

ollama/        Ollama inference API
  client.go    Chat + ChatWithTools (tool-calling loop with max iterations)

gemini/        Gemini API (research phase only)
  client.go    Single Generate method, generativelanguage.googleapis.com

mcp/           Model Context Protocol client
  client.go    JSON-RPC over stdio, implements ollama.ToolHandler

agents/        One file per pipeline phase, each with a system prompt + model assignment
  researcher.go   Gemini 2.5 Flash — gathers external context
  planner.go      deepseek-r1:14b — produces plan, requests info, or decomposes
  designer.go     qwen2.5-coder:14b — API contracts, data models
  coder.go        qwen2.5-coder:14b — implementation with MCP tools
  reviewer.go     phi4:14b — three reviews: correctness, security, intent
  iterator.go     qwen2.5-coder:14b — applies review feedback

harness/       Phase context assembly
  context.go   Loads README, ARCHITECTURE, CONVENTIONS from GitHub API
  composite.go CompositeToolHandler — routes tool calls to correct handler by name

sandbox/       Security boundary
  sanitize.go  Unicode normalization (strips zero-width, bidi, tag characters)

traces/        Observability
  trace.go     Structured JSON trace format for agent metrics
```

## Data flow

```
GitHub Issue (factory:ready)
  │
  ├─ SanitizeInput(title, body)
  ├─ LoadRepoContext(README, ARCHITECTURE, CONVENTIONS)
  ├─ Clone repo (shallow) + start Serena MCP (if configured)
  │
  ├─ Phase 0: Gather (qwen2.5-coder + doc tools + Serena LSP) → targeted context
  ├─ Phase 1: Research (Gemini) → external context
  ├─ Phase 2: Plan (deepseek-r1) → plan | needs_info | decompose
  ├─ Phase 3: Design (qwen2.5-coder) → API contracts, file structure
  ├─ Phase 4: Code (qwen2.5-coder + MCP) → implementation
  ├─ Phase 5: Review (phi4) → correctness + security + intent
  └─ Phase 6: Iterate (qwen2.5-coder) → apply feedback, loop to review
```

Each phase receives outputs from all prior phases via `PhaseContext`.

### Context gathering (Phase 0)

The gatherer uses a composite tool set:
- **Doc tools** (`harness/tools.go`) — list/read documents and sections from repo docs via GitHub API
- **Serena MCP** (`mcp/client.go`) — LSP-powered code navigation (find definitions, references, symbol search) via a shallow clone

Both tool sets are registered in a `CompositeToolHandler` that routes calls by tool name. If Serena is not configured or fails to start, the gatherer degrades gracefully to doc-only tools.

## Authentication

Two auth modes, selected per repo in config:

- **GitHub App** (preferred) — `AppAuth` generates RS256 JWTs from the app's private key, exchanges them for installation access tokens via GitHub API, caches tokens with 5-minute pre-expiry refresh
- **PAT** (fallback) — static token wrapped in `staticToken` implementing `TokenSource`

Both implement `TokenSource` so `Client` is auth-agnostic.

## Credential isolation

Credentials never appear in config files, logs, or agent context:

- `GITHUB_APP_PRIVATE_KEY_PATH` env var → PEM file path (k8s Secret volume mount)
- `GEMINI_API_KEY` env var → API key (k8s Secret)
- Config file (ConfigMap) holds only non-secret settings

## Label state machine

Issues move through states via GitHub labels:

```
factory:ready → factory:in-progress → factory:done
                                     → factory:needs-info (awaiting human)
                                     → factory:needs-human (stuck)
                                     → factory:tracking (decomposed into sub-issues)
```

## Key interfaces

- `github.TokenSource` — `Token(ctx) (string, error)` — implemented by `staticToken` and `AppAuth`
- `ollama.ToolHandler` — `Execute(ctx, name, args) (string, error)` — implemented by `mcp.Client`, `ContextToolHandler`, `CompositeToolHandler`

## Infrastructure

- **Runtime**: k3s single-node cluster (Fedora, hostname manolito)
- **GPU**: NVIDIA RTX 3060 12GB VRAM, shared across all inference
- **Ollama**: Separate namespace (`ai`), models pre-pulled
- **Orchestrator**: Own namespace (`factory`), no GPU needed
- **Registry**: quay.io/ruben/factory-orchestrator
- **CI**: GitHub Actions (gofmt, go vet, go test -race, go build)

## Current status

Phases 0-2 (gather, research, plan) are wired end-to-end. Serena MCP is integrated into the gatherer for LSP-powered code navigation. Phases 3-6 (design, code, review, iterate) have agent implementations but are not yet called from the main orchestration loop.
