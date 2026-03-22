package updatecache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCachedUpdateInfoIsValid(t *testing.T) {
	t.Parallel()

	t.Run("valid cache for same version", func(t *testing.T) {
		c := &CachedUpdateInfo{
			CheckedAt:  time.Now(),
			ForVersion: "v1.0.0",
		}
		assert.True(t, c.IsValid("v1.0.0"))
	})

	t.Run("invalid cache for different version", func(t *testing.T) {
		c := &CachedUpdateInfo{
			CheckedAt:  time.Now(),
			ForVersion: "v1.0.0",
		}
		assert.False(t, c.IsValid("v2.0.0"))
	})

	t.Run("expired cache", func(t *testing.T) {
		c := &CachedUpdateInfo{
			CheckedAt:  time.Now().Add(-25 * time.Hour),
			ForVersion: "v1.0.0",
		}
		assert.False(t, c.IsValid("v1.0.0"))
	})
}

func TestGitHubCacheRoundTrip(t *testing.T) {
	// Override cache dir to use a temp directory
	origGetCacheDir := getCacheDirFunc
	tmpDir := t.TempDir()
	getCacheDirFunc = func() (string, error) { return tmpDir, nil }
	t.Cleanup(func() { getCacheDirFunc = origGetCacheDir })

	key := "test-releases-list"
	body := []byte(`[{"tag_name":"server/v1.0.0"}]`)

	// Save to cache
	err := SaveGitHubCache(key, body)
	require.NoError(t, err)

	// Load from cache
	cached, err := LoadGitHubCache(key)
	require.NoError(t, err)
	assert.Equal(t, body, cached.Body)
	assert.True(t, cached.IsValid())
}

func TestGitHubCacheMiss(t *testing.T) {
	origGetCacheDir := getCacheDirFunc
	tmpDir := t.TempDir()
	getCacheDirFunc = func() (string, error) { return tmpDir, nil }
	t.Cleanup(func() { getCacheDirFunc = origGetCacheDir })

	_, err := LoadGitHubCache("nonexistent-key")
	assert.Error(t, err)
}

func TestGitHubCacheExpiry(t *testing.T) {
	origGetCacheDir := getCacheDirFunc
	tmpDir := t.TempDir()
	getCacheDirFunc = func() (string, error) { return tmpDir, nil }
	t.Cleanup(func() { getCacheDirFunc = origGetCacheDir })

	key := "test-expiry"
	body := []byte(`{"data":"test"}`)

	// Write a cache entry with an old timestamp
	cached := CachedGitHubResponse{
		Body:     body,
		CachedAt: time.Now().Add(-10 * time.Minute), // 10 minutes ago, beyond 5-min TTL
	}
	data, err := json.Marshal(cached)
	require.NoError(t, err)

	fileName := cacheKeyToFileName(key)
	err = os.WriteFile(filepath.Join(tmpDir, fileName), data, 0644)
	require.NoError(t, err)

	// Load should fail because cache is expired
	_, err = LoadGitHubCache(key)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cache expired")
}

func TestGitHubCacheOverwrite(t *testing.T) {
	origGetCacheDir := getCacheDirFunc
	tmpDir := t.TempDir()
	getCacheDirFunc = func() (string, error) { return tmpDir, nil }
	t.Cleanup(func() { getCacheDirFunc = origGetCacheDir })

	key := "test-overwrite"

	// Write first version
	err := SaveGitHubCache(key, []byte(`"old"`))
	require.NoError(t, err)

	// Overwrite with new version
	err = SaveGitHubCache(key, []byte(`"new"`))
	require.NoError(t, err)

	// Should get the new version
	cached, err := LoadGitHubCache(key)
	require.NoError(t, err)
	assert.Equal(t, []byte(`"new"`), cached.Body)
}

func TestCachedGitHubResponseIsValid(t *testing.T) {
	t.Parallel()

	t.Run("fresh cache is valid", func(t *testing.T) {
		c := &CachedGitHubResponse{CachedAt: time.Now()}
		assert.True(t, c.IsValid())
	})

	t.Run("4-minute-old cache is valid", func(t *testing.T) {
		c := &CachedGitHubResponse{CachedAt: time.Now().Add(-4 * time.Minute)}
		assert.True(t, c.IsValid())
	})

	t.Run("6-minute-old cache is invalid", func(t *testing.T) {
		c := &CachedGitHubResponse{CachedAt: time.Now().Add(-6 * time.Minute)}
		assert.False(t, c.IsValid())
	})
}

func TestCacheKeyToFileName(t *testing.T) {
	t.Parallel()

	// Same key should produce the same filename
	name1 := cacheKeyToFileName("test-key")
	name2 := cacheKeyToFileName("test-key")
	assert.Equal(t, name1, name2)

	// Different keys should produce different filenames
	name3 := cacheKeyToFileName("other-key")
	assert.NotEqual(t, name1, name3)

	// Should have the expected prefix
	assert.Contains(t, name1, "github-cache-")
	assert.Contains(t, name1, ".json")
}

func TestGitHubCacheCorruptedData(t *testing.T) {
	origGetCacheDir := getCacheDirFunc
	tmpDir := t.TempDir()
	getCacheDirFunc = func() (string, error) { return tmpDir, nil }
	t.Cleanup(func() { getCacheDirFunc = origGetCacheDir })

	key := "test-corrupted"
	fileName := cacheKeyToFileName(key)
	err := os.WriteFile(filepath.Join(tmpDir, fileName), []byte("not valid json"), 0644)
	require.NoError(t, err)

	_, err = LoadGitHubCache(key)
	assert.Error(t, err)
}
