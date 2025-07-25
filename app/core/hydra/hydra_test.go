package hydra

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/hydraide/hydraide/app/core/filesystem"
	"github.com/hydraide/hydraide/app/core/hydra/lock"
	"github.com/hydraide/hydraide/app/core/hydra/swamp"
	"github.com/hydraide/hydraide/app/core/safeops"
	"github.com/hydraide/hydraide/app/core/settings"
	"github.com/hydraide/hydraide/app/name"
	"github.com/stretchr/testify/assert"
	"log/slog"
	"sync"
	"testing"
	"time"
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
		beacon, err := swampInterface.GetTreasuresByBeacon(swamp.BeaconTypeCreationTime, swamp.IndexOrderDesc, 0, 100000)
		assert.Nil(t, err, "should be nil")
		assert.Equal(t, allTests, len(beacon), "should be equal")

		// delete 1 treasure (key-15)
		err = swampInterface.DeleteTreasure("key-8", false)
		assert.Nil(t, err, "should be nil")

		// try to get all items back from the creationType beacon
		allTreasures, err := swampInterface.GetTreasuresByBeacon(swamp.BeaconTypeCreationTime, swamp.IndexOrderDesc, 0, 100000)
		assert.Nil(t, err, "should be nil")
		assert.Equal(t, allTests-1, len(allTreasures), "should be equal")

		time.Sleep(2 * time.Second)

	})

	t.Run("should get treasures from beacon after the swamp closed, treasure deleted then got from the beacon", func(t *testing.T) {

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
		beacon, err := swampInterface.GetTreasuresByBeacon(swamp.BeaconTypeCreationTime, swamp.IndexOrderDesc, 0, 100000)
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
		allTreasures, err := swampInterface.GetTreasuresByBeacon(swamp.BeaconTypeCreationTime, swamp.IndexOrderDesc, 0, 100000)
		assert.Nil(t, err, "should be nil")
		assert.Equal(t, allTests-1, len(allTreasures), "should be equal")

	})

	t.Run("should metadata work", func(t *testing.T) {

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

// hydra_test.go:690: Total time: 3.989516361s (1000000 op)
// Average per op: 3.989µs
func TestHydraInsertTiming(t *testing.T) {

	elysiumInterface := safeops.New()
	lockerInterface := lock.New()
	fsInterface := filesystem.New()

	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)
	fss := &settings.FileSystemSettings{
		WriteIntervalSec: 1,
		MaxFileSizeByte:  8192,
	}

	settingsInterface.RegisterPattern(
		name.New().Sanctuary("test").Realm("*").Swamp("*"),
		false, 1, fss,
	)

	hydraInterface := New(settingsInterface, elysiumInterface, lockerInterface, fsInterface)
	swampName := name.New().Sanctuary("test").Realm("timing").Swamp("swamp")

	count := 1000000
	start := time.Now()

	for i := 0; i < count; i++ {
		si, _ := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		ti := si.CreateTreasure(fmt.Sprintf("treasure-%d", i))
		tg := ti.StartTreasureGuard(true)
		ti.SetContentString(tg, fmt.Sprintf("content-%d", i))
		ti.Save(tg)
		ti.ReleaseTreasureGuard(tg)
	}

	elapsed := time.Since(start)
	perOp := elapsed / time.Duration(count)
	t.Logf("Total time: %s (%d op)\nAverage per op: %s", elapsed, count, perOp)

	// destroy the swamp after the test
	swampInterface, _ := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
	swampInterface.Destroy()

}

// hydra_test.go:732: Total time: 3.930572938s (1000000 op)
// Average per op: 3.93µs
func TestHydraInsertTiming_InMemory(t *testing.T) {

	elysiumInterface := safeops.New()
	lockerInterface := lock.New()
	fsInterface := filesystem.New()

	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)

	settingsInterface.RegisterPattern(
		name.New().Sanctuary(sanctuaryForQuickTest).Realm("inmemory").Swamp("summonandsave"),
		true, // inMemory = true
		1,
		nil, // nincs szükség FileSystemSettings-re inMemory módban
	)

	hydraInterface := New(settingsInterface, elysiumInterface, lockerInterface, fsInterface)
	swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("inmemory").Swamp("summonandsave")

	count := 1000000
	start := time.Now()

	for i := 0; i < count; i++ {
		si, _ := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
		ti := si.CreateTreasure(fmt.Sprintf("treasure-%d", i))
		tg := ti.StartTreasureGuard(true)
		ti.SetContentString(tg, fmt.Sprintf("content-%d", i))
		ti.Save(tg)
		ti.ReleaseTreasureGuard(tg)
	}

	elapsed := time.Since(start)
	perOp := elapsed / time.Duration(count)
	t.Logf("Total time: %s (%d op)\nAverage per op: %s", elapsed, count, perOp)

	// cleanup
	swampInterface, _ := hydraInterface.SummonSwamp(context.Background(), 10, swampName)
	swampInterface.Destroy()

}

