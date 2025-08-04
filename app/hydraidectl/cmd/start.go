package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancerunner"
	"github.com/spf13/cobra"
)

var startInstance string

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the HydrAIDE instance",
	Run: func(cmd *cobra.Command, args []string) {
		instanceController := instancerunner.NewInstanceController(
			instancerunner.WithTimeout(20*time.Second),
			instancerunner.WithGracefulStartStopTimeout(10*time.Second),
		)
		context := context.Background()
		err := instanceController.StartInstance(context, startInstance)

		if err != nil {
			switch {
			case errors.Is(err, instancerunner.ErrServiceNotFound):
				fmt.Printf("❌ Instance \"%s\" not found.\n", startInstance)
				// Todo: Change the message when list-instances is available
				// fmt.Printf("❌ Instance \"%s\" not found.\nUse `hydraidectl list-instances` to see available instances.\n", startInstance)
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
	startCmd.MarkFlagRequired("instance")
}
