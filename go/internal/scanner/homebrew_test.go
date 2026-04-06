package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHomebrewScannerID(t *testing.T) {
	s := NewHomebrewScanner("/home")
	if s.ID() != "homebrew" {
		t.Errorf("expected 'homebrew', got %q", s.ID())
	}
	if s.Name() == "" {
		t.Error("expected non-empty name")
	}
	if s.Category() == "" {
		t.Error("expected non-empty category")
	}
}

func TestHomebrewScannerScan(t *testing.T) {
	home := t.TempDir()
	// Create known homebrew paths.
	cachePath := filepath.Join(home, "Library", "Caches", "Homebrew")
	logsPath := filepath.Join(home, "Library", "Logs", "Homebrew")
	os.MkdirAll(cachePath, 0o755)
	os.MkdirAll(logsPath, 0o755)

	s := NewHomebrewScanner(home)
	rules, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) < 2 {
		t.Errorf("expected at least 2 rules (cache + logs), got %d", len(rules))
	}
}

func TestHomebrewScannerScanEmpty(t *testing.T) {
	home := t.TempDir() // no homebrew paths
	s := NewHomebrewScanner(home)
	rules, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules for empty home, got %d", len(rules))
	}
}

func TestFormatCount(t *testing.T) {
	tests := []struct {
		n        int
		singular string
		want     string
	}{
		{1, "item", "1 item"},
		{5, "item", "5 items"},
		{0, "file", "0 files"},
	}
	for _, tt := range tests {
		got := formatCount(tt.n, tt.singular)
		if got != tt.want {
			t.Errorf("formatCount(%d, %q) = %q, want %q", tt.n, tt.singular, got, tt.want)
		}
	}
}

func TestHomebrewScanNoBrewCLI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PATH", t.TempDir())

	cacheDir := filepath.Join(home, "Library", "Caches", "Homebrew")
	os.MkdirAll(cacheDir, 0o700)

	s := NewHomebrewScanner(home)
	rules, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Should find the static cache dir even without brew CLI.
	if len(rules) == 0 {
		t.Error("expected cache dir to be found via filesystem scan")
	}
}
