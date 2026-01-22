package cmd

import (
	"bufio"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	v2 "github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler/v2"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/spf13/cobra"
)

// inspectCmd represents the inspect command
var inspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "Inspect and debug a swamp file",
	Long: `
💠 Swamp Inspector / Debugger

Inspects a V2 swamp file and displays all entries for debugging purposes.
This tool is useful for understanding what's inside a swamp file and
diagnosing issues like unexpected fragmentation.

USAGE:
  hydraidectl inspect --instance prod --swamp path/to/swamp
  hydraidectl inspect --instance prod --swamp path/to/swamp --json --output debug.json

FLAGS:
  --instance   Instance name (required)
  --swamp      Relative path to swamp (without .hyd extension)
  --page       Page number for pagination (default: 1)
  --per-page   Entries per page (default: 20)
  --json       Output as JSON format
  --output     Output file path for JSON export

The output includes:
  • File header information (created, blocks, entries)
  • Swamp name from metadata
  • All entries with operation type, key, and data size
  • For INSERT/UPDATE: treasure metadata (timestamps, creator, etc.)
  • Fragmentation analysis
`,
	Run: runInspectCmd,
}

var (
	inspectInstanceName string
	inspectSwampPath    string
	inspectPage         int
	inspectPerPage      int
	inspectJSONOutput   bool
	inspectOutputFile   string
)

func init() {
	rootCmd.AddCommand(inspectCmd)

	inspectCmd.Flags().StringVarP(&inspectInstanceName, "instance", "i", "", "Instance name (required)")
	inspectCmd.Flags().StringVarP(&inspectSwampPath, "swamp", "s", "", "Relative path to swamp (without .hyd extension)")
	inspectCmd.Flags().IntVar(&inspectPage, "page", 1, "Page number for pagination")
	inspectCmd.Flags().IntVar(&inspectPerPage, "per-page", 20, "Entries per page")
	inspectCmd.Flags().BoolVarP(&inspectJSONOutput, "json", "j", false, "Output as JSON")
	inspectCmd.Flags().StringVarP(&inspectOutputFile, "output", "o", "", "Output file path for JSON export")
	_ = inspectCmd.MarkFlagRequired("instance")
	_ = inspectCmd.MarkFlagRequired("swamp")
}

// InspectReport holds the complete inspection data
type InspectReport struct {
	FilePath             string         `json:"file_path"`
	SwampName            string         `json:"swamp_name,omitempty"`
	FileSizeBytes        int64          `json:"file_size_bytes"`
	CreatedAt            string         `json:"created_at,omitempty"`
	ModifiedAt           string         `json:"modified_at,omitempty"`
	BlockCount           uint64         `json:"block_count"`
	TotalEntries         int            `json:"total_entries"`
	LiveEntries          int            `json:"live_entries"`
	DeadEntries          int            `json:"dead_entries"`
	FragmentationPercent float64        `json:"fragmentation_percent"`
	Entries              []InspectEntry `json:"entries"`
}

// InspectEntry represents a single entry in the inspection
type InspectEntry struct {
	Index         int                  `json:"index"`
	Operation     string               `json:"operation"`
	Key           string               `json:"key"`
	DataSizeBytes int                  `json:"data_size_bytes"`
	IsLive        bool                 `json:"is_live"`
	TreasureMeta  *InspectTreasureMeta `json:"treasure_meta,omitempty"`
}

// InspectTreasureMeta contains decoded treasure metadata
type InspectTreasureMeta struct {
	Key              string `json:"key"`
	CreatedAt        string `json:"created_at,omitempty"`
	CreatedBy        string `json:"created_by,omitempty"`
	ModifiedAt       string `json:"modified_at,omitempty"`
	ModifiedBy       string `json:"modified_by,omitempty"`
	DeletedAt        string `json:"deleted_at,omitempty"`
	DeletedBy        string `json:"deleted_by,omitempty"`
	ExpireAt         string `json:"expire_at,omitempty"`
	PayloadSizeBytes int    `json:"payload_size_bytes"`
	ContentType      string `json:"content_type,omitempty"`
}

