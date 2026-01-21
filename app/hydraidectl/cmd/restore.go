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
dataPath := filepath.Join(instance.BasePath, "data")
oldDataPath := dataPath + ".old." + time.Now().Format("20060102150405")
if _, statErr := os.Stat(dataPath); statErr == nil {
fmt.Printf("  Backing up current data to %s\n", filepath.Base(oldDataPath))
if renameErr := os.Rename(dataPath, oldDataPath); renameErr != nil {
fmt.Printf("Error: %v\n", renameErr)
os.Exit(1)
}
}
var totalSize int64
var fileCount int
if sourceInfo.IsDir() {
totalSize, fileCount, err = copyBackupDir(restoreSource, instance.BasePath)
} else if strings.HasSuffix(restoreSource, ".tar.gz") || strings.HasSuffix(restoreSource, ".tgz") {
totalSize, fileCount, err = extractRestoreTarGz(restoreSource, instance.BasePath)
} else {
fmt.Println("Error: Unknown backup format")
os.Exit(1)
}
if err != nil {
fmt.Printf("Error: %v\n", err)
fmt.Println("Restoring previous data...")
_ = os.RemoveAll(dataPath)
_ = os.Rename(oldDataPath, dataPath)
os.Exit(1)
}
_ = os.RemoveAll(oldDataPath)
fmt.Printf("Restore complete: %d files, %.2f MB, %s\n", fileCount, float64(totalSize)/(1024*1024), time.Since(startTime).Round(time.Second))
fmt.Printf("Starting instance...\n")
_ = runner.StartInstance(ctx, restoreInstanceName)
}
func extractRestoreTarGz(src, dst string) (int64, int, error) {
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
targetPath := filepath.Join(dst, header.Name)
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
