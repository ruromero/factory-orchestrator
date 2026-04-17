package agents

import (
	"context"
	"fmt"

	"github.com/ruromero/factory-orchestrator/ollama"
)

const gathererModel = "qwen3:14b"

const gathererSystemPrompt = `You are a context gathering agent for a software development planner.

Given a GitHub issue, you must gather enough project context to produce an accurate implementation plan. You have access to project documentation, source files, and code navigation tools.

Strategy:
1. Read the document summaries provided to understand the project structure
2. Read the ARCHITECTURE.md sections relevant to this issue
3. Search for existing implementations: extract key terms from the issue (feature names, entity names, actions) and search the codebase for them. Many issues describe changes to features that ALREADY EXIST — you must find them.
4. Use code navigation tools (find definitions, references, symbol search) to locate relevant code
5. Read the actual source files that will need modification
6. Trace the full data path: if the issue involves an entity (e.g., "borrowing", "user", "email"), find every layer — database schema/queries, backend handlers/services, API routes, and frontend components that touch it

Be thorough — read the source code, not just documentation. Use code navigation tools to efficiently locate relevant code instead of manually browsing directories. The planner needs to know:
- What already exists: endpoints, handlers, database tables, UI components related to the issue
- What does NOT exist yet: the gap between current state and what the issue asks for
- Exact file paths, function signatures, data models, and API patterns
- How the existing system works (e.g., how emails are sent, how templates are stored, how i18n works)
- Configuration, environment variables, or infrastructure relevant to the change

CRITICAL: Do not assume something needs to be built from scratch. Always search for existing code first. If the issue mentions "extend borrowing", search for "extend" in the codebase. If it mentions "email notification", find the existing email system and understand how it works.

When you have gathered enough context, produce a final response with the assembled context organized into:
1. EXISTING CODE: What already exists that is relevant (with file paths, function names, schemas)
2. GAPS: What is missing or needs to change
3. PATTERNS: How similar features are currently implemented

Do NOT produce a plan — just gather and organize the context.`

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
