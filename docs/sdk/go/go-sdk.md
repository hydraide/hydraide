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
10. [➕ Increment / Decrement – Atomic State Without the Overhead](#-increment--decrement--atomic-state-without-the-overhead)
11. [📌 Slice & Reverse Indexing in HydrAIDE](#-slice--reverse-indexing-in-hydraide)

---

## 🔌 Connect to the HydrAIDE Server Using the SDK

The first and most essential step is establishing a connection to the HydrAIDE server using the Go SDK.

To do this, implement the `repo` package. This package is typically placed under `/utils/repo` and should be 
initialized during your application's startup sequence.

You can find the repo implementation and usage examples here:

📁 [`repo.go`](examples/models/utils/repo/repo.go)

### How to Start Your Server Using the Repo Package

For a complete working example of how to initialize and run your service using the `repo` package, take a look at the demo application:

▶️ [`main.go` in app-queue](examples/applications/app-queue/main.go)m a minimal end-to-end example of SDK setup and Swamp registration with a queue service

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
````

Internally, HydrAIDE stores this under a Swamp like:

```
/users/profiles/user-123
```

Each field is stored in binary chunks — only if the value is present (thanks to `hydraide:"omitempty"`).

#### 📂 SDK Example Files

| Function                       | SDK Status | Go Example                                                       |
|--------------------------------| ---------- | ---------------------------------------------------------------- |
| `Profile Save, Read, Destroy` | ✅ Ready    | [profile_save_read_destroy.go](examples/models/profile_save_read_destroy.go)   |

🧪 **Looking for a complete production-ready model?**
Check out [profile_save_read_destroy.go](examples/models/profile_save_read_destroy.go) — a real-world example with nested structs, 
timestamps, and struct pointers for user avatars, preferences, and security.

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
  🔄 Fully reactive: supports real-time streaming via Subscribe()

> 💡 Catalog Swamps are the most reactive data structures in HydrAIDE.
> They are the primary targets for Subscribe() operations, making it easy to listen to individual keys (Treasure-level events), filtered value changes, or full Swamp activity.
> This makes Catalogs ideal for building live dashboards, notification systems, audit pipelines, and streaming analytics.

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
}

_ = user.Save(repo) // Upserts the record
```

This stores a Treasure in:

```
/users/catalog/all → key: user-123 → value: Payload + metadata
```

HydrAIDE will track when and who wrote the data, and can later stream or react to changes over time.

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


Tökéletes ötlet, Peti. Itt egy javasolt `#### 📚 Good to Know` szekció, amit **közvetlenül a `🧯 When Not to Use Catalogs`** után tudsz beilleszteni.

Ez a rész segít a skálázhatósági problémák kezelésében, és gyakorlati példákon keresztül mutatja meg, hogyan lehet a Catalog Swampokat *logikailag szegmentálni*:


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

```go
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
| CatalogUpdate             | ✅ Ready | [catalog_update.go](examples/models/catalog_update.go)              |
| CatalogUpdateMany         | ✅ Ready | [catalog_update_many.go](examples/models/catalog_update_many.go)              |
| CatalogDelete             | ✅ Ready | [catalog_delete.go](examples/models/catalog_delete.go)              |
| CatalogDeleteMany         | ✅ Ready | [catalog_delete.go](examples/models/catalog_delete.go)              |
| CatalogDeleteManyFromMany | ✅ Ready | [catalog_delete_many_from_many.go](examples/models/catalog_delete_many_from_many.go)            |
| CatalogSave               | ✅ Ready | [catalog_save.go](examples/models/catalog_save.go)             |
| CatalogSaveMany           | ✅ Ready | [catalog_save_many.go](examples/models/catalog_save_many.go)             |
| CatalogSaveManyToMany     | ✅ Ready | [catalog_save_many_to_many.go](examples/models/catalog_save_many_to_many.go)             |
| CatalogShiftExpired       | ✅ Ready | [catalog_shift_expired.go](examples/models/catalog_shift_expired.go)              |

---

### ➕ Increment / Decrement – Atomic State Without the Overhead

HydrAIDE’s `Increment*` family of functions enables **atomic, type-safe updates** of numeric values — without reading, locking, or overwriting state manually.

Whether you're updating:

* a user's **rate limit**,
* a device's **event count**,
* a game **score**,
* a financial **balance**,
* or a processing **threshold**,

…you can do it with **one intent-first operation** — optionally guarded by conditions like *“only increment if current value < 100”*.

#### 🧠 Why this is a game-changer:

* ⚡ **Atomic execution** — no race conditions, no read-modify-write logic
* 🔒 **Treasure-level locking only** — never blocks the entire Swamp
* 🧬 **Strongly typed** — choose from `int8`, `uint32`, `float64`, etc.
* ✅ **Condition-aware** — support for rich comparisons:
  `Equal`, `NotEqual`, `GreaterThan`, `LessThanOrEqual`, etc.

> This isn’t just math — it’s **concurrent state mutation**, encoded as intention.

#### 📌 One demo to rule them all

All `Increment*` functions work the same way — only the type changes.

To see a complete example in action (including conditional logic and memory-only Swamps), check out:

👉 [Catalog Model Rate Limit Counter](examples/models/increment.go)

This single model demonstrates how to:

* atomically update a counter,
* guard the operation with a relational condition,
* scale to thousands of users with no locks or I/O,
* and reset the state via `Destroy()`.

It applies to **all numeric increment types**, from `int8` to `float64`.

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

> 💡 Only the numeric type changes — the logic stays the same.

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
