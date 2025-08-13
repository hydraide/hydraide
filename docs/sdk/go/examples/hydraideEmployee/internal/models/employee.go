package models

import (
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

const (
	employeeSanctuary = "company"
	employeeRealm     = "employees"
)

// Employee represents an employee profile stored in the Hydraide database.
//
// Fields:
//   - ID: Unique identifier for the employee (generated automatically).
//   - FirstName, LastName: Employee's name.
//   - Email: Employee's email address.
//   - Position: Job title or position.
//   - StartDate: When the employee started (set automatically).
//   - IsActive: Whether the employee is currently active.
//
// Example:
//
//	emp := &Employee{
//		FirstName: "RAJ",
//		LastName: "Smith",
//		Email: "RAJ@example.com",
//		Position: "Developer",
//	}
type Employee struct {
	ID        string    `json:"id"`
	FirstName string    `json:"firstName"`
	LastName  string    `json:"lastName"`
	Email     string    `json:"email"`
	Position  string    `json:"position"`
	StartDate time.Time `json:"startDate"`
	IsActive  bool      `json:"isActive"`
}

// createSwampName builds the Hydraide "swamp" name for this employee.
// A swamp is a logical container for data in Hydraide DB.
func (e *Employee) createSwampName() name.Name {
	return name.New().Sanctuary(employeeSanctuary).Realm(employeeRealm).Swamp(e.ID)
}

// Save persists the employee profile to the Hydraide database.
//
// This uses the ProfileSave API, which stores the struct as a profile in Hydraide.
//
// Example:
//
//	err := emp.Save(repo)
func (e *Employee) Save(r repo.Repo) error {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()
	return r.GetHydraidego().ProfileSave(ctx, e.createSwampName(), e)
}

// Load retrieves the employee profile from the Hydraide database by ID.
//
// This uses the ProfileRead API, which loads the struct fields from Hydraide.
//
// Example:
//
//	emp := &Employee{ID: "emp-1234"}
//	err := emp.Load(repo)
func (e *Employee) Load(r repo.Repo) error {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()
	return r.GetHydraidego().ProfileRead(ctx, e.createSwampName(), e)
}

// Destroy deletes the employee profile from the Hydraide database.
//
// This uses the Destroy API, which removes the profile from Hydraide.
//
// Example:
//
//	err := emp.Destroy(repo)
func (e *Employee) Destroy(r repo.Repo) error {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()
	return r.GetHydraidego().Destroy(ctx, e.createSwampName())
}

// RegisterEmployeePattern registers the Hydraide pattern for employee profiles.
//
// Patterns define how data is stored and managed in Hydraide. This must be called
// before saving or loading employees.
//
// Example:
//
//	err := RegisterEmployeePattern(repo)
func RegisterEmployeePattern(r repo.Repo) error {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	pattern := name.New().Sanctuary(employeeSanctuary).Realm(employeeRealm).Swamp("*")

	req := &hydraidego.RegisterSwampRequest{
		SwampPattern:    pattern,
		CloseAfterIdle:  5 * time.Minute,
		IsInMemorySwamp: false,
		FilesystemSettings: &hydraidego.SwampFilesystemSettings{
			WriteInterval: 1 * time.Second,
			MaxFileSize:   1048576,
		},
	}

	errorResponses := r.GetHydraidego().RegisterSwamp(ctx, req)
	if errorResponses != nil {
		return hydraidehelper.ConcatErrors(errorResponses)
	}
	return nil
}

const KudosCounterKey = "KudosCount"

// IncrementKudos increases the kudos count for this employee in Hydraide DB.
//
// This uses the IncrementInt32 API, which atomically increments a counter
// associated with the employee. Metadata is set for auditing.
//
// Params:
//   - r: Hydraide repository.
//   - actor: The user or system giving kudos.
//
// Returns:
//   - newCount: The new kudos count after incrementing.
//   - error: Any error encountered.
//
// Example:
//
//	newCount, err := emp.IncrementKudos(repo, "admin-user")
func (e *Employee) IncrementKudos(r repo.Repo, actor string) (int32, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	createMeta := &hydraidego.IncrementMetaRequest{
		SetCreatedAt: true,  // Tell the server to set the creation timestamp.
		SetCreatedBy: actor, // Set the creator to our 'actor' string.
	}

	updateMeta := &hydraidego.IncrementMetaRequest{
		SetUpdatedAt: true,  // Tell the server to set the update timestamp.
		SetUpdatedBy: actor, // Set the updater to our 'actor' string.
	}

	newCount, _, err := r.GetHydraidego().IncrementInt32(
		ctx,
		e.createSwampName(),
		KudosCounterKey,
		1,
		nil,
		createMeta, // Metadata for creation
		updateMeta, // Metadata for update
	)

	if err != nil {
		return 0, err
	}

	return newCount, nil
}
