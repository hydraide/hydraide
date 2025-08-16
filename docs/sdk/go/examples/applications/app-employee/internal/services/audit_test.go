package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/applications/app-employee/internal/models"
	"github.com/hydraide/hydraide/sdk/go/hydraidego"
)

// MockHydraidego implements a mock for hydraidego.Hydraidego
type MockHydraidego struct {
	mock.Mock
}

func (m *MockHydraidego) Subscribe(ctx context.Context, name interface{}, b bool, model interface{}, cb func(any, hydraidego.EventStatus, error) error) error {
	// Simulate StatusNew event
	cb(&models.EmployeeIndex{EmployeeID: "emp-1"}, hydraidego.StatusNew, nil)
	// Simulate StatusDeleted event
	cb(&models.EmployeeIndex{EmployeeID: "emp-2"}, hydraidego.StatusDeleted, nil)
	return nil
}

// MockRepo implements the repo.Repo interface for testing.
type MockRepo struct {
	mock.Mock
}

func (m *MockRepo) GetHydraidego() hydraidego.Hydraidego {
	args := m.Called()
	return args.Get(0).(hydraidego.Hydraidego)
}

func TestStartAuditService_Positive(t *testing.T) {
	mockRepo := new(MockRepo)
	mockHydra := new(MockHydraidego)
	mockRepo.On("GetHydraidego").Return(mockHydra)

	// Should not panic or error
	StartAuditService(mockRepo)
	assert.True(t, true)
}
