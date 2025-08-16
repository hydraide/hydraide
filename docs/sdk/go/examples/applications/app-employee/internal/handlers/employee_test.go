// C:\go\src\hydraide\docs\sdk\go\examples\applications\app-employee\internal\handlers\employee_test.go
package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/applications/app-employee/internal/models"
	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
)

// #############################################################################
// Mocks Setup
// #############################################################################

// MockHydraidego provides a complete mock for the hydraidego.Hydraidego interface.
type MockHydraidego struct {
	mock.Mock
}

// MockRepo implements the repo.Repo interface for testing purposes.
type MockRepo struct {
	mock.Mock
}

func (m *MockRepo) GetHydraidego() hydraidego.Hydraidego {
	args := m.Called()
	return args.Get(0).(hydraidego.Hydraidego)
}

// --- Complete MockHydraidego Implementation ---

func (m *MockHydraidego) Heartbeat(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockHydraidego) RegisterSwamp(ctx context.Context, request *hydraidego.RegisterSwampRequest) []error {
	args := m.Called(ctx, request)
	if errs, ok := args.Get(0).([]error); ok {
		return errs
	}
	return nil
}

func (m *MockHydraidego) DeRegisterSwamp(ctx context.Context, swampName name.Name) []error {
	args := m.Called(ctx, swampName)
	if errs, ok := args.Get(0).([]error); ok {
		return errs
	}
	return nil
}

func (m *MockHydraidego) Lock(ctx context.Context, key string, ttl time.Duration) (string, error) {
	args := m.Called(ctx, key, ttl)
	return args.String(0), args.Error(1)
}

func (m *MockHydraidego) Unlock(ctx context.Context, key string, lockID string) error {
	args := m.Called(ctx, key, lockID)
	return args.Error(0)
}

func (m *MockHydraidego) IsSwampExist(ctx context.Context, swampName name.Name) (bool, error) {
	args := m.Called(ctx, swampName)
	return args.Bool(0), args.Error(1)
}

func (m *MockHydraidego) IsKeyExists(ctx context.Context, swampName name.Name, key string) (bool, error) {
	args := m.Called(ctx, swampName, key)
	return args.Bool(0), args.Error(1)
}

func (m *MockHydraidego) CatalogCreate(ctx context.Context, swampName name.Name, model any) error {
	args := m.Called(ctx, swampName, model)
	return args.Error(0)
}

func (m *MockHydraidego) CatalogCreateMany(ctx context.Context, swampName name.Name, models []any, iterator hydraidego.CreateManyIteratorFunc) error {
	args := m.Called(ctx, swampName, models, iterator)
	return args.Error(0)
}

func (m *MockHydraidego) CatalogCreateManyToMany(ctx context.Context, request []*hydraidego.CatalogManyToManyRequest, iterator hydraidego.CatalogCreateManyToManyIteratorFunc) error {
	args := m.Called(ctx, request, iterator)
	return args.Error(0)
}

func (m *MockHydraidego) CatalogRead(ctx context.Context, swampName name.Name, key string, model any) error {
	args := m.Called(ctx, swampName, key, model)
	return args.Error(0)
}

func (m *MockHydraidego) CatalogReadMany(ctx context.Context, swampName name.Name, index *hydraidego.Index, model any, iterator hydraidego.CatalogReadManyIteratorFunc) error {
	args := m.Called(ctx, swampName, index, model, iterator)
	if ids, ok := args.Get(0).([]string); ok {
		for _, id := range ids {
			iterator(&models.EmployeeIndex{EmployeeID: id})
		}
	}
	return args.Error(1)
}

func (m *MockHydraidego) CatalogUpdate(ctx context.Context, swampName name.Name, model any) error {
	args := m.Called(ctx, swampName, model)
	return args.Error(0)
}

func (m *MockHydraidego) CatalogUpdateMany(ctx context.Context, swampName name.Name, models []any, iterator hydraidego.CatalogUpdateManyIteratorFunc) error {
	args := m.Called(ctx, swampName, models, iterator)
	return args.Error(0)
}

