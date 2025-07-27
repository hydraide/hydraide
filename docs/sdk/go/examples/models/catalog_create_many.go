package models

import (
	"github.com/hydraide/hydraide/docs/sdk/go/examples/models/utils/hydraidehelper"
	"github.com/hydraide/hydraide/docs/sdk/go/examples/models/utils/repo"
	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"log/slog"
)

// CatalogModelUserCreateManyExample demonstrates how to insert multiple user Emails into
// a HydrAIDE Catalog using CatalogCreateMany — ensuring that no existing entries are overwritten.
//
// 🧠 Use case:
// This model is ideal for bulk user import scenarios, such as uploading users from a CSV file
// or an external API, where each user is uniquely identified by their email address.
//
// ⚠️ Behavior:
// - Only users who do not already exist will be inserted
// - Existing records are skipped without overwrite
// - The iterator receives per-record success or failure feedback
type CatalogModelUserCreateManyExample struct {
	UserEmailAddress string `hydraide:"key"` // Unique identifier for the user – used as the Treasure key
}

// CreateMany demonstrates how to batch-insert users into the HydrAIDE catalog
// using CatalogCreateMany. This method simulates importing a predefined list
// of user email addresses.
//
// ✅ Use Case:
// Perfect for bulk user registration, CSV imports, or mass ID ingestion where
// overwrite is not allowed and each key must be inserted only if it doesn’t exist.
//
// 🚀 Performance Advantage:
// CatalogCreateMany sends **all entries in a single gRPC call**, reducing the
// overhead of multiple roundtrips. This dramatically improves throughput and
// efficiency compared to calling CatalogCreate() or Save() in a loop.
//
// 🪄 Iterator Support:
// An inline iterator function is used to track which records were inserted
// and which were skipped (e.g., already existed).
func (c *CatalogModelUserCreateManyExample) CreateMany(r repo.Repo) {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := r.GetHydraidego()

	// 🧪 Example dataset: emails to import (e.g. from a CSV or external API)
	emails := []string{
		"alice@example.com",
		"bob@example.com",
		"carol@example.com",
		"dave@example.com",
	}

	var models []any
	// HydrAIDE requires each model to be a pointer to a struct tagged with `hydraide:"key"`.
	// This ensures that each entry is recognized as a valid Treasure during insertion.
	for _, email := range emails {
		model := &CatalogModelUserCreateManyExample{UserEmailAddress: email}
		models = append(models, model)
	}

	// 🔁 Use CatalogCreateMany with an iterator function
	err := h.CatalogCreateMany(ctx, c.createCatalogName(), models, func(key string, err error) error {

		if err != nil {

			// 🧠 NOTE: HydrAIDE SDK always returns structured, type-safe error objects.
			// These errors can be safely inspected using the helper functions in `error.go`,
			// such as: IsAlreadyExists(err), IsSwampNotFound(err), IsFailedPrecondition(err), etc.
			//
			// Avoid relying on raw error string matching — use the SDK helpers for robustness.
			if hydraidego.IsAlreadyExists(err) {
				// The user already existed in the catalog — skipped silently
				slog.Info("⚠️ User already exists, skipping insert",
					"user_email", key)
			} else {
				// Other error — could be validation or database issue
				slog.Error("🔥 Error inserting user into catalog",
					"user_email", key, "error", err)
			}

		} else {
			slog.Info("✅ Successfully inserted new user into catalog",
				"user_email", key)
		}
		return nil // continue processing
	})

	if err != nil {
		slog.Info("🔥 Bulk insert failed",
			"error", err)
	}

}

// createCatalogName defines the Swamp name used to store the imported users.
// In this example, all records go to `users/catalog/all
func (c *CatalogModelUserCreateManyExample) createCatalogName() name.Name {
	return name.New().Sanctuary("users").Realm("catalog").Swamp("all")
}
