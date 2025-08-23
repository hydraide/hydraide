// Package downloader provides utilities for downloading hydraserver binaries from GitHub releases
package downloader

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
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
//	var d downloader.BinaryDownloader = downloader.New()
//	err := d.DownloadHydraServer("v1.2.3", "/usr/local/bin")
//	if err != nil {
//		log.Fatal(err)
//	}
//
// Notes:
// - This implementation supports archives (tar.gz and zip) and verifies SHA256 checksums from sibling .sha256 assets.
// - Extraction is done in pure Go (no external tar/zip executables required), so it is cross-platform.
// - The archive is cached; optional re-installation will reuse the cache if size matches and checksum passes.
// - Inside the archive we expect a binary named either "hydraide" (Unix) or "hydraide.exe" (Windows).
// - Asset names follow: hydraide-{os}-{arch}.tar.gz (+ .sha256). For Windows, .zip is also supported if released.
//
// GOOS/GOARCH mapping:
//   - linux, darwin, freebsd → tar.gz
//   - windows → zip preferred (tar.gz also supported if present)
//   - arm → armv7 asset name (current HydrAIDE release convention)
//
// Security:
//   - Extraction prevents path traversal by ensuring each file stays within the destination directory.
//   - Checksum must match before extraction proceeds.
//
// (we keep exported names as-is to remain API-compatible)
//
//nolint:govet
type BinaryDownloader interface {
	// DownloadHydraServer downloads the hydraserver binary for the specified version
	// If version is empty or "latest", it downloads the latest release
	// returns the downloaded version tag (e.g., "v1.2.3") or an error
	DownloadHydraServer(version string, basePath string) (string, error)

	// GetLatestVersion returns the latest available version tag
	GetLatestVersion() (string, error)

	// GetLatestVersionWithoutServerPrefix returns the latest version tag without the "server/" prefix
	GetLatestVersionWithoutServerPrefix() string

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
// - Digest: (Not provided by GitHub API for release assets; ignored if empty.)
// Used to identify and download the correct binary and checksum files from a release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
}

