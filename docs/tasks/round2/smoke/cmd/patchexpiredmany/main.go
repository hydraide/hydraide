// patchexpiredmany is a smoke test for R2-3:
// CatalogPatchExpiredManyFromMany. Seeds expired entries across two
// swamps, claims them in a single multi-swamp RPC, asserts the per-swamp
// counts match.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hydraide/hydraide/docs/tasks/round2/smoke/internal/setup"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name"
)

type Row struct {
	Key       string    `hydraide:"key"`
	ClaimedBy string    `hydraide:"value"`
	ExpireAt  time.Time `hydraide:"expireAt"`
}

func main() {
	h := setup.Connect()
	swamps, teardown := setup.FreshSwamps(h, "patchexpiredmany", 2)
	defer teardown()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Seed 4 expired entries in swamp A, 2 in swamp B.
	seed := func(sw name.Name, n int) {
		for i := 0; i < n; i++ {
			_, err := h.CatalogPatch(ctx, sw, fmt.Sprintf("k%d", i)).
				Set("ClaimedBy", "").
				WithExpiredAt(time.Now().UTC().Add(-time.Hour)). // already expired
				Exec()
			if err != nil {
				fail("seed %s/%d: %v", sw.Get(), i, err)
			}
		}
	}
	seed(swamps[0], 4)
	seed(swamps[1], 2)

	requests := []*hydraidego.PatchExpiredManyFromManyRequest{
		{SwampName: swamps[0], HowMany: 4,
			Builder: hydraidego.NewPatchExpiredOps().
				Set("ClaimedBy", "worker-1").
				WithExpiredAt(time.Now().UTC().Add(time.Hour))},
		{SwampName: swamps[1], HowMany: 5, // ask more than exists
			Builder: hydraidego.NewPatchExpiredOps().
				Set("ClaimedBy", "worker-1").
				WithExpiredAt(time.Now().UTC().Add(time.Hour))},
	}

	perSwamp := map[string]int{}
	err := h.CatalogPatchExpiredManyFromMany(ctx, requests, Row{},
		func(swampName name.Name, model any, status hydraidego.PatchStatus, swampErr error) error {
			if swampErr != nil {
				return fmt.Errorf("swamp %s: %v", swampName.Get(), swampErr)
			}
			if status != hydraidego.PatchStatusPatched {
				return fmt.Errorf("swamp %s status %s", swampName.Get(), status)
			}
			perSwamp[swampName.Get()]++
			return nil
		})
	if err != nil {
		fail("PatchExpiredManyFromMany: %v", err)
	}
	if perSwamp[swamps[0].Get()] != 4 {
		fail("swamp A: want 4 got %d", perSwamp[swamps[0].Get()])
	}
	if perSwamp[swamps[1].Get()] != 2 {
		fail("swamp B: want 2 got %d", perSwamp[swamps[1].Get()])
	}
	fmt.Println("PASS: patchexpiredmany")
}

func fail(format string, args ...any) {
	log.Printf("FAIL: "+format, args...)
	os.Exit(1)
}
