# Bug — streaming-read RPCs leak a Vigil on panic, permanently blocking swamp Destroy/Close

**Found:** 2026-05-15
**Reporter:** Peter Gebri (Trendizz)
**Affected versions:** server `v3.19.0` (reproduced on `main` @ `90a511c`); the
defer-less pattern predates that commit, so older too.
**Severity:** High — a single panic inside the per-query loop of
`GetByIndexStreamFromMany` or `GetStream` leaks a Vigil on the affected swamp.
The swamp's `Destroy()` / `Close()` then blocks **forever** at
`WaitForActiveVigilsClosed()`. Every subsequent `SummonSwamp` for that swamp
hits the 30-second `WaitForGracefulClose` deadline and the swamp is endlessly
"dropped" and re-created. In practice the swamp becomes permanently
unresponsive (all reads/writes time out) until the process restarts.

---

## Summary

`Vigil` (`app/core/hydra/swamp/vigil/vigil.go`) is the in-flight-operation
counter that `Destroy()` / `Close()` wait on before tearing a swamp down. Every
RPC that touches a swamp must pair `BeginVigil()` with a guaranteed
`CeaseVigil()`. Almost all of them use `defer swampInterface.CeaseVigil()`
immediately after `BeginVigil()`.

**Two** RPCs do not:

- `GetByIndexStreamFromMany` (`app/server/gateway/gateway.go`, `BeginVigil()`
  at line **781** on `main`)
- `GetStream` (`app/server/gateway/gateway.go`, `BeginVigil()` at line **957**
  on `main`)

Both call `BeginVigil()` **without** a `defer`, then call `CeaseVigil()`
manually on every individual return branch (lines 806, 820, 871, 882, 887 for
the first; 997, 1003, 1008, 1019 for the second).

Both functions also have `defer handlePanic()` at the top
(`gateway.go:758` and `:934`). `handlePanic()`
(`gateway.go:2926`) calls `recover()` and logs — it does
**not** re-panic. So if anything in the per-query body panics, the panic
unwinds **past** the manual `CeaseVigil()`, is swallowed by `handlePanic()`,
the RPC returns normally to the caller — and the Vigil count for that swamp is
left at `+1` forever.

`vigil.WaitForActiveVigilsClosed()` is an unconditional
`sync.Cond` wait with no timeout:

```go
func (v *vigil) WaitForActiveVigilsClosed() {
	v.cond.L.Lock()
	defer v.cond.L.Unlock()
	for v.HasActiveVigils() {   // never becomes false again after a leak
		v.cond.Wait()
	}
}
```

`Destroy()` (`app/core/hydra/swamp/swamp.go:2323`) calls this
**before** `goRoutineCancelFunction()`, so a leaked Vigil means `Destroy()`
never reaches the goroutine-cancel, the swamp's `goRoutineContext` never
closes, and `WaitForGracefulClose()` (used by the next `SummonSwamp` while the
swamp `IsClosing()`) times out after 30s on every subsequent call.

## Field evidence

Observed against the `crawler-unit` instance via the Trendizz Graylog
(`source:hydraide-crawler-unit`), on the `llm/logs/calls` swamp, repeating
every ~30 seconds:

```
level=err  message="the swamp can not be closed in 30 seconds, so we need to drop it"
           swampName="llm/logs/calls"  closeError="{}"
level=warn message="the summoning context is done, summoning is cancelled"
           swampName="llm/logs/calls"
```

`closeError="{}"` = the close did not fail with an error, it simply never
completed within 30s — consistent with `Destroy()` parked in
`WaitForActiveVigilsClosed()` on a leaked Vigil. The downstream client
(a test suite doing `Save` → `CatalogReadManyStream` → `Destroy` cycles on
that swamp) saw every operation after the first time out with
`Code 3, context timeout exceeded`; only the very first cold run on a fresh
swamp passed.

`CatalogReadManyStream` / `CatalogReadManyFromMany` is served by
`GetByIndexStreamFromMany`; the multi-key profile read is served by
`GetStream`. Both are exactly the two leaky RPCs.

## Root cause

The defer-less manual-cleanup pattern. The original body of
`GetByIndexStreamFromMany` (HEAD `90a511c`, lines 781–892):

