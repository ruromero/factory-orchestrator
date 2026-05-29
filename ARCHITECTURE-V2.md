# Architecture v2 — Multi-App Sandboxed Factory

## Overview

The factory-orchestrator evolves from a monolithic Go binary into a set of
purpose-built binaries, each with scoped credentials and optional sandbox
isolation. The orchestrator polls GitHub repos for issues tagged
`factory:ready`, runs them through a phased LLM pipeline, and opens PRs with
the results. v2 adds: separate GitHub App identities per trust boundary,
OpenShell sandboxing per phase, a pluggable external review integration, a
self-improvement feedback loop, and comprehensive guardrails.

This document is the authoritative reference for the v2 architecture. Each
section maps to one or more implementation tasks.

---

## 1. Binary Layout

Single Go source repo (`go.mod`), multiple `cmd/` entries. Shared library
packages are unchanged — each binary imports only what it needs. The Go
compiler dead-code-eliminates unused packages per binary.

```
cmd/
  dispatcher/main.go     long-running: poll loop, triage, phase orchestration
  gatherer/main.go       one-shot: context gathering via Ollama + Serena
  researcher/main.go     one-shot: external research via Gemini
  planner/main.go        one-shot: planning via OpenAI-compatible API
  designer/main.go       one-shot: technical design via Ollama
  coder/main.go          one-shot: code generation via Ollama + Serena
  reviewer/main.go       one-shot: factory review + arbiter (DeepSeek API)
  iterator/main.go       one-shot: apply review feedback via Ollama + Serena
  committer/main.go      one-shot: branch, commit, PR creation, merge
  feedback/main.go       one-shot: external review loop + feedback capture
  eval/main.go           golden-set evaluation harness
  dashboard/main.go      web UI: config, monitoring, reports, control
    frontend/            React app (embedded via embed.FS)

config/
  config.go              shared Config struct (extracted from cmd/config.go)

pipeline/
  state.go               State struct + JSON serialization
  store.go               StateStore interface + file-backed implementation
  parse.go               parseCodeOutput, reviewNeedsIteration, formatReviewFeedback
  validate.go            ValidateFiles (path traversal, blocked paths)
  guardrails.go          limit checks (iterations, cost, scope)

review/
  types.go               ReviewFinding, ArbiterResult, ExternalReviewAdapter interface
  adapters/
    qodo.go              QodoAdapter: parse /agentic_review output, trigger reviews
    human.go             HumanAdapter: parse GitHub PR review comments

openshell/
  client.go              Go client for OpenShell gRPC API
  sandbox.go             sandbox lifecycle (create, exec, upload/download, destroy)
  policy.go              load per-phase network policy YAML

eval/
  eval.go                TestCase, Assertion types, runner logic
  report.go              pass/fail aggregation and reporting
```

### Shared packages (unchanged)

```
github/      GitHub REST API client, App auth, readiness checks
ollama/      Ollama inference API + tool-calling loop
gemini/      Gemini API client
openai/      OpenAI-compatible API client
mcp/         MCP client (JSON-RPC over stdio)
harness/     context assembly, tool handling, LSP installation
sandbox/     input sanitization (Unicode normalization)
agents/      pure functions: one per pipeline phase, no shared state
traces/      structured JSON trace logging
```

### Build

```makefile
BINARIES := dispatcher gatherer researcher planner designer coder \
            reviewer iterator committer feedback eval dashboard

build: $(BINARIES)

$(BINARIES):
	CGO_ENABLED=0 go build -o bin/$@ ./cmd/$@/
```

---

## 2. GitHub App Identity and Permissions

Three GitHub Apps, aligned to trust boundaries. Creating a separate App per
binary would be over-engineering — GitHub Apps have per-installation setup
overhead (webhook URLs, private keys, installation grants per repo).

| GitHub App | Binaries | Permissions | Rationale |
|---|---|---|---|
| **factory-dispatcher** | dispatcher | issues:write, contents:read | Read/write issues and labels, read repo files for readiness and context. Cannot create branches or PRs. |
| **factory-worker** | gatherer, coder, iterator | contents:read | Read repo contents and clone for Serena. Cannot write issues, create branches, or open PRs. |
| **factory-committer** | committer, feedback | contents:write, pull_requests:write, issues:write | Create branches, commits, PRs, and relabel issues. The only identity that can push code. |

**researcher**, **planner**, **designer**, **reviewer** need zero GitHub
credentials. All inputs come from pipeline state. All outputs go to pipeline
state.

### Scoped installation tokens

Extend `github/app_auth.go` with:

```go
func (a *AppAuth) TokenWithPermissions(ctx context.Context, perms map[string]string) (string, error)
```

Each binary requests tokens with only its required permissions, even though
the App itself has the superset. If a scoped token leaks from a sandbox, its
blast radius is limited.

### Configuration

```json
{
  "apps": {
    "dispatcher": {
      "app_id": 111,
      "installation_id": 222,
      "private_key_path": "/keys/dispatcher.pem"
    },
    "worker": {
      "app_id": 333,
      "installation_id": 444,
      "private_key_path": "/keys/worker.pem"
    },
    "committer": {
      "app_id": 555,
      "installation_id": 666,
      "private_key_path": "/keys/committer.pem"
    }
  }
}
```

---

## 3. Pipeline State Management