func runInspectCmd(_ *cobra.Command, _ []string) {
	fs := filesystem.New()
	store, err := buildmeta.New(fs)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}

	instance, err := store.GetInstance(inspectInstanceName)
	if err != nil {
		fmt.Printf("❌ Error: Instance '%s' not found: %v\n", inspectInstanceName, err)
		os.Exit(1)
	}

	dataPath := filepath.Join(instance.BasePath, "data")

	// Build full path to swamp file
	swampPath := inspectSwampPath
	if !strings.HasSuffix(swampPath, ".hyd") {
		swampPath = swampPath + ".hyd"
	}
	fullPath := filepath.Join(dataPath, swampPath)

	// Check if file exists
	fileInfo, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		fmt.Printf("❌ Error: Swamp file not found: %s\n", fullPath)
		os.Exit(1)
	}
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}

	// Analyze the swamp
	report, err := analyzeSwampForInspect(fullPath, fileInfo)
	if err != nil {
		fmt.Printf("❌ Error analyzing swamp: %v\n", err)
		os.Exit(1)
	}

	// Output
	if inspectJSONOutput || inspectOutputFile != "" {
		outputInspectJSON(report)
	} else {
		outputInspectTable(report)
	}
}

func analyzeSwampForInspect(filePath string, fileInfo os.FileInfo) (*InspectReport, error) {
	report := &InspectReport{
		FilePath:      filePath,
		FileSizeBytes: fileInfo.Size(),
	}

	// Open reader
	reader, err := v2.NewFileReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		_ = reader.Close()
	}()

	// Get header info
	header := reader.GetHeader()
	if header != nil {
		report.BlockCount = header.BlockCount
		if header.CreatedAt > 0 {
			report.CreatedAt = time.Unix(0, header.CreatedAt).Format(time.RFC3339)
		}
		if header.ModifiedAt > 0 {
			report.ModifiedAt = time.Unix(0, header.ModifiedAt).Format(time.RFC3339)
		}
	}

	// Read all entries and track live/dead
	liveKeys := make(map[string]int) // key -> last entry index
	var entries []InspectEntry
	entryIndex := 0

	_, err = reader.ReadAllEntries(func(entry v2.Entry) bool {
		entryIndex++

		// Determine operation string
		opStr := operationToString(entry.Operation)

		// Check for metadata entry (swamp name)
		if entry.Operation == v2.OpMetadata && entry.Key == v2.MetadataEntryKey {
			report.SwampName = string(entry.Data)
			// Don't add metadata to entries list for cleaner output
			return true
		}

		inspectEntry := InspectEntry{
			Index:         entryIndex,
			Operation:     opStr,
			Key:           entry.Key,
			DataSizeBytes: len(entry.Data),
		}

		// Try to decode treasure metadata for INSERT/UPDATE
		if entry.Operation == v2.OpInsert || entry.Operation == v2.OpUpdate {
			if meta := decodeTreasureMeta(entry.Data); meta != nil {
				inspectEntry.TreasureMeta = meta
			}
		}

		entries = append(entries, inspectEntry)

		// Track live/dead status
		switch entry.Operation {
		case v2.OpDelete:
			delete(liveKeys, entry.Key)
		case v2.OpInsert, v2.OpUpdate:
			liveKeys[entry.Key] = len(entries) - 1
		}

		return true
	})

	if err != nil {
		return nil, fmt.Errorf("failed to read entries: %w", err)
	}

	// Mark live entries
	for _, idx := range liveKeys {
		if idx < len(entries) {
			entries[idx].IsLive = true
		}
	}

	// Calculate stats
	report.TotalEntries = len(entries)
	report.LiveEntries = len(liveKeys)
	report.DeadEntries = report.TotalEntries - report.LiveEntries
	if report.TotalEntries > 0 {
		report.FragmentationPercent = float64(report.DeadEntries) / float64(report.TotalEntries) * 100
	}

	report.Entries = entries

	return report, nil
}

