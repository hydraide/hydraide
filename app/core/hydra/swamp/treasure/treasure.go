package treasure

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/guard"
)

// Treasure represents a single piece of data in a swamp, along with a set of
// operations that can be executed upon it. It's designed to be concurrency-safe
// and to ensure the correct sequence of operations on the data.
//
// Most of the methods in Treasure interface require a guardID which is
// provided by the Guard interface. This ensures that the methods are
// executed in the order they were called by different goroutines.
type Treasure interface {

	// -------------------------- HEAD FUNCTIONS -------------------------- //
	// Head function are functions that are used by the Had and the Body of the Hydra as well

	// Guard is a function that is used to protect the treasure from being modified by other goroutines
	//
	// As you can see, we use the guardID for every function call except for the GetKey function.
	// The Guard object and the GuardID ensure that a given function, e.g. CloneContent, runs only when
	// another goroutine is not using it. The Guard is not a standard sync.Mutex, but a special object that not
	// only prevents another routine from accessing a single treasure at the same time, but also establishes an
	// order, as it's very important in databases to have calling routines modify or access information in the
	// order they were called. For example, if a user wants to top up their balance, we first top it up, and then
	// they can purchase anything from their balance. If the two routines come in almost simultaneously, the regular
	// Sync package doesn't guarantee that the order would necessarily be top-up followed by purchase. That's why we
	// use the Guard to protect the data and also ensure the correct sequence.
	guard.Guard

	Clone(guardID guard.ID) Treasure

	// GetKey returns the unique key identifying the treasure in the database.
	//
	// GetKey is an exception to the guard protection rule, and can be called without
	// acquiring a guardID from Guard interface. This is because reading the key is
	// considered a non-mutating and non-order-sensitive operation.
	//
	// Example:
	//     treasure := acquireTreasureFromDB("someKey")
	//     key := treasure.GetKey()
	//     fmt.Println("The key is:", key)
	//
	// Use-cases:
	// 1. Fetching the key for logging or debugging purposes.
	// 2. Using the key to index into other data structures or databases.
	// 3. Passing the key to other services or components that need to identify the treasure.
	GetKey() string

	// IsDifferentFrom returns true if the content of the treasure is different from the content of the otherTreasure.
	// This function is useful for comparing two treasures to determine if they are different.
	//
	// This function requires a guardID, which ensures that the method
	// is executed in a concurrency-safe manner and in the sequence
	// it was called among other methods that require a guardID.
	//
	// To acquire and release a guardID, use the Guard interface's
	// StartTreasureGuard and ReleaseTreasureGuard methods, respectively.
	//
	// Example:
	//     ...
	//
	//     treasure := swamp.GetTreasure("someKey")
	//     guardID := treasure.StartTreasureGuard()
	//     isDifferent := treasure.IsDifferentFrom(guardID, otherTreasure)
	//     treasure.ReleaseTreasureGuard(guardID)
	//     fmt.Println("The content is different:", isDifferent)
	//
	// Use-cases:
	// 1. To determine if the content of a treasure has changed.
	// 2. To perform conditional logic based on whether the content is different.
	IsDifferentFrom(guardID guard.ID, otherTreasure Treasure) bool

	// GetContentType returns the type of content stored in the treasure.
	//
	// This function requires a guardID, which ensures that the method
	// is executed in a concurrency-safe manner and in the sequence
	// it was called among other methods that require a guardID.
	//
	// Example:
	//     ...
	//
	//     treasure := swamp.GetTreasure("someKey")
	//     contentType := treasure.GetContentType()
	//     fmt.Println("The content type is:", contentType)
	//
	// Use-cases:
	// 1. To perform conditional logic based on the content type.
	// 2. To log or debug the type of content being worked upon.
	// 3. To facilitate type-specific serialization or deserialization processes.
	GetContentType() ContentType

	// GetCreatedAt returns the UnixNano timestamp marking the creation time of the treasure.
	// This is useful for auditing, tracking life cycle, and time-sensitive operations on the treasure.
	//
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     creationTime := treasure.GetCreatedAt()
	//     fmt.Println("The treasure was created at:", time.Unix(0, creationTime))
	//
	// Use-cases:
	// 1. For auditing purposes to know when the treasure was created.
	// 2. To implement time-based expiration policies.
	// 3. To sort or filter treasures based on their age.
	GetCreatedAt() int64

	// GetCreatedBy returns the userID of the entity who created the treasure.
	// This method can help in situations where ownership or creatorship information is necessary for
	// authorization or auditing. If the treasure is system-generated, or if the CreatedBy field is not set,
	// this method returns an empty string.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     creator := treasure.GetCreatedBy()
	//     fmt.Println("The treasure was created by:", creator)
	//
	// Use-cases:
	// 1. Auditing who created specific treasures.
	// 2. Implementing ownership-based access controls.
	// 3. Data analytics related to user-generated treasures.
	GetCreatedBy() string

	// GetDeletedAt returns the UnixNano timestamp indicating when the treasure was deleted.
	// This can be useful for auditing or record-keeping. If the treasure has not been deleted, this method
	// returns 0. To ensure a synchronized and safe access, a guardID obtained from StartTreasureGuard is
	// necessary.
	//
	// It is crucial to release the guardID after its use with the method ReleaseTreasureGuard to avoid
	// locking the treasure for future access.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     deletionTime := treasure.GetDeletedAt()
	//     if deletionTime == 0 {
	//         fmt.Println("The treasure has not been deleted.")
	//     } else {
	//         fmt.Println("The treasure was deleted at:", deletionTime)
	//     }
	//
	// Use-cases:
	// 1. Auditing or record-keeping of when specific treasures were deleted.
	// 2. Conditional logic based on whether a treasure has been deleted.
	// 3. Data analytics related to treasure lifecycle.
	GetDeletedAt() int64

	// GetDeletedBy returns the userID of the user who deleted the treasure.
	// This function is useful for auditing or identifying who performed the deletion action.
	// The method returns an empty string if the treasure hasn't been deleted yet, or if the DeletedBy field
	// was not set during the deletion process.
	//
	// A guardID, obtained via the StartTreasureGuard method, is required for synchronized and safe access.
	//
	// It's important to release the guardID using the method ReleaseTreasureGuard to ensure that the
	// treasure can be accessed by others in the future.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     deletedBy := treasure.GetDeletedBy()
	//     if deletedBy == "" {
	//         fmt.Println("The treasure has not been deleted, or the deleter is not specified.")
	//     } else {
	//         fmt.Println("The treasure was deleted by:", deletedBy)
	//     }
	//
	// Use-cases:
	// 1. Auditing to find out who deleted specific treasures.
	// 2. Implementing business logic that varies depending on the user who deleted the treasure.
	// 3. Providing informative logs or analytics data.
	GetDeletedBy() string

	// GetShadowDelete returns true if the treasure has been shadow-deleted.
	GetShadowDelete() bool

	// GetModifiedAt returns the UnixNano timestamp of the last modification made to the treasure.
	// This function is useful for tracking changes and understanding the state of the treasure over time.
	// The method returns 0 if the treasure has not been modified since its creation.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     modifiedAt := treasure.GetModifiedAt()
	//     if modifiedAt == 0 {
	//         fmt.Println("The treasure has not been modified yet.")
	//     } else {
	//         fmt.Println("The treasure was last modified at UnixNano time:", modifiedAt)
	//     }
	//
	// Use-cases:
	// 1. For auditing and change tracking of treasures.
	// 2. To implement caching mechanisms that rely on last modification time.
	// 3. To provide a historical context in analytics or dashboards.
	GetModifiedAt() int64

	// GetModifiedBy returns the userID of the individual who last modified the treasure.
	// This function serves as a means to track who last made changes to the treasure, thus aiding in auditing and accountability.
	// The returned string will be empty if the treasure has not been modified or if the ModifiedBy field is not set.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     modifiedBy := treasure.GetModifiedBy()
	//     if modifiedBy == "" {
	//         fmt.Println("The treasure has not been modified yet or the ModifiedBy field is not set.")
	//     } else {
	//         fmt.Println("The treasure was last modified by:", modifiedBy)
	//     }
	//
	// Use-cases:
	// 1. For auditing purposes, to know who last modified the treasure.
	// 2. To implement permissions and role-based access control.
	// 3. To facilitate communication within a team by knowing who to reach out to about specific changes.
	GetModifiedBy() string

	// GetExpirationTime returns the expiration time of the treasure in UnixNano format.
	// This function is used to determine when a particular treasure will expire and become eligible for specific tasks or removal.
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     expirationTime := treasure.GetExpirationTime()
	//
	//     if expirationTime == 0 {
	//         fmt.Println("The treasure does not have an expiration time set.")
	//     } else {
	//         fmt.Println("The treasure will expire at:", expirationTime)
	//     }
	//
	// Use-cases:
	// 1. To implement rate limiting, for example, not accessing a website more frequently than every 10 seconds.
	// 2. To perform actions only after a certain period has elapsed.
	// 3. To clean up or recycle resources that are no longer needed after their expiration.
	GetExpirationTime() int64

	// IsExpired returns true if the treasure's ExpirationTime is set and the current time is greater than the ExpirationTime.
	// IsExpired returns false if the ExpirationTime is not set or if the current time is less than the ExpirationTime.
	// This function allows for quick checks to determine if a treasure should no longer be utilized or accessed.
	//
	// It's imperative to release the guardID using the ReleaseTreasureGuard method, ensuring that the treasure remains accessible to others.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     if treasure.IsExpired() {
	//         fmt.Println("The treasure is expired.")
	//     } else {
	//         fmt.Println("The treasure is not expired.")
	//     }
	//
	// Use-cases:
	// 1. To prevent making API calls or resource-consuming operations on expired treasures.
	// 2. To execute cleanup routines selectively.
	// 3. For decision-making in conditional logic where expiration is a key factor.
	IsExpired() bool

	// GetContentString returns the content of the treasure as a string.
	// This function allows for type-safe retrieval of string data stored within the treasure.
	//
	// The function returns an error if the content type is not a string.
	// If the content is nil, the function will return an empty string but the error will be nil.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     content, err := treasure.GetContentString()
	//     if err != nil {
	//         fmt.Println("Error:", err)
	//     } else {
	//         fmt.Println("Content:", content)
	//     }
	//
	// Use-cases:
	// 1. When we need to selectively read string data without wanting to deal with type assertions.
	// 2. For avoiding run-time panics due to incorrect type assertions.
	// 3. In applications where string data types are frequently used and must be safely accessed.
	GetContentString() (string, error)

	// GetContentUint8 returns the content of the treasure as a uint8.
	// Returns an error if the content type is not uint8. If the content is nil, it returns 0 with no error.
	GetContentUint8() (uint8, error)
	// GetContentUint16 returns the content of the treasure as a uint16.
	// Returns an error if the content type is not uint16. If the content is nil, it returns 0 with no error.
	GetContentUint16() (uint16, error)
	// GetContentUint32 returns the content of the treasure as a uint32.
	// Returns an error if the content type is not uint32. If the content is nil, it returns 0 with no error.
	GetContentUint32() (uint32, error)
	// GetContentUint64 returns the content of the treasure as a uint64.
	// Returns an error if the content type is not uint64. If the content is nil, it returns 0 with no error.
	GetContentUint64() (uint64, error)
	// GetContentInt8 returns the content of the treasure as an int8.
	// Returns an error if the content type is not int8. If the content is nil, it returns 0 with no error.
	GetContentInt8() (int8, error)
	// GetContentInt16 returns the content of the treasure as an int16.
	// Returns an error if the content type is not int16. If the content is nil, it returns 0 with no error.
	GetContentInt16() (int16, error)
	// GetContentInt32 returns the content of the treasure as an int32.
	// Returns an error if the content type is not int32. If the content is nil, it returns 0 with no error.
	GetContentInt32() (int32, error)
	// GetContentInt64 returns the content of the treasure as an int64.
	// Returns an error if the content type is not int64. If the content is nil, it returns 0 with no error.
	GetContentInt64() (int64, error)
	// GetContentFloat32 returns the content of the treasure as a float32.
	// Returns an error if the content type is not float32. If the content is nil, it returns 0.0 with no error.
	GetContentFloat32() (float32, error)
	// GetContentFloat64 returns the content of the treasure as a float64.
	// Returns an error if the content type is not float64. If the content is nil, it returns 0.0 with no error.
	GetContentFloat64() (float64, error)

	// Uint32SliceGetAll returns all stored uint32 values in the Uint32Slice.
	// This function retrieves the entire slice as a Uint32Slice type, ensuring that
	// the stored data remains structured in 4-byte aligned uint32 blocks.
	//
	// Returns an error if the content type is invalid or the slice is uninitialized.
	//
	// Example:
	//     slice, err := treasure.Uint32SliceGetAll()
	//     if err != nil {
	//         fmt.Println("Error:", err)
	//     } else {
	//         fmt.Println("Stored values:", slice)
	//     }
	//
	// Use cases:
	// - When an application needs to perform set operations on the stored uint32 values.
	// - To retrieve all associated domain IDs or unique identifiers efficiently.
	Uint32SliceGetAll() ([]uint32, error)

	// Uint32SlicePush adds one or more uint32 values to the Uint32Slice.
	// This function ensures that duplicate values are not inserted, preserving the integrity
	// of the stored set. Each uint32 value is stored in a compact 4-byte format.
	//
	// If the slice does not exist, it initializes it before appending new values.
	//
	// Returns an error if the operation fails due to memory constraints or internal inconsistencies.
	//
	// Example:
	//     err := treasure.Uint32SlicePush([]uint32{123456, 789012})
	//     if err != nil {
	//         fmt.Println("Error adding values:", err)
	//     }
	//
	// Use cases:
	// - Efficiently indexing new uint32 values without duplicates.
	// - Preventing redundant storage of identifiers in memory-sensitive applications.
	Uint32SlicePush([]uint32) error

	// Uint32SliceDelete removes specific uint32 values from the Uint32Slice.
	// This function scans the slice and removes occurrences of the specified values,
	// ensuring that the remaining data remains intact.
	//
	// If a value is not found, it is simply ignored. The function restructures the slice
	// to maintain a continuous 4-byte aligned structure.
	//
	// Returns an error if the operation encounters an issue, such as invalid data formatting.
	//
	// Example:
	//     err := treasure.Uint32SliceDelete([]uint32{123456})
	//     if err != nil {
	//         fmt.Println("Error deleting values:", err)
	//     }
	//
	// Use cases:
	// - Dynamically managing uint32-based identifiers, such as domain indexes or lookup keys.
	// - Reducing storage size by eliminating unnecessary or outdated values.
	Uint32SliceDelete([]uint32) error

	// Uint32SliceSize returns the number of uint32 values stored in the Uint32Slice.
	// Since each value is stored in 4 bytes, the size is computed as `len(slice) / 4`.
	//
	// If the slice is empty or uninitialized, it returns 0 without an error.
	//
	// Example:
	//     count, err := treasure.Uint32SliceSize()
	//     if err != nil {
	//         fmt.Println("Error retrieving size:", err)
	//     } else {
	//         fmt.Println("Total stored values:", count)
	//     }
	//
	// Use cases:
	// - Checking if a Uint32Slice is empty before performing operations.
	// - Monitoring storage efficiency by tracking the number of stored uint32 identifiers.
	Uint32SliceSize() (int, error)

	// GetContentBool returns the content of the treasure as a boolean.
	// This function provides a type-safe way to retrieve boolean data stored within the treasure.
	//
	// The function returns an error if the content type is not a boolean.
	// If the content is nil, the function will return false, but the error will be nil.
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//
	//     content, err := treasure.GetContentBool()
	//     if err != nil {
	//         fmt.Println("Error:", err)
	//     } else {
	//         fmt.Println("Content:", content)
	//     }
	// Use-cases:
	// 1. When the application needs to selectively read boolean data without dealing with type assertions.
	// 2. To avoid run-time panics that can occur with incorrect type assertions.
	// 3. In feature flags, settings, or condition checks where boolean values control application behavior.
	GetContentBool() (bool, error)

	// GetContentByteArray retrieves the content of the treasure as a byte array.
	// This method is particularly useful for storing and retrieving large datasets or binary files within a treasure.
	//
	// The method returns an error if the content type is not a byte array.
	// If the content is nil, the returned byte array will also be nil, but the error will be nil.
	// Example:
	//     treasure := swamp.GetTreasure("fileKey")
	//
	//     byteArray, err := treasure.GetContentByteArray()
	//     if err != nil {
	//         fmt.Println("Error:", err)
	//     } else {
	//         fmt.Println("Retrieved byte array:", byteArray)
	//     }
	//
	// Use-cases:
	// 1. Storing and retrieving binary files like images, audio, or executables.
	// 2. Handling large chunks of data that are best represented in raw byte form.
	// 3. For optimal performance when handling data that doesn't need to be parsed or altered.
	GetContentByteArray() ([]byte, error)

	// CloneContent returns a copy of the content of the treasure, decoupled from
	// the original treasure. This means any modifications made to the returned
	// content will not affect the original treasure's content.
	//
	// This function requires a guardID for concurrency-safe access to the treasure.
	// Obtaining a guardID and not releaseing it can result in locking the treasure
	// from future accesses.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     guardID := treasure.StartTreasureGuard()
	//
	//     clonedContent := treasure.CloneContent(guardID)
	//
	//     treasure.ReleaseTreasureGuard(guardID)
	//     // Now, clonedContent can be modified without affecting the original treasure's content.
	//
	// Use-cases:
	// 1. To perform operations on the content without affecting the original treasure.
	// 2. To generate a snapshot of the content for backup or versioning.
	// 3. To simplify operations that require the content but not the treasure itself.
	CloneContent(guardID guard.ID) Content

	// SetContent replaces the content of the treasure with the specified content,
	// decoupled from the original treasure. This means any modifications made to
	// the original content will not affect the new content set by this function.
	//
	// This function requires a guardID for concurrency-safe access to the treasure.
	// Obtaining a guardID and not releaseing it can result in locking the treasure
	// from future accesses.
	//
	// IMPORTANT: Use this function only if the new content has been obtained using
	// the CloneContent method. If not, consider using specialized setter methods like
	// SetContentByteArray instead.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     guardID := treasure.StartTreasureGuard()
	//
	//     // First, clone the content
	//     clonedContent := treasure.CloneContent(guardID)
	//
	//     // Perform operations on clonedContent
	//
	//     // Now, replace the original content with the modified cloned content
	//     treasure.SetContent(guardID, clonedContent)
	//
	//     treasure.ReleaseTreasureGuard(guardID)
	//     // The original content has been replaced by clonedContent, and can now be
	//     // independently modified.
	//
	// Use-cases:
	// 1. To replace the content after having performed certain operations on a cloned version.
	// 2. To update the content in a concurrency-safe manner.
	// 3. To maintain versioning or backups by replacing content only when it's confirmed to be safe.
	//
	SetContent(guardID guard.ID, content Content)

	// SetExpirationTime sets a future expiration time for the treasure.
	SetExpirationTime(guardID guard.ID, expirationTime time.Time)

	// SetContentVoid sets the content of the treasure to nil.
	// This effectively wipes the existing content, freeing up storage and resetting the state.
	// A guardID, which can be obtained via the StartTreasureGuard method, is required to synchronize and secure access to the treasure.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     guardID := treasure.StartTreasureGuard()
	//
	//     treasure.SetContentVoid(guardID)
	//
	// Use-cases:
	// 1. Deleting a dataset but keeping the treasure itself for future use.
	// 2. Temporarily disabling a treasure without removing it from the system.
	// 3. Resource management, e.g., releasing memory or storage when the content is no longer needed.
	SetContentVoid(guardID guard.ID)

	// ResetContentVoid resets the 'ContentTypeVoid' field of the treasure's content to false.
	// This method should be called whenever the content type of the treasure changes from 'ContentTypeVoid' to any other type.
	// A guardID, which can be obtained via the StartTreasureGuard method, is required to synchronize and secure access to the treasure.
	//
	// Warning:
	// If you change the content from 'ContentTypeVoid' to any other type, it is imperative to call this function to ensure proper behavior.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     guardID := treasure.StartTreasureGuard()
	//
	//     treasure.SetContentInt64(guardID, 42)  // Content changes from 'ContentTypeVoid' to 'ContentTypeInt64'
	//     treasure.ResetContentVoid(guardID)  // Must be called to reset the 'ContentTypeVoid' status
	//
	//     treasure.ReleaseTreasureGuard(guardID)
	//
	// Use-cases:
	// 1. Ensuring that the treasure's content type is correctly reset when changing from 'ContentTypeVoid' to any other type.
	// 2. Resource management, e.g., making sure that the treasure's state is consistent.
	ResetContentVoid(guardID guard.ID)

	// SetContentString sets the content of the treasure to a string value.
	// A guardID, which can be obtained via the StartTreasureGuard method, is required to synchronize and secure access to the treasure.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     guardID := treasure.StartTreasureGuard()
	//
	//     treasure.SetContentString(guardID, "Hello, world!")  // Content is set to a string
	//
	//     treasure.ReleaseTreasureGuard(guardID)
	//
	// Use-cases:
	// 1. Updating or assigning a string value to the treasure's content.
	// 2. Data population, especially when the treasure's previous content type is not a string.
	SetContentString(guardID guard.ID, content string)

	// ResetContentString resets the 'ContentTypeString' field of the treasure's content to nil.
	// This method should be called whenever the content type of the treasure changes from 'ContentTypeString' to any other type.
	// A guardID, which can be obtained via the StartTreasureGuard method, is required to synchronize and secure access to the treasure.
	//
	// Warning:
	// If you change the content from 'ContentTypeString' to any other type, it is imperative to call this function to ensure proper behavior.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     guardID := treasure.StartTreasureGuard()
	//
	//     treasure.SetContentInt64(guardID, 42)  // Content changes from 'ContentTypeString' to 'ContentTypeInt64'
	//     treasure.ResetContentString(guardID)  // Must be called to reset the 'ContentTypeString' status
	//
	//     treasure.ReleaseTreasureGuard(guardID)
	//
	// Use-cases:
	// 1. Ensuring that the treasure's content type is correctly reset when changing from 'ContentTypeString' to any other type.
	// 2. Resource management, e.g., making sure that the treasure's state is consistent.
	ResetContentString(guardID guard.ID)

	SetContentUint8(guardID guard.ID, content uint8)
	SetContentUint16(guardID guard.ID, content uint16)
	SetContentUint32(guardID guard.ID, content uint32)
	SetContentUint64(guardID guard.ID, content uint64)
	SetContentInt8(guardID guard.ID, content int8)
	SetContentInt16(guardID guard.ID, content int16)
	SetContentInt32(guardID guard.ID, content int32)
	SetContentInt64(guardID guard.ID, content int64)

	ResetContentUint8(guardID guard.ID)
	ResetContentUint16(guardID guard.ID)
	ResetContentUint32(guardID guard.ID)
	ResetContentUint64(guardID guard.ID)
	ResetContentInt8(guardID guard.ID)
	ResetContentInt16(guardID guard.ID)
	ResetContentInt32(guardID guard.ID)
	ResetContentInt64(guardID guard.ID)

	ResetContentUint32Slice(guardID guard.ID)

	SetContentFloat32(guardID guard.ID, content float32)
	SetContentFloat64(guardID guard.ID, content float64)

	ResetContentFloat32(guardID guard.ID)
	ResetContentFloat64(guardID guard.ID)

	// SetContentBool sets the content of the treasure to a boolean value.
	// A guardID, which can be obtained via the StartTreasureGuard method, is required to synchronize and secure access to the treasure.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     guardID := treasure.StartTreasureGuard()
	//
	//     treasure.SetContentBool(guardID, true)  // Content is set to boolean
	//
	//     treasure.ReleaseTreasureGuard(guardID)
	//
	// Use-cases:
	// 1. Updating or assigning a boolean value to the treasure's content.
	// 2. Data population, especially when the treasure's previous content type is not boolean.
	SetContentBool(guardID guard.ID, content bool)

	// ResetContentBool resets the 'Bool' field of the treasure's content to nil.
	// This method should be called whenever the content type of the treasure changes from 'Bool' to any other type.
	// A guardID, which can be obtained via the StartTreasureGuard method, is required to synchronize and secure access to the treasure.
	//
	// Warning:
	// If you change the content from 'Bool' to any other type, it is imperative to call this function to ensure proper behavior.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     guardID := treasure.StartTreasureGuard()
	//
	//     treasure.SetContentInt64(guardID, 42)  // Content changes from 'Bool' to 'ContentTypeInt64'
	//     treasure.ResetContentBool(guardID)  // Must be called to reset the 'Bool' status
	//
	//     treasure.ReleaseTreasureGuard(guardID)
	//
	// Use-cases:
	// 1. Ensuring that the treasure's content type is correctly reset when changing from 'Bool' to any other type.
	// 2. Resource management, e.g., making sure that the treasure's state is consistent.
	ResetContentBool(guardID guard.ID)

	// SetContentByteArray sets the content of the treasure to a byte array.
	// A guardID, which can be obtained via the StartTreasureGuard method, is required to synchronize and secure access to the treasure.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     guardID := treasure.StartTreasureGuard()
	//
	//     byteArrayContent := []byte{0x01, 0x02, 0x03}
	//     treasure.SetContentByteArray(guardID, byteArrayContent)  // Content is set to a byte array
	//
	//     treasure.ReleaseTreasureGuard(guardID)
	//
	// Use-cases:
	// 1. Updating or assigning a byte array value to the treasure's content.
	// 2. Data population, especially when the treasure's previous content type is not a byte array.
	SetContentByteArray(guardID guard.ID, content []byte)

	// ResetContentByteArray resets the 'ContentTypeByteArray' field of the treasure's content to nil.
	// This method should be called whenever the content type of the treasure changes from 'ContentTypeByteArray' to any other type.
	// A guardID, which can be obtained via the StartTreasureGuard method, is required to synchronize and secure access to the treasure.
	//
	// Warning:
	// If you change the content from 'ContentTypeByteArray' to any other type, it is imperative to call this function to ensure proper behavior.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     guardID := treasure.StartTreasureGuard()
	//
	//     treasure.SetContentString(guardID, "new value")       // Content changes from 'ContentTypeByteArray' to 'ContentTypeString'
	//     treasure.ResetContentByteArray(guardID)              // Must be called to reset the 'ContentTypeByteArray' status
	//
	//     treasure.ReleaseTreasureGuard(guardID)
	//
	// Use-cases:
	// 1. Ensuring that the treasure's content type is correctly reset when changing from 'ContentTypeByteArray' to any other type.
	// 2. Resource management, e.g., making sure that the treasure's state is consistent.
	ResetContentByteArray(guardID guard.ID)

	// SetCreatedAt sets the UnixNano timestamp marking the creation time of the Treasure.
	// This optional method enriches the Treasure metadata, offering more granularity in
	// data tracking and potential auditing or versioning capabilities.
	//
	// A guardID, obtainable via the StartTreasureGuard method, is required for synchronized and secure access.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     guardID := treasure.StartTreasureGuard()
	//
	//     treasure.SetCreatedAt(guardID)  // Automatically sets the creation time to the current time in UnixNano
	//
	//     treasure.ReleaseTreasureGuard(guardID)
	//
	// Use-cases:
	// 1. Auditing and tracking of data creation.
	// 2. Historical data versioning and management.
	//
	// Note: The method automatically assigns the current UnixNano timestamp as the creation time.
	//
	// This is an optional field. Developers are encouraged to use it where applicable for better data governance.
	SetCreatedAt(guardID guard.ID, createdAt time.Time)

	// SetModifiedAt sets the UnixNano timestamp marking the last modification time of the Treasure.
	// This optional method is particularly useful for tracking data changes over time, potentially aiding
	// in debugging, auditing, or other operational insights.
	//
	// A guardID, obtainable via the StartTreasureGuard method, is required for synchronized and secure access.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     guardID := treasure.StartTreasureGuard()
	//
	//     treasure.SetModifiedAt(guardID)  // Automatically sets the modification time to the current time in UnixNano
	//
	//     treasure.ReleaseTreasureGuard(guardID)
	//
	// Use-cases:
	// 1. Auditing and tracking of data modification.
	// 2. Historical data versioning and management.
	//
	// Note: The method automatically assigns the current UnixNano timestamp as the modification time.
	//
	// This is an optional field. Developers are encouraged to use it where applicable for better data governance.
	SetModifiedAt(guardID guard.ID, modifiedAt time.Time)

	// SetCreatedBy sets the creator ID and the current time as the creation time of the treasure.
	// This function is optional but recommended to use for better user operation tracking.
	// A guardID, which can be obtained via the StartTreasureGuard method, is required to synchronize and secure access to the treasure.
	//
	// Note:
	// If you are storing large data sets like logs where user information is not crucial, you can skip calling this method to save storage space.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     guardID := treasure.StartTreasureGuard()
	//
	//     treasure.SetCreatedBy(guardID, "userID123")  // Sets the creator ID and the creation time
	//
	//     treasure.ReleaseTreasureGuard(guardID)
	//
	// Use-cases:
	// 1. To identify who created the treasure.
	// 2. For auditing and tracking purposes.
	SetCreatedBy(guardID guard.ID, createdBy string)

	// SetModifiedBy sets the modifier ID and the current time as the modified time of the treasure.
	// This function is optional but recommended to use for better user operation tracking.
	// A guardID, which can be obtained via the StartTreasureGuard method, is required to synchronize and secure access to the treasure.
	//
	// Note:
	// If you are storing large data sets like logs where user information is not crucial, you can skip calling this method to save storage space.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     guardID := treasure.StartTreasureGuard()
	//
	//     treasure.SetModifiedBy(guardID, "userID456")  // Sets the modifier ID and the modification time
	//
	//     treasure.ReleaseTreasureGuard(guardID)
	//
	// Use-cases:
	// 1. To identify who last modified the treasure.
	// 2. For auditing and tracking purposes.
	SetModifiedBy(guardID guard.ID, modifiedBy string)

	// LoadFromClone loads the Treasure from a Clone.
	// This function is used by the Hydra Body to load the Treasure from a Clone.
	LoadFromClone(guardID guard.ID, clone Treasure)

	// Save saves the Treasure to the file system.
	Save(guardID guard.ID) TreasureStatus

	CheckIfContentChanged(newContent *Content) bool

	IsContentChanged() bool

	IsContentTypeChanged() bool
	IsExpirationTimeChanged() bool
	IsCreatedAtChanged() bool
	IsCreatedByChanged() bool
	IsDeletedAtChanged() bool
	IsDeletedByChanged() bool
	IsModifiedAtChanged() bool
	IsModifiedByChanged() bool

	// -------------------------- BODY FUNCTIONS -------------------------- //
	// System function are functions that are used by the system and should not be used by the Head of the Hydra

	// GetFileName returns the name of the file where the Treasure is stored.
	// The string pointer will be nil if the treasure is not stored in a file yet and resides only in memory.
	// This function is not accessible from Head Plugins.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     fileName := treasure.GetFileName()  // Retrieves the file name or nil
	//
	// Use-cases:
	// 1. For internal system-level auditing or tracking.
	// 2. To diagnose issues with file storage.
	GetFileName() *string

	// BodySetFileName sets the file name where the Treasure is stored.
	// This function is not accessible from Head Plugins.
	//
	// A guardID, which can be obtained via the StartTreasureGuard method, is required to synchronize and secure access to the treasure.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     guardID := treasure.StartTreasureGuard()
	//
	//     treasure.SetFileName(guardID, "newFile.txt")  // Sets the file name
	//
	//     treasure.ReleaseTreasureGuard(guardID)
	//
	// Use-cases:
	// 1. To explicitly specify where the treasure will be stored in the file system.
	// 2. To manage the organization and naming of treasures in persistent storage.
	BodySetFileName(guardID guard.ID, fileName string)

	// BodySetForDeletion sets the treasure for deletion.
	// This function marks the treasure as deleted by setting various fields: `DeletedAt` and `DeletedBy`.
	// It can optionally remove the content and expiration time based on the `shadowDelete` parameter.
	//
	// The `shadowDelete` parameter determines whether the content and expiration time should be removed:
	// - If `shadowDelete` is set to `true`, the treasure will be marked as deleted, but its content and expiration time
	//   will remain intact, allowing for potential restoration or reference to the original state of the treasure.
	// - If `shadowDelete` is set to `false`, the function sets the content to `nil`, the expiration time to 0, and the version
	//   is set to 0, indicating that the treasure is effectively purged but can still be accessed in memory until the Chronicler
	//   finalizes the deletion.
	//
	// The treasure remains accessible in memory with a deleted status indicator until the Chronicler commits the changes to the
	// file system. Once committed, the Chronicler will actually delete the data, removing the key from the file system and
	// subsequently purging it from memory as well.
	//
	// A `guardID`, which can be obtained via the `StartTreasureGuard` method, is required to synchronize and secure access to the treasure.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     guardID := treasure.StartTreasureGuard()
	//
	//     treasure.BodySetForDeletion(guardID, "userID123", true)  // Marks the treasure for shadow deletion
	//     treasure.BodySetForDeletion(guardID, "userID123", false) // Marks the treasure for complete deletion
	//
	//     treasure.ReleaseTreasureGuard(guardID)
	//
	// Use-cases:
	// 1. To initiate the secure removal of sensitive or obsolete data.
	// 2. To manage the lifecycle of a treasure, especially in situations where the data is ephemeral or subject to frequent changes.
	// 3. To perform a "soft delete" operation with `shadowDelete` for future restoration or auditing purposes.
	BodySetForDeletion(guardID guard.ID, byUserID string, shadowDelete bool)

	// BodySetKey sets the key of the treasure when it is created.
	// This function is critical as it establishes the unique identifier for the treasure. It is called only once,
	// at the moment of the treasure's creation within the system. Once the key is set, it cannot be changed.
	//
	// A guardID, which can be obtained via the StartTreasureGuard method, is required to synchronize and secure access to the treasure.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     guardID := treasure.StartTreasureGuard()
	//
	//     treasure.SetKey(guardID, "newUniqueKey")  // Sets the unique key for the treasure
	//
	//     treasure.ReleaseTreasureGuard(guardID)
	//
	// Use-cases:
	// 1. To uniquely identify a newly created treasure.
	// 2. To ensure data integrity by preventing duplicate keys in the system.
	//
	// Note: This function should only be called at the time of treasure creation.
	BodySetKey(guardID guard.ID, key string)

	// ConvertToByte serializes the Treasure to a binary
	//
	// Note: This method is reserved for use by the Hydra Body. Hydra Head Plugins should NOT invoke this method.
	//
	// A guardID, obtainable via the StartTreasureGuard method, is required for synchronized and secure access.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     guardID := treasure.StartTreasureGuard()
	//
	//     jsonBytes, err := treasure.ConvertToByte(guardID)
	//
	//     if err != nil {
	//         log.Println("Failed to convert Treasure to JSON:", err)
	//     }
	//
	//     treasure.ReleaseTreasureGuard(guardID)
	//
	// Use-cases:
	// 1. Persisting the Treasure state to disk.
	// 2. Sharing the Treasure state with other parts of the system that can read JSON.
	ConvertToByte(guardID guard.ID) ([]byte, error)

	// LoadFromByte load the Treasure from a binary
	//
	// Note: This method is reserved for use by the Hydra Body. Hydra Head Plugins should NOT invoke this method.
	//
	// A guardID, obtainable via the StartTreasureGuard method, is required for synchronized and secure access.
	//
	// Example:
	//     treasure := swamp.GetTreasure("someKey")
	//     guardID := treasure.StartTreasureGuard()
	//
	//     err := treasure.LoadFromByte(guardID, jsonBytes, "someFileName")
	//
	//     if err != nil {
	//         log.Println("Failed to load Treasure from JSON:", err)
	//     }
	//
	//     treasure.ReleaseTreasureGuard(guardID)
	//
	// Use-cases:
	// 1. Loading a previously saved Treasure state from disk.
	// 2. Initializing a new Treasure instance from a JSON representation.
	LoadFromByte(guardID guard.ID, b []byte, fileName string) error
}

