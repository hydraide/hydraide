package instancerunner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/locker"
)

// SERVICE_STATUS_TICKER is time interval to check status of service
// after start/stop instance is initiated.
const SERVICE_STATUS_TICKER = 300 * time.Millisecond

// NoOpHandler is an slog.Handler that discards all log records.
type NoOpHandler struct{}

func (h *NoOpHandler) Enabled(_ context.Context, _ slog.Level) bool  { return false }
func (h *NoOpHandler) Handle(_ context.Context, _ slog.Record) error { return nil }
func (h *NoOpHandler) WithAttrs(_ []slog.Attr) slog.Handler          { return h }
func (h *NoOpHandler) WithGroup(_ string) slog.Handler               { return h }

// logger is the package's structured logger. By default, it is a silent logger that discards all output.
// A custom logger can be set using the SetLogger function.
var logger *slog.Logger

func init() {
	// By default instance runner is silent. Logs are discarded.
	logger = slog.New(&NoOpHandler{})
}

// SetupLogger allows a client to provide a custom structured logger for verbose output.
func SetupLogger(customLogger *slog.Logger) {
	logger = customLogger
}

// InstanceController defines control operations for HydrAIDE instances.
// The interface provides methods to start, stop, and restart a named service,
// abstracting the underlying operating system's service management.
type InstanceController interface {
	// StartInstance starts the system service for the given instanceName.
	//
	// It performs a pre-flight check to ensure the service file exists and
	// that the service is not already running. The function will block
	// until the service is confirmed to be active or the operation times out.
	//
	// It returns the following errors:
	//  - ErrServiceNotFound: if the service file does not exist on the system.
	//  - ErrServiceAlreadyRunning: if a start command is issued for a
	//    service that is already active.
	//  - CmdError: if a low-level command (e.g., `systemctl start`) fails.
	//  - OperationError: a high-level error wrapping a low-level issue,
	//    providing context about the instance and operation.
	StartInstance(ctx context.Context, instanceName string) error

	// StopInstance gracefully stops the system service for the given instanceName.
	//
	// The function issues a stop command and then actively polls the service status
	// to ensure it has fully shut down before returning. It respects the
	// context's deadline for the overall operation and the `gracefulShutdownTimeout`
	// for polling.
	//
	// It returns the following errors:
	//  - ErrServiceNotFound: if the service file does not exist.
	//  - ErrServiceNotRunning: if a stop command is issued for a service that is not active.
	//  - CmdError: if a low-level command (e.g., `systemctl stop`) fails.
	//  - OperationError: a high-level error wrapping a low-level issue.
	StopInstance(ctx context.Context, instanceName string) error

	// RestartInstance performs a graceful stop followed by a start of the service.
	//
	// The function uses the `StopInstance` and `StartInstance` methods to perform the restart.
	// If the service is not running, it will simply perform a start operation.
	// The entire operation is governed by the context's deadline.
	//
	// It returns the following errors:
	//  - ErrServiceNotFound: if the service file does not exist.
	//  - CmdError: if any low-level command fails during the stop or start phases.
	//  - OperationError: a high-level error wrapping a low-level issue.
	RestartInstance(ctx context.Context, instanceName string) error

	// InstanceExists checks if the service file for a given instance exists on the system.
	// This function is intended for quick pre-flight checks in a CLI for better user experience.
	// It returns a boolean and an error if the check itself fails
	//
	//  - CmdError: if any low-level command fails during the stop or start phases.
	InstanceExists(ctx context.Context, instanceName string) (bool, error)
}

// systemdController implements InstanceController for Linux systems.
type systemdController struct {
	timeout                  time.Duration
	gracefulStartStopTimeout time.Duration
}

