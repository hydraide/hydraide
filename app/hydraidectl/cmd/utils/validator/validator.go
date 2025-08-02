package validator

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Size constants for message and fragment sizes
const (
	Byte                int64 = 1
	KB                        = 1024 * Byte
	MB                        = 1024 * KB
	GB                        = 1024 * MB
	MinMessageSize            = 10 * MB // 10MB
	MaxMessageSize            = 10 * GB // 10GB
	DefaultMessageSize        = 10 * MB // 10MB
	MinFragmentSize           = 8 * KB  // 8KB
	MaxFragmentSize           = 1 * GB  // 1GB
	DefaultFragmentSize       = 8 * KB  // 8KB
)

// Validator defines an interface for common validation operations used in HydrAIDE configuration.
type Validator interface {
	// ValidatePort validates that the provided port string is a valid integer between 1 and 65535.
	// Parameters:
	//   - ctx: Context for cancellation and logging
	//   - portStr: The port string to validate
	// Returns:
	//   - string: The validated port string
	//   - error: Any error encountered during validation
	ValidatePort(ctx context.Context, portStr string) (string, error)

	// ValidateLoglevel validates whether the provided log level fits slog log levels and returns a valid string.
	// Parameters:
	//   - ctx: Context for cancellation and logging
	//   - logLevel: The log level string to validate
	// Returns:
	//   - string: The validated log level ("debug", "info", "warn", "error", or "info" if empty)
	//   - error: Any error encountered during validation
	ValidateLoglevel(ctx context.Context, logLevel string) (string, error)

	// ParseMessageSize parses a human-readable message size input and returns the size in bytes.
	// Parameters:
	//   - ctx: Context for cancellation and logging
	//   - input: The size string to parse (e.g., "100MB", "10485760")
	// Returns:
	//   - int64: The size in bytes
	//   - error: Any error encountered during parsing
	ParseMessageSize(ctx context.Context, input string) (int64, error)

	// ValidateMessageSize validates that the size is within acceptable bounds (10MB to 10GB).
	// Parameters:
	//   - ctx: Context for cancellation and logging
	//   - size: The size in bytes to validate
	// Returns:
	//   - int64: The validated size
	//   - error: Any error encountered during validation
	ValidateMessageSize(ctx context.Context, size int64) (int64, error)

	// ParseFragmentSize parses a human-readable fragment size input and returns the size in bytes.
	// Parameters:
	//   - ctx: Context for cancellation and logging
	//   - input: The size string to parse (e.g., "8KB", "512MB")
	// Returns:
	//   - int64: The size in bytes
	//   - error: Any error encountered during parsing
	ParseFragmentSize(ctx context.Context, input string) (int64, error)

	// ValidateFragmentSize validates that the fragment size is within acceptable bounds (8KB to 1GB).
	// Parameters:
	//   - ctx: Context for cancellation and logging
	//   - size: The size in bytes to validate
	// Returns:
	//   - int64: The validated size
	//   - error: Any error encountered during validation
	ValidateFragmentSize(ctx context.Context, size int64) (int64, error)

	// FormatSize converts a size in bytes to a human-readable format.
	// Parameters:
	//   - ctx: Context for cancellation and logging
	//   - bytes: The size in bytes to format
	// Returns:
	//   - string: The formatted size (e.g., "10.0MB", "1.5GB")
	FormatSize(ctx context.Context, bytes int64) string
}

// validatorImpl implements the Validator interface.
type validatorImpl struct{}

// New creates a new instance of the Validator interface.
func New() Validator {
	return &validatorImpl{}
}

// ValidatePort implements the ValidatePort method of the Validator interface.
func (v *validatorImpl) ValidatePort(ctx context.Context, portStr string) (string, error) {
	if portStr == "" {
		return "", fmt.Errorf("port cannot be empty")
	}

	portStr = strings.TrimSpace(portStr)
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", fmt.Errorf("port must be a valid integer")
	}
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("port must be between 1 and 65535")
	}
	return portStr, nil
}

// ValidateLoglevel implements the ValidateLoglevel method of the Validator interface.
func (v *validatorImpl) ValidateLoglevel(ctx context.Context, logLevel string) (string, error) {
	logLevel = strings.ToLower(strings.TrimSpace(logLevel))
	validLoglevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}

	if logLevel == "" {
		return "info", nil
	}
	if validLoglevels[logLevel] {
		return logLevel, nil
	}
	return "", fmt.Errorf("loglevel must be 'debug', 'info', 'warn' or 'error'")
}

