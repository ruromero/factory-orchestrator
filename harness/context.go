package harness

import (
	"context"
	"log/slog"

	"github.com/ruromero/factory-orchestrator/github"
)

type PhaseContext struct {
	IssueTitle      string
	IssueBody       string
	ResearchContext string
	Conventions     string
	Architecture    string
	Readme          string
	Plan            string
	Design          string
	Code            string
	ReviewFeedback  string
}

func LoadRepoContext(ctx context.Context, gh *github.Client) PhaseContext {
	var pc PhaseContext

	for _, f := range []struct {
		path string
		dest *string
	}{
		{"CONVENTIONS.md", &pc.Conventions},
		{"ARCHITECTURE.md", &pc.Architecture},
		{"README.md", &pc.Readme},
	} {
		content, err := gh.GetFileContent(ctx, f.path)
		if err != nil {
			slog.Warn("could not load repo context file", "file", f.path, "error", err)
		} else {
			*f.dest = content
		}
	}

	return pc
}
