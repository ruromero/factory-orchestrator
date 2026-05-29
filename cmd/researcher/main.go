package main

import (
	"context"
	"log/slog"

	"github.com/ruromero/factory-orchestrator/agents"
	helpers "github.com/ruromero/factory-orchestrator/cmd/internal"
	"github.com/ruromero/factory-orchestrator/gemini"
)

func main() {
	cfg, state := helpers.MustLoadConfigAndState()

	if cfg.GeminiAPIKey == "" {
		slog.Info("no Gemini API key configured, skipping research")
		state.Phase = "research-done"
		helpers.MustSaveState(state)
		return
	}

	gem := gemini.NewClient(cfg.GeminiAPIKey)
	ctx := context.Background()

	result, err := agents.Research(ctx, gem, state.IssueTitle, state.IssueBody, state.Summaries)
	if err != nil {
		slog.Warn("research failed, continuing without", "error", err)
		state.Phase = "research-done"
		helpers.MustSaveState(state)
		return
	}

	state.ResearchContext = result
	state.Phase = "research-done"
	helpers.MustSaveState(state)
}
