package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRustScannerID(t *testing.T) {
	s := NewRustScanner("/home", nil)
	if s.ID() != "rust" {
		t.Errorf("expected 'rust', got %q", s.ID())
	}
	if s.Name() == "" {
		t.Error("expected non-empty name")
	}
	if s.Category() == "" {
		t.Error("expected non-empty category")
	}
}

func TestRustScannerFindsTargetWithCargoToml(t *testing.T) {
	tmp := t.TempDir()
	project := filepath.Join(tmp, "myrust")
	targetDir := filepath.Join(project, "target")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "Cargo.toml"), []byte("[package]\nname = \"test\""), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewRustScanner(tmp, []string{tmp})
	rules, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, r := range rules {
		if r.Path == targetDir {
			found = true
			if r.Safety != "safe" {
				t.Errorf("expected safe, got %s", r.Safety)
			}
		}
	}
	if !found {
		t.Error("target/ with Cargo.toml not found")
	}
}

func TestRustScannerSkipsTargetWithoutCargoToml(t *testing.T) {
	tmp := t.TempDir()
	targetDir := filepath.Join(tmp, "notrust", "target")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// No Cargo.toml

	s := NewRustScanner(tmp, []string{tmp})
	rules, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range rules {
		if r.Path == targetDir {
			t.Error("target/ without Cargo.toml should be skipped")
		}
	}
}

func TestRustScannerKnownPaths(t *testing.T) {
	tmp := t.TempDir()
	cargoReg := filepath.Join(tmp, ".cargo", "registry")
	if err := os.MkdirAll(cargoReg, 0o755); err != nil {
		t.Fatal(err)
	}

	s := NewRustScanner(tmp, nil)
	rules, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, r := range rules {
		if r.ID == "cargo-registry" {
			found = true
			if r.Safety != "moderate" {
				t.Errorf("expected moderate for cargo-registry, got %s", r.Safety)
			}
		}
	}
	if !found {
		t.Error("cargo-registry not found")
	}
}
