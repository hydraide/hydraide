package instancerunner

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// InstanceController defines basic control operations for a HydrAIDE instance.
type InstanceController interface {
	StartInstance(ctx context.Context, instanceName string) error
	StopInstance(ctx context.Context, instanceName string) error
	RestartInstance(ctx context.Context, instanceName string) error
}

// systemdController implements InstanceController for Linux.
type systemdController struct{}

// StartInstance starts a systemd user service.
func (c *systemdController) StartInstance(ctx context.Context, instance string) error {
	service := fmt.Sprintf("hydraserver-%s.service", instance)

	// Get the required environment variables for the user session
	env, err := c.getUserSystemdEnv()
	if err != nil {
		return fmt.Errorf("failed to get user systemd environment: %w", err)
	}

	cmd := exec.CommandContext(ctx, "systemctl", "--user", "start", service)
	cmd.Env = env

	fmt.Printf("Starting service '%s' with command: %s %s\n", service, cmd.Path, strings.Join(cmd.Args, " "))

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to start service '%s': %w", service, err)
	}

	log.Printf("Successfully started service '%s'\n", service)
	return nil
}

// StopInstance stops a systemd user service.
func (c *systemdController) StopInstance(ctx context.Context, instance string) error {
	service := fmt.Sprintf("hydraserver-%s.service", instance)

	cmd := exec.CommandContext(ctx, "systemctl", "--user", "stop", service)

	env, err := c.getUserSystemdEnv()
	if err != nil {
		return fmt.Errorf("failed to get user systemd environment: %w", err)
	}
	cmd.Env = env

	fmt.Printf("Stopping service '%s' with command: %s %s\n", service, cmd.Path, strings.Join(cmd.Args, " "))

	err = cmd.Run()
	if err != nil {
		// Check if the service is actually still active.
		isActive, checkErr := c.isServiceActive(service)
		if checkErr != nil {
			return fmt.Errorf("failed to check service status after stop attempt: %w", checkErr)
		}
		if isActive {
			return fmt.Errorf("failed to stop service '%s': %w", service, err)
		}
		// If it's not active, we can consider the stop successful, even if cmd.Run() returned an error.
		log.Printf("Service '%s' was already stopped. Stop operation considered successful.", service)
	} else {
		log.Printf("Successfully stopped service '%s'\n", service)
	}

	return nil
}

// RestartInstance stops and then starts a systemd user service instance.
func (c *systemdController) RestartInstance(ctx context.Context, instanceName string) error {
	log.Printf("Attempting to restart instance '%s'...", instanceName)

	// Stop the service.
	log.Printf("Stopping service for instance '%s'...", instanceName)
	err := c.StopInstance(ctx, instanceName)
	if err != nil {
		return fmt.Errorf("failed to stop instance '%s' for restart: %w", instanceName, err)
	}

	// Wait till the service is shut down completely.
	log.Printf("Waiting for service '%s' to be fully stopped...", instanceName)
	pollingCtx, pollingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pollingCancel()

	// Check the service status until it's inactive.
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
loop:
	for {
		select {
		case <-pollingCtx.Done():
			return fmt.Errorf("service '%s' did not stop in time for restart", instanceName)
		case <-ticker.C:
			isActive, err := c.isServiceActive(instanceName)
			if err != nil {
				return fmt.Errorf("failed to check status of '%s' during restart: %w", instanceName, err)
			}
			if !isActive {
				log.Printf("Service '%s' is confirmed stopped.", instanceName)
				break loop
			}
		}
	}

	// Start the service.
	log.Printf("Starting service for instance '%s'...", instanceName)
	err = c.StartInstance(ctx, instanceName)
	if err != nil {
		return fmt.Errorf("failed to start instance '%s' after stop: %w", instanceName, err)
	}

	log.Printf("Successfully restarted instance '%s'.", instanceName)
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

// Helper function to check service status
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

// windowsController implements InstanceController for Windows using PowerShell.
type windowsController struct{}

func (c *windowsController) StartInstance(ctx context.Context, instance string) error {
	service := fmt.Sprintf("hydraserver-%s", instance)
	cmd := exec.Command("powershell", "Start-Service", service)
	return cmd.Run()
}

func (c *windowsController) StopInstance(ctx context.Context, instance string) error {
	service := fmt.Sprintf("hydraserver-%s", instance)
	cmd := exec.Command("powershell", "Stop-Service", service)
	return cmd.Run()
}

func (c *windowsController) RestartInstance(ctx context.Context, instance string) error {
	return nil
}

// NewInstanceController returns the appropriate controller based on the OS.
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
