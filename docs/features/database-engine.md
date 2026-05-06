## Database engine — model and operation

HydrAIDE does not use schemas or SQL. The shape of the data lives in the developer's struct definition; the engine stores values in their native binary form. Where richer access is needed, the [query engine](query-engine.md) provides server-side filters, vector and geo predicates, streaming results — without a SQL surface.

The reference SDK is in Go: any struct you define is stored binary, type-preserving, and loaded back as the same struct. The wire protocol is gRPC, so any language with protoc support can talk to the server directly without an SDK. See [pure gRPC control](pure-grpc-control.md).

### Why it was designed this way

HydrAIDE was built for [Trendizz.com](https://trendizz.com) — a B2B search system that needed to store crawl data from millions of websites and make it available in real time, on a single server, without a separate cache or pub/sub layer. The existing options (SQL, NoSQL, Redis, Kafka) did not combine the speed, real-time reactivity, and memory efficiency required. The result was the **Swamp** — a physical and logical unit that owns the location, behaviour, and lifecycle of a slice of the data.

### How it works in practice

To store data, you simply define a Go struct, which itself becomes the Swamp — the storage unit — for example, a user profile:

```go
type UserProfile struct {
    Name   string
    Age    uint8
    Active bool
}
```

Then save it in Profile mode:

```go
profile := &UserProfile{
    Name:   "Alice",
    Age:    34,
    Active: true,
}

h.ProfileSave(ctx, name.New().Sanctuary("users").Realm("profiles").Swamp("alice"), profile)
```

No field definitions on the database side, no `ALTER TABLE`, no JSON conversion. The data is stored in its native binary form and loaded back exactly the same way.

**What happens on save:** HydrAIDE resolves the Swamp name deterministically, computes the target folder/server, creates the Swamp on first access, stores the struct fields as binary **Treasures**, emits events to subscribers, and — if persistence is enabled — flushes the write to disk in the background.

For storage-engine measurements (insert/update/delete/read latencies and on-disk size), see [V2 benchmark results](../benchmarks/V2_RESULTS_SUMMARY.md).

### What you get

- **O(1) routing.** Swamp name is hashed deterministically to a folder; no central index, no scan.
- **One language.** The struct is the schema. See [struct-first data model](struct-first-data-model.md).
- **Reactivity built in.** Every write emits an event over a Subscribe stream. See [reactivity & subscriptions](reactivity-and-subscription-logic.md).
- **Memory only on access.** Swamps load on first use and evict after a configurable idle window. See [Swamp lifecycle](swamp-lifecycle.md).

For server-side filtering, vector similarity, and geographic queries on top of this model, see the [query engine](query-engine.md).
