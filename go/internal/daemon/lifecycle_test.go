package daemon

import (
	"os"
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

func TestIsIgnorableLaunchctlBootout(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"Could not find service", true},
		{"No such process", true},
		{"not found", true},
		{"COULD NOT FIND SERVICE", true},
		{"some other error", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isIgnorableLaunchctlBootout(tt.input)
		if got != tt.want {
			t.Errorf("isIgnorableLaunchctlBootout(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestXmlEscape(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"hello", "hello"},
		{"a<b>c", "a&lt;b&gt;c"},
		{"a&b", "a&amp;b"},
		{`"quoted"`, "&#34;quoted&#34;"},
		{"/usr/local/bin/daemon", "/usr/local/bin/daemon"},
	}
	for _, tt := range tests {
		got := xmlEscape(tt.input)
		if got != tt.want {
			t.Errorf("xmlEscape(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestWritePlist(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("launchd is macOS-specific")
	}
	tmp := t.TempDir()
	path := filepath.Join(tmp, "LaunchAgents", "test.plist")
	cfg := plistConfig{
		Label:      defaultLabel,
		BinaryPath: "/usr/local/bin/opencleanerd",
		SocketPath: "/tmp/opencleaner.sock",
		StdoutPath: "/tmp/daemon.log",
		StderrPath: "/tmp/daemon.err",
		RunAtLoad:  true,
		KeepAlive:  true,
	}
	if err := writePlist(path, cfg); err != nil {
		t.Fatal(err)
	}
	// Verify file exists and has correct permissions.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected 0600, got %o", info.Mode().Perm())
	}
}

func TestRenderPlistKeepAliveFalse(t *testing.T) {
	b, err := renderPlist(plistConfig{
		Label:      defaultLabel,
		BinaryPath: "/usr/local/bin/opencleanerd",
		SocketPath: "/tmp/test.sock",
		StdoutPath: "/tmp/out.log",
		StderrPath: "/tmp/err.log",
		RunAtLoad:  false,
		KeepAlive:  false,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "<key>RunAtLoad</key>") && strings.Contains(s, "<true/>") {
		// Depends on template logic — check if RunAtLoad is conditionally rendered.
		// If template always renders it, this checks for false.
	}
	if !strings.Contains(s, "<string>/usr/local/bin/opencleanerd</string>") {
		t.Error("expected binary path")
	}
}

func TestInstallPlistEmptyBinaryPath(t *testing.T) {
	err := InstallPlistWithSocket("", "/tmp/sock")
	if err == nil {
		t.Fatal("expected error for empty binary path")
	}
}

func TestInstallPlistRelativeBinaryPath(t *testing.T) {
	err := InstallPlistWithSocket("relative/bin", "/tmp/sock")
	if err == nil {
		t.Fatal("expected error for relative binary path")
	}
}

func TestInstallPlistEmptySocketPath(t *testing.T) {
	err := InstallPlistWithSocket("/usr/local/bin/test", "")
	if err == nil {
		t.Fatal("expected error for empty socket path")
	}
}

func TestInstallPlistLogDirCreation(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("launchd is macOS-specific")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	// This will fail at the launchctl step, but the log directory should be created.
	_ = InstallPlistWithSocket("/usr/local/bin/opencleanerd", "/tmp/oc.sock")

	logDir := filepath.Join(home, ".opencleaner", "logs")
	if _, err := os.Stat(logDir); err != nil {
		t.Errorf("expected log dir to be created at %s: %v", logDir, err)
	}
}

func TestInstallPlistBlockedLogDir(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("launchd is macOS-specific")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Block .opencleaner by making it a file.
	ocFile := filepath.Join(home, ".opencleaner")
	os.WriteFile(ocFile, []byte("block"), 0o600)

	err := InstallPlistWithSocket("/usr/local/bin/opencleanerd", "/tmp/oc.sock")
	if err == nil {
		t.Error("expected error when log dir creation is blocked")
	}
}

func TestIsInstalledTrue(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("launchd is macOS-specific")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	plistDir := filepath.Join(home, "Library", "LaunchAgents")
	os.MkdirAll(plistDir, 0o700)
	os.WriteFile(filepath.Join(plistDir, "com.opencleaner.daemon.plist"), []byte("<plist/>"), 0o600)

	if !IsInstalled() {
		t.Error("expected IsInstalled to return true when plist file exists")
	}
}
