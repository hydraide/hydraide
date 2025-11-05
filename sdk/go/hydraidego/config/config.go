package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// E2ETestConfig contains all environment variables needed for E2E tests
type E2ETestConfig struct {
	// Server certificate files
	ServerCertFile string
	ServerKeyFile  string
	CACertFile     string

	// Client certificate files
	ClientCertFile string
	ClientKeyFile  string

	// Server connection details
	TestServerAddr string // Format: "host:port"

	// Optional settings
	GRPCConnAnalysis bool
}

// LoadE2ETestConfig loads the E2E test configuration from environment variables
// It first attempts to load a .env file from the SDK directory if it exists
func LoadE2ETestConfig() (*E2ETestConfig, error) {
	// Try to load .env file from multiple possible locations
	possiblePaths := []string{
		".env",                            // Current directory (when running tests from sdk/go/hydraidego)
		"../../../sdk/go/hydraidego/.env", // From nested test directories
	}

	loaded := false
	for _, envPath := range possiblePaths {
		if err := godotenv.Load(envPath); err == nil {
			slog.Info("Loaded .env file successfully", "path", envPath)
			loaded = true
			break
		}
	}

	if !loaded {
		slog.Debug("No .env file found in any expected location, using system environment variables")
	}

	cfg := &E2ETestConfig{}

	// Load required environment variables
	var missing []string

	cfg.ServerCertFile = os.Getenv("HYDRAIDE_E2E_SERVER_CERT_FILE")
	if cfg.ServerCertFile == "" {
		missing = append(missing, "HYDRAIDE_E2E_SERVER_CERT_FILE")
	}

	cfg.ServerKeyFile = os.Getenv("HYDRAIDE_E2E_SERVER_KEY_FILE")
	if cfg.ServerKeyFile == "" {
		missing = append(missing, "HYDRAIDE_E2E_SERVER_KEY_FILE")
	}

	cfg.CACertFile = os.Getenv("HYDRAIDE_E2E_CA_CERT_FILE")
	if cfg.CACertFile == "" {
		missing = append(missing, "HYDRAIDE_E2E_CA_CERT_FILE")
	}

	cfg.ClientCertFile = os.Getenv("HYDRAIDE_E2E_CLIENT_CERT_FILE")
	if cfg.ClientCertFile == "" {
		missing = append(missing, "HYDRAIDE_E2E_CLIENT_CERT_FILE")
	}

	cfg.ClientKeyFile = os.Getenv("HYDRAIDE_E2E_CLIENT_KEY_FILE")
	if cfg.ClientKeyFile == "" {
		missing = append(missing, "HYDRAIDE_E2E_CLIENT_KEY_FILE")
	}

	cfg.TestServerAddr = os.Getenv("HYDRAIDE_E2E_TEST_SERVER_ADDR")
	if cfg.TestServerAddr == "" {
		missing = append(missing, "HYDRAIDE_E2E_TEST_SERVER_ADDR")
	}

	// Check if any required variables are missing
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %v", missing)
	}

	// Load optional settings
	grpcConnAnalysisStr := os.Getenv("HYDRAIDE_E2E_GRPC_CONN_ANALYSIS")
	if grpcConnAnalysisStr == "" {
		cfg.GRPCConnAnalysis = false
	} else {
		var err error
		cfg.GRPCConnAnalysis, err = strconv.ParseBool(grpcConnAnalysisStr)
		if err != nil {
			slog.Warn("Invalid value for HYDRAIDE_E2E_GRPC_CONN_ANALYSIS, defaulting to false", "value", grpcConnAnalysisStr)
			cfg.GRPCConnAnalysis = false
		}
	}

	return cfg, nil
}

// Validate checks if all required files exist
func (c *E2ETestConfig) Validate() error {
	filesToCheck := map[string]string{
		"server certificate": c.ServerCertFile,
		"server key":         c.ServerKeyFile,
		"CA certificate":     c.CACertFile,
		"client certificate": c.ClientCertFile,
		"client key":         c.ClientKeyFile,
	}

	for name, path := range filesToCheck {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return fmt.Errorf("%s file not found: %s", name, path)
		} else if err != nil {
			return fmt.Errorf("error checking %s file: %w", name, err)
		}
	}

	return nil
}
