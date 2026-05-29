package pipeline

import (
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
		RepoName:        "factory-orchestrator",
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

func TestLoadStateNotFound(t *testing.T) {
	_, err := LoadState("/nonexistent/path/state.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
