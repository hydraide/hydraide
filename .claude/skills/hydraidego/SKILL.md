---
name: hydraidego
description: Building HydrAIDE data models and applications with the Go SDK (`hydraidego`). Sanctuary/Realm/Swamp addressing, Profile vs Catalog patterns, struct tags, msgpack encoding, server-side filters (AND/OR, vector, geo, nested-slice, phrase, IN), atomic increments, distributed locks, real-time subscriptions, structural patches, indexing/pagination, common pitfalls. Use when designing, implementing, or debugging Go application code against HydrAIDE. For other languages, see the corresponding `hydraide<lang>` skill. For server operations, see the `hydraidectl` skill.
---

# HydrAIDE — Go SDK Data Model and Application Guide

This skill is the working reference for building on HydrAIDE with the **Go SDK** (`hydraidego`). Read it before designing a new model or touching unfamiliar parts of the API.

The proto file is the source of truth for the wire protocol. The Go SDK (`hydraidego`) is a convenience wrapper over it — anything described here that uses SDK names corresponds to a method on the proto.

---

## 1. Core concepts

### Addressing: Sanctuary → Realm → Swamp

HydrAIDE uses a deterministic 3-level namespace. Every piece of data lives inside a Swamp.

```
Sanctuary → Realm → Swamp
   ↓          ↓       ↓
service    type     unique-id
```

```go
name.New().
    Sanctuary("myapp").
    Realm("user-profile").
    Swamp(userID)
```

Rules:

- All three levels are required, minimum one character.
- Alphanumeric, plus `-` and `:`. Avoid `/` (internal separator).
- `*` is allowed only in `RegisterSwamp` patterns.

Typical sharding strategies:

| Use case | Sanctuary | Realm | Swamp |
|---|---|---|---|
| User profile | myapp | user-profile | `<userID>` |
| Per-tenant catalog | myapp | order-catalog | `<tenantID>` |
| Per-domain index | myapp | domain-catalog | `<tld>` |
| Compound key | myapp | message-catalog | `<tenantID>:<conversationID>` |
| Distributed lock | myapp | lock-catalog | `<tenantID>` |

### Two data models: Profile vs Catalog

| Property | Profile | Catalog |
|---|---|---|
| Storage unit | Single entity per Swamp | Key/value collection per Swamp |
| Struct tags | Field names = keys | `key` + `value` tags |
| Metadata | None built in | `createdAt`, `updatedAt`, `createdBy`, `updatedBy`, `expireAt` |
| Use when | One canonical record per Swamp | Many records keyed inside a Swamp |
| Operations | `ProfileSave` / `ProfileRead` | `CatalogSave` / `CatalogRead` / `CatalogDelete` and many more |

Decision rule:

- One logical entity per Swamp → **Profile**.
- Many keyed records per Swamp → **Catalog**.
- Need pagination, sorting, server-side filtering → **Catalog** (Index + Filter support).

---

## 2. Profile model

A single logical entity per Swamp. Each struct field is stored as its own Treasure inside the Swamp.

### Struct definition

```go
type UserProfile struct {
    UserID            string
    DisplayName       string
    IsActive          bool      `hydraide:"omitempty"`
    DailyMessageLimit int32
    LastLoginAt       time.Time `hydraide:"omitempty,deletable"`
    Timezone          string    `hydraide:"omitempty,deletable"`
    CreatedAt         time.Time
    UpdatedAt         time.Time `hydraide:"omitempty,deletable"`
}
```

### Addressing helper

```go
func (m *UserProfile) name() name.Name {
    return name.New().
        Sanctuary("myapp").
        Realm("user-profile").
        Swamp(m.UserID)
}
```

### Save and load

```go
func (m *UserProfile) Save(r repo.Repo) error {
    ctx, cancel := hydraidehelper.CreateHydraContext()
    defer cancel()

    _, err := r.GetHydraidego().ProfileSave(ctx, m.name(), m)
    if err != nil {
        slog.Error("Failed to save user profile", "userID", m.UserID, "error", err)
        return err
    }
    return nil
}

func (m *UserProfile) Load(r repo.Repo) error {
    ctx, cancel := hydraidehelper.CreateHydraContext()
    defer cancel()

    if err := r.GetHydraidego().ProfileRead(ctx, m.name(), m); err != nil {
        if hydraidego.IsSwampNotFound(err) || hydraidego.IsNotFound(err) {
            return ErrUserNotFound
        }
        slog.Error("Failed to load user profile", "userID", m.UserID, "error", err)
        return err
    }
    return nil
}
```

### Batch read

```go
func LoadBatch(r repo.Repo, userIDs []string) ([]*UserProfile, error) {
    if len(userIDs) == 0 {
        return nil, nil
    }
    ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
    defer cancel()

    swamps := make([]name.Name, 0, len(userIDs))
    for _, id := range userIDs {
        swamps = append(swamps, name.New().
            Sanctuary("myapp").Realm("user-profile").Swamp(id))
    }

    var out []*UserProfile
    err := r.GetHydraidego().ProfileReadBatch(ctx, swamps, &UserProfile{},
        func(swampName name.Name, model any, err error) error {
            if err != nil {
                if hydraidego.IsSwampNotFound(err) || hydraidego.IsNotFound(err) {
                    return nil
                }
                return nil
            }
            out = append(out, model.(*UserProfile))
            return nil
        })
    return out, err
}
```

### Filtered batch read

`ProfileReadBatchWithFilter` runs server-side filters across many Profile Swamps in one streaming call.

```go
filters := hydraidego.FilterAND(
    hydraidego.FilterBytesFieldBool(hydraidego.Equal, "IsActive", true),
)

err := r.GetHydraidego().ProfileReadBatchWithFilter(ctx, swamps, filters,
    &UserProfile{}, 0, // maxResults=0 → unlimited
    func(swampName name.Name, model any, err error) error {
        if err != nil {
            if hydraidego.IsSwampNotFound(err) || hydraidego.IsNotFound(err) {
                return nil
            }
            return err
        }
        // handle model.(*UserProfile)
        return nil
    })
```

### Profile operations

| Operation | Method |
|---|---|
| Save | `ProfileSave(ctx, name, model)` |
| Read | `ProfileRead(ctx, name, model)` |
| Batch read | `ProfileReadBatch(ctx, names, model, iterator)` |
| Batch save | `ProfileSaveBatch(ctx, names, models, iterator)` |
| Filtered read | `ProfileReadWithFilter(ctx, name, filters, model)` |
| Filtered batch read | `ProfileReadBatchWithFilter(ctx, names, filters, model, maxResults, iterator)` |
| Delete | `Destroy(ctx, name)` |

