package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
)

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the HydrAIDE container",
	Run: func(cmd *cobra.Command, args []string) {

		fmt.Println("🔁 Restarting HydrAIDE...")

		// todo: get the instance name

	},
}

func init() {
	rootCmd.AddCommand(restartCmd)
}
