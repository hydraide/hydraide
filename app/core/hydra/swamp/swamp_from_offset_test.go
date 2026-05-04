package swamp

import (
	"fmt"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/filesystem"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/metadata"
	"github.com/hydraide/hydraide/app/core/settings"
	"github.com/hydraide/hydraide/app/name"
	"github.com/stretchr/testify/assert"
)

// TestSwamp_GetTreasuresByBeacon_FromOffset isolates the question raised in the
// bug report: does the swamp/beacon layer correctly honour From > 0 for
// pagination? This test does NOT exercise the gateway, filters or the SDK —
// only the in-process swamp layer. If this test fails, the regression is below
// the gateway. If it passes, the bug must live in the gateway, filter eval, or
// the SDK.
func TestSwamp_GetTreasuresByBeacon_FromOffset(t *testing.T) {
	const total = 100

	fsInterface := filesystem.New()
	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)
	fss := &settings.FileSystemSettings{
		WriteIntervalSec: 1,
		MaxFileSizeByte:  8192,
	}
	settingsInterface.RegisterPattern(
		name.New().Sanctuary(sanctuaryForQuickTest).Realm("*").Swamp("*"),
		false, 1, fss,
	)
	closeAfterIdle := 1 * time.Second
	writeInterval := 1 * time.Second
	maxFileSize := int64(8192)

	swampName := name.New().
		Sanctuary(sanctuaryForQuickTest).
		Realm("from-offset").
		Swamp("creation-time")

	hashPath := swampName.GetFullHashPath(
		settingsInterface.GetHydraAbsDataFolderPath(),
		testAllServers, testMaxDepth, testMaxFolderPerLevel,
	)
	chroniclerInterface := chronicler.New(hashPath, maxFileSize, testMaxDepth, fsInterface, metadata.New(hashPath))
	chroniclerInterface.CreateDirectoryIfNotExists()

	fssSwamp := &FilesystemSettings{
		ChroniclerInterface: chroniclerInterface,
		WriteInterval:       writeInterval,
	}

	metadataInterface := metadata.New(hashPath)
	swampInterface := New(
		swampName, closeAfterIdle, fssSwamp,
		func(e *Event) {}, func(i *Info) {}, func(n name.Name) {},
		metadataInterface,
	)
	swampInterface.BeginVigil()
	defer func() {
		swampInterface.CeaseVigil()
		swampInterface.Destroy()
	}()

	// Seed: 100 treasures with monotonically increasing CreatedAt.
	now := time.Now()
	for i := 0; i < total; i++ {
		tr := swampInterface.CreateTreasure(fmt.Sprintf("k-%03d", i))
		guardID := tr.StartTreasureGuard(true)
		tr.SetCreatedAt(guardID, now.Add(time.Duration(i)*time.Millisecond))
		tr.SetModifiedAt(guardID, now.Add(time.Duration(i)*time.Millisecond))
		tr.SetContentString(guardID, fmt.Sprintf("v-%d", i))
		tr.ReleaseTreasureGuard(guardID)

		guardID = tr.StartTreasureGuard(true)
		_ = tr.Save(guardID)
		tr.ReleaseTreasureGuard(guardID)
	}

	// Sanity check: all 100 are visible with From=0, Limit=0.
	all, err := swampInterface.GetTreasuresByBeacon(BeaconTypeCreationTime, IndexOrderDesc, 0, 0, nil, nil)
	assert.Nil(t, err)
	assert.Equal(t, total, len(all), "From=0/Limit=0 must return all treasures")

	// Case matrix — replicates the bug-report's From values.
	cases := []struct {
		name      string
		from      int32
		limit     int32
		wantCount int
	}{
		// Limit=0 → swamp expands to Count() internally; expect (total - from) items.
		{"from=0,limit=0", 0, 0, total},
		{"from=5,limit=0", 5, 0, total - 5},
		{"from=20,limit=0", 20, 0, total - 20},
		{"from=50,limit=0", 50, 0, total - 50},
		{"from=99,limit=0", 99, 0, 1},
		{"from=100,limit=0", 100, 0, 0},

		// Limit>0 → window of Limit items starting at From.
		{"from=0,limit=5", 0, 5, 5},
		{"from=5,limit=5", 5, 5, 5},
		{"from=20,limit=5", 20, 5, 5},
		{"from=50,limit=5", 50, 5, 5},
		{"from=98,limit=5", 98, 5, 2}, // only 2 left
	}

	for _, c := range cases {
		t.Run("desc/"+c.name, func(t *testing.T) {
			got, err := swampInterface.GetTreasuresByBeacon(
				BeaconTypeCreationTime, IndexOrderDesc,
				c.from, c.limit, nil, nil,
			)
			assert.Nil(t, err)
			assert.Equal(t, c.wantCount, len(got),
				"From=%d Limit=%d (DESC): unexpected slice length", c.from, c.limit)
		})
		t.Run("asc/"+c.name, func(t *testing.T) {
			got, err := swampInterface.GetTreasuresByBeacon(
				BeaconTypeCreationTime, IndexOrderAsc,
				c.from, c.limit, nil, nil,
			)
			assert.Nil(t, err)
			assert.Equal(t, c.wantCount, len(got),
				"From=%d Limit=%d (ASC): unexpected slice length", c.from, c.limit)
		})
	}

	// Same matrix on the key beacon — the bug report mentions IndexKey is also affected.
	for _, c := range cases {
		t.Run("key-desc/"+c.name, func(t *testing.T) {
			got, err := swampInterface.GetTreasuresByBeacon(
				BeaconTypeKey, IndexOrderDesc,
				c.from, c.limit, nil, nil,
			)
			assert.Nil(t, err)
			assert.Equal(t, c.wantCount, len(got),
				"From=%d Limit=%d (KEY/DESC): unexpected slice length", c.from, c.limit)
		})
	}
}
