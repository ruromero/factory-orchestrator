package pipeline

import (
	"testing"
)

func TestParseCodeOutput(t *testing.T) {
	t.Run("single file", func(t *testing.T) {
		input := "FILE: main.go\n```go\npackage main\n```"
		files := ParseCodeOutput(input)
		if len(files) != 1 {
			t.Fatalf("got %d files, want 1", len(files))
		}
		if files[0].Path != "main.go" {
			t.Errorf("path = %q, want %q", files[0].Path, "main.go")
		}
		if files[0].Content != "package main" {
			t.Errorf("content = %q, want %q", files[0].Content, "package main")
		}
	})

	t.Run("multiple files", func(t *testing.T) {
		input := "FILE: a.go\n```go\npackage a\n```\nFILE: b.go\n```go\npackage b\n```"
		files := ParseCodeOutput(input)
		if len(files) != 2 {
			t.Fatalf("got %d files, want 2", len(files))
		}
		if files[0].Path != "a.go" {
			t.Errorf("files[0].path = %q, want %q", files[0].Path, "a.go")
		}
		if files[1].Path != "b.go" {
			t.Errorf("files[1].path = %q, want %q", files[1].Path, "b.go")
		}
	})

	t.Run("no files", func(t *testing.T) {
		files := ParseCodeOutput("just some text with no file markers")
		if len(files) != 0 {
			t.Fatalf("got %d files, want 0", len(files))
		}
	})

	t.Run("empty input", func(t *testing.T) {
		files := ParseCodeOutput("")
		if len(files) != 0 {
			t.Fatalf("got %d files, want 0", len(files))
		}
	})
}

func TestReviewNeedsIteration(t *testing.T) {
	t.Run("critical finding", func(t *testing.T) {
		if !ReviewNeedsIteration("[CRITICAL] bug found", "", "") {
			t.Error("expected true for CRITICAL finding")
		}
	})

	t.Run("medium finding", func(t *testing.T) {
		if !ReviewNeedsIteration("", "[MEDIUM] improvement needed", "") {
			t.Error("expected true for MEDIUM finding")
		}
	})

	t.Run("low finding only", func(t *testing.T) {
		if ReviewNeedsIteration("[LOW] minor style", "[LOW] minor naming", "[PASS]") {
			t.Error("expected false for LOW-only findings")
		}
	})

	t.Run("pass", func(t *testing.T) {
		if ReviewNeedsIteration("[PASS]", "[PASS]", "[PASS]") {
			t.Error("expected false for all PASS")
		}
	})
}

func TestFormatReviewFeedback(t *testing.T) {
	result := FormatReviewFeedback("correct", "secure", "aligned")
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	for _, want := range []string{"Correctness Review", "Security Review", "Intent Review", "correct", "secure", "aligned"} {
		if !contains(result, want) {
			t.Errorf("result missing %q", want)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
