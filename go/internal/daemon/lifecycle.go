package daemon

import (
	"bytes"
	"embed"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/opencleaner/opencleaner/internal/transport"
)

const defaultLabel = "com.opencleaner.daemon"

//go:embed com.opencleaner.daemon.plist
var plistTemplateFS embed.FS

type plistConfig struct {
	Label      string
	BinaryPath string
	SocketPath string
	StdoutPath string
	StderrPath string
	RunAtLoad  bool
	KeepAlive  bool
}

func PlistPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "LaunchAgents", defaultLabel+".plist")
}

func InstallPlist(binaryPath string) error {
	return InstallPlistWithSocket(binaryPath, transport.DefaultSocketPath())
}

func InstallPlistWithSocket(binaryPath, socketPath string) error {
	if binaryPath == "" {
		return errors.New("binaryPath required")
	}
	if !filepath.IsAbs(binaryPath) {
		return errors.New("binaryPath must be absolute")
	}
	if socketPath == "" {
		return errors.New("socketPath required")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	logDir := filepath.Join(home, ".opencleaner", "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return err
	}

	cfg := plistConfig{
		Label:      defaultLabel,
		BinaryPath: binaryPath,
		SocketPath: socketPath,
		StdoutPath: filepath.Join(logDir, "daemon.log"),
		StderrPath: filepath.Join(logDir, "daemon.err"),
		RunAtLoad:  true,
		KeepAlive:  true,
	}

	plistPath := PlistPath()
	if plistPath == "" {
		return errors.New("failed to resolve plist path")
	}
	if err := writePlist(plistPath, cfg); err != nil {
		return err
	}

	uid := strconv.Itoa(os.Getuid())
	_, _ = exec.Command("launchctl", "bootout", "gui/"+uid, plistPath).CombinedOutput()
	cmd := exec.Command("launchctl", "bootstrap", "gui/"+uid, plistPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl bootstrap failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func UninstallPlist() error {
	plistPath := PlistPath()
	if plistPath == "" {
		return errors.New("failed to resolve plist path")
	}
	uid := strconv.Itoa(os.Getuid())

	var bootoutErr error
	if _, err := os.Stat(plistPath); err == nil {
		bootoutCmd := exec.Command("launchctl", "bootout", "gui/"+uid, plistPath)
		bootoutOut, err := bootoutCmd.CombinedOutput()
		if err != nil && !isIgnorableLaunchctlBootout(string(bootoutOut)) {
			bootoutErr = fmt.Errorf("launchctl bootout failed: %w (%s)", err, strings.TrimSpace(string(bootoutOut)))
		}
	}

	if err := os.Remove(plistPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return bootoutErr
}

func IsInstalled() bool {
	p := PlistPath()
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

func IsRunning() bool {
	uid := strconv.Itoa(os.Getuid())
	out, err := exec.Command("launchctl", "print", "gui/"+uid+"/"+defaultLabel).CombinedOutput()
	if err != nil {
		return false
	}
	s := string(out)
	return strings.Contains(s, "state = running") || strings.Contains(s, "pid =")
}

func Restart() error {
	uid := strconv.Itoa(os.Getuid())
	cmd := exec.Command("launchctl", "kickstart", "-k", "gui/"+uid+"/"+defaultLabel)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl kickstart failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func isIgnorableLaunchctlBootout(out string) bool {
	s := strings.ToLower(out)
	return strings.Contains(s, "could not find service") ||
		strings.Contains(s, "no such process") ||
		strings.Contains(s, "not found")
}

func writePlist(path string, cfg plistConfig) error {
	b, err := renderPlist(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func xmlEscape(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

func renderPlist(cfg plistConfig) ([]byte, error) {
	b, err := plistTemplateFS.ReadFile("com.opencleaner.daemon.plist")
	if err != nil {
		return nil, err
	}
	tmpl, err := template.New("plist").Funcs(template.FuncMap{"xml": xmlEscape}).Parse(string(b))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, cfg); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
