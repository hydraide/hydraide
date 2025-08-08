package models

import (
	"log/slog"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// CatalogModelQuote represents a typed record (Treasure) inside a HydrAIDE Catalog Swamp,
// where each quote is uniquely identified and enriched with metadata.
//
// This model demonstrates how to use `CatalogRead()` to quickly fetch a single entry
// by key and populate a Go struct using HydrAIDE's field tags.
//
// ✅ Purpose:
// Use this model when you want to perform **high-speed, single-key lookups**
// from a catalog — e.g., detail views, edit workflows, or existence checks.
//
// 🧠 How CatalogRead works:
//
//   - You must first provide a struct with a valid `hydraide:"key"` field populated
//     (in this case: `QuoteID`).
//
//   - When `CatalogRead()` is called, HydrAIDE looks up that key in the specified Swamp.
//     If the record exists, all the matching fields in the struct will be populated.
//
//   - If the key does **not exist**, no mutation happens and an `ErrCodeNotFound` is returned.
//     So your struct remains unchanged if no data is found.
//
// 📦 Included fields in this example:
//
//   - `QuoteID`   → required key (for lookup)
//
//   - `QuoteText` → value (actual quote content)
//
//   - `CreatedBy`, `CreatedAt`, `UpdatedBy`, `UpdatedAt` → optional metadata fields
//
//     These metadata fields are filled **automatically** by the HydrAIDE engine (if present in the record)
//
// 🔧 Usage example:
//
//	q := &CatalogModelQuote{QuoteID: "quote-42"}
//	err := q.Load(repo)
//
//	if err != nil {
//	    log.Fatal(err) // or check for IsNotFound()
//	}
//
//	fmt.Println("Quote:", q.QuoteText, "by", q.CreatedBy)
//
// 💡 Tip:
// The `Load()` method in this model wraps the raw `CatalogRead()` call with logging and error handling.
// You can copy this pattern for other models to create reusable read logic.
type CatalogModelQuote struct {
	QuoteID   string    `hydraide:"key"`       // Unique ID of the quote
	QuoteText string    `hydraide:"value"`     // The actual quote content
	CreatedBy string    `hydraide:"createdBy"` // Who added the quote
	CreatedAt time.Time `hydraide:"createdAt"` // When it was added
	UpdatedBy string    `hydraide:"updatedBy"` // Who last edited it
	UpdatedAt time.Time `hydraide:"updatedAt"` // When it was last updated
}

// Load loads a single quote from the HydrAIDE catalog using its QuoteID.
//
// This demonstrates how to use CatalogRead to retrieve a typed entry with value and metadata.
//
// 🧠 Use case:
// - You need to display quote details in a detail view
// - You want to check if a quote exists before updating it
// - You want to inspect who added or last edited the quote
func (c *CatalogModelQuote) Load(r repo.Repo) error {

	// Create a context with a default timeout using the helper.
	// This ensures the request is cancelled if it takes too long,
	// preventing hangs or leaking resources.
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// Retrieve the HydrAIDE SDK instance from the repository.
	h := r.GetHydraidego()

	quote := &CatalogModelQuote{}

	// Attempt to load the quote by its key from the Swamp `quotes/catalog/main`
	err := h.CatalogRead(ctx, c.createCatalogName(), c.QuoteID, c)
	if err != nil {
		if hydraidego.IsNotFound(err) {
			slog.Warn("Quote not found", "quoteID", c.QuoteID)
			return nil
		}
		slog.Error("Failed to load quote", "quoteID", c.QuoteID, "error", err)
		return err
	}

	slog.Info("Quote loaded",
		"quoteID", quote.QuoteID,
		"text", quote.QuoteText,
		"createdBy", quote.CreatedBy,
		"createdAt", quote.CreatedAt,
		"updatedBy", quote.UpdatedBy,
		"updatedAt", quote.UpdatedAt,
	)

	return nil

}

// RegisterPattern registers the Swamp used to store quotes in the HydrAIDE system.
//
// 🧠 Why this matters:
//
// Before you can read or write to a specific Swamp in HydrAIDE, it must be **registered**
// at startup using `RegisterSwamp()`. This tells the engine how to store data,
// how long to cache it, and how frequently to flush it to disk.
//
// ⚠️ You should call this function **once** during application startup —
// ideally before calling `Load()`, `Save()`, or `CatalogRead()`/`CatalogSave()`.
// If the Swamp is not registered, reads and writes may fail with `ErrCodeSwampNotFound`.
//
// ✅ Configuration used:
//
//   - Swamp: `quotes/catalog/main`
//   - CloseAfterIdle: 6 hours → keeps the Swamp in memory if used frequently
//   - Persistent storage: disk-backed with fast flush (every 10s)
//   - Max chunk size: 8 KB files → great for high-frequency updates or small records
//
// 🔧 Best Practice:
// Place this call inside your service initialization logic — typically next to
// other model-based `RegisterPattern()` calls.
func (c *CatalogModelQuote) RegisterPattern(repo repo.Repo) error {
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
		SwampPattern: c.createCatalogName(),

		// Keep the Swamp warm for 6 hours after last usage
		CloseAfterIdle: time.Second * 21600,

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

// createCatalogName defines the Swamp name used to store and retrieve quotes.
//
// HydrAIDE uses a 3-part hierarchical namespace to define logical data domains:
//   - Sanctuary → top-level domain (e.g. "quotes", "users", "products")
//   - Realm     → subdomain or grouping (e.g. "catalog", "logs", "settings")
//   - Swamp     → actual container for data (e.g. "main", "archive", "drafts")
//
// In this example:
//
//	Sanctuary: "quotes"     → the domain of quote-related data
//	Realm:     "catalog"    → signifies this is a catalog-style Swamp
//	Swamp:     "main"       → default bucket where all quotes are stored
//
// You can modify this pattern for multi-tenant setups, language variants,
// or custom categorizations (e.g. Swamp per author, per language, etc.)
//
// This function is reused in `Load()`, `RegisterPattern()`, and other quote operations
// to ensure consistency in Swamp resolution.
//
// 💡 Tip:
// Always build Swamp names using the `name.New()` pattern to guarantee type-safety
// and compatibility with HydrAIDE’s server routing and hashing logic.
func (c *CatalogModelQuote) createCatalogName() name.Name {
	return name.New().Sanctuary("quotes").Realm("catalog").Swamp("main")
}