---

## 3. Catalog model

A collection of keyed records inside a Swamp.

### Struct definition

```go
type OrderCatalog struct {
    OrderID   string        `hydraide:"key"`
    Payload   *OrderPayload `hydraide:"value"`
    CreatedAt time.Time     `hydraide:"createdAt,omitempty"`
    CreatedBy string        `hydraide:"createdBy,omitempty"`
    UpdatedAt time.Time     `hydraide:"updatedAt,omitempty"`
    UpdatedBy string        `hydraide:"updatedBy,omitempty"`
}

type OrderPayload struct {
    CustomerID string
    Status     int8
    AmountCent int64
    Currency   string
    Items      []OrderItem
}

type OrderItem struct {
    SKU      string
    Quantity int32
    Price    int64
}
```

### Payload struct: NO `msgpack` tags needed

The HydrAIDE BytesField filters use the Go struct field names directly. Server-side filtering matches by the Go field name — `msgpack` tags are not needed.

```go
// Correct — no tag, the Go field name is the filter key
type OrderPayload struct {
    Status     int8
    Currency   string
    AmountCent int64
}

// Filter: FilterBytesFieldString(Equal, "Currency", "EUR")
// "Currency" is the Go field name — no msgpack tag required.
```

If you do add an `msgpack` tag (e.g. `msgpack:"cur"`), the filter must then reference the tag value (`"cur"`), not the field name. By default, omit the tag.

### Addressing

```go
func (m *OrderCatalog) name(tenantID string) name.Name {
    return name.New().
        Sanctuary("myapp").
        Realm("order-catalog").
        Swamp(tenantID)
}
```

### CRUD

```go
// Upsert
func (m *OrderCatalog) Save(r repo.Repo, tenantID string) error {
    ctx, cancel := hydraidehelper.CreateHydraContext()
    defer cancel()
    _, err := r.GetHydraidego().CatalogSave(ctx, m.name(tenantID), m)
    return err
}

// Read by key
func (m *OrderCatalog) Load(r repo.Repo, tenantID string) error {
    ctx, cancel := hydraidehelper.CreateHydraContext()
    defer cancel()
    if err := r.GetHydraidego().CatalogRead(ctx, m.name(tenantID), m.OrderID, m); err != nil {
        if hydraidego.IsSwampNotFound(err) || hydraidego.IsNotFound(err) {
            return nil
        }
        return err
    }
    return nil
}

// Delete by key
func (m *OrderCatalog) Delete(r repo.Repo, tenantID, orderID string) error {
    ctx, cancel := hydraidehelper.CreateHydraContext()
    defer cancel()
    return r.GetHydraidego().CatalogDelete(ctx, m.name(tenantID), orderID)
}
```

### List with index and ordering

```go
func ListOrders(r repo.Repo, tenantID string) ([]*OrderCatalog, error) {
    ctx, cancel := hydraidehelper.CreateHydraContext()
    defer cancel()

    index := &hydraidego.Index{
        IndexType:  hydraidego.IndexCreationTime,
        IndexOrder: hydraidego.IndexOrderDesc,
        Limit:      0, // 0 = all
    }

    var out []*OrderCatalog
    err := r.GetHydraidego().CatalogReadMany(ctx,
        name.New().Sanctuary("myapp").Realm("order-catalog").Swamp(tenantID),
        index, OrderCatalog{},
        func(model any) error {
            out = append(out, model.(*OrderCatalog))
            return nil
        })
    if err != nil {
        if hydraidego.IsSwampNotFound(err) || hydraidego.IsNotFound(err) {
            return out, nil
        }
        return nil, err
    }
    return out, nil
}
```

### Batch read by keys

```go
err := r.GetHydraidego().CatalogReadBatch(ctx, swamp, orderIDs, OrderCatalog{},
    func(model any) error {
        // handle model.(*OrderCatalog)
        return nil
    })
```

### Batch save

```go
models := make([]any, 0, len(orders))
for _, o := range orders {
    models = append(models, o)
}
err := r.GetHydraidego().CatalogSaveMany(ctx, swamp, models,
    func(key string, status hydraidego.EventStatus) error {
        return nil
    })
```

### Multi-Swamp operations (ManyToMany)

Batch operations across many Swamps in one call. The SDK groups requests by server automatically.

```go
// Batch upsert across many Swamps
err := h.CatalogSaveManyToMany(ctx, requests,
    func(swampName name.Name, key string, status hydraidego.EventStatus) error {
        return nil
    })

// Batch create (errors if a key already exists)
err := h.CatalogCreateManyToMany(ctx, requests,
    func(swampName name.Name, key string, err error) error {
        return nil
    })

// Batch delete across many Swamps
err := h.CatalogDeleteManyFromMany(ctx, requests,
    func(key string, err error) error {
        return nil
    })
```

Request types:

```go
type CatalogManyToManyRequest struct {
    SwampName name.Name
    Models    []any
}

type CatalogDeleteManyFromManyRequest struct {
    SwampName name.Name
    Keys      []string
}
```

### Bulk key-existence (`AreKeysExist`)

A single round-trip check whether many keys exist. Far more efficient than `IsKeyExists` in a loop.

```go
existMap, err := r.GetHydraidego().AreKeysExist(ctx, swamp, keys) // map[string]bool
```

- Returns `map[string]bool` — every requested key is present, value is `true` if it exists, `false` otherwise.
- Returns `false` for every key when the Swamp does not exist (no error).
- Empty input list short-circuits without a network call.
- Handles duplicate input keys correctly.

Typical uses: dedupe before batch insert; check which keys are already locked; skip already-processed records.

### Shift (read-and-delete in one call)

```go
// Pull a known key out of the catalog
err := r.GetHydraidego().CatalogShiftBatch(ctx, swamp, keys, OrderCatalog{},
    func(model any) error {
        // handle one popped record
        return nil
    })

// Pull expired records (TTL pattern)
err := r.GetHydraidego().CatalogShiftExpired(ctx, swamp, howMany, OrderCatalog{},
    func(model any) error {
        // handle one expired record
        return nil
    })
```

### Multi-Swamp streaming read

