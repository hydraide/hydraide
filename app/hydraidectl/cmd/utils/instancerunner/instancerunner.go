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
	service := fmt.Sprintf("hydraserver-%s.service", instance)

	// Pre-flight check: ensure the service file exists before attempting to start it.
	exists, err := c.checkServiceExists(service)
	if err != nil {
		return fmt.Errorf("failed to check for service '%s' existence: %w", service, err)
	}
	if !exists {
		return fmt.Errorf("service '%s' not found", service)
	}

	locker, err := locker.NewLocker(instance)
	if err != nil {
		return err
	}
	if err := locker.Lock(); err != nil {
		return fmt.Errorf("failed to lock instance '%s': %w", instance, err)
	}
	// Use defer to ensure the lock is always released when the function exits.
	defer locker.Unlock()
	return c.startInstanceOpe(ctx, service)
}

// startInstanceOpe is the core logic for starting an instance, lock is held.
func (c *systemdController) startInstanceOpe(ctx context.Context, service string) error {

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
	service := fmt.Sprintf("hydraserver-%s.service", instance)

	// Pre-flight check: ensure the service file exists.
	exists, err := c.checkServiceExists(service)
	if err != nil {
		return fmt.Errorf("failed to check for service '%s' existence: %w", service, err)
	}
	if !exists {
		return fmt.Errorf("service '%s' not found", service)
	}

	locker, err := locker.NewLocker(instance)
	if err != nil {
		return err
	}
	if err := locker.Lock(); err != nil {
		return fmt.Errorf("failed to lock instance '%s': %w", instance, err)
	}
	// Use defer to ensure the lock is always released when the function exits.
	defer locker.Unlock()

	return c.stopInstanceOpe(ctx, service)
}

// stopInstanceOpe is the core logic for stopping an instance, lock is held.
func (c *systemdController) stopInstanceOpe(ctx context.Context, service string) error {

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

	service := fmt.Sprintf("hydraserver-%s.service", instanceName)

	// Pre-flight check: ensure the service file exists.
	exists, err := c.checkServiceExists(service)
	if err != nil {
		return fmt.Errorf("failed to check for service '%s' existence: %w", service, err)
	}
	if !exists {
		return fmt.Errorf("service '%s' not found", service)
	}

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
	if err := c.stopInstanceOpe(ctx, service); err != nil {
		return fmt.Errorf("failed to gracefully stop instance '%s' for restart: %w", instanceName, err)
	}

	// Start the service.
	if err := c.startInstanceOpe(ctx, service); err != nil {
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

// windowsController implements InstanceController for Windows via NSSM.
type windowsController struct {
	useNssm bool
}

// StartInstance installs (if needed) and starts an NSSM-wrapped service.
//
// It acquires the same file-based lock, then checks for existence via `nssm status`.
// If missing, returns an error. Otherwise it calls `nssm start`.
func (c *windowsController) StartInstance(ctx context.Context, instance string) error {
	service := fmt.Sprintf("hydraserver-%s", instance)

	// Check if the service exists before attempting to start.
	exists, err := c.checkServiceExists(ctx, service)
	if err != nil {
		return fmt.Errorf("failed to check for service '%s' existence: %w", service, err)
	}
	if !exists {
		return fmt.Errorf("service '%s' not found", service)
	}

	locker, err := locker.NewLocker(instance)
	if err != nil {
		return err
	}
	if err := locker.Lock(); err != nil {
		return fmt.Errorf("failed to lock instance '%s': %w", instance, err)
	}
	// Use defer to ensure the lock is always released when the function exits.
	defer locker.Unlock()

	return c.startInstanceOp(ctx, service)
}

func (c *windowsController) startInstanceOp(ctx context.Context, service string) error {
	logger.Printf("[windows] Attempting to start service '%s'", service)
	var cmd *exec.Cmd
	if c.useNssm {
		cmd = exec.CommandContext(ctx, "nssm", "start", service)
	} else {
		cmd = exec.CommandContext(ctx, "powershell", "-Command", fmt.Sprintf("Start-Service -Name '%s'", service))
	}

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to start service '%s': %w", service, err)
	}

	logger.Printf("[windows] Successfully started service '%s'", service)
	return nil
}

// StopInstance stops an NSSM service and then polls until it’s confirmed stopped.
func (c *windowsController) StopInstance(ctx context.Context, instance string) error {
	service := fmt.Sprintf("hydraserver-%s", instance)

	exists, err := c.checkServiceExists(ctx, service)
	if err != nil {
		return fmt.Errorf("failed to check for service '%s': %w", service, err)
	}
	if !exists {
		return fmt.Errorf("service '%s' not found", service)
	}

	locker, err := locker.NewLocker(instance)
	if err != nil {
		return err
	}
	if err := locker.Lock(); err != nil {
		return fmt.Errorf("failed to lock instance '%s': %w", instance, err)
	}
	defer locker.Unlock()

	return c.stopInstanceOp(ctx, service)
}

func (c *windowsController) stopInstanceOp(ctx context.Context, service string) error {
	// If already stopped, nothing to do
	running, err := c.isServiceRunning(ctx, service)
	if err != nil {
		return fmt.Errorf("status check failed for '%s': %w", service, err)
	}
	if !running {
		logger.Printf("[windows] Service '%s' already stopped", service)
		return nil
	}

	logger.Printf("[windows] Stopping NSSM service '%s'", service)
	cmd := exec.CommandContext(ctx, "nssm", "stop", service)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nssm stop failed: %v\n%s", err, out)
	}

	// Poll until stopped or timeout
	timeout := 5 * time.Second
	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-pollCtx.Done():
			return fmt.Errorf("service '%s' did not stop within %s: %w", service, timeout, pollCtx.Err())
		case <-tick.C:
			running, err := c.isServiceRunning(ctx, service)
			if err != nil {
				return fmt.Errorf("status check failed during stop poll: %w", err)
			}
			if !running {
				logger.Printf("[windows] Service '%s' confirmed stopped", service)
				return nil
			}
		}
	}
}

