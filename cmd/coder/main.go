package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/ruromero/la-fabriquilla/agents"
	helpers "github.com/ruromero/la-fabriquilla/cmd/internal"
	"github.com/ruromero/la-fabriquilla/harness"
	"github.com/ruromero/la-fabriquilla/mcp"
	"github.com/ruromero/la-fabriquilla/ollama"
	"github.com/ruromero/la-fabriquilla/pipeline"
)

func main() {
	cfg, state := helpers.MustLoadConfigAndState()

	ol := ollama.NewClient(cfg.OllamaURL)
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

	var serenaClient *mcp.Client
	if sess != nil {
		serenaClient = sess.Client
	}
	coderTools, coderHandler := harness.BuildCoderTools(serenaClient)

	gh := helpers.MustGitHubClientForApp(cfg, "worker", state)
	rc := harness.LoadRepoContext(ctx, gh)
	gatherTools, gatherHandler := harness.BuildGatherTools(rc, gh, serenaClient)

	code, err := agents.Code(ctx, ol, state.Design, state.ResearchContext, state.Conventions, coderTools, coderHandler)
	if err != nil {
		slog.Error("code phase failed", "error", err)
		os.Exit(1)
	}

	review, err := agents.Review(ctx, ol, code, state.Design, state.PlanContent, state.Conventions, gatherTools, gatherHandler)
	if err != nil {
		slog.Error("review phase failed", "error", err)
		os.Exit(1)
	}

	for i := 0; i < cfg.MaxIterations && pipeline.ReviewNeedsIteration(review.Correctness, review.Security, review.Intent); i++ {
		slog.Info("starting iteration", "iteration", i+1, "max", cfg.MaxIterations)
		feedback := pipeline.FormatReviewFeedback(review.Correctness, review.Security, review.Intent)
		code, err = agents.Iterate(ctx, ol, code, feedback, coderTools, coderHandler)
		if err != nil {
			slog.Error("iterate phase failed", "iteration", i+1, "error", err)
			os.Exit(1)
		}
		review, err = agents.Review(ctx, ol, code, state.Design, state.PlanContent, state.Conventions, gatherTools, gatherHandler)
		if err != nil {
			slog.Error("review phase failed", "iteration", i+1, "error", err)
			os.Exit(1)
		}
	}

	parsed := pipeline.ParseCodeOutput(code)

	state.Code = code
	state.Review = &pipeline.ReviewState{
		Correctness: review.Correctness,
		Security:    review.Security,
		Intent:      review.Intent,
	}
	state.Files = parsed
	state.Phase = "code-done"
	helpers.MustSaveState(state)
}
