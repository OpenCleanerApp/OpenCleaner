package cleaner

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/opencleaner/opencleaner/internal/safety"
)

func TrashDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".Trash"), nil
}

func MoveToTrash(absPath string, dryRun bool) (string, error) {
	resolved, leafIsSymlink, err := safety.ResolveForNonSymlink(absPath)
	if err != nil {
		return "", err
	}

	trash, err := TrashDir()
	if err != nil {
		return "", err
	}

	base := filepath.Base(absPath)
	dst := filepath.Join(trash, base)
	if _, err := os.Lstat(dst); err == nil {
		suffix := time.Now().UTC().Format("20060102T150405Z")
		dst = filepath.Join(trash, fmt.Sprintf("%s.%s", base, suffix))
	}
	if dryRun {
		return dst, nil
	}

	if err := os.MkdirAll(trash, 0o700); err != nil {
		return "", err
	}

	src := absPath
	if !leafIsSymlink {
		src = resolved
	}
	if err := os.Rename(src, dst); err != nil {
		return "", err
	}
	return dst, nil
}
