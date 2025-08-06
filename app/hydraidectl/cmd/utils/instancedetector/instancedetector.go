package instancedetector

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

type Instance struct {
	Name   string
	Status string
}

// Detector is the interface for detecting and querying the status of HydrAIDE
// application instances on a given operating system.
type Detector interface {
	// ListInstances returns a slice of all detected HydrAIDE instances.
	//
	// This method scans the system for all services that match the naming
	// convention and returns a slice of Instance structs containing their
	// name and normalized status. It is designed for a broad overview of
	// all running and installed instances.
	//
	// It returns an empty slice and a nil error if no matching instances
	// are found. A non-nil error is returned if the underlying system
	// command fails to execute or its output cannot be parsed.
	ListInstances(ctx context.Context) ([]Instance, error)

	// GetInstanceStatus returns the normalized status of a single HydrAIDE instance
	// specified by its name.
	//
	// This method is highly efficient as it performs a direct query for a
	// single service, avoiding the overhead of listing all services. The
	// returned status is one of the following: "active", "inactive",
	// "failed", "unknown", or "not-found".
	//
	// It returns a non-nil error if the underlying system command fails to
	// execute for reasons other than the service not existing.
	GetInstanceStatus(ctx context.Context, instanceName string) (string, error)
}

// CommandExecutor defines an interface for executing a command and returning its output.
type CommandExecutor interface {
	Execute(ctx context.Context, name string, args ...string) ([]byte, error)
}

type commandExecutorImpl struct{}

func (e *commandExecutorImpl) Execute(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

func NewDetector() (Detector, error) {
	executor := &commandExecutorImpl{}
	switch runtime.GOOS {
	case "linux":
		return &linuxDetector{executor: executor}, nil
	case "windows":
		return &windowsDetector{executor: executor}, nil
	default:
		return nil, fmt.Errorf("unsupported operating system")
	}
}

// Regular expression to extract the instance name from a service unit
// (hydraserver-<instance-name>.service).
var reUnitName = regexp.MustCompile(`^hydraserver-(.*?)\.service$`)

type linuxDetector struct {
	executor CommandExecutor
}

type systemctlUnit struct {
	Name      string `json:"unit"`
	Active    string `json:"active"`
	Sub       string `json:"sub"`
	LoadState string `json:"load"`
}

func (d *linuxDetector) ListInstances(ctx context.Context) ([]Instance, error) {

	output, err := d.executor.Execute(ctx, "systemctl", "list-units", "--type=service", "--all", "--output", "json")

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if len(output) == 0 && exitErr.ExitCode() == 1 {
				return []Instance{}, nil
			}
			return nil, fmt.Errorf("systemctl command failed with exit code %d: %s", exitErr.ExitCode(), string(output))
		}
		return nil, fmt.Errorf("failed to execute systemctl command: %v", err)
	}

	units, err := parseSystemctlJSON(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse systemctl output: %w", err)
	}

	var instances []Instance

	for _, unit := range units {
		match := reUnitName.FindStringSubmatch(unit.Name)
		if len(match) < 2 || unit.LoadState != "loaded" {
			continue
		}
		unitName := match[1]
		normalizedStatus := normalizeStatus(unit.Sub)
		instances = append(instances, Instance{Name: unitName, Status: normalizedStatus})

	}
	return instances, nil
}

// GetInstanceStatus retrieves the status of a single instance using systemctl query.
func (d *linuxDetector) GetInstanceStatus(ctx context.Context, instanceName string) (string, error) {
	serviceName := fmt.Sprintf("hydraserver-%s.service", instanceName)

	listOutput, listErr := d.executor.Execute(ctx, "systemctl", "list-unit-files", "--no-pager", serviceName)

	if listErr != nil {
		return "not-found", nil
	}

	// If the output is empty or doesn't contain the service name, it's not found.
	if strings.TrimSpace(string(listOutput)) == "" {
		return "not-found", nil
	}

	// Get status
	showOutput, err := d.executor.Execute(ctx, "systemctl", "show", serviceName, "--property=SubState")

	if err != nil {
		return "unknown", nil
	}

	// The output will be in the format "SubState=running"
	parts := strings.SplitN(strings.TrimSpace(string(showOutput)), "=", 2)
	if len(parts) != 2 {
		return "unknown", nil
	}

	return normalizeStatus(parts[1]), nil
}

