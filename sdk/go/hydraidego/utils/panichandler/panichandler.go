package panichandler

import (
	"fmt"
	"log/slog"
	"runtime/debug"
)

// PanicHandler is a defensive helper that safely recovers from panics,
// especially useful inside goroutines. Should always be used with defer.
//
// Example:
//
//	go func() {
//	    defer panichandler.PanicHandler()
//	    // risky code here
//	}()
//
// Behavior:
// - If a panic occurs, it is caught via recover()
// - The error and full stack trace are logged using slog
// - A fallback message is printed to stdout for visibility
//
// This function does not rethrow the panic.
// It allows the goroutine to fail silently and safely.
func PanicHandler() {
	if r := recover(); r != nil {
		slog.Error("Recovered from panic",
			"error", fmt.Sprintf("%v", r),
			"stacktrace", string(debug.Stack()),
		)
		fmt.Printf("Recovered from panichandler: %v\n", r)
	}
}
