package hydraidego

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/hydraide/hydraide/generated/hydraidepbgo"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/client"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/vmihailenco/msgpack/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	errorMessageConnectionError     = "connection error"
	errorMessageCtxTimeout          = "context timeout exceeded"
	errorMessageCtxClosedByClient   = "context closed by client"
	errorMessageInvalidArgument     = "invalid argument"
	errorMessageNotFound            = "sanctuary not found"
	errorMessageUnknown             = "unknown error"
	errorMessageSwampNameNotCorrect = "swamp name is not correct"
	errorMessageSwampNotFound       = "swamp not found"
	errorMessageInternalError       = "internal error"
	errorMessageKeyAlreadyExists    = "key already exists"
	errorMessageKeyNotFound         = "key not found"
	errorMessageConditionNotMet     = "condition not met - the value is"
	errorMessageShuttingDown        = "HydrAIDE server is shutting down"
)

const (
	tagHydrAIDE  = "hydraide"
	tagKey       = "key"
	tagValue     = "value"
	tagOmitempty = "omitempty"
	tagDeletable = "deletable"
	tagCreatedAt = "createdAt"
	tagCreatedBy = "createdBy"
	tagUpdatedAt = "updatedAt"
	tagUpdatedBy = "updatedBy"
	tagExpireAt   = "expireAt"
	tagSearchMeta = "searchMeta"
)

type Hydraidego interface {
	Heartbeat(ctx context.Context) error
	RegisterSwamp(ctx context.Context, request *RegisterSwampRequest) []error
	DeRegisterSwamp(ctx context.Context, swampName name.Name) []error
	Lock(ctx context.Context, key string, ttl time.Duration) (lockID string, err error)
	Unlock(ctx context.Context, key string, lockID string) error
	IsSwampExist(ctx context.Context, swampName name.Name) (bool, error)
	IsKeyExists(ctx context.Context, swampName name.Name, key string) (bool, error)
	CatalogCreate(ctx context.Context, swampName name.Name, model any) error
	CatalogCreateMany(ctx context.Context, swampName name.Name, models []any, iterator CreateManyIteratorFunc) error
	CatalogCreateManyToMany(ctx context.Context, request []*CatalogManyToManyRequest, iterator CatalogCreateManyToManyIteratorFunc) error
	CatalogRead(ctx context.Context, swampName name.Name, key string, model any) error
	CatalogReadMany(ctx context.Context, swampName name.Name, index *Index, model any, iterator CatalogReadManyIteratorFunc) error
	CatalogReadBatch(ctx context.Context, swampName name.Name, keys []string, model any, iterator CatalogReadManyIteratorFunc) error
	CatalogUpdate(ctx context.Context, swampName name.Name, model any) error
	CatalogUpdateMany(ctx context.Context, swampName name.Name, models []any, iterator CatalogUpdateManyIteratorFunc) error
	CatalogDelete(ctx context.Context, swampName name.Name, key string) error
	CatalogDeleteMany(ctx context.Context, swampName name.Name, keys []string, iterator CatalogDeleteIteratorFunc) error
	CatalogDeleteManyFromMany(ctx context.Context, request []*CatalogDeleteManyFromManyRequest, iterator CatalogDeleteIteratorFunc) error
	CatalogSave(ctx context.Context, swampName name.Name, model any) (eventStatus EventStatus, err error)
	CatalogSaveMany(ctx context.Context, swampName name.Name, models []any, iterator CatalogSaveManyIteratorFunc) error
	CatalogSaveManyToMany(ctx context.Context, request []*CatalogManyToManyRequest, iterator CatalogSaveManyToManyIteratorFunc) error
	CatalogShiftExpired(ctx context.Context, swampName name.Name, howMany int32, model any, iterator CatalogShiftExpiredIteratorFunc) error
	CatalogShiftBatch(ctx context.Context, swampName name.Name, keys []string, model any, iterator CatalogShiftBatchIteratorFunc) error
	ProfileSave(ctx context.Context, swampName name.Name, model any) (err error)
	ProfileSaveBatch(ctx context.Context, swampNames []name.Name, models []any, iterator ProfileSaveBatchIteratorFunc) error
	ProfileRead(ctx context.Context, swampName name.Name, model any) (err error)
	ProfileReadBatch(ctx context.Context, swampNames []name.Name, model any, iterator ProfileReadBatchIteratorFunc) error
	Count(ctx context.Context, swampName name.Name) (int32, error)
	Destroy(ctx context.Context, swampName name.Name) error
	DestroyBulk(ctx context.Context, swampNames []name.Name, progressFn func(destroyed, failed, total int64)) error
	Subscribe(ctx context.Context, swampName name.Name, getExistingData bool, model any, iterator SubscribeIteratorFunc) error

	IncrementInt8(ctx context.Context, swampName name.Name, key string, value int8, condition *Int8Condition, setIfNotExist *IncrementMetaRequest, setIfExist *IncrementMetaRequest) (int8, *IncrementMetaResponse, error)
	IncrementInt16(ctx context.Context, swampName name.Name, key string, value int16, condition *Int16Condition, setIfNotExist *IncrementMetaRequest, setIfExist *IncrementMetaRequest) (int16, *IncrementMetaResponse, error)
	IncrementInt32(ctx context.Context, swampName name.Name, key string, value int32, condition *Int32Condition, setIfNotExist *IncrementMetaRequest, setIfExist *IncrementMetaRequest) (int32, *IncrementMetaResponse, error)
	IncrementInt64(ctx context.Context, swampName name.Name, key string, value int64, condition *Int64Condition, setIfNotExist *IncrementMetaRequest, setIfExist *IncrementMetaRequest) (int64, *IncrementMetaResponse, error)
	IncrementUint8(ctx context.Context, swampName name.Name, key string, value uint8, condition *Uint8Condition, setIfNotExist *IncrementMetaRequest, setIfExist *IncrementMetaRequest) (uint8, *IncrementMetaResponse, error)
	IncrementUint16(ctx context.Context, swampName name.Name, key string, value uint16, condition *Uint16Condition, setIfNotExist *IncrementMetaRequest, setIfExist *IncrementMetaRequest) (uint16, *IncrementMetaResponse, error)
	IncrementUint32(ctx context.Context, swampName name.Name, key string, value uint32, condition *Uint32Condition, setIfNotExist *IncrementMetaRequest, setIfExist *IncrementMetaRequest) (uint32, *IncrementMetaResponse, error)
	IncrementUint64(ctx context.Context, swampName name.Name, key string, value uint64, condition *Uint64Condition, setIfNotExist *IncrementMetaRequest, setIfExist *IncrementMetaRequest) (uint64, *IncrementMetaResponse, error)
	IncrementFloat32(ctx context.Context, swampName name.Name, key string, value float32, condition *Float32Condition, setIfNotExist *IncrementMetaRequest, setIfExist *IncrementMetaRequest) (float32, *IncrementMetaResponse, error)
	IncrementFloat64(ctx context.Context, swampName name.Name, key string, value float64, condition *Float64Condition, setIfNotExist *IncrementMetaRequest, setIfExist *IncrementMetaRequest) (float64, *IncrementMetaResponse, error)
	Uint32SlicePush(ctx context.Context, swampName name.Name, KeyValuesPair []*KeyValuesPair) error
	Uint32SliceDelete(ctx context.Context, swampName name.Name, KeyValuesPair []*KeyValuesPair) error
	Uint32SliceSize(ctx context.Context, swampName name.Name, key string) (int64, error)
	Uint32SliceIsValueExist(ctx context.Context, swampName name.Name, key string, value uint32) (bool, error)
	CompactSwamp(ctx context.Context, swampName name.Name) error
	CatalogReadManyStream(ctx context.Context, swampName name.Name, index *Index, filters *FilterGroup, model any, iterator CatalogReadManyIteratorFunc) error
	CatalogReadManyFromMany(ctx context.Context, request []*CatalogReadManyFromManyRequest, model any, iterator CatalogReadManyFromManyIteratorFunc) error
	ProfileReadWithFilter(ctx context.Context, swampName name.Name, filters *FilterGroup, model any) (bool, error)
	ProfileReadBatchWithFilter(ctx context.Context, swampNames []name.Name, filters *FilterGroup, model any, maxResults int32, iterator ProfileReadBatchWithFilterIteratorFunc) error
}

// Index defines the configuration for index-based queries in HydrAIDE.
//
// Indexes allow you to read data from a Swamp in a specific order,
// with optional filtering and pagination.
//
// ✅ Use with `CatalogReadMany()` to read a stream of records
// based on keys, values, or metadata fields like creation time.
//
// Fields:
//   - IndexType:     what field to index on (key, value, createdAt, etc.)
//   - IndexOrder:    ascending or descending result order
//   - From:          offset for pagination (0 = from start)
//   - Limit:         max number of results to return (0 = no limit)
//   - FromTime:      inclusive lower bound (records with time >= FromTime are included)
//   - ToTime:        exclusive upper bound (records with time < ToTime are included)
//
// Example:
//
//	Read the latest 10 entries by creation time:
//	&Index{
//	    IndexType:  IndexCreationTime,
//	    IndexOrder: IndexOrderDesc,
//	    From:       0,
//	    Limit:      10,
//	}
//
//	Read all entries created between two timestamps:
//	&Index{
//	    IndexType:  IndexCreationTime,
//	    IndexOrder: IndexOrderAsc,
//	    From:       0,
//	    Limit:      0,
//	    FromTime:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
//	    ToTime:     time.Date(2024, 6, 30, 23, 59, 59, 0, time.UTC),
//	}
type Index struct {
	IndexType               // What field to use for sorting/filtering
	IndexOrder              // Ascending or Descending order
	From        int32       // Offset: how many records to skip (0 = start from first)
	Limit       int32       // Max results to return (0 = return all) — pre-filter engine limit
	FromTime    *time.Time  // Inclusive lower bound for time-based filtering - optional. It can be nil
	ToTime      *time.Time  // Exclusive upper bound for time-based filtering - optional. It can be nil
	MaxResults  int32       // Post-filter limit: stop streaming after N matches (0 = unlimited)
	ExcludeKeys  []string   // Keys to skip server-side before filter evaluation (nil = no exclusion)
	IncludedKeys []string   // Whitelist: only these keys can appear in results (nil = no restriction)
	KeysOnly     bool       // If true, response contains only Key + IsExist (no content or metadata)
}

// IndexType specifies which field to use as the index during a read.
//
// This controls what HydrAIDE engine uses to sort and filter the Treasures.
//
// Supported types:
//
//   - IndexKey            → Use the Treasure key (string)
//   - IndexValueString    → Use the value, if it's a string
//   - IndexValueUintX     → Use unsigned int value (8/16/32/64)
//   - IndexValueIntX      → Use signed int value (8/16/32/64)
//   - IndexValueFloatX    → Use float values (32/64)
//   - IndexExpirationTime → Use `expireAt` metadata
//   - IndexCreationTime   → Use `createdAt` metadata
//   - IndexUpdateTime     → Use `updatedAt` metadata
//
// 💡 The index type must match the actual data type of the stored value.
// For example, if the value is `float64`, use `IndexValueFloat64`.
type IndexType int

const (
	IndexKey         IndexType = iota + 1 // Sort by the Treasure key (string)
	IndexValueString                      // Sort by the value if it's a string
	IndexValueUint8
	IndexValueUint16
	IndexValueUint32
	IndexValueUint64
	IndexValueInt8
	IndexValueInt16
	IndexValueInt32
	IndexValueInt64
	IndexValueFloat32
	IndexValueFloat64
	IndexExpirationTime // Use the metadata field `expireAt`
	IndexCreationTime   // Use the metadata field `createdAt`
	IndexUpdateTime     // Use the metadata field `updatedAt`
)

// IndexOrder defines the direction of sorting when reading data by index.
//
// Use IndexOrderAsc for oldest → newest, or lowest → highest.
// Use IndexOrderDesc for newest → oldest, or highest → lowest.
type IndexOrder int

const (
	IndexOrderAsc  IndexOrder = iota + 1 // Ascending (A → Z, 0 → 9, oldest → newest)
	IndexOrderDesc                       // Descending (Z → A, 9 → 0, newest → oldest)
)

type EventStatus int

const (
	StatusUnknown EventStatus = iota
	StatusSwampNotFound
	StatusTreasureNotFound
	StatusNew
	StatusModified
	StatusNothingChanged
	StatusDeleted
)

type RegisterSwampRequest struct {
	// SwampPattern defines the pattern to register in HydrAIDE.
	// You can use wildcards (*) for dynamic parts.
	//
	// If the pattern includes a wildcard, HydrAIDE registers it on all servers,
	// since it cannot predict where the actual Swamp will reside after resolution.
	//
	// If the pattern has no wildcard, HydrAIDE uses its internal logic to determine
	// which server should handle the Swamp, and registers it only there.
	//
	// Example (no wildcard): Sanctuary("users").Realm("logs").Swamp("johndoe")
	// → Registered only on one server.
	//
	// Example (with wildcard): Sanctuary("users").Realm("logs").Swamp("*")
	// → Registered on all servers to ensure universal match.
	SwampPattern name.Name

	// CloseAfterIdle defines the idle time (inactivity period) after which
	// the Swamp is automatically closed and flushed from memory.
	//
	// When this timeout expires, HydrAIDE will:
	// - flush all changes to disk (if persistent),
	// - unload the Swamp from RAM,
	// - release any temporary resources.
	//
	// This helps keep memory lean and ensures disk durability when needed.
	CloseAfterIdle time.Duration

	// IsInMemorySwamp controls whether the Swamp should exist only in memory.
	//
	// If true → Swamp data is volatile and will be lost when closed.
	// If false → Swamp data is also persisted to disk.
	//
	// In-memory Swamps are ideal for:
	// - transient data between services,
	// - ephemeral socket messages,
	// - disappearing chat messages,
	// or any short-lived data flow.
	//
	// ⚠️ Warning: For in-memory Swamps, if CloseAfterIdle triggers,
	// all data is permanently lost.
	IsInMemorySwamp bool

	// FilesystemSettings provides persistence-related configuration.
	//
	// This is ignored if IsInMemorySwamp is true.
	// If persistence is enabled (IsInMemorySwamp = false), these settings control:
	// - how often data is flushed to disk,
	// - how large each chunk file can grow.
	//
	// If nil, the server will use its default settings.
	FilesystemSettings *SwampFilesystemSettings
}

// EncodingFormat controls how complex value types (structs, slices, maps, pointers)
// are serialized into the BytesVal field of a Treasure.
//
// Default (EncodingGOB): Uses Go's native binary encoding for backward compatibility.
// EncodingMsgPack: Uses MessagePack, a cross-language binary format that supports
// server-side field-level inspection for query filtering via BytesFieldPath.
//
// Changing this only affects new writes. Existing data is auto-detected when reading.
type EncodingFormat int

const (
	// EncodingGOB uses Go's native binary encoding (default, backward compatible).
	// Data encoded with GOB can only be read by Go SDK clients.
	EncodingGOB EncodingFormat = iota

	// EncodingMsgPack uses MessagePack encoding, a cross-language binary format.
	// Supports server-side field-level inspection for query filtering.
	// Data can be read by any language SDK (Python, JS, Rust, Java, etc.)
	EncodingMsgPack
)

type SwampFilesystemSettings struct {

	// WriteInterval defines how often (in seconds) HydrAIDE should write
	// new, modified, or deleted Treasures from memory to disk.
	//
	// If the Swamp is closed before this interval expires, it will still flush all data.
	// This setting optimizes for SSD wear vs. durability:
	// - Short intervals = safer but more writes.
	// - Longer intervals = fewer writes, but higher risk if crash occurs.
	//
	// Minimum allowed value is 1 second.
	WriteInterval time.Duration

	// MaxFileSize defines the maximum compressed chunk size on disk.
	//
	// Deprecated: This field is only used by the legacy V1 storage engine.
	// After migrating to V2 (using `hydraidectl migrate`), this field is ignored
	// and can be safely removed from your code.
	MaxFileSize int

	// EncodingFormat controls how complex value types are serialized into BytesVal.
	//
	// Default (EncodingGOB): Go binary encoding. Backward compatible with all existing data.
	// EncodingMsgPack: MessagePack encoding. Cross-language, supports server-side field filtering.
	//
	// To migrate existing data from GOB to MessagePack:
	// 1. Set EncodingFormat to EncodingMsgPack in RegisterSwamp
	// 2. Read all data from the swamp (auto-detected as GOB)
	// 3. Re-save all data (SDK writes in MessagePack, server detects byte-level change)
	// 4. Call CompactSwamp to remove old GOB entries from the .hyd file
	EncodingFormat EncodingFormat
}

// TimeField identifies which Treasure timestamp field to filter on.
type TimeField int

const (
	// TimeFieldCreatedAt filters on the Treasure's creation timestamp.
	TimeFieldCreatedAt TimeField = iota
	// TimeFieldUpdatedAt filters on the Treasure's last update timestamp.
	TimeFieldUpdatedAt
	// TimeFieldExpiredAt filters on the Treasure's expiration timestamp.
	TimeFieldExpiredAt
)

// Filter defines a server-side filter predicate applied to Treasures.
// Exactly one typed value field should be set; it determines which Treasure field to compare.
// If BytesFieldPath is set, the filter extracts from the MessagePack-encoded BytesVal instead.
type Filter struct {
	operator       RelationalOperator
	int8Val        *int8
	int16Val       *int16
	int32Val       *int32
	int64Val       *int64
	uint8Val       *uint8
	uint16Val      *uint16
	uint32Val      *uint32
	uint64Val      *uint64
	float32Val     *float32
	float64Val     *float64
	stringVal      *string
	boolVal        *bool
	bytesFieldPath *string
	timeVal        *time.Time
	timeField      *TimeField
	treasureKey    *string // Profile mode: which Treasure key this filter targets
	label          *string // Optional label for match tracking in SearchResultMeta
}

// WithLabel sets a label for match tracking. When this filter matches,
// the label appears in SearchResultMeta.MatchedLabels.
func (f *Filter) WithLabel(label string) *Filter {
	f.label = &label
	return f
}

// ForKey sets the TreasureKey for profile-mode filtering.
// In profile mode, each struct field is stored as a separate Treasure keyed by field name.
// This method specifies which Treasure the filter should evaluate against.
//
// Example:
//
//	hydraidego.FilterInt32(hydraidego.GreaterThan, 25).ForKey("Age")
func (f *Filter) ForKey(key string) *Filter {
	f.treasureKey = &key
	return f
}

// --- Filter constructors for primitive Treasure value types ---

func FilterInt8(op RelationalOperator, value int8) *Filter {
	return &Filter{operator: op, int8Val: &value}
}
func FilterInt16(op RelationalOperator, value int16) *Filter {
	return &Filter{operator: op, int16Val: &value}
}
func FilterInt32(op RelationalOperator, value int32) *Filter {
	return &Filter{operator: op, int32Val: &value}
}
func FilterInt64(op RelationalOperator, value int64) *Filter {
	return &Filter{operator: op, int64Val: &value}
}
func FilterUint8(op RelationalOperator, value uint8) *Filter {
	return &Filter{operator: op, uint8Val: &value}
}
func FilterUint16(op RelationalOperator, value uint16) *Filter {
	return &Filter{operator: op, uint16Val: &value}
}
func FilterUint32(op RelationalOperator, value uint32) *Filter {
	return &Filter{operator: op, uint32Val: &value}
}
func FilterUint64(op RelationalOperator, value uint64) *Filter {
	return &Filter{operator: op, uint64Val: &value}
}
func FilterFloat32(op RelationalOperator, value float32) *Filter {
	return &Filter{operator: op, float32Val: &value}
}
func FilterFloat64(op RelationalOperator, value float64) *Filter {
	return &Filter{operator: op, float64Val: &value}
}
func FilterString(op RelationalOperator, value string) *Filter {
	return &Filter{operator: op, stringVal: &value}
}
func FilterBool(op RelationalOperator, value bool) *Filter {
	return &Filter{operator: op, boolVal: &value}
}

// --- Filter constructors for Treasure timestamp fields ---

// FilterCreatedAt creates a filter that compares against the Treasure's CreatedAt timestamp.
// Supported operators: Equal, NotEqual, GreaterThan, GreaterThanOrEqual, LessThan, LessThanOrEqual, IsEmpty, IsNotEmpty.
func FilterCreatedAt(op RelationalOperator, value time.Time) *Filter {
	tf := TimeFieldCreatedAt
	return &Filter{operator: op, timeVal: &value, timeField: &tf}
}

// FilterUpdatedAt creates a filter that compares against the Treasure's UpdatedAt timestamp.
// Supported operators: Equal, NotEqual, GreaterThan, GreaterThanOrEqual, LessThan, LessThanOrEqual, IsEmpty, IsNotEmpty.
func FilterUpdatedAt(op RelationalOperator, value time.Time) *Filter {
	tf := TimeFieldUpdatedAt
	return &Filter{operator: op, timeVal: &value, timeField: &tf}
}

// FilterExpiredAt creates a filter that compares against the Treasure's ExpiredAt timestamp.
// Supported operators: Equal, NotEqual, GreaterThan, GreaterThanOrEqual, LessThan, LessThanOrEqual, IsEmpty, IsNotEmpty.
func FilterExpiredAt(op RelationalOperator, value time.Time) *Filter {
	tf := TimeFieldExpiredAt
	return &Filter{operator: op, timeVal: &value, timeField: &tf}
}

// --- Filter constructors for BytesVal field-level filtering (MessagePack only) ---

func FilterBytesFieldInt8(op RelationalOperator, fieldPath string, value int8) *Filter {
	return &Filter{operator: op, bytesFieldPath: &fieldPath, int8Val: &value}
}
func FilterBytesFieldInt16(op RelationalOperator, fieldPath string, value int16) *Filter {
	return &Filter{operator: op, bytesFieldPath: &fieldPath, int16Val: &value}
}
func FilterBytesFieldInt32(op RelationalOperator, fieldPath string, value int32) *Filter {
	return &Filter{operator: op, bytesFieldPath: &fieldPath, int32Val: &value}
}
func FilterBytesFieldInt64(op RelationalOperator, fieldPath string, value int64) *Filter {
	return &Filter{operator: op, bytesFieldPath: &fieldPath, int64Val: &value}
}
func FilterBytesFieldUint8(op RelationalOperator, fieldPath string, value uint8) *Filter {
	return &Filter{operator: op, bytesFieldPath: &fieldPath, uint8Val: &value}
}
func FilterBytesFieldUint16(op RelationalOperator, fieldPath string, value uint16) *Filter {
	return &Filter{operator: op, bytesFieldPath: &fieldPath, uint16Val: &value}
}
func FilterBytesFieldUint32(op RelationalOperator, fieldPath string, value uint32) *Filter {
	return &Filter{operator: op, bytesFieldPath: &fieldPath, uint32Val: &value}
}
func FilterBytesFieldUint64(op RelationalOperator, fieldPath string, value uint64) *Filter {
	return &Filter{operator: op, bytesFieldPath: &fieldPath, uint64Val: &value}
}
func FilterBytesFieldFloat32(op RelationalOperator, fieldPath string, value float32) *Filter {
	return &Filter{operator: op, bytesFieldPath: &fieldPath, float32Val: &value}
}
func FilterBytesFieldFloat64(op RelationalOperator, fieldPath string, value float64) *Filter {
	return &Filter{operator: op, bytesFieldPath: &fieldPath, float64Val: &value}
}
func FilterBytesFieldString(op RelationalOperator, fieldPath string, value string) *Filter {
	return &Filter{operator: op, bytesFieldPath: &fieldPath, stringVal: &value}
}
func FilterBytesFieldBool(op RelationalOperator, fieldPath string, value bool) *Filter {
	return &Filter{operator: op, bytesFieldPath: &fieldPath, boolVal: &value}
}

// --- Slice Contains filters ---

// FilterBytesFieldSliceContainsInt8 checks if the []int8 slice at fieldPath contains value.
func FilterBytesFieldSliceContainsInt8(bytesFieldPath string, value int8) *Filter {
	return &Filter{operator: SliceContains, bytesFieldPath: &bytesFieldPath, int8Val: &value}
}

// FilterBytesFieldSliceContainsInt32 checks if the []int32 slice at fieldPath contains value.
func FilterBytesFieldSliceContainsInt32(bytesFieldPath string, value int32) *Filter {
	return &Filter{operator: SliceContains, bytesFieldPath: &bytesFieldPath, int32Val: &value}
}

// FilterBytesFieldSliceContainsInt64 checks if the []int64 slice at fieldPath contains value.
func FilterBytesFieldSliceContainsInt64(bytesFieldPath string, value int64) *Filter {
	return &Filter{operator: SliceContains, bytesFieldPath: &bytesFieldPath, int64Val: &value}
}

// FilterBytesFieldSliceContainsString checks if the []string slice at fieldPath contains value (exact, case-sensitive).
func FilterBytesFieldSliceContainsString(bytesFieldPath string, value string) *Filter {
	return &Filter{operator: SliceContains, bytesFieldPath: &bytesFieldPath, stringVal: &value}
}

// --- Slice NotContains filters ---

// FilterBytesFieldSliceNotContainsInt8 checks that the []int8 slice at fieldPath does NOT contain value.
func FilterBytesFieldSliceNotContainsInt8(bytesFieldPath string, value int8) *Filter {
	return &Filter{operator: SliceNotContains, bytesFieldPath: &bytesFieldPath, int8Val: &value}
}

// FilterBytesFieldSliceNotContainsInt32 checks that the []int32 slice at fieldPath does NOT contain value.
func FilterBytesFieldSliceNotContainsInt32(bytesFieldPath string, value int32) *Filter {
	return &Filter{operator: SliceNotContains, bytesFieldPath: &bytesFieldPath, int32Val: &value}
}

// FilterBytesFieldSliceNotContainsInt64 checks that the []int64 slice at fieldPath does NOT contain value.
func FilterBytesFieldSliceNotContainsInt64(bytesFieldPath string, value int64) *Filter {
	return &Filter{operator: SliceNotContains, bytesFieldPath: &bytesFieldPath, int64Val: &value}
}

// FilterBytesFieldSliceNotContainsString checks that the []string slice at fieldPath does NOT contain value.
func FilterBytesFieldSliceNotContainsString(bytesFieldPath string, value string) *Filter {
	return &Filter{operator: SliceNotContains, bytesFieldPath: &bytesFieldPath, stringVal: &value}
}

// --- Slice Substring filters ---

// FilterBytesFieldSliceContainsSubstring checks if any string element in the slice contains substring (case-insensitive).
func FilterBytesFieldSliceContainsSubstring(bytesFieldPath string, substring string) *Filter {
	return &Filter{operator: SliceContainsSubstring, bytesFieldPath: &bytesFieldPath, stringVal: &substring}
}

// FilterBytesFieldSliceNotContainsSubstring checks that no string element in the slice contains substring (case-insensitive).
func FilterBytesFieldSliceNotContainsSubstring(bytesFieldPath string, substring string) *Filter {
	return &Filter{operator: SliceNotContainsSubstring, bytesFieldPath: &bytesFieldPath, stringVal: &substring}
}

// --- Slice Length filter ---

// FilterBytesFieldSliceLen compares the length of the slice at bytesFieldPath using the given operator.
// Uses the #len pseudo-field internally.
func FilterBytesFieldSliceLen(op RelationalOperator, bytesFieldPath string, length int32) *Filter {
	lenPath := bytesFieldPath + ".#len"
	return &Filter{operator: op, bytesFieldPath: &lenPath, int32Val: &length}
}

// --- Nested Slice Any filters ---

// FilterBytesFieldNestedSliceAnyString checks if ANY element in the struct slice has a fieldName matching the condition.
// Uses the [*] wildcard syntax internally.
func FilterBytesFieldNestedSliceAnyString(slicePath string, fieldName string, op RelationalOperator, value string) *Filter {
	anyPath := slicePath + "[*]." + fieldName
	return &Filter{operator: op, bytesFieldPath: &anyPath, stringVal: &value}
}

// FilterBytesFieldNestedSliceAnyInt8 checks if ANY element in the struct slice has a fieldName matching the condition.
func FilterBytesFieldNestedSliceAnyInt8(slicePath string, fieldName string, op RelationalOperator, value int8) *Filter {
	anyPath := slicePath + "[*]." + fieldName
	return &Filter{operator: op, bytesFieldPath: &anyPath, int8Val: &value}
}

// FilterBytesFieldNestedSliceAnyBool checks if ANY element in the struct slice has a fieldName matching the condition.
func FilterBytesFieldNestedSliceAnyBool(slicePath string, fieldName string, op RelationalOperator, value bool) *Filter {
	anyPath := slicePath + "[*]." + fieldName
	return &Filter{operator: op, bytesFieldPath: &anyPath, boolVal: &value}
}

// SearchMeta contains metadata about how a Treasure matched the search criteria.
// Populated during filter evaluation when labeled filters or VectorFilters are used.
type SearchMeta struct {
	// VectorScores contains cosine similarity scores (one per VectorFilter, in order).
	VectorScores []float32
	// MatchedLabels contains labels of filters that evaluated to true.
	MatchedLabels []string
}

// FilterLogic defines how conditions within a FilterGroup are combined.
type FilterLogic int

const (
	// FilterLogicAND requires ALL conditions to be true (default).
	FilterLogicAND FilterLogic = iota
	// FilterLogicOR requires at least ONE condition to be true.
	FilterLogicOR
)

// FilterItem is an interface implemented by both Filter and FilterGroup,
// allowing them to be used interchangeably in FilterAND/FilterOR constructors.
type FilterItem interface {
	isFilterItem()
}

// isFilterItem marks Filter as a valid FilterItem.
func (f *Filter) isFilterItem() {}

// FilterGroup is a recursive filter structure supporting nested AND/OR logic.
//
// A FilterGroup contains leaf-level Filters and nested SubGroups combined with AND or OR logic.
//
// Evaluation rules:
//   - AND: ALL Filters AND ALL SubGroups must evaluate to true
//   - OR: at least ONE Filter OR ONE SubGroup must evaluate to true
//   - Empty group (no Filters, no SubGroups) passes all Treasures (no filtering)
//
// Example: (price > 100 AND (status == "active" OR status == "pending"))
//
//	hydraidego.FilterAND(
//	    hydraidego.FilterFloat64(hydraidego.GreaterThan, 100.0),
//	    hydraidego.FilterOR(
//	        hydraidego.FilterString(hydraidego.Equal, "active"),
//	        hydraidego.FilterString(hydraidego.Equal, "pending"),
//	    ),
//	)
type FilterGroup struct {
	logic              FilterLogic
	filters            []*Filter
	subGroups          []*FilterGroup
	phraseFilters      []*PhraseFilter
	vectorFilters      []*VectorFilter
	geoDistanceFilters []*GeoDistanceFilter
}

// isFilterItem marks FilterGroup as a valid FilterItem.
func (fg *FilterGroup) isFilterItem() {}

// PhraseFilter checks if specified words appear at consecutive positions
// in a word-index map (map[string][]int) stored in the Treasure's BytesVal.
//
// The BytesFieldPath identifies the field within the MessagePack-encoded BytesVal
// that holds the word-index map. Each key in the map is a word, and the value
// is a sorted list of positions where that word appears in a text.
//
// If Negate is false: matches when the words ARE found at consecutive positions.
// If Negate is true: matches when the words are NOT found at consecutive positions.
type PhraseFilter struct {
	bytesFieldPath string
	words          []string
	negate         bool
	treasureKey    *string // Profile mode: which Treasure key this filter targets
	label          *string // Optional label for match tracking in SearchResultMeta
}

// WithLabel sets a label for match tracking on this PhraseFilter.
func (pf *PhraseFilter) WithLabel(label string) *PhraseFilter {
	pf.label = &label
	return pf
}

// ForKey sets the TreasureKey for profile-mode phrase filtering.
// See Filter.ForKey for details.
//
// Example:
//
//	hydraidego.FilterPhrase("WordIndex", "hello", "world").ForKey("Content")
func (pf *PhraseFilter) ForKey(key string) *PhraseFilter {
	pf.treasureKey = &key
	return pf
}

// isFilterItem marks PhraseFilter as a valid FilterItem.
func (pf *PhraseFilter) isFilterItem() {}

// FilterPhrase creates a PhraseFilter that matches when the specified words
// appear at consecutive positions in the word-index map at bytesFieldPath.
func FilterPhrase(bytesFieldPath string, words ...string) *PhraseFilter {
	return &PhraseFilter{bytesFieldPath: bytesFieldPath, words: words, negate: false}
}

// FilterNotPhrase creates a PhraseFilter that matches when the specified words
// do NOT appear at consecutive positions in the word-index map at bytesFieldPath.
func FilterNotPhrase(bytesFieldPath string, words ...string) *PhraseFilter {
	return &PhraseFilter{bytesFieldPath: bytesFieldPath, words: words, negate: true}
}

// VectorFilter performs cosine similarity matching against a float32 vector
// stored in the Treasure's MessagePack-encoded BytesVal.
//
// The filter extracts a []float32 field from BytesVal at the given BytesFieldPath,
// computes the dot product with the QueryVector (both must be pre-normalized to unit length),
// and returns true if the similarity score meets or exceeds MinSimilarity.
//
// Both the stored vectors and QueryVector MUST be L2-normalized (unit length).
// Use NormalizeVector() before storing and before searching.
//
// Example:
//
//	queryVec := hydraidego.NormalizeVector(rawQueryVector)
//	hydraidego.FilterVector("Embedding", queryVec, 0.70)
type VectorFilter struct {
	bytesFieldPath string
	queryVector    []float32
	minSimilarity  float32
	treasureKey    *string // Profile mode: which Treasure key this filter targets
	label          *string // Optional label for match tracking in SearchResultMeta
}

// WithLabel sets a label for match tracking on this VectorFilter.
func (vf *VectorFilter) WithLabel(label string) *VectorFilter {
	vf.label = &label
	return vf
}

// isFilterItem marks VectorFilter as a valid FilterItem.
func (vf *VectorFilter) isFilterItem() {}

// ForKey sets the TreasureKey for profile-mode vector filtering.
// In profile mode, each struct field is stored as a separate Treasure keyed by field name.
// This method specifies which Treasure the vector filter should evaluate against.
//
// Example:
//
//	hydraidego.FilterVector("Embedding", queryVec, 0.70).ForKey("MainProfile")
func (vf *VectorFilter) ForKey(key string) *VectorFilter {
	vf.treasureKey = &key
	return vf
}

// FilterVector creates a VectorFilter that matches Treasures whose vector field
// has cosine similarity >= minSimilarity with the given query vector.
//
// Parameters:
//   - bytesFieldPath: dot-separated path to the []float32 field in BytesVal (e.g. "Embedding")
//   - queryVector: the search vector (must be L2-normalized)
//   - minSimilarity: minimum cosine similarity threshold (0.0 – 1.0)
//
// Example:
//
//	hydraidego.FilterAND(
//	    hydraidego.FilterBytesFieldString(hydraidego.Equal, "Category", "business"),
//	    hydraidego.FilterBytesFieldString(hydraidego.Equal, "Language", "hu"),
//	    hydraidego.FilterVector("Embedding", queryVec, 0.70),
//	)
func FilterVector(bytesFieldPath string, queryVector []float32, minSimilarity float32) *VectorFilter {
	return &VectorFilter{
		bytesFieldPath: bytesFieldPath,
		queryVector:    queryVector,
		minSimilarity:  minSimilarity,
	}
}

// GeoMode determines whether a GeoDistance filter matches inside or outside the radius.
type GeoMode int

const (
	// GeoInside matches Treasures within the specified radius (distance <= radius).
	GeoInside GeoMode = iota
	// GeoOutside matches Treasures beyond the specified radius (distance > radius).
	GeoOutside
)

// GeoDistanceFilter performs geographic distance filtering using the Haversine formula.
type GeoDistanceFilter struct {
	latFieldPath string
	lngFieldPath string
	refLatitude  float64
	refLongitude float64
	radiusKm     float64
	mode         GeoMode
	treasureKey  *string
	label        *string // Optional label for match tracking in SearchResultMeta
}

// WithLabel sets a label for match tracking on this GeoDistanceFilter.
func (gf *GeoDistanceFilter) WithLabel(label string) *GeoDistanceFilter {
	gf.label = &label
	return gf
}

// isFilterItem marks GeoDistanceFilter as a valid FilterItem.
func (gf *GeoDistanceFilter) isFilterItem() {}

// ForKey sets the TreasureKey for profile-mode geo distance filtering.
func (gf *GeoDistanceFilter) ForKey(key string) *GeoDistanceFilter {
	gf.treasureKey = &key
	return gf
}

