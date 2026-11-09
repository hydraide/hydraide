package hydra

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/flushlimit"
	"github.com/hydraide/hydraide/app/name"
)

// GracefulStop with every flush slot taken must wait (closes queue on the
// limiter), and once the slots come back it must flush and close every swamp
// with all data on disk.
func TestGracefulStop_FullFlushLimiterFinishes(t *testing.T) {
	const (
		limit      = 4
		swampCount = 120
		perSwamp   = 10
	)
	flushlimit.SetLimit(limit)
	t.Cleanup(func() { flushlimit.SetLimit(flushlimit.DefaultLimit) })
	flushlimit.ResetPeak()

	dataRoot := t.TempDir()
	// long idle: only GracefulStop closes the swamps
	h, _ := newHydraForDataLoss(t, dataRoot, 3600)

	swampNames := make([]name.Name, swampCount)
	for i := range swampNames {
		swampNames[i] = name.New().Sanctuary(dataLossHydraSanctuary).Realm("flushlimit").Swamp(fmt.Sprintf("s%04d", i))
	}

	var wg sync.WaitGroup
	for i := range swampNames {
		wg.Add(1)
		go func(n name.Name) {
			defer wg.Done()
			saveTreasuresViaHydra(t, h, n, 0, perSwamp)
		}(swampNames[i])
	}
	wg.Wait()

	// take every slot: no flush or close can reach the disk now
	releases := make([]func(), limit)
	for i := range releases {
		releases[i] = flushlimit.Acquire()
	}

	stopped := make(chan struct{})
	go func() {
		h.(*hydra).gracefulStopWithin(60*time.Second, 50*time.Second, 20*time.Millisecond)
		close(stopped)
	}()

	select {
	case <-stopped:
		for _, r := range releases {
			r()
		}
		t.Fatal("GracefulStop returned while every flush slot was taken")
	case <-time.After(300 * time.Millisecond):
	}
	if flushlimit.Waiting() == 0 {
		t.Fatal("no flush is waiting for a slot; the closes did not queue on the limiter")
	}

	for _, r := range releases {
		r()
	}

	select {
	case <-stopped:
	case <-time.After(55 * time.Second):
		t.Fatal("GracefulStop did not finish after the slots were released")
	}

	if n := h.CountActiveSwamps(); n != 0 {
		t.Fatalf("%d swamps still open after GracefulStop", n)
	}
	if p := flushlimit.Peak(); p > limit {
		t.Fatalf("limiter peak %d above limit %d", p, limit)
	}
	if flushlimit.InFlight() != 0 {
		t.Fatalf("flush slots still held: %d", flushlimit.InFlight())
	}

	// reopen on the same data and count every swamp
	h2, _ := newHydraForDataLoss(t, dataRoot, 3600)
	for i, n := range swampNames {
		if got := countViaHydra(t, h2, n); got != perSwamp {
			t.Fatalf("swamp %d: %d treasures after reopen, want %d", i, got, perSwamp)
		}
	}
	h2.(*hydra).gracefulStopWithin(30*time.Second, 20*time.Second, 20*time.Millisecond)
}
