package settings

import (
	"fmt"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/name"
	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {

	// destroy test folder

	t.Run("should load default setting for the swamp", func(t *testing.T) {

		maxDepthOfFolders := 2
		maxFoldersPerLevel := 100000

		configs := New(maxDepthOfFolders, maxFoldersPerLevel)

		swampName := name.New().Sanctuary("settingstest1").Realm("myrealm").Swamp("myswamp")

		configInterface := configs.GetBySwampName(swampName)

		assert.Equal(t, int64(65536), configInterface.GetMaxFileSizeByte(), "should be equal")
		assert.Equal(t, 5*time.Second, configInterface.GetCloseAfterIdle(), "should be equal")
		assert.Equal(t, 1*time.Second, configInterface.GetWriteInterval(), "should be equal")
		assert.Equal(t, swampName.Get(), configInterface.GetPattern().Get(), "should be equal")

	})

	t.Run("should add new pattern and sanctuary", func(t *testing.T) {

		maxDepthOfFolders := 2
		maxFoldersPerLevel := 2000

		configs := New(maxDepthOfFolders, maxFoldersPerLevel)
		pattern := name.New().Sanctuary("settingstest2").Realm("*").Swamp("info")

		configs.RegisterPattern(pattern, false, 5, &FileSystemSettings{
			WriteIntervalSec: 14,
			MaxFileSizeByte:  888888,
		})

		// töröljük a patternt
		defer configs.DeregisterPattern(pattern)

		// the swamp name does not match any pattern, so it should return the default settings
		settingsObj := configs.GetBySwampName(name.New().Sanctuary("settingstest2").Realm("index.hu").Swamp("info"))

		assert.NotNil(t, settingsObj, "should not be nil")

		fmt.Println("pattern:", settingsObj.GetPattern())

		assert.Equal(t, int64(888888), settingsObj.GetMaxFileSizeByte(), "should be equal")
		assert.Equal(t, 14*time.Second, settingsObj.GetWriteInterval(), "should be equal")
		assert.Equal(t, 5*time.Second, settingsObj.GetCloseAfterIdle(), "should be equal")

	})

}

func TestEngine(t *testing.T) {

	t.Run("should return V1 as default engine", func(t *testing.T) {
		maxDepthOfFolders := 2
		maxFoldersPerLevel := 100000

		configs := New(maxDepthOfFolders, maxFoldersPerLevel)

		// Default should be V1
		assert.Equal(t, EngineV1, configs.GetEngine(), "default engine should be V1")
		assert.False(t, configs.IsV2Engine(), "IsV2Engine should be false by default")
	})

	t.Run("should set engine to V2", func(t *testing.T) {
		maxDepthOfFolders := 2
		maxFoldersPerLevel := 100000

		configs := New(maxDepthOfFolders, maxFoldersPerLevel)

		// Set engine to V2
		err := configs.SetEngine(EngineV2)
		assert.NoError(t, err, "SetEngine should not return error")

		// Verify V2 is set
		assert.Equal(t, EngineV2, configs.GetEngine(), "engine should be V2")
		assert.True(t, configs.IsV2Engine(), "IsV2Engine should be true")

		// Set back to V1
		err = configs.SetEngine(EngineV1)
		assert.NoError(t, err, "SetEngine should not return error")

		// Verify V1 is set
		assert.Equal(t, EngineV1, configs.GetEngine(), "engine should be V1")
		assert.False(t, configs.IsV2Engine(), "IsV2Engine should be false")
	})

	t.Run("should reject invalid engine version", func(t *testing.T) {
		maxDepthOfFolders := 2
		maxFoldersPerLevel := 100000

		configs := New(maxDepthOfFolders, maxFoldersPerLevel)

		// Set invalid engine
		err := configs.SetEngine("V3")
		assert.Error(t, err, "SetEngine should return error for invalid version")
		assert.Contains(t, err.Error(), "invalid engine version")

		// Engine should still be V1
		assert.Equal(t, EngineV1, configs.GetEngine(), "engine should remain V1")
	})

	t.Run("should persist engine setting across reload", func(t *testing.T) {
		maxDepthOfFolders := 2
		maxFoldersPerLevel := 100000

		// First instance - set V2
		configs1 := New(maxDepthOfFolders, maxFoldersPerLevel)
		err := configs1.SetEngine(EngineV2)
		assert.NoError(t, err, "SetEngine should not return error")

		// Second instance - should load V2 from settings.json
		configs2 := New(maxDepthOfFolders, maxFoldersPerLevel)
		assert.Equal(t, EngineV2, configs2.GetEngine(), "engine should be V2 after reload")

		// Cleanup - set back to V1
		_ = configs2.SetEngine(EngineV1)
	})
}
