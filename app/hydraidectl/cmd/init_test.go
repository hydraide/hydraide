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