### State struct

All inter-phase data in a single JSON-serializable struct:

```go
package pipeline

type State struct {
    // Identity
    RepoOwner   string `json:"repo_owner"`
    RepoName    string `json:"repo_name"`
    IssueNumber int    `json:"issue_number"`

    // Phase tracking
    Phase     string `json:"phase"`
    Iteration int    `json:"iteration"`

    // Inputs (set by dispatcher)
    IssueTitle     string `json:"issue_title"`
    IssueBody      string `json:"issue_body"`
    CommentHistory string `json:"comment_history,omitempty"`
    Summaries      string `json:"summaries"`
    Conventions    string `json:"conventions"`

    // Phase outputs (set by each phase binary)
    GatheredContext string        `json:"gathered_context,omitempty"`
    ResearchContext string        `json:"research_context,omitempty"`
    PlanOutcome     string        `json:"plan_outcome,omitempty"`
    PlanContent     string        `json:"plan_content,omitempty"`
    Design          string        `json:"design,omitempty"`
    Code            string        `json:"code,omitempty"`
    Review          *ReviewState  `json:"review,omitempty"`
    ArbiterResult   *ArbiterState `json:"arbiter_result,omitempty"`
    Files           []FileState   `json:"files,omitempty"`

    // PR tracking
    PRNumber int    `json:"pr_number,omitempty"`
    PRBranch string `json:"pr_branch,omitempty"`

    // Timestamps
    StartedAt time.Time `json:"started_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

### Storage

File-backed: `/data/pipeline/{owner}/{repo}/{issue_number}.json`

```go
type StateStore interface {
    Save(ctx context.Context, key string, state *State) error
    Load(ctx context.Context, key string) (*State, error)
}
```

Each phase binary follows the same pattern:

```go
func main() {
    statePath := os.Getenv("PIPELINE_STATE_PATH")
    state, _ := pipeline.LoadState(statePath)
    // ... do phase work ...
    state.Phase = "gather-done"
    state.UpdatedAt = time.Now()
    pipeline.SaveState(statePath, state)
}
```

### File validation

Before committing, the committer validates all file paths:

```go
func ValidateFiles(files []FileState, blockedPatterns []string) error
```

Rejects:
- Path traversal (`../`, absolute paths)
- Blocked patterns (see Guardrails section)
- Empty paths or content

---

## 4. OpenShell Sandbox Integration

### What runs inside vs outside sandboxes

| Component | Sandboxed | Network policy | Why |
|---|---|---|---|
| dispatcher | No | N/A | Trusted deterministic code, needs full GitHub API access |
| gatherer | Yes | Ollama (localhost:11434) | Runs LLM with tool calls against cloned repo |
| researcher | Yes | generativelanguage.googleapis.com | Calls external Gemini API |
| planner | Yes | configured planner endpoint | Calls external API (Gemini, DeepSeek) |
| designer | Yes | Ollama (localhost:11434) | Calls local LLM |
| coder | Yes | Ollama (localhost:11434) | Runs LLM with read+write Serena tools |
| reviewer | Yes | DeepSeek API endpoint | High-judgment arbitration via external API |
| iterator | Yes | Ollama (localhost:11434) | Applies fixes with LLM + Serena tools |
| committer | No | N/A | Trusted deterministic code, needs GitHub write access |
| feedback | No | N/A | Trusted deterministic code, needs GitHub API access |

The dispatcher and committer are deterministic Go code with no LLM
interaction. Sandboxing them adds latency for zero security benefit. The
threat model targets LLM-driven phases.

### Go client for OpenShell

New `openshell/` package wrapping the gRPC API:

```go
type Client struct { /* gRPC connection to OpenShell gateway */ }

func (c *Client) CreateSandbox(ctx context.Context, spec SandboxSpec) error
func (c *Client) WaitReady(ctx context.Context, name string, timeout time.Duration) error
func (c *Client) Exec(ctx context.Context, name string, cmd []string, timeout time.Duration) (ExecResult, error)
func (c *Client) Upload(ctx context.Context, name, localPath, remotePath string) error
func (c *Client) Download(ctx context.Context, name, remotePath, localPath string) error
func (c *Client) Delete(ctx context.Context, name string) error
```

### Sandbox lifecycle per phase

```
dispatcher creates sandbox "factory-{phase}-{issue}"
  → uploads state.json into sandbox
  → exec /usr/local/bin/{phase}
  → downloads updated state.json
  → destroys sandbox
```

### Network policy files

```
deploy/sandbox-policies/
  gatherer.yaml
  researcher.yaml
  planner.yaml
  designer.yaml
  coder.yaml
  reviewer.yaml
  iterator.yaml
```

Example (coder):

```yaml
network:
  default: deny
  allow:
    - endpoint: "localhost:11434"
      methods: [POST]
      paths: ["/api/chat"]
```

The coder sandbox has NO access to `api.github.com`. Even if the LLM is
manipulated via prompt injection, it physically cannot push code.

### GPU considerations

Ollama runs as a system service on the host, not inside sandboxes. Sandboxes
call Ollama over HTTP — no GPU passthrough needed. Sandbox specs use
`gpu: false`.

---

## 5. Sandbox Images

Language-specific images extend a common base. The dispatcher selects the
right image based on the target repo's language (configured per-repo in
`config.json`).

