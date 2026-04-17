package agents

import (
	"context"
	"fmt"

	"github.com/ruromero/factory-orchestrator/ollama"
)

const plannerModel = "deepseek-r1:14b"

const plannerSystemPrompt = `You are a software project planner. You are given a GitHub issue along with relevant project context that was gathered specifically for this issue.

You must do ONE of the following:

1. If the issue has enough information and is small enough for a single PR:
   Produce a structured implementation plan with numbered steps, risks, and dependencies.
   Reference specific files, modules, and patterns from the project context.
   Start your response with "PLAN:" on the first line.

2. If the issue lacks critical information that CANNOT be discovered from the codebase:
   Only ask about business decisions, external credentials, or domain-specific requirements
   that no amount of code reading would answer. Never ask about implementation details
   like which framework, library, or templating engine is used — those are in the code.
   Start your response with "NEEDS_INFO:" on the first line.

3. If the issue is too complex for a single PR:
   Decompose it into smaller sub-issues, each independently implementable.
   List each sub-issue with a title, description, and dependency order.
   Start your response with "DECOMPOSE:" on the first line.

Prefer producing a plan with reasonable assumptions over asking for information.
Implementation details that the coder can discover from the source code should not
block planning. Only use NEEDS_INFO for truly missing business context.

Be specific and actionable. Reference actual file paths and existing code patterns. Do not generate code.`

type PlanResult struct {
	Outcome string
	Content string
}

func Plan(ctx context.Context, ol *ollama.Client, issueTitle, issueBody, researchContext, gatheredContext, conventions, commentHistory string) (PlanResult, error) {
	userPrompt := fmt.Sprintf("## Issue: %s\n\n%s", issueTitle, issueBody)
	if commentHistory != "" {
		userPrompt += fmt.Sprintf("\n\n## Discussion\n\nPrevious comments on this issue (may contain answers to earlier questions):\n\n%s", commentHistory)
	}
	if gatheredContext != "" {
		userPrompt += fmt.Sprintf("\n\n## Project Context\n\n%s", gatheredContext)
	}
	if conventions != "" {
		userPrompt += fmt.Sprintf("\n\n## Project Conventions\n\nFollow these conventions in your plan:\n\n%s", conventions)
	}
	if researchContext != "" {
		userPrompt += fmt.Sprintf("\n\n## Research Context\n\n%s", researchContext)
	}

	resp, err := ol.Chat(ctx, ollama.ChatRequest{
		Model: plannerModel,
		Messages: []ollama.Message{
			{Role: "system", Content: plannerSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Options: &ollama.Options{Temperature: 0},
	})
	if err != nil {
		return PlanResult{}, fmt.Errorf("planner chat: %w", err)
	}

	content := resp.Message.Content
	outcome := "plan"
	if len(content) > 11 && content[:11] == "NEEDS_INFO:" {
		outcome = "needs_info"
	} else if len(content) > 11 && content[:11] == "DECOMPOSE:" {
		outcome = "decompose"
	}

	return PlanResult{Outcome: outcome, Content: content}, nil
}