```go
swampInterface.BeginVigil()                       // 781 — NO defer

fromTime, toTime := parseOptionalTimestamps(...)
...
if plan.Mode != PlanModeBypass && bucketExecPreconditions(beaconType) {
	candidates := collectBucketCandidates(swampInterface, plan.Hints)   // can panic
	candidates = applyTimeRange(candidates, beaconType, fromTime, toTime)
	sortCandidates(candidates, beaconType, order)
	treasures = applyFromLimit(candidates, query.GetFrom(), query.GetLimit())
	residualFilters = plan.Residual
} else {
	treasures, err = swampInterface.GetTreasuresByBeacon(...)
	if err != nil {
		swampInterface.CeaseVigil()               // 806 — manual
		return status.Error(...)
	}
	residualFilters = filters
}
...
for _, treasureInterface := range treasures {
	if stream.Context().Err() != nil {
		swampInterface.CeaseVigil()               // 820 — manual
		return stream.Context().Err()
	}
	...
	if needsMeta {
		matched, meta = evaluateNativeFilterGroupWithMeta(treasureInterface, residualFilters) // can panic
	} else {
		matched = evaluateNativeFilterGroup(treasureInterface, residualFilters)               // can panic
	}
	...
	t := &hydrapb.Treasure{}
	treasureToKeyValuePair(treasureInterface, t)  // can panic
	...
	if err := stream.Send(resp); err != nil {
		swampInterface.CeaseVigil()               // 871 — manual
		return err
	}
	...
	if globalMax > 0 && globalCount >= globalMax {
		swampInterface.CeaseVigil()               // 882 — manual
		return nil
	}
}
swampInterface.CeaseVigil()                       // 887 — manual (normal path)
```

Every `return` path has a hand-placed `CeaseVigil()`. A **panic** (e.g. in
`treasureToKeyValuePair`, `evaluateNativeFilterGroup*`,
`collectBucketCandidates`, or any nil-deref / type-assert inside the loop) does
not take a `return` path: it unwinds the stack, skipping every manual
`CeaseVigil()`, and is recovered by `defer handlePanic()`. Net effect: Vigil
leaked, RPC returns as if nothing happened, swamp permanently un-closable.

`GetStream` (HEAD lines 957–1019) has the identical shape with manual
`CeaseVigil()` at 997 / 1003 / 1008 / 1019.

These are the **only two** `BeginVigil()` callsites in `gateway.go` lacking an
immediately-following `defer CeaseVigil()`. Grep to confirm:

```bash
grep -n 'BeginVigil()' app/server/gateway/gateway.go
# every other hit has 'defer swampInterface.CeaseVigil()' on the next line
```

## Reproduction (engine-only, no Trendizz, no live instance)

The bug is deterministic if you force a panic inside the per-query loop while a
Vigil is held. A `stream` whose `Send()` panics is the cleanest injector,
because `Send()` is called *after* `BeginVigil()` and the recovered panic
skips the manual `CeaseVigil()`.

Suggested test in `app/server/gateway/` (new file
`gateway_stream_vigil_test.go`). Pseudocode — adapt the harness to however the
existing `gateway_*_test.go` files build a `Gateway` + real swamp:

```go
// panicStream implements HydraideService_GetByIndexStreamFromManyServer.
// Every method is a thin stub except Send, which panics — simulating any
// panic inside the per-query loop (treasureToKeyValuePair, filter eval, …).
type panicStream struct {
	grpc.ServerStream
	ctx context.Context
}
func (p *panicStream) Context() context.Context { return p.ctx }
func (p *panicStream) Send(*hydrapb.GetByIndexStreamFromManyResponse) error {
	panic("injected panic inside per-query loop")
}

func TestGetByIndexStreamFromMany_VigilNotLeakedOnPanic(t *testing.T) {
	g, swampName, islandID := newGatewayWithSeededSwamp(t) // seed >=1 treasure
	sw := summonSwamp(t, g, islandID, swampName)           // get the swamp iface

	require.False(t, sw.HasActiveVigils(), "precondition: no vigils")

	req := &hydrapb.GetByIndexStreamFromManyRequest{
		Queries: []*hydrapb.GetByIndexStreamFromManyQuery{{
			IslandID:  islandID,
			SwampName: swampName.Get(),
			IndexType: hydrapb.IndexType_CREATION_TIME,
			// no filters → beacon-walk path → at least one treasure →
			// loop reaches stream.Send() → panicStream panics
		}},
	}

	// handlePanic() recovers the panic; the RPC returns without re-panicking.
	_ = g.GetByIndexStreamFromMany(req, &panicStream{ctx: context.Background()})

	// BUG: on HEAD this is true forever — the manual CeaseVigil() was skipped.
	require.False(t, sw.HasActiveVigils(),
		"Vigil leaked: CeaseVigil() skipped when per-query body panicked")

	// And the consequence: Destroy() blocks forever on WaitForActiveVigilsClosed().
	done := make(chan struct{})
	go func() { sw.Destroy(); close(done) }()
	select {
	case <-done: // fixed behaviour
	case <-time.After(5 * time.Second):
		t.Fatal("Destroy() blocked on WaitForActiveVigilsClosed() — Vigil leak")
	}
}
```

Add the mirror test for `GetStream` with a `panicStream` whose
`Send(*hydrapb.GetStreamResponse)` panics; seed keys so
`evaluateNativeProfileFilterGroup` passes and the loop reaches `Send`.

