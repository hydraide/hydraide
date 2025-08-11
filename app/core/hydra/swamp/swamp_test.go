package swamp

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/filesystem"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/metadata"
	"github.com/hydraide/hydraide/app/core/settings"
	"github.com/hydraide/hydraide/app/name"
	"github.com/stretchr/testify/assert"
)

const (
	sanctuaryForQuickTest = "quick-test"
	testAllServers        = 100
	testMaxDepth          = 3
	testMaxFolderPerLevel = 2000
)

func TestNew(t *testing.T) {

	fsInterface := filesystem.New()
	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)
	// gyors filementés és bezárás a tesztekhez
	fss := &settings.FileSystemSettings{
		WriteIntervalSec: 1,
		MaxFileSizeByte:  8192,
	}
	settingsInterface.RegisterPattern(name.New().Sanctuary(sanctuaryForQuickTest).Realm("*").Swamp("*"), false, 1, fss)
	closeAfterIdle := 1 * time.Second
	writeInterval := 1 * time.Second
	maxFileSize := int64(8192)

	t.Run("should create a treasure", func(t *testing.T) {

		swampEventCallbackFunc := func(e *Event) {
			fmt.Println("event received")
		}

		closeCallbackFunc := func(n name.Name) {
			t.Log("swamp closed" + n.Get())
		}

		swampInfoCallbackFunc := func(i *Info) {
			fmt.Println("info received")
		}

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("create").Swamp("treasure")

		hashPath := swampName.GetFullHashPath(settingsInterface.GetHydraAbsDataFolderPath(), testAllServers, testMaxDepth, testMaxFolderPerLevel)
		chroniclerInterface := chronicler.New(hashPath, maxFileSize, testMaxDepth, fsInterface, metadata.New(hashPath))
		chroniclerInterface.CreateDirectoryIfNotExists()

		fssSwamp := &FilesystemSettings{
			ChroniclerInterface: chroniclerInterface,
			WriteInterval:       writeInterval,
		}

		metadataInterface := metadata.New(hashPath)
		swampInterface := New(swampName, closeAfterIdle, fssSwamp, swampEventCallbackFunc, swampInfoCallbackFunc, closeCallbackFunc, metadataInterface)

		treasureInterface := swampInterface.CreateTreasure("test")
		assert.NotNil(t, treasureInterface)

		swampInterface.Destroy()

	})

	t.Run("should close a swamp by the close function", func(t *testing.T) {

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("should-close-the-swamp").Swamp("by-the-close-button")

		hashPath := swampName.GetFullHashPath(settingsInterface.GetHydraAbsDataFolderPath(), testAllServers, testMaxDepth, testMaxFolderPerLevel)
		chroniclerInterface := chronicler.New(hashPath, maxFileSize, testMaxDepth, fsInterface, metadata.New(hashPath))
		chroniclerInterface.CreateDirectoryIfNotExists()

		wg := &sync.WaitGroup{}
		wg.Add(1)

		closeCounter := 0

		swampEventCallbackFunc := func(e *Event) {
			fmt.Println("event received")
		}

		closeCallbackFunc := func(n name.Name) {
			t.Log("swamp closed " + n.Get())
			closeCounter++
			// a destroy funkció is küld egy closed eseményt, ezért itt 2 esemény is keletkezik
			if closeCounter == 1 {
				wg.Done()
			}
		}

		swampInfoCallbackFunc := func(i *Info) {
			fmt.Println("info received")
		}

		fssSwamp := &FilesystemSettings{
			ChroniclerInterface: chroniclerInterface,
			WriteInterval:       writeInterval,
		}

		metadataInterface := metadata.New(hashPath)
		swampInterface := New(swampName, closeAfterIdle, fssSwamp, swampEventCallbackFunc, swampInfoCallbackFunc, closeCallbackFunc, metadataInterface)
		swampInterface.BeginVigil()

		treasureInterface := swampInterface.CreateTreasure("test")
		// treasureInterface should not be nil
		assert.NotNil(t, treasureInterface)
		guardID := treasureInterface.StartTreasureGuard(true)
		treasureInterface.Save(guardID)
		treasureInterface.ReleaseTreasureGuard(guardID)

		swampInterface.CeaseVigil()
		swampInterface.Close()

		wg.Wait()

		swampInterface.Destroy()

	})

	t.Run("should close a swamp by the idle setting", func(t *testing.T) {

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("should-close-the-swamp").Swamp("by-idle-setting")

		hashPath := swampName.GetFullHashPath(settingsInterface.GetHydraAbsDataFolderPath(), testAllServers, testMaxDepth, testMaxFolderPerLevel)
		chroniclerInterface := chronicler.New(hashPath, maxFileSize, testMaxDepth, fsInterface, metadata.New(hashPath))
		chroniclerInterface.CreateDirectoryIfNotExists()

		isClosed := int32(0)
		swampEventCallbackFunc := func(e *Event) {
			fmt.Println("event received")
		}

		closeCallbackFunc := func(n name.Name) {
			t.Log("swamp closed" + n.Get())
			atomic.StoreInt32(&isClosed, 1)
		}

		swampInfoCallbackFunc := func(i *Info) {
			fmt.Println("info received")
		}

		fssSwamp := &FilesystemSettings{
			ChroniclerInterface: chroniclerInterface,
			WriteInterval:       writeInterval,
		}
		metadataInterface := metadata.New(hashPath)
		swampInterface := New(swampName, closeAfterIdle, fssSwamp, swampEventCallbackFunc, swampInfoCallbackFunc, closeCallbackFunc, metadataInterface)

		swampInterface.BeginVigil()
		treasureInterface := swampInterface.CreateTreasure("test")
		if treasureInterface == nil {
			t.Errorf("treasureInterface should not be nil")
		}
		swampInterface.CeaseVigil()

		time.Sleep(2100 * time.Millisecond)

		assert.Equal(t, int32(1), atomic.LoadInt32(&isClosed), "swamp should be closed")

		swampInterface.Destroy()

	})

}
func TestSwamp_DeleteAllTreasures(t *testing.T) {

	fsInterface := filesystem.New()
	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)
	// gyors filementés és bezárás a tesztekhez
	// gyors filementés és bezárás a tesztekhez
	fss := &settings.FileSystemSettings{
		WriteIntervalSec: 1,
		MaxFileSizeByte:  8192,
	}
	settingsInterface.RegisterPattern(name.New().Sanctuary(sanctuaryForQuickTest).Realm("*").Swamp("*"), false, 1, fss)
	closeAfterIdle := 1 * time.Second
	writeInterval := 1 * time.Second
	maxFileSize := int64(8192)

	t.Run("should delete all treasures", func(t *testing.T) {

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("should-delete").Swamp("all-treasures")

		hashPath := swampName.GetFullHashPath(settingsInterface.GetHydraAbsDataFolderPath(), testAllServers, testMaxDepth, testMaxFolderPerLevel)
		chroniclerInterface := chronicler.New(hashPath, maxFileSize, testMaxDepth, fsInterface, metadata.New(hashPath))
		chroniclerInterface.CreateDirectoryIfNotExists()

		swampEventCallbackFunc := func(e *Event) {
			fmt.Println("event received")
		}

		closeCallbackFunc := func(n name.Name) {
			t.Log("swamp closed" + n.Get())
		}

		swampInfoCallbackFunc := func(i *Info) {
			fmt.Println("info received")
		}

		fssSwamp := &FilesystemSettings{
			ChroniclerInterface: chroniclerInterface,
			WriteInterval:       writeInterval,
		}

		metadataInterface := metadata.New(hashPath)
		swampInterface := New(swampName, closeAfterIdle, fssSwamp, swampEventCallbackFunc, swampInfoCallbackFunc, closeCallbackFunc, metadataInterface)
		swampInterface.BeginVigil()
		for i := 0; i < 100; i++ {
			treasureInterface := swampInterface.CreateTreasure(fmt.Sprintf("test-%d", i))
			if treasureInterface == nil {
				t.Errorf("treasureInterface should not be nil")
			}
			guardID := treasureInterface.StartTreasureGuard(true)
			_ = treasureInterface.Save(guardID)
			treasureInterface.ReleaseTreasureGuard(guardID)
		}

		assert.Equal(t, 100, swampInterface.CountTreasures(), "treasures should be 100")

		// get all treasures
		treasures := swampInterface.GetAll()
		for _, treasure := range treasures {
			if err := swampInterface.DeleteTreasure(treasure.GetKey(), false); err != nil {
				t.Errorf("error should be nil")
			}
		}
		assert.Equal(t, 0, swampInterface.CountTreasures(), "treasures should be 0")

		swampInterface.CeaseVigil()

	})

}
func TestSwamp_SendingInformation(t *testing.T) {

	fsInterface := filesystem.New()
	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)
	// gyors filementés és bezárás a tesztekhez
	fss := &settings.FileSystemSettings{
		WriteIntervalSec: 1,
		MaxFileSizeByte:  8192,
	}
	settingsInterface.RegisterPattern(name.New().Sanctuary(sanctuaryForQuickTest).Realm("*").Swamp("*"), false, 1, fss)
	closeAfterIdle := 1 * time.Second
	writeInterval := 1 * time.Second
	maxFileSize := int64(8192)

	t.Run("should send information after all saved treasures", func(t *testing.T) {

		allTests := 100

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("should-send").Swamp("information")

		hashPath := swampName.GetFullHashPath(settingsInterface.GetHydraAbsDataFolderPath(), testAllServers, testMaxDepth, testMaxFolderPerLevel)
		chroniclerInterface := chronicler.New(hashPath, maxFileSize, testMaxDepth, fsInterface, metadata.New(hashPath))
		chroniclerInterface.CreateDirectoryIfNotExists()

		wg := &sync.WaitGroup{}
		wg.Add(1)

		allInfoCounter := 0
		swampEventCallbackFunc := func(e *Event) {}

		closeCallbackFunc := func(n name.Name) {}

		swampInfoCallbackFunc := func(i *Info) {
			allInfoCounter++
			if allInfoCounter == allTests {
				wg.Done()
			}
		}

		fssSwamp := &FilesystemSettings{
			ChroniclerInterface: chroniclerInterface,
			WriteInterval:       writeInterval,
		}

		metadataInterface := metadata.New(hashPath)
		swampInterface := New(swampName, closeAfterIdle, fssSwamp, swampEventCallbackFunc, swampInfoCallbackFunc, closeCallbackFunc, metadataInterface)

		swampInterface.StartSendingInformation()

		for i := 0; i < allTests; i++ {
			treasureInterface := swampInterface.CreateTreasure(fmt.Sprintf("test-%d-%d", time.Now().UnixNano(), i))
			if treasureInterface == nil {
				t.Errorf("treasureInterface should not be nil")
			}
			guardID := treasureInterface.StartTreasureGuard(true)
			_ = treasureInterface.Save(guardID)
			treasureInterface.ReleaseTreasureGuard(guardID)
		}

		wg.Wait()

		swampInterface.StopSendingInformation()

		swampInterface.Destroy()

	})

}
func TestSwamp_SendingEvent(t *testing.T) {

	fsInterface := filesystem.New()
	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)
	// gyors filementés és bezárás a tesztekhez
	fss := &settings.FileSystemSettings{
		WriteIntervalSec: 1,
		MaxFileSizeByte:  8192,
	}
	settingsInterface.RegisterPattern(name.New().Sanctuary(sanctuaryForQuickTest).Realm("*").Swamp("*"), false, 1, fss)
	closeAfterIdle := 1 * time.Second
	writeInterval := 1 * time.Second
	maxFileSize := int64(8192)

	t.Run("should send events after all saved treasures", func(t *testing.T) {

		allTests := 100

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("should-send").Swamp("event")

		hashPath := swampName.GetFullHashPath(settingsInterface.GetHydraAbsDataFolderPath(), testAllServers, testMaxDepth, testMaxFolderPerLevel)
		chroniclerInterface := chronicler.New(hashPath, maxFileSize, testMaxDepth, fsInterface, metadata.New(hashPath))
		chroniclerInterface.CreateDirectoryIfNotExists()

		wg := &sync.WaitGroup{}
		wg.Add(allTests)
		eventCounter := 0
		swampEventCallbackFunc := func(e *Event) {
			eventCounter++
			if eventCounter == allTests {
				wg.Done()
			}
		}

		closeCallbackFunc := func(n name.Name) {}

		swampInfoCallbackFunc := func(i *Info) {}

		fssSwamp := &FilesystemSettings{
			ChroniclerInterface: chroniclerInterface,
			WriteInterval:       writeInterval,
		}

		metadataInterface := metadata.New(hashPath)
		swampInterface := New(swampName, closeAfterIdle, fssSwamp, swampEventCallbackFunc, swampInfoCallbackFunc, closeCallbackFunc, metadataInterface)
		defer swampInterface.Destroy()

		swampInterface.BeginVigil()
		swampInterface.StartSendingEvents()

		for i := 0; i < allTests; i++ {
			treasureInterface := swampInterface.CreateTreasure(fmt.Sprintf("test-%d-%d", time.Now().Unix(), i))
			if treasureInterface == nil {
				t.Errorf("treasureInterface should not be nil")
			}
			guardID := treasureInterface.StartTreasureGuard(true)
			_ = treasureInterface.Save(guardID)
			treasureInterface.ReleaseTreasureGuard(guardID)
		}

		swampInterface.CeaseVigil()

		wg.Done()

		swampInterface.BeginVigil()
		swampInterface.StopSendingEvents()
		swampInterface.CeaseVigil()

	})

}
func TestSwamp_GetTreasuresByBeacon(t *testing.T) {

	fsInterface := filesystem.New()
	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)
	// gyors filementés és bezárás a tesztekhez
	fss := &settings.FileSystemSettings{
		WriteIntervalSec: 1,
		MaxFileSizeByte:  8192,
	}
	settingsInterface.RegisterPattern(name.New().Sanctuary(sanctuaryForQuickTest).Realm("*").Swamp("*"), false, 1, fss)
	closeAfterIdle := 1 * time.Second
	writeInterval := 1 * time.Second
	maxFileSize := int64(8192)

	t.Run("Should Get treasures by the beacon", func(t *testing.T) {

		allTests := 10

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("should-get-treasure").Swamp("by-beacon")

		hashPath := swampName.GetFullHashPath(settingsInterface.GetHydraAbsDataFolderPath(), testAllServers, testMaxDepth, testMaxFolderPerLevel)
		chroniclerInterface := chronicler.New(hashPath, maxFileSize, testMaxDepth, fsInterface, metadata.New(hashPath))
		chroniclerInterface.CreateDirectoryIfNotExists()

		swampEventCallbackFunc := func(e *Event) {}

		closeCallbackFunc := func(n name.Name) {}

		swampInfoCallbackFunc := func(i *Info) {}

		fssSwamp := &FilesystemSettings{
			ChroniclerInterface: chroniclerInterface,
			WriteInterval:       writeInterval,
		}

		metadataInterface := metadata.New(hashPath)
		swampInterface := New(swampName, closeAfterIdle, fssSwamp, swampEventCallbackFunc, swampInfoCallbackFunc, closeCallbackFunc, metadataInterface)
		swampInterface.BeginVigil()

		for i := 0; i < allTests; i++ {
			treasureInterface := swampInterface.CreateTreasure(fmt.Sprintf("%d", i))
			if treasureInterface == nil {
				t.Errorf("treasureInterface should not be nil")
			}
			guardID := treasureInterface.StartTreasureGuard(true)
			treasureInterface.SetCreatedAt(guardID, time.Now())
			treasureInterface.SetModifiedAt(guardID, time.Now())
			treasureInterface.SetContentString(guardID, fmt.Sprintf("content-%d", i))
			treasureInterface.ReleaseTreasureGuard(guardID)

			guardID = treasureInterface.StartTreasureGuard(true)
			_ = treasureInterface.Save(guardID)
			treasureInterface.ReleaseTreasureGuard(guardID)

			time.Sleep(time.Millisecond * 10)
		}

		receivedTreasures, err := swampInterface.GetTreasuresByBeacon(BeaconTypeCreationTime, IndexOrderAsc, 0, 10)
		assert.Nil(t, err, "error should be nil")
		assert.Equal(t, allTests, len(receivedTreasures), "treasures should be 10")

		lastID := 0
		for _, tr := range receivedTreasures {
			keyInt, err := strconv.Atoi(tr.GetKey())
			assert.Nil(t, err, "error should be nil")
			assert.Equal(t, lastID, keyInt, "key should be in order")
			lastID++
		}

		receivedTreasures, err = swampInterface.GetTreasuresByBeacon(BeaconTypeCreationTime, IndexOrderDesc, 0, 10)
		assert.Nil(t, err, "error should be nil")
		assert.Equal(t, allTests, len(receivedTreasures), "treasures should be 10")

		lastID = 9
		for _, tr := range receivedTreasures {
			keyInt, err := strconv.Atoi(tr.GetKey())
			assert.Nil(t, err, "error should be nil")
			assert.Equal(t, lastID, keyInt, "key should be in order")
			lastID--
		}

		receivedTreasures, err = swampInterface.GetTreasuresByBeacon(BeaconTypeUpdateTime, IndexOrderAsc, 0, 5)
		assert.Nil(t, err, "error should be nil")
		assert.Equal(t, 5, len(receivedTreasures), "treasures should be 5")

		lastID = 0
		for _, tr := range receivedTreasures {
			keyInt, err := strconv.Atoi(tr.GetKey())
			assert.Nil(t, err, "error should be nil")
			assert.Equal(t, lastID, keyInt, "key should be in order")
			lastID++
		}

		receivedTreasures, err = swampInterface.GetTreasuresByBeacon(BeaconTypeUpdateTime, IndexOrderDesc, 0, 5)
		assert.Nil(t, err, "error should be nil")
		assert.Equal(t, 5, len(receivedTreasures), "treasures should be 5")

		lastID = 9
		for _, tr := range receivedTreasures {
			keyInt, err := strconv.Atoi(tr.GetKey())
			assert.Nil(t, err, "error should be nil")
			assert.Equal(t, lastID, keyInt, "key should be in order")
			lastID--
		}

		receivedTreasures, err = swampInterface.GetTreasuresByBeacon(BeaconTypeValueString, IndexOrderAsc, 0, 10)
		assert.Nil(t, err, "error should be nil")
		assert.Equal(t, 10, len(receivedTreasures), "treasures should be 10")

		lastID = 0
		for _, tr := range receivedTreasures {
			keyInt, err := strconv.Atoi(tr.GetKey())
			assert.Nil(t, err, "error should be nil")
			assert.Equal(t, lastID, keyInt, "key should be in order")
			lastID++
		}

		receivedTreasures, err = swampInterface.GetTreasuresByBeacon(BeaconTypeValueString, IndexOrderDesc, 0, 10)
		assert.Nil(t, err, "error should be nil")
		assert.Equal(t, 10, len(receivedTreasures), "treasures should be 10")

		lastID = 9
		for _, tr := range receivedTreasures {
			keyInt, err := strconv.Atoi(tr.GetKey())
			assert.Nil(t, err, "error should be nil")
			assert.Equal(t, lastID, keyInt, "key should be in order")
			lastID--
		}

		swampInterface.CeaseVigil()
		swampInterface.Destroy()

	})

	t.Run("should get treasures by the int beacon", func(t *testing.T) {

		allTests := 10

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("should-get-treasure").Swamp("by-int-beacon")

		hashPath := swampName.GetFullHashPath(settingsInterface.GetHydraAbsDataFolderPath(), testAllServers, testMaxDepth, testMaxFolderPerLevel)
		chroniclerInterface := chronicler.New(hashPath, maxFileSize, testMaxDepth, fsInterface, metadata.New(hashPath))
		chroniclerInterface.CreateDirectoryIfNotExists()

		swampEventCallbackFunc := func(e *Event) {}

		closeCallbackFunc := func(n name.Name) {}

		swampInfoCallbackFunc := func(i *Info) {}

		fssSwamp := &FilesystemSettings{
			ChroniclerInterface: chroniclerInterface,
			WriteInterval:       writeInterval,
		}

		metadataInterface := metadata.New(hashPath)
		swampInterface := New(swampName, closeAfterIdle, fssSwamp, swampEventCallbackFunc, swampInfoCallbackFunc, closeCallbackFunc, metadataInterface)
		swampInterface.BeginVigil()

		for i := 0; i < allTests; i++ {
			treasureInterface := swampInterface.CreateTreasure(fmt.Sprintf("%d", i))
			if treasureInterface == nil {
				t.Errorf("treasureInterface should not be nil")
			}
			guardID := treasureInterface.StartTreasureGuard(true)
			treasureInterface.SetContentInt64(guardID, int64(i))
			treasureInterface.ReleaseTreasureGuard(guardID)

			guardID = treasureInterface.StartTreasureGuard(true)
			_ = treasureInterface.Save(guardID)
			treasureInterface.ReleaseTreasureGuard(guardID)

		}

		receivedTreasures, err := swampInterface.GetTreasuresByBeacon(BeaconTypeValueInt64, IndexOrderAsc, 0, 10)
		assert.Nil(t, err, "error should be nil")
		assert.Equal(t, allTests, len(receivedTreasures), "treasures should be 10")

		lastID := 0
		for _, tr := range receivedTreasures {
			i, _ := tr.GetContentInt64()
			assert.Nil(t, err, "error should be nil")
			assert.Equal(t, int64(lastID), i, "key should be in order")
			lastID++
		}

		receivedTreasures, err = swampInterface.GetTreasuresByBeacon(BeaconTypeValueInt64, IndexOrderDesc, 0, 10)
		assert.Nil(t, err, "error should be nil")
		assert.Equal(t, allTests, len(receivedTreasures), "treasures should be 10")

		lastID = 9
		for _, tr := range receivedTreasures {
			keyInt, err := strconv.Atoi(tr.GetKey())
			assert.Nil(t, err, "error should be nil")
			assert.Equal(t, lastID, keyInt, "key should be in order")
			lastID--
		}

		swampInterface.CeaseVigil()
		swampInterface.Destroy()

	})

	t.Run("should get treasures by the float beacon", func(t *testing.T) {

		allTests := 10

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("should-get-treasure").Swamp("by-float-beacon")

		hashPath := swampName.GetFullHashPath(settingsInterface.GetHydraAbsDataFolderPath(), testAllServers, testMaxDepth, testMaxFolderPerLevel)
		chroniclerInterface := chronicler.New(hashPath, maxFileSize, testMaxDepth, fsInterface, metadata.New(hashPath))
		chroniclerInterface.CreateDirectoryIfNotExists()

		swampEventCallbackFunc := func(e *Event) {}

		closeCallbackFunc := func(n name.Name) {}

		swampInfoCallbackFunc := func(i *Info) {}

		fssSwamp := &FilesystemSettings{
			ChroniclerInterface: chroniclerInterface,
			WriteInterval:       writeInterval,
		}

		metadataInterface := metadata.New(hashPath)
		swampInterface := New(swampName, closeAfterIdle, fssSwamp, swampEventCallbackFunc, swampInfoCallbackFunc, closeCallbackFunc, metadataInterface)
		swampInterface.BeginVigil()

		for i := 0; i < allTests; i++ {
			treasureInterface := swampInterface.CreateTreasure(fmt.Sprintf("%d", i))
			if treasureInterface == nil {
				t.Errorf("treasureInterface should not be nil")
			}
			guardID := treasureInterface.StartTreasureGuard(true)
			treasureInterface.SetContentFloat64(guardID, 0.12+float64(i))
			treasureInterface.ReleaseTreasureGuard(guardID)

			guardID = treasureInterface.StartTreasureGuard(true)
			_ = treasureInterface.Save(guardID)
			treasureInterface.ReleaseTreasureGuard(guardID)

		}

		receivedTreasures, err := swampInterface.GetTreasuresByBeacon(BeaconTypeValueFloat64, IndexOrderAsc, 0, 10)
		assert.Nil(t, err, "error should be nil")
		assert.Equal(t, allTests, len(receivedTreasures), "treasures should be 10")

		lastID := 0
		for _, tr := range receivedTreasures {
			keyInt, err := strconv.Atoi(tr.GetKey())
			assert.Nil(t, err, "error should be nil")
			assert.Equal(t, lastID, keyInt, "key should be in order")
			lastID++
		}

		receivedTreasures, err = swampInterface.GetTreasuresByBeacon(BeaconTypeValueFloat64, IndexOrderDesc, 0, 10)
		assert.Nil(t, err, "error should be nil")
		assert.Equal(t, allTests, len(receivedTreasures), "treasures should be 10")

		lastID = 9
		for _, tr := range receivedTreasures {
			keyInt, err := strconv.Atoi(tr.GetKey())
			assert.Nil(t, err, "error should be nil")
			assert.Equal(t, lastID, keyInt, "key should be in order")
			lastID--
		}

		swampInterface.CeaseVigil()
		swampInterface.Destroy()

	})

	t.Run("should get treasures by the expiration time beacon", func(t *testing.T) {

		allTests := 10

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("should-get-treasure").Swamp("by-expiration-time-beacon")

		hashPath := swampName.GetFullHashPath(settingsInterface.GetHydraAbsDataFolderPath(), testAllServers, testMaxDepth, testMaxFolderPerLevel)
		chroniclerInterface := chronicler.New(hashPath, maxFileSize, testMaxDepth, fsInterface, metadata.New(hashPath))
		chroniclerInterface.CreateDirectoryIfNotExists()

		swampEventCallbackFunc := func(e *Event) {}

		closeCallbackFunc := func(n name.Name) {}

		swampInfoCallbackFunc := func(i *Info) {}

		fssSwamp := &FilesystemSettings{
			ChroniclerInterface: chroniclerInterface,
			WriteInterval:       writeInterval,
		}

		metadataInterface := metadata.New(hashPath)
		swampInterface := New(swampName, closeAfterIdle, fssSwamp, swampEventCallbackFunc, swampInfoCallbackFunc, closeCallbackFunc, metadataInterface)
		swampInterface.BeginVigil()

		for i := 0; i < allTests; i++ {
			treasureInterface := swampInterface.CreateTreasure(fmt.Sprintf("%d", i))
			if treasureInterface == nil {
				t.Errorf("treasureInterface should not be nil")
			}
			guardID := treasureInterface.StartTreasureGuard(true)
			treasureInterface.SetContentFloat64(guardID, 0.12+float64(i))
			treasureInterface.SetExpirationTime(guardID, time.Now().Add(-time.Second*time.Duration(i)))
			treasureInterface.ReleaseTreasureGuard(guardID)

			guardID = treasureInterface.StartTreasureGuard(true)
			_ = treasureInterface.Save(guardID)
			treasureInterface.ReleaseTreasureGuard(guardID)

			time.Sleep(time.Millisecond * 10)
		}

		receivedTreasures, err := swampInterface.GetTreasuresByBeacon(BeaconTypeExpirationTime, IndexOrderAsc, 0, 10)
		assert.Nil(t, err, "error should be nil")
		assert.Equal(t, allTests, len(receivedTreasures), "treasures should be 10")

		lastID := 9
		for _, tr := range receivedTreasures {
			keyInt, err := strconv.Atoi(tr.GetKey())
			assert.Nil(t, err, "error should be nil")
			assert.Equal(t, lastID, keyInt, "key should be in order")
			lastID--
		}

		receivedTreasures, err = swampInterface.GetTreasuresByBeacon(BeaconTypeExpirationTime, IndexOrderDesc, 0, 10)
		assert.Nil(t, err, "error should be nil")
		assert.Equal(t, allTests, len(receivedTreasures), "treasures should be 10")

		lastID = 0
		for _, tr := range receivedTreasures {
			keyInt, err := strconv.Atoi(tr.GetKey())
			assert.Nil(t, err, "error should be nil")
			assert.Equal(t, lastID, keyInt, "key should be in order")
			lastID++
		}

		swampInterface.CeaseVigil()
		swampInterface.Destroy()

	})

	t.Run("should get treasures from the beacon after deleting some treasures", func(t *testing.T) {

		allTests := 10

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("should-get-treasure-from-beacon").Swamp("after-deleting-some-treasures")

		hashPath := swampName.GetFullHashPath(settingsInterface.GetHydraAbsDataFolderPath(), testAllServers, testMaxDepth, testMaxFolderPerLevel)
		chroniclerInterface := chronicler.New(hashPath, maxFileSize, testMaxDepth, fsInterface, metadata.New(hashPath))
		chroniclerInterface.CreateDirectoryIfNotExists()

		swampEventCallbackFunc := func(e *Event) {}

		closeCallbackFunc := func(n name.Name) {}

		swampInfoCallbackFunc := func(i *Info) {}

		fssSwamp := &FilesystemSettings{
			ChroniclerInterface: chroniclerInterface,
			WriteInterval:       writeInterval,
		}

		metadataInterface := metadata.New(hashPath)
		swampInterface := New(swampName, closeAfterIdle, fssSwamp, swampEventCallbackFunc, swampInfoCallbackFunc, closeCallbackFunc, metadataInterface)
		swampInterface.BeginVigil()
		defer swampInterface.CeaseVigil()

		defaultTime := time.Now()

		// set treasures for the swamp
		for i := 0; i < allTests; i++ {

			func() {

				treasureInterface := swampInterface.CreateTreasure(fmt.Sprintf("%d", i))
				if treasureInterface == nil {
					t.Errorf("treasureInterface should not be nil")
				}

				guardID := treasureInterface.StartTreasureGuard(true)
				defer treasureInterface.ReleaseTreasureGuard(guardID)

				treasureInterface.SetCreatedAt(guardID, defaultTime.Add(time.Duration(i)*time.Nanosecond))
				treasureInterface.SetContentString(guardID, fmt.Sprintf("content-%d", i))
				_ = treasureInterface.Save(guardID)

			}()

		}

		// try to get all treasures back from the creation time beacon
		allTreasures, err := swampInterface.GetTreasuresByBeacon(BeaconTypeCreationTime, IndexOrderAsc, 0, 100000)
		assert.NoError(t, err, "error should be nil")
		assert.Equal(t, allTests, len(allTreasures), "treasures should be 10")

		// delete 1 treasure from the swamp with key 3
		_ = swampInterface.DeleteTreasure("3", false)

		// try to get all treasures back from the creation time beacon
		allTreasures, err = swampInterface.GetTreasuresByBeacon(BeaconTypeCreationTime, IndexOrderAsc, 0, 100000)
		assert.NoError(t, err, "error should be nil")
		assert.Equal(t, allTests-1, len(allTreasures), "treasures should be 8")

	})

	t.Run("should get treasures from beacon after the swamp closed, treasure deleted then got from the beacon", func(t *testing.T) {

		allTests := 10

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("should-get-treasure-from-beacon").Swamp("after-swamp-closed")

		hashPath := swampName.GetFullHashPath(settingsInterface.GetHydraAbsDataFolderPath(), testAllServers, testMaxDepth, testMaxFolderPerLevel)
		chroniclerInterface := chronicler.New(hashPath, maxFileSize, testMaxDepth, fsInterface, metadata.New(hashPath))
		chroniclerInterface.CreateDirectoryIfNotExists()

		swampEventCallbackFunc := func(e *Event) {}

		closeCallbackFunc := func(n name.Name) {}

		swampInfoCallbackFunc := func(i *Info) {}

		fssSwamp := &FilesystemSettings{
			ChroniclerInterface: chroniclerInterface,
			WriteInterval:       writeInterval,
		}

		metadataInterface := metadata.New(hashPath)
		swampInterface := New(swampName, closeAfterIdle, fssSwamp, swampEventCallbackFunc, swampInfoCallbackFunc, closeCallbackFunc, metadataInterface)
		swampInterface.BeginVigil()
		defer swampInterface.CeaseVigil()

		defaultTime := time.Now()

		// set treasures for the swamp
		for i := 0; i < allTests; i++ {

			func() {

				treasureInterface := swampInterface.CreateTreasure(fmt.Sprintf("%d", i))
				if treasureInterface == nil {
					t.Errorf("treasureInterface should not be nil")
				}

				guardID := treasureInterface.StartTreasureGuard(true)
				defer treasureInterface.ReleaseTreasureGuard(guardID)

				treasureInterface.SetCreatedAt(guardID, defaultTime.Add(time.Duration(i)*time.Nanosecond))
				treasureInterface.SetContentString(guardID, fmt.Sprintf("content-%d", i))
				_ = treasureInterface.Save(guardID)

			}()

		}

		// wait for the swamp to close/write all treasures to the filesystem
		time.Sleep(3 * time.Second)

		fssSwamp = &FilesystemSettings{
			ChroniclerInterface: chroniclerInterface,
			WriteInterval:       writeInterval,
		}

		// create a new swamp with the same name and simulate the re-summoning of the swamp
		metadataInterface = metadata.New(hashPath)
		swampInterface = New(swampName, closeAfterIdle, fssSwamp, swampEventCallbackFunc, swampInfoCallbackFunc, closeCallbackFunc, metadataInterface)
		swampInterface.BeginVigil()
		defer swampInterface.CeaseVigil()

		// delete 1 treasure from the swamp with key 3
		_ = swampInterface.DeleteTreasure("3", false)

		// try to get all treasures back from the creation time beacon
		allTreasures, err := swampInterface.GetTreasuresByBeacon(BeaconTypeCreationTime, IndexOrderAsc, 0, 100000)
		assert.NoError(t, err, "error should be nil")
		assert.Equal(t, allTests-1, len(allTreasures), "treasures should be 9")

	})

}

