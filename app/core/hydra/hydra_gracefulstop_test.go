package hydra

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp"
	"github.com/hydraide/hydraide/app/name"
)

// fakeCloseSwamp is a swamp whose Close is slow. Only the methods the
// shutdown path calls are implemented; the embedded interface is nil.
type fakeCloseSwamp struct {
	swamp.Swamp
	h         *hydra
	swampName name.Name
	closing   int32
	// beforeRemove runs before the swamp leaves the swamps map, afterRemove
	// after it (models a flush that is still running after the map delete).
	beforeRemove func()
	afterRemove  func()
	finished     int32
}

func (f *fakeCloseSwamp) GetName() name.Name      { return f.swampName }
func (f *fakeCloseSwamp) IsClosing() bool         { return atomic.LoadInt32(&f.closing) == 1 }
func (f *fakeCloseSwamp) StopSendingInformation() {}
func (f *fakeCloseSwamp) StopSendingEvents()      {}
func (f *fakeCloseSwamp) Close() {
	// Idempotent like the real swamp: only the first caller tears down.
	if !atomic.CompareAndSwapInt32(&f.closing, 0, 1) {
		return
	}
	if f.beforeRemove != nil {
		f.beforeRemove()
	}
	f.h.closeEventCallbackFunction(f.swampName)
	if f.afterRemove != nil {
		f.afterRemove()
	}
	atomic.StoreInt32(&f.finished, 1)
}

func addFakeSwamp(h *hydra, swampName string, before, after func()) *fakeCloseSwamp {
	n := name.New().Sanctuary("test").Realm("gracefulstop").Swamp(swampName)
	f := &fakeCloseSwamp{h: h, swampName: n, beforeRemove: before, afterRemove: after}
	h.swamps.Store(n.Get(), f)
	return f
}

func runGracefulStop(h *hydra, budget, forceAfter time.Duration) time.Duration {
	start := time.Now()
	h.gracefulStopWithin(budget, forceAfter, 20*time.Millisecond)
	return time.Since(start)
}

// GracefulStop must wait for a slow close, also past the force-close point
// (the force pass calls Close again, which returns at once for a swamp that
// is already closing).
func TestGracefulStop_WaitsForSlowSwampClose(t *testing.T) {
	h := &hydra{}
	slow := addFakeSwamp(h, "slow", func() { time.Sleep(600 * time.Millisecond) }, nil)
	fast := addFakeSwamp(h, "fast", nil, nil)

	elapsed := runGracefulStop(h, 10*time.Second, 100*time.Millisecond)

	if atomic.LoadInt32(&slow.finished) != 1 || atomic.LoadInt32(&fast.finished) != 1 {
		t.Fatalf("GracefulStop returned before all closes finished")
	}
	if elapsed < 600*time.Millisecond {
		t.Fatalf("GracefulStop returned too early: %v", elapsed)
	}
	if h.CountActiveSwamps() != 0 {
		t.Fatalf("swamps still open: %d", h.CountActiveSwamps())
	}
}

// A close goroutine that is still flushing after its swamp left the map must
// also be waited for.
func TestGracefulStop_WaitsForCloserAfterMapRemoval(t *testing.T) {
	h := &hydra{}
	f := addFakeSwamp(h, "late-flush", nil, func() { time.Sleep(500 * time.Millisecond) })

	elapsed := runGracefulStop(h, 10*time.Second, 5*time.Second)

	if atomic.LoadInt32(&f.finished) != 1 {
		t.Fatalf("GracefulStop returned while the close goroutine was still running (after %v)", elapsed)
	}
}

// When a close never finishes, GracefulStop returns once the budget is spent.
func TestGracefulStop_ReturnsAfterBudget(t *testing.T) {
	h := &hydra{}
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	addFakeSwamp(h, "stuck", func() { <-release }, nil)

	budget := 400 * time.Millisecond
	elapsed := runGracefulStop(h, budget, 100*time.Millisecond)

	if elapsed < budget {
		t.Fatalf("GracefulStop returned before the budget: %v", elapsed)
	}
	if elapsed > budget+2*time.Second {
		t.Fatalf("GracefulStop did not return after the budget: %v", elapsed)
	}
	if h.CountActiveSwamps() != 1 {
		t.Fatalf("expected the stuck swamp to stay open, got %d", h.CountActiveSwamps())
	}
}

// No open swamp: GracefulStop returns at once.
func TestGracefulStop_NoSwamps(t *testing.T) {
	h := &hydra{}
	if elapsed := runGracefulStop(h, 10*time.Second, 5*time.Second); elapsed > time.Second {
		t.Fatalf("GracefulStop with no swamps took %v", elapsed)
	}
}

// Many swamps close in parallel: 64 swamps of 200 ms each finish far below
// the sequential 12.8 s.
func TestGracefulStop_ClosesInParallel(t *testing.T) {
	h := &hydra{}
	for i := 0; i < 64; i++ {
		addFakeSwamp(h, "p"+string(rune('a'+i%26))+string(rune('a'+i/26)), func() { time.Sleep(200 * time.Millisecond) }, nil)
	}
	elapsed := runGracefulStop(h, 30*time.Second, 20*time.Second)
	if h.CountActiveSwamps() != 0 {
		t.Fatalf("swamps still open: %d", h.CountActiveSwamps())
	}
	if elapsed > 3*time.Second {
		t.Fatalf("closing looks sequential: %v", elapsed)
	}
}