```
deploy/sandbox-images/
  base/
    Dockerfile          git, gh CLI, python3, Serena, all factory phase binaries
  go/
    Dockerfile          extends base: Go toolchain + gopls
  rust/
    Dockerfile          extends base: Rust toolchain + rust-analyzer
  typescript/
    Dockerfile          extends base: Node + typescript-language-server
```

### Base image

```dockerfile
FROM ubuntu:24.04
RUN apt-get update && apt-get install -y \
    git gh curl python3 python3-pip jq \
    && rm -rf /var/lib/apt/lists/*
RUN pip3 install serena
COPY bin/* /usr/local/bin/
```

### Language images

```dockerfile
# go/Dockerfile
FROM factory-base:latest
RUN curl -LO https://go.dev/dl/go1.24.linux-amd64.tar.gz \
    && tar -C /usr/local -xzf go*.tar.gz && rm go*.tar.gz
ENV PATH="/usr/local/go/bin:${PATH}"
RUN go install golang.org/x/tools/gopls@latest

# rust/Dockerfile
FROM factory-base:latest
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
ENV PATH="/root/.cargo/bin:${PATH}"
RUN rustup component add rust-analyzer

# typescript/Dockerfile
FROM factory-base:latest
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y nodejs
RUN npm install -g typescript-language-server typescript
```

### Per-repo configuration

```json
{
  "repos": [
    {
      "owner": "ruromero",
      "repo": "factory-orchestrator",
      "language": "go",
      "sandbox_image": "factory-go:latest"
    }
  ]
}
```

---

## 6. Review Architecture

### Review sources

Three independent review sources feed into a central arbiter:

```
Factory reviewer (sandbox, DeepSeek API)
  Independent review with full project context
  Correctness + security + intent alignment
  Produces structured ReviewFinding[]

External reviewer (pluggable — Qodo today)
  Parsed through ExternalReviewAdapter interface
  Produces structured ReviewFinding[]

Human reviewer (via GitHub PR reviews)
  Parsed through same adapter interface
  Produces structured ReviewFinding[]

        ↓ all findings ↓

Arbiter (part of reviewer binary, DeepSeek API)
  Consumes all findings + project context
  Classifies each → fix_here | subtask | root_cause | dismissed
  Produces ArbiterResult
```

### Pluggable external reviewer

```go
package review

type ReviewFinding struct {
    Category    string // missing_tests, error_handling, security, performance, style
    Severity    string // critical, medium, low
    File        string
    Line        int
    Description string
    Source      string // "qodo", "factory_reviewer", "human"
}

type ArbiterResult struct {
    FixHere    []ReviewFinding   `json:"fix_here"`
    Subtasks   []ReviewFinding   `json:"subtasks"`
    RootCauses []RootCause       `json:"root_causes"`
    Dismissed  []DismissedFinding `json:"dismissed"`
}

type RootCause struct {
    Description       string `json:"description"`
    Source             string `json:"source"`
    FactoryAssessment string `json:"factory_assessment"`
    ProposedIssueTitle string `json:"proposed_issue_title"`
}

type DismissedFinding struct {
    Description string `json:"description"`
    Source      string `json:"source"`
    Reason     string `json:"reason"`
}

type ExternalReviewAdapter interface {
    ParseFindings(ctx context.Context, comments []github.Comment) ([]ReviewFinding, error)
    TriggerReview(ctx context.Context, gh *github.Client, prNumber int) error
    ReviewReady(ctx context.Context, comments []github.Comment) bool
}
```

### QodoAdapter

```go
package adapters

type QodoAdapter struct{}

func (q *QodoAdapter) TriggerReview(ctx context.Context, gh *github.Client, prNumber int) error {
    return gh.CreateComment(ctx, prNumber, "/agentic_review")
}

func (q *QodoAdapter) ReviewReady(ctx context.Context, comments []github.Comment) bool {
    // Look for comment from Qodo bot posted after trigger
}

func (q *QodoAdapter) ParseFindings(ctx context.Context, comments []github.Comment) ([]ReviewFinding, error) {
    // Parse Qodo's structured review output into []ReviewFinding
}
```

### HumanAdapter

```go
type HumanAdapter struct{}

func (h *HumanAdapter) ParseFindings(ctx context.Context, comments []github.Comment) ([]ReviewFinding, error) {
    // Parse GitHub PR review comments (CHANGES_REQUESTED reviews)
    // into []ReviewFinding
}
```

### Arbiter behavior

The arbiter (running as part of the reviewer binary via DeepSeek API):

1. Receives all `[]ReviewFinding` from factory reviewer, external reviewer,
   and human
2. For each finding, decides:
   - **fix_here**: simple fix, iterator can handle in this PR
   - **subtask**: needs planning but belongs in this PR
   - **root_cause**: systemic issue requiring its own issue lifecycle
   - **dismissed**: invalid given project context (with stated reason)
3. If a finding was dismissed in iteration N and the same finding reappears
   in iteration N+1: auto-dismiss (prevent deadlock)
4. Logs all decisions to the feedback system

---

## 7. Complete Pipeline Flow

