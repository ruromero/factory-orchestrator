# la-fabriquilla

Autonomous software development orchestrator. Polls GitHub issues tagged `fabriquilla:ready`, drives them through a phased pipeline using local LLMs (Ollama) and Gemini, and opens PRs with the results.

## Pipeline

1. **Research** — Gemini API for external context gathering
2. **Plan** — configurable model via any OpenAI-compatible API (Gemini, DeepSeek, MiniMax, etc.) decomposes the issue into an implementation plan
3. **Design** — qwen3:14b produces API contracts, data models, file structure *(not yet wired)*
4. **Code** — qwen3:14b + Serena MCP (LSP tools) writes the implementation *(not yet wired)*
5. **Review** — qwen3:14b (correctness + security + intent) + Qodo (GitHub AI reviewer) *(not yet wired)*
6. **Iterate** — qwen3:14b applies review feedback (max N loops) *(not yet wired)*

See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed data flow and package layout.

## Requirements

- k3s cluster with [Ollama](https://ollama.com) deployed (GPU access)
- Models pulled: `qwen3:14b`
- GitHub App installed on target repos (recommended), or a GitHub PAT
- Gemini API key (free tier)

## Quick start

```bash
# Build
go build -o orchestrator ./cmd/

# Configure
cp config.example.json config.json
# Edit config.json with your app_id, installation_id per repo

# Credentials via env vars (never in config)
export GITHUB_APP_PRIVATE_KEY_PATH=/etc/factory/github-app.pem
export GEMINI_API_KEY=your-key

# Run
./orchestrator -config config.json
```

## Authentication

### Three GitHub Apps (recommended)

The factory uses three separate GitHub Apps with scoped permissions, aligned to trust boundaries:

| App | Used by | Permissions |
|---|---|---|
| **factory-dispatcher** | dispatcher | Issues (Read & write), Contents (Read-only), Metadata (Read-only) |
| **factory-worker** | gatherer, coder | Contents (Read-only), Metadata (Read-only) |
| **factory-committer** | committer | Contents (Read & write), Pull requests (Read & write), Issues (Read & write), Metadata (Read-only) |

Setup for each app:

1. Create a GitHub App at **Settings > Developer settings > GitHub Apps**
   - Homepage URL: your repo URL
   - Disable Webhook (uncheck "Active")
   - Set only the permissions listed above for each app
2. Generate a private key and download the `.pem` file
3. Install the app on target repos — note the **Installation ID** from the URL (`github.com/settings/installations/<id>`)
4. Configure the `apps` map in `config.json` (see Configuration below)
5. Set private key paths via env vars or config

Env vars for private key paths:
- `FACTORY_DISPATCHER_KEY_PATH` — dispatcher app private key
- `FACTORY_WORKER_KEY_PATH` — worker app private key
- `FACTORY_COMMITTER_KEY_PATH` — committer app private key
- `GITHUB_APP_PRIVATE_KEY_PATH` — fallback for any app without an explicit path

### Single GitHub App (simpler alternative)

If you prefer a simpler setup, you can use a single GitHub App with all permissions. Configure it per-repo in `config.json` with `app_id` and `installation_id`. The factory will use this app for all binaries.

### PAT (fallback)

If `app_id` is not set, the orchestrator falls back to a static token:

```json
{"owner": "ruromero", "repo": "example", "token": "ghp_..."}
```

## Configuration

The orchestrator supports multiple repos in a single instance. Credentials are loaded from env vars, not the config file:

- `GEMINI_API_KEY` — Gemini API key
- `PLANNER_API_KEY` — API key for the planner's OpenAI-compatible endpoint
- `GITHUB_APP_PRIVATE_KEY_PATH` — fallback path to a GitHub App `.pem` file
- `FACTORY_DISPATCHER_KEY_PATH` — dispatcher app private key path
- `FACTORY_WORKER_KEY_PATH` — worker app private key path
- `FACTORY_COMMITTER_KEY_PATH` — committer app private key path

```json
{
  "ollama_url": "http://ollama.ai.svc.cluster.local:11434",
  "poll_interval": "30s",
  "max_iterations": 3,
  "shadow_mode": true,
  "planner": {
    "base_url": "https://generativelanguage.googleapis.com/v1beta/openai",
    "model": "gemini-2.5-flash"
  },
  "apps": {
    "dispatcher": {"app_id": 111111, "installation_id": 222222},
    "worker": {"app_id": 333333, "installation_id": 444444},
    "committer": {"app_id": 555555, "installation_id": 666666}
  },
  "repos": [
    {"owner": "ruromero", "repo": "la-fabriquilla"},
    {"owner": "ruromero", "repo": "example-repo"}
  ]
}
```

When `apps` is configured, each binary authenticates with its scoped App identity. The `repos` list no longer needs `app_id`/`installation_id` per repo (those are inherited from the app config). Per-repo auth fields are still supported as a fallback for single-app setups.

## Repo readiness

The factory will skip repos that don't meet minimum requirements:
- `README.md` — project overview, purpose, and setup instructions
- `ARCHITECTURE.md` — module layout, data models, API surface, infrastructure dependencies
- `CONVENTIONS.md` — coding standards, patterns, and best practices that all agents must follow
- `CODEOWNERS` — protects security-critical paths from autonomous modification
- `CLAUDE.md` — minimal context file with non-obvious constraints
- `.serena/` — Serena MCP project config for LSP-powered code navigation

The planner receives `README.md`, `ARCHITECTURE.md`, and `CONVENTIONS.md` as context to produce plans that fit the actual system. These docs can link to subdocuments for deeper detail.

## GitHub labels

| Label | Meaning |
|-------|---------|
| `fabriquilla:ready` | Issue ready for the factory to pick up |
| `fabriquilla:in-progress` | Factory is working on this issue |
| `fabriquilla:needs-info` | Planner needs more info from human |
| `fabriquilla:needs-human` | Factory stuck, requires human intervention |
| `fabriquilla:done` | PR opened, ready for human merge |
| `fabriquilla:tracking` | Parent issue decomposed into sub-issues |
| `fabriquilla:blocked` | Sub-issue waiting on dependency |
| `fabriquilla:requirements` | Repo missing required files (ARCHITECTURE.md, etc.) |

## k8s deployment

In Kubernetes, credentials are injected via Secrets — never baked into images or ConfigMaps:

```yaml
# Secret with the PEM, Gemini key, and planner key
kubectl create secret generic factory-creds \
  --from-file=github-app.pem=/path/to/key.pem \
  --from-literal=GEMINI_API_KEY=your-key \
  --from-literal=PLANNER_API_KEY=your-planner-key

# Mount PEM as a volume, Gemini key as env var
# See Dockerfile for the scratch-based image
```

The config file goes in a ConfigMap. Credentials stay in the Secret, mounted at `/etc/factory/`.

## Design

Built following [fullsend](https://github.com/fullsend-ai/fullsend) patterns:
- Security-first: input sanitization, credential isolation, no agent self-modification
- Decomposed review: correctness, security, intent alignment as separate agents
- Shadow mode: all PRs require human approval until graduation
- Zero framework cognition: orchestrator handles mechanics, LLMs handle judgment
- MCP integration: Serena (LSP) and Context7 (docs) as tool providers

## License

Apache License 2.0
