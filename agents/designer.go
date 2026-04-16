package agents

import (
	"context"
	"fmt"

	"github.com/ruromero/factory-orchestrator/ollama"
)

const designerModel = "qwen2.5-coder:14b"

const designerSystemPrompt = `You are a software architect. Given an implementation plan, produce a technical design document that includes:

1. API contracts (endpoints, request/response schemas)
2. Data models (structs, database schema changes)
3. Component boundaries and interfaces
4. File structure (new files to create, existing files to modify)
5. Dependencies and libraries needed

Output structured markdown. Do not write implementation code.`

func Design(ctx context.Context, ol *ollama.Client, plan, researchContext string) (string, error) {
	userPrompt := fmt.Sprintf("## Implementation Plan\n\n%s", plan)
	if researchContext != "" {
		userPrompt += fmt.Sprintf("\n\n## Research Context\n\n%s", researchContext)
	}

	resp, err := ol.Chat(ctx, ollama.ChatRequest{
		Model: designerModel,
		Messages: []ollama.Message{
			{Role: "system", Content: designerSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Options: &ollama.Options{Temperature: 0},
	})
	if err != nil {
		return "", fmt.Errorf("designer chat: %w", err)
	}

	return resp.Message.Content, nil
}
