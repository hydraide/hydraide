package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancedetector"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancehealth"
	"github.com/spf13/cobra"
)

// instance struct with Json annotation
type instance struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Health string `json:"health"`
}

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
		noHealth, _ := cmd.Flags().GetBool("no-health")

		if !quiet {
			fmt.Println("Scanning for HydrAIDE instances...")
		}

		// Create a new detector for the current operating system.
		detector, err := instancedetector.NewDetector()
		if err != nil {
			fmt.Printf("Failed to load instances: %v", err)
			return
		}

		ctx := context.Background()
		instancesList, err := detector.ListInstances(ctx)
		if err != nil {
			fmt.Printf("Error listing instances: %v", err)
		}

		if len(instancesList) == 0 {
			if !quiet {
				fmt.Println("No HydrAIDE instances found.")
			}
			return
		}

		var instancesWithHealth []instance
		if !noHealth {
			// Get a slice of instance names for the health checker.
			instanceNames := make([]string, len(instancesList))
			for i, inst := range instancesList {
				instanceNames[i] = inst.Name
			}

			healthMap := getInstancesWithHealth(ctx, instanceNames)
			for _, inst := range instancesList {
				health, ok := healthMap[inst.Name]
				if !ok {
					health = "unknown"
				}
				instancesWithHealth = append(instancesWithHealth, instance{
					Name:   inst.Name,
					Status: inst.Status,
					Health: health,
				})
			}
		} else {
			for _, inst := range instancesList {
				instancesWithHealth = append(instancesWithHealth, instance{
					Name:   inst.Name,
					Status: inst.Status,
					Health: "",
				})
			}
		}

		switch {
		case quiet:
			for _, inst := range instancesWithHealth {
				fmt.Println(inst.Name)
			}
			return
		case jsonOutput || outputFormat == "json":
			if noHealth {
				// skip health
				outputJSON, err := json.MarshalIndent(instancesList, "", "  ")
				if err != nil {
					fmt.Printf("Error generating JSON output: %v", err)
				}
				fmt.Println(string(outputJSON))
			} else {
				outputJSON, err := json.MarshalIndent(instancesWithHealth, "", "  ")
				if err != nil {
					fmt.Printf("Error generating JSON output: %v", err)
				}
				fmt.Println(string(outputJSON))
			}
		default:
			fmt.Printf("Found %d HydrAIDE instances:\n", len(instancesList))

			// Map to detect duplicate instance names.
			instanceMap := make(map[string]int, len(instancesList))
			for _, instance := range instancesList {
				instanceMap[instance.Name]++
			}

			const colWidth = 20

			// Print headers.
			headerFormat := fmt.Sprintf("%%-%ds %%-%ds\n", colWidth, colWidth)
			if !noHealth {
				headerFormat = fmt.Sprintf("%%-%ds %%-%ds %%-%ds\n", colWidth, colWidth, colWidth)
				fmt.Printf(headerFormat, "Name", "Service Status", "Health")
				fmt.Printf("%s\n", strings.Repeat("-", colWidth*3+2))
			} else {
				fmt.Printf(headerFormat, "Name", "Service Status")
				fmt.Printf("%s\n", strings.Repeat("-", colWidth*2+1))
			}

			// Print data rows.
			for _, inst := range instancesWithHealth {
				warning := ""
				if instanceMap[inst.Name] > 1 {
					warning = "[WARNING: Duplicate service detected]"
				}

				if !noHealth {
					fmt.Printf("%-*s %-*s %-*s %s\n", colWidth, inst.Name, colWidth, inst.Status, colWidth, inst.Health, warning)
				} else {
					fmt.Printf("%-*s %-*s %s\n", colWidth, inst.Name, colWidth, inst.Status, warning)
				}
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().BoolP("quiet", "q", false, "Return only the instance names, one per line")
	listCmd.Flags().BoolP("json", "j", false, "Return structured output in JSON format")
	listCmd.Flags().Bool("no-health", false, "Skip health information of the instance")
	listCmd.Flags().StringP("output", "o", "", "Output format")
}

func getInstancesWithHealth(ctx context.Context, instanceNames []string) map[string]string {

	// Get healthstatus of all instances
	healthChecker := instancehealth.NewInstanceHealth()
	healthStatusList := healthChecker.GetListHealthStatus(ctx, instanceNames)

	// flatten instance health to map name -> health
	instanceHealthMap := make(map[string]string)
	for _, instanceHealth := range healthStatusList {
		switch instanceHealth.Status {
		case "healthy":
			instanceHealthMap[instanceHealth.InstanceName] = "healthy"
		case "unhealthy":
			instanceHealthMap[instanceHealth.InstanceName] = "unhealthy"
		default:
			instanceHealthMap[instanceHealth.InstanceName] = "unknown"
		}
	}

	return instanceHealthMap
}
