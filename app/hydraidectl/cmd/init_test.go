package cmd

import (
	"testing"
)

func TestValidatePort(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		{
			name:     "valid port 4900",
			input:    "4900",
			expected: "4900",
			hasError: false,
		},
		{
			name:     "valid port 1",
			input:    "1",
			expected: "1",
			hasError: false,
		},
		{
			name:     "valid port 65535",
			input:    "65535",
			expected: "65535",
			hasError: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
			hasError: true,
		},
		{
			name:     "port 0 (invalid)",
			input:    "0",
			expected: "",
			hasError: true,
		},
		{
			name:     "port 65536 (invalid)",
			input:    "65536",
			expected: "",
			hasError: true,
		},
		{
			name:     "negative port",
			input:    "-1",
			expected: "",
			hasError: true,
		},
		{
			name:     "non-numeric input",
			input:    "abc",
			expected: "",
			hasError: true,
		},
		{
			name:     "port with spaces",
			input:    " 4900 ",
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validatePort(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("Expected error for input '%s', but got none", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for input '%s', but got: %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("Expected result '%s' for input '%s', but got '%s'", tt.expected, tt.input, result)
				}
			}
		})
	}
}

func TestValidateLoglevel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		{"Lowercase debug", "debug", "debug", false},
		{"Lowercase info", "info", "info", false},
		{"Lowercase warn", "warn", "warn", false},
		{"Lowercase error", "error", "error", false},
		{"Uppercase INFO", "INFO", "info", false},
		{"Mixed casing", "DeBuG", "debug", false},
		{"With spaces", "  warn  ", "warn", false},
		{"Empty string", "", "info", false},
		{"Weird casing", "dEbUg", "debug", false},
		{"Newline wrapped", "\ninfo\n", "info", false},
		{"Unsupported trace", "trace", "", true},
		{"Unsupported string", "invalid level", "", true},
		{"Special characters", "@debug", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validateLoglevel(tt.input)
			if tt.hasError && err == nil {
				t.Errorf("Expected error for input '%s', but got none", tt.input)
			}
			if !tt.hasError && err != nil {
				t.Errorf("Expected no error for input '%s', but got: %v", tt.input, err)
			}
			if result != tt.expected {
				t.Errorf("Expected result '%s' for input '%s', but got '%s'", tt.expected, tt.input, result)
			}
		})
	}
}

func TestParseMessageSize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
		hasError bool
	}{
		{
			name:     "empty input (default)",
			input:    "",
			expected: DefaultMessageSize,
			hasError: false,
		},
		{
			name:     "raw bytes - 10MB",
			input:    "10485760",
			expected: 10485760,
			hasError: false,
		},
		{
			name:     "100MB",
			input:    "100MB",
			expected: 100 * MB,
			hasError: false,
		},
		{
			name:     "1GB",
			input:    "1GB",
			expected: 1 * GB,
			hasError: false,
		},
		{
			name:     "1.5GB",
			input:    "1.5GB",
			expected: int64(1.5 * float64(GB)),
			hasError: false,
		},
		{
			name:     "case insensitive - 50mb",
			input:    "50mb",
			expected: 50 * MB,
			hasError: false,
		},
		{
			name:     "with spaces",
			input:    " 200MB ",
			expected: 200 * MB,
			hasError: false,
		},
		{
			name:     "bytes with B suffix",
			input:    "10485760B",
			expected: 10485760,
			hasError: false,
		},
		{
			name:     "KB unit",
			input:    "10240KB",
			expected: 10240 * KB,
			hasError: false,
		},
		{
			name:     "too small - 5MB",
			input:    "5MB",
			expected: 0,
			hasError: true,
		},
		{
			name:     "too large - 20GB",
			input:    "20GB",
			expected: 0,
			hasError: true,
		},
		{
			name:     "invalid format - no number",
			input:    "MB",
			expected: 0,
			hasError: true,
		},
		{
			name:     "invalid number",
			input:    "abcMB",
			expected: 0,
			hasError: true,
		},
		{
			name:     "negative number",
			input:    "-100MB",
			expected: 0,
			hasError: true,
		},
		{
			name:     "unsupported unit",
			input:    "100TB",
			expected: 0,
			hasError: true,
		},
		{
			name:     "zero value",
			input:    "0",
			expected: 0,
			hasError: true,
		},
		{
			name:     "minimum valid size - 10MB",
			input:    "10MB",
			expected: 10 * MB,
			hasError: false,
		},
		{
			name:     "maximum valid size - 10GB",
			input:    "10GB",
			expected: 10 * GB,
			hasError: false,
		},
		{
			name:     "multiple decimal points",
			input:    "1.5.2GB",
			expected: 0,
			hasError: true,
		},
		{
			name:     "floating point precision test",
			input:    "1.999GB",
			expected: 2146409906, // actual result from int64(1.999*GB + 0.5)
			hasError: false,
		},
		{
			name:     "decimal point without digits",
			input:    ".5GB",
			expected: 536870912, // int64(0.5*GB + 0.5)
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseMessageSize(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("Expected error for input '%s', but got none", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for input '%s', but got: %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("Expected result %d for input '%s', but got %d", tt.expected, tt.input, result)
				}
			}
		})
	}
}

func TestValidateMessageSize(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected int64
		hasError bool
	}{
		{
			name:     "valid size - 10MB",
			input:    10 * MB,
			expected: 10 * MB,
			hasError: false,
		},
		{
			name:     "valid size - 1GB",
			input:    1 * GB,
			expected: 1 * GB,
			hasError: false,
		},
		{
			name:     "valid size - 10GB",
			input:    10 * GB,
			expected: 10 * GB,
			hasError: false,
		},
		{
			name:     "too small - 5MB",
			input:    5 * MB,
			expected: 0,
			hasError: true,
		},
		{
			name:     "too large - 20GB",
			input:    20 * GB,
			expected: 0,
			hasError: true,
		},
		{
			name:     "zero",
			input:    0,
			expected: 0,
			hasError: true,
		},
		{
			name:     "negative",
			input:    -1000,
			expected: 0,
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validateMessageSize(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("Expected error for input %d, but got none", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for input %d, but got: %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("Expected result %d for input %d, but got %d", tt.expected, tt.input, result)
				}
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected string
	}{
		{
			name:     "bytes",
			input:    512,
			expected: "512B",
		},
		{
			name:     "KB",
			input:    2048,
			expected: "2.0KB",
		},
		{
			name:     "MB",
			input:    10 * MB,
			expected: "10.0MB",
		},
		{
			name:     "GB",
			input:    2 * GB,
			expected: "2.0GB",
		},
		{
			name:     "fractional GB",
			input:    int64(1.5 * float64(GB)),
			expected: "1.5GB",
		},
		{
			name:     "large MB value",
			input:    512 * MB,
			expected: "512.0MB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatSize(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s' for input %d, but got '%s'", tt.expected, tt.input, result)
			}
		})
	}
}
