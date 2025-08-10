package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/elevation"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancerunner"
	"github.com/spf13/cobra"
)

var startInstance string

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the HydrAIDE instance if it is not already running",
	Long: `Starts an existing HydrAIDE instance that has been previously created and registered as a service.
This command can only be used if the instance was first set up with 'init' and then configured as a service with 'service'.
If the instance is already running, the command does nothing.`,
	Run: func(cmd *cobra.Command, args []string) {

		if !elevation.IsElevated() {
			fmt.Println(elevation.Hint(instanceName))
			return
		}

		instanceController := instancerunner.NewInstanceController(
			instancerunner.WithTimeout(20*time.Second),
			instancerunner.WithGracefulStartStopTimeout(10*time.Second),
		)

		if instanceController == nil {
			fmt.Printf("❌ unsupported operating system: %s", runtime.GOOS)
			return
		}

		ctx := context.Background()

		exists, err := instanceController.InstanceExists(ctx, startInstance)
		if err != nil {
			fmt.Println("failed to verify instance existence: ", err)
		}

		if !exists {
			fmt.Printf("❌ Instance \"%s\" not found.\nUse `hydraidectl list-instances` to see available instances.\n", startInstance)
			os.Exit(1)
		}

		err = instanceController.StartInstance(ctx, startInstance)

		if err != nil {
			switch {
			case errors.Is(err, instancerunner.ErrServiceNotFound):
				fmt.Printf("❌ Instance \"%s\" not found.\nUse `hydraidectl list-instances` to see available instances.\n", startInstance)
				os.Exit(1)

			case errors.Is(err, instancerunner.ErrServiceAlreadyRunning):
				fmt.Printf("🟡 Instance \"%s\" is already running. No action taken.\n", startInstance)
				os.Exit(2)

			default:
				var cmdErr *instancerunner.CmdError
				if errors.As(err, &cmdErr) {
					fmt.Printf("❌ Failed to start instance '%s': %v\nOutput: %s\n", startInstance, cmdErr.Err, cmdErr.Output)
				} else {
					fmt.Printf("❌ Failed to start instance '%s': %v\n", startInstance, err)
				}
				os.Exit(3)
			}
		}
		fmt.Printf("✅ Instance \"%s\" successfully started. Status: active\n", startInstance)
	},
}

func init() {
	rootCmd.AddCommand(startCmd)

	startCmd.Flags().StringVarP(&startInstance, "instance", "i", "", "Name of the service instance")
	if err := startCmd.MarkFlagRequired("instance"); err != nil {
		fmt.Println("Error marking 'instance' flag as required:", err)
		os.Exit(1)
	}

}
