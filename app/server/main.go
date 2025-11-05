package main

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"syscall"
	"time"

	"github.com/hydraide/hydraide/app/panichandler"
	"github.com/hydraide/hydraide/app/paniclogger"
	"github.com/hydraide/hydraide/app/server/loghandlers/fallback"
	"github.com/hydraide/hydraide/app/server/loghandlers/graylog"
	"github.com/hydraide/hydraide/app/server/loghandlers/slogmulti"
	"github.com/hydraide/hydraide/app/server/server"

	"github.com/joho/godotenv"
)

var serverInterface server.Server

var (
	graylogServer         = ""
	graylogServiceName    = "HydrAIDE-Server"
	logLevel              = "debug"
	localLoggingEnabled   = false
	hydraMaxMessageSize   = 104857600   // 100 MB
	defaultCloseAfterIdle = int64(1)    // 1 second
	defaultWriteInterval  = int64(10)   // 10 seconds
	defaultFileSize       = int64(8192) // 8 KB
	systemResourceLogging = false
	serverCrtPath         = ""
	serverKeyPath         = ""
	clientCaCrtPath       = ""
	hydraServerPort       = 4444
	healthCheckPort       = 4445
)

const (
	hydrAIDEDefaultRootPath = "/hydraide"
)

func init() {

	// Load environment variables from .env files before anything else
	_ = godotenv.Load()

	// check if the HYDRAIDE_SERVER_PORT and HEALTH_CHECK_PORT environment variables are set
	var err error
	if os.Getenv("HYDRAIDE_SERVER_PORT") != "" {
		if hydraServerPort, err = strconv.Atoi(os.Getenv("HYDRAIDE_SERVER_PORT")); err != nil {
			panic(fmt.Sprintf("HYDRAIDE_SERVER_PORT must be a number without any string characters: %v", err))
		}
	}
	if os.Getenv("HEALTH_CHECK_PORT") != "" {
		if healthCheckPort, err = strconv.Atoi(os.Getenv("HEALTH_CHECK_PORT")); err != nil {
			panic(fmt.Sprintf("HEALTH_CHECK_PORT must be a number without any string characters: %v", err))
		}
	}

	if os.Getenv("HYDRAIDE_ROOT_PATH") == "" {
		// for the docker container, the hydrAIDE root path is set to /hydraide
		// needed, because we use this env variable in the settings package, too
		if err := os.Setenv("HYDRAIDE_ROOT_PATH", hydrAIDEDefaultRootPath); err != nil {
			panic(fmt.Sprintf("failed to set HYDRAIDE_ROOT_PATH environment variable: %v", err))
		}
	}

	// should be handled these for linux and windows
	serverCrtPath = filepath.Join(os.Getenv("HYDRAIDE_ROOT_PATH"), "certificate", "server.crt")
	serverKeyPath = filepath.Join(os.Getenv("HYDRAIDE_ROOT_PATH"), "certificate", "server.key")
	clientCaCrtPath = filepath.Join(os.Getenv("HYDRAIDE_ROOT_PATH"), "certificate", "ca.crt")

	if _, err := os.Stat(serverCrtPath); os.IsNotExist(err) {
		slog.Error("server certificate file server.crt are not found", "error", err.Error())
		panic(fmt.Sprintf("server certificate file server.crt are not found in %s", serverCrtPath))
	}

	// check if the server key and certificate files exist
	if _, err := os.Stat(serverCrtPath); os.IsNotExist(err) {
		slog.Error("server certificate file server.crt are not found", "error", err.Error())
		panic(fmt.Sprintf("server certificate file server.crt are not found in %s", serverCrtPath))
	}
	if _, err := os.Stat(serverKeyPath); os.IsNotExist(err) {
		slog.Error("server certificate file server.key are not found", "error", err.Error())
		panic(fmt.Sprintf("server certificate file server.key are not found in %s", serverKeyPath))
	}

	// log level must have
	if os.Getenv("LOG_LEVEL") != "" {
		logLevel = os.Getenv("LOG_LEVEL")
	}

	if os.Getenv("LOCAL_LOGGING_ENABLED") == "true" {
		localLoggingEnabled = true // default local logging is disabled
	}

	if os.Getenv("SYSTEM_RESOURCE_LOGGING") == "true" {
		systemResourceLogging = true // default system resource logging is disabled
	}

	if os.Getenv("GRAYLOG_ENABLED") == "true" {
		if os.Getenv("GRAYLOG_SERVER") != "" {
			graylogServer = os.Getenv("GRAYLOG_SERVER")
		}
		// GRAYLOG_SERVICE_NAME is optional environment variable. Set the graylog service name only if it is set
		if os.Getenv("GRAYLOG_SERVICE_NAME") != "" {
			graylogServiceName = os.Getenv("GRAYLOG_SERVICE_NAME")
		}
	}

	// HYDRA_MAX_MESSAGE_SIZE environment variable
	if os.Getenv("GRPC_MAX_MESSAGE_SIZE") != "" {
		var err error
		hydraMaxMessageSize, err = strconv.Atoi(os.Getenv("GRPC_MAX_MESSAGE_SIZE"))
		if err != nil {
			slog.Error("GRPC_MAX_MESSAGE_SIZE must be a number without any string characters", "error", err)
			panic("GRPC_MAX_MESSAGE_SIZE must be a number without any string characters")
		}
	}

	if os.Getenv("HYDRAIDE_DEFAULT_CLOSE_AFTER_IDLE") != "" {
		dcai, err := strconv.Atoi(os.Getenv("HYDRAIDE_DEFAULT_CLOSE_AFTER_IDLE"))
		if err != nil {
			slog.Error("HYDRAIDE_DEFAULT_CLOSE_AFTER_IDLE must be a number without any string characters", "error", err)
			panic("HYDRAIDE_DEFAULT_CLOSE_AFTER_IDLE must be a number without any string characters")
		}
		defaultCloseAfterIdle = int64(dcai)
	}

	if os.Getenv("HYDRAIDE_DEFAULT_WRITE_INTERVAL") != "" {
		dwi, err := strconv.Atoi(os.Getenv("HYDRAIDE_DEFAULT_WRITE_INTERVAL"))
		if err != nil {
			slog.Error("HYDRAIDE_DEFAULT_WRITE_INTERVAL must be a number without any string characters", "error", err)
			panic("HYDRAIDE_DEFAULT_WRITE_INTERVAL must be a number without any string characters")
		}
		defaultWriteInterval = int64(dwi)
	}

	if os.Getenv("HYDRAIDE_DEFAULT_FILE_SIZE") != "" {
		dfs, err := strconv.Atoi(os.Getenv("HYDRAIDE_DEFAULT_FILE_SIZE"))
		if err != nil {
			slog.Error("HYDRAIDE_DEFAULT_FILE_SIZE must be a number without any string characters", "error", err)
			panic("HYDRAIDE_DEFAULT_FILE_SIZE must be a number without any string characters")
		}
		defaultFileSize = int64(dfs)
	}

}

