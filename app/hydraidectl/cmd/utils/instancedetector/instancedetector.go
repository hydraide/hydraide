package instancedetector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// Instance represents a single HydrAIDE server instance discovered on the host.
// Name is the logical instance identifier (derived from the OS service name).
// Status is a normalized, cross-platform state. Expected values:
//   - "active"       : service is running
//   - "inactive"     : installed/loaded but not running
//   - "failed"       : service exists but is in a failure state
//   - "activating"   : service is starting
//   - "deactivating" : service is stopping
//   - "unknown"      : state could not be determined
//   - "not-found"    : (only for direct queries) the instance does not exist
type Instance struct {
	Name   string
	Status string
}

// Detector abstracts OS-specific discovery and status queries for HydrAIDE
// application instances. An implementation exists per supported OS
// (Linux/systemd, Windows/SCM via PowerShell, macOS/launchd).
//
// Implementations must be context-aware: they should honor ctx cancellation
// and timeouts while invoking OS commands.
type Detector interface {
	// ListInstances scans the host for HydrAIDE services using the platform
	// conventions (e.g., systemd units, Windows services, launchd daemons)
	// and returns a list of Instances with normalized statuses.
	//
	// Returns:
	//   - ([]Instance{}, nil) when no matching services are present
	//   - (nil, error) if the underlying OS command fails or its output
	//     cannot be parsed
	ListInstances(ctx context.Context) ([]Instance, error)

	// GetInstanceStatus returns the normalized status of a single instance
	// identified by its logical name (the part after "hydraserver-" on Linux/Windows,
	// or after "com.hydraide.hydraserver-" on macOS).
	//
	// It performs a direct, single-service query and is therefore more efficient
	// than ListInstances when you only need one instance.
	//
	// Returns one of: "active", "inactive", "failed", "activating", "deactivating",
	// "unknown", or "not-found". A non-nil error is reserved for exceptional
	// execution failures (e.g., command invocation errors unrelated to absence).
	GetInstanceStatus(ctx context.Context, instanceName string) (string, error)
}

// CommandExecutor encapsulates command execution so it can be mocked in tests.
// Implementations should respect the provided context for cancellation/timeouts.
type CommandExecutor interface {
	Execute(ctx context.Context, name string, args ...string) ([]byte, error)
}

// commandExecutorImpl is the default executor based on os/exec.
// It returns stdout on success; on non-zero exit it returns an *exec.ExitError
// (with any captured stderr available via the error and/or Output()).
type commandExecutorImpl struct{}

