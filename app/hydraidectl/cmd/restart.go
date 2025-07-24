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
		// Később ide jön majd a docker-compose restart
	},
}

func init() {
	rootCmd.AddCommand(restartCmd)
}
