// Package hydrex
//
// Hydrex is a modular indexing and data-management layer built on top of the hydraidego interface.
// It provides domain-based structured storage, efficient indexing, and fast querying capabilities for B2B applications or internal data workflows.
//
// ## Purpose
//
// Hydrex allows you to:
// - Store structured data entries per domain (any unique identifier, not just DNS-based).
// - Automatically maintain index references for fast reverse lookup.
// - Safely update entries by syncing additions and deletions between the index and the core data.
// - Retrieve all key-value pairs associated with a domain (core data).
// - Query index entries to find all domains that match a specific key.
// - Cleanly destroy all domain-related entries from both core storage and all relevant indices.
//
// ## Key Concepts
//
// - **Core Data**: Represents the actual values associated with a domain (e.g., metadata, tags, analysis results).
// - **Index**: Reverse-mapping layer for looking up domains by key (e.g., find all domains with the tag "SEO").
// - **Domain**: A logical identifier for any entity (e.g., website, customer ID, etc.)—does not need to be a DNS domain.
//
// ## Main Operations
//
// - `Save`: Adds/updates core data and manages index consistency. Old keys not present in the update will be removed.
// - `GetCoreData`: Returns the core data values for a specific domain.
// - `GetIndexData`: Retrieves all domains associated with a given key.
// - `Destroy`: Fully removes all data related to a domain from both core and index layers.
//
// ## Example Use Case
//
// 1. Initialize Hydrex:
//
// ```go
//
//	hydraide := hydraidego.New()
//	hx := hydrex.New(hydraide)
//
//	// Save data under a domain:
//
//	hx.Save(ctx, "product_tags", "mydomain.com", map[string]*hydrex.CoreData{
//	  "category": {Key: "category", Value: "software", CreatedAt: time.Now()},
//	  "feature": {Key: "feature", Value: "AI", CreatedAt: time.Now()},
//	})
//
//	// Get core data for domain:
//	core := hx.GetCoreData(ctx, "product_tags", "mydomain.com")
//
//	// Get index data for a key:
//	hits := hx.GetIndexData(ctx, "product_tags", "feature")
//
//	// Remove all data for a domain:
//	hx.Destroy(ctx, "product_tags", "mydomain.com")
//
// ```
//
// Hydrex is ideal for systems that need searchable tagging, metadata enrichment, or content annotation based on dynamic, domain-scoped entities.
package hydrex

import (
	"context"
	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"log/slog"

	"time"
)

const (
	sanctuaryHydraideIndex    = "hydraideIndex"
	sanctuaryHydraideCoreData = "hydraideCoreData"
)

type Hydrex interface {
	Save(ctx context.Context, indexName string, domain string, items map[string]*CoreData)
	GetCoreData(ctx context.Context, indexName string, domain string) []*CoreData
	GetIndexData(ctx context.Context, indexName string, key string) []*IndexedData
	Destroy(ctx context.Context, indexName string, domain string)
}

// CoreData represents a single key-value metadata entry assigned to a specific domain.
//
// Each key must be unique within the domain scope.
// The `CreatedAt` timestamp is used for ordering and traceability.
type CoreData struct {
	Key       string    `hydraide:"key"`
	Value     string    `hydraide:"value"`
	CreatedAt time.Time `hydraide:"createdAt"`
}

// IndexedData represents a domain reference within an index associated with a key.
//
// This struct enables reverse lookup: "Which domains have this key?".
// Stored as part of the index layer, not the core data.
type IndexedData struct {
	Domain    string    `hydraide:"key"`
	CreatedAt time.Time `hydraide:"createdAt"`
}

type hydrex struct {
	hydraidegoInterface hydraidego.Hydraidego
}

// New creates a new Hydrex instance bound to a specific hydraidego interface.
//
// It also registers the required swamp patterns for core data and index usage.
// This should be called once per runtime/service instance.
func New(hydrunInterface hydraidego.Hydraidego) Hydrex {

	i := &hydrex{
		hydraidegoInterface: hydrunInterface,
	}

	i.registerPattern()

	return i

}