```
1. Human creates GitHub issue, adds label "factory:ready"

2. Dispatcher polls GitHub, finds issue
   ├── Checks repo readiness (required files)
   ├── Swaps label: factory:ready → factory:in-progress
   ├── Loads repo context (README, ARCHITECTURE, CONVENTIONS)
   ├── Sanitizes issue title/body
   ├── Loads human comment history
   ├── Initializes pipeline State, saves to disk
   │
   ├── Phase: Gather (sandbox, parallel with research)
   │     Network: Ollama only
   │     Input: issue title/body + repo summaries
   │     Output: gathered context → State
   │
   ├── Phase: Research (sandbox, parallel with gather)
   │     Network: Gemini API only
   │     Input: issue title/body + repo summaries
   │     Output: research context → State
   │
   ├── Phase: Plan (sandbox)
   │     Network: planner API only
   │     Input: gathered + research context + conventions + comments
   │     Output: plan outcome + content → State
   │     │
   │     ├── needs_info → comment on issue, label factory:needs-info, stop
   │     ├── decompose → create sub-issues, label factory:tracking, stop
   │     └── plan → continue
   │
   ├── Phase: Design (sandbox)
   │     Network: Ollama only
   │     Input: plan + research context + conventions
   │     Output: technical design → State
   │
   ├── Phase: Code (sandbox)
   │     Network: Ollama only
   │     Input: design + research context + conventions
   │     Tools: Serena read+write
   │     Output: code + parsed files → State
   │
   ├── Committer creates PR (not sandboxed)
   │     Creates branch via Git Data API
   │     Commits files from State
   │     Opens PR with plan + review in body
   │
   ├── Feedback loop (not sandboxed)
   │     │
   │     ├── External reviewer (Qodo) auto-reviews on PR creation
   │     │
   │     ├── Factory reviewer (sandbox, DeepSeek)
   │     │     Independent review with full project context
   │     │
   │     ├── Arbiter (sandbox, DeepSeek)
   │     │     Synthesizes factory + external + human findings
   │     │     Classifies: fix_here / subtask / root_cause / dismissed
   │     │
   │     ├── If fix_here or subtask items:
   │     │     Iterator sandbox applies fixes
   │     │     Committer pushes new commits
   │     │     Feedback binary triggers /agentic_review
   │     │     Loop (max_iterations per cycle, max_total_iterations overall)
   │     │
   │     ├── If root_cause items:
   │     │     Dispatcher creates new issues with factory:ready
   │     │     (max 3 per PR, depth 1 — root cause issues cannot spawn
   │     │      further root cause issues)
   │     │
   │     └── If clean or max iterations: exit loop
   │
   ├── PR ready for human
   │     │
   │     ├── Human approves:
   │     │     Committer checks all status checks pass
   │     │     Committer checks no unresolved review threads
   │     │     Committer merges PR
   │     │     Label factory:done
   │     │
   │     └── Human requests changes:
   │           Parse review comments via HumanAdapter
   │           Re-enter arbiter → iterate loop
   │
   └── Post-merge monitoring
         Watch CI on main for 30 minutes after merge
         If CI breaks:
           Create revert PR automatically
           Create new issue with failure context
           Label factory:reverted on original issue
```

---

## 8. Guardrails

All guardrails are enforced in deterministic code (dispatcher, committer,
feedback binary). No guardrail depends on LLM judgment.

### Iteration limits

| Guardrail | Default | Enforcement point |
|---|---|---|
| max_iterations | 3 | feedback binary: per review cycle |
| max_total_iterations | 5 | feedback binary: across all cycles for one issue |
| max_wall_time | 30m | dispatcher: total processing time per issue |

When hit: stop iterating, label `factory:needs-human`, post comment with
what converged and what didn't.

### Scope limits

| Guardrail | Default | Enforcement point |
|---|---|---|
| max_subtasks_per_pr | 3 | reviewer: if arbiter proposes more, escalate |
| max_files_changed | 20 | committer: refuse to commit if exceeded |
| max_pr_size_lines | 500 | committer: refuse if diff exceeds this |

When hit: stop, label `factory:needs-human`, comment "PR scope has grown
beyond safe limits."

### Root cause limits

| Guardrail | Default | Enforcement point |
|---|---|---|
| max_root_cause_issues_per_pr | 3 | feedback binary |
| max_root_cause_depth | 1 | dispatcher: root cause issues cannot create root cause issues |

Root cause issues run through the full pipeline but their review phase can
only produce `fix_here` and `subtask` classifications. This prevents
recursive issue spawning.

### Cost governance

| Guardrail | Default | Enforcement point |
|---|---|---|
| max_api_tokens_per_issue | configurable | dispatcher: cumulative across all phases |
| max_issues_per_hour | 5 | dispatcher: rate limit on processing |
| max_issues_per_day | 20 | dispatcher: daily cap |

### Self-modification protection

The committer refuses to commit changes to these paths:

```
.github/workflows/*     CI/CD pipelines
CODEOWNERS               permission boundaries
.pr_agent.toml           review tool configuration
CONVENTIONS.md           agent instructions
ARCHITECTURE.md          system design docs
ARCHITECTURE-V2.md       system design docs
CLAUDE.md                agent context
.serena/*                MCP configuration
deploy/*                 k8s manifests and sandbox configs
```

Enforced by `pipeline.ValidateFiles()` in the committer binary. If any file
matches a blocked pattern, the committer labels `factory:needs-human` with a
comment explaining which files were blocked and why.

### Review deadlock prevention

