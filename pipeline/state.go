package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type State struct {
	RepoOwner   string `json:"repo_owner"`
	RepoName    string `json:"repo_name"`
	IssueNumber int    `json:"issue_number"`

	Phase     string `json:"phase"`
	Iteration int    `json:"iteration"`

	IssueTitle     string `json:"issue_title"`
	IssueBody      string `json:"issue_body"`
	CommentHistory string `json:"comment_history,omitempty"`
	Summaries      string `json:"summaries"`
	Conventions    string `json:"conventions"`

	GatheredContext string       `json:"gathered_context,omitempty"`
	ResearchContext string       `json:"research_context,omitempty"`
	PlanOutcome     string       `json:"plan_outcome,omitempty"`
	PlanContent     string       `json:"plan_content,omitempty"`
	Design          string       `json:"design,omitempty"`
	Code            string       `json:"code,omitempty"`
	Review          *ReviewState `json:"review,omitempty"`
	Files           []FileState  `json:"files,omitempty"`

	PRNumber int    `json:"pr_number,omitempty"`
	PRBranch string `json:"pr_branch,omitempty"`

	CloneDir string `json:"clone_dir,omitempty"`

	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ReviewState struct {
	Correctness string `json:"correctness"`
	Security    string `json:"security"`
	Intent      string `json:"intent"`
}

type FileState struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func LoadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	return &s, nil
}

func SaveState(path string, s *State) error {
	s.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}
