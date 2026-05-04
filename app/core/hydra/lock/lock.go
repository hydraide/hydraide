package lock

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hydraide/hydraide/app/panichandler"
)

type Lock interface {
	// Lock queues requests based on a unique key. This is a blocking method and only returns
	// once the calling goroutine receives permission — meaning it is next in line.
	//
	// The ttl parameter defines how long the lock should remain valid.
	// The ttl is always required and must never be zero or omitted.
	//
	// If the ttl expires before the lock is released, the function must return with an error,
	// the lock must be freed, and the next caller in the queue should proceed.
	//
	// If the lock cannot be acquired, the function returns an error.
	//
	// If the lock is successfully acquired, the caller receives a unique lockID,
	// which must later be used in the Unlock method to release the lock.
	//
	// However, if the ttl expires, the lock must still be cleaned up by its lockID
	// to avoid deadlocks.
	Lock(ctx context.Context, key string, ttl time.Duration) (lockID string, err error)
	// Unlock releases a lock that was previously acquired via the Lock method.
	// The lock is released based on the provided lockID.
	//
	// If the given lockID does not exist, the function immediately returns an error.
	Unlock(key string, lockID string) error
}

type lock struct {
	// queues stores per-key FIFO queues of waiting callers.
	queues sync.Map // map[string]*queue
}

func New() Lock {
	return &lock{}
}

// caller represents a single waiter in the queue.
//
// ready is closed by Unlock (or auto-TTL) of the previous caller when this
// caller becomes the head of the queue. The waiting goroutine selects on
// this channel and the caller's context — no busy-wait, no CPU spin.
//
// done is closed by remove() once the caller leaves the queue (whether via
// Unlock, TTL expiration, or context cancellation). The auto-unlock watchdog
// selects on done so it can terminate immediately on Unlock instead of
// living for the full TTL.
type caller struct {
	id    string
	ready chan struct{}
	done  chan struct{}
}

type queue struct {
	mu      sync.Mutex
	callers []*caller
}

func newQueue() *queue {
	return &queue{}
}

// enqueue appends a new caller. If it lands at the head (queue was empty),
// its ready channel is pre-closed so it can proceed immediately.
func (q *queue) enqueue(c *caller) {
	q.mu.Lock()
	defer q.mu.Unlock()
	wasEmpty := len(q.callers) == 0
	q.callers = append(q.callers, c)
	if wasEmpty {
		close(c.ready)
	}
}

// remove deletes the caller with the given id from the queue. If the removed
// caller was at the head, the next caller's ready channel is closed so it can
// proceed. Returns true if the caller was found.
func (q *queue) remove(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, c := range q.callers {
		if c.id != id {
			continue
		}
		wasHead := i == 0
		q.callers = append(q.callers[:i], q.callers[i+1:]...)
		// Signal the leaving caller's watchdog so it can terminate.
		// Each caller has a fresh done channel and remove() only matches
		// once per id, so this close is safe.
		close(c.done)
		if wasHead && len(q.callers) > 0 {
			// Wake the next waiter.
			close(q.callers[0].ready)
		}
		return true
	}
	return false
}

func (l *lock) getQueue(key string) *queue {
	if v, ok := l.queues.Load(key); ok {
		return v.(*queue)
	}
	actual, _ := l.queues.LoadOrStore(key, newQueue())
	return actual.(*queue)
}

func (l *lock) Lock(ctx context.Context, key string, ttl time.Duration) (lockID string, err error) {
	lockID = uuid.NewString()

	c := &caller{
		id:    lockID,
		ready: make(chan struct{}),
		done:  make(chan struct{}),
	}

	q := l.getQueue(key)
	q.enqueue(c)

	// Wait until either we become the head of the queue (ready closed),
	// or the caller's context is done.
	select {
	case <-c.ready:
		// We hold the lock now. Start the auto-release watchdog: if the caller
		// forgets to Unlock or crashes, the TTL will release the lock and
		// wake the next waiter. The watchdog uses a fresh background context
		// so the caller's ctx-cancellation does not prematurely release a
		// successfully-acquired lock.
		panichandler.SafeGo("auto-unlock", func() {
			t := time.NewTimer(ttl)
			defer t.Stop()
			select {
			case <-t.C:
				q.remove(lockID)
			case <-c.done:
				// Unlock (or another remove) already took us out;
				// no work for the watchdog.
			}
		})
		return lockID, nil

	case <-ctx.Done():
		// We never acquired the lock. Remove ourselves from the queue.
		// If we happened to land at the head between enqueue and select
		// (race window: enqueue closed our ready right after we entered
		// select), remove() still does the right thing — it wakes the
		// next waiter when removing the head.
		q.remove(lockID)
		return "", errors.New("lock timeout")
	}
}

// Unlock releases the lock. It removes the caller from the queue and (if it
// was the head) wakes the next waiter via the queue's remove() logic.
//
// Returning an error when the lockID is not found preserves the original
// contract — typically this means the TTL already expired.
func (l *lock) Unlock(key string, lockID string) error {
	v, ok := l.queues.Load(key)
	if !ok {
		return errors.New("caller not found")
	}
	q := v.(*queue)
	if !q.remove(lockID) {
		return errors.New("caller not found")
	}
	return nil
}
