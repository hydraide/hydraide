package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"hydraideEmployee/internal/models"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// EmployeeHandler handles HTTP requests related to employee management.
//
// It uses a Hydraide repository to perform CRUD operations, search, and other
// actions on employee data stored in the Hydraide database.
//
// Example usage:
//
//	handler := &EmployeeHandler{Repo: repo}
//	router.HandleFunc("/employees", handler.CreateEmployee).Methods("POST")
type EmployeeHandler struct {
	// Repo is the Hydraide repository used for all DB operations.
	Repo repo.Repo
}

// CreateEmployee handles HTTP POST requests to create a new employee.
//
// It decodes the request body into an Employee struct, assigns a unique ID,
// sets the start date and active status, and saves the employee to Hydraide DB.
//
// It also updates the main index and search index for fast lookup.
//
// Example request (JSON):
//
//	POST /employees
//	{
//		"firstName": "Mock",
//		"lastName": "Arch",
//		"email": "mock@example.com",
//		"position": "Developer"
//	}
func (h *EmployeeHandler) CreateEmployee(w http.ResponseWriter, r *http.Request) {
	var emp models.Employee
	if err := json.NewDecoder(r.Body).Decode(&emp); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Assign a unique ID and set metadata.
	emp.ID = "emp-" + uuid.New().String()
	emp.StartDate = time.Now().UTC()
	emp.IsActive = true

	// Save the employee profile to Hydraide DB.
	if err := emp.Save(h.Repo); err != nil {
		http.Error(w, "Failed to save employee profile", http.StatusInternalServerError)
		return
	}

	// Add the employee to the main index for listing.
	if err := models.NewEmployeeIndex("").BulkAddToIndex(h.Repo, []string{emp.ID}); err != nil {
		http.Error(w, "Failed to update main index", http.StatusInternalServerError)
		return
	}

	// Add the employee to the search index for fast searching.
	if err := models.NewSearchIndex("").BulkAddToSearchIndex(h.Repo, []*models.Employee{&emp}); err != nil {
		log.Printf("Warning: Failed to add employee %s to search index: %v", emp.ID, err)
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(emp)
}

// GetEmployeeHandler handles HTTP GET requests for a specific employee by ID.
//
// It extracts the employee ID from the URL and delegates to GetEmployee.
//
// Example:
//
//	GET /employees/emp-1234
func (h *EmployeeHandler) GetEmployeeHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	h.GetEmployee(w, r, id)
}

// UpdateEmployeeHandler handles HTTP PUT requests to update an employee by ID.
//
// It extracts the employee ID from the URL and delegates to UpdateEmployee.
//
// Example:
//
//	PUT /employees/emp-1234
func (h *EmployeeHandler) UpdateEmployeeHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	h.UpdateEmployee(w, r, id)
}

// DeleteEmployeeHandler handles HTTP DELETE requests to remove an employee by ID.
//
// It extracts the employee ID from the URL and delegates to DeleteEmployee.
//
// Example:
//
//	DELETE /employees/emp-1234
func (h *EmployeeHandler) DeleteEmployeeHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	h.DeleteEmployee(w, r, id)
}

// GiveKudosHandler handles HTTP POST requests to give kudos to an employee.
//
// It extracts the employee ID from the URL and delegates to GiveKudos.
//
// Example:
//
//	POST /employees/emp-1234/kudos
func (h *EmployeeHandler) GiveKudosHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	h.GiveKudos(w, r, id)
}

func (h *EmployeeHandler) GetEmployee(w http.ResponseWriter, r *http.Request, id string) {
	emp := &models.Employee{ID: id}
	if err := emp.Load(h.Repo); err != nil {
		if hydraidego.IsSwampNotFound(err) {
			http.Error(w, "Employee not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to retrieve employee", http.StatusInternalServerError)
		}
		return
	}
	json.NewEncoder(w).Encode(emp)
}

