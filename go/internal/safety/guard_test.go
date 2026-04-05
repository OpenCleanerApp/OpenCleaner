package safety

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasTraversalPattern(t *testing.T) {
	cases := []struct {
		p    string
		want bool
	}{
		{"..", true},
		{"../x", true},
		{"/a/../b", true},
		{"/a/b/..", true},
		{"/a/b", false},
		{"/a/..foo/b", false},
		{"..foo", false},
		{"/..foo", false},
		{"", false},
	}

	for _, tc := range cases {
		if got := HasTraversalPattern(tc.p); got != tc.want {
			t.Fatalf("HasTraversalPattern(%q)=%v want %v", tc.p, got, tc.want)
		}
	}
}

func TestValidatePathSafetyRefusals(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{"", "relative/path", "/", filepath.Clean(home), "/System", "/usr"} {
		if err := ValidatePathSafety(p); err == nil {
			t.Fatalf("expected ValidatePathSafety(%q) to error", p)
		}
	}
}

func TestValidatePathSafetyAllowedTemp(t *testing.T) {
	p := "/tmp/opencleaner-test-safe"
	if err := ValidatePathSafety(p); err != nil {
		t.Fatalf("expected /tmp path to be allowed, got err: %v", err)
	}
}

func TestValidatePathSafetyHomeNeverTouchAndConfig(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	for _, rel := range []string{"Documents", "Desktop", "Downloads", ".config"} {
		p := filepath.Join(home, rel)
		if err := ValidatePathSafety(p); err == nil {
			t.Fatalf("expected %q to be protected", p)
		}
	}
}

func TestResolveForNonSymlinkRejectsSymlinkedAncestorIntoProtected(t *testing.T) {
	tmp := t.TempDir()
	sys := filepath.Join(tmp, "sys")
	if err := os.Symlink("/System", sys); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	// This path is outside protected prefixes as a string, but resolves into /System.
	p := filepath.Join(sys, "Library")
	if _, _, err := ResolveForNonSymlink(p); err == nil {
		t.Fatalf("expected resolved path under /System to be rejected")
	}
}

func TestResolveForNonSymlinkLeafSymlinkDoesNotResolveTarget(t *testing.T) {
	tmp := t.TempDir()
	link := filepath.Join(tmp, "link")
	if err := os.Symlink("/System/Library", link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	resolved, isLink, err := ResolveForNonSymlink(link)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !isLink {
		t.Fatalf("expected leafIsSymlink")
	}
	if resolved != link {
		t.Fatalf("expected resolved==original for leaf symlink, got %q", resolved)
	}
}

func TestSafeRemoveDryRunDoesNotRemove(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "file.txt")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := SafeRemove(p, true); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, err := os.Lstat(p); err != nil {
		t.Fatalf("expected file to still exist, got: %v", err)
	}
}

func TestSafeRemoveUnlinksSymlinkOnly(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target.txt")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	if err := SafeRemove(link, false); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("expected symlink to be removed, got: %v", err)
	}
	if _, err := os.Lstat(target); err != nil {
		t.Fatalf("expected target to remain, got: %v", err)
	}
}

func TestSafeRemoveRemovesRegularFile(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "file.txt")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := SafeRemove(p, false); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, err := os.Lstat(p); !os.IsNotExist(err) {
		t.Fatalf("expected file removed, got: %v", err)
	}
}
