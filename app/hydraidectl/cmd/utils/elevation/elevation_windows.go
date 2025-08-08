//go:build windows

package elevation

import "golang.org/x/sys/windows"

// IsElevated checks whether the current process is running
// with elevated privileges (i.e., as a member of the Administrators group).
// On systems with UAC enabled, a non-elevated process in the Administrators group
// will still return false here.
func IsElevated() bool {
	// Get handle to the current process.
	h, err := windows.GetCurrentProcess()
	if err != nil {
		return false
	}

	// Open the process token for querying.
	var token windows.Token
	if err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token); err != nil {
		return false
	}
	defer token.Close()

	// Built‑in Administrators SID (v0.34.0: only the SID type is required).
	admSID, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return false
	}

	// Check group membership; under UAC this returns false if not elevated.
	isMember, err := token.IsMember(admSID)
	return err == nil && isMember
}

// Hint returns a user-facing message explaining that the command
// must be run from an elevated Administrator PowerShell or Command Prompt,
// including the correct example command for the given instance name.
func Hint(instanceName string) string {
	return "This command must be run from an elevated Administrator PowerShell/Command Prompt.\n" +
		"Please run PowerShell as Administrator and execute:\n" +
		"hydraidectl service --instance " + instanceName
}
