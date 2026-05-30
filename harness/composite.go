package harness

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ruromero/la-fabriquilla/ollama"
	"github.com/ruromero/la-fabriquilla/sandbox"
)

type CompositeToolHandler struct {
	routes map[string]ollama.ToolHandler
}

func NewCompositeToolHandler() *CompositeToolHandler {
	return &CompositeToolHandler{routes: make(map[string]ollama.ToolHandler)}
}

func (c *CompositeToolHandler) Register(tools []ollama.Tool, handler ollama.ToolHandler) {
	for _, t := range tools {
		c.routes[t.Function.Name] = handler
	}
}

func (c *CompositeToolHandler) Execute(ctx context.Context, name string, args map[string]any) (string, error) {
	h, ok := c.routes[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	result, err := h.Execute(ctx, name, args)
	if err != nil {
		redactedMsg, _ := sandbox.RedactSecrets(err.Error())
		return "", fmt.Errorf("%s", redactedMsg)
	}
	redacted, events := sandbox.RedactSecrets(result)
	if len(events) > 0 {
		patterns := make([]string, len(events))
		for i, e := range events {
			patterns[i] = fmt.Sprintf("%s(%d)", e.Pattern, e.Count)
		}
		slog.Warn("credentials redacted from tool response",
			"tool", name,
			"patterns", strings.Join(patterns, ", "),
		)
	}
	return redacted, nil
}

func FilterTools(tools []ollama.Tool, allowed map[string]bool) []ollama.Tool {
	var filtered []ollama.Tool
	for _, t := range tools {
		if allowed[t.Function.Name] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}