// ContentType represents the type of the content stored in a Treasure instance.
// Different constants are defined to help developers specify and understand
// the type of data they are dealing with. It aids in both serialization and
// deserialization processes and adds a layer of type safety.
type ContentType int

const (
	ContentTypeVoid ContentType = iota
	ContentTypeUint8
	ContentTypeUint16
	ContentTypeUint32
	ContentTypeUint64
	ContentTypeInt8
	ContentTypeInt16
	ContentTypeInt32
	ContentTypeInt64
	ContentTypeFloat32
	ContentTypeFloat64
	ContentTypeString
	ContentTypeBoolean
	ContentTypeByteArray
	ContentTypeUint32Slice
)

// Uint32Slice is a specialized byte slice designed for storing
// and managing 4-byte (uint32) values in a compact and efficient format.
//
// This type is useful when working with datasets where fixed-size
// 32-bit integers need to be stored, searched, and manipulated
// while minimizing memory overhead compared to traditional structures.
//
// Key use cases:
// - Efficient storage of large sets of uint32 identifiers (e.g., hashed keys, IDs).
// - Compact representation of indexed data to reduce memory footprint.
// - Fast binary operations such as searching, appending, and deleting elements.
//
// Unlike a generic []byte slice, Uint32Slice ensures that its contents
// are structured in 4-byte segments, making operations predictable
// and optimized for performance.
type Uint32Slice []byte

