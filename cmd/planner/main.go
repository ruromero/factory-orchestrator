package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/ruromero/la-fabriquilla/agents"
	helpers "github.com/ruromero/la-fabriquilla/cmd/internal"
	"github.com/ruromero/la-fabriquilla/openai"
)

func main() {
	cfg, state := helpers.MustLoadConfigAndState()

	if cfg.Planner.BaseURL == "" || cfg.Planner.APIKey == "" {
		slog.Error("planner API not configured")
		os.Exit(1)
	}

	client := openai.NewClient(cfg.Planner.BaseURL, cfg.Planner.APIKey)
	ctx := context.Background()

	plan, err := agents.Plan(ctx, client, cfg.Planner.Model,
		state.IssueTitle, state.IssueBody,
		state.ResearchContext, state.GatheredContext,
		state.Conventions, state.CommentHistory)
	if err != nil {
		slog.Error("plan phase failed", "error", err)
		os.Exit(1)
	}

	state.PlanOutcome = plan.Outcome
	state.PlanContent = plan.Content
	state.Phase = "plan-done"
	helpers.MustSaveState(state)
}