func (h *EmployeeHandler) UpdateEmployee(w http.ResponseWriter, r *http.Request, id string) {
	existingEmp := &models.Employee{ID: id}
	if err := existingEmp.Load(h.Repo); err != nil {
		if hydraidego.IsSwampNotFound(err) {
			http.Error(w, "Employee not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to retrieve employee for update", http.StatusInternalServerError)
		}
		return
	}
	oldEmpData := *existingEmp

	var updates models.Employee
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Apply updates
	if updates.FirstName != "" {
		existingEmp.FirstName = updates.FirstName
	}
	if updates.LastName != "" {
		existingEmp.LastName = updates.LastName
	}
	if updates.Email != "" {
		existingEmp.Email = updates.Email
	}
	if updates.Position != "" {
		existingEmp.Position = updates.Position
	}
	existingEmp.IsActive = updates.IsActive

	if err := existingEmp.Save(h.Repo); err != nil {
		http.Error(w, "Failed to save updated employee", http.StatusInternalServerError)
		return
	}

	// Update search index by removing old tokens and adding new ones
	if err := models.NewSearchIndex("").BulkRemoveFromSearchIndex(h.Repo, []*models.Employee{&oldEmpData}); err != nil {
		log.Printf("Warning: Failed to remove old search tokens for %s: %v", id, err)
	}
	if err := models.NewSearchIndex("").BulkAddToSearchIndex(h.Repo, []*models.Employee{existingEmp}); err != nil {
		log.Printf("Warning: Failed to add new search tokens for %s: %v", id, err)
	}

	json.NewEncoder(w).Encode(existingEmp)
}

func (h *EmployeeHandler) DeleteEmployee(w http.ResponseWriter, r *http.Request, id string) {
	empToDelete := &models.Employee{ID: id}
	if err := empToDelete.Load(h.Repo); err == nil {
		if err := models.NewSearchIndex("").BulkRemoveFromSearchIndex(h.Repo, []*models.Employee{empToDelete}); err != nil {
			log.Printf("Warning: Failed to remove employee %s from search index: %v", id, err)
		}
	}

	if err := empToDelete.Destroy(h.Repo); err != nil && !hydraidego.IsSwampNotFound(err) {
		http.Error(w, "Failed to delete employee profile", http.StatusInternalServerError)
		return
	}

	if err := models.NewEmployeeIndex("").BulkRemoveFromIndex(h.Repo, []string{id}); err != nil && !hydraidego.IsNotFound(err) {
		http.Error(w, "Failed to remove employee from main index", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *EmployeeHandler) ListEmployees(w http.ResponseWriter, r *http.Request) {
	pageQuery := r.URL.Query().Get("page")
	limitQuery := r.URL.Query().Get("limit")

	page, err := strconv.Atoi(pageQuery)
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(limitQuery)
	if err != nil || limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	offset := (page - 1) * limit
	ids, totalItems, err := models.NewEmployeeIndex("").GetPaginatedIDs(h.Repo, offset, limit)
	if err != nil {
		http.Error(w, "Could not retrieve employee index", http.StatusInternalServerError)
		log.Printf("Error fetching paginated IDs: %v", err)
		return
	}

	employees := h.loadEmployeesInParallel(ids)

	totalPages := 0
	if totalItems > 0 {
		totalPages = (totalItems + limit - 1) / limit
	}

	response := struct {
		Data       []*models.Employee `json:"data"`
		Pagination struct {
			CurrentPage int `json:"currentPage"`
			PageSize    int `json:"pageSize"`
			TotalItems  int `json:"totalItems"`
			TotalPages  int `json:"totalPages"`
		} `json:"pagination"`
	}{
		Data: employees,
		Pagination: struct {
			CurrentPage int `json:"currentPage"`
			PageSize    int `json:"pageSize"`
			TotalItems  int `json:"totalItems"`
			TotalPages  int `json:"totalPages"`
		}{
			CurrentPage: page,
			PageSize:    limit,
			TotalItems:  totalItems,
			TotalPages:  totalPages,
		},
	}

	json.NewEncoder(w).Encode(response)
}

func (h *EmployeeHandler) SearchEmployees(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, `Query parameter "q" is required`, http.StatusBadRequest)
		return
	}

	ids, err := models.NewSearchIndex("").FindEmployeeIDsByToken(h.Repo, query)
	if err != nil {
		http.Error(w, "Error during search", http.StatusInternalServerError)
		return
	}

	employees := h.loadEmployeesInParallel(ids)
	json.NewEncoder(w).Encode(employees)
}

