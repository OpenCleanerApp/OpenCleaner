package safety

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateNoSymlinkAncestors_EmptyRoot(t *testing.T) {
	err := ValidateNoSymlinkAncestorsWithin("", "/tmp/foo")
	if err == nil {
		t.Error("expected error for empty root")
	}
}

func TestValidateNoSymlinkAncestors_EmptyPath(t *testing.T) {
	err := ValidateNoSymlinkAncestorsWithin("/tmp", "")
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestValidateNoSymlinkAncestors_RelativePaths(t *testing.T) {
	err := ValidateNoSymlinkAncestorsWithin("tmp", "/foo")
	if err == nil {
		t.Error("expected error for relative root")
	}
	err = ValidateNoSymlinkAncestorsWithin("/tmp", "foo")
	if err == nil {
		t.Error("expected error for relative path")
	}
}

func TestValidateNoSymlinkAncestors_SamePath(t *testing.T) {
	tmp := t.TempDir()
	err := ValidateNoSymlinkAncestorsWithin(tmp, tmp)
	if err != nil {
		t.Errorf("expected nil for same path, got %v", err)
	}
}

func TestValidateNoSymlinkAncestors_PathOutsideRoot(t *testing.T) {
	err := ValidateNoSymlinkAncestorsWithin("/tmp/a", "/tmp/b")
	if err == nil {
		t.Error("expected error for path outside root")
	}
}

func TestValidateNoSymlinkAncestors_NoSymlinks(t *testing.T) {
	tmp := t.TempDir()
	child := filepath.Join(tmp, "a", "b", "c")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	err := ValidateNoSymlinkAncestorsWithin(tmp, child)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestValidateNoSymlinkAncestors_SymlinkAncestor(t *testing.T) {
	tmp := t.TempDir()
	real := filepath.Join(tmp, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	err := ValidateNoSymlinkAncestorsWithin(tmp, filepath.Join(link, "child"))
	if err == nil {
		t.Error("expected error for symlink ancestor")
	}
	if !strings.Contains(err.Error(), "symlink ancestor") {
		t.Errorf("expected symlink ancestor error, got %v", err)
	}
}

func TestValidateNoSymlinkAncestors_NonExistentTail(t *testing.T) {
	tmp := t.TempDir()
	// Path doesn't fully exist — should pass (remaining components not checked).
	err := ValidateNoSymlinkAncestorsWithin(tmp, filepath.Join(tmp, "nonexistent", "deep", "path"))
	if err != nil {
		t.Errorf("expected nil for non-existent tail, got %v", err)
	}
}

func TestValidateNoSymlinkAncestors_TraversalPattern(t *testing.T) {
	err := ValidateNoSymlinkAncestorsWithin("/tmp", "/tmp/../etc/passwd")
	if err == nil {
		t.Error("expected error for traversal pattern")
	}
}
