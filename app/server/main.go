package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/hydraide/hydraide/app/graylog"
	"github.com/hydraide/hydraide/app/server/server"
	"github.com/sirupsen/logrus"
)

var serverInterface server.Server

var (
	graylogServer         = ""
	graylogServiceName    = ""
	logLevel              = "trace"
	logTimeFormat         = "2006-01-02 15:04:05"
	hydraMaxMessageSize   = 1024 * 1024 * 1024 * 5 // 5 GB
	defaultCloseAfterIdle = int64(0)
	defaultWriteInterval  = int64(0)
	defaultFileSize       = int64(0) // 1 GB
	systemResourceLogging = false
	serverCrtPath         = ""
	serverKeyPath         = ""
	hydraServerPort       = int(0)
	healthCheckPort       = int(0)
)

func init() {

	// Load environment variables from .env files before anything else
	_ = godotenv.Load()

	// check if the server key and certificate files exist
	isServerCertificateCrtOk := true
	isServerCertificateKeyOk := true

	// check if the HYDRAIDE_SERVER_PORT and HEALTH_CHECK_PORT environment variables are set
	var err error
	if os.Getenv("HYDRAIDE_SERVER_PORT") == "" {
		logrus.WithFields(logrus.Fields{
			"error": "HYDRAIDE_SERVER_PORT is not set",
		}).Panic("HYDRAIDE_SERVER_PORT environment variable is not set")
	}
	if os.Getenv("HEALTH_CHECK_PORT") == "" {
		logrus.WithFields(logrus.Fields{
			"error": "HEALTH_CHECK_PORT is not set",
		}).Panic("HEALTH_CHECK_PORT environment variable is not set")
	}

	// convert the HYDRAIDE_SERVER_PORT and HEALTH_CHECK_PORT environment variables to integers
	if hydraServerPort, err = strconv.Atoi(os.Getenv("HYDRAIDE_SERVER_PORT")); err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Panic("HYDRAIDE_SERVER_PORT must be a number without any string characters")
	}

	if healthCheckPort, err = strconv.Atoi(os.Getenv("HEALTH_CHECK_PORT")); err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Panic("HEALTH_CHECK_PORT must be a number without any string characters")
	}

	if os.Getenv("HYDRAIDE_ROOT_PATH") == "" {
		logrus.WithFields(logrus.Fields{
			"error": "HYDRAIDE_ROOT_PATH is not set",
		}).Panic("HYDRAIDE_ROOT_PATH environment variable is not set")
	}

	// should be handled these for linux and windows
	serverCrtPath = filepath.Join(os.Getenv("HYDRAIDE_ROOT_PATH"), "certificate", "ca.crt")
	serverKeyPath = filepath.Join(os.Getenv("HYDRAIDE_ROOT_PATH"), "certificate", "ca.key")

	if _, err := os.Stat(serverCrtPath); os.IsNotExist(err) {
		isServerCertificateCrtOk = false
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Errorf("server certificate file server.crt are not found in %s", serverCrtPath)
	}
	if _, err := os.Stat(serverKeyPath); os.IsNotExist(err) {
		isServerCertificateKeyOk = false
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Errorf("server certificate file server.key are not found in %s", serverKeyPath)
	}

	// stops the program if the server certificate files are not found
	if !isServerCertificateCrtOk || !isServerCertificateKeyOk {
		logrus.Panic("one of the server certificate files are not found. program terminated")
	}

	// log level must have
	if os.Getenv("LOG_LEVEL") == "" {
		logrus.WithFields(logrus.Fields{
			"error": "LOG_LEVEL is not set",
		}).Panic("LOG_LEVEL environment variable is not set")
	} else {
		logLevel = os.Getenv("LOG_LEVEL")
		if logLevel == "" {
			logrus.WithFields(logrus.Fields{
				"error": "LOG_LEVEL is not set",
			}).Panic("necessary LOG_LEVEL environment variable is not set")
		}
	}

	if os.Getenv("LOG_TIME_FORMAT") == "" {
		logrus.WithFields(logrus.Fields{
			"error": "LOG_TIME_FORMAT is not set",
		}).Panic("LOG_TIME_FORMAT environment variable is not set")
	} else {
		logTimeFormat = os.Getenv("LOG_TIME_FORMAT")
		if logTimeFormat == "" {
			logrus.WithFields(logrus.Fields{
				"error": "LOG_TIME_FORMAT is not set",
			}).Panic("necessary LOG_TIME_FORMAT environment variable is not set")
		}
	}

	if os.Getenv("SYSTEM_RESOURCE_LOGGING") == "" {
		logrus.WithFields(logrus.Fields{
			"error": "SYSTEM_RESOURCE_LOGGING is not set",
		}).Panic("SYSTEM_RESOURCE_LOGGING environment variable is not set")
	} else {
		if os.Getenv("SYSTEM_RESOURCE_LOGGING") == "true" {
			systemResourceLogging = true
		}
	}

	if os.Getenv("GRAYLOG_ENABLED") == "true" {

		if os.Getenv("GRAYLOG_SERVER") != "" {
			graylogServer = os.Getenv("GRAYLOG_SERVER")
			if graylogServer == "" {
				logrus.WithFields(logrus.Fields{
					"error": "GRAYLOG_SERVER is not set",
				}).Panic("necessary GRAYLOG_SERVER environment variable is not set")
			}
		}
		// GRAYLOG_SERVICE_NAME is optional environment variable. Set the graylog service name only if it is set
		if os.Getenv("GRAYLOG_SERVICE_NAME") != "" {
			graylogServiceName = os.Getenv("GRAYLOG_SERVICE_NAME")
			if graylogServiceName == "" {
				logrus.WithFields(logrus.Fields{
					"error": "GRAYLOG_SERVICE_NAME is not set",
				}).Panic("necessary GRAYLOG_SERVICE_NAME environment variable is not set")
			}
		}

	}

	// HYDRA_MAX_MESSAGE_SIZE environment variable
	if os.Getenv("GRPC_MAX_MESSAGE_SIZE") == "" {
		logrus.WithFields(logrus.Fields{
			"error": "GRPC_MAX_MESSAGE_SIZE is not set",
		}).Panic("necessary GRPC_MAX_MESSAGE_SIZE environment variable is not set")
	} else {
		var err error
		hydraMaxMessageSize, err = strconv.Atoi(os.Getenv("GRPC_MAX_MESSAGE_SIZE"))
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"error": err,
			}).Panic("GRPC_MAX_MESSAGE_SIZE must be a number without any string characters")
		}
	}

	if os.Getenv("HYDRAIDE_DEFAULT_CLOSE_AFTER_IDLE") == "" {
		logrus.WithFields(logrus.Fields{
			"error": "HYDRAIDE_DEFAULT_CLOSE_AFTER_IDLE is not set",
		}).Panic("necessary HYDRAIDE_DEFAULT_CLOSE_AFTER_IDLE environment variable is not set")
	} else {
		dcai, err := strconv.Atoi(os.Getenv("HYDRAIDE_DEFAULT_CLOSE_AFTER_IDLE"))
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"error": err,
			}).Panic("HYDRAIDE_DEFAULT_CLOSE_AFTER_IDLE must be a number without any string characters")
		}
		defaultCloseAfterIdle = int64(dcai)
	}

	if os.Getenv("HYDRAIDE_DEFAULT_WRITE_INTERVAL") == "" {
		logrus.WithFields(logrus.Fields{
			"error": "HYDRAIDE_DEFAULT_WRITE_INTERVAL is not set",
		}).Panic("necessary HYDRAIDE_DEFAULT_WRITE_INTERVAL environment variable is not set")
	} else {
		dwi, err := strconv.Atoi(os.Getenv("HYDRAIDE_DEFAULT_WRITE_INTERVAL"))
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"error": err,
			}).Panic("HYDRAIDE_DEFAULT_WRITE_INTERVAL must be a number without any string characters")
		}
		defaultWriteInterval = int64(dwi)
	}

	if os.Getenv("HYDRAIDE_DEFAULT_FILE_SIZE") == "" {
		logrus.WithFields(logrus.Fields{
			"error": "HYDRAIDE_DEFAULT_FILE_SIZE is not set",
		}).Panic("necessary HYDRAIDE_DEFAULT_FILE_SIZE environment variable is not set")
	} else {
		dfs, err := strconv.Atoi(os.Getenv("HYDRAIDE_DEFAULT_FILE_SIZE"))
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"error": err,
			}).Panic("HYDRAIDE_DEFAULT_FILE_SIZE must be a number without any string characters")
		}
		defaultFileSize = int64(dfs)
	}

}

