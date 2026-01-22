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

// compactCmd represents the compact command
var compactCmd = &cobra.Command{
	Use:   "compact",
	Short: "Compact swamp files to remove fragmentation",
	Long: `
💠 Swamp Compaction

Compacts all V2 swamp files in a HydrAIDE instance to remove dead entries
and reclaim disk space. The instance will be stopped during compaction.

USAGE:
  hydraidectl compact --instance prod
  hydraidectl compact --instance prod --parallel 4 --restart
  hydraidectl compact --instance prod --dry-run

FLAGS:
  --instance    Instance name (required)
  --parallel    Number of parallel workers (default: 4)
  --threshold   Fragmentation threshold percentage (default: 20)
  --restart     Restart instance after compaction
  --dry-run     Only analyze, don't perform compaction
  --json        Output as JSON format

The compaction process:
  1. Stops the instance (if running)
  2. Scans all V2 swamps for fragmentation
  3. Compacts swamps above the threshold
  4. Reports space savings
  5. Optionally restarts the instance
`,
	Run: runCompactCmd,
}

var (
	compactInstanceName string
	compactParallel     int
	compactThreshold    float64
	compactRestart      bool
	compactDryRun       bool
	compactJSONOutput   bool
)

func init() {
	rootCmd.AddCommand(compactCmd)

	compactCmd.Flags().StringVarP(&compactInstanceName, "instance", "i", "", "Instance name (required)")
	compactCmd.Flags().IntVarP(&compactParallel, "parallel", "p", 4, "Number of parallel workers")
	compactCmd.Flags().Float64VarP(&compactThreshold, "threshold", "t", 20.0, "Fragmentation threshold percentage")
	compactCmd.Flags().BoolVarP(&compactRestart, "restart", "r", false, "Restart instance after compaction")
	compactCmd.Flags().BoolVar(&compactDryRun, "dry-run", false, "Only analyze, don't compact")
	compactCmd.Flags().BoolVarP(&compactJSONOutput, "json", "j", false, "Output as JSON")
	_ = compactCmd.MarkFlagRequired("instance")
}

// CompactReport holds the complete compaction report
type CompactReport struct {
	Instance         string              `json:"instance"`
	StartedAt        string              `json:"started_at"`
	CompletedAt      string              `json:"completed_at"`
	Duration         string              `json:"duration"`
	DryRun           bool                `json:"dry_run"`
	TotalSwamps      int                 `json:"total_swamps"`
	ScannedSwamps    int                 `json:"scanned_swamps"`
	CompactedSwamps  int                 `json:"compacted_swamps"`
	SkippedSwamps    int                 `json:"skipped_swamps"`
	FailedSwamps     int                 `json:"failed_swamps"`
	TotalOldSize     int64               `json:"total_old_size_bytes"`
	TotalNewSize     int64               `json:"total_new_size_bytes"`
	SpaceSaved       int64               `json:"space_saved_bytes"`
	EntriesRemoved   int64               `json:"entries_removed"`
	AvgFragBefore    float64             `json:"avg_fragmentation_before"`
	AvgFragAfter     float64             `json:"avg_fragmentation_after"`
	CompactedDetails []CompactSwampInfo  `json:"compacted_swamps_details,omitempty"`
	FailedDetails    []CompactFailedInfo `json:"failed_swamps_details,omitempty"`
}

// CompactSwampInfo contains info about a compacted swamp
type CompactSwampInfo struct {
	Path           string  `json:"path"`
	OldSize        int64   `json:"old_size_bytes"`
	NewSize        int64   `json:"new_size_bytes"`
	SpaceSaved     int64   `json:"space_saved_bytes"`
	FragBefore     float64 `json:"fragmentation_before"`
	EntriesRemoved int     `json:"entries_removed"`
}

// CompactFailedInfo contains info about a failed compaction
type CompactFailedInfo struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

