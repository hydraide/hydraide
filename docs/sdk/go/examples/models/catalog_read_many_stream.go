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

// CatalogModelProduct represents a product in an e-commerce catalog.
//
// This model demonstrates streaming reads with server-side filtering.
// Products are stored per category (e.g. products/catalog/electronics).
type CatalogModelProduct struct {
	SKU       string    `hydraide:"key"`
	Name      string    `hydraide:"value"`
	Price     float64   `hydraide:"value,omitempty"`
	CreatedAt time.Time `hydraide:"createdAt"`
}

// ReadExpensiveProducts demonstrates CatalogReadManyStream with server-side filtering.
//
// Instead of loading all products into memory and filtering client-side,
// this uses server-side filters so only matching Treasures are sent over the network.
//
// Benefits over CatalogReadMany:
//   - Server-side filtering: non-matching records never leave the server
//   - Streaming: results arrive one-by-one, no single large response message
//   - Memory-efficient: ideal for large datasets (millions of records)
//
// Uses FilterAND: all conditions must be true for a Treasure to pass.
func (c *CatalogModelProduct) ReadExpensiveProducts(r repo.Repo, category string, minPrice float64) ([]*CatalogModelProduct, error) {

	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()

	var products []*CatalogModelProduct

	// Define the index: read by creation time, newest first
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
		Limit:      0, // no limit — stream all matching results
	}

	// Define server-side filters using FilterAND (all conditions must match).
	// Only products with Float64Val (price) > minPrice will be sent to the client.
	// The server evaluates this before sending — saving bandwidth and memory.
	filters := hydraidego.FilterAND(
		hydraidego.FilterFloat64(hydraidego.GreaterThan, minPrice),
	)

	// Build the swamp name for this category
	swamp := name.New().Sanctuary("products").Realm("catalog").Swamp(category)

	// CatalogReadManyStream opens a server-streaming gRPC connection.
	// Each matching Treasure is sent individually over the stream.
	err := h.CatalogReadManyStream(ctx, swamp, index, filters, CatalogModelProduct{}, func(model any) error {

		product, ok := model.(*CatalogModelProduct)
		if !ok {
			return hydraidego.NewError(hydraidego.ErrCodeInvalidModel, "unexpected model type")
		}

		products = append(products, product)
		return nil
	})

	if err != nil {
		slog.Error("Failed to stream products", "category", category, "error", err)
		return nil, err
	}

	slog.Info("Products loaded via stream", "category", category, "count", len(products))
	return products, nil
}

// ReadProductsByStringFilter demonstrates filtering on the Treasure's StringVal field.
//
// This finds all products whose value (Name) exactly matches the given name.
// Useful for exact-match lookups across a large catalog.
func (c *CatalogModelProduct) ReadProductsByStringFilter(r repo.Repo, category string, exactName string) ([]*CatalogModelProduct, error) {

	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()

	var products []*CatalogModelProduct

	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexKey,
		IndexOrder: hydraidego.IndexOrderAsc,
	}

	// Filter on StringVal: only Treasures where the value == exactName
	filters := hydraidego.FilterAND(
		hydraidego.FilterString(hydraidego.Equal, exactName),
	)

	swamp := name.New().Sanctuary("products").Realm("catalog").Swamp(category)

	err := h.CatalogReadManyStream(ctx, swamp, index, filters, CatalogModelProduct{}, func(model any) error {
		product, ok := model.(*CatalogModelProduct)
		if !ok {
			return fmt.Errorf("unexpected model type")
		}
		products = append(products, product)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return products, nil
}

// ReadProductsWithoutFilter demonstrates CatalogReadManyStream without any filters.
//
// When filters is nil, CatalogReadManyStream behaves like CatalogReadMany
// but with streaming delivery — results arrive one-by-one instead of all at once.
func (c *CatalogModelProduct) ReadProductsWithoutFilter(r repo.Repo, category string, limit int32) ([]*CatalogModelProduct, error) {

	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()

	var products []*CatalogModelProduct

	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
		Limit:      limit,
	}

	swamp := name.New().Sanctuary("products").Realm("catalog").Swamp(category)

	// nil filters = no server-side filtering, all results are streamed
	err := h.CatalogReadManyStream(ctx, swamp, index, nil, CatalogModelProduct{}, func(model any) error {
		product, ok := model.(*CatalogModelProduct)
		if !ok {
			return fmt.Errorf("unexpected model type")
		}
		products = append(products, product)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return products, nil
}

// ReadProductsWithComplexFilter demonstrates nested AND/OR filter groups.
//
// This finds products that are expensive (price > 500) AND whose name
// either contains "Pro" OR starts with "Ultra".
//
// This demonstrates:
//   - Nested FilterGroup with AND/OR logic
//   - String operators: Contains, StartsWith
func (c *CatalogModelProduct) ReadProductsWithComplexFilter(r repo.Repo, category string) ([]*CatalogModelProduct, error) {

	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()

	var products []*CatalogModelProduct

	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	// Complex filter: price > 500 AND (name contains "Pro" OR name starts with "Ultra")
	filters := hydraidego.FilterAND(
		hydraidego.FilterFloat64(hydraidego.GreaterThan, 500.0),
		hydraidego.FilterOR(
			hydraidego.FilterString(hydraidego.Contains, "Pro"),
			hydraidego.FilterString(hydraidego.StartsWith, "Ultra"),
		),
	)

	swamp := name.New().Sanctuary("products").Realm("catalog").Swamp(category)

	err := h.CatalogReadManyStream(ctx, swamp, index, filters, CatalogModelProduct{}, func(model any) error {
		product, ok := model.(*CatalogModelProduct)
		if !ok {
			return fmt.Errorf("unexpected model type")
		}
		products = append(products, product)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return products, nil
}
