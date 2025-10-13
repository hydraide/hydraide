// Package beacon is the beacon package for the swamp. We can store treasures in different orders in the beacon, and the beacon
// helps us find the treasures in the beacon. The beacon is several maps of treasures, and the beacon is a map of indexes.
// Beacons always exist only in the Memory
package beacon

import (
	"errors"
	"fmt"
	"maps"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
)

type Beacon interface {

	// GetAll retrieves all treasures from the beacon's internal data structures.
	// The function ensures thread-safety by acquiring a read transaction to prevent concurrent write operations.
	//
	// Returns:
	// - map[string]treasure.Treasure: A map of treasures, where the keys are the unique identifiers for the treasures.
	//
	// Side Effects:
	// - None.
	//
	// When to Use This Function:
	// 1. When you need to access all treasures stored in the beacon's internal data structures for read-only
	// operations or further processing.
	GetAll() map[string]treasure.Treasure

	// Count retrieves the total number of unique treasures (represented by their keys) in an ordered beacon object.
	// The function is thread-safe, utilizing read locks to prevent race conditions.
	//
	// Returns:
	// - int: The number of unique treasures stored in the beacon's treasuresByKeys map.
	//
	// Side Effects:
	// - Sets the 'initialized' flag of the beacon to 1, indicating that the beacon object has been accessed.
	//
	// Usage:
	// This function is useful in scenarios such as:
	// 1. Monitoring the total number of treasures in the beacon for system health and performance checks.
	// 2. Decision-making based on the current resource count in the beacon.
	// 3. Validating whether a bulk addition or removal operation has achieved the desired state.
	Count() int

	// PushManyFromMap adds multiple treasures to both the beacon's internal unordered map
	// (treasuresByKeys) and to its ordered slice (treasuresByOrder), if the beacon is set
	// to maintain an ordered list.
	//
	// The function is designed to be thread-safe, locking the beacon object to prevent
	// concurrent modifications that could result in data inconsistencies.
	//
	// Parameters:
	// - treasures map[string]treasure.Treasure: A map of treasures to be added.
	//   The keys serve as the unique identifiers for the treasures.
	//
	// Side Effects:
	// - The function modifies the internal map of the beacon object (treasuresByKeys) to
	//   include the new treasures.
	// - If the beacon is set to maintain an ordered list of treasures (isOrdered == true),
	//   the function also appends these new treasures to the internal ordered slice
	//   (treasuresByOrder).
	//
	// When to Use This Function:
	// 1. For batch operations where multiple treasures need to be added simultaneously
	//    for efficiency reasons.
	// 2. When initially populating the beacon object from an external data source,
	//    like a database or another API.
	// 3. During data synchronization tasks where the beacon's internal data structures
	//    need to be updated to match an external source.
	// 4. For any operations involving bulk transfers or modifications of treasures
	//    within or across beacon objects.
	PushManyFromMap(treasures map[string]treasure.Treasure)

	// Add inserts a new treasure into the beacon's internal data structures.
	// The function ensures thread-safety by using a mutex transaction to prevent concurrent
	// modifications. It only adds the treasure if it doesn't already exist in the beacon's
	// map (treasuresByKeys).
	//
	// Parameters:
	// - d treasure.Treasure: The treasure object to be added. The object must implement
	//   a GetKey() method that returns a unique identifier for the treasure.
	//
	// Side Effects:
	// - If the treasure's key is not already in the map, the function adds the treasure
	//   to the internal map of the beacon object (treasuresByKeys).
	// - If the beacon is set to maintain an ordered list (isOrdered == true),
	//   the function also appends the new treasure to the ordered slice (treasuresByOrder).
	//
	// When to Use This Function:
	// 1. When you need to insert a new treasure object into the beacon's internal data
	//    structures and you want to ensure that the object is unique (based on its key).
	// 2. During real-time operations where new treasures are discovered and need to
	//    be immediately added to the beacon.
	// 3. In scenarios where a specific order of the treasures is required, and the beacon
	//    is set to maintain an ordered list.
	Add(d treasure.Treasure)

	// Get retrieves a treasure object from the beacon's internal data structures based on
	// a provided unique key. The function ensures thread-safety by acquiring a read transaction
	// to prevent concurrent write operations.
	//
	// Parameters:
	// - key string: The unique identifier for the treasure object to be retrieved.
	//
	// Returns:
	// - d treasure.Treasure: The treasure object associated with the given key.
	//                        Returns nil if the key does not exist in the map.
	//
	// Side Effects:
	// - None.
	//
	// When to Use This Function:
	// 1. When you need to access a specific treasure object from the beacon's internal data
	//    structures for read-only operations or further processing.
	// 2. For real-time query functionalities where a fast, thread-safe access to a specific
	//    treasure object is required.
	// 3. During debugging or logging procedures where you need to quickly inspect a specific
	//    treasure object based on its unique key.
	Get(key string) (d treasure.Treasure)

	// GetManyFromOrderPosition retrieves a slice of treasure objects from the beacon's
	// ordered list, starting from a specified offset position and up to a specified limit.
	// This function ensures thread-safety by acquiring a read transaction to prevent concurrent
	// write operations.
	//
	// Parameters:
	// - from int: The offset index from where to start retrieving the treasure objects.
	// - limit int: The maximum number of treasure objects to retrieve.
	//
	// Returns:
	// - []treasure.Treasure: A slice containing the retrieved treasure objects.
	// - error: An error object if the operation is unsuccessful. Reasons for failure can
	//          include the beacon not being ordered or if the 'from' parameter exceeds
	//          the number of available elements.
	//
	// Side Effects:
	// - None.
	//
	// When to Use This Function:
	// 1. For paginated retrieval of treasures where you need to fetch a specific range
	//    of treasure objects from the ordered list.
	// 2. For partial loading or 'infinite scroll' functionalities in a frontend application.
	// 3. During logging or debugging procedures to inspect a range of treasure objects.
	// 4. When constructing reports or analytics and you need to access a specific subset
	//    of ordered treasure objects for calculations or summary.
	GetManyFromOrderPosition(orderPosition *OrderPosition) ([]treasure.Treasure, error)

	// GetManyFromKey retrieves a slice of treasure objects starting from a specified key
	// up to a given limit. The function sorts the returned treasures based on their creation time.
	// This function ensures thread-safety by acquiring a read transaction and releases individual treasures'
	// guards after cloning.
	//
	// Parameters:
	// - fromKey *string: The key from where to start retrieving treasure objects.
	//                     If nil, starts from the beginning.
	// - limit *int32: The maximum number of treasure objects to retrieve. If nil, the default limit is 100.
	//
	// Returns:
	// - []treasure.Treasure: A sorted slice containing the retrieved and cloned treasure objects.
	// - error: An error object if the operation is unsuccessful, e.g., if the beacon is not ordered.
	//
	// Side Effects:
	// - Locks and releases individual treasure guards for cloning.
	//
	// When to Use This Function:
	// 1. For filtered retrieval of treasures starting from a specific key in an ordered list.
	// 2. To clone and isolate specific treasures for localized modifications or computations.
	// 3. For constructing analytics reports where the dataset starts from a particular key.
	// 4. For incremental data loading scenarios in both front-end and back-end operations.
	// 5. During logging or debugging procedures to inspect a range of treasure objects based on a key.
	GetManyFromKey(fromKey *string, limit *int32) ([]treasure.Treasure, error)

	// FilterOrderedTreasures filters the treasures stored in a beacon object based on the provided filter function.
	// It returns a slice of treasures that meet the filter criteria and an error if the beacon is not ordered.
	//
	// Parameters:
	// - filterFunc: A callback function that takes a Treasure object as input and returns a boolean.
	//               Only the Treasure objects for which this function returns true will be included in the result.
	// - howMany: The maximum number of Treasure objects to include in the returned slice.
	// - remove: A boolean indicating whether to remove the filtered treasures from the beacon object.
	//
	// Returns:
	// - []Treasure: A slice of Treasure objects that meet the filter criteria.
	// - error: An error object if the beacon is not ordered, otherwise nil.
	//
	// Usage:
	// This function can be useful in various scenarios such as:
	// 1. Finding treasures that are overdue for maintenance and removing them.
	// 2. Quickly locating high-value treasures for a priority task.
	// 3. Extracting a subset of treasures based on certain characteristics (e.g., type, age, value).
	//
	// Thread-Safety:
	// The function is thread-safe, it locks the beacon object during its operation to prevent race conditions.
	//
	// Example:
	// filterFunc := func(t treasure.Treasure) bool {
	//     return t.Value > 10
	// }
	// treasures, err := myBeacon.FilterOrderedTreasures(filterFunc, 5, true)
	// if err != nil {
	//     log.Fatal(err)
	// }
	// fmt.Println("Filtered treasures:", treasures)
	//
	// Example 2:
	// Data Cleanup: Let's say you have a pool of treasures, and you want to clean up those that are not in active use or have been marked as obsolete. You can use this function to find and remove them.
	// filterObsolete := func(t treasure.Treasure) bool {
	//    return t.Status == "obsolete"
	// }
	// _, err := beaconObj.FilterOrderedTreasures(filterObsolete, 10, true)
	// if err != nil {
	//    log.Println("Error:", err)
	// }
	FilterOrderedTreasures(filterFunc func(treasureObj treasure.Treasure) bool, howMany int, remove bool) ([]treasure.Treasure, error)

	// Delete removes a treasure object with a specified key from both the treasuresByKeys map and
	// the treasuresByOrder slice, if ordering is enabled. The function ensures thread-safety by
	// acquiring a write transaction before performing deletion operations.
	//
	// Parameters:
	// - key string: The key of the treasure object to be deleted.
	//
	// Side Effects:
	// - Modifies the treasuresByKeys map by removing the specified key-value pair.
	// - Modifies the treasuresByOrder slice by removing the element with the specified key, if ordering is enabled.
	//
	// When to Use This Function:
	// 1. When you need to remove a specific treasure object identified by its key, either due to user action or some internal condition.
	// 2. To clean up resources or to maintain a cap on the number of stored treasures in certain scenarios.
	// 3. To facilitate dynamic updates to the stored data without restarting the application or service.
	// 4. In caching scenarios where you need to evict certain items based on custom logic or policies.
	// 5. During data synchronization tasks between different systems or services, where certain items need to be removed for consistency.
	Delete(key string)

	// ShiftOne removes a treasure object with a specified key from both the treasuresByKeys map and
	// the treasuresByOrder slice, and returns the removed treasure. The function ensures thread-safety by
	// acquiring a write transaction before performing the deletion and retrieval operations.
	//
	// Parameters:
	// - key string: The key of the treasure object to be deleted and returned.
	//
	// Returns:
	// - d treasure.Treasure: The deleted treasure object, if found. Otherwise, it returns nil.
	//
	// Side Effects:
	// - Modifies the treasuresByKeys map by removing the specified key-value pair.
	// - Modifies the treasuresByOrder slice by removing the element with the specified key, if ordering is enabled.
	//
	// When to Use This Function:
	// 1. When you need to both remove and retrieve a specific treasure object identified by its key.
	// 2. In scenarios where an element needs to be moved from one collection to another.
	// 3. When implementing deque (double-ended queue) behaviors, where an item can be removed from the front or back of the list.
	// 4. During data synchronization tasks between different systems or services, where items need to be processed and then removed.
	// 5. In caching scenarios where you need to evict certain items and move them to a different data structure or storage.
	ShiftOne(key string) (d treasure.Treasure)

	// ShiftMany removes a specified number of treasure objects from the treasuresByOrder slice
	// and the corresponding entries from the treasuresByKeys map. It returns the removed treasures
	// as a slice. Thread-safety is ensured via a write transaction.
	//
	// Parameters:
	// - howMany int: The number of treasures to remove and return.
	//
	// Returns:
	// - []treasure.Treasure: A slice of removed treasure objects.
	//
	// Side Effects:
	// - Modifies treasuresByKeys by removing keys corresponding to shifted treasures.
	// - Modifies treasuresByOrder by removing the shifted treasures.
	//
	// When to Use This Function:
	// 1. When you need to bulk-remove and retrieve a specific number of treasures.
	// 2. In queue-like scenarios, where the first 'howMany' elements should be processed and removed.
	// 3. When you need to move a set number of elements from one data structure to another.
	// 4. For implementing rate-limiting mechanisms or load balancers that distribute a certain number of tasks/items.
	// 5. For evicting a set number of items in a cache as part of a cache eviction strategy.
	ShiftMany(howMany int) []treasure.Treasure

	// ShiftExpired removes and returns a specified number of expired treasure objects
	// from the treasuresByOrder slice and the corresponding entries from the treasuresByKeys map.
	// Expired treasures are identified based on their expiration time. Thread safety is ensured via a write transaction.
	//
	// Parameters:
	// - howMany int: The maximum number of expired treasures to remove and return.
	//
	// Returns:
	// - []treasure.Treasure: A slice of removed, expired treasure objects.
	//
	// Side Effects:
	// - Modifies treasuresByKeys by removing keys corresponding to shifted expired treasures.
	// - Modifies treasuresByOrder by removing the shifted expired treasures.
	//
	// When to Use This Function:
	// 1. In scenarios where stale or outdated data should be removed.
	// 2. For implementing cache eviction based on time expiration.
	// 3. To free up resources or decrease memory footprint by removing expired data.
	// 4. For data archival or backup processes that operate on expired data before removal.
	// 5. When implementing rate-limiting or leasing systems where expired items should be processed separately.
	ShiftExpired(howMany int) []treasure.Treasure

	// CloneOrderedTreasures clones and returns all treasure objects from the treasuresByOrder slice.
	// Optionally, it also resets the internal state of the beacon based on the 'thenReset' flag.
	// Thread safety is ensured via a write transaction.
	//
	// Parameters:
	// - thenReset bool: Flag to determine whether or not to reset the internal state of the beacon.
	//
	// Returns:
	// - []treasure.Treasure: A clone of all the ordered treasure objects in the beacon.
	//
	// Side Effects:
	// - If thenReset is true, empties treasuresByOrder and clears treasuresByKeys map.
	//
	// When to Use This Function:
	// 1. To get a snapshot of the current state of treasures for external processing without affecting the internal state.
	// 2. To reset the internal state after cloning, useful for garbage collection or resource management.
	// 3. When implementing features like data export, data snapshot, or backup.
	// 4. For debugging and testing, where a non-mutating copy of data is required.
	// 5. In cases where you want to pass a copy of the internal data to another service or component without the risk of mutation.
	CloneOrderedTreasures(thenReset bool) []treasure.Treasure

	// CloneUnorderedTreasures clones and returns all treasure objects from the treasuresByKeys map.
	// Optionally, it also resets the internal state of the beacon based on the 'thenReset' flag.
	// Thread safety is ensured via a write transaction.
	//
	// Parameters:
	// - thenReset bool: Flag to determine whether or not to reset the internal state of the beacon.
	//
	// Returns:
	// - map[string]treasure.Treasure: A clone of all the unordered treasure objects in the beacon.
	//
	// Side Effects:
	// - If thenReset is true, empties treasuresByOrder and clears treasuresByKeys map.
	//
	// When to Use This Function:
	// 1. To get a snapshot of the current state of treasures for external processing without affecting the internal state.
	// 2. To reset the internal state after cloning, which can be useful for garbage collection or resource management.
	// 3. When implementing features like data analytics, exporting the data, or backup.
	// 4. For debugging and testing, where a non-mutating copy of data is essential.
	// 5. In scenarios where you want to pass a copy of the internal data to another service or component without the risk of mutation.
	CloneUnorderedTreasures(thenReset bool) map[string]treasure.Treasure

	// SetIsOrdered controls whether the beacon will maintain the order of treasures or not.
	// If set to true, the function makes sure the beacon uses the treasuresByOrder slice for storing and managing treasures.
	// Thread safety is ensured via a write transaction.
	//
	// Parameters:
	// - isOrdered bool: Flag indicating whether or not to maintain order of the treasures.
	//
	// Side Effects:
	// 1. If isOrdered is false, clears the treasuresByOrder slice.
	// 2. If isOrdered is true, copies the treasures from the unordered map to the ordered slice.
	//
	// When to Use This Function:
	// 1. To switch between ordered and unordered storage dynamically based on the requirements.
	// 2. For implementing features like sorting and prioritizing of treasures, enabling functionalities like First-In-First-Out or sorted access.
	// 3. When starting to add new features that require ordering, this function can be used to toggle that behavior on.
	// 4. As a configuration setting that might be tweaked during runtime or for different deployment scenarios.
	//
	// Use-Cases for Trendizz:
	// - When you need to perform ordered operations, like sorted exports or batch processing in a specific sequence, this function is crucial.
	// - During reporting or analytics where the sequence of treasures may carry important information or insights.
	SetIsOrdered(isOrdered bool)

	// IsOrdered checks whether the beacon is set to maintain the order of treasures or not.
	// It's a thread-safe operation, protected by a read transaction.
	//
	// Returns:
	// - bool: The value of the isOrdered flag.
	//
	// Side Effects:
	// - None.
	//
	// When to Use This Function:
	// 1. When you need to conditionally execute code that depends on whether the treasure storage is ordered or not.
	// 2. For logging or debugging purposes, to confirm the internal state of the beacon.
	// 3. As a pre-check before calling other functions that have different behaviors based on ordering.
	// 4. To understand system behavior and settings dynamically during runtime.
	//
	// Use-Cases for Trendizz:
	// - To decide whether to perform operations that rely on the order of elements. For example, in analytics or reporting, the ordered state can affect how data is aggregated or displayed.
	// - For diagnostics and system checks, this function can be used to confirm system behavior or during troubleshooting.
	IsOrdered() bool

	// IsInitialized checks if the beacon is initialized. It's a thread-safe operation, protected by a read transaction.
	//
	// Returns:
	// - bool: Returns true if initialized (b.initialized == 1), false otherwise.
	//
	// Side Effects:
	// - None.
	//
	// When to Use This Function:
	// 1. Before performing operations that should only be executed on an initialized beacon.
	// 2. For logging or debugging to know the initialization state of the beacon.
	// 3. As a condition in tests to ensure the beacon is initialized before running specific test cases.
	//
	// Use-Cases for Trendizz:
	// - To ensure that the system or certain modules/components only interact with an initialized beacon. This is crucial for ensuring data consistency and system integrity.
	// - For diagnostic purposes, you can use this function to understand the system's state at any point.
	IsInitialized() bool

	// SetInitialized sets the initialization status of the beacon object to true or false. This operation is atomic.
	//
	// Parameters:
	// - init: The initialization status to set, either true or false.
	//
	// Returns:
	// - None
	//
	// Side Effects:
	// - Updates the 'initialized' field in the beacon object.
	//
	// When to Use This Function:
	// 1. After the beacon has been fully set up and is ready for use, to signal to other parts of the system that it's safe to interact with this beacon.
	// 2. During testing, to simulate an uninitialized or initialized beacon to see how other components react.
	// 3. When shutting down or resetting the beacon, you may want to set it to uninitialized to prevent other operations.
	//
	// Use-Cases for Trendizz:
	// - Use it to set the initialization status as part of the boot-up sequence.
	// - Before taking the beacon out of service for maintenance or updates, set it to uninitialized to ensure that no unwanted operations are carried out on an inconsistent state.
	// - When dynamically adding or removing beacons, use this to set their initialization state appropriately.
	SetInitialized(init bool)

	// Reset resets the internal state of the beacon object. This clears all treasures and sets the initialization status to false (or uninitialized).
	//
	// Parameters:
	// - None
	//
	// Returns:
	// - None
	//
	// Side Effects:
	// - Resets the 'treasuresByKeys' map to an empty map.
	// - Sets the 'treasuresByOrder' slice to nil.
	// - Sets the 'initialized' flag to 0 (uninitialized).
	//
	// When to Use This Function:
	// 1. When you want to clean up the beacon's internal state for whatever reason (e.g., before re-initialization, before shutting down, or after a certain operation).
	// 2. During testing, to bring the beacon back to an initial state.
	// 3. To reset the beacon state when a fatal error occurs, to restart it clean.
	//
	// Use-Cases for Trendizz:
	// - If we're modifying our treasure storage logic or switching between ordered and unordered modes, this can act as a reset button.
	// - During scheduled maintenance or updates, you can reset the beacon and then initialize it with new data or configurations.
	// - If there's an issue and we detect that the beacon has reached an inconsistent state, reset and reinitialize.
	Reset()

	// IsExists checks if a treasure with the given key exists within the beacon.
	//
	// Parameters:
	// - key: The string identifier for the treasure.
	//
	// Returns:
	// - ContentTypeBoolean: true if the treasure exists, false otherwise.
	//
	// Side Effects:
	// - Sets the 'initialized' flag to 1 (true) before proceeding.
	//
	// When to Use This Function:
	// 1. To confirm the existence of a particular treasure before attempting to read or manipulate it.
	// 2. As a condition check in other methods that require the treasure to exist for their logic.
	// 3. To prevent duplicate entries by checking the existence first before adding a new treasure.
	//
	// Use-Cases for Trendizz:
	// - To verify the existence of a particular indexed page or content before performing search operations.
	// - To check if certain data is already indexed or not, hence optimizing the process of indexing.
	// - To improve error handling by not proceeding with operations that require an existing treasure.
	//
	// Note:
	// The function uses read-transaction (RLock) to ensure multiple read operations can occur simultaneously without blocking each other while preserving data integrity.
	IsExists(key string) bool

	// SortBy... Common Functionality:
	// 1. Each sorting function first locks the beacon instance using the internal mutex `mu` to ensure thread safety during the sorting operation.
	// 2. It then checks if the beacon is ordered (`isOrdered`). If it's not ordered, an error will be returned.
	// 3. Finally, the function uses Go's built-in `sort.Slice()` method to sort the `treasures` slice based on the given attribute.
	//
	// Use Cases:
	// - Dynamic Search Parameters: Allow users to sort results based on their preferred attributes.
	// - Analytics and Reporting: Easily generate sorted lists for further analysis.
	// - Optimized Data Access: Faster retrieval of data due to pre-sorted lists.
	// - Cache Strategy: Facilitate cache eviction strategies by sorting by expiration time.
	//
	// Common Error Handling:
	// - All sorting functions will return an error if the `isOrdered` flag is not set, ensuring uniform error handling across all sorting functions.
	//
	// Performance and Scalability:
	// - Proper implementation of these functions should not introduce significant performance overhead but will increase the codebase's maintainability and readability.
	//
	// Designed to facilitate the development and maintainability of Trendizz's core SaaS offering.

	SortByCreationTimeAsc() error
	SortByCreationTimeDesc() error
	SortByKeyAsc() error
	SortByKeyDesc() error
	SortByExpirationTimeAsc() error
	SortByExpirationTimeDesc() error
	SortByUpdateTimeAsc() error
	SortByUpdateTimeDesc() error

	SortByValueFloat32ASC() error
	SortByValueFloat32DESC() error
	SortByValueFloat64ASC() error
	SortByValueFloat64DESC() error

	SortByValueUint8ASC() error
	SortByValueUint8DESC() error
	SortByValueUint16ASC() error
	SortByValueUint16DESC() error
	SortByValueUint32ASC() error
	SortByValueUint32DESC() error
	SortByValueUint64ASC() error
	SortByValueUint64DESC() error

	SortByValueInt8ASC() error
	SortByValueInt8DESC() error
	SortByValueInt16ASC() error
	SortByValueInt16DESC() error
	SortByValueInt32ASC() error
	SortByValueInt32DESC() error
	SortByValueInt64ASC() error
	SortByValueInt64DESC() error

	SortByValueStringASC() error
	SortByValueStringDESC() error

	// Iterate goes through each treasure in the beacon's `treasuresByKeys` map and calls the given `iterFunc` with the treasure object as its parameter. It's designed to provide a unified approach for iterating over all the elements in a beacon instance while ensuring thread safety.
	//
	// How it Works:
	// 1. Locks the beacon instance for reading using the read-transaction (`RLock()`) from the internal mutex `mu` to prevent any writes during the iteration. The read-transaction allows multiple goroutines to read the beacon concurrently.
	// 2. Iterates over the treasures, invoking the provided `iterFunc` on each. If `iterFunc` returns `false`, the iteration will break immediately.
	// 3. Unlocks the beacon instance automatically using `defer`, ensuring that the transaction is released after the function returns.
	//
	// Usage Scenarios:
	// - Bulk Operations: Perform operations like logging, transformations, or validation on all treasures in a beacon.
	// - Analytics: Extract information or statistics about the current state of treasures in the beacon.
	// - Debugging: A convenient way to examine the current elements during debugging sessions.
	//
	// Considerations:
	// - The function is blocking and locks the entire beacon for reading, so consider the performance implications if you have a large number of treasures or if the `iterFunc` performs time-consuming operations.
	// - Use the `Clone` methods if you need to perform iterations without locking the beacon.
	//
	// Concurrency:
	// - Because the function uses a read-transaction, other goroutines can still read the beacon concurrently, but no writes will be allowed during the iteration.
	//
	// This function is crucial for implementing various features and capabilities in Trendizz's core SaaS offering, ensuring that we maintain the high level of quality and performance that our B2B customers expect.
	//
	Iterate(iterFunc func(treasureObj treasure.Treasure) bool, it IterationType)
}