// StartInstance starts a systemd user service.
// This function acquires a file-based lock to ensure exclusive access to the instance.
func (c *systemdController) StartInstance(ctx context.Context, instance string) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	service := fmt.Sprintf("hydraserver-%s.service", instance)

	// Pre-flight check: ensure the service file exists before attempting to start it.
	exists, err := c.checkServiceExists(service)
	if err != nil {
		return NewOperationError(instance, "check service existence", err)
	}
	if !exists {
		return ErrServiceNotFound
	}

	// Check if the service is already active.
	isActive, err := c.isServiceActive(service)
	if err != nil {
		return NewOperationError(instance, "check service status", err)
	}
	if isActive {
		logger.Info("Service is already running. No action needed.", "service_name", service)
		return ErrServiceAlreadyRunning
	}

	locker, err := locker.NewLocker(instance)
	if err != nil {
		logger.Error("Failed to get Instance Locker")
		return NewOperationError(instance, "get locker", err)
	}
	if err := locker.Lock(); err != nil {
		return NewOperationError(instance, "lock instance", err)
	}
	logger.Debug("Locked instance", "instance_name", instance)
	// Use defer to ensure the lock is always released when the function exits.
	defer locker.Unlock()
	return c.startInstanceOp(ctx, service)
}

// startInstanceOpe is the core logic for starting an instance, lock is held.
func (c *systemdController) startInstanceOp(ctx context.Context, service string) error {

	isEnabledCmd := exec.CommandContext(ctx, "systemctl", "is-enabled", "--quiet", service)
	if err := isEnabledCmd.Run(); err != nil {
		// If the service is not enabled, we enable it.
		logger.Info("Enable Service", "service_name", service)
		enableCmd := exec.CommandContext(ctx, "systemctl", "enable", service)
		if err := enableCmd.Run(); err != nil {
			return NewOperationError(service, "enable service", &CmdError{
				Command: "systemctl enable",
				Output:  "",
				Err:     err,
			})
		}
	}

	cmd := exec.CommandContext(ctx, "systemctl", "start", service)

	logger.Info("Attempting to start service", "service_name", service)

	err := cmd.Run()
	if err != nil {
		logger.Info("failed to start service", "service_name", service)
		return NewOperationError(service, "start service", NewCmdError("systemctl start", "", err))
	}

	logger.Info("Waiting for service to be fully started", "service_name", service)

	// poll until successfully started or timeout
	pollingCtx, pollingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pollingCancel()
	logger.Debug("Created polling with timeout context to check service status")

	ticker := time.NewTicker(SERVICE_STATUS_TICKER)
	defer ticker.Stop()

	for {
		select {
		case <-pollingCtx.Done():
			return NewOperationError(service, "graceful start", pollingCtx.Err())
		case <-ticker.C:
			isActive, checkErr := c.isServiceActive(service)
			if checkErr != nil {
				return NewOperationError(service, "check status during graceful start", checkErr)
			}
			if isActive {
				logger.Info("Service is confirmed started.", "service_name", service)
				return nil
			}
		}
	}
}

// StopInstance stops a systemd user service gracefully.
// This function acquires a file-based lock to ensure exclusive access to the instance.
func (c *systemdController) StopInstance(ctx context.Context, instance string) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	service := fmt.Sprintf("hydraserver-%s.service", instance)
	logger.Debug("Stop Instance")
	// Pre-flight check: ensure the service file exists.
	exists, err := c.checkServiceExists(service)
	if err != nil {
		return NewOperationError(instance, "check service existence", err)
	}
	if !exists {
		return ErrServiceNotFound
	}

	// Check if the service is already inactive.
	isActive, err := c.isServiceActive(service)
	if err != nil {
		logger.Info("Failed to check status of", "service_name", service)
		return NewOperationError(service, "check service status", err)
	}
	if !isActive {
		logger.Info("Service is already stopped. No action needed.", "service_name", service)
		return ErrServiceNotRunning
	}

	locker, err := locker.NewLocker(instance)
	if err != nil {
		logger.Error("Failed to get instance locker")
		return NewOperationError(instance, "Acquire lock", err)
	}
	if err := locker.Lock(); err != nil {
		return fmt.Errorf("failed to lock instance '%s': %w", instance, err)
	}
	logger.Debug("File lock instance", "instance_name", instance)
	// Use defer to ensure the lock is always released when the function exits.
	defer locker.Unlock()

	return c.stopInstanceOp(ctx, service)
}

