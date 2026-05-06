# HydrAIDE Go examples

Runnable Go examples for the HydrAIDE SDK. Every example here:

- Connects to a real HydrAIDE instance.
- Has a `main.go` you can run with `go run .`.
- Has a `*_test.go` that exercises the same code path under
  `-tags=integration`.
- Cleans up its own swamps so re-runs are conflict-free.

## Prerequisites

One command brings up a local HydrAIDE plus auto-generated TLS material:

```bash
docker compose up -d
```

The init container generates `ca.crt`, `client.crt`, `client.key`,
`server.crt`, and `server.key` into `./certificate/`. The `internal/setup`
helper finds them automatically — no environment variables required for
the in-tree examples.

## Layout

```
examples/
├── docker-compose.yml          # one-command local instance + cert init
├── Makefile                    # make quickstart / make recipe-* / make test-examples
├── .env.example                # public defaults
├── internal/
│   └── setup/                  # shared connect helper, also used by tests
├── 01-quickstart/              # 5 minutes: connect → save → read → subscribe → exit
├── 02-recipes/                 # one folder per use case
│   ├── ttl-queue/              # delayed task queue using CatalogShiftExpired
│   ├── atomic-patch/           # field-level mutation via PatchTreasures
│   ├── live-subscribe/         # real-time events on every write
│   ├── profile-per-user/       # Swamp-per-user with the Profile model
│   ├── catalog-list/           # CatalogReadMany pagination via Index
│   ├── atomic-counter/         # IncrementInt64 under concurrent writers
│   ├── business-lock/          # named distributed lock with TTL
│   └── advanced-filters/       # AND/OR + field-level + IN-style filters
└── 03-reference-apps/          # end-to-end CRUD apps over fasthttp + Postman/OpenAPI
    ├── todo-api/                # textbook CRUD with PatchTreasures + status filter
    ├── url-shortener/           # short codes + IncrementInt64 click counters
    └── multi-tenant-saas/       # one Swamp per tenant + business lock
```

## Where to start

| If you want to … | Read / run … |
|---|---|
| See HydrAIDE work end-to-end in 80 lines | [`01-quickstart/`](01-quickstart/) |
| Schedule deferred work without Redis or Kafka | [`02-recipes/ttl-queue/`](02-recipes/ttl-queue/) |
| Patch a single field without a read-modify-write round-trip | [`02-recipes/atomic-patch/`](02-recipes/atomic-patch/) |
| React to swamp changes in real time | [`02-recipes/live-subscribe/`](02-recipes/live-subscribe/) |
| Store one record per user/tenant in its own Swamp | [`02-recipes/profile-per-user/`](02-recipes/profile-per-user/) |
| List and paginate a Catalog with an ordered index | [`02-recipes/catalog-list/`](02-recipes/catalog-list/) |
| Run safe concurrent counters without a client mutex | [`02-recipes/atomic-counter/`](02-recipes/atomic-counter/) |
| Coordinate workers with a named lock and TTL | [`02-recipes/business-lock/`](02-recipes/business-lock/) |
| Filter by msgpack-field with AND/OR/IN logic | [`02-recipes/advanced-filters/`](02-recipes/advanced-filters/) |
| See an end-to-end CRUD HTTP API over HydrAIDE | [`03-reference-apps/todo-api/`](03-reference-apps/todo-api/) |
| See atomic counters in a real read-heavy service | [`03-reference-apps/url-shortener/`](03-reference-apps/url-shortener/) |
| See Swamp-per-tenant isolation with business locks | [`03-reference-apps/multi-tenant-saas/`](03-reference-apps/multi-tenant-saas/) |
| Test your own HydrAIDE models the same way | [`docs/sdk/go/testing.md`](../testing.md) |

## Common commands

```bash
make up                    # start the local HydrAIDE
make quickstart            # run 01-quickstart
make recipe-ttl-queue      # run a specific recipe
make recipe-atomic-patch
make recipe-live-subscribe
make app-todo-api          # start a reference app on :8080
make app-url-shortener     # start the short-link service on :8081
make app-multi-tenant-saas # start the per-tenant user store on :8082
make test-examples         # integration tests against the running instance
make down                  # stop and remove the local instance
```

## Pointing tests at a different instance

Create `.env.local` (gitignored) at the example tree root:

```
HYDRA_HOST=192.168.106.100:8030
HYDRA_CERT=/absolute/path/to/certificate
```

The Makefile sources `.env.local` automatically when present. **Use this
only with unit-test HydrAIDE instances, never with live data.** See
[`testing.md`](../testing.md) for the full rationale.