```go
requests := make([]*hydraidego.CatalogReadManyFromManyRequest, 0)
for _, sn := range swamps {
    requests = append(requests, &hydraidego.CatalogReadManyFromManyRequest{
        SwampName: sn,
        Index: &hydraidego.Index{
            IndexType:  hydraidego.IndexCreationTime,
            IndexOrder: hydraidego.IndexOrderDesc,
            MaxResults: 10,
        },
        Filters: nil, // per-Swamp filter is allowed
    })
}

err := h.CatalogReadManyFromMany(ctx, requests, OrderCatalog{},
    func(swampName name.Name, model any, err error) error {
        if err != nil {
            return nil // skip per-Swamp errors
        }
        // handle model
        return nil
    })
```

### Catalog operation reference

| Operation | Method | Notes |
|---|---|---|
| Upsert | `CatalogSave(ctx, name, model)` | Create or update |
| Create | `CatalogCreate(ctx, name, model)` | Errors if already exists |
| Read | `CatalogRead(ctx, name, key, model)` | Single key |
| Update | `CatalogUpdate(ctx, name, model)` | Errors if missing |
| Delete | `CatalogDelete(ctx, name, key)` | Single key |
| Batch read | `CatalogReadBatch(ctx, name, keys, model, iter)` | Many keys |
| List | `CatalogReadMany(ctx, name, index, model, iter)` | Index-ordered |
| Filtered list | `CatalogReadManyStream(ctx, name, index, filters, model, iter)` | Server-side filter |
| Multi-Swamp read | `CatalogReadManyFromMany(ctx, reqs, model, iter)` | Stream from many |
| Batch save | `CatalogSaveMany(ctx, name, models, cb)` | Many into one Swamp |
| Multi-Swamp save | `CatalogSaveManyToMany(ctx, reqs, iter)` | Many into many |
| Batch create | `CatalogCreateMany(ctx, name, models, iter)` | Errors on existing |
| Multi-Swamp create | `CatalogCreateManyToMany(ctx, reqs, iter)` | Errors on existing |
| Batch update | `CatalogUpdateMany(ctx, name, models, iter)` | Errors on missing |
| Batch delete | `CatalogDeleteMany(ctx, name, keys, iter)` | Many keys |
| Multi-Swamp delete | `CatalogDeleteManyFromMany(ctx, reqs, iter)` | Many from many |
| Shift | `CatalogShiftBatch(ctx, name, keys, model, iter)` | Read + delete |
| Shift expired | `CatalogShiftExpired(ctx, name, howMany, model, iter)` | TTL drain |
| Count | `Count(ctx, name)` | Total entries |
| Single existence | `IsKeyExists(ctx, name, key)` | Boolean |
| Batch existence | `AreKeysExist(ctx, name, keys)` | `map[string]bool` |

---

## 4. Uint32Slice operations

Native `[]uint32` Treasures, deduplicated automatically. Useful for inverted indexes, many-to-many edges, and set membership.

```go
type KeyValuesPair struct {
    Key    string
    Values []uint32
}

// Push values (deduplicated; auto-creates Swamp/Treasure)
err := h.Uint32SlicePush(ctx, swamp, []*hydraidego.KeyValuesPair{
    {Key: "category:hotel", Values: []uint32{1001, 1002, 1003}},
})

// Remove values (auto-deletes empty Treasures and the Swamp when fully drained)
err := h.Uint32SliceDelete(ctx, swamp, []*hydraidego.KeyValuesPair{
    {Key: "category:hotel", Values: []uint32{1001}},
})

// Slice size (errors if key missing or wrong type)
size, err := h.Uint32SliceSize(ctx, swamp, "category:hotel")

// Membership test
exists, err := h.Uint32SliceIsValueExist(ctx, swamp, "category:hotel", 1001)
```

Typical uses:

| Pattern | Example |
|---|---|
| Inverted index | `Push("category:hotel", domainID)` |
| Many-to-many edge set | `Push("campaign:abc", contactID)` |
| Set membership | `IsValueExist("blocked-ips", ipHash)` |
| Cleanup | `Delete("campaign:abc", contactID)` |

---

## 5. Increment operations (atomic counters)

Atomic numeric counters with optional condition and metadata. Available for every numeric type:

`IncrementInt8/16/32/64`, `IncrementUint8/16/32/64`, `IncrementFloat32/64`.

### Signature (Int32 example)

```go
func (h *hydraidego) IncrementInt32(
    ctx context.Context,
    swampName name.Name,
    key string,
    value int32,                          // delta (positive or negative)
    condition *Int32Condition,            // optional precondition
    setIfNotExist *IncrementMetaRequest,  // metadata when creating
    setIfExist *IncrementMetaRequest,     // metadata when updating
) (int32, *IncrementMetaResponse, error)
```

### Conditional increment

```go
type Int32Condition struct {
    RelationalOperator RelationalOperator
    Value              int32
}
```

If the condition fails:

- The value does not change.
- The current value is returned alongside `ErrConditionNotMet`.

### Metadata

```go
type IncrementMetaRequest struct {
    SetCreatedAt bool
    SetCreatedBy string
    SetUpdatedAt bool
    SetUpdatedBy string
    ExpiredAt    time.Time // zero = no TTL
}

type IncrementMetaResponse struct {
    CreatedAt, UpdatedAt, ExpiredAt time.Time
    CreatedBy, UpdatedBy            string
}
```

### Examples

```go
// Plain increment
newVal, _, err := h.IncrementInt32(ctx, swamp, "page-views", 1, nil, nil, nil)

// Conditional: only increment while < 100
newVal, _, err := h.IncrementInt32(ctx, swamp, "daily-emails", 1,
    &hydraidego.Int32Condition{
        RelationalOperator: hydraidego.LessThan,
        Value:              100,
    },
    &hydraidego.IncrementMetaRequest{SetCreatedAt: true, SetCreatedBy: "worker"},
    &hydraidego.IncrementMetaRequest{SetUpdatedAt: true, SetUpdatedBy: "worker"},
)
if err != nil {
    if hydraidego.IsConditionNotMet(err) {
        // limit reached, value unchanged
    }
}

// Decrement with negative delta
newVal, _, err := h.IncrementInt32(ctx, swamp, "credits", -5, nil, nil, nil)
```

Notes:

