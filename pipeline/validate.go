package pipeline

import (
	"fmt"
	"path/filepath"
	"strings"
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
