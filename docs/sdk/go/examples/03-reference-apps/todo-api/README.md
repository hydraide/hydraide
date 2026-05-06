# 03-reference-apps/todo-api

Textbook CRUD service backed by a single HydrAIDE Catalog Swamp.

The shape every developer recognises — so the HydrAIDE-specific moves
stand out:

- **`PATCH /todos/{id}`** flips a single field via `PatchTreasures`. No
  read-modify-write round-trip. Concurrent patches on the same todo
  serialise via the engine's per-key FIFO lock.
- **`GET /todos?status=open|done`** evaluates the filter on the server
  with `FilterBytesFieldBool`. The HTTP handler never iterates a full
  list to filter client-side.
- **One Catalog Swamp**, indexed by creation time on demand. No
  persistent secondary index files; the index is built when first read
  and discarded when the swamp evicts.

## Run it

```bash
docker compose up -d           # if not already up
make app-todo-api              # binds :8080
```

App startup prints:

```
todo-api ready on http://localhost:8080
import postman_collection.json (File → Import) for a ready-to-run workspace
```

## Test it from Postman

1. **File → Import** in Postman.
2. Choose [`postman_collection.json`](postman_collection.json) (or
   [`openapi.yaml`](openapi.yaml) — Postman accepts both).
3. The collection variable `baseUrl` defaults to `http://localhost:8080`.
4. Run **Create todo** first; the test script captures the new id into
   `{{todoId}}` so the subsequent requests work without manual paste.

Order to follow on the first run: **Create → List (all) → Patch (mark
done) → List (status=done) → Delete**.

## Test it from curl

```bash
# create
curl -s -X POST http://localhost:8080/todos \
  -H 'content-type: application/json' \
  -d '{"title":"buy milk","dueAt":"2026-12-31T17:00:00Z"}'

# list (all)
curl -s http://localhost:8080/todos | jq

# list (only open)
curl -s 'http://localhost:8080/todos?status=open' | jq

# patch (mark done) — replace <id> with the id from create
curl -s -X PATCH http://localhost:8080/todos/<id> \
  -H 'content-type: application/json' \
  -d '{"done":true}'

# delete
curl -s -X DELETE http://localhost:8080/todos/<id> -i
```

## Endpoints

| Method | Path | Body | Returns |
|---|---|---|---|
| POST | `/todos` | `{title, dueAt?}` | `201` + Todo |
| GET | `/todos` | — | `200` + Todo[] |
| GET | `/todos?status=open|done` | — | `200` + filtered Todo[] |
| GET | `/todos/{id}` | — | `200` + Todo, `404` if missing |
| PATCH | `/todos/{id}` | any subset of `{title, done, dueAt}` | `200` + post-patch Todo |
| DELETE | `/todos/{id}` | — | `204` |

## Test it from Go

```bash
make test-examples
```

Covers create → read → patch → list (with filter) → delete + 404 after
delete, against the real HydrAIDE.