func (h *EmployeeHandler) BulkCreateEmployees(w http.ResponseWriter, r *http.Request) {
	var newEmployees []*models.Employee
	if err := json.NewDecoder(r.Body).Decode(&newEmployees); err != nil {
		http.Error(w, "Invalid request body: expected an array of employees", http.StatusBadRequest)
		return
	}

	var createdEmployees []*models.Employee
	var employeeIDs []string

	for _, emp := range newEmployees {
		emp.ID = "emp-" + uuid.New().String()
		emp.StartDate = time.Now().UTC()
		emp.IsActive = true
		if err := emp.Save(h.Repo); err != nil {
			log.Printf("Error saving employee %s: %v. Skipping.", emp.Email, err)
			continue
		}
		createdEmployees = append(createdEmployees, emp)
		employeeIDs = append(employeeIDs, emp.ID)
	}

	if err := models.NewEmployeeIndex("").BulkAddToIndex(h.Repo, employeeIDs); err != nil {
		http.Error(w, "Failed to bulk update main index", http.StatusInternalServerError)
		return
	}

	if err := models.NewSearchIndex("").BulkAddToSearchIndex(h.Repo, createdEmployees); err != nil {
		log.Printf("Warning: Failed to bulk add employees to search index: %v", err)
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(createdEmployees)
}

func (h *EmployeeHandler) BulkUpdateEmployees(w http.ResponseWriter, r *http.Request) {
	var updates []*models.Employee
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		http.Error(w, "Invalid request body: expected an array of employee updates", http.StatusBadRequest)
		return
	}

	var oldEmployeeData []*models.Employee
	var updatedEmployeeData []*models.Employee

	for _, update := range updates {
		if update.ID == "" {
			continue
		}

		existingEmp := &models.Employee{ID: update.ID}
		if err := existingEmp.Load(h.Repo); err != nil {
			log.Printf("Skipping update for non-existent employee ID: %s", update.ID)
			continue
		}

		oldEmployeeData = append(oldEmployeeData, &models.Employee{
			ID: existingEmp.ID, FirstName: existingEmp.FirstName, LastName: existingEmp.LastName,
			Email: existingEmp.Email, Position: existingEmp.Position,
		})

		// Apply updates
		if update.FirstName != "" {
			existingEmp.FirstName = update.FirstName
		}
		if update.LastName != "" {
			existingEmp.LastName = update.LastName
		}
		if update.Email != "" {
			existingEmp.Email = update.Email
		}
		if update.Position != "" {
			existingEmp.Position = update.Position
		}
		existingEmp.IsActive = update.IsActive

		if err := existingEmp.Save(h.Repo); err != nil {
			log.Printf("Error saving updated employee %s: %v", existingEmp.ID, err)
			continue
		}
		updatedEmployeeData = append(updatedEmployeeData, existingEmp)
	}

	if err := models.NewSearchIndex("").BulkRemoveFromSearchIndex(h.Repo, oldEmployeeData); err != nil {
		log.Printf("Warning: Failed to bulk remove old search tokens: %v", err)
	}
	if err := models.NewSearchIndex("").BulkAddToSearchIndex(h.Repo, updatedEmployeeData); err != nil {
		log.Printf("Warning: Failed to bulk add new search tokens: %v", err)
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "Bulk update process completed."})
}

func (h *EmployeeHandler) loadEmployeesInParallel(ids []string) []*models.Employee {
	var wg sync.WaitGroup
	employeeChan := make(chan *models.Employee, len(ids))

	for _, id := range ids {
		wg.Add(1)
		go func(employeeID string) {
			defer wg.Done()
			emp := &models.Employee{ID: employeeID}
			if err := emp.Load(h.Repo); err == nil {
				employeeChan <- emp
			} else {
				log.Printf("Failed to load employee %s: %v", employeeID, err)
			}
		}(id)
	}

	wg.Wait()
	close(employeeChan)

	employees := make([]*models.Employee, 0, len(ids))
	for emp := range employeeChan {
		employees = append(employees, emp)
	}
	return employees
}

func (h *EmployeeHandler) GiveKudos(w http.ResponseWriter, r *http.Request, id string) {
	emp := &models.Employee{ID: id}
	if err := emp.Load(h.Repo); err != nil {
		if hydraidego.IsSwampNotFound(err) {
			http.Error(w, "Employee not found", http.StatusNotFound)
		} else {
			log.Printf("Error retrieving employee %s before giving kudos: %v", id, err)
			http.Error(w, "Failed to retrieve employee", http.StatusInternalServerError)
		}
		return
	}

	kudosGiver := "system-user-007"

	newCount, err := emp.IncrementKudos(h.Repo, kudosGiver)
	if err != nil {
		log.Printf("Error incrementing kudos for employee %s: %v", id, err)
		http.Error(w, "Failed to give kudos due to a server error", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"employeeId":    id,
		"newKudosCount": newCount,
		"status":        "Kudos given successfully!",
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding kudos response: %v", err)
	}
}
