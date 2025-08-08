//go:build windows

package elevation

import "golang.org/x/sys/windows"

// IsElevated checks whether the current process is running
// with elevated privileges (i.e., as a member of the Administrators group).
// On systems with UAC enabled, a non-elevated process in the Administrators group
// will still return false here.
func IsElevated() bool {
	// Get a handle to the current process.
	h := windows.GetCurrentProcess()

	// Open the process token so we can query security information.
	var token windows.Token
	if err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token); err != nil {
		return false
	}
	defer token.Close()

	// Create the SID (Security Identifier) for the built-in Administrators group.
	admSID, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid, nil)
	if err != nil {
		return false
	}

	// Check whether the process token belongs to the Administrators group.
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
