package instancerunner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

var instanceName = "temporary-test-service"

func TestStartInstance(t *testing.T) {
	// Setup service
	serviceName, err := createTestServiceFile()
	if err != nil {
		fmt.Printf("Failed to create service file: %s", err.Error())
		t.Error(err)
	}
	// Stop service and remove service file post-test
	t.Cleanup(func() {
		exec.Command("systemctl", "--user", "stop", serviceName).Run()
		removeTestServiceFile()
	})

	instance := NewInstanceController()
	if _, ok := instance.(*systemdController); !ok {
		t.Fatal("Expected systemdController, got a different type")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Logf("Calling StartInstance for '%s'", serviceName)
	err = instance.StartInstance(ctx, instanceName)
	if err != nil {
		t.Fatalf("StartInstance failed with error: %v", err)
	}

	// Give systemd a moment to update its state
	time.Sleep(1 * time.Second)

	t.Log("Verifying service is up...")
	isActive, err := isServiceActive(serviceName)
	if err != nil {
		t.Fatalf("Failed to check service status: %v", err)
	}

	if !isActive {
		t.Fatal("Service is not running after being started")
	}

	t.Logf("Service '%s' is up and running.", serviceName)
}

func TestStopInstance(t *testing.T) {
	// Setup service
	serviceName, err := createTestServiceFile()
	if err != nil {
		fmt.Printf("Failed to create service file: %s", err.Error())
		t.Error(err)
	}
	// Stop service and remove service file post-test
	t.Cleanup(func() {
		exec.Command("systemctl", "--user", "stop", serviceName).Run()
		removeTestServiceFile()
	})

	instance := NewInstanceController()
	if _, ok := instance.(*systemdController); !ok {
		t.Fatal("Expected systemdController, got a different type")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Ensure the service is running before attempting to stop.
	if err := instance.StartInstance(ctx, instanceName); err != nil {
		t.Fatalf("Pre-start failed: %v", err)
	}
	time.Sleep(1 * time.Second) // Wait for complete start before proceeding.

	t.Logf("Calling StopInstance for '%s'", instanceName)
	err = instance.StopInstance(ctx, instanceName)
	if err != nil {
		t.Fatalf("StopInstance failed with unexpected error: %v", err)
	}

	t.Log("Verifying service is stopped...")
	isActive, err := isServiceActive(serviceName)
	if err != nil {
		t.Fatalf("Failed to check service status after stop: %v", err)
	}

	if isActive {
		t.Fatal("Service is still running after being stopped")
	}
	t.Logf("Service '%s' is successfully stopped.", serviceName)
}

func TestRestartInstance(t *testing.T) {
	// Setup service
	serviceName, err := createTestServiceFile()
	if err != nil {
		fmt.Printf("Failed to create service file: %s", err.Error())
		t.Error(err)
	}
	// Stop service and remove service file post-test
	t.Cleanup(func() {
		exec.Command("systemctl", "--user", "stop", serviceName).Run()
		removeTestServiceFile()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Ensure the service is not running before the test.
	stopCmd := exec.Command("systemctl", "--user", "stop", serviceName)
	stopCmd.Run()

	instance := NewInstanceController()
	if _, ok := instance.(*systemdController); !ok {
		t.Fatal("Expected systemdController")
	}

	// Start the service once to ensure it can be started
	if err := instance.StartInstance(ctx, instanceName); err != nil {
		t.Fatalf("Pre-start failed: %v", err)
	}
	time.Sleep(1 * time.Second)

	t.Logf("Calling RestartInstance for '%s'", instanceName)
	if err := instance.RestartInstance(ctx, instanceName); err != nil {
		t.Fatalf("RestartInstance failed unexpectedly: %v", err)
	}

	// The restart function now handles waiting, so we just need to check the final state.
	t.Log("Verifying service is up after restart...")
	isActive, err := isServiceActive(serviceName)
	if err != nil {
		t.Fatalf("Failed to check service status after restart: %v", err)
	}
	if !isActive {
		t.Fatal("Service is not running after being restarted")
	}
}

func TestMissingService(t *testing.T) {
	instance := NewInstanceController()
	if _, ok := instance.(*systemdController); !ok {
		t.Fatal("Expected systemdController")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	missingInstanceName := "non-existent-test-service"
	t.Logf("Testing StartInstance with a missing service '%s'", missingInstanceName)
	err := instance.StartInstance(ctx, missingInstanceName)
	if err == nil {
		t.Fatal("Expected an error for a missing service, but got none")
	}

	expectedError := fmt.Sprintf("service 'hydraserver-%s.service' not found", missingInstanceName)
	if err.Error() != expectedError {
		t.Fatalf("Expected error '%s', but got '%s'", expectedError, err.Error())
	}
}

// Helper function to check service status.
func isServiceActive(serviceName string) (bool, error) {
	cmd := exec.Command("systemctl", "--user", "is-active", "--quiet", serviceName)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if exitError, ok := err.(*exec.ExitError); ok {
		// Inactive service or unknown unit
		if exitError.ExitCode() == 3 || exitError.ExitCode() == 4 {
			return false, nil
		}
	}
	return false, err
}

// createTestServiceFile creates a dummy systemd user service file for testing purposes.
// It returns the service name to the created service file.
func createTestServiceFile() (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("Only linux OS is supported.")
	}

	content := `
		[Unit]
		Description=HydrAIDE Test Service
		
		[Service]
		ExecStart=/bin/bash -c "while true; do echo 'Service running...'; sleep 1; done"
		Restart=always
		
		[Install]
		WantedBy=default.target
	`

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	serviceDir := filepath.Join(homeDir, ".config", "systemd", "user")
	serviceFile := fmt.Sprintf("hydraserver-%s.service", instanceName)
	fullPath := filepath.Join(serviceDir, serviceFile)

	// Create the directory if it doesn't exist
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create service directory: %w", err)
	}

	// Write the service file content
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write service file: %w", err)
	}

	// Reload the systemd daemon to pick up the new service file.
	// This is a crucial step for the tests to work.
	cmd := exec.Command("systemctl", "--user", "daemon-reload")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to run 'systemctl --user daemon-reload': %w", err)
	}

	return serviceFile, nil
}

// removeTestServiceFile stops the service and removes the file created by createTestServiceFile.
func removeTestServiceFile() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}
	serviceFile := fmt.Sprintf("hydraserver-%s.service", instanceName)
	fullPath := filepath.Join(homeDir, ".config", "systemd", "user", serviceFile)

	// Stop the service before attempting to remove the file
	exec.Command("systemctl", "--user", "stop", serviceFile).Run()
	os.Remove(fullPath)
	exec.Command("systemctl", "--user", "daemon-reload").Run()
}
