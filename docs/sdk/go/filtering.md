# HydrAIDE Go SDK — Filter System Reference

Server-side filtering allows HydrAIDE to evaluate conditions in memory before sending data over gRPC, eliminating unnecessary network overhead.

All filter types implement the `FilterItem` interface and can be combined with `FilterAND` / `FilterOR`.

---

## Combining Filters

```go
// AND: all conditions must be true
filters := hydraidego.FilterAND(filter1, filter2, filter3)

// OR: at least one condition must be true
filters := hydraidego.FilterOR(filter1, filter2)

// Nested: (price > 100 AND (status == "active" OR status == "pending"))
filters := hydraidego.FilterAND(
    hydraidego.FilterBytesFieldFloat64(hydraidego.GreaterThan, "Price", 100.0),
    hydraidego.FilterOR(
        hydraidego.FilterBytesFieldString(hydraidego.Equal, "Status", "active"),
        hydraidego.FilterBytesFieldString(hydraidego.Equal, "Status", "pending"),
    ),
)
```

---

## Operators

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

## Basic Filters

Direct comparison on Treasure content value.

```go
hydraidego.FilterInt8(hydraidego.Equal, int8(42))
hydraidego.FilterInt32(hydraidego.GreaterThan, 100)
hydraidego.FilterInt64(hydraidego.LessThan, int64(999))
hydraidego.FilterFloat32(hydraidego.GreaterThanOrEqual, float32(3.14))
hydraidego.FilterFloat64(hydraidego.LessThanOrEqual, 99.99)
hydraidego.FilterString(hydraidego.Contains, "hello")
hydraidego.FilterBool(hydraidego.Equal, true)
```

## Timestamp Filters

```go
hydraidego.FilterCreatedAt(hydraidego.GreaterThan, time.Now().Add(-24*time.Hour))
hydraidego.FilterUpdatedAt(hydraidego.LessThan, time.Now())
hydraidego.FilterExpiredAt(hydraidego.LessThan, time.Now())
```

---

## BytesField Filters

When Treasure content is MsgPack-encoded (`BytesVal`), you can filter on fields inside
the encoded struct using dot-separated paths.

```go
// Simple field
hydraidego.FilterBytesFieldString(hydraidego.Equal, "Status", "active")

// Nested field
hydraidego.FilterBytesFieldFloat64(hydraidego.GreaterThan, "Address.Lat", 47.0)

// Map key existence
hydraidego.FilterBytesFieldString(hydraidego.HasKey, "Metadata", "email")
hydraidego.FilterBytesFieldString(hydraidego.HasNotKey, "Metadata", "phone")
```

Available types: `FilterBytesFieldInt8`, `Int16`, `Int32`, `Int64`, `Uint8`, `Uint16`,
`Uint32`, `Uint64`, `Float32`, `Float64`, `String`, `Bool`.

---

## Slice Filters

Check if a slice field contains (or does not contain) a specific value.

### SliceContains — Exact match in slice

```go
// Does LLMSiteFunctions contain 7 (Booking)?
hydraidego.FilterBytesFieldSliceContainsInt8("LLMSiteFunctions", int8(7))

// Does PaymentProviders contain "Barion"? (case-sensitive exact match)
hydraidego.FilterBytesFieldSliceContainsString("PaymentProviders", "Barion")

// Also available:
hydraidego.FilterBytesFieldSliceContainsInt32("Field", int32(100))
hydraidego.FilterBytesFieldSliceContainsInt64("Field", int64(999))
```

### SliceNotContains — Negated exact match

```go
// Exclude e-commerce sites (SiteFunctions does NOT contain 1)
hydraidego.FilterBytesFieldSliceNotContainsInt8("LLMSiteFunctions", int8(1))

// Does NOT accept Stripe
hydraidego.FilterBytesFieldSliceNotContainsString("PaymentProviders", "Stripe")
```

### SliceContainsSubstring — Case-insensitive substring in string slices

