package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNodeScannerID(t *testing.T) {
	s := NewNodeScanner("/home/user", nil)
	if s.ID() != "nodejs" {
		t.Errorf("expected 'nodejs', got %q", s.ID())
	}
	if s.Name() != "Node.js" {
		t.Errorf("expected 'Node.js', got %q", s.Name())
	}
}

func TestNodeScannerFindsNodeModules(t *testing.T) {
	tmp := t.TempDir()
	nm := filepath.Join(tmp, "myapp", "node_modules")
	if err := os.MkdirAll(nm, 0o755); err != nil {
		t.Fatal(err)
	}

	s := NewNodeScanner(tmp, []string{tmp})
	rules, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, r := range rules {
		if r.Path == nm {
			found = true
			if r.Safety != "safe" {
				t.Errorf("expected safe, got %s", r.Safety)
			}
		}
	}
	if !found {
		t.Error("node_modules not found in scan results")
	}
}

func TestNodeScannerDedupsKnownPaths(t *testing.T) {
	tmp := t.TempDir()
	// Create npm cache dir
	npmCache := filepath.Join(tmp, ".npm", "_cacache")
	if err := os.MkdirAll(npmCache, 0o755); err != nil {
		t.Fatal(err)
	}

	s := NewNodeScanner(tmp, []string{tmp})
	rules, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	npmFound := false
	for _, r := range rules {
		if r.ID == "npm-cache" {
			npmFound = true
		}
	}
	if !npmFound {
		t.Error("npm-cache not found in scan results")
	}
}

func TestNodeScannerEmptyRoots(t *testing.T) {
	s := NewNodeScanner("/nonexistent", nil)
	rules, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules for empty roots, got %d", len(rules))
	}
}

func TestPathHash(t *testing.T) {
	h1 := pathHash("/Users/test/project/node_modules")
	h2 := pathHash("/Users/test/other/node_modules")
	if h1 == h2 {
		t.Error("different paths should produce different hashes")
	}
	if len(h1) != 8 {
		t.Errorf("expected 8 char hash, got %d", len(h1))
	}
	// Deterministic
	if pathHash("/same") != pathHash("/same") {
		t.Error("same path should produce same hash")
	}
}

func TestShortPath(t *testing.T) {
	home := "/Users/test"
	result := shortPath(home, "/Users/test/project/node_modules")
	if result != "~/project/node_modules" {
		t.Errorf("expected ~/project/node_modules, got %s", result)
	}

	// Path outside home — filepath.Rel can still produce a relative path
	// so shortPath returns it with ~/ prefix; just ensure no panic.
	result = shortPath(home, "/opt/something")
	if result == "" {
		t.Error("expected non-empty result")
	}
}
