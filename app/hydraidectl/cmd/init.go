package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/certificate"
	"github.com/spf13/cobra"
)

type CertConfig struct {
	CN  string
	DNS []string
	IP  []string
}

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
	FileSize               int
	HydraidePort           string
	HydraideBasePath       string
	HealthCheckPort        string
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Run the quick install wizard",
	Run: func(cmd *cobra.Command, args []string) {

		reader := bufio.NewReader(os.Stdin)

		fmt.Println("🚀 Starting HydrAIDE install wizard...\n")

		var cert CertConfig
		var envCfg EnvConfig

		// Certificate CN – default = localhost
		fmt.Println("🌐 TLS Certificate Setup")
		fmt.Println("🔖 Common Name (CN) is the main name assigned to the certificate.")
		fmt.Println("It usually identifies your company or internal system.")
		fmt.Print("CN (e.g. yourcompany, api.hydraide.local) (default: hydraide): ")
		cnInput, _ := reader.ReadString('\n')
		cert.CN = strings.TrimSpace(cnInput)
		if cert.CN == "" {
			cert.CN = "hydraide"
		}

		// localhost hozzáadása
		cert.DNS = append(cert.DNS, "localhost")
		cert.IP = append(cert.IP, "127.0.0.1")

		// IP-k:belső s külső címek
		fmt.Println("\n🌐 Add additional IP addresses to the certificate?")
		fmt.Println("By default, '127.0.0.1' is included for localhost access.")
		fmt.Println()
		fmt.Println("Now, list any other IP addresses where clients will access the HydrAIDE server.")
		fmt.Println("For example, if the HydrAIDE container is reachable at 192.168.106.100:4900, include that IP.")
		fmt.Println("These IPs must match the address used in the TLS connection, or it will fail.")
		fmt.Print("Do you want to add other IPs besides 127.0.0.1? (y/n): ")

		ans, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(ans)) == "y" {
			fmt.Print("Enter IPs (comma-separated, e.g. 192.168.1.5,10.0.0.12): ")
			ipInput, _ := reader.ReadString('\n')
			ips := strings.Split(strings.TrimSpace(ipInput), ",")
			for _, ip := range ips {
				ip = strings.TrimSpace(ip)
				if ip != "" {
					cert.IP = append(cert.IP, ip)
				}
			}
		}

		fmt.Println("\n🌐 Will clients connect via a domain name (FQDN)?")
		fmt.Println("This includes public domains (e.g. api.example.com) or internal DNS (e.g. hydraide.lan).")
		fmt.Println("To ensure secure TLS connections, you must list any domains that clients will use.")
		fmt.Print("Add domain names to the certificate? (y/n): ")
		ans, _ = reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(ans)) == "y" {
			fmt.Print("Enter domain names (comma-separated, e.g. api.example.com,hydraide.local): ")
			dnsInput, _ := reader.ReadString('\n')
			domains := strings.Split(strings.TrimSpace(dnsInput), ",")
			for _, d := range domains {
				d = strings.TrimSpace(d)
				if d != "" {
					cert.DNS = append(cert.DNS, d)
				}
			}
		}

		fmt.Println("\n🔌 Port Configuration")
		fmt.Println("This is the external port on your host machine that will map to the HydrAIDE container.")
		fmt.Println("Clients will use this port to communicate with the HydrAIDE server.")
		fmt.Print("Which port should HydrAIDE listen on? (default: 4900): ")
		envCfg.HydraidePort, _ = reader.ReadString('\n')
		envCfg.HydraidePort = strings.TrimSpace(envCfg.HydraidePort)
		if envCfg.HydraidePort == "" {
			envCfg.HydraidePort = "4900"
		}

		fmt.Println("\n📁 Base Path for HydrAIDE")
		fmt.Println("This is the main directory where HydrAIDE will store its core files.")
		fmt.Print("Base path (default: /mnt/hydraide): ")
		envCfg.HydraideBasePath, _ = reader.ReadString('\n')
		envCfg.HydraideBasePath = strings.TrimSpace(envCfg.HydraideBasePath)
		if envCfg.HydraideBasePath == "" {
			envCfg.HydraideBasePath = "/mnt/hydraide"
		}

		fmt.Println("\n📝 Logging Configuration")

		// LOG_LEVEL
		fmt.Println("🔍 Log Level: Controls the amount of detail in system logs")
		fmt.Println("   Options: trace, debug, info, warn, error, fatal, panic")
		fmt.Println("   Recommended: 'info' for production, 'debug' for troubleshooting")
		fmt.Print("Log level [default: info]: ")
		logLevel, _ := reader.ReadString('\n')
		logLevel = strings.TrimSpace(logLevel)
		if logLevel == "" {
			logLevel = "info"
		}
		envCfg.LogLevel = logLevel

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
		fmt.Println("📏 Max Message Size: Maximum size for gRPC messages (bytes)")
		fmt.Println("   Default: 5GB (5368709120) - Adjust for large data transfers")
		fmt.Print("Max message size [default: 5368709120]: ")
		maxSizeInput, _ := reader.ReadString('\n')
		maxSizeInput = strings.TrimSpace(maxSizeInput)
		if maxSizeInput == "" {
			envCfg.GRPCMaxMessageSize = 5368709120
		} else {
			if size, err := strconv.ParseInt(maxSizeInput, 10, 64); err == nil {
				envCfg.GRPCMaxMessageSize = size
			} else {
				fmt.Printf("⚠️ Invalid number, using default 5GB. Error: %v\n", err)
				envCfg.GRPCMaxMessageSize = 5368709120
			}
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
		fmt.Println("   Time in seconds before idle Swamps are automatically closed")
		fmt.Print("Idle timeout [default: 10]: ")
		idleInput, _ := reader.ReadString('\n')
		idleInput = strings.TrimSpace(idleInput)
		if idleInput == "" {
			envCfg.CloseAfterIdle = 10
		} else {
			if idle, err := strconv.Atoi(idleInput); err == nil {
				envCfg.CloseAfterIdle = idle
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
		fmt.Println("   Size in bytes for Swamp storage fragments (default: 8KB)")
		fmt.Print("File fragment size [default: 8192]: ")
		sizeInput, _ := reader.ReadString('\n')
		sizeInput = strings.TrimSpace(sizeInput)
		if sizeInput == "" {
			envCfg.FileSize = 8192
		} else {
			if size, err := strconv.Atoi(sizeInput); err == nil {
				envCfg.FileSize = size
			} else {
				fmt.Printf("⚠️ Invalid number, using default 8192 bytes. Error: %v\n", err)
				envCfg.FileSize = 8192
			}
		}

		// HEALTH CHECK PORT
		fmt.Println("\n❤️‍🩹 Health Check Endpoint")
		fmt.Println("   Separate port for health checks and monitoring")
		fmt.Print("Health check port [default: 4901]: ")
		healthPort, _ := reader.ReadString('\n')
		healthPort = strings.TrimSpace(healthPort)
		if healthPort == "" {
			healthPort = "4901"
		}
		envCfg.HealthCheckPort = healthPort

		// ======================
		// CONFIGURATION SUMMARY
		// ======================
		fmt.Println("\n🔧 Configuration Summary:")
		fmt.Println("=== NETWORK ===")
		fmt.Println("  • CN:         ", cert.CN)
		fmt.Println("  • DNS SANs:   ", strings.Join(cert.DNS, ", "))
		fmt.Println("  • IP SANs:    ", strings.Join(cert.IP, ", "))
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
		fmt.Printf("  • Max Message Size: %.2f GB\n", float64(envCfg.GRPCMaxMessageSize)/1024/1024/1024)
		fmt.Println("  • Error Logging:   ", envCfg.GRPCServerErrorLogging)

		fmt.Println("\n=== STORAGE ===")
		fmt.Println("  • Close After Idle: ", envCfg.CloseAfterIdle, "seconds")
		fmt.Println("  • Write Interval:   ", envCfg.WriteInterval, "seconds")
		fmt.Printf("  • File Fragment Size: %d bytes (%.2f KB)\n",
			envCfg.FileSize, float64(envCfg.FileSize)/1024)

		fmt.Println("\n=== PATHS ===")
		fmt.Println("  • Base Path:  ", envCfg.HydraideBasePath)

		// Confirmation
		fmt.Print("\n✅ Proceed with installation? (y/n): ")
		confirm, _ := reader.ReadString('\n')
		confirm = strings.ToLower(strings.TrimSpace(confirm))
		if confirm != "y" && confirm != "yes" {
			fmt.Println("🚫 Installation cancelled.")
			return
		}

		fmt.Println("\n✅ Starting installation...")

		// todo: start the instance installation process

		// - todo: create the necessary directories

		folders := []string{"certificate", "data", "settings"}
		fmt.Println("📂 Creating application folders...", folders)
		err := utils.CreateFolders(envCfg.HydraideBasePath, folders)
		if err != nil {
			fmt.Println("❌ Error creating application folders:", err)
			return
		}
		// double check if Directory created or not
		if verbose, err := utils.CheckDirectoryExists(envCfg.HydraideBasePath, folders); err != nil {
			fmt.Println("❌ Error checking directories:", err)
			return
		} else {
			fmt.Println(verbose)
		}

		// - todo: generate the TLS certificate
		fmt.Println("🔒 Generating TLS certificate...")
		certGen := certificate.New(cert.CN, cert.DNS, cert.IP)
		if err = certGen.Generate(); err != nil {
			fmt.Println("❌ Error generating TLS certificate:", err)
			return
		}
		fmt.Println("✅ TLS certificate generated successfully.")
		clientCRT, serverCRT, serverKEY := certGen.Files()
		fmt.Println("  • Client CRT: ", clientCRT)
		fmt.Println("  • Server CRT: ", serverCRT)
		fmt.Println("  • Server KEY: ", serverKEY)

		// - todo: copy the server and client TLS certificate to the certificate directory

		fmt.Println("📂 Copying TLS certificates to the certificate directory...")
		fmt.Printf("  • Client CRT: From %s  to  %s \n", clientCRT, filepath.Join(envCfg.HydraideBasePath, "certificate", filepath.Base(clientCRT)))
		utils.MoveFile(clientCRT, filepath.Join(envCfg.HydraideBasePath, "certificate", filepath.Base(clientCRT)))
		fmt.Printf("  • Server CRT: From %s  to  %s \n", serverCRT, filepath.Join(envCfg.HydraideBasePath, "certificate", filepath.Base(serverCRT)))
		utils.MoveFile(serverCRT, filepath.Join(envCfg.HydraideBasePath, "certificate", filepath.Base(serverCRT)))
		fmt.Printf("  • Server KEY: From %s  to  %s \n", serverKEY, filepath.Join(envCfg.HydraideBasePath, "certificate", filepath.Base(serverKEY)))
		utils.MoveFile(serverKEY, filepath.Join(envCfg.HydraideBasePath, "certificate", filepath.Base(serverKEY)))

		fmt.Println("✅ TLS certificates copied successfully.")

		// - todo: create the .env file (based on the .env_sample) to base path and fill in the values
		// ===========================
		// CREATE .ENV FILE
		// ===========================
		currentDir, err := os.Getwd()
		if err != nil {
			fmt.Println("❌ Error getting current directory:", err)
			return
		}

		envPath := filepath.Join(currentDir, ".env")

		// Check if .env exists and warn user
		if _, err := os.Stat(envPath); err == nil {
			fmt.Printf("\n⚠️  Found existing .env file at: %s\n", envPath)

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
				fmt.Println("ℹ️  Keeping existing .env file")
				fmt.Println("✅ Proceeding with installation using existing configuration")
				return
			}

			fmt.Println("🔄 Overwriting existing .env file...")
		}

		// Create or truncate the .env file
		envFile, err := os.Create(envPath) // This automatically clears the file if it exists
		if err != nil {
			fmt.Println("❌ Error creating .env file:", err)
			return
		}
		defer envFile.Close()

		// Write all environment variables
		writer := bufio.NewWriter(envFile)
		writeEnv := func(key, value string) {
			_, _ = writer.WriteString(fmt.Sprintf("%s=%s\n", key, value))
		}

		// Write header comment
		_, _ = writer.WriteString("# HydrAIDE Configuration\n")
		_, _ = writer.WriteString("# Generated automatically - DO NOT EDIT MANUALLY\n\n")

		// Write all configuration values
		writeEnv("LOG_LEVEL", envCfg.LogLevel)
		writeEnv("LOG_TIME_FORMAT", "2006-01-02T15:04:05Z07:00")
		writeEnv("SYSTEM_RESOURCE_LOGGING", strconv.FormatBool(envCfg.SystemResourceLogging))
		writeEnv("GRAYLOG_ENABLED", strconv.FormatBool(envCfg.GraylogEnabled))
		writeEnv("GRAYLOG_SERVER", envCfg.GraylogServer)
		writeEnv("GRAYLOG_SERVICE_NAME", envCfg.GraylogServiceName)
		writeEnv("GRPC_MAX_MESSAGE_SIZE", strconv.FormatInt(envCfg.GRPCMaxMessageSize, 10))
		writeEnv("GRPC_SERVER_ERROR_LOGGING", strconv.FormatBool(envCfg.GRPCServerErrorLogging))
		writeEnv("HYDRAIDE_ROOT_PATH", envCfg.HydraideBasePath)
		writeEnv("HYDRAIDE_SERVER_PORT", envCfg.HydraidePort)
		writeEnv("HYDRAIDE_DEFAULT_CLOSE_AFTER_IDLE", strconv.Itoa(envCfg.CloseAfterIdle))
		writeEnv("HYDRAIDE_DEFAULT_WRITE_INTERVAL", strconv.Itoa(envCfg.WriteInterval))
		writeEnv("HYDRAIDE_DEFAULT_FILE_SIZE", strconv.Itoa(envCfg.FileSize))
		writeEnv("HEALTH_CHECK_PORT", envCfg.HealthCheckPort)

		// Add final newline and flush
		_, _ = writer.WriteString("\n")
		if err := writer.Flush(); err != nil {
			fmt.Println("❌ Error writing to .env file:", err)
			return
		}

		fmt.Println("✅ .env file created/updated successfully at:", envPath)

		// - todo: download the latest binary (or the tagged one) from the github releases
		// - todo: create a service file based on the user's operating system
		// - todo: start the service

	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func defaultInstallPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "HydrAIDE", "bin")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "HydrAIDE", "bin")
	default:
		return filepath.Join(home, ".hydraide", "bin")
	}
}
