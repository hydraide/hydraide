package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancedetector"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all installed HydrAIDE instances and their status",
	Long: `Detects and lists all HydrAIDE instances registered as OS-level services.
		The command scans for services named 'hydraserver-<instance-name>' and reports
		their current status on the system.`,

	Run: func(cmd *cobra.Command, args []string) {
		quiet, _ := cmd.Flags().GetBool("quiet")
		jsonOutput, _ := cmd.Flags().GetBool("json")
		outputFormat, _ := cmd.Flags().GetString("output")

		if !quiet {
			fmt.Println("Scanning for HydrAIDE instances...")
		}

		// Create a new detector for the current operating system.
		detector, err := instancedetector.NewDetector()
		if err != nil {
			fmt.Printf("Failed to load instances: %v", err)
			return
		}

		instances, err := detector.ListInstances(context.Background())
		if err != nil {
			fmt.Printf("Error listing instances: %v", err)
		}

		if len(instances) == 0 {
			if !quiet {
				fmt.Println("No HydrAIDE instances found.")
			}
			return
		}

		if jsonOutput || outputFormat == "json" {
			outputJSON, err := json.MarshalIndent(instances, "", "  ")
			if err != nil {
				fmt.Printf("Error generating JSON output: %v", err)
			}
			fmt.Println(string(outputJSON))
			return
		}

		if quiet {
			for _, instance := range instances {
				fmt.Println(instance.Name)
			}
			return
		}

		fmt.Printf("Found %d HydrAIDE instances:\n", len(instances))

		// Map to detect duplicate instance names.
		instanceMap := make(map[string]int, len(instances))
		for _, instance := range instances {
			instanceMap[instance.Name]++
		}

		for _, instance := range instances {
			if instanceMap[instance.Name] > 1 {
				fmt.Printf("- %-15s (%s)    [WARNING: Duplicate service detected]\n", instance.Name, instance.Status)
			} else {
				fmt.Printf("- %-15s (%s)\n", instance.Name, instance.Status)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().BoolP("quiet", "q", false, "Return only the instance names, one per line")
	listCmd.Flags().BoolP("json", "j", false, "Return structured output in JSON format")
	listCmd.Flags().StringP("output", "o", "", "Output format")
}
