# 01-quickstart

The smallest end-to-end demo. About 80 lines of code, runs against a live
HydrAIDE instance, exits cleanly.

## What it does

1. Connect to HydrAIDE.
2. Register a swamp pattern under `examples/quickstart/*`.
3. Save one `User` Treasure (`alice` with score 10).
4. Read it back.
5. Subscribe to the swamp.
6. Update `alice` (score 10 → 11) so the subscription has something to print.
7. Wait for the subscription event to arrive.
8. Destroy the swamp (so the demo can be re-run idempotently).

## Run it

From the example tree root:

```bash
docker compose up -d        # in-tree HydrAIDE + auto-generated certs
make quickstart
```

Or directly:

```bash
cd 01-quickstart
go run .
```

Expected output:

```
saved alice: NEW
read alice: email=alice@example.com score=10
subscribe event: id=alice status=NEW score=10        # initial event
updated alice (score 10 → 11)
subscribe event: id=alice status=MODIFIED score=11   # the patch landed
destroyed quickstart swamp
done.
```

## Test it

```bash
make test-examples
```
