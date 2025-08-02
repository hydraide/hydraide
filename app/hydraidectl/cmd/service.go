package cmd

import (
	"fmt"

	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/servicehelper"
	"github.com/spf13/cobra"
)

var instanceName string
var noPrompt bool

const (
	TEMP_FILENAME = "hydraide-cache"
)

// serviceCmd represents the service command
var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Set up a persistent service for hydraserver",
	Run: func(cmd *cobra.Command, args []string) {
		if instanceName == "" {
			fmt.Println("Please provide a unique instance name using the --instance flag.")
			return
		}

		sp := servicehelper.New()

		// Check if the service already exists
		exists, err := sp.ServiceExists(instanceName)
		if err != nil {
			fmt.Printf("Error checking service existence: %v\n", err)
			return
		}
		if exists {
			fmt.Printf("A service with the name '%s' already exists. Please choose a different instance name.\n", instanceName)
			return
		}

		// get basepath from build meta
		bm, err := buildmeta.New()
		if err != nil {
			fmt.Println("Failed to load buildmeat")
		}
		basepath, err := bm.Get("basepath")
		if err != nil {
			fmt.Println("Base Path is not found in metadata", err)
			return
		}

		fmt.Println("Base Path is found in metadata", basepath)

		// Generate the service file
		err = sp.GenerateServiceFile(instanceName, basepath)
		if err != nil {
			fmt.Printf("Error generating service file: %v\n", err)
			return
		}

		fmt.Printf("Service file for instance '%s' created successfully.\n", instanceName)

		// Prompt to enable and start the service
		if !noPrompt {
			var response string
			fmt.Print("Do you want to enable and start this service now? (y/n): ")
			fmt.Scanln(&response)
			if response != "y" {
				fmt.Println("Service setup complete. You can enable and start it manually later.")
				return
			}
		}

		err = sp.EnableAndStartService(instanceName)
		if err != nil {
			fmt.Printf("Error enabling and starting service: %v\n", err)
			return
		}

		fmt.Printf("Service '%s' enabled and started successfully.\n", instanceName)
	},
}

func init() {
	rootCmd.AddCommand(serviceCmd)

	serviceCmd.Flags().StringVarP(&instanceName, "instance", "i", "", "Unique name for the service instance")
	serviceCmd.Flags().BoolVar(&noPrompt, "no-prompt", false, "Skip prompts and enable/start the service automatically")
}
