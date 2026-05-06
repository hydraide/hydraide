## Query engine — server-side filters over a streaming gRPC API

The struct-first model gives you direct save/read on a single record. The query engine is what you reach for when you need to look at many records through a lens — "all active subscriptions in tenant X", "documents semantically near this query vector", "places within 50 km of Budapest".

Filters run **server-side**. Matching Treasures are streamed back one by one over gRPC. There is no SQL surface — filters are composed as protobuf messages, which the SDKs wrap in fluent builders.

---

### What the engine evaluates

Filters are organised into a `FilterGroup`, a recursive structure with `Logic = AND | OR` and an arbitrary tree of nested `SubGroups`. A group holds any combination of the predicate types below.

| Predicate | What it matches |
|---|---|
| **`TreasureFilter`** | Scalar comparison on a Treasure's typed value field, on a path inside its MessagePack `BytesVal`, or on metadata timestamps (`CreatedAt`, `UpdatedAt`, `ExpiredAt`). Operators include `EQUAL`, `NOT_EQUAL`, `GREATER_THAN`, `LESS_THAN`, `GREATER_THAN_OR_EQUAL`, `LESS_THAN_OR_EQUAL`, plus `STRING_IN`, `INT32_IN`, `INT64_IN` for set membership. |
| **`PhraseFilter`** | Consecutive-word phrase matching against a word-position-index map (`map[string][]int`) inside `BytesVal`. Supports negation. |
| **`VectorFilter`** | Cosine similarity on a `[]float32` field at a given `BytesFieldPath`. Both the stored vector and the query vector must be L2-normalised; matching is then a dot product against `MinSimilarity`. |
| **`GeoDistanceFilter`** | Great-circle (Haversine) distance from a reference lat/lng to a `(LatFieldPath, LngFieldPath)` pair. `INSIDE` / `OUTSIDE` mode for radius matching; band filters compose two `GeoDistanceFilter`s in an AND group. Null-island coordinates (0,0) are excluded automatically. |
| **`NestedSliceWhereFilter`** | Conditions evaluated against elements of a nested slice inside `BytesVal`. Quantifier modes: `ANY` (default, at least one element matches), `ALL`, `NONE`, `COUNT` (compare match count via `CountOperator` / `CountValue`). The inner conditions are themselves a `FilterGroup`, so you can nest AND/OR/sub-filters per element. |

Each filter type also accepts an optional `Label`. When set, the label appears in `SearchResultMeta.MatchedLabels` on every Treasure that satisfied that filter — useful when you want to know *which* condition caused the match.

---

### Where filters apply

Filters are part of the streaming read RPCs:

| RPC | Use it for |
|---|---|
| `GetByIndexStream` | Stream matches from a single Swamp, ordered by an index (creation time, update time, expiration, key). Supports pagination via `From` / `Limit`, post-filter `MaxResults`, `ExcludeKeys`, `IncludedKeys`, and `KeysOnly` (return keys without bodies). |
| `GetByIndexStreamFromMany` | Stream matches across many Swamps in one call. Each `SwampQuery` carries its own index, order, filters and limits; a global `MaxResults` caps the combined output. |
| Profile-mode streams | Where a profile is stored as one Treasure per struct field, each filter sets `TreasureKey` to target a specific field-Treasure inside the profile. |

Both Catalog Swamps (one Treasure per record) and Profile Swamps (one Treasure per field) are supported. The `TreasureKey` field on every filter type controls which Treasure inside a profile is evaluated; in Catalog mode it is left unset.

---

### Time bounds

The streaming reads accept optional `FromTime` and `ToTime` arguments. When the underlying index is one of the time-based variants (`EXPIRATION_TIME`, `CREATION_TIME`, `UPDATE_TIME`), the server narrows the scan to that window before any filter evaluation runs.

---

### Match metadata

`SearchResultMeta` is returned per match and carries:

- `VectorScores` — cosine similarity for each `VectorFilter` that matched, in declaration order.
- `MatchedLabels` — labels of every filter (any predicate type) that evaluated to true on this Treasure.

Useful when you compose mixed filters and want to know which lane the record came in on.

---

### Practical notes

- **MessagePack only for nested paths.** `BytesFieldPath` walks a MessagePack-encoded value. GOB-encoded `BytesVal` will not match — pick MessagePack at write time if you intend to filter on inner fields.
- **Vectors must be normalised.** Pre-normalise both stored vectors and the query vector to unit length. The engine multiplies dot products against `MinSimilarity`; if vectors are not normalised, scores are not in `[0, 1]`.
- **Empty filter group passes everything.** A `FilterGroup` with no filters and no sub-groups is a no-op. The stream then behaves like `GetByIndex` — useful when you want streaming pagination without conditions.
- **Pre-filtering pays off.** Cheap scalar filters in the same group prune the candidate set before vector or phrase scoring runs. Place narrow `TreasureFilter`s above expensive predicates.

---

### Where to go next

- The proto definitions live in [`proto/hydraide.proto`](../../proto/hydraide.proto) — search for `--- QUERY FILTER SYSTEM ---` and `--- STREAMING READ REQUESTS/RESPONSES ---`.
- The Go SDK exposes fluent builders for these types; see the [Go SDK reference](../sdk/go/go-sdk.md).
- For the writing side that pairs with these reads, see [Structural MessagePack patch](structural-msgpack-patch.md) — atomic field-level writes on the same MessagePack records that the query engine filters on.
