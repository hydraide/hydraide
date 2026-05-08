// condmany is a smoke test: 10 keys with mixed Counter values, batch
// CAS-patch only those where Counter < 1.
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

func main() {
	h := setup.Connect()
	swamp, teardown := setup.FreshSwamp(h, "condmany")
	defer teardown()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Half with Counter=0, half with Counter=1.
	for i := 0; i < 5; i++ {
		if _, err := h.CatalogPatchField(ctx, swamp, fmt.Sprintf("k%d", i), "Counter", int32(0)); err != nil {
			fail("seed k%d: %v", i, err)
		}
	}
	for i := 5; i < 10; i++ {
		if _, err := h.CatalogPatchField(ctx, swamp, fmt.Sprintf("k%d", i), "Counter", int32(1)); err != nil {
			fail("seed k%d: %v", i, err)
		}
	}

	requests := make([]*hydraidego.PatchManyRequest, 0, 10)
	for i := 0; i < 10; i++ {
		requests = append(requests, &hydraidego.PatchManyRequest{
			Key:    fmt.Sprintf("k%d", i),
			Fields: map[string]any{"Counter": int32(99)},
			Cond: &hydraidego.PatchCond{
				Op:    hydraidego.PatchCondLessThan,
				Path:  "Counter",
				Value: int32(1),
			},
		})
	}

	patched, condFailed := 0, 0
	err := h.CatalogPatchFieldsMany(ctx, swamp, requests,
		func(key string, st hydraidego.PatchStatus, errMsg string) error {
			switch st {
			case hydraidego.PatchStatusPatched:
				patched++
			case hydraidego.PatchStatusConditionNotMet:
				condFailed++
			default:
				return fmt.Errorf("unexpected status %s for %s", st, key)
			}
			return nil
		})
	if err != nil {
		fail("PatchFieldsMany: %v", err)
	}
	if patched != 5 {
		fail("expected 5 patched, got %d", patched)
	}
	if condFailed != 5 {
		fail("expected 5 CONDITION_NOT_MET, got %d", condFailed)
	}
	fmt.Println("PASS: condmany")
}

func fail(format string, args ...any) {
	log.Printf("FAIL: "+format, args...)
	os.Exit(1)
}
