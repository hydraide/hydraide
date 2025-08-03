package instancerunner

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Define a test instance name to use for creating temporary services.
var instanceName = "temporary-test-service"

// TestDefaultLoggerIsSilent verifies that the default logger uses the NoOpHandler
// and does not produce any output.
func TestDefaultLoggerIsSilent(t *testing.T) {
	if logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Default logger's Enabled method returned true, but should be silent")
	}
	if logger.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Default logger's Enabled method returned true, but should be silent")
	}
}

// TestSetupLogger verifies that the SetupLogger function successfully replaces the default logger.
func TestSetupLogger(t *testing.T) {
	// Create a buffer to capture the output of our custom logger.
	var buf bytes.Buffer
	customLogger := slog.New(slog.NewTextHandler(&buf, nil))

	originalLogger := logger
	t.Cleanup(func() {
		logger = originalLogger
	})

	// Set the custom logger using the public SetupLogger function.
	SetupLogger(customLogger)

	msg := "This is a test log message"
	key := "environment"
	value := "test"
	logger.Info(msg, key, value)

	// We check for the expected content without the timestamp, which is variable.
	expectedSubString := fmt.Sprintf("level=INFO msg=\"%s\" %s=%s", msg, key, value)
	output := buf.String()
	if !strings.Contains(output, expectedSubString) {
		t.Errorf("Expected logger output to contain '%s', but got '%s'", expectedSubString, output)
	}

	// We can also verify that the output starts with a timestamp.
	if !strings.HasPrefix(output, "time=") {
		t.Error("Expected logger output to start with a timestamp, but it didn't")
	}
}

// TestNoOpHandlerInterface checks that the NoOpHandler implements the slog.Handler interface.
func TestNoOpHandlerInterface(t *testing.T) {
	var handler slog.Handler = &NoOpHandler{}

	handler.Enabled(context.Background(), slog.LevelInfo)
	handler.Handle(context.Background(), slog.Record{})
	handler.WithAttrs([]slog.Attr{})
	handler.WithGroup("group")

	if h := handler.WithAttrs([]slog.Attr{}); h != handler {
		t.Errorf("WithAttrs did not return the same handler instance")
	}
	if h := handler.WithGroup("group"); h != handler {
		t.Errorf("WithGroup did not return the same handler instance")
	}
}

