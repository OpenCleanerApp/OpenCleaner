package scanner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPythonScannerID(t *testing.T) {
	s := NewPythonScanner("/home", nil)
	if s.ID() != "python" {
		t.Errorf("expected 'python', got %q", s.ID())
	}
	if s.Name() == "" {
		t.Error("expected non-empty name")
	}
	if s.Category() == "" {
		t.Error("expected non-empty category")
	}
}

func TestPythonScannerFindsPycache(t *testing.T) {
	tmp := t.TempDir()
	pc := filepath.Join(tmp, "myproject", "__pycache__")
	if err := os.MkdirAll(pc, 0o755); err != nil {
		t.Fatal(err)
	}

	s := NewPythonScanner(tmp, []string{tmp})
	rules, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, r := range rules {
		if r.Path == pc {
			found = true
			if r.Safety != "safe" {
				t.Errorf("expected safe for __pycache__, got %s", r.Safety)
			}
		}
	}
	if !found {
		t.Error("__pycache__ not found in scan results")
	}
}

func TestPythonScannerFindsVenvWithCfg(t *testing.T) {
	tmp := t.TempDir()
	venv := filepath.Join(tmp, "project", ".venv")
	if err := os.MkdirAll(venv, 0o755); err != nil {
		t.Fatal(err)
	}
	// Must have pyvenv.cfg
	if err := os.WriteFile(filepath.Join(venv, "pyvenv.cfg"), []byte("home = /usr/bin"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewPythonScanner(tmp, []string{tmp})
	rules, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, r := range rules {
		if r.Path == venv {
			found = true
			if r.Safety != "moderate" {
				t.Errorf("expected moderate for .venv, got %s", r.Safety)
			}
		}
	}
	if !found {
		t.Error(".venv with pyvenv.cfg not found in scan results")
	}
}

func TestPythonScannerSkipsVenvWithoutCfg(t *testing.T) {
	tmp := t.TempDir()
	venv := filepath.Join(tmp, "project", ".venv")
	if err := os.MkdirAll(venv, 0o755); err != nil {
		t.Fatal(err)
	}
	// No pyvenv.cfg — should be skipped

	s := NewPythonScanner(tmp, []string{tmp})
	rules, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range rules {
		if r.Path == venv {
			t.Error(".venv without pyvenv.cfg should be skipped")
		}
	}
}

func TestPythonScannerFindsVenv(t *testing.T) {
	tmp := t.TempDir()
	venv := filepath.Join(tmp, "project", "venv")
	if err := os.MkdirAll(venv, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(venv, "pyvenv.cfg"), []byte("home = /usr/bin"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewPythonScanner(tmp, []string{tmp})
	rules, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, r := range rules {
		if r.Path == venv {
			found = true
		}
	}
	if !found {
		t.Error("venv/ with pyvenv.cfg not found in scan results")
	}
}

func TestIsVirtualenv(t *testing.T) {
	tmp := t.TempDir()

	// With pyvenv.cfg
	withCfg := filepath.Join(tmp, "with")
	os.MkdirAll(withCfg, 0o755)
	os.WriteFile(filepath.Join(withCfg, "pyvenv.cfg"), []byte(""), 0o644)
	if !isVirtualenv(withCfg) {
		t.Error("expected true for dir with pyvenv.cfg")
	}

	// Without pyvenv.cfg
	withoutCfg := filepath.Join(tmp, "without")
	os.MkdirAll(withoutCfg, 0o755)
	if isVirtualenv(withoutCfg) {
		t.Error("expected false for dir without pyvenv.cfg")
	}
}

func TestPythonScannerFindsPipCache(t *testing.T) {
	tmp := t.TempDir()
	pipCache := filepath.Join(tmp, "Library", "Caches", "pip")
	if err := os.MkdirAll(pipCache, 0o755); err != nil {
		t.Fatal(err)
	}

	s := NewPythonScanner(tmp, nil) // no scan roots, just known paths
	rules, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, r := range rules {
		if r.ID == "python-pip-cache" {
			found = true
		}
	}
	if !found {
		t.Error("pip cache not found")
	}
}

func TestPythonScanVenvNotVirtualenv(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(home, "proj")
	// Create a venv/ dir that is NOT a virtualenv (no pyvenv.cfg).
	os.MkdirAll(filepath.Join(project, "venv"), 0o700)
	// Create a .venv/ dir that is NOT a virtualenv.
	os.MkdirAll(filepath.Join(project, ".venv"), 0o700)

	s := NewPythonScanner(home, []string{home})
	rules, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rules {
		if strings.Contains(r.ID, "venv") {
			t.Errorf("should not find venv without pyvenv.cfg: %s", r.ID)
		}
	}
}