func (m *MockHydraidego) CatalogDelete(ctx context.Context, swampName name.Name, key string) error {
	args := m.Called(ctx, swampName, key)
	return args.Error(0)
}

func (m *MockHydraidego) CatalogDeleteMany(ctx context.Context, swampName name.Name, keys []string, iterator hydraidego.CatalogDeleteIteratorFunc) error {
	args := m.Called(ctx, swampName, keys, iterator)
	return args.Error(0)
}

func (m *MockHydraidego) CatalogDeleteManyFromMany(ctx context.Context, request []*hydraidego.CatalogDeleteManyFromManyRequest, iterator hydraidego.CatalogDeleteIteratorFunc) error {
	args := m.Called(ctx, request, iterator)
	return args.Error(0)
}

func (m *MockHydraidego) CatalogSave(ctx context.Context, swampName name.Name, model any) (hydraidego.EventStatus, error) {
	args := m.Called(ctx, swampName, model)
	return args.Get(0).(hydraidego.EventStatus), args.Error(1)
}

func (m *MockHydraidego) CatalogSaveMany(ctx context.Context, swampName name.Name, models []any, iterator hydraidego.CatalogSaveManyIteratorFunc) error {
	args := m.Called(ctx, swampName, models, iterator)
	return args.Error(0)
}

func (m *MockHydraidego) CatalogSaveManyToMany(ctx context.Context, request []*hydraidego.CatalogManyToManyRequest, iterator hydraidego.CatalogSaveManyToManyIteratorFunc) error {
	args := m.Called(ctx, request, iterator)
	return args.Error(0)
}

func (m *MockHydraidego) CatalogShiftExpired(ctx context.Context, swampName name.Name, howMany int32, model any, iterator hydraidego.CatalogShiftExpiredIteratorFunc) error {
	args := m.Called(ctx, swampName, howMany, model, iterator)
	return args.Error(0)
}

func (m *MockHydraidego) ProfileSave(ctx context.Context, swampName name.Name, model any) error {
	args := m.Called(ctx, swampName, model)
	return args.Error(0)
}

func (m *MockHydraidego) ProfileRead(ctx context.Context, name name.Name, model any) error {
	args := m.Called(ctx, name, model)
	if args.Get(0) != nil {
		emp, ok := model.(*models.Employee)
		if ok {
			sourceEmp, ok2 := args.Get(0).(*models.Employee)
			if ok2 {
				*emp = *sourceEmp
			}
		}
	}
	return args.Error(1)
}

func (m *MockHydraidego) Count(ctx context.Context, swampName name.Name) (int32, error) {
	args := m.Called(ctx, swampName)
	return int32(args.Int(0)), args.Error(1)
}

func (m *MockHydraidego) Destroy(ctx context.Context, swampName name.Name) error {
	args := m.Called(ctx, swampName)
	return args.Error(0)
}

func (m *MockHydraidego) Subscribe(ctx context.Context, swampName name.Name, getExistingData bool, model any, iterator hydraidego.SubscribeIteratorFunc) error {
	args := m.Called(ctx, swampName, getExistingData, model, iterator)
	return args.Error(0)
}

