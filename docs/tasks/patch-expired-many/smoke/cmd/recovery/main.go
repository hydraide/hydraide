// recovery is a smoke test: claim with already-elapsed lease, then
// re-claim from a different worker to confirm the recovery flow.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
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
	const total = 10

	h := setup.Connect()
	swamp, teardown := setup.FreshSwamp(h, "recovery")
	defer teardown()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for i := 0; i < total; i++ {
		_, err := h.CatalogPatch(ctx, swamp, fmt.Sprintf("d%d.hu", i)).
			Set("ClaimedBy", "").
			WithExpiredAt(time.Now().UTC().Add(-time.Hour)).
			Exec()
		if err != nil {
			fail("seed: %v", err)
		}
	}

	// Worker A claims with a lease already in the past — simulating an
	// instant crash.
	first := 0
	err := h.CatalogPatchExpired(ctx, swamp, 100, Catalog{},
		func(model any, st hydraidego.PatchStatus) error {
			first++
			return nil
		},
		hydraidego.NewPatchExpiredOps().
			Set("ClaimedBy", "worker-A").
			WithExpiredAt(time.Now().UTC().Add(-50*time.Millisecond)),
	)
	if err != nil {
		fail("worker-A claim: %v", err)
	}
	if first != total {
		fail("worker-A expected %d claims, got %d", total, first)
	}

	// Worker B re-claims because the worker-A lease elapsed already.
	second := 0
	err = h.CatalogPatchExpired(ctx, swamp, 100, Catalog{},
		func(model any, st hydraidego.PatchStatus) error {
			m := model.(*Catalog)
			if m.ClaimedBy != "worker-B" {
				return fmt.Errorf("want worker-B, got %q for %s", m.ClaimedBy, m.Domain)
			}
			second++
			return nil
		},
		hydraidego.NewPatchExpiredOps().
			Set("ClaimedBy", "worker-B").
			WithExpiredAt(time.Now().UTC().Add(time.Hour)),
	)
	if err != nil {
		fail("worker-B re-claim: %v", err)
	}
	if second != total {
		fail("worker-B expected %d re-claims, got %d", total, second)
	}
	fmt.Println("PASS: recovery")
}

func fail(format string, args ...any) {
	log.Printf("FAIL: "+format, args...)
	os.Exit(1)
}