// GeoDistance creates a GeoDistanceFilter that matches Treasures based on their
// geographic distance from a reference point using the Haversine formula.
//
// Parameters:
//   - latFieldPath: dot-separated path to the latitude (float64) field in BytesVal
//   - lngFieldPath: dot-separated path to the longitude (float64) field in BytesVal
//   - refLat: reference point latitude in WGS84 degrees
//   - refLng: reference point longitude in WGS84 degrees
//   - radiusKm: distance threshold in kilometers
//   - mode: GeoInside (within radius) or GeoOutside (beyond radius)
//
// Records with lat == 0 AND lng == 0 (Null Island / missing data) are automatically excluded.
//
// Example — find domains within 50 km of Budapest:
//
//	hydraidego.GeoDistance("geo_latitude", "geo_longitude", 47.4979, 19.0402, 50.0, hydraidego.GeoInside)
//
// Example — 50-150 km band:
//
//	hydraidego.FilterAND(
//	    hydraidego.GeoDistance("geo_latitude", "geo_longitude", 47.4979, 19.0402, 50.0, hydraidego.GeoOutside),
//	    hydraidego.GeoDistance("geo_latitude", "geo_longitude", 47.4979, 19.0402, 150.0, hydraidego.GeoInside),
//	)
func GeoDistance(latFieldPath, lngFieldPath string, refLat, refLng, radiusKm float64, mode GeoMode) *GeoDistanceFilter {
	return &GeoDistanceFilter{
		latFieldPath: latFieldPath,
		lngFieldPath: lngFieldPath,
		refLatitude:  refLat,
		refLongitude: refLng,
		radiusKm:     radiusKm,
		mode:         mode,
	}
}

// NormalizeVector returns a new L2-normalized copy of the input vector (unit length).
// If the input vector is a zero vector (all elements are 0), returns a zero-length slice.
// Normalized vectors allow cosine similarity to be computed as a simple dot product.
func NormalizeVector(v []float32) []float32 {
	if len(v) == 0 {
		return nil
	}

	var norm float32
	for _, x := range v {
		norm += x * x
	}
	if norm == 0 {
		return nil
	}

	invNorm := float32(1.0 / math.Sqrt(float64(norm)))
	result := make([]float32, len(v))
	for i, x := range v {
		result[i] = x * invNorm
	}
	return result
}

// CosineSimilarity computes the cosine similarity between two float32 vectors.
// Returns a value between -1.0 and 1.0, where 1.0 means identical direction.
// Returns 0 if either vector is zero or dimensions don't match.
// This function works with non-normalized vectors (includes magnitude normalization).
// For pre-normalized vectors, use a simple dot product instead for better performance.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / float32(math.Sqrt(float64(normA))*math.Sqrt(float64(normB)))
}

// FilterAND creates a FilterGroup that requires ALL conditions to be true.
// Accepts any combination of *Filter, *FilterGroup, *PhraseFilter, *VectorFilter, and *GeoDistanceFilter items.
func FilterAND(items ...FilterItem) *FilterGroup {
	g := &FilterGroup{logic: FilterLogicAND}
	for _, item := range items {
		switch v := item.(type) {
		case *Filter:
			g.filters = append(g.filters, v)
		case *FilterGroup:
			g.subGroups = append(g.subGroups, v)
		case *PhraseFilter:
			g.phraseFilters = append(g.phraseFilters, v)
		case *VectorFilter:
			g.vectorFilters = append(g.vectorFilters, v)
		case *GeoDistanceFilter:
			g.geoDistanceFilters = append(g.geoDistanceFilters, v)
		}
	}
	return g
}

// FilterOR creates a FilterGroup that requires at least ONE condition to be true.
// Accepts any combination of *Filter, *FilterGroup, *PhraseFilter, *VectorFilter, and *GeoDistanceFilter items.
func FilterOR(items ...FilterItem) *FilterGroup {
	g := &FilterGroup{logic: FilterLogicOR}
	for _, item := range items {
		switch v := item.(type) {
		case *Filter:
			g.filters = append(g.filters, v)
		case *FilterGroup:
			g.subGroups = append(g.subGroups, v)
		case *PhraseFilter:
			g.phraseFilters = append(g.phraseFilters, v)
		case *VectorFilter:
			g.vectorFilters = append(g.vectorFilters, v)
		case *GeoDistanceFilter:
			g.geoDistanceFilters = append(g.geoDistanceFilters, v)
		}
	}
	return g
}

// CatalogReadManyFromManyRequest defines a per-swamp query for multi-swamp streaming reads.
type CatalogReadManyFromManyRequest struct {
	SwampName name.Name
	Index     *Index
	Filters   *FilterGroup
}

// CatalogReadManyFromManyIteratorFunc is called for each Treasure found during
// a multi-swamp streaming read, along with the source swamp name.
type CatalogReadManyFromManyIteratorFunc func(swampName name.Name, model any) error

type hydraidego struct {
	client          client.Client
	patternEncoding map[string]EncodingFormat
	patternMu       sync.RWMutex
}

func New(client client.Client) Hydraidego {
	return &hydraidego{
		client:          client,
		patternEncoding: make(map[string]EncodingFormat),
	}
}

// getEncodingForSwamp returns the encoding format registered for the given swamp name.
// It first checks for an exact match, then tries wildcard pattern matching.
// Returns EncodingGOB if no matching pattern is found (backward compatible default).
func (h *hydraidego) getEncodingForSwamp(swampName name.Name) EncodingFormat {
	h.patternMu.RLock()
	defer h.patternMu.RUnlock()

	nameStr := swampName.Get()

	// Exact match first
	if enc, ok := h.patternEncoding[nameStr]; ok {
		return enc
	}

	// Wildcard pattern match
	for pattern, enc := range h.patternEncoding {
		if matched, _ := path.Match(pattern, nameStr); matched {
			return enc
		}
	}

	return EncodingGOB
}

// Heartbeat checks if all HydrAIDE servers are reachable.
// If any server is unreachable, it returns an aggregated error.
// If all are reachable, it returns nil.
//
// This method can be used to monitor the health of your HydrAIDE cluster.
// However, note that HydrAIDE clients have automatic reconnection logic,
// so a temporary network issue may not surface unless it persists.
func (h *hydraidego) Heartbeat(ctx context.Context) error {

	// Retrieve all unique gRPC service clients from the internal client pool.
	serviceClients := h.client.GetUniqueServiceClients()

	// Collect any errors encountered during heartbeat checks.
	allErrors := make([]string, 0)

	// Iterate through each server and perform a heartbeat ping.
	for _, serviceClient := range serviceClients {
		_, err := serviceClient.Heartbeat(ctx, &hydraidepbgo.HeartbeatRequest{
			Ping: "ping",
		})

		// If an error occurred, add it to the collection.
		if err != nil {
			allErrors = append(allErrors, fmt.Sprintf("error: %v", err))
		}
	}

	// If any servers failed to respond, return a formatted error containing all issues.
	if len(allErrors) > 0 {
		return fmt.Errorf("one or many servers are not reachable: %v", allErrors)
	}

	// All servers responded successfully — return nil to indicate success.
	return nil
}

// RegisterSwamp registers a Swamp pattern across the appropriate HydrAIDE servers.
//
// This method is required before using a Swamp. It tells HydrAIDE how to handle
// memory, persistence, and routing for that pattern.
//
//   - If the SwampPattern contains any wildcard (e.g. Sanctuary("*"), Realm("*"), or Swamp("*")),
//     the pattern is registered on **all** servers.
//   - If the pattern is exact (no wildcard at any level), it is registered **only on the responsible server**,
//     based on HydrAIDE's internal name-to-folder mapping.
//
// ⚠️ While wildcarding the Sanctuary is technically possible, it is not recommended,
// as Sanctuary represents a high-level logical domain and should remain stable.
//
// Returns a list of errors, one for each server where registration failed.
// If registration is fully successful, it returns nil.
func (h *hydraidego) RegisterSwamp(ctx context.Context, request *RegisterSwampRequest) []error {

	// Container to collect any errors during registration.
	allErrors := make([]error, 0)

	// Validate that SwampPattern is provided.
	if request.SwampPattern == nil {
		allErrors = append(allErrors, fmt.Errorf("SwampPattern is required"))
		return allErrors
	}

	// List of servers where the Swamp pattern will be registered.
	selectedServers := make([]hydraidepbgo.HydraideServiceClient, 0)

	// Wildcard patterns must be registered on all servers,
	// because we don’t know in advance which server will handle each resolved Swamp.
	if request.SwampPattern.IsWildcardPattern() {
		selectedServers = h.client.GetUniqueServiceClients()
	} else {
		// For non-wildcard patterns, we determine the responsible server
		// using HydrAIDE’s name-based routing logic.
		selectedServers = append(selectedServers, h.client.GetServiceClient(request.SwampPattern))
	}

	// Iterate through the selected servers and register the Swamp on each.
	for _, serviceClient := range selectedServers {

		// Construct the RegisterSwampRequest payload for the gRPC call.
		rsr := &hydraidepbgo.RegisterSwampRequest{
			SwampPattern:    request.SwampPattern.Get(),
			CloseAfterIdle:  int64(request.CloseAfterIdle.Seconds()),
			IsInMemorySwamp: request.IsInMemorySwamp,
		}

		// If the Swamp is persistent (not in-memory), apply filesystem settings.
		if !request.IsInMemorySwamp && request.FilesystemSettings != nil {
			wi := int64(request.FilesystemSettings.WriteInterval.Seconds())
			mfs := int64(request.FilesystemSettings.MaxFileSize)
			rsr.WriteInterval = &wi
			rsr.MaxFileSize = &mfs

			// Send encoding format to server if explicitly set to MsgPack
			if request.FilesystemSettings.EncodingFormat == EncodingMsgPack {
				ef := hydraidepbgo.EncodingFormat_MSGPACK
				rsr.EncodingFormat = &ef
			}
		}

		// Store encoding format locally for use during write operations
		if request.FilesystemSettings != nil && request.FilesystemSettings.EncodingFormat == EncodingMsgPack {
			h.patternMu.Lock()
			h.patternEncoding[request.SwampPattern.Get()] = EncodingMsgPack
			h.patternMu.Unlock()
		}

		// Attempt to register the Swamp pattern on the current server.
		_, err := serviceClient.RegisterSwamp(ctx, rsr)

		// Handle any errors returned from the gRPC call.
		if err != nil {
			if s, ok := status.FromError(err); ok {
				switch s.Code() {
				case codes.Unavailable:
					allErrors = append(allErrors, NewError(ErrCodeConnectionError, errorMessageConnectionError))
				case codes.DeadlineExceeded:
					allErrors = append(allErrors, NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout))
				case codes.Canceled:
					allErrors = append(allErrors, NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient))
				case codes.InvalidArgument:
					allErrors = append(allErrors, NewError(ErrCodeInvalidArgument, fmt.Sprintf("%s: %v", errorMessageInvalidArgument, s.Message())))
				case codes.NotFound:
					allErrors = append(allErrors, NewError(ErrCodeNotFound, fmt.Sprintf("%s: %v", errorMessageNotFound, s.Message())))
				default:
					allErrors = append(allErrors, NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err)))
				}
			} else {
				allErrors = append(allErrors, NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err)))
			}
		}
	}

	// If any server failed, return the list of errors.
	if len(allErrors) > 0 {
		return allErrors
	}

	// All servers responded successfully – registration complete.
	return nil
}

// DeRegisterSwamp removes a previously registered Swamp pattern from the relevant HydrAIDE server(s).
//
// 🧠 This is the **counterpart of RegisterSwamp()**, and follows the same routing logic:
//   - If the pattern includes any wildcard (e.g. Sanctuary("*"), Realm("*"), or Swamp("*")),
//     the deregistration is propagated to **all** servers.
//   - If the pattern is fully qualified (no wildcards), the request is routed to the exact responsible server
//     using HydrAIDE’s O(1) folder mapping logic.
//
// 🔥 Important notes:
//
// This function **does not delete any data** or existing Swamps — it only removes the **pattern registration**
// from the internal registry. This affects how future pattern-based operations behave.
//
// ✅ **When should you use this?**
//   - When you're deprecating a pattern **permanently**, e.g. restructuring your domain logic.
//   - When you're **migrating** from one pattern to another, and want to avoid potential pattern conflicts.
//   - When your team changes the logic of how logs, sessions, credits, etc. are stored,
//     and you want to cleanly retire the old pattern.
//
// ⚠️ **When should you NOT use this?**
//   - If a Swamp is just temporarily inactive or empty — it will unload itself automatically.
//     There is no need to deregister unless you're redesigning structure.
//
// 🛠️ Typical migration flow:
// 1. Migrate existing data to a new Swamp pattern
// 2. Delete the old Swamp's Treasures (using Delete or DeleteAll)
// 3. Finally, call `DeRegisterSwamp()` to remove the pattern itself
//
// ❗ If you skip step 2, the Swamp files may remain on disk even if the pattern is gone.
//
// Returns:
// - A list of errors if deregistration fails on any server
// - Nil if deregistration completes successfully across all relevant servers
func (h *hydraidego) DeRegisterSwamp(ctx context.Context, swampName name.Name) []error {

	// Container to collect any errors during the deregistration process.
	allErrors := make([]error, 0)

	// Validate that a SwampPattern (name) is provided.
	if swampName == nil {
		allErrors = append(allErrors, fmt.Errorf("SwampPattern is required"))
		return allErrors
	}

	// Determine the list of servers from which the Swamp pattern should be deregistered.
	selectedServers := make([]hydraidepbgo.HydraideServiceClient, 0)

	// If the pattern includes wildcards, deregistration must be broadcast to all known servers,
	// since the Swamp may have been registered on any of them.
	if swampName.IsWildcardPattern() {
		selectedServers = h.client.GetUniqueServiceClients()
	} else {
		// If the pattern is fully qualified (non-wildcard),
		// we resolve it to a specific server based on HydrAIDE's name hashing logic.
		selectedServers = append(selectedServers, h.client.GetServiceClient(swampName))
	}

	// Perform the actual deregistration request on each selected server.
	for _, serviceClient := range selectedServers {

		// Build the DeregisterSwampRequest payload for the gRPC call.
		rsr := &hydraidepbgo.DeRegisterSwampRequest{
			SwampPattern: swampName.Get(),
		}

		// Send the deregistration request to the server.
		_, err := serviceClient.DeRegisterSwamp(ctx, rsr)

		// Handle any errors returned by the gRPC layer and convert them to SDK error codes.
		if err != nil {
			if s, ok := status.FromError(err); ok {
				switch s.Code() {
				case codes.Unavailable:
					allErrors = append(allErrors, NewError(ErrCodeConnectionError, errorMessageConnectionError))
				case codes.DeadlineExceeded:
					allErrors = append(allErrors, NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout))
				case codes.Canceled:
					allErrors = append(allErrors, NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient))
				case codes.InvalidArgument:
					allErrors = append(allErrors, NewError(ErrCodeInvalidArgument, fmt.Sprintf("%s: %v", errorMessageInvalidArgument, s.Message())))
				case codes.NotFound:
					allErrors = append(allErrors, NewError(ErrCodeNotFound, fmt.Sprintf("%s: %v", errorMessageNotFound, s.Message())))
				default:
					allErrors = append(allErrors, NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err)))
				}
			} else {
				allErrors = append(allErrors, NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err)))
			}
		}
	}

	// Return any collected errors if deregistration failed on one or more servers.
	if len(allErrors) > 0 {
		return allErrors
	}

	// Deregistration completed successfully on all target servers.
	return nil
}

// Lock acquires a distributed business-level lock for a specific domain/key.
//
// This is not tied to a single Swamp or Treasure — it’s a **cross-cutting domain lock**.
// You can use it to serialize logic across services or modules that operate
// on the same logical entity (e.g. user ID, order ID, transaction flow).
//
// 🧠 Ideal for scenarios like:
// - Credit transfers between users
// - Order/payment processing pipelines
// - Any sequence of operations where **no other process** should interfere
//
// ⚠️ Locking is **not required** for general Swamp access, reads, or standard writes.
// Use it **only** when your logic depends on critical, exclusive execution.
// Example: You want to deduct 10 credits from UserA and add it to UserB —
// and no other process should modify either user’s balance until this is done.
//
// ⚠️ This is a blocking lock — your flow will **wait** until the lock becomes available.
// The lock is acquired only when no other process holds it.
//
// ➕ The `ttl` ensures the system is self-healing:
// If a client crashes or forgets to unlock, the lock is **automatically released** after the TTL expires.
//
// ⏳ Important context behavior:
//   - If another client holds the lock, your request will block until it's released.
//   - If you set a context timeout or deadline, **make sure it's long enough** for the other process
//     to finish and call `Unlock()` — otherwise you may get a context timeout before acquiring the lock.
//
// ⚠️ The lock is issued **only on the first server**, to ensure consistency across distributed setups.
//
// Parameters:
//   - key:     Unique string representing the business domain to lock (e.g. "user:1234:credit")
//   - ttl:     Time-to-live for the lock. If not unlocked manually, it's auto-released after this duration.
//
// Returns:
// - lockID:   A unique identifier for the acquired lock — must be passed to `Unlock()`.
// - err:      Error if the lock could not be acquired, or if the context expired.
func (h *hydraidego) Lock(ctx context.Context, key string, ttl time.Duration) (lockID string, err error) {

	// Get available servers
	serverClients := h.client.GetUniqueServiceClients()

	// check if the key is empty
	if key == "" {
		return "", NewError(ErrCodeInvalidArgument, fmt.Sprintf("%s: %s", errorMessageInvalidArgument, "key cannot be empty"))
	}

	// check if the TTL is invalid
	if ttl.Milliseconds() <= 1000 {
		return "", NewError(ErrCodeInvalidArgument, fmt.Sprintf("%s: %dms", errorMessageInvalidArgument, ttl.Milliseconds()))
	}

	// Always acquire business-level locks from the first server for consistency
	response, err := serverClients[0].Lock(ctx, &hydraidepbgo.LockRequest{
		Key: key,
		TTL: ttl.Milliseconds(),
	})

	// Handle network and gRPC-specific errors
	if err != nil {
		if s, ok := status.FromError(err); ok {
			switch s.Code() {
			case codes.Unavailable:
				return "", NewError(ErrCodeConnectionError, errorMessageConnectionError)
			case codes.DeadlineExceeded:
				return "", NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
			case codes.Canceled:
				return "", NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient)
			default:
				return "", NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
			}
		} else {
			return "", NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
		}
	}

	// Defensive check in case server returns an empty lockID
	if response.GetLockID() == "" {
		return "", NewError(ErrCodeNotFound, "lock ID not found")
	}

	// Successfully acquired the lock
	return response.GetLockID(), nil
}

// Unlock releases a previously acquired business-level lock using the lock ID.
//
// This is the counterpart of `Lock()`, and must be called once the critical section ends.
// The lock is matched by both key and lock ID, ensuring safety even in multi-client flows.
//
// ⚠️ Unlock always targets the first server — consistency is maintained at the entry point.
//
// Parameters:
// - key:     Same key used during locking (e.g. "user:1234:credit")
// - lockID:  The unique lock identifier returned by Lock()
//
// Returns:
// - err:     If the lock was not found, already released, or an error occurred during release.
func (h *hydraidego) Unlock(ctx context.Context, key string, lockID string) error {

	// Get available servers
	serverClients := h.client.GetUniqueServiceClients()

	// check if the key is empty
	if key == "" {
		return NewError(ErrCodeInvalidArgument, fmt.Sprintf("%s: %s", errorMessageInvalidArgument, "key cannot be empty"))
	}
	// check if the lockID is empty
	if lockID == "" {
		return NewError(ErrCodeInvalidArgument, fmt.Sprintf("%s: %s", errorMessageInvalidArgument, "lock ID cannot be empty"))
	}

	_, err := serverClients[0].Unlock(ctx, &hydraidepbgo.UnlockRequest{
		Key:    key,
		LockID: lockID,
	})

	if err != nil {
		if s, ok := status.FromError(err); ok {
			switch s.Code() {
			case codes.Unavailable:
				return NewError(ErrCodeConnectionError, errorMessageConnectionError)
			case codes.DeadlineExceeded:
				return NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
			case codes.Canceled:
				return NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient)
			case codes.NotFound:
				return NewError(ErrCodeNotFound, "key, or lock ID not found")
			default:
				return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
			}
		} else {
			return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
		}
	}

	// Lock released successfully
	return nil

}

// IsSwampExist checks whether a specific Swamp currently exists in the HydrAIDE system.
//
// ⚠️ This is a **direct existence check** – it does NOT accept wildcards or patterns.
// You must provide a fully resolved Swamp name (Sanctuary + Realm + Swamp).
//
// ✅ When to use this:
// - When you want to check if a Swamp was previously created by another process
// - When a Swamp may have been deleted automatically (e.g., became empty)
// - When you want to determine Swamp presence **without hydrating or loading data**
// - As part of fast lookups, hydration conditionals, or visibility toggles
//
// 🔍 **Real-world example**:
// Suppose you're generating AI analysis per domain and storing them in separate Swamps:
//
//	Sanctuary("domains").Realm("ai").Swamp("trendizz.com")
//	Sanctuary("domains").Realm("ai").Swamp("hydraide.io")
//
// When rendering a UI list of domains, you don’t want to load full AI data.
// Instead, use `IsSwampExist()` to check if an AI analysis exists for each domain,
// and show a ✅ or ❌ icon accordingly — without incurring I/O or memory cost.
//
// ⚙️ Behavior:
// - If the Swamp exists → returns (true, nil)
// - If it never existed or was auto-deleted → returns (false, nil)
// - If the swamp name was nil, the server returns InvalidArgument and the SDK returns (false, ErrCodeNotFound)
// - If a server error occurs → returns (false, error)
//
// 🚀 This check is extremely fast: O(1) routing + metadata lookup.
// ➕ It does **not hydrate or load** the Swamp into memory — it only checks for existence on disk.
//
//	If the Swamp is already open, it stays open. If not, it stays closed.
//	This allows for high-frequency checks without affecting memory or system state.
//
// ⚠️ Requires that the Swamp pattern for the given name was previously registered.
func (h *hydraidego) IsSwampExist(ctx context.Context, swampName name.Name) (bool, error) {

	response, err := h.client.GetServiceClient(swampName).IsSwampExist(ctx, &hydraidepbgo.IsSwampExistRequest{
		IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName: swampName.Get(),
	})

	if err != nil {
		if s, ok := status.FromError(err); ok {
			switch s.Code() {
			case codes.Aborted:
				// HydrAIDE server is shutting down
				return false, NewError(ErrorShuttingDown, errorMessageShuttingDown)
			case codes.Unavailable:
				return false, NewError(ErrCodeConnectionError, errorMessageConnectionError)
			case codes.DeadlineExceeded:
				return false, NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
			case codes.Canceled:
				return false, NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient)
			case codes.InvalidArgument:
				return false, NewError(ErrCodeNotFound, fmt.Sprintf("%s: %v", errorMessageSwampNameNotCorrect, s.Message()))
			case codes.FailedPrecondition:
				// this is not an error, just means swamp does not exist
				return false, nil
			default:
				return false, NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
			}
		} else {
			return false, NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
		}
	}

	if !response.IsExist {
		return false, nil
	}

	return true, nil

}

// IsKeyExists checks whether a specific key exists inside a given Swamp.
//
// 🔍 This is a **memory-aware** check — the Swamp is always hydrated (loaded into memory) as part of this operation.
// If the Swamp was not yet loaded, it will be loaded now and remain in memory based on its registered settings
// (e.g. CloseAfterIdle, persistence, etc.).
//
// ✅ When to use this:
// - When you want to check if a given key has been previously inserted into a Swamp
// - When you're implementing **unique key checks**, deduplication, or conditional inserts
// - When performance is critical and the Swamp is expected to be open or heavily reused
//
// ⚠️ Difference from `IsSwampExist()`:
// `IsSwampExist()` checks for Swamp presence on disk **without hydration**
// `IsKeyExists()` loads the Swamp and searches for the exact key
//
// 🧠 Real-world example:
// In Trendizz.com’s crawler, we keep domain-specific Swamps (e.g. `.hu`, `.de`, `.fr`) open in memory.
// Each Swamp contains a list of already-seen domains.
// Before crawling a new domain, we call `IsKeyExists()` to check if it's already indexed.
// This lets us skip unnecessary work and ensures we don't reprocess the same domain twice.
//
// 🔁 Return values:
// - `(true, nil)` → Swamp and key both exist
// - `(false, nil)` → Swamp exists, but key does not
// - `(false, ErrCodeSwampNotFound)` → Swamp does not exist
// - `(false, <other error>)` → Some database/server issue occurred
//
// ⚠️ Always use **fully qualified Swamp names** – no wildcards allowed.
// If the Swamp was not registered, or was deleted due to being empty, this will return `ErrCodeSwampNotFound`.
func (h *hydraidego) IsKeyExists(ctx context.Context, swampName name.Name, key string) (bool, error) {

	response, err := h.client.GetServiceClient(swampName).IsKeyExist(ctx, &hydraidepbgo.IsKeyExistRequest{
		IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName: swampName.Get(),
		Key:       key,
	})

	if err != nil {
		if s, ok := status.FromError(err); ok {
			switch s.Code() {
			case codes.Aborted:
				// HydrAIDE server is shutting down
				return false, NewError(ErrorShuttingDown, errorMessageShuttingDown)
			case codes.Unavailable:
				return false, NewError(ErrCodeConnectionError, errorMessageConnectionError)
			case codes.DeadlineExceeded:
				return false, NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
			case codes.Canceled:
				return false, NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient)
			case codes.InvalidArgument:
				return false, NewError(ErrCodeNotFound, fmt.Sprintf("%s: %v", errorMessageInvalidArgument, s.Message()))
			case codes.FailedPrecondition:
				return false, NewError(ErrCodeSwampNotFound, fmt.Sprintf("%s: %v", errorMessageNotFound, s.Message()))
			default:
				return false, NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
			}
		} else {
			return false, NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
		}
	}

	if !response.IsExist {
		return false, nil
	}

	return true, nil

}

// CatalogCreate inserts a new Treasure into a Swamp using a tagged Go struct as the input model.
//
// 🧠 Purpose:
// This is a "catalog-style" insert — ideal for Swamps that hold a large number of similar entities,
// like users, transactions, credit logs, etc. Think of it as writing a row into a virtual table.
//
// ✅ Behavior:
// - Creates the Swamp if it does not exist yet
// - Inserts the provided key-value pair **only if the key does not already exist**
// - Will return an error if the key already exists
//
// 📦 Model requirements:
// - The `model` must be a **pointer to a struct**
// - The struct must contain a `hydraide:"key"` field with a non-empty string
// - Optionally, it may contain:
//
//   - `hydraide:"value"` → the main value of the Treasure.
//
//     ✅ Supported types:
//
//   - string, bool
//
//   - int8, int16, int32, int64
//
//   - uint8, uint16, uint32, uint64
//
//   - float32, float64
//
//   - struct or pointer to struct (automatically GOB-encoded)
//
//     🔬 Best practice:
//     Always use the **smallest suitable numeric type**.
//     For example: prefer `uint8` or `int16` over `int`.
//     HydrAIDE stores values in raw binary form — so smaller types directly reduce
//     memory usage and disk space.
//
//   - `hydraide:"expireAt"`   → expiration logic (time.Time)
//
//   - `hydraide:"createdBy"`  → who created it (string)
//
//   - `hydraide:"createdAt"`  → when it was created (time.Time)
//
//   - `hydraide:"updatedBy"`  → optional metadata
//
//   - `hydraide:"updatedAt"`  → optional metadata
//
// ✨ Example use case 1:
// You store user records in a Swamp:
//
//	Sanctuary("system").Realm("users").Swamp("all")
//
// Each call to `CatalogCreate()` adds a new user — uniquely identified by `UserUUID`.
//
// ✨ Example use case 2 (real-world):
// In Trendizz.com’s domain crawler, we store known domains in Swamps per TLD.
// Instead of first checking if a domain exists, we call `CatalogCreate()` directly.
// If the domain already exists → we receive `ErrCodeAlreadyExists`.
// If it doesn’t → it is inserted in one step.
// This saves a read roundtrip and simplifies the control flow.
//
// 🔁 Return values:
// - `nil` → success, insert completed
// - `ErrCodeAlreadyExists` → key already exists in the Swamp
// - `ErrCodeInvalidModel` → struct is invalid (e.g. not a pointer, missing tags)
// - Other database-level error codes if something went wrong
// Example: CreditLog model used with CatalogCreate()
// This model stores credit-related changes per user in a Swamp.
// Each record is identified by UserUUID and optionally enriched with metadata.
func (h *hydraidego) CatalogCreate(ctx context.Context, swampName name.Name, model any) error {

	kvPair, err := convertCatalogModelToKeyValuePair(model, h.getEncodingForSwamp(swampName))
	if err != nil {
		return NewError(ErrCodeInvalidModel, err.Error())
	}

	// egyetlen adatot hozunk létre a hydrában overwrit nélkül.
	// A swamp mindenképpen létrejön, ha még nem létezett, de a kulcs csak akkor, ha még nem létezett
	setResponse, err := h.client.GetServiceClient(swampName).Set(ctx, &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
				SwampName: swampName.Get(),
				KeyValues: []*hydraidepbgo.KeyValuePair{
					kvPair,
				},
				CreateIfNotExist: true,
				Overwrite:        false,
			},
		},
	})

	if err != nil {
		if s, ok := status.FromError(err); ok {
			switch s.Code() {
			case codes.Aborted:
				// HydrAIDE server is shutting down
				return NewError(ErrorShuttingDown, errorMessageShuttingDown)
			case codes.Unavailable:
				return NewError(ErrCodeConnectionError, errorMessageConnectionError)
			case codes.DeadlineExceeded:
				return NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
			case codes.Canceled:
				return NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient)
			case codes.Internal:
				return NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("%s: %v", errorMessageInternalError, s.Message()))
			default:
				return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
			}
		} else {
			return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
		}
	}

	// if the key already exists, the status will be NOTHIN_CHANGED
	for _, swamp := range setResponse.GetSwamps() {
		for _, kv := range swamp.GetKeysAndStatuses() {
			if kv.GetStatus() == hydraidepbgo.Status_NOTHING_CHANGED {
				return NewError(ErrCodeAlreadyExists, errorMessageKeyAlreadyExists)
			}
		}
	}

	return nil

}

type CreateManyIteratorFunc func(key string, err error) error

// CatalogCreateMany inserts multiple Treasures into a single Swamp using catalog-style logic.
//
// 🧠 Purpose:
// Use this when you want to batch-insert a list of records into a Swamp,
// each with its own unique key — for example, uploading multiple users, products,
// or log entries at once.
//
// ✅ Behavior:
// - Creates the Swamp if it does not exist yet
// - Converts each input model into a KeyValuePair using `convertCatalogModelToKeyValuePair()`
// - Inserts all items in a single SetRequest
// - Fails **only** if the gRPC call fails or if a model is invalid
//
// 📦 Model Requirements:
// Each element in `models` must be a pointer to a struct,
// and follow the same field tagging rules as in `CatalogCreate()`:
//   - `hydraide:"key"`     → required non-empty string
//   - `hydraide:"value"`   → optional value (primitive, struct, pointer)
//   - `hydraide:"createdAt"`, `expireAt`, etc. → optional metadata
//
// 🔁 Iterator (optional):
// You may provide an `iterator` function to handle per-record responses.
// It will be called for each inserted item with:
//
//	key string  → the unique Treasure key
//	err error   → nil if inserted successfully,
//	              `ErrCodeAlreadyExists` if the key was already present
//
// This allows you to track insert success/failure **per item**, without manually parsing the response.
//
// If `iterator` is `nil`, the function will insert all models silently, and return only global errors.
//
// ✨ Example use:
//
//	var users []any = []any{&User1, &User2, &User3}
//	err := client.CatalogCreateMany(ctx, name.Swamp("users", "all", "2025"), users, func(key string, err error) error {
//	    if err != nil {
//	        log.Printf("❌ failed to insert %s: %v", key, err)
//	    } else {
//	        log.Printf("✅ inserted: %s", key)
//	    }
//	    return nil
//	})
//
// 🧯 Error Handling:
// - If any model is invalid → `ErrCodeInvalidModel`
// - If the entire gRPC Set call fails → appropriate connection or database error
// - If a key already exists → passed back through the iterator as `ErrCodeAlreadyExists`
// - If no iterator is provided, duplicates are silently skipped
//
// 🔁 Return:
//   - `nil` if the operation succeeded and/or the iterator handled everything
//   - Any error returned by the iterator will abort processing and be returned immediately
//   - If the underlying gRPC Set request fails (e.g. connection error, database failure),
//     the function returns a global error (e.g. ErrCodeConnectionError, ErrCodeInternalDatabaseError, etc.)
func (h *hydraidego) CatalogCreateMany(ctx context.Context, swampName name.Name, models []any, iterator CreateManyIteratorFunc) error {

	encoding := h.getEncodingForSwamp(swampName)
	kvPairs := make([]*hydraidepbgo.KeyValuePair, 0, len(models))

	for _, model := range models {
		kvPair, err := convertCatalogModelToKeyValuePair(model, encoding)
		if err != nil {
			return NewError(ErrCodeInvalidModel, err.Error())
		}
		kvPairs = append(kvPairs, kvPair)
	}

	setResponse, err := h.client.GetServiceClient(swampName).Set(ctx, &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				IslandID:         swampName.GetIslandID(h.client.GetAllIslands()),
				SwampName:        swampName.Get(),
				KeyValues:        kvPairs,
				CreateIfNotExist: true,
				Overwrite:        false,
			},
		},
	})

	if err != nil {
		if s, ok := status.FromError(err); ok {
			switch s.Code() {
			case codes.Aborted:
				// HydrAIDE server is shutting down
				return NewError(ErrorShuttingDown, errorMessageShuttingDown)
			case codes.Unavailable:
				return NewError(ErrCodeConnectionError, errorMessageConnectionError)
			case codes.DeadlineExceeded:
				return NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
			case codes.Canceled:
				return NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient)
			case codes.Internal:
				return NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("%s: %v", errorMessageInternalError, s.Message()))
			default:
				return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
			}
		} else {
			return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
		}
	}

	// process the response and start the iterator if the iterator is not nil
	if iterator != nil {
		for _, swamp := range setResponse.GetSwamps() {
			for _, kv := range swamp.GetKeysAndStatuses() {
				if kv.GetStatus() == hydraidepbgo.Status_NOTHING_CHANGED {
					if iterErr := iterator(kv.GetKey(), NewError(ErrCodeAlreadyExists, errorMessageKeyAlreadyExists)); iterErr != nil {
						return iterErr
					}
				} else {
					if iterErr := iterator(kv.GetKey(), nil); iterErr != nil {
						return iterErr
					}
				}
			}
		}
	}

	return nil

}

type CatalogCreateManyToManyIteratorFunc func(swampName name.Name, key string, err error) error

type CatalogManyToManyRequest struct {
	SwampName name.Name
	Models    []any
}

