package hydra

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/hydraide/hydraide/app/core/filesystem"
	"github.com/hydraide/hydraide/app/core/hydra/lock"
	"github.com/hydraide/hydraide/app/core/hydra/swamp"
	"github.com/hydraide/hydraide/app/core/safeops"
	"github.com/hydraide/hydraide/app/core/settings"
	"github.com/hydraide/hydraide/app/name"
)

// =============================================================================
// INSERT BENCHMARKS
// =============================================================================

// BenchmarkHydraInsert_V1 measures insert time with V1 (legacy) engine.
// Each operation (b.N) = 1 treasure insert.
//
// Run with: go test -run=^$ -bench=BenchmarkHydraInsert_V1 -benchmem -benchtime=100000x
func BenchmarkHydraInsert_V1(b *testing.B) {
	benchmarkInsert(b, false) // V1 engine
}

// BenchmarkHydraInsert_V2 measures insert time with V2 (append-only) engine.
// Each operation (b.N) = 1 treasure insert.
//
// Run with: go test -run=^$ -bench=BenchmarkHydraInsert_V2 -benchmem -benchtime=100000x
func BenchmarkHydraInsert_V2(b *testing.B) {
	benchmarkInsert(b, true) // V2 engine
}

func benchmarkInsert(b *testing.B, useV2 bool) {
	elysiumInterface := safeops.New()
	lockerInterface := lock.New()
	fsInterface := filesystem.New()

	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)

	// Enable V2 engine if requested
	if useV2 {
		_ = settingsInterface.SetEngine(settings.EngineV2)
	}

	fss := &settings.FileSystemSettings{
		WriteIntervalSec: 1,
		MaxFileSizeByte:  8192,
	}

	engineName := "v1"
	if useV2 {
		engineName = "v2"
	}

	settingsInterface.RegisterPattern(
		name.New().Sanctuary("bench").Realm(engineName).Swamp("*"),
		false, 1, fss,
	)

	hydraInterface := New(settingsInterface, elysiumInterface, lockerInterface, fsInterface)
	swampName := name.New().Sanctuary("bench").Realm(engineName).Swamp("insert")

	// Pre-summon the swamp
	si, _ := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
	si.BeginVigil()

	b.ResetTimer()

	// Each iteration = 1 treasure insert
	for i := 0; i < b.N; i++ {
		ti := si.CreateTreasure(fmt.Sprintf("treasure-%d", i))
		tg := ti.StartTreasureGuard(true)
		ti.SetContentString(tg, fmt.Sprintf("content-%d", i))
		ti.Save(tg)
		ti.ReleaseTreasureGuard(tg)
	}

	b.StopTimer()
	si.CeaseVigil()
	si.Destroy()
}

// =============================================================================
// IN-MEMORY INSERT BENCHMARKS (no disk I/O)
// =============================================================================

// BenchmarkHydraInsert_InMemory measures insert time with in-memory swamp (no disk).
// Each operation (b.N) = 1 treasure insert.
//
// Run with: go test -run=^$ -bench=BenchmarkHydraInsert_InMemory -benchmem -benchtime=1000000x
func BenchmarkHydraInsert_InMemory(b *testing.B) {
	elysiumInterface := safeops.New()
	lockerInterface := lock.New()
	fsInterface := filesystem.New()

	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)

	settingsInterface.RegisterPattern(
		name.New().Sanctuary("bench").Realm("inmemory").Swamp("insert"),
		true, // inMemory = true (no disk I/O)
		3600,
		nil,
	)

	hydraInterface := New(settingsInterface, elysiumInterface, lockerInterface, fsInterface)
	swampName := name.New().Sanctuary("bench").Realm("inmemory").Swamp("insert")

	si, _ := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
	si.BeginVigil()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ti := si.CreateTreasure(fmt.Sprintf("treasure-%d", i))
		tg := ti.StartTreasureGuard(true)
		ti.SetContentString(tg, fmt.Sprintf("content-%d", i))
		ti.Save(tg)
		ti.ReleaseTreasureGuard(tg)
	}

	b.StopTimer()
	si.CeaseVigil()
	si.Destroy()
}

// =============================================================================
// GET BENCHMARKS
// =============================================================================

