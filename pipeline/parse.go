package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CoderOutputSchema is the JSON Schema for structured coder output.
// When passed as the Format field in an Ollama ChatRequest, the model
// is constrained to output valid JSON matching this schema.
var CoderOutputSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"files": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":     map[string]any{"type": "string"},
					"language": map[string]any{"type": "string"},
					"content":  map[string]any{"type": "string"},
				},
				"required": []string{"path", "content"},
			},
		},
	},
	"required": []string{"files"},
}

type coderOutput struct {
	Files []struct {
		Path     string `json:"path"`
		Language string `json:"language"`
		Content  string `json:"content"`
	} `json:"files"`
}

// ParseStructuredCodeOutput parses JSON structured coder output into a FileState slice.
func ParseStructuredCodeOutput(jsonOutput string) ([]FileState, error) {
	var out coderOutput
	if err := json.Unmarshal([]byte(jsonOutput), &out); err != nil {
		return nil, err
	}
	var files []FileState
	for _, f := range out.Files {
		files = append(files, FileState{Path: f.Path, Content: f.Content})
	}
	return files, nil
}

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
