// Package name provides a deterministic naming convention for HydrAIDE,
// helping developers structure and resolve data into a three-level hierarchy:
// Sanctuary → Realm → Swamp.
//
// This structure enables:
// - O(1) access to data based on name
// - Stateless client-side routing in multi-server setups
// - Predictable folder and server mapping without orchestrators
//
// Each Name is constructed step-by-step (Sanctuary → Realm → Swamp),
// and the full path can be retrieved using Get(). Additionally,
// GetIslandID(allFolders) maps the current name to a consistent
// server index (1-based), using a fast and collision-resistant hash.
//
// 🛑 Constraints:
//
//   - All three components — Sanctuary, Realm, and Swamp — are **required**.
//   - Each part must be at least **1 character long**.
//   - The `/` character is **not allowed** in any part of the name.
//     It is reserved as a structural delimiter and must not be used within values.
//   - The use of **alphanumeric characters** (a–z, A–Z, 0–9) is strongly recommended
//     to ensure compatibility and predictable routing across all environments.
//
// ⚠️ Note: This package does **not** perform runtime validation of these constraints.
// To keep the `name` package as fast as possible (ns-level execution),
// no checks or sanitization are performed internally.
// It is the responsibility of the caller to ensure valid input.
// Adding validation would significantly degrade performance (10–20x).
//
// Example usage:
//
//	name := New().Sanctuary("users").Realm("profiles").Swamp("alice123")
//	fmt.Println(name.Get()) // "users/profiles/alice123"
//	fmt.Println(name.GetIslandID(1000)) // e.g. 774
//
// Use Load(path) to reconstruct a Name from an existing path string.
//
// This package is used across HydrAIDE SDKs to:
// - Determine data placement
// - Support distributed architectures
// - Enforce clean, intention-driven naming
//
// ----------------------------------------
// 📘 HydrAIDE Go SDK
// Full SDK documentation:
// https://github.com/hydraide/hydraide/blob/main/docs/sdk/go/go-sdk.md
// ----------------------------------------
package name

import (
	"github.com/cespare/xxhash/v2"
	"strings"
	"sync"
)

// Name defines a structured identifier used in HydrAIDE to deterministically
// map data into a distributed, folder-based architecture.
//
// Each Name represents a three-level hierarchy:
//
//	Sanctuary → Realm → Swamp
//
// This structure is essential for:
// - Organizing Swamps into logical domains
// - Generating predictable folder paths
// - Assigning each Swamp to a specific folder without coordination
//
// The interface supports fluent chaining:
//
//	name := New().
//	    Sanctuary("users").
//	    Realm("profiles").
//	    Swamp("alice123")
//
//	name.Get()                 // "users/profiles/alice123"
//	name.GetIslandID(100) // e.g. 42
//
// Usage of GetFolderNumber ensures even distribution of data across N folders,
// enabling stateless multi-node architectures without external orchestrators.
//
// See also: Load(path string) to reconstruct a Name from a path.
type Name interface {
	Sanctuary(sanctuaryID string) Name
	Realm(realmName string) Name
	Swamp(swampName string) Name
	Get() string
	GetIslandID(allIslands uint64) uint64
	IsWildcardPattern() bool
}

type name struct {
	Path           string
	SanctuaryID    string
	RealmName      string
	SwampName      string
	IslandNumber   uint64
	hashPathMu     sync.Mutex
	folderNumberMu sync.Mutex
}

// New creates a new empty Name instance.
// Use this as the starting point for building hierarchical names
// by chaining Sanctuary(), Realm(), and Swamp().
func New() Name {
	return &name{}
}

// Sanctuary sets the top-level domain of the Name.
// Typically used to group major logical areas (e.g. "users", "products").
func (n *name) Sanctuary(sanctuaryID string) Name {
	return &name{
		SanctuaryID: sanctuaryID,
		Path:        sanctuaryID,
	}
}

// Realm sets the second-level scope under the Sanctuary.
// Often used to further categorize Swamps (e.g. "profiles", "settings").
func (n *name) Realm(realmName string) Name {
	return &name{
		SanctuaryID: n.SanctuaryID,
		RealmName:   realmName,
		Path:        n.Path + "/" + realmName,
	}
}

// Swamp sets the final segment of the Name — the Swamp itself.
// This represents the concrete storage unit where Treasures are kept.
// The full path becomes: sanctuary/realm/swamp.
func (n *name) Swamp(swampName string) Name {
	return &name{
		SanctuaryID: n.SanctuaryID,
		RealmName:   n.RealmName,
		SwampName:   swampName,
		Path:        n.Path + "/" + swampName,
	}
}

// Get returns the full hierarchical path of the Name in the format:
//
//	"sanctuary/realm/swamp"
//
// 🔒 Internal use only: This method is intended for SDK-level logic,
// such as logging, folder path generation, or internal diagnostics.
// SDK users should never need to call this directly.
func (n *name) Get() string {
	return n.Path
}

// GetIslandID returns the deterministic, 1-based ID of the Island where this Name physically resides.
//
// An Island is HydrAIDE’s smallest migratable physical unit — a deterministic storage zone that groups
// one or more Swamps under the same hash bucket. The result of this function is used to determine
// which HydrAIDE server should store the Swamp represented by this Name.
//
// The IslandID is calculated using a fast, consistent xxhash over the combined
// SanctuaryID, RealmName, and SwampName. The hash value is mapped into the provided `allIslands`
// range, which must be consistent across all clients and routers to ensure predictable behavior.
//
// 📦 What is an Island?
// - A logical+physical storage unit that lives as a top-level folder (e.g. /data/234/)
// - The place where a Swamp is anchored
// - A fixed destination for a given SwampName, regardless of infrastructure changes
//
// 🌐 Why does this matter?
// - Enables decentralized routing without coordination
// - Makes server assignments stateless and predictable
// - Supports seamless migration (moving Islands ≠ renaming Swamps)
//
// 🚫 This function should not be used directly by application code.
// It is intended for SDK-internal routing logic.
//
// Example:
//
//	islandID := name.GetIslandID(1000)
//	client := router.Route(islandID)
//
// 💡 If you update the hash space (allIslands), all previous IslandID mappings change.
// Keep `allIslands` fixed across your system lifetime for stable routing.
func (n *name) GetIslandID(allIslands uint64) uint64 {

	n.folderNumberMu.Lock()
	defer n.folderNumberMu.Unlock()

	if n.IslandNumber != 0 {
		return n.IslandNumber
	}

	hash := xxhash.Sum64([]byte(n.SanctuaryID + n.RealmName + n.SwampName))

	n.IslandNumber = hash%allIslands + 1

	return n.IslandNumber

}

// IsWildcardPattern returns true if any part of the Name is set to "*".
func (n *name) IsWildcardPattern() bool {
	return n.SanctuaryID == "*" || n.RealmName == "*" || n.SwampName == "*"
}

// Load reconstructs a Name from a given path string in the format:
//
//	"sanctuary/realm/swamp"
//
// It parses the path segments and returns a Name instance with all fields set.
//
// 🔒 Internal use only: This function is intended for SDK-level logic,
// such as reconstructing a Name from persisted references, file paths, or routing metadata.
// It should not be called by application developers directly.
func Load(path string) Name {
	splitPath := strings.Split(path, "/")
	sanctuaryID := splitPath[0]
	realmName := splitPath[1]
	swampName := splitPath[2]

	return &name{
		Path:        sanctuaryID + "/" + realmName + "/" + swampName,
		SanctuaryID: sanctuaryID,
		RealmName:   realmName,
		SwampName:   swampName,
	}
}
