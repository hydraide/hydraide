// Package queue provides a generic task queue interface that enables scheduling,
// storing, and retrieving background or delayed jobs using HydrAIDE's typed Swamp engine.
//
// This package is ideal for tasks such as:
//   - Email delivery (delayed or retryable)
//   - Background scraping and data ingestion
//   - Asynchronous workflows
//   - Time-based triggers or reactive scheduling
//
// Each queue is implemented as a dedicated HydrAIDE Swamp, with fully typed payloads,
// expiration awareness, and FIFO retrieval of due tasks.
//
// Usage:
//
// 1. Add a task to the queue:
//
//	qs := queue.New(myRepo)
//
//	type EmailTask struct {
//	    Recipient string
//	    Subject   string
//	    Body      string
//	}
//
//	task := &EmailTask{
//	    Recipient: "user@example.com",
//	    Subject:   "Welcome to our platform!",
//	    Body:      "Thank you for registering.",
//	}
//
//	taskID, err := qs.Add("email_queue", task, time.Now().UTC().Add(5*time.Minute))
//	if err != nil {
//	    log.Fatalf("Failed to enqueue task: %v", err)
//	}
//	fmt.Println("Task ID:", taskID)
//
// 2. Retrieve due tasks:
//
//	loaded, err := qs.Get("email_queue", EmailTask{}, 5)
//	if err != nil {
//	    log.Fatalf("Failed to load tasks: %v", err)
//	}
//
//	for taskID, data := range loaded {
//	    email, ok := data.(*EmailTask)
//	    if !ok {
//	        log.Printf("Type mismatch for task %s", taskID)
//	        continue
//	    }
//	    fmt.Printf("Sending email to %s: %s\n", email.Recipient, email.Subject)
//	}
//
// Notes:
//
// - Each queue (Swamp) must store values of a single, consistent struct type.
// - The `expireAt` timestamp determines task visibility. Tasks are retrievable only after this time.
// - All times must be set in UTC. HydrAIDE sorts and compares expiration using UTC exclusively.
// - Values are stored as JSON inside the Swamp and restored via reflection-based unmarshaling.
//
// Internals:
//
//   - All tasks are stored as `ModelCatalogQueue` entries inside the HydrAIDE Swamp
//     `queueService/catalog/{queueName}`.
//   - Expired tasks are returned in order of expiration.
//   - The queue is fully reactive: entries can be subscribed to and streamed in real-time if needed.
//
// Limitations:
//
// - Does not support deduplication or retries out-of-the-box.
// - Task uniqueness is determined only by generated UUIDs.
//
// Recommended patterns:
//
// - Use short-lived queues (e.g. 5–15 min delays) for retry-based systems.
// - Use longer expiration (e.g. 24h+) for deferred reporting, analytics, or newsletter batching.
// - Run `Get()` from a scheduled service or use subscriptions if live stream processing is needed.
//
// This package is built to demonstrate how structured, delay-based workflows can be modeled
// inside HydrAIDE using native Swamp logic — without Redis, Kafka, or background daemons.
package queue

