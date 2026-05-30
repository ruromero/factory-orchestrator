package sandbox

import (
	"regexp"
	"strings"
)

// SensitivePattern pairs a human-readable name with a compiled regex
// for detecting credentials, secrets, and internal infrastructure details.
type SensitivePattern struct {
	Name    string
	Pattern *regexp.Regexp
}

// RedactionEvent records a single redaction for audit logging.
type RedactionEvent struct {
	Pattern string
	Line    int
}

// SensitivePatterns is the shared list of secret-detection patterns,
// used by both RedactSecrets (input path) and pipeline.ValidateContents (output path).
var SensitivePatterns = []SensitivePattern{
	{"private key", regexp.MustCompile(`-----BEGIN\s+(RSA|EC|DSA|OPENSSH|PGP)?\s*PRIVATE KEY-----`)},
	{"API key assignment", regexp.MustCompile(`(?i)(api[_-]?key|apikey|secret[_-]?key|access[_-]?key)\s*[:=]\s*["']?[A-Za-z0-9/+=_-]{16,}`)},
	{"password assignment", regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*["'][^"']{4,}["']`)},
	{"bearer token", regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]{20,}`)},
	{"IPv4 address", regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\b`)},
	{"hostname pattern", regexp.MustCompile(`(?i)\b[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.(?:internal|local|lan|home|corp|intranet)\b`)},
	{"AWS access key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"GitHub token", regexp.MustCompile(`ghp_[A-Za-z0-9]{36}`)},
	{"generic secret env", regexp.MustCompile(`(?i)(GITHUB_TOKEN|GEMINI_API_KEY|PLANNER_API_KEY|DEEPSEEK_API_KEY)\s*=\s*[^\s$]{8,}`)},
}

// RedactSecrets scans text for credentials and replaces matches with
// [REDACTED:<pattern>]. Returns the redacted text and a list of
// redaction events for audit logging.
func RedactSecrets(text string) (string, []RedactionEvent) {
	var events []RedactionEvent
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		for _, sp := range SensitivePatterns {
			matches := sp.Pattern.FindAllStringIndex(line, -1)
			if len(matches) > 0 {
				line = sp.Pattern.ReplaceAllString(line, "[REDACTED:"+sp.Name+"]")
				for range matches {
					events = append(events, RedactionEvent{
						Pattern: sp.Name,
						Line:    i + 1,
					})
				}
			}
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n"), events
}
