package harness

// PhaseContext holds the assembled context for an agent phase.
type PhaseContext struct {
	IssueTitle      string
	IssueBody       string
	ResearchContext string
	Plan            string
	Design          string
	Code            string
	ReviewFeedback  string
}
