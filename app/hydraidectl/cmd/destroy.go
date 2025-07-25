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

		// TODO: This command allows the user to:
		//  - stop a specific instance by its name
		//  - and then fully destroy that instance along with all its associated data.
		//  This is especially useful for local testing scenarios.

	},
}

func init() {
	rootCmd.AddCommand(destroyCmd)
}