func TestIncrementUint8_NewKey_NoMetadata(t *testing.T) {

	s := newSwampForTest(t, "NewKey", "NoMetadata")

	newVal, inc, meta, err := s.IncrementUint8(
		"counter:new:no-meta",
		5,
		nil,
		nil, // metadataIfNotExist
		nil, // metadataIfExist
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc {
		t.Fatalf("expected incremented=true")
	}
	if newVal != 5 {
		t.Fatalf("expected newVal=5, got %d", newVal)
	}
	if meta != nil {
		t.Fatalf("expected metadataResponse=nil for no metadata, got: %#v", meta)
	}

}
func TestIncrementUint8_NewKey_WithMetadataIfNotExist(t *testing.T) {
	s := newSwampForTest(t, "NewKey", "WithMetadataIfNotExist")

	before := time.Now().UTC()
	metaReq := &IncrementMetadataRequest{
		CreatedAt: true,
		CreatedBy: "alice",
		UpdatedAt: true,
		UpdatedBy: "alice",
		ExpiredAt: before.Add(10 * time.Minute),
	}

	newVal, inc, meta, err := s.IncrementUint8(
		"counter:new:with-meta",
		1,
		nil,
		metaReq,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 1 {
		t.Fatalf("bad increment result (inc=%v, newVal=%d)", inc, newVal)
	}
	if meta == nil {
		t.Fatalf("expected metadataResponse not nil")
	}

	// laza ellenőrzés az időkre (nem pontosan now, de > before)
	if meta.CreatedAt.Before(before) {
		t.Fatalf("CreatedAt not set correctly: %v <= %v", meta.CreatedAt, before)
	}
	if meta.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt not set correctly: %v <= %v", meta.UpdatedAt, before)
	}
	if meta.CreatedBy != "alice" || meta.UpdatedBy != "alice" {
		t.Fatalf("bad CreatedBy/UpdatedBy: %+v", meta)
	}
	if meta.ExpiredAt.IsZero() {
		t.Fatalf("ExpiredAt should be set")
	}
}
func TestIncrementUint8_ExistingKey_NoMetadata(t *testing.T) {
	s := newSwampForTest(t, "ExistingKey", "NoMetadata")

	// előkészítés: legyen létező uint8 érték
	key := "counter:exist:no-meta"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentUint8(guardID, 10)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	newVal, inc, meta, err := s.IncrementUint8(
		key,
		7,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 17 {
		t.Fatalf("expected 10+7=17, got inc=%v newVal=%d", inc, newVal)
	}
	if meta != nil {
		t.Fatalf("expected metadataResponse=nil when no metadata provided")
	}
}
func TestIncrementUint8_ExistingKey_WithMetadataIfExist(t *testing.T) {
	s := newSwampForTest(t, "ExistingKey", "WithMetadataIfExist")

	key := "counter:exist:with-meta"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentUint8(guardID, 3)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	before := time.Now().UTC()
	metaReq := &IncrementMetadataRequest{
		UpdatedAt: true,
		UpdatedBy: "bob",
		// direkt nem állítunk Created* mezőket, mert létező elemről beszélünk
	}

	newVal, inc, meta, err := s.IncrementUint8(
		key,
		2,
		nil,
		nil,     // ifNotExist
		metaReq, // ifExist
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 5 {
		t.Fatalf("expected 3+2=5, got %d", newVal)
	}
	if meta == nil {
		t.Fatalf("metadataResponse must not be nil when metadataIfExist is provided")
	}
	// BUG DETECTOR:
	// Ha a kódban rossz a hívás és ifExist helyett ifNotExist-et ad át,
	// akkor UpdatedAt/UpdatedBy nem frissül. Ez a teszt ilyenkor FAIL-el.
	if meta.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt was not updated for existing treasure (did you pass metadataRequestIfExist to setMetaForIncrement?)")
	}
	if meta.UpdatedBy != "bob" {
		t.Fatalf("UpdatedBy not set to 'bob': %+v", meta)
	}

}
func TestIncrementUint8_ConditionBlocksIncrement(t *testing.T) {
	s := newSwampForTest(t, "ConditionBlocks", "Increment")

	key := "counter:cond:block"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentUint8(guardID, 42)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	cond := &IncrementUInt8Condition{
		RelationalOperator: RelationalOperatorGreaterThan, // csak ha > 50 növelnénk
		Value:              50,
	}

	newVal, inc, meta, err := s.IncrementUint8(
		key,
		1,
		cond,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inc {
		t.Fatalf("expected incremented=false due to condition")
	}
	if newVal != 42 {
		t.Fatalf("value must remain 42, got %d", newVal)
	}
	if meta != nil {
		t.Fatalf("metadataResponse should be nil when nothing changed")
	}
}
func TestIncrementUint8_WrongType_Error(t *testing.T) {
	s := newSwampForTest(t, "WrongType", "Error")

	// előkészítés: hozzunk létre treasure-t, ami NEM uint8 típusú
	key := "counter:wrong:type"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	// szimuláljuk a rossz típust: pl. SetContentUint8 helyett állítsunk
	// más tartalomtípust (ezt a konkrét projekted API-ja szerint csináld).
	tr.SetContentString(guardID, "not-an-uint8")
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	_, inc, _, err := s.IncrementUint8(
		key,
		1,
		nil,
		nil,
		nil,
	)
	if err == nil {
		t.Fatalf("expected error for wrong content type")
	}
	if inc {
		t.Fatalf("should not have incremented")
	}
	if !errors.Is(err, errors.New(ErrorValueIsNotInt)) && err.Error() != ErrorValueIsNotInt {
		t.Fatalf("unexpected error, got=%v want=%s", err, ErrorValueIsNotInt)
	}
}
func TestIncrementUint8_OverflowWraps(t *testing.T) {
	s := newSwampForTest(t, "Overflow", "Wraps")

	key := "counter:overflow"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentUint8(guardID, 250)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	newVal, inc, _, err := s.IncrementUint8(
		key,
		10, // 250 + 10 = 260 -> uint8 wrap -> 4
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc {
		t.Fatalf("expected increment")
	}
	if newVal != 4 {
		t.Fatalf("expected wrap to 4, got %d", newVal)
	}
}

func TestIncrementUint16_NewKey_NoMetadata(t *testing.T) {
	s := newSwampForTest(t, "Uint16", "NewKey_NoMetadata")

	newVal, inc, meta, err := s.IncrementUint16(
		"u16:new:no-meta",
		5,
		nil,
		nil, // metadataIfNotExist
		nil, // metadataIfExist
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc {
		t.Fatalf("expected incremented=true")
	}
	if newVal != 5 {
		t.Fatalf("expected newVal=5, got %d", newVal)
	}
	if meta != nil {
		t.Fatalf("expected metadataResponse=nil for no metadata, got: %#v", meta)
	}
}
func TestIncrementUint16_NewKey_WithMetadataIfNotExist(t *testing.T) {
	s := newSwampForTest(t, "Uint16", "NewKey_WithMetadataIfNotExist")

	before := time.Now().UTC()
	metaReq := &IncrementMetadataRequest{
		CreatedAt: true,
		CreatedBy: "alice",
		UpdatedAt: true,
		UpdatedBy: "alice",
		ExpiredAt: before.Add(10 * time.Minute),
	}

	newVal, inc, meta, err := s.IncrementUint16(
		"u16:new:with-meta",
		1,
		nil,
		metaReq, // ifNotExist
		nil,     // ifExist
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 1 {
		t.Fatalf("bad increment result (inc=%v, newVal=%d)", inc, newVal)
	}
	if meta == nil {
		t.Fatalf("expected metadataResponse not nil")
	}
	if meta.CreatedAt.Before(before) {
		t.Fatalf("CreatedAt not set correctly: %v <= %v", meta.CreatedAt, before)
	}
	if meta.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt not set correctly: %v <= %v", meta.UpdatedAt, before)
	}
	if meta.CreatedBy != "alice" || meta.UpdatedBy != "alice" {
		t.Fatalf("bad CreatedBy/UpdatedBy: %+v", meta)
	}
	if meta.ExpiredAt.IsZero() {
		t.Fatalf("ExpiredAt should be set")
	}
}
func TestIncrementUint16_ExistingKey_NoMetadata(t *testing.T) {
	s := newSwampForTest(t, "Uint16", "ExistingKey_NoMetadata")

	key := "u16:exist:no-meta"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentUint16(guardID, 10)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	newVal, inc, meta, err := s.IncrementUint16(
		key,
		7,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 17 {
		t.Fatalf("expected 10+7=17, got inc=%v newVal=%d", inc, newVal)
	}
	if meta != nil {
		t.Fatalf("expected metadataResponse=nil when no metadata provided")
	}
}
func TestIncrementUint16_ExistingKey_WithMetadataIfExist(t *testing.T) {
	s := newSwampForTest(t, "Uint16", "ExistingKey_WithMetadataIfExist")

	key := "u16:exist:with-meta"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentUint16(guardID, 3)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	before := time.Now().UTC()
	metaReq := &IncrementMetadataRequest{
		UpdatedAt: true,
		UpdatedBy: "bob",
	}

	newVal, inc, meta, err := s.IncrementUint16(
		key,
		2,
		nil,
		nil,     // ifNotExist
		metaReq, // ifExist
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 5 {
		t.Fatalf("expected 3+2=5, got %d", newVal)
	}
	if meta == nil {
		t.Fatalf("metadataResponse must not be nil when metadataIfExist is provided")
	}
	if meta.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt was not updated for existing treasure (did you pass metadataRequestIfExist to setMetaForIncrement?)")
	}
	if meta.UpdatedBy != "bob" {
		t.Fatalf("UpdatedBy not set to 'bob': %+v", meta)
	}
}
func TestIncrementUint16_ConditionBlocksIncrement(t *testing.T) {
	s := newSwampForTest(t, "Uint16", "ConditionBlocksIncrement")

	key := "u16:cond:block"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentUint16(guardID, 42)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	cond := &IncrementUInt16Condition{
		RelationalOperator: RelationalOperatorGreaterThan, // csak ha > 50 növelnénk
		Value:              50,
	}

	newVal, inc, meta, err := s.IncrementUint16(
		key,
		1,
		cond,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inc {
		t.Fatalf("expected incremented=false due to condition")
	}
	if newVal != 42 {
		t.Fatalf("value must remain 42, got %d", newVal)
	}
	if meta != nil {
		t.Fatalf("metadataResponse should be nil when nothing changed")
	}
}
func TestIncrementUint16_WrongType_Error(t *testing.T) {
	s := newSwampForTest(t, "Uint16", "WrongType_Error")

	key := "u16:wrong:type"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	// állítsunk szándékosan más típusú tartalmat
	tr.SetContentString(guardID, "not-an-uint16")
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	_, inc, _, err := s.IncrementUint16(
		key,
		1,
		nil,
		nil,
		nil,
	)
	if err == nil {
		t.Fatalf("expected error for wrong content type")
	}
	if inc {
		t.Fatalf("should not have incremented")
	}
	if !errors.Is(err, errors.New(ErrorValueIsNotInt)) && err.Error() != ErrorValueIsNotInt {
		t.Fatalf("unexpected error, got=%v want=%s", err, ErrorValueIsNotInt)
	}
}
func TestIncrementUint16_OverflowWraps(t *testing.T) {
	s := newSwampForTest(t, "Uint16", "OverflowWraps")

	key := "u16:overflow"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentUint16(guardID, 65530)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	newVal, inc, _, err := s.IncrementUint16(
		key,
		10, // 65530 + 10 = 65540 -> uint16 wrap -> 4
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc {
		t.Fatalf("expected increment")
	}
	if newVal != 4 {
		t.Fatalf("expected wrap to 4, got %d", newVal)
	}
}

func TestIncrementUint32_NewKey_NoMetadata(t *testing.T) {
	s := newSwampForTest(t, "Uint32", "NewKey_NoMetadata")

	newVal, inc, meta, err := s.IncrementUint32(
		"u32:new:no-meta",
		5,
		nil,
		nil,
		nil,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc {
		t.Fatalf("expected incremented=true")
	}
	if newVal != 5 {
		t.Fatalf("expected newVal=5, got %d", newVal)
	}
	if meta != nil {
		t.Fatalf("expected metadataResponse=nil for no metadata, got: %#v", meta)
	}
}
func TestIncrementUint32_NewKey_WithMetadataIfNotExist(t *testing.T) {
	s := newSwampForTest(t, "Uint32", "NewKey_WithMetadataIfNotExist")

	before := time.Now().UTC()
	metaReq := &IncrementMetadataRequest{
		CreatedAt: true,
		CreatedBy: "alice",
		UpdatedAt: true,
		UpdatedBy: "alice",
		ExpiredAt: before.Add(10 * time.Minute),
	}

	newVal, inc, meta, err := s.IncrementUint32(
		"u32:new:with-meta",
		1,
		nil,
		metaReq,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 1 {
		t.Fatalf("bad increment result (inc=%v, newVal=%d)", inc, newVal)
	}
	if meta == nil {
		t.Fatalf("expected metadataResponse not nil")
	}
	if meta.CreatedAt.Before(before) {
		t.Fatalf("CreatedAt not set correctly: %v <= %v", meta.CreatedAt, before)
	}
	if meta.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt not set correctly: %v <= %v", meta.UpdatedAt, before)
	}
	if meta.CreatedBy != "alice" || meta.UpdatedBy != "alice" {
		t.Fatalf("bad CreatedBy/UpdatedBy: %+v", meta)
	}
	if meta.ExpiredAt.IsZero() {
		t.Fatalf("ExpiredAt should be set")
	}
}
func TestIncrementUint32_ExistingKey_NoMetadata(t *testing.T) {
	s := newSwampForTest(t, "Uint32", "ExistingKey_NoMetadata")

	key := "u32:exist:no-meta"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentUint32(guardID, 10)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	newVal, inc, meta, err := s.IncrementUint32(
		key,
		7,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 17 {
		t.Fatalf("expected 10+7=17, got inc=%v newVal=%d", inc, newVal)
	}
	if meta != nil {
		t.Fatalf("expected metadataResponse=nil when no metadata provided")
	}
}
func TestIncrementUint32_ExistingKey_WithMetadataIfExist(t *testing.T) {
	s := newSwampForTest(t, "Uint32", "ExistingKey_WithMetadataIfExist")

	key := "u32:exist:with-meta"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentUint32(guardID, 3)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	before := time.Now().UTC()
	metaReq := &IncrementMetadataRequest{
		UpdatedAt: true,
		UpdatedBy: "bob",
	}

	newVal, inc, meta, err := s.IncrementUint32(
		key,
		2,
		nil,
		nil,
		metaReq,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 5 {
		t.Fatalf("expected 3+2=5, got %d", newVal)
	}
	if meta == nil {
		t.Fatalf("metadataResponse must not be nil when metadataIfExist is provided")
	}
	if meta.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt was not updated for existing treasure (did you pass metadataRequestIfExist to setMetaForIncrement?)")
	}
	if meta.UpdatedBy != "bob" {
		t.Fatalf("UpdatedBy not set to 'bob': %+v", meta)
	}
}
func TestIncrementUint32_ConditionBlocksIncrement(t *testing.T) {
	s := newSwampForTest(t, "Uint32", "ConditionBlocksIncrement")

	key := "u32:cond:block"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentUint32(guardID, 42)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	cond := &IncrementUInt32Condition{
		RelationalOperator: RelationalOperatorGreaterThan,
		Value:              50,
	}

	newVal, inc, meta, err := s.IncrementUint32(
		key,
		1,
		cond,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inc {
		t.Fatalf("expected incremented=false due to condition")
	}
	if newVal != 42 {
		t.Fatalf("value must remain 42, got %d", newVal)
	}
	if meta != nil {
		t.Fatalf("metadataResponse should be nil when nothing changed")
	}
}
func TestIncrementUint32_WrongType_Error(t *testing.T) {
	s := newSwampForTest(t, "Uint32", "WrongType_Error")

	key := "u32:wrong:type"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentString(guardID, "not-an-uint32")
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	_, inc, _, err := s.IncrementUint32(
		key,
		1,
		nil,
		nil,
		nil,
	)
	if err == nil {
		t.Fatalf("expected error for wrong content type")
	}
	if inc {
		t.Fatalf("should not have incremented")
	}
	if !errors.Is(err, errors.New(ErrorValueIsNotInt)) && err.Error() != ErrorValueIsNotInt {
		t.Fatalf("unexpected error, got=%v want=%s", err, ErrorValueIsNotInt)
	}
}
func TestIncrementUint32_OverflowWraps(t *testing.T) {
	s := newSwampForTest(t, "Uint32", "OverflowWraps")

	key := "u32:overflow"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentUint32(guardID, math.MaxUint32-5)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	newVal, inc, _, err := s.IncrementUint32(
		key,
		10,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc {
		t.Fatalf("expected increment")
	}
	expected := (math.MaxUint32 - 5) + 10
	if newVal != uint32(expected) {
		t.Fatalf("expected wrap to %d, got %d", int32(expected), newVal)
	}

}

func TestIncrementUint64_NewKey_NoMetadata(t *testing.T) {
	s := newSwampForTest(t, "Uint64", "NewKey_NoMetadata")

	newVal, inc, meta, err := s.IncrementUint64(
		"u64:new:no-meta",
		5,
		nil,
		nil,
		nil,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc {
		t.Fatalf("expected incremented=true")
	}
	if newVal != 5 {
		t.Fatalf("expected newVal=5, got %d", newVal)
	}
	if meta != nil {
		t.Fatalf("expected metadataResponse=nil for no metadata, got: %#v", meta)
	}
}
func TestIncrementUint64_NewKey_WithMetadataIfNotExist(t *testing.T) {
	s := newSwampForTest(t, "Uint64", "NewKey_WithMetadataIfNotExist")

	before := time.Now().UTC()
	metaReq := &IncrementMetadataRequest{
		CreatedAt: true,
		CreatedBy: "alice",
		UpdatedAt: true,
		UpdatedBy: "alice",
		ExpiredAt: before.Add(10 * time.Minute),
	}

	newVal, inc, meta, err := s.IncrementUint64(
		"u64:new:with-meta",
		1,
		nil,
		metaReq,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 1 {
		t.Fatalf("bad increment result (inc=%v, newVal=%d)", inc, newVal)
	}
	if meta == nil {
		t.Fatalf("expected metadataResponse not nil")
	}
	if meta.CreatedAt.Before(before) {
		t.Fatalf("CreatedAt not set correctly: %v <= %v", meta.CreatedAt, before)
	}
	if meta.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt not set correctly: %v <= %v", meta.UpdatedAt, before)
	}
	if meta.CreatedBy != "alice" || meta.UpdatedBy != "alice" {
		t.Fatalf("bad CreatedBy/UpdatedBy: %+v", meta)
	}
	if meta.ExpiredAt.IsZero() {
		t.Fatalf("ExpiredAt should be set")
	}
}
func TestIncrementUint64_ExistingKey_NoMetadata(t *testing.T) {
	s := newSwampForTest(t, "Uint64", "ExistingKey_NoMetadata")

	key := "u64:exist:no-meta"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentUint64(guardID, 10)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	newVal, inc, meta, err := s.IncrementUint64(
		key,
		7,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 17 {
		t.Fatalf("expected 10+7=17, got inc=%v newVal=%d", inc, newVal)
	}
	if meta != nil {
		t.Fatalf("expected metadataResponse=nil when no metadata provided")
	}
}
func TestIncrementUint64_ExistingKey_WithMetadataIfExist(t *testing.T) {
	s := newSwampForTest(t, "Uint64", "ExistingKey_WithMetadataIfExist")

	key := "u64:exist:with-meta"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentUint64(guardID, 3)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	before := time.Now().UTC()
	metaReq := &IncrementMetadataRequest{
		UpdatedAt: true,
		UpdatedBy: "bob",
	}

	newVal, inc, meta, err := s.IncrementUint64(
		key,
		2,
		nil,
		nil,
		metaReq,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 5 {
		t.Fatalf("expected 3+2=5, got %d", newVal)
	}
	if meta == nil {
		t.Fatalf("metadataResponse must not be nil when metadataIfExist is provided")
	}
	if meta.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt was not updated for existing treasure (did you pass metadataRequestIfExist to setMetaForIncrement?)")
	}
	if meta.UpdatedBy != "bob" {
		t.Fatalf("UpdatedBy not set to 'bob': %+v", meta)
	}
}
func TestIncrementUint64_ConditionBlocksIncrement(t *testing.T) {
	s := newSwampForTest(t, "Uint64", "ConditionBlocksIncrement")

	key := "u64:cond:block"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentUint64(guardID, 42)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	cond := &IncrementUInt64Condition{
		RelationalOperator: RelationalOperatorGreaterThan,
		Value:              50,
	}

	newVal, inc, meta, err := s.IncrementUint64(
		key,
		1,
		cond,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inc {
		t.Fatalf("expected incremented=false due to condition")
	}
	if newVal != 42 {
		t.Fatalf("value must remain 42, got %d", newVal)
	}
	if meta != nil {
		t.Fatalf("metadataResponse should be nil when nothing changed")
	}
}
func TestIncrementUint64_WrongType_Error(t *testing.T) {
	s := newSwampForTest(t, "Uint64", "WrongType_Error")

	key := "u64:wrong:type"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentString(guardID, "not-an-uint64")
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	_, inc, _, err := s.IncrementUint64(
		key,
		1,
		nil,
		nil,
		nil,
	)
	if err == nil {
		t.Fatalf("expected error for wrong content type")
	}
	if inc {
		t.Fatalf("should not have incremented")
	}
	if !errors.Is(err, errors.New(ErrorValueIsNotInt)) && err.Error() != ErrorValueIsNotInt {
		t.Fatalf("unexpected error, got=%v want=%s", err, ErrorValueIsNotInt)
	}
}
func TestIncrementUint64_OverflowWraps(t *testing.T) {
	s := newSwampForTest(t, "Uint64", "OverflowWraps")

	key := "u64:overflow"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentUint64(guardID, math.MaxUint64-5)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	newVal, inc, _, err := s.IncrementUint64(
		key,
		10,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc {
		t.Fatalf("expected increment")
	}

	// >>> Itt a lényeg: ne konstans kifejezést használj
	start := uint64(math.MaxUint64 - 5)
	delta := uint64(10)
	expected := start + delta // futásidőben wrap-el 4-re

	if newVal != expected {
		t.Fatalf("expected wrap to %d, got %d", expected, newVal)
	}
}

func TestIncrementInt8_NewKey_NoMetadata(t *testing.T) {
	s := newSwampForTest(t, "Int8", "NewKey_NoMetadata")

	newVal, inc, meta, err := s.IncrementInt8(
		"i8:new:no-meta",
		5,
		nil,
		nil, // metadataIfNotExist
		nil, // metadataIfExist
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc {
		t.Fatalf("expected incremented=true")
	}
	if newVal != 5 {
		t.Fatalf("expected newVal=5, got %d", newVal)
	}
	if meta != nil {
		t.Fatalf("expected metadataResponse=nil for no metadata, got: %#v", meta)
	}
}
func TestIncrementInt8_NewKey_WithMetadataIfNotExist(t *testing.T) {
	s := newSwampForTest(t, "Int8", "NewKey_WithMetadataIfNotExist")

	before := time.Now().UTC()
	metaReq := &IncrementMetadataRequest{
		CreatedAt: true,
		CreatedBy: "alice",
		UpdatedAt: true,
		UpdatedBy: "alice",
		ExpiredAt: before.Add(10 * time.Minute),
	}

	newVal, inc, meta, err := s.IncrementInt8(
		"i8:new:with-meta",
		1,
		nil,
		metaReq, // ifNotExist
		nil,     // ifExist
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 1 {
		t.Fatalf("bad increment result (inc=%v, newVal=%d)", inc, newVal)
	}
	if meta == nil {
		t.Fatalf("expected metadataResponse not nil")
	}
	if meta.CreatedAt.Before(before) {
		t.Fatalf("CreatedAt not set correctly: %v <= %v", meta.CreatedAt, before)
	}
	if meta.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt not set correctly: %v <= %v", meta.UpdatedAt, before)
	}
	if meta.CreatedBy != "alice" || meta.UpdatedBy != "alice" {
		t.Fatalf("bad CreatedBy/UpdatedBy: %+v", meta)
	}
	if meta.ExpiredAt.IsZero() {
		t.Fatalf("ExpiredAt should be set")
	}
}
func TestIncrementInt8_ExistingKey_NoMetadata(t *testing.T) {
	s := newSwampForTest(t, "Int8", "ExistingKey_NoMetadata")

	key := "i8:exist:no-meta"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentInt8(guardID, 10)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	newVal, inc, meta, err := s.IncrementInt8(
		key,
		7,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 17 {
		t.Fatalf("expected 10+7=17, got inc=%v newVal=%d", inc, newVal)
	}
	if meta != nil {
		t.Fatalf("expected metadataResponse=nil when no metadata provided")
	}
}
func TestIncrementInt8_ExistingKey_WithMetadataIfExist(t *testing.T) {
	s := newSwampForTest(t, "Int8", "ExistingKey_WithMetadataIfExist")

	key := "i8:exist:with-meta"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentInt8(guardID, 3)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	before := time.Now().UTC()
	metaReq := &IncrementMetadataRequest{
		UpdatedAt: true,
		UpdatedBy: "bob",
	}

	newVal, inc, meta, err := s.IncrementInt8(
		key,
		2,
		nil,
		nil,     // ifNotExist
		metaReq, // ifExist
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 5 {
		t.Fatalf("expected 3+2=5, got %d", newVal)
	}
	if meta == nil {
		t.Fatalf("metadataResponse must not be nil when metadataIfExist is provided")
	}
	if meta.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt was not updated for existing treasure (did you pass metadataRequestIfExist to setMetaForIncrement?)")
	}
	if meta.UpdatedBy != "bob" {
		t.Fatalf("UpdatedBy not set to 'bob': %+v", meta)
	}
}
func TestIncrementInt8_ConditionBlocksIncrement(t *testing.T) {
	s := newSwampForTest(t, "Int8", "ConditionBlocksIncrement")

	key := "i8:cond:block"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentInt8(guardID, 42)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	cond := &IncrementInt8Condition{
		RelationalOperator: RelationalOperatorGreaterThan, // csak ha > 50 növelnénk
		Value:              50,
	}

	newVal, inc, meta, err := s.IncrementInt8(
		key,
		1,
		cond,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inc {
		t.Fatalf("expected incremented=false due to condition")
	}
	if newVal != 42 {
		t.Fatalf("value must remain 42, got %d", newVal)
	}
	if meta != nil {
		t.Fatalf("metadataResponse should be nil when nothing changed")
	}
}
func TestIncrementInt8_WrongType_Error(t *testing.T) {
	s := newSwampForTest(t, "Int8", "WrongType_Error")

	key := "i8:wrong:type"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentString(guardID, "not-an-int8")
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	_, inc, _, err := s.IncrementInt8(
		key,
		1,
		nil,
		nil,
		nil,
	)
	if err == nil {
		t.Fatalf("expected error for wrong content type")
	}
	if inc {
		t.Fatalf("should not have incremented")
	}
	if !errors.Is(err, errors.New(ErrorValueIsNotInt)) && err.Error() != ErrorValueIsNotInt {
		t.Fatalf("unexpected error, got=%v want=%s", err, ErrorValueIsNotInt)
	}
}
func TestIncrementInt8_OverflowWraps(t *testing.T) {
	s := newSwampForTest(t, "Int8", "OverflowWraps")

	key := "i8:overflow"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentInt8(guardID, 120) // közel a MaxInt8-hoz (127)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	newVal, inc, _, err := s.IncrementInt8(
		key,
		10, // 120 + 10 = 130 -> int8 wrap -> -126
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc {
		t.Fatalf("expected increment")
	}
	expected := int8(-126) // vagy: int8(int16(120) + 10)
	if newVal != expected {
		t.Fatalf("expected wrap to %d, got %d", expected, newVal)
	}
}

func TestIncrementInt16_NewKey_NoMetadata(t *testing.T) {
	s := newSwampForTest(t, "Int16", "NewKey_NoMetadata")

	newVal, inc, meta, err := s.IncrementInt16(
		"i16:new:no-meta",
		5,
		nil,
		nil, // metadataIfNotExist
		nil, // metadataIfExist
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc {
		t.Fatalf("expected incremented=true")
	}
	if newVal != 5 {
		t.Fatalf("expected newVal=5, got %d", newVal)
	}
	if meta != nil {
		t.Fatalf("expected metadataResponse=nil for no metadata, got: %#v", meta)
	}
}
func TestIncrementInt16_NewKey_WithMetadataIfNotExist(t *testing.T) {
	s := newSwampForTest(t, "Int16", "NewKey_WithMetadataIfNotExist")

	before := time.Now().UTC()
	metaReq := &IncrementMetadataRequest{
		CreatedAt: true,
		CreatedBy: "alice",
		UpdatedAt: true,
		UpdatedBy: "alice",
		ExpiredAt: before.Add(10 * time.Minute),
	}

	newVal, inc, meta, err := s.IncrementInt16(
		"i16:new:with-meta",
		1,
		nil,
		metaReq, // ifNotExist
		nil,     // ifExist
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 1 {
		t.Fatalf("bad increment result (inc=%v, newVal=%d)", inc, newVal)
	}
	if meta == nil {
		t.Fatalf("expected metadataResponse not nil")
	}
	if meta.CreatedAt.Before(before) {
		t.Fatalf("CreatedAt not set correctly: %v <= %v", meta.CreatedAt, before)
	}
	if meta.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt not set correctly: %v <= %v", meta.UpdatedAt, before)
	}
	if meta.CreatedBy != "alice" || meta.UpdatedBy != "alice" {
		t.Fatalf("bad CreatedBy/UpdatedBy: %+v", meta)
	}
	if meta.ExpiredAt.IsZero() {
		t.Fatalf("ExpiredAt should be set")
	}
}
func TestIncrementInt16_ExistingKey_NoMetadata(t *testing.T) {
	s := newSwampForTest(t, "Int16", "ExistingKey_NoMetadata")

	key := "i16:exist:no-meta"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentInt16(guardID, 10)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	newVal, inc, meta, err := s.IncrementInt16(
		key,
		7,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 17 {
		t.Fatalf("expected 10+7=17, got inc=%v newVal=%d", inc, newVal)
	}
	if meta != nil {
		t.Fatalf("expected metadataResponse=nil when no metadata provided")
	}
}
func TestIncrementInt16_ExistingKey_WithMetadataIfExist(t *testing.T) {
	s := newSwampForTest(t, "Int16", "ExistingKey_WithMetadataIfExist")

	key := "i16:exist:with-meta"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentInt16(guardID, 3)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	before := time.Now().UTC()
	metaReq := &IncrementMetadataRequest{
		UpdatedAt: true,
		UpdatedBy: "bob",
	}

	newVal, inc, meta, err := s.IncrementInt16(
		key,
		2,
		nil,
		nil,     // ifNotExist
		metaReq, // ifExist
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 5 {
		t.Fatalf("expected 3+2=5, got %d", newVal)
	}
	if meta == nil {
		t.Fatalf("metadataResponse must not be nil when metadataIfExist is provided")
	}
	if meta.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt was not updated for existing treasure (did you pass metadataRequestIfExist to setMetaForIncrement?)")
	}
	if meta.UpdatedBy != "bob" {
		t.Fatalf("UpdatedBy not set to 'bob': %+v", meta)
	}
}
func TestIncrementInt16_ConditionBlocksIncrement(t *testing.T) {
	s := newSwampForTest(t, "Int16", "ConditionBlocksIncrement")

	key := "i16:cond:block"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentInt16(guardID, 42)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	cond := &IncrementInt16Condition{
		RelationalOperator: RelationalOperatorGreaterThan, // csak ha > 50 növelnénk
		Value:              50,
	}

	newVal, inc, meta, err := s.IncrementInt16(
		key,
		1,
		cond,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inc {
		t.Fatalf("expected incremented=false due to condition")
	}
	if newVal != 42 {
		t.Fatalf("value must remain 42, got %d", newVal)
	}
	if meta != nil {
		t.Fatalf("metadataResponse should be nil when nothing changed")
	}
}
func TestIncrementInt16_WrongType_Error(t *testing.T) {
	s := newSwampForTest(t, "Int16", "WrongType_Error")

	key := "i16:wrong:type"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentString(guardID, "not-an-int16")
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	_, inc, _, err := s.IncrementInt16(
		key,
		1,
		nil,
		nil,
		nil,
	)
	if err == nil {
		t.Fatalf("expected error for wrong content type")
	}
	if inc {
		t.Fatalf("should not have incremented")
	}
	if !errors.Is(err, errors.New(ErrorValueIsNotInt)) && err.Error() != ErrorValueIsNotInt {
		t.Fatalf("unexpected error, got=%v want=%s", err, ErrorValueIsNotInt)
	}
}
func TestIncrementInt16_OverflowWraps(t *testing.T) {
	s := newSwampForTest(t, "Int16", "OverflowWraps")

	key := "i16:overflow"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentInt16(guardID, 32760) // közel a MaxInt16-hoz (32767)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	newVal, inc, _, err := s.IncrementInt16(
		key,
		10, // 32760 + 10 = 32770 -> int16 wrap -> -32766
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc {
		t.Fatalf("expected increment")
	}
	expected := int16(-32766)
	if newVal != expected {
		t.Fatalf("expected wrap to %d, got %d", expected, newVal)
	}
}

func TestIncrementInt32_NewKey_NoMetadata(t *testing.T) {
	s := newSwampForTest(t, "Int32", "NewKey_NoMetadata")

	newVal, inc, meta, err := s.IncrementInt32(
		"i32:new:no-meta",
		5,
		nil,
		nil,
		nil,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc {
		t.Fatalf("expected incremented=true")
	}
	if newVal != 5 {
		t.Fatalf("expected newVal=5, got %d", newVal)
	}
	if meta != nil {
		t.Fatalf("expected metadataResponse=nil for no metadata, got: %#v", meta)
	}
}
func TestIncrementInt32_NewKey_WithMetadataIfNotExist(t *testing.T) {
	s := newSwampForTest(t, "Int32", "NewKey_WithMetadataIfNotExist")

	before := time.Now().UTC()
	metaReq := &IncrementMetadataRequest{
		CreatedAt: true,
		CreatedBy: "alice",
		UpdatedAt: true,
		UpdatedBy: "alice",
		ExpiredAt: before.Add(10 * time.Minute),
	}

	newVal, inc, meta, err := s.IncrementInt32(
		"i32:new:with-meta",
		1,
		nil,
		metaReq,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 1 {
		t.Fatalf("bad increment result (inc=%v, newVal=%d)", inc, newVal)
	}
	if meta == nil {
		t.Fatalf("expected metadataResponse not nil")
	}
	if meta.CreatedAt.Before(before) {
		t.Fatalf("CreatedAt not set correctly: %v <= %v", meta.CreatedAt, before)
	}
	if meta.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt not set correctly: %v <= %v", meta.UpdatedAt, before)
	}
	if meta.CreatedBy != "alice" || meta.UpdatedBy != "alice" {
		t.Fatalf("bad CreatedBy/UpdatedBy: %+v", meta)
	}
	if meta.ExpiredAt.IsZero() {
		t.Fatalf("ExpiredAt should be set")
	}
}
func TestIncrementInt32_ExistingKey_NoMetadata(t *testing.T) {
	s := newSwampForTest(t, "Int32", "ExistingKey_NoMetadata")

	key := "i32:exist:no-meta"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentInt32(guardID, 10)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	newVal, inc, meta, err := s.IncrementInt32(
		key,
		7,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 17 {
		t.Fatalf("expected 10+7=17, got inc=%v newVal=%d", inc, newVal)
	}
	if meta != nil {
		t.Fatalf("expected metadataResponse=nil when no metadata provided")
	}
}
func TestIncrementInt32_ExistingKey_WithMetadataIfExist(t *testing.T) {
	s := newSwampForTest(t, "Int32", "ExistingKey_WithMetadataIfExist")

	key := "i32:exist:with-meta"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentInt32(guardID, 3)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	before := time.Now().UTC()
	metaReq := &IncrementMetadataRequest{
		UpdatedAt: true,
		UpdatedBy: "bob",
	}

	newVal, inc, meta, err := s.IncrementInt32(
		key,
		2,
		nil,
		nil,
		metaReq,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 5 {
		t.Fatalf("expected 3+2=5, got %d", newVal)
	}
	if meta == nil {
		t.Fatalf("metadataResponse must not be nil when metadataIfExist is provided")
	}
	if meta.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt was not updated for existing treasure (did you pass metadataRequestIfExist to setMetaForIncrement?)")
	}
	if meta.UpdatedBy != "bob" {
		t.Fatalf("UpdatedBy not set to 'bob': %+v", meta)
	}
}
func TestIncrementInt32_ConditionBlocksIncrement(t *testing.T) {
	s := newSwampForTest(t, "Int32", "ConditionBlocksIncrement")

	key := "i32:cond:block"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentInt32(guardID, 42)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	cond := &IncrementInt32Condition{
		RelationalOperator: RelationalOperatorGreaterThan,
		Value:              50,
	}

	newVal, inc, meta, err := s.IncrementInt32(
		key,
		1,
		cond,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inc {
		t.Fatalf("expected incremented=false due to condition")
	}
	if newVal != 42 {
		t.Fatalf("value must remain 42, got %d", newVal)
	}
	if meta != nil {
		t.Fatalf("metadataResponse should be nil when nothing changed")
	}
}
func TestIncrementInt32_WrongType_Error(t *testing.T) {
	s := newSwampForTest(t, "Int32", "WrongType_Error")

	key := "i32:wrong:type"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentString(guardID, "not-an-int32")
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	_, inc, _, err := s.IncrementInt32(
		key,
		1,
		nil,
		nil,
		nil,
	)
	if err == nil {
		t.Fatalf("expected error for wrong content type")
	}
	if inc {
		t.Fatalf("should not have incremented")
	}
	if !errors.Is(err, errors.New(ErrorValueIsNotInt)) && err.Error() != ErrorValueIsNotInt {
		t.Fatalf("unexpected error, got=%v want=%s", err, ErrorValueIsNotInt)
	}
}
func TestIncrementInt32_OverflowWraps(t *testing.T) {
	s := newSwampForTest(t, "Int32", "OverflowWraps")

	key := "i32:overflow"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentInt32(guardID, 2147483640) // közel a MaxInt32-hoz (2147483647)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	newVal, inc, _, err := s.IncrementInt32(
		key,
		10, // wrap-elni fog
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc {
		t.Fatalf("expected increment")
	}

	// >>> ne konstans kifejezést használj
	start := int32(2147483640)
	delta := int32(10)
	expected := start + delta // runtime wrap: -2147483646

	if newVal != expected {
		t.Fatalf("expected wrap to %d, got %d", expected, newVal)
	}
}

func TestIncrementInt64_NewKey_NoMetadata(t *testing.T) {
	s := newSwampForTest(t, "Int64", "NewKey_NoMetadata")

	newVal, inc, meta, err := s.IncrementInt64(
		"i64:new:no-meta",
		5,
		nil,
		nil,
		nil,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc {
		t.Fatalf("expected incremented=true")
	}
	if newVal != 5 {
		t.Fatalf("expected newVal=5, got %d", newVal)
	}
	if meta != nil {
		t.Fatalf("expected metadataResponse=nil for no metadata, got: %#v", meta)
	}
}
func TestIncrementInt64_NewKey_WithMetadataIfNotExist(t *testing.T) {
	s := newSwampForTest(t, "Int64", "NewKey_WithMetadataIfNotExist")

	before := time.Now().UTC()
	metaReq := &IncrementMetadataRequest{
		CreatedAt: true,
		CreatedBy: "alice",
		UpdatedAt: true,
		UpdatedBy: "alice",
		ExpiredAt: before.Add(10 * time.Minute),
	}

	newVal, inc, meta, err := s.IncrementInt64(
		"i64:new:with-meta",
		1,
		nil,
		metaReq,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 1 {
		t.Fatalf("bad increment result (inc=%v, newVal=%d)", inc, newVal)
	}
	if meta == nil {
		t.Fatalf("expected metadataResponse not nil")
	}
	if meta.CreatedAt.Before(before) {
		t.Fatalf("CreatedAt not set correctly: %v <= %v", meta.CreatedAt, before)
	}
	if meta.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt not set correctly: %v <= %v", meta.UpdatedAt, before)
	}
	if meta.CreatedBy != "alice" || meta.UpdatedBy != "alice" {
		t.Fatalf("bad CreatedBy/UpdatedBy: %+v", meta)
	}
	if meta.ExpiredAt.IsZero() {
		t.Fatalf("ExpiredAt should be set")
	}
}
func TestIncrementInt64_ExistingKey_NoMetadata(t *testing.T) {
	s := newSwampForTest(t, "Int64", "ExistingKey_NoMetadata")

	key := "i64:exist:no-meta"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentInt64(guardID, 10)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	newVal, inc, meta, err := s.IncrementInt64(
		key,
		7,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 17 {
		t.Fatalf("expected 10+7=17, got inc=%v newVal=%d", inc, newVal)
	}
	if meta != nil {
		t.Fatalf("expected metadataResponse=nil when no metadata provided")
	}
}
func TestIncrementInt64_ExistingKey_WithMetadataIfExist(t *testing.T) {
	s := newSwampForTest(t, "Int64", "ExistingKey_WithMetadataIfExist")

	key := "i64:exist:with-meta"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentInt64(guardID, 3)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	before := time.Now().UTC()
	metaReq := &IncrementMetadataRequest{
		UpdatedAt: true,
		UpdatedBy: "bob",
	}

	newVal, inc, meta, err := s.IncrementInt64(
		key,
		2,
		nil,
		nil,
		metaReq,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 5 {
		t.Fatalf("expected 3+2=5, got %d", newVal)
	}
	if meta == nil {
		t.Fatalf("metadataResponse must not be nil when metadataIfExist is provided")
	}
	if meta.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt was not updated for existing treasure (did you pass metadataRequestIfExist to setMetaForIncrement?)")
	}
	if meta.UpdatedBy != "bob" {
		t.Fatalf("UpdatedBy not set to 'bob': %+v", meta)
	}
}
func TestIncrementInt64_ConditionBlocksIncrement(t *testing.T) {
	s := newSwampForTest(t, "Int64", "ConditionBlocksIncrement")

	key := "i64:cond:block"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentInt64(guardID, 42)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	cond := &IncrementInt64Condition{
		RelationalOperator: RelationalOperatorGreaterThan,
		Value:              50,
	}

	newVal, inc, meta, err := s.IncrementInt64(
		key,
		1,
		cond,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inc {
		t.Fatalf("expected incremented=false due to condition")
	}
	if newVal != 42 {
		t.Fatalf("value must remain 42, got %d", newVal)
	}
	if meta != nil {
		t.Fatalf("metadataResponse should be nil when nothing changed")
	}
}
func TestIncrementInt64_WrongType_Error(t *testing.T) {
	s := newSwampForTest(t, "Int64", "WrongType_Error")

	key := "i64:wrong:type"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentString(guardID, "not-an-int64")
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	_, inc, _, err := s.IncrementInt64(
		key,
		1,
		nil,
		nil,
		nil,
	)
	if err == nil {
		t.Fatalf("expected error for wrong content type")
	}
	if inc {
		t.Fatalf("should not have incremented")
	}
	if !errors.Is(err, errors.New(ErrorValueIsNotInt)) && err.Error() != ErrorValueIsNotInt {
		t.Fatalf("unexpected error, got=%v want=%s", err, ErrorValueIsNotInt)
	}
}
func TestIncrementInt64_OverflowWraps(t *testing.T) {
	s := newSwampForTest(t, "Int64", "OverflowWraps")

	key := "i64:overflow"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentInt64(guardID, math.MaxInt64-5)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	newVal, inc, _, err := s.IncrementInt64(
		key,
		10,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc {
		t.Fatalf("expected increment")
	}

	// → ne konstans kifejezést használj
	start := int64(math.MaxInt64 - 5)
	delta := int64(10)
	expected := start + delta // futásidőben wrap-el

	// alternatíva (explicit mod 2^64):
	// expected := int64(uint64(start) + uint64(delta))

	if newVal != expected {
		t.Fatalf("expected wrap to %d, got %d", expected, newVal)
	}
}

func TestIncrementFloat32_NewKey_NoMetadata(t *testing.T) {
	s := newSwampForTest(t, "Float32", "NewKey_NoMetadata")

	newVal, inc, meta, err := s.IncrementFloat32(
		"f32:new:no-meta",
		5,
		nil,
		nil, // metadataIfNotExist
		nil, // metadataIfExist
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc {
		t.Fatalf("expected incremented=true")
	}
	if newVal != 5 {
		t.Fatalf("expected newVal=5, got %v", newVal)
	}
	if meta != nil {
		t.Fatalf("expected metadataResponse=nil for no metadata, got: %#v", meta)
	}
}
func TestIncrementFloat32_NewKey_WithMetadataIfNotExist(t *testing.T) {
	s := newSwampForTest(t, "Float32", "NewKey_WithMetadataIfNotExist")

	before := time.Now().UTC()
	metaReq := &IncrementMetadataRequest{
		CreatedAt: true,
		CreatedBy: "alice",
		UpdatedAt: true,
		UpdatedBy: "alice",
		ExpiredAt: before.Add(10 * time.Minute),
	}

	newVal, inc, meta, err := s.IncrementFloat32(
		"f32:new:with-meta",
		1,
		nil,
		metaReq, // ifNotExist
		nil,     // ifExist
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 1 {
		t.Fatalf("bad increment result (inc=%v, newVal=%v)", inc, newVal)
	}
	if meta == nil {
		t.Fatalf("expected metadataResponse not nil")
	}
	if meta.CreatedAt.Before(before) {
		t.Fatalf("CreatedAt not set correctly: %v <= %v", meta.CreatedAt, before)
	}
	if meta.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt not set correctly: %v <= %v", meta.UpdatedAt, before)
	}
	if meta.CreatedBy != "alice" || meta.UpdatedBy != "alice" {
		t.Fatalf("bad CreatedBy/UpdatedBy: %+v", meta)
	}
	if meta.ExpiredAt.IsZero() {
		t.Fatalf("ExpiredAt should be set")
	}
}
func TestIncrementFloat32_ExistingKey_NoMetadata(t *testing.T) {
	s := newSwampForTest(t, "Float32", "ExistingKey_NoMetadata")

	key := "f32:exist:no-meta"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentFloat32(guardID, 10)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	newVal, inc, meta, err := s.IncrementFloat32(
		key,
		7,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 17 {
		t.Fatalf("expected 10+7=17, got inc=%v newVal=%v", inc, newVal)
	}
	if meta != nil {
		t.Fatalf("expected metadataResponse=nil when no metadata provided")
	}
}
func TestIncrementFloat32_ExistingKey_WithMetadataIfExist(t *testing.T) {
	s := newSwampForTest(t, "Float32", "ExistingKey_WithMetadataIfExist")

	key := "f32:exist:with-meta"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentFloat32(guardID, 3)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	before := time.Now().UTC()
	metaReq := &IncrementMetadataRequest{
		UpdatedAt: true,
		UpdatedBy: "bob",
	}

	newVal, inc, meta, err := s.IncrementFloat32(
		key,
		2,
		nil,
		nil,     // ifNotExist
		metaReq, // ifExist
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 5 {
		t.Fatalf("expected 3+2=5, got %v", newVal)
	}
	if meta == nil {
		t.Fatalf("metadataResponse must not be nil when metadataIfExist is provided")
	}
	if meta.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt was not updated for existing treasure (did you pass metadataRequestIfExist to setMetaForIncrement?)")
	}
	if meta.UpdatedBy != "bob" {
		t.Fatalf("UpdatedBy not set to 'bob': %+v", meta)
	}
}
func TestIncrementFloat32_ConditionBlocksIncrement(t *testing.T) {
	s := newSwampForTest(t, "Float32", "ConditionBlocksIncrement")

	key := "f32:cond:block"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentFloat32(guardID, 42)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	cond := &IncrementFloat32Condition{
		RelationalOperator: RelationalOperatorGreaterThan,
		Value:              50,
	}

	newVal, inc, meta, err := s.IncrementFloat32(
		key,
		1,
		cond,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inc {
		t.Fatalf("expected incremented=false due to condition")
	}
	if newVal != 42 {
		t.Fatalf("value must remain 42, got %v", newVal)
	}
	if meta != nil {
		t.Fatalf("metadataResponse should be nil when nothing changed")
	}
}
func TestIncrementFloat32_WrongType_Error(t *testing.T) {
	s := newSwampForTest(t, "Float32", "WrongType_Error")

	key := "f32:wrong:type"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentString(guardID, "not-a-float32")
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	_, inc, _, err := s.IncrementFloat32(
		key,
		1,
		nil,
		nil,
		nil,
	)
	if err == nil {
		t.Fatalf("expected error for wrong content type")
	}
	if inc {
		t.Fatalf("should not have incremented")
	}
}
func TestIncrementFloat32_OverflowToInf(t *testing.T) {
	s := newSwampForTest(t, "Float32", "OverflowToInf")

	key := "f32:overflow"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentFloat32(guardID, float32(math.MaxFloat32))
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	// nagy delta, hogy túlcsorduljon +Inf-re
	delta := float32(math.MaxFloat32)

	newVal, inc, _, err := s.IncrementFloat32(
		key,
		delta,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc {
		t.Fatalf("expected increment")
	}

	expected := float32(math.Inf(1))
	if !float32IsInf(newVal) {
		t.Fatalf("expected +Inf, got %v", newVal)
	}
	if math.IsNaN(float64(newVal)) || newVal != expected {
		// float32(+Inf) összevethető expected-del
		t.Fatalf("expected %v, got %v", expected, newVal)
	}
}

func TestIncrementFloat64_NewKey_NoMetadata(t *testing.T) {
	s := newSwampForTest(t, "Float64", "NewKey_NoMetadata")

	newVal, inc, meta, err := s.IncrementFloat64(
		"f64:new:no-meta",
		5,
		nil,
		nil, // metadataIfNotExist
		nil, // metadataIfExist
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc {
		t.Fatalf("expected incremented=true")
	}
	if newVal != 5 {
		t.Fatalf("expected newVal=5, got %v", newVal)
	}
	if meta != nil {
		t.Fatalf("expected metadataResponse=nil for no metadata, got: %#v", meta)
	}
}
func TestIncrementFloat64_NewKey_WithMetadataIfNotExist(t *testing.T) {
	s := newSwampForTest(t, "Float64", "NewKey_WithMetadataIfNotExist")

	before := time.Now().UTC()
	metaReq := &IncrementMetadataRequest{
		CreatedAt: true,
		CreatedBy: "alice",
		UpdatedAt: true,
		UpdatedBy: "alice",
		ExpiredAt: before.Add(10 * time.Minute),
	}

	newVal, inc, meta, err := s.IncrementFloat64(
		"f64:new:with-meta",
		1,
		nil,
		metaReq, // ifNotExist
		nil,     // ifExist
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 1 {
		t.Fatalf("bad increment result (inc=%v, newVal=%v)", inc, newVal)
	}
	if meta == nil {
		t.Fatalf("expected metadataResponse not nil")
	}
	if meta.CreatedAt.Before(before) {
		t.Fatalf("CreatedAt not set correctly: %v <= %v", meta.CreatedAt, before)
	}
	if meta.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt not set correctly: %v <= %v", meta.UpdatedAt, before)
	}
	if meta.CreatedBy != "alice" || meta.UpdatedBy != "alice" {
		t.Fatalf("bad CreatedBy/UpdatedBy: %+v", meta)
	}
	if meta.ExpiredAt.IsZero() {
		t.Fatalf("ExpiredAt should be set")
	}
}
func TestIncrementFloat64_ExistingKey_NoMetadata(t *testing.T) {
	s := newSwampForTest(t, "Float64", "ExistingKey_NoMetadata")

	key := "f64:exist:no-meta"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentFloat64(guardID, 10)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	newVal, inc, meta, err := s.IncrementFloat64(
		key,
		7,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 17 {
		t.Fatalf("expected 10+7=17, got inc=%v newVal=%v", inc, newVal)
	}
	if meta != nil {
		t.Fatalf("expected metadataResponse=nil when no metadata provided")
	}
}
func TestIncrementFloat64_ExistingKey_WithMetadataIfExist(t *testing.T) {
	s := newSwampForTest(t, "Float64", "ExistingKey_WithMetadataIfExist")

	key := "f64:exist:with-meta"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentFloat64(guardID, 3)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	before := time.Now().UTC()
	metaReq := &IncrementMetadataRequest{
		UpdatedAt: true,
		UpdatedBy: "bob",
	}

	newVal, inc, meta, err := s.IncrementFloat64(
		key,
		2,
		nil,
		nil,     // ifNotExist
		metaReq, // ifExist
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc || newVal != 5 {
		t.Fatalf("expected 3+2=5, got %v", newVal)
	}
	if meta == nil {
		t.Fatalf("metadataResponse must not be nil when metadataIfExist is provided")
	}
	if meta.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt was not updated for existing treasure (did you pass metadataRequestIfExist to setMetaForIncrement?)")
	}
	if meta.UpdatedBy != "bob" {
		t.Fatalf("UpdatedBy not set to 'bob': %+v", meta)
	}
}
func TestIncrementFloat64_ConditionBlocksIncrement(t *testing.T) {
	s := newSwampForTest(t, "Float64", "ConditionBlocksIncrement")

	key := "f64:cond:block"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentFloat64(guardID, 42)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	cond := &IncrementFloat64Condition{
		RelationalOperator: RelationalOperatorGreaterThan,
		Value:              50,
	}

	newVal, inc, meta, err := s.IncrementFloat64(
		key,
		1,
		cond,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inc {
		t.Fatalf("expected incremented=false due to condition")
	}
	if newVal != 42 {
		t.Fatalf("value must remain 42, got %v", newVal)
	}
	if meta != nil {
		t.Fatalf("metadataResponse should be nil when nothing changed")
	}
}
func TestIncrementFloat64_WrongType_Error(t *testing.T) {
	s := newSwampForTest(t, "Float64", "WrongType_Error")

	key := "f64:wrong:type"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentString(guardID, "not-a-float64")
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	_, inc, _, err := s.IncrementFloat64(
		key,
		1,
		nil,
		nil,
		nil,
	)
	if err == nil {
		t.Fatalf("expected error for wrong content type")
	}
	if inc {
		t.Fatalf("should not have incremented")
	}
}
func TestIncrementFloat64_OverflowToInf(t *testing.T) {
	s := newSwampForTest(t, "Float64", "OverflowToInf")

	key := "f64:overflow"
	tr := s.CreateTreasure(key)
	guardID := tr.StartTreasureGuard(true)
	tr.SetContentFloat64(guardID, math.MaxFloat64)
	tr.Save(guardID)
	tr.ReleaseTreasureGuard(guardID)

	delta := math.MaxFloat64

	newVal, inc, _, err := s.IncrementFloat64(
		key,
		delta,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inc {
		t.Fatalf("expected increment")
	}

	expected := math.Inf(1)
	if !math.IsInf(newVal, 1) {
		t.Fatalf("expected +Inf, got %v", newVal)
	}
	if math.IsNaN(newVal) || newVal != expected {
		t.Fatalf("expected %v, got %v", expected, newVal)
	}
}

// Test for GetChronicler
// Test for GetName
// Test for GetTreasure
// Test for GetManyTreasures
// Test for TreasureExists
func TestSwamp_GetTreasuresByBeaconWithVariousMethod(t *testing.T) {

	fsInterface := filesystem.New()
	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)
	// gyors filementés és bezárás a tesztekhez

	fss := &settings.FileSystemSettings{
		WriteIntervalSec: 1,
		MaxFileSizeByte:  8192,
	}

	settingsInterface.RegisterPattern(name.New().Sanctuary(sanctuaryForQuickTest).Realm("*").Swamp("*"), false, 1, fss)
	closeAfterIdle := 1 * time.Second
	writeInterval := 1 * time.Second
	maxFileSize := int64(8192)

	t.Run("should get treasures by the beacon with various method", func(t *testing.T) {

		allTests := 10

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("should-get-treasure-from-beacon").Swamp("wit-various-method")

		hashPath := swampName.GetFullHashPath(settingsInterface.GetHydraAbsDataFolderPath(), testAllServers, testMaxDepth, testMaxFolderPerLevel)
		chroniclerInterface := chronicler.New(hashPath, maxFileSize, testMaxDepth, fsInterface, metadata.New(hashPath))
		chroniclerInterface.CreateDirectoryIfNotExists()

		swampEventCallbackFunc := func(e *Event) {}

		closeCallbackFunc := func(n name.Name) {}

		swampInfoCallbackFunc := func(i *Info) {}

		fssSwamp := &FilesystemSettings{
			ChroniclerInterface: chroniclerInterface,
			WriteInterval:       writeInterval,
		}

		metadataInterface := metadata.New(hashPath)
		swampInterface := New(swampName, closeAfterIdle, fssSwamp, swampEventCallbackFunc, swampInfoCallbackFunc, closeCallbackFunc, metadataInterface)
		swampInterface.BeginVigil()

		for i := 0; i < allTests; i++ {
			treasureInterface := swampInterface.CreateTreasure(fmt.Sprintf("%d", i))
			if treasureInterface == nil {
				t.Errorf("treasureInterface should not be nil")
			}
			guardID := treasureInterface.StartTreasureGuard(true)
			treasureInterface.SetContentFloat64(guardID, 0.12+float64(i))
			treasureInterface.SetCreatedAt(guardID, time.Now())
			treasureInterface.SetExpirationTime(guardID, time.Now().Add(-time.Second*time.Duration(i)))
			treasureInterface.ReleaseTreasureGuard(guardID)

			guardID = treasureInterface.StartTreasureGuard(true)
			_ = treasureInterface.Save(guardID)
			treasureInterface.ReleaseTreasureGuard(guardID)

			time.Sleep(time.Millisecond * 10)
		}

		receivedChroniclerInterface := swampInterface.GetChronicler()
		assert.NotNil(t, receivedChroniclerInterface, "chroniclerInterface should not be nil")
		assert.True(t, receivedChroniclerInterface.IsFilesystemInitiated(), "chroniclerInterface should be initiated")

		receivedName := swampInterface.GetName()
		assert.Equal(t, swampName, receivedName, "name should be equal")

		treasureObject, err := swampInterface.GetTreasure("0")
		assert.Nil(t, err, "error should be nil")
		assert.NotNil(t, treasureObject, "treasureObject should not be nil")
		assert.Equal(t, "0", treasureObject.GetKey(), "key should be equal")
		assert.True(t, swampInterface.TreasureExists("0"), "treasure should exist")

		// get and delete the treasure 0
		treasureObject, err = swampInterface.GetTreasure("0")
		_ = swampInterface.DeleteTreasure(treasureObject.GetKey(), false)

		assert.Nil(t, err, "error should be nil")
		assert.NotNil(t, treasureObject, "treasureObject should not be nil")
		assert.Equal(t, "0", treasureObject.GetKey(), "key should be equal")
		assert.False(t, swampInterface.TreasureExists("0"), "treasure should NOT exist anymore")

		treasureObject, err = swampInterface.GetTreasure("0")
		assert.NotNil(t, err, fmt.Sprintf("error should be nil err: %s", err))
		assert.Nil(t, treasureObject, "treasureObject should be nil")

		swampInterface.CeaseVigil()
		swampInterface.Destroy()

	})

}

// Test for GetAndDeleteRandomTreasures
// Test for GetAndDeleteExpiredTreasures
func TestSwamp_GetAndDelete(t *testing.T) {

	fsInterface := filesystem.New()
	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)
	// gyors filementés és bezárás a tesztekhez

	fss := &settings.FileSystemSettings{
		WriteIntervalSec: 1,
		MaxFileSizeByte:  8192,
	}

	settingsInterface.RegisterPattern(name.New().Sanctuary(sanctuaryForQuickTest).Realm("*").Swamp("*"), false, 1, fss)
	closeAfterIdle := 1 * time.Second
	writeInterval := 1 * time.Second
	maxFileSize := int64(8192)

	t.Run("should get and delete treasures", func(t *testing.T) {

		allTests := 10

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("should-get").Swamp("and-delete-treasures")

		hashPath := swampName.GetFullHashPath(settingsInterface.GetHydraAbsDataFolderPath(), testAllServers, testMaxDepth, testMaxFolderPerLevel)
		chroniclerInterface := chronicler.New(hashPath, maxFileSize, testMaxDepth, fsInterface, metadata.New(hashPath))
		chroniclerInterface.CreateDirectoryIfNotExists()

		swampEventCallbackFunc := func(e *Event) {}

		closeCallbackFunc := func(n name.Name) {}

		swampInfoCallbackFunc := func(i *Info) {}

		fssSwamp := &FilesystemSettings{
			ChroniclerInterface: chroniclerInterface,
			WriteInterval:       writeInterval,
		}

		metadataInterface := metadata.New(hashPath)
		swampInterface := New(swampName, closeAfterIdle, fssSwamp, swampEventCallbackFunc, swampInfoCallbackFunc, closeCallbackFunc, metadataInterface)
		swampInterface.BeginVigil()

		for i := 0; i < allTests; i++ {
			treasureInterface := swampInterface.CreateTreasure(fmt.Sprintf("%d", i))
			if treasureInterface == nil {
				t.Errorf("treasureInterface should not be nil")
			}
			guardID := treasureInterface.StartTreasureGuard(true)
			treasureInterface.SetContentFloat64(guardID, 0.12+float64(i))
			treasureInterface.SetCreatedAt(guardID, time.Now())
			treasureInterface.SetExpirationTime(guardID, time.Now().Add(-time.Second*time.Duration(i)))
			treasureInterface.ReleaseTreasureGuard(guardID)

			guardID = treasureInterface.StartTreasureGuard(true)
			_ = treasureInterface.Save(guardID)
			treasureInterface.ReleaseTreasureGuard(guardID)

			time.Sleep(time.Millisecond * 10)
		}

		assert.Equal(t, allTests, swampInterface.CountTreasures())

		swampInterface.CeaseVigil()
		swampInterface.Destroy()

	})

}

func TestSwamp_GetAllTreasures(t *testing.T) {

	fsInterface := filesystem.New()
	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)
	// gyors filementés és bezárás a tesztekhez

	fss := &settings.FileSystemSettings{
		WriteIntervalSec: 1,
		MaxFileSizeByte:  8192,
	}

	settingsInterface.RegisterPattern(name.New().Sanctuary(sanctuaryForQuickTest).Realm("*").Swamp("*"), false, 1, fss)
	closeAfterIdle := 1 * time.Second
	writeInterval := 1 * time.Second
	maxFileSize := int64(8192)

	t.Run("should get all treasures", func(t *testing.T) {

		allTests := 10

		swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("should-get").Swamp("all-treasures")

		hashPath := swampName.GetFullHashPath(settingsInterface.GetHydraAbsDataFolderPath(), testAllServers, testMaxDepth, testMaxFolderPerLevel)
		chroniclerInterface := chronicler.New(hashPath, maxFileSize, testMaxDepth, fsInterface, metadata.New(hashPath))
		chroniclerInterface.CreateDirectoryIfNotExists()

		swampEventCallbackFunc := func(e *Event) {}

		closeCallbackFunc := func(n name.Name) {}

		swampInfoCallbackFunc := func(i *Info) {}

		fssSwamp := &FilesystemSettings{
			ChroniclerInterface: chroniclerInterface,
			WriteInterval:       writeInterval,
		}

		metadataInterface := metadata.New(hashPath)
		swampInterface := New(swampName, closeAfterIdle, fssSwamp, swampEventCallbackFunc, swampInfoCallbackFunc, closeCallbackFunc, metadataInterface)
		swampInterface.BeginVigil()

		receivedTreasures := swampInterface.GetAll()
		assert.Nil(t, receivedTreasures, "treasures should be nil")

		for i := 0; i < allTests; i++ {
			treasureInterface := swampInterface.CreateTreasure(fmt.Sprintf("%d", i))
			if treasureInterface == nil {
				t.Errorf("treasureInterface should not be nil")
			}
			guardID := treasureInterface.StartTreasureGuard(true)
			treasureInterface.SetContentString(guardID, fmt.Sprintf("content-%d", i))
			treasureInterface.ReleaseTreasureGuard(guardID)

			guardID = treasureInterface.StartTreasureGuard(true)
			_ = treasureInterface.Save(guardID)
			treasureInterface.ReleaseTreasureGuard(guardID)

		}

		treasures := swampInterface.GetAll()
		assert.Equal(t, allTests, len(treasures), "treasures should be 10")

		swampInterface.CeaseVigil()

	})
}

// elapsed time in seconds: 0.000312
// all elements int the swamp after end: 8066
// elements per second: 320362.907101
func TestSaveSpeed(t *testing.T) {

	allTest := 100

	fsInterface := filesystem.New()
	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)
	// gyors filementés és bezárás a tesztekhez

	fss := &settings.FileSystemSettings{
		WriteIntervalSec: 1,
		MaxFileSizeByte:  8192,
	}

	settingsInterface.RegisterPattern(name.New().Sanctuary(sanctuaryForQuickTest).Realm("*").Swamp("*"), false, 1, fss)
	closeAfterIdle := 1 * time.Second
	writeInterval := 0 * time.Second
	maxFileSize := int64(8192)

	swampName := name.New().Sanctuary(sanctuaryForQuickTest).Realm("testing").Swamp("save-speed")

	hashPath := swampName.GetFullHashPath(settingsInterface.GetHydraAbsDataFolderPath(), testAllServers, testMaxDepth, testMaxFolderPerLevel)
	chroniclerInterface := chronicler.New(hashPath, maxFileSize, testMaxDepth, fsInterface, metadata.New(hashPath))
	chroniclerInterface.CreateDirectoryIfNotExists()

	swampEventCallbackFunc := func(e *Event) {}
	closeCallbackFunc := func(n name.Name) {}
	swampInfoCallbackFunc := func(i *Info) {}

	fssSwamp := &FilesystemSettings{
		ChroniclerInterface: chroniclerInterface,
		WriteInterval:       writeInterval,
	}

	metadataInterface := metadata.New(hashPath)
	swampInterface := New(swampName, closeAfterIdle, fssSwamp, swampEventCallbackFunc, swampInfoCallbackFunc, closeCallbackFunc, metadataInterface)

	swampInterface.BeginVigil()

	fmt.Printf("all elements int the swamp before starting: %d \n", swampInterface.CountTreasures())

	begin := time.Now()

	finishedChannel := make(chan bool)
	waiter := make(chan bool)
	go func() {
		finishedCount := 0
		for {
			<-finishedChannel
			finishedCount++
			if finishedCount == allTest {
				fmt.Println("all done")
				waiter <- true
			}
		}
	}()

	for i := 0; i < allTest; i++ {
		go func(counter int, fc chan<- bool) {

			newTreasure := swampInterface.CreateTreasure(fmt.Sprintf("test-%d", counter))
			guardID := newTreasure.StartTreasureGuard(true)

			newTreasure.SetContentString(guardID, "lorem ipsum dolor sit")
			defer newTreasure.ReleaseTreasureGuard(guardID)

			_ = newTreasure.Save(guardID)

			fc <- true
		}(i, finishedChannel)
	}

	<-waiter

	end := time.Now()
	elapsed := end.Sub(begin)

	fmt.Printf("elapsed time in seconds: %f \n", elapsed.Seconds())
	fmt.Printf("all elements int the swamp after end: %d \n", swampInterface.CountTreasures())

	// calculate how many elements per second
	fmt.Printf("elements per second: %f \n", float64(allTest)/elapsed.Seconds())

	swampInterface.CeaseVigil()

}

// newSwampForTest create a new Swamp instance for testing purposes.
func newSwampForTest(t *testing.T, realmName, swampName string) Swamp {

	t.Helper()

	fsInterface := filesystem.New()
	settingsInterface := settings.New(testMaxDepth, testMaxFolderPerLevel)

	// gyors filementés és bezárás a tesztekhez
	fss := &settings.FileSystemSettings{
		WriteIntervalSec: 1,
		MaxFileSizeByte:  8192,
	}

	settingsInterface.RegisterPattern(name.New().Sanctuary(sanctuaryForQuickTest).Realm(realmName).Swamp("*"), false, 1, fss)
	closeAfterIdle := 1 * time.Second
	writeInterval := 1 * time.Second
	maxFileSize := int64(8192)

	swampNameObj := name.New().Sanctuary(sanctuaryForQuickTest).Realm(realmName).Swamp(swampName)

	hashPath := swampNameObj.GetFullHashPath(settingsInterface.GetHydraAbsDataFolderPath(), testAllServers, testMaxDepth, testMaxFolderPerLevel)
	chroniclerInterface := chronicler.New(hashPath, maxFileSize, testMaxDepth, fsInterface, metadata.New(hashPath))
	chroniclerInterface.CreateDirectoryIfNotExists()

	swampEventCallbackFunc := func(e *Event) {}

	closeCallbackFunc := func(n name.Name) {}

	swampInfoCallbackFunc := func(i *Info) {}

	fssSwamp := &FilesystemSettings{
		ChroniclerInterface: chroniclerInterface,
		WriteInterval:       writeInterval,
	}

	metadataInterface := metadata.New(hashPath)
	return New(swampNameObj, closeAfterIdle, fssSwamp, swampEventCallbackFunc, swampInfoCallbackFunc, closeCallbackFunc, metadataInterface)

}

func float32IsInf(f float32) bool {
	return math.IsInf(float64(f), 1)
}
