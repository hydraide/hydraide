package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	v2 "github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler/v2"
	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

// statsCmd represents the stats command
var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show detailed swamp statistics and health report",
	Long: `
💠 Swamp Statistics & Health Report

Analyzes all V2 swamps in a HydrAIDE instance and provides detailed statistics
including fragmentation levels, compaction recommendations, and size information.

USAGE:
  hydraidectl stats --instance prod
  hydraidectl stats --instance prod --json
  hydraidectl stats --instance prod --latest

FLAGS:
  --instance   Instance name (required)
  --json       Output as JSON format
  --latest     Show the last saved report instead of running a new scan
  --parallel   Number of parallel workers (default: 4)

The report includes:
  • Total database size and swamp count
  • Average/median records per swamp
  • Top 10 largest swamps
  • Top 10 most fragmented swamps
  • Compaction recommendations
  • Oldest and newest swamps

Reports are automatically saved to <instance>/.hydraide/stats-report-latest.json
`,
	Run: runStatsCmd,
}

var (
	statsInstanceName string
	statsJSONOutput   bool
	statsLatest       bool
	statsParallel     int
)

func init() {
	rootCmd.AddCommand(statsCmd)

	statsCmd.Flags().StringVarP(&statsInstanceName, "instance", "i", "", "Instance name (required)")
	statsCmd.Flags().BoolVarP(&statsJSONOutput, "json", "j", false, "Output as JSON")
	statsCmd.Flags().BoolVarP(&statsLatest, "latest", "l", false, "Show last saved report")
	statsCmd.Flags().IntVarP(&statsParallel, "parallel", "p", 4, "Number of parallel workers")
	_ = statsCmd.MarkFlagRequired("instance")
}

// SwampStats holds statistics for a single swamp
type SwampStats struct {
	Path            string    `json:"path"`
	Name            string    `json:"name"`
	SwampName       string    `json:"swamp_name,omitempty"` // Actual swamp name from metadata
	SizeBytes       int64     `json:"size_bytes"`
	LiveEntries     int       `json:"live_entries"`
	TotalEntries    int       `json:"total_entries"`
	DeadEntries     int       `json:"dead_entries"`
	Fragmentation   float64   `json:"fragmentation_percent"`
	NeedsCompaction bool      `json:"needs_compaction"`
	CreatedAt       time.Time `json:"created_at"`
	ModifiedAt      time.Time `json:"modified_at"`
}

// StatsReport holds the complete statistics report
type StatsReport struct {
	Instance     string    `json:"instance"`
	GeneratedAt  time.Time `json:"generated_at"`
	ScanDuration string    `json:"scan_duration"`

	// Summary
	TotalDatabaseSize     int64   `json:"total_database_size_bytes"`
	TotalSwamps           int     `json:"total_swamps"`
	TotalLiveRecords      int64   `json:"total_live_records"`
	TotalEntries          int64   `json:"total_entries"`
	TotalDeadEntries      int64   `json:"total_dead_entries"`
	AvgRecordsPerSwamp    float64 `json:"avg_records_per_swamp"`
	MedianRecordsPerSwamp int     `json:"median_records_per_swamp"`
	AvgSwampSize          int64   `json:"avg_swamp_size_bytes"`

	// Fragmentation
	AvgFragmentation        float64 `json:"avg_fragmentation_percent"`
	SwampsNeedingCompaction int     `json:"swamps_needing_compaction"`
	ReclaimableSpaceBytes   int64   `json:"reclaimable_space_bytes"`

	// Dates
	OldestSwamp *SwampStats `json:"oldest_swamp,omitempty"`
	NewestSwamp *SwampStats `json:"newest_swamp,omitempty"`

	// Top lists
	LargestSwamps        []SwampStats `json:"largest_swamps"`
	MostFragmentedSwamps []SwampStats `json:"most_fragmented_swamps"`

	// All swamps (for detailed JSON output)
	AllSwamps []SwampStats `json:"all_swamps,omitempty"`
}

const (
	statsWorkingDir     = ".hydraide"
	statsReportFile     = "stats-report-latest.json"
	compactionThreshold = 0.20 // 20% fragmentation threshold
)