- If the Swamp/Treasure does not exist, it is created (initial value = delta).
- The Swamp does not need to be registered for Increment ops.
- Type mismatch (existing Treasure with a different numeric type) is an error.

---

## 6. Real-time subscriptions

Notify clients on Swamp changes via a streaming gRPC call.

```go
func (h *hydraidego) Subscribe(
    ctx context.Context,
    swampName name.Name,
    getExistingData bool,    // true = stream existing snapshot first
    model any,                // struct template (NOT a pointer)
    iterator SubscribeIteratorFunc,
) error
```

```go
type SubscribeIteratorFunc func(model any, eventStatus EventStatus, err error) error
```

`eventStatus` values: `StatusNew`, `StatusModified`, `StatusDeleted`, `StatusNothingChanged` (snapshot rows).

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

err := h.Subscribe(ctx, swamp, true, OrderCatalog{},
    func(model any, status hydraidego.EventStatus, err error) error {
        if err != nil {
            slog.Error("Subscribe error", "error", err)
            return nil
        }
        m := model.(*OrderCatalog)
        switch status {
        case hydraidego.StatusNew:
            // freshly created
        case hydraidego.StatusModified:
            // updated
        case hydraidego.StatusDeleted:
            // deleted
        case hydraidego.StatusNothingChanged:
            // initial snapshot row
        }
        return nil
    })
```

Subscriptions deliver write events in FIFO order. They are not a durable work queue — for retries, acknowledgements, or dead-letter handling, use a real queue (NATS JetStream, Kafka) alongside HydrAIDE.

---

## 7. Lifecycle operations

```go
// Single Swamp delete (and all its Treasures)
err := h.Destroy(ctx, swamp)

// Bulk delete with progress callback (bidi streaming, batched 500/batch)
err := h.DestroyBulk(ctx, swamps,
    func(destroyed, failed, total int64) {
        slog.Info("DestroyBulk progress",
            "destroyed", destroyed, "failed", failed, "total", total)
    })

// Force compaction (rewrites the .hyd file, drops dead entries)
err := h.CompactSwamp(ctx, swamp)
```

Other system ops:

| Operation | Method | Notes |
|---|---|---|
| Heartbeat | `Heartbeat(ctx)` | Server liveness |
| Swamp existence | `IsSwampExist(ctx, name)` | Boolean |
| Register pattern | `RegisterSwamp(ctx, req)` | Required at startup (see §9) |
| Deregister pattern | `DeRegisterSwamp(ctx, name)` | Pattern only — data stays |

---

## 8. Struct tags reference

```go
type MyModel struct {
    // === Catalog-only ===
    Key     string     `hydraide:"key"`              // Required for Catalog
    Payload *MyPayload `hydraide:"value"`            // Stored as msgpack binary

    // === Optional metadata (Catalog) ===
    CreatedAt time.Time `hydraide:"createdAt,omitempty"`
    UpdatedAt time.Time `hydraide:"updatedAt,omitempty"`
    CreatedBy string    `hydraide:"createdBy,omitempty"`
    UpdatedBy string    `hydraide:"updatedBy,omitempty"`

    // === TTL ===
    ExpireAt time.Time `hydraide:"expireAt,omitempty"`

    // === Search metadata (auto-populated on read) ===
    Meta *hydraidego.SearchMeta `hydraide:"searchMeta"`

    // === Modifiers ===
    // omitempty: skip writing zero values
    // deletable: delete the field on the server when zero (was non-zero)
    Optional  string `hydraide:"omitempty"`
    Removable string `hydraide:"omitempty,deletable"`
}
```

### `SearchMeta`

```go
type SearchMeta struct {
    VectorScores  []float32 // cosine-similarity scores per VectorFilter, in declaration order
    MatchedLabels []string  // labels of filters that matched (set via .WithLabel)
}
```

Auto-populated when the model carries `hydraide:"searchMeta"` and the read uses Vector/labelled filters. Works with `KeysOnly` reads as well.

### Payload struct (the `value` of a Catalog) — no `msgpack` tags

The HydrAIDE engine encodes payloads with msgpack and indexes them by Go field name. Tags are not required; the filter key is the field name.

### Type rules

Allowed: `string`, `bool`, `int8/16/32/64`, `uint8/16/32/64`, `float32/64`, `time.Time`, pointers to structs, slices and maps of the above.

Forbidden:

- `int` and `uint` without explicit size — always pick `int32`, `int64`, etc.
- `any` / `interface{}` without a concrete type.
- Function types.

---

## 9. Swamp registration (required)

Every model must register its Swamp pattern at application startup, before any read or write.

```go
func (m *OrderCatalog) RegisterPattern(r repo.Repo) error {
    ctx, cancel := hydraidehelper.CreateHydraContext()
    defer cancel()

    errs := r.GetHydraidego().RegisterSwamp(ctx, &hydraidego.RegisterSwampRequest{
        SwampPattern: name.New().
            Sanctuary("myapp").
            Realm("order-catalog").
            Swamp("*"),                            // wildcard for all Swamps in this realm
        CloseAfterIdle: time.Second * 120,         // evict from RAM after 2 min idle
        FilesystemSettings: &hydraidego.SwampFilesystemSettings{
            EncodingFormat: hydraidego.EncodingMsgPack, // ALWAYS MsgPack
            WriteInterval:  time.Second * 5,            // flush to disk every 5s
        },
    })

    if errs != nil && len(errs) > 0 {
        return errs[0]
    }
    return nil
}
```

**Always set `EncodingFormat: hydraidego.EncodingMsgPack`.** Server-side filtering on payload fields requires msgpack — GOB-encoded payloads are read-compatible but not filterable on inner fields.

Typical `CloseAfterIdle` values:

| Access pattern | `CloseAfterIdle` |
|---|---|
| Hot (frequent access, e.g. user profile) | 5–10 minutes |
| Warm (active conversations) | 1–2 minutes |
| Cold (rarely touched logs) | 30 seconds |
| Long-lived hot dataset | hours / days |

---

## 10. Encoding — always MsgPack

Why msgpack:

- **Server-side filtering.** The engine can extract a single field from a msgpack blob and filter on it without sending the whole record over the wire. GOB blobs cannot be filtered server-side.
- **Cross-language.** Any language with a msgpack library can read the payload.
- **Read-compatibility.** Old GOB-encoded payloads are still read correctly, but new writes should always use msgpack.

---

## 11. Server-side filters

### Scalar field filters

```go
// Numeric
hydraidego.FilterBytesFieldInt8(op, "Field", value)
hydraidego.FilterBytesFieldInt16(op, "Field", value)
hydraidego.FilterBytesFieldInt32(op, "Field", value)
hydraidego.FilterBytesFieldInt64(op, "Field", value)
hydraidego.FilterBytesFieldUint8(op, "Field", value)
hydraidego.FilterBytesFieldUint16(op, "Field", value)
hydraidego.FilterBytesFieldUint32(op, "Field", value)
hydraidego.FilterBytesFieldUint64(op, "Field", value)
hydraidego.FilterBytesFieldFloat32(op, "Field", value)
hydraidego.FilterBytesFieldFloat64(op, "Field", value)