// Content is a struct that holds the actual data in a Treasure instance.
// The use of pointer fields and omitempty allows us to minimize the memory footprint
// and serialization size by only including fields that are actually used.
type Content struct {
	Void      bool     // If true, indicates that the content is void, optimizing storage.
	Uint8     *uint8   // Holds an integer value if applicable. Nil if not used.
	Uint16    *uint16  // Holds an integer value if applicable. Nil if not used.
	Uint32    *uint32  // Holds an integer value if applicable. Nil if not used.
	Uint64    *uint64  // Holds an integer value if applicable. Nil if not used.
	Int8      *int8    // Holds an integer value if applicable. Nil if not used.
	Int16     *int16   // Holds an integer value if applicable. Nil if not used.
	Int32     *int32   // Holds an integer value if applicable. Nil if not used.
	Int64     *int64   // Holds an integer value if applicable. Nil if not used.
	Float32   *float32 // Holds a floating-point value if applicable. Nil if not used.
	Float64   *float64 // Holds a floating-point value if applicable. Nil if not used.
	String    *string  // Holds a string value if applicable. Nil if not used.
	Boolean   *bool    // Holds a boolean value if applicable. Nil if not used.
	ByteArray []byte   // Holds binary data if applicable. Empty if not used.
	// Uint32Slice is a specialized byte slice designed for storing and managing 4-byte (uint32) values in a
	// compact and efficient format.
	Uint32Slice *Uint32Slice
}

