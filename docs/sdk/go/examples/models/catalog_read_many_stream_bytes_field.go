package models

import (
	"fmt"
	"log/slog"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// CatalogModelProductWithDetails demonstrates filtering inside complex struct fields.
//
// The Details field is a struct stored in the Treasure's BytesVal.
// With MessagePack encoding enabled, the server can reach inside this struct
// and filter on individual fields using dot-separated paths.
type CatalogModelProductWithDetails struct {
	SKU     string         `hydraide:"key"`
	Name    string         `hydraide:"value"`
	Price   float64        `hydraide:"value,omitempty"`
	Details ProductDetails `hydraide:"value"` // stored in BytesVal as MessagePack
}

// ProductDetails contains structured product metadata.
// Each field can be individually filtered on the server using FilterBytesField* constructors.
type ProductDetails struct {
	Brand   string
	Color   string
	Weight  float64
	InStock bool
	Address ProductAddress
}

// ProductAddress is a nested struct within ProductDetails.
// Fields are accessible via dot-separated paths, e.g. "Address.City".
type ProductAddress struct {
	City    string
	Country string
	ZipCode string
}

// ReadByBrand demonstrates simple BytesField string filtering.
//
// Finds all products where Details.Brand matches exactly.
// The server decodes the MessagePack BytesVal, extracts the "Brand" field,
// and compares it — only matching products are sent to the client.
func (c *CatalogModelProductWithDetails) ReadByBrand(r repo.Repo, category string, brand string) ([]*CatalogModelProductWithDetails, error) {

	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()

	var products []*CatalogModelProductWithDetails

	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	// Filter on a field inside the BytesVal struct
	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldString(hydraidego.Equal, "Brand", brand),
	)

	swamp := name.New().Sanctuary("products").Realm("catalog").Swamp(category)

	err := h.CatalogReadManyStream(ctx, swamp, index, filters, CatalogModelProductWithDetails{}, func(model any) error {
		product, ok := model.(*CatalogModelProductWithDetails)
		if !ok {
			return fmt.Errorf("unexpected model type")
		}
		products = append(products, product)
		return nil
	})

	if err != nil {
		return nil, err
	}

	slog.Info("Products by brand loaded", "brand", brand, "count", len(products))
	return products, nil
}

// ReadByNestedAddress demonstrates filtering on deeply nested struct fields.
//
// Uses dot-separated paths to reach into nested structs: "Address.City", "Address.Country".
// This example finds all in-stock products from a specific city and country.
func (c *CatalogModelProductWithDetails) ReadByNestedAddress(r repo.Repo, category string, city string, country string) ([]*CatalogModelProductWithDetails, error) {

	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()

	var products []*CatalogModelProductWithDetails

	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	// AND: must be in stock AND from the specified city AND country
	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldBool(hydraidego.Equal, "InStock", true),
		hydraidego.FilterBytesFieldString(hydraidego.Equal, "Address.City", city),
		hydraidego.FilterBytesFieldString(hydraidego.Equal, "Address.Country", country),
	)

	swamp := name.New().Sanctuary("products").Realm("catalog").Swamp(category)

	err := h.CatalogReadManyStream(ctx, swamp, index, filters, CatalogModelProductWithDetails{}, func(model any) error {
		product, ok := model.(*CatalogModelProductWithDetails)
		if !ok {
			return fmt.Errorf("unexpected model type")
		}
		products = append(products, product)
		return nil
	})

	if err != nil {
		return nil, err
	}

	slog.Info("Products from address loaded", "city", city, "country", country, "count", len(products))
	return products, nil
}

// ReadWithMixedFilters demonstrates combining primitive Treasure field filters
// with BytesField struct filters in the same FilterGroup.
//
// This finds expensive products (price > minPrice on Float64Val) from a specific
// brand (inside BytesVal struct) whose color contains a substring.
func (c *CatalogModelProductWithDetails) ReadWithMixedFilters(r repo.Repo, category string, minPrice float64, brand string, colorKeyword string) ([]*CatalogModelProductWithDetails, error) {

	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()

	var products []*CatalogModelProductWithDetails

	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	// Mix primitive filters with BytesField filters:
	// price > minPrice (primitive Float64Val)
	// AND Brand == brand (BytesField)
	// AND Color contains colorKeyword (BytesField with string Contains)
	filters := hydraidego.FilterAND(
		hydraidego.FilterFloat64(hydraidego.GreaterThan, minPrice),
		hydraidego.FilterBytesFieldString(hydraidego.Equal, "Brand", brand),
		hydraidego.FilterBytesFieldString(hydraidego.Contains, "Color", colorKeyword),
	)

	swamp := name.New().Sanctuary("products").Realm("catalog").Swamp(category)

	err := h.CatalogReadManyStream(ctx, swamp, index, filters, CatalogModelProductWithDetails{}, func(model any) error {
		product, ok := model.(*CatalogModelProductWithDetails)
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

// ReadWithComplexBytesFieldFilter demonstrates nested AND/OR logic
// combined with BytesField filters.
//
// Finds products that are: (Apple OR Samsung) AND (heavy OR from Budapest)
func (c *CatalogModelProductWithDetails) ReadWithComplexBytesFieldFilter(r repo.Repo, category string) ([]*CatalogModelProductWithDetails, error) {

	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()

	var products []*CatalogModelProductWithDetails

	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	// Complex nested filter:
	// (Brand == "Apple" OR Brand == "Samsung")
	// AND
	// (Weight > 1.0 OR Address.City == "Budapest")
	filters := hydraidego.FilterAND(
		hydraidego.FilterOR(
			hydraidego.FilterBytesFieldString(hydraidego.Equal, "Brand", "Apple"),
			hydraidego.FilterBytesFieldString(hydraidego.Equal, "Brand", "Samsung"),
		),
		hydraidego.FilterOR(
			hydraidego.FilterBytesFieldFloat64(hydraidego.GreaterThan, "Weight", 1.0),
			hydraidego.FilterBytesFieldString(hydraidego.Equal, "Address.City", "Budapest"),
		),
	)

	swamp := name.New().Sanctuary("products").Realm("catalog").Swamp(category)

	err := h.CatalogReadManyStream(ctx, swamp, index, filters, CatalogModelProductWithDetails{}, func(model any) error {
		product, ok := model.(*CatalogModelProductWithDetails)
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
