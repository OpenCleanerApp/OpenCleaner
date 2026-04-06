package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
	"github.com/opencleaner/opencleaner/internal/scheduler"
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

// --- Transport handler tests ---

func startTestDaemonWithScheduler(t *testing.T) (socketPath string, sched *scheduler.Scheduler) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("unix domain sockets not supported on windows")
	}

	base := os.TempDir()
	if st, err := os.Stat("/tmp"); err == nil && st.IsDir() {
		base = "/tmp"
	}
	tmp, err := os.MkdirTemp(base, "opencleaner-")
	if err != nil {
		t.Fatal(err)
	}
	home := tmp
	t.Setenv("HOME", home)

	socketPath = filepath.Join(tmp, "oc.sock")
	broker := stream.NewBroker()
	eng := engine.New(nil, broker, audit.NewLogger(filepath.Join(tmp, "audit.log")))
	srv := NewServer(eng, broker, socketPath, "test")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sched = scheduler.New(eng, broker, logger)
	srv.SetScheduler(sched)

	ln, err := ListenUnixSocket(socketPath)
	if err != nil {
		_ = os.RemoveAll(tmp)
		t.Fatal(err)
	}
	httpSrv := &http.Server{Handler: srv.Handler()}
	go httpSrv.Serve(ln)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		httpSrv.Shutdown(ctx)
		ln.Close()
		os.RemoveAll(tmp)
	})
	return socketPath, sched
}

func TestRealWire_SchedulerGetNoScheduler(t *testing.T) {
	socketPath, _ := startTestDaemon(t)
	client := NewUnixSocketHTTPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/api/v1/scheduler", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

func TestRealWire_SchedulerGetWithScheduler(t *testing.T) {
	socketPath, _ := startTestDaemonWithScheduler(t)
	client := NewUnixSocketHTTPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/api/v1/scheduler", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}
}

func TestRealWire_SchedulerPutEnable(t *testing.T) {
	socketPath, _ := startTestDaemonWithScheduler(t)
	client := NewUnixSocketHTTPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	body := `{"enabled":true,"interval":"daily","time":"03:00"}`
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, "http://unix/api/v1/scheduler", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}

	var status scheduler.ScheduleStatus
	json.NewDecoder(resp.Body).Decode(&status)
	if !status.Enabled {
		t.Error("expected enabled after PUT")
	}
}

