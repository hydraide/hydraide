package instancedetector

import (
	"context"
	"fmt"
	"os"
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

type linuxDetector struct {
}

func (d *linuxDetector) ListInstances(ctx context.Context) ([]Instance, error) {

	// List all user services, including inactive and failed ones.
	cmd := exec.Command("systemctl", "--user", "list-units", "--type=service", "--all", "--no-legend")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("systemctl command failed with exit code %d: %s", exitErr.ExitCode(), string(output))
		}
		return nil, fmt.Errorf("failed to execute systemctl command: %v", err)
	}

	lines := strings.Split(string(output), "\n")

	instancesMap := make(map[string]Instance)
	// Regular expression to extract the instance name from a service unit (hydraserver-<instance-name>.service).
	reUnitName := regexp.MustCompile(`^hydraserver-(.*?)\.service$`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			// A valid service unit line should have at least UNIT, LOAD, ACTIVE, SUB states.
			// hydraserver-dev.service  loaded active running <description>
			continue
		}

		unitName := fields[0]
		matches := reUnitName.FindStringSubmatch(unitName)
		if len(matches) < 2 {
			// unit name doesn't match expected service naming convention.
			continue
		}

		instanceName := matches[1]
		subState := fields[3]

		// Normalize the systemd 'SUB' state to standard terms.
		normalizedStatus := "unknown"
		switch subState {
		case "running", "active", "activating":
			normalizedStatus = "active"
		case "dead", "inactive", "deactivating":
			normalizedStatus = "inactive"
		case "failed":
			normalizedStatus = "failed"
		default:
			normalizedStatus = "unknown"
		}

		// Check for duplicate instance names and discard.
		if _, exists := instancesMap[instanceName]; exists {
			fmt.Fprintf(os.Stderr, "Warning: Duplicate service entry found for instance '%s'. Only the first encountered status will be used.\n", instanceName)
		} else {
			instancesMap[instanceName] = Instance{Name: instanceName, Status: normalizedStatus}
		}
	}

	// Convert the map of unique instances back to a slice for consistent return.
	var instances []Instance
	for _, inst := range instancesMap {
		instances = append(instances, inst)
	}
	return instances, nil
}

type windowsDetector struct {
}

func (d *windowsDetector) ListInstances(ctx context.Context) ([]Instance, error) {
	return []Instance{{Name: "", Status: "Active"}}, nil
}
