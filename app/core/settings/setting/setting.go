package setting

import (
	"github.com/hydraide/hydraide/app/name"
	"time"
)

type Setting interface {
	// GetPattern returns the pattern.
	// Real-world scenario: Useful for debugging or logging to know where the swamp's data is physically stored
	GetPattern() name.Name
	// GetMaxFileSizeByte returns the maximum file size that a swamp's file can be.
	// Real-world scenario: If you have a swamp that is write-heavy and rarely deletes data, setting a higher value can
	// improve performance. If the swamp frequently deletes or modifies data, a smaller value can be more efficient.
	GetMaxFileSizeByte() int64
	// GetCloseAfterIdle returns the time after which a swamp should close itself if it's not being used.
	// Real-world scenario: For swamps that are frequently accessed, setting a higher value ensures that they remain
	// in memory for quicker access. For rarely accessed swamps, a lower value helps in freeing up memory.
	GetCloseAfterIdle() time.Duration
	// GetWriteInterval returns the time interval at which the swamp writes its data to SSD.
	// Real-world scenario: For swamps that are rarely modified, a higher value reduces unnecessary writes.
	// For swamps that are frequently modified, a lower value ensures that changes are saved more frequently.
	// If a swamp experiences a high volume of changes, it's advisable to set this value to no less than one minute
	// to batch writes and reduce both SSD and memory load.
	GetWriteInterval() time.Duration
	// GetSwampType returns the type of the swamp.
	// Real-world scenario: In-memory swamps are useful for testing and broadcasting data between services.
	// Permanent swamps are useful for storing data that needs to be persisted.
	GetSwampType() SwampType
	// UseChroniclerV2 returns true if the swamp should use the new V2 append-only chronicler.
	// V2 is 32-112x faster and uses 50% less storage than V1.
	UseChroniclerV2() bool
}

type SwampType string

const (
	// InMemorySwamp the swamp just keeps the data in memory and doesn't write to SSD.
	InMemorySwamp SwampType = "InMemorySwamp"
	// PermanentSwamp the swamp writes to SSD. CloseAfterIdleSec, WriteIntervalSec, MaxFileSizeByte  settings are used.
	PermanentSwamp SwampType = "PermanentSwamp"
)

type SwampSetting struct {
	Pattern name.Name
	// InMemory true if the swamp just keeps the data in memory and doesn't write to SSD.
	// useful for testing and broadcasting data between services.
	InMemory bool
	// CloseAfterIdleSec Only used if InMemory is false, because the in-memory swamps never close.
	CloseAfterIdleSec time.Duration
	// WriteIntervalSec Only used if InMemory is false, because the in-memory swamps never write to SSD.
	WriteIntervalSec time.Duration
	// MaxFileSizeByte The maximum file size of the swamp's file. Only used if the swamp is not in-memory swamp (i.e. it writes to SSD).
	MaxFileSizeByte int64
	// ChroniclerV2 enables the new append-only V2 chronicler format.
	// V2 is 32-112x faster and uses 50% less storage than V1.
	ChroniclerV2 bool
}

type setting struct {
	ws *SwampSetting
}

func New(ws *SwampSetting) Setting {
	return &setting{
		ws: ws,
	}
}

// GetPattern get the pattern
func (s *setting) GetPattern() name.Name {
	return s.ws.Pattern
}

// GetMaxFileSizeByte get the max loader size byte of the swamp
func (s *setting) GetMaxFileSizeByte() int64 {
	return s.ws.MaxFileSizeByte
}

// GetCloseAfterIdle get the close after idle seconds of the swamp
func (s *setting) GetCloseAfterIdle() time.Duration {
	return s.ws.CloseAfterIdleSec
}

// GetWriteInterval get the write interval seconds of the swamp
func (s *setting) GetWriteInterval() time.Duration {
	return s.ws.WriteIntervalSec
}

// GetSwampType get the swamp type
func (s *setting) GetSwampType() SwampType {
	if s.ws.InMemory {
		return InMemorySwamp
	}
	return PermanentSwamp
}

// UseChroniclerV2 returns true if the swamp should use the V2 chronicler
func (s *setting) UseChroniclerV2() bool {
	return s.ws.ChroniclerV2
}