Expected on HEAD: both `require.False(... HasActiveVigils ...)` fail and the
`Destroy()` goroutine never returns (5s timeout fires). With the fix below,
both pass and `Destroy()` returns immediately.

If a panic-injecting stream is awkward in the existing harness, an equivalent
non-panic reproduction is harder, because every *non-panic* return branch does
have a manual `CeaseVigil()`. Panic is the canonical trigger and the one seen
in the field (a recovered panic deep in filter/treasure conversion under
concurrent Save+Destroy load).

## Proposed fix

Wrap the per-query body of **both** RPCs in a closure that pairs
`BeginVigil()` with `defer CeaseVigil()`, and have the closure return
`(stop bool, err error)` so the outer loop can still break out of the whole
stream (`return nil`) or propagate an error. This guarantees `CeaseVigil()`
on every path including panic (the deferred call runs during stack unwind,
before the function-level `handlePanic()` recovers).

Sketch for `GetByIndexStreamFromMany`:

```go
stop, err := func() (stop bool, err error) {
	swampInterface.BeginVigil()
	defer swampInterface.CeaseVigil()

	// ... entire former per-query body ...
	// replace each `CeaseVigil(); return X`   with `return false, X`
	// replace `CeaseVigil(); return nil` (globalMax/stream-done) with `return true, nil`
	// normal end of body:                          `return false, nil`
}()
if err != nil { return err }
if stop      { return nil }
if globalMax > 0 && globalCount >= globalMax { return nil }
```

`globalCount` stays declared in the outer function so the closure mutates the
shared counter (closure capture by reference) — semantics unchanged. Same
transformation for `GetStream` (its closure only needs `stop` for the
`maxResults` early-return).

This is a pure control-flow refactor: the order of operations, the streamed
output, and every error/early-return is identical to HEAD. The only behavioural
change is that `CeaseVigil()` is now panic-safe.

### Defense-in-depth (optional, separate decision)

Even with the callsites fixed, `vigil.WaitForActiveVigilsClosed()` has no
timeout, so any *future* leaking callsite reproduces this class of hang. A
bounded/`context`-aware variant of `WaitForActiveVigilsClosed` used by
`Destroy()`/`Close()` would convert "hang forever" into "log + drop after N
seconds". This is intentionally **not** part of the primary fix: a bounded wait
can tear a swamp down while a *legitimate* long operation still holds a Vigil,
which risks the data-corruption the Vigil exists to prevent. Treat it as a
follow-up design question, not a drop-in.

## Status of the fix in this worktree

A fix matching "Proposed fix" above has **already been applied** to
`app/server/gateway/gateway.go` in this working tree (uncommitted at the time
of writing — `git diff app/server/gateway/gateway.go`). It builds and
`go vet`s clean. The pre-existing `TestGatewayPatch_*` failures in
`go test ./app/server/gateway/` are unrelated (Patch semantics, 0.00s,
`expected:1 actual:0`) — they fail on clean HEAD too and do not touch the
stream RPCs. No dedicated stream/vigil unit test existed before; the
reproduction test above is the missing coverage and should be added alongside
the fix so it cannot regress.

The end-to-end confirmation (the Trendizz `inference-service/log_service`
suite going green) additionally requires the `crawler-unit` instance to be
restarted on a HydrAIDE binary that includes this fix; until then its
`llm/logs/calls` swamp stays wedged until the 30-minute `CloseAfterIdle`
elapses or the instance is restarted.

---

## Resolution

Resolved. The closure + `defer CeaseVigil()` refactor described under
"Proposed fix" is applied to both `GetByIndexStreamFromMany` and `GetStream`
in `app/server/gateway/gateway.go`. Pure control-flow change: streamed output
and every error/early-return path are identical to HEAD; the only behavioural
difference is that `CeaseVigil()` is now panic-safe.

Regression coverage added in
`app/server/gateway/gateway_stream_vigil_test.go`:
`TestGetByIndexStreamFromMany_VigilNotLeakedOnPanic` and
`TestGetStream_VigilNotLeakedOnPanic`. Both build a real Zeus + Hydra + Gateway,
seed a swamp, drive the RPC with a `Send()`-panicking stream, then assert the
swamp has no active vigils and `Destroy()` returns within 5s. Verified: both
pass on the fixed code and both fail on the pre-fix code (vigil stays leaked,
`HasActiveVigils()` true).

The `vigil.WaitForActiveVigilsClosed()` timeout ("Defense-in-depth") is
intentionally **not** part of this fix — deferred as a separate decision for
the reason stated in that section.

The pre-existing `TestGatewayPatch_*` failures in
`go test ./app/server/gateway/` are unrelated (Patch semantics) and fail on
clean HEAD without this change too.
