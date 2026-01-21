package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/spf13/cobra"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Remove old V1 or V2 files after migration",
	Long: `Remove old storage files after migration.

EXAMPLES:
  # Remove V1 files (after V2 migration)
  hydraidectl cleanup --instance prod --v1-files

  # Remove V2 files (after rollback to V1)
  hydraidectl cleanup --instance prod --v2-files

  # Dry run (show what would be deleted)
  hydraidectl cleanup --instance prod --v1-files --dry-run
`,
	Run: runCleanupCmd,
}

var (
	cleanupInstanceName string
	cleanupV1Files      bool
	cleanupV2Files      bool
	cleanupDryRun       bool
)

func init() {
	rootCmd.AddCommand(cleanupCmd)
	cleanupCmd.Flags().StringVarP(&cleanupInstanceName, "instance", "i", "", "Instance name (required)")
	cleanupCmd.Flags().BoolVar(&cleanupV1Files, "v1-files", false, "Remove V1 chunk files/folders")
	cleanupCmd.Flags().BoolVar(&cleanupV2Files, "v2-files", false, "Remove V2 .hyd files")
	cleanupCmd.Flags().BoolVar(&cleanupDryRun, "dry-run", false, "Show what would be deleted without deleting")
	_ = cleanupCmd.MarkFlagRequired("instance")
}

func runCleanupCmd(cmd *cobra.Command, args []string) {
	if !cleanupV1Files && !cleanupV2Files {
		fmt.Println("Error: Specify --v1-files or --v2-files")
		os.Exit(1)
	}

	if cleanupV1Files && cleanupV2Files {
		fmt.Println("Error: Cannot specify both --v1-files and --v2-files")
		os.Exit(1)
	}

	fs := filesystem.New()
	store, err := buildmeta.New(fs)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	instance, err := store.GetInstance(cleanupInstanceName)
	if err != nil {
		fmt.Printf("Error: Instance not found: %v\n", err)
		os.Exit(1)
	}

	dataPath := filepath.Join(instance.BasePath, "data")

	if cleanupDryRun {
		fmt.Println("DRY RUN - No files will be deleted\n")
	}

	var totalSize int64
	var fileCount int
	var toDelete []string

	err = filepath.Walk(dataPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			// For V1 cleanup, check if directory contains chunk files
			if cleanupV1Files {
				entries, _ := os.ReadDir(path)
				hasChunks := false
				for _, e := range entries {
					if strings.HasSuffix(e.Name(), ".snappy") || e.Name() == "meta.json" {
						hasChunks = true
						break
					}
				}
				if hasChunks && path != dataPath {
					toDelete = append(toDelete, path)
					// Get folder size
					_ = filepath.Walk(path, func(p string, i os.FileInfo, e error) error {
						if e == nil && !i.IsDir() {
							totalSize += i.Size()
							fileCount++
						}
						return nil
					})
					return filepath.SkipDir
				}
			}
			return nil
		}

		// For V2 cleanup
		if cleanupV2Files && strings.HasSuffix(path, ".hyd") {
			toDelete = append(toDelete, path)
			totalSize += info.Size()
			fileCount++
		}

		return nil
	})

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if len(toDelete) == 0 {
		fmt.Println("No files to clean up.")
		return
	}

	fmt.Printf("Files/folders to delete: %d\n", len(toDelete))
	fmt.Printf("Total size: %.2f MB\n", float64(totalSize)/(1024*1024))
	fmt.Println()

	if cleanupDryRun {
		fmt.Println("Would delete:")
		for _, p := range toDelete {
			relPath, _ := filepath.Rel(dataPath, p)
			fmt.Printf("  %s\n", relPath)
		}
		return
	}

	fmt.Print("Continue with deletion? [y/N]: ")
	var response string
	fmt.Scanln(&response)
	if response != "y" && response != "Y" {
		fmt.Println("Aborted.")
		return
	}

	var deletedCount int
	for _, p := range toDelete {
		if err := os.RemoveAll(p); err != nil {
			fmt.Printf("Error deleting %s: %v\n", p, err)
		} else {
			deletedCount++
		}
	}

	fmt.Printf("\n✅ Deleted %d items\n", deletedCount)
}
