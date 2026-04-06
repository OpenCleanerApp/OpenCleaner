package cleaner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/opencleaner/opencleaner/internal/safety"
)

const undoManifestVersion = 1

type UndoEntry struct {
	SrcPath string    `json:"src_path"`
	DstPath string    `json:"dst_path"`
	Bytes   int64     `json:"bytes"`
	Time    time.Time `json:"time"`
}

type UndoManifest struct {
	Version int         `json:"version"`
	Entries []UndoEntry `json:"entries"`
}

func SaveManifest(entries []UndoEntry, baseDir string) error {
	if len(entries) == 0 {
		return nil
	}
	path, err := undoManifestPath(baseDir)
	if err != nil {
		return err
	}

	m := UndoManifest{Version: undoManifestVersion, Entries: entries}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "last.*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpName, path)
}

func LoadManifest(baseDir string) (*UndoManifest, error) {
	path, err := undoManifestPath(baseDir)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var m UndoManifest
	if err := json.NewDecoder(f).Decode(&m); err != nil {
		return nil, err
	}
	if m.Version != undoManifestVersion {
		return nil, fmt.Errorf("unsupported undo manifest version %d", m.Version)
	}
	return &m, nil
}

func ClearManifest(baseDir string) error {
	path, err := undoManifestPath(baseDir)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func Restore(ctx context.Context, manifest *UndoManifest) (restored int, failed []string, err error) {
	if manifest == nil {
		return 0, nil, errors.New("nil manifest")
	}
	if len(manifest.Entries) == 0 {
		return 0, nil, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return 0, nil, fmt.Errorf("undo requires home dir: %w", err)
	}
	if home == "" {
		return 0, nil, errors.New("undo requires home dir")
	}
	home = filepath.Clean(home)

	trash, err := TrashDir()
	if err != nil {
		return 0, nil, err
	}
	trash = filepath.Clean(trash)

	if !isWithinRoot(home, trash) {
		return 0, nil, errors.New("trash dir is outside home")
	}
	if err := safety.ValidateNoSymlinkAncestorsWithin(home, trash); err != nil {
		return 0, nil, err
	}

	for _, ent := range manifest.Entries {
		select {
		case <-ctx.Done():
			return restored, failed, ctx.Err()
		default:
		}

		src := filepath.Clean(ent.SrcPath)
		dst := filepath.Clean(ent.DstPath)

		if src == "" || dst == "" {
			failed = append(failed, src)
			continue
		}
		if !filepath.IsAbs(src) || !filepath.IsAbs(dst) {
			failed = append(failed, src)
			continue
		}
		if safety.HasTraversalPattern(ent.SrcPath) || safety.HasTraversalPattern(ent.DstPath) {
			failed = append(failed, src)
			continue
		}

		if !isWithinRoot(home, src) {
			failed = append(failed, src)
			continue
		}
		if err := safety.ValidatePathSafety(src); err != nil {
			failed = append(failed, src)
			continue
		}
		if !isWithinRoot(trash, dst) {
			failed = append(failed, src)
			continue
		}

		srcDir := filepath.Dir(src)
		if err := safety.ValidateNoSymlinkAncestorsWithin(home, srcDir); err != nil {
			failed = append(failed, src)
			continue
		}
		if err := safety.ValidateNoSymlinkAncestorsWithin(home, filepath.Dir(dst)); err != nil {
			failed = append(failed, src)
			continue
		}

		if _, err := os.Lstat(dst); err != nil {
			failed = append(failed, src)
			continue
		}
		if _, err := os.Lstat(src); err == nil {
			failed = append(failed, src)
			continue
		}

		if err := os.MkdirAll(srcDir, 0o700); err != nil {
			failed = append(failed, src)
			continue
		}
		if err := safety.ValidateNoSymlinkAncestorsWithin(home, srcDir); err != nil {
			failed = append(failed, src)
			continue
		}
		if err := os.Rename(dst, src); err != nil {
			failed = append(failed, src)
			continue
		}
		restored++
	}

	return restored, failed, nil
}

func undoManifestPath(baseDir string) (string, error) {
	base := baseDir
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".opencleaner")
	}
	if !filepath.IsAbs(base) {
		return "", errors.New("baseDir must be absolute")
	}
	return filepath.Join(base, "undo", "last.json"), nil
}

func isWithinRoot(root, absPath string) bool {
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	if safety.HasTraversalPattern(rel) {
		return false
	}
	return true
}