```go
// Any activity contains "tattoo"? (matches "custom tattoo design", "Tattoo Art", etc.)
hydraidego.FilterBytesFieldSliceContainsSubstring("LLMDetailedActivities", "tattoo")

// No activity contains "permanent makeup"
hydraidego.FilterBytesFieldSliceNotContainsSubstring("LLMDetailedActivities", "permanent makeup")
```

### Combining slice filters

```go
// Tattoo parlors excluding permanent makeup
hydraidego.FilterAND(
    hydraidego.FilterBytesFieldSliceContainsSubstring("LLMDetailedActivities", "tattoo"),
    hydraidego.FilterBytesFieldSliceNotContainsSubstring("LLMDetailedActivities", "permanent makeup"),
)

// Hospitality OR Health industry
hydraidego.FilterOR(
    hydraidego.FilterBytesFieldSliceContainsInt8("LLMIndustrySectors", int8(1)),
    hydraidego.FilterBytesFieldSliceContainsInt8("LLMIndustrySectors", int8(6)),
)
```

---

## Slice Length (#len)

Check the length of a slice or map using the `#len` pseudo-field.

```go
// Has at least 1 contact
hydraidego.FilterBytesFieldSliceLen(hydraidego.GreaterThan, "LLMContacts", 0)

// Exactly 3 industry sectors
hydraidego.FilterBytesFieldSliceLen(hydraidego.Equal, "LLMIndustrySectors", 3)

// No product categories (empty slice)
hydraidego.FilterBytesFieldSliceLen(hydraidego.Equal, "LLMProductCategories", 0)

// Works with maps too: metadata has fewer than 5 keys
hydraidego.FilterBytesFieldSliceLen(hydraidego.LessThan, "Metadata", 5)
```

Internally, `FilterBytesFieldSliceLen` appends `.#len` to the field path, so
`"LLMContacts"` becomes `"LLMContacts.#len"` and returns the length as an integer
that can be compared with any standard operator.

---

## Nested Slice Any ([*] wildcard)

Check if ANY element in a struct slice has a field matching a condition.

```go
// At least 1 contact has a non-empty email
hydraidego.FilterBytesFieldNestedSliceAnyString("LLMContacts", "Email", hydraidego.IsNotEmpty, "")

// At least 1 contact is a CEO
hydraidego.FilterBytesFieldNestedSliceAnyString("LLMContacts", "Role", hydraidego.Equal, "CEO")

// At least 1 contact has a domain-matching email
hydraidego.FilterBytesFieldNestedSliceAnyBool("LLMContacts", "IsDomainMatch", hydraidego.Equal, true)

// At least 1 contact email contains "@company.com"
hydraidego.FilterBytesFieldNestedSliceAnyString("LLMContacts", "Email", hydraidego.Contains, "@company.com")
```

Internally, these constructors build a path like `"LLMContacts[*].Email"`.
The server iterates over each element in the slice and returns true if ANY element's
field matches the operator.

Available types: `FilterBytesFieldNestedSliceAnyString`, `NestedSliceAnyInt8`, `NestedSliceAnyBool`.

> **Limitation:** `NestedSliceAny` checks ONE condition per filter. When multiple `NestedSliceAny`
> filters are AND-ed, each may match a DIFFERENT element. To require that the SAME element
> satisfies ALL conditions, use `FilterNestedSliceWhere` (see below).

---

## IN Filters — Set Membership

Check if a field value is a member of a set of allowed values. More efficient and
readable than chaining multiple `Equal` conditions with `FilterOR`.

```go
// String IN: CampaignID is one of the active campaigns
hydraidego.FilterBytesFieldStringIn("CampaignID", "camp-abc", "camp-def", "camp-ghi")

// Int32 IN: Status is Active(1) or Finished(3)
hydraidego.FilterBytesFieldInt32In("Status", 1, 3)

// Int64 IN: Timestamp matches one of the scheduled times
hydraidego.FilterBytesFieldInt64In("ScheduledAt", 1712534400, 1712620800, 1712707200)
```

IN filters work with `[*]` wildcard paths (any element match) and inside
`FilterNestedSliceWhere` conditions.

