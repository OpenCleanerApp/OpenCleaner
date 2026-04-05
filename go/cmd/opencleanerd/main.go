package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/opencleaner/opencleaner/internal/audit"
	"github.com/opencleaner/opencleaner/internal/engine"
	"github.com/opencleaner/opencleaner/internal/rules"
	"github.com/opencleaner/opencleaner/internal/stream"
	"github.com/opencleaner/opencleaner/internal/transport"
)

var version = "dev"

func main() {
	socketPath := flag.String("socket", transport.DefaultSocketPath(), "unix socket path")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	auditPath, err := audit.DefaultAuditLogPath()
	if err != nil {
		log.Fatal(err)
	}
	auditLogger := audit.NewLogger(auditPath)

	broker := stream.NewBroker()
	eng := engine.New(rules.BuiltinRules(home), broker, auditLogger)
	srv := transport.NewServer(eng, broker, *socketPath, version)

	ln, err := transport.ListenUnixSocket(*socketPath)
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()
	defer os.Remove(*socketPath)

	httpSrv := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	fmt.Printf("opencleanerd listening on unix socket %s\n", *socketPath)
	if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
