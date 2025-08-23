package validator

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestNew ensures that New() returns a non-nil Validator instance.
func TestNew(t *testing.T) {
	validator := New()
	if validator == nil {
		t.Fatal("New() returned nil")
	}
	if _, ok := validator.(*validatorImpl); !ok {
		t.Errorf("New() returned unexpected type: %T", validator)
	}
}

// TestValidatePort tests the ValidatePort method for various port inputs.
func TestValidatePort(t *testing.T) {
	validator := New()
	ctx := context.Background()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid port",
			input:   "8080",
			want:    "8080",
			wantErr: false,
		},
		{
			name:    "minimum port",
			input:   "1",
			want:    "1",
			wantErr: false,
		},
		{
			name:    "maximum port",
			input:   "65535",
			want:    "65535",
			wantErr: false,
		},
		{
			name:    "empty port",
			input:   "",
			want:    "",
			wantErr: true,
			errMsg:  "port cannot be empty",
		},
		{
			name:    "non-integer port",
			input:   "abc",
			want:    "",
			wantErr: true,
			errMsg:  "port must be a valid integer",
		},
		{
			name:    "port below range",
			input:   "0",
			want:    "",
			wantErr: true,
			errMsg:  "port must be between 1 and 65535",
		},
		{
			name:    "port above range",
			input:   "65536",
			want:    "",
			wantErr: true,
			errMsg:  "port must be between 1 and 65535",
		},
		{
			name:    "port with spaces",
			input:   "  8080  ",
			want:    "8080",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validator.ValidatePort(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePort() name=%s input = %s error = %v, wantErr %v", tt.name, tt.input, err, tt.wantErr)
				return
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidatePort() error message = %q, want %q", err.Error(), tt.errMsg)
			}
			if got != tt.want {
				t.Errorf("ValidatePort() got = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestValidateLoglevel tests the ValidateLoglevel method for various log level inputs.
func TestValidateLoglevel(t *testing.T) {
	validator := New()
	ctx := context.Background()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid log level: debug",
			input:   "debug",
			want:    "debug",
			wantErr: false,
		},
		{
			name:    "valid log level: info",
			input:   "info",
			want:    "info",
			wantErr: false,
		},
		{
			name:    "valid log level: warn",
			input:   "warn",
			want:    "warn",
			wantErr: false,
		},
		{
			name:    "valid log level: error",
			input:   "error",
			want:    "error",
			wantErr: false,
		},
		{
			name:    "empty log level",
			input:   "",
			want:    "info",
			wantErr: false,
		},
		{
			name:    "log level with spaces",
			input:   "  INFO  ",
			want:    "info",
			wantErr: false,
		},
		{
			name:    "invalid log level",
			input:   "invalid",
			want:    "",
			wantErr: true,
			errMsg:  "loglevel must be 'debug', 'info', 'warn' or 'error'",
		},
		{
			name:    "case insensitive log level",
			input:   "DEBUG",
			want:    "debug",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validator.ValidateLoglevel(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLoglevel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateLoglevel() error message = %q, want %q", err.Error(), tt.errMsg)
			}
			if got != tt.want {
				t.Errorf("ValidateLoglevel() got = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestParseMessageSize tests the ParseMessageSize method for various input formats.
func TestParseMessageSize(t *testing.T) {
	validator := New()
	ctx := context.Background()

	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
		errMsg  string
	}{
		{
			name:    "empty input",
			input:   "",
			want:    DefaultMessageSize,
			wantErr: false,
		},
		{
			name:    "valid raw bytes",
			input:   "10485760",
			want:    10 * MB,
			wantErr: false,
		},
		{
			name:    "valid size with MB unit",
			input:   "100MB",
			want:    100 * MB,
			wantErr: false,
		},
		{
			name:    "valid size with GB unit",
			input:   "1GB",
			want:    1 * GB,
			wantErr: false,
		},
		{
			name:    "valid size with KB unit",
			input:   "1024KB",
			want:    1024 * KB,
			wantErr: false,
		},
		{
			name:    "valid size with B unit",
			input:   "10485760B",
			want:    10 * MB,
			wantErr: false,
		},
		{
			name:    "valid size with decimal",
			input:   "1.5GB",
			want:    1610612736, // 1.5 * 1024 * 1024 * 1024
			wantErr: false,
		},
		{
			name:    "size below minimum",
			input:   "1MB",
			want:    0,
			wantErr: true,
			errMsg:  fmt.Sprintf("size too small: minimum is %s (%d bytes)", validator.FormatSize(ctx, MinMessageSize), MinMessageSize),
		},
		{
			name:    "size above maximum",
			input:   "11GB",
			want:    0,
			wantErr: true,
			errMsg:  fmt.Sprintf("size too large: maximum is %s (%d bytes)", validator.FormatSize(ctx, MaxMessageSize), MaxMessageSize),
		},
		{
			name:    "invalid number",
			input:   "abcMB",
			want:    0,
			wantErr: true,
			errMsg:  "invalid number: abc",
		},
		{
			name:    "negative size",
			input:   "-10MB",
			want:    0,
			wantErr: true,
			errMsg:  "size cannot be negative",
		},
		{
			name:    "multiple decimal points",
			input:   "1.2.3MB",
			want:    0,
			wantErr: true,
			errMsg:  "invalid format: multiple decimal points not allowed",
		},
		{
			name:    "unsupported unit",
			input:   "10TB",
			want:    0,
			wantErr: true,
			errMsg:  "unsupported unit 'TB': supported units are B, KB, MB, GB",
		},
		{
			name:    "empty number",
			input:   "MB",
			want:    0,
			wantErr: true,
			errMsg:  "invalid format: use raw bytes (e.g., 10485760) or size with unit (e.g., 100MB, 1GB)",
		},
		{
			name:    "case insensitive unit",
			input:   "100mb",
			want:    100 * MB,
			wantErr: false,
		},
		{
			name:    "input with spaces",
			input:   "  100 MB  ",
			want:    100 * MB,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validator.ParseMessageSize(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMessageSize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ParseMessageSize() error message = %q, want %q", err.Error(), tt.errMsg)
			}
			if got != tt.want {
				t.Errorf("ParseMessageSize() got = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestValidateMessageSize tests the ValidateMessageSize method for various size inputs.
func TestValidateMessageSize(t *testing.T) {
	validator := New()
	ctx := context.Background()

	tests := []struct {
		name    string
		input   int64
		want    int64
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid size",
			input:   100 * MB,
			want:    100 * MB,
			wantErr: false,
		},
		{
			name:    "minimum size",
			input:   MinMessageSize,
			want:    MinMessageSize,
			wantErr: false,
		},
		{
			name:    "maximum size",
			input:   MaxMessageSize,
			want:    MaxMessageSize,
			wantErr: false,
		},
		{
			name:    "below minimum",
			input:   MinMessageSize - 1,
			want:    0,
			wantErr: true,
			errMsg:  fmt.Sprintf("size too small: minimum is %s (%d bytes)", validator.FormatSize(ctx, MinMessageSize), MinMessageSize),
		},
		{
			name:    "above maximum",
			input:   MaxMessageSize + 1,
			want:    0,
			wantErr: true,
			errMsg:  fmt.Sprintf("size too large: maximum is %s (%d bytes)", validator.FormatSize(ctx, MaxMessageSize), MaxMessageSize),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validator.ValidateMessageSize(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMessageSize() name = %v  input = %v error = %v, wantErr %v", tt.name, tt.input, err, tt.wantErr)
				return
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateMessageSize() error message = %q, want %q", err.Error(), tt.errMsg)
			}
			if got != tt.want {
				t.Errorf("ValidateMessageSize() got = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestParseFragmentSize tests the ParseFragmentSize method for various input formats.
func TestParseFragmentSize(t *testing.T) {
	validator := New()
	ctx := context.Background()

	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
		errMsg  string
	}{
		{
			name:    "empty input",
			input:   "",
			want:    DefaultFragmentSize,
			wantErr: false,
		},
		{
			name:    "valid raw bytes",
			input:   "8192",
			want:    8 * KB,
			wantErr: false,
		},
		{
			name:    "valid size with KB unit",
			input:   "64KB",
			want:    64 * KB,
			wantErr: false,
		},
		{
			name:    "valid size with MB unit",
			input:   "512MB",
			want:    512 * MB,
			wantErr: false,
		},
		{
			name:    "valid size with GB unit",
			input:   "1GB",
			want:    1 * GB,
			wantErr: false,
		},
		{
			name:    "valid size with B unit",
			input:   "8192B",
			want:    8 * KB,
			wantErr: false,
		},
		{
			name:    "valid size with decimal",
			input:   "1.5MB",
			want:    1572864, // 1.5 * 1024 * 1024
			wantErr: false,
		},
		{
			name:    "size below minimum",
			input:   "4096",
			want:    0,
			wantErr: true,
			errMsg:  fmt.Sprintf("fragment size must be at least %s (%d bytes)", validator.FormatSize(ctx, MinFragmentSize), MinFragmentSize),
		},
		{
			name:    "size above maximum",
			input:   "2GB",
			want:    0,
			wantErr: true,
			errMsg:  fmt.Sprintf("fragment size must be at most %s (%d bytes)", validator.FormatSize(ctx, MaxFragmentSize), MaxFragmentSize),
		},
		{
			name:    "invalid number",
			input:   "abcKB",
			want:    0,
			wantErr: true,
			errMsg:  "invalid format: use raw bytes (e.g., 8192) or size with unit (e.g., 8KB, 512MB)",
		},
		{
			name:    "negative size",
			input:   "-8KB",
			want:    0,
			wantErr: true,
			errMsg:  "invalid format: use raw bytes (e.g., 8192) or size with unit (e.g., 8KB, 512MB)",
		},
		{
			name:    "multiple decimal points",
			input:   "1.2.3KB",
			want:    0,
			wantErr: true,
			errMsg:  "invalid format: multiple decimal points not allowed",
		},
		{
			name:    "unsupported unit",
			input:   "10TB",
			want:    0,
			wantErr: true,
			errMsg:  "unsupported unit 'TB': supported units are B, KB, MB, GB",
		},
		{
			name:    "empty number",
			input:   "KB",
			want:    0,
			wantErr: true,
			errMsg:  "invalid format: use raw bytes (e.g., 8192) or size with unit (e.g., 8KB, 512MB)",
		},
		{
			name:    "case insensitive unit",
			input:   "64kb",
			want:    64 * KB,
			wantErr: false,
		},
		{
			name:    "input with spaces",
			input:   "64KB",
			want:    64 * KB,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validator.ParseFragmentSize(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFragmentSize() name = %v  input = %v error = %v, wantErr %v", tt.name, tt.input, err, tt.wantErr)
				return
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ParseFragmentSize() name = %v  input = %v error message = %q, want %q", tt.name, tt.input, err.Error(), tt.errMsg)
			}
			if got != tt.want {
				t.Errorf("ParseFragmentSize() name = %v  input = %v got = %v, want %v", tt.name, tt.input, got, tt.want)
			}
		})
	}
}

// TestValidateFragmentSize tests the ValidateFragmentSize method for various size inputs.
func TestValidateFragmentSize(t *testing.T) {
	validator := New()
	ctx := context.Background()

	tests := []struct {
		name    string
		input   int64
		want    int64
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid size",
			input:   64 * KB,
			want:    64 * KB,
			wantErr: false,
		},
		{
			name:    "minimum size",
			input:   MinFragmentSize,
			want:    MinFragmentSize,
			wantErr: false,
		},
		{
			name:    "maximum size",
			input:   MaxFragmentSize,
			want:    MaxFragmentSize,
			wantErr: false,
		},
		{
			name:    "below minimum",
			input:   MinFragmentSize - 1,
			want:    0,
			wantErr: true,
			errMsg:  fmt.Sprintf("fragment size must be at least %s (%d bytes)", validator.FormatSize(ctx, MinFragmentSize), MinFragmentSize),
		},
		{
			name:    "above maximum",
			input:   MaxFragmentSize + 1,
			want:    0,
			wantErr: true,
			errMsg:  fmt.Sprintf("fragment size must be at most %s (%d bytes)", validator.FormatSize(ctx, MaxFragmentSize), MaxFragmentSize),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validator.ValidateFragmentSize(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFragmentSize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateFragmentSize() error message = %q, want %q", err.Error(), tt.errMsg)
			}
			if got != tt.want {
				t.Errorf("ValidateFragmentSize() got = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFormatSize tests the FormatSize method for various byte inputs.
func TestFormatSize(t *testing.T) {
	validator := New()
	ctx := context.Background()

	tests := []struct {
		name  string
		input int64
		want  string
	}{
		{
			name:  "bytes",
			input: 500,
			want:  "500B",
		},
		{
			name:  "kilobytes",
			input: 2048,
			want:  "2.0KB",
		},
		{
			name:  "megabytes",
			input: 10 * MB,
			want:  "10.0MB",
		},
		{
			name:  "gigabytes",
			input: 1 * GB,
			want:  "1.0GB",
		},
		{
			name:  "exact kilobyte boundary",
			input: KB,
			want:  "1.0KB",
		},
		{
			name:  "exact megabyte boundary",
			input: MB,
			want:  "1.0MB",
		},
		{
			name:  "exact gigabyte boundary",
			input: GB,
			want:  "1.0GB",
		},
		{
			name:  "fractional megabytes",
			input: 1572864, // 1.5MB
			want:  "1.5MB",
		},
		{
			name:  "fractional gigabytes",
			input: 1610612736, // 1.5GB
			want:  "1.5GB",
		},
		{
			name:  "zero bytes",
			input: 0,
			want:  "0B",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validator.FormatSize(ctx, tt.input)
			if got != tt.want {
				t.Errorf("FormatSize() got = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestContextCancellation tests the behavior of all Validator methods under context cancellation.
func TestContextCancellation(t *testing.T) {
	validator := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel the context immediately

	tests := []struct {
		name string
		fn   func(ctx context.Context) error
	}{
		{
			name: "ValidatePort",
			fn: func(ctx context.Context) error {
				_, err := validator.ValidatePort(ctx, "8080")
				return err
			},
		},
		{
			name: "ValidateLoglevel",
			fn: func(ctx context.Context) error {
				_, err := validator.ValidateLoglevel(ctx, "info")
				return err
			},
		},
		{
			name: "ParseMessageSize",
			fn: func(ctx context.Context) error {
				_, err := validator.ParseMessageSize(ctx, "100MB")
				return err
			},
		},
		{
			name: "ValidateMessageSize",
			fn: func(ctx context.Context) error {
				_, err := validator.ValidateMessageSize(ctx, 100*MB)
				return err
			},
		},
		{
			name: "ParseFragmentSize",
			fn: func(ctx context.Context) error {
				_, err := validator.ParseFragmentSize(ctx, "64KB")
				return err
			},
		},
		{
			name: "ValidateFragmentSize",
			fn: func(ctx context.Context) error {
				_, err := validator.ValidateFragmentSize(ctx, 64*KB)
				return err
			},
		},
		{
			name: "FormatSize",
			fn: func(ctx context.Context) error {
				_ = validator.FormatSize(ctx, 100*MB)
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn(ctx)
			if err != nil {
				t.Errorf("%s with cancelled context returned error: %v", tt.name, err)
			}
		})
	}
}

// TestValidateTimeout tests the ValidateTimeout method for various timeout inputs.
func TestValidateTimeout(t *testing.T) {
	validator := New()
	ctx := context.Background()

	tests := []struct {
		name    string
		param   string
		timeout time.Duration
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid timeout - 5 seconds",
			param:   "cmd-timeout",
			timeout: 5 * time.Second,
			wantErr: false,
		},
		{
			name:    "valid timeout - 1 second (minimum)",
			param:   "cmd-timeout",
			timeout: 1 * time.Second,
			wantErr: false,
		},
		{
			name:    "valid timeout - 15 minutes (maximum)",
			param:   "cmd-timeout",
			timeout: 15 * time.Minute,
			wantErr: false,
		},
		{
			name:    "invalid timeout - less than 1 second",
			param:   "cmd-timeout",
			timeout: 500 * time.Millisecond,
			wantErr: true,
			errMsg:  "cmd-timeout must be at least 1s",
		},
		{
			name:    "invalid timeout - 0 seconds",
			param:   "cmd-timeout",
			timeout: 0,
			wantErr: true,
			errMsg:  "cmd-timeout must be at least 1s",
		},
		{
			name:    "invalid timeout - negative value",
			param:   "cmd-timeout",
			timeout: -5 * time.Second,
			wantErr: true,
			errMsg:  "cmd-timeout must be at least 1s",
		},
		{
			name:    "invalid timeout - more than 15 minutes",
			param:   "cmd-timeout",
			timeout: 16 * time.Minute,
			wantErr: true,
			errMsg:  "cmd-timeout must not exceed 15m0s",
		},
		{
			name:    "invalid timeout - 1 hour",
			param:   "cmd-timeout",
			timeout: 1 * time.Hour,
			wantErr: true,
			errMsg:  "cmd-timeout must not exceed 15m0s",
		},
		{
			name:    "valid graceful-timeout - 60 seconds",
			param:   "graceful-timeout",
			timeout: 60 * time.Second,
			wantErr: false,
		},
		{
			name:    "valid graceful-timeout - 10 seconds",
			param:   "graceful-timeout",
			timeout: 10 * time.Second,
			wantErr: false,
		},
		{
			name:    "invalid graceful-timeout - less than minimum",
			param:   "graceful-timeout",
			timeout: 100 * time.Millisecond,
			wantErr: true,
			errMsg:  "graceful-timeout must be at least 1s",
		},
		{
			name:    "invalid graceful-timeout - exceeds maximum",
			param:   "graceful-timeout",
			timeout: 20 * time.Minute,
			wantErr: true,
			errMsg:  "graceful-timeout must not exceed 15m0s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateTimeout(ctx, tt.param, tt.timeout)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTimeout() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("ValidateTimeout() error message = %v, want %v", err.Error(), tt.errMsg)
			}
		})
	}
}
