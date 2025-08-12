package servicehelper

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/locker"
)

// ServiceManager defines the interface for managing platform-specific services.
type ServiceManager interface {
	// GenerateServiceFile creates a platform-specific service configuration.
	GenerateServiceFile(instanceName, basePath string) error

	// ServiceExists checks if a service with the given name exists.
	ServiceExists(instanceName string) (bool, error)

	// EnableAndStartService enables and starts the service.
	EnableAndStartService(instanceName, basePath string) error

	// RemoveService removes a service configuration.
	RemoveService(instanceName string) error
}

const (
	BASE_SERVICE_NAME     = "hydraserver"
	LINUX_OS              = "linux"
	WINDOWS_OS            = "windows"
	MAC_OS                = "darwin"
	WINDOWS_BINARY_NAME   = "hydraide-windows-amd64.exe"
	LINUX_MAC_BINARY_NAME = "hydraide-linux-amd64"
	HTTP_TIMEOUT          = 10
)

// serviceManagerImpl implements the ServiceManager interface for different platforms.
type serviceManagerImpl struct{}

// New creates a new instance of ServiceManager.
func New() ServiceManager {
	return &serviceManagerImpl{}
}

// ensureLogDirectory creates the logs directory if it doesn't exist
func ensureLogDirectory(basePath string) (string, error) {
	logDir := filepath.Join(basePath, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create logs directory: %v", err)
	}

	logFile := filepath.Join(logDir, "app.log")
	slog.Info("Log directory ensured", "path", logDir)
	slog.Info("Log file path", "path", logFile)

	return logFile, nil
}