type beacon struct {
	mu              sync.RWMutex
	treasuresByKeys map[string]treasure.Treasure
	// treasuresByOrder is used for storing Treasures in the order they were added to the beacon, or can be sorted by
	// other sorters like expiration time, creation time, etc...
	treasuresByOrder []treasure.Treasure
	// initialized is used for initializing the treasure only once
	// We use the initialized field to determine whether anything has used the beacon before.
	// Instantiation alone does not initialize the beacon, but its first use does. This is necessary because we need
	// to know if there was anything that wants to use the beacon, as the beacon needs to be built only in this case.
	initialized int32
	// isOrdered true if we want to keep the treasures in treasuresByOrder too.
	// The beacon will also use the treasuresByOrder slice for storing treasures. This becomes necessary when we want to sort
	// the treasures, whether based on the time they were added, or through more complex sorting such as by expiration date
	// etc... Don't forget to set the value to True, otherwise the beacon won't handle the treasuresByOrder array, and it will
	// always be empty!
	isOrdered bool
	//
	sortOrder SortOrder
}

type SortOrder int

const (
	SortByExpirationTimeAsc SortOrder = iota + 1
	SortByExpirationTimeDesc
	SortByCreatedAtAsc
	SortByCreatedAtDesc
	SortByModifiedAtAsc
	SortByModifiedAtDesc
)

