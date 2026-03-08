package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/explore"
	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/spf13/cobra"
)

var exploreCmd = &cobra.Command{
	Use:   "explore",
	Short: "Interactive swamp hierarchy explorer (Sanctuary / Realm / Swamp)",
	Long: `
  HydrAIDE Swamp Explorer

Interactive TUI for browsing the Sanctuary / Realm / Swamp hierarchy.
Navigate with arrow keys, Enter to drill down, Esc to go back.

When used with --instance on a running server, you can also delete
Sanctuaries, Realms, or individual Swamps by pressing 'd'. Deletion
requires double confirmation and uses the server's DestroyBulk API.

USAGE:
  # Direct filesystem mode (no running server needed, read-only):
  hydraidectl explore --data-path /var/hydraide/data

  # Instance mode (reads data path from instance config, supports deletion):
  hydraidectl explore --instance prod
`,
	Run: runExploreCmd,
}

var (
	exploreDataPath string
	exploreInstance string
)

func init() {
	rootCmd.AddCommand(exploreCmd)

	exploreCmd.Flags().StringVarP(&exploreDataPath, "data-path", "d", "", "Direct path to data directory")
	exploreCmd.Flags().StringVarP(&exploreInstance, "instance", "i", "", "Instance name")
}

func runExploreCmd(cmd *cobra.Command, args []string) {
	dataPath, basePath := resolveExploreDataPath()
	if dataPath == "" {
		fmt.Println("Error: provide --data-path or --instance")
		os.Exit(1)
	}

	model := explore.NewModel(dataPath, exploreInstance, basePath)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running explorer: %v\n", err)
		os.Exit(1)
	}
}

func resolveExploreDataPath() (dataPath string, basePath string) {
	if exploreDataPath != "" {
		return exploreDataPath, ""
	}
	if exploreInstance != "" {
		fs := filesystem.New()
		store, err := buildmeta.New(fs)
		if err != nil {
			fmt.Printf("  Error: %v\n", err)
			os.Exit(1)
		}
		instance, err := store.GetInstance(exploreInstance)
		if err != nil {
			fmt.Printf("  Error: instance '%s' not found: %v\n", exploreInstance, err)
			os.Exit(1)
		}
		return filepath.Join(instance.BasePath, "data"), instance.BasePath
	}
	return "", ""
}

// resolveServerAddr reads the server port from the instance .env file.
func resolveServerAddr(basePath string) string {
	envPath := filepath.Join(basePath, ".env")
	port := "5554"
	if envData, err := os.ReadFile(envPath); err == nil {
		for _, line := range strings.Split(string(envData), "\n") {
			if strings.HasPrefix(line, "HYDRAIDE_SERVER_PORT=") {
				port = strings.TrimPrefix(line, "HYDRAIDE_SERVER_PORT=")
				port = strings.TrimSpace(port)
				break
			}
		}
	}
	return fmt.Sprintf("localhost:%s", port)
}
