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
//   - Generic 50-tenant claim cycle wall-clock
//   - lifecycle: re-summon after CloseAfterIdle rebuilds the bucket
//   - concurrency: parallel cold-builds on different fields
//   - bench matrix: cold vs warm across (size × tenant cardinality)
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
	Tenant   int64  `msgpack:"tenant"`
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

func seed(ctx context.Context, h hydraidego.Hydraidego, swamp name.Name, n, tenantCard int) error {
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
					Tenant:   int64(i % tenantCard),
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
	tenantCard := 10
	statuses := []string{"ready", "pending", "running", "done", "failed"}
	categories := []string{"A", "B", "C", "D", "E"}
	items := make([]any, 0, n)
	byTenant := map[int64]int{}
	byStatus := map[string]int{}
	for i := 0; i < n; i++ {
		body := &Body{
			Tenant:   int64(i % tenantCard),
			Status:   statuses[i%len(statuses)],
			Category: categories[i%len(categories)],
			Score:    int64(i),
		}
		items = append(items, &Item{Key: fmt.Sprintf("k%04d", i), Body: body})
		byTenant[body.Tenant]++
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

	run("M2 tenant=5",
		hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "tenant", 5)),
		byTenant[5])

	exp5ready := 0
	for _, it := range items {
		x := it.(*Item)
		if x.Body.Tenant == 5 && x.Body.Status == "ready" {
			exp5ready++
		}
	}
	run("M5 tenant=5 AND status=ready",
		hydraidego.FilterAND(
			hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "tenant", 5),
			hydraidego.FilterBytesFieldString(hydraidego.Equal, "status", "ready"),
		),
		exp5ready)

	run("M6 tenant=5 OR tenant=6",
		hydraidego.FilterOR(
			hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "tenant", 5),
			hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "tenant", 6),
		),
		byTenant[5]+byTenant[6])

	exp7 := 0
	for _, it := range items {
		x := it.(*Item)
		if x.Body.Tenant == 5 || x.Body.Status == "ready" {
			exp7++
		}
	}
	run("M7 tenant=5 OR status=ready",
		hydraidego.FilterOR(
			hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "tenant", 5),
			hydraidego.FilterBytesFieldString(hydraidego.Equal, "status", "ready"),
		),
		exp7)

	run("M8 tenant IN (1,2,3)",
		hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64In("tenant", 1, 2, 3)),
		byTenant[1]+byTenant[2]+byTenant[3])

	exp9 := 0
	for _, it := range items {
		x := it.(*Item)
		if x.Body.Tenant == 5 && x.Body.Score > 100 {
			exp9++
		}
	}
	run("M9 tenant=5 AND score>100 (range residual)",
		hydraidego.FilterAND(
			hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "tenant", 5),
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
		if x.Body.Tenant == 5 && x.Body.Status != "ready" {
			exp22++
		}
	}
	run("M22 tenant=5 AND status!=ready",
		hydraidego.FilterAND(
			hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "tenant", 5),
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
	filter := hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "tenant", 42))

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

func cycle50Tenant(ctx context.Context, h hydraidego.Hydraidego, n int) {
	fmt.Printf("=== Generic 50-tenant claim cycle (n=%d, tenantCard=100) ===\n", n)
	sw := swampName("cycle50", "main")
	_ = h.Destroy(ctx, sw)
	defer func() { _ = h.Destroy(ctx, sw) }()

	if err := seed(ctx, h, sw, n, 100); err != nil {
		log.Fatalf("seed: %v", err)
	}
	idx := &hydraidego.Index{IndexType: hydraidego.IndexKey, IndexOrder: hydraidego.IndexOrderAsc}

	warmup := hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "tenant", 0))
	_ = h.CatalogReadManyStream(ctx, sw, idx, warmup, Item{}, func(_ any) error { return nil })

	t0 := time.Now()
	total := 0
	for tenant := int64(0); tenant < 50; tenant++ {
		filter := hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "tenant", tenant))
		_ = h.CatalogReadManyStream(ctx, sw, idx, filter, Item{}, func(_ any) error {
			total++
			return nil
		})
	}
	elapsed := time.Since(t0)
	record(fmt.Sprintf("50-tenant-cycle/n=%d", n), true, elapsed,
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
	filter := hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "tenant", 7))

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
		{"tenant=10", hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "tenant", 10)), 5000 / 50},
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