// String / Bool
hydraidego.FilterBytesFieldString(op, "Field", "value")
hydraidego.FilterBytesFieldBool(op, "Field", true)

// Time (convenience wrapper — internally int64 Unix seconds, value.UTC().Unix())
hydraidego.FilterBytesFieldTime(op, "NextSendAt", time.Now())
// Use time.Time{} to filter on zero/empty time
```

### Metadata filters

```go
hydraidego.FilterCreatedAt(op, time.Now().Add(-24*time.Hour))
hydraidego.FilterUpdatedAt(op, time.Now().Add(-1*time.Hour))
hydraidego.FilterExpiredAt(op, time.Now())
```

### Slice filters

```go
// Element membership
hydraidego.FilterBytesFieldSliceContainsInt8("Tags", int8(7))
hydraidego.FilterBytesFieldSliceContainsInt32("CategoryIDs", int32(42))
hydraidego.FilterBytesFieldSliceContainsInt64("Timestamps", int64(1712534400))
hydraidego.FilterBytesFieldSliceContainsString("Tags", "premium")

// Negated membership
hydraidego.FilterBytesFieldSliceNotContainsInt8(...)
hydraidego.FilterBytesFieldSliceNotContainsInt32(...)
hydraidego.FilterBytesFieldSliceNotContainsInt64(...)
hydraidego.FilterBytesFieldSliceNotContainsString(...)

// Substring search inside a string slice (case-insensitive)
hydraidego.FilterBytesFieldSliceContainsSubstring("Tags", "premium-")
hydraidego.FilterBytesFieldSliceNotContainsSubstring("Tags", "banned-")

// Slice/map length
hydraidego.FilterBytesFieldSliceLen(hydraidego.GreaterThan, "Items", 0)
// internally a ".#len" pseudo-field
```

### IN filters (set membership)

Faster and more readable than chains of Equal + FilterOR.

```go
hydraidego.FilterBytesFieldStringIn("CampaignID", "camp-a", "camp-b", "camp-c")
hydraidego.FilterBytesFieldInt32In("Status", 1, 3, 5)
hydraidego.FilterBytesFieldInt64In("ScheduledAt", 1712534400, 1712620800)
```

### Nested-slice filters

For slices inside the payload that hold structs.

#### `Any` — at least one element matches one condition

```go
hydraidego.FilterBytesFieldNestedSliceAnyString("Contacts", "Email", hydraidego.IsNotEmpty, "")
hydraidego.FilterBytesFieldNestedSliceAnyInt8("Entries", "Status", hydraidego.Equal, 1)
hydraidego.FilterBytesFieldNestedSliceAnyBool("Items", "IsActive", hydraidego.Equal, true)
```

#### `Where` — at least one element where ALL conditions hold simultaneously

`FilterNestedSliceWhere` guarantees that all inner conditions are evaluated against the **same element** (multiple `Any` filters could match different elements).

```go
hydraidego.FilterNestedSliceWhere("CampaignEntries",
    hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 1),
    hydraidego.FilterBytesFieldStringIn("CampaignID", activeCampaignIDs...),
    hydraidego.FilterBytesFieldTime(hydraidego.LessThanOrEqual, "NextSendAt", time.Now()),
    hydraidego.FilterBytesFieldTime(hydraidego.GreaterThan, "NextSendAt", time.Time{}),
)
```

#### `All` — every element satisfies every condition

```go
hydraidego.FilterNestedSliceAll("Entries",
    hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 3),
)
// Empty slice: true (vacuous truth)
```

#### `None` — no element satisfies the conditions

```go
hydraidego.FilterNestedSliceNone("Entries",
    hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 1),
)
// Empty slice: true
```

#### `Count` — count matching elements with a relational operator

```go
hydraidego.FilterNestedSliceCount("Entries",
    hydraidego.GreaterThanOrEqual, 3,
    hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 1),
)
```

Common features of all nested-slice filters:

- Dot-notation paths to deeply nested slices: `"Outer.Inner.Items"`.
- `.WithLabel("name")` for label tracking in `SearchMeta`.
- `.ForKey("TreasureKey")` for Profile-mode filtering (see §13).
- Inner conditions can compose with `FilterOR` for per-element OR logic.

### Phrase search

Consecutive-word search inside a `map[string][]int` word-position index in the payload.

```go
// Match: words appear at consecutive positions
hydraidego.FilterPhrase("WordIndex", "general", "terms", "conditions")

// Negated
hydraidego.FilterNotPhrase("WordIndex", "secret", "clause")
```

- Case-sensitive exact match.
- The target field must be `map[string][]int`.

### Vector similarity (cosine)

```go
queryVec := hydraidego.NormalizeVector(rawVec)
hydraidego.FilterVector("Embedding", queryVec, 0.70) // min similarity 0.70

// Helpers
normalized := hydraidego.NormalizeVector(v) // L2 normalisation
score := hydraidego.CosineSimilarity(a, b)
```

Both stored vectors and the query vector must be L2-normalised.

### Geographic distance (Haversine)

```go
hydraidego.GeoDistance("Lat", "Lng", 47.497, 19.040, 50.0, hydraidego.GeoInside)   // within 50 km
hydraidego.GeoDistance("Lat", "Lng", 47.497, 19.040, 100.0, hydraidego.GeoOutside) // beyond 100 km
```

### AND / OR composition

```go
filters := hydraidego.FilterAND(
    hydraidego.FilterBytesFieldString(hydraidego.Equal, "Status", "active"),
    hydraidego.FilterBytesFieldInt32(hydraidego.GreaterThan, "Priority", 5),
)