// GitHubRelease represents a GitHub release response from the GitHub API.
//
// Fields:
// - TagName: The version tag of the release (e.g., "v1.2.3").
// - Assets: List of assets (files) attached to the release.
// - Draft / Prerelease: Filtering controls.
//
// Used to select the correct archive and checksum for a given version.
type GitHubRelease struct {
	TagName    string  `json:"tag_name"`
	Assets     []Asset `json:"assets"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
}

// DefaultDownloader is the main implementation of the BinaryDownloader interface.
//
// Fields:
// - cacheDir: Directory where downloaded files are cached.
// - progressCallback: Optional callback for reporting download progress.
// - httpClient: HTTP client for network requests.
//
// Behavior:
// - Downloads hydraserver archives from GitHub releases.
// - Verifies checksum via sibling .sha256 asset.
// - Extracts archive (tar.gz / zip) in pure Go.
// - Installs binary into target basePath with proper permissions.
type DefaultDownloader struct {
	cacheDir         string
	progressCallback ProgressCallback
	httpClient       *http.Client
}

// -----------------------
// Global constants
// -----------------------

// Global constants used throughout the downloader package.
//
// OWNER / REPO:
//
//	Identify the official GitHub repository where HydrAIDE
//	binaries are published.
//
// TEMP_FILENAME:
//
//	Name of the temporary folder used as a base for caching
//	downloaded release assets.
//
// HTTP_TIMEOUT:
//
//	Default timeout (in minutes) for HTTP requests to GitHub
//	or when fetching binary assets.
//
// WINDOWS_OS:
//
//	Constant for detecting Windows platforms.
const (
	OWNER         = "hydraide"
	REPO          = "hydraide"
	TEMP_FILENAME = "hydraide-cache"
	HTTP_TIMEOUT  = 10 // minutes
	WINDOWS_OS    = "windows"
)

// -----------------------
// Constructors & Setters
// -----------------------

// New creates a new DefaultDownloader instance.
//
// Behavior:
// - Initializes a new HTTP client with a default timeout.
// - Returns the instance as a BinaryDownloader interface.
//
// This is the recommended entrypoint for obtaining a downloader.
func New() BinaryDownloader {
	return &DefaultDownloader{
		httpClient: &http.Client{Timeout: HTTP_TIMEOUT * time.Minute},
	}
}

// SetCacheDir sets the directory where downloaded binaries will be cached.
//
// Notes:
//   - By default, a temporary OS-specific directory is used.
//   - Providing a custom cache directory ensures binaries persist
//     across runs or can be shared between processes.
func (d *DefaultDownloader) SetCacheDir(dir string) {
	d.cacheDir = dir
	fmt.Println("SetCacheDir executed")
}

// SetProgressCallback attaches a callback function to report
// download progress in real time.
//
// The callback receives:
// - downloaded bytes
// - total bytes
// - percentage (0.0–100.0)
//
// This allows integrations (CLI, UI, logging) to provide
// feedback during long downloads.
func (d *DefaultDownloader) SetProgressCallback(callback ProgressCallback) {
	d.progressCallback = callback
}

// -----------------------
// GitHub API Helpers
// -----------------------

// GetLatestVersion queries the hydraide/hydraide GitHub repository
// and returns the latest stable server version tag.
//
// - Drafts and prereleases are skipped.
// - Internally, this delegates to GetLatestVersionByPrefix("server/").
func (d *DefaultDownloader) GetLatestVersion() (string, error) {
	return d.GetLatestVersionByPrefix("server/")
}

func (d *DefaultDownloader) GetLatestVersionWithoutServerPrefix() string {
	// This method is not part of the BinaryDownloader interface,
	// but it can be useful for internal purposes.
	// It simply calls GetLatestVersionByPrefix with an empty prefix.
	version, err := d.GetLatestVersionByPrefix("server/")
	if err != nil {
		return "unknown"
	}
	// remove server prefix from the version tag
	if strings.HasPrefix(version, "server/") {
		version = strings.TrimPrefix(version, "server/")
	}
	return version
}

// GetLatestVersionByPrefix fetches the latest release tag that
// starts with the given prefix.
//
// Filtering rules:
// - Skips draft and prerelease entries.
// - Returns the first tag that matches the prefix.
//
// Usage example:
//
//	version, err := d.GetLatestVersionByPrefix("server/")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println("Latest server version:", version)
func (d *DefaultDownloader) GetLatestVersionByPrefix(prefix string) (string, error) {
	githubURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=100", OWNER, REPO)
	resp, err := d.githubGET(githubURL)
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
			return r.TagName, nil
		}
	}
	return "", fmt.Errorf("no release found with prefix %q", prefix)
}

// DownloadHydraServer downloads and installs the HydrAIDE server binary
// for the given version into the provided basePath.
//
// Supported archive formats:
// - .tar.gz (all platforms)
// - .zip (Windows preferred if available)
//
// Workflow:
//  1. Prepares and ensures the cache directory exists.
//  2. Resolves the desired version (specific or "latest").
//  3. Fetches release information from GitHub.
//  4. Selects the appropriate archive/checksum pair based on OS/arch.
//  5. Downloads the archive if not cached or if cache is invalid.
//  6. Verifies the archive against the provided SHA256 checksum (if available).
//  7. Extracts the archive contents (pure Go tar/zip extract).
//  8. Places the binary into the target basePath under the correct name
//     ("hydraide" for Unix, "hydraide.exe" for Windows).
//  9. Ensures correct file permissions on Unix systems.
//
// Caching behavior:
// - Files are cached under a temp directory (hydraide-cache).
// - Cache is keyed by version + archive name.
// - If cache file exists and matches expected size, re-download is skipped.
//
// Security:
// - Extraction prevents path traversal by validating target paths.
// - If checksum verification fails, the cached archive is deleted.
// - Extraction is performed in pure Go with no external dependencies.
//
// Parameters:
//   - version: Release tag to install (e.g., "server/v1.2.3").
//     If empty or "latest", the latest stable release will be used.
//   - basePath: Directory where the final binary will be placed.
//
// Returns:
// - error if any step fails (download, checksum, extraction, installation).
//
// Example:
//
//	d := downloader.New()
//	err := d.DownloadHydraServer("latest", "/usr/local/bin")
//	if err != nil {
//	    log.Fatalf("install failed: %v", err)
//	}
//
// Notes:
// - On Windows, .zip archives are preferred if available.
// - If no checksum is found, a warning is logged and installation proceeds.
// - If cross-device rename fails, a copy + delete fallback is used.
func (d *DefaultDownloader) DownloadHydraServer(version string, basePath string) (string, error) {
	// Prepare cache dir
	cacheDir := filepath.Join(os.TempDir(), TEMP_FILENAME)
	d.SetCacheDir(cacheDir)
	perm := os.ModePerm
	if runtime.GOOS != WINDOWS_OS {
		perm = 0o755
	}
	if err := os.MkdirAll(d.cacheDir, perm); err != nil {
		logger.Error("Failed to create cache directory", "dir", d.cacheDir, "error", err)
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Resolve version
	if version == "" || strings.EqualFold(version, "latest") {
		v, err := d.GetLatestVersion()
		if err != nil {
			return "", fmt.Errorf("failed to get latest version: %w", err)
		}
		version = v
	}
	logger.Info("Preparing to download hydraserver", "version", version)

	// Fetch release
	release, err := d.getReleaseByTag(version)
	if err != nil {
		return "", fmt.Errorf("failed to get release information: %w", err)
	}

	// Determine desired archive names
	archiveTGZ, checksumTGZ := d.archiveNamesTarGz()
	archiveZIP, checksumZIP := d.archiveNamesZip()

	// Prefer .zip on Windows if present, else tar.gz
	wantArchive, wantChecksum := archiveTGZ, checksumTGZ
	if runtime.GOOS == WINDOWS_OS {
		if hasAsset(release.Assets, archiveZIP) && hasAsset(release.Assets, wantChecksum) {
			wantArchive, wantChecksum = archiveZIP, checksumZIP
		} else if hasAsset(release.Assets, archiveZIP) && !hasAsset(release.Assets, checksumZIP) {
			// zip present but no zip checksum; fall back to tgz if tgz + sha exist
			if hasAsset(release.Assets, archiveTGZ) && hasAsset(release.Assets, checksumTGZ) {
				wantArchive, wantChecksum = archiveTGZ, checksumTGZ
			} else {
				wantArchive, wantChecksum = archiveZIP, "" // proceed without checksum (not ideal)
			}
		}
	}

	binAsset, shaAsset, err := selectAssetsByName(release.Assets, wantArchive, wantChecksum)
	if err != nil {
		return "", err
	}

	// Cache file path (include version to avoid collisions)
	versionClean := strings.TrimPrefix(version, "server/")
	cacheArchive := filepath.Join(d.cacheDir, fmt.Sprintf("%s_%s", versionClean, binAsset.Name))

	// Download archive (with progress)
	if !d.isCacheValid(cacheArchive, binAsset.Size) {
		logger.Info("Downloading hydraserver archive", "url", binAsset.BrowserDownloadURL, "destination", cacheArchive)
		if err := d.downloadFile(binAsset.BrowserDownloadURL, cacheArchive, binAsset.Size); err != nil {
			return "", fmt.Errorf("failed to download archive: %w", err)
		}
	}

	// Checksum verification (if available)
	if shaAsset != nil {
		sum, err := d.fetchChecksum(shaAsset.BrowserDownloadURL)
		if err != nil {
			_ = os.Remove(cacheArchive)
			return "", fmt.Errorf("failed to fetch checksum: %w", err)
		}
		if err := d.verifyChecksum(cacheArchive, sum); err != nil {
			_ = os.Remove(cacheArchive)
			return "", fmt.Errorf("checksum verification failed: %w", err)
		}
		logger.Info("Checksum verified successfully")
	} else {
		logger.Warn("No checksum asset found for archive; proceeding without verification", "archive", binAsset.Name)
	}

	// Ensure target directory exists
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return "", fmt.Errorf("failed to create target directory: %w", err)
	}

	// Extract to basePath
	var extractedPath string
	if strings.HasSuffix(strings.ToLower(binAsset.Name), ".zip") {
		extractedPath, err = d.extractZip(cacheArchive, basePath)
	} else {
		extractedPath, err = d.extractTarGz(cacheArchive, basePath)
	}
	if err != nil {
		return "", fmt.Errorf("failed to extract archive: %w", err)
	}

	// Determine final binary name and path
	finalName := "hydraide"
	if runtime.GOOS == WINDOWS_OS {
		finalName = "hydraide.exe"
	}
	targetPath := filepath.Join(basePath, finalName)

	// Move/rename extracted binary to final name if necessary
	if extractedPath != "" && !sameFilepath(extractedPath, targetPath) {
		if err := os.Rename(extractedPath, targetPath); err != nil {
			// possibly cross-device; fallback to copy
			if err2 := copyFile(extractedPath, targetPath); err2 != nil {
				return "", fmt.Errorf("install move failed: %v / copy fallback: %v", err, err2)
			}
			_ = os.Remove(extractedPath)
		}
	}

	// Permissions on Unix
	if runtime.GOOS != WINDOWS_OS {
		_ = os.Chmod(targetPath, 0o755)
	}

	logger.Info("Hydraserver installed", "path", targetPath)
	return versionClean, nil

}

// -----------------------
// HTTP helpers & GitHub API
// -----------------------

// githubGET performs an HTTP GET request against the given URL using the downloader's HTTP client.
//
// Behavior:
// - Ensures that the HTTP client is initialized (with timeout).
// - Sets the "Accept" header to request GitHub JSON responses.
// - Returns the raw *http.Response for further handling.
//
// Parameters:
// - u: fully qualified URL string to request.
//
// Returns:
// - *http.Response if successful (caller must close Body).
// - error if request construction or execution fails.
func (d *DefaultDownloader) githubGET(u string) (*http.Response, error) {
	d.ensureHTTP()
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	return d.httpClient.Do(req)
}

// getReleaseByTag retrieves metadata for a specific GitHub release tag.
//
// Workflow:
// 1. Builds the GitHub API URL for the given release tag.
// 2. Performs a GET request using githubGET().
// 3. Returns an error if:
//   - Release not found (404),
//   - API responds with non-200 status,
//   - JSON decoding fails.
//
// 4. On success, returns a GitHubRelease struct with tag name and asset list.
//
// Parameters:
// - tag: release tag string (e.g., "server/v1.2.3").
//
// Returns:
// - *GitHubRelease if found.
// - error otherwise.
//
// Logging:
// - Logs an error if fetching fails.
// - Logs info when release is successfully fetched with asset count.
func (d *DefaultDownloader) getReleaseByTag(tag string) (*GitHubRelease, error) {
	githubURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", OWNER, REPO, url.PathEscape(tag))
	resp, err := d.githubGET(githubURL)
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
		return nil, fmt.Errorf("release %s not found", tag)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d for release %s", resp.StatusCode, tag)
	}
	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode release response: %w", err)
	}
	logger.Info("Fetched hydraserver release by tag", "tag", release.TagName, "assets_count", len(release.Assets))
	return &release, nil
}

// -----------------------
// Asset selection & naming
// -----------------------

// hasAsset checks whether a release asset list contains a file with the given name.
//
// Parameters:
// - assets: slice of Asset objects from GitHubRelease.
// - name: filename to check for.
//
// Returns:
// - true if asset with exact name exists.
// - false otherwise.
func hasAsset(assets []Asset, name string) bool {
	for _, a := range assets {
		if a.Name == name {
			return true
		}
	}
	return false
}

// selectAssetsByName locates the requested archive and its checksum asset
// by matching names in the release's asset list.
//
// Parameters:
// - assets: slice of Asset objects from GitHubRelease.
// - wantedArchive: required archive filename (e.g., hydraide-linux-amd64.tar.gz).
// - wantedChecksum: optional checksum filename (may be empty).
//
// Returns:
// - *Asset for archive,
// - *Asset for checksum (nil if not found or not required),
// - error if archive not found.
//
// Notes:
// - The checksum asset is optional (some releases may omit for .zip).
// - Caller must handle the case where shaA == nil.
func selectAssetsByName(assets []Asset, wantedArchive, wantedChecksum string) (*Asset, *Asset, error) {
	var archA *Asset
	var shaA *Asset
	for i := range assets {
		a := &assets[i]
		if a.Name == wantedArchive {
			archA = a
		}
		if wantedChecksum != "" && a.Name == wantedChecksum {
			shaA = a
		}
	}
	if archA == nil {
		return nil, nil, fmt.Errorf("asset not found: %s", wantedArchive)
	}
	return archA, shaA, nil
}

// targetTriplet resolves the OS/architecture triplet
// used in HydrAIDE binary asset naming.
//
// Behavior:
//   - Uses runtime.GOOS and runtime.GOARCH.
//   - Special case: if arch == "arm", maps to "armv7"
//     (to match current HydrAIDE release convention).
//
// Returns:
// - osName: runtime OS (e.g., "linux", "windows").
// - arch: normalized architecture string.
func (d *DefaultDownloader) targetTriplet() (osName, arch string) {
	osName = runtime.GOOS
	arch = runtime.GOARCH
	if arch == "arm" {
		arch = "armv7"
	}
	return
}

// archiveBaseName builds the base filename prefix
// for HydrAIDE release assets based on OS/arch.
//
// Example output:
// - "hydraide-linux-amd64"
// - "hydraide-windows-arm64"
func (d *DefaultDownloader) archiveBaseName() string {
	osName, arch := d.targetTriplet()
	return fmt.Sprintf("hydraide-%s-%s", osName, arch)
}

// archiveNamesTarGz returns the expected archive filename
// and its checksum filename for tar.gz assets.
//
// Example:
// - "hydraide-linux-amd64.tar.gz"
// - "hydraide-linux-amd64.tar.gz.sha256"
func (d *DefaultDownloader) archiveNamesTarGz() (archive, checksum string) {
	base := d.archiveBaseName()
	return base + ".tar.gz", base + ".tar.gz.sha256"
}

// archiveNamesZip returns the expected archive filename
// and its checksum filename for zip assets.
//
// Example:
// - "hydraide-windows-amd64.zip"
// - "hydraide-windows-amd64.zip.sha256"
func (d *DefaultDownloader) archiveNamesZip() (archive, checksum string) {
	base := d.archiveBaseName()
	return base + ".zip", base + ".zip.sha256"
}

// -----------------------
// Cache & download helpers
// -----------------------

// isCacheValid checks whether a cached file exists and matches the expected size.
//
// Parameters:
// - cacheFile: full path to the cached archive.
// - expectedSize: expected file size in bytes (from GitHub release asset metadata).
//
// Returns:
// - true if the file exists and size matches.
// - false otherwise.
//
// Notes:
// - Does not verify checksum, only compares file size.
// - Used as a fast pre-check before attempting to re-download.
func (d *DefaultDownloader) isCacheValid(cacheFile string, expectedSize int64) bool {
	stat, err := os.Stat(cacheFile)
	if err != nil {
		return false
	}
	return stat.Size() == expectedSize
}

// downloadFile downloads a file from a given URL and saves it to destination.
//
// Workflow:
// 1. Ensures HTTP client is available.
// 2. Issues a GET request to srcURL.
// 3. Validates status code == 200 OK.
// 4. Creates the destination file.
// 5. Streams response body into file.
// 6. Optionally wraps response body in ProgressReader if a progressCallback is set.
// 7. Removes destination file if write fails.
//
// Parameters:
// - srcURL: direct download URL of the file.
// - destination: full path to write file to.
// - expectedSize: expected file size, used for progress reporting.
//
// Returns:
// - error if download or write fails.
// - nil if file saved successfully.
//
// Logging:
// - Logs error if request fails.
// - Logs info once file is downloaded successfully.
func (d *DefaultDownloader) downloadFile(srcURL, destination string, expectedSize int64) error {
	d.ensureHTTP()
	resp, err := d.httpClient.Get(srcURL)
	if err != nil {
		logger.Error("Failed to start download", "url", srcURL, "error", err)
		return fmt.Errorf("failed to start download: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Error("Failed to close response body", "url", srcURL, "error", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}
	file, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			logger.Error("Failed to close file", "file", destination, "error", err)
		}
	}()

	var reader io.Reader = resp.Body
	if d.progressCallback != nil && expectedSize > 0 {
		reader = &ProgressReader{Reader: resp.Body, Total: expectedSize, Callback: d.progressCallback}
	}
	if _, err := io.Copy(file, reader); err != nil {
		_ = os.Remove(destination)
		return fmt.Errorf("failed to write file: %w", err)
	}
	logger.Info("File downloaded successfully", "file", destination)
	return nil
}

// fetchChecksum downloads and parses a SHA256 checksum file.
//
// Behavior:
//   - Downloads the checksum file from the given URL.
//   - Accepts different formats:
//     "<hex>  filename", "sha256:<hex>", or just "<hex>".
//   - Normalizes the result to "sha256:<hex>" format.
//
// Parameters:
// - u: URL of the checksum file (e.g., *.sha256 asset).
//
// Returns:
// - checksum string in format "sha256:<hex>".
// - error if request fails or content cannot be read.
func (d *DefaultDownloader) fetchChecksum(u string) (string, error) {
	d.ensureHTTP()
	resp, err := d.httpClient.Get(u)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Error("Failed to close response body", "url", u, "error", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksum download failed: %d", resp.StatusCode)
	}
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(content))
	fields := strings.Fields(text)
	var sum string
	if len(fields) > 0 {
		sum = fields[0]
	} else {
		sum = text
	}
	if !strings.HasPrefix(sum, "sha256:") {
		sum = "sha256:" + sum
	}
	return sum, nil
}

// verifyChecksum verifies that a file's SHA256 hash matches the expected value.
//
// Workflow:
// 1. Opens the file and streams contents through a SHA256 hasher.
// 2. Encodes the hash to hex and prefixes with "sha256:".
// 3. Compares computed value against expected string.
// 4. Returns error if mismatch.
//
// Parameters:
// - filename: path to the file to verify.
// - expected: expected checksum string in format "sha256:<hex>".
//
// Returns:
// - nil if checksum matches.
// - error if file cannot be read or checksum does not match.
//
// Logging:
// - Logs successful verification with filename.
func (d *DefaultDownloader) verifyChecksum(filename, expected string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil {
			logger.Error("Failed to close file", "file", filename, "error", err)
		}
	}()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(hasher.Sum(nil))
	actual = "sha256:" + actual
	if actual != expected {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual)
	}
	logger.Info("Checksum verified", "file", filename)
	return nil
}

// -----------------------
// Extraction helpers
// -----------------------

// extractTarGz extracts a .tar.gz archive into the given destination directory.
//
// Workflow:
// 1. Opens the archive file and wraps it in a gzip reader.
// 2. Iterates through tar entries and processes each one:
//   - Validates that the target path stays inside destDir (prevents path traversal).
//   - Creates directories with 0755 permissions.
//   - Extracts regular files by streaming contents to disk.
//   - Detects the hydraide binary inside the archive:
//   - On Windows: matches "hydraide.exe" or any *.exe.
//   - On Unix: matches "hydraide" or "hydraide-*" and sets executable permission (0755).
//
// 3. Returns the path of the extracted binary if found.
//
// Parameters:
// - archivePath: path to the .tar.gz file.
// - destDir: directory where files should be extracted.
//
// Returns:
// - string: full path to the extracted hydraide binary (empty if not found).
// - error if archive cannot be opened, read, or extracted.
//
// Security:
// - Ensures no file escapes the destination directory (prevents Zip Slip / Tar Slip).
func (d *DefaultDownloader) extractTarGz(archivePath, destDir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := f.Close(); err != nil {
			logger.Error("Failed to close archive file", "file", archivePath, "error", err)
		}
	}()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := gz.Close(); err != nil {
			logger.Error("Failed to close gzip reader", "file", archivePath, "error", err)
		}
	}()
	tr := tar.NewReader(gz)
	var extractedBinary string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		target := filepath.Join(destDir, filepath.Clean(hdr.Name))
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return "", fmt.Errorf("illegal file path in archive: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return "", err
			}
			out, err := os.Create(target)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(out, tr); err != nil {
				if closeErr := out.Close(); closeErr != nil {
					logger.Error("Failed to close output file", "file", target, "error", closeErr)
				}
				return "", err
			}
			if oErr := out.Close(); oErr != nil {
				logger.Error("Failed to close output file", "file", target, "error", oErr)
				return "", oErr
			}
			base := filepath.Base(target)
			if runtime.GOOS == WINDOWS_OS {
				if base == "hydraide.exe" || strings.HasSuffix(strings.ToLower(base), ".exe") {
					extractedBinary = target
				}
			} else {
				if base == "hydraide" || strings.HasPrefix(base, "hydraide-") {
					extractedBinary = target
					_ = os.Chmod(target, 0o755)
				}
			}
		default:
			// ignore other types
		}
	}
	return extractedBinary, nil
}

// extractZip extracts a .zip archive into the given destination directory.
//
// Workflow:
// 1. Opens the archive file with zip.OpenReader.
// 2. Iterates through files and processes each one:
//   - Validates that the target path stays inside destDir (prevents path traversal).
//   - Creates directories with 0755 permissions.
//   - Extracts files by streaming contents to disk.
//   - Detects the hydraide binary inside the archive:
//   - On Windows: matches "hydraide.exe" or any *.exe.
//   - On Unix: matches "hydraide" or "hydraide-*" and sets executable permission (0755).
//
// 3. Returns the path of the extracted binary if found.
//
// Parameters:
// - archivePath: path to the .zip file.
// - destDir: directory where files should be extracted.
//
// Returns:
// - string: full path to the extracted hydraide binary (empty if not found).
// - error if archive cannot be opened, read, or extracted.
//
// Security:
// - Ensures no file escapes the destination directory (prevents Zip Slip).
func (d *DefaultDownloader) extractZip(archivePath, destDir string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := r.Close(); err != nil {
			logger.Error("Failed to close zip archive", "file", archivePath, "error", err)
		}
	}()
	var extractedBinary string
	for _, f := range r.File {
		target := filepath.Join(destDir, filepath.Clean(f.Name))
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return "", fmt.Errorf("illegal file path in archive: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", err
		}
		in, err := f.Open()
		if err != nil {
			return "", err
		}
		out, err := os.Create(target)
		if err != nil {
			if iErr := in.Close(); iErr != nil {
				logger.Error("Failed to close input file", "file", f.Name, "error", iErr)
			}
			return "", err
		}
		if _, err := io.Copy(out, in); err != nil {
			if iErr := in.Close(); iErr != nil {
				logger.Error("Failed to close input file", "file", f.Name, "error", iErr)
			}
			if oErr := out.Close(); oErr != nil {
				logger.Error("Failed to close output file", "file", target, "error", oErr)
			}
			return "", err
		}
		if iErr := in.Close(); iErr != nil {
			logger.Error("Failed to close input file", "file", f.Name, "error", iErr)
		}
		if oErr := out.Close(); oErr != nil {
			logger.Error("Failed to close output file", "file", target, "error", oErr)
		}

		base := filepath.Base(target)
		if runtime.GOOS == WINDOWS_OS {
			if base == "hydraide.exe" || strings.HasSuffix(strings.ToLower(base), ".exe") {
				extractedBinary = target
			}
		} else {
			if base == "hydraide" || strings.HasPrefix(base, "hydraide-") {
				extractedBinary = target
				_ = os.Chmod(target, 0o755)
			}
		}
	}
	return extractedBinary, nil
}

// -----------------------
// Misc helpers
// -----------------------

// ProgressReader wraps an io.Reader and tracks the number of bytes read.
// It invokes a ProgressCallback to report download progress.
//
// Fields:
// - Reader: underlying io.Reader (e.g., HTTP response body).
// - Total: total number of bytes expected to read.
// - Downloaded: number of bytes read so far.
// - Callback: optional function to report progress.
//
// Behavior:
//   - Each Read() call updates the Downloaded count.
//   - If Callback is set and Total > 0, it computes percentage progress
//     and calls Callback(downloaded, total, percent).
type ProgressReader struct {
	Reader     io.Reader
	Total      int64
	Downloaded int64
	Callback   ProgressCallback
}

// Read implements the io.Reader interface for ProgressReader.
// Updates progress statistics and calls the progress callback if defined.
//
// Parameters:
// - p: destination byte slice.
//
// Returns:
// - number of bytes read.
// - error from the underlying Reader.
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	pr.Downloaded += int64(n)
	if pr.Callback != nil && pr.Total > 0 {
		percent := float64(pr.Downloaded) / float64(pr.Total) * 100
		pr.Callback(pr.Downloaded, pr.Total, percent)
	}
	return n, err
}

// copyFile copies a file from src to dst using streaming I/O.
//
// Workflow:
// - Opens src file for reading.
// - Creates dst file for writing.
// - Streams content with io.Copy.
// - Closes files and returns error if any step fails.
//
// Parameters:
// - src: source file path.
// - dst: destination file path.
//
// Returns:
// - nil on success.
// - error if file cannot be opened, written, or closed.
func copyFile(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := s.Close(); err != nil {
			logger.Error("Failed to close source file", "file", src, "error", err)
		}
	}()
	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(d, s); err != nil {
		if dErr := d.Close(); dErr != nil {
			logger.Error("Failed to close destination file", "file", dst, "error", dErr)
		}
		return err
	}
	return d.Close()
}

// sameFilepath checks whether two file paths resolve to the same absolute path.
//
// Parameters:
// - a: first file path.
// - b: second file path.
//
// Returns:
// - true if both paths refer to the same absolute file.
// - false otherwise.
//
// Notes:
// - Ignores errors from filepath.Abs (empty string returned on error).
func sameFilepath(a, b string) bool {
	ap, _ := filepath.Abs(a)
	bp, _ := filepath.Abs(b)
	return ap == bp
}

// ensureHTTP ensures that the downloader has a valid HTTP client.
// If no client is set, it creates a new one with the default timeout.
//
// This is used internally before any network requests.
func (d *DefaultDownloader) ensureHTTP() {
	if d.httpClient == nil {
		d.httpClient = &http.Client{Timeout: HTTP_TIMEOUT * time.Minute}
	}
}
