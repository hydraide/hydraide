package models

import (
	"log/slog"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// CatalogModelAchievementUnlock demonstrates how to use HydrAIDE's CatalogSaveManyToMany
// to record multiple achievement unlocks across many Swamps — in a single, efficient operation.
//
// 🧠 Use-case: multiplayer games where a player can unlock many achievements at once,
// such as at the end of a match, campaign, or boss fight.
//
// For each unlocked achievement, a separate Swamp is used:
//
//	→ Swamp name: `achievements/unlocked/<achievement-id>`
//	→ Key: `user-id`
//	→ Value: `timestamp of unlock`
//
// ✅ CatalogSaveManyToMany is the ideal function here because:
//   - It can write to many Swamps in one batch
//   - It automatically distributes writes across Hydra servers
//   - It provides fine-grained event feedback per Treasure
//
// 🔁 Each save triggers one of three `EventStatus` values per Treasure:
//   - `StatusNew` → the achievement was unlocked for the first time
//   - `StatusModified` → the timestamp changed or was updated
//   - `StatusNothingChanged` → already stored, no update needed
//
// This model helps demonstrate how Swamp-per-key design fits real-time,
// event-driven systems like games, IoT, or semantic indexing.
//
// ---
//
// 🔧 Example usage:
//
//	unlocked := []string{"first-kill", "10-wins", "level-50", "survivor"}
//	user := "player-123"
//	timestamp := time.Now()
//
//	err := (&CatalogModelAchievementUnlock{}).
//	    SaveAchievements(repoInstance, user, unlocked, timestamp)
//	if err != nil {
//	    log.Fatalf("Failed to save achievements: %v", err)
//	}
//
// This will insert the user's ID into 4 different Swamps — one for each achievement.
// If the user already unlocked one of them previously, only new or changed entries are updated.
type CatalogModelAchievementUnlock struct {
	UserID     string    `hydraide:"key"`       // Who unlocked the achievement
	UnlockedAt time.Time `hydraide:"createdAt"` // When it was unlocked
}

// SaveAchievements stores all unlocked achievements for a user
// in the corresponding Swamps (one Swamp per achievement).
//
// 📌 In this model, each achievement has its own Swamp:
//
//	→ Swamp: `achievements/unlocked/<achievement-id>`
//	→ Key:   `user-id`
//	→ Meta / UnlockedAt: `timestamp of unlock`
//
// This means that unlocking 10 achievements will write to 10 different Swamps.
//
// ✅ This function uses CatalogSaveManyToMany – a powerful HydrAIDE feature that:
//   - Saves multiple Treasures across multiple Swamps
//   - Automatically routes writes to the correct Hydra servers
//   - Triggers per-key status feedback: New, Modified, or NothingChanged
func (c *CatalogModelAchievementUnlock) SaveAchievements(r repo.Repo, userID string, achievementIDs []string, unlockedAt time.Time) error {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := r.GetHydraidego()

	// Build the batch request:
	// For each achievement, we generate:
	//   - A model of the user unlocking it
	//   - A corresponding Swamp name based on that achievement ID
	requests := make([]*hydraidego.CatalogManyToManyRequest, 0, len(achievementIDs))
	for _, achievementID := range achievementIDs {
		model := &CatalogModelAchievementUnlock{
			UserID:     userID,
			UnlockedAt: unlockedAt,
		}

		// Swamp pattern: achievements/unlocked/<achievement-id>
		swampName := c.createCatalogName(achievementID)

		// Append request to the batch
		requests = append(requests, &hydraidego.CatalogManyToManyRequest{
			SwampName: swampName,
			Models:    []any{model},
		})
	}

	// 🧠 Track the event statuses (for observability or audit logs)
	var newCount, modifiedCount, unchangedCount int

	// Perform the multi-Swamp save operation.
	// The iterator will be called once per (swamp + user) pair,
	// and tells us whether it was a new write, an update, or a no-op.
	err := h.CatalogSaveManyToMany(ctx, requests, func(swamp name.Name, key string, status hydraidego.EventStatus) error {
		switch status {
		case hydraidego.StatusNew:
			slog.Info("Achievement unlocked",
				"swamp", swamp.Get(), // e.g. achievements/unlocked/first-kill
				"userID", key,
				"status", status)
			newCount++
		case hydraidego.StatusModified:
			slog.Info("Achievement updated",
				"swamp", swamp.Get(),
				"userID", key,
				"status", status)
			modifiedCount++
		case hydraidego.StatusNothingChanged:
			slog.Info("Achievement unchanged",
				"swamp", swamp.Get(),
				"userID", key,
				"status", status)
			unchangedCount++
		default:
			slog.Warn("Unexpected status for achievement unlock",
				"swamp", swamp.Get(),
				"userID", key,
				"status", status)
		}
		return nil // returning an error here would abort the entire batch
	})

	// Handle and propagate any failure from the CatalogSaveManyToMany call
	if err != nil {
		return err
	}

	// Summary output – visible in logs or ops dashboards
	slog.Info("SaveAchievementsForUser complete",
		"newCount", newCount,
		"modifiedCount", modifiedCount,
		"unchangedCount", unchangedCount)

	return nil
}

// RegisterPattern declares the Swamp behavior for all achievement-unlock Swamps.
//
// 🧠 Why use a wildcard (`*`) pattern here?
//
// In this model, every achievement ID gets its **own unique Swamp**, like:
//
//	→ `achievements/unlocked/first-kill`
//	→ `achievements/unlocked/level-100`
//	→ `achievements/unlocked/win-10-matches`
//
// Since these Swamps are created **dynamically**, one per achievement,
// we register a wildcard pattern to define common behavior for *all* of them:
//
//	→ `achievements/unlocked/*`
//
// ✅ This pattern ensures that every new Swamp created under `achievements/unlocked/`
// will automatically inherit the same settings:
//
//   - In-memory timeout: 6 hours (if idle, it will be flushed and closed)
//   - Persistent storage: data is saved to disk (not just in RAM)
//   - Write strategy: small files, flushed every 10s (good for small, frequent writes)
//
// Without this wildcard, each individual Swamp would either:
//   - Not be initialized at all (default behavior)
//   - Or require manual `RegisterSwamp()` call per achievement, which is impractical
//
// So this function acts like a "Swamp class config" for an entire category.
//
// 📌 Must be called once before the first use of SaveAchievements.
func (c *CatalogModelAchievementUnlock) RegisterPattern(r repo.Repo) error {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := r.GetHydraidego()

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
		// The Swamp pattern name: users/catalog/all
		SwampPattern: name.New().Sanctuary("achievements").Realm("unlocked").Swamp("*"),

		// Keep Swamp in memory for 6 hours of idle time
		CloseAfterIdle: time.Second * 21600,

		// Use persistent, disk-backed storage
		IsInMemorySwamp: false,

		// Configure file writing: frequent small chunks
		FilesystemSettings: &hydraidego.SwampFilesystemSettings{
			WriteInterval: time.Second * 10, // flush changes every 10s
			MaxFileSize:   8192,             // 8 KB max chunk size
		},
	})

	if errorResponses != nil {
		return hydraidehelper.ConcatErrors(errorResponses)
	}

	return nil
}

// createCatalogName returns the name of the Catalog Swamp
// where the user data will be stored.
func (c *CatalogModelAchievementUnlock) createCatalogName(achievementID string) name.Name {
	return name.New().Sanctuary("achievements").Realm("unlocked").Swamp(achievementID)
}