filters := hydraidego.FilterOR(
    hydraidego.FilterBytesFieldString(hydraidego.Equal, "Category", "A"),
    hydraidego.FilterBytesFieldString(hydraidego.Equal, "Category", "B"),
)

// Nested
filters := hydraidego.FilterAND(
    hydraidego.FilterOR(
        hydraidego.FilterBytesFieldString(hydraidego.Equal, "Category", "A"),
        hydraidego.FilterBytesFieldString(hydraidego.Equal, "Category", "B"),
    ),
    hydraidego.FilterBytesFieldBool(hydraidego.Equal, "InStock", true),
)
```

### Labels and Profile-mode targeting

```go
// Label → appears in SearchMeta.MatchedLabels for matched records
hydraidego.FilterBytesFieldString(hydraidego.Contains, "Name", "hotel").WithLabel("has-hotel")

// Profile-mode: which Treasure inside the profile to evaluate
hydraidego.FilterBytesFieldBool(hydraidego.Equal, "IsActive", true).ForKey("Settings")
```

### Operators

```
Equal, NotEqual
GreaterThan, GreaterThanOrEqual, LessThan, LessThanOrEqual
Contains, NotContains            (string)
StartsWith, EndsWith             (string)
IsEmpty, IsNotEmpty
HasKey, HasNotKey                (map)
SliceContains, SliceNotContains
StringIn, Int32In, Int64In
```

---

## 12. Indexing and pagination

```go
index := &hydraidego.Index{
    IndexType:    hydraidego.IndexCreationTime, // Key, CreationTime, UpdateTime
    IndexOrder:   hydraidego.IndexOrderDesc,    // Asc or Desc
    From:         0,                             // offset (records to skip)
    Limit:        100,                           // pre-filter scan limit
    FromTime:     &startTime,                    // inclusive lower time bound
    ToTime:       &endTime,                      // exclusive upper time bound
    MaxResults:   20,                            // post-filter max returned
    ExcludeKeys:  []string{"k1", "k2"},          // pre-filter blacklist (O(1))
    IncludedKeys: []string{"k3"},                // pre-filter whitelist (O(1))
    KeysOnly:     true,                          // only Key + IsExist
}
```

Notes:

- **`ExcludeKeys`** runs before filters with O(1) lookup — ideal for "show me more" pagination without offsets, and for dedupe.
- **`IncludedKeys`** is a pre-filter whitelist. Order: `IncludedKeys → ExcludeKeys → Filters → Response`.
- **`KeysOnly`** is the fastest read mode (no value/metadata transport). `SearchMeta` still works — read keys with their match labels and vector scores, then `CatalogReadBatch` the top-N.

### Pagination via `ExcludeKeys`

```go
seen := []string{}
for page := 0; page < maxPages; page++ {
    index := &hydraidego.Index{
        IndexType:   hydraidego.IndexCreationTime,
        IndexOrder:  hydraidego.IndexOrderDesc,
        MaxResults:  20,
        ExcludeKeys: seen,
    }
    err := h.CatalogReadManyStream(ctx, swamp, index, filters, MyModel{},
        func(model any) error {
            m := model.(*MyModel)
            seen = append(seen, m.Key)
            // collect m
            return nil
        })
    if /* no more results */ {
        break
    }
}
```

### Two-phase read (`KeysOnly` + `ReadBatch`)

```go
// Phase 1: collect candidate keys quickly
var matched []string
index := &hydraidego.Index{
    IndexType:  hydraidego.IndexCreationTime,
    IndexOrder: hydraidego.IndexOrderDesc,
    MaxResults: 1000,
    KeysOnly:   true,
}
h.CatalogReadManyStream(ctx, swamp, index, filters, MyModel{},
    func(model any) error {
        matched = append(matched, model.(*MyModel).Key)
        return nil
    })

// Phase 2: full content for the top N
h.CatalogReadBatch(ctx, swamp, matched[:10], MyModel{},
    func(model any) error {
        // handle full record
        return nil
    })
```

---

## 13. Distributed locks

Cross-service synchronisation without an external broker.

```go
lockKey := fmt.Sprintf("user-balance-%s", userID)
lockID, err := h.Lock(ctx, lockKey, 5*time.Second) // TTL minimum 1001 ms
if err != nil {
    return fmt.Errorf("lock failed: %w", err)
}
defer func() {
    if unlockErr := h.Unlock(ctx, lockKey, lockID); unlockErr != nil {
        slog.Error("failed to unlock", "lockKey", lockKey, "error", unlockErr)
    }
}()

// === critical section ===
```

Lock semantics:

- **FIFO queue.** Waiters are served in arrival order.
- **TTL minimum 1001 ms.** The TTL bounds the worst-case held duration; a crashed holder releases the lock automatically when the TTL expires.
- **`lockID`.** Required for `Unlock`; it is the value returned by `Lock`.
- **Always `defer Unlock`.** Never leave a lock dangling.
- Works across processes and services that share the same HydrAIDE instance.

### Catalog-backed application lock

If you also want a record of who holds what (a "lock catalog" for audit), pair the lock primitive with a Catalog model:

```go
type LockCatalog struct {
    Key       string    `hydraide:"key"`
    Reference string    `hydraide:"value"`
    CreatedAt time.Time `hydraide:"createdAt"`
    CreatedBy string    `hydraide:"createdBy"`
    ExpireAt  time.Time `hydraide:"expireAt,omitempty"`
}
```

Use `CatalogShiftExpired` for periodic cleanup of orphaned locks; use `AreKeysExist` to check which keys are currently locked in a single round-trip.

---

## 14. Structural patches (atomic field-level mutation)

`CatalogPatch` mutates individual fields inside a msgpack-encoded Catalog Treasure on the server, atomically, without a read-modify-write round-trip from the client. This is the right tool when:

- You only need to change a few fields of a large payload.
- You need the change to be conditional (compare-and-swap style).
- You want incremental updates (counters, append to slices) without locking.

### Convenience entry points

```go
// Single field SET
status, err := h.CatalogPatchField(ctx, swamp, key, "Status", int8(2))

// Multiple field SETs in one round-trip
status, err := h.CatalogPatchFields(ctx, swamp, key, map[string]any{
    "Status":     int8(2),
    "AmountCent": int64(1990),
})

// Many keys in one RPC
err := h.CatalogPatchFieldsMany(ctx, swamp, requests,
    func(key string, status hydraidego.PatchStatus, errMsg string) error {
        return nil
    })
