package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/opencleaner/opencleaner/internal/audit"
	"github.com/opencleaner/opencleaner/internal/daemon"
	"github.com/opencleaner/opencleaner/internal/engine"
	"github.com/opencleaner/opencleaner/internal/rules"
	"github.com/opencleaner/opencleaner/internal/stream"
	"github.com/opencleaner/opencleaner/internal/transport"
	"github.com/opencleaner/opencleaner/pkg/logger"
)

var version = "dev"

func main() {
	socketPath := flag.String("socket", transport.DefaultSocketPath(), "unix socket path")
	install := flag.Bool("install", false, "install launchd plist and exit")
	logLevel := flag.String("log-level", "info", "log level: debug|info|warn|error")
	logJSON := flag.Bool("log-json", false, "emit logs as JSON")
	flag.Parse()

	level := parseLogLevel(*logLevel)
	var log *slog.Logger
	if *logJSON {
		log = logger.NewJSON(level)
	} else {
		log = logger.New(level)
	}

	if *install {
		exe, err := os.Executable()
		if err != nil {
			log.Error("failed to resolve executable", "err", err)
			os.Exit(1)
		}
		if err := daemon.InstallPlistWithSocket(exe, *socketPath); err != nil {
			log.Error("launchd install failed", "err", err)
			os.Exit(1)
		}
		log.Info("launchd plist installed", "plist", daemon.PlistPath())
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	home, err := os.UserHomeDir()
	if err != nil {
		log.Error("failed to resolve home dir", "err", err)
		os.Exit(1)
	}

	auditPath, err := audit.DefaultAuditLogPath()
	if err != nil {
		log.Error("failed to resolve audit log path", "err", err)
		os.Exit(1)
	}
	auditLogger := audit.NewLogger(auditPath)

	broker := stream.NewBroker()
	eng := engine.New(rules.BuiltinRules(home), broker, auditLogger)
	srv := transport.NewServer(eng, broker, *socketPath, version)

	ln, err := transport.ListenUnixSocket(*socketPath)
	if err != nil {
		log.Error("failed to listen", "socket", *socketPath, "err", err)
		os.Exit(1)
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

	log.Info("opencleanerd starting", "version", version, "socket", *socketPath)
	if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("http serve failed", "err", err)
		os.Exit(1)
	}
}

func parseLogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
