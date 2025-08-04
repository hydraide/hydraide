package hydraidego

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"github.com/hydraide/hydraide/generated/hydraidepbgo"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/client"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"io"
	"reflect"
	"sync"
	"time"
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
)

const (
	tagHydrAIDE  = "hydraide"
	tagKey       = "key"
	tagValue     = "value"
	tagOmitempty = "omitempty"
	tagCreatedAt = "createdAt"
	tagCreatedBy = "createdBy"
	tagUpdatedAt = "updatedAt"
	tagUpdatedBy = "updatedBy"
	tagExpireAt  = "expireAt"
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
	CatalogUpdate(ctx context.Context, swampName name.Name, model any) error
	CatalogUpdateMany(ctx context.Context, swampName name.Name, models []any, iterator CatalogUpdateManyIteratorFunc) error
	CatalogDelete(ctx context.Context, swampName name.Name, key string) error
	CatalogDeleteMany(ctx context.Context, swampName name.Name, keys []string, iterator CatalogDeleteIteratorFunc) error
	CatalogDeleteManyFromMany(ctx context.Context, request []*CatalogDeleteManyFromManyRequest, iterator CatalogDeleteIteratorFunc) error
	CatalogSave(ctx context.Context, swampName name.Name, model any) (eventStatus EventStatus, err error)
	CatalogSaveMany(ctx context.Context, swampName name.Name, models []any, iterator CatalogSaveManyIteratorFunc) error
	CatalogSaveManyToMany(ctx context.Context, request []*CatalogManyToManyRequest, iterator CatalogSaveManyToManyIteratorFunc) error
	CatalogShiftExpired(ctx context.Context, swampName name.Name, howMany int32, model any, iterator CatalogShiftExpiredIteratorFunc) error
	ProfileSave(ctx context.Context, swampName name.Name, model any) (err error)
	ProfileRead(ctx context.Context, swampName name.Name, model any) (err error)
	Count(ctx context.Context, swampName name.Name) (int32, error)
	Destroy(ctx context.Context, swampName name.Name) error
	Subscribe(ctx context.Context, swampName name.Name, getExistingData bool, model any, iterator SubscribeIteratorFunc) error
	IncrementInt8(ctx context.Context, swampName name.Name, key string, value int8, condition *Int8Condition) (int8, error)
	IncrementInt16(ctx context.Context, swampName name.Name, key string, value int16, condition *Int16Condition) (int16, error)
	IncrementInt32(ctx context.Context, swampName name.Name, key string, value int32, condition *Int32Condition) (int32, error)
	IncrementInt64(ctx context.Context, swampName name.Name, key string, value int64, condition *Int64Condition) (int64, error)
	IncrementUint8(ctx context.Context, swampName name.Name, key string, value uint8, condition *Uint8Condition) (uint8, error)
	IncrementUint16(ctx context.Context, swampName name.Name, key string, value uint16, condition *Uint16Condition) (uint16, error)
	IncrementUint32(ctx context.Context, swampName name.Name, key string, value uint32, condition *Uint32Condition) (uint32, error)
	IncrementUint64(ctx context.Context, swampName name.Name, key string, value uint64, condition *Uint64Condition) (uint64, error)
	IncrementFloat32(ctx context.Context, swampName name.Name, key string, value float32, condition *Float32Condition) (float32, error)
	IncrementFloat64(ctx context.Context, swampName name.Name, key string, value float64, condition *Float64Condition) (float64, error)
	Uint32SlicePush(ctx context.Context, swampName name.Name, KeyValuesPair []*KeyValuesPair) error
	Uint32SliceDelete(ctx context.Context, swampName name.Name, KeyValuesPair []*KeyValuesPair) error
	Uint32SliceSize(ctx context.Context, swampName name.Name, key string) (int64, error)
	Uint32SliceIsValueExist(ctx context.Context, swampName name.Name, key string, value uint32) (bool, error)
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
//
// Example:
//
//	Read the latest 10 entries by creation time:
//
//	&Index{
//	    IndexType:  IndexCreationTime,
//	    IndexOrder: IndexOrderDesc,
//	    From:       0,
//	    Limit:      10,
//	}
type Index struct {
	IndexType        // What field to use for sorting/filtering
	IndexOrder       // Ascending or Descending order
	From       int32 // Offset: how many records to skip (0 = start from first)
	Limit      int32 // Max results to return (0 = return all)
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
	// Once this size is reached, a new chunk is created for further writes.
	//
	// This prevents large file rewrites, which can damage SSDs over time.
	// Smaller sizes → more files, better endurance.
	// Larger sizes → fewer files, better read performance for rarely-changed data.
	//
	// ⚠️ Always ensure MaxFileSize is larger than the filesystem block size.
	// HydrAIDE automatically compresses data, so this refers to the compressed size.
	MaxFileSize int
}

type hydraidego struct {
	client client.Client
}

func New(client client.Client) Hydraidego {
	return &hydraidego{
		client: client,
	}
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
			case codes.Unavailable:
				return false, NewError(ErrCodeConnectionError, errorMessageConnectionError)
			case codes.DeadlineExceeded:
				return false, NewError(ErrCodeCtxTimeout, errorMessageCtxTimeout)
			case codes.Canceled:
				return false, NewError(ErrCodeCtxClosedByClient, errorMessageCtxClosedByClient)
			case codes.InvalidArgument:
				return false, NewError(ErrCodeNotFound, fmt.Sprintf("%s: %v", errorMessageSwampNameNotCorrect, s.Message()))
			case codes.FailedPrecondition:
				return false, NewError(ErrCodeSwampNotFound, fmt.Sprintf("%s: %v", errorMessageSwampNotFound, s.Message()))
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

	kvPair, err := convertCatalogModelToKeyValuePair(model)
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

	kvPairs := make([]*hydraidepbgo.KeyValuePair, 0, len(models))

	for _, model := range models {
		kvPair, err := convertCatalogModelToKeyValuePair(model)
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

		kvPairs := make([]*hydraidepbgo.KeyValuePair, 0, len(req.Models))

		for _, model := range req.Models {
			kvPair, err := convertCatalogModelToKeyValuePair(model)
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

	// Fetch all matching Treasures from the Hydra engine based on the Index parameters
	response, err := h.client.GetServiceClient(swampName).GetByIndex(ctx, &hydraidepbgo.GetByIndexRequest{
		IslandID:  swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName: swampName.Get(),
		IndexType: indexTypeProtoFormat,
		OrderType: orderTypeProtoFormat,
		From:      index.From,
		Limit:     index.Limit,
	})

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
	kvPair, err := convertCatalogModelToKeyValuePair(model)
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
	kvPairs := make([]*hydraidepbgo.KeyValuePair, 0, len(models))
	for _, model := range models {
		kvPair, err := convertCatalogModelToKeyValuePair(model)
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
	kvPair, err := convertCatalogModelToKeyValuePair(model)
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
	kvPairs := make([]*hydraidepbgo.KeyValuePair, 0, len(models))
	for _, model := range models {
		kvPair, err := convertCatalogModelToKeyValuePair(model)
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
		kvPairs := make([]*hydraidepbgo.KeyValuePair, 0, len(req.Models))

		// Convert each model into a KeyValuePair
		for _, model := range req.Models {
			kvPair, err := convertCatalogModelToKeyValuePair(model)
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

		if s, ok := status.FromError(err); ok {
			switch s.Code() {
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

	kvPairs, err := convertProfileModelToKeyValuePair(model)

	if err != nil {
		return NewError(ErrCodeInvalidModel, err.Error())
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
		// Translate server-side or network error to client-side semantics
		if s, ok := status.FromError(err); ok {
			switch s.Code() {
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
		}
		return NewError(ErrCodeUnknown, fmt.Sprintf("%s: %v", errorMessageUnknown, err))
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
			if s, ok := status.FromError(err); ok {
				switch s.Code() {
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

type RelationalOperator int

const (
	NotEqual RelationalOperator = iota
	Equal
	GreaterThanOrEqual
	GreaterThan
	LessThanOrEqual
	LessThan
)

// IncrementInt8 performs an atomic int8 increment on a Treasure inside the given Swamp.
//
// If the specified Swamp or Treasure does not exist, this function will automatically create them.
// This means you do **not** need to call CatalogCreate or CatalogSave beforehand.
//
// If a condition is provided, the increment will only occur if the current value
// satisfies the given relational constraint — evaluated **atomically on the server**.
//
// 🧠 This is the Int8 version of HydrAIDE’s type-safe increment operation.
//
//	The same logic applies for other numeric types (Int16, Int32, Float64, etc.),
//	and this function can be duplicated/adapted accordingly.
//
// Parameters:
//   - ctx:        context for cancellation and timeout
//   - swampName:  target Swamp where the Treasure lives
//   - key:        unique key of the Treasure to increment
//   - value:      the delta (positive or negative) to add
//   - condition:  optional constraint on the current value before incrementing
//
// Returns:
//   - new int8 value after increment (if successful)
//   - error if the operation failed, or if the condition was not met
//
// ⚠️ Notes:
//   - If the Treasure exists but holds a value of a **different type** (e.g., float64), this operation will fail.
//   - Decrementing is supported by simply passing a **negative delta value**.
//   - All proto values are transmitted as int32, but converted back to int8 here.
//   - If the condition is not met, the function returns a specific ErrConditionNotMet error.
//
// ✅ Conditional Logic:
//
//   - The `condition` lets you define rules for when an increment should occur.
//
//   - It uses a `RelationalOperator` enum to compare the **current value** against a reference.
//
//     Supported operators include:
//
//   - Equal (==)
//
//   - NotEqual (!=)
//
//   - GreaterThan (>)
//
//   - GreaterThanOrEqual (>=)
//
//   - LessThan (<)
//
//   - LessThanOrEqual (<=)
//
//     Example:
//     Only increment if the current value is greater than 10:
//     &Int8Condition{RelationalOperator: GreaterThan, Value: 10}
//
// ✅ Example usage:
//
//	IncrementInt8(ctx, "scoreboard", "user:42", 1, &Int8Condition{GreaterThan, 0})
func (h *hydraidego) IncrementInt8(ctx context.Context, swampName name.Name, key string, value int8, condition *Int8Condition) (int8, error) {

	r := &hydraidepbgo.IncrementInt8Request{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		Key:         key,
		IncrementBy: int32(value),
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
		return 0, errorHandler(err)
	}

	// return with the new value if the increment was successful
	if response.GetIsIncremented() {
		return int8(response.GetValue()), nil
	}

	return 0, NewError(ErrConditionNotMet, fmt.Sprintf("%s: %d", errorMessageConditionNotMet, response.GetValue()))

}

// IncrementInt16 performs an atomic int16 increment on a Treasure inside the given Swamp.
//
// If the specified Swamp or Treasure does not exist, this function will automatically create them.
// This means you do **not** need to call CatalogCreate or CatalogSave beforehand.
//
// If a condition is provided, the increment will only occur if the current value
// satisfies the given relational constraint — evaluated **atomically on the server**.
//
// 🧠 This is the Int16 version of HydrAIDE’s type-safe increment operation.
//
//	The same logic applies for other numeric types (Int8, Int32, Uint64, Float64, etc.),
//	and this function can be duplicated/adapted accordingly.
//
// Parameters:
//   - ctx:        context for cancellation and timeout
//   - swampName:  target Swamp where the Treasure lives
//   - key:        unique key of the Treasure to increment
//   - value:      the delta (positive or negative) to add
//   - condition:  optional constraint on the current value before incrementing
//
// Returns:
//   - new int16 value after increment (if successful)
//   - error if the operation failed, or if the condition was not met
//
// ⚠️ Notes:
//   - If the Treasure exists but holds a value of a **different type** (e.g., int32), this operation will fail.
//   - Decrementing is supported by simply passing a **negative delta value**.
//   - All proto values are transmitted as int32, but converted back to int16 here.
//   - If the condition is not met, the function returns a specific ErrConditionNotMet error.
//
// ✅ Conditional Logic:
//
//   - The `condition` lets you define rules for when an increment should occur.
//
//   - It uses a `RelationalOperator` enum to compare the **current value** against a reference.
//
//     Supported operators include:
//
//   - Equal (==)
//
//   - NotEqual (!=)
//
//   - GreaterThan (>)
//
//   - GreaterThanOrEqual (>=)
//
//   - LessThan (<)
//
//   - LessThanOrEqual (<=)
//
//     Example:
//     Only increment if the current value is less than or equal to 500:
//     &Int16Condition{RelationalOperator: LessThanOrEqual, Value: 500}
//
// ✅ Example usage:
//
//	IncrementInt16(ctx, "metrics", "api:retry-count", 1, &Int16Condition{GreaterThan, 0})
func (h *hydraidego) IncrementInt16(ctx context.Context, swampName name.Name, key string, value int16, condition *Int16Condition) (int16, error) {

	r := &hydraidepbgo.IncrementInt16Request{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		Key:         key,
		IncrementBy: int32(value),
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
		return 0, errorHandler(err)
	}

	// return with the new value if the increment was successful
	if response.GetIsIncremented() {
		return int16(response.GetValue()), nil
	}

	return 0, NewError(ErrConditionNotMet, fmt.Sprintf("%s: %d", errorMessageConditionNotMet, response.GetValue()))

}

// IncrementInt32 performs an atomic int32 increment on a Treasure inside the given Swamp.
//
// If the specified Swamp or Treasure does not exist, this function will automatically create them.
// This means you do **not** need to call CatalogCreate or CatalogSave beforehand.
//
// If a condition is provided, the increment will only occur if the current value
// satisfies the given relational constraint — evaluated **atomically on the server**.
//
// 🧠 This is the Int32 version of HydrAIDE’s type-safe increment operation.
//
//	The same logic applies for other numeric types (Int8, Int16, Uint64, Float64, etc.),
//	and this function can be duplicated/adapted accordingly.
//
// Parameters:
//   - ctx:        context for cancellation and timeout
//   - swampName:  target Swamp where the Treasure lives
//   - key:        unique key of the Treasure to increment
//   - value:      the delta (positive or negative) to add
//   - condition:  optional constraint on the current value before incrementing
//
// Returns:
//   - new int32 value after increment (if successful)
//   - error if the operation failed, or if the condition was not met
//
// ⚠️ Notes:
//   - If the Treasure exists but holds a value of a **different type** (e.g., float64), this operation will fail.
//   - Decrementing is supported by simply passing a **negative delta value**.
//   - All proto values are transmitted as int32, and this function also returns an int32 directly.
//   - If the condition is not met, the function returns a specific ErrConditionNotMet error.
//
// ✅ Conditional Logic:
//
//   - The `condition` lets you define rules for when an increment should occur.
//
//   - It uses a `RelationalOperator` enum to compare the **current value** against a reference.
//
//     Supported operators include:
//
//   - Equal (==)
//
//   - NotEqual (!=)
//
//   - GreaterThan (>)
//
//   - GreaterThanOrEqual (>=)
//
//   - LessThan (<)
//
//   - LessThanOrEqual (<=)
//
//     Example:
//     Only increment if the current value is exactly 100:
//     &Int32Condition{RelationalOperator: Equal, Value: 100}
//
// ✅ Example usage:
//
//	IncrementInt32(ctx, "user-stats", "user:1234:logins", 1, &Int32Condition{GreaterThanOrEqual, 0})
func (h *hydraidego) IncrementInt32(ctx context.Context, swampName name.Name, key string, value int32, condition *Int32Condition) (int32, error) {

	r := &hydraidepbgo.IncrementInt32Request{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		Key:         key,
		IncrementBy: value,
	}

	if condition != nil {
		r.Condition = &hydraidepbgo.IncrementInt32Condition{
			RelationalOperator: convertRelationalOperatorToProtoOperator(condition.RelationalOperator),
			Value:              condition.Value,
		}
	}

	response, err := h.client.GetServiceClient(swampName).IncrementInt32(ctx, r)

	if err != nil {
		return 0, errorHandler(err)
	}

	// return with the new value if the increment was successful
	if response.GetIsIncremented() {
		return response.GetValue(), nil
	}

	return 0, NewError(ErrConditionNotMet, fmt.Sprintf("%s: %d", errorMessageConditionNotMet, response.GetValue()))

}

// IncrementInt64 performs an atomic int64 increment on a Treasure inside the given Swamp.
//
// If the specified Swamp or Treasure does not exist, this function will automatically create them.
// This means you do **not** need to call CatalogCreate or CatalogSave beforehand.
//
// If a condition is provided, the increment will only occur if the current value
// satisfies the given relational constraint — evaluated **atomically on the server**.
//
// 🧠 This is the Int64 version of HydrAIDE’s type-safe increment operation.
//
//	The same logic applies for other numeric types (Int8, Int16, Int32, Uint64, Float64, etc.),
//	and this function can be duplicated/adapted accordingly.
//
// Parameters:
//   - ctx:        context for cancellation and timeout
//   - swampName:  target Swamp where the Treasure lives
//   - key:        unique key of the Treasure to increment
//   - value:      the delta (positive or negative) to add
//   - condition:  optional constraint on the current value before incrementing
//
// Returns:
//   - new int64 value after increment (if successful)
//   - error if the operation failed, or if the condition was not met
//
// ⚠️ Notes:
//   - If the Treasure exists but holds a value of a **different type** (e.g., int32 or float64), this operation will fail.
//   - Decrementing is supported by simply passing a **negative delta value**.
//   - All proto values are transmitted as int64 and returned as int64.
//   - If the condition is not met, the function returns a specific ErrConditionNotMet error.
//
// ✅ Conditional Logic:
//
//   - The `condition` lets you define rules for when an increment should occur.
//
//   - It uses a `RelationalOperator` enum to compare the **current value** against a reference.
//
//     Supported operators include:
//
//   - Equal (==)
//
//   - NotEqual (!=)
//
//   - GreaterThan (>)
//
//   - GreaterThanOrEqual (>=)
//
//   - LessThan (<)
//
//   - LessThanOrEqual (<=)
//
//     Example:
//     Only increment if the current value is greater than or equal to 10,000:
//     &Int64Condition{RelationalOperator: GreaterThanOrEqual, Value: 10000}
//
// ✅ Example usage:
//
//	IncrementInt64(ctx, "finance", "user:987:balance", 500, &Int64Condition{LessThan, 100000})
func (h *hydraidego) IncrementInt64(ctx context.Context, swampName name.Name, key string, value int64, condition *Int64Condition) (int64, error) {

	r := &hydraidepbgo.IncrementInt64Request{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		Key:         key,
		IncrementBy: value,
	}

	if condition != nil {
		r.Condition = &hydraidepbgo.IncrementInt64Condition{
			RelationalOperator: convertRelationalOperatorToProtoOperator(condition.RelationalOperator),
			Value:              condition.Value,
		}
	}

	response, err := h.client.GetServiceClient(swampName).IncrementInt64(ctx, r)

	if err != nil {
		return 0, errorHandler(err)
	}

	// return with the new value if the increment was successful
	if response.GetIsIncremented() {
		return response.GetValue(), nil
	}

	return 0, NewError(ErrConditionNotMet, fmt.Sprintf("%s: %d", errorMessageConditionNotMet, response.GetValue()))

}

// IncrementUint8 performs an atomic uint8 increment on a Treasure inside the given Swamp.
//
// If the specified Swamp or Treasure does not exist, this function will automatically create them.
// This means you do **not** need to call CatalogCreate or CatalogSave beforehand.
//
// If a condition is provided, the increment will only occur if the current value
// satisfies the given relational constraint — evaluated **atomically on the server**.
//
// 🧠 This is the Uint8 version of HydrAIDE’s type-safe increment operation.
//
//	The same logic applies for other numeric types (Int16, Uint32, Float64, etc.),
//	and this function can be duplicated/adapted accordingly.
//
// Parameters:
//   - ctx:        context for cancellation and timeout
//   - swampName:  target Swamp where the Treasure lives
//   - key:        unique key of the Treasure to increment
//   - value:      the delta (positive or negative) to add (note: negative values will underflow)
//   - condition:  optional constraint on the current value before incrementing
//
// Returns:
//   - new uint8 value after increment (if successful)
//   - error if the operation failed, or if the condition was not met
//
// ⚠️ Notes:
//   - If the Treasure exists but holds a value of a **different type** (e.g., int64 or float32), this operation will fail.
//   - Since this is an unsigned type, **negative delta values are not allowed** (and may cause underflow).
//   - All proto values are transmitted as uint32, and this function converts them to uint8.
//   - If the condition is not met, the function returns a specific ErrConditionNotMet error.
//
// ✅ Conditional Logic:
//
//   - The `condition` lets you define rules for when an increment should occur.
//
//   - It uses a `RelationalOperator` enum to compare the **current value** against a reference.
//
//     Supported operators include:
//
//   - Equal (==)
//
//   - NotEqual (!=)
//
//   - GreaterThan (>)
//
//   - GreaterThanOrEqual (>=)
//
//   - LessThan (<)
//
//   - LessThanOrEqual (<=)
//
//     Example:
//     Only increment if the current value is less than 255:
//     &Uint8Condition{RelationalOperator: LessThan, Value: 255}
//
// ✅ Example usage:
//
//	IncrementUint8(ctx, "badge-points", "user:100:stars", 1, &Uint8Condition{GreaterThanOrEqual, 0})
func (h *hydraidego) IncrementUint8(ctx context.Context, swampName name.Name, key string, value uint8, condition *Uint8Condition) (uint8, error) {
	r := &hydraidepbgo.IncrementUint8Request{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		Key:         key,
		IncrementBy: uint32(value),
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
		return 0, errorHandler(err)
	}

	if response.GetIsIncremented() {
		return uint8(response.GetValue()), nil
	}

	return 0, NewError(ErrConditionNotMet, fmt.Sprintf("%s: %d", errorMessageConditionNotMet, response.GetValue()))
}

// IncrementUint16 performs an atomic uint16 increment on a Treasure inside the given Swamp.
//
// If the specified Swamp or Treasure does not exist, this function will automatically create them.
// This means you do **not** need to call CatalogCreate or CatalogSave beforehand.
//
// If a condition is provided, the increment will only occur if the current value
// satisfies the given relational constraint — evaluated **atomically on the server**.
//
// 🧠 This is the Uint16 version of HydrAIDE’s type-safe increment operation.
//
//	The same logic applies for other numeric types (Uint8, Uint32, Float64, etc.),
//	and this function can be duplicated/adapted accordingly.
//
// Parameters:
//   - ctx:        context for cancellation and timeout
//   - swampName:  target Swamp where the Treasure lives
//   - key:        unique key of the Treasure to increment
//   - value:      the delta (positive value to add — negative values are not supported)
//   - condition:  optional constraint on the current value before incrementing
//
// Returns:
//   - new uint16 value after increment (if successful)
//   - error if the operation failed, or if the condition was not met
//
// ⚠️ Notes:
//   - If the Treasure exists but holds a value of a **different type** (e.g., int32 or float64), this operation will fail.
//   - Since this is an unsigned type, **negative delta values are not allowed** (and may cause underflow).
//   - All proto values are transmitted as uint32, and this function converts them to uint16.
//   - If the condition is not met, the function returns a specific ErrConditionNotMet error.
//
// ✅ Conditional Logic:
//
//   - The `condition` lets you define rules for when an increment should occur.
//
//   - It uses a `RelationalOperator` enum to compare the **current value** against a reference.
//
//     Supported operators include:
//
//   - Equal (==)
//
//   - NotEqual (!=)
//
//   - GreaterThan (>)
//
//   - GreaterThanOrEqual (>=)
//
//   - LessThan (<)
//
//   - LessThanOrEqual (<=)
//
//     Example:
//     Only increment if the current value is less than 10000:
//     &Uint16Condition{RelationalOperator: LessThan, Value: 10000}
//
// ✅ Example usage:
//
//	IncrementUint16(ctx, "api-quota", "user:42:limit", 250, &Uint16Condition{GreaterThanOrEqual, 100})
func (h *hydraidego) IncrementUint16(ctx context.Context, swampName name.Name, key string, value uint16, condition *Uint16Condition) (uint16, error) {
	r := &hydraidepbgo.IncrementUint16Request{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		Key:         key,
		IncrementBy: uint32(value),
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
		return 0, errorHandler(err)
	}

	if response.GetIsIncremented() {
		return uint16(response.GetValue()), nil
	}

	return 0, NewError(ErrConditionNotMet, fmt.Sprintf("%s: %d", errorMessageConditionNotMet, response.GetValue()))
}

// IncrementUint32 performs an atomic uint32 increment on a Treasure inside the given Swamp.
//
// If the specified Swamp or Treasure does not exist, this function will automatically create them.
// This means you do **not** need to call CatalogCreate or CatalogSave beforehand.
//
// If a condition is provided, the increment will only occur if the current value
// satisfies the given relational constraint — evaluated **atomically on the server**.
//
// 🧠 This is the Uint32 version of HydrAIDE’s type-safe increment operation.
//
//	The same logic applies for other numeric types (Uint8, Uint16, Float64, etc.),
//	and this function can be duplicated/adapted accordingly.
//
// Parameters:
//   - ctx:        context for cancellation and timeout
//   - swampName:  target Swamp where the Treasure lives
//   - key:        unique key of the Treasure to increment
//   - value:      the delta (positive value to add — negative values are not supported)
//   - condition:  optional constraint on the current value before incrementing
//
// Returns:
//   - new uint32 value after increment (if successful)
//   - error if the operation failed, or if the condition was not met
//
// ⚠️ Notes:
//   - If the Treasure exists but holds a value of a **different type** (e.g., int64 or float32), this operation will fail.
//   - Since this is an unsigned type, **negative delta values are not allowed** (and may cause underflow).
//   - All proto values are transmitted and returned as uint32 — no conversion is required in this case.
//   - If the condition is not met, the function returns a specific ErrConditionNotMet error.
//
// ✅ Conditional Logic:
//
//   - The `condition` lets you define rules for when an increment should occur.
//
//   - It uses a `RelationalOperator` enum to compare the **current value** against a reference.
//
//     Supported operators include:
//
//   - Equal (==)
//
//   - NotEqual (!=)
//
//   - GreaterThan (>)
//
//   - GreaterThanOrEqual (>=)
//
//   - LessThan (<)
//
//   - LessThanOrEqual (<=)
//
//     Example:
//     Only increment if the current value is greater than 1,000,000:
//     &Uint32Condition{RelationalOperator: GreaterThan, Value: 1_000_000}
//
// ✅ Example usage:
//
//	IncrementUint32(ctx, "metrics", "user:42:pageviews", 100, &Uint32Condition{LessThanOrEqual, 5_000_000})
func (h *hydraidego) IncrementUint32(ctx context.Context, swampName name.Name, key string, value uint32, condition *Uint32Condition) (uint32, error) {
	r := &hydraidepbgo.IncrementUint32Request{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		Key:         key,
		IncrementBy: value,
	}

	if condition != nil {
		r.Condition = &hydraidepbgo.IncrementUint32Condition{
			RelationalOperator: convertRelationalOperatorToProtoOperator(condition.RelationalOperator),
			Value:              condition.Value,
		}
	}

	response, err := h.client.GetServiceClient(swampName).IncrementUint32(ctx, r)
	if err != nil {
		return 0, errorHandler(err)
	}

	if response.GetIsIncremented() {
		return response.GetValue(), nil
	}

	return 0, NewError(ErrConditionNotMet, fmt.Sprintf("%s: %d", errorMessageConditionNotMet, response.GetValue()))
}

// IncrementUint64 performs an atomic uint64 increment on a Treasure inside the given Swamp.
//
// If the specified Swamp or Treasure does not exist, this function will automatically create them.
// This means you do **not** need to call CatalogCreate or CatalogSave beforehand.
//
// If a condition is provided, the increment will only occur if the current value
// satisfies the given relational constraint — evaluated **atomically on the server**.
//
// 🧠 This is the Uint64 version of HydrAIDE’s type-safe increment operation.
//
//	The same logic applies for other numeric types (Uint8, Uint32, Float64, etc.),
//	and this function can be duplicated/adapted accordingly.
//
// Parameters:
//   - ctx:        context for cancellation and timeout
//   - swampName:  target Swamp where the Treasure lives
//   - key:        unique key of the Treasure to increment
//   - value:      the delta (positive value to add — negative values are not supported)
//   - condition:  optional constraint on the current value before incrementing
//
// Returns:
//   - new uint64 value after increment (if successful)
//   - error if the operation failed, or if the condition was not met
//
// ⚠️ Notes:
//   - If the Treasure exists but holds a value of a **different type** (e.g., int32 or float64), this operation will fail.
//   - Since this is an unsigned type, **negative delta values are not allowed** (and may cause underflow).
//   - All proto values are transmitted and returned as uint64 — no conversion is required in this case.
//   - If the condition is not met, the function returns a specific ErrConditionNotMet error.
//
// ✅ Conditional Logic:
//
//   - The `condition` lets you define rules for when an increment should occur.
//
//   - It uses a `RelationalOperator` enum to compare the **current value** against a reference.
//
//     Supported operators include:
//
//   - Equal (==)
//
//   - NotEqual (!=)
//
//   - GreaterThan (>)
//
//   - GreaterThanOrEqual (>=)
//
//   - LessThan (<)
//
//   - LessThanOrEqual (<=)
//
//     Example:
//     Only increment if the current value is less than 1 billion:
//     &Uint64Condition{RelationalOperator: LessThan, Value: 1_000_000_000}
//
// ✅ Example usage:
//
//	IncrementUint64(ctx, "billing", "user:abc:total-bytes-used", 1_000_000, &Uint64Condition{LessThanOrEqual, 5_000_000_000})
func (h *hydraidego) IncrementUint64(ctx context.Context, swampName name.Name, key string, value uint64, condition *Uint64Condition) (uint64, error) {
	r := &hydraidepbgo.IncrementUint64Request{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		Key:         key,
		IncrementBy: value,
	}

	if condition != nil {
		r.Condition = &hydraidepbgo.IncrementUint64Condition{
			RelationalOperator: convertRelationalOperatorToProtoOperator(condition.RelationalOperator),
			Value:              condition.Value,
		}
	}

	response, err := h.client.GetServiceClient(swampName).IncrementUint64(ctx, r)
	if err != nil {
		return 0, errorHandler(err)
	}

	if response.GetIsIncremented() {
		return response.GetValue(), nil
	}

	return 0, NewError(ErrConditionNotMet, fmt.Sprintf("%s: %d", errorMessageConditionNotMet, response.GetValue()))
}

// IncrementFloat32 performs an atomic float32 increment on a Treasure inside the given Swamp.
//
// If the specified Swamp or Treasure does not exist, this function will automatically create them.
// This means you do **not** need to call CatalogCreate or CatalogSave beforehand.
//
// If a condition is provided, the increment will only occur if the current value
// satisfies the given relational constraint — evaluated **atomically on the server**.
//
// 🧠 This is the Float32 version of HydrAIDE’s type-safe increment operation.
//
//	The same logic applies for other numeric types (Int32, Float64, etc.), and this function can be
//	duplicated/adapted accordingly.
//
// Parameters:
//   - ctx:        context for cancellation and timeout
//   - swampName:  target Swamp where the Treasure lives
//   - key:        unique key of the Treasure to increment
//   - value:      the amount to increment by (can be negative to decrement)
//   - condition:  optional constraint on the current value before incrementing
//
// Returns:
//   - new float32 value after increment (if successful)
//   - error if the operation failed, or if the condition was not met
//
// ⚠️ Notes:
//   - If the Treasure exists but holds a value of a **different type** (e.g., int64), this operation will fail.
//   - Decrementing is supported by simply passing a **negative delta value**.
//   - All proto values are transmitted as float32 and returned as float32.
//   - Floating-point equality comparisons (`==`, `!=`) may be affected by precision limits — use with care.
//   - If the condition is not met, the function returns a specific ErrConditionNotMet error.
//
// ✅ Conditional Logic:
//
//   - The `condition` lets you define rules for when an increment should occur.
//
//   - It uses a `RelationalOperator` enum to compare the **current value** against a reference.
//
//     Supported operators include:
//
//   - Equal (==)
//
//   - NotEqual (!=)
//
//   - GreaterThan (>)
//
//   - GreaterThanOrEqual (>=)
//
//   - LessThan (<)
//
//   - LessThanOrEqual (<=)
//
//     Example:
//     Only increment if the current value is less than 99.9:
//     &Float32Condition{RelationalOperator: LessThan, Value: 99.9}
//
// ✅ Example usage:
//
//	IncrementFloat32(ctx, "analytics", "user:session-duration", 2.5, &Float32Condition{GreaterThan, 0})
func (h *hydraidego) IncrementFloat32(ctx context.Context, swampName name.Name, key string, value float32, condition *Float32Condition) (float32, error) {
	r := &hydraidepbgo.IncrementFloat32Request{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		Key:         key,
		IncrementBy: value,
	}

	if condition != nil {
		r.Condition = &hydraidepbgo.IncrementFloat32Condition{
			RelationalOperator: convertRelationalOperatorToProtoOperator(condition.RelationalOperator),
			Value:              condition.Value,
		}
	}

	response, err := h.client.GetServiceClient(swampName).IncrementFloat32(ctx, r)
	if err != nil {
		return 0, errorHandler(err)
	}

	if response.GetIsIncremented() {
		return response.GetValue(), nil
	}

	return 0, NewError(ErrConditionNotMet, fmt.Sprintf("%s: %f", errorMessageConditionNotMet, response.GetValue()))
}

// IncrementFloat64 performs an atomic float64 increment on a Treasure inside the given Swamp.
//
// If the specified Swamp or Treasure does not exist, this function will automatically create them.
// This means you do **not** need to call CatalogCreate or CatalogSave beforehand.
//
// If a condition is provided, the increment will only occur if the current value
// satisfies the given relational constraint — evaluated **atomically on the server**.
//
// 🧠 This is the Float64 version of HydrAIDE’s type-safe increment operation.
//
//	The same logic applies for other numeric types (Int64, Float32, etc.), and this function can be
//	duplicated/adapted accordingly.
//
// Parameters:
//   - ctx:        context for cancellation and timeout
//   - swampName:  target Swamp where the Treasure lives
//   - key:        unique key of the Treasure to increment
//   - value:      the amount to increment by (can be negative to decrement)
//   - condition:  optional constraint on the current value before incrementing
//
// Returns:
//   - new float64 value after increment (if successful)
//   - error if the operation failed, or if the condition was not met
//
// ⚠️ Notes:
//   - If the Treasure exists but holds a value of a **different type** (e.g., int64), this operation will fail.
//   - Decrementing is supported by simply passing a **negative delta value**.
//   - All proto values are transmitted and returned as float64 — no conversion needed.
//   - Floating-point equality comparisons (`==`, `!=`) may be affected by precision limits — consider using tolerances.
//   - If the condition is not met, the function returns a specific ErrConditionNotMet error.
//
// ✅ Conditional Logic:
//
//   - The `condition` lets you define rules for when an increment should occur.
//
//   - It uses a `RelationalOperator` enum to compare the **current value** against a reference.
//
//     Supported operators include:
//
//   - Equal (==)
//
//   - NotEqual (!=)
//
//   - GreaterThan (>)
//
//   - GreaterThanOrEqual (>=)
//
//   - LessThan (<)
//
//   - LessThanOrEqual (<=)
//
//     Example:
//     Only increment if the current value is greater than or equal to 1000.0:
//     &Float64Condition{RelationalOperator: GreaterThanOrEqual, Value: 1000.0}
//
// ✅ Example usage:
//
//	IncrementFloat64(ctx, "finance", "user:abc:wallet-balance", 49.95, &Float64Condition{LessThan, 10_000.0})
func (h *hydraidego) IncrementFloat64(ctx context.Context, swampName name.Name, key string, value float64, condition *Float64Condition) (float64, error) {
	r := &hydraidepbgo.IncrementFloat64Request{
		IslandID:    swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:   swampName.Get(),
		Key:         key,
		IncrementBy: value,
	}

	if condition != nil {
		r.Condition = &hydraidepbgo.IncrementFloat64Condition{
			RelationalOperator: convertRelationalOperatorToProtoOperator(condition.RelationalOperator),
			Value:              condition.Value,
		}
	}

	response, err := h.client.GetServiceClient(swampName).IncrementFloat64(ctx, r)
	if err != nil {
		return 0, errorHandler(err)
	}

	if response.GetIsIncremented() {
		return response.GetValue(), nil
	}

	return 0, NewError(ErrConditionNotMet, fmt.Sprintf("%s: %f", errorMessageConditionNotMet, response.GetValue()))
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

	keySlices := make([]*hydraidepbgo.KeySlicePair, len(KeyValuesPair))

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

	keySlices := make([]*hydraidepbgo.KeySlicePair, len(KeyValuesPair))

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
// - Slices and maps (serialized as GOB-encoded binary blobs)
// - Structs and pointers (also GOB-encoded)
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
func convertCatalogModelToKeyValuePair(model any) (*hydraidepbgo.KeyValuePair, error) {

	// Get the reflection value of the input model
	v := reflect.ValueOf(model)

	// 🧪 Validate the input: it must be a pointer to a struct.
	// This is required because we'll be using reflection to iterate over the fields
	// and extract tags and values dynamically. Non-pointer or non-struct inputs are invalid.
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return nil, errors.New("input must be a pointer to a struct")
	}

	// This flag tracks whether any value has been set.
	// If no value is provided (only key or metadata), we'll later set VoidVal = true.
	valueVoid := true

	// Initialize the KeyValuePair that will hold the final encoded output
	kvPair := &hydraidepbgo.KeyValuePair{}

	// Get the actual struct (dereferenced value) and its type
	v = v.Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {

		field := t.Field(i)

		// Check if the field has a `hydraide:"omitempty"` tag,
		// and skip it if the value is considered "empty" (zero, nil, blank, etc.)
		if tag, ok := field.Tag.Lookup(tagHydrAIDE); ok && tag == tagOmitempty {

			value := v.Field(i)
			if isFieldEmpty(value) {
				continue
			}

		}

		// Check if the current field is marked as the `key` field (via `hydraide:"key"` tag)
		if key, ok := field.Tag.Lookup(tagHydrAIDE); ok && key == tagKey {

			value := v.Field(i)

			// Validate that the field is a non-empty string — required for all HydrAIDE Treasures.
			// Keys must always be explicit and unique within a Swamp.
			if value.Kind() == reflect.String && value.String() != "" {
				// Found the key — assign it to the KeyValuePair
				kvPair.Key = value.String()
				valueVoid = false
				continue
			}

			// If the key field is missing or empty, this is an invalid model
			return nil, errors.New("key field must be a non-empty string")
		}

		// Check if the current field is tagged as the `value` field (via `hydraide:"value"`)
		// This field holds the actual value of the Treasure.
		// We detect its type using reflection and populate the corresponding proto field in KeyValuePair.
		if key, ok := field.Tag.Lookup(tagHydrAIDE); ok && key == tagValue {

			value := v.Field(i)

			// convert the value to KeyValuePair
			if err := convertFieldToKvPair(value, kvPair); err != nil {
				return nil, err
			}

		}

		// Process the `expireAt` field (tagged with `hydraide:"expireAt"`).
		// This defines the logical expiration time of the Treasure.
		// Once the given timestamp is reached, HydrAIDE will treat the record as expired.
		// - Must be of type `time.Time`
		// - Must not be the zero time
		// - Automatically converted to a `timestamppb.Timestamp` for protobuf
		if key, ok := field.Tag.Lookup(tagHydrAIDE); ok && key == tagExpireAt {
			value := v.Field(i)
			if value.Kind() != reflect.Struct || value.Type() != reflect.TypeOf(time.Time{}) {
				return nil, errors.New("expireAt field must be a time.Time")
			}
			expireAt := value.Interface().(time.Time).UTC()
			if expireAt.IsZero() {
				return nil, errors.New("expireAt field must be a non-zero time.Time")
			}
			kvPair.ExpiredAt = timestamppb.New(expireAt)
			valueVoid = false
			continue
		}

		// Process the `createdBy` field (tagged with `hydraide:"createdBy"`).
		// Optional metadata indicating who or what created the Treasure.
		// - Must be of type `string`
		// - Empty values are ignored
		if key, ok := field.Tag.Lookup(tagHydrAIDE); ok && key == tagCreatedBy {
			value := v.Field(i)
			if value.Kind() != reflect.String {
				return nil, errors.New("createdBy field must be a string")
			}
			if value.String() != "" {
				createdBy := value.String()
				kvPair.CreatedBy = &createdBy
				valueVoid = false
			}
			continue
		}

		// Process the `createdAt` field (tagged with `hydraide:"createdAt"`).
		// Optional metadata representing when the Treasure was created.
		// - Must be of type `time.Time`
		// - Must not be zero
		// - Converted to protobuf-compatible timestamp
		if key, ok := field.Tag.Lookup(tagHydrAIDE); ok && key == tagCreatedAt {
			value := v.Field(i)
			if value.Kind() != reflect.Struct || value.Type() != reflect.TypeOf(time.Time{}) {
				return nil, errors.New("createdAt field must be a time.Time")
			}
			createdAt := value.Interface().(time.Time).UTC()
			if createdAt.IsZero() {
				return nil, errors.New("createdAt field must be a non-zero time.Time")
			}
			kvPair.CreatedAt = timestamppb.New(createdAt)
			valueVoid = false
			continue
		}

		// Process the `updatedBy` field (tagged with `hydraide:"updatedBy"`).
		// Optional metadata indicating who or what last updated the Treasure.
		// - Must be of type `string`
		// - Ignored if empty
		if key, ok := field.Tag.Lookup(tagHydrAIDE); ok && key == tagUpdatedBy {
			value := v.Field(i)
			if value.Kind() != reflect.String {
				return nil, errors.New("updatedBy field must be a string")
			}
			if value.String() != "" {
				updatedBy := value.String()
				kvPair.UpdatedBy = &updatedBy
				valueVoid = false
			}
			continue
		}

		// Process the `updatedAt` field (tagged with `hydraide:"updatedAt"`).
		// Optional metadata representing the last modification time of the Treasure.
		// - Must be of type `time.Time`
		// - Must be non-zero
		// - Automatically converted to a `timestamppb.Timestamp` for protobuf transmission
		if key, ok := field.Tag.Lookup(tagHydrAIDE); ok && key == tagUpdatedAt {
			value := v.Field(i)
			if value.Kind() != reflect.Struct || value.Type() != reflect.TypeOf(time.Time{}) {
				return nil, errors.New("updatedAt field must be a time.Time")
			}
			updatedAt := value.Interface().(time.Time).UTC()
			if updatedAt.IsZero() {
				return nil, errors.New("updatedAt field must be a non-zero time.Time")
			}
			kvPair.UpdatedAt = timestamppb.New(updatedAt)
			valueVoid = false
			continue
		}

	}

	// Final validation: the key must be present and non-empty.
	// This is a hard requirement — all Treasures in HydrAIDE must have a key.
	if kvPair.Key == "" {
		return nil, errors.New("key field not found")
	}

	// If no value was set during processing, mark the KeyValuePair as void.
	// This tells HydrAIDE that the record has no explicit value (e.g. it's a flag, or purely metadata).
	if valueVoid {
		kvPair.VoidVal = &valueVoid
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

		if key, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && key == tagKey {
			v.Elem().Field(i).SetString(treasure.GetKey())
			continue
		}

		if key, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && key == tagValue {

			field := v.Elem().Field(i)

			// set proto treasure to model
			if err := setProtoTreasureToModel(treasure, field); err != nil {
				return err
			}

			continue

		}

		if key, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && key == tagExpireAt {
			if treasure.ExpiredAt != nil {
				v.Elem().Field(i).Set(reflect.ValueOf(treasure.ExpiredAt.AsTime()))
			}
			continue
		}

		if key, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && key == tagCreatedBy {
			if treasure.CreatedBy != nil {
				v.Elem().Field(i).SetString(*treasure.CreatedBy)
			}
			continue
		}

		if key, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && key == tagCreatedAt {
			if treasure.CreatedAt != nil {
				v.Elem().Field(i).Set(reflect.ValueOf(treasure.CreatedAt.AsTime()))
			}
			continue
		}

		if key, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && key == tagUpdatedBy {
			if treasure.UpdatedBy != nil {
				v.Elem().Field(i).SetString(*treasure.UpdatedBy)
			}
			continue
		}

		if key, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && key == tagUpdatedAt {
			if treasure.UpdatedAt != nil {
				v.Elem().Field(i).Set(reflect.ValueOf(treasure.UpdatedAt.AsTime()))
			}
			continue
		}

	}

	return nil

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
				field.SetBytes(treasure.GetBytesVal())
			} else {

				decoder := gob.NewDecoder(bytes.NewReader(treasure.GetBytesVal()))
				decoded := reflect.New(field.Type()).Interface()

				if err := decoder.Decode(decoded); err != nil {
					return fmt.Errorf("failed to decode gob into slice field: %w", err)
				}

				field.Set(reflect.ValueOf(decoded).Elem())
			}

		case reflect.Map, reflect.Ptr:

			decoder := gob.NewDecoder(bytes.NewReader(treasure.GetBytesVal()))
			decoded := reflect.New(field.Type()).Interface()

			if err := decoder.Decode(decoded); err != nil {
				return fmt.Errorf("failed to decode gob into map/ptr field: %w", err)
			}

			field.Set(reflect.ValueOf(decoded).Elem())

		default:
			return nil
		}
	}

	return nil

}

// convertComplexModelToKeyValuePair convert a complex model to a key value pair
func convertProfileModelToKeyValuePair(model any) ([]*hydraidepbgo.KeyValuePair, error) {

	// check if the model is not a pointer
	v := reflect.ValueOf(model)

	// ellenőrizzük, hogy a model egy pointer-e és egy struct-e
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return nil, errors.New("input must be a pointer to a struct")
	}

	var kvPairs []*hydraidepbgo.KeyValuePair

	v = v.Elem()
	t := v.Type()

	// ellenőrizzük és kiszedjük a szükséges mezőket és azok értékeit
	for i := 0; i < t.NumField(); i++ {

		field := t.Field(i)

		// Skip fields with "omitempty" if they are empty or nil
		if tag, ok := field.Tag.Lookup(tagHydrAIDE); ok && tag == tagOmitempty {
			value := v.Field(i)
			if isFieldEmpty(value) {
				continue
			}
		}

		kvPair := &hydraidepbgo.KeyValuePair{
			Key: field.Name,
		}

		// ellenőrizzük, hogy mi a mező típusa és annak megfelelően beállítjuk a value-t
		value := v.Field(i)

		// convert to KeyValuePair the value
		if err := convertFieldToKvPair(value, kvPair); err != nil {
			return nil, err
		}

		kvPairs = append(kvPairs, kvPair)

	}

	// process the value field
	return kvPairs, nil

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

// convert one field to a key value pair
func convertFieldToKvPair(value reflect.Value, kvPair *hydraidepbgo.KeyValuePair) (err error) {

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
	case reflect.Int, reflect.Int64:
		intVal := value.Int()
		kvPair.Int64Val = &intVal
	// 🔬 Floating point numbers
	case reflect.Float32:
		floatVal := float32(value.Float())
		kvPair.Float32Val = &floatVal
	case reflect.Float64:
		floatVal := value.Float()
		kvPair.Float64Val = &floatVal
	// 🧱 Complex binary types – slices, maps, pointers, structs (excluding time)
	case reflect.Slice:

		// Special case for []byte → raw binary value
		if value.Type().Elem().Kind() == reflect.Uint8 {
			kvPair.BytesVal = value.Bytes()
		} else {
			// All other slices are GOB-encoded
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
		registerGobTypeIfNeeded(value.Interface())
		var buf bytes.Buffer
		encoder := gob.NewEncoder(&buf)
		if encErr := encoder.Encode(value.Interface()); encErr != nil {
			err = fmt.Errorf("could not GOB-encode map: %w", encErr)
			break
		}
		kvPair.BytesVal = buf.Bytes()

	case reflect.Ptr:

		if value.IsNil() {
			// Ignore nil pointers
			break
		}

		registerGobTypeIfNeeded(value.Interface())
		var buf bytes.Buffer
		encoder := gob.NewEncoder(&buf)
		if encErr := encoder.Encode(value.Interface()); encErr != nil {
			err = fmt.Errorf("could not GOB-encode pointer value: %w", encErr)
			break
		}

		kvPair.BytesVal = buf.Bytes()

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
