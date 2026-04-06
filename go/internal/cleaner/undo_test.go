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

// --- Additional edge-case tests ---

func TestClearManifest(t *testing.T) {
	base := t.TempDir()
	entries := []UndoEntry{{SrcPath: "/a", DstPath: "/b", Bytes: 1, Time: time.Now().UTC()}}
	if err := SaveManifest(entries, base); err != nil {
		t.Fatal(err)
	}
	if err := ClearManifest(base); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(base); err == nil {
		t.Fatal("expected error after clear")
	}
}

func TestClearManifestNonExistent(t *testing.T) {
	base := t.TempDir()
	if err := ClearManifest(base); err != nil {
		t.Fatalf("clear non-existent should succeed, got %v", err)
	}
}

func TestSaveManifestEmpty(t *testing.T) {
	base := t.TempDir()
	if err := SaveManifest(nil, base); err != nil {
		t.Fatal(err)
	}
	// No file should be written.
	_, err := LoadManifest(base)
	if err == nil {
		t.Fatal("expected error for no manifest file")
	}
}

func TestUndoManifestPathRelative(t *testing.T) {
	_, err := undoManifestPath("relative/dir")
	if err == nil {
		t.Fatal("expected error for relative baseDir")
	}
}

func TestUndoManifestPathEmpty(t *testing.T) {
	// Uses HOME; should return a path.
	path, err := undoManifestPath("")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %s", path)
	}
}

func TestRestoreNilManifest(t *testing.T) {
	_, _, err := Restore(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil manifest")
	}
}

func TestRestoreEmptyEntries(t *testing.T) {
	m := &UndoManifest{Version: undoManifestVersion}
	restored, failed, err := Restore(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	if restored != 0 || len(failed) != 0 {
		t.Fatalf("expected 0/0, got %d/%d", restored, len(failed))
	}
}

func TestRestoreContextCancelled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trash, _ := TrashDir()
	os.MkdirAll(trash, 0o700)
	dst := filepath.Join(trash, "cancel-test")
	os.WriteFile(dst, []byte("x"), 0o600)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	m := &UndoManifest{Version: undoManifestVersion, Entries: []UndoEntry{
		{SrcPath: filepath.Join(home, "Library", "f"), DstPath: dst, Bytes: 1, Time: time.Now().UTC()},
	}}
	_, _, err := Restore(ctx, m)
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestRestoreRelativeSrc(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trash, _ := TrashDir()
	os.MkdirAll(trash, 0o700)
	dst := filepath.Join(trash, "rel-test")
	os.WriteFile(dst, []byte("x"), 0o600)

	m := &UndoManifest{Version: undoManifestVersion, Entries: []UndoEntry{
		{SrcPath: "relative/path", DstPath: dst, Bytes: 1, Time: time.Now().UTC()},
	}}
	restored, failed, _ := Restore(context.Background(), m)
	if restored != 0 {
		t.Error("expected 0 restored")
	}
	if len(failed) != 1 {
		t.Error("expected 1 failed")
	}
}

func TestRestoreSrcOutsideHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trash, _ := TrashDir()
	os.MkdirAll(trash, 0o700)
	dst := filepath.Join(trash, "outside-test")
	os.WriteFile(dst, []byte("x"), 0o600)

	m := &UndoManifest{Version: undoManifestVersion, Entries: []UndoEntry{
		{SrcPath: "/etc/passwd", DstPath: dst, Bytes: 1, Time: time.Now().UTC()},
	}}
	restored, failed, _ := Restore(context.Background(), m)
	if restored != 0 {
		t.Error("expected 0 restored for outside-home src")
	}
	if len(failed) != 1 {
		t.Error("expected 1 failed")
	}
}

func TestRestoreSrcAlreadyExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	src := filepath.Join(home, "Library", "Caches", "existing")
	os.MkdirAll(filepath.Dir(src), 0o700)
	os.WriteFile(src, []byte("existing"), 0o600)

	trash, _ := TrashDir()
	os.MkdirAll(trash, 0o700)
	dst := filepath.Join(trash, "existing")
	os.WriteFile(dst, []byte("backup"), 0o600)

	m := &UndoManifest{Version: undoManifestVersion, Entries: []UndoEntry{
		{SrcPath: src, DstPath: dst, Bytes: 6, Time: time.Now().UTC()},
	}}
	restored, failed, _ := Restore(context.Background(), m)
	if restored != 0 {
		t.Error("should not restore when src already exists")
	}
	if len(failed) != 1 {
		t.Error("expected 1 failed")
	}
}

func TestRestoreDstMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trash, _ := TrashDir()
	os.MkdirAll(trash, 0o700)

	src := filepath.Join(home, "Library", "Caches", "missing-dst")
	m := &UndoManifest{Version: undoManifestVersion, Entries: []UndoEntry{
		{SrcPath: src, DstPath: filepath.Join(trash, "nonexistent"), Bytes: 1, Time: time.Now().UTC()},
	}}
	restored, failed, _ := Restore(context.Background(), m)
	if restored != 0 {
		t.Error("should not restore when dst missing")
	}
	if len(failed) != 1 {
		t.Error("expected 1 failed")
	}
}

func TestRestoreTraversalPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trash, _ := TrashDir()
	os.MkdirAll(trash, 0o700)
	dst := filepath.Join(trash, "traversal-test")
	os.WriteFile(dst, []byte("x"), 0o600)

	m := &UndoManifest{Version: undoManifestVersion, Entries: []UndoEntry{
		{SrcPath: filepath.Join(home, "Library", "..", "..", "etc"), DstPath: dst, Bytes: 1, Time: time.Now().UTC()},
	}}
	restored, failed, _ := Restore(context.Background(), m)
	if restored != 0 {
		t.Error("should not restore with traversal")
	}
	if len(failed) != 1 {
		t.Error("expected 1 failed")
	}
}

func TestIsWithinRoot(t *testing.T) {
	tests := []struct {
		root, path string
		want       bool
	}{
		{"/home/user", "/home/user/Library", true},
		{"/home/user", "/home/user", true},
		{"/home/user", "/home/other", false},
		{"/home/user", "/etc", false},
		{"/a", "/a/b/c", true},
	}
	for _, tt := range tests {
		got := isWithinRoot(tt.root, tt.path)
		if got != tt.want {
			t.Errorf("isWithinRoot(%q, %q) = %v, want %v", tt.root, tt.path, got, tt.want)
		}
	}
}

func TestSaveManifestMultipleEntries(t *testing.T) {
	base := t.TempDir()
	entries := []UndoEntry{
		{SrcPath: "/a/1", DstPath: "/b/1", Bytes: 100, Time: time.Now().UTC()},
		{SrcPath: "/a/2", DstPath: "/b/2", Bytes: 200, Time: time.Now().UTC()},
		{SrcPath: "/a/3", DstPath: "/b/3", Bytes: 300, Time: time.Now().UTC()},
	}
	if err := SaveManifest(entries, base); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(m.Entries))
	}
}

func TestMoveToTrashAndRestore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	src := filepath.Join(home, "Library", "test-move")
	os.MkdirAll(src, 0o700)
	os.WriteFile(filepath.Join(src, "data"), []byte("content"), 0o600)

	dst, err := MoveToTrash(src, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(src); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("src should be gone")
	}
	if _, err := os.Lstat(dst); err != nil {
		t.Fatal("dst should exist in trash")
	}
}

func TestMoveToTrashDryRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	src := filepath.Join(home, "Library", "dry-test")
	os.MkdirAll(src, 0o700)
	os.WriteFile(filepath.Join(src, "data"), []byte("content"), 0o600)

	dst, err := MoveToTrash(src, true)
	if err != nil {
		t.Fatal(err)
	}
	if dst == "" {
		t.Fatal("expected non-empty dst path even in dry-run")
	}
	if _, err := os.Lstat(src); err != nil {
		t.Fatal("src should still exist in dry-run")
	}
}

