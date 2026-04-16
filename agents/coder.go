package agents

import (
	"context"
	"fmt"

	"github.com/ruromero/factory-orchestrator/ollama"
)

const coderModel = "qwen2.5-coder:14b"

const coderSystemPrompt = `You are a software developer. Given a technical design, implement the code changes.

For each file, output:

FILE: path/to/file
` + "```" + `language
<file contents>
` + "```" + `

Rules:
- Write complete file contents, not patches
- Include tests
- Update documentation if behavior changes
- Follow existing code style and conventions
- Do not add unnecessary dependencies`

func Code(ctx context.Context, ol *ollama.Client, design, researchContext string, tools []ollama.Tool, handler ollama.ToolHandler) (string, error) {
	userPrompt := fmt.Sprintf("## Technical Design\n\n%s", design)
	if researchContext != "" {
		userPrompt += fmt.Sprintf("\n\n## Research Context\n\n%s", researchContext)
	}

	req := ollama.ChatRequest{
		Model: coderModel,
		Messages: []ollama.Message{
			{Role: "system", Content: coderSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Tools:   tools,
		Options: &ollama.Options{Temperature: 0},
	}

	if len(tools) > 0 && handler != nil {
		resp, err := ol.ChatWithTools(ctx, req, handler, 20)
		if err != nil {
			return "", fmt.Errorf("coder chat with tools: %w", err)
		}
		return resp.Message.Content, nil
	}

	resp, err := ol.Chat(ctx, req)
	if err != nil {
		return "", fmt.Errorf("coder chat: %w", err)
	}
	return resp.Message.Content, nil
}
