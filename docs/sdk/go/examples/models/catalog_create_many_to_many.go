package models

import (
	"log/slog"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// CatalogModelTagReference represents a reference between a tag and a document in HydrAIDE.
//
// Each instance of this model is stored in a separate Swamp named after a tag
// (e.g. `tags/references/ai`, `tags/references/news`, etc.), and its purpose is
// to establish a lightweight many-to-many link between tags and document identifiers.
//
// 🧠 Concept:
//
//   - A **tag** is a logical grouping mechanism (similar to a label or keyword).
//     It defines a Swamp that holds all document IDs associated with that tag.
//
//   - A **document** can belong to multiple tags → so the same document ID will be
//     inserted into multiple Swamps (one per tag).
//
//   - A **tag Swamp** contains many documents → each as a separate Treasure.
//
//   - Only the `hydraide:"key"` is used in this model — no payload or metadata is stored.
//     This makes the operation memory- and storage-efficient.
//
// 🧪 Example:
//
//	Tag "ai" → Swamp: `tags/references/ai`
//	Tag "startup" → Swamp: `tags/references/startup`
//
//	Document "doc-123" can be inserted into both Swamps,
//	meaning it belongs to both tags.
//
// 🚀 Performance & Architecture:
//
//	This example showcases the power of HydrAIDE’s `CatalogCreateManyToMany()` API,
//	which enables **batch insertion into multiple Swamps across multiple servers**
//	in a **single high-efficiency call**.
//
//	Instead of issuing one request per tag or document, HydrAIDE automatically:
//	  - Groups insert operations by destination server
//	  - Generates one gRPC `SetRequest` per server (not per Swamp)
//	  - Handles Swamp creation if it does not exist
//	  - Ensures idempotency (no overwrite)
//
//	Despite the complexity, you can still receive **fine-grained per-record feedback**
//	via an iterator function, which reports success or error (e.g. already exists)
//	for every document-key and Swamp combination.
//
// ✅ Why this model matters:
//
//   - Enables fast, tag-based reverse lookup (e.g. “which docs belong to #ai?”)
//   - Avoids heavy joins or complex indexes — the Swamp itself acts as the index
//   - Works well in distributed architectures with minimal network overhead
//   - Highly scalable even when tagging thousands of documents with hundreds of tags
type CatalogModelTagReference struct {
	DocumentID string `hydraide:"key"` // Unique identifier of the document
}

// CreateManyToManyByTags demonstrates how to insert a document into multiple tag Swamps.
//
// This function uses CatalogCreateManyToMany to batch-insert the document ID into multiple
// Swamps named after the provided tags. Each Swamp corresponds to a tag and contains the
// document ID if it’s not already present.
//
// ✅ This avoids unnecessary overwrites and sends data in a single SetRequest per server.
func (c *CatalogModelTagReference) CreateManyToManyByTags(r repo.Repo, documentID string, tags []string) {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := r.GetHydraidego()

	var requests []*hydraidego.CatalogManyToManyRequest

	// 🔁 Build request list: one Swamp per tag
	// For each tag, we create a separate request where the Swamp name is derived
	// from the tag name (e.g. "tags/references/ai") and the document ID is the only model.
	for _, tag := range tags {
		req := &hydraidego.CatalogManyToManyRequest{
			SwampName: c.createSwampForTag(tag), // dynamically generate Swamp name for each tag
			Models: []any{
				&CatalogModelTagReference{
					DocumentID: documentID, // this is the only field we save per tag
				},
			},
		}
		requests = append(requests, req) // accumulate all requests into a single list
	}

	// 🚀 Perform bulk tagging operation using HydrAIDE’s CatalogCreateManyToMany
	// This sends a single SetRequest per server, batching Swamps and entries together.
	// We also pass an inline iterator function to receive feedback per inserted item.
	err := h.CatalogCreateManyToMany(ctx, requests, func(swamp name.Name, key string, err error) error {

		// 🔁 Called once per key/tag combination. Swamp = tag Swamp, key = DocumentID.

		if err != nil {
			if hydraidego.IsAlreadyExists(err) {
				// ℹ️ The document was already present in this tag — no need to insert again.
				slog.Warn("Document already tagged",
					"documentID", key,
					"swamp", swamp.Get(), // e.g. tags/references/startup
				)
			} else {
				// ❌ An unexpected error occurred during insert (validation, network, etc.)
				slog.Error("Failed to tag document",
					"documentID", key,
					"swamp", swamp.Get(),
					"error", err,
				)
			}
		} else {
			// ✅ Document was successfully tagged under this Swamp (tag)
			slog.Info("Document successfully tagged",
				"documentID", key,
				"swamp", swamp.Get(),
			)
		}
		return nil // continue processing the rest of the entries
	})

	// 🔥 If the entire operation fails (e.g. network issue, invalid request structure),
	// the error is returned here and should be logged.
	if err != nil {
		slog.Error("Bulk tagging operation failed", "error", err)
	}

}

// RegisterPattern registers the Swamp pattern for all tag-based reference catalogs.
//
// ⚠️ This must be called once during system startup, before any insert or query happens.
//
// This setup ensures that all Swamps under the pattern `tags/references/*`
// are created with the same configuration and performance characteristics.
//
// ✅ Why the wildcard matters:
// The wildcard Swamp pattern (`*`) means:
//
//	→ "Apply this configuration to all Swamps under this namespace"
//
// For example:
//   - `tags/references/ai`
//   - `tags/references/news`
//   - `tags/references/startup`
//   - `tags/references/nlp`
//
// All of these Swamps will inherit the same rules (memory handling, flush timing, max file size),
// even if they are created dynamically at runtime.
//
// This is especially useful for many-to-many catalog models where the number of Swamps
// may grow based on user input, tags, domains, regions, or other dynamic keys.
func (c *CatalogModelTagReference) RegisterPattern(repo repo.Repo) error {
	h := repo.GetHydraidego()

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
		// 🧠 Wildcard pattern ensures all tag-based Swamps (e.g. tags/references/ai) share the same configuration.
		SwampPattern: name.New().Sanctuary("tags").Realm("references").Swamp("*"),

		// 🕒 Keep each Swamp in memory for 5 minutes after last usage.
		// This balances responsiveness and memory efficiency for tag-based access patterns.
		CloseAfterIdle: time.Second * 60 * 5, // 5 minutes

		// 💾 Use persistent storage (not in-memory only)
		IsInMemorySwamp: false,

		// ⚙️ File flush settings: write small changes often to disk
		FilesystemSettings: &hydraidego.SwampFilesystemSettings{
			WriteInterval: time.Second * 10, // flush every 10 seconds
			MaxFileSize:   8192,             // max chunk size = 8 KB
		},
	})

	if errorResponses != nil {
		return hydraidehelper.ConcatErrors(errorResponses)
	}

	return nil
}

// createSwampForTag returns a unique Swamp name based on the tag.
// For example: `tags/references/ai`
func (c *CatalogModelTagReference) createSwampForTag(tag string) name.Name {
	return name.New().Sanctuary("tags").Realm("references").Swamp(tag)
}