func (e *commandExecutorImpl) Execute(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

// NewDetector constructs an OS-specific Detector based on runtime.GOOS.
// Supported values:
//   - linux   : systemd via `systemctl`
//   - windows : Windows Service Control Manager via PowerShell
//   - darwin  : launchd via `launchctl`
//
// Returns an error for unsupported platforms.
func NewDetector() (Detector, error) {
	executor := &commandExecutorImpl{}
	switch runtime.GOOS {
	case "linux":
		return &linuxDetector{executor: executor}, nil
	case "windows":
		return &windowsDetector{executor: executor}, nil
	case "darwin":
		return &darwinDetector{executor: executor}, nil
	default:
		return nil, fmt.Errorf("unsupported operating system")
	}
}

// reUnitName extracts the logical instance name from a systemd unit.
// Matches:  hydraserver-<instance>.service
// Capture:  <instance>
var reUnitName = regexp.MustCompile(`^hydraserver-(.*?)\.service$`)

// linuxDetector implements Detector for Linux using systemd.
type linuxDetector struct {
	executor CommandExecutor
}

// systemctlUnit mirrors relevant fields from `systemctl ... --output json`.
// Note: keys must align with systemd JSON output field names.
type systemctlUnit struct {
	Name      string `json:"unit"`   // full unit name (e.g., hydraserver-foo.service)
	Active    string `json:"active"` // high-level active state (not used for normalization)
	Sub       string `json:"sub"`    // sub-state (used for precise normalization)
	LoadState string `json:"load"`   // "loaded" indicates unit is installed/known
}

// ListInstances enumerates all systemd services, filters HydrAIDE units by name,
// and returns normalized statuses based on the unit Sub state.
//
// Behavior notes:
//   - Treats exit code 1 with empty output from systemctl as "no matches".
//   - Only includes units with LoadState == "loaded" (installed/known units).
//   - Normalization is done via normalizeStatus(Sub).
func (d *linuxDetector) ListInstances(ctx context.Context) ([]Instance, error) {
	// Ask systemd for all services in JSON for robust parsing.
	output, err := d.executor.Execute(ctx, "systemctl", "list-units", "--type=service", "--all", "--output", "json")
	if err != nil {
		// systemctl uses exit code 1 for "no units matched" in some contexts.
		if exitErr, ok := err.(*exec.ExitError); ok {
			if len(output) == 0 && exitErr.ExitCode() == 1 {
				return []Instance{}, nil
			}
			return nil, fmt.Errorf("systemctl command failed with exit code %d: %s", exitErr.ExitCode(), string(output))
		}
		return nil, fmt.Errorf("failed to execute systemctl command: %v", err)
	}

	// Parse the JSON payload into a typed slice for filtering.
	units, err := parseSystemctlJSON(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse systemctl output: %w", err)
	}

	var instances []Instance
	for _, unit := range units {
		// Keep only hydraserver-* units that are actually loaded.
		match := reUnitName.FindStringSubmatch(unit.Name)
		if len(match) < 2 || unit.LoadState != "loaded" {
			continue
		}
		unitName := match[1]
		normalizedStatus := normalizeStatus(unit.Sub)
		instances = append(instances, Instance{
			Name:   unitName,
			Status: normalizedStatus,
		})
	}

	return instances, nil
}

// GetInstanceStatus queries a single hydraserver unit's state efficiently.
//
// Flow:
//  1. Validate existence via `systemctl list-unit-files <name>`.
//     - Empty output → "not-found"
//     - Execution error → treated as "not-found" (absence is not exceptional here).
//  2. Retrieve SubState via `systemctl show ... --property=SubState`.
//     - Parse "SubState=<value>" and normalize.
//     - Any execution/parse issue → "unknown".
func (d *linuxDetector) GetInstanceStatus(ctx context.Context, instanceName string) (string, error) {
	serviceName := fmt.Sprintf("hydraserver-%s.service", instanceName)

	// Quick existence check against unit files; avoids scanning all units.
	listOutput, listErr := d.executor.Execute(ctx, "systemctl", "list-unit-files", "--no-pager", serviceName)
	if listErr != nil {
		// Absence is not an exceptional situation for callers; surface "not-found".
		return "not-found", nil
	}

	// If nothing returned (e.g., unit unknown), consider it missing.
	if strings.TrimSpace(string(listOutput)) == "" {
		return "not-found", nil
	}

	// Ask systemd for the precise sub-state (running/exited/failed/...).
	showOutput, err := d.executor.Execute(ctx, "systemctl", "show", serviceName, "--property=SubState")
	if err != nil {
		// Unit exists but status could not be retrieved reliably.
		return "unknown", nil
	}

	// Expect "SubState=running" (or other sub-state).
	parts := strings.SplitN(strings.TrimSpace(string(showOutput)), "=", 2)
	if len(parts) != 2 {
		return "unknown", nil
	}

	return normalizeStatus(parts[1]), nil
}

// parseSystemctlJSON unmarshals the JSON output of `systemctl list-units`
// into a slice of systemctlUnit structs.
//
// The JSON output is expected to be an array where each element corresponds
// to a systemd unit with fields like `unit`, `active`, `sub`, and `load`.
// If unmarshalling fails, an error is returned.
func parseSystemctlJSON(data []byte) ([]systemctlUnit, error) {
	var units []systemctlUnit
	if err := json.Unmarshal(data, &units); err != nil {
		return nil, err
	}
	return units, nil
}

// normalizeStatus maps systemd `SubState` values into high-level,
// platform-agnostic status strings.
//
// Input examples (systemd states) → Output:
//   - "running"      → "active"
//   - "exited"/"dead"/"inactive" → "inactive"
//   - "failed"       → "failed"
//   - "activating"   → "activating"
//   - "deactivating" → "deactivating"
//   - anything else  → "unknown"
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

// windowsDetector implements the Detector interface for Windows systems.
// It queries the Service Control Manager (SCM) through PowerShell
// to discover and report the status of HydrAIDE services.
type windowsDetector struct {
	executor CommandExecutor
}

// powershellService represents the JSON output of a PowerShell
// `Get-Service` command for a specific service.
// The fields map directly to PowerShell's Name and Status properties.
type powershellService struct {
	Name   string `json:"Name"`
	Status string `json:"Status"`
}

// reServiceName extracts the HydrAIDE instance name from a Windows service
// following the convention: hydraserver-<instance-name>.
var reServiceName = regexp.MustCompile(`^hydraserver-(.*)$`)

// ListInstances returns all HydrAIDE service instances detected on Windows.
// It uses PowerShell to list services named "hydraserver-*", parses the
// output into powershellService structs, and converts them into Instance
// values with normalized statuses.
//
// Returns an empty slice if no services are found.
// Returns an error if the PowerShell command fails or the JSON output
// cannot be parsed.
func (d *windowsDetector) ListInstances(ctx context.Context) ([]Instance, error) {
	psCommand := "Get-Service -Name 'hydraserver-*' | Select-Object Name,Status | ConvertTo-Json"
	output, err := d.executor.Execute(ctx, "powershell.exe", "-NoProfile", "-Command", psCommand)
	if err != nil {
		return nil, fmt.Errorf("failed to execute PowerShell command: %w, output: %s", err, string(output))
	}
	if len(output) == 0 || strings.TrimSpace(string(output)) == "[]" {
		return []Instance{}, nil
	}

	var services []powershellService
	if err := json.Unmarshal(output, &services); err != nil {
		// PowerShell returns a single object instead of an array
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

// GetInstanceStatus queries the status of a single HydrAIDE service
// on Windows using PowerShell.
//
// Returns one of: "active", "inactive", "activating", "deactivating",
// "failed", "unknown", or "not-found".
// Returns an error only if the PowerShell execution itself fails
// for reasons unrelated to the service not existing.
func (d *windowsDetector) GetInstanceStatus(ctx context.Context, instanceName string) (string, error) {
	serviceName := fmt.Sprintf("hydraserver-%s", instanceName)
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

// normalizeWindowsStatus maps Windows service statuses into normalized,
// cross-platform terms used by the Detector interface.
//
// Input examples → Output:
//   - "Running"       → "active"
//   - "Stopped"       → "inactive"
//   - "StartPending"  → "activating"
//   - "StopPending"   → "deactivating"
//   - "Paused"        → "inactive"
//   - anything else   → "unknown"
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

// --- macOS (launchd) ---------------------------------------------------------
//
// Naming conventions align with the service helper:
//   Label: com.hydraide.hydraserver-<instance>
//   Plist: /Library/LaunchDaemons/com.hydraide.hydraserver-<instance>.plist
//
// The detector reads launch daemon plists under /Library/LaunchDaemons to
// discover instances, and uses `launchctl print system/<label>` to infer status.

type darwinDetector struct {
	executor CommandExecutor
}

const (
	darwinLaunchDaemonsDir = "/Library/LaunchDaemons"
	darwinLabelPrefix      = "com.hydraide.hydraserver-"
)

// reDarwinPlist matches a launchd plist named with our convention and captures
// the logical instance name.
// Example:
//
//	com.hydraide.hydraserver-foo.plist → capture "foo"
var reDarwinPlist = regexp.MustCompile(`^com\.hydraide\.hydraserver-(.+)\.plist$`)

// reLastExitCode extracts the last exit code from a `launchctl print` output.
// Non-zero codes usually indicate failure.
var reLastExitCode = regexp.MustCompile(`last exit code = (\d+)`)

// ListInstances scans /Library/LaunchDaemons for plists that follow the
// HydrAIDE naming convention (com.hydraide.hydraserver-*.plist), derives the
// instance name from each file, then queries launchd for status via
// GetInstanceStatus.
//
// Behavior:
//   - If the directory is unreadable (e.g., permissions), returns an empty
//     slice (to match other platform behaviors).
//   - Each discovered plist is treated as an installed instance; status is
//     derived from `launchctl print system/<label>` heuristics.
func (d *darwinDetector) ListInstances(ctx context.Context) ([]Instance, error) {
	dir, err := os.ReadDir(darwinLaunchDaemonsDir)
	if err != nil {
		// Keep behavior consistent with other platforms: unreadable dirs mean
		// "no visible instances" rather than a hard failure.
		return []Instance{}, nil
	}

	instances := make([]Instance, 0, len(dir))
	for _, de := range dir {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		m := reDarwinPlist.FindStringSubmatch(name)
		if len(m) != 2 {
			continue
		}
		inst := m[1]
		status, _ := d.GetInstanceStatus(ctx, inst)
		instances = append(instances, Instance{Name: inst, Status: status})
	}
	return instances, nil
}

// GetInstanceStatus returns the normalized status of a single HydrAIDE instance
// managed by launchd.
//
// Flow:
//  1. Build the expected plist path from the instance name. If it does not
//     exist → "not-found".
//  2. Run `launchctl print system/<label>` to fetch detailed state.
//     - On execution error (not loaded / insufficient privileges), but the
//     plist exists → treat as "inactive" (installed but not running).
//  3. Normalize the printed output via normalizeDarwinStatus.
//
// Notes:
//   - Returning "inactive" on `launchctl print` failure aligns with Linux
//     semantics where a loaded-but-not-running unit is "inactive". If you want
//     stricter semantics, change the failure return to "unknown".
func (d *darwinDetector) GetInstanceStatus(ctx context.Context, instanceName string) (string, error) {
	label := darwinLabelPrefix + instanceName
	plistPath := filepath.Join(darwinLaunchDaemonsDir, label+".plist")

	if _, err := os.Stat(plistPath); err != nil {
		if os.IsNotExist(err) {
			return "not-found", nil
		}
		// Any other stat error: avoid failing hard; surface uncertainty.
		return "unknown", nil
	}

	// `launchctl print system/<label>` provides a detailed live state snapshot.
	out, err := d.executor.Execute(ctx, "launchctl", "print", "system/"+label)
	if err != nil {
		// Plist exists but the service likely isn't loaded/running or access is limited.
		// Treat as inactive (installed but not running).
		return "inactive", nil
	}

	return normalizeDarwinStatus(string(out)), nil
}

// normalizeDarwinStatus converts `launchctl print` output into a normalized,
// cross-platform status string using robust, order-insensitive heuristics.
//
// Heuristics:
//   - If output contains "state = running" or "PID = <n>" → "active"
//   - If output contains "state = exited" → "inactive"
//   - If output contains "last exit code = <n>" and n != 0 → "failed"
//   - If output contains "state = waiting" or "state = starting" → "activating"
//   - If output contains "state = stopping" → "deactivating"
//   - Otherwise → "unknown"
func normalizeDarwinStatus(printOutput string) string {
	s := strings.ToLower(printOutput)

	// Running / PID present → active
	if strings.Contains(s, "state = running") {
		return "active"
	}
	// Presence of a PID typically indicates a running job (not always present).
	if strings.Contains(s, "pid = ") {
		return "active"
	}

	// Exited → not running
	if strings.Contains(s, "state = exited") {
		return "inactive"
	}

	// Non-zero last exit code → failed; zero → clean exit → inactive
	if m := reLastExitCode.FindStringSubmatch(s); len(m) == 2 {
		if m[1] != "0" {
			return "failed"
		}
		return "inactive"
	}

	// Transitional states
	if strings.Contains(s, "state = waiting") || strings.Contains(s, "state = starting") {
		return "activating"
	}
	if strings.Contains(s, "state = stopping") {
		return "deactivating"
	}

	return "unknown"
}