// BenchmarkHydraGet measures get time from in-memory swamp.
// Each operation (b.N) = 1 treasure get.
//
// Run with: go test -run=^$ -bench=BenchmarkHydraGet -benchmem -benchtime=1000000x
func BenchmarkHydraGet(b *testing.B) {
	elysiumInterface := safeops.New()
	lockerInterface := lock.New()
	fsInterface := filesystem.New()

	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)

	settingsInterface.RegisterPattern(
		name.New().Sanctuary("bench").Realm("get").Swamp("readonly"),
		true,
		3600,
		nil,
	)

	hydraInterface := New(settingsInterface, elysiumInterface, lockerInterface, fsInterface)
	swampName := name.New().Sanctuary("bench").Realm("get").Swamp("readonly")

	// Setup: Insert 10K treasures first
	si, _ := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
	si.BeginVigil()

	numKeys := 10000
	keys := make([]string, numKeys)
	for i := 0; i < numKeys; i++ {
		keys[i] = fmt.Sprintf("key-%d", i)
		ti := si.CreateTreasure(keys[i])
		tg := ti.StartTreasureGuard(true)
		ti.SetContentString(tg, fmt.Sprintf("content-%d", i))
		ti.Save(tg)
		ti.ReleaseTreasureGuard(tg)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = si.GetTreasure(keys[i%numKeys])
	}

	b.StopTimer()
	si.CeaseVigil()
	si.Destroy()
}

// =============================================================================
// PARALLEL GET BENCHMARKS
// =============================================================================

// BenchmarkHydraGet_Parallel measures parallel get performance.
//
// Run with: go test -run=^$ -bench=BenchmarkHydraGet_Parallel -benchmem -cpu=1,2,4,8,16
func BenchmarkHydraGet_Parallel(b *testing.B) {
	elysiumInterface := safeops.New()
	lockerInterface := lock.New()
	fsInterface := filesystem.New()

	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)

	settingsInterface.RegisterPattern(
		name.New().Sanctuary("bench").Realm("parallel").Swamp("get"),
		true,
		3600,
		nil,
	)

	hydraInterface := New(settingsInterface, elysiumInterface, lockerInterface, fsInterface)
	swampName := name.New().Sanctuary("bench").Realm("parallel").Swamp("get")

	si, _ := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
	si.BeginVigil()

	numKeys := 10000
	keys := make([]string, numKeys)
	for i := 0; i < numKeys; i++ {
		keys[i] = fmt.Sprintf("key-%d", i)
		ti := si.CreateTreasure(keys[i])
		tg := ti.StartTreasureGuard(true)
		ti.SetContentString(tg, fmt.Sprintf("content-%d", i))
		ti.Save(tg)
		ti.ReleaseTreasureGuard(tg)
	}

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_, _ = si.GetTreasure(keys[i%numKeys])
			i++
		}
	})

	b.StopTimer()
	si.CeaseVigil()
	si.Destroy()
}

// BenchmarkHydraGet_MultiSwamp measures get performance across multiple swamps.
//
// Run with: go test -run=^$ -bench=BenchmarkHydraGet_MultiSwamp -benchmem -cpu=1,2,4,8,16
func BenchmarkHydraGet_MultiSwamp(b *testing.B) {
	elysiumInterface := safeops.New()
	lockerInterface := lock.New()
	fsInterface := filesystem.New()
	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)

	realm := "multiswamp"
	swampCount := 16

	swamps := make([]swamp.Swamp, swampCount)

	for i := 0; i < swampCount; i++ {
		sanctuary := name.New().Sanctuary("bench").Realm(realm).Swamp(fmt.Sprintf("swamp-%d", i))
		settingsInterface.RegisterPattern(
			sanctuary,
			true,
			3600,
			nil,
		)
	}

	hydraInterface := New(settingsInterface, elysiumInterface, lockerInterface, fsInterface)

	for i := 0; i < swampCount; i++ {
		swampName := name.New().Sanctuary("bench").Realm(realm).Swamp(fmt.Sprintf("swamp-%d", i))
		si, _ := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		si.BeginVigil()

		ti := si.CreateTreasure("treasure")
		tg := ti.StartTreasureGuard(true)
		ti.SetContentString(tg, "multiswamp-content")
		ti.Save(tg)
		ti.ReleaseTreasureGuard(tg)

		swamps[i] = si
	}

	var idx uint64
	var mu sync.Mutex

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		mu.Lock()
		myIdx := int(idx % uint64(swampCount))
		idx++
		mu.Unlock()

		si := swamps[myIdx]
		for pb.Next() {
			_, _ = si.GetTreasure("treasure")
		}
	})

	b.StopTimer()

	for _, si := range swamps {
		si.CeaseVigil()
		si.Destroy()
	}
}
