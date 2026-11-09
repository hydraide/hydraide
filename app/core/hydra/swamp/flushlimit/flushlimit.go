// Package flushlimit caps how many swamps write to disk at the same time,
// process-wide.
//
// Every dirty swamp flushes from its own goroutine (write ticker, idle close,
// inline flush on Save with WriteInterval 0, destroy, forced compaction). A
// goroutine blocked in a write or fsync syscall pins an OS thread, and the Go
// runtime creates a new thread for every other runnable goroutine. Under a
// bulk load that touches thousands of swamps on a slow disk this used to
// pile up thousands of threads until the runtime hit its 10000-thread limit
// and killed the process ("fatal error: thread exhaustion").
//
// The limiter is a counting semaphore around the disk part of those paths.
// Flushes over the cap wait on a channel, which parks the goroutine without
// holding an OS thread, so the thread count stays near the cap.
//
// Lock ordering rule for callers: acquire a slot only around pure disk work
// (chronicler Write/Sync/Close/Destroy/Compaction, metadata save), never
// while holding a treasure guard, and never acquire a second slot while
// holding one. A slot holder may then only block on its own swamp's
// chronicler mutex, the guards of the treasures it writes, and the disk;
// none of those holders waits for a slot, so slots always come back.
package flushlimit

import (
	"sync/atomic"
)

// DefaultLimit is the number of concurrent swamp flushes allowed when the
// server does not configure a value.
const DefaultLimit = 64

type limiter struct {
	slots chan struct{}
}

var (
	current  atomic.Pointer[limiter]
	inFlight atomic.Int64
	peak     atomic.Int64
	waiting  atomic.Int64
)

func init() {
	current.Store(newLimiter(DefaultLimit))
}

func newLimiter(n int) *limiter {
	if n <= 0 {
		n = DefaultLimit
	}
	return &limiter{slots: make(chan struct{}, n)}
}

// SetLimit sets the process-wide number of concurrent flushes. A value <= 0
// restores DefaultLimit. Meant to be called once at server start; calling it
// later is safe (a flush that already holds a slot releases it to the
// limiter it came from), but the old and new caps briefly add up.
func SetLimit(n int) {
	current.Store(newLimiter(n))
}

// Limit returns the configured number of concurrent flushes.
func Limit() int {
	return cap(current.Load().slots)
}

// Acquire blocks until a flush slot is free and returns the function that
// gives it back. The release function must be called exactly once; extra
// calls are ignored.
func Acquire() (release func()) {
	l := current.Load()
	select {
	case l.slots <- struct{}{}:
	default:
		waiting.Add(1)
		l.slots <- struct{}{}
		waiting.Add(-1)
	}

	n := inFlight.Add(1)
	for {
		p := peak.Load()
		if n <= p || peak.CompareAndSwap(p, n) {
			break
		}
	}

	var released atomic.Bool
	return func() {
		if !released.CompareAndSwap(false, true) {
			return
		}
		inFlight.Add(-1)
		<-l.slots
	}
}

// Do runs fn while holding a flush slot.
func Do(fn func()) {
	release := Acquire()
	defer release()
	fn()
}

// InFlight returns the number of flushes holding a slot right now.
func InFlight() int64 {
	return inFlight.Load()
}

// Waiting returns the number of flushes waiting for a slot right now.
func Waiting() int64 {
	return waiting.Load()
}

// Peak returns the highest InFlight value seen since the last ResetPeak.
func Peak() int64 {
	return peak.Load()
}

// ResetPeak sets Peak to the current InFlight value.
func ResetPeak() {
	peak.Store(inFlight.Load())
}
