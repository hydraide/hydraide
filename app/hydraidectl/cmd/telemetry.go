package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancerunner"
	"github.com/spf13/cobra"
)

var telemetryCmd = &cobra.Command{
	Use:   "telemetry",
	Short: "Enable or disable telemetry collection for observe command",
	Long: `Telemetry controls the real-time monitoring data collection on the HydrAIDE server.

When enabled, the server collects:
  - All gRPC call details (method, swamp, duration, status)
  - Error information with stack traces
  - Client connection statistics

This data is required for the 'observe' command to work.

Examples:
  hydraidectl telemetry --instance prod           # Show current status
  hydraidectl telemetry --instance prod --enable  # Enable telemetry
  hydraidectl telemetry --instance prod --disable # Disable telemetry
`,
	Run: runTelemetryCmd,
}

var (
	telemetryInstanceName string
	telemetryEnable       bool
	telemetryDisable      bool
	telemetryJSONFormat   bool
)

func init() {
	rootCmd.AddCommand(telemetryCmd)

	telemetryCmd.Flags().StringVarP(&telemetryInstanceName, "instance", "i", "", "Instance name (required)")
	telemetryCmd.Flags().BoolVar(&telemetryEnable, "enable", false, "Enable telemetry")
	telemetryCmd.Flags().BoolVar(&telemetryDisable, "disable", false, "Disable telemetry")
	telemetryCmd.Flags().BoolVarP(&telemetryJSONFormat, "json", "j", false, "Output as JSON")
	_ = telemetryCmd.MarkFlagRequired("instance")
}

func runTelemetryCmd(cmd *cobra.Command, args []string) {
	fs := filesystem.New()
	store, err := buildmeta.New(fs)
	if err != nil {
		printTelemetryErr("Failed to initialize metadata store", err)
		os.Exit(1)
	}

	instance, err := store.GetInstance(telemetryInstanceName)
	if err != nil {
		printTelemetryErr(fmt.Sprintf("Instance '%s' not found", telemetryInstanceName), err)
		os.Exit(1)
	}

	settingsPath := filepath.Join(instance.BasePath, "settings", "settings.json")

	// Load current settings
	settings, _ := loadEngineSettings(settingsPath)
	if settings == nil {
		settings = &SettingsModelEngine{}
	}

	// Check for conflicting flags
	if telemetryEnable && telemetryDisable {
		fmt.Println("❌ Error: Cannot use --enable and --disable together")
		os.Exit(1)
	}

	// Just show status if no action flag
	if !telemetryEnable && !telemetryDisable {
		outputTelemetryStatus(settings.TelemetryEnabled)
		return
	}

	// Change telemetry setting
	newValue := telemetryEnable
	oldValue := settings.TelemetryEnabled

	if newValue == oldValue {
		action := "enabled"
		if !newValue {
			action = "disabled"
		}
		fmt.Printf("Telemetry is already %s\n", action)
		return
	}

	settings.TelemetryEnabled = newValue
	if err := saveEngineSettings(settingsPath, settings); err != nil {
		printTelemetryErr("Failed to save settings", err)
		os.Exit(1)
	}

	action := "enabled"
	if !newValue {
		action = "disabled"
	}
	fmt.Printf("✅ Telemetry %s\n", action)

	// Ask to restart
	fmt.Print("Restart instance now for changes to take effect? [Y/n]: ")
	var response string
	fmt.Scanln(&response)
	if response == "" || response == "y" || response == "Y" {
		doTelemetryRestart()
	} else {
		fmt.Println("")
		fmt.Println("⚠️  Changes will take effect after restart.")
		fmt.Printf("   Restart manually with: hydraidectl restart --instance %s\n", telemetryInstanceName)
	}
}

func doTelemetryRestart() {
	fmt.Printf("🔄 Restarting instance '%s'...\n", telemetryInstanceName)
	ctx := context.Background()
	runner := instancerunner.NewInstanceController()
	if err := runner.RestartInstance(ctx, telemetryInstanceName); err != nil {
		fmt.Printf("❌ Failed to restart: %v\n", err)
		return
	}
	fmt.Printf("✅ Instance '%s' restarted successfully\n", telemetryInstanceName)
}

func outputTelemetryStatus(enabled bool) {
	status := "disabled"
	if enabled {
		status = "enabled"
	}

	if telemetryJSONFormat {
		result := map[string]interface{}{
			"instance":          telemetryInstanceName,
			"telemetry_enabled": enabled,
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("Instance:  %s\n", telemetryInstanceName)
		fmt.Printf("Telemetry: %s\n", status)
		if !enabled {
			fmt.Println("")
			fmt.Println("Enable with: hydraidectl telemetry --instance " + telemetryInstanceName + " --enable")
		}
	}
}

func printTelemetryErr(msg string, err error) {
	if telemetryJSONFormat {
		result := map[string]string{
			"instance": telemetryInstanceName,
			"status":   "error",
			"message":  msg,
		}
		if err != nil {
			result["error"] = err.Error()
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("❌ Error: %s\n", msg)
		if err != nil {
			fmt.Printf("   %v\n", err)
		}
	}
}