// GenerateServiceFile generates a platform-specific service file for hydraserver.
// It delegates to platform-specific implementations based on the operating system.
//
// Parameters:
//   - instanceName: The name of the service instance
//   - basePath: The base path where the service executable is located
//
// Returns:
//   - error: Any error encountered during service file generation
func (s *serviceManagerImpl) GenerateServiceFile(instanceName, basePath string) error {
	slog.Info("Generating service file", "instance", instanceName, "os", runtime.GOOS)

	switch runtime.GOOS {
	case LINUX_OS:
		return s.generateSystemdService(instanceName, basePath)
	case MAC_OS:
		return s.generateLaunchdService(instanceName, basePath)
	case WINDOWS_OS:
		return s.generateWindowsNSSMService(instanceName, basePath)
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// generateSystemdService creates a systemd service file for Linux systems.
//
// Parameters:
//   - instanceName: The name of the service instance
//   - basePath: The base path where the service executable is located
//
// Returns:
//   - error: Any error encountered during service file creation
func (s *serviceManagerImpl) generateSystemdService(instanceName, basePath string) error {
	slog.Info("Creating systemd service for Linux")

	serviceName := fmt.Sprintf("%s-%s", BASE_SERVICE_NAME, instanceName)

	runCommand := func(name string, args ...string) error {
		cmd := exec.Command(name, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	serviceFilePath := filepath.Join("/etc", "systemd", "system", fmt.Sprintf("%s.service", serviceName))
	executablePath := filepath.Join(basePath, LINUX_MAC_BINARY_NAME)

	// Ensure log directory exists
	logFile, err := ensureLogDirectory(basePath)
	if err != nil {
		return err
	}

	serviceContent := fmt.Sprintf(`[Unit]
			Description=HydrAIDE Service - %s
			After=network.target

			[Service]
			ExecStart=%s
			WorkingDirectory=%s
			Restart=always
			RestartSec=5
			StandardOutput=append:%s
			StandardError=append:%s

			[Install]
			WantedBy=multi-user.target
`, instanceName, executablePath, basePath, logFile, logFile)

	// Create parent directories if they don't exist
	if err := os.MkdirAll(filepath.Dir(serviceFilePath), 0755); err != nil {
		return fmt.Errorf("failed to create directories for service file: %v", err)
	}

	// Check if the service file already exists
	if _, err := os.Stat(serviceFilePath); err == nil {
		slog.Warn("Service file already exists", "path", serviceFilePath)
		return fmt.Errorf("service file '%s' already exists", serviceFilePath)
	}

	// Write the service file
	if err := os.WriteFile(serviceFilePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %v", err)
	}

	// Reload systemd daemon (system-level, not user-level)
	if err := runCommand("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload systemd daemon: %v", err)
	}

	// Enable the service (system-level, not user-level)
	if err := runCommand("systemctl", "enable", serviceName+".service"); err != nil {
		return fmt.Errorf("failed to enable service: %v", err)
	}

	// Start the service (system-level, not user-level)
	if err := runCommand("systemctl", "start", serviceName+".service"); err != nil {
		return fmt.Errorf("failed to start service: %v", err)
	}

	slog.Info("Service file created successfully", "path", serviceFilePath)
	slog.Info("Logs will be written to", "path", logFile)
	return nil
}

// generateLaunchdService creates a launchd service file for macOS systems.
//
// Parameters:
//   - instanceName: The name of the service instance
//   - basePath: The base path where the service executable is located
//
// Returns:
//   - error: Any error encountered during service file creation
func (s *serviceManagerImpl) generateLaunchdService(instanceName, basePath string) error {
	slog.Info("Creating launchd service for macOS")
	// TODO: Implement launchd service file generation logic
	return fmt.Errorf("launchd service generation not implemented")
}

// checkAndInstallNSSM checks if NSSM (Non-Sucking Service Manager) is installed on the system.
// If NSSM is not found in the system PATH, it attempts to install it using winget.
// Logs all actions and returns an error if installation fails.
func (s *serviceManagerImpl) checkAndInstallNSSM() error {
	slog.Info("Checking if NSSM is installed")

	// Try to run `nssm version` to verify it's available
	cmd := exec.Command("nssm", "version")
	if err := cmd.Run(); err == nil {
		slog.Info("NSSM is already installed and available in PATH")
		return nil
	}

	slog.Warn("NSSM not found. Attempting installation using winget")

	// Construct winget install command
	installCmd := exec.Command("winget", "install", "--id=nssm.nssm", "--source=winget", "--accept-package-agreements", "--accept-source-agreements")

	output, err := installCmd.CombinedOutput()
	if err != nil {
		slog.Error("Failed to install NSSM via winget", "error", err, "output", string(output))
		return fmt.Errorf("failed to install NSSM via winget: %w", err)
	}

	slog.Info("NSSM installed successfully using winget", "output", string(output))
	return nil
}

// generateWindowsNSSMService creates a Windows service using NSSM
func (s *serviceManagerImpl) generateWindowsNSSMService(instanceName, basePath string) error {
	slog.Info("Creating Windows service using NSSM")

	// Check and install NSSM if needed
	if err := s.checkAndInstallNSSM(); err != nil {
		return fmt.Errorf("NSSM installation failed: %v", err)
	}

	serviceName := fmt.Sprintf("%s-%s", BASE_SERVICE_NAME, instanceName)
	executablePath := filepath.Join(basePath, WINDOWS_BINARY_NAME)

	// Ensure log directory exists
	logFile, err := ensureLogDirectory(basePath)
	if err != nil {
		return err
	}

	// Check if executable exists
	if _, err := os.Stat(executablePath); os.IsNotExist(err) {
		return fmt.Errorf("executable not found at: %s", executablePath)
	}

	slog.Info("Installing NSSM service", "service", serviceName)

	// Install the service using NSSM
	cmd := exec.Command("nssm", "install", serviceName, executablePath)
	cmd.Dir = basePath
	if output, err := cmd.CombinedOutput(); err != nil {
		slog.Error("Failed to install NSSM service", "output", string(output), "error", err)
		return fmt.Errorf("failed to install NSSM service: %v", err)
	}

	// Configure service parameters
	configs := [][]string{
		{"set", serviceName, "DisplayName", fmt.Sprintf("HydrAIDE Service - %s", instanceName)},
		{"set", serviceName, "Description", fmt.Sprintf("HydrAIDE Service Instance: %s", instanceName)},
		{"set", serviceName, "Start", "SERVICE_AUTO_START"},
		{"set", serviceName, "AppDirectory", basePath},
		{"set", serviceName, "AppStdout", logFile},
		{"set", serviceName, "AppStderr", logFile},
		{"set", serviceName, "AppRotateFiles", "1"},
		{"set", serviceName, "AppRotateSeconds", "86400"},  // Rotate daily
		{"set", serviceName, "AppRotateBytes", "10485760"}, // 10MB
	}

	for _, config := range configs {
		cmd := exec.Command("nssm", config...)
		if output, err := cmd.CombinedOutput(); err != nil {
			slog.Warn("Failed to set NSSM config", "config", config[2], "error", err, "output", string(output))
		} else {
			slog.Info("Set NSSM config successfully", "config", config[2])
		}
	}

	slog.Info("NSSM service configured successfully", "service", serviceName)
	slog.Info("Logs will be written to", "path", logFile)
	return nil
}

// ServiceExists checks if a service with the given name exists on the system.
//
// Parameters:
//   - instanceName: The name of the service instance to check
//
// Returns:
//   - bool: True if the service exists, false otherwise
//   - error: Any error encountered during the check
func (s *serviceManagerImpl) ServiceExists(instanceName string) (bool, error) {
	serviceName := fmt.Sprintf("%s-%s", BASE_SERVICE_NAME, instanceName)
	slog.Info("Checking if service exists", "service", serviceName, "os", runtime.GOOS)

	switch runtime.GOOS {
	case LINUX_OS:
		// Check system-level service file (consistent with creation)
		serviceFilePath := filepath.Join("/etc", "systemd", "system", fmt.Sprintf("%s.service", serviceName))
		_, err := os.Stat(serviceFilePath)
		if err == nil {
			slog.Info("Service file found", "path", serviceFilePath)
			return true, nil
		}
		if os.IsNotExist(err) {
			slog.Info("Service file not found", "path", serviceFilePath)
			return false, nil
		}
		return false, fmt.Errorf("failed to check service existence: %v", err)

	case MAC_OS:
		// TODO: Implement launchctl service existence check
		return false, fmt.Errorf("launchd service existence check not implemented")

	case WINDOWS_OS:
		// Check NSSM service first
		cmd := exec.Command("nssm", "status", serviceName)
		if output, err := cmd.CombinedOutput(); err == nil {
			status := strings.TrimSpace(string(output))
			slog.Info("NSSM service found", "service", serviceName, "status", status)
			return true, nil
		}

		// Check Task Scheduler (fallback)
		cmd = exec.Command("schtasks", "/query", "/tn", serviceName)
		if err := cmd.Run(); err == nil {
			slog.Info("Scheduled task found", "task", serviceName)
			return true, nil
		}

		// Check Registry startup (fallback)
		regCmd := fmt.Sprintf(`Get-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "%s" -ErrorAction SilentlyContinue`, serviceName)
		cmd = exec.Command("powershell", "-Command", regCmd)
		if err := cmd.Run(); err == nil {
			slog.Info("Registry startup entry found", "entry", serviceName)
			return true, nil
		}

		// Check Startup folder (fallback)
		startupFolder := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
		shortcutPath := filepath.Join(startupFolder, fmt.Sprintf("%s.lnk", serviceName))
		if _, err := os.Stat(shortcutPath); err == nil {
			slog.Info("Startup folder shortcut found", "path", shortcutPath)
			return true, nil
		}

		slog.Info("No service found", "service", serviceName)
		return false, nil

	default:
		return false, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// EnableAndStartService enables and starts a service on the system.
//
// Parameters:
//   - instanceName: The name of the service instance to enable and start
//   - basePath: The base path where the service executable is located
//
// Returns:
//   - error: Any error encountered during the operation
func (s *serviceManagerImpl) EnableAndStartService(instanceName, basePath string) error {
	serviceName := fmt.Sprintf("%s-%s", BASE_SERVICE_NAME, instanceName)
	slog.Info("Starting service", "service", serviceName, "os", runtime.GOOS)

	switch runtime.GOOS {
	case LINUX_OS:
		// Use system-level systemd commands (consistent with creation)

		// Reload systemd daemon first
		slog.Info("Reloading systemd daemon")
		cmd := exec.Command("systemctl", "daemon-reload")
		if err := cmd.Run(); err != nil {
			slog.Warn("Failed to reload systemd daemon", "error", err)
		}

		// Enable the service
		slog.Info("Enabling service", "service", serviceName)
		cmd = exec.Command("systemctl", "enable", fmt.Sprintf("%s.service", serviceName))
		if output, err := cmd.CombinedOutput(); err != nil {
			slog.Error("Failed to enable service", "error", err, "output", string(output))
			return fmt.Errorf("failed to enable service: %v", err)
		}

		// Start the service
		slog.Info("Starting service", "service", serviceName)
		cmd = exec.Command("systemctl", "start", fmt.Sprintf("%s.service", serviceName))
		if output, err := cmd.CombinedOutput(); err != nil {
			slog.Error("Failed to start service", "error", err, "output", string(output))
			return fmt.Errorf("failed to start service: %v", err)
		}

		// Check service status
		slog.Info("Checking service status", "service", serviceName)
		cmd = exec.Command("systemctl", "status", fmt.Sprintf("%s.service", serviceName), "--no-pager")
		if output, err := cmd.CombinedOutput(); err == nil {
			slog.Info("Service status", "output", string(output))
		} else {
			slog.Warn("Failed to check service status", "error", err, "output", string(output))
		}

		slog.Info("Service enabled and started successfully", "service", serviceName)

	case MAC_OS:
		// TODO: Implement launchctl load and start logic
		return fmt.Errorf("launchd service enabling not implemented")

	case WINDOWS_OS:
		// Try NSSM service first
		slog.Info("Attempting to start NSSM service", "service", serviceName)
		cmd := exec.Command("nssm", "start", serviceName)
		if output, err := cmd.CombinedOutput(); err == nil {
			slog.Info("NSSM service started successfully", "service", serviceName, "output", string(output))

			// Check service status
			statusCmd := exec.Command("nssm", "status", serviceName)
			if statusOutput, err := statusCmd.CombinedOutput(); err == nil {
				status := strings.TrimSpace(string(statusOutput))
				slog.Info("Service status", "service", serviceName, "status", status)
			}
			return nil
		} else {
			slog.Warn("NSSM start failed", "error", err, "output", string(output))
		}

		// Fallback to other methods
		servicePath := filepath.Join(basePath, WINDOWS_BINARY_NAME)

		// Check if it's a scheduled task
		slog.Info("Attempting to start scheduled task", "task", serviceName)
		cmd = exec.Command("schtasks", "/query", "/tn", serviceName)
		if err := cmd.Run(); err == nil {
			// Run the scheduled task
			runCmd := exec.Command("schtasks", "/run", "/tn", serviceName)
			if err := runCmd.Run(); err != nil {
				slog.Error("Failed to start scheduled task", "error", err)
				return fmt.Errorf("failed to start scheduled task: %v", err)
			}
			slog.Info("Scheduled task started successfully", "task", serviceName)
			return nil
		}

		// If not a scheduled task, try to start the executable directly
		slog.Info("Attempting to start executable directly", "path", servicePath)
		cmd = exec.Command(servicePath)
		cmd.Dir = basePath
		if err := cmd.Start(); err != nil {
			slog.Error("Failed to start service executable", "error", err)
			return fmt.Errorf("failed to start service executable: %v", err)
		}

		slog.Info("Service started successfully", "service", serviceName)
		return nil

	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
	return nil
}

// RemoveService removes a service configuration from the system.
//
// Parameters:
//   - instanceName: The name of the service instance to remove
//
// Returns:
//   - error: Any error encountered during service removal
func (s *serviceManagerImpl) RemoveService(instanceName string) error {
	serviceName := fmt.Sprintf("%s-%s", BASE_SERVICE_NAME, instanceName)
	slog.Info("Removing service", "service", serviceName, "os", runtime.GOOS)

	switch runtime.GOOS {
	case LINUX_OS:
		// Use system-level systemd commands (consistent with creation)
		serviceFilePath := filepath.Join("/etc", "systemd", "system", fmt.Sprintf("%s.service", serviceName))

		// Stop the service if running
		slog.Info("Stopping service", "service", serviceName)
		cmd := exec.Command("systemctl", "stop", fmt.Sprintf("%s.service", serviceName))
		if output, err := cmd.CombinedOutput(); err != nil {
			slog.Warn("Failed to stop service", "error", err, "output", string(output))
		} else {
			slog.Info("Service stopped successfully", "service", serviceName)
		}

		// delete lock file
		if err := locker.DeleteLockFile(instanceName); err != nil {
			// log the error and continue
			slog.Error("Failed to delete lock file for instance", "instanceName", instanceName)
		}

		// Disable the service
		slog.Info("Disabling service", "service", serviceName)
		cmd = exec.Command("systemctl", "disable", fmt.Sprintf("%s.service", serviceName))
		if output, err := cmd.CombinedOutput(); err != nil {
			slog.Warn("Failed to disable service", "error", err, "output", string(output))
		} else {
			slog.Info("Service disabled successfully", "service", serviceName)
		}

		// Remove the service file
		slog.Info("Removing service file", "path", serviceFilePath)
		if err := os.Remove(serviceFilePath); err != nil && !os.IsNotExist(err) {
			slog.Error("Failed to remove service file", "error", err)
			return fmt.Errorf("failed to remove service file: %v", err)
		}

		// Reload systemd daemon
		slog.Info("Reloading systemd daemon")
		cmd = exec.Command("systemctl", "daemon-reload")
		if err := cmd.Run(); err != nil {
			slog.Warn("Failed to reload systemd daemon", "error", err)
		}

		slog.Info("Service removed successfully", "service", serviceName)

	case MAC_OS:
		// TODO: Implement launchctl service removal
		return fmt.Errorf("launchd service removal not implemented")

	case WINDOWS_OS:
		// Try to remove NSSM service first
		slog.Info("Attempting to remove NSSM service", "service", serviceName)

		// Stop the service first
		stopCmd := exec.Command("nssm", "stop", serviceName)
		if output, err := stopCmd.CombinedOutput(); err != nil {
			slog.Warn("Failed to stop NSSM service", "error", err, "output", string(output))
		} else {
			slog.Info("NSSM service stopped successfully", "service", serviceName)
		}

		// delete lock file
		if err := locker.DeleteLockFile(instanceName); err != nil {
			// log the error and continue
			slog.Error("Failed to delete lock file for instance", "instanceName", instanceName)
		}

		// Remove the service
		removeCmd := exec.Command("nssm", "remove", serviceName, "confirm")
		if output, err := removeCmd.CombinedOutput(); err != nil {
			slog.Warn("Failed to remove NSSM service", "error", err, "output", string(output))
		} else {
			slog.Info("NSSM service removed successfully", "service", serviceName)
		}

		// Try to remove from Task Scheduler (fallback)
		slog.Info("Removing scheduled task (if exists)", "task", serviceName)
		cmd := exec.Command("schtasks", "/delete", "/tn", serviceName, "/f")
		if output, err := cmd.CombinedOutput(); err != nil {
			slog.Warn("Scheduled task removal failed", "error", err, "output", string(output))
		} else {
			slog.Info("Scheduled task removed", "task", serviceName)
		}

		// Try to remove from Registry (fallback)
		slog.Info("Removing registry entry (if exists)", "entry", serviceName)
		regCmd := fmt.Sprintf(`Remove-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "%s" -ErrorAction SilentlyContinue`, serviceName)
		cmd = exec.Command("powershell", "-Command", regCmd)
		if output, err := cmd.CombinedOutput(); err != nil {
			slog.Warn("Registry removal failed", "error", err, "output", string(output))
		} else {
			slog.Info("Registry entry removed", "entry", serviceName)
		}

		// Try to remove from Startup folder (fallback)
		slog.Info("Removing startup shortcut (if exists)", "shortcut", serviceName)
		startupFolder := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
		shortcutPath := filepath.Join(startupFolder, fmt.Sprintf("%s.lnk", serviceName))
		if err := os.Remove(shortcutPath); err != nil && !os.IsNotExist(err) {
			slog.Warn("Failed to remove startup shortcut", "error", err)
		} else {
			slog.Info("Startup shortcut removed", "shortcut", serviceName)
		}

		slog.Info("Service removal completed", "service", serviceName)
		return nil

	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
	return nil
}
