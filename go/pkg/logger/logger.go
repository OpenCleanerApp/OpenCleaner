package logger

import (
	"io"
	"log/slog"
	"os"
)

func New(level slog.Level) *slog.Logger {
	return newWithHandler(os.Stderr, func(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
		return slog.NewTextHandler(w, opts)
	}, level)
}

func NewJSON(level slog.Level) *slog.Logger {
	return newWithHandler(os.Stderr, func(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
		return slog.NewJSONHandler(w, opts)
	}, level)
}

func newWithHandler(w io.Writer, mk func(io.Writer, *slog.HandlerOptions) slog.Handler, level slog.Level) *slog.Logger {
	return slog.New(mk(w, &slog.HandlerOptions{Level: level}))
}
