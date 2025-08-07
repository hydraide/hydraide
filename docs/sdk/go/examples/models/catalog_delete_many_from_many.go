package models

import (
	"log/slog"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// CatalogModelPostMention represents a mention of a user under a specific post.
//
// ✅ Swamp model:
// Each post has its own Swamp → mentions/posts/{postID}
// Inside the Swamp, each key is a userID that was mentioned in that post.
// The value may contain optional context (e.g., message snippet).
//
// ✅ Example Swamps:
// - mentions/posts/post-123 → key: user-999
// - mentions/posts/post-456 → key: user-999
//
// 🧠 What does this model demonstrate?
//
// This model is designed to show how you can use HydrAIDE’s distributed
// deletion engine (`CatalogDeleteManyFromMany`) to **remove a specific user**
// from multiple Swamps (posts) **in a single, efficient operation**.
//
// 🔥 Why CatalogDeleteManyFromMany matters:
//
// In a real-world system, users may be mentioned in **many different posts**,
// and those posts (Swamps) may live on **different Hydra servers**.
//
// HydrAIDE’s `CatalogDeleteManyFromMany` automatically:
// - Resolves which Swamp is hosted on which server
// - Groups deletion requests per host to minimize roundtrips
// - Performs all deletions in parallel, distributed across the cluster
// - Calls an iterator callback per key to handle result status
//
// This avoids unnecessary network hops, coordination, and complexity.
//
// 🧹 Example use case:
//
// Suppose a user deletes their account. You want to remove all mentions
// of that user from every post where they were tagged.
//
// To do that:
//
// 1. In the user's profile, store the following structure:
//
//	PostMentions map[string]interface{} // e.g., {"post-123": true, "post-456": true}
//
// 2. When deleting the user, extract the post IDs from PostMentions
//
// 3. Call DeleteMentionsFromAllPosts(), which will:
//
//   - Build delete requests for Swamps like:
//     mentions/posts/post-123 → key: userID
//     mentions/posts/post-456 → key: userID
//
//   - Execute them with CatalogDeleteManyFromMany()
//
//   - Log outcomes per key via iterator
//
// 🔔 Delete Events:
//
// Every successful key deletion triggers a `CatalogDeleted` event.
// All subscribers of the affected Swamp (e.g. the post UI) will receive these events automatically.
//
// This ensures:
// - Real-time update of the post’s UI (e.g. mention removed live)
// - Propagation across tabs, devices, or collaborating clients
//
// 🧠 Summary:
//
// This model demonstrates how to:
// - Scale deletion of a single user’s footprint across many content-based Swamps
// - Keep post indexes clean
// - Maintain real-time sync via events
type CatalogModelPostMention struct {
	UserID    string    `hydraide:"key"`       // ID of the mentioned user
	Context   string    `hydraide:"value"`     // Optional snippet or message preview
	CreatedBy string    `hydraide:"createdBy"` // Who mentioned the user
	CreatedAt time.Time `hydraide:"createdAt"` // When the mention occurred
}

// DeleteMentionsFromAllPosts removes the given userID from all post mention Swamps.
//
// This uses HydrAIDE’s distributed CatalogDeleteManyFromMany() to efficiently delete
// the same user key from multiple Swamps — each corresponding to a post.
//
// 🔁 This is useful when:
// - A user is deleted and must be removed from all mentions
// - A privacy request or moderation action requires full removal
//
// 💡 Example Swamps:
// - mentions/posts/post-123 → key = user-999
// - mentions/posts/post-456 → key = user-999
//
// 📘 Triggers CatalogDeleted events for each affected Swamp if subscribers exist.
func (c *CatalogModelPostMention) DeleteMentionsFromAllPosts(r repo.Repo, userID string, postIDs []string) error {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := r.GetHydraidego()

	// Build delete requests: one per post, always same key
	var reqs []*hydraidego.CatalogDeleteManyFromManyRequest
	for _, postID := range postIDs {
		reqs = append(reqs, &hydraidego.CatalogDeleteManyFromManyRequest{
			SwampName: c.createCatalogName(postID),
			Keys:      []string{userID},
		})
	}

	// Run distributed deletion and log each outcome
	err := h.CatalogDeleteManyFromMany(ctx, reqs, func(key string, err error) error {
		switch {
		case err == nil:
			slog.Info("Mention deleted", "userID", key)
		case hydraidego.IsNotFound(err):
			slog.Warn("Mention not found", "userID", key)
		case hydraidego.IsSwampNotFound(err):
			slog.Warn("Post Swamp not found")
		default:
			slog.Error("Failed to delete mention", "userID", key, "error", err)
		}
		return nil
	})

	if err != nil {
		slog.Error("Distributed mention deletion failed", "userID", userID, "error", err)
	}
	return err
}

// RegisterPattern registers the Swamp pattern used for storing user mentions under posts.
//
// ✅ Swamp pattern: mentions/posts/*
//
//	→ Each post has its own Swamp (e.g., mentions/posts/post-123)
//
// This uses a **wildcard-based pattern registration**, which ensures that:
//
// - All post-based Swamps share the same configuration (storage, memory, flush policy)
// - You do NOT need to register each Swamp (post) individually
// - Any new Swamp matching this pattern will automatically inherit this behavior
//
// 📦 Storage settings applied:
// - Disk-backed persistence (not in-memory only)
// - Kept hot in memory for 5 minutes after last access
// - Written to disk every 10 seconds in 8 KB chunks
//
// 💡 This approach is ideal when you have many Swamps following the same structural rule
// (e.g. one Swamp per post, one Swamp per room, etc.) and want to enforce a unified policy.
//
// ⚠️ This must be called once during app startup to activate the pattern globally.
func (c *CatalogModelPostMention) RegisterPattern(repo repo.Repo) error {
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
		// Register the exact Swamp: quotes/catalog/main
		SwampPattern: name.New().Sanctuary("mentions").Realm("posts").Swamp("*"),

		// Keep the Swamp warm fopr 5 minutes
		CloseAfterIdle: time.Second * 300, // 5 minutes

		// Use persistent storage with disk-backed flush
		IsInMemorySwamp: false,

		// Set up frequent disk flushing for low-latency persistence
		FilesystemSettings: &hydraidego.SwampFilesystemSettings{
			WriteInterval: time.Second * 10, // flush every 10 seconds
			MaxFileSize:   8192,             // 8 KB file chunk size
		},
	})

	if errorResponses != nil {
		return hydraidehelper.ConcatErrors(errorResponses)
	}

	return nil
}

// createCatalogName generates the full Swamp name for storing mentions under a specific post.
//
// ✅ Swamp structure:
// - Sanctuary: "mentions"
// - Realm:     "posts"
// - Swamp:     {postID} (dynamic per post)
//
// This ensures that each post gets its own dedicated Swamp for mention records.
// Use this method to avoid hardcoded strings and ensure consistent naming.
func (c *CatalogModelPostMention) createCatalogName(postID string) name.Name {
	return name.New().Sanctuary("mentions").
		Realm("posts").
		Swamp(postID)
}