If a finding is dismissed by the arbiter in iteration N and the **same
finding** (matched by file + description similarity) reappears in iteration
N+1: auto-dismiss without re-arbitration. Log as "persistent disagreement"
in the feedback system. The human sees both perspectives in PR comments.

---

## 9. needs_info Cases

Any phase can signal that it needs human input. The dispatcher posts a
structured comment and labels the issue `factory:needs-info`.

### Comment format

```markdown
## Factory: Additional Information Needed

**Phase:** {phase_name}
**Issue:** #{issue_number} — {issue_title}

### Questions

1. {specific question}
2. {specific question}

### What the factory already knows

- {relevant context gathered so far}

### Suggested options (if applicable)

a) {option with trade-off description}
b) {option with trade-off description}
c) Something else — please specify

<!-- factory:needs-info:{phase}:{question_count} -->
```

The HTML comment at the bottom enables the dispatcher to parse which phase
asked and track response patterns.

### Cases by phase

**Planner:**
- Ambiguous requirements (multiple valid interpretations)
- Missing reproduction steps for bug reports
- Conflicting constraints (issue vs. ARCHITECTURE.md)
- References to external systems without sufficient context

**Gatherer:**
- Referenced code doesn't exist in the codebase
- Multiple candidates for an ambiguous reference (e.g., "the User model"
  when there are three)
- Open design decision discovered in code (`// TODO: decide on approach`)

**Designer:**
- Design choice requires human decision (REST vs. gRPC when project uses
  both)
- Impact on public API contract (conventions require sign-off)
- Two valid approaches with different trade-offs and no clear winner

**Reviewer/Arbiter:**
- Security concern that can't be confidently dismissed or confirmed
- Implementation matches the plan but the plan may have misinterpreted
  intent
- Qodo and factory reviewer have irreconcilable disagreement on a finding

**Committer:**
- PR touches files protected by CODEOWNERS that require specific reviewers

**Guardrail triggers:**
- Max iterations reached with unresolved critical findings
- PR scope exceeded limits
- Root cause issue limit reached with additional root causes identified

### Guideline for agents

> Return `needs_info` only when proceeding would likely produce **wrong**
> output, not just imperfect output. If a reasonable assumption can be made,
> state the assumption and proceed. The review phase catches bad assumptions.

This prevents `needs_info` from becoming a bottleneck where agents ask for
clarification on every ambiguity.

---

## 10. Self-Improvement Feedback Loop

### Structured feedback capture

The feedback binary logs every review cycle as append-only JSONL:

```
feedback/{owner}/{repo}/log.jsonl
```

Each entry:

```json
{
  "issue": 42,
  "pr": 43,
  "timestamp": "2026-05-30T14:00:00Z",
  "qodo_effort": 3,
  "findings": [
    {
      "category": "missing_tests",
      "severity": "medium",
      "source": "qodo",
      "file": "config/config.go",
      "fixed_by_iterator": true,
      "iterations_to_fix": 1
    },
    {
      "category": "error_handling",
      "severity": "high",
      "source": "factory_reviewer",
      "file": "github/client.go",
      "fixed_by_iterator": false,
      "escalated_to_human": true
    }
  ],
  "dismissed_findings": [
    {
      "description": "Function too complex",
      "source": "qodo",
      "reason": "Matches pattern in harness/composite.go"
    }
  ],
  "root_causes_created": [
    {
      "issue_number": 44,
      "title": "Establish config validation pattern",
      "category": "missing_validation"
    }
  ],
  "human_override": false,
  "human_edits": []
}
```

### Periodic analysis

A manual or cron-triggered analysis reads the feedback log and produces
actionable recommendations:

```
Top recurring findings (last 30 days):
  1. missing_tests — 12 occurrences, 8 fixed by iterator, 4 escalated
     → ACTION: update coder prompt to require test per new function
  2. error_handling — 9 occurrences, 2 fixed by iterator, 7 escalated
     → ACTION: add errcheck linter rule to CI (deterministic > LLM)
  3. unused_imports — 6 occurrences, 6 fixed by iterator
     → ACTION: add goimports to pre-commit
```

### Improvement categories

- **Recurring finding** → update agent prompt or add linter/lint rule
  (prefer deterministic over LLM)
- **Iterator can't fix** → escalate immediately in future (skip iteration)
- **Human overrides** → capture as golden-set test cases
- **Human edits to factory code** → the before/after becomes a coder test
- **Dismissed findings that humans later agree with** → arbiter prompt needs
  adjustment

### Graduation signal

The rate of root cause issues per PR should decrease over time. A repo where
findings trend toward zero is a candidate for reduced human oversight. A
repo where the same categories recur is not ready for autonomy graduation.

---

## 11. Golden-Set Evaluation

### Test case structure

```
tests/golden/
  planner/
    case-001-simple-bug.json       happy path → outcome "plan"
    case-002-missing-info.json     ambiguous issue → outcome "needs_info"
    case-003-complex-task.json     large scope → outcome "decompose"
  coder/
    case-001-add-function.json     add new function with tests
    case-002-modify-existing.json  modify existing code
    case-003-injection-trap.json   prompt injection in issue body
  reviewer/
    case-001-clean-code.json       correct code → no critical findings
    case-002-planted-bug.json      code with intentional bug → finds it
    case-003-scope-creep.json      code that exceeds issue scope → flags it
  arbiter/
    case-001-dismiss-qodo.json     Qodo finding invalid for project context
    case-002-root-cause.json       finding is systemic, should create issue
    case-003-subtask.json          finding needs work within PR
```

