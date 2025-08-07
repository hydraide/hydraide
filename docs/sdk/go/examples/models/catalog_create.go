package models

import (
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// CatalogModelUserCreateExample demonstrates how to insert a new user ID into
// a HydrAIDE Catalog using CatalogCreate — ensuring no overwrite occurs.
//
// This pattern is ideal when registering users for the first time and you want
// to ensure the key is created only if it does not already exist.
//
// ⚠️ Unlike Save(), this function will return an error if the record already exists.
// This guarantees idempotent registration and prevents accidental data overwrite.
//
// 🧠 This example also demonstrates how to design a minimal Catalog model
// that contains **only a key** — no value (`hydraide:"value"`) and no metadata
// (`createdAt`, `updatedAt`, etc.).
//
// This is useful when:
//   - You only want to track the existence of an ID (e.g. who registered)
//   - You don’t need to store additional user data
//   - You want ultra-compact Swamps with minimal memory and disk footprint
//
// In HydrAIDE, this pattern is fully supported and highly performant — ideal
// for millions of fast inserts where only the key matters.
type CatalogModelUserCreateExample struct {
	UserUUID string `hydraide:"key"` // Unique identifier for the user – used as the Treasure key
}

// SaveUserIfNotExist inserts the user into the catalog only if the record does not already exist.
//
// It uses CatalogCreate(), which automatically fails if the key is already present in the Swamp.
// This eliminates the need to call IsKeyExists() manually and avoids race conditions.
//
// Example usage:
//
//	user := &CatalogModelUserCreateExample{UserUUID: "user-123"}
//	err := user.SaveUserIfNotExist(repo)
//	if err != nil {
//	    log.Fatalf("Failed to create user: %v", err)
//	}
//
// When to use:
//   - Use this during user **registration**, onboarding, or any logic that must ensure
//     the user ID is unique and **not already present**.
//   - Avoids overwriting partial or uninitialized user records.
//   - Perfect for write-once semantics and safe inserts.
func (c *CatalogModelUserCreateExample) SaveUserIfNotExist(r repo.Repo) error {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := r.GetHydraidego()

	// Save the user into the catalog using CatalogCreate.
	// This will attempt to insert the Treasure only if the key does not already exist.
	// If the key exists, an ErrCodeAlreadyExists error will be returned.
	//
	// 🔍 Need more detail about how this function works under the hood?
	// Follow the CatalogCreate() method in the SDK and read its documentation comment.
	// It includes behavior about write safety, error handling, and struct validation logic.
	err := h.CatalogCreate(ctx, c.createCatalogName(), c)

	if err != nil {

		// 🧠 NOTE: HydrAIDE SDK always returns structured, type-safe error objects.
		// These errors can be safely inspected using the helper functions in `error.go`,
		// such as: IsAlreadyExists(err), IsSwampNotFound(err), IsFailedPrecondition(err), etc.
		//
		// Avoid relying on raw error string matching — use the SDK helpers for robustness.
		if hydraidego.IsAlreadyExists(err) {
			// The user already exists in the catalog — no need to create it again.
			// This is not a real error, but depending on your logic,
			// you can decide whether to treat it as a failure or silently skip.
			return nil
		}

		// Any other error is likely a real database issue.
		// You may want to log and propagate it.
		return err
	}

	// Everything succeeded — new record was inserted.
	return nil
}

// RegisterPattern registers the Swamp for the given user catalog model.
// ⚠️ This must be called once on system startup, before using SaveUserIfNotExist().
func (c *CatalogModelUserCreateExample) RegisterPattern(repo repo.Repo) error {
	h := repo.GetHydraidego()

	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	errorResponses := h.RegisterSwamp(ctx, &hydraidego.RegisterSwampRequest{
		// The Swamp pattern name: users/catalog/all
		SwampPattern: c.createCatalogName(),

		// Keep Swamp in memory for 6 hours of idle time
		CloseAfterIdle: time.Second * 21600,

		// Use persistent, disk-backed storage
		IsInMemorySwamp: false,

		// Configure file writing: frequent small chunks
		FilesystemSettings: &hydraidego.SwampFilesystemSettings{
			WriteInterval: time.Second * 10, // flush changes every 10s
			MaxFileSize:   8192,             // 8 KB max chunk size
		},
	})

	if errorResponses != nil {
		return hydraidehelper.ConcatErrors(errorResponses)
	}

	return nil
}

// createCatalogName returns the Name object pointing to the Swamp
// where user IDs will be stored.
// In this example, all users are stored in the Swamp: users/catalog/all
func (c *CatalogModelUserCreateExample) createCatalogName() name.Name {
	return name.New().Sanctuary("users").Realm("catalog").Swamp("all")
}
