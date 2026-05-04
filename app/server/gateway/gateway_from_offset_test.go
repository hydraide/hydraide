package gateway

import (
	"fmt"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/filesystem"
	"github.com/hydraide/hydraide/app/core/hydra/swamp"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/metadata"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/hydraide/hydraide/app/core/settings"
	"github.com/hydraide/hydraide/app/name"
	hydrapb "github.com/hydraide/hydraide/generated/hydraidepbgo"
)

// TestGateway_FromOffset_WithFilter reproduces the bug-report scenario where
// CatalogReadManyStream(Index.From > 0, filters != nil) allegedly returns 0
// items. The test bypasses gRPC and the SDK, building a real swamp +
// gateway-equivalent loop. Each case calls the real
// swamp.GetTreasuresByBeacon and then runs the exact filter+MaxResults loop
// that gateway.GetByIndexStream uses.
//
// If this test passes, the bug is NOT in the swamp/beacon/gateway code path —
// it must live in the SDK, the proto wire layer, or the caller.
func TestGateway_FromOffset_WithFilter(t *testing.T) {
	const total = 100

	fsInterface := filesystem.New()
	settingsInterface := settings.New(3, 2000)
	settingsInterface.RegisterPattern(
		name.New().Sanctuary("gw-test").Realm("*").Swamp("*"),
		false, 1,
		&settings.FileSystemSettings{WriteIntervalSec: 1, MaxFileSizeByte: 8192},
	)

	swampName := name.New().Sanctuary("gw-test").Realm("from-offset").Swamp("filter")
	hashPath := swampName.GetFullHashPath(
		settingsInterface.GetHydraAbsDataFolderPath(),
		100, 3, 2000,
	)
	chroniclerInterface := chronicler.New(hashPath, int64(8192), 3, fsInterface, metadata.New(hashPath))
	chroniclerInterface.CreateDirectoryIfNotExists()

	fssSwamp := &swamp.FilesystemSettings{
		ChroniclerInterface: chroniclerInterface,
		WriteInterval:       1 * time.Second,
	}
	metadataInterface := metadata.New(hashPath)
	swampInterface := swamp.New(
		swampName, 1*time.Second, fssSwamp,
		func(e *swamp.Event) {}, func(i *swamp.Info) {}, func(n name.Name) {},
		metadataInterface,
	)
	swampInterface.BeginVigil()
	defer func() {
		swampInterface.CeaseVigil()
		swampInterface.Destroy()
	}()

	// Seed: 100 treasures, each carrying a msgpack-encoded {IsIndexed: true, Slot: i}
	// payload. CreatedAt is monotonically increasing so creation-time order is stable.
	now := time.Now()
	for i := 0; i < total; i++ {
		tr := swampInterface.CreateTreasure(fmt.Sprintf("k-%03d", i))
		gid := tr.StartTreasureGuard(true)
		tr.SetCreatedAt(gid, now.Add(time.Duration(i)*time.Millisecond))
		tr.SetModifiedAt(gid, now.Add(time.Duration(i)*time.Millisecond))
		tr.SetContentByteArray(gid, makeMsgpackBytesVal(t, map[string]interface{}{
			"IsIndexed": true,
			"Slot":      int64(i),
		}))
		tr.ReleaseTreasureGuard(gid)

		gid = tr.StartTreasureGuard(true)
		_ = tr.Save(gid)
		tr.ReleaseTreasureGuard(gid)
	}

	// Sanity: from=0 + filter -> all 100 match.
	allFiltered := runGatewayLoop(t, swampInterface, 0, 0, isIndexedTrueFilter(), 0)
	if len(allFiltered) != total {
		t.Fatalf("baseline (from=0, filter on, no max): want %d, got %d", total, len(allFiltered))
	}

	// === Reproduction of the bug-report's case matrix ===
	// Filter is `IsIndexed == true` (matches all). MaxResults=5.
	cases := []struct {
		name string
		from int32
		want int
	}{
		{"from=0", 0, 5},
		{"from=5", 5, 5},
		{"from=20", 20, 5},
		{"from=50", 50, 5},
		{"from=95", 95, 5}, // exactly enough left to fill MaxResults
		{"from=98", 98, 2}, // only 2 left → 2 streamed
		{"from=100", 100, 0},
	}

	for _, c := range cases {
		t.Run("desc/limit=0/"+c.name, func(t *testing.T) {
			got := runGatewayLoop(t, swampInterface, c.from, 0, isIndexedTrueFilter(), 5)
			if len(got) != c.want {
				t.Errorf("From=%d Limit=0 MaxResults=5: want %d streamed, got %d",
					c.from, c.want, len(got))
			}
		})
	}

	// Also verify with a real-world filter that REJECTS some records — the
	// stream-loop has to skip non-matching records *after* the From offset.
	// Half the records (Slot odd) get IsIndexed=false retroactively.
	for i := 1; i < total; i += 2 {
		tr, err := swampInterface.GetTreasure(fmt.Sprintf("k-%03d", i))
		if err != nil {
			t.Fatalf("GetTreasure: %v", err)
		}
		gid := tr.StartTreasureGuard(true)
		tr.SetContentByteArray(gid, makeMsgpackBytesVal(t, map[string]interface{}{
			"IsIndexed": false,
			"Slot":      int64(i),
		}))
		tr.ReleaseTreasureGuard(gid)
		gid = tr.StartTreasureGuard(true)
		_ = tr.Save(gid)
		tr.ReleaseTreasureGuard(gid)
	}

	// ASC order: index 0 (Slot 0, true), 1 (false), 2 (true), …
	// From=0 with MaxResults=5 → first 5 even-slot items: Slot 0,2,4,6,8
	// From=5 with MaxResults=5 → skip first 5 records (slots 0..4), then collect
	//   matching: slots 6,8,10,12,14
	t.Run("asc/limit=0/from=5/half-match", func(t *testing.T) {
		got := runGatewayLoopOrder(t, swampInterface, swamp.IndexOrderAsc, 5, 0, isIndexedTrueFilter(), 5)
		if len(got) != 5 {
			t.Fatalf("From=5 (ASC, half-match): want 5, got %d", len(got))
		}
		wantKeys := []string{"k-006", "k-008", "k-010", "k-012", "k-014"}
		for i, k := range wantKeys {
			if got[i].Key != k {
				t.Errorf("From=5 ASC: position %d: want key %s, got %s", i, k, got[i].Key)
			}
		}
	})
}

