package models

import (
	"fmt"
	"log/slog"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// --- Models ---

// FilteredUserProfile represents a user profile stored as separate Treasures per field.
type FilteredUserProfile struct {
	Name   string `hydraide:"Name"`
	Age    int32  `hydraide:"Age"`
	Status string `hydraide:"Status"`
	Email  string `hydraide:"Email"`
}

// SearchableProfile represents a profile with full-text search capability.
type SearchableProfile struct {
	Title     string           `hydraide:"Title"`
	Content   string           `hydraide:"Content"`
	WordIndex map[string][]int `hydraide:"WordIndex"` // word -> position list
}

// --- Profile Filtering Examples ---

// ReadActiveAdultUser demonstrates single profile filtering with ForKey().
// The profile is only loaded if Age > 18 AND Status == "active".
func (u *FilteredUserProfile) ReadActiveAdultUser(r repo.Repo) (bool, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("users").Realm("profiles").Swamp("alice")

	filters := hydraidego.FilterAND(
		hydraidego.FilterInt32(hydraidego.GreaterThan, 18).ForKey("Age"),
		hydraidego.FilterString(hydraidego.Equal, "active").ForKey("Status"),
	)

	user := &FilteredUserProfile{}
	matched, err := h.ProfileReadWithFilter(ctx, swamp, filters, user)
	if err != nil {
		slog.Error("ReadActiveAdultUser", "error", err)
		return false, err
	}

	if matched {
		fmt.Printf("User: %s, Age: %d, Status: %s\n", user.Name, user.Age, user.Status)
	}
	return matched, nil
}

// --- Multi-Profile Streaming Examples ---

// FindActiveUsers demonstrates multi-profile streaming with filters.
// Scans multiple user profiles and returns only those matching the criteria.
func (u *FilteredUserProfile) FindActiveUsers(r repo.Repo) ([]*FilteredUserProfile, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()

	swampNames := []name.Name{
		name.New().Sanctuary("users").Realm("profiles").Swamp("alice"),
		name.New().Sanctuary("users").Realm("profiles").Swamp("bob"),
		name.New().Sanctuary("users").Realm("profiles").Swamp("charlie"),
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterString(hydraidego.Equal, "active").ForKey("Status"),
	)

	var results []*FilteredUserProfile
	err := h.ProfileReadBatchWithFilter(ctx, swampNames, filters, &FilteredUserProfile{}, 0,
		func(swampName name.Name, model any, err error) error {
			if err != nil {
				slog.Warn("profile not found", "swamp", swampName.Get())
				return nil // skip errors, continue
			}
			user := model.(*FilteredUserProfile)
			results = append(results, user)
			return nil
		})

	if err != nil {
		return nil, err
	}
	return results, nil
}

// FindFirstNActiveUsers demonstrates MaxResults with profile streaming.
// Returns at most maxResults matching profiles, then stops.
func (u *FilteredUserProfile) FindFirstNActiveUsers(r repo.Repo, swampNames []name.Name, maxResults int32) ([]*FilteredUserProfile, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()

	filters := hydraidego.FilterAND(
		hydraidego.FilterString(hydraidego.Equal, "active").ForKey("Status"),
		hydraidego.FilterInt32(hydraidego.GreaterThan, 18).ForKey("Age"),
	)

	var results []*FilteredUserProfile
	err := h.ProfileReadBatchWithFilter(ctx, swampNames, filters, &FilteredUserProfile{}, maxResults,
		func(swampName name.Name, model any, err error) error {
			if err != nil {
				return nil
			}
			results = append(results, model.(*FilteredUserProfile))
			return nil
		})

	if err != nil {
		return nil, err
	}
	return results, nil
}

// --- Phrase Search on Profiles ---

// SearchProfilesByPhrase demonstrates phrase search targeting a profile field.
func (s *SearchableProfile) SearchProfilesByPhrase(r repo.Repo, swampNames []name.Name) ([]*SearchableProfile, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()

	filters := hydraidego.FilterAND(
		hydraidego.FilterPhrase("WordIndex", "altalanos", "szerzodesi", "feltetelek").ForKey("WordIndex"),
	)

	var results []*SearchableProfile
	err := h.ProfileReadBatchWithFilter(ctx, swampNames, filters, &SearchableProfile{}, 0,
		func(swampName name.Name, model any, err error) error {
			if err != nil {
				return nil
			}
			results = append(results, model.(*SearchableProfile))
			return nil
		})

	if err != nil {
		return nil, err
	}
	return results, nil
}

// --- MaxResults with Catalog Streaming ---

// ReadTopProducts demonstrates MaxResults with CatalogReadManyStream.
// Returns at most 10 matching products.
func (u *FilteredUserProfile) ReadTopProducts(r repo.Repo) ([]*CatalogModelProduct, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("products").Realm("catalog").Swamp("electronics")
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
		MaxResults: 10, // stop after 10 matches
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldFloat64(hydraidego.GreaterThan, "Price", 50.0),
	)

	var results []*CatalogModelProduct
	err := h.CatalogReadManyStream(ctx, swamp, index, filters, CatalogModelProduct{}, func(model any) error {
		product := model.(*CatalogModelProduct)
		results = append(results, product)
		return nil
	})

	if err != nil {
		return nil, err
	}
	return results, nil
}

// --- OR Logic with Profile Filters ---

// FindUsersWithORLogic demonstrates OR logic in profile filtering.
// Matches profiles where Status == "active" OR Age > 30.
func (u *FilteredUserProfile) FindUsersWithORLogic(r repo.Repo, swampNames []name.Name) ([]*FilteredUserProfile, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()

	filters := hydraidego.FilterOR(
		hydraidego.FilterString(hydraidego.Equal, "active").ForKey("Status"),
		hydraidego.FilterInt32(hydraidego.GreaterThan, 30).ForKey("Age"),
	)

	var results []*FilteredUserProfile
	err := h.ProfileReadBatchWithFilter(ctx, swampNames, filters, &FilteredUserProfile{}, 0,
		func(swampName name.Name, model any, err error) error {
			if err != nil {
				return nil
			}
			results = append(results, model.(*FilteredUserProfile))
			return nil
		})

	if err != nil {
		return nil, err
	}
	return results, nil
}
