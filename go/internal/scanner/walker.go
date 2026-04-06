package scanner

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// WalkConfig controls bounded recursive directory discovery.
type WalkConfig struct {
	RootDirs   []string        // starting directories to walk
	TargetName string          // directory name to match (e.g. "node_modules")
	MaxDepth   int             // max recursion depth from each root (default 10)
	SkipNames  map[string]bool // dir names to skip entirely
	SkipHidden bool            // skip dot-prefixed dirs (may need false for .venv)
	OnMatch    func(path string)
}

var defaultSkipNames = map[string]bool{
	".git":      true,
	".hg":       true,
	"vendor":    true,
	"__MACOSX":  true,
	".Trash":    true,
}

// Walk performs a bounded recursive search for directories named TargetName
// within each RootDir, calling OnMatch for every match found.
// Matched directories are not descended into (SkipDir).
func Walk(ctx context.Context, cfg WalkConfig) error {
	if cfg.MaxDepth <= 0 {
		cfg.MaxDepth = 10
	}
	if cfg.SkipNames == nil {
		cfg.SkipNames = defaultSkipNames
	}

	for _, root := range cfg.RootDirs {
		info, err := os.Lstat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		root = filepath.Clean(root)

		if err := walkRoot(ctx, root, cfg); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// permission errors etc — skip root, continue
			continue
		}
	}
	return nil
}

func walkRoot(ctx context.Context, root string, cfg WalkConfig) error {
	count := 0
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}

		// Check context every 200 entries.
		count++
		if count%200 == 0 {
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}

		if !d.IsDir() {
			return nil
		}

		// Don't process root itself.
		if path == root {
			return nil
		}

		name := d.Name()

		// Depth check: count separators in the relative path.
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return filepath.SkipDir
		}
		depth := strings.Count(rel, string(os.PathSeparator)) + 1
		if depth > cfg.MaxDepth {
			return filepath.SkipDir
		}

		// Skip hidden dirs (unless disabled for scanners like .venv).
		if cfg.SkipHidden && len(name) > 0 && name[0] == '.' {
			return filepath.SkipDir
		}

		// Skip explicitly named dirs.
		if cfg.SkipNames[name] {
			return filepath.SkipDir
		}

		// Don't follow symlinks into directories.
		if d.Type()&os.ModeSymlink != 0 {
			return filepath.SkipDir
		}

		// Match target.
		if name == cfg.TargetName {
			if cfg.OnMatch != nil {
				cfg.OnMatch(path)
			}
			return filepath.SkipDir // don't recurse into matched dir
		}

		return nil
	})
}
