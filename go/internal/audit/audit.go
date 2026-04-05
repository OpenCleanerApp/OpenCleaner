package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Entry struct {
	Time    time.Time `json:"time"`
	Op      string    `json:"op"`
	SrcPath string    `json:"src_path"`
	DstPath string    `json:"dst_path,omitempty"`
	Bytes   int64     `json:"bytes"`
	DryRun  bool      `json:"dry_run"`
	OK      bool      `json:"ok"`
	Error   string    `json:"error,omitempty"`
}

type Logger struct {
	path string
}

func DefaultAuditLogPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".opencleaner", "logs", "audit.log"), nil
}

func NewLogger(path string) *Logger {
	return &Logger{path: path}
}

func (l *Logger) Append(e Entry) error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	return enc.Encode(e)
}

func (l *Logger) Path() string { return l.path }
