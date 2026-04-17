package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ruromero/factory-orchestrator/agents"
	"github.com/ruromero/factory-orchestrator/gemini"
	"github.com/ruromero/factory-orchestrator/github"
	"github.com/ruromero/factory-orchestrator/harness"
	"github.com/ruromero/factory-orchestrator/ollama"
	"github.com/ruromero/factory-orchestrator/sandbox"
)

func main() {
	configPath := flag.String("config", "config.json", "path to config file")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	ol := ollama.NewClient(cfg.OllamaURL)

	var gem *gemini.Client
	if cfg.GeminiAPIKey != "" {
		gem = gemini.NewClient(cfg.GeminiAPIKey)
	}

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
			pollAllRepos(ctx, ol, gem, cfg)
		case <-sigCh:
			slog.Info("shutting down")
			cancel()
			return
		case <-ctx.Done():
			return
		}
	}
}

func pollAllRepos(ctx context.Context, ol *ollama.Client, gem *gemini.Client, cfg Config) {
	for _, repo := range cfg.Repos {
		gh, err := newGitHubClient(repo)
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

		issues, err := gh.ListIssuesByLabel(ctx, "factory:ready")
		if err != nil {
			log.Error("failed to poll issues", "error", err)
			continue
		}

		for _, issue := range issues {
			log.Info("processing issue", "number", issue.Number, "title", issue.Title)

			if err := gh.AddLabel(ctx, issue.Number, "factory:in-progress"); err != nil {
				log.Error("failed to add label", "issue", issue.Number, "error", err)
				continue
			}
			if err := gh.RemoveLabel(ctx, issue.Number, "factory:ready"); err != nil {
				log.Error("failed to remove label", "issue", issue.Number, "error", err)
			}

			if err := processIssue(ctx, gh, ol, gem, cfg, issue); err != nil {
				log.Error("failed to process issue", "issue", issue.Number, "error", err)
				gh.AddLabel(ctx, issue.Number, "factory:needs-human")
			}
		}
	}
}

func newGitHubClient(repo RepoConfig) (*github.Client, error) {
	if repo.UsesAppAuth() {
		auth, err := github.NewAppAuth(repo.AppID, repo.PrivateKeyPath, repo.InstallationID)
		if err != nil {
			return nil, fmt.Errorf("app auth: %w", err)
		}
		return github.NewClientWithAppAuth(auth, repo.Owner, repo.Repo), nil
	}
	return github.NewClient(repo.Token, repo.Owner, repo.Repo), nil
}

func processIssue(ctx context.Context, gh *github.Client, ol *ollama.Client, gem *gemini.Client, cfg Config, issue github.Issue) error {
	log := slog.With("issue", issue.Number)

	rc := harness.LoadRepoContext(ctx, gh)
	handler := harness.NewContextToolHandler(rc, gh)

	issueTitle := sandbox.SanitizeInput(issue.Title)
	issueBody := sandbox.SanitizeInput(issue.Body)

	commentHistory := loadHumanComments(ctx, gh, issue.Number)

	log.Info("starting context gathering phase")
	gatheredCtx, err := agents.GatherContext(ctx, ol, issueTitle, issueBody, rc.Summaries(), harness.ContextTools(), handler)
	if err != nil {
		log.Warn("context gathering failed, continuing with summaries", "error", err)
		gatheredCtx = rc.Summaries()
	}

	log.Info("starting research phase")
	researchCtx, err := agents.Research(ctx, gem, issueTitle, issueBody)
	if err != nil {
		log.Warn("research phase failed, continuing without", "error", err)
		researchCtx = ""
	}

	log.Info("starting plan phase")
	plan, err := agents.Plan(ctx, ol, issueTitle, issueBody, researchCtx, gatheredCtx, rc.Conventions(), commentHistory)
	if err != nil {
		return fmt.Errorf("plan phase: %w", err)
	}

	switch plan.Outcome {
	case "needs_info":
		log.Info("planner needs more info")
		comment := fmt.Sprintf("## Factory: Additional Information Needed\n\n%s", plan.Content)
		if err := gh.CreateComment(ctx, issue.Number, comment); err != nil {
			return fmt.Errorf("post needs-info comment: %w", err)
		}
		gh.RemoveLabel(ctx, issue.Number, "factory:in-progress")
		return gh.AddLabel(ctx, issue.Number, "factory:needs-info")

	case "decompose":
		log.Info("planner decomposing issue")
		comment := fmt.Sprintf("## Factory: Issue Decomposed\n\nThis issue is too complex for a single PR. Creating sub-issues.\n\n%s", plan.Content)
		if err := gh.CreateComment(ctx, issue.Number, comment); err != nil {
			return fmt.Errorf("post decompose comment: %w", err)
		}
		if err := createSubIssues(ctx, gh, issue.Number, plan.Content); err != nil {
			return fmt.Errorf("create sub-issues: %w", err)
		}
		gh.RemoveLabel(ctx, issue.Number, "factory:in-progress")
		return gh.AddLabel(ctx, issue.Number, "factory:tracking")

	case "plan":
		log.Info("plan produced, posting to issue")
		comment := fmt.Sprintf("## Factory: Implementation Plan\n\n%s", plan.Content)
		if researchCtx != "" {
			comment += fmt.Sprintf("\n\n<details><summary>Research Context</summary>\n\n%s\n\n</details>", researchCtx)
		}
		if err := gh.CreateComment(ctx, issue.Number, comment); err != nil {
			return fmt.Errorf("post plan comment: %w", err)
		}

		return nil
	}

	return fmt.Errorf("unknown plan outcome: %s", plan.Outcome)
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

const readinessCommentMarker = "<!-- factory:readiness -->"

func notifyReadinessFailure(ctx context.Context, gh *github.Client, readiness github.ReadinessResult) {
	issues, err := gh.ListIssuesByLabel(ctx, "factory:ready")
	if err != nil || len(issues) == 0 {
		return
	}

	comment := fmt.Sprintf("%s\n## Factory: Repository Not Ready\n\nThis repository is missing required files:\n\n", readinessCommentMarker)
	for _, f := range readiness.Missing {
		comment += fmt.Sprintf("- `%s`\n", f)
	}
	comment += "\nSee [Repo readiness](https://github.com/ruromero/factory-orchestrator#repo-readiness) for details on required files.\n"
	comment += "Once the missing files are added, relabel this issue `factory:ready` to retry."

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
		gh.RemoveLabel(ctx, issue.Number, "factory:ready")
		gh.AddLabel(ctx, issue.Number, "factory:requirements")
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
		created, err := gh.CreateIssue(ctx, title, body, []string{"factory:ready"})
		if err != nil {
			return fmt.Errorf("create sub-issue %q: %w", title, err)
		}
		checklist.WriteString(fmt.Sprintf("- [ ] #%d — %s\n", created.Number, title))
		slog.Info("created sub-issue", "parent", parentNumber, "child", created.Number, "title", title)
	}

	return gh.CreateComment(ctx, parentNumber, checklist.String())
}
