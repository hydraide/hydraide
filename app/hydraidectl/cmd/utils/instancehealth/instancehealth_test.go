package instancehealth

import (
	"context"
	"fmt"
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

func getMockInstances(n int) []string {
	instances := make([]string, n)
	for i := range n {
		instances[i] = fmt.Sprintf("instance-%d", i)
	}
	return instances
}

// BenchmarkHealthChecks contains nested benchmarks for concurrent and sequential tests.
func BenchmarkHealthChecks(b *testing.B) {
	h := NewInstanceHealth()
	ctx := context.Background()

	testCases := []struct {
		name         string
		numInstances int
	}{
		{"2_Instances", 2},
		{"10_Instances", 10},
		{"100_Instances", 100},
		// {"1000_Instances", 1000},
	}

	// Group for the Concurrent benchmark
	b.Run("Concurrent", func(b *testing.B) {
		for _, tc := range testCases {
			b.Run(tc.name, func(b *testing.B) {
				instances := getMockInstances(tc.numInstances)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					h.GetListHealthStatus(ctx, instances)
				}
			})
		}
	})

	// Group for the Sequential benchmark
	b.Run("Sequential", func(b *testing.B) {
		for _, tc := range testCases {
			b.Run(tc.name, func(b *testing.B) {
				instances := getMockInstances(tc.numInstances)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					// Here we simulate the sequential call
					for _, instance := range instances {
						h.GetHealthStatus(ctx, instance)
					}
				}
			})
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

// func TestGetListStatus(t *testing.T) {
// 	instance := NewInstanceHealth()
// 	list := []string{}
// 	statuses := instance.GetListHealthStatus(context.TODO(), list)
// 	for _, status := range statuses {
// 		fmt.Println(status)
// 	}
// }
