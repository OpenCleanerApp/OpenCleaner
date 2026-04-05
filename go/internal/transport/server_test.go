package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/opencleaner/opencleaner/internal/stream"
	"github.com/opencleaner/opencleaner/pkg/types"
)

type signalRecorder struct {
	*httptest.ResponseRecorder
	wrote chan struct{}
	once  sync.Once
}

func newSignalRecorder() *signalRecorder {
	return &signalRecorder{ResponseRecorder: httptest.NewRecorder(), wrote: make(chan struct{})}
}

func (r *signalRecorder) Write(p []byte) (int, error) {
	r.once.Do(func() { close(r.wrote) })
	return r.ResponseRecorder.Write(p)
}

func (r *signalRecorder) Flush() {
	r.ResponseRecorder.Flush()
}

func TestServer_StatusOK(t *testing.T) {
	srv := NewServer(nil, stream.NewBroker(), "/tmp/opencleaner.sock", "test")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)

	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "\"socket_path\"") {
		t.Fatalf("expected socket_path in response, got %q", rr.Body.String())
	}
}

func TestServer_ScanMethodNotAllowed(t *testing.T) {
	srv := NewServer(nil, stream.NewBroker(), "/tmp/opencleaner.sock", "test")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/scan", nil)

	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
}

func TestServer_CleanRejectsInvalidJSON(t *testing.T) {
	srv := NewServer(nil, stream.NewBroker(), "/tmp/opencleaner.sock", "test")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clean", strings.NewReader("not-json"))

	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestServer_CleanRejectsUnknownField(t *testing.T) {
	srv := NewServer(nil, stream.NewBroker(), "/tmp/opencleaner.sock", "test")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clean", strings.NewReader(`{"item_ids":["a"],"strategy":"trash","unknown":1}`))

	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestServer_ProgressStreamWritesData(t *testing.T) {
	broker := stream.NewBroker()
	srv := NewServer(nil, broker, "/tmp/opencleaner.sock", "test")

	broker.Publish(types.ProgressEvent{Type: "scanning", Progress: 0.2, Message: "starting"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rr := newSignalRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/progress/stream", nil).WithContext(ctx)

	done := make(chan struct{})
	go func() {
		srv.Handler().ServeHTTP(rr, req)
		close(done)
	}()

	select {
	case <-rr.wrote:
	case <-done:
		t.Fatal("stream handler exited without writing")
	case <-time.After(2 * time.Second):
		t.Fatal("stream handler did not write")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream handler did not exit")
	}

	body := rr.Body.String()
	if !strings.Contains(body, "data: ") {
		t.Fatalf("expected SSE data prefix, got %q", body)
	}
	if !strings.Contains(body, "\"type\":\"scanning\"") {
		t.Fatalf("expected scanning event, got %q", body)
	}
}
