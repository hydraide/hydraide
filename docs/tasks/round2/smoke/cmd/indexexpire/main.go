// indexexpire is a smoke test for R2-6 (already-existing
// IndexExpirationTime support). Seeds 5 entries with shuffled
// ExpiredAt values and asserts CatalogReadManyStream returns them in
// ascending ExpireAt order.
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
	swamp, teardown := setup.FreshSwamp(h, "indexexpire")
	defer teardown()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)
	// Seed in shuffled order — keys k0..k4, ExpireAt at +5h, +1h, +3h, +2h, +4h.
	offsets := []time.Duration{5 * time.Hour, 1 * time.Hour, 3 * time.Hour, 2 * time.Hour, 4 * time.Hour}
	for i, off := range offsets {
		_, err := h.CatalogPatch(ctx, swamp, fmt.Sprintf("k%d", i)).
			Set("X", int32(i)).
			WithExpiredAt(now.Add(off)).
			Exec()
		if err != nil {
			fail("seed k%d: %v", i, err)
		}
	}

	// Expected ASC order: +1h (k1), +2h (k3), +3h (k2), +4h (k4), +5h (k0).
	wantOrder := []string{"k1", "k3", "k2", "k4", "k0"}

	got := make([]string, 0, 5)
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexExpirationTime,
		IndexOrder: hydraidego.IndexOrderAsc,
		MaxResults: 100,
	}
	err := h.CatalogReadManyStream(ctx, swamp, index, nil, Row{},
		func(model any) error {
			row := model.(*Row)
			got = append(got, row.Key)
			return nil
		})
	if err != nil {
		fail("CatalogReadManyStream: %v", err)
	}
	if len(got) != len(wantOrder) {
		fail("got %d rows, want %d", len(got), len(wantOrder))
	}
	for i, k := range wantOrder {
		if got[i] != k {
			fail("ASC order index %d: want %s got %s (full=%v)", i, k, got[i], got)
		}
	}
	fmt.Println("PASS: indexexpire")
}

func fail(format string, args ...any) {
	log.Printf("FAIL: "+format, args...)
	os.Exit(1)
}