### Test case format

```json
{
  "name": "planner-recognizes-missing-info",
  "phase": "planner",
  "inputs": {
    "issue_title": "Fix the bug",
    "issue_body": "It's broken, please fix",
    "research_context": "",
    "gathered_context": "",
    "conventions": "..."
  },
  "assertions": [
    {"type": "outcome_equals", "value": "needs_info"},
    {"type": "output_contains", "value": "reproduce"},
    {"type": "output_not_contains", "value": "PLAN:"}
  ],
  "pass_threshold": "8/10"
}
```

### Assertion types

| Type | Description |
|---|---|
| outcome_equals | PlanResult.Outcome matches exactly |
| output_contains | Output text contains substring |
| output_not_contains | Output text must not contain substring |
| file_count_gte | Number of parsed files ≥ N |
| file_paths_include | Specific file path appears in output |
| severity_present | ReviewResult contains a specific severity tag |
| compiles | Output code compiles (run `go build` in sandbox) |
| tests_pass | Output code passes tests (run `go test` in sandbox) |

### Evaluation harness

`cmd/eval/main.go`:

1. Load golden-set directory
2. For each test case, run the agent function N times (default 10)
3. Check all assertions against each run
4. Report pass rate per case and overall
5. Exit non-zero if any case falls below its `pass_threshold`

### When to run

- On every agent prompt change (system prompt in `agents/*.go`)
- Weekly cron for drift detection (model updates can change behavior)
- Before switching to a new model version
- After adding new test cases from human overrides

---

## 12. Model Assignments

| Phase | Model | Provider | Reasoning |
|---|---|---|---|
| Gatherer | qwen3:14b | Ollama (local) | Tool calling, read-only code exploration |
| Researcher | Gemini 2.5 Flash | Google API (free tier) | Broad external research |
| Planner | Configurable | OpenAI-compatible API | Supports Gemini, DeepSeek, etc. |
| Designer | qwen3:14b | Ollama (local) | Structured technical output |
| Coder | qwen3:14b | Ollama (local) | Tool calling, code generation |
| Reviewer + Arbiter | DeepSeek | DeepSeek API | High-judgment: synthesize findings, dismiss/escalate |
| Iterator | qwen3:14b | Ollama (local) | Apply fixes with Serena tools |

The reviewer/arbiter uses DeepSeek because arbitration — deciding whether to
dismiss a Qodo finding or escalate a root cause — is the highest-judgment
task in the pipeline. Local 14B models may lack the nuance for reliable
"dismiss vs. escalate" decisions. Token volume for the arbiter is low (it
reads findings and project context, not full codebases), so API cost is
minimal.

---

## 13. Deployment

### Minimum requirements

| Resource | Minimum | Recommended | Notes |
|---|---|---|---|
| RAM | 24 GB | 32+ GB | Ollama (~12 GB) + gateway (~512 MB) + sandbox (~4 GB) + OS |
| GPU VRAM | 10 GB | 12+ GB | For qwen3:14b via Ollama. Larger models need more. |
| CPU | 4 cores | 8+ cores | Ollama uses 2 cores during inference, sandbox uses up to 2 |
| Disk | 50 GB | 100+ GB | Models (~10 GB), sandbox images (~5 GB each), repo clones |
| OS | Linux (x86_64) | Fedora, Ubuntu, Debian | OpenShell requires Linux kernel (Landlock, seccomp). macOS works via Docker Desktop but with weaker isolation. |

Docker Engine >= 28.04 or Podman >= 5.x required for OpenShell. NVIDIA
Container Toolkit required if running inference inside sandboxes (not
needed when Ollama runs on the host).

### Infrastructure

```
Host machine
│
├── Ollama (systemd service, always running)
│     Local model loaded in VRAM
│     Listening on localhost:11434
│
├── OpenShell gateway (Docker container, always running)
│     Manages sandbox lifecycle, ~512MB RAM
│     SQLite backend (single-node)
│
├── factory-dispatcher (systemd service or k3s Deployment)
│     Long-running poll loop
│     Mounts /data/pipeline for state files
│     Contains all phase binaries in same image
│
├── factory-dashboard (systemd service or k3s Deployment)
│     Web UI on port 8080
│     Reads /data/pipeline and /data/feedback
│     Single bearer token auth
│
└── OpenShell sandboxes (ephemeral containers)
      Created per-phase, destroyed after completion
      One active sandbox at a time (sequential pipeline)
      Docker driver (containers on same node)
```

### Resource budget

| Component | CPU | Memory | GPU |
|---|---|---|---|
| Ollama + local model | 2 cores | 10-12 GB | 10-12 GB VRAM |
| OpenShell gateway | 0.5 core | 512 MB | none |
| dispatcher | 0.1 core | 128 MB | none |
| dashboard | 0.1 core | 128 MB | none |
| active sandbox (peak) | 2 cores | 4 GB | none |

Phases run sequentially, so sandbox resources are not additive. Peak memory
usage is approximately 18 GB (Ollama + gateway + one active sandbox).

### Container image

