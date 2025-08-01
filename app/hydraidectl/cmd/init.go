package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/certificate"
	"github.com/spf13/cobra"
)

// validatePort validates that the provided port string is a valid integer between 1 and 65535
func validatePort(portStr string) (string, error) {
	if portStr == "" {
		return "", fmt.Errorf("port cannot be empty")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", fmt.Errorf("port must be a valid integer")
	}
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("port must be between 1 and 65535")
	}
	return portStr, nil
}

// validateLoglevel validates whether provided loglevel fits slog loglevels and returns a valid string
func validateLoglevel(logLevel string) (string, error) {
	logLevel = strings.ToLower(strings.TrimSpace(logLevel))
	validLoglevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}

	if logLevel == "" {
		return "info", nil
	}
	if validLoglevels[logLevel] {
		return logLevel, nil
	}
	return "", fmt.Errorf("loglevel must be 'debug', 'info', 'warn' or 'error'")
}

// Message size constants
const (
	Byte = 1
	KB   = 1024 * Byte
	MB   = 1024 * KB
	GB   = 1024 * MB
)

const (
	MinMessageSize     = 10 * MB // 10MB
	MaxMessageSize     = 10 * GB // 10GB
	DefaultMessageSize = 10 * MB // 10MB (changed from 5GB)
)

// parseMessageSize parses human-readable message size input and returns bytes
func parseMessageSize(input string) (int64, error) {
	input = strings.TrimSpace(input)

	// Handle empty input (default)
	if input == "" {
		return DefaultMessageSize, nil
	}

	// Try parsing as raw bytes first
	if val, err := strconv.ParseInt(input, 10, 64); err == nil {
		return validateMessageSize(val)
	}

	// Parse size with unit (case-insensitive)
	input = strings.ToUpper(input)

	// Check for multiple decimal points
	if strings.Count(input, ".") > 1 {
		return 0, fmt.Errorf("invalid format: multiple decimal points not allowed")
	}

	// Extract number and unit using more robust parsing
	var numStr strings.Builder
	var unit string

	for i, r := range input {
		if (r >= '0' && r <= '9') || r == '.' {
			numStr.WriteRune(r)
		} else {
			unit = input[i:]
			break
		}
	}

	numStrFinal := numStr.String()
	if numStrFinal == "" {
		return 0, fmt.Errorf("invalid format: use raw bytes (e.g., 10485760) or size with unit (e.g., 100MB, 1GB)")
	}

	// Parse the numeric part
	num, err := strconv.ParseFloat(numStrFinal, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number: %s", numStrFinal)
	}

	if num < 0 {
		return 0, fmt.Errorf("size cannot be negative")
	}

	// Convert based on unit
	var multiplier int64
	switch unit {
	case "", "B":
		multiplier = Byte
	case "KB":
		multiplier = KB
	case "MB":
		multiplier = MB
	case "GB":
		multiplier = GB
	default:
		return 0, fmt.Errorf("unsupported unit '%s': supported units are B, KB, MB, GB", unit)
	}

	// Calculate total bytes with proper rounding to avoid floating-point precision issues
	totalBytes := int64(num*float64(multiplier) + 0.5)

	return validateMessageSize(totalBytes)
}

// validateMessageSize validates that the size is within acceptable bounds
func validateMessageSize(size int64) (int64, error) {
	if size < MinMessageSize {
		return 0, fmt.Errorf("size too small: minimum is %s (%d bytes)", formatSize(MinMessageSize), MinMessageSize)
	}
	if size > MaxMessageSize {
		return 0, fmt.Errorf("size too large: maximum is %s (%d bytes)", formatSize(MaxMessageSize), MaxMessageSize)
	}
	return size, nil
}

// formatSize converts bytes to human-readable format
func formatSize(bytes int64) string {
	if bytes >= GB {
		return fmt.Sprintf("%.1fGB", float64(bytes)/float64(GB))
	}
	if bytes >= MB {
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(MB))
	}
	if bytes >= KB {
		return fmt.Sprintf("%.1fKB", float64(bytes)/float64(KB))
	}
	return fmt.Sprintf("%dB", bytes)
}

// Fragment size constants
const (
	MinFragmentSize     = 8 * KB // 8KB
	MaxFragmentSize     = 1 * GB // 1GB
	DefaultFragmentSize = 8 * KB // 8KB
)

// parseFragmentSize parses human-readable fragment size input and returns bytes
func parseFragmentSize(input string) (int, error) {
	input = strings.TrimSpace(input)

	// Handle empty input (default)
	if input == "" {
		return DefaultFragmentSize, nil
	}

	// Check for multiple decimal points
	if strings.Count(input, ".") > 1 {
		return 0, fmt.Errorf("invalid format: multiple decimal points not allowed")
	}

	// Extract number and unit using more robust parsing
	var numStr strings.Builder
	var unit string

	for i, r := range input {
		if (r >= '0' && r <= '9') || r == '.' {
			numStr.WriteRune(r)
		} else {
			unit = input[i:]
			break
		}
	}

	numPart := numStr.String()
	unit = strings.ToUpper(strings.TrimSpace(unit))

	// Parse the number
	var num float64
	var err error
	if numPart == "" {
		return 0, fmt.Errorf("invalid format: no number found")
	}

	num, err = strconv.ParseFloat(numPart, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number format: %v", err)
	}

	if num < 0 {
		return 0, fmt.Errorf("fragment size cannot be negative")
	}

	// Determine the multiplier based on unit
	var multiplier int64
	switch unit {
	case "":
		// Raw bytes
		multiplier = 1
	case "B":
		multiplier = 1
	case "KB":
		multiplier = KB
	case "MB":
		multiplier = MB
	case "GB":
		multiplier = GB
	default:
		return 0, fmt.Errorf("unsupported unit '%s'. Supported units: B, KB, MB, GB (or raw bytes)", unit)
	}

	// Calculate total bytes with proper rounding to avoid floating-point precision issues
	totalBytes := int64(num*float64(multiplier) + 0.5)

	return validateFragmentSize(int(totalBytes))
}

