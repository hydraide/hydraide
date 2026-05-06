package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/certificate"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/downloader"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/elevation"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/env"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancehealth"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/portfinder"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/servicehelper"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/validator"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

var (
	initAdvanced     bool
	initInstanceFlag string
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Install a new HydrAIDE instance end-to-end",
	Long: `Installs a new HydrAIDE instance: gathers minimal configuration, generates
TLS certificates, downloads the latest server binary, registers a systemd
service, starts it, and waits until the instance reports healthy.

By default the wizard asks only for the instance name and base path; everything
else uses production-ready defaults (localhost-only TLS, gRPC on the lowest
free port from 4900, slog logging at info level, V2 storage engine).

Use --advanced to expose every tunable (Graylog, gRPC message size, log level,
custom TLS SANs, etc.). Settings written by either path can be changed later
with 'hydraidectl edit'.`,

	Run: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolVar(&initAdvanced, "advanced", false,
		"Show every configuration prompt instead of using defaults")
	initCmd.Flags().StringVarP(&initInstanceFlag, "instance", "i", "",
		"Instance name (skips the prompt when provided)")
}

func runInit(cmd *cobra.Command, args []string) {
	if !preflightCheck() {
		os.Exit(1)
	}

	reader := bufio.NewReader(os.Stdin)
	fs := filesystem.New()
	ctx := context.Background()

	bm, err := buildmeta.New(fs)
	if err != nil {
		fmt.Printf("❌ Failed to initialize metadata store: %v\n", err)
		os.Exit(1)
	}

	instanceName := promptInstanceName(reader, bm)
	fmt.Printf("\n🚀 Installing HydrAIDE instance: %q\n", instanceName)

	basePath := promptBasePath(reader, instanceName)

	envSettings := &env.Settings{HydrAIDEBasePath: basePath}
	v := validator.New()

	// Ports — auto-pick a free pair, only prompt in --advanced.
	grpcPort, healthPort := pickPorts(ctx, fs, reader, v)
	envSettings.HydrAIDEGRPCPort = strconv.Itoa(grpcPort)
	envSettings.HydrAIDEHealthCheckPort = strconv.Itoa(healthPort)

	// TLS — default localhost-only, prompt only in --advanced.
	certPrompts := certificate.NewPrompts()
	if initAdvanced {
		certPrompts.Start(reader)
	} else {
		certPrompts.SetDefaults()
	}

	// Logging + gRPC — defaults outside --advanced.
	if initAdvanced {
		fillAdvancedSettings(reader, ctx, v, envSettings)
	} else {
		applyDefaultSettings(envSettings)
	}

	printSummary(ctx, v, certPrompts, envSettings)
	if !confirm(reader, "\n✅ Proceed with installation? (y/n) [default: y]: ", true) {
		fmt.Println("🚫 Installation cancelled.")
		return
	}

	if !prepareDirectories(ctx, fs, reader, basePath) {
		return
	}

	if !generateAndPlaceCerts(ctx, fs, certPrompts, basePath) {
		return
	}

	if err := writeSettingsJSON(ctx, fs, basePath); err != nil {
		fmt.Printf("❌ %v\n", err)
		return
	}

	if err := writeEnvFile(ctx, fs, basePath, envSettings); err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}

	version, err := downloadServerBinary(basePath)
	if err != nil {
		fmt.Printf("❌ Failed to download HydrAIDE server binary: %v\n", err)
		os.Exit(1)
	}

	if err := bm.SaveInstance(instanceName, buildmeta.InstanceMetadata{
		BasePath: basePath,
		Version:  version,
	}); err != nil {
		fmt.Printf("❌ Failed to save metadata for instance %q: %v\n", instanceName, err)
		os.Exit(1)
	}

	if err := installAndStartService(instanceName, basePath); err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}

	if err := waitForHealthy(ctx, instanceName); err != nil {
		fmt.Printf("⚠️  Service started but is not yet healthy: %v\n", err)
		fmt.Println("    Check the logs with: journalctl -u hydraserver-" + instanceName + " -n 100")
		os.Exit(1)
	}

	printClientKit(instanceName, basePath, envSettings, certPrompts)
}