// TreasureStatus is an enumeration type representing the status of a "Treasure" operation in the Swamp.
//
// This enumeration defines various status values that indicate the result of operations related to "Treasures" in the Swamp.
// It is used to provide information about the outcome of functions such as storing, updating, or deleting treasures.
//
// Possible Values:
// - StatusVoid (TreasureStatus): This is a special status value indicating that no status should be sent to the channel.
// - StatusNew (TreasureStatus): Sent to the channel when a new Treasure is created in the Swamp.
// - StatusModified (TreasureStatus): Sent to the channel when a Treasure is modified.
// - StatusDeleted (TreasureStatus): Sent to the channel when a Treasure is deleted.
// - StatusSame (TreasureStatus): Not sent to the channel when a Treasure is not modified.
//
// Use-cases:
// 1. Providing clear status information about Treasure-related operations.
// 2. Determining the outcome of operations and taking appropriate actions based on the status value.
type TreasureStatus int8

const (
	StatusVoid     TreasureStatus = -1   // This is a special status that indicates that we don't want to send any status to the channel.
	StatusNew      TreasureStatus = iota // StatusNew send to the channel when a new Treasure is created in the swamp as new Treasure.
	StatusModified                       // StatusModified send to the channel when a Treasure is modified
	StatusDeleted                        // StatusDeleted send to the channel when a Treasure is deleted
	StatusSame                           // StatusSame not send any data to the channel when a Treasure is not modified
)

