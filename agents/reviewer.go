package agents

import (
	"context"
	"fmt"

	"github.com/ruromero/factory-orchestrator/ollama"
)

const reviewerModel = "phi4:14b"

const correctnessPrompt = `You are a senior engineer performing an adversarial code review. Your job is to find problems, not approve code.

Evaluate the code against the design and plan. For each issue found, output:

[CRITICAL] Title — description, affected files/lines
[MEDIUM] Title — description, affected files/lines
[LOW] Title — description, affected files/lines

Check for:
- Logic errors and edge cases
- Missing error handling
- Test adequacy (do tests verify behavior or just assert code runs?)
- Test integrity (were existing tests weakened?)
- API contract violations
- Missing documentation updates

If no issues found, output: [PASS] No issues found.`

const securityPrompt = `You are a security engineer reviewing code for vulnerabilities.

Check for:
- Injection vulnerabilities (SQL, command, XSS)
- Authentication/authorization bypasses
- Data exposure (PII, credentials, tokens in logs)
- Unsafe deserialization
- Dependency vulnerabilities
- Missing input validation at system boundaries

For each issue, output:
[CRITICAL] Title — description, CWE if applicable, affected files/lines
[MEDIUM] Title — description
[LOW] Title — description

If no security issues found, output: [PASS] No security issues found.`

const intentPrompt = `You are reviewing a PR for intent alignment and scope.

Given the original issue, the plan, and the code changes, check:
- Does the PR implement what the issue requested?
- Is there scope creep (changes beyond what was planned)?
- Are all planned items addressed?
- If documentation was affected by code changes, was it updated?

Output:
[ALIGNED] — if the PR matches the intent
[SCOPE_CREEP] Title — description of out-of-scope changes
[MISSING] Title — planned items not implemented
[DOCS_OUTDATED] Title — documentation not updated to match code changes`

type ReviewResult struct {
	Correctness string
	Security    string
	Intent      string
}

func Review(ctx context.Context, ol *ollama.Client, code, design, plan, conventions string, tools []ollama.Tool, handler ollama.ToolHandler) (ReviewResult, error) {
	codeContext := fmt.Sprintf("## Plan\n\n%s\n\n## Design\n\n%s\n\n## Code\n\n%s", plan, design, code)
	if conventions != "" {
		codeContext += fmt.Sprintf("\n\n## Project Conventions\n\nVerify code follows these conventions:\n\n%s", conventions)
	}

	correctness, err := reviewWith(ctx, ol, correctnessPrompt, codeContext, tools, handler)
	if err != nil {
		return ReviewResult{}, fmt.Errorf("correctness review: %w", err)
	}

	security, err := reviewWith(ctx, ol, securityPrompt, codeContext, tools, handler)
	if err != nil {
		return ReviewResult{}, fmt.Errorf("security review: %w", err)
	}

	intent, err := reviewWith(ctx, ol, intentPrompt, codeContext, nil, nil)
	if err != nil {
		return ReviewResult{}, fmt.Errorf("intent review: %w", err)
	}

	return ReviewResult{
		Correctness: correctness,
		Security:    security,
		Intent:      intent,
	}, nil
}

func reviewWith(ctx context.Context, ol *ollama.Client, systemPrompt, userContent string, tools []ollama.Tool, handler ollama.ToolHandler) (string, error) {
	req := ollama.ChatRequest{
		Model: reviewerModel,
		Messages: []ollama.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userContent},
		},
		Tools:   tools,
		Options: &ollama.Options{Temperature: 0},
	}

	if len(tools) > 0 && handler != nil {
		resp, err := ol.ChatWithTools(ctx, req, handler, 10)
		if err != nil {
			return "", err
		}
		return resp.Message.Content, nil
	}

	resp, err := ol.Chat(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Message.Content, nil
}