// preflightCheck verifies the host is suitable: privilege + systemd presence.
// On non-Linux hosts the systemd check is skipped (macOS/Windows handled by
// servicehelper).
func preflightCheck() bool {
	if !elevation.IsElevated() {
		fmt.Println(elevation.Hint("<instance>"))
		fmt.Println("\n💡 Tip: 'hydraidectl init' installs a system service, so root is required.")
		return false
	}
	if runtime.GOOS == "linux" {
		if _, err := os.Stat("/run/systemd/system"); err != nil {
			fmt.Println("❌ HydrAIDE requires systemd, but no systemd is running on this host.")
			fmt.Println("   If you are inside a container, use the prebuilt Docker image instead.")
			return false
		}
	}
	return true
}

func promptInstanceName(reader *bufio.Reader, bm buildmeta.MetadataStore) string {
	if initInstanceFlag != "" {
		if _, err := bm.GetInstance(initInstanceFlag); err == nil {
			fmt.Printf("❌ An instance named %q already exists on this host.\n", initInstanceFlag)
			os.Exit(1)
		}
		return initInstanceFlag
	}
	for {
		fmt.Print("✨ Instance name (e.g. 'prod', 'dev-local'): ")
		nameInput, _ := reader.ReadString('\n')
		name := strings.TrimSpace(nameInput)
		if name == "" {
			fmt.Println("🚫 Instance name cannot be empty.")
			continue
		}
		if _, err := bm.GetInstance(name); err == nil {
			fmt.Printf("🚫 An instance named %q already exists. Choose another.\n", name)
			continue
		}
		return name
	}
}

func promptBasePath(reader *bufio.Reader, instanceName string) string {
	defaultBase := filepath.Join("/mnt/hydraide", instanceName)
	if runtime.GOOS == "windows" {
		defaultBase = filepath.Join(`C:\mnt\hydraide`, instanceName)
	}
	fmt.Printf("📁 Base path [default: %s]: ", defaultBase)
	in, _ := reader.ReadString('\n')
	in = strings.TrimSpace(in)
	if in == "" {
		return defaultBase
	}
	return in
}

// pickPorts finds the lowest free (gRPC, health) pair starting at 4900/4901,
// avoiding ports already claimed by other registered instances. In --advanced
// the result is offered as the default; the user can override.
func pickPorts(ctx context.Context, fs filesystem.FileSystem, reader *bufio.Reader, v validator.Validator) (int, int) {
	reserved, err := portfinder.ReservedPorts(ctx, fs)
	if err != nil {
		// Best-effort: continue with an empty reservation set; the listening
		// check will still keep us off bound ports.
		reserved = map[int]bool{}
	}
	grpc, health, err := portfinder.FindFreePair(reserved, portfinder.DefaultGRPCPort, portfinder.PortBumpStep, portfinder.MaxAttempts)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		fmt.Println("   Free up a port pair near 4900 or pass --advanced to choose manually.")
		os.Exit(1)
	}

	if !initAdvanced {
		fmt.Printf("🔌 Ports: gRPC=%d, health=%d (auto-selected)\n", grpc, health)
		return grpc, health
	}

	for {
		fmt.Printf("🔌 gRPC port [default: %d]: ", grpc)
		in, _ := reader.ReadString('\n')
		in = strings.TrimSpace(in)
		if in == "" {
			break
		}
		valid, err := v.ValidatePort(ctx, in)
		if err != nil {
			fmt.Printf("❌ Invalid port: %v\n", err)
			continue
		}
		p, _ := strconv.Atoi(valid)
		if !portfinder.IsPortFree(p) || !portfinder.IsPortFree(p+1) {
			fmt.Printf("❌ Port %d or %d is already in use.\n", p, p+1)
			continue
		}
		grpc, health = p, p+1
		break
	}
	fmt.Printf("   Selected: gRPC=%d, health=%d\n", grpc, health)
	return grpc, health
}

