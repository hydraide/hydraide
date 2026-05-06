# 02-recipes/atomic-patch

Atomic field-level mutation on a msgpack Treasure using `PatchTreasures`.

## Why this matters

The classical alternative is **read-modify-write**: GET the row, change a
field client-side, PUT the whole row back. That pattern races under
concurrent writers and forces every update to ship the entire payload.

`PatchTreasures` sends only **the field path and the new value**. The
engine applies the SET op atomically under the per-key guard. Every other
field is untouched, network bandwidth is minimal, and concurrent writers
can patch disjoint fields on the same key in parallel without a global
lock.

## What the recipe does

1. Save a `User` Treasure with a nested `Profile`.
2. Toggle `isVerified` from `false` to `true` with `CatalogPatchField` —
   one field, one op.
3. Patch `score` and `loginCount` together with `CatalogPatchFields` —
   multiple fields, one round-trip, one atomic guard.
4. Read the Treasure back and assert `email` is untouched while the
   patched fields reflect the new values.

## Encoding requirement

`PatchTreasures` requires the swamp to be **msgpack-encoded**. Calling it
on a GOB-encoded Treasure returns `PatchStatusEncodingNotSupported`. The
shared `setup.Pattern` helper registers all example swamps with
`EncodingMsgPack` for this reason.

## Run it

```bash
docker compose up -d
make recipe-atomic-patch
```

## Test it

```bash
make test-examples
```
