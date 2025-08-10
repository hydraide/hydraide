package models

import (
	"log/slog"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// CatalogModelTaskStatus tracks the status of an existing task inside a HydrAIDE catalog.
//
// This model is designed for scenarios where a task (e.g. job, workflow item, ticket)
// already exists in the system and its status needs to be updated — but never created.
//
// ✅ Purpose:
// Use this model with CatalogUpdate() when you want to modify an existing task’s status
// (e.g. change it from "pending" to "in-progress" or "done"), but want to make sure
// that you’re not accidentally creating a new record.
//
// 🛠 Why CatalogUpdate instead of Save or Create?
//
// - CatalogCreate() inserts only new records (errors if already exists)
// - CatalogSave() does upsert: create or update
// - ✅ CatalogUpdate() updates only if the key and Swamp already exist
//
// This ensures safety and predictability when modifying production state,
// like workflows, audit trails, background jobs, etc.
//
// 🧠 Model structure:
//
// - TaskID (hydraide:"key") → uniquely identifies the task
// - Status (hydraide:"value") → current state of the task (e.g. "done", "failed")
// - UpdatedBy / UpdatedAt → optional metadata to track change history
//
// ⚠️ Important:
// Before calling CatalogUpdate(), you must make sure the record exists in the system.
// Otherwise the update will fail with ErrCodeNotFound.
//
// To do that, use:
//
// - CatalogRead() → if you want to load by ID
// - CatalogReadMany() → if you want to load a filtered list (e.g. recent tasks)
//
// 📚 See SDK examples:
// - /docs/sdk/go/examples/models/catalog_read.go
// - /docs/sdk/go/examples/models/catalog_read_many.go
//
// 🔧 Update flow example:
//
// Suppose an admin service receives a webhook to mark a task as "completed":
//
// ```go
//
//	task := &CatalogModelTaskStatus{
//			TaskID:    "task-abc-123",
//			Status:    "done",
//			UpdatedBy: "admin-service",
//			UpdatedAt: time.Now(),
//	}
//
// err := task.UpdateStatus(repo)
//
//	if err != nil {
//			if hydraidego.IsNotFound(err) {
//				log.Warn("Task not found – can't update a nonexistent task")
//			} else {
//				log.Error("Failed to update task status", err)
//			}
//	}
//
// ```
//
// 🧠 Summary:
// This model is perfect for mutation-type operations where you want:
// - Strict safety: don't create anything
// - Controlled flow: only update when known to exist
// - Metadata tracking for audit and observability
type CatalogModelTaskStatus struct {
	TaskID    string    `hydraide:"key"`       // Unique task identifier
	Status    string    `hydraide:"value"`     // New task status (e.g., "in-progress", "done")
	UpdatedBy string    `hydraide:"updatedBy"` // Who performed the update
	UpdatedAt time.Time `hydraide:"updatedAt"` // When the update occurred
}

// UpdateStatus attempts to update the task's status.
// It fails if the task does not already exist.
func (c *CatalogModelTaskStatus) UpdateStatus(r repo.Repo) error {
	// Create a context with timeout/cancellation support
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	// Access the HydrAIDE SDK client from the repository
	h := r.GetHydraidego()

	// Resolve the Swamp where the task is stored
	swamp := c.getSwampName()

	// Attempt to update the task status
	// CatalogUpdate requires that both the Swamp and the key already exist
	err := h.CatalogUpdate(ctx, swamp, c)

	if err != nil {
		// Case 1: The task key was not found → can't update what doesn't exist
		if hydraidego.IsNotFound(err) {
			slog.Warn("Task not found", "taskID", c.TaskID)
		} else if hydraidego.IsSwampNotFound(err) {
			// Case 2: The Swamp itself is missing → system misconfiguration
			slog.Error("Task Swamp does not exist", "swamp", swamp.Get())
		} else {
			// Case 3: Unexpected error (e.g. timeout, connection, internal issue)
			slog.Error("Failed to update task status", "taskID", c.TaskID, "error", err)
		}
		return err
	}

	// Success: the task was updated
	slog.Info("Task status updated", "taskID", c.TaskID, "status", c.Status)
	return nil
}

// RegisterPattern registers the Swamp used for storing task status records.
//
// 🧠 Why this is important:
//
// This function must be called once during application startup to tell HydrAIDE
// how to store and manage the Swamp where tasks are tracked.
//
// Since we store all task statuses in a single Swamp (`tasks/catalog/main`),
// there's no need for wildcard or dynamic Swamp naming.
//
// ✅ Swamp configuration:
//
//   - Name:         tasks/catalog/main
//   - Storage:      disk-backed (not in-memory only)
//   - Flush policy: write every 10 seconds in small chunks (8 KB max)
//   - Memory hint:  keep Swamp in memory for 1 hour of inactivity
//
// 💡 Ideal for task systems where hundreds/thousands of tasks are updated
// frequently, and read latency is important for dashboards or workers.
func (c *CatalogModelTaskStatus) RegisterPattern(repo repo.Repo) error {
	h := repo.GetHydraidego()

	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

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
		SwampPattern: c.getSwampName(),

		// Keep the Swamp hot for 1 hour after last access
		CloseAfterIdle: time.Hour,

		// Use persistent disk storage
		IsInMemorySwamp: false,

		// Small, fast chunks for frequent writes
		FilesystemSettings: &hydraidego.SwampFilesystemSettings{
			WriteInterval: time.Second * 10,
			MaxFileSize:   8192, // 8 KB
		},
	})

	if errorResponses != nil {
		return hydraidehelper.ConcatErrors(errorResponses)
	}
	return nil
}

// getSwampName returns the canonical Swamp name used for task status records.
//
// 📦 Swamp structure:
//
//	Sanctuary: "tasks"      → domain for task-related data
//	Realm:     "catalog"    → catalog-style Swamp
//	Swamp:     "main"       → single container for all task statuses
//
// This ensures consistent naming across Save, Read, Update, and Register calls.
//
// 💡 Tip: Use this whenever you need to interact with the task Swamp
// to avoid typos and ensure correct routing in distributed setups.
func (c *CatalogModelTaskStatus) getSwampName() name.Name {
	return name.New().
		Sanctuary("tasks").Realm("catalog").Swamp("main")
}
