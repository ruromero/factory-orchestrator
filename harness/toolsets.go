package harness

import (
	"github.com/ruromero/factory-orchestrator/github"
	"github.com/ruromero/factory-orchestrator/mcp"
	"github.com/ruromero/factory-orchestrator/ollama"
)

var SerenaGatherAllowed = map[string]bool{
	"find_symbol":                    true,
	"find_referencing_symbols":       true,
	"find_referencing_code_snippets": true,
	"get_symbols_overview":           true,
	"read_file":                      true,
	"list_dir":                       true,
	"search_for_pattern":             true,
}

var SerenaCoderAllowed = map[string]bool{
	"find_symbol":                    true,
	"find_referencing_symbols":       true,
	"find_referencing_code_snippets": true,
	"get_symbols_overview":           true,
	"read_file":                      true,
	"list_dir":                       true,
	"search_for_pattern":             true,
	"replace_symbol_body":            true,
	"insert_before_symbol":           true,
	"insert_after_symbol":            true,
	"replace_content":                true,
}

func BuildGatherTools(rc *RepoContext, gh *github.Client, serena *mcp.Client) ([]ollama.Tool, ollama.ToolHandler) {
	contextHandler := NewContextToolHandler(rc, gh)
	contextTools := ContextTools()

	if serena == nil {
		return contextTools, contextHandler
	}

	serenaReadTools := FilterTools(serena.Tools(), SerenaGatherAllowed)

	composite := NewCompositeToolHandler()
	composite.Register(contextTools, contextHandler)
	composite.Register(serenaReadTools, serena)

	allTools := append(contextTools, serenaReadTools...)
	return allTools, composite
}

func BuildCoderTools(serena *mcp.Client) ([]ollama.Tool, ollama.ToolHandler) {
	if serena == nil {
		return nil, nil
	}
	tools := FilterTools(serena.Tools(), SerenaCoderAllowed)
	return tools, serena
}
