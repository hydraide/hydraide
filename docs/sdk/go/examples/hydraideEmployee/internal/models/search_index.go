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

// SearchIndex represents an entry in the search index in Hydraide DB.
//
// This struct is used to map search tokens to employee IDs for fast lookup.
//
// Field:
//   - EmployeeID: The unique ID of the employee (used as the key in Hydraide).
//
// Example:
//
//	searchIndex := NewSearchIndex("emp-1234")
//
// All methods for SearchIndex use pointer receivers for clarity and maintainability.
type SearchIndex struct {
	EmployeeID string `hydraide:"key"`
}

// NewSearchIndex initializes a new SearchIndex object.
func NewSearchIndex(employeeID string) *SearchIndex {
	return &SearchIndex{EmployeeID: employeeID}
}

// createSearchSwampName builds the Hydraide swamp name for a search token.
// Each token is stored in its own swamp for fast lookup.
func (si *SearchIndex) createSearchSwampName(token string) name.Name {
	// Simple token validation to prevent empty swamp names
	token = strings.TrimSpace(token)
	if token == "" {
		token = "empty"
	}
	return name.New().Sanctuary("company_search").Realm("employee_tokens").Swamp(token)
}

// TokenizeEmployee generates a list of unique search tokens from an employee's data.
//
// Example:
//
//	tokens := NewSearchIndex("").TokenizeEmployee(emp)
func (si *SearchIndex) TokenizeEmployee(emp *Employee) []string {
	re := regexp.MustCompile(`\w+`)
	fields := []string{
		emp.FirstName,
		emp.LastName,
		emp.Email,
		emp.Position,
	}
	tokenSet := make(map[string]struct{})
	for _, field := range fields {
		for _, token := range re.FindAllString(strings.ToLower(field), -1) {
			tokenSet[token] = struct{}{}
		}
	}
	tokens := make([]string, 0, len(tokenSet))
	for token := range tokenSet {
		tokens = append(tokens, token)
	}
	return tokens
}

// BulkAddToSearchIndex adds multiple employees to the search index in Hydraide DB.
//
// Example:
//
//	err := NewSearchIndex("").BulkAddToSearchIndex(repo, []*Employee{emp1, emp2})
func (si *SearchIndex) BulkAddToSearchIndex(r repo.Repo, employees []*Employee) error {
	tokenToEmpIDs := make(map[string][]string)
	for _, emp := range employees {
		tokens := si.TokenizeEmployee(emp)
		for _, token := range tokens {
			tokenToEmpIDs[token] = append(tokenToEmpIDs[token], emp.ID)
		}
	}

	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	for token, empIDs := range tokenToEmpIDs {
		models := make([]any, len(empIDs))
		for i, id := range empIDs {
			models[i] = &SearchIndex{EmployeeID: id}
		}
		req := &hydraidego.CatalogManyToManyRequest{
			SwampName: si.createSearchSwampName(token),
			Models:    models,
		}
		if err := r.GetHydraidego().CatalogSaveManyToMany(ctx, []*hydraidego.CatalogManyToManyRequest{req}, nil); err != nil {
			return err
		}
	}
	return nil
}

// BulkRemoveFromSearchIndex removes multiple employees from the search index in Hydraide DB.
//
// Example:
//
//	err := NewSearchIndex("").BulkRemoveFromSearchIndex(repo, []*Employee{emp1, emp2})
func (si *SearchIndex) BulkRemoveFromSearchIndex(r repo.Repo, employees []*Employee) error {
	tokenToEmpIDs := make(map[string][]string)
	for _, emp := range employees {
		tokens := si.TokenizeEmployee(emp)
		for _, token := range tokens {
			tokenToEmpIDs[token] = append(tokenToEmpIDs[token], emp.ID)
		}
	}

	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	for token, empIDs := range tokenToEmpIDs {
		req := &hydraidego.CatalogDeleteManyFromManyRequest{
			SwampName: si.createSearchSwampName(token),
			Keys:      empIDs,
		}
		if err := r.GetHydraidego().CatalogDeleteManyFromMany(ctx, []*hydraidego.CatalogDeleteManyFromManyRequest{req}, nil); err != nil {
			return err
		}
	}
	return nil
}

// FindEmployeeIDsByToken searches for employee IDs by a given token.
//
// Example:
//
//	ids, err := NewSearchIndex("").FindEmployeeIDsByToken(repo, "developer")
func (si *SearchIndex) FindEmployeeIDsByToken(r repo.Repo, token string) ([]string, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()
	var ids []string

	swampName := si.createSearchSwampName(strings.ToLower(token))
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
// Example:
//
//	err := NewSearchIndex("").RegisterSearchIndexPattern(repo)
func (si *SearchIndex) RegisterSearchIndexPattern(r repo.Repo) error {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	// Register a pattern for each possible token (for demo, register a generic one)
	req := &hydraidego.RegisterSwampRequest{
		SwampPattern:    si.createSearchSwampName("{token}"),
		CloseAfterIdle:  1 * time.Hour,
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
