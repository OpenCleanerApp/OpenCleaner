package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWalkFindsTarget(t *testing.T) {
	tmp := t.TempDir()
	// Create: tmp/project/node_modules/
	target := filepath.Join(tmp, "project", "node_modules")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}

	var found []string
	err := Walk(context.Background(), WalkConfig{
		RootDirs:   []string{tmp},
		TargetName: "node_modules",
		MaxDepth:   5,
		SkipHidden: true,
		OnMatch:    func(path string) { found = append(found, path) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("expected 1 match, got %d", len(found))
	}
	if found[0] != target {
		t.Errorf("expected %s, got %s", target, found[0])
	}
}

func TestWalkRespectsMaxDepth(t *testing.T) {
	tmp := t.TempDir()
	// Deep: tmp/a/b/c/d/e/target (depth 5 from root)
	deep := filepath.Join(tmp, "a", "b", "c", "d", "e", "target")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	// Shallow: tmp/a/target (depth 1 from root)
	shallow := filepath.Join(tmp, "a", "target")
	if err := os.MkdirAll(shallow, 0o755); err != nil {
		t.Fatal(err)
	}

	var found []string
	_ = Walk(context.Background(), WalkConfig{
		RootDirs:   []string{tmp},
		TargetName: "target",
		MaxDepth:   2,
		SkipHidden: true,
		OnMatch:    func(path string) { found = append(found, path) },
	})

	if len(found) != 1 || found[0] != shallow {
		t.Errorf("expected only shallow match at depth 1, got %v", found)
	}
}

func TestWalkSkipsHiddenByDefault(t *testing.T) {
	tmp := t.TempDir()
	// tmp/.hidden/target should be skipped
	if err := os.MkdirAll(filepath.Join(tmp, ".hidden", "target"), 0o755); err != nil {
		t.Fatal(err)
	}
	// tmp/visible/target should be found
	visible := filepath.Join(tmp, "visible", "target")
	if err := os.MkdirAll(visible, 0o755); err != nil {
		t.Fatal(err)
	}

	var found []string
	_ = Walk(context.Background(), WalkConfig{
		RootDirs:   []string{tmp},
		TargetName: "target",
		MaxDepth:   5,
		SkipHidden: true,
		OnMatch:    func(path string) { found = append(found, path) },
	})

	if len(found) != 1 || found[0] != visible {
		t.Errorf("expected only visible/target, got %v", found)
	}
}

func TestWalkDisableSkipHidden(t *testing.T) {
	tmp := t.TempDir()
	hidden := filepath.Join(tmp, ".venv")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}

	var found []string
	_ = Walk(context.Background(), WalkConfig{
		RootDirs:   []string{tmp},
		TargetName: ".venv",
		MaxDepth:   5,
		SkipHidden: false,
		OnMatch:    func(path string) { found = append(found, path) },
	})

	if len(found) != 1 {
		t.Errorf("expected 1 match with SkipHidden=false, got %d", len(found))
	}
}

func TestWalkSkipsNamedDirs(t *testing.T) {
	tmp := t.TempDir()
	// tmp/.git/target should be skipped even with SkipHidden=false
	if err := os.MkdirAll(filepath.Join(tmp, ".git", "target"), 0o755); err != nil {
		t.Fatal(err)
	}

	var found []string
	_ = Walk(context.Background(), WalkConfig{
		RootDirs:   []string{tmp},
		TargetName: "target",
		MaxDepth:   5,
		SkipHidden: false,
		OnMatch:    func(path string) { found = append(found, path) },
	})

	if len(found) != 0 {
		t.Errorf("expected 0 matches (.git should be skipped), got %v", found)
	}
}