func TestRealWire_SchedulerPutInvalid(t *testing.T) {
	socketPath, _ := startTestDaemonWithScheduler(t)
	client := NewUnixSocketHTTPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	body := `{"enabled":true,"interval":"bad"}`
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, "http://unix/api/v1/scheduler", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestRealWire_SchedulerPutBadJSON(t *testing.T) {
	socketPath, _ := startTestDaemonWithScheduler(t)
	client := NewUnixSocketHTTPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, "http://unix/api/v1/scheduler", strings.NewReader(`{invalid`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestRealWire_SchedulerDelete(t *testing.T) {
	socketPath, sched := startTestDaemonWithScheduler(t)
	sched.Start(scheduler.Schedule{Enabled: true, Interval: "daily", TimeOfDay: "03:00"})

	client := NewUnixSocketHTTPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, "http://unix/api/v1/scheduler", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}
}

func TestRealWire_SchedulerMethodNotAllowed(t *testing.T) {
	socketPath, _ := startTestDaemonWithScheduler(t)
	client := NewUnixSocketHTTPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/api/v1/scheduler", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}

func TestRealWire_UndoNoManifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	socketPath, _, _ := startTestDaemonWithRules(t, nil)
	client := NewUnixSocketHTTPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/api/v1/undo", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(b))
	}
}

func TestRealWire_UndoMethodNotAllowed(t *testing.T) {
	socketPath, _ := startTestDaemon(t)
	client := NewUnixSocketHTTPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/api/v1/undo", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}

func TestRealWire_ScanMethodNotAllowed(t *testing.T) {
	socketPath, _ := startTestDaemon(t)
	client := NewUnixSocketHTTPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/api/v1/scan", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}

func TestRealWire_CleanMethodNotAllowed(t *testing.T) {
	socketPath, _ := startTestDaemon(t)
	client := NewUnixSocketHTTPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/api/v1/clean", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}

func TestRealWire_CleanBadJSON(t *testing.T) {
	socketPath, _ := startTestDaemon(t)
	client := NewUnixSocketHTTPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/api/v1/clean", strings.NewReader(`{bad`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestRealWire_SetScheduler(t *testing.T) {
	socketPath, _, broker := startTestDaemonWithRules(t, nil)
	_ = socketPath
	_ = broker
	// SetScheduler is tested implicitly via startTestDaemonWithScheduler.
	// This just verifies the server can be constructed without a scheduler.
}

func TestRealWire_ScanSuccess(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	target := filepath.Join(home, "Library", "Caches", "scan-target")
	os.MkdirAll(target, 0o700)
	os.WriteFile(filepath.Join(target, "data.txt"), []byte("hello"), 0o600)

	ruleSet := []rules.Rule{{
		ID:         "scan-test",
		Name:       "Scan Test",
		Path:       target,
		Category:   types.CategorySystem,
		Safety:     types.SafetySafe,
		SafetyNote: "test",
		Desc:       "test",
	}}

	socketPath, _, _ := startTestDaemonWithRules(t, ruleSet)
	client := NewUnixSocketHTTPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/api/v1/scan", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}
	var result types.ScanResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Items) == 0 {
		t.Error("expected at least 1 scan item")
	}
}

func TestRealWire_CleanSuccess(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trashDir := filepath.Join(home, ".Trash")
	os.MkdirAll(trashDir, 0o700)

	target := filepath.Join(home, "Library", "Caches", "clean-target")
	os.MkdirAll(target, 0o700)
	os.WriteFile(filepath.Join(target, "data.txt"), []byte("hello"), 0o600)

	ruleSet := []rules.Rule{{
		ID:         "clean-test",
		Name:       "Clean Test",
		Path:       target,
		Category:   types.CategorySystem,
		Safety:     types.SafetySafe,
		SafetyNote: "test",
		Desc:       "test",
	}}

	socketPath, _, _ := startTestDaemonWithRules(t, ruleSet)
	client := NewUnixSocketHTTPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// First scan.
	scanReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/api/v1/scan", nil)
	scanResp, err := client.Do(scanReq)
	if err != nil {
		t.Fatal(err)
	}
	scanResp.Body.Close()

	// Then clean.
	cleanBody, _ := json.Marshal(types.CleanRequest{
		ItemIDs:  []string{"clean-test"},
		Strategy: types.CleanStrategyTrash,
		DryRun:   true,
	})
	cleanReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/api/v1/clean", bytes.NewReader(cleanBody))
	cleanReq.Header.Set("Content-Type", "application/json")
	cleanResp, err := client.Do(cleanReq)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanResp.Body.Close()
	if cleanResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(cleanResp.Body)
		t.Fatalf("expected 200, got %d: %s", cleanResp.StatusCode, string(b))
	}
	var result types.CleanResult
	if err := json.NewDecoder(cleanResp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.CleanedCount != 1 {
		t.Errorf("expected 1 cleaned, got %d", result.CleanedCount)
	}
}

func TestRealWire_CleanExtraJSON(t *testing.T) {
	socketPath, _ := startTestDaemon(t)
	client := NewUnixSocketHTTPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Two JSON objects in body should be rejected.
	body := `{"item_ids":["a"],"strategy":"trash"}{"extra":"value"}`
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/api/v1/clean", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for extra JSON, got %d", resp.StatusCode)
	}
}

func TestListenUnixSocketStaleCleaned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix domain sockets not supported on windows")
	}
	base := os.TempDir()
	if st, err := os.Stat("/tmp"); err == nil && st.IsDir() {
		base = "/tmp"
	}
	tmp, err := os.MkdirTemp(base, "opencleaner-stale-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	socketPath := filepath.Join(tmp, "stale.sock")

	// Create a listener then close it (leaves stale socket file).
	ln1, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	ln1.Close()

	// Now ListenUnixSocket should clean up the stale socket.
	ln2, err := ListenUnixSocket(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	ln2.Close()
}

func TestRealWire_StatusJSON(t *testing.T) {
	socketPath, _ := startTestDaemon(t)
	client := NewUnixSocketHTTPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/api/v1/status", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("expected application/json, got %q", resp.Header.Get("Content-Type"))
	}
	var status types.DaemonStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if !status.OK {
		t.Error("expected OK=true")
	}
	if status.Version != "test" {
		t.Errorf("expected version=test, got %q", status.Version)
	}
}