func TestLoadManifestBadVersion(t *testing.T) {
	dir := t.TempDir()
	undoDir := filepath.Join(dir, "undo")
	os.MkdirAll(undoDir, 0o700)
	os.WriteFile(filepath.Join(undoDir, "last.json"), []byte(`{"version":999,"entries":[]}`), 0o600)

	_, err := LoadManifest(dir)
	if err == nil {
		t.Fatal("expected error for bad version")
	}
}

func TestLoadManifestNoFile(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadManifest(dir)
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
}

func TestLoadManifestBadJSON(t *testing.T) {
	dir := t.TempDir()
	undoDir := filepath.Join(dir, "undo")
	os.MkdirAll(undoDir, 0o700)
	os.WriteFile(filepath.Join(undoDir, "last.json"), []byte(`not json`), 0o600)

	_, err := LoadManifest(dir)
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}
}

func TestRestoreSuccessful(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trashDir := filepath.Join(home, ".Trash")
	os.MkdirAll(trashDir, 0o700)

	// Create item in trash.
	trashItem := filepath.Join(trashDir, "restored-file")
	os.WriteFile(trashItem, []byte("data"), 0o600)

	// Original location (doesn't exist yet).
	origDir := filepath.Join(home, "Documents")
	os.MkdirAll(origDir, 0o700)
	origPath := filepath.Join(origDir, "restored-file")

	manifest := &UndoManifest{
		Version: 1,
		Entries: []UndoEntry{
			{SrcPath: origPath, DstPath: trashItem, Bytes: 4},
		},
	}

	restored, failed, err := Restore(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if restored != 1 {
		t.Errorf("expected 1 restored, got %d", restored)
	}
	if len(failed) != 0 {
		t.Errorf("expected 0 failed, got %v", failed)
	}

	if _, err := os.Lstat(origPath); err != nil {
		t.Error("file should be restored to original path")
	}
	if _, err := os.Lstat(trashItem); err == nil {
		t.Error("file should be removed from trash")
	}
}

func TestSaveManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	entries := []UndoEntry{
		{SrcPath: "/home/user/a", DstPath: "/home/user/.Trash/a", Bytes: 100},
		{SrcPath: "/home/user/b", DstPath: "/home/user/.Trash/b", Bytes: 200},
	}
	if err := SaveManifest(entries, dir); err != nil {
		t.Fatal(err)
	}

	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m.Entries))
	}
	if m.Entries[0].Bytes != 100 || m.Entries[1].Bytes != 200 {
		t.Errorf("entries bytes mismatch")
	}
}

func TestRestoreDstNotInTrash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trashDir := filepath.Join(home, ".Trash")
	os.MkdirAll(trashDir, 0o700)

	// dst is NOT in trash — should fail validation.
	manifest := &UndoManifest{
		Version: 1,
		Entries: []UndoEntry{
			{SrcPath: filepath.Join(home, "a"), DstPath: filepath.Join(home, "other", "b"), Bytes: 1},
		},
	}
	restored, failed, err := Restore(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if restored != 0 {
		t.Errorf("expected 0 restored, got %d", restored)
	}
	if len(failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(failed))
	}
}

func TestMoveToTrashCollision(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trashDir := filepath.Join(home, ".Trash")
	os.MkdirAll(trashDir, 0o700)

	// Pre-create item with same name in trash to trigger collision path.
	srcDir := filepath.Join(home, "Library", "collision")
	os.MkdirAll(srcDir, 0o700)
	os.WriteFile(filepath.Join(srcDir, "data"), []byte("src"), 0o600)

	trashItem := filepath.Join(trashDir, "collision")
	os.WriteFile(trashItem, []byte("existing"), 0o600)

	dst, err := MoveToTrash(srcDir, false)
	if err != nil {
		t.Fatal(err)
	}
	// dst should be different from the base "collision" name.
	if filepath.Base(dst) == "collision" {
		t.Error("expected unique trash name due to collision")
	}
}

