# Map-body Catalogs

A Catalog stores key → value entries inside a Swamp. The "value" can take one of two shapes:

| Shape | Wire body | Use when |
|---|---|---|
| **Single-value** | one msgpack-encoded payload (struct, slice, primitive) referenced by `hydraide:"value"` | the body is opaque to the server; you only Save and Read it whole |
| **Map-body** | a top-level msgpack map keyed by field name (each field tagged `hydraide:"FieldName"`) | any individual field will be Patched, filtered, or conditioned on server-side |

Both shapes are first-class. They are mutually exclusive on a single struct: the SDK rejects models that mix `hydraide:"value"` with `hydraide:"FieldName"` body fields.

## When to pick which

Pick map-body if any of the following is true:

- You will mutate individual fields with `CatalogPatch`, `CatalogPatchField(s)`, `CatalogPatchFieldsMany`, `CatalogPatchExpired`, or `CatalogPatchExpiredManyFromMany`.
- You will use server-side `BytesFieldPath` filters (scalar comparisons, slice membership, vector cosine, geo, phrase) on a body field.
- You want the on-disk body to be language-portable msgpack that other SDKs can decode without the GOB header.

Pick single-value if:

- The body is opaque to the server; the client decides what to do with it on read.
- You never need server-side field-level conditions.

## Wire format

A map-body Catalog is stored on disk as:

```
[ 0xC7 0x00 ] [ msgpack fixmap ]
```

The first two bytes are HydrAIDE's msgpack magic prefix. The rest is a standard msgpack map whose keys are the `hydraide:"FieldName"` tag values, in declaration order:

```go
type CrawlReady struct {
    Domain    string    `hydraide:"key"`        // → Treasure.Key (not in body)
    ASN       string    `hydraide:"ASN"`        // body["ASN"]
    TLD       string    `hydraide:"TLD"`        // body["TLD"]
    Priority  int8      `hydraide:"Priority"`   // body["Priority"]
    ClaimedBy string    `hydraide:"ClaimedBy,omitempty"`
    ClaimedAt time.Time `hydraide:"ClaimedAt,omitempty"`
    ExpireAt  time.Time `hydraide:"expireAt"`   // → Treasure.ExpiredAt (not in body)
}
```

Reserved tags (`key`, `expireAt`, `createdAt`, `createdBy`, `updatedAt`, `updatedBy`) map to first-class Treasure slots and never appear in the body. The wire body contains only the non-reserved tagged fields.

## Save / Read symmetry with Patch

Map-body Catalogs round-trip through every Catalog API:

| API | Encoding | Decoding |
|---|---|---|
| `CatalogSave` / `CatalogSaveMany` | `map[string]any` per non-reserved tag → msgpack → wrap with magic prefix → `BytesVal` | n/a |
| `CatalogRead` / `CatalogReadMany` / `CatalogReadManyFromMany` | n/a | unwrap `BytesVal` → msgpack-decode → assign by tag value |
| `CatalogPatch*` | per-op `Set("ASN", v)` writes `body["ASN"]` | iterator decodes the post-patch body the same way |

A document that is Saved by client A and Patched by client B sees the same body shape. The encoder honors `omitempty` per body field; the decoder leaves missing keys at the type's zero value.

## Version compatibility

| SDK | Server | Map-body Save | Map-body Read | Patch flows | Notes |
|---|---|---|---|---|---|
| go ≥ v3.4.0 | any v3.x | ✅ | ✅ | ✅ | full symmetry |
| go v3.3.x and earlier | any v3.x | ❌ silent skip | ❌ body fields nil | ✅ | Patch worked, Save/Read silently dropped flat-tagged fields |

Earlier SDKs (≤ v3.3.x) only handled the standard reserved tags in `convertCatalogModelToKeyValuePair` and `convertProtoTreasureToCatalogModel`. Models with `hydraide:"FieldName"` body tags would Save successfully but write empty bodies, and Read would return the default zero values for those fields. Patch flows already encoded/decoded the body correctly because they address fields by explicit path.

If you previously worked around the gap with a Patch-based seed helper (e.g. `SaveFlatCatalog` that issued `CatalogPatchFieldsMany` with `Set` ops + `CreateIfNotExist`), you can replace it with a direct `CatalogSave` / `CatalogSaveMany` after upgrading the SDK to v3.4.0 or newer. The on-disk format is identical, so existing data needs no migration.

## Meta-only patches

A `CatalogPatch(...).Exec()` call that carries only metadata helpers (`WithUpdatedAt`, `WithUpdatedBy`, `WithExpiredAt`, `WithoutExpiredAt`) and no ops is the "slide TTL forward / refresh metadata without rewriting the body" form. The server applies the metadata on top of the existing body. SDK ≥ v3.4.0 dispatches it; earlier SDKs short-circuited meta-only single-key patches with `ErrCodeInvalidModel "ops list is empty"`. Multi-key (`CatalogPatchFieldsMany`) and expired (`CatalogPatchExpired*`) flows already accepted meta-only builders before v3.4.0.

## See also

- [`structural-msgpack-patch.md`](structural-msgpack-patch.md) — Patch operation semantics and the msgpack body format on disk.
- [`patch-expired-treasures.md`](patch-expired-treasures.md) — atomic claim flow over expired entries.
- [`query-engine.md`](query-engine.md) — server-side filters that read body fields via `BytesFieldPath`.
