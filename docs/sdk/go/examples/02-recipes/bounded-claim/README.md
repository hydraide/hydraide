# 02-recipes/bounded-claim

A worker pool that claims tasks under a server-enforced concurrency cap.
No application-side counter, no distributed lock, no drift.

## How it works

- The work queue is **one Catalog swamp** (`examples/bounded-claim/crawl`).
- Each task is a Treasure carrying a `Status` field (`pending` → `claimed` → `done`)
  and an `ExpireAt` timestamp that doubles as the lease deadline.
- A worker claims tasks via `CatalogPatchExpiredWithResult` + `WithCap`:
  - `ExpireAt < now()` selects expired (≈ ready-to-claim) tasks.
  - `WithCap` constrains the post-op count of "claimed and still leased"
    records to `≤ MaxParallel`.
  - The patch flips `Status` to `claimed` and slides `ExpireAt` forward.
- Multiple workers can run concurrently. The Cap serialises them on the
  swamp's `capMu` long enough to keep the count exact — no race window.
- A crashing worker simply lets its lease expire; the next call sees the
  task as ready again and claims it.

## When to use this pattern

- Crawler claiming per-ASN with a hard concurrency cap (the canonical use case).
- Email send pool under a provider rate-limit.
- Inference workers competing for GPU slots.
- Any "N concurrent workers, fixed cap" claim pattern.

## What this replaces

| Old pattern | Why it's wrong |
|---|---|
| App-side counter incremented on claim, decremented on finalize | Every code path that forgets to decrement leaks +1. The drift is monotone — eventually the cap looks full while no records match. |
| `Lock`/`Unlock` around the claim path | Serialises the entire claim. Throughput ceiling = lock acquisition rate. |
| Soft cap with over-claim tolerance | Fine for advisory caps. Not fine when the cap protects an IP ban, GPU OOM, or third-party rate-limit penalty. |

## Run it

```bash
docker compose up -d        # if not already up
go run ./docs/sdk/go/examples/02-recipes/bounded-claim
```

The recipe seeds 30 tasks, spawns 8 workers, and runs them under
`MaxParallel = 5`. You will see the workers claim in batches, throughput
bounded by the cap, and no worker ever observes more than 5
simultaneously-claimed tasks.

## See also

- [`docs/features/cap-quota.md`](../../../../features/cap-quota.md) — concept docs for the Cap primitive.
- [`docs/features/catalog-shift.md`](../../../../features/catalog-shift.md) — for the claim-and-remove pattern variant.
- [`.claude/skills/hydraidego/SKILL.md`](../../../../../.claude/skills/hydraidego/SKILL.md) §14b — the full SDK guide for Cap.
