package pipeline

import (
	"strings"
	"testing"
)

func TestValidateFiles(t *testing.T) {
	blocked := []string{"CODEOWNERS", "CONVENTIONS.md", ".github/workflows/*"}

	t.Run("valid files", func(t *testing.T) {
		files := []FileState{
			{Path: "cmd/main.go", Content: "package main"},
			{Path: "config/config.go", Content: "package config"},
		}
		if err := ValidateFiles(files, blocked); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("empty path", func(t *testing.T) {
		files := []FileState{{Path: "", Content: "x"}}
		err := ValidateFiles(files, blocked)
		if err == nil {
			t.Fatal("expected error for empty path")
		}
		if !strings.Contains(err.Error(), "empty") {
			t.Errorf("error = %q, want mention of empty", err.Error())
		}
	})

	t.Run("absolute path", func(t *testing.T) {
		files := []FileState{{Path: "/etc/passwd", Content: "x"}}
		err := ValidateFiles(files, blocked)
		if err == nil {
			t.Fatal("expected error for absolute path")
		}
		if !strings.Contains(err.Error(), "absolute") {
			t.Errorf("error = %q, want mention of absolute", err.Error())
		}
	})

	t.Run("path traversal", func(t *testing.T) {
		files := []FileState{{Path: "../../../etc/passwd", Content: "x"}}
		err := ValidateFiles(files, blocked)
		if err == nil {
			t.Fatal("expected error for path traversal")
		}
		if !strings.Contains(err.Error(), "traversal") {
			t.Errorf("error = %q, want mention of traversal", err.Error())
		}
	})

	t.Run("blocked CODEOWNERS", func(t *testing.T) {
		files := []FileState{{Path: "CODEOWNERS", Content: "x"}}
		err := ValidateFiles(files, blocked)
		if err == nil {
			t.Fatal("expected error for blocked path")
		}
		if !strings.Contains(err.Error(), "blocked") {
			t.Errorf("error = %q, want mention of blocked", err.Error())
		}
	})

	t.Run("blocked CONVENTIONS.md", func(t *testing.T) {
		files := []FileState{{Path: "CONVENTIONS.md", Content: "x"}}
		err := ValidateFiles(files, blocked)
		if err == nil {
			t.Fatal("expected error for blocked path")
		}
	})

	t.Run("empty file list", func(t *testing.T) {
		if err := ValidateFiles(nil, blocked); err != nil {
			t.Errorf("unexpected error for empty list: %v", err)
		}
	})
}
