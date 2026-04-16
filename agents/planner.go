package agents

import (
	"context"
	"fmt"

	"github.com/ruromero/factory-orchestrator/ollama"
)

const plannerModel = "deepseek-r1:14b"

const plannerSystemPrompt = `You are a software project planner. Given a GitHub issue, you must do ONE of the following:

1. If the issue has enough information and is small enough for a single PR:
   Produce a structured implementation plan with numbered steps, risks, and dependencies.
   Start your response with "PLAN:" on the first line.

2. If the issue lacks critical information needed to proceed:
   Explain what information is missing and what questions need answering.
   Start your response with "NEEDS_INFO:" on the first line.

3. If the issue is too complex for a single PR:
   Decompose it into smaller sub-issues, each independently implementable.
   List each sub-issue with a title, description, and dependency order.
   Start your response with "DECOMPOSE:" on the first line.

Be specific and actionable. Do not generate code.`

type PlanResult struct {
	Outcome  string // "plan", "needs_info", or "decompose"
	Content  string
}

func Plan(ctx context.Context, ol *ollama.Client, issueTitle, issueBody, researchContext string) (PlanResult, error) {
	userPrompt := fmt.Sprintf("## Issue: %s\n\n%s", issueTitle, issueBody)
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
