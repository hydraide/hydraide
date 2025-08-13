package downloader

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDownloadHydraServerWithBasePath(t *testing.T) {
	// Step 1: Create base path in os.TempDir()/HydraideTest
	basePath := os.TempDir() + "/HydraideTest"
	err := os.MkdirAll(basePath, os.ModePerm)
	assert.NoError(t, err, "Failed to create base path")
	defer func() {
		if err := os.RemoveAll(basePath); err != nil {
			log.Printf("Failed to clean up base path: %v", err)
		}
	}()

	// Step 2: Create object using New method
	d := New()
	assert.NotNil(t, d, "New should return a non-nil BinaryDownloader")

	// Step 3: Call DownloadHydraServer
	err = d.DownloadHydraServer("latest", basePath)
	assert.NoError(t, err, "DownloadHydraServer should not return an error")

	// Step 4: Verify base path cleanup
	_, err = os.Stat(basePath)
	assert.NoError(t, err, "Base path should exist after test")
}

// TestNew tests the New function
func TestNew(t *testing.T) {
	d := New()
	assert.NotNil(t, d, "New should return a non-nil BinaryDownloader")

}

// TestSetCacheDir tests the SetCacheDir method
func TestSetCacheDir(t *testing.T) {
	d := &DefaultDownloader{}
	cacheDir := "/tmp/test-cache"
	d.SetCacheDir(cacheDir)
	assert.Equal(t, cacheDir, d.cacheDir, "SetCacheDir should set the cache directory")
}

// TestSetProgressCallback tests the SetProgressCallback method
func TestSetProgressCallback(t *testing.T) {
	d := &DefaultDownloader{}
	callback := func(downloaded, total int64, percent float64) {}
	d.SetProgressCallback(callback)
	assert.NotNil(t, d.progressCallback, "SetProgressCallback should set the progress callback")
}

// TestDownloadHydraServer tests the DownloadHydraServer method
func TestDownloadHydraServer(t *testing.T) {
	tempDir := os.TempDir() + "/HydraideTest"
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			log.Printf("Failed to clean up temp directory: %v", err)
		}
	}()

	d := &DefaultDownloader{httpClient: &http.Client{}}

	// Capture logs using the standard log package
	var logBuffer bytes.Buffer
	log.SetOutput(&logBuffer)

	err := d.DownloadHydraServer("latest", tempDir)
	assert.NoError(t, err, "DownloadHydraServer should not return an error")

	// Print captured logs
	fmt.Println("Captured Logs:\n", logBuffer.String())
}

// TestGetLatestVersion tests the GetLatestVersion method
func TestGetLatestVersion(t *testing.T) {
	d := &DefaultDownloader{
		httpClient: &http.Client{},
	}

	version, err := d.GetLatestVersion()
	assert.NoError(t, err, "GetLatestVersion should not return an error")
	assert.Equal(t, "server/v2.1.5", version, "GetLatestVersion should return the correct version")
}

// TestProgressReader tests the ProgressReader struct
func TestProgressReader(t *testing.T) {
	var progressCalled bool
	callback := func(downloaded, total int64, percent float64) {
		progressCalled = true
	}

	reader := &ProgressReader{
		Reader:   bytes.NewReader([]byte("test data")),
		Total:    9,
		Callback: callback,
	}

	buf := make([]byte, 4)
	_, err := reader.Read(buf)
	assert.NoError(t, err, "Read should not return an error")
	assert.True(t, progressCalled, "Callback should be called during Read")
}

// TestVerifyChecksum tests the verifyChecksum method
func TestVerifyChecksum(t *testing.T) {
	d := &DefaultDownloader{}
	file, err := os.CreateTemp("", "testfile")
	assert.NoError(t, err, "Temporary file creation should not return an error")
	defer func() {
		if err := os.Remove(file.Name()); err != nil {
			log.Printf("Failed to clean up temporary file: %v", err)
		}
	}()

	_, err = file.WriteString("test data")
	assert.NoError(t, err, "Writing to temporary file should not return an error")
	_ = file.Close()

	expectedChecksum := "sha256:9e6c6b7b8b8c8c8c8c8c8c8c8c8c8c8c8c8c8c8c8c8c8c8c8c8c8c8c8c8c8c8"
	err = d.verifyChecksum(file.Name(), expectedChecksum)
	assert.Error(t, err, "verifyChecksum should return an error for mismatched checksum")
}
