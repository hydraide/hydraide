package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
)

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Stop and remove HydrAIDE completely",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("⚠️ This will stop and remove your HydrAIDE setup.")
		// TODO: Confirm & delete containers + volumes
	},
}

func init() {
	rootCmd.AddCommand(destroyCmd)
}
