package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/ruromero/factory-orchestrator/agents"
	helpers "github.com/ruromero/factory-orchestrator/cmd/internal"
	"github.com/ruromero/factory-orchestrator/ollama"
)

func main() {
	cfg, state := helpers.MustLoadConfigAndState()

	ol := ollama.NewClient(cfg.OllamaURL)
	ctx := context.Background()

	result, err := agents.Design(ctx, ol, state.PlanContent, state.ResearchContext, state.Conventions)
	if err != nil {
		slog.Error("design phase failed", "error", err)
		os.Exit(1)
	}

	state.Design = result
	state.Phase = "design-done"
	helpers.MustSaveState(state)
}
