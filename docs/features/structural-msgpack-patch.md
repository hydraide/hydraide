# 🧬 Structural MessagePack Patch – Type-Preserving Atomic Field Mutations

## Philosophy

Imagine you have a deeply nested document — a user profile, a domain crawl status, a workflow record — stored as a single MessagePack-encoded value. You want to flip a single boolean flag, increment a counter, append an event to a log. Traditionally you have three options, each painful:

1. **Read the entire document, mutate it in your client, write it back.** Race conditions everywhere unless you wrap the whole thing in a distributed lock — and that lock becomes the bottleneck the moment two writers want different fields on the same record.
2. **Split the document into many separate keys.** Now you've traded one race condition for a fan-out of writes that lose atomicity entirely.
3. **Decode-modify-encode on the server.** Loses every type distinction the wire format encoded — `int8` becomes `int64`, `time.Time` decays into a string, your downstream readers break in production six months later.

The **HydrAIDE structural MessagePack patch primitive** gives you a fourth option: send the server a *typed list of mutations* (set this field, increment that one, append to this slice, merge into that map), and the server applies them **directly to the encoded byte stream** under a per-key guard. Untouched fields keep their exact wire encoding. Mutated fields take on the type the client encoded. Other writers on the same key queue behind you on the same FIFO lock that powers `IncrementInt8`. Other writers on *different* keys run fully in parallel.

This isn't a clever encoding trick — it's a different mental model for "update one field of a thing." Instead of *"fetch state, transform locally, write state back, hope nobody else got there first,"* you say *"add one to `Counter`, set `IsCrawling` to true, and only do it if `Owner == 'alice'`."* The server answers *"done, here's the status."* No round-trip, no client-side lock, no read-modify-write window for races to exploit.

## Operation

When a client issues a `PatchTreasures` RPC, it carries a multi-key batch. Each `TreasurePatch` targets one key inside the named swamp and contains:

* an ordered list of **typed ops** — `SET`, `DELETE`, `INC`, `APPEND`, `PREPEND`, `REMOVE_AT`, `REMOVE_VAL`, `MERGE`,
* an optional **pre-condition** (`EQUAL`, `NOT_EQUAL`, `GREATER_THAN`/`OR_EQUAL`, `LESS_THAN`/`OR_EQUAL`, `EXISTS`, `NOT_EXISTS`),
* metadata flags (`SetUpdatedAt`, `SetUpdatedBy`, `SetCreatedAt`, `SetCreatedBy`).

For each key the server walks the FIFO lock queue, summons the existing MessagePack body, parses only its **structural skeleton** (no leaf decoding — leaves stay as raw byte ranges into the original blob), evaluates the condition, applies every op against the skeleton in order, and emits a freshly serialized blob where every untouched leaf is byte-copied verbatim from the input. Mutated leaves carry their pre-encoded msgpack bytes from the client.

The result is then written back via the swamp's normal save path, so the change inherits compaction, replication, subscription notifications, and every other piece of swamp lifecycle behavior for free.

### Type Preservation Guarantee

Because untouched leaves are *never decoded* — only their byte ranges are tracked — they round-trip with byte-level identity:

* `int8` stays `int8` (msgpack code `0xd0`)
* `int16` stays `int16` (`0xd1`)
* `time.Time` stays its canonical extension encoding (`0xc7…`)
* `[]byte` stays as binary, not as base64 string drift
* nested maps preserve key order and per-key codec choices

Mutated leaves take on whatever type the client encoded into `Op.Value`. `INC` is class-aware — it preserves the *target's* type code (incrementing an `int8` returns an `int8`, never widening to `int64`); cross-class deltas (a `float64` delta on an `int32` field) are rejected as `TYPE_MISMATCH`.

### Per-Key Atomicity, No Cross-Key Atomicity

Inside a single `TreasurePatch`, every op runs under the same guard hold and either all commit or none do. If op #4 fails, ops #1–3 are discarded — the original blob is never partially mutated.

