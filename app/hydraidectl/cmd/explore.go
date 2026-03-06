package cmd

import (
	"fmt"
	"os"
	"path/filepath"

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

USAGE:
  # Direct filesystem mode (no running server needed):
  hydraidectl explore --data-path /var/hydraide/data

  # Instance mode (reads data path from instance config):
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
	dataPath := resolveExploreDataPath()
	if dataPath == "" {
		fmt.Println("Error: provide --data-path or --instance")
		os.Exit(1)
	}

	model := explore.NewModel(dataPath)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running explorer: %v\n", err)
		os.Exit(1)
	}
}

func resolveExploreDataPath() string {
	if exploreDataPath != "" {
		return exploreDataPath
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
		return filepath.Join(instance.BasePath, "data")
	}
	return ""
}
