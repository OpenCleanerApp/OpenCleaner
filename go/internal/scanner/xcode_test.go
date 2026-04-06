package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestXcodeScannerID(t *testing.T) {
	s := NewXcodeScanner("/home")
	if s.ID() != "xcode" {
		t.Errorf("expected 'xcode', got %q", s.ID())
	}
}

func TestXcodeScannerFindsKnownPaths(t *testing.T) {
	tmp := t.TempDir()

	// Create simulator runtimes dir
	simRuntimes := filepath.Join(tmp, "Library", "Developer", "CoreSimulator", "Profiles", "Runtimes")
	if err := os.MkdirAll(simRuntimes, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create SwiftUI previews dir
	previews := filepath.Join(tmp, "Library", "Developer", "Xcode", "UserData", "Previews")
	if err := os.MkdirAll(previews, 0o755); err != nil {
		t.Fatal(err)
	}

	s := NewXcodeScanner(tmp)
	rules, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	ids := map[string]bool{}
	for _, r := range rules {
		ids[r.ID] = true
	}

	if !ids["xcode-simulator-runtimes"] {
		t.Error("xcode-simulator-runtimes not found")
	}
	if !ids["xcode-previews"] {
		t.Error("xcode-previews not found")
	}
}

func TestXcodeScannerPerVersionDeviceSupport(t *testing.T) {
	tmp := t.TempDir()
	iosDS := filepath.Join(tmp, "Library", "Developer", "Xcode", "iOS DeviceSupport")

	// Create version dirs
	for _, ver := range []string{"17.0", "16.4", "15.5"} {
		if err := os.MkdirAll(filepath.Join(iosDS, ver), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Create a file (should be ignored)
	os.WriteFile(filepath.Join(iosDS, ".DS_Store"), []byte{}, 0o644)

	s := NewXcodeScanner(tmp)
	rules, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	versionRules := 0
	for _, r := range rules {
		if r.Safety == "moderate" && r.Category == "developer" {
			versionRules++
		}
	}
	if versionRules != 3 {
		t.Errorf("expected 3 version-specific rules, got %d", versionRules)
	}
}

func TestXcodeScannerNoXcode(t *testing.T) {
	tmp := t.TempDir()
	// Nothing created — no Xcode dirs

	s := NewXcodeScanner(tmp)
	rules, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules when no Xcode dirs exist, got %d", len(rules))
	}
}
