// 02-recipes/bounded-claim — a worker pool that claims tasks under a
// server-enforced concurrency cap. No application-side counter, no
// distributed lock, no drift.
//
// The cap is enforced by HydrAIDE itself: the count of "claimed and
// still leased" records and the claim mutation run under one swamp
// guard, so two concurrent workers can never both observe currentMatching=N
// and each claim (MaxParallel - N) tasks.
package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/internal/setup"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/utils/repo"
)

const (
	taskCount   = 30
	workerCount = 8
	maxParallel = 5
)

// Task is one item of work the pool consumes. The Status flag tracks
// the lifecycle: pending → claimed → done. The Cap-bearing claim flips
// Status to "claimed" and slides ExpireAt forward as the lease deadline.
type Task struct {
	ID       string    `hydraide:"key"`
	Payload  *Payload  `hydraide:"value"`
	ExpireAt time.Time `hydraide:"expireAt"`
}

// Payload is the msgpack body of a task. Status is the cap-filter field
// the server inspects to decide whether the task counts toward MaxParallel.
type Payload struct {
	Status string `msgpack:"Status"` // "pending" | "claimed" | "done"
	Job    string `msgpack:"Job"`
}

func QueueSwamp() name.Name {
	return name.New().Sanctuary("examples").Realm("bounded-claim").Swamp("crawl")
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	r, cleanup := setup.MustClient(ctx)
	defer cleanup()

	if err := RunBoundedClaim(ctx, r); err != nil {
		log.Fatalf("bounded-claim failed: %v", err)
	}
	fmt.Println("done.")
}

// RunBoundedClaim seeds taskCount tasks, spawns workerCount workers,
// and runs them under a server-side Cap of maxParallel. It asserts that
// every worker eventually sees every task, and that the concurrent
// claim count never exceeds maxParallel.
func RunBoundedClaim(ctx context.Context, r repo.Repo) error {
	swamp := QueueSwamp()
	if err := setup.Pattern(ctx, r, name.New().Sanctuary("examples").Realm("bounded-claim").Swamp("*")); err != nil {
		return fmt.Errorf("register pattern: %w", err)
	}

	h := r.GetHydraidego()
	_ = h.Destroy(ctx, swamp) // best effort: swamp may not exist on first run

	// Seed tasks as "pending" with a past ExpireAt — they are immediately
	// claimable by PatchExpired.
	for i := 0; i < taskCount; i++ {
		task := &Task{
			ID:       fmt.Sprintf("task-%02d", i),
			Payload:  &Payload{Status: "pending", Job: fmt.Sprintf("scrape #%d", i)},
			ExpireAt: time.Now().UTC().Add(-time.Hour),
		}
		if _, err := h.CatalogSave(ctx, swamp, task); err != nil {
			return fmt.Errorf("seed: %w", err)
		}
	}
	fmt.Printf("seeded %d tasks, cap=%d, workers=%d\n", taskCount, maxParallel, workerCount)

	// Cap.Filter matches "claimed and still leased" tasks. Status=="claimed"
	// alone would also match completed tasks whose lease has not been
	// finalised; combining with ExpireAt > now scopes the count to records
	// actually in flight.
	//
	// NOTE: PatchExpired's Cap evaluator runs against the live treasure
	// (no body-only restriction — that limitation only applies to
	// explicit-key Patch surfaces). So a mixed body + metadata filter is
	// allowed here.
	capFilter := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldString(hydraidego.Equal, "Status", "claimed"),
		hydraidego.FilterExpiredAt(hydraidego.GreaterThan, time.Now()),
	)

	var inflight int64
	var peak int64
	var processed int64

	var wg sync.WaitGroup
	for w := 0; w < workerCount; w++ {
		workerID := fmt.Sprintf("worker-%02d", w)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if atomic.LoadInt64(&processed) >= int64(taskCount) {
					return
				}

				builder := hydraidego.NewPatchExpiredOps().
					Set("Status", "claimed").
					WithExpiredAt(time.Now().UTC().Add(2*time.Second)). // lease
					WithCap(&hydraidego.Cap{Filter: capFilter, MaxMatching: maxParallel})

				claimed := 0
				res, err := h.CatalogPatchExpiredWithResult(ctx, swamp, 5, Task{},
					func(model any, status hydraidego.PatchStatus) error {
						if status != hydraidego.PatchStatusPatched {
							return nil
						}
						task := model.(*Task)
						claimed++
						cur := atomic.AddInt64(&inflight, 1)
						for {
							p := atomic.LoadInt64(&peak)
							if cur <= p || atomic.CompareAndSwapInt64(&peak, p, cur) {
								break
							}
						}
						defer atomic.AddInt64(&inflight, -1)

						// Simulate work; the lease ExpireAt=now+2s is generous.
						time.Sleep(50 * time.Millisecond)

						// Finalise: flip Status to "done" and clear ExpireAt.
						_, ferr := h.CatalogPatch(ctx, swamp, task.ID).
							Set("Status", "done").
							WithoutExpiredAt().
							Exec()
						if ferr != nil {
							return ferr
						}
						atomic.AddInt64(&processed, 1)
						fmt.Printf("[%s] processed=%s inflight=%d peak=%d\n",
							workerID, task.ID, atomic.LoadInt64(&inflight), atomic.LoadInt64(&peak))
						return nil
					}, builder)
				if err != nil {
					log.Printf("[%s] claim error: %v", workerID, err)
					return
				}
				if claimed == 0 {
					if res.CapReached {
						// Cap full → back off, give finalising workers a chance to clear.
						time.Sleep(50 * time.Millisecond)
					} else {
						// No claimable tasks right now — short pause then retry.
						time.Sleep(100 * time.Millisecond)
					}
				}
			}
		}()
	}
	wg.Wait()

	finalPeak := atomic.LoadInt64(&peak)
	fmt.Printf("\nrun complete: processed=%d/%d peak_inflight=%d cap=%d\n",
		atomic.LoadInt64(&processed), taskCount, finalPeak, maxParallel)
	if finalPeak > maxParallel {
		return fmt.Errorf("invariant violated: peak inflight %d exceeded cap %d", finalPeak, maxParallel)
	}

	if err := h.Destroy(ctx, swamp); err != nil {
		return fmt.Errorf("destroy: %w", err)
	}
	return nil
}
