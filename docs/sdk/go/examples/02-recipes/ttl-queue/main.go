// 02-recipes/ttl-queue — a delayed task queue built on a single HydrAIDE
// swamp. No Redis, no Kafka, no scheduler service.
//
// The queue is one Catalog swamp. Each task is a Treasure with an
// `ExpireAt` timestamp; the consumer atomically pops only the tasks whose
// expiration has passed (CatalogShiftExpired), guaranteeing exclusivity
// without a separate locking layer.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/hydraide/hydraide/docs/sdk/go/examples/internal/setup"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/utils/repo"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r, cleanup := setup.MustClient(ctx)
	defer cleanup()

	if err := RunTTLQueue(ctx, r); err != nil {
		log.Fatalf("ttl-queue failed: %v", err)
	}
	fmt.Println("done.")
}

// Task is the payload stored in each queue Treasure. Picking the smallest
// useful struct keeps the demo legible — real workloads embed whatever
// they need.
type Task struct {
	ID       string    `hydraide:"key"`
	Payload  *Payload  `hydraide:"value"`
	ExpireAt time.Time `hydraide:"expireAt"`
}

// Payload is the msgpack body of a task.
type Payload struct {
	Subject string `msgpack:"subject"`
	Body    string `msgpack:"body"`
}

// QueueSwamp is the namespace this recipe uses.
func QueueSwamp() name.Name {
	return name.New().Sanctuary("examples").Realm("ttl-queue").Swamp("emails")
}

// RunTTLQueue enqueues five tasks with staggered expirations and then
// polls the queue until all of them have been delivered. Idempotent: it
// destroys the swamp on entry so re-runs start clean.
func RunTTLQueue(ctx context.Context, r repo.Repo) error {
	swamp := QueueSwamp()

	if err := setup.Pattern(ctx, r, name.New().Sanctuary("examples").Realm("ttl-queue").Swamp("*")); err != nil {
		return fmt.Errorf("register pattern: %w", err)
	}

	h := r.GetHydraidego()
	_ = h.Destroy(ctx, swamp) // best effort: swamp may not exist on first run

	const taskCount = 5
	enqueueAt := time.Now().UTC()

	for i := 0; i < taskCount; i++ {
		task := &Task{
			ID: uuid.New().String(),
			Payload: &Payload{
				Subject: fmt.Sprintf("welcome email #%d", i),
				Body:    "thanks for signing up",
			},
			// Stagger so we can watch them pop in order.
			ExpireAt: enqueueAt.Add(time.Duration(i+1) * 500 * time.Millisecond),
		}
		if _, err := h.CatalogSave(ctx, swamp, task); err != nil {
			return fmt.Errorf("enqueue: %w", err)
		}
		fmt.Printf("enqueued task=%s expireAt=%s\n", task.ID[:8], task.ExpireAt.Format("15:04:05.000"))
	}

	// Poll until every task has been consumed. In a real worker this loop
	// runs forever inside a goroutine.
	delivered := 0
	deadline := time.After(taskCount*time.Second + 2*time.Second)

	for delivered < taskCount {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("only delivered %d/%d tasks before deadline", delivered, taskCount)
		default:
		}

		err := h.CatalogShiftExpired(ctx, swamp, int32(taskCount), Task{}, func(model any) error {
			task, ok := model.(*Task)
			if !ok {
				return fmt.Errorf("unexpected model type")
			}
			delivered++
			fmt.Printf("delivered task=%s subject=%q at=%s\n",
				task.ID[:8], task.Payload.Subject, time.Now().UTC().Format("15:04:05.000"))
			return nil
		})
		if err != nil {
			return fmt.Errorf("shift expired: %w", err)
		}

		// Tiny sleep so we don't burn CPU between polls. A real worker
		// would use a backoff or a Subscribe stream.
		time.Sleep(150 * time.Millisecond)
	}

	if err := h.Destroy(ctx, swamp); err != nil {
		return fmt.Errorf("destroy: %w", err)
	}
	fmt.Println("destroyed queue swamp")
	return nil
}
