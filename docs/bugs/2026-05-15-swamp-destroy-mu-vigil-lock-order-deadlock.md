# Bug — `swamp.Destroy()` deadlocks: holds `s.mu` write-lock while waiting for vigils that need `s.mu` read-lock

**Found:** 2026-05-15
**Reporter:** Peter Gebri (Trendizz)
**Affected versions:** reproduced on `main` and on `server/v3.19.2`
(commit `79be83f`, which already contains the gateway "CeaseVigil on panic"
fix — that fix does **not** address this bug; this is a separate, deeper
swamp-lifecycle bug).
**Severity:** High — under concurrent `Save` + `Destroy` on the same swamp,
`Destroy()` deadlocks permanently. Every subsequent `SummonSwamp` for that
swamp then hits the hard-coded `WaitForGracefulClose(30s)` deadline and the
swamp is logged as "can not be closed in 30 seconds, so we need to drop it"
every ~30s. The swamp becomes effectively unusable (all reads/writes time
out) until the process restarts.

---

## Summary

`swamp.Destroy()` ([`app/core/hydra/swamp/swamp.go:2308`](../../app/core/hydra/swamp/swamp.go)) acquires the
swamp's `s.mu` **write-lock** and *then*, still holding it, calls
`s.Vigil.WaitForActiveVigilsClosed()`:

```go
func (s *swamp) Destroy() {
	...
	atomic.StoreInt32(&s.closing, 1)        // 2313

	s.mu.Lock()                              // 2315  ← write-lock acquired
	defer s.mu.Unlock()

	s.StopSendingInformation()
	s.StopSendingEvents()

	s.Vigil.WaitForActiveVigilsClosed()      // 2323  ← waits for vigils == 0
	                                         //         WHILE holding s.mu

	s.goRoutineCancelFunction()              // 2328  ← never reached on deadlock
	...
	s.sendClosedEvent()                      // 2341  ← never reached → swamp
	slog.Info("Destroy: completed", ...)     // 2343    never removed from map
}
```

A concurrent writer holds a vigil across a `Save` that needs the **same
`s.mu` as a read-lock**:

`SaveFunction` ([`app/core/hydra/swamp/swamp.go:2153`](../../app/core/hydra/swamp/swamp.go)) takes `s.mu.RLock()`.
The standard write path is `BeginVigil()` → `CreateTreasure` →
`treasure.Save()` → `swamp.SaveFunction()` → `s.mu.RLock()` → … →
`CeaseVigil()`. The vigil is held for the whole duration, including the
`s.mu.RLock()` acquisition.

Classic AB–BA lock-order inversion:

```
Writer goroutine                         Destroy goroutine
----------------                         -----------------
BeginVigil()            → vigils = 1
                                         s.mu.Lock()            (acquired)
treasure.Save()
  swamp.SaveFunction()
    s.mu.RLock()  ─── blocks ───────────  (writer can't get RLock:
                                            Destroy holds the write-lock)
(never reaches CeaseVigil)                WaitForActiveVigilsClosed()
                                          ─── blocks ─── (vigils never
                                            reaches 0: writer is stuck
                                            before CeaseVigil)
            ▲                                         │
            └─────────────  deadlock  ────────────────┘
```

Neither side can progress. `Destroy()` never reaches
`goRoutineCancelFunction()` (2328), so the swamp's `goRoutineContext` never
closes, so every other goroutine that summoned this swamp while it was
`IsClosing()` blocks in `WaitForGracefulClose` for the full 30s, logs
"the swamp can not be closed in 30 seconds, so we need to drop it", and the
cycle repeats indefinitely.

## `Close()` is NOT affected

`swamp.Close()` ([`app/core/hydra/swamp/swamp.go:2249`](../../app/core/hydra/swamp/swamp.go)) does **not** take
`s.mu.Lock()` and does **not** call `WaitForActiveVigilsClosed()`. It is
driven from the idle ticker, which only fires when `!HasActiveVigils()`
already. So the lock-order inversion is unique to `Destroy()`. Any fix
should focus on `Destroy()`; `Close()` can be left as-is (but verify the
idle-ticker `!HasActiveVigils()` precondition still holds after any change).

## Field evidence

Trendizz `crawler-unit` HydrAIDE instance (`source:hydraide-crawler-unit`
in Graylog), swamp `llm/logs/calls`, **on server/v3.19.2** (the
vigil-leak-fixed build), during a `go test -count=10` run of the
`inference-service/log_service` suite (08:59–09:04 UTC, 2026-05-15):