One fat image containing all binaries:

```dockerfile
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
RUN for bin in dispatcher gatherer researcher planner designer coder \
    reviewer iterator committer feedback eval dashboard; do \
      CGO_ENABLED=0 go build -o /out/$bin ./cmd/$bin/; \
    done

FROM ubuntu:24.04
RUN apt-get update && apt-get install -y git python3 python3-pip \
    && rm -rf /var/lib/apt/lists/*
RUN pip3 install serena
COPY --from=build /out/* /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/dispatcher"]
```

The dispatcher image runs the dispatcher as entrypoint. When invoking phase
binaries as subprocesses (v1), they are in the same image. When invoking via
OpenShell (v2), this image is the sandbox image with entrypoint overridden.

---

## 14. Dashboard

A lightweight web UI for configuring, monitoring, and analyzing the factory.
Single-user, home lab — basic auth with a token from an env var
(`DASHBOARD_TOKEN`).

### Binary

Another entry in the monorepo: `cmd/dashboard/main.go`. A Go HTTP server
with the React frontend embedded via `embed.FS` — single binary, no
separate frontend deployment.

```
cmd/
  dashboard/
    main.go                Go HTTP server + API routes
    frontend/              React app (embedded at build time)
      src/
        pages/
          Pipeline.tsx     live pipeline status per issue
          Config.tsx       repos, credentials, models, guardrails
          Reports.tsx      feedback analysis, metrics, trends
          Evals.tsx        golden-set evaluation results
        components/
          IssueQueue.tsx   factory:ready / in-progress / done
          PhaseTimeline.tsx phase progress with durations
          FindingsChart.tsx recurring finding categories over time
          CostTracker.tsx  API token usage per issue/phase/model
```

### Capabilities

**Configure:**
- Repos: add/remove, set language/sandbox image, autonomy level (shadow/auto)
- Credentials: set/rotate GitHub App keys, Gemini/DeepSeek API keys
  (write-only — never displayed after saving)
- Models: assign models to phases, configure API endpoints
- Guardrails: tune limits per repo (max_iterations, max_pr_size, etc.)
- External reviewer: select adapter, configure trigger command

**Observe (live):**
- Pipeline status: which issue is being processed, current phase, duration
- Sandbox status: active sandboxes, resource usage
- Issue queue: `factory:ready` → `factory:in-progress` → `factory:done`
- Current iterate loop: cycle number, remaining findings
- Structured logs: filterable by repo, phase, severity

**Report (historical):**
- Feedback trends: recurring finding categories over time, by repo
- Per-repo metrics: issues processed, PRs merged, human override rate,
  average time-to-PR, cost per issue
- Cost breakdown: API tokens per phase, per model, per repo
- Golden-set eval results: pass rates per agent role, drift over time
- Autonomy readiness: root cause rate trend, graduation candidates

**Control:**
- Shadow mode toggle per repo
- Pause/resume processing per repo
- Retry a stuck issue (relabel to `factory:ready`)
- Force-escalate to human (label `factory:needs-human`)

### API routes

```
GET    /api/pipeline/active            current pipeline runs
GET    /api/pipeline/{owner}/{repo}/{issue}  status of specific issue
GET    /api/repos                       configured repos + status
PUT    /api/repos/{owner}/{repo}        update repo config
PUT    /api/credentials/{name}          set/rotate a credential (write-only)
GET    /api/feedback/{owner}/{repo}     feedback log analysis
GET    /api/metrics/{owner}/{repo}      aggregated metrics
GET    /api/evals/latest                latest golden-set results
POST   /api/control/pause/{owner}/{repo}    pause processing
POST   /api/control/resume/{owner}/{repo}   resume processing
POST   /api/control/retry/{owner}/{repo}/{issue}  retry stuck issue
```

### Data sources

No new database. The dashboard reads from data the factory already produces:

| Data | Source | Access |
|---|---|---|
| Pipeline status | `/data/pipeline/{owner}/{repo}/{issue}.json` | read |
| Feedback history | `/data/feedback/{owner}/{repo}/log.jsonl` | read |
| Eval results | `tests/golden/` output | read |
| Configuration | `config.json` | read/write |
| Issue/PR status | GitHub API (via dispatcher credentials) | read |
| Structured logs | stdout JSON logs (captured by container runtime) | read |

### Deployment

```
Host machine
│
├── ... (existing services)
│
└── factory-dashboard (systemd service or k3s Deployment)
      Port: 8080
      Mounts: /data/pipeline (read), /data/feedback (read), config.json (rw)
      Env: DASHBOARD_TOKEN=<random-token>
      Auth: Bearer token in Authorization header
      Resources: 0.1 core, 128 MB
```

### Security

- Auth: single bearer token from `DASHBOARD_TOKEN` env var
- Credentials are write-only — the API never returns stored secrets
- Config writes go through validation before saving
- The dashboard uses the factory-dispatcher GitHub App credentials for
  read-only GitHub API access (issue/PR status)
- No direct access to sandboxes or LLM endpoints

---

## 15. Migration Phases

Incremental migration from the current monolith. Each phase is independently
deployable and testable.

### Phase 0: Extract shared packages

Move code from `cmd/main.go` into shared packages. No behavior change.

