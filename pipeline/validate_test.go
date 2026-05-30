package pipeline

import (
	"strings"
	"testing"
)

func TestValidateContents(t *testing.T) {
	t.Run("private key", func(t *testing.T) {
		files := []FileState{{Path: "config.go", Content: "-----BEGIN RSA PRIVATE KEY-----\nMIIE..."}}
		v := ValidateContents(files)
		if len(v) == 0 {
			t.Fatal("expected violation for private key")
		}
		if v[0].Pattern != "private key" {
			t.Errorf("pattern = %q, want %q", v[0].Pattern, "private key")
		}
	})

	t.Run("API key assignment", func(t *testing.T) {
		files := []FileState{{Path: "main.go", Content: `apiKey = "sk-1234567890abcdef1234567890abcdef"`}}
		v := ValidateContents(files)
		if len(v) == 0 {
			t.Fatal("expected violation for API key")
		}
	})

	t.Run("password in config", func(t *testing.T) {
		files := []FileState{{Path: "config.yaml", Content: `password: "hunter2secret"`}}
		v := ValidateContents(files)
		if len(v) == 0 {
			t.Fatal("expected violation for password")
		}
	})

	t.Run("IPv4 address", func(t *testing.T) {
		files := []FileState{{Path: "deploy.go", Content: `host := "192.168.1.100"`}}
		v := ValidateContents(files)
		if len(v) == 0 {
			t.Fatal("expected violation for IP address")
		}
	})

	t.Run("internal hostname", func(t *testing.T) {
		files := []FileState{{Path: "config.go", Content: `url := "http://myserver.internal:8080"`}}
		v := ValidateContents(files)
		if len(v) == 0 {
			t.Fatal("expected violation for internal hostname")
		}
	})

	t.Run("GitHub token", func(t *testing.T) {
		files := []FileState{{Path: "main.go", Content: `token := "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij"`}}
		v := ValidateContents(files)
		if len(v) == 0 {
			t.Fatal("expected violation for GitHub token")
		}
	})

	t.Run("AWS access key", func(t *testing.T) {
		files := []FileState{{Path: "aws.go", Content: `key := "AKIAIOSFODNN7EXAMPLE"`}}
		v := ValidateContents(files)
		if len(v) == 0 {
			t.Fatal("expected violation for AWS key")
		}
	})

	t.Run("env var with value", func(t *testing.T) {
		files := []FileState{{Path: ".env", Content: `GEMINI_API_KEY=AIzaSyB1234567890abcdef`}}
		v := ValidateContents(files)
		if len(v) == 0 {
			t.Fatal("expected violation for env var secret")
		}
	})

	t.Run("clean code", func(t *testing.T) {
		files := []FileState{{Path: "main.go", Content: `package main

func main() {
	fmt.Println("hello")
}`}}
		v := ValidateContents(files)
		if len(v) != 0 {
			t.Errorf("expected no violations, got %d: %+v", len(v), v)
		}
	})

	t.Run("env var reference is safe", func(t *testing.T) {
		files := []FileState{{Path: "config.go", Content: `key := os.Getenv("GEMINI_API_KEY")`}}
		v := ValidateContents(files)
		if len(v) != 0 {
			t.Errorf("expected no violations for env var reference, got %d: %+v", len(v), v)
		}
	})

	t.Run("line number tracking", func(t *testing.T) {
		files := []FileState{{Path: "config.go", Content: "line1\nline2\npassword: \"secret1234\"\nline4"}}
		v := ValidateContents(files)
		if len(v) == 0 {
			t.Fatal("expected violation")
		}
		if v[0].Line != 3 {
			t.Errorf("line = %d, want 3", v[0].Line)
		}
	})
}

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