// CatalogCreateManyToMany inserts batches of catalog-style models across multiple Swamps,
// and intelligently groups requests per server to optimize communication.
//
// 🧠 Use this function when:
// - You want to insert data into multiple Swamps (e.g. one per user, per region, per domain)
// - The Swamps may be distributed across multiple HydrAIDE servers
// - You want to minimize the number of gRPC calls and batch multiple writes per server
//
// ✅ Behavior:
// - Groups all SwampRequests by their destination server (based on Swamp name hashing)
// - Sends **one SetRequest per server**, bundling all Swamps and KeyValuePairs
// - Converts each model using `convertCatalogModelToKeyValuePair()`
// - Automatically creates Swamps if they don't exist
// - Does **not overwrite existing keys**
//
// 🔁 Iterator function (optional):
//
//	If provided, it will be called for every inserted key, with:
//	- swampName: the Swamp where the key was written
//	- key: the actual key
//	- err: nil if success, or ErrCodeAlreadyExists if key already existed
//
//	You can use this to log, retry, or track insert results.
//
// Example:
//
//	requests := []*CatalogManyToManyRequest{
//	    {
//	        SwampName: name.New().Sanctuary("domains").Realm("ai").Swamp("hu"),
//	        Models: []any{...},
//	    },
//	    {
//	        SwampName: name.New().Sanctuary("domains").Realm("ai").Swamp("de"),
//	        Models: []any{...},
//	    },
//	    {
//	        SwampName: name.New().Sanctuary("domains").Realm("ai").Swamp("fr"),
//	        Models: []any{...},
//	    },
//	}
//
//	err := client.CatalogCreateManyToMany(ctx, requests, func(swamp name.Name, key string, err error) error {
//	    if err != nil {
//	        log.Printf("❌ failed to insert %s into %s: %v", key, swamp.Get(), err)
//	    } else {
//	        log.Printf("✅ inserted %s into %s", key, swamp.Get())
//	    }
//	    return nil
//	})
//
// 🔥 Ideal for:
// - Crawler results
// - Batch imports
// - Indexing pipelines
// - Data normalization jobs
//
// 🧯 Errors:
// - Any invalid model → `ErrCodeInvalidModel`
// - gRPC/connection errors → mapped to consistent SDK error codes
// - Iterator errors → if the callback returns a non-nil error, processing stops immediately
func (h *hydraidego) CatalogCreateManyToMany(ctx context.Context, request []*CatalogManyToManyRequest, iterator CatalogCreateManyToManyIteratorFunc) error {

	type requestGroup struct {
		client        hydraidepbgo.HydraideServiceClient
		swampRequests []*hydraidepbgo.SwampRequest
	}

	serverRequests := make(map[string]*requestGroup)

	for _, req := range request {

		// lekérdezzük a szewrver adatait a swamp neve alapján
		clientAndHost := h.client.GetServiceClientAndHost(req.SwampName)

		if _, ok := serverRequests[clientAndHost.Host]; !ok {
			serverRequests[clientAndHost.Host] = &requestGroup{
				client: clientAndHost.GrpcClient,
			}
		}

		encoding := h.getEncodingForSwamp(req.SwampName)
		kvPairs := make([]*hydraidepbgo.KeyValuePair, 0, len(req.Models))

		for _, model := range req.Models {
			kvPair, err := convertCatalogModelToKeyValuePair(model, encoding)
			if err != nil {
				return NewError(ErrCodeInvalidModel, err.Error())
			}
			kvPairs = append(kvPairs, kvPair)
		}

		serverRequests[clientAndHost.Host].swampRequests = append(serverRequests[clientAndHost.Host].swampRequests, &hydraidepbgo.SwampRequest{
			IslandID:         req.SwampName.GetIslandID(h.client.GetAllIslands()),
			SwampName:        req.SwampName.Get(),
			KeyValues:        kvPairs,
			CreateIfNotExist: true,
			Overwrite:        false,
		})

	}

	for _, reqGroup := range serverRequests {

		setResponse, err := reqGroup.client.Set(ctx, &hydraidepbgo.SetRequest{
			Swamps: reqGroup.swampRequests,
		})

		if err != nil {
			if s, ok := status.FromError(err); ok {
				switch s.Code() {
				case codes.Aborted:
					// HydrAIDE server is shutting down
					return NewError(ErrorShuttingDown, errorMessageShuttingDown)
				case codes.Unavailable:
					return NewError(ErrCodeConnectionError, errorMessageConnectionError)
				case codes.DeadlineExceeded:
					return NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
				case codes.Canceled:
					return NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient)
				case codes.InvalidArgument:
					return NewError(ErrCodeInvalidArgument, fmt.Sprintf("%s: %v", errorMessageInvalidArgument, s.Message()))
				case codes.Internal:
					return NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("%s: %v", errorMessageInternalError, s.Message()))
				default:
					return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
				}
			} else {
				return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
			}
		}

		// process the response and start the iterator if the iterator is not nil
		if iterator != nil {
			for _, swamp := range setResponse.GetSwamps() {
				swampNameObj := name.Load(swamp.GetSwampName())
				// végigmegyünk a kulcsokon és azok státuszán
				for _, kv := range swamp.GetKeysAndStatuses() {
					if kv.GetStatus() == hydraidepbgo.Status_NOTHING_CHANGED {
						if iterErr := iterator(swampNameObj, kv.GetKey(), NewError(ErrCodeAlreadyExists, errorMessageKeyAlreadyExists)); iterErr != nil {
							return iterErr
						}
					} else {
						if iterErr := iterator(swampNameObj, kv.GetKey(), nil); iterErr != nil {
							return iterErr
						}
					}
				}
			}
		}

	}

	return nil

}

// CatalogRead retrieves a single Treasure by key from the specified Swamp,
// and unmarshals the result into the provided Go model.
//
// 🧠 Use this function when:
// - You want to read a single key from a Swamp
// - You want the result directly mapped into a typed struct
// - You need reliable error codes (e.g. key not found, invalid model, etc.)
//
// ✅ Behavior:
// - Sends a GetRequest to the Hydra server responsible for the Swamp
// - Extracts the first returned Treasure (if exists)
// - Automatically unmarshals the value into the given struct using field tags
//   - Required: `hydraide:"key"`
//   - Optional: `hydraide:"value"`, `expireAt`, `createdBy`, etc.
//
// - Supports GOB-decoded slices, maps, pointers, and all primitive types
//
// 📌 Notes:
// - The model parameter must be a pointer to a struct
// - Only one Treasure is expected — if none found, returns ErrCodeNotFound
//
// 🔥 Ideal for:
// - Real-time lookups
// - Detail views
// - Conditional logic (e.g. check if user already exists)
//
// 🧯 Errors:
// - Key not found → `ErrCodeNotFound`
// - Invalid model or conversion error → `ErrCodeInvalidModel`
// - Swamp not found → `ErrCodeSwampNotFound`
// - Timeout / context / network issues → appropriate SDK error codes
func (h *hydraidego) CatalogRead(ctx context.Context, swampName name.Name, key string, model any) error {

	swamps := []*hydraidepbgo.GetSwamp{
		{
			IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
			SwampName: swampName.Get(),
			Keys:      []string{key},
		},
	}

	response, err := h.client.GetServiceClient(swampName).Get(ctx, &hydraidepbgo.GetRequest{
		Swamps: swamps,
	})

	if err != nil {
		return errorHandler(err)
	}

	for _, swamp := range response.GetSwamps() {
		for _, treasure := range swamp.GetTreasures() {
			if treasure.IsExist == false {
				return NewError(ErrCodeNotFound, "key not found")
			}

			if convErr := convertProtoTreasureToCatalogModel(treasure, model); convErr != nil {
				return NewError(ErrCodeInvalidModel, convErr.Error())
			}
			return nil
		}
	}

	return NewError(ErrCodeNotFound, "key not found")

}

type CatalogReadManyIteratorFunc func(model any) error

// CatalogReadMany reads a set of Treasures from a Swamp using the provided Index, and applies a callback to each.
//
// This function enables high-performance, filtered reads from a Swamp based on a preconstructed Index,
// and feeds each unmarshaled result into a user-defined iterator function.
//
// ✅ Use when you want to:
//   - Stream filtered results from a Swamp using index-based logic
//   - Unmarshal Treasures into a typed model
//   - Apply business logic or collect results via a custom iterator
//
// ⚙️ Parameters:
//   - ctx: Context for cancellation and timeout.
//   - swampName: The logical name of the Swamp to query.
//   - index: A non-nil Index instance describing how to filter, order, and limit the read.
//   - model: A non-pointer struct type. Used as the template for unmarshaling Treasures.
//   - iterator: A non-nil function that is called once per result. Returning an error stops the loop.
//
// ⚠️ Requirements:
//   - `index` must not be nil — otherwise the call fails.
//   - `iterator` must not be nil — otherwise the call fails.
//   - `model` must be a **non-pointer** struct. Pointer types will cause an error response.
//
// 📦 Behavior:
//   - Internally calls Hydra’s `GetByIndex` gRPC method to fetch raw Treasures.
//   - Skips non-existing (`IsExist == false`) entries silently.
//   - For each result, creates a new instance of the model type, fills it from the Treasure,
//     and passes it to `iterator`.
//   - If `iterator` returns an error, iteration halts and the same error is returned.
//
// 🧠 Philosophy:
//   - Zero shared state: every call is isolated and memory-safe.
//   - The function is sync and respects the calling thread/context.
//   - Ideal for streaming reads, pipelines, transformations.
func (h *hydraidego) CatalogReadMany(ctx context.Context, swampName name.Name, index *Index, model any, iterator CatalogReadManyIteratorFunc) error {

	// Validate required parameters
	if index == nil {
		return NewError(ErrCodeInvalidArgument, "index can not be nil")
	}
	if iterator == nil {
		return NewError(ErrCodeInvalidArgument, "iterator can not be nil")
	}

	// Ensure that the model is not a pointer type (we create new instances internally)
	if reflect.TypeOf(model).Kind() == reflect.Ptr {
		return NewError(ErrCodeInvalidArgument, "model cannot be a pointer")
	}

	// Convert index type and order into the proto format expected by the backend
	indexTypeProtoFormat := convertIndexTypeToProtoIndexType(index.IndexType)
	orderTypeProtoFormat := convertOrderTypeToProtoOrderType(index.IndexOrder)

	indexRequest := &hydraidepbgo.GetByIndexRequest{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		IndexType:   indexTypeProtoFormat,
		OrderType:   orderTypeProtoFormat,
		From:        index.From,
		Limit:       index.Limit,
		ExcludeKeys:  index.ExcludeKeys,
		IncludedKeys: index.IncludedKeys,
		KeysOnly:     index.KeysOnly,
	}

	indexRequest.FromTime = toOptionalTimestamppb(index.FromTime)
	indexRequest.ToTime = toOptionalTimestamppb(index.ToTime)

	// Fetch all matching Treasures from the Hydra engine based on the Index parameters
	response, err := h.client.GetServiceClient(swampName).GetByIndex(ctx, indexRequest)

	if err != nil {
		return errorHandler(err)
	}

	// Iterate through each returned Treasure and convert it into a usable model instance
	for _, treasure := range response.GetTreasures() {

		// Skip non-existent records
		if treasure.IsExist == false {
			continue
		}

		// Create a fresh instance of the model (we clone the type, not the original value)
		modelValue := reflect.New(reflect.TypeOf(model)).Interface()

		// Unmarshal the Treasure into the model using the internal conversion logic
		if convErr := convertProtoTreasureToCatalogModel(treasure, modelValue); convErr != nil {
			return NewError(ErrCodeInvalidModel, convErr.Error())
		}

		// Pass the result to the user-provided iterator function
		// If it returns an error, halt iteration and return the error
		if iterErr := iterator(modelValue); iterErr != nil {
			return iterErr
		}
	}

	// If we reached here, everything was successful
	return nil
}

// CatalogReadBatch retrieves multiple Treasures by their keys from the specified Swamp,
// and unmarshals each result into a fresh instance of the provided model type,
// passing it to an iterator function for processing.
//
// 🧠 Use this function when:
// - You want to read many specific keys from a single Swamp in one request
// - You know the exact keys you need (no filtering or indexing required)
// - You want to process each result in a streaming fashion via an iterator
//
// ✅ Behavior:
// - Sends a single GetByKeys gRPC request with all the provided keys
// - Fetches all existing Treasures matching the keys in one batch call
// - Missing keys are silently ignored (no error, just not included in results)
// - For each returned Treasure, creates a new instance of the model type
// - Unmarshals the Treasure data into the model
// - Calls the iterator function with the unmarshaled model
// - If the iterator returns an error, stops processing and returns that error
//
// ⚙️ Parameters:
//   - ctx: Context for cancellation and timeout
//   - swampName: The logical name of the Swamp to read from
//   - keys: A slice of keys to retrieve. Empty slice returns immediately with no error
//   - model: A non-pointer struct type used as template for unmarshaling each Treasure
//   - iterator: A non-nil function called for each found Treasure
//
// ⚠️ Requirements:
//   - `iterator` must not be nil — otherwise the call fails
//   - `model` must be a non-pointer struct — pointers will cause an error
//
// 📦 Behavior:
//   - Internally calls GetByKeys gRPC method to fetch raw Treasures
//   - Skips non-existing (`IsExist == false`) entries silently
//   - For each result, creates a new instance of the model type, fills it from the Treasure,
//     and passes it to the iterator
//   - If iterator returns an error, iteration halts and the same error is returned
//
// 🧠 Philosophy:
//   - Zero shared state: every call is isolated and memory-safe
//   - The function is sync and respects the calling thread/context
//   - Ideal for batch lookups, bulk reads, cache warming
//
// 🔥 Ideal for:
// - Fetching user profiles by a list of IDs
// - Bulk data validation
// - Reading configuration entries
// - Cache population
// - Multi-key lookup operations (30-50× faster than multiple single reads)
//
// Example:
//
//	keys := []string{"user:1", "user:2", "user:3"}
//	var users []User
//	err := client.CatalogReadBatch(ctx, swampName, keys, User{}, func(model any) error {
//	    user := model.(*User)
//	    users = append(users, *user)
//	    return nil
//	})
//
// 🧯 Errors:
// - If iterator is nil → `ErrCodeInvalidArgument`
// - If model is a pointer → `ErrCodeInvalidArgument`
// - Invalid model conversion → `ErrCodeInvalidModel`
// - gRPC/connection errors → mapped to consistent SDK error codes
func (h *hydraidego) CatalogReadBatch(ctx context.Context, swampName name.Name, keys []string, model any, iterator CatalogReadManyIteratorFunc) error {

	// Validate required parameters
	if iterator == nil {
		return NewError(ErrCodeInvalidArgument, "iterator can not be nil")
	}

	// Ensure that the model is not a pointer type (we create new instances internally)
	if reflect.TypeOf(model).Kind() == reflect.Ptr {
		return NewError(ErrCodeInvalidArgument, "model cannot be a pointer")
	}

	// If no keys provided, return early (not an error, just nothing to do)
	if len(keys) == 0 {
		return nil
	}

	// Create the gRPC request for batch key retrieval
	getByKeysRequest := &hydraidepbgo.GetByKeysRequest{
		IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName: swampName.Get(),
		Keys:      keys,
	}

	// Fetch all matching Treasures from the Hydra engine
	response, err := h.client.GetServiceClient(swampName).GetByKeys(ctx, getByKeysRequest)

	if err != nil {
		return errorHandler(err)
	}

	// Iterate through each returned Treasure and convert it into a usable model instance
	for _, treasure := range response.GetTreasures() {

		// Skip non-existent records
		if treasure.IsExist == false {
			continue
		}

		// Create a fresh instance of the model (we clone the type, not the original value)
		modelValue := reflect.New(reflect.TypeOf(model)).Interface()

		// Unmarshal the Treasure into the model using the internal conversion logic
		if convErr := convertProtoTreasureToCatalogModel(treasure, modelValue); convErr != nil {
			return NewError(ErrCodeInvalidModel, convErr.Error())
		}

		// Pass the result to the user-provided iterator function
		// If it returns an error, halt iteration and return the error
		if iterErr := iterator(modelValue); iterErr != nil {
			return iterErr
		}
	}

	// If we reached here, everything was successful
	return nil
}

// CatalogUpdate updates a single existing Treasure inside a given Swamp.
//
// This method performs an *in-place update* based on the key derived from the provided model.
// It will NOT create the Swamp or the key if they do not already exist.
// If the Swamp or key is missing, a descriptive error will be returned.
//
// ✅ Use when:
//   - You want to overwrite an existing value in a Swamp
//   - You already know the key exists and just want to update its content
//
// ⚠️ Constraints:
//   - `model` must not be nil
//   - `model` must implement a valid key via `hydrun:"key"`
//   - The Swamp and key must already exist
//
// 🧠 Behavior:
//   - Converts the model to a typed binary KeyValuePair
//   - Sends an update (not insert) request to the Hydra engine
//   - If the key or Swamp doesn’t exist, returns a clear error
//
// 🛠️ No creation. No upsert. Just pure update.
func (h *hydraidego) CatalogUpdate(ctx context.Context, swampName name.Name, model any) error {

	// Ensure the model is provided
	if model == nil {
		return NewError(ErrCodeInvalidModel, "model is nil")
	}

	// Convert the model into a typed key-value pair based on struct tags and reflection
	kvPair, err := convertCatalogModelToKeyValuePair(model, h.getEncodingForSwamp(swampName))
	if err != nil {
		return NewError(ErrCodeInvalidModel, err.Error())
	}

	// Send a Set request to update the value in Hydra
	// Note:
	// - CreateIfNotExist = false → Swamp must already exist
	// - Overwrite = true         → Overwrite existing key, but do NOT create new key
	response, err := h.client.GetServiceClient(swampName).Set(ctx, &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				IslandID:         swampName.GetIslandID(h.client.GetAllIslands()),
				SwampName:        swampName.Get(),
				KeyValues:        []*hydraidepbgo.KeyValuePair{kvPair},
				CreateIfNotExist: false,
				Overwrite:        true,
			},
		},
	})

	// Handle potential gRPC or Hydra-specific errors
	if err != nil {
		if s, ok := status.FromError(err); ok {
			switch s.Code() {
			case codes.Aborted:
				// HydrAIDE server is shutting down
				return NewError(ErrorShuttingDown, errorMessageShuttingDown)
			case codes.Unavailable:
				return NewError(ErrCodeConnectionError, errorMessageConnectionError)
			case codes.DeadlineExceeded:
				return NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
			case codes.Canceled:
				return NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient)
			case codes.Internal:
				return NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("%s: %v", errorMessageInternalError, s.Message()))
			default:
				return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
			}
		}
		// Non-gRPC error
		return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
	}

	// Check if the Swamp exists in the response
	for _, swamp := range response.GetSwamps() {
		if swamp.GetErrorCode() == hydraidepbgo.SwampResponse_SwampDoesNotExist {
			return NewError(ErrCodeSwampNotFound, errorMessageSwampNotFound)
		}

		// Check if the key was actually found and updated
		for _, kStatus := range swamp.GetKeysAndStatuses() {
			if kStatus.GetStatus() == hydraidepbgo.Status_NOT_FOUND {
				return NewError(ErrCodeNotFound, errorMessageKeyNotFound)
			}
		}
	}

	// Success — the update was completed
	return nil
}

type CatalogUpdateManyIteratorFunc func(key string, status EventStatus) error

// CatalogUpdateMany updates multiple existing Treasures inside a single Swamp.
//
// This is a batch-safe operation that performs a non-creating update:
// it will only update Treasures that already exist — and will skip or report keys that don’t.
//
// ✅ Use when:
//   - You want to update many Treasures at once (bulk overwrite)
//   - You want to ensure that no new Treasures are accidentally created
//   - You want per-Treasure feedback using a callback
//
// ⚠️ Constraints:
//   - Treasures that do not exist will not be created
//   - The Swamp must already exist
//   - The `iterator` (if provided) will receive a status per key
//
// 💡 Typical use case:
//   - Audit-safe batch update: "only touch existing records"
//   - Change tracking: get status feedback per update
//
// 🧠 Behavior:
//   - Converts each model to a binary KeyValuePair
//   - Sends them in a single Set request with overwrite-only behavior
//   - Streams each key’s result status to the provided iterator
//   - Iterator can early-return with error to abort processing
func (h *hydraidego) CatalogUpdateMany(ctx context.Context, swampName name.Name, models []any, iterator CatalogUpdateManyIteratorFunc) error {

	// Ensure models slice is not nil
	if models == nil {
		return NewError(ErrCodeInvalidModel, "model is nil")
	}

	// Convert all models to KeyValuePair (binary form)
	encoding := h.getEncodingForSwamp(swampName)
	kvPairs := make([]*hydraidepbgo.KeyValuePair, 0, len(models))
	for _, model := range models {
		kvPair, err := convertCatalogModelToKeyValuePair(model, encoding)
		if err != nil {
			return NewError(ErrCodeInvalidModel, err.Error())
		}
		kvPairs = append(kvPairs, kvPair)
	}

	// Perform the batch Set request
	// Note:
	// - CreateIfNotExist = false → No new Swamps will be created
	// - Overwrite = true         → Only update existing keys
	response, err := h.client.GetServiceClient(swampName).Set(ctx, &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				IslandID:         swampName.GetIslandID(h.client.GetAllIslands()),
				SwampName:        swampName.Get(),
				KeyValues:        kvPairs,
				CreateIfNotExist: false,
				Overwrite:        true,
			},
		},
	})

	// Handle transport or protocol-level errors
	if err != nil {
		if s, ok := status.FromError(err); ok {
			switch s.Code() {
			case codes.Aborted:
				// HydrAIDE server is shutting down
				return NewError(ErrorShuttingDown, errorMessageShuttingDown)
			case codes.Unavailable:
				return NewError(ErrCodeConnectionError, errorMessageConnectionError)
			case codes.DeadlineExceeded:
				return NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
			case codes.Canceled:
				return NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient)
			case codes.Internal:
				return NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("%s: %v", errorMessageInternalError, s.Message()))
			default:
				return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
			}
		}
		return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
	}

	// If an iterator is provided, report status per key
	if iterator != nil {
		for _, swamp := range response.GetSwamps() {

			// Report if the entire Swamp was not found
			if swamp.GetErrorCode() == hydraidepbgo.SwampResponse_SwampDoesNotExist {
				if iterErr := iterator("", StatusSwampNotFound); iterErr != nil {
					return iterErr
				}
			}

			// Report status per Treasure (key)
			for _, kStatus := range swamp.GetKeysAndStatuses() {
				stat := convertProtoStatusToStatus(kStatus.GetStatus())
				if iterErr := iterator(kStatus.GetKey(), stat); iterErr != nil {
					return iterErr
				}
			}
		}
	}

	// All updates and iteration finished successfully
	return nil
}

// CatalogDelete removes a single Treasure from a given Swamp by key.
//
// This operation performs a hard delete. If the key exists, it is removed immediately.
// If the key is the last in the Swamp, the entire Swamp is also deleted.
//
// ✅ Use when:
//   - You want to permanently delete a Treasure by its key
//   - You want automatic cleanup of empty Swamps (zero-state)
//
// ⚠️ Behavior:
//   - If the Swamp does not exist → returns ErrCodeSwampNotFound
//   - If the key does not exist   → returns ErrCodeNotFound
//   - If deletion is successful   → returns nil
//   - If the deleted Treasure was the last → the Swamp folder is removed entirely
//
// 💡 This is an idempotent operation: calling it on a non-existent key is safe, but results in error.
func (h *hydraidego) CatalogDelete(ctx context.Context, swampName name.Name, key string) error {

	// Send a delete request for the specified key inside the given Swamp
	response, err := h.client.GetServiceClient(swampName).Delete(ctx, &hydraidepbgo.DeleteRequest{
		Swamps: []*hydraidepbgo.DeleteRequest_SwampKeys{
			{
				IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
				SwampName: swampName.Get(),
				Keys:      []string{key},
			},
		},
	})

	// Handle transport or protocol-level errors
	if err != nil {
		return errorHandler(err)
	}

	// Iterate over Swamp-level responses
	for _, r := range response.GetResponses() {

		// If the Swamp doesn't exist at all, return specific error
		if r.ErrorCode != nil && r.GetErrorCode() == hydraidepbgo.DeleteResponse_SwampDeleteResponse_SwampDoesNotExist {
			return NewError(ErrCodeSwampNotFound, errorMessageSwampNotFound)
		}

		// Check per-key deletion status
		for _, ksPair := range r.GetKeyStatuses() {
			switch ksPair.GetStatus() {

			// Key was not found in the Swamp
			case hydraidepbgo.Status_NOT_FOUND:
				return NewError(ErrCodeNotFound, errorMessageKeyNotFound)

			// Key was successfully deleted
			case hydraidepbgo.Status_DELETED:
				return nil
			}
		}
	}

	// If no status matched or something unexpected happened
	return NewError(ErrCodeUnknown, errorMessageUnknown)
}

type CatalogDeleteIteratorFunc func(key string, err error) error

// CatalogDeleteMany removes multiple Treasures from a single Swamp by key.
//
// This batch operation performs hard deletes across multiple keys in one request.
// It does **not** create or ignore missing Swamps or Treasures — instead, it explicitly reports each outcome.
//
// If provided, the `iterator` callback will be invoked once for each processed key (or Swamp-level error),
// allowing custom error handling, metrics, or conditional flow control.
//
// ✅ Use when:
//   - You want to delete many Treasures at once
//   - You want to handle each deletion result individually
//   - You need full visibility into what was deleted, not found, or failed
//
// ⚠️ Behavior:
//   - If the Swamp does not exist → `iterator("", ErrCodeSwampNotFound)`
//   - If a key does not exist     → `iterator(key, ErrCodeNotFound)`
//   - If a key is deleted         → `iterator(key, nil)`
//   - If `iterator` returns an error → iteration stops immediately and the same error is returned
//
// 💡 Swamps with zero Treasures left after deletion are automatically removed.
func (h *hydraidego) CatalogDeleteMany(ctx context.Context, swampName name.Name, keys []string, iterator CatalogDeleteIteratorFunc) error {

	// Send a bulk delete request to Hydra for all specified keys
	response, err := h.client.GetServiceClient(swampName).Delete(ctx, &hydraidepbgo.DeleteRequest{
		Swamps: []*hydraidepbgo.DeleteRequest_SwampKeys{
			{
				IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
				SwampName: swampName.Get(),
				Keys:      keys,
			},
		},
	})
	if err != nil {
		return errorHandler(err)
	}

	// If an iterator is provided, walk through all results and emit per-key outcome
	if iterator != nil {
		for _, r := range response.GetResponses() {

			// Swamp-level error: Swamp not found
			if r.ErrorCode != nil && r.GetErrorCode() == hydraidepbgo.DeleteResponse_SwampDeleteResponse_SwampDoesNotExist {
				if iterErr := iterator("", NewError(ErrCodeSwampNotFound, errorMessageSwampNotFound)); iterErr != nil {
					return iterErr
				}
				continue
			}

			// Iterate over each key and report its individual status
			for _, ksPair := range r.GetKeyStatuses() {
				switch ksPair.GetStatus() {

				// Key not found in Swamp
				case hydraidepbgo.Status_NOT_FOUND:
					if iterErr := iterator(ksPair.GetKey(),
						NewError(ErrCodeNotFound, fmt.Sprintf("key (%s) not found", ksPair.GetKey()))); iterErr != nil {
						return iterErr
					}

				// Key successfully deleted
				case hydraidepbgo.Status_DELETED:
					if iterErr := iterator(ksPair.GetKey(), nil); iterErr != nil {
						return iterErr
					}
				}
			}
		}
	}

	// Deletion complete, all statuses reported (if iterator was set)
	return nil
}

type CatalogDeleteManyFromManyRequest struct {
	SwampName name.Name
	Keys      []string
}

// CatalogDeleteManyFromMany deletes keys from multiple Swamps — across multiple servers — in a single operation.
//
// This function performs distributed, batched deletion of Treasures using their Swamp name and key,
// regardless of which Hydra server holds the Swamp. The system automatically resolves which server
// handles each Swamp, groups the operations by host, and executes the deletes efficiently.
//
// ✅ Use when:
//   - You need to delete Treasures from many Swamps at once
//   - You are in a multi-server / distributed environment
//   - You want to preserve full control and observability using an iterator
//
// ⚠️ Behavior:
//   - Automatically resolves the host for each Swamp via `GetServiceClientAndHost`
//   - Groups deletion requests by server to minimize roundtrips
//   - Calls the `iterator` (if provided) with each key's result status
//   - If the last key in a Swamp is deleted, the Swamp is removed as well
//
// 💡 Internally built on Hydra’s stateless distributed architecture — no central coordinator needed.
func (h *hydraidego) CatalogDeleteManyFromMany(ctx context.Context, request []*CatalogDeleteManyFromManyRequest, iterator CatalogDeleteIteratorFunc) error {

	type requestGroup struct {
		client hydraidepbgo.HydraideServiceClient
		keys   []string
	}

	// Group delete requests by server (host)
	serverRequests := make(map[string]*requestGroup)

	for _, req := range request {

		// Determine which server hosts the given Swamp (based on its name)
		clientAndHost := h.client.GetServiceClientAndHost(req.SwampName)

		// Initialize group for this server if needed
		if _, ok := serverRequests[clientAndHost.Host]; !ok {
			serverRequests[clientAndHost.Host] = &requestGroup{
				client: clientAndHost.GrpcClient,
			}
		}

		// Add keys to this server group
		serverRequests[clientAndHost.Host].keys = req.Keys
	}

	// Process each group of Swamps per server
	for _, reqGroup := range serverRequests {

		// Build a list of Swamp+Key combinations for this batch
		swamps := make([]*hydraidepbgo.DeleteRequest_SwampKeys, 0, len(request))
		for _, req := range request {
			swampName := req.SwampName.Get()
			swamps = append(swamps, &hydraidepbgo.DeleteRequest_SwampKeys{
				IslandID:  req.SwampName.GetIslandID(h.client.GetAllIslands()),
				SwampName: swampName,
				Keys:      req.Keys,
			})
		}

		// Execute the delete request to this server
		response, err := reqGroup.client.Delete(ctx, &hydraidepbgo.DeleteRequest{
			Swamps: swamps,
		})

		// If the server is unreachable or error occurs, return immediately
		if err != nil {
			return errorHandler(err)
		}

		// Process response and call the iterator (if provided)
		if iterator != nil {
			for _, r := range response.GetResponses() {

				// Swamp does not exist
				if r.ErrorCode != nil && r.GetErrorCode() == hydraidepbgo.DeleteResponse_SwampDeleteResponse_SwampDoesNotExist {
					if iterErr := iterator("", NewError(ErrCodeSwampNotFound, errorMessageSwampNotFound)); iterErr != nil {
						return iterErr
					}
					continue
				}

				// Iterate over each key's deletion status
				for _, ksPair := range r.GetKeyStatuses() {
					switch ksPair.GetStatus() {

					// Key not found in the Swamp
					case hydraidepbgo.Status_NOT_FOUND:
						if iterErr := iterator(
							ksPair.GetKey(),
							NewError(ErrCodeNotFound, fmt.Sprintf("key (%s) not found", ksPair.GetKey())),
						); iterErr != nil {
							return iterErr
						}

					// Key successfully deleted
					case hydraidepbgo.Status_DELETED:
						if iterErr := iterator(ksPair.GetKey(), nil); iterErr != nil {
							return iterErr
						}
					}
				}
			}
		}
	}

	// All deletions processed successfully
	return nil
}

// CatalogSave stores or updates a single Treasure in a Swamp — creating the Swamp and key if needed.
//
// This function performs an intelligent write operation:
// - If the Swamp does not exist → it is automatically created
// - If the key does not exist   → it is created with the given value
// - If the key exists           → it is updated (only if needed)
//
// ✅ Use when:
//   - You want a safe "set-if-new, update-if-exists" logic
//   - You don’t care if the Treasure already exists — you just want the current value saved
//   - You need feedback about *what actually happened* (was it created, updated, unchanged?)
//
// ⚙️ Returns:
//   - `StatusNew`:        The Treasure was newly created
//   - `StatusModified`:   The Treasure existed and was modified
//   - `StatusNothingChanged`: The Treasure already existed and the new value was identical
//   - `StatusUnknown`:    Something went wrong (see error)
//
// 💡 This function is preferred for cases where you don’t want to check existence beforehand.
// It is atomic, clean, and supports real-time reactive updates.
func (h *hydraidego) CatalogSave(ctx context.Context, swampName name.Name, model any) (eventStatus EventStatus, err error) {

	// Convert the model into a KeyValuePair (binary format) using reflection + hydrun tags
	kvPair, err := convertCatalogModelToKeyValuePair(model, h.getEncodingForSwamp(swampName))
	if err != nil {
		return StatusUnknown, NewError(ErrCodeInvalidModel, err.Error())
	}

	// Perform the Set operation with full upsert behavior:
	// - CreateIfNotExist = true → will create Swamp if needed
	// - Overwrite = true        → will update key if it exists
	setResponse, err := h.client.GetServiceClient(swampName).Set(ctx, &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				IslandID:         swampName.GetIslandID(h.client.GetAllIslands()),
				SwampName:        swampName.Get(),
				KeyValues:        []*hydraidepbgo.KeyValuePair{kvPair},
				CreateIfNotExist: true,
				Overwrite:        true,
			},
		},
	})
	if err != nil {
		// Translate gRPC or Hydra-specific error into user-friendly error
		if s, ok := status.FromError(err); ok {
			switch s.Code() {
			case codes.Aborted:
				// HydrAIDE server is shutting down
				return StatusUnknown, NewError(ErrorShuttingDown, errorMessageShuttingDown)
			case codes.Unavailable:
				return StatusUnknown, NewError(ErrCodeConnectionError, errorMessageConnectionError)
			case codes.DeadlineExceeded:
				return StatusUnknown, NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
			case codes.Canceled:
				return StatusUnknown, NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient)
			case codes.Internal:
				return StatusUnknown, NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("%s: %v", errorMessageInternalError, s.Message()))
			default:
				return StatusUnknown, NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
			}
		}
		// Non-gRPC error
		return StatusUnknown, NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
	}

	// Extract the result status from the first key in the response
	// (We only sent one key, so only one result is expected)
	for _, swamp := range setResponse.GetSwamps() {
		for _, kv := range swamp.GetKeysAndStatuses() {
			// Translate the proto response status into our local EventStatus enum
			return convertProtoStatusToStatus(kv.GetStatus()), nil
		}
	}

	// Should never reach here – fallback in case something unexpected happens
	return StatusUnknown, NewError(ErrCodeUnknown, errorMessageUnknown)
}

// CatalogSaveManyIteratorFunc is a callback used by CatalogSaveMany.
//
// It is invoked for each Treasure that was processed, with:
//   - `key`: The unique identifier of the Treasure
//   - `status`: The result status (New, Modified, NothingChanged)
//
// Returning an error will immediately halt the entire operation.
type CatalogSaveManyIteratorFunc func(key string, status EventStatus) error

// CatalogSaveMany stores or updates multiple Treasures in a single Swamp in a single batch operation.
//
// This is the multi-record variant of `Save()`, optimized for batch scenarios. It accepts a slice of models,
// converts them into binary KeyValuePairs, and upserts them into the specified Swamp.
//
// ✅ Use when:
//   - You want to insert or update multiple Treasures at once
//   - You want to ensure the Swamp is created if it doesn’t exist
//   - You want per-key feedback using an iterator
//
// ⚙️ Behavior:
//   - If the Swamp does not exist → it will be created
//   - If a key does not exist     → it will be created
//   - If a key exists             → it will be updated or left untouched (if identical)
//   - `iterator` (optional) will be called for each key with its EventStatus
//
// 🔁 Possible statuses per key (via iterator):
//   - StatusNew
//   - StatusModified
//   - StatusNothingChanged
//
// 💡 Efficient for bulk imports, migrations, or synchronized state updates.
func (h *hydraidego) CatalogSaveMany(ctx context.Context, swampName name.Name, models []any, iterator CatalogSaveManyIteratorFunc) error {

	// Convert all provided models into KeyValuePair slices
	encoding := h.getEncodingForSwamp(swampName)
	kvPairs := make([]*hydraidepbgo.KeyValuePair, 0, len(models))
	for _, model := range models {
		kvPair, err := convertCatalogModelToKeyValuePair(model, encoding)
		if err != nil {
			return NewError(ErrCodeInvalidModel, err.Error())
		}
		kvPairs = append(kvPairs, kvPair)
	}

	// Send a Set request with upsert semantics:
	// - CreateIfNotExist = true → creates Swamp if needed
	// - Overwrite = true        → updates keys if they exist
	setResponse, err := h.client.GetServiceClient(swampName).Set(ctx, &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				IslandID:         swampName.GetIslandID(h.client.GetAllIslands()),
				SwampName:        swampName.Get(),
				KeyValues:        kvPairs,
				CreateIfNotExist: true,
				Overwrite:        true,
			},
		},
	})

	// Handle gRPC or internal errors with detailed messages
	if err != nil {
		if s, ok := status.FromError(err); ok {
			switch s.Code() {
			case codes.Aborted:
				// HydrAIDE server is shutting down
				return NewError(ErrorShuttingDown, errorMessageShuttingDown)
			case codes.Unavailable:
				return NewError(ErrCodeConnectionError, errorMessageConnectionError)
			case codes.DeadlineExceeded:
				return NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
			case codes.Canceled:
				return NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient)
			case codes.Internal:
				return NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("%s: %v", errorMessageInternalError, s.Message()))
			default:
				return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
			}
		}
		// Non-gRPC error
		return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
	}

	// Process response and trigger iterator if defined
	if iterator != nil {
		for _, swamp := range setResponse.GetSwamps() {
			for _, kv := range swamp.GetKeysAndStatuses() {

				// Convert proto status to user-level status and pass it to iterator
				if iterErr := iterator(kv.GetKey(), convertProtoStatusToStatus(kv.GetStatus())); iterErr != nil {
					return iterErr
				}
			}
		}
	}

	// All operations completed successfully
	return nil
}

// CatalogSaveManyToManyIteratorFunc is used to stream per-Treasure result feedback in CatalogSaveManyToMany.
//
// Parameters:
//   - `swampName`: The Swamp in which the key was saved
//   - `key`: The unique identifier of the Treasure
//   - `status`: The result of the operation (New, Modified, NothingChanged)
//
// Returning an error aborts the entire save operation immediately.
type CatalogSaveManyToManyIteratorFunc func(swampName name.Name, key string, status EventStatus) error

