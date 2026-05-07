// 02-recipes/advanced-filters — server-side filtering with AND/OR logic.
//
// HydrAIDE evaluates filters on the server. The client sends a
// FilterGroup describing the predicate; the engine streams back only the
// Treasures that match. There is no client-side `for _, x := range all`
// loop, no over-fetching, no SELECT-then-throw-away.
//
// This recipe shows three queries against a single Catalog of products:
//
//  1. AND: category == "books" AND price > 1500
//  2. OR:  category == "books" OR  category == "music"
//  3. IN-style restriction via Index.IncludedKeys.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/internal/setup"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/utils/repo"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r, cleanup := setup.MustClient(ctx)
	defer cleanup()

	if err := RunAdvancedFilters(ctx, r); err != nil {
		log.Fatalf("advanced-filters failed: %v", err)
	}
	fmt.Println("done.")
}

// Product is the Catalog model. Field-level filters target paths inside
// Detail (the msgpack body).
type Product struct {
	SKU    string  `hydraide:"key"`
	Detail *Detail `hydraide:"value"`
}

type Detail struct {
	Category string `msgpack:"category"`
	PriceCt  int64  `msgpack:"priceCt"`
	Title    string `msgpack:"title"`
}

// ProductSwamp is the namespace for this recipe.
func ProductSwamp() name.Name {
	return name.New().Sanctuary("examples").Realm("advanced-filters").Swamp("products")
}

// RunAdvancedFilters seeds a small product catalog and runs three
// representative queries against it.
func RunAdvancedFilters(ctx context.Context, r repo.Repo) error {
	swamp := ProductSwamp()

	if err := setup.Pattern(ctx, r,
		name.New().Sanctuary("examples").Realm("advanced-filters").Swamp("*")); err != nil {
		return fmt.Errorf("register pattern: %w", err)
	}

	h := r.GetHydraidego()
	_ = h.Destroy(ctx, swamp)
	defer func() { _ = h.Destroy(ctx, swamp) }()

	seed := []*Product{
		{SKU: "B-001", Detail: &Detail{Category: "books", PriceCt: 1200, Title: "The Pragmatic Programmer"}},
		{SKU: "B-002", Detail: &Detail{Category: "books", PriceCt: 1900, Title: "Designing Data-Intensive Applications"}},
		{SKU: "B-003", Detail: &Detail{Category: "books", PriceCt: 2400, Title: "Crafting Interpreters"}},
		{SKU: "M-001", Detail: &Detail{Category: "music", PriceCt: 1500, Title: "Kind of Blue"}},
		{SKU: "M-002", Detail: &Detail{Category: "music", PriceCt: 999, Title: "Discovery"}},
		{SKU: "T-001", Detail: &Detail{Category: "tools", PriceCt: 4500, Title: "Mechanical Pencil"}},
	}
	for _, p := range seed {
		if _, err := h.CatalogSave(ctx, swamp, p); err != nil {
			return fmt.Errorf("seed %s: %w", p.SKU, err)
		}
	}
	fmt.Printf("seeded %d products\n", len(seed))

	indexAll := &hydraidego.Index{
		IndexType:  hydraidego.IndexKey,
		IndexOrder: hydraidego.IndexOrderAsc,
	}

	// Query 1 — AND: books that cost more than €15.00.
	fmt.Println("\nquery: category == books AND priceCt > 1500")
	andFilter := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldString(hydraidego.Equal, "category", "books"),
		hydraidego.FilterBytesFieldInt64(hydraidego.GreaterThan, "priceCt", 1500),
	)
	if err := streamAndPrint(ctx, h, swamp, indexAll, andFilter); err != nil {
		return err
	}

	// Query 2 — OR: anything in books OR music.
	fmt.Println("\nquery: category == books OR category == music")
	orFilter := hydraidego.FilterOR(
		hydraidego.FilterBytesFieldString(hydraidego.Equal, "category", "books"),
		hydraidego.FilterBytesFieldString(hydraidego.Equal, "category", "music"),
	)
	if err := streamAndPrint(ctx, h, swamp, indexAll, orFilter); err != nil {
		return err
	}

	// Query 3 — IN-style: only specific SKUs (key whitelist via Index).
	fmt.Println("\nquery: SKU IN (B-001, T-001)")
	indexWhitelist := &hydraidego.Index{
		IndexType:    hydraidego.IndexKey,
		IndexOrder:   hydraidego.IndexOrderAsc,
		IncludedKeys: []string{"B-001", "T-001"},
	}
	if err := streamAndPrint(ctx, h, swamp, indexWhitelist, nil); err != nil {
		return err
	}

	return nil
}

func streamAndPrint(ctx context.Context, h hydraidego.Hydraidego, swamp name.Name, index *hydraidego.Index, filter *hydraidego.FilterGroup) error {
	return h.CatalogReadManyStream(ctx, swamp, index, filter, Product{}, func(model any) error {
		p := model.(*Product)
		fmt.Printf("  %s [%s] %d ct — %s\n", p.SKU, p.Detail.Category, p.Detail.PriceCt, p.Detail.Title)
		return nil
	})
}
