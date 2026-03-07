package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	v2 "github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler/v2"
	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancerunner"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

var migrateV2ToV3Cmd = &cobra.Command{
	Use:   "v2-to-v3",
	Short: "Upgrade V2 files to V3 format (swamp name in header, faster scanning)",
	Long: `
💠 Migrate V2 → V3

Upgrades all V2-format .hyd swamp files to V3 format. V3 stores the swamp
name in the file header, enabling ~100x faster scanning for tools like
'explore' and 'stats'. The instance will be stopped during the upgrade.

Already-V3 files are skipped automatically.

USAGE:
  hydraidectl migrate v2-to-v3 --instance prod
  hydraidectl migrate v2-to-v3 --instance prod --parallel 8 --restart
  hydraidectl migrate v2-to-v3 --instance prod --dry-run

FLAGS:
  --instance    Instance name (required)
  --parallel    Number of parallel workers (default: 4)
  --restart     Restart instance after upgrade
  --dry-run     Only analyze, don't perform upgrade
  --json        Output as JSON format
`,
	Run: runMigrateV2ToV3,
}

var (
	migrateV2V3InstanceName string
	migrateV2V3Parallel     int
	migrateV2V3Restart      bool
	migrateV2V3DryRun       bool
	migrateV2V3JSONOutput   bool
)

func init() {
	migrateCmd.AddCommand(migrateV2ToV3Cmd)

	migrateV2ToV3Cmd.Flags().StringVarP(&migrateV2V3InstanceName, "instance", "i", "", "Instance name (required)")
	migrateV2ToV3Cmd.Flags().IntVarP(&migrateV2V3Parallel, "parallel", "p", 4, "Number of parallel workers")
	migrateV2ToV3Cmd.Flags().BoolVarP(&migrateV2V3Restart, "restart", "r", false, "Restart instance after upgrade")
	migrateV2ToV3Cmd.Flags().BoolVar(&migrateV2V3DryRun, "dry-run", false, "Only analyze, don't upgrade")
	migrateV2ToV3Cmd.Flags().BoolVarP(&migrateV2V3JSONOutput, "json", "j", false, "Output as JSON")
	_ = migrateV2ToV3Cmd.MarkFlagRequired("instance")
}

// MigrateV2V3Report holds the complete V2→V3 migration report
type MigrateV2V3Report struct {
	Instance      string                  `json:"instance"`
	StartedAt     string                  `json:"started_at"`
	CompletedAt   string                  `json:"completed_at"`
	Duration      string                  `json:"duration"`
	DryRun        bool                    `json:"dry_run"`
	TotalFiles    int                     `json:"total_files"`
	V2Files       int                     `json:"v2_files"`
	V3Files       int                     `json:"v3_files"`
	UpgradedFiles int                     `json:"upgraded_files"`
	FailedFiles   int                     `json:"failed_files"`
	TotalOldSize  int64                   `json:"total_old_size_bytes"`
	TotalNewSize  int64                   `json:"total_new_size_bytes"`
	SpaceSaved    int64                   `json:"space_saved_bytes"`
	FailedDetails []MigrateV2V3FailedInfo `json:"failed_details,omitempty"`
}

// MigrateV2V3FailedInfo contains info about a failed migration
type MigrateV2V3FailedInfo struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

