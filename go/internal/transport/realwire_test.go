package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/opencleaner/opencleaner/internal/audit"
	"github.com/opencleaner/opencleaner/internal/engine"
	"github.com/opencleaner/opencleaner/internal/rules"
	"github.com/opencleaner/opencleaner/internal/stream"
	"github.com/opencleaner/opencleaner/pkg/types"
)

func startTestDaemon(t *testing.T) (socketPath string, broker *stream.Broker) {
	socketPath, _, broker = startTestDaemonWithRules(t, nil)
	return socketPath, broker
}

func startTestDaemonWithRules(t *testing.T, ruleSet []rules.Rule) (socketPath string, tmp string, broker *stream.Broker) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("unix domain sockets not supported on windows")
	}

	// On macOS, t.TempDir() (os.TempDir) can be too long for unix socket paths (sun_path limit).
	// Prefer /tmp when available to keep the socket path short.
	base := os.TempDir()
	if st, err := os.Stat("/tmp"); err == nil && st.IsDir() {
		base = "/tmp"
	}
	tmp, err := os.MkdirTemp(base, "opencleaner-")
	if err != nil {
		t.Fatal(err)
	}

	socketPath = filepath.Join(tmp, "opencleaner.sock")
	broker = stream.NewBroker()
	eng := engine.New(ruleSet, broker, audit.NewLogger(filepath.Join(tmp, "audit.log")))
	srv := NewServer(eng, broker, socketPath, "test")

	ln, err := ListenUnixSocket(socketPath)
	if err != nil {
		_ = os.RemoveAll(tmp)
		t.Fatal(err)
	}

	httpSrv := &http.Server{Handler: srv.Handler()}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- httpSrv.Serve(ln)
	}()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(ctx); err != nil {
			_ = httpSrv.Close() // force-close if graceful shutdown times out
		}
		_ = ln.Close()
		_ = os.RemoveAll(tmp)

		select {
		case err := <-serveErr:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				t.Errorf("http server exited unexpectedly: %v", err)
			}
		case <-time.After(1 * time.Second):
			t.Errorf("http server did not exit")
		}
	})

	return socketPath, tmp, broker
}

func TestRealWire_StatusOverUnixSocket(t *testing.T) {
	socketPath, _ := startTestDaemon(t)

	client := NewUnixSocketHTTPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/api/v1/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
}

func TestRealWire_ProgressStreamIsChunkedAndWellFormed(t *testing.T) {
	socketPath, broker := startTestDaemon(t)

	// Publish before subscribing so the handler will immediately emit the last event.
	broker.Publish(types.ProgressEvent{Type: "scanning", Progress: 0.2, Message: "starting"})

	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))

	if _, err := fmt.Fprintf(conn, "GET /api/v1/progress/stream HTTP/1.1\r\nHost: unix\r\nAccept: text/event-stream\r\n\r\n"); err != nil {
		t.Fatal(err)
	}

	r := bufio.NewReader(conn)
	hdrRaw, err := readUntilDoubleCRLF(r, 64*1024)
	if err != nil {
		t.Fatal(err)
	}

	status, headers := parseRawHTTPHeaders(t, hdrRaw)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d\n%s", status, string(hdrRaw))
	}
	if te := strings.ToLower(headers["transfer-encoding"]); !strings.Contains(te, "chunked") {
		t.Fatalf("expected Transfer-Encoding: chunked, got %q", headers["transfer-encoding"])
	}

	chunk, err := readChunk(r, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(chunk, []byte("data: ")) {
		t.Fatalf("expected SSE data prefix, got %q", string(chunk))
	}
	if !bytes.Contains(chunk, []byte(`"type":"scanning"`)) {
		t.Fatalf("expected scanning event, got %q", string(chunk))
	}
	if !bytes.Contains(chunk, []byte("\n\n")) {
		t.Fatalf("expected SSE event terminator, got %q", string(chunk))
	}
}

func readUntilDoubleCRLF(r *bufio.Reader, max int) ([]byte, error) {
	var buf []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		buf = append(buf, b)
		if len(buf) > max {
			return nil, errors.New("header too large")
		}
		if len(buf) >= 4 && bytes.Equal(buf[len(buf)-4:], []byte("\r\n\r\n")) {
			return buf, nil
		}
	}
}

func parseRawHTTPHeaders(t *testing.T, hdr []byte) (int, map[string]string) {
	t.Helper()
	lines := strings.Split(string(hdr), "\r\n")
	statusLine := lines[0]
	parts := strings.Fields(statusLine)
	if len(parts) < 2 {
		t.Fatalf("invalid status line: %q", statusLine)
	}
	code, err := strconv.Atoi(parts[1])
	if err != nil {
		t.Fatalf("invalid status code in %q: %v", statusLine, err)
	}

	h := map[string]string{}
	for _, l := range lines[1:] {
		if l == "" {
			break
		}
		idx := strings.Index(l, ":")
		if idx < 0 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(l[:idx]))
		v := strings.TrimSpace(l[idx+1:])
		h[k] = v
	}
	return code, h
}

