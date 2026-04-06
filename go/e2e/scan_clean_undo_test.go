//go:build darwin && e2e

package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opencleaner/opencleaner/internal/transport"
	"github.com/opencleaner/opencleaner/pkg/types"
)

func TestE2E_ScanCleanUndo_ProgressSSE(t *testing.T) {
	cliBin, daemonBin := buildBinaries(t)

	home := t.TempDir()
	socketPath := shortSocketPath(t)
	env := map[string]string{
		"HOME": home,
	}

	// Seed a deterministic target for builtin rule: homebrew-cache => ~/Library/Caches/Homebrew
	seedDir := filepath.Join(home, "Library", "Caches", "Homebrew", "OpenCleanerE2E")
	if err := os.MkdirAll(seedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	seedFile := filepath.Join(seedDir, "blob.bin")
	seedBytes := bytes.Repeat([]byte{0xAB}, 4096)
	if err := os.WriteFile(seedFile, seedBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	startDaemon(t, daemonBin, env, socketPath)
	waitFor(t, 5*time.Second, func() error {
		res := runCmd(t, env, cliBin, "--socket="+socketPath, "--json", "status")
		if res.Code != 0 {
			return fmt.Errorf("status failed: %s", strings.TrimSpace(res.Stderr))
		}
		return nil
	})

	// Open SSE stream before operations so we observe progress types.
	events, stop := startSSE(t, socketPath)
	defer stop()

	// Scan
	var scan types.ScanResult
	{
		res := runCmd(t, env, cliBin, "--socket="+socketPath, "--json", "scan")
		if res.Code != 0 {
			t.Fatalf("scan failed: %s", res.Stderr)
		}
		if err := json.Unmarshal([]byte(res.Stdout), &scan); err != nil {
			t.Fatalf("scan json decode failed: %v\nstdout=%s", err, res.Stdout)
		}
		it, ok := findItem(scan.Items, "homebrew-cache")
		if !ok {
			t.Fatalf("expected scan item homebrew-cache, got %d items", len(scan.Items))
		}
		if it.Size < int64(len(seedBytes)) {
			t.Fatalf("expected homebrew-cache size >= %d, got %d", len(seedBytes), it.Size)
		}
	}
	requireEventType(t, events, "scanning", 2*time.Second)
	requireEventType(t, events, "complete", 2*time.Second)

	// Clean dry-run should not move files
	{
		res := runCmd(t, env, cliBin, "--socket="+socketPath, "--json", "clean", "homebrew-cache", "--dry-run")
		if res.Code != 0 {
			t.Fatalf("clean dry-run failed: %s", res.Stderr)
		}
		var cr types.CleanResult
		if err := json.Unmarshal([]byte(res.Stdout), &cr); err != nil {
			t.Fatalf("clean dry-run json decode failed: %v\nstdout=%s", err, res.Stdout)
		}
		if !cr.DryRun {
			t.Fatalf("expected DryRun=true")
		}
		if _, err := os.Stat(seedFile); err != nil {
			t.Fatalf("expected seed file to remain after dry-run: %v", err)
		}
	}

	// Clean execute (trash) + undo manifest written
	{
		res := runCmd(t, env, cliBin, "--socket="+socketPath, "--json", "clean", "homebrew-cache", "--execute", "--yes", "--strategy=trash")
		if res.Code != 0 {
			t.Fatalf("clean execute failed: %s", res.Stderr)
		}
		var cr types.CleanResult
		if err := json.Unmarshal([]byte(res.Stdout), &cr); err != nil {
			t.Fatalf("clean execute json decode failed: %v\nstdout=%s", err, res.Stdout)
		}
		if cr.DryRun {
			t.Fatalf("expected DryRun=false")
		}
		if cr.CleanedCount != 1 {
			t.Fatalf("expected CleanedCount=1, got %d", cr.CleanedCount)
		}

		if _, err := os.Stat(seedDir); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected seed dir moved to trash, stat err=%v", err)
		}

		manifestPath := filepath.Join(home, ".opencleaner", "undo", "last.json")
		if _, err := os.Stat(manifestPath); err != nil {
			t.Fatalf("expected undo manifest to exist: %v", err)
		}
	}
	requireEventType(t, events, "cleaning", 2*time.Second)
	requireEventType(t, events, "complete", 2*time.Second)

	// Undo restores
	{
		res := runCmd(t, env, cliBin, "--socket="+socketPath, "--json", "undo")
		if res.Code != 0 {
			t.Fatalf("undo failed: %s", res.Stderr)
		}
		var ur types.UndoResult
		if err := json.Unmarshal([]byte(res.Stdout), &ur); err != nil {
			t.Fatalf("undo json decode failed: %v\nstdout=%s", err, res.Stdout)
		}
		if ur.RestoredCount != 1 {
			t.Fatalf("expected RestoredCount=1, got %d", ur.RestoredCount)
		}
		if _, err := os.Stat(seedFile); err != nil {
			t.Fatalf("expected seed file restored: %v", err)
		}
	}
	requireEventType(t, events, "undoing", 2*time.Second)
	requireEventType(t, events, "complete", 2*time.Second)

	// Second undo should fail (manifest cleared/pruned).
	{
		res := runCmd(t, env, cliBin, "--socket="+socketPath, "--json", "undo")
		if res.Code == 0 {
			t.Fatalf("expected undo to fail when no manifest")
		}
	}
}

func startSSE(t *testing.T, socketPath string) (<-chan types.ProgressEvent, func()) {
	t.Helper()
	client := transport.NewUnixSocketHTTPClient(socketPath)

	req, err := http.NewRequest(http.MethodGet, "http://unix/api/v1/progress/stream", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("sse request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		t.Fatalf("sse status=%d", resp.StatusCode)
	}

	ch := make(chan types.ProgressEvent, 128)
	stop := func() {
		_ = resp.Body.Close()
	}

	go func() {
		s := bufio.NewScanner(resp.Body)
		for s.Scan() {
			line := s.Text()
			if strings.HasPrefix(line, "data: ") {
				payload := strings.TrimPrefix(line, "data: ")
				var evt types.ProgressEvent
				if err := json.Unmarshal([]byte(payload), &evt); err == nil {
					select {
					case ch <- evt:
					default:
					}
				}
			}
		}
		close(ch)
	}()

	return ch, stop
}

func requireEventType(t *testing.T, ch <-chan types.ProgressEvent, typ string, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatalf("timed out waiting for event type=%q", typ)
		case evt, ok := <-ch:
			if !ok {
				t.Fatalf("event stream closed while waiting for type=%q", typ)
			}
			if evt.Type == typ {
				return
			}
		}
	}
}

func findItem(items []types.ScanItem, id string) (types.ScanItem, bool) {
	for _, it := range items {
		if it.ID == id {
			return it, true
		}
	}
	return types.ScanItem{}, false
}