// applyDefaultSettings fills in all non-network settings with sane defaults so
// the user is not asked about them.
func applyDefaultSettings(s *env.Settings) {
	s.LogLevel = "info"
	s.SystemResourceLogging = false
	s.GraylogEnabled = false
	s.GRPCMaxMessageSize = validator.DefaultMessageSize
	s.GRPCServerErrorLogging = true
}

// fillAdvancedSettings runs the full prompt set for users who passed --advanced.
// Mirrors the original wizard but skips the network prompts already handled.
func fillAdvancedSettings(reader *bufio.Reader, ctx context.Context, v validator.Validator, s *env.Settings) {
	// Log level
	for {
		fmt.Print("\n📝 Log level (debug|info|warn|error) [default: info]: ")
		in, _ := reader.ReadString('\n')
		level, err := v.ValidateLoglevel(ctx, in)
		if err != nil {
			fmt.Printf("❌ %v\n", err)
			continue
		}
		s.LogLevel = level
		break
	}

	fmt.Print("💻 Enable system resource logging (CPU/mem/disk)? (y/n) [default: n]: ")
	res, _ := reader.ReadString('\n')
	s.SystemResourceLogging = isYes(res)

	fmt.Print("📊 Enable Graylog centralized logging? (y/n) [default: n]: ")
	gr, _ := reader.ReadString('\n')
	s.GraylogEnabled = isYes(gr)
	if s.GraylogEnabled {
		fmt.Print("   Graylog server (host:port): ")
		srv, _ := reader.ReadString('\n')
		s.GraylogServer = strings.TrimSpace(srv)

		fmt.Print("   Graylog service name [default: hydraide-prod]: ")
		name, _ := reader.ReadString('\n')
		name = strings.TrimSpace(name)
		if name == "" {
			name = "hydraide-prod"
		}
		s.GraylogServiceName = name
	}

	for {
		fmt.Print("📏 Max gRPC message size [default: 10MB]: ")
		in, _ := reader.ReadString('\n')
		in = strings.TrimSpace(in)
		if in == "" {
			s.GRPCMaxMessageSize = validator.DefaultMessageSize
			break
		}
		size, err := v.ParseMessageSize(ctx, in)
		if err != nil {
			fmt.Printf("❌ %v\n", err)
			continue
		}
		s.GRPCMaxMessageSize = size
		break
	}

	fmt.Print("⚠️  Log gRPC server errors? (y/n) [default: y]: ")
	gerr, _ := reader.ReadString('\n')
	g := strings.ToLower(strings.TrimSpace(gerr))
	s.GRPCServerErrorLogging = g != "n" && g != "no"
}

func printSummary(ctx context.Context, v validator.Validator, cp certificate.Prompts, s *env.Settings) {
	fmt.Println("\n🔧 Configuration Summary:")
	fmt.Println("=== NETWORK ===")
	fmt.Println("  • CN:         ", cp.GetCN())
	fmt.Println("  • DNS SANs:   ", strings.Join(cp.GetDNS(), ", "))
	fmt.Println("  • IP SANs:    ", strings.Join(cp.GetIP(), ", "))
	fmt.Println("  • gRPC port:  ", s.HydrAIDEGRPCPort)
	fmt.Println("  • Health port:", s.HydrAIDEHealthCheckPort)

	fmt.Println("\n=== LOGGING ===")
	fmt.Println("  • Log level:       ", s.LogLevel)
	fmt.Println("  • Resource logging:", s.SystemResourceLogging)
	fmt.Println("  • Graylog:         ", s.GraylogEnabled)
	if s.GraylogEnabled {
		fmt.Println("      • Server: ", s.GraylogServer)
		fmt.Println("      • Service:", s.GraylogServiceName)
	}

	fmt.Println("\n=== gRPC ===")
	fmt.Printf("  • Max message size: %s\n", v.FormatSize(ctx, s.GRPCMaxMessageSize))
	fmt.Println("  • Error logging:   ", s.GRPCServerErrorLogging)

	fmt.Println("\n=== STORAGE ===")
	fmt.Println("  • Engine:    V2 (append-only, single-file)")
	fmt.Println("  • Base path:", s.HydrAIDEBasePath)
}

