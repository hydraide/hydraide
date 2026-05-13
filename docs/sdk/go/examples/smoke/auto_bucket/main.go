// smoke/auto_bucket — live HydrAIDE smoke + benchmark harness for the
// auto-built field-bucket index feature.
//
// Run from this directory:
//
//	go run .
//
// Connects to the dev compose instance (localhost:5980 via mTLS) and
// exercises every observable surface that the bucket-index touches:
//
//   - matrix correctness: M2 / M5 / M6 / M7 / M8 / M9 / M14 / M19 / M22
//   - cold-vs-warm latency on swamp sizes 1K / 10K / 50K
//   - Trendizz-shaped 50-ASN cycle wall-clock
//   - lifecycle: re-summon after CloseAfterIdle rebuilds the bucket
//   - concurrency: parallel cold-builds on different fields
//   - bench matrix: cold vs warm across (size × ASN cardinality)
//
// Exit status: 0 on full PASS, 1 on any FAIL.
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/internal/setup"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/utils/repo"
)

type Item struct {
	Key  string `hydraide:"key"`
	Body *Body  `hydraide:"value"`
}

type Body struct {
	Asn      int64  `msgpack:"asn"`
	Status   string `msgpack:"status"`
	Category string `msgpack:"category"`
	Score    int64  `msgpack:"score"`
}

func swampName(realm, swamp string) name.Name {
	return name.New().Sanctuary("smoke-bucket").Realm(realm).Swamp(swamp)
}

func swampPattern(realm string) name.Name {
	return name.New().Sanctuary("smoke-bucket").Realm(realm).Swamp("*")
}

type Result struct {
	Name    string
	Pass    bool
	Note    string
	Latency time.Duration
}

var results []Result

func record(name string, pass bool, latency time.Duration, note string) {
	results = append(results, Result{Name: name, Pass: pass, Note: note, Latency: latency})
	tag := "PASS"
	if !pass {
		tag = "FAIL"
	}
	if latency > 0 {
		fmt.Printf("  [%s] %-50s  %12v  %s\n", tag, name, latency.Round(time.Microsecond), note)
	} else {
		fmt.Printf("  [%s] %-50s  %s\n", tag, name, note)
	}
}

func seed(ctx context.Context, h hydraidego.Hydraidego, swamp name.Name, n, asnCard int) error {
	r := rand.New(rand.NewSource(42))
	statuses := []string{"ready", "pending", "running", "done", "failed"}
	categories := []string{"A", "B", "C", "D", "E"}

	batchSize := 500
	for batchStart := 0; batchStart < n; batchStart += batchSize {
		end := batchStart + batchSize
		if end > n {
			end = n
		}
		models := make([]any, 0, end-batchStart)
		for i := batchStart; i < end; i++ {
			models = append(models, &Item{
				Key: fmt.Sprintf("k%07d", i),
				Body: &Body{
					Asn:      int64(i % asnCard),
					Status:   statuses[i%len(statuses)],
					Category: categories[i%len(categories)],
					Score:    int64(r.Intn(1000)),
				},
			})
		}
		if err := h.CatalogSaveMany(ctx, swamp, models, nil); err != nil {
			return fmt.Errorf("save batch [%d,%d): %w", batchStart, end, err)
		}
	}
	return nil
}