// RestartInstance simply chains StopInstance then StartInstance under one lock.
func (c *windowsController) RestartInstance(ctx context.Context, instance string) error {
	logger.Printf("[windows] Restarting instance '%s'", instance)

	locker, err := locker.NewLocker(instance)
	if err != nil {
		return err
	}
	if err := locker.Lock(); err != nil {
		return fmt.Errorf("failed to lock instance '%s': %w", instance, err)
	}
	defer locker.Unlock()

	service := fmt.Sprintf("hydraserver-%s", instance)
	if err := c.stopInstanceOp(ctx, service); err != nil {
		return fmt.Errorf("failed to stop for restart: %w", err)
	}
	if err := c.startInstanceOp(ctx, service); err != nil {
		return fmt.Errorf("failed to start after stop: %w", err)
	}
	logger.Printf("[windows] Successfully restarted '%s'", instance)
	return nil
}

// checkServiceExists checks if a Windows service exists using NSSM or PowerShell.
func (c *windowsController) checkServiceExists(ctx context.Context, service string) (bool, error) {
	if c.useNssm {
		// `nssm status` exits with code 2 if the service is not installed.
		cmd := exec.CommandContext(ctx, "nssm", "status", service)
		err := cmd.Run()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 {
				return false, nil // Service not installed
			}
			return false, fmt.Errorf("nssm check failed: %w", err)
		}
		return true, nil
	} else {
		// Use PowerShell to check for the service.
		cmd := exec.CommandContext(ctx, "powershell", "-Command", fmt.Sprintf("Get-Service -Name '%s'", service))
		err := cmd.Run()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
				return false, nil // Service not found
			}
			return false, fmt.Errorf("powershell get-service failed: %w", err)
		}
		return true, nil
	}
}

// isServiceRunning returns true if the Windows service is running.
func (c *windowsController) isServiceRunning(ctx context.Context, service string) (bool, error) {
	if c.useNssm {
		// `nssm status` exits with code 0 if running.
		cmd := exec.CommandContext(ctx, "nssm", "status", service)
		err := cmd.Run()
		return err == nil, nil
	} else {
		// Use PowerShell to check the service status.
		cmd := exec.CommandContext(ctx, "powershell", "-Command", fmt.Sprintf("(Get-Service -Name '%s').Status -eq 'Running'", service))
		output, err := cmd.Output()
		if err != nil {
			// This can happen if the service is not found, in which case it's not running.
			return false, nil
		}
		return strings.TrimSpace(string(output)) == "True", nil
	}
}

// checkNssmExists checks if nssm.exe is available in the system's PATH.
func checkNssmExists() bool {
	_, err := exec.LookPath("nssm.exe")
	return err == nil
}

// NewInstanceController returns the appropriate controller based on the OS.
// Currently, it supports "linux" and "windows". For any other OS, it returns nil.
func NewInstanceController() InstanceController {
	switch runtime.GOOS {
	case "windows":
		useNssm := checkNssmExists()
		if useNssm {
			logger.Println("NSSM found. Using NSSM for Windows service management.")
		} else {
			logger.Println("NSSM not found. Falling back to PowerShell for Windows service management.")
		}
		return &windowsController{useNssm: useNssm}
	case "linux":
		return &systemdController{}
	default:
		return nil
	}
}