func main() {

	defer panicHandler()

	// Start graylog only when the GRAYLOG_ENABLED environment variable is set to true
	if graylogServer != "" {
		// init Graylog connection with our Graylog server
		g := graylog.New(graylogServer, graylogServiceName)
		g.SetLogLevel(logLevel)
	} else {
		logrus.SetFormatter(&logrus.TextFormatter{
			DisableColors:   true,
			FullTimestamp:   true,
			TimestampFormat: logTimeFormat,
		})
		logrus.SetOutput(os.Stdout)
		setLogrusLogLevel()
	}

	// start the new Hydra server
	serverInterface = server.New(&server.Configuration{
		CertificateCrtFile:    serverCrtPath,
		CertificateKeyFile:    serverKeyPath,
		HydraServerPort:       hydraServerPort,
		HydraMaxMessageSize:   hydraMaxMessageSize,
		DefaultCloseAfterIdle: defaultCloseAfterIdle,
		DefaultWriteInterval:  defaultWriteInterval,
		DefaultFileSize:       defaultFileSize,
		SystemResourceLogging: systemResourceLogging,
	})

	if err := serverInterface.Start(); err != nil {
		log.Fatal(err)
	}

	go func() {
		http.HandleFunc("/health", healthCheckHandler)
		port := fmt.Sprintf(":%d", healthCheckPort)
		if err := http.ListenAndServe(port, nil); err != nil {
			logrus.WithFields(logrus.Fields{
				"error": err,
			}).Error("http server error - health check check server is not running")
		}
	}()

	// blocker for the main goroutine and waiting for kill signal
	waitingForKillSignal()

}