import (
	"encoding/json"
	"log/slog"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/panichandler"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// QueueService – HydrAIDE-based delayed task queue interface.
//
// This interface defines how structured background jobs can be enqueued, retrieved, monitored,
// or destroyed using a typed Swamp logic in HydrAIDE.
//
// Each queue is uniquely identified by a name and operates on one single struct type.
// Expiration is UTC-based and defines task readiness for processing.
//
// Namespace structure:
// - Sanctuary: "queueService" → fixed high-level container
// - Realm:     "catalog"      → internal grouping of queues
//
// All task data is stored in JSON format inside HydrAIDE Treasures, and deserialized using reflection.
//
// Queue constraints:
// - Only one struct type per queue.
// - All expiration times must be in UTC.
// - HydrAIDE returns tasks in ascending order of expiration time.

const (
	// HydrAIDE Swamp namespace constants.
	// These define the target Sanctuary and Realm for all queues.
	// Ensure no other model in your system uses the same identifiers to avoid collisions.
	queuesSanctuary    = "queueService"
	queuesRealmCatalog = "catalog"
)

type Service interface {
	// Add inserts a new task (payload) into the specified queue.
	//
	// Notes:
	// - The payload must be a struct (not a pointer).
	// - Each queue must contain only one specific struct type.
	// - The same queue cannot mix task types.
	// - Expiration defines when the task becomes *available* for retrieval.
	// - Expired tasks are returned in FIFO order by the HydrAIDE engine.
	// - All expiration times must be set in UTC!
	//
	// Example:
	//     expireAt := time.Now().UTC().Add(10 * time.Minute)
	//
	// Returns:
	// - A unique UUID string identifying the task
	// - Error if the task cannot be stored
	Add(queueName string, payload any, expireAt time.Time) (taskID string, err error)

	// Get retrieves a set of expired (due) tasks from the specified queue.
	//
	// Parameters:
	// - queueName: the queue to read from.
	// - taskStruct: a sample struct (not pointer!) that defines the task type to deserialize into.
	// - howMany: the number of tasks to retrieve.
	//     - If set to 0, all due tasks will be returned.
	//     - If N > 0, returns up to N due tasks, sorted by expiration.
	//
	// Returns:
	// - A map where:
	//     - key is the task UUID (string)
	//     - value is a POINTER to the restored struct (`*YourTaskType`)
	// - Error if something goes wrong
	//
	// Important:
	// The returned map contains values that are *typed pointers*, ready for use.
	Get(queueName string, taskStruct any, howMany int32) (tasks map[string]any, err error)

	// GetSize returns the number of tasks currently present in the given queue.
	// This includes all unexpired and expired tasks that haven’t been deleted.
	GetSize(queueName string) int

	// Destroy removes the specified queue from HydrAIDE.
	// This deletes all stored tasks and their underlying Swamp.
	Destroy(queueName string) error
}

// queueService is the internal implementation of the queue.Service interface.
// It wraps a HydrAIDE-compatible repo.Repo and provides typed access to Swamp-backed queues.
type queueService struct {
	repoInterface repo.Repo
}

// New creates a new queueService instance and registers the HydrAIDE pattern
// required to operate queues in the current server context.
//
// This function must be called once during application initialization.
// It ensures that the `ModelCatalogQueue` pattern is registered into HydrAIDE’s
// Swamp engine — so that Swamps like `queueService/catalog/email_queue` are
// correctly recognized, hydrated, and persisted.
//
// Parameters:
// - repoInterface: the injected HydrAIDE repository that connects to the target instance.
//
// Returns:
// - A Service implementation ready to use for queue operations (Add, Get, GetSize, Destroy).
func New(repoInterface repo.Repo) Service {
	qs := &queueService{
		repoInterface: repoInterface,
	}

	// Register the pattern on first instantiation.
	// This step is required so HydrAIDE knows how to interpret the queue Swamps.
	queue := &ModelCatalogQueue{}
	if err := queue.RegisterPattern(repoInterface); err != nil {
		slog.Error("cannot register pattern for queue",
			"error", err,
		)
	}

	return qs
}

// Add inserts a new task into the given queue.
//
// The task is serialized to JSON and stored in the HydrAIDE Swamp with a unique UUID key.
// Tasks are not retrievable until their expiration time (expireAt) is reached.
//
// Parameters:
// - queueName: the name of the queue (Swamp key suffix under "queueService/catalog").
// - payload: the task to enqueue. Must be a struct. Will be serialized to JSON.
// - expireAt: the UTC timestamp that defines when the task becomes visible for processing.
//
// Returns:
// - taskID: a UUID string used as the Swamp key for the task
// - err: any error encountered during marshaling or save
//
// Notes:
// - The expiration time **must** be in UTC.
// - HydrAIDE sorts tasks by ExpireAt and ensures FIFO retrieval of expired tasks.
// - The actual Swamp key is: "queueService/catalog/{queueName}/{taskUUID}".
//
// Example call:
//
//	qs.Add("email_queue", EmailTask{...}, time.Now().UTC().Add(5 * time.Minute))
func (q *queueService) Add(queueName string, payload any, expireAt time.Time) (taskID string, err error) {
	defer panichandler.PanicHandler()

	// Generate unique UUID for the task.
	taskUUID := uuid.New().String()

	// Serialize payload to JSON.
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		slog.Error("cannot marshal payload to JSON",
			"error", err,
		)
		return "", err
	}

	// Create the Swamp Treasure with metadata and raw task data.
	queue := &ModelCatalogQueue{
		TaskUUID: taskUUID,
		TaskData: payloadJSON,
		ExpireAt: expireAt, // Must be in UTC!
	}

	// Save the task to the HydrAIDE Swamp.
	err = queue.Save(q.repoInterface, queueName)
	if err != nil {
		return "", err
	}

	return taskUUID, nil
}

