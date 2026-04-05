package safety

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Ported from clmm-clean-my-mac-cli: src/utils/fs.ts
var protectedPaths = []string{
	"/System",
	"/usr",
	"/bin",
	"/sbin",
	"/etc",
	"/var/log",
	"/var/db",
	"/var/root",
	"/private/var/db",
	"/private/var/root",
	"/private/var/log",
	"/Library/Apple",
	"/Applications/Utilities",
}

// Ported from clmm-clean-my-mac-cli: src/utils/fs.ts
var allowedPaths = []string{
	"/tmp",
	"/private/tmp",
	"/var/tmp",
	"/private/var/tmp",
	"/var/folders",
	"/private/var/folders",
}

// PRD never-touch list additions (user data).
var homeNeverTouch = []string{
	"Documents",
	"Desktop",
	"Pictures",
	"Music",
	"Movies",
	"Downloads",
	".ssh",
	".gnupg",
	".gitconfig",
	".zshrc",
	".bashrc",
}

func HasTraversalPattern(p string) bool {
	if p == "" {
		return false
	}
	// Check elements exactly equal to ".." (avoid false positives like "..foo").
	for _, part := range strings.Split(filepath.ToSlash(p), "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func IsProtectedPath(absPath string) bool {
	p := filepath.Clean(absPath)

	for _, a := range allowedPaths {
		if hasPathPrefix(p, a) {
			return false
		}
	}
	for _, pr := range protectedPaths {
		if hasPathPrefix(p, pr) {
			return true
		}
	}

	home, err := os.UserHomeDir()
	if err == nil {
		for _, rel := range homeNeverTouch {
			if hasPathPrefix(p, filepath.Join(home, rel)) {
				return true
			}
		}
		// selective: ~/.config only safe subdirs should be touched; protect entire dir by default.
		if hasPathPrefix(p, filepath.Join(home, ".config")) {
			return true
		}
	}

	return false
}

func ValidatePathSafety(absPath string) error {
	if absPath == "" {
		return errors.New("refusing to delete empty path")
	}
	if !filepath.IsAbs(absPath) {
		return errors.New("refusing to delete non-absolute path")
	}
	p := filepath.Clean(absPath)

	if HasTraversalPattern(absPath) {
		return errors.New("refusing path with traversal pattern")
	}
	if p == "/" {
		return errors.New("refusing to delete root directory")
	}
	if IsProtectedPath(p) {
		return errors.New("refusing to delete protected path")
	}

	home, err := os.UserHomeDir()
	if err == nil && p == filepath.Clean(home) {
		return errors.New("refusing to delete home directory")
	}

	return nil
}

// ResolveForNonSymlink validates the string path AND (when the leaf is not a symlink) validates the
// resolved path with symlinked ancestors expanded. This prevents "safe-looking" paths from resolving
// into protected areas via symlink tricks in parent components.
func ResolveForNonSymlink(absPath string) (resolved string, leafIsSymlink bool, err error) {
	if err := ValidatePathSafety(absPath); err != nil {
		return "", false, err
	}

	info, err := os.Lstat(absPath)
	if err != nil {
		return "", false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return absPath, true, nil
	}

	dir := filepath.Dir(absPath)
	base := filepath.Base(absPath)

	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", false, err
	}
	resolved = filepath.Join(realDir, base)
	if err := ValidatePathSafety(resolved); err != nil {
		return "", false, err
	}
	return resolved, false, nil
}

// SafeRemove implements TOCTOU mitigation:
// validate safety, lstat immediately before deletion, unlink symlinks only.
func SafeRemove(absPath string, dryRun bool) error {
	if dryRun {
		return nil
	}

	resolved, leafIsSymlink, err := ResolveForNonSymlink(absPath)
	if err != nil {
		return err
	}
	if leafIsSymlink {
		return os.Remove(absPath)
	}

	// Re-check immediately before deletion in case the leaf changed.
	info, err := os.Lstat(resolved)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return os.Remove(resolved)
	}
	return os.RemoveAll(resolved)
}

func hasPathPrefix(path, prefix string) bool {
	p := filepath.Clean(path)
	pr := filepath.Clean(prefix)
	if p == pr {
		return true
	}
	if strings.HasPrefix(p, pr+string(filepath.Separator)) {
		return true
	}
	return false
}
