package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ruromero/factory-orchestrator/github"
	"github.com/ruromero/factory-orchestrator/ollama"
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
			pollAllRepos(ctx, ol, cfg)
		case <-sigCh:
			slog.Info("shutting down")
			cancel()
			return
		case <-ctx.Done():
			return
		}
	}
}

func pollAllRepos(ctx context.Context, ol *ollama.Client, cfg Config) {
	for _, repo := range cfg.Repos {
		gh := github.NewClient(repo.Token, repo.Owner, repo.Repo)
		log := slog.With("repo", repo.Owner+"/"+repo.Repo)

		readiness, err := gh.CheckReadiness(ctx)
		if err != nil {
			log.Error("readiness check failed", "error", err)
			continue
		}
		if !readiness.Ready {
			log.Warn("repo not ready, skipping", "missing", readiness.Missing)
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

			if err := processIssue(ctx, gh, ol, cfg, issue); err != nil {
				log.Error("failed to process issue", "issue", issue.Number, "error", err)
			}
		}
	}
}

func processIssue(ctx context.Context, gh *github.Client, ol *ollama.Client, cfg Config, issue github.Issue) error {
	// Phase 1: Research (Gemini) — TODO
	// Phase 2: Plan
	// Phase 3: Design
	// Phase 4: Code
	// Phase 5: Review
	// Phase 6: Iterate
	return nil
}