// hydra_test.go:773: Total time: 3.576679347s (1000000 op)
// Average per op: 3.576µs
func TestHydraBulkInsertTiming_InMemory(t *testing.T) {
	elysiumInterface := safeops.New()
	lockerInterface := lock.New()
	fsInterface := filesystem.New()

	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)

	settingsInterface.RegisterPattern(
		name.New().Sanctuary(sanctuaryForQuickTest).Realm("bulk").Swamp("inmemory"),
		true, // inMemory = true
		3500, // closeAfterIdleSec
		nil,  // nincs fájlkorlát
	)

	hydraInterface := New(settingsInterface, elysiumInterface, lockerInterface, fsInterface)
	swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("bulk").Swamp("inmemory")

	// Swamp summon egyszer, a benchmark előtt
	si, _ := hydraInterface.SummonSwamp(context.Background(), 10, swampName)

	count := 1000000
	start := time.Now()

	for i := 0; i < count; i++ {
		ti := si.CreateTreasure(fmt.Sprintf("treasure-%d", i))
		tg := ti.StartTreasureGuard(true)
		ti.SetContentString(tg, fmt.Sprintf("content-%d", i))
		ti.Save(tg)
		ti.ReleaseTreasureGuard(tg)
	}

	elapsed := time.Since(start)
	perOp := elapsed / time.Duration(count)
	t.Logf("Total time: %s (%d op)\nAverage per op: %s", elapsed, count, perOp)

	// cleanup
	si.Destroy()
}

// hydra_test.go:816: Total time: 78.091304ms (1000000 op)
// Average per op: 78ns
func TestHydraGetTiming(t *testing.T) {
	elysiumInterface := safeops.New()
	lockerInterface := lock.New()
	fsInterface := filesystem.New()

	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)
	fss := &settings.FileSystemSettings{
		WriteIntervalSec: 1,
		MaxFileSizeByte:  8192,
	}

	settingsInterface.RegisterPattern(
		name.New().Sanctuary(sanctuaryForQuickTest).Realm("gettest").Swamp("readonly"),
		false, // nem inMemory most, de lehetne az is
		3500,
		fss,
	)

	hydraInterface := New(settingsInterface, elysiumInterface, lockerInterface, fsInterface)

	swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("gettest").Swamp("readonly")
	si, _ := hydraInterface.SummonSwamp(context.Background(), 10, swampName)

	// Létrehozzuk a treasure-t, amit olvasni fogunk
	ti := si.CreateTreasure("treasure")
	tg := ti.StartTreasureGuard(true)
	ti.SetContentString(tg, "content")
	ti.Save(tg)
	ti.ReleaseTreasureGuard(tg)

	// Most mérjük a GET-ek sebességét
	count := 1000000
	start := time.Now()

	for i := 0; i < count; i++ {
		_, _ = si.GetTreasure("treasure")
	}

	elapsed := time.Since(start)
	perOp := elapsed / time.Duration(count)
	t.Logf("Total time: %s (%d op)\nAverage per op: %s", elapsed, count, perOp)

	si.Destroy()
}

// hydra_test.go:853: Batch Get – Total: 2.308329ms (10k op)
// Average per op: 230ns
func TestHydraBatchGetTiming(t *testing.T) {
	elysiumInterface := safeops.New()
	lockerInterface := lock.New()
	fsInterface := filesystem.New()

	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)

	settingsInterface.RegisterPattern(
		name.New().Sanctuary(sanctuaryForQuickTest).Realm("batchget").Swamp("multi"),
		true, // inMemory
		3600,
		nil,
	)

	hydraInterface := New(settingsInterface, elysiumInterface, lockerInterface, fsInterface)
	swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("batchget").Swamp("multi")

	si, _ := hydraInterface.SummonSwamp(context.Background(), 10, swampName)

	// Feltöltünk előre 10.000 treasure-t
	for i := 0; i < 10000; i++ {
		ti := si.CreateTreasure(fmt.Sprintf("treasure-%d", i))
		tg := ti.StartTreasureGuard(true)
		ti.SetContentString(tg, fmt.Sprintf("content-%d", i))
		ti.Save(tg)
		ti.ReleaseTreasureGuard(tg)
	}

	// Most mérjük, hogy mennyi idő alatt listázzuk ki őket
	start := time.Now()
	for i := 0; i < 10000; i++ {
		_, _ = si.GetTreasure(fmt.Sprintf("treasure-%d", i))
	}
	elapsed := time.Since(start)
	perOp := elapsed / 10000

	t.Logf("Batch Get – Total: %s (10k op)\nAverage per op: %s", elapsed, perOp)

	si.Destroy()
}

