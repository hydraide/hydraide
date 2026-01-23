package cmd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/observe"
	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancerunner"
	hydrapb "github.com/hydraide/hydraide/generated/hydraidepbgo"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var observeCmd = &cobra.Command{
	Use:   "observe",
	Short: "Real-time monitoring dashboard for HydrAIDE",
	Long: `Observe provides a real-time TUI dashboard for monitoring all gRPC calls,
errors, and client activity on a HydrAIDE server.

Features:
  - Live stream of all gRPC calls with timing and status
  - Error details with stack traces
  - Statistics and metrics
  - Pause/resume and filtering

The observe command requires telemetry to be enabled on the server.
If not enabled, it will prompt you to enable it and restart the instance.

Examples:
  hydraidectl observe --instance prod
  hydraidectl observe --instance prod --errors-only
  hydraidectl observe --instance prod --simple --stats
`,
	Run: runObserve,
}

var (
	observeInstanceName string
	observeErrorsOnly   bool
	observeFilter       string
	observeSimpleMode   bool
	observeStatsOnly    bool
)

func init() {
	rootCmd.AddCommand(observeCmd)

	observeCmd.Flags().StringVarP(&observeInstanceName, "instance", "i", "", "Instance name (required)")
	observeCmd.Flags().BoolVar(&observeErrorsOnly, "errors-only", false, "Only show error events")
	observeCmd.Flags().StringVar(&observeFilter, "filter", "", "Filter by swamp pattern (e.g., 'auth/*')")
	observeCmd.Flags().BoolVar(&observeSimpleMode, "simple", false, "Simple text output instead of TUI")
	observeCmd.Flags().BoolVar(&observeStatsOnly, "stats", false, "Show statistics only")
	_ = observeCmd.MarkFlagRequired("instance")
}

func runObserve(cmd *cobra.Command, args []string) {
	// Get instance information
	fs := filesystem.New()
	store, err := buildmeta.New(fs)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}

	instance, err := store.GetInstance(observeInstanceName)
	if err != nil {
		fmt.Printf("❌ Error: Instance '%s' not found: %v\n", observeInstanceName, err)
		os.Exit(1)
	}

	// Get paths from instance
	settingsPath := filepath.Join(instance.BasePath, "settings", "settings.json")
	certsPath := filepath.Join(instance.BasePath, "certificate")

	// Check if telemetry is enabled
	settings, _ := loadEngineSettings(settingsPath)
	if settings == nil {
		settings = &SettingsModelEngine{}
	}

	if !settings.TelemetryEnabled {
		fmt.Println("⚠️  Telemetry is not enabled on this instance.")
		fmt.Println("")
		fmt.Println("To use observe, telemetry must be enabled and the instance must be restarted.")
		fmt.Print("Enable telemetry and restart now? [y/N]: ")

		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Observe cancelled. Enable telemetry manually with:")
			fmt.Printf("  hydraidectl telemetry --instance %s --enable\n", observeInstanceName)
			os.Exit(0)
		}

		// Enable telemetry
		settings.TelemetryEnabled = true
		if err := saveEngineSettings(settingsPath, settings); err != nil {
			fmt.Printf("❌ Error saving settings: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Telemetry enabled")

		// Restart instance
		fmt.Printf("🔄 Restarting instance '%s'...\n", observeInstanceName)
		ctx := context.Background()
		runner := instancerunner.NewInstanceController()
		if err := runner.RestartInstance(ctx, observeInstanceName); err != nil {
			fmt.Printf("❌ Error restarting instance: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Instance restarted")
		fmt.Println("")

		// Wait a bit for the server to be ready
		fmt.Println("⏳ Waiting for server to be ready...")
		time.Sleep(3 * time.Second)
	}

	// Build connection parameters from instance
	// Read port from .env file
	envPath := filepath.Join(instance.BasePath, ".env")
	port := "5554" // default
	if envData, err := os.ReadFile(envPath); err == nil {
		for _, line := range strings.Split(string(envData), "\n") {
			if strings.HasPrefix(line, "HYDRAIDE_SERVER_PORT=") {
				port = strings.TrimPrefix(line, "HYDRAIDE_SERVER_PORT=")
				port = strings.TrimSpace(port)
				break
			}
		}
	}

	serverAddr := fmt.Sprintf("localhost:%s", port)
	certFile := filepath.Join(certsPath, "client.crt")
	keyFile := filepath.Join(certsPath, "client.key")
	caFile := filepath.Join(certsPath, "ca.crt")

	// Check if cert files exist
	for _, f := range []string{certFile, keyFile, caFile} {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			fmt.Printf("❌ Error: Certificate file not found: %s\n", f)
			os.Exit(1)
		}
	}

	if observeSimpleMode || observeStatsOnly {
		runSimpleObserve(serverAddr, certFile, keyFile, caFile)
		return
	}

	model := observe.NewModel(serverAddr, certFile, keyFile, caFile)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running observe: %v\n", err)
		os.Exit(1)
	}
}