// mutationPropagation seeds a swamp, builds a bucket, then runs every
// mutation kind (insert / update / delete) against an already-warm
// bucket and verifies the lookup count changes match the expected
// post-mutation state.
func mutationPropagation(ctx context.Context, h hydraidego.Hydraidego) {
	fmt.Println("=== Mutation propagation against a warm bucket ===")
	sw := swampName("mutation", "main")
	_ = h.Destroy(ctx, sw)
	defer func() { _ = h.Destroy(ctx, sw) }()

	// Seed: 100 records, tenantCard=10. Each tenant slot holds 10.
	if err := seed(ctx, h, sw, 100, 10); err != nil {
		log.Fatalf("seed: %v", err)
	}
	idx := &hydraidego.Index{IndexType: hydraidego.IndexKey, IndexOrder: hydraidego.IndexOrderAsc}

	count := func(tenant int64) int {
		got := 0
		filter := hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "tenant", tenant))
		_ = h.CatalogReadManyStream(ctx, sw, idx, filter, Item{}, func(_ any) error {
			got++
			return nil
		})
		return got
	}

	// Build the bucket via the first lookup.
	base := count(3)
	if base != 10 {
		record("mutation/build", false, 0, fmt.Sprintf("initial tenant=3 count=%d, want 10", base))
		return
	}
	record("mutation/build", true, 0, fmt.Sprintf("initial tenant=3 count=%d", base))

	// Insert: add 5 new records with tenant=3.
	t0 := time.Now()
	inserts := make([]any, 5)
	for i := 0; i < 5; i++ {
		inserts[i] = &Item{
			Key:  fmt.Sprintf("new-%d", i),
			Body: &Body{Tenant: 3, Status: "ready", Category: "A", Score: 999},
		}
	}
	_ = h.CatalogSaveMany(ctx, sw, inserts, nil)
	after := count(3)
	record("mutation/insert", after == 15, time.Since(t0),
		fmt.Sprintf("post-insert count=%d, want 15", after))

	// Update: move 2 records from tenant=3 to tenant=7. Use CatalogSave on
	// existing keys (overwrite).
	t0 = time.Now()
	updates := []any{
		&Item{Key: "new-0", Body: &Body{Tenant: 7, Status: "ready", Category: "A", Score: 999}},
		&Item{Key: "new-1", Body: &Body{Tenant: 7, Status: "ready", Category: "A", Score: 999}},
	}
	_ = h.CatalogSaveMany(ctx, sw, updates, nil)
	afterT3 := count(3)
	afterT7 := count(7)
	pass := afterT3 == 13 && afterT7 == 12
	record("mutation/update", pass, time.Since(t0),
		fmt.Sprintf("tenant=3 count=%d (want 13), tenant=7 count=%d (want 12)", afterT3, afterT7))

	// Delete: remove 3 records from tenant=3.
	t0 = time.Now()
	for i := 2; i < 5; i++ {
		_ = h.CatalogDelete(ctx, sw, fmt.Sprintf("new-%d", i))
	}
	afterDel := count(3)
	record("mutation/delete", afterDel == 10, time.Since(t0),
		fmt.Sprintf("post-delete count=%d, want 10", afterDel))
}

// multiBucketSync builds two buckets on the same swamp, mutates a
// record so both buckets must update, and verifies both reflect.
func multiBucketSync(ctx context.Context, h hydraidego.Hydraidego) {
	fmt.Println("=== Multi-bucket sync on single Save ===")
	sw := swampName("multibucket", "main")
	_ = h.Destroy(ctx, sw)
	defer func() { _ = h.Destroy(ctx, sw) }()

	// Seed deterministic: 50 records, tenant ∈ [0,9], status cycles
	// through 5 values.
	statuses := []string{"ready", "pending", "running", "done", "failed"}
	items := make([]any, 50)
	for i := 0; i < 50; i++ {
		items[i] = &Item{
			Key:  fmt.Sprintf("k%02d", i),
			Body: &Body{Tenant: int64(i % 10), Status: statuses[i%5], Category: "X", Score: int64(i)},
		}
	}
	if err := h.CatalogSaveMany(ctx, sw, items, nil); err != nil {
		log.Fatalf("seed: %v", err)
	}

	idx := &hydraidego.Index{IndexType: hydraidego.IndexKey, IndexOrder: hydraidego.IndexOrderAsc}
	count := func(filter *hydraidego.FilterGroup) int {
		got := 0
		_ = h.CatalogReadManyStream(ctx, sw, idx, filter, Item{}, func(_ any) error {
			got++
			return nil
		})
		return got
	}

	// Build tenant bucket and status bucket via two lookups.
	tenantFilter := hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "tenant", 3))
	statusFilter := hydraidego.FilterAND(hydraidego.FilterBytesFieldString(hydraidego.Equal, "status", "ready"))
	cTenant := count(tenantFilter)
	cStatus := count(statusFilter)
	record("multibucket/build", cTenant == 5 && cStatus == 10, 0,
		fmt.Sprintf("tenant=3 count=%d (want 5), status=ready count=%d (want 10)", cTenant, cStatus))

	// Pick k03 (originally tenant=3, status=done) and rewrite it as
	// tenant=7, status=ready. Both buckets must update: tenant=3 -> 4,
	// tenant=7 -> 6; status=done -> 9, status=ready -> 11.
	t0 := time.Now()
	_ = h.CatalogSaveMany(ctx, sw, []any{
		&Item{Key: "k03", Body: &Body{Tenant: 7, Status: "ready", Category: "X", Score: 3}},
	}, nil)
	cT3 := count(tenantFilter)
	cT7 := count(hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "tenant", 7)))
	cStatusReady := count(statusFilter)
	cStatusDone := count(hydraidego.FilterAND(hydraidego.FilterBytesFieldString(hydraidego.Equal, "status", "done")))

	pass := cT3 == 4 && cT7 == 6 && cStatusReady == 11 && cStatusDone == 9
	record("multibucket/single-save-updates-both", pass, time.Since(t0),
		fmt.Sprintf("tenant=3:%d(want 4) tenant=7:%d(want 6) status=ready:%d(want 11) status=done:%d(want 9)",
			cT3, cT7, cStatusReady, cStatusDone))
}

