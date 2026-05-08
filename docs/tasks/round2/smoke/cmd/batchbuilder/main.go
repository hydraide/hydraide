// batchbuilder is a smoke test for R2-2: builder-reuse PatchManyRequest.
// Mixes Set + Inc + Append + IfField conditions across one batch.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hydraide/hydraide/docs/tasks/round2/smoke/internal/setup"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3"
)

func main() {
	h := setup.Connect()
	swamp, teardown := setup.FreshSwamp(h, "batchbuilder")
	defer teardown()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Seed with Counter=0 so the IfFieldLessThan(1) condition holds.
	for i := 0; i < 3; i++ {
		if _, err := h.CatalogPatchField(ctx, swamp, fmt.Sprintf("k%d", i), "Counter", int32(0)); err != nil {
			fail("seed k%d: %v", i, err)
		}
	}

	requests := []*hydraidego.PatchManyRequest{
		// Mixed ops: Set + Inc + Append + Cond.
		{Builder: hydraidego.NewPatchBuilder("k0").
			Set("Status", int8(2)).
			Inc("Counter", int32(1)).
			Append("History[]", "claim").
			IfFieldLessThan("Counter", int32(1))},
		// CAS that fails: Counter is 0, condition asks > 5.
		{Builder: hydraidego.NewPatchBuilder("k1").
			Set("Status", int8(99)).
			IfFieldGreaterThan("Counter", int32(5))},
		// Plain Set + per-key Meta (TTL slide).
		{Builder: hydraidego.NewPatchBuilder("k2").
			Set("Status", int8(3)).
			WithUpdatedAt().
			WithExpiredAt(time.Now().UTC().Add(24 * time.Hour))},
	}

	want := map[string]hydraidego.PatchStatus{
		"k0": hydraidego.PatchStatusPatched,
		"k1": hydraidego.PatchStatusConditionNotMet,
		"k2": hydraidego.PatchStatusPatched,
	}
	got := map[string]hydraidego.PatchStatus{}
	err := h.CatalogPatchFieldsMany(ctx, swamp, requests,
		func(key string, st hydraidego.PatchStatus, errMsg string) error {
			got[key] = st
			return nil
		})
	if err != nil {
		fail("PatchFieldsMany: %v", err)
	}
	for k, w := range want {
		if got[k] != w {
			fail("status %s: want %s got %s", k, w, got[k])
		}
	}
	fmt.Println("PASS: batchbuilder")
}

func fail(format string, args ...any) {
	log.Printf("FAIL: "+format, args...)
	os.Exit(1)
}