func matrixCorrectness(ctx context.Context, h hydraidego.Hydraidego) {
	fmt.Println("=== Matrix correctness (M2-M22, 500 records) ===")
	sw := swampName("matrix", "main")
	_ = h.Destroy(ctx, sw)
	defer func() { _ = h.Destroy(ctx, sw) }()

	n := 500
	asnCard := 10
	statuses := []string{"ready", "pending", "running", "done", "failed"}
	categories := []string{"A", "B", "C", "D", "E"}
	items := make([]any, 0, n)
	byAsn := map[int64]int{}
	byStatus := map[string]int{}
	for i := 0; i < n; i++ {
		body := &Body{
			Asn:      int64(i % asnCard),
			Status:   statuses[i%len(statuses)],
			Category: categories[i%len(categories)],
			Score:    int64(i),
		}
		items = append(items, &Item{Key: fmt.Sprintf("k%04d", i), Body: body})
		byAsn[body.Asn]++
		byStatus[body.Status]++
	}
	if err := h.CatalogSaveMany(ctx, sw, items, nil); err != nil {
		log.Fatalf("seed: %v", err)
	}

	idx := &hydraidego.Index{IndexType: hydraidego.IndexKey, IndexOrder: hydraidego.IndexOrderAsc}

	run := func(label string, filter *hydraidego.FilterGroup, expected int) {
		t0 := time.Now()
		got := 0
		err := h.CatalogReadManyStream(ctx, sw, idx, filter, Item{}, func(_ any) error {
			got++
			return nil
		})
		latency := time.Since(t0)
		if err != nil {
			record(label, false, latency, fmt.Sprintf("error: %v", err))
			return
		}
		if got != expected {
			record(label, false, latency, fmt.Sprintf("got=%d expected=%d", got, expected))
			return
		}
		record(label, true, latency, fmt.Sprintf("rows=%d", got))
	}

	run("M2 asn=5",
		hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "asn", 5)),
		byAsn[5])

	exp5ready := 0
	for _, it := range items {
		x := it.(*Item)
		if x.Body.Asn == 5 && x.Body.Status == "ready" {
			exp5ready++
		}
	}
	run("M5 asn=5 AND status=ready",
		hydraidego.FilterAND(
			hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "asn", 5),
			hydraidego.FilterBytesFieldString(hydraidego.Equal, "status", "ready"),
		),
		exp5ready)

	run("M6 asn=5 OR asn=6",
		hydraidego.FilterOR(
			hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "asn", 5),
			hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "asn", 6),
		),
		byAsn[5]+byAsn[6])

	exp7 := 0
	for _, it := range items {
		x := it.(*Item)
		if x.Body.Asn == 5 || x.Body.Status == "ready" {
			exp7++
		}
	}
	run("M7 asn=5 OR status=ready",
		hydraidego.FilterOR(
			hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "asn", 5),
			hydraidego.FilterBytesFieldString(hydraidego.Equal, "status", "ready"),
		),
		exp7)

	run("M8 asn IN (1,2,3)",
		hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64In("asn", 1, 2, 3)),
		byAsn[1]+byAsn[2]+byAsn[3])

	exp9 := 0
	for _, it := range items {
		x := it.(*Item)
		if x.Body.Asn == 5 && x.Body.Score > 100 {
			exp9++
		}
	}
	run("M9 asn=5 AND score>100 (range residual)",
		hydraidego.FilterAND(
			hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "asn", 5),
			hydraidego.FilterBytesFieldInt64(hydraidego.GreaterThan, "score", 100),
		),
		exp9)

	exp14 := 0
	for _, it := range items {
		x := it.(*Item)
		if x.Body.Score > 100 && x.Body.Score < 200 {
			exp14++
		}
	}
	run("M14 score>100 AND score<200 (bypass)",
		hydraidego.FilterAND(
			hydraidego.FilterBytesFieldInt64(hydraidego.GreaterThan, "score", 100),
			hydraidego.FilterBytesFieldInt64(hydraidego.LessThan, "score", 200),
		),
		exp14)

	exp22 := 0
	for _, it := range items {
		x := it.(*Item)
		if x.Body.Asn == 5 && x.Body.Status != "ready" {
			exp22++
		}
	}
	run("M22 asn=5 AND status!=ready",
		hydraidego.FilterAND(
			hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "asn", 5),
			hydraidego.FilterBytesFieldString(hydraidego.NotEqual, "status", "ready"),
		),
		exp22)

	run("M19 empty filter (full sweep)", nil, n)
}

func coldVsWarm(ctx context.Context, h hydraidego.Hydraidego, sizeName string, n int) {
	fmt.Printf("=== Cold vs warm latency (size=%s, n=%d) ===\n", sizeName, n)
	sw := swampName("cold-warm", sizeName)
	_ = h.Destroy(ctx, sw)
	defer func() { _ = h.Destroy(ctx, sw) }()

	if err := seed(ctx, h, sw, n, 100); err != nil {
		log.Fatalf("seed: %v", err)
	}

	idx := &hydraidego.Index{IndexType: hydraidego.IndexKey, IndexOrder: hydraidego.IndexOrderAsc}
	filter := hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "asn", 42))

	measure := func() (time.Duration, int) {
		t0 := time.Now()
		got := 0
		_ = h.CatalogReadManyStream(ctx, sw, idx, filter, Item{}, func(_ any) error {
			got++
			return nil
		})
		return time.Since(t0), got
	}

	cold, gotCold := measure()
	warm1, gotWarm := measure()
	warm2, _ := measure()
	warm3, _ := measure()

	if gotCold != gotWarm {
		record(fmt.Sprintf("cold-warm-correctness/%s", sizeName), false, 0,
			fmt.Sprintf("cold=%d warm=%d (mismatch)", gotCold, gotWarm))
	} else {
		record(fmt.Sprintf("cold-warm-correctness/%s", sizeName), true, 0,
			fmt.Sprintf("rows=%d", gotCold))
	}
	speedup := float64(cold) / float64(warm1)
	record(fmt.Sprintf("cold-warm-perf/%s", sizeName), true, warm1,
		fmt.Sprintf("cold=%v warm1=%v warm2=%v warm3=%v speedup=%.1fx",
			cold.Round(time.Microsecond),
			warm1.Round(time.Microsecond),
			warm2.Round(time.Microsecond),
			warm3.Round(time.Microsecond),
			speedup))
}