func prepareDirectories(ctx context.Context, fs filesystem.FileSystem, reader *bufio.Reader, basePath string) bool {
	folders := []string{"certificate", "data", "settings"}
	allExist := true
	var missing []string
	for _, f := range folders {
		p := filepath.Join(basePath, f)
		exists, err := fs.CheckIfDirExists(ctx, p)
		if err != nil {
			fmt.Printf("❌ Error checking %s: %v\n", p, err)
			return false
		}
		if !exists {
			allExist = false
			missing = append(missing, p)
		}
	}

	if !allExist {
		for _, p := range missing {
			if err := fs.CreateDir(ctx, p, 0755); err != nil {
				fmt.Printf("❌ Error creating %s: %v\n", p, err)
				return false
			}
			fmt.Printf("✅ Created %s\n", p)
		}
		return true
	}

	fmt.Println("\n⚠️  All required folders already exist under", basePath)
	fmt.Println("🚨 Continuing will DELETE existing certificates, data and settings.")
	if !confirm(reader, "❓ Are you sure? (y/n): ", false) {
		fmt.Println("🚫 Installation cancelled.")
		return false
	}
	fmt.Print("❓ Type 'delete' to confirm: ")
	second, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(second)) != "delete" {
		fmt.Println("🚫 Installation cancelled.")
		return false
	}
	for _, f := range folders {
		p := filepath.Join(basePath, f)
		if err := fs.RemoveDir(ctx, p); err != nil {
			fmt.Printf("❌ Error deleting %s: %v\n", p, err)
			return false
		}
		if err := fs.CreateDir(ctx, p, 0755); err != nil {
			fmt.Printf("❌ Error creating %s: %v\n", p, err)
			return false
		}
	}
	fmt.Println("✅ Folders reset.")
	return true
}

func generateAndPlaceCerts(ctx context.Context, fs filesystem.FileSystem, cp certificate.Prompts, basePath string) bool {
	if err := cp.GenerateCert(); err != nil {
		fmt.Printf("❌ Cert generation failed: %v\n", err)
		return false
	}
	for _, file := range cp.GetCertificateFiles() {
		dest := filepath.Join(basePath, "certificate", filepath.Base(file))
		if err := fs.MoveFile(ctx, file, dest); err != nil {
			fmt.Printf("❌ Error moving cert %s: %v\n", file, err)
			return false
		}
	}
	fmt.Println("✅ TLS certificates placed in", filepath.Join(basePath, "certificate"))
	return true
}

func writeSettingsJSON(ctx context.Context, fs filesystem.FileSystem, basePath string) error {
	data, err := json.MarshalIndent(map[string]interface{}{"engine": "V2"}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings.json: %w", err)
	}
	p := filepath.Join(basePath, "settings", "settings.json")
	if err := fs.WriteFile(ctx, p, data, 0644); err != nil {
		return fmt.Errorf("write settings.json: %w", err)
	}
	fmt.Println("✅ Storage engine set to V2")
	return nil
}

func writeEnvFile(ctx context.Context, fs filesystem.FileSystem, basePath string, s *env.Settings) error {
	e := env.New(fs, basePath)
	if err := e.Set(ctx, s); err != nil {
		return fmt.Errorf("write .env: %w", err)
	}
	fmt.Println("✅ .env written to", e.GetEnvPath())
	return nil
}

