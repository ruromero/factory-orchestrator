package traces

import (
	"encoding/json"
	"log/slog"
	"time"
)

type Trace struct {
	IssueNumber  int       `json:"issue_number"`
	Phase        string    `json:"phase"`
	Model        string    `json:"model"`
	PromptTokens int       `json:"prompt_tokens"`
	CompTokens   int       `json:"completion_tokens"`
	ToolCalls    int       `json:"tool_calls"`
	Duration     string    `json:"duration"`
	StartedAt    time.Time `json:"started_at"`
}

func Log(t Trace) {
	data, err := json.Marshal(t)
	if err != nil {
		slog.Error("failed to marshal trace", "error", err)
		return
	}
	slog.Info("agent_trace", "trace", string(data))
}
