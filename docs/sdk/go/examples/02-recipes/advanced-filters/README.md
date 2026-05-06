# 02-recipes/advanced-filters

Server-side filtering with AND/OR logic and field-level predicates on
msgpack values.

## What it shows

Three queries against a small product catalog:

1. **AND**: `category == "books" AND priceCt > 1500`
2. **OR**: `category == "books" OR category == "music"`
3. **IN-style**: only specific SKUs, restricted via `Index.IncludedKeys`

## Why server-side filters matter

Filters are evaluated **inside the engine**, on the same goroutine that
reads the data, against the msgpack bytes already in memory. The client
never receives Treasures that don't match the predicate. There is no
client-side filter loop, no over-fetching, no SELECT-then-throw-away.

## Filter primitives used here

| Primitive | What it does |
|---|---|
| `FilterAND(items...)` | All items must match |
| `FilterOR(items...)` | At least one item must match |
| `FilterBytesFieldString(op, path, value)` | Compare a msgpack string field |
| `FilterBytesFieldInt64(op, path, value)` | Compare a msgpack int64 field |
| `Index.IncludedKeys` | Whitelist of Treasure keys (IN-style) |

The full set includes `FilterBytesFieldStringIn` for native `IN`
matching, plus phrase, vector, geo-distance and nested-slice filters.
See `sdk/go/hydraidego/hydraidego.go` for the complete list.

## Run it

```bash
docker compose up -d
make recipe-advanced-filters
```

## Test it

```bash
make test-examples
```
