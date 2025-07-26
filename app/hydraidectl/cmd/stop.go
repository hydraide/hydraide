package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the hydrAIDE instance",
	Run: func(cmd *cobra.Command, args []string) {

		fmt.Println("⚠️ This will stop and remove your HydrAIDE setup.")

		// TODO: This Stop command is responsible for stopping a running HydrAIDE instance by its name.
		//  (IMPORTANT: This command must *not* delete the instance — it should only stop it if it is currently running.)
		//  At the end of the command, report whether the instance was running and whether it was successfully stopped.
		//  The process should be terminated using SIGTERM.
		//
		//  todo: IMPORTANT!! HydrAIDE performs a graceful shutdown when it receives a SIGTERM signal,
		//   which means we must wait in the background until the shutdown actually completes.
		//   Monitor the process and only notify the user once the process has fully terminated.
		//
		//  todo: During the shutdown, it's highly recommended to periodically inform the user
		//   that the shutdown is still in progress — and **strongly advise** them not to shut down the server/PC
		//   until the operation has finished, in order to avoid data loss.

	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