func TestMoveToTrashNonExistent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".Trash"), 0o700)

	_, err := MoveToTrash(filepath.Join(home, "nonexistent"), false)
	if err == nil {
		t.Error("expected error for non-existent source")
	}
}

func TestTrashDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir, err := TrashDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(home, ".Trash") {
		t.Errorf("expected %s, got %s", filepath.Join(home, ".Trash"), dir)
	}
}

func TestRestoreSymlinkInSrcParent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trashDir := filepath.Join(home, ".Trash")
	os.MkdirAll(trashDir, 0o700)
	trashItem := filepath.Join(trashDir, "item")
	os.WriteFile(trashItem, []byte("data"), 0o600)

	// Create symlink in src parent path.
	realDir := filepath.Join(home, "real")
	os.MkdirAll(realDir, 0o700)
	linkDir := filepath.Join(home, "linked")
	os.Symlink(realDir, linkDir)

	manifest := &UndoManifest{
		Version: 1,
		Entries: []UndoEntry{
			{SrcPath: filepath.Join(linkDir, "item"), DstPath: trashItem, Bytes: 4},
		},
	}
	restored, failed, err := Restore(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if restored != 0 {
		t.Errorf("expected 0 restored (symlink parent), got %d", restored)
	}
	if len(failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(failed))
	}
}

func TestClearManifestAbsPath(t *testing.T) {
	dir := t.TempDir()
	undoDir := filepath.Join(dir, "undo")
	os.MkdirAll(undoDir, 0o700)
	mPath := filepath.Join(undoDir, "last.json")
	os.WriteFile(mPath, []byte(`{"version":1,"entries":[]}`), 0o600)

	if err := ClearManifest(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mPath); !errors.Is(err, os.ErrNotExist) {
		t.Error("manifest should be deleted")
	}
}

func TestRestoreValidatePathSafetyFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trashDir := filepath.Join(home, ".Trash")
	os.MkdirAll(trashDir, 0o700)
	trashItem := filepath.Join(trashDir, "item")
	os.WriteFile(trashItem, []byte("data"), 0o600)

	// Use root "/" as src — ValidatePathSafety will reject it.
	manifest := &UndoManifest{
		Version: 1,
		Entries: []UndoEntry{
			{SrcPath: "/", DstPath: trashItem, Bytes: 4},
		},
	}
	_, failed, err := Restore(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 {
		t.Errorf("expected 1 failed (protected path), got %d", len(failed))
	}
}

func TestRestoreSymlinkInDstParent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trashDir := filepath.Join(home, ".Trash")
	os.MkdirAll(trashDir, 0o700)

	// Create symlink inside trash dir.
	realTrash := filepath.Join(trashDir, "real")
	os.MkdirAll(realTrash, 0o700)
	linkInTrash := filepath.Join(trashDir, "linked")
	os.Symlink(realTrash, linkInTrash)

	trashItem := filepath.Join(linkInTrash, "item")
	os.WriteFile(filepath.Join(realTrash, "item"), []byte("data"), 0o600)

	origPath := filepath.Join(home, "Documents", "item")
	os.MkdirAll(filepath.Dir(origPath), 0o700)

	manifest := &UndoManifest{
		Version: 1,
		Entries: []UndoEntry{
			{SrcPath: origPath, DstPath: trashItem, Bytes: 4},
		},
	}
	_, failed, err := Restore(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 {
		t.Errorf("expected 1 failed (symlink in dst parent), got %d", len(failed))
	}
}

