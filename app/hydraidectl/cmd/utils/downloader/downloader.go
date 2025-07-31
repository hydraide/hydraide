// Package downloader provides utilities for downloading hydraserver binaries from GitHub releases
package downloader

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

/*
BinaryDownloader defines the interface for downloading  binaries from GitHub releases https://github.com/hydraide/hydraide/releases.

This interface abstracts the logic for:
- Downloading a hydraserver binary for a specific or latest version.
- Querying the latest available version.
- Setting a cache directory for downloads.
- Setting a progress callback for reporting download progress.

Implementations should use these methods to provide a consistent download experience, including caching and progress reporting.

Example usage:

	var d downloader.BinaryDownloader = downloader.NewDownloader()
	err := d.DownloadHydraServer("v1.2.3", "/usr/local/bin")
	if err != nil {
		log.Fatal(err)
	}
*/
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

// ProgressCallback is called during download to report progress
/*
ProgressCallback is a function type used to report download progress.

Arguments:
- downloaded: Number of bytes downloaded so far.
- total: Total number of bytes to download.
- percent: Download progress as a percentage (0.0 to 100.0).

This allows the downloader to provide real-time feedback to the user interface or CLI.

Example:
	func(progress, total int64, percent float64) {
		fmt.Printf("Downloaded %d/%d bytes (%.2f%%)\n", progress, total, percent)
	}
*/
type ProgressCallback func(downloaded, total int64, percent float64)

// GitHubRelease represents a GitHub release response from API
/*
Asset represents a downloadable file (asset) attached to a GitHub release.

Fields:
- Name: The filename of the asset.
- BrowserDownloadURL: Direct URL to download the asset.
- Size: Size of the asset in bytes.

Used to identify and download the correct binary and checksum files from a release.
*/
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
}

/*
GitHubRelease represents a GitHub release response from the GitHub API.

Fields:
- TagName: The version tag of the release (e.g., "v1.2.3").
- Assets: List of assets (files) attached to the release.

Used to select the correct binary and checksum for a given version.
*/
type GitHubRelease struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// DefaultDownloader implements BinaryDownloader using GitHub API
/*
DefaultDownloader is the main implementation of the BinaryDownloader interface.

Purpose:
- Handles downloading hydraserver binaries from GitHub releases, including caching, progress reporting, and OS-specific logic.

Fields:
- cacheDir: Directory where downloaded binaries are cached to avoid redundant downloads.
- progressCallback: Optional callback for reporting download progress.
- httpClient: HTTP client used for all network requests (configurable for testing or custom timeouts).

This struct encapsulates all state and logic needed for robust, cross-platform binary downloads.
*/
type DefaultDownloader struct {
	cacheDir         string
	progressCallback ProgressCallback
	httpClient       *http.Client
}

/*
Global constants for downloader package.

- OWNER, REPO: GitHub repository owner and name, used to construct API URLs for hydraide releases.
- TEMP_FILENAME: Name for the cache directory used for temporary downloads.
- LINUX_OS, WINDOWS_OS, MAC_OS: OS identifiers for platform-specific logic.
- WINDOWS_BINARY_NAME, LINUX_MAC_BINARY_NAME: Expected binary names for each OS, used to select the correct asset from GitHub releases.
- HTTP_TIMEOUT: Timeout (in minutes) for HTTP client requests.

These constants centralize configuration and platform-specific values, making the downloader logic portable and maintainable.
*/
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

/*
NewDownloader creates a new DefaultDownloader instance.

Purpose:
- Constructs a DefaultDownloader with a pre-configured HTTP client and default timeout.
- Intended as the main entry point for users to obtain a BinaryDownloader implementation.

Recommended Temporary Cache Locations:
- Linux/macOS: /tmp/hydraide-cache
- Windows:     %TEMP%\hydraide-cache

Returns:
- BinaryDownloader: A new instance ready for use.

Example usage:

	d := downloader.NewDownloader()
	err := d.DownloadHydraServer("latest", "/usr/local/bin")
	if err != nil {
		log.Fatal(err)
	}
*/
func NewDownloader() BinaryDownloader {
	fmt.Println("Entered: NewDownloader()")
	defer fmt.Println("Exiting: NewDownloader()")
	return &DefaultDownloader{
		httpClient: &http.Client{
			Timeout: HTTP_TIMEOUT * time.Minute,
		},
	}
}