func runCompactCmd(_ *cobra.Command, _ []string) {
	fs := filesystem.New()
	store, err := buildmeta.New(fs)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}

	instance, err := store.GetInstance(compactInstanceName)
	if err != nil {
		fmt.Printf("❌ Error: Instance '%s' not found: %v\n", compactInstanceName, err)
		os.Exit(1)
	}

	dataPath := filepath.Join(instance.BasePath, "data")

	// Check if data path exists
	if _, err := os.Stat(dataPath); os.IsNotExist(err) {
		fmt.Printf("❌ Error: Data path not found: %s\n", dataPath)
		os.Exit(1)
	}

	startTime := time.Now()
	report := &CompactReport{
		Instance:  compactInstanceName,
		StartedAt: startTime.Format(time.RFC3339),
		DryRun:    compactDryRun,
	}

	ctx := context.Background()
	runner := instancerunner.NewInstanceController()

	// Stop instance if not dry-run
	wasRunning := false
	if !compactDryRun {
		fmt.Printf("🛑 Stopping instance '%s'...\n", compactInstanceName)
		if err := runner.StopInstance(ctx, compactInstanceName); err != nil {
			// Instance might not be running, that's OK
			fmt.Printf("   (Instance was not running)\n")
		} else {
			wasRunning = true
			fmt.Printf("   Instance stopped\n")
		}
		fmt.Println()
	}

	// Find all V2 swamp files
	var swampFiles []string
	err = filepath.Walk(dataPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".hyd") {
			swampFiles = append(swampFiles, path)
		}
		return nil
	})

	if err != nil {
		fmt.Printf("❌ Error scanning data path: %v\n", err)
		os.Exit(1)
	}

	report.TotalSwamps = len(swampFiles)

	if len(swampFiles) == 0 {
		fmt.Println("⚠️  No V2 swamps found.")
		os.Exit(0)
	}

	// Convert threshold from percentage to ratio
	thresholdRatio := compactThreshold / 100.0

	// Run compaction with worker pool
	runCompaction(swampFiles, thresholdRatio, report)

	// Calculate final stats
	report.CompletedAt = time.Now().Format(time.RFC3339)
	report.Duration = time.Since(startTime).Round(time.Millisecond).String()
	if report.TotalOldSize > 0 {
		report.SpaceSaved = report.TotalOldSize - report.TotalNewSize
	}

	// Restart instance if requested
	if compactRestart && !compactDryRun && wasRunning {
		fmt.Println()
		fmt.Printf("🚀 Restarting instance '%s'...\n", compactInstanceName)
		if err := runner.StartInstance(ctx, compactInstanceName); err != nil {
			fmt.Printf("⚠️  Warning: Could not restart instance: %v\n", err)
		} else {
			fmt.Printf("   Instance started\n")
		}
	}

	// Output
	if compactJSONOutput {
		outputCompactJSON(report)
	} else {
		outputCompactTable(report)
	}
}

func runCompaction(swampFiles []string, threshold float64, report *CompactReport) {
	// Create progress bar
	bar := progressbar.NewOptions(len(swampFiles),
		progressbar.OptionSetDescription("🔧 Compacting swamps"),
		progressbar.OptionSetWidth(40),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetItsString("swamps"),
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
		resultsMu    sync.Mutex
		wg           sync.WaitGroup
		workCh       = make(chan string, len(swampFiles))
		compacted    []CompactSwampInfo
		failed       []CompactFailedInfo
		totalOldSize int64
		totalNewSize int64
		totalRemoved int64
		totalFragSum float64
		scannedCount int64
		compactCount int64
		skippedCount int64
		failedCount  int64
	)

	// Start workers
	for i := 0; i < compactParallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filePath := range workCh {
				result := compactSwamp(filePath, threshold)

				resultsMu.Lock()
				atomic.AddInt64(&scannedCount, 1)

				if result.Error != nil {
					atomic.AddInt64(&failedCount, 1)
					failed = append(failed, CompactFailedInfo{
						Path:  filePath,
						Error: result.Error.Error(),
					})
				} else if result.Compacted {
					atomic.AddInt64(&compactCount, 1)
					atomic.AddInt64(&totalOldSize, result.OldFileSize)
					atomic.AddInt64(&totalNewSize, result.NewFileSize)
					atomic.AddInt64(&totalRemoved, int64(result.RemovedEntries))
					totalFragSum += result.Fragmentation * 100 // Convert to percentage

					compacted = append(compacted, CompactSwampInfo{
						Path:           filePath,
						OldSize:        result.OldFileSize,
						NewSize:        result.NewFileSize,
						SpaceSaved:     result.OldFileSize - result.NewFileSize,
						FragBefore:     result.Fragmentation * 100,
						EntriesRemoved: result.RemovedEntries,
					})
				} else {
					atomic.AddInt64(&skippedCount, 1)
				}
				resultsMu.Unlock()

				_ = bar.Add(1)
			}
		}()
	}

	// Send work
	for _, file := range swampFiles {
		workCh <- file
	}
	close(workCh)

	// Wait for completion
	wg.Wait()
	_ = bar.Finish()

	// Update report
	report.ScannedSwamps = int(scannedCount)
	report.CompactedSwamps = int(compactCount)
	report.SkippedSwamps = int(skippedCount)
	report.FailedSwamps = int(failedCount)
	report.TotalOldSize = totalOldSize
	report.TotalNewSize = totalNewSize
	report.EntriesRemoved = totalRemoved
	report.CompactedDetails = compacted
	report.FailedDetails = failed

	if compactCount > 0 {
		report.AvgFragBefore = totalFragSum / float64(compactCount)
		report.AvgFragAfter = 0 // After compaction, fragmentation is 0
	}
}

func compactSwamp(filePath string, threshold float64) *v2.CompactionResult {
	compactor := v2.NewCompactor(filePath, v2.DefaultMaxBlockSize, threshold)

	// If dry-run, just check if compaction is needed
	if compactDryRun {
		shouldCompact, fragmentation, err := compactor.ShouldCompact()
		return &v2.CompactionResult{
			Compacted:     shouldCompact,
			Fragmentation: fragmentation,
			Error:         err,
		}
	}

	// Perform actual compaction
	result, err := compactor.Compact()
	if err != nil {
		return &v2.CompactionResult{Error: err}
	}
	return result
}

