// Package hydraidehelper is a utility package providing reusable helpers
// to standardize context timeout behavior for HydrAIDE operations,
// and to implement logical-level distributed locking in the HydrAIDE database.
package hydraidehelper

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// CreateHydraContext creates a context with a 5-second timeout.
// This is a convenience function to ensure consistent timeout handling
// across HydrAIDE operations. The default 5 seconds is typically more than
// sufficient for most database interactions, but can be overridden at call-site if needed.
func CreateHydraContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

// Lock creates a logical, database-level lock using HydrAIDE's native locking mechanism.
// The lock is always created in Island 0, ensuring consistency across all clients.
// You can define how long the lock should be held via the TTL parameter. After the TTL expires,
// the lock is automatically released by HydrAIDE to prevent deadlocks,
// even if a client crashes or fails to unlock explicitly.
//
// When to use this?
// - When you want to protect a logical operation, not a specific Swamp
// - When concurrent access to a critical section must be serialized
//
// Example use case:
// If you're updating a user's balance and want to prevent concurrent access:
//
//	Lock name: "bearbite_balance_update"
//	→ All other clients trying to acquire the same lock will be queued.
//	→ Once the first process finishes, the next in line acquires the lock.
func Lock(r repo.Repo, lockName string, ttl time.Duration) (lockID string) {
	ctx, cancelFunc := context.WithTimeout(context.Background(), ttl)
	defer cancelFunc()
	lockID, err := r.GetHydraidego().Lock(ctx, lockName, ttl)
	if err != nil {
		slog.Error("failed to create hydra lock", "err", err.Error(), lockName, lockName)
		return ""
	}
	return lockID
}

// Unlock releases a previously acquired lock.
// Note: If you forget to call Unlock, HydrAIDE will still automatically
// release the lock after the original TTL expires.
func Unlock(r repo.Repo, lockName, lockID string) {
	ctx, cancelFunc := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelFunc()
	if err := r.GetHydraidego().Unlock(ctx, lockName, lockID); err != nil {
		slog.Error("failed to unlock the hydra lock", "lockName", lockName, "lockID", lockID, "error", err.Error())
		return
	}
}

// ConcatErrors combines multiple HydrAIDE-related errors into a single error instance.
// Useful for streaming operations or iterative responses where you collect multiple errors
// and want to report them together.
func ConcatErrors(errs []error) error {

	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}

	errString := ""
	for _, e := range errs {
		errString += e.Error() + "\n"
	}

	return errors.New(strings.TrimSpace(errString))

}