func TestHydraGetTiming_Parallel(t *testing.T) {
	elysiumInterface := safeops.New()
	lockerInterface := lock.New()
	fsInterface := filesystem.New()

	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)
	settingsInterface.RegisterPattern(
		name.New().Sanctuary(sanctuaryForQuickTest).Realm("parallel").Swamp("get"),
		true,
		3600,
		nil,
	)

	hydraInterface := New(settingsInterface, elysiumInterface, lockerInterface, fsInterface)
	swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("parallel").Swamp("get")
	si, _ := hydraInterface.SummonSwamp(context.Background(), 10, swampName)

	// upload the swamp with treasures
	totalTreasures := 10000
	for i := 0; i < totalTreasures; i++ {
		ti := si.CreateTreasure(fmt.Sprintf("treasure-%d", i))
		tg := ti.StartTreasureGuard(true)
		ti.SetContentString(tg, fmt.Sprintf("content-%d", i))
		ti.Save(tg)
		ti.ReleaseTreasureGuard(tg)
	}

	threads := 16
	opsPerThread := 100000
	totalOps := threads * opsPerThread

	var wg sync.WaitGroup
	wg.Add(threads)

	start := time.Now()

	for tIdx := 0; tIdx < threads; tIdx++ {
		go func(thread int) {
			defer wg.Done()
			for i := 0; i < opsPerThread; i++ {
				_, _ = si.GetTreasure(fmt.Sprintf("treasure-%d", i%totalTreasures))
			}
		}(tIdx)
	}

	wg.Wait()
	elapsed := time.Since(start)
	perOp := elapsed / time.Duration(totalOps)

	t.Logf("Parallel GET (%d threads, %d op) — Total: %s\nAverage per op: %s", threads, totalOps, elapsed, perOp)

	si.Destroy()
}

func TestHydraGetTiming_Parallel_MultiSwamp(t *testing.T) {
	elysiumInterface := safeops.New()
	lockerInterface := lock.New()
	fsInterface := filesystem.New()
	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)

	realm := "multiswamp"
	swampCount := 16
	opsPerSwamp := 100000
	totalOps := swampCount * opsPerSwamp

	swamps := make([]swamp.Swamp, swampCount)

	// registering the pattern for multiple swamps
	for i := 0; i < swampCount; i++ {
		sanctuary := name.New().Sanctuary(sanctuaryForQuickTest).Realm(realm).Swamp(fmt.Sprintf("swamp-%d", i))
		settingsInterface.RegisterPattern(
			sanctuary,
			true, // inMemory
			3600,
			nil,
		)
	}

	hydraInterface := New(settingsInterface, elysiumInterface, lockerInterface, fsInterface)

	// Summon + upload
	for i := 0; i < swampCount; i++ {
		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm(realm).Swamp(fmt.Sprintf("swamp-%d", i))
		si, _ := hydraInterface.SummonSwamp(context.Background(), 10, swampName)

		ti := si.CreateTreasure("treasure")
		tg := ti.StartTreasureGuard(true)
		ti.SetContentString(tg, "multiswamp-content")
		ti.Save(tg)
		ti.ReleaseTreasureGuard(tg)

		swamps[i] = si
	}

	var wg sync.WaitGroup
	wg.Add(swampCount)

	start := time.Now()
	for i := 0; i < swampCount; i++ {
		si := swamps[i]
		go func(swampIdx int) {
			defer wg.Done()
			for j := 0; j < opsPerSwamp; j++ {
				_, _ = si.GetTreasure("treasure")
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)
	perOp := elapsed / time.Duration(totalOps)

	t.Logf("MultiSwamp Parallel GET (%d swamps × %d op) — Total: %s\nAverage per op: %s", swampCount, opsPerSwamp, elapsed, perOp)

	// Cleanup
	for _, si := range swamps {
		si.Destroy()
	}
}