// SetCacheDir sets the cache directory for downloaded files
/*
SetCacheDir sets the directory where downloaded binaries will be cached.

Purpose:
- Allows the user or internal logic to specify a custom cache directory for storing downloaded files.
- Caching avoids redundant downloads and speeds up repeated operations.

Input:
- dir: Path to the directory to use for caching.

Example usage:
	d.SetCacheDir("/tmp/hydraide-cache")
*/
func (d *DefaultDownloader) SetCacheDir(dir string) {
	fmt.Printf("SetCacheDir called with dir: %s\n", dir)
	d.cacheDir = dir
	fmt.Println("SetCacheDir executed")
}

// SetProgressCallback sets the progress callback function
/*
SetProgressCallback sets the callback function to report download progress.

Purpose:
- Allows the user to receive real-time updates on download progress (bytes downloaded, total, percent).

Input:
- callback: Function of type ProgressCallback to be called during downloads.

Example usage:
	d.SetProgressCallback(func(downloaded, total int64, percent float64) {
		fmt.Printf("Progress: %d/%d (%.2f%%)\n", downloaded, total, percent)
	})
*/
func (d *DefaultDownloader) SetProgressCallback(callback ProgressCallback) {
	fmt.Println("SetProgressCallback called")
	d.progressCallback = callback
	fmt.Println("SetProgressCallback executed")
}

