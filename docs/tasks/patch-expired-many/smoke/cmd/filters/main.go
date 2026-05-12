// filters is the regression smoke for the two PatchExpired bugs fixed
// in this branch:
//
//  1. HowMany == 0 means "all currently-expired matching Filters"
//     (mirrors ShiftExpiredTreasures). Pre-fix, HowMany == 0 returned
//     an empty result.
//
//  2. Filters on PatchExpiredTreasuresRequest scope the candidate set
//     BEFORE HowMany / Cap budget arithmetic — symmetric to
//     ShiftMatchingTreasures.Filters. Pre-fix, the only way to
//     sub-scope PatchExpired was the per-key Condition, which still
//     consumed HowMany / Cap budget per record.
//
// The four test cases mirror the original bug report's Test 1–4 plus
// two Filters cases that the original implementation could not express.
//
// Run with the in-tree dev compose stack (docs/sdk/go/examples) on
// localhost:5980 — HYDRAIDE_HOST / cert env vars override.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hydraide/hydraide/docs/tasks/patch-expired-many/smoke/internal/setup"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name"
)

type entry struct {
	Domain    string    `hydraide:"key"      msgpack:"-"`
	ExpireAt  time.Time `hydraide:"expireAt" msgpack:"-"`
	ASN       string    `hydraide:"-"        msgpack:"ASN"`
	ClaimedBy string    `hydraide:"-"        msgpack:"ClaimedBy"`
}

func seed(ctx context.Context, h hydraidego.Hydraidego, swamp name.Name) {
	past := time.Now().UTC().Add(-2 * time.Minute)
	for i := 0; i < 100; i++ {
		for _, asn := range []string{"AS-X", "AS-Y"} {
			_, err := h.CatalogPatch(ctx, swamp, fmt.Sprintf("%s-%03d", asn, i)).
				Set("ASN", asn).
				Set("ClaimedBy", "").
				WithExpiredAt(past).
				Exec()
			if err != nil {
				fail("seed: %v", err)
			}
		}
	}
}

func count(ctx context.Context, h hydraidego.Hydraidego, swamp name.Name, howMany int32, ops *hydraidego.PatchExpiredOps) (patched, condFail int, capReached bool) {
	res, err := h.CatalogPatchExpiredWithResult(ctx, swamp, howMany, entry{},
		func(_ any, st hydraidego.PatchStatus) error {
			switch st {
			case hydraidego.PatchStatusPatched:
				patched++
			default:
				condFail++
			}
			return nil
		}, ops)
	if err != nil {
		fail("PatchExpired: %v", err)
	}
	if res != nil {
		capReached = res.CapReached
	}
	return
}

