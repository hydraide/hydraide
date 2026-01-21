package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler/v2/migrator"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate HydrAIDE data from V1 to V2 format",
	Long: `
💠 Migrate HydrAIDE Data

Migrates HydrAIDE swamp data from V1 (multi-file chunks) to V2 (single-file append-only) format.

IMPORTANT: 
  - Stop the HydrAIDE service before running migration
  - Create a backup before migration (ZFS snapshot recommended)
  - Run with --dry-run first to validate

USAGE:
  1. Validation (dry-run):
     hydraidectl migrate --data-path=/var/hydraide/data --dry-run

  2. Live migration with verification:
     hydraidectl migrate --data-path=/var/hydraide/data --verify

  3. Full migration with old file cleanup:
     hydraidectl migrate --data-path=/var/hydraide/data --verify --delete-old

EXAMPLES:
  # Validate all swamps (no changes made)
  hydraidectl migrate --data-path=/var/hydraide/data --dry-run

  # Migrate with verification (keeps old files)
  hydraidectl migrate --data-path=/var/hydraide/data --verify

  # Full migration with cleanup
  hydraidectl migrate --data-path=/var/hydraide/data --verify --delete-old --parallel=8
`,
	Run: runMigrate,
}

var (
	migrateDataPath   string
	migrateDryRun     bool
	migrateVerify     bool
	migrateDeleteOld  bool
	migrateParallel   int
	migrateJSONOutput bool
)

func init() {
	rootCmd.AddCommand(migrateCmd)

	migrateCmd.Flags().StringVar(&migrateDataPath, "data-path", "", "Path to HydrAIDE data directory (required)")
	migrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "Validate only, don't write any data")
	migrateCmd.Flags().BoolVar(&migrateVerify, "verify", false, "Verify migration after writing")
	migrateCmd.Flags().BoolVar(&migrateDeleteOld, "delete-old", false, "Delete old V1 files after successful migration")
	migrateCmd.Flags().IntVar(&migrateParallel, "parallel", 4, "Number of parallel workers")
	migrateCmd.Flags().BoolVar(&migrateJSONOutput, "json", false, "Output result as JSON")

	_ = migrateCmd.MarkFlagRequired("data-path")
}

func runMigrate(cmd *cobra.Command, args []string) {
	// Validate data path
	if migrateDataPath == "" {
		fmt.Println("❌ Error: --data-path is required")
		os.Exit(1)
	}

	// Check if data path exists
	if _, err := os.Stat(migrateDataPath); os.IsNotExist(err) {
		fmt.Printf("❌ Error: Data path does not exist: %s\n", migrateDataPath)
		os.Exit(1)
	}

	// Show warning if not dry-run
	if !migrateDryRun {
		fmt.Println("")
		fmt.Println("⚠️  WARNING: This will modify your HydrAIDE data!")
		fmt.Println("")
		fmt.Println("Before proceeding, ensure:")
		fmt.Println("  1. HydrAIDE service is STOPPED")
		fmt.Println("  2. You have a BACKUP (ZFS snapshot recommended)")
		fmt.Println("")

		if migrateDeleteOld {
			fmt.Println("🔥 --delete-old is enabled: Old V1 files will be DELETED after migration!")
			fmt.Println("")
		}

		fmt.Print("Do you want to continue? [y/N]: ")
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Migration cancelled.")
			os.Exit(0)
		}
		fmt.Println("")
	}

	// Create migrator config
	config := migrator.Config{
		DataPath:       migrateDataPath,
		DryRun:         migrateDryRun,
		Verify:         migrateVerify,
		DeleteOld:      migrateDeleteOld,
		Parallel:       migrateParallel,
		ProgressReport: 5 * time.Second,
	}

	// Create and run migrator
	fmt.Println("🚀 Starting migration...")
	if migrateDryRun {
		fmt.Println("   Mode: DRY-RUN (validation only)")
	} else {
		fmt.Println("   Mode: LIVE MIGRATION")
	}
	fmt.Printf("   Data path: %s\n", migrateDataPath)
	fmt.Printf("   Parallel workers: %d\n", migrateParallel)
	fmt.Println("")

	m, err := migrator.New(config)
	if err != nil {
		fmt.Printf("❌ Error creating migrator: %v\n", err)
		os.Exit(1)
	}

	result, err := m.Run()
	if err != nil {
		fmt.Printf("❌ Error during migration: %v\n", err)
		os.Exit(1)
	}

	// Output result
	if migrateJSONOutput {
		jsonData, err := result.ToJSON()
		if err != nil {
			fmt.Printf("❌ Error encoding result: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(jsonData))
	} else {
		fmt.Println(result.Summary())
	}

	// Exit with error code if there were failures
	if len(result.FailedSwamps) > 0 {
		os.Exit(1)
	}
}
