// Package graylog implements a slog.Handler that sends logs to Graylog using
// asynchronous background dispatch with fallback awareness.
package graylog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
)

// Handler is a slog.Handler implementation that sends log messages to a Graylog server via TCP asynchronously.
// It uses an internal queue and a background dispatcher to avoid blocking the main execution flow.
// If the connection fails, it retries automatically and maintains the queue until shutdown.
type Handler struct {
	address string      // Graylog TCP address (e.g., "127.0.0.1:12201")
	host    string      // Logical hostname or service identifier sent as the GELF "host" field
	level   slog.Level  // Minimum log level to emit (e.g., Info, Warn, Error)
	attrs   []slog.Attr // Static attributes included with every log record

	queue    chan []byte        // Buffered channel for asynchronous log message delivery
	ctx      context.Context    // Context for graceful shutdown of the dispatcher
	cancel   context.CancelFunc // Cancel function to signal dispatcher shutdown
	once     sync.Once          // Ensures dispatcher is started only once
	retryMux sync.Mutex         // Mutex to synchronize reconnect attempts
	conn     net.Conn           // Active TCP connection to Graylog
}

const queueSize = 1000 // Max number of pending messages before dropping new ones

// New creates a new asynchronous Graylog handler with a background dispatcher.
// It connects to the specified Graylog address and starts a goroutine that sends logs from a buffered queue.
func New(address, host string, level slog.Level) (*Handler, error) {
	ctx, cancel := context.WithCancel(context.Background())
	h := &Handler{
		address: address,
		host:    host,
		level:   level,
		queue:   make(chan []byte, queueSize),
		ctx:     ctx,
		cancel:  cancel,
	}
	go h.dispatcher() // Start background log dispatcher
	return h, nil
}

// Enabled reports whether a given log level is enabled for this handler.
// This allows slog to skip formatting logs that would not be sent.
func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// WithAttrs returns a shallow copy of the handler with additional attributes added.
// These attributes will be included in all subsequent log messages emitted by the handler.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{
		address: h.address,
		host:    h.host,
		level:   h.level,
		attrs:   append(h.attrs, attrs...), // merge existing and new attributes
		queue:   h.queue,
		ctx:     h.ctx,
		cancel:  h.cancel,
	}
}

// WithGroup returns the same handler, as attribute grouping is not supported in this implementation.
func (h *Handler) WithGroup(_ string) slog.Handler {
	return h
}

// Handle processes a single log record and enqueues it for asynchronous delivery to Graylog.
// It transforms the slog.Record into a GELF-compatible JSON object, appends static and dynamic attributes,
// and sends it to the internal queue for background dispatch.
func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	extras := map[string]interface{}{}

	// Merge static attributes from handler setup
	for _, attr := range h.attrs {
		extras[attr.Key] = attr.Value.Any()
	}

	// Merge dynamic attributes from the log record
	r.Attrs(func(attr slog.Attr) bool {
		extras[attr.Key] = attr.Value.Any()
		return true
	})

	// Extract and optionally isolate stack trace
	var stack string
	if s, ok := extras["stack"]; ok {
		stack, _ = s.(string)
		delete(extras, "stack") // avoid duplicate field in GELF payload
	}

	// Base GELF fields required by Graylog
	msg := map[string]interface{}{
		"version":       "1.1",
		"host":          h.host,
		"short_message": r.Message,
		"timestamp":     float64(r.Time.UnixNano()) / 1e9,
		"level":         convertLevel(r.Level),
	}

	// Add full_message only if stack is available
	if stack != "" {
		msg["full_message"] = stack
		msg["_stack_only"] = true // optional internal flag for debugging
	}

	// Prefix extra attributes with "_" as required by GELF spec
	for k, v := range extras {
		msg["_"+k] = v
	}

	// Marshal message to JSON format
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal GELF: %w", err)
	}

	// Enqueue for background delivery; drop if queue is full
	select {
	case h.queue <- append(data, '\n'):
		return nil
	default:
		return errors.New("graylog queue is full") // optional: could write to fallback
	}
}

// dispatcher runs in a background goroutine and sends log messages from the queue to the Graylog server.
// It maintains a single persistent TCP connection, reconnecting if the connection is lost.
func (h *Handler) dispatcher() {
	var err error

	for {
		select {
		case <-h.ctx.Done():
			// Graceful shutdown: close connection if active
			if h.conn != nil {
				_ = h.conn.Close()
			}
			return

		case msg := <-h.queue:
			func() {
				h.retryMux.Lock()
				defer h.retryMux.Unlock()

				// Establish TCP connection if not already connected
				if h.conn == nil {
					fmt.Println("Graylog connection not established, attempting to connect...")

					h.conn, err = net.Dial("tcp", h.address)
					if err != nil {
						return // skip this message, keep it in queue
					}
				}

				// Write message + NULL byte (0x00 terminator required by GELF TCP)
				_, err := h.conn.Write(append(msg, 0x00))
				if err != nil {
					_ = h.conn.Close()
					h.conn = nil
					return
				}

			}()
		}
	}
}

// convertLevel maps slog.Level values to GELF numerical levels.
// GELF uses syslog-style severity levels (0=emergency to 7=debug).
func convertLevel(level slog.Level) int {
	switch level {
	case slog.LevelDebug:
		return 7 // debug
	case slog.LevelInfo:
		return 6 // info
	case slog.LevelWarn:
		return 4 // warning
	case slog.LevelError:
		return 3 // error
	default:
		return 6 // fallback to info
	}
}

// Close gracefully shuts down the handler, stopping the dispatcher goroutine
// and releasing the TCP connection to Graylog.
func (h *Handler) Close() error {
	h.once.Do(func() {
		h.cancel() // cancel dispatcher context
	})
	return nil
}