// New returns a new beacon
func New() Beacon {
	return &beacon{
		treasuresByKeys: make(map[string]treasure.Treasure),
	}
}

// GetAll returns all the treasures in the beacon
func (b *beacon) GetAll() map[string]treasure.Treasure {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.treasuresByKeys
}

type IterationType int

const (
	// IterationTypeOrdered iteration by the ordered slice
	IterationTypeOrdered IterationType = iota + 1
	// IterationTypeKey iteration by the keys map
	IterationTypeKey
)

// Iterate iterates over the beacon and calls the iterFunc for each treasure
// The whole beacon is locked during the iteration so that no other goroutine can modify or read the beacon while we are
// iterating. Use the Clone methods if you want to iterate over the beacon without locking it.
func (b *beacon) Iterate(iterFunc func(treasureObj treasure.Treasure) bool, it IterationType) {

	b.mu.RLock()
	defer b.mu.RUnlock()

	if it == IterationTypeKey {
		for _, treasureObj := range b.treasuresByKeys {
			if !iterFunc(treasureObj) {
				break
			}
		}
		return
	} else if it == IterationTypeOrdered {
		for _, treasureObj := range b.treasuresByOrder {
			if !iterFunc(treasureObj) {
				break
			}
		}
		return
	}

	return

}

// Get returns the element with the given key
func (b *beacon) Get(key string) (d treasure.Treasure) {
	atomic.StoreInt32(&b.initialized, 1)
	b.mu.RLock()
	defer b.mu.RUnlock()
	if treasureObj, ok := b.treasuresByKeys[key]; ok {
		return treasureObj
	}
	return nil
}