func TestRealWire_CleanBlocksExcludedPaths(t *testing.T) {
	socketPath, tmp, _ := startTestDaemonWithRules(t, []rules.Rule{})

	target := filepath.Join(tmp, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "a.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Start a second daemon with a rule pointing at our target.
	socketPath, _, _ = startTestDaemonWithRules(t, []rules.Rule{
		{
			ID:       "test-target",
			Name:     "Test Target",
			Path:     target,
			Category: types.CategorySystem,
			Safety:   types.SafetySafe,
			Desc:     "test data",
		},
	})

	client := NewUnixSocketHTTPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run scan to populate lastScan.
	{
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/api/v1/scan", strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
		}
		var scan types.ScanResult
		if err := json.NewDecoder(resp.Body).Decode(&scan); err != nil {
			t.Fatal(err)
		}
		if len(scan.Items) != 1 || scan.Items[0].ID != "test-target" {
			t.Fatalf("expected 1 item test-target, got %+v", scan.Items)
		}
	}

	// Clean should be blocked by exclude_paths.
	{
		body := fmt.Sprintf(`{"item_ids":["test-target"],"strategy":"trash","dry_run":true,"exclude_paths":[%q]}`, target)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/api/v1/clean", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
		}
		var res types.CleanResult
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			t.Fatal(err)
		}
		if res.CleanedCount != 0 || res.CleanedSize != 0 {
			t.Fatalf("expected cleaned=0, got count=%d size=%d", res.CleanedCount, res.CleanedSize)
		}
		if len(res.FailedItems) != 1 || res.FailedItems[0] != "test-target" {
			t.Fatalf("expected failed_items=[test-target], got %+v", res.FailedItems)
		}
		if res.AuditLogPath == "" {
			t.Fatalf("expected audit_log_path")
		}

		b, err := os.ReadFile(res.AuditLogPath)
		if err != nil {
			t.Fatal(err)
		}
		log := string(b)
		if !strings.Contains(log, "blocked_exclude") {
			t.Fatalf("expected blocked_exclude in audit log, got: %s", log)
		}
		if !strings.Contains(log, target) {
			t.Fatalf("expected target path in audit log, got: %s", log)
		}
	}
}

func TestRealWire_UndoEndpoint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix domain sockets not supported on windows")
	}

	base := os.TempDir()
	if st, err := os.Stat("/tmp"); err == nil && st.IsDir() {
		base = "/tmp"
	}
	home, err := os.MkdirTemp(base, "opencleaner-home-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(home)
	})
	t.Setenv("HOME", home)

	target := filepath.Join(home, "Library", "Caches", "undo-target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "a.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	socketPath, _, _ := startTestDaemonWithRules(t, []rules.Rule{{
		ID:       "test-target",
		Name:     "Test Target",
		Path:     target,
		Category: types.CategorySystem,
		Safety:   types.SafetySafe,
		Desc:     "test data",
	}})

	client := NewUnixSocketHTTPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Scan to populate lastScan.
	{
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/api/v1/scan", strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
		}
	}

	// Clean (execute) should move the target into ~/.Trash.
	{
		body := `{"item_ids":["test-target"],"strategy":"trash","dry_run":false}`
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/api/v1/clean", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
		}
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected target moved to trash, got err=%v", err)
	}

	// Undo should restore it.
	{
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/api/v1/undo", strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
		}
		var res types.UndoResult
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			t.Fatal(err)
		}
		if res.RestoredCount != 1 {
			t.Fatalf("expected restored_count=1, got %d", res.RestoredCount)
		}
	}
	if _, err := os.Lstat(target); err != nil {
		t.Fatalf("expected target restored: %v", err)
	}
}

func readChunk(r *bufio.Reader, maxBytes int) ([]byte, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	sizeTok := strings.SplitN(line, ";", 2)[0]
	sizeTok = strings.TrimSpace(sizeTok)
	var size int
	_, err = fmt.Sscanf(sizeTok, "%x", &size)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		// Consume trailing CRLF and optional trailers; for this test we can stop here.
		_, _ = r.ReadString('\n')
		return nil, io.EOF
	}
	if size > maxBytes {
		return nil, errors.New("chunk too large")
	}

	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	crlf := make([]byte, 2)
	if _, err := io.ReadFull(r, crlf); err != nil {
		return nil, err
	}
	if !bytes.Equal(crlf, []byte("\r\n")) {
		return nil, errors.New("missing chunk CRLF")
	}

	return buf, nil
}
