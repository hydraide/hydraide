package models

import (
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// BasicsIsSwampExist demonstrates how to perform a lightweight,
// non-intrusive existence check for a specific Swamp in the HydrAIDE system.
//
// Unlike IsKeyExists(), this check does NOT load or hydrate the Swamp.
// It's ideal for presence detection, UI indicators, or flow gating logic.
//
// 🔍 Example use cases:
// - Show ✅/❌ in UI based on whether a Swamp exists for a domain
// - Avoid unnecessary hydration for Swamps that were already deleted
// - Conditionally run logic (e.g. load details, start pipeline) if Swamp is present
type BasicsIsSwampExist struct {
	MyModelKey   string `hydraide:"key"`   // This field will be used as the Treasure key
	MyModelValue string `hydraide:"value"` // This field can hold any value, not used in counting
}

// IsSwampExist checks whether a specific Swamp currently exists in the HydrAIDE system.
//
// ⚠️ This is a **direct existence check** – it does NOT accept wildcards or patterns.
// You must provide a fully resolved Swamp name (Sanctuary + Realm + Swamp).
//
// ✅ When to use this:
// - When you want to check if a Swamp was previously created by another process
// - When a Swamp may have been deleted automatically (e.g., became empty)
// - When you want to determine Swamp presence **without hydrating or loading data**
// - As part of fast lookups, hydration conditionals, or visibility toggles
//
// 🔍 **Real-world example**:
// Suppose you're generating AI analysis per domain and storing them in separate Swamps:
//
//	Sanctuary("domains").Realm("ai").Swamp("trendizz.com")
//	Sanctuary("domains").Realm("ai").Swamp("hydraide.io")
//
// When rendering a UI list of domains, you don’t want to load full AI data.
// Instead, use `IsSwampExist()` to check if an AI analysis exists for each domain,
// and show a ✅ or ❌ icon accordingly — without incurring I/O or memory cost.
//
// ⚙️ Behavior:
// - If the Swamp exists → returns (true, nil)
// - If it never existed or was auto-deleted → returns (false, nil)
// - If a server error occurs → returns (false, error)
//
// 🚀 This check is extremely fast: O(1) routing + metadata lookup.
// ➕ It does **not hydrate or load** the Swamp into memory — it only checks for existence on disk.
//
//	If the Swamp is already open, it stays open. If not, it stays closed.
//	This allows for high-frequency checks without affecting memory or system state.
//
// ⚠️ Requires that the Swamp pattern for the given name was previously registered.
func (m *BasicsIsSwampExist) IsSwampExist(repo repo.Repo) (isExist bool, err error) {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := repo.GetHydraidego()

	return h.IsSwampExist(ctx, name.New().Sanctuary("MySanctuary").Realm("MyRealm").Swamp("BasicsIsSwampExist"))

}
