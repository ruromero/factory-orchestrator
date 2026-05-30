package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/ruromero/factory-orchestrator/agents"
	helpers "github.com/ruromero/factory-orchestrator/cmd/internal"
	"github.com/ruromero/factory-orchestrator/harness"
	"github.com/ruromero/factory-orchestrator/mcp"
	"github.com/ruromero/factory-orchestrator/ollama"
)

func main() {
	cfg, state := helpers.MustLoadConfigAndState()

	ol := ollama.NewClient(cfg.OllamaURL)
	gh := helpers.MustGitHubClientForApp(cfg, "worker", state)

	ctx := context.Background()

	var sess *harness.SerenaSession
	if state.CloneDir != "" {
		var err error
		sess, err = harness.StartSerenaFromClone(ctx, state.CloneDir, cfg.Serena)
		if err != nil {
			slog.Warn("failed to start Serena", "error", err)
		}
	}
	if sess != nil {
		defer sess.Cleanup()
	}

	rc := harness.LoadRepoContext(ctx, gh)

	var serenaClient *mcp.Client
	if sess != nil {
		serenaClient = sess.Client
	}
	tools, handler := harness.BuildGatherTools(rc, gh, serenaClient)

	result, err := agents.GatherContext(ctx, ol, state.IssueTitle, state.IssueBody, state.Summaries, tools, handler)
	if err != nil {
		slog.Error("gather context failed", "error", err)
		os.Exit(1)
	}

	state.GatheredContext = result
	state.Phase = "gather-done"
	helpers.MustSaveState(state)
}
