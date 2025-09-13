// Copyright (c) 2025 HydrAIDE
//
// WHY THIS TEST EXISTS
// --------------------
// Go 1.25 introduces an experimental "Greenteagc" garbage collector mode. We want
// a *repeatable, production-adjacent micro-benchmark* that stresses GC behavior
// in two clearly separated phases so we can compare:
//   1) Legacy (pre-Greenteagc) GC vs. 2) New Greenteagc GC in Go 1.25.
//
// What this test does
// -------------------
// • Phase A ("create burst"):
//     - Create **1,000,000** in-memory swamps under a dedicated sanctuary/realm.
//     - Each swamp receives one treasure with a small content write + save.
//     - Immediately after creation we snapshot GC counters + heap state.
// • Phase B ("idle-close window"):
//     - Swamps auto-close after **30 seconds** of inactivity (configured below).
//     - We sleep 35s so all handles become unreachable and eligible for GC.
//     - We snapshot GC + heap state again once the idle window elapses.
//
// What the logs show
// ------------------
// For each phase we log deltas between GC snapshots:
//   - wall:            Real (wall) time spent in the phase.
//   - cycles_Δ:        Number of GC cycles that completed in the phase.
//   - gc_cpu_total_Δ:  Total CPU time attributed to GC (seconds).
//   - gc_cpu_pause_Δ:  CPU time attributed to stop-the-world (STW) pauses.
//   - mark_*_Δ:        Breakdown of marking CPU by assist/dedicated/idle workers.
//   - scavenge_cpu_Δ:  CPU time spent scavenging (returning memory to the OS).
//   - heap_live_end:   Live heap size at the *end* of the phase.
//   - heap_goal_end:   Heap target at the *end* of the phase.
//   - heap_objects_end:Number of objects on the heap at the end of the phase.
//   - pause_p{50,95,99}_ms: Approx. GC pause distribution percentiles (ms).
//   - goroutines_end:  Number of goroutines at the end of the phase.
//
// How to use the results
// ----------------------
// Run the same binary twice—once with the legacy collector and once with the
// new Greenteagc GC enabled. Compare the two logs:
//   • Allocation throughput: Lower gc_cpu_total_Δ and fewer/shorter pauses
//     during "A) create burst" suggest the collector handles allocation pressure
//     better.
//   • Latency under idle reclamation: During "B) idle-close window", look for
//     reduced pause percentiles and reduced scavenge_cpu_Δ to assess whether
//     memory is returned with less CPU or fewer/faster STWs.
//   • Memory headroom and convergence: Compare heap_goal_end vs heap_live_end;
//     a closer goal→live convergence with stable latency is desirable.
//   • Marking distribution: Shifts between assist/dedicated/idle mark CPU can
//     reveal different scheduling trade-offs in the new collector.
//
// Caveats
// -------
// • This is a *synthetic* stress test. Always corroborate with production traces.
// • CPU count, GOMAXPROCS, machine noise, and allocator frag patterns affect
//   results; fix hardware and run multiple iterations.
// • The GC pause histogram is coarse (bucketed) and percentiles are approximate.
//
// --------------------------------------------------------------------------------

package hydra

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"runtime"
	"runtime/metrics"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/filesystem"
	"github.com/hydraide/hydraide/app/core/hydra/lock"
	"github.com/hydraide/hydraide/app/core/safeops"
	"github.com/hydraide/hydraide/app/core/settings"
	"github.com/hydraide/hydraide/app/name"
	"github.com/stretchr/testify/assert"
)

// ---------- GC METRICS HELPERS (local to this test) ----------

// gcSnap captures a point-in-time view of relevant runtime/metrics keys.
// We then compute deltas between two snapshots to attribute GC work to a phase.
type gcSnap struct {
	at               time.Time
	cyclesTotal      uint64
	gcCpuTotalSec    float64
	gcCpuPauseSec    float64
	markAssistSec    float64
	markDedicatedSec float64
	markIdleSec      float64
	scavengeTotalSec float64
	heapLiveBytes    uint64
	heapGoalBytes    uint64
	heapObjects      uint64
	pauseHist        *metrics.Float64Histogram
}

// The specific metrics we rely on. All of these are provided by runtime/metrics
// in Go 1.20+ and are stable enough for comparative testing.
var gcKeys = []string{
	"/gc/cycles/total:gc-cycles",              // total completed GC cycles
	"/cpu/classes/gc/total:cpu-seconds",       // total CPU time spent in GC
	"/cpu/classes/gc/pause:cpu-seconds",       // total CPU time in STW pauses
	"/cpu/classes/gc/mark/assist:cpu-seconds", // marking work done as assist
	"/cpu/classes/gc/mark/dedicated:cpu-seconds",
	"/cpu/classes/gc/mark/idle:cpu-seconds",
	"/cpu/classes/scavenge/total:cpu-seconds", // scavenge CPU (heap release)
	"/gc/heap/live:bytes",                     // live heap size
	"/gc/heap/goal:bytes",                     // heap target size
	"/gc/heap/objects:objects",                // number of heap objects
	"/sched/pauses/total/gc:seconds",          // histogram of GC STW pauses
}

