package telemetry

import (
	"runtime"
	"sync"
	"time"
)

// ErrorDetails contains detailed information about an error event.
type ErrorDetails struct {
	Event      Event             // The original telemetry event
	StackTrace string            // Full stack trace
	Category   string            // Categorized error type
	Context    map[string]string // Additional context
}

// ErrorStore stores detailed error information with stack traces.
type ErrorStore struct {
	mu       sync.RWMutex
	errors   map[string]ErrorDetails // Keyed by event ID
	order    []string                // Order of insertion for LRU eviction
	capacity int
}

// NewErrorStore creates a new error store with the given capacity.
func NewErrorStore(capacity int) *ErrorStore {
	if capacity <= 0 {
		capacity = 10000
	}
	return &ErrorStore{
		errors:   make(map[string]ErrorDetails),
		order:    make([]string, 0, capacity),
		capacity: capacity,
	}
}

// Store adds error details to the store.
func (es *ErrorStore) Store(event Event) {
	es.mu.Lock()
	defer es.mu.Unlock()

	// Capture stack trace
	stackTrace := captureStackTrace(3) // Skip this function and callers

	// Categorize the error
	category := categorizeError(event.ErrorCode, event.ErrorMsg)

	details := ErrorDetails{
		Event:      event,
		StackTrace: stackTrace,
		Category:   category,
		Context:    make(map[string]string),
	}

	// Add context
	if event.SwampName != "" {
		details.Context["swamp"] = event.SwampName
	}
	if len(event.Keys) > 0 {
		details.Context["keys"] = formatKeys(event.Keys)
	}
	details.Context["client_ip"] = event.ClientIP
	details.Context["duration_ms"] = formatInt64(event.DurationMs)

	// Store the error
	es.errors[event.ID] = details
	es.order = append(es.order, event.ID)

	// Evict oldest if over capacity
	for len(es.order) > es.capacity {
		oldestID := es.order[0]
		es.order = es.order[1:]
		delete(es.errors, oldestID)
	}
}

// Get retrieves error details by event ID.
func (es *ErrorStore) Get(eventID string) (ErrorDetails, bool) {
	es.mu.RLock()
	defer es.mu.RUnlock()

	details, ok := es.errors[eventID]
	return details, ok
}

// GetRecent returns the most recent errors up to the limit.
func (es *ErrorStore) GetRecent(limit int) []ErrorDetails {
	es.mu.RLock()
	defer es.mu.RUnlock()

	if limit <= 0 || limit > len(es.order) {
		limit = len(es.order)
	}

	result := make([]ErrorDetails, 0, limit)
	for i := len(es.order) - 1; i >= 0 && len(result) < limit; i-- {
		if details, ok := es.errors[es.order[i]]; ok {
			result = append(result, details)
		}
	}

	return result
}

// GetByCategory returns errors matching a specific category.
func (es *ErrorStore) GetByCategory(category string, limit int) []ErrorDetails {
	es.mu.RLock()
	defer es.mu.RUnlock()

	result := make([]ErrorDetails, 0)
	for i := len(es.order) - 1; i >= 0 && len(result) < limit; i-- {
		if details, ok := es.errors[es.order[i]]; ok {
			if details.Category == category {
				result = append(result, details)
			}
		}
	}

	return result
}

// GetAggregated returns aggregated error information.
func (es *ErrorStore) GetAggregated() []AggregatedError {
	es.mu.RLock()
	defer es.mu.RUnlock()

	aggMap := make(map[string]*AggregatedError)

	for _, details := range es.errors {
		key := details.Event.ErrorCode + ":" + details.Category
		if agg, ok := aggMap[key]; ok {
			agg.Count++
			if details.Event.Timestamp.After(agg.LastOccurrence) {
				agg.LastOccurrence = details.Event.Timestamp
				agg.LastSwamp = details.Event.SwampName
				agg.LastEventID = details.Event.ID
			}
		} else {
			aggMap[key] = &AggregatedError{
				ErrorCode:      details.Event.ErrorCode,
				Category:       details.Category,
				Message:        truncate(details.Event.ErrorMsg, 100),
				Count:          1,
				LastSwamp:      details.Event.SwampName,
				LastOccurrence: details.Event.Timestamp,
				LastEventID:    details.Event.ID,
			}
		}
	}

	result := make([]AggregatedError, 0, len(aggMap))
	for _, agg := range aggMap {
		result = append(result, *agg)
	}

	return result
}

