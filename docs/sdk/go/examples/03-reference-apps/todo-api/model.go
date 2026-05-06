// todo-api: a textbook CRUD service backed by a single HydrAIDE Catalog
// Swamp.
//
// The HydrAIDE-specific moves stand out against a familiar todo shape:
//
//   - PATCH /todos/{id} flips fields with a single PatchTreasures op —
//     no read-modify-write round-trip.
//   - GET /todos?status=open|done evaluates the filter on the server,
//     not in client code.
//   - Concurrent PATCHes on the same todo serialise via the engine's
//     per-key FIFO lock; no application-level locking required.
package main

import (
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
)

// Todo is the Catalog model. The msgpack value (TodoBody) is the body
// of a single todo; field-level patches target paths inside it.
type Todo struct {
	ID   string    `hydraide:"key"`
	Body *TodoBody `hydraide:"value"`
}

// TodoBody is the patchable msgpack payload.
type TodoBody struct {
	Title     string    `msgpack:"title"`
	Done      bool      `msgpack:"done"`
	DueAt     time.Time `msgpack:"dueAt"`
	CreatedAt time.Time `msgpack:"createdAt"`
}

// Swamp is the Catalog this app stores all todos in.
func Swamp() name.Name {
	return name.New().Sanctuary("apps").Realm("todo-api").Swamp("todos")
}

// SwampPattern is what the app registers at startup. The wildcard is a
// no-op for this single-Swamp app but matches how multi-tenant apps
// would scale the same pattern.
func SwampPattern() name.Name {
	return name.New().Sanctuary("apps").Realm("todo-api").Swamp("*")
}