// SetIsOrdered sets the isOrdered flag to true or false
// The beacon will also use the treasuresByOrder slice for storing treasures. This becomes necessary when we want to sort
// the treasures, whether based on the time they were added, or through more complex sorting such as by expiration date
// etc... SetIsOrdered automatically copies the unordered treasures to the ordered slice (reset it, then copy the unordered treasures)
func (b *beacon) SetIsOrdered(isOrdered bool) {

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.isOrdered == isOrdered {
		return
	}

	// clear the isOrdered slice if we don't want to keep the ordered treasures
	if !isOrdered {
		b.treasuresByOrder = nil // reset the ordered treasures slice
		b.isOrdered = isOrdered  // set the isOrdered flag
		return
	}

	// copy the unordered treasures to the ordered treasures
	for _, treasureObj := range b.treasuresByKeys {
		b.treasuresByOrder = append(b.treasuresByOrder, treasureObj)
	}

	b.isOrdered = isOrdered

}

// IsOrdered returns the isOrdered flag that we set when the beacon was created
func (b *beacon) IsOrdered() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.isOrdered
}

// SetInitialized set the initialized status to true or false
func (b *beacon) SetInitialized(init bool) {
	if init {
		atomic.StoreInt32(&b.initialized, 1)
		return
	}
	atomic.StoreInt32(&b.initialized, 0)
}

