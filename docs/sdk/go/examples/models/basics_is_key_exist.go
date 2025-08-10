package models

import (
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// BasicsIsKeyExist demonstrates how to check if a specific key (Treasure)
// already exists in a given Swamp using the HydrAIDE SDK.
//
// 🧠 Typical use case:
// Use this in conditional logic where system behavior depends on whether a key exists.
// For example:
// - Send a welcome email only if a user ID hasn't been seen before.
// - Skip a process if the entity already exists.
// - Run onboarding logic only once.
type BasicsIsKeyExist struct {
	MyModelKey   string `hydraide:"key"`   // The key we want to check for existence in the Swamp
	MyModelValue string `hydraide:"value"` // Not used in this check, but part of the struct model
}

// IsKeyExist returns true if the specified key exists in the given Swamp.
//
// ⚙️ Behavior:
// - Loads (hydrates) the Swamp into memory if it isn't already
// - Searches for the presence of the given key (`m.MyModelKey`)
//
// ✅ Use this when:
// - You want to trigger a logic branch conditionally, based on prior presence
// - You implement first-time-only logic (e.g., onboarding)
// - You want to check history/state without scanning or reading the full data
//
// ⚠️ Notes:
// - This is different from `IsSwampExist()`, which only checks for Swamp presence on disk
// - Wildcards are not allowed — you must provide a fully qualified Swamp name
// - Swamp hydration cost should be considered if you're calling this at scale
//
// 🔁 Return values:
// - (true, nil)  → Swamp exists and key exists
// - (false, nil) → Swamp exists but key does not
// - (false, ErrCodeSwampNotFound) → Swamp does not exist
// - (false, other error) → Transport or server error occurred
func (m *BasicsIsKeyExist) IsKeyExist(repo repo.Repo) (isExist bool, err error) {

	// Create a bounded context to ensure graceful timeout behavior
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the typed HydrAIDE SDK instance
	h := repo.GetHydraidego()

	// Check for the existence of the key (m.MyModelKey) in the specified Swamp.
	return h.IsKeyExists(ctx,
		name.New().Sanctuary("MySanctuary").Realm("MyRealm").Swamp("BasicsIsKeyExist"),
		m.MyModelKey)
}
