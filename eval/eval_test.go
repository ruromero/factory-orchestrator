package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseThreshold(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		tests := []struct {
			input     string
			wantPass  int
			wantTotal int
		}{
			{"8/10", 8, 10},
			{"1/1", 1, 1},
			{"0/5", 0, 5},
			{"10/10", 10, 10},
		}
		for _, tt := range tests {
			t.Run(tt.input, func(t *testing.T) {
				p, tot, err := ParseThreshold(tt.input)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if p != tt.wantPass {
					t.Errorf("passes = %d, want %d", p, tt.wantPass)
				}
				if tot != tt.wantTotal {
					t.Errorf("total = %d, want %d", tot, tt.wantTotal)
				}
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		tests := []string{
			"abc",
			"8/0",
			"",
			"8",
			"/10",
			"11/10",
			"-1/10",
		}
		for _, input := range tests {
			t.Run(input, func(t *testing.T) {
				_, _, err := ParseThreshold(input)
				if err == nil {
					t.Fatalf("expected error for %q, got nil", input)
				}
			})
		}
	})
}

func TestCheckAssertion(t *testing.T) {
	t.Run("outcome_equals", func(t *testing.T) {
		a := Assertion{Type: "outcome_equals", Value: "plan"}
		if !CheckAssertion(a, "plan: fix the nil pointer", nil) {
			t.Error("expected pass when output starts with value")
		}
		if !CheckAssertion(a, "outcome: plan produced", nil) {
			t.Error("expected pass when output contains value")
		}
		if CheckAssertion(a, "needs_info: missing details", nil) {
			t.Error("expected fail when output does not contain value")
		}
	})

	t.Run("output_contains", func(t *testing.T) {
		a := Assertion{Type: "output_contains", Value: "func NewParser"}
		if !CheckAssertion(a, "added func NewParser to parser.go", nil) {
			t.Error("expected pass")
		}
		if CheckAssertion(a, "added a new function", nil) {
			t.Error("expected fail")
		}
	})

	t.Run("output_not_contains", func(t *testing.T) {
		a := Assertion{Type: "output_not_contains", Value: "NEEDS_INFO"}
		if !CheckAssertion(a, "plan: everything clear", nil) {
			t.Error("expected pass when substring absent")
		}
		if CheckAssertion(a, "NEEDS_INFO: clarification needed", nil) {
			t.Error("expected fail when substring present")
		}
	})

	t.Run("file_count_gte", func(t *testing.T) {
		files := []FileState{{Path: "a.go"}, {Path: "b.go"}, {Path: "c.go"}}
		a := Assertion{Type: "file_count_gte", Value: "2"}
		if !CheckAssertion(a, "", files) {
			t.Error("expected pass: 3 >= 2")
		}
		a.Value = "5"
		if CheckAssertion(a, "", files) {
			t.Error("expected fail: 3 < 5")
		}
		a.Value = "bad"
		if CheckAssertion(a, "", files) {
			t.Error("expected fail for invalid value")
		}
	})

	t.Run("file_paths_include", func(t *testing.T) {
		files := []FileState{{Path: "parser/parser.go"}, {Path: "main.go"}}
		a := Assertion{Type: "file_paths_include", Value: "parser/parser.go"}
		if !CheckAssertion(a, "", files) {
			t.Error("expected pass")
		}
		a.Value = "missing.go"
		if CheckAssertion(a, "", files) {
			t.Error("expected fail")
		}
	})

	t.Run("severity_present", func(t *testing.T) {
		a := Assertion{Type: "severity_present", Value: "CRITICAL"}
		if !CheckAssertion(a, "Found issue [CRITICAL] nil dereference", nil) {
			t.Error("expected pass")
		}
		if CheckAssertion(a, "Found issue [MEDIUM] naming", nil) {
			t.Error("expected fail")
		}
	})

	t.Run("compiles_skipped", func(t *testing.T) {
		a := Assertion{Type: "compiles", Value: ""}
		if !CheckAssertion(a, "", nil) {
			t.Error("compiles should return true (skipped)")
		}
	})

	t.Run("tests_pass_skipped", func(t *testing.T) {
		a := Assertion{Type: "tests_pass", Value: ""}
		if !CheckAssertion(a, "", nil) {
			t.Error("tests_pass should return true (skipped)")
		}
	})

	t.Run("unknown_type", func(t *testing.T) {
		a := Assertion{Type: "nonexistent", Value: "x"}
		if CheckAssertion(a, "anything", nil) {
			t.Error("unknown type should return false")
		}
	})
}

func TestLoadTestCases(t *testing.T) {
	t.Run("loads_valid_files", func(t *testing.T) {
		dir := t.TempDir()

		tc := TestCase{
			Name:          "test-case-1",
			Phase:         "planner",
			Inputs:        map[string]string{"issue_title": "Fix bug"},
			Assertions:    []Assertion{{Type: "output_contains", Value: "fix"}},
			PassThreshold: "8/10",
		}
		data, err := json.Marshal(tc)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "case-001.json"), data, 0644); err != nil {
			t.Fatal(err)
		}

		// Non-JSON file should be ignored.
		if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0644); err != nil {
			t.Fatal(err)
		}

		cases, err := LoadTestCases(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cases) != 1 {
			t.Fatalf("got %d cases, want 1", len(cases))
		}
		if cases[0].Name != "test-case-1" {
			t.Errorf("name = %q, want %q", cases[0].Name, "test-case-1")
		}
	})

	t.Run("loads_subdirectories", func(t *testing.T) {
		dir := t.TempDir()
		subdir := filepath.Join(dir, "planner")
		if err := os.Mkdir(subdir, 0755); err != nil {
			t.Fatal(err)
		}

		tc := TestCase{
			Name:          "sub-case",
			Phase:         "planner",
			Inputs:        map[string]string{},
			Assertions:    []Assertion{},
			PassThreshold: "1/1",
		}
		data, err := json.Marshal(tc)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(subdir, "case.json"), data, 0644); err != nil {
			t.Fatal(err)
		}

		cases, err := LoadTestCases(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cases) != 1 {
			t.Fatalf("got %d cases, want 1", len(cases))
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{invalid"), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadTestCases(dir)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})

	t.Run("nonexistent_dir", func(t *testing.T) {
		_, err := LoadTestCases("/nonexistent/path/that/does/not/exist")
		if err == nil {
			t.Fatal("expected error for nonexistent directory")
		}
	})
}

func TestFormatReport(t *testing.T) {
	results := []RunResult{
		{
			Case:      "planner/case-001",
			Runs:      10,
			Passes:    10,
			Threshold: 8,
			TotalRuns: 10,
			Pass:      true,
		},
		{
			Case:      "coder/case-001",
			Runs:      10,
			Passes:    6,
			Threshold: 8,
			TotalRuns: 10,
			Pass:      false,
			Failures: []string{
				`Run 3: assertion failed: output_contains "func NewParser"`,
			},
		},
	}

	report := FormatReport(results)
	if report == "" {
		t.Fatal("report should not be empty")
	}
	if !contains(report, "Golden-Set Evaluation Report") {
		t.Error("missing report header")
	}
	if !contains(report, "PASS") {
		t.Error("missing PASS status")
	}
	if !contains(report, "FAIL") {
		t.Error("missing FAIL status")
	}
	if !contains(report, "1/2 passed") {
		t.Error("missing overall summary")
	}
	if !contains(report, `output_contains "func NewParser"`) {
		t.Error("missing failure detail")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && searchSubstring(s, substr))
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