// IsInitialized returns the initialized flag
func (b *beacon) IsInitialized() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.initialized == 1
}

func (b *beacon) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.treasuresByKeys = make(map[string]treasure.Treasure)
	b.treasuresByOrder = nil
	atomic.StoreInt32(&b.initialized, 0)
}

// PushManyFromMap add elements to the main unordered map and the ordered slice too if,
// the ordered list is enabled
func (b *beacon) PushManyFromMap(treasures map[string]treasure.Treasure) {
	b.mu.Lock()
	defer b.mu.Unlock()
	maps.Copy(b.treasuresByKeys, treasures)
	// add elements to the ordered treasure if there is any ordered treasures
	if b.isOrdered {
		for _, treasureObj := range treasures {
			b.treasuresByOrder = append(b.treasuresByOrder, treasureObj)
		}
	}
}

// Add adds a new element to the beacon
func (b *beacon) Add(d treasure.Treasure) {
	atomic.StoreInt32(&b.initialized, 1)
	// add element if the key is not in the map
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.treasuresByKeys[d.GetKey()]; !ok {
		b.treasuresByKeys[d.GetKey()] = d
		if b.isOrdered {
			b.treasuresByOrder = append(b.treasuresByOrder, d)
		}
	}
}

// Delete removes the element with the given key
func (b *beacon) Delete(key string) {

	atomic.StoreInt32(&b.initialized, 1)

	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.treasuresByKeys[key]; ok {
		delete(b.treasuresByKeys, key)
	}

	//* delete from treasuresByOrder slice
	//* delete from desc slice
	if b.isOrdered {
		for index, treasureObj := range b.treasuresByOrder {
			if treasureObj.GetKey() == key {
				b.treasuresByOrder = append(b.treasuresByOrder[:index], b.treasuresByOrder[index+1:]...)
				break
			}
		}
	}

}

// IsExists checks if the key exists in the beacon
func (b *beacon) IsExists(key string) bool {

	atomic.StoreInt32(&b.initialized, 1)

	b.mu.RLock()
	defer b.mu.RUnlock()

	if _, ok := b.treasuresByKeys[key]; ok {
		return true
	}

	return false

}

func (b *beacon) Count() int {

	atomic.StoreInt32(&b.initialized, 1)

	b.mu.RLock()
	defer b.mu.RUnlock()

	return len(b.treasuresByKeys)

}

// ShiftOne removes the element with the given key and returns it
func (b *beacon) ShiftOne(key string) (d treasure.Treasure) {

	atomic.StoreInt32(&b.initialized, 1)

	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.treasuresByKeys[key]; ok {
		d = b.treasuresByKeys[key]
		delete(b.treasuresByKeys, key)

		if b.isOrdered {
			for indx, treasureObj := range b.treasuresByOrder {
				if treasureObj.GetKey() == key {
					b.treasuresByOrder = append(b.treasuresByOrder[:indx], b.treasuresByOrder[indx+1:]...)
					break
				}
			}
		}
	}
	return
}

// ShiftMany removes the element with the given numbers and returns them
func (b *beacon) ShiftMany(howMany int) []treasure.Treasure {

	atomic.StoreInt32(&b.initialized, 1)

	b.mu.Lock()
	defer b.mu.Unlock()

	var shiftedTreasures []treasure.Treasure
	var remainingTreasures []treasure.Treasure
	counter := 0
	for _, treasureObj := range b.treasuresByOrder {
		if counter < howMany {
			lockID := treasureObj.StartTreasureGuard(true)
			clonedTreasure := treasureObj.Clone(lockID)
			treasureObj.ReleaseTreasureGuard(lockID)
			shiftedTreasures = append(shiftedTreasures, clonedTreasure)
			delete(b.treasuresByKeys, treasureObj.GetKey())
			counter++
		} else {
			remainingTreasures = append(remainingTreasures, treasureObj)
		}
	}
	b.treasuresByOrder = remainingTreasures
	return shiftedTreasures

}

func (b *beacon) ShiftExpired(howMany int) []treasure.Treasure {

	atomic.StoreInt32(&b.initialized, 1)

	b.mu.Lock()
	defer b.mu.Unlock()

	var shiftedTreasures []treasure.Treasure
	var remainingTreasures []treasure.Treasure

	counter := 0
	for _, treasureObj := range b.treasuresByOrder {
		lockerID := treasureObj.StartTreasureGuard(true)
		if counter < howMany && treasureObj.GetExpirationTime() < time.Now().UTC().UnixNano() {
			clonedTreasure := treasureObj.Clone(lockerID)
			shiftedTreasures = append(shiftedTreasures, clonedTreasure)
			delete(b.treasuresByKeys, treasureObj.GetKey())
			counter++
		} else {
			remainingTreasures = append(remainingTreasures, treasureObj)
		}
		treasureObj.ReleaseTreasureGuard(lockerID)
	}
	b.treasuresByOrder = remainingTreasures
	return shiftedTreasures

}

