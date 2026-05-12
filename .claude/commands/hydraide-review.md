---
allowed-tools: Bash(git diff:*), Bash(git log:*), Bash(git status:*), Bash(git branch:*)
description: Review HydrAIDE-related code changes against the pitfall checklist (struct tags, error handling, batch usage, lifecycle).
---

## Context

- Current branch: !`git branch --show-current`
- Files changed vs main: !`git diff --stat main...HEAD`
- Staged changes: !`git diff --cached --stat`
- Unstaged changes: !`git diff --stat`

## Your task

Activate the `hydraidego` skill, then review the HydrAIDE-related code changes (use `git diff` on the relevant files) against this checklist. Report findings grouped by severity.

### Critical (silently broken)

- **`createdAt` zero-value on Catalog save**: any Catalog model with a `hydraide:"createdAt"` tag must have `CreatedAt = time.Now().UTC()` before save. Zero value makes the server silently drop the write. Grep the diff for `CatalogSave`, `CatalogCreate`, `CatalogSaveMany`, `CatalogCreateMany` and verify.
- **Bare `int` or `uint` types**: in any HydrAIDE-touching struct or filter call, only explicit-size types are allowed (`int32`, `int64`, `uint32`, etc.). Bare `int`/`uint` causes runtime errors and platform inconsistency.
- **`EncodingFormat`**: every `RegisterSwamp` call must set `EncodingFormat: hydraidego.EncodingMsgPack`. GOB blocks server-side filtering.
- **Race in shift patterns**: read-many followed by `CatalogShiftBatch` without a lock is a data-loss race. Either use `CatalogShiftExpired`/`CatalogShift` (atomic), or wrap in `Lock`/`Unlock`.
- **`ExpireAt` clock-skew margin**: any `ExpireAt = time.Now()` or `Add(-1*time.Second)` for "already expired" is flaky under NTP skew. Require at least `Add(-1 * time.Minute)` past margin.
- **App-side claim counter alongside `WithCap` / `CatalogPatchExpired`**: if the diff introduces an `Increment*` / `Decrement*` counter next to a Cap-bearing claim path, that counter will drift and break the cap over hours. Flag it and recommend deletion — Cap is the server-side source of truth.
- **Claim/quota pattern without Cap**: if the diff implements a claim pattern (status flip, lease) with a `Count(filter)` + claim sequence outside one RPC, there's a race window. Recommend `WithCap(*Cap)` on the claim call — the count + claim happens server-side under one guard.

### Important (slow or wrong)

- **Loop-of-single-key calls**: `for _, k := range keys { CatalogRead(...) }` should be `CatalogReadBatch(...)`. Same for `IsKeyExists` -> `AreKeysExist`.
- **Missing `RegisterPattern` at startup**: every model used must have `RegisterPattern()` called before first read/write.
- **Error handling**: read paths must treat `IsSwampNotFound(err) || IsNotFound(err)` as "not found, not error". Other errors must be logged with `slog.Error` and surfaced.
- **No context timeout**: every HydrAIDE call must have a context with timeout (`hydraidehelper.CreateHydraContext()` for default, `context.WithTimeout` for long batches).
- **`go func()` instead of `panichandler.SafeGo`**: any goroutine in server-side code must use `panichandler.SafeGo("label", func(){})`.

### Style / consistency

- **`msgpack` tags inside Catalog payload structs**: should NOT be present unless deliberate. Filters use Go field names; tags break that.
- **`name()` / `createName()` helper exists** for the model and is used everywhere instead of inline `name.New()...`.

## Output format

For each finding:
- **Severity** (Critical / Important / Style)
- **File:line** reference (clickable markdown link if possible)
- **What's wrong**
- **Suggested fix** (one-line, concrete)

If everything passes, say so explicitly and list the files reviewed.
