package agents

import (
	"context"
	"fmt"

	"github.com/ruromero/factory-orchestrator/ollama"
)

const gathererModel = "qwen2.5-coder:14b"

const gathererSystemPrompt = `You are a context gathering agent for a software development planner.

Given a GitHub issue, you must gather enough project context to produce an accurate implementation plan. You have access to project documentation and source files.

Start by reading the document summaries provided. Then use your tools to drill into the sections, files, and code that are relevant to this specific issue.

Gather context about:
- The modules, files, and data models affected by this issue
- Existing patterns and conventions relevant to the change
- API surface or UI components that would be impacted
- Any infrastructure or configuration considerations

When you have gathered enough context, produce a final response with the assembled context organized by relevance. Include specific file paths, function names, data structures, and patterns you found. Do NOT produce a plan — just gather and organize the context.`

const maxGatherCalls = 15

func GatherContext(ctx context.Context, ol *ollama.Client, issueTitle, issueBody, summaries string, tools []ollama.Tool, handler ollama.ToolHandler) (string, error) {
	userPrompt := fmt.Sprintf("## Issue: %s\n\n%s\n\n## Project Summaries\n\n%s", issueTitle, issueBody, summaries)

	resp, err := ol.ChatWithTools(ctx, ollama.ChatRequest{
		Model: gathererModel,
		Messages: []ollama.Message{
			{Role: "system", Content: gathererSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Tools:   tools,
		Options: &ollama.Options{Temperature: 0},
	}, handler, maxGatherCalls)
	if err != nil {
		return "", fmt.Errorf("gatherer: %w", err)
	}

	return resp.Message.Content, nil
}