// stopInstanceOpe is the core logic for stopping an instance, lock is held.
func (c *systemdController) stopInstanceOp(ctx context.Context, service string) error {

	cmd := exec.CommandContext(ctx, "systemctl", "stop", service)

	logger.Info("Attempting to stop service", "service_name", service)

	// Issue the stop command. The command itself doesn't wait for shutdown.
	if err := cmd.Run(); err != nil {
		return NewOperationError(service, "stop service", &CmdError{
			Command: "systemctl stop",
			Output:  "",
			Err:     err,
		})
	}

	// Poll the service status to ensure it has fully shut down.
	logger.Info("Waiting for service to be fully stopped...", "service_name", service)

	pollingCtx, pollingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pollingCancel()
	logger.Debug("Created polling with timeout context to check service status")

	ticker := time.NewTicker(SERVICE_STATUS_TICKER)
	defer ticker.Stop()

	for {
		select {
		case <-pollingCtx.Done():
			return NewOperationError(service, "graceful stop", pollingCtx.Err())
		case <-ticker.C:
			isActive, checkErr := c.isServiceActive(service)
			if checkErr != nil {
				return NewOperationError(service, "check status during graceful stop", checkErr)
			}
			if !isActive {
				logger.Info("Service is confirmed stopped.", "service_name", service)
				return nil
			}
		}
	}
}

// RestartInstance stops and then starts a systemd user service instance.
func (c *systemdController) RestartInstance(ctx context.Context, instanceName string) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	logger.Info("Attempting to restart instance", "instance_name", instanceName)

	service := fmt.Sprintf("hydraserver-%s.service", instanceName)

	// Pre-flight check: ensure the service file exists.
	exists, err := c.checkServiceExists(service)
	if err != nil {
		return NewOperationError(instanceName, "check service existence", err)
	}
	if !exists {
		return ErrServiceNotFound
	}

	// Acquire the lock for the entire restart operation.
	locker, err := locker.NewLocker(instanceName)
	if err != nil {
		logger.Error("Failed to get instance locker")
		return NewOperationError(instanceName, "acquire locker", err)
	}
	if err := locker.Lock(); err != nil {
		return NewOperationError(instanceName, "lock instance", err)
	}
	defer locker.Unlock()

	logger.Info("Performing restart of the instance", "instance_name", instanceName)
	// Stop the service gracefully.
	logger.Debug("Stop Instance", "instance_name", instanceName)
	if err := c.stopInstanceOp(ctx, service); err != nil {
		return NewOperationError(instanceName, "stop for restart", err)
	}

	// Start the service.
	logger.Debug("Start Instance", "instance_name", instanceName)
	if err := c.startInstanceOp(ctx, service); err != nil {
		return NewOperationError(instanceName, "start after stop", err)
	}

	logger.Info("Successfully restarted instance", "instance_name", instanceName)
	return nil
}

// InstanceExists checks if the service file for a given instance exists on the system.
func (c *systemdController) InstanceExists(ctx context.Context, instanceName string) (bool, error) {
	service := fmt.Sprintf("hydraserver-%s.service", instanceName)
	return c.checkServiceExists(service)
}

// checkServiceExists checks if a service file exists.
func (c *systemdController) checkServiceExists(serviceName string) (bool, error) {
	logger.Debug("Check service exists..")
	cmd := exec.Command("systemctl", "is-active", "--quiet", serviceName)
	err := cmd.Run()
	if err == nil {
		// Service is active, exists.
		return true, nil
	}
	if exitError, ok := err.(*exec.ExitError); ok {
		// Exit code 4 means the service does not exist.
		if exitError.ExitCode() == 4 {
			return false, nil
		}
		// service exists but is not running.
		return true, nil
	}
	return false, NewCmdError("systemctl is-active", "service exist check", err)
}

// isServiceActive checks if a service is currently active.
func (c *systemdController) isServiceActive(serviceName string) (bool, error) {
	logger.Debug("Check service active", "service_name", serviceName)
	cmd := exec.Command("systemctl", "is-active", "--quiet", serviceName)
	err := cmd.Run()
	if err == nil {
		logger.Debug("Service is active!")
		return true, nil
	}
	if exitError, ok := err.(*exec.ExitError); ok {
		// Inactive service or unknown
		if exitError.ExitCode() == 3 || exitError.ExitCode() == 4 {
			logger.Debug("Service is inactive")
			return false, nil
		}
	}
	logger.Debug("Service is inactive")
	return false, err
}

// windowsController implements InstanceController for Windows via NSSM.
type windowsController struct {
	useNssm                  bool
	timeout                  time.Duration
	gracefulStartStopTimeout time.Duration
}

