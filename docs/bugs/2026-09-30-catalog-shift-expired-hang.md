# Bug: `ShiftExpired` / `Shift` deadlocks against a concurrent overwrite-`Save` or `Delete` on the same swamp (beacon mu vs. treasure guard lock-order inversion)

**Found:** 2026-09-30
**Reporter:** Peter Gebri (Trendizz)
**Affected versions:** reproduced on `main` at `cedbea6` (`server/v3.19.6-1-gcedbea6`),
both engine-level and against a private server built from that commit.
- The **overwrite-`Save`** half of the cycle exists since `7e8809f`
  (`feat(patch): support ExpiredAt mutation via PatchTreasures meta`, first
  released in `server/v3.13.0`, 2026-05-07).
- The **`Delete`** half of the cycle is older: both lock sites date back to the
  initial import (`e955b11`, 2025-07-20).

The Trendizz live and unittest servers reportedly run the same build, so they
are affected.

**Severity:** High. The swamp stays hung until the process restarts: every
later `Set`, `ShiftExpired` or `Shift` on that swamp times out. Nothing
recovers it automatically, and data waiting in the write buffer is not flushed.

---

## Summary

The shift paths lock in the order **beacon `mu` → per-treasure guard**.
The write and delete paths lock in the order **per-treasure guard → beacon
`mu`**. Both paths use the same `expirationTimeBeaconASC` (or whichever beacon
`CatalogShift` scans). This is a classic AB/BA deadlock.

| Path | First lock | Then |
|---|---|---|
| `beacon.ShiftExpired` [`beacon.go:980`](../../app/core/hydra/swamp/beacon/beacon.go) / [`:989`](../../app/core/hydra/swamp/beacon/beacon.go) | `b.mu.Lock()` | `treasureObj.StartTreasureGuard(true)` for **every** row in `treasuresByOrder`, including rows that have not expired |
| `beacon.ShiftMatching` [`beacon.go:1020`](../../app/core/hydra/swamp/beacon/beacon.go) / [`:1059`](../../app/core/hydra/swamp/beacon/beacon.go) | `b.mu.Lock()` | `StartTreasureGuard(true)` for every row |
| gateway `Set` [`gateway.go:292`](../../app/server/gateway/gateway.go) → `treasure.Save` → `swamp.SaveFunction`, overwriting an existing key with `ExpiredAt` set: [`swamp.go:2192`](../../app/core/hydra/swamp/swamp.go) (`deleteTreasureIfBeaconInitialized(expirationTimeBeaconASC)` → `beacon.Delete` [`beacon.go:858`](../../app/core/hydra/swamp/beacon/beacon.go)) and [`swamp.go:2195`](../../app/core/hydra/swamp/swamp.go) (`addToExpirationTimeBeacon` → `beacon.Add`) | per-treasure guard (held by the gateway from `:292` until the deferred release) | `expirationTimeBeaconASC.mu` |
| content-type change branch of `SaveFunction` [`swamp.go:2182`/`:2185`](../../app/core/hydra/swamp/swamp.go) | guard | every beacon `mu` |
| `DeleteTreasure` → `deleteHandler` [`swamp.go:2907`](../../app/core/hydra/swamp/swamp.go) (guard) → `deleteTreasureFromBeacons` [`swamp.go:2931`](../../app/core/hydra/swamp/swamp.go) → [`:3005`](../../app/core/hydra/swamp/swamp.go) | guard | every initialised beacon `mu`, including `expirationTimeBeaconASC` |

Why an overwrite always reaches the beacon: `keyValuesToTreasure` calls
`SetExpirationTime` whenever the request has a valid `ExpiredAt`
([`gateway.go:2746`](../../app/server/gateway/gateway.go)).
`treasure.SetExpirationTime` sets `expirationTimeChanged = true`
unconditionally, even when the value did not change
([`treasure.go:1452`](../../app/core/hydra/swamp/treasure/treasure.go)). So
every `CatalogSave` that overwrites an existing key of a model with an
`expireAt` field takes the modified branch at `swamp.go:2192`.

The deadlock:

```
Set / Delete goroutine (key K)                 Shift goroutine
------------------------------                 ---------------
K.StartTreasureGuard()          (holds K)
                                               expASC.mu.Lock()        (holds expASC)
                                               for each row in expASC:
                                                 row.StartTreasureGuard()
                                                 ... reaches K -> waits for K's guard
SaveFunction / deleteHandler:
  expASC.Delete(K) -> expASC.mu.Lock()
  -> waits for expASC                          <-- DEADLOCK: neither can proceed
```

The shifter takes a guard on every row, not only on the expired prefix (the
loop never breaks). A write to any key already indexed in the swamp can
therefore close the cycle, whether that key is due or not.

After the deadlock, other work on the swamp gets stuck behind it:

- A `Set` of a **new** key reaches `addTreasureToBeacons`, which takes
  `s.beaconBuildMu` ([`swamp.go:2976`](../../app/core/hydra/swamp/swamp.go)).
  It then blocks on `expASC.mu` in `addToExpirationTimeBeacon`
  ([`swamp.go:3346`](../../app/core/hydra/swamp/swamp.go)) while still holding
  `beaconBuildMu`. From then on every new-key `Set` and every lazy
  `buildBeacon` on that swamp blocks.
- The swamp's write listener (`fileWriterHandler` → `chronicler.Write` →
  `writeNewTreasures`, [`chronicler.go:301`](../../app/core/hydra/swamp/chronicler/chronicler.go))
  blocks on the guard of K while holding `s.writerLock` and the chronicler
  `mu`. Buffered writes are no longer flushed, and a close or flush of the
  swamp cannot finish.
- The gRPC handlers keep their vigils, so the swamp is never evicted.
- `Count` and single-key `Get` still answer, because they only use
  `beaconKey` with a read lock.

When neither side runs, nothing breaks: saves alone never hang, and a shifter
alone never hangs. Only the two together on the same swamp form the cycle.

## Reproduction

### 1. Engine-level regression tests (deterministic, no server)

[`app/core/hydra/swamp/swamp_shift_guard_deadlock_test.go`](../../app/core/hydra/swamp/swamp_shift_guard_deadlock_test.go)

| Test | What it does | Result on `cedbea6` |
|---|---|---|
| `TestSwamp_ShiftExpired_ConcurrentOverwriteSave_NoDeadlock` | Forced interleaving. The writer holds K's guard. The test polls the goroutine stacks until the shifter is parked in `beacon.ShiftExpired` → `guard.StartTreasureGuard`. Then the writer runs `Save` with a new `ExpiredAt`. | FAIL, deadlock detected after 5 s |
| `TestSwamp_ShiftMatching_ConcurrentOverwriteSave_NoDeadlock` | Same, via `CloneAndDeleteMatchingTreasures(BeaconTypeExpirationTime, ...)` (the `CatalogShift` path) | FAIL |
| `TestSwamp_ShiftExpired_ProductionPattern_NoDeadlock` | Production shape: 1 shifter loop plus 4 writers upserting 32 keys with past, now and future `ExpiredAt`. No forced ordering. | FAIL, hangs after about 8 operations |
| `TestSwamp_ShiftExpired_ConcurrentDelete_NoDeadlock` | 1 shifter loop plus 4 writers that insert fresh keys and `DeleteTreasure` them (no overwrites) | FAIL, hangs after about 750 operations |

```bash
cd ~/development/hydraide
go test -count=1 -run 'ConcurrentOverwriteSave|ProductionPattern|ConcurrentDelete' ./app/core/hydra/swamp/
```

The tests detect the hang with a watchdog. On failure they print the relevant
goroutine stacks and skip the swamp teardown, because `Destroy()` on a
deadlocked swamp would block the test binary. The whole run takes about 12 s.
The server-side packages are not run by the GitHub workflows (only the SDK and
examples are), so these red tests do not break CI. They are the regression
gate for the fix.

### 2. Private server over gRPC (the reported scenario)

A server was built from `cedbea6`. It ran from a temp `HYDRAIDE_ROOT_PATH` on a
random port with self-signed mTLS certificates, and was killed afterwards. The
client used the Go SDK:

- It registered the swamp `repro/scheduler-queue/pending` (`WriteInterval` 2 s,
  msgpack), the same settings as the Trendizz scheduler queue.
