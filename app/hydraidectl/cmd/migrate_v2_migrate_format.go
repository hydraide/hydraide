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

var migrateV2MigrateFormatCmd = &cobra.Command{
	Use:   "v2-migrate-format",
	Short: "Upgrade .hyd file headers to embed swamp name for faster scanning",
	Long: `
💠 V2 Migrate Format

Rewrites .hyd swamp file headers to embed the swamp name directly after the
64-byte file header. This enables ~100x faster metadata scanning for tools
like 'explore' and 'stats', because the swamp name can be read in ~100 bytes
without decompressing any data blocks.

Files that already have the embedded name are skipped automatically.
The instance will be stopped during the upgrade.

This is a V2 engine internal format optimization — no engine version change
is involved. The server reads both old and new format files transparently.

USAGE:
  hydraidectl migrate v2-migrate-format --instance prod
  hydraidectl migrate v2-migrate-format --instance prod --parallel 8 --restart
  hydraidectl migrate v2-migrate-format --instance prod --dry-run

FLAGS:
  --instance    Instance name (required)
  --parallel    Number of parallel workers (default: 4)
  --restart     Restart instance after upgrade
  --dry-run     Only analyze, don't perform upgrade
  --json        Output as JSON format
`,
	Run: runMigrateV2Format,
}

var (
	migrateV2FmtInstanceName string
	migrateV2FmtParallel     int
	migrateV2FmtRestart      bool
	migrateV2FmtDryRun       bool
	migrateV2FmtJSONOutput   bool
)

func init() {
	migrateCmd.AddCommand(migrateV2MigrateFormatCmd)

	migrateV2MigrateFormatCmd.Flags().StringVarP(&migrateV2FmtInstanceName, "instance", "i", "", "Instance name (required)")
	migrateV2MigrateFormatCmd.Flags().IntVarP(&migrateV2FmtParallel, "parallel", "p", 4, "Number of parallel workers")
	migrateV2MigrateFormatCmd.Flags().BoolVarP(&migrateV2FmtRestart, "restart", "r", false, "Restart instance after upgrade")
	migrateV2MigrateFormatCmd.Flags().BoolVar(&migrateV2FmtDryRun, "dry-run", false, "Only analyze, don't upgrade")
	migrateV2MigrateFormatCmd.Flags().BoolVarP(&migrateV2FmtJSONOutput, "json", "j", false, "Output as JSON")
	_ = migrateV2MigrateFormatCmd.MarkFlagRequired("instance")
}

// MigrateV2FmtReport holds the complete format migration report
type MigrateV2FmtReport struct {
	Instance        string                   `json:"instance"`
	StartedAt       string                   `json:"started_at"`
	CompletedAt     string                   `json:"completed_at"`
	Duration        string                   `json:"duration"`
	DryRun          bool                     `json:"dry_run"`
	TotalFiles      int                      `json:"total_files"`
	OldFormatFiles  int                      `json:"old_format_files"`
	NewFormatFiles  int                      `json:"new_format_files"`
	UpgradedFiles   int                      `json:"upgraded_files"`
	FailedFiles     int                      `json:"failed_files"`
	TotalOldSize    int64                    `json:"total_old_size_bytes"`
	TotalNewSize    int64                    `json:"total_new_size_bytes"`
	SpaceSaved      int64                    `json:"space_saved_bytes"`
	FailedDetails   []MigrateV2FmtFailedInfo `json:"failed_details,omitempty"`
}

// MigrateV2FmtFailedInfo contains info about a failed migration
type MigrateV2FmtFailedInfo struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

