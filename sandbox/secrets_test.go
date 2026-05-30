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
			"private key block",
			"-----BEGIN RSA PRIVATE KEY-----\nMIIE\n-----END RSA PRIVATE KEY-----",
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
			"connection string postgres",
			"postgresql://user:pass@host:5432/db",
			"[REDACTED:connection string]",
			"connection string",
		},
		{
			"connection string mongodb",
			"mongodb+srv://admin:s3cret@cluster.example.com/mydb",
			"[REDACTED:connection string]",
			"connection string",
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

func TestRedactSecretsPEMBlock(t *testing.T) {
	input := "before\n-----BEGIN RSA PRIVATE KEY-----\nMIIE...\nbase64data\n-----END RSA PRIVATE KEY-----\nafter"
	got, events := RedactSecrets(input)

	if strings.Contains(got, "MIIE") {
		t.Error("PEM body was not redacted")
	}
	if strings.Contains(got, "base64data") {
		t.Error("PEM base64 data was not redacted")
	}
	if !strings.Contains(got, "[REDACTED:private key]") {
		t.Error("missing private key redaction marker")
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Error("surrounding text was incorrectly removed")
	}

	foundKey := false
	for _, e := range events {
		if e.Pattern == "private key" {
			foundKey = true
		}
	}
	if !foundKey {
		t.Error("missing private key event")
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

	eventMap := make(map[string]RedactionEvent)
	for _, e := range events {
		eventMap[e.Pattern] = e
	}
	if e := eventMap["password assignment"]; e.Count != 1 || e.FirstLine != 2 {
		t.Errorf("password: count=%d firstLine=%d, want count=1 firstLine=2", e.Count, e.FirstLine)
	}
	if e := eventMap["GitHub token"]; e.Count != 1 || e.FirstLine != 4 {
		t.Errorf("GitHub token: count=%d firstLine=%d, want count=1 firstLine=4", e.Count, e.FirstLine)
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

func TestGetSensitivePatterns(t *testing.T) {
	patterns := GetSensitivePatterns()
	if len(patterns) != len(sensitivePatterns) {
		t.Fatalf("got %d patterns, want %d", len(patterns), len(sensitivePatterns))
	}
	// Modifying the copy must not affect the original.
	patterns[0].Name = "MODIFIED"
	if sensitivePatterns[0].Name == "MODIFIED" {
		t.Error("GetSensitivePatterns returned a reference, not a copy")
	}
}