func downloadServerBinary(basePath string) (string, error) {
	var bar *progressbar.ProgressBar
	progressFn := func(downloaded, total int64, percent float64) {
		if bar == nil {
			bar = progressbar.NewOptions64(total,
				progressbar.OptionSetDescription("Downloading"),
				progressbar.OptionShowBytes(true),
			)
		}
		_ = bar.Set64(downloaded)
	}
	d := downloader.New()
	d.SetProgressCallback(progressFn)
	version, err := d.DownloadHydraServer("latest", basePath)
	if err != nil {
		return "", err
	}
	fmt.Printf("\n✅ HydrAIDE server binary (%s) downloaded\n", version)
	return version, nil
}

func installAndStartService(instanceName, basePath string) error {
	sp := servicehelper.New()
	exists, err := sp.ServiceExists(instanceName)
	if err != nil {
		return fmt.Errorf("checking service: %w", err)
	}
	if exists {
		return fmt.Errorf("a service for instance %q already exists; remove it first", instanceName)
	}
	fmt.Println("\n🛠️  Installing system service...")
	if err := sp.GenerateServiceFile(instanceName, basePath); err != nil {
		return fmt.Errorf("generate service file: %w", err)
	}
	if err := sp.EnableAndStartService(instanceName, basePath); err != nil {
		return fmt.Errorf("enable/start service: %w", err)
	}
	fmt.Printf("✅ Service hydraserver-%s enabled and started\n", instanceName)
	return nil
}

// waitForHealthy polls the instance's health endpoint until it reports healthy
// or the timeout elapses.
func waitForHealthy(ctx context.Context, instanceName string) error {
	fmt.Print("⏳ Waiting for instance to become healthy")
	ih := instancehealth.NewInstanceHealth()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		fmt.Print(".")
		st := ih.GetHealthStatus(ctx, instanceName)
		if st.Status == "healthy" {
			fmt.Println(" ✅")
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	fmt.Println(" ❌")
	return fmt.Errorf("instance %q did not become healthy within 30s", instanceName)
}

func printClientKit(instanceName, basePath string, s *env.Settings, cp certificate.Prompts) {
	certDir := filepath.Join(basePath, "certificate")
	host := "localhost"
	if dns := cp.GetDNS(); len(dns) > 0 && dns[0] != "" {
		host = dns[0]
	}

	fmt.Println("\n🎉 Installation complete!")
	fmt.Println()
	fmt.Println("👉 Client connection kit — copy these three files to your application host:")
	fmt.Println()
	fmt.Printf("   %s\n", filepath.Join(certDir, "ca.crt"))
	fmt.Printf("   %s\n", filepath.Join(certDir, "client.crt"))
	fmt.Printf("   %s\n", filepath.Join(certDir, "client.key"))
	fmt.Println()
	fmt.Printf("   Connect to:   %s:%s\n", host, s.HydrAIDEGRPCPort)
	fmt.Printf("   Health probe: %s:%s/health\n", host, s.HydrAIDEHealthCheckPort)
	fmt.Printf("   ServerName:   %s\n", cp.GetCN())
	fmt.Println()
	fmt.Println("   Go SDK config snippet:")
	fmt.Println()
	fmt.Println("       client := hydraidego.New(&client.Config{")
	fmt.Printf("           ServerHost: %q,\n", fmt.Sprintf("%s:%s", host, s.HydrAIDEGRPCPort))
	fmt.Println("           CACertPath: \"/path/to/ca.crt\",")
	fmt.Println("           ClientCert: \"/path/to/client.crt\",")
	fmt.Println("           ClientKey:  \"/path/to/client.key\",")
	fmt.Printf("           ServerName: %q,\n", cp.GetCN())
	fmt.Println("       })")
	fmt.Println()
	fmt.Printf("Manage this instance with: hydraidectl list | hydraidectl edit -i %s\n", instanceName)
}

func confirm(reader *bufio.Reader, prompt string, defaultYes bool) bool {
	fmt.Print(prompt)
	in, _ := reader.ReadString('\n')
	in = strings.ToLower(strings.TrimSpace(in))
	if in == "" {
		return defaultYes
	}
	return in == "y" || in == "yes"
}

func isYes(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "y" || s == "yes"
}
