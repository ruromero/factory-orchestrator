package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	original := &State{
		RepoOwner:       "ruromero",
		RepoName:        "la-fabriquilla",
		IssueNumber:     42,
		Phase:           "plan",
		Iteration:       1,
		IssueTitle:      "Add rate limiting",
		IssueBody:       "We need rate limiting on all endpoints",
		GatheredContext: "found middleware package",
		PlanOutcome:     "plan",
		PlanContent:     "Add middleware with token bucket",
		Review: &ReviewState{
			Correctness: "[PASS]",
			Security:    "[PASS]",
			Intent:      "[PASS]",
		},
		Files: []FileState{
			{Path: "middleware/ratelimit.go", Content: "package middleware"},
		},
		StartedAt: time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC),
	}

	if err := SaveState(path, original); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state file not created: %v", err)
	}

	loaded, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	if loaded.RepoOwner != original.RepoOwner {
		t.Errorf("RepoOwner = %q, want %q", loaded.RepoOwner, original.RepoOwner)
	}
	if loaded.IssueNumber != original.IssueNumber {
		t.Errorf("IssueNumber = %d, want %d", loaded.IssueNumber, original.IssueNumber)
	}
	if loaded.Phase != original.Phase {
		t.Errorf("Phase = %q, want %q", loaded.Phase, original.Phase)
	}
	if loaded.PlanOutcome != original.PlanOutcome {
		t.Errorf("PlanOutcome = %q, want %q", loaded.PlanOutcome, original.PlanOutcome)
	}
	if loaded.Review == nil {
		t.Fatal("Review is nil after round-trip")
	}
	if loaded.Review.Correctness != original.Review.Correctness {
		t.Errorf("Review.Correctness = %q, want %q", loaded.Review.Correctness, original.Review.Correctness)
	}
	if len(loaded.Files) != 1 {
		t.Fatalf("Files count = %d, want 1", len(loaded.Files))
	}
	if loaded.Files[0].Path != "middleware/ratelimit.go" {
		t.Errorf("Files[0].Path = %q, want %q", loaded.Files[0].Path, "middleware/ratelimit.go")
	}
	if loaded.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set by SaveState")
	}
}

func TestPhaseTokensRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	original := &State{
		RepoOwner:   "ruromero",
		RepoName:    "la-fabriquilla",
		IssueNumber: 50,
		Phase:       "code-done",
		PhaseTokens: []TokenUsage{
			{
				Phase:            "researcher",
				Model:            "gemini-2.5-flash",
				PromptTokens:     1000,
				CompletionTokens: 500,
				WallTimeSeconds:  2.5,
			},
			{
				Phase:            "gatherer",
				Model:            "qwen3:14b",
				PromptTokens:     2000,
				CompletionTokens: 800,
				WallTimeSeconds:  15.3,
			},
			{
				Phase:            "coder",
				Model:            "qwen3:14b",
				PromptTokens:     3000,
				CompletionTokens: 1200,
				WallTimeSeconds:  30.0,
			},
		},
		StartedAt: time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC),
	}

	if err := SaveState(path, original); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	loaded, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	if len(loaded.PhaseTokens) != 3 {
		t.Fatalf("PhaseTokens count = %d, want 3", len(loaded.PhaseTokens))
	}

	for i, want := range original.PhaseTokens {
		got := loaded.PhaseTokens[i]
		if got.Phase != want.Phase {
			t.Errorf("[%d] Phase = %q, want %q", i, got.Phase, want.Phase)
		}
		if got.Model != want.Model {
			t.Errorf("[%d] Model = %q, want %q", i, got.Model, want.Model)
		}
		if got.PromptTokens != want.PromptTokens {
			t.Errorf("[%d] PromptTokens = %d, want %d", i, got.PromptTokens, want.PromptTokens)
		}
		if got.CompletionTokens != want.CompletionTokens {
			t.Errorf("[%d] CompletionTokens = %d, want %d", i, got.CompletionTokens, want.CompletionTokens)
		}
		if got.WallTimeSeconds != want.WallTimeSeconds {
			t.Errorf("[%d] WallTimeSeconds = %f, want %f", i, got.WallTimeSeconds, want.WallTimeSeconds)
		}
	}
}

func TestPhaseTokensOmittedWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	original := &State{
		RepoOwner:   "ruromero",
		RepoName:    "la-fabriquilla",
		IssueNumber: 50,
		Phase:       "plan",
		StartedAt:   time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC),
	}

	if err := SaveState(path, original); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// phase_tokens should not appear in JSON when empty (omitempty)
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal raw: %v", err)
	}
	if _, exists := raw["phase_tokens"]; exists {
		t.Error("phase_tokens should be omitted when empty")
	}
}

func TestLoadStateNotFound(t *testing.T) {
	_, err := LoadState("/nonexistent/path/state.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
