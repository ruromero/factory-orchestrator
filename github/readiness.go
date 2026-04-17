package github

import (
	"context"
	"fmt"
	"strings"
)

var requiredFiles = []string{
	"CODEOWNERS",
	"CLAUDE.md",
	"CONVENTIONS.md",
	"ARCHITECTURE.md",
	"README.md",
}

type ReadinessResult struct {
	Ready   bool
	Missing []string
}

// CheckReadiness verifies the repo has the minimum required files
// before the factory will accept work from it.
func (c *Client) CheckReadiness(ctx context.Context) (ReadinessResult, error) {
	var missing []string

	// CODEOWNERS can be in root, .github/, or docs/
	codeownersFound := false
	for _, path := range []string{"CODEOWNERS", ".github/CODEOWNERS", "docs/CODEOWNERS"} {
		exists, err := c.FileExists(ctx, path)
		if err != nil {
			return ReadinessResult{}, fmt.Errorf("check %s: %w", path, err)
		}
		if exists {
			codeownersFound = true
			break
		}
	}
	if !codeownersFound {
		missing = append(missing, "CODEOWNERS")
	}

	for _, file := range requiredFiles {
		if strings.EqualFold(file, "CODEOWNERS") {
			continue
		}
		exists, err := c.FileExists(ctx, file)
		if err != nil {
			return ReadinessResult{}, fmt.Errorf("check %s: %w", file, err)
		}
		if !exists {
			missing = append(missing, file)
		}
	}

	return ReadinessResult{
		Ready:   len(missing) == 0,
		Missing: missing,
	}, nil
}