// CloneOrderedTreasures returns the clone of all the orderedTreasures in the beacon
func (b *beacon) CloneOrderedTreasures(thenReset bool) []treasure.Treasure {

	atomic.StoreInt32(&b.initialized, 1)

	b.mu.Lock()
	defer b.mu.Unlock()

	// clone the slice because we don't want to expose the internal slice
	clone := make([]treasure.Treasure, len(b.treasuresByOrder))
	for index, treasureObj := range b.treasuresByOrder {
		lockerID := treasureObj.StartTreasureGuard(true)
		clone[index] = treasureObj.Clone(lockerID)
		treasureObj.ReleaseTreasureGuard(lockerID)
	}

	if thenReset {
		b.treasuresByOrder = nil
		b.treasuresByKeys = make(map[string]treasure.Treasure)
	}

	return clone

}

// CloneUnorderedTreasures returns the clone of all the unorderedTreasures in the beacon
func (b *beacon) CloneUnorderedTreasures(thenReset bool) map[string]treasure.Treasure {

	atomic.StoreInt32(&b.initialized, 1)

	b.mu.Lock()
	defer b.mu.Unlock()

	treasuresClone := make(map[string]treasure.Treasure)
	for key, value := range b.treasuresByKeys {
		guardID := value.StartTreasureGuard(true)
		treasuresClone[key] = value.Clone(guardID)
		value.ReleaseTreasureGuard(guardID)
	}

	if thenReset {
		b.treasuresByOrder = nil
		b.treasuresByKeys = make(map[string]treasure.Treasure)
	}

	return treasuresClone
}

type OrderPosition struct {
	From     int        // Start index (inclusive) for retrieval; half-open interval logic: includes From, excludes From+Limit.
	Limit    int        // Maximum number of elements to return. If 0, returns all elements from From to the end. The range is [From, From+Limit).
	FromTime *time.Time // Optional: filter treasures with creation/expiration time >= FromTime.
	ToTime   *time.Time // Optional: filter treasures with creation/expiration time < ToTime. The time range is half-open: [FromTime, ToTime).
}

// GetManyFromOrderPosition retrieves a slice of treasure objects from the beacon's ordered list,
// starting from a specified offset position and up to a specified limit. Optionally, it can filter
// treasures by a time range if FromTime or ToTime is provided in the OrderPosition.
// The function is thread-safe, acquiring a read lock to prevent concurrent write operations.
//
// Parameters:
//   - orderPosition *OrderPosition: Specifies the offset (From), limit (Limit), and optional time range (FromTime, ToTime).
//
// Returns:
//   - []treasure.Treasure: A slice containing the retrieved treasure objects.
//   - error: An error if the beacon is not ordered, otherwise nil.
//
// Side Effects:
//   - Sets the 'initialized' flag to 1, indicating the beacon has been accessed.
//
// Usage Scenarios:
//  1. Paginated retrieval of treasures from the ordered list.
//  2. Partial loading or infinite scroll functionalities.
//  3. Filtering treasures by creation or expiration time.
//  4. Reporting or analytics requiring a subset of ordered treasures.
//
// Example:
//
//	pos := &OrderPosition{From: 10, Limit: 5}
//	treasures, err := beacon.GetManyFromOrderPosition(pos)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println("Treasures:", treasures)
func (b *beacon) GetManyFromOrderPosition(orderPosition *OrderPosition) ([]treasure.Treasure, error) {
	atomic.StoreInt32(&b.initialized, 1)

	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.isOrdered {
		return nil, errors.New("beacon is not ordered")
	}

	// Validate and set initial bounds
	startIdx := 0
	endIdx := len(b.treasuresByOrder) - 1

	// Apply time filtering if specified
	if orderPosition.FromTime != nil || orderPosition.ToTime != nil {
		startIdx, endIdx = b.findTimeRangeBounds(orderPosition.FromTime, orderPosition.ToTime)
		if endIdx < startIdx || startIdx < 0 {
			return []treasure.Treasure{}, nil
		}
	}

	// Apply offset
	actualStart := startIdx + orderPosition.From
	if actualStart > endIdx {
		return []treasure.Treasure{}, nil
	}

	// Apply limit
	var actualEnd int
	if orderPosition.Limit == 0 {
		actualEnd = endIdx
	} else {
		actualEnd = actualStart + orderPosition.Limit - 1
		if actualEnd > endIdx {
			actualEnd = endIdx
		}
	}

	// Prepare result slice
	resultSize := actualEnd - actualStart + 1
	if resultSize <= 0 {
		return []treasure.Treasure{}, nil
	}

	result := make([]treasure.Treasure, resultSize)
	for i := 0; i < resultSize; i++ {
		result[i] = b.treasuresByOrder[actualStart+i]
	}

	return result, nil
}

// getTimestampFromTreasure extracts the relevant timestamp based on sort order
func (b *beacon) getTimestampFromTreasure(t treasure.Treasure) int64 {
	switch b.sortOrder {
	case SortByExpirationTimeAsc, SortByExpirationTimeDesc:
		return t.GetExpirationTime()
	case SortByCreatedAtAsc, SortByCreatedAtDesc:
		return t.GetCreatedAt()
	case SortByModifiedAtAsc, SortByModifiedAtDesc:
		return t.GetModifiedAt()
	default:
		return 0
	}
}

