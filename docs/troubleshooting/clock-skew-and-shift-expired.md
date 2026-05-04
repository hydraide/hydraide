# `CatalogShiftExpired` flake — clock skew, not a HydrAIDE bug

If you've landed here, you're probably chasing a flaky test or production
incident that looks like:

- `CatalogSave` with `ExpireAt = time.Now().Add(-1 * time.Second)` (or another
  small negative offset) **succeeds**.
- An immediate `CatalogShiftExpired` on the same Swamp returns **0 entries**.
- But `Count`, `IsKeyExists`, and `CatalogRead` all confirm the entry is in
  the Swamp with the past `ExpireAt`.
- Locally (single machine, client and server share a clock) the same code
  passes 100%.
- Against a deployed HydrAIDE on another host, it fails intermittently or
  repeatedly with no obvious trigger.

**This is almost always client/server clock skew, not a HydrAIDE correctness
bug.** This page explains why, how to confirm, and how to fix it.

## Why it happens

`CatalogShiftExpired` is a *server-side* operation. The server iterates the
expiration index of the Swamp and returns Treasures whose `ExpirationTime`
is strictly less than the server's own `time.Now().UTC().UnixNano()`:

```go
// internal: app/core/hydra/swamp/beacon/beacon.go
if treasureObj.GetExpirationTime() < time.Now().UTC().UnixNano() {
    // shift it out
}
```

The client, on the other hand, encodes `ExpireAt` against the *client's*
`time.Now()` and ships it over gRPC as a `google.protobuf.Timestamp`.

If the client clock is ahead of the server clock by Δ:

- Client computes `ExpireAt = client_now − 1s`.
- Server receives `ExpireAt = client_now − 1s`.
- Server compares against its own `now`, which is `client_now − Δ`.
- The check becomes `(client_now − 1s) < (client_now − Δ)`, i.e.
  `−1s < −Δ`, i.e. `Δ < 1s`.

So **whenever the client clock leads the server clock by more than your
"-1s" margin, the server thinks the entry is still in the future** and
declines to shift it. The entry is genuinely persisted; it just isn't
expired *yet* from the server's authoritative point of view.

This is the correct semantic for a distributed system: the server is the
source of truth for "is this expired". Allowing the client's clock to
override would let a misbehaving client pull entries early and break TTL
guarantees for everyone else.

## How to confirm it's clock skew

### Quick check — measure skew

```bash
A=$(date -u +%s%3N)
B=$(ssh user@hydraide-host 'date -u +%s%3N')
echo "skew=$((A-B)) ms (positive means client is ahead of server)"
```

Anything ≳ 200ms is enough to make small-margin `ExpireAt` flaky over time
as NTP drifts back and forth. Multi-second skew makes it deterministic.

### Forensic check — read the actual nanos from the server

If you can enable debug logging on the server, log the `ExpirationTime` and
the server-side `time.Now().UTC().UnixNano()` at the moment of the
comparison. Compare them: if `expT > now`, the entry isn't expired *yet*
according to the server, regardless of what the client thinks. Convert the
nanos to ISO timestamps to see the gap in human terms.

### Cross-check — does it pass locally?

Run the same code with both client and server on a single machine (e.g.
`localhost:5950`). If it passes 100% there, the code is correct and the
deployed-environment failure is environmental, not algorithmic.

## How to fix it

### In application code — use a generous margin

Replace tight margins:

```go
// ❌ flaky — fails whenever clock skew > 1s
ExpireAt: time.Now().Add(-1 * time.Second)
```

with a margin that comfortably exceeds realistic clock skew:

```go
// ✅ robust — survives multi-second NTP excursions
ExpireAt: time.Now().Add(-30 * time.Second)
```

`30 * time.Second` is a safe default for normally-NTP-disciplined hosts.
Use `-1 * time.Minute` if your environment has known clock issues
(virtualization, containers without time sync, satellite/edge nodes).

This applies anywhere "treat as expired right now" semantics is needed —
e.g. requeue-on-recovery flows, immediate retry queues, dead-letter sweeps.

### In infrastructure — keep clocks in sync

Run `chrony` or `systemd-timesyncd` on every HydrAIDE host. Aim for skew
under 100ms. Containers should mount the host clock or run their own NTP
client; otherwise they can drift seconds or even minutes from the host.

Quick verification:

```bash
ssh hydraide-host 'chronyc tracking' # or: timedatectl status
```

Look for "System clock synchronized: yes" and an offset under 100ms.

### Don't try to "fix" it in HydrAIDE

The temptation is to add a tolerance window in HydrAIDE — e.g.
"shift if `expT < now + 5s`". Don't. It would:

- Let the client unilaterally pull entries up to 5s before their TTL,
  breaking the contract for every consumer.
- Hide real problems (a host with chronic clock skew is a problem worth
  knowing about).
- Introduce a new tunable that callers will misconfigure.

The right place to handle skew is in the application's choice of margin,
or — better — by keeping the clocks aligned.

## Why HydrAIDE itself isn't buggy

Three pieces of evidence:

1. The HydrAIDE in-process tests for this exact pattern
   (`TestShiftExpiredThenSaveSameKey_Persistence`,
   `TestSaveExpiredThenImmediateShift`) pass 100% — same code, same
   chronicler V2, same beacons, no gRPC, no clock skew.
2. A locally-built HydrAIDE server (same git commit as the deployed one)
   passes the same Save→ShiftExpired hammer 600+/600+ when client and
   server share a clock.
3. Server-side debug logs show the entry IS in `beaconKey`, IS in
   `expirationTimeBeaconASC.treasuresByKeys`, IS in
   `expirationTimeBeaconASC.treasuresByOrder`, but the
   `expT < server_now` check returns false — by exactly the magnitude of
   the measured client/server clock skew.

The expiration check is correct. The "bug" is upstream: the application
asks the server to expire an entry that, from the server's clock's
perspective, hasn't expired yet.

## Summary checklist

When `CatalogShiftExpired` returns 0 unexpectedly:

- [ ] Measure client/server clock skew (`date -u +%s%3N` on both).
- [ ] Check the `ExpireAt` margin in the failing call. If it's `< 5s`, that's
      the cause — widen it to `30s+`.
- [ ] Reproduce locally. If local passes, it's environmental.
- [ ] Add NTP/chrony to the deployed host if missing.
- [ ] Only after the above: open a HydrAIDE issue with a self-contained
      reproducer that runs on a single host (so clock skew is excluded).
