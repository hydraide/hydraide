package lock

import (
	"context"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Ezek a tesztek a Lock implementációt stresszelik a meglévő unit-tesztek
// által NEM lefedett pontokon:
//
//  1. Hot-key contention: sok goroutine vár UGYANAZON a key-en. Itt jelenik
//     meg a busy-wait spinlock (for { select { default: continue } }) hatása.
//  2. Goroutine-leak: minden megszerzett lock indít egy auto-unlock goroutine-t,
//     ami a TTL vagy ctx-Done-ig fut. Az Unlock NEM állítja le ezt — csak a
//     callerID-t veszi ki a queue-ból. Ha sok lock van rövid idő alatt,
//     halmozódnak a goroutine-ek.
//  3. Queue-leak: a sync.Map soha nem törli az üres queue-kat.
//
// A tesztek a fix ELŐTT várhatóan a baseline-t adják (rossz P99, sok goroutine),
// a fix UTÁN szigorú threshold-okkal kell zöldnek lenniük.

// TestLockHotKeyContention — 50 goroutine ugyanazon a key-en, mindegyik
// 1ms-os "munkát" végez lock alatt. P99 lock-szerzési időt mérjük.
//
// Egy egészséges sor-alapú lock: ha 50 goroutine sorban vár, és minden
// 1ms-ot tart, akkor a 49. helyen lévő ~50ms-ot vár. P99 max 100ms körül.
// Busy-wait spinlock alatt: a CPU-éhes várakozók eltolják az aktív
// goroutine-t, így a P99 másodperces tartományba mehet kis goroutine-szám
// mellett is (és ha a CPU tényleg betelt, ctx timeout-ok jönnek).
func TestLockHotKeyContention(t *testing.T) {
	l := New()
	const (
		numGoroutines = 50
		workDuration  = 1 * time.Millisecond
		ctxTimeout    = 30 * time.Second
		lockTTL       = 10 * time.Second
	)

	latencies := make([]time.Duration, 0, numGoroutines)
	var latMu sync.Mutex
	var ctxTimeouts atomic.Int64
	var success atomic.Int64

	var wg sync.WaitGroup
	startGate := make(chan struct{})

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startGate

			ctx, cancel := context.WithTimeout(context.Background(), ctxTimeout)
			defer cancel()

			lockStart := time.Now()
			lockID, err := l.Lock(ctx, "hot-key", lockTTL)
			elapsed := time.Since(lockStart)

			if err != nil {
				ctxTimeouts.Add(1)
				return
			}

			latMu.Lock()
			latencies = append(latencies, elapsed)
			latMu.Unlock()

			time.Sleep(workDuration)

			if uErr := l.Unlock("hot-key", lockID); uErr != nil {
				t.Errorf("unlock failed: %v", uErr)
				return
			}
			success.Add(1)
		}()
	}

	close(startGate)
	wg.Wait()

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	if len(latencies) == 0 {
		t.Fatalf("no successful lock acquisitions (ctxTimeouts=%d)", ctxTimeouts.Load())
	}

	p50 := latencies[len(latencies)*50/100]
	p95 := latencies[len(latencies)*95/100]
	p99 := latencies[len(latencies)*99/100]
	max := latencies[len(latencies)-1]

	t.Logf("hot-key contention (n=%d, work=%s):", numGoroutines, workDuration)
	t.Logf("  success=%d ctxTimeout=%d", success.Load(), ctxTimeouts.Load())
	t.Logf("  P50=%s P95=%s P99=%s MAX=%s", p50, p95, p99, max)

	// Egészséges threshold: 50 goroutine × 1ms = 50ms ideal,
	// 500ms már egyértelmű busy-wait jel (10× várás-szer).
	if p99 > 500*time.Millisecond {
		t.Errorf("HOT-KEY P99 too high: %s > 500ms — busy-wait gyanú", p99)
	}
	if ctxTimeouts.Load() > 0 {
		t.Errorf("ctx timeouts a 30s ablakon belül: %d — busy-wait CPU starvation jel", ctxTimeouts.Load())
	}
}

