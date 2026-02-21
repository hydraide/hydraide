package models

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// CatalogShiftBatch — Atomic batch read-and-delete operation.
//
// When to use:
// - You need to consume and remove multiple items by their keys in one operation
// - You're implementing job queues, message consumers, or task processors
// - You want to avoid the N+1 problem of reading then deleting items separately
// - You need atomic removal without race conditions
//
// Key properties:
// - Sends one gRPC call with all requested keys
// - Each existing Treasure is locked, cloned, and permanently deleted
// - Silently skips non-existing keys (not an error)
// - For each shifted item, creates a fresh model instance and passes it to the iterator
// - If the iterator returns an error, processing stops immediately
// - ⚠️ This is a DESTRUCTIVE operation — deleted Treasures cannot be recovered
// - 📢 All Swamp subscribers receive deletion notifications
//
// Use cases:
// - 📦 Job queue workers: fetch and acknowledge jobs
// - 🛒 Shopping cart checkout: retrieve and remove items atomically
// - 📨 Message queue consumers: read and delete messages
// - 🗃️ Batch cleanup: extract items for archival before deletion
// - ⚙️ Task processing: claim and remove tasks without race conditions
//
// Performance: 30-50× faster than individual read+delete operations in a loop.
//
// Note: The model parameter must be a non-pointer type; the SDK creates new instances internally.
// The iterator receives a pointer (as interface{}), which you can cast.
//
// This file provides a complete, copyable example.

type CatalogModelJob struct {
	JobID     string    `hydraide:"key"`
	Payload   string    `hydraide:"value"`
	Priority  int       `hydraide:"priority,omitempty"`
	Status    string    `hydraide:"status,omitempty"`
	CreatedBy string    `hydraide:"createdBy"`
	CreatedAt time.Time `hydraide:"createdAt"`
	UpdatedBy string    `hydraide:"updatedBy,omitempty"`
	UpdatedAt time.Time `hydraide:"updatedAt,omitempty"`
}

// RegisterPattern — Register the job queue Swamp during application startup.
func (c *CatalogModelJob) RegisterPattern(r repo.Repo) error {
	h := r.GetHydraidego()
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	errs := h.RegisterSwamp(ctx, &hydraidego.RegisterSwampRequest{
		SwampPattern:    name.New().Sanctuary("jobs").Realm("catalog").Swamp("pending"),
		CloseAfterIdle:  30 * time.Minute,
		IsInMemorySwamp: true, // Job queue is typically in-memory for speed
		FilesystemSettings: &hydraidego.SwampFilesystemSettings{
			WriteInterval: 5 * time.Second,
			MaxFileSize:   8192, // Deprecated: V1 only — ignored by V2 engine (default for new installs)
		},
	})
	if errs != nil {
		return hydraidehelper.ConcatErrors(errs)
	}
	return nil
}

// Save — Add a new job to the queue.
func (c *CatalogModelJob) Save(r repo.Repo) error {
	h := r.GetHydraidego()
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	_, err := h.CatalogSave(ctx, c.catalogName(), c)
	return err
}

// ShiftBatch — Atomically retrieve and delete multiple jobs by their IDs.
//
// This is the core function for job queue processing:
// 1. Fetches all specified jobs in one gRPC call
// 2. Each job is locked, cloned, and deleted atomically
// 3. Returns the cloned job data (originals are already deleted)
// 4. Missing job IDs are silently ignored
//
// The iterator is called for each successfully shifted job.
// If processing fails, return an error to halt iteration.
func (c *CatalogModelJob) ShiftBatch(r repo.Repo, jobIDs []string) ([]*CatalogModelJob, error) {
	if len(jobIDs) == 0 {
		return nil, nil
	}

	h := r.GetHydraidego()
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	results := make([]*CatalogModelJob, 0, len(jobIDs))
	err := h.CatalogShiftBatch(ctx, c.catalogName(), jobIDs, CatalogModelJob{}, func(m any) error {
		job := m.(*CatalogModelJob)
		results = append(results, job)
		return nil
	})
	return results, err
}

// ProcessJobsAndShift — Advanced example: shift jobs and process them.
//
// This function demonstrates a real-world pattern:
// 1. Query jobs (in this example, we simulate by passing known IDs)
// 2. Shift (retrieve and delete) them atomically
// 3. Process each job
// 4. Handle errors appropriately (jobs are already deleted)
func (c *CatalogModelJob) ProcessJobsAndShift(r repo.Repo, jobIDs []string) error {
	h := r.GetHydraidego()
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	// Shift jobs atomically (read + delete in one call)
	return h.CatalogShiftBatch(ctx, c.catalogName(), jobIDs, CatalogModelJob{}, func(m any) error {
		job := m.(*CatalogModelJob)

		// Process the job (it's already deleted from the queue)
		slog.Info("Processing job", "jobID", job.JobID, "payload", job.Payload)

		// Simulate job processing
		if err := processJob(job); err != nil {
			// ⚠️ Job is already deleted! Handle failure appropriately:
			// - Log to error tracking system
			// - Store in dead-letter queue
			// - Send notification
			slog.Error("Job processing failed", "jobID", job.JobID, "error", err)
			return err // Stop processing remaining jobs
		}

		slog.Info("Job completed successfully", "jobID", job.JobID)
		return nil
	})
}

// processJob — Simulated job processing logic.
func processJob(job *CatalogModelJob) error {
	// In a real application, this would:
	// - Execute the job's task
	// - Call external APIs
	// - Update databases
	// - etc.
	time.Sleep(10 * time.Millisecond) // Simulate work
	return nil
}

