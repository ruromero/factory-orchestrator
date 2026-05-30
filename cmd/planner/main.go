package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/ruromero/la-fabriquilla/agents"
	helpers "github.com/ruromero/la-fabriquilla/cmd/internal"
	"github.com/ruromero/la-fabriquilla/openai"
	"github.com/ruromero/la-fabriquilla/traces"
)

func main() {
	cfg, state := helpers.MustLoadConfigAndState()

	if cfg.Planner.BaseURL == "" || cfg.Planner.APIKey == "" {
		slog.Error("planner API not configured")
		os.Exit(1)
	}

	client := openai.NewClient(cfg.Planner.BaseURL, cfg.Planner.APIKey)
	ctx := context.Background()

	start := time.Now()
	plan, err := agents.Plan(ctx, client, cfg.Planner.Model,
		state.IssueTitle, state.IssueBody,
		state.ResearchContext, state.GatheredContext,
		state.Conventions, state.CommentHistory)
	elapsed := time.Since(start)
	if err != nil {
		slog.Error("plan phase failed", "error", err)
		os.Exit(1)
	}

	state.RecordTokenUsage("planner", plan.Model, plan.PromptTokens, plan.CompTokens, 0, elapsed.Seconds())

	traces.Log(traces.Trace{
		IssueNumber:  state.IssueNumber,
		Phase:        "planner",
		Model:        plan.Model,
		PromptTokens: plan.PromptTokens,
		CompTokens:   plan.CompTokens,
		Duration:     elapsed.String(),
		StartedAt:    start,
	})

	state.PlanOutcome = plan.Outcome
	state.PlanContent = plan.Content
	state.Phase = "plan-done"
	helpers.MustSaveState(state)
}