// validateFragmentSize validates that the fragment size is within acceptable range
func validateFragmentSize(size int) (int, error) {
	if size < MinFragmentSize {
		return 0, fmt.Errorf("fragment size must be at least %d bytes (8KB), got %d", MinFragmentSize, size)
	}

	if size > MaxFragmentSize {
		return 0, fmt.Errorf("fragment size must be at most %d bytes (1GB), got %d", MaxFragmentSize, size)
	}

	return size, nil
}

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

		fmt.Println("🚀 Starting HydrAIDE install wizard...")
		fmt.Println()

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
		fmt.Println("This is the port where the HydrAIDE binary server will listen for client connections.")
		fmt.Println("Set the bind port for the HydrAIDE server instance.")

		// Port validation loop for main port
		for {
			fmt.Print("Which port should HydrAIDE listen on? (default: 4900): ")
			portInput, _ := reader.ReadString('\n')
			portInput = strings.TrimSpace(portInput)

			if portInput == "" {
				envCfg.HydraidePort = "4900"
				break
			}

			validPort, err := validatePort(portInput)
			if err != nil {
				fmt.Printf("❌ Invalid port: %v. Please try again.\n", err)
				continue
			}

			envCfg.HydraidePort = validPort
			break
		}

		fmt.Println("\n📁 Base Path for HydrAIDE")
		fmt.Println("This is the main directory where HydrAIDE will store its core files.")
		fmt.Print("Base path (default: /mnt/hydraide): ")
		envCfg.HydraideBasePath, _ = reader.ReadString('\n')
		envCfg.HydraideBasePath = strings.TrimSpace(envCfg.HydraideBasePath)
		if envCfg.HydraideBasePath == "" {
			envCfg.HydraideBasePath = "/mnt/hydraide"
		}

		// LOG_LEVEL
		fmt.Println("\n📝 Logging Configuration")
		fmt.Println("   - Controls the verbosity of system logs.")
		fmt.Println("   - Options: debug | info | warn | error")
		fmt.Println("   - Default: info (recommended for production)")

		// loglevel validation loop
		for {
			fmt.Print("Choose log level [default: info]: ")
			logLevel, _ := reader.ReadString('\n')

			logLevel, err := validateLoglevel(logLevel)
			if err != nil {
				fmt.Printf("\n ❌ Invalid loglevel: %v. Please try again.\n", err)
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
		fmt.Printf("   Valid range: %s to %s\n", formatSize(MinMessageSize), formatSize(MaxMessageSize))

		// Message size validation loop
		for {
			fmt.Printf("Max message size [default: %s]: ", formatSize(DefaultMessageSize))
			maxSizeInput, _ := reader.ReadString('\n')
			maxSizeInput = strings.TrimSpace(maxSizeInput)

			size, err := parseMessageSize(maxSizeInput)
			if err != nil {
				fmt.Printf("❌ Invalid input: %v. Please try again.\n", err)
				continue
			}

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
		fmt.Println("   Controls the size of storage fragments for Swamp data")
		fmt.Println("   Accepts human-readable format: 8KB, 64KB, 1MB, 512MB, 1GB")
		fmt.Println("   Range: 8KB to 1GB (default: 8KB)")

		// Fragment size validation loop
		for {
			fmt.Print("Storage fragment size [default: 8KB]: ")
			sizeInput, _ := reader.ReadString('\n')

			validSize, err := parseFragmentSize(sizeInput)
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

			validPort, err := validatePort(healthPortInput)
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
		fmt.Printf("  • Max Message Size: %s (%d bytes)\n", formatSize(envCfg.GRPCMaxMessageSize), envCfg.GRPCMaxMessageSize)
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
		if err := utils.MoveFile(clientCRT, filepath.Join(envCfg.HydraideBasePath, "certificate", filepath.Base(clientCRT))); err != nil {
			fmt.Println("❌ Error copying client certificate:", err)
			return
		}
		fmt.Printf("  • Server CRT: From %s  to  %s \n", serverCRT, filepath.Join(envCfg.HydraideBasePath, "certificate", filepath.Base(serverCRT)))
		if err := utils.MoveFile(serverCRT, filepath.Join(envCfg.HydraideBasePath, "certificate", filepath.Base(serverCRT))); err != nil {
			fmt.Println("❌ Error copying server certificate:", err)
			return
		}
		fmt.Printf("  • Server KEY: From %s  to  %s \n", serverKEY, filepath.Join(envCfg.HydraideBasePath, "certificate", filepath.Base(serverKEY)))
		if err := utils.MoveFile(serverKEY, filepath.Join(envCfg.HydraideBasePath, "certificate", filepath.Base(serverKEY))); err != nil {
			fmt.Println("❌ Error copying server key:", err)
			return
		}

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
		defer func() {
			if err := envFile.Close(); err != nil {
				fmt.Println("❌ Error closing .env file:", err)
			} else {
				fmt.Println("✅ .env file closed successfully.")
			}
		}()

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
