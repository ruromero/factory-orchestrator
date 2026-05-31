package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	helpers "github.com/ruromero/la-fabriquilla/cmd/internal"
	"github.com/ruromero/la-fabriquilla/config"
	"github.com/ruromero/la-fabriquilla/github"
	"github.com/ruromero/la-fabriquilla/harness"
	"github.com/ruromero/la-fabriquilla/pipeline"
	"github.com/ruromero/la-fabriquilla/sandbox"
)

var configPath string

func main() {
	flag.StringVar(&configPath, "config", "config.json", "path to config file")
	flag.Parse()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	repoNames := make([]string, len(cfg.Repos))
	for i, r := range cfg.Repos {
		repoNames[i] = r.Owner + "/" + r.Repo
	}
	slog.Info("factory orchestrator starting",
		"repos", repoNames,
		"poll_interval", cfg.PollInterval.String(),
		"shadow_mode", cfg.ShadowMode,
	)

	ticker := time.NewTicker(cfg.PollInterval.Duration)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pollAllRepos(ctx, cfg)
		case <-sigCh:
			slog.Info("shutting down")
			cancel()
			return
		case <-ctx.Done():
			return
		}
	}
}

func pollAllRepos(ctx context.Context, cfg config.Config) {
	for _, repo := range cfg.Repos {
		gh, err := helpers.NewGitHubClientForApp(cfg, "dispatcher", repo.Owner, repo.Repo)
		if err != nil {
			slog.Error("failed to create github client", "repo", repo.Owner+"/"+repo.Repo, "error", err)
			continue
		}
		log := slog.With("repo", repo.Owner+"/"+repo.Repo)

		readiness, err := gh.CheckReadiness(ctx)
		if err != nil {
			log.Error("readiness check failed", "error", err)
			continue
		}
		if !readiness.Ready {
			log.Warn("repo not ready", "missing", readiness.Missing)
			notifyReadinessFailure(ctx, gh, readiness)
			continue
		}

		issues, err := gh.ListIssuesByLabel(ctx, "fabriquilla:ready")
		if err != nil {
			log.Error("failed to poll issues", "error", err)
			continue
		}

		for _, issue := range issues {
			log.Info("processing issue", "number", issue.Number, "title", issue.Title)

			if err := gh.AddLabel(ctx, issue.Number, "fabriquilla:in-progress"); err != nil {
				log.Error("failed to add label", "issue", issue.Number, "error", err)
				continue
			}
			if err := gh.RemoveLabel(ctx, issue.Number, "fabriquilla:ready"); err != nil {
				log.Error("failed to remove label", "issue", issue.Number, "error", err)
			}

			if err := processIssue(ctx, gh, cfg, issue); err != nil {
				log.Error("failed to process issue", "issue", issue.Number, "error", err)
				gh.AddLabel(ctx, issue.Number, "fabriquilla:needs-human")
			}
		}
	}
}

