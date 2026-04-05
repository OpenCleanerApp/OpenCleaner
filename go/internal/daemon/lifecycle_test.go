package daemon

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPlistPath(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("launchd is macOS-specific")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	got := PlistPath()
	want := filepath.Join(home, "Library", "LaunchAgents", "com.opencleaner.daemon.plist")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestPlistTemplateRendering(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("launchd is macOS-specific")
	}
	b, err := renderPlist(plistConfig{
		Label:      defaultLabel,
		BinaryPath: "/usr/local/bin/opencleanerd",
		SocketPath: "/tmp/opencleaner.sock",
		StdoutPath: "/tmp/daemon.log",
		StderrPath: "/tmp/daemon.err",
		RunAtLoad:  true,
		KeepAlive:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "<string>com.opencleaner.daemon</string>") {
		t.Fatalf("expected label in plist, got:\n%s", s)
	}
	if !strings.Contains(s, "--socket=/tmp/opencleaner.sock") {
		t.Fatalf("expected socket arg in plist, got:\n%s", s)
	}
	if !strings.Contains(s, "<key>KeepAlive</key>") || !strings.Contains(s, "<true/>") {
		t.Fatalf("expected KeepAlive true in plist, got:\n%s", s)
	}
}

func TestIsInstalledFalseByDefault(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("launchd is macOS-specific")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	if IsInstalled() {
		t.Fatal("expected not installed")
	}
}
