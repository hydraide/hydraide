// Package updatecache provides caching for hydraidectl update checks.
// This prevents excessive GitHub API calls by caching update information locally.
package updatecache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	// CacheFileName is the name of the cache file
	CacheFileName = "update-cache.json"
	// CacheDuration is how long the cache is valid (24 hours)
	CacheDuration = 24 * time.Hour
)

// CachedUpdateInfo represents cached update check information
type CachedUpdateInfo struct {
	Latest      *string   `json:"latest,omitempty"`
	IsAvailable bool      `json:"isAvailable"`
	URL         *string   `json:"url,omitempty"`
	CheckedAt   time.Time `json:"checkedAt"`
	ForVersion  string    `json:"forVersion"` // The CLI version this check was for
}

// getCacheDir returns the hydraidectl cache directory path
func getCacheDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".hydraidectl"), nil
}

// getCachePath returns the full path to the cache file
func getCachePath() (string, error) {
	cacheDir, err := getCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, CacheFileName), nil
}

// Load reads the cached update info from disk
func Load() (*CachedUpdateInfo, error) {
	cachePath, err := getCachePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}

	var cache CachedUpdateInfo
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}

	return &cache, nil
}

// Save writes the update info to the cache file
func Save(info *CachedUpdateInfo) error {
	cacheDir, err := getCacheDir()
	if err != nil {
		return err
	}

	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return err
	}

	cachePath := filepath.Join(cacheDir, CacheFileName)

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(cachePath, data, 0644)
}

// IsValid checks if the cache is still valid for the given CLI version
func (c *CachedUpdateInfo) IsValid(currentVersion string) bool {
	// Cache is invalid if it was for a different CLI version
	if c.ForVersion != currentVersion {
		return false
	}

	// Cache is invalid if it's older than CacheDuration
	return time.Since(c.CheckedAt) < CacheDuration
}

// Clear removes the cache file
func Clear() error {
	cachePath, err := getCachePath()
	if err != nil {
		return err
	}
	return os.Remove(cachePath)
}