```

### Builder API (ordered ops, conditions, metadata)

`CatalogPatch` returns a `PatchBuilder`. Ops execute in declaration order; the patch is applied atomically per (Swamp, Key).

```go
status, err := h.CatalogPatch(ctx, swamp, key).
    Set("Status", int8(2)).
    Inc("AmountCent", int64(500)).
    Append("History", auditEntry).
    IfFieldEquals("Version", int32(7)).        // CAS precondition
    WithUpdatedAt().
    WithUpdatedBy("worker-42").
    Exec()
```

Available ops on `PatchBuilder`:

| Op | Effect |
|---|---|
| `Set(path, value)` | Assigns `value` at the given path. |
| `Delete(path)` | Removes the field (or map key) at the path. |
| `Inc(path, delta)` | Atomic numeric increment (works on int*/uint*/float* types). |
| `Append(path, value)` | Appends to a slice at the path. |
| `Prepend(path, value)` | Prepends to a slice. |
| `RemoveAt(path)` | Removes a slice element by index in the path. |
| `RemoveVal(path, value)` | Removes a matching element from a slice. |
| `Merge(path, value)` | Merges a struct/map into the existing value. |
| `NoCreate()` | Patch does not create a missing Treasure (returns `PatchStatusKeyNotFound` instead). |

Conditions (precondition for the whole patch):

| Condition | Effect |
|---|---|
| `IfFieldEquals(path, v)` | Only apply if `path == v`. |
| `IfFieldNotEquals(path, v)` | Only apply if `path != v`. |
| `IfFieldGreaterThan(path, v)` / `IfFieldGreaterThanOrEqual` / `IfFieldLessThan` / `IfFieldLessThanOrEqual` | Numeric comparisons. |
| `IfFieldExists(path)` / `IfFieldNotExists(path)` | Existence check. |

Metadata helpers:

- `WithUpdatedAt()` — server stamps the patched Treasure's `ModifiedAt` to now.
- `WithUpdatedBy(userID)` — server stamps `ModifiedBy`.

### Patch result codes

```go
const (
    PatchStatusPatched              // ops applied to existing treasure
    PatchStatusCreated              // CreateIfNotExist=true, treasure created
    PatchStatusKeyNotFound          // missing and creation suppressed (NoCreate)
    PatchStatusConditionNotMet      // precondition was false
    PatchStatusFieldNotFound        // reserved
    PatchStatusTypeMismatch         // op or condition crossed types
    PatchStatusPathInvalid          // malformed path
    PatchStatusEncodingNotSupported // existing treasure isn't msgpack-encoded
    PatchStatusInternalError        // unexpected server failure
)
```

When you depend on the result, check `status` before checking `err`:

```go
status, err := h.CatalogPatch(ctx, swamp, key).
    Inc("Credits", int64(-cost)).
    IfFieldGreaterThanOrEqual("Credits", int64(cost)).
    Exec()

switch {
case err != nil:
    return err
case status == hydraidego.PatchStatusConditionNotMet:
    return ErrInsufficientCredits
case status == hydraidego.PatchStatusPatched:
    // ok
}
```

### When NOT to use Patch

- **You need cross-key atomicity.** Patches are atomic per (Swamp, Key); they do not span keys. For multi-record atomic updates, use a [distributed lock](#13-distributed-locks).
- **The Treasure is GOB-encoded.** Patch requires msgpack. Migrate the model to msgpack first (`EncodingFormat: hydraidego.EncodingMsgPack` + a CompactSwamp to rewrite the file).
- **You need to read-then-decide on the same record.** Use `CatalogRead` (or a lock + Save). Patch is for mutations expressed as a fixed sequence of ops.

---

## 15. Critical rules and pitfalls

### `createdAt` must always be set

If your model declares `hydraide:"createdAt"` and you save with a zero-value time, **the server silently drops the write** — no error.

```go
// WRONG — silently dropped
e := &MyModel{Key: "x", Payload: &Payload{...}} // CreatedAt zero

// CORRECT
e := &MyModel{
    Key:       "x",
    Payload:   &Payload{...},
    CreatedAt: time.Now().UTC(),
}
```

### Never use bare `int` or `uint`

Use explicit-size types. Bare `int`/`uint` causes runtime errors and cross-platform inconsistency.

```go
type Payload struct {
    Count int32 // not `int`
    Big   int64
}
```

### Batch over loops

```go
// WRONG — N round-trips
for _, k := range keys {
    h.CatalogRead(ctx, swamp, k, &m)
}

// CORRECT — 1 round-trip
h.CatalogReadBatch(ctx, swamp, keys, MyModel{}, iter)

// WRONG — N round-trips for existence
for _, k := range keys {
    h.IsKeyExists(ctx, swamp, k)
}

// CORRECT — 1 round-trip
h.AreKeysExist(ctx, swamp, keys)
```

### Always use a context timeout

```go
// Default (~30s)
ctx, cancel := hydraidehelper.CreateHydraContext()
defer cancel()

// Long batch
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
defer cancel()
```

### Error checks: `IsSwampNotFound` AND `IsNotFound`

```go
if err := h.CatalogRead(ctx, n, key, m); err != nil {
    if hydraidego.IsSwampNotFound(err) || hydraidego.IsNotFound(err) {
        return nil // not found is not an error
    }
    return err
}
```

### Register patterns at startup

`RegisterPattern()` must run during service start, before any read/write touches the corresponding Swamp.

### `WriteInterval` does not affect reads

`WriteInterval` controls how often dirty Treasures are flushed to disk. Reads always serve from memory — they do not wait for a flush.

### Atomic shift patterns — never read-then-shift without a lock

```go
// WRONG — race, possible data loss
keys := readMany(swamp)            // T1: keys = [A, B]
                                    // T2: another goroutine writes [C, D]
shiftBatch(swamp, keys)             // T3: pulls A, B — D, C may be lost on the next sweep

// CORRECT (1) — atomic shift
h.CatalogShiftExpired(ctx, swamp, n, MyModel{}, iter)

