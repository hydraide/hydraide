// Package downloader provides utilities for downloading hydraserver binaries from GitHub releases
package downloader

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var logger = slog.New(slog.NewTextHandler(os.Stdout, nil))

// BinaryDownloader defines the interface for downloading hydraserver binaries from GitHub releases.
// This interface abstracts the logic for:
// - Downloading a hydraserver binary for a specific or latest version.
// - Querying the latest available version.
// - Setting a cache directory for downloads.
// - Setting a progress callback for reporting download progress.
// Implementations should use these methods to provide a consistent download experience, including caching and progress reporting.
// Example usage:
//
//	var d downloader.BinaryDownloader = downloader.NewDownloader()
//	err := d.DownloadHydraServer("v1.2.3", "/usr/local/bin")
//	if err != nil {
//		log.Fatal(err)
//	}
type BinaryDownloader interface {
	// DownloadHydraServer downloads the hydraserver binary for the specified version
	// If version is empty or "latest", it downloads the latest release
	DownloadHydraServer(version string, basePath string) error

	// GetLatestVersion returns the latest available version tag
	GetLatestVersion() (string, error)

	// SetCacheDir sets the cache directory
	SetCacheDir(dir string)

	// SetProgressCallback sets a callback function for download progress
	SetProgressCallback(callback ProgressCallback)
}

// ProgressCallback is a function type used to report download progress.
// Arguments:
// - downloaded: Number of bytes downloaded so far.
// - total: Total number of bytes to download.
// - percent: Download progress as a percentage (0.0 to 100.0).
// This allows the downloader to provide real-time feedback to the user interface or CLI.
// Example:
//
//	func(progress, total int64, percent float64) {
//		fmt.Printf("Downloaded %d/%d bytes (%.2f%%)\n", progress, total, percent)
//	}
type ProgressCallback func(downloaded, total int64, percent float64)

// Asset represents a downloadable file attached to a GitHub release.
// Fields:
// - Name: The filename of the asset.
// - BrowserDownloadURL: Direct URL to download the asset.
// - Size: Size of the asset in bytes.
// Used to identify and download the correct binary and checksum files from a release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
}