func main() {
	h := setup.Connect()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// ------------------------------------------------------------------
	// Test 1 — HowMany == 0 means "all currently-expired".
	// Pre-fix: patched=0. Post-fix: patched=200.
	// ------------------------------------------------------------------
	swamp1, td1 := setup.FreshSwamp(h, "filters-t1")
	defer td1()
	seed(ctx, h, swamp1)

	ops1 := hydraidego.NewPatchExpiredOps().
		Set("ClaimedBy", "w").
		WithExpiredAt(time.Now().UTC().Add(24 * time.Hour))
	p, cf, cr := count(ctx, h, swamp1, 0, ops1)
	expect("Test 1", p == 200 && cf == 0 && !cr,
		fmt.Sprintf("patched=%d condFail=%d capReached=%v (want 200/0/false)", p, cf, cr))

	// ------------------------------------------------------------------
	// Test 2 — HowMany == 200 baseline.
	// ------------------------------------------------------------------
	swamp2, td2 := setup.FreshSwamp(h, "filters-t2")
	defer td2()
	seed(ctx, h, swamp2)

	ops2 := hydraidego.NewPatchExpiredOps().
		Set("ClaimedBy", "w").
		WithExpiredAt(time.Now().UTC().Add(24 * time.Hour))
	p, cf, cr = count(ctx, h, swamp2, 200, ops2)
	expect("Test 2", p == 200 && cf == 0,
		fmt.Sprintf("patched=%d condFail=%d (want 200/0)", p, cf))

	// ------------------------------------------------------------------
	// Test 3 — Cap (no scope) bounds the patched count.
	// ------------------------------------------------------------------
	swamp3, td3 := setup.FreshSwamp(h, "filters-t3")
	defer td3()
	seed(ctx, h, swamp3)

	ops3 := hydraidego.NewPatchExpiredOps().
		Set("ClaimedBy", "w").
		WithExpiredAt(time.Now().UTC().Add(24 * time.Hour)).
		WithCap(&hydraidego.Cap{
			Filter: hydraidego.FilterAND(
				hydraidego.FilterBytesFieldString(hydraidego.NotEqual, "ClaimedBy", ""),
				hydraidego.FilterExpiredAt(hydraidego.GreaterThan, time.Now().UTC()),
			),
			MaxMatching: 10,
		})
	p, cf, cr = count(ctx, h, swamp3, 200, ops3)
	expect("Test 3", p == 10 && cf == 0 && cr,
		fmt.Sprintf("patched=%d condFail=%d capReached=%v (want 10/0/true)", p, cf, cr))

	// ------------------------------------------------------------------
	// Test 4 — Filters scope to ASN==X; without a Cap, all 100 X records
	// patch and no Y record is touched. Pre-fix, the only way to scope
	// was IfFieldEquals which still walked the full expired index and
	// reported CONDITION_NOT_MET for Y records.
	// ------------------------------------------------------------------
	swamp4, td4 := setup.FreshSwamp(h, "filters-t4")
	defer td4()
	seed(ctx, h, swamp4)

	ops4 := hydraidego.NewPatchExpiredOps().
		Set("ClaimedBy", "w").
		WithExpiredAt(time.Now().UTC().Add(24 * time.Hour)).
		WithFilters(
			hydraidego.FilterAND(
				hydraidego.FilterBytesFieldString(hydraidego.Equal, "ASN", "AS-X"),
			),
		)
	p, cf, cr = count(ctx, h, swamp4, 0, ops4)
	expect("Test 4", p == 100 && cf == 0 && !cr,
		fmt.Sprintf("patched=%d condFail=%d capReached=%v (want 100/0/false)", p, cf, cr))

	// ------------------------------------------------------------------
	// Test 5 — Filters + Cap: per-ASN bounded claim. 100 ASN==X
	// candidates, MaxMatching=10. Only ASN==X records consume the
	// budget; ASN==Y is excluded by Filters and never touched.
	// ------------------------------------------------------------------
	swamp5, td5 := setup.FreshSwamp(h, "filters-t5")
	defer td5()
	seed(ctx, h, swamp5)

	ops5 := hydraidego.NewPatchExpiredOps().
		Set("ClaimedBy", "w").
		WithExpiredAt(time.Now().UTC().Add(24 * time.Hour)).
		WithFilters(
			hydraidego.FilterAND(
				hydraidego.FilterBytesFieldString(hydraidego.Equal, "ASN", "AS-X"),
			),
		).
		WithCap(&hydraidego.Cap{
			Filter: hydraidego.FilterAND(
				hydraidego.FilterBytesFieldString(hydraidego.Equal, "ASN", "AS-X"),
				hydraidego.FilterBytesFieldString(hydraidego.NotEqual, "ClaimedBy", ""),
				hydraidego.FilterExpiredAt(hydraidego.GreaterThan, time.Now().UTC()),
			),
			MaxMatching: 10,
		})
	p, cf, cr = count(ctx, h, swamp5, 0, ops5)
	expect("Test 5", p == 10 && cf == 0 && cr,
		fmt.Sprintf("patched=%d condFail=%d capReached=%v (want 10/0/true)", p, cf, cr))

	fmt.Println("PASS: filters")
}

func expect(label string, ok bool, detail string) {
	if !ok {
		fail("%s: %s", label, detail)
	}
	fmt.Printf("OK   %s — %s\n", label, detail)
}

func fail(format string, args ...any) {
	log.Printf("FAIL: "+format, args...)
	os.Exit(1)
}
