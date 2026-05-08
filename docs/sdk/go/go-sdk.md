# 🐹 HydrAIDE SDK – Go Edition

Welcome to the official **HydrAIDE SDK for Go**, your gateway to building intelligent,
distributed, real-time systems using the HydrAIDE engine.

This SDK provides programmatic access to HydrAIDE's powerful features such as swamp-based data structures,
lock-free operations, real-time subscriptions, and stateless routing, all tailored to Go developers.

## 📦 Install

```bash
go get github.com/hydraide/hydraide/sdk/go/hydraidego/v3@latest
```

Pinned version: `go get github.com/hydraide/hydraide/sdk/go/hydraidego/v3@v3.0.1`. Upgrade with `go get -u`. Full instructions, version compatibility, and troubleshooting in [`install.md`](install.md). For HydrAIDE contributors working on the SDK itself, see [`contributor-setup.md`](contributor-setup.md).

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
12. [🧬 Field-Level Patches – Type-Preserving Structural Mutations](#-field-level-patches--type-preserving-structural-mutations)
13. [📌 Slice & Reverse Indexing in HydrAIDE](#-slice--reverse-indexing-in-hydraide)
14. [🧪 Testing with Real Database Connection](#-testing-with-real-database-connection)

---

## 🔌 Connect to the HydrAIDE Server Using the SDK

The first and most essential step is establishing a connection to the HydrAIDE server using the Go SDK.

To do this, implement the `repo` package. This package is typically placed under `/utils/repo` and should be 
initialized during your application's startup sequence.

You can find the repo implementation and usage examples here:

📁 [`repo.go`](../../../sdk/go/hydraidego/utils/repo/repo.go)

### How to Start Your Server Using the Repo Package

For a complete working example of how to initialize and run your service using the `repo` package, take a look at the demo application:

▶️ [`01-quickstart`](examples/01-quickstart/) — a minimal end-to-end example of SDK setup and Swamp registration with a queue service

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

📄 [example](examples/)

**Catalogs**, on the other hand, are key–value Swamps where you can store many unique keys — each mapped to its own custom value.
This is ideal for scenarios like tracking all registered user IDs, counting how many users exist in total, or displaying a list of users in an admin dashboard.

📄 [example](examples/)

In both cases, data is defined using regular Go `struct`s decorated with HydrAIDE field tags.
You work with the data through model-bound methods that make saving, querying, or subscribing extremely simple and intuitive.

Throughout this SDK documentation (except for the Heartbeat example), all model samples are either Profile-based or Catalog-based, reflecting real production patterns.

> 💬 If anything is unclear or if you'd like to request improvements or clarification in the examples, feel free to open a **Docs Improvement issue**. We'd love your feedback.


### 🧠 System

| Function  | SDK Status | Example Go Models and Docs                                  |
| --------- | ------- |-------------------------------------------------------------|
| Heartbeat | ✅ Ready | [example](examples/)  |

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
| Lock     | ✅ Ready | [example](examples/) |
| Unlock   | ✅ Ready | [example](examples/) |

---

### 🌿 Swamp & Treasure

These functions manage the lifecycle and existence of Swamps (data containers) and their Treasures (records),
including registration, validation, destruction, and real-time subscriptions.

| Function        | SDK Status | Example Go Models and Docs                                               |
| --------------- | ---------- |--------------------------------------------------------------------------|
| RegisterSwamp   | ✅ Ready | [example](examples/)     |
| DeRegisterSwamp | ✅ Ready | [example](examples/) |
| IsSwampExist    | ✅ Ready | [example](examples/)     |
| IsKeyExists     | ✅ Ready | [example](examples/)         |
| AreKeysExist    | ✅ Ready | [example](examples/)     |
| Count           | ✅ Ready | [example](examples/)                       |
| Destroy         | ✅ Ready | [example](examples/)                   |
| DestroyBulk     | ✅ Ready | Bulk destroy multiple swamps via bidirectional streaming                  |
| Subscribe       | ✅ Ready | [example](examples/)               |

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
| `Profile Save, Read, Destroy` | ✅ Ready    | [example](examples/)   |
| `Profile Read Batch` | ✅ Ready    | [example](examples/)   |
| `Profile Save Batch` | ✅ Ready    | [example](examples/)   |

🧪 **Looking for a complete production-ready model?**
Check out [example](examples/) — a real-world example with nested structs, 
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

📖 **Full examples and best practices:** [example](examples/)

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

📖 **Full examples and best practices:** [example](examples/)

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

> Full example: [example](examples/)

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

> Full example: [example](examples/)

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

> Full example: [example](examples/)

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
| `FilterBytesFieldTime(op, path, time.Time)` | time.Time field (stored as int64 Unix seconds) |
| `FilterBytesFieldStringIn(path, values...)` | string field ∈ set of values |
| `FilterBytesFieldInt32In(path, values...)` | int32 field ∈ set of values |
| `FilterBytesFieldInt64In(path, values...)` | int64 field ∈ set of values |

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

> Full example: [example](examples/)

##### IN Filters — Set Membership

Check if a field value belongs to a set of allowed values:

```go
// String IN: match any of the active campaigns
hydraidego.FilterBytesFieldStringIn("CampaignID", "camp-1", "camp-2", "camp-3")

// Int32 IN: Status is Active(1) or Finished(3)
hydraidego.FilterBytesFieldInt32In("Status", 1, 3)

// Int64 IN: match specific timestamps
hydraidego.FilterBytesFieldInt64In("ScheduledAt", 1712534400, 1712620800)
```

##### Time Convenience Filter

`time.Time` fields stored as int64 Unix seconds can be filtered with `FilterBytesFieldTime`:

```go
hydraidego.FilterBytesFieldTime(hydraidego.LessThanOrEqual, "NextSendAt", time.Now())
hydraidego.FilterBytesFieldTime(hydraidego.GreaterThan, "NextSendAt", time.Time{}) // exclude zero
```

##### Nested Slice Where — Multi-Condition Element Matching

Unlike `NestedSliceAny` (one condition per filter), `FilterNestedSliceWhere` guarantees that the **same element** satisfies ALL conditions:

```go
// Find domains with at least ONE CampaignEntry that is Active AND in our campaigns AND ready
filters := hydraidego.FilterAND(
    hydraidego.FilterNestedSliceWhere("CampaignEntries",
        hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 1),
        hydraidego.FilterBytesFieldStringIn("CampaignID", activeCampaignIDs...),
        hydraidego.FilterBytesFieldTime(hydraidego.LessThanOrEqual, "NextSendAt", time.Now()),
    ),
)
```

Four modes: `FilterNestedSliceWhere` (any element), `FilterNestedSliceAll` (every element), `FilterNestedSliceNone` (no element), `FilterNestedSliceCount` (count + compare).

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

> Full example: [example](examples/)

---

##### Vector Similarity Search (FilterVector)

HydrAIDE supports **server-side cosine similarity filtering** for embedding vectors stored in MessagePack-encoded BytesVal fields. This enables semantic search directly inside HydrAIDE — no external vector database needed.

Vectors are stored as `[]float32` fields in your struct and must be **L2-normalized** (unit length) before saving. Use `NormalizeVector()` to normalize:

```go
type DomainPayload struct {
    Category  string    `msgpack:"Category"`
    Language  string    `msgpack:"Language"`
    Embedding []float32 `msgpack:"Embedding"` // 384-dim normalized vector
}

// Before saving: normalize the embedding vector
payload.Embedding = hydraidego.NormalizeVector(rawEmbedding)
```

Use `FilterVector` to find Treasures whose vector field has cosine similarity >= a threshold with your query vector:

```go
// Generate and normalize the query vector (e.g. from an embedding model)
queryVector := hydraidego.NormalizeVector(generateEmbedding("wellness hotel with pool"))

// Combine pre-filters with vector similarity search
filters := hydraidego.FilterAND(
    hydraidego.FilterBytesFieldString(hydraidego.Equal, "Category", "business"),
    hydraidego.FilterBytesFieldString(hydraidego.Equal, "Language", "hu"),
    hydraidego.FilterVector("Embedding", queryVector, 0.70), // min similarity 0.70
)

err := h.CatalogReadManyStream(ctx, swampName, &hydraidego.Index{
    IndexType:  hydraidego.IndexKey,
    IndexOrder: hydraidego.IndexOrderAsc,
}, filters, &DomainPayload{}, func(model any) error {
    domain := model.(*DomainPayload)
    fmt.Printf("Match: %s\n", domain.Category)
    return nil
})
```

**How it works:**
1. The server decodes the MessagePack BytesVal and extracts the `[]float32` vector at `BytesFieldPath`
2. Computes the dot product between the stored vector and the query vector (dot product = cosine similarity for normalized vectors)
3. Returns true if the similarity score >= `MinSimilarity`

**Key points:**
- Both stored vectors and query vectors **must be L2-normalized** — use `NormalizeVector()` before saving and before searching
- `FilterVector` works with any dimension (384, 768, etc.) — stored and query vectors must have the same dimension
- Combinable with any other filter in `FilterAND` / `FilterOR` — pre-filter by category, language, etc. to narrow the search space before vector comparison
- Works in both **catalog mode** and **profile mode** (with `.ForKey()`)
- Typical similarity thresholds: `0.70` (similar), `0.80` (very similar), `0.90` (near-identical)

**Performance:** Dot product on 384-dim vectors takes ~150ns. Scanning 100K pre-filtered vectors completes in ~15ms.

**Utility functions:**

```go
// Normalize a vector to unit length (required before storing and searching)
normalized := hydraidego.NormalizeVector(rawVector)

// Compute cosine similarity between two vectors (for client-side validation/testing)
score := hydraidego.CosineSimilarity(vectorA, vectorB)
```

---

##### Geographic Distance Search (GeoDistance)

HydrAIDE supports **server-side geographic distance filtering** using the Haversine formula. This enables location-based queries directly inside HydrAIDE — no external GIS database needed.

Coordinates are stored as `float64` latitude/longitude fields (WGS84 decimal degrees) in your MessagePack-encoded BytesVal struct:

```go
type DomainPayload struct {
    Category     string  `msgpack:"Category"`
    GeoLatitude  float64 `msgpack:"geo_latitude"`
    GeoLongitude float64 `msgpack:"geo_longitude"`
}
```

Use `GeoDistance` to find Treasures within or beyond a specified radius from a reference point:

```go
// Find all domains within 50 km of Budapest
filters := hydraidego.FilterAND(
    hydraidego.GeoDistance("geo_latitude", "geo_longitude", 47.4979, 19.0402, 50.0, hydraidego.GeoInside),
)

// Find all domains MORE than 200 km from Budapest
filters := hydraidego.FilterAND(
    hydraidego.GeoDistance("geo_latitude", "geo_longitude", 47.4979, 19.0402, 200.0, hydraidego.GeoOutside),
)

// Band filter: 50–150 km from Budapest (combine two GeoDistance filters)
filters := hydraidego.FilterAND(
    hydraidego.GeoDistance("geo_latitude", "geo_longitude", 47.4979, 19.0402, 50.0, hydraidego.GeoOutside),
    hydraidego.GeoDistance("geo_latitude", "geo_longitude", 47.4979, 19.0402, 150.0, hydraidego.GeoInside),
)

// Combine with other filters: category + geo
filters := hydraidego.FilterAND(
    hydraidego.FilterBytesFieldString(hydraidego.Equal, "Category", "business"),
    hydraidego.GeoDistance("geo_latitude", "geo_longitude", 47.4979, 19.0402, 50.0, hydraidego.GeoInside),
)

err := h.CatalogReadManyStream(ctx, swampName, &hydraidego.Index{
    IndexType:  hydraidego.IndexKey,
    IndexOrder: hydraidego.IndexOrderAsc,
}, filters, DomainPayload{}, func(model any) error {
    domain := model.(*DomainPayload)
    fmt.Printf("Nearby: %s (%.4f, %.4f)\n", domain.Category, domain.GeoLatitude, domain.GeoLongitude)
    return nil
})
```

**How it works:**
1. The server decodes the MessagePack BytesVal and extracts the latitude/longitude `float64` fields at the given paths
2. A **bounding box pre-filter** quickly eliminates obviously out-of-range points using simple float comparisons (~2ns)
3. For points inside the bounding box, the **Haversine formula** computes the exact great-circle distance (~86ns)
4. `GeoInside` matches when distance <= radius; `GeoOutside` matches when distance > radius

**Key points:**
- Coordinates must be **WGS84 decimal degrees** (standard GPS format used by Google Maps, OpenStreetMap, etc.)
- Records with `lat == 0 AND lng == 0` (**Null Island**) are automatically excluded — this handles missing coordinate data gracefully
- Supports **dot-separated field paths** for nested structs (e.g., `"Location.Lat"`, `"Location.Lng"`)
- Combinable with any other filter in `FilterAND` / `FilterOR`
- Works in both **catalog mode** and **profile mode** (with `.ForKey()`)
- The Haversine formula accuracy is ~0.5% of real distance — sufficient for most use cases

**Performance:** Bounding box check: ~2ns. Haversine computation: ~86ns. Full filter pipeline (including MessagePack decode): ~1.7µs per Treasure.

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

// Vector similarity filters
hydraidego.FilterVector("Embedding", queryVec, 0.70).ForKey("MainProfile")
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

> Runnable example: [`02-recipes/advanced-filters`](examples/02-recipes/advanced-filters/) — AND/OR + IN-style filtering against a small product catalog.

---

##### 🧮 Operators

Every relational operator works against typed values; string operators target string fields; existence operators ignore the comparison value.

| Operator | Description |
|----------|-------------|
| `Equal` | value == reference |
| `NotEqual` | value != reference |
| `GreaterThan` | value > reference |
| `GreaterThanOrEqual` | value >= reference |
| `LessThan` | value < reference |
| `LessThanOrEqual` | value <= reference |
| `Contains` | string contains substring (case-sensitive) |
| `NotContains` | string does NOT contain substring |
| `StartsWith` | string starts with prefix |
| `EndsWith` | string ends with suffix |
| `IsEmpty` | field is nil/unset or empty string |
| `IsNotEmpty` | field exists and is non-empty |
| `HasKey` | BytesVal map contains the specified key |
| `HasNotKey` | BytesVal map does NOT contain the key |
| `SliceContains` | BytesVal slice contains exact value |
| `SliceNotContains` | BytesVal slice does NOT contain value |
| `SliceContainsSubstring` | any string element contains substring (case-insensitive) |
| `SliceNotContainsSubstring` | no string element contains substring |
| `StringIn` | field value equals any of the given string values |
| `Int32In` | field value equals any of the given int32 values |
| `Int64In` | field value equals any of the given int64 values |

---

##### 📦 Slice Filters

Three flavours of slice membership testing on msgpack slice fields.

**SliceContains — exact match in a slice:**

```go
// Does LLMSiteFunctions contain 7 (Booking)?
hydraidego.FilterBytesFieldSliceContainsInt8("LLMSiteFunctions", int8(7))

// Does PaymentProviders contain "Barion"? (case-sensitive)
hydraidego.FilterBytesFieldSliceContainsString("PaymentProviders", "Barion")

// Also: SliceContainsInt32, SliceContainsInt64
```

**SliceNotContains — negated exact match:**

```go
// Exclude e-commerce sites
hydraidego.FilterBytesFieldSliceNotContainsInt8("LLMSiteFunctions", int8(1))

// Does NOT accept Stripe
hydraidego.FilterBytesFieldSliceNotContainsString("PaymentProviders", "Stripe")
```

**SliceContainsSubstring — case-insensitive substring in string slices:**

```go
// Any activity contains "tattoo" (matches "custom tattoo design", "Tattoo Art", …)
hydraidego.FilterBytesFieldSliceContainsSubstring("LLMDetailedActivities", "tattoo")

// No activity contains "permanent makeup"
hydraidego.FilterBytesFieldSliceNotContainsSubstring("LLMDetailedActivities", "permanent makeup")
```

These compose with `FilterAND` / `FilterOR` like any other filter.

---

##### 📏 Slice Length (`#len`)

Check the length of a slice or map via the `#len` pseudo-field. Internally this appends `.#len` to the field path and exposes the length as an int comparable with any standard operator.

```go
// At least 1 contact
hydraidego.FilterBytesFieldSliceLen(hydraidego.GreaterThan, "LLMContacts", 0)

// Exactly 3 industry sectors
hydraidego.FilterBytesFieldSliceLen(hydraidego.Equal, "LLMIndustrySectors", 3)

// Empty slice (no product categories)
hydraidego.FilterBytesFieldSliceLen(hydraidego.Equal, "LLMProductCategories", 0)

// Maps work too: metadata has fewer than 5 keys
hydraidego.FilterBytesFieldSliceLen(hydraidego.LessThan, "Metadata", 5)
```

---

##### 🪞 Nested Slice Any (`[*]` wildcard)

Match if **any** element in a struct slice has a field satisfying a condition. Internally builds a path like `"LLMContacts[*].Email"`.

```go
// At least 1 contact has a non-empty email
hydraidego.FilterBytesFieldNestedSliceAnyString("LLMContacts", "Email", hydraidego.IsNotEmpty, "")

// At least 1 contact is a CEO
hydraidego.FilterBytesFieldNestedSliceAnyString("LLMContacts", "Role", hydraidego.Equal, "CEO")

// At least 1 contact has a domain-matching email
hydraidego.FilterBytesFieldNestedSliceAnyBool("LLMContacts", "IsDomainMatch", hydraidego.Equal, true)
```

Available types: `NestedSliceAnyString`, `NestedSliceAnyInt8`, `NestedSliceAnyBool`.

> **Limitation:** each filter checks one condition. AND-ing two `NestedSliceAny` filters may match different elements. To require the **same** element to satisfy all conditions, use `FilterNestedSliceWhere` (see above).

---

##### 🚫 ExcludeKeys — server-side key exclusion

Skip specified keys before filter evaluation. O(1) lookup per treasure (~10ns).

Use cases: pagination without offset, deduplication, "show more" patterns.

```go
// Second page: exclude first page results
index := &hydraidego.Index{
    IndexType:   hydraidego.IndexCreationTime,
    IndexOrder:  hydraidego.IndexOrderDesc,
    MaxResults:  10,
    ExcludeKeys: []string{"domain1.com", "domain2.com", "domain3.com"},
}
```

Works with `CatalogReadManyStream`, `CatalogReadManyFromMany`, and `CatalogReadMany`.

---

##### ✅ IncludedKeys — server-side key whitelist

Restrict the result set to a specific list of keys. Runs **before** ExcludeKeys and filters.

Execution order: **IncludedKeys → ExcludeKeys → Filters → Response**.

Use cases: subset search within a precomputed candidate list, re-validation, user selections.

```go
index := &hydraidego.Index{
    IndexType:    hydraidego.IndexCreationTime,
    IndexOrder:   hydraidego.IndexOrderDesc,
    IncludedKeys: candidateKeys,
    ExcludeKeys:  alreadySeenKeys,
    MaxResults:   10,
}
```

---

##### 🪶 KeysOnly — lightweight key-only results

Return only Treasure keys (Key + IsExist), skipping content serialization. Reduces gRPC payload dramatically (~16× faster than full conversion, ~17ns per treasure).

Use cases: large result discovery, two-phase search (KeysOnly to find, `CatalogReadBatch` for the chosen ones).

```go
// Phase 1: discover matching keys
index := &hydraidego.Index{
    IndexType:  hydraidego.IndexCreationTime,
    IndexOrder: hydraidego.IndexOrderDesc,
    MaxResults: 1000,
    KeysOnly:   true,
}

var matchedKeys []string
h.CatalogReadManyStream(ctx, swamp, index, filters, Model{}, func(model any) error {
    matchedKeys = append(matchedKeys, model.(*Model).Key)
    return nil
})

// Phase 2: hydrate the chosen subset
h.CatalogReadBatch(ctx, swamp, matchedKeys[:10], Model{}, func(model any) error { ... })
```

---

##### 🏷️ SearchResultMeta — scores and matched labels

When filters carry labels or VectorFilters are used, each streaming result includes a `SearchMeta` with similarity scores and the matched filter labels. Works in all modes — including `KeysOnly`.

**Adding labels:**

```go
hydraidego.FilterBytesFieldSliceContainsInt8("LLMSiteFunctions", int8(7)).WithLabel("booking")
hydraidego.FilterVector("Embedding", queryVec, 0.5).WithLabel("semantic")
hydraidego.GeoDistance("Lat", "Lng", 47.497, 19.040, 50.0, hydraidego.GeoInside).WithLabel("location")
hydraidego.FilterPhrase("WordIndex", "hello", "world").WithLabel("phrase")
```

Unlabeled filters work normally but do not appear in `MatchedLabels`. VectorFilter scores are captured regardless of labels.

**Reading the metadata** via the `hydraide:"searchMeta"` tag — auto-populated on read, never written:

```go
type MyModel struct {
    Domain string                 `hydraide:"key"`
    Body   *Body                  `hydraide:"value"`
    Meta   *hydraidego.SearchMeta `hydraide:"searchMeta"`
}

h.CatalogReadManyStream(ctx, swamp, index, filters, MyModel{}, func(model any) error {
    m := model.(*MyModel)
    if m.Meta != nil {
        if len(m.Meta.VectorScores) > 0 {
            fmt.Printf("  relevance: %.2f\n", m.Meta.VectorScores[0])
        }
        fmt.Printf("  matched: %v\n", m.Meta.MatchedLabels)
    }
    return nil
})
```

In OR groups every matching branch is reported (not just the first):

```go
filters := hydraidego.FilterOR(
    hydraidego.FilterBytesFieldSliceContainsInt8("LLMIndustrySectors", int8(1)).WithLabel("hospitality"),
    hydraidego.FilterBytesFieldSliceContainsInt8("LLMIndustrySectors", int8(6)).WithLabel("health"),
)
// A domain in both sectors → MatchedLabels = ["hospitality", "health"]
```

**Performance:** zero overhead on the fast path (no labels, no vectors). With labels/vectors, ~35% overhead (~1.8µs vs ~1.3µs).

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
| `CatalogShiftExpired` | ✅ Yes     | ✅ Yes       | Process expired items by TTL — drains the queue |
| `CatalogPatchExpired` | ✅ Yes     | ❌ No        | Atomic in-place patch of expired items — claim-and-keep |
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

> Full example with edge cases and best practices: [example](examples/)

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
| CatalogCreate             | ✅ Ready | [example](examples/)             |
| CatalogCreateMany         | ✅ Ready | [example](examples/)             |
| CatalogCreateManyToMany   | ✅ Ready | [example](examples/)             |
| CatalogRead               | ✅ Ready | [example](examples/)              |
| CatalogReadMany           | ✅ Ready | [example](examples/)            |
| CatalogReadBatch          | ✅ Ready | [example](examples/)            |
| CatalogUpdate             | ✅ Ready | [example](examples/)              |
| CatalogUpdateMany         | ✅ Ready | [example](examples/)              |
| CatalogDelete             | ✅ Ready | [example](examples/)              |
| CatalogDeleteMany         | ✅ Ready | [example](examples/)              |
| CatalogDeleteManyFromMany | ✅ Ready | [example](examples/)            |
| CatalogSave               | ✅ Ready | [example](examples/)             |
| CatalogSaveMany           | ✅ Ready | [example](examples/)             |
| CatalogSaveManyToMany     | ✅ Ready | [example](examples/)             |
| CatalogReadManyStream     | ✅ Ready | [example](examples/)            |
| CatalogReadManyFromMany   | ✅ Ready | [example](examples/)            |
| CompactSwamp              | ✅ Ready | Encoding migration helper — forces .hyd file rewrite              |
| CatalogShiftExpired       | ✅ Ready | [example](examples/)              |
| CatalogPatchExpired       | ✅ Ready | Atomic in-place patch of expired entries — see §Field-Level Patches |
| CatalogShiftBatch         | ✅ Ready | [example](examples/)              |

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

👉 [Atomic counter recipe](examples/02-recipes/atomic-counter/)

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

### 🧬 Field-Level Patches – Type-Preserving Structural Mutations

HydrAIDE’s `CatalogPatch*` family lets you mutate **individual fields** inside a MessagePack-encoded Catalog Treasure — without ever loading the whole document on the client side.

Whether you want to:

* flip a single boolean flag on a hot key (`IsCrawling`, `IsRejected`, …),
* atomically increment a nested counter (`Counters.Views`),
* append an event to a slice (`Events[]`),
* shallow-merge a partial update map into a nested document,

…you can do it with **one server-side splice**, optionally guarded by a condition, optionally stamping `UpdatedAt`/`UpdatedBy` in the same atomic call.

#### 🧠 Why this is a game-changer

* ⚡ **No more `Lock + Load + Save` loops** — the round-trip pattern that produced `MULTI-HOLD` lock contention on hot keys disappears entirely
* 🧬 **Wire-level type preservation** — untouched fields stay byte-identical (`int8` stays `int8`, `time.Time` keeps its canonical extension encoding), and mutated fields take on the exact type the client encoded
* 🔒 **Per-key atomicity** — every op in a patch either commits together or none does, under the same FIFO guard that powers `IncrementInt8`
* 🚀 **Per-key parallelism** — different keys run fully in parallel; same-key writers queue without blocking the swamp
* 🎯 **Path expressions** — dotted nested fields and bracketed array indices: `Foo`, `Foo.Bar.Baz`, `Tags[3]`, `Tags[]` (append marker)
* 🛡️ **Conditional safety** — eight comparators (`EQUAL`, `NOT_EQUAL`, `GT`/`GTE`, `LT`/`LTE`, `EXISTS`, `NOT_EXISTS`) block ops when the pre-condition does not hold

> Read the philosophy and rationale here: [`docs/features/structural-msgpack-patch.md`](../../features/structural-msgpack-patch.md)

#### 🧰 Available Functions

| Function                   | Purpose                                                                  |
| -------------------------- | ------------------------------------------------------------------------ |
| `CatalogPatchField`        | One key, one field — the simplest one-liner                              |
| `CatalogPatchFields`       | One key, multiple fields — atomic under one guard hold                   |
| `CatalogPatchFieldsMany`   | Multi-key batch with a per-key result iterator (and an optional per-request `Cond`) |
| `CatalogPatch`             | Fluent **builder** for advanced ops (Inc, Append, Merge, Conditions, Meta) |
| `CatalogPatchExpired`      | Atomic in-place patch of expired treasures — disjoint subsets across concurrent callers, queue-claim primitive |

> ⚠️ The patch primitive **requires MessagePack-encoded Catalog Treasures**. Register the swamp with `EncodingFormat: EncodingMsgPack`. GOB-encoded values return `ENCODING_NOT_SUPPORTED`. Profile Swamps are not patchable — there each struct field is its own Treasure key, so `ProfileSave` already gives you the same effect.

#### 📌 Quick Examples

##### 1. Single field

```go
status, err := h.CatalogPatchField(ctx, swampName, "domain1.hu", "IsInQueue", true)
if err != nil { /* transport error */ }
switch status {
case hydraidego.PatchStatusCreated, hydraidego.PatchStatusPatched:
    // applied
case hydraidego.PatchStatusEncodingNotSupported:
    // existing record is not msgpack-encoded
}
```

##### 2. Multiple fields in one call

```go
_, err := h.CatalogPatchFields(ctx, swampName, "domain1.hu", map[string]any{
    "IsCrawling":     false,
    "IsRejected":     true,
    "RejectedReason": int16(7),       // stays int16 on the wire
    "StatusCounter":  int32(1),       // stays int32 on the wire
})
```

##### 3. Multi-key batch with iterator

```go
err := h.CatalogPatchFieldsMany(ctx, swampName, []*hydraidego.PatchManyRequest{
    {Key: "d1.hu", Fields: map[string]any{"IsInQueue": true}},
    {Key: "d2.hu", Fields: map[string]any{"IsCrawling": false}},
    {Key: "d3.hu", Fields: map[string]any{"IsRejected": true}},
}, func(key string, status hydraidego.PatchStatus, errMsg string) error {
    if status != hydraidego.PatchStatusPatched && status != hydraidego.PatchStatusCreated {
        slog.Warn("patch outcome", "key", key, "status", status, "error", errMsg)
    }
    return nil
})
```

Each `PatchManyRequest` can also carry an optional `Cond *PatchCond` for **batched optimistic CAS** in a single round-trip — the per-request equivalent of the builder's `IfField*` chain:

```go
requests := []*hydraidego.PatchManyRequest{
    {
        Key:    "d1.hu",
        Fields: map[string]any{"ClaimedBy": "worker-A"},
        Cond:   &hydraidego.PatchCond{Op: hydraidego.PatchCondEqual, Path: "ClaimedBy", Value: ""},
    },
    {
        Key:    "d2.hu",
        Fields: map[string]any{"Counter": int32(1)},
        Cond:   &hydraidego.PatchCond{Op: hydraidego.PatchCondLessThan, Path: "Counter", Value: int32(3)},
    },
}
```

CAS failures surface as `PatchStatusConditionNotMet` per request; the rest of the batch still applies.

##### 4. Builder API — conditional, multi-op

```go
status, err := h.
    CatalogPatch(ctx, swampName, "domain1.hu").
    Inc("StatusCounter", int32(1)).            // atomic counter bump (preserves int32)
    Set("IsInQueue", true).                    // boolean flag
    Append("Events[]", "boot").                // append to a slice
    IfFieldEquals("Owner", "alice").           // pre-condition
    WithUpdatedAt().
    WithUpdatedBy("worker-7").
    Exec()

switch status {
case hydraidego.PatchStatusPatched:
    // all three ops applied
case hydraidego.PatchStatusConditionNotMet:
    // someone else changed Owner — patch was skipped
case hydraidego.PatchStatusKeyNotFound:
    // would only happen with .NoCreate(); default is auto-create
}
```

##### 5. Optimistic update on a numeric field

```go
status, err := h.
    CatalogPatch(ctx, swampName, "domain1.hu").
    Inc("Counter", int32(1)).
    IfFieldLessThan("Counter", int32(100)).    // only if still under limit
    Exec()

if status == hydraidego.PatchStatusConditionNotMet {
    // limit reached, no increment performed
}
```

##### 6. Slide or clear `ExpiredAt` in the same patch (server v3.13.0+)

```go
// Attach (or refresh) a 24h TTL while bumping a counter.
status, err := h.
    CatalogPatch(ctx, swampName, "session:abc").
    Inc("HitCount", int32(1)).
    WithUpdatedAt().
    WithExpiredAt(time.Now().UTC().Add(24 * time.Hour)).
    Exec()

// Promote a transient row to permanent — drop the TTL atomically.
status, err = h.
    CatalogPatch(ctx, swampName, "session:abc").
    Set("Tier", "permanent").
    WithoutExpiredAt().
    Exec()
```

##### 7. Atomic claim of expired treasures (`CatalogPatchExpired`)

The in-place sibling of `CatalogShiftExpired`. Selects up to `howMany` expired treasures under the swamp's beacon lock, applies a shared op-set + meta to each one under its per-key guard, and re-indexes them with their new `ExpireAt`. **Concurrent callers receive disjoint subsets** — same atomic-claim guarantee as `CatalogShiftExpired`, but the data stays in the swamp.

The classic use is a crash-safe queue claim with a single round-trip (no separate fetch + lock):

```go
builder := hydraidego.NewPatchExpiredOps().
    Set("ClaimedBy", workerID).
    WithExpiredAt(time.Now().UTC().Add(24 * time.Hour)).  // lease deadline
    IfFieldEquals("ClaimedBy", "")                        // optional CAS gate

err := h.CatalogPatchExpired(ctx, swampName, 50, MyCatalog{},
    func(model any, status hydraidego.PatchStatus) error {
        m := model.(*MyCatalog)
        // process the claimed entry; the new ExpireAt is the lease deadline
        return nil
    }, builder)
```

Recovery works without extra code: if the worker crashes, the lease (`ExpireAt = claim_time + 24h`) elapses and the next caller re-claims the entry. CONDITION_NOT_MET treasures stay in the expired index with their unchanged `ExpireAt` so the next call can retry them — useful for "claim only the entries where `ClaimedBy == ''`" without server-side state.

Empty ops + non-nil meta is the **meta-only patch** form (slide `ExpireAt` forward without changing the body — typical for lease extensions and recheck deferrals):

```go
err := h.CatalogPatchExpired(ctx, swampName, 100, MyCatalog{},
    func(model any, status hydraidego.PatchStatus) error { return nil },
    hydraidego.NewPatchExpiredOps().WithExpiredAt(time.Now().UTC().Add(7 * 24 * time.Hour)))
```

`PatchExpiredOps` mirrors `PatchBuilder` minus the per-key `Exec`:

| Surface     | Helpers                                                                       |
| ----------- | ----------------------------------------------------------------------------- |
| Ops         | `Set`, `Inc`, `Append`, `Prepend`, `Delete`, `RemoveAt`, `RemoveVal`, `Merge` |
| Conditions  | every `IfField*` from `PatchBuilder`, single condition per builder            |
| Meta        | `WithUpdatedAt`, `WithUpdatedBy`, `WithExpiredAt(t)`, `WithoutExpiredAt()`    |

> Read the philosophy and atomicity contract here: [`docs/features/patch-expired-treasures.md`](../../features/patch-expired-treasures.md). Requires server v3.14.0 or newer.

#### 📚 Op Reference

| Builder Method          | Wire Op       | Notes                                                                                              |
| ----------------------- | ------------- | -------------------------------------------------------------------------------------------------- |
| `Set(path, value)`      | `SET`         | Replaces or creates the value at *path*. Auto-creates missing intermediate maps.                   |
| `Delete(path)`          | `DELETE`      | Removes the field/index. Missing target is a no-op.                                                |
| `Inc(path, delta)`      | `INC`         | Class-aware increment. Preserves the target's exact numeric msgpack code (`int8` stays `int8`).    |
| `Append(path, value)`   | `APPEND`      | Path must end in `[]` — `Tags[]`. Auto-creates an empty array on a missing field.                  |
| `Prepend(path, value)`  | `PREPEND`     | Same path/auto-create rules as `Append`.                                                           |
| `RemoveAt(path)`        | `REMOVE_AT`   | Path must include an index — `Tags[3]`. Out-of-range yields `PATH_INVALID`.                        |
| `RemoveVal(path, value)`| `REMOVE_VAL`  | Removes the first array element whose msgpack-encoded bytes equal *value*. Not present is a no-op. |
| `Merge(path, value)`    | `MERGE`       | Shallow merge of a map into the target map. Conflicting keys overwrite; others are preserved.      |

#### 🛡️ Condition Reference

| Builder Method                            | Wire Op                  |
| ----------------------------------------- | ------------------------ |
| `IfFieldEquals(path, value)`              | `EQUAL`                  |
| `IfFieldNotEquals(path, value)`           | `NOT_EQUAL`              |
| `IfFieldGreaterThan(path, value)`         | `GREATER_THAN`           |
| `IfFieldGreaterThanOrEqual(path, value)`  | `GREATER_THAN_OR_EQUAL`  |
| `IfFieldLessThan(path, value)`            | `LESS_THAN`              |
| `IfFieldLessThanOrEqual(path, value)`     | `LESS_THAN_OR_EQUAL`     |
| `IfFieldExists(path)`                     | `EXISTS`                 |
| `IfFieldNotExists(path)`                  | `NOT_EXISTS`             |

Conditions are evaluated **before any op runs**. If the comparison fails, you get `PatchStatusConditionNotMet` and the blob is left untouched. Only one condition is supported per patch in V1; for compound logic, issue multiple sequential `PatchTreasures` calls (atomicity is then per-call, not across calls).

#### 🏷️ Metadata Helpers

| Builder Method                | Wire Field         | Notes                                                                                                        |
| ----------------------------- | ------------------ | ------------------------------------------------------------------------------------------------------------ |
| `WithUpdatedAt()`             | `SetUpdatedAt`     | Server stamps `ModifiedAt` to now on the patched Treasure.                                                   |
| `WithUpdatedBy(userID)`       | `SetUpdatedBy`     | Server stamps `ModifiedBy = userID`.                                                                         |
| `WithExpiredAt(t time.Time)`  | `SetExpiredAt`     | Server sets `ExpiredAt` (existing or newly created Treasure). Slide a TTL or attach one. Zero `time.Time` clears the TTL, equivalent to `WithoutExpiredAt()`. Requires server **v3.13.0+**; older servers silently drop the field. |
| `WithoutExpiredAt()`          | `ClearExpiredAt`   | Resets `ExpiredAt` to "never expires". Wins over a prior `WithExpiredAt` on the same builder.                |

Metadata is applied under the same per-key guard as the ops, so the body and metadata changes commit atomically. Created\* helpers (set via raw `PatchMeta` in lower-level calls) only apply to Treasures created in the same call.

#### 📊 Status Codes

A patch can return one of nine statuses:

| `PatchStatus`                    | Meaning                                                                       |
| -------------------------------- | ----------------------------------------------------------------------------- |
| `PatchStatusPatched`             | Ops were applied to an existing record.                                       |
| `PatchStatusCreated`             | Record did not exist, was created and patched (`CreateIfNotExist` was true).  |
| `PatchStatusKeyNotFound`         | Record did not exist and `.NoCreate()` was set on the builder.                |
| `PatchStatusConditionNotMet`     | Condition evaluated to false — ops were not applied.                          |
| `PatchStatusFieldNotFound`       | Reserved for ops that strictly require an existing field (currently unused).  |
| `PatchStatusTypeMismatch`        | Op crossed a type boundary (INC on a string, MERGE on a non-map, etc.).       |
| `PatchStatusPathInvalid`         | Malformed path or unresolvable index (e.g. out-of-range `Tags[10]`).          |
| `PatchStatusEncodingNotSupported`| Existing Treasure value is not msgpack-encoded.                               |
| `PatchStatusInternalError`       | Unexpected server failure. Returns a non-nil Go error in addition.            |

> ⚙️ The Go SDK only returns a non-nil `error` for transport-level failures and `INTERNAL_ERROR`. Every other per-key outcome — including `CONDITION_NOT_MET`, `TYPE_MISMATCH`, `KEY_NOT_FOUND` — surfaces as a status, so `if err != nil` doesn't swallow business logic.

#### 🧱 Auto-Create Behavior

Both the helpers (`CatalogPatchField`, `CatalogPatchFields`, `CatalogPatchFieldsMany`) and the builder default to `CreateIfNotExist = true`. Missing intermediate map levels are auto-created when the final segment is a field name, so

```go
h.CatalogPatchField(ctx, swampName, "domain1.hu", "stats.crawl.attempts", int32(1))
```

…will create the empty `stats` map, then the empty `crawl` map, then the `attempts` field, even if none of them existed before. To opt out, use the builder’s `.NoCreate()`:

```go
status, _ := h.CatalogPatch(ctx, swampName, "missing-key").
    NoCreate().
    Set("x", int8(1)).
    Exec()
// status == PatchStatusKeyNotFound
```

#### 🚦 Production Notes

* **Stress run, locally**: 6 workers patching different flag fields across 100 domains for 5 seconds dispatched ~44k patches with zero per-key errors and zero lost updates.
* **Telemetry**: every `PatchTreasures` call is captured by the same unary interceptor that instruments the rest of the gRPC API, so you can stream patch activity via `SubscribeToTelemetry` without extra wiring.
* **Compaction**: patches produce normal `OpUpdate` entries on the underlying `.hyd` file; the existing compaction lifecycle applies.
* **Backward compatibility**: this is purely additive — no existing RPC was changed, every prior client keeps working exactly as before.

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

All of these are demonstrated in the [example](examples/) Go model, which shows how to:

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

📖 [Testing HydrAIDE models against a live instance](testing.md)

This approach eliminates the need for complex mocking while giving you confidence that your code works correctly in production-like conditions.
