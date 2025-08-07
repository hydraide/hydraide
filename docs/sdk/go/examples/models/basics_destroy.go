package models

import (
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// BasicsDestroy demonstrates how to permanently erase all Treasures
// in a specific Swamp using the HydrAIDE SDK.
//
// 🧨 This method deletes all data from a Swamp AND triggers its removal from disk.
// However, the Swamp pattern remains registered — meaning new data can still be written
// to the same name in the future.
type BasicsDestroy struct {
	MyModelKey   string `hydraide:"key"`   // Used as the Treasure key, not relevant for Destroy
	MyModelValue string `hydraide:"value"` // Optional field, unused in this operation
}

// Destroy permanently removes the contents of a Swamp.
// If the Swamp becomes empty, it is automatically removed from disk and memory.
//
// ✅ Best use cases:
// - Cleaning up a full Swamp without deleting keys one-by-one
// - Resetting a test environment or sandbox between test runs
// - Fully deleting a profile-type Swamp (e.g. users/profiles/petergebri)
//
// ⚙️ Behavior:
// - Deletes all Treasures (key-value pairs) inside the Swamp, but not one by one
// - The entire Swamp folder is removed from disk (zero footprint)
// - The pattern registration (Swamp name) remains active and reusable
//
// ⚠️ Important:
// - This does not deregister the pattern (use DeRegisterSwamp() if needed)
// - Once the Swamp is destroyed, any existing data is permanently gone
func (m *BasicsDestroy) Destroy(repo repo.Repo) (err error) {

	// Create a cancellation-aware context with timeout
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Get the HydrAIDE SDK instance from the repository
	h := repo.GetHydraidego()

	// Perform the Swamp-wide destruction
	return h.Destroy(ctx, name.New().Sanctuary("MySanctuary").Realm("MyRealm").Swamp("BasicsDestroy"))
}
