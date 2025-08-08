package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancerunner"
	"github.com/spf13/cobra"
)

var stopInstance string

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the HydrAIDE instance",
	Run: func(cmd *cobra.Command, args []string) {

		if os.Geteuid() != 0 {
			fmt.Println("This command must be run as root or with sudo to create a system service.")
			fmt.Println("Please run 'sudo hydraidectl stop --instance " + instanceName + "'")
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

		exists, err := instanceController.InstanceExists(ctx, stopInstance)
		if err != nil {
			fmt.Println("failed to verify instance existence: ", err)
		}

		if !exists {
			fmt.Printf("❌ Instance \"%s\" not found.\nUse `hydraidectl list-instances` to see available instances.\n", stopInstance)
			os.Exit(1)
		}

		fmt.Printf("🟡 Shutting down instance \"%s\"...\n", stopInstance)
		fmt.Println("⚠️  HydrAIDE shutdown in progress... Do not power off or kill the service. Data may be flushing to disk.")

		err = instanceController.StopInstance(ctx, stopInstance)

		if err != nil {
			switch {
			case errors.Is(err, instancerunner.ErrServiceNotFound):
				fmt.Printf("❌ Instance \"%s\" not found.\nUse `hydraidectl list-instances` to see available instances.\n", stopInstance)
				os.Exit(1)

			case errors.Is(err, instancerunner.ErrServiceNotRunning):
				fmt.Printf("🟡 Instance \"%s\" is already stopped. No action taken.\n", stopInstance)
				os.Exit(2)

			default:
				var cmdErr *instancerunner.CmdError
				if errors.As(err, &cmdErr) {
					fmt.Printf("❌ Failed to stop instance '%s': %v\nOutput: %s\n", stopInstance, cmdErr.Err, cmdErr.Output)
				} else {
					fmt.Printf("❌ Failed to stop instance '%s': %v\n", stopInstance, err)
				}
				os.Exit(3)
			}
		}

		fmt.Printf("✅ Instance \"%s\" has been stopped. Status: inactive\n", stopInstance)

		//  todo: During the shutdown, it's highly recommended to periodically inform the user
		//   that the shutdown is still in progress — and **strongly advise** them not to shut down the server/PC
		//   Currently, instancerunner, handles the graceful shutdown - need to continously print till we get a return.

	},
}

func init() {
	rootCmd.AddCommand(stopCmd)

	stopCmd.Flags().StringVarP(&stopInstance, "instance", "i", "", "Name of the service instance")
	if err := stopCmd.MarkFlagRequired("instance"); err != nil {
		fmt.Println("Error marking 'instance' flag as required:", err)
		os.Exit(1)
	}
}
