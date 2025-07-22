// Package repo provides a centralized and injectable abstraction for managing HydrAIDE SDK connections.
//
// This layer wraps the raw `hydraidego.Hydraidego` client and exposes it via a lightweight interface (Repo),
// enabling clean separation of infrastructure concerns from application logic.
//
// Features:
// - Encapsulates HydrAIDE client initialization and connection handling.
// - Allows mocking of HydrAIDE access during tests (e.g., with a fake Repo).
// - Simplifies dependency injection for services or workers using queue, store, or catalog logic.
//
// Usage:
//
//	import "yourapp/repo"
//
//	repo := repo.New(servers, allIslands, maxMessageSize, enableDiagnostics)
//	db := repo.GetHydraidego()
//	db.Save(...), db.Read(...), etc.
//
// This abstraction is typically passed to all internal modules (e.g., queueService, catalogService),
// which then operate purely against the Repo interface without managing any low-level SDK logic.
//
// Notes:
// - The underlying SDK is `hydraidego`, the official Go client for HydrAIDE.
// - Connection errors during startup will panic — ensure server availability before invoking New().
//
// This package is part of the example-applications infrastructure layer.
package repo

import (
	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/client"
)

// Repo is an interface that provides access to the HydrAIDE Go SDK (hydraidego).
// It is designed to be injected into services, allowing for mocking in tests
// and clean separation between infrastructure and logic layers.
type Repo interface {
	GetHydraidego() hydraidego.Hydraidego
}

type repo struct {
	hydraidegoInterface hydraidego.Hydraidego
}

// New creates and initializes a HydrAIDE client wrapper (Repo interface).
//
// This function performs the following steps:
//  1. Creates a new HydrAIDE gRPC client with the given server list.
//  2. Establishes the connection to HydrAIDE servers.
//  3. Wraps the client into a high-level SDK (`hydraidego`).
//  4. Returns a Repo instance that exposes the SDK.
//
// Parameters:
// - servers: list of HydrAIDE gRPC endpoints (can be multiple nodes).
// - allIslands: number of total folder-islands in the HydrAIDE cluster (for routing).
// - maxMessageSize: max message size in bytes allowed by gRPC (e.g. 5GB for bulk).
// - connectionAnalysis: if true, enables connection diagnostics and timing logs.
//
// Returns:
// - Repo: a connected and ready-to-use HydrAIDE access interface.
//
// Panics:
//   - If no HydrAIDE servers can be reached during Connect(), the function panics.
//     This is intentional, as the app cannot proceed without a connected data engine.
func New(servers []*client.Server, allIslands uint64, maxMessageSize int, connectionAnalysis bool) Repo {
	// Initialize the HydrAIDE gRPC client.
	clientInterface := client.New(servers, allIslands, maxMessageSize)

	// Attempt to connect to all provided servers.
	if err := clientInterface.Connect(connectionAnalysis); err != nil {
		panic(err) // No fallback — app cannot proceed without connection
	}

	// Wrap the client into the Go SDK abstraction.
	hydraideInterface := hydraidego.New(clientInterface)

	// Return a fully prepared Repo instance.
	return &repo{
		hydraidegoInterface: hydraideInterface,
	}
}

// GetHydraidego returns the HydrAIDE Go SDK interface.
//
// This allows services and handlers to perform all Swamp, Treasure,
// and logic operations supported by hydraidego.
//
// Example:
//
//	db := repo.GetHydraidego()
//	db.Save(...), db.Read(...), etc.
func (r *repo) GetHydraidego() hydraidego.Hydraidego {
	return r.hydraidegoInterface
}
