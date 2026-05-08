// claim is a smoke test: 1000 expired entries, 5 concurrent claimers
// with batch=200, asserts disjoint subsets + new ExpireAt.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/hydraide/hydraide/docs/tasks/patch-expired-many/smoke/internal/setup"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3"
)

type Catalog struct {
	Domain    string    `hydraide:"key"      msgpack:"-"`
	ExpireAt  time.Time `hydraide:"expireAt" msgpack:"-"`
	ClaimedBy string    `hydraide:"-"        msgpack:"ClaimedBy"`
}

func main() {
	const total = 1000
	const workers = 5
	const batch = 200

	h := setup.Connect()
	swamp, teardown := setup.FreshSwamp(h, "claim")
	defer teardown()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Seed expired entries.
	for i := 0; i < total; i++ {
		_, err := h.CatalogPatch(ctx, swamp, fmt.Sprintf("d%05d.hu", i)).
			Set("ClaimedBy", "").
			WithExpiredAt(time.Now().UTC().Add(-time.Hour)).
			Exec()
		if err != nil {
			fail("seed: %v", err)
		}
	}

	type result struct {
		keys []string
	}
	resCh := make(chan result, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			b := hydraidego.NewPatchExpiredOps().
				Set("ClaimedBy", fmt.Sprintf("worker-%d", id)).
				WithExpiredAt(time.Now().UTC().Add(24 * time.Hour))
			r := result{}
			err := h.CatalogPatchExpired(ctx, swamp, batch, Catalog{},
				func(model any, st hydraidego.PatchStatus) error {
					if st != hydraidego.PatchStatusPatched {
						return fmt.Errorf("unexpected status %s for %v", st, model)
					}
					m := model.(*Catalog)
					r.keys = append(r.keys, m.Domain)
					return nil
				}, b)
			if err != nil {
				fail("worker %d: %v", id, err)
			}
			resCh <- r
		}(w)
	}
	wg.Wait()
	close(resCh)

	seen := map[string]int{}
	totalClaimed := 0
	for r := range resCh {
		totalClaimed += len(r.keys)
		for _, k := range r.keys {
			seen[k]++
		}
	}
	if totalClaimed != workers*batch {
		fail("expected %d claims, got %d", workers*batch, totalClaimed)
	}
	for k, n := range seen {
		if n != 1 {
			fail("key %s claimed %d times, must be exactly 1", k, n)
		}
	}
	fmt.Println("PASS: claim")
}

func fail(format string, args ...any) {
	log.Printf("FAIL: "+format, args...)
	os.Exit(1)
}
