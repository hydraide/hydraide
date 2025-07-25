// Package metadata provides persistent metadata storage for swamps.
// It tracks when each swamp was created, when its data was last updated,
// and what path or identifier is used to access it.
//
// It also allows storing custom key-value metadata alongside each swamp,
// which can be used by the client based on their business logic.
//
// For example, a client may store access control rules, such as
// what roles can access the swamp, or which users have permission —
// and build custom logic on top of that if needed.
package metadata

import (
	"encoding/gob"
	"github.com/hydraide/hydraide/app/name"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const MetaFile = "meta"

type Meta struct {
	// The full name of the swamp, used for reverse lookup even if accessed through a hashed folder path. Variable length.
	SwampName string
	// CreatedAt is the timestamp when the swamp was initially created.
	CreatedAt time.Time
	// UpdatedAt is the timestamp of the last data modification in the swamp.
	UpdatedAt time.Time
	// BackupAt is the timestamp of the last backup operation.
	// TODO: Make this persistable for our upcoming backup system.
	BackupAt time.Time
	// KeyValuePairs are custom metadata entries that can only be set internally by the system.
	KeyValuePairs map[string]string
}

type Metadata interface {
	// LoadFromFile loads the metadata from a file into memory.
	LoadFromFile()

	// SaveToFile persists the metadata to disk — but only if it has changed — and then frees memory.
	SaveToFile()

	// SetSwampName sets the swamp name inside the metadata.
	// This is needed for cases where swamps are loaded by iterating over hashed folder names:
	// we load the metadata first, extract the swamp name, and then load the swamp using its proper name.
	// Therefore, the swamp name must always be saved inside the metadata.
	SetSwampName(swampName name.Name)

	// GetSwampName returns the swamp's name, used to access the swamp directly.
	GetSwampName() name.Name

	// GetCreatedAt returns the timestamp of metadata creation.
	GetCreatedAt() time.Time

	// GetUpdatedAt returns the timestamp of the last metadata update.
	GetUpdatedAt() time.Time

	// GetKey returns the value of a custom metadata key.
	GetKey(key string) string

	// SetKey stores a value under the given metadata key.
	SetKey(key, value string)

	// DeleteKey removes a key-value pair from the metadata.
	DeleteKey(key string) error

	// SetUpdatedAt updates the last modification timestamp to now.
	SetUpdatedAt()

	// Destroy deletes the metadata file and clears the in-memory metadata object.
	// Useful when the object it was attached to has been deleted and will never be used again.
	// Note: currently the file system deletes the metadata file via DeleteAllFiles within the swamp folder,
	// either when Destroy is called on the swamp, or when the swamp has no remaining data.
	Destroy()
}

type metadata struct {
	mu         sync.RWMutex
	meta       *Meta
	path       string
	isModified bool
}

func New(path string) Metadata {
	m := &metadata{
		path: path,
		meta: &Meta{
			KeyValuePairs: make(map[string]string),
		},
	}
	return m
}

func (m *metadata) LoadFromFile() {
	m.load()
	// After loading, we check whether the metadata has a valid creation timestamp.
	// If not, we immediately set it after the first load — this value will then be considered the creation time.
	if m.meta.CreatedAt == (time.Time{}) {
		m.meta.CreatedAt = time.Now().UTC()
	}
}

func (m *metadata) SaveToFile() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.isModified {
		return
	}
	if err := m.save(); err != nil {
		slog.Error("failed to save metadata to file", "error", err)
	}
}

func (m *metadata) SetSwampName(swampName name.Name) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sn := swampName.Get()
	if m.meta.SwampName == "" {
		m.meta.SwampName = sn
		m.isModified = true
		return
	}
	if m.meta.SwampName != sn {
		m.meta.SwampName = sn
		m.isModified = true
	}
}

func (m *metadata) GetSwampName() name.Name {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return name.Load(m.meta.SwampName)
}

func (m *metadata) GetCreatedAt() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.meta.CreatedAt
}

func (m *metadata) GetUpdatedAt() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.meta.UpdatedAt
}

func (m *metadata) Destroy() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.meta = &Meta{KeyValuePairs: make(map[string]string)} // default
	m.delete()
}

func (m *metadata) GetKey(key string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if value, ok := m.meta.KeyValuePairs[key]; ok {
		return value
	}
	return ""
}

func (m *metadata) SetKey(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.meta.KeyValuePairs[key]
	if ok && existing == value {
		return
	}
	m.meta.KeyValuePairs[key] = value
	m.isModified = true
}

func (m *metadata) DeleteKey(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.meta.KeyValuePairs[key]; !ok {
		return nil
	}
	delete(m.meta.KeyValuePairs, key)
	m.isModified = true
	return nil
}

func (m *metadata) SetUpdatedAt() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.meta.UpdatedAt = time.Now().UTC()
	m.isModified = true
}

// load metadata
func (m *metadata) load() {

	file, err := os.Open(filepath.Join(m.path, MetaFile))
	if err != nil {
		// this is not an error, because the metadata file does not exist yet
		return
	}

	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("failed to close metadata file", "error", err)
		}
	}()

	// Unmarshal the GOB encoded file into m.meta
	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&m.meta); err != nil {
		slog.Error("failed to decode metadata from GOB", "error", err)
		m.meta = &Meta{KeyValuePairs: make(map[string]string)} // default
	}
}

// save the metadata to the filesystem
func (m *metadata) save() error {

	mFile := filepath.Join(m.path, MetaFile)

	// IMPORTANT!! We do NOT create the folder structure here — it's the swamp's responsibility to create it.
	file, err := os.Create(mFile)
	if err != nil {
		slog.Error("failed to create metadata file", "error", err, "metadata_file", mFile)
		return err
	}

	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("failed to close metadata file", "error", err)
		}
	}()

	// gob encoder
	encoder := gob.NewEncoder(file)
	if err := encoder.Encode(m.meta); err != nil {
		slog.Error("failed to encode metadata to GOB", "error", err)
		return err
	}

	return nil

}

func (m *metadata) delete() {
	// delete metadata from the filesystem
	_ = os.Remove(filepath.Join(m.path, MetaFile))
}