// sequentialBuildsBothCorrect builds an `tenant` bucket, then a `status`
// bucket on the same swamp, and verifies both return correct counts
// after the second build. Complements concurrentColdBuilds.
func sequentialBuildsBothCorrect(ctx context.Context, h hydraidego.Hydraidego) {
	fmt.Println("=== Sequential builds: two fields, both stay correct ===")
	sw := swampName("sequential", "main")
	_ = h.Destroy(ctx, sw)
	defer func() { _ = h.Destroy(ctx, sw) }()

	if err := seed(ctx, h, sw, 1000, 10); err != nil {
		log.Fatalf("seed: %v", err)
	}
	idx := &hydraidego.Index{IndexType: hydraidego.IndexKey, IndexOrder: hydraidego.IndexOrderAsc}

	count := func(filter *hydraidego.FilterGroup) int {
		got := 0
		_ = h.CatalogReadManyStream(ctx, sw, idx, filter, Item{}, func(_ any) error { got++; return nil })
		return got
	}

	// First build: tenant=4.
	first := count(hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "tenant", 4)))
	// Second build (different field): status=ready. 5 statuses cycling
	// over 1000 rows → 200 of each.
	second := count(hydraidego.FilterAND(hydraidego.FilterBytesFieldString(hydraidego.Equal, "status", "ready")))
	// Re-query the first bucket to confirm it survived the second
	// build (different field path, must not interfere).
	firstAgain := count(hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "tenant", 4)))

	pass := first == 100 && second == 200 && firstAgain == 100
	record("sequential-builds", pass, 0,
		fmt.Sprintf("tenant=4 first=%d (want 100), status=ready=%d (want 200), tenant=4 again=%d (want 100)",
			first, second, firstAgain))
}

func benchMatrix(ctx context.Context, h hydraidego.Hydraidego) {
	fmt.Println("=== Benchmark matrix (size × tenant cardinality) ===")
	type cell struct {
		size, tenantCard int
	}
	cells := []cell{
		{1000, 10},
		{10000, 50},
		{10000, 100},
		{50000, 100},
		{50000, 500},
	}
	for _, c := range cells {
		sw := swampName("bench", fmt.Sprintf("s%d-a%d", c.size, c.tenantCard))
		_ = h.Destroy(ctx, sw)
		if err := seed(ctx, h, sw, c.size, c.tenantCard); err != nil {
			log.Fatalf("seed: %v", err)
		}
		idx := &hydraidego.Index{IndexType: hydraidego.IndexKey, IndexOrder: hydraidego.IndexOrderAsc}
		filter := hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "tenant", 0))

		t0 := time.Now()
		_ = h.CatalogReadManyStream(ctx, sw, idx, filter, Item{}, func(_ any) error { return nil })
		cold := time.Since(t0)

		samples := make([]time.Duration, 5)
		for i := 0; i < 5; i++ {
			f := hydraidego.FilterAND(hydraidego.FilterBytesFieldInt64(hydraidego.Equal, "tenant", int64(i+1)))
			tx := time.Now()
			_ = h.CatalogReadManyStream(ctx, sw, idx, f, Item{}, func(_ any) error { return nil })
			samples[i] = time.Since(tx)
		}
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		warmMed := samples[len(samples)/2]
		speedup := float64(cold) / float64(warmMed)

		record(fmt.Sprintf("bench size=%d tenantCard=%d", c.size, c.tenantCard), true, warmMed,
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

	for _, realm := range []string{"matrix", "mutation", "multibucket", "sequential", "cold-warm", "cycle50", "concurrent", "bench"} {
		if err := setup.Pattern(ctx, r, swampPattern(realm)); err != nil {
			log.Fatalf("register realm %s: %v", realm, err)
		}
	}

	matrixCorrectness(ctx, h)
	mutationPropagation(ctx, h)
	multiBucketSync(ctx, h)
	sequentialBuildsBothCorrect(ctx, h)
	coldVsWarm(ctx, h, "1K", 1000)
	coldVsWarm(ctx, h, "10K", 10000)
	coldVsWarm(ctx, h, "50K", 50000)
	cycle50Tenant(ctx, h, 50000)
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