// Cleanup removes errors older than the given duration.
func (es *ErrorStore) Cleanup(maxAge time.Duration) int {
	es.mu.Lock()
	defer es.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	newOrder := make([]string, 0, len(es.order))
	for _, id := range es.order {
		if details, ok := es.errors[id]; ok {
			if details.Event.Timestamp.Before(cutoff) {
				delete(es.errors, id)
				removed++
			} else {
				newOrder = append(newOrder, id)
			}
		}
	}

	es.order = newOrder
	return removed
}

// AggregatedError represents aggregated error information.
type AggregatedError struct {
	ErrorCode      string
	Category       string
	Message        string
	Count          int
	LastSwamp      string
	LastOccurrence time.Time
	LastEventID    string
}

// Error categories
const (
	CategoryCompression   = "compression"
	CategoryDecompression = "decompression"
	CategoryValidation    = "validation"
	CategoryNotFound      = "not_found"
	CategoryPermission    = "permission"
	CategoryTimeout       = "timeout"
	CategoryInternal      = "internal"
	CategoryUnknown       = "unknown"
)

// categorizeError determines the error category based on code and message.
func categorizeError(code, msg string) string {
	// Check message content first (more specific)
	msgLower := toLowerCase(msg)
	if containsAny(msgLower, "decompress", "decompression", "unpack") {
		return CategoryDecompression
	}
	if containsAny(msgLower, "compress", "compression") {
		return CategoryCompression
	}
	if containsAny(msgLower, "invalid", "validation", "required", "missing") {
		return CategoryValidation
	}
	if containsAny(msgLower, "not found", "notfound", "does not exist") {
		return CategoryNotFound
	}
	if containsAny(msgLower, "permission", "denied", "unauthorized", "forbidden") {
		return CategoryPermission
	}
	if containsAny(msgLower, "timeout", "deadline", "exceeded") {
		return CategoryTimeout
	}

	// Fall back to error code
	switch code {
	case "InvalidArgument":
		return CategoryValidation
	case "NotFound":
		return CategoryNotFound
	case "PermissionDenied", "Unauthenticated":
		return CategoryPermission
	case "DeadlineExceeded":
		return CategoryTimeout
	case "Internal":
		return CategoryInternal
	}

	return CategoryUnknown
}

// captureStackTrace captures the current stack trace.
func captureStackTrace(skip int) string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)

	// Skip the first few frames (runtime + our functions)
	stack := string(buf[:n])
	lines := splitLines(stack)

	// Skip header and requested frames
	startLine := 1 + (skip * 2) // Each frame is 2 lines
	if startLine >= len(lines) {
		return stack
	}

	result := ""
	for i := startLine; i < len(lines); i++ {
		result += lines[i] + "\n"
	}

	return result
}

// Helper functions

func formatKeys(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	if len(keys) == 1 {
		return keys[0]
	}

	result := keys[0]
	for i := 1; i < len(keys) && i < 5; i++ {
		result += ", " + keys[i]
	}
	if len(keys) > 5 {
		result += "..."
	}
	return result
}

func formatInt64(n int64) string {
	// Simple int64 to string without fmt
	if n == 0 {
		return "0"
	}

	negative := n < 0
	if negative {
		n = -n
	}

	digits := make([]byte, 0, 20)
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}

	// Reverse
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}

	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}

func toLowerCase(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}

func containsAny(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if containsSubstring(s, sub) {
			return true
		}
	}
	return false
}

func containsSubstring(s, sub string) bool {
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	lines := make([]string, 0)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
