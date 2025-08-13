package services

import (
	"context"
	"log"
	"time"

	"hydraideEmployee/internal/models"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// StartAuditService starts a background audit service that listens for changes
// to the employee index in Hydraide DB.
//
// This function subscribes to new and deleted employee events and logs them.
// It demonstrates how to use Hydraide's event-driven API for real-time monitoring.
//
// Params:
//   - r: Hydraide repository.
//
// Example:
//
//	go services.StartAuditService(repo)
func StartAuditService(r repo.Repo) {
	log.Println("[Audit Service] Starting...")

	// Use a background context for the subscription.
	ctx := context.Background()
	swampToIndex := name.New().Sanctuary("company_index").Realm("employee_ids").Swamp("all")

	// Subscribe to changes in the employee index.
	// The callback is called for each new or deleted employee.
	err := r.GetHydraidego().Subscribe(
		ctx,
		swampToIndex,
		false,
		models.EmployeeIndex{},
		func(model any, eventStatus hydraidego.EventStatus, err error) error {
			if err != nil {
				log.Printf("[Audit Service] Subscription error: %v", err)
				return nil
			}

			switch eventStatus {
			case hydraidego.StatusNew:
				// Log when a new employee is created.
				if entry, ok := model.(*models.EmployeeIndex); ok {
					log.Printf("[AUDIT LOG] New Employee Created - ID: %s at %s", entry.EmployeeID, time.Now().UTC().Format(time.RFC3339))
				}
			case hydraidego.StatusDeleted:
				// Log when an employee is deleted.
				if entry, ok := model.(*models.EmployeeIndex); ok {
					log.Printf("[AUDIT LOG] Employee Deleted - ID: %s at %s", entry.EmployeeID, time.Now().UTC().Format(time.RFC3339))
				}
			}
			return nil
		},
	)

	if err != nil {
		log.Printf("[Audit Service] Failed to start subscription: %v", err)
	}
}
