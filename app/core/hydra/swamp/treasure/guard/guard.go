// Package guard provides a concurrency-safe way to transaction a Treasure for exclusive access.
// It ensures that only one goroutine can execute operations on a Treasure at a given time.
// It also maintains a queue of goroutines waiting for the transaction, ensuring that they
// gain access to the Treasure in the order they requested it.
//
// As you can see, we use the guardID for every function call except for the GetKey function.
// The Guard object and the GuardID ensure that a given function, e.g. CloneContent, runs only when
// another goroutine is not using it. The Guard is not a standard sync.Mutex, but a special object that not
// only prevents another routine from accessing a single treasure at the same time, but also establishes an
// order, as it's very important in databases to have calling routines modify or access information in the
// order they were called. For example, if a user wants to top up their balance, we first top it up, and then
// they can purchase anything from their balance. If the two routines come in almost simultaneously, the regular
// Sync package doesn't guarantee that the order would necessarily be top-up followed by purchase. That's why we
// use the Guard to protect the data and also ensure the correct sequence.
package guard

import (
	"errors"
	"sync"
	"sync/atomic"
)

// Guard is an interface that defines methods for locking and unlocking a Treasure in a thread-safe and
// orderly manner. The interface is specifically designed to be part of the Treasure interface.
//
// Methods:
// - StartTreasureGuard: Acquires a transaction and returns a unique guardID.
// - ReleaseTreasureGuard: Releases the transaction for a given guardID.
// - CanExecute: Checks if the given guardID is first in the queue, and thus can perform operations on the Treasure.
type Guard interface {

	// StartTreasureGuard is a function that acquires a transaction on the Treasure and returns a unique guardID.
	// The function can be called with or without the waiting parameter. If waiting is true, the function will
	// block the goroutine until the Treasure is unlocked. If waiting is false, the function will return immediately
	// with a guardID of 0 if the Treasure is locked. This allows the calling goroutine to decide whether to wait
	// for the Treasure to be unlocked or to proceed with other operations.
	//
	// Example usage:
	//     guardID := treasure.StartTreasureGuard(true)
	//     // Perform your operations
	//     ...
	//     // Release the transaction
	//     treasure.ReleaseTreasureGuard(guardID)
	//
	StartTreasureGuard(waiting bool, bodyAuthID ...string) (guardID ID)

	// ReleaseTreasureGuard is a function that releases the transaction on the Treasure for the given guardID.
	// Once a goroutine completes its operations on the Treasure, it should call this method to free up
	// the Treasure for other goroutines in the queue.
	//
	// This is crucial for maintaining the order and flow of operations, as well as for system resources,
	// since failing to call this function could lead to other goroutines being indefinitely blocked or a resource deadlock.
	//
	// The function expects the guardID that was obtained from the StartTreasureGuard call. It's essential
	// to match the guardID correctly; otherwise, the unlock operation won't be successful.
	//
	// Example usage:
	//     guardID := treasure.StartTreasureGuard()
	//     // Perform your operations
	//     ...
	//     // Release the transaction
	//     treasure.ReleaseTreasureGuard(guardID)
	//
	// Important: Always make sure to call this function after you've completed your operations on the Treasure.
	// Failure to do so will prevent other goroutines in the queue from accessing the Treasure.
	//
	// Parameters:
	// - guardID int64: The unique identifier that was used to transaction the Treasure.
	//
	// Note: There is no need to manually call CanExecute before calling this function. The Guard system internally
	// handles the logic to allow the next goroutine in the queue to proceed.
	ReleaseTreasureGuard(guardID ID)

	// CanExecute is an internal function that checks if the given guardID is the first in the queue,
	// thereby determining if the calling goroutine can proceed with operations on the Treasure.
	//
	// IMPORTANT: This function is integrated into all Treasure-related functions and SHOULD NOT be called
	// directly, especially not from the Hydra head. Doing so can cause unintended behaviors and may disrupt
	// the sequencing of operations on the Treasure.
	//
	// The function waits until the guardID at the head of the queue matches the given guardID, effectively
	// blocking the goroutine until it's its turn. This ensures that operations are executed in the order they
	// were initiated, which is crucial for data consistency in database-like settings.
	//
	// Once CanExecute confirms that the guardID is at the head of the queue, the Treasure-related function will
	// proceed with its operation.
	//
	// Note: CanExecute is designed to be part of the internal workflow of the Treasure system. It ensures
	// correct sequencing but is not meant for external invocation. Always use the other functions provided by
	// the Treasure interface, and they will handle the sequencing internally using CanExecute.
	CanExecute(guardID ID, isBodyFunction ...bool) error
}