// Model is the model of the treasure but DO NOT modify this struct from outside the package
type Model struct {
	Key              string   // unique key of the content. This is the string from the map[string]
	Content          *Content // content of the treasure. May be nil if we want to delete the content or the content is EMPTY
	CreatedAt        int64    // when the data inserted into the system in UnixNano
	CreatedBy        string   // UID of the creator, who created the treasure
	CreatedByChanged bool     // flag to indicate if the created by is changed or not
	DeletedAt        int64    // the unix time (UnixNano) if the content was removed from the map
	DeletedBy        string   // UID of the deleter, who deleted the treasure
	ModifiedAt       int64    // the unix time (UnixNano) if the content was modified
	ModifiedBy       string   // UID of the modifier, who modified the treasure
	ExpirationTime   int64    // the unix time for time type ordering. This field should be empty, but useful if we want to create a message queue
	FileName         *string  // the current file name pointer. Pointer because we don't want to store the file name in the database
}

type treasure struct {
	mu       sync.RWMutex
	treasure Model
	guard.Guard
	saveMethod            func(t Treasure, guardID guard.ID) TreasureStatus
	expirationTimeChanged bool // flag to indicate if the expiration time is changed or not
	contentChanged        bool // flag to indicate if the content is changed or not
	contentTypeChanged    bool // flag to indicate if the content type is changed or not
	createdAtChanged      bool // flag to indicate if the created at is changed or not
	createdByChanged      bool // flag to indicate if the created by is changed or not
	deletedAtChanged      bool // flag to indicate if the deleted at is changed or not
	deletedByChanged      bool // flag to indicate if the deleted by is changed or not
	shadowDeleted         bool // flag to indicate if the treasure is shadow deleted or not
	modifiedAtChanged     bool // flag to indicate if the modified at is changed or not
	modifiedByChanged     bool // flag to indicate if the modified by is changed or not
}

func New(saveMethod func(t Treasure, guardID guard.ID) TreasureStatus) Treasure {
	return &treasure{
		treasure:   Model{},
		Guard:      guard.New(),
		saveMethod: saveMethod,
	}
}

// LoadFromClone loads the treasure from a clone
func (t *treasure) LoadFromClone(guardID guard.ID, clone Treasure) {
	_ = t.Guard.CanExecute(guardID)
	t.treasure = clone.(*treasure).treasure
}

// GetContentType returns the content type of the treasure
func (t *treasure) GetContentType() ContentType {

	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.treasure.Content == nil || t.treasure.Content.Void {
		return ContentTypeVoid
	}
	if t.treasure.Content.Uint8 != nil {
		return ContentTypeUint8
	}
	if t.treasure.Content.Uint16 != nil {
		return ContentTypeUint16
	}
	if t.treasure.Content.Uint32 != nil {
		return ContentTypeUint32
	}
	if t.treasure.Content.Uint64 != nil {
		return ContentTypeUint64
	}
	if t.treasure.Content.Int8 != nil {
		return ContentTypeInt8
	}
	if t.treasure.Content.Int16 != nil {
		return ContentTypeInt16
	}
	if t.treasure.Content.Int32 != nil {
		return ContentTypeInt32
	}
	if t.treasure.Content.Int64 != nil {
		return ContentTypeInt64
	}
	if t.treasure.Content.Float32 != nil {
		return ContentTypeFloat32
	}
	if t.treasure.Content.Float64 != nil {
		return ContentTypeFloat64
	}
	if t.treasure.Content.String != nil {
		return ContentTypeString
	}
	if t.treasure.Content.Boolean != nil {
		return ContentTypeBoolean
	}
	if t.treasure.Content.ByteArray != nil {
		return ContentTypeByteArray
	}
	if t.treasure.Content.Uint32Slice != nil {
		return ContentTypeUint32Slice
	}
	return ContentTypeVoid
}

func (t *treasure) ResetContentByteArray(guardID guard.ID) {
	_ = t.Guard.CanExecute(guardID)
	if t.treasure.Content != nil && t.treasure.Content.ByteArray != nil {
		t.contentChanged = true
		t.contentTypeChanged = true
		t.treasure.Content.ByteArray = nil
	}
}
func (t *treasure) ResetContentBool(guardID guard.ID) {
	_ = t.Guard.CanExecute(guardID)
	if t.treasure.Content != nil && t.treasure.Content.Boolean != nil {
		t.contentChanged = true
		t.contentTypeChanged = true
		t.treasure.Content.Boolean = nil
	}
}
func (t *treasure) ResetContentFloat32(guardID guard.ID) {
	_ = t.Guard.CanExecute(guardID)
	if t.treasure.Content != nil && t.treasure.Content.Float32 != nil {
		t.contentChanged = true
		t.contentTypeChanged = true
		t.treasure.Content.Float32 = nil
	}
}
func (t *treasure) ResetContentFloat64(guardID guard.ID) {
	_ = t.Guard.CanExecute(guardID)
	if t.treasure.Content != nil && t.treasure.Content.Float64 != nil {
		t.contentChanged = true
		t.contentTypeChanged = true
		t.treasure.Content.Float64 = nil
	}
}
func (t *treasure) ResetContentUint8(guardID guard.ID) {
	_ = t.Guard.CanExecute(guardID)
	if t.treasure.Content != nil && t.treasure.Content.Uint8 != nil {
		t.contentChanged = true
		t.contentTypeChanged = true
		t.treasure.Content.Uint8 = nil
	}
}
func (t *treasure) ResetContentUint16(guardID guard.ID) {
	_ = t.Guard.CanExecute(guardID)
	if t.treasure.Content != nil && t.treasure.Content.Uint16 != nil {
		t.contentChanged = true
		t.contentTypeChanged = true
		t.treasure.Content.Uint16 = nil
	}
}
func (t *treasure) ResetContentUint32(guardID guard.ID) {
	_ = t.Guard.CanExecute(guardID)
	if t.treasure.Content != nil && t.treasure.Content.Uint32 != nil {
		t.contentChanged = true
		t.contentTypeChanged = true
		t.treasure.Content.Uint32 = nil
	}
}
func (t *treasure) ResetContentUint64(guardID guard.ID) {
	_ = t.Guard.CanExecute(guardID)
	if t.treasure.Content != nil && t.treasure.Content.Uint64 != nil {
		t.contentChanged = true
		t.contentTypeChanged = true
		t.treasure.Content.Uint64 = nil
	}
}
func (t *treasure) ResetContentInt8(guardID guard.ID) {
	_ = t.Guard.CanExecute(guardID)
	if t.treasure.Content != nil && t.treasure.Content.Int8 != nil {
		t.contentChanged = true
		t.contentTypeChanged = true
		t.treasure.Content.Int8 = nil
	}
}
func (t *treasure) ResetContentInt16(guardID guard.ID) {
	_ = t.Guard.CanExecute(guardID)
	if t.treasure.Content != nil && t.treasure.Content.Int16 != nil {
		t.contentChanged = true
		t.contentTypeChanged = true
		t.treasure.Content.Int16 = nil
	}
}
func (t *treasure) ResetContentInt32(guardID guard.ID) {
	_ = t.Guard.CanExecute(guardID)
	if t.treasure.Content != nil && t.treasure.Content.Int32 != nil {
		t.contentChanged = true
		t.contentTypeChanged = true
		t.treasure.Content.Int32 = nil
	}
}
func (t *treasure) ResetContentInt64(guardID guard.ID) {
	_ = t.Guard.CanExecute(guardID)
	if t.treasure.Content != nil && t.treasure.Content.Int64 != nil {
		t.contentChanged = true
		t.contentTypeChanged = true
		t.treasure.Content.Int64 = nil
	}
}
func (t *treasure) ResetContentUint32Slice(guardID guard.ID) {
	_ = t.Guard.CanExecute(guardID)
	if t.treasure.Content != nil && t.treasure.Content.Uint32Slice != nil {
		t.contentChanged = true
		t.contentTypeChanged = true
		t.treasure.Content.Uint32Slice = nil
	}
}

func (t *treasure) ResetContentString(guardID guard.ID) {
	_ = t.Guard.CanExecute(guardID)
	if t.treasure.Content != nil && t.treasure.Content.String != nil {
		t.contentChanged = true
		t.contentTypeChanged = true
		t.treasure.Content.String = nil
	}
}
func (t *treasure) ResetContentVoid(guardID guard.ID) {
	_ = t.Guard.CanExecute(guardID)
	if t.treasure.Content != nil && t.treasure.Content.Void {
		t.contentTypeChanged = true
		t.contentChanged = true
		t.treasure.Content.Void = false
	}
}

func (t *treasure) SetCreatedBy(guardID guard.ID, createdBy string) {
	_ = t.Guard.CanExecute(guardID)
	t.createdByChanged = true
	t.treasure.CreatedBy = createdBy
}
func (t *treasure) SetModifiedBy(guardID guard.ID, modifiedBy string) {
	_ = t.Guard.CanExecute(guardID)
	t.modifiedByChanged = true
	t.treasure.ModifiedBy = modifiedBy
}