// findTimeRangeBounds locates the inclusive start and end indices in the ordered slice
// for treasures whose timestamp falls within a half-open time window [fromTime, toTime).
// Half-open semantics:
// - fromTime is inclusive: ts >= fromTime.
// - toTime is exclusive: ts < toTime. If toTime is nil, the upper bound is unbounded.
// - If fromTime is nil, the lower bound is unbounded.
//
// Ordering assumptions:
//   - b.treasuresByOrder is already sorted by the timestamp selected via b.sortOrder
//     (created, modified, or expiration), either ascending or descending; the search adapts.
//   - Timestamps are obtained via getTimestampFromTreasure.
//
// Returns:
// - (startIdx, endIdx) where both are inclusive. If no items match, returns (0, -1).
//
// Complexity:
// - O(log n) per boundary using binary search; overall O(log n).
//
// Concurrency:
// - Caller must hold the appropriate lock (the public caller acquires an RLock).
func (b *beacon) findTimeRangeBounds(fromTime, toTime *time.Time) (int, int) {
	n := len(b.treasuresByOrder)
	if n == 0 {
		return 0, -1
	}

	var fromNano, toNano int64
	if fromTime != nil {
		fromNano = fromTime.UTC().UnixNano()
	}
	if toTime != nil {
		toNano = toTime.UTC().UnixNano()
	}

	isAscending := b.sortOrder == SortByExpirationTimeAsc ||
		b.sortOrder == SortByCreatedAtAsc ||
		b.sortOrder == SortByModifiedAtAsc

	startIdx := 0
	endIdx := n - 1

	if isAscending {
		// Asc: start = first idx with ts >= fromNano
		if fromTime != nil {
			l, r := 0, n
			for l < r {
				m := l + (r-l)/2
				if b.getTimestampFromTreasure(b.treasuresByOrder[m]) < fromNano {
					l = m + 1
				} else {
					r = m
				}
			}
			startIdx = l
		}

		// Asc: end = last idx with ts < toNano (exclusive upper bound)
		if toTime != nil {
			l, r := 0, n
			for l < r {
				m := l + (r-l)/2
				if b.getTimestampFromTreasure(b.treasuresByOrder[m]) < toNano {
					l = m + 1
				} else {
					r = m
				}
			}
			endIdx = l - 1
		}
	} else {
		// Descending order:
		// For ts decreasing with index:
		// - start = first idx with ts < toNano (exclusive upper bound)
		// - end   = last idx with ts >= fromNano (inclusive lower bound)

		// Desc: start = first idx where ts < toNano
		if toTime != nil {
			l, r := 0, n
			for l < r {
				m := l + (r-l)/2
				if b.getTimestampFromTreasure(b.treasuresByOrder[m]) < toNano {
					r = m
				} else {
					l = m + 1
				}
			}
			startIdx = l
		}

		// Desc: end = last idx where ts >= fromNano
		if fromTime != nil {
			l, r := 0, n
			for l < r {
				m := l + (r-l)/2
				if b.getTimestampFromTreasure(b.treasuresByOrder[m]) < fromNano {
					r = m
				} else {
					l = m + 1
				}
			}
			endIdx = l - 1
		}
	}

	// Normalize and validate bounds
	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx >= n {
		endIdx = n - 1
	}
	if startIdx > endIdx || startIdx >= n || endIdx < 0 {
		return 0, -1
	}

	return startIdx, endIdx
}

// GetManyFromKey returns the elements with the given offset and limit
func (b *beacon) GetManyFromKey(fromKey *string, limit *int32) ([]treasure.Treasure, error) {

	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.isOrdered {
		return nil, errors.New("beacon is not ordered")
	}

	counterLimit := int32(100)
	if limit != nil {
		counterLimit = *limit
	}

	// get the treasures from the orderedTreasures
	var selectedTreasures []treasure.Treasure
	counter := int32(0)
	foundKey := false

	for _, t := range b.treasuresByOrder {
		// if the fromKey is not nil, skip the treasure until the fromKey is found
		if !foundKey && (fromKey != nil && *fromKey != t.GetKey()) {
			continue
		}
		foundKey = true
		counter++
		selectedTreasures = append(selectedTreasures, t)
		if counter == counterLimit {
			break
		}
	}

	return selectedTreasures, nil

}

// FilterOrderedTreasures function for filtering the ordered treasures
func (b *beacon) FilterOrderedTreasures(filterFunc func(treasureObj treasure.Treasure) bool, howMany int, remove bool) ([]treasure.Treasure, error) {

	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.isOrdered {
		return nil, errors.New("beacon is not ordered")
	}

	var filteredTreasures []treasure.Treasure
	counter := 0

	for index, treasureObj := range b.treasuresByOrder {
		if counter == howMany {
			break
		}
		if filterFunc(treasureObj) {
			filteredTreasures = append(filteredTreasures, treasureObj)
			// remove the item from the original slice
			// and from the map too
			if remove {
				delete(b.treasuresByKeys, treasureObj.GetKey())
				b.treasuresByOrder = append(b.treasuresByOrder[:index], b.treasuresByOrder[index+1:]...)
			}
			counter++
		}
	}
	return filteredTreasures, nil
}

// SortByCreationTimeAsc sorts the orderedTreasures by the creation time ascending
func (b *beacon) SortByCreationTimeAsc() error {

	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}

	b.sortOrder = SortByCreatedAtAsc
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		return b.treasuresByOrder[k].GetCreatedAt() < b.treasuresByOrder[l].GetCreatedAt()
	})

	return nil

}

// SortByCreationTimeDesc sorts the orderedTreasures by the creation time descending
func (b *beacon) SortByCreationTimeDesc() error {

	b.mu.Lock()
	defer b.mu.Unlock()

	b.sortOrder = SortByCreatedAtDesc
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		return b.treasuresByOrder[k].GetCreatedAt() > b.treasuresByOrder[l].GetCreatedAt()
	})
	return nil
}

// SortByKeyAsc sorts the orderedTreasures by the key ascending
func (b *beacon) SortByKeyAsc() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		return b.treasuresByOrder[k].GetKey() < b.treasuresByOrder[l].GetKey()
	})
	return nil
}

// SortByKeyDesc sorts the orderedTreasures by the key descending
func (b *beacon) SortByKeyDesc() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		return b.treasuresByOrder[k].GetKey() > b.treasuresByOrder[l].GetKey()
	})
	return nil
}

func (b *beacon) SortByExpirationTimeAsc() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}

	b.sortOrder = SortByExpirationTimeAsc
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		return b.treasuresByOrder[k].GetExpirationTime() < b.treasuresByOrder[l].GetExpirationTime()
	})
	return nil
}

func (b *beacon) SortByExpirationTimeDesc() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	b.sortOrder = SortByExpirationTimeDesc
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		return b.treasuresByOrder[k].GetExpirationTime() > b.treasuresByOrder[l].GetExpirationTime()
	})
	return nil
}
func (b *beacon) SortByUpdateTimeAsc() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	b.sortOrder = SortByModifiedAtAsc
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		return b.treasuresByOrder[k].GetModifiedAt() < b.treasuresByOrder[l].GetModifiedAt()
	})
	return nil
}
func (b *beacon) SortByUpdateTimeDesc() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	b.sortOrder = SortByModifiedAtDesc
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		return b.treasuresByOrder[k].GetModifiedAt() > b.treasuresByOrder[l].GetModifiedAt()
	})
	return nil
}

func (b *beacon) SortByValueFloat32ASC() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		kVal, err := b.treasuresByOrder[k].GetContentFloat32()
		if err != nil {
			return false
		}
		lVal, err := b.treasuresByOrder[l].GetContentFloat32()
		if err != nil {
			return false
		}
		return kVal < lVal
	})
	return nil
}
func (b *beacon) SortByValueFloat32DESC() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		kVal, err := b.treasuresByOrder[k].GetContentFloat32()
		if err != nil {
			return false
		}
		lVal, err := b.treasuresByOrder[l].GetContentFloat32()
		if err != nil {
			return false
		}
		return kVal > lVal
	})
	return nil
}
func (b *beacon) SortByValueFloat64ASC() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		kVal, err := b.treasuresByOrder[k].GetContentFloat64()
		if err != nil {
			return false
		}
		lVal, err := b.treasuresByOrder[l].GetContentFloat64()
		if err != nil {
			return false
		}
		return kVal < lVal
	})
	return nil
}
func (b *beacon) SortByValueFloat64DESC() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		kVal, err := b.treasuresByOrder[k].GetContentFloat64()
		if err != nil {
			return false
		}
		lVal, err := b.treasuresByOrder[l].GetContentFloat64()
		if err != nil {
			return false
		}
		return kVal > lVal
	})
	return nil
}