func parseSystemctlJSON(data []byte) ([]systemctlUnit, error) {
	var units []systemctlUnit
	if err := json.Unmarshal(data, &units); err != nil {
		return nil, err
	}
	return units, nil
}

// normalizeStatus maps systemd SUB state into our high-level status.
func normalizeStatus(sub string) string {
	switch sub {
	case "running":
		return "active"
	case "exited", "dead", "inactive":
		return "inactive"
	case "failed":
		return "failed"
	case "activating":
		return "activating"
	case "deactivating":
		return "deactivating"
	default:
		return "unknown"
	}
}

type windowsDetector struct {
	executor CommandExecutor
}

// powershellService is a Go struct to unmarshal the JSON output from PowerShell.
type powershellService struct {
	Name   string `json:"Name"`
	Status string `json:"Status"`
}

var reServiceName = regexp.MustCompile(`^hydraserver-(.*)$`)

func (d *windowsDetector) ListInstances(ctx context.Context) ([]Instance, error) {
	// Using PowerShell command to get services and format them as a JSON string.
	psCommand := "Get-Service -Name 'hydraserver-*' | Select-Object Name,Status | ConvertTo-Json"

	// Execute the PowerShell command using the injected executor.
	output, err := d.executor.Execute(ctx, "powershell.exe", "-NoProfile", "-Command", psCommand)

	if err != nil {
		return nil, fmt.Errorf("failed to execute PowerShell command: %w, output: %s", err, string(output))
	}

	if len(output) == 0 || strings.TrimSpace(string(output)) == "[]" {
		return []Instance{}, nil
	}

	// Unmarshal the JSON output into a slice of powershellService structs.
	var services []powershellService
	if err := json.Unmarshal(output, &services); err != nil {

		// If only one service then powershell doesnt return array.
		var single powershellService
		if err2 := json.Unmarshal(output, &single); err2 != nil {
			return nil, fmt.Errorf("invalid PowerShell JSON: %w", err)
		}
		services = []powershellService{single}
	}

	var instances []Instance
	for _, service := range services {
		match := reServiceName.FindStringSubmatch(service.Name)
		if len(match) < 2 {
			continue
		}

		instanceName := match[1]
		normalizedStatus := normalizeWindowsStatus(service.Status)
		instances = append(instances, Instance{Name: instanceName, Status: normalizedStatus})
	}

	return instances, nil
}

// GetInstanceStatus retrieves the status of a single instance using PowerShell query.
func (d *windowsDetector) GetInstanceStatus(ctx context.Context, instanceName string) (string, error) {
	serviceName := fmt.Sprintf("hydraserver-%s", instanceName)

	// The -ErrorAction SilentlyContinue prevents Get-Service from throwing an error
	// if the service is not found. Instead, it returns a null object.
	psCommand := fmt.Sprintf("(Get-Service -Name '%s' -ErrorAction SilentlyContinue).Status", serviceName)
	output, err := d.executor.Execute(ctx, "powershell.exe", "-NoProfile", "-Command", psCommand)

	if err != nil {
		return "", fmt.Errorf("failed to get status for service '%s': %w", serviceName, err)
	}

	status := strings.TrimSpace(string(output))
	if status == "" {
		return "not-found", nil
	}

	return normalizeWindowsStatus(status), nil
}

// normalizeWindowsStatus maps Windows service status strings to our standard terms.
func normalizeWindowsStatus(status string) string {
	lowerStatus := strings.ToLower(status)
	switch lowerStatus {
	case "running":
		return "active"
	case "stopped":
		return "inactive"
	case "startpending":
		return "activating"
	case "stoppending":
		return "deactivating"
	case "paused":
		return "inactive"
	default:
		return "unknown"
	}
}