func runMigrateV2Format(_ *cobra.Command, _ []string) {
	fs := filesystem.New()
	store, err := buildmeta.New(fs)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}

	instance, err := store.GetInstance(migrateV2FmtInstanceName)
	if err != nil {
		fmt.Printf("❌ Error: Instance '%s' not found: %v\n", migrateV2FmtInstanceName, err)
		os.Exit(1)
	}

	dataPath := filepath.Join(instance.BasePath, "data")
	if _, err := os.Stat(dataPath); os.IsNotExist(err) {
		fmt.Printf("❌ Error: Data path not found: %s\n", dataPath)
		os.Exit(1)
	}

	startTime := time.Now()
	report := &MigrateV2FmtReport{
		Instance:  migrateV2FmtInstanceName,
		StartedAt: startTime.Format(time.RFC3339),
		DryRun:    migrateV2FmtDryRun,
	}

	ctx := context.Background()
	runner := instancerunner.NewInstanceController()

	// Stop instance if not dry-run
	wasRunning := false
	if !migrateV2FmtDryRun {
		fmt.Printf("🛑 Stopping instance '%s'...\n", migrateV2FmtInstanceName)
		if err := runner.StopInstance(ctx, migrateV2FmtInstanceName); err != nil {
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

	// Run format migration
	runV2FormatMigration(hydFiles, report)

	// Calculate final stats
	report.CompletedAt = time.Now().Format(time.RFC3339)
	report.Duration = time.Since(startTime).Round(time.Millisecond).String()
	report.SpaceSaved = report.TotalOldSize - report.TotalNewSize

	// Restart instance if requested
	if migrateV2FmtRestart && !migrateV2FmtDryRun && wasRunning {
		fmt.Println()
		fmt.Printf("🚀 Restarting instance '%s'...\n", migrateV2FmtInstanceName)
		if err := runner.StartInstance(ctx, migrateV2FmtInstanceName); err != nil {
			fmt.Printf("⚠️  Warning: Could not restart instance: %v\n", err)
		} else {
			fmt.Printf("   Instance started\n")
		}
	}

	// Output
	if migrateV2FmtJSONOutput {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Printf("❌ Error encoding JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
	} else {
		outputV2FmtTable(report)
	}
}

func runV2FormatMigration(hydFiles []string, report *MigrateV2FmtReport) {
	bar := progressbar.NewOptions(len(hydFiles),
		progressbar.OptionSetDescription("🔄 Upgrading file headers"),
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
		resultsMu      sync.Mutex
		wg             sync.WaitGroup
		workCh         = make(chan string, len(hydFiles))
		failed         []MigrateV2FmtFailedInfo
		oldFmtCount    int64
		newFmtCount    int64
		upgradedCount  int64
		failedCount    int64
		totalOldSize   int64
		totalNewSize   int64
	)

	for i := 0; i < migrateV2FmtParallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filePath := range workCh {
				oldSize, newSize, needsUpgrade, err := migrateFileV2Format(filePath)

				resultsMu.Lock()
				if err != nil {
					atomic.AddInt64(&failedCount, 1)
					failed = append(failed, MigrateV2FmtFailedInfo{
						Path:  filePath,
						Error: err.Error(),
					})
				} else if needsUpgrade {
					atomic.AddInt64(&oldFmtCount, 1)
					atomic.AddInt64(&upgradedCount, 1)
					atomic.AddInt64(&totalOldSize, oldSize)
					atomic.AddInt64(&totalNewSize, newSize)
				} else {
					atomic.AddInt64(&newFmtCount, 1)
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

	report.OldFormatFiles = int(oldFmtCount)
	report.NewFormatFiles = int(newFmtCount)
	report.UpgradedFiles = int(upgradedCount)
	report.FailedFiles = int(failedCount)
	report.TotalOldSize = totalOldSize
	report.TotalNewSize = totalNewSize
	report.FailedDetails = failed
}

// migrateFileV2Format upgrades a single .hyd file to embed swamp name in header.
// Returns (oldSize, newSize, needsUpgrade, error).
func migrateFileV2Format(filePath string) (int64, int64, bool, error) {
	reader, err := v2.NewFileReader(filePath)
	if err != nil {
		return 0, 0, false, err
	}

	header := reader.GetHeader()

	// Already has embedded name — skip
	if header.IsV3() {
		reader.Close()
		return 0, 0, false, nil
	}

	// Dry-run: just report that it needs upgrade
	if migrateV2FmtDryRun {
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
		return oldSize, 0, true, fmt.Errorf("no swamp name found in file")
	}

	// Write to temp file with embedded name in header
	tempPath := filePath + ".fmtmigrate"
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

func outputV2FmtTable(report *MigrateV2FmtReport) {
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if report.DryRun {
		fmt.Printf("  💠 V2 Format Migration Analysis (DRY-RUN) - %s\n", report.Instance)
	} else {
		fmt.Printf("  💠 V2 Format Migration Report - %s\n", report.Instance)
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	fmt.Println("📊 SUMMARY")
	fmt.Println(strings.Repeat("─", 60))
	printV2FmtRow("Total .hyd Files", fmt.Sprintf("%d", report.TotalFiles))
	printV2FmtRow("Already Upgraded", fmt.Sprintf("%d", report.NewFormatFiles))
	if report.DryRun {
		printV2FmtRow("Need Upgrade", fmt.Sprintf("%d", report.OldFormatFiles))
	} else {
		printV2FmtRow("Upgraded", fmt.Sprintf("✅ %d", report.UpgradedFiles))
	}
	if report.FailedFiles > 0 {
		printV2FmtRow("Failed", fmt.Sprintf("❌ %d", report.FailedFiles))
	}
	printV2FmtRow("Duration", report.Duration)
	fmt.Println()

	if report.UpgradedFiles > 0 && !report.DryRun {
		fmt.Println("💾 SPACE ANALYSIS")
		fmt.Println(strings.Repeat("─", 60))
		printV2FmtRow("Size Before", formatBytes(report.TotalOldSize))
		printV2FmtRow("Size After", formatBytes(report.TotalNewSize))
		if report.SpaceSaved > 0 {
			printV2FmtRow("Space Saved", fmt.Sprintf("✅ %s", formatBytes(report.SpaceSaved)))
		} else if report.SpaceSaved < 0 {
			printV2FmtRow("Size Increase", formatBytes(-report.SpaceSaved))
		}
		fmt.Println()
	}

	// Failed files
	if len(report.FailedDetails) > 0 {
		fmt.Println("❌ FAILED FILES")
		fmt.Println(strings.Repeat("─", 70))
		for _, f := range report.FailedDetails {
			fmt.Printf("  • %s\n", truncateV2FmtPath(f.Path, 50))
			fmt.Printf("    Error: %s\n", f.Error)
		}
		fmt.Println()
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  Completed: %s\n", report.CompletedAt)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	if report.DryRun && report.OldFormatFiles > 0 {
		fmt.Println("💡 RECOMMENDATION")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Printf("   %d file(s) need format upgrade.\n", report.OldFormatFiles)
		fmt.Printf("   Run without --dry-run to perform the upgrade:\n")
		fmt.Printf("   hydraidectl migrate v2-migrate-format --instance %s --parallel %d\n", report.Instance, migrateV2FmtParallel)
		fmt.Println()
	}

	if !report.DryRun && !migrateV2FmtRestart {
		fmt.Println("💡 The instance was stopped for the migration.")
		fmt.Printf("   To start it: hydraidectl start --instance %s\n", report.Instance)
		fmt.Println()
	}
}

func printV2FmtRow(label, value string) {
	fmt.Printf("  %-28s │ %s\n", label, value)
}

func truncateV2FmtPath(path string, maxLen int) string {
	if idx := strings.Index(path, "/data/"); idx >= 0 {
		path = path[idx+6:]
	}
	path = strings.TrimSuffix(path, ".hyd")
	if len(path) <= maxLen {
		return path
	}
	return "..." + path[len(path)-maxLen+3:]
}
