# HydrAIDE Go SDK - E2E Test Configuration

This directory contains the E2E test configuration package for the HydrAIDE Go SDK.

## Setup for Development

### 1. Create your local .env file

Copy the example environment file and fill in your actual paths:

```bash
cp .env.example .env
```

### 2. Edit .env with your certificate paths

Open the `.env` file and update the paths to point to your actual certificate files:

```dotenv
# Server Certificate Files
HYDRAIDE_E2E_SERVER_CERT_FILE=/path/to/your/server.crt
HYDRAIDE_E2E_SERVER_KEY_FILE=/path/to/your/server.key

# Certificate Authority
HYDRAIDE_E2E_CA_CERT_FILE=/path/to/your/ca.crt

# Client Certificate Files
HYDRAIDE_E2E_CLIENT_CERT_FILE=/path/to/your/client.crt
HYDRAIDE_E2E_CLIENT_KEY_FILE=/path/to/your/client.key

# Test Server Address
HYDRAIDE_E2E_TEST_SERVER_ADDR=localhost:50051

# Optional: Enable gRPC connection analysis
HYDRAIDE_E2E_GRPC_CONN_ANALYSIS=false
```

### 3. Run the tests

```bash
go test -v ./...
```

## Environment Variables

| Variable Name | Required | Description | Example |
|--------------|----------|-------------|---------|
| `HYDRAIDE_E2E_SERVER_CERT_FILE` | Yes | Path to server TLS certificate | `/path/to/server.crt` |
| `HYDRAIDE_E2E_SERVER_KEY_FILE` | Yes | Path to server TLS private key | `/path/to/server.key` |
| `HYDRAIDE_E2E_CA_CERT_FILE` | Yes | Path to CA certificate | `/path/to/ca.crt` |
| `HYDRAIDE_E2E_CLIENT_CERT_FILE` | Yes | Path to client TLS certificate | `/path/to/client.crt` |
| `HYDRAIDE_E2E_CLIENT_KEY_FILE` | Yes | Path to client TLS private key | `/path/to/client.key` |
| `HYDRAIDE_E2E_TEST_SERVER_ADDR` | Yes | Test server address (host:port) | `localhost:50051` |
| `HYDRAIDE_E2E_GRPC_CONN_ANALYSIS` | No | Enable gRPC connection debugging | `true` or `false` (default: `false`) |

## Notes

- The `.env` file is excluded from version control (listed in `.gitignore`)
- Never commit sensitive certificate paths or the `.env` file to the repository
- Each developer should maintain their own `.env` file with their local certificate paths
- All environment variable names follow the pattern: `HYDRAIDE_E2E_*` for consistency

## Config Package Usage

The config package automatically:
1. Loads environment variables from `.env` file (if it exists)
2. Falls back to system environment variables
3. Validates that all required variables are set
4. Validates that all certificate files exist
5. Provides clear error messages if something is missing

Example usage in your own code:

```go
import "github.com/hydraide/hydraide/sdk/go/hydraidego/config"

cfg, err := config.LoadE2ETestConfig()
if err != nil {
    log.Fatalf("Failed to load config: %v", err)
}

if err := cfg.Validate(); err != nil {
    log.Fatalf("Config validation failed: %v", err)
}

// Use cfg.ServerCertFile, cfg.ClientCertFile, etc.
```
