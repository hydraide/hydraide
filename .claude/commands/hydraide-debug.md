---
description: Guided diagnostic flow for a HydrAIDE issue. Walks through the common pitfalls (clock skew, register pattern, encoding, logs).
allowed-tools: AskUserQuestion, Read, Bash(grep:*), Bash(git log:*), Bash(git diff:*), Bash(go list:*)
---

## Context

- Current Go module: !`go list -m 2>/dev/null || echo "not in a Go module"`
- Recent HydrAIDE-related commits: !`git log --oneline -10 -- "*hydraide*" "*Hydra*" 2>/dev/null | head -10`

The user is reporting a HydrAIDE problem. **Activate the `hydraidego` skill** before diagnosing (and `hydraidectl` if the issue looks operational). For conceptual "why does it work this way" questions, also activate `hydraide`.

## Your task

Work through the four steps **in order**. Do not skip step 1 — half of HydrAIDE bug reports vanish once you ask the right scoping questions.

### Step 1: Scope the symptom (single AskUserQuestion call)

Ask all four questions in one tool call:

1. **Symptom type**: error message, missing data, slow response, panic, or other?
2. **Environment**: local dev, test instance, or production?
3. **Topology**: single host (everything on one machine), or distributed (API + workers + HydrAIDE on different hosts)?
4. **Trigger**: just started (recent change like a deploy, new model, or server upgrade), or always been like this?

Wait for answers before proceeding.

### Step 2: Walk the pitfall checklist in priority order

**Stop at the first match.** Do not run the whole list when symptom 1 already explains it.

1. **Clock skew + tight `ExpireAt` margin** (#1 cause of flaky `ShiftExpired returns 0`):
   - If symptom mentions `CatalogShiftExpired` returning 0 unexpectedly:
     - Ask the user to grep their codebase: `grep -rn "ExpireAt.*Add" --include="*.go" .`
     - Look for any `ExpireAt = time.Now().UTC().Add(-X)` where X is less than `time.Minute`.
     - In distributed deployments, NTP skew oscillates 100ms–2s. Anything tighter than 1 minute past `now` is flaky.
     - Fix: change to `time.Now().UTC().Add(-1 * time.Minute)`. Don't blame the engine.

2. **`createdAt` zero on Catalog save** (silent write drop):
   - If symptom is "writes return no error, reads return nothing":
     - Look at the model definition. Has `hydraide:"createdAt"` tag?
     - Look at all `CatalogSave`/`CatalogCreate` callsites. Is `CreatedAt = time.Now().UTC()` set?
     - If missing: that's the bug. Fix the callsite, not the model.

3. **`RegisterPattern` not called**:
   - If errors mention "swamp not registered" or undefined behaviour at startup:
     - Find the model's `RegisterPattern()` method.
     - Search for callers in `main.go` / service init / startup paths.
     - If no caller: add the call before the first read/write of that model.

4. **GOB-encoded payload + filter not matching**:
   - If symptom is "filter returns nothing but data exists":
     - Suggest running `hydraidectl inspect <swamp>` to check encoding.
     - If GOB: the swamp's payload is not server-side filterable. Switch the model's `RegisterPattern` to `EncodingMsgPack`, then `hydraidectl compact` the swamp to rewrite the file. New writes will be msgpack from then on.

5. **Bare `int` / `uint` types**:
   - Cross-platform inconsistency or runtime failure on filter type assertion.
   - Grep the model: `grep -n "\\bint\\b\\|\\buint\\b" path/to/model.go`
   - Replace with `int32`, `int64`, `uint32`, etc.

6. **Subscribe used as a queue** (architectural bug):
   - If symptom involves "missed events" or "I want retries":
     - Subscriptions are FIFO event streams, not durable queues. No retries, no acks, no dead-letter.
     - For work distribution use NATS JetStream / Kafka alongside HydrAIDE.

7. **Application-side claim counter drift** (cap looks full while no records match):
   - Symptom: throughput drops gradually over hours, "cap full" log noise, but a Count(filter) over the cap-filter returns far below `MaxMatching`.
   - Cause: the application maintains its own `Increment*` counter alongside a Cap-bearing claim path. Every code path that forgets to decrement (panic, shutdown mid-task, network timeout, alternative finalize) leaks +1. The drift is monotone — eventually the app-counter looks "full" while actual matching state is empty.
   - Diagnostic: grep for `Increment` / `Decrement` alongside `WithCap` or `CatalogPatchExpired`. If both exist, the counter is redundant and lying.
   - Fix: delete the app-side counter. Trust the server-side Cap. The reconciler that "smooths the drift" is treating a symptom — the counter never had to exist.

### Step 3: Check the logs

If steps 1-2 didn't pinpoint the bug:
- For Trendizz logs: activate the `graylog` skill.
- For a single instance: suggest `hydraidectl observe <instance>` and look for:
  - PANIC entries (server-side bug, not application)
  - ERROR around the symptom timestamp
  - gRPC `DeadlineExceeded` (context timeout too short — increase to 5+ minutes for batch reads)
  - "swamp closed" or "swamp not summoned" (lifecycle issue)

### Step 4: Reproduce, fix, regression-test

If diagnosis is clear:
- Suggest the fix in code, with `file:line` references.
- Suggest adding a regression test against a **real** HydrAIDE test instance (no mocks). Reference the `hydraidego` skill section 16 on testing patterns.

If diagnosis is ambiguous or you've ruled out all six pitfalls:
- Ask for: the failing model file, the exact error message + stack trace, and the relevant log slice from around the failure timestamp (UTC).
- **Do not guess.** Do not propose changes without evidence.

## Constraints

- Don't recommend mocking the engine. Tests must hit a real HydrAIDE instance (mock/prod divergence has burned the user before — see the `Clock skew vs ShiftExpired` memory).
- Don't suggest server config changes (`CloseAfterIdle`, `WriteInterval`) without a concrete reason from the symptoms.
- Don't propose `Lock`/`Unlock` as a fix unless the symptom is a true race (concurrent writers on the same key with read-modify-write between them).
- If multiple pitfalls match, name them in priority order — but recommend fixing one at a time and verifying before moving on.
