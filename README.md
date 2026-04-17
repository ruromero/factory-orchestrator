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

### GitHub App (recommended)

A GitHub App gives the factory its own identity so CODEOWNERS can distinguish bot commits from human commits. Setup:

1. Create a GitHub App at **Settings > Developer settings > GitHub Apps**
   - Homepage URL: your repo URL
   - Disable Webhook (uncheck "Active")
   - Permissions: **Contents** (Read & write), **Issues** (Read & write), **Pull requests** (Read & write), **Metadata** (Read-only)
2. Generate a private key and download the `.pem` file
3. Install the app on target repos — note the **Installation ID** from the URL (`github.com/settings/installations/<id>`)
4. Set `GITHUB_APP_PRIVATE_KEY_PATH` env var pointing to the `.pem` file
5. Configure each repo in `config.json` with `app_id` and `installation_id`

### PAT (fallback)

If `app_id` is not set, the orchestrator falls back to a static token:

```json
{"owner": "ruromero", "repo": "example", "token": "ghp_..."}
```

## Configuration

The orchestrator supports multiple repos in a single instance. Credentials are loaded from env vars, not the config file:

- `GEMINI_API_KEY` — Gemini API key
- `GITHUB_APP_PRIVATE_KEY_PATH` — path to the GitHub App `.pem` file (applies to all repos without an explicit `private_key_path`)

```json
{
  "ollama_url": "http://ollama.ai.svc.cluster.local:11434",
  "poll_interval": "30s",
  "max_iterations": 3,
  "shadow_mode": true,
  "repos": [
    {"owner": "ruromero", "repo": "factory-orchestrator", "app_id": 123456, "installation_id": 789012},
    {"owner": "ruromero", "repo": "bunko.sh", "app_id": 123456, "installation_id": 789013}
  ]
}
```

## Repo readiness

The factory will skip repos that don't meet minimum requirements:
- `CODEOWNERS` — protects security-critical paths from autonomous modification
- `CLAUDE.md` — minimal context file with non-obvious constraints
- `CONVENTIONS.md` — project conventions, coding standards, and best practices that all agents must follow

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

## k8s deployment

In Kubernetes, credentials are injected via Secrets — never baked into images or ConfigMaps:

```yaml
# Secret with the PEM and Gemini key
kubectl create secret generic factory-creds \
  --from-file=github-app.pem=/path/to/key.pem \
  --from-literal=GEMINI_API_KEY=your-key

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
