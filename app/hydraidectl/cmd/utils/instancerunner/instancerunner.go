package instancerunner

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancerunner/locker"
)

// logger is the package's logger. By default, it is a silent logger that discards all output.
// A custom logger can be set using the SetLogger function.
var logger *log.Logger

func init() {
	// By default instance runner is silent. Logs are discarded.
	logger = log.New(io.Discard, "", 0)
}

// SetupLogger allows a client to provide a custom logger for verbose output.
func SetupLogger(customLogger *log.Logger) {
	logger = customLogger
}

// InstanceController defines control operations for a HydrAIDE instances.
// The interface provides methods to start, stop, and restart a named service.
type InstanceController interface {
	// StartInstance starts the system service for the given instanceName.
	// It returns an error if the service file does not exist, or if the start command fails.
	StartInstance(ctx context.Context, instanceName string) error

	// StopInstance gracefully stops the system service for the given instanceName.
	// It issues a stop command and then actively polls the service status
	// to ensure it has fully shut down before returning.
	// A 5-second timeout is used to prevent indefinite waiting.
	StopInstance(ctx context.Context, instanceName string) error

	// RestartInstance performs a graceful stop followed by a start of the service.
	// It uses StopInstance and StartInstance methods to perform restart.
	RestartInstance(ctx context.Context, instanceName string) error
}

// systemdController implements InstanceController for Linux systems.
type systemdController struct{}

// StartInstance starts a systemd user service.
// This function acquires a file-based lock to ensure exclusive access to the instance.
func (c *systemdController) StartInstance(ctx context.Context, instance string) error {
	locker, err := locker.NewLocker(instance)
	if err != nil {
		return err
	}
	if err := locker.Lock(); err != nil {
		return fmt.Errorf("failed to lock instance '%s': %w", instance, err)
	}
	// Use defer to ensure the lock is always released when the function exits.
	defer locker.Unlock()
	return c.startInstanceOpe(ctx, instance)
}

// startInstanceOpe is the core logic for starting an instance, lock is held.
func (c *systemdController) startInstanceOpe(ctx context.Context, instance string) error {
	service := fmt.Sprintf("hydraserver-%s.service", instance)

	// Pre-flight check: ensure the service file exists before attempting to start it.
	exists, err := c.checkServiceExists(service)
	if err != nil {
		return fmt.Errorf("failed to check for service '%s' existence: %w", service, err)
	}
	if !exists {
		return fmt.Errorf("service '%s' not found", service)
	}

	// Get the required environment variables for the user session
	env, err := c.getUserSystemdEnv()
	if err != nil {
		return fmt.Errorf("failed to get user systemd environment: %w", err)
	}

	cmd := exec.CommandContext(ctx, "systemctl", "--user", "start", service)
	cmd.Env = env

	logger.Printf("Attempting to start service '%s'", service)

	err = cmd.Run()
	if err != nil {
		logger.Printf("failed to start service '%s'", service)
		return fmt.Errorf("failed to start service '%s': %w", service, err)
	}

	logger.Printf("Successfully started service '%s'", service)
	return nil
}

// StopInstance stops a systemd user service gracefully.
// This function acquires a file-based lock to ensure exclusive access to the instance.
func (c *systemdController) StopInstance(ctx context.Context, instance string) error {
	locker, err := locker.NewLocker(instance)
	if err != nil {
		return err
	}
	if err := locker.Lock(); err != nil {
		return fmt.Errorf("failed to lock instance '%s': %w", instance, err)
	}
	// Use defer to ensure the lock is always released when the function exits.
	defer locker.Unlock()

	return c.stopInstanceOpe(ctx, instance)
}

// stopInstanceOpe is the core logic for stopping an instance, lock is held.
func (c *systemdController) stopInstanceOpe(ctx context.Context, instance string) error {
	service := fmt.Sprintf("hydraserver-%s.service", instance)

	// Pre-flight check: ensure the service file exists.
	exists, err := c.checkServiceExists(service)
	if err != nil {
		return fmt.Errorf("failed to check for service '%s' existence: %w", service, err)
	}
	if !exists {
		return fmt.Errorf("service '%s' not found", service)
	}

	// Check if the service is already inactive.
	isActive, err := c.isServiceActive(service)
	if err != nil {
		logger.Printf("Failed to check status of '%s'", service)
		return fmt.Errorf("failed to check status of '%s': %w", service, err)
	}
	if !isActive {
		logger.Printf("Service '%s' is already stopped. No action needed.", service)
		return nil
	}

	cmd := exec.CommandContext(ctx, "systemctl", "--user", "stop", service)

	env, err := c.getUserSystemdEnv()
	if err != nil {
		return fmt.Errorf("failed to get user systemd environment: %w", err)
	}
	cmd.Env = env

	logger.Printf("Attempting to stop service '%s'", service)

	// Issue the stop command. The command itself doesn't wait for shutdown.
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to issue stop command for service '%s': %w", service, err)
	}

	// Poll the service status to ensure it has fully shut down.
	logger.Printf("Waiting for service '%s' to be fully stopped...", service)

	// Default to a 5-second timeout for graceful shutdown.
	// Todo: Configurable timeout
	pollingCtx, pollingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pollingCancel()

	ticker := time.NewTicker(200 * time.Millisecond) // Poll every 200ms
	defer ticker.Stop()

	for {
		select {
		case <-pollingCtx.Done():
			return fmt.Errorf("service '%s' did not stop gracefully within the timeout: %w", service, pollingCtx.Err())
		case <-ticker.C:
			isActive, checkErr := c.isServiceActive(service)
			if checkErr != nil {
				return fmt.Errorf("failed to check service status during graceful stop: %w", checkErr)
			}
			if !isActive {
				logger.Printf("Service '%s' is confirmed stopped.", service)
				return nil
			}
		}
	}
}