func operationToString(op uint8) string {
	switch op {
	case v2.OpInsert:
		return "INSERT"
	case v2.OpUpdate:
		return "UPDATE"
	case v2.OpDelete:
		return "DELETE"
	case v2.OpMetadata:
		return "METADATA"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", op)
	}
}

func decodeTreasureMeta(data []byte) *InspectTreasureMeta {
	if len(data) == 0 {
		return nil
	}

	// Try to decode as treasure.Model using GOB
	var model treasure.Model
	decoder := gob.NewDecoder(strings.NewReader(string(data)))
	// Use bytes.NewReader for proper binary handling
	decoder = gob.NewDecoder(bytesReader(data))
	if err := decoder.Decode(&model); err != nil {
		// If decoding fails, return basic info
		return &InspectTreasureMeta{
			PayloadSizeBytes: len(data),
			ContentType:      "DECODE_ERROR",
		}
	}

	meta := &InspectTreasureMeta{
		Key:              model.Key,
		CreatedBy:        model.CreatedBy,
		ModifiedBy:       model.ModifiedBy,
		DeletedBy:        model.DeletedBy,
		PayloadSizeBytes: len(data),
	}

	// Format timestamps
	if model.CreatedAt > 0 {
		meta.CreatedAt = time.Unix(0, model.CreatedAt).Format(time.RFC3339)
	}
	if model.ModifiedAt > 0 {
		meta.ModifiedAt = time.Unix(0, model.ModifiedAt).Format(time.RFC3339)
	}
	if model.DeletedAt > 0 {
		meta.DeletedAt = time.Unix(0, model.DeletedAt).Format(time.RFC3339)
	}
	if model.ExpirationTime > 0 {
		meta.ExpireAt = time.Unix(0, model.ExpirationTime).Format(time.RFC3339)
	}

	// Determine content type
	if model.Content != nil {
		meta.ContentType = determineContentType(model.Content)
	}

	return meta
}

// bytesReader wraps []byte for gob decoder
type bytesReaderType struct {
	data []byte
	pos  int
}

func bytesReader(data []byte) *bytesReaderType {
	return &bytesReaderType{data: data}
}

func (r *bytesReaderType) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, fmt.Errorf("EOF")
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func determineContentType(content *treasure.Content) string {
	if content == nil || content.Void {
		return "VOID"
	}
	if content.Uint8 != nil {
		return "UINT8"
	}
	if content.Uint16 != nil {
		return "UINT16"
	}
	if content.Uint32 != nil {
		return "UINT32"
	}
	if content.Uint64 != nil {
		return "UINT64"
	}
	if content.Int8 != nil {
		return "INT8"
	}
	if content.Int16 != nil {
		return "INT16"
	}
	if content.Int32 != nil {
		return "INT32"
	}
	if content.Int64 != nil {
		return "INT64"
	}
	if content.Float32 != nil {
		return "FLOAT32"
	}
	if content.Float64 != nil {
		return "FLOAT64"
	}
	if content.Boolean != nil {
		return "BOOL"
	}
	if content.String != nil {
		return "STRING"
	}
	if len(content.ByteArray) > 0 {
		return "BYTES"
	}
	if content.Uint32Slice != nil {
		return "UINT32_SLICE"
	}
	return "UNKNOWN"
}

func outputInspectJSON(report *InspectReport) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Printf("❌ Error encoding JSON: %v\n", err)
		os.Exit(1)
	}

	if inspectOutputFile != "" {
		if err := os.WriteFile(inspectOutputFile, data, 0644); err != nil {
			fmt.Printf("❌ Error writing file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ Report saved to: %s\n", inspectOutputFile)
	} else {
		fmt.Println(string(data))
	}
}