// CatalogSaveManyToMany performs a multi-Swamp, multi-Treasure batch upsert across distributed servers.
//
// This function accepts a list of Swamp–model pairs and efficiently distributes the write operations
// to the correct Hydra servers based on Swamp name. It acts as a bulk "save" (insert-or-update)
// for heterogeneous, distributed Swamp structures.
//
// ✅ Use when:
//   - You want to upsert into many different Swamps in a single operation
//   - You want the Swamps to be automatically created if they don’t exist
//   - You want per-Treasure feedback using an iterator
//   - You’re in a multi-server environment and need transparent routing
//
// ⚙️ Behavior:
//   - Each model is converted into a Treasure (KeyValuePair)
//   - Swamps are grouped by their deterministic host (via name hashing)
//   - Each server receives its subset of Swamps and executes a batch Set
//   - Iterator (if provided) reports back key-level status with Swamp name context
//
// 🔁 Possible `EventStatus` values per key:
//   - StatusNew
//   - StatusModified
//   - StatusNothingChanged
//
// 💡 This is one of the most powerful primitives in HydrAIDE – a true distributed, deterministic upsert.
func (h *hydraidego) CatalogSaveManyToMany(ctx context.Context, request []*CatalogManyToManyRequest, iterator CatalogSaveManyToManyIteratorFunc) error {

	type requestBySwamp struct {
		swampName name.Name
		request   *hydraidepbgo.SwampRequest
	}

	// Prepare the per-swamp KeyValuePairs
	swamps := make([]*requestBySwamp, 0, len(request))
	for _, req := range request {

		swampName := req.SwampName.Get()
		encoding := h.getEncodingForSwamp(req.SwampName)
		kvPairs := make([]*hydraidepbgo.KeyValuePair, 0, len(req.Models))

		// Convert each model into a KeyValuePair
		for _, model := range req.Models {
			kvPair, err := convertCatalogModelToKeyValuePair(model, encoding)
			if err != nil {
				return NewError(ErrCodeInvalidModel, err.Error())
			}
			kvPairs = append(kvPairs, kvPair)
		}

		// Build the SwampRequest for this Swamp
		swamps = append(swamps, &requestBySwamp{
			swampName: req.SwampName,
			request: &hydraidepbgo.SwampRequest{
				IslandID:         req.SwampName.GetIslandID(h.client.GetAllIslands()),
				SwampName:        swampName,
				KeyValues:        kvPairs,
				CreateIfNotExist: true,
				Overwrite:        true,
			},
		})
	}

	type requestGroup struct {
		client   hydraidepbgo.HydraideServiceClient
		requests []*hydraidepbgo.SwampRequest
	}

	// Group requests by target Hydra server (based on SwampName hashing)
	serverRequests := make(map[string]*requestGroup)
	for _, sw := range swamps {

		// Resolve which server should handle this Swamp
		clientAndHost := h.client.GetServiceClientAndHost(sw.swampName)

		// Initialize group for server if needed
		if _, ok := serverRequests[clientAndHost.Host]; !ok {
			serverRequests[clientAndHost.Host] = &requestGroup{
				client: clientAndHost.GrpcClient,
			}
		}

		// Add this SwampRequest to the correct server group
		serverRequests[clientAndHost.Host].requests = append(serverRequests[clientAndHost.Host].requests, sw.request)
	}

	// Process requests grouped per server
	for _, reqGroup := range serverRequests {

		// Perform the batch Set operation for this server
		setResponse, err := reqGroup.client.Set(ctx, &hydraidepbgo.SetRequest{
			Swamps: reqGroup.requests,
		})

		if err != nil {
			// Map gRPC-level errors to internal codes
			if s, ok := status.FromError(err); ok {
				switch s.Code() {
				case codes.Aborted:
					// HydrAIDE server is shutting down
					return NewError(ErrorShuttingDown, errorMessageShuttingDown)
				case codes.Unavailable:
					return NewError(ErrCodeConnectionError, errorMessageConnectionError)
				case codes.DeadlineExceeded:
					return NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
				case codes.Canceled:
					return NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient)
				case codes.Internal:
					return NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("%s: %v", errorMessageInternalError, s.Message()))
				default:
					return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
				}
			}
			// Non-gRPC error
			return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
		}

		// Stream back statuses to the iterator, if one was provided
		if iterator != nil {
			for _, swamp := range setResponse.GetSwamps() {

				// Restore the logical Swamp name from the response
				swampNameObj := name.Load(swamp.GetSwampName())

				// Iterate through each key's status and invoke the callback
				for _, kv := range swamp.GetKeysAndStatuses() {
					if iterErr := iterator(swampNameObj, kv.GetKey(), convertProtoStatusToStatus(kv.GetStatus())); iterErr != nil {
						return iterErr
					}
				}
			}
		}
	}

	// All operations completed successfully
	return nil
}

// CatalogShiftExpiredIteratorFunc is used to stream per-Treasure result feedback in CatalogShiftExpired.
//
// Parameters:
//   - `swampName`: The Swamp in which the key was saved
//   - `key`: The unique identifier of the Treasure
//   - `status`: The result of the operation (New, Modified, NothingChanged)
//
// Returning an error aborts the entire shift operation immediately.
type CatalogShiftExpiredIteratorFunc func(model any) error

// CatalogShiftExpired performs a deterministic TTL-based data shift from a single Swamp.
//
// This function identifies and extracts expired Treasures from the specified Swamp based on their `expiredAt` metadata,
// deleting them in the same operation. It acts as a zero-waste, time-sensitive queue popper, ideal for timed workflows,
// scheduling systems, or real-time cleanup logic.
//
// ✅ Use when:
//   - You want to fetch and delete expired Treasures in one atomic operation
//   - You’re implementing a time-based queue, delayed job processor, or TTL-backed store
//   - You want thread-safe, lock-safe logic that ensures exclusive access to expired items
//
// ⚙️ Behavior:
//   - Scans the Swamp for Treasures whose `expiredAt` timestamp has **already passed**
//   - Requires each Treasure to have a properly defined and set `expireAt` field:
//     `ExpireAt time.Time ` + "`hydraide:\"expireAt\"`"
//   - ⚠️ The `ExpireAt` value **must be set in UTC** — HydrAIDE internally compares using `time.Now().UTC()`
//   - Shifts (removes) up to `howMany` expired Treasures, ordered by expiry time
//   - If `howMany == 0`, all expired Treasures are returned and removed
//   - Returns each expired Treasure as a fully unmarshaled struct (via iterator callback)
//   - The operation is atomic and **thread-safe**, guaranteeing no double-processing
//
// 📦 `model` usage:
//   - This must be a **non-pointer, empty struct instance**, e.g. `ModelCatalogQueue{}`
//   - It is used internally to infer the type to which expired Treasures should be unmarshaled
//   - ❌ Passing a pointer (e.g. `&ModelCatalogQueue{}`) will break internal decoding and must be avoided
//   - ✅ Always pass the same struct type here that was used when saving the original Treasure
//
// 🛡️ Guarantees:
//   - No duplicate returns even under concurrent calls
//   - Deleted Treasures are permanently removed from the Swamp
//   - Treasures without an `expireAt` field or with a future expiry (based on UTC) are ignored
//   - Treasures that do not exist or failed unmarshaling are silently skipped
//
// 💡 Ideal for implementing:
//   - Delayed messaging queues
//   - Expiring session dispatchers
//   - Time-triggered workflow engines
//
// 💬 If the iterator function returns an error, the operation halts immediately.
//
// ❌ Will not return Treasures that:
//   - Lack an `expireAt` field
//   - Have an `expireAt` value that is in the future **(as measured by `time.Now().UTC()`)**
func (h *hydraidego) CatalogShiftExpired(ctx context.Context, swampName name.Name, howMany int32, model any, iterator CatalogShiftExpiredIteratorFunc) error {

	// send a ShiftExpiredTreasures request to the HydrAIDE service
	response, err := h.client.GetServiceClient(swampName).ShiftExpiredTreasures(ctx, &hydraidepbgo.ShiftExpiredTreasuresRequest{
		IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName: swampName.Get(),
		HowMany:   howMany,
	})

	// Handle gRPC or internal errors with detailed messages
	if err != nil {
		return errorHandler(err)
	}

	// Process response and trigger iterator if defined
	if iterator != nil {
		// Iterate through each returned Treasure and convert it into a usable model instance
		for _, treasure := range response.GetTreasures() {

			// Skip non-existent records
			if treasure.IsExist == false {
				continue
			}

			// Create a fresh instance of the model (we clone the type, not the original value)
			modelValue := reflect.New(reflect.TypeOf(model)).Interface()

			// Unmarshal the Treasure into the model using the internal conversion logic
			if convErr := convertProtoTreasureToCatalogModel(treasure, modelValue); convErr != nil {
				return NewError(ErrCodeInvalidModel, convErr.Error())
			}

			// Pass the result to the user-provided iterator function
			// If it returns an error, halt iteration and return the error
			if iterErr := iterator(modelValue); iterErr != nil {
				return iterErr
			}
		}
	}

	// All operations completed successfully
	return nil

}

// CatalogShiftBatchIteratorFunc is used to stream per-Treasure result feedback in CatalogShiftBatch.
//
// It receives each successfully shifted (cloned and deleted) Treasure as a fully unmarshaled model instance.
//
// Parameters:
//   - `model`: The unmarshaled Treasure that was just removed from the Swamp
//
// Returning an error aborts the entire shift operation immediately.
type CatalogShiftBatchIteratorFunc func(model any) error

// CatalogShiftBatch retrieves and deletes multiple Treasures from a Swamp by their keys in a single atomic operation.
//
// This function performs a **batch shift** operation, which combines read, clone, and delete operations
// for multiple Treasures identified by their keys. It's ideal for scenarios where you need to consume
// and remove items from the Swamp in one call, such as job queue processing, shopping cart checkout,
// or message queue consumption.
//
// ✅ Use when:
//   - You want to fetch and delete multiple specific Treasures by their keys in one operation
//   - You're implementing job queues, task processors, or message consumers
//   - You need to atomically remove items after retrieving them
//   - You want to avoid the N+1 problem of reading then deleting multiple items separately
//
// ⚙️ Behavior:
//   - Takes a list of keys and fetches all matching Treasures from the specified Swamp
//   - For each existing Treasure:
//   - Locks the Treasure (treasure-level lock, thread-safe)
//   - Clones the Treasure data
//   - Deletes the original Treasure from the Swamp (permanent deletion)
//   - Returns the cloned data
//   - Missing keys are silently ignored (not an error, just not included in results)
//   - Each Treasure is unmarshaled into the provided model type
//   - The iterator is called for each successfully shifted Treasure
//   - If `keys` is empty, returns immediately with no error
//
// 📦 `model` usage:
//   - This must be a **non-pointer, empty struct instance**, e.g. `Job{}`
//   - It is used internally to infer the type to which Treasures should be unmarshaled
//   - ❌ Passing a pointer (e.g. `&Job{}`) will result in an error
//   - ✅ Always pass the same struct type here that was used when saving the original Treasure
//
// 🛡️ Guarantees:
//   - Thread-safe: treasure-level locks prevent concurrent access issues
//   - Atomic per treasure: each treasure is cloned and deleted in one operation
//   - No duplicate processing: once deleted, the treasure is permanently removed
//   - Order is not guaranteed: results may come back in any order
//
// ⚠️ Important notes:
//   - This is a **destructive operation** — deleted Treasures cannot be recovered
//   - This is a **permanent deletion** (not shadow delete)
//   - All Swamp subscribers will receive deletion notifications via the event stream
//
// 💡 Ideal for implementing:
//   - Job queue workers: fetch jobs and acknowledge (delete) them
//   - Shopping cart checkout: retrieve items and remove them from cart
//   - Message queue consumers: read and acknowledge messages
//   - Batch cleanup operations: extract items for archival before deletion
//   - Task processing systems: claim and remove tasks atomically
//
// 💬 If the iterator function returns an error, the operation halts immediately.
//
// Example - Job Queue Processing:
//
//	jobKeys := []string{"job:123", "job:456", "job:789"}
//	var processedJobs []Job
//
//	err := client.CatalogShiftBatch(ctx, swampName, jobKeys, Job{}, func(model any) error {
//	    job := model.(*Job)
//	    // Process the job (it's already deleted from the queue)
//	    if err := processJob(job); err != nil {
//	        return err // Stop processing on error
//	    }
//	    processedJobs = append(processedJobs, *job)
//	    return nil
//	})
//
// Example - Shopping Cart Checkout:
//
//	itemKeys := []string{"cart:item1", "cart:item2", "cart:item3"}
//	var purchasedItems []CartItem
//
//	err := client.CatalogShiftBatch(ctx, cartSwamp, itemKeys, CartItem{}, func(model any) error {
//	    item := model.(*CartItem)
//	    purchasedItems = append(purchasedItems, *item)
//	    return nil
//	})
//	// Items are now removed from cart, ready for purchase processing
//
// 🧯 Errors:
//   - If iterator is nil → `ErrCodeInvalidArgument`
//   - If model is a pointer → `ErrCodeInvalidArgument`
//   - Invalid model conversion → `ErrCodeInvalidModel`
//   - gRPC/connection errors → mapped to consistent SDK error codes
func (h *hydraidego) CatalogShiftBatch(ctx context.Context, swampName name.Name, keys []string, model any, iterator CatalogShiftBatchIteratorFunc) error {

	// Validate required parameters
	if iterator == nil {
		return NewError(ErrCodeInvalidArgument, "iterator can not be nil")
	}

	// Ensure that the model is not a pointer type (we create new instances internally)
	if reflect.TypeOf(model).Kind() == reflect.Ptr {
		return NewError(ErrCodeInvalidArgument, "model cannot be a pointer")
	}

	// If no keys provided, return early (not an error, just nothing to do)
	if len(keys) == 0 {
		return nil
	}

	// Create the gRPC request for batch shift operation
	shiftByKeysRequest := &hydraidepbgo.ShiftByKeysRequest{
		IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName: swampName.Get(),
		Keys:      keys,
	}

	// Send ShiftByKeys request to the HydrAIDE service
	response, err := h.client.GetServiceClient(swampName).ShiftByKeys(ctx, shiftByKeysRequest)

	// Handle gRPC or internal errors with detailed messages
	if err != nil {
		return errorHandler(err)
	}

	// Iterate through each returned Treasure and convert it into a usable model instance
	for _, treasure := range response.GetTreasures() {

		// Skip non-existent records (should not happen in shift, but defensive programming)
		if treasure.IsExist == false {
			continue
		}

		// Create a fresh instance of the model (we clone the type, not the original value)
		modelValue := reflect.New(reflect.TypeOf(model)).Interface()

		// Unmarshal the Treasure into the model using the internal conversion logic
		if convErr := convertProtoTreasureToCatalogModel(treasure, modelValue); convErr != nil {
			return NewError(ErrCodeInvalidModel, convErr.Error())
		}

		// Pass the result to the user-provided iterator function
		// If it returns an error, halt iteration and return the error
		if iterErr := iterator(modelValue); iterErr != nil {
			return iterErr
		}
	}

	// All operations completed successfully
	return nil

}

// ProfileSave stores a full profile-like struct in the given Swamp as a set of key-value pairs.
//
// Unlike the Catalog-based Save methods (which use a single key per record), ProfileSave decomposes
// the given struct into individual fields — each saved as a standalone Treasure inside the same Swamp.
//
// ✅ Use when:
//   - You want to store a logically unified object (e.g. user profile, app config, product metadata)
//   - You want to load and save the full object *as one unit*
//   - You want each field to be addressable as its own key
//
// ⚙️ Behavior:
//   - Each struct field becomes its own key inside the Swamp
//   - Fields are encoded efficiently (primitive types and GOB structs supported)
//   - Fields with `hydraide:"omitempty"` tag will be skipped if they’re empty
//   - If the Swamp doesn’t exist, it will be created
//
// ⚠️ **Important: `model` must be a pointer to a struct.**
//   - This is required for proper field extraction via reflection.
//   - Passing a non-pointer value will result in an error.
//
// 💡 Best used for profiles, preferences, system snapshots, or grouped state representations.
func (h *hydraidego) ProfileSave(ctx context.Context, swampName name.Name, model any) (err error) {

	kvPairs, deletableKeys, err := convertProfileModelToKeyValuePair(model, h.getEncodingForSwamp(swampName))

	if err != nil {
		return NewError(ErrCodeInvalidModel, err.Error())
	}

	// if there is at least one deletable key, we need to delete them first
	// this is to avoid having stale keys in the swamp
	// e.g. if the model has a field that was previously set, but is now empty and marked as deletable,
	// we need to delete that key from the swamp to reflect the current state of the model
	// otherwise, the key would remain in the swamp with its old value, which is not what we want
	// this ensures that the swamp always reflects the current state of the model accurately
	if len(deletableKeys) > 0 {

		// Clean up any keys that were marked for deletion (deletable fields)
		_, err = h.client.GetServiceClient(swampName).Delete(ctx, &hydraidepbgo.DeleteRequest{
			Swamps: []*hydraidepbgo.DeleteRequest_SwampKeys{
				{
					IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
					SwampName: swampName.Get(),
					Keys:      deletableKeys,
				},
			},
		})

		if err != nil {
			if s, ok := status.FromError(err); ok {
				switch s.Code() {
				case codes.Aborted:
					// HydrAIDE server is shutting down
					return NewError(ErrorShuttingDown, errorMessageShuttingDown)
				case codes.Unavailable:
					return NewError(ErrCodeConnectionError, errorMessageConnectionError)
				case codes.DeadlineExceeded:
					return NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
				case codes.Canceled:
					return NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient)
				default:
					return nil
				}
			} else {
				return nil
			}
		}
	}

	_, err = h.client.GetServiceClient(swampName).Set(ctx, &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				IslandID:         swampName.GetIslandID(h.client.GetAllIslands()),
				SwampName:        swampName.Get(),
				KeyValues:        kvPairs,
				CreateIfNotExist: true,
				Overwrite:        true,
			},
		},
	})

	if err != nil {
		if s, ok := status.FromError(err); ok {
			switch s.Code() {
			case codes.Aborted:
				// HydrAIDE server is shutting down
				return NewError(ErrorShuttingDown, errorMessageShuttingDown)
			case codes.Unavailable:
				return NewError(ErrCodeConnectionError, errorMessageConnectionError)
			case codes.DeadlineExceeded:
				return NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
			case codes.Canceled:
				return NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient)
			case codes.Internal:
				return NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("%s: %v", errorMessageInternalError, s.Message()))
			default:
				return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
			}
		} else {
			return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
		}
	}

	return nil

}

type ProfileSaveBatchIteratorFunc func(swampName name.Name, err error) error

// ProfileSaveBatch saves multiple complete profile-like structs to multiple Swamps in a single gRPC call.
//
// This is a highly optimized version of `ProfileSave` designed to efficiently save
// dozens or even hundreds of profile models at once, rather than making individual
// requests in a loop.
//
// ✅ Use when:
//   - You need to save 50-100+ profile models (e.g., bulk user updates, batch imports)
//   - You want to minimize network round-trips and latency
//   - Each profile is stored in a separate Swamp with its fields as individual keys
//   - You want to apply deletable field cleanup across multiple profiles at once
//
// ⚙️ Behavior:
//   - Validates that swampNames and models have the same length
//   - Converts each model to KeyValuePairs using reflection and struct tags
//   - Handles deletable fields for each profile (deletes empty deletable fields first)
//   - Groups operations by target server based on Swamp name hashing
//   - Executes batch Set and Delete operations per server
//   - Calls the `iterator` function for each Swamp with success or error status
//
// 📦 Model Requirements:
//   - Each model must be a pointer to a struct
//   - Fields should use `hydraide` tags to indicate field names
//   - Supports `omitempty` and `deletable` tags same as ProfileSave
//
// 🔁 Iterator (required):
//   - Called once per profile with:
//   - `swampName`: the name of the Swamp being processed
//   - `err`: error if the save failed, nil if successful
//   - Return `nil` to continue processing, or an error to abort
//
// ✨ Example use:
//
//	type UserProfile struct {
//	    Username string `hydraide:"Username"`
//	    Email    string `hydraide:"Email"`
//	    Age      int    `hydraide:"Age"`
//	}
//
//	swampNames := []name.Name{
//	    name.New().Sanctuary("users").Realm("profiles").Swamp("alice"),
//	    name.New().Sanctuary("users").Realm("profiles").Swamp("bob"),
//	    // ... 50 more users
//	}
//
//	models := []any{&profile1, &profile2, ...} // same length as swampNames
//
//	err := client.ProfileSaveBatch(ctx, swampNames, models, func(swampName name.Name, err error) error {
//	    if err != nil {
//	        log.Printf("❌ failed to save %s: %v", swampName.Get(), err)
//	        return nil // continue with other profiles
//	    }
//	    log.Printf("✅ saved %s", swampName.Get())
//	    return nil
//	})
//
// 🚀 Performance:
//   - 50 individual ProfileSave calls = 50+ network round-trips (with deletes)
//   - 1 ProfileSaveBatch call with 50 swamps = 1-2 network round-trips
//   - Can dramatically reduce total save time from seconds to milliseconds
//
// ⚠️ **Important:**
//   - swampNames and models must have the same length
//   - swampNames[i] corresponds to models[i]
//   - iterator must not be nil
//
// 💡 Best used for bulk profile updates, batch imports, or synchronized state saves.
func (h *hydraidego) ProfileSaveBatch(ctx context.Context, swampNames []name.Name, models []any, iterator ProfileSaveBatchIteratorFunc) error {

	if iterator == nil {
		return NewError(ErrCodeInvalidArgument, "iterator must not be nil")
	}

	if len(swampNames) == 0 {
		return NewError(ErrCodeInvalidArgument, "swampNames must not be empty")
	}

	if len(swampNames) != len(models) {
		return NewError(ErrCodeInvalidArgument, fmt.Sprintf("swampNames and models must have the same length: got %d swampNames and %d models", len(swampNames), len(models)))
	}

	// Track which swamps need deletable key cleanup
	type swampDeleteRequest struct {
		swampName     name.Name
		deletableKeys []string
	}

	// Track Set requests per swamp
	type swampSetRequest struct {
		swampName name.Name
		kvPairs   []*hydraidepbgo.KeyValuePair
	}

	deleteRequests := make([]swampDeleteRequest, 0)
	setRequests := make([]swampSetRequest, 0, len(swampNames))

	// Convert all models and prepare requests
	for i, model := range models {
		swampName := swampNames[i]

		kvPairs, deletableKeys, err := convertProfileModelToKeyValuePair(model, h.getEncodingForSwamp(swampName))
		if err != nil {
			// Call iterator with error for this specific swamp
			iterErr := iterator(swampName, NewError(ErrCodeInvalidModel, err.Error()))
			if iterErr != nil {
				return iterErr
			}
			continue
		}

		// Track deletable keys for this swamp if any
		if len(deletableKeys) > 0 {
			deleteRequests = append(deleteRequests, swampDeleteRequest{
				swampName:     swampName,
				deletableKeys: deletableKeys,
			})
		}

		// Track set request for this swamp
		setRequests = append(setRequests, swampSetRequest{
			swampName: swampName,
			kvPairs:   kvPairs,
		})
	}

	// Step 1: Handle deletable keys first (if any)
	if len(deleteRequests) > 0 {
		// Group delete requests by server
		type deleteGroup struct {
			client hydraidepbgo.HydraideServiceClient
			swamps []*hydraidepbgo.DeleteRequest_SwampKeys
		}

		serverDeleteGroups := make(map[string]*deleteGroup)

		for _, delReq := range deleteRequests {
			clientAndHost := h.client.GetServiceClientAndHost(delReq.swampName)

			if _, ok := serverDeleteGroups[clientAndHost.Host]; !ok {
				serverDeleteGroups[clientAndHost.Host] = &deleteGroup{
					client: clientAndHost.GrpcClient,
					swamps: make([]*hydraidepbgo.DeleteRequest_SwampKeys, 0),
				}
			}

			serverDeleteGroups[clientAndHost.Host].swamps = append(serverDeleteGroups[clientAndHost.Host].swamps, &hydraidepbgo.DeleteRequest_SwampKeys{
				IslandID:  delReq.swampName.GetIslandID(h.client.GetAllIslands()),
				SwampName: delReq.swampName.Get(),
				Keys:      delReq.deletableKeys,
			})
		}

		// Execute delete requests per server
		for _, delGroup := range serverDeleteGroups {
			_, err := delGroup.client.Delete(ctx, &hydraidepbgo.DeleteRequest{
				Swamps: delGroup.swamps,
			})

			if err != nil {
				if s, ok := status.FromError(err); ok {
					switch s.Code() {
					case codes.Aborted:
						return NewError(ErrorShuttingDown, errorMessageShuttingDown)
					case codes.Unavailable:
						return NewError(ErrCodeConnectionError, errorMessageConnectionError)
					case codes.DeadlineExceeded:
						return NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
					case codes.Canceled:
						return NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient)
					default:
						// Silently continue on delete errors (non-critical)
						continue
					}
				}
			}
		}
	}

	// Step 2: Handle Set requests
	// Group set requests by server
	type setGroup struct {
		client   hydraidepbgo.HydraideServiceClient
		swamps   []*hydraidepbgo.SwampRequest
		swampMap map[string]name.Name // swampName string -> name.Name for iterator callback
	}

	serverSetGroups := make(map[string]*setGroup)

	for _, setReq := range setRequests {
		clientAndHost := h.client.GetServiceClientAndHost(setReq.swampName)

		if _, ok := serverSetGroups[clientAndHost.Host]; !ok {
			serverSetGroups[clientAndHost.Host] = &setGroup{
				client:   clientAndHost.GrpcClient,
				swamps:   make([]*hydraidepbgo.SwampRequest, 0),
				swampMap: make(map[string]name.Name),
			}
		}

		swampNameStr := setReq.swampName.Get()
		serverSetGroups[clientAndHost.Host].swampMap[swampNameStr] = setReq.swampName
		serverSetGroups[clientAndHost.Host].swamps = append(serverSetGroups[clientAndHost.Host].swamps, &hydraidepbgo.SwampRequest{
			IslandID:         setReq.swampName.GetIslandID(h.client.GetAllIslands()),
			SwampName:        swampNameStr,
			KeyValues:        setReq.kvPairs,
			CreateIfNotExist: true,
			Overwrite:        true,
		})
	}

	// Execute set requests per server and call iterator
	for _, setGroup := range serverSetGroups {
		_, err := setGroup.client.Set(ctx, &hydraidepbgo.SetRequest{
			Swamps: setGroup.swamps,
		})

		if err != nil {
			if s, ok := status.FromError(err); ok {
				var returnErr error
				switch s.Code() {
				case codes.Aborted:
					returnErr = NewError(ErrorShuttingDown, errorMessageShuttingDown)
				case codes.Unavailable:
					returnErr = NewError(ErrCodeConnectionError, errorMessageConnectionError)
				case codes.DeadlineExceeded:
					returnErr = NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
				case codes.Canceled:
					returnErr = NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient)
				case codes.Internal:
					returnErr = NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("%s: %v", errorMessageInternalError, s.Message()))
				default:
					returnErr = NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
				}

				// Call iterator with error for all swamps in this group
				for swampNameStr, swampName := range setGroup.swampMap {
					iterErr := iterator(swampName, returnErr)
					if iterErr != nil {
						return iterErr
					}
					_ = swampNameStr // avoid unused warning
				}
				continue
			} else {
				// Non-gRPC error - report to all swamps in group
				returnErr := NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
				for _, swampName := range setGroup.swampMap {
					iterErr := iterator(swampName, returnErr)
					if iterErr != nil {
						return iterErr
					}
				}
				continue
			}
		}

		// Success - call iterator with nil error for all swamps in this group
		for _, swampName := range setGroup.swampMap {
			iterErr := iterator(swampName, nil)
			if iterErr != nil {
				return iterErr
			}
		}
	}

	return nil
}

// ProfileRead loads a complete profile-like struct from a Swamp, field by field.
//
// This is the counterpart of `ProfileSave`, used to reconstruct a previously saved struct
// where each field was stored as a separate Treasure under the same Swamp.
//
// ✅ Use when:
//   - You previously saved a full object using ProfileSave
//   - You want to load the entire profile into a struct with one operation
//   - You expect all keys to be grouped under the same Swamp
//
// ⚙️ Behavior:
//   - Uses the struct field tags to determine the expected keys
//   - Tries to retrieve all specified keys in one `Get` call
//   - If the Swamp doesn't exist → returns ErrCodeSwampNotFound
//   - If a key is missing → silently skipped
//   - Fields are populated using reflection-based decoding
//
// ⚠️ **Important: `model` must be a pointer to a struct.**
//   - This is required for mutation and correct data binding via reflection.
//
// 💡 Best used for reading profiles, grouped settings, or full-object states.
func (h *hydraidego) ProfileRead(ctx context.Context, swampName name.Name, model any) (err error) {

	// Extract the expected keys from the model using reflection and struct tags
	keys, err := getKeyFromProfileModel(model)
	if err != nil {
		return NewError(ErrCodeInvalidModel, err.Error())
	}

	// Try to fetch all keys from the Swamp in a single operation
	response, err := h.client.GetServiceClient(swampName).Get(ctx, &hydraidepbgo.GetRequest{
		Swamps: []*hydraidepbgo.GetSwamp{
			{
				IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
				SwampName: swampName.Get(),
				Keys:      keys,
			},
		},
	})
	if err != nil {
		return errorHandler(err)
	}

	// Parse the response and assign values to the model fields
	for _, swamp := range response.GetSwamps() {
		for _, treasure := range swamp.GetTreasures() {
			// If the key does not exist, skip it silently
			if !treasure.IsExist {
				continue
			}

			// Use reflection to set the value into the model struct
			err = setTreasureValueToProfileModel(model, treasure)
			if err != nil {
				// Skip faulty assignments silently to avoid halting the whole load
				continue
			}
		}
	}

	// Successfully populated all available fields into the model
	return nil

}

type ProfileReadBatchIteratorFunc func(swampName name.Name, model any, err error) error

// ProfileReadBatch loads multiple complete profile-like structs from multiple Swamps in a single gRPC call.
//
// This is a highly optimized version of `ProfileRead` designed to efficiently load
// dozens or even hundreds of profile models at once, rather than making individual
// requests in a loop.
//
// ✅ Use when:
//   - You need to load 50-100+ profile models (e.g., user settings, entity states)
//   - You want to minimize network round-trips and latency
//   - Each profile is stored in a separate Swamp with its fields as individual keys
//
// ⚙️ Behavior:
//   - Batches all Swamp read requests into a single gRPC `Get` call
//   - For each Swamp, extracts the expected keys from the model's struct tags
//   - Fetches all keys from all Swamps in parallel on the server side
//   - Calls the `iterator` function for each Swamp with its populated model
//   - If a Swamp does not exist → iterator is called with ErrCodeSwampNotFound
//   - If a key is missing → silently skipped (same as ProfileRead)
//
// 📦 Model Requirements:
//   - `model` must be a pointer to a struct (used as a template for reflection)
//   - The same model type will be used to populate results for all Swamps
//   - Each field should be tagged to indicate which key to read from the Swamp
//
// 🔁 Iterator (required):
//   - Called once per Swamp with:
//   - `swampName`: the name of the Swamp being processed
//   - `model`: a populated copy of the model (you must type-assert it)
//   - `err`: error if the Swamp couldn't be read (e.g., ErrCodeSwampNotFound)
//   - Return `nil` to continue processing, or an error to abort
//
// ✨ Example use:
//
//	type UserSettings struct {
//	    Theme       string `hydraide:"Theme"`
//	    Language    string `hydraide:"Language"`
//	    Notifications bool `hydraide:"Notifications"`
//	}
//
//	swampNames := []name.Name{
//	    name.Swamp("user-settings", "user", "alice"),
//	    name.Swamp("user-settings", "user", "bob"),
//	    // ... 50 more users
//	}
//
//	var results []*UserSettings
//	err := client.ProfileReadBatch(ctx, swampNames, &UserSettings{}, func(swampName name.Name, model any, err error) error {
//	    if err != nil {
//	        log.Printf("❌ failed to read %s: %v", swampName.Get(), err)
//	        return nil // continue with other swamps
//	    }
//	    settings := model.(*UserSettings)
//	    results = append(results, settings)
//	    return nil
//	})
//
// 🚀 Performance:
//   - 50 individual ProfileRead calls = 50 network round-trips
//   - 1 ProfileReadBatch call with 50 swamps = 1 network round-trip
//   - Can dramatically reduce total read time from seconds to milliseconds
//
// ⚠️ **Important: `model` must be a pointer to a struct, and `iterator` must not be nil.**
//
// 💡 Best used for bulk profile loading, dashboard data fetching, or batch entity hydration.
func (h *hydraidego) ProfileReadBatch(ctx context.Context, swampNames []name.Name, model any, iterator ProfileReadBatchIteratorFunc) error {

	if iterator == nil {
		return NewError(ErrCodeInvalidArgument, "iterator must not be nil")
	}

	if len(swampNames) == 0 {
		return NewError(ErrCodeInvalidArgument, "swampNames must not be empty")
	}

	// Validate that model is a pointer to a struct
	v := reflect.ValueOf(model)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return NewError(ErrCodeInvalidModel, "model must be a pointer to a struct")
	}

	// Extract the expected keys from the model using reflection and struct tags
	keys, err := getKeyFromProfileModel(model)
	if err != nil {
		return NewError(ErrCodeInvalidModel, err.Error())
	}

	// Build the batch GetRequest with all swamps
	getSwamps := make([]*hydraidepbgo.GetSwamp, 0, len(swampNames))
	for _, swampName := range swampNames {
		getSwamps = append(getSwamps, &hydraidepbgo.GetSwamp{
			IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
			SwampName: swampName.Get(),
			Keys:      keys,
		})
	}

	// Execute the batch Get request
	response, err := h.client.GetServiceClient(swampNames[0]).Get(ctx, &hydraidepbgo.GetRequest{
		Swamps: getSwamps,
	})

	if err != nil {
		return errorHandler(err)
	}

	// Process each swamp response and call the iterator
	for i, swampResponse := range response.GetSwamps() {
		swampName := swampNames[i]

		// Check if the swamp exists
		if !swampResponse.GetIsExist() {
			// Swamp doesn't exist - call iterator with error
			iterErr := iterator(swampName, nil, NewError(ErrCodeSwampNotFound, fmt.Sprintf("%s: %s", errorMessageSwampNotFound, swampName.Get())))
			if iterErr != nil {
				return iterErr
			}
			continue
		}

		// Create a new instance of the model for this swamp
		modelType := reflect.TypeOf(model).Elem()
		newModel := reflect.New(modelType).Interface()

		// Parse the response and assign values to the model fields
		for _, treasure := range swampResponse.GetTreasures() {
			// If the key does not exist, skip it silently
			if !treasure.IsExist {
				continue
			}

			// Use reflection to set the value into the model struct
			err = setTreasureValueToProfileModel(newModel, treasure)
			if err != nil {
				// Skip faulty assignments silently to avoid halting the whole load
				continue
			}
		}

		// Call the iterator with the populated model
		iterErr := iterator(swampName, newModel, nil)
		if iterErr != nil {
			return iterErr
		}
	}

	return nil
}

// ProfileReadBatchWithFilterIteratorFunc is called for each profile that passes the filter conditions
// during a multi-profile filtered read.
type ProfileReadBatchWithFilterIteratorFunc func(swampName name.Name, model any, err error) error

// ProfileReadWithFilter reads a single profile from a Swamp and applies server-side filters.
// Returns (true, nil) if the profile matches the filter conditions and was populated into the model.
// Returns (false, nil) if the profile does not match (model is not populated).
// Returns (false, error) on communication or validation errors.
//
// The filters should use ForKey() to target specific profile fields:
//
//	filters := hydraidego.FilterAND(
//	    hydraidego.FilterInt32(hydraidego.GreaterThan, 18).ForKey("Age"),
//	    hydraidego.FilterString(hydraidego.Equal, "active").ForKey("Status"),
//	)
//	matched, err := h.ProfileReadWithFilter(ctx, swampName, filters, &user)
func (h *hydraidego) ProfileReadWithFilter(ctx context.Context, swampName name.Name, filters *FilterGroup, model any) (bool, error) {

	// Extract the expected keys from the model using reflection and struct tags
	keys, err := getKeyFromProfileModel(model)
	if err != nil {
		return false, NewError(ErrCodeInvalidModel, err.Error())
	}

	// Build the GetStream request with a single profile query
	query := &hydraidepbgo.ProfileSwampQuery{
		IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName: swampName.Get(),
		Keys:      keys,
		Filters:   convertFilterGroupToProto(filters),
	}

	stream, err := h.client.GetServiceClient(swampName).GetStream(ctx, &hydraidepbgo.GetStreamRequest{
		Queries:    []*hydraidepbgo.ProfileSwampQuery{query},
		MaxResults: 1,
	})
	if err != nil {
		return false, errorHandler(err)
	}

	// Try to receive one matching profile
	response, recvErr := stream.Recv()
	if recvErr != nil {
		if recvErr == io.EOF {
			return false, nil // no match
		}
		return false, errorHandler(recvErr)
	}

	if !response.GetIsExist() {
		return false, NewError(ErrCodeSwampNotFound, fmt.Sprintf("%s: %s", errorMessageSwampNotFound, swampName.Get()))
	}

	// Populate the model from the received treasures
	for _, treasure := range response.GetTreasures() {
		if !treasure.IsExist {
			continue
		}
		if setErr := setTreasureValueToProfileModel(model, treasure); setErr != nil {
			continue
		}
	}

	return true, nil
}

