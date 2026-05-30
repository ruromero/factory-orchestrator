package harness

import (
	"context"
	"fmt"
	"log/slog"

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
		return "", err
	}
	redacted, events := sandbox.RedactSecrets(result)
	for _, e := range events {
		slog.Warn("credential redacted from tool response",
			"tool", name,
			"pattern", e.Pattern,
			"line", e.Line,
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
