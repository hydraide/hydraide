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

type Detector interface {
	ListInstances(ctx context.Context) ([]Instance, error)
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
	// Nssm doesn't provide direct or easy way to list the services and status.
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
