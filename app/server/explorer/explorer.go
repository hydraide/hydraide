// Package explorer provides filesystem scanning and hierarchical indexing
// of HydrAIDE swamp files (.hyd). It reads swamp names from file headers
// (V3 fast path: ~100 bytes/file) and builds an in-memory index organized
// by Sanctuary / Realm / Swamp hierarchy.
//
// The Explorer can be used both server-side (via gRPC endpoints) and
// directly from the CLI (for local testing without a running server).
package explorer

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ScanState represents the current state of a scan operation.
type ScanState string

const (
	ScanStateIdle    ScanState = "idle"
	ScanStateRunning ScanState = "running"
	ScanStateDone    ScanState = "done"
	ScanStateError   ScanState = "error"
)

// ScanStatus holds the current scan status and statistics.
type ScanStatus struct {
	State        ScanState
	StartedAt    time.Time
	FinishedAt   time.Time
	TotalFiles   int64
	ScannedFiles int64
	ErrorCount   int64
	Duration     time.Duration
	Error        string
}

// SanctuaryInfo holds aggregated information about a Sanctuary.
type SanctuaryInfo struct {
	Name       string
	RealmCount int64
	SwampCount int64
	TotalSize  int64
}

// RealmInfo holds aggregated information about a Realm within a Sanctuary.
type RealmInfo struct {
	Sanctuary  string
	Name       string
	SwampCount int64
	TotalSize  int64
}

// SwampDetail holds detailed information about a single swamp file.
type SwampDetail struct {
	Sanctuary  string
	Realm      string
	Swamp      string
	FilePath   string
	FileSize   int64
	CreatedAt  time.Time
	ModifiedAt time.Time
	EntryCount uint64
	BlockCount uint64
	IslandID            string
	Version             uint16
	EstimatedMemorySize uint64
}

// SwampFilter specifies filtering and pagination for swamp listing.
type SwampFilter struct {
	Sanctuary   string
	Realm       string
	SwampPrefix string
	Offset      int64
	Limit       int64
}

// SwampListResult contains paginated swamp listing results.
type SwampListResult struct {
	Swamps []*SwampDetail
	Total  int64
	Offset int64
	Limit  int64
}

// SizeInfo holds aggregated size information at any hierarchy level.
type SizeInfo struct {
	Sanctuary string
	Realm     string
	Swamp     string
	TotalSize int64
	FileCount int64
}

// Explorer provides filesystem scanning and querying of swamp files.
type Explorer struct {
	dataPath string
	idx      *hierarchicalIndex

	scanMu     sync.Mutex
	scanStatus atomic.Value // *ScanStatus
}

// New creates a new Explorer for the given data directory path.
func New(dataPath string) *Explorer {
	e := &Explorer{
		dataPath: dataPath,
		idx:      newIndex(),
	}
	e.scanStatus.Store(&ScanStatus{State: ScanStateIdle})
	return e
}

// Scan starts a filesystem scan. Returns an error if a scan is already running.
// The scan runs in the foreground (blocks until complete).
func (e *Explorer) Scan(ctx context.Context) error {
	e.scanMu.Lock()
	current := e.scanStatus.Load().(*ScanStatus)
	if current.State == ScanStateRunning {
		e.scanMu.Unlock()
		return fmt.Errorf("scan already running")
	}

	status := &ScanStatus{
		State:     ScanStateRunning,
		StartedAt: time.Now(),
	}
	e.scanStatus.Store(status)
	e.scanMu.Unlock()

	// Clear old index
	e.idx.clear()

	// Run scanner
	err := e.scanDirectory(ctx, status)

	// Update final status
	status.FinishedAt = time.Now()
	status.Duration = status.FinishedAt.Sub(status.StartedAt)
	if err != nil {
		status.State = ScanStateError
		status.Error = err.Error()
	} else {
		status.State = ScanStateDone
	}
	e.scanStatus.Store(status)

	return err
}

// GetScanStatus returns the current scan status.
func (e *Explorer) GetScanStatus() *ScanStatus {
	return e.scanStatus.Load().(*ScanStatus)
}

// ListSanctuaries returns all sanctuaries sorted by name.
func (e *Explorer) ListSanctuaries() []*SanctuaryInfo {
	return e.idx.listSanctuaries()
}

// ListRealms returns all realms for a given sanctuary, sorted by name.
func (e *Explorer) ListRealms(sanctuary string) []*RealmInfo {
	return e.idx.listRealms(sanctuary)
}

// ListSwamps returns swamps matching the filter with pagination.
func (e *Explorer) ListSwamps(filter *SwampFilter) *SwampListResult {
	if filter.Limit <= 0 {
		filter.Limit = 100
	}
	if filter.Limit > 1000 {
		filter.Limit = 1000
	}
	return e.idx.listSwamps(filter)
}

// ListAllSwamps returns all swamps matching the given sanctuary and realm without pagination limits.
// If realm is empty, all swamps in the sanctuary are returned.
func (e *Explorer) ListAllSwamps(sanctuary, realm string) []*SwampDetail {
	return e.idx.listAllSwamps(sanctuary, realm)
}

// GetSwampDetail returns detailed information about a specific swamp.
func (e *Explorer) GetSwampDetail(sanctuary, realm, swamp string) (*SwampDetail, error) {
	return e.idx.getSwampDetail(sanctuary, realm, swamp)
}

// GetSize returns aggregated size information at any hierarchy level.
func (e *Explorer) GetSize(sanctuary, realm, swamp string) (*SizeInfo, error) {
	return e.idx.getSize(sanctuary, realm, swamp)
}

// GetDataPath returns the configured data path.
func (e *Explorer) GetDataPath() string {
	return e.dataPath
}
