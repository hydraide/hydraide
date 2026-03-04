package models

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// --- Models ---

// DocumentModel represents a document with a full-text word index.
// The WordIndex field stores word positions for phrase search.
type DocumentModel struct {
	ID        string           `hydraide:"key"`
	Title     string           `hydraide:"value"`
	WordIndex map[string][]int `hydraide:"value"` // word -> position list
}

// UserWithMetadata represents a user with a dynamic metadata map.
type UserWithMetadata struct {
	UserID   string                 `hydraide:"key"`
	Name     string                 `hydraide:"value"`
	Metadata map[string]interface{} `hydraide:"value"` // dynamic key-value pairs
}

// --- Timestamp Filtering Examples ---

// ReadRecentlyCreated demonstrates filtering on the CreatedAt timestamp.
// Only Treasures created in the last 24 hours are returned.
func (d *DocumentModel) ReadRecentlyCreated(r repo.Repo) ([]*CatalogModelProduct, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("products").Realm("catalog").Swamp("electronics")
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	filters := hydraidego.FilterAND(
		hydraidego.FilterCreatedAt(hydraidego.GreaterThan, cutoff),
	)

	var results []*CatalogModelProduct
	err := h.CatalogReadManyStream(ctx, swamp, index, filters, CatalogModelProduct{}, func(model any) error {
		product := model.(*CatalogModelProduct)
		results = append(results, product)
		return nil
	})

	if err != nil {
		slog.Error("ReadRecentlyCreated", "error", err)
		return nil, err
	}
	return results, nil
}

// ReadExpiredTreasures demonstrates filtering for expired Treasures.
func (d *DocumentModel) ReadExpiredTreasures(r repo.Repo) error {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("sessions").Realm("auth").Swamp("tokens")
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexExpirationTime,
		IndexOrder: hydraidego.IndexOrderAsc,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterExpiredAt(hydraidego.LessThan, time.Now()),
	)

	return h.CatalogReadManyStream(ctx, swamp, index, filters, DocumentModel{}, func(model any) error {
		doc := model.(*DocumentModel)
		fmt.Printf("Expired: %s\n", doc.ID)
		return nil
	})
}

// --- Map Key Existence Examples ---

// ReadUsersWithEmail demonstrates HasKey filtering.
// Only users whose Metadata map contains the "email" key are returned.
func (u *UserWithMetadata) ReadUsersWithEmail(r repo.Repo) ([]*UserWithMetadata, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("users").Realm("profiles").Swamp("eu")
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexKey,
		IndexOrder: hydraidego.IndexOrderAsc,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldString(hydraidego.HasKey, "Metadata", "email"),
	)

	var results []*UserWithMetadata
	err := h.CatalogReadManyStream(ctx, swamp, index, filters, UserWithMetadata{}, func(model any) error {
		user := model.(*UserWithMetadata)
		results = append(results, user)
		return nil
	})

	if err != nil {
		return nil, err
	}
	return results, nil
}

// ReadUsersWithoutPhone demonstrates HasNotKey filtering.
func (u *UserWithMetadata) ReadUsersWithoutPhone(r repo.Repo) ([]*UserWithMetadata, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("users").Realm("profiles").Swamp("eu")
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexKey,
		IndexOrder: hydraidego.IndexOrderAsc,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldString(hydraidego.HasNotKey, "Metadata", "phone"),
	)

	var results []*UserWithMetadata
	err := h.CatalogReadManyStream(ctx, swamp, index, filters, UserWithMetadata{}, func(model any) error {
		user := model.(*UserWithMetadata)
		results = append(results, user)
		return nil
	})

	if err != nil {
		return nil, err
	}
	return results, nil
}

// --- Phrase Search Examples ---

// SearchPhrase demonstrates phrase search with consecutive word positions.
// Finds documents containing the phrase "altalanos szerzodesi feltetelek".
func (d *DocumentModel) SearchPhrase(r repo.Repo) ([]*DocumentModel, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("documents").Realm("legal").Swamp("contracts")
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterPhrase("WordIndex", "altalanos", "szerzodesi", "feltetelek"),
	)

	var results []*DocumentModel
	err := h.CatalogReadManyStream(ctx, swamp, index, filters, DocumentModel{}, func(model any) error {
		doc := model.(*DocumentModel)
		results = append(results, doc)
		return nil
	})

	if err != nil {
		return nil, err
	}
	return results, nil
}

// SearchExcludePhrase demonstrates negated phrase search.
// Returns documents that do NOT contain the phrase.
func (d *DocumentModel) SearchExcludePhrase(r repo.Repo) ([]*DocumentModel, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("documents").Realm("legal").Swamp("contracts")
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterNotPhrase("WordIndex", "altalanos", "szerzodesi", "feltetelek"),
	)

	var results []*DocumentModel
	err := h.CatalogReadManyStream(ctx, swamp, index, filters, DocumentModel{}, func(model any) error {
		doc := model.(*DocumentModel)
		results = append(results, doc)
		return nil
	})

	if err != nil {
		return nil, err
	}
	return results, nil
}

// SearchPhraseWithTimestamp demonstrates combining phrase search with timestamp filtering.
// Finds recent documents (last 7 days) containing a specific phrase.
func (d *DocumentModel) SearchPhraseWithTimestamp(r repo.Repo) ([]*DocumentModel, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("documents").Realm("legal").Swamp("contracts")
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterCreatedAt(hydraidego.GreaterThan, time.Now().Add(-7*24*time.Hour)),
		hydraidego.FilterPhrase("WordIndex", "adatvedelmi", "nyilatkozat"),
	)

	var results []*DocumentModel
	err := h.CatalogReadManyStream(ctx, swamp, index, filters, DocumentModel{}, func(model any) error {
		doc := model.(*DocumentModel)
		results = append(results, doc)
		return nil
	})

	if err != nil {
		return nil, err
	}
	return results, nil
}
