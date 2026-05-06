# Swamp lifecycle — memory eviction and zero-garbage cleanup

A Swamp is not always loaded. The engine keeps state in memory only while it is being used, and it removes state from disk and memory the moment it is no longer needed. Two mechanisms make this happen, and they are configured separately:

1. **Idle eviction** — a Swamp loads on first access and is automatically evicted from memory after a configurable idle window. Per-Swamp configuration, set in code.
2. **Empty-Swamp removal** — when the last Treasure inside a Swamp is deleted, the `.hyd` file and the in-memory state are removed inline, in the same operation. No scheduler, no sweeper, no cron.

This page covers both.

---

## Idle eviction — memory budget per Swamp

Most databases either keep the working set in memory or serve it from disk. Developers rarely get to decide *which* parts live in RAM and for *how long* — that decision is hidden inside the engine.

HydrAIDE makes that decision a per-Swamp configuration. A Swamp loads into memory on first access and is automatically evicted after a configurable idle window. The choice — minutes, hours, days, or "never evict" — is yours, and it can be different for each Swamp pattern.

This matters when:

* A frequently accessed dataset (active sessions, hot tenants) should stay resident so every call doesn't hit the disk.
* A rarely-touched dataset (cold archives, per-user detail records) should not occupy RAM and should hydrate on demand from SSD.

### How it works

This is controlled via the `CloseAfterIdle` setting, which — like everything else in HydrAIDE — can be set directly in code when registering a Swamp.

When a Swamp is first accessed (hydrated), it is loaded from SSD into memory. If unused for the configured period, it is automatically removed from memory.

```go
CloseAfterIdle: time.Second * 21600, // Remove from memory after 6 hours idle
```

The Go garbage collector handles memory release once the Swamp's structures are no longer referenced — no extra background processes are required.

### Practical example — registering a Swamp with memory management

```go
type CatalogModelUserSaveExample struct {
    UserUUID  string    `hydraide:"key"`
    Payload   *Payload  `hydraide:"value"`
    CreatedBy string    `hydraide:"createdBy"`
    CreatedAt time.Time `hydraide:"createdAt"`
    UpdatedBy string    `hydraide:"updatedBy"`
    UpdatedAt time.Time `hydraide:"updatedAt"`
}

type Payload struct {
    LastLogin time.Time
    IsBanned  bool
}

func (c *CatalogModelUserSaveExample) RegisterPattern(r repo.Repo) error {
    ctx, cancel := hydraidehelper.CreateHydraContext()
    defer cancel()

    h := r.GetHydraidego()

    errs := h.RegisterSwamp(ctx, &hydraidego.RegisterSwampRequest{
        SwampPattern:    c.createCatalogName(), // e.g., users/catalog/all
        CloseAfterIdle:  6 * time.Hour,         // Keep in RAM for 6 hours of inactivity
        IsInMemorySwamp: false,                 // Persist to disk as well
        FilesystemSettings: &hydraidego.SwampFilesystemSettings{
            WriteInterval: 10 * time.Second, // How often to flush changes from memory to disk
        },
    })

    if errs != nil {
        return hydraidehelper.ConcatErrors(errs)
    }
    return nil
}
```

The example above shows the three knobs that matter: `CloseAfterIdle` controls the RAM lifetime, `IsInMemorySwamp` controls whether the Swamp is also persisted to disk, and `WriteInterval` controls how often dirty Treasures are flushed.

### Real-world example

With this approach, [Trendizz](https://trendizz.com) handles billions of word associations across millions of websites from a single server. The full story is on [dev.to](https://dev.to/hydraide/how-i-made-europe-searchable-from-a-single-server-the-story-of-hydraide-432h).

### What you get

* **Per-Swamp control over memory lifetime** — keep what's needed resident, evict what isn't.
* **A multi-TB dataset can live on a single server** with a fraction of it in RAM at any moment, as long as the working set fits.
* **Hydration is part of the read path** — no separate cache layer to maintain.

---

## Empty-Swamp removal — zero garbage

When the last Treasure is removed from a Swamp, the Swamp itself is removed — its file on disk and its in-memory state — as part of the same operation that deleted the last entry. There is no scheduled cleanup, no separate sweeper thread, no background process that revisits dead Swamps.

This is the part of the lifecycle that is *truly* free. (For partially-fragmented Swamps that still hold live entries, the storage engine runs file-level compaction automatically when fragmentation crosses a threshold — see [V2 storage engine](v2-storage-engine.md). Compaction is a separate mechanism from empty-Swamp removal.)

### How it works

When `Destroy` removes the last Treasure from a Swamp:

1. The `.hyd` file is removed from the SSD inline.
2. The Swamp's in-memory structures are released to the Go garbage collector.
3. No separate cleanup task is queued.

The same call that deletes the last entry also tears the Swamp down, in the same code path, before returning to the client.

### Configuration is preserved

The pattern that registered the Swamp (via `RegisterSwamp`) stays valid. If new data later arrives for a Swamp with that name, the engine creates a fresh `.hyd` file and rehydrates the in-memory state — the developer does not have to re-register or rebuild anything.

### What this saves you

- No `VACUUM`, `REPACK`, or compaction cron jobs for the case of empty Swamps.
- No background eviction sweeps to reclaim deleted RAM.
- No leftover files on disk for Swamps that became empty.

### What this does not save you

- **Backups, upgrades, monitoring, and capacity planning** are still operations work that the engine cannot do for you.
- **File-level compaction** for Swamps that still contain live entries does run — automatically on Swamp close above the configured fragmentation threshold, or on demand via `hydraidectl compact`. See [V2 storage engine](v2-storage-engine.md) for the mechanism.
- **TTL-based expiration** of individual Treasures is a separate feature (`expireAt` + `CatalogShiftExpired`), not part of "zero garbage". The application calls `ShiftExpired` on the cadence it wants.

This isn't garbage collection magic — it's the simpler property that an empty Swamp has nothing to retain, so the engine doesn't keep state for it.

---

## How the two mechanisms relate

| Trigger | What gets removed | When |
|---|---|---|
| Swamp goes idle for `CloseAfterIdle` | In-memory state only; the `.hyd` file stays on disk | Background, per-Swamp configuration |
| Last Treasure deleted from a Swamp | Both `.hyd` file on disk **and** in-memory state | Inline, on the delete operation |
| Fragmentation > 50% on Swamp close | The `.hyd` file is rewritten without dead entries; in-memory state unchanged | On Swamp close, or on demand via `hydraidectl compact` |

The first two are described above; the third is the V2 storage engine's compaction — see [V2 storage engine](v2-storage-engine.md).
