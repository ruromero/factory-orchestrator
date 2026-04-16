package agents

import (
	"context"
	"fmt"

	"github.com/ruromero/factory-orchestrator/gemini"
)

const researchModel = "gemini-2.5-flash"

const researchPrompt = `You are a research assistant for a software development team. Given a GitHub issue, research and gather relevant context that will help a developer implement it.

Focus on:
1. Relevant API documentation, library usage, and examples
2. Known patterns or solutions for this type of problem
3. Potential pitfalls or edge cases to watch out for
4. Any security considerations

Be concise and actionable. Structure your output as a reference document that a developer can use alongside the issue description.

## Issue: %s

%s`

func Research(ctx context.Context, gem *gemini.Client, issueTitle, issueBody string) (string, error) {
	if gem == nil {
		return "", nil
	}

	prompt := fmt.Sprintf(researchPrompt, issueTitle, issueBody)
	result, err := gem.Generate(ctx, researchModel, prompt)
	if err != nil {
		return "", fmt.Errorf("research: %w", err)
	}
	return result, nil
}
