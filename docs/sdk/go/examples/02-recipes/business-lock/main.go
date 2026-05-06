// 02-recipes/business-lock — application-level distributed locks with TTL.
//
// HydrAIDE's `Lock` is a logical lock keyed by a name string. Two
// processes asking for the same lock will queue up; the first to acquire
// it holds the lock until it explicitly Unlock()s — or until the TTL
// expires, whichever comes first. The TTL guarantees a crashed holder
// cannot deadlock the system.
//
// This is **not** a per-key write lock — those are automatic and run
// inside the engine. Business locks are for higher-level invariants:
// "only one worker may process this user at a time", "only one process
// may run the daily rollup", etc.
package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/internal/setup"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r, cleanup := setup.MustClient(ctx)
	defer cleanup()

	if err := RunBusinessLock(ctx, r); err != nil {
		log.Fatalf("business-lock failed: %v", err)
	}
	fmt.Println("done.")
}

// RunBusinessLock starts two workers that race for the same lock. The
// engine queues them: worker A acquires, holds for 1s, releases; worker
// B then acquires. The two timestamp ranges must not overlap.
func RunBusinessLock(ctx context.Context, repository repo.Repo) error {
	r := repository
	const lockName = "examples/business-lock/daily-rollup"
	const ttl = 5 * time.Second
	const holdFor = time.Second

	h := r.GetHydraidego()

	type span struct {
		worker        string
		acquired, end time.Time
	}
	results := make(chan span, 2)

	var wg sync.WaitGroup
	wg.Add(2)
	for _, id := range []string{"A", "B"} {
		go func(worker string) {
			defer wg.Done()
			fmt.Printf("worker %s: requesting lock\n", worker)
			lockID, err := h.Lock(ctx, lockName, ttl)
			if err != nil {
				log.Printf("worker %s: lock failed: %v", worker, err)
				return
			}
			acquired := time.Now()
			fmt.Printf("worker %s: acquired lock at %s\n", worker, acquired.Format("15:04:05.000"))
			time.Sleep(holdFor)
			end := time.Now()
			if err := h.Unlock(ctx, lockName, lockID); err != nil {
				log.Printf("worker %s: unlock failed: %v", worker, err)
			}
			fmt.Printf("worker %s: released lock at %s\n", worker, end.Format("15:04:05.000"))
			results <- span{worker, acquired, end}
		}(id)
	}
	wg.Wait()
	close(results)

	spans := make([]span, 0, 2)
	for s := range results {
		spans = append(spans, s)
	}
	if len(spans) != 2 {
		return fmt.Errorf("expected 2 spans, got %d", len(spans))
	}
	a, b := spans[0], spans[1]
	if a.acquired.After(b.acquired) {
		a, b = b, a
	}
	gap := b.acquired.Sub(a.end)
	fmt.Printf("ordering: worker %s held %s, worker %s waited and acquired %v after release\n",
		a.worker, a.end.Sub(a.acquired).Round(time.Millisecond), b.worker, gap.Round(time.Millisecond))

	if b.acquired.Before(a.end) {
		return fmt.Errorf("lock failed: worker %s acquired before worker %s released", b.worker, a.worker)
	}
	return nil
}