func main() {

	defer panicHandler()

	// ----------------------------------------------------------------------------
	// Panic logger inicializálása - MINDIG aktív
	// ----------------------------------------------------------------------------
	if err := paniclogger.Init(); err != nil {
		fmt.Printf("WARNING: failed to initialize panic logger: %v\n", err)
	}
	defer func() {
		if err := paniclogger.Close(); err != nil {
			fmt.Printf("WARNING: failed to close panic logger: %v\n", err)
		}
	}()

	// ----------------------------------------------------------------------------
	// Logger setup with console output + optional Graylog (+ optional file fallback)
	// ----------------------------------------------------------------------------
	// Logging architecture:
	// - Always: logs go to console
	// - If Graylog is enabled AND defined:
	//   - logs go to Graylog
	//   - if LOCAL_LOGGING_ENABLED=true: logs also go to fallback.log when Graylog is down
	//   - if LOCAL_LOGGING_ENABLED=false: logs go ONLY to Graylog (no fallback file)
	// - If Graylog is undefined: logs go ONLY to console (no file write)
	// - Panic logs: ALWAYS go to panic.log (regardless of any setting)
	// ----------------------------------------------------------------------------

	ll := parseLogLevel(logLevel)
	graylogAvailable := graylogServer != ""

	// Console handler — always active
	consoleHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: ll,
	})

	// Optional Graylog + optional fallback setup
	var combinedHandler slog.Handler

	if graylogAvailable {
		// Attempt to connect to Graylog
		gh, err := graylog.New(graylogServer, graylogServiceName, ll)
		if err != nil {
			fmt.Printf("failed to connect to Graylog: %v\n", err)
			graylogAvailable = false
		} else {
			defer func() { _ = gh.Close() }()
			slog.Info("Graylog handler initialized",
				slog.String("server", graylogServer),
				slog.String("service", graylogServiceName))

			// Only use local file fallback if explicitly enabled
			if localLoggingEnabled {
				// Local file fallback (only enabled if user explicitly enables it)
				localHandler := fallback.LocalHandler(ll)

				combinedHandler = fallback.New(
					gh,
					localHandler,
					func() bool {
						conn, err := net.DialTimeout("tcp", graylogServer, 1*time.Second)
						if err != nil {
							return false
						}
						_ = conn.Close()
						return true
					},
				)
				slog.Info("Local logging fallback enabled (will write to fallback.log if Graylog is unavailable)")
			} else {
				// Use Graylog directly without file fallback
				combinedHandler = gh
				slog.Info("Local logging fallback disabled (logs will be lost if Graylog is unavailable)")
			}
		}
	}

	// Final logger: console only, or console + Graylog (+ optional fallback)
	if combinedHandler != nil {
		logger := slog.New(slogmulti.New(consoleHandler, combinedHandler))
		slog.SetDefault(logger)
	} else {
		logger := slog.New(consoleHandler)
		slog.SetDefault(logger)
	}

	slog.Info("logger successfully initialized")

	// start the new Hydra server
	serverInterface = server.New(&server.Configuration{
		CertificateCrtFile:    serverCrtPath,
		CertificateKeyFile:    serverKeyPath,
		ClientCAFile:          clientCaCrtPath,
		HydraServerPort:       hydraServerPort,
		HydraMaxMessageSize:   hydraMaxMessageSize,
		DefaultCloseAfterIdle: defaultCloseAfterIdle,
		DefaultWriteInterval:  defaultWriteInterval,
		DefaultFileSize:       defaultFileSize,
		SystemResourceLogging: systemResourceLogging,
	})

	if err := serverInterface.Start(); err != nil {
		slog.Error("HydrAIDE server is not running", "error", err)
		panic(fmt.Sprintf("HydrAIDE server is not running: %v", err))
	}

	panichandler.SafeGo("health-check-server", func() {
		http.HandleFunc("/health", healthCheckHandler)
		port := fmt.Sprintf(":%d", healthCheckPort)
		if err := http.ListenAndServe(port, nil); err != nil {
			slog.Error("http server error - health check server is not running", "error", err)
		}
	})

	// blocker for the main goroutine and waiting for kill signal
	waitingForKillSignal()

}

