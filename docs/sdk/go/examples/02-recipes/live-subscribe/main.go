// 02-recipes/live-subscribe — react to swamp changes in real time using
// Subscribe.
//
// The Subscribe stream emits an event for every Treasure insert, update,
// and delete. No Kafka, no NATS, no separate pub/sub — the engine itself
// is the broker.
//
// This recipe spins up a subscriber goroutine, then performs a small
// scripted sequence of writes (insert × 3, update, delete) so the
// subscription has something to print. It exits when every expected event
// has been observed.
package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/internal/setup"
	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r, cleanup := setup.MustClient(ctx)
	defer cleanup()

	if err := RunLiveSubscribe(ctx, r); err != nil {
		log.Fatalf("live-subscribe failed: %v", err)
	}
	fmt.Println("done.")
}

// Order is the model. Subscribe events arrive as *Order pointers.
type Order struct {
	ID     string  `hydraide:"key"`
	Detail *Detail `hydraide:"value"`
}

type Detail struct {
	Status string `msgpack:"status"`
	Cents  int64  `msgpack:"cents"`
}

// SubscribeSwamp is the namespace for this recipe.
func SubscribeSwamp() name.Name {
	return name.New().Sanctuary("examples").Realm("live-subscribe").Swamp("orders")
}

// RunLiveSubscribe is the test-friendly entry point.
//
// We expect to observe:
//   - 3 NEW events (one per insert)
//   - 1 MODIFIED event (the update)
//   - 1 DELETED event (the delete)
func RunLiveSubscribe(ctx context.Context, r repo.Repo) error {
	swamp := SubscribeSwamp()

	if err := setup.Pattern(ctx, r, name.New().Sanctuary("examples").Realm("live-subscribe").Swamp("*")); err != nil {
		return fmt.Errorf("register pattern: %w", err)
	}

	h := r.GetHydraidego()
	_ = h.Destroy(ctx, swamp) // clean re-runs

	const expectedEvents = 5
	var observed int64

	subCtx, subCancel := context.WithCancel(ctx)
	defer subCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := h.Subscribe(subCtx, swamp, false, Order{}, func(model any, eventStatus hydraidego.EventStatus, _ error) error {
			order, ok := model.(*Order)
			if !ok {
				return nil
			}
			detail := "deleted"
			if order.Detail != nil {
				detail = fmt.Sprintf("status=%s cents=%d", order.Detail.Status, order.Detail.Cents)
			}
			fmt.Printf("event: id=%s status=%s %s\n", order.ID, setup.EventStatusName(eventStatus), detail)

			// StatusNothingChanged events are emitted for the catch-up
			// phase when getExistingData=true. We pass false above, so we
			// only count NEW / MODIFIED / DELETED.
			if eventStatus == hydraidego.StatusNew ||
				eventStatus == hydraidego.StatusModified ||
				eventStatus == hydraidego.StatusDeleted {
				if atomic.AddInt64(&observed, 1) >= int64(expectedEvents) {
					subCancel()
				}
			}
			return nil
		})
		if err != nil && err != context.Canceled {
			log.Printf("subscribe stopped: %v", err)
		}
	}()

	// Let the stream attach before any writes.
	time.Sleep(300 * time.Millisecond)

	// Three inserts.
	for i := 1; i <= 3; i++ {
		o := &Order{
			ID:     fmt.Sprintf("order-%d", i),
			Detail: &Detail{Status: "pending", Cents: int64(1000 * i)},
		}
		if _, err := h.CatalogSave(ctx, swamp, o); err != nil {
			return fmt.Errorf("insert %s: %w", o.ID, err)
		}
		fmt.Printf("inserted %s\n", o.ID)
	}

	// One update.
	updated := &Order{
		ID:     "order-2",
		Detail: &Detail{Status: "shipped", Cents: 2000},
	}
	if _, err := h.CatalogSave(ctx, swamp, updated); err != nil {
		return fmt.Errorf("update: %w", err)
	}
	fmt.Println("updated order-2 (status pending → shipped)")

	// One delete.
	if err := h.CatalogDelete(ctx, swamp, "order-3"); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	fmt.Println("deleted order-3")

	// Wait for the subscriber to see all five events. A short hard
	// deadline guards against event loss.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		subCancel()
		return fmt.Errorf("only observed %d/%d events before deadline", atomic.LoadInt64(&observed), expectedEvents)
	}

	if err := h.Destroy(ctx, swamp); err != nil {
		return fmt.Errorf("destroy: %w", err)
	}
	fmt.Println("destroyed live-subscribe swamp")
	return nil
}
