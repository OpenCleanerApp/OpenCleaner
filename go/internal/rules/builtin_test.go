package rules

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencleaner/opencleaner/internal/safety"
)

func TestBuiltinRules_Invariants(t *testing.T) {
	home := t.TempDir()
	rs := BuiltinRules(home)
	if len(rs) == 0 {
		t.Fatal("expected builtin rules")
	}

	seen := map[string]struct{}{}
	for _, r := range rs {
		if r.ID == "" {
			t.Fatalf("rule has empty ID: %+v", r)
		}
		if r.Name == "" {
			t.Fatalf("rule %q has empty Name", r.ID)
		}
		if r.Path == "" {
			t.Fatalf("rule %q has empty Path", r.ID)
		}
		if r.Category == "" {
			t.Fatalf("rule %q has empty Category", r.ID)
		}
		if r.Safety == "" {
			t.Fatalf("rule %q has empty Safety", r.ID)
		}
		if strings.TrimSpace(r.SafetyNote) == "" {
			t.Fatalf("rule %q has empty SafetyNote", r.ID)
		}
		if strings.TrimSpace(r.Desc) == "" {
			t.Fatalf("rule %q has empty Desc", r.ID)
		}

		if _, ok := seen[r.ID]; ok {
			t.Fatalf("duplicate rule ID %q", r.ID)
		}
		seen[r.ID] = struct{}{}

		if !filepath.IsAbs(r.Path) {
			t.Fatalf("rule %q has non-absolute path %q", r.ID, r.Path)
		}
		rel, err := filepath.Rel(home, r.Path)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			t.Fatalf("rule %q path not under home: home=%q path=%q rel=%q err=%v", r.ID, home, r.Path, rel, err)
		}
		if err := safety.ValidatePathSafety(r.Path); err != nil {
			t.Fatalf("rule %q path rejected by safety guard: %v", r.ID, err)
		}
	}
}
