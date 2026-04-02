package models

import (
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// BasicsAreKeysExist demonstrates how to check if multiple keys (Treasures)
// exist in a given Swamp using a single batch request.
//
// 🧠 Typical use case:
// Use this when you need to verify the existence of many keys at once without
// reading their full content. This is significantly more efficient than calling
// IsKeyExists in a loop, as it makes a single gRPC round-trip.
// For example:
// - Bulk pre-validation before batch inserts (skip already-existing keys).
// - Deduplication checks across a list of candidate keys.
// - Filtering a list of IDs to determine which ones are already indexed.
type BasicsAreKeysExist struct {
	MyModelKey   string `hydraide:"key"`   // One of the keys we want to check
	MyModelValue string `hydraide:"value"` // Not used in this check, but part of the struct model
}

// AreKeysExist returns a map indicating which of the specified keys exist in the given Swamp.
//
// ⚙️ Behavior:
// - If the Swamp does not exist on disk, all keys are returned as false (no error).
// - If the Swamp exists, it is hydrated into memory and each key is checked via a
//   single read-lock on the in-memory index — very fast for large batches.
// - All requested keys appear in the result map, including missing ones (mapped to false).
// - Empty key list returns an empty map immediately without a network call.
//
// ✅ Use this when:
// - You need to check existence of many keys efficiently in one call
// - You want to avoid the overhead of reading full Treasure data (unlike CatalogReadBatch)
// - You implement batch-insert logic that should skip duplicates
//
// ⚠️ Notes:
// - This is the batch version of IsKeyExists — prefer this for 2+ keys
// - Wildcards are not allowed — you must provide a fully qualified Swamp name
// - Duplicate keys in the input are handled gracefully (map will contain one entry)
//
// 🔁 Return values:
// - (map[string]bool, nil) → map from each key to true (exists) or false (not found)
// - (nil, error) → Transport or server error occurred
func (m *BasicsAreKeysExist) AreKeysExist(repo repo.Repo, keys []string) (results map[string]bool, err error) {

	// Create a bounded context to ensure graceful timeout behavior
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the typed HydrAIDE SDK instance
	h := repo.GetHydraidego()

	// Check for the existence of multiple keys in the specified Swamp.
	return h.AreKeysExist(ctx,
		name.New().Sanctuary("MySanctuary").Realm("MyRealm").Swamp("BasicsAreKeysExist"),
		keys)
}