```go
// Combined: find domains where ANY contact has Role in {"CEO", "CTO", "CFO"}
hydraidego.FilterAND(
    hydraidego.FilterBytesFieldSliceLen(hydraidego.GreaterThan, "LLMContacts", 0),
    hydraidego.FilterBytesFieldStringIn("LLMContacts[*].Role", "CEO", "CTO", "CFO"),
)
```

---

## Time Convenience Filter

`time.Time` fields are stored as `int64` Unix seconds in MessagePack. The `FilterBytesFieldTime`
wrapper handles the conversion automatically:

```go
// NextSendAt <= now (ready to send)
hydraidego.FilterBytesFieldTime(hydraidego.LessThanOrEqual, "NextSendAt", time.Now())

// CreatedAt > 24 hours ago
hydraidego.FilterBytesFieldTime(hydraidego.GreaterThan, "CreatedAt", time.Now().Add(-24*time.Hour))

// Exclude zero-time entries (NextSendAt is set)
hydraidego.FilterBytesFieldTime(hydraidego.GreaterThan, "NextSendAt", time.Time{})
```

Internally this is equivalent to `FilterBytesFieldInt64(op, path, value.UTC().Unix())`.

---

## Nested Slice Where — Multi-Condition Element Matching

Check conditions against EACH element in a struct slice individually. Unlike `NestedSliceAny`
(which tests ONE condition per filter), these filters guarantee that the SAME element
satisfies ALL conditions simultaneously.

Four modes are available:

| Constructor | Semantics |
|-------------|-----------|
| `FilterNestedSliceWhere(path, conditions...)` | At least ONE element satisfies ALL conditions |
| `FilterNestedSliceAll(path, conditions...)` | EVERY element satisfies ALL conditions |
| `FilterNestedSliceNone(path, conditions...)` | NO element satisfies ALL conditions |
| `FilterNestedSliceCount(path, op, count, conditions...)` | Count matching elements, compare with operator |

### FilterNestedSliceWhere (ANY / WHERE)

"Is there at least ONE element where ALL conditions are true simultaneously?"

```go
// Find domains where at least one CampaignEntry is:
//   Active (Status=1) AND in one of our campaigns AND ready to send
filters := hydraidego.FilterAND(
    hydraidego.FilterNestedSliceWhere("CampaignEntries",
        hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 1),
        hydraidego.FilterBytesFieldStringIn("CampaignID", activeCampaignIDs...),
        hydraidego.FilterBytesFieldTime(hydraidego.LessThanOrEqual, "NextSendAt", time.Now()),
        hydraidego.FilterBytesFieldTime(hydraidego.GreaterThan, "NextSendAt", time.Time{}),
    ),
)
```

**Why this matters:** Without `FilterNestedSliceWhere`, AND-ing three separate
`NestedSliceAny` filters would match even if Status=1 is on element[0],
CampaignID matches on element[1], and NextSendAt on element[2].
`FilterNestedSliceWhere` guarantees all conditions match on the SAME element.

### FilterNestedSliceAll

"Does EVERY element satisfy ALL conditions?"

```go
// All campaign entries are finished
filters := hydraidego.FilterAND(
    hydraidego.FilterNestedSliceAll("CampaignEntries",
        hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 3), // Finished
    ),
)
```

Empty slice → `true` (vacuous truth: "every element satisfies" when there are no elements).

### FilterNestedSliceNone

"Does NO element satisfy ALL conditions?"

```go
// No campaign entry is active
filters := hydraidego.FilterAND(
    hydraidego.FilterNestedSliceNone("CampaignEntries",
        hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 1), // Active
    ),
)
```

Empty slice → `true` (no elements can satisfy anything).

### FilterNestedSliceCount

"How many elements satisfy ALL conditions? Compare against a threshold."

```go
// At least 3 active campaign entries
filters := hydraidego.FilterAND(
    hydraidego.FilterNestedSliceCount("CampaignEntries",
        hydraidego.GreaterThanOrEqual, 3,
        hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 1),
    ),
)

// Exactly 0 excluded entries (no exclusions)
filters := hydraidego.FilterAND(
    hydraidego.FilterNestedSliceCount("CampaignEntries",
        hydraidego.Equal, 0,
        hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 2), // Excluded
    ),
)
```

