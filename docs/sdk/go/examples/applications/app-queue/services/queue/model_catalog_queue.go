// Package queue
//
// 📚 Catalog Queue Example – Real-Life HydrAIDE Model
//
// This file demonstrates a real-world Catalog model implementation using HydrAIDE.
// It shows how to build a lightweight, persistent, and time-aware task queue backed by the HydrAIDE database.
//
// ✅ What this model is:
// - A catalog-based queue called `ModelCatalogQueue`
// - Each item (task) has a unique key, raw payload (as []byte), and a UTC-based expiration time
// - The queue supports TTL-based task activation and ensures single-processing guarantees
//
// 🧩 Why Catalog?
// - Catalogs in HydrAIDE are ideal for scenarios where:
//   - You need heterogeneous records with simple key/value mappings
//   - You want easy upsert/save semantics
//   - You don’t need full-text search or indexing across all fields
//
// 🔁 Key operations supported:
// - Save(): Push a task into the queue with an optional future execution time (via `ExpireAt`)
// - LoadExpired(): Atomically pop expired tasks from the queue (thread-safe, one consumer at a time)
// - Count(): Get number of tasks in the queue
// - DestroyQueue(): Completely delete a queue (e.g. after test)
// - RegisterPattern(): Register the queue's structure and storage configuration in HydrAIDE
//
// 🛠 Storage strategy:
// - Persistent on disk (not in-memory)
// - Files split into 8KB chunks for efficient I/O
// - Write buffering enabled (1s interval) for performance
// - Keeps queues open in memory for 6 hours (fast access)
//
// ⚠️ Best Practices:
// - Always call RegisterPattern() (ONLY ONCE) before using Save/Load
// - Use CatalogShiftExpired to ensure safe TTL-based consumption
// - Store task payloads as GOB-encoded structs or JSON-encoded []byte if flexible schema is needed
//
// 🧪 Use case example:
// This model is used in testing to simulate a task queue. Tasks are inserted with future expiration timestamps,
// and processed by a worker that polls expired items. Ideal for:
// - Delayed jobs
// - Scheduled tasks
// - Event queueing across distributed systems
package queue

