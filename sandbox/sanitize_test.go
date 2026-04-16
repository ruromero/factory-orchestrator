package sandbox

import "testing"

func TestSanitizeInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"clean text", "hello world", "hello world"},
		{"preserves newlines", "line1\nline2", "line1\nline2"},
		{"strips zero-width space", "hello\u200Bworld", "helloworld"},
		{"strips zero-width joiner", "hello\u200Dworld", "helloworld"},
		{"strips BOM", "\uFEFFhello", "hello"},
		{"strips bidi override", "hello\u202Aworld", "helloworld"},
		{"strips bidi isolate", "hello\u2066world", "helloworld"},
		{"strips tag characters", "hello\U000E0041world", "helloworld"},
		{"preserves tabs", "hello\tworld", "hello\tworld"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeInput(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeInput(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
