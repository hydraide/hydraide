package cmd

import (
	"context"
	"fmt"
	"time"

	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/downloader"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancedetector"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/servicehelper"
	"github.com/spf13/cobra"
)

var updateInstance string

var updateCmd = &cobra.Command{

	Use:   "update",
	Short: "Frissít egy instance-t a legújabb HydrAIDE verzióra",
	Long: `Az pdate csak a legújabb bináris letöltését végzi el. Az updated követően futtasd a restart parancsot ha 
már futott, vagy a start parancsot, ha még nem futott az instance`,

	Run: func(cmd *cobra.Command, args []string) {

		downloaderInterface := downloader.New()
		metaInterface, err := buildmeta.New(filesystem.New())
		if err != nil {
			// Handle error if metadata store cannot be created
			fmt.Printf("Error while loading metadata: %v\n", err)
			return
		}

		// check the current version of the instance
		instanceMeta, err := metaInterface.GetInstance(updateInstance)
		if err != nil {
			// Handle error if instance metadata cannot be retrieved
			fmt.Printf("Error while retrieving instance metadata: %v\n", err)
			return
		}

		detector, err := instancedetector.NewDetector()
		if err != nil {
			fmt.Printf("Failed to load instances: %v", err)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		status, err := detector.GetInstanceStatus(ctx, updateInstance)
		if status != "inactive" && status != "unknown" {
			fmt.Printf("Please stop the instance '%s' before updating it.\n", updateInstance)
			return
		}

		latestVersion := downloaderInterface.GetLatestVersionWithoutServerPrefix()
		if latestVersion == "unknown" {
			fmt.Println("Unable to determine the latest version of HydrAIDE. Please try again later.")
			return
		}
		if latestVersion == instanceMeta.Version {
			fmt.Printf("The instance '%s' is already up to date with version %s.\n", updateInstance, latestVersion)
			return
		}

		// try toi download the latest version
		downloadedVersion, err := downloaderInterface.DownloadHydraServer("latest", instanceMeta.BasePath)
		if err != nil {
			fmt.Printf("Error while downloading the latest version of HydrAIDE: %v\n", err)
			return
		}

		instanceMeta.Version = downloadedVersion
		if err := metaInterface.SaveInstance(updateInstance, instanceMeta); err != nil {
			// Handle error if instance metadata cannot be saved
			fmt.Printf("Error while saving instance metadata: %v\n", err)
			return
		}

		// remove the old service, and create a new one with the updated version
		serviceHelperInterface := servicehelper.New()
		_ = serviceHelperInterface.RemoveService(updateInstance)
		_ = serviceHelperInterface.GenerateServiceFile(updateInstance, instanceMeta.BasePath)

		fmt.Printf("Instance '%s' has been successfully updated to version %s.\n", updateInstance, downloadedVersion)
		fmt.Println("Please run 'sudo hydraidectl start --instance ", updateInstance, "' to apply the update.")

	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.Flags().StringVarP(&updateInstance, "instance", "i", "", "Name of the service instance")
}
