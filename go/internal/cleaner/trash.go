package cleaner

import (
	"errors"
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
		nonce := time.Now().UTC().UnixNano()
		for i := 0; i < 1000; i++ {
			cand := filepath.Join(trash, fmt.Sprintf("%s.%d.%d", base, nonce, i))
			if _, err := os.Lstat(cand); errors.Is(err, os.ErrNotExist) {
				dst = cand
				break
			} else if err != nil {
				return "", err
			}
		}
		if dst == filepath.Join(trash, base) {
			return "", errors.New("failed to find unique trash path")
		}
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
