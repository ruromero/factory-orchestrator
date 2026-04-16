# factory-orchestrator

Autonomous software development orchestrator. Polls GitHub issues tagged `factory:ready`, drives them through a phased pipeline using local LLMs (Ollama) and Gemini, and opens PRs with the results.

## Pipeline

1. **Research** — Gemini API + Context7 MCP for current library docs
2. **Plan** — deepseek-r1:14b decomposes the issue into an implementation plan
3. **Design** — qwen2.5-coder:14b produces API contracts, data models, file structure
4. **Code** — qwen2.5-coder:14b + Serena MCP (LSP tools) writes the implementation
5. **Review** — phi4:14b (correctness + security + intent) + Qodo (GitHub AI reviewer)
6. **Iterate** — qwen2.5-coder:14b applies review feedback (max N loops)

## Requirements

- k3s cluster with [Ollama](https://ollama.com) deployed (GPU access)
- Models pulled: `deepseek-r1:14b`, `qwen2.5-coder:14b`, `phi4:14b`
- GitHub PAT with repo/issue access
- Gemini API key (free tier)

## Quick start

```bash
# Build
go build -o orchestrator ./cmd/

# Configure
cp config.example.json config.json
# Edit config.json with your tokens and repos

# Run
./orchestrator -config config.json
```

## Configuration

The orchestrator supports multiple repos in a single instance:

```json
{
  "ollama_url": "http://ollama.ai.svc.cluster.local:11434",
  "gemini_api_key": "...",
  "poll_interval": "30s",
  "max_iterations": 3,
  "shadow_mode": true,
  "repos": [
    {"owner": "ruromero", "repo": "factory-orchestrator", "token": "ghp_..."},
    {"owner": "ruromero", "repo": "bunko.sh", "token": "ghp_..."}
  ]
}
```

## Repo readiness

The factory will skip repos that don't meet minimum requirements:
- `CODEOWNERS` — protects security-critical paths from autonomous modification
- `CLAUDE.md` — minimal context file with non-obvious constraints

## GitHub labels

| Label | Meaning |
|-------|---------|
| `factory:ready` | Issue ready for the factory to pick up |
| `factory:in-progress` | Factory is working on this issue |
| `factory:needs-info` | Planner needs more info from human |
| `factory:needs-human` | Factory stuck, requires human intervention |
| `factory:done` | PR opened, ready for human merge |
| `factory:tracking` | Parent issue decomposed into sub-issues |
| `factory:blocked` | Sub-issue waiting on dependency |

## Design

Built following [fullsend](https://github.com/fullsend-ai/fullsend) patterns:
- Security-first: input sanitization, credential isolation, no agent self-modification
- Decomposed review: correctness, security, intent alignment as separate agents
- Shadow mode: all PRs require human approval until graduation
- Zero framework cognition: orchestrator handles mechanics, LLMs handle judgment
- MCP integration: Serena (LSP) and Context7 (docs) as tool providers

## License

Apache License 2.0
