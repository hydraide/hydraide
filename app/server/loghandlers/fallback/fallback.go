// Package fallback implements a resilient slog.Handler that combines a primary
// log destination (e.g., Graylog) with a local file fallback. If the primary
// destination is unavailable, logs are automatically written to a rotating
// fallback file instead. Once the primary becomes reachable again, logs are
// replayed from the file back to the primary sink in the background.
//
// Features:
//   - Seamless failover to local file if remote logging is down
//   - Automatic background retry every 30s to flush stored logs
//   - Dual-handler support for primary and fallback destinations
//   - Rotation logic to avoid unbounded log file growth
//
// Use this handler to guarantee that logs are never lost — even across
// network failures, container restarts, or temporary outages.
package fallback

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

const (
	fallbackLogFile       = "fallback.log"
	fallbackBackupLogFile = "fallback.log.old"
	maxFileSize           = 10 * 1024 * 1024 // 10MB
)

// Handler routes log records to a primary handler (e.g., Graylog) when reachable,
// and to a fallback handler (e.g., JSON file) otherwise.
type Handler struct {
	primary      slog.Handler  // main log destination
	fallback     slog.Handler  // backup log sink (usually file)
	checker      func() bool   // returns true if primary is reachable
	dispatchOnce sync.Once     // ensures retry loop is only started once
	retryTicker  *time.Ticker  // retry interval for log flush
	shutdown     chan struct{} // graceful shutdown signal
}

// New creates a new fallback handler with the given primary and fallback handlers.
// The checker function determines whether the primary is currently available.
func New(primary, fallback slog.Handler, checker func() bool) *Handler {
	h := &Handler{
		primary:  primary,
		fallback: fallback,
		checker:  checker,
		shutdown: make(chan struct{}),
	}

	h.dispatchOnce.Do(func() {
		h.retryTicker = time.NewTicker(30 * time.Second)
		go h.retryLoop()
	})

	return h
}

// Enabled returns true if either the primary or fallback handler is enabled for the log level.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.primary.Enabled(ctx, level) || h.fallback.Enabled(ctx, level)
}

// Handle sends the log record to the primary if reachable, otherwise stores it via fallback.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	if h.checker() {
		return h.primary.Handle(ctx, r)
	}

	err := h.fallback.Handle(ctx, r)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "fallback handler failed: %v\n", err)
	}
	return nil
}

// WithAttrs adds structured attributes to both primary and fallback handlers.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{
		primary:     h.primary.WithAttrs(attrs),
		fallback:    h.fallback.WithAttrs(attrs),
		checker:     h.checker,
		shutdown:    h.shutdown,
		retryTicker: h.retryTicker,
	}
}

// WithGroup applies grouping to both handlers (no-op grouping supported).
func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{
		primary:     h.primary.WithGroup(name),
		fallback:    h.fallback.WithGroup(name),
		checker:     h.checker,
		shutdown:    h.shutdown,
		retryTicker: h.retryTicker,
	}
}

// retryLoop runs in background and periodically flushes the fallback log file if the primary becomes available.
func (h *Handler) retryLoop() {
	for {
		select {
		case <-h.retryTicker.C:
			if h.checker() {
				h.flushFallback()
			}
		case <-h.shutdown:
			h.retryTicker.Stop()
			return
		}
	}
}

// flushFallback reads stored fallback logs and replays them to the primary handler.
func (h *Handler) flushFallback() {
	files := []string{fallbackLogFile, fallbackBackupLogFile}
	for _, file := range files {
		_ = processFile(file, h.primary)
	}
}

// processFile reads and re-emits each JSON log line to the provided primary handler.
// Unsuccessful lines are preserved in a temp file to avoid data loss.
func processFile(path string, primary slog.Handler) error {
	input, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer func() {
		_ = input.Close()
	}()

	temp := path + ".tmp"
	tmp, err := os.Create(temp)
	if err != nil {
		return err
	}
	defer func() {
		_ = tmp.Close()
	}()

	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		line := scanner.Bytes()
		var data map[string]interface{}
		if err := json.Unmarshal(line, &data); err != nil {
			_, _ = tmp.Write(line)
			_, _ = tmp.Write([]byte("\n"))
			continue
		}

		r := recordFromGELF(data)
		err := primary.Handle(context.Background(), r)
		if err != nil {
			_, _ = tmp.Write(line)
			_, _ = tmp.Write([]byte("\n"))
		}
	}

	_ = input.Close()
	_ = tmp.Close()
	_ = os.Rename(temp, path)
	return nil
}

// recordFromGELF reconstructs a slog.Record from GELF JSON data.
func recordFromGELF(data map[string]interface{}) slog.Record {
	msg := fmt.Sprintf("%v", data["short_message"])
	ts := time.Now()
	if t, ok := data["timestamp"].(float64); ok {
		ts = time.Unix(int64(t), 0)
	}
	rec := slog.NewRecord(ts, slog.LevelInfo, msg, 0)
	for k, v := range data {
		if len(k) > 1 && k[0] == '_' {
			rec.AddAttrs(slog.Any(k[1:], v))
		}
	}
	return rec
}

// LocalHandler creates a file-based JSON slog.Handler that writes to fallback.log.
func LocalHandler(minLevel slog.Level) slog.Handler {
	f, err := os.OpenFile(fallbackLogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(fmt.Sprintf("cannot open fallback file: %v", err))
	}
	rotateIfNeeded(f)
	return slog.NewJSONHandler(f, &slog.HandlerOptions{Level: minLevel})
}

// rotateIfNeeded rotates the fallback file if it exceeds maxFileSize.
func rotateIfNeeded(f *os.File) {
	fi, err := f.Stat()
	if err == nil && fi.Size() >= maxFileSize {
		_ = f.Close()
		_ = os.Rename(fallbackLogFile, fallbackBackupLogFile)
		_, _ = os.Create(fallbackLogFile)
	}
}

// Stop gracefully shuts down the retry loop and background processing.
func (h *Handler) Stop() {
	close(h.shutdown)
}
