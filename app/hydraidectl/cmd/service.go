package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/servicehelper"
	"github.com/spf13/cobra"
)

var instanceName string
var noPrompt bool

const (
	TEMP_FILENAME = "hydraide-cache"
)

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Set up a persistent service for hydraserver",
	Run: func(cmd *cobra.Command, args []string) {
		if instanceName == "" {
			fmt.Println("Please provide a unique instance name using the --instance flag.")
			return
		}

		fs := filesystem.New()
		sp := servicehelper.New()

		// Check if the service file already exists on the OS
		exists, err := sp.ServiceExists(instanceName)
		if err != nil {
			fmt.Printf("Error checking service existence: %v\n", err)
			return
		}
		if exists {
			fmt.Printf("A service with the name '%s' already exists on this system. Please choose a different instance name or destroy the existing one.\n", instanceName)
			return
		}

		if os.Geteuid() != 0 {
			fmt.Println("This command must be run as root or with sudo to create a system service.")
			fmt.Println("Please run 'sudo hydraidectl service --instance " + instanceName + "'")
			return
		}

		// Load instance metadata
		fmt.Println("🔍 Loading instance metadata...")
		// Use the filesystem utility to get the metadata store
		bm, err := buildmeta.New(fs)
		if err != nil {
			fmt.Println("Failed to load metadata store:", err)
			return
		}
		instanceData, err := bm.GetInstance(instanceName)
		if err != nil {
			fmt.Printf("❌ Could not find metadata for instance '%s'.\n", instanceName)
			fmt.Println("👉 Please run 'hydraidectl init' first to create the instance.")
			return
		}
		basepath := instanceData.BasePath

		fmt.Println("Base path for instance found in metadata:", basepath)

		// Generate the service file
		err = sp.GenerateServiceFile(instanceName, basepath)
		if err != nil {
			fmt.Printf("Error generating service file: %v\n", err)
			return
		}
		fmt.Printf("Service file for instance '%s' created successfully.\n", instanceName)

		if !noPrompt {
			fmt.Print("Do you want to enable and start this service now? (y/n): ")
			reader := bufio.NewReader(os.Stdin)
			response, _ := reader.ReadString('\n')
			response = strings.ToLower(strings.TrimSpace(response))
			if response != "y" && response != "yes" {
				fmt.Println("Service setup complete. You can enable and start it manually later.")
				return // Exit cleanly if user says no.
			}
		}

		err = sp.EnableAndStartService(instanceName, basepath)
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
	serviceCmd.MarkFlagRequired("instance")
}
