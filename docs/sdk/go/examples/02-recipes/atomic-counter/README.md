# 02-recipes/atomic-counter

Atomic int64 counters via `IncrementInt64`.

## Why this matters

Read-modify-write counters race under concurrent writers. Two goroutines
read 5, both compute 6, both write 6 — the second increment is lost.

`IncrementInt64` sends a single delta to the engine, which applies it
under the per-key guard. The recipe spawns **100 goroutines that each
add 1** to the same key, then asserts the final value is exactly 100. No
client-side mutex, no Redis `INCR`.

## Bonus capability

`IncrementInt64` also supports a server-side **condition** (only apply
the delta if the current value satisfies a relational operator) and
**setIfNotExist / setIfExist** metadata descriptors that fire in the
same atomic call. See the SDK docs for the full signature.

## Run it

```bash
docker compose up -d
make recipe-atomic-counter
```

## Test it

```bash
make test-examples
```