func runMigrateV2ToV3(_ *cobra.Command, _ []string) {
	fs := filesystem.New()
	store, err := buildmeta.New(fs)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}

	instance, err := store.GetInstance(migrateV2V3InstanceName)
	if err != nil {
		fmt.Printf("❌ Error: Instance '%s' not found: %v\n", migrateV2V3InstanceName, err)
		os.Exit(1)
	}

	dataPath := filepath.Join(instance.BasePath, "data")
	if _, err := os.Stat(dataPath); os.IsNotExist(err) {
		fmt.Printf("❌ Error: Data path not found: %s\n", dataPath)
		os.Exit(1)
	}

	startTime := time.Now()
	report := &MigrateV2V3Report{
		Instance:  migrateV2V3InstanceName,
		StartedAt: startTime.Format(time.RFC3339),
		DryRun:    migrateV2V3DryRun,
	}

	ctx := context.Background()
	runner := instancerunner.NewInstanceController()

	// Stop instance if not dry-run
	wasRunning := false
	if !migrateV2V3DryRun {
		fmt.Printf("🛑 Stopping instance '%s'...\n", migrateV2V3InstanceName)
		if err := runner.StopInstance(ctx, migrateV2V3InstanceName); err != nil {
			fmt.Printf("   (Instance was not running)\n")
		} else {
			wasRunning = true
			fmt.Printf("   Instance stopped\n")
		}
		fmt.Println()
	}

	// Find all .hyd files
	var hydFiles []string
	err = filepath.Walk(dataPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, ".hyd") {
			hydFiles = append(hydFiles, path)
		}
		return nil
	})
	if err != nil {
		fmt.Printf("❌ Error scanning data path: %v\n", err)
		os.Exit(1)
	}

	report.TotalFiles = len(hydFiles)

	if len(hydFiles) == 0 {
		fmt.Println("⚠️  No .hyd files found.")
		os.Exit(0)
	}

	// Run migration
	runV2ToV3Migration(hydFiles, report)

	// Calculate final stats
	report.CompletedAt = time.Now().Format(time.RFC3339)
	report.Duration = time.Since(startTime).Round(time.Millisecond).String()
	report.SpaceSaved = report.TotalOldSize - report.TotalNewSize

	// Restart instance if requested
	if migrateV2V3Restart && !migrateV2V3DryRun && wasRunning {
		fmt.Println()
		fmt.Printf("🚀 Restarting instance '%s'...\n", migrateV2V3InstanceName)
		if err := runner.StartInstance(ctx, migrateV2V3InstanceName); err != nil {
			fmt.Printf("⚠️  Warning: Could not restart instance: %v\n", err)
		} else {
			fmt.Printf("   Instance started\n")
		}
	}

	// Output
	if migrateV2V3JSONOutput {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Printf("❌ Error encoding JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
	} else {
		outputV2ToV3Table(report)
	}
}

func runV2ToV3Migration(hydFiles []string, report *MigrateV2V3Report) {
	bar := progressbar.NewOptions(len(hydFiles),
		progressbar.OptionSetDescription("🔄 Migrating V2 → V3"),
		progressbar.OptionSetWidth(40),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetItsString("files"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "█",
			SaucerHead:    "█",
			SaucerPadding: "░",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionFullWidth(),
		progressbar.OptionClearOnFinish(),
	)

	var (
		resultsMu     sync.Mutex
		wg            sync.WaitGroup
		workCh        = make(chan string, len(hydFiles))
		failed        []MigrateV2V3FailedInfo
		v2Count       int64
		v3Count       int64
		upgradedCount int64
		failedCount   int64
		totalOldSize  int64
		totalNewSize  int64
	)

	for i := 0; i < migrateV2V3Parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filePath := range workCh {
				oldSize, newSize, wasV2, err := migrateFileV2ToV3(filePath)

				resultsMu.Lock()
				if err != nil {
					atomic.AddInt64(&failedCount, 1)
					failed = append(failed, MigrateV2V3FailedInfo{
						Path:  filePath,
						Error: err.Error(),
					})
				} else if wasV2 {
					atomic.AddInt64(&v2Count, 1)
					atomic.AddInt64(&upgradedCount, 1)
					atomic.AddInt64(&totalOldSize, oldSize)
					atomic.AddInt64(&totalNewSize, newSize)
				} else {
					atomic.AddInt64(&v3Count, 1)
				}
				resultsMu.Unlock()

				_ = bar.Add(1)
			}
		}()
	}

	for _, file := range hydFiles {
		workCh <- file
	}
	close(workCh)

	wg.Wait()
	_ = bar.Finish()

	report.V2Files = int(v2Count)
	report.V3Files = int(v3Count)
	report.UpgradedFiles = int(upgradedCount)
	report.FailedFiles = int(failedCount)
	report.TotalOldSize = totalOldSize
	report.TotalNewSize = totalNewSize
	report.FailedDetails = failed
}

