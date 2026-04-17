package harness

import (
	"context"
	"log/slog"

	"github.com/ruromero/factory-orchestrator/github"
)

// PhaseContext holds the assembled context for an agent phase.
type PhaseContext struct {
	IssueTitle      string
	IssueBody       string
	ResearchContext string
	Conventions     string
	Plan            string
	Design          string
	Code            string
	ReviewFeedback  string
}

// LoadRepoContext fetches repo-level context files that all agents need.
func LoadRepoContext(ctx context.Context, gh *github.Client) PhaseContext {
	var pc PhaseContext

	conventions, err := gh.GetFileContent(ctx, "CONVENTIONS.md")
	if err != nil {
		slog.Warn("could not load CONVENTIONS.md", "error", err)
	} else {
		pc.Conventions = conventions
	}

	return pc
}