// --- Mock Implementations for Increment functions ---
func (m *MockHydraidego) IncrementInt8(ctx context.Context, swampName name.Name, key string, value int8, condition *hydraidego.Int8Condition, setIfNotExist *hydraidego.IncrementMetaRequest, setIfExist *hydraidego.IncrementMetaRequest) (int8, *hydraidego.IncrementMetaResponse, error) {
	args := m.Called(ctx, swampName, key, value, condition, setIfNotExist, setIfExist)
	return int8(args.Int(0)), nil, args.Error(1)
}
func (m *MockHydraidego) IncrementInt16(ctx context.Context, swampName name.Name, key string, value int16, condition *hydraidego.Int16Condition, setIfNotExist *hydraidego.IncrementMetaRequest, setIfExist *hydraidego.IncrementMetaRequest) (int16, *hydraidego.IncrementMetaResponse, error) {
	args := m.Called(ctx, swampName, key, value, condition, setIfNotExist, setIfExist)
	return int16(args.Int(0)), nil, args.Error(1)
}
func (m *MockHydraidego) IncrementInt32(ctx context.Context, swampName name.Name, key string, value int32, condition *hydraidego.Int32Condition, setIfNotExist *hydraidego.IncrementMetaRequest, setIfExist *hydraidego.IncrementMetaRequest) (int32, *hydraidego.IncrementMetaResponse, error) {
	args := m.Called(ctx, swampName, key, value, condition, setIfNotExist, setIfExist)
	return int32(args.Int(0)), nil, args.Error(1)
}
func (m *MockHydraidego) IncrementInt64(ctx context.Context, swampName name.Name, key string, value int64, condition *hydraidego.Int64Condition, setIfNotExist *hydraidego.IncrementMetaRequest, setIfExist *hydraidego.IncrementMetaRequest) (int64, *hydraidego.IncrementMetaResponse, error) {
	args := m.Called(ctx, swampName, key, value, condition, setIfNotExist, setIfExist)
	return int64(args.Int(0)), nil, args.Error(1)
}
func (m *MockHydraidego) IncrementUint8(ctx context.Context, swampName name.Name, key string, value uint8, condition *hydraidego.Uint8Condition, setIfNotExist *hydraidego.IncrementMetaRequest, setIfExist *hydraidego.IncrementMetaRequest) (uint8, *hydraidego.IncrementMetaResponse, error) {
	args := m.Called(ctx, swampName, key, value, condition, setIfNotExist, setIfExist)
	return uint8(args.Int(0)), nil, args.Error(1)
}
func (m *MockHydraidego) IncrementUint16(ctx context.Context, swampName name.Name, key string, value uint16, condition *hydraidego.Uint16Condition, setIfNotExist *hydraidego.IncrementMetaRequest, setIfExist *hydraidego.IncrementMetaRequest) (uint16, *hydraidego.IncrementMetaResponse, error) {
	args := m.Called(ctx, swampName, key, value, condition, setIfNotExist, setIfExist)
	return uint16(args.Int(0)), nil, args.Error(1)
}
func (m *MockHydraidego) IncrementUint32(ctx context.Context, swampName name.Name, key string, value uint32, condition *hydraidego.Uint32Condition, setIfNotExist *hydraidego.IncrementMetaRequest, setIfExist *hydraidego.IncrementMetaRequest) (uint32, *hydraidego.IncrementMetaResponse, error) {
	args := m.Called(ctx, swampName, key, value, condition, setIfNotExist, setIfExist)
	return uint32(args.Int(0)), nil, args.Error(1)
}
func (m *MockHydraidego) IncrementUint64(ctx context.Context, swampName name.Name, key string, value uint64, condition *hydraidego.Uint64Condition, setIfNotExist *hydraidego.IncrementMetaRequest, setIfExist *hydraidego.IncrementMetaRequest) (uint64, *hydraidego.IncrementMetaResponse, error) {
	args := m.Called(ctx, swampName, key, value, condition, setIfNotExist, setIfExist)
	return uint64(args.Int(0)), nil, args.Error(1)
}
func (m *MockHydraidego) IncrementFloat32(ctx context.Context, swampName name.Name, key string, value float32, condition *hydraidego.Float32Condition, setIfNotExist *hydraidego.IncrementMetaRequest, setIfExist *hydraidego.IncrementMetaRequest) (float32, *hydraidego.IncrementMetaResponse, error) {
	args := m.Called(ctx, swampName, key, value, condition, setIfNotExist, setIfExist)
	// *** FIX: Use Get() with a type assertion to retrieve the float value. ***
	return args.Get(0).(float32), nil, args.Error(1)
}
func (m *MockHydraidego) IncrementFloat64(ctx context.Context, swampName name.Name, key string, value float64, condition *hydraidego.Float64Condition, setIfNotExist *hydraidego.IncrementMetaRequest, setIfExist *hydraidego.IncrementMetaRequest) (float64, *hydraidego.IncrementMetaResponse, error) {
	args := m.Called(ctx, swampName, key, value, condition, setIfNotExist, setIfExist)
	// *** FIX: Use Get() with a type assertion to retrieve the float value. ***
	return args.Get(0).(float64), nil, args.Error(1)
}

