package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultAuditLogPath(t *testing.T) {
	p, err := DefaultAuditLogPath()
	if err != nil {
		t.Fatal(err)
	}
	if p == "" {
		t.Fatal("expected non-empty path")
	}
	if !filepath.IsAbs(p) {
		t.Errorf("expected absolute path, got %s", p)
	}
	if filepath.Base(p) != "audit.log" {
		t.Errorf("expected audit.log, got %s", filepath.Base(p))
	}
}

func TestNewLogger(t *testing.T) {
	l := NewLogger("/tmp/test.log")
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
	if l.Path() != "/tmp/test.log" {
		t.Errorf("expected /tmp/test.log, got %s", l.Path())
	}
}

func TestAppendCreatesFileAndWritesJSON(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "logs", "audit.log")
	l := NewLogger(logPath)

	entry := Entry{
		Time:    time.Now().UTC(),
		Op:      "test_op",
		SrcPath: "/tmp/src",
		DstPath: "/tmp/dst",
		Bytes:   1024,
		DryRun:  false,
		OK:      true,
	}

	if err := l.Append(entry); err != nil {
		t.Fatal(err)
	}

	// File should exist.
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}

	var decoded Entry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Op != "test_op" {
		t.Errorf("expected op=test_op, got %s", decoded.Op)
	}
	if decoded.SrcPath != "/tmp/src" {
		t.Errorf("expected src=/tmp/src, got %s", decoded.SrcPath)
	}
	if decoded.Bytes != 1024 {
		t.Errorf("expected bytes=1024, got %d", decoded.Bytes)
	}
}

func TestAppendMultipleEntries(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")
	l := NewLogger(logPath)

	for i := 0; i < 3; i++ {
		if err := l.Append(Entry{Op: "op", OK: true}); err != nil {
			t.Fatal(err)
		}
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}

	// Each entry is a JSON line.
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != 3 {
		t.Errorf("expected 3 lines, got %d", lines)
	}
}

func TestAppendWithError(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")
	l := NewLogger(logPath)

	entry := Entry{
		Op:    "failed_op",
		OK:    false,
		Error: "something went wrong",
	}
	if err := l.Append(entry); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Entry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Error != "something went wrong" {
		t.Errorf("expected error field, got %q", decoded.Error)
	}
}

func TestAppendReadOnlyDir(t *testing.T) {
	dir := t.TempDir()
	roDir := filepath.Join(dir, "readonly")
	os.MkdirAll(roDir, 0o700)
	os.Chmod(roDir, 0o400) // read-only
	defer os.Chmod(roDir, 0o700)

	logPath := filepath.Join(roDir, "subdir", "audit.log")
	l := NewLogger(logPath)
	err := l.Append(Entry{Op: "test"})
	if err == nil {
		t.Error("expected error writing to read-only directory")
	}
}

func TestDefaultAuditLogPathWithCustomHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	p, err := DefaultAuditLogPath()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(p) {
		t.Errorf("expected absolute path, got %s", p)
	}
}

func TestAppendOpenFileError(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "logs", "audit.log")
	// Create a file at the logs path so OpenFile on "audit.log" inside it fails.
	os.MkdirAll(filepath.Join(dir, "logs"), 0o700)
	// Make the file exist as a dir-blocking entity.
	os.WriteFile(logPath, []byte("not a dir"), 0o600)
	// Now set log path inside the file (which isn't a dir).
	l := NewLogger(filepath.Join(logPath, "nested", "audit.log"))
	err := l.Append(Entry{Op: "test"})
	if err == nil {
		t.Error("expected error when log path is inside a file")
	}
}
