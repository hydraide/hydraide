// shiftexpiredmany is a smoke test for R2-7:
// CatalogShiftExpiredManyFromMany. Seeds expired entries across two
// swamps and asserts the per-swamp counts after a single multi-swamp
// shift.
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
	Key      string    `hydraide:"key"`
	X        int8      `hydraide:"value"`
	ExpireAt time.Time `hydraide:"expireAt"`
}

func main() {
	h := setup.Connect()
	swamps, teardown := setup.FreshSwamps(h, "shiftexpiredmany", 2)
	defer teardown()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	seed := func(sw name.Name, n int) {
		for i := 0; i < n; i++ {
			_, err := h.CatalogPatch(ctx, sw, fmt.Sprintf("k%d", i)).
				Set("X", int8(1)).
				WithExpiredAt(time.Now().UTC().Add(-time.Hour)).
				Exec()
			if err != nil {
				fail("seed %s/%d: %v", sw.Get(), i, err)
			}
		}
	}
	seed(swamps[0], 3)
	seed(swamps[1], 1)

	requests := []*hydraidego.ShiftExpiredManyFromManyRequest{
		{SwampName: swamps[0], HowMany: 5},
		{SwampName: swamps[1], HowMany: 5},
	}

	perSwamp := map[string]int{}
	err := h.CatalogShiftExpiredManyFromMany(ctx, requests, Row{},
		func(swampName name.Name, model any, swampErr error) error {
			if swampErr != nil {
				return fmt.Errorf("swamp %s: %v", swampName.Get(), swampErr)
			}
			perSwamp[swampName.Get()]++
			return nil
		})
	if err != nil {
		fail("ShiftExpiredManyFromMany: %v", err)
	}
	if perSwamp[swamps[0].Get()] != 3 {
		fail("swamp A: want 3 got %d", perSwamp[swamps[0].Get()])
	}
	if perSwamp[swamps[1].Get()] != 1 {
		fail("swamp B: want 1 got %d", perSwamp[swamps[1].Get()])
	}
	fmt.Println("PASS: shiftexpiredmany")
}

func fail(format string, args ...any) {
	log.Printf("FAIL: "+format, args...)
	os.Exit(1)
}