// Get retrieves expired (due) tasks from the specified queue.
//
// Tasks are returned in expiration order, and each is deserialized into a new instance
// of the given struct type (taskStruct). The return value is a map of task UUIDs to
// pointer instances of that type.
//
// Parameters:
// - queueName: the name of the queue (Swamp).
// - taskStruct: a sample (non-pointer) struct value used only for type inference.
// - howMany: the number of tasks to retrieve. If 0, returns all expired tasks.
//
// Returns:
// - tasks: map[string]any → keys are task UUIDs, values are pointers to deserialized structs.
// - err: error if loading or decoding fails.
//
// Notes:
// - taskStruct is used to determine the target type for JSON deserialization.
// - Returned values are POINTERS (e.g. `*EmailTask`).
// - The map is safe for direct iteration.
//
// Example:
//
//	loaded, _ := qs.Get("email_queue", EmailTask{}, 10)
//	for id, data := range loaded {
//	    fmt.Println("Send to:", task.Recipient)
//	}
func (q *queueService) Get(queueName string, taskStruct any, howMany int32) (tasks map[string]any, err error) {
	defer panichandler.PanicHandler()

	// Load expired tasks from the Swamp.
	queue := &ModelCatalogQueue{}
	loadedTasks, err := queue.LoadExpired(q.repoInterface, queueName, howMany)
	if err != nil || loadedTasks == nil {
		return nil, err
	}

	respMap := make(map[string]any)
	for _, task := range loadedTasks {
		// Create a new instance of taskStruct's type (as pointer).
		newTask := reflect.New(reflect.TypeOf(taskStruct)).Interface()

		// Deserialize JSON into the new instance.
		if convertErr := json.Unmarshal(task.TaskData, newTask); convertErr != nil {
			slog.Error("cannot unmarshal task data",
				"error", convertErr,
			)
			return nil, convertErr
		}

		respMap[task.TaskUUID] = newTask
	}

	return respMap, nil
}

// GetSize returns the number of tasks currently stored in the specified queue.
//
// This includes all tasks — expired and non-expired — that have not been deleted.
//
// Parameters:
// - queueName: the name of the queue (Swamp)
//
// Returns:
// - int: number of tasks (Treasures) currently in the Swamp
//
// Notes:
// - If the Swamp does not exist, the return value is 0.
// - This is a lightweight operation — uses internal HydrAIDE counter logic.
func (q *queueService) GetSize(queueName string) int {
	defer panichandler.PanicHandler()

	queue := &ModelCatalogQueue{}
	return queue.Count(q.repoInterface, queueName)
}

// Destroy deletes the entire queue (Swamp) identified by queueName.
//
// All stored tasks are removed from memory and disk.
//
// Parameters:
// - queueName: the name of the queue to destroy.
//
// Returns:
// - error: if the Swamp could not be deleted (e.g., not found or permission denied)
//
// Notes:
// - Once destroyed, the queue will be completely removed from HydrAIDE.
// - Any attempt to read from this queue after destruction will return an error or empty result.
func (q *queueService) Destroy(queueName string) error {
	defer panichandler.PanicHandler()

	queue := &ModelCatalogQueue{}
	return queue.DestroyQueue(q.repoInterface, queueName)
}
