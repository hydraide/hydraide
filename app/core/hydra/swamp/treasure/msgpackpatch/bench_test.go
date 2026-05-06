package msgpackpatch

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vmihailenco/msgpack/v5"
)

// Benchmark suite for the structural-splice patch primitive. Sizes are
// chosen to bracket realistic Treasure payloads: 1KB is a typical small
// Catalog row, 10KB is a medium payload with several nested fields, 100KB
// is a stress upper bound (large embedded log array etc.).
//
// Each benchmark also runs the naive decode-to-map baseline so the
// speed-up of the splice is visible side-by-side. The expected pattern
// is that the splice scales near-linearly in input size while staying
// significantly faster than the naive approach because it avoids
// materializing the entire value tree.

// buildPayload returns a msgpack blob whose top-level map approximates
// the requested size in bytes. The map mixes a few small typed fields
// (int8 / int32 / float64 / string) with a large filler string to hit
// the target.
func buildPayload(targetBytes int) []byte {
	// Account for the overhead of the small fields (~64 bytes).
	fillerSize := targetBytes - 64
	if fillerSize < 0 {
		fillerSize = 0
	}
	payload := map[string]any{
		"i8":  int8(7),
		"i32": int32(70000),
		"f64": float64(3.14159),
		"s":   "hi",
		"data": strings.Repeat("x", fillerSize),
	}
	b, err := msgpack.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return b
}

// runSizedBenchmark runs benchSplice + benchNaiveBaseline at a given
// payload size, reporting bytes/op via b.SetBytes so go test's MB/s
// numbers reflect throughput.
func runSizedBenchmark(b *testing.B, sizeLabel string, sizeBytes int) {
	b.Run("splice", func(b *testing.B) {
		blob := buildPayload(sizeBytes)
		newVal, _ := msgpack.Marshal("HI")
		b.SetBytes(int64(len(blob)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := Apply(blob, []Op{
				{Kind: OpSet, Path: "s", Value: newVal},
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("naive_decode_modify_encode", func(b *testing.B) {
		blob := buildPayload(sizeBytes)
		b.SetBytes(int64(len(blob)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var m map[string]any
			if err := msgpack.Unmarshal(blob, &m); err != nil {
				b.Fatal(err)
			}
			m["s"] = "HI"
			if _, err := msgpack.Marshal(m); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkPatch_SingleField_1KB(b *testing.B) {
	runSizedBenchmark(b, "1KB", 1024)
}

func BenchmarkPatch_SingleField_10KB(b *testing.B) {
	runSizedBenchmark(b, "10KB", 10*1024)
}

func BenchmarkPatch_SingleField_100KB(b *testing.B) {
	runSizedBenchmark(b, "100KB", 100*1024)
}

// Multi-field patch: 10 SET ops on a 1KB blob, all in a single Apply call.
// This is the primary expected production pattern (CatalogPatchFields
// with several flag updates per call).
func BenchmarkPatch_TenFields_1KB(b *testing.B) {
	blob, ops := buildTenFieldPayload(1024)
	b.SetBytes(int64(len(blob)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Apply(blob, ops)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSkeletonParse_1KB(b *testing.B) {
	blob := buildPayload(1024)
	b.SetBytes(int64(len(blob)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Parse(blob); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSkeletonParse_10KB(b *testing.B) {
	blob := buildPayload(10 * 1024)
	b.SetBytes(int64(len(blob)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Parse(blob); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSkeletonParse_100KB(b *testing.B) {
	blob := buildPayload(100 * 1024)
	b.SetBytes(int64(len(blob)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Parse(blob); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPatch_ConditionalUpdate measures the realistic AllDomains-flag
// scenario: a payload with several boolean flags, plus an INC + a SET
// under a Condition. This is the closest synthetic workload to the
// Trendizz cutover hot path.
func BenchmarkPatch_ConditionalUpdate(b *testing.B) {
	payload := map[string]any{
		"IsCrawling":     false,
		"IsRejected":     false,
		"IsInQueue":      true,
		"RejectedReason": int16(0),
		"Counter":        int32(0),
		"Owner":          "alice",
	}
	blob, err := msgpack.Marshal(payload)
	if err != nil {
		b.Fatal(err)
	}

	trueRaw, _ := msgpack.Marshal(true)
	delta, _ := msgpack.Marshal(int32(1))
	threshold, _ := msgpack.Marshal("alice")

	cond := &Condition{
		Path:      "Owner",
		Op:        CondEqual,
		Threshold: threshold,
	}

	b.SetBytes(int64(len(blob)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ApplyWithCondition(blob, []Op{
			{Kind: OpSet, Path: "IsCrawling", Value: trueRaw},
			{Kind: OpInc, Path: "Counter", Value: delta},
		}, cond)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// buildTenFieldPayload returns a blob with ten f0..f9 string fields
// plus filler to hit the target size, and ten SET ops mutating each
// field to a new string value.
func buildTenFieldPayload(targetBytes int) ([]byte, []Op) {
	const fields = 10
	payload := make(map[string]any, fields+1)
	for i := 0; i < fields; i++ {
		payload[fmt.Sprintf("f%d", i)] = fmt.Sprintf("v%d", i)
	}
	// Add filler to reach the target payload size.
	overhead := 200
	fillerSize := targetBytes - overhead
	if fillerSize > 0 {
		payload["data"] = strings.Repeat("x", fillerSize)
	}
	blob, err := msgpack.Marshal(payload)
	if err != nil {
		panic(err)
	}
	ops := make([]Op, 0, fields)
	for i := 0; i < fields; i++ {
		val, _ := msgpack.Marshal(fmt.Sprintf("V%d", i))
		ops = append(ops, Op{Kind: OpSet, Path: fmt.Sprintf("f%d", i), Value: val})
	}
	return blob, ops
}
