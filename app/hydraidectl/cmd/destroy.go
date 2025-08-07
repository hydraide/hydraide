package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancerunner"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/servicehelper"
	"github.com/spf13/cobra"
)

var (
	destroyInstance string
	purgeData       bool
)

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Stops, disables, and removes a HydrAIDE instance.",
	Long: `Stops and removes a HydrAIDE instance.

This command will always perform a graceful shutdown and remove the system service definition.
Use the --purge flag to also permanently delete the entire base directory, including all data, certificates, and the server binary. This action is irreversible and requires manual confirmation.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()

		fs := filesystem.New()
		bm, err := buildmeta.New(fs)
		if err != nil {
			fmt.Println("❌ Failed to initialize metadata store:", err)
			os.Exit(1)
		}

		instanceController := instancerunner.NewInstanceController()
		serviceManager := servicehelper.New()

		fmt.Printf("🔥 Initializing destruction for instance: \"%s\"\n", destroyInstance)

		fmt.Println("🔄 Checking instance status...")
		err = instanceController.StopInstance(ctx, destroyInstance)
		if err != nil && err != instancerunner.ErrServiceNotRunning && err != instancerunner.ErrServiceNotFound {
			fmt.Printf("❌ Could not stop instance '%s': %v\n", destroyInstance, err)
			fmt.Println("🚫 Aborting destroy operation. Please stop the service manually before proceeding.")
			os.Exit(1)
		}
		fmt.Println("✅ Instance is stopped or was not running.")

		fmt.Println("🗑️  Removing service definition...")
		if err := serviceManager.RemoveService(destroyInstance); err != nil {
			fmt.Printf("⚠️  Could not remove service definition: %v\n", err)
			fmt.Println("   You may need to remove it manually. Continuing...")
		} else {
			fmt.Println("✅ Service definition removed.")
		}

		// FIXED: Added delay to prevent console output overlap.
		time.Sleep(500 * time.Millisecond)

		instanceData, metaErr := bm.GetInstance(destroyInstance)
		if metaErr != nil {
			fmt.Printf("⚠️  Could not find metadata for instance '%s'. Cannot purge data.\n", destroyInstance)
			fmt.Printf("✅ Destruction of service '%s' complete.\n", destroyInstance)
			os.Exit(0)
		}
		basePath := instanceData.BasePath

		if purgeData {
			fmt.Println("\n====================================================================")
			fmt.Println("‼️ DANGER: FULL DATA PURGE INITIATED ‼️")
			fmt.Printf("⚠️ You are about to permanently delete the entire base path for instance '%s'.\n", destroyInstance)
			fmt.Printf("   Directory to be deleted: %s\n", basePath)
			fmt.Println("   This includes all data, certificates, logs, and the binary.")
			fmt.Println("   This operation is IRREVERSIBLE.")
			fmt.Println("====================================================================")
			fmt.Printf("\n👉 To confirm, type the full instance name ('%s'): ", destroyInstance)

			reader := bufio.NewReader(os.Stdin)
			input, _ := reader.ReadString('\n')
			if strings.TrimSpace(input) != destroyInstance {
				fmt.Println("🚫 Input did not match. Data purge aborted.")
				os.Exit(0)
			}

			fmt.Println("✅ Confirmation received. Proceeding with base path deletion.")

			// FIXED: Use the robust fs.RemoveDir which wraps os.RemoveAll.
			if err := fs.RemoveDir(ctx, basePath); err != nil {
				fmt.Printf("❌ An error occurred during deletion: %v\n", err)
				fmt.Println("   Please check file permissions and try again.")
				os.Exit(1)
			}
			fmt.Printf("✅ Base path '%s' deleted successfully.\n", basePath)
		} else {
			fmt.Println("\nℹ️  --purge flag not specified. The service was removed, but all data remains.")
			fmt.Printf("   Data directory: %s\n", basePath)
		}

		// CHANGED: Clean up the instance from the metadata file.
		if err := bm.DeleteInstance(destroyInstance); err != nil {
			fmt.Printf("⚠️  Could not remove instance metadata: %v\n", err)
		} else {
			fmt.Println("✅ Instance metadata cleaned up.")
		}
		fmt.Printf("\n✅ Destruction of instance '%s' complete.\n", destroyInstance)
	},
}

func init() {
	rootCmd.AddCommand(destroyCmd)
	destroyCmd.Flags().StringVarP(&destroyInstance, "instance", "i", "", "Name of the HydrAIDE instance to destroy (required)")
	destroyCmd.Flags().BoolVar(&purgeData, "purge", false, "Permanently delete the entire base path for the instance")
	destroyCmd.MarkFlagRequired("instance")
}
