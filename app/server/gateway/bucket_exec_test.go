package gateway

import (
	"testing"
	"time"

	hydra "github.com/hydraide/hydraide/app/core/hydra/swamp"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
)

// newTreasureCreatedAt builds a minimal Treasure with key + CreatedAt.
// Used only for time-range filtering tests; content is irrelevant.
func newTreasureCreatedAt(key string, createdAt time.Time) treasure.Treasure {
	tr := treasure.New(noopSave)
	gid := tr.StartTreasureGuard(true)
	tr.BodySetKey(gid, key)
	tr.SetCreatedAt(gid, createdAt)
	tr.ReleaseTreasureGuard(gid)
	return tr
}

// TestApplyTimeRange_ToTimeExclusive pins the bucket-routed path to the
// documented semantics for Index.ToTime: exclusive upper bound. A record
// whose CreatedAt equals ToTime must be dropped, otherwise cursor-style
// pagination (next-page ToTime = previous-page last CreatedAt) returns
// the boundary record twice. See docs/bugs/2026-05-15-bucket-routed-totime-inclusive.md.
func TestApplyTimeRange_ToTimeExclusive(t *testing.T) {
	base := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	t0 := base
	t1 := base.Add(1 * time.Hour)
	t2 := base.Add(2 * time.Hour)

	cands := []treasure.Treasure{
		newTreasureCreatedAt("k0", t0),
		newTreasureCreatedAt("k1", t1),
		newTreasureCreatedAt("k2", t2),
	}

	// ToTime = t2: must drop the t2 record (exclusive upper bound).
	out := applyTimeRange(cands, hydra.BeaconTypeCreationTime, nil, &t2)
	if len(out) != 2 {
		t.Fatalf("ToTime exclusive: want 2 results, got %d", len(out))
	}
	for _, tr := range out {
		if tr.GetCreatedAt() == t2.UnixNano() {
			t.Fatalf("ToTime exclusive: boundary record (ts == ToTime) must be dropped, got key=%s", tr.GetKey())
		}
	}
}

// TestApplyTimeRange_FromTimeInclusive pins the inclusive lower bound:
// a record whose CreatedAt equals FromTime must be kept.
func TestApplyTimeRange_FromTimeInclusive(t *testing.T) {
	base := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	t0 := base
	t1 := base.Add(1 * time.Hour)

	cands := []treasure.Treasure{
		newTreasureCreatedAt("k0", t0),
		newTreasureCreatedAt("k1", t1),
	}

	// FromTime = t0: must keep the t0 record (inclusive lower bound).
	out := applyTimeRange(cands, hydra.BeaconTypeCreationTime, &t0, nil)
	if len(out) != 2 {
		t.Fatalf("FromTime inclusive: want 2 results, got %d", len(out))
	}
}
