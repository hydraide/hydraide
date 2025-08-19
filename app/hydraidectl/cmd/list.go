package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/downloader"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/env"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancedetector"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancehealth"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().BoolP("quiet", "q", false, "Return only the instance names, one per line")
	listCmd.Flags().BoolP("json", "j", false, "Return structured output in JSON format")
	listCmd.Flags().Bool("no-health", false, "Skip health information of the instance")
	listCmd.Flags().StringP("output", "o", "", "Output format")
}

// instance struct with Json annotation
type instance struct {
	Name            string `json:"name"`
	ServerPort      string `json:"server_port,omitempty"`
	ServerVersion   string `json:"server_version,omitempty"`
	UpdateAvailable string `json:"update_available,omitempty"`
	Status          string `json:"status"`
	Health          string `json:"health,omitempty"`
	BasePath        string `json:"base_path,omitempty"`
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

		downloaderInterface := downloader.New()
		latestVersion := downloaderInterface.GetLatestVersionWithoutServerPrefix()

		ctx := context.Background()

		buildMeta, err := buildmeta.New(filesystem.New())
		if err != nil {
			fmt.Printf("Error loading metadata: %v\n", err)
			return
		}

		allInstances, err := buildMeta.GetAllInstances()
		if err != nil {
			fmt.Printf("Error retrieving all instances metadata: %v\n", err)
			return
		}

		// Gyűjtsük ki és rendezzük a neveket (case-insensitive névsor)
		names := make([]string, 0, len(allInstances))
		for name := range allInstances {
			names = append(names, name)
		}
		sort.Slice(names, func(i, j int) bool {
			return strings.ToLower(names[i]) < strings.ToLower(names[j])
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		instanceHealthInterface := instancehealth.NewInstanceHealth()
		if err != nil {
			fmt.Printf("Error listing instances: %v\n", err)
			return
		}

		instancesWithHealth := make([]instance, 0, len(allInstances))

		filesystemInterface := filesystem.New()

		// FIGYELEM: innentől névsorrendben iterálunk
		for _, name := range names {
			meta := allInstances[name]

			status, err := detector.GetInstanceStatus(ctx, name)
			if err != nil {
				status = "unknown"
			}

			if meta.Version == "" {
				meta.Version = "unknown"
			}

			envInterface := env.New(filesystemInterface, meta.BasePath)
			loadedEnv, err := envInterface.Load(ctx)
			if err != nil {
				fmt.Printf("Error loading environment for instance '%s': %v\n", name, err)
				continue
			}

			ins := instance{
				Name:          name,
				ServerPort:    loadedEnv.HydrAIDEGRPCPort,
				ServerVersion: meta.Version,
				Status:        status,
				BasePath:      meta.BasePath,
			}

			if latestVersion != "unknown" && meta.Version != latestVersion {
				ins.UpdateAvailable = "yes"
				if !jsonOutput {
					ins.UpdateAvailable = fmt.Sprintf("⚠️ %s", ins.UpdateAvailable)
				}
			} else {
				ins.UpdateAvailable = "no"
			}

			if !noHealth {
				ins.Health = instanceHealthInterface.GetHealthStatus(ctx, name).Status
			}

			instancesWithHealth = append(instancesWithHealth, ins)
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
				outputJSON, err := json.MarshalIndent(instancesWithHealth, "", "  ")
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

			fmt.Printf("Found %d HydrAIDE instances:\n", len(allInstances))
			fmt.Printf("Latest server version: %s\n", latestVersion)

			// Map to detect duplicate instance names.
			instanceMap := make(map[string]int, len(allInstances))
			for name := range allInstances {
				instanceMap[name]++
			}

			const colWidth = 20

			// Print headers.
			headerFormat := fmt.Sprintf("%%-%ds %%-%ds %%-%ds %%-%ds %%-%ds %%-%ds\n", colWidth, colWidth, colWidth, colWidth, colWidth, colWidth)
			if !noHealth {
				headerFormat = fmt.Sprintf("%%-%ds %%-%ds %%-%ds %%-%ds %%-%ds %%-%ds %%-%ds\n", colWidth, colWidth, colWidth, colWidth, colWidth, colWidth, colWidth)
				fmt.Printf(headerFormat, "Name", "Server Port", "Server Version", "Update Available", "Service Status", "Health", "Base Path")
				fmt.Printf("%s\n", strings.Repeat("-", colWidth*8+2))
			} else {
				fmt.Printf(headerFormat, "Name", "Server Port", "Server Version", "Update Available", "Service Status", "Base Path")
				fmt.Printf("%s\n", strings.Repeat("-", colWidth*7+1))
			}

			// Print data rows.
			for _, inst := range instancesWithHealth {

				warning := ""

				if instanceMap[inst.Name] > 1 {
					warning = "[WARNING: Duplicate service detected]"
				}

				if !noHealth {
					fmt.Printf("%-*s %-*s %-*s %-*s %-*s %-*s %-*s %s\n", colWidth, inst.Name, colWidth, inst.ServerPort, colWidth, inst.ServerVersion, colWidth, inst.UpdateAvailable, colWidth, inst.Status, colWidth, inst.Health, colWidth, inst.BasePath, warning)
				} else {
					fmt.Printf("%-*s %-*s %-*s %-*s %-*s %-*s %s\n", colWidth, inst.Name, colWidth, inst.ServerPort, colWidth, inst.ServerVersion, colWidth, inst.UpdateAvailable, colWidth, inst.Status, colWidth, inst.BasePath, warning)
				}

			}
		}
	},
}