func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func panicHandler() {
	if r := recover(); r != nil {
		// get the stack trace
		stackTrace := debug.Stack()

		// Always write panic to panic.log
		paniclogger.LogPanic("main function", r, string(stackTrace))

		// Log the panic error and stack trace to console/Graylog
		slog.Error("caught panic", "error", r, "stack", string(stackTrace))

		// get the graceful stop
		gracefulStop()
	}
}

func gracefulStop() {
	// stop the microservice and exit the program
	serverInterface.Stop()
	slog.Info("hydra server stopped gracefully. Program is exiting...")
	// waiting for logs to be written to the file
	time.Sleep(1 * time.Second)

	// Close panic logger before exit
	if err := paniclogger.Close(); err != nil {
		fmt.Printf("WARNING: failed to close panic logger: %v\n", err)
	}

	// exit the program if the microservice is stopped gracefully
	os.Exit(0)
}

func waitingForKillSignal() {
	slog.Info("HydrAIDE server waiting for kill signal")
	gracefulStopSignal := make(chan os.Signal, 1)
	signal.Notify(gracefulStopSignal, syscall.SIGKILL, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	// waiting for graceful stop signal
	<-gracefulStopSignal
	slog.Info("kill signal received, stopping the server gracefully")
	gracefulStop()
}

func healthCheckHandler(w http.ResponseWriter, _ *http.Request) {
	if serverInterface == nil {
		// unhealthy
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !serverInterface.IsHydraRunning() {
		// unhealthy
		w.WriteHeader(http.StatusInternalServerError)
	}
	// healthy
	w.WriteHeader(http.StatusOK)
}
