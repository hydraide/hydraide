// Package telemetry provides real-time monitoring and event collection for HydrAIDE.
// It captures all gRPC calls, errors, and client activity with time-based storage
// for replay functionality.
package telemetry

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// Event represents a single telemetry event from a gRPC call.
type Event struct {
	ID           string    // Unique event ID
	Timestamp    time.Time // When the call happened
	Method       string    // gRPC method name (Get, Set, Delete, etc.)
	SwampName    string    // Full swamp path (sanctuary/realm/swamp)
	Keys         []string  // Affected keys
	DurationMs   int64     // Call duration in milliseconds
	Success      bool      // Was the call successful
	ErrorCode    string    // gRPC error code (if error)
	ErrorMsg     string    // Error message (if error)
	ClientIP     string    // Client IP address
	RequestSize  int64     // Request payload size in bytes
	ResponseSize int64     // Response payload size in bytes
	HasDetails   bool      // True if detailed error info is available in ErrorStore
}

// Subscriber is a channel that receives telemetry events.
type Subscriber chan Event

// Collector defines the interface for telemetry collection.
type Collector interface {
	// Record adds a new telemetry event to the buffer.
	Record(event Event)

	// Subscribe registers a new subscriber to receive real-time events.
	// Returns a Subscriber channel and a function to unsubscribe.
	Subscribe(filter SubscribeFilter) (Subscriber, func())

	// GetHistory retrieves events within a time range.
	GetHistory(from, to time.Time, filter HistoryFilter) []Event

	// GetStats returns aggregated statistics for the given time window.
	GetStats(windowMinutes int) Stats

	// Close shuts down the collector and cleans up resources.
	Close()
}

// SubscribeFilter defines filters for real-time subscriptions.
type SubscribeFilter struct {
	ErrorsOnly       bool     // Only receive error events
	Methods          []string // Filter by method names (empty = all)
	SwampPattern     string   // Swamp name pattern filter (e.g., "auth/*")
	IncludeSuccesses bool     // Include successful calls (default: true)
}

// HistoryFilter defines filters for history queries.
type HistoryFilter struct {
	ErrorsOnly   bool     // Only return errors
	Methods      []string // Filter by method names
	SwampPattern string   // Swamp name pattern
	Limit        int      // Max events to return (0 = no limit)
}

// Stats contains aggregated telemetry statistics.
type Stats struct {
	TotalCalls    int64        // Total number of calls
	ErrorCount    int64        // Number of errors
	ErrorRate     float64      // Error percentage (0-100)
	AvgDurationMs float64      // Average call duration
	ActiveClients int          // Number of unique clients
	TopSwamps     []SwampStats // Top swamps by call count
	TopErrors     []ErrorStats // Most frequent errors
}

// SwampStats contains statistics for a specific swamp.
type SwampStats struct {
	SwampName     string  // Full swamp path
	CallCount     int64   // Number of calls
	ErrorCount    int64   // Number of errors
	AvgDurationMs float64 // Average duration
}

// ErrorStats contains statistics for a specific error type.
type ErrorStats struct {
	ErrorCode      string    // gRPC error code
	ErrorMessage   string    // Error message (truncated)
	Count          int64     // Number of occurrences
	LastSwamp      string    // Last swamp that had this error
	LastOccurrence time.Time // When it last occurred
}

// collector implements the Collector interface with a time-based ring buffer.
type collector struct {
	mu          sync.RWMutex
	events      []Event            // Ring buffer for events
	head        int                // Next write position
	count       int                // Current number of events
	capacity    int                // Maximum events to store
	retention   time.Duration      // How long to keep events
	subscribers map[string]subInfo // Active subscribers
	errorStore  *ErrorStore        // Detailed error information
	clientStats *ClientTracker     // Client activity tracking
	closed      bool
}

type subInfo struct {
	ch     Subscriber
	filter SubscribeFilter
}

// Config holds configuration for the telemetry collector.
type Config struct {
	// Capacity is the maximum number of events to store (default: 100000)
	Capacity int

	// Retention is how long to keep events (default: 30 minutes)
	Retention time.Duration

	// ErrorStoreCapacity is the max number of detailed errors to store (default: 10000)
	ErrorStoreCapacity int
}