func outputCompactJSON(report *CompactReport) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Printf("❌ Error encoding JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

func outputCompactTable(report *CompactReport) {
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if report.DryRun {
		fmt.Printf("  💠 Compaction Analysis (DRY-RUN) - %s\n", report.Instance)
	} else {
		fmt.Printf("  💠 Compaction Report - %s\n", report.Instance)
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// Summary section
	fmt.Println("📊 SUMMARY")
	fmt.Println(strings.Repeat("─", 60))
	printCompactRow("Total Swamps", fmt.Sprintf("%d", report.TotalSwamps))
	printCompactRow("Scanned", fmt.Sprintf("%d", report.ScannedSwamps))
	printCompactRow("Compacted", fmt.Sprintf("%d", report.CompactedSwamps))
	printCompactRow("Skipped (below threshold)", fmt.Sprintf("%d", report.SkippedSwamps))
	if report.FailedSwamps > 0 {
		printCompactRow("Failed", fmt.Sprintf("❌ %d", report.FailedSwamps))
	}
	printCompactRow("Duration", report.Duration)
	fmt.Println()

	// Space savings
	if report.CompactedSwamps > 0 || report.DryRun {
		fmt.Println("💾 SPACE ANALYSIS")
		fmt.Println(strings.Repeat("─", 60))
		if !report.DryRun {
			printCompactRow("Size Before", formatBytes(report.TotalOldSize))
			printCompactRow("Size After", formatBytes(report.TotalNewSize))
			printCompactRow("Space Saved", fmt.Sprintf("✅ %s", formatBytes(report.SpaceSaved)))
			if report.TotalOldSize > 0 {
				savingsPercent := float64(report.SpaceSaved) / float64(report.TotalOldSize) * 100
				printCompactRow("Savings", fmt.Sprintf("%.1f%%", savingsPercent))
			}
			printCompactRow("Entries Removed", fmt.Sprintf("%d", report.EntriesRemoved))
		} else {
			printCompactRow("Would Compact", fmt.Sprintf("%d swamps", report.CompactedSwamps))
		}
		if report.AvgFragBefore > 0 {
			printCompactRow("Avg Fragmentation Before", fmt.Sprintf("%.1f%%", report.AvgFragBefore))
		}
		fmt.Println()
	}

	// Top compacted swamps
	if len(report.CompactedDetails) > 0 && !report.DryRun {
		maxShow := 10
		if len(report.CompactedDetails) < maxShow {
			maxShow = len(report.CompactedDetails)
		}

		fmt.Printf("📦 TOP %d COMPACTED SWAMPS\n", maxShow)
		fmt.Println(strings.Repeat("─", 70))
		fmt.Printf("  %-3s  %-30s  %12s  %12s  %8s\n", "#", "Swamp", "Old Size", "New Size", "Saved")
		fmt.Println(strings.Repeat("─", 70))

		for i := 0; i < maxShow; i++ {
			s := report.CompactedDetails[i]
			name := truncateCompactPath(s.Path, 30)
			fmt.Printf("  %-3d  %-30s  %12s  %12s  %8s\n",
				i+1,
				name,
				formatBytes(s.OldSize),
				formatBytes(s.NewSize),
				formatBytes(s.SpaceSaved),
			)
		}
		fmt.Println()
	}

	// Failed swamps
	if len(report.FailedDetails) > 0 {
		fmt.Println("❌ FAILED SWAMPS")
		fmt.Println(strings.Repeat("─", 70))
		for _, f := range report.FailedDetails {
			fmt.Printf("  • %s\n", truncateCompactPath(f.Path, 50))
			fmt.Printf("    Error: %s\n", f.Error)
		}
		fmt.Println()
	}

	// Footer
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  Completed: %s\n", report.CompletedAt)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// Recommendation
	if report.DryRun && report.CompactedSwamps > 0 {
		fmt.Println("💡 RECOMMENDATION")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Printf("   %d swamp(s) can be compacted.\n", report.CompactedSwamps)
		fmt.Printf("   Run without --dry-run to perform compaction:\n")
		fmt.Printf("   hydraidectl compact --instance %s --parallel %d\n", report.Instance, compactParallel)
		fmt.Println()
	}

	if !report.DryRun && !compactRestart {
		fmt.Println("💡 The instance was stopped for compaction.")
		fmt.Printf("   To start it: hydraidectl start --instance %s\n", report.Instance)
		fmt.Println()
	}
}

func printCompactRow(label, value string) {
	fmt.Printf("  %-28s │ %s\n", label, value)
}

func truncateCompactPath(path string, maxLen int) string {
	// Get just the relative part after "data/"
	if idx := strings.Index(path, "/data/"); idx >= 0 {
		path = path[idx+6:]
	}
	path = strings.TrimSuffix(path, ".hyd")

	if len(path) <= maxLen {
		return path
	}
	return "..." + path[len(path)-maxLen+3:]
}
