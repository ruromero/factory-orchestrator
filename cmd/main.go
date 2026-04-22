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
	"github.com/ruromero/factory-orchestrator/mcp"
	"github.com/ruromero/factory-orchestrator/ollama"
	"github.com/ruromero/factory-orchestrator/openai"
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

	var planner *openai.Client
	if cfg.Planner.BaseURL != "" && cfg.Planner.APIKey != "" {
		planner = openai.NewClient(cfg.Planner.BaseURL, cfg.Planner.APIKey)
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
			pollAllRepos(ctx, ol, gem, planner, cfg)
		case <-sigCh:
			slog.Info("shutting down")
			cancel()
			return
		case <-ctx.Done():
			return
		}
	}
}

func pollAllRepos(ctx context.Context, ol *ollama.Client, gem *gemini.Client, planner *openai.Client, cfg Config) {
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

			if err := processIssue(ctx, gh, ol, gem, planner, cfg, issue); err != nil {
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

type serenaSession struct {
	cloneDir string
	serena   *mcp.Client
	cleanup  func()
}

func startSerena(ctx context.Context, gh *github.Client, cfg Config, log *slog.Logger) *serenaSession {
	if !cfg.Serena.Enabled() {
		return nil
	}

	cloneDir, cloneCleanup, err := gh.CloneShallow(ctx)
	if err != nil {
		log.Warn("failed to clone repo for Serena", "error", err)
		return nil
	}

	lspBinDir, err := harness.InstallLanguageServers(ctx, cloneDir)
	if err != nil {
		log.Warn("failed to set up language servers", "error", err)
	}

	args := append(cfg.Serena.Args, "--project", cloneDir)
	serena := mcp.NewClient(cfg.Serena.Command, args...)
	if lspBinDir != "" {
		env := os.Environ()
		npmBin := fmt.Sprintf("%s/bin", lspBinDir)
		env = append(env, fmt.Sprintf("PATH=%s:%s:%s", lspBinDir, npmBin, os.Getenv("PATH")))
		serena.SetEnv(env)
	}
	if err := serena.Start(ctx); err != nil {
		log.Warn("failed to start Serena", "error", err)
		cloneCleanup()
		return nil
	}

	return &serenaSession{
		cloneDir: cloneDir,
		serena:   serena,
		cleanup: func() {
			if err := serena.Stop(); err != nil {
				log.Warn("failed to stop Serena", "error", err)
			}
			cloneCleanup()
		},
	}
}

func processIssue(ctx context.Context, gh *github.Client, ol *ollama.Client, gem *gemini.Client, planner *openai.Client, cfg Config, issue github.Issue) error {
	log := slog.With("issue", issue.Number)

	rc := harness.LoadRepoContext(ctx, gh)

	issueTitle := sandbox.SanitizeInput(issue.Title)
	issueBody := sandbox.SanitizeInput(issue.Body)

	commentHistory := loadHumanComments(ctx, gh, issue.Number)

	sess := startSerena(ctx, gh, cfg, log)
	if sess != nil {
		defer sess.cleanup()
	}

	gatherTools, gatherHandler := buildGatherTools(rc, gh, sess)

	log.Info("starting context gathering phase")
	gatheredCtx, err := agents.GatherContext(ctx, ol, issueTitle, issueBody, rc.Summaries(), gatherTools, gatherHandler)
	if err != nil {
		log.Warn("context gathering failed, continuing with summaries", "error", err)
		gatheredCtx = rc.Summaries()
	}

	log.Info("starting research phase")
	researchCtx, err := agents.Research(ctx, gem, issueTitle, issueBody, rc.Summaries())
	if err != nil {
		log.Warn("research phase failed, continuing without", "error", err)
		researchCtx = ""
	}

	log.Info("starting plan phase")
	plan, err := agents.Plan(ctx, planner, cfg.Planner.Model, issueTitle, issueBody, researchCtx, gatheredCtx, rc.Conventions(), commentHistory)
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

		log.Info("starting design phase")
		design, err := agents.Design(ctx, ol, plan.Content, researchCtx, rc.Conventions())
		if err != nil {
			return fmt.Errorf("design phase: %w", err)
		}

		log.Info("starting code phase")
		coderTools, coderHandler := buildCoderTools(sess)
		code, err := agents.Code(ctx, ol, design, researchCtx, rc.Conventions(), coderTools, coderHandler)
		if err != nil {
			return fmt.Errorf("code phase: %w", err)
		}

		log.Info("starting review phase")
		review, err := agents.Review(ctx, ol, code, design, plan.Content, rc.Conventions(), gatherTools, gatherHandler)
		if err != nil {
			return fmt.Errorf("review phase: %w", err)
		}

		for i := 0; i < cfg.MaxIterations && reviewNeedsIteration(review); i++ {
			log.Info("starting iteration", "iteration", i+1, "max", cfg.MaxIterations)
			feedback := formatReviewFeedback(review)
			code, err = agents.Iterate(ctx, ol, code, feedback, coderTools, coderHandler)
			if err != nil {
				return fmt.Errorf("iterate phase %d: %w", i+1, err)
			}
			review, err = agents.Review(ctx, ol, code, design, plan.Content, rc.Conventions(), gatherTools, gatherHandler)
			if err != nil {
				return fmt.Errorf("review phase (iteration %d): %w", i+1, err)
			}
		}

		files := parseCodeOutput(code)
		if len(files) == 0 {
			return fmt.Errorf("no file changes parsed from code output")
		}

		if cfg.ShadowMode {
			log.Info("shadow mode: posting code as comment instead of PR", "files", len(files))
			comment := fmt.Sprintf("## Factory: Implementation (Shadow Mode)\n\n%d file(s) generated.\n\n%s", len(files), code)
			gh.CreateComment(ctx, issue.Number, comment)
			gh.RemoveLabel(ctx, issue.Number, "factory:in-progress")
			gh.AddLabel(ctx, issue.Number, "factory:done")
			return nil
		}

		branchName := fmt.Sprintf("factory/issue-%d", issue.Number)
		baseSHA, err := gh.GetBranchSHA(ctx, "main")
		if err != nil {
			return fmt.Errorf("get base branch SHA: %w", err)
		}
		if err := gh.CreateBranch(ctx, branchName, baseSHA); err != nil {
			return fmt.Errorf("create branch: %w", err)
		}

		commitMsg := fmt.Sprintf("feat: %s (#%d)", issueTitle, issue.Number)
		if _, err := gh.CreateCommit(ctx, branchName, commitMsg, files); err != nil {
			return fmt.Errorf("create commit: %w", err)
		}

		prTitle := fmt.Sprintf("feat: %s", issueTitle)
		prBody := fmt.Sprintf("Closes #%d\n\n## Plan\n\n%s\n\n## Review\n\n### Correctness\n%s\n\n### Security\n%s\n\n### Intent\n%s",
			issue.Number, plan.Content, review.Correctness, review.Security, review.Intent)
		pr, err := gh.CreatePullRequest(ctx, prTitle, prBody, branchName, "main")
		if err != nil {
			return fmt.Errorf("create PR: %w", err)
		}

		log.Info("PR created", "pr", pr.Number, "url", pr.HTMLURL)
		gh.RemoveLabel(ctx, issue.Number, "factory:in-progress")
		gh.AddLabel(ctx, issue.Number, "factory:done")
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

func buildGatherTools(rc *harness.RepoContext, gh *github.Client, sess *serenaSession) ([]ollama.Tool, ollama.ToolHandler) {
	contextHandler := harness.NewContextToolHandler(rc, gh)
	contextTools := harness.ContextTools()

	if sess == nil {
		return contextTools, contextHandler
	}

	serenaReadTools := harness.FilterTools(sess.serena.Tools(), serenaGatherAllowed)

	composite := harness.NewCompositeToolHandler()
	composite.Register(contextTools, contextHandler)
	composite.Register(serenaReadTools, sess.serena)

	allTools := append(contextTools, serenaReadTools...)
	return allTools, composite
}

func buildCoderTools(sess *serenaSession) ([]ollama.Tool, ollama.ToolHandler) {
	if sess == nil {
		return nil, nil
	}
	tools := harness.FilterTools(sess.serena.Tools(), serenaCoderAllowed)
	return tools, sess.serena
}

var serenaGatherAllowed = map[string]bool{
	"find_symbol":                    true,
	"find_referencing_symbols":       true,
	"find_referencing_code_snippets": true,
	"get_symbols_overview":           true,
	"read_file":                      true,
	"list_dir":                       true,
	"search_for_pattern":             true,
}

var serenaCoderAllowed = map[string]bool{
	"find_symbol":                    true,
	"find_referencing_symbols":       true,
	"find_referencing_code_snippets": true,
	"get_symbols_overview":           true,
	"read_file":                      true,
	"list_dir":                       true,
	"search_for_pattern":             true,
	"replace_symbol_body":            true,
	"insert_before_symbol":           true,
	"insert_after_symbol":            true,
	"replace_content":                true,
}

func parseCodeOutput(output string) []github.FileChange {
	var files []github.FileChange
	lines := strings.Split(output, "\n")
	var currentPath string
	var content strings.Builder
	inBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "FILE:") {
			currentPath = strings.TrimSpace(strings.TrimPrefix(trimmed, "FILE:"))
			continue
		}
		if !inBlock && strings.HasPrefix(trimmed, "```") && currentPath != "" {
			inBlock = true
			content.Reset()
			continue
		}
		if inBlock && trimmed == "```" {
			files = append(files, github.FileChange{
				Path:    currentPath,
				Content: strings.TrimRight(content.String(), "\n"),
			})
			currentPath = ""
			inBlock = false
			continue
		}
		if inBlock {
			content.WriteString(line)
			content.WriteString("\n")
		}
	}
	return files
}

func reviewNeedsIteration(review agents.ReviewResult) bool {
	combined := review.Correctness + review.Security + review.Intent
	return strings.Contains(combined, "[CRITICAL]") || strings.Contains(combined, "[MEDIUM]")
}

func formatReviewFeedback(review agents.ReviewResult) string {
	return fmt.Sprintf("## Correctness Review\n\n%s\n\n## Security Review\n\n%s\n\n## Intent Review\n\n%s",
		review.Correctness, review.Security, review.Intent)
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
