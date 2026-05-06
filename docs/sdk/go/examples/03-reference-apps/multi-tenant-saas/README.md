# 03-reference-apps/multi-tenant-saas

Per-tenant user store with business locks. The pattern HydrAIDE was
built around, presented as a generic B2B SaaS shape so the lesson
transfers to any domain.

## What's in the design

Each tenant gets its **own Swamp** at
`apps/multi-tenant-saas/{tenantID}`. Tenants are physically isolated:

- Their data lives in its own `.hyd` file on disk.
- Their lock domain is independent — concurrent writes to tenant
  `acme` and tenant `zenith` never contend with each other.
- Their eviction timer is independent — `acme` can be hot in memory
  while `zenith` evicts after idle.
- Deleting a tenant means removing one file from disk. No `DELETE FROM
  users WHERE tenant_id = ?` across millions of rows.

There is no `tenant_id` column anywhere because the tenant lives in
the Swamp address itself. Anything you might want to enforce with
row-level security is structurally impossible to violate.

## HydrAIDE strengths used

- **Swamp-per-tenant isolation** — see above.
- **`PatchTreasures`** for partial user updates (no read-modify-write).
- **Built-in business locks** for `POST .../{id}/claim` — a second
  concurrent claim returns `409 Conflict` because the lock is held.
  The TTL guarantees a crashed worker cannot deadlock the system.
- **Empty-Swamp removal from disk** when a tenant is deleted.

## Run it

```bash
docker compose up -d
make app-multi-tenant-saas     # binds :8082
```

App startup prints:

```
multi-tenant-saas ready on http://localhost:8082
import postman_collection.json (File → Import) for a ready-to-run workspace
```

## Postman

**File → Import** [`postman_collection.json`](postman_collection.json).
The collection variable `tenant` defaults to `acme`. Run **Create
user** once, then change `tenant` to `zenith` and run it again — the
two tenants' lists do not bleed into each other.

For the lock demo: run **Claim user** twice in quick succession (or
from two Postman windows). The second one returns `409 Conflict` until
the first holder's TTL expires.

## Curl

```bash
# create user in acme
curl -s -X POST http://localhost:8082/tenants/acme/users \
  -H 'content-type: application/json' \
  -d '{"email":"alice@acme.io","name":"Alice"}'

# list acme users
curl -s http://localhost:8082/tenants/acme/users | jq

# patch (deactivate)
curl -s -X PATCH http://localhost:8082/tenants/acme/users/<id> \
  -H 'content-type: application/json' \
  -d '{"isActive":false}'

# claim with TTL
curl -s -X POST http://localhost:8082/tenants/acme/users/<id>/claim \
  -H 'content-type: application/json' \
  -d '{"holdSeconds":30}'

# delete tenant entirely
curl -s -X DELETE -i http://localhost:8082/tenants/acme
```

## Endpoints

| Method | Path | Body | Returns |
|---|---|---|---|
| POST | `/tenants/{t}/users` | `{email, name}` | `201` + User |
| GET | `/tenants/{t}/users` | — | `200` + User[] |
| GET | `/tenants/{t}/users/{id}` | — | `200` + User |
| PATCH | `/tenants/{t}/users/{id}` | any subset of `{email, name, isActive}` | `200` + post-patch User |
| DELETE | `/tenants/{t}/users/{id}` | — | `204` |
| POST | `/tenants/{t}/users/{id}/claim` | `{holdSeconds?}` | `200` or `409` |
| DELETE | `/tenants/{t}` | — | `204` (entire Swamp file removed) |
