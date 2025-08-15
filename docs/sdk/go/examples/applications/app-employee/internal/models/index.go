package models

import (
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

const (
	indexSanctuary = "company_index"
	indexRealm     = "employee_ids"
	indexSwamp     = "all"
)

// EmployeeIndex represents an entry in the employee index in Hydraide DB.
//
// This struct is used to keep track of all employee IDs for listing and pagination.
//
// Field:
//   - EmployeeID: The unique ID of the employee (used as the key in Hydraide).
//
// Example:
//
//	indexEntry := NewEmployeeIndex("emp-1234")
//
// All methods for EmployeeIndex use pointer receivers for clarity and maintainability.
type EmployeeIndex struct {
	EmployeeID string `hydraide:"key"`
}

// NewEmployeeIndex initializes a new EmployeeIndex object.
func NewEmployeeIndex(employeeID string) *EmployeeIndex {
	return &EmployeeIndex{EmployeeID: employeeID}
}

// createIndexSwampName builds the Hydraide swamp name for the employee index.
func (ei *EmployeeIndex) createIndexSwampName() name.Name {
	return name.New().Sanctuary(indexSanctuary).Realm(indexRealm).Swamp(indexSwamp)
}

// BulkAddToIndex adds multiple employee IDs to the main index in Hydraide DB.
//
// This is used to keep track of all employees for listing and pagination.
//
// Params:
//   - r: Hydraide repository.
//   - employeeIDs: Slice of employee IDs to add.
//
// Example:
//
//	err := NewEmployeeIndex("").BulkAddToIndex(repo, []string{"emp-1", "emp-2"})
func (ei *EmployeeIndex) BulkAddToIndex(r repo.Repo, employeeIDs []string) error {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	models := make([]any, len(employeeIDs))
	for i, id := range employeeIDs {
		models[i] = &EmployeeIndex{EmployeeID: id}
	}

	return r.GetHydraidego().CatalogSaveMany(ctx, ei.createIndexSwampName(), models, nil)
}

// BulkRemoveFromIndex removes multiple employee IDs from the main index in Hydraide DB.
//
// This is used when employees are deleted.
//
// Params:
//   - r: Hydraide repository.
//   - employeeIDs: Slice of employee IDs to remove.
//
// Example:
//
//	err := NewEmployeeIndex("").BulkRemoveFromIndex(repo, []string{"emp-1", "emp-2"})
func (ei *EmployeeIndex) BulkRemoveFromIndex(r repo.Repo, employeeIDs []string) error {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()
	return r.GetHydraidego().CatalogDeleteMany(ctx, ei.createIndexSwampName(), employeeIDs, nil)
}

// GetPaginatedIDs retrieves a paginated list of employee IDs from the index.
//
// This is used for implementing pagination in the employee list API.
//
// Params:
//   - r: Hydraide repository.
//   - offset: The starting index (zero-based).
//   - limit: The maximum number of IDs to return.
//
// Returns:
//   - ids: Slice of employee IDs.
//   - total: Total number of employees in the index.
//   - err: Any error encountered.
//
// Example:
//
//	ids, total, err := NewEmployeeIndex("").GetPaginatedIDs(repo, 0, 10)
func (ei *EmployeeIndex) GetPaginatedIDs(r repo.Repo, offset int, limit int) (ids []string, total int, err error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	swampName := ei.createIndexSwampName()

	totalCount, err := r.GetHydraidego().Count(ctx, swampName)
	if err != nil {
		if hydraidego.IsSwampNotFound(err) {
			return []string{}, 0, nil
		}
		return nil, 0, err
	}
	total = int(totalCount)

	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderAsc,
		From:       int32(offset),
		Limit:      int32(limit),
	}

	// Read employee IDs from the index using a callback.
	err = r.GetHydraidego().CatalogReadMany(ctx, swampName, index, EmployeeIndex{}, func(model any) error {
		if entry, ok := model.(*EmployeeIndex); ok {
			ids = append(ids, entry.EmployeeID)
		}
		return nil
	})

	if err != nil && !hydraidego.IsSwampNotFound(err) {
		return nil, 0, err
	}

	return ids, total, nil
}

// RegisterIndexPattern registers the Hydraide pattern for the employee index.
//
// This must be called before using the index for listing or pagination.
//
// Params:
//   - r: Hydraide repository.
//
// Example:
//
//	err := NewEmployeeIndex("").RegisterIndexPattern(repo)
func (ei *EmployeeIndex) RegisterIndexPattern(r repo.Repo) error {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	req := &hydraidego.RegisterSwampRequest{
		SwampPattern:    ei.createIndexSwampName(),
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