// ProfileReadBatchWithFilter reads multiple profiles from multiple Swamps with server-side filtering.
// Only profiles that pass the filter conditions are streamed back and passed to the iterator.
//
// The filters are shared across all swamp queries and should use ForKey() to target specific profile fields.
// maxResults limits the total number of matching profiles (0 = unlimited).
//
// Results are streamed per-profile. The iterator receives:
//   - swampName: which profile swamp matched
//   - model: populated profile model (type-assert to your struct pointer)
//   - err: error if the swamp doesn't exist
//
// Return nil from the iterator to continue, or an error to abort.
func (h *hydraidego) ProfileReadBatchWithFilter(ctx context.Context, swampNames []name.Name, filters *FilterGroup, model any, maxResults int32, iterator ProfileReadBatchWithFilterIteratorFunc) error {

	if iterator == nil {
		return NewError(ErrCodeInvalidArgument, "iterator must not be nil")
	}
	if len(swampNames) == 0 {
		return NewError(ErrCodeInvalidArgument, "swampNames must not be empty")
	}

	// Validate that model is a pointer to a struct
	v := reflect.ValueOf(model)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return NewError(ErrCodeInvalidModel, "model must be a pointer to a struct")
	}

	// Extract the expected keys from the model using reflection and struct tags
	keys, err := getKeyFromProfileModel(model)
	if err != nil {
		return NewError(ErrCodeInvalidModel, err.Error())
	}

	protoFilters := convertFilterGroupToProto(filters)

	// Group queries by server
	type serverGroup struct {
		client  hydraidepbgo.HydraideServiceClient
		queries []*hydraidepbgo.ProfileSwampQuery
	}

	serverGroups := make(map[string]*serverGroup)

	for _, sn := range swampNames {
		clientAndHost := h.client.GetServiceClientAndHost(sn)

		if _, ok := serverGroups[clientAndHost.Host]; !ok {
			serverGroups[clientAndHost.Host] = &serverGroup{
				client: clientAndHost.GrpcClient,
			}
		}

		serverGroups[clientAndHost.Host].queries = append(serverGroups[clientAndHost.Host].queries, &hydraidepbgo.ProfileSwampQuery{
			IslandID:  sn.GetIslandID(h.client.GetAllIslands()),
			SwampName: sn.Get(),
			Keys:      keys,
			Filters:   protoFilters,
		})
	}

	// Execute streaming requests per server
	for _, sg := range serverGroups {

		stream, err := sg.client.GetStream(ctx, &hydraidepbgo.GetStreamRequest{
			Queries:    sg.queries,
			MaxResults: maxResults,
		})
		if err != nil {
			return errorHandler(err)
		}

		for {
			response, recvErr := stream.Recv()
			if recvErr != nil {
				if recvErr == io.EOF {
					break // this server's stream is done
				}
				return errorHandler(recvErr)
			}

			sn := name.Load(response.GetSwampName())

			if !response.GetIsExist() {
				iterErr := iterator(sn, nil, NewError(ErrCodeSwampNotFound, fmt.Sprintf("%s: %s", errorMessageSwampNotFound, sn.Get())))
				if iterErr != nil {
					return iterErr
				}
				continue
			}

			// Create a new instance of the model for this profile
			modelType := reflect.TypeOf(model).Elem()
			newModel := reflect.New(modelType).Interface()

			for _, treasure := range response.GetTreasures() {
				if !treasure.IsExist {
					continue
				}
				if setErr := setTreasureValueToProfileModel(newModel, treasure); setErr != nil {
					continue
				}
			}

			iterErr := iterator(sn, newModel, nil)
			if iterErr != nil {
				return iterErr
			}
		}
	}

	return nil
}

// Count returns the number of Treasures stored in a given Swamp.
//
// This function queries the Hydra cluster and asks for the element count (Treasure count)
// for the specified Swamp. It is optimized for fast metadata retrieval without loading the actual data.
//
// ✅ Use when:
//   - You need to check how many elements are inside a Swamp
//   - You want to decide whether to load, paginate, or process based on size
//   - You want to verify existence (a non-existent Swamp will return an error)
//
// ⚙️ Behavior:
//   - If the Swamp exists, returns its element count (int32)
//   - If the Swamp does not exist → returns `ErrCodeSwampNotFound`
//   - If other errors occur (timeout, unavailable, etc.) → returns relevant wrapped error
//   - A valid Swamp will always contain at least 1 Treasure
//
// 💡 Best used for dashboards, admin tooling, paginated APIs, or cleanup logic.
func (h *hydraidego) Count(ctx context.Context, swampName name.Name) (int32, error) {

	// Request the count of treasures from the given Swamp
	response, err := h.client.GetServiceClient(swampName).Count(ctx, &hydraidepbgo.CountRequest{
		Swamps: []*hydraidepbgo.CountRequest_SwampIdentifier{
			{
				IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
				SwampName: swampName.Get(),
			},
		},
	})

	// Translate known gRPC and internal errors
	if err != nil {
		if s, ok := status.FromError(err); ok {
			switch s.Code() {
			case codes.Aborted:
				// HydrAIDE server is shutting down
				return 0, NewError(ErrorShuttingDown, errorMessageShuttingDown)
			case codes.Unavailable:
				return 0, NewError(ErrCodeConnectionError, errorMessageConnectionError)
			case codes.DeadlineExceeded:
				return 0, NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
			case codes.Canceled:
				return 0, NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient)
			case codes.Internal:
				return 0, NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("%s: %v", errorMessageInternalError, s.Message()))
			case codes.FailedPrecondition:
				return 0, NewError(ErrCodeSwampNotFound, fmt.Sprintf("%s: %v", errorMessageSwampNotFound, s.Message()))
			case codes.InvalidArgument:
				return 0, NewError(ErrCodeInvalidArgument, fmt.Sprintf("%s: %v", errorMessageInvalidArgument, s.Message()))
			default:
				return 0, NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
			}
		}
		return 0, NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
	}

	// Return the count from the response (exactly one Swamp expected)
	for _, swamp := range response.GetSwamps() {
		return swamp.GetCount(), nil
	}

	// Should not reach here – fallback error
	return 0, NewError(ErrCodeUnknown, errorMessageUnknown)
}

// Destroy permanently deletes an entire Swamp and all of its Treasures.
//
// This operation irreversibly removes all key-value pairs from the specified Swamp.
// It is the most destructive function in the HydrAIDE system and should be used with caution.
//
// ✅ Use when:
//   - You want to completely delete a logical unit of data (e.g. user profile, product snapshot)
//   - You no longer need *any* of the keys within a Swamp
//   - You are cleaning up inactive, orphaned, or deprecated Swamps
//
// ⚙️ Behavior:
//   - Deletes all Treasures under the given Swamp name
//   - Swamp will no longer be addressable or countable after this operation
//   - The operation is atomic and handled on the server side
//
// 💡 Typical usage:
//   - Deleting an entire user profile (`Profile*` Swamps)
//   - Resetting a sandbox/test environment
//   - Cleanup after full deactivation or archival
//
// ⚠️ There is no undo.
//   - Once a Swamp is destroyed, its data is permanently gone.
//   - Always confirm the swampName before using this function.
func (h *hydraidego) Destroy(ctx context.Context, swampName name.Name) error {

	// Send the destroy request to the correct server based on swampName hashing
	_, err := h.client.GetServiceClient(swampName).Destroy(ctx, &hydraidepbgo.DestroyRequest{
		IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName: swampName.Get(),
	})

	if err != nil {
		// Return internal error with context
		return NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
	}

	// Swamp successfully removed
	return nil
}

// DestroyBulk permanently deletes multiple swamps using bidirectional streaming.
//
// This method groups swamp names by their target server (based on IslandID routing)
// and opens a DestroyBulk stream to each server. Targets are sent in batches of 500.
//
// The optional progressFn callback is called periodically with the current totals
// (destroyed, failed, totalReceived) across all servers. Pass nil to skip progress reporting.
//
// ⚠️ This operation is irreversible. All data for the targeted swamps will be permanently lost.
//
// Example:
//
//	names := []name.Name{name.New("users", "profiles", "alice"), name.New("users", "profiles", "bob")}
//	err := h.DestroyBulk(ctx, names, func(destroyed, failed, total int64) {
//	    fmt.Printf("Progress: %d/%d destroyed, %d failed\n", destroyed, total, failed)
//	})
func (h *hydraidego) DestroyBulk(ctx context.Context, swampNames []name.Name, progressFn func(destroyed, failed, total int64)) error {
	if len(swampNames) == 0 {
		return nil
	}

	allIslands := h.client.GetAllIslands()

	// Group targets by their service client (server)
	type target struct {
		islandID  uint64
		swampName string
	}
	groups := make(map[hydraidepbgo.HydraideServiceClient][]target)
	for _, sn := range swampNames {
		client := h.client.GetServiceClient(sn)
		if client == nil {
			continue
		}
		groups[client] = append(groups[client], target{
			islandID:  sn.GetIslandID(allIslands),
			swampName: sn.Get(),
		})
	}

	// Process each server group
	var totalDestroyed, totalFailed, totalReceived int64
	for client, targets := range groups {
		stream, err := client.DestroyBulk(ctx)
		if err != nil {
			return NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("failed to open DestroyBulk stream: %v", err))
		}

		// Send in batches of 500
		const batchSize = 500
		for i := 0; i < len(targets); i += batchSize {
			end := i + batchSize
			if end > len(targets) {
				end = len(targets)
			}

			req := &hydraidepbgo.DestroyBulkRequest{}
			for _, t := range targets[i:end] {
				req.Targets = append(req.Targets, &hydraidepbgo.DestroyBulkTarget{
					IslandID:  t.islandID,
					SwampName: t.swampName,
				})
			}

			if err := stream.Send(req); err != nil {
				return NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("failed to send DestroyBulk batch: %v", err))
			}
		}

		if err := stream.CloseSend(); err != nil {
			return NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("failed to close DestroyBulk send: %v", err))
		}

		// Read responses until done
		for {
			resp, err := stream.Recv()
			if err != nil {
				break
			}
			totalDestroyed = totalDestroyed - totalDestroyed + resp.Destroyed // per-server counts reset
			totalFailed = totalFailed - totalFailed + resp.Failed
			totalReceived = totalReceived - totalReceived + resp.TotalReceived
			if progressFn != nil {
				progressFn(resp.Destroyed, resp.Failed, resp.TotalReceived)
			}
			if resp.Done {
				break
			}
		}
	}

	if totalFailed > 0 {
		return NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("DestroyBulk completed with %d failures out of %d targets", totalFailed, totalReceived))
	}

	return nil
}

// CompactSwamp forces a full rewrite of the swamp's .hyd file,
// removing all dead (superseded) entries and reducing fragmentation to 0%.
// This is useful after encoding migration (GOB → MessagePack) to clean up
// old entries and reclaim disk space.
func (h *hydraidego) CompactSwamp(ctx context.Context, swampName name.Name) error {

	_, err := h.client.GetServiceClient(swampName).CompactSwamp(ctx, &hydraidepbgo.CompactSwampRequest{
		IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName: swampName.Get(),
	})

	if err != nil {
		return NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
	}

	return nil
}

// convertFilterToProto converts a single SDK Filter to a proto TreasureFilter.
func convertFilterToProto(f *Filter) *hydraidepbgo.TreasureFilter {
	pf := &hydraidepbgo.TreasureFilter{
		Operator:       convertRelationalOperatorToProtoOperator(f.operator),
		BytesFieldPath: f.bytesFieldPath,
	}

	switch {
	case f.int8Val != nil:
		pf.CompareValue = &hydraidepbgo.TreasureFilter_Int8Val{Int8Val: int32(*f.int8Val)}
	case f.int16Val != nil:
		pf.CompareValue = &hydraidepbgo.TreasureFilter_Int16Val{Int16Val: int32(*f.int16Val)}
	case f.int32Val != nil:
		pf.CompareValue = &hydraidepbgo.TreasureFilter_Int32Val{Int32Val: *f.int32Val}
	case f.int64Val != nil:
		pf.CompareValue = &hydraidepbgo.TreasureFilter_Int64Val{Int64Val: *f.int64Val}
	case f.uint8Val != nil:
		pf.CompareValue = &hydraidepbgo.TreasureFilter_Uint8Val{Uint8Val: uint32(*f.uint8Val)}
	case f.uint16Val != nil:
		pf.CompareValue = &hydraidepbgo.TreasureFilter_Uint16Val{Uint16Val: uint32(*f.uint16Val)}
	case f.uint32Val != nil:
		pf.CompareValue = &hydraidepbgo.TreasureFilter_Uint32Val{Uint32Val: *f.uint32Val}
	case f.uint64Val != nil:
		pf.CompareValue = &hydraidepbgo.TreasureFilter_Uint64Val{Uint64Val: *f.uint64Val}
	case f.float32Val != nil:
		pf.CompareValue = &hydraidepbgo.TreasureFilter_Float32Val{Float32Val: *f.float32Val}
	case f.float64Val != nil:
		pf.CompareValue = &hydraidepbgo.TreasureFilter_Float64Val{Float64Val: *f.float64Val}
	case f.stringVal != nil:
		pf.CompareValue = &hydraidepbgo.TreasureFilter_StringVal{StringVal: *f.stringVal}
	case f.boolVal != nil:
		bv := hydraidepbgo.Boolean_FALSE
		if *f.boolVal {
			bv = hydraidepbgo.Boolean_TRUE
		}
		pf.CompareValue = &hydraidepbgo.TreasureFilter_BoolVal{BoolVal: bv}
	case f.timeVal != nil && f.timeField != nil:
		ts := timestamppb.New(*f.timeVal)
		switch *f.timeField {
		case TimeFieldCreatedAt:
			pf.CompareValue = &hydraidepbgo.TreasureFilter_CreatedAtVal{CreatedAtVal: ts}
		case TimeFieldUpdatedAt:
			pf.CompareValue = &hydraidepbgo.TreasureFilter_UpdatedAtVal{UpdatedAtVal: ts}
		case TimeFieldExpiredAt:
			pf.CompareValue = &hydraidepbgo.TreasureFilter_ExpiredAtVal{ExpiredAtVal: ts}
		}
	}

	if f.treasureKey != nil {
		pf.TreasureKey = f.treasureKey
	}
	if f.label != nil {
		pf.Label = f.label
	}

	return pf
}

// convertFilterGroupToProto converts an SDK FilterGroup to a proto FilterGroup.
// Returns nil if the group is nil (no filtering).
func convertFilterGroupToProto(group *FilterGroup) *hydraidepbgo.FilterGroup {
	if group == nil {
		return nil
	}

	pg := &hydraidepbgo.FilterGroup{}

	if group.logic == FilterLogicOR {
		pg.Logic = hydraidepbgo.FilterLogic_OR
	} else {
		pg.Logic = hydraidepbgo.FilterLogic_AND
	}

	for _, f := range group.filters {
		if f != nil {
			pg.Filters = append(pg.Filters, convertFilterToProto(f))
		}
	}

	for _, sg := range group.subGroups {
		if sg != nil {
			pg.SubGroups = append(pg.SubGroups, convertFilterGroupToProto(sg))
		}
	}

	for _, pf := range group.phraseFilters {
		if pf != nil {
			protoPF := &hydraidepbgo.PhraseFilter{
				BytesFieldPath: pf.bytesFieldPath,
				Words:          pf.words,
				Negate:         pf.negate,
			}
			if pf.treasureKey != nil {
				protoPF.TreasureKey = pf.treasureKey
			}
			if pf.label != nil {
				protoPF.Label = pf.label
			}
			pg.PhraseFilters = append(pg.PhraseFilters, protoPF)
		}
	}

	for _, vf := range group.vectorFilters {
		if vf != nil {
			protoVF := &hydraidepbgo.VectorFilter{
				BytesFieldPath: vf.bytesFieldPath,
				QueryVector:    vf.queryVector,
				MinSimilarity:  vf.minSimilarity,
			}
			if vf.treasureKey != nil {
				protoVF.TreasureKey = vf.treasureKey
			}
			if vf.label != nil {
				protoVF.Label = vf.label
			}
			pg.VectorFilters = append(pg.VectorFilters, protoVF)
		}
	}

	for _, gf := range group.geoDistanceFilters {
		if gf != nil {
			protoGF := &hydraidepbgo.GeoDistanceFilter{
				LatFieldPath: gf.latFieldPath,
				LngFieldPath: gf.lngFieldPath,
				RefLatitude:  gf.refLatitude,
				RefLongitude: gf.refLongitude,
				RadiusKm:     gf.radiusKm,
			}
			if gf.mode == GeoOutside {
				protoGF.Mode = hydraidepbgo.GeoDistanceMode_OUTSIDE
			}
			if gf.treasureKey != nil {
				protoGF.TreasureKey = gf.treasureKey
			}
			if gf.label != nil {
				protoGF.Label = gf.label
			}
			pg.GeoDistanceFilters = append(pg.GeoDistanceFilters, protoGF)
		}
	}

	return pg
}

// CatalogReadManyStream is the streaming variant of CatalogReadMany.
// It reads Treasures from a single swamp using server-streaming, with optional server-side filtering.
// Each matching Treasure is streamed individually instead of being collected into one response.
// If filters is nil, all Treasures matching the index criteria are returned.
//
// The filters parameter accepts a *FilterGroup built with FilterAND() or FilterOR() constructors.
// FilterGroups support nested AND/OR logic for complex boolean expressions.
func (h *hydraidego) CatalogReadManyStream(ctx context.Context, swampName name.Name, index *Index, filters *FilterGroup, model any, iterator CatalogReadManyIteratorFunc) error {

	if index == nil {
		return NewError(ErrCodeInvalidArgument, "index can not be nil")
	}
	if iterator == nil {
		return NewError(ErrCodeInvalidArgument, "iterator can not be nil")
	}
	if reflect.TypeOf(model).Kind() == reflect.Ptr {
		return NewError(ErrCodeInvalidArgument, "model cannot be a pointer")
	}

	request := &hydraidepbgo.GetByIndexStreamRequest{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		IndexType:   convertIndexTypeToProtoIndexType(index.IndexType),
		OrderType:   convertOrderTypeToProtoOrderType(index.IndexOrder),
		From:        index.From,
		Limit:       index.Limit,
		Filters:     convertFilterGroupToProto(filters),
		MaxResults:  index.MaxResults,
		ExcludeKeys:  index.ExcludeKeys,
		IncludedKeys: index.IncludedKeys,
		KeysOnly:     index.KeysOnly,
	}

	request.FromTime = toOptionalTimestamppb(index.FromTime)
	request.ToTime = toOptionalTimestamppb(index.ToTime)

	stream, err := h.client.GetServiceClient(swampName).GetByIndexStream(ctx, request)
	if err != nil {
		return errorHandler(err)
	}

	for {
		response, recvErr := stream.Recv()
		if recvErr != nil {
			if recvErr == io.EOF {
				return nil // stream completed normally
			}
			return errorHandler(recvErr)
		}

		treasure := response.GetTreasure()
		if treasure == nil || !treasure.IsExist {
			continue
		}

		modelValue := reflect.New(reflect.TypeOf(model)).Interface()
		if convErr := convertProtoTreasureToCatalogModel(treasure, modelValue); convErr != nil {
			return NewError(ErrCodeInvalidModel, convErr.Error())
		}

		setSearchMetaOnModel(modelValue, response.GetMeta())

		if iterErr := iterator(modelValue); iterErr != nil {
			return iterErr
		}
	}
}

// CatalogReadManyFromMany reads from multiple swamps using server-streaming with per-swamp filtering.
// Results arrive swamp-by-swamp in the order of the request slice.
// The iterator receives the source swamp name along with each matching Treasure.
func (h *hydraidego) CatalogReadManyFromMany(ctx context.Context, request []*CatalogReadManyFromManyRequest, model any, iterator CatalogReadManyFromManyIteratorFunc) error {

	if len(request) == 0 {
		return NewError(ErrCodeInvalidArgument, "request can not be empty")
	}
	if iterator == nil {
		return NewError(ErrCodeInvalidArgument, "iterator can not be nil")
	}
	if reflect.TypeOf(model).Kind() == reflect.Ptr {
		return NewError(ErrCodeInvalidArgument, "model cannot be a pointer")
	}

	// Group queries by server
	type serverGroup struct {
		client  hydraidepbgo.HydraideServiceClient
		queries []*hydraidepbgo.SwampQuery
	}

	serverGroups := make(map[string]*serverGroup)

	for _, req := range request {
		if req.Index == nil {
			return NewError(ErrCodeInvalidArgument, "index can not be nil in request")
		}

		clientAndHost := h.client.GetServiceClientAndHost(req.SwampName)

		if _, ok := serverGroups[clientAndHost.Host]; !ok {
			serverGroups[clientAndHost.Host] = &serverGroup{
				client: clientAndHost.GrpcClient,
			}
		}

		query := &hydraidepbgo.SwampQuery{
			IslandID:    req.SwampName.GetIslandID(h.client.GetAllIslands()),
			SwampName:   req.SwampName.Get(),
			IndexType:   convertIndexTypeToProtoIndexType(req.Index.IndexType),
			OrderType:   convertOrderTypeToProtoOrderType(req.Index.IndexOrder),
			From:        req.Index.From,
			Limit:       req.Index.Limit,
			Filters:     convertFilterGroupToProto(req.Filters),
			MaxResults:  req.Index.MaxResults,
			ExcludeKeys:  req.Index.ExcludeKeys,
			IncludedKeys: req.Index.IncludedKeys,
			KeysOnly:     req.Index.KeysOnly,
		}

		query.FromTime = toOptionalTimestamppb(req.Index.FromTime)
		query.ToTime = toOptionalTimestamppb(req.Index.ToTime)

		serverGroups[clientAndHost.Host].queries = append(serverGroups[clientAndHost.Host].queries, query)
	}

	// Execute streaming requests per server
	for _, sg := range serverGroups {

		stream, err := sg.client.GetByIndexStreamFromMany(ctx, &hydraidepbgo.GetByIndexStreamFromManyRequest{
			Queries: sg.queries,
		})
		if err != nil {
			return errorHandler(err)
		}

		for {
			response, recvErr := stream.Recv()
			if recvErr != nil {
				if recvErr == io.EOF {
					break // this server's stream is done, move to next
				}
				return errorHandler(recvErr)
			}

			treasure := response.GetTreasure()
			if treasure == nil || !treasure.IsExist {
				continue
			}

			swampName := name.Load(response.GetSwampName())

			modelValue := reflect.New(reflect.TypeOf(model)).Interface()
			if convErr := convertProtoTreasureToCatalogModel(treasure, modelValue); convErr != nil {
				return NewError(ErrCodeInvalidModel, convErr.Error())
			}

			setSearchMetaOnModel(modelValue, response.GetMeta())

			if iterErr := iterator(swampName, modelValue); iterErr != nil {
				return iterErr
			}
		}
	}

	return nil
}

type SubscribeIteratorFunc func(model any, eventStatus EventStatus, err error) error

// Subscribe sets up a real-time event stream for a given Swamp, allowing you to react to changes as they happen.
//
// This is one of the most powerful primitives in HydrAIDE – it enables reactive, event-driven systems
// without the need for external brokers (e.g. Kafka, NATS).
//
// ✅ Use when:
//   - You want to track changes in a Swamp live (insert, update, delete)
//   - You want to unify existing data and future updates in a single stream
//   - You are building reactive systems (notifications, brokers, socket push, AI pipeline progress)
//
// ⚙️ Behavior:
//   - Subscribes to Swamp-level changes via gRPC stream
//   - The `iterator` callback receives one message per change (with status)
//   - `model` must be a **non-pointer type**, used as a blueprint
//   - Each call to `iterator(modelInstance, status, err)` passes a freshly filled pointer to modelInstance
//   - If `getExistingData` is true:
//   - All current Treasures are loaded and passed first (in ascending creation time)
//   - Then the live stream begins from that point
//
// ⚠️ Notes:
//   - The subscription is **non-blocking**; the stream runs in a background goroutine
//   - The stream will stop if:
//   - the context is canceled
//   - the iterator returns an error
//   - the server closes the stream
//   - If an event conversion fails, the error is passed to the iterator (non-fatal)
//
// 💡 Typical use cases:
//   - Watching a Swamp for AI completion signals
//   - Acting as a message queue for microservices
//   - Forwarding real-time updates to WebSocket clients
//   - Triggering logic in distributed workflows
func (h *hydraidego) Subscribe(ctx context.Context, swampName name.Name, getExistingData bool, model any, iterator SubscribeIteratorFunc) error {

	// check if the iterator is nil
	if iterator == nil {
		// iterator can not be nil
		return NewError(ErrCodeInvalidArgument, "iterator can not be nil")
	}

	// get the existing data if needed
	if getExistingData {

		// get all data by the index creation time in ascending order
		response, err := h.client.GetServiceClient(swampName).GetByIndex(ctx, &hydraidepbgo.GetByIndexRequest{
			IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
			SwampName: swampName.Get(),
			IndexType: hydraidepbgo.IndexType_CREATION_TIME,
			OrderType: hydraidepbgo.OrderType_ASC,
			From:      0,
			Limit:     0,
		})

		if err != nil {
			// only server-side errors are handled here
			if s, ok := status.FromError(err); ok {
				switch s.Code() {
				case codes.Unavailable:
					return NewError(ErrCodeConnectionError, errorMessageConnectionError)
				case codes.DeadlineExceeded:
					return NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
				case codes.Internal:
					return NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("%s: %v", errorMessageInternalError, s.Message()))
				}
			}
		}

		// go through the treasures and load them to the model if the user wants to get the existing data
		for _, treasure := range response.GetTreasures() {

			if treasure.IsExist == false {
				continue
			}

			// create a new instance of the model
			modelInstance := reflect.New(reflect.TypeOf(model)).Interface()

			// ConvertProtoTreasureToModel function will load the data to the model
			if convErr := convertProtoTreasureToCatalogModel(treasure, modelInstance); convErr != nil {
				return NewError(ErrCodeInvalidModel, convErr.Error())
			}

			// call the iterator function and handle its error
			// exit the loop if the iterator returns an error
			if iErr := iterator(modelInstance, StatusNothingChanged, nil); iErr != nil {
				return iErr
			}

		}

	}

	// subscribe to the events
	eventClient, err := h.client.GetServiceClient(swampName).SubscribeToEvents(ctx, &hydraidepbgo.SubscribeToEventsRequest{
		IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName: swampName.Get(),
	})

	if err != nil {
		if s, ok := status.FromError(err); ok {
			switch s.Code() {
			case codes.Aborted:
				// HydrAIDE server is shutting down
				return NewError(ErrorShuttingDown, errorMessageShuttingDown)
			case codes.Unavailable:
				return NewError(ErrCodeConnectionError, errorMessageConnectionError)
			case codes.DeadlineExceeded:
				return NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
			case codes.InvalidArgument:
				return NewError(ErrCodeInvalidArgument, errorMessageInvalidArgument)
			case codes.Internal:
				return NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("%s: %v", errorMessageInternalError, s.Message()))
			default:
				return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
			}
		} else {
			return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
		}
	}

	// listen to the events and block until the context is closed, the event stream is closed or error occurs in the
	// stream or the iterator
	go func() {
		for {
			select {
			case <-ctx.Done():
				// context closed by client
				return
			default:

				event, receiveErr := eventClient.Recv()
				// if the connection is closed, then we can exit the loop and do not listen to the events anymore
				if receiveErr != nil {
					if receiveErr == io.EOF {
						// connection gracefully closed by the server
						return
					}
					// call iterator function with error
					if iErr := iterator(nil, StatusUnknown, NewError(ErrCodeUnknown, receiveErr.Error())); iErr != nil {
						return
					}
					// unexpected error while receiving the event
					return
				}

				// create a new instance of the model
				modelInstance := reflect.New(reflect.TypeOf(model)).Interface()
				var convErr error

				// switch the event status and load the data to the model
				// the conversion error will be stored in the convErr variable and pass it to the iterator
				switch event.Status {
				case hydraidepbgo.Status_NEW, hydraidepbgo.Status_UPDATED, hydraidepbgo.Status_NOTHING_CHANGED:
					convErr = convertProtoTreasureToCatalogModel(event.GetTreasure(), modelInstance)
				case hydraidepbgo.Status_DELETED:
					convErr = convertProtoTreasureToCatalogModel(event.GetDeletedTreasure(), modelInstance)
				}

				// call the iterator function and handle its error
				// exit the loop if the iterator returns an error
				if iErr := iterator(modelInstance, convertProtoStatusToStatus(event.Status), convErr); iErr != nil {
					// iteration error
					return
				}

				continue

			}
		}
	}()

	return nil

}

type Int8Condition struct {
	RelationalOperator RelationalOperator
	Value              int8
}

type Int16Condition struct {
	RelationalOperator RelationalOperator
	Value              int16
}

type Int32Condition struct {
	RelationalOperator RelationalOperator
	Value              int32
}
type Int64Condition struct {
	RelationalOperator RelationalOperator
	Value              int64
}

type Uint8Condition struct {
	RelationalOperator RelationalOperator
	Value              uint8
}

type Uint16Condition struct {
	RelationalOperator RelationalOperator
	Value              uint16
}

type Uint32Condition struct {
	RelationalOperator RelationalOperator
	Value              uint32
}
type Uint64Condition struct {
	RelationalOperator RelationalOperator
	Value              uint64
}

type Float32Condition struct {
	RelationalOperator RelationalOperator
	Value              float32
}

type Float64Condition struct {
	RelationalOperator RelationalOperator
	Value              float64
}

// RelationalOperator defines the set of supported relational comparison
// operators for conditional increment operations in HydrAIDE.
//
// These operators are evaluated **atomically on the server** against the
// current stored value of a Treasure before applying an increment.
//
// ✅ Where it's used:
// Pass a RelationalOperator inside a type-specific condition struct
// (e.g., `Int8Condition`, `Int32Condition`) when calling an Increment*
// method to control whether the increment should occur.
//
// ✅ How it works:
//   - The server retrieves the current value for the given Treasure.
//   - It compares that value to the `Value` provided in your condition.
//   - If the comparison passes, the increment is applied.
//   - If the comparison fails, the increment is skipped, but the current
//     value and metadata are still returned along with `ErrConditionNotMet`.
//
// ⚠️ Notes:
//   - All comparisons are performed in the numeric domain of the increment
//     type (e.g., int8, int32).
//   - This is a strict **single-value** comparison — no complex expressions
//     or logical chaining.
//   - Equality and inequality are type-precise: int8(10) is not equal to
//     int8(11), but will be equal to int8(10).
type RelationalOperator int

const (
	// NotEqual means "current value != condition value"
	NotEqual RelationalOperator = iota

	// Equal means "current value == condition value"
	Equal

	// GreaterThanOrEqual means "current value >= condition value"
	GreaterThanOrEqual

	// GreaterThan means "current value > condition value"
	GreaterThan

	// LessThanOrEqual means "current value <= condition value"
	LessThanOrEqual

	// LessThan means "current value < condition value"
	LessThan

	// Contains means "string contains substring" (case-sensitive)
	Contains

	// NotContains means "string does NOT contain substring" (case-sensitive)
	NotContains

	// StartsWith means "string starts with prefix" (case-sensitive)
	StartsWith

	// EndsWith means "string ends with suffix" (case-sensitive)
	EndsWith

	// IsEmpty means "field is nil/unset or empty string" (CompareValue is ignored)
	IsEmpty

	// IsNotEmpty means "field exists and is non-empty" (CompareValue is ignored)
	IsNotEmpty

	// HasKey means "BytesVal map contains the specified key" (uses StringVal as key name, requires BytesFieldPath)
	HasKey

	// HasNotKey means "BytesVal map does NOT contain the specified key" (uses StringVal as key name, requires BytesFieldPath)
	HasNotKey

	// SliceContains means "BytesVal slice contains the exact value" (requires BytesFieldPath)
	SliceContains

	// SliceNotContains means "BytesVal slice does NOT contain the exact value" (requires BytesFieldPath)
	SliceNotContains

	// SliceContainsSubstring means "any string element in BytesVal slice contains substring" (case-insensitive, requires BytesFieldPath)
	SliceContainsSubstring

	// SliceNotContainsSubstring means "no string element in BytesVal slice contains substring" (case-insensitive, requires BytesFieldPath)
	SliceNotContainsSubstring
)

// IncrementMetaRequest defines optional metadata to be set when performing
// an Increment operation in a HydrAIDE Catalog-type Swamp.
//
// This struct lets you specify creation, update, and expiration metadata
// that HydrAIDE will automatically attach to the Treasure as part of the
// same atomic increment call.
//
// 🧠 Why this matters:
// Normally, if you want to set lifecycle metadata (`createdAt`, `updatedBy`, etc.),
// you would have to check if the Treasure exists, then either create or update it
// in separate calls. With IncrementMetaRequest, you can tell HydrAIDE what metadata
// to set in either case — without doing any client-side existence checks.
//
// ✅ Usage:
// Pass a pointer to IncrementMetaRequest into the `setIfNotExist` or `setIfExist`
// parameters of an `Increment*` method:
//
//   - `setIfNotExist`: Applied only if the Treasure does not yet exist
//   - `setIfExist`: Applied only if the Treasure already exists
//
// HydrAIDE will atomically:
//  1. Increment the numeric value (if condition passes)
//  2. Apply the appropriate metadata based on existence
//
// ⚠️ Notes:
// - All fields are optional; unset fields are ignored.
// - ExpiredAt can be used to attach a TTL at increment time.
// - This metadata mechanism is supported only in Catalog-type Swamps.
type IncrementMetaRequest struct {
	SetCreatedAt bool      // If true, sets CreatedAt to the current server time
	SetCreatedBy string    // If non-empty, sets CreatedBy to this identifier
	SetUpdatedAt bool      // If true, sets UpdatedAt to the current server time
	SetUpdatedBy string    // If non-empty, sets UpdatedBy to this identifier
	ExpiredAt    time.Time // If non-zero, sets ExpiredAt to this exact time
}

// IncrementMetaResponse contains the metadata returned by HydrAIDE
// after an Increment operation.
//
// Even if the increment condition fails, HydrAIDE still returns the
// latest value and the current metadata, so you can inspect state without
// making a separate read call.
//
// ✅ When returned:
//   - On any successful Increment* call (condition met or not)
//   - On failed conditional increments, value/meta are still populated,
//     but an ErrConditionNotMet is also returned
//
// 📌 Typical uses:
// - Auditing (knowing who created/updated a Treasure and when)
// - Inspecting expiration state without a read query
// - Confirming that metadata changes were applied as intended
type IncrementMetaResponse struct {
	CreatedAt time.Time // Timestamp when the Treasure was created
	CreatedBy string    // Identifier of the creator
	UpdatedAt time.Time // Timestamp when the Treasure was last updated
	UpdatedBy string    // Identifier of the last updater
	ExpiredAt time.Time // Expiration time of the Treasure (zero if none)
}

func convertMetaToProtoMeta(meta *IncrementMetaRequest) *hydraidepbgo.IncrementRequestMetadata {

	if meta == nil {
		return nil
	}

	protoMeta := &hydraidepbgo.IncrementRequestMetadata{}
	if meta.SetCreatedAt {
		protoMeta.CreatedAt = &meta.SetCreatedAt
	}
	if meta.SetCreatedBy != "" {
		protoMeta.CreatedBy = &meta.SetCreatedBy
	}
	if meta.SetUpdatedAt {
		protoMeta.UpdatedAt = &meta.SetUpdatedAt
	}
	if meta.SetUpdatedBy != "" {
		protoMeta.UpdatedBy = &meta.SetUpdatedBy
	}
	if !meta.ExpiredAt.IsZero() {
		protoMeta.ExpiredAt = timestamppb.New(meta.ExpiredAt)
	}
	return protoMeta

}

func convertIncrementMetaResponse(response *hydraidepbgo.IncrementResponseMetadata) *IncrementMetaResponse {

	if response == nil {
		return nil
	}

	meta := &IncrementMetaResponse{}
	if response.CreatedAt != nil {
		meta.CreatedAt = response.GetCreatedAt().AsTime()
	}
	if response.CreatedBy != nil {
		meta.CreatedBy = response.GetCreatedBy()
	}
	if response.UpdatedAt != nil {
		meta.UpdatedAt = response.GetUpdatedAt().AsTime()
	}
	if response.UpdatedBy != nil {
		meta.UpdatedBy = response.GetUpdatedBy()
	}
	if response.ExpiredAt != nil {
		meta.ExpiredAt = response.GetExpiredAt().AsTime()
	}

	return meta

}

