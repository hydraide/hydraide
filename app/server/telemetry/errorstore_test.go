package telemetry

import (
	"testing"
	"time"
)

func TestErrorStore_Store(t *testing.T) {
	store := NewErrorStore(100)

	event := Event{
		ID:        "test-id-1",
		Timestamp: time.Now(),
		Method:    "Get",
		SwampName: "test/swamp/one",
		Success:   false,
		ErrorCode: "Internal",
		ErrorMsg:  "decompression failed: invalid data",
		ClientIP:  "192.168.1.1",
	}

	store.Store(event)

	// Should be able to retrieve
	details, ok := store.Get("test-id-1")
	if !ok {
		t.Fatal("Expected to find error details")
	}

	if details.Event.ID != "test-id-1" {
		t.Errorf("Expected ID 'test-id-1', got '%s'", details.Event.ID)
	}

	if details.Category != CategoryDecompression {
		t.Errorf("Expected category '%s', got '%s'", CategoryDecompression, details.Category)
	}

	if details.StackTrace == "" {
		t.Error("Expected stack trace to be captured")
	}
}

func TestErrorStore_Capacity(t *testing.T) {
	store := NewErrorStore(10) // Small capacity

	// Store more errors than capacity
	for i := 0; i < 25; i++ {
		event := Event{
			ID:        "test-id-" + string(rune('a'+i)),
			Timestamp: time.Now(),
			Method:    "Get",
			Success:   false,
			ErrorCode: "Internal",
			ErrorMsg:  "test error",
		}
		store.Store(event)
	}

	// Oldest should be evicted
	_, ok := store.Get("test-id-a")
	if ok {
		t.Error("Expected oldest error to be evicted")
	}

	// Recent should still exist
	recent := store.GetRecent(5)
	if len(recent) != 5 {
		t.Errorf("Expected 5 recent errors, got %d", len(recent))
	}
}

func TestCategorizeError(t *testing.T) {
	tests := []struct {
		code     string
		msg      string
		expected string
	}{
		{"InvalidArgument", "missing required field", CategoryValidation},
		{"NotFound", "swamp not found", CategoryNotFound},
		{"PermissionDenied", "access denied", CategoryPermission},
		{"DeadlineExceeded", "timeout", CategoryTimeout},
		{"Internal", "decompression failed: invalid snappy data", CategoryDecompression},
		{"Internal", "compression error: lz4 failed", CategoryCompression},
		{"Internal", "unknown error", CategoryInternal},
		{"Unknown", "random error", CategoryUnknown},
	}

	for _, test := range tests {
		result := categorizeError(test.code, test.msg)
		if result != test.expected {
			t.Errorf("categorizeError(%q, %q) = %q, expected %q",
				test.code, test.msg, result, test.expected)
		}
	}
}

func TestErrorStore_GetByCategory(t *testing.T) {
	store := NewErrorStore(100)

	// Store errors of different categories
	errors := []struct {
		code string
		msg  string
	}{
		{"Internal", "decompression failed"},
		{"InvalidArgument", "missing field"},
		{"Internal", "decompress error"},
		{"NotFound", "swamp not found"},
		{"Internal", "compression failed"},
	}

	for i, e := range errors {
		event := Event{
			ID:        "test-" + string(rune('a'+i)),
			Timestamp: time.Now(),
			Success:   false,
			ErrorCode: e.code,
			ErrorMsg:  e.msg,
		}
		store.Store(event)
	}

	// Get decompression errors
	decompressionErrors := store.GetByCategory(CategoryDecompression, 10)
	if len(decompressionErrors) != 2 {
		t.Errorf("Expected 2 decompression errors, got %d", len(decompressionErrors))
	}

	// Get validation errors
	validationErrors := store.GetByCategory(CategoryValidation, 10)
	if len(validationErrors) != 1 {
		t.Errorf("Expected 1 validation error, got %d", len(validationErrors))
	}
}

func TestErrorStore_GetAggregated(t *testing.T) {
	store := NewErrorStore(100)

	// Store multiple similar errors
	for i := 0; i < 5; i++ {
		event := Event{
			ID:        "decomp-" + string(rune('a'+i)),
			Timestamp: time.Now(),
			SwampName: "test/swamp/one",
			Success:   false,
			ErrorCode: "Internal",
			ErrorMsg:  "decompression failed",
		}
		store.Store(event)
	}

	for i := 0; i < 3; i++ {
		event := Event{
			ID:        "valid-" + string(rune('a'+i)),
			Timestamp: time.Now(),
			SwampName: "test/swamp/two",
			Success:   false,
			ErrorCode: "InvalidArgument",
			ErrorMsg:  "missing field",
		}
		store.Store(event)
	}

	aggregated := store.GetAggregated()
	if len(aggregated) != 2 {
		t.Errorf("Expected 2 aggregated error types, got %d", len(aggregated))
	}

	// Find decompression errors
	var decompAgg *AggregatedError
	for i := range aggregated {
		if aggregated[i].Category == CategoryDecompression {
			decompAgg = &aggregated[i]
			break
		}
	}

	if decompAgg == nil {
		t.Fatal("Expected to find decompression aggregated error")
	}

	if decompAgg.Count != 5 {
		t.Errorf("Expected count 5, got %d", decompAgg.Count)
	}
}

func TestErrorStore_Cleanup(t *testing.T) {
	store := NewErrorStore(100)

	// Store old errors
	oldTime := time.Now().Add(-1 * time.Hour)
	for i := 0; i < 5; i++ {
		event := Event{
			ID:        "old-" + string(rune('a'+i)),
			Timestamp: oldTime,
			Success:   false,
			ErrorCode: "Internal",
			ErrorMsg:  "old error",
		}
		store.Store(event)
	}

	// Store recent errors
	for i := 0; i < 5; i++ {
		event := Event{
			ID:        "new-" + string(rune('a'+i)),
			Timestamp: time.Now(),
			Success:   false,
			ErrorCode: "Internal",
			ErrorMsg:  "new error",
		}
		store.Store(event)
	}

	// Cleanup errors older than 30 minutes
	removed := store.Cleanup(30 * time.Minute)
	if removed != 5 {
		t.Errorf("Expected to remove 5 old errors, removed %d", removed)
	}

	// Old errors should be gone
	_, ok := store.Get("old-a")
	if ok {
		t.Error("Expected old error to be cleaned up")
	}

	// New errors should still exist
	_, ok = store.Get("new-a")
	if !ok {
		t.Error("Expected new error to still exist")
	}
}