// StartInstance installs (if needed) and starts an NSSM-wrapped service.
//
// It acquires the same file-based lock, then checks for existence via `nssm status`.
// If missing, returns an error. Otherwise it calls `nssm start`.
func (c *windowsController) StartInstance(ctx context.Context, instance string) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	service := fmt.Sprintf("hydraserver-%s", instance)

	// Check if the service exists before attempting to start.
	exists, err := c.checkServiceExists(ctx, service)
	if err != nil {
		return NewOperationError(instance, "check service existence", err)
	}
	if !exists {
		return ErrServiceNotFound
	}

	locker, err := locker.NewLocker(instance)
	if err != nil {
		return NewOperationError(instance, "acquire locker", err)
	}
	if err := locker.Lock(); err != nil {
		return NewOperationError(instance, "lock instance", err)
	}
	// Use defer to ensure the lock is always released when the function exits.
	defer locker.Unlock()

	return c.startInstanceOp(ctx, service)
}

func (c *windowsController) startInstanceOp(ctx context.Context, service string) error {
	running, err := c.isServiceRunning(ctx, service)
	if err != nil {
		return NewOperationError(service, "check service status", err)
	}
	if running {
		logger.Info("[windows] Service already running", "service_name", service)
		return ErrServiceAlreadyRunning
	}

	logger.Info("[windows] Attempting to start service", "service_name", service)
	var cmd *exec.Cmd
	if c.useNssm {
		cmd = exec.CommandContext(ctx, "nssm", "start", service)
	} else {
		cmd = exec.CommandContext(ctx, "powershell", "-Command", fmt.Sprintf("Start-Service -Name '%s'", service))
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return NewOperationError(service, "start service", &CmdError{
			Command: fmt.Sprintf("start service for '%s'", service),
			Output:  string(out),
			Err:     err,
		})
	}

	// Poll until started or timeout
	timeout := c.gracefulStartStopTimeout
	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	tick := time.NewTicker(SERVICE_STATUS_TICKER)
	defer tick.Stop()

	for {
		select {
		case <-pollCtx.Done():
			return NewOperationError(service, "graceful start", pollCtx.Err())
		case <-tick.C:
			running, err := c.isServiceRunning(ctx, service)
			if err != nil {
				return NewOperationError(service, "status check during start poll", err)
			}
			if running {
				logger.Info("[windows] Service confirmed started", "service_name", service)
				return nil
			}
		}
	}
}

// StopInstance stops an NSSM service and then polls until it’s confirmed stopped.
func (c *windowsController) StopInstance(ctx context.Context, instance string) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	service := fmt.Sprintf("hydraserver-%s", instance)

	exists, err := c.checkServiceExists(ctx, service)
	if err != nil {
		return NewOperationError(instance, "check service existence", err)
	}
	if !exists {
		return ErrServiceNotFound
	}

	locker, err := locker.NewLocker(instance)
	if err != nil {
		return NewOperationError(instance, "acquire locker", err)
	}
	if err := locker.Lock(); err != nil {
		return NewOperationError(instance, "lock instance", err)
	}
	defer locker.Unlock()

	return c.stopInstanceOp(ctx, service)
}

func (c *windowsController) stopInstanceOp(ctx context.Context, service string) error {
	// If already stopped, nothing to do
	running, err := c.isServiceRunning(ctx, service)
	if err != nil {
		return NewOperationError(service, "check service status", err)
	}
	if !running {
		logger.Info("[windows] Service already stopped", "service_name", service)
		return ErrServiceNotRunning
	}

	logger.Info("[windows] Stopping NSSM service", "service_name", service)
	var cmd *exec.Cmd
	if c.useNssm {
		cmd = exec.CommandContext(ctx, "nssm", "stop", service)
	} else {
		cmd = exec.CommandContext(ctx, "powershell", "-Command", fmt.Sprintf("Stop-Service -Name '%s'", service))
	}

	if out, err := cmd.CombinedOutput(); err != nil {
		return NewOperationError(service, "stop service", &CmdError{
			Command: fmt.Sprintf("stop service for '%s'", service),
			Output:  string(out),
			Err:     err,
		})
	}

	// Poll until stopped or timeout
	timeout := c.gracefulStartStopTimeout
	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	tick := time.NewTicker(SERVICE_STATUS_TICKER)
	defer tick.Stop()

	for {
		select {
		case <-pollCtx.Done():
			return NewOperationError(service, "graceful stop", pollCtx.Err())
		case <-tick.C:
			running, err := c.isServiceRunning(ctx, service)
			if err != nil {
				return NewOperationError(service, "status check during stop poll", err)
			}
			if !running {
				logger.Info("[windows] Service confirmed stopped", "service_name", service)
				return nil
			}
		}
	}
}

