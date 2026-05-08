package swamp

import (
	"fmt"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/msgpackpatch"
	"github.com/vmihailenco/msgpack/v5"
)

// In-process benchmarks for PatchExpired. These exercise the engine
// (beacon selection + per-key guard + Save + reindex) without the gRPC
// layer, isolating the engine cost from network and serialization.
//
// Run with:
//
//	go test -bench=BenchmarkPatchExpired -benchmem -run=^$ \
//	    ./app/core/hydra/swamp/...
//
// The matrix mirrors docs/tasks/patch-expired-many/PLAN.md "Benchmarks".
// Each benchmark reseeds when the expired set drains so steady-state
// allocations stay representative.

// benchSeedExpiredSwamp creates a fresh test swamp and seeds it with
// `total` msgpack treasures. The first `expired` entries get a past
// ExpiredAt; the rest get a future one.
func benchSeedExpiredSwamp(b *testing.B, suffix string, total, expired int) Swamp {
	b.Helper()
	s := patchTestSwampTB(b, "patch-bench", suffix)
	now := time.Now().UTC()
	for i := 0; i < total; i++ {
		key := fmt.Sprintf("k-%07d", i)
		body, err := msgpack.Marshal(map[string]any{
			"Counter":   int32(0),
			"ClaimedBy": "",
		})
		if err != nil {
			b.Fatalf("msgpack: %v", err)
		}
		tr := s.CreateTreasure(key)
		gid := tr.StartTreasureGuard(true)
		tr.SetContentByteArray(gid, append([]byte{0xC7, 0x00}, body...))
		var exp time.Time
		if i < expired {
			exp = now.Add(-time.Hour).Add(time.Duration(i) * time.Microsecond)
		} else {
			exp = now.Add(time.Hour).Add(time.Duration(i) * time.Microsecond)
		}
		tr.SetExpirationTime(gid, exp)
		tr.Save(gid)
		tr.ReleaseTreasureGuard(gid)
	}
	return s
}

func benchEncode(b *testing.B, v any) []byte {
	b.Helper()
	out, err := msgpack.Marshal(v)
	if err != nil {
		b.Fatalf("msgpack: %v", err)
	}
	return out
}

// =============================================================================
// Benchmarks
// =============================================================================

// BenchmarkPatchExpired_Throughput sweeps batch size at fixed swamp size
// and a high expired fraction.
func BenchmarkPatchExpired_Throughput(b *testing.B) {
	for _, batch := range []int32{50, 500, 5000} {
		b.Run(fmt.Sprintf("batch=%d", batch), func(b *testing.B) {
			const total = 100_000
			const expired = 50_000
			s := benchSeedExpiredSwamp(b, fmt.Sprintf("throughput-%d", batch), total, expired)

			ops := []msgpackpatch.Op{
				{Kind: msgpackpatch.OpSet, Path: "ClaimedBy", Value: benchEncode(b, "worker")},
			}
			meta := &PatchFieldsMeta{SetExpiredAt: time.Now().UTC().Add(time.Hour)}

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				entries, err := s.PatchExpired(batch, ops, nil, meta)
				if err != nil {
					b.Fatalf("PatchExpired: %v", err)
				}
				if len(entries) == 0 {
					b.StopTimer()
					s = benchSeedExpiredSwamp(b, fmt.Sprintf("throughput-%d-r-%d", batch, i), total, expired)
					b.StartTimer()
				}
			}
		})
	}
}

// BenchmarkPatchExpired_Scale fixes batch and expired fraction, varies
// swamp size. Surfaces the O(K log K) sort cost on the ascending
// expiration index.
func BenchmarkPatchExpired_Scale(b *testing.B) {
	for _, total := range []int{10_000, 100_000, 500_000} {
		expired := total / 10 // 10%
		b.Run(fmt.Sprintf("swamp=%d", total), func(b *testing.B) {
			s := benchSeedExpiredSwamp(b, fmt.Sprintf("scale-%d", total), total, expired)
			ops := []msgpackpatch.Op{
				{Kind: msgpackpatch.OpSet, Path: "ClaimedBy", Value: benchEncode(b, "worker")},
			}
			meta := &PatchFieldsMeta{SetExpiredAt: time.Now().UTC().Add(time.Hour)}

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				entries, err := s.PatchExpired(500, ops, nil, meta)
				if err != nil {
					b.Fatalf("PatchExpired: %v", err)
				}
				if len(entries) == 0 {
					b.StopTimer()
					s = benchSeedExpiredSwamp(b, fmt.Sprintf("scale-%d-r-%d", total, i), total, expired)
					b.StartTimer()
				}
			}
		})
	}
}

// BenchmarkPatchExpired_OpsPerPatch isolates the per-op cost.
func BenchmarkPatchExpired_OpsPerPatch(b *testing.B) {
	for _, opsCount := range []int{0, 1, 5} {
		b.Run(fmt.Sprintf("ops=%d", opsCount), func(b *testing.B) {
			const total = 100_000
			const expired = 10_000
			s := benchSeedExpiredSwamp(b, fmt.Sprintf("ops-%d", opsCount), total, expired)
			ops := make([]msgpackpatch.Op, 0, opsCount)
			for i := 0; i < opsCount; i++ {
				ops = append(ops, msgpackpatch.Op{
					Kind:  msgpackpatch.OpSet,
					Path:  fmt.Sprintf("Field%d", i),
					Value: benchEncode(b, int32(i)),
				})
			}
			meta := &PatchFieldsMeta{SetExpiredAt: time.Now().UTC().Add(time.Hour)}

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				entries, err := s.PatchExpired(500, ops, nil, meta)
				if err != nil {
					b.Fatalf("PatchExpired: %v", err)
				}
				if len(entries) == 0 {
					b.StopTimer()
					s = benchSeedExpiredSwamp(b, fmt.Sprintf("ops-%d-r-%d", opsCount, i), total, expired)
					b.StartTimer()
				}
			}
		})
	}
}
