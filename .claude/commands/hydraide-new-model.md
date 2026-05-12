---
description: Generate a new HydrAIDE data model (Profile or Catalog) with RegisterPattern boilerplate and a test scaffold.
allowed-tools: Bash(go list:*), Bash(grep:*), Bash(find:*), Read, Write, Edit, AskUserQuestion
---

## Context

- Current Go module: !`go list -m 2>/dev/null || echo "not in a Go module"`
- Existing model files in this project: !`find . -path ./node_modules -prune -o -path ./.git -prune -o -type f -name "model_*.go" -print 2>/dev/null | head -10`

The user wants to create a new HydrAIDE data model. **Activate the `hydraidego` skill first** — it has the full reference for tags, types, register patterns, and pitfalls (especially section 17 "Designing a new model checklist").

## Your task

Follow the steps in order. Do NOT skip the confirmation in step 3.

### Step 1: Gather requirements (single AskUserQuestion call)

Ask all four questions in one tool call:
1. **Model type**: Profile (one entity per Swamp) vs Catalog (many keyed records per Swamp). If unsure, default to Catalog.
2. **Sanctuary** name (e.g. `myapp`). If the current Go module name suggests one, propose it as the default option.
3. **Realm** name (e.g. `user-profile`, `order-catalog`). Suggest one based on the model's purpose.
4. **Swamp identifier strategy**: per-tenant, per-user, compound key (`<tenant>:<id>`), or other.

### Step 2: Gather fields

Ask the user, in plain prose, for the field list. For each field they describe:
- Map to a typed Go field. **Reject bare `int`/`uint`** — insist on `int32`, `int64`, `uint32`, etc.
- Use `time.Time` for timestamps.
- For Catalog payloads, decide which fields are scalar (top of struct) vs nested (inside the `value` payload struct).

If they want timestamps, propose `CreatedAt`, `UpdatedAt` as `hydraide:` metadata tags (Catalog only) instead of plain fields.

### Step 3: Confirm before generating

Print a short summary block:
```
Going to create:
- File: <proposed path>
- Type: Profile|Catalog
- Address: Sanctuary("X").Realm("Y").Swamp(...)
- Fields: ...
- CloseAfterIdle: <chosen value>
```

Ask: "Looks right? Generate now, or adjust first?" Wait for confirmation. Do not skip this.

### Step 4: Generate the model file

Suggest a file path based on existing project conventions (look at `model_*.go` siblings). Confirm the path with the user if ambiguous.

The file MUST contain:
- Struct definition with correct `hydraide:` tags. Profile: field-as-Treasure with `omitempty` / `deletable` modifiers. Catalog: `key`, `value`, `createdAt`, `updatedAt`, optional `expireAt`.
- `name(...)` helper returning `name.Name`.
- `Save`, `Load` (and `Delete` for Catalog) methods using `r.GetHydraidego()`.
- `RegisterPattern(r repo.Repo)` method. Use `EncodingFormat: hydraidego.EncodingMsgPack` (always). Set `CloseAfterIdle` based on the access pattern from step 1 (default: 2 minutes for warm, 30 seconds for cold, 5 minutes for hot per-user/tenant).
- All read paths handle `IsSwampNotFound(err) || IsNotFound(err)` as "not found, not error".
- All error branches use `slog.Error` with structured fields.
- Context with timeout via `hydraidehelper.CreateHydraContext()`.

### Step 5: Generate the test file

Use a real-instance test suite (`testify/suite` style, no mocks):
- `SetupSuite`: connect to the test instance, call `RegisterPattern`.
- `TearDownTest`: `Destroy` the test Swamp(s).
- One smoke test: Save → Load roundtrip with realistic field values.
- Comment at the top: `// Run against a real HydrAIDE test instance, never mocked.`

For Catalog payloads with `createdAt`, the smoke test MUST set `CreatedAt = time.Now().UTC()` before save (otherwise the server silently drops the write — flag this as a pitfall in the comment too).

### Step 6: Final summary

Print:
- Files created (clickable markdown links).
- One TODO line: "Wire `RegisterPattern()` into your service startup, before the first read or write."
- Any decision the user should revisit later (e.g. CloseAfterIdle value).
- If the model represents a claim / queue / rate-limited resource: a one-line note that quota enforcement should use server-side `Cap` on the claim call (e.g. `builder.WithCap(&hydraidego.Cap{...})`), not an app-side counter. Link the §14b "Bounded atomic claim with Cap" section of the hydraidego skill.

## Constraints

- DO NOT mock the engine in tests.
- DO NOT use bare `int`/`uint`.
- DO NOT skip `CreatedAt = time.Now().UTC()` when the model has a `createdAt` tag (silent drop on save).
- ALWAYS `EncodingMsgPack`. Never GOB (filters break server-side).
- Refuse to add `msgpack` struct tags to Catalog payload types unless the user has a specific reason — filters use Go field names by default.
