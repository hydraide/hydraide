# 02-recipes/ttl-queue

A delayed task queue built on a single HydrAIDE swamp. No Redis, no Kafka,
no scheduler service.

## How it works

- The queue is **one Catalog swamp** (`examples/ttl-queue/emails`).
- Each task is a Treasure with an `ExpireAt` timestamp.
- The consumer calls `CatalogShiftExpired`, which **atomically reads and
  deletes** every Treasure whose `ExpireAt` has passed. Two consumers can
  run side by side — the engine guarantees each task is delivered to
  exactly one of them.

## When to use this pattern

- Delayed email delivery, retry scheduling, deferred notifications.
- Reactive workflows where "do this in N seconds" is the simplest API.
- Any time you'd reach for `Redis ZADD` + a polling worker.

## Run it

```bash
docker compose up -d        # if not already up
make recipe-ttl-queue
```

Or directly:

```bash
cd 02-recipes/ttl-queue
go run .
```

## Test it

```bash
make test-examples
```

## Gotcha — `ShiftExpired` returns 0 against a remote HydrAIDE

If your test enqueues a task with `ExpireAt` set just barely in the
past and the immediate `CatalogShiftExpired` call returns no entries —
locally everything works, but against a deployed instance it flakes —
the cause is almost always **client/server clock skew**, not a
HydrAIDE bug. Full debug recipe:
[Clock skew and `ShiftExpired`](../../../../troubleshooting/clock-skew-and-shift-expired.md).
