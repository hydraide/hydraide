package models

import (
	"context"
	"log/slog"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// ModelCatalogMessages demonstrates a simple socket-like message system powered by HydrAIDE.
//
// This model is ideal for scenarios where multiple services need to publish messages
// to a central Swamp (data vault), and one or more clients — such as WebSocket frontends —
// need to receive those messages in real time.
//
// It can also be used for microservice-to-microservice communication,
// where services emit and listen to domain events via HydrAIDE.
//
// In our monorepo architecture, models like this are shared across services.
// Each model is either "thin" (self-contained) or wrapped by a service layer
// for more advanced business logic.
//
// This design makes the model reusable across services, without requiring each one
// to implement custom socket or broker logic. HydrAIDE becomes the central message broker,
// automatically handling persistence (if needed), subscriptions, delivery, and cleanup.
//
// ▶️ This example specifically demonstrates how to use the **Subscription** API of HydrAIDE,
// including live data streaming, event filtering, and optional message cleanup.
//
// See the `Subscribe()` method for a complete, idiomatic implementation.
type ModelCatalogMessages struct {
	// Unique message ID
	MessageID string `hydraide:"key"`

	// Message payload in JSON format.
	// It should already be a serialized JSON string, so no need to marshal it again.
	Message string `hydraide:"value"`

	// Timestamp when the message was created
	CreatedAt time.Time `hydraide:"createdAt"`

	// Optional expiration timestamp.
	// Once this is passed, the message can be deleted from the Swamp automatically.
	ExpireAt time.Time `hydraide:"expireAt"`
}

// Save stores a new message in the HydrAIDE database.
// Other microservices that subscribe to this Swamp will receive the message instantly.
func (m *ModelCatalogMessages) Save(r repo.Repo) error {
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	h := r.GetHydraidego()
	_, err := h.CatalogSave(ctx, m.getName(), m)
	return err
}

// Subscribe connects to the Swamp and listens for new messages in real time.
// Callback is invoked only when a new message arrives.
// The subscription ends when the provided context is cancelled.
// This is useful for live dashboards, inter-service events, or pushing updates to UIs.
//
// If getExistingData=true, HydrAIDE will also deliver already-existing Treasures
// in the Swamp as if they were new – useful when you want both history and live updates.
//
// ⚠️ Note: You must pass a **non-pointer** empty struct of the same type to receive events.
func (m *ModelCatalogMessages) Subscribe(ctx context.Context, r repo.Repo, callbackFunc func(m *ModelCatalogMessages) error) error {

	h := r.GetHydraidego()

	// Subscription is a non-blocking operation.
	// The callback function will only be invoked when a new message arrives.

	// If `getExistingData` is set to true, HydrAIDE will deliver all existing Treasures
	// currently stored in the Swamp as if they were new events — before it starts streaming new ones.
	//
	// This is useful when you expect that some messages might already exist,
	// and want to process them immediately, without performing a separate Read() query.
	// It gives you both historical and live messages in a single call.

	// ⚠️ Important: The model type passed to Subscribe (e.g. ModelCatalogMessages{}) must be:
	// - a non-pointer struct (not *ModelCatalogMessages)
	// - matching the actual type stored in the Swamp
	// HydrAIDE will use this as the target type for decoding event payloads.

	// The provided callback function is invoked whenever a new message (or "Treasure") is received.
	// This function should contain your event processing logic.
	err := h.Subscribe(ctx, m.getName(), false, ModelCatalogMessages{}, func(model any, eventStatus hydraidego.EventStatus, err error) error {

		// This is where we handle messages received via the event stream.

		// Each message comes with an associated event status, which indicates
		// the type of change that triggered the event.
		// You can use this status to determine how to process each message.

		// Possible values of `eventStatus`:
		// - StatusNew:            A new Treasure was added to the Swamp.
		// - StatusModified:       An existing Treasure was updated.
		// - StatusNothingChanged: The Treasure was re-broadcasted without changes (e.g. on hydration).
		// - StatusDeleted:        The Treasure was deleted from the Swamp.

		// Based on this, you can apply different logic — e.g. render, patch, ignore, or cleanup

		// Cast the received model to the expected type.
		message := model.(*ModelCatalogMessages)
		if err != nil {
			slog.Error("Error in subscription callback function", "err", err)
			return err
		}

		// write the received message to the log for debugging purposes
		slog.Info("Message received",
			"eventStatus", eventStatus,
			"model", model,
		)

		// in this example, if the message is new, we call the callback function
		if eventStatus == hydraidego.StatusNew {

			e := callbackFunc(message)
			if e != nil {
				slog.Error("Error in callbackFunc", "err", e)
				return e
			}

			// Optionally, you can delete the message from the Swamp immediately after calling `callbackFunc(message)`.
			// Doing so will trigger a new event (StatusDeleted), which will also be delivered to this subscription.

			// However, since this handler only processes events with StatusNew,
			// the delete event will be ignored — no further action will be taken.
			//
			// This is useful when you want to implement auto-cleanup after successful message processing.

			return nil
		}

		return nil

	})

	// return with error if subscription failed
	if err != nil {
		slog.Error("Error in subscribe", "err", err)
		return err
	}

	// Ha minden rendben volt, akkor nil-t adunk vissza
	return nil

}

// Destroy completely removes the entire Swamp that contains all messages of this type.
//
// ⚠️ This operation deletes every Treasure in the Swamp and the Swamp itself,
// including in-memory and persisted data (if any).
//
// It is primarily intended for testing or development purposes —
// such as resetting state between test runs or clearing all messages before starting a new session.
//
// Note: Once destroyed, the Swamp must be re-registered before reuse.
func (m *ModelCatalogMessages) Destroy(r repo.Repo) error {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := r.GetHydraidego()

	return h.Destroy(ctx, m.getName())
}

// RegisterPattern registers the Swamp pattern for this model with the HydrAIDE engine.
//
// In this example, we are registering an **in-memory Swamp**:
// - Data lives only in RAM and is never written to disk.
// - Ideal for pub/sub scenarios where messages are ephemeral.
// - Subscriptions still work exactly the same as with disk-backed Swamps.
//
// The `CloseAfterIdle` setting ensures the Swamp stays alive for 24 hours after last use,
// which prevents automatic unloading even if it's not actively accessed.
//
// 🧠 Tip: For more examples of how Swamps can be configured — including disk persistence,
// chunk sizes, TTLs, and compression — see the `basics_register_swamp.go` reference example.
func (m *ModelCatalogMessages) RegisterPattern(r repo.Repo) error {

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
		SwampPattern:    m.getName(),
		CloseAfterIdle:  time.Second * 86400, // Keep alive for 1 day even if idle
		IsInMemorySwamp: true,                // Do not persist to disk; purely volatile
	})

	if errorResponses != nil {
		return hydraidehelper.ConcatErrors(errorResponses)
	}
	return nil
}

// getName returns the fully qualified Swamp name for this model,
// using the HydrAIDE hierarchical naming convention:
//
// - Sanctuary: "socketService" → logical grouping of services
// - Realm:     "catalog"       → specific domain within the service
// - Swamp:     "messages"      → the exact dataset / message vault
//
// This name uniquely identifies where messages of this model are stored.
// It also determines folder mapping and event routing within HydrAIDE.
//
// 📛 Naming is a core design element in HydrAIDE — it drives storage, access, subscriptions,
// and distribution. Think of this as both the "path" and "identity" of your data.
func (m *ModelCatalogMessages) getName() name.Name {
	return name.New().Sanctuary("socketService").Realm("catalog").Swamp("messages")
}
