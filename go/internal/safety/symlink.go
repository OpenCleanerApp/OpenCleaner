package safety

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateNoSymlinkAncestorsWithin ensures that all *existing* path components from root
// down to absPath are not symlinks.
//
// This mitigates cases where a safe-looking path (stringwise) resolves outside the
// intended tree because an ancestor directory is a symlink.
func ValidateNoSymlinkAncestorsWithin(root, absPath string) error {
	if root == "" {
		return errors.New("root required")
	}
	if absPath == "" {
		return errors.New("path required")
	}
	if !filepath.IsAbs(root) || !filepath.IsAbs(absPath) {
		return errors.New("root and path must be absolute")
	}

	root = filepath.Clean(root)
	absPath = filepath.Clean(absPath)

	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}
	// Any traversal means absPath escapes root.
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || HasTraversalPattern(rel) {
		return errors.New("path is outside root")
	}

	cur := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if errors.Is(err, os.ErrNotExist) {
			// Remaining components do not exist yet.
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink ancestor: %s", cur)
		}
	}
	return nil
}
