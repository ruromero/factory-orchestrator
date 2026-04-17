package agents

import (
	"context"
	"fmt"

	"github.com/ruromero/factory-orchestrator/gemini"
)

const researchModel = "gemini-2.5-flash"

const researchPrompt = `You are a research assistant for a software development team. Given a GitHub issue and the project's tech stack, research and gather relevant context that will help a developer implement it.

IMPORTANT: Only provide examples and patterns that match the project's actual tech stack described below. Do NOT suggest libraries, frameworks, or patterns from other languages or ecosystems.

Focus on:
1. Relevant API documentation, library usage, and examples for the specific tech stack
2. Known patterns or solutions for this type of problem in this tech stack
3. Potential pitfalls or edge cases to watch out for
4. Any security considerations

Be concise and actionable. Structure your output as a reference document that a developer can use alongside the issue description.

%s

## Issue: %s

%s`

func Research(ctx context.Context, gem *gemini.Client, issueTitle, issueBody, techContext string) (string, error) {
	if gem == nil {
		return "", nil
	}

	stackSection := ""
	if techContext != "" {
		stackSection = fmt.Sprintf("## Project Tech Stack\n\n%s", techContext)
	}

	prompt := fmt.Sprintf(researchPrompt, stackSection, issueTitle, issueBody)
	result, err := gem.Generate(ctx, researchModel, prompt)
	if err != nil {
		return "", fmt.Errorf("research: %w", err)
	}
	return result, nil
}
