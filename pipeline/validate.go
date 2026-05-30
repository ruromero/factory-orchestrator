package pipeline

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ruromero/la-fabriquilla/sandbox"
)

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
	patterns := sandbox.GetSensitivePatterns()
	for _, f := range files {
		for _, sp := range patterns {
			matches := sp.Pattern.FindAllStringIndex(f.Content, -1)
			for _, m := range matches {
				line := 1 + strings.Count(f.Content[:m[0]], "\n")
				matched := f.Content[m[0]:m[1]]
				if len(matched) > 80 {
					matched = matched[:80] + "..."
				}
				violations = append(violations, ContentViolation{
					File:    f.Path,
					Line:    line,
					Pattern: sp.Name,
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
