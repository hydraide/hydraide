package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/spf13/cobra"
)

// Build-time variables (set via ldflags)
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Command flags
var (
	versionInstance string
	versionJSON     bool
	versionNoRemote bool
	versionPre      bool
	versionTimeout  int
)

// VersionInfo represents CLI version information
type VersionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"buildDate"`
	Platform  string `json:"platform"`
}

// InstanceVersionInfo represents instance version information
type InstanceVersionInfo struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"buildDate"`
}

// UpdateInfo represents update check information
type UpdateInfo struct {
	Latest      *string   `json:"latest"`
	IsAvailable bool      `json:"isAvailable"`
	URL         *string   `json:"url"`
	Checked     time.Time `json:"checked"`
	Channel     string    `json:"channel"`
	Error       string    `json:"error,omitempty"` // Error message if update check failed
}

// VersionOutput represents the complete version command output
type VersionOutput struct {
	CLI      VersionInfo          `json:"cli"`
	Instance *InstanceVersionInfo `json:"instance,omitempty"`
	Update   *UpdateInfo          `json:"update,omitempty"`
}

// GitHubRelease represents a GitHub release
type GitHubRelease struct {
	TagName    string    `json:"tag_name"`
	Name       string    `json:"name"`
	Prerelease bool      `json:"prerelease"`
	Draft      bool      `json:"draft"`
	HTMLURL    string    `json:"html_url"`
	CreatedAt  time.Time `json:"created_at"`
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display version information",
	Long: `Display version information for hydraidectl CLI and optionally for a HydrAIDE instance.
Also checks for available updates on GitHub by default.

Exit codes:
  0 - Success
  1 - Error reading instance version
  2 - Other errors`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		output := VersionOutput{
			CLI: VersionInfo{
				Version:   Version,
				Commit:    Commit,
				BuildDate: BuildDate,
				Platform:  runtime.GOOS + "/" + runtime.GOARCH,
			},
		}

		// Read instance version if requested
		if versionInstance != "" {
			instanceVer, err := getInstanceVersion(ctx, versionInstance)
			if err != nil {
				if versionJSON {
					outputJSON(output)
				} else {
					fmt.Fprintf(os.Stderr, "Error reading instance version: %v\n", err)
				}
				os.Exit(1)
			}
			output.Instance = instanceVer
		}

		// Check for updates unless disabled
		if !versionNoRemote {
			updateInfo := checkForUpdates(ctx, Version, versionPre, time.Duration(versionTimeout)*time.Second)
			output.Update = updateInfo
		}

		// Output results
		if versionJSON {
			outputJSON(output)
		} else {
			outputHumanReadable(output)
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)

	versionCmd.Flags().StringVar(&versionInstance, "instance", "", "Include version information for specified instance")
	versionCmd.Flags().BoolVar(&versionJSON, "json", false, "Output in JSON format")
	versionCmd.Flags().BoolVar(&versionNoRemote, "no-remote", false, "Skip GitHub update check")
	versionCmd.Flags().BoolVar(&versionPre, "pre", false, "Include pre-releases in update check")
	versionCmd.Flags().IntVar(&versionTimeout, "timeout", 3, "Network timeout in seconds for update check")
}

// getInstanceVersion reads version information from instance metadata
func getInstanceVersion(_ context.Context, instanceName string) (*InstanceVersionInfo, error) {
	// Create filesystem and buildmeta store
	fs := filesystem.New()
	store, err := buildmeta.New(fs)
	if err != nil {
		return nil, fmt.Errorf("failed to create metadata store: %w", err)
	}

	// Get instance metadata
	instanceMeta, err := store.GetInstance(instanceName)
	if err != nil {
		return nil, fmt.Errorf("failed to load instance metadata: %w", err)
	}

	// For now, we'll use the instance's metadata version info
	// In a real implementation, this would read more detailed version info
	return &InstanceVersionInfo{
		Name:      instanceName,
		Version:   instanceMeta.Version,
		Commit:    "unknown", // Would be in metadata.json if stored
		BuildDate: "unknown", // Would be in metadata.json if stored
	}, nil
}

