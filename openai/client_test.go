package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChatWithUsage(t *testing.T) {
	resp := chatResponse{
		Choices: []struct {
			Message Message `json:"message"`
		}{
			{Message: Message{Role: "assistant", Content: "test response"}},
		},
		Usage: struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		}{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-key")

	text, usage, err := c.ChatWithUsage(context.Background(), "test-model", "system prompt", "user prompt")
	if err != nil {
		t.Fatalf("ChatWithUsage: %v", err)
	}
	if text != "test response" {
		t.Errorf("text = %q, want %q", text, "test response")
	}
	if usage.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d, want 100", usage.PromptTokens)
	}
	if usage.CompletionTokens != 50 {
		t.Errorf("CompletionTokens = %d, want 50", usage.CompletionTokens)
	}
}

func TestChatWithUsageZeroUsage(t *testing.T) {
	resp := chatResponse{
		Choices: []struct {
			Message Message `json:"message"`
		}{
			{Message: Message{Role: "assistant", Content: "no usage"}},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-key")

	text, usage, err := c.ChatWithUsage(context.Background(), "test-model", "sys", "usr")
	if err != nil {
		t.Fatalf("ChatWithUsage: %v", err)
	}
	if text != "no usage" {
		t.Errorf("text = %q, want %q", text, "no usage")
	}
	if usage.PromptTokens != 0 || usage.CompletionTokens != 0 {
		t.Errorf("expected zero usage, got prompt=%d comp=%d", usage.PromptTokens, usage.CompletionTokens)
	}
}

func TestChatWithUsageEmptyChoices(t *testing.T) {
	resp := chatResponse{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-key")

	_, _, err := c.ChatWithUsage(context.Background(), "test-model", "sys", "usr")
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestChatWithUsageHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-key")

	_, _, err := c.ChatWithUsage(context.Background(), "test-model", "sys", "usr")
	if err == nil {
		t.Fatal("expected error for HTTP 400")
	}
}

func TestChatBackwardCompatible(t *testing.T) {
	resp := chatResponse{
		Choices: []struct {
			Message Message `json:"message"`
		}{
			{Message: Message{Role: "assistant", Content: "compat"}},
		},
		Usage: struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		}{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-key")

	text, err := c.Chat(context.Background(), "test-model", "sys", "usr")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if text != "compat" {
		t.Errorf("text = %q, want %q", text, "compat")
	}
}