func TestRestoreSrcAlreadyExistsConflict(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trashDir := filepath.Join(home, ".Trash")
	os.MkdirAll(trashDir, 0o700)
	trashItem := filepath.Join(trashDir, "exists")
	os.WriteFile(trashItem, []byte("trash"), 0o600)

	origDir := filepath.Join(home, "Documents")
	os.MkdirAll(origDir, 0o700)
	origPath := filepath.Join(origDir, "exists")
	os.WriteFile(origPath, []byte("original"), 0o600)

	manifest := &UndoManifest{
		Version: 1,
		Entries: []UndoEntry{
			{SrcPath: origPath, DstPath: trashItem, Bytes: 5},
		},
	}
	_, failed, err := Restore(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 {
		t.Errorf("expected 1 failed (src exists), got %d", len(failed))
	}
}

func TestIsWithinRootCleaner(t *testing.T) {
	tests := []struct {
		root, path string
		want       bool
	}{
		{"/home/user", "/home/user/sub", true},
		{"/home/user", "/home/user", true},
		{"/home/user", "/home/other", false},
		{"/home/user", "/etc/passwd", false},
		{"/home/user", "/home/user/../other", false},
	}
	for _, tt := range tests {
		got := isWithinRoot(tt.root, tt.path)
		if got != tt.want {
			t.Errorf("isWithinRoot(%q, %q) = %v, want %v", tt.root, tt.path, got, tt.want)
		}
	}
}

func TestRestoreEmptyPathEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trashDir := filepath.Join(home, ".Trash")
	os.MkdirAll(trashDir, 0o700)

	manifest := &UndoManifest{
		Version: 1,
		Entries: []UndoEntry{
			{SrcPath: "", DstPath: filepath.Join(trashDir, "x"), Bytes: 1},
		},
	}
	_, failed, err := Restore(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 {
		t.Errorf("expected 1 failed for empty src, got %d", len(failed))
	}
}

func TestRestoreRelativePathEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trashDir := filepath.Join(home, ".Trash")
	os.MkdirAll(trashDir, 0o700)

	manifest := &UndoManifest{
		Version: 1,
		Entries: []UndoEntry{
			{SrcPath: "relative/path", DstPath: filepath.Join(trashDir, "x"), Bytes: 1},
		},
	}
	_, failed, err := Restore(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 {
		t.Errorf("expected 1 failed for relative path, got %d", len(failed))
	}
}

func TestRestoreDstOutsideTrashEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trashDir := filepath.Join(home, ".Trash")
	os.MkdirAll(trashDir, 0o700)

	manifest := &UndoManifest{
		Version: 1,
		Entries: []UndoEntry{
			{SrcPath: filepath.Join(home, "Documents", "item"), DstPath: filepath.Join(home, "nottrash", "item"), Bytes: 4},
		},
	}
	_, failed, err := Restore(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 {
		t.Errorf("expected 1 failed for dst outside trash, got %d", len(failed))
	}
}

func TestRestoreDstNotFoundEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trashDir := filepath.Join(home, ".Trash")
	os.MkdirAll(trashDir, 0o700)

	manifest := &UndoManifest{
		Version: 1,
		Entries: []UndoEntry{
			{SrcPath: filepath.Join(home, "Documents", "item"), DstPath: filepath.Join(trashDir, "missing"), Bytes: 4},
		},
	}
	_, failed, err := Restore(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 {
		t.Errorf("expected 1 failed for missing dst, got %d", len(failed))
	}
}

func TestMoveToTrashSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trashDir := filepath.Join(home, ".Trash")
	os.MkdirAll(trashDir, 0o700)

	realFile := filepath.Join(home, "realfile")
	os.WriteFile(realFile, []byte("real"), 0o600)

	link := filepath.Join(home, "link")
	os.Symlink(realFile, link)

	dst, err := MoveToTrash(link, false)
	if err != nil {
		t.Fatal(err)
	}
	if dst == "" {
		t.Error("expected non-empty dst")
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Error("expected symlink to be removed")
	}
	if _, err := os.Stat(realFile); err != nil {
		t.Error("expected real file to still exist")
	}
}

func TestMoveToTrashDryRunSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trashDir := filepath.Join(home, ".Trash")
	os.MkdirAll(trashDir, 0o700)

	realFile := filepath.Join(home, "realfile")
	os.WriteFile(realFile, []byte("real"), 0o600)

	link := filepath.Join(home, "link")
	os.Symlink(realFile, link)

	dst, err := MoveToTrash(link, true)
	if err != nil {
		t.Fatal(err)
	}
	if dst == "" {
		t.Error("expected non-empty dst")
	}
	if _, err := os.Lstat(link); err != nil {
		t.Error("expected symlink to still exist in dry run")
	}
}

func TestRestoreMkdirAllCreatesParent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trashDir := filepath.Join(home, ".Trash")
	os.MkdirAll(trashDir, 0o700)

	// Put an item in trash.
	trashItem := filepath.Join(trashDir, "deep-item")
	os.WriteFile(trashItem, []byte("data"), 0o600)

	// Set src to a path whose parent dir does NOT exist — MkdirAll should create it.
	deepDir := filepath.Join(home, "Projects", "sub", "deep")
	srcPath := filepath.Join(deepDir, "deep-item")

	manifest := &UndoManifest{
		Version: 1,
		Entries: []UndoEntry{
			{SrcPath: srcPath, DstPath: trashItem, Bytes: 4},
		},
	}
	restored, failed, err := Restore(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 0 {
		t.Errorf("expected 0 failed, got %d: %v", len(failed), failed)
	}
	if restored != 1 {
		t.Errorf("expected 1 restored, got %d", restored)
	}

	// Verify the parent dir was created and file is restored.
	if _, err := os.Stat(srcPath); err != nil {
		t.Errorf("expected restored file at %s: %v", srcPath, err)
	}
}

func TestUndoManifestPathRelativeError(t *testing.T) {
	_, err := undoManifestPath("relative/path")
	if err == nil {
		t.Fatal("expected error for relative baseDir")
	}
}

func TestIsWithinRootEdgeCases(t *testing.T) {
	if !isWithinRoot("/a", "/a") {
		t.Error("root==path should return true")
	}
	if isWithinRoot("/a/b", "/a/b/../../c") {
		t.Error("should be false for traversal path")
	}
}

func TestRestoreMkdirAllFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trash := filepath.Join(home, ".Trash")
	os.MkdirAll(trash, 0o700)
	trashItem := filepath.Join(trash, "item")
	os.WriteFile(trashItem, []byte("data"), 0o600)

	// Make the parent dir a file so MkdirAll fails.
	blockDir := filepath.Join(home, "blocked")
	os.WriteFile(blockDir, []byte("not-a-dir"), 0o600)

	srcPath := filepath.Join(blockDir, "sub", "item")
	manifest := &UndoManifest{
		Version: 1,
		Entries: []UndoEntry{{SrcPath: srcPath, DstPath: trashItem, Bytes: 4}},
	}
	restored, failed, err := Restore(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if restored != 0 {
		t.Errorf("expected 0 restored, got %d", restored)
	}
	if len(failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(failed))
	}
}

func TestRestoreRenameFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trash := filepath.Join(home, ".Trash")
	os.MkdirAll(trash, 0o700)

	// Create a directory in trash as the "file" to restore.
	trashItem := filepath.Join(trash, "rename-fail")
	os.MkdirAll(trashItem, 0o700)
	os.WriteFile(filepath.Join(trashItem, "inner"), []byte("x"), 0o600)

	// Make destination directory read-only so Rename fails.
	destDir := filepath.Join(home, "ro-parent")
	os.MkdirAll(destDir, 0o700)
	srcPath := filepath.Join(destDir, "rename-fail")

	// Make parent read-only.
	os.Chmod(destDir, 0o500)
	defer os.Chmod(destDir, 0o700) // cleanup

	manifest := &UndoManifest{
		Version: 1,
		Entries: []UndoEntry{{SrcPath: srcPath, DstPath: trashItem, Bytes: 1}},
	}
	restored, failed, err := Restore(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if restored != 0 {
		t.Errorf("expected 0 restored, got %d", restored)
	}
	if len(failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(failed))
	}
}

func TestSaveManifestReadOnlyDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create the .opencleaner dir, then make it read-only.
	ocDir := filepath.Join(home, ".opencleaner")
	os.MkdirAll(ocDir, 0o700)
	os.Chmod(ocDir, 0o500)
	defer os.Chmod(ocDir, 0o700)

	err := SaveManifest([]UndoEntry{{SrcPath: "/a", DstPath: "/b", Bytes: 1}}, "")
	if err == nil {
		t.Error("expected error writing to read-only dir")
	}
}

func TestSaveManifestCreateTempFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create the full undo dir, then make it read-only so CreateTemp fails.
	undoDir := filepath.Join(home, ".opencleaner", "undo")
	os.MkdirAll(undoDir, 0o700)
	os.Chmod(undoDir, 0o500)
	defer os.Chmod(undoDir, 0o700)

	err := SaveManifest([]UndoEntry{{SrcPath: "/a", DstPath: "/b", Bytes: 1}}, "")
	if err == nil {
		t.Error("expected error from CreateTemp in read-only undo dir")
	}
}

func TestClearManifestNoFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// ClearManifest on non-existing file should succeed (not error).
	err := ClearManifest("")
	if err != nil {
		t.Errorf("ClearManifest with no file should not error: %v", err)
	}
}

func TestRestoreContextCancellation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trash := filepath.Join(home, ".Trash")
	os.MkdirAll(trash, 0o700)

	// Create two items in trash.
	for i := 0; i < 2; i++ {
		name := filepath.Join(trash, "ctx-item-"+string(rune('a'+i)))
		os.WriteFile(name, []byte("data"), 0o600)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	manifest := &UndoManifest{
		Version: 1,
		Entries: []UndoEntry{
			{SrcPath: filepath.Join(home, "restored-a"), DstPath: filepath.Join(trash, "ctx-item-a"), Bytes: 4},
			{SrcPath: filepath.Join(home, "restored-b"), DstPath: filepath.Join(trash, "ctx-item-b"), Bytes: 4},
		},
	}
	_, _, err := Restore(ctx, manifest)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRestoreSymlinkAncestorInSrcDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trash := filepath.Join(home, ".Trash")
	os.MkdirAll(trash, 0o700)
	trashItem := filepath.Join(trash, "item")
	os.WriteFile(trashItem, []byte("data"), 0o600)

	// Create a symlink in the path to srcDir.
	realDir := filepath.Join(home, "real")
	os.MkdirAll(realDir, 0o700)
	linkDir := filepath.Join(home, "linked")
	os.Symlink(realDir, linkDir)

	srcPath := filepath.Join(linkDir, "item")
	manifest := &UndoManifest{
		Version: 1,
		Entries: []UndoEntry{{SrcPath: srcPath, DstPath: trashItem, Bytes: 4}},
	}
	restored, failed, _ := Restore(context.Background(), manifest)
	if restored != 0 {
		t.Error("should not restore through symlink ancestor")
	}
	if len(failed) != 1 {
		t.Error("should fail for symlink ancestor in src dir")
	}
}

func TestRestoreTraversalPatternInPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trash := filepath.Join(home, ".Trash")
	os.MkdirAll(trash, 0o700)
	trashItem := filepath.Join(trash, "item")
	os.WriteFile(trashItem, []byte("data"), 0o600)

	// Use raw string to preserve traversal pattern (filepath.Join normalizes it away).
	srcPath := home + "/a/../../../etc/passwd"
	manifest := &UndoManifest{
		Version: 1,
		Entries: []UndoEntry{{SrcPath: srcPath, DstPath: trashItem, Bytes: 4}},
	}
	restored, failed, _ := Restore(context.Background(), manifest)
	if restored != 0 {
		t.Error("should not restore with traversal pattern")
	}
	if len(failed) == 0 {
		t.Error("should fail for traversal pattern")
	}
}

