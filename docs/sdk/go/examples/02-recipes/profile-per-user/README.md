# 02-recipes/profile-per-user

Swamp-per-user profile storage using the **Profile** record shape.

## Catalog vs Profile, briefly

| Use Catalog when | Use Profile when |
|---|---|
| You list, filter or paginate many records together | Each record is an independent unit |
| The records share a lifecycle | Each record evicts/locks/destroys independently |
| You query across the set | You always know the exact record you want |

Per-user profiles, per-tenant config, per-device state — Profile.
Order list, audit log, queue — Catalog.

## What this recipe does

1. Register the pattern `examples/profiles/*` (one wildcard, every user
   slots in).
2. Save Alice and Bob into **separate Swamps** (`examples/profiles/alice`
   and `examples/profiles/bob`).
3. Read Alice back.
4. Update Alice's score and save again — `ProfileSave` is idempotent and
   only the changed fields move on disk.
5. Destroy both Swamps.

## Run it

```bash
docker compose up -d
make recipe-profile-per-user
```

## Test it

```bash
make test-examples
```
