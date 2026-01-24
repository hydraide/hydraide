package telemetry

import (
	"sync"
	"time"
)

// ClientInfo contains information about a connected client.
type ClientInfo struct {
	IP           string    // Client IP address
	FirstSeen    time.Time // When the client first connected
	LastActivity time.Time // When the client was last active
	CallCount    int64     // Total number of calls
	ErrorCount   int64     // Total number of errors
}

// ClientTracker tracks client activity.
type ClientTracker struct {
	mu      sync.RWMutex
	clients map[string]*ClientInfo
}

// NewClientTracker creates a new client tracker.
func NewClientTracker() *ClientTracker {
	return &ClientTracker{
		clients: make(map[string]*ClientInfo),
	}
}

// RecordActivity records activity from a client.
func (ct *ClientTracker) RecordActivity(clientIP string, timestamp time.Time) {
	if clientIP == "" {
		return
	}

	ct.mu.Lock()
	defer ct.mu.Unlock()

	if client, ok := ct.clients[clientIP]; ok {
		client.LastActivity = timestamp
		client.CallCount++
	} else {
		ct.clients[clientIP] = &ClientInfo{
			IP:           clientIP,
			FirstSeen:    timestamp,
			LastActivity: timestamp,
			CallCount:    1,
		}
	}
}

// RecordError records an error from a client.
func (ct *ClientTracker) RecordError(clientIP string) {
	if clientIP == "" {
		return
	}

	ct.mu.Lock()
	defer ct.mu.Unlock()

	if client, ok := ct.clients[clientIP]; ok {
		client.ErrorCount++
	}
}

// GetActiveClients returns clients active within the given duration.
func (ct *ClientTracker) GetActiveClients(within time.Duration) []ClientInfo {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	cutoff := time.Now().Add(-within)
	result := make([]ClientInfo, 0)

	for _, client := range ct.clients {
		if client.LastActivity.After(cutoff) {
			result = append(result, *client)
		}
	}

	return result
}

// GetActiveClientCount returns the number of clients active within the given duration.
func (ct *ClientTracker) GetActiveClientCount(within time.Duration) int {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	cutoff := time.Now().Add(-within)
	count := 0

	for _, client := range ct.clients {
		if client.LastActivity.After(cutoff) {
			count++
		}
	}

	return count
}

// GetClient returns information about a specific client.
func (ct *ClientTracker) GetClient(clientIP string) (ClientInfo, bool) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	if client, ok := ct.clients[clientIP]; ok {
		return *client, true
	}
	return ClientInfo{}, false
}

// Cleanup removes clients inactive for longer than the given duration.
func (ct *ClientTracker) Cleanup(maxInactive time.Duration) int {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	cutoff := time.Now().Add(-maxInactive)
	removed := 0

	for ip, client := range ct.clients {
		if client.LastActivity.Before(cutoff) {
			delete(ct.clients, ip)
			removed++
		}
	}

	return removed
}

// GetTotalClients returns the total number of tracked clients.
func (ct *ClientTracker) GetTotalClients() int {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return len(ct.clients)
}

// GetTopClients returns the top N clients by call count.
func (ct *ClientTracker) GetTopClients(n int) []ClientInfo {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	clients := make([]ClientInfo, 0, len(ct.clients))
	for _, client := range ct.clients {
		clients = append(clients, *client)
	}

	// Sort by call count (descending)
	for i := 0; i < len(clients) && i < n; i++ {
		for j := i + 1; j < len(clients); j++ {
			if clients[j].CallCount > clients[i].CallCount {
				clients[i], clients[j] = clients[j], clients[i]
			}
		}
	}

	if len(clients) > n {
		clients = clients[:n]
	}

	return clients
}
