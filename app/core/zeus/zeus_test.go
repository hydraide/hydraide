package zeus

import (
	"context"
	"fmt"
	"github.com/hydraide/hydraide/app/core/filesystem"
	"github.com/hydraide/hydraide/app/core/hydra/swamp"
	"github.com/hydraide/hydraide/app/core/settings"
	"github.com/hydraide/hydraide/app/name"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	maxDepthOfFolders     = 3
	maxFoldersPerLevel    = 10000
	sanctuaryForQuickTest = "hydraquicktest"
)

func TestZeus_StartHydra(t *testing.T) {

	settingsInterface := settings.New(maxDepthOfFolders, maxFoldersPerLevel)
	fsInterface := filesystem.New()

	t.Run("test", func(t *testing.T) {

		zeusInterface := New(settingsInterface, fsInterface)
		zeusInterface.StartHydra()

		hydraInterface := zeusInterface.GetHydra()

		require.NotNil(t, hydraInterface)

		swampObject, err := hydraInterface.SummonSwamp(context.Background(), 10, name.New().Swamp("testSwamp"))
		defer swampObject.Destroy()

		swampObject.BeginVigil()
		require.NoError(t, err)
		require.NotNil(t, swampObject)

		// const content = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Mauris varius, ante sit amet placerat iaculis, quam metus congue nibh, ac pellentesque ex augue eu tellus"

		// insert treasures
		start := time.Now()
		for i := 1; i <= 10; i++ {
			treasureObj := swampObject.CreateTreasure(fmt.Sprintf("%d", i))
			treasureGuardID := treasureObj.StartTreasureGuard(true)
			treasureObj.SetContentInt64(treasureGuardID, int64(i))
			treasureObj.Save(treasureGuardID)
			treasureObj.ReleaseTreasureGuard(treasureGuardID)
		}
		end := time.Now()

		// calculate the elapsed time
		elapsed := end.Sub(start)

		fmt.Println("insert treasures elapsed time: ", elapsed)

		// get treasures from the beacon
		treasures, err := swampObject.GetTreasuresByBeacon(swamp.BeaconTypeValueInt64, swamp.IndexOrderAsc, 0, 3)
		require.NoError(t, err)

		for _, treasure := range treasures {
			c, cErr := treasure.GetContentInt64()
			require.NoError(t, cErr)
			fmt.Printf("treasure: %s \t %d \n", treasure.GetKey(), c)
		}

		swampObject.CeaseVigil()

		time.Sleep(10 * time.Second)

		zeusInterface.StopHydra()

		fmt.Println("hydra stopped successfully")

	})

	t.Run("settings test", func(t *testing.T) {

		allTests := 10
		settingsInterface.RegisterPattern(name.New().Sanctuary(sanctuaryForQuickTest).Realm("*").Swamp("*"), false, 3600, &settings.FileSystemSettings{
			WriteIntervalSec: 1,
			MaxFileSizeByte:  8192, // 8KB
		})

		zeusInterface := New(settingsInterface, fsInterface)
		zeusInterface.StartHydra()

		hydraInterface := zeusInterface.GetHydra()

		require.NotNil(t, hydraInterface)

		swampObject, err := hydraInterface.SummonSwamp(context.Background(), 10, name.New().Sanctuary(sanctuaryForQuickTest).Realm("user").Swamp("petergebri"))
		defer swampObject.Destroy()

		swampObject.BeginVigil()
		require.NoError(t, err)
		require.NotNil(t, swampObject)

		// insert treasures to the swamp
		for i := 1; i <= allTests; i++ {
			treasureObj := swampObject.CreateTreasure(fmt.Sprintf("%d", i))
			treasureGuardID := treasureObj.StartTreasureGuard(true)
			treasureObj.SetContentInt64(treasureGuardID, int64(i))
			treasureObj.Save(treasureGuardID)
			treasureObj.ReleaseTreasureGuard(treasureGuardID)
		}

		// get treasures from the beacon
		require.Equal(t, allTests, swampObject.CountTreasures())

		swampObject.CeaseVigil()

		time.Sleep(10 * time.Second)

		zeusInterface.StopHydra()

		fmt.Println("hydra stopped successfully")

	})

}