func cycle50ASN(ctx context.Context, h hydraidego.Hydraidego, n int) {
	fmt.Printf("=== Trendizz-shaped 50-ASN cycle (n=%d, asnCard=100) ===\n", n)
	sw := swampName("cycle50", "main")
	_ = h.Destroy(ctx, sw)
	defer func() { _ = h.Destroy(ctx, sw) }()

	if err := seed(ctx, h, sw, n, 100); err != nil {
		log.Fatalf("seed: %v", err)
	}
	idx := &hydraidego.Index{IndexType: hydraidego.IndexKey, IndexOrder: hydraidego.IndexOrderAsc}

	warmup := hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "asn", 0))
	_ = h.CatalogReadManyStream(ctx, sw, idx, warmup, Item{}, func(_ any) error { return nil })

	t0 := time.Now()
	total := 0
	for asn := int64(0); asn < 50; asn++ {
		filter := hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "asn", asn))
		_ = h.CatalogReadManyStream(ctx, sw, idx, filter, Item{}, func(_ any) error {
			total++
			return nil
		})
	}
	elapsed := time.Since(t0)
	record(fmt.Sprintf("50-asn-cycle/n=%d", n), true, elapsed,
		fmt.Sprintf("total=%d, per-call=%v", total, (elapsed/50).Round(time.Microsecond)))
}

func lifecycleRebuild(ctx context.Context, r repo.Repo, h hydraidego.Hydraidego) {
	fmt.Println("=== Lifecycle: re-summon rebuilds the bucket ===")
	pattern := name.New().Sanctuary("smoke-bucket").Realm("lifecycle").Swamp("*")
	if errs := r.GetHydraidego().RegisterSwamp(ctx, &hydraidego.RegisterSwampRequest{
		SwampPattern:    pattern,
		CloseAfterIdle:  2 * time.Second,
		IsInMemorySwamp: false,
		FilesystemSettings: &hydraidego.SwampFilesystemSettings{
			WriteInterval:  time.Second,
			MaxFileSize:    8192,
			EncodingFormat: hydraidego.EncodingMsgPack,
		},
	}); len(errs) > 0 {
		log.Fatalf("register lifecycle pattern: %v", errs[0])
	}

	sw := swampName("lifecycle", "main")
	_ = h.Destroy(ctx, sw)
	defer func() { _ = h.Destroy(ctx, sw) }()

	if err := seed(ctx, h, sw, 5000, 50); err != nil {
		log.Fatalf("seed: %v", err)
	}
	idx := &hydraidego.Index{IndexType: hydraidego.IndexKey, IndexOrder: hydraidego.IndexOrderAsc}
	filter := hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "asn", 7))

	t0 := time.Now()
	c1 := 0
	_ = h.CatalogReadManyStream(ctx, sw, idx, filter, Item{}, func(_ any) error { c1++; return nil })
	build1 := time.Since(t0)

	time.Sleep(4 * time.Second)

	t0 = time.Now()
	c2 := 0
	_ = h.CatalogReadManyStream(ctx, sw, idx, filter, Item{}, func(_ any) error { c2++; return nil })
	build2 := time.Since(t0)

	pass := c1 == c2 && c1 > 0
	record("lifecycle-rebuild", pass, build2,
		fmt.Sprintf("call1=%v rows=%d, call2=%v rows=%d (rebuild expected)",
			build1.Round(time.Microsecond), c1, build2.Round(time.Microsecond), c2))
}

