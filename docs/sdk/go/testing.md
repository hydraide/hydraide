# Testing HydrAIDE models against a live instance

Every recipe and reference app in [`docs/sdk/go/examples/`](examples/)
ships with an integration test suite that runs against a real HydrAIDE
instance. This page documents the conventions so you can write the same
kind of tests for your own HydrAIDE-backed code.

## Why tests hit a live instance, not a mock

- A mock of HydrAIDE is a mock of your understanding of HydrAIDE. The
  parts most likely to break (encoding, indexing, subscription ordering,
  per-key locking semantics) are exactly the parts a hand-rolled mock
  cannot reproduce faithfully.
- HydrAIDE is light enough that a dedicated test instance costs nothing.
  A single binary or a `docker compose up` is the entire setup.
- Running real writes through the engine catches SDK regressions the
  moment they happen — both for users of HydrAIDE and for HydrAIDE itself.

This is the same rule the Trendizz monorepo enforces for its production
services: tests run against unit HydrAIDE instances, never against live.

## The four core patterns

Every test in the example tree applies these four patterns. They are
enough on their own to test most HydrAIDE models without help.

### 1. A shared connection helper

One place owns connection setup. Every test (and every runnable example)
goes through it. In the example tree this is
[`internal/setup`](examples/internal/setup/setup.go); the equivalent in
the Trendizz monorepo is `utils/testhelper/`.

```go
r, tracker := setup.NewTestClient(t)
```

`NewTestClient` reads `HYDRA_HOST` and `HYDRA_CERT` from the environment
(falling back to the in-tree compose defaults), connects, and registers a
`t.Cleanup` that destroys every Swamp the test recorded.

### 2. Pattern registration

Before saving anything, register the swamp pattern. This tells HydrAIDE
how to lay out files on disk, what encoding to use, and how long to keep
the swamp resident in memory.

```go
err := setup.Pattern(ctx, r, name.New().
    Sanctuary("examples").Realm("atomic-patch").Swamp("*"))
```

The `*` is a wildcard that covers every swamp under that
sanctuary/realm. You register the pattern once per test suite (or once
per process for a long-running app).

### 3. Per-test Swamp isolation

Each test uses a swamp namespace derived from the test name (or simply
declared by the recipe — recipe tests can rely on the recipe owning its
own swamp). Two tests in the same package never write to the same swamp.

```go
tracker.Track(PatchSwamp())  // record the swamp for cleanup
```

The tracker tears down every recorded swamp at the end of the test, so
re-runs and parallel runs are conflict-free.

### 4. Same code in `main.go` and `_test.go`

The runnable example calls an exported function (e.g. `RunAtomicPatch`).
The test calls the same function. There is no test-only code path, no
`if testing { ... }` branches, and no risk of the docs drifting from what
CI verifies.

```go
// main.go
func RunAtomicPatch(ctx context.Context, r repo.Repo) error { /* ... */ }

// main_test.go
func TestAtomicPatch(t *testing.T) {
    r, tracker := setup.NewTestClient(t)
    tracker.Track(PatchSwamp())
    if err := RunAtomicPatch(ctx, r); err != nil { t.Fatal(err) }
}
```

## Build tag convention

All HydrAIDE integration tests carry `//go:build integration` at the top
of the file. This way `go test ./...` from the repo root does not require
a running HydrAIDE — only `go test -tags=integration ./...` does.

```go
//go:build integration

package main
```

The Makefile target `make test-examples` adds the tag for you.

## Running against a non-default instance

By default the tests target the `docker compose` instance defined in
[`examples/docker-compose.yml`](examples/docker-compose.yml). To point at
a different HydrAIDE — typically a unit-test instance on your network —
create a `.env.local` file at the example tree root:

```
HYDRA_HOST=192.168.106.100:8030
HYDRA_CERT=/absolute/path/to/your/certificate/dir
```

The Makefile picks `.env.local` up automatically when present.
`.env.local` is in `.gitignore` and must never be committed.

**Tests must never target a live HydrAIDE instance.** Concurrent writes
from local code into a live engine risk double-send and queue corruption.
The `.env.local` mechanism is for unit instances only.

## CI

GitHub Actions runs the example test suite on every push via
[`.github/workflows/examples-test.yml`](../../.github/workflows/examples-test.yml):

1. `docker compose up -d` (in `docs/sdk/go/examples/`).
2. Wait for the `hydraide` service health check.
3. `go test -tags=integration -count=1 ./docs/sdk/go/examples/...`.
4. `docker compose down -v` on cleanup.

If you write your own HydrAIDE-backed service, mirror this workflow: a
compose-managed test instance, a build-tagged integration suite, and
cleanup that destroys whatever the tests created.

## Common gotchas

- **`CatalogShiftExpired` returning 0 entries on a freshly written
  Treasure with a past `ExpireAt`.** Almost always client/server clock
  skew, not a HydrAIDE bug — see
  [Clock skew and `ShiftExpired`](../troubleshooting/clock-skew-and-shift-expired.md)
  for the full debug recipe.

## Reference

- Recipe with the canonical setup: [`02-recipes/atomic-patch`](examples/02-recipes/atomic-patch/)
- Setup helper source: [`internal/setup`](examples/internal/setup/)
- CI workflow: [`examples-test.yml`](../../.github/workflows/examples-test.yml)