// ParseMessageSize implements the ParseMessageSize method of the Validator interface.
func (v *validatorImpl) ParseMessageSize(ctx context.Context, input string) (int64, error) {
	input = strings.TrimSpace(input)

	// Handle empty input (default)
	if input == "" {
		return DefaultMessageSize, nil
	}

	// Try parsing as raw bytes first
	if val, err := strconv.ParseInt(input, 10, 64); err == nil {
		return v.ValidateMessageSize(ctx, val)
	}

	// Parse size with unit (case-insensitive)
	input = strings.ToUpper(input)

	// Check for multiple decimal points
	if strings.Count(input, ".") > 1 {
		return 0, fmt.Errorf("invalid format: multiple decimal points not allowed")
	}

	// Extract number and unit using more robust parsing
	var numStr strings.Builder
	var unit string

	for i, r := range input {
		if (r >= '0' && r <= '9') || r == '.' {
			numStr.WriteRune(r)
		} else {
			unit = input[i:]
			break
		}
	}

	numStrFinal := numStr.String()
	if numStrFinal == "" {
		return 0, fmt.Errorf("invalid format: use raw bytes (e.g., 10485760) or size with unit (e.g., 100MB, 1GB)")
	}

	// Parse the numeric part
	num, err := strconv.ParseFloat(numStrFinal, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number: %s", numStrFinal)
	}

	if num < 0 {
		return 0, fmt.Errorf("size cannot be negative")
	}

	// Convert based on unit
	var multiplier int64
	switch unit {
	case "", "B":
		multiplier = Byte
	case "KB":
		multiplier = KB
	case "MB":
		multiplier = MB
	case "GB":
		multiplier = GB
	default:
		return 0, fmt.Errorf("unsupported unit '%s': supported units are B, KB, MB, GB", unit)
	}

	// Calculate total bytes with proper rounding to avoid floating-point precision issues
	totalBytes := int64(num*float64(multiplier) + 0.5)

	return v.ValidateMessageSize(ctx, totalBytes)
}

// ValidateMessageSize implements the ValidateMessageSize method of the Validator interface.
func (v *validatorImpl) ValidateMessageSize(ctx context.Context, size int64) (int64, error) {
	if size < MinMessageSize {
		return 0, fmt.Errorf("size too small: minimum is %s (%d bytes)", v.FormatSize(ctx, MinMessageSize), MinMessageSize)
	}
	if size > MaxMessageSize {
		return 0, fmt.Errorf("size too large: maximum is %s (%d bytes)", v.FormatSize(ctx, MaxMessageSize), MaxMessageSize)
	}
	return size, nil
}

// ParseFragmentSize implements the ParseFragmentSize method of the Validator interface.
func (v *validatorImpl) ParseFragmentSize(ctx context.Context, input string) (int64, error) {
	input = strings.TrimSpace(input)

	// Handle empty input (default)
	if input == "" {
		return DefaultFragmentSize, nil
	}

	// Try parsing as raw bytes first
	if val, err := strconv.ParseInt(input, 10, 64); err == nil {
		return v.ValidateFragmentSize(ctx, val)
	}

	// Parse size with unit (case-insensitive)
	input = strings.ToUpper(input)

	// Check for multiple decimal points
	if strings.Count(input, ".") > 1 {
		return 0, fmt.Errorf("invalid format: multiple decimal points not allowed")
	}

	// Extract number and unit using more robust parsing
	var numStr strings.Builder
	var unit string

	for i, r := range input {
		if (r >= '0' && r <= '9') || r == '.' {
			numStr.WriteRune(r)
		} else {
			unit = input[i:]
			break
		}
	}

	numStrFinal := numStr.String()
	if numStrFinal == "" {
		return 0, fmt.Errorf("invalid format: use raw bytes (e.g., 8192) or size with unit (e.g., 8KB, 512MB)")
	}

	// Parse the numeric part
	num, err := strconv.ParseFloat(numStrFinal, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number: %s", numStrFinal)
	}

	if num < 0 {
		return 0, fmt.Errorf("fragment size cannot be negative")
	}

	// Convert based on unit
	var multiplier int64
	switch unit {
	case "", "B":
		multiplier = Byte
	case "KB":
		multiplier = KB
	case "MB":
		multiplier = MB
	case "GB":
		multiplier = GB
	default:
		return 0, fmt.Errorf("unsupported unit '%s': supported units are B, KB, MB, GB", unit)
	}

	// Calculate total bytes with proper rounding to avoid floating-point precision issues
	totalBytes := int64(num*float64(multiplier) + 0.5)

	return v.ValidateFragmentSize(ctx, totalBytes)
}

// ValidateFragmentSize implements the ValidateFragmentSize method of the Validator interface.
func (v *validatorImpl) ValidateFragmentSize(ctx context.Context, size int64) (int64, error) {
	if size < MinFragmentSize {
		return 0, fmt.Errorf("fragment size must be at least %s (%d bytes)", v.FormatSize(ctx, MinFragmentSize), MinFragmentSize)
	}
	if size > MaxFragmentSize {
		return 0, fmt.Errorf("fragment size must be at most %s (%d bytes)", v.FormatSize(ctx, MaxFragmentSize), MaxFragmentSize)
	}
	return size, nil
}

// FormatSize implements the FormatSize method of the Validator interface.
func (v *validatorImpl) FormatSize(ctx context.Context, bytes int64) string {
	if bytes >= GB {
		return fmt.Sprintf("%.1fGB", float64(bytes)/float64(GB))
	}
	if bytes >= MB {
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(MB))
	}
	if bytes >= KB {
		return fmt.Sprintf("%.1fKB", float64(bytes)/float64(KB))
	}
	return fmt.Sprintf("%dB", bytes)
}
