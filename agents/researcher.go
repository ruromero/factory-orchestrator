package agents

import (
	"context"
	"fmt"
)

// ResearchFunc is the function signature for the research phase.
// It will be backed by Gemini API calls.
type ResearchFunc func(ctx context.Context, issueTitle, issueBody string) (string, error)

// PlaceholderResearch returns a no-op researcher for initial development.
func PlaceholderResearch() ResearchFunc {
	return func(ctx context.Context, issueTitle, issueBody string) (string, error) {
		return fmt.Sprintf("No research context available for: %s", issueTitle), nil
	}
}