// takeSnap reads all keys in one go to form a coherent snapshot.
func takeSnap() gcSnap {
	samples := make([]metrics.Sample, len(gcKeys))
	for i, k := range gcKeys {
		samples[i].Name = k
	}
	metrics.Read(samples)

	get := func(n string) metrics.Value {
		for _, s := range samples {
			if s.Name == n {
				return s.Value
			}
		}
		return metrics.Value{}
	}

	return gcSnap{
		at:               time.Now(),
		cyclesTotal:      get("/gc/cycles/total:gc-cycles").Uint64(),
		gcCpuTotalSec:    get("/cpu/classes/gc/total:cpu-seconds").Float64(),
		gcCpuPauseSec:    get("/cpu/classes/gc/pause:cpu-seconds").Float64(),
		markAssistSec:    get("/cpu/classes/gc/mark/assist:cpu-seconds").Float64(),
		markDedicatedSec: get("/cpu/classes/gc/mark/dedicated:cpu-seconds").Float64(),
		markIdleSec:      get("/cpu/classes/gc/mark/idle:cpu-seconds").Float64(),
		scavengeTotalSec: get("/cpu/classes/scavenge/total:cpu-seconds").Float64(),
		heapLiveBytes:    get("/gc/heap/live:bytes").Uint64(),
		heapGoalBytes:    get("/gc/heap/goal:bytes").Uint64(),
		heapObjects:      get("/gc/heap/objects:objects").Uint64(),
		pauseHist:        get("/sched/pauses/total/gc:seconds").Float64Histogram(),
	}
}

// diff computes (b - a) deltas for GC/CPU counters, and returns b.at - a.at
// as the phase wall time. This directly attributes GC work to a labeled phase.
func diff(a, b gcSnap) (dt time.Duration,
	cycles uint64, gcTotal, gcPause, assist, dedicated, idle, scavenge float64) {

	dt = b.at.Sub(a.at)
	cycles = b.cyclesTotal - a.cyclesTotal
	gcTotal = b.gcCpuTotalSec - a.gcCpuTotalSec
	gcPause = b.gcCpuPauseSec - a.gcCpuPauseSec
	assist = b.markAssistSec - a.markAssistSec
	dedicated = b.markDedicatedSec - a.markDedicatedSec
	idle = b.markIdleSec - a.markIdleSec
	scavenge = b.scavengeTotalSec - a.scavengeTotalSec
	return
}

// pct returns an *approximate* percentile from the GC pause histogram
// (in seconds). The histogram is bucketed; we return the lower bound of the
// bucket where the running count crosses the target percentile.
//
// Notes:
//   - Percentiles are intentionally coarse but stable across runs.
//   - We convert to milliseconds at the logging call site for readability.
func pct(h *metrics.Float64Histogram, p float64) float64 {
	if h == nil || len(h.Buckets) == 0 {
		return 0
	}
	total := uint64(0)
	for _, c := range h.Counts {
		total += c
	}
	if total == 0 {
		return 0
	}
	target := uint64(math.Ceil(float64(total) * p))
	if target == 0 {
		target = 1
	}
	run := uint64(0)
	for i := 0; i < len(h.Counts); i++ {
		run += h.Counts[i]
		if run >= target {
			// Return the lower bound of the bucket in which the percentile falls.
			if i == 0 {
				return math.SmallestNonzeroFloat64
			}
			if i-1 < len(h.Buckets) {
				return h.Buckets[i-1]
			}
			return h.Buckets[len(h.Buckets)-1]
		}
	}
	return h.Buckets[len(h.Buckets)-1]
}

// b2s formats bytes with binary units for human-readable logs.
func b2s(b uint64) string {
	const (
		KB = 1 << 10
		MB = 1 << 20
		GB = 1 << 30
		TB = 1 << 40
	)
	switch {
	case b >= TB:
		return fmt.Sprintf("%.2fTB", float64(b)/TB)
	case b >= GB:
		return fmt.Sprintf("%.2fGB", float64(b)/GB)
	case b >= MB:
		return fmt.Sprintf("%.2fMB", float64(b)/MB)
	case b >= KB:
		return fmt.Sprintf("%.2fKB", float64(b)/KB)
	default:
		return fmt.Sprintf("%dB", b)
	}
}

// r2 rounds to 2 decimals for stable, compact logging.
func r2(f float64) float64 { return math.Round(f*100) / 100 }

// ---------- CONCRETE GC STRESS TEST ----------

const (
	// Dedicated namespacing for this test so it doesn't interfere with other runs.
	sanctuaryForGcTest = "GCSanctuary"
	realmForGCTest     = "gc"
)

