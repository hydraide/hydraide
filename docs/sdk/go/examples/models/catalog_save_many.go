package models

import (
	"log"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// CatalogModelGamePlayer saves multiple players at once into the Swamp.
//
// 🧠 This function demonstrates a **batch save pattern** for online multiplayer games,
// where all active players must be registered at the start of a session.
//
// 🔥 Use-case: Lobby-based games (e.g. FPS matches, arena games, card battles)
// where multiple players join a game instance at once, and their state must be
// initialized and tracked together.
//
// Each player is stored as a Treasure inside a shared Swamp, scoped to the `gameID`.
//
// 💾 The Swamp is named: games/current-sessions/<gameID>
//
// ✅ `CatalogSaveMany` creates each Treasure if it doesn't exist (StatusNew),
// or updates it if the key exists but the value changed (StatusModified).
// If nothing has changed, it triggers StatusNothingChanged.
//
// 🔁 For each player, an event is fired — so downstream logic (leaderboards, subscriptions,
// UI overlays) can react in real-time.
//
// This is ideal for:
//   - Initializing state at game start
//   - Restoring presence after reconnect
//   - Synchronizing client-server state in reactive dashboards
//
// Important:
// - This does not deduplicate or validate players – it writes whatever is passed.
// - Always call RegisterPattern before using SaveMany to ensure the Swamp is properly configured.
type CatalogModelGamePlayer struct {
	PlayerID  string    `hydraide:"key"`       // Unique player ID
	Data      *GameData `hydraide:"value"`     // Player's session data
	CreatedAt time.Time `hydraide:"createdAt"` // Timestamp of joining the game
	UpdatedAt time.Time `hydraide:"updatedAt"` // Last time this data was updated
}

// GameData represents player's runtime data.
type GameData struct {
	Level    int  // Current level of the player
	IsOnline bool // Is the player currently online
	Score    int  // Current score
	IsBanned bool // Banned players are excluded from rankings
}

// SaveMany saves multiple players at once into the Swamp.
// Each operation triggers a real-time status event (New, Modified, NothingChanged).
func (c *CatalogModelGamePlayer) SaveMany(r repo.Repo, gameID string, players []*CatalogModelGamePlayer) error {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := r.GetHydraidego()

	// Convert players to []any
	models := make([]any, 0, len(players))
	for _, p := range players {
		models = append(models, p)
	}

	// Pick one of them to resolve Swamp name
	if len(players) == 0 {
		return nil
	}

	swamp := c.createCatalogName(gameID)

	// Save all players in a single batch using CatalogSaveMany.
	//
	// 💡 For each player, the provided iterator function will be invoked
	// with the resulting `EventStatus`, allowing custom handling per key.
	//
	// You can:
	//   - Track how many new players joined
	//   - Trigger events for modified records
	//   - Ignore or log unchanged players
	//   - Collect keys for further processing (e.g. leaderboard update)
	//
	// This iterator runs sequentially *after* the batch operation succeeded.
	var newCount, modifiedCount, unchangedCount int
	var modifiedKeys []string

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
	err := h.CatalogSaveMany(ctx, swamp, models, func(key string, status hydraidego.EventStatus) error {
		switch status {
		case hydraidego.StatusNew:
			log.Printf("[SaveMany] 🎉 New player registered: %s", key)
			newCount++
		case hydraidego.StatusModified:
			log.Printf("[SaveMany] 🔁 Player updated: %s", key)
			modifiedCount++
			modifiedKeys = append(modifiedKeys, key)
		case hydraidego.StatusNothingChanged:
			log.Printf("[SaveMany] ✅ No change for: %s", key)
			unchangedCount++
		default:
			log.Printf("[SaveMany] ⚠️ Unknown status for: %s", key)
		}
		return nil // returning error would halt iteration
	})
	if err != nil {
		return err
	}

	log.Printf("CatalogSaveMany finished: %d new, %d modified, %d unchanged players",
		newCount, modifiedCount, unchangedCount)

	// Optional: trigger downstream logic
	if len(modifiedKeys) > 0 {
		// ✅ Trigger downstream logic, e.g. update scoreboard service or UI overlays
		// go triggerLeaderboardUpdate(modifiedKeys)
	}

	return nil

}

// RegisterPattern defines the Swamp behavior: disk-backed, with 15min memory timeout.
func (c *CatalogModelGamePlayer) RegisterPattern(r repo.Repo, gameID string) error {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := r.GetHydraidego()

	errorResponses := h.RegisterSwamp(ctx, &hydraidego.RegisterSwampRequest{
		// The Swamp pattern name: games/current-sessions/[uuid]
		SwampPattern: c.createCatalogName(gameID),

		// Keep Swamp in memory for 10 minutes for the game session
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

// createCatalogName returns the Swamp name for storing game players.
func (c *CatalogModelGamePlayer) createCatalogName(gameID string) name.Name {
	return name.New().Sanctuary("games").Realm("current-sessions").Swamp(gameID)
}
