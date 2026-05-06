# 02-recipes/catalog-list

Listing and paginating a Catalog with `CatalogReadMany` and an ordered
`Index`.

## What it shows

- Insert 10 articles into a single Catalog Swamp.
- Read them back in pages of 4, ordered by key descending.
- Demonstrate that the index is built on demand — no persistent index
  files, no schema migration to add a new sort order.

## The Index descriptor

```go
&hydraidego.Index{
    IndexType:  hydraidego.IndexKey,        // sort by key
    IndexOrder: hydraidego.IndexOrderDesc,  // newest first
    From:       offset,                     // pagination offset
    Limit:      pageSize,                   // page size
}
```

Other index types include `IndexValueString`, `IndexValueInt64`,
`IndexCreationTime`, `IndexUpdateTime`, `IndexExpirationTime`.

## Run it

```bash
docker compose up -d
make recipe-catalog-list
```

## Test it

```bash
make test-examples
```
