package instancedetector

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
)

type Instance struct {
	Name   string
	Status string
}

type Detector interface {
	ListInstances(ctx context.Context) ([]Instance, error)
}

func NewDetector() (Detector, error) {
	switch runtime.GOOS {
	case "linux":
		return &linuxDetector{}, nil
	case "windows":
		return &windowsDetector{}, nil
	default:
		return nil, fmt.Errorf("unsupported operating system")
	}
}

// Regular expression to extract the instance name from a service unit
// (hydraserver-<instance-name>.service).
var reUnitName = regexp.MustCompile(`^hydraserver-(.*?)\.service$`)

type linuxDetector struct {
}

// systemctlUnit is subset of fields from instance list.
type systemctlUnit struct {
	Name      string `json:"unit"`
	Active    string `json:"active"`
	Sub       string `json:"sub"`
	LoadState string `json:"load"`
}

func (d *linuxDetector) ListInstances(ctx context.Context) ([]Instance, error) {

	// List all user services, including inactive and failed ones.
	cmd := exec.CommandContext(ctx, "systemctl", "list-units", "--type=service", "--all", "--output", "json")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// No instances found
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
		// Normalize the systemd 'SUB' state to standard terms.
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
}

func (d *windowsDetector) ListInstances(ctx context.Context) ([]Instance, error) {
	return []Instance{{Name: "", Status: "Active"}}, nil
}
