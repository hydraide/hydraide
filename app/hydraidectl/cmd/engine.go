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

type EngineVersionType string

const (
	EngineVersionV1 EngineVersionType = "V1"
	EngineVersionV2 EngineVersionType = "V2"
)

type SettingsModelEngine struct {
	Engine           EngineVersionType      `json:"engine,omitempty"`
	TelemetryEnabled bool                   `json:"telemetry_enabled,omitempty"`
	Patterns         map[string]interface{} `json:"patterns,omitempty"`
}

var engineCmd = &cobra.Command{
	Use:   "engine",
	Short: "View or change the HydrAIDE storage engine version",
	Run:   runEngineCmd,
}
var (
	engineInstanceName string
	engineSetValue     string
	engineJSONFormat   bool
)

func init() {
	rootCmd.AddCommand(engineCmd)
	engineCmd.Flags().StringVarP(&engineInstanceName, "instance", "i", "", "Instance name (required)")
	engineCmd.Flags().StringVar(&engineSetValue, "set", "", "Set engine version (V1 or V2)")
	engineCmd.Flags().BoolVarP(&engineJSONFormat, "json", "j", false, "Output as JSON")
	_ = engineCmd.MarkFlagRequired("instance")
}
func runEngineCmd(cmd *cobra.Command, args []string) {
	fs := filesystem.New()
	store, err := buildmeta.New(fs)
	if err != nil {
		printEngineErr("Failed to initialize metadata store", err)
		os.Exit(1)
	}
	instance, err := store.GetInstance(engineInstanceName)
	if err != nil {
		printEngineErr(fmt.Sprintf("Instance '%s' not found", engineInstanceName), err)
		os.Exit(1)
	}
	settingsPath := filepath.Join(instance.BasePath, "settings", "settings.json")
	if engineSetValue != "" {
		changeEngineVersion(settingsPath)
		return
	}
	displayCurrentEngine(settingsPath)
}
func displayCurrentEngine(settingsPath string) {
	settings, err := loadEngineSettings(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			outputEngine("V1", "Default (no settings.json)")
			return
		}
		printEngineErr("Failed to load settings", err)
		os.Exit(1)
	}
	engine := settings.Engine
	if engine == "" {
		engine = EngineVersionV1
	}
	outputEngine(string(engine), "")
}
func changeEngineVersion(settingsPath string) {
	newEngine := EngineVersionType(engineSetValue)
	if newEngine != EngineVersionV1 && newEngine != EngineVersionV2 {
		printEngineErr(fmt.Sprintf("Invalid engine version: %s", engineSetValue), fmt.Errorf("must be V1 or V2"))
		os.Exit(1)
	}
	settings, err := loadEngineSettings(settingsPath)
	if err != nil && !os.IsNotExist(err) {
		printEngineErr("Failed to load settings", err)
		os.Exit(1)
	}
	if settings == nil {
		settings = &SettingsModelEngine{}
	}
	oldEngine := settings.Engine
	if oldEngine == "" {
		oldEngine = EngineVersionV1
	}
	if oldEngine == newEngine {
		fmt.Printf("Engine is already set to %s\n", newEngine)
		return
	}
	if newEngine == EngineVersionV2 {
		fmt.Println("\nWARNING: Switching to V2 engine")
		fmt.Println("Ensure you have migrated ALL data first!")
		fmt.Print("Continue? [y/N]: ")
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Aborted.")
			return
		}
	}
	settings.Engine = newEngine
	if err := saveEngineSettings(settingsPath, settings); err != nil {
		printEngineErr("Failed to save settings", err)
		os.Exit(1)
	}
	fmt.Printf("Engine changed from %s to %s\n", oldEngine, newEngine)
	fmt.Print("Restart instance now? [Y/n]: ")
	var response string
	fmt.Scanln(&response)
	if response == "" || response == "y" || response == "Y" {
		doEngineRestart()
	}
}
func doEngineRestart() {
	fmt.Printf("Restarting instance '%s'...\n", engineInstanceName)
	ctx := context.Background()
	runner := instancerunner.NewInstanceController()
	if err := runner.RestartInstance(ctx, engineInstanceName); err != nil {
		fmt.Printf("Failed to restart: %v\n", err)
		return
	}
	fmt.Printf("Instance '%s' restarted successfully\n", engineInstanceName)
}
func loadEngineSettings(path string) (*SettingsModelEngine, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var settings SettingsModelEngine
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("failed to parse settings.json: %w", err)
	}
	return &settings, nil
}
func saveEngineSettings(path string, settings *SettingsModelEngine) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create settings directory: %w", err)
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write settings.json: %w", err)
	}
	return nil
}
func outputEngine(engine string, note string) {
	if engineJSONFormat {
		result := map[string]string{"instance": engineInstanceName, "engine": engine}
		if note != "" {
			result["note"] = note
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("Instance: %s\n", engineInstanceName)
		fmt.Printf("Engine:   %s\n", engine)
		if note != "" {
			fmt.Printf("Note:     %s\n", note)
		}
	}
}
func printEngineErr(msg string, err error) {
	if engineJSONFormat {
		result := map[string]string{"instance": engineInstanceName, "status": "error", "message": msg}
		if err != nil {
			result["error"] = err.Error()
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("Error: %s\n", msg)
		if err != nil {
			fmt.Printf("  %v\n", err)
		}
	}
}
