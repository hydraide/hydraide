// Package updatecache provides caching for hydraidectl update checks.
// This prevents excessive GitHub API calls by caching update information locally.
package updatecache

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// CacheFileName is the name of the cache file
	CacheFileName = "update-cache.json"
	// CacheDuration is how long the cache is valid (24 hours)
	CacheDuration = 24 * time.Hour
	// GitHubCacheDuration is how long GitHub API responses are cached (5 minutes)
	GitHubCacheDuration = 5 * time.Minute
)

// CachedUpdateInfo represents cached update check information
type CachedUpdateInfo struct {
	Latest      *string   `json:"latest,omitempty"`
	IsAvailable bool      `json:"isAvailable"`
	URL         *string   `json:"url,omitempty"`
	CheckedAt   time.Time `json:"checkedAt"`
	ForVersion  string    `json:"forVersion"` // The CLI version this check was for
}

// getCacheDirFunc is the function used to resolve the cache directory.
// It can be overridden in tests.
var getCacheDirFunc = getCacheDirDefault

// getCacheDirDefault returns the hydraidectl cache directory path
func getCacheDirDefault() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".hydraidectl"), nil
}

// getCacheDir returns the hydraidectl cache directory path
func getCacheDir() (string, error) {
	return getCacheDirFunc()
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

// CachedGitHubResponse represents a cached HTTP response from the GitHub API
type CachedGitHubResponse struct {
	Body     []byte    `json:"body"`
	CachedAt time.Time `json:"cachedAt"`
}

// IsValid checks if the cached response is still within the TTL
func (c *CachedGitHubResponse) IsValid() bool {
	return time.Since(c.CachedAt) < GitHubCacheDuration
}

// cacheKeyToFileName converts a cache key (URL or logical name) to a safe filename
func cacheKeyToFileName(key string) string {
	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("github-cache-%x.json", hash[:8])
}

// LoadGitHubCache reads a cached GitHub API response for the given cache key
func LoadGitHubCache(key string) (*CachedGitHubResponse, error) {
	cacheDir, err := getCacheDir()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filepath.Join(cacheDir, cacheKeyToFileName(key)))
	if err != nil {
		return nil, err
	}

	var cached CachedGitHubResponse
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, err
	}

	if !cached.IsValid() {
		return nil, fmt.Errorf("cache expired")
	}

	return &cached, nil
}

// SaveGitHubCache writes a GitHub API response to the cache
func SaveGitHubCache(key string, body []byte) error {
	cacheDir, err := getCacheDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return err
	}

	cached := CachedGitHubResponse{
		Body:     body,
		CachedAt: time.Now(),
	}

	data, err := json.Marshal(cached)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(cacheDir, cacheKeyToFileName(key)), data, 0644)
}
