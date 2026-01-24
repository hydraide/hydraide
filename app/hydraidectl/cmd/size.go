package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/spf13/cobra"
)

var sizeCmd = &cobra.Command{
	Use:   "size",
	Short: "Show size of HydrAIDE instance data",
	Run:   runSizeCmd,
}

var (
	sizeInstanceName string
	sizeDetailed     bool
	sizeJSONFormat   bool
)

func init() {
	rootCmd.AddCommand(sizeCmd)
	sizeCmd.Flags().StringVarP(&sizeInstanceName, "instance", "i", "", "Instance name (required)")
	sizeCmd.Flags().BoolVar(&sizeDetailed, "detailed", false, "Show top swamps by size")
	sizeCmd.Flags().BoolVarP(&sizeJSONFormat, "json", "j", false, "Output as JSON")
	_ = sizeCmd.MarkFlagRequired("instance")
}

type sizeInfo struct {
	Instance   string      `json:"instance"`
	DataPath   string      `json:"data_path"`
	TotalSize  int64       `json:"total_size_bytes"`
	TotalFiles int         `json:"total_files"`
	V1Files    int         `json:"v1_files"`
	V1Size     int64       `json:"v1_size_bytes"`
	V2Files    int         `json:"v2_files"`
	V2Size     int64       `json:"v2_size_bytes"`
	TopSwamps  []swampSize `json:"top_swamps,omitempty"`
}

type swampSize struct {
	Name string `json:"name"`
	Size int64  `json:"size_bytes"`
}

func runSizeCmd(cmd *cobra.Command, args []string) {
	fs := filesystem.New()
	store, err := buildmeta.New(fs)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	instance, err := store.GetInstance(sizeInstanceName)
	if err != nil {
		fmt.Printf("Error: Instance not found: %v\n", err)
		os.Exit(1)
	}

	dataPath := filepath.Join(instance.BasePath, "data")
	if _, err := os.Stat(dataPath); os.IsNotExist(err) {
		fmt.Printf("Error: Data path not found: %s\n", dataPath)
		os.Exit(1)
	}

	info := &sizeInfo{
		Instance: sizeInstanceName,
		DataPath: dataPath,
	}

	swampSizes := make(map[string]int64)

	err = filepath.Walk(dataPath, func(path string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fileInfo.IsDir() {
			return nil
		}

		info.TotalFiles++
		info.TotalSize += fileInfo.Size()

		relPath, _ := filepath.Rel(dataPath, path)

		if strings.HasSuffix(path, ".hyd") {
			info.V2Files++
			info.V2Size += fileInfo.Size()
		} else {
			info.V1Files++
			info.V1Size += fileInfo.Size()
		}

		// Track swamp sizes (first 2 dirs)
		parts := strings.Split(relPath, string(os.PathSeparator))
		if len(parts) >= 2 {
			swampName := filepath.Join(parts[0], parts[1])
			swampSizes[swampName] += fileInfo.Size()
		}

		return nil
	})

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Get top swamps
	if sizeDetailed && len(swampSizes) > 0 {
		var sortedSwamps []swampSize
		for name, size := range swampSizes {
			sortedSwamps = append(sortedSwamps, swampSize{Name: name, Size: size})
		}
		sort.Slice(sortedSwamps, func(i, j int) bool {
			return sortedSwamps[i].Size > sortedSwamps[j].Size
		})
		if len(sortedSwamps) > 10 {
			sortedSwamps = sortedSwamps[:10]
		}
		info.TopSwamps = sortedSwamps
	}

	if sizeJSONFormat {
		data, _ := json.MarshalIndent(info, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Printf("HydrAIDE Instance: %s\n", sizeInstanceName)
	fmt.Println(strings.Repeat("=", 40))
	fmt.Printf("Data Path:   %s\n", dataPath)
	fmt.Printf("Total Size:  %.2f MB\n", float64(info.TotalSize)/(1024*1024))
	fmt.Printf("Total Files: %d\n", info.TotalFiles)
	fmt.Println()
	fmt.Printf("V1 Files:    %d (%.2f MB)\n", info.V1Files, float64(info.V1Size)/(1024*1024))
	fmt.Printf("V2 Files:    %d (%.2f MB)\n", info.V2Files, float64(info.V2Size)/(1024*1024))

	if info.V1Files > 0 && info.V2Files > 0 {
		fmt.Println("\n⚠️  Mixed V1/V2 data detected! Migration may be incomplete.")
	} else if info.V1Files > 0 {
		fmt.Println("\n💡 Tip: Migrate to V2 for better performance:")
		fmt.Printf("   hydraidectl migrate --instance %s --full\n", sizeInstanceName)
	}

	if sizeDetailed && len(info.TopSwamps) > 0 {
		fmt.Println("\nTop 10 Largest Swamps:")
		for i, s := range info.TopSwamps {
			fmt.Printf("  %2d. %-30s %.2f MB\n", i+1, s.Name, float64(s.Size)/(1024*1024))
		}
	}
}