/*
DownloadHydraServer downloads the hydraserver binary for the specified version.

Purpose:
- Downloads the hydraserver binary for a given version (or the latest if version is empty/"latest").
- Handles caching, checksum verification, and installation to the specified basePath.
- Reports progress if a callback is set.

Inputs:
- version: Version tag to download (e.g., "v1.2.3" or "latest").
- basePath: Directory where the binary should be installed.

Output:
- error: Non-nil if download, verification, or installation fails.

Example usage:

	err := d.DownloadHydraServer("v1.2.3", "/usr/local/bin")
	if err != nil {
		log.Fatal(err)
	}
*/
func (d *DefaultDownloader) DownloadHydraServer(version string, basePath string) error {
	fmt.Println("Entered: DownloadHydraServer()")

	// Use OS temp directory with user-specific folder
	cacheDir := filepath.Join(os.TempDir(), TEMP_FILENAME)
	fmt.Printf("cacheDir set to: %s\n", cacheDir)
	d.SetCacheDir(cacheDir)
	fmt.Println("SetCacheDir called in DownloadHydraServer")

	perm := os.ModePerm
	fmt.Printf("Initial perm: %v\n", perm)

	if d.getOS() == LINUX_OS {
		fmt.Println("Detected Linux OS, setting perm to 0755")
		perm = 0755
	}
	if err := os.MkdirAll(d.cacheDir, perm); err != nil {
		fmt.Printf("Failed to create cache directory: %v\n", err)
		return fmt.Errorf("failed to create cache directory: %w", err)
	}
	fmt.Println("Cache directory created or already exists")

	// Resolve version
	if version == "" || version == "latest" {
		fmt.Println("Resolving latest version")
		latestVersion, err := d.GetLatestVersion()
		if err != nil {
			fmt.Printf("Failed to get latest version: %v\n", err)
			return fmt.Errorf("failed to get latest version: %w", err)
		}
		version = latestVersion
		fmt.Printf("Latest version resolved: %s\n", version)
	}

	fmt.Printf("🔍 Resolving hydraserver version: %s\n", version)

	// Get release information
	release, err := d.getReleaseByTag(version)
	if err != nil {
		fmt.Printf("Failed to get release information: %v\n", err)
		return fmt.Errorf("failed to get release information: %w", err)
	}
	fmt.Println("Release information obtained")

	// Determine the correct binary name for current OS
	binaryName := d.getBinaryNameForOS()
	fmt.Printf("Binary name for OS: %s\n", binaryName)

	// Find the binary asset
	var binaryAsset *Asset

	for _, asset := range release.Assets {
		fmt.Printf("Checking asset: %s\n", asset.Name)
		if asset.Name == binaryName {
			binaryAsset = &asset
			fmt.Println("Binary asset found", asset.Name)
		}
	}

	if binaryAsset == nil {
		fmt.Printf("Binary %s not found in release %s\n", binaryName, version)
		return fmt.Errorf("binary %s not found in release %s", binaryName, version)
	}
	fmt.Println("Binary asset confirmed")

	fmt.Printf("📦 Found binary: %s (%.2f MB)\n", binaryAsset.Name, float64(binaryAsset.Size)/1024/1024)

	// Check cache first
	cacheFile := filepath.Join(d.cacheDir, fmt.Sprintf("%s_%s", version, binaryName))
	fmt.Printf("Cache file path: %s\n", cacheFile)
	if d.isCacheValid(cacheFile, binaryAsset.Size) {
		fmt.Printf("🚀 Using cached binary: %s\n", cacheFile)
		fmt.Println("Calling installFromCache with cached binary")
		return d.installFromCache(cacheFile, basePath, binaryName)
	}
	fmt.Println("Cache not valid or not found, proceeding to download")

	// Download checksum if available

	fmt.Printf("Checksum : Expected CHecksum  %s \n", binaryAsset.Digest)

	// Download binary to cache
	fmt.Printf("⬇️  Downloading %s...\n", binaryAsset.Name)
	if err := d.downloadFile(binaryAsset.BrowserDownloadURL, cacheFile, binaryAsset.Size); err != nil {
		fmt.Printf("Failed to download binary: %v\n", err)
		return fmt.Errorf("failed to download binary: %w", err)
	}
	fmt.Println("Binary downloaded to cache")

	// Verify checksum if available
	if binaryAsset.Digest != "" {
		fmt.Printf("🔍 Verifying checksum...\n")
		if err := d.verifyChecksum(cacheFile, binaryAsset.Digest); err != nil {
			fmt.Printf("Checksum verification failed: %v\n", err)
			os.Remove(cacheFile)
			fmt.Println("Invalid cache file removed")
			return fmt.Errorf("checksum verification failed: %w", err)
		}
		fmt.Printf("✅ Checksum verified successfully\n")
	}
	fmt.Println("Checksum verification (if any) complete")

	// Install from cache
	fmt.Println("Calling installFromCache after download")
	return d.installFromCache(cacheFile, basePath, binaryName)
}

/*
GetLatestVersion returns the latest available version tag from the hydraide GitHub repository.

Purpose:
- Queries the GitHub API for the latest release version tag.

Inputs:
- None.

Outputs:
- string: The latest version tag (e.g., "v1.2.3").
- error: Non-nil if the API call or decoding fails.

Example usage:

	latest, err := d.GetLatestVersion()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Latest version:", latest)
*/
func (d *DefaultDownloader) GetLatestVersion() (string, error) {
	fmt.Println("Entered: GetLatestVersion()")
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", OWNER, REPO)
	fmt.Printf("GitHub latest release URL: %s\n", url)

	resp, err := d.httpClient.Get(url)
	if err != nil {
		fmt.Printf("Failed to fetch latest release: %v\n", err)
		return "", fmt.Errorf("failed to fetch latest release: %w", err)
	}
	defer func() {
		resp.Body.Close()
		fmt.Println("Closed response body in GetLatestVersion")
	}()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("GitHub API returned status %d\n", resp.StatusCode)
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		fmt.Printf("Failed to decode release response: %v\n", err)
		return "", fmt.Errorf("failed to decode release response: %w", err)
	}
	fmt.Printf("Latest version tag: %s\n", release.TagName)

	return release.TagName, nil
}