// GitHubRelease represents a GitHub release response from the GitHub API.
// Fields:
// - TagName: The version tag of the release (e.g., "v1.2.3").
// - Assets: List of assets (files) attached to the release.
// Used to select the correct binary and checksum for a given version.
type GitHubRelease struct {
	TagName    string  `json:"tag_name"`
	Assets     []Asset `json:"assets"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
}

// DefaultDownloader is the main implementation of the BinaryDownloader interface.
// Purpose:
// - Handles downloading hydraserver binaries from GitHub releases, including caching, progress reporting, and OS-specific logic.
// Fields:
// - cacheDir: Directory where downloaded binaries are cached to avoid redundant downloads.
// - progressCallback: Optional callback for reporting download progress.
// - httpClient: HTTP client used for all network requests (configurable for testing or custom timeouts).
// This struct encapsulates all state and logic needed for robust, cross-platform binary downloads.
type DefaultDownloader struct {
	cacheDir         string
	progressCallback ProgressCallback
	httpClient       *http.Client
}

// Global constants for downloader package.
// - OWNER, REPO: GitHub repository owner and name, used to construct API URLs for hydraide releases.
// - TEMP_FILENAME: Name for the cache directory used for temporary downloads.
// - LINUX_OS, WINDOWS_OS, MAC_OS: OS identifiers for platform-specific logic.
// - WINDOWS_BINARY_NAME, LINUX_MAC_BINARY_NAME: Expected binary names for each OS, used to select the correct asset from GitHub releases.
// - HTTP_TIMEOUT: Timeout (in minutes) for HTTP client requests.
// These constants centralize configuration and platform-specific values, making the downloader logic portable and maintainable.
const (
	OWNER                 = "hydraide"
	REPO                  = "hydraide"
	TEMP_FILENAME         = "hydraide-cache"
	LINUX_OS              = "linux"
	WINDOWS_OS            = "windows"
	MAC_OS                = "darwin"
	WINDOWS_BINARY_NAME   = "hydraide-windows-amd64.exe"
	LINUX_MAC_BINARY_NAME = "hydraide-linux-amd64"
	HTTP_TIMEOUT          = 10
)

// New creates a new DefaultDownloader instance.
// Purpose:
// - Constructs a DefaultDownloader with a pre-configured HTTP client and default timeout.
// - Intended as the main entry point for users to obtain a BinaryDownloader implementation.
// Recommended Temporary Cache Locations:
// - Linux/macOS: /tmp/hydraide-cache
// - Windows:     %TEMP%\hydraide-cache
// Returns:
// - BinaryDownloader: A new instance ready for use.
// Example usage:
//
//	d := downloader.NewDownloader()
//	err := d.DownloadHydraServer("latest", "/usr/local/bin")
//	if err != nil {
//		log.Fatal(err)
//	}
func New() BinaryDownloader {
	return &DefaultDownloader{
		httpClient: &http.Client{
			Timeout: HTTP_TIMEOUT * time.Minute,
		},
	}
}

// SetCacheDir sets the directory where downloaded binaries will be cached.
// Purpose:
// - Allows the user or internal logic to specify a custom cache directory for storing downloaded files.
// - Caching avoids redundant downloads and speeds up repeated operations.
// Input:
// - dir: Path to the directory to use for caching.
// Example usage:
//
//	d.SetCacheDir("/tmp/hydraide-cache")
func (d *DefaultDownloader) SetCacheDir(dir string) {
	d.cacheDir = dir
	fmt.Println("SetCacheDir executed")
}

// SetProgressCallback sets the callback function to report download progress.
// Purpose:
// - Allows the user to receive real-time updates on download progress (bytes downloaded, total, percent).
// Input:
// - callback: Function of type ProgressCallback to be called during downloads.
// Example usage:
//
//	d.SetProgressCallback(func(downloaded, total int64, percent float64) {
//		fmt.Printf("Progress: %d/%d (%.2f%%)\n", downloaded, total, percent)
//	})
func (d *DefaultDownloader) SetProgressCallback(callback ProgressCallback) {
	d.progressCallback = callback
}

// DownloadHydraServer downloads the hydraserver binary for the specified version.
// Purpose:
// - Downloads the hydraserver binary for a given version (or the latest if version is empty/"latest").
// - Handles caching, checksum verification, and installation to the specified basePath.
// - Reports progress if a callback is set.
// Inputs:
// - version: Version tag to download (e.g., "v1.2.3" or "latest").
// - basePath: Directory where the binary should be installed.
// Output:
// - error: Non-nil if download, verification, or installation fails.
// Example usage:
//
//	err := d.DownloadHydraServer("v1.2.3", "/usr/local/bin")
//	if err != nil {
//		log.Fatal(err)
//	}
func (d *DefaultDownloader) DownloadHydraServer(version string, basePath string) error {
	// Set up cache directory
	cacheDir := filepath.Join(os.TempDir(), TEMP_FILENAME)
	d.SetCacheDir(cacheDir)
	perm := os.ModePerm
	if d.getOS() == LINUX_OS {
		perm = 0755
	}
	if err := os.MkdirAll(d.cacheDir, perm); err != nil {
		logger.Error("Failed to create cache directory", "dir", d.cacheDir, "error", err)
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Resolve version
	if version == "" || version == "latest" {
		latestVersion, err := d.GetLatestVersion()
		if err != nil {
			logger.Error("Failed to get latest hydraserver version", "error", err)
			return fmt.Errorf("failed to get latest version: %w", err)
		}
		version = latestVersion
	}

	logger.Info("Preparing to download hydraserver", "version", version)

	// Get release information
	release, err := d.getReleaseByTag(version)
	if err != nil {
		logger.Error("Failed to get release information", "version", version, "error", err)
		return fmt.Errorf("failed to get release information: %w", err)
	}

	// Determine the correct binary name for current OS
	binaryName := d.getBinaryNameForOS()

	// Find the binary asset
	var binaryAsset *Asset
	for _, asset := range release.Assets {
		if asset.Name == binaryName {
			binaryAsset = &asset
			break
		}
	}

	if binaryAsset == nil {
		logger.Error("Binary not found in release", "binary", binaryName, "version", version)
		return fmt.Errorf("binary %s not found in release %s", binaryName, version)
	}

	logger.Info("Found hydraserver binary", "name", binaryAsset.Name, "version", version, "size_mb", float64(binaryAsset.Size)/1024/1024)

	version = strings.TrimPrefix(version, "server/")
	// Check cache first
	cacheFile := filepath.Join(d.cacheDir, fmt.Sprintf("%s_%s", version, binaryName))
	if d.isCacheValid(cacheFile, binaryAsset.Size) {
		logger.Info("Using cached hydraserver binary", "file", cacheFile)
		return d.installFromCache(cacheFile, basePath, binaryName)
	}

	logger.Info("Downloading hydraserver binary", "url", binaryAsset.BrowserDownloadURL, "destination", cacheFile)

	// Download binary to cache
	if err := d.downloadFile(binaryAsset.BrowserDownloadURL, cacheFile, binaryAsset.Size); err != nil {
		logger.Error("Failed to download hydraserver binary", "error", err)
		return fmt.Errorf("failed to download binary: %w", err)
	}

	// Verify checksum if available
	if binaryAsset.Digest != "" {
		logger.Info("Verifying checksum for downloaded binary")
		if err := d.verifyChecksum(cacheFile, binaryAsset.Digest); err != nil {
			logger.Error("Checksum verification failed", "error", err)
			if err := os.Remove(cacheFile); err != nil {
				logger.Error("Failed to remove invalid cached binary", "file", cacheFile, "error", err)
				return fmt.Errorf("failed to remove invalid cached binary: %w", err)
			}
			return fmt.Errorf("checksum verification failed: %w", err)
		}
		logger.Info("Checksum verified successfully")
	}

	logger.Info("Installing hydraserver binary", "target_dir", basePath)
	return d.installFromCache(cacheFile, basePath, binaryName)
}

func (d *DefaultDownloader) githubGET(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	return d.httpClient.Do(req)
}

func (d *DefaultDownloader) GetLatestVersionByPrefix(prefix string) (string, error) {
	// GitHub a legújabbaktól listáz, ezért elég az első 100-at megnézni
	githubUrl := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=100", OWNER, REPO)

	resp, err := d.githubGET(githubUrl)
	if err != nil {
		logger.Error("Failed to fetch releases list", "error", err)
		return "", fmt.Errorf("failed to fetch releases list: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Error("Failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		logger.Error("GitHub API returned error status when listing releases", "status", resp.StatusCode)
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var releases []GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		logger.Error("Failed to decode releases list", "error", err)
		return "", fmt.Errorf("failed to decode releases list: %w", err)
	}

	for _, r := range releases {
		if r.Draft || r.Prerelease {
			continue
		}
		if strings.HasPrefix(r.TagName, prefix) {
			logger.Info("Fetched latest version by prefix", "prefix", prefix, "version", r.TagName)
			return r.TagName, nil
		}
	}

	return "", fmt.Errorf("no release found with prefix %q", prefix)
}

// GetLatestVersion returns the latest available version tag from the hydraide GitHub repository.
// Purpose:
// - Queries the GitHub API for the latest release version tag.
// Inputs:
// - None.
// Outputs:
// - string: The latest version tag (e.g., "v1.2.3").
// - error: Non-nil if the API call or decoding fails.
// Example usage:
//
//	latest, err := d.GetLatestVersion()
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Println("Latest version:", latest)
func (d *DefaultDownloader) GetLatestVersion() (string, error) {
	// Backward-compatible default: server
	return d.GetLatestVersionByPrefix("server/")
}

// getReleaseByTag fetches release information for a specific version tag from the hydraide GitHub repository.
// Purpose:
// - Queries the GitHub API for the release corresponding to the given tag.
// Inputs:
// - tag: Version tag to fetch (e.g., "v1.2.3").
// Outputs:
// - *GitHubRelease: Pointer to the release information (assets, tag name).
// - error: Non-nil if the API call, decoding, or tag lookup fails.
// Example usage:
//
//	release, err := d.getReleaseByTag("v1.2.3")
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Println("Assets:", release.Assets)
func (d *DefaultDownloader) getReleaseByTag(tag string) (*GitHubRelease, error) {
	githubUrl := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", OWNER, REPO, url.PathEscape(tag))

	resp, err := d.httpClient.Get(githubUrl)
	if err != nil {
		logger.Error("Failed to fetch hydraserver release by tag", "tag", tag, "error", err)
		return nil, fmt.Errorf("failed to fetch release %s: %w", tag, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Error("Failed to close response body", "tag", tag, "error", err)
		}
	}()

	if resp.StatusCode == http.StatusNotFound {
		logger.Error("Hydraserver release not found for tag", "tag", tag)
		return nil, fmt.Errorf("release %s not found", tag)
	}

	if resp.StatusCode != http.StatusOK {
		logger.Error("GitHub API returned error status for release by tag", "tag", tag, "status", resp.StatusCode)
		return nil, fmt.Errorf("GitHub API returned status %d for release %s", resp.StatusCode, tag)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		logger.Error("Failed to decode release response", "tag", tag, "error", err)
		return nil, fmt.Errorf("failed to decode release response: %w", err)
	}

	logger.Info("Fetched hydraserver release by tag", "tag", release.TagName, "assets_count", len(release.Assets))
	return &release, nil
}

// getOS returns the current operating system name as a string.
// Purpose:
// - Provides a platform-agnostic way to determine the OS for binary selection and permission logic.
// Inputs:
// - None.
// Outputs:
// - string: The OS name (e.g., "linux", "windows", "darwin").
// Example usage:
//
//	osName := d.getOS()
//	if osName == "windows" {
//		// Use Windows-specific logic
//	}
func (d *DefaultDownloader) getOS() string {
	return runtime.GOOS
}

// getBinaryNameForOS returns the correct hydraserver binary filename for the current operating system.
// Purpose:
// - Selects the appropriate binary filename based on the OS, for use in asset selection and installation.
// Inputs:
// - None.
// Outputs:
// - string: The binary filename (e.g., "hydraide-windows-amd64.exe" or "hydraide-linux-amd64").
// Example usage:
//
//	binaryName := d.getBinaryNameForOS()
//	fmt.Println("Download binary:", binaryName)
func (d *DefaultDownloader) getBinaryNameForOS() string {
	switch d.getOS() {
	case WINDOWS_OS:
		return WINDOWS_BINARY_NAME
	case LINUX_OS:
		return LINUX_MAC_BINARY_NAME
	case MAC_OS:
		return LINUX_MAC_BINARY_NAME
	default:
		return LINUX_MAC_BINARY_NAME
	}
}

// isCacheValid checks if a cached file exists and matches the expected size.
// Purpose:
// - Determines whether a previously downloaded binary can be reused from cache.
// Inputs:
// - cacheFile: Path to the cached file.
// - expectedSize: Expected file size in bytes.
// Outputs:
// - bool: true if the file exists and matches the expected size, false otherwise.
// Example usage:
//
//	if d.isCacheValid("/tmp/hydraide-cache/v1.2.3_hydraide-linux-amd64", 12345678) {
//		// Use cached binary
//	}
func (d *DefaultDownloader) isCacheValid(cacheFile string, expectedSize int64) bool {
	stat, err := os.Stat(cacheFile)
	if err != nil {
		return false
	}
	return stat.Size() == expectedSize
}

// downloadFile downloads a file from a URL to a destination path, with optional progress reporting.
// Purpose:
// - Handles the actual download of binary or checksum files, reporting progress if a callback is set.
// Inputs:
// - url: Source URL to download from.
// - destination: Path to save the downloaded file.
// - expectedSize: Expected size of the file in bytes (for progress reporting).
// Outputs:
// - error: Non-nil if the download or file write fails.
// Example usage:
//
//	err := d.downloadFile(asset.BrowserDownloadURL, "/tmp/hydraide-cache/binary", asset.Size)
//	if err != nil {
//		log.Fatal(err)
//	}
func (d *DefaultDownloader) downloadFile(url, destination string, expectedSize int64) error {
	resp, err := d.httpClient.Get(url)
	if err != nil {
		logger.Error("Failed to start download", "url", url, "error", err)
		return fmt.Errorf("failed to start download: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Error("Failed to close response body", "url", url, "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		logger.Error("Download failed with HTTP error", "url", url, "status", resp.StatusCode)
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	file, err := os.Create(destination)
	if err != nil {
		logger.Error("Failed to create file for download", "file", destination, "error", err)
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			logger.Error("Failed to close downloaded file", "file", destination, "error", err)
		}
	}()

	// Create progress reader if callback is set
	var reader io.Reader = resp.Body
	if d.progressCallback != nil {
		reader = &ProgressReader{
			Reader:   resp.Body,
			Total:    expectedSize,
			Callback: d.progressCallback,
		}
	}

	_, err = io.Copy(file, reader)
	if err != nil {
		logger.Error("Failed to write downloaded file", "file", destination, "error", err)
		if err := os.Remove(destination); err != nil {
			logger.Error("Failed to remove invalid downloaded file", "file", destination, "error", err)
			return fmt.Errorf("failed to remove invalid file: %w", err)
		}
		return fmt.Errorf("failed to write file: %w", err)
	}

	logger.Info("File downloaded successfully", "file", destination)
	return nil
}

// downloadChecksum downloads and parses a SHA256 checksum file
func (d *DefaultDownloader) downloadChecksum(url string) (string, error) {
	resp, err := d.httpClient.Get(url)
	if err != nil {
		logger.Error("Failed to download checksum", "url", url, "error", err)
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Error("Failed to close checksum response body", "url", url, "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		logger.Error("Checksum download failed with HTTP error", "url", url, "status", resp.StatusCode)
		return "", fmt.Errorf("checksum download failed with status %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("Failed to read checksum content", "url", url, "error", err)
		return "", err
	}

	// Parse checksum (format: "checksum filename")
	parts := strings.Fields(string(content))
	if len(parts) < 1 {
		logger.Error("Invalid checksum format", "url", url)
		return "", fmt.Errorf("invalid checksum format")
	}

	return strings.TrimSpace(parts[0]), nil
}

// verifyChecksum verifies the SHA256 checksum of a file
func (d *DefaultDownloader) verifyChecksum(filename, expected string) error {
	file, err := os.Open(filename)
	if err != nil {
		logger.Error("Failed to open file for checksum verification", "file", filename, "error", err)
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			logger.Error("Failed to close file after checksum verification", "file", filename, "error", err)
		}
	}()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		logger.Error("Failed to hash file for checksum verification", "file", filename, "error", err)
		return err
	}

	actual := hex.EncodeToString(hasher.Sum(nil))
	actual = fmt.Sprintf("%s:%s", "sha256", actual)
	if actual != expected {
		logger.Error("Checksum mismatch", "file", filename, "expected", expected, "actual", actual)
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual)
	}

	logger.Info("Checksum verified", "file", filename)
	return nil
}

// installFromCache copies the binary from cache to the target location
func (d *DefaultDownloader) installFromCache(cacheFile, basePath, binaryName string) error {
	targetPath := filepath.Join(basePath, binaryName)

	// Create source and destination files
	src, err := os.Open(cacheFile)
	if err != nil {
		logger.Error("Failed to open cached binary for installation", "file", cacheFile, "error", err)
		return fmt.Errorf("failed to open cache file: %w", err)
	}
	defer func() {
		if err := src.Close(); err != nil {
			logger.Error("Failed to close source file after installation", "file", cacheFile, "error", err)
		}
	}()

	// Ensure target directory exists
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		logger.Error("Failed to create target directory for installation", "dir", filepath.Dir(targetPath), "error", err)
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	dst, err := os.Create(targetPath)
	if err != nil {
		logger.Error("Failed to create target file for installation", "file", targetPath, "error", err)
		return fmt.Errorf("failed to create target file: %w", err)
	}
	defer func() {
		if err := dst.Close(); err != nil {
			logger.Error("Failed to close destination file after installation", "file", targetPath, "error", err)
		}
	}()

	// Copy file
	if _, err := io.Copy(dst, src); err != nil {
		logger.Error("Failed to copy binary to target location", "src", cacheFile, "dst", targetPath, "error", err)
		return fmt.Errorf("failed to copy file: %w", err)
	}

	// Make executable on Unix-like systems
	if runtime.GOOS != "windows" {
		if err := os.Chmod(targetPath, 0755); err != nil {
			logger.Error("Failed to make binary executable", "file", targetPath, "error", err)
			return fmt.Errorf("failed to make binary executable: %w", err)
		}
	}

	logger.Info("Hydraserver downloaded successfully", "path", targetPath)
	return nil
}

// ProgressReader wraps an io.Reader to report download progress
// ProgressReader wraps an io.Reader to provide download progress reporting.
// Purpose:
// - Tracks the number of bytes read from an underlying reader (typically an HTTP response body).
// - Calls a ProgressCallback with the current progress, total size, and percent complete.
// Fields:
// - Reader: The underlying io.Reader to read from.
// - Total: Total number of bytes expected (for percent calculation).
// - Downloaded: Number of bytes read so far.
// - Callback: Function to call with progress updates.
// Used internally by DefaultDownloader to provide real-time progress feedback during downloads.
type ProgressReader struct {
	Reader     io.Reader
	Total      int64
	Downloaded int64
	Callback   ProgressCallback
}

// Read implements io.Reader interface with progress reporting
// Read implements the io.Reader interface for ProgressReader.
// Behavior:
// - Reads up to len(p) bytes from the underlying Reader.
// - Updates the Downloaded field with the number of bytes read.
// - If Callback is set and Total > 0, calls the callback with the current progress and percent complete.
// Inputs:
// - p: Byte slice to read data into.
// Outputs:
// - n: Number of bytes read.
// - err: Any error encountered during reading.
// Example usage:
//
//	var pr = &ProgressReader{Reader: resp.Body, Total: 1000, Callback: cb}
//	buf := make([]byte, 512)
//	for {
//		n, err := pr.Read(buf)
//		if err == io.EOF {
//			break
//		}
//		// process buf[:n]
//	}
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	pr.Downloaded += int64(n)

	if pr.Callback != nil && pr.Total > 0 {
		percent := float64(pr.Downloaded) / float64(pr.Total) * 100
		pr.Callback(pr.Downloaded, pr.Total, percent)
	}

	return n, err
}
