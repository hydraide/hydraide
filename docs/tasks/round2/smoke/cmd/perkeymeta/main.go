// perkeymeta is a smoke test for R2-1: per-key Meta on TreasurePatch.
// Patches two keys with a request-level WithExpiredAt; one key carries
// its own per-key WithExpiredAt that overrides the request-level one.
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

type Row struct {
	Key      string    `hydraide:"key"`
	X        int32     `hydraide:"value"`
	ExpireAt time.Time `hydraide:"expireAt"`
}

func main() {
	h := setup.Connect()
	swamp, teardown := setup.FreshSwamp(h, "perkeymeta")
	defer teardown()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sharedExp := time.Now().UTC().Add(1 * time.Hour).Truncate(time.Microsecond)
	customExp := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Microsecond)

	requests := []*hydraidego.PatchManyRequest{
		// Inherits request-level WithExpiredAt(sharedExp) — set on the
		// builder itself, since CatalogPatchFieldsMany has no
		// request-level Meta knob, every key carries its own.
		{Builder: hydraidego.NewPatchBuilder("shared").
			Set("X", int32(1)).
			WithExpiredAt(sharedExp)},
		// Per-key Meta overrides the "shared" timing.
		{Builder: hydraidego.NewPatchBuilder("custom").
			Set("X", int32(2)).
			WithExpiredAt(customExp)},
	}
	if err := h.CatalogPatchFieldsMany(ctx, swamp, requests, nil); err != nil {
		fail("PatchFieldsMany: %v", err)
	}

	got := map[string]Row{}
	for _, k := range []string{"shared", "custom"} {
		var row Row
		if err := h.CatalogRead(ctx, swamp, k, &row); err != nil {
			fail("read %s: %v", k, err)
		}
		got[k] = row
	}
	if !got["shared"].ExpireAt.Equal(sharedExp) {
		fail("shared ExpireAt: want %v got %v", sharedExp, got["shared"].ExpireAt)
	}
	if !got["custom"].ExpireAt.Equal(customExp) {
		fail("custom ExpireAt: want %v got %v", customExp, got["custom"].ExpireAt)
	}
	fmt.Println("PASS: perkeymeta")
}

func fail(format string, args ...any) {
	log.Printf("FAIL: "+format, args...)
	os.Exit(1)
}