/*
getReleaseByTag fetches release information for a specific version tag from the hydraide GitHub repository.

Purpose:
- Queries the GitHub API for the release corresponding to the given tag.

Inputs:
- tag: Version tag to fetch (e.g., "v1.2.3").

Outputs:
- *GitHubRelease: Pointer to the release information (assets, tag name).
- error: Non-nil if the API call, decoding, or tag lookup fails.

Example usage:

	release, err := d.getReleaseByTag("v1.2.3")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Assets:", release.Assets)
*/
func (d *DefaultDownloader) getReleaseByTag(tag string) (*GitHubRelease, error) {
	fmt.Printf("Entered: getReleaseByTag(%s)\n", tag)

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", OWNER, REPO, tag)
	fmt.Printf("GitHub release by tag URL: %s\n", url)

	resp, err := d.httpClient.Get(url)
	if err != nil {
		fmt.Printf("Failed to fetch release %s: %v\n", tag, err)
		return nil, fmt.Errorf("failed to fetch release %s: %w", tag, err)
	}
	defer func() {
		resp.Body.Close()
		fmt.Println("Closed response body in getReleaseByTag")
	}()

	if resp.StatusCode == http.StatusNotFound {
		fmt.Printf("Release %s not found\n", tag)
		return nil, fmt.Errorf("release %s not found", tag)
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("GitHub API returned status %d for release %s\n", resp.StatusCode, tag)
		return nil, fmt.Errorf("GitHub API returned status %d for release %s", resp.StatusCode, tag)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		fmt.Printf("Failed to decode release response: %v\n", err)
		return nil, fmt.Errorf("failed to decode release response: %w", err)
	}
	fmt.Printf("Release tag: %s, assets count: %d\n", release.TagName, len(release.Assets))

	return &release, nil
}

/*
getOS returns the current operating system name as a string.

Purpose:
- Provides a platform-agnostic way to determine the OS for binary selection and permission logic.

Inputs:
- None.

Outputs:
- string: The OS name (e.g., "linux", "windows", "darwin").

Example usage:

	osName := d.getOS()
	if osName == "windows" {
		// Use Windows-specific logic
	}
*/
func (d *DefaultDownloader) getOS() string {
	fmt.Println("getOS called")
	return runtime.GOOS
}

/*
getBinaryNameForOS returns the correct hydraserver binary filename for the current operating system.

Purpose:
- Selects the appropriate binary filename based on the OS, for use in asset selection and installation.

Inputs:
- None.

Outputs:
- string: The binary filename (e.g., "hydraide-windows-amd64.exe" or "hydraide-linux-amd64").

Example usage:

	binaryName := d.getBinaryNameForOS()
	fmt.Println("Download binary:", binaryName)
*/
func (d *DefaultDownloader) getBinaryNameForOS() string {
	fmt.Println("getBinaryNameForOS called")
	switch d.getOS() {
	case WINDOWS_OS:
		fmt.Println("OS is windows")
		return WINDOWS_BINARY_NAME
	case LINUX_OS:
		fmt.Println("OS is linux")
		return LINUX_MAC_BINARY_NAME
	case MAC_OS:
		fmt.Println("OS is darwin")
		return LINUX_MAC_BINARY_NAME
	default:
		fmt.Println("OS is default (linux/mac)")
		return LINUX_MAC_BINARY_NAME
	}
}

