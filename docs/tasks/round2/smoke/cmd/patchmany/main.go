// patchmany is a smoke test for R2-4: CatalogPatchManyToMany. Patches
// records across two swamps in one round-trip; asserts per-swamp success.
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

func main() {
	h := setup.Connect()
	swamps, teardown := setup.FreshSwamps(h, "patchmany", 2)
	defer teardown()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	requests := []*hydraidego.CatalogPatchManyToManyRequest{
		{SwampName: swamps[0], Patches: []*hydraidego.PatchManyRequest{
			{Builder: hydraidego.NewPatchBuilder("a1").Set("X", int8(1)).WithExpiredAt(time.Now().UTC().Add(time.Hour))},
			{Builder: hydraidego.NewPatchBuilder("a2").Set("X", int8(2))},
		}},
		{SwampName: swamps[1], Patches: []*hydraidego.PatchManyRequest{
			{Builder: hydraidego.NewPatchBuilder("b1").Set("Y", "hi")},
		}},
	}

	created, swampErrs := 0, 0
	err := h.CatalogPatchManyToMany(ctx, requests,
		func(swampName name.Name, key string, status hydraidego.PatchStatus, errMsg string, swampErr error) error {
			if swampErr != nil {
				swampErrs++
				return nil
			}
			if status == hydraidego.PatchStatusCreated {
				created++
			} else {
				return fmt.Errorf("swamp %s key %s status %s err=%q", swampName.Get(), key, status, errMsg)
			}
			return nil
		})
	if err != nil {
		fail("PatchManyToMany: %v", err)
	}
	if swampErrs != 0 {
		fail("expected 0 swamp errors, got %d", swampErrs)
	}
	if created != 3 {
		fail("expected 3 CREATED, got %d", created)
	}
	fmt.Println("PASS: patchmany")
}

func fail(format string, args ...any) {
	log.Printf("FAIL: "+format, args...)
	os.Exit(1)
}
