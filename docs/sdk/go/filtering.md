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

## Complete Example

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
```
