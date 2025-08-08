package appserver

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/applications/app-queue/services/queue"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// AppServer defines the core interface for starting and stopping the demo application.
// It acts as an entry point for wiring services to the HydrAIDE SDK.
type AppServer interface {
	Start()
	Stop()
}

// appServer is a simple demonstration wrapper for initializing services using HydrAIDE.
// It is intentionally minimal and not intended as a full web framework or REST API server.
//
// The goal is to show how domain services (e.g. queue logic) can interact with the HydrAIDE data engine
// via a clean separation of concerns (repo → service → app).
type appServer struct {
	repoInterface repo.Repo     // provides access to HydrAIDE's SDK
	queueService  queue.Service // example domain logic that uses HydrAIDE for working with time-based queues
}

// New creates a new AppServer instance.
//
// This constructor receives a `repoInterface`, which encapsulates all HydrAIDE SDK operations.
// The returned AppServer can be used to start business services that operate on HydrAIDE Swamps and Treasures.
//
// 💡 This project is not meant to demonstrate a full-featured HTTP API server.
// It is intentionally focused on showcasing how to structure and use HydrAIDE-backed models and services
// in a clean, testable way within a real Go application.
func New(repoInterface repo.Repo) AppServer {
	return &appServer{
		repoInterface: repoInterface,
	}
}

// Start initializes all internal services.
//
// This is where you would typically wire your domain logic.
// In this demo, we only initialize the queueService, which is ready to be called via CLI, tests,
// or wrapped later into REST/WebSocket endpoints if needed.
//
// You already have full access to the HydrAIDE SDK via the `repoInterface` inside this layer.
func (a *appServer) Start() {

	// Initialize the queue service using the HydrAIDE-backed repo
	a.queueService = queue.New(a.repoInterface)

	queueName := "myTestQueue"

	// Since this is a demo/test app, we may restart it multiple times.
	// To avoid ID collisions or residual data, we destroy the queue on each startup.
	if err := a.queueService.Destroy(queueName); err != nil {
		slog.Error("Failed to destroy queue",
			"queueName", queueName,
			"error", err,
		)
		return
	}

	// Define the data structure to be stored in the queue.
	// This struct will be serialized and persisted in HydrAIDE.
	type Task struct {
		ID      string
		Message string
	}

	// Start a background goroutine that continuously polls the queue
	// and processes tasks as soon as they expire (i.e., become available).
	go func() {
		for {

			// Get up to 1 expired task from the queue.
			// Internally this uses HydrAIDE's TTL mechanism and CatalogShiftExpired().
			task, err := a.queueService.Get(queueName, Task{}, 1)
			if err != nil {
				slog.Error("waiting for new task",
					"queueName", queueName,
				)
				time.Sleep(1 * time.Second) // wait before retrying
				continue
			}

			// Iterate through the expired tasks and simulate processing them.
			for _, t := range task {

				// Type assertion from `any` to *Task
				receivedTask := t.(*Task)

				slog.Info("expired task received",
					"queueName", queueName,
					"taskID", receivedTask.ID,
					"taskMessage", receivedTask.Message,
				)

				// Simulate task processing logic (e.g. handling job, calling API, etc.)
				time.Sleep(1 * time.Second)
			}
		}
	}()

	// Create and enqueue 10 tasks into the queue.
	// Each task is given an `ExpireAt` of +3 seconds, meaning:
	// they become available for processing ~3 seconds after insertion.
	for i := 0; i < 10; i++ {

		task := &Task{
			ID:      fmt.Sprintf("task-%d", i),
			Message: fmt.Sprintf("message-%d", i),
		}

		taskID, err := a.queueService.Add(queueName, task, time.Now().Add(3*time.Second))

		if err != nil {
			slog.Error("Failed to add task to queue",
				"queueName", queueName,
				"taskID", taskID,
				"error", err,
			)
			continue
		}

		slog.Info("task added to queue successfully",
			"queueName", queueName,
			"taskID", taskID,
			"taskMessage", task.Message,
		)
	}
}

// Stop performs any needed graceful shutdown logic.
//
// In this demo application, we don't maintain persistent listeners or open connections,
// but in a real-world scenario, you'd close database handles, stop background workers, etc.
func (a *appServer) Stop() {
	// graceful shutdown logic for the application would go here
}