// IncrementInt8 performs an atomic int8 increment on a Treasure inside the given Swamp,
// with optional, server-side metadata control for both the "create" and "update" paths.
//
// Core behavior
// -------------
//   - If the Swamp or Treasure does not exist, HydrAIDE will **create** them.
//   - If a `condition` is provided, the increment runs **atomically on the server** only if
//     the current value satisfies the given relational operator (>, >=, ==, etc.).
//   - The function **always returns** the value and metadata that are currently stored for
//     the Treasure. If the condition is not met, you'll get:
//   - the current value (not incremented),
//   - the resolved metadata (see below),
//   - and `ErrConditionNotMet` as the error.
//     This makes it easy to branch your logic without an extra read.
//
// Metadata control (optional)
// ---------------------------
// You can provide two optional metadata descriptors to control lifecycle/audit fields
// **in the same atomic call**, without pre-checking existence on the client:
//
//   - setIfNotExist (*IncrementMetaRequest):
//     Applied if the Treasure must be **created** as part of this call.
//     Typical use: set CreatedAt/CreatedBy and an initial ExpiredAt.
//
//   - setIfExist (*IncrementMetaRequest):
//     Applied if the Treasure **already exists** (i.e., an update path).
//     Typical use: set UpdatedAt/UpdatedBy and optionally refresh ExpiredAt.
//
// Fields in IncrementMetaRequest:
//   - SetCreatedAt (bool) → when true, server sets CreatedAt=now.UTC()
//   - SetCreatedBy (string) → when non-empty, server sets CreatedBy to this value
//   - SetUpdatedAt (bool) → when true, server sets UpdatedAt=now.UTC()
//   - SetUpdatedBy (string) → when non-empty, server sets UpdatedBy to this value
//   - ExpiredAt (time.Time) → when non-zero, server sets ExpiredAt to this instant
//
// The server chooses which of the two (create/update) descriptors to apply based on whether
// the Treasure existed **before** the increment attempt. You do **not** need to check
// existence on the client side.
//
// Parameters
// ----------
//   - ctx:           context for cancellation and timeout
//   - swampName:     target Swamp where the Treasure lives
//   - key:           unique key of the Treasure to increment
//   - value:         the delta (positive or negative) to add
//   - condition:     optional constraint on the current value before incrementing
//   - setIfNotExist: optional metadata settings if the Treasure must be created
//   - setIfExist:    optional metadata settings if the Treasure already exists
//
// Returns
// -------
//   - int8:                   the value stored after the call (incremented if condition succeeded)
//   - *IncrementMetaResponse: the resolved metadata (CreatedAt/By, UpdatedAt/By, ExpiredAt)
//   - error:                  nil on success; `ErrConditionNotMet` if the condition failed;
//     or a transport/validation error
//
// Notes
// -----
//   - Decrementing is supported by passing a negative delta.
//   - If the Treasure exists but has a **different value type** (e.g. float64), the call fails.
//   - Proto values travel as int32 on the wire and are converted back to int8 here.
//   - Conditional operators (Equal, NotEqual, GreaterThan, GreaterThanOrEqual,
//     LessThan, LessThanOrEqual) compare against the **current** stored value.
//
// Examples
// --------
// 1) Increment with automatic creation + creation metadata
//
//	newVal, meta, err := h.IncrementInt8(
//	  ctx,
//	  name.New().Sanctuary("scores").Realm("users").Swamp("global"),
//	  "user:42",
//	  1,
//	  nil, // no condition
//	  &IncrementMetaRequest{ // setIfNotExist
//	    SetCreatedAt: true,
//	    SetCreatedBy: "game-service",
//	    ExpiredAt:    time.Now().Add(24 * time.Hour),
//	  },
//	  &IncrementMetaRequest{ // setIfExist
//	    SetUpdatedAt: true,
//	    SetUpdatedBy: "game-service",
//	  },
//	)
//
//	// If "user:42" did not exist, it's created, incremented, and Created* + ExpiredAt are set.
//	// If it existed, it's incremented and Updated* are set.
//	// In both cases, `meta` contains the resolved metadata.
//
// 2) Conditional increment that may not run
//
//	newVal, meta, err := h.IncrementInt8(
//	  ctx, swamp, "counter", 5,
//	  &Int8Condition{RelationalOperator: GreaterThan, Value: 10},
//	  nil, nil,
//	)
//
//	// If current value <= 10 → err == ErrConditionNotMet,
//	// but you still get `newVal` (the unchanged current value) and `meta`.
//
// Implementation detail
// ---------------------
// Internally this calls the HydrAIDE gRPC Increment API for int8 and maps the optional
// `setIfNotExist` / `setIfExist` descriptors to server-side metadata mutations. The server
// applies exactly one of them (create vs. update) based on pre-increment existence, then
// evaluates the condition atomically and returns both the resulting value and metadata.
func (h *hydraidego) IncrementInt8(ctx context.Context, swampName name.Name, key string, value int8, condition *Int8Condition, setIfNotExist *IncrementMetaRequest, setIfExist *IncrementMetaRequest) (int8, *IncrementMetaResponse, error) {

	r := &hydraidepbgo.IncrementInt8Request{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		Key:         key,
		IncrementBy: int32(value),
	}

	if setIfNotExist != nil {
		r.SetIfNotExist = convertMetaToProtoMeta(setIfNotExist)
	}
	if setIfExist != nil {
		r.SetIfExist = convertMetaToProtoMeta(setIfExist)
	}

	if condition != nil {
		r.Condition = &hydraidepbgo.IncrementInt8Condition{
			RelationalOperator: convertRelationalOperatorToProtoOperator(condition.RelationalOperator),
			// convert to int32 because the proto message is int32, but the HydrAIDE will convert it back to int8
			Value: int32(condition.Value),
		}
	}

	response, err := h.client.GetServiceClient(swampName).IncrementInt8(ctx, r)
	if err != nil {
		return 0, nil, errorHandler(err)
	}

	// return with the new value if the increment was successful
	if response.GetIsIncremented() {
		return int8(response.GetValue()), convertIncrementMetaResponse(response.GetMetadata()), nil
	}

	return int8(response.GetValue()), convertIncrementMetaResponse(response.GetMetadata()), NewError(ErrConditionNotMet, fmt.Sprintf("%s: %d", errorMessageConditionNotMet, response.GetValue()))

}

// IncrementInt16 performs an atomic int16 increment on a Treasure inside the given Swamp,
// with optional, server-side metadata control for both the "create" and "update" paths.
//
// Core behavior
// -------------
//   - If the Swamp or Treasure does not exist, HydrAIDE will **create** them.
//   - If a `condition` is provided, the increment runs **atomically on the server** only if
//     the current value satisfies the given relational operator (>, >=, ==, etc.).
//   - The function **always returns** the value and metadata that are currently stored for
//     the Treasure. If the condition is not met, you'll get:
//   - the current value (not incremented),
//   - the resolved metadata (see below),
//   - and `ErrConditionNotMet` as the error.
//     This makes it easy to branch your logic without an extra read.
//
// Metadata control (optional)
// ---------------------------
// You can provide two optional metadata descriptors to control lifecycle/audit fields
// **in the same atomic call**, without pre-checking existence on the client:
//
//   - setIfNotExist (*IncrementMetaRequest):
//     Applied if the Treasure must be **created** as part of this call.
//     Typical use: set CreatedAt/CreatedBy and an initial ExpiredAt.
//
//   - setIfExist (*IncrementMetaRequest):
//     Applied if the Treasure **already exists** (i.e., an update path).
//     Typical use: set UpdatedAt/UpdatedBy and optionally refresh ExpiredAt.
//
// Fields in IncrementMetaRequest:
//   - SetCreatedAt (bool) → when true, server sets CreatedAt=now.UTC()
//   - SetCreatedBy (string) → when non-empty, server sets CreatedBy to this value
//   - SetUpdatedAt (bool) → when true, server sets UpdatedAt=now.UTC()
//   - SetUpdatedBy (string) → when non-empty, server sets UpdatedBy to this value
//   - ExpiredAt (time.Time) → when non-zero, server sets ExpiredAt to this instant
//
// The server chooses which of the two (create/update) descriptors to apply based on whether
// the Treasure existed **before** the increment attempt. You do **not** need to check
// existence on the client side.
//
// Parameters
// ----------
//   - ctx:           context for cancellation and timeout
//   - swampName:     target Swamp where the Treasure lives
//   - key:           unique key of the Treasure to increment
//   - value:         the delta (positive or negative) to add
//   - condition:     optional constraint on the current value before incrementing
//   - setIfNotExist: optional metadata settings if the Treasure must be created
//   - setIfExist:    optional metadata settings if the Treasure already exists
//
// Returns
// -------
//   - int16:                  the value stored after the call (incremented if condition succeeded)
//   - *IncrementMetaResponse: the resolved metadata (CreatedAt/By, UpdatedAt/By, ExpiredAt)
//   - error:                  nil on success; `ErrConditionNotMet` if the condition failed;
//     or a transport/validation error
//
// Notes
// -----
//   - Decrementing is supported by passing a negative delta.
//   - If the Treasure exists but has a **different value type** (e.g. float64), the call fails.
//   - Proto values travel as int32 on the wire and are converted back to int16 here.
//   - Conditional operators (Equal, NotEqual, GreaterThan, GreaterThanOrEqual,
//     LessThan, LessThanOrEqual) compare against the **current** stored value.
//
// Examples
// --------
// 1) Increment with automatic creation + creation metadata
//
//	newVal, meta, err := h.IncrementInt16(
//	  ctx,
//	  name.New().Sanctuary("scores").Realm("users").Swamp("global"),
//	  "user:42",
//	  1,
//	  nil, // no condition
//	  &IncrementMetaRequest{ // setIfNotExist
//	    SetCreatedAt: true,
//	    SetCreatedBy: "game-service",
//	    ExpiredAt:    time.Now().Add(24 * time.Hour),
//	  },
//	  &IncrementMetaRequest{ // setIfExist
//	    SetUpdatedAt: true,
//	    SetUpdatedBy: "game-service",
//	  },
//	)
//
//	// If "user:42" did not exist, it's created, incremented, and Created* + ExpiredAt are set.
//	// If it existed, it's incremented and Updated* are set.
//	// In both cases, `meta` contains the resolved metadata.
//
// 2) Conditional increment that may not run
//
//	newVal, meta, err := h.IncrementInt16(
//	  ctx, swamp, "counter", 5,
//	  &Int16Condition{RelationalOperator: GreaterThan, Value: 10},
//	  nil, nil,
//	)
//
//	// If current value <= 10 → err == ErrConditionNotMet,
//	// but you still get `newVal` (the unchanged current value) and `meta`.
//
// Implementation detail
// ---------------------
// Internally this calls the HydrAIDE gRPC Increment API for int16 and maps the optional
// `setIfNotExist` / `setIfExist` descriptors to server-side metadata mutations. The server
// applies exactly one of them (create vs. update) based on pre-increment existence, then
// evaluates the condition atomically and returns both the resulting value and metadata.
func (h *hydraidego) IncrementInt16(ctx context.Context, swampName name.Name, key string, value int16, condition *Int16Condition, setIfNotExist *IncrementMetaRequest, setIfExist *IncrementMetaRequest) (int16, *IncrementMetaResponse, error) {

	r := &hydraidepbgo.IncrementInt16Request{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		Key:         key,
		IncrementBy: int32(value),
	}

	if setIfNotExist != nil {
		r.SetIfNotExist = convertMetaToProtoMeta(setIfNotExist)
	}
	if setIfExist != nil {
		r.SetIfExist = convertMetaToProtoMeta(setIfExist)
	}

	if condition != nil {
		r.Condition = &hydraidepbgo.IncrementInt16Condition{
			RelationalOperator: convertRelationalOperatorToProtoOperator(condition.RelationalOperator),
			// convert to int32 because the proto message is int32, but the HydrAIDE will convert it back to int8
			Value: int32(condition.Value),
		}
	}

	response, err := h.client.GetServiceClient(swampName).IncrementInt16(ctx, r)

	if err != nil {
		return 0, nil, errorHandler(err)
	}

	// return with the new value if the increment was successful
	if response.GetIsIncremented() {
		return int16(response.GetValue()), convertIncrementMetaResponse(response.GetMetadata()), nil
	}

	return int16(response.GetValue()), convertIncrementMetaResponse(response.GetMetadata()), NewError(ErrConditionNotMet, fmt.Sprintf("%s: %d", errorMessageConditionNotMet, response.GetValue()))

}

// IncrementInt32 performs an atomic int32 increment on a Treasure inside the given Swamp,
// with optional, server-side metadata control for both the "create" and "update" paths.
//
// Core behavior
// -------------
//   - If the Swamp or Treasure does not exist, HydrAIDE will **create** them.
//   - If a `condition` is provided, the increment runs **atomically on the server** only if
//     the current value satisfies the given relational operator (>, >=, ==, etc.).
//   - The function **always returns** the value and metadata that are currently stored for
//     the Treasure. If the condition is not met, you'll get:
//   - the current value (not incremented),
//   - the resolved metadata (see below),
//   - and `ErrConditionNotMet` as the error.
//     This makes it easy to branch your logic without an extra read.
//
// Metadata control (optional)
// ---------------------------
// You can provide two optional metadata descriptors to control lifecycle/audit fields
// **in the same atomic call**, without pre-checking existence on the client:
//
//   - setIfNotExist (*IncrementMetaRequest):
//     Applied if the Treasure must be **created** as part of this call.
//     Typical use: set CreatedAt/CreatedBy and an initial ExpiredAt.
//
//   - setIfExist (*IncrementMetaRequest):
//     Applied if the Treasure **already exists** (i.e., an update path).
//     Typical use: set UpdatedAt/UpdatedBy and optionally refresh ExpiredAt.
//
// Fields in IncrementMetaRequest:
//   - SetCreatedAt (bool) → when true, server sets CreatedAt=now.UTC()
//   - SetCreatedBy (string) → when non-empty, server sets CreatedBy to this value
//   - SetUpdatedAt (bool) → when true, server sets UpdatedAt=now.UTC()
//   - SetUpdatedBy (string) → when non-empty, server sets UpdatedBy to this value
//   - ExpiredAt (time.Time) → when non-zero, server sets ExpiredAt to this instant
//
// The server chooses which of the two (create/update) descriptors to apply based on whether
// the Treasure existed **before** the increment attempt. You do **not** need to check
// existence on the client side.
//
// Parameters
// ----------
//   - ctx:           context for cancellation and timeout
//   - swampName:     target Swamp where the Treasure lives
//   - key:           unique key of the Treasure to increment
//   - value:         the delta (positive or negative) to add
//   - condition:     optional constraint on the current value before incrementing
//   - setIfNotExist: optional metadata settings if the Treasure must be created
//   - setIfExist:    optional metadata settings if the Treasure already exists
//
// Returns
// -------
//   - int32:                  the value stored after the call (incremented if condition succeeded)
//   - *IncrementMetaResponse: the resolved metadata (CreatedAt/By, UpdatedAt/By, ExpiredAt)
//   - error:                  nil on success; `ErrConditionNotMet` if the condition failed;
//     or a transport/validation error
//
// Notes
// -----
//   - Decrementing is supported by passing a negative delta.
//   - If the Treasure exists but has a **different value type** (e.g. float64), the call fails.
//   - Proto values travel as int32 on the wire and remain int32 here (no narrowing conversion).
//   - Conditional operators (Equal, NotEqual, GreaterThan, GreaterThanOrEqual,
//     LessThan, LessThanOrEqual) compare against the **current** stored value.
//
// Examples
// --------
// 1) Increment with automatic creation + creation metadata
//
//	newVal, meta, err := h.IncrementInt32(
//	  ctx,
//	  name.New().Sanctuary("scores").Realm("users").Swamp("global"),
//	  "user:42",
//	  1,
//	  nil, // no condition
//	  &IncrementMetaRequest{ // setIfNotExist
//	    SetCreatedAt: true,
//	    SetCreatedBy: "game-service",
//	    ExpiredAt:    time.Now().Add(24 * time.Hour),
//	  },
//	  &IncrementMetaRequest{ // setIfExist
//	    SetUpdatedAt: true,
//	    SetUpdatedBy: "game-service",
//	  },
//	)
//
//	// If "user:42" did not exist, it's created, incremented, and Created* + ExpiredAt are set.
//	// If it existed, it's incremented and Updated* are set.
//	// In both cases, `meta` contains the resolved metadata.
//
// 2) Conditional increment that may not run
//
//	newVal, meta, err := h.IncrementInt32(
//	  ctx, swamp, "counter", 5,
//	  &Int32Condition{RelationalOperator: GreaterThan, Value: 10},
//	  nil, nil,
//	)
//
//	// If current value <= 10 → err == ErrConditionNotMet,
//	// but you still get `newVal` (the unchanged current value) and `meta`.
//
// Implementation detail
// ---------------------
// Internally this calls the HydrAIDE gRPC Increment API for int32 and maps the optional
// `setIfNotExist` / `setIfExist` descriptors to server-side metadata mutations. The server
// applies exactly one of them (create vs. update) based on pre-increment existence, then
// evaluates the condition atomically and returns both the resulting value and metadata.
func (h *hydraidego) IncrementInt32(ctx context.Context, swampName name.Name, key string, value int32, condition *Int32Condition, setIfNotExist *IncrementMetaRequest, setIfExist *IncrementMetaRequest) (int32, *IncrementMetaResponse, error) {

	r := &hydraidepbgo.IncrementInt32Request{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		Key:         key,
		IncrementBy: value,
	}

	if setIfNotExist != nil {
		r.SetIfNotExist = convertMetaToProtoMeta(setIfNotExist)
	}
	if setIfExist != nil {
		r.SetIfExist = convertMetaToProtoMeta(setIfExist)
	}

	if condition != nil {
		r.Condition = &hydraidepbgo.IncrementInt32Condition{
			RelationalOperator: convertRelationalOperatorToProtoOperator(condition.RelationalOperator),
			Value:              condition.Value,
		}
	}

	response, err := h.client.GetServiceClient(swampName).IncrementInt32(ctx, r)

	if err != nil {
		return 0, nil, errorHandler(err)
	}

	// return with the new value if the increment was successful
	if response.GetIsIncremented() {
		return response.GetValue(), convertIncrementMetaResponse(response.GetMetadata()), nil
	}

	return response.GetValue(), convertIncrementMetaResponse(response.GetMetadata()), NewError(ErrConditionNotMet, fmt.Sprintf("%s: %d", errorMessageConditionNotMet, response.GetValue()))

}

// IncrementInt64 performs an atomic int64 increment on a Treasure inside the given Swamp,
// with optional, server-side metadata control for both the "create" and "update" paths.
//
// Core behavior
// -------------
//   - If the Swamp or Treasure does not exist, HydrAIDE will **create** them.
//   - If a `condition` is provided, the increment runs **atomically on the server** only if
//     the current value satisfies the given relational operator (>, >=, ==, etc.).
//   - The function **always returns** the value and metadata that are currently stored for
//     the Treasure. If the condition is not met, you'll get:
//   - the current value (not incremented),
//   - the resolved metadata (see below),
//   - and `ErrConditionNotMet` as the error.
//     This makes it easy to branch your logic without an extra read.
//
// Metadata control (optional)
// ---------------------------
// You can provide two optional metadata descriptors to control lifecycle/audit fields
// **in the same atomic call**, without pre-checking existence on the client:
//
//   - setIfNotExist (*IncrementMetaRequest):
//     Applied if the Treasure must be **created** as part of this call.
//     Typical use: set CreatedAt/CreatedBy and an initial ExpiredAt.
//
//   - setIfExist (*IncrementMetaRequest):
//     Applied if the Treasure **already exists** (i.e., an update path).
//     Typical use: set UpdatedAt/UpdatedBy and optionally refresh ExpiredAt.
//
// Fields in IncrementMetaRequest:
//   - SetCreatedAt (bool) → when true, server sets CreatedAt=now.UTC()
//   - SetCreatedBy (string) → when non-empty, server sets CreatedBy to this value
//   - SetUpdatedAt (bool) → when true, server sets UpdatedAt=now.UTC()
//   - SetUpdatedBy (string) → when non-empty, server sets UpdatedBy to this value
//   - ExpiredAt (time.Time) → when non-zero, server sets ExpiredAt to this instant
//
// The server chooses which of the two (create/update) descriptors to apply based on whether
// the Treasure existed **before** the increment attempt. You do **not** need to check
// existence on the client side.
//
// Parameters
// ----------
//   - ctx:           context for cancellation and timeout
//   - swampName:     target Swamp where the Treasure lives
//   - key:           unique key of the Treasure to increment
//   - value:         the delta (positive or negative) to add
//   - condition:     optional constraint on the current value before incrementing
//   - setIfNotExist: optional metadata settings if the Treasure must be created
//   - setIfExist:    optional metadata settings if the Treasure already exists
//
// Returns
// -------
//   - int64:                  the value stored after the call (incremented if condition succeeded)
//   - *IncrementMetaResponse: the resolved metadata (CreatedAt/By, UpdatedAt/By, ExpiredAt)
//   - error:                  nil on success; `ErrConditionNotMet` if the condition failed;
//     or a transport/validation error
//
// Notes
// -----
//   - Decrementing is supported by passing a negative delta.
//   - If the Treasure exists but has a **different value type** (e.g. float64), the call fails.
//   - Proto values travel as int64 on the wire and remain int64 here (no narrowing conversion).
//   - Conditional operators (Equal, NotEqual, GreaterThan, GreaterThanOrEqual,
//     LessThan, LessThanOrEqual) compare against the **current** stored value.
//
// Examples
// --------
// 1) Increment with automatic creation + creation metadata
//
//	newVal, meta, err := h.IncrementInt64(
//	  ctx,
//	  name.New().Sanctuary("scores").Realm("users").Swamp("global"),
//	  "user:42",
//	  1,
//	  nil, // no condition
//	  &IncrementMetaRequest{ // setIfNotExist
//	    SetCreatedAt: true,
//	    SetCreatedBy: "game-service",
//	    ExpiredAt:    time.Now().Add(24 * time.Hour),
//	  },
//	  &IncrementMetaRequest{ // setIfExist
//	    SetUpdatedAt: true,
//	    SetUpdatedBy: "game-service",
//	  },
//	)
//
//	// If "user:42" did not exist, it's created, incremented, and Created* + ExpiredAt are set.
//	// If it existed, it's incremented and Updated* are set.
//	// In both cases, `meta` contains the resolved metadata.
//
// 2) Conditional increment that may not run
//
//	newVal, meta, err := h.IncrementInt64(
//	  ctx, swamp, "counter", 5,
//	  &Int64Condition{RelationalOperator: GreaterThan, Value: 10},
//	  nil, nil,
//	)
//
//	// If current value <= 10 → err == ErrConditionNotMet,
//	// but you still get `newVal` (the unchanged current value) and `meta`.
//
// Implementation detail
// ---------------------
// Internally this calls the HydrAIDE gRPC Increment API for int64 and maps the optional
// `setIfNotExist` / `setIfExist` descriptors to server-side metadata mutations. The server
// applies exactly one of them (create vs. update) based on pre-increment existence, then
// evaluates the condition atomically and returns both the resulting value and metadata.
func (h *hydraidego) IncrementInt64(ctx context.Context, swampName name.Name, key string, value int64, condition *Int64Condition, setIfNotExist *IncrementMetaRequest, setIfExist *IncrementMetaRequest) (int64, *IncrementMetaResponse, error) {

	r := &hydraidepbgo.IncrementInt64Request{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		Key:         key,
		IncrementBy: value,
	}

	if setIfNotExist != nil {
		r.SetIfNotExist = convertMetaToProtoMeta(setIfNotExist)
	}
	if setIfExist != nil {
		r.SetIfExist = convertMetaToProtoMeta(setIfExist)
	}

	if condition != nil {
		r.Condition = &hydraidepbgo.IncrementInt64Condition{
			RelationalOperator: convertRelationalOperatorToProtoOperator(condition.RelationalOperator),
			Value:              condition.Value,
		}
	}

	response, err := h.client.GetServiceClient(swampName).IncrementInt64(ctx, r)

	if err != nil {
		return 0, nil, errorHandler(err)
	}

	// return with the new value if the increment was successful
	if response.GetIsIncremented() {
		return response.GetValue(), convertIncrementMetaResponse(response.GetMetadata()), nil
	}

	return response.GetValue(), convertIncrementMetaResponse(response.GetMetadata()), NewError(ErrConditionNotMet, fmt.Sprintf("%s: %d", errorMessageConditionNotMet, response.GetValue()))

}

// IncrementUint8 performs an atomic uint8 increment on a Treasure inside the given Swamp,
// with optional, server-side metadata control for both the "create" and "update" paths.
//
// Core behavior
// -------------
//   - If the Swamp or Treasure does not exist, HydrAIDE will **create** them.
//   - If a `condition` is provided, the increment runs **atomically on the server** only if
//     the current value satisfies the given relational operator (>, >=, ==, etc.).
//   - The function **always returns** the value and metadata that are currently stored for
//     the Treasure. If the condition is not met, you'll get:
//   - the current value (not incremented),
//   - the resolved metadata (see below),
//   - and `ErrConditionNotMet` as the error.
//     This makes it easy to branch your logic without an extra read.
//
// Metadata control (optional)
// ---------------------------
// You can provide two optional metadata descriptors to control lifecycle/audit fields
// **in the same atomic call**, without pre-checking existence on the client:
//
//   - setIfNotExist (*IncrementMetaRequest):
//     Applied if the Treasure must be **created** as part of this call.
//     Typical use: set CreatedAt/CreatedBy and an initial ExpiredAt.
//
//   - setIfExist (*IncrementMetaRequest):
//     Applied if the Treasure **already exists** (i.e., an update path).
//     Typical use: set UpdatedAt/UpdatedBy and optionally refresh ExpiredAt.
//
// Fields in IncrementMetaRequest:
//   - SetCreatedAt (bool) → when true, server sets CreatedAt=now.UTC()
//   - SetCreatedBy (string) → when non-empty, server sets CreatedBy to this value
//   - SetUpdatedAt (bool) → when true, server sets UpdatedAt=now.UTC()
//   - SetUpdatedBy (string) → when non-empty, server sets UpdatedBy to this value
//   - ExpiredAt (time.Time) → when non-zero, server sets ExpiredAt to this instant
//
// The server chooses which of the two (create/update) descriptors to apply based on whether
// the Treasure existed **before** the increment attempt. You do **not** need to check
// existence on the client side.
//
// Parameters
// ----------
//   - ctx:           context for cancellation and timeout
//   - swampName:     target Swamp where the Treasure lives
//   - key:           unique key of the Treasure to increment
//   - value:         the delta (positive or negative) to add
//   - condition:     optional constraint on the current value before incrementing
//   - setIfNotExist: optional metadata settings if the Treasure must be created
//   - setIfExist:    optional metadata settings if the Treasure already exists
//
// Returns
// -------
//   - uint8:                  the value stored after the call (incremented if condition succeeded)
//   - *IncrementMetaResponse: the resolved metadata (CreatedAt/By, UpdatedAt/By, ExpiredAt)
//   - error:                  nil on success; `ErrConditionNotMet` if the condition failed;
//     or a transport/validation error
//
// Notes
// -----
//   - Decrementing is supported by passing a negative delta (taking into account uint8 underflow rules).
//   - If the Treasure exists but has a **different value type** (e.g. float64), the call fails.
//   - Proto values travel as uint32 on the wire and are converted back to uint8 here.
//   - Conditional operators (Equal, NotEqual, GreaterThan, GreaterThanOrEqual,
//     LessThan, LessThanOrEqual) compare against the **current** stored value.
//
// Examples
// --------
// 1) Increment with automatic creation + creation metadata
//
//	newVal, meta, err := h.IncrementUint8(
//	  ctx,
//	  name.New().Sanctuary("scores").Realm("users").Swamp("global"),
//	  "user:42",
//	  1,
//	  nil, // no condition
//	  &IncrementMetaRequest{ // setIfNotExist
//	    SetCreatedAt: true,
//	    SetCreatedBy: "game-service",
//	    ExpiredAt:    time.Now().Add(24 * time.Hour),
//	  },
//	  &IncrementMetaRequest{ // setIfExist
//	    SetUpdatedAt: true,
//	    SetUpdatedBy: "game-service",
//	  },
//	)
//
//	// If "user:42" did not exist, it's created, incremented, and Created* + ExpiredAt are set.
//	// If it existed, it's incremented and Updated* are set.
//	// In both cases, `meta` contains the resolved metadata.
//
// 2) Conditional increment that may not run
//
//	newVal, meta, err := h.IncrementUint8(
//	  ctx, swamp, "counter", 5,
//	  &Uint8Condition{RelationalOperator: GreaterThan, Value: 10},
//	  nil, nil,
//	)
//
//	// If current value <= 10 → err == ErrConditionNotMet,
//	// but you still get `newVal` (the unchanged current value) and `meta`.
//
// Implementation detail
// ---------------------
// Internally this calls the HydrAIDE gRPC Increment API for uint8 and maps the optional
// `setIfNotExist` / `setIfExist` descriptors to server-side metadata mutations. The server
// applies exactly one of them (create vs. update) based on pre-increment existence, then
// evaluates the condition atomically and returns both the resulting value and metadata.
func (h *hydraidego) IncrementUint8(ctx context.Context, swampName name.Name, key string, value uint8, condition *Uint8Condition, setIfNotExist *IncrementMetaRequest, setIfExist *IncrementMetaRequest) (uint8, *IncrementMetaResponse, error) {
	r := &hydraidepbgo.IncrementUint8Request{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		Key:         key,
		IncrementBy: uint32(value),
	}

	if setIfNotExist != nil {
		r.SetIfNotExist = convertMetaToProtoMeta(setIfNotExist)
	}
	if setIfExist != nil {
		r.SetIfExist = convertMetaToProtoMeta(setIfExist)
	}

	if condition != nil {
		r.Condition = &hydraidepbgo.IncrementUint8Condition{
			RelationalOperator: convertRelationalOperatorToProtoOperator(condition.RelationalOperator),
			// convert to uint32 because the proto message is uint32, but the HydrAIDE will convert it back to uint8
			Value: uint32(condition.Value),
		}
	}

	response, err := h.client.GetServiceClient(swampName).IncrementUint8(ctx, r)
	if err != nil {
		return 0, nil, errorHandler(err)
	}

	if response.GetIsIncremented() {
		return uint8(response.GetValue()), convertIncrementMetaResponse(response.GetMetadata()), nil
	}

	return uint8(response.GetValue()), convertIncrementMetaResponse(response.GetMetadata()), NewError(ErrConditionNotMet, fmt.Sprintf("%s: %d", errorMessageConditionNotMet, response.GetValue()))

}

// IncrementUint16 performs an atomic uint16 increment on a Treasure inside the given Swamp,
// with optional, server-side metadata control for both the "create" and "update" paths.
//
// Core behavior
// -------------
//   - If the Swamp or Treasure does not exist, HydrAIDE will **create** them.
//   - If a `condition` is provided, the increment runs **atomically on the server** only if
//     the current value satisfies the given relational operator (>, >=, ==, etc.).
//   - The function **always returns** the value and metadata that are currently stored for
//     the Treasure. If the condition is not met, you'll get:
//   - the current value (not incremented),
//   - the resolved metadata (see below),
//   - and `ErrConditionNotMet` as the error.
//     This makes it easy to branch your logic without an extra read.
//
// Metadata control (optional)
// ---------------------------
// You can provide two optional metadata descriptors to control lifecycle/audit fields
// **in the same atomic call**, without pre-checking existence on the client:
//
//   - setIfNotExist (*IncrementMetaRequest):
//     Applied if the Treasure must be **created** as part of this call.
//     Typical use: set CreatedAt/CreatedBy and an initial ExpiredAt.
//
//   - setIfExist (*IncrementMetaRequest):
//     Applied if the Treasure **already exists** (i.e., an update path).
//     Typical use: set UpdatedAt/UpdatedBy and optionally refresh ExpiredAt.
//
// Fields in IncrementMetaRequest:
//   - SetCreatedAt (bool) → when true, server sets CreatedAt=now.UTC()
//   - SetCreatedBy (string) → when non-empty, server sets CreatedBy to this value
//   - SetUpdatedAt (bool) → when true, server sets UpdatedAt=now.UTC()
//   - SetUpdatedBy (string) → when non-empty, server sets UpdatedBy to this value
//   - ExpiredAt (time.Time) → when non-zero, server sets ExpiredAt to this instant
//
// The server chooses which of the two (create/update) descriptors to apply based on whether
// the Treasure existed **before** the increment attempt. You do **not** need to check
// existence on the client side.
//
// Parameters
// ----------
//   - ctx:           context for cancellation and timeout
//   - swampName:     target Swamp where the Treasure lives
//   - key:           unique key of the Treasure to increment
//   - value:         the delta (positive or negative) to add
//   - condition:     optional constraint on the current value before incrementing
//   - setIfNotExist: optional metadata settings if the Treasure must be created
//   - setIfExist:    optional metadata settings if the Treasure already exists
//
// Returns
// -------
//   - uint16:                 the value stored after the call (incremented if condition succeeded)
//   - *IncrementMetaResponse: the resolved metadata (CreatedAt/By, UpdatedAt/By, ExpiredAt)
//   - error:                  nil on success; `ErrConditionNotMet` if the condition failed;
//     or a transport/validation error
//
// Notes
// -----
//   - Decrementing is supported by passing a negative delta (taking into account uint16 underflow rules).
//   - If the Treasure exists but has a **different value type** (e.g. float64), the call fails.
//   - Proto values travel as uint32 on the wire and are converted back to uint16 here.
//   - Conditional operators (Equal, NotEqual, GreaterThan, GreaterThanOrEqual,
//     LessThan, LessThanOrEqual) compare against the **current** stored value.
//
// Examples
// --------
// 1) Increment with automatic creation + creation metadata
//
//	newVal, meta, err := h.IncrementUint16(
//	  ctx,
//	  name.New().Sanctuary("scores").Realm("users").Swamp("global"),
//	  "user:42",
//	  1,
//	  nil, // no condition
//	  &IncrementMetaRequest{ // setIfNotExist
//	    SetCreatedAt: true,
//	    SetCreatedBy: "game-service",
//	    ExpiredAt:    time.Now().Add(24 * time.Hour),
//	  },
//	  &IncrementMetaRequest{ // setIfExist
//	    SetUpdatedAt: true,
//	    SetUpdatedBy: "game-service",
//	  },
//	)
//
//	// If "user:42" did not exist, it's created, incremented, and Created* + ExpiredAt are set.
//	// If it existed, it's incremented and Updated* are set.
//	// In both cases, `meta` contains the resolved metadata.
//
// 2) Conditional increment that may not run
//
//	newVal, meta, err := h.IncrementUint16(
//	  ctx, swamp, "counter", 5,
//	  &Uint16Condition{RelationalOperator: GreaterThan, Value: 10},
//	  nil, nil,
//	)
//
//	// If current value <= 10 → err == ErrConditionNotMet,
//	// but you still get `newVal` (the unchanged current value) and `meta`.
//
// Implementation detail
// ---------------------
// Internally this calls the HydrAIDE gRPC Increment API for uint16 and maps the optional
// `setIfNotExist` / `setIfExist` descriptors to server-side metadata mutations. The server
// applies exactly one of them (create vs. update) based on pre-increment existence, then
// evaluates the condition atomically and returns both the resulting value and metadata.
func (h *hydraidego) IncrementUint16(ctx context.Context, swampName name.Name, key string, value uint16, condition *Uint16Condition, setIfNotExist *IncrementMetaRequest, setIfExist *IncrementMetaRequest) (uint16, *IncrementMetaResponse, error) {
	r := &hydraidepbgo.IncrementUint16Request{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		Key:         key,
		IncrementBy: uint32(value),
	}

	if setIfNotExist != nil {
		r.SetIfNotExist = convertMetaToProtoMeta(setIfNotExist)
	}
	if setIfExist != nil {
		r.SetIfExist = convertMetaToProtoMeta(setIfExist)
	}

	if condition != nil {
		r.Condition = &hydraidepbgo.IncrementUint16Condition{
			RelationalOperator: convertRelationalOperatorToProtoOperator(condition.RelationalOperator),
			// convert to uint32 because the proto message is uint32, but the HydrAIDE will convert it back to uint16
			Value: uint32(condition.Value),
		}
	}

	response, err := h.client.GetServiceClient(swampName).IncrementUint16(ctx, r)
	if err != nil {
		return 0, nil, errorHandler(err)
	}

	if response.GetIsIncremented() {
		return uint16(response.GetValue()), convertIncrementMetaResponse(response.GetMetadata()), nil
	}

	return uint16(response.GetValue()), convertIncrementMetaResponse(response.GetMetadata()), NewError(ErrConditionNotMet, fmt.Sprintf("%s: %d", errorMessageConditionNotMet, response.GetValue()))
}

