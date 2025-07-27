// Package hydraidego provides a structured error handling system for the HydrAIDE SDK.
//
// It defines a set of standardized error codes and utility functions to simplify
// error detection, propagation, and interpretation when interacting with the Hydra database engine.
//
// Errors returned by HydrAIDE SDK functions can be inspected using helper functions like
// IsConnectionError(), IsInvalidArgument(), or IsSwampNotFound() to determine the nature
// of the failure without relying on string matching.
//
// Custom error instances can be created using NewError(), and existing errors can be
// introspected with GetErrorCode() and GetErrorMessage().
//
// This package is designed to work seamlessly with Hydra’s real-time and context-driven architecture,
// supporting gRPC environments, context timeouts, swamp validation, and structural model validation.
package hydraidego

import (
	"errors"
	"fmt"
)

// ErrorCode represents predefined error codes used throughout the HydrAIDE SDK.
type ErrorCode int

const (
	ErrCodeConnectionError ErrorCode = iota
	ErrCodeInternalDatabaseError
	ErrCodeCtxClosedByClient
	ErrCodeCtxTimeout
	ErrCodeSwampNotFound
	ErrCodeFailedPrecondition
	ErrCodeInvalidArgument
	ErrCodeNotFound
	ErrCodeAlreadyExists
	ErrCodeInvalidModel
	ErrConditionNotMet
	ErrCodeUnknown
)

// Error represents a structured error used across HydrAIDE operations.
type Error struct {
	Code    ErrorCode // Unique error code
	Message string    // Human-readable error message
}

// Error implements the built-in error interface.
func (e *Error) Error() string {
	return fmt.Sprintf("Code: %d, Message: %s", e.Code, e.Message)
}

// NewError creates a new instance of HydrAIDE error with a given code and message.
func NewError(code ErrorCode, message string) error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// GetErrorCode extracts the ErrorCode from an error, if available.
// If the error is nil or not a HydrAIDE error, ErrCodeUnknown is returned.
func GetErrorCode(err error) ErrorCode {
	if err == nil {
		return ErrCodeUnknown
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ErrCodeUnknown
}

// GetErrorMessage returns the message from a HydrAIDE error.
// If the error is not of type *Error, an empty string is returned.
func GetErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Message
	}
	return ""
}

// IsConnectionError returns true if the error indicates a connection issue
// between the client and the Hydra database service.
func IsConnectionError(err error) bool {
	return GetErrorCode(err) == ErrCodeConnectionError
}

// IsInternalDatabaseError returns true if the error was caused by an internal
// failure within the Hydra database system.
func IsInternalDatabaseError(err error) bool {
	return GetErrorCode(err) == ErrCodeInternalDatabaseError
}

// IsCtxClosedByClient returns true if the operation failed because the context
// was cancelled by the client.
func IsCtxClosedByClient(err error) bool {
	return GetErrorCode(err) == ErrCodeCtxClosedByClient
}

// IsCtxTimeout returns true if the operation failed due to a context timeout.
func IsCtxTimeout(err error) bool {
	return GetErrorCode(err) == ErrCodeCtxTimeout
}

// IsSwampNotFound returns true if the requested swamp (data space) was not found.
// This may not always be a strict error, but it indicates the absence of the swamp.
func IsSwampNotFound(err error) bool {
	return GetErrorCode(err) == ErrCodeSwampNotFound
}

// IsFailedPrecondition returns true if the operation was not executed
// because the preconditions were not met.
func IsFailedPrecondition(err error) bool {
	return GetErrorCode(err) == ErrCodeFailedPrecondition
}

// IsInvalidArgument returns true if the error was caused by invalid input parameters,
// such as malformed keys or unsupported filter values.
func IsInvalidArgument(err error) bool {
	return GetErrorCode(err) == ErrCodeInvalidArgument
}

// IsNotFound returns true if a specific entity (e.g. lock, key, swamp) was not found.
// The meaning depends on the function context, such as missing key or lock in Unlock(),
// or missing swamp in Read().
func IsNotFound(err error) bool {
	return GetErrorCode(err) == ErrCodeNotFound
}

// IsAlreadyExists returns true if an entity (such as a key or ID) already exists and
// cannot be overwritten.
func IsAlreadyExists(err error) bool {
	return GetErrorCode(err) == ErrCodeAlreadyExists
}

// IsInvalidModel returns true if the given model structure is invalid or cannot be
// properly serialized for the requested operation.
func IsInvalidModel(err error) bool {
	return GetErrorCode(err) == ErrCodeInvalidModel
}

// IsUnknown returns true if the error does not match any known HydrAIDE error code.
func IsUnknown(err error) bool {
	return GetErrorCode(err) == ErrCodeUnknown
}

func IsConditionNotMet(err error) bool {
	return GetErrorCode(err) == ErrConditionNotMet
}
