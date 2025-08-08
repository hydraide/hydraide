package models

import (
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// CatalogModelUserSaveExample defines a typed record (Treasure) inside a HydrAIDE Catalog Swamp.
//
// A Catalog is a special kind of Swamp in HydrAIDE: it's a type-safe, stream-aware vault
// that groups related Treasures under a common Swamp name. Ideal for use cases like
// admin dashboards, where you want to list, filter or react to user registrations.
//
// This model stores one user per Treasure, using:
//   - `hydraide:"key"`    → for unique identifier (e.g. user ID)
//   - `hydraide:"value"`  → for actual user payload
//   - Optional metadata fields (`createdAt`, `updatedAt`, etc.) for lifecycle tracking
//
// 📌 NOTE: Only the `hydraide` struct tags matter!
//
//   - Field names in Go can be anything (e.g., `UserID`, `UUID`, `ID`)
//   - What HydrAIDE uses is the tag: `hydraide:"key"`, `hydraide:"value"`, etc.
//
// 🧠 You can rename `UserUUID` to `ID` and it still works — as long as the tag remains the same.
//
// 🔍 For a complete reference on how to build Catalog models — including TTL, advanced
// metadata and indexing — see:
//
//	→ `/docs/sdk/go/examples/model_catalog_example.go`
//
// ---
//
// Example usage:
//
//	user := &CatalogModelUserSaveExample{
//	    UserUUID: "user-123",
//	    Payload: &Payload{
//	        LastLogin: time.Now(),
//	        IsBanned:  false,
//	    },
//	    CreatedBy: "admin-service",
//	    CreatedAt: time.Now(),
//	}
//
//	if err := user.Save(repoInstance); err != nil {
//	    log.Fatalf("Failed to save user: %v", err)
//	}
//
// This will persist the user to the Swamp `users/catalog/all`. If the entry already exists and
// the payload has changed, it will be updated. Otherwise, the operation is a no-op.
type CatalogModelUserSaveExample struct {
	UserUUID  string    `hydraide:"key"`       // Unique identifier for the user – used as the Treasure key
	Payload   *Payload  `hydraide:"value"`     // Actual content of the user profile
	CreatedBy string    `hydraide:"createdBy"` // Who created this user record (optional)
	CreatedAt time.Time `hydraide:"createdAt"` // When was this record created (optional)
	UpdatedBy string    `hydraide:"updatedBy"` // Who last modified the record (optional)
	UpdatedAt time.Time `hydraide:"updatedAt"` // Last modification timestamp (optional)
}

// Payload represents the business-level content of a user.
// Extend this struct with any fields relevant to your use case.
// HydrAIDE stores and hydrates it as a typed binary object.
type Payload struct {
	LastLogin time.Time // When did the user last log in
	IsBanned  bool      // Whether the user is currently banned
}

// Save persists this user into the HydrAIDE Catalog.
//
// If the record does not exist, it will be created.
// If it exists and the payload or metadata differs, it will be updated.
// Otherwise, nothing happens.
//
// The return value includes an event status (New, Modified, NothingChanged)
// which can be used to trigger downstream logic if needed.
//
// ⚠️ Important:
//
// This function is ideal when updating an **already existing user**, such as updating their
// login timestamp, toggling their ban status, or modifying metadata.
//
// However, it is **not suitable** in the following cases:
//
// ❶ You need to ensure the record is created *only if* it doesn't already exist (e.g. user registration).
//
//	In those cases, use:
//	- `CatalogCreate()` instead of `CatalogSave()` – only inserts if the key does not exist.
//	- Or use `IsKeyExists()` before saving:
//
// ❷ You have **not loaded the original data** before updating.
//
//	HydrAIDE saves the entire struct as-is. So if your model fields are partially empty,
//	calling `Save()` can unintentionally **overwrite existing values with blanks**.
//
//	➤ Always call `CatalogRead` (or similar load function) first to hydrate the model
//	  from the database before making updates.
//
// This ensures:
// - You don’t unintentionally delete fields with empty values
// - You retain metadata and previous values
// - You act on a fully hydrated model
func (c *CatalogModelUserSaveExample) Save(r repo.Repo) error {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := r.GetHydraidego()

	// Save the user into the catalog. This will either insert or update the entry.
	eventStatus, err := h.CatalogSave(ctx, c.createCatalogName(), c)

	// Optional: handle the result of the save operation (new, modified, unchanged)
	switch eventStatus {
	case hydraidego.StatusNew:
		// The record was newly created
	case hydraidego.StatusModified:
		// The record existed and was updated
	case hydraidego.StatusNothingChanged:
		// No changes were detected
	default:
		// An error likely occurred
	}

	return err
}

// RegisterPattern declares this Catalog Swamp to the HydrAIDE engine.
//
// This method defines how the Swamp behaves in terms of:
// - memory lifetime (CloseAfterIdle)
// - write behavior (WriteInterval)
// - storage limits (MaxFileSize)
// - persistence model (disk-backed)
//
// This setup is ideal for high-read catalogs that need occasional writes.
func (c *CatalogModelUserSaveExample) RegisterPattern(r repo.Repo) error {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := r.GetHydraidego()

	// RegisterSwamp always returns a []error.
	// Each error (if any) represents a failure during Swamp registration on a HydrAIDE server.
	//
	// ⚠️ Even when only a single Swamp pattern is registered, HydrAIDE may attempt to replicate or validate
	// the pattern across multiple server nodes (depending on your cluster).
	//
	// ➕ Return behavior:
	// - If all servers succeeded → returns nil
	// - If one or more servers failed → returns a non-nil []error
	//
	// 🧠 To convert this into a single `error`, you can use the helper:
	//     hydraidehelper.ConcatErrors(errorResponses)
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

// createCatalogName returns the name of the Catalog Swamp
// where the user data will be stored.
func (c *CatalogModelUserSaveExample) createCatalogName() name.Name {
	return name.New().Sanctuary("users").Realm("catalog").Swamp("all")
}
