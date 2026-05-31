package pipeline

import "fmt"

// CheckCostBudget returns an error if the pipeline has exceeded the token budget.
func CheckCostBudget(state *State, maxTokens int) error {
	if maxTokens <= 0 {
		return nil // no budget configured
	}
	total := state.TotalPromptTokens + state.TotalCompTokens
	if total > maxTokens {
		return fmt.Errorf("token budget exceeded: %d used, limit %d", total, maxTokens)
	}
	return nil
}

// CheckPRScope returns an error if the generated files exceed scope limits.
func CheckPRScope(files []FileState, maxFiles, maxLines int) error {
	if maxFiles > 0 && len(files) > maxFiles {
		return fmt.Errorf("too many files changed: %d, limit %d", len(files), maxFiles)
	}
	if maxLines > 0 {
		totalLines := 0
		for _, f := range files {
			totalLines += countLines(f.Content)
		}
		if totalLines > maxLines {
			return fmt.Errorf("PR too large: %d lines, limit %d", totalLines, maxLines)
		}
	}
	return nil
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := 1
	for i := range s {
		if s[i] == '\n' {
			n++
		}
	}
	return n
}
