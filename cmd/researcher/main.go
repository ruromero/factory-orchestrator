package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/ruromero/la-fabriquilla/agents"
	helpers "github.com/ruromero/la-fabriquilla/cmd/internal"
	"github.com/ruromero/la-fabriquilla/gemini"
	"github.com/ruromero/la-fabriquilla/traces"
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

	start := time.Now()
	result, err := agents.ResearchWithUsage(ctx, gem, state.IssueTitle, state.IssueBody, state.Summaries)
	elapsed := time.Since(start)
	if err != nil {
		slog.Warn("research failed, continuing without", "error", err)
		state.Phase = "research-done"
		helpers.MustSaveState(state)
		return
	}

	state.RecordTokenUsage("researcher", result.Model, result.PromptTokens, result.CompTokens, 0, elapsed.Seconds())

	traces.Log(traces.Trace{
		IssueNumber:     state.IssueNumber,
		Phase:           "researcher",
		Model:           result.Model,
		PromptTokens:    result.PromptTokens,
		CompTokens:      result.CompTokens,
		Duration:        elapsed.String(),
		StartedAt:       start,
		CumPromptTokens: state.TotalPromptTokens,
		CumCompTokens:   state.TotalCompTokens,
		CumCostUSD:      state.TotalCostUSD,
	})

	state.ResearchContext = result.Content
	state.Phase = "research-done"
	helpers.MustSaveState(state)
}