func runStatsCmd(_ *cobra.Command, _ []string) {
	fs := filesystem.New()
	store, err := buildmeta.New(fs)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}

	instance, err := store.GetInstance(statsInstanceName)
	if err != nil {
		fmt.Printf("❌ Error: Instance '%s' not found: %v\n", statsInstanceName, err)
		os.Exit(1)
	}

	dataPath := filepath.Join(instance.BasePath, "data")
	workingDir := filepath.Join(instance.BasePath, statsWorkingDir)
	reportPath := filepath.Join(workingDir, statsReportFile)

	// Handle --latest flag
	if statsLatest {
		showLatestReport(reportPath)
		return
	}

	// Check if data path exists
	if _, err := os.Stat(dataPath); os.IsNotExist(err) {
		fmt.Printf("❌ Error: Data path not found: %s\n", dataPath)
		os.Exit(1)
	}

	// Ensure working directory exists
	if err := os.MkdirAll(workingDir, 0755); err != nil {
		fmt.Printf("⚠️  Warning: Could not create working directory: %v\n", err)
	}

	// Run the scan
	report := runScan(statsInstanceName, dataPath, statsParallel)

	// Save report
	if err := saveReport(reportPath, report); err != nil {
		fmt.Printf("⚠️  Warning: Could not save report: %v\n", err)
	}

	// Output
	if statsJSONOutput {
		outputStatsJSON(report)
	} else {
		outputStatsTable(report)
	}
}

func showLatestReport(reportPath string) {
	data, err := os.ReadFile(reportPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("❌ No previous report found.")
			fmt.Println("   Run 'hydraidectl stats --instance <name>' to generate a report first.")
			os.Exit(1)
		}
		fmt.Printf("❌ Error reading report: %v\n", err)
		os.Exit(1)
	}

	var report StatsReport
	if err := json.Unmarshal(data, &report); err != nil {
		fmt.Printf("❌ Error parsing report: %v\n", err)
		os.Exit(1)
	}

	if statsJSONOutput {
		outputStatsJSON(&report)
	} else {
		fmt.Printf("📂 Showing cached report from: %s\n\n", report.GeneratedAt.Format(time.RFC3339))
		outputStatsTable(&report)
	}
}

