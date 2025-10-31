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

// CatalogReadBatch — Fast multi-key reads from a single Swamp.
//
// When to use:
// - You know the exact keys you want to read
// - You want to minimize roundtrips (one request, many results)
// - You prefer processing results one-by-one via an iterator
//
// Key properties:
// - Sends one gRPC call with all requested keys
// - Silently skips non-existing keys
// - For each match, creates a fresh model instance and passes it to the iterator
// - If the iterator returns an error, processing stops immediately
//
// Note: The model parameter must be a non-pointer type; the SDK creates new instances internally
// (via reflect.New) and populates them. The iterator receives a pointer (as interface{}), which you can cast.
//
// This file provides a complete, copyable example.

type CatalogModelUserBasic struct {
	UserID    string    `hydraide:"key"`
	Name      string    `hydraide:"value"`
	CreatedBy string    `hydraide:"createdBy"`
	CreatedAt time.Time `hydraide:"createdAt"`
}

// RegisterPattern — Register the Swamp during application startup.
func (c *CatalogModelUserBasic) RegisterPattern(r repo.Repo) error {
	h := r.GetHydraidego()
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	errs := h.RegisterSwamp(ctx, &hydraidego.RegisterSwampRequest{
		SwampPattern:    name.New().Sanctuary("users").Realm("catalog").Swamp("all"),
		CloseAfterIdle:  2 * time.Hour,
		IsInMemorySwamp: false,
		FilesystemSettings: &hydraidego.SwampFilesystemSettings{
			WriteInterval: 10 * time.Second,
			MaxFileSize:   8192,
		},
	})
	if errs != nil {
		return hydraidehelper.ConcatErrors(errs)
	}
	return nil
}

// Save — Upsert sample data to demonstrate batch reading.
func (c *CatalogModelUserBasic) Save(r repo.Repo) error {
	h := r.GetHydraidego()
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	_, err := h.CatalogSave(ctx, c.catalogName(), c)
	return err
}

// ReadBatch — Read multiple users in a single request.
func (c *CatalogModelUserBasic) ReadBatch(r repo.Repo, userIDs []string) ([]*CatalogModelUserBasic, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}

	h := r.GetHydraidego()
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	results := make([]*CatalogModelUserBasic, 0, len(userIDs))
	err := h.CatalogReadBatch(ctx, c.catalogName(), userIDs, CatalogModelUserBasic{}, func(m any) error {
		u := m.(*CatalogModelUserBasic)
		results = append(results, u)
		return nil
	})
	return results, err
}

// Example — end-to-end demo: register, save, batch read.
func Example_CatalogReadBatch() {
	var r repo.Repo // Your app initializes this (see repo.go in the SDK)

	// 1) Register the Swamp
	_ = (&CatalogModelUserBasic{}).RegisterPattern(r)

	// 2) Insert some sample data
	_ = (&CatalogModelUserBasic{UserID: "user-1", Name: "Alice", CreatedBy: "example", CreatedAt: time.Now().UTC()}).Save(r)
	_ = (&CatalogModelUserBasic{UserID: "user-2", Name: "Bob", CreatedBy: "example", CreatedAt: time.Now().UTC()}).Save(r)
	_ = (&CatalogModelUserBasic{UserID: "user-3", Name: "Carol", CreatedBy: "example", CreatedAt: time.Now().UTC()}).Save(r)

	// 3) Batch read: user-1, user-3, and a missing key
	ids := []string{"user-1", "user-3", "missing"}
	rows, err := (&CatalogModelUserBasic{}).ReadBatch(r, ids)
	if err != nil {
		slog.Error("CatalogReadBatch error", "err", err)
		return
	}

	for _, u := range rows {
		fmt.Printf("%s -> %s\n", u.UserID, u.Name)
	}
	// Output:
	// user-1 -> Alice
	// user-3 -> Carol
}

func (c *CatalogModelUserBasic) catalogName() name.Name {
	return name.New().Sanctuary("users").Realm("catalog").Swamp("all")
}