func (b *beacon) SortByValueUint8ASC() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		kVal, err := b.treasuresByOrder[k].GetContentUint8()
		if err != nil {
			return false
		}
		lVal, err := b.treasuresByOrder[l].GetContentUint8()
		if err != nil {
			return false
		}
		return kVal < lVal
	})
	return nil
}
func (b *beacon) SortByValueUint8DESC() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		kVal, err := b.treasuresByOrder[k].GetContentUint8()
		if err != nil {
			return false
		}
		lVal, err := b.treasuresByOrder[l].GetContentUint8()
		if err != nil {
			return false
		}
		return kVal > lVal
	})
	return nil
}

func (b *beacon) SortByValueUint16ASC() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		kVal, err := b.treasuresByOrder[k].GetContentUint16()
		if err != nil {
			return false
		}
		lVal, err := b.treasuresByOrder[l].GetContentUint16()
		if err != nil {
			return false
		}
		return kVal < lVal
	})
	return nil
}
func (b *beacon) SortByValueUint16DESC() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		kVal, err := b.treasuresByOrder[k].GetContentUint16()
		if err != nil {
			return false
		}
		lVal, err := b.treasuresByOrder[l].GetContentUint16()
		if err != nil {
			return false
		}
		return kVal > lVal
	})
	return nil
}

func (b *beacon) SortByValueUint32ASC() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		kVal, err := b.treasuresByOrder[k].GetContentUint32()
		if err != nil {
			return false
		}
		lVal, err := b.treasuresByOrder[l].GetContentUint32()
		if err != nil {
			return false
		}
		return kVal < lVal
	})
	return nil
}
func (b *beacon) SortByValueUint32DESC() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		kVal, err := b.treasuresByOrder[k].GetContentUint32()
		if err != nil {
			return false
		}
		lVal, err := b.treasuresByOrder[l].GetContentUint32()
		if err != nil {
			return false
		}
		return kVal > lVal
	})
	return nil
}

func (b *beacon) SortByValueUint64ASC() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		kVal, err := b.treasuresByOrder[k].GetContentUint64()
		if err != nil {
			return false
		}
		lVal, err := b.treasuresByOrder[l].GetContentUint64()
		if err != nil {
			return false
		}
		return kVal < lVal
	})
	return nil
}
func (b *beacon) SortByValueUint64DESC() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		kVal, err := b.treasuresByOrder[k].GetContentUint64()
		if err != nil {
			return false
		}
		lVal, err := b.treasuresByOrder[l].GetContentUint64()
		if err != nil {
			return false
		}
		return kVal > lVal
	})
	return nil
}

func (b *beacon) SortByValueInt8ASC() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		kVal, err := b.treasuresByOrder[k].GetContentInt8()
		if err != nil {
			return false
		}
		lVal, err := b.treasuresByOrder[l].GetContentInt8()
		if err != nil {
			return false
		}
		return kVal < lVal
	})
	return nil
}
func (b *beacon) SortByValueInt8DESC() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		kVal, err := b.treasuresByOrder[k].GetContentInt8()
		if err != nil {
			return false
		}
		lVal, err := b.treasuresByOrder[l].GetContentInt8()
		if err != nil {
			return false
		}
		return kVal > lVal
	})
	return nil
}
func (b *beacon) SortByValueInt16ASC() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		kVal, err := b.treasuresByOrder[k].GetContentInt16()
		if err != nil {
			return false
		}
		lVal, err := b.treasuresByOrder[l].GetContentInt16()
		if err != nil {
			return false
		}
		return kVal < lVal
	})
	return nil
}
func (b *beacon) SortByValueInt16DESC() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		kVal, err := b.treasuresByOrder[k].GetContentInt16()
		if err != nil {
			return false
		}
		lVal, err := b.treasuresByOrder[l].GetContentInt16()
		if err != nil {
			return false
		}
		return kVal > lVal
	})
	return nil
}
func (b *beacon) SortByValueInt32ASC() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		kVal, err := b.treasuresByOrder[k].GetContentInt32()
		if err != nil {
			return false
		}
		lVal, err := b.treasuresByOrder[l].GetContentInt32()
		if err != nil {
			return false
		}
		return kVal < lVal
	})
	return nil
}
func (b *beacon) SortByValueInt32DESC() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		kVal, err := b.treasuresByOrder[k].GetContentInt32()
		if err != nil {
			return false
		}
		lVal, err := b.treasuresByOrder[l].GetContentInt32()
		if err != nil {
			return false
		}
		return kVal > lVal
	})
	return nil
}

func (b *beacon) SortByValueInt64ASC() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}

	// Extract values in advance, and throw an error if any of them are invalid.
	type item struct {
		value int64
		t     treasure.Treasure
	}

	items := make([]item, 0, len(b.treasuresByOrder))
	for i, t := range b.treasuresByOrder {
		v, err := t.GetContentInt64()
		if err != nil {
			return fmt.Errorf("cannot sort ascending: index %d, key %q is not an int64: %w", i, t.GetKey(), err)
		}
		items = append(items, item{value: v, t: t})
	}

	// ordering the items by value
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].value < items[j].value
	})

	// load back all ordered items
	for i, it := range items {
		b.treasuresByOrder[i] = it.t
	}

	return nil
}

func (b *beacon) SortByValueInt64DESC() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}

	type item struct {
		value int64
		t     treasure.Treasure
	}

	items := make([]item, 0, len(b.treasuresByOrder))
	for i, t := range b.treasuresByOrder {
		v, err := t.GetContentInt64()
		if err != nil {
			return fmt.Errorf("cannot sort descending: index %d, key %q is not an int64: %w", i, t.GetKey(), err)
		}
		items = append(items, item{value: v, t: t})
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].value > items[j].value
	})

	for i, it := range items {
		b.treasuresByOrder[i] = it.t
	}

	return nil
}

func (b *beacon) SortByValueStringASC() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		kVal, err := b.treasuresByOrder[k].GetContentString()
		if err != nil {
			return false
		}
		lVal, err := b.treasuresByOrder[l].GetContentString()
		if err != nil {
			return false
		}
		return kVal < lVal
	})
	return nil
}
func (b *beacon) SortByValueStringDESC() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isOrdered {
		return errors.New("the beacon is not ordered")
	}
	sort.Slice(b.treasuresByOrder, func(k, l int) bool {
		kVal, err := b.treasuresByOrder[k].GetContentString()
		if err != nil {
			return false
		}
		lVal, err := b.treasuresByOrder[l].GetContentString()
		if err != nil {
			return false
		}
		return kVal > lVal
	})
	return nil
}
