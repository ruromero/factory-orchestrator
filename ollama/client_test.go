package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestChatPopulatesTokenCounts(t *testing.T) {
	resp := ChatResponse{
		Message:         Message{Role: "assistant", Content: "hello"},
		PromptEvalCount: 42,
		EvalCount:       18,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)

	got, err := c.Chat(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got.Message.Content != "hello" {
		t.Errorf("content = %q, want %q", got.Message.Content, "hello")
	}
	if got.PromptEvalCount != 42 {
		t.Errorf("PromptEvalCount = %d, want 42", got.PromptEvalCount)
	}
	if got.EvalCount != 18 {
		t.Errorf("EvalCount = %d, want 18", got.EvalCount)
	}
}

func TestChatWithToolsAccumulatesTokens(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")

		switch n {
		case 1:
			// First call: model requests a tool call
			json.NewEncoder(w).Encode(ChatResponse{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{Function: ToolFunction{Name: "test_tool", Arguments: map[string]any{"key": "val"}}},
					},
				},
				PromptEvalCount: 100,
				EvalCount:       30,
			})
		case 2:
			// Second call: model requests another tool call
			json.NewEncoder(w).Encode(ChatResponse{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{Function: ToolFunction{Name: "test_tool", Arguments: map[string]any{"key": "val2"}}},
					},
				},
				PromptEvalCount: 150,
				EvalCount:       40,
			})
		default:
			// Final call: model returns content
			json.NewEncoder(w).Encode(ChatResponse{
				Message:         Message{Role: "assistant", Content: "final answer"},
				PromptEvalCount: 200,
				EvalCount:       50,
			})
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)

	handler := &mockToolHandler{result: "tool output"}

	got, err := c.ChatWithTools(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hi"}},
		Tools: []Tool{
			{Type: "function", Function: ToolDef{Name: "test_tool", Description: "a test tool"}},
		},
	}, handler, 10)
	if err != nil {
		t.Fatalf("ChatWithTools: %v", err)
	}
	if got.Message.Content != "final answer" {
		t.Errorf("content = %q, want %q", got.Message.Content, "final answer")
	}

	// Token counts should be accumulated: 100+150+200 = 450, 30+40+50 = 120
	if got.PromptEvalCount != 450 {
		t.Errorf("accumulated PromptEvalCount = %d, want 450", got.PromptEvalCount)
	}
	if got.EvalCount != 120 {
		t.Errorf("accumulated EvalCount = %d, want 120", got.EvalCount)
	}
}

func TestChatWithToolsNoToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ChatResponse{
			Message:         Message{Role: "assistant", Content: "direct answer"},
			PromptEvalCount: 50,
			EvalCount:       25,
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)

	got, err := c.ChatWithTools(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, &mockToolHandler{}, 10)
	if err != nil {
		t.Fatalf("ChatWithTools: %v", err)
	}
	if got.PromptEvalCount != 50 {
		t.Errorf("PromptEvalCount = %d, want 50", got.PromptEvalCount)
	}
	if got.EvalCount != 25 {
		t.Errorf("EvalCount = %d, want 25", got.EvalCount)
	}
}

type mockToolHandler struct {
	result string
}

func (m *mockToolHandler) Execute(_ context.Context, name string, args map[string]any) (string, error) {
	return m.result, nil
}