```
level=err  message="the swamp can not be closed in 30 seconds, so we need to drop it"  swampName="llm/logs/calls"  closeError="{}"   (×10)
level=warn message="the summoning context is done, summoning is cancelled"             swampName="llm/logs/calls"                      (×3)
Destroy: starting / Destroy: completed pairs are fast (~ms) — but at least
one Destroy never completes, and the "can not be closed" lines recur on a
~30s period (= the hard-coded WaitForGracefulClose timeout).
```

`closeError="{}"` = the close didn't fail with an error, it simply never
finished within 30s — consistent with `Destroy()` parked at
`WaitForActiveVigilsClosed()` while holding `s.mu`.

The triggering client workload is `TestConcurrentToggle` in
`backend/inference-service/platform-backend/services/log_service/model_call_log_catalog_test.go`:
~20 goroutines doing `LogCall` (server side: `Set` RPC → `BeginVigil` →
`SaveFunction` → `s.mu.RLock`) concurrently with a toggle path that calls
`Destroy` (server side: `Destroy` RPC → `swamp.Destroy()` → `s.mu.Lock` →
`WaitForActiveVigilsClosed`) on the **same** `llm/logs/calls` swamp.

## Reproduction (engine-only, isolated, no Trendizz, no live instance)

A self-contained reproducer is at
[`app/core/hydra/hydra_inmem_rapid_destroy_test.go`](../../app/core/hydra/hydra_inmem_rapid_destroy_test.go)
(`TestInMemRapidSummonDestroyRace`). **Note:** this file is currently
untracked (not yet committed) — it must be committed/handed over together
with this report so the engine owner can run the gate. It models the Trendizz shape at the
engine level: one in-memory swamp, the same name reused, N writer goroutines
doing `SummonSwamp → BeginVigil → CreateTreasure+Save → CeaseVigil`, and M
destroyer goroutines doing `SummonSwamp → Destroy()` on that same name, each
`SummonSwamp` using a 5s context (= `hydraidehelper.CreateHydraContext`).

Run:

```bash
cd ~/development/hydraide
go test -run TestInMemRapidSummonDestroyRace -timeout 40s ./app/core/hydra/
```

Observed on the buggy code (both `main` and `server/v3.19.2` semantics):

```
workload did NOT finish within 1m0s — likely a swamp-lifecycle deadlock.
summonOK=2 summonErrs=8 destroys=0 maxSummon=30.000508731s
```

- `destroys=0` — not a single `Destroy()` completed.
- `maxSummon=30.000508731s` — a `SummonSwamp` blocked for exactly the
  hard-coded `WaitForGracefulClose(30s)`.

Goroutine dump from the `-timeout` kill (the smoking gun):

```
goroutine 58 [sync.Cond.Wait]:
  vigil.(*vigil).WaitForActiveVigilsClosed   vigil.go:76
  swamp.(*swamp).Destroy                     swamp.go:2323   ← holds s.mu (2315), waits vigils

goroutine 51..57 [sync.RWMutex.RLock]:
  swamp.(*swamp).SaveFunction                swamp.go:2153   ← waits s.mu.RLock
  treasure.(*treasure).Save                  treasure.go:2126
  (each is between sw.BeginVigil() and sw.CeaseVigil() → vigils > 0)

goroutine 59,60 [sync.Mutex.Lock]:
  swamp.(*swamp).Destroy                     swamp.go:2315   ← more Destroys queued on s.mu

goroutine 50 [select]:
  swamp.(*swamp).WaitForGracefulClose        swamp.go:2060
  hydra.(*hydra).SummonSwamp                 hydra.go (WaitForGracefulClose 30s)
```

This is the deadlock proven in isolation. Keep this test (or fold it into
the existing hydra concurrency suite) as the regression gate for any fix.

## Why the gateway vigil-leak fix didn't solve it

`server/v3.19.2` `fix(gateway): guarantee CeaseVigil on panic in streaming
reads` (commit `79be83f`) is a real and correct fix for a *different* bug —
see [`2026-05-15-stream-rpc-vigil-leak-on-panic.md`](2026-05-15-stream-rpc-vigil-leak-on-panic.md).
That bug leaks a vigil when a streaming-read RPC panics. **This** bug needs
no panic and no streaming read: a plain concurrent `Save` + `Destroy` on the
same swamp deadlocks because of `Destroy()`'s lock order. The vigil-leak fix
makes `WaitForActiveVigilsClosed()` correct *when callers behave*; it does
nothing about `Destroy()` holding `s.mu` while it waits.