- 1 goroutine ran `CatalogShiftExpired(howMany=10)` in a loop.
- 4 goroutines ran `CatalogSave` of `{key, value, createdAt, expireAt}` over 32
  rotating keys, with `ExpireAt` in the past, now or the future.
- Every call used a 5 s context.

Result: the swamp hung in **under 2 s**, after 157 operations. Follow-up
probes on the hung swamp:

```
probe CatalogSave new key          err=Code: 3, Message: context timeout exceeded (5.0s)
probe CatalogRead existing key     err=Code: 7, Message: key not found (0.0s)
probe CatalogShiftExpired          err=Code: 3, Message: context timeout exceeded (5.0s)
probe Count                        err=<nil> (0.0s)
```

## Goroutine dump of the hung server

The server was started with `GOTRACEBACK=all` and dumped with `SIGABRT`.
`SIGQUIT` does not work for this: `main.go:346` traps it for graceful
shutdown. Excerpt, with frames trimmed; `0x1db627ca0aa0` is the swamp's
`expirationTimeBeaconASC`:

```
goroutine 128 [sync.Cond.Wait]:                          <-- HOLDS expASC.mu, waits for K's guard
guard.(*guard).StartTreasureGuard(...)            guard/guard.go:142
beacon.(*beacon).ShiftExpired(0x1db627ca0aa0, 0xa) beacon/beacon.go:989
swamp.(*swamp).CloneAndDeleteExpiredTreasures(...) swamp/swamp.go:2623
gateway.shiftExpiredOneSwamp(...)                 gateway/gateway.go:1139
gateway.Gateway.ShiftExpiredTreasures(...)        gateway/gateway.go:1108

goroutine 129 [sync.Mutex.Lock]:                         <-- HOLDS K's guard, waits for expASC.mu
beacon.(*beacon).Delete(0x1db627ca0aa0, ...)      beacon/beacon.go:858
swamp.(*swamp).deleteTreasureIfBeaconInitialized  swamp/swamp.go:3015
swamp.(*swamp).SaveFunction(...)                  swamp/swamp.go:2192
treasure.(*treasure).Save(...)                    treasure/treasure.go:2126
gateway.Gateway.Set.func1.1(...)                  gateway/gateway.go:298

goroutine 22 [sync.Mutex.Lock]:                          <-- new-key Set, holds beaconBuildMu, stuck behind the cycle
beacon.(*beacon).Add(0x1db627ca0aa0, ...)         beacon/beacon.go:843
swamp.(*swamp).addToExpirationTimeBeacon(...)     swamp/swamp.go:3346
swamp.(*swamp).addTreasureToBeacons(...)          swamp/swamp.go:2988
swamp.(*swamp).SaveFunction(...)                  swamp/swamp.go:2147
gateway.Gateway.Set.func1.1(...)                  gateway/gateway.go:298

goroutine 160 [sync.Mutex.Lock]:                         <-- next ShiftExpired RPC, queued on expASC.mu
beacon.(*beacon).ShiftExpired(0x1db627ca0aa0, 0xa) beacon/beacon.go:980

goroutine 42 [sync.Cond.Wait]:                           <-- swamp write listener, holds writerLock + chronicler mu
guard.(*guard).StartTreasureGuard(...)            guard/guard.go:142
chronicler.(*chronicler).writeNewTreasures(...)   chronicler/chronicler.go:301
chronicler.(*chronicler).Write(...)               chronicler/chronicler.go:268
swamp.(*swamp).fileWriterHandler(...)             swamp/swamp.go:2866
```

The engine tests print the same pair of frames: `beacon.go:989` or `:1059`
against `swamp.go:2192` → `beacon.go:858`. The delete variant shows
`swamp.go:2931` → `:3005` → `beacon.go:858`.

## Expected vs. observed

- **Expected:** `CatalogShiftExpired`, `CatalogShift`, `CatalogSave` and
  `CatalogDelete` may run concurrently on one swamp. The shift returns the
  rows that are due at that moment, and the writes are serialised per key by
  the treasure guard.
- **Observed:** within seconds under this mix, the swamp deadlocks
  permanently. Every later mutating call and every shift call on it times out
  on the client side until the server process restarts.

## Root cause