// migrateFileV2ToV3 migrates a single V2 file to V3 format.
// Returns (oldSize, newSize, wasV2, error).
func migrateFileV2ToV3(filePath string) (int64, int64, bool, error) {
	reader, err := v2.NewFileReader(filePath)
	if err != nil {
		return 0, 0, false, err
	}

	header := reader.GetHeader()

	// Already V3 — skip
	if header.IsV3() {
		reader.Close()
		return 0, 0, false, nil
	}

	// Dry-run: just report that it's V2
	if migrateV2V3DryRun {
		reader.Close()
		info, _ := os.Stat(filePath)
		size := int64(0)
		if info != nil {
			size = info.Size()
		}
		return size, size, true, nil
	}

	// Get old file size
	oldInfo, err := os.Stat(filePath)
	if err != nil {
		reader.Close()
		return 0, 0, true, err
	}
	oldSize := oldInfo.Size()

	// Load all entries and swamp name
	index, swampName, err := reader.LoadIndex()
	if err != nil {
		reader.Close()
		return oldSize, 0, true, err
	}
	reader.Close()

	if swampName == "" {
		return oldSize, 0, true, fmt.Errorf("no swamp name found in V2 file")
	}

	// Write to temp file as V3
	tempPath := filePath + ".v3migrate"
	writer, err := v2.NewFileWriterWithName(tempPath, v2.DefaultMaxBlockSize, swampName)
	if err != nil {
		return oldSize, 0, true, err
	}

	for key, data := range index {
		entry := v2.Entry{
			Operation: v2.OpInsert,
			Key:       key,
			Data:      data,
		}
		if err := writer.WriteEntry(entry); err != nil {
			writer.Close()
			os.Remove(tempPath)
			return oldSize, 0, true, err
		}
	}

	if err := writer.Close(); err != nil {
		os.Remove(tempPath)
		return oldSize, 0, true, err
	}

	// Atomic rename
	if err := os.Rename(tempPath, filePath); err != nil {
		os.Remove(tempPath)
		return oldSize, 0, true, err
	}

	newInfo, err := os.Stat(filePath)
	if err != nil {
		return oldSize, 0, true, err
	}

	return oldSize, newInfo.Size(), true, nil
}

func outputV2ToV3Table(report *MigrateV2V3Report) {
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if report.DryRun {
		fmt.Printf("  💠 V2 → V3 Migration Analysis (DRY-RUN) - %s\n", report.Instance)
	} else {
		fmt.Printf("  💠 V2 → V3 Migration Report - %s\n", report.Instance)
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	fmt.Println("📊 SUMMARY")
	fmt.Println(strings.Repeat("─", 60))
	printV2V3Row("Total .hyd Files", fmt.Sprintf("%d", report.TotalFiles))
	printV2V3Row("Already V3", fmt.Sprintf("%d", report.V3Files))
	if report.DryRun {
		printV2V3Row("Need Migration (V2)", fmt.Sprintf("%d", report.V2Files))
	} else {
		printV2V3Row("Migrated (V2 → V3)", fmt.Sprintf("✅ %d", report.UpgradedFiles))
	}
	if report.FailedFiles > 0 {
		printV2V3Row("Failed", fmt.Sprintf("❌ %d", report.FailedFiles))
	}
	printV2V3Row("Duration", report.Duration)
	fmt.Println()

	if report.UpgradedFiles > 0 && !report.DryRun {
		fmt.Println("💾 SPACE ANALYSIS")
		fmt.Println(strings.Repeat("─", 60))
		printV2V3Row("Size Before", formatBytes(report.TotalOldSize))
		printV2V3Row("Size After", formatBytes(report.TotalNewSize))
		if report.SpaceSaved > 0 {
			printV2V3Row("Space Saved", fmt.Sprintf("✅ %s", formatBytes(report.SpaceSaved)))
		} else if report.SpaceSaved < 0 {
			printV2V3Row("Size Increase", formatBytes(-report.SpaceSaved))
		}
		fmt.Println()
	}

	// Failed files
	if len(report.FailedDetails) > 0 {
		fmt.Println("❌ FAILED FILES")
		fmt.Println(strings.Repeat("─", 70))
		for _, f := range report.FailedDetails {
			fmt.Printf("  • %s\n", truncateV2V3Path(f.Path, 50))
			fmt.Printf("    Error: %s\n", f.Error)
		}
		fmt.Println()
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  Completed: %s\n", report.CompletedAt)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	if report.DryRun && report.V2Files > 0 {
		fmt.Println("💡 RECOMMENDATION")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Printf("   %d file(s) need migrating from V2 to V3.\n", report.V2Files)
		fmt.Printf("   Run without --dry-run to perform the migration:\n")
		fmt.Printf("   hydraidectl migrate v2-to-v3 --instance %s --parallel %d\n", report.Instance, migrateV2V3Parallel)
		fmt.Println()
	}

	if !report.DryRun && !migrateV2V3Restart {
		fmt.Println("💡 The instance was stopped for the migration.")
		fmt.Printf("   To start it: hydraidectl start --instance %s\n", report.Instance)
		fmt.Println()
	}
}

func printV2V3Row(label, value string) {
	fmt.Printf("  %-28s │ %s\n", label, value)
}

func truncateV2V3Path(path string, maxLen int) string {
	if idx := strings.Index(path, "/data/"); idx >= 0 {
		path = path[idx+6:]
	}
	path = strings.TrimSuffix(path, ".hyd")
	if len(path) <= maxLen {
		return path
	}
	return "..." + path[len(path)-maxLen+3:]
}
