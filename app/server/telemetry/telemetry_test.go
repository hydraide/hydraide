package telemetry

import (
	"testing"
	"time"
)

func TestCollector_Record(t *testing.T) {
	cfg := Config{
		Capacity:           100,
		Retention:          5 * time.Minute,
		ErrorStoreCapacity: 50,
	}

	collector := New(cfg)
	defer collector.Close()

	// Record a successful event
	event := Event{
		Method:    "Get",
		SwampName: "test/swamp/one",
		Keys:      []string{"key1"},
		Success:   true,
		ClientIP:  "192.168.1.1",
	}

	collector.Record(event)

	// Get stats
	stats := collector.GetStats(5)
	if stats.TotalCalls != 1 {
		t.Errorf("Expected 1 call, got %d", stats.TotalCalls)
	}
	if stats.ErrorCount != 0 {
		t.Errorf("Expected 0 errors, got %d", stats.ErrorCount)
	}
}

func TestCollector_RecordError(t *testing.T) {
	cfg := Config{
		Capacity:           100,
		Retention:          5 * time.Minute,
		ErrorStoreCapacity: 50,
	}

	collector := New(cfg)
	defer collector.Close()

	// Record an error event
	event := Event{
		Method:    "Set",
		SwampName: "test/swamp/two",
		Keys:      []string{"key1"},
		Success:   false,
		ErrorCode: "Internal",
		ErrorMsg:  "decompression failed: invalid data",
		ClientIP:  "192.168.1.2",
	}

	collector.Record(event)

	// Get stats
	stats := collector.GetStats(5)
	if stats.TotalCalls != 1 {
		t.Errorf("Expected 1 call, got %d", stats.TotalCalls)
	}
	if stats.ErrorCount != 1 {
		t.Errorf("Expected 1 error, got %d", stats.ErrorCount)
	}
	if stats.ErrorRate != 100 {
		t.Errorf("Expected 100%% error rate, got %.2f%%", stats.ErrorRate)
	}
}