func (t *treasure) Clone(guardID guard.ID) Treasure {

	_ = t.Guard.CanExecute(guardID)

	newObj := &treasure{
		treasure: Model{
			Key:            t.treasure.Key,
			ExpirationTime: t.treasure.ExpirationTime,
			Content:        nil,
			CreatedAt:      t.treasure.CreatedAt,
			CreatedBy:      t.treasure.CreatedBy,
			DeletedAt:      0,
			DeletedBy:      "",
			ModifiedAt:     t.treasure.ModifiedAt,
			ModifiedBy:     t.treasure.ModifiedBy,
			// We don't want to clone the file name pointer because this treasure will be a new treasure and the fileName
			// should be a new, too.
			// And the chronicler will write the treasure as a new one, only if the fileName is nil
			FileName: nil,
		},
		Guard: guard.New(),
	}

	// clone just the content
	newContent := Content{}
	if t.treasure.Content != nil {
		newContent = t.cloneContent()
	}

	newObj.treasure.Content = &newContent

	return newObj

}

func (t *treasure) CloneContent(guardID guard.ID) Content {
	_ = t.Guard.CanExecute(guardID)
	return t.cloneContent()
}

func (t *treasure) cloneContent() Content {
	newContent := Content{}
	if t.treasure.Content != nil {
		if t.treasure.Content.String != nil {
			newString := *t.treasure.Content.String
			newContent.String = &newString
		} else if t.treasure.Content.Uint8 != nil {
			newInt := *t.treasure.Content.Uint8
			newContent.Uint8 = &newInt
		} else if t.treasure.Content.Uint16 != nil {
			newInt := *t.treasure.Content.Uint16
			newContent.Uint16 = &newInt
		} else if t.treasure.Content.Uint32 != nil {
			newInt := *t.treasure.Content.Uint32
			newContent.Uint32 = &newInt
		} else if t.treasure.Content.Uint64 != nil {
			newInt := *t.treasure.Content.Uint64
			newContent.Uint64 = &newInt
		} else if t.treasure.Content.Int8 != nil {
			newInt := *t.treasure.Content.Int8
			newContent.Int8 = &newInt
		} else if t.treasure.Content.Int16 != nil {
			newInt := *t.treasure.Content.Int16
			newContent.Int16 = &newInt
		} else if t.treasure.Content.Int32 != nil {
			newInt := *t.treasure.Content.Int32
			newContent.Int32 = &newInt
		} else if t.treasure.Content.Int64 != nil {
			newInt := *t.treasure.Content.Int64
			newContent.Int64 = &newInt
		} else if t.treasure.Content.Float32 != nil {
			newFloat := *t.treasure.Content.Float32
			newContent.Float32 = &newFloat
		} else if t.treasure.Content.Float64 != nil {
			newFloat := *t.treasure.Content.Float64
			newContent.Float64 = &newFloat
		} else if t.treasure.Content.Boolean != nil {
			newBool := *t.treasure.Content.Boolean
			newContent.Boolean = &newBool
		} else if t.treasure.Content.ByteArray != nil {
			newByteArray := make([]byte, len(t.treasure.Content.ByteArray))
			copy(newByteArray, t.treasure.Content.ByteArray)
			newContent.ByteArray = newByteArray
		} else if t.treasure.Content.Uint32Slice != nil {
			newSlice := make(Uint32Slice, len(*t.treasure.Content.Uint32Slice))
			copy(newSlice, *t.treasure.Content.Uint32Slice)
			newContent.Uint32Slice = &newSlice
		} else if t.treasure.Content.Void {
			newContent.Void = true
		}
	}
	return newContent
}

func (t *treasure) SetContent(guardID guard.ID, content Content) {

	_ = t.Guard.CanExecute(guardID)

	t.contentChanged = false
	if t.IsContentTypeChanged() {
		t.contentChanged = true
	}

	t.treasure.Content = &content

}

func (t *treasure) CheckIfContentChanged(newContent *Content) bool {

	// ha még nincs benne tartalom, de most bekerülne akkor biztos új tartalom lesz
	if t.treasure.Content == nil && newContent != nil {
		return true
	}

	// lekédezzük a treasure jelenlegi content típusát
	ct := t.GetContentType()

	switch ct {
	case ContentTypeVoid:
		if newContent != nil && !newContent.Void {
			return true
		}
	case ContentTypeUint8:
		if newContent == nil || newContent.Uint8 == nil || *newContent.Uint8 != *t.treasure.Content.Uint8 {
			return true
		}
	case ContentTypeUint16:
		if newContent == nil || newContent.Uint16 == nil || *newContent.Uint16 != *t.treasure.Content.Uint16 {
			return true
		}
	case ContentTypeUint32:
		if newContent == nil || newContent.Uint32 == nil || *newContent.Uint32 != *t.treasure.Content.Uint32 {
			return true
		}
	case ContentTypeUint64:
		if newContent == nil || newContent.Uint64 == nil || *newContent.Uint64 != *t.treasure.Content.Uint64 {
			return true
		}
	case ContentTypeInt8:
		if newContent == nil || newContent.Int8 == nil || *newContent.Int8 != *t.treasure.Content.Int8 {
			return true
		}
	case ContentTypeInt16:
		if newContent == nil || newContent.Int16 == nil || *newContent.Int16 != *t.treasure.Content.Int16 {
			return true
		}
	case ContentTypeInt32:
		if newContent == nil || newContent.Int32 == nil || *newContent.Int32 != *t.treasure.Content.Int32 {
			return true
		}
	case ContentTypeInt64:
		if newContent == nil || newContent.Int64 == nil || *newContent.Int64 != *t.treasure.Content.Int64 {
			return true
		}
	case ContentTypeFloat32:
		if newContent == nil || newContent.Float32 == nil || *newContent.Float32 != *t.treasure.Content.Float32 {
			return true
		}
	case ContentTypeFloat64:
		if newContent == nil || newContent.Float64 == nil || *newContent.Float64 != *t.treasure.Content.Float64 {
			return true
		}
	case ContentTypeString:
		if newContent == nil || newContent.String == nil || *newContent.String != *t.treasure.Content.String {
			return true
		}
	case ContentTypeBoolean:
		if newContent == nil || newContent.Boolean == nil || *newContent.Boolean != *t.treasure.Content.Boolean {
			return true
		}
	case ContentTypeByteArray:
		if newContent == nil || newContent.ByteArray == nil || !bytes.Equal(newContent.ByteArray, t.treasure.Content.ByteArray) {
			return true
		}
	case ContentTypeUint32Slice:
		if newContent == nil || newContent.Uint32Slice == nil || !bytes.Equal(*newContent.Uint32Slice, *t.treasure.Content.Uint32Slice) {
			return true
		}
	default:
		return false
	}

	return false

}

func (t *treasure) BodySetFileName(guardID guard.ID, fileName string) {
	if canExecuteErr := t.Guard.CanExecute(guardID); canExecuteErr != nil {
		return
	}
	// does not increase the version because the fileName is not part of the content
	t.treasure.FileName = &fileName
}

func (t *treasure) BodySetForDeletion(guardID guard.ID, byUserID string, shadowDelete bool) {
	if canExecuteErr := t.Guard.CanExecute(guardID); canExecuteErr != nil {
		return
	}
	timeNow := time.Now().UTC().UnixNano()

	t.deletedAtChanged = true
	t.deletedByChanged = true

	t.treasure.DeletedAt = timeNow
	t.treasure.DeletedBy = byUserID

	t.shadowDeleted = shadowDelete

	// DO NOT DELETE THE CONTENT and the expiration time from the treasure if the shadowDelete is true, because we need
	// the whole treasure in unchanged state to be able to restore it, or read it as a deleted treasure
	if !shadowDelete {
		// set the content to nil
		t.treasure.Content = nil
		// set the expiration time to 0 because the treasure is deleted, and we don't want to process it by the expiration time
		t.treasure.ExpirationTime = 0
	}

}

func (t *treasure) GetKey() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.treasure.Key
}

func (t *treasure) SetExpirationTime(guardID guard.ID, expirationTime time.Time) {
	_ = t.Guard.CanExecute(guardID)
	t.expirationTimeChanged = true
	t.treasure.ExpirationTime = expirationTime.UTC().UnixNano()
}

func (t *treasure) GetExpirationTime() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.treasure.ExpirationTime
}

func (t *treasure) GetCreatedAt() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.treasure.CreatedAt
}

func (t *treasure) GetCreatedBy() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.treasure.CreatedBy
}

func (t *treasure) GetDeletedAt() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.treasure.DeletedAt
}

func (t *treasure) GetDeletedBy() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.treasure.DeletedBy
}

func (t *treasure) GetShadowDelete() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.shadowDeleted
}

func (t *treasure) SetModifiedAt(guardID guard.ID, modifiedAt time.Time) {
	_ = t.Guard.CanExecute(guardID)
	t.modifiedAtChanged = true
	t.treasure.ModifiedAt = modifiedAt.UTC().UnixNano()
}

func (t *treasure) GetModifiedAt() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.treasure.ModifiedAt
}

func (t *treasure) GetModifiedBy() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.treasure.ModifiedBy
}

func (t *treasure) GetFileName() *string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.treasure.FileName
}

// BodySetKey sets the key of the treasure
func (t *treasure) BodySetKey(guardID guard.ID, key string) {
	if canExecuteErr := t.Guard.CanExecute(guardID); canExecuteErr != nil {
		return // do nothing
	}
	t.treasure.DeletedBy = ""
	t.treasure.DeletedAt = 0
	t.treasure.Key = key
}

