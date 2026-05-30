package pipeline

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var sensitivePatterns = []struct {
	name    string
	pattern *regexp.Regexp
}{
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

// ValidateFiles checks file paths for traversal attacks and blocked patterns.
// NOTE: filepath.Match does not match across '/' separators, so multi-level
// patterns like ".github/workflows/*.yml" only work for single-directory depth.
// For v1 this is sufficient since blocked patterns are simple top-level globs.
func ValidateFiles(files []FileState, blockedPatterns []string) error {
	for _, f := range files {
		if f.Path == "" {
			return fmt.Errorf("empty file path")
		}
		if filepath.IsAbs(f.Path) {
			return fmt.Errorf("absolute path not allowed: %s", f.Path)
		}
		if strings.Contains(f.Path, "..") {
			return fmt.Errorf("path traversal not allowed: %s", f.Path)
		}
		for _, pattern := range blockedPatterns {
			matched, err := filepath.Match(pattern, f.Path)
			if err != nil {
				continue
			}
			if matched {
				return fmt.Errorf("blocked path: %s (matches %s)", f.Path, pattern)
			}
			if strings.Contains(pattern, "/") {
				dir := filepath.Dir(f.Path)
				if matched, _ := filepath.Match(pattern, dir+"/"+filepath.Base(f.Path)); matched {
					return fmt.Errorf("blocked path: %s (matches %s)", f.Path, pattern)
				}
			}
		}
	}
	return nil
}

// ValidateContents scans file contents for secrets, credentials, private
// keys, IP addresses, and internal hostnames. Returns all violations found.
func ValidateContents(files []FileState) []ContentViolation {
	var violations []ContentViolation
	for _, f := range files {
		for _, sp := range sensitivePatterns {
			matches := sp.pattern.FindAllStringIndex(f.Content, -1)
			for _, m := range matches {
				line := 1 + strings.Count(f.Content[:m[0]], "\n")
				matched := f.Content[m[0]:m[1]]
				if len(matched) > 80 {
					matched = matched[:80] + "..."
				}
				violations = append(violations, ContentViolation{
					File:    f.Path,
					Line:    line,
					Pattern: sp.name,
					Match:   matched,
				})
			}
		}
	}
	return violations
}

type ContentViolation struct {
	File    string
	Line    int
	Pattern string
	Match   string
}
