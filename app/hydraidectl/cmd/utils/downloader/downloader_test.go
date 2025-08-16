package downloader

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGetLatestVersionFlexible checks that we can fetch a latest tag
// without hard-coding an exact version string.
func TestGetLatestVersionFlexible(t *testing.T) {
	t.Parallel()
	d := New()
	v, err := d.GetLatestVersion()
	assert.NoError(t, err)
	if !assert.NotEmpty(t, v) {
		return
	}
	assert.True(t, strings.HasPrefix(v, "server/"), "latest version should start with 'server/' (got %s)", v)
}

// TestDownloadHydraServerLatest downloads the latest archive, verifies extraction
// and that the final binary exists at basePath/hydraide[.exe].
func TestDownloadHydraServerLatest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network download in short mode")
	}

	basePath := filepath.Join(os.TempDir(), "HydraideTest_Integration")
	_ = os.RemoveAll(basePath)
	assert.NoError(t, os.MkdirAll(basePath, 0o755))
	t.Cleanup(func() { _ = os.RemoveAll(basePath) })

	d := New()
	// Optional: show progress into a buffer to ensure callback is wired.
	var progCalled bool
	d.SetProgressCallback(func(downloaded, total int64, percent float64) {
		progCalled = true
	})

	err := d.DownloadHydraServer("latest", basePath)
	assert.NoError(t, err)

	bin := "hydraide"
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	finalPath := filepath.Join(basePath, bin)
	st, statErr := os.Stat(finalPath)
	if assert.NoError(t, statErr, "final binary should exist: %s", finalPath) {
		assert.False(t, st.IsDir())
	}
	assert.True(t, progCalled, "progress callback should have been called at least once")

	if runtime.GOOS != "windows" {
		mode := st.Mode()
		assert.Equal(t, os.FileMode(0o755), mode&0o777, "unix binaries should be executable (0755)")
	}
}

// TestProgressReader ensures the callback is invoked while reading.
func TestProgressReader(t *testing.T) {
	t.Parallel()
	var called bool
	cb := func(downloaded, total int64, percent float64) { called = true }
	pr := &ProgressReader{Reader: bytes.NewReader([]byte("0123456789")), Total: 10, Callback: cb}
	buf := make([]byte, 4)
	_, err := pr.Read(buf)
	assert.NoError(t, err)
	assert.True(t, called, "callback should be called during Read")
}

// TestVerifyChecksumMismatch writes a small temp file and expects a mismatch error.
func TestVerifyChecksumMismatch(t *testing.T) {
	t.Parallel()
	d := &DefaultDownloader{}
	f, err := os.CreateTemp("", "hydraide_verify_*.bin")
	assert.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(f.Name()) })
	_, _ = f.WriteString("test data")
	_ = f.Close()

	err = d.verifyChecksum(f.Name(), "sha256:0000000000000000000000000000000000000000000000000000000000000000")
	assert.Error(t, err, "verifyChecksum should fail on wrong expected hash")
}

// TestArchiveNameHelpers just sanity-checks the naming helpers produce expected suffixes.
func TestArchiveNameHelpers(t *testing.T) {
	t.Parallel()
	d := &DefaultDownloader{}
	archTGZ, sumTGZ := d.archiveNamesTarGz()
	archZIP, sumZIP := d.archiveNamesZip()
	assert.True(t, strings.HasSuffix(strings.ToLower(archTGZ), ".tar.gz"))
	assert.True(t, strings.HasSuffix(strings.ToLower(sumTGZ), ".tar.gz.sha256"))
	assert.True(t, strings.HasSuffix(strings.ToLower(archZIP), ".zip"))
	assert.True(t, strings.HasSuffix(strings.ToLower(sumZIP), ".zip.sha256"))
}

// TestTargetTriplet ensures non-empty mapping and arm→armv7 fallback where applicable.
func TestTargetTriplet(t *testing.T) {
	t.Parallel()
	d := &DefaultDownloader{}
	osName, arch := d.targetTriplet()
	assert.NotEmpty(t, osName)
	assert.NotEmpty(t, arch)
	if runtime.GOARCH == "arm" {
		assert.Equal(t, "armv7", arch)
	}
}

// Example-style doc test for readability (does not assert network).
func ExampleDefaultDownloader() {
	d := New()
	_ = d.DownloadHydraServer("latest", filepath.Join(os.TempDir(), "HydraideExample"))
	fmt.Println("ok")
	// Output: ok
}
