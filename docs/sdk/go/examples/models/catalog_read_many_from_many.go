package models

import (
	"log/slog"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// CatalogModelOrder represents an order in an order management system.
//
// Orders are distributed across multiple regional Swamps (e.g. orders/eu/hu, orders/eu/de).
// This model demonstrates how to read from multiple Swamps in a single streaming call.
type CatalogModelOrder struct {
	OrderID string `hydraide:"key"`
	Status  string `hydraide:"value"` // "pending", "shipped", "delivered"
}

// ReadPendingOrdersFromRegions demonstrates CatalogReadManyFromMany.
//
// This function reads orders from multiple regional Swamps in a single streaming call,
// applying per-swamp filters. Results arrive swamp-by-swamp in the request order.
//
// Use cases:
//   - Cross-region order dashboards
//   - Aggregated reporting across multiple Swamps
//   - Multi-tenant data collection with per-tenant filters
//
// Each SwampQuery can have its own Index and Filters, allowing different
// query parameters per Swamp in the same streaming call.
func (c *CatalogModelOrder) ReadPendingOrdersFromRegions(r repo.Repo, regions []string) (map[string][]*CatalogModelOrder, error) {

	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()

	// Build per-region query requests
	requests := make([]*hydraidego.CatalogReadManyFromManyRequest, 0, len(regions))
	for _, region := range regions {
		requests = append(requests, &hydraidego.CatalogReadManyFromManyRequest{
			SwampName: name.New().Sanctuary("orders").Realm("eu").Swamp(region),
			Index: &hydraidego.Index{
				IndexType:  hydraidego.IndexKey,
				IndexOrder: hydraidego.IndexOrderAsc,
			},
			// Only fetch orders with Status == "pending"
			Filters: hydraidego.FilterAND(
				hydraidego.FilterString(hydraidego.Equal, "pending"),
			),
		})
	}

	// Collect results grouped by swamp
	results := make(map[string][]*CatalogModelOrder)

	// CatalogReadManyFromMany streams results from all requested Swamps.
	// The iterator receives the source swamp name with each result.
	err := h.CatalogReadManyFromMany(ctx, requests, CatalogModelOrder{}, func(swampName name.Name, model any) error {

		order, ok := model.(*CatalogModelOrder)
		if !ok {
			return hydraidego.NewError(hydraidego.ErrCodeInvalidModel, "unexpected model type")
		}

		key := swampName.Get()
		results[key] = append(results[key], order)
		return nil
	})

	if err != nil {
		slog.Error("Failed to read orders from regions", "error", err)
		return nil, err
	}

	// Log results per region
	for region, orders := range results {
		slog.Info("Pending orders loaded", "region", region, "count", len(orders))
	}

	return results, nil
}

// ReadAllOrdersFromRegions demonstrates CatalogReadManyFromMany without filters.
//
// When Filters is nil in a CatalogReadManyFromManyRequest, all Treasures
// matching the Index criteria are streamed from that Swamp.
func (c *CatalogModelOrder) ReadAllOrdersFromRegions(r repo.Repo, regions []string, limit int32) ([]*CatalogModelOrder, error) {

	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()

	requests := make([]*hydraidego.CatalogReadManyFromManyRequest, 0, len(regions))
	for _, region := range regions {
		requests = append(requests, &hydraidego.CatalogReadManyFromManyRequest{
			SwampName: name.New().Sanctuary("orders").Realm("eu").Swamp(region),
			Index: &hydraidego.Index{
				IndexType:  hydraidego.IndexCreationTime,
				IndexOrder: hydraidego.IndexOrderDesc,
				Limit:      limit,
			},
			// No filters: all results from this swamp are streamed
		})
	}

	var allOrders []*CatalogModelOrder

	err := h.CatalogReadManyFromMany(ctx, requests, CatalogModelOrder{}, func(swampName name.Name, model any) error {
		order, ok := model.(*CatalogModelOrder)
		if !ok {
			return hydraidego.NewError(hydraidego.ErrCodeInvalidModel, "unexpected model type")
		}
		allOrders = append(allOrders, order)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return allOrders, nil
}

// ReadOrdersWithComplexFilter demonstrates nested AND/OR filtering across multiple Swamps.
//
// This reads orders that are either "pending" OR whose status contains "ship"
// from multiple regions.
func (c *CatalogModelOrder) ReadOrdersWithComplexFilter(r repo.Repo, regions []string) (map[string][]*CatalogModelOrder, error) {

	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()

	requests := make([]*hydraidego.CatalogReadManyFromManyRequest, 0, len(regions))
	for _, region := range regions {
		requests = append(requests, &hydraidego.CatalogReadManyFromManyRequest{
			SwampName: name.New().Sanctuary("orders").Realm("eu").Swamp(region),
			Index: &hydraidego.Index{
				IndexType:  hydraidego.IndexKey,
				IndexOrder: hydraidego.IndexOrderAsc,
			},
			// OR filter: status == "pending" OR status contains "ship"
			Filters: hydraidego.FilterOR(
				hydraidego.FilterString(hydraidego.Equal, "pending"),
				hydraidego.FilterString(hydraidego.Contains, "ship"),
			),
		})
	}

	results := make(map[string][]*CatalogModelOrder)

	err := h.CatalogReadManyFromMany(ctx, requests, CatalogModelOrder{}, func(swampName name.Name, model any) error {
		order, ok := model.(*CatalogModelOrder)
		if !ok {
			return hydraidego.NewError(hydraidego.ErrCodeInvalidModel, "unexpected model type")
		}
		key := swampName.Get()
		results[key] = append(results[key], order)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return results, nil
}
