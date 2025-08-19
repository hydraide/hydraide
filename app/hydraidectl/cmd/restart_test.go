package cmd

import (
	"testing"
	"time"
)

func TestValidateRestartTimeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid timeout - 30 seconds (default)",
			timeout: 30 * time.Second,
			wantErr: false,
		},
		{
			name:    "valid timeout - 1 minute",
			timeout: 1 * time.Minute,
			wantErr: false,
		},
		{
			name:    "valid timeout - 1 second (minimum)",
			timeout: 1 * time.Second,
			wantErr: false,
		},
		{
			name:    "valid timeout - 15 minutes (maximum)",
			timeout: 15 * time.Minute,
			wantErr: false,
		},
		{
			name:    "invalid timeout - less than 1 second",
			timeout: 500 * time.Millisecond,
			wantErr: true,
			errMsg:  "cmd-timeout must be at least 1s",
		},
		{
			name:    "invalid timeout - 0 seconds",
			timeout: 0,
			wantErr: true,
			errMsg:  "cmd-timeout must be at least 1s",
		},
		{
			name:    "invalid timeout - negative value",
			timeout: -10 * time.Second,
			wantErr: true,
			errMsg:  "cmd-timeout must be at least 1s",
		},
		{
			name:    "invalid timeout - more than 15 minutes",
			timeout: 16 * time.Minute,
			wantErr: true,
			errMsg:  "cmd-timeout must not exceed 15m0s",
		},
		{
			name:    "invalid timeout - 2 hours",
			timeout: 2 * time.Hour,
			wantErr: true,
			errMsg:  "cmd-timeout must not exceed 15m0s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTimeoutValue("cmd-timeout", tt.timeout)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTimeoutValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("validateTimeoutValue() error message = %v, want %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestValidateRestartGracefulTimeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid graceful-timeout - 10 seconds (default)",
			timeout: 10 * time.Second,
			wantErr: false,
		},
		{
			name:    "valid graceful-timeout - 1 minute",
			timeout: 1 * time.Minute,
			wantErr: false,
		},
		{
			name:    "valid graceful-timeout - 15 minutes (maximum)",
			timeout: 15 * time.Minute,
			wantErr: false,
		},
		{
			name:    "invalid graceful-timeout - less than minimum",
			timeout: 900 * time.Millisecond,
			wantErr: true,
			errMsg:  "graceful-timeout must be at least 1s",
		},
		{
			name:    "invalid graceful-timeout - exceeds maximum",
			timeout: 30 * time.Minute,
			wantErr: true,
			errMsg:  "graceful-timeout must not exceed 15m0s",
		},
		{
			name:    "edge case - exactly at minimum",
			timeout: 1 * time.Second,
			wantErr: false,
		},
		{
			name:    "edge case - exactly at maximum",
			timeout: 15 * time.Minute,
			wantErr: false,
		},
		{
			name:    "warning threshold - 1.5 seconds",
			timeout: 1500 * time.Millisecond,
			wantErr: false,
		},
		{
			name:    "warning threshold - 2 seconds",
			timeout: 2 * time.Second,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTimeoutValue("graceful-timeout", tt.timeout)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTimeoutValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("validateTimeoutValue() error message = %v, want %v", err.Error(), tt.errMsg)
			}
		})
	}
}
