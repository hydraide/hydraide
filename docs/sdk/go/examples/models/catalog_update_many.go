package models

import (
	"log/slog"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// CatalogModelBulkTaskItem represents one task to be updated inside a HydrAIDE Swamp.
//
// This model demonstrates how to use CatalogUpdateMany() to safely perform batch updates
// on existing Treasures — without creating new records.
//
// ✅ Purpose:
// Use this model when you want to update the status of many tasks at once,
// while ensuring that:
//   - No new records are accidentally created
//   - Only existing keys are modified
//   - Each update provides per-task feedback via an iterator
//
// 🧠 Why use CatalogUpdateMany instead of SaveMany?
//
//   - SaveMany() behaves like upsert — it creates missing records
//   - CatalogUpdateMany() only modifies Treasures that already exist
//     → If a key is missing, it's skipped (not created)
//
// This is ideal for:
//   - Audit-safe mutation flows (e.g., background jobs, admin tools)
//   - Retry logic where only known task IDs should be touched
//   - Webhooks or batch jobs that mark items as "done", "failed", etc.
//
// 📥 Important:
// To safely use CatalogUpdateMany(), you should **load the target Treasures first**
// using CatalogReadMany() — so you're sure the keys exist before update.
//
// 📘 Example available:
// See `examples/models/catalog_read_many.go` for how to pre-load a batch of Treasures.
//
// 📦 Fields:
// - TaskID    → required key (unique task identifier)
// - Status    → value field (new task status)
// - UpdatedBy → optional metadata (who performed the update)
// - UpdatedAt → optional timestamp (when it was updated)
//
// 🔁 Example usage:
//
//	items := []*CatalogModelBulkTaskItem{
//	    {TaskID: "task-101", Status: "done", UpdatedBy: "worker-1", UpdatedAt: time.Now()},
//	    {TaskID: "task-102", Status: "done", UpdatedBy: "worker-1", UpdatedAt: time.Now()},
//	}
//
//	err := items[0].UpdateMany(repo, items)
//
//	if err != nil {
//	    log.Fatal("Batch update failed:", err)
//	}
//
// 💡 Tip:
// The iterator in CatalogUpdateMany() allows logging or metrics per task update:
//   - If key not found → log as warning
//   - If updated → log success or count metrics
//
// 🧠 Summary:
// This model ensures:
// - Safe batch mutation of task statuses
// - No risk of record creation
// - Clean audit trail via metadata
// - Fast and predictable behavior in high-volume systems
// - Designed to be used **after loading targets via CatalogReadMany()**
type CatalogModelBulkTaskItem struct {
	TaskID    string    `hydraide:"key"`       // Unique task identifier
	Status    string    `hydraide:"value"`     // New task status (e.g., "in-progress", "done")
	UpdatedBy string    `hydraide:"updatedBy"` // Who performed the update
	UpdatedAt time.Time `hydraide:"updatedAt"` // When the update occurred
}

// UpdateMany performs a batch update for existing tasks only.
//
// This method uses HydrAIDE’s CatalogUpdateMany() to update multiple
// task records in a single call. It ensures that:
//   - Only existing records are modified
//   - Missing tasks are skipped without failing the batch
//   - Per-task feedback is logged via an iterator callback
//
// 🧠 To understand how CatalogUpdateMany works internally,
// see the full SDK docs or implementation of:
//
//	func (h *hydraidego) CatalogUpdateMany(...)
//
// Recommended usage pattern:
// - Load records first via CatalogReadMany()
// - Apply in-place mutations (e.g., change Status)
// - Then pass the modified list into UpdateMany()
//
// ⚠️ Do NOT use this to insert new tasks. It will skip missing ones
// without creating them, preserving data safety in audit-sensitive flows.
func (c *CatalogModelBulkTaskItem) UpdateMany(r repo.Repo, tasks []*CatalogModelBulkTaskItem) error {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := r.GetHydraidego()

	// Define the Swamp location (tasks/catalog/main).
	swamp := c.getSwampName()

	// Convert our task slice to a generic model slice accepted by the SDK.
	models := make([]any, len(tasks))
	for i, t := range tasks {
		models[i] = t
	}

	// Perform the batch update using the HydrAIDE SDK.
	// The iterator function provides status feedback per key.
	err := h.CatalogUpdateMany(ctx, swamp, models, func(key string, status hydraidego.EventStatus) error {
		switch status {
		case hydraidego.StatusModified:
			slog.Info("Task status updated", "taskID", key)
		case hydraidego.StatusTreasureNotFound:
			slog.Warn("Task not found – skipped", "taskID", key)
		case hydraidego.StatusSwampNotFound:
			slog.Error("Swamp not found", "taskID", key, "swamp", swamp.Get())
		default:
			slog.Error("Unexpected status during batch update", "taskID", key, "status", status)
		}
		return nil
	})

	if err != nil {
		slog.Error("Batch update failed", "error", err)
	}

	return err
}

// RegisterSwamp ensures the Swamp used for batch task updates is properly configured.
// Call this once during app startup to enable memory + disk-backed persistence.
func (c *CatalogModelBulkTaskItem) RegisterSwamp(r repo.Repo) error {
	h := r.GetHydraidego()

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
	errs := h.RegisterSwamp(ctx, &hydraidego.RegisterSwampRequest{
		SwampPattern: c.getSwampName(),

		// Persist to disk in small chunks, ideal for frequent batch updates
		IsInMemorySwamp: false,
		CloseAfterIdle:  time.Hour,

		FilesystemSettings: &hydraidego.SwampFilesystemSettings{
			WriteInterval: time.Second * 10,
			MaxFileSize:   8192, // 8 KB chunks
		},
	})

	if errs != nil {
		return hydraidehelper.ConcatErrors(errs)
	}
	return nil
}

// getSwampName returns the canonical Swamp name used for task status records.
func (c *CatalogModelBulkTaskItem) getSwampName() name.Name {
	return name.New().Sanctuary("tasks").Realm("catalog").Swamp("main")
}