// IncrementUint32 performs an atomic uint32 increment on a Treasure inside the given Swamp,
// with optional, server-side metadata control for both the "create" and "update" paths.
//
// Core behavior
// -------------
//   - If the Swamp or Treasure does not exist, HydrAIDE will **create** them.
//   - If a `condition` is provided, the increment runs **atomically on the server** only if
//     the current value satisfies the given relational operator (>, >=, ==, etc.).
//   - The function **always returns** the value and metadata that are currently stored for
//     the Treasure. If the condition is not met, you'll get:
//   - the current value (not incremented),
//   - the resolved metadata (see below),
//   - and `ErrConditionNotMet` as the error.
//     This makes it easy to branch your logic without an extra read.
//
// Metadata control (optional)
// ---------------------------
// You can provide two optional metadata descriptors to control lifecycle/audit fields
// **in the same atomic call**, without pre-checking existence on the client:
//
//   - setIfNotExist (*IncrementMetaRequest):
//     Applied if the Treasure must be **created** as part of this call.
//     Typical use: set CreatedAt/CreatedBy and an initial ExpiredAt.
//
//   - setIfExist (*IncrementMetaRequest):
//     Applied if the Treasure **already exists** (i.e., an update path).
//     Typical use: set UpdatedAt/UpdatedBy and optionally refresh ExpiredAt.
//
// Fields in IncrementMetaRequest:
//   - SetCreatedAt (bool) → when true, server sets CreatedAt=now.UTC()
//   - SetCreatedBy (string) → when non-empty, server sets CreatedBy to this value
//   - SetUpdatedAt (bool) → when true, server sets UpdatedAt=now.UTC()
//   - SetUpdatedBy (string) → when non-empty, server sets UpdatedBy to this value
//   - ExpiredAt (time.Time) → when non-zero, server sets ExpiredAt to this instant
//
// The server chooses which of the two (create/update) descriptors to apply based on whether
// the Treasure existed **before** the increment attempt. You do **not** need to check
// existence on the client side.
//
// Parameters
// ----------
//   - ctx:           context for cancellation and timeout
//   - swampName:     target Swamp where the Treasure lives
//   - key:           unique key of the Treasure to increment
//   - value:         the delta (positive or negative) to add
//   - condition:     optional constraint on the current value before incrementing
//   - setIfNotExist: optional metadata settings if the Treasure must be created
//   - setIfExist:    optional metadata settings if the Treasure already exists
//
// Returns
// -------
//   - uint32:                 the value stored after the call (incremented if condition succeeded)
//   - *IncrementMetaResponse: the resolved metadata (CreatedAt/By, UpdatedAt/By, ExpiredAt)
//   - error:                  nil on success; `ErrConditionNotMet` if the condition failed;
//     or a transport/validation error
//
// Notes
// -----
//   - Decrementing is supported by passing a “negative” delta conceptually — for uint32 this
//     means you must handle wrap-around semantics yourself. Prefer signed types if you need
//     true negative deltas.
//   - If the Treasure exists but has a **different value type** (e.g. float64), the call fails.
//   - Proto values travel as uint32 on the wire and remain uint32 here (no narrowing conversion).
//   - Conditional operators (Equal, NotEqual, GreaterThan, GreaterThanOrEqual,
//     LessThan, LessThanOrEqual) compare against the **current** stored value.
//
// Examples
// --------
// 1) Increment with automatic creation + creation metadata
//
//	newVal, meta, err := h.IncrementUint32(
//	  ctx,
//	  name.New().Sanctuary("scores").Realm("users").Swamp("global"),
//	  "user:42",
//	  1,
//	  nil, // no condition
//	  &IncrementMetaRequest{ // setIfNotExist
//	    SetCreatedAt: true,
//	    SetCreatedBy: "game-service",
//	    ExpiredAt:    time.Now().Add(24 * time.Hour),
//	  },
//	  &IncrementMetaRequest{ // setIfExist
//	    SetUpdatedAt: true,
//	    SetUpdatedBy: "game-service",
//	  },
//	)
//
//	// If "user:42" did not exist, it's created, incremented, and Created* + ExpiredAt are set.
//	// If it existed, it's incremented and Updated* are set.
//	// In both cases, `meta` contains the resolved metadata.
//
// 2) Conditional increment that may not run
//
//	newVal, meta, err := h.IncrementUint32(
//	  ctx, swamp, "counter", 5,
//	  &Uint32Condition{RelationalOperator: GreaterThan, Value: 10},
//	  nil, nil,
//	)
//
//	// If current value <= 10 → err == ErrConditionNotMet,
//	// but you still get `newVal` (the unchanged current value) and `meta`.
//
// Implementation detail
// ---------------------
// Internally this calls the HydrAIDE gRPC Increment API for uint32 and maps the optional
// `setIfNotExist` / `setIfExist` descriptors to server-side metadata mutations. The server
// applies exactly one of them (create vs. update) based on pre-increment existence, then
// evaluates the condition atomically and returns both the resulting value and metadata.
func (h *hydraidego) IncrementUint32(ctx context.Context, swampName name.Name, key string, value uint32, condition *Uint32Condition, setIfNotExist *IncrementMetaRequest, setIfExist *IncrementMetaRequest) (uint32, *IncrementMetaResponse, error) {
	r := &hydraidepbgo.IncrementUint32Request{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		Key:         key,
		IncrementBy: value,
	}

	if setIfNotExist != nil {
		r.SetIfNotExist = convertMetaToProtoMeta(setIfNotExist)
	}
	if setIfExist != nil {
		r.SetIfExist = convertMetaToProtoMeta(setIfExist)
	}

	if condition != nil {
		r.Condition = &hydraidepbgo.IncrementUint32Condition{
			RelationalOperator: convertRelationalOperatorToProtoOperator(condition.RelationalOperator),
			Value:              condition.Value,
		}
	}

	response, err := h.client.GetServiceClient(swampName).IncrementUint32(ctx, r)
	if err != nil {
		return 0, nil, errorHandler(err)
	}

	if response.GetIsIncremented() {
		return response.GetValue(), convertIncrementMetaResponse(response.GetMetadata()), nil
	}

	return response.GetValue(), convertIncrementMetaResponse(response.GetMetadata()), NewError(ErrConditionNotMet, fmt.Sprintf("%s: %d", errorMessageConditionNotMet, response.GetValue()))
}

// IncrementUint64 performs an atomic uint64 increment on a Treasure inside the given Swamp,
// with optional, server-side metadata control for both the "create" and "update" paths.
//
// Core behavior
// -------------
//   - If the Swamp or Treasure does not exist, HydrAIDE will **create** them.
//   - If a `condition` is provided, the increment runs **atomically on the server** only if
//     the current value satisfies the given relational operator (>, >=, ==, etc.).
//   - The function **always returns** the value and metadata that are currently stored for
//     the Treasure. If the condition is not met, you'll get:
//   - the current value (not incremented),
//   - the resolved metadata (see below),
//   - and `ErrConditionNotMet` as the error.
//     This makes it easy to branch your logic without an extra read.
//
// Metadata control (optional)
// ---------------------------
// You can provide two optional metadata descriptors to control lifecycle/audit fields
// **in the same atomic call**, without pre-checking existence on the client:
//
//   - setIfNotExist (*IncrementMetaRequest):
//     Applied if the Treasure must be **created** as part of this call.
//     Typical use: set CreatedAt/CreatedBy and an initial ExpiredAt.
//
//   - setIfExist (*IncrementMetaRequest):
//     Applied if the Treasure **already exists** (i.e., an update path).
//     Typical use: set UpdatedAt/UpdatedBy and optionally refresh ExpiredAt.
//
// Fields in IncrementMetaRequest:
//   - SetCreatedAt (bool) → when true, server sets CreatedAt=now.UTC()
//   - SetCreatedBy (string) → when non-empty, server sets CreatedBy to this value
//   - SetUpdatedAt (bool) → when true, server sets UpdatedAt=now.UTC()
//   - SetUpdatedBy (string) → when non-empty, server sets UpdatedBy to this value
//   - ExpiredAt (time.Time) → when non-zero, server sets ExpiredAt to this instant
//
// The server chooses which of the two (create/update) descriptors to apply based on whether
// the Treasure existed **before** the increment attempt. You do **not** need to check
// existence on the client side.
//
// Parameters
// ----------
//   - ctx:           context for cancellation and timeout
//   - swampName:     target Swamp where the Treasure lives
//   - key:           unique key of the Treasure to increment
//   - value:         the delta to add (uint64)
//   - condition:     optional constraint on the current value before incrementing
//   - setIfNotExist: optional metadata settings if the Treasure must be created
//   - setIfExist:    optional metadata settings if the Treasure already exists
//
// Returns
// -------
//   - uint64:                 the value stored after the call (incremented if condition succeeded)
//   - *IncrementMetaResponse: the resolved metadata (CreatedAt/By, UpdatedAt/By, ExpiredAt)
//   - error:                  nil on success; `ErrConditionNotMet` if the condition failed;
//     or a transport/validation error
//
// Notes
// -----
//   - Unsigned underflow/overflow semantics apply. If you need negative deltas, prefer a signed type.
//   - If the Treasure exists but has a **different value type** (e.g. float64), the call fails.
//   - Proto values travel as uint64 on the wire and remain uint64 here (no narrowing conversion).
//   - Conditional operators (Equal, NotEqual, GreaterThan, GreaterThanOrEqual,
//     LessThan, LessThanOrEqual) compare against the **current** stored value.
//
// Examples
// --------
// 1) Increment with automatic creation + creation metadata
//
//	newVal, meta, err := h.IncrementUint64(
//	  ctx,
//	  name.New().Sanctuary("scores").Realm("users").Swamp("global"),
//	  "user:42",
//	  1,
//	  nil, // no condition
//	  &IncrementMetaRequest{ // setIfNotExist
//	    SetCreatedAt: true,
//	    SetCreatedBy: "game-service",
//	    ExpiredAt:    time.Now().Add(24 * time.Hour),
//	  },
//	  &IncrementMetaRequest{ // setIfExist
//	    SetUpdatedAt: true,
//	    SetUpdatedBy: "game-service",
//	  },
//	)
//
//	// If "user:42" did not exist, it's created, incremented, and Created* + ExpiredAt are set.
//	// If it existed, it's incremented and Updated* are set.
//	// In both cases, `meta` contains the resolved metadata.
//
// 2) Conditional increment that may not run
//
//	newVal, meta, err := h.IncrementUint64(
//	  ctx, swamp, "counter", 5,
//	  &Uint64Condition{RelationalOperator: GreaterThan, Value: 10},
//	  nil, nil,
//	)
//
//	// If current value <= 10 → err == ErrConditionNotMet,
//	// but you still get `newVal` (the unchanged current value) and `meta`.
//
// Implementation detail
// ---------------------
// Internally this calls the HydrAIDE gRPC Increment API for uint64 and maps the optional
// `setIfNotExist` / `setIfExist` descriptors to server-side metadata mutations. The server
// applies exactly one of them (create vs. update) based on pre-increment existence, then
// evaluates the condition atomically and returns both the resulting value and metadata.
func (h *hydraidego) IncrementUint64(ctx context.Context, swampName name.Name, key string, value uint64, condition *Uint64Condition, setIfNotExist *IncrementMetaRequest, setIfExist *IncrementMetaRequest) (uint64, *IncrementMetaResponse, error) {
	r := &hydraidepbgo.IncrementUint64Request{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		Key:         key,
		IncrementBy: value,
	}

	if setIfNotExist != nil {
		r.SetIfNotExist = convertMetaToProtoMeta(setIfNotExist)
	}
	if setIfExist != nil {
		r.SetIfExist = convertMetaToProtoMeta(setIfExist)
	}

	if condition != nil {
		r.Condition = &hydraidepbgo.IncrementUint64Condition{
			RelationalOperator: convertRelationalOperatorToProtoOperator(condition.RelationalOperator),
			Value:              condition.Value,
		}
	}

	response, err := h.client.GetServiceClient(swampName).IncrementUint64(ctx, r)
	if err != nil {
		return 0, nil, errorHandler(err)
	}

	if response.GetIsIncremented() {
		return response.GetValue(), convertIncrementMetaResponse(response.GetMetadata()), nil
	}

	return response.GetValue(), convertIncrementMetaResponse(response.GetMetadata()), NewError(ErrConditionNotMet, fmt.Sprintf("%s: %d", errorMessageConditionNotMet, response.GetValue()))
}

// IncrementFloat32 performs an atomic float32 increment on a Treasure inside the given Swamp,
// with optional, server-side metadata control for both the "create" and "update" paths.
//
// Core behavior
// -------------
//   - If the Swamp or Treasure does not exist, HydrAIDE will **create** them.
//   - If a `condition` is provided, the increment runs **atomically on the server** only if
//     the current value satisfies the given relational operator (>, >=, ==, etc.).
//   - The function **always returns** the value and metadata that are currently stored for
//     the Treasure. If the condition is not met, you'll get:
//   - the current value (not incremented),
//   - the resolved metadata (see below),
//   - and `ErrConditionNotMet` as the error.
//     This makes it easy to branch your logic without an extra read.
//
// Metadata control (optional)
// ---------------------------
// You can provide two optional metadata descriptors to control lifecycle/audit fields
// **in the same atomic call**, without pre-checking existence on the client:
//
//   - setIfNotExist (*IncrementMetaRequest):
//     Applied if the Treasure must be **created** as part of this call.
//     Typical use: set CreatedAt/CreatedBy and an initial ExpiredAt.
//
//   - setIfExist (*IncrementMetaRequest):
//     Applied if the Treasure **already exists** (i.e., an update path).
//     Typical use: set UpdatedAt/UpdatedBy and optionally refresh ExpiredAt.
//
// Fields in IncrementMetaRequest:
//   - SetCreatedAt (bool) → when true, server sets CreatedAt=now.UTC()
//   - SetCreatedBy (string) → when non-empty, server sets CreatedBy to this value
//   - SetUpdatedAt (bool) → when true, server sets UpdatedAt=now.UTC()
//   - SetUpdatedBy (string) → when non-empty, server sets UpdatedBy to this value
//   - ExpiredAt (time.Time) → when non-zero, server sets ExpiredAt to this instant
//
// The server chooses which of the two (create/update) descriptors to apply based on whether
// the Treasure existed **before** the increment attempt. You do **not** need to check
// existence on the client side.
//
// Parameters
// ----------
//   - ctx:           context for cancellation and timeout
//   - swampName:     target Swamp where the Treasure lives
//   - key:           unique key of the Treasure to increment
//   - value:         the delta (positive or negative) to add
//   - condition:     optional constraint on the current value before incrementing
//   - setIfNotExist: optional metadata settings if the Treasure must be created
//   - setIfExist:    optional metadata settings if the Treasure already exists
//
// Returns
// -------
//   - float32:               the value stored after the call (incremented if condition succeeded)
//   - *IncrementMetaResponse: the resolved metadata (CreatedAt/By, UpdatedAt/By, ExpiredAt)
//   - error:                  nil on success; `ErrConditionNotMet` if the condition failed;
//     or a transport/validation error
//
// Notes
// -----
//   - Decrementing is supported by passing a negative delta.
//   - If the Treasure exists but has a **different value type** (e.g. int64), the call fails.
//   - Conditional operators (Equal, NotEqual, GreaterThan, GreaterThanOrEqual,
//     LessThan, LessThanOrEqual) compare against the **current** stored value.
//   - Floating point precision follows Go's float32 rules; tiny differences can
//     affect equality-based conditions.
//
// Examples
// --------
// 1) Increment with automatic creation + creation metadata
//
//	newVal, meta, err := h.IncrementFloat32(
//	  ctx,
//	  name.New().Sanctuary("scores").Realm("users").Swamp("global"),
//	  "user:42",
//	  1.5,
//	  nil, // no condition
//	  &IncrementMetaRequest{ // setIfNotExist
//	    SetCreatedAt: true,
//	    SetCreatedBy: "calc-service",
//	    ExpiredAt:    time.Now().Add(24 * time.Hour),
//	  },
//	  &IncrementMetaRequest{ // setIfExist
//	    SetUpdatedAt: true,
//	    SetUpdatedBy: "calc-service",
//	  },
//	)
//
//	// If "user:42" did not exist, it's created, incremented, and Created* + ExpiredAt are set.
//	// If it existed, it's incremented and Updated* are set.
//	// In both cases, `meta` contains the resolved metadata.
//
// 2) Conditional increment that may not run
//
//	newVal, meta, err := h.IncrementFloat32(
//	  ctx, swamp, "ratio", 0.25,
//	  &Float32Condition{RelationalOperator: GreaterThan, Value: 5.0},
//	  nil, nil,
//	)
//
//	// If current value <= 5.0 → err == ErrConditionNotMet,
//	// but you still get `newVal` (the unchanged current value) and `meta`.
//
// Implementation detail
// ---------------------
// Internally this calls the HydrAIDE gRPC Increment API for float32 and maps the optional
// `setIfNotExist` / `setIfExist` descriptors to server-side metadata mutations. The server
// applies exactly one of them (create vs. update) based on pre-increment existence, then
// evaluates the condition atomically and returns both the resulting value and metadata.
func (h *hydraidego) IncrementFloat32(ctx context.Context, swampName name.Name, key string, value float32, condition *Float32Condition, setIfNotExist *IncrementMetaRequest, setIfExist *IncrementMetaRequest) (float32, *IncrementMetaResponse, error) {
	r := &hydraidepbgo.IncrementFloat32Request{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		Key:         key,
		IncrementBy: value,
	}

	if setIfNotExist != nil {
		r.SetIfNotExist = convertMetaToProtoMeta(setIfNotExist)
	}
	if setIfExist != nil {
		r.SetIfExist = convertMetaToProtoMeta(setIfExist)
	}

	if condition != nil {
		r.Condition = &hydraidepbgo.IncrementFloat32Condition{
			RelationalOperator: convertRelationalOperatorToProtoOperator(condition.RelationalOperator),
			Value:              condition.Value,
		}
	}

	response, err := h.client.GetServiceClient(swampName).IncrementFloat32(ctx, r)
	if err != nil {
		return 0, nil, errorHandler(err)
	}

	if response.GetIsIncremented() {
		return response.GetValue(), convertIncrementMetaResponse(response.GetMetadata()), nil
	}

	return response.GetValue(), convertIncrementMetaResponse(response.GetMetadata()), NewError(ErrConditionNotMet, fmt.Sprintf("%s: %f", errorMessageConditionNotMet, response.GetValue()))
}

// IncrementFloat64 performs an atomic float64 increment on a Treasure inside the given Swamp,
// with optional, server-side metadata control for both the "create" and "update" paths.
//
// Core behavior
// -------------
//   - If the Swamp or Treasure does not exist, HydrAIDE will **create** them.
//   - If a `condition` is provided, the increment runs **atomically on the server** only if
//     the current value satisfies the given relational operator (>, >=, ==, etc.).
//   - The function **always returns** the value and metadata that are currently stored for
//     the Treasure. If the condition is not met, you'll get:
//   - the current value (not incremented),
//   - the resolved metadata (see below),
//   - and `ErrConditionNotMet` as the error.
//     This makes it easy to branch your logic without an extra read.
//
// Metadata control (optional)
// ---------------------------
// You can provide two optional metadata descriptors to control lifecycle/audit fields
// **in the same atomic call**, without pre-checking existence on the client:
//
//   - setIfNotExist (*IncrementMetaRequest):
//     Applied if the Treasure must be **created** as part of this call.
//     Typical use: set CreatedAt/CreatedBy and an initial ExpiredAt.
//
//   - setIfExist (*IncrementMetaRequest):
//     Applied if the Treasure **already exists** (i.e., an update path).
//     Typical use: set UpdatedAt/UpdatedBy and optionally refresh ExpiredAt.
//
// Fields in IncrementMetaRequest:
//   - SetCreatedAt (bool) → when true, server sets CreatedAt=now.UTC()
//   - SetCreatedBy (string) → when non-empty, server sets CreatedBy to this value
//   - SetUpdatedAt (bool) → when true, server sets UpdatedAt=now.UTC()
//   - SetUpdatedBy (string) → when non-empty, server sets UpdatedBy to this value
//   - ExpiredAt (time.Time) → when non-zero, server sets ExpiredAt to this instant
//
// The server chooses which of the two (create/update) descriptors to apply based on whether
// the Treasure existed **before** the increment attempt. You do **not** need to check
// existence on the client side.
//
// Parameters
// ----------
//   - ctx:           context for cancellation and timeout
//   - swampName:     target Swamp where the Treasure lives
//   - key:           unique key of the Treasure to increment
//   - value:         the delta (positive or negative) to add
//   - condition:     optional constraint on the current value before incrementing
//   - setIfNotExist: optional metadata settings if the Treasure must be created
//   - setIfExist:    optional metadata settings if the Treasure already exists
//
// Returns
// -------
//   - float64:               the value stored after the call (incremented if condition succeeded)
//   - *IncrementMetaResponse: the resolved metadata (CreatedAt/By, UpdatedAt/By, ExpiredAt)
//   - error:                  nil on success; `ErrConditionNotMet` if the condition failed;
//     or a transport/validation error
//
// Notes
// -----
//   - Decrementing is supported by passing a negative delta.
//   - If the Treasure exists but has a **different value type** (e.g. int32), the call fails.
//   - Conditional operators (Equal, NotEqual, GreaterThan, GreaterThanOrEqual,
//     LessThan, LessThanOrEqual) compare against the **current** stored value.
//   - Floating point precision follows Go's float64 rules; tiny differences can
//     affect equality-based conditions.
//
// Examples
// --------
// 1) Increment with automatic creation + creation metadata
//
//	newVal, meta, err := h.IncrementFloat64(
//	  ctx,
//	  name.New().Sanctuary("scores").Realm("users").Swamp("global"),
//	  "user:42",
//	  2.75,
//	  nil, // no condition
//	  &IncrementMetaRequest{ // setIfNotExist
//	    SetCreatedAt: true,
//	    SetCreatedBy: "calc-service",
//	    ExpiredAt:    time.Now().Add(24 * time.Hour),
//	  },
//	  &IncrementMetaRequest{ // setIfExist
//	    SetUpdatedAt: true,
//	    SetUpdatedBy: "calc-service",
//	  },
//	)
//
//	// If "user:42" did not exist, it's created, incremented, and Created* + ExpiredAt are set.
//	// If it existed, it's incremented and Updated* are set.
//	// In both cases, `meta` contains the resolved metadata.
//
// 2) Conditional increment that may not run
//
//	newVal, meta, err := h.IncrementFloat64(
//	  ctx, swamp, "ratio", 0.5,
//	  &Float64Condition{RelationalOperator: GreaterThan, Value: 10.0},
//	  nil, nil,
//	)
//
//	// If current value <= 10.0 → err == ErrConditionNotMet,
//	// but you still get `newVal` (the unchanged current value) and `meta`.
//
// Implementation detail
// ---------------------
// Internally this calls the HydrAIDE gRPC Increment API for float64 and maps the optional
// `setIfNotExist` / `setIfExist` descriptors to server-side metadata mutations. The server
// applies exactly one of them (create vs. update) based on pre-increment existence, then
// evaluates the condition atomically and returns both the resulting value and metadata.
func (h *hydraidego) IncrementFloat64(ctx context.Context, swampName name.Name, key string, value float64, condition *Float64Condition, setIfNotExist *IncrementMetaRequest, setIfExist *IncrementMetaRequest) (float64, *IncrementMetaResponse, error) {

	r := &hydraidepbgo.IncrementFloat64Request{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		Key:         key,
		IncrementBy: value,
	}

	if setIfNotExist != nil {
		r.SetIfNotExist = convertMetaToProtoMeta(setIfNotExist)
	}
	if setIfExist != nil {
		r.SetIfExist = convertMetaToProtoMeta(setIfExist)
	}

	if condition != nil {
		r.Condition = &hydraidepbgo.IncrementFloat64Condition{
			RelationalOperator: convertRelationalOperatorToProtoOperator(condition.RelationalOperator),
			Value:              condition.Value,
		}
	}

	response, err := h.client.GetServiceClient(swampName).IncrementFloat64(ctx, r)
	if err != nil {
		return 0, nil, errorHandler(err)
	}

	if response.GetIsIncremented() {
		return response.GetValue(), convertIncrementMetaResponse(response.GetMetadata()), nil
	}

	return response.GetValue(), convertIncrementMetaResponse(response.GetMetadata()), NewError(ErrConditionNotMet, fmt.Sprintf("%s: %f", errorMessageConditionNotMet, response.GetValue()))

}

type KeyValuesPair struct {
	Key    string
	Values []uint32
}

// Uint32SlicePush adds unique uint32 values to multiple slice-type Treasures within a given Swamp.
//
// For each key in the provided KeyValuesPair list, the function will push the given values
// to the corresponding slice in the Swamp — but **only if those values are not already present**.
//
// If the Swamp or any referenced Treasure does not yet exist, they will be **automatically created**.
//
// 🧠 This is an atomic, idempotent mutation function for managing uint32 slices in HydrAIDE.
//
// Parameters:
//   - ctx:           context for cancellation and timeout
//   - swampName:     the target Swamp where the Treasures are stored
//   - KeyValuesPair: list of keys and the values to add to each corresponding Treasure slice
//
// Behavior:
//   - If a value is **already present** in the slice, it will not be added again.
//   - Values that are **not yet present** will be appended in the order received.
//   - The operation is **atomic** per key: each slice update is isolated and deduplicated server-side.
//   - The Swamp and Treasures will be **auto-created** if they don't exist.
//   - If the Treasure exists but is **not of uint32 slice type**, an error is returned.
//
// Returns:
//   - nil if all operations succeed
//   - error only if there is a low-level database or type mismatch issue
//
// ✅ Example usage:
//
//	err := sdk.Uint32SlicePush(ctx, "index:reverse", []*KeyValuesPair{
//	  {Key: "domain:google.com", Values: []uint32{123, 456}},
//	  {Key: "domain:openai.com",  Values: []uint32{789}},
//	})
//
//	// Result:
//	// - domain:google.com slice will now include 123 and 456 (only if not already present)
//	// - domain:openai.com slice will now include 789
func (h *hydraidego) Uint32SlicePush(ctx context.Context, swampName name.Name, KeyValuesPair []*KeyValuesPair) error {

	keySlices := make([]*hydraidepbgo.KeySlicePair, 0, len(KeyValuesPair))

	for _, value := range KeyValuesPair {
		keySlices = append(keySlices, &hydraidepbgo.KeySlicePair{
			Key:    value.Key,
			Values: value.Values,
		})
	}

	_, err := h.client.GetServiceClient(swampName).Uint32SlicePush(ctx, &hydraidepbgo.AddToUint32SlicePushRequest{
		IslandID:      swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:     swampName.Get(),
		KeySlicePairs: keySlices,
	})

	if err != nil {
		return errorHandler(err)
	}

	return nil

}

// Uint32SliceDelete removes specific uint32 values from slice-type Treasures inside a given Swamp.
//
// For each key in the provided KeyValuesPair list, the function attempts to delete the specified
// values from the corresponding Treasure's uint32 slice.
//
// ⚠️ If the Treasure does not exist, the operation **does not return an error** — it is treated as a no-op.
//
// 🧠 This is an atomic, idempotent mutation function with built-in garbage collection:
//   - If a Treasure becomes empty after deletion, it is **automatically removed**
//   - If a Swamp becomes empty as a result, it is **also removed**
//
// Parameters:
//   - ctx:           context for cancellation and timeout
//   - swampName:     the target Swamp where the Treasures are stored
//   - KeyValuesPair: list of keys and the values to remove from each corresponding Treasure slice
//
// Behavior:
//   - Values that do not exist in the slice will be ignored (no error)
//   - Treasures that do not exist will be skipped (no error)
//   - Empty Treasures are deleted automatically
//   - Empty Swamps are deleted automatically
//   - The operation is **atomic per key**, and safe to repeat (idempotent)
//
// Returns:
//   - nil if all operations succeed or are skipped
//   - error only in case of low-level database or type mismatch issues
//
// ✅ Example usage:
//
//	err := sdk.Uint32SliceDelete(ctx, "index:reverse", []*KeyValuesPair{
//	  {Key: "domain:google.com", Values: []uint32{123, 456}},
//	  {Key: "domain:openai.com",  Values: []uint32{789}},
//	})
//
//	// Result:
//	// - domain:google.com: values 123 and 456 are removed (if present)
//	// - domain:openai.com: value 789 is removed (if present)
//	// - Empty Treasures/Swamps are automatically garbage collected
func (h *hydraidego) Uint32SliceDelete(ctx context.Context, swampName name.Name, KeyValuesPair []*KeyValuesPair) error {

	keySlices := make([]*hydraidepbgo.KeySlicePair, 0, len(KeyValuesPair))

	for _, value := range KeyValuesPair {
		keySlices = append(keySlices, &hydraidepbgo.KeySlicePair{
			Key:    value.Key,
			Values: value.Values,
		})
	}

	_, err := h.client.GetServiceClient(swampName).Uint32SliceDelete(ctx, &hydraidepbgo.Uint32SliceDeleteRequest{
		IslandID:      swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:     swampName.Get(),
		KeySlicePairs: keySlices,
	})

	if err != nil {
		return errorHandler(err)
	}

	return nil

}

// Uint32SliceSize returns the number of unique uint32 values stored in a slice-type Treasure.
//
// This operation is useful for diagnostics, monitoring, or when you need to evaluate
// whether a slice is empty, near capacity, or ready for cleanup.
//
// 🧠 This is a read-only, atomic operation that works on slice-based Treasures.
//
//	It only applies to Treasures that store `[]uint32` values.
//
// Parameters:
//   - ctx:        context for cancellation and timeout
//   - swampName:  the name of the Swamp where the Treasure lives
//   - key:        the unique key of the Treasure to inspect
//
// Behavior:
//   - If the key does **not exist**, an `ErrCodeInvalidArgument` is returned
//   - If the key exists but is **not a uint32 slice**, an `ErrCodeFailedPrecondition` is returned
//   - Otherwise, returns the exact number of values in the slice
//
// Returns:
//   - the current size of the slice (number of elements)
//   - error if the key is invalid or a low-level database error occurs
//
// ✅ Example usage:
//
//	size, err := sdk.Uint32SliceSize(ctx, "index:reverse", "domain:openai.com")
//	if err != nil {
//	  log.Fatal(err)
//	}
//	fmt.Printf("Slice has %d items.\n", size)
func (h *hydraidego) Uint32SliceSize(ctx context.Context, swampName name.Name, key string) (int64, error) {

	response, err := h.client.GetServiceClient(swampName).Uint32SliceSize(ctx, &hydraidepbgo.Uint32SliceSizeRequest{
		IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName: swampName.Get(),
		Key:       key,
	})

	if err != nil {
		if s, ok := status.FromError(err); ok {
			switch s.Code() {
			case codes.Unavailable:
				return 0, NewError(ErrCodeConnectionError, errorMessageConnectionError)
			case codes.DeadlineExceeded:
				return 0, NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
			case codes.FailedPrecondition:
				return 0, NewError(ErrCodeFailedPrecondition, fmt.Sprintf("%v", s.Message()))
			case codes.InvalidArgument:
				return 0, NewError(ErrCodeInvalidArgument, "the key does not exist")
			case codes.Internal:
				return 0, NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("%s: %v", errorMessageInternalError, s.Message()))
			default:
				return 0, NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
			}
		} else {
			return 0, NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
		}
	}

	// if the request was successful, return the size
	return response.GetSize(), nil

}

// Uint32SliceIsValueExist checks whether a specific uint32 value exists in the slice-type Treasure.
//
// This is a lightweight, read-only operation that can be used to validate if a reverse index
// already contains a given value before pushing or deleting it.
//
// 🧠 This is a fast lookup function that works on `[]uint32`-based Treasures.
//
//	It is particularly useful in indexing, deduplication, and logic-driven filtering.
//
// Parameters:
//   - ctx:        context for cancellation and timeout
//   - swampName:  the name of the Swamp where the Treasure lives
//   - key:        the unique key of the Treasure (i.e., the slice container)
//   - value:      the uint32 value to check for existence in the slice
//
// Behavior:
//   - If the key exists and the value is present, returns `true`
//   - If the key exists but the value is not in the slice, returns `false`
//   - If the key does not exist or type is invalid, returns an error
//
// Returns:
//   - `true` if the value is found in the slice
//   - `false` if not found
//   - `error` if the key is invalid, type mismatched, or a database-level failure occurred
//
// ✅ Example usage:
//
//	exists, err := sdk.Uint32SliceIsValueExist(ctx, "index:reverse", "domain:google.com", 123)
//	if err != nil {
//	  log.Fatal(err)
//	}
//	if exists {
//	  fmt.Println("Already indexed")
//	} else {
//	  fmt.Println("Needs indexing")
//	}
func (h *hydraidego) Uint32SliceIsValueExist(ctx context.Context, swampName name.Name, key string, value uint32) (bool, error) {

	response, err := h.client.GetServiceClient(swampName).Uint32SliceIsValueExist(ctx, &hydraidepbgo.Uint32SliceIsValueExistRequest{
		IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName: swampName.Get(),
		Key:       key,
		Value:     value,
	})

	if err != nil {
		if s, ok := status.FromError(err); ok {
			switch s.Code() {
			case codes.Unavailable:
				return false, NewError(ErrCodeConnectionError, errorMessageConnectionError)
			case codes.DeadlineExceeded:
				return false, NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
			case codes.FailedPrecondition:
				return false, NewError(ErrCodeFailedPrecondition, fmt.Sprintf("%v", s.Message()))
			case codes.Internal:
				return false, NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("%s: %v", errorMessageInternalError, s.Message()))
			default:
				return false, NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
			}
		} else {
			return false, NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
		}
	}

	// if the request was successful
	return response.GetIsExist(), nil

}

func getKeyFromProfileModel(model any) ([]string, error) {

	// check if the model is not a pointer
	v := reflect.ValueOf(model)

	// ellenőrizzük, hogy a model egy pointer-e és egy struct-e
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return nil, errors.New("input must be a pointer to a struct")
	}

	var keys []string

	v = v.Elem()
	t := v.Type()

	// get the keys from the struct
	for i := 0; i < t.NumField(); i++ {
		keys = append(keys, t.Field(i).Name)
	}

	return keys, nil

}

func setTreasureValueToProfileModel(model any, treasure *hydraidepbgo.Treasure) error {

	key := treasure.GetKey()
	// find the key in the model by the name of the field.

	v := reflect.ValueOf(model)
	v = v.Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).Name == key {
			// we found the key in the model
			field := v.Field(i)
			if err := setProtoTreasureToModel(treasure, field); err != nil {
				return err
			}
		}
	}

	return nil

}

// toOptionalTimestamppb converts a *time.Time to a *timestamppb.Timestamp.
// Returns nil if t is nil or zero.
func toOptionalTimestamppb(t *time.Time) *timestamppb.Timestamp {
	if t != nil && !t.IsZero() {
		return timestamppb.New(*t)
	}
	return nil
}

// ConvertIndexTypeToProtoIndexType convert the index type to proto index type
func convertIndexTypeToProtoIndexType(indexType IndexType) hydraidepbgo.IndexType_Type {
	switch indexType {
	case IndexKey:
		return hydraidepbgo.IndexType_KEY
	case IndexValueString:
		return hydraidepbgo.IndexType_VALUE_STRING
	case IndexValueUint8:
		return hydraidepbgo.IndexType_VALUE_UINT8
	case IndexValueUint16:
		return hydraidepbgo.IndexType_VALUE_UINT16
	case IndexValueUint32:
		return hydraidepbgo.IndexType_VALUE_UINT32
	case IndexValueUint64:
		return hydraidepbgo.IndexType_VALUE_UINT64
	case IndexValueInt8:
		return hydraidepbgo.IndexType_VALUE_INT8
	case IndexValueInt16:
		return hydraidepbgo.IndexType_VALUE_INT16
	case IndexValueInt32:
		return hydraidepbgo.IndexType_VALUE_INT32
	case IndexValueInt64:
		return hydraidepbgo.IndexType_VALUE_INT64
	case IndexValueFloat32:
		return hydraidepbgo.IndexType_VALUE_FLOAT32
	case IndexValueFloat64:
		return hydraidepbgo.IndexType_VALUE_FLOAT64
	case IndexExpirationTime:
		return hydraidepbgo.IndexType_EXPIRATION_TIME
	case IndexCreationTime:
		return hydraidepbgo.IndexType_CREATION_TIME
	case IndexUpdateTime:
		return hydraidepbgo.IndexType_UPDATE_TIME
	default:
		return hydraidepbgo.IndexType_CREATION_TIME
	}
}

// ConvertOrderTypeToProtoOrderType convert the order type to proto order type
func convertOrderTypeToProtoOrderType(orderType IndexOrder) hydraidepbgo.OrderType_Type {
	switch orderType {
	case IndexOrderAsc:
		return hydraidepbgo.OrderType_ASC
	case IndexOrderDesc:
		return hydraidepbgo.OrderType_DESC
	default:
		return hydraidepbgo.OrderType_ASC
	}
}