func (t *treasure) ConvertToByte(guardID guard.ID) ([]byte, error) {

	if canExecuteErr := t.Guard.CanExecute(guardID); canExecuteErr != nil {
		return nil, canExecuteErr
	}

	// copy the treasure to a new variable
	// and set the filePointer to empty one, because we don't want to save the filePointer to the Chronicler
	newObj := &treasure{
		treasure: Model{
			Key:            t.treasure.Key,
			ExpirationTime: t.treasure.ExpirationTime,
			CreatedAt:      t.treasure.CreatedAt,
			CreatedBy:      t.treasure.CreatedBy,
			DeletedAt:      t.treasure.DeletedAt,
			DeletedBy:      t.treasure.DeletedBy,
			ModifiedAt:     t.treasure.ModifiedAt,
			ModifiedBy:     t.treasure.ModifiedBy,
			Content:        t.treasure.Content,
			FileName:       nil,
		},
	}

	// if the treasure content is not VOID
	if newObj.treasure.Content != nil && !newObj.treasure.Content.Void {
		newObj.treasure.Content = t.treasure.Content
	}

	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	err := encoder.Encode(t.treasure)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil

}

func (t *treasure) LoadFromByte(guardID guard.ID, b []byte, fileName string) error {
	// guard ellenőrzése
	if canExecuteErr := t.Guard.CanExecute(guardID); canExecuteErr != nil {
		return canExecuteErr
	}

	// bináris adat betöltése
	buf := bytes.NewReader(b)

	// Dekódolás gob formátumból
	decoder := gob.NewDecoder(buf)
	err := decoder.Decode(&t.treasure)
	if err != nil {
		return err
	}
	// filenév beállítása
	t.treasure.FileName = &fileName
	return nil
}

func (t *treasure) SetContentVoid(guardID guard.ID) {
	_ = t.Guard.CanExecute(guardID)

	// if the content is not changed, do nothing
	if t.treasure.Content != nil && t.treasure.Content.Void {
		return
	}

	t.contentChanged = true
	if t.treasure.Content == nil {
		t.treasure.Content = &Content{
			Void: true,
		}
	}

	if t.treasure.Content.Void != false {
		t.treasure.Content.Void = true
	}

}

func (t *treasure) SetContentString(guardID guard.ID, content string) {
	_ = t.Guard.CanExecute(guardID)

	// if the content is not changed, do nothing
	if t.treasure.Content != nil && t.treasure.Content.String != nil && *t.treasure.Content.String == content {
		return
	}

	t.contentChanged = true
	t.treasure.Content = &Content{
		String: &content,
	}
}

func (t *treasure) SetContentUint8(guardID guard.ID, content uint8) {
	_ = t.Guard.CanExecute(guardID)
	// if the content is not changed, do nothing
	if t.treasure.Content != nil && t.treasure.Content.Uint8 != nil && *t.treasure.Content.Uint8 == content {
		return
	}
	t.contentChanged = true
	t.treasure.Content = &Content{
		Uint8: &content,
	}
}
func (t *treasure) SetContentUint16(guardID guard.ID, content uint16) {
	_ = t.Guard.CanExecute(guardID)
	// if the content is not changed, do nothing
	if t.treasure.Content != nil && t.treasure.Content.Uint16 != nil && *t.treasure.Content.Uint16 == content {
		return
	}
	t.contentChanged = true
	t.treasure.Content = &Content{
		Uint16: &content,
	}
}
func (t *treasure) SetContentUint32(guardID guard.ID, content uint32) {
	_ = t.Guard.CanExecute(guardID)
	// if the content is not changed, do nothing
	if t.treasure.Content != nil && t.treasure.Content.Uint32 != nil && *t.treasure.Content.Uint32 == content {
		return
	}
	t.contentChanged = true
	t.treasure.Content = &Content{
		Uint32: &content,
	}
}
func (t *treasure) SetContentUint64(guardID guard.ID, content uint64) {
	_ = t.Guard.CanExecute(guardID)
	// if the content is not changed, do nothing
	if t.treasure.Content != nil && t.treasure.Content.Uint64 != nil && *t.treasure.Content.Uint64 == content {
		return
	}
	t.contentChanged = true
	t.treasure.Content = &Content{
		Uint64: &content,
	}
}
func (t *treasure) SetContentInt8(guardID guard.ID, content int8) {
	_ = t.Guard.CanExecute(guardID)
	// if the content is not changed, do nothing
	if t.treasure.Content != nil && t.treasure.Content.Int8 != nil && *t.treasure.Content.Int8 == content {
		return
	}
	t.contentChanged = true
	t.treasure.Content = &Content{
		Int8: &content,
	}
}
func (t *treasure) SetContentInt16(guardID guard.ID, content int16) {
	_ = t.Guard.CanExecute(guardID)
	// if the content is not changed, do nothing
	if t.treasure.Content != nil && t.treasure.Content.Int16 != nil && *t.treasure.Content.Int16 == content {
		return
	}
	t.contentChanged = true
	t.treasure.Content = &Content{
		Int16: &content,
	}
}
func (t *treasure) SetContentInt32(guardID guard.ID, content int32) {
	_ = t.Guard.CanExecute(guardID)
	// if the content is not changed, do nothing
	if t.treasure.Content != nil && t.treasure.Content.Int32 != nil && *t.treasure.Content.Int32 == content {
		return
	}
	t.contentChanged = true
	t.treasure.Content = &Content{
		Int32: &content,
	}
}
func (t *treasure) SetContentInt64(guardID guard.ID, content int64) {

	_ = t.Guard.CanExecute(guardID)

	// if the content is not changed, do nothing
	if t.treasure.Content != nil && t.treasure.Content.Int64 != nil && *t.treasure.Content.Int64 == content {
		return
	}

	t.contentChanged = true
	t.treasure.Content = &Content{
		Int64: &content,
	}
}

func (t *treasure) SetContentFloat32(guardID guard.ID, content float32) {
	_ = t.Guard.CanExecute(guardID)

	// if treasure content is not changed, do nothing
	if t.treasure.Content != nil && t.treasure.Content.Float32 != nil && *t.treasure.Content.Float32 == content {
		return
	}

	t.contentChanged = true
	t.treasure.Content = &Content{
		Float32: &content,
	}
}

func (t *treasure) SetContentFloat64(guardID guard.ID, content float64) {
	_ = t.Guard.CanExecute(guardID)

	// if treasure content is not changed, do nothing
	if t.treasure.Content != nil && t.treasure.Content.Float64 != nil && *t.treasure.Content.Float64 == content {
		return
	}

	t.contentChanged = true
	t.treasure.Content = &Content{
		Float64: &content,
	}
}

func (t *treasure) SetContentBool(guardID guard.ID, content bool) {
	_ = t.Guard.CanExecute(guardID)

	// if treasure content is not changed, do nothing
	if t.treasure.Content != nil && t.treasure.Content.Boolean != nil && *t.treasure.Content.Boolean == content {
		return
	}

	t.contentChanged = true
	t.treasure.Content = &Content{
		Boolean: &content,
	}

}

func (t *treasure) SetContentByteArray(guardID guard.ID, content []byte) {
	_ = t.Guard.CanExecute(guardID)
	// if the content is not changed, do nothing
	if t.treasure.Content != nil && t.treasure.Content.ByteArray != nil && bytes.Equal(t.treasure.Content.ByteArray, content) {
		return
	}
	t.contentChanged = true
	t.treasure.Content = &Content{
		ByteArray: content,
	}
}

// SetCreatedAt set the created at of the treasure to the current time without locking the mutex
func (t *treasure) SetCreatedAt(guardID guard.ID, createdAt time.Time) {
	_ = t.Guard.CanExecute(guardID)
	t.createdAtChanged = true
	t.treasure.CreatedAt = createdAt.UTC().UnixNano()
}

// GetContentString gets the content of the treasure as a string
func (t *treasure) GetContentString() (string, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.treasure.Content == nil || t.treasure.Content.String == nil {
		return "", fmt.Errorf("content type is not a string")
	}
	return *t.treasure.Content.String, nil
}

func (t *treasure) GetContentUint8() (uint8, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.treasure.Content == nil || t.treasure.Content.Uint8 == nil {
		return 0, fmt.Errorf("content type is not uint8")
	}
	return *t.treasure.Content.Uint8, nil
}
func (t *treasure) GetContentUint16() (uint16, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.treasure.Content == nil || t.treasure.Content.Uint16 == nil {
		return 0, fmt.Errorf("content type is not uint16")
	}
	return *t.treasure.Content.Uint16, nil
}
func (t *treasure) GetContentUint32() (uint32, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.treasure.Content == nil || t.treasure.Content.Uint32 == nil {
		return 0, fmt.Errorf("content type is not uint32")
	}
	return *t.treasure.Content.Uint32, nil
}
func (t *treasure) GetContentUint64() (uint64, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.treasure.Content == nil || t.treasure.Content.Uint64 == nil {
		return 0, fmt.Errorf("content type is not uint64")
	}
	return *t.treasure.Content.Uint64, nil
}
func (t *treasure) GetContentInt8() (int8, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.treasure.Content == nil || t.treasure.Content.Int8 == nil {
		return 0, fmt.Errorf("content type is not int8")
	}
	return *t.treasure.Content.Int8, nil
}
func (t *treasure) GetContentInt16() (int16, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.treasure.Content == nil || t.treasure.Content.Int16 == nil {
		return 0, fmt.Errorf("content type is not int16")
	}
	return *t.treasure.Content.Int16, nil
}
func (t *treasure) GetContentInt32() (int32, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.treasure.Content == nil || t.treasure.Content.Int32 == nil {
		return 0, fmt.Errorf("content type is not int32")
	}
	return *t.treasure.Content.Int32, nil
}

// GetContentInt64 gets the content of the treasure as an integer
func (t *treasure) GetContentInt64() (int64, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.treasure.Content == nil || t.treasure.Content.Int64 == nil {
		return 0, fmt.Errorf("content type is not int64")
	}
	return *t.treasure.Content.Int64, nil
}

func (t *treasure) GetContentFloat32() (float32, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.treasure.Content == nil || t.treasure.Content.Float32 == nil {
		return 0, fmt.Errorf("content type is not a float32")
	}
	return *t.treasure.Content.Float32, nil
}

func (t *treasure) GetContentFloat64() (float64, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.treasure.Content == nil || t.treasure.Content.Float64 == nil {
		return 0, fmt.Errorf("content type is not a float64")
	}
	return *t.treasure.Content.Float64, nil
}

