package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "hydraidectl",
	Short: "HydrAIDE Control CLI",
	Long: `
💠 HydrAIDE Control CLI

Welcome to hydraidectl – your tool to install, restart, destroy and inspect your HydrAIDE system.

📚 Full documentation for hydraidectl is available here: https://github.com/hydraide/hydraide/tree/main/docs/hydraidectl/hydraidectl-user-manual.md

Usage:
  hydraidectl <command>

Try:
  hydraidectl init
  hydraidectl start
  hydraidectl restart
  hydraidectl stop
  hydraidectl destroy
  hydraidectl list
`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println("❌ Error:", err)
		os.Exit(1)
	}
}