// --- Mock Implementations for Uint32Slice functions ---
func (m *MockHydraidego) Uint32SlicePush(ctx context.Context, swampName name.Name, KeyValuesPair []*hydraidego.KeyValuesPair) error {
	args := m.Called(ctx, swampName, KeyValuesPair)
	return args.Error(0)
}
func (m *MockHydraidego) Uint32SliceDelete(ctx context.Context, swampName name.Name, KeyValuesPair []*hydraidego.KeyValuesPair) error {
	args := m.Called(ctx, swampName, KeyValuesPair)
	return args.Error(0)
}
func (m *MockHydraidego) Uint32SliceSize(ctx context.Context, swampName name.Name, key string) (int64, error) {
	args := m.Called(ctx, swampName, key)
	return int64(args.Int(0)), args.Error(1)
}
func (m *MockHydraidego) Uint32SliceIsValueExist(ctx context.Context, swampName name.Name, key string, value uint32) (bool, error) {
	args := m.Called(ctx, swampName, key, value)
	return args.Bool(0), args.Error(1)
}

// Helper to create a sample employee for tests.
func mockEmployee() *models.Employee {
	return &models.Employee{
		ID:        "emp-" + uuid.New().String(),
		FirstName: "Alice",
		LastName:  "Smith",
		Email:     "alice.smith@example.com",
		Position:  "Engineer",
		StartDate: time.Now().UTC(),
		IsActive:  true,
	}
}

// #############################################################################
// Test Cases
// #############################################################################

