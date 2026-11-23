// Package observability provides structured logging and Prometheus metrics.
package observability

import (
	"io"
	"log/slog"
)

// NewLogger returns a JSON (production) or text (local) structured logger.
func NewLogger(w io.Writer, format string, level slog.Level, version string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if format == "text" {
		h = slog.NewTextHandler(w, opts)
	} else {
		h = slog.NewJSONHandler(w, opts)
	}
	return slog.New(h).With("service", "nebula", "version", version)
}
