package models

import (
	"log/slog"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// CatalogModelUserSessionLog represents a single login session for a specific user.
//
// Each record is stored in a dedicated Swamp named after the user ID (e.g. `sessions/by-user/12345`),
// allowing fast, isolated reads per user.
//
// Ideal for per-user dashboards, session audit logs, and authentication analytics.
type CatalogModelUserSessionLog struct {
	SessionID string    `hydraide:"key"`       // Unique session ID
	LoginIP   string    `hydraide:"value"`     // Originating IP address
	CreatedAt time.Time `hydraide:"createdAt"` // Timestamp of the session
}

// ReadLastSessions reads the last N login sessions for a specific user.
//
// This demonstrates how to use `CatalogReadMany()` with dynamic Swamp routing,
// where each user has their own dedicated Swamp (e.g. `sessions/by-user/<userID>`).
//
// ✅ Purpose:
// - Display recent logins in a dashboard
// - Audit user activity
// - Stream per-user session history in descending order (latest first)
func (c *CatalogModelUserSessionLog) ReadLastSessions(r repo.Repo, userID string, limit int) ([]*CatalogModelUserSessionLog, error) {

	// Create a cancellable context to avoid hanging calls or leaks.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Get the HydrAIDE SDK client from the repo wrapper.
	h := r.GetHydraidego()

	var sessions []*CatalogModelUserSessionLog

	// ⚙️ Define the index settings for the read operation.
	//
	//   IndexType:     IndexCreationTime → sort by `createdAt` metadata
	//   IndexOrder:    IndexOrderDesc    → newest sessions first
	//   From:          0                 → start from the top
	//   Limit:         N                 → number of sessions to return
	//
	// This pattern is ideal for time-based logs like user sessions.
	//
	// 📌 Internal note:
	//
	// In contrast to traditional databases, HydrAIDE does NOT create physical indexes.
	// The `IndexType` here is purely logical — used to sort/filter records at read time.
	//
	// For example, if you use `IndexCreationTime`:
	//
	// ── Traditional DB:
	// - Requires an explicit index (e.g. B-tree on `createdAt`)
	// - Maintains that index on every insert/update
	// - Stores additional index data on disk
	//
	// ── HydrAIDE:
	// - Reads all matching Treasures into memory
	// - Sorts them using their `createdAt` metadata (stored with the record)
	// - No extra disk writes, no duplication, no sync needed
	//
	// 🔬 Under the hood:
	// - HydrAIDE internally builds **in-memory hash indexes** during Swamp hydration.
	// - These indexes are built **only once**, the first time the Swamp is loaded
	//   and queried with an `IndexType` filter.
	// - Indexes are NOT persisted — they are rebuilt from disk when the Swamp is reopened.
	//
	// 💡 Tip for maximum performance:
	//   Keep the Swamp in memory longer by using `CloseAfterIdle: longDuration`
	//   during `RegisterSwamp()`. This prevents frequent unload/load cycles,
	//   so the index stays hot and lookups remain instant.
	//
	// ✅ This model makes HydrAIDE ideal for:
	//   - log-style time-series ingestion
	//   - fast reads with minimal write overhead
	//   - small-to-medium Swamps that are queried often
	//
	// ⚠️ If you need to sort huge data across all users or all time,
	//    consider distributing it across multiple Swamps (e.g., per user, per day).
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime, // Use `createdAt` metadata for sorting
		IndexOrder: hydraidego.IndexOrderDesc,    // Descending = newest first
		From:       0,                            // No offset (start from latest)
		Limit:      int32(limit),                 // Limit the number of results
	}

	// Generate the Swamp name for this user's session data
	// Example: sessions/by-user/12345
	swamp := c.createSwampForUser(userID)

	// Perform the indexed read from the user's Swamp
	err := h.CatalogReadMany(ctx, swamp, index, CatalogModelUserSessionLog{}, func(model any) error {

		// The SDK will provide a new model instance per result — type-assert to expected type
		session, ok := model.(*CatalogModelUserSessionLog)
		if !ok {
			// If model type is not what we expect, return an error to halt the iteration
			return hydraidego.NewError(hydraidego.ErrCodeInvalidModel, "unexpected session model")
		}

		// Append the valid session to our result slice
		sessions = append(sessions, session)
		return nil
	})

	// Handle possible failure from CatalogReadMany or the iterator
	if err != nil {
		slog.Error("Failed to read sessions", "userID", userID, "error", err)
		return nil, err
	}

	// Log successful read
	slog.Info("Sessions loaded", "userID", userID, "count", len(sessions))
	return sessions, nil
}

// RegisterPattern registers a wildcard Swamp pattern for user-based session storage.
//
// Applies to Swamps like:
//   - sessions/by-user/123
//   - sessions/by-user/abc-xyz
//
// ⚠️ Why wildcard (`*`) is used **in this example**:
//
// In this model, each user has their own dedicated Swamp (e.g. `sessions/by-user/<userID>`)
// to store their session logs in isolation.
//
// Since we don’t know all user IDs in advance, we register a **wildcard pattern**
// with `Swamp("*")` to apply shared storage settings to **all possible user-specific Swamps**.
//
// This pattern is useful when:
//   - You split data per user, tenant, or entity
//   - You want to apply consistent configuration without per-instance registration
//
// 💡 This setup is ideal for models like session logs, where each user writes to a unique Swamp.
//
// 🕒 Why keep it in memory for 30 minutes?
//
//   - `CloseAfterIdle: 30m` tells HydrAIDE to keep each Swamp in memory
//     **for 30 minutes after the last access**.
//   - This avoids repeated hydration/unload cycles during active periods.
//
// ⚡️ Performance tip:
// Keeping a Swamp memory-resident ensures faster read access (no disk load, no index rebuild)
// especially if you're calling `ReadLastSessions()` frequently (e.g., dashboard, audit tools).
func (c *CatalogModelUserSessionLog) RegisterPattern(repo repo.Repo) error {
	h := repo.GetHydraidego()
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

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
		// Wildcard pattern: applies to ALL user-specific session logs
		SwampPattern: name.New().Sanctuary("sessions").Realm("by-user").Swamp("*"),

		// Keep each session Swamp in memory for 30 minutes after last use
		CloseAfterIdle: time.Minute * 30,

		// Disk-backed storage for persistence
		IsInMemorySwamp: false,

		// Write small chunks frequently to reduce data loss and latency
		FilesystemSettings: &hydraidego.SwampFilesystemSettings{
			WriteInterval: time.Second * 10, // flush every 10s
			MaxFileSize:   8192,             // 8KB chunk size
		},
	})

	if errorResponses != nil {
		return hydraidehelper.ConcatErrors(errorResponses)
	}
	return nil
}

// createSwampForUser returns the Swamp where this user's session logs are stored.
// Example: sessions/by-user/12345
func (c *CatalogModelUserSessionLog) createSwampForUser(userID string) name.Name {
	return name.New().Sanctuary("sessions").Realm("by-user").Swamp(userID)
}
