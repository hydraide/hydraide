package models

import (
	"log/slog"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// CatalogModelRateLimitCounter demonstrates HydrAIDE’s full power of conditional,
// type-safe, atomic increment operations — now with optional, server-side metadata
// control for both "create" and "update" paths.
//
// 🔐 Use case:
// Implements a per-user rate limiter where each user may perform an action
// (e.g. password reset, API call) up to a fixed number of times within a given
// time window (here, max 10 actions per minute).
//
// 🧠 What it demonstrates:
//   - Lock-free, atomic increment guarded by server-side conditions
//   - No need to read the current value before writing
//   - Automatic Treasure creation if it doesn’t exist
//   - Optional metadata assignment (CreatedAt/By, UpdatedAt/By, ExpiredAt)
//     in the same atomic operation
//   - Isolation at the Treasure level — no Swamp-wide locks
//
// ✅ About Increment* functions:
//
// HydrAIDE supports atomic `Increment` for all numeric types:
//
//   - Int8, Int16, Int32, Int64
//   - Uint8, Uint16, Uint32, Uint64
//   - Float32, Float64
//
// Modern form:
//
//	func (h *hydraidego) IncrementUint8(
//	    ctx context.Context,
//	    swampName name.Name,
//	    key string,
//	    value uint8,
//	    condition *Uint8Condition,
//	    setIfNotExist *IncrementMetaRequest,
//	    setIfExist *IncrementMetaRequest,
//	) (uint8, *IncrementMetaResponse, error)
//
// Parameters of note:
//   - `condition`: Optional rule applied atomically before incrementing.
//   - `setIfNotExist`: Metadata applied only if Treasure must be created.
//   - `setIfExist`: Metadata applied only if Treasure already exists.
//
// 🧮 Supported relational operators for conditions:
//   - Equal (==)
//   - NotEqual (!=)
//   - GreaterThan (>)
//   - GreaterThanOrEqual (>=)
//   - LessThan (<)
//   - LessThanOrEqual (<=)
//
// 🛡️ In this example:
//
// The `AttemptRateLimitedAction()` method:
//   - Atomically increments the per-user counter by 1
//   - Only if the current value is `< 10`
//   - Otherwise, rejects the increment with `ErrConditionNotMet`
//   - Always returns the latest value and metadata, even when rejected
//
// 📊 Example metadata usage here:
//   - When creating a new counter: set CreatedAt/CreatedBy and ExpiredAt (1-minute TTL)
//   - When updating an existing counter: set UpdatedAt/UpdatedBy and refresh ExpiredAt
//
// 🚀 Why this matters:
//   - Perfect for rolling time-window limits (via ExpiredAt refresh)
//   - Enables stateless design: once ExpiredAt passes, the Treasure disappears
//   - All handled server-side, atomically, with zero race conditions
//
// Typical applications:
//   - Password reset attempt limits
//   - API request throttling
//   - Abuse prevention
//   - Feature access gating
//
// 🔁 Usage:
//
//	c := &CatalogModelRateLimitCounter{UserID: "user-abc123"}
//	allowed := c.AttemptRateLimitedAction(repoInstance)
//
//	if allowed {
//	    // perform the action
//	} else {
//	    // reject or delay the request
//	}
//
// ⚠️ Important:
// Locking is scoped only to the **target Treasure**.
// This means thousands of distinct users can be incremented concurrently,
// safely and scalably, within the same Swamp without contention.
type CatalogModelRateLimitCounter struct {
	UserID string `hydraide:"key"`
	Count  uint8  `hydraide:"value"`
}

// AttemptRateLimitedAction enforces a strict, per-user action limit using HydrAIDE’s
// server-side, conditional Increment operation — without ever loading the current value first.
//
// Core purpose
// ------------
// This method implements a rate-limiting decision in a single atomic call to the HydrAIDE
// server. It increments a per-user counter *only if* the current value is still below
// the allowed maximum (here, < 10). If the limit is reached or exceeded, the increment
// is rejected and no state is changed.
//
// How it works
// ------------
//  1. A bounded context (`ctx`) is created for the request to ensure it times out if the
//     server is slow or unavailable.
//  2. The Swamp name is built via `createName()`, ensuring all counters for this use-case
//     share the same logical storage location in HydrAIDE.
//  3. The `IncrementUint8()` API is called with:
//     - the user’s unique key (`c.UserID`),
//     - an increment delta of `1`,
//     - a `Uint8Condition` that enforces: currentValue < 10.
//  4. HydrAIDE applies the condition and, if met, increments the counter atomically.
//     If not met, it returns `ErrConditionNotMet` without modifying the value.
//  5. The return value and error are examined:
//     - On `ErrConditionNotMet`: a warning is logged and the method returns `false`.
//     - On any other error: an error is logged and the method returns `false`.
//     - On success: an info log is written and the method returns `true`.
//
// Why this is powerful
// --------------------
//   - **Lock-free concurrency**: No client-side locking, no mutexes, no read-before-write.
//     Multiple clients can safely operate on the same key in parallel.
//   - **Single-roundtrip decision**: Both the check and the increment happen server-side
//     in one call, eliminating race conditions.
//   - **Clear intent**: The condition encodes business rules directly into the increment
//     request (here, “less than 10”).
//   - **Scalable isolation**: The lock scope is the single Treasure (user counter), not
//     the entire Swamp, allowing thousands of users to be tracked independently.
//
// Typical use cases
// -----------------
// - API rate limiting (requests per minute/hour/day)
// - Password reset attempt limits
// - Abuse prevention for high-cost operations
// - Feature gating based on usage counts
//
// Extending the pattern
// ---------------------
//   - Change the `RelationalOperator` or `Value` to enforce different thresholds.
//   - Use other `Increment*` variants (Int16, Uint64, Float32, etc.) for different types
//     or larger ranges.
//   - Combine with time-bucketed Swamp destruction (`Destroy()`) or TTL metadata to
//     reset counts automatically.
//
// In short, AttemptRateLimitedAction is an example of **intent-driven, condition-guarded,
// lock-free state mutation** — the HydrAIDE way of doing controlled increments without
// race conditions or extra reads.
func (c *CatalogModelRateLimitCounter) AttemptRateLimitedAction(r repo.Repo) bool {

	// Bounded context
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	h := r.GetHydraidego()
	swamp := c.createName()

	// Window/TTL: rolling 1 minute
	now := time.Now().UTC()

	setIfNotExist := &hydraidego.IncrementMetaRequest{
		SetCreatedAt: true,
		SetCreatedBy: "ratelimit",
		ExpiredAt:    now.Add(time.Minute),
	}
	setIfExist := &hydraidego.IncrementMetaRequest{
		SetUpdatedAt: true,
		SetUpdatedBy: "ratelimit",
		ExpiredAt:    now.Add(time.Minute), // refresh the expiration on every increment
	}

	// Atomic, server-side increment operation only if the condition is met
	newVal, meta, err := h.IncrementUint8(
		ctx,
		swamp,
		c.UserID,
		1, // increment by 1
		&hydraidego.Uint8Condition{
			RelationalOperator: hydraidego.LessThan,
			Value:              10, // max 10 action attempts allowed
		},
		setIfNotExist,
		setIfExist,
	)

	// Handle the result of the increment operation
	if err != nil {

		// If the condition was not met, it means the user has exceeded the limit
		if hydraidego.IsConditionNotMet(err) {
			slog.Warn("Rate limit exceeded",
				"userID", c.UserID,
				"current", newVal,
				"expiredAt", func() any {
					if meta != nil && !meta.ExpiredAt.IsZero() {
						return meta.ExpiredAt
					}
					return "n/a"
				}(),
			)
			return false
		}

		slog.Error("Rate-limit increment failed",
			"userID", c.UserID,
			"error", err,
		)
		return false
	}

	slog.Info("Action allowed, counter incremented",
		"userID", c.UserID,
		"newVal", newVal,
		"createdAt", func() any {
			if meta != nil && !meta.CreatedAt.IsZero() {
				return meta.CreatedAt
			}
			return "n/a"
		}(),
		"updatedAt", func() any {
			if meta != nil && !meta.UpdatedAt.IsZero() {
				return meta.UpdatedAt
			}
			return "n/a"
		}(),
		"expiredAt", func() any {
			if meta != nil && !meta.ExpiredAt.IsZero() {
				return meta.ExpiredAt
			}
			return "n/a"
		}(),
	)

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