func TestMoveToTrashDryRunCollision(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trash := filepath.Join(home, ".Trash")
	os.MkdirAll(trash, 0o700)
	os.WriteFile(filepath.Join(trash, "coltest"), []byte("existing"), 0o600)

	src := filepath.Join(home, "coltest")
	os.WriteFile(src, []byte("new"), 0o600)

	dst, err := MoveToTrash(src, true)
	if err != nil {
		t.Fatal(err)
	}
	if dst == filepath.Join(trash, "coltest") {
		t.Error("expected unique path due to collision in dry-run")
	}
	if _, err := os.Stat(src); err != nil {
		t.Error("source should still exist in dry-run")
	}
}

func TestMoveToTrashCreatesTrashDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Do NOT create .Trash — MoveToTrash should create it.
	src := filepath.Join(home, "auto-trash-test")
	os.WriteFile(src, []byte("data"), 0o600)

	dst, err := MoveToTrash(src, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Errorf("expected file in trash at %s", dst)
	}
}

func TestMoveToTrashMkdirAllError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Block .Trash creation by placing a read-only file at its path.
	trashPath := filepath.Join(home, ".Trash")
	os.WriteFile(trashPath, []byte("blocker"), 0o400)
	defer os.Chmod(trashPath, 0o600)

	src := filepath.Join(home, "move-test")
	os.WriteFile(src, []byte("data"), 0o600)

	_, err := MoveToTrash(src, false)
	if err == nil {
		t.Error("expected error from MkdirAll with file blocking trash dir")
	}
}

func TestRestoreProtectedSrcPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trash := filepath.Join(home, ".Trash")
	os.MkdirAll(trash, 0o700)
	trashItem := filepath.Join(trash, "app")
	os.WriteFile(trashItem, []byte("data"), 0o600)

	// src = home itself — passes isWithinRoot(home, home) but
	// ValidatePathSafety refuses to delete the home directory.
	manifest := &UndoManifest{
		Version: 1,
		Entries: []UndoEntry{{SrcPath: home, DstPath: trashItem, Bytes: 4}},
	}
	restored, failed, _ := Restore(context.Background(), manifest)
	if restored != 0 {
		t.Error("should not restore to home dir")
	}
	if len(failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(failed))
	}
}

func TestRestoreSrcNotWithinHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trash := filepath.Join(home, ".Trash")
	os.MkdirAll(trash, 0o700)
	trashItem := filepath.Join(trash, "app")
	os.WriteFile(trashItem, []byte("data"), 0o600)

	manifest := &UndoManifest{
		Version: 1,
		Entries: []UndoEntry{{SrcPath: "/Applications", DstPath: trashItem, Bytes: 4}},
	}
	restored, failed, _ := Restore(context.Background(), manifest)
	if restored != 0 {
		t.Error("should not restore to path outside home")
	}
	if len(failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(failed))
	}
}

func TestRestoreSymlinkAncestorInDstDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trash := filepath.Join(home, ".Trash")
	os.MkdirAll(trash, 0o700)

	// Create a symlink inside trash.
	realDir := filepath.Join(home, "realtrash")
	os.MkdirAll(realDir, 0o700)
	linkDir := filepath.Join(trash, "link")
	os.Symlink(realDir, linkDir)

	trashItem := filepath.Join(linkDir, "item")
	os.WriteFile(filepath.Join(realDir, "item"), []byte("data"), 0o600)

	manifest := &UndoManifest{
		Version: 1,
		Entries: []UndoEntry{{
			SrcPath: filepath.Join(home, "Documents", "item"),
			DstPath: trashItem,
			Bytes:   4,
		}},
	}
	restored, failed, _ := Restore(context.Background(), manifest)
	if restored != 0 {
		t.Error("should not restore from symlinked dst dir")
	}
	if len(failed) != 1 {
		t.Error("expected 1 failed for symlink in dst dir")
	}
}