### Complex conditions with OR logic

Conditions accept any `FilterItem` — including `FilterOR` for per-element OR logic:

```go
// Find where at least one entry is Active AND in campaign-1 OR campaign-2
filters := hydraidego.FilterAND(
    hydraidego.FilterNestedSliceWhere("CampaignEntries",
        hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 1),
        hydraidego.FilterOR(
            hydraidego.FilterBytesFieldString(hydraidego.Equal, "CampaignID", "camp-1"),
            hydraidego.FilterBytesFieldString(hydraidego.Equal, "CampaignID", "camp-2"),
        ),
    ),
)
```

### Nested path (dot-separated)

SlicePath supports dot-separated navigation for deeply nested slices:

```go
// Slice at Outer.Inner.Items
hydraidego.FilterNestedSliceWhere("Outer.Inner.Items",
    hydraidego.FilterBytesFieldString(hydraidego.Equal, "Name", "target"),
)
```

### Labels and profile mode

```go
// With label tracking
hydraidego.FilterNestedSliceWhere("CampaignEntries",
    hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 1),
).WithLabel("has-active-campaign")

// Profile mode
hydraidego.FilterNestedSliceWhere("CampaignEntries",
    hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 1),
).ForKey("CampaignData")
```

### Edge cases

| Scenario | WHERE (ANY) | ALL | NONE | COUNT |
|----------|:-----------:|:---:|:----:|:-----:|
| Empty slice | `false` | `true` | `true` | compare 0 |
| Missing field | `false` | `true` | `true` | compare 0 |
| Nil elements in slice | skipped | fail | skipped | not counted |
| No conditions | `true` | `true` | `false` | compare len(slice) |

---

## Phrase Search

Search for consecutive words within a word-index map (`map[string][]int`).

```go
// Find documents containing the exact phrase
hydraidego.FilterPhrase("WordIndex", "altalanos", "szerzodesi", "feltetelek")

// Exclude documents containing the phrase
hydraidego.FilterNotPhrase("WordIndex", "titkos", "zaradek")
```

---

## Vector Similarity

Cosine similarity search on float32 vectors stored in BytesVal.

```go
queryVec := hydraidego.NormalizeVector([]float32{0.1, 0.2, 0.3, ...})
hydraidego.FilterVector("Embedding", queryVec, 0.8) // min similarity threshold
```

---

## Geographic Distance

Haversine-formula distance filtering with bounding box pre-filter.

```go
// Within 50km of Budapest
hydraidego.GeoDistance("Lat", "Lng", 47.497, 19.040, 50.0, hydraidego.GeoInside)

// Outside 100km of Budapest
hydraidego.GeoDistance("Lat", "Lng", 47.497, 19.040, 100.0, hydraidego.GeoOutside)
```

---

## Profile Mode (ForKey)

In profile mode, each struct field is stored as a separate Treasure. Use `.ForKey()` to
target a specific Treasure key:

```go
hydraidego.FilterInt32(hydraidego.GreaterThan, 18).ForKey("Age")
hydraidego.FilterString(hydraidego.Equal, "active").ForKey("Status")
hydraidego.FilterPhrase("WordIndex", "hello", "world").ForKey("WordIndex")
```

---

## ExcludeKeys — Server-Side Key Exclusion

Prevents specified keys from appearing in search results. Runs **before** filter
evaluation for maximum performance (O(1) map lookup per treasure, ~10ns).

Use cases:
- **Pagination without offset:** exclude already-seen keys on subsequent pages
- **Deduplication:** exclude keys already shown from other sources
- **"Show more" patterns:** fetch next batch excluding previous results