func concurrentColdBuilds(ctx context.Context, h hydraidego.Hydraidego) {
	fmt.Println("=== Concurrent cold builds on different fields ===")
	sw := swampName("concurrent", "main")
	_ = h.Destroy(ctx, sw)
	defer func() { _ = h.Destroy(ctx, sw) }()

	if err := seed(ctx, h, sw, 5000, 50); err != nil {
		log.Fatalf("seed: %v", err)
	}
	idx := &hydraidego.Index{IndexType: hydraidego.IndexKey, IndexOrder: hydraidego.IndexOrderAsc}

	type query struct {
		label  string
		filter *hydraidego.FilterGroup
		want   int
	}
	queries := []query{
		{"asn=10", hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "asn", 10)), 5000 / 50},
		{"status=ready", hydraidego.FilterAND(hydraidego.FilterBytesFieldString(hydraidego.Equal, "status", "ready")), -1},
		{"category=A", hydraidego.FilterAND(hydraidego.FilterBytesFieldString(hydraidego.Equal, "category", "A")), -1},
	}

	var wg sync.WaitGroup
	var fail atomic.Int32
	t0 := time.Now()
	for _, q := range queries {
		wg.Add(1)
		go func(q query) {
			defer wg.Done()
			got := 0
			err := h.CatalogReadManyStream(ctx, sw, idx, q.filter, Item{}, func(_ any) error {
				got++
				return nil
			})
			if err != nil {
				fmt.Printf("    [%s] error: %v\n", q.label, err)
				fail.Add(1)
				return
			}
			if q.want >= 0 && got != q.want {
				fmt.Printf("    [%s] got=%d want=%d\n", q.label, got, q.want)
				fail.Add(1)
				return
			}
			fmt.Printf("    [%s] rows=%d\n", q.label, got)
		}(q)
	}
	wg.Wait()
	elapsed := time.Since(t0)
	record("concurrent-cold-builds", fail.Load() == 0, elapsed,
		fmt.Sprintf("%d parallel queries", len(queries)))
}

func benchMatrix(ctx context.Context, h hydraidego.Hydraidego) {
	fmt.Println("=== Benchmark matrix (size × ASN cardinality) ===")
	type cell struct {
		size, asnCard int
	}
	cells := []cell{
		{1000, 10},
		{10000, 50},
		{10000, 100},
		{50000, 100},
		{50000, 500},
	}
	for _, c := range cells {
		sw := swampName("bench", fmt.Sprintf("s%d-a%d", c.size, c.asnCard))
		_ = h.Destroy(ctx, sw)
		if err := seed(ctx, h, sw, c.size, c.asnCard); err != nil {
			log.Fatalf("seed: %v", err)
		}
		idx := &hydraidego.Index{IndexType: hydraidego.IndexKey, IndexOrder: hydraidego.IndexOrderAsc}
		filter := hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "asn", 0))

		t0 := time.Now()
		_ = h.CatalogReadManyStream(ctx, sw, idx, filter, Item{}, func(_ any) error { return nil })
		cold := time.Since(t0)

		samples := make([]time.Duration, 5)
		for i := 0; i < 5; i++ {
			f := hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "asn", int64(i+1)))
			tx := time.Now()
			_ = h.CatalogReadManyStream(ctx, sw, idx, f, Item{}, func(_ any) error { return nil })
			samples[i] = time.Since(tx)
		}
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		warmMed := samples[len(samples)/2]
		speedup := float64(cold) / float64(warmMed)

		record(fmt.Sprintf("bench size=%d asnCard=%d", c.size, c.asnCard), true, warmMed,
			fmt.Sprintf("cold=%v warmMed=%v speedup=%.1fx",
				cold.Round(time.Microsecond), warmMed.Round(time.Microsecond), speedup))

		_ = h.Destroy(ctx, sw)
	}
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	r, cleanup := setup.MustClient(ctx)
	defer cleanup()
	h := r.GetHydraidego()

	for _, realm := range []string{"matrix", "cold-warm", "cycle50", "concurrent", "bench"} {
		if err := setup.Pattern(ctx, r, swampPattern(realm)); err != nil {
			log.Fatalf("register realm %s: %v", realm, err)
		}
	}

	matrixCorrectness(ctx, h)
	coldVsWarm(ctx, h, "1K", 1000)
	coldVsWarm(ctx, h, "10K", 10000)
	coldVsWarm(ctx, h, "50K", 50000)
	cycle50ASN(ctx, h, 50000)
	lifecycleRebuild(ctx, r, h)
	concurrentColdBuilds(ctx, h)
	benchMatrix(ctx, h)

	fmt.Println()
	fmt.Println("=== Summary ===")
	pass, fail := 0, 0
	for _, res := range results {
		if res.Pass {
			pass++
		} else {
			fail++
		}
	}
	fmt.Printf("Total: %d  PASS: %d  FAIL: %d\n", pass+fail, pass, fail)
	if fail > 0 {
		fmt.Println("\nFailures:")
		for _, res := range results {
			if !res.Pass {
				fmt.Printf("  - %s: %s\n", res.Name, res.Note)
			}
		}
		os.Exit(1)
	}
}