func runSimpleObserve(serverAddr, certFile, keyFile, caFile string) {
	// Load client certificate (mTLS)
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		fmt.Printf("Error loading client certificate: %v\n", err)
		os.Exit(1)
	}

	// Load CA certificate
	caCert, err := os.ReadFile(caFile)
	if err != nil {
		fmt.Printf("Error reading CA certificate: %v\n", err)
		os.Exit(1)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		fmt.Println("Error: Failed to parse CA certificate")
		os.Exit(1)
	}

	// Extract hostname for SNI
	hostOnly := strings.Split(serverAddr, ":")[0]

	// Create TLS config with client cert, CA cert, and SNI
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		ServerName:   hostOnly,
		MinVersion:   tls.VersionTLS13,
	}

	creds := credentials.NewTLS(tlsConfig)

	// Use grpc.NewClient instead of deprecated grpc.Dial
	conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		fmt.Printf("Error connecting to server: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	client := hydrapb.NewHydraideServiceClient(conn)

	if observeStatsOnly {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		stats, err := client.GetTelemetryStats(ctx, &hydrapb.TelemetryStatsRequest{
			WindowMinutes: 5,
		})
		if err != nil {
			fmt.Printf("Error getting stats: %v\n", err)
			os.Exit(1)
		}

		printObserveStats(stats)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := client.SubscribeToTelemetry(ctx, &hydrapb.TelemetrySubscribeRequest{
		ErrorsOnly:         observeErrorsOnly,
		FilterSwampPattern: observeFilter,
		IncludeSuccesses:   !observeErrorsOnly,
	})
	if err != nil {
		fmt.Printf("Error subscribing to telemetry: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("HydrAIDE Observe - Simple Mode")
	fmt.Println("==============================")
	fmt.Println("Streaming events... (Press Ctrl+C to stop)")
	fmt.Println()

	for {
		event, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("Error receiving event: %v\n", err)
			break
		}

		printObserveEvent(event)
	}
}

func printObserveStats(stats *hydrapb.TelemetryStatsResponse) {
	fmt.Println("HydrAIDE Statistics (last 5 minutes)")
	fmt.Println("====================================")
	fmt.Printf("Total Calls:    %d\n", stats.TotalCalls)
	fmt.Printf("Errors:         %d\n", stats.ErrorCount)
	fmt.Printf("Error Rate:     %.2f%%\n", stats.ErrorRate)
	fmt.Printf("Avg Duration:   %.2fms\n", stats.AvgDurationMs)
	fmt.Printf("Active Clients: %d\n", stats.ActiveClients)

	if len(stats.TopSwamps) > 0 {
		fmt.Println("\nTop Swamps:")
		for i, s := range stats.TopSwamps {
			fmt.Printf("  %d. %s (%d calls, %d errors)\n",
				i+1, s.SwampName, s.CallCount, s.ErrorCount)
		}
	}

	if len(stats.TopErrors) > 0 {
		fmt.Println("\nTop Errors:")
		for i, e := range stats.TopErrors {
			fmt.Printf("  %d. [%dx] %s: %s\n",
				i+1, e.Count, e.ErrorCode, e.ErrorMessage)
		}
	}
}

func printObserveEvent(event *hydrapb.TelemetryEvent) {
	timestamp := event.Timestamp.AsTime().Format("15:04:05.000")

	status := "OK"
	if !event.Success {
		status = fmt.Sprintf("ERR %s", event.ErrorCode)
	}

	swampName := event.SwampName
	if len(swampName) > 40 {
		swampName = swampName[:37] + "..."
	}

	fmt.Printf("%s | %-8s | %-40s | %4dms | %s\n",
		timestamp,
		event.Method,
		swampName,
		event.DurationMs,
		status)

	if !event.Success && event.ErrorMessage != "" {
		fmt.Printf("         +-- %s\n", event.ErrorMessage)
	}
}
