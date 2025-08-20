package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancehealth"
	"github.com/spf13/cobra"
)

var healthInstance string

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Health of HydrAIDE instance",
	Long: `The 'health' command connects to a specified HydrAIDE service instance
and performs a basic health check. The command returns an exit code that can be
used in scripts or automated pipelines to determine the instance's status.

Exit codes:
0 - The instance is healthy.
1 - The instance is unhealthy.
3 - An unexpected status was returned.`,

	Run: func(cmd *cobra.Command, args []string) {
		healthChecker := instancehealth.NewInstanceHealth()
		statusResponse := healthChecker.GetHealthStatus(context.Background(), healthInstance)

		switch statusResponse.Status {
		case "healthy":
			fmt.Println("healthy")
			os.Exit(0)
		case "unhealthy":
			fmt.Println("unhealthy")
			os.Exit(1)
		default:
			os.Exit(3)
		}
	},
}

func init() {
	rootCmd.AddCommand(healthCmd)

	healthCmd.Flags().StringVarP(&healthInstance, "instance", "i", "", "Name of the service instance")
	healthCmd.MarkFlagRequired("instance")
}
