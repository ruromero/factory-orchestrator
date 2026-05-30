package sandbox

import (
	"strings"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantRedact  string
		wantPattern string
	}{
		{
			"private key",
			"-----BEGIN RSA PRIVATE KEY-----\nMIIE...",
			"[REDACTED:private key]",
			"private key",
		},
		{
			"API key assignment",
			`api_key = "sk-abc123def456ghi789"`,
			"[REDACTED:API key assignment]",
			"API key assignment",
		},
		{
			"password assignment",
			`password = "s3cret!"`,
			"[REDACTED:password assignment]",
			"password assignment",
		},
		{
			"bearer token",
			"Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.xxxxx",
			"[REDACTED:bearer token]",
			"bearer token",
		},
		{
			"IPv4 address",
			"192.168.1.100",
			"[REDACTED:IPv4 address]",
			"IPv4 address",
		},
		{
			"hostname pattern",
			"db.internal",
			"[REDACTED:hostname pattern]",
			"hostname pattern",
		},
		{
			"AWS access key",
			"AKIAIOSFODNN7EXAMPLE",
			"[REDACTED:AWS access key]",
			"AWS access key",
		},
		{
			"GitHub token",
			"ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij",
			"[REDACTED:GitHub token]",
			"GitHub token",
		},
		{
			"generic secret env",
			"GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxx",
			"[REDACTED:generic secret env]",
			"generic secret env",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, events := RedactSecrets(tt.input)
			if !strings.Contains(got, tt.wantRedact) {
				t.Errorf("RedactSecrets(%q) = %q, want to contain %q", tt.input, got, tt.wantRedact)
			}
			if len(events) == 0 {
				t.Fatalf("expected at least one redaction event for %q", tt.name)
			}
			foundPattern := false
			for _, e := range events {
				if e.Pattern == tt.wantPattern {
					foundPattern = true
					break
				}
			}
			if !foundPattern {
				t.Errorf("expected event with pattern %q, got %+v", tt.wantPattern, events)
			}
		})
	}
}

func TestRedactSecretsFalsePositives(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"env var reference", `apiKey := os.Getenv("API_KEY")`},
		{"comment mentioning API key", "// TODO: add API key validation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, events := RedactSecrets(tt.input)
			if got != tt.input {
				t.Errorf("RedactSecrets(%q) = %q, want no change", tt.input, got)
			}
			if len(events) != 0 {
				t.Errorf("expected no events, got %+v", events)
			}
		})
	}
}

func TestRedactSecretsMultiLine(t *testing.T) {
	input := "line1\npassword = \"hunter2!\"\nline3\nghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij\nline5"
	got, events := RedactSecrets(input)

	if strings.Contains(got, "hunter2") {
		t.Error("password was not redacted")
	}
	if strings.Contains(got, "ghp_ABCDEF") {
		t.Error("GitHub token was not redacted")
	}
	if !strings.Contains(got, "[REDACTED:password assignment]") {
		t.Error("missing password redaction marker")
	}
	if !strings.Contains(got, "[REDACTED:GitHub token]") {
		t.Error("missing GitHub token redaction marker")
	}

	// Verify line numbers
	lineMap := make(map[string]int)
	for _, e := range events {
		lineMap[e.Pattern] = e.Line
	}
	if lineMap["password assignment"] != 2 {
		t.Errorf("password line = %d, want 2", lineMap["password assignment"])
	}
	if lineMap["GitHub token"] != 4 {
		t.Errorf("GitHub token line = %d, want 4", lineMap["GitHub token"])
	}
}

func TestRedactSecretsCleanText(t *testing.T) {
	input := "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}"
	got, events := RedactSecrets(input)
	if got != input {
		t.Errorf("clean text was modified: %q", got)
	}
	if len(events) != 0 {
		t.Errorf("expected no events for clean text, got %+v", events)
	}
}
