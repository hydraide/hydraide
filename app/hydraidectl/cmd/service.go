package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/elevation"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/servicehelper"
	"github.com/spf13/cobra"
)

var instanceName string
var noPrompt bool
var serviceJSON bool
var autoStart bool
var noStart bool

// ServiceOutput represents the JSON output structure for the service command
type ServiceOutput struct {
	Instance           string `json:"instance"`
	BasePath           string `json:"basePath"`
	ServiceFileCreated bool   `json:"serviceFileCreated"`
	Enabled            bool   `json:"enabled"`
	Started            bool   `json:"started"`
	Status             string `json:"status"`
	ErrorMessage       string `json:"errorMessage,omitempty"`
}

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Set up a persistent service for HydrAIDE and start it",
	Long: `Configures HydrAIDE to run as a persistent background service on your system.
This command registers the current HydrAIDE instance as an OS-level service,
ensuring it starts automatically on system boot and keeps running in the background.
Once installed, the service is started immediately.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Check for conflicting flags
		if autoStart && noStart {
			if serviceJSON {
				output := ServiceOutput{
					Instance:     instanceName,
					Status:       "error",
					ErrorMessage: "Cannot use both --auto-start and --no-start flags together",
				}
				outputServiceJSON(output)
			} else {
				fmt.Println("Error: Cannot use both --auto-start and --no-start flags together")
			}
			return
		}

		if instanceName == "" {
			if serviceJSON {
				output := ServiceOutput{
					Status:       "error",
					ErrorMessage: "Please provide a unique instance name using the --instance flag",
				}
				outputServiceJSON(output)
			} else {
				fmt.Println("Please provide a unique instance name using the --instance flag.")
			}
			return
		}

		// Initialize output for JSON mode
		output := ServiceOutput{
			Instance: instanceName,
			Status:   "success",
		}

		fs := filesystem.New()
		sp := servicehelper.New()

		// Check if the service file already exists on the OS
		exists, err := sp.ServiceExists(instanceName)
		if err != nil {
			if serviceJSON {
				output.Status = "error"
				output.ErrorMessage = fmt.Sprintf("Error checking service existence: %v", err)
				outputServiceJSON(output)
			} else {
				fmt.Printf("Error checking service existence: %v\n", err)
			}
			return
		}
		if exists {
			if serviceJSON {
				output.Status = "error"
				output.ErrorMessage = fmt.Sprintf("A service with the name '%s' already exists on this system", instanceName)
				outputServiceJSON(output)
			} else {
				fmt.Printf("A service with the name '%s' already exists on this system. Please choose a different instance name or destroy the existing one.\n", instanceName)
			}
			return
		}

		if !elevation.IsElevated() {
			if serviceJSON {
				output.Status = "error"
				output.ErrorMessage = "Administrator/root privileges required"
				outputServiceJSON(output)
			} else {
				fmt.Println(elevation.Hint(instanceName))
			}
			return
		}

		// Load instance metadata
		if !serviceJSON {
			fmt.Println("🔍 Loading instance metadata...")
		}
		// Use the filesystem utility to get the metadata store
		bm, err := buildmeta.New(fs)
		if err != nil {
			if serviceJSON {
				output.Status = "error"
				output.ErrorMessage = fmt.Sprintf("Failed to load metadata store: %v", err)
				outputServiceJSON(output)
			} else {
				fmt.Println("Failed to load metadata store:", err)
			}
			return
		}
		instanceData, err := bm.GetInstance(instanceName)
		if err != nil {
			if serviceJSON {
				output.Status = "error"
				output.ErrorMessage = fmt.Sprintf("Could not find metadata for instance '%s'. Please run 'hydraidectl init' first", instanceName)
				outputServiceJSON(output)
			} else {
				fmt.Printf("❌ Could not find metadata for instance '%s'.\n", instanceName)
				fmt.Println("👉 Please run 'hydraidectl init' first to create the instance.")
			}
			return
		}
		basepath := instanceData.BasePath
		output.BasePath = basepath

		if !serviceJSON {
			fmt.Println("Base path for instance found in metadata:", basepath)
		}

		// Generate the service file
		err = sp.GenerateServiceFile(instanceName, basepath)
		if err != nil {
			if serviceJSON {
				output.Status = "error"
				output.ErrorMessage = fmt.Sprintf("Error generating service file: %v", err)
				outputServiceJSON(output)
			} else {
				fmt.Printf("Error generating service file: %v\n", err)
			}
			return
		}
		output.ServiceFileCreated = true
		if !serviceJSON {
			fmt.Printf("Service file for instance '%s' created successfully.\n", instanceName)
		}

		// Determine whether to start the service
		shouldStart := false
		if autoStart || noPrompt { // noPrompt is deprecated but kept for backward compatibility
			shouldStart = true
		} else if noStart {
			shouldStart = false
		} else {
			// Interactive mode - ask the user (only if not in JSON mode)
			if !serviceJSON {
				fmt.Print("Do you want to enable and start this service now? (y/n): ")
				reader := bufio.NewReader(os.Stdin)
				response, _ := reader.ReadString('\n')
				response = strings.ToLower(strings.TrimSpace(response))
				if response == "y" || response == "yes" {
					shouldStart = true
				}
			}
		}

		if !shouldStart {
			if serviceJSON {
				output.Enabled = false
				output.Started = false
				outputServiceJSON(output)
			} else {
				fmt.Println("Service setup complete. You can enable and start it manually later.")
			}
			return
		}

		err = sp.EnableAndStartService(instanceName, basepath)
		if err != nil {
			if serviceJSON {
				output.Status = "error"
				output.ErrorMessage = fmt.Sprintf("Error enabling and starting service: %v", err)
				output.Enabled = false
				output.Started = false
				outputServiceJSON(output)
			} else {
				fmt.Printf("Error enabling and starting service: %v\n", err)
			}
			return
		}
		output.Enabled = true
		output.Started = true
		if serviceJSON {
			outputServiceJSON(output)
		} else {
			fmt.Printf("Service '%s' enabled and started successfully.\n", instanceName)
		}
	},
}

// outputServiceJSON outputs the service status as JSON
func outputServiceJSON(output ServiceOutput) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		// Fallback to fmt.Printf if JSON encoding fails
		fmt.Printf(`{"status":"error","errorMessage":"Failed to encode JSON: %v"}\n`, err)
	}
}

func init() {
	rootCmd.AddCommand(serviceCmd)

	serviceCmd.Flags().StringVarP(&instanceName, "instance", "i", "", "Unique name for the service instance")
	serviceCmd.Flags().BoolVar(&noPrompt, "no-prompt", false, "Skip prompts and enable/start the service automatically (deprecated, use --auto-start)")
	serviceCmd.Flags().BoolVar(&serviceJSON, "json", false, "Output in JSON format")
	serviceCmd.Flags().BoolVar(&autoStart, "auto-start", false, "Automatically enable and start the service without prompting")
	serviceCmd.Flags().BoolVar(&noStart, "no-start", false, "Create the service file but do not enable or start the service")
	if err := serviceCmd.MarkFlagRequired("instance"); err != nil {
		fmt.Println("Error marking 'instance' flag as required:", err)
		os.Exit(1)
	}

}
