## Struct-first data model — your code is the schema

### What this means

In HydrAIDE the default way to store and read data is by handing the SDK a native struct. There is no separate schema definition, no migration step, no query string to compose. The struct's field tags tell the engine which field is the key, which is the value, and which fields are optional metadata. The same struct shape is used to read the data back.

When richer access is needed — filtering, vector similarity, geographic distance, field-level inspection — the [query engine](query-engine.md) handles it server-side. The struct stays the source of truth; the query engine is the layer you reach for when you need to look at many records through a lens.

### Why it works this way

Every database you adopt comes with its own dialect to learn before anyone on the team can ship safely. A new hire — or a returning contributor — has to map the language they already know onto the database's surface. That gap is where bugs and accidental load problems live.

Storing data as native structs collapses that gap. Code review stays in one language. The shape you read in the code is the shape on the wire and the shape on disk. New contributors are productive on the first day, because there is no second language between them and the data.

📚 **Full runnable examples:** [Go examples](../sdk/go/examples/)

---

### Example — Catalog save

This example saves a user record into a Catalog-type Swamp using `CatalogSave()`. If the key exists, it updates; if not, it creates.

```go
// Package name for your example models
package models

// Standard library imports used by this snippet
import (
    "log"
    "time"

    // Helper packages from the example repo (create context, access SDK, build names)
    "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/utils/hydraidehelper"
    "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/utils/repo"
    "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name"
)

// CatalogModelUserSaveExample defines a typed record (Treasure) stored in a Catalog-type Swamp.
//
// Field tags under `hydraide:"..."` tell HydrAIDE how to serialize and treat each field:
//   - `key`       → unique identifier of the Treasure
//   - `value`     → the main payload (can be primitives or a struct)
//   - metadata    → createdBy/At, updatedBy/At (optional; stored only if non-empty)
//
// You can rename the fields freely; only the tags matter to HydrAIDE.
type CatalogModelUserSaveExample struct {
    UserUUID  string    `hydraide:"key"`       // Unique user ID → becomes the Treasure key
    Payload   *Payload  `hydraide:"value"`     // Business payload → stored in binary with full type safety
    CreatedBy string    `hydraide:"createdBy"` // Optional audit metadata: who created it
    CreatedAt time.Time `hydraide:"createdAt"` // Optional audit metadata: when it was created
    UpdatedBy string    `hydraide:"updatedBy"` // Optional audit metadata: who modified it last
    UpdatedAt time.Time `hydraide:"updatedAt"` // Optional audit metadata: when it was last modified
}

// Payload holds the business-level content of the user. Extend freely as needed.
// HydrAIDE stores it in native binary form (no JSON), preserving exact types.
type Payload struct {
    LastLogin time.Time // Timestamp of the user's last login
    IsBanned  bool      // Example business flag
}

// Save persists the model into a Catalog Swamp.
// - If the key does not exist → it creates a new Treasure.
// - If the key exists and content changes → it updates in place.
// - If nothing changed → it's a no-op.
//
// NOTE:
// Use CatalogCreate() if you want "insert-only" semantics that error on duplicates.
// Use CatalogUpdate() if you want "update-only" semantics that fail if the key is missing.
func (c *CatalogModelUserSaveExample) Save(r repo.Repo) error {
    // Create a timeout-aware context (prevents hangs, enforces upper bound per call)
    ctx, cancel := hydraidehelper.CreateHydraContext()
    defer cancel()

    // Access the HydrAIDE Go SDK through your repo abstraction
    h := r.GetHydraidego()

    // Build the target Swamp name using the naming convention:
    // Sanctuary("users") / Realm("catalog") / Swamp("all")
    // This pattern deterministically maps to a folder/server under the hood.
    swamp := name.New().Sanctuary("users").Realm("catalog").Swamp("all")

    // Perform the save (upsert-like) operation
    // The returned event status (ignored here) tells you if it was New/Modified/NothingChanged.
    _, err := h.CatalogSave(ctx, swamp, c)
    return err
}

// Example usage showing a typical flow
func Example_CatalogSave() {
    // Assume repoInstance implements repo.Repo and already holds a HydrAIDE SDK client
    var repoInstance repo.Repo

    // Prepare a new user record to persist
    user := &CatalogModelUserSaveExample{
        UserUUID: "user-123", // Unique key
        Payload: &Payload{     // Value (any typed struct works)
            LastLogin: time.Now(),
            IsBanned:  false,
        },
        CreatedBy: "admin-service", // Optional metadata
        CreatedAt: time.Now(),       // Optional metadata
    }

    // Save to HydrAIDE (create or update depending on existing state)
    if err := user.Save(repoInstance); err != nil {
        log.Fatalf("failed to save user: %v", err)
    }
}
```

### What you get

- One language end to end. The struct in your code is the record on disk.
- No migrations for additive changes. Optional fields stay backwards-compatible because the engine stores values in their native binary form, not JSON.
- Code review captures both the data shape and the access pattern at the same time.
- When you need cross-record reads — filters, vector search, geographic queries — reach for the [query engine](query-engine.md) and stream results back over gRPC. The struct stays the schema; the query is the lens.