// runGatewayLoop replays gateway.GetByIndexStream's body without the
// gRPC stream. Returns the list of treasures that would have been streamed.
func runGatewayLoop(
	t *testing.T,
	s swamp.Swamp,
	from, limit int32,
	filters *hydrapb.FilterGroup,
	maxResults int32,
) []*hydrapb.Treasure {
	return runGatewayLoopOrder(t, s, swamp.IndexOrderDesc, from, limit, filters, maxResults)
}

func runGatewayLoopOrder(
	t *testing.T,
	s swamp.Swamp,
	order swamp.BeaconOrder,
	from, limit int32,
	filters *hydrapb.FilterGroup,
	maxResults int32,
) []*hydrapb.Treasure {
	t.Helper()

	treasures, err := s.GetTreasuresByBeacon(
		swamp.BeaconTypeCreationTime, order,
		from, limit, nil, nil,
	)
	if err != nil {
		t.Fatalf("GetTreasuresByBeacon: %v", err)
	}

	var streamed []*hydrapb.Treasure
	var matchCount int32
	for _, tr := range treasures {
		if filters != nil && !evaluateNativeFilterGroup(tr, filters) {
			continue
		}
		out := &hydrapb.Treasure{}
		treasureToKeyValuePair(tr, out)
		streamed = append(streamed, out)
		matchCount++
		if maxResults > 0 && matchCount >= maxResults {
			break
		}
	}
	return streamed
}

// isIndexedTrueFilter mirrors the bug-report's filter:
//
//	hydraidego.FilterAND(
//	    hydraidego.FilterBytesFieldBool(hydraidego.Equal, "IsIndexed", true),
//	)
func isIndexedTrueFilter() *hydrapb.FilterGroup {
	path := "IsIndexed"
	return &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{{
			Operator:       hydrapb.Relational_EQUAL,
			BytesFieldPath: &path,
			CompareValue:   &hydrapb.TreasureFilter_BoolVal{BoolVal: hydrapb.Boolean_TRUE},
		}},
	}
}

var _ = treasure.New // keep import used if helper imports drift
