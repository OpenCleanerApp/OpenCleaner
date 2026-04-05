package cleaner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMoveToTrashDryRunDoesNotMove(t *testing.T) {
	tmp := t.TempDir()
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
