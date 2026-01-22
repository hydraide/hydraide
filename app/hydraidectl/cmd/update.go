package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/downloader"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancedetector"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancehealth"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancerunner"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/servicehelper"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

// CLI flag values for update command
var (
	updateInstance string
	updateNoStart  bool
)

// updateCmd defines the "update" subcommand for the CLI.
// Flow: gracefully stop (if running) -> download latest binary -> save metadata
// -> (re)generate service definition -> optionally start instance -> wait for healthy.
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update an instance to the latest HydrAIDE version",
	Long: `The update command performs an all-in-one upgrade:
1) gracefully stops the instance if it is running,
2) downloads the latest server binary,
3) updates the service definition,
4) optionally starts the instance (unless --no-start is used),
5) waits until it becomes healthy (if started).

Use --no-start when you want to update the binary without starting the server,
for example before running a migration.`,

	Run: func(cmd *cobra.Command, args []string) {
		// Initialize helpers
		downloaderInterface := downloader.New()
		metaInterface, err := buildmeta.New(filesystem.New())
		if err != nil {
			fmt.Printf("Error while loading metadata: %v\n", err)
			os.Exit(1)
		}

		// Check if any instances exist
		instances, err := metaInterface.GetAllInstances()
		if err != nil {
			fmt.Printf("Error while retrieving instances: %v\n", err)
			os.Exit(1)
		}
		if len(instances) == 0 {
			fmt.Println("No HydrAIDE instances found.")
			fmt.Println("Use 'hydraidectl init' to create and initialize an instance before running update.")
			return
		}

		// Load instance metadata
		instanceMeta, err := metaInterface.GetInstance(updateInstance)
		if err != nil {
			fmt.Printf("Error while retrieving instance metadata: %v\n", err)
			os.Exit(1)
		}

		// Check latest available version
		latestVersion := downloaderInterface.GetLatestVersionWithoutServerPrefix()
		if latestVersion == "unknown" {
			fmt.Println("Unable to determine the latest version of HydrAIDE. Please try again later.")
			os.Exit(1)
		}
		if latestVersion == instanceMeta.Version {
			fmt.Printf("The instance %q is already up to date (version %s).\n", updateInstance, latestVersion)
			os.Exit(0)
		}

		// Prepare detectors/controllers
		detector, err := instancedetector.NewDetector()
		if err != nil {
			fmt.Printf("Failed to load instances: %v\n", err)
			os.Exit(1)
		}

		// Context for the whole operation (stop/start/health)
		ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
		defer cancel()

		serviceHelperInterface := servicehelper.New()
		instanceController := instancerunner.NewInstanceController(
			instancerunner.WithTimeout(20*time.Second),
			instancerunner.WithGracefulStartStopTimeout(600*time.Second),
		)

		// Graceful stop if active
		status, err := detector.GetInstanceStatus(ctx, updateInstance)
		if err != nil {
			fmt.Printf("Warning: failed to get instance status for %q: %v\n", updateInstance, err)
		}
		if status != "inactive" && status != "unknown" {
			if err := instanceController.StopInstance(ctx, updateInstance); err != nil {
				fmt.Printf("Error while stopping the instance %q: %v\n", updateInstance, err)
				os.Exit(1)
			}
			fmt.Printf("Instance %q stopped gracefully.\n", updateInstance)
		}

		// Wire progress bar
		var bar *progressbar.ProgressBar
		progressFn := func(downloaded, total int64, percent float64) {
			if bar == nil {
				bar = progressbar.NewOptions64(
					total,
					progressbar.OptionSetDescription("Downloading"),
					progressbar.OptionShowBytes(true),
				)
			}
			if err := bar.Set64(downloaded); err != nil {
				fmt.Printf("❌ Error updating progress bar: %v\n", err)
				os.Exit(1)
			}
		}
		downloaderInterface.SetProgressCallback(progressFn)

		// Download latest binary into base path
		downloadedVersion, err := downloaderInterface.DownloadHydraServer("latest", instanceMeta.BasePath)
		if err != nil {
			fmt.Printf("Error while downloading the latest version of HydrAIDE: %v\n", err)
			os.Exit(1)
		}

		// Save new version into metadata
		instanceMeta.Version = downloadedVersion
		if err := metaInterface.SaveInstance(updateInstance, instanceMeta); err != nil {
			fmt.Printf("Error while saving instance metadata: %v\n", err)
			os.Exit(1)
		}

		// (Re)create service definition for the updated binary
		_ = serviceHelperInterface.RemoveService(updateInstance)

		// If --no-start flag is set, register service without starting
		if updateNoStart {
			_ = serviceHelperInterface.GenerateServiceFileNoStart(updateInstance, instanceMeta.BasePath)
			fmt.Printf("Instance %q has been successfully updated to version %s.\n", updateInstance, downloadedVersion)
			fmt.Println("The instance was NOT started (--no-start flag). Start it manually with:")
			fmt.Printf("  sudo hydraidectl start --instance %s\n", updateInstance)
			return
		}

		// Normal flow: generate service file (which also starts it)
		_ = serviceHelperInterface.GenerateServiceFile(updateInstance, instanceMeta.BasePath)

		fmt.Printf("Instance %q has been successfully updated to version %s and started.\n", updateInstance, downloadedVersion)

		// Wait until healthy (respecting context timeout)
		instanceHealthInterface := instancehealth.NewInstanceHealth()
		startWait := time.Now()
		for {

			// Check for timeout/cancellation
			select {
			case <-ctx.Done():
				fmt.Printf("Timed out while waiting for instance %q to become healthy (waited %s).\n", updateInstance, time.Since(startWait).Truncate(time.Second))
				os.Exit(1)
			default:
			}

			healthStatus := instanceHealthInterface.GetHealthStatus(ctx, updateInstance)
			if healthStatus.Status == "healthy" {
				fmt.Printf("Instance %q is now healthy and ready for use. (Waited %s)\n", updateInstance, time.Since(startWait).Truncate(time.Second))
				break
			}

			fmt.Printf("Waiting for instance %q to become healthy...\n", updateInstance)
			time.Sleep(1 * time.Second)

		}

	},
}

func init() {
	// Register the "update" subcommand and its flags on the root command.
	rootCmd.AddCommand(updateCmd)
	updateCmd.Flags().StringVarP(&updateInstance, "instance", "i", "", "Name of the service instance")
	updateCmd.Flags().BoolVar(&updateNoStart, "no-start", false, "Update without starting the instance (useful before migration)")
}
