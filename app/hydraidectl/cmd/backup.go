package cmd

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancerunner"
	"github.com/spf13/cobra"
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Create a backup of HydrAIDE instance data",
	Run:   runBackupCmd,
}

var (
	backupInstanceName string
	backupTarget       string
	backupCompress     bool
	backupNoStop       bool
)

func init() {
	rootCmd.AddCommand(backupCmd)
	backupCmd.Flags().StringVarP(&backupInstanceName, "instance", "i", "", "Instance name (required)")
	backupCmd.Flags().StringVarP(&backupTarget, "target", "t", "", "Target backup path (required)")
	backupCmd.Flags().BoolVar(&backupCompress, "compress", false, "Compress backup as tar.gz")
	backupCmd.Flags().BoolVar(&backupNoStop, "no-stop", false, "Do not stop instance before backup")
	_ = backupCmd.MarkFlagRequired("instance")
	_ = backupCmd.MarkFlagRequired("target")
}

func runBackupCmd(cmd *cobra.Command, args []string) {
	fs := filesystem.New()
	store, err := buildmeta.New(fs)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	instance, err := store.GetInstance(backupInstanceName)
	if err != nil {
		fmt.Printf("Error: Instance not found: %v\n", err)
		os.Exit(1)
	}

	if !backupNoStop {
		fmt.Printf("Stopping instance...\n")
		ctx := context.Background()
		runner := instancerunner.NewInstanceController()
		_ = runner.StopInstance(ctx, backupInstanceName)
	}

	startTime := time.Now()
	fmt.Printf("Creating backup...\n")
	fmt.Printf("  Source: %s\n", instance.BasePath)
	fmt.Printf("  Target: %s\n", backupTarget)

	var totalSize int64
	var fileCount int

	if backupCompress {
		totalSize, fileCount, err = createBackupTarGz(instance.BasePath, backupTarget)
	} else {
		totalSize, fileCount, err = copyBackupDir(instance.BasePath, backupTarget)
	}

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Backup complete: %d files, %.2f MB, %s\n", fileCount, float64(totalSize)/(1024*1024), time.Since(startTime).Round(time.Second))

	if !backupNoStop {
		fmt.Printf("Starting instance...\n")
		ctx := context.Background()
		runner := instancerunner.NewInstanceController()
		_ = runner.StartInstance(ctx, backupInstanceName)
	}
}

func copyBackupDir(src, dst string) (int64, int, error) {
	var totalSize int64
	var fileCount int

	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(src, path)
		targetPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
		dstFile, err := os.Create(targetPath)
		if err != nil {
			return err
		}
		defer dstFile.Close()

		written, err := io.Copy(dstFile, srcFile)
		if err != nil {
			return err
		}
		totalSize += written
		fileCount++
		return os.Chmod(targetPath, info.Mode())
	})
	return totalSize, fileCount, err
}

func createBackupTarGz(src, dst string) (int64, int, error) {
	var totalSize int64
	var fileCount int

	_ = os.MkdirAll(filepath.Dir(dst), 0755)
	file, err := os.Create(dst)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	gzWriter := gzip.NewWriter(file)
	defer gzWriter.Close()
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	baseDir := filepath.Base(src)
	err = filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		header, _ := tar.FileInfoHeader(info, "")
		relPath, _ := filepath.Rel(src, path)
		header.Name = filepath.Join(baseDir, relPath)
		_ = tarWriter.WriteHeader(header)
		if info.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		written, _ := io.Copy(tarWriter, f)
		totalSize += written
		fileCount++
		return nil
	})
	return totalSize, fileCount, err
}
