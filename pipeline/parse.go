package pipeline

import (
	"fmt"
	"strings"
)

func ParseCodeOutput(output string) []FileState {
	var files []FileState
	lines := strings.Split(output, "\n")
	var currentPath string
	var content strings.Builder
	inBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "FILE:") {
			currentPath = strings.TrimSpace(strings.TrimPrefix(trimmed, "FILE:"))
			continue
		}
		if !inBlock && strings.HasPrefix(trimmed, "```") && currentPath != "" {
			inBlock = true
			content.Reset()
			continue
		}
		if inBlock && trimmed == "```" {
			files = append(files, FileState{
				Path:    currentPath,
				Content: strings.TrimRight(content.String(), "\n"),
			})
			currentPath = ""
			inBlock = false
			continue
		}
		if inBlock {
			content.WriteString(line)
			content.WriteString("\n")
		}
	}
	return files
}

func ReviewNeedsIteration(correctness, security, intent string) bool {
	combined := correctness + security + intent
	return strings.Contains(combined, "[CRITICAL]") || strings.Contains(combined, "[MEDIUM]")
}

func FormatReviewFeedback(correctness, security, intent string) string {
	return fmt.Sprintf("## Correctness Review\n\n%s\n\n## Security Review\n\n%s\n\n## Intent Review\n\n%s",
		correctness, security, intent)
}