func TestRealWire_CleanDefaultStrategy(t *testing.T) {
	// Set up real targets so scan finds something.
	home := t.TempDir()
	t.Setenv("HOME", home)

	targetDir := filepath.Join(home, "Library", "Caches", "test-cache")
	os.MkdirAll(targetDir, 0o700)
	os.WriteFile(filepath.Join(targetDir, "data"), []byte("cached"), 0o600)

	testRules := []rules.Rule{{
		ID:       "test-cache",
		Name:     "Test Cache",
		Path:     targetDir,
		Category: types.CategorySystem,
		Safety:   types.SafetySafe,
	}}
	socketPath, _, _ := startTestDaemonWithRules(t, testRules)
	client := NewUnixSocketHTTPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Scan first.
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/api/v1/scan", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Clean without specifying strategy — should default to "trash".
	body := `{"item_ids":["test-cache"]}`
	req, _ = http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/api/v1/clean", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
}

func TestRealWire_SchedulerNoInit(t *testing.T) {
	// Server without scheduler set.
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets")
	}
	base := os.TempDir()
	if st, err := os.Stat("/tmp"); err == nil && st.IsDir() {
		base = "/tmp"
	}
	tmp, err := os.MkdirTemp(base, "opencleaner-nosched-")
	if err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(tmp, "test.sock")
	broker := stream.NewBroker()
	eng := engine.New(nil, broker, audit.NewLogger(filepath.Join(tmp, "audit.log")))
	srv := NewServer(eng, broker, socketPath, "test")
	// NOT calling SetScheduler.

	ln, err := ListenUnixSocket(socketPath)
	if err != nil {
		os.RemoveAll(tmp)
		t.Fatal(err)
	}
	httpSrv := &http.Server{Handler: srv.Handler()}
	go httpSrv.Serve(ln)
	t.Cleanup(func() {
		httpSrv.Close()
		ln.Close()
		os.RemoveAll(tmp)
	})

	client := NewUnixSocketHTTPClient(socketPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/api/v1/scheduler", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for nil scheduler, got %d", resp.StatusCode)
	}
}

func TestRealWire_ProgressStreamSSE(t *testing.T) {
	socketPath, broker := startTestDaemon(t)

	// Use a raw connection to avoid client timeout issues with SSE.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a transport that dials the unix socket.
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	client := &http.Client{Transport: tr}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/api/v1/progress/stream", nil)

	// Start reading in a goroutine.
	type sseResult struct {
		msg string
		err error
	}
	ch := make(chan sseResult, 1)

	go func() {
		resp, err := client.Do(req)
		if err != nil {
			ch <- sseResult{err: err}
			return
		}
		defer resp.Body.Close()

		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				var evt types.ProgressEvent
				if err := json.Unmarshal([]byte(data), &evt); err != nil {
					ch <- sseResult{err: err}
					return
				}
				ch <- sseResult{msg: evt.Message}
				return
			}
		}
		ch <- sseResult{err: fmt.Errorf("stream ended without event")}
	}()

	// Give the subscriber time to register.
	time.Sleep(200 * time.Millisecond)

	broker.Publish(types.ProgressEvent{
		Type:    "scan",
		Message: "testing SSE",
	})

	select {
	case res := <-ch:
		if res.err != nil {
			t.Fatal(res.err)
		}
		if res.msg != "testing SSE" {
			t.Errorf("expected 'testing SSE', got %q", res.msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SSE event")
	}
}

func TestListenUnixSocketNonSocketFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets")
	}
	base := os.TempDir()
	if st, err := os.Stat("/tmp"); err == nil && st.IsDir() {
		base = "/tmp"
	}
	tmp, err := os.MkdirTemp(base, "opencleaner-nonsock-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	sockPath := filepath.Join(tmp, "notasocket")
	os.WriteFile(sockPath, []byte("regular file"), 0o600)

	_, err = ListenUnixSocket(sockPath)
	if err == nil {
		t.Fatal("expected error for non-socket file")
	}
	if !strings.Contains(err.Error(), "non-socket") {
		t.Errorf("expected 'non-socket' in error, got: %v", err)
	}
}

func TestListenUnixSocketActiveSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets")
	}
	base := os.TempDir()
	if st, err := os.Stat("/tmp"); err == nil && st.IsDir() {
		base = "/tmp"
	}
	tmp, err := os.MkdirTemp(base, "opencleaner-active-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	sockPath := filepath.Join(tmp, "active.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close() // keep it active

	_, err = ListenUnixSocket(sockPath)
	if err == nil {
		t.Fatal("expected error for active socket")
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Errorf("expected 'already in use' error, got: %v", err)
	}
}