/*
isCacheValid checks if a cached file exists and matches the expected size.

Purpose:
- Determines whether a previously downloaded binary can be reused from cache.

Inputs:
- cacheFile: Path to the cached file.
- expectedSize: Expected file size in bytes.

Outputs:
- bool: true if the file exists and matches the expected size, false otherwise.

Example usage:

	if d.isCacheValid("/tmp/hydraide-cache/v1.2.3_hydraide-linux-amd64", 12345678) {
		// Use cached binary
	}
*/
func (d *DefaultDownloader) isCacheValid(cacheFile string, expectedSize int64) bool {
	fmt.Printf("isCacheValid called for file: %s, expected size: %d\n", cacheFile, expectedSize)
	stat, err := os.Stat(cacheFile)
	if err != nil {
		fmt.Printf("os.Stat error: %v\n", err)
		return false
	}
	fmt.Printf("Cache file size: %d\n", stat.Size())
	return stat.Size() == expectedSize
}

/*
downloadFile downloads a file from a URL to a destination path, with optional progress reporting.

Purpose:
- Handles the actual download of binary or checksum files, reporting progress if a callback is set.

Inputs:
- url: Source URL to download from.
- destination: Path to save the downloaded file.
- expectedSize: Expected size of the file in bytes (for progress reporting).

Outputs:
- error: Non-nil if the download or file write fails.

Example usage:

	err := d.downloadFile(asset.BrowserDownloadURL, "/tmp/hydraide-cache/binary", asset.Size)
	if err != nil {
		log.Fatal(err)
	}
*/
func (d *DefaultDownloader) downloadFile(url, destination string, expectedSize int64) error {
	fmt.Printf("downloadFile called: url=%s, destination=%s, expectedSize=%d\n", url, destination, expectedSize)
	resp, err := d.httpClient.Get(url)
	if err != nil {
		fmt.Printf("Failed to start download: %v\n", err)
		return fmt.Errorf("failed to start download: %w", err)
	}
	defer func() {
		resp.Body.Close()
		fmt.Println("Closed response body in downloadFile")
	}()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Download failed with status %d\n", resp.StatusCode)
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	file, err := os.Create(destination)
	if err != nil {
		fmt.Printf("Failed to create file: %v\n", err)
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() {
		file.Close()
		fmt.Println("Closed file in downloadFile")
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
		fmt.Printf("Failed to write file: %v\n", err)
		os.Remove(destination) // Clean up on error
		fmt.Println("Removed destination file due to error")
		return fmt.Errorf("failed to write file: %w", err)
	}
	fmt.Println("File downloaded successfully")

	return nil
}

// downloadChecksum downloads and parses a SHA256 checksum file
func (d *DefaultDownloader) downloadChecksum(url string) (string, error) {
	fmt.Printf("downloadChecksum called: url=%s\n", url)
	resp, err := d.httpClient.Get(url)
	if err != nil {
		fmt.Printf("Failed to get checksum: %v\n", err)
		return "", err
	}
	defer func() {
		resp.Body.Close()
		fmt.Println("Closed response body in downloadChecksum")
	}()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Checksum download failed with status %d\n", resp.StatusCode)
		return "", fmt.Errorf("checksum download failed with status %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Failed to read checksum content: %v\n", err)
		return "", err
	}
	fmt.Println("Checksum content read")

	// Parse checksum (format: "checksum filename")
	parts := strings.Fields(string(content))
	if len(parts) < 1 {
		fmt.Println("Invalid checksum format")
		return "", fmt.Errorf("invalid checksum format")
	}
	fmt.Printf("Checksum parsed: %s\n", strings.TrimSpace(parts[0]))

	return strings.TrimSpace(parts[0]), nil
}

// verifyChecksum verifies the SHA256 checksum of a file
func (d *DefaultDownloader) verifyChecksum(filename, expected string) error {
	fmt.Printf("verifyChecksum called: filename=%s, expected=%s\n", filename, expected)
	file, err := os.Open(filename)
	if err != nil {
		fmt.Printf("Failed to open file: %v\n", err)
		return err
	}
	defer func() {
		file.Close()
		fmt.Println("Closed file in verifyChecksum")
	}()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		fmt.Printf("Failed to hash file: %v\n", err)
		return err
	}
	fmt.Println("File hashed")

	actual := hex.EncodeToString(hasher.Sum(nil))
	actual = fmt.Sprintf("%s:%s", "sha256", actual)
	fmt.Printf("Actual checksum: %s\n", actual)
	if actual != expected {
		fmt.Printf("Checksum mismatch: expected %s, got %s\n", expected, actual)
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual)
	}
	fmt.Println("Checksum verified")

	return nil
}

