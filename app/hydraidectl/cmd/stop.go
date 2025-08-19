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
	"github.com/spf13/cobra"
)

var stopInstance string

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the HydrAIDE instance if it is running",
	Long: `Stops an existing HydrAIDE instance that has been previously created and registered as a service.
This command can only be used if the instance was first set up with 'init' and then configured as a service with 'service'.
If the instance is not running, the command does nothing.`,
	Run: func(cmd *cobra.Command, args []string) {

		if !elevation.IsElevated() {
			fmt.Println(elevation.Hint(instanceName))
			os.Exit(3)
		}

		jsonOutput, _ := cmd.Flags().GetBool("json")
		outputFormat, _ := cmd.Flags().GetString("output")
		printJson := jsonOutput || outputFormat == "json"

		instanceController := instancerunner.NewInstanceController(
			instancerunner.WithTimeout(30*time.Second),
			instancerunner.WithGracefulStartStopTimeout(600*time.Second),
		)

		if instanceController == nil {
			fmt.Printf("❌ unsupported operating system: %s", runtime.GOOS)
			os.Exit(3)
		}

		ctx := context.Background()

		exists, err := instanceController.InstanceExists(ctx, stopInstance)
		if err != nil {
			if printJson {
				printJsonStop(err)
				return
			}
			fmt.Println("failed to verify instance existence: ", err)
		}

		if !exists {
			if printJson {
				printJsonStop(fmt.Errorf("instance '%s' does not exist", stopInstance))
				return
			}
			fmt.Printf("❌ Instance \"%s\" not found.\nUse `hydraidectl list-instances` to see available instances.\n", stopInstance)
			os.Exit(1)
		}

		if !printJson {
			fmt.Printf("🟡 Shutting down instance \"%s\"...\n", stopInstance)
			fmt.Println("⚠️  HydrAIDE shutdown in progress... Do not power off or kill the service. Data may be flushing to disk.")
		}

		err = instanceController.StopInstance(ctx, stopInstance)

		if printJson {
			printJsonStop(err)
			return
		}

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

	stopCmd.Flags().BoolP("json", "j", false, "Return structured output in JSON format")
	stopCmd.Flags().StringP("output", "o", "", "Output format")
}

func printJsonStop(err error) {
	var jsonResponse *JsonLifecycleInfo
	if err != nil {
		jsonResponse = &JsonLifecycleInfo{
			Instance:  stopInstance,
			Action:    "stop",
			Status:    "error",
			Message:   err.Error(),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}
	} else {
		jsonResponse = &JsonLifecycleInfo{
			Instance:  stopInstance,
			Action:    "stop",
			Status:    "success",
			Message:   "instance stopped successfully",
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