// TestStartInstance verifies that the StartInstance function correctly starts a service.
func TestStartInstance(t *testing.T) {
	// Setup service dynamically based on the OS.
	serviceName, err := createTestServiceFile()
	if err != nil {
		t.Fatalf("Failed to create service file: %v", err)
	}

	// Ensure the service is stopped and removed after the test.
	t.Cleanup(func() {
		stopCmd := exec.CommandContext(context.Background(), getStopCommand(), getStopArgs(serviceName)...)
		stopCmd.Run()
		removeTestServiceFile()
	})

	instance := NewInstanceController()
	if _, ok := instance.(*systemdController); !ok && runtime.GOOS == "linux" {
		t.Fatal("Expected systemdController for Linux, got a different type")
	}
	if _, ok := instance.(*windowsController); !ok && runtime.GOOS == "windows" {
		t.Fatal("Expected windowsController for Windows, got a different type")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Logf("Calling StartInstance for '%s' on %s", serviceName, runtime.GOOS)
	err = instance.StartInstance(ctx, instanceName)
	if err != nil {
		t.Fatalf("StartInstance failed with error: %v", err)
	}

	// Give the service a moment to update its state.
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

// TestStopInstance verifies that the StopInstance function correctly stops a running service.
func TestStopInstance(t *testing.T) {
	// Setup service dynamically based on the OS.
	serviceName, err := createTestServiceFile()
	if err != nil {
		t.Fatalf("Failed to create service file: %v", err)
	}

	// Ensure the service is stopped and removed after the test.
	t.Cleanup(func() {
		stopCmd := exec.CommandContext(context.Background(), getStopCommand(), getStopArgs(serviceName)...)
		stopCmd.Run()
		removeTestServiceFile()
	})

	instance := NewInstanceController(WithTimeout(10 * time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Ensure the service is running before attempting to stop.
	if err := instance.StartInstance(ctx, instanceName); err != nil {
		t.Fatalf("Pre-start failed: %v", err)
	}
	time.Sleep(1 * time.Second) // Wait for complete start before proceeding.

	t.Logf("Calling StopInstance for '%s' on %s", instanceName, runtime.GOOS)
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

// TestRestartInstance verifies that RestartInstance correctly stops and starts a service.
func TestRestartInstance(t *testing.T) {
	// Setup service dynamically based on the OS.
	serviceName, err := createTestServiceFile()
	if err != nil {
		t.Fatalf("Failed to create service file: %v", err)
	}

	// Ensure the service is stopped and removed after the test.
	t.Cleanup(func() {
		stopCmd := exec.CommandContext(context.Background(), getStopCommand(), getStopArgs(serviceName)...)
		stopCmd.Run()
		removeTestServiceFile()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Ensure the service is not running before the test.
	exec.CommandContext(context.Background(), getStopCommand(), getStopArgs(serviceName)...).Run()

	instance := NewInstanceController()
	// Start the service once to ensure it can be started
	if err := instance.StartInstance(ctx, instanceName); err != nil {
		t.Fatalf("Pre-start failed: %v", err)
	}
	time.Sleep(1 * time.Second)

	t.Logf("Calling RestartInstance for '%s' on %s", instanceName, runtime.GOOS)
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

// TestMissingService verifies that an error is returned when a non-existent service is requested.
func TestMissingService(t *testing.T) {
	instance := NewInstanceController()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	missingInstanceName := "non-existent-test-service"
	t.Logf("Testing StartInstance with a missing service '%s' on %s", missingInstanceName, runtime.GOOS)
	err := instance.StartInstance(ctx, missingInstanceName)
	if err == nil {
		t.Fatal("Expected an error for a missing service, but got none")
	}

	// Verify the error message matches the expected format for the given OS.
	var expectedError string
	switch runtime.GOOS {
	case "linux":
		expectedError = fmt.Sprintf("service 'hydraserver-%s.service' not found", missingInstanceName)
	case "windows":
		expectedError = fmt.Sprintf("service 'hydraserver-%s' not found", missingInstanceName)
	}

	if err.Error() != expectedError {
		t.Fatalf("Expected error '%s', but got '%s'", expectedError, err.Error())
	}
}

// isServiceActive is a helper function to check if a service is currently active,
// adapting the command based on the OS.
func isServiceActive(serviceName string) (bool, error) {
	switch runtime.GOOS {
	case "linux":
		cmd := exec.Command("systemctl", "is-active", "--quiet", serviceName)
		err := cmd.Run()
		if err == nil {
			return true, nil
		}
		if exitError, ok := err.(*exec.ExitError); ok {
			if exitError.ExitCode() == 3 || exitError.ExitCode() == 4 {
				return false, nil
			}
		}
		return false, err
	case "windows":
		// Use powershell to get service status.
		// 'Get-Service' returns a non-zero exit code if the service does not exist.
		cmd := exec.Command("powershell", "-Command", fmt.Sprintf("(Get-Service '%s').Status -eq 'Running'", serviceName))
		output, err := cmd.Output()
		if err != nil {
			// Service not found or some other error occurred.
			return false, nil
		}
		return strings.TrimSpace(string(output)) == "True", nil
	}
	return false, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
}

// createTestServiceFile creates a dummy service file/entry for testing purposes,
// adapting the method based on the OS.
func createTestServiceFile() (string, error) {
	switch runtime.GOOS {
	case "linux":
		return createLinuxTestServiceFile()
	case "windows":
		return createWindowsTestServiceFile()
	}
	return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
}

// createLinuxTestServiceFile creates a dummy systemd user service file.
func createLinuxTestServiceFile() (string, error) {

	// Replace ExecStart with actual hydraide service binpath and workdir for E2E test.
	content := `
		[Unit]
		Description=HydrAIDE Test Service

		[Service]
		ExecStart=/bin/bash -c "while true; do echo 'Service running...'; sleep 1; done"
		Restart=always
		
		[Install]
		WantedBy=default.target
	`

	serviceDir := filepath.Join("/etc", "systemd", "system")
	serviceFile := fmt.Sprintf("hydraserver-%s.service", instanceName)
	fullPath := filepath.Join(serviceDir, serviceFile)

	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create service directory: %w", err)
	}

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write service file: %w", err)
	}

	cmd := exec.Command("systemctl", "daemon-reload")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to run 'systemctl daemon-reload': %w", err)
	}

	return serviceFile, nil
}

// createWindowsTestServiceFile creates a dummy Windows service using sc.exe.
func createWindowsTestServiceFile() (string, error) {
	serviceName := fmt.Sprintf("hydraserver-%s", instanceName)

	var cmd *exec.Cmd
	if checkNssm() {
		cmd = exec.Command("nssm", "install", serviceName, "cmd.exe", "/c", "ping 127.0.0.1 -n 60 >nul")
	} else {
		cmd = exec.Command("sc", "create", serviceName, "binPath=", `cmd.exe /c "ping 127.0.0.1 -n 60 >nul"`, "start=", "demand")
	}
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to create Windows service '%s': %w", serviceName, err)
	}
	return serviceName, nil
}

// removeTestServiceFile stops the service and removes the service entry.
func removeTestServiceFile() {
	switch runtime.GOOS {
	case "linux":
		removeLinuxTestServiceFile()
	case "windows":
		removeWindowsTestServiceFile()
	}
}

// removeLinuxTestServiceFile removes a dummy systemd user service file.
func removeLinuxTestServiceFile() {
	serviceFile := fmt.Sprintf("hydraserver-%s.service", instanceName)
	fullPath := filepath.Join("/etc", "systemd", "system", serviceFile)
	exec.Command("systemctl", "stop", serviceFile).Run()
	os.Remove(fullPath)
	exec.Command("systemctl", "daemon-reload").Run()
}

// removeWindowsTestServiceFile removes a dummy Windows service.
func removeWindowsTestServiceFile() {
	serviceName := fmt.Sprintf("hydraserver-%s", instanceName)
	if checkNssm() {
		exec.Command("nssm", "stop", serviceName).Run()
		exec.Command("nssm", "remove", serviceName, "confirm").Run()
	} else {
		exec.Command("sc", "stop", serviceName).Run()
		exec.Command("sc", "delete", serviceName).Run()
	}
}

// getStopCommand returns the appropriate command for stopping a service based on the OS.
func getStopCommand() string {
	if runtime.GOOS == "linux" {
		return "systemctl"
	}
	return "sc"
}

func getStopArgs(serviceName string) []string {
	return []string{"stop", serviceName}
}

func checkNssm() bool {
	_, err := exec.LookPath("nssm.exe")
	return err == nil
}