- Move `cmd/config.go` → `config/config.go` (change package, export)
- Create `pipeline/state.go` with `State` struct and `LoadState`/`SaveState`
- Create `pipeline/parse.go` with `ParseCodeOutput`,
  `ReviewNeedsIteration`, `FormatReviewFeedback`
- Create `pipeline/validate.go` with `ValidateFiles`
- Create `harness/toolsets.go` with `BuildGatherTools`, `BuildCoderTools`,
  `SerenaGatherAllowed`, `SerenaCoderAllowed`
- Verify monolithic `cmd/main.go` still compiles using new imports

### Phase 1: Split into separate binaries

Create `cmd/{dispatcher,gatherer,...}/main.go`. Each is a thin main that
loads config, loads/saves state, and calls the appropriate agent function.

The dispatcher invokes phase binaries via `exec.Command()` — subprocesses
in the same container. State passes through the filesystem.

Verify: process an issue end-to-end in shadow mode.

### Phase 2: Three GitHub Apps

Create three GitHub Apps. Add `TokenWithPermissions()` to
`github/app_auth.go`. Update config to support multiple App identities.

Verify: gatherer can read files but cannot create PRs. Committer can create
PRs. Researcher gets no GitHub token.

### Phase 3: Pipeline state serialization

Wire `pipeline.State` end-to-end. Each binary reads/writes state.
Replace in-memory variable passing with state file round-trips.

Verify: compare output of stateful pipeline vs. monolith on same issue.

### Phase 4: Review adapter interface

Create `review/` package with `ExternalReviewAdapter` interface.
Implement `QodoAdapter`. Wire into the feedback binary.

Verify: factory parses Qodo's `/agentic_review` output into
`[]ReviewFinding`.

### Phase 5: Arbiter phase

Add DeepSeek API client (OpenAI-compatible, reuse `openai/` package).
Build arbiter logic in the reviewer binary. Produces `ArbiterResult`.

Verify: arbiter correctly classifies findings as fix_here/subtask/
root_cause/dismissed on test cases.

### Phase 6: Feedback binary

Build `cmd/feedback/main.go`. Implements the post-PR review loop:
poll for external review, trigger factory review, run arbiter,
invoke iterator if needed, log structured feedback.

Verify: end-to-end PR with Qodo review → arbiter → iterate → re-review.

### Phase 7: OpenShell integration

Build `openshell/` package. Write network policy YAML per phase.
Modify dispatcher to create OpenShell sandboxes instead of
`exec.Command()`.

Start with coder only (highest risk). Expand to other phases.

Verify: coder sandbox cannot reach api.github.com. State file
round-trips through upload/download.

### Phase 8: Sandbox images

Build Dockerfiles for base + go + rust + typescript images.
Configure per-repo image selection.

Verify: each image has the required language server and tools.

### Phase 9: Guardrails

Implement all guardrails in dispatcher, committer, and feedback binary.
Add `pipeline/guardrails.go` with limit checks.

Verify: hitting max_iterations labels `factory:needs-human`. Committer
rejects blocked paths. Root cause depth > 1 is blocked.

### Phase 10: Golden-set evaluation

Build `cmd/eval/main.go` and `eval/` package.
Create initial test cases (3 per agent role minimum).

Verify: `./bin/eval --dir tests/golden/planner` runs and reports pass
rates.

### Phase 11: Post-merge monitoring

Add post-merge CI watching to the feedback binary.
Implement automatic revert PR creation on CI failure.

Verify: simulate CI failure after merge, verify revert PR is created.

### Phase 12: Human review adapter + merge automation

Build `HumanAdapter` in `review/adapters/human.go`.
Add merge logic to committer (check status checks, merge on approval).

Verify: human approval triggers merge. Human change request triggers
re-arbitration.

### Phase 13: Dashboard

Build `cmd/dashboard/main.go` with Go HTTP server.
Build React frontend in `cmd/dashboard/frontend/`.
Embed frontend via `embed.FS` for single-binary deployment.
Implement API routes for pipeline status, config, feedback, evals, control.

Verify: dashboard shows live pipeline status, feedback trends render,
config changes persist, credential fields are write-only.

---

## Design Principles

1. **Security is the foundation, not a layer.** Every component designed
   with adversarial thinking. Sandbox isolation, scoped credentials, blocked
   paths.
2. **Autonomy is earned, not granted.** Repos graduate from shadow mode
   based on demonstrated safety. Root cause rates and human override rates
   are the graduation metrics.
3. **Deterministic where possible.** Guardrails, file validation, merge
   gates — all in Go code, never in LLM judgment. Recurring agent judgments
   should be codified into linter rules or scanner policies.
4. **Zero framework cognition.** The orchestrator handles mechanics. All
   judgment is deferred to LLMs via prompts. No judgment calls in framework
   code.
5. **Pluggable external review.** The external reviewer is behind an
   interface. Swapping Qodo for another tool requires one adapter, not a
   rewrite.
6. **The factory improves itself.** Structured feedback from every review
   cycle drives prompt updates, linter additions, and golden-set growth.
   Root cause issues close the loop.
7. **Agents communicate through forge artifacts.** Issues, PRs, comments,
   labels, status checks. No side channels, no agent-to-agent API.
8. **Review is harder than generation.** The arbiter role uses a stronger
   model (DeepSeek) than the generation roles (local qwen3:14b). Review
   deserves more intelligence, not less.
