// shiftmatching is the smoke harness for the parametric CatalogShift
// surface plus Cap-on-PatchExpired. It exercises three end-to-end flows
// against a real HydrAIDE instance:
//
//  1. CatalogShift with IndexKey ASC + a Filter (selects only "pending"
//     status), asserts deterministic order and result count.
//  2. CatalogShift with Cap (no claimed records yet → budget caps the
//     result), asserts CapReached=true and exact result count.
//  3. CatalogPatchExpiredWithResult with Cap (some claimed records
//     already present → budget=2), asserts CapReached=true and the
//     post-op state respects Cap.MaxMatching.
//
// Run with HYDRAIDE_HOST + cert env vars set (see the setup helper).
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
	Status   string    `hydraide:"Status"`
	ExpireAt time.Time `hydraide:"expireAt"`
}

func main() {
	h := setup.Connect()
	swamps, teardown := setup.FreshSwamps(h, "shiftmatching", 3)
	defer teardown()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	runShiftWithFilter(ctx, h, swamps[0])
	runShiftWithCap(ctx, h, swamps[1])
	runPatchExpiredCap(ctx, h, swamps[2])

	fmt.Println("PASS: shiftmatching")
}

// runShiftWithFilter seeds 3 "pending" + 2 "done" expired records and
// asserts that CatalogShift with a status="pending" filter returns
// exactly the 3 pending records.
func runShiftWithFilter(ctx context.Context, h hydraidego.Hydraidego, swamp name.Name) {
	for i := 0; i < 3; i++ {
		seed(ctx, h, swamp, fmt.Sprintf("k-%d", i), "pending")
	}
	for i := 3; i < 5; i++ {
		seed(ctx, h, swamp, fmt.Sprintf("k-%d", i), "done")
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldString(hydraidego.Equal, "Status", "pending"),
	)

	seen := 0
	res, err := h.CatalogShift(ctx, swamp, &hydraidego.ShiftRequest{
		IndexType:  hydraidego.IndexKey,
		IndexOrder: hydraidego.IndexOrderAsc,
		HowMany:    100,
		Filters:    filters,
	}, Row{}, func(model any) error {
		seen++
		return nil
	})
	if err != nil {
		fail("shift-with-filter: %v", err)
	}
	if seen != 3 {
		fail("shift-with-filter: want 3 returned, got %d", seen)
	}
	if res.CapReached {
		fail("shift-with-filter: CapReached must be false (no Cap)")
	}
}

// runShiftWithCap seeds 10 "pending" records, then runs CatalogShift
// with a Cap that limits the operation to 4 results.
func runShiftWithCap(ctx context.Context, h hydraidego.Hydraidego, swamp name.Name) {
	for i := 0; i < 10; i++ {
		seed(ctx, h, swamp, fmt.Sprintf("k-%d", i), "pending")
	}

	// Cap.Filter counts records with Status=="claimed" — currently zero,
	// so budget = 4 - 0 = 4. The shift returns at most 4 records and
	// CapReached must be true (10 candidates > 4 budget).
	capFilter := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldString(hydraidego.Equal, "Status", "claimed"),
	)

	seen := 0
	res, err := h.CatalogShift(ctx, swamp, &hydraidego.ShiftRequest{
		IndexType:  hydraidego.IndexKey,
		IndexOrder: hydraidego.IndexOrderAsc,
		HowMany:    100,
		Cap:        &hydraidego.Cap{Filter: capFilter, MaxMatching: 4},
	}, Row{}, func(model any) error {
		seen++
		return nil
	})
	if err != nil {
		fail("shift-with-cap: %v", err)
	}
	if seen != 4 {
		fail("shift-with-cap: budget=4 want 4 returned, got %d", seen)
	}
	if !res.CapReached {
		fail("shift-with-cap: CapReached must be true when budget bounds result")
	}
}

// runPatchExpiredCap seeds 5 "claimed" + 5 "pending" expired records,
// then patches pending → claimed under a Cap with MaxMatching=6 (already
// 5 claimed → budget=1). Asserts exactly 1 patched, CapReached=true.
func runPatchExpiredCap(ctx context.Context, h hydraidego.Hydraidego, swamp name.Name) {
	for i := 0; i < 5; i++ {
		seed(ctx, h, swamp, fmt.Sprintf("c-%d", i), "claimed")
	}
	for i := 0; i < 5; i++ {
		seed(ctx, h, swamp, fmt.Sprintf("p-%d", i), "pending")
	}

	capFilter := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldString(hydraidego.Equal, "Status", "claimed"),
	)

	builder := hydraidego.NewPatchExpiredOps().
		Set("Status", "claimed").
		WithCap(&hydraidego.Cap{Filter: capFilter, MaxMatching: 6}).
		WithExpiredAt(time.Now().UTC().Add(time.Hour))

	patched := 0
	res, err := h.CatalogPatchExpiredWithResult(ctx, swamp, 100, Row{},
		func(model any, status hydraidego.PatchStatus) error {
			if status == hydraidego.PatchStatusPatched {
				patched++
			}
			return nil
		}, builder)
	if err != nil {
		fail("patch-expired-cap: %v", err)
	}
	if patched != 1 {
		fail("patch-expired-cap: budget=1 want 1 patched, got %d", patched)
	}
	if !res.CapReached {
		fail("patch-expired-cap: CapReached must be true when budget bounds result")
	}
}

// seed writes a single Row with a past ExpiredAt so it lands in the
// PatchExpired / Shift selection set immediately.
func seed(ctx context.Context, h hydraidego.Hydraidego, swamp name.Name, key, status string) {
	_, err := h.CatalogPatch(ctx, swamp, key).
		Set("Status", status).
		WithExpiredAt(time.Now().UTC().Add(-time.Hour)).
		Exec()
	if err != nil {
		fail("seed %s/%s: %v", swamp.Get(), key, err)
	}
}

func fail(format string, args ...any) {
	log.Printf("FAIL: "+format, args...)
	os.Exit(1)
}