`beacon.ShiftExpired` and `beacon.ShiftMatching` acquire per-treasure guards
while holding the beacon's `mu`. Every other path acquires the guard first and
the beacon `mu` second: gateway `Set` → `SaveFunction`, and `deleteHandler`.
The codebase already knows about this ordering. `ShiftMatching`'s own Cap
pre-count (`beacon.go:1033`) and `SelectExpiredForPatchWithCap`
(`beacon.go:1122`) both carry the comment *"Acquiring StartTreasureGuard here
would create a lock-order inversion against any caller that holds a
per-treasure guard and then wants the beacon mu"*. Those two functions avoid
the inversion. The selection loops of `ShiftExpired` and `ShiftMatching` still
have it.

Commit `7e8809f` added the expiration-beacon refresh in `SaveFunction`'s
modified branch. Since then, every overwrite with `ExpiredAt` takes part in
the cycle, which turned a rare race (delete only) into one that hangs within
seconds.

Same pattern, not reproduced here, worth checking during the fix:
`beacon.CloneUnorderedTreasures` (`beacon.go:1326`/`:1331`) and `ShiftMany`
(`:953`/`:961`) also take guards under `b.mu`. `CloneUnorderedTreasures` runs
on `beaconKey`, from `swamp.CloneTreasures` (`swamp.go:2525`) and from the
auto field-bucket index build (`swamp_bucket.go:53`). `SaveFunction` calls
`s.beaconKey.Get()` (an `RLock`) while the caller holds the guard
(`swamp.go:2125`), so a bucket build racing an overwrite looks like the same
kind of cycle, on `beaconKey`.

## Suggested fix directions

1. **Select without guards, then guard each selected row outside the beacon
   lock (recommended).** This mirrors `PatchExpired`.
   - Under `b.mu`, choose the rows using the guard-free getters. `ExpirationTime`
     and the body are protected by the treasure's own `t.mu`, as the Cap count
     already relies on. Remove the chosen rows from `treasuresByOrder` and
     `treasuresByKeys`, then release `b.mu`.
   - Outside the lock, for each chosen row: take its guard, re-check the
     predicate (`exp != 0 && exp < now`, or the filter), then clone and delete
     it under that same guard. Delete via a `deleteHandler` variant that
     accepts an existing `guardID`.
   - If the re-check fails, re-insert the row into the beacon, as
     `ReindexExpiration` does for `PatchExpired`. That happens when a
     concurrent `Save` moved `ExpiredAt` into the future or the row no longer
     matches.
   - Trade-offs: the shift is no longer one atomic snapshot. A row can be
     selected and then released back when it no longer qualifies. This is the
     same semantics `PatchExpired` already has, and it is correct for a queue.
     It needs care around the key being re-created between selection and
     delete, the same interplay that the `treasuresWaitingForWriter.Delete` in
     `SaveFunction` handles.
2. **Minimal hotfix: try-lock inside the scan.** In `ShiftExpired` and
   `ShiftMatching`, use `StartTreasureGuard(false)`. A row whose guard is busy
   is being written right now: treat it as remaining and shift it on a later
   call.
   - Trade-offs: a very small diff, and all four tests above should go green.
   - `HowMany` can be under-filled while writes are in flight. That is
     harmless for polling consumers.
   - It does not fix the same pattern in `CloneUnorderedTreasures`, which needs
     a full snapshot and cannot skip rows.
3. **Reduce exposure (complements 1 or 2, not a fix on its own).**
   `expirationTimeBeaconASC` is kept sorted: `addToExpirationTimeBeacon` calls
   `SortByExpirationTimeAsc`. `ShiftExpired` can therefore read `exp` without
   a guard and stop at the first row that has not expired, instead of taking a
   guard on every row of the swamp. This also removes an O(N) guard walk from
   every poll.
4. Optionally, make `SetExpirationTime` set `expirationTimeChanged` only when
   the value actually changes. That removes the beacon reshuffle for
   idempotent re-saves. It does not remove the deadlock: a real `ExpiredAt`
   move, and `Delete`, still invert.

Not recommended: making `SaveFunction` take the beacon locks before the guard.
The guard is acquired by the gateway before `Save` is called, so enforcing
that order would mean restructuring every write path.

## Impact on Trendizz production

