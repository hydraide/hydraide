package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/elevation"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancerunner"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/validator"
	"github.com/spf13/cobra"
)

var (
	restartInstance        string
	restartCmdTimeout      time.Duration
	restartGracefulTimeout time.Duration
)

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
			os.Exit(3)
		}

		jsonOutput, _ := cmd.Flags().GetBool("json")
		outputFormat, _ := cmd.Flags().GetString("output")
		printJson := jsonOutput || outputFormat == "json"

		// Validate timeouts
		v := validator.New()
		if err := v.ValidateTimeout(context.Background(), "cmd-timeout", restartCmdTimeout); err != nil {
			if printJson {
				printJsonRestart(err)
				return
			}
			fmt.Printf("❌ %v\n", err)
			os.Exit(3)
		}

		if err := v.ValidateTimeout(context.Background(), "graceful-timeout", restartGracefulTimeout); err != nil {
			if printJson {
				printJsonRestart(err)
				return
			}
			fmt.Printf("❌ %v\n", err)
			os.Exit(3)
		}

		// Warn if timeouts are very small
		if restartCmdTimeout < 2*time.Second && !printJson {
			fmt.Printf("⚠️  Warning: cmd-timeout of %v is very small and may cause issues\n", restartCmdTimeout)
		}
		if restartGracefulTimeout < 2*time.Second && !printJson {
			fmt.Printf("⚠️  Warning: graceful-timeout of %v is very small and may cause issues\n", restartGracefulTimeout)
		}

		instanceController := instancerunner.NewInstanceController(
			instancerunner.WithTimeout(restartCmdTimeout),
			instancerunner.WithGracefulStartStopTimeout(restartGracefulTimeout),
		)

		if instanceController == nil {
			fmt.Printf("❌ unsupported operating system: %s", runtime.GOOS)
			os.Exit(3)
		}

		ctx := context.Background()

		exists, err := instanceController.InstanceExists(ctx, restartInstance)
		if err != nil {
			if printJson {
				printJsonRestart(err)
				return
			}
			fmt.Println("failed to verify instance existence: ", err)
		}

		if !exists {
			if printJson {
				printJsonRestart(fmt.Errorf("instance '%s' does not exist", restartInstance))
				return
			}
			fmt.Printf("❌ Instance \"%s\" not found.\nUse `hydraidectl list-instances` to see available instances.\n", restartInstance)
			os.Exit(1)
		}

		// Stop the instance
		if !printJson {
			fmt.Printf("🔁 Restarting instance \"%s\"...\n", restartInstance)
		}

		err = instanceController.StopInstance(ctx, restartInstance)

		if printJson && err != nil {
			printJsonRestart(err)
			return
		}

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
			if !printJson {
				fmt.Printf("✅ Instance \"%s\" has been stopped. Status: inactive\n", restartInstance)
			}
		}

		// We only proceed with the start if the stop phase didn't have a fatal error.
		// A fatal error would have already exited the program above.
		err = instanceController.StartInstance(ctx, restartInstance)

		if printJson {
			printJsonRestart(err)
			return
		}

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
	restartCmd.Flags().DurationVar(&restartCmdTimeout, "cmd-timeout", 30*time.Second, "Timeout for the command execution (min: 1s, max: 15m)")
	restartCmd.Flags().DurationVar(&restartGracefulTimeout, "graceful-timeout", 60*time.Second, "Timeout for graceful start/stop operations (min: 1s, max: 15m)")
	if err := restartCmd.MarkFlagRequired("instance"); err != nil {
		fmt.Printf("Error marking 'instance' flag as required: %v\n", err)
		os.Exit(1)
	}
	restartCmd.Flags().BoolP("json", "j", false, "Return structured output in JSON format")
	restartCmd.Flags().StringP("output", "o", "", "Output format")
}

func printJsonRestart(err error) {
	var jsonResponse *JsonLifecycleInfo
	if err != nil {
		jsonResponse = &JsonLifecycleInfo{
			Instance:  restartInstance,
			Action:    "restart",
			Status:    "error",
			Message:   err.Error(),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}
	} else {
		jsonResponse = &JsonLifecycleInfo{
			Instance:  restartInstance,
			Action:    "restart",
			Status:    "success",
			Message:   "instance restarted successfully",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}
	}

	outputJSON, err := json.MarshalIndent(jsonResponse, "", "  ")
	if err != nil {
		fmt.Printf("Error generating JSON output: %v", err)
		os.Exit(3)
	}
	fmt.Println(string(outputJSON))
}