func TestCollector_Subscribe(t *testing.T) {
	cfg := Config{
		Capacity:           100,
		Retention:          5 * time.Minute,
		ErrorStoreCapacity: 50,
	}

	collector := New(cfg)
	defer collector.Close()

	// Subscribe
	filter := SubscribeFilter{
		IncludeSuccesses: true,
	}
	ch, unsubscribe := collector.Subscribe(filter)
	defer unsubscribe()

	// Record an event
	event := Event{
		Method:    "Get",
		SwampName: "test/swamp/one",
		Success:   true,
		ClientIP:  "192.168.1.1",
	}

	collector.Record(event)

	// Should receive the event
	select {
	case received := <-ch:
		if received.Method != "Get" {
			t.Errorf("Expected method 'Get', got '%s'", received.Method)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for event")
	}
}

func TestCollector_SubscribeErrorsOnly(t *testing.T) {
	cfg := Config{
		Capacity:           100,
		Retention:          5 * time.Minute,
		ErrorStoreCapacity: 50,
	}

	collector := New(cfg)
	defer collector.Close()

	// Subscribe to errors only
	filter := SubscribeFilter{
		ErrorsOnly:       true,
		IncludeSuccesses: true,
	}
	ch, unsubscribe := collector.Subscribe(filter)
	defer unsubscribe()

	// Record a successful event (should not be received)
	successEvent := Event{
		Method:    "Get",
		SwampName: "test/swamp/one",
		Success:   true,
		ClientIP:  "192.168.1.1",
	}
	collector.Record(successEvent)

	// Record an error event (should be received)
	errorEvent := Event{
		Method:    "Set",
		SwampName: "test/swamp/two",
		Success:   false,
		ErrorCode: "Internal",
		ErrorMsg:  "test error",
		ClientIP:  "192.168.1.2",
	}
	collector.Record(errorEvent)

	// Should receive only the error event
	select {
	case received := <-ch:
		if received.Success {
			t.Error("Should only receive error events")
		}
		if received.Method != "Set" {
			t.Errorf("Expected method 'Set', got '%s'", received.Method)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for error event")
	}
}

func TestCollector_GetHistory(t *testing.T) {
	cfg := Config{
		Capacity:           100,
		Retention:          5 * time.Minute,
		ErrorStoreCapacity: 50,
	}

	collector := New(cfg)
	defer collector.Close()

	now := time.Now()

	// Record several events
	for i := 0; i < 10; i++ {
		event := Event{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Method:    "Get",
			SwampName: "test/swamp/one",
			Success:   i%3 != 0, // Every 3rd is an error
			ClientIP:  "192.168.1.1",
		}
		if !event.Success {
			event.ErrorCode = "Internal"
			event.ErrorMsg = "test error"
		}
		collector.Record(event)
	}

	// Get all history
	events := collector.GetHistory(now.Add(-time.Minute), now.Add(time.Minute), HistoryFilter{})
	if len(events) != 10 {
		t.Errorf("Expected 10 events, got %d", len(events))
	}

	// Get only errors
	errors := collector.GetHistory(now.Add(-time.Minute), now.Add(time.Minute), HistoryFilter{
		ErrorsOnly: true,
	})
	if len(errors) != 4 { // 0, 3, 6, 9 are errors
		t.Errorf("Expected 4 errors, got %d", len(errors))
	}

	// Get with limit
	limited := collector.GetHistory(now.Add(-time.Minute), now.Add(time.Minute), HistoryFilter{
		Limit: 5,
	})
	if len(limited) != 5 {
		t.Errorf("Expected 5 events, got %d", len(limited))
	}
}

func TestCollector_RingBuffer(t *testing.T) {
	cfg := Config{
		Capacity:           10, // Small capacity for testing
		Retention:          5 * time.Minute,
		ErrorStoreCapacity: 10,
	}

	collector := New(cfg)
	defer collector.Close()

	now := time.Now()

	// Record more events than capacity
	for i := 0; i < 25; i++ {
		event := Event{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Method:    "Get",
			SwampName: "test/swamp/one",
			Success:   true,
			ClientIP:  "192.168.1.1",
		}
		collector.Record(event)
	}

	// Should only have last 10 events
	events := collector.GetHistory(now.Add(-time.Minute), now.Add(time.Hour), HistoryFilter{})
	if len(events) != 10 {
		t.Errorf("Expected 10 events (capacity), got %d", len(events))
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern  string
		value    string
		expected bool
	}{
		{"", "anything", true},
		{"exact", "exact", true},
		{"exact", "notexact", false},
		{"auth/*", "auth/tokens", true},
		{"auth/*", "auth/users/123", true},
		{"auth/*", "cache/tokens", false},
		{"user/sessions/*", "user/sessions/abc123", true},
	}

	for _, test := range tests {
		result := matchPattern(test.pattern, test.value)
		if result != test.expected {
			t.Errorf("matchPattern(%q, %q) = %v, expected %v",
				test.pattern, test.value, result, test.expected)
		}
	}
}

func TestCollector_Stats(t *testing.T) {
	cfg := Config{
		Capacity:           100,
		Retention:          5 * time.Minute,
		ErrorStoreCapacity: 50,
	}

	collector := New(cfg)
	defer collector.Close()

	// Record events from multiple clients and swamps
	for i := 0; i < 20; i++ {
		event := Event{
			Timestamp:  time.Now(),
			Method:     "Get",
			SwampName:  "test/swamp/" + string(rune('a'+i%5)),
			DurationMs: int64(i * 10),
			Success:    i%4 != 0,
			ClientIP:   "192.168.1." + string(rune('1'+i%3)),
		}
		if !event.Success {
			event.ErrorCode = "Internal"
			event.ErrorMsg = "test error"
		}
		collector.Record(event)
	}

	stats := collector.GetStats(5)

	if stats.TotalCalls != 20 {
		t.Errorf("Expected 20 calls, got %d", stats.TotalCalls)
	}

	if stats.ErrorCount != 5 { // 0, 4, 8, 12, 16 are errors
		t.Errorf("Expected 5 errors, got %d", stats.ErrorCount)
	}

	if len(stats.TopSwamps) == 0 {
		t.Error("Expected top swamps to be populated")
	}

	if stats.ActiveClients == 0 {
		t.Error("Expected active clients to be > 0")
	}
}
