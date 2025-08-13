package instancehealth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckHealth(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	t.Run("successful 200 OK", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		status, err := checkHealth(ctx, ts.URL)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if status != "healthy" {
			t.Fatalf("Expected status 'healthy', got: '%s'", status)
		}
	})

	t.Run("unhealthy 404 Not Found", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer ts.Close()

		status, err := checkHealth(ctx, ts.URL)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if status != "unhealthy" {
			t.Fatalf("Expected status 'unhealthy', got: '%s'", status)
		}
	})

	t.Run("unhealthy 500 Internal Server Error", func(t *testing.T) {
		// Create a mock server that returns 500
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()

		status, err := checkHealth(ctx, ts.URL)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if status != "unhealthy" {
			t.Fatalf("Expected status 'unhealthy', got: '%s'", status)
		}
	})

	t.Run("network request fails", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		ts.Close()

		status, err := checkHealth(ctx, ts.URL)
		if err == nil {
			t.Fatalf("Expected an error, got none.")
		}
		if status != "" {
			t.Fatalf("Expected an empty status, got: '%s'", status)
		}
	})
}

// Can be used as integration test - create a instance and replace the instance parameter.
// func TestGetStatus(t *testing.T) {
// 	instance := NewInstanceHealth()
// 	status := instance.GetHealthStatus(context.TODO(), "new-health")
// 	if status.Error != nil {
// 		t.Errorf("expected some status without error, got error %s", status.Error)
// 	}
// }
