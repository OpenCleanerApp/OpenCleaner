package cleaner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMoveToTrashDryRunDoesNotMove(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	p := filepath.Join(tmp, "file.txt")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	dst, err := MoveToTrash(p, true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if dst == "" {
		t.Fatalf("expected dst to be returned")
	}

	trash, err := TrashDir()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(dst) != trash {
		t.Fatalf("expected dst under %q, got %q", trash, dst)
	}

	if _, err := os.Lstat(p); err != nil {
		t.Fatalf("expected original file to remain, got: %v", err)
	}
}

func TestMoveToTrashCollisionUsesUniqueName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trash := filepath.Join(home, ".Trash")
	if err := os.MkdirAll(trash, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(trash, "file.txt"), []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}

	srcDir := filepath.Join(home, "src")
	if err := os.MkdirAll(srcDir, 0o700); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(srcDir, "file.txt")
	if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	dst, err := MoveToTrash(src, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if dst == filepath.Join(trash, "file.txt") {
		t.Fatalf("expected dst to avoid collision, got %q", dst)
	}
	if _, err := os.Lstat(dst); err != nil {
		t.Fatalf("expected dst to exist, got: %v", err)
	}
	if _, err := os.Lstat(src); err == nil {
		t.Fatalf("expected src to be moved")
	}
}
