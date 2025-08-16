// Package main demonstrates how to run a simple queue example on top of a HydrAIDE instance.
//
// Prerequisites
// -------------
// 1. Install and start a HydrAIDE instance following the official installation manual.
// 2. Ensure you have a valid certificate set (generated during instance initialization).
//
// Before running, configure the following environment variables:
//
//	HYDRAIDE_HOST        → Address of the HydrAIDE server (e.g. "localhost:4200").
//	HYDRAIDE_CA_CRT      → Full filesystem path to `ca.crt` from the instance’s certificate folder.
//	                       If the client runs on another machine, copy `ca.crt` locally while keeping
//	                       the original on the server, and set the local absolute path.
//	HYDRAIDE_CLIENT_CRT  → Full path to `client.crt` (client certificate generated at instance init).
//	HYDRAIDE_CLIENT_KEY  → Full path to `client.key` (client private key generated at instance init).
//
// Optional:
//
//	CONNECTION_ANALYSIS  → If set to "true", the SDK logs detailed TLS connection
//	                       and authentication results. Useful for debugging/integration
//	                       but not recommended in production.
//
// What this example does
// ----------------------
// - Connects to a HydrAIDE instance with mutual TLS authentication.
// - Inserts 10 scheduled tasks into a queue (1-second expiration each).
// - Logs every task as it is enqueued and again when it expires and is received back.
//
// Example log output when tasks are added:
//
//	2025/08/16 12:19:28 INFO task added to queue successfully queueName=myTestQueue taskID=f0ed27e7-... taskMessage=message-0
//	2025/08/16 12:19:28 INFO task added to queue successfully queueName=myTestQueue taskID=5d5217de-... taskMessage=message-1
//	...
//
// Example log output when tasks expire:
//
//	2025/08/16 12:19:32 INFO expired task received queueName=myTestQueue taskID=task-1 taskMessage=message-1
//	2025/08/16 12:19:33 INFO expired task received queueName=myTestQueue taskID=task-2 taskMessage=message-2
//	...
//
// The program runs for roughly 11 seconds, performing actual queue operations
// and logging both enqueue and dequeue phases.
package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/applications/app-queue/appserver"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/client"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

var (
	appServer           appserver.AppServer
	connAnalysisEnabled bool
)

func init() {
	if os.Getenv("CONNECTION_ANALYSIS") == "true" {
		connAnalysisEnabled = true
	}
}

func main() {

	// Start the HydrAIDE environment with one or more distributed servers.
	// This example demonstrates a single-server setup with deterministic island partitioning.
	//
	// 🧠 Island ranges:
	// - Server 1 handles islands 1–1000
	// - Total islands: 1000
	//
	// Each server definition includes:
	// - Host: the gRPC endpoint where the HydrAIDE instance is reachable (e.g. "localhost:5444")
	// - FromIsland / ToIsland: the inclusive numeric range of islands assigned to this server
	// - CACrtPath: path to the server’s CA certificate (`ca.crt`), used to validate the server cert
	// - ClientCrtPath: path to the client certificate (`client.crt`) issued by this server during init
	// - ClientKeyPath: path to the private key (`client.key`) for the client certificate
	//
	// 🔍 What is an Island?
	// An island is a deterministic numeric bucket (1..N) directly mapped to a filesystem folder.
	// Swamps are hashed into these islands, making routing stable and predictable.
	// Once the `allIslands` value is set (e.g. 1000), it must remain fixed to avoid hash breakage.
	//
	// ❗ Important:
	// - Each HydrAIDE instance generates its own `client.crt` / `client.key` at init time.
	// - These client certs are **server-specific**. You cannot reuse one server’s client cert to connect to another.
	// - If you run multiple servers, you must configure separate CA + client certs per server.
	//
	// ✅ Recommended pattern:
	// - Start with a large `allIslands` (e.g. 1000)
	// - In single-server mode: FromIsland=1, ToIsland=1000
	// - Later split ranges as you scale horizontally:
	//     • Server 1: islands 1–500 with its own cert set
	//     • Server 2: islands 501–1000 with its own cert set
	//
	// 💡 This design guarantees:
	// - Mutual TLS authentication (server verifies client, client verifies server)
	// - Deterministic Swamp→Island→Server routing
	// - Easy migration: islands can be reassigned to new servers without rehashing Swamps
	//
	repoInterface := repo.New([]*client.Server{
		{
			// Server 1 – handles islands 1–1000
			// Use "localhost:5444" if running in Docker with port mapped from 4444
			Host:          os.Getenv("HYDRAIDE_HOST"),
			FromIsland:    1,
			ToIsland:      1000,
			CACrtPath:     os.Getenv("HYDRAIDE_CA_CRT"),
			ClientCrtPath: os.Getenv("HYDRAIDE_CLIENT_CRT"),
			ClientKeyPath: os.Getenv("HYDRAIDE_CLIENT_KEY"),
		},
	},
		1000,                // Total number of islands in the system
		10485760,            // Max gRPC message size (10MB)
		connAnalysisEnabled, // Enable connection analysis on startup (set true for debugging/integration tests)
	)

	// Start the AppServer, which handles the web application layer and business logic.
	appServer = appserver.New(repoInterface)
	appServer.Start()

	// Prevent the program from exiting immediately.
	// This keeps the server running until an OS-level stop signal is received (e.g. SIGINT or SIGTERM).
	waitingForKillSignal()

}

// gracefulStop cleanly shuts down the application server and terminates the program.
// This function is typically triggered by an OS-level stop signal (e.g. SIGINT, SIGTERM).
func gracefulStop() {
	// Stop the application server (closes listeners, releases resources, etc.)
	appServer.Stop()
	slog.Info("application stopped gracefully")

	// Exit the process with status code 0 (success)
	os.Exit(0)
}

// waitingForKillSignal blocks the main thread and waits for a termination signal (SIGINT, SIGTERM, etc.).
// When such a signal is received, it initiates a graceful shutdown of the application.
func waitingForKillSignal() {
	slog.Info("waiting for graceful stop signal...")

	// Create a buffered channel to listen for OS termination signals
	gracefulStopSignal := make(chan os.Signal, 1)

	// Register interest in specific system signals that should trigger shutdown
	signal.Notify(gracefulStopSignal, syscall.SIGKILL, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Block execution until a signal is received
	<-gracefulStopSignal
	slog.Info("received graceful stop signal, stopping application...")

	// Perform graceful shutdown
	gracefulStop()
}