import (
	"errors"
	"log/slog"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

type ModelCatalogQueue struct {

	// TaskUUID A unique task identifier within the queue.
	// Can be a domain-specific key, a UUID, or any other globally unique identifier.
	TaskUUID string `hydraide:"key"`

	// TaskData The payload of the queued task.
	// In this example, it's stored as a raw byte slice, allowing you to encode any structure you want
	// (e.g. GOB, JSON, Protobuf, etc.) before saving it.
	TaskData []byte `hydraide:"value"`

	// ExpireAt The logical expiration time after which the task becomes active and eligible for processing.
	// Before this timestamp, the task will not be returned by the loader (e.g. LoadExpired).
	ExpireAt time.Time `hydraide:"expireAt"`
}

// Save inserts the task into the queue.
//
// This method saves the current task (`ModelCatalogQueue`) into the appropriate Swamp within HydrAIDE.
// The queue is modeled as a Catalog, where each task is stored as a key-value pair with an optional expiration time.
// If the Swamp for the given queue name does not exist yet, it will be created automatically.
//
// ✅ Behavior:
// - The task is saved under a unique `TaskUUID` key
// - The task payload (`TaskData`) is stored as-is (typically encoded as []byte)
// - The `ExpireAt` timestamp defines when the task becomes eligible for consumption
//
// ⚠️ Notes:
// - Uses a default context with a 5-second timeout (via `CreateHydraContext()`)
// - Must call `RegisterPattern()` once before using this method in a new environment
//
// Example usage:
//
//	task := &ModelCatalogQueue{
//	    TaskUUID:  "job-1234",
//	    TaskData:  encodedPayload,
//	    ExpireAt:  time.Now().UTC().Add(1 * time.Minute),
//	}
//	err := task.Save(repo, "emailQueue")
func (m *ModelCatalogQueue) Save(repo repo.Repo, queueName string) (err error) {
	// Set a scoped Hydra context with timeout (safe for RPC operations)
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Access the HydrAIDE SDK instance from the repository
	h := repo.GetHydraidego()

	// Construct the fully qualified Swamp name based on queue identifier
	modelCatalogName := m.createModelCatalogQueueSwampName(queueName)

	// Save the full typed struct into HydrAIDE under the resolved Swamp name
	_, err = h.CatalogSave(ctx, modelCatalogName, m)
	return err
}

// LoadExpired retrieves one or more expired tasks from the queue (Swamp).
// If no expired task exists, it returns an empty list without error.
// When a task is fetched from the Swamp, it is immediately deleted — ensuring exclusivity.
// This guarantees that no two processes can pick up the same task concurrently.
// If a process fails to process the task, it must explicitly re-save it into the queue.
// The operation is thread-safe due to HydrAIDE's per-Swamp write lock mechanism.
func (m *ModelCatalogQueue) LoadExpired(repo repo.Repo, queueName string, howMany int32) (mcq []*ModelCatalogQueue, err error) {

	// Create a bounded context for the HydrAIDE operation (safe timeout for gRPC)
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Get the HydrAIDE client instance
	h := repo.GetHydraidego()

	// Construct the Swamp name used for storing queue tasks
	modelCatalogName := m.createModelCatalogQueueSwampName(queueName)

	// Initialize the return slice to hold expired tasks
	mcq = make([]*ModelCatalogQueue, 0)

	// Use HydrAIDE's CatalogShiftExpired, which atomically reads + deletes expired Treasures.
	// This operation is thread-safe and uses FIFO ordering for expired entries.
	//
	// Important:
	// The third parameter (e.g., ModelCatalogQueue{}) MUST be a non-pointer instance.
	// It's only used to determine the model type for decoding internally,
	// so passing a pointer (e.g., &ModelCatalogQueue{}) would cause incorrect type inference
	// and may break unmarshal logic. Always pass a value, not a pointer.
	err = h.CatalogShiftExpired(ctx, modelCatalogName, howMany, ModelCatalogQueue{}, func(model any) error {

		// Convert the generic returned model into our typed ModelCatalogQueue
		queueTask, ok := model.(*ModelCatalogQueue)
		if !ok {
			slog.Error("invalid model type",
				"queueName", queueName,
			)
			return errors.New("wrong model type")
		}

		// Append the expired task to the result list
		mcq = append(mcq, queueTask)
		return nil
	})

	return mcq, err
}

// Count is a lightweight helper method that returns the current number of tasks in the specified queue (Swamp).
// It’s typically used for diagnostics, monitoring, or lightweight metrics — and executes in constant time
// thanks to HydrAIDE's internal memory indexing.
func (m *ModelCatalogQueue) Count(repo repo.Repo, queueName string) int {

	// Create a bounded Hydra context for safe gRPC execution
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE client instance
	h := repo.GetHydraidego()

	// Count the number of entries in the specified Swamp
	elements, err := h.Count(ctx, m.createModelCatalogQueueSwampName(queueName))
	if err != nil {
		return 0
	}
	return int(elements)
}

// DestroyQueue completely removes a queue (Swamp) from HydrAIDE, including all its data.
// Primarily intended for cleanup in testing environments or temporary systems.
func (m *ModelCatalogQueue) DestroyQueue(repo repo.Repo, queueName string) error {
	// Access the HydrAIDE SDK instance
	h := repo.GetHydraidego()

	// Create a bounded Hydra context with a safe timeout
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Destroy the entire Swamp — this deletes all Treasures and removes the folder from disk
	// The Swamp will also be unloaded from memory immediately
	if err := h.Destroy(ctx, m.createModelCatalogQueueSwampName(queueName)); err != nil {
		return err
	}
	return nil
}

// RegisterPattern registers the Swamp pattern for all queues in HydrAIDE.
// This function must be called once during startup, before any Save or Load is attempted.
func (m *ModelCatalogQueue) RegisterPattern(repo repo.Repo) error {

	// Access the HydrAIDE client
	h := repo.GetHydraidego()

	// Create a bounded context for this registration operation
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Register the Swamp pattern and its configuration
	errorResponses := h.RegisterSwamp(ctx, &hydraidego.RegisterSwampRequest{
		// The pattern applies to all Swamps under the 'queueService/catalog/*' namespace
		// For example, it matches: queueService/catalog/messages, queueService/catalog/email, etc.
		SwampPattern: name.New().Sanctuary(queuesSanctuary).Realm(queuesRealmCatalog).Swamp("*"),

		// Keep the Swamp open in memory for 6 hours after last access
		// This avoids repeated hydration for frequently accessed queues
		CloseAfterIdle: time.Second * time.Duration(21600), // 6 hours

		// This is not an ephemeral in-memory Swamp — we persist it to disk
		IsInMemorySwamp: false,

		// Filesystem configuration for how data is written to disk
		FilesystemSettings: &hydraidego.SwampFilesystemSettings{
			// Data is written to disk in 1-second intervals after modification
			// Good balance between performance and write frequency for high-throughput queues
			// Can be lowered for durability or increased to reduce I/O
			WriteInterval: time.Second * 1,

			// Max file size for binary chunks — small size minimizes SSD wear
			// 8KB ensures fast, compressible, delta-efficient chunking
			MaxFileSize: 8192, // 8 KB
		},
	})

	// If there were any validation or transport-level errors, concatenate and return them
	if errorResponses != nil {
		return hydraidehelper.ConcatErrors(errorResponses)
	}

	return nil
}

// createModelCatalogQueueSwampName constructs the fully-qualified Swamp name
// for a specific queue under the catalog namespace in HydrAIDE.
func (m *ModelCatalogQueue) createModelCatalogQueueSwampName(queueName string) name.Name {
	return name.New().
		Sanctuary(queuesSanctuary).
		Realm(queuesRealmCatalog).
		Swamp(queueName)
}
