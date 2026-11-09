package flushlimit

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The limiter never lets more than Limit() callers run at once, and every
// caller eventually runs.
func TestDo_BoundsConcurrency(t *testing.T) {
	SetLimit(4)
	t.Cleanup(func() { SetLimit(DefaultLimit) })
	ResetPeak()

	var running, maxRunning, done atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Do(func() {
				n := running.Add(1)
				for {
					m := maxRunning.Load()
					if n <= m || maxRunning.CompareAndSwap(m, n) {
						break
					}
				}
				time.Sleep(time.Millisecond)
				running.Add(-1)
				done.Add(1)
			})
		}()
	}
	wg.Wait()

	if got := maxRunning.Load(); got > 4 {
		t.Fatalf("max concurrent = %d, limit 4", got)
	}
	if got := Peak(); got > 4 {
		t.Fatalf("Peak = %d, limit 4", got)
	}
	if done.Load() != 200 {
		t.Fatalf("done = %d, want 200", done.Load())
	}
	if InFlight() != 0 || Waiting() != 0 {
		t.Fatalf("leftover in-flight=%d waiting=%d", InFlight(), Waiting())
	}
}

// A full limiter blocks Acquire until a slot comes back; a double release
// does not free an extra slot.
func TestAcquire_BlocksWhenFull(t *testing.T) {
	SetLimit(1)
	t.Cleanup(func() { SetLimit(DefaultLimit) })

	release := Acquire()
	got := make(chan struct{})
	go func() {
		r := Acquire()
		close(got)
		r()
	}()

	select {
	case <-got:
		t.Fatal("second Acquire did not wait for the full limiter")
	case <-time.After(100 * time.Millisecond):
	}
	if Waiting() != 1 {
		t.Fatalf("Waiting = %d, want 1", Waiting())
	}

	release()
	release() // ignored
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("second Acquire did not get the released slot")
	}

	r := Acquire()
	r()
	if InFlight() != 0 {
		t.Fatalf("InFlight = %d after all releases", InFlight())
	}
}

func TestSetLimit_DefaultOnNonPositive(t *testing.T) {
	SetLimit(0)
	if Limit() != DefaultLimit {
		t.Fatalf("Limit = %d, want %d", Limit(), DefaultLimit)
	}
	SetLimit(7)
	if Limit() != 7 {
		t.Fatalf("Limit = %d, want 7", Limit())
	}
	SetLimit(DefaultLimit)
}
