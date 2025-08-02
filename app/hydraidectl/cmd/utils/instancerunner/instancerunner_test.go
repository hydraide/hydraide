package instancerunner

import (
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"
)

// IMPORTANT MANUAL STEP:
// CREATE A VALID SERVICE FILE TO PERFORM THIS TEST OR MAKE SURE IT EXISTS.
// Example:
// Create a file named 'hydraserver-test5.service' in ~/.config/systemd/user/
// with the following content:
//
// [Unit]
// Description=HydrAIDE Test Service
//
// [Service]
// ExecStart=/bin/bash -c "while true; do echo 'Service running...'; sleep 1; done"
// Restart=always
//
// [Install]
// WantedBy=default.target
//
// After creating the file, run 'systemctl --user daemon-reload'
var serviceName = "hydraserver-test5.service"
var instanceName = "test5"

func TestStartInstance(t *testing.T) {
	// We make sure the service not already running
	stopCmd := exec.Command("systemctl", "--user", "stop", serviceName)
	stopCmd.Run()

	t.Cleanup(func() {
		exec.Command("systemctl", "--user", "stop", serviceName).Run()
	})

	instance := NewInstanceController()
	if _, ok := instance.(*systemdController); !ok {
		t.Fatal("Expected systemdController, got a different type")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Logf("Calling StartInstance for '%s'", serviceName)
	err := instance.StartInstance(ctx, instanceName)
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

	t.Cleanup(func() {
		// Clean up by stopping the service at the end, just in case
		exec.Command("systemctl", "--user", "stop", serviceName).Run()
	})

	t.Logf("Calling StopInstance for '%s'", instanceName)
	err := instance.StopInstance(ctx, instanceName)
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
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Ensure the service is not running before the test.
	stopCmd := exec.Command("systemctl", "--user", "stop", serviceName)
	stopCmd.Run()

	t.Cleanup(func() {
		exec.Command("systemctl", "--user", "stop", serviceName).Run()
	})

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