// GetContentBool gets the content of the treasure as a bool
func (t *treasure) GetContentBool() (bool, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.treasure.Content == nil || t.treasure.Content.Boolean == nil {
		return false, fmt.Errorf("content type is not a bool")
	}
	return *t.treasure.Content.Boolean, nil
}

// GetContentByteArray gets the content of the treasure as a byte array
func (t *treasure) GetContentByteArray() ([]byte, error) {

	t.mu.RLock()
	defer t.mu.RUnlock()

	// Checking if the content is nil or if the content type is not a byte array.
	if t.treasure.Content == nil || t.treasure.Content.ByteArray == nil {
		return nil, fmt.Errorf("content type is not a byte array")
	}

	return t.treasure.Content.ByteArray, nil

}

// IsExpired checks if the treasure's time order is expired
func (t *treasure) IsExpired() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.treasure.ExpirationTime == 0 {
		return false
	}
	return t.treasure.ExpirationTime < time.Now().UTC().UnixNano()
}

// IsDifferentFrom checks if the treasure is different from another treasure
func (t *treasure) IsDifferentFrom(guardID guard.ID, otherTreasure Treasure) bool {

	_ = t.Guard.CanExecute(guardID)

	if t.treasure.Key != otherTreasure.GetKey() {
		return true
	}
	if t.treasure.ExpirationTime != otherTreasure.GetExpirationTime() {
		return true
	}

	cat := otherTreasure.GetCreatedAt()

	if t.treasure.CreatedAt != cat {
		return true
	}
	if t.treasure.CreatedBy != otherTreasure.GetCreatedBy() {
		return true
	}
	if t.treasure.DeletedAt != otherTreasure.GetDeletedAt() {
		return true
	}
	if t.treasure.DeletedBy != otherTreasure.GetDeletedBy() {
		return true
	}
	if t.treasure.ModifiedAt != otherTreasure.GetModifiedAt() {
		return true
	}
	if t.treasure.ModifiedBy != otherTreasure.GetModifiedBy() {
		return true
	}

	switch otherTreasure.GetContentType() {
	case ContentTypeVoid:
		if t.treasure.Content == nil || (t.treasure.Content != nil && t.treasure.Content.Void) {
			return true
		}
	case ContentTypeString:
		if t.treasure.Content == nil {
			return true
		} else {
			stringContent, err := otherTreasure.GetContentString()
			if err != nil {
				return true
			}
			if *t.treasure.Content.String != stringContent {
				return true
			}
		}
	case ContentTypeUint8:
		if t.treasure.Content == nil {
			return true
		} else {
			uintContent, err := otherTreasure.GetContentUint8()
			if err != nil {
				return true
			}
			if *t.treasure.Content.Uint8 != uintContent {
				return true
			}
		}
	case ContentTypeUint16:
		if t.treasure.Content == nil {
			return true
		} else {
			uintContent, err := otherTreasure.GetContentUint16()
			if err != nil {
				return true
			}
			if *t.treasure.Content.Uint16 != uintContent {
				return true
			}
		}
	case ContentTypeUint32:
		if t.treasure.Content == nil {
			return true
		} else {
			uintContent, err := otherTreasure.GetContentUint32()
			if err != nil {
				return true
			}
			if *t.treasure.Content.Uint32 != uintContent {
				return true
			}
		}
	case ContentTypeUint64:
		if t.treasure.Content == nil {
			return true
		} else {
			uintContent, err := otherTreasure.GetContentUint64()
			if err != nil {
				return true
			}
			if *t.treasure.Content.Uint64 != uintContent {
				return true
			}
		}
	case ContentTypeInt8:
		if t.treasure.Content == nil {
			return true
		} else {
			intContent, err := otherTreasure.GetContentInt8()
			if err != nil {
				return true
			}
			if *t.treasure.Content.Int8 != intContent {
				return true
			}
		}
	case ContentTypeInt16:
		if t.treasure.Content == nil {
			return true
		} else {
			intContent, err := otherTreasure.GetContentInt16()
			if err != nil {
				return true
			}
			if *t.treasure.Content.Int16 != intContent {
				return true
			}
		}
	case ContentTypeInt32:
		if t.treasure.Content == nil {
			return true
		} else {
			intContent, err := otherTreasure.GetContentInt32()
			if err != nil {
				return true
			}
			if *t.treasure.Content.Int32 != intContent {
				return true
			}
		}
	case ContentTypeInt64:
		if t.treasure.Content == nil {
			return true
		} else {
			intContent, err := otherTreasure.GetContentInt64()
			if err != nil {
				return true
			}
			if *t.treasure.Content.Int64 != intContent {
				return true
			}
		}
	case ContentTypeFloat32:
		if t.treasure.Content == nil {
			return true
		} else {
			floatContent, err := otherTreasure.GetContentFloat32()
			if err != nil {
				return true
			}
			if *t.treasure.Content.Float32 != floatContent {
				return true
			}
		}
	case ContentTypeFloat64:
		if t.treasure.Content == nil {
			return true
		} else {
			floatContent, err := otherTreasure.GetContentFloat64()
			if err != nil {
				return true
			}
			if *t.treasure.Content.Float64 != floatContent {
				return true
			}
		}
	case ContentTypeBoolean:
		if t.treasure.Content == nil {
			return true
		} else {
			boolContent, err := otherTreasure.GetContentBool()
			if err != nil {
				return true
			}
			if *t.treasure.Content.Boolean != boolContent {
				return true
			}
		}
	case ContentTypeByteArray:
		if t.treasure.Content == nil {
			return true
		} else {
			byteArrayContent, err := otherTreasure.GetContentByteArray()
			if err != nil {
				return true
			}
			if !reflect.DeepEqual(t.treasure.Content.ByteArray, byteArrayContent) {
				return true
			}
		}
	case ContentTypeUint32Slice:
		if t.treasure.Content == nil {
			return true
		} else {
			uint32SliceContent, err := otherTreasure.Uint32SliceGetAll()
			if err != nil {
				return true
			}
			if !reflect.DeepEqual(t.treasure.Content.Uint32Slice, uint32SliceContent) {
				return true
			}
		}
	}
	return false
}

// Save saves the treasure to the Swamp
func (t *treasure) Save(guardID guard.ID) TreasureStatus {
	_ = t.Guard.CanExecute(guardID)
	return t.saveMethod(t, guardID)
}

func (t *treasure) IsContentChanged() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.contentChanged
}

func (t *treasure) IsExpirationTimeChanged() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.expirationTimeChanged
}

func (t *treasure) IsCreatedAtChanged() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.createdAtChanged
}

func (t *treasure) IsCreatedByChanged() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.createdByChanged
}

func (t *treasure) IsDeletedAtChanged() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.deletedAtChanged
}

func (t *treasure) IsDeletedByChanged() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.deletedByChanged
}

func (t *treasure) IsModifiedAtChanged() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.modifiedAtChanged
}

func (t *treasure) IsModifiedByChanged() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.modifiedByChanged
}

func (t *treasure) IsContentTypeChanged() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.contentTypeChanged
}

func (t *treasure) Uint32SliceGetAll() ([]uint32, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.treasure.Content == nil || t.treasure.Content.Uint32Slice == nil {
		return nil, fmt.Errorf("content type is not a uint32 slice")
	}
	var result []uint32
	for i := 0; i < len(*t.treasure.Content.Uint32Slice); i += 4 {
		result = append(result, binary.LittleEndian.Uint32((*t.treasure.Content.Uint32Slice)[i:i+4]))
	}
	return result, nil
}

func (t *treasure) Uint32SlicePush(values []uint32) error {

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.treasure.Content == nil {
		t.treasure.Content = &Content{}
	}
	if t.treasure.Content.Uint32Slice == nil {
		t.treasure.Content.Uint32Slice = new(Uint32Slice)
	}

	// Konvertáljuk a meglévő slice-ot egy gyors lookup map-pé
	existing := make(map[uint32]struct{})
	for i := 0; i < len(*t.treasure.Content.Uint32Slice); i += 4 {
		v := binary.LittleEndian.Uint32((*t.treasure.Content.Uint32Slice)[i : i+4])
		existing[v] = struct{}{}
	}

	// Új elemek hozzáadása (csak, ha még nem léteznek)
	var buf []byte
	for _, v := range values {
		if _, found := existing[v]; !found {
			temp := make([]byte, 4)
			binary.LittleEndian.PutUint32(temp, v)
			buf = append(buf, temp...)
			existing[v] = struct{}{} // Hozzáadjuk a lookup map-hez is
		}
	}

	// Ha van új elem, akkor appendeljük
	if len(buf) > 0 {
		*t.treasure.Content.Uint32Slice = append(*t.treasure.Content.Uint32Slice, buf...)
		t.contentChanged = true
	}

	return nil

}

func (t *treasure) Uint32SliceDelete(values []uint32) error {

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.treasure.Content == nil || t.treasure.Content.Uint32Slice == nil {
		return nil // Ha nincs adat, nincs mit törölni
	}

	var newSlice []byte
	for i := 0; i < len(*t.treasure.Content.Uint32Slice); i += 4 {
		v := binary.LittleEndian.Uint32((*t.treasure.Content.Uint32Slice)[i : i+4])
		shouldDelete := false
		for _, del := range values {
			if v == del {
				shouldDelete = true
				break
			}
		}
		if !shouldDelete {
			t.contentChanged = true
			newSlice = append(newSlice, (*t.treasure.Content.Uint32Slice)[i:i+4]...)
		}
	}

	// Frissítjük a slice-ot a törölt elemek nélkül
	*t.treasure.Content.Uint32Slice = newSlice

	return nil

}
func (t *treasure) Uint32SliceSize() (int, error) {

	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.treasure.Content == nil || t.treasure.Content.Uint32Slice == nil {
		return 0, fmt.Errorf("content type is not a uint32 slice")
	}

	// Az összes bájt számát elosztjuk 4-gyel (mert 1 elem = 4 bájt)
	return len(*t.treasure.Content.Uint32Slice) / 4, nil

}
