package hydra

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hydraide/hydraide/app/core/filesystem"
	"github.com/hydraide/hydraide/app/core/hydra/lock"
	"github.com/hydraide/hydraide/app/core/hydra/swamp"
	"github.com/hydraide/hydraide/app/core/safeops"
	"github.com/hydraide/hydraide/app/core/settings"
	"github.com/hydraide/hydraide/app/name"
	"github.com/stretchr/testify/assert"
)

const (
	// testServerNumber      = 100
	testMaxDepth          = 3
	testMaxFolderPerLevel = 2000
	sanctuaryForQuickTest = "hydraquicktest"
)

func TestHydra_SummonSwamp(t *testing.T) {

	elysiumInterface := safeops.New()

	lockerInterface := lock.New()

	fsInterface := filesystem.New()
	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)
	// gyors filementés és bezárás a tesztekhez
	fss := &settings.FileSystemSettings{
		WriteIntervalSec: 1,
		MaxFileSizeByte:  8192,
	}

	settingsInterface.RegisterPattern(name.New().Sanctuary(sanctuaryForQuickTest).Realm("*").Swamp("*"), false, 1, fss)
	hydraInterface := New(settingsInterface, elysiumInterface, lockerInterface, fsInterface)

	t.Run("should summon a non existing swamp", func(t *testing.T) {

		newSwampName := name.New().Sanctuary("summon").Realm("non-existing").Swamp("swamp")
		swampInterface, _ := hydraInterface.SummonSwamp(context.Background(), 10, newSwampName)

		assert.NotNil(t, swampInterface, "should not be nil")
		assert.Equal(t, newSwampName, swampInterface.GetName(), "should be equal")

		swampInterface.Destroy()

	})

	t.Run("should summon an existing swamp", func(t *testing.T) {

		newSwampName := name.New().Sanctuary("summon").Realm("existing").Swamp("swamp")

		// summon a swamp at the first time
		swampInterface, _ := hydraInterface.SummonSwamp(context.Background(), 10, newSwampName)

		assert.NotNil(t, swampInterface, "should not be nil")
		assert.Equal(t, newSwampName, swampInterface.GetName(), "should be equal")

		// summon the swamp again
		swampInterface, _ = hydraInterface.SummonSwamp(context.Background(), 10, newSwampName)
		assert.NotNil(t, swampInterface, "should not be nil")
		assert.Equal(t, newSwampName, swampInterface.GetName(), "should be equal")

		// destory the swamp after the test
		swampInterface.Destroy()

	})

	t.Run("should exists the swamp", func(t *testing.T) {

		newSwampName := name.New().Sanctuary("summon").Realm("is-existing").Swamp("swamp")

		swampInterface, _ := hydraInterface.SummonSwamp(context.Background(), 10, newSwampName)

		assert.NotNil(t, swampInterface, "should not be nil")
		assert.Equal(t, newSwampName, swampInterface.GetName(), "should be equal")

		isExists, _ := hydraInterface.IsExistSwamp(10, newSwampName)

		assert.True(t, isExists, "should be true")

		// töröljük a swmpot tesztelés után
		swampInterface.Destroy()

	})

	t.Run("should not exists the swamp", func(t *testing.T) {

		newSwampName := name.New().Sanctuary("summon").Realm("should-not-exist").Swamp("swamp")

		isExists, _ := hydraInterface.IsExistSwamp(10, newSwampName)

		assert.False(t, isExists, "should be false")

	})

	t.Run("should list all active swamps", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping test with 7s sleep in short mode")
		}

		allActiveSwamps := hydraInterface.ListActiveSwamps()
		assert.Equal(t, 0, len(allActiveSwamps), "should be equal")

		var testSwampNames []name.Name
		allTests := 10
		for i := 0; i < allTests; i++ {
			testSwampNames = append(testSwampNames, name.New().Sanctuary("test").Realm("active-swamp-list").Swamp(fmt.Sprintf("swamp-%d", i)))
		}

		for i := 0; i < allTests; i++ {
			_, _ = hydraInterface.SummonSwamp(context.Background(), 10, testSwampNames[i])
		}

		allActiveSwamps = hydraInterface.ListActiveSwamps()
		assert.Equal(t, allTests, len(allActiveSwamps), "should be equal")

		// wait for 7 seconds to close all swamps because of the swamp's default timeout is 5 seconds without any activity
		time.Sleep(7 * time.Second)

		allActiveSwamps = hydraInterface.ListActiveSwamps()
		assert.Equal(t, 0, len(allActiveSwamps), "should be equal")

		// destroy test swamps
		for i := 0; i < allTests; i++ {
			swampInterface, _ := hydraInterface.SummonSwamp(context.Background(), 10, testSwampNames[i])
			swampInterface.Destroy()
		}

	})

	t.Run("should create treasure with same key", func(t *testing.T) {

		swampInterface, err := hydraInterface.SummonSwamp(context.Background(), 10, name.New().Sanctuary(sanctuaryForQuickTest).Realm("treasure-with").Swamp("same-key"))
		assert.Nil(t, err, "should be nil")

		allTests := 10

		wg := sync.WaitGroup{}
		wg.Add(allTests)

		swampInterface.BeginVigil()
		for i := 0; i < allTests; i++ {

			go func(counter int) {
				treasureInterface := swampInterface.CreateTreasure("same-key")
				guardID := treasureInterface.StartTreasureGuard(true)
				treasureInterface.SetContentString(guardID, fmt.Sprintf("my-content-%d", counter))
				treasureInterface.Save(guardID)
				treasureInterface.ReleaseTreasureGuard(guardID)
				wg.Done()
			}(i)

		}
		swampInterface.CeaseVigil()

		wg.Wait()

		// ellenőrizzük és csak 1 treasure-t kellene találnunk
		allTreasures := swampInterface.CountTreasures()
		if err != nil {
			t.Error(err)
		}
		assert.Equal(t, 1, allTreasures, "should be equal")

		time.Sleep(2 * time.Second)

		// destroy the swamp after the test
		swampInterface.Destroy()

	})

	t.Run("insert words with domains per words", func(t *testing.T) {

		allWords := 100
		allDomainsPerWord := 100

		var words []string
		for i := 0; i < allWords; i++ {
			words = append(words, fmt.Sprintf("word-%d", i))
		}

		var domains []string
		for i := 0; i < allDomainsPerWord; i++ {
			domains = append(domains, fmt.Sprintf("domain-%d.com", i))
		}

		wg := sync.WaitGroup{}
		wg.Add(len(words) * len(domains))

		// run tests with all words
		for _, word := range words {

			go func(workingWord string) {

				swampCtx, cancelFunc := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancelFunc()

				swampInterface, err := hydraInterface.SummonSwamp(swampCtx, 10, name.New().Sanctuary(sanctuaryForQuickTest).Realm("test-words-to-domains").Swamp(workingWord))
				if err != nil {
					slog.Error("error while summoning swamp", "error", err)
					return
				}

				swampInterface.BeginVigil()
				defer swampInterface.CeaseVigil()

				for _, domain := range domains {
					treasureInterface := swampInterface.CreateTreasure(domain)
					guardID := treasureInterface.StartTreasureGuard(true)
					treasureInterface.Save(guardID)
					treasureInterface.ReleaseTreasureGuard(guardID)
					wg.Done()
				}

			}(word)

		}

		wg.Wait()

		// nyissuk be az összes swampot és ellenőrizzük, hogy bekerült-e az összes domain
		for _, word := range words {
			swampInterface, err := hydraInterface.SummonSwamp(context.Background(), 10, name.New().Sanctuary(sanctuaryForQuickTest).Realm("test-words-to-domains").Swamp(word))
			assert.NoError(t, err, "should be nil")
			assert.NotNil(t, swampInterface, "should not be nil")
			assert.Equal(t, allDomainsPerWord, swampInterface.CountTreasures(), "should be equal")
			// ha minden ok, akkor töröljük a swampot, hogy a tesztet követően ne maradjon benn a hydra-ban
			swampInterface.Destroy()
		}

	})

	t.Run("should destroy the swamp after all treasures deleted", func(t *testing.T) {

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("treasure-delete").Swamp("after-all-keys-deleted")

		swampInterface, err := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		assert.Nil(t, err, "should be nil")

		treasure := swampInterface.CreateTreasure("treasure-1")

		guardID := treasure.StartTreasureGuard(true)
		treasure.Save(guardID)
		treasure.ReleaseTreasureGuard(guardID)

		// megszámoljuk a treasure-ket
		allTreasures := swampInterface.CountTreasures()
		assert.Equal(t, 1, allTreasures, "should be equal")

		// töröljük a treasure-t a kulcsa alapján
		err = swampInterface.DeleteTreasure("treasure-1", false)
		assert.Nil(t, err, "should be nil")

		// várunk egy kci kicsit, hogy a hydra törölje a treasure és a swampot is egyaránt
		time.Sleep(100 * time.Millisecond)

		// a swampnak nem szabadna léteznie
		isExists, err := hydraInterface.IsExistSwamp(10, swampName)
		assert.NoError(t, err, "should be nil")
		assert.False(t, isExists, "should be false")

	})

	t.Run("e2e gateway simulation: ShiftExpiredTreasures then Count should see swamp as non-existent", func(t *testing.T) {

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("e2e-gateway-sim").Swamp("shift-then-count")

		// ============================================================
		// PHASE 1: Simulate gateway Set handler — create swamp and add a treasure
		// ============================================================
		swampInterface, err := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		assert.Nil(t, err, "SummonSwamp should succeed")

		swampInterface.BeginVigil()
		treasure := swampInterface.CreateTreasure("msg-1")
		guardID := treasure.StartTreasureGuard(true)
		treasure.SetExpirationTime(guardID, time.Now().Add(-1*time.Second)) // expired 1 second ago
		treasure.SetContentString(guardID, "test message content")
		treasure.Save(guardID)
		treasure.ReleaseTreasureGuard(guardID)
		swampInterface.CeaseVigil()

		t.Logf("PHASE 1: treasure created, CountTreasures=%d", swampInterface.CountTreasures())
		assert.Equal(t, 1, swampInterface.CountTreasures(), "should have 1 treasure after save")

		// Wait for write ticker to flush to disk (writeInterval=1s in test settings)
		time.Sleep(1500 * time.Millisecond)

		// ============================================================
		// PHASE 2: Simulate gateway Count handler — verify treasure exists
		// ============================================================
		// Count handler: checkSwampName(checkExist=true) → SummonSwamp → BeginVigil → CountTreasures → CeaseVigil
		isExists, err := hydraInterface.IsExistSwamp(10, swampName)
		assert.NoError(t, err)
		assert.True(t, isExists, "swamp should exist before delete")

		summonedSwamp, err := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		assert.NoError(t, err)

		summonedSwamp.BeginVigil()
		count := summonedSwamp.CountTreasures()
		summonedSwamp.CeaseVigil()
		t.Logf("PHASE 2: Count before delete: %d", count)
		assert.Equal(t, 1, count, "count should be 1")

		// ============================================================
		// PHASE 3: Simulate gateway ShiftExpiredTreasures handler
		// ============================================================
		// ShiftExpiredTreasures: checkSwampName → SummonSwamp → BeginVigil → CloneAndDeleteExpiredTreasures → defer CeaseVigil
		summonedSwamp2, err := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		assert.NoError(t, err)

		summonedSwamp2.BeginVigil()
		shifted, err := summonedSwamp2.CloneAndDeleteExpiredTreasures(1000000000)
		assert.Nil(t, err, "CloneAndDeleteExpiredTreasures should succeed")
		t.Logf("PHASE 3: shifted=%d, IsClosing=%v", len(shifted), summonedSwamp2.IsClosing())
		assert.Equal(t, 1, len(shifted), "should shift 1 expired treasure")
		// Gateway defer CeaseVigil
		summonedSwamp2.CeaseVigil()

		// Small delay to let destroy finish
		time.Sleep(200 * time.Millisecond)

		// ============================================================
		// PHASE 4: Simulate gateway Count handler AFTER delete
		// ============================================================
		// Count handler: checkSwampName(checkExist=true) → should fail because swamp was destroyed
		isExists, err = hydraInterface.IsExistSwamp(10, swampName)
		t.Logf("PHASE 4: IsExistSwamp=%v, err=%v", isExists, err)
		assert.NoError(t, err, "IsExistSwamp should not error")
		assert.False(t, isExists, "swamp should NOT exist after auto-destroy — this is what the trendizz-api test expects")

		// If the swamp DOES still exist, let's investigate why
		if isExists {
			t.Log("BUG: swamp still exists! Investigating...")
			summonedSwamp3, err := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
			if err == nil {
				summonedSwamp3.BeginVigil()
				t.Logf("  CountTreasures=%d, IsClosing=%v", summonedSwamp3.CountTreasures(), summonedSwamp3.IsClosing())
				summonedSwamp3.CeaseVigil()
				// cleanup
				summonedSwamp3.Destroy()
			} else {
				t.Logf("  SummonSwamp error: %v", err)
			}
		}

	})

	t.Run("e2e gateway simulation: fast path — no write ticker flush before delete", func(t *testing.T) {

		// This simulates the trendizz-api scenario exactly:
		// Save → (only ~200ms) → DeleteAllExpired → Count
		// The write ticker (1s interval) has NOT flushed the treasure to disk yet

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("e2e-gateway-sim").Swamp("fast-no-flush")

		// PHASE 1: Save a treasure (gateway Set handler)
		swampInterface, err := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		assert.Nil(t, err)

		swampInterface.BeginVigil()
		tr := swampInterface.CreateTreasure("msg-1")
		gid := tr.StartTreasureGuard(true)
		tr.SetExpirationTime(gid, time.Now().Add(-1*time.Second)) // already expired
		tr.SetContentString(gid, "test message")
		tr.Save(gid)
		tr.ReleaseTreasureGuard(gid)
		swampInterface.CeaseVigil()

		t.Logf("PHASE 1: saved, CountTreasures=%d, GetFileName=%v", swampInterface.CountTreasures(), tr.GetFileName())

		// NO sleep here — simulating fast path where write ticker hasn't flushed yet
		// Only 200ms like the trendizz-api test
		time.Sleep(200 * time.Millisecond)

		// PHASE 2: Count (gateway Count handler)
		isExists, _ := hydraInterface.IsExistSwamp(10, swampName)
		assert.True(t, isExists, "swamp should exist")
		s2, _ := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		s2.BeginVigil()
		count := s2.CountTreasures()
		s2.CeaseVigil()
		t.Logf("PHASE 2: count=%d", count)
		assert.Equal(t, 1, count)

		// PHASE 3: ShiftExpiredTreasures (gateway handler)
		s3, _ := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		s3.BeginVigil()
		shifted, err := s3.CloneAndDeleteExpiredTreasures(1000000000)
		assert.Nil(t, err)
		t.Logf("PHASE 3: shifted=%d, IsClosing=%v", len(shifted), s3.IsClosing())
		s3.CeaseVigil()

		time.Sleep(200 * time.Millisecond)

		// PHASE 4: Count after delete (gateway Count handler)
		isExists, err = hydraInterface.IsExistSwamp(10, swampName)
		t.Logf("PHASE 4: IsExistSwamp=%v, err=%v", isExists, err)
		assert.NoError(t, err)
		assert.False(t, isExists, "swamp should NOT exist after auto-destroy")

		if isExists {
			t.Log("BUG: swamp still exists after fast-path delete!")
			s4, _ := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
			s4.BeginVigil()
			t.Logf("  CountTreasures=%d, IsClosing=%v", s4.CountTreasures(), s4.IsClosing())
			s4.CeaseVigil()
			s4.Destroy()
		}

	})

	t.Run("e2e gateway simulation: verify .hyd file is not recreated by write ticker after destroy", func(t *testing.T) {

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("e2e-gateway-sim").Swamp("hyd-file-check")

		// PHASE 1: Save a treasure and wait for disk flush
		swampInterface, err := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		assert.Nil(t, err)

		swampInterface.BeginVigil()
		tr := swampInterface.CreateTreasure("msg-1")
		gid := tr.StartTreasureGuard(true)
		tr.SetExpirationTime(gid, time.Now().Add(-1*time.Second))
		tr.SetContentString(gid, "content")
		tr.Save(gid)
		tr.ReleaseTreasureGuard(gid)
		swampInterface.CeaseVigil()

		// Wait for write ticker to flush to disk (writeInterval=1s)
		time.Sleep(1500 * time.Millisecond)

		// Get the .hyd file path from the chronicler (more reliable than manual calculation)
		hydFilePath := swampInterface.GetChronicler().GetSwampAbsPath() + ".hyd"
		_, statErr := os.Stat(hydFilePath)
		if statErr != nil {
			t.Logf("PHASE 1: .hyd file NOT found at %s (may not have flushed yet), skipping file-level checks", hydFilePath)
		} else {
			t.Logf("PHASE 1: .hyd file exists at %s", hydFilePath)
		}
		fileExistedBefore := statErr == nil

		// PHASE 2: ShiftExpiredTreasures (auto-destroy)
		s2, _ := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		s2.BeginVigil()
		shifted, _ := s2.CloneAndDeleteExpiredTreasures(1000000000)
		t.Logf("PHASE 2: shifted=%d, IsClosing=%v", len(shifted), s2.IsClosing())
		s2.CeaseVigil()

		time.Sleep(100 * time.Millisecond)

		// Only check file deletion if the file existed before
		if fileExistedBefore {
			_, statErr = os.Stat(hydFilePath)
			t.Logf("PHASE 2: .hyd file deleted = %v", os.IsNotExist(statErr))
			assert.True(t, os.IsNotExist(statErr), ".hyd file should be deleted after Destroy")

			// PHASE 3: Wait longer to see if write ticker recreates the .hyd file
			time.Sleep(3 * time.Second)
			_, statErr = os.Stat(hydFilePath)
			t.Logf("PHASE 3: .hyd file recreated after 3s = %v", !os.IsNotExist(statErr))
			assert.True(t, os.IsNotExist(statErr), ".hyd file should NOT be recreated by write ticker after Destroy")
		}

		// PHASE 4: Final IsExistSwamp check (always runs)
		isExists, _ := hydraInterface.IsExistSwamp(10, swampName)
		t.Logf("PHASE 4: IsExistSwamp=%v", isExists)
		assert.False(t, isExists, "swamp should not exist after auto-destroy")

		if isExists {
			t.Log("BUG: swamp still exists after auto-destroy!")
			s3, err := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
			if err == nil {
				s3.Destroy()
			}
		}
	})

	t.Run("should create and modify treasure", func(t *testing.T) {

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("treasure-get-and-modify").Swamp("get-and-modify")

		swampInterface, err := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		assert.Nil(t, err, "should be nil")

		treasure := swampInterface.CreateTreasure("treasure-1")

		guardID := treasure.StartTreasureGuard(true)
		treasure.SetContentString(guardID, "content1")
		treasure.Save(guardID)
		treasure.ReleaseTreasureGuard(guardID)

		// megszámoljuk a treasure-ket
		allTreasures := swampInterface.CountTreasures()
		assert.Equal(t, 1, allTreasures, "should be equal")

		// visszaolvassuk a treasure-t
		treasure, err = swampInterface.GetTreasure("treasure-1")
		assert.NotNil(t, treasure, "should not be nil")

		// módosítjuk a contentet
		guardID = treasure.StartTreasureGuard(false)
		treasure.SetContentString(guardID, "content2")
		treasure.Save(guardID)
		treasure.ReleaseTreasureGuard(guardID)

		// visszaolvassuk a treasure-t
		treasure, err = swampInterface.GetTreasure("treasure-1")
		assert.NotNil(t, treasure, "should not be nil")

		content, err := treasure.GetContentString()
		assert.NoError(t, err, "should be nil")
		assert.Equal(t, "content2", content, "should be equal")

		// megvárjuk a kiírásokat is
		time.Sleep(3 * time.Second)

		// beolvassuk a contentet megint kiírást követően is
		treasure, err = swampInterface.GetTreasure("treasure-1")
		assert.NoError(t, err, "should be nil")
		content, err = treasure.GetContentString()
		assert.NoError(t, err, "should be nil")
		assert.Equal(t, "content2", content, "should be equal")

		// töröljük a swampot
		swampInterface.Destroy()

	})

	t.Run("should get treasures from the beacon after deleting some treasures through the hydra", func(t *testing.T) {

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("get-after-delete").Swamp("by-beacon")

		swampInterface, err := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		swampInterface.BeginVigil()

		assert.Nil(t, err, "should be nil")

		defer func() {
			swampInterface.CeaseVigil()
			// destroy the swamp after the test
			swampInterface.Destroy()
		}()

		allTests := 10

		defaultTime := time.Now()
		wg := sync.WaitGroup{}
		wg.Add(allTests)

		for i := 0; i < allTests; i++ {

			go func(counter int) {
				treasureInterface := swampInterface.CreateTreasure(fmt.Sprintf("key-%d", counter))
				guardID := treasureInterface.StartTreasureGuard(true)
				defer treasureInterface.ReleaseTreasureGuard(guardID)
				treasureInterface.SetContentString(guardID, fmt.Sprintf("my-content-%d", counter))
				treasureInterface.SetCreatedAt(guardID, defaultTime.Add(time.Duration(counter)*time.Nanosecond))
				treasureInterface.Save(guardID)
				wg.Done()
			}(i)

		}

		// wait for all treasures to be saved
		wg.Wait()

		// try to get all items back from the creationType beacon
		beacon, err := swampInterface.GetTreasuresByBeacon(swamp.BeaconTypeCreationTime, swamp.IndexOrderDesc, 0, 100000, nil, nil)
		assert.Nil(t, err, "should be nil")
		assert.Equal(t, allTests, len(beacon), "should be equal")

		// delete 1 treasure (key-15)
		err = swampInterface.DeleteTreasure("key-8", false)
		assert.Nil(t, err, "should be nil")

		// try to get all items back from the creationType beacon
		allTreasures, err := swampInterface.GetTreasuresByBeacon(swamp.BeaconTypeCreationTime, swamp.IndexOrderDesc, 0, 100000, nil, nil)
		assert.Nil(t, err, "should be nil")
		assert.Equal(t, allTests-1, len(allTreasures), "should be equal")

		time.Sleep(2 * time.Second)

	})

	t.Run("should get treasures from beacon after the swamp closed, treasure deleted then got from the beacon", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping test with 7s sleep in short mode")
		}

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("get-after-swamp-close-then-delete").Swamp("by-beacon")

		swampInterface, err := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		swampInterface.BeginVigil()

		assert.Nil(t, err, "should be nil")

		defer func() {
			swampInterface2, err := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
			assert.Nil(t, err, "should be nil")
			// destroy the swamp after the test
			swampInterface2.Destroy()
		}()

		allTests := 10

		defaultTime := time.Now()
		wg := sync.WaitGroup{}
		wg.Add(allTests)

		for i := 0; i < allTests; i++ {

			go func(counter int) {
				treasureInterface := swampInterface.CreateTreasure(fmt.Sprintf("key-%d", counter))
				guardID := treasureInterface.StartTreasureGuard(true)
				defer treasureInterface.ReleaseTreasureGuard(guardID)
				treasureInterface.SetContentString(guardID, fmt.Sprintf("my-content-%d", counter))
				treasureInterface.SetCreatedAt(guardID, defaultTime.Add(time.Duration(counter)*time.Nanosecond))
				treasureInterface.Save(guardID)
				wg.Done()
			}(i)

		}

		// wait for all treasures to be saved
		wg.Wait()

		// try to get all items back from the creationType beacon
		beacon, err := swampInterface.GetTreasuresByBeacon(swamp.BeaconTypeCreationTime, swamp.IndexOrderDesc, 0, 100000, nil, nil)
		assert.Nil(t, err, "should be nil")
		assert.Equal(t, allTests, len(beacon), "should be equal")
		// let the swamp to be closed
		swampInterface.CeaseVigil()

		// wait fo the swamp to be closed
		time.Sleep(7 * time.Second)

		// summon the swamp again
		swampInterface, err = hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		assert.Nil(t, err, "should be nil")
		swampInterface.BeginVigil()
		defer swampInterface.CeaseVigil()

		// delete 1 treasure (key-8)
		err = swampInterface.DeleteTreasure("key-8", false)
		assert.Nil(t, err, "should be nil")

		// try to get all items back from the creationType beacon after deleted the treasure
		allTreasures, err := swampInterface.GetTreasuresByBeacon(swamp.BeaconTypeCreationTime, swamp.IndexOrderDesc, 0, 100000, nil, nil)
		assert.Nil(t, err, "should be nil")
		assert.Equal(t, allTests-1, len(allTreasures), "should be equal")

	})

	t.Run("should metadata work", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping test with 15s total sleep in short mode")
		}

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("metadata").Swamp("metadatatest")

		swampInterface, err := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		swampInterface.BeginVigil()

		assert.Nil(t, err, "should be nil")

		defer func() {
			swampInterface2, err := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
			assert.Nil(t, err, "should be nil")
			// destroy the swamp after the test
			swampInterface2.Destroy()
		}()

		allTests := 2

		defaultTime := time.Now()
		wg := sync.WaitGroup{}
		wg.Add(allTests)

		for i := 0; i < allTests; i++ {

			go func(counter int) {
				treasureInterface := swampInterface.CreateTreasure(fmt.Sprintf("key-%d", counter))
				guardID := treasureInterface.StartTreasureGuard(true)
				defer treasureInterface.ReleaseTreasureGuard(guardID)
				treasureInterface.SetContentString(guardID, fmt.Sprintf("my-content-%d", counter))
				treasureInterface.SetCreatedAt(guardID, defaultTime.Add(time.Duration(counter)*time.Nanosecond))
				treasureInterface.Save(guardID)
				wg.Done()
			}(i)

		}

		// wait for all treasures to be saved
		wg.Wait()

		firstCreatedAt := swampInterface.GetMetadata().GetCreatedAt()
		firstUpdatedAt := swampInterface.GetMetadata().GetUpdatedAt()

		assert.NotEqual(t, time.Time{}, firstCreatedAt)
		assert.Less(t, firstCreatedAt, time.Now())

		assert.NotEqual(t, time.Time{}, firstUpdatedAt)
		assert.Less(t, firstUpdatedAt, time.Now())
		swampInterface.CeaseVigil()

		// várunk az írásra és a swamp bezárására
		time.Sleep(5 * time.Second)

		// summon the swamp again
		swampInterface, err = hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		assert.Nil(t, err, "should be nil")
		swampInterface.BeginVigil()

		assert.Equal(t, swampInterface.GetMetadata().GetSwampName().Get(), swampName.Get())

		// add new data
		treasureInterface := swampInterface.CreateTreasure("key-100")
		guardID := treasureInterface.StartTreasureGuard(true)
		treasureInterface.SetContentString(guardID, "my-content-100")
		treasureInterface.Save(guardID)
		treasureInterface.ReleaseTreasureGuard(guardID)

		secondCreatedAt := swampInterface.GetMetadata().GetCreatedAt()
		secondUpdatedAt := swampInterface.GetMetadata().GetUpdatedAt()
		swampInterface.CeaseVigil()

		assert.Equal(t, firstCreatedAt, secondCreatedAt)
		assert.Greater(t, secondUpdatedAt, firstUpdatedAt)

		time.Sleep(5 * time.Second)

		// most csak betöltjük a swampot, de nem módosítjuk
		swampInterface, err = hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		assert.Nil(t, err, "should be nil")

		thirdCreatedAt := swampInterface.GetMetadata().GetCreatedAt()
		thirdUpdatedAt := swampInterface.GetMetadata().GetUpdatedAt()

		assert.True(t, firstCreatedAt.Equal(thirdCreatedAt))
		assert.True(t, secondUpdatedAt.Equal(thirdUpdatedAt))

		time.Sleep(5 * time.Second)

	})

	t.Run("should subscribe to swamp events works", func(t *testing.T) {

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("subscription-test").Swamp("subscribe-to-event")

		// destroy the swamp before the test, if the swamp exists
		swampInterface, err := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		assert.Nil(t, err, "should be nil")
		swampInterface.Destroy()

		clientID := uuid.New()

		defer func() {

			// unsubscribe from the event
			err := hydraInterface.UnsubscribeFromSwampEvents(clientID, swampName)
			assert.Nil(t, err, "should be nil")

			// destroy the swamp after the test
			swampInterface, err := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
			assert.Nil(t, err, "should be nil")
			swampInterface.Destroy()

		}()

		alltests := 10
		wg := sync.WaitGroup{}
		wg.Add(alltests)

		err = hydraInterface.SubscribeToSwampEvents(clientID, swampName, func(event *swamp.Event) {
			wg.Done()
		})

		assert.Nil(t, err, "should be nil")

		swampInterface.BeginVigil()
		defer swampInterface.CeaseVigil()

		swampInterface, err = hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		assert.Nil(t, err, "should be nil")
		// insertáljunk be 10 treasure-t
		for i := 0; i < alltests; i++ {
			treasureInterface := swampInterface.CreateTreasure(fmt.Sprintf("key-%d", i))
			guardID := treasureInterface.StartTreasureGuard(true)
			treasureInterface.SetContentString(guardID, fmt.Sprintf("my-content-%d", i))
			treasureInterface.Save(guardID)
			treasureInterface.ReleaseTreasureGuard(guardID)
		}

		// várjuk meg, hogy az esemény megérkezzen és a feliratkozott függvény lefusson
		wg.Wait()

	})

	t.Run("should subscribe to swamp info works", func(t *testing.T) {

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("subscription-test").Swamp("subscribe-to-swamp-info")

		// destroy the swamp before the test, if the swamp exists
		swampInterface, err := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		assert.Nil(t, err, "should be nil")
		swampInterface.Destroy()

		clientID := uuid.New()

		defer func() {
			// unsubscribe from the event
			err := hydraInterface.UnsubscribeFromSwampInfo(clientID, swampName)
			assert.Nil(t, err, "should be nil")
			// destroy the swamp after the test
			swampInterface, err := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
			assert.Nil(t, err, "should be nil")
			swampInterface.Destroy()

		}()

		alltests := 10
		wg := sync.WaitGroup{}
		wg.Add(alltests)

		err = hydraInterface.SubscribeToSwampInfo(clientID, swampName, func(info *swamp.Info) {
			wg.Done()
		})
		assert.Nil(t, err, "should be nil")

		swampInterface.BeginVigil()
		defer swampInterface.CeaseVigil()

		swampInterface, err = hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		assert.Nil(t, err, "should be nil")

		// insertáljunk be 10 treasure-t
		for i := 0; i < alltests; i++ {
			treasureInterface := swampInterface.CreateTreasure(fmt.Sprintf("key-%d", i))
			guardID := treasureInterface.StartTreasureGuard(true)
			treasureInterface.SetContentString(guardID, fmt.Sprintf("my-content-%d", i))
			treasureInterface.Save(guardID)
			treasureInterface.ReleaseTreasureGuard(guardID)
		}

		wg.Wait()

	})

	t.Run("should list and count all active swamps", func(t *testing.T) {

		// create 10 swmaps
		var testSwampNames []name.Name
		allTests := 10
		for i := 0; i < allTests; i++ {
			testSwampNames = append(testSwampNames, name.New().Sanctuary("test").Realm("active-swamp-list").Swamp(fmt.Sprintf("swamp-%d", i)))
		}

		// summon the swamps
		for i := 0; i < allTests; i++ {
			_, _ = hydraInterface.SummonSwamp(context.Background(), 10, testSwampNames[i])
		}

		allActiveSwamps := hydraInterface.ListActiveSwamps()
		assert.Equal(t, allTests, len(allActiveSwamps), "should be equal")
		activeSwampCounter := hydraInterface.CountActiveSwamps()
		assert.Equal(t, allTests, activeSwampCounter, "should be equal")

		// delete all swamps
		for i := 0; i < allTests; i++ {
			swampInterface, _ := hydraInterface.SummonSwamp(context.Background(), 10, testSwampNames[i])
			swampInterface.Destroy()
		}

	})

}
