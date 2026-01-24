// Package metadata provides persistent metadata storage for swamps.
package metadata

import (
	"time"

	"github.com/hydraide/hydraide/app/name"
)

// noopMetadata is a no-operation metadata implementation for V2 engine.
// It implements the Metadata interface but does nothing - all methods are no-ops.
// This is used when metadata is stored in the .hyd file instead of a separate meta file.
//
// The V2 chronicler handles all metadata persistence internally, so this dummy
// implementation prevents memory allocation and unnecessary operations.
type noopMetadata struct {
	swampName name.Name
	createdAt time.Time
}

// NewNoop creates a new no-operation metadata implementation for V2 engine.
// This is a lightweight dummy that satisfies the Metadata interface without
// allocating memory for metadata storage or performing any file I/O.
func NewNoop() Metadata {
	return &noopMetadata{
		createdAt: time.Now().UTC(),
	}
}

func (m *noopMetadata) LoadFromFile() {
	// No-op: V2 engine loads metadata from .hyd file
}

func (m *noopMetadata) SaveToFile() {
	// No-op: V2 engine saves metadata in .hyd file
}

func (m *noopMetadata) SetSwampName(swampName name.Name) {
	// Store in memory only - needed for GetSwampName() calls
	m.swampName = swampName
}

func (m *noopMetadata) GetSwampName() name.Name {
	return m.swampName
}

func (m *noopMetadata) GetCreatedAt() time.Time {
	return m.createdAt
}

func (m *noopMetadata) GetUpdatedAt() time.Time {
	// Return current time as V2 tracks this in the file header
	return time.Now().UTC()
}

func (m *noopMetadata) GetKey(key string) string {
	// No-op: V2 stores key-value pairs in .hyd file
	return ""
}

func (m *noopMetadata) SetKey(key, value string) {
	// No-op: V2 stores key-value pairs in .hyd file
}

func (m *noopMetadata) DeleteKey(key string) error {
	// No-op: V2 stores key-value pairs in .hyd file
	return nil
}

func (m *noopMetadata) SetUpdatedAt() {
	// No-op: V2 updates ModifiedAt in file header automatically
}

func (m *noopMetadata) Destroy() {
	// No-op: V2 handles file cleanup
}

func (m *noopMetadata) DisableFilePersistence() {
	// No-op: already disabled by design
}