// checkForUpdates checks GitHub for newer releases
func checkForUpdates(ctx context.Context, currentVersion string, includePre bool, timeout time.Duration) *UpdateInfo {
	info := &UpdateInfo{
		Checked: time.Now(),
		Channel: "stable",
	}

	if includePre {
		info.Channel = "all"
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: timeout,
	}

	// Fetch releases from GitHub API
	url := "https://api.github.com/repos/hydraide/hydraide/releases"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		info.Error = fmt.Sprintf("failed to create request: %v", err)
		return info
	}

	resp, err := client.Do(req)
	if err != nil {
		info.Error = fmt.Sprintf("failed to check for updates: %v", err)
		return info
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden {
			info.Error = "GitHub API rate limit exceeded. Try again later"
		} else {
			info.Error = fmt.Sprintf("GitHub API returned status %d", resp.StatusCode)
		}
		return info
	}

	var releases []GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		info.Error = fmt.Sprintf("failed to parse GitHub response: %v", err)
		return info
	}

	// Find the latest release
	for _, release := range releases {
		if release.Draft {
			continue
		}
		if release.Prerelease && !includePre {
			continue
		}

		// Check if this is a hydraidectl release
		if !strings.Contains(release.TagName, "hydraidectl") {
			continue
		}

		// Extract version from tag (e.g., "hydraidectl/v1.2.3" -> "v1.2.3")
		parts := strings.Split(release.TagName, "/")
		if len(parts) < 2 {
			continue
		}
		latestVersion := parts[1]

		// Simple version comparison (in production, use semver library)
		if isNewerVersion(latestVersion, currentVersion) {
			info.Latest = &latestVersion
			info.IsAvailable = true
			info.URL = &release.HTMLURL
			break
		}
	}

	return info
}

// isNewerVersion performs semantic version comparison
func isNewerVersion(latest, current string) bool {
	// Handle dev version - always consider updates available
	if current == "dev" {
		return true
	}

	// Remove 'v' prefix if present for parsing
	latest = strings.TrimPrefix(latest, "v")
	current = strings.TrimPrefix(current, "v")

	// Parse versions using semver
	latestVer, err := parseSemver(latest)
	if err != nil {
		// If we can't parse the latest version, fall back to string comparison
		return latest > current
	}

	currentVer, err := parseSemver(current)
	if err != nil {
		// If we can't parse current version, assume update is available
		return true
	}

	// Use semver comparison
	return latestVer.GreaterThan(currentVer)
}

// parseSemver attempts to parse a version string as semver
func parseSemver(version string) (*semver.Version, error) {
	// Try to parse as-is first
	v, err := semver.NewVersion(version)
	if err == nil {
		return v, nil
	}

	// If parsing fails and version doesn't contain dots, assume it's a major version only
	if !strings.Contains(version, ".") {
		version = version + ".0.0"
		return semver.NewVersion(version)
	}

	return nil, err
}

// outputJSON outputs version information in JSON format
func outputJSON(output VersionOutput) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(output)
}

// outputHumanReadable outputs version information in human-readable format
func outputHumanReadable(output VersionOutput) {
	// CLI version
	fmt.Printf("hydraidectl %s (commit %s, %s)\n",
		output.CLI.Version,
		shortCommit(output.CLI.Commit),
		output.CLI.BuildDate)

	// Instance version if available
	if output.Instance != nil {
		fmt.Printf("Instance %s: %s (commit %s, %s)\n",
			output.Instance.Name,
			output.Instance.Version,
			shortCommit(output.Instance.Commit),
			output.Instance.BuildDate)
	}

	// Update information
	if output.Update != nil {
		if output.Update.Error != "" {
			fmt.Fprintf(os.Stderr, "Update check failed: %s\n", output.Update.Error)
		} else if output.Update.IsAvailable && output.Update.Latest != nil {
			fmt.Printf("Update: %s available → run: hydraidectl self-update\n", *output.Update.Latest)
		} else if !versionNoRemote {
			fmt.Println("Up to date.")
		}
	}
}

// shortCommit returns the first 7 characters of a commit hash
func shortCommit(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}
