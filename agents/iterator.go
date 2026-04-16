package agents

import (
	"context"
	"fmt"

	"github.com/ruromero/factory-orchestrator/ollama"
)

const iteratorSystemPrompt = `You are a software developer applying review feedback to code.

Given the current code and a structured review with severity levels, apply fixes:
1. Address all [CRITICAL] issues first
2. Then address [MEDIUM] issues
3. [LOW] issues are optional

For each file changed, output the complete updated file:

FILE: path/to/file
` + "```" + `language
<complete file contents>
` + "```" + `

Do not introduce new features. Only fix the issues raised in the review.`

func Iterate(ctx context.Context, ol *ollama.Client, code, reviewFeedback string, tools []ollama.Tool, handler ollama.ToolHandler) (string, error) {
	userPrompt := fmt.Sprintf("## Current Code\n\n%s\n\n## Review Feedback\n\n%s", code, reviewFeedback)

	req := ollama.ChatRequest{
		Model: coderModel,
		Messages: []ollama.Message{
			{Role: "system", Content: iteratorSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Tools:   tools,
		Options: &ollama.Options{Temperature: 0},
	}

	if len(tools) > 0 && handler != nil {
		resp, err := ol.ChatWithTools(ctx, req, handler, 20)
		if err != nil {
			return "", fmt.Errorf("iterate with tools: %w", err)
		}
		return resp.Message.Content, nil
	}

	resp, err := ol.Chat(ctx, req)
	if err != nil {
		return "", fmt.Errorf("iterate chat: %w", err)
	}
	return resp.Message.Content, nil
}