func runScan(instanceName, dataPath string, parallel int) *StatsReport {
	startTime := time.Now()

	// Find all V2 swamp files
	var swampFiles []string
	err := filepath.Walk(dataPath, func(path string, info os.FileInfo, err error) error {
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

	if len(swampFiles) == 0 {
		fmt.Println("⚠️  No V2 swamps found. The instance may be using V1 format or is empty.")
		fmt.Println("   Use 'hydraidectl migrate --instance <name> --full' to migrate to V2.")
		os.Exit(0)
	}

	// Create progress bar
	bar := progressbar.NewOptions(len(swampFiles),
		progressbar.OptionSetDescription("🔍 Scanning swamps"),
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

	// Analyze swamps with worker pool
	var (
		results   []SwampStats
		resultsMu sync.Mutex
		wg        sync.WaitGroup
		workCh    = make(chan string, len(swampFiles))
		processed int64
	)

	// Start workers
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filePath := range workCh {
				stats := analyzeSwamp(filePath, dataPath)
				if stats != nil {
					resultsMu.Lock()
					results = append(results, *stats)
					resultsMu.Unlock()
				}
				atomic.AddInt64(&processed, 1)
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

	// Build report
	report := buildReport(instanceName, results, time.Since(startTime))
	return report
}

func analyzeSwamp(filePath, dataPath string) *SwampStats {
	// Get file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil
	}

	// Get relative path for name
	relPath, _ := filepath.Rel(dataPath, filePath)
	name := strings.TrimSuffix(relPath, ".hyd")

	stats := &SwampStats{
		Path:       filePath,
		Name:       name,
		SizeBytes:  fileInfo.Size(),
		ModifiedAt: fileInfo.ModTime(),
	}

	// Read V2 file for detailed stats
	reader, err := v2.NewFileReader(filePath)
	if err != nil {
		// If we can't read, return basic stats
		return stats
	}
	defer func() {
		_ = reader.Close()
	}()

	// Get header info
	header := reader.GetHeader()
	if header != nil {
		stats.CreatedAt = time.Unix(0, header.CreatedAt)
	}

	// Load index to get swamp name and calculate live entries
	index, swampName, err := reader.LoadIndex()
	if err != nil {
		return stats
	}

	// Set the actual swamp name from metadata
	if swampName != "" {
		stats.SwampName = swampName
	}

	// Calculate totals by reading all entries
	// We need to re-read to count total entries (LoadIndex only gives us live keys)
	// Reopen the reader for counting
	reader2, err := v2.NewFileReader(filePath)
	if err != nil {
		stats.LiveEntries = len(index)
		return stats
	}
	defer reader2.Close()

	totalCount := 0
	_, _ = reader2.ReadAllEntries(func(entry v2.Entry) bool {
		// Skip metadata entries
		if entry.Operation != v2.OpMetadata {
			totalCount++
		}
		return true
	})

	stats.LiveEntries = len(index)
	stats.TotalEntries = totalCount
	stats.DeadEntries = totalCount - len(index)
	if totalCount > 0 {
		stats.Fragmentation = float64(stats.DeadEntries) / float64(totalCount) * 100
	}
	stats.NeedsCompaction = stats.Fragmentation >= compactionThreshold*100

	return stats
}

func buildReport(instanceName string, swamps []SwampStats, duration time.Duration) *StatsReport {
	report := &StatsReport{
		Instance:     instanceName,
		GeneratedAt:  time.Now(),
		ScanDuration: duration.Round(time.Millisecond).String(),
		TotalSwamps:  len(swamps),
	}

	if len(swamps) == 0 {
		return report
	}

	// Calculate totals
	var (
		totalSize       int64
		totalLive       int64
		totalEntries    int64
		totalDead       int64
		totalFrag       float64
		compactionCount int
		reclaimable     int64
		recordCounts    []int
		oldest, newest  *SwampStats
	)

	for i := range swamps {
		s := &swamps[i]
		totalSize += s.SizeBytes
		totalLive += int64(s.LiveEntries)
		totalEntries += int64(s.TotalEntries)
		totalDead += int64(s.DeadEntries)
		totalFrag += s.Fragmentation
		recordCounts = append(recordCounts, s.LiveEntries)

		if s.NeedsCompaction {
			compactionCount++
			// Estimate reclaimable space based on fragmentation
			reclaimable += int64(float64(s.SizeBytes) * (s.Fragmentation / 100))
		}

		// Track oldest/newest
		if !s.CreatedAt.IsZero() {
			if oldest == nil || s.CreatedAt.Before(oldest.CreatedAt) {
				oldest = s
			}
			if newest == nil || s.CreatedAt.After(newest.CreatedAt) {
				newest = s
			}
		}
	}

	report.TotalDatabaseSize = totalSize
	report.TotalLiveRecords = totalLive
	report.TotalEntries = totalEntries
	report.TotalDeadEntries = totalDead
	report.AvgRecordsPerSwamp = float64(totalLive) / float64(len(swamps))
	report.AvgSwampSize = totalSize / int64(len(swamps))
	report.AvgFragmentation = totalFrag / float64(len(swamps))
	report.SwampsNeedingCompaction = compactionCount
	report.ReclaimableSpaceBytes = reclaimable
	report.OldestSwamp = oldest
	report.NewestSwamp = newest

	// Calculate median
	sort.Ints(recordCounts)
	if len(recordCounts) > 0 {
		mid := len(recordCounts) / 2
		if len(recordCounts)%2 == 0 {
			report.MedianRecordsPerSwamp = (recordCounts[mid-1] + recordCounts[mid]) / 2
		} else {
			report.MedianRecordsPerSwamp = recordCounts[mid]
		}
	}

	// Top 10 largest
	sortedBySize := make([]SwampStats, len(swamps))
	copy(sortedBySize, swamps)
	sort.Slice(sortedBySize, func(i, j int) bool {
		return sortedBySize[i].SizeBytes > sortedBySize[j].SizeBytes
	})
	if len(sortedBySize) > 10 {
		sortedBySize = sortedBySize[:10]
	}
	report.LargestSwamps = sortedBySize

	// Top 10 most fragmented
	sortedByFrag := make([]SwampStats, len(swamps))
	copy(sortedByFrag, swamps)
	sort.Slice(sortedByFrag, func(i, j int) bool {
		return sortedByFrag[i].Fragmentation > sortedByFrag[j].Fragmentation
	})
	if len(sortedByFrag) > 10 {
		sortedByFrag = sortedByFrag[:10]
	}
	report.MostFragmentedSwamps = sortedByFrag

	// Store all swamps for JSON output
	report.AllSwamps = swamps

	return report
}

func saveReport(path string, report *StatsReport) error {
	// Don't include all swamps in saved report to keep file size reasonable
	reportToSave := *report
	reportToSave.AllSwamps = nil

	data, err := json.MarshalIndent(reportToSave, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func outputStatsJSON(report *StatsReport) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Printf("❌ Error encoding JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

func outputStatsTable(report *StatsReport) {
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  💠 HydrAIDE Swamp Statistics - %s\n", report.Instance)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// Summary section
	fmt.Println("📊 SUMMARY")
	fmt.Println(strings.Repeat("─", 60))
	printRow("Total Database Size", formatBytes(report.TotalDatabaseSize))
	printRow("Total Swamps", fmt.Sprintf("%d", report.TotalSwamps))
	printRow("Total Live Records", formatNumber(report.TotalLiveRecords))
	printRow("Total Entries (incl. deleted)", formatNumber(report.TotalEntries))
	printRow("Dead Entries", formatNumber(report.TotalDeadEntries))
	printRow("Avg Records/Swamp", fmt.Sprintf("%.1f", report.AvgRecordsPerSwamp))
	printRow("Median Records/Swamp", fmt.Sprintf("%d", report.MedianRecordsPerSwamp))
	printRow("Avg Swamp Size", formatBytes(report.AvgSwampSize))
	printRow("Scan Duration", report.ScanDuration)
	fmt.Println()

	// Fragmentation section
	fmt.Println("🔧 FRAGMENTATION & COMPACTION")
	fmt.Println(strings.Repeat("─", 60))

	avgFragIcon := "✅"
	if report.AvgFragmentation > 20 {
		avgFragIcon = "⚠️"
	}
	if report.AvgFragmentation > 50 {
		avgFragIcon = "🔴"
	}

	printRow("Average Fragmentation", fmt.Sprintf("%s %.1f%%", avgFragIcon, report.AvgFragmentation))
	printRow("Swamps Needing Compaction", fmt.Sprintf("%d (>%.0f%% fragmented)", report.SwampsNeedingCompaction, compactionThreshold*100))
	printRow("Estimated Reclaimable Space", formatBytes(report.ReclaimableSpaceBytes))
	fmt.Println()

	// Oldest/Newest swamps
	if report.OldestSwamp != nil || report.NewestSwamp != nil {
		fmt.Println("📅 TIMELINE")
		fmt.Println(strings.Repeat("─", 80))

		if report.OldestSwamp != nil {
			swampName := report.OldestSwamp.SwampName
			if swampName == "" {
				swampName = "(no name)"
			}
			fmt.Printf("  Oldest Swamp:\n")
			fmt.Printf("    Path: %s\n", report.OldestSwamp.Name)
			fmt.Printf("    Name: %s\n", swampName)
			fmt.Printf("    Date: %s\n", report.OldestSwamp.CreatedAt.Format("2006-01-02 15:04:05"))
		}
		if report.NewestSwamp != nil {
			swampName := report.NewestSwamp.SwampName
			if swampName == "" {
				swampName = "(no name)"
			}
			fmt.Printf("  Newest Swamp:\n")
			fmt.Printf("    Path: %s\n", report.NewestSwamp.Name)
			fmt.Printf("    Name: %s\n", swampName)
			fmt.Printf("    Date: %s\n", report.NewestSwamp.CreatedAt.Format("2006-01-02 15:04:05"))
		}
		fmt.Println()
	}

	// Top 10 Largest
	if len(report.LargestSwamps) > 0 {
		fmt.Println("📦 TOP 10 LARGEST SWAMPS")
		fmt.Println(strings.Repeat("─", 80))

		for i, s := range report.LargestSwamps {
			swampName := s.SwampName
			if swampName == "" {
				swampName = "(no name)"
			}
			fmt.Printf("  %d. %s\n", i+1, s.Name)
			fmt.Printf("     Name: %s\n", swampName)
			fmt.Printf("     Size: %s | Records: %s\n", formatBytes(s.SizeBytes), formatNumber(int64(s.LiveEntries)))
			fmt.Println()
		}
	}

	// Top 10 Most Fragmented
	if len(report.MostFragmentedSwamps) > 0 {
		fmt.Println("⚡ TOP 10 MOST FRAGMENTED SWAMPS")
		fmt.Println(strings.Repeat("─", 80))

		for i, s := range report.MostFragmentedSwamps {
			compactIcon := "—"
			if s.NeedsCompaction {
				compactIcon = "⚠️"
			}
			swampName := s.SwampName
			if swampName == "" {
				swampName = "(no name)"
			}
			fmt.Printf("  %d. %s %s\n", i+1, s.Name, compactIcon)
			fmt.Printf("     Name: %s\n", swampName)
			fmt.Printf("     Frag: %.1f%% | Dead: %d | Live: %d\n", s.Fragmentation, s.DeadEntries, s.LiveEntries)
			fmt.Println()
		}
	}

	// Footer
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  Generated: %s\n", report.GeneratedAt.Format(time.RFC3339))
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// Recommendations
	if report.SwampsNeedingCompaction > 0 {
		fmt.Println("💡 RECOMMENDATIONS")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Printf("   %d swamp(s) have >%.0f%% fragmentation.\n", report.SwampsNeedingCompaction, compactionThreshold*100)
		fmt.Printf("   Estimated %s can be reclaimed with compaction.\n", formatBytes(report.ReclaimableSpaceBytes))
		fmt.Println()
	}
}

func printRow(label, value string) {
	fmt.Printf("  %-32s │ %s\n", label, value)
}

// Helper functions

func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func formatNumber(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1000000)
}

func truncateName(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	return name[:maxLen-3] + "..."
}