// TestHydra_Go_NewGC creates one million swamps, then relies on the configured
// 30s idle timeout to make them unreachable so the collector can reclaim them.
// We log GC & heap deltas for the create burst and the subsequent idle window.
//
// Run strategy:
//   - Run once with the legacy collector.
//   - Run again with the new Greenteagc GC enabled.
//   - Compare the two logs for throughput, latency, and memory behavior.
func TestHydra_Go_NewGC(t *testing.T) {
	// (Optional) Pin CPU cores for reproducibility across runs.
	// Beware: changing this affects GC pacing and scheduler decisions.
	// runtime.GOMAXPROCS(8)

	// Minimal wiring to spin up a Hydra instance.
	elysium := safeops.New()
	locker := lock.New()
	fs := filesystem.New()
	cfg := settings.New(1, 10000)

	// Configure auto-close: swamps idle for 30s are closed, removing live refs.
	// This makes the objects eligible for GC during the subsequent idle window.
	cfg.RegisterPattern(name.New().Sanctuary(sanctuaryForGcTest).Realm(realmForGCTest).Swamp("*"), true, 30, nil)

	// NOTE: Adjust constructor import if your project layout differs.
	h := New(cfg, elysium, locker, fs)

	// ---- Baseline GC snapshot before any work. This is our Phase A anchor.
	start := takeSnap()

	fmt.Println("Starting GC test...")

	// ---- Phase A (create burst): Create 1,000,000 unique swamps.
	// Each has a single treasure with a small content payload to drive allocation.
	N := 1_000_000
	for i := 0; i < N; i++ {
		swampName := name.New().Sanctuary(sanctuaryForGcTest).Realm(realmForGCTest).Swamp(fmt.Sprintf("swamp%d", i))

		// Summon a swamp with 1 treasure slot.
		si, err := h.SummonSwamp(context.Background(), 1, swampName)
		if err != nil {
			t.Fatal(err)
		}

		// Create a treasure, start a guard, write content, save, and release.
		// This touches allocator hot paths and typical write workflows.
		ti := si.CreateTreasure(fmt.Sprintf("treasure-%d", i))
		tg := ti.StartTreasureGuard(true)
		ti.SetContentString(tg, fmt.Sprintf("content-%d", i))
		ti.Save(tg)
		ti.ReleaseTreasureGuard(tg)
	}

	fmt.Println("Created", N, "swamps.")

	// Sanity guard: we expect all swamps to be active at this point.
	logged := h.CountActiveSwamps()
	assert.Equal(t, N, logged, "should be equal")

	// Snapshot immediately after creation. This bounds the Phase A metrics.
	afterCreate := takeSnap()

	// ---- Phase B (idle-close window): wait beyond the 30s idle timeout.
	// We buffer by 5s to ensure all swamps have crossed the boundary and any
	// guards/refs are truly gone, allowing GC to reclaim memory.
	time.Sleep(35 * time.Second)

	// Snapshot after idle window. This bounds Phase B metrics.
	afterClose := takeSnap()

	// By now, all swamps should have auto-closed and been dereferenced.
	logged = h.CountActiveSwamps()
	assert.Equal(t, 0, logged, "should be equal")

	// ---- Phase logger: compute deltas and emit compact, comparable metrics.
	logPhase := func(label string, a, b gcSnap) {
		dt, cycles, gcTotal, gcPause, assist, dedicated, idle, scav := diff(a, b)

		// Convert pause percentiles to milliseconds for easier reading.
		p50ms := pct(b.pauseHist, 0.50) * 1000.0
		p95ms := pct(b.pauseHist, 0.95) * 1000.0
		p99ms := pct(b.pauseHist, 0.99) * 1000.0

		slog.Info("gc-measure",
			"phase", label,
			"wall", dt.String(),
			"cycles_Δ", cycles,
			"gc_cpu_total_Δ_sec", r2(gcTotal),
			"gc_cpu_pause_Δ_sec", r2(gcPause),
			"mark_assist_Δ_sec", r2(assist),
			"mark_dedicated_Δ_sec", r2(dedicated),
			"mark_idle_Δ_sec", r2(idle),
			"scavenge_cpu_Δ_sec", r2(scav),
			"heap_live_end", b2s(b.heapLiveBytes),
			"heap_goal_end", b2s(b.heapGoalBytes),
			"heap_objects_end", b.heapObjects,
			"pause_p50_ms", r2(p50ms),
			"pause_p95_ms", r2(p95ms),
			"pause_p99_ms", r2(p99ms),
			"goroutines_end", runtime.NumGoroutine(),
		)
	}

	// Phase A: allocation + write workload under pressure.
	logPhase("A) create burst", start, afterCreate)

	// Phase B: reclamation/scavenge after 30s idle close.
	logPhase("B) idle-close window", afterCreate, afterClose)

	// Interpretation guide (not logged, for readers of the test code):
	//   - If Greenteagc GC reduces gc_cpu_total_Δ_sec and pause_pXX_ms in Phase A
	//     without increasing heap_live_end drastically, it likely scales better
	//     under allocation pressure.
	//   - If Greenteagc GC shows lower gc_cpu_pause_Δ_sec and stable/shorter
	//     pause_pXX_ms in Phase B, reclamation is less disruptive.
	//   - A smaller gap between heap_goal_end and heap_live_end suggests better
	//     pacing/targeting of the heap, but always weigh this against latency.
}
