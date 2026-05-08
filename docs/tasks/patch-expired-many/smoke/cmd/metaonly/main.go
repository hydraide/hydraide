// metaonly is a smoke test: empty Ops + Meta.SetExpiredAt slides the
// lease forward without touching the body.
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
	Counter   int32     `hydraide:"-"        msgpack:"Counter"`
}

func main() {
	h := setup.Connect()
	swamp, teardown := setup.FreshSwamp(h, "metaonly")
	defer teardown()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const total = 4
	for i := 0; i < total; i++ {
		_, err := h.CatalogPatch(ctx, swamp, fmt.Sprintf("d%d.hu", i)).
			Set("ClaimedBy", "init").
			Set("Counter", int32(7)).
			WithExpiredAt(time.Now().UTC().Add(-time.Hour)).
			Exec()
		if err != nil {
			fail("seed: %v", err)
		}
	}

	newExp := time.Now().UTC().Add(2 * time.Hour)
	patched := 0
	err := h.CatalogPatchExpired(ctx, swamp, 100, Catalog{},
		func(model any, st hydraidego.PatchStatus) error {
			m := model.(*Catalog)
			if st != hydraidego.PatchStatusPatched {
				return fmt.Errorf("status %s for %s", st, m.Domain)
			}
			if m.ClaimedBy != "init" || m.Counter != 7 {
				return fmt.Errorf("body mutated on meta-only patch: %+v", m)
			}
			if delta := m.ExpireAt.Sub(newExp); delta > time.Second || delta < -time.Second {
				return fmt.Errorf("ExpireAt drift: got %v want %v", m.ExpireAt, newExp)
			}
			patched++
			return nil
		},
		hydraidego.NewPatchExpiredOps().WithExpiredAt(newExp).WithUpdatedAt(),
	)
	if err != nil {
		fail("meta-only PatchExpired: %v", err)
	}
	if patched != total {
		fail("expected %d patched, got %d", total, patched)
	}
	fmt.Println("PASS: metaonly")
}

func fail(format string, args ...any) {
	log.Printf("FAIL: "+format, args...)
	os.Exit(1)
}