// convertCatalogModelToKeyValuePair converts a Go struct (passed as pointer) into a HydrAIDE-compatible KeyValuePair message.
//
// 🧠 This is an **internal serialization helper** used by the Go SDK to translate user-defined models
// into the binary format that HydrAIDE expects when inserting or updating Treasures.
//
// ✅ Supported field tags:
// - `hydraide:"key"`       → Marks the string field to use as the Treasure key (must be non-empty).
// - `hydraide:"value"`     → Marks the value field (can be any supported primitive or complex type).
// - `hydraide:"expireAt"`  → Optional `time.Time`, marks the logical expiry time of the Treasure.
// - `hydraide:"createdAt"` / `createdBy` / `updatedAt` / `updatedBy` → Optional metadata fields.
// - `hydraide:"omitempty"` → Skips the field during encoding if it's zero, nil, or empty.
//
// ✅ Supported value types:
// - Primitives: string, bool, int, uint, float (various widths)
// - time.Time (as int64 UNIX timestamp)
// - Slices and maps (serialized as GOB or MessagePack binary blobs depending on encoding setting)
// - Structs and pointers (also GOB or MessagePack encoded)
// - `nil` / empty values are optionally excluded if marked with `omitempty`
//
// ⚠️ Requirements:
// - The input **must be a pointer to a struct**, otherwise the function returns an error.
// - The struct **must contain a field marked as `hydrun:"key"`** with a non-empty string.
// - The value can be a primitive or complex field marked with `hydrun:"value"`.
// - If no value is provided, the resulting KeyValuePair will include a `VoidVal=true` marker.
//
// 🧬 Why this matters:
// HydrAIDE works with protocol-level binary messages.
// Every Treasure must be sent as a KeyValuePair with a valid key and (optionally) a value.
// This function bridges Go structs and HydrAIDE’s native format, abstracting encoding logic.
//
// ✨ This is how arbitrary business models (e.g. `UserProfile`, `InvoiceItem`) are safely,
// efficiently and correctly transformed into Treasure representations.
//
// 📌 If you're building a new SDK (e.g. for Python, Rust, Node.js), your implementation
// should follow the same principles:
// - Tag-driven key/value separation
// - Support for void values and expiration
// - Metadata injection
// - Optional field skipping (e.g. omitempty)
// - Consistent type coercion for known value types
func convertCatalogModelToKeyValuePair(model any, encoding EncodingFormat) (*hydraidepbgo.KeyValuePair, error) {

	// Get the reflection value of the input model
	v := reflect.ValueOf(model)

	// 🧪 Validate the input: it must be a pointer to a struct.
	// This is required because we'll be using reflection to iterate over the fields
	// and extract tags and values dynamically. Non-pointer or non-struct inputs are invalid.
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return nil, errors.New("input must be a pointer to a struct")
	}

	// Initialize the KeyValuePair that will hold the final encoded output
	kvPair := &hydraidepbgo.KeyValuePair{}

	// Get the actual struct (dereferenced value) and its type
	v = v.Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {

		field := t.Field(i)

		// Check if the current field is marked as the `key` field (via `hydraide:"key"` tag)
		if key, ok := field.Tag.Lookup(tagHydrAIDE); ok && key == tagKey {

			value := v.Field(i)

			// Validate that the field is a non-empty string — required for all HydrAIDE Treasures.
			// Keys must always be explicit and unique within a Swamp.
			if value.Kind() == reflect.String && value.String() != "" {
				// Found the key — assign it to the KeyValuePair
				kvPair.Key = value.String()
				continue
			}

			// If the key field is missing or empty, this is an invalid model
			return nil, errors.New("key field must be a non-empty string")
		}

		// Check if the current field is tagged as the `value` field (via `hydraide:"value"`)
		// This field holds the actual value of the Treasure.
		// We detect its type using reflection and populate the corresponding proto field in KeyValuePair.
		if key, ok := field.Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagValue) {

			value := v.Field(i)
			isEmpty := isFieldEmpty(value)
			if isEmpty {
				// This flag tracks whether any value has been set.
				// If no value is provided (only key or metadata), we'll later set VoidVal = true.
				valueVoid := true
				kvPair.VoidVal = &valueVoid
			}
			if strings.Contains(key, tagOmitempty) && isEmpty {
				// If omitempty is set and the field is empty, skip setting the value
				continue
			}

			// convert the value to KeyValuePair
			if err := convertFieldToKvPair(value, kvPair, encoding); err != nil {
				return nil, err
			}

		}

		// Process the `expireAt` field (tagged with `hydraide:"expireAt"`).
		// This defines the logical expiration time of the Treasure.
		// Once the given timestamp is reached, HydrAIDE will treat the record as expired.
		// - Must be of type `time.Time`
		// - If omitempty is set, zero values are skipped without error
		// - Otherwise must be non-zero
		// - Automatically converted to a `timestamppb.Timestamp` for protobuf
		if key, ok := field.Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagExpireAt) {

			value := v.Field(i)
			hasOmitempty := strings.Contains(key, tagOmitempty)

			if hasOmitempty && isFieldEmpty(value) {
				// If omitempty is set and the field is empty, skip setting expireAt
				continue
			}

			if value.Kind() != reflect.Struct || value.Type() != reflect.TypeOf(time.Time{}) {
				return nil, errors.New("expireAt field must be a time.Time")
			}
			expireAt := value.Interface().(time.Time).UTC()

			// Only validate non-zero if omitempty is NOT set
			if !hasOmitempty && expireAt.IsZero() {
				return nil, errors.New("expireAt field must be a non-zero time.Time")
			}

			// If omitempty is set and we got here, the value is non-zero, so we can set it
			if !expireAt.IsZero() {
				kvPair.ExpiredAt = timestamppb.New(expireAt)
			}
			continue

		}

		// Process the `createdBy` field (tagged with `hydraide:"createdBy"`).
		// Optional metadata indicating who or what created the Treasure.
		// - Must be of type `string`
		// - Empty values are ignored
		if key, ok := field.Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagCreatedBy) {

			value := v.Field(i)

			if strings.Contains(key, tagOmitempty) && isFieldEmpty(value) {
				// If omitempty is set and the field is empty, skip setting createdBy
				continue
			}

			if value.Kind() != reflect.String {
				return nil, errors.New("createdBy field must be a string")
			}

			if value.String() != "" {
				createdBy := value.String()
				kvPair.CreatedBy = &createdBy
			}

			continue
		}

		// Process the `createdAt` field (tagged with `hydraide:"createdAt"`).
		// Optional metadata representing when the Treasure was created.
		// - Must be of type `time.Time`
		// - If omitempty is set, zero values are skipped without error
		// - Otherwise must be non-zero
		// - Converted to protobuf-compatible timestamp
		if key, ok := field.Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagCreatedAt) {

			value := v.Field(i)
			hasOmitempty := strings.Contains(key, tagOmitempty)

			if hasOmitempty && isFieldEmpty(value) {
				continue
			}

			if value.Kind() != reflect.Struct || value.Type() != reflect.TypeOf(time.Time{}) {
				return nil, errors.New("createdAt field must be a time.Time")
			}
			createdAt := value.Interface().(time.Time).UTC()

			// Only validate non-zero if omitempty is NOT set
			if !hasOmitempty && createdAt.IsZero() {
				return nil, errors.New("createdAt field must be a non-zero time.Time")
			}

			// If omitempty is set and we got here, the value is non-zero, so we can set it
			if !createdAt.IsZero() {
				kvPair.CreatedAt = timestamppb.New(createdAt)
			}
			continue
		}

		// Process the `updatedBy` field (tagged with `hydraide:"updatedBy"`).
		// Optional metadata indicating who or what last updated the Treasure.
		// - Must be of type `string`
		// - If omitempty is set, empty values are skipped
		// - Otherwise empty values are still allowed but not set
		if key, ok := field.Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagUpdatedBy) {

			value := v.Field(i)
			hasOmitempty := strings.Contains(key, tagOmitempty)

			if hasOmitempty && isFieldEmpty(value) {
				// If omitempty is set and the field is empty, skip setting updatedBy
				continue
			}

			if value.Kind() != reflect.String {
				return nil, errors.New("updatedBy field must be a string")
			}

			// Only set if the value is non-empty
			if value.String() != "" {
				updatedBy := value.String()
				kvPair.UpdatedBy = &updatedBy
			}
			continue
		}

		// Process the `updatedAt` field (tagged with `hydraide:"updatedAt"`).
		// Optional metadata representing the last modification time of the Treasure.
		// - Must be of type `time.Time`
		// - If omitempty is set, zero values are skipped without error
		// - Otherwise must be non-zero
		// - Automatically converted to a `timestamppb.Timestamp` for protobuf transmission
		if key, ok := field.Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagUpdatedAt) {

			value := v.Field(i)
			hasOmitempty := strings.Contains(key, tagOmitempty)

			if hasOmitempty && isFieldEmpty(value) {
				// If omitempty is set and the field is empty, skip setting updatedAt
				continue
			}

			if value.Kind() != reflect.Struct || value.Type() != reflect.TypeOf(time.Time{}) {
				return nil, errors.New("updatedAt field must be a time.Time")
			}
			updatedAt := value.Interface().(time.Time).UTC()

			// Only validate non-zero if omitempty is NOT set
			if !hasOmitempty && updatedAt.IsZero() {
				return nil, errors.New("updatedAt field must be a non-zero time.Time")
			}

			// If omitempty is set and we got here, the value is non-zero, so we can set it
			if !updatedAt.IsZero() {
				kvPair.UpdatedAt = timestamppb.New(updatedAt)
			}
			continue
		}

	}

	// Final validation: the key must be present and non-empty.
	// This is a hard requirement — all Treasures in HydrAIDE must have a key.
	if kvPair.Key == "" {
		return nil, errors.New("key field not found")
	}

	// Return the fully constructed KeyValuePair for insertion into the system.
	return kvPair, nil

}

// convertProtoTreasureToCatalogModel maps a hydraidepbgo.Treasure protobuf object back into a Go struct.
//
// The target model must be a pointer to a struct. Fields are matched using `hydraide` struct tags:
// - `key`: assigns Treasure.Key to the struct's key field.
// - `value`: maps the appropriate typed value from Treasure into the struct's value field.
// - `expireAt`, `createdBy`, `createdAt`, `updatedBy`, `updatedAt`: optional metadata fields.
//
// Supported value conversions include:
// - Primitive types: string, bool, intX, uintX, floatX
// - time.Time (from int64 UNIX timestamp)
// - []byte (raw bytes)
// - All other slices, maps, and pointers (GOB-encoded in BytesVal)
//
// If the field type does not match the Treasure value type, it is silently skipped.
// If decoding fails (e.g. from GOB), an error is returned.
func convertProtoTreasureToCatalogModel(treasure *hydraidepbgo.Treasure, model any) error {

	v := reflect.ValueOf(model)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return errors.New("input must be a pointer to a struct at convertProtoTreasureToCatalogModel")
	}

	t := v.Elem().Type()
	for i := 0; i < t.NumField(); i++ {

		if key, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagKey) {
			v.Elem().Field(i).SetString(treasure.GetKey())
			continue
		}

		if key, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagValue) {

			field := v.Elem().Field(i)

			// set proto treasure to model
			if err := setProtoTreasureToModel(treasure, field); err != nil {
				return err
			}

			continue

		}

		if key, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagExpireAt) {
			if treasure.ExpiredAt != nil {
				v.Elem().Field(i).Set(reflect.ValueOf(treasure.ExpiredAt.AsTime()))
			}
			continue
		}

		if key, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagCreatedBy) {
			if treasure.CreatedBy != nil {
				v.Elem().Field(i).SetString(*treasure.CreatedBy)
			}
			continue
		}

		if key, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagCreatedAt) {
			if treasure.CreatedAt != nil {
				v.Elem().Field(i).Set(reflect.ValueOf(treasure.CreatedAt.AsTime()))
			}
			continue
		}

		if key, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagUpdatedBy) {
			if treasure.UpdatedBy != nil {
				v.Elem().Field(i).SetString(*treasure.UpdatedBy)
			}
			continue
		}

		if key, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagUpdatedAt) {
			if treasure.UpdatedAt != nil {
				v.Elem().Field(i).Set(reflect.ValueOf(treasure.UpdatedAt.AsTime()))
			}
			continue
		}

	}

	return nil

}

// setSearchMetaOnModel populates the model's searchMeta-tagged field from a proto SearchResultMeta.
// This field is read-only: it is only populated during search/read responses and is never
// processed during write operations (Set, Create, Update).
func setSearchMetaOnModel(model any, protoMeta *hydraidepbgo.SearchResultMeta) {
	if protoMeta == nil {
		return
	}
	if len(protoMeta.VectorScores) == 0 && len(protoMeta.MatchedLabels) == 0 {
		return
	}

	v := reflect.ValueOf(model)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return
	}

	t := v.Elem().Type()
	for i := 0; i < t.NumField(); i++ {
		if tag, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && tag == tagSearchMeta {
			field := v.Elem().Field(i)
			if field.Type() == reflect.TypeOf((*SearchMeta)(nil)) {
				meta := &SearchMeta{
					VectorScores:  protoMeta.VectorScores,
					MatchedLabels: protoMeta.MatchedLabels,
				}
				field.Set(reflect.ValueOf(meta))
			}
			return
		}
	}
}

func setProtoTreasureToModel(treasure *hydraidepbgo.Treasure, field reflect.Value) error {

	if treasure.StringVal != nil {
		switch field.Kind() {
		case reflect.String:
			field.SetString(treasure.GetStringVal())
			return nil
		default:
			return nil
		}
	}

	if treasure.Uint8Val != nil {
		switch field.Kind() {
		case reflect.Uint8:
			field.SetUint(uint64(treasure.GetUint8Val()))
			return nil
		default:
			// skip the field because the value type is not the same as the model field type
			return nil
		}
	}

	if treasure.Uint16Val != nil {
		switch field.Kind() {
		case reflect.Uint16:
			field.SetUint(uint64(treasure.GetUint16Val()))
			return nil
		default:
			// skip the field because the value type is not the same as the model field type
			return nil
		}
	}

	if treasure.Uint32Val != nil {
		switch field.Kind() {
		case reflect.Uint32:
			field.SetUint(uint64(treasure.GetUint32Val()))
			return nil
		default:
			// skip the field because the value type is not the same as the model field type
			return nil
		}
	}

	if treasure.Uint64Val != nil {
		switch field.Kind() {
		case reflect.Uint64:
			field.SetUint(treasure.GetUint64Val())
			return nil
		default:
			// skip the field because the value type is not the same as the model field type
			return nil
		}
	}

	if treasure.Int8Val != nil {
		switch field.Kind() {
		case reflect.Int8:
			field.SetInt(int64(treasure.GetInt8Val()))
			return nil
		default:
			// skip the field because the value type is not the same as the model field type
			return nil
		}
	}

	if treasure.Int16Val != nil {
		switch field.Kind() {
		case reflect.Int16:
			field.SetInt(int64(treasure.GetInt16Val()))
			return nil
		default:
			// skip the field because the value type is not the same as the model field type
			return nil
		}
	}

	if treasure.Int32Val != nil {
		switch field.Kind() {
		case reflect.Int32:
			field.SetInt(int64(treasure.GetInt32Val()))
			return nil
		default:
			// skip the field because the value type is not the same as the model field type
			return nil
		}
	}

	if treasure.Int64Val != nil {
		switch field.Kind() {
		case reflect.Int64:
			field.SetInt(treasure.GetInt64Val())
			return nil

		case reflect.Struct:

			// ha time.Time típusú mezőről van szó
			if field.Type() == reflect.TypeOf(time.Time{}) {
				// konvertáljuk vissza time.Time-ra az int64 UNIX timestampet
				timestamp := time.Unix(treasure.GetInt64Val(), 0).UTC()
				field.Set(reflect.ValueOf(timestamp))
			}
			return nil

		default:
			// skip the field because the value type is not the same as the model field type
			return nil
		}
	}

	if treasure.Float32Val != nil {
		switch field.Kind() {
		case reflect.Float32:
			field.SetFloat(float64(treasure.GetFloat32Val()))
			return nil
		default:
			// skip the field because the value type is not the same as the model field type
			return nil
		}
	}

	if treasure.Float64Val != nil {
		switch field.Kind() {
		case reflect.Float64:
			field.SetFloat(treasure.GetFloat64Val())
			return nil
		default:
			// skip the field because the value type is not the same as the model field type
			return nil
		}
	}

	if treasure.BoolVal != nil {
		switch field.Kind() {
		case reflect.Bool:
			field.SetBool(treasure.GetBoolVal() == hydraidepbgo.Boolean_TRUE)
			return nil
		default:
			// skip the field because the value type is not the same as the model field type
			return nil
		}
	}

	if treasure.BytesVal != nil {
		switch field.Kind() {
		case reflect.Slice:
			if field.Type().Elem().Kind() == reflect.Uint8 {
				// Raw []byte → direct assignment
				field.SetBytes(treasure.GetBytesVal())
			} else {
				data := treasure.GetBytesVal()
				decoded := reflect.New(field.Type()).Interface()

				if isMsgpackEncoded(data) {
					// MessagePack decode
					if err := msgpack.Unmarshal(unwrapMsgpack(data), decoded); err != nil {
						return fmt.Errorf("failed to msgpack-decode slice field: %w", err)
					}
				} else {
					// Legacy GOB decode
					decoder := gob.NewDecoder(bytes.NewReader(data))
					if err := decoder.Decode(decoded); err != nil {
						return fmt.Errorf("failed to gob-decode slice field: %w", err)
					}
				}

				field.Set(reflect.ValueOf(decoded).Elem())
			}

		case reflect.Map, reflect.Ptr:
			data := treasure.GetBytesVal()
			decoded := reflect.New(field.Type()).Interface()

			if isMsgpackEncoded(data) {
				// MessagePack decode
				if err := msgpack.Unmarshal(unwrapMsgpack(data), decoded); err != nil {
					return fmt.Errorf("failed to msgpack-decode map/ptr field: %w", err)
				}
			} else {
				// Legacy GOB decode
				decoder := gob.NewDecoder(bytes.NewReader(data))
				if err := decoder.Decode(decoded); err != nil {
					return fmt.Errorf("failed to gob-decode map/ptr field: %w", err)
				}
			}

			field.Set(reflect.ValueOf(decoded).Elem())

		default:
			return nil
		}
	}

	if treasure.Uint32Slice != nil {
		switch field.Kind() {
		case reflect.Slice:
			if field.Type().Elem().Kind() == reflect.Uint32 {
				field.Set(reflect.ValueOf(treasure.GetUint32Slice()))
			} else {
				return fmt.Errorf("failed to set uint32 slice to field: field is not []uint32 but %s", field.Type().String())
			}
		default:
			return fmt.Errorf("failed to set uint32 slice to field: field is not a slice but %s", field.Type().String())
		}
	}

	return nil

}

// convertProfileModelToKeyValuePair converts a complex model to key-value pairs
// and collects deletable keys (if tagged with hydraide:"deletable" and empty).
//
// This function is used to serialize a Go struct (passed as a pointer) into a slice of HydrAIDE-compatible KeyValuePair messages.
// It also collects the names of fields that are empty and marked as deletable, so they can be removed from the database.
//
// Rules:
// - The input must be a pointer to a struct, otherwise an error is returned.
// - For each struct field:
//   - If the field is empty and tagged with hydraide:"deletable", its name is added to the deleteKeys slice.
//   - If the field is empty and tagged with hydraide:"omitempty", it is skipped (not saved).
//   - Otherwise, a KeyValuePair is created for the field and added to the kvPairs slice.
//
// - Returns the list of KeyValuePair objects, the list of deletable keys, and an error if any.
func convertProfileModelToKeyValuePair(model any, encoding EncodingFormat) ([]*hydraidepbgo.KeyValuePair, []string, error) {
	// Check if the model is a pointer to a struct
	v := reflect.ValueOf(model)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return nil, nil, errors.New("input must be a pointer to a struct")
	}

	var kvPairs []*hydraidepbgo.KeyValuePair
	var deleteKeys []string

	v = v.Elem()
	t := v.Type()

	// Iterate over struct fields
	for i := 0; i < t.NumField(); i++ {

		field := t.Field(i)
		value := v.Field(i)

		// Parse hydraide tags
		if tag, ok := field.Tag.Lookup(tagHydrAIDE); ok {

			tags := strings.Split(tag, ",")
			isOmitempty := slices.Contains(tags, tagOmitempty)
			isDeletable := slices.Contains(tags, tagDeletable)

			if isFieldEmpty(value) {
				if isDeletable {
					// If empty and deletable, add to deleteKeys
					deleteKeys = append(deleteKeys, field.Name)
					continue
				}
				if isOmitempty {
					// If empty and omitempty, skip saving
					continue
				}
			}

		}

		// Normal KeyValuePair creation
		kvPair := &hydraidepbgo.KeyValuePair{
			Key: field.Name,
		}

		if err := convertFieldToKvPair(value, kvPair, encoding); err != nil {
			return nil, nil, err
		}

		kvPairs = append(kvPairs, kvPair)

	}

	return kvPairs, deleteKeys, nil
}

func isFieldEmpty(value reflect.Value) bool {

	// Evaluate "emptiness" based on Go's zero-value semantics per type
	// Strings → must not be empty
	// Pointers → must not be nil
	// Numbers → must not be zero
	// Slices/Maps → must not be nil or empty
	// time.Time → must not be zero (uninitialized)
	if (value.Kind() == reflect.String && value.String() == "") ||
		(value.Kind() == reflect.Ptr && value.IsNil()) ||
		(value.Kind() == reflect.Int8 && value.Int() == 0) ||
		(value.Kind() == reflect.Int16 && value.Int() == 0) ||
		(value.Kind() == reflect.Int32 && value.Int() == 0) ||
		(value.Kind() == reflect.Int64 && value.Int() == 0) ||
		(value.Kind() == reflect.Int && value.Int() == 0) ||
		(value.Kind() == reflect.Uint8 && value.Uint() == 0) ||
		(value.Kind() == reflect.Uint16 && value.Uint() == 0) ||
		(value.Kind() == reflect.Uint32 && value.Uint() == 0) ||
		(value.Kind() == reflect.Uint64 && value.Uint() == 0) ||
		(value.Kind() == reflect.Uint && value.Uint() == 0) ||
		(value.Kind() == reflect.Float32 && value.Float() == 0) ||
		(value.Kind() == reflect.Float64 && value.Float() == 0) ||
		(value.Kind() == reflect.Slice && (value.IsNil() || value.Len() == 0)) ||
		(value.Kind() == reflect.Map && (value.IsNil() || value.Len() == 0)) ||
		(value.Type() == reflect.TypeOf(time.Time{}) && value.Interface().(time.Time).IsZero()) {

		// If the field is empty, skip further processing and continue to the next field
		return true
	}

	return false

}

// msgpack format detection constants.
// We prepend a 2-byte magic prefix to all MessagePack-encoded data: [0xC7, 0x00]
// (MessagePack ext format with length 0 and type 0). GOB will never produce this sequence.
const (
	msgpackMagic0 byte = 0xC7
	msgpackMagic1 byte = 0x00
)

// isMsgpackEncoded returns true if the byte slice has the MessagePack magic prefix.
func isMsgpackEncoded(data []byte) bool {
	return len(data) >= 2 && data[0] == msgpackMagic0 && data[1] == msgpackMagic1
}

// wrapMsgpack prepends the MessagePack magic prefix to the encoded data.
func wrapMsgpack(data []byte) []byte {
	result := make([]byte, len(data)+2)
	result[0] = msgpackMagic0
	result[1] = msgpackMagic1
	copy(result[2:], data)
	return result
}

// unwrapMsgpack removes the 2-byte MessagePack magic prefix.
func unwrapMsgpack(data []byte) []byte {
	return data[2:]
}

// convert one field to a key value pair
func convertFieldToKvPair(value reflect.Value, kvPair *hydraidepbgo.KeyValuePair, encoding EncodingFormat) (err error) {

	switch value.Kind() {
	// 🧵 Simple primitives (string, bool, numbers)
	case reflect.String:
		stringVal := value.String()
		kvPair.StringVal = &stringVal
	case reflect.Bool:
		// HydrAIDE uses a custom Boolean enum to allow storing `false` values explicitly
		boolVal := hydraidepbgo.Boolean_FALSE
		if value.Bool() {
			boolVal = hydraidepbgo.Boolean_TRUE
		}
		kvPair.BoolVal = &boolVal
	// 🧮 Unsigned integers
	case reflect.Uint8:
		val := uint32(value.Uint())
		kvPair.Uint8Val = &val
	case reflect.Uint16:
		val := uint32(value.Uint())
		kvPair.Uint16Val = &val
	case reflect.Uint32:
		val := uint32(value.Uint())
		kvPair.Uint32Val = &val
	case reflect.Uint64:
		intVal := value.Uint()
		kvPair.Uint64Val = &intVal
	// 🔢 Signed integers
	case reflect.Int8:
		val := int32(value.Int())
		kvPair.Int8Val = &val
	case reflect.Int16:
		val := int32(value.Int())
		kvPair.Int16Val = &val
	case reflect.Int32:
		val := int32(value.Int())
		kvPair.Int32Val = &val
	case reflect.Int64:
		intVal := value.Int()
		kvPair.Int64Val = &intVal
	// 🔬 Floating point numbers
	case reflect.Float32:
		floatVal := float32(value.Float())
		kvPair.Float32Val = &floatVal
	case reflect.Float64:
		floatVal := value.Float()
		kvPair.Float64Val = &floatVal
	// Complex binary types – slices, maps, pointers, structs (excluding time)
	case reflect.Slice:

		// Special case for []byte → raw binary value (no encoding wrapper)
		if value.Type().Elem().Kind() == reflect.Uint8 {
			kvPair.BytesVal = value.Bytes()
		} else if encoding == EncodingMsgPack {
			encoded, encErr := msgpack.Marshal(value.Interface())
			if encErr != nil {
				err = fmt.Errorf("could not msgpack-encode slice: %w", encErr)
				break
			}
			kvPair.BytesVal = wrapMsgpack(encoded)
		} else {
			registerGobTypeIfNeeded(value.Interface())
			var buf bytes.Buffer
			encoder := gob.NewEncoder(&buf)
			if encErr := encoder.Encode(value.Interface()); encErr != nil {
				err = fmt.Errorf("could not GOB-encode slice: %w", encErr)
				break
			}
			kvPair.BytesVal = buf.Bytes()
		}

	case reflect.Map:
		if encoding == EncodingMsgPack {
			encoded, encErr := msgpack.Marshal(value.Interface())
			if encErr != nil {
				err = fmt.Errorf("could not msgpack-encode map: %w", encErr)
				break
			}
			kvPair.BytesVal = wrapMsgpack(encoded)
		} else {
			registerGobTypeIfNeeded(value.Interface())
			var buf bytes.Buffer
			encoder := gob.NewEncoder(&buf)
			if encErr := encoder.Encode(value.Interface()); encErr != nil {
				err = fmt.Errorf("could not GOB-encode map: %w", encErr)
				break
			}
			kvPair.BytesVal = buf.Bytes()
		}

	case reflect.Ptr:

		if value.IsNil() {
			// Ignore nil pointers
			break
		}

		if encoding == EncodingMsgPack {
			encoded, encErr := msgpack.Marshal(value.Interface())
			if encErr != nil {
				err = fmt.Errorf("could not msgpack-encode pointer value: %w", encErr)
				break
			}
			kvPair.BytesVal = wrapMsgpack(encoded)
		} else {
			registerGobTypeIfNeeded(value.Interface())
			var buf bytes.Buffer
			encoder := gob.NewEncoder(&buf)
			if encErr := encoder.Encode(value.Interface()); encErr != nil {
				err = fmt.Errorf("could not GOB-encode pointer value: %w", encErr)
				break
			}
			kvPair.BytesVal = buf.Bytes()
		}

	// 🕒 Special case for time.Time → store as int64 (Unix timestamp)
	case reflect.Struct:
		if value.Type() == reflect.TypeOf(time.Time{}) {
			timeValue := value.Interface().(time.Time)
			if !timeValue.IsZero() {
				intVal := timeValue.UTC().Unix()
				kvPair.Int64Val = &intVal
			}
		}

	// ❌ Any other unsupported type is rejected explicitly
	default:
		err = errors.New(fmt.Sprintf("unsupported value type: %s", value.Kind().String()))
	}

	return err

}

var (
	// registeredTypes keeps track of all types registered with gob to prevent duplicate registrations.
	registeredTypes = make(map[reflect.Type]struct{})
	mutex           sync.Mutex
)

// registerGobTypeIfNeeded safely registers a type with the gob encoder,
// making sure the same type is not registered multiple times.
//
// This function handles both pointer and base types by recursively registering
// the underlying struct when a pointer is provided.
//
// If the type has already been registered, the function does nothing.
// If gob.Register panics (e.g. due to naming conflicts), it recovers gracefully.
func registerGobTypeIfNeeded(val interface{}) {
	t := reflect.TypeOf(val)

	// If it's a pointer, first register the underlying base type (e.g., *StructX -> StructX)
	if t.Kind() == reflect.Ptr {
		registerGobTypeIfNeeded(reflect.Zero(t.Elem()).Interface())
		// Avoid registering the pointer type separately to reduce conflict risk.
		// Remove this return if you want to explicitly register both base and pointer types.
		return
	}

	mutex.Lock()
	defer mutex.Unlock()

	// Only register if not already registered
	if _, ok := registeredTypes[t]; !ok {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("Recovered in registerGobTypeIfNeeded: %v\n", r)
			}
		}()

		gob.Register(val)
		registeredTypes[t] = struct{}{}
	}
}

// convertProtoStatusToStatus convert the proto status to the event status
func convertProtoStatusToStatus(status hydraidepbgo.Status_Code) EventStatus {

	switch status {
	case hydraidepbgo.Status_NOT_FOUND:
		return StatusTreasureNotFound
	case hydraidepbgo.Status_NEW:
		return StatusNew
	case hydraidepbgo.Status_UPDATED:
		return StatusModified
	case hydraidepbgo.Status_DELETED:
		return StatusDeleted
	case hydraidepbgo.Status_NOTHING_CHANGED:
		return StatusNothingChanged
	default:
		return StatusNothingChanged
	}

}

func errorHandler(err error) error {

	if s, ok := status.FromError(err); ok {
		switch s.Code() {
		case codes.Aborted:
			// HydrAIDE server is shutting down
			return NewError(ErrorShuttingDown, errorMessageShuttingDown)
		case codes.Unavailable:
			return NewError(ErrCodeConnectionError, errorMessageConnectionError)
		case codes.DeadlineExceeded:
			return NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
		case codes.Canceled:
			return NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient)
		case codes.FailedPrecondition:
			return NewError(ErrCodeSwampNotFound, fmt.Sprintf("%s: %v", errorMessageSwampNotFound, s.Message()))
		case codes.Internal:
			return NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("%s: %v", errorMessageInternalError, s.Message()))
		default:
			return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
		}
	} else {
		return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
	}

}

// ConvertRelationalOperatorToProtoOperator connvert the relational operator to proto operator
func convertRelationalOperatorToProtoOperator(operator RelationalOperator) hydraidepbgo.Relational_Operator {
	switch operator {
	case NotEqual:
		return hydraidepbgo.Relational_NOT_EQUAL
	case GreaterThanOrEqual:
		return hydraidepbgo.Relational_GREATER_THAN_OR_EQUAL
	case GreaterThan:
		return hydraidepbgo.Relational_GREATER_THAN
	case LessThanOrEqual:
		return hydraidepbgo.Relational_LESS_THAN_OR_EQUAL
	case LessThan:
		return hydraidepbgo.Relational_LESS_THAN
	case Contains:
		return hydraidepbgo.Relational_CONTAINS
	case NotContains:
		return hydraidepbgo.Relational_NOT_CONTAINS
	case StartsWith:
		return hydraidepbgo.Relational_STARTS_WITH
	case EndsWith:
		return hydraidepbgo.Relational_ENDS_WITH
	case IsEmpty:
		return hydraidepbgo.Relational_IS_EMPTY
	case IsNotEmpty:
		return hydraidepbgo.Relational_IS_NOT_EMPTY
	case HasKey:
		return hydraidepbgo.Relational_HAS_KEY
	case HasNotKey:
		return hydraidepbgo.Relational_HAS_NOT_KEY
	case SliceContains:
		return hydraidepbgo.Relational_SLICE_CONTAINS
	case SliceNotContains:
		return hydraidepbgo.Relational_SLICE_NOT_CONTAINS
	case SliceContainsSubstring:
		return hydraidepbgo.Relational_SLICE_CONTAINS_SUBSTRING
	case SliceNotContainsSubstring:
		return hydraidepbgo.Relational_SLICE_NOT_CONTAINS_SUBSTRING
	case Equal:
		fallthrough
	default:
		return hydraidepbgo.Relational_EQUAL
	}
}

// ErrorCode represents predefined error codes used throughout the HydrAIDE SDK.
type ErrorCode int

const (
	ErrCodeConnectionError ErrorCode = iota
	ErrCodeInternalDatabaseError
	ErrCodeCtxClosedByClient
	ErrCodeCtxTimeout
	ErrCodeSwampNotFound
	ErrCodeFailedPrecondition
	ErrCodeInvalidArgument
	ErrCodeNotFound
	ErrCodeAlreadyExists
	ErrCodeInvalidModel
	ErrConditionNotMet
	ErrCodeUnknown
	ErrorShuttingDown ErrorCode = 1001
)

// Error represents a structured error used across HydrAIDE operations.
type Error struct {
	Code    ErrorCode // Unique error code
	Message string    // Human-readable error message
}

// Error implements the built-in error interface.
func (e *Error) Error() string {
	return fmt.Sprintf("Code: %d, Message: %s", e.Code, e.Message)
}

// NewError creates a new instance of HydrAIDE error with a given code and message.
func NewError(code ErrorCode, message string) error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// GetErrorCode extracts the ErrorCode from an error, if available.
// If the error is nil or not a HydrAIDE error, ErrCodeUnknown is returned.
func GetErrorCode(err error) ErrorCode {
	if err == nil {
		return ErrCodeUnknown
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ErrCodeUnknown
}

// GetErrorMessage returns the message from a HydrAIDE error.
// If the error is not of type *Error, an empty string is returned.
func GetErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Message
	}
	return ""
}

// IsConnectionError returns true if the error indicates a connection issue
// between the client and the Hydra database service.
func IsConnectionError(err error) bool {
	return GetErrorCode(err) == ErrCodeConnectionError
}

// IsInternalDatabaseError returns true if the error was caused by an internal
// failure within the Hydra database system.
func IsInternalDatabaseError(err error) bool {
	return GetErrorCode(err) == ErrCodeInternalDatabaseError
}

// IsCtxClosedByClient returns true if the operation failed because the context
// was cancelled by the client.
func IsCtxClosedByClient(err error) bool {
	return GetErrorCode(err) == ErrCodeCtxClosedByClient
}

// IsCtxTimeout returns true if the operation failed due to a context timeout.
func IsCtxTimeout(err error) bool {
	return GetErrorCode(err) == ErrCodeCtxTimeout
}

// IsSwampNotFound returns true if the requested swamp (data space) was not found.
// This may not always be a strict error, but it indicates the absence of the swamp.
func IsSwampNotFound(err error) bool {
	return GetErrorCode(err) == ErrCodeSwampNotFound
}

// IsFailedPrecondition returns true if the operation was not executed
// because the preconditions were not met.
func IsFailedPrecondition(err error) bool {
	return GetErrorCode(err) == ErrCodeFailedPrecondition
}

// IsInvalidArgument returns true if the error was caused by invalid input parameters,
// such as malformed keys or unsupported filter values.
func IsInvalidArgument(err error) bool {
	return GetErrorCode(err) == ErrCodeInvalidArgument
}

// IsNotFound returns true if a specific entity (e.g. lock, key, swamp) was not found.
// The meaning depends on the function context, such as missing key or lock in Unlock(),
// or missing swamp in Read().
func IsNotFound(err error) bool {
	return GetErrorCode(err) == ErrCodeNotFound
}

// IsAlreadyExists returns true if an entity (such as a key or ID) already exists and
// cannot be overwritten.
func IsAlreadyExists(err error) bool {
	return GetErrorCode(err) == ErrCodeAlreadyExists
}

// IsInvalidModel returns true if the given model structure is invalid or cannot be
// properly serialized for the requested operation.
func IsInvalidModel(err error) bool {
	return GetErrorCode(err) == ErrCodeInvalidModel
}

// IsUnknown returns true if the error does not match any known HydrAIDE error code.
func IsUnknown(err error) bool {
	return GetErrorCode(err) == ErrCodeUnknown
}

func IsConditionNotMet(err error) bool {
	return GetErrorCode(err) == ErrConditionNotMet
}
