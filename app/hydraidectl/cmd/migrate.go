package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler/v2/migrator"
	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancerunner"
	"github.com/spf13/cobra"
)

const migrationLockFile = ".migration-lock"

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate HydrAIDE data from V1 to V2 format",
	Long: `
💠 Migrate HydrAIDE Data

Migrates HydrAIDE swamp data from V1 (multi-file chunks) to V2 (single-file append-only) format.

USAGE WITH INSTANCE (recommended):
  hydraidectl migrate --instance prod --full

USAGE WITH DATA PATH (manual):
  hydraidectl migrate --data-path=/var/hydraide/data --verify

FLAGS:
  --instance   Use instance name (automatically handles stop/start/engine)
  --full       Complete migration: stop → migrate → set engine V2 → cleanup → start
  --dry-run    Validate only, don't write any data
`,
	Run: runMigrate,
}

var (
	migrateDataPath     string
	migrateInstanceName string
	migrateFull         bool
	migrateDryRun       bool
	migrateVerify       bool
	migrateDeleteOld    bool
	migrateParallel     int
	migrateJSONOutput   bool
)

func init() {
	rootCmd.AddCommand(migrateCmd)

	migrateCmd.Flags().StringVarP(&migrateInstanceName, "instance", "i", "", "Instance name (use with --full)")
	migrateCmd.Flags().StringVar(&migrateDataPath, "data-path", "", "Path to HydrAIDE data directory")
	migrateCmd.Flags().BoolVar(&migrateFull, "full", false, "Complete migration (stop → migrate → set V2 → cleanup → start)")
	migrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "Validate only, don't write any data")
	migrateCmd.Flags().BoolVar(&migrateVerify, "verify", false, "Verify migration after writing")
	migrateCmd.Flags().BoolVar(&migrateDeleteOld, "delete-old", false, "Delete old V1 files after successful migration")
	migrateCmd.Flags().IntVar(&migrateParallel, "parallel", 4, "Number of parallel workers")
	migrateCmd.Flags().BoolVar(&migrateJSONOutput, "json", false, "Output result as JSON")
}

func runMigrate(cmd *cobra.Command, args []string) {
	// Handle --instance mode
	if migrateInstanceName != "" {
		runMigrateWithInstance()
		return
	}

	// Handle --data-path mode (legacy)
	if migrateDataPath == "" {
		fmt.Println("❌ Error: --instance or --data-path is required")
		os.Exit(1)
	}

	runMigrateWithDataPath()
}

func runMigrateWithInstance() {
	fs := filesystem.New()
	store, err := buildmeta.New(fs)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}

	instance, err := store.GetInstance(migrateInstanceName)
	if err != nil {
		fmt.Printf("❌ Error: Instance '%s' not found: %v\n", migrateInstanceName, err)
		os.Exit(1)
	}

	migrateDataPath = filepath.Join(instance.BasePath, "data")
	settingsPath := filepath.Join(instance.BasePath, "settings", "settings.json")
	lockPath := filepath.Join(instance.BasePath, migrationLockFile)

	// Check migration lock
	if _, err := os.Stat(lockPath); err == nil {
		fmt.Println("❌ Error: Migration already in progress (lock file exists)")
		fmt.Printf("   Lock file: %s\n", lockPath)
		fmt.Println("   If no migration is running, delete the lock file manually.")
		os.Exit(1)
	}

	// Create lock file
	if !migrateDryRun {
		if err := os.WriteFile(lockPath, []byte(time.Now().Format(time.RFC3339)), 0644); err != nil {
			fmt.Printf("❌ Error creating lock file: %v\n", err)
			os.Exit(1)
		}
		defer os.Remove(lockPath)
	}

	ctx := context.Background()
	runner := instancerunner.NewInstanceController()

	// Full migration: stop instance first
	if migrateFull && !migrateDryRun {
		fmt.Printf("Stopping instance '%s'...\n", migrateInstanceName)
		_ = runner.StopInstance(ctx, migrateInstanceName)

		// Enable full mode settings
		migrateVerify = true
		migrateDeleteOld = true
	}

	// Run migration
	runMigrateWithDataPath()

	// Full migration: set engine to V2 (but do NOT auto-start)
	if migrateFull && !migrateDryRun {
		fmt.Println("\nSetting engine to V2...")
		settings, _ := loadEngineSettings(settingsPath)
		if settings == nil {
			settings = &SettingsModelEngine{}
		}
		settings.Engine = EngineVersionV2
		if err := saveEngineSettings(settingsPath, settings); err != nil {
			fmt.Printf("⚠️  Warning: Could not set engine to V2: %v\n", err)
		} else {
			fmt.Println("✅ Engine set to V2")
		}

		fmt.Println("")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("🎉 Migration completed successfully!")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("")
		fmt.Println("The instance was NOT started automatically.")
		fmt.Println("Please verify the migration results above, then start the server manually:")
		fmt.Printf("  sudo hydraidectl start --instance %s\n", migrateInstanceName)
		fmt.Println("")
	}
}

func runMigrateWithDataPath() {
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
