package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultScanRoots_WithSpecificDirs(t *testing.T) {
	home := t.TempDir()
	projects := filepath.Join(home, "Projects")
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatal(err)
	}
	developer := filepath.Join(home, "Developer")
	if err := os.MkdirAll(developer, 0o755); err != nil {
		t.Fatal(err)
	}

	roots := DefaultScanRoots(home)
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots, got %d: %v", len(roots), roots)
	}
	for _, r := range roots {
		if r != projects && r != developer {
			t.Errorf("unexpected root: %s", r)
		}
	}
	for _, r := range roots {
		if r == home {
			t.Error("home should NOT be included when specific dirs exist")
		}
	}
}

func TestDefaultScanRoots_FallbackToHome(t *testing.T) {
	home := t.TempDir()
	roots := DefaultScanRoots(home)
	if len(roots) != 1 || roots[0] != home {
		t.Fatalf("expected [%s], got %v", home, roots)
	}
}

func TestDefaultScanRoots_AllKnownDirs(t *testing.T) {
	home := t.TempDir()
	dirs := []string{"Projects", "Developer", "src", filepath.Join("go", "src"), "workspace", "code"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	roots := DefaultScanRoots(home)
	if len(roots) != len(dirs) {
		t.Fatalf("expected %d roots, got %d: %v", len(dirs), len(roots), roots)
	}
}

func TestRunCommand(t *testing.T) {
	out, err := RunCommand(context.Background(), "echo", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello\n" && out != "hello" {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestRunCommandNotFound(t *testing.T) {
	// RunCommand gracefully skips when binary not found: returns ("", nil).
	out, err := RunCommand(context.Background(), "nonexistent-command-xyz-12345")
	if err != nil {
		t.Fatalf("expected nil error for not-found (graceful skip), got %v", err)
	}
	if out != "" {
		t.Fatalf("expected empty output, got %q", out)
	}
}

func TestRunCommandCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := RunCommand(ctx, "sleep", "10")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