// RestartInstance simply chains StopInstance then StartInstance under one lock.
func (c *windowsController) RestartInstance(ctx context.Context, instance string) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	logger.Info("[windows] Restarting instance", "instance_name", instance)

	locker, err := locker.NewLocker(instance)
	if err != nil {
		return NewOperationError(instance, "acquire locker", err)
	}
	if err := locker.Lock(); err != nil {
		return NewOperationError(instance, "lock instance", err)
	}
	defer locker.Unlock()

	service := fmt.Sprintf("hydraserver-%s", instance)
	if err := c.stopInstanceOp(ctx, service); err != nil && !errors.Is(err, ErrServiceNotRunning) {
		return NewOperationError(instance, "stop for restart", err)
	}
	if err := c.startInstanceOp(ctx, service); err != nil {
		return NewOperationError(instance, "start after stop", err)
	}
	logger.Info("[windows] Successfully restarted", "service_name", instance)
	return nil
}

// InstanceExists checks if the service file for a given instance exists on the system.
func (c *windowsController) InstanceExists(ctx context.Context, instanceName string) (bool, error) {
	service := fmt.Sprintf("hydraserver-%s", instanceName)
	return c.checkServiceExists(ctx, service)
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
			return false, &CmdError{
				Command: "nssm status",
				Err:     err,
			}
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
			return false, &CmdError{
				Command: "powershell Get-Service",
				Err:     err,
			}
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
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 {
				return false, ErrServiceNotFound
			}
			return false, &CmdError{
				Command: "nssm status",
				Err:     err,
			}
		}
		return err == nil, nil
	} else {
		// Use PowerShell to check the service status.
		cmd := exec.CommandContext(ctx, "powershell", "-Command", fmt.Sprintf("(Get-Service -Name '%s').Status -eq 'Running'", service))
		output, err := cmd.Output()
		if err != nil {
			// If the service is not found, it's not running, and we can return ErrServiceNotFound.
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
				return false, ErrServiceNotFound
			}
			return false, &CmdError{
				Command: "powershell Get-Service",
				Output:  string(output),
				Err:     err,
			}
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
// Take timeout in seconds to time out start/stop/restart operations.
func NewInstanceController(options ...Option) InstanceController {

	// timeout defaults to 5 seconds
	cfg := &opts{timeout: 20 * time.Second, startStopTimout: 10 * time.Second}
	for _, option := range options {
		option(cfg)
	}

	switch runtime.GOOS {
	case "windows":
		useNssm := checkNssmExists()
		if useNssm {
			logger.Info("NSSM found. Using NSSM for Windows service management.")
		} else {
			logger.Info("NSSM not found. Falling back to PowerShell for Windows service management.")
		}
		return &windowsController{useNssm: useNssm, timeout: cfg.timeout, gracefulStartStopTimeout: cfg.startStopTimout}
	case "linux":
		return &systemdController{timeout: cfg.timeout, gracefulStartStopTimeout: cfg.startStopTimout}
	default:
		return nil
	}
}

// Option will help set configurations for instance runner
type Option func(*opts)
type opts struct {
	timeout         time.Duration
	startStopTimout time.Duration
}

// WithStopTimeout takes timeout duration and sets it as timeout for
// StartInstance, StopInstance or RestartInstance to complete.
func WithTimeout(d time.Duration) Option {
	return func(o *opts) { o.timeout = d }
}

// WithGracefulStartStopTimeout takes time duration and sets it as timeout to check
// service status after StartInstance or StopInstance call.
func WithGracefulStartStopTimeout(d time.Duration) Option {
	return func(o *opts) { o.startStopTimout = d }
}