Source grep of `~/development/trendizz-monorepo` (read-only). A swamp is at
risk when a shift runs on it concurrently with an **overwrite of an existing
key that carries `ExpireAt`**, or with a **`CatalogDelete`**. Append-only
swamps with unique keys are not in the cycle, because the new-key `Save` path
does not hold a guard that the shifter waits for.

| Swamp / call site | Shift | Concurrent overwrite / delete on the same swamp | Risk |
|---|---|---|---|
| `meeting/scheduler-queue/pending`: `meeting_service/model_scheduler_queue.go` (`shiftDue`, worker `meeting_scheduler_worker` every 60 s) | `CatalogShiftExpired` (prod); `CatalogShift` with `FilterExpiredAt`+`FilterBytesFieldStringIn` (tests) | `scheduleAction` upserts `{meetingID}:{action}` with a new `ExpireAt`. `cancelAction`/`cancelAllScheduled` issue `CatalogDelete` for up to 5 keys. Both come from HTTP handlers. | **Yes.** Low traffic, so the window is narrow. When it hits, all meeting time-based actions (expire, T-1 day, T-10 min, no-show) stop, and schedule/cancel calls fail with `ErrInternal` after 5 s, until `hydraserver` is restarted. |
| inference `log_service/model_error_bucket_catalog.go` | `CatalogShiftExpired` (cleanup) | Read-modify-`CatalogSave` of the same bucket key with `ExpireAt = now+24h` on every error | **Yes**, the highest overwrite rate of all sites |
| `lock_service/model_lock_catalog.go` | `CatalogShiftExpired` (expiry cleanup) | `Save`/`SaveMany` of lock keys (refresh overwrites the key) plus deletes on unlock | **Yes** |
| `email_service/model_domains_waiting_for_sending_catalog.go` | `CatalogShiftExpired(howMany=1)` from the sender | `CatalogSave`/`CatalogSaveMany` with `ExpireAt`; re-queuing a domain overwrites its key | **Likely** |
| `email_service/model_campaign_waiting_domains_catalog.go` | `CatalogShift` (filtered) | `CatalogSave`/`CatalogSaveMany` | **Likely** if keys are re-saved |
| `shared/crawlerdomain` DNS-timeout swamps (crawler `prefilter/pool.go`) | `CatalogShiftExpiredManyFromMany` (server: `shiftExpiredOneSwamp` → same engine path) | `CatalogSave` of the domain key with `ExpireAt = now+24h` | **Possible** when a domain times out again before it is shifted |
| inference `model_metric_history_catalog.go` (minute key) | `CatalogShiftExpired` | `CatalogSave` of a per-minute key, which can overwrite within the same minute | **Possible** |
| inference call log / call detail, crawler activity log, public-API call log | `CatalogShiftExpired` | unique append-only keys | Not in the cycle unless those keys are deleted or overwritten |

A hung swamp stays hung. The only recovery is restarting the HydrAIDE process
(`systemctl restart hydraserver-...`), and that can lose whatever sat in the
hung swamp's write buffer, because the write listener is blocked.

## Workaround until fixed

- **Serialise the shift against writes and deletes of the same swamp in the
  client.** For the meeting scheduler, every writer (`scheduleAction`,
  `cancelAction`) and the shifter (`shiftDue`) live in the API process, and
  workers run in a single instance, so one package-level `sync.Mutex` around
  those functions removes the concurrency. The same approach works for any
  single-process owner of a queue swamp. It does not help when several
  processes write to the same swamp.
- **Avoid overwrites on shifted swamps where possible.** Do not re-save an
  existing key's `ExpireAt` while a shifter may be running. A `CatalogDelete`
  followed by a save does not help, because `Delete` is also part of the
  cycle.
- **Operational signature for detection:** on one swamp, repeated client
  `context timeout exceeded` errors on `Set`/`ShiftExpiredTreasures` while
  `Count` still answers. Recovery is a HydrAIDE restart. Run the Trendizz
  fleet shutdown/restart sequence (skill `trendizz-hydraide-ops`) rather than
  a bare restart where possible.

## Status

Open. The regression gate is the four tests in
`app/core/hydra/swamp/swamp_shift_guard_deadlock_test.go`. They must go green
without weakening their watchdogs.
