package models

import (
	"log/slog"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// CatalogModelRateLimitCounter demonstrates the power of conditional, type-safe,
// atomic increment operations in HydrAIDE — without ever reading, locking or blocking.
//
// 🔐 Use case:
// This model implements a per-user rate limiter where a user can only perform
// an action (e.g. password reset, API call) up to 10 times in a given time window.
//
// 🧠 What it demonstrates:
// - Lock-free atomic increment with conditions
// - No need to load the current value before writing
// - No external locking or mutexes — everything happens server-side, atomically
// - Full support for conditionally guarded state updates
//
// ✅ About Increment* functions:
//
// HydrAIDE supports atomic `Increment` operations for all numeric types:
//
//   - Int8, Int16, Int32, Int64
//   - Uint8, Uint16, Uint32, Uint64
//   - Float32, Float64
//
// Each has the form:
//
//	func (h *hydraidego) IncrementUint8(
//		ctx context.Context,
//		swampName name.Name,
//		key string,
//		value uint8,
//		condition *Uint8Condition,
//	) (uint8, error)
//
// These functions:
//
//   - Automatically create the Treasure if it doesn't exist
//   - Atomically increment the value without loading it first
//   - Optionally apply a condition on the current value before incrementing
//   - Fail with `ErrConditionNotMet` if the condition is not satisfied
//
// 🧮 Supported relational operators:
//
//   - Equal (==)
//   - NotEqual (!=)
//   - GreaterThan (>)
//   - GreaterThanOrEqual (>=)
//   - LessThan (<)
//   - LessThanOrEqual (<=)
//
// 🛡️ In this example:
//
// We implement a strict rate limit: max 10 actions per user.
// The `AttemptRateLimitedAction()` method:
//   - Atomically increments the counter
//   - Only if the current value is `< 10`
//   - Otherwise, the request is denied with no state change
//
// This pattern is ideal for:
//
//   - Password reset limits
//   - API rate limiting
//   - Usage throttling
//   - Abuse prevention
//
// 🔁 Usage:
//
//	c := &CatalogModelRateLimitCounter{}
//	allowed := c.AttemptRateLimitedAction(repoInstance, "user-abc123")
//
//	if allowed {
//	    // perform the action
//	} else {
//	    // reject or delay the request
//	}
//
// HydrAIDE handles everything — concurrency, safety, and atomicity — under the hood.
//
// This is not just an increment.
// This is **intent-first state control**, without friction.
//
// → Welcome to lock-free, condition-driven updates — the HydrAIDE way.
//
// ⚠️ Important:
// The increment operation locks only the **target Treasure** — not the entire Swamp.
// This means thousands of users can be rate-limited concurrently,
// safely and scalably, within the same Swamp — without contention.
type CatalogModelRateLimitCounter struct {
	UserID string `hydraide:"key"`
	Count  uint8  `hydraide:"value"`
}

// AttemptRateLimitedAction checks whether the given user is allowed to perform
// an action under a strict per-minute rate limit policy.
//
// 🧠 It uses HydrAIDE’s atomic IncrementUint8() function with a relational
// condition to ensure that the action count does not exceed the allowed limit.
//
// ⚠️ The condition ensures that:
//   - If the current value is < 10 → it is incremented and allowed
//   - If the current value is >= 10 → the increment is rejected
//
// This is a pure server-side, lock-free decision with no need to load the value first.
//
// 🔁 Time-based reset (important!):
//
//   - This implementation assumes that the rate limit counter is valid
//     *only within a time window* (e.g., 1 minute).
//
//   - To reset the state periodically, you can:
//
//     Destroy the entire Swamp every N seconds:
//
//   - h.Destroy(ctx, swamp)
//
//   - This deletes all users and resets their counters to zero
//
//   - Useful for time-bucketed, ephemeral rate limiting
//
// 🚀 Why use Destroy?
// Destroying the Swamp clears *all* users in one atomic call:
// - Rate limits reset to zero
// - Unused or offline users are purged
// - No manual loops or field updates needed
//
// 📌 Use a background cron job (e.g. every 60s) to call `h.Destroy(ctx, swamp)`.
//
// 🧱 This is ideal for high-scale systems where thousands of user limits are tracked.
//
// → State evaporates when no longer needed.
//
//	Rate limiting becomes stateless by design.
func (c *CatalogModelRateLimitCounter) AttemptRateLimitedAction(r repo.Repo, userID string) bool {

	// Create a context with a default timeout using the helper.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := r.GetHydraidego()

	swamp := c.createName()

	// Attempt to increment the counter if it's still under the limit
	newVal, err := h.IncrementUint8(ctx, swamp, userID, 1, &hydraidego.Uint8Condition{
		RelationalOperator: hydraidego.LessThan,
		Value:              10, // Max 10 actions per minute
	})

	// Evaluate result
	if err != nil {
		if hydraidego.IsConditionNotMet(err) {
			slog.Warn("Rate limit exceeded", "userID", userID)
			return false
		}
		slog.Error("Rate-limit increment failed", "userID", userID, "error", err)
		return false
	}

	slog.Info("Action allowed, counter incremented", "userID", userID, "newVal", newVal)
	return true
}

// RegisterPattern configures the Swamp used for per-user rate limiting.
//
// 🧠 Design rationale:
//
// In rate limiting, we don’t need long-term persistence — we only care
// about the current state during a specific time window (e.g., 1 minute).
//
// That’s why we configure this Swamp as:
//
// ✅ In-Memory Only (`IsInMemorySwamp: true`):
//   - Nothing is written to disk
//   - No I/O overhead, no cleanup required
//   - Memory is automatically reclaimed when unused
//
// ✅ Short Idle Expiration (`CloseAfterIdle: 60s`):
//   - If no user triggers the rate limiter for 1 minute,
//     the entire Swamp disappears from memory
//
// ✅ Outcome:
//   - Zero disk usage
//   - Auto-reset of all counters without manual deletion
//   - Stateless, ephemeral design — ideal for high-churn, real-time workloads
//
// 📌 This setup keeps your system lean, reactive and self-cleaning.
//
//	The moment rate limiting is no longer needed — it vanishes.
func (c *CatalogModelRateLimitCounter) RegisterPattern(repo repo.Repo) error {

	// Access the HydrAIDE client
	h := repo.GetHydraidego()

	// Create a bounded context for this registration operation
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// RegisterSwamp always returns a []error.
	// Each error (if any) represents a failure during Swamp registration on a HydrAIDE server.
	//
	// ⚠️ Even when only a single Swamp pattern is registered, HydrAIDE may attempt to replicate or validate
	// the pattern across multiple server nodes (depending on your cluster).
	//
	// ➕ Return behavior:
	// - If all servers succeeded → returns nil
	// - If one or more servers failed → returns a non-nil []error
	//
	// 🧠 To convert this into a single `error`, you can use the helper:
	//     hydraidehelper.ConcatErrors(errorResponses)
	errorResponses := h.RegisterSwamp(ctx, &hydraidego.RegisterSwampRequest{

		SwampPattern: c.createName(),

		CloseAfterIdle: time.Second * 60, // 1 minute

		// This is not an ephemeral in-memory Swamp — we persist it to disk
		IsInMemorySwamp: true,
	})

	// If there were any validation or transport-level errors, concatenate and return them
	if errorResponses != nil {
		return hydraidehelper.ConcatErrors(errorResponses)
	}

	return nil
}

// createModelCatalogQueueSwampName constructs the fully-qualified Swamp name
// for a specific queue under the catalog namespace in HydrAIDE.
func (c *CatalogModelRateLimitCounter) createName() name.Name {
	return name.New().Sanctuary("users").Realm("ratelimit").Swamp("counter")
}