// CORRECT (2) — guard with a lock
lockID, _ := h.Lock(ctx, lockKey, 5*time.Second)
defer h.Unlock(ctx, lockKey, lockID)
keys := readManyStream(swamp, filter)
shiftBatch(swamp, keys)
```

### `ExpireAt` clock skew — give "already expired" a one-minute past margin

`CatalogShiftExpired` compares `ExpireAt` against the **server's clock**, not the client's. In a distributed deployment (API + workers + HydrAIDE on different hosts), client/server clock skew is a normal NTP-driven oscillation, typically 100 ms – 2 s.

If you want a record to be picked up on the **next** `ShiftExpired` (queue-flow: save, then drain immediately), use **at least one minute of past margin**, not `-1s` or `-100ms`.

```go
// WRONG — flaky under NTP skew
m.ExpireAt = time.Now().UTC().Add(-1 * time.Second)
m.ExpireAt = time.Now().UTC().Add(-100 * time.Millisecond)
m.ExpireAt = now // 0 margin — guaranteed flake

// CORRECT — safe margin
m.ExpireAt = time.Now().UTC().Add(-1 * time.Minute)
```

Symptoms that point to clock skew:

- Local single-host tests pass 100%.
- Multi-host or production runs are intermittently flaky.
- Stable for a while after a server restart, drifts later (NTP corrections).
- Failure is time-shaped (clusters), not cold-start-shaped.

Quick decision table:

| Intent | `ExpireAt` |
|---|---|
| "Already expired" / drain on next sweep | `now.Add(-1 * time.Minute)` |
| "Available after a cooldown" | `now.Add(cooldown)` |
| Explicit TTL | `now.Add(ttl)` |
| No expiration | `time.Time{}` (zero) |

---

## 16. Testing patterns

Run integration tests against a real HydrAIDE test instance — do not mock the engine. Mocked tests give you false confidence; the wire shape, encoding, filter semantics, and clock interactions are exactly what you need to exercise.

A typical structure:

```go
type OrderCatalogTestSuite struct {
    // your test harness that connects to a real HydrAIDE instance
    suite.Suite
    repo repo.Repo
}

func (s *OrderCatalogTestSuite) SetupSuite() {
    // connect to your HydrAIDE test instance here
    s.repo = ...

    // REQUIRED: register the pattern before any test runs
    s.NoError((&OrderCatalog{}).RegisterPattern(s.repo))
}

func (s *OrderCatalogTestSuite) TearDownTest() {
    // clean up the test Swamp(s) so each test starts fresh
    _ = s.repo.GetHydraidego().Destroy(context.Background(),
        name.New().Sanctuary("myapp").Realm("order-catalog").Swamp("test-tenant"))
}

func TestOrderCatalogTestSuite(t *testing.T) {
    suite.Run(t, new(OrderCatalogTestSuite))
}
```

Recommended:

- One test instance per test package (parallelism vs. shared-Swamp clashes).
- `TearDownTest` destroys the Swamps your test touched.
- Assert on real reads, not on mocked SDK return values.
- Time-based tests use injected clocks or accept the one-minute clock-skew margin (see §15).

---

## 17. Designing a new model — checklist

Before writing code, decide:

1. **Profile or Catalog?** One entity per Swamp → Profile. Many keyed records → Catalog.
2. **Sharding strategy.** What goes in the Swamp identifier? Per-tenant, per-user, per-domain, compound key.
3. **Filtering needs.** Server-side filters on payload fields → use Go field names directly. Sorting → Index. Pagination → `ExcludeKeys` + `MaxResults`. Nested struct lists → `NestedSliceWhere/All/None/Count`. Set membership → `*In` filters.
4. **TTL?** `expireAt` tag + `CatalogShiftExpired`. Cleanup is not automatic — you call it.
5. **Counters?** `Increment*` ops with optional condition + metadata. No Catalog model required.
6. **Set / inverted index?** `Uint32Slice` ops (push/delete/size/exists). Auto-deduped, auto-GCed.
7. **Cross-service synchronisation?** Distributed lock (§13).
8. **Real-time notification?** `Subscribe` (§6).
9. **Field-level updates on large payloads?** `CatalogPatch` (§14).

Per-model deliverables:

- [ ] `model_*_profile.go` or `model_*_catalog.go`
- [ ] `model_*_test.go` covering Save / Load / list / filter / TTL paths
- [ ] `hydraide` tags on top-level fields (`key`, `value`, `createdAt`, etc.)
- [ ] No `msgpack` tags inside the payload struct
- [ ] No bare `int` / `uint` — explicit sizes only
- [ ] `CreatedAt` always set before save (`time.Now().UTC()`)
- [ ] `RegisterPattern()` with `EncodingFormat: hydraidego.EncodingMsgPack`
- [ ] `Destroy()` helper for tests and admin paths
- [ ] `name()` / `createName()` helper
- [ ] Error handling that treats `IsSwampNotFound` / `IsNotFound` as "not found, not an error"
- [ ] `slog` logging in every error branch
- [ ] Context with timeout
- [ ] Test suite that connects to a real HydrAIDE instance and `TearDownTest` cleanup
- [ ] Batch ops everywhere a loop would otherwise issue many round-trips
- [ ] `SearchMeta` field if vector or labelled filters are used

---

## 18. Operations — see the `hydraidectl` skill

For installing, upgrading, backing up, restoring, migrating, inspecting, and observing HydrAIDE instances, use the dedicated [`hydraidectl` skill](../hydraidectl/SKILL.md). It covers every CLI subcommand, common workflows, operational rules (e.g. stop clients before upgrade), and troubleshooting.

---

## 19. Where to look in this repo

| What you want | Where |
|---|---|
| Wire protocol (source of truth) | [`proto/hydraide.proto`](../../../proto/hydraide.proto) |
| Go SDK | [`sdk/go/hydraidego/`](../../../sdk/go/hydraidego/) |
| Patch SDK | [`sdk/go/hydraidego/hydraidego_patch.go`](../../../sdk/go/hydraidego/hydraidego_patch.go) |
| Storage engine internals | [`app/core/hydra/swamp/chronicler/v2/`](../../../app/core/hydra/swamp/chronicler/v2/) |
| Patch primitive | [`app/core/hydra/swamp/treasure/msgpackpatch/`](../../../app/core/hydra/swamp/treasure/msgpackpatch/) |
| Filters server-side | [`app/server/gateway/filter*.go`](../../../app/server/gateway/) |
| Conventions | [`CLAUDE.md`](../../../CLAUDE.md) |
| Feature docs | [`docs/features/`](../../../docs/features/) |
| Benchmarks | [`docs/benchmarks/`](../../../docs/benchmarks/) |
