package logger

import (
	"log/slog"
	"testing"
)

func TestNew(t *testing.T) {
	l := New(slog.LevelInfo)
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
	if !l.Enabled(nil, slog.LevelInfo) {
		t.Error("expected info level enabled")
	}
	if l.Enabled(nil, slog.LevelDebug) {
		t.Error("expected debug level disabled")
	}
}

func TestNewJSON(t *testing.T) {
	l := NewJSON(slog.LevelDebug)
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
	if !l.Enabled(nil, slog.LevelDebug) {
		t.Error("expected debug level enabled")
	}
}

func TestNewWithWarnLevel(t *testing.T) {
	l := New(slog.LevelWarn)
	if l.Enabled(nil, slog.LevelInfo) {
		t.Error("expected info disabled at warn level")
	}
	if !l.Enabled(nil, slog.LevelWarn) {
		t.Error("expected warn enabled")
	}
}
