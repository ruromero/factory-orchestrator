package agents

import (
	"context"
	"fmt"

	"github.com/ruromero/factory-orchestrator/ollama"
)

const gathererModel = "qwen2.5-coder:14b"

const gathererSystemPrompt = `You are a context gathering agent for a software development planner.

Given a GitHub issue, you must gather enough project context to produce an accurate implementation plan. You have access to project documentation, source files, and code navigation tools.

Strategy:
1. Read the document summaries provided to understand the project structure
2. Read the ARCHITECTURE.md sections relevant to this issue
3. Use code navigation tools (find definitions, references, symbol search) to locate relevant code
4. Read the actual source files that will need modification
5. Check existing patterns (e.g., how similar features are implemented)

Be thorough — read the source code, not just documentation. Use code navigation tools to efficiently locate relevant code instead of manually browsing directories. The planner needs to know:
- Exact file paths and module structure involved
- Existing function signatures, data models, and API patterns
- How similar features are currently implemented (look for examples)
- Configuration, environment variables, or infrastructure relevant to the change

When you have gathered enough context, produce a final response with the assembled context organized by relevance. Include specific file paths, function names, data structures, and code patterns you found. Do NOT produce a plan — just gather and organize the context.`

const maxGatherCalls = 25

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
