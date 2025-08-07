package models

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// BasicsLock demonstrates how to safely update a shared logical resource
// (e.g. user balance) using HydrAIDE’s built-in distributed lock system.
//
// 🔒 Why use this?
// In microservice architectures, multiple routines or services may attempt to update
// the same user data concurrently. Without a shared locking mechanism, this can lead to:
// - Race conditions
// - Inconsistent writes
// - Lost updates
//
// HydrAIDE offers a **per-key FIFO lock queue** that:
// - Works across services, processes, and even servers
// - Automatically releases locks after a TTL (deadlock prevention)
// - Requires no external infra (no Redis, no etcd)
//
// ✅ Use this pattern when:
// - You update shared state (e.g. balances, carts, scores)
// - You need guaranteed serialization by key
// - You want a lock that survives microservice restarts
//
// ❗ What this is NOT:
// - This is not a goroutine mutex (like `sync.Mutex`)
// - It’s not scoped to memory or a process — it’s global and distributed
//
// 🛠 Best practice:
// - Use meaningful, specific lock keys (e.g. "userBalance-{userID}")
// - Call Lock() as early as possible
// - Always use `defer Unlock()`
// - Set a reasonable TTL based on expected execution time
//
// 💡 TTL explained:
// If your service crashes or forgets to unlock, the lock is automatically released
// after the TTL expires. This prevents stuck locks and promotes system resilience.
type BasicsLock struct {
	UserID      string `hydraide:"key"`   // The user's unique identifier — used as the lock scope
	UserBalance int32  `hydraide:"value"` // Not used in this example, but part of the model
}

// IncreaseUserBalance shows how to lock, perform a critical operation,
// and unlock safely — with TTL fallback.
//
// ⚙️ Lock behavior:
// - Locks are scoped to a custom key (in this case: userBalance-{UserID})
// - All routines trying to acquire the same lock will wait (FIFO)
// - Each lock has a TTL (e.g. 5s) to prevent deadlocks
// - If the TTL expires, the lock is released automatically
//
// ✅ Why this works:
// - Stateless services can coordinate updates
// - You avoid race conditions and write conflicts
// - You don’t need Redis, etcd, or external lock managers
//
// 🔐 Best practice:
// - Call Lock() early in the function
// - Always defer Unlock(), even on error paths
// - Use unique lock keys per resource (e.g. per-user, per-basket)
func (m *BasicsLock) IncreaseUserBalance(repo repo.Repo) error {

	// Create a timeout-aware context
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Get the HydrAIDE SDK instance
	h := repo.GetHydraidego()

	// Generate a unique lock key per user
	// This ensures that each user’s balance is independently lockable
	lockKey := fmt.Sprintf("userBalance-%s", m.UserID)

	// Attempt to acquire the lock — with a TTL to auto-release if something goes wrong
	lockID, err := h.Lock(ctx, lockKey, time.Second*5)
	if err != nil {
		return err
	}

	// Always defer unlock to ensure the lock is released even on panic or early return
	defer func() {
		if err := h.Unlock(ctx, lockKey, lockID); err != nil {
			slog.Error("failed to unlock the HydrAIDE lock",
				"lockKey", lockKey, "lockID", lockID, "error", err)
		}
	}()

	// 🔁 At this point, only this routine holds the lock for this user.
	// You can safely perform the critical section logic here:

	// 1. Load the user's current balance from HydrAIDE
	// 2. Modify the balance (e.g. add +100 credits)
	// 3. Save the updated value back to the Swamp
	// 4. Return nil (or any error from the operation)

	return nil
}
