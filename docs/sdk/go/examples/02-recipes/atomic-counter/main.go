// 02-recipes/atomic-counter — atomic int64 counters via IncrementInt64.
//
// The classical alternative is read-modify-write: GET the value, add one
// client-side, PUT it back. That pattern races under concurrent writers
// — two goroutines both read 5, both write 6, the second increment is
// lost.
//
// IncrementInt64 sends a single delta to the engine, which applies it
// under the per-key guard. Concurrent increments on the same key
// serialise correctly; concurrent increments on disjoint keys run in
// parallel.
package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/internal/setup"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r, cleanup := setup.MustClient(ctx)
	defer cleanup()

	if err := RunAtomicCounter(ctx, r); err != nil {
		log.Fatalf("atomic-counter failed: %v", err)
	}
	fmt.Println("done.")
}

// CounterSwamp is the namespace for this recipe.
func CounterSwamp() name.Name {
	return name.New().Sanctuary("examples").Realm("atomic-counter").Swamp("metrics")
}

// RunAtomicCounter spawns 100 goroutines that each call IncrementInt64
// once on the same key. The final value must be exactly 100.
func RunAtomicCounter(ctx context.Context, r repo.Repo) error {
	swamp := CounterSwamp()

	if err := setup.Pattern(ctx, r,
		name.New().Sanctuary("examples").Realm("atomic-counter").Swamp("*")); err != nil {
		return fmt.Errorf("register pattern: %w", err)
	}

	h := r.GetHydraidego()
	_ = h.Destroy(ctx, swamp)
	defer func() { _ = h.Destroy(ctx, swamp) }()

	const workers = 100
	const key = "page-views"

	// Each goroutine sends one increment and reports the post-increment
	// value back. The largest reported value is the final state, which
	// must equal the worker count if the engine serialised the writes.
	var wg sync.WaitGroup
	wg.Add(workers)
	values := make(chan int64, workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			v, _, err := h.IncrementInt64(ctx, swamp, key, 1, nil, nil, nil)
			if err != nil {
				errs <- err
				return
			}
			values <- v
		}()
	}
	wg.Wait()
	close(errs)
	close(values)

	for err := range errs {
		if err != nil {
			return fmt.Errorf("increment: %w", err)
		}
	}

	var maxValue int64
	count := 0
	for v := range values {
		count++
		if v > maxValue {
			maxValue = v
		}
	}
	fmt.Printf("final counter value after %d concurrent increments: %d\n", count, maxValue)
	if maxValue != int64(workers) {
		return fmt.Errorf("counter lost updates: want %d, got %d", workers, maxValue)
	}
	return nil
}
