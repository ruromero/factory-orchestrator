package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateWithUsage(t *testing.T) {
	resp := generateResponse{
		Candidates: []struct {
			Content struct {
				Parts []part `json:"parts"`
			} `json:"content"`
		}{
			{Content: struct {
				Parts []part `json:"parts"`
			}{Parts: []part{{Text: "hello world"}}}},
		},
		UsageMetadata: struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		}{
			PromptTokenCount:     10,
			CandidatesTokenCount: 20,
			TotalTokenCount:      30,
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := &Client{
		apiKey:  "test-key",
		baseURL: srv.URL,
		http:    srv.Client(),
	}

	text, usage, err := c.GenerateWithUsage(context.Background(), "test-model", "test prompt")
	if err != nil {
		t.Fatalf("GenerateWithUsage: %v", err)
	}
	if text != "hello world" {
		t.Errorf("text = %q, want %q", text, "hello world")
	}
	if usage.PromptTokens != 10 {
		t.Errorf("PromptTokens = %d, want 10", usage.PromptTokens)
	}
	if usage.CompletionTokens != 20 {
		t.Errorf("CompletionTokens = %d, want 20", usage.CompletionTokens)
	}
}

func TestGenerateWithUsageZeroUsage(t *testing.T) {
	// Verify that missing usageMetadata results in zero values (not an error).
	resp := generateResponse{
		Candidates: []struct {
			Content struct {
				Parts []part `json:"parts"`
			} `json:"content"`
		}{
			{Content: struct {
				Parts []part `json:"parts"`
			}{Parts: []part{{Text: "no usage"}}}},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := &Client{
		apiKey:  "test-key",
		baseURL: srv.URL,
		http:    srv.Client(),
	}

	text, usage, err := c.GenerateWithUsage(context.Background(), "test-model", "test prompt")
	if err != nil {
		t.Fatalf("GenerateWithUsage: %v", err)
	}
	if text != "no usage" {
		t.Errorf("text = %q, want %q", text, "no usage")
	}
	if usage.PromptTokens != 0 {
		t.Errorf("PromptTokens = %d, want 0", usage.PromptTokens)
	}
	if usage.CompletionTokens != 0 {
		t.Errorf("CompletionTokens = %d, want 0", usage.CompletionTokens)
	}
}

func TestGenerateWithUsageEmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(generateResponse{})
	}))
	defer srv.Close()

	c := &Client{
		apiKey:  "test-key",
		baseURL: srv.URL,
		http:    srv.Client(),
	}

	_, _, err := c.GenerateWithUsage(context.Background(), "test-model", "test prompt")
	if err == nil {
		t.Fatal("expected error for empty response")
	}
}

func TestGenerateWithUsageHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer srv.Close()

	c := &Client{
		apiKey:  "test-key",
		baseURL: srv.URL,
		http:    srv.Client(),
	}

	_, _, err := c.GenerateWithUsage(context.Background(), "test-model", "test prompt")
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

func TestGenerateBackwardCompatible(t *testing.T) {
	resp := generateResponse{
		Candidates: []struct {
			Content struct {
				Parts []part `json:"parts"`
			} `json:"content"`
		}{
			{Content: struct {
				Parts []part `json:"parts"`
			}{Parts: []part{{Text: "compat test"}}}},
		},
		UsageMetadata: struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		}{
			PromptTokenCount:     5,
			CandidatesTokenCount: 15,
			TotalTokenCount:      20,
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := &Client{
		apiKey:  "test-key",
		baseURL: srv.URL,
		http:    srv.Client(),
	}

	// Generate should still work and just discard usage.
	text, err := c.Generate(context.Background(), "test-model", "test prompt")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if text != "compat test" {
		t.Errorf("text = %q, want %q", text, "compat test")
	}
}
