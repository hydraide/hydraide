package instancerunner

import (
	"errors"
	"fmt"
)

// ErrServiceAlreadyRunning is returned when a start command is issued for a
// service that is already active.
var ErrServiceAlreadyRunning = errors.New("service is already running")

// ErrServiceNotRunning is returned when a stop or restart command is issued for a
// service that is not currently active.
var ErrServiceNotRunning = errors.New("service is not running")

// ErrServiceNotFound is returned when a requested service does not exist on the system.
var ErrServiceNotFound = errors.New("service not found")

// CmdError is a generic error type for failures that occur while executing an
// external command. It contains detailed information about the command,
// its output, and the underlying error, which is crucial for debugging.
type CmdError struct {
	Command string
	Output  string
	Err     error
}

// Error implements the error interface for CmdError.
func (e *CmdError) Error() string {
	return fmt.Sprintf("command '%s' failed: %v\nOutput: %s", e.Command, e.Err, e.Output)
}

// Unwrap provides access to the underlying error.
func (e *CmdError) Unwrap() error {
	return e.Err
}

// OperationError is a high-level error returned when a lifecycle operation
// (start, stop, restart) on a service fails for an internal reason. It is
// designed to wrap low-level details.
type OperationError struct {
	Instance  string
	Operation string
	Err       error
}

// Error implements the error interface for OperationError.
func (e *OperationError) Error() string {
	return fmt.Sprintf("failed to perform %s on instance '%s': %v", e.Operation, e.Instance, e.Err)
}

// Unwrap provides access to the underlying error.
func (e *OperationError) Unwrap() error {
	return e.Err
}

// NewOperationError is a constructor that creates a new OperationError.
func NewOperationError(instanceName, operation string, err error) error {
	return &OperationError{
		Instance:  instanceName,
		Operation: operation,
		Err:       err,
	}
}

// NewCmdError is a constructor that creates a new CmdError.
// It is used when system commands fail.
func NewCmdError(command string, operation string, err error) error {
	return &CmdError{
		Command: command,
		Output:  operation,
		Err:     err,
	}
}
