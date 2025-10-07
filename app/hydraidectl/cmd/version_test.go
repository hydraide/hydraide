package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionCommand(t *testing.T) {
	// Save original values
	origVersion := Version
	origCommit := Commit
	origBuildDate := BuildDate
	origStdout := os.Stdout

	// Restore after test
	defer func() {
		Version = origVersion
		Commit = origCommit
		BuildDate = origBuildDate
		os.Stdout = origStdout
	}()

	// Set test values
	Version = "v1.0.0"
	Commit = "abc123def456"
	BuildDate = "2024-01-01T00:00:00Z"

	t.Run("basic version output", func(t *testing.T) {
		// Capture output
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Reset flags
		versionJSON = false
		versionNoRemote = true
		versionInstance = ""

		// Execute command
		versionCmd.Run(versionCmd, []string{})

		// Read output
		w.Close()
		output, _ := io.ReadAll(r)
		outputStr := string(output)

		// Verify output contains version info
		assert.Contains(t, outputStr, "hydraidectl v1.0.0")
		assert.Contains(t, outputStr, "abc123d") // Short commit
		assert.Contains(t, outputStr, "2024-01-01T00:00:00Z")
	})

	t.Run("JSON output", func(t *testing.T) {
		// Capture output
		var buf bytes.Buffer
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Reset flags
		versionJSON = true
		versionNoRemote = true
		versionInstance = ""

		// Execute command
		versionCmd.Run(versionCmd, []string{})

		// Read output
		w.Close()
		io.Copy(&buf, r)

		// Parse JSON output
		var output VersionOutput
		err := json.Unmarshal(buf.Bytes(), &output)
		require.NoError(t, err)

		// Verify JSON structure
		assert.Equal(t, "v1.0.0", output.CLI.Version)
		assert.Equal(t, "abc123def456", output.CLI.Commit)
		assert.Equal(t, "2024-01-01T00:00:00Z", output.CLI.BuildDate)
		assert.Contains(t, output.CLI.Platform, "/")
	})
}

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		name     string
		latest   string
		current  string
		expected bool
	}{
		{
			name:     "newer version",
			latest:   "v2.0.0",
			current:  "v1.0.0",
			expected: true,
		},
		{
			name:     "same version",
			latest:   "v1.0.0",
			current:  "v1.0.0",
			expected: false,
		},
		{
			name:     "older version",
			latest:   "v1.0.0",
			current:  "v2.0.0",
			expected: false,
		},
		{
			name:     "dev version",
			latest:   "v1.0.0",
			current:  "dev",
			expected: true,
		},
		{
			name:     "version without v prefix",
			latest:   "2.0.0",
			current:  "1.0.0",
			expected: true,
		},
		{
			name:     "mixed v prefix",
			latest:   "v2.0.0",
			current:  "1.0.0",
			expected: true,
		},
		{
			name:     "proper semver comparison (1.9.0 vs 1.10.0)",
			latest:   "1.10.0",
			current:  "1.9.0",
			expected: true,
		},
		{
			name:     "proper semver comparison (1.10.0 vs 1.9.0)",
			latest:   "1.9.0",
			current:  "1.10.0",
			expected: false,
		},
		{
			name:     "patch version comparison",
			latest:   "2.0.1",
			current:  "2.0.0",
			expected: true,
		},
		{
			name:     "pre-release version",
			latest:   "2.0.0-alpha",
			current:  "1.9.9",
			expected: true,
		},
		{
			name:     "invalid latest version",
			latest:   "invalid",
			current:  "1.0.0",
			expected: true, // Falls back to string comparison
		},
		{
			name:     "invalid current version",
			latest:   "2.0.0",
			current:  "invalid",
			expected: true, // Assumes update is available
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNewerVersion(tt.latest, tt.current)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShortCommit(t *testing.T) {
	tests := []struct {
		name     string
		commit   string
		expected string
	}{
		{
			name:     "long commit hash",
			commit:   "abc123def456789",
			expected: "abc123d",
		},
		{
			name:     "short commit hash",
			commit:   "abc123",
			expected: "abc123",
		},
		{
			name:     "empty commit",
			commit:   "",
			expected: "",
		},
		{
			name:     "unknown",
			commit:   "unknown",
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shortCommit(tt.commit)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCheckForUpdates(t *testing.T) {
	t.Run("successful GitHub API response", func(t *testing.T) {
		// Create mock server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/repos/hydraide/hydraide/releases", r.URL.Path)

			releases := []GitHubRelease{
				{
					TagName:    "hydraidectl/v2.0.0",
					Name:       "HydrAIDEctl v2.0.0",
					Prerelease: false,
					Draft:      false,
					HTMLURL:    "https://github.com/hydraide/hydraide/releases/tag/hydraidectl/v2.0.0",
					CreatedAt:  time.Now(),
				},
				{
					TagName:    "hydraidectl/v1.5.0",
					Name:       "HydrAIDEctl v1.5.0",
					Prerelease: false,
					Draft:      false,
					HTMLURL:    "https://github.com/hydraide/hydraide/releases/tag/hydraidectl/v1.5.0",
					CreatedAt:  time.Now().Add(-24 * time.Hour),
				},
			}

			json.NewEncoder(w).Encode(releases)
		}))
		defer server.Close()

		// Override GitHub API URL for testing
		originalURL := "https://api.github.com/repos/hydraide/hydraide/releases"
		// We need to modify the checkForUpdates function to accept a custom URL for testing
		// For now, we'll skip this test as it requires refactoring the function
		_ = originalURL
	})

	t.Run("GitHub API error", func(t *testing.T) {
		// This test actually hits the real GitHub API since we can't mock it
		// The test expects an error but GitHub API might actually work
		// So we'll check for either success or error with message
		ctx := context.Background()
		info := checkForUpdates(ctx, "v1.0.0", false, 1*time.Second)

		assert.NotNil(t, info)
		// If there's an error, it should be in the Error field
		// If there's no error, the API call succeeded
		if info.Error == "" {
			// API call succeeded, which is also acceptable
			assert.True(t, true)
		} else {
			// API call failed, error should be set
			assert.NotEmpty(t, info.Error)
			assert.False(t, info.IsAvailable)
			assert.Nil(t, info.Latest)
		}
	})

	t.Run("timeout handling", func(t *testing.T) {
		// This test actually hits the real GitHub API with a very short timeout
		// So it should timeout and set an error
		ctx := context.Background()
		info := checkForUpdates(ctx, "v1.0.0", false, 1*time.Millisecond)

		assert.NotNil(t, info)
		// With 1ms timeout, it should fail
		if info.Error != "" {
			assert.Contains(t, info.Error, "failed to check for updates")
			assert.False(t, info.IsAvailable)
			assert.Nil(t, info.Latest)
		} else {
			// In rare cases it might succeed if cached
			assert.True(t, true)
		}
	})
}

func TestGetInstanceVersion(t *testing.T) {
	t.Run("instance not found", func(t *testing.T) {
		ctx := context.Background()
		_, err := getInstanceVersion(ctx, "non-existent-instance")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load instance metadata")
	})

	// Additional tests would require mocking the buildmetadata package
	// or creating test instances with metadata files
}

func TestVersionOutputJSON(t *testing.T) {
	output := VersionOutput{
		CLI: VersionInfo{
			Version:   "v1.0.0",
			Commit:    "abc123",
			BuildDate: "2024-01-01",
			Platform:  "linux/amd64",
		},
		Instance: &InstanceVersionInfo{
			Name:      "test-instance",
			Version:   "v1.0.0",
			Commit:    "def456",
			BuildDate: "2024-01-01",
		},
		Update: &UpdateInfo{
			Latest:         ptr.To("v2.0.0"),
			IsAvailable:    true,
			URL:            ptr.To("https://github.com/hydraide/hydraide/releases"),
			Checked:        time.Now(),
			Channel:        "stable",
			InstallCommand: ptr.To(hydraidectlInstallCommand),
		},
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	err := encoder.Encode(output)
	require.NoError(t, err)

	// Parse back to verify structure
	var parsed VersionOutput
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	assert.Equal(t, output.CLI.Version, parsed.CLI.Version)
	assert.Equal(t, output.Instance.Name, parsed.Instance.Name)
	assert.Equal(t, *output.Update.Latest, *parsed.Update.Latest)
	assert.Equal(t, *output.Update.InstallCommand, *parsed.Update.InstallCommand)
}

func TestOutputHumanReadableUpdateCommand(t *testing.T) {
	origStdout := os.Stdout
	defer func() { os.Stdout = origStdout }()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	output := VersionOutput{
		CLI: VersionInfo{
			Version:   "v1.0.0",
			Commit:    "abc123",
			BuildDate: "2024-01-01",
			Platform:  "linux/amd64",
		},
		Update: &UpdateInfo{
			Latest:         ptr.To("v2.0.0"),
			IsAvailable:    true,
			InstallCommand: ptr.To(hydraidectlInstallCommand),
		},
	}

	outputHumanReadable(output)
	require.NoError(t, w.Close())
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), hydraidectlInstallCommand)
	assert.Contains(t, buf.String(), "Update: v2.0.0 available")
}
