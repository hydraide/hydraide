package env

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
)

type Env interface {
	// IsExists checks if the .env file exists in the specified base path.
	IsExists(ctx context.Context) bool
	// GetEnvPath returns the path to the .env file.
	GetEnvPath() string
	// Set writes the psrovided settings to the .env file in the specified base path.
	Set(ctx context.Context, s *Settings) error
	// Load reads the .env file and parses the settings into a Settings struct.
	Load(ctx context.Context) (*Settings, error)
}

type Settings struct {
	LogLevel                string
	SystemResourceLogging   bool
	GraylogEnabled          bool
	GraylogServer           string
	GraylogServiceName      string
	GRPCMaxMessageSize      int64
	GRPCServerErrorLogging  bool
	HydrAIDEBasePath        string
	HydrAIDEGRPCPort        string
	HydrAIDEHealthCheckPort string
	SwampCloseAfterIdle     int
	SwampWriteInterval      int
	SwampDefaultFileSize    int64
}

type env struct {
	fs          filesystem.FileSystem
	basePath    string
	envPath     string
	settingsMap map[string]string
}

func New(fs filesystem.FileSystem, basePath string) Env {
	return &env{
		fs:      fs,
		envPath: filepath.Join(basePath, ".env"),
	}
}

// GetEnvPath returns the path to the .env file.
func (e *env) GetEnvPath() string {
	return e.envPath
}

// IsExists check if the .env file exists in the specified base path.
func (e *env) IsExists(ctx context.Context) bool {
	exists, err := e.fs.CheckIfFileExists(ctx, e.envPath)
	if err != nil {
		return false
	}
	if !exists {
		return false
	}
	return true
}

// Set writes the provided settings to the .env file in the specified base path.
func (e *env) Set(ctx context.Context, s *Settings) error {

	var sb strings.Builder
	sb.WriteString("# HydrAIDE Configuration\n")
	sb.WriteString("# Generated automatically - DO NOT EDIT MANUALLY\n\n")
	sb.WriteString(fmt.Sprintf("LOG_LEVEL=%s\n", s.LogLevel))
	sb.WriteString("LOG_TIME_FORMAT=2006-01-02T15:04:05Z07:00\n")
	sb.WriteString(fmt.Sprintf("SYSTEM_RESOURCE_LOGGING=%t\n", s.SystemResourceLogging))
	sb.WriteString(fmt.Sprintf("GRAYLOG_ENABLED=%t\n", s.GraylogEnabled))
	sb.WriteString(fmt.Sprintf("GRAYLOG_SERVER=%s\n", s.GraylogServer))
	sb.WriteString(fmt.Sprintf("GRAYLOG_SERVICE_NAME=%s\n", s.GraylogServiceName))
	sb.WriteString(fmt.Sprintf("GRPC_MAX_MESSAGE_SIZE=%d\n", s.GRPCMaxMessageSize))
	sb.WriteString(fmt.Sprintf("GRPC_SERVER_ERROR_LOGGING=%t\n", s.GRPCServerErrorLogging))
	sb.WriteString(fmt.Sprintf("HYDRAIDE_ROOT_PATH=%s\n", s.HydrAIDEBasePath))
	sb.WriteString(fmt.Sprintf("HYDRAIDE_SERVER_PORT=%s\n", s.HydrAIDEGRPCPort))
	sb.WriteString(fmt.Sprintf("HYDRAIDE_DEFAULT_CLOSE_AFTER_IDLE=%d\n", s.SwampCloseAfterIdle))
	sb.WriteString(fmt.Sprintf("HYDRAIDE_DEFAULT_WRITE_INTERVAL=%d\n", s.SwampWriteInterval))
	sb.WriteString(fmt.Sprintf("HYDRAIDE_DEFAULT_FILE_SIZE=%d\n", s.SwampDefaultFileSize))
	sb.WriteString(fmt.Sprintf("HEALTH_CHECK_PORT=%s\n", s.HydrAIDEHealthCheckPort))
	sb.WriteString("\n")

	content := []byte(sb.String())
	if err := e.fs.WriteFile(ctx, e.envPath, content, 0644); err != nil {
		return err
	}

	return nil

}