type ID int64

type guard struct {
	mu             sync.RWMutex
	cond           *sync.Cond
	waitForUnlock  []int64 // Queue of transaction IDs waiting for the Treasure to be unlocked.
	largestGuardID int64   // The highest transaction ID that has been unlocked.
	bodyAuthID     string  // The bodyAuthID of the goroutine that currently holds the transaction.
}

const (
	BodyAuthID = "kby1CXR0wkj@qpa2ynq"
)

// New creates a new Guard instance.
func New() Guard {
	l := &guard{
		waitForUnlock: make([]int64, 0),
	}
	l.cond = sync.NewCond(&l.mu)
	return l
}

// StartTreasureGuard start the treasure guard for the treasure and waits until the Treasure is unlocked if the waiting is true.
// otherwise it returns with 0
func (g *guard) StartTreasureGuard(waiting bool, bodyAuthID ...string) (guardID ID) {
	// Lock the condition's associated lock to ensure thread safety
	g.cond.L.Lock()
	defer g.cond.L.Unlock() // Ensure the lock is always released at the end of the function

	// If bodyAuthID is provided and g.bodyAuthID is empty, set it
	if len(bodyAuthID) > 0 {
		if g.bodyAuthID == "" {
			g.bodyAuthID = bodyAuthID[0]
		}
	}

	// If waiting is true, wait until the treasure is unlocked
	if waiting {
		// Increment the largestGuardID atomically and assign it to gID
		gID := atomic.AddInt64(&g.largestGuardID, 1)
		// Swamp the new guard ID to the waitForUnlock slice
		g.waitForUnlock = append(g.waitForUnlock, gID)
		if len(bodyAuthID) > 0 {
			g.bodyAuthID = bodyAuthID[0]
		}
		// Wait while the current guard ID is not the first in the queue
		for g.waitForUnlock[0] != gID {
			g.cond.Wait()
		}
		// Return the guard ID
		return ID(gID)
	} else {
		// If not waiting, and there are no other guards waiting
		if len(g.waitForUnlock) == 0 {
			// Increment the largestGuardID atomically and assign it to gID
			gID := atomic.AddInt64(&g.largestGuardID, 1)
			// Swamp the new guard ID to the waitForUnlock slice
			g.waitForUnlock = append(g.waitForUnlock, gID)
			if len(bodyAuthID) > 0 {
				g.bodyAuthID = bodyAuthID[0]
			}
			// Return the guard ID
			return ID(gID)
		}
	}

	// If not waiting and there are other guards waiting, return 0
	return ID(0)
}

// ReleaseTreasureGuard releases the transaction on the Treasure for the given guardID.
// It also notifies all waiting goroutines that the Treasure is now unlocked.
func (g *guard) ReleaseTreasureGuard(guardID ID) {

	g.cond.L.Lock()
	defer func() {
		g.cond.L.Unlock()
	}()

	if len(g.waitForUnlock) > 0 && g.waitForUnlock[0] == int64(guardID) {
		g.waitForUnlock = g.waitForUnlock[1:]
		if len(g.waitForUnlock) == 0 {
			atomic.StoreInt64(&g.largestGuardID, 0)
		}
		g.cond.Broadcast()
		return
	}

}

// CanExecute checks if the given guardID is the first in the queue.
// If it's not, the function panics, indicating a critical programming error.
// it will also check if the bodyAuthID is the same as the one that was used to transaction the Treasure.
// It will return error only if the bodyAuthID is not the same as the one that was used to transaction the Treasure and the
// function is a body function.... otherwise it will return nil.
func (g *guard) CanExecute(guardID ID, isBodyFunction ...bool) error {
	g.cond.L.Lock()
	defer g.cond.L.Unlock()
	if len(isBodyFunction) > 0 && isBodyFunction[0] {
		if g.bodyAuthID != BodyAuthID {
			return errors.New("can not execute body function, the bodyAuthID is not correct")
		}
	}
	// an empty queue means no guard is held at all (e.g. a guardID of another
	// treasure, or one already released): an error, not an index panic
	if len(g.waitForUnlock) == 0 || g.waitForUnlock[0] != int64(guardID) {
		return errors.New("the given guardID is not the first in the queue - execution aborted")
	}
	return nil
}
