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

var restartInstance string

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the HydrAIDE instance if it is running (gracefully stops and starts it)",
	Long: `Restarts an existing HydrAIDE instance that has been previously created and registered as a service.
This command first gracefully stops the running instance, ensuring all operations are completed,
and then starts it again. It can only be used if the instance was first set up with 'init'
and then configured as a service with 'service'.`,
	Run: func(cmd *cobra.Command, args []string) {

		if !elevation.IsElevated() {
			fmt.Println(elevation.Hint(instanceName))
			return
		}

		instanceController := instancerunner.NewInstanceController(
			instancerunner.WithTimeout(30*time.Second),
			instancerunner.WithGracefulStartStopTimeout(10*time.Second),
		)

		if instanceController == nil {
			fmt.Printf("❌ unsupported operating system: %s", runtime.GOOS)
			return
		}

		ctx := context.Background()

		exists, err := instanceController.InstanceExists(ctx, restartInstance)
		if err != nil {
			fmt.Println("failed to verify instance existence: ", err)
		}

		if !exists {
			fmt.Printf("❌ Instance \"%s\" not found.\nUse `hydraidectl list-instances` to see available instances.\n", restartInstance)
			os.Exit(1)
		}

		// Stop the instance
		fmt.Printf("🔁 Restarting instance \"%s\"...\n", restartInstance)

		err = instanceController.StopInstance(ctx, restartInstance)

		if err != nil {
			switch {
			case errors.Is(err, instancerunner.ErrServiceNotFound):
				fmt.Printf("❌ Instance \"%s\" not found.\nUse `hydraidectl list-instances` to see available instances.\n", restartInstance)
				os.Exit(1)

			case errors.Is(err, instancerunner.ErrServiceNotRunning):
				fmt.Printf("🟡 Instance \"%s\" is already stopped. No action taken.\n", restartInstance)
				os.Exit(2)

			default:
				var cmdErr *instancerunner.CmdError
				if errors.As(err, &cmdErr) {
					fmt.Printf("❌ Failed to stop instance '%s': %v\nOutput: %s\n", restartInstance, cmdErr.Err, cmdErr.Output)
				} else {
					fmt.Printf("❌ Failed to stop instance '%s': %v\n", restartInstance, err)
				}
				os.Exit(3)
			}
		} else {
			fmt.Printf("✅ Instance \"%s\" has been stopped. Status: inactive\n", restartInstance)
		}

		// We only proceed with the start if the stop phase didn't have a fatal error.
		// A fatal error would have already exited the program above.
		err = instanceController.StartInstance(ctx, restartInstance)

		if err != nil {

			var opErr *instancerunner.OperationError

			if errors.As(err, &opErr) {
				fmt.Printf("❌ Failed to start instance '%s': %v\n", restartInstance, opErr)
			} else {
				fmt.Printf("❌ Failed to start instance '%s': %v\n", restartInstance, err)
			}
			os.Exit(3)
		}

		fmt.Printf("✅ Restart complete. Status: active\n")
	},

	//   Currently, instancerunner, handles the graceful shutdown - need to continously print till we get a return.
	//  todo: During the shutdown, it's highly recommended to periodically inform the user
	//   that the shutdown is still in progress — and **strongly advise** them not to shut down the server/PC
}

func init() {
	rootCmd.AddCommand(restartCmd)

	restartCmd.Flags().StringVarP(&restartInstance, "instance", "i", "", "Name of the service instance")
	if err := restartCmd.MarkFlagRequired("instance"); err != nil {
		fmt.Println("Error marking 'instance' flag as required:", err)
		os.Exit(1)
	}

}
