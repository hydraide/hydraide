package main

import (
	"log"
	"net/http"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/applications/app-employee/internal/handlers"
	"github.com/hydraide/hydraide/docs/sdk/go/examples/applications/app-employee/internal/services"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/applications/app-employee/internal/models"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/applications/app-employee/internal/db"

	"github.com/gorilla/mux"
)

// main is the entrypoint for the HydraideEmployee application.
//
// It initializes the Hydraide database connection, registers data patterns,
// starts the audit service, and sets up HTTP API endpoints for employee management.
//
// This is a good starting point for understanding how a Go application interacts
// with the Hydraide database and exposes RESTful APIs.
//
// Example:
//
//	Run the server and access endpoints like /employees, /search, etc.
func main() {
	// Initialize Hydraide DB connection and repository.
	db.Init()
	repo := db.GetRepo()

	// Register Hydraide data patterns for employees, index, and search.
	emp := &models.Employee{}
	if err := emp.RegisterEmployeePattern(repo); err != nil {
		log.Fatalf("Failed to register employee pattern: %v", err)
	}
	if err := models.NewEmployeeIndex("").RegisterIndexPattern(repo); err != nil {
		log.Fatalf("Failed to register index pattern: %v", err)
	}
	if err := models.NewSearchIndex("").RegisterSearchIndexPattern(repo); err != nil {
		log.Fatalf("Failed to register search index pattern: %v", err)
	}

	// Start the audit service in a background goroutine.
	go services.StartAuditService(repo)
	log.Println("Hydraide Swamp patterns registered successfully.")

	// Create the employee handler with the Hydraide repository.
	employeeHandler := &handlers.EmployeeHandler{Repo: repo}

	// Set up the HTTP router and endpoints.
	router := mux.NewRouter()

	// Search endpoint: Find employees by search token.
	router.HandleFunc("/search", employeeHandler.SearchEmployees).Methods(http.MethodGet)

	// Employee list and create endpoints.
	router.HandleFunc("/employees", employeeHandler.ListEmployees).Methods(http.MethodGet)
	router.HandleFunc("/employees", employeeHandler.CreateEmployee).Methods(http.MethodPost)

	// Bulk operations for creating and updating employees.
	router.HandleFunc("/employees/bulk", employeeHandler.BulkCreateEmployees).Methods(http.MethodPost)
	router.HandleFunc("/employees/bulk", employeeHandler.BulkUpdateEmployees).Methods(http.MethodPut)

	// Employee CRUD by ID.
	router.HandleFunc("/employees/{id}", employeeHandler.GetEmployeeHandler).Methods(http.MethodGet)
	router.HandleFunc("/employees/{id}", employeeHandler.UpdateEmployeeHandler).Methods(http.MethodPut)
	router.HandleFunc("/employees/{id}", employeeHandler.DeleteEmployeeHandler).Methods(http.MethodDelete)

	// Give kudos to an employee.
	router.HandleFunc("/employees/{id}/kudos", employeeHandler.GiveKudosHandler).Methods(http.MethodPost)

	port := ":8080"
	log.Printf("Server starting on port %s...", port)
	// Start the HTTP server.
	if err := http.ListenAndServe(port, router); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
