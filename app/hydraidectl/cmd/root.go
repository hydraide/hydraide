package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/updatecache"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "hydraidectl",
	Short:   "HydrAIDE Control CLI",
	Version: Version,
	Long: `
💠 HydrAIDE Control CLI (` + Version + `)

Welcome to hydraidectl – your tool to install, manage, and operate your HydrAIDE system.

📚 Full documentation: https://github.com/hydraide/hydraide/tree/main/docs/hydraidectl

INSTANCE LIFECYCLE:
  init        Create and configure a new HydrAIDE instance
  service     Register as a persistent system service (systemd)
  start       Start an instance
  stop        Gracefully stop an instance
  restart     Restart an instance
  destroy     Remove an instance (optionally purge data)

MONITORING & STATUS:
  list        Show all registered instances with status
  health      Check health status of an instance
  observe     Real-time monitoring dashboard (TUI) for debugging
  telemetry   Enable/disable telemetry collection for observe
  version     Display CLI and instance version information

UPDATES & MIGRATION:
  update      Update an instance to the latest HydrAIDE version
  migrate     Migrate data from V1 to V2 storage format
  engine      View or change storage engine version (V1/V2)

DATA MANAGEMENT:
  backup      Create a backup of instance data
  restore     Restore instance data from backup
  cleanup     Remove orphaned/old storage files
  size        Show instance data storage size
  stats       Show detailed swamp statistics and health report

CERTIFICATES:
  cert        Generate TLS certificates (standalone)

EXAMPLES:
  hydraidectl init
  hydraidectl list
  sudo hydraidectl start --instance prod
  sudo hydraidectl update --instance prod --no-start
  hydraidectl migrate --instance prod --full
  hydraidectl observe --instance prod
  hydraidectl telemetry --instance prod --enable
`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Skip update check for version command itself
		if cmd.Name() == "version" {
			return
		}
		// Check for hydraidectl updates before running any command
		checkAndNotifyUpdate()
	},
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println("❌ Error:", err)
		os.Exit(1)
	}
}

func init() {
	// Disable Cobra's automatic "completion" command
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	// Set the version - needs to be done in init() because Version is set via ldflags
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate("hydraidectl {{.Version}}\n")
}

// checkAndNotifyUpdate checks for hydraidectl updates using cache and prints a notification if available
func checkAndNotifyUpdate() {
	// First, try to use cached update info
	cached, err := updatecache.Load()
	if err == nil && cached.IsValid(Version) {
		// Use cached info
		if cached.IsAvailable && cached.Latest != nil {
			printUpdateNotification(*cached.Latest)
		}
		return
	}

	// Cache miss or invalid - check for updates with quick timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	updateInfo := checkForUpdates(ctx, Version, false, 2*time.Second)
	if updateInfo == nil {
		return
	}

	// Save to cache (ignore errors - cache is best-effort)
	cacheInfo := &updatecache.CachedUpdateInfo{
		Latest:      updateInfo.Latest,
		IsAvailable: updateInfo.IsAvailable,
		URL:         updateInfo.URL,
		CheckedAt:   time.Now(),
		ForVersion:  Version,
	}
	_ = updatecache.Save(cacheInfo)

	// Show notification if update available
	if updateInfo.IsAvailable && updateInfo.Latest != nil {
		printUpdateNotification(*updateInfo.Latest)
	}
}

// printUpdateNotification prints the update notification banner
func printUpdateNotification(latestVersion string) {
	fmt.Println("")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  🆕 New hydraidectl version available: %s → %s\n", Version, latestVersion)
	fmt.Println("  Update with:")
	fmt.Printf("    %s\n", hydraidectlInstallCommand)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("")
}