```go
// First page: no exclusions
index := &hydraidego.Index{
    IndexType:  hydraidego.IndexCreationTime,
    IndexOrder: hydraidego.IndexOrderDesc,
    MaxResults: 10,
}

// Second page: exclude first page results
index := &hydraidego.Index{
    IndexType:   hydraidego.IndexCreationTime,
    IndexOrder:  hydraidego.IndexOrderDesc,
    MaxResults:  10,
    ExcludeKeys: []string{"domain1.com", "domain2.com", "domain3.com"},
}
```

Works with `CatalogReadManyStream`, `CatalogReadManyFromMany`, and `CatalogReadMany`.
Can be combined with IncludedKeys, filters, MaxResults, and all other parameters.

---

## IncludedKeys — Server-Side Key Whitelist

Restricts search results to only the specified keys. When set, a treasure must
have its key in the IncludedKeys list to be eligible for filter evaluation.
Runs before ExcludeKeys and filters (O(1) map lookup per treasure).

Execution order: **IncludedKeys -> ExcludeKeys -> Filters -> Response**

Use cases:
- **Subset search:** filter within a pre-computed candidate list
- **Re-validation:** re-check a known set of keys against updated filters
- **User selections:** search only within user-selected domains

```go
// Search only within these candidate domains
index := &hydraidego.Index{
    IndexType:    hydraidego.IndexCreationTime,
    IndexOrder:   hydraidego.IndexOrderDesc,
    IncludedKeys: []string{"domain1.com", "domain2.com", "domain3.com"},
}

// Combined: search within candidates, excluding already-seen
index := &hydraidego.Index{
    IndexType:    hydraidego.IndexCreationTime,
    IndexOrder:   hydraidego.IndexOrderDesc,
    IncludedKeys: candidateKeys,
    ExcludeKeys:  alreadySeenKeys,
    MaxResults:   10,
}
```

---

## KeysOnly — Lightweight Key-Only Results

Returns only treasure keys (Key + IsExist) in the response, skipping all content
serialization. Reduces gRPC payload size dramatically (~16x faster than full conversion).

Use cases:
- **Large result sets:** discover 1000+ matching keys without content overhead
- **Two-phase search:** KeysOnly for discovery, then CatalogReadBatch for selected keys
- **Counting with key tracking:** know which keys matched, not just how many

```go
// Phase 1: discover matching keys (lightweight, ~17ns per treasure)
index := &hydraidego.Index{
    IndexType:  hydraidego.IndexCreationTime,
    IndexOrder: hydraidego.IndexOrderDesc,
    MaxResults: 1000,
    KeysOnly:   true,
}

var matchedKeys []string
h.CatalogReadManyStream(ctx, swamp, index, filters, Model{}, func(model any) error {
    m := model.(*Model)
    matchedKeys = append(matchedKeys, m.Key)
    return nil
})

// Phase 2: fetch full content for top 10 results
h.CatalogReadBatch(ctx, swamp, matchedKeys[:10], Model{}, func(model any) error { ... })
```

---

## SearchResultMeta — Scores and Match Labels

When filters have labels or VectorFilters are used, each streaming result includes
a `SearchMeta` with vector similarity scores and matched filter labels.

This works in all modes including KeysOnly — you get metadata without content overhead.

### Adding labels to filters

```go
hydraidego.FilterBytesFieldSliceContainsInt8("LLMSiteFunctions", int8(7)).WithLabel("booking")
hydraidego.FilterBytesFieldSliceContainsInt8("LLMIndustrySectors", int8(6)).WithLabel("health")
hydraidego.FilterVector("Embedding", queryVec, 0.5).WithLabel("semantic")
hydraidego.GeoDistance("Lat", "Lng", 47.497, 19.040, 50.0, hydraidego.GeoInside).WithLabel("location")
hydraidego.FilterPhrase("WordIndex", "hello", "world").WithLabel("phrase")
```

Unlabeled filters work normally but don't appear in `MatchedLabels`.
VectorFilter scores are always captured regardless of labels.

### Reading metadata via searchMeta tag

Add a `*hydraidego.SearchMeta` field with the `hydraide:"searchMeta"` tag to your model.
It is automatically populated during search/read responses. This field is **read-only**:
it is never processed during write operations (Set, Create, Update).

