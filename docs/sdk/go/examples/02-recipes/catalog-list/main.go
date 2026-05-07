// 02-recipes/catalog-list — listing and paginating a Catalog with
// CatalogReadMany using an ordered Index.
//
// The classical alternative is `SELECT ... ORDER BY ... LIMIT ... OFFSET`.
// HydrAIDE's equivalent is an Index descriptor (sort field, direction,
// offset, limit) handed to CatalogReadMany. The engine builds the index
// on first use and discards it when the Swamp evicts — no persistent
// secondary index files, no ALTER TABLE.
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

	if err := RunCatalogList(ctx, r); err != nil {
		log.Fatalf("catalog-list failed: %v", err)
	}
	fmt.Println("done.")
}

// Article is the Catalog model.
type Article struct {
	Slug    string   `hydraide:"key"`
	Body    *Body    `hydraide:"value"`
}

type Body struct {
	Title  string `msgpack:"title"`
	Author string `msgpack:"author"`
}

// ArticleSwamp is the namespace for this recipe.
func ArticleSwamp() name.Name {
	return name.New().Sanctuary("examples").Realm("catalog-list").Swamp("articles")
}

// RunCatalogList inserts ten articles, then pages through them in two
// batches of four, ordered by key descending.
func RunCatalogList(ctx context.Context, r repo.Repo) error {
	swamp := ArticleSwamp()

	if err := setup.Pattern(ctx, r,
		name.New().Sanctuary("examples").Realm("catalog-list").Swamp("*")); err != nil {
		return fmt.Errorf("register pattern: %w", err)
	}

	h := r.GetHydraidego()
	_ = h.Destroy(ctx, swamp)
	defer func() { _ = h.Destroy(ctx, swamp) }()

	const total = 10
	for i := 1; i <= total; i++ {
		article := &Article{
			Slug: fmt.Sprintf("article-%02d", i),
			Body: &Body{
				Title:  fmt.Sprintf("Post number %d", i),
				Author: pickAuthor(i),
			},
		}
		if _, err := h.CatalogSave(ctx, swamp, article); err != nil {
			return fmt.Errorf("save: %w", err)
		}
	}
	fmt.Printf("inserted %d articles\n", total)

	pageSize := int32(4)
	for page := int32(0); page < 3; page++ {
		index := &hydraidego.Index{
			IndexType:  hydraidego.IndexKey,
			IndexOrder: hydraidego.IndexOrderDesc,
			From:       page * pageSize,
			Limit:      pageSize,
		}
		fmt.Printf("page %d (offset=%d, limit=%d):\n", page+1, index.From, index.Limit)
		err := h.CatalogReadMany(ctx, swamp, index, Article{}, func(model any) error {
			a := model.(*Article)
			fmt.Printf("  %s — %q by %s\n", a.Slug, a.Body.Title, a.Body.Author)
			return nil
		})
		if err != nil {
			return fmt.Errorf("read page %d: %w", page+1, err)
		}
	}

	return nil
}

func pickAuthor(i int) string {
	if i%2 == 0 {
		return "alice"
	}
	return "bob"
}
