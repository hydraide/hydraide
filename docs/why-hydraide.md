# Why HydrAIDE exists

HydrAIDE was not built to compete with the data engines that exist. It was built because, for one specific workload at [Trendizz](https://trendizz.com), every off-the-shelf option ran out of room — for different reasons, all at the same time.

This page is the origin story. It's an honest account of what we tried, what broke, and the architectural realisation that pushed us to stop combining tools and start building a single one.

## The workload that started it (2021)

We were building a B2B keyword search across the European web. The shape of the data was unusual:

- **2M+ websites** indexed at the time, with the corpus expected to keep growing.
- We stored every **word** separately, and under each word we kept the full set of domains where it appears.
- The words themselves were sharded: in practice the system had to manage **tens of millions of independent storage units** (what we now call Swamps).
- Search across the entire European corpus had to return in **under 3 seconds**, end to end.
- The hardware budget was **128 GB of RAM** and several SSDs. The dataset on disk was **multiple terabytes**.

The breakthrough we needed was: given a search query, figure out *which* mini-stores to open, load them into memory at full speed, run the set logic, and respond — all of it within the latency budget, on a single server.

This is the workload that *forced* HydrAIDE into existence. HydrAIDE still runs the full Trendizz stack today, with significantly more data on the same class of hardware than the original 2021 prototype could have handled — the result of years of incremental optimisation in storage layout, memory usage, and access patterns.

## What we tried, and what broke

A note on credibility: I had been a serious PostgreSQL user for years before this — including running enterprise-scale ad-serving systems on it. The "Postgres did not work" story below is not someone bouncing off SQL, it's someone who knew Postgres very well and watched it become the wrong tool for *this* shape of data.

### PostgreSQL — first attempt

Tried first. Ran into deadlocks under our concurrent write patterns. Performance fell off after tens of millions of rows. The storage model didn't match what we needed (millions of small, naturally partitioned units rather than a few large tables). We could not get the European-wide search under our 3-second target. Postgres remains an excellent choice for relational, transactional, ad-hoc-queryable data — it just isn't built for this shape.

### MongoDB

Tried next. The RAM footprint was the killer: multi-TB data on a 128 GB box doesn't work when the engine wants the working set resident. The same constraint ruled out any "everything must live in RAM" engine. Disk-based B-tree designs were the other obvious direction — those came up too slow on the kinds of scans we needed.

### Redis

Looked at it briefly and ruled it out. Redis is fine for what it is, but the workload was multi-TB structured data, not a working set that fits in memory.

### ArangoDB

Evaluated. Didn't solve the core problem either — same fundamental tension between memory budget, disk speed, and how the data wanted to be sharded.

### Cloud and managed services (DynamoDB, AWS, Google)

Ruled out on economics. We were funding a startup and could not afford to spend our runway on cloud storage, egress, and per-operation pricing at this data volume. We needed a single beefy server we owned.

### Microsoft SQL, Oracle

Ruled out on licensing. Per-CPU-core pricing on engines like these is a non-starter for a workload that wanted to use every core on a multi-CPU box.

## The pattern that kept breaking

Looking at what every option had in common, the failure mode was structural, not implementation-specific:

- **Either the engine wanted everything in RAM** — and our data was multi-TB on 128 GB.
- **Or it couldn't shard naturally** onto multiple physical SSDs in a way that mapped to how our data was actually partitioned (per word, per domain).
- **Or it was disk-bound through a B-tree** that walked too many pages for our latency budget.
- **And in every case, productive use required deep DB-specific expertise.** A strong engineer who knew Postgres deeply was useless for the Mongo path, and vice versa. Each tool added a hiring constraint on top of the technical problem.

Underneath all of that was a single mismatch: every engine assumed the data lives in *one logical place* and gets accessed via a *query language*. Our data was already split into millions of independent units by nature, and the access pattern was *"open exactly the right small store, do work on it, close it"*. The query language was overhead in the path; the central coordination was overhead in the path.

## The decision

By 2021 the conclusion was clear: combining off-the-shelf engines was producing more operational complexity than the actual problem deserved. So I sat down to design a single engine specifically for this shape of workload.

The non-goals were as important as the goals. **I did not want to invent another SQL or NoSQL dialect.** I did not want to ship yet another query engine. Adopters had to be productive without learning a new language at all.

Instead I started from a few first principles:

- **The code is the schema.** The Go struct in your application is the record on disk. No schema definition step, no migration step, no second language between the developer and the data.
- **The developer decides what lives in memory and for how long.** Per Swamp, configurable. The engine doesn't second-guess.
- **The unit of partition is natural to the domain.** One Swamp per word, per domain, per tenant, per user — whatever the natural unit is. The engine routes to it deterministically without a coordinator.
- **Indexes are built on demand, not maintained forever.** Most engines keep persistent caches and indexes around so that lookups are fast. HydrAIDE builds the internal indexes a Swamp needs *on the fly*, only when something is actually being read or filtered, and discards them when the Swamp evicts. We pay the cost when there is real work to do, and pay nothing — in disk space or RAM — for indexes nobody is currently using.
- **No external broker for events.** The engine itself emits change events. The Trendizz dashboard had to be fully reactive without us running a Kafka or Redis pub/sub alongside it.
- **Scale by adding Swamps, not by reshaping a global table.** Growth was a normal-mode operation, not a re-architecture.
- **One process to run.** A startup running on its own metal cannot afford five operationally-distinct services to keep alive. The engine had to do storage, indexing, pub/sub, and TTL in one binary.

## What it became

HydrAIDE has been running in production at Trendizz since 2024 and is being actively developed. It is production-ready, but it is not finished — every year we have shaved more off storage overhead, made the lifecycle smarter, added field-level atomic patches, added server-side filters and vector / geo / phrase queries.

What we deliberately did not chase: SQL compatibility, OLAP, multi-key cross-Swamp transactions. Those are real needs that real engines solve well — they just are not what HydrAIDE is for. See the [README's "What HydrAIDE is not for"](../README.md#what-hydraide-is-not-for) section for the honest list of non-fits.

## A note on AI-assisted development

There is a happy accident worth flagging. The same properties that let a human Go developer be productive on day one — code is the schema, no separate query language, native Go structs round-trip to disk and back — turn out to make HydrAIDE unusually easy for AI coding assistants to work with. Claude and similar tools generate correct HydrAIDE models on the first try, because there is no second mental model to bridge.

We did not build HydrAIDE for AI. We built it so that humans could stay in their codebase. AI tools happen to benefit from the same shape, which is a useful coincidence in 2026 — and one of the reasons we ship [Claude Code skills](../.claude/skills/) and [project guidance](../CLAUDE.md) alongside the engine itself.

## Why this matters if you're evaluating HydrAIDE

If your data already looks like *"millions of natural namespaces"* — per-tenant, per-user, per-device, per-agent, per-document, per-domain — and you would otherwise be stitching together a database, a cache, a pub/sub, and a scheduler to make it work, HydrAIDE is the engine that came out of that exact problem.

If your data is one big relational table with foreign keys and analytical queries, that is not the workload HydrAIDE was born from, and it is not what you should be reaching for.
