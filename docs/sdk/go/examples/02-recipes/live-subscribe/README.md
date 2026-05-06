# 02-recipes/live-subscribe

React to swamp changes in real time using `Subscribe`. No Kafka, no NATS,
no separate pub/sub — the engine itself is the broker.

## What it shows

- Open a `Subscribe` stream against an order swamp.
- Insert three orders.
- Update one of them.
- Delete one of them.
- Watch the subscriber print five events in order, then exit cleanly.

## Why this matters

Most data engines need a separate event bus to deliver "this row changed"
notifications. HydrAIDE emits them on the same gRPC connection that wrote
the data. The latency between a write and the corresponding event is
sub-millisecond on the same machine.

The `Subscribe` callback receives:

- `model` — a freshly populated `*Order`
- `eventStatus` — `StatusNew`, `StatusModified`, `StatusDeleted`, or
  `StatusNothingChanged` (only emitted during the `getExistingData=true`
  catch-up phase)
- `err` — non-nil if a single event failed to decode (non-fatal)

## Run it

```bash
docker compose up -d
make recipe-live-subscribe
```

Expected output (event lines may interleave with the write lines):

```
inserted order-1
event: id=order-1 status=NEW status=pending cents=1000
inserted order-2
event: id=order-2 status=NEW status=pending cents=2000
inserted order-3
event: id=order-3 status=NEW status=pending cents=3000
updated order-2 (status pending → shipped)
event: id=order-2 status=MODIFIED status=shipped cents=2000
deleted order-3
event: id=order-3 status=DELETED deleted
destroyed live-subscribe swamp
done.
```

## Test it

```bash
make test-examples
```
