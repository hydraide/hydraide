## ✍️ No More Queries — No SELECT, no WHERE. **Your struct is the query.**

### Philosophy

I’ve always found it painful that every database forces its own query language on developers. Each engine has a different syntax you must learn before you can be productive. The bigger problem is onboarding: even if you hire a great engineer, they still need to learn the database’s query dialect before they can ship safely. Meanwhile, a seemingly harmless query can be inefficient—or outright destabilize the system.

HydrAIDE was designed so that any developer who already knows the host language can be productive **on day one**. You never have to “step out” of code into a separate query language, and you can’t run unstable/inefficient queries by accident. Ramp‑up time is minimal, and when you join an existing codebase you immediately understand what each piece of code does—because the code *is* the query.

That is why HydrAIDE stores data via native **structs** in Go (and in every native SDK), and exposes simple SDK functions bound to gRPC methods. The goal is a developer‑friendly system you’ll enjoy using from the first minute—without learning a brand‑new language.

📚 **All models types with full, runnable examples:** [models examples](../sdk/go/examples/models)

---

### Example — Catalog Save Model

Below is a minimal example that saves a user record into a **Catalog** Swamp using `CatalogSave()`. If the key exists, it updates; if not, it creates.

```go
// Package name for your example models
package models

// Standard library imports used by this snippet
import (
    "log"
    "time"

    // Helper packages from the example repo (create context, access SDK, build names)
    "github.com/hydraide/hydraide/docs/sdk/go/examples/models/utils/hydraidehelper"
    "github.com/hydraide/hydraide/docs/sdk/go/examples/models/utils/repo"
    "github.com/hydraide/hydraide/sdk/go/hydraidego/name"
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
    LastLogin time.Time // Timestamp of the user’s last login
    IsBanned  bool      // Example business flag
}

// Save persists the model into a Catalog Swamp.
// - If the key does not exist → it creates a new Treasure.
// - If the key exists and content changes → it updates in place.
// - If nothing changed → it’s a no-op.
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
        CreatedAt: time.Now(),        // Optional metadata
    }

    // Save to HydrAIDE (create or update depending on existing state)
    if err := user.Save(repoInstance); err != nil {
        log.Fatalf("failed to save user: %v", err)
    }
}
```

**Why this matters**

* No separate query language to learn or maintain
* Code review stays in one place (the language you already use)
* Safer by construction: no accidental "bad queries"
* The code you read is the exact behavior HydrAIDE executes

> With HydrAIDE, you don’t query your data — you **shape** it. Your struct is the query.