// setLogrusLogLevel beállítjuk a log szintet a logLevel változó alapján, ha csak logrust használunk
func setLogrusLogLevel() {
	switch logLevel {
	case "trace":
		logrus.SetLevel(logrus.TraceLevel)
	case "debug":
		logrus.SetLevel(logrus.DebugLevel)
	case "info":
		logrus.SetLevel(logrus.InfoLevel)
	case "warn":
		logrus.SetLevel(logrus.WarnLevel)
	case "error":
		logrus.SetLevel(logrus.ErrorLevel)
	case "fatal":
		logrus.SetLevel(logrus.FatalLevel)
	case "panic":
		logrus.SetLevel(logrus.PanicLevel)
	default:
		logrus.SetLevel(logrus.InfoLevel)
	}
}

func panicHandler() {
	if r := recover(); r != nil {
		// Lekérjük a stack trace-t
		stackTrace := debug.Stack()
		// Logoljuk a pánikot és a stack trace-t
		logrus.WithFields(logrus.Fields{
			"error": r,
			"stack": string(stackTrace),
		}).Error("caught panic")
		// Hívd meg a gracefulStop függvényt
		gracefulStop()
	}
}

func gracefulStop() {
	// stop the microservice and exit the program
	serverInterface.Stop()
	logrus.Info("hydra server stopped gracefully. Program is exiting...")
	// waiting for logs to be written to the file
	time.Sleep(1 * time.Second)
	// exit the program if the microservice is stopped gracefully
	os.Exit(0)
}

func waitingForKillSignal() {
	logrus.Info("hydra server waiting for kill signal")
	gracefulStopSignal := make(chan os.Signal, 1)
	signal.Notify(gracefulStopSignal, syscall.SIGKILL, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	// waiting for graceful stop signal
	<-gracefulStopSignal
	logrus.Info("kill signal received")
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
