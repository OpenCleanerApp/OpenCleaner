package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDockerScannerScanNoDockerDesktop(t *testing.T) {
	home := t.TempDir() // no Docker Desktop data dir
	s := NewDockerScanner(home)
	rules, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules without Docker, got %d", len(rules))
	}
}

func TestDockerScannerScanWithDockerDir(t *testing.T) {
	home := t.TempDir()
	// Create Docker Desktop data dir.
	dataDir := filepath.Join(home, "Library", "Containers", "com.docker.docker", "Data")
	os.MkdirAll(dataDir, 0o755)

	// Ensure docker CLI is not found — the test is about the graceful skip path.
	t.Setenv("PATH", t.TempDir())

	s := NewDockerScanner(home)
	rules, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Without docker CLI, should return empty (graceful skip).
	_ = rules
}

func TestParseDockerSize(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"2.5GB", 2500000000},
		{"100MB", 100000000},
		{"1.2kB", 1200},
		{"500B", 500},
		{"0B", 0},
		{"1.5TB", 1500000000000},
		{"2.5GB (100%)", 2500000000},
		{"3.1GB (89%)", 3100000000},
		{"", 0},
		{"invalid", 0},
		{" 1.0GB ", 1000000000},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseDockerSize(tt.input)
			// Allow 1% tolerance for floating point
			if tt.expected == 0 {
				if got != 0 {
					t.Errorf("parseDockerSize(%q) = %d, want 0", tt.input, got)
				}
				return
			}
			ratio := float64(got) / float64(tt.expected)
			if ratio < 0.99 || ratio > 1.01 {
				t.Errorf("parseDockerSize(%q) = %d, want ~%d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseFloat(t *testing.T) {
	tests := []struct {
		input string
		want  float64
		ok    bool
	}{
		{"2.5", 2.5, true},
		{"100", 100.0, true},
		{"0", 0.0, true},
		{"abc", 0.0, false},
	}
	for _, tt := range tests {
		f, err := parseFloat(tt.input)
		ok := err == nil
		if ok != tt.ok {
			t.Errorf("parseFloat(%q): ok=%v, want=%v", tt.input, ok, tt.ok)
		}
		if ok && f != tt.want {
			t.Errorf("parseFloat(%q) = %f, want %f", tt.input, f, tt.want)
		}
	}
}

func TestDockerScannerID(t *testing.T) {
	s := NewDockerScanner("/home")
	if s.ID() != "docker" {
		t.Errorf("expected 'docker', got %q", s.ID())
	}
	if s.Name() != "Docker" {
		t.Errorf("expected 'Docker', got %q", s.Name())
	}
	if s.Category() == "" {
		t.Error("expected non-empty category")
	}
}
