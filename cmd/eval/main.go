package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/ruromero/la-fabriquilla/eval"
)

func main() {
	dir := flag.String("dir", "tests/golden", "path to golden-set directory")
	runs := flag.Int("runs", 10, "number of runs per test case")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cases, err := eval.LoadTestCases(*dir)
	if err != nil {
		slog.Error("failed to load test cases", "dir", *dir, "error", err)
		os.Exit(1)
	}

	if len(cases) == 0 {
		slog.Warn("no test cases found", "dir", *dir)
		os.Exit(0)
	}

	slog.Info("loaded test cases", "count", len(cases), "runs_per_case", *runs)

	var results []eval.RunResult
	for _, tc := range cases {
		threshold, totalRuns, err := eval.ParseThreshold(tc.PassThreshold)
		if err != nil {
			slog.Error("invalid threshold", "case", tc.Name, "error", err)
			os.Exit(1)
		}

		// Structural validation: check assertions against a mock output.
		// Actual LLM execution will be wired in later.
		mockOutput := buildMockOutput(tc)
		mockFiles := buildMockFiles(tc)

		passes := 0
		var failures []string
		for run := 1; run <= *runs; run++ {
			allPassed := true
			for _, a := range tc.Assertions {
				if !eval.CheckAssertion(a, mockOutput, mockFiles) {
					allPassed = false
					failures = append(failures,
						fmt.Sprintf("Run %d: assertion failed: %s %q", run, a.Type, a.Value))
				}
			}
			if allPassed {
				passes++
			}
		}

		result := eval.RunResult{
			Case:      tc.Phase + "/" + tc.Name,
			Runs:      *runs,
			Passes:    passes,
			Threshold: threshold,
			TotalRuns: totalRuns,
			Pass:      passes >= threshold,
			Failures:  failures,
		}
		results = append(results, result)
	}

	fmt.Print(eval.FormatReport(results))

	for _, r := range results {
		if !r.Pass {
			os.Exit(1)
		}
	}
}

// buildMockOutput creates a synthetic output that satisfies the test
// case's assertions for structural validation. When LLM execution is
// wired in, this will be replaced by actual model output.
func buildMockOutput(tc eval.TestCase) string {
	var parts []string

	for _, a := range tc.Assertions {
		switch a.Type {
		case "outcome_equals":
			parts = append(parts, a.Value)
		case "output_contains":
			parts = append(parts, a.Value)
		case "severity_present":
			parts = append(parts, "["+a.Value+"]")
		}
	}

	return fmt.Sprintf("%s\n", joinParts(parts))
}

// buildMockFiles creates a synthetic file list that satisfies
// file-related assertions.
func buildMockFiles(tc eval.TestCase) []eval.FileState {
	var files []eval.FileState
	maxCount := 0

	for _, a := range tc.Assertions {
		switch a.Type {
		case "file_paths_include":
			files = append(files, eval.FileState{Path: a.Value})
		case "file_count_gte":
			n := 0
			fmt.Sscanf(a.Value, "%d", &n)
			if n > maxCount {
				maxCount = n
			}
		}
	}

	// Pad file list to satisfy file_count_gte.
	for len(files) < maxCount {
		files = append(files, eval.FileState{
			Path: fmt.Sprintf("mock/file_%d.go", len(files)),
		})
	}

	return files
}

func joinParts(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " "
		}
		result += p
	}
	return result
}
