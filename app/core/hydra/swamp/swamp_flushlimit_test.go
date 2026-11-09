package swamp

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler"
	v2 "github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler/v2"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/flushlimit"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/metadata"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/hydraide/hydraide/app/name"
)

// diskOpCounter counts chronicler disk calls running at the same time across
// every swamp of a test, independently of the limiter's own counters.
type diskOpCounter struct {
	running atomic.Int64
	max     atomic.Int64
	calls   atomic.Int64
}

func (d *diskOpCounter) enter() {
	d.calls.Add(1)
	n := d.running.Add(1)
	for {
		m := d.max.Load()
		if n <= m || d.max.CompareAndSwap(m, n) {
			break
		}
	}
}

func (d *diskOpCounter) leave() { d.running.Add(-1) }

// slowChronicler models a slow disk (md RAID fsync): Write, Sync and Close
// take a few milliseconds, so unbounded flushes would overlap massively.
type slowChronicler struct {
	chronicler.Chronicler
	counter *diskOpCounter
	delay   time.Duration
}

func (c *slowChronicler) Write(treasures []treasure.Treasure) {
	c.counter.enter()
	defer c.counter.leave()
	time.Sleep(c.delay)
	c.Chronicler.Write(treasures)
}

func (c *slowChronicler) Sync() error {
	c.counter.enter()
	defer c.counter.leave()
	time.Sleep(c.delay)
	return c.Chronicler.Sync()
}

func (c *slowChronicler) Close() error {
	c.counter.enter()
	defer c.counter.leave()
	time.Sleep(c.delay)
	return c.Chronicler.Close()
}

// Many swamps flushing at once (inline flush on every Save with
// WriteInterval 0, then Close) never run more disk operations at the same
// time than the flush limit, and every record is on disk after Close.
func TestFlushLimit_ManySwampsBoundedAndDurable(t *testing.T) {
	const (
		limit      = 4
		swampCount = 150
		perSwamp   = 5
	)
	flushlimit.SetLimit(limit)
	t.Cleanup(func() { flushlimit.SetLimit(flushlimit.DefaultLimit) })
	flushlimit.ResetPeak()

	dataRoot := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(dataRoot, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	counter := &diskOpCounter{}
	hydPaths := make([]string, swampCount)

	var wg sync.WaitGroup
	for i := 0; i < swampCount; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			swampName := name.New().Sanctuary("flushlimit").Realm("many").Swamp(fmt.Sprintf("s%04d", i))
			hashPath := swampName.GetFullHashPath(dataRoot, dataLossIslandID, dataLossMaxDepth, dataLossMaxFolders)
			hydPaths[i] = hashPath + ".hyd"

			chron := chronicler.NewV2WithName(hashPath, dataLossMaxDepth, swampName.Get())
			chron.CreateDirectoryIfNotExists()
			slow := &slowChronicler{Chronicler: chron, counter: counter, delay: 2 * time.Millisecond}
			meta := metadata.NewNoop()
			meta.SetSwampName(swampName)

			fss := &FilesystemSettings{ChroniclerInterface: slow, WriteInterval: 0}
			sw := New(swampName, time.Hour, fss, func(*Event) {}, func(*Info) {}, func(name.Name) {}, meta)

			sw.BeginVigil()
			for j := 0; j < perSwamp; j++ {
				tr := sw.CreateTreasure(fmt.Sprintf("key-%d", j))
				guardID := tr.StartTreasureGuard(true)
				tr.SetContentString(guardID, fmt.Sprintf("swamp=%d record=%d", i, j))
				_ = tr.Save(guardID)
				tr.ReleaseTreasureGuard(guardID)
			}
			sw.CeaseVigil()
			sw.Close()
		}(i)
	}
	wg.Wait()

	t.Logf("disk calls=%d, max concurrent disk calls=%d, limiter peak=%d, limit=%d",
		counter.calls.Load(), counter.max.Load(), flushlimit.Peak(), limit)

	if got := counter.max.Load(); got > limit {
		t.Fatalf("%d disk operations ran at once, limit is %d", got, limit)
	}
	if got := counter.max.Load(); got < 2 {
		t.Fatalf("only %d disk operation ran at once; the test did not create contention", got)
	}
	if got := flushlimit.Peak(); got > limit {
		t.Fatalf("limiter peak %d above limit %d", got, limit)
	}
	if flushlimit.InFlight() != 0 {
		t.Fatalf("flush slots still held after all closes: %d", flushlimit.InFlight())
	}

	for i, p := range hydPaths {
		reader, err := v2.NewFileReader(p)
		if err != nil {
			t.Fatalf("swamp %d: open %s: %v", i, p, err)
		}
		entries := 0
		if _, err := reader.ReadAllEntries(func(v2.Entry) bool { entries++; return true }); err != nil {
			_ = reader.Close()
			t.Fatalf("swamp %d: read: %v", i, err)
		}
		_ = reader.Close()
		if entries != perSwamp {
			t.Fatalf("swamp %d: %d entries on disk, want %d", i, entries, perSwamp)
		}
	}
}

// A swamp whose flush waits for a slot still closes and persists once the
// slot comes back, and Close does not finish while the limiter is full.
func TestFlushLimit_CloseWaitsForSlot(t *testing.T) {
	flushlimit.SetLimit(1)
	t.Cleanup(func() { flushlimit.SetLimit(flushlimit.DefaultLimit) })

	dataRoot := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(dataRoot, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	swampName := name.New().Sanctuary("flushlimit").Realm("wait").Swamp("one")
	hashPath := swampName.GetFullHashPath(dataRoot, dataLossIslandID, dataLossMaxDepth, dataLossMaxFolders)
	chron := chronicler.NewV2WithName(hashPath, dataLossMaxDepth, swampName.Get())
	chron.CreateDirectoryIfNotExists()
	meta := metadata.NewNoop()
	meta.SetSwampName(swampName)
	fss := &FilesystemSettings{ChroniclerInterface: chron, WriteInterval: time.Hour}
	sw := New(swampName, time.Hour, fss, func(*Event) {}, func(*Info) {}, func(name.Name) {}, meta)

	sw.BeginVigil()
	tr := sw.CreateTreasure("k")
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentString(guardID, "v")
	_ = tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)
	sw.CeaseVigil()

	release := flushlimit.Acquire()
	closed := make(chan struct{})
	go func() {
		sw.Close()
		close(closed)
	}()

	select {
	case <-closed:
		release()
		t.Fatal("Close finished while the limiter was full")
	case <-time.After(200 * time.Millisecond):
	}
	release()

	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not finish after the slot was released")
	}

	reader, err := v2.NewFileReader(hashPath + ".hyd")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer reader.Close()
	entries := 0
	if _, err := reader.ReadAllEntries(func(v2.Entry) bool { entries++; return true }); err != nil {
		t.Fatalf("read: %v", err)
	}
	if entries != 1 {
		t.Fatalf("%d entries on disk, want 1", entries)
	}
}
