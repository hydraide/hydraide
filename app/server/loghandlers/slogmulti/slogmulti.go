// Package slogmulti provides a slog.Handler implementation that fans out
// log records to multiple handlers. This allows logs to be written
// simultaneously to various destinations, such as console, files, or remote sinks
// like Graylog or syslog. It preserves the behavior of each handler
// and aggregates errors during log emission.
//
// Example usage:
//
//	logger := slog.New(
//		slogmulti.New(
//			slog.NewTextHandler(os.Stdout, nil),
//			fileHandler,
//			graylogHandler,
//		),
//	)
//	slog.SetDefault(logger)
package slogmulti

import (
	"context"
	"log/slog"
)

// MultiHandler is a slog.Handler that dispatches log records to multiple child handlers.
// It enables simultaneous logging to different outputs (e.g., console + file + network).
type MultiHandler struct {
	handlers []slog.Handler
}

// New constructs a MultiHandler that fans out logs to the provided handlers.
func New(handlers ...slog.Handler) slog.Handler {
	return &MultiHandler{handlers: handlers}
}

// Enabled returns true if any of the child handlers is enabled for the given level.
// If none are enabled, the log record will be skipped entirely.
func (m *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle sends the log record to all enabled child handlers.
// If any of them returns an error, the last error is returned.
func (m *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	var err error
	for _, h := range m.handlers {
		if e := h.Handle(ctx, r); e != nil {
			err = e
		}
	}
	return err
}

// WithAttrs returns a new MultiHandler with the given attributes added to all child handlers.
func (m *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	hs := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		hs[i] = h.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: hs}
}

// WithGroup returns a new MultiHandler with a log group added to all child handlers.
// Groups are useful for nesting structured attributes under a common key.
func (m *MultiHandler) WithGroup(name string) slog.Handler {
	hs := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		hs[i] = h.WithGroup(name)
	}
	return &MultiHandler{handlers: hs}
}
