// Package threadguard watches the number of OS threads the server created.
//
// The Go runtime never gives a created OS thread back, so the thread count
// only grows and stays at its peak. When it reaches the runtime's limit
// (debug.SetMaxThreads, 10000 by default) the process dies with "fatal
// error: thread exhaustion". Threads pile up when many goroutines block in
// syscalls at once (write, fsync on a slow disk). The flush limiter keeps
// that bounded; this package is the safety net and the early warning.
package threadguard

import (
	"context"
	"log/slog"
	"runtime/debug"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/hydraide/hydraide/app/panichandler"
)

const (
	// DefaultMaxThreads is the runtime thread limit the server sets at start.
	// It is a net only: a server that needs this many threads is already in
	// trouble, but it keeps running and logs instead of dying.
	DefaultMaxThreads = 30000
	// DefaultWarnThreshold is the OS thread count above which the watchdog
	// logs a warning.
	DefaultWarnThreshold = 2000
	// minWarnGap is the shortest time between two warnings.
	minWarnGap = time.Minute
)

// SetMaxThreads sets the Go runtime's OS thread limit. Values <= 0 use
// DefaultMaxThreads. It returns the previous limit.
func SetMaxThreads(n int) int {
	if n <= 0 {
		n = DefaultMaxThreads
	}
	return debug.SetMaxThreads(n)
}

// ThreadCount returns the number of OS threads the runtime created. Created
// threads are never released, so this is also the peak.
func ThreadCount() int {
	return pprof.Lookup("threadcreate").Count()
}

// warner decides when a thread count is worth a warning: above the
// threshold, higher than the count of the last warning (the count never
// drops, so a flat count says nothing new), and at most once per minWarnGap.
type warner struct {
	mu            sync.Mutex
	threshold     int
	lastWarnCount int
	lastWarnAt    time.Time
}

func (w *warner) shouldWarn(count int, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.threshold <= 0 || count < w.threshold || count <= w.lastWarnCount {
		return false
	}
	if !w.lastWarnAt.IsZero() && now.Sub(w.lastWarnAt) < minWarnGap {
		return false
	}
	w.lastWarnCount = count
	w.lastWarnAt = now
	return true
}

// StartWatchdog checks the OS thread count every interval and logs a warning
// when it crossed threshold (<= 0 uses DefaultWarnThreshold). extra adds
// attributes to the warning (e.g. flush limiter state); it may be nil.
func StartWatchdog(ctx context.Context, threshold int, interval time.Duration, extra func() []any) {
	if threshold <= 0 {
		threshold = DefaultWarnThreshold
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	w := &warner{threshold: threshold}
	panichandler.SafeGo("thread-watchdog", func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				count := ThreadCount()
				if !w.shouldWarn(count, now) {
					continue
				}
				attrs := []any{"threads", count, "threshold", threshold}
				if extra != nil {
					attrs = append(attrs, extra()...)
				}
				slog.Warn("OS thread count is high; threads blocked in syscalls (slow disk?) pile up and are never released", attrs...)
			}
		}
	})
}