func outputInspectTable(report *InspectReport) {
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  💠 Swamp Inspector - %s\n", report.FilePath)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// File info section
	fmt.Println("📁 FILE INFO")
	fmt.Println(strings.Repeat("─", 60))
	if report.SwampName != "" {
		printInspectRow("Swamp Name", report.SwampName)
	}
	printInspectRow("File Size", formatBytes(report.FileSizeBytes))
	if report.CreatedAt != "" {
		printInspectRow("Created", report.CreatedAt)
	}
	if report.ModifiedAt != "" {
		printInspectRow("Modified", report.ModifiedAt)
	}
	printInspectRow("Blocks", fmt.Sprintf("%d", report.BlockCount))
	printInspectRow("Total Entries", fmt.Sprintf("%d", report.TotalEntries))
	printInspectRow("Live Entries", fmt.Sprintf("%d", report.LiveEntries))
	printInspectRow("Dead Entries", fmt.Sprintf("%d", report.DeadEntries))

	fragIcon := "✅"
	if report.FragmentationPercent > 20 {
		fragIcon = "⚠️"
	}
	if report.FragmentationPercent > 50 {
		fragIcon = "🔴"
	}
	printInspectRow("Fragmentation", fmt.Sprintf("%s %.1f%%", fragIcon, report.FragmentationPercent))
	fmt.Println()

	// Entries section with pagination
	totalPages := (len(report.Entries) + inspectPerPage - 1) / inspectPerPage
	if totalPages == 0 {
		totalPages = 1
	}

	startIdx := (inspectPage - 1) * inspectPerPage
	endIdx := startIdx + inspectPerPage
	if endIdx > len(report.Entries) {
		endIdx = len(report.Entries)
	}

	if len(report.Entries) == 0 {
		fmt.Println("📦 No entries found")
	} else {
		fmt.Printf("📦 ENTRIES (Page %d/%d)\n", inspectPage, totalPages)
		fmt.Println(strings.Repeat("─", 80))
		fmt.Printf("  %-5s  %-8s  %-6s  %-35s  %10s\n", "#", "Op", "Live?", "Key", "Data Size")
		fmt.Println(strings.Repeat("─", 80))

		for i := startIdx; i < endIdx && i < len(report.Entries); i++ {
			entry := report.Entries[i]
			liveStr := "❌"
			if entry.IsLive {
				liveStr = "✅"
			}

			keyDisplay := entry.Key
			if len(keyDisplay) > 35 {
				keyDisplay = keyDisplay[:32] + "..."
			}

			fmt.Printf("  %-5d  %-8s  %-6s  %-35s  %10s\n",
				entry.Index,
				entry.Operation,
				liveStr,
				keyDisplay,
				formatBytes(int64(entry.DataSizeBytes)),
			)

			// Show treasure meta if available and in detailed mode
			if entry.TreasureMeta != nil && entry.TreasureMeta.CreatedAt != "" {
				fmt.Printf("         └─ Created: %s by %s\n",
					entry.TreasureMeta.CreatedAt,
					entry.TreasureMeta.CreatedBy,
				)
			}
		}
		fmt.Println()
	}

	// If there are more pages, show hint
	if totalPages > 1 && inspectPage < totalPages {
		fmt.Printf("💡 Use --page %d to see more entries\n", inspectPage+1)
		fmt.Println()
	}

	// Interactive mode for terminal
	if !inspectJSONOutput && inspectOutputFile == "" && len(report.Entries) > inspectPerPage {
		fmt.Print("Press Enter to continue or 'q' to quit: ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		if strings.TrimSpace(input) != "q" && inspectPage < totalPages {
			inspectPage++
			outputInspectTable(report)
		}
	}
}

func printInspectRow(label, value string) {
	fmt.Printf("  %-20s │ %s\n", label, value)
}
