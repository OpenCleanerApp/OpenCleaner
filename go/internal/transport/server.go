package transport

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/opencleaner/opencleaner/internal/engine"
	"github.com/opencleaner/opencleaner/internal/scheduler"
	"github.com/opencleaner/opencleaner/internal/stream"
	"github.com/opencleaner/opencleaner/pkg/types"
)

type Server struct {
	engine     *engine.Engine
	broker     *stream.Broker
	scheduler  *scheduler.Scheduler
	socketPath string
	version    string
}

func NewServer(engine *engine.Engine, broker *stream.Broker, socketPath, version string) *Server {
	return &Server{engine: engine, broker: broker, socketPath: socketPath, version: version}
}

// SetScheduler attaches the scheduler (called after construction to avoid circular init).
func (s *Server) SetScheduler(sched *scheduler.Scheduler) {
	s.scheduler = sched
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/status", s.handleStatus)
	mux.HandleFunc("/api/v1/scan", s.handleScan)
	mux.HandleFunc("/api/v1/clean", s.handleClean)
	mux.HandleFunc("/api/v1/undo", s.handleUndo)
	mux.HandleFunc("/api/v1/scheduler", s.handleScheduler)
	mux.HandleFunc("/api/v1/progress/stream", s.handleProgressStream)
	return mux
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, types.DaemonStatus{OK: true, Version: s.version, SocketPath: s.socketPath})
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	res, err := s.engine.Scan(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleClean(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req types.CleanRequest
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}

	if req.Strategy == "" {
		req.Strategy = types.CleanStrategyTrash
	}
	res, err := s.engine.Clean(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleUndo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	res, err := s.engine.Undo(r.Context())
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, os.ErrNotExist) {
			code = http.StatusNotFound
		}
		writeJSON(w, code, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleProgressStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	debug := os.Getenv("OPENCLEANER_DEBUG") != "" || os.Getenv("OPENCLEANER_DEBUG_SSE") != ""
	defer func() {
		if debug {
			log.Printf("sse: unsubscribe remote=%s subs=%d", r.RemoteAddr, s.broker.SubscriberCount())
		}
	}()

	rc := http.NewResponseController(w)
	ch := s.broker.Subscribe(r.Context())
	if debug {
		log.Printf("sse: subscribe remote=%s subs=%d", r.RemoteAddr, s.broker.SubscriberCount())
	}
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			_ = rc.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				if debug {
					log.Printf("sse: ping write failed remote=%s err=%v", r.RemoteAddr, err)
				}
				return
			}
			f.Flush()
		case evt, ok := <-ch:
			if !ok {
				return
			}
			b, err := json.Marshal(evt)
			if err != nil {
				if debug {
					log.Printf("sse: marshal failed remote=%s err=%v", r.RemoteAddr, err)
				}
				return
			}

			var buf bytes.Buffer
			buf.Grow(len("data: ") + len(b) + 2)
			buf.WriteString("data: ")
			buf.Write(b)
			buf.WriteString("\n\n")

			_ = rc.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := w.Write(buf.Bytes()); err != nil {
				if debug {
					log.Printf("sse: event write failed remote=%s err=%v", r.RemoteAddr, err)
				}
				return
			}
			f.Flush()
		}
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleScheduler(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "scheduler not initialized"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.scheduler.Status())

	case http.MethodPut:
		var cfg scheduler.Schedule
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		home, _ := os.UserHomeDir()
		if err := s.scheduler.UpdateConfig(cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		_ = scheduler.SaveConfig(home, cfg)
		writeJSON(w, http.StatusOK, s.scheduler.Status())

	case http.MethodDelete:
		s.scheduler.Stop()
		home, _ := os.UserHomeDir()
		_ = scheduler.RemoveConfig(home)
		writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})

	default:
		w.Header().Set("Allow", "GET, PUT, DELETE")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}
