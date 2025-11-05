package servicehelper

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/locker"
)

// CommandRunner defines an abstraction for running external system commands.
//
// ✅ Purpose:
//   - Decouples execution logic from concrete implementations
//   - Makes it easier to test components by mocking command execution
//
// Typical implementations:
//   - RealRunner → executes real OS commands via `exec.Command`
//   - MockRunner → test double for unit testing, avoids running real binaries
type CommandRunner interface {
	// Run executes an external command with arguments.
	//
	// 📤 Returns:
	// - Combined stdout + stderr output (as []byte)
	// - Error if execution failed or exit code != 0
	//
	// ⚠️ Note: The caller is responsible for handling and interpreting the output.
	Run(name string, args ...string) ([]byte, error)
}

// RealRunner is the default implementation of CommandRunner.
//
// ✅ Behavior:
// - Uses Go's `exec.Command` to run the requested binary
// - Captures combined stdout and stderr output
// - Returns raw results to the caller
//
// ❗ It does not log, retry, or sanitize output — responsibility is on the caller.
type RealRunner struct{}

// Run executes an external command with given arguments and
// returns its combined stdout/stderr output.
func (RealRunner) Run(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	// The caller decides whether to log or print the output.
	return cmd.CombinedOutput()
}

// FSPaths defines filesystem locations relevant for service management.
//
// 🗂️ Fields:
// - SystemdDir → where Linux systemd service files are stored (e.g. /etc/systemd/system)
// - LaunchDaemonsDir → where macOS launchd plist files are stored (e.g. /Library/LaunchDaemons)
// - Extendable: WindowsStartupDir, custom folders, etc.
//
// ✅ Purpose:
// - Centralizes platform-specific service file paths
// - Makes testing easier by overriding with temporary folders
type FSPaths struct {
	SystemdDir       string // e.g. /etc/systemd/system
	LaunchDaemonsDir string // e.g. /Library/LaunchDaemons
}

// deps groups together dependencies required for service management.
//
// Contains:
// - runner → command execution abstraction
// - paths  → platform-specific file paths
//
// ✅ Benefit:
// - Provides easy injection for testing
// - Enables swapping between real and mock dependencies
type deps struct {
	runner CommandRunner
	paths  FSPaths
}

// defaultDeps initializes production-grade defaults.
//
// - RealRunner for executing commands
// - Standard system paths (/etc/systemd/system, /Library/LaunchDaemons)
//
// ✅ Used when no explicit dependency injection is provided.
func defaultDeps() deps {
	return deps{
		runner: RealRunner{},
		paths: FSPaths{
			SystemdDir:       "/etc/systemd/system",
			LaunchDaemonsDir: "/Library/LaunchDaemons",
		},
	}
}

// ServiceManager defines the interface for managing platform-specific services.
//
// Each implementation should support:
// - Linux (systemd)
// - macOS (launchd)
// - Windows (planned / WSL)
//
// ✅ Methods:
// - GenerateServiceFile → writes service definition (unit/plist/etc.)
// - ServiceExists       → checks if the service is already installed
// - EnableAndStartService → enables auto-start and runs the service
// - RemoveService       → disables and deletes the service
type ServiceManager interface {
	GenerateServiceFile(instanceName, basePath string) error
	ServiceExists(instanceName string) (bool, error)
	EnableAndStartService(instanceName, basePath string) error
	RemoveService(instanceName string) error
}

// Common constants used across platforms for service and binary naming.
const (
	BASE_SERVICE_NAME     = "hydraserver"  // logical service name prefix
	LINUX_OS              = "linux"        // runtime.GOOS value for Linux
	WINDOWS_OS            = "windows"      // runtime.GOOS value for Windows
	MAC_OS                = "darwin"       // runtime.GOOS value for macOS
	WINDOWS_BINARY_NAME   = "hydraide.exe" // default binary name for Windows
	LINUX_MAC_BINARY_NAME = "hydraide"     // default binary name for Linux/macOS
)

