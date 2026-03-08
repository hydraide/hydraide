# 🐹 HydrAIDE SDK – Go Edition

Welcome to the official **HydrAIDE SDK for Go**, your gateway to building intelligent,
distributed, real-time systems using the HydrAIDE engine.

This SDK provides programmatic access to HydrAIDE's powerful features such as swamp-based data structures,
lock-free operations, real-time subscriptions, and stateless routing, all tailored to Go developers.


## 📚 Table of Contents

1. [🔌 Connect to the HydrAIDE Server Using the SDK](#-connect-to-the-hydraide-server-using-the-sdk)
2. [📦 At a Glance](#-at-a-glance)
3. [🧭 Naming Conventions](#-naming-conventions)
3. [🧠 System](#-system)
4. [🔐 Business Logic](#-business-logic)
5. [🌿 Swamp & Treasure](#-swamp--treasure)
6. [🧬 Profile Swamps](#-profile-swamps)
7. [🗂️ Catalog Swamps](#-catalog-swamps)
8. [📚 Good to Know: Split Catalogs When Needed](#-good-to-know-split-catalogs-when-needed)
9. [🧯 When Not to Use Catalogs](#-when-not-to-use-catalogs)
10. [🔍 Server-Side Filtering & Streaming](#-server-side-filtering--streaming)
11. [➕ Increment / Decrement – Atomic State Without the Overhead](#-increment--decrement--atomic-state-with-metadata-control)
12. [📌 Slice & Reverse Indexing in HydrAIDE](#-slice--reverse-indexing-in-hydraide)
13. [🧪 Testing with Real Database Connection](#-testing-with-real-database-connection)

---

## 🔌 Connect to the HydrAIDE Server Using the SDK

The first and most essential step is establishing a connection to the HydrAIDE server using the Go SDK.

To do this, implement the `repo` package. This package is typically placed under `/utils/repo` and should be 
initialized during your application's startup sequence.

You can find the repo implementation and usage examples here:

📁 [`repo.go`](../../../sdk/go/hydraidego/utils/repo/repo.go)

### How to Start Your Server Using the Repo Package

For a complete working example of how to initialize and run your service using the `repo` package, take a look at the demo application:

▶️ [`main.go` in app-queue](examples/applications/app-queue/main.go) — a minimal end-to-end example of SDK setup and Swamp registration with a queue service

---

## 📦 At a Glance

Below you'll find a wide range of examples and documentation — including complete Go files and ready-made solutions — showing how to use the SDK in **production-ready applications**.

---

## 🧭 Naming Conventions

HydrAIDE uses a structured three-level naming system to deterministically route, organize, and store data across distributed servers — without requiring coordination or a central orchestrator.

Each data location is uniquely identified using a **Name**, which follows the pattern:

```
Sanctuary → Realm → Swamp
```

This naming hierarchy allows HydrAIDE to:

* Automatically map names to consistent server locations (using `GetIslandID`)
* Create predictable folder paths on disk
* Enable stateless client-side routing
* Keep lookup performance at **O(1)** regardless of system size

### ✅ Basic Example

```go
name := name.New().
    Sanctuary("users").
    Realm("profiles").
    Swamp("alice123")

fmt.Println(name.Get())            // users/profiles/alice123
fmt.Println(name.GetIslandID(100)) // e.g. 42
```

This name will deterministically map to one of 100 Islands (physical folders or servers). 
It always resolves to the **same target** for the same input, across all clients.

### 🚫 Constraints

To keep routing fast and deterministic, **names must follow strict constraints**:

* All three components (`Sanctuary`, `Realm`, `Swamp`) are **required**
* Each component must be at least **1 character long**
* The `/` character is **not allowed** in any part of the name
  It is used internally as a structural separator, using it would break routing
* The use of **alphanumeric characters only** (`a–z`, `A–Z`, `0–9`) is **strongly recommended**
* Wildcards (`*`) are allowed only for internal system use (e.g. pattern-based registration)

> ℹ️ No runtime validation is enforced by the SDK for performance reasons.
> The `name` package is one of the most frequently called components in HydrAIDE — adding input validation would 
> slow it down by 10–20×. It is the **developer’s responsibility** to follow the documented rules.

### ✍️ Constructing a Name

You can create a `name.Name` using fluent chaining:

```go
n := name.New().
    Sanctuary("projects").
    Realm("files").
    Swamp("report-2025-Q1")
```

Or reconstruct one from an existing string path:

```go
n := name.Load("projects/files/report-2025-Q1")
```

This is useful when deserializing names from persisted storage or incoming metadata.

### 🧪 Common Patterns

| Use case          | Sanctuary | Realm    | Swamp         |
| ----------------- |----------| -------- | ------------- |
| User profiles     | users    | profiles | user-123      |
| Game achievements | games    | unlocked | level-10-boss |
| Analytics         | domains  | ai       | example.com   |
| Chat rooms        | chat     | room     | room-42       |
| Queued tasks      | tasks    | catalog  | main          |

These names are used consistently across the SDK, in Catalogs, Profiles, event subscriptions, locking, routing, and more.

### 🔐 Island Mapping

To determine where data should physically reside, you can use:

```go
island := name.GetIslandID(1000) // e.g. returns 774
```

This returns a **1-based index** from 1 to N, based on a fast, collision-resistant `xxhash` of the full path.
All clients using the same `allIslands` count will resolve the same name to the same server — even without coordination.

---

### Profiles and Catalogs

The Go SDK offers a simple yet powerful way to manage data through two fundamental patterns: **Profiles** and **Catalogs**.

**Profiles** are designed to represent all structured data related to a single entity — for example, a user.
Each user has their own dedicated Profile Swamp, which can store all of their relevant information such as name, avatar, registration date, last login time, and more.
A profile can hold any amount of data — but always belongs to exactly one entity (like one user).

📄 [`model_profile_example.go`](examples/models/model_profile_example.go)

**Catalogs**, on the other hand, are key–value Swamps where you can store many unique keys — each mapped to its own custom value.
This is ideal for scenarios like tracking all registered user IDs, counting how many users exist in total, or displaying a list of users in an admin dashboard.

📄 [`model_catalog_example.go`](examples/models/model_catalog_example.go)

In both cases, data is defined using regular Go `struct`s decorated with HydrAIDE field tags.
You work with the data through model-bound methods that make saving, querying, or subscribing extremely simple and intuitive.

Throughout this SDK documentation (except for the Heartbeat example), all model samples are either Profile-based or Catalog-based, reflecting real production patterns.

> 💬 If anything is unclear or if you'd like to request improvements or clarification in the examples, feel free to open a **Docs Improvement issue**. We'd love your feedback.


### 🧠 System

| Function  | SDK Status | Example Go Models and Docs                                  |
| --------- | ------- |-------------------------------------------------------------|
| Heartbeat | ✅ Ready | [basics_heartbeat.go](examples/models/basics_heartbeat.go)  |

---

### 🔐 Business Logic

The functions under Business Logic enable **cross-cutting coordination** across distributed services.

These are not tied to a specific Swamp or Treasure — they operate on shared, logical domains like user balances,
order flows, or transaction states.

- `Lock()` acquires a **blocking distributed lock** for a given domain key to ensure no concurrent execution happens.
- `Unlock()` safely releases it using a provided lock ID.

Use these when you need to **serialize operations** across services or modules.

Ideal for:
- Credit transfers
- Order/payment sequences
- Ensuring consistency during critical logic

| Function | SDK Status | Example Go Models and Docs                                     |
| -------- | ------- |----------------------------------------------------------------|
| Lock     | ✅ Ready | [basics_lock_unlock.go](examples/models/basics_lock_unlock.go) |
| Unlock   | ✅ Ready | [basics_lock_unlock.go](examples/models/basics_lock_unlock.go) |

---

### 🌿 Swamp & Treasure

These functions manage the lifecycle and existence of Swamps (data containers) and their Treasures (records),
including registration, validation, destruction, and real-time subscriptions.

| Function        | SDK Status | Example Go Models and Docs                                               |
| --------------- | ---------- |--------------------------------------------------------------------------|
| RegisterSwamp   | ✅ Ready | [basics_register_swamp.go](examples/models/basics_register_swamp.go)     |
| DeRegisterSwamp | ✅ Ready | [basics_deregister_swamp.go](examples/models/basics_deregister_swamp.go) |
| IsSwampExist    | ✅ Ready | [basics_is_swamp_exist.go](examples/models/basics_is_swamp_exist.go)     |
| IsKeyExists     | ✅ Ready | [basics_is_key_exist.go](examples/models/basics_is_key_exist.go)         |
| Count           | ✅ Ready | [basics_count.go](examples/models/basics_count.go)                       |
| Destroy         | ✅ Ready | [basics_destroy.go](examples/models/basics_destroy.go)                   |
| DestroyBulk     | ✅ Ready | Bulk destroy multiple swamps via bidirectional streaming                  |
| Subscribe       | ✅ Ready | [basics_subscribe.go](examples/models/basics_subscribe.go)               |

---

### 🧬 Profile Swamps

**Profile Swamps** are designed for storing rich, structured data tied to a single unique entity — such as a user, website, or property.  
They are optimized for managing heterogeneous fields (e.g., name, timestamps, nested metadata) in a **single logical unit**, addressed by a unique Swamp name.

Unlike Catalogs (which store many entries via keys), Profiles represent **one entity per Swamp**, making them ideal for persistent, reference-level data structures.

#### 📌 Common Use Cases

- 👤 A user account with fields like email, avatar, registration date, and login history
- 🌐 A website’s core configuration: domain, engine type, description, status
- 🏠 A real estate listing: square footage, location, type, last updated timestamp
- 🧩 Any entity that has a stable identity and holds multiple fields under it

#### ✅ Key Characteristics

- 🔹 Accessed by **Swamp name**, not key or filter
- 🧠 Efficient binary format with `hydraide:"omitempty"` support
- ❌ Automatic per-field delete method support using the `hydraide:deletable` tag
- 📦 Supports nested pointer structs and typed primitives
- 🔄 Used for full hydration (ProfileRead) and overwrite (ProfileSave)
- 🔐 Can be locked at the Swamp level if needed

#### 📦 Example Use Case: User Profile

```go
user := &UserProfile{UserID: "user-123"}
_ = user.Load(repo) // Hydrates entire profile

user.Email = "new@email.com"
user.Preferences.DarkMode = true
_ = user.Save(repo) // Saves all changes at once
```

Internally, HydrAIDE stores this under a Swamp like:

```
/users/profiles/user-123
```

Each field is stored in binary chunks — only if the value is present (thanks to `hydraide:"omitempty"`).

#### 📂 SDK Example Files

| Function                       | SDK Status | Go Example                                                       |
|--------------------------------| ---------- | ---------------------------------------------------------------- |
| `Profile Save, Read, Destroy` | ✅ Ready    | [profile_save_read_destroy.go](examples/models/profile_save_read_destroy.go)   |
| `Profile Read Batch` | ✅ Ready    | [profile_read_batch.go](examples/models/profile_read_batch.go)   |
| `Profile Save Batch` | ✅ Ready    | [profile_save_batch.go](examples/models/profile_save_batch.go)   |

🧪 **Looking for a complete production-ready model?**
Check out [profile_save_read_destroy.go](examples/models/profile_save_read_destroy.go) — a real-world example with nested structs, 
timestamps, and struct pointers for user avatars, preferences, and security.

#### 🚀 Bulk Profile Operations with Batch Functions

When working with **multiple Profile Swamps**, using individual operations in a loop is inefficient because it creates one network round-trip per profile.

**ProfileReadBatch and ProfileSaveBatch** solve this by processing all profiles in **one or few gRPC calls**, dramatically improving performance.

##### 📥 ProfileReadBatch - Bulk Loading

```go
// ❌ Slow approach: 100 network calls
for _, userID := range userIDs {
    user := &UserProfile{}
    client.ProfileRead(ctx, name.New().Sanctuary("users").Realm("profiles").Swamp(userID), user)
}

// ✅ Fast approach: 1 network call
swampNames := []name.Name{
    name.New().Sanctuary("users").Realm("profiles").Swamp("alice"),
    name.New().Sanctuary("users").Realm("profiles").Swamp("bob"),
    // ... 98 more
}

var results []*UserProfile
client.ProfileReadBatch(ctx, swampNames, &UserProfile{}, func(swampName name.Name, model any, err error) error {
    if err != nil {
        log.Printf("Failed to load %s: %v", swampName.Get(), err)
        return nil // Continue with other profiles
    }
    profile := model.(*UserProfile)
    results = append(results, profile)
    return nil
})
```

📖 **Full examples and best practices:** [profile_read_batch.go](examples/models/profile_read_batch.go)

##### 💾 ProfileSaveBatch - Bulk Saving

```go
// ❌ Slow approach: 100+ network calls
for _, profile := range profiles {
    client.ProfileSave(ctx, name.New().Sanctuary("users").Realm("profiles").Swamp(profile.UserID), profile)
}

// ✅ Fast approach: 1-3 network calls (grouped by server)
swampNames := []name.Name{
    name.New().Sanctuary("users").Realm("profiles").Swamp("alice"),
    name.New().Sanctuary("users").Realm("profiles").Swamp("bob"),
    // ... 98 more
}

models := []any{&profile1, &profile2, ...} // Must match swampNames length

client.ProfileSaveBatch(ctx, swampNames, models, func(swampName name.Name, err error) error {
    if err != nil {
        log.Printf("Failed to save %s: %v", swampName.Get(), err)
        return nil // Continue with other profiles
    }
    log.Printf("✅ Saved %s", swampName.Get())
    return nil
})
```

📖 **Full examples and best practices:** [profile_save_batch.go](examples/models/profile_save_batch.go)

**Performance improvement: 20-100x faster** for bulk operations! 🚀

Key features:
- ✅ Automatic server-side routing and grouping
- ✅ Deletable field support (automatic cleanup)
- ✅ Per-profile error handling via iterator
- ✅ Supports omitempty tags
- ✅ Works across multiple servers transparently

---

### 🗂️ Catalog Swamps

**Catalog Swamps** are optimized for storing *structured, queryable lists* of entries — such as users, logs, tags, messages, or domain entries — where each item follows a common schema and is stored using a **unique key** inside a shared Swamp.

This model fits best when you need to:

* 💾 Store hundreds, thousands, or millions of typed entries
* 🔍 Query individual entries by key (CatalogRead)
* 📊 Filter or stream entries based on time or value (CatalogReadMany)
* ✍️ Write or update entries using predictable logic (Create, Save, Update)
* 🧠 Keep track of creation/update time and metadata (e.g. who added it)

#### ✅ Key Characteristics

* 🔑 Accessed by **record key**, within a named Swamp
* 🔁 Supports one-to-many and many-to-many storage patterns
* 📌 Highly efficient for *appendable*, *reactive* data types (e.g. events, logs)
* 🧩 Can use metadata decorators: `createdBy`, `createdAt`, `updatedBy`, `updatedAt`
* 🧪 Index-based read operations with configurable order & limit
* 🧠 Ideal for structured slices, trees, or versioned record lists
* 🔄 Fully reactive: supports real-time streaming via Subscribe()
* 🆕 Supports `omitempty` decorator for all fields except the `key`

#### 🆕 About `omitempty`

In Catalog Swamps, every field **except the key** can use the `omitempty` decorator in its `hydraide` tag.  

* If a field is tagged with `omitempty` and its value is empty/zero/nil:
  * It will **not** be uploaded to HydrAIDE,
  * It will **not** be validated,
  * It will **not** be stored in memory or on disk.  
* This is especially useful for metadata fields like `updatedAt` and `updatedBy`, which should remain absent when creating a new record.  
* Later, when updates occur, these fields can be set and will then be stored normally.

👉 Example:

```go
UpdatedBy string    `hydraide:"updatedBy,omitempty"`
UpdatedAt time.Time `hydraide:"updatedAt,omitempty"`
```

If these fields are empty during initial creation, they will simply not exist in HydrAIDE.

#### 📌 Common Use Cases

* 👥 **Users catalog** – keyed by userID, stores last login, ban status, etc.
* 📓 **Notes or messages** – keyed by noteID, stores message text, timestamps
* 🧠 **Tags or references** – documents stored under tag-named Swamps
* 📈 **Event logs** – every entry is append-only, searchable by creation time
* 🔐 **Lock tables** – key is the lock, value is who holds it and until when

#### 📦 Example: Storing Users in a Catalog

```go
user := &CatalogModelUser{
	UserUUID: "user-123",
	Payload: &Payload{
		LastLogin: time.Now(),
		IsBanned:  false,
	},
	CreatedBy: "auth-service",
	CreatedAt: time.Now(),
	UpdatedBy: "",                 // will be ignored, due to omitempty
	UpdatedAt: time.Time{},        // will be ignored, due to omitempty
}

_ = user.Save(repo) // Upserts the record
```

This stores a Treasure in:

```
/users/catalog/all → key: user-123 → value: Payload + metadata
```

HydrAIDE will track when and who wrote the data, and can later stream or react to changes over time.
Because of `omitempty`, `UpdatedBy` and `UpdatedAt` are not stored until they actually hold values.

--- 

#### 🔎 Indexed Reads

You can stream entries by time using:

```go
index := &hydraidego.Index{
	IndexType:  hydraidego.IndexCreationTime,
	IndexOrder: hydraidego.IndexOrderDesc,
	Limit:      10,
}
_ = h.CatalogReadMany(ctx, swampName, index, CatalogModelUser{}, func(m any) error { ... })
```

Unlike relational databases, **HydrAIDE builds indexes in memory on-demand** using fast, in-memory hashing — reducing storage duplication and ensuring sub-ms reads in hydrated Swamps.
To keep performance high, consider keeping the Swamp in memory longer (e.g. `CloseAfterIdle: 1h`).

---

#### 🔎 Batch key-based read — CatalogReadBatch

CatalogReadBatch is ideal when you already know the exact keys you want to fetch from the same Swamp and need to retrieve them in one fast roundtrip.

- Sends a single gRPC request with all provided keys
- Silently skips non-existent keys (no error; they’re simply not included)
- For each found Treasure, creates a fresh instance of the provided model type and passes it to your iterator
- If your iterator returns an error, processing stops immediately and that error is returned

Requirements:
- Iterator must not be nil
- Model must be a non-pointer type (the SDK internally creates new instances per record)

Quick example:

```go
// A basic user record (key + value + metadata)
type CatalogModelUserBasic struct {
    UserID    string    `hydraide:"key"`
    Name      string    `hydraide:"value"`
    CreatedBy string    `hydraide:"createdBy"`
    CreatedAt time.Time `hydraide:"createdAt"`
}

func ReadUsersByIDs(r repo.Repo, ids []string) ([]*CatalogModelUserBasic, error) {
    if len(ids) == 0 {
        return nil, nil
    }
    ctx, cancel := hydraidehelper.CreateHydraContext()
    defer cancel()

    h := r.GetHydraidego()
    swamp := name.New().Sanctuary("users").Realm("catalog").Swamp("all")

    out := make([]*CatalogModelUserBasic, 0, len(ids))
    err := h.CatalogReadBatch(ctx, swamp, ids, CatalogModelUserBasic{}, func(m any) error {
        u := m.(*CatalogModelUserBasic)
        out = append(out, u)
        return nil
    })
    return out, err
}
```

Great for:
- Loading profiles by a set of IDs
- Fast cache warm-up
- Bulk validation/verification
- Reading configuration entries by a list of keys

> Full example: [catalog_read_batch.go](examples/models/catalog_read_batch.go)

---

#### 🔍 Server-Side Filtering & Streaming

HydrAIDE supports **server-side filtering** with **nested AND/OR logic** and **streaming reads** — enabling efficient querying of large datasets without loading everything into memory or transferring non-matching records over the network.

##### Why Streaming?

`CatalogReadMany` loads all results into a single gRPC response message. For small-to-medium datasets this is fine, but when a Swamp contains millions of Treasures, this becomes a problem:

- **Memory**: The entire result set must fit in one proto message
- **Latency**: You wait for ALL results before processing the first one
- **Network**: All data travels in one large payload

`CatalogReadManyStream` solves this by streaming each matching Treasure individually over a gRPC server-stream.

##### FilterGroup — Nested AND/OR Logic

Filters are organized into **FilterGroups** that support recursive AND/OR logic. A FilterGroup contains leaf-level filters and nested sub-groups, all combined with either AND or OR logic.

Use `FilterAND(...)` and `FilterOR(...)` to build filter trees:

```go
// Simple AND: price > 100 AND status == "active"
filters := hydraidego.FilterAND(
    hydraidego.FilterFloat64(hydraidego.GreaterThan, 100.0),
    hydraidego.FilterString(hydraidego.Equal, "active"),
)

// Nested AND/OR: price > 100 AND (status == "active" OR status == "pending")
filters := hydraidego.FilterAND(
    hydraidego.FilterFloat64(hydraidego.GreaterThan, 100.0),
    hydraidego.FilterOR(
        hydraidego.FilterString(hydraidego.Equal, "active"),
        hydraidego.FilterString(hydraidego.Equal, "pending"),
    ),
)

// Deep nesting: (A AND B) OR (C AND D)
filters := hydraidego.FilterOR(
    hydraidego.FilterAND(
        hydraidego.FilterInt32(hydraidego.GreaterThan, 10),
        hydraidego.FilterString(hydraidego.Contains, "premium"),
    ),
    hydraidego.FilterAND(
        hydraidego.FilterInt32(hydraidego.LessThanOrEqual, 10),
        hydraidego.FilterString(hydraidego.StartsWith, "free"),
    ),
)
```

Evaluation rules:
- **AND**: ALL leaf filters AND ALL sub-groups must be true
- **OR**: at least ONE leaf filter OR ONE sub-group must be true
- **Empty group** (no filters, no sub-groups): passes all Treasures
- **nil filters**: no filtering applied (all Treasures pass)

##### Server-Side Filters

Filters are evaluated **on the server** before results are sent to the client. This means non-matching Treasures never leave the HydrAIDE server — saving bandwidth, memory, and processing time.

**Available filter types** (matching the Treasure's typed value fields):

| Constructor | Matches Against |
|-------------|----------------|
| `FilterInt8(op, value)` | Treasure's `Int8Val` |
| `FilterInt16(op, value)` | Treasure's `Int16Val` |
| `FilterInt32(op, value)` | Treasure's `Int32Val` |
| `FilterInt64(op, value)` | Treasure's `Int64Val` |
| `FilterUint8(op, value)` | Treasure's `Uint8Val` |
| `FilterUint16(op, value)` | Treasure's `Uint16Val` |
| `FilterUint32(op, value)` | Treasure's `Uint32Val` |
| `FilterUint64(op, value)` | Treasure's `Uint64Val` |
| `FilterFloat32(op, value)` | Treasure's `Float32Val` |
| `FilterFloat64(op, value)` | Treasure's `Float64Val` |
| `FilterString(op, value)` | Treasure's `StringVal` |
| `FilterBool(op, value)` | Treasure's `BoolVal` |

**Relational operators** (for all types):

| Operator | Meaning |
|----------|---------|
| `Equal` | `==` |
| `NotEqual` | `!=` |
| `GreaterThan` | `>` |
| `GreaterThanOrEqual` | `>=` |
| `LessThan` | `<` |
| `LessThanOrEqual` | `<=` |

**String-specific operators** (only valid with `FilterString` / `FilterBytesFieldString`):

| Operator | Meaning |
|----------|---------|
| `Contains` | String contains substring (case-sensitive) |
| `NotContains` | String does NOT contain substring (case-sensitive) |
| `StartsWith` | String starts with prefix (case-sensitive) |
| `EndsWith` | String ends with suffix (case-sensitive) |

**Existence operators** (for all types — the CompareValue is ignored, only the field type matters):

| Operator | Meaning |
|----------|---------|
| `IsEmpty` | Field is `nil`/unset, or empty string `""` for strings |
| `IsNotEmpty` | Field exists and is non-empty |

**Map key operators** (only valid with `FilterBytesFieldString` — checks if a key exists in a `map[string]...` inside BytesVal):

| Operator | Meaning |
|----------|---------|
| `HasKey` | The map at BytesFieldPath contains the specified key |
| `HasNotKey` | The map at BytesFieldPath does NOT contain the specified key |

These operators work with both primitive Treasure fields and BytesField struct fields:

```go
// Find Treasures where StringVal is set (not nil and not "")
filters := hydraidego.FilterAND(
    hydraidego.FilterString(hydraidego.IsNotEmpty, ""),  // value is ignored
)

// Find Treasures where Int32Val is NOT set (nil)
filters := hydraidego.FilterAND(
    hydraidego.FilterInt32(hydraidego.IsEmpty, 0),  // value is ignored
)

// BytesField: find products where Brand field exists and is non-empty
filters := hydraidego.FilterAND(
    hydraidego.FilterBytesFieldString(hydraidego.IsNotEmpty, "Brand", ""),  // value is ignored
)

// Combine with other filters: price > 100 AND description is not empty
filters := hydraidego.FilterAND(
    hydraidego.FilterFloat64(hydraidego.GreaterThan, 100.0),
    hydraidego.FilterBytesFieldString(hydraidego.IsNotEmpty, "Description", ""),
)
```

##### CatalogReadManyStream — Streaming Read with Filters

```go
// Read all products with price > 100.0, newest first
index := &hydraidego.Index{
    IndexType:  hydraidego.IndexCreationTime,
    IndexOrder: hydraidego.IndexOrderDesc,
}

filters := hydraidego.FilterAND(
    hydraidego.FilterFloat64(hydraidego.GreaterThan, 100.0),
)

err := h.CatalogReadManyStream(ctx, swamp, index, filters, ProductModel{}, func(model any) error {
    product := model.(*ProductModel)
    fmt.Printf("Product: %s, Price: %.2f\n", product.Name, product.Price)
    return nil
})
```

When `filters` is `nil`, all Treasures matching the Index criteria are streamed (equivalent to `CatalogReadMany` but with streaming delivery).

> Full example: [catalog_read_many_stream.go](examples/models/catalog_read_many_stream.go)

##### CatalogReadManyFromMany — Multi-Swamp Streaming Read

Reads from **multiple Swamps** in a single streaming call, with per-swamp Index and Filters.
Results arrive swamp-by-swamp in the request order. The iterator receives the source swamp name.

```go
requests := []*hydraidego.CatalogReadManyFromManyRequest{
    {
        SwampName: name.New().Sanctuary("orders").Realm("eu").Swamp("hu"),
        Index:     &hydraidego.Index{IndexType: hydraidego.IndexKey, IndexOrder: hydraidego.IndexOrderAsc},
        Filters:   hydraidego.FilterAND(hydraidego.FilterString(hydraidego.Equal, "pending")),
    },
    {
        SwampName: name.New().Sanctuary("orders").Realm("eu").Swamp("de"),
        Index:     &hydraidego.Index{IndexType: hydraidego.IndexKey, IndexOrder: hydraidego.IndexOrderAsc},
        Filters:   hydraidego.FilterOR(
            hydraidego.FilterString(hydraidego.Equal, "pending"),
            hydraidego.FilterString(hydraidego.Contains, "ship"),
        ),
    },
}

err := h.CatalogReadManyFromMany(ctx, requests, OrderModel{}, func(swampName name.Name, model any) error {
    order := model.(*OrderModel)
    fmt.Printf("[%s] Order: %s - %s\n", swampName.Get(), order.OrderID, order.Status)
    return nil
})
```

> Full example: [catalog_read_many_from_many.go](examples/models/catalog_read_many_from_many.go)

##### Filtering Inside Complex Types (BytesField Filters)

When a Catalog model contains **struct, slice, map, or pointer fields**, HydrAIDE stores them encoded in the Treasure's `BytesVal`. With **MessagePack encoding** enabled, the server can **reach inside** these complex values and filter on individual fields — including deeply nested structs.

This is one of HydrAIDE's most powerful features: you can store rich, structured data in a Treasure and query specific fields within it, all evaluated on the server.

**How it works:**

Consider a product catalog model with a nested struct:

```go
type Product struct {
    SKU     string  `hydraide:"key"`
    Name    string  `hydraide:"value"`
    Details Details `hydraide:"value"` // stored in BytesVal as MessagePack
}

type Details struct {
    Brand    string
    Color    string
    Weight   float64
    InStock  bool
    Address  Address
}

type Address struct {
    City    string
    Country string
    ZipCode string
}
```

The `Details` struct (including the nested `Address`) is serialized into the Treasure's `BytesVal`. With MessagePack encoding, the server decodes this and navigates to any field using a **dot-separated path**.

**Available BytesField filter constructors:**

| Constructor | Extracts & Compares |
|-------------|---------------------|
| `FilterBytesFieldInt8(op, path, value)` | int8 field at path |
| `FilterBytesFieldInt16(op, path, value)` | int16 field at path |
| `FilterBytesFieldInt32(op, path, value)` | int32 field at path |
| `FilterBytesFieldInt64(op, path, value)` | int64 field at path |
| `FilterBytesFieldUint8(op, path, value)` | uint8 field at path |
| `FilterBytesFieldUint16(op, path, value)` | uint16 field at path |
| `FilterBytesFieldUint32(op, path, value)` | uint32 field at path |
| `FilterBytesFieldUint64(op, path, value)` | uint64 field at path |
| `FilterBytesFieldFloat32(op, path, value)` | float32 field at path |
| `FilterBytesFieldFloat64(op, path, value)` | float64 field at path |
| `FilterBytesFieldString(op, path, value)` | string field at path |
| `FilterBytesFieldBool(op, path, value)` | bool field at path |

All relational operators (`Equal`, `NotEqual`, `GreaterThan`, etc.) and string operators (`Contains`, `StartsWith`, etc.) work with BytesField filters.

**Examples:**

```go
// 1. Simple field filter: find products by brand
filters := hydraidego.FilterAND(
    hydraidego.FilterBytesFieldString(hydraidego.Equal, "Brand", "Apple"),
)

// 2. Nested field filter: find products from Budapest
filters := hydraidego.FilterAND(
    hydraidego.FilterBytesFieldString(hydraidego.Equal, "Address.City", "Budapest"),
)

// 3. Multiple field filters with AND: heavy Apple products
filters := hydraidego.FilterAND(
    hydraidego.FilterBytesFieldString(hydraidego.Equal, "Brand", "Apple"),
    hydraidego.FilterBytesFieldFloat64(hydraidego.GreaterThan, "Weight", 0.5),
)

// 4. String operators on struct fields: brands containing "Sam"
filters := hydraidego.FilterAND(
    hydraidego.FilterBytesFieldString(hydraidego.Contains, "Brand", "Sam"),
)

// 5. AND/OR with BytesField: (Brand == "Apple" OR Brand == "Samsung") AND InStock == true
filters := hydraidego.FilterAND(
    hydraidego.FilterOR(
        hydraidego.FilterBytesFieldString(hydraidego.Equal, "Brand", "Apple"),
        hydraidego.FilterBytesFieldString(hydraidego.Equal, "Brand", "Samsung"),
    ),
    hydraidego.FilterBytesFieldBool(hydraidego.Equal, "InStock", true),
)

// 6. Mixed: primitive filter + BytesField filter
//    price > 100 (Float64Val) AND country starts with "H" (inside BytesVal struct)
filters := hydraidego.FilterAND(
    hydraidego.FilterFloat64(hydraidego.GreaterThan, 100.0),
    hydraidego.FilterBytesFieldString(hydraidego.StartsWith, "Address.Country", "H"),
)

// 7. Deep nesting with OR: products from Budapest OR products with ZipCode ending in "00"
filters := hydraidego.FilterOR(
    hydraidego.FilterBytesFieldString(hydraidego.Equal, "Address.City", "Budapest"),
    hydraidego.FilterBytesFieldString(hydraidego.EndsWith, "Address.ZipCode", "00"),
)
```

**Full streaming example:**

```go
index := &hydraidego.Index{
    IndexType:  hydraidego.IndexCreationTime,
    IndexOrder: hydraidego.IndexOrderDesc,
}

// Find all Apple products from Hungary that are in stock
filters := hydraidego.FilterAND(
    hydraidego.FilterBytesFieldString(hydraidego.Equal, "Brand", "Apple"),
    hydraidego.FilterBytesFieldString(hydraidego.Equal, "Address.Country", "Hungary"),
    hydraidego.FilterBytesFieldBool(hydraidego.Equal, "InStock", true),
)

swamp := name.New().Sanctuary("products").Realm("catalog").Swamp("electronics")

err := h.CatalogReadManyStream(ctx, swamp, index, filters, Product{}, func(model any) error {
    product := model.(*Product)
    fmt.Printf("%s — %s (%s)\n", product.SKU, product.Details.Brand, product.Details.Address.City)
    return nil
})
```

> Full example: [catalog_read_many_stream_bytes_field.go](examples/models/catalog_read_many_stream_bytes_field.go)

**Important:** BytesField filtering requires **MessagePack encoding**. If the Treasure's `BytesVal` uses GOB encoding (the default), the server cannot inspect its contents and the filter returns `false` (no match). See below for how to enable MessagePack.

##### MessagePack Encoding

By default, HydrAIDE uses Go's GOB encoding for complex types (structs, slices, maps, pointers stored in `BytesVal`). To enable BytesField filtering, you must opt into **MessagePack encoding**:

```go
h.RegisterSwamp(ctx, &hydraidego.RegisterSwampRequest{
    SwampPattern: name.New().Sanctuary("products").Realm("catalog").Swamp("*"),
    CloseAfterIdle: time.Hour,
    FilesystemSettings: &hydraidego.SwampFilesystemSettings{
        WriteInterval:  time.Second * 5,
        EncodingFormat: hydraidego.EncodingMsgPack, // Enable MessagePack encoding
    },
})
```

MessagePack is also the recommended encoding for **cross-language compatibility** — unlike GOB, MessagePack is supported in Python, Node.js, Rust, Java, and every other major language.

**Encoding migration** from GOB to MessagePack is automatic:
1. Set `EncodingFormat: hydraidego.EncodingMsgPack` in `RegisterSwamp`
2. Read existing data (auto-detected as GOB)
3. Re-save the data (SDK writes in MessagePack, server detects the byte-level change)
4. Call `h.CompactSwamp(ctx, swampName)` to remove old GOB entries from the `.hyd` file

> Primitive types (int, string, bool, float) are **not affected** by encoding changes. They always use native proto fields directly. Only complex types (structs, slices, maps, pointers) use `BytesVal` with GOB/MessagePack.

##### Timestamp Filtering

HydrAIDE Treasures have three built-in timestamp fields: **CreatedAt**, **UpdatedAt**, and **ExpiredAt**. You can filter on these using dedicated constructors:

| Constructor | Compares Against |
|-------------|-----------------|
| `FilterCreatedAt(op, time.Time)` | Treasure's creation timestamp |
| `FilterUpdatedAt(op, time.Time)` | Treasure's last update timestamp |
| `FilterExpiredAt(op, time.Time)` | Treasure's expiration timestamp |

All relational operators work with timestamps: `Equal`, `NotEqual`, `GreaterThan`, `GreaterThanOrEqual`, `LessThan`, `LessThanOrEqual`. The existence operators `IsEmpty` and `IsNotEmpty` also work (to check if a timestamp is set).

```go
// Find Treasures created in the last 24 hours
cutoff := time.Now().Add(-24 * time.Hour)
filters := hydraidego.FilterAND(
    hydraidego.FilterCreatedAt(hydraidego.GreaterThan, cutoff),
)

// Find expired Treasures (ExpiredAt is before now)
filters := hydraidego.FilterAND(
    hydraidego.FilterExpiredAt(hydraidego.LessThan, time.Now()),
)

// Find Treasures that have never been updated (UpdatedAt is nil)
filters := hydraidego.FilterAND(
    hydraidego.FilterUpdatedAt(hydraidego.IsEmpty, time.Time{}), // value is ignored for IsEmpty
)

// Find Treasures with an expiration date set
filters := hydraidego.FilterAND(
    hydraidego.FilterExpiredAt(hydraidego.IsNotEmpty, time.Time{}), // value is ignored
)

// Combine: created after Jan 1 2024 AND has expiration
filters := hydraidego.FilterAND(
    hydraidego.FilterCreatedAt(hydraidego.GreaterThanOrEqual, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
    hydraidego.FilterExpiredAt(hydraidego.IsNotEmpty, time.Time{}),
)
```

##### Map Key Existence (HasKey / HasNotKey)

When a BytesVal field contains a `map[string]...`, you can check if a specific key exists in the map using the `HasKey` and `HasNotKey` operators with `FilterBytesFieldString`:

```go
// Model with a map field
type UserProfile struct {
    ID       string                 `hydraide:"key"`
    Name     string                 `hydraide:"value"`
    Metadata map[string]interface{} `hydraide:"value"` // stored in BytesVal
}

// Find users whose Metadata map contains the "email" key
filters := hydraidego.FilterAND(
    hydraidego.FilterBytesFieldString(hydraidego.HasKey, "Metadata", "email"),
)

// Find users whose Metadata map does NOT contain "phone"
filters := hydraidego.FilterAND(
    hydraidego.FilterBytesFieldString(hydraidego.HasNotKey, "Metadata", "phone"),
)

// Combine: has email key AND name starts with "J"
filters := hydraidego.FilterAND(
    hydraidego.FilterBytesFieldString(hydraidego.HasKey, "Metadata", "email"),
    hydraidego.FilterString(hydraidego.StartsWith, "J"),
)
```

The `HasKey`/`HasNotKey` operators only work with MessagePack-encoded map fields accessed through `BytesFieldPath`. If the field at the path is not a map, `HasKey` returns `false` and `HasNotKey` returns `true`.

##### Phrase Search (FilterPhrase / FilterNotPhrase)

HydrAIDE supports **phrase search** for full-text search scenarios. This is designed for word-index data stored as `map[string][]int` in a BytesVal field, where:
- **Keys** are words (lowercase, normalized)
- **Values** are sorted position lists indicating where each word appears in a text

The phrase filter checks if specified words appear at **consecutive positions**, forming a phrase.

```go
// Model with a word-index field
type Document struct {
    ID        string            `hydraide:"key"`
    Title     string            `hydraide:"value"`
    WordIndex map[string][]int  `hydraide:"value"` // stored in BytesVal as MessagePack
}

// Example word-index for text "Az általános szerződési feltételek érvényesek."
// WordIndex: {
//   "az":           [0],
//   "altalanos":    [1],
//   "szerzodesi":   [2],
//   "feltetelek":   [3],
//   "ervenyesek":   [4],
// }
```

Use `FilterPhrase` to find documents containing a phrase, and `FilterNotPhrase` to exclude them:

```go
// Find documents containing the phrase "altalanos szerzodesi feltetelek"
filters := hydraidego.FilterAND(
    hydraidego.FilterPhrase("WordIndex", "altalanos", "szerzodesi", "feltetelek"),
)

// Exclude documents containing the phrase "altalanos szerzodesi feltetelek"
filters := hydraidego.FilterAND(
    hydraidego.FilterNotPhrase("WordIndex", "altalanos", "szerzodesi", "feltetelek"),
)

// Combine phrase search with other filters:
// Find recent documents containing the phrase
filters := hydraidego.FilterAND(
    hydraidego.FilterCreatedAt(hydraidego.GreaterThan, time.Now().Add(-7*24*time.Hour)),
    hydraidego.FilterPhrase("WordIndex", "altalanos", "szerzodesi", "feltetelek"),
)

// OR logic: match either phrase
filters := hydraidego.FilterOR(
    hydraidego.FilterPhrase("WordIndex", "altalanos", "szerzodesi"),
    hydraidego.FilterPhrase("WordIndex", "adatvedelmi", "nyilatkozat"),
)
```

**How the algorithm works:**
1. The server decodes the MessagePack BytesVal and extracts the word-index map at `BytesFieldPath`
2. For each word in the phrase, it retrieves the sorted position list from the map
3. If any word is missing from the map, the phrase is not found
4. Starting from each position of the first word, it checks if subsequent words appear at positions `pos+1`, `pos+2`, etc. using binary search
5. `FilterPhrase` matches when consecutive positions are found; `FilterNotPhrase` matches when they are NOT found

> Full example: [catalog_advanced_filters.go](examples/models/catalog_advanced_filters.go)

---

##### Profile Filtering (ForKey / ProfileReadWithFilter)

HydrAIDE's filter system extends to **profile reads**. In profile mode, each struct field is stored as a separate Treasure keyed by field name. Filters use the `ForKey()` method to target specific fields:

```go
type UserProfile struct {
    Name   string `hydraide:"Name"`
    Age    int32  `hydraide:"Age"`
    Status string `hydraide:"Status"`
}

// Build filters targeting specific profile fields
filters := hydraidego.FilterAND(
    hydraidego.FilterInt32(hydraidego.GreaterThan, 18).ForKey("Age"),
    hydraidego.FilterString(hydraidego.Equal, "active").ForKey("Status"),
)

// Read a single profile — returns (true, nil) if it matches
user := &UserProfile{}
matched, err := h.ProfileReadWithFilter(ctx, swampName, filters, user)
if err != nil {
    log.Fatal(err)
}
if matched {
    fmt.Printf("User: %s, Age: %d\n", user.Name, user.Age)
}
```

`ForKey()` works with all filter types:

```go
// Primitive filters
hydraidego.FilterInt32(hydraidego.GreaterThan, 25).ForKey("Age")
hydraidego.FilterString(hydraidego.Contains, "admin").ForKey("Role")

// Timestamp filters
hydraidego.FilterCreatedAt(hydraidego.GreaterThan, cutoff).ForKey("LastLogin")

// BytesField filters (for complex types stored as MessagePack)
hydraidego.FilterBytesFieldString(hydraidego.HasKey, "Settings", "email").ForKey("Metadata")

// Phrase filters
hydraidego.FilterPhrase("WordIndex", "hello", "world").ForKey("Content")
```

---

##### Multi-Profile Streaming (ProfileReadBatchWithFilter)

Read from **multiple profile swamps** with shared filters, streaming matching profiles back:

```go
swampNames := []name.Name{
    name.New().Sanctuary("users").Realm("profiles").Swamp("alice"),
    name.New().Sanctuary("users").Realm("profiles").Swamp("bob"),
    name.New().Sanctuary("users").Realm("profiles").Swamp("charlie"),
    // ... hundreds more
}

filters := hydraidego.FilterAND(
    hydraidego.FilterInt32(hydraidego.GreaterThan, 18).ForKey("Age"),
    hydraidego.FilterString(hydraidego.Equal, "active").ForKey("Status"),
)

var results []*UserProfile
err := h.ProfileReadBatchWithFilter(ctx, swampNames, filters, &UserProfile{}, 0,
    func(swampName name.Name, model any, err error) error {
        if err != nil {
            return nil // skip errors, continue
        }
        user := model.(*UserProfile)
        results = append(results, user)
        return nil
    })
```

The `maxResults` parameter (4th argument, `0` = unlimited) limits the total number of matching profiles streamed back. The stream stops after `maxResults` matches.

Profiles are grouped by server automatically for efficient network usage (same pattern as `CatalogReadManyFromMany`).

---

##### MaxResults — Post-Filter Limit for Streaming

All streaming reads support `MaxResults` — a post-filter limit that stops the stream after N matches:

```go
// Catalog: stop after first 10 matches
index := &hydraidego.Index{
    IndexType:  hydraidego.IndexCreationTime,
    IndexOrder: hydraidego.IndexOrderDesc,
    MaxResults: 10, // stop streaming after 10 matches
}

var results []*Product
err := h.CatalogReadManyStream(ctx, swamp, index, filters, Product{}, func(model any) error {
    results = append(results, model.(*Product))
    return nil
})
// results will contain at most 10 items
```

**MaxResults vs Limit:**

| Field | Level | Behavior |
|-------|-------|----------|
| `Limit` | Pre-filter | Engine fetches at most N candidates from the index |
| `MaxResults` | Post-filter | Stream stops after N candidates pass the filters |

Use `Limit` to bound the search space, and `MaxResults` to bound the result set.

For `CatalogReadManyFromMany`, `MaxResults` works per-swamp (each swamp's query has its own limit via `Index.MaxResults`).

For `ProfileReadBatchWithFilter`, `maxResults` is a global limit across all profiles.

> Full example: [profile_advanced_filters.go](examples/models/profile_advanced_filters.go)

---

#### 🔎 Batch key-based shift — CatalogShiftBatch

CatalogShiftBatch is designed for scenarios where you need to **retrieve and delete** multiple Treasures by their keys in a single atomic operation. This is particularly useful for job queue processing, message consumption, shopping cart checkout, and other consume-and-remove workflows.

**What makes it different from CatalogReadBatch?**

- CatalogReadBatch **reads** Treasures without deleting them
- CatalogShiftBatch **reads AND deletes** Treasures in one atomic operation
- Each Treasure is cloned before deletion, ensuring you get the full data
- Original Treasures are permanently removed from the Swamp (not shadow delete)

**Key characteristics:**

- ⚡ **Atomic operation**: Each treasure is locked, cloned, and deleted safely
- 🔒 **Thread-safe**: Treasure-level locks prevent concurrent access issues
- 🗑️ **Permanent deletion**: Removed treasures cannot be recovered
- 🔕 **Event notifications**: All swamp subscribers receive deletion events
- 🎯 **Selective**: Only specified keys are affected
- 🚫 **Graceful**: Missing keys are silently ignored (no error)

**Requirements:**

- Iterator must not be nil
- Model must be a non-pointer type (the SDK internally creates new instances per record)
- Keys can be an empty slice (returns immediately with no error)

**Quick example — Job Queue Processing:**

```go
// Define your job model
type CatalogModelJob struct {
    JobID      string    `hydraide:"key"`
    Payload    string    `hydraide:"value"`
    Priority   int       `hydraide:"priority"`
    CreatedBy  string    `hydraide:"createdBy"`
    CreatedAt  time.Time `hydraide:"createdAt"`
}

// Fetch and consume jobs from the queue
func ProcessJobBatch(r repo.Repo, jobIDs []string) error {
    ctx, cancel := hydraidehelper.CreateHydraContext()
    defer cancel()

    h := r.GetHydraidego()
    swamp := name.New().Sanctuary("jobs").Realm("catalog").Swamp("pending")

    // Shift (clone and delete) jobs atomically
    return h.CatalogShiftBatch(ctx, swamp, jobIDs, CatalogModelJob{}, func(m any) error {
        job := m.(*CatalogModelJob)
        
        // Process the job (it's already deleted from the queue)
        if err := executeJob(job); err != nil {
            log.Printf("Job %s failed: %v", job.JobID, err)
            // Job is already deleted, handle failure appropriately
            return err
        }
        
        log.Printf("✅ Job %s completed", job.JobID)
        return nil
    })
}
```

**Use cases:**

- 📦 **Job queue workers**: Fetch jobs and acknowledge (delete) them atomically
- 🛒 **Shopping cart checkout**: Retrieve cart items and remove them in one call
- 📨 **Message queue consumers**: Read and acknowledge messages
- 🗃️ **Batch cleanup**: Extract items for archival before deletion
- ⚙️ **Task processing**: Claim and remove tasks without race conditions

**Comparison with other batch operations:**

| Operation             | Reads Data | Deletes Data | Use Case |
|-----------------------|------------|--------------|----------|
| `CatalogReadBatch`    | ✅ Yes     | ❌ No        | Fetch multiple records by keys |
| `CatalogShiftBatch`   | ✅ Yes     | ✅ Yes       | Consume and remove records atomically |
| `CatalogShiftExpired` | ✅ Yes     | ✅ Yes       | Process expired items by TTL |
| `CatalogDeleteMany`   | ❌ No      | ✅ Yes       | Delete without reading data |

**Performance benefits:**

- 30-50× faster than individual read+delete operations in a loop
- Single gRPC call instead of N network roundtrips
- No risk of partial deletion (each treasure operation is atomic)
- No double-processing in concurrent environments

**Important notes:**

- ⚠️ **Destructive operation**: Deleted Treasures cannot be recovered
- ⚠️ **Permanent deletion**: This is not a shadow delete
- 📢 **Event stream**: Subscribers will be notified of deletions
- 🎯 **Order not guaranteed**: Results may come back in any order

> Full example with edge cases and best practices: [catalog_shift_batch.go](examples/models/catalog_shift_batch.go)

---

### 📚 Good to Know: Split Catalogs When Needed

While Catalog Swamps are highly scalable, **putting too many entries into a single Swamp** can reduce performance 
— especially for real-time filtering, event subscriptions, and storage efficiency.

To keep things fast and clean:

> 📦 **Segment large catalogs into multiple logical Swamps**, based on a meaningful key like prefix, user, region, or time window.

#### 🧩 Practical Sharding Strategies

| Use Case                | Strategy                         | Swamp Pattern Example                           | Why it Helps                                       |
| ----------------------- | -------------------------------- |-------------------------------------------------| -------------------------------------------------- |
| 🌍 Millions of tags     | Split by first letter            | `tags/catalog/a`, `tags/catalog/b`, ...         | Limits Swamp size; enables faster reads and writes |
| 👥 User session logs    | Split by user ID + month         | `sessions/<userID>/<YYYYMM>`                    | Natural time + user partition; simplifies cleanup  |
| 📈 Logs or events       | Split by time or service         | `logs/api/202507`, `logs/db/202507`             | Enables stream isolation and easier archiving      |
| 🏷️ Document references | Use tag as Swamp name            | `tags/references/ai`, `tags/references/go`      | Natural many-to-many model; easy reverse lookup    |
| 🧠 Search term tracking | Split by language or word length | `search/terms-en/short`, `search/terms-fr/long` | Reduces per-Swamp memory; isolates data logically  |

#### 💡 Design Tip

When deciding on a segmentation scheme, ask:

* 🔸 *Would I ever need to read or stream all entries at once?*
  → If not, you can safely split into smaller Swamps.

* 🔸 *Is my query logic scoped to a subset (e.g. one user, one month)?*
  → Then use that scope in your Swamp name!

* 🔸 *Will this Catalog grow indefinitely (e.g. logs, metrics)?*
  → Use time-based sharding: monthly or weekly Swamps make cleanup easier.

---

#### 📇 Shard Index Catalog: Track Your Shards

If you decide to segment a Catalog into multiple Swamps (e.g., by letter, user ID, or month), it's **often helpful to maintain a *central Catalog* that tracks all used shard keys**.

> This way, you always know what Swamps exist — even if they were created dynamically.

##### 📌 Example: Tag Shard Index

Suppose you split your tag Catalog by starting letter:

* `tags/catalog/a`
* `tags/catalog/b`
* …
* `tags/catalog/z`

You can maintain a separate Swamp like:

```go
// CatalogModelTagShardIndex represents a known shard for tags.
type CatalogModelTagShardIndex struct {
	Letter string `hydraide:"key"` // e.g. "a", "b", "c", ...
}
```

Then store entries in a central index Swamp:

```
/tags/shard-index/main
```

This Catalog tells your app which Swamps are known and can be iterated for:

* 🧭 Admin panels that list all existing shards
* ⚙️ Cron jobs that clean or export each Swamp
* 📊 Dashboards that show per-shard stats

##### 📌 Example: Session Logs by Month

For time-based segmentation, you can store a tracker like:

```go
// CatalogModelMonthShardIndex tracks known months used for session storage.
type CatalogModelMonthShardIndex struct {
	YearMonth string `hydraide:"key"` // Format: "2025-07", "2025-08"
}
```

Stored under:

```
/sessions/shard-index/by-month
```

This allows you to:

* Show a list of available months
* Stream sessions per month
* Archive or purge old data confidently

#### 🧠 Why This Matters

Keeping a central index of used shards gives you:

* 🔍 Discoverability: You don’t have to scan disk or guess swamp names
* 🛠️ Automation: Background jobs can iterate shards easily
* 💡 Analytics: You can measure growth per shard
* ✅ Reliability: Safer to purge or process known Swamps

---

#### 🧯 When Not to Use Catalogs

Catalogs are not suitable when:

* You only want to store *a single record per Swamp* → use **Profiles** instead
* You need to increment or patch partial values → use custom logic or ProfileMerge
* You want full relational joins — HydrAIDE is NoSQL by design

📂 **SDK Example Files and documentation**:

| Function                  | SDK Status | Example Go Models and Docs |
|---------------------------| ------- |----------------------------|
| CatalogCreate             | ✅ Ready | [catalog_create.go](examples/models/catalog_create.go)             |
| CatalogCreateMany         | ✅ Ready | [catalog_create_many.go](examples/models/catalog_create_many.go)             |
| CatalogCreateManyToMany   | ✅ Ready | [catalog_create_many_to_many.go](examples/models/catalog_create_many_to_many.go)             |
| CatalogRead               | ✅ Ready | [catalog_read.go](examples/models/catalog_read.go)              |
| CatalogReadMany           | ✅ Ready | [catalog_read_many.go](examples/models/catalog_read_many.go)            |
| CatalogReadBatch          | ✅ Ready | [catalog_read_batch.go](examples/models/catalog_read_batch.go)            |
| CatalogUpdate             | ✅ Ready | [catalog_update.go](examples/models/catalog_update.go)              |
| CatalogUpdateMany         | ✅ Ready | [catalog_update_many.go](examples/models/catalog_update_many.go)              |
| CatalogDelete             | ✅ Ready | [catalog_delete.go](examples/models/catalog_delete.go)              |
| CatalogDeleteMany         | ✅ Ready | [catalog_delete.go](examples/models/catalog_delete.go)              |
| CatalogDeleteManyFromMany | ✅ Ready | [catalog_delete_many_from_many.go](examples/models/catalog_delete_many_from_many.go)            |
| CatalogSave               | ✅ Ready | [catalog_save.go](examples/models/catalog_save.go)             |
| CatalogSaveMany           | ✅ Ready | [catalog_save_many.go](examples/models/catalog_save_many.go)             |
| CatalogSaveManyToMany     | ✅ Ready | [catalog_save_many_to_many.go](examples/models/catalog_save_many_to_many.go)             |
| CatalogReadManyStream     | ✅ Ready | [catalog_read_many_stream.go](examples/models/catalog_read_many_stream.go)            |
| CatalogReadManyFromMany   | ✅ Ready | [catalog_read_many_from_many.go](examples/models/catalog_read_many_from_many.go)            |
| CompactSwamp              | ✅ Ready | Encoding migration helper — forces .hyd file rewrite              |
| CatalogShiftExpired       | ✅ Ready | [catalog_shift_expired.go](examples/models/catalog_shift_expired.go)              |
| CatalogShiftBatch         | ✅ Ready | [catalog_shift_batch.go](examples/models/catalog_shift_batch.go)              |

---

### ➕ Increment / Decrement – Atomic State With Metadata Control

HydrAIDE’s `Increment*` family of functions enables **atomic, type-safe updates** of numeric values — without reading, locking, or manually overwriting state.

Whether you're updating:

* a user's **rate limit**,
* a device's **event count**,
* a game **score**,
* a financial **balance**,
* or a processing **threshold**,

…you can do it with **one intent-first operation** — optionally guarded by conditions like *“only increment if current value < 100”* **and** optionally applying lifecycle/audit metadata in the same atomic step.

#### 🧠 Why this is a game-changer:

* ⚡ **Atomic execution** — no race conditions, no read-modify-write logic
* 🔒 **Treasure-level locking only** — never blocks the entire Swamp
* 🧬 **Strongly typed** — choose from `int8`, `uint32`, `float64`, etc.
* ✅ **Condition-aware** — support for rich comparisons: `Equal`, `NotEqual`, `GreaterThan`, `LessThanOrEqual`, etc.
* 🏷️ **Metadata control** — set `CreatedAt`, `CreatedBy`, `UpdatedAt`, `UpdatedBy`, or `ExpiredAt` depending on whether the Treasure is created or updated

> This isn’t just math — it’s **concurrent state mutation**, encoded as intention, with audit and TTL control built-in.

#### 📌 One demo to rule them all

All `Increment*` functions work the same way — only the type changes.

To see a complete example in action (including conditional logic, metadata usage, and memory-only Swamps), check out:

👉 [Catalog Model Rate Limit Counter](examples/models/increment.go)

This single model demonstrates how to:

* atomically update a counter,
* guard the operation with a relational condition,
* set metadata fields differently for creation vs. update,
* scale to thousands of users with no locks or I/O,
* and reset the state via `Destroy()` or ExpiredAt.

It applies to **all numeric increment types**, from `int8` to `float64`.

#### 💡 Metadata parameters

Each `Increment*` function now supports two optional metadata descriptors:

* `setIfNotExist` — applied if the Treasure must be created
* `setIfExist` — applied if the Treasure already exists

Example fields in `IncrementMetaRequest`:

* `SetCreatedAt` — set creation timestamp automatically
* `SetCreatedBy` — set creator ID
* `SetUpdatedAt` — set update timestamp automatically
* `SetUpdatedBy` — set updater ID
* `ExpiredAt` — set absolute expiration timestamp

This lets you control creation/update auditing and TTL in the same atomic call.

### Available Increment Functions

| Function         | SDK Status | Example Demo Model                  |
| ---------------- | ---------- | ----------------------------------- |
| IncrementInt8    | ✅ Ready    | ✅ `RateLimitCounter` (shared logic) |
| IncrementInt16   | ✅ Ready    | ✅ `RateLimitCounter` (shared logic) |
| IncrementInt32   | ✅ Ready    | ✅ `RateLimitCounter` (shared logic) |
| IncrementInt64   | ✅ Ready    | ✅ `RateLimitCounter` (shared logic) |
| IncrementUint8   | ✅ Ready    | ✅ `RateLimitCounter` (demonstrated) |
| IncrementUint16  | ✅ Ready    | ✅ `RateLimitCounter` (shared logic) |
| IncrementUint32  | ✅ Ready    | ✅ `RateLimitCounter` (shared logic) |
| IncrementUint64  | ✅ Ready    | ✅ `RateLimitCounter` (shared logic) |
| IncrementFloat32 | ✅ Ready    | ✅ `RateLimitCounter` (shared logic) |
| IncrementFloat64 | ✅ Ready    | ✅ `RateLimitCounter` (shared logic) |

> 💡 Only the numeric type changes — the logic stays the same. The same metadata and condition patterns apply to all variants.

---

### 📌 Slice & Reverse Indexing in HydrAIDE

HydrAIDE provides native support for atomic operations on `[]uint32` slices within Swamps — enabling highly efficient and scalable **reverse indexing**.

#### 🧠 What is a Reverse Index?

A reverse index is a structure that **maps from a value back to its references**.

Instead of storing “Product X was viewed by User A” as a one-way event, we store:

```text
→ Product X → [UserA, UserB, UserC]
→ Product Y → [UserA, UserF]
```

This allows you to instantly answer questions like:

* *Who interacted with this product?*
* *Which users engaged with this tag?*
* *How many users are linked to this entity?*

Reverse indexes are especially powerful when:

* There are **many-to-many** relationships
* You need **fast set membership** or **frequency analytics**
* You want to **avoid full scans** of large datasets

#### 🗂️ Reverse Index in a Catalog Context

In HydrAIDE, reverse indexes are stored in **Catalog Swamps**, and the key-value logic looks like this:

```
Swamp name:     tags/products/<tag>
Treasure key:   product-ID
Treasure value: []uint32 (user IDs)
```

This gives you:

* One Swamp per tag
* One Treasure per product
* One slice per product: listing all user IDs who interacted

#### 🧰 Available Functions (all in one model-based demo)

HydrAIDE offers atomic, in-place operations for managing `[]uint32` values under each key.
These are implemented **in a single documented Go model**, not as separate files.

| Function Name             | Description                                                               |
| ------------------------- | ------------------------------------------------------------------------- |
| `Uint32SlicePush`         | Adds unique values to a slice (append-only, deduplicated)                 |
| `Uint32SliceDelete`       | Removes values from a slice (with auto-GC for empty Treasures and Swamps) |
| `Uint32SliceSize`         | Returns the number of elements in the slice (slice length)                |
| `Uint32SliceIsValueExist` | Checks whether a specific value exists in a slice                         |

All of these are demonstrated in the [ModelTagProductViewers](examples/models/slice_and_reverse_index.go) Go model, which shows how to:

* Index users under tagged products
* Query reverse relationships
* Manage slice contents atomically and efficiently

#### 🚀 Why it matters

This slice-based reverse indexing system gives you:

* **High performance** (no reads needed for write)
* **Thread safety** (atomic updates)
* **Real-time cleanup** (empty slices and Swamps vanish instantly)
* **Minimal storage footprint** (no duplication, no bloat)

Whether you're building recommender systems, behavioral logs, or tag-driven interactions,
this is HydrAIDE's way of giving you **database-native reverse sets**, without the overhead of external joins or slow full scans.

---

## 🧪 Testing with Real Database Connection

One of HydrAIDE's unique advantages is that you can **test your code with a real database connection** instead of relying on mocks. Since HydrAIDE is extremely lightweight and resource-efficient, maintaining a dedicated test instance is both practical and cost-effective.

### Why Test with a Real Database?

- ✅ **No mocking complexity** – Test against actual data operations
- ✅ **Real-world accuracy** – Catch issues that mocks might miss
- ✅ **Fast execution** – HydrAIDE's performance keeps tests quick
- ✅ **Simple setup** – Just connect to your test instance
- ✅ **Production parity** – Tests reflect real behavior

### Critical Best Practices

⚠️ **Always Register Patterns**: Never forget to call `RegisterPattern()` in your setup! Without it, swamp operations will fail.

⚠️ **Always Clean Up**: In your test suite's teardown, call the `Destroy()` method to remove test-created swamps. This ensures:

- Tests can be re-run without ID conflicts or stale data
- Your test database stays clean between runs
- Consistent test results every time

🔒 **Never Hardcode Credentials**: Always load connection details from environment variables, never commit sensitive data to your repository.

### Quick Example

```go
type ProductTestSuite struct {
	suite.Suite
	repoInterface repo.Repo
}

func (s *ProductTestSuite) SetupSuite() {
	// Connect to your dedicated test HydrAIDE instance
	// Load credentials from environment variables (never hardcode!)
	testHost := os.Getenv("HYDRAIDE_TEST_HOST")
	if testHost == "" {
		testHost = "localhost:50051" // Safe default for local dev
	}
	
	s.repoInterface = repo.New([]*client.Server{
		{
			Host:          testHost,
			FromIsland:    0,
			ToIsland:      99,
			CACrtPath:     os.Getenv("HYDRAIDE_TEST_CA_CRT"),
			ClientCrtPath: os.Getenv("HYDRAIDE_TEST_CLIENT_CRT"),
			ClientKeyPath: os.Getenv("HYDRAIDE_TEST_CLIENT_KEY"),
		},
	}, 100, 4194304, false)

	// CRITICAL: Register the pattern before any operations
	model := &Product{}
	err := model.RegisterPattern(s.repoInterface)
	if err != nil {
		s.T().Fatalf("Failed to register pattern: %v", err)
	}
	
	_ = model.Destroy(s.repoInterface) // Clean any leftover data
}

func (s *ProductTestSuite) TearDownSuite() {
	// CRITICAL: Clean up all test swamps
	model := &Product{}
	_ = model.Destroy(s.repoInterface)
}

func (s *ProductTestSuite) TestProductCRUD() {
	product := &Product{
		ProductID: uuid.New().String(),
		Name:      "Test Product",
		Price:     99.99,
	}

	// Real database operations
	err := product.Save(s.repoInterface)
	assert.Nil(s.T(), err)

	loaded := &Product{ProductID: product.ProductID}
	err = loaded.Load(s.repoInterface)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), product.Name, loaded.Name)
}
```

### Testing Beyond Models: Service Layer Too

This approach isn't just for models! You can easily test your **entire service layer** with real database operations:

- ✅ **Business logic** – Test complex workflows with actual state
- ✅ **Subscriptions** – Test reactive updates and event streams in real-time
- ✅ **Integration tests** – Test how services interact with shared data
- ✅ **Real-time features** – Test WebSocket handlers, notifications, etc.

This means you can test subscription logic, reactive patterns, and event-driven architectures without complex mocking infrastructure.

### Complete Testing Guide

For comprehensive examples including:

- **Test suite structure** with setup/teardown
- **RegisterPattern() requirements** and proper initialization
- **Profile swamp testing** (CRUD operations)
- **Catalog swamp testing** (expiration handling)
- **Service layer testing** with subscriptions and reactive logic
- **Bulk operations** and cleanup patterns
- **Security best practices** with environment variables
- **Complete configuration examples**

See the full guide here:

📖 [Testing with Real Database Connection Guide](examples/models/testing_with_real_database.md)

This approach eliminates the need for complex mocking while giving you confidence that your code works correctly in production-like conditions.