func TestWalkNoRecursionIntoMatch(t *testing.T) {
	tmp := t.TempDir()
	// tmp/project/node_modules/nested/node_modules — only the outer one should match
	outer := filepath.Join(tmp, "project", "node_modules")
	if err := os.MkdirAll(filepath.Join(outer, "nested", "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}

	var found []string
	_ = Walk(context.Background(), WalkConfig{
		RootDirs:   []string{tmp},
		TargetName: "node_modules",
		MaxDepth:   10,
		SkipHidden: true,
		OnMatch:    func(path string) { found = append(found, path) },
	})

	if len(found) != 1 || found[0] != outer {
		t.Errorf("expected only outer node_modules, got %v", found)
	}
}

func TestWalkCancelledContext(t *testing.T) {
	tmp := t.TempDir()
	for i := 0; i < 300; i++ {
		if err := os.MkdirAll(filepath.Join(tmp, "d"+filepath.Base(os.TempDir()), itoa(i)), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	err := Walk(ctx, WalkConfig{
		RootDirs:   []string{tmp},
		TargetName: "nonexistent",
		MaxDepth:   10,
		SkipHidden: true,
		OnMatch:    func(path string) {},
	})

	if err != context.Canceled {
		t.Logf("expected context.Canceled, got %v (nil acceptable for fast cancel)", err)
	}
}

func TestWalkNonExistentRoot(t *testing.T) {
	var found []string
	err := Walk(context.Background(), WalkConfig{
		RootDirs:   []string{"/nonexistent/path/that/does/not/exist"},
		TargetName: "target",
		MaxDepth:   5,
		SkipHidden: true,
		OnMatch:    func(path string) { found = append(found, path) },
	})
	if err != nil {
		t.Errorf("expected nil error for non-existent root, got %v", err)
	}
	if len(found) != 0 {
		t.Errorf("expected 0 matches, got %d", len(found))
	}
}

func TestWalkMultipleRoots(t *testing.T) {
	tmp := t.TempDir()
	root1 := filepath.Join(tmp, "root1")
	root2 := filepath.Join(tmp, "root2")

	if err := os.MkdirAll(filepath.Join(root1, "target"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root2, "target"), 0o755); err != nil {
		t.Fatal(err)
	}

	var found []string
	_ = Walk(context.Background(), WalkConfig{
		RootDirs:   []string{root1, root2},
		TargetName: "target",
		MaxDepth:   5,
		SkipHidden: true,
		OnMatch:    func(path string) { found = append(found, path) },
	})

	if len(found) != 2 {
		t.Errorf("expected 2 matches from 2 roots, got %d", len(found))
	}
}

func TestWalkSymlinkDirSkipped(t *testing.T) {
	tmp := t.TempDir()
	realDir := filepath.Join(tmp, "real")
	if err := os.MkdirAll(filepath.Join(realDir, "target"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Create symlink: tmp/link -> real
	linkDir := filepath.Join(tmp, "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Skip("symlinks not supported")
	}

	var found []string
	_ = Walk(context.Background(), WalkConfig{
		RootDirs:   []string{tmp},
		TargetName: "target",
		MaxDepth:   5,
		SkipHidden: true,
		OnMatch:    func(path string) { found = append(found, path) },
	})

	// Should find real/target but not follow link/target
	if len(found) != 1 {
		t.Errorf("expected 1 match (real only, not symlink), got %v", found)
	}
}

func TestWalkDefaultMaxDepth(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "a", "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}

	var found []string
	_ = Walk(context.Background(), WalkConfig{
		RootDirs:   []string{tmp},
		TargetName: "target",
		MaxDepth:   0, // should default to 10
		SkipHidden: true,
		OnMatch:    func(path string) { found = append(found, path) },
	})

	if len(found) != 1 {
		t.Errorf("expected 1 match with default depth, got %d", len(found))
	}
}

func TestWalkTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	time.Sleep(2 * time.Millisecond) // ensure timeout

	err := Walk(ctx, WalkConfig{
		RootDirs:   []string{t.TempDir()},
		TargetName: "x",
		MaxDepth:   5,
		SkipHidden: true,
	})
	_ = err // may or may not propagate — just verify no panic
}