// Load reads the .env file and parses the settings into a Settings struct.
func (e *env) Load(ctx context.Context) (*Settings, error) {

	// load the file from the filesystem
	bytes, err := e.fs.ReadFile(ctx, e.envPath)
	if err != nil {
		return nil, err
	}

	// iterating through the lines to parse the settings
	lines := strings.Split(string(bytes), "\n")
	settings := &Settings{}
	for _, line := range lines {
		if strings.HasPrefix(line, "LOG_LEVEL") {
			settings.LogLevel = strings.TrimSpace(strings.Split(line, "=")[1])
			continue
		}
		if strings.HasPrefix(line, "SYSTEM_RESOURCE_LOGGING") {
			settings.SystemResourceLogging = strings.TrimSpace(strings.Split(line, "=")[1]) == "true"
			continue
		}
		if strings.HasPrefix(line, "GRAYLOG_ENABLED") {
			settings.GraylogEnabled = strings.TrimSpace(strings.Split(line, "=")[1]) == "true"
			continue
		}
		if strings.HasPrefix(line, "GRAYLOG_SERVER") {
			settings.GraylogServer = strings.TrimSpace(strings.Split(line, "=")[1])
			continue
		}
		if strings.HasPrefix(line, "GRAYLOG_SERVICE_NAME") {
			settings.GraylogServiceName = strings.TrimSpace(strings.Split(line, "=")[1])
			continue
		}
		if strings.HasPrefix(line, "GRPC_MAX_MESSAGE_SIZE") {
			sizeStr := strings.TrimSpace(strings.Split(line, "=")[1])
			var size int64
			_, _ = fmt.Sscanf(sizeStr, "%d", &size)
			settings.GRPCMaxMessageSize = size
			continue
		}
		if strings.HasPrefix(line, "GRPC_SERVER_ERROR_LOGGING") {
			settings.GRPCServerErrorLogging = strings.TrimSpace(strings.Split(line, "=")[1]) == "true"
			continue
		}
		if strings.HasPrefix(line, "HYDRAIDE_ROOT_PATH") {
			settings.HydrAIDEBasePath = strings.TrimSpace(strings.Split(line, "=")[1])
			continue
		}
		if strings.HasPrefix(line, "HYDRAIDE_SERVER_PORT") {
			settings.HydrAIDEGRPCPort = strings.TrimSpace(strings.Split(line, "=")[1])
			continue
		}
		if strings.HasPrefix(line, "HYDRAIDE_DEFAULT_CLOSE_AFTER_IDLE") {
			closeAfterIdleStr := strings.TrimSpace(strings.Split(line, "=")[1])
			var closeAfterIdle int
			_, _ = fmt.Sscanf(closeAfterIdleStr, "%d", &closeAfterIdle)
			settings.SwampCloseAfterIdle = closeAfterIdle
			continue
		}
		if strings.HasPrefix(line, "HYDRAIDE_DEFAULT_WRITE_INTERVAL") {
			writeIntervalStr := strings.TrimSpace(strings.Split(line, "=")[1])
			var writeInterval int
			_, _ = fmt.Sscanf(writeIntervalStr, "%d", &writeInterval)
			settings.SwampWriteInterval = writeInterval
			continue
		}
		if strings.HasPrefix(line, "HYDRAIDE_DEFAULT_FILE_SIZE") {
			fileSizeStr := strings.TrimSpace(strings.Split(line, "=")[1])
			var fileSize int64
			_, _ = fmt.Sscanf(fileSizeStr, "%d", &fileSize)
			settings.SwampDefaultFileSize = fileSize
			continue
		}
		if strings.HasPrefix(line, "HEALTH_CHECK_PORT") {
			settings.HydrAIDEHealthCheckPort = strings.TrimSpace(strings.Split(line, "=")[1])
			continue
		}
	}

	return settings, nil

}