// serviceManagerImpl is the default implementation of ServiceManager.
//
// ✅ Responsibilities:
// - Encapsulates platform-specific service management (systemd, launchd, NSSM)
// - Uses injected dependencies (runner, paths) for testability
//
// 🔧 Created via:
// - New() → production-ready instance with defaultDeps()
// - newWithDeps() → test helper for injecting mocks/fakes
type serviceManagerImpl struct {
	d deps
}

// New returns a ServiceManager with default dependencies.
//
// ✅ Usage in production:
//
//	sm := New()
//	sm.GenerateServiceFile("hydraide-prod", "/opt/hydraide")
//
// Injects:
// - RealRunner for executing system commands
// - Standard FS paths (/etc/systemd/system, /Library/LaunchDaemons)
func New() ServiceManager {
	return &serviceManagerImpl{d: defaultDeps()}
}

// newWithDeps creates a ServiceManager with custom dependencies.
//
// ⚠️ Only intended for testing.
//
//	Example: pass a MockRunner or temporary folder paths.
func newWithDeps(d deps) *serviceManagerImpl {
	return &serviceManagerImpl{d: d}
}

// GenerateServiceFile writes the service definition for the current OS.
//
// ✅ Behavior by platform:
// - Linux   → generates a systemd unit in /etc/systemd/system
// - macOS   → generates a launchd plist in /Library/LaunchDaemons
// - Windows → registers service via NSSM
//
// ⚠️ Unsupported OS returns an error.
//
// 📘 Example:
//
//	sm.GenerateServiceFile("hydraide-test", "/opt/hydraide")
//
// This creates the necessary service descriptor for automatic startup.
func (s *serviceManagerImpl) GenerateServiceFile(instanceName, basePath string) error {
	slog.Info("Generating service file", "instance", instanceName, "os", runtime.GOOS)
	switch runtime.GOOS {
	case LINUX_OS:
		return s.generateSystemdService(instanceName, basePath)
	case MAC_OS:
		return s.generateLaunchdService(instanceName, basePath)
	case WINDOWS_OS:
		return s.generateWindowsNSSMService(instanceName, basePath)
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// generateSystemdService creates and installs a systemd unit file for HydrAIDE on Linux.
//
// ✅ Responsibilities:
// - Generates a fully functional `.service` unit file under /etc/systemd/system
// - Logs are automatically collected by systemd journald (no file needed)
// - Reloads systemd, enables, and starts the service automatically
//
// 📂 Service file location:
//
//	/etc/systemd/system/hydraserver-<instance>.service
//
// 📝 Unit file content includes:
// - ExecStart → HydrAIDE binary path (`basePath/hydraide`)
// - WorkingDirectory → `basePath`
// - Restart policy (always, with 5s delay)
// - StandardOutput/Error → journald (native systemd logging)
//
// 📊 Viewing logs:
//
//	journalctl -u hydraserver-<instance> -f  # follow mode
//	journalctl -u hydraserver-<instance> -n 100  # last 100 lines
//
// ⚠️ Safety checks:
// - If a service file with the same name already exists → returns error
// - Ensures parent directories exist before writing
//
// 🔁 Commands executed via runner:
// - `systemctl daemon-reload` → refresh systemd units
// - `systemctl enable <service>` → enable auto-start at boot
// - `systemctl start <service>` → start immediately
//
// 📘 Example usage:
//
//	sm.generateSystemdService("prod", "/opt/hydraide")
//
// This will create and start `/etc/systemd/system/hydraserver-prod.service`
// with logs available via journalctl.
func (s *serviceManagerImpl) generateSystemdService(instanceName, basePath string) error {
	slog.Info("Creating systemd service for Linux")
	serviceName := fmt.Sprintf("%s-%s", BASE_SERVICE_NAME, instanceName)

	// Wrapper around runner.Run → streams output to stdout for transparency.
	run := func(name string, args ...string) error {
		out, err := s.d.runner.Run(name, args...)
		if len(out) > 0 {
			if _, werr := fmt.Fprint(os.Stdout, string(out)); werr != nil {
				return werr
			}
		}
		return err
	}

	serviceFilePath := filepath.Join(s.d.paths.SystemdDir, serviceName+".service")
	executablePath := filepath.Join(basePath, LINUX_MAC_BINARY_NAME)

	// Systemd unit file content.
	// Logs are automatically collected by journald (no need for app.log file)
	// View logs with: journalctl -u hydraserver-<instance> -f
	serviceContent := fmt.Sprintf(`[Unit]
Description=HydrAIDE Service - %s
After=network.target

[Service]
ExecStart=%s
WorkingDirectory=%s
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`, instanceName, executablePath, basePath)

	// Ensure target directory exists
	if err := os.MkdirAll(filepath.Dir(serviceFilePath), 0o755); err != nil {
		return fmt.Errorf("failed to create directories for service file: %v", err)
	}

	// Prevent overwriting an existing service definition.
	if _, err := os.Stat(serviceFilePath); err == nil {
		slog.Warn("Service file already exists", "path", serviceFilePath)
		return fmt.Errorf("service file '%s' already exists", serviceFilePath)
	}

	// Write the systemd unit file.
	if err := os.WriteFile(serviceFilePath, []byte(serviceContent), 0o644); err != nil {
		return fmt.Errorf("failed to write service file: %v", err)
	}

	// Reload and enable service.
	if err := run("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload systemd daemon: %v", err)
	}
	if err := run("systemctl", "enable", serviceName+".service"); err != nil {
		return fmt.Errorf("failed to enable service: %v", err)
	}
	if err := run("systemctl", "start", serviceName+".service"); err != nil {
		return fmt.Errorf("failed to start service: %v", err)
	}

	slog.Info("Service file created successfully", "path", serviceFilePath)
	slog.Info("Logs will be available via journald", "command", fmt.Sprintf("journalctl -u %s -f", serviceName))
	return nil
}

// generateLaunchdService creates and installs a launchd service for HydrAIDE on macOS.
//
// ✅ Responsibilities:
// - Generates a `.plist` file under /Library/LaunchDaemons
// - Ensures the HydrAIDE binary exists and is executable
// - Logs are automatically collected by macOS Unified Logging (no file needed)
// - Boots, enables, and kickstarts the service via launchctl
//
// 📂 Service file location:
//
//	/Library/LaunchDaemons/com.hydraide.hydraserver-<instance>.plist
//
// 📝 Plist includes:
// - Label → com.hydraide.hydraserver-<instance>
// - ProgramArguments → full path to HydrAIDE binary
// - WorkingDirectory → `basePath`
// - RunAtLoad → true (auto-start on boot)
// - KeepAlive → restarts unless exit was successful
// - ProcessType → Background
//
// 📊 Viewing logs:
//
//	log stream --predicate 'processImagePath contains "hydraide"'  # live
//	log show --predicate 'processImagePath contains "hydraide"' --last 1h  # history
//
// ⚠️ Notes & Safety:
// - Root privileges are required to write to /Library/LaunchDaemons
// - Existing service is refreshed by `bootout + bootstrap` if bootstrap fails
// - Ensures binary has execute permission (chmod 0755)
// - Sets plist ownership to root:wheel and permission 0644
//
// 🔁 Commands executed via runner:
// - `chown root:wheel <plist>`
// - `chmod 644 <plist>`
// - `launchctl bootstrap system <plist>`
// - `launchctl enable system/<label>`
// - `launchctl kickstart -k system/<label>`
//
// 📘 Example usage:
//
//	sm.generateLaunchdService("prod", "/opt/hydraide")
//
// This will create and activate:
//
//	/Library/LaunchDaemons/com.hydraide.hydraserver-prod.plist
//
// Logs will be available via macOS Unified Logging.
func (s *serviceManagerImpl) generateLaunchdService(instanceName, basePath string) error {
	slog.Info("Creating launchd service for macOS")

	// Root required for system-wide daemon
	label := fmt.Sprintf("com.hydraide.%s-%s", BASE_SERVICE_NAME, instanceName)
	plistPath := filepath.Join(s.d.paths.LaunchDaemonsDir, label+".plist")
	executablePath := filepath.Join(basePath, LINUX_MAC_BINARY_NAME)

	// Verify binary exists and is executable
	if fi, err := os.Stat(executablePath); err != nil || fi.IsDir() {
		return fmt.Errorf("executable not found at: %s", executablePath)
	} else {
		_ = os.Chmod(executablePath, 0o755)
	}

	// Launchd plist content
	// Logs are automatically collected by macOS Unified Logging
	// View logs with: log stream --predicate 'processImagePath contains "hydraide"'
	// Or: log show --predicate 'processImagePath contains "hydraide"' --last 1h
	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
	</array>
	<key>WorkingDirectory</key><string>%s</string>
	<key>RunAtLoad</key><true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key><false/>
	</dict>
	<key>ProcessType</key><string>Background</string>
</dict>
</plist>
`, label, executablePath, basePath)

	// Write plist file
	if err := os.WriteFile(plistPath, []byte(plistContent), 0o644); err != nil {
		return fmt.Errorf("failed to write plist: %v", err)
	}
	// Ownership and permissions (root:wheel, 0644)
	_, _ = s.d.runner.Run("chown", "root:wheel", plistPath)
	_, _ = s.d.runner.Run("chmod", "644", plistPath)

	// Load service via launchctl
	if out, err := s.d.runner.Run("launchctl", "bootstrap", "system", plistPath); err != nil {
		// If already loaded, refresh with bootout + bootstrap
		slog.Warn("bootstrap failed, trying bootout+bootstrap", "error", err, "output", string(out))
		_, _ = s.d.runner.Run("launchctl", "bootout", "system", label)
		if out2, err2 := s.d.runner.Run("launchctl", "bootstrap", "system", plistPath); err2 != nil {
			return fmt.Errorf("launchctl bootstrap failed: %v; output: %s", err2, string(out2))
		}
	}

	// Enable + start service
	_, _ = s.d.runner.Run("launchctl", "enable", "system/"+label)
	if out, err := s.d.runner.Run("launchctl", "kickstart", "-k", "system/"+label); err != nil {
		slog.Warn("kickstart failed", "error", err, "output", string(out))
	}

	slog.Info("Plist file created successfully", "path", plistPath)
	slog.Info("Logs will be available via macOS Unified Logging", "command", "log stream --predicate 'processImagePath contains \"hydraide\"'")
	return nil
}

// ServiceExists checks whether a HydrAIDE service for the given instance exists
// on the current operating system.
//
// ✅ Responsibilities:
// - Detects presence of a service definition or active registration
// - Implements platform-specific checks for Linux, macOS, and Windows
//
// 📘 Behavior by platform:
//
// 🔹 Linux (systemd):
// - Looks for `/etc/systemd/system/hydraserver-<instance>.service`
// - Returns true if the service file exists
// - Distinguishes between "not found" and "stat error"
//
// 🔹 macOS (launchd):
//   - Looks for `/Library/LaunchDaemons/com.hydraide.hydraserver-<instance>.plist`
//   - If present, attempts `launchctl print system/<label>`
//     → If successful → service is present and enabled
//     → If fails → plist exists, but service may be disabled
//
// 🔹 Windows:
// - Checks in multiple layers, in order of preference:
//  1. **NSSM**: `nssm status <service>`
//  2. **Task Scheduler**: `schtasks /query /tn <service>`
//  3. **Registry startup key**: HKCU:\Software\Microsoft\Windows\CurrentVersion\Run
//  4. **Startup folder shortcut**: `%APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup\<service>.lnk`
//
// - Returns true if any method finds a match
//
// ⚠️ Notes:
//   - This method checks for **existence only** (file/entry present).
//     It does not guarantee that the service is currently running.
//   - Errors other than "not found" are returned as failures.
//   - On unsupported OS, returns an error.
//
// 📘 Example usage:
//
//	exists, err := sm.ServiceExists("prod")
//	if err != nil { log.Fatal(err) }
//	if exists { fmt.Println("Service already installed") }
func (s *serviceManagerImpl) ServiceExists(instanceName string) (bool, error) {
	serviceName := fmt.Sprintf("%s-%s", BASE_SERVICE_NAME, instanceName)
	slog.Info("Checking if service exists", "service", serviceName, "os", runtime.GOOS)

	switch runtime.GOOS {
	case LINUX_OS:
		serviceFilePath := filepath.Join(s.d.paths.SystemdDir, serviceName+".service")
		_, err := os.Stat(serviceFilePath)
		if err == nil {
			slog.Info("Service file found", "path", serviceFilePath)
			return true, nil
		}
		if os.IsNotExist(err) {
			slog.Info("Service file not found", "path", serviceFilePath)
			return false, nil
		}
		return false, fmt.Errorf("failed to check service existence: %v", err)

	case MAC_OS:
		label := "com.hydraide." + serviceName
		plistPath := filepath.Join(s.d.paths.LaunchDaemonsDir, label+".plist")

		_, err := os.Stat(plistPath)
		if err == nil {
			// try to read the service status
			if out, err := s.d.runner.Run("launchctl", "print", "system/"+label); err == nil {
				slog.Info("launchd service present", "label", label, "status_len", len(out))
				return true, nil
			}
			slog.Info("launchd plist present (service may be disabled)", "label", label)
			return true, nil
		}

		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat plist: %v", err)

	case WINDOWS_OS:
		// NSSM
		if output, err := s.d.runner.Run("nssm", "status", serviceName); err == nil {
			status := strings.TrimSpace(string(output))
			slog.Info("NSSM service found", "service", serviceName, "status", status)
			return true, nil
		}
		// Task Scheduler
		if _, err := s.d.runner.Run("schtasks", "/query", "/tn", serviceName); err == nil {
			slog.Info("Scheduled task found", "task", serviceName)
			return true, nil
		}
		// Registry
		regCmd := fmt.Sprintf(`Get-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "%s" -ErrorAction SilentlyContinue`, serviceName)
		if _, err := s.d.runner.Run("powershell", "-Command", regCmd); err == nil {
			slog.Info("Registry startup entry found", "entry", serviceName)
			return true, nil
		}
		// Startup folder
		startupFolder := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
		shortcutPath := filepath.Join(startupFolder, serviceName+".lnk")
		if _, err := os.Stat(shortcutPath); err == nil {
			slog.Info("Startup shortcut found", "path", shortcutPath)
			return true, nil
		}
		return false, nil

	default:
		return false, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// EnableAndStartService enables and starts the HydrAIDE service for the given instance.
//
// ✅ Responsibilities:
// - Ensures the service is enabled for auto-start
// - Starts (or kickstarts) the service immediately
// - Uses platform-specific tooling (systemd / launchd / NSSM, Task Scheduler)
//
// 📘 Behavior by platform:
//
// 🔹 Linux (systemd)
// - daemon-reload → enable → start → status (logged if available)
//
// 🔹 macOS (launchd)
// - If not bootstrapped: bootstrap with the system plist
// - enable → kickstart (-k) to restart if already running
//
// 🔹 Windows
// - Prefer NSSM: nssm start + status log
// - Else Task Scheduler: schtasks /run
// - Else fallback: start the executable directly in background (best-effort)
//
// ⚠️ Notes:
// - This method assumes the service definition already exists (e.g., unit/plist created).
// - Returns detailed error with captured tool output on failure.
// - `basePath` is used on Windows fallback to locate the binary.
//
// 📌 Example:
//
//	if err := sm.EnableAndStartService("prod", "/opt/hydraide"); err != nil { log.Fatal(err) }
func (s *serviceManagerImpl) EnableAndStartService(instanceName, basePath string) error {
	serviceName := fmt.Sprintf("%s-%s", BASE_SERVICE_NAME, instanceName)
	slog.Info("Starting service", "service", serviceName, "os", runtime.GOOS)

	switch runtime.GOOS {
	case LINUX_OS:
		_, _ = s.d.runner.Run("systemctl", "daemon-reload")
		if out, err := s.d.runner.Run("systemctl", "enable", serviceName+".service"); err != nil {
			return fmt.Errorf("failed to enable service: %v; output: %s", err, string(out))
		}
		if out, err := s.d.runner.Run("systemctl", "start", serviceName+".service"); err != nil {
			return fmt.Errorf("failed to start service: %v; output: %s", err, string(out))
		}
		if out, err := s.d.runner.Run("systemctl", "status", serviceName+".service", "--no-pager"); err == nil {
			slog.Info("Service status", "output", string(out))
		}
		return nil

	case MAC_OS:
		label := "com.hydraide." + serviceName
		plistPath := filepath.Join(s.d.paths.LaunchDaemonsDir, label+".plist")
		// If not bootstrapped yet, bootstrap it now.
		if _, err := s.d.runner.Run("launchctl", "print", "system/"+label); err != nil {
			if out2, err2 := s.d.runner.Run("launchctl", "bootstrap", "system", plistPath); err2 != nil {
				return fmt.Errorf("launchctl bootstrap failed: %v; output: %s", err2, string(out2))
			}
		}
		_, _ = s.d.runner.Run("launchctl", "enable", "system/"+label)
		if out, err := s.d.runner.Run("launchctl", "kickstart", "-k", "system/"+label); err != nil {
			return fmt.Errorf("launchctl kickstart failed: %v; output: %s", err, string(out))
		}
		return nil

	case WINDOWS_OS:
		// Prefer NSSM if installed.
		if out, err := s.d.runner.Run("nssm", "start", serviceName); err == nil {
			slog.Info("NSSM service started", "service", serviceName, "output", string(out))
			if st, err := s.d.runner.Run("nssm", "status", serviceName); err == nil {
				slog.Info("Service status", "status", strings.TrimSpace(string(st)))
			}
			return nil
		}
		// Otherwise, try Task Scheduler.
		if _, err := s.d.runner.Run("schtasks", "/query", "/tn", serviceName); err == nil {
			if _, err := s.d.runner.Run("schtasks", "/run", "/tn", serviceName); err != nil {
				return fmt.Errorf("failed to start scheduled task: %v", err)
			}
			return nil
		}
		// Last resort: start the binary directly (best-effort).
		servicePath := filepath.Join(basePath, WINDOWS_BINARY_NAME)
		cmd := exec.Command(servicePath)
		cmd.Dir = basePath
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("failed to start service executable: %v", err)
		}
		return nil

	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// RemoveService stops and removes the HydrAIDE service definition for the given instance.
//
// ✅ Responsibilities:
// - Gracefully stops the service (best-effort)
// - Removes platform-specific service artifacts (unit/plist/NSSM, scheduled task, registry entries, shortcuts)
// - Cleans up HydrAIDE instance lock file (best-effort)
//
// 📘 Behavior by platform:
//
// 🔹 Linux (systemd)
// - systemctl stop → disable → remove unit file → daemon-reload
// - Deletes instance lock file via locker.DeleteLockFile()
//
// 🔹 macOS (launchd)
// - launchctl bootout (ignore if not loaded) → remove plist
// - Deletes instance lock file via locker.DeleteLockFile()
//
// 🔹 Windows
// - NSSM stop → NSSM remove
// - Delete Scheduled Task (schtasks /delete /f)
// - Remove HKCU Run entry (PowerShell Remove-ItemProperty)
// - Remove Startup shortcut (.lnk) from %APPDATA%\...\Startup
// - Deletes instance lock file via locker.DeleteLockFile()
//
// ⚠️ Notes:
// - Non-fatal cleanup steps are logged and ignored when safe (best-effort philosophy).
// - Returns an error only when a hard removal step fails (e.g., deleting unit/plist file).
// - On unsupported OS, returns an error.
//
// 📌 Example:
//
//	if err := sm.RemoveService("prod"); err != nil { log.Fatal(err) }
func (s *serviceManagerImpl) RemoveService(instanceName string) error {
	serviceName := fmt.Sprintf("%s-%s", BASE_SERVICE_NAME, instanceName)
	slog.Info("Removing service", "service", serviceName, "os", runtime.GOOS)

	switch runtime.GOOS {
	case LINUX_OS:
		serviceFilePath := filepath.Join(s.d.paths.SystemdDir, serviceName+".service")
		_, _ = s.d.runner.Run("systemctl", "stop", serviceName+".service")

		if err := locker.DeleteLockFile(instanceName); err != nil {
			slog.Error("Failed to delete lock file for instance", "instanceName", instanceName)
		}

		_, _ = s.d.runner.Run("systemctl", "disable", serviceName+".service")
		if err := os.Remove(serviceFilePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove service file: %v", err)
		}
		_, _ = s.d.runner.Run("systemctl", "daemon-reload")
		return nil

	case MAC_OS:
		label := "com.hydraide." + serviceName
		plistPath := filepath.Join(s.d.paths.LaunchDaemonsDir, label+".plist")

		// Stop/unload (ignore if not loaded)
		if out, err := s.d.runner.Run("launchctl", "bootout", "system", label); err != nil {
			slog.Warn("bootout failed (maybe not loaded)", "error", err, "output", string(out))
		}

		if err := locker.DeleteLockFile(instanceName); err != nil {
			slog.Error("Failed to delete lock file for instance", "instanceName", instanceName)
		}

		// Remove plist
		if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove plist: %v", err)
		}
		return nil

	case WINDOWS_OS:
		// NSSM stop + remove (best-effort)
		_, _ = s.d.runner.Run("nssm", "stop", serviceName)

		if err := locker.DeleteLockFile(instanceName); err != nil {
			slog.Error("Failed to delete lock file for instance", "instanceName", instanceName)
		}

		_, _ = s.d.runner.Run("nssm", "remove", serviceName, "confirm")
		// Task Scheduler
		_, _ = s.d.runner.Run("schtasks", "/delete", "/tn", serviceName, "/f")
		// Registry Run entry
		regCmd := fmt.Sprintf(`Remove-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "%s" -ErrorAction SilentlyContinue`, serviceName)
		_, _ = s.d.runner.Run("powershell", "-Command", regCmd)
		// Startup shortcut
		startupFolder := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
		_ = os.Remove(filepath.Join(startupFolder, serviceName+".lnk"))
		return nil

	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// checkAndInstallNSSM verifies that NSSM is available, and installs it via winget if missing.
//
// ✅ Responsibilities:
// - Check whether `nssm` is present in PATH (`nssm version`)
// - If not found, attempt a non-interactive install via `winget`
// - Provide clear logs for both success and failure cases
//
// 🔹 Behavior:
// - Returns nil immediately when NSSM is already installed
// - Invokes: winget install --id nssm.nssm --source winget
// - Uses `--accept-package-agreements` and `--accept-source-agreements` for unattended install
//
// ⚠️ Requirements & Notes (Windows only):
//   - Requires Windows 10/11 with **winget** available and configured
//   - May require **administrator** privileges depending on environment/policies
//   - Corporate environments with restricted winget access may block the install
//   - If winget is unavailable, this method will return an error — caller should
//     handle fallback (manual download or bundled installer)
//
// 📌 Example:
//
//	if err := sm.checkAndInstallNSSM(); err != nil {
//	    return fmt.Errorf("NSSM unavailable: %w", err)
//	}
func (s *serviceManagerImpl) checkAndInstallNSSM() error {
	slog.Info("Checking if NSSM is installed")
	if _, err := s.d.runner.Run("nssm", "version"); err == nil {
		slog.Info("NSSM is already installed and available in PATH")
		return nil
	}

	slog.Warn("NSSM not found. Attempting installation using winget")

	if output, err := s.d.runner.Run(
		"winget", "install",
		"--id=nssm.nssm",
		"--source=winget",
		"--accept-package-agreements",
		"--accept-source-agreements",
	); err != nil {
		slog.Error("Failed to install NSSM via winget", "error", err, "output", string(output))
		return fmt.Errorf("failed to install NSSM via winget: %w", err)
	}

	slog.Info("NSSM installed successfully using winget")
	return nil
}

// generateWindowsNSSMService creates and configures a Windows service for HydrAIDE using NSSM.
//
// ✅ Responsibilities:
// - Verifies/installs NSSM (via winget) if not present
// - Installs a Windows service: hydraserver-<instance>
// - Logs are automatically collected by Windows Event Log (no file needed)
//
// 🗂️ Artifacts:
// - Binary: <basePath>\hydraide.exe
//
// 🔧 NSSM settings applied:
// - DisplayName:  "HydrAIDE Service - <instance>"
// - Description:  "HydrAIDE Service Instance: <instance>"
// - Start:        SERVICE_AUTO_START (start at boot)
// - AppDirectory: <basePath>
//
// 📊 Viewing logs:
//   - Windows Event Viewer → Windows Logs → Application
//   - Filter by source: "HydrAIDE Service" or the service name
//
// ⚠️ Requirements & Notes:
//   - Requires Windows with winget available for automatic NSSM install,
//     or NSSM already on PATH.
//   - If the executable is missing, returns a descriptive error.
//   - NSSM configuration steps are best-effort: failures are logged as warnings,
//     but do not abort the whole setup once the service is installed.
//
// 📌 Example:
//
//	err := sm.generateWindowsNSSMService("prod", `C:\HydrAIDE`)
//	if err != nil { log.Fatal(err) }
func (s *serviceManagerImpl) generateWindowsNSSMService(instanceName, basePath string) error {
	slog.Info("Creating Windows service using NSSM")

	if err := s.checkAndInstallNSSM(); err != nil {
		return fmt.Errorf("NSSM installation failed: %v", err)
	}

	serviceName := fmt.Sprintf("%s-%s", BASE_SERVICE_NAME, instanceName)
	executablePath := filepath.Join(basePath, WINDOWS_BINARY_NAME)

	if _, err := os.Stat(executablePath); os.IsNotExist(err) {
		return fmt.Errorf("executable not found at: %s", executablePath)
	}

	slog.Info("Installing NSSM service", "service", serviceName)
	if output, err := s.d.runner.Run("nssm", "install", serviceName, executablePath); err != nil {
		slog.Error("Failed to install NSSM service", "output", string(output), "error", err)
		return fmt.Errorf("failed to install NSSM service: %v", err)
	}

	// Configure NSSM service settings
	// Logs are automatically collected by Windows Event Log
	// View logs in Event Viewer under Windows Logs > Application
	configs := [][]string{
		{"set", serviceName, "DisplayName", fmt.Sprintf("HydrAIDE Service - %s", instanceName)},
		{"set", serviceName, "Description", fmt.Sprintf("HydrAIDE Service Instance: %s", instanceName)},
		{"set", serviceName, "Start", "SERVICE_AUTO_START"},
		{"set", serviceName, "AppDirectory", basePath},
	}
	for _, cfg := range configs {
		if output, err := s.d.runner.Run("nssm", cfg...); err != nil {
			slog.Warn("Failed to set NSSM config", "config", cfg, "error", err, "output", string(output))
		} else {
			slog.Info("Set NSSM config successfully", "config", cfg)
		}
	}

	slog.Info("NSSM service configured successfully", "service", serviceName)
	slog.Info("Logs will be available in Windows Event Viewer", "location", "Windows Logs > Application")
	return nil
}