## Proposed fix direction (decision left to the engine owner)

The minimal, targeted change: in `Destroy()`, wait for vigils to drain
**before** acquiring `s.mu`, not after. `closing` is already set to `1` at
line 2313 (before any lock), which is the documented mechanism to stop new
transactions — so the vigil count should be able to reach 0 without the
write-lock held:

```go
func (s *swamp) Destroy() {
	atomic.StoreInt32(&s.closing, 1)          // already first, good

	s.StopSendingInformation()                 // safe without s.mu? verify
	s.StopSendingEvents()

	s.Vigil.WaitForActiveVigilsClosed()        // ← MOVED before s.mu.Lock()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.goRoutineCancelFunction()
	if atomic.LoadInt32(&s.inMemorySwamp) == 0 {
		s.chroniclerInterface.Destroy()
	}
	s.dropAllBuckets()
	s.sendClosedEvent()
}
```

**Points the engine owner must verify before committing this** (do not
apply blindly — this is the swamp teardown critical section):

1. **`closing=1` must actually gate new vigils.** Confirm every
   `BeginVigil()` caller (gateway RPC handlers, internal paths) checks
   `IsClosing()` / `closing` before taking a vigil, OR that a vigil taken
   after `closing=1` cannot wedge teardown. If a new vigil can still be
   acquired between `WaitForActiveVigilsClosed()` returning and `s.mu.Lock()`,
   the wait is not sufficient on its own and a re-check / different
   serialization is needed.
2. **`StopSendingInformation()` / `StopSendingEvents()` without `s.mu`.**
   They are currently called under the write-lock. Verify they are safe to
   call before acquiring `s.mu` (they appear to manipulate event/info
   subscription state, not the treasure map — but confirm).
3. **Concurrent `Destroy()` calls.** Two `Destroy()` goroutines on the same
   swamp: today the second serializes on `s.mu.Lock()`. After the change
   they both pass `WaitForActiveVigilsClosed()` then both contend on
   `s.mu`; `goRoutineCancelFunction()` / `sendClosedEvent()` must be
   idempotent or guarded (a `closeMutex`-style once, like `Close()` uses at
   2251–2259, is the natural pattern).
4. **`WaitForActiveVigilsClosed()` has no timeout.** Independent of the
   lock-order fix: a future leaking vigil callsite would still hang
   `Destroy()` forever (just without holding `s.mu`). A bounded/`context`
   variant for teardown is worth considering as defense-in-depth, but is a
   separate decision (a bounded wait can tear a swamp down while a
   legitimate long op still holds a vigil — the very thing vigils prevent).

## Status

**Resolved.** `Destroy()`'s lock order was corrected in
[`app/core/hydra/swamp/swamp.go`](../../app/core/hydra/swamp/swamp.go):
the vigil drain (`WaitForActiveVigilsClosed()`) now runs **before**
`s.mu.Lock()`, so an in-flight writer can complete its
`SaveFunction` → `s.mu.RLock()` and reach `CeaseVigil()` while `Destroy()`
waits, breaking the AB/BA cycle. `closing=1` is still set first (it gates
`SummonSwamp()` via `IsClosing()`, stopping the inflow of new vigils), and a
new `destroyed` flag guarded by `closeMutex` (same pattern as `Close()`)
makes concurrent `Destroy()` calls idempotent now that they no longer
serialize on `s.mu.Lock()`. `StopSendingInformation/Events` are pure atomic
stores and were verified safe to call without `s.mu`. The
`WaitForActiveVigilsClosed()` timeout question (point 4) is intentionally
left as a separate, future decision.

Verification:

- `TestInMemRapidSummonDestroyRace` (the regression gate) was red
  (`destroys=0`, `maxSummon≈30s`, 40s timeout) on `79be83f` and is now
  green in 0.07s.
- Full `go test ./app/core/hydra/ ./app/core/hydra/swamp/...` green, no
  regression (including the `vigil` and `chronicler` suites).
- Live smoke against a fresh dev image (docs/sdk/go/examples compose):
  quickstart + all 8 recipes green, all 3 reference-app build-checks ok,
  and the `-tags=integration` example suite (`make test-examples`) all
  green.
