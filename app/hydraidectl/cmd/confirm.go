package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// confirmClientsStopped prints the standard "stop your clients first" warning
// and asks the user to confirm before a destructive lifecycle operation
// (stop, restart, upgrade) tries to halt the running service.
//
// HydrAIDE protects in-flight data and refuses to shut down gracefully while
// any client still holds an open TCP connection — the stop phase will hang.
// Without this prompt, an unaware operator would think the CLI is broken.
//
// `op` is a verb that goes into the warning text ("stop", "restart",
// "upgrade"). When `skipPrompt` is true (e.g. caller passed --yes for
// scripting), the function prints the warning as a notice and returns true
// without blocking on stdin.
//
// Returns true when the operation should proceed, false when the user
// declined.
func confirmClientsStopped(op string, skipPrompt bool) bool {
	fmt.Println()
	fmt.Printf("⚠️  About to %s the HydrAIDE service.\n", op)
	fmt.Println("   Stop ALL client applications that hold open connections to this instance")
	fmt.Println("   first — HydrAIDE protects in-flight data and will not shut down gracefully")
	fmt.Println("   while clients are still connected (the stop phase will hang).")
	if skipPrompt {
		fmt.Println("   --yes was passed; proceeding without interactive confirmation.")
		return true
	}
	fmt.Print("   Have you stopped all clients? (y/n) [default: n]: ")
	reader := bufio.NewReader(os.Stdin)
	in, _ := reader.ReadString('\n')
	in = strings.ToLower(strings.TrimSpace(in))
	return in == "y" || in == "yes"
}
