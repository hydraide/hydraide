package models

import (
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// CatalogModelBasicsDeregisterSwamp demonstrates how to remove a Swamp pattern
// from the HydrAIDE internal registry using the SDK.
//
// 🧠 Reminder:
// This does *not* delete Swamps or Treasures — it only removes the associated
// pattern from the system, so that future pattern-based operations no longer match it.
type CatalogModelBasicsDeregisterSwamp struct {
	MyModelKey   string `hydraide:"key"`   // Used as the Treasure key, not relevant to deregistration
	MyModelValue string `hydraide:"value"` // Optional field, not used in this example
}

// DeregisterSwamp calls DeRegisterSwamp() via the SDK to remove a specific Swamp pattern.
//
// ✅ When to use:
// - You're permanently deprecating a logic pattern
// - You've migrated data to a new Swamp and want to cleanly unregister the old one
//
// ⚠️ When NOT to use:
// - If the Swamp is only temporarily empty or idle — HydrAIDE unloads it automatically
//
// 🔁 Flow recommendation:
// 1. Migrate or archive data
// 2. Delete Treasures with Delete() or DeleteAll()
// 3. Call DeRegisterSwamp() to remove the pattern
//
// Returns:
// - Nil if deregistration succeeds
// - A list of errors if any target server failed
func (m *CatalogModelBasicsDeregisterSwamp) DeregisterSwamp(repo repo.Repo) (errors []error) {

	// Set up a timeout-bound context to avoid hanging calls
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the typed HydrAIDE SDK instance
	h := repo.GetHydraidego()

	// DeRegister the Swamp pattern from the registry.
	// This will affect how future pattern-based operations resolve this Swamp.
	return h.DeRegisterSwamp(ctx, name.New().Sanctuary("MySanctuary").Realm("MyRealm").Swamp("CatalogModelBasicsDeregisterSwamp"))
}