func TestCreateEmployee(t *testing.T) {
	mockRepo := new(MockRepo)
	mockHydra := new(MockHydraidego)
	mockRepo.On("GetHydraidego").Return(mockHydra)
	handler := &EmployeeHandler{Repo: mockRepo}

	// Mock expectations
	mockHydra.On("ProfileSave", mock.Anything, mock.Anything, mock.AnythingOfType("*models.Employee")).Return(nil)
	mockHydra.On("CatalogSaveMany", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockHydra.On("CatalogSaveManyToMany", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	emp := &models.Employee{FirstName: "Alice", LastName: "Smith", Email: "alice@example.com", Position: "Engineer"}
	body, _ := json.Marshal(emp)
	req, _ := http.NewRequest(http.MethodPost, "/employees", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.CreateEmployee(rr, req)
	assert.Equal(t, http.StatusCreated, rr.Code)
	assert.Contains(t, rr.Body.String(), "alice@example.com")
}

func TestGetEmployee_Positive(t *testing.T) {
	mockRepo := new(MockRepo)
	mockHydra := new(MockHydraidego)
	mockRepo.On("GetHydraidego").Return(mockHydra)
	handler := &EmployeeHandler{Repo: mockRepo}

	emp := mockEmployee()
	mockHydra.On("ProfileRead", mock.Anything, mock.Anything, mock.AnythingOfType("*models.Employee")).Return(emp, nil)

	req, _ := http.NewRequest(http.MethodGet, "/employees/"+emp.ID, nil)
	rr := httptest.NewRecorder()

	router := mux.NewRouter()
	router.HandleFunc("/employees/{id}", handler.GetEmployeeHandler)
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), emp.ID)
}

func TestGetEmployee_NotFound(t *testing.T) {
	mockRepo := new(MockRepo)
	mockHydra := new(MockHydraidego)
	mockRepo.On("GetHydraidego").Return(mockHydra)
	handler := &EmployeeHandler{Repo: mockRepo}

	notFoundErr := hydraidego.NewError(hydraidego.ErrCodeSwampNotFound, "swamp not found")
	mockHydra.On("ProfileRead", mock.Anything, mock.Anything, mock.Anything).Return(nil, notFoundErr)

	req, _ := http.NewRequest(http.MethodGet, "/employees/non-existent-id", nil)
	rr := httptest.NewRecorder()

	router := mux.NewRouter()
	router.HandleFunc("/employees/{id}", handler.GetEmployeeHandler)
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestUpdateEmployee(t *testing.T) {
	mockRepo := new(MockRepo)
	mockHydra := new(MockHydraidego)
	mockRepo.On("GetHydraidego").Return(mockHydra)
	handler := &EmployeeHandler{Repo: mockRepo}

	existingEmp := mockEmployee()
	updatePayload := map[string]string{"position": "Senior Engineer"}
	body, _ := json.Marshal(updatePayload)

	mockHydra.On("ProfileRead", mock.Anything, mock.Anything, mock.Anything).Return(existingEmp, nil)
	mockHydra.On("ProfileSave", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockHydra.On("CatalogDeleteManyFromMany", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockHydra.On("CatalogSaveManyToMany", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	req, _ := http.NewRequest(http.MethodPut, "/employees/"+existingEmp.ID, bytes.NewReader(body))
	rr := httptest.NewRecorder()

	router := mux.NewRouter()
	router.HandleFunc("/employees/{id}", handler.UpdateEmployeeHandler)
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "Senior Engineer")
}

func TestDeleteEmployee(t *testing.T) {
	mockRepo := new(MockRepo)
	mockHydra := new(MockHydraidego)
	mockRepo.On("GetHydraidego").Return(mockHydra)
	handler := &EmployeeHandler{Repo: mockRepo}

	empToDelete := mockEmployee()

	mockHydra.On("ProfileRead", mock.Anything, mock.Anything, mock.Anything).Return(empToDelete, nil)
	mockHydra.On("CatalogDeleteManyFromMany", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockHydra.On("Destroy", mock.Anything, mock.Anything).Return(nil)
	mockHydra.On("CatalogDeleteMany", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	req, _ := http.NewRequest(http.MethodDelete, "/employees/"+empToDelete.ID, nil)
	rr := httptest.NewRecorder()

	router := mux.NewRouter()
	router.HandleFunc("/employees/{id}", handler.DeleteEmployeeHandler)
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
}

func TestGiveKudos(t *testing.T) {
	mockRepo := new(MockRepo)
	mockHydra := new(MockHydraidego)
	mockRepo.On("GetHydraidego").Return(mockHydra)
	handler := &EmployeeHandler{Repo: mockRepo}

	emp := mockEmployee()

	mockHydra.On("ProfileRead", mock.Anything, mock.Anything, mock.Anything).Return(emp, nil)
	// For the IncrementInt32, we need to return the expected new count (1) and a nil error.
	mockHydra.On("IncrementInt32", mock.Anything, mock.Anything, "KudosCount", int32(1), mock.Anything, mock.Anything, mock.Anything).Return(1, nil)

	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/employees/%s/kudos", emp.ID), nil)
	rr := httptest.NewRecorder()

	router := mux.NewRouter()
	router.HandleFunc("/employees/{id}/kudos", handler.GiveKudosHandler)
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"newKudosCount":1`)
}

func TestListEmployees(t *testing.T) {
	mockRepo := new(MockRepo)
	mockHydra := new(MockHydraidego)
	mockRepo.On("GetHydraidego").Return(mockHydra)
	handler := &EmployeeHandler{Repo: mockRepo}

	emp1 := mockEmployee()
	emp2 := mockEmployee()
	ids := []string{emp1.ID, emp2.ID}

	mockHydra.On("Count", mock.Anything, mock.Anything).Return(len(ids), nil)
	mockHydra.On("CatalogReadMany", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(ids, nil)

	mockHydra.On("ProfileRead", mock.Anything, mock.MatchedBy(func(n name.Name) bool { return n.Get() == "company/employees/"+emp1.ID }), mock.Anything).Return(emp1, nil)
	mockHydra.On("ProfileRead", mock.Anything, mock.MatchedBy(func(n name.Name) bool { return n.Get() == "company/employees/"+emp2.ID }), mock.Anything).Return(emp2, nil)

	req, _ := http.NewRequest(http.MethodGet, "/employees?page=1&limit=2", nil)
	rr := httptest.NewRecorder()

	handler.ListEmployees(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), emp1.ID)
	assert.Contains(t, rr.Body.String(), emp2.ID)
	assert.Contains(t, rr.Body.String(), `"totalItems":2`)
}