// DefaultConfig returns the default telemetry configuration.
func DefaultConfig() Config {
	return Config{
		Capacity:           100000,
		Retention:          30 * time.Minute,
		ErrorStoreCapacity: 10000,
	}
}

// New creates a new telemetry collector with the given configuration.
func New(cfg Config) Collector {
	if cfg.Capacity <= 0 {
		cfg.Capacity = DefaultConfig().Capacity
	}
	if cfg.Retention <= 0 {
		cfg.Retention = DefaultConfig().Retention
	}
	if cfg.ErrorStoreCapacity <= 0 {
		cfg.ErrorStoreCapacity = DefaultConfig().ErrorStoreCapacity
	}

	c := &collector{
		events:      make([]Event, cfg.Capacity),
		capacity:    cfg.Capacity,
		retention:   cfg.Retention,
		subscribers: make(map[string]subInfo),
		errorStore:  NewErrorStore(cfg.ErrorStoreCapacity),
		clientStats: NewClientTracker(),
	}

	return c
}

// Record adds a new telemetry event to the buffer and notifies subscribers.
func (c *collector) Record(event Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	// Generate ID if not set
	if event.ID == "" {
		event.ID = uuid.New().String()
	}

	// Set timestamp if not set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Store in ring buffer
	c.events[c.head] = event
	c.head = (c.head + 1) % c.capacity
	if c.count < c.capacity {
		c.count++
	}

	// Track client activity
	c.clientStats.RecordActivity(event.ClientIP, event.Timestamp)

	// Store error details if this is an error
	if !event.Success && event.ErrorMsg != "" {
		event.HasDetails = true
		c.errorStore.Store(event)
	}

	// Notify subscribers (non-blocking)
	for _, sub := range c.subscribers {
		if c.matchesFilter(event, sub.filter) {
			select {
			case sub.ch <- event:
			default:
				// Subscriber is slow, skip this event
			}
		}
	}
}

// Subscribe registers a new subscriber to receive real-time events.
func (c *collector) Subscribe(filter SubscribeFilter) (Subscriber, func()) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		ch := make(Subscriber)
		close(ch)
		return ch, func() {}
	}

	id := uuid.New().String()
	ch := make(Subscriber, 100) // Buffer to prevent blocking

	c.subscribers[id] = subInfo{
		ch:     ch,
		filter: filter,
	}

	unsubscribe := func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if sub, ok := c.subscribers[id]; ok {
			close(sub.ch)
			delete(c.subscribers, id)
		}
	}

	return ch, unsubscribe
}

// GetHistory retrieves events within a time range.
func (c *collector) GetHistory(from, to time.Time, filter HistoryFilter) []Event {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []Event
	limit := filter.Limit
	if limit <= 0 {
		limit = c.capacity
	}

	// Iterate through the ring buffer
	for i := 0; i < c.count && len(result) < limit; i++ {
		// Calculate actual index (oldest to newest)
		idx := (c.head - c.count + i + c.capacity) % c.capacity
		event := c.events[idx]

		// Check time range
		if event.Timestamp.Before(from) || event.Timestamp.After(to) {
			continue
		}

		// Check retention
		if time.Since(event.Timestamp) > c.retention {
			continue
		}

		// Apply filters
		if filter.ErrorsOnly && event.Success {
			continue
		}

		if len(filter.Methods) > 0 && !contains(filter.Methods, event.Method) {
			continue
		}

		if filter.SwampPattern != "" && !matchPattern(filter.SwampPattern, event.SwampName) {
			continue
		}

		result = append(result, event)
	}

	return result
}

