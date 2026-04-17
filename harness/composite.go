package harness

import (
	"context"
	"fmt"

	"github.com/ruromero/factory-orchestrator/ollama"
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
	return h.Execute(ctx, name, args)
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
