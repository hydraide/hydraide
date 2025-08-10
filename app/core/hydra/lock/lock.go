package lock

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/hydraide/hydraide/docs/sdk/go/examples/applications/app-queue/utils/panichandler"
	"sync"
	"time"
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
	// This map stores the waiting goroutines for each key.
	// Each entry holds a queue struct that contains the waiting goroutines.
	queues sync.Map
}

func New() Lock {
	return &lock{}
}

func newQueue() *queue {
	q := &queue{}
	return q
}

type queue struct {
	mu      sync.RWMutex
	callers []string
}

func (q *queue) AddCaller(caller string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.callers = append(q.callers, caller)
}

func (q *queue) DeleteCaller(callerID string) error {

	q.mu.Lock()
	defer q.mu.Unlock()

	for i, caller := range q.callers {

		if caller == callerID {

			// If there are more waiting goroutines, remove the first one from the queue.
			// If the queue is empty, delete the entry entirely.
			if len(q.callers) > 1 {
				q.callers = append(q.callers[:i], q.callers[i+1:]...)
			} else {
				q.callers = []string{}
			}
			return nil

		}
	}

	return errors.New("caller not found")

}

func (q *queue) CanExecute(callerID string) bool {

	q.mu.RLock()
	defer q.mu.RUnlock()

	if len(q.callers) == 0 {
		return true
	}

	return q.callers[0] == callerID

}

// StartAutoUnlock ensures that the lock is forcefully released when the TTL expires.
func (q *queue) StartAutoUnlock(ctx context.Context, callerID string, ttl time.Duration) {

	// handling the panic because this function called by a goroutine
	defer panichandler.PanicHandler()

	// Remove the caller after the TTL expires. Whichever timeout comes first will be used.
	// Exit the goroutine immediately afterward.
	t := time.NewTicker(ttl)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			// If the callerID is found in the waiting queue, remove it.
			_ = q.DeleteCaller(callerID)
			return
		case <-t.C:
			_ = q.DeleteCaller(callerID)
			return
		}
	}

}

func (l *lock) Lock(ctx context.Context, key string, ttl time.Duration) (lockID string, err error) {

	// generate lockID
	lockID = uuid.NewString()
	// Retrieve the queue associated with the key, or create it if it doesn't exist.
	q := l.getQueue(key)
	// Add the caller as a waiting entry in the queue.
	q.AddCaller(lockID)

	for {
		select {
		case <-ctx.Done():

			// On timeout, remove the caller from the queue since it can no longer wait.
			_ = q.DeleteCaller(lockID)
			return "", errors.New("lock timeout")

		default:

			// Check if the caller is at the front of the queue.
			if q.CanExecute(lockID) {
				// When granting the lock to the caller, start a goroutine
				// that will automatically release the lock after the TTL expires.
				// This prevents deadlocks in the database that could block other callers.
				// If the lock is released this way, Unlock will return an error
				// indicating that a timeout occurred.
				go q.StartAutoUnlock(ctx, lockID, ttl)
				return lockID, nil
			}

			continue

		}
	}

}

func (l *lock) Unlock(key string, lockID string) error {
	// Retrieve the queue associated with the given key.
	q := l.getQueue(key)
	// Remove the caller from the queue.
	return q.DeleteCaller(lockID)
}

func (l *lock) getQueue(key string) *queue {
	actual, _ := l.queues.LoadOrStore(key, newQueue())
	return actual.(*queue)
}