func TestListenUnixSocketMkdirAllError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "block")
	os.WriteFile(blocker, []byte("x"), 0o600)
	// Socket path nested under a file — MkdirAll will fail.
	sockPath := filepath.Join(blocker, "nested", "test.sock")
	_, err := ListenUnixSocket(sockPath)
	if err == nil {
		t.Fatal("expected error for MkdirAll failure")
	}
}

func TestListenUnixSocketListenError(t *testing.T) {
	dir := t.TempDir()
	// Use a very long path to trigger net.Listen failure (sun_path limit).
	longName := strings.Repeat("a", 200)
	sockPath := filepath.Join(dir, longName, "test.sock")
	os.MkdirAll(filepath.Dir(sockPath), 0o700)
	_, err := ListenUnixSocket(sockPath)
	if err == nil {
		t.Fatal("expected error for too-long socket path")
	}
}

func TestRealWire_ProgressStreamDebugSSE(t *testing.T) {
	t.Setenv("OPENCLEANER_DEBUG_SSE", "1")
	socketPath, broker := startTestDaemon(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	client := &http.Client{Transport: tr}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/api/v1/progress/stream", nil)

	type sseResult struct {
		msg string
		err error
	}
	ch := make(chan sseResult, 1)

	go func() {
		resp, err := client.Do(req)
		if err != nil {
			ch <- sseResult{err: err}
			return
		}
		defer resp.Body.Close()

		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				var evt types.ProgressEvent
				if err := json.Unmarshal([]byte(data), &evt); err != nil {
					ch <- sseResult{err: err}
					return
				}
				ch <- sseResult{msg: evt.Message}
				return
			}
		}
		ch <- sseResult{err: fmt.Errorf("stream ended")}
	}()

	time.Sleep(200 * time.Millisecond)
	broker.Publish(types.ProgressEvent{Type: "scan", Message: "debug test"})

	select {
	case res := <-ch:
		if res.err != nil {
			t.Fatal(res.err)
		}
		if res.msg != "debug test" {
			t.Errorf("expected 'debug test', got %q", res.msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}
}

func TestRealWire_ProgressStreamClientDisconnect(t *testing.T) {
	socketPath, _ := startTestDaemon(t)

	// Connect at the raw socket level to test disconnect handling.
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}

	// Send a raw HTTP request for the SSE stream.
	_, err = conn.Write([]byte("GET /api/v1/progress/stream HTTP/1.1\r\nHost: unix\r\n\r\n"))
	if err != nil {
		t.Fatal(err)
	}

	// Read just enough to confirm headers were sent.
	buf := make([]byte, 256)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _ := conn.Read(buf)
	if n > 0 && !strings.Contains(string(buf[:n]), "text/event-stream") {
		t.Errorf("expected event-stream header, got: %s", buf[:n])
	}

	// Close connection to simulate abrupt client disconnect.
	conn.Close()

	// Server should handle the broken pipe gracefully.
	time.Sleep(100 * time.Millisecond)
}

func TestRealWire_ScanReturnsError(t *testing.T) {
	// Create daemon with duplicate rule IDs to trigger engine.Scan error.
	dupeRules := []rules.Rule{
		{ID: "dupe", Name: "A", Path: "/tmp/a", Category: types.CategorySystem, Safety: types.SafetySafe, SafetyNote: "test", Desc: "test"},
		{ID: "dupe", Name: "B", Path: "/tmp/b", Category: types.CategorySystem, Safety: types.SafetySafe, SafetyNote: "test", Desc: "test"},
	}
	socketPath, _, _ := startTestDaemonWithRules(t, dupeRules)
	client := NewUnixSocketHTTPClient(socketPath)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/api/v1/scan", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["error"] == nil {
		t.Error("expected error field in response")
	}
}

func TestRealWire_CleanReturnsError(t *testing.T) {
	socketPath, _ := startTestDaemon(t)
	client := NewUnixSocketHTTPClient(socketPath)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Request to clean non-existent item IDs — should succeed but with 0 cleaned.
	// Actually, Clean with empty items returns error.
	body := `{"item_ids":[],"strategy":"trash"}`
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/api/v1/clean", strings.NewReader(body))
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestRealWire_UndoNotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	socketPath, _ := startTestDaemon(t)
	client := NewUnixSocketHTTPClient(socketPath)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/api/v1/undo", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for no manifest, got %d", resp.StatusCode)
	}
}