// TestLockGoroutineLeakAfterUnlock — egyszeri Lock+Unlock NEM növelhet
// trvös aktív goroutine-t. A jelenlegi impl auto-unlock goroutine-t indít
// minden Lock-nál, ami a TTL vagy ctx-Done-ig fut.
//
// Ez a teszt a fix előtt VÁRHATÓAN bukik, mert az auto-unlock goroutine
// életben marad ttl=5s-ig, miközben az Unlock nem szignalizálja, hogy
// már fel lett oldva.
func TestLockGoroutineLeakAfterUnlock(t *testing.T) {
	l := New()
	const numCycles = 100

	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	startGoroutines := runtime.NumGoroutine()

	for i := 0; i < numCycles; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		lockID, err := l.Lock(ctx, "leak-key", 5*time.Second)
		if err != nil {
			cancel()
			t.Fatalf("lock failed: %v", err)
		}
		if uErr := l.Unlock("leak-key", lockID); uErr != nil {
			cancel()
			t.Fatalf("unlock failed: %v", uErr)
		}
		cancel()
	}

	// Adjunk a scheduler-nek esélyt cleanup-ra
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	afterGoroutines := runtime.NumGoroutine()

	leak := afterGoroutines - startGoroutines
	t.Logf("goroutine count: start=%d after=%d leak=%d (cycles=%d)",
		startGoroutines, afterGoroutines, leak, numCycles)

	// Minden Lock+Unlock után max 1-2 goroutine-nyi átmeneti zaj megengedett.
	// 100 ciklus után 10+ leak az auto-unlock goroutine halmozódását mutatja.
	if leak > 10 {
		t.Errorf("goroutine leak: %d új goroutine 100 Lock+Unlock után — auto-unlock nem terminál Unlock-kor", leak)
	}
}

// TestLockBusyWaitDetection — méri, hogy egy csendes várakozó goroutine
// scheduler-t mennyire terheli. A jelenlegi for{select{default:continue}}
// loop 100% CPU-t fogyaszt egy magon, amíg a sora kerül.
//
// Mérési proxy: a fő goroutine közben próbál egy egyszerű "munka-loop"-ot
// futtatni. Ha a scheduler szabad, sok iteráció megy. Ha N waiter
// busy-wait-el, a scheduler ki van éhezve, és a "munka-loop" lassul.
func TestLockBusyWaitDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping scheduler stress in short mode")
	}

	// 1) Baseline: hány iterációt csinálunk 200ms alatt waiter-ek nélkül
	baseline := countIterations(200 * time.Millisecond)

	// 2) Ugyanaz, de N goroutine vár ugyanazon a key-en (busy-wait kandidált)
	l := New()
	holderCtx, holderCancel := context.WithCancel(context.Background())
	defer holderCancel()

	holderLockID, err := l.Lock(holderCtx, "cpu-key", 30*time.Second)
	if err != nil {
		t.Fatalf("holder lock failed: %v", err)
	}

	const numWaiters = 20
	var wg sync.WaitGroup
	for i := 0; i < numWaiters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			lockID, err := l.Lock(ctx, "cpu-key", 1100*time.Millisecond)
			if err == nil {
				_ = l.Unlock("cpu-key", lockID)
			}
		}()
	}

	// Adjunk a waiter-eknek időt elindulni
	time.Sleep(50 * time.Millisecond)

	// Most mérjük újra a "munka-loop"-ot, miközben a waiter-ek pörögnek
	stressed := countIterations(200 * time.Millisecond)

	_ = l.Unlock("cpu-key", holderLockID)
	wg.Wait()

	ratio := float64(baseline) / float64(stressed)
	t.Logf("scheduler stress: baseline=%d stressed=%d ratio=%.2fx (n_waiters=%d)",
		baseline, stressed, ratio, numWaiters)

	// Egészséges (channel-alapú) lock alatt a waiter-ek alszanak,
	// a fő goroutine ugyanannyi munkát végez (ratio ~1.0).
	// Busy-wait alatt a ratio jellemzően 2x-10x — a waiter-ek elveszik a CPU-t.
	if ratio > 2.0 {
		t.Errorf("scheduler-éhezés: %.2fx lassulás %d várakozóval — busy-wait gyanú", ratio, numWaiters)
	}
}

// countIterations ennyi iteráción megy át a megadott időablak alatt.
// Egyszerű CPU-érzékeny számláló, scheduler-éhezés méréséhez.
func countIterations(d time.Duration) int64 {
	deadline := time.Now().Add(d)
	var n int64
	for time.Now().Before(deadline) {
		n++
		// Apró munka, hogy a fordító ne optimalizálja ki
		if n%10000 == 0 {
			runtime.Gosched()
		}
	}
	return n
}