func processIssue(ctx context.Context, gh *github.Client, cfg config.Config, issue github.Issue) error {
	log := slog.With("issue", issue.Number)

	store := pipeline.NewFileStateStore(cfg.StateDir)
	key := pipeline.StateKey(gh.Owner(), gh.Repo(), issue.Number)

	rc := harness.LoadRepoContext(ctx, gh)

	issueTitle := sandbox.SanitizeInput(issue.Title)
	issueBody := sandbox.SanitizeInput(issue.Body)
	commentHistory := loadHumanComments(ctx, gh, issue.Number)

	state := &pipeline.State{
		RepoOwner:      gh.Owner(),
		RepoName:       gh.Repo(),
		IssueNumber:    issue.Number,
		Phase:          "init",
		IssueTitle:     issueTitle,
		IssueBody:      issueBody,
		CommentHistory: commentHistory,
		Summaries:      rc.Summaries(),
		Conventions:    rc.Conventions(),
		StartedAt:      time.Now(),
	}

	sess, err := harness.CloneAndStartSerena(ctx, gh, cfg.Serena)
	if err != nil {
		log.Warn("failed to start Serena, continuing without", "error", err)
	}
	if sess != nil {
		defer sess.Cleanup()
		state.CloneDir = sess.CloneDir
	}

	if err := store.Save(ctx, key, state); err != nil {
		return fmt.Errorf("save initial state: %w", err)
	}

	statePath := store.StatePath(key)

	log.Info("starting gather phase")
	if err := runPhase(ctx, &cfg, "gatherer", statePath); err != nil {
		return fmt.Errorf("gather phase: %w", err)
	}

	log.Info("starting research phase")
	if err := runPhase(ctx, &cfg, "researcher", statePath); err != nil {
		log.Warn("research phase failed, continuing", "error", err)
	}

	log.Info("starting plan phase")
	if err := runPhase(ctx, &cfg, "planner", statePath); err != nil {
		return fmt.Errorf("plan phase: %w", err)
	}

	state, err = store.Load(ctx, key)
	if err != nil {
		return fmt.Errorf("reload state after plan: %w", err)
	}

	switch state.PlanOutcome {
	case "needs_info":
		log.Info("planner needs more info")
		comment := fmt.Sprintf("## Factory: Additional Information Needed\n\n%s", state.PlanContent)
		if err := gh.CreateComment(ctx, issue.Number, comment); err != nil {
			return fmt.Errorf("post needs-info comment: %w", err)
		}
		gh.RemoveLabel(ctx, issue.Number, "fabriquilla:in-progress")
		return gh.AddLabel(ctx, issue.Number, "fabriquilla:needs-info")

	case "decompose":
		log.Info("planner decomposing issue")
		comment := fmt.Sprintf("## Factory: Issue Decomposed\n\nThis issue is too complex for a single PR. Creating sub-issues.\n\n%s", state.PlanContent)
		if err := gh.CreateComment(ctx, issue.Number, comment); err != nil {
			return fmt.Errorf("post decompose comment: %w", err)
		}
		if err := createSubIssues(ctx, gh, issue.Number, state.PlanContent); err != nil {
			return fmt.Errorf("create sub-issues: %w", err)
		}
		gh.RemoveLabel(ctx, issue.Number, "fabriquilla:in-progress")
		return gh.AddLabel(ctx, issue.Number, "fabriquilla:tracking")

	case "plan":
		log.Info("plan produced, posting to issue")
		comment := fmt.Sprintf("## Factory: Implementation Plan\n\n%s", state.PlanContent)
		if state.ResearchContext != "" {
			comment += fmt.Sprintf("\n\n<details><summary>Research Context</summary>\n\n%s\n\n</details>", state.ResearchContext)
		}
		if err := gh.CreateComment(ctx, issue.Number, comment); err != nil {
			return fmt.Errorf("post plan comment: %w", err)
		}

		log.Info("starting design phase")
		if err := runPhase(ctx, &cfg, "designer", statePath); err != nil {
			return fmt.Errorf("design phase: %w", err)
		}

		log.Info("starting code phase (includes review+iterate)")
		if err := runPhase(ctx, &cfg, "coder", statePath); err != nil {
			return fmt.Errorf("code phase: %w", err)
		}

		log.Info("starting commit phase")
		if err := runPhase(ctx, &cfg, "committer", statePath); err != nil {
			return fmt.Errorf("commit phase: %w", err)
		}

		state, err = store.Load(ctx, key)
		if err != nil {
			return fmt.Errorf("reload state after commit: %w", err)
		}

		if state.PRNumber > 0 {
			log.Info("PR created", "pr", state.PRNumber)
		}
		return nil
	}

	return fmt.Errorf("unknown plan outcome: %s", state.PlanOutcome)
}

func runPhase(ctx context.Context, cfg *config.Config, binary, statePath string) error {
	maxRetries := cfg.MaxPhaseRetries
	if maxRetries <= 0 {
		maxRetries = 2
	}
	timeout := cfg.PhaseDuration(binary)

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(30<<(attempt-1)) * time.Second // 30s, 60s, 120s
			slog.Info("retrying phase", "phase", binary, "attempt", attempt+1, "backoff", backoff)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		phaseCtx, cancel := context.WithTimeout(ctx, timeout)
		cmd := exec.CommandContext(phaseCtx, binary)
		cmd.Env = append(os.Environ(),
			"PIPELINE_STATE_PATH="+statePath,
			"CONFIG_PATH="+configPath,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		timedOut := phaseCtx.Err() == context.DeadlineExceeded
		cancel()

		if err == nil {
			return nil
		}

		lastErr = fmt.Errorf("%s (attempt %d/%d): %w", binary, attempt+1, maxRetries+1, err)
		slog.Warn("phase failed", "phase", binary, "attempt", attempt+1, "error", err, "timed_out", timedOut)

		if !timedOut && !isRetryable(err) {
			return lastErr
		}
	}
	return lastErr
}

