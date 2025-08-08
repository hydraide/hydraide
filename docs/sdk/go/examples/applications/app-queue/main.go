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
	appServer appserver.AppServer
)

func main() {

	// Start the HydrAIDE environment with two distributed servers.
	// This setup demonstrates a dual-server architecture with deterministic island partitioning.
	//
	// 🧠 Island ranges:
	// - Server 1 handles islands 1–100
	// - Server 2 handles islands 101–200
	// - Total islands: 200
	//
	// Each server must define:
	// - Host address where the HydrAIDE instance is reachable (e.g. "localhost:5444")
	// - Valid TLS certificate path (full path to `ca.crt` or equivalent)
	// - Assigned island range using `FromIsland` and `ToIsland`
	//
	// 🔍 What is an Island?
	// In HydrAIDE, an "island" is a deterministic numeric bucket (typically 1–N),
	// each corresponding directly to a folder on disk that holds Swamps.
	// The system uses a stable, hash-based algorithm to distribute Swamps evenly across all islands,
	// ensuring balanced storage and consistent routing without needing a central coordinator.
	//
	// ⚖️ Why deterministic islands?
	// The main goal is effortless horizontal scaling.
	// Because Swamps are routed to islands based on a fixed hash function over their full names,
	// their island assignment is **immutable** unless the total number of islands (`AllIslands`) changes.
	//
	// ❗ Important: You cannot change the total number of islands later without breaking hash distribution.
	// This means:
	//    → Once you choose a value for AllIslands (e.g. 200), all Swamp hashes are bound to that scale.
	//    → If you later reduce it (e.g. from 200 to 100), existing Swamps will no longer map to valid folders.
	//    → Therefore, always choose a **larger AllIsland count from the beginning** to allow room for future growth.
	//
	// ✅ Recommended pattern:
	// - Start with a large value, e.g. AllIslands = 1000
	// - In single-server mode: FromIsland=1, ToIsland=1000 (server handles all folders)
	// - Later, split load as needed:
	//     • Server 1: islands 1–500
	//     • Server 2: islands 501–1000
	// - Or more granular:
	//     • Server 1: islands 1–333
	//     • Server 2: islands 334–666
	//     • Server 3: islands 667–1000
	//
	// 🧠 You can also assign uneven ranges based on server capacity:
	// - Server 1 (slow disk):    islands 1–200
	// - Server 2 (SSD):          islands 201–800
	// - Server 3 (high-memory):  islands 801–1000
	//
	// 💡 Swamp distribution is stable and even:
	// Swamps are deterministically mapped to islands using their full string name (e.g. "user/profiles/alice").
	// This ensures even folder spread and minimal skew, even across thousands of Swamps.
	// Although some Swamps may be accessed more frequently (hot keys),
	// HydrAIDE's design keeps them isolated and memory-safe, avoiding systemic bottlenecks.
	//repoInterface := repo.New([]*client.Server{
	//	{
	//		// Server 1 – handles islands 1–100
	//		// Use "localhost:5444" if running in Docker with port mapped from 4444
	//		Host:       os.Getenv("HYDRA_HOST_1"),
	//		FromIsland: 1,
	//		ToIsland:   100,
	//		// Client certificate for connecting to HydrAIDE securely (full file path + extension)
	//		// Example: "/etc/hydraide/certs/ca.crt"
	//		CertFilePath: os.Getenv("HYDRA_CERT_1"),
	//	},
	//	{
	//		// Server 2 – handles islands 101–200
	//		// Use "localhost:5445" or "remote-ip:5445" for the second HydrAIDE instance
	//		Host:         os.Getenv("HYDRA_HOST_2"),
	//		FromIsland:   101,
	//		ToIsland:     200,
	//		CertFilePath: os.Getenv("HYDRA_CERT_2"),
	//	},
	//},
	//	200,      // Total number of islands in the system
	//	10485760, // Max gRPC message size (10MB)
	//	true,     // Enable connection analysis on startup (useful during integration tests)
	//)

	repoInterface := repo.New([]*client.Server{
		{
			// Server 1 – handles islands 1–100
			// Use "localhost:5444" if running in Docker with port mapped from 4444
			Host:       os.Getenv("HYDRA_HOST"),
			FromIsland: 1,
			ToIsland:   1000,
			// Client certificate for connecting to HydrAIDE securely (full file path + extension)
			// Example: "/etc/hydraide/certs/ca.crt"
			CertFilePath: os.Getenv("HYDRA_CERT"),
		},
	},
		1000,     // Total number of islands in the system
		10485760, // Max gRPC message size (10MB)
		false,    // Enable connection analysis on startup (useful during integration tests)
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