// Save persists domain-scoped core data and updates the related indexes accordingly.
//
// For each domain (which can be any unique identifier, not limited to DNS):
// - Keys not present in the new `items` map but existing in current storage will be deleted.
// - New keys not yet present will be added to both the core data and their respective indexes.
// - Unchanged keys will remain intact.
//
// Requirements:
// - `indexName` must be unique across the entire system (think of it as a namespace).
// - `domain` must be unique within the given index.
// - Each key in `items` must be unique within the domain scope.
//
// This method ensures full consistency between the stored data and all reverse indices.
func (h *hydrex) Save(ctx context.Context, indexName string, domain string, items map[string]*CoreData) {

	// get the current core data for the domain
	existingCoreData := make(map[string]*CoreData)

	coreDataName := h.createCoreDataName(indexName, domain)

	// get all data for the domain
	_ = h.hydraidegoInterface.CatalogReadMany(ctx, coreDataName, &hydraidego.Index{
		IndexType:  hydraidego.IndexKey,
		IndexOrder: hydraidego.IndexOrderAsc,
		From:       0,
		Limit:      0,
	}, CoreData{}, func(model any) error {
		// load the existing core data into a map
		m := model.(*CoreData)
		existingCoreData[m.Key] = m
		return nil
	})

	itemsForDelete := make([]string, 0)
	itemsForSave := make([]any, 0)

	deleteManyFromManyReq := make([]*hydraidego.CatalogDeleteManyFromManyRequest, 0)
	saveManyToManyReq := make([]*hydraidego.CatalogManyToManyRequest, 0)

	// iterating through the existing core data
	for key := range existingCoreData {
		if _, ok := items[key]; !ok {
			itemsForDelete = append(itemsForDelete, key)
			// töröljük a domaint-t az indexekből, ahol már nem létezik az adat
			deleteManyFromManyReq = append(deleteManyFromManyReq, &hydraidego.CatalogDeleteManyFromManyRequest{
				SwampName: h.createIndexName(indexName, key),
				Keys:      []string{domain},
			})
		}
	}

	// delete the domain from all indexes where it no longer exists
	if len(deleteManyFromManyReq) > 0 {
		_ = h.hydraidegoInterface.CatalogDeleteManyFromMany(ctx, deleteManyFromManyReq, nil)
	}

	// delete domains from the core data
	if len(itemsForDelete) > 0 {
		_ = h.hydraidegoInterface.CatalogDeleteMany(ctx, coreDataName, itemsForDelete, nil)
	}

	// iterating through the new items
	for key, data := range items {
		if _, ok := existingCoreData[key]; !ok {

			// array for saving new items
			itemsForSave = append(itemsForSave, &CoreData{
				Key:       key,
				Value:     data.Value,
				CreatedAt: time.Now(),
			})

			// save hydrex by this array
			saveManyToManyReq = append(saveManyToManyReq, &hydraidego.CatalogManyToManyRequest{
				SwampName: h.createIndexName(indexName, key),
				Models: []any{
					&IndexedData{
						Domain:    domain,
						CreatedAt: time.Now(),
					},
				},
			})

		}
	}

	// add new key to the core data
	if err := h.hydraidegoInterface.CatalogSaveMany(ctx, coreDataName, itemsForSave, nil); err != nil {
		slog.Error("Error while saving core data", "error", err)
	}

	// save all domains to the indexes
	if err := h.hydraidegoInterface.CatalogSaveManyToMany(ctx, saveManyToManyReq, nil); err != nil {
		slog.Error("Error while saving hydrex", "error", err)
	}

}

// GetCoreData retrieves all key-value entries (CoreData) associated with the given domain.
//
// Returns the list in descending order of creation time.
// Useful for fetching all metadata or annotations related to a specific entity.
//
// If no data is found, returns an empty slice.
func (h *hydrex) GetCoreData(ctx context.Context, indexName string, domain string) []*CoreData {

	cdn := h.createCoreDataName(indexName, domain)

	cd := make([]*CoreData, 0)

	_ = h.hydraidegoInterface.CatalogReadMany(ctx, cdn, &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
		From:       0,
		Limit:      0,
	}, CoreData{}, func(model any) error {
		m := model.(*CoreData)
		cd = append(cd, &CoreData{
			Key:       m.Key,
			Value:     m.Value,
			CreatedAt: m.CreatedAt,
		})
		return nil
	})

	return cd

}

