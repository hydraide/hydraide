package instancerunner

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

// IMPORTANT MANUAL STEP
// CREATE A VALID SERVICE FILE TO PERFORM THIS TEST OR MAKE SURE IT EXISTS
var serviceName = "hydraserver-test5.service"
var instanceName = "test5"

func TestStartInstance(t *testing.T) {

	// We make sure the service not already running
	stopCmd := exec.Command("systemctl", "--user", "stop", serviceName)
	stopCmd.Run()

	// It ensures the service is stopped to prevent interference with other tests.
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start the service once to ensure it can be started
	if err := instance.StartInstance(ctx, instanceName); err != nil {
		t.Fatalf("Pre-start failed: %v", err)
	}

	// Wait for service to be up and running
	time.Sleep(1 * time.Second)
	isActive, err := isServiceActive(serviceName)
	if err != nil {
		t.Fatalf("Failed to check service status after start: %v", err)
	}
	if !isActive {
		t.Fatalf("Service '%s' is not active. Cannot test stop functionality.", serviceName)
	}
	t.Logf("Service '%s' is confirmed to be running.", serviceName)

	t.Cleanup(func() {
		// Clean up by stopping the service at the end, just in case
		exec.Command("systemctl", "--user", "stop", serviceName).Run()
	})

	t.Logf("Calling StopInstance for '%s'", instanceName)
	err = instance.StopInstance(ctx, instanceName)
	if err != nil {
		t.Fatalf("StopInstance failed with unexpected error: %v", err)
	}

	// Give systemd a moment to update its state
	time.Sleep(1 * time.Second)

	t.Log("Verifying service is stopped...")
	isActive, err = isServiceActive(serviceName)
	if err != nil {
		t.Fatalf("Failed to check service status after stop: %v", err)
	}

	if isActive {
		t.Fatal("Service is still running after being stopped")
	}

	t.Logf("Service '%s' is successfully stopped.", serviceName)
}

func TestRestartInstance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Ensure the service is not running before the test.
	stopCmd := exec.Command("systemctl", "--user", "stop", serviceName)
	stopCmd.Run() // Don't check error, as it might already be stopped

	// Clean up after the test.
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

	// Give the service a moment to stabilize
	time.Sleep(1 * time.Second)

	// Call the restart function
	if err := instance.RestartInstance(ctx, instanceName); err != nil {
		t.Fatalf("RestartInstance failed unexpectedly: %v", err)
	}

	// Give the service a moment to finish restarting
	time.Sleep(1 * time.Second)

	// Verify the service is up after the restart.
	isActive, err := isServiceActive(serviceName)
	if err != nil {
		t.Fatalf("Failed to check service status after restart: %v", err)
	}
	if !isActive {
		t.Fatal("Service is not running after being restarted")
	}
}

// Helper function to check service status
func isServiceActive(serviceName string) (bool, error) {
	cmd := exec.Command("systemctl", "--user", "is-active", "--quiet", serviceName)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if exitError, ok := err.(*exec.ExitError); ok {
		// Inactive service
		if exitError.ExitCode() == 3 {
			return false, nil
		}
	}
	return false, err
}
