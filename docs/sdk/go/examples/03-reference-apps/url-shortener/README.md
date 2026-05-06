# 03-reference-apps/url-shortener

Short-code → long-URL service. Demonstrates the read-heavy / write-cheap
pattern: a Catalog read on every redirect, an atomic `IncrementInt64`
on every click.

## HydrAIDE strengths used

- **`IncrementInt64`** for the click counter — never races, no
  read-modify-write, no client-side mutex.
- **O(1) deterministic addressing** — the short code maps to a Swamp
  without a metadata lookup; the redirect path is one network hop.
- **Two swamps, two lifecycles** — the link Catalog and the click
  counter live in independent swamps so each evicts on its own schedule.

## Run it

```bash
docker compose up -d
make app-url-shortener         # binds :8081
```

App startup prints:

```
url-shortener ready on http://localhost:8081
import postman_collection.json (File → Import) for a ready-to-run workspace
```

## Postman

**File → Import** [`postman_collection.json`](postman_collection.json).
Run **Create short link** first; the test script captures the new
`code` into `{{code}}` so the rest of the requests work without paste.

Try **Follow short link** several times in a row, then **Stats for
code** — the click count goes up by exactly one per redirect, even if
you mash the button.

## Curl

```bash
# create
curl -s -X POST http://localhost:8081/links \
  -H 'content-type: application/json' \
  -d '{"url":"https://hydraide.io"}' | jq

# follow (302)
curl -sI http://localhost:8081/<code>

# stats
curl -s http://localhost:8081/links/<code>/stats | jq

# delete
curl -s -X DELETE -i http://localhost:8081/links/<code>
```

## Endpoints

| Method | Path | Body | Returns |
|---|---|---|---|
| POST | `/links` | `{url}` | `201` + `{code,url,createdAt}` |
| GET | `/{code}` | — | `302` + `Location:` |
| GET | `/links/{code}/stats` | — | `200` + `{code,url,clicks}` |
| DELETE | `/links/{code}` | — | `204` |
