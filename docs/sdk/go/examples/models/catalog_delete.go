package models

import (
	"log/slog"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// CatalogModelChatMessage represents a single chat message in a room-specific Swamp.
//
// ✅ Each chat room is mapped to its own Swamp: chat/room/{roomID}
// ✅ Each message is stored as a Treasure inside that Swamp (key = MessageID)
//
// This model demonstrates how to use HydrAIDE’s CatalogDelete and CatalogDeleteMany
// to safely and efficiently remove chat messages — both individually and in batches.
//
// 🧨 Deletion behavior:
// - Messages are permanently removed from storage (no soft delete, no flags)
// - If the last message is deleted from a Swamp, the entire Swamp is destroyed
// - Swamp patterns remain active and reusable without re-registration
//
// ✂️ Use CatalogDelete when:
// - You want to delete a **single message**
// - You want precise logging or per-message feedback
//
// 🧹 Use CatalogDeleteMany when:
// - You want to delete **multiple messages at once** from the same room
// - You want to process results in bulk, but with per-key callbacks
// - You want to reduce load and write amplification
//
// 💡 Both methods trigger `CatalogDeleted` events for Subscribers of the Swamp.
// This ensures real-time updates to connected clients or systems.
//
// See Delete() and DeleteMany() for detailed examples and behavior.
type CatalogModelChatMessage struct {
	MessageID string    `hydraide:"key"`       // Unique ID of the message
	Text      string    `hydraide:"value"`     // The message content
	CreatedBy string    `hydraide:"createdBy"` // Who sent the message
	CreatedAt time.Time `hydraide:"createdAt"` // When it was sent
}

// Delete removes a single message from the specified chat room (Swamp).
//
// This method calls HydrAIDE’s CatalogDelete to permanently delete a single Treasure
// (chat message) from a room-specific Swamp (`chat/room/{roomID}`).
//
// If this was the last message in the Swamp:
//
//	→ The Swamp folder is automatically deleted from disk (zero-waste behavior)
//
// 📘 See hydraidego.CatalogDelete() for full logic and edge-case behavior.
//
// 🔁 Example usage:
//
//	msg := &CatalogModelChatMessage{
//	    MessageID: "msg-abc123",
//	}
//
//	err := msg.Delete(repo, "room-42")
//	if err != nil {
//	    log.Fatal("Failed to delete message:", err)
//	}
//
// In this example:
// - Only the Treasure with key "msg-abc123" will be removed
// - If it's the last message in "room-42", that Swamp is destroyed too
func (c *CatalogModelChatMessage) Delete(r repo.Repo, roomID string) error {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := r.GetHydraidego()

	swamp := c.getSwampName(roomID)

	// 📘 See CatalogDelete() for full behavior documentation
	err := h.CatalogDelete(ctx, swamp, c.MessageID)
	if err != nil {
		if hydraidego.IsNotFound(err) {
			slog.Warn("Message not found – nothing to delete", "messageID", c.MessageID)
		} else if hydraidego.IsSwampNotFound(err) {
			slog.Warn("Room Swamp already gone", "swamp", swamp.Get())
		} else {
			slog.Error("Failed to delete message", "messageID", c.MessageID, "error", err)
		}
		return err
	}

	slog.Info("Message deleted", "messageID", c.MessageID, "roomID", roomID)
	return nil
}

// DeleteMany removes multiple messages from the same chat room (Swamp) in a single operation.
//
// This method demonstrates how to use HydrAIDE’s CatalogDeleteMany() to batch-delete
// multiple Treasures (messages) from a single room Swamp, while logging each outcome.
//
// ✅ Benefits:
// - More efficient than calling Delete() individually
// - Frees up memory and disk if this empties the Swamp
// - Keeps the Swamp clean and minimal, especially for volatile chat rooms
//
// ⚠️ Behavior:
// - If the deleted messages were the last in the room, the Swamp is removed from disk
// - The Swamp pattern remains active — no need to re-register
//
// 🧠 Iterator advantage:
// The provided iterator receives per-message results, allowing:
// - Error handling for specific keys
// - Conditional logging or audit collection
// - Real-time metrics or feedback generation
//
// 🔔 Events:
// For every deleted message, HydrAIDE emits a `CatalogDeleted` event.
//
// If the Swamp (chat room) has active Subscribers, each one will
// automatically receive a deletion notification for each affected key.
//
// This ensures:
// - Real-time UI sync for chat clients
// - Instant propagation of deletion state across devices or users
//
// 🔁 Example usage:
//
//	msg := &CatalogModelChatMessage{}
//
//	err := msg.DeleteMany(repo, "room-42", []string{
//	    "msg-001", "msg-002", "msg-003",
//	})
//
//	if err != nil {
//	    log.Fatal("Batch deletion failed:", err)
//	}
//
// In this example:
// - All listed message IDs are deleted from chat/room/room-42
// - Each deletion is logged individually
// - If these were the last messages in the room, the Swamp is destroyed
// - All Subscribers to the room will receive `CatalogDeleted` events
func (c *CatalogModelChatMessage) DeleteMany(r repo.Repo, roomID string, messageIDs []string) error {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := r.GetHydraidego()

	swamp := c.getSwampName(roomID)

	// Perform batch deletion using CatalogDeleteMany
	err := h.CatalogDeleteMany(ctx, swamp, messageIDs, func(key string, err error) error {
		switch {
		case err == nil:
			slog.Info("Message deleted", "messageID", key, "roomID", roomID)
		case hydraidego.IsNotFound(err):
			slog.Warn("Message not found – skipped", "messageID", key)
		case hydraidego.IsSwampNotFound(err):
			slog.Warn("Room Swamp not found", "roomID", roomID)
		default:
			slog.Error("Failed to delete message", "messageID", key, "roomID", roomID, "error", err)
		}
		return nil // Continue processing all keys
	})

	if err != nil {
		slog.Error("Batch message deletion failed", "roomID", roomID, "error", err)
	}

	return err
}

// RegisterPattern registers the Swamp pattern used for storing chat messages by room.
//
// ✅ Swamp pattern: chat/room/*
//
//	→ Each chat room is its own Swamp, identified by its room ID
//
// This registration ensures that:
// - Swamps are disk-backed (not in-memory only)
// - Each Swamp stays "hot" in memory for 30 minutes after last use
// - Write operations are flushed to disk every 10 seconds in small 8KB chunks
//
// 🧠 Why this matters:
// - Enables high-performance chat at scale, with per-room isolation
// - Frees up memory automatically when rooms are inactive
// - Ensures that deleted rooms leave no trace unless repopulated
//
// ⚠️ Note:
// This must be called once during application startup to activate
// the chat/room/* Swamp pattern across all rooms.
//
// ✅ Example Swamp:
// - Room ID: "room-abc123"
// - Swamp:   chat/room/room-abc123
//
// After deletion of the last message:
// - The Swamp folder is automatically removed from disk
// - No need to re-register; pattern remains valid for reuse
func (c *CatalogModelChatMessage) RegisterPattern(repo repo.Repo) error {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := repo.GetHydraidego()

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
		SwampPattern: name.New().
			Sanctuary("chat").
			Realm("room").Swamp("*"),

		// Keep each session Swamp in memory for 30 minutes after last use
		CloseAfterIdle: time.Minute * 30,

		// Disk-backed storage for persistence
		IsInMemorySwamp: false,

		// Write small chunks frequently to reduce data loss and latency
		FilesystemSettings: &hydraidego.SwampFilesystemSettings{
			WriteInterval: time.Second * 10, // flush every 10s
			MaxFileSize:   8192,             // 8KB chunk size
		},
	})

	if errorResponses != nil {
		return hydraidehelper.ConcatErrors(errorResponses)
	}
	return nil
}

func (c *CatalogModelChatMessage) getSwampName(roomID string) name.Name {
	return name.New().
		Sanctuary("chat").
		Realm("room").
		Swamp(roomID)
}
