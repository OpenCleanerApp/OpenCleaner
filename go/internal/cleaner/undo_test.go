package cleaner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadManifest_RoundTrip(t *testing.T) {
	base := t.TempDir()
	entries := []UndoEntry{{
		SrcPath: filepath.Join(base, "src"),
		DstPath: filepath.Join(base, "dst"),
		Bytes:   123,
		Time:    time.Now().UTC().Truncate(time.Second),
	}}

	if err := SaveManifest(entries, base); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(base)
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != undoManifestVersion {
		t.Fatalf("expected version %d, got %d", undoManifestVersion, m.Version)
	}
	if len(m.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m.Entries))
	}
	if m.Entries[0].SrcPath != entries[0].SrcPath || m.Entries[0].DstPath != entries[0].DstPath || m.Entries[0].Bytes != entries[0].Bytes {
		t.Fatalf("unexpected entry: %+v", m.Entries[0])
	}
}

func TestRestoreMovesBack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	srcDir := filepath.Join(home, "Library", "Caches")
	if err := os.MkdirAll(srcDir, 0o700); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(srcDir, "file.txt")
	content := []byte("hello")
	if err := os.WriteFile(src, content, 0o600); err != nil {
		t.Fatal(err)
	}

	dst, err := MoveToTrash(src, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(src); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected src to be moved, got err=%v", err)
	}
	if _, err := os.Lstat(dst); err != nil {
		t.Fatalf("expected dst to exist: %v", err)
	}

	m := &UndoManifest{Version: undoManifestVersion, Entries: []UndoEntry{{SrcPath: src, DstPath: dst, Bytes: int64(len(content)), Time: time.Now().UTC()}}}
	restored, failed, err := Restore(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	if restored != 1 {
		t.Fatalf("expected restored=1, got %d", restored)
	}
	if len(failed) != 0 {
		t.Fatalf("expected no failures, got %v", failed)
	}
	if _, err := os.Lstat(src); err != nil {
		t.Fatalf("expected src restored: %v", err)
	}
	if _, err := os.Lstat(dst); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected dst removed from trash, got err=%v", err)
	}
}

func TestRestoreFailsForNonTrashDst(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	src := filepath.Join(home, "Library", "Caches", "file.txt")
	m := &UndoManifest{Version: undoManifestVersion, Entries: []UndoEntry{{SrcPath: src, DstPath: "/System/evil", Bytes: 1, Time: time.Now().UTC()}}}
	restored, failed, err := Restore(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	if restored != 0 {
		t.Fatalf("expected restored=0, got %d", restored)
	}
	if len(failed) != 1 {
		t.Fatalf("expected 1 failed item, got %v", failed)
	}
}

func TestRestoreFailsWhenSrcHasSymlinkAncestor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	srcParent := filepath.Join(home, "Library", "Caches")
	if err := os.MkdirAll(srcParent, 0o700); err != nil {
		t.Fatal(err)
	}

	outside := t.TempDir()
	link := filepath.Join(srcParent, "restore-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(link, "file.txt")
	trash, err := TrashDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(trash, 0o700); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(trash, "file.txt")
	if err := os.WriteFile(dst, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := &UndoManifest{Version: undoManifestVersion, Entries: []UndoEntry{{SrcPath: src, DstPath: dst, Bytes: 5, Time: time.Now().UTC()}}}
	restored, failed, err := Restore(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	if restored != 0 {
		t.Fatalf("expected restored=0, got %d", restored)
	}
	if len(failed) != 1 {
		t.Fatalf("expected 1 failed item, got %v", failed)
	}
	if _, err := os.Lstat(dst); err != nil {
		t.Fatalf("expected dst to remain in trash: %v", err)
	}
}
