package models

import (
	"regexp"
	"strings"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

const (
	searchIndexSanctuary = "search_index"
	searchIndexRealm     = "tokens"
)

// SearchIndex represents an entry in the employee search index in Hydraide DB.
//
// This struct is used to associate search tokens with employee IDs for fast lookup.
//
// Field:
//   - EmployeeID: The unique ID of the employee (used as the key in Hydraide).
//
// Example:
//
//	searchEntry := &SearchIndex{EmployeeID: "emp-1234"}
type SearchIndex struct {
	EmployeeID string `hydraide:"key"`
}

// createSearchSwampName builds the Hydraide swamp name for a search token.
// Each token is stored in its own swamp for fast lookup.
func createSearchSwampName(token string) name.Name {
	// Simple token validation to prevent empty swamp names
	if token == "" {
		token = "empty"
	}
	return name.New().Sanctuary(searchIndexSanctuary).Realm(searchIndexRealm).Swamp(token)
}

// TokenizeEmployee generates a list of unique search tokens from an employee's data.
//
// Tokens are extracted from first name, last name, email, and position.
// Only tokens longer than 2 characters are included.
//
// Params:
//   - emp: Pointer to Employee struct.
//
// Returns:
//   - []string: Slice of unique tokens.
//
// Example:
//
//	tokens := TokenizeEmployee(emp)
func TokenizeEmployee(emp *Employee) []string {
	re := regexp.MustCompile(`\w+`)
	fullText := strings.Join([]string{emp.FirstName, emp.LastName, emp.Email, emp.Position}, " ")
	tokens := re.FindAllString(strings.ToLower(fullText), -1)
	uniqueTokens := make(map[string]bool)
	for _, token := range tokens {
		if len(token) > 2 {
			uniqueTokens[token] = true
		}
	}
	result := make([]string, 0, len(uniqueTokens))
	for token := range uniqueTokens {
		result = append(result, token)
	}
	return result
}

// BulkAddToSearchIndex adds multiple employees to the search index in Hydraide DB.
//
// For each employee, tokens are generated and associated with their ID for fast search.
//
// Params:
//   - r: Hydraide repository.
//   - employees: Slice of Employee pointers to add.
//
// Example:
//
// err := BulkAddToSearchIndex(repo, []*Employee{emp1, emp2})
func BulkAddToSearchIndex(r repo.Repo, employees []*Employee) error {
	tokenToEmpIDs := make(map[string][]string)

	for _, emp := range employees {
		tokens := TokenizeEmployee(emp)
		for _, token := range tokens {
			tokenToEmpIDs[token] = append(tokenToEmpIDs[token], emp.ID)
		}
	}

	requests := make([]*hydraidego.CatalogManyToManyRequest, 0, len(tokenToEmpIDs))
	for token, empIDs := range tokenToEmpIDs {
		models := make([]any, len(empIDs))
		for i, empID := range empIDs {
			models[i] = &SearchIndex{EmployeeID: empID}
		}
		req := &hydraidego.CatalogManyToManyRequest{
			SwampName: createSearchSwampName(token),
			Models:    models,
		}
		requests = append(requests, req)
	}

	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()
	return r.GetHydraidego().CatalogSaveManyToMany(ctx, requests, nil)
}

// BulkRemoveFromSearchIndex removes multiple employees from the search index in Hydraide DB.
//
// For each employee, tokens are generated and their ID is removed from the associated swamps.
//
// Params:
//   - r: Hydraide repository.
//   - employees: Slice of Employee pointers to remove.
//
// Example:
//
//	err := BulkRemoveFromSearchIndex(repo, []*Employee{emp1, emp2})
func BulkRemoveFromSearchIndex(r repo.Repo, employees []*Employee) error {
	tokenToEmpIDs := make(map[string][]string)

	for _, emp := range employees {
		tokens := TokenizeEmployee(emp)
		for _, token := range tokens {
			tokenToEmpIDs[token] = append(tokenToEmpIDs[token], emp.ID)
		}
	}

	requests := make([]*hydraidego.CatalogDeleteManyFromManyRequest, 0, len(tokenToEmpIDs))
	for token, empIDs := range tokenToEmpIDs {
		req := &hydraidego.CatalogDeleteManyFromManyRequest{
			SwampName: createSearchSwampName(token),
			Keys:      empIDs,
		}
		requests = append(requests, req)
	}

	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()
	return r.GetHydraidego().CatalogDeleteManyFromMany(ctx, requests, nil)
}

// FindEmployeeIDsByToken searches for employee IDs by a given token.
//
// This is used to implement fast search functionality in the API.
//
// Params:
//   - r: Hydraide repository.
//   - token: The search token to look up.
//
// Returns:
//   - []string: Slice of matching employee IDs.
//   - error: Any error encountered.
//
// Example:
//
//	ids, err := FindEmployeeIDsByToken(repo, "developer")
func FindEmployeeIDsByToken(r repo.Repo, token string) ([]string, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	var ids []string
	swampName := createSearchSwampName(strings.ToLower(token))
	index := &hydraidego.Index{IndexType: hydraidego.IndexKey, IndexOrder: hydraidego.IndexOrderAsc, Limit: 0}

	err := r.GetHydraidego().CatalogReadMany(ctx, swampName, index, SearchIndex{}, func(model any) error {
		if entry, ok := model.(*SearchIndex); ok {
			ids = append(ids, entry.EmployeeID)
		}
		return nil
	})

	if err != nil && !hydraidego.IsSwampNotFound(err) {
		return nil, err
	}
	return ids, nil
}

// RegisterSearchIndexPattern registers the Hydraide pattern for the search index.
//
// This must be called before using the search index for fast lookup.
//
// Params:
//   - r: Hydraide repository.
//
// Example:
//
//	err := RegisterSearchIndexPattern(repo)
func RegisterSearchIndexPattern(r repo.Repo) error {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	pattern := name.New().Sanctuary(searchIndexSanctuary).Realm(searchIndexRealm).Swamp("*")

	req := &hydraidego.RegisterSwampRequest{
		SwampPattern:    pattern,
		CloseAfterIdle:  10 * time.Minute,
		IsInMemorySwamp: false,
		FilesystemSettings: &hydraidego.SwampFilesystemSettings{
			WriteInterval: 10 * time.Second,
			MaxFileSize:   8192,
		},
	}

	errorResponses := r.GetHydraidego().RegisterSwamp(ctx, req)
	if errorResponses != nil {
		return hydraidehelper.ConcatErrors(errorResponses)
	}
	return nil
}
