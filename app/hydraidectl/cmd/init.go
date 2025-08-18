package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/certificate"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/downloader"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/validator"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

type EnvConfig struct {
	LogLevel               string
	LogTimeFormat          string
	SystemResourceLogging  bool
	GraylogEnabled         bool
	GraylogServer          string
	GraylogServiceName     string
	GRPCMaxMessageSize     int64
	GRPCServerErrorLogging bool
	CloseAfterIdle         int
	WriteInterval          int
	FileSize               int64
	HydraidePort           string
	HydraideBasePath       string
	HealthCheckPort        string
}

var initCmd = &cobra.Command{

	Use:   "init",
	Short: "Run the quick install wizard to create a new HydrAIDE instance",
	Long: `Launches the interactive quick install wizard for HydrAIDE.
This command guides you through the process of creating a new HydrAIDE instance, including setting its service name, storage location, and configuration.`,

	Run: func(cmd *cobra.Command, args []string) {
		reader := bufio.NewReader(os.Stdin)
		fs := filesystem.New()
		bm, err := buildmeta.New(fs)
		if err != nil {
			fmt.Printf("❌ Failed to initialize metadata store: %v\n", err)
			os.Exit(1)
		}

		var instanceName string
		for {
			fmt.Print("✨ Please provide a unique name for this new instance (e.g., 'prod', 'dev-local'): ")
			nameInput, _ := reader.ReadString('\n')
			instanceName = strings.TrimSpace(nameInput)
			if instanceName == "" {
				fmt.Println("🚫 Instance name cannot be empty.")
				continue
			}

			if _, err := bm.GetInstance(instanceName); err == nil {
				fmt.Printf("🚫 An instance named '%s' already exists. Please choose another name.\n", instanceName)
				continue
			}
			break
		}

		fmt.Printf("\n🚀 Starting HydrAIDE setup for instance: \"%s\"\n\n", instanceName)

		var envCfg EnvConfig

		validator := validator.New()
		ctx := context.Background()

		// start the certificate prompting
		certPrompts := certificate.NewPrompts()
		certPrompts.Start(reader)

		fmt.Println("\n🔌 Port Configuration")
		fmt.Println("This is the port where the HydrAIDE binary server will listen for client connections.")
		fmt.Println("Set the bind port for the HydrAIDE server instance.")

		// Port validation loop for main port
		for {
			fmt.Print("Which port should HydrAIDE listen on? [default: 4900]: ")
			portInput, _ := reader.ReadString('\n')
			portInput = strings.TrimSpace(portInput)

			if portInput == "" {
				envCfg.HydraidePort = "4900"
				break
			}

			validPort, err := validator.ValidatePort(ctx, portInput)
			if err != nil {
				fmt.Printf("❌ Invalid port: %v. Please try again.\n", err)
				continue
			}

			envCfg.HydraidePort = validPort
			break
		}

		fmt.Println("\n📁 Base Path for HydrAIDE")
		fmt.Println("This is the main directory where HydrAIDE will store its core files.")
		defaultBasePath := "/mnt/hydraide"
		if runtime.GOOS == "windows" {
			defaultBasePath = `C:\mnt\hydraide`
		}
		fmt.Printf("Base path (default: %s): ", defaultBasePath)
		envCfg.HydraideBasePath, _ = reader.ReadString('\n')
		envCfg.HydraideBasePath = strings.TrimSpace(envCfg.HydraideBasePath)
		if envCfg.HydraideBasePath == "" {
			envCfg.HydraideBasePath = defaultBasePath
		}

		// LOG_LEVEL
		fmt.Println("\n📝 Logging Configuration")
		fmt.Println("   - Controls the verbosity of system logs.")
		fmt.Println("   - Options: debug | info | warn | error")
		fmt.Println("   - Default: info (recommended for production)")

		// Loglevel validation loop
		for {
			fmt.Print("Choose log level [default: info]: ")
			logLevel, _ := reader.ReadString('\n')

			logLevel, err := validator.ValidateLoglevel(ctx, logLevel)
			if err != nil {
				fmt.Printf("\n❌ Invalid loglevel: %v. Please try again.\n", err)
				continue
			}

			envCfg.LogLevel = logLevel
			break
		}

		// SYSTEM_RESOURCE_LOGGING
		fmt.Println("\n💻 System Resource Monitoring")
		fmt.Println("   Enables periodic logging of CPU, memory, and disk usage")
		fmt.Println("   Useful for performance monitoring but adds log entries")
		fmt.Print("Enable system resource logging? (y/n) [default: n]: ")
		resLogInput, _ := reader.ReadString('\n')
		resLogInput = strings.ToLower(strings.TrimSpace(resLogInput))
		envCfg.SystemResourceLogging = (resLogInput == "y" || resLogInput == "yes")

		// GRAYLOG CONFIGURATION
		fmt.Println("\n📊 Graylog Integration")
		fmt.Print("Enable Graylog centralized logging? (y/n) [default: n]: ")
		graylogInput, _ := reader.ReadString('\n')
		graylogInput = strings.ToLower(strings.TrimSpace(graylogInput))
		envCfg.GraylogEnabled = (graylogInput == "y" || graylogInput == "yes")

		if envCfg.GraylogEnabled {
			fmt.Println("🌐 Graylog Server Address")
			fmt.Println("   Format: host:port (e.g., graylog.example.com:5140)")
			fmt.Print("Graylog server address: ")
			graylogServer, _ := reader.ReadString('\n')
			envCfg.GraylogServer = strings.TrimSpace(graylogServer)

			fmt.Println("\n📛 Graylog Service Identifier")
			fmt.Println("   Unique name for this HydrAIDE instance in Graylog")
			fmt.Print("Service name [default: hydraide-prod]: ")
			serviceName, _ := reader.ReadString('\n')
			serviceName = strings.TrimSpace(serviceName)
			if serviceName == "" {
				serviceName = "hydraide-prod"
			}
			envCfg.GraylogServiceName = serviceName
		}

		// GRPC CONFIGURATION
		fmt.Println("\n📡 gRPC Settings")

		// GRPC_MAX_MESSAGE_SIZE
		fmt.Println("📏 Max Message Size: Maximum size for gRPC messages")
		fmt.Println("   Accepts raw bytes or human-readable format (e.g., 100MB, 1GB)")
		fmt.Println("   Recommended: 100MB-1GB for large transfers")

		// Message size validation loop
		for {
			fmt.Printf("Max message size [default: %s]: ", "10MB")
			maxSizeInput, _ := reader.ReadString('\n')
			maxSizeInput = strings.TrimSpace(maxSizeInput)

			if maxSizeInput == "" {
				maxSizeInput = "10MB"
			}
			size, err := validator.ParseMessageSize(ctx, maxSizeInput)
			if err != nil {
				fmt.Printf("❌ Invalid input: %v. Please try again.\n", err)
				continue
			}
			fmt.Printf("✅ Valid size: %s (%d bytes)\n", validator.FormatSize(ctx, size), size)
			envCfg.GRPCMaxMessageSize = size
			break
		}

		// GRPC_SERVER_ERROR_LOGGING
		fmt.Println("\n⚠️ gRPC Error Logging")
		fmt.Println("   Logs detailed errors from gRPC server operations")
		fmt.Print("Enable gRPC error logging? (y/n) [default: y]: ")
		grpcErrInput, _ := reader.ReadString('\n')
		grpcErrInput = strings.ToLower(strings.TrimSpace(grpcErrInput))
		envCfg.GRPCServerErrorLogging = (grpcErrInput != "n" && grpcErrInput != "no")

		// SWAMP STORAGE SETTINGS
		fmt.Println("\n🏞️ Swamp Storage Configuration")

		// CLOSE_AFTER_IDLE
		fmt.Println("⏱️ Auto-Close Idle Swamps")
		fmt.Println("   Time in seconds before idle Swamps are automatically closed. Between 10 sec and 3600 sec.")
		fmt.Print("Idle timeout [default: 10]: ")
		idleInput, _ := reader.ReadString('\n')
		idleInput = strings.TrimSpace(idleInput)
		if idleInput == "" {
			envCfg.CloseAfterIdle = 10
		} else {
			if idle, err := strconv.Atoi(idleInput); err == nil {

				envCfg.CloseAfterIdle = idle
				if envCfg.CloseAfterIdle < 10 || envCfg.CloseAfterIdle > 3600 {
					fmt.Printf("⚠️ Idle timeout must be between 10 and 3600 seconds. Using default 10s.\n")
					envCfg.CloseAfterIdle = 10
				} else {
					fmt.Printf("✅ Idle timeout set to %d seconds.\n", envCfg.CloseAfterIdle)
				}

			} else {
				fmt.Printf("⚠️ Invalid number, using default 10s. Error: %v\n", err)
				envCfg.CloseAfterIdle = 10
			}
		}

		// WRITE_INTERVAL
		fmt.Println("\n⏱️ Disk Write Frequency")
		fmt.Println("   How often (in seconds) Swamp data is written to disk")
		fmt.Print("Write interval [default: 5]: ")
		writeInput, _ := reader.ReadString('\n')
		writeInput = strings.TrimSpace(writeInput)
		if writeInput == "" {
			envCfg.WriteInterval = 5
		} else {
			if interval, err := strconv.Atoi(writeInput); err == nil {
				envCfg.WriteInterval = interval
			} else {
				fmt.Printf("⚠️ Invalid number, using default 5s. Error: %v\n", err)
				envCfg.WriteInterval = 5
			}
		}

		// FILE_SIZE
		fmt.Println("\n📦 Storage Fragment Size")
		fmt.Println("   Controls the size of storage fragments for Swamp data")
		fmt.Println("   Accepts human-readable format: 8KB, 64KB, 1MB, 512MB, 1GB")
		fmt.Println("   Range: 8KB to 1GB [default: 8KB]")

		// Fragment size validation loop
		for {
			fmt.Printf("Storage fragment size [default: %s]: ", "8KB")
			sizeInput, _ := reader.ReadString('\n')

			validSize, err := validator.ParseFragmentSize(ctx, sizeInput)
			if err != nil {
				fmt.Printf("❌ Invalid fragment size: %v. Please try again.\n", err)
				continue
			}

			envCfg.FileSize = validSize
			break
		}

		// HEALTH CHECK PORT
		fmt.Println("\n❤️‍🩹 Health Check Endpoint")
		fmt.Println("   Separate port for health checks and monitoring")

		// Port validation loop for health check port
		for {
			fmt.Print("Health check port [default: 4901]: ")
			healthPortInput, _ := reader.ReadString('\n')
			healthPortInput = strings.TrimSpace(healthPortInput)

			if healthPortInput == "" {
				envCfg.HealthCheckPort = "4901"
				break
			}

			validPort, err := validator.ValidatePort(ctx, healthPortInput)
			if err != nil {
				fmt.Printf("❌ Invalid port: %v. Please try again.\n", err)
				continue
			}

			if validPort == envCfg.HydraidePort {
				fmt.Println("❌ Health check port cannot be the same as the main port. Please choose a different port.")
				continue
			}

			envCfg.HealthCheckPort = validPort
			break
		}

		// CONFIGURATION SUMMARY
		fmt.Println("\n🔧 Configuration Summary:")
		fmt.Println("=== NETWORK ===")
		fmt.Println("  • CN:         ", certPrompts.GetCN())
		fmt.Println("  • DNS SANs:   ", strings.Join(certPrompts.GetDNS(), ", "))
		fmt.Println("  • IP SANs:    ", strings.Join(certPrompts.GetIP(), ", "))
		fmt.Println("  • Main Port:  ", envCfg.HydraidePort)
		fmt.Println("  • Health Port:", envCfg.HealthCheckPort)

		fmt.Println("\n=== LOGGING ===")
		fmt.Println("  • Log Level:       ", envCfg.LogLevel)
		fmt.Println("  • Resource Logging:", envCfg.SystemResourceLogging)
		fmt.Println("  • Graylog Enabled: ", envCfg.GraylogEnabled)
		if envCfg.GraylogEnabled {
			fmt.Println("      • Server:     ", envCfg.GraylogServer)
			fmt.Println("      • Service:    ", envCfg.GraylogServiceName)
		}

		fmt.Println("\n=== gRPC ===")
		fmt.Printf("  • Max Message Size: %s (%d bytes)\n", validator.FormatSize(ctx, envCfg.GRPCMaxMessageSize), envCfg.GRPCMaxMessageSize)
		fmt.Println("  • Error Logging:   ", envCfg.GRPCServerErrorLogging)

		fmt.Println("\n=== STORAGE ===")
		fmt.Println("  • Close After Idle: ", envCfg.CloseAfterIdle, "seconds")
		fmt.Println("  • Write Interval:   ", envCfg.WriteInterval, "seconds")
		fmt.Printf("  • File Fragment Size: %d bytes (%.2f KB)\n",
			envCfg.FileSize, float64(envCfg.FileSize)/1024)

		fmt.Println("\n=== PATHS ===")
		fmt.Println("  • Base Path:  ", envCfg.HydraideBasePath)

		// Confirmation
		fmt.Print("\n✅ Proceed with installation? (y/n) [default: n]: ")
		confirm, _ := reader.ReadString('\n')
		confirm = strings.ToLower(strings.TrimSpace(confirm))
		if confirm != "y" && confirm != "yes" {
			fmt.Println("🚫 Installation cancelled.")
			return
		}

		fmt.Println("\n✅ Starting installation...")

		// Check and create necessary directories
		folders := []string{"certificate", "data", "settings"}
		fmt.Println("📂 Checking application folders...", folders)

		// Check if all required folders exist
		allExist := true
		var missingFolders []string
		for _, folder := range folders {
			fullPath := filepath.Join(envCfg.HydraideBasePath, folder)
			exists, err := fs.CheckIfDirExists(ctx, fullPath)
			if err != nil {
				fmt.Printf("❌ Error checking directory %s: %v\n", fullPath, err)
				return
			}
			if !exists {
				allExist = false
				missingFolders = append(missingFolders, fullPath)
			}
		}

		// Handle missing folders
		if !allExist {
			fmt.Println("⚠️ The following folders are missing:", missingFolders)
			fmt.Println("🔄 Attempting to create missing folders...")
			for _, folder := range missingFolders {
				if err := fs.CreateDir(ctx, folder, 0755); err != nil {
					fmt.Printf("❌ Error creating directory %s: %v\n", folder, err)
					return
				}
				fmt.Printf("✅ Created directory: %s\n", folder)
			}
		} else {

			// All folders exist, warn about potential data loss
			fmt.Println("\n⚠️ WARNING: All required folders already exist:", strings.Join(folders, ", "))
			fmt.Println("🚨 Continuing may DELETE ALL EXISTING DATA in these folders!")
			fmt.Println("This includes certificates, data, and settings, which could lead to loss of previous configurations.")

			// First confirmation
			fmt.Print("\n❓ Are you sure you want to proceed? This will DELETE all data in these folders. (y/n): ")
			firstConfirm, _ := reader.ReadString('\n')
			firstConfirm = strings.ToLower(strings.TrimSpace(firstConfirm))
			if firstConfirm != "y" && firstConfirm != "yes" {
				fmt.Println("🚫 Installation cancelled due to user choice.")
				return
			}

			// Second confirmation: require typing "delete"
			fmt.Print("\n❓ To confirm, type 'delete' to proceed with data deletion: ")
			secondConfirm, _ := reader.ReadString('\n')
			secondConfirm = strings.ToLower(strings.TrimSpace(secondConfirm))
			if secondConfirm != "delete" {
				fmt.Println("🚫 Installation cancelled. You did not type 'delete'.")
				return
			}

			// Delete existing folders to ensure a clean slate
			for _, folder := range folders {
				fullPath := filepath.Join(envCfg.HydraideBasePath, folder)
				if err := fs.RemoveDir(ctx, fullPath); err != nil {
					fmt.Printf("❌ Error deleting directory %s: %v\n", fullPath, err)
					return
				}
				fmt.Printf("🗑️ Deleted existing directory: %s\n", fullPath)
			}

			// Recreate the folders
			for _, folder := range folders {
				fullPath := filepath.Join(envCfg.HydraideBasePath, folder)
				if err := fs.CreateDir(ctx, fullPath, 0755); err != nil {
					fmt.Printf("❌ Error creating directory %s: %v\n", fullPath, err)
					return
				}
				fmt.Printf("✅ Created directory: %s\n", fullPath)
			}
		}

		// Verify all folders exist after creation
		fmt.Println("\n📂 Verifying application folders...")
		for _, folder := range folders {
			fullPath := filepath.Join(envCfg.HydraideBasePath, folder)
			exists, err := fs.CheckIfDirExists(ctx, fullPath)
			if err != nil {
				fmt.Printf("❌ Error checking directory %s: %v\n", fullPath, err)
				return
			}
			if !exists {
				fmt.Printf("❌ Directory does not exist: %s\n", fullPath)
				return
			}
			fmt.Printf("✅ Directory exists: %s\n", fullPath)
		}

		// Generate the TLS certificate
		fmt.Println("\n🔒 Generating TLS certificate...")
		certGen := certificate.New(certPrompts.GetCN(), certPrompts.GetDNS(), certPrompts.GetIP())
		if err := certGen.Generate(); err != nil {
			fmt.Println("❌ Error generating TLS certificate:", err)
			return
		}
		fmt.Println("✅ TLS certificate generated successfully.")

		var certFiles []string
		caCRT, caKEY, serverCRT, serverKEY, clientCRT, clientKEY := certGen.Files()
		certFiles = []string{caCRT, caKEY, serverCRT, serverKEY, clientCRT, clientKEY}

		fmt.Println("\n📄 TLS Certificate Files:")
		fmt.Println("  • CA CRT:     ", caCRT)
		fmt.Println("  • CA KEY:     ", caKEY)
		fmt.Println("  • Server CRT: ", serverCRT)
		fmt.Println("  • Server KEY: ", serverKEY)
		fmt.Println("  • Client CRT: ", clientCRT)
		fmt.Println("  • Client KEY: ", clientKEY)

		// Copy the server and client TLS certificates to the certificate directory
		fmt.Println("\n📂 Copying TLS certificates to the certificate directory...")

		// move all certFiles to the certificate directory
		for _, file := range certFiles {
			destPath := filepath.Join(envCfg.HydraideBasePath, "certificate", filepath.Base(file))
			fmt.Printf("  • Moving %s to %s\n", file, destPath)
			if err := fs.MoveFile(ctx, file, destPath); err != nil {
				fmt.Println("❌ Error moving certificate file:", err)
				return
			}
			fmt.Printf("✅ Moved %s to %s\n", file, destPath)
		}

		fmt.Println("✅ TLS certificates copied successfully.")

		// Create the .env file
		envPath := filepath.Join(envCfg.HydraideBasePath, ".env")
		exists, err := fs.CheckIfFileExists(ctx, envPath)
		if err != nil {
			fmt.Println("❌ Error checking .env file:", err)
			return
		}
		if exists {
			fmt.Printf("\n⚠️ Found existing .env file at: %s\n", envPath)
			// Show current content
			existingContent, err := os.ReadFile(envPath)
			if err == nil {
				fmt.Println("\n📄 Current .env content:")
				fmt.Println(strings.Repeat("-", 40))
				fmt.Println(string(existingContent))
				fmt.Println(strings.Repeat("-", 40))
			}

			// Confirm overwrite
			fmt.Print("\n❓ Do you want to overwrite this file? (y/n) [default: y]: ")
			overwrite, _ := reader.ReadString('\n')
			overwrite = strings.ToLower(strings.TrimSpace(overwrite))
			if overwrite == "n" || overwrite == "no" {
				fmt.Println("ℹ️ Keeping existing .env file")
				fmt.Println("✅ Proceeding with installation using existing configuration")
				return
			}
			fmt.Println("🔄 Overwriting existing .env file...")
		}

		// Write .env file
		var sb strings.Builder
		sb.WriteString("# HydrAIDE Configuration\n")
		sb.WriteString("# Generated automatically - DO NOT EDIT MANUALLY\n\n")
		sb.WriteString(fmt.Sprintf("LOG_LEVEL=%s\n", envCfg.LogLevel))
		sb.WriteString("LOG_TIME_FORMAT=2006-01-02T15:04:05Z07:00\n")
		sb.WriteString(fmt.Sprintf("SYSTEM_RESOURCE_LOGGING=%t\n", envCfg.SystemResourceLogging))
		sb.WriteString(fmt.Sprintf("GRAYLOG_ENABLED=%t\n", envCfg.GraylogEnabled))
		sb.WriteString(fmt.Sprintf("GRAYLOG_SERVER=%s\n", envCfg.GraylogServer))
		sb.WriteString(fmt.Sprintf("GRAYLOG_SERVICE_NAME=%s\n", envCfg.GraylogServiceName))
		sb.WriteString(fmt.Sprintf("GRPC_MAX_MESSAGE_SIZE=%d\n", envCfg.GRPCMaxMessageSize))
		sb.WriteString(fmt.Sprintf("GRPC_SERVER_ERROR_LOGGING=%t\n", envCfg.GRPCServerErrorLogging))
		sb.WriteString(fmt.Sprintf("HYDRAIDE_ROOT_PATH=%s\n", envCfg.HydraideBasePath))
		sb.WriteString(fmt.Sprintf("HYDRAIDE_SERVER_PORT=%s\n", envCfg.HydraidePort))
		sb.WriteString(fmt.Sprintf("HYDRAIDE_DEFAULT_CLOSE_AFTER_IDLE=%d\n", envCfg.CloseAfterIdle))
		sb.WriteString(fmt.Sprintf("HYDRAIDE_DEFAULT_WRITE_INTERVAL=%d\n", envCfg.WriteInterval))
		sb.WriteString(fmt.Sprintf("HYDRAIDE_DEFAULT_FILE_SIZE=%d\n", envCfg.FileSize))
		sb.WriteString(fmt.Sprintf("HEALTH_CHECK_PORT=%s\n", envCfg.HealthCheckPort))
		sb.WriteString("\n")

		content := []byte(sb.String())
		if err := fs.WriteFile(ctx, envPath, content, 0644); err != nil {
			fmt.Println("❌ Error writing .env file:", err)
			return
		}
		fmt.Println("✅ .env file created/updated successfully at:", envPath)

		// Download the latest binary
		serverDownloaderObject := downloader.New()
		var bar *progressbar.ProgressBar
		progressFn := func(downloaded, total int64, percent float64) {
			if bar == nil {
				bar = progressbar.NewOptions64(total,
					progressbar.OptionSetDescription("Downloading"),
					progressbar.OptionShowBytes(true),
				)
			}
			if err := bar.Set64(downloaded); err != nil {
				fmt.Printf("❌ Error updating progress bar: %v\n", err)
				return
			}
		}

		serverDownloaderObject.SetProgressCallback(progressFn)
		if err := serverDownloaderObject.DownloadHydraServer("latest", envCfg.HydraideBasePath); err != nil {
			fmt.Printf("❌ Failed to download HydrAIDE server binary: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("\n✅ HydrAIDE server binary downloaded successfully.")

		// Save instance metadata
		fmt.Println("\n💾 Saving instance metadata...")
		instanceData := buildmeta.InstanceMetadata{
			BasePath: envCfg.HydraideBasePath,
		}
		if err := bm.SaveInstance(instanceName, instanceData); err != nil {
			fmt.Printf("❌ Failed to save metadata for instance '%s': %v\n", instanceName, err)
			os.Exit(1)
		}

		configPath, _ := bm.GetConfigFilePath()
		fmt.Printf("\n✅ Metadata for instance '%s' saved to %s\n", instanceName, configPath)
		fmt.Println("✅ Installation complete!")
		fmt.Printf("👉 You can now register this instance as a system service by running:\n")
		fmt.Printf("   sudo hydraidectl service --instance %s\n", instanceName)
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
