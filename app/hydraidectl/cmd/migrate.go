package cmd

import (
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate HydrAIDE data between storage format versions",
	Long: `
💠 HydrAIDE Data Migration

Migrate swamp data between storage format versions.

SUBCOMMANDS:
  v1-to-v2    Migrate V1 (multi-file chunks) to V2 (single-file append-only)
  v2-to-v3    Upgrade V2 files to V3 format (swamp name in header, faster scanning)

EXAMPLES:
  hydraidectl migrate v1-to-v2 --instance prod --full
  hydraidectl migrate v2-to-v3 --instance prod --restart
  hydraidectl migrate v2-to-v3 --instance prod --dry-run
`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(migrateCmd)
}