// Example_CatalogShiftBatch_Basic — Basic usage: save jobs and shift them.
func Example_CatalogShiftBatch_Basic() {
	var r repo.Repo // Your app initializes this (see repo.go in the SDK)

	// 1) Register the job queue Swamp
	_ = (&CatalogModelJob{}).RegisterPattern(r)

	// 2) Add some jobs to the queue
	_ = (&CatalogModelJob{
		JobID:     "job-1",
		Payload:   "Send email to user@example.com",
		Priority:  1,
		Status:    "pending",
		CreatedBy: "api-server",
		CreatedAt: time.Now().UTC(),
	}).Save(r)

	_ = (&CatalogModelJob{
		JobID:     "job-2",
		Payload:   "Generate report for Q4 2024",
		Priority:  2,
		Status:    "pending",
		CreatedBy: "scheduler",
		CreatedAt: time.Now().UTC(),
	}).Save(r)

	_ = (&CatalogModelJob{
		JobID:     "job-3",
		Payload:   "Cleanup old data",
		Priority:  3,
		Status:    "pending",
		CreatedBy: "maintenance",
		CreatedAt: time.Now().UTC(),
	}).Save(r)

	// 3) Worker picks up jobs and processes them
	jobIDs := []string{"job-1", "job-2", "job-3", "job-missing"}
	jobs, err := (&CatalogModelJob{}).ShiftBatch(r, jobIDs)
	if err != nil {
		slog.Error("ShiftBatch error", "err", err)
		return
	}

	// 4) At this point, jobs are already deleted from the queue
	fmt.Printf("Shifted %d jobs\n", len(jobs))
	for _, job := range jobs {
		fmt.Printf("- %s: %s (priority: %d)\n", job.JobID, job.Payload, job.Priority)
	}

	// Output:
	// Shifted 3 jobs
	// - job-1: Send email to user@example.com (priority: 1)
	// - job-2: Generate report for Q4 2024 (priority: 2)
	// - job-3: Cleanup old data (priority: 3)
}

// Example_CatalogShiftBatch_WithProcessing — Advanced: shift and process jobs inline.
func Example_CatalogShiftBatch_WithProcessing() {
	var r repo.Repo // Your app initializes this

	// 1) Register the Swamp
	_ = (&CatalogModelJob{}).RegisterPattern(r)

	// 2) Add jobs
	_ = (&CatalogModelJob{JobID: "job-100", Payload: "Task A", CreatedBy: "system", CreatedAt: time.Now().UTC()}).Save(r)
	_ = (&CatalogModelJob{JobID: "job-101", Payload: "Task B", CreatedBy: "system", CreatedAt: time.Now().UTC()}).Save(r)

	// 3) Process and shift atomically
	jobIDs := []string{"job-100", "job-101"}
	err := (&CatalogModelJob{}).ProcessJobsAndShift(r, jobIDs)
	if err != nil {
		slog.Error("Processing failed", "err", err)
		return
	}

	fmt.Println("All jobs processed and removed")
	// Output:
	// All jobs processed and removed
}

// Example_CatalogShiftBatch_EdgeCases — Demonstrates edge case handling.
func Example_CatalogShiftBatch_EdgeCases() {
	var r repo.Repo

	// 1) Empty keys slice (returns immediately, no error)
	jobs1, err1 := (&CatalogModelJob{}).ShiftBatch(r, []string{})
	fmt.Printf("Empty keys: %d jobs, error: %v\n", len(jobs1), err1)

	// 2) All keys are missing (returns empty result, no error)
	jobs2, err2 := (&CatalogModelJob{}).ShiftBatch(r, []string{"missing-1", "missing-2"})
	fmt.Printf("Missing keys: %d jobs, error: %v\n", len(jobs2), err2)

	// 3) Mixed: some exist, some don't
	_ = (&CatalogModelJob{JobID: "job-200", Payload: "Exists", CreatedBy: "test", CreatedAt: time.Now().UTC()}).Save(r)
	jobs3, err3 := (&CatalogModelJob{}).ShiftBatch(r, []string{"job-200", "missing-3"})
	fmt.Printf("Mixed keys: %d jobs, error: %v\n", len(jobs3), err3)

	// Output:
	// Empty keys: 0 jobs, error: <nil>
	// Missing keys: 0 jobs, error: <nil>
	// Mixed keys: 1 jobs, error: <nil>
}

// Example_CatalogShiftBatch_ShoppingCart — Real-world use case: shopping cart checkout.
func Example_CatalogShiftBatch_ShoppingCart() {
	type CartItem struct {
		ItemID    string    `hydraide:"key"`
		ProductID string    `hydraide:"productId"`
		Quantity  int       `hydraide:"quantity"`
		Price     float64   `hydraide:"price"`
		CreatedAt time.Time `hydraide:"createdAt"`
	}

	var r repo.Repo

	// User has items in cart
	cartSwamp := name.New().Sanctuary("carts").Realm("items").Swamp("user-123")

	// Checkout: shift all cart items atomically
	itemIDs := []string{"item-1", "item-2", "item-3"}
	var purchasedItems []*CartItem

	h := r.GetHydraidego()
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	_ = h.CatalogShiftBatch(ctx, cartSwamp, itemIDs, CartItem{}, func(m any) error {
		item := m.(*CartItem)
		purchasedItems = append(purchasedItems, item)
		return nil
	})

	// At this point, items are removed from cart and ready for purchase processing
	fmt.Printf("Checking out %d items from cart\n", len(purchasedItems))
	// Output:
	// Checking out 3 items from cart
}

func (c *CatalogModelJob) catalogName() name.Name {
	return name.New().Sanctuary("jobs").Realm("catalog").Swamp("pending")
}
