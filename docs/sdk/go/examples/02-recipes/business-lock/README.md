# 02-recipes/business-lock

Application-level distributed lock with TTL.

## What's the difference vs. the engine's per-key lock?

| | Per-key write lock | Business lock |
|---|---|---|
| Granularity | One Treasure key | Anything you give a name to |
| Scope | Inside a Swamp | Cross-Swamp, cross-process |
| Activation | Automatic on every write | Explicit `Lock` / `Unlock` |
| Use case | Engine internals | Application invariants |

Business locks let you say "only one worker may run this rollup",
"only one process may claim this user", "only one publisher may push
to this channel" — and have the engine enforce it without a Redis
mutex or a Postgres advisory lock.

## TTL safety

If a holder crashes without releasing, the engine releases the lock
when its TTL expires. This is why `Lock` requires a TTL: you cannot
build a deadlock-prone system with it.

## What this recipe does

Two goroutines race for the same lock. The engine queues them — the
first wins, holds for 1 second, releases; the second then acquires.
The recipe asserts the two acquire/release windows do not overlap.

## Run it

```bash
docker compose up -d
make recipe-business-lock
```

## Test it

```bash
make test-examples
```
