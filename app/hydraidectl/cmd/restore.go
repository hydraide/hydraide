package cmd

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancerunner"
	"github.com/spf13/cobra"
)

var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore HydrAIDE instance data from a backup",
	Run:   runRestoreCmd,
}

var (
	restoreInstanceName string
	restoreSource       string
	restoreForce        bool
)

func init() {
	rootCmd.AddCommand(restoreCmd)
	restoreCmd.Flags().StringVarP(&restoreInstanceName, "instance", "i", "", "Instance name (required)")
	restoreCmd.Flags().StringVarP(&restoreSource, "source", "s", "", "Source backup path (required)")
	restoreCmd.Flags().BoolVar(&restoreForce, "force", false, "Skip confirmation prompt")
	_ = restoreCmd.MarkFlagRequired("instance")
	_ = restoreCmd.MarkFlagRequired("source")
}

func runRestoreCmd(cmd *cobra.Command, args []string) {
	fs := filesystem.New()
	store, err := buildmeta.New(fs)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	instance, err := store.GetInstance(restoreInstanceName)
	if err != nil {
		fmt.Printf("Error: Instance not found: %v\n", err)
		os.Exit(1)
	}

	sourceInfo, err := os.Stat(restoreSource)
	if os.IsNotExist(err) {
		fmt.Printf("Error: Source not found: %s\n", restoreSource)
		os.Exit(1)
	}

	if !restoreForce {
		fmt.Println("\nWARNING: This will REPLACE all current data!")
		fmt.Printf("  Instance: %s\n", restoreInstanceName)
		fmt.Printf("  Target:   %s\n", instance.BasePath)
		fmt.Printf("  Source:   %s\n", restoreSource)
		fmt.Print("\nContinue? [y/N]: ")
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Aborted.")
			return
		}
	}

	fmt.Printf("Stopping instance...\n")
	ctx := context.Background()
	runner := instancerunner.NewInstanceController()
	_ = runner.StopInstance(ctx, restoreInstanceName)

	startTime := time.Now()
	fmt.Printf("Restoring from backup...\n")

	// For tar.gz backups, we need to extract to the parent directory
	// because the archive contains the instance folder itself (e.g., instance-name/data/...)
	// For directory backups, we extract directly to instance.BasePath

	var totalSize int64
	var fileCount int

	if sourceInfo.IsDir() {
		// Directory backup: backup current data folder, then copy
		dataPath := filepath.Join(instance.BasePath, "data")
		oldDataPath := dataPath + ".old." + time.Now().Format("20060102150405")
		if _, statErr := os.Stat(dataPath); statErr == nil {
			fmt.Printf("  Backing up current data to %s\n", filepath.Base(oldDataPath))
			if renameErr := os.Rename(dataPath, oldDataPath); renameErr != nil {
				fmt.Printf("Error: %v\n", renameErr)
				os.Exit(1)
			}
		}
		totalSize, fileCount, err = copyBackupDir(restoreSource, instance.BasePath)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			fmt.Println("Restoring previous data...")
			_ = os.RemoveAll(dataPath)
			_ = os.Rename(oldDataPath, dataPath)
			os.Exit(1)
		}
		_ = os.RemoveAll(oldDataPath)
	} else if strings.HasSuffix(restoreSource, ".tar.gz") || strings.HasSuffix(restoreSource, ".tgz") {
		// Tar.gz backup: the archive contains the instance folder (e.g., instance-name/data/...)
		// We strip the first path component and extract directly into instance.BasePath
		// 1. Backup the entire instance folder
		// 2. Delete the instance folder
		// 3. Create fresh instance folder
		// 4. Extract content (stripping first path component) into it

		oldInstancePath := instance.BasePath + ".old." + time.Now().Format("20060102150405")

		// Backup current instance folder
		if _, statErr := os.Stat(instance.BasePath); statErr == nil {
			fmt.Printf("  Backing up current instance to %s\n", filepath.Base(oldInstancePath))
			if renameErr := os.Rename(instance.BasePath, oldInstancePath); renameErr != nil {
				fmt.Printf("Error: Failed to backup instance folder: %v\n", renameErr)
				os.Exit(1)
			}
		}

		// Create fresh instance folder
		if mkErr := os.MkdirAll(instance.BasePath, 0755); mkErr != nil {
			fmt.Printf("Error: Failed to create instance folder: %v\n", mkErr)
			fmt.Println("Restoring previous instance...")
			_ = os.Rename(oldInstancePath, instance.BasePath)
			os.Exit(1)
		}

		// Extract directly to instance.BasePath (stripping first path component)
		totalSize, fileCount, err = extractRestoreTarGz(restoreSource, instance.BasePath)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			fmt.Println("Restoring previous instance...")
			_ = os.RemoveAll(instance.BasePath)
			_ = os.Rename(oldInstancePath, instance.BasePath)
			os.Exit(1)
		}

		// Remove old backup
		_ = os.RemoveAll(oldInstancePath)
	} else {
		fmt.Println("Error: Unknown backup format")
		os.Exit(1)
	}

	fmt.Printf("Restore complete: %d files, %.2f MB, %s\n", fileCount, float64(totalSize)/(1024*1024), time.Since(startTime).Round(time.Second))
	fmt.Println("")
	fmt.Println("The instance was NOT restarted automatically after restore.")
	fmt.Println("Start it manually with:")
	fmt.Printf("  sudo hydraidectl start --instance %s\n", restoreInstanceName)
}

// extractRestoreTarGz extracts the backup archive directly into the instance folder.
// The archive contains paths like "instance-name/data/..." but we strip the first
// path component and extract directly into the target instance folder.
func extractRestoreTarGz(src, instanceBasePath string) (int64, int, error) {
	var totalSize int64
	var fileCount int

	file, err := os.Open(src)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return 0, 0, err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return totalSize, fileCount, err
		}

		// Strip the first path component (the archive's root folder name)
		// e.g., "trendizz-outbound-unittest/data/..." -> "data/..."
		pathParts := strings.SplitN(header.Name, string(os.PathSeparator), 2)
		if len(pathParts) < 2 {
			// This is the root folder itself, skip it
			continue
		}
		relativePath := pathParts[1]
		if relativePath == "" {
			continue
		}

		targetPath := filepath.Join(instanceBasePath, relativePath)
		switch header.Typeflag {
		case tar.TypeDir:
			if mkErr := os.MkdirAll(targetPath, os.FileMode(header.Mode)); mkErr != nil {
				return totalSize, fileCount, mkErr
			}
		case tar.TypeReg:
			_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
			outFile, createErr := os.Create(targetPath)
			if createErr != nil {
				return totalSize, fileCount, createErr
			}
			written, copyErr := io.Copy(outFile, tarReader)
			outFile.Close()
			if copyErr != nil {
				return totalSize, fileCount, copyErr
			}
			_ = os.Chmod(targetPath, os.FileMode(header.Mode))
			totalSize += written
			fileCount++
		}
	}
	return totalSize, fileCount, nil
}