```go
type MyModel struct {
    Domain  string                  `hydraide:"key"`
    Payload []byte                  `hydraide:"value"`
    Meta    *hydraidego.SearchMeta  `hydraide:"searchMeta"` // auto-populated on read
}

h.CatalogReadManyStream(ctx, swamp, index, filters, MyModel{},
    func(model any) error {
        m := model.(*MyModel)
        fmt.Printf("Key: %s\n", m.Domain)
        if m.Meta != nil {
            if len(m.Meta.VectorScores) > 0 {
                fmt.Printf("  Relevance: %.2f\n", m.Meta.VectorScores[0])
            }
            fmt.Printf("  Matched: %v\n", m.Meta.MatchedLabels)
        }
        return nil
    })
```

### KeysOnly with metadata

SearchMeta is populated even in KeysOnly mode — scores and labels without content:
```go
index := &hydraidego.Index{
    MaxResults: 100,
    KeysOnly:   true,
}
// model.Meta.VectorScores and model.Meta.MatchedLabels are still populated
```

### OR groups with labels

In OR groups, all matching branches are reported (not just the first):

```go
filters := hydraidego.FilterOR(
    hydraidego.FilterBytesFieldSliceContainsInt8("LLMIndustrySectors", int8(1)).WithLabel("hospitality"),
    hydraidego.FilterBytesFieldSliceContainsInt8("LLMIndustrySectors", int8(6)).WithLabel("health"),
)
// If a domain has both sectors 1 and 6:
// meta.MatchedLabels = ["hospitality", "health"]
// If it only has sector 6:
// meta.MatchedLabels = ["health"]
```

### Performance

- **Without labels/vectors (fast path):** zero overhead, same as before
- **With meta:** ~35% overhead for score capture + label tracking (~1.8us vs ~1.3us)
- **Fast-path detection:** automatic via `hasAnyLabels` — no opt-in needed

---

## Complete Examples

```go
// Complex search: Booking sites in Budapest area with Barion, contacts, no permanent makeup
filters := hydraidego.FilterAND(
    hydraidego.FilterBytesFieldSliceContainsInt8("LLMSiteFunctions", int8(7)),
    hydraidego.FilterBytesFieldSliceContainsString("PaymentProviders", "Barion"),
    hydraidego.FilterBytesFieldSliceNotContainsSubstring("LLMDetailedActivities", "permanent makeup"),
    hydraidego.FilterBytesFieldSliceLen(hydraidego.GreaterThan, "LLMContacts", 0),
    hydraidego.FilterBytesFieldNestedSliceAnyString("LLMContacts", "Email", hydraidego.IsNotEmpty, ""),
    hydraidego.GeoDistance("Lat", "Lng", 47.497, 19.040, 50.0, hydraidego.GeoInside),
)

// Worker query: domains ready to send in active campaigns
activeCampaignIDs := []string{"camp-1", "camp-2", "camp-3"}
filters = hydraidego.FilterAND(
    hydraidego.FilterNestedSliceWhere("CampaignEntries",
        hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 1),
        hydraidego.FilterBytesFieldStringIn("CampaignID", activeCampaignIDs...),
        hydraidego.FilterBytesFieldTime(hydraidego.LessThanOrEqual, "NextSendAt", time.Now()),
        hydraidego.FilterBytesFieldTime(hydraidego.GreaterThan, "NextSendAt", time.Time{}),
    ),
)

// Dashboard: count active domains per campaign (KeysOnly for performance)
filters = hydraidego.FilterAND(
    hydraidego.FilterNestedSliceWhere("CampaignEntries",
        hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 1),
        hydraidego.FilterBytesFieldString(hydraidego.Equal, "CampaignID", campaignID),
    ),
)

// Campaign deletion: find domains to update
filters = hydraidego.FilterAND(
    hydraidego.FilterNestedSliceWhere("CampaignEntries",
        hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 1),
        hydraidego.FilterBytesFieldString(hydraidego.Equal, "CampaignID", deletedCampaignID),
    ),
)
```
