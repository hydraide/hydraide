package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all installed HydrAIDE instances",
	Run: func(cmd *cobra.Command, args []string) {

		fmt.Println("Listing hydraide instances...")

		// TODO: List all installed HydrAIDE instances.
		//  To do this, we need to know what instances are currently installed.
		//  Then determine which ones are currently running, and which ones are stopped.
		//  (We'll need a lightweight utility that monitors processes for this.)
		//  It's important that we can also list the *name* of each instance,
		//  because the user will need this name to issue the `start` command.
		//  The `ls` command should simply return the list of instances on the system,
		//  along with their current status (e.g. running, stopped).

	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