func isRetryable(err error) bool {
	// Timeout (hung phase) — retryable
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// Signal kill (OOM, etc.) — retryable
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode() > 128 // killed by signal
	}
	// Normal exit code (1, 2, etc.) — permanent failure
	return false
}

func loadHumanComments(ctx context.Context, gh *github.Client, issueNumber int) string {
	comments, err := gh.ListComments(ctx, issueNumber)
	if err != nil {
		slog.Warn("could not load issue comments", "issue", issueNumber, "error", err)
		return ""
	}

	var b strings.Builder
	for _, c := range comments {
		if strings.HasSuffix(c.User.Login, "[bot]") {
			continue
		}
		body := sandbox.SanitizeInput(c.Body)
		if body == "" {
			continue
		}
		fmt.Fprintf(&b, "**@%s**:\n%s\n\n", c.User.Login, body)
	}
	return strings.TrimSpace(b.String())
}

const readinessCommentMarker = "<!-- fabriquilla:readiness -->"

func notifyReadinessFailure(ctx context.Context, gh *github.Client, readiness github.ReadinessResult) {
	issues, err := gh.ListIssuesByLabel(ctx, "fabriquilla:ready")
	if err != nil || len(issues) == 0 {
		return
	}

	comment := fmt.Sprintf("%s\n## Factory: Repository Not Ready\n\nThis repository is missing required files:\n\n", readinessCommentMarker)
	for _, f := range readiness.Missing {
		comment += fmt.Sprintf("- `%s`\n", f)
	}
	comment += "\nSee [Repo readiness](https://github.com/ruromero/la-fabriquilla#repo-readiness) for details on required files.\n"
	comment += "Once the missing files are added, relabel this issue `fabriquilla:ready` to retry."

	for _, issue := range issues {
		existing, err := gh.ListComments(ctx, issue.Number)
		if err != nil {
			continue
		}
		alreadyNotified := false
		for _, c := range existing {
			if strings.Contains(c.Body, readinessCommentMarker) {
				alreadyNotified = true
				break
			}
		}
		if alreadyNotified {
			continue
		}

		if err := gh.CreateComment(ctx, issue.Number, comment); err != nil {
			slog.Error("failed to post readiness comment", "issue", issue.Number, "error", err)
			continue
		}
		gh.RemoveLabel(ctx, issue.Number, "fabriquilla:ready")
		gh.AddLabel(ctx, issue.Number, "fabriquilla:requirements")
		slog.Info("notified issue about missing requirements", "issue", issue.Number, "missing", readiness.Missing)
	}
}

func createSubIssues(ctx context.Context, gh *github.Client, parentNumber int, decomposeContent string) error {
	lines := strings.Split(decomposeContent, "\n")
	var subIssues []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			title := strings.TrimLeft(trimmed, "-* ")
			if title != "" {
				subIssues = append(subIssues, title)
			}
		}
	}

	var checklist strings.Builder
	checklist.WriteString(fmt.Sprintf("Sub-issues created from #%d:\n\n", parentNumber))

	for _, title := range subIssues {
		body := fmt.Sprintf("Parent issue: #%d\n\nSub-task: %s", parentNumber, title)
		created, err := gh.CreateIssue(ctx, title, body, []string{"fabriquilla:ready"})
		if err != nil {
			return fmt.Errorf("create sub-issue %q: %w", title, err)
		}
		checklist.WriteString(fmt.Sprintf("- [ ] #%d — %s\n", created.Number, title))
		slog.Info("created sub-issue", "parent", parentNumber, "child", created.Number, "title", title)
	}

	return gh.CreateComment(ctx, parentNumber, checklist.String())
}