// installFromCache copies the binary from cache to the target location
func (d *DefaultDownloader) installFromCache(cacheFile, basePath, binaryName string) error {
	fmt.Printf("installFromCache called: cacheFile=%s, basePath=%s, binaryName=%s\n", cacheFile, basePath, binaryName)
	targetPath := filepath.Join(basePath, binaryName)
	fmt.Printf("Target path: %s\n", targetPath)

	// Create source and destination files
	src, err := os.Open(cacheFile)
	if err != nil {
		fmt.Printf("Failed to open cache file: %v\n", err)
		return fmt.Errorf("failed to open cache file: %w", err)
	}
	defer func() {
		src.Close()
		fmt.Println("Closed src file in installFromCache")
	}()

	// Ensure target directory exists
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		fmt.Printf("Failed to create target directory: %v\n", err)
		return fmt.Errorf("failed to create target directory: %w", err)
	}
	fmt.Println("Target directory ensured")

	dst, err := os.Create(targetPath)
	if err != nil {
		fmt.Printf("Failed to create target file: %v\n", err)
		return fmt.Errorf("failed to create target file: %w", err)
	}
	defer func() {
		dst.Close()
		fmt.Println("Closed dst file in installFromCache")
	}()

	// Copy file
	if _, err := io.Copy(dst, src); err != nil {
		fmt.Printf("Failed to copy file: %v\n", err)
		return fmt.Errorf("failed to copy file: %w", err)
	}
	fmt.Println("File copied to target")

	// Make executable on Unix-like systems
	if runtime.GOOS != "windows" {
		fmt.Println("Making binary executable (Unix-like system)")
		if err := os.Chmod(targetPath, 0755); err != nil {
			fmt.Printf("Failed to make binary executable: %v\n", err)
			return fmt.Errorf("failed to make binary executable: %w", err)
		}
		fmt.Printf("✅ Binary made executable\n")
	}

	fmt.Printf("✅ hydraserver installed successfully: %s\n", targetPath)
	return nil
}

// ProgressReader wraps an io.Reader to report download progress
/*
ProgressReader wraps an io.Reader to provide download progress reporting.

Purpose:
- Tracks the number of bytes read from an underlying reader (typically an HTTP response body).
- Calls a ProgressCallback with the current progress, total size, and percent complete.

Fields:
- Reader: The underlying io.Reader to read from.
- Total: Total number of bytes expected (for percent calculation).
- Downloaded: Number of bytes read so far.
- Callback: Function to call with progress updates.

Used internally by DefaultDownloader to provide real-time progress feedback during downloads.
*/
type ProgressReader struct {
	Reader     io.Reader
	Total      int64
	Downloaded int64
	Callback   ProgressCallback
}

// Read implements io.Reader interface with progress reporting
/*
Read implements the io.Reader interface for ProgressReader.

Behavior:
- Reads up to len(p) bytes from the underlying Reader.
- Updates the Downloaded field with the number of bytes read.
- If Callback is set and Total > 0, calls the callback with the current progress and percent complete.

Inputs:
- p: Byte slice to read data into.

Outputs:
- n: Number of bytes read.
- err: Any error encountered during reading.

Example usage:
	var pr = &ProgressReader{Reader: resp.Body, Total: 1000, Callback: cb}
	buf := make([]byte, 512)
	for {
		n, err := pr.Read(buf)
		if err == io.EOF {
			break
		}
		// process buf[:n]
	}
*/
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	pr.Downloaded += int64(n)

	if pr.Callback != nil && pr.Total > 0 {
		percent := float64(pr.Downloaded) / float64(pr.Total) * 100
		pr.Callback(pr.Downloaded, pr.Total, percent)
	}

	return n, err
}