// pkg: github.com/hydraide/hydraide/app/core/zeus
// cpu: AMD Ryzen Threadripper 2950X 16-Core Processor
// BenchmarkNew-32    	  623088	      2486 ns/op
// PASS
func BenchmarkNew(b *testing.B) {

	settingsInterface := settings.New(maxDepthOfFolders, maxFoldersPerLevel)
	fsInterface := filesystem.New()

	settingsInterface.RegisterPattern(name.New().Sanctuary(sanctuaryForQuickTest).Realm("*").Swamp("*"), false, 10, &settings.FileSystemSettings{
		WriteIntervalSec: 10,
		MaxFileSizeByte:  8192, // 8KB
	})

	zeusInterface := New(settingsInterface, fsInterface)
	zeusInterface.StartHydra()

	hydraInterface := zeusInterface.GetHydra()

	require.NotNil(b, hydraInterface)

	swampObject, err := hydraInterface.SummonSwamp(context.Background(), 10, name.New().Sanctuary(sanctuaryForQuickTest).Realm("user").Swamp("petergebri"))
	defer swampObject.Destroy()

	if err != nil {
		b.Fatal(err)
	}
	swampObject.BeginVigil()

	require.NoError(b, err)
	require.NotNil(b, swampObject)

	const content = "trendizz.com"

	b.ResetTimer()

	for i := 1; i <= b.N; i++ {
		treasureObj := swampObject.CreateTreasure(strconv.Itoa(i))
		treasureGuardID := treasureObj.StartTreasureGuard(true)
		treasureObj.SetContentString(treasureGuardID, content)
		treasureObj.Save(treasureGuardID)
		treasureObj.ReleaseTreasureGuard(treasureGuardID)
	}

	swampObject.CeaseVigil()

}

func BenchmarkRead(b *testing.B) {
	settingsInterface := settings.New(maxDepthOfFolders, maxFoldersPerLevel)
	fsInterface := filesystem.New()

	settingsInterface.RegisterPattern(name.New().Sanctuary(sanctuaryForQuickTest).Realm("*").Swamp("*"), false, 10, &settings.FileSystemSettings{
		WriteIntervalSec: 10,
		MaxFileSizeByte:  8192, // 8KB
	})

	zeusInterface := New(settingsInterface, fsInterface)
	zeusInterface.StartHydra()

	hydraInterface := zeusInterface.GetHydra()
	require.NotNil(b, hydraInterface)

	swampObject, err := hydraInterface.SummonSwamp(context.Background(), 10, name.New().Sanctuary(sanctuaryForQuickTest).Realm("user").Swamp("petergebri"))
	defer swampObject.Destroy()

	require.NoError(b, err)
	require.NotNil(b, swampObject)
	swampObject.BeginVigil()

	// --- Előkészítés: írjunk be b.N darab Treasure-t ---
	const content = "trendizz.com"
	for i := 1; i <= b.N; i++ {
		treasureObj := swampObject.CreateTreasure(strconv.Itoa(i))
		treasureGuardID := treasureObj.StartTreasureGuard(true)
		treasureObj.SetContentString(treasureGuardID, content)
		treasureObj.Save(treasureGuardID)
		treasureObj.ReleaseTreasureGuard(treasureGuardID)
	}

	b.ResetTimer()

	// --- Mérési szakasz: olvasás ---
	for i := 1; i <= b.N; i++ {
		key := strconv.Itoa(i)
		treasure, err := swampObject.GetTreasure(key)
		if err != nil {
			b.Fatalf("failed to read treasure %s: %v", key, err)
		}
		_, err = treasure.GetContentString()
		if err != nil {
			b.Fatalf("failed to read content from treasure %s: %v", key, err)
		}
	}

	swampObject.CeaseVigil()
}