// GetStats returns aggregated statistics for the given time window.
func (c *collector) GetStats(windowMinutes int) Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cutoff := time.Now().Add(-time.Duration(windowMinutes) * time.Minute)

	var stats Stats
	swampCounts := make(map[string]*SwampStats)
	errorCounts := make(map[string]*ErrorStats)
	clientSet := make(map[string]struct{})
	var totalDuration int64

	for i := 0; i < c.count; i++ {
		idx := (c.head - c.count + i + c.capacity) % c.capacity
		event := c.events[idx]

		if event.Timestamp.Before(cutoff) {
			continue
		}

		stats.TotalCalls++
		totalDuration += event.DurationMs

		if !event.Success {
			stats.ErrorCount++

			// Track error stats
			key := event.ErrorCode + ":" + truncate(event.ErrorMsg, 50)
			if es, ok := errorCounts[key]; ok {
				es.Count++
				if event.Timestamp.After(es.LastOccurrence) {
					es.LastOccurrence = event.Timestamp
					es.LastSwamp = event.SwampName
				}
			} else {
				errorCounts[key] = &ErrorStats{
					ErrorCode:      event.ErrorCode,
					ErrorMessage:   truncate(event.ErrorMsg, 100),
					Count:          1,
					LastSwamp:      event.SwampName,
					LastOccurrence: event.Timestamp,
				}
			}
		}

		// Track swamp stats
		if event.SwampName != "" {
			if ss, ok := swampCounts[event.SwampName]; ok {
				ss.CallCount++
				if !event.Success {
					ss.ErrorCount++
				}
				// Running average
				ss.AvgDurationMs = (ss.AvgDurationMs*float64(ss.CallCount-1) + float64(event.DurationMs)) / float64(ss.CallCount)
			} else {
				swampCounts[event.SwampName] = &SwampStats{
					SwampName:     event.SwampName,
					CallCount:     1,
					ErrorCount:    boolToInt(!event.Success),
					AvgDurationMs: float64(event.DurationMs),
				}
			}
		}

		// Track unique clients
		if event.ClientIP != "" {
			clientSet[event.ClientIP] = struct{}{}
		}
	}

	// Calculate averages and rates
	if stats.TotalCalls > 0 {
		stats.AvgDurationMs = float64(totalDuration) / float64(stats.TotalCalls)
		stats.ErrorRate = float64(stats.ErrorCount) / float64(stats.TotalCalls) * 100
	}

	stats.ActiveClients = len(clientSet)

	// Get top swamps (by call count)
	stats.TopSwamps = topNSwamps(swampCounts, 5)

	// Get top errors (by count)
	stats.TopErrors = topNErrors(errorCounts, 5)

	return stats
}

// Close shuts down the collector and cleans up resources.
func (c *collector) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	c.closed = true

	// Close all subscriber channels
	for id, sub := range c.subscribers {
		close(sub.ch)
		delete(c.subscribers, id)
	}
}

// matchesFilter checks if an event matches a subscription filter.
func (c *collector) matchesFilter(event Event, filter SubscribeFilter) bool {
	if filter.ErrorsOnly && event.Success {
		return false
	}

	if !filter.IncludeSuccesses && event.Success {
		return false
	}

	if len(filter.Methods) > 0 && !contains(filter.Methods, event.Method) {
		return false
	}

	if filter.SwampPattern != "" && !matchPattern(filter.SwampPattern, event.SwampName) {
		return false
	}

	return true
}

// Helper functions

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func matchPattern(pattern, value string) bool {
	// Simple glob matching: supports * at the end
	if pattern == "" {
		return true
	}

	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(value) >= len(prefix) && value[:len(prefix)] == prefix
	}

	return pattern == value
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func topNSwamps(m map[string]*SwampStats, n int) []SwampStats {
	result := make([]SwampStats, 0, len(m))
	for _, v := range m {
		result = append(result, *v)
	}

	// Simple bubble sort for small n
	for i := 0; i < len(result) && i < n; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].CallCount > result[i].CallCount {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	if len(result) > n {
		result = result[:n]
	}
	return result
}

func topNErrors(m map[string]*ErrorStats, n int) []ErrorStats {
	result := make([]ErrorStats, 0, len(m))
	for _, v := range m {
		result = append(result, *v)
	}

	// Simple bubble sort for small n
	for i := 0; i < len(result) && i < n; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Count > result[i].Count {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	if len(result) > n {
		result = result[:n]
	}
	return result
}