// GetIndexData retrieves all domains that have the given key in their saved items,
// based on the specified `indexName` (namespace).
//
// This is essentially a reverse index query: "Who has the key X?"
//
// If the index does not exist or the key has no associated domains, returns nil.
func (h *hydrex) GetIndexData(ctx context.Context, indexName string, key string) []*IndexedData {

	cd := make([]*IndexedData, 0)

	in := h.createIndexName(indexName, key)

	err := h.hydraidegoInterface.CatalogReadMany(ctx, in, &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
		From:       0,
		Limit:      0,
	}, IndexedData{}, func(model any) error {

		m := model.(*IndexedData)
		cd = append(cd, &IndexedData{
			Domain:    m.Domain,
			CreatedAt: m.CreatedAt,
		})
		return nil

	})

	if err != nil {
		slog.Error("Error while reading hydrex", "error", err)
	}

	return cd

}

// Destroy removes all data associated with the given domain:
// - Deletes all core data entries tied to the domain.
// - Removes the domain from all related key-based indexes.
//
// This operation is irreversible and should be used for full cleanup
// (e.g., GDPR erasure, object deletion).
func (h *hydrex) Destroy(ctx context.Context, indexName string, domain string) {

	coreDataName := h.createCoreDataName(indexName, domain)

	deleteManyFromManyReq := make([]*hydraidego.CatalogDeleteManyFromManyRequest, 0)

	_ = h.hydraidegoInterface.CatalogReadMany(ctx, coreDataName, &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
		From:       0,
		Limit:      0,
	}, CoreData{}, func(model any) error {

		m := model.(*CoreData)

		deleteManyFromManyReq = append(deleteManyFromManyReq, &hydraidego.CatalogDeleteManyFromManyRequest{
			SwampName: h.createIndexName(indexName, m.Key),
			Keys:      []string{domain},
		})

		return nil

	})

	_ = h.hydraidegoInterface.Destroy(ctx, coreDataName)

	if len(deleteManyFromManyReq) > 0 {
		_ = h.hydraidegoInterface.CatalogDeleteManyFromMany(ctx, deleteManyFromManyReq, nil)
	}

}

// registerPattern registers the swamp patterns used by Hydrex for both
// core data and key-based indexes.
//
// These patterns must be registered before any data operation to ensure
// consistency and proper file management (e.g., write interval, file size).
//
// Note: Swamps are created lazily upon first write.
func (h *hydrex) registerPattern() {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := h.hydraidegoInterface.RegisterSwamp(ctx, &hydraidego.RegisterSwampRequest{
		SwampPattern:    name.New().Sanctuary(sanctuaryHydraideIndex).Realm("*").Swamp("*"),
		CloseAfterIdle:  1 * time.Second,
		IsInMemorySwamp: false,
		FilesystemSettings: &hydraidego.SwampFilesystemSettings{
			WriteInterval: 1,
			MaxFileSize:   8192,
		},
	})

	if err != nil {
		slog.Error("Error while registering hydrunIndex pattern", "error", err)
	}

	err = h.hydraidegoInterface.RegisterSwamp(ctx, &hydraidego.RegisterSwampRequest{
		SwampPattern:    name.New().Sanctuary(sanctuaryHydraideCoreData).Realm("*").Swamp("*"),
		CloseAfterIdle:  1 * time.Second,
		IsInMemorySwamp: false,
		FilesystemSettings: &hydraidego.SwampFilesystemSettings{
			WriteInterval: 1,
			MaxFileSize:   8192,
		},
	})

	if err != nil {
		slog.Error("Error while registering hydrunCoreData pattern", "error", err)
	}

}

// createCoreDataName generates a unique storage name (swamp) for domain-scoped core data.
//
// Pattern: Sanctuary = hydraideCoreData / Realm = indexName / Swamp = domain
// Example: hydraideCoreData.myIndex.customer123
//
// This defines where the domain’s key-value pairs will be stored.
func (h *hydrex) createCoreDataName(indexName, domain string) name.Name {
	return name.New().Sanctuary(sanctuaryHydraideCoreData).Realm(indexName).Swamp(domain)
}

// createIndexName generates a unique storage name (swamp) for the reverse index of a specific key.
//
// Pattern: Sanctuary = hydraideIndex / Realm = indexName / Swamp = key
// Example: hydraideIndex.productTags.feature
//
// This defines where the list of domains containing a given key will be stored.
func (h *hydrex) createIndexName(indexName, key string) name.Name {
	return name.New().Sanctuary(sanctuaryHydraideIndex).Realm(indexName).Swamp(key)
}
