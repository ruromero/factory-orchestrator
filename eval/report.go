package eval

import (
	"fmt"
	"strings"
)

// FormatReport produces a human-readable evaluation summary from the
// given run results.
func FormatReport(results []RunResult) string {
	var b strings.Builder
	b.WriteString("Golden-Set Evaluation Report\n")
	b.WriteString("============================\n\n")

	passed := 0
	for _, r := range results {
		status := "PASS"
		if !r.Pass {
			status = "FAIL"
		} else {
			passed++
		}
		b.WriteString(fmt.Sprintf("%-45s %s  (%d/%d, threshold %d/%d)\n",
			r.Case, status, r.Passes, r.Runs, r.Threshold, r.TotalRuns))

		for _, f := range r.Failures {
			b.WriteString(fmt.Sprintf("  - %s\n", f))
		}
	}

	b.WriteString(fmt.Sprintf("\nOverall: %d/%d passed\n", passed, len(results)))
	return b.String()
}
