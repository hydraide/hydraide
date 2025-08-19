package cmd

import (
	"fmt"
	"time"
)

const (
	minTimeout = 1 * time.Second
	maxTimeout = 15 * time.Minute
)

// validateTimeoutValue validates that a timeout is within acceptable bounds.
// This is a shared validation function used by all lifecycle commands.
func validateTimeoutValue(name string, timeout time.Duration) error {
	if timeout < minTimeout {
		return fmt.Errorf("%s must be at least %v", name, minTimeout)
	}
	if timeout > maxTimeout {
		return fmt.Errorf("%s must not exceed %v", name, maxTimeout)
	}
	return nil
}
