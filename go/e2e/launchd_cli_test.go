//go:build darwin && e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2E_DaemonLifecycle_CLI_UsesLaunchctlStub(t *testing.T) {
	cliBin, daemonBin := buildBinaries(t)

	home := t.TempDir()
	socketPath := shortSocketPath(t)

	stubDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(stubDir, 0o700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "launchctl.log")

	launchctlPath := filepath.Join(stubDir, "launchctl")
	stub := "#!/bin/sh\n" +
		"set -eu\n" +
		"echo \"$@\" >> \"${LAUNCHCTL_LOG}\"\n" +
		"cmd=\"$1\"\n" +
		"case \"$cmd\" in\n" +
		"  bootout|bootstrap|kickstart) exit 0 ;;\n" +
		"  print) echo 'state = running'; exit 0 ;;\n" +
		"  *) echo 'unexpected launchctl subcommand' 1>&2; exit 2 ;;\n" +
		"esac\n"
	if err := os.WriteFile(launchctlPath, []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}

	env := map[string]string{
		"HOME":          home,
		"PATH":          stubDir + ":" + os.Getenv("PATH"),
		"LAUNCHCTL_LOG": logPath,
	}

	// install
	res := runCmd(t, env, cliBin, "--socket="+socketPath, "daemon", "install", "--binary-path="+daemonBin)
	if res.Code != 0 {
		t.Fatalf("daemon install failed: %s", res.Stderr)
	}

	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.opencleaner.daemon.plist")
	b, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("expected plist written: %v", err)
	}
	plist := string(b)
	if !strings.Contains(plist, daemonBin) {
		t.Fatalf("plist missing daemon binary path")
	}
	if !strings.Contains(plist, socketPath) {
		t.Fatalf("plist missing socket path")
	}

	logb, _ := os.ReadFile(logPath)
	logs := string(logb)
	if !strings.Contains(logs, "bootout") || !strings.Contains(logs, "bootstrap") {
		t.Fatalf("expected launchctl bootout+bootstrap, got logs=%q", logs)
	}

	// restart
	res = runCmd(t, env, cliBin, "--socket="+socketPath, "daemon", "restart")
	if res.Code != 0 {
		t.Fatalf("daemon restart failed: %s", res.Stderr)
	}
	logb, _ = os.ReadFile(logPath)
	if !strings.Contains(string(logb), "kickstart") {
		t.Fatalf("expected launchctl kickstart")
	}

	// uninstall
	res = runCmd(t, env, cliBin, "--socket="+socketPath, "daemon", "uninstall")
	if res.Code != 0 {
		t.Fatalf("daemon uninstall failed: %s", res.Stderr)
	}
	if _, err := os.Stat(plistPath); !os.IsNotExist(err) {
		t.Fatalf("expected plist removed")
	}
}

func TestE2E_DaemonLifecycle_DaemonInstallFlag_UsesLaunchctlStub(t *testing.T) {
	_, daemonBin := buildBinaries(t)

	home := t.TempDir()
	socketPath := shortSocketPath(t)

	stubDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(stubDir, 0o700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "launchctl.log")

	launchctlPath := filepath.Join(stubDir, "launchctl")
	stub := "#!/bin/sh\n" +
		"set -eu\n" +
		"echo \"$@\" >> \"${LAUNCHCTL_LOG}\"\n" +
		"cmd=\"$1\"\n" +
		"case \"$cmd\" in\n" +
		"  bootout|bootstrap) exit 0 ;;\n" +
		"  *) exit 0 ;;\n" +
		"esac\n"
	if err := os.WriteFile(launchctlPath, []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}

	env := map[string]string{
		"HOME":          home,
		"PATH":          stubDir + ":" + os.Getenv("PATH"),
		"LAUNCHCTL_LOG": logPath,
	}

	res := runCmd(t, env, daemonBin, "-install", "-socket", socketPath)
	if res.Code != 0 {
		t.Fatalf("opencleanerd -install failed: %s", res.Stderr)
	}

	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.opencleaner.daemon.plist")
	if _, err := os.Stat(plistPath); err != nil {
		t.Fatalf("expected plist written: %v", err)
	}
}
