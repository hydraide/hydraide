package servicehelper

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ServiceManager defines the interface for managing platform-specific services.
type ServiceManager interface {
	// GenerateServiceFile creates a platform-specific service configuration.
	GenerateServiceFile(instanceName, basePath string) error

	// ServiceExists checks if a service with the given name exists.
	ServiceExists(instanceName string) (bool, error)

	// EnableAndStartService enables and starts the service.
	EnableAndStartService(instanceName string) error

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

// NewServiceManager creates a new instance of ServiceManager.
func New() ServiceManager {
	return &serviceManagerImpl{}
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
	switch runtime.GOOS {
	case LINUX_OS:
		return generateSystemdService(instanceName, basePath)
	case MAC_OS:
		return generateLaunchdService(instanceName, basePath)
	case WINDOWS_OS:
		return generateWindowsService(instanceName, basePath)
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
func generateSystemdService(instanceName, basePath string) error {
	serviceName := fmt.Sprintf("%s-%s", BASE_SERVICE_NAME, instanceName)
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		return fmt.Errorf("HOME environment variable not set")
	}
	serviceFilePath := filepath.Join(homeDir, ".config", "systemd", "user", fmt.Sprintf("%s.service", serviceName))

	serviceContent := fmt.Sprintf(`[Unit]
					Description=HydrAIDE Service - %s
					After=network.target

					[Service]
					ExecStart=%s/hydraserver
					WorkingDirectory=%s
					Restart=always
					User=%s

					[Install]
					WantedBy=default.target
					`, instanceName, filepath.Join(basePath, LINUX_MAC_BINARY_NAME), basePath, os.Getenv("USER"))

	// Create parent directories if they don't exist
	if err := os.MkdirAll(filepath.Dir(serviceFilePath), 0755); err != nil {
		return fmt.Errorf("failed to create directories for service file: %v", err)
	}

	// Check if the service file already exists
	if _, err := os.Stat(serviceFilePath); err == nil {
		return fmt.Errorf("service file '%s' already exists", serviceFilePath)
	}

	// Write the service file
	if err := os.WriteFile(serviceFilePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %v", err)
	}

	fmt.Printf("Service file '%s' created successfully.\n", serviceFilePath)
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
func generateLaunchdService(instanceName, basePath string) error {
	// TODO: Implement launchd service file generation logic
	return fmt.Errorf("launchd service generation not implemented")
}

// generateWindowsService creates a Windows service using nssm or PowerShell.
//
// Parameters:
//   - instanceName: The name of the service instance
//   - basePath: The base path where the service executable is located
//
// Returns:
//   - error: Any error encountered during service creation
func generateWindowsService(instanceName, basePath string) error {
	serviceName := fmt.Sprintf("%s-%s", BASE_SERVICE_NAME, instanceName)
	servicePath := filepath.Join(basePath, WINDOWS_BINARY_NAME)

	// Check if nssm is available
	if _, err := exec.LookPath("nssm"); err == nil {
		cmd := exec.Command("nssm", "install", serviceName, servicePath)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create service using nssm: %v", err)
		}
		fmt.Printf("Service '%s' created successfully using nssm.\n", serviceName)
		return nil
	}

	// Fallback to PowerShell's New-Service
	powershellCmd := fmt.Sprintf(`New-Service -Name "%s" -BinaryPathName "%s" -DisplayName "%s" -StartupType Automatic`, serviceName, servicePath, serviceName)
	cmd := exec.Command("powershell", "-Command", powershellCmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create service using PowerShell: %v, cmd: %s", err, powershellCmd)
	}

	fmt.Printf("Service '%s' created successfully using PowerShell.\n", serviceName)
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
	switch runtime.GOOS {
	case LINUX_OS:
		serviceName := fmt.Sprintf("%s-%s", BASE_SERVICE_NAME, instanceName)
		serviceFilePath := filepath.Join(os.Getenv("HOME"), ".config", "systemd", "user", fmt.Sprintf("%s.service", serviceName))
		_, err := os.Stat(serviceFilePath)
		if err == nil {
			return true, nil
		}
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check service existence: %v", err)
	case MAC_OS:
		// TODO: Implement launchctl service existence check
		return false, fmt.Errorf("launchd service existence check not implemented")
	case WINDOWS_OS:
		serviceName := fmt.Sprintf("%s-%s", BASE_SERVICE_NAME, instanceName)
		cmd := exec.Command("powershell", "-Command", fmt.Sprintf(`Get-Service -Name "%s" -ErrorAction SilentlyContinue`, serviceName))
		if err := cmd.Run(); err == nil {
			return true, nil
		}
		return false, nil
	default:
		return false, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// EnableAndStartService enables and starts a service on the system.
//
// Parameters:
//   - instanceName: The name of the service instance to enable and start
//
// Returns:
//   - error: Any error encountered during the operation
func (s *serviceManagerImpl) EnableAndStartService(instanceName string) error {
	switch runtime.GOOS {
	case LINUX_OS:
		serviceName := fmt.Sprintf("%s-%s", BASE_SERVICE_NAME, instanceName)
		// Enable the service
		cmd := exec.Command("systemctl", "--user", "enable", fmt.Sprintf("%s.service", serviceName))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to enable service: %v", err)
		}
		// Start the service
		cmd = exec.Command("systemctl", "--user", "start", fmt.Sprintf("%s.service", serviceName))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to start service: %v", err)
		}
		fmt.Printf("Service '%s' enabled and started successfully.\n", serviceName)
	case MAC_OS:
		// TODO: Implement launchctl load and start logic
		return fmt.Errorf("launchd service enabling not implemented")
	case WINDOWS_OS:
		serviceName := fmt.Sprintf("%s-%s", BASE_SERVICE_NAME, instanceName)
		cmd := exec.Command("powershell", "-Command", fmt.Sprintf(`Start-Service -Name "%s"`, serviceName))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to start service: %v", err)
		}
		fmt.Printf("Service '%s' started successfully.\n", serviceName)
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
	switch runtime.GOOS {
	case LINUX_OS:
		serviceName := fmt.Sprintf("%s-%s", BASE_SERVICE_NAME, instanceName)
		serviceFilePath := filepath.Join(os.Getenv("HOME"), ".config", "systemd", "user", fmt.Sprintf("%s.service", serviceName))

		// Stop the service if running
		_ = exec.Command("systemctl", "--user", "stop", fmt.Sprintf("%s.service", serviceName)).Run()

		// Disable the service
		_ = exec.Command("systemctl", "--user", "disable", fmt.Sprintf("%s.service", serviceName)).Run()

		// Remove the service file
		if err := os.Remove(serviceFilePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove service file: %v", err)
		}
		fmt.Printf("Service '%s' removed successfully.\n", serviceName)
	case MAC_OS:
		// TODO: Implement launchctl service removal
		return fmt.Errorf("launchd service removal not implemented")
	case WINDOWS_OS:
		serviceName := fmt.Sprintf("%s-%s", BASE_SERVICE_NAME, instanceName)
		// Try nssm first
		if _, err := exec.LookPath("nssm"); err == nil {
			cmd := exec.Command("nssm", "remove", serviceName, "confirm")
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to remove service using nssm: %v", err)
			}
		} else {
			// Fallback to PowerShell
			cmd := exec.Command("powershell", "-Command", fmt.Sprintf(`Stop-Service -Name "%s" -Force; Remove-Service -Name "%s"`, serviceName, serviceName))
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to remove service using PowerShell: %v", err)
			}
		}
		fmt.Printf("Service '%s' removed successfully.\n", serviceName)
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
	return nil
}
