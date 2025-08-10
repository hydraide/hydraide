package models

import (
	"fmt"
	"hash/fnv"
	"log/slog"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// ModelTagProductViewers demonstrates HydrAIDE’s native reverse index capabilities
// using `uint32` slice-based Treasures within tag-based Swamps.
//
// 🧠 Goal of this example:
//
// This demo shows how you can use HydrAIDE’s special Uint32Slice functions to build a
// **reverse lookup structure** — where each tag (`<tag>`) maps to a Swamp like:
//
//	tags/products/<tag>
//
// Inside that Swamp, each `productID` is a key, and the value is a `[]uint32` list of user IDs
// who interacted with (e.g. viewed or ordered) the product under that tag.
//
// Instead of saving full user objects or large JSON payloads, this pattern enables
// **compact, high-speed indexing** — ideal for analytics, personalization, or behavioral modeling.
//
// 🚀 Why this is useful:
//
// This pattern is perfect for use cases where:
//   - You need to track **many values** (e.g. user IDs, session IDs) under a single key
//   - You want **deduplicated**, append-only behavior with **garbage collection built-in**
//   - You care about memory efficiency, fast access, and real-time usage
//
// ✅ Example real-world use cases:
//
//   - Recommender systems (what users interacted with what tags or products)
//   - Tag-based statistics (how many users engaged with each tag/product)
//   - Slice-based behavioral modeling (which products triggered what reactions)
//
// 🛠️ What’s included in this example:
//
// This model provides a full production-ready implementation with all required operations:
//
//   - `PushViewersToTag()`       → add new viewers (Uint32SlicePush)
//   - `DeleteViewersFromTags()`  → remove viewers (Uint32SliceDelete)
//   - `GetSliceSize()`           → count viewers for a product
//   - `IsValueExist()`           → check if a user is already listed
//   - `RegisterPattern()`        → register the Swamp in memory or persistent mode
//
// Each function is explained in its own documentation block below.
//
// ✨ Bonus:
// Tag names are auto-hashed using `fnv32a`, ensuring deterministic and safe Swamp naming
// even when tag strings include special characters or non-ASCII input.
//
// 📦 Catalog-style loading:
//
// Although this model demonstrates Uint32 slice logic, the Swamp behaves like any other
// **Catalog** in HydrAIDE. This means:
//
//   - You can retrieve a specific Treasure by key (e.g., product ID)
//   - Or you can iterate over the entire Swamp to analyze all records under a given tag
//
// This makes it suitable for use cases that need full snapshot reads or fine-grained access.
//
// 🔄 Atomic mutation without fetch:
//
// The main power of this model is that you can **atomically mutate the Treasure value**
// (i.e., the `[]uint32` slice) without having to:
//
//   - Fetch the full slice from HydrAIDE
//   - Modify it in memory
//   - Push it back via a full update
//
// Instead, operations like `PushViewersToTag()` or `DeleteViewersFromTags()` run **entirely server-side**:
// they append or remove values in-place, ensuring atomicity, deduplication, and garbage collection.
//
// This results in:
//   - Lower latency
//   - No race conditions
//   - Safer concurrent updates
//
// 🧠 In essence:
// You interact declaratively with HydrAIDE — describe what should change, and it handles the rest.
type ModelTagProductViewers struct {
	ProductID string   `hydraide:"key"` // e.g. "product-123"
	UserIDs   []uint32 // e.g. []uint32{101, 102, 103}
}

// PushViewersToTag appends user IDs to the uint32 slice assigned to a given product inside a tag-specific Swamp.
//
// This function performs an **atomic, in-place update** of the slice value stored under the specified product ID,
// inside the Swamp `tags/products/<tag>`. Each tag corresponds to its own Swamp, and each product ID
// acts as a unique Treasure key within that Swamp.
//
// ✅ Key features:
//
//   - If the Swamp or Treasure doesn’t exist yet, it will be automatically created.
//   - If a user ID is already present in the slice, it will NOT be added again.
//   - Only **new, unique** values are appended — preserving slice integrity without duplication.
//
// 🧠 Why use this?
//
// Unlike traditional systems that require you to:
//  1. Load the current slice (e.g. `[]uint32`)
//  2. Append manually
//  3. Save the updated record back
//
// HydrAIDE lets you perform this entire mutation **server-side**, atomically, without ever fetching the full slice.
//
// This means:
//   - No race conditions
//   - Better performance (no round-trips)
//   - Cleaner business logic
//
// 🧪 Example:
//
//	viewer := &ModelTagProductViewers{
//	    ProductID: "product-123",
//	    UserIDs:   []uint32{101, 102},
//	}
//
//	err := viewer.PushViewersToTag(repo, "black-friday")
//	// Adds users 101 and 102 to the slice for product-123 under "black-friday" tag
//
// ⚠️ Note:
// Values are deduplicated **per operation**, but not globally across different tags or products.
// Each Swamp is isolated by its tag name.
func (m *ModelTagProductViewers) PushViewersToTag(r repo.Repo, tagName string) error {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := r.GetHydraidego()

	return h.Uint32SlicePush(ctx, m.createSwampName(tagName), []*hydraidego.KeyValuesPair{
		{
			Key:    m.ProductID,
			Values: m.UserIDs,
		},
	})
}

// DeleteViewersFromTags removes one or more user IDs from uint32 slice-type Treasures inside a tag-specific Swamp.
//
// Each element in the `kvPairs` list defines a product ID (as the key) and a list of user IDs to remove from
// the associated slice. The operation runs against a Swamp with the name `tags/products/<tag>`.
//
// ✅ Key behaviors:
//
//   - If the target Treasure (product ID) doesn’t exist, the operation is a **no-op** — no error is returned.
//   - If the slice exists but the specified user IDs are not present, they are simply ignored.
//   - If a slice becomes empty after removal, the **Treasure is automatically deleted**.
//   - If all Treasures in the Swamp are deleted, the **entire Swamp is garbage collected**.
//
// 🔄 Atomic & idempotent by design:
//
// HydrAIDE executes this deletion entirely server-side. You don’t need to:
//   - Load the current slice
//   - Filter it manually
//   - Save it back
//
// Instead, the mutation is performed atomically per key, eliminating race conditions and improving performance.
//
// 🧪 Example:
//
//	err := viewerModel.DeleteViewersFromTags(repo, "autumn-sale", []*ModelTagProductViewers{
//	    {ProductID: "product-123", UserIDs: []uint32{1001, 1002}},
//	    {ProductID: "product-999", UserIDs: []uint32{1003}},
//	})
//
//	// Removes 1001 and 1002 from product-123 slice (if present)
//	// If any slice becomes empty → it is deleted
//	// If the Swamp ends up empty → the Swamp is deleted
//
// 🧠 Tip:
// This is a great way to enforce lifecycle hygiene for reverse indexes — only active relationships remain in memory.
func (m *ModelTagProductViewers) DeleteViewersFromTags(r repo.Repo, tagName string, kvPairs []*ModelTagProductViewers) error {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := r.GetHydraidego()

	// Build input KeyValuesPair list
	pairs := make([]*hydraidego.KeyValuesPair, 0, len(kvPairs))
	for _, entry := range kvPairs {
		pairs = append(pairs, &hydraidego.KeyValuesPair{
			Key:    entry.ProductID,
			Values: entry.UserIDs,
		})
	}

	// Perform delete operation
	return h.Uint32SliceDelete(ctx, m.createSwampName(tagName), pairs)
}

// GetSliceSize returns the number of user IDs stored in the uint32 slice for a specific product ID,
// within a tag-specific Swamp (e.g., `tags/products/<tag>`).
//
// This read-only operation is useful for diagnostics, analytics, or sanity checks,
// especially when you want to verify whether a product has active associations
// under a given tag.
//
// ✅ Key behaviors:
//
//   - If the key (product ID) does not exist, an error is returned (`ErrCodeInvalidArgument`).
//   - If the key exists but is not of type `[]uint32`, an error is returned (`ErrCodeFailedPrecondition`).
//   - Otherwise, returns the **exact number of values** in the slice (i.e., viewer count).
//
// 🔄 Use cases:
//
//   - Determine how many users interacted with a product in a specific tag context
//   - Check whether a slice is empty before triggering deletion logic
//   - Visualize user engagement levels across products or tags
//
// 🧪 Example:
//
//	size, err := viewerModel.GetSliceSize(repo, "flash-sale", "product-123")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Product-123 has %d viewers in 'flash-sale' tag.\n", size)
//
// 🧠 Tip:
// This method does **not** load the slice contents — only its size. It’s highly optimized for fast lookups.
func (m *ModelTagProductViewers) GetSliceSize(r repo.Repo, tagName string, key string) (int64, error) {

	// Create a context with timeout.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Get HydrAIDE client
	h := r.GetHydraidego()
	// Get the size of the slice for the given key in the specified tag Swamp
	return h.Uint32SliceSize(ctx, m.createSwampName(tagName), key)

}

// IsValueExist checks whether a specific user ID exists inside the uint32 slice
// associated with a product ID in a tag-specific Swamp (e.g., `tags/products/<tag>`).
//
// This is a lightweight, read-only operation optimized for fast membership testing
// within slice-type Treasures. It does **not** load or return the full slice — only
// answers whether a given value is present.
//
// ✅ Key behaviors:
//
//   - Returns `true` if the value exists in the slice under the specified key.
//   - Returns `false` if the value is not found.
//   - Returns an error if:
//   - The key does not exist (`ErrCodeInvalidArgument`)
//   - The Treasure is not of type `[]uint32` (`ErrCodeFailedPrecondition`)
//
// 🔄 Use cases:
//
//   - Validate whether a user has already been recorded for a product-tag combination
//   - Prevent redundant writes before calling `PushViewersToTag()`
//   - Use as guard logic in indexing or behavioral flows
//
// 🧪 Example:
//
//	exists, err := viewerModel.IsValueExist(repo, "summer-sale", "product-123", 101)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if exists {
//	    fmt.Println("User already listed.")
//	} else {
//	    fmt.Println("Needs indexing.")
//	}
//
// 🧠 Tip:
// Unlike `PushViewersToTag()`, this method is purely diagnostic — it performs no mutation.
// It’s ideal for conditional logic or analytics.
func (m *ModelTagProductViewers) IsValueExist(r repo.Repo, tagName string, key string, value uint32) (bool, error) {

	// Create a context with timeout.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Get HydrAIDE client
	h := r.GetHydraidego()

	return h.Uint32SliceIsValueExist(ctx, m.createSwampName(tagName), key, value)

}

// RegisterPattern declares the slice-type Swamp used for a specific tag,
// enabling HydrAIDE to prepare a container for storing product → []userID mappings.
//
// Each tag gets its own Swamp with the pattern: `tags/products/<tag>`.
// This model is ideal for reverse index patterns, such as recommendation tracking,
// behavioral analysis, or tag-driven interaction logs.
//
// ✅ Configuration behavior:
//
//   - **CloseAfterIdle**: Swamp will automatically unload from memory after 5 seconds of inactivity.
//   - **IsInMemorySwamp**: Set to `false`, meaning the Swamp is persisted to disk.
//   - **FilesystemSettings**:
//   - Changes are flushed every 10 seconds.
//   - Each chunk file is limited to 8 KB.
//
// 🧠 Notes:
//
//   - If the Swamp already exists, this operation is idempotent — no changes occur.
//   - You should call this before any usage of `PushViewersToTag()` or other mutation functions.
//   - Tag names are auto-hashed in `createSwampName()` for compatibility and uniformity.
//
// 🧪 Example:
//
//	err := viewerModel.RegisterPattern(repo, "black-friday")
//	if err != nil {
//	    log.Fatal(err)
//	}
func (m *ModelTagProductViewers) RegisterPattern(r repo.Repo, tagName string) error {

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

		// The Swamp pattern name: tags/products/all
		SwampPattern: m.createSwampName(tagName),

		// Keep Swamp in memory for 5 sec of idle time
		CloseAfterIdle: time.Second * 5,

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

// createSwampName returns a deterministic Swamp name for the tag-based viewer slice map.
func (m *ModelTagProductViewers) createSwampName(tagName string) name.Name {
	return name.New().Sanctuary("tags").Realm("products").Swamp(m.hashTagName(tagName))
}

// hashTagName returns a deterministic 32-bit hexadecimal hash of the given tag string,
// using the FNV-1a algorithm.
//
// This function is used to generate a **safe and uniform Swamp name** based on a tag,
// regardless of whether the original tag contains special characters, whitespace,
// non-ASCII input, or very long strings.
//
// ✅ Why use hashing?
//
//   - Ensures consistent Swamp naming across environments
//   - Avoids invalid folder names (e.g., spaces, accents, slashes)
//   - Helps prevent excessively long or malformed Swamp paths
//   - Uniform distribution improves file system balance
//
// 🧠 Algorithm:
//
//   - Uses `FNV-1a` 32-bit non-cryptographic hash
//   - Output is a lowercase hex string, e.g. `"a3d93bcf"`
//   - Same input always yields same output (deterministic)
//
// ⚠️ Note:
//
//   - This is not meant for cryptographic or security purposes — it's for internal Swamp routing.
//   - Collisions are **very unlikely** at 32-bit scale for normal tag usage, but theoretically possible.
//   - If hashing fails (should be rare), an error is logged and an empty string is returned —
//     Swamp creation will then fail gracefully.
//
// 🧪 Example:
//
//	tag := "🔥 Black Friday 2025! 💥"
//	hashed := viewerModel.hashTagName(tag)
//	// → "3f6a5c12"
func (m *ModelTagProductViewers) hashTagName(tag string) string {
	h := fnv.New32a()
	_, err := h.Write([]byte(tag))
	if err != nil {
		// If hashing fails, return a default value to avoid panics
		slog.Error("Failed to hash tag name:", err)
		return ""
	}
	return fmt.Sprintf("%x", h.Sum32()) // pl: "a3d93bcf"
}
