package models

import (
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// CatalogModelBasicsCount is a minimal example model used for demo purposes.
// It shows how to interact with the HydrAIDE engine to count the number of Treasures
// inside a specific Swamp.
type CatalogModelBasicsCount struct {
	MyModelKey   string `hydraide:"key"`   // This field will be used as the Treasure key
	MyModelValue string `hydraide:"value"` // This field can hold any value, not used in counting
}

// Count uses the HydrAIDE SDK to return the number of Treasures stored in a named Swamp.
// It demonstrates how to:
// - set up a context with timeout
// - connect to the HydrAIDE client
// - call the Count() method with a specific Swamp name
func (m *CatalogModelBasicsCount) Count(repo repo.Repo) (allTreasures int32, err error) {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := repo.GetHydraidego()

	// Execute the Count operation on a specific Swamp.
	// The Swamp name follows the HydrAIDE naming pattern:
	// Sanctuary → Realm → Swamp
	//
	// In this case:
	// - Sanctuary: "MySanctuary"
	// - Realm:     "MyRealm"
	// - Swamp:     "CatalogModelBasicCount"
	//
	// The Count method returns:
	// - number of Treasures (records) in the Swamp
	// - or an error if the Swamp doesn't exist or the call fails
	//
	// Notes:
	// - If the Swamp does not exist: ErrCodeSwampNotFound is returned
	// - If the Swamp is empty: returns 0
	// - If the Swamp is loaded: returns actual count (at least 1 if it was ever written to)
	//
	// ✅ Best use cases:
	// - Admin dashboards
	// - Cleanup scripts
	// - Pagination setup (e.g., "show total pages")
	return h.Count(ctx, name.New().Sanctuary("MySanctuary").Realm("MyRealm").Swamp("CatalogModelBasicCount"))

}