// RestartInstance stops and then starts a systemd user service instance.
func (c *systemdController) RestartInstance(ctx context.Context, instanceName string) error {
	logger.Printf("Attempting to restart instance '%s'...", instanceName)

	// Acquire the lock for the entire restart operation.
	locker, err := locker.NewLocker(instanceName)
	if err != nil {
		return err
	}
	if err := locker.Lock(); err != nil {
		return fmt.Errorf("failed to lock instance '%s': %w", instanceName, err)
	}
	defer locker.Unlock()

	// Stop the service gracefully.
	if err := c.stopInstanceOpe(ctx, instanceName); err != nil {
		return fmt.Errorf("failed to gracefully stop instance '%s' for restart: %w", instanceName, err)
	}

	// Start the service.
	if err := c.startInstanceOpe(ctx, instanceName); err != nil {
		return fmt.Errorf("failed to start instance '%s' after stop: %w", instanceName, err)
	}

	logger.Printf("Successfully restarted instance '%s'.", instanceName)
	return nil
}

// getUserSystemdEnv retrieves the necessary environment variables for a systemd user session.
func (c *systemdController) getUserSystemdEnv() ([]string, error) {
	cmd := exec.Command("systemctl", "--user", "show-environment")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run 'systemctl --user show-environment': %w", err)
	}

	envMap := make(map[string]string)
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// Build the final environment slice
	var finalEnv []string
	finalEnv = append(finalEnv, os.Environ()...) // Start with the existing environment
	if dbusAddress, ok := envMap["DBUS_SESSION_BUS_ADDRESS"]; ok {
		finalEnv = append(finalEnv, "DBUS_SESSION_BUS_ADDRESS="+dbusAddress)
	}
	if xdgRuntimeDir, ok := envMap["XDG_RUNTIME_DIR"]; ok {
		finalEnv = append(finalEnv, "XDG_RUNTIME_DIR="+xdgRuntimeDir)
	} else {
		// Fallback for XDG_RUNTIME_DIR if not found, though unlikely
		finalEnv = append(finalEnv, fmt.Sprintf("XDG_RUNTIME_DIR=/run/user/%d", os.Getuid()))
	}

	return finalEnv, nil
}

// checkServiceExists checks if a service file exists.
func (c *systemdController) checkServiceExists(serviceName string) (bool, error) {
	cmd := exec.Command("systemctl", "--user", "is-active", "--quiet", serviceName)
	err := cmd.Run()
	if err == nil {
		// Service is active, exists.
		return true, nil
	}
	if exitError, ok := err.(*exec.ExitError); ok {
		// Exit code 4 means the service unit does not exist.
		if exitError.ExitCode() == 4 {
			return false, nil
		}
		// service exists but is not running.
		return true, nil
	}
	return false, err
}

// isServiceActive checks if a service is currently active.
func (c *systemdController) isServiceActive(serviceName string) (bool, error) {
	cmd := exec.Command("systemctl", "--user", "is-active", "--quiet", serviceName)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if exitError, ok := err.(*exec.ExitError); ok {
		// Inactive service or unknown
		if exitError.ExitCode() == 3 || exitError.ExitCode() == 4 {
			return false, nil
		}
	}
	return false, err
}

// windowsController implements InstanceController for Windows.
type windowsController struct{}

func (c *windowsController) StartInstance(ctx context.Context, instance string) error {
	return nil
}

func (c *windowsController) StopInstance(ctx context.Context, instance string) error {
	return nil
}

func (c *windowsController) RestartInstance(ctx context.Context, instance string) error {
	return nil
}

// NewInstanceController returns the appropriate controller based on the OS.
// Currently, it supports "linux" and "windows". For any other OS, it returns nil.
func NewInstanceController() InstanceController {
	switch runtime.GOOS {
	case "windows":
		return &windowsController{}
	case "linux":
		return &systemdController{}
	default:
		return nil
	}
}
