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

// Used by start/stop/restart lifecycle commands to print result in json format
type JsonLifecycleInfo struct {
	Instance  string `json:"instance"`
	Action    string `json:"action"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

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
			os.Exit(3)
		}

		jsonOutput, _ := cmd.Flags().GetBool("json")
		outputFormat, _ := cmd.Flags().GetString("output")
		printJson := jsonOutput || outputFormat == "json"

		instanceController := instancerunner.NewInstanceController(
			instancerunner.WithTimeout(20*time.Second),
			instancerunner.WithGracefulStartStopTimeout(10*time.Second),
		)

		if instanceController == nil {
			fmt.Printf("❌ unsupported operating system: %s", runtime.GOOS)
			os.Exit(3)
		}

		ctx := context.Background()

		exists, err := instanceController.InstanceExists(ctx, startInstance)
		if err != nil {
			if printJson {
				printJsonStart(err)
				return
			}
			fmt.Println("failed to verify instance existence: ", err)
		}

		if !exists {
			if printJson {
				printJsonStart(fmt.Errorf("instance '%s' not found", startInstance))
				return
			}
			fmt.Printf("❌ Instance \"%s\" not found.\nUse `hydraidectl list-instances` to see available instances.\n", startInstance)
			os.Exit(1)
		}

		err = instanceController.StartInstance(ctx, startInstance)

		if printJson {
			printJsonStart(err)
			return
		}

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
	startCmd.Flags().BoolP("json", "j", false, "Return structured output in JSON format")
	startCmd.Flags().StringP("output", "o", "", "Output format")
	if err := startCmd.MarkFlagRequired("instance"); err != nil {
		fmt.Printf("Error marking 'instance' flag as required: %v\n", err)
		os.Exit(1)
	}
}

func printJsonStart(err error) {
	var jsonResponse *JsonLifecycleInfo
	if err != nil {
		jsonResponse = &JsonLifecycleInfo{
			Instance:  startInstance,
			Action:    "start",
			Status:    "error",
			Message:   err.Error(),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}
	} else {
		jsonResponse = &JsonLifecycleInfo{
			Instance:  startInstance,
			Action:    "start",
			Status:    "success",
			Message:   "instance started successfully",
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
