// Package scanner hosts concrete rules.Scanner implementations for
// dev-tool artifact discovery (node_modules, Docker, Xcode, Python,
// Rust, Homebrew). Each scanner implements the rules.Scanner interface
// defined in internal/rules.
//
// Shared utilities:
//   - walker.go: bounded recursive directory walker
//   - exec.go: CLI command execution with graceful skip
package scanner

import (
	"os"
	"path/filepath"
)

// DefaultScanRoots returns common project directories that exist under home.
// Scanners that do recursive walks use these as starting points.
// Only falls back to home if no specific subdirectories exist, to avoid
// redundant traversal of child dirs that are already listed.
func DefaultScanRoots(home string) []string {
	specific := []string{
		filepath.Join(home, "Projects"),
		filepath.Join(home, "Developer"),
		filepath.Join(home, "src"),
		filepath.Join(home, "go", "src"),
		filepath.Join(home, "workspace"),
		filepath.Join(home, "code"),
	}
	roots := make([]string, 0, len(specific))
	for _, c := range specific {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			roots = append(roots, c)
		}
	}
	if len(roots) == 0 {
		roots = append(roots, home) // fallback only
	}
	return roots
}