Across keys in the same batch there is **no atomicity**: each key takes its own guard, runs independently, and reports its own per-key `PatchResult.Status`. A type mismatch on one domain's record does not stop the patch on the next. This is intentional — the batch is for round-trip efficiency, not for distributed transaction semantics.

### Conditional Updates

A `PatchCondition` is evaluated once, before any op runs. If the comparison does not hold, the result is `CONDITION_NOT_MET` and the blob is left untouched. Comparators are class-aware: numeric comparisons respect `int` vs `uint` vs `float`; string and `bool` comparisons are byte-exact. Compound conditions are deliberately not supported in the V1 wire format — chain multiple sequential `PatchTreasures` calls if you need them.

## Advantages

* **Eliminates client-side lock loops** — the `Lock + Load + Save` round-trip pattern (and every `MULTI-HOLD` warning that comes with it) goes away on hot keys
* **Preserves wire-level types** — no silent `int8 → int64` widening, no `time.Time → string` drift, no broken downstream readers
* **Atomic multi-field updates** — flip several flags at once with one guard hold and one disk write
* **Concurrency-friendly** — different keys run in parallel; the same key serializes via the existing FIFO queue that powers `IncrementInt8`
* **Conditional safety** — optimistic-style pre-checks built into the wire format
* **Auto-create + metadata in one call** — `CreateIfNotExist` plus `SetCreatedAt`/`SetCreatedBy`/`SetUpdatedAt`/`SetUpdatedBy` removes the ceremony around new-record bootstrapping

## Comparable primitives elsewhere

If you have used patch-style updates in other systems, this primitive sits in the same family — with type preservation as the differentiator:

* **MongoDB** has `$set` / `$inc` / `$push` over BSON. BSON carries a different type lattice from msgpack and tends to widen integers unless you reach for explicit wrappers.
* **Redis** has `HINCRBY` and field-level operations on a flat hash. No nested-path syntax, no nested arrays, types are mostly string.
* **PostgreSQL JSONB** offers `jsonb_set`, concatenation, and removal operators. JSONB normalises numeric types on the way in, so the wire format's distinction between `int8` and `int64` is not preserved.
* **DynamoDB `UpdateItem`** supports typed expressions on top-level and document-path attributes, with type discipline that is closer to msgpack's, plus separate rate-limit accounting on document-path updates.
* **Generic SQL with JSON columns**: read row, mutate JSON in application code, write row back — the read-modify-write pattern this primitive is designed to avoid.

These are real and useful in their own engines — the structural patch is what fits naturally inside HydrAIDE because the wire format is already msgpack and the per-key FIFO lock is already running.

## When to Reach For It

Use the structural patch when:

* You store a record-shaped value (struct or map) per key in a Catalog Swamp
* Multiple writers can target *different fields* on the *same key* concurrently
* Type discipline matters to your downstream readers (cross-language SDKs, query filters that inspect specific msgpack codes)
* You'd otherwise wrap the read-modify-write in a `Lock`/`Unlock` pair

Stick with `CatalogSave` (full-record overwrite) when:

* Every update touches most of the fields anyway
* You don't have concurrent writers per key (and never will)
* You're working with Profile Swamps — those store one field per Treasure, so the patch primitive doesn't apply; `ProfileSave` already gives you field-level overrides

## See Also

* SDK reference: [`docs/sdk/go/go-sdk.md`](../sdk/go/go-sdk.md) — search for *Field-Level Patches* for the Go API and end-to-end examples
* Engine internals: [`app/core/hydra/swamp/treasure/msgpackpatch/`](../../app/core/hydra/swamp/treasure/msgpackpatch/) — the structural skeleton parser, op pipeline, and condition evaluator
* Wire format: [`proto/hydraide.proto`](../../proto/hydraide.proto) — search for `PatchTreasures`, `PatchOp`, `PatchCondition`, `PatchResult`
