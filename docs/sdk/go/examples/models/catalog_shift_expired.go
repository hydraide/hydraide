package models

import (
	"errors"
	"log/slog"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// ModelCatalogQueue
//
// 📚 TTL-Based Catalog Queue Example – HydrAIDE Model
//
// This model demonstrates a lightweight, time-sensitive task queue implementation
// using HydrAIDE's `CatalogShiftExpired()` primitive for safe, atomic task consumption.
//
// ✅ What is this model?
// - A queue entry (`ModelCatalogQueue`) stored as a Treasure in a Swamp
// - Each task has:
//   - A unique ID (`TaskUUID`)
//   - Raw payload (`TaskData` as []byte)
//   - A scheduled activation time (`ExpireAt`) — when the task becomes "processable"
//
// 🔁 Core behavior:
// - You save tasks into a Swamp (e.g. `queue/catalog/email`) with a **future** `ExpireAt` timestamp
// - Later, when tasks "expire" (i.e. `ExpireAt <= now`), they become retrievable
// - The `LoadExpired()` method uses `CatalogShiftExpired()` to:
//   - Atomically read AND remove expired tasks from the Swamp
//   - Prevent double-processing (tasks are deleted immediately)
//   - Return fully parsed task structs via the iterator
//
// ⚠️ VERY IMPORTANT:
//   - You **must** set the `ExpireAt` field to a future UTC time when inserting a task.
//     If the field is not set, or it's in the future, `LoadExpired()` will not return it.
//   - Example: `ExpireAt: time.Now().Add(30 * time.Second).UTC()`
//
// 🧪 Example usage:
//
//	// Insert a task (not shown here — you can use CatalogSave or SaveMany)
//	task := &ModelCatalogQueue{
//	    TaskUUID:  "task-123",
//	    TaskData:  []byte("do something later"),
//	    ExpireAt:  time.Now().Add(10 * time.Second).UTC(),
//	}
//
//	// Later, when polling expired tasks:
//
//	queue := &ModelCatalogQueue{}
//	tasks, err := queue.LoadExpired(repoInstance, "email", 10)
//	if err != nil {
//	    log.Fatalf("Failed to load expired tasks: %v", err)
//	}
//
//	for _, task := range tasks {
//	    fmt.Printf("Expired task: ID=%s, Payload=%s\n", task.TaskUUID, string(task.TaskData))
//	}
//
// 🛡️ Guarantees:
// - Each task is only returned once, even in concurrent environments
// - Non-expired or missing tasks are ignored silently
// - Deletion and unmarshaling are handled internally by HydrAIDE
//
// 📡 Event propagation:
//   - Every task removed via `CatalogShiftExpired()` triggers a `StatusDeleted` event.
//   - If you have a subscription on the Swamp (e.g. `queue/catalog/email`),
//     you will receive a real-time `StatusDeleted` notification with the deleted Treasure content.
//   - This allows reactive queue visualizations, audit logs, or downstream triggers
//     without polling or manual inspection.
//
// ➤ This makes `LoadExpired()` not just a queue consumer — but also a **subscription-aware**,
//
//	real-time event trigger point inside HydrAIDE.
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

// LoadExpired retrieves one or more expired tasks from the queue (Swamp).
// If no expired task exists, it returns an empty list without error.
// When a task is fetched from the Swamp, it is immediately deleted — ensuring exclusivity.
// This guarantees that no two processes can pick up the same task concurrently.
// If a process fails to process the task, it must explicitly re-save it into the queue.
// The operation is thread-safe due to HydrAIDE's per-Swamp write lock mechanism.
func (m *ModelCatalogQueue) LoadExpired(r repo.Repo, queueName string, howMany int32) (mcq []*ModelCatalogQueue, err error) {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := r.GetHydraidego()

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

// RegisterPattern registers the Swamp pattern for all queues in HydrAIDE.
// This function must be called once during startup, before any Save or Load is attempted.
func (m *ModelCatalogQueue) RegisterPattern(repo repo.Repo) error {

	// Access the HydrAIDE client
	h := repo.GetHydraidego()

	// Create a bounded context for this registration operation
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// RegisterSwamp always returns a []error.
	// Each error (if any) represents a failure during Swamp registration on a HydrAIDE server.
	//
	// ⚠️ Even when only a single Swamp pattern is registered, HydrAIDE may attempt to replicate or validate
	// the pattern across multiple server nodes (depending on your cluster).
	//
	// ➕ Return behavior:
	// - If all servers succeeded → returns nil
	// - If one or more servers failed → returns a non-nil []error
	//
	// 🧠 To convert this into a single `error`, you can use the helper:
	//     hydraidehelper.ConcatErrors(errorResponses)
	errorResponses := h.RegisterSwamp(ctx, &hydraidego.RegisterSwampRequest{
		// The pattern applies to all Swamps under the 'queueService/catalog/*' namespace
		// For example, it matches: queueService/catalog/messages, queueService/catalog/email, etc.
		SwampPattern: name.New().Sanctuary("queue").Realm("catalog").Swamp("*"),

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
		Sanctuary("queue").
		Realm("catalog").
		Swamp(queueName)
}
