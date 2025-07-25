package swamp

import (
	"context"
	"errors"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/beacon"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/metadata"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/guard"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/vigil"
	"github.com/hydraide/hydraide/app/name"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

const (
	ErrorValueIsNotInt   = "the value is not an integer"
	ErrorValueIsNotFloat = "the value is not a float"
)

type Swamp interface {

	// Vigil handler for the swamp
	vigil.Vigil

	// GetMetadata visszaadja a Metaadat objektumát a swampnak
	GetMetadata() metadata.Metadata

	// WaitForGracefulClose Wait for the swamp to close gracefully and writes the treasures to the filesystem
	// this is a blocker function and any threads can be subscribed to this function
	WaitForGracefulClose(ctx context.Context) error

	// CountTreasuresWaitingForWriter returns the number of treasures waiting for the writer to write them to the filesystem.
	CountTreasuresWaitingForWriter() int

	// CreateTreasure creates a single "Treasure" object that can be populated with data.
	//
	// This function takes a key string as a parameter to uniquely identify the treasure once it's stored in the Swamp.
	// You can create a new Treasure object using this function, populate it with data using the methods provided by the
	// `treasure.Treasure` interface, and then save it in the Swamp using the `SaveTreasure` function.
	//
	// The function creates a Treasure object and sets only its key. If necessary, all other parameter settings depend
	// on the developer and business logic.
	//
	// The function does not save the Treasure to any swamp, it simply creates it as a standalone object that exists
	// only in memory. Saving to a swamp is left to the developer and the business logic to decide.
	//
	// Example:
	//     // Start a Vigil to prevent the Swamp from shutting down
	//     swamp.BeginVigil()
	//
	//     // Create a Treasure object with a unique key
	//     myTreasure := swamp.CreateTreasure("someKey")
	//	   // Populate the treasure with data
	//		...
	//
	//     // Check the status of the treasure if you want
	//
	//     // Release the Vigil. This is important to unlock the Swamp and allow it to shut down.
	//     swamp.CeaseVigil()
	//
	// Returns:
	// - A `treasure.Treasure` interface representing a treasure that can be populated with data and stored in the Swamp.
	//
	// Use-cases:
	// 1. Creating a new Treasure object with a unique key.
	// 2. Populating the treasure with data before storing it in the Swamp.
	CreateTreasure(key string) treasure.Treasure

	// GetTreasure retrieves a single "Treasure" from a "Swamp" by its unique key.
	//
	// This function takes a key string as a parameter, which uniquely identifies the desired treasure within the Swamp.
	// It returns a `treasure.Treasure` object representing the retrieved treasure and an error if the retrieval fails,
	// or the treasure doesn't exist.
	//
	// Real-world use-case:
	//    Fetching specific user details for profile display.
	//
	// Example:
	//     key := "user123"
	//
	// 	   swamp.BeginVigil()
	//     retrievedTreasure, err := swamp.GetTreasure(key)
	//	   swamp.CeaseVigil()
	//
	//     if err != nil {
	//         log.Println("Failed to retrieve the treasure:", err)
	//     } else {
	//         log.Printf("treasure: %+v\n", retrievedTreasure)
	//     }
	//
	// Returns:
	// - A `treasure.Treasure` object representing the retrieved treasure.
	// - An error if the retrieval fails, because the treasure does not exist, which can be checked with `err != nil`.
	//
	// Use-cases:
	// 1. Fetching specific data or resources from the Swamp using a unique key.
	// 2. Real-world scenarios such as retrieving user details for profile display.
	GetTreasure(key string) (treasure treasure.Treasure, err error)

	// GetAll retrieves all "Treasures" from a "Swamp."
	//
	// This function fetches all treasures from the Swamp and returns them as a map of key-value pairs, where the key is the
	// unique key of the treasure and the value is the treasure itself.
	//
	// Real-world use-case:
	// Fetching all products for a product listing page.
	//
	// Example:
	//     swamp.BeginVigil()
	//     retrievedTreasures := swamp.GetAll()
	//     swamp.CeaseVigil()
	//
	//     if err != nil {
	//         log.Println("Failed to retrieve treasures:", err)
	//     } else {
	//         for _, treasure := range retrievedTreasures {
	//             ... // Do something with the treasure
	//         }
	//     }
	//
	// Returns:
	// - A map of key-value pairs representing the retrieved treasures.
	GetAll() map[string]treasure.Treasure

	// GetTreasuresByBeacon retrieves one or more "Treasures" from a "Swamp" based on a selected Beacon (index).
	//
	// This function allows you to query treasures from a Swamp using a Beacon, which is an index that helps in efficient retrieval.
	// If the specified Beacon doesn't exist, the method will dynamically create it during the first call. Beacons are designed
	// to be memory-efficient and exist in memory only when they are actively used, providing immediate access after the initial
	// "cold-start."
	//
	// Important Note: When working with a large dataset and using Beacons, it's advisable to keep the Swamp open for as long as
	// possible. Closing the Swamp will remove the Beacon from memory, necessitating a new "cold-start" during the next query.
	//
	// Parameters:
	// - beaconType (BeaconType): The type of Beacon to use for sorting treasures. It can be CreationTime, ExpirationTime, UpdateTime,
	//   ValueInt, or ValueFloat, depending on your requirements.
	// - beaconOrderType (BeaconOrder): The sorting order for the Beacon, which can be ascending (IndexOrderAsc) or descending (IndexOrderDesc).
	// - from (int): The starting position to begin retrieving treasures based on the Beacon.
	// - limit (int32): The maximum number of treasures to retrieve.
	// - delete (bool): If true, the retrieved treasures will be deleted from the Swamp after retrieval. If false,
	//   the treasures will remain in the Swamp.
	//
	// Real-world Examples:
	// - Example 1: In a stock trading application, you could use the `ValueFloat` Beacon to quickly retrieve stocks that have
	//   reached a certain price for immediate buying or selling actions.
	//
	//     swampName := name.Name("stock_swamp")
	//     beaconType := BeaconTypeValueFloat64
	//     beaconOrderType := IndexOrderDesc
	//     startingPosition := 0
	//     maxTreasuresToRetrieve := 10
	//     deleteRetrievedTreasures := false
	//
	//	   swampName.BeginVigil()
	//     retrievedStocks, err := swampName.GetTreasuresByBeacon(beaconType, beaconOrderType, startingPosition, maxTreasuresToRetrieve, deleteRetrievedTreasures)
	//     swampName.CeaseVigil()
	//
	//     if err != nil {
	//         log.Println("Error retrieving stocks:", err)
	//     }
	//
	//     // Process the retrieved stocks as needed.
	//     // ...
	//
	// - Example 2: In a content management system, you could use the `CreationTime` Beacon to fetch articles that were created
	//   within a certain time frame for auditing or analytics.
	//
	//     swampName := name.Name("content_swamp")
	//     beaconType := BeaconTypeCreationTime
	//     beaconOrderType := IndexOrderAsc
	//     startingPosition := 0
	//     maxTreasuresToRetrieve := 20
	//     deleteRetrievedTreasures := false
	//	   swampName.BeginVigil()
	//     retrievedArticles, err := swampName.GetTreasuresByBeacon(beaconType, beaconOrderType, startingPosition, maxTreasuresToRetrieve, deleteRetrievedTreasures)
	//     swampName.CeaseVigil()
	//     if err != nil {
	//         log.Println("Error retrieving articles:", err)
	//     }
	//
	//     // Process the retrieved articles as needed.
	//     // ...
	//
	// Returns:
	// ([]treasure.Treasure): A slice of retrieved treasures based on the specified Beacon and parameters.
	// (error): An error if any issues occur during the retrieval process.
	//
	// Use-cases:
	// 1. Efficient retrieval of treasures based on specific criteria using Beacons.
	// 2. Real-time data querying and processing for applications with dynamic data.
	GetTreasuresByBeacon(beaconType BeaconType, beaconOrderType BeaconOrder, from int32, limit int32) ([]treasure.Treasure, error)

	// CloneAndDeleteExpiredTreasures retrieves one or more expired Treasures from the Swamp based on their expiration
	// time and removes them. , Use this function carefully as it deletes the Treasures from the Swamp.
	//
	// Parameters:
	// - howMany (int32): The maximum number of expired Treasures to retrieve and remove from the Swamp.
	//
	// Real-world Examples:
	// - Example 1: Handling Time-Sensitive Tasks
	//   -----------------------------------------
	//   This function is particularly useful for handling tasks that can only be executed after a certain amount of time has passed.
	//   For instance, you could use this function to manage a queue of tasks that are time-sensitive.
	//   Once the time condition is met, the task can be safely and efficiently retrieved from the Swamp for execution.
	//
	//     howMany := int32(5) // Retrieve and remove up to 5 expired Treasures.
	//     yourSwamp.BeginVigil()
	//     expiredTasks, err := yourSwamp.CloneAndDeleteExpiredTreasures(howMany)
	//     yourSwamp.CeaseVigil()
	//     if err != nil {
	//         log.Println("Error retrieving and removing expired tasks:", err)
	//     }
	//
	//   // Process the retrieved and removed expired tasks.
	//   // ...
	//
	// - Example 2: Scheduled Email Sending in an Email Marketing Platform
	//   ---------------------------------------------------------------
	//   Imagine you are developing an email marketing platform where users can schedule emails to be sent at a specific time.
	//   You could use GetAndDeleteExpiredTreasures to store these scheduled emails in the Swamp with an ExpirationTime set to the time they should be sent.
	//   Once the ExpirationTime is reached, the email can be retrieved and sent automatically.
	//   This ensures that emails are sent exactly when they are supposed to, keeping your database organized and your operations efficient.
	//
	//     howMany := int32(10) // Retrieve and remove up to 10 expired emails.
	//     emailSwamp.BeginVigil()
	//     expiredEmails, err := emailSwamp.CloneAndDeleteExpiredTreasures(howMany)
	//     emailSwamp.CeaseVigil()
	//     if err != nil {
	//         log.Println("Error retrieving and removing expired emails:", err)
	//     }
	//
	//   // Send the retrieved and removed expired emails.
	//   // ...
	//
	// Returns:
	// ([]treasure.Treasure): A slice of retrieved and removed expired Treasures from the Swamp.
	// (error): An error if any issues occur during the retrieval and removal process. The error will be nil, if no
	//  expired Treasures are found.
	//
	// Use-cases:
	// 1. Handling time-sensitive tasks and actions that become valid or relevant after a certain amount of time has passed.
	// 2. Automating scheduled operations, such as sending emails or notifications, based on expiration times.
	CloneAndDeleteExpiredTreasures(howMany int32) ([]treasure.Treasure, error)

	// DeleteTreasure deletes a single "Treasure" from a "Swamp" by its unique key.
	//
	// Parameters:
	// - key (string): The unique key of the Treasure to be deleted from the Swamp.
	// - shadowDelete (bool): If true, the Treasure will be shadow-deleted, meaning it will be marked as deleted but not physically removed.
	//                        If false, the Treasure will be permanently deleted from the Swamp.
	//
	// Real-world use-case:
	// - Deleting a user account upon request. In scenarios where users request to delete their accounts, this function can be used
	//   to permanently remove the user's data (Treasure) from the Swamp.
	//
	// Example Usage:
	// ----------------
	// Suppose you have a Swamp named "user_data" where each Treasure represents user data.
	// When a user requests to delete their account, you can use DeleteTreasure to delete their data from the Swamp.
	//
	//     userKeyToDelete := "user123" // The unique key of the user to be deleted.
	//     userDataSwamp.BeginVigil()
	//     err := userDataSwamp.DeleteTreasure(userKeyToDelete, true) // the user account is shadow-deleted so that it can be recovered if needed.
	//     userDataSwamp.CeaseVigil()
	//     if err != nil {
	//         log.Println("Error deleting user data:", err)
	//     }
	//
	// Returns:
	// (error): An error if any issues occur during the deletion process.
	//
	// Use-cases:
	// 1. Securely deleting specific data entries, such as user accounts or records, from the Swamp.
	DeleteTreasure(key string, shadowDelete bool) error

	// CloneTreasures returns a clone of the main beaconKey map.
	//
	// Real-world use-case:
	// - This function is used when we want to copy the treasures from one swamp to another.
	//   It can be particularly useful for data synchronization or migration scenarios, where you need to duplicate
	//   the treasures stored in one Swamp and transfer them to another.
	//
	// Important! When you make any modifications to the cloned Treasures, they will not be reflected in the source
	// Treasure. Cloning creates completely new Treasure objects that have separate lives from the original Treasures.
	//
	// Example Usage:
	// ----------------
	// Suppose you have two Swamps, "sourceSwamp" and "destinationSwamp," and you want to copy treasures from the source Swamp to the destination Swamp.
	// You can use CloneTreasures to clone the treasures from the source Swamp and then add them to the destination Swamp.
	//
	//	   // Start a Vigil to prevent the Swamps from shutting down
	//     sourceSwamp.BeginVigil()
	//     destinationSwamp.BeginVigil()
	//
	//     sourceTreasures := sourceSwamp.CloneTreasures()
	//     for key, treasure := range sourceTreasures {
	//         destinationSwamp.SaveTreasure(treasure)
	//     }
	//
	// 	   // Release the Vigil. This is important to unlock the Swamps and allow them to shut down.
	//	   sourceSwamp.CeaseVigil()
	//     destinationSwamp.CeaseVigil()
	//
	//   // The treasures from the source Swamp are now duplicated in the destination Swamp.
	//
	// Returns:
	// (map[string]treasure.Treasure): A cloned map containing the treasures from the main beaconKey map.
	CloneTreasures() map[string]treasure.Treasure

	// GetName returns the name of the swamp.
	//
	// Real-world use-case:
	// - This function helps in identifying the swamp, especially useful when multiple swamps exist within the system.
	//
	// Example Usage:
	// ----------------
	// When you have multiple swamps in your system with different purposes or data, you can use GetName to retrieve
	// the name of a specific swamp for identification.
	//
	//     mySwamp.BeginVigil()
	//     swampName := mySwamp.GetName()
	//	   log.Println("Swamp name:", swampName)
	//	   mySwamp.CeaseVigil()
	//
	//   // Prints the name of the swamp to the console for identification.
	//
	// Returns:
	// (name.Name): The name of the swamp.
	GetName() name.Name

	// TreasureExists checks if the given key exists in the swamp.
	//
	// Parameters:
	// - key (string): The unique key to check for existence in the swamp.
	//
	// Real-world use-case:
	// - This function can be useful before attempting to bury a new treasure or unearth an existing one. You can use it to ensure
	//   that a key is available in the swamp before performing any operations on it.
	//
	// Example Usage:
	// ----------------
	// Before adding a new treasure with a specific key, you can use TreasureExists to check if that key already exists in the swamp.
	//
	//     keyToCheck := "unique_key"
	// 	   mySwamp.BeginVigil()
	//     exists := mySwamp.TreasureExists(keyToCheck)
	//     if exists {
	//         log.Println("Treasure with key", keyToCheck, "already exists in the swamp.")
	//     } else {
	//         // Proceed to bury the new treasure with the keyToCheck.
	//         // ...
	//     }
	//     mySwamp.CeaseVigil()
	//
	// Returns:
	// (bool): True if the key exists in the swamp, false otherwise.
	TreasureExists(key string) bool

	// CountTreasures returns the number of treasures in the swamp.
	//
	// Real-world use-case:
	// - This function can be useful for capacity planning or when you want to get information about the state of the swamp.
	//   It provides the total count of treasures currently stored in the swamp.
	//
	// Example Usage:
	// ----------------
	// If you want to monitor the size of your swamp or plan for potential scaling, you can use CountTreasures to retrieve
	// the current count of treasures.
	//
	//	   mySwamp.BeginVigil()
	//     numberOfTreasures := mySwamp.CountTreasures()
	//     log.Println("Total treasures in the swamp:", numberOfTreasures)
	//     mySwamp.CeaseVigil()
	//
	// Returns:
	// (int): The number of treasures in the swamp.
	CountTreasures() int

	// IsClosing returns true if the swamp is currently in the process of closing.
	//
	// Real-world use-case:
	// - This function can be useful for preventing new operations on a swamp that is in the process of closing.
	//   By checking the return value of IsClosing, you can avoid potential data loss or inconsistencies by
	//   ensuring that no new operations are initiated during the closing phase.
	//
	// Example Usage:
	// ----------------
	// You can use IsClosing to check whether the swamp is closing before performing certain operations.
	//
	//     if mySwamp.IsClosing() {
	//         // Handle the case when the swamp is in the process of closing.
	//     } else {
	//         // Perform normal operations on the swamp.
	//     }
	IsClosing() bool

	// Destroy locks the swamp for closing, sends the closing event, and destroys the swamp in the memory and the filesystem too.
	// It also stops the goroutines running inside the swamp.
	//
	// Real-world use-case:
	// - This function is useful if you want to permanently delete a swamp and all its data from the system.
	//
	// Important Note:
	// - It's important to note that you should call CeaseVigil() before invoking this function. Destroy relies on s.Vigil.WaitForActiveVigilsClosed()
	//   to wait for the Vigils to finish before proceeding with the closing process.
	// - In this special case, the Hydra Body will not send individual deletion events to the subscribers since the
	//   entire swamp is being destroyed. However, it will send the swamp info to the subscribers with a count of 0.
	//
	// Example Usage:
	// ----------------
	// When you need to close and destroy a swamp safely, you can call Destroy to ensure that all resources are properly released,
	// events are sent, and goroutines are terminated.
	//
	//     mySwamp.Destroy()
	Destroy()

	// All functions below can only be accessed by Hydra, not by Head --------------------------------------------------

	// WriteTreasuresToFilesystem prepares the swamp for removal from the Hydra's memory by writing its treasures to the filesystem.
	//
	// Real-world use-case:
	// - This function is crucial for data persistence and is typically invoked before shutting down a swamp or the Hydra itself.
	//   It ensures that the treasures stored in the swamp are safely written to the filesystem, allowing for data recovery
	//   and restoration in case of system restarts or crashes.
	//
	// Important:
	// - This function should not be used by the Hydra Head!!!
	//
	// Example Usage:
	// ----------------
	// Before shutting down the Hydra or a swamp, it's essential to call WriteTreasuresToFilesystem to save all treasures
	// to the filesystem for data persistence.
	//
	//     mySwamp.BeginVigil()
	//     mySwamp.WriteTreasuresToFilesystem()
	//     mySwamp.CeaseVigil()
	//
	//   // Treasures are now safely written to the filesystem.
	//
	// Notes:
	// - This function should be called to ensure that valuable data is not lost when the system is halted or restarted.
	WriteTreasuresToFilesystem()

	// GetChronicler returns the Chronicler interface associated with the swamp.
	//
	// Real-world use-case:
	// - The Chronicler is responsible for logging and tracking changes in the swamp, making this function essential for auditing and debugging.
	//   It also manages data persistence by writing swamp data to the filesystem and, if necessary, compressing it.
	//   Additionally, the Chronicler can retrieve data from the filesystem into memory, enabling efficient data loading.
	//
	// Example Usage:
	// ----------------
	// When you need to audit or debug the activities and changes in the swamp, you can use GetChronicler to obtain the
	// associated Chronicler interface.
	//
	//     swampChronicler := mySwamp.GetChronicler()
	//     // Now, you can use swampChronicler to write data to the filesystem, retrieve data from the filesystem, or write changes...
	//
	// Returns:
	// (chronicler.Chronicler): The Chronicler interface associated with the swamp.
	GetChronicler() chronicler.Chronicler

	// Close allows the Hydra to close the swamp safely.
	//
	// Real-world use-case:
	// - This function is crucial for the proper shutdown of a swamp. It ensures that all data is persisted to the filesystem,
	//   and resources are freed, allowing the swamp to gracefully exit.
	//   Additionally, SaveToFile waits for all Vigils to finish, indicating that any ongoing tasks or operations have completed.
	//   After closing, a closed event is sent to the Hydra Body to inform it about the swamp's closure.
	//   While the swamp's memory is released, it continues to exist in the filesystem, making it available for future use.
	//
	// Important Note:
	// - Close is a critical function for optimizing memory usage in the system, allowing it to retain only the necessary data.
	// - This function should NEVER be directly called by the Hydra Head, as the closure of a swamp depends on various factors
	//   monitored by the Hydra Body. The Hydra Body is responsible for coordinating and managing swamp closures, ensuring the
	//   proper execution of the process.
	// - IMPORTANT! Never run the mySwamp.BeginVigil() function before the Close function, as the closure won't complete.
	//   This is because the SaveToFile function relies on the WaitForActiveVigilsClosed() function.
	//
	// Example Usage:
	// ----------------
	// When you need to shut down a swamp safely, you can call Close to ensure that all data is saved, resources are freed,
	// and ongoing tasks are completed.
	//
	//     mySwamp.SaveToFile()
	//
	// Notes:
	// - Close is a critical function for optimizing memory usage in the system, allowing it to retain only the necessary data.
	Close()

	// StartSendingInformation enables the swamp to send information and events to the Hydra if there are any clients interested in receiving them.
	// This function is typically used to initiate real-time data streaming to clients who have subscribed to updates from this swamp.
	//
	// Real-world use-case:
	// - This function allows the swamp to begin sending information and events to the Hydra when there are clients interested in receiving them.
	// - It's a mechanism to conserve resources by only sending information when there are subscribers, avoiding unnecessary channel usage when there are none.
	//
	// Important Note:
	// - This function should NEVER be directly called by the Hydra Head!
	//
	// Example Usage:
	// ----------------
	// You can use StartSendingInformation when you want the swamp to start sending real-time updates to subscribed clients.
	//
	//     mySwamp.StartSendingInformation()
	StartSendingInformation()

	// StopSendingInformation disables the swamp from sending information to the Hydra.
	// This function is used to halt real-time data streaming to clients, usually when there are no more subscribers or when the swamp is about to be closed.
	//
	// Real-world use-case:
	// - Use this function to stop sending real-time updates and information to clients.
	// - It is typically invoked when there are no more subscribers or when the swamp is being closed to conserve resources.
	//
	// Important Note:
	// - This function should NEVER be directly called by the Hydra Head!
	//
	// Example Usage:
	// ----------------
	// You can use StopSendingInformation when you want to cease real-time updates being sent from the swamp to clients.
	//
	//     mySwamp.StopSendingInformation()
	StopSendingInformation()

	// StartSendingEvents enables the swamp to send events to the Hydra if there are any clients interested in receiving them.
	// This function is used to initiate event-based notifications to clients, providing real-time updates on changes within the swamp.
	//
	// Real-world use-case:
	// - Use this function to start sending real-time event notifications to clients who have subscribed to events from this swamp.
	//
	// Important Note:
	// - This function should NEVER be directly called by the Hydra Head!
	//
	// Example Usage:
	// ----------------
	// You can use StartSendingEvents when you want to initiate event-based notifications to clients about changes within the swamp.
	//
	//     mySwamp.StartSendingEvents()
	StartSendingEvents()

	// StopSendingEvents disables the swamp from sending events to the Hydra.
	// This function is used to stop event-based notifications to clients, usually when there are no more subscribers or when the swamp is about to be closed.
	//
	// Real-world use-case:
	// - Use this function to halt event-based notifications to clients when they are no longer interested or when the swamp is about to be closed.
	//
	// Important Note:
	// - This function should NEVER be directly called by the Hydra Head!
	//
	// Example Usage:
	// ----------------
	// You can use StopSendingEvents when you want to stop sending event notifications to clients, ensuring that no more event updates are sent.
	//
	//     mySwamp.StopSendingEvents()
	StopSendingEvents()

	// GetBeacon one beacon from the swamp by the beacon type and order.
	// This function useful if we want to iterate over the beacon and get the treasures from it
	GetBeacon(beaconType BeaconType, order BeaconOrder) beacon.Beacon

	IncrementUint8(key string, i uint8, condition *IncrementUInt8Condition) (newValue uint8, incremented bool, err error)
	IncrementUint16(key string, i uint16, condition *IncrementUInt16Condition) (newValue uint16, incremented bool, err error)
	IncrementUint32(key string, i uint32, condition *IncrementUInt32Condition) (newValue uint32, incremented bool, err error)
	IncrementUint64(key string, i uint64, condition *IncrementUInt64Condition) (newValue uint64, incremented bool, err error)

	IncrementInt8(key string, i int8, condition *IncrementInt8Condition) (newValue int8, incremented bool, err error)
	IncrementInt16(key string, i int16, condition *IncrementInt16Condition) (newValue int16, incremented bool, err error)
	IncrementInt32(key string, i int32, condition *IncrementInt32Condition) (newValue int32, incremented bool, err error)

	// IncrementInt64 increases or decreases the value associated with a given key based on a condition.
	//
	// Parameters:
	// - key: The key associated with the value to be modified.
	// - i: The value by which the current value should be incremented or decremented.
	// - condition: An optional condition that determines whether the operation can be performed.
	//              If the condition is not met, the function returns an error.
	//
	// Returns:
	// - newValue: The new value after incrementing or decrementing.
	// - incremented: A boolean indicating whether the value was incremented or not.
	// - err: An error if the condition is not met or if the existing value is not an integer.
	//
	// Operation:
	// 1. Retrieves the treasure object associated with the given key.
	// 2. Ensures that only one goroutine can access the treasure object at a time using a guard.
	// 3. Retrieves the existing value as an integer, returning an error if this fails.
	// 4. Checks the optionally provided condition:
	//    - If the condition is not met, returns an error and sets incremented to false.
	// 5. Increments or decrements the existing value by the provided amount.
	// 6. Sets the new value in the treasure object.
	// 7. Saves the treasure object.
	// 8. Returns the new value and a boolean indicating whether the value was incremented.
	//
	// Example:
	//
	// condition := &IncrementInt64Condition{
	//     RelationalOperator: RelationalOperatorEqual,
	//     Value:              100,
	// }
	// newValue, incremented, err := s.IncrementInt64("myKey", 10, condition)
	// if err != nil {
	//     log.Fatal(err)
	// }
	// fmt.Println("New Value:", newValue, "Incremented:", incremented)
	IncrementInt64(key string, i int64, condition *IncrementInt64Condition) (newValue int64, incremented bool, err error)

	IncrementFloat32(key string, f float32, condition *IncrementFloat32Condition) (newValue float32, incremented bool, err error)

	// IncrementFloat64 64 increases or decreases the value associated with a given key based on a condition.
	//
	// Parameters:
	// - key: The key associated with the value to be modified.
	// - f: The value by which the current value should be incremented or decremented.
	// - condition: An optional condition that determines whether the operation can be performed.
	//              If the condition is not met, the function returns an error.
	//
	// Returns:
	// - newValue: The new value after incrementing or decrementing.
	// - incremented: A boolean indicating whether the value was incremented or not.
	// - err: An error if the condition is not met or if the existing value is not a float.
	//
	// Operation:
	// 1. Retrieves the treasure object associated with the given key.
	// 2. Ensures that only one goroutine can access the treasure object at a time using a guard.
	// 3. Retrieves the existing value as a float, returning an error if this fails.
	// 4. Checks the optionally provided condition:
	//    - If the condition is not met, returns an error and sets incremented to false.
	// 5. Increments or decrements the existing value by the provided amount.
	// 6. Sets the new value in the treasure object.
	// 7. Saves the treasure object.
	// 8. Returns the new value and a boolean indicating whether the value was incremented.
	//
	// Example:
	//
	// condition := &IncrementFloat64Condition{
	//     RelationalOperator: RelationalOperatorEqual,
	//     Value:              100.0,
	// }
	// newValue, incremented, err := s.IncrementFloat64("myKey", 10.5, condition)
	// if err != nil {
	//     log.Fatal(err)
	// }
	// fmt.Println("New Value:", newValue, "Incremented:", incremented)
	IncrementFloat64(key string, f float64, condition *IncrementFloat64Condition) (newValue float64, incremented bool, err error)
}

const (
	ErrorTreasureDoesNotExists = "treasure does not exists"
)

// BeaconType is used to define the type of the Beacon.
type BeaconType int8

const (
	// BeaconTypeKey is used to sort Treasures in the Swamp based on their keys.
	// This type is suitable for Swamps where Treasures have unique keys.
	BeaconTypeKey BeaconType = iota
	// BeaconTypeCreationTime is used to sort Treasures in the Swamp based on CreationTime.
	// CreationTime is an int64 type timestamp. It's important to note that CreationTime is not unique;
	// multiple Treasures may have the same CreationTime. This type is only suitable for Swamps where Treasures have a CreationTime field.
	BeaconTypeCreationTime
	// BeaconTypeExpirationTime is used to sort Treasures in the Swamp based on ExpirationTime.
	// ExpirationTime is an int64 type timestamp. It's important to note that ExpirationTime is not unique;
	// multiple Treasures may have the same ExpirationTime. This type is only suitable for Swamps where Treasures have an ExpirationTime field.
	BeaconTypeExpirationTime
	// BeaconTypeUpdateTime is used to sort Treasures in the Swamp based on UpdateTime.
	// UpdateTime is an int64 type timestamp. It's important to note that UpdateTime is not unique;
	// multiple Treasures may have the same UpdateTime. This type is only suitable for Swamps where Treasures have an UpdateTime field.
	BeaconTypeUpdateTime
	BeaconTypeValueUint8
	BeaconTypeValueUint16
	BeaconTypeValueUint32
	BeaconTypeValueUint64
	BeaconTypeValueInt8
	BeaconTypeValueInt16
	BeaconTypeValueInt32
	// BeaconTypeValueInt64 represents sorting Treasures in the Swamp based on the content of ValueInt, not the key.
	// ValueInt is an int64 type value. It's important to note that ValueInt is not necessarily unique;
	// multiple Treasures may have the same ValueInt. This type is only suitable for Swamps where Treasures have a ValueInt field.
	BeaconTypeValueInt64

	BeaconTypeValueFloat32
	// BeaconTypeValueFloat64 represents sorting Treasures in the Swamp based on the content of ValueFloat, not the key.
	// ValueFloat is a float64 type value. It's important to note that ValueFloat is not necessarily unique;
	// multiple Treasures may have the same ValueFloat. This type is only suitable for Swamps where Treasures have a ValueFloat field.
	BeaconTypeValueFloat64
	// BeaconTypeValueString represents sorting Treasures in the Swamp based on the content of ValueString, not the key.
	// ValueString is a string type value. It's important to note that ValueString is not necessarily unique;
	// multiple Treasures may have the same ValueString. This type is only suitable for Swamps where Treasures have a ValueString field.
	BeaconTypeValueString
)

// BeaconOrder is used to define the sorting order for Beacons.
type BeaconOrder int

const (
	// IndexOrderAsc specifies ascending order for sorting Beacons.
	// It means sorting from the smallest value to the largest.
	IndexOrderAsc BeaconOrder = 1
	// IndexOrderDesc specifies descending order for sorting Beacons.
	// It means sorting from the largest value to the smallest.
	IndexOrderDesc BeaconOrder = 2
)

// Event represents an event that occurs in the Swamp, capturing changes to treasures and other relevant information.
//
// This structure is used to record and communicate events happening in the Swamp, such as the addition of a new treasure,
// modification of an existing treasure, or its deletion. It provides details about the event, including the swamp's name,
// the involved treasures (both new and modified/deleted), the event's timestamp, and the type of event that occurred.
//
// IMPORTANT: Events are automatically sent within the system, so the Head doesn't need to worry about sending the event. However,
// the Head can also subscribe to any event of any swamp, and upon subscription, it will receive this Event object.
//
// Fields:
//   - SwampName (name.Name): The name of the Swamp where the event occurred.
//   - Treasure (treasure.Treasure): The new treasure that was added to the Swamp. This field is relevant when the event
//     is related to adding a treasure.
//   - OldTreasure (treasure.Treasure): The treasure itself that was modified or deleted. This field is relevant when
//     the event is related to modifying or deleting a treasure.
//   - EventTime (int64): The time of the event in Unix time (milliseconds).
//   - StatusType (TreasureStatus): The type of the event that occurred, which can indicate whether it was a creation,
//     modification, deletion, or no change to a treasure.
//
// Use-cases:
// 1. Create a realtime chat application where users can see when other users join or leave the chat room or send messages.
// 1. Logging and tracking events in the Swamp.
// 2. Providing detailed information about changes to treasures and their timestamps.
type Event struct {
	SwampName       name.Name               // name of the swamp
	Treasure        treasure.Treasure       // the new treasure that is added to the swamp
	OldTreasure     treasure.Treasure       // the treasure itself that is modified or deleted
	DeletedTreasure treasure.Treasure       // the treasure that is deleted
	EventTime       int64                   // the time of the event in unix time (millisecond)
	StatusType      treasure.TreasureStatus // type of the event that is happened
}

// Info is a structure used to retrieve real-time information about a Swamp, specifically the count of treasures it contains.
//
// This structure allows subscribers to obtain the number of treasures within a given Swamp. Real-time counts of treasures
// can be useful in various scenarios, such as when building a dashboard to display the number of treasures in a Swamp.
//
// Fields:
// - SwampName (name.Name): The name of the Swamp for which the treasure count is being provided.
// - AllElements (uint64): The total number of treasures present in the Swamp represented by SwampName.
//
// Use-cases:
// 1. Real-time monitoring of the number of treasures within a Swamp.
// 2. Displaying treasure counts on dashboards or user interfaces.
type Info struct {
	SwampName   name.Name
	AllElements uint64
}

type swamp struct {
	mu              sync.RWMutex
	writerLock      sync.Mutex // mutex for the writer
	closeWriteMutex sync.Mutex // mutex for the closeWrite function
	closeMutex      sync.Mutex // mutex for the close function

	vigil.Vigil

	goRoutineContext           context.Context // context for the goroutines
	goRoutineCancelFunction    context.CancelFunc
	isInformationSendingActive int32 // if the swamp is sending information to the client
	isEventSendingActive       int32 // if the swamp is sending events to the client

	// -------------------  the following fields are used for setting up the swamp -------------------
	name name.Name // unique name of the swamp
	// -------------------  the following fields are used for the ordered lists -------------------
	// beaconKey is the main index of the swamp.
	// this is an ordered index by the creation time of the Treasures
	beaconKey beacon.Beacon // this is the main index of the swamp.

	closeAfterIdle      time.Duration // the minimum time that the swamp is in the memory
	lastInteractionTime int64         // the last time that the swamp is interacted with the client
	writeInterval       time.Duration // the interval that the swamp writes the Treasures to the chroniclerInterface

	// all beaconKey are sorted by the following fields
	keyBeaconASC             beacon.Beacon // ordered list of the Treasures by the ascendant BeaconKey field
	keyBeaconDESC            beacon.Beacon // ordered list of the Treasures by the descendant BeaconKey field
	expirationTimeBeaconASC  beacon.Beacon // ordered list of the Treasures by the ascendant ExpirationTime field
	expirationTimeBeaconDESC beacon.Beacon // ordered list of the Treasures by the descendant ExpirationTime field
	creationTimeBeaconASC    beacon.Beacon // ordered list of the Treasures by the ascendant CreatedAt field
	creationTimeBeaconDESC   beacon.Beacon // ordered list of the Treasures by the descendant CreatedAt field
	updateTimeBeaconASC      beacon.Beacon // ordered list of the Treasures by the ascendant UpdatedAt field
	updateTimeBeaconDESC     beacon.Beacon // ordered list of the Treasures by the descendant UpdatedAt field

	valueBeaconASC  beacon.Beacon // ordered list of the Treasures by the ascendant Value field
	valueBeaconDESC beacon.Beacon // ordered list of the Treasures by the descendant Value field

	// -------------------  the following fields are used for the unordered list -------------------
	// treasuresWaitingForWriter just the key of the treasures that are waiting for the writer to write them to the chroniclerInterface
	// because we need to check the existence of the treasure in the treasuresForWriter list
	treasuresWaitingForWriter beacon.Beacon

	chroniclerInterface chronicler.Chronicler // the chroniclerInterface that the swamp is using

	// -------------------  the following fields are used for the closing -------------------
	closing int32 // will be 1 if the swamp is closing

	isFilesystemWritingActive int32 // if the swamp is writing to the filesystem

	swampEventCallback func(event *Event) // the callback function that is called when an event is happened in the swamp
	swampInfoCallback  func(info *Info)   // the callback function that is called when the swamp info is requested
	swampCloseCallback func(n name.Name)  // the callback function that is called when the swamp is closed

	inMemorySwamp int32 // if the swamp is an in-memory swamp we don't write it to the filesystem

	metadataInterface metadata.Metadata // the metadata interface that the swamp is using
}

type FilesystemSettings struct {
	ChroniclerInterface chronicler.Chronicler
	WriteInterval       time.Duration
}

// New creates a new swamp object
func New(name name.Name, closeAfterIdle time.Duration, filesystemSettings *FilesystemSettings,
	swampEventCallback func(event *Event), swampInfoCallback func(info *Info), swampCloseCallback func(n name.Name),
	metadataInterface metadata.Metadata) Swamp {

	s := &swamp{
		name:                name,
		lastInteractionTime: time.Now().UnixNano(),
		Vigil:               vigil.New(),
		swampEventCallback:  swampEventCallback,
		swampInfoCallback:   swampInfoCallback,
		swampCloseCallback:  swampCloseCallback,
		closeAfterIdle:      closeAfterIdle,
		metadataInterface:   metadataInterface,
	}

	/// IMPORTANT the w.expirationTimeBeaconASC will be nil if orderType is unordered!!!!
	s.beaconKey = beacon.New()

	if filesystemSettings == nil {
		// the swamp is an IN-Memory swamp
		atomic.StoreInt32(&s.inMemorySwamp, 1)
	} else {
		// the swamp is permanent swamp. The data will be written to the filesystem and loaded from the filesystem
		s.writeInterval = filesystemSettings.WriteInterval
		atomic.StoreInt32(&s.inMemorySwamp, 0)
		s.chroniclerInterface = filesystemSettings.ChroniclerInterface
		// regisztráljuk a chroniclerInterface-be azt a funkciót, amit a chronicler akkor hív meg, amikor
		// filepointer eseemény történik, azaz a chronicler vissza akarja küldeni, hogy melyik Treasure-nak mi lett
		// a filepointer-e
		s.chroniclerInterface.RegisterFilePointerFunction(s.FilePointerCallbackFunction)
		// load the swamp from the chroniclerInterface while the swamp is created
		s.chroniclerInterface.RegisterSaveFunction(s.SaveFunction)
		// The swamp is Permanent-Type so we need to load the data from the filesystem
		s.chroniclerInterface.Load(s.beaconKey)
	}

	s.goRoutineContext, s.goRoutineCancelFunction = context.WithCancel(context.Background())

	s.keyBeaconASC = beacon.New()
	s.keyBeaconASC.SetIsOrdered(true)
	s.keyBeaconDESC = beacon.New()
	s.keyBeaconDESC.SetIsOrdered(true)

	s.expirationTimeBeaconASC = beacon.New()
	s.expirationTimeBeaconASC.SetIsOrdered(true)
	s.expirationTimeBeaconDESC = beacon.New()
	s.expirationTimeBeaconDESC.SetIsOrdered(true)

	s.creationTimeBeaconASC = beacon.New()
	s.creationTimeBeaconASC.SetIsOrdered(true)
	s.creationTimeBeaconDESC = beacon.New()
	s.creationTimeBeaconDESC.SetIsOrdered(true)

	s.updateTimeBeaconASC = beacon.New()
	s.updateTimeBeaconASC.SetIsOrdered(true)
	s.updateTimeBeaconDESC = beacon.New()
	s.updateTimeBeaconDESC.SetIsOrdered(true)

	s.valueBeaconASC = beacon.New()
	s.valueBeaconASC.SetIsOrdered(true)
	s.valueBeaconDESC = beacon.New()
	s.valueBeaconDESC.SetIsOrdered(true)

	// create beacon for the treasuresWaitingForWriter
	s.treasuresWaitingForWriter = beacon.New()
	// the treasuresWaitingForWriter is not ordered, because the ordering is not important here
	s.treasuresWaitingForWriter.SetIsOrdered(false)

	// Initiates monitoring of swamp write, close, and new filename events.
	// Do not start the autow-riter if the writeInterval is 0 because it means the swamp is a simple in-memory swamp or
	// we want to flush the data to the filesystem immediately.
	if atomic.LoadInt32(&s.inMemorySwamp) == 0 && s.writeInterval > 0 {
		go s.startWriteListener()
	}

	go s.startCloseListener()

	return s

}

func (s *swamp) GetMetadata() metadata.Metadata {
	return s.metadataInterface
}

type IncrementUInt8Condition struct {
	RelationalOperator RelationalOperator
	Value              uint8
}
type IncrementUInt16Condition struct {
	RelationalOperator RelationalOperator
	Value              uint16
}
type IncrementUInt32Condition struct {
	RelationalOperator RelationalOperator
	Value              uint32
}
type IncrementUInt64Condition struct {
	RelationalOperator RelationalOperator
	Value              uint64
}
type IncrementInt8Condition struct {
	RelationalOperator RelationalOperator
	Value              int8
}
type IncrementInt16Condition struct {
	RelationalOperator RelationalOperator
	Value              int16
}
type IncrementInt32Condition struct {
	RelationalOperator RelationalOperator
	Value              int32
}

type IncrementInt64Condition struct {
	RelationalOperator RelationalOperator
	Value              int64
}

type RelationalOperator int

const (
	RelationalOperatorEqual              RelationalOperator = 1
	RelationalOperatorNotEqual           RelationalOperator = 2
	RelationalOperatorGreaterThan        RelationalOperator = 3
	RelationalOperatorGreaterThanOrEqual RelationalOperator = 4
	RelationalOperatorLessThan           RelationalOperator = 5
	RelationalOperatorLessThanOrEqual    RelationalOperator = 6
)

func (s *swamp) IncrementUint8(key string, i uint8, condition *IncrementUInt8Condition) (newValue uint8, incremented bool, err error) {
	// get the key treasure by its key
	treasureObj := s.beaconKey.Get(key)
	// ha a treasure nem létezik még, akkor létrehozzuk azt
	if treasureObj == nil {
		// a treasure még nem létezett, így létrehozzuk azt
		func() {
			treasureObj = s.CreateTreasure(key)
			guardID := treasureObj.StartTreasureGuard(true)
			defer treasureObj.ReleaseTreasureGuard(guardID)
			treasureObj.SetContentUint8(guardID, 0)
		}()

	} else {
		// ha a treasure létezett, már, akkor ellenőrizzük a tartalom típusát
		contentType := treasureObj.GetContentType()
		// ha nem integer volt benne eddig, akkor hibát dobunk
		if contentType != treasure.ContentTypeUint8 {
			return 0, false, errors.New(ErrorValueIsNotInt)
		}
	}

	// biztosítjuk, hogy a treasure-hoz egyszerre csak egy goroutine férjen hozzá
	guardID := treasureObj.StartTreasureGuard(true, guard.BodyAuthID)
	defer treasureObj.ReleaseTreasureGuard(guardID)

	// lekérdezzük a jelenlegi integer értékét a treasure-nek
	contentInt, err := treasureObj.GetContentUint8()
	if err != nil {
		return 0, false, errors.New(ErrorValueIsNotInt)
	}

	// ellenőrizzük a feltételt, ha van megadva
	if condition != nil {
		switch condition.RelationalOperator {
		case RelationalOperatorEqual:
			if contentInt != condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorNotEqual:
			if contentInt == condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorGreaterThan:
			if contentInt <= condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorGreaterThanOrEqual:
			if contentInt < condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorLessThan:
			if contentInt >= condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorLessThanOrEqual:
			if contentInt > condition.Value {
				return contentInt, false, nil
			}
		}
	}

	// increment or decrement the value
	contentInt += i
	// beállítjuk az új értéket
	treasureObj.SetContentUint8(guardID, contentInt)
	// elmentjük a treasure-t
	treasureObj.Save(guardID)

	// visszaadjuk az új értéket és hogy incrementálva lett-e
	return contentInt, true, nil
}
func (s *swamp) IncrementUint16(key string, i uint16, condition *IncrementUInt16Condition) (newValue uint16, incremented bool, err error) {
	// get the key treasure by its key
	treasureObj := s.beaconKey.Get(key)
	// ha a treasure nem létezik még, akkor létrehozzuk azt
	if treasureObj == nil {
		// a treasure még nem létezett, így létrehozzuk azt
		func() {
			treasureObj = s.CreateTreasure(key)
			guardID := treasureObj.StartTreasureGuard(true)
			defer treasureObj.ReleaseTreasureGuard(guardID)
			treasureObj.SetContentUint16(guardID, 0)
		}()

	} else {
		// ha a treasure létezett, már, akkor ellenőrizzük a tartalom típusát
		contentType := treasureObj.GetContentType()
		// ha nem integer volt benne eddig, akkor hibát dobunk
		if contentType != treasure.ContentTypeUint16 {
			return 0, false, errors.New(ErrorValueIsNotInt)
		}
	}

	// biztosítjuk, hogy a treasure-hoz egyszerre csak egy goroutine férjen hozzá
	guardID := treasureObj.StartTreasureGuard(true, guard.BodyAuthID)
	defer treasureObj.ReleaseTreasureGuard(guardID)

	// lekérdezzük a jelenlegi integer értékét a treasure-nek
	contentInt, err := treasureObj.GetContentUint16()
	if err != nil {
		return 0, false, errors.New(ErrorValueIsNotInt)
	}

	// ellenőrizzük a feltételt, ha van megadva
	if condition != nil {
		switch condition.RelationalOperator {
		case RelationalOperatorEqual:
			if contentInt != condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorNotEqual:
			if contentInt == condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorGreaterThan:
			if contentInt <= condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorGreaterThanOrEqual:
			if contentInt < condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorLessThan:
			if contentInt >= condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorLessThanOrEqual:
			if contentInt > condition.Value {
				return contentInt, false, nil
			}
		}
	}

	// increment or decrement the value
	contentInt += i
	// beállítjuk az új értéket
	treasureObj.SetContentUint16(guardID, contentInt)
	// elmentjük a treasure-t
	treasureObj.Save(guardID)

	// visszaadjuk az új értéket és hogy incrementálva lett-e
	return contentInt, true, nil
}
func (s *swamp) IncrementUint32(key string, i uint32, condition *IncrementUInt32Condition) (newValue uint32, incremented bool, err error) {
	// get the key treasure by its key
	treasureObj := s.beaconKey.Get(key)
	// ha a treasure nem létezik még, akkor létrehozzuk azt
	if treasureObj == nil {
		// a treasure még nem létezett, így létrehozzuk azt
		func() {
			treasureObj = s.CreateTreasure(key)
			guardID := treasureObj.StartTreasureGuard(true)
			defer treasureObj.ReleaseTreasureGuard(guardID)
			treasureObj.SetContentUint32(guardID, 0)
		}()

	} else {
		// ha a treasure létezett, már, akkor ellenőrizzük a tartalom típusát
		contentType := treasureObj.GetContentType()
		// ha nem integer volt benne eddig, akkor hibát dobunk
		if contentType != treasure.ContentTypeUint32 {
			return 0, false, errors.New(ErrorValueIsNotInt)
		}
	}

	// biztosítjuk, hogy a treasure-hoz egyszerre csak egy goroutine férjen hozzá
	guardID := treasureObj.StartTreasureGuard(true, guard.BodyAuthID)
	defer treasureObj.ReleaseTreasureGuard(guardID)

	// lekérdezzük a jelenlegi integer értékét a treasure-nek
	contentInt, err := treasureObj.GetContentUint32()
	if err != nil {
		return 0, false, errors.New(ErrorValueIsNotInt)
	}

	// ellenőrizzük a feltételt, ha van megadva
	if condition != nil {
		switch condition.RelationalOperator {
		case RelationalOperatorEqual:
			if contentInt != condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorNotEqual:
			if contentInt == condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorGreaterThan:
			if contentInt <= condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorGreaterThanOrEqual:
			if contentInt < condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorLessThan:
			if contentInt >= condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorLessThanOrEqual:
			if contentInt > condition.Value {
				return contentInt, false, nil
			}
		}
	}

	// increment or decrement the value
	contentInt += i
	// beállítjuk az új értéket
	treasureObj.SetContentUint32(guardID, contentInt)
	// elmentjük a treasure-t
	treasureObj.Save(guardID)

	// visszaadjuk az új értéket és hogy incrementálva lett-e
	return contentInt, true, nil
}
func (s *swamp) IncrementUint64(key string, i uint64, condition *IncrementUInt64Condition) (newValue uint64, incremented bool, err error) {
	// get the key treasure by its key
	treasureObj := s.beaconKey.Get(key)
	// ha a treasure nem létezik még, akkor létrehozzuk azt
	if treasureObj == nil {
		// a treasure még nem létezett, így létrehozzuk azt
		func() {
			treasureObj = s.CreateTreasure(key)
			guardID := treasureObj.StartTreasureGuard(true)
			defer treasureObj.ReleaseTreasureGuard(guardID)
			treasureObj.SetContentUint64(guardID, 0)
		}()

	} else {
		// ha a treasure létezett, már, akkor ellenőrizzük a tartalom típusát
		contentType := treasureObj.GetContentType()
		// ha nem integer volt benne eddig, akkor hibát dobunk
		if contentType != treasure.ContentTypeUint64 {
			return 0, false, errors.New(ErrorValueIsNotInt)
		}
	}

	// biztosítjuk, hogy a treasure-hoz egyszerre csak egy goroutine férjen hozzá
	guardID := treasureObj.StartTreasureGuard(true, guard.BodyAuthID)
	defer treasureObj.ReleaseTreasureGuard(guardID)

	// lekérdezzük a jelenlegi integer értékét a treasure-nek
	contentInt, err := treasureObj.GetContentUint64()
	if err != nil {
		return 0, false, errors.New(ErrorValueIsNotInt)
	}

	// ellenőrizzük a feltételt, ha van megadva
	if condition != nil {
		switch condition.RelationalOperator {
		case RelationalOperatorEqual:
			if contentInt != condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorNotEqual:
			if contentInt == condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorGreaterThan:
			if contentInt <= condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorGreaterThanOrEqual:
			if contentInt < condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorLessThan:
			if contentInt >= condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorLessThanOrEqual:
			if contentInt > condition.Value {
				return contentInt, false, nil
			}
		}
	}

	// increment or decrement the value
	contentInt += i
	// beállítjuk az új értéket
	treasureObj.SetContentUint64(guardID, contentInt)
	// elmentjük a treasure-t
	treasureObj.Save(guardID)

	// visszaadjuk az új értéket és hogy incrementálva lett-e
	return contentInt, true, nil
}
func (s *swamp) IncrementInt8(key string, i int8, condition *IncrementInt8Condition) (newValue int8, incremented bool, err error) {
	// get the key treasure by its key
	treasureObj := s.beaconKey.Get(key)
	// ha a treasure nem létezik még, akkor létrehozzuk azt
	if treasureObj == nil {
		// a treasure még nem létezett, így létrehozzuk azt
		func() {
			treasureObj = s.CreateTreasure(key)
			guardID := treasureObj.StartTreasureGuard(true)
			defer treasureObj.ReleaseTreasureGuard(guardID)
			treasureObj.SetContentInt8(guardID, 0)
		}()

	} else {
		// ha a treasure létezett, már, akkor ellenőrizzük a tartalom típusát
		contentType := treasureObj.GetContentType()
		// ha nem integer volt benne eddig, akkor hibát dobunk
		if contentType != treasure.ContentTypeInt8 {
			return 0, false, errors.New(ErrorValueIsNotInt)
		}
	}

	// biztosítjuk, hogy a treasure-hoz egyszerre csak egy goroutine férjen hozzá
	guardID := treasureObj.StartTreasureGuard(true, guard.BodyAuthID)
	defer treasureObj.ReleaseTreasureGuard(guardID)

	// lekérdezzük a jelenlegi integer értékét a treasure-nek
	contentInt, err := treasureObj.GetContentInt8()
	if err != nil {
		return 0, false, errors.New(ErrorValueIsNotInt)
	}

	// ellenőrizzük a feltételt, ha van megadva
	if condition != nil {
		switch condition.RelationalOperator {
		case RelationalOperatorEqual:
			if contentInt != condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorNotEqual:
			if contentInt == condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorGreaterThan:
			if contentInt <= condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorGreaterThanOrEqual:
			if contentInt < condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorLessThan:
			if contentInt >= condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorLessThanOrEqual:
			if contentInt > condition.Value {
				return contentInt, false, nil
			}
		}
	}

	// increment or decrement the value
	contentInt += i
	// beállítjuk az új értéket
	treasureObj.SetContentInt8(guardID, contentInt)
	// elmentjük a treasure-t
	treasureObj.Save(guardID)

	// visszaadjuk az új értéket és hogy incrementálva lett-e
	return contentInt, true, nil
}
func (s *swamp) IncrementInt16(key string, i int16, condition *IncrementInt16Condition) (newValue int16, incremented bool, err error) {
	// get the key treasure by its key
	treasureObj := s.beaconKey.Get(key)
	// ha a treasure nem létezik még, akkor létrehozzuk azt
	if treasureObj == nil {
		// a treasure még nem létezett, így létrehozzuk azt
		func() {
			treasureObj = s.CreateTreasure(key)
			guardID := treasureObj.StartTreasureGuard(true)
			defer treasureObj.ReleaseTreasureGuard(guardID)
			treasureObj.SetContentInt16(guardID, 0)
		}()

	} else {
		// ha a treasure létezett, már, akkor ellenőrizzük a tartalom típusát
		contentType := treasureObj.GetContentType()
		// ha nem integer volt benne eddig, akkor hibát dobunk
		if contentType != treasure.ContentTypeInt16 {
			return 0, false, errors.New(ErrorValueIsNotInt)
		}
	}

	// biztosítjuk, hogy a treasure-hoz egyszerre csak egy goroutine férjen hozzá
	guardID := treasureObj.StartTreasureGuard(true, guard.BodyAuthID)
	defer treasureObj.ReleaseTreasureGuard(guardID)

	// lekérdezzük a jelenlegi integer értékét a treasure-nek
	contentInt, err := treasureObj.GetContentInt16()
	if err != nil {
		return 0, false, errors.New(ErrorValueIsNotInt)
	}

	// ellenőrizzük a feltételt, ha van megadva
	if condition != nil {
		switch condition.RelationalOperator {
		case RelationalOperatorEqual:
			if contentInt != condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorNotEqual:
			if contentInt == condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorGreaterThan:
			if contentInt <= condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorGreaterThanOrEqual:
			if contentInt < condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorLessThan:
			if contentInt >= condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorLessThanOrEqual:
			if contentInt > condition.Value {
				return contentInt, false, nil
			}
		}
	}

	// increment or decrement the value
	contentInt += i
	// beállítjuk az új értéket
	treasureObj.SetContentInt16(guardID, contentInt)
	// elmentjük a treasure-t
	treasureObj.Save(guardID)

	// visszaadjuk az új értéket és hogy incrementálva lett-e
	return contentInt, true, nil
}
func (s *swamp) IncrementInt32(key string, i int32, condition *IncrementInt32Condition) (newValue int32, incremented bool, err error) {
	// get the key treasure by its key
	treasureObj := s.beaconKey.Get(key)
	// ha a treasure nem létezik még, akkor létrehozzuk azt
	if treasureObj == nil {
		// a treasure még nem létezett, így létrehozzuk azt
		func() {
			treasureObj = s.CreateTreasure(key)
			guardID := treasureObj.StartTreasureGuard(true)
			defer treasureObj.ReleaseTreasureGuard(guardID)
			treasureObj.SetContentInt32(guardID, 0)
		}()

	} else {
		// ha a treasure létezett, már, akkor ellenőrizzük a tartalom típusát
		contentType := treasureObj.GetContentType()
		// ha nem integer volt benne eddig, akkor hibát dobunk
		if contentType != treasure.ContentTypeInt32 {
			return 0, false, errors.New(ErrorValueIsNotInt)
		}
	}

	// biztosítjuk, hogy a treasure-hoz egyszerre csak egy goroutine férjen hozzá
	guardID := treasureObj.StartTreasureGuard(true, guard.BodyAuthID)
	defer treasureObj.ReleaseTreasureGuard(guardID)

	// lekérdezzük a jelenlegi integer értékét a treasure-nek
	contentInt, err := treasureObj.GetContentInt32()
	if err != nil {
		return 0, false, errors.New(ErrorValueIsNotInt)
	}

	// ellenőrizzük a feltételt, ha van megadva
	if condition != nil {
		switch condition.RelationalOperator {
		case RelationalOperatorEqual:
			if contentInt != condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorNotEqual:
			if contentInt == condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorGreaterThan:
			if contentInt <= condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorGreaterThanOrEqual:
			if contentInt < condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorLessThan:
			if contentInt >= condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorLessThanOrEqual:
			if contentInt > condition.Value {
				return contentInt, false, nil
			}
		}
	}

	// increment or decrement the value
	contentInt += i
	// beállítjuk az új értéket
	treasureObj.SetContentInt32(guardID, contentInt)
	// elmentjük a treasure-t
	treasureObj.Save(guardID)

	// visszaadjuk az új értéket és hogy incrementálva lett-e
	return contentInt, true, nil
}

func (s *swamp) IncrementInt64(key string, i int64, condition *IncrementInt64Condition) (newValue int64, incremented bool, err error) {

	// get the key treasure by its key
	treasureObj := s.beaconKey.Get(key)
	// ha a treasure nem létezik még, akkor létrehozzuk azt
	if treasureObj == nil {
		// a treasure még nem létezett, így létrehozzuk azt
		func() {
			treasureObj = s.CreateTreasure(key)
			guardID := treasureObj.StartTreasureGuard(true)
			defer treasureObj.ReleaseTreasureGuard(guardID)
			treasureObj.SetContentInt64(guardID, 0)
		}()

	} else {
		// ha a treasure létezett, már, akkor ellenőrizzük a tartalom típusát
		contentType := treasureObj.GetContentType()
		// ha nem integer volt benne eddig, akkor hibát dobunk
		if contentType != treasure.ContentTypeInt64 {
			return 0, false, errors.New(ErrorValueIsNotInt)
		}
	}

	// biztosítjuk, hogy a treasure-hoz egyszerre csak egy goroutine férjen hozzá
	guardID := treasureObj.StartTreasureGuard(true, guard.BodyAuthID)
	defer treasureObj.ReleaseTreasureGuard(guardID)

	// lekérdezzük a jelenlegi integer értékét a treasure-nek
	contentInt, err := treasureObj.GetContentInt64()
	if err != nil {
		return 0, false, errors.New(ErrorValueIsNotInt)
	}

	// ellenőrizzük a feltételt, ha van megadva
	if condition != nil {
		switch condition.RelationalOperator {
		case RelationalOperatorEqual:
			if contentInt != condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorNotEqual:
			if contentInt == condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorGreaterThan:
			if contentInt <= condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorGreaterThanOrEqual:
			if contentInt < condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorLessThan:
			if contentInt >= condition.Value {
				return contentInt, false, nil
			}
		case RelationalOperatorLessThanOrEqual:
			if contentInt > condition.Value {
				return contentInt, false, nil
			}
		}
	}

	// increment or decrement the value
	contentInt += i
	// beállítjuk az új értéket
	treasureObj.SetContentInt64(guardID, contentInt)
	// elmentjük a treasure-t
	treasureObj.Save(guardID)

	// visszaadjuk az új értéket és hogy incrementálva lett-e
	return contentInt, true, nil

}

type IncrementFloat32Condition struct {
	RelationalOperator RelationalOperator
	Value              float32
}

type IncrementFloat64Condition struct {
	RelationalOperator RelationalOperator
	Value              float64
}

func (s *swamp) IncrementFloat32(key string, f float32, condition *IncrementFloat32Condition) (newValue float32, incremented bool, err error) {

	// get the key treasure by its key
	treasureObj := s.beaconKey.Get(key)
	// ha a treasure nem létezik még, akkor létrehozzuk azt
	if treasureObj == nil {
		// a treasure még nem létezett, így létrehozzuk azt
		func() {
			treasureObj = s.CreateTreasure(key)
			guardID := treasureObj.StartTreasureGuard(true)
			defer treasureObj.ReleaseTreasureGuard(guardID)
			treasureObj.SetContentFloat32(guardID, 0)
		}()

	} else {
		// ha a treasure létezett, már, akkor ellenőrizzük a tartalom típusát
		contentType := treasureObj.GetContentType()
		// ha nem float volt benne eddig, akkor hibát dobunk
		if contentType != treasure.ContentTypeFloat32 {
			return 0, false, errors.New(ErrorValueIsNotFloat)
		}
	}

	// ensure that only one goroutine can access the treasure object at a time
	guardID := treasureObj.StartTreasureGuard(true, guard.BodyAuthID)
	defer treasureObj.ReleaseTreasureGuard(guardID)

	// get the float value and return an error if it fails
	contentFloat, err := treasureObj.GetContentFloat32()
	if err != nil {
		return 0, false, errors.New(ErrorValueIsNotFloat)
	}

	// check the condition if provided
	if condition != nil {
		switch condition.RelationalOperator {
		case RelationalOperatorEqual:
			if contentFloat != condition.Value {
				return contentFloat, false, nil
			}
		case RelationalOperatorNotEqual:
			if contentFloat == condition.Value {
				return contentFloat, false, nil
			}
		case RelationalOperatorGreaterThan:
			if contentFloat <= condition.Value {
				return contentFloat, false, nil
			}
		case RelationalOperatorGreaterThanOrEqual:
			if contentFloat < condition.Value {
				return contentFloat, false, nil
			}
		case RelationalOperatorLessThan:
			if contentFloat >= condition.Value {
				return contentFloat, false, nil
			}
		case RelationalOperatorLessThanOrEqual:
			if contentFloat > condition.Value {
				return contentFloat, false, nil
			}
		}
	}

	// increment or decrement the value
	contentFloat += f

	// set the new value
	treasureObj.SetContentFloat32(guardID, contentFloat)
	// save the treasure object
	treasureObj.Save(guardID)

	// return the new value and whether it was incremented
	return contentFloat, true, nil
}

func (s *swamp) IncrementFloat64(key string, f float64, condition *IncrementFloat64Condition) (newValue float64, incremented bool, err error) {

	// get the key treasure by its key
	treasureObj := s.beaconKey.Get(key)
	// ha a treasure nem létezik még, akkor létrehozzuk azt
	if treasureObj == nil {
		// a treasure még nem létezett, így létrehozzuk azt
		func() {
			treasureObj = s.CreateTreasure(key)
			guardID := treasureObj.StartTreasureGuard(true)
			defer treasureObj.ReleaseTreasureGuard(guardID)
			treasureObj.SetContentFloat64(guardID, 0)
		}()

	} else {
		// ha a treasure létezett, már, akkor ellenőrizzük a tartalom típusát
		contentType := treasureObj.GetContentType()
		// ha nem float volt benne eddig, akkor hibát dobunk
		if contentType != treasure.ContentTypeFloat64 {
			return 0, false, errors.New(ErrorValueIsNotFloat)
		}
	}

	// ensure that only one goroutine can access the treasure object at a time
	guardID := treasureObj.StartTreasureGuard(true, guard.BodyAuthID)
	defer treasureObj.ReleaseTreasureGuard(guardID)

	// get the float value and return an error if it fails
	contentFloat, err := treasureObj.GetContentFloat64()
	if err != nil {
		return 0, false, errors.New(ErrorValueIsNotFloat)
	}

	// check the condition if provided
	if condition != nil {
		switch condition.RelationalOperator {
		case RelationalOperatorEqual:
			if contentFloat != condition.Value {
				return contentFloat, false, nil
			}
		case RelationalOperatorNotEqual:
			if contentFloat == condition.Value {
				return contentFloat, false, nil
			}
		case RelationalOperatorGreaterThan:
			if contentFloat <= condition.Value {
				return contentFloat, false, nil
			}
		case RelationalOperatorGreaterThanOrEqual:
			if contentFloat < condition.Value {
				return contentFloat, false, nil
			}
		case RelationalOperatorLessThan:
			if contentFloat >= condition.Value {
				return contentFloat, false, nil
			}
		case RelationalOperatorLessThanOrEqual:
			if contentFloat > condition.Value {
				return contentFloat, false, nil
			}
		}
	}

	// increment or decrement the value
	contentFloat += f

	// set the new value
	treasureObj.SetContentFloat64(guardID, contentFloat)
	// save the treasure object
	treasureObj.Save(guardID)

	// return the new value and whether it was incremented
	return contentFloat, true, nil
}

func (s *swamp) GetBeacon(beaconType BeaconType, order BeaconOrder) beacon.Beacon {

	switch beaconType {
	case BeaconTypeCreationTime:
		s.buildBeacon(s.creationTimeBeaconASC, s.creationTimeBeaconDESC, BeaconTypeCreationTime)
		if order == IndexOrderAsc {
			return s.creationTimeBeaconASC
		}
		return s.creationTimeBeaconDESC
	case BeaconTypeExpirationTime:
		s.buildBeacon(s.expirationTimeBeaconASC, s.expirationTimeBeaconDESC, BeaconTypeExpirationTime)
		if order == IndexOrderAsc {
			return s.expirationTimeBeaconASC
		}
		return s.expirationTimeBeaconDESC
	case BeaconTypeUpdateTime:
		s.buildBeacon(s.updateTimeBeaconASC, s.updateTimeBeaconDESC, BeaconTypeUpdateTime)
		if order == IndexOrderAsc {
			return s.updateTimeBeaconASC
		}
		return s.updateTimeBeaconDESC
	case BeaconTypeValueInt64, BeaconTypeValueFloat64, BeaconTypeValueString:
		s.buildBeacon(s.valueBeaconASC, s.valueBeaconDESC, BeaconTypeValueInt64)
		if order == IndexOrderAsc {
			return s.valueBeaconASC
		}
		return s.valueBeaconDESC
	case BeaconTypeKey:
		s.buildBeacon(s.keyBeaconASC, s.keyBeaconDESC, BeaconTypeKey)
		if order == IndexOrderAsc {
			return s.keyBeaconASC
		}
		return s.keyBeaconDESC
	default:
		return nil
	}

}

// WaitForGracefulClose this is a blocker function. Any thread can call this function to wait for the treasure to
// be successfully closed. This function will return nil if the treasure is successfully closed. Otherwise, it will
// return an error.
func (s *swamp) WaitForGracefulClose(ctx context.Context) error {

	// if the swamp is not closing yet, we can not wait for the swamp to be closed
	if atomic.LoadInt32(&s.closing) == 0 {
		return errors.New("swamp is not closing yet")
	}

	for {
		select {
		case <-ctx.Done():
			// tha main context is done, we can not wait for the swamp to be closed
			return errors.New("context is done")
		case <-s.goRoutineContext.Done():
			// the swamp is closed successfully
			return nil
		}
	}

}

// CountTreasuresWaitingForWriter returns the number of treasures that are waiting for the writer to write them to the chroniclerInterface
func (s *swamp) CountTreasuresWaitingForWriter() int {
	return s.treasuresWaitingForWriter.Count()
}

// CreateTreasure creates a new Treasure object if it is not existing in the swamp or returns with the existing one.
func (s *swamp) CreateTreasure(key string) treasure.Treasure {

	// return with the original treasure if it is existing
	// this working like the Load function
	if treasureObj := s.beaconKey.Get(key); treasureObj != nil {
		// return with the original treasure
		return treasureObj
	}

	t := treasure.New(s.SaveFunction)
	guardID := t.StartTreasureGuard(true, guard.BodyAuthID)
	t.BodySetKey(guardID, key)
	t.ReleaseTreasureGuard(guardID)

	return t
}

func (s *swamp) SaveFunction(t treasure.Treasure, guardID guard.ID) treasure.TreasureStatus {

	// set the last interaction time to the current time
	atomic.StoreInt64(&s.lastInteractionTime, time.Now().UnixNano())

	existedTreasureObj := s.beaconKey.Get(t.GetKey())
	// we must add the treasure to the treasuresWaitingForWriter index if it is not exists in the swamp
	// because this is means that the treasure is not written to the chroniclerInterface yet
	// and the treasure is totally new
	if existedTreasureObj == nil {

		// add the treasure to the treasuresWaitingForWriter index
		s.treasuresWaitingForWriter.Add(t)

		// add treasure to the beaconKey index
		s.beaconKey.Add(t)
		// add treasure to all other beacons if needed
		s.addTreasureToBeacons(t)
		s.sendEventToHydra(t, nil, treasure.StatusNew)
		s.sendSwampInfo()

		// immediately write the treasure to the chroniclerInterface if the write interval is 0
		s.mu.RLock()
		wi := s.writeInterval
		inMem := s.inMemorySwamp
		s.mu.RUnlock()
		if wi == 0 && inMem == 0 {
			// treasure lock feloldása hogy a kírás azonnal történjen
			t.ReleaseTreasureGuard(guardID)
			// write the treasure to the chroniclerInterface
			s.fileWriterHandler(false)
		}

		// beállítjuk az utolsó módosítás dátumát a metában
		s.metadataInterface.SetUpdatedAt()
		// return with statusNew
		return treasure.StatusNew

	}

	// if the treasure is existing in the swamp, we need to check if it is modified or not
	if t.IsContentChanged() || t.IsContentTypeChanged() || t.IsExpirationTimeChanged() ||
		t.IsCreatedAtChanged() || t.IsCreatedByChanged() || t.IsDeletedAtChanged() ||
		t.IsDeletedByChanged() || t.IsModifiedAtChanged() || t.IsModifiedByChanged() {

		// if the content type changed...
		if t.IsContentTypeChanged() {
			// delete the treasure from the beacons
			s.deleteTreasureFromBeacons(t.GetKey())
			// add the treasure back to the beacons if the content type is not void
			if t.GetContentType() != treasure.ContentTypeVoid {
				s.addTreasureToBeacons(t)
			}
		}

		// the treasure is modified, we need to add it to the swamp and write it to the chroniclerInterface
		s.treasuresWaitingForWriter.Add(t)

		// send the event to the hydra
		s.sendEventToHydra(t, existedTreasureObj, treasure.StatusModified)

		// immediately write the treasure to the chroniclerInterface if the write interval is 0
		s.mu.RLock()
		wi := s.writeInterval
		inMem := s.inMemorySwamp
		s.mu.RUnlock()
		if wi == 0 && inMem == 0 {
			// treasure lock feloldása hogy a kírás azonnal történjen
			t.ReleaseTreasureGuard(guardID)
			// write the treasure to the chroniclerInterface
			s.fileWriterHandler(false)
		}

		// beállítjuk az utolsó módosítás dátumát a metában
		s.metadataInterface.SetUpdatedAt()
		// return with statusModified
		return treasure.StatusModified

	}

	// nothing changed, we don't need to send events to the hydra
	return treasure.StatusSame

}

// Close closes the swamp (write all waiting treasures to the chroniclerInterface) and stops all goroutines inside the swamp
// Sends the stop event to the manager at the end of the function
// The manager uses this function to close the swamp only if there is a gracefulStop signal
// DO NOT ADD TRANSACTION IF YOU CALL THIS FUNCTION, because the swamp can not be closed until the last transaction is released
func (s *swamp) Close() {

	s.closeMutex.Lock()
	if atomic.LoadInt32(&s.closing) == 1 {
		// the swamp is already closing
		s.closeMutex.Unlock()
		return
	}
	// set closing to 1 immediately to prevent other transactions to be created on the swamp
	atomic.StoreInt32(&s.closing, 1)
	s.closeMutex.Unlock()

	// write all treasures to the chroniclerInterface that are waiting for the writer and don't send events to the hydra
	// because we are closing the swamp and ask the chroniclerInterface to not send file pointers for new files, because,
	// we are closing the swamp and we don't need to write the file pointers to the treasures
	if atomic.LoadInt32(&s.inMemorySwamp) == 0 {
		s.chroniclerInterface.DontSendFilePointer()
		// write files to the filesystem that are waiting for the writer
		s.fileWriterHandler(true)
		// save metadata to the filesystem if there is any changes
		s.metadataInterface.SaveToFile()
	}

	// megvárjuk a bezárás előtt, hogy a chronicler minden adatot kiírjon a filerendszerbe, különben lehet olyan, hogy
	// újra megnyitják a swampot és még nem tud mindent beolvasni, mert még nem írta ki a chroniclerInterface a filerendszerbe
	// az adatokat. Így fontos, hogy a chroniclerInterface minden adatot kiírjon a filerendszerbe, mielőtt a swamp bezáródik

	// close internal routines because we are closing the swamp and we don't need to listen for new events or write ticker
	s.goRoutineCancelFunction()

	// send the closed event to the hydra
	s.sendClosedEvent()

	return

}

// sendClosedEvent sends a signal to the Manager because the swamp is successfully closed itself
func (s *swamp) sendClosedEvent() {
	s.swampCloseCallback(s.name)
}

// Destroy destroys all treasures in the chroniclerInterface, and all its treasures and stops all goroutines inside the swamp
// DO NOT ADD TRANSACTION IF YOU CALL THIS FUNCTION, because the swamp can not be closed until the last transaction is released
// !!!!!!!! IMPORTANT: YOU NEED TO RELEASE THE TRANSACTION BEFORE CALLING THIS FUNCTION, BECAUSE THIS FUNCTION
// WAITS FOR ALL TRANSACTIONS TO BE RELEASED
func (s *swamp) Destroy() {

	atomic.StoreInt32(&s.closing, 1)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.StopSendingInformation()
	s.StopSendingEvents()

	s.Vigil.WaitForActiveVigilsClosed()

	// stops all goroutines inside the swamp
	s.goRoutineCancelFunction()

	// destroy the chroniclerInterface
	if atomic.LoadInt32(&s.inMemorySwamp) == 0 {
		// ez a filesystem-en keresztül törli a swampot és a meta filet is egyben
		s.chroniclerInterface.Destroy()
	}

	// send the closed event to the ManagerInterface that will delete the swamp from the map
	s.sendClosedEvent()

	return

}

// IsClosing returns true if the swamp is closing
// using atomic function, because the atomic functions is much faster than the mutex
// Ez a funkció egyben meg is hosszabbítja a swamp lastInteractionTime mezőjét is, ami miatt a swamp nem fog bezáródni
// azonnal, így biztonsággal kiadható még a BeginVigil() utasítás is, valamint a swampot lekérdező funkciók is
// biztonsággal használhatóak
func (s *swamp) IsClosing() bool {
	// set the last interaction time to the current time
	atomic.StoreInt64(&s.lastInteractionTime, time.Now().UnixNano())
	return atomic.LoadInt32(&s.closing) == 1
}

// GetName get the name of the swamp
func (s *swamp) GetName() name.Name {
	// set the last interaction time to the current time
	atomic.StoreInt64(&s.lastInteractionTime, time.Now().UnixNano())
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.name
}

// GetChronicler returns the chroniclerInterface of the swamp
func (s *swamp) GetChronicler() chronicler.Chronicler {
	// set the last interaction time to the current time
	atomic.StoreInt64(&s.lastInteractionTime, time.Now().UnixNano())
	return s.chroniclerInterface
}

// StartSendingInformation is a function that starts sending information about the swamp to the client if the client is subscribed to it
func (s *swamp) StartSendingInformation() {
	// set the last interaction time to the current time
	atomic.StoreInt64(&s.lastInteractionTime, time.Now().UnixNano())
	atomic.StoreInt32(&s.isInformationSendingActive, 1)
}

// StopSendingInformation is a function that stops sending information about the swamp to the client if the client is unsubscribed from it
func (s *swamp) StopSendingInformation() {
	// set the last interaction time to the current time
	atomic.StoreInt64(&s.lastInteractionTime, time.Now().UnixNano())
	atomic.StoreInt32(&s.isInformationSendingActive, 0)
}

// StartSendingEvents is a function that starts sending events about the swamp to the client if the client is subscribed to it
func (s *swamp) StartSendingEvents() {
	// set the last interaction time to the current time
	atomic.StoreInt64(&s.lastInteractionTime, time.Now().UnixNano())
	atomic.StoreInt32(&s.isEventSendingActive, 1)
}

// StopSendingEvents is a function that stops sending events about the swamp to the client if the client is unsubscribed from it
func (s *swamp) StopSendingEvents() {
	// set the last interaction time to the current time
	atomic.StoreInt64(&s.lastInteractionTime, time.Now().UnixNano())
	atomic.StoreInt32(&s.isEventSendingActive, 0)
}

// GetTreasuresByBeacon can get and delete treasures from indexes
func (s *swamp) GetTreasuresByBeacon(beaconType BeaconType, beaconOrderType BeaconOrder, from int32, limit int32) ([]treasure.Treasure, error) {

	// set the last interaction time to the current time
	atomic.StoreInt64(&s.lastInteractionTime, time.Now().UnixNano())

	// if the limit 0 its means that we need to get all treasures from the beacon from, the "from" parameter
	if limit == 0 {
		// get the element count of the beacon
		limit = int32(s.beaconKey.Count())
	}

	var selectedTreasures []treasure.Treasure
	var err error
	switch beaconType {
	case BeaconTypeKey:
		selectedTreasures, err = s.findInKeyBeacon(beaconOrderType, from, limit)
	case BeaconTypeExpirationTime:
		selectedTreasures, err = s.findInExpirationTimeBeacon(beaconOrderType, from, limit)
	case BeaconTypeCreationTime:
		selectedTreasures, err = s.findInCreationTimeBeacon(beaconOrderType, from, limit)
	case BeaconTypeUpdateTime:
		selectedTreasures, err = s.findInUpdateTimeBeacon(beaconOrderType, from, limit)
	default:
		// find in value-based beacons
		selectedTreasures, err = s.findInValueBeacon(beaconOrderType, beaconType, from, limit)
	}

	if err != nil {
		return nil, err
	}

	var returningTreasures []treasure.Treasure
	for _, d := range selectedTreasures {
		returningTreasures = append(returningTreasures, d)
	}

	return returningTreasures, nil

}

// CloneTreasures returns a clone of the swamp object with all beacons and treasures
func (s *swamp) CloneTreasures() map[string]treasure.Treasure {
	// set the last interaction time to the current time
	atomic.StoreInt64(&s.lastInteractionTime, time.Now().UnixNano())
	return s.beaconKey.CloneUnorderedTreasures(false)
}

// GetTreasure Retrieves a single "Treasure" from a "Swamp" by its unique key.
// Real-world use-case: Fetching a specific user's details for profile display.
func (s *swamp) GetTreasure(key string) (treasure treasure.Treasure, err error) {
	// set the last interaction time to the current time
	atomic.StoreInt64(&s.lastInteractionTime, time.Now().UnixNano())
	if treasureObj := s.beaconKey.Get(key); treasureObj != nil {
		// return with the original treasure
		return treasureObj, nil
	}
	return nil, errors.New(ErrorTreasureDoesNotExists)
}

// GetAll Retrieves all "Treasures" from a "Swamp".
func (s *swamp) GetAll() map[string]treasure.Treasure {
	// set the last interaction time to the current time
	atomic.StoreInt64(&s.lastInteractionTime, time.Now().UnixNano())
	if s.beaconKey.Count() == 0 {
		return nil
	}
	return s.beaconKey.GetAll()
}

// CountTreasures Returns the number of treasures in the swamp.
// This function can be useful for capacity planning or when you want to get information about the state of the swamp.
func (s *swamp) CountTreasures() int {
	// set the last interaction time to the current time
	atomic.StoreInt64(&s.lastInteractionTime, time.Now().UnixNano())
	return s.beaconKey.Count()
}

// DeleteTreasure Deletes a single "Treasure" from a "Swamp" by its unique key.
// Real-world use-case: Deleting a user account upon request.
// shadowDelete is a flag that indicates whether the treasure should be deleted from the chroniclerInterface too
// if the shadowDelete is true, then the treasure will be flagged as deleted and it will not be deleted from the chroniclerInterface
// if the shadowDelete is false, then the treasure will be deleted from the chroniclerInterface too
func (s *swamp) DeleteTreasure(key string, shadowDelete bool) error {

	// set the last interaction time to the current time
	atomic.StoreInt64(&s.lastInteractionTime, time.Now().UnixNano())
	if !s.beaconKey.IsExists(key) {
		return errors.New(ErrorTreasureDoesNotExists)
	}

	// delete the treasure from the beaconKey
	// delete the treasure from the swamp and from the chroniclerInterface too
	s.deleteHandler(key, shadowDelete)

	// destroy the swamp if there is no treasure in it
	if s.beaconKey.Count() == 0 {
		// feloldjuk a vigiliát, mert nincs több treasure a swampban és a Destroy megkövetelei a Vigil feloldását
		s.CeaseVigil()
		s.Destroy()
		return nil
	}

	return nil

}

// CloneAndDeleteExpiredTreasures retrieves one or more expired Treasures from the Swamp based on their expiration time and removes them.
// Use this function carefully as it deletes the Treasures from the Swamp.
func (s *swamp) CloneAndDeleteExpiredTreasures(howMany int32) ([]treasure.Treasure, error) {

	// set the last interaction time to the current time
	atomic.StoreInt64(&s.lastInteractionTime, time.Now().UnixNano())

	// build the expirationTimeIndex if it is not built yet
	s.buildBeacon(s.expirationTimeBeaconASC, s.expirationTimeBeaconDESC, BeaconTypeExpirationTime)

	// shift the expired treasures from the swamp
	shiftedTreasures := s.expirationTimeBeaconASC.ShiftExpired(int(howMany))

	// delete the shifted treasures from the other indexes
	for _, d := range shiftedTreasures {
		// delete the treasure from the beaconKey
		// A lejárt treasureok esetében mindig valódi törlést végzünk és nem csak "törölt" flaggel jelöljük meg a treasuret
		s.deleteHandler(d.GetKey(), false)
	}

	// return with the shifted treasures
	return shiftedTreasures, nil
}

// TreasureExists Checks if the given key exists in the swamp.
// This function can be useful before attempting to bury a new treasure or unearth an existing one.
func (s *swamp) TreasureExists(key string) bool {
	// set the last interaction time to the current time
	atomic.StoreInt64(&s.lastInteractionTime, time.Now().UnixNano())
	return s.beaconKey.IsExists(key)
}

// WriteTreasuresToFilesystem writes all new, modified or deleted treasures to the filesystem by the chroniclerInterface
func (s *swamp) WriteTreasuresToFilesystem() {
	// set the last interaction time to the current time
	atomic.StoreInt64(&s.lastInteractionTime, time.Now().UnixNano())
	if atomic.LoadInt32(&s.inMemorySwamp) == 0 {
		s.fileWriterHandler(false)
	}
}

// fileWriterHandler writes all new, modified or deleted treasures to the chroniclerInterface
// and sets the writeActive to 0 when it is finished and empty the treasuresWaitingForWriter slice
// ONLY ONE fileWriterHandler can be active at the same time to prevent the concurrent loader writes to the chroniclerInterface
func (s *swamp) fileWriterHandler(isCloseWrite bool) {

	// ha nem zárási esemény miatt akarjuk a kiírást megvalósítani
	if !isCloseWrite {

		func() {
			s.writerLock.Lock()
			defer s.writerLock.Unlock()
			if atomic.LoadInt32(&s.isFilesystemWritingActive) == 1 {
				return
			}
			atomic.StoreInt32(&s.isFilesystemWritingActive, 1)
		}()

		// feloldjuk az írási lockot mert még nem kell zárjuk a swampot
		defer atomic.StoreInt32(&s.isFilesystemWritingActive, 0)

	} else {

		// ha zárási esemény miatt akarjuk a kiírást megvalósítani
		// akkor elég, ha csak beállítjuk az aktív írást, 1-re, mert később
		// már nem lesz szükség rá.
		atomic.StoreInt32(&s.isFilesystemWritingActive, 1)

	}

	// if there is no treasures waiting for write, then return
	if s.treasuresWaitingForWriter.Count() == 0 {
		return
	}

	var treasuresToWrite []treasure.Treasure
	s.treasuresWaitingForWriter.Iterate(func(t treasure.Treasure) bool {
		treasuresToWrite = append(treasuresToWrite, t)
		return true
	}, beacon.IterationTypeKey)

	// delete the treasures from the swamp and from the chroniclerInterface too
	for _, t := range treasuresToWrite {
		// delete the treasure from the treasuresWaitingForWriter index
		s.treasuresWaitingForWriter.Delete(t.GetKey())
	}

	// A Write funkció megvárja ameddig az előző write befejezi a munkáját, így nem kell
	// külön szinkronizálni a két írási folyamatot
	s.chroniclerInterface.Write(treasuresToWrite)

}

// Info returns detailed information about the swamp
func (s *swamp) sendSwampInfo() {
	if atomic.LoadInt32(&s.isInformationSendingActive) == 0 {
		return
	}

	message := &Info{
		SwampName:   s.GetName(),
		AllElements: uint64(s.beaconKey.Count()),
	}

	s.swampInfoCallback(message)

}

// deleteHandler deletes the treasure from the swamp
func (s *swamp) deleteHandler(key string, shadowDelete bool) (deletedTreasure treasure.Treasure) {

	// clone the treasure itself to the clonedTreasure
	treasureObj := s.beaconKey.Get(key)
	if treasureObj == nil {
		return nil
	}

	guardID := treasureObj.StartTreasureGuard(true, guard.BodyAuthID)
	defer treasureObj.ReleaseTreasureGuard(guardID)

	// Még változtatás előtt lemásoljuk a Treasure-t, hogy egy clone-t készíthessünk róla, hogy a törölt treasure-t minden
	// adatával együtt vissza tudjuk adni.
	clonedTreasure := treasureObj.Clone(guardID)

	// remove the treasure from the treasuresWaitingForWriter slice if the treasure does not have a loader pointer
	// because it is meaning the treasure is not saved yet to the chroniclerInterface, but it is deleted from the swamp
	if treasureObj.GetFileName() == nil {
		// delete the treasure from the treasuresWaitingForWriter index
		s.treasuresWaitingForWriter.Delete(key)
	} else {
		// set the treasure for deletion
		// todo: itt meg kell oldani, hogy a törlésnél legyen kérhető a shadow delete is.
		treasureObj.BodySetForDeletion(guardID, "", shadowDelete)
		s.treasuresWaitingForWriter.Add(treasureObj)
		// beállítjuk az utolsó módosítás dátumát a metában
		s.metadataInterface.SetUpdatedAt()
	}

	// delete the treasure from the beaconKey after cloned the treasure
	s.beaconKey.Delete(key)
	// delete the treasure from all active indexes
	s.deleteTreasureFromBeacons(key)

	// send the deleted event_channel_handler to the neen
	s.sendDeletedEventToClient(clonedTreasure)
	s.sendSwampInfo()

	return treasureObj

}

// sendDeletedEventToClient sends the deleted event_channel_handler to the Hydra
func (s *swamp) sendDeletedEventToClient(d treasure.Treasure) {

	if atomic.LoadInt32(&s.isEventSendingActive) == 0 {
		return
	}

	e := &Event{
		SwampName:       s.GetName(),
		Treasure:        nil,
		OldTreasure:     nil,
		DeletedTreasure: d,
		EventTime:       time.Now().UTC().UnixNano(),
		StatusType:      treasure.StatusDeleted,
	}

	s.swampEventCallback(e)

}

// addTreasureToBeacons - add treasures to all the indexes
func (s *swamp) addTreasureToBeacons(d treasure.Treasure) {

	// try to add the treasure to the keyBeacon
	s.addToKeyBeacon(d)
	if d.GetCreatedAt() != 0 {
		s.addToCreationTimeBeacon(d)
	}
	if d.GetModifiedAt() != 0 {
		s.addToUpdateTimeBeacon(d)
	}
	if d.GetExpirationTime() != 0 {
		s.addToExpirationTimeBeacon(d)
	}

	// value beacon
	s.addToValueBeacon(d)

}

// deleteTreasureFromBeacons - delete the treasure from all beacons if the treasure is exists in the beacon
func (s *swamp) deleteTreasureFromBeacons(key string) {
	// delete the key from the beacon only if the beacon is initialized
	s.deleteTreasureIfBeaconInitialized(s.keyBeaconASC, key)
	s.deleteTreasureIfBeaconInitialized(s.keyBeaconDESC, key)
	s.deleteTreasureIfBeaconInitialized(s.creationTimeBeaconASC, key)
	s.deleteTreasureIfBeaconInitialized(s.creationTimeBeaconDESC, key)
	s.deleteTreasureIfBeaconInitialized(s.updateTimeBeaconASC, key)
	s.deleteTreasureIfBeaconInitialized(s.updateTimeBeaconDESC, key)
	s.deleteTreasureIfBeaconInitialized(s.expirationTimeBeaconASC, key)
	s.deleteTreasureIfBeaconInitialized(s.expirationTimeBeaconDESC, key)
	s.deleteTreasureIfBeaconInitialized(s.valueBeaconASC, key)
	s.deleteTreasureIfBeaconInitialized(s.valueBeaconDESC, key)

}

// deleteTreasureIfBeaconInitialized - delete the key from the beacon only if the beacon is initialized
func (s *swamp) deleteTreasureIfBeaconInitialized(b beacon.Beacon, key string) {
	if b.IsInitialized() {
		b.Delete(key)
	}
}

// findInCreationTimeBeacon - find the treasures in the creationTimeBeaconASC or creationTimeBeaconDESC slice
// Build the two indexes if they are not exists or the indexes are empty
func (s *swamp) findInCreationTimeBeacon(order BeaconOrder, from int32, limit int32) ([]treasure.Treasure, error) {
	s.buildBeacon(s.creationTimeBeaconASC, s.creationTimeBeaconDESC, BeaconTypeCreationTime)
	switch order {
	case IndexOrderAsc:
		return s.creationTimeBeaconASC.GetManyFromOrderPosition(int(from), int(limit))
	case IndexOrderDesc:
		return s.creationTimeBeaconDESC.GetManyFromOrderPosition(int(from), int(limit))
	default:
		return nil, errors.New("invalid order")
	}
}

// findInUpdateTimeBeacon - find the treasures in the updateTimeBeaconASC or updateTimeBeaconDESC slice
// Build the two indexes if they are not exists or the indexes are empty
func (s *swamp) findInUpdateTimeBeacon(order BeaconOrder, from int32, limit int32) ([]treasure.Treasure, error) {
	s.buildBeacon(s.updateTimeBeaconASC, s.updateTimeBeaconDESC, BeaconTypeUpdateTime)
	switch order {
	case IndexOrderAsc:
		return s.updateTimeBeaconASC.GetManyFromOrderPosition(int(from), int(limit))
	case IndexOrderDesc:
		return s.updateTimeBeaconDESC.GetManyFromOrderPosition(int(from), int(limit))
	default:
		return nil, errors.New("invalid order")
	}
}

// findInKeyBeacon - find the treasures in the keyBeaconASC or keyBeaconDESC slice
// Build the two indexes if they are not exists or the indexes are empty
func (s *swamp) findInKeyBeacon(order BeaconOrder, from int32, limit int32) ([]treasure.Treasure, error) {
	s.buildBeacon(s.keyBeaconASC, s.keyBeaconDESC, BeaconTypeKey)
	switch order {
	case IndexOrderAsc:
		return s.keyBeaconASC.GetManyFromOrderPosition(int(from), int(limit))
	case IndexOrderDesc:
		return s.keyBeaconDESC.GetManyFromOrderPosition(int(from), int(limit))
	default:
		return nil, errors.New("invalid order")
	}
}

// findInExpirationTimeBeacon - find the treasures in the expirationTimeBeaconASC or expirationTimeBeaconDESC slice
// Build the two indexes if they are not exists or the indexes are empty
func (s *swamp) findInExpirationTimeBeacon(order BeaconOrder, from int32, limit int32) ([]treasure.Treasure, error) {
	s.buildBeacon(s.expirationTimeBeaconASC, s.expirationTimeBeaconDESC, BeaconTypeExpirationTime)
	switch order {
	case IndexOrderAsc:
		return s.expirationTimeBeaconASC.GetManyFromOrderPosition(int(from), int(limit))
	case IndexOrderDesc:
		return s.expirationTimeBeaconDESC.GetManyFromOrderPosition(int(from), int(limit))
	default:
		return nil, errors.New("invalid order")
	}
}

// findInValueBeacon - find the treasures in the valueIntBeaconASC or valueIntBeaconDESC slice
// Build the two indexes if they are not exists or the indexes are empty
func (s *swamp) findInValueBeacon(order BeaconOrder, bc BeaconType, from int32, limit int32) ([]treasure.Treasure, error) {
	s.buildBeacon(s.valueBeaconASC, s.valueBeaconDESC, bc)
	switch order {
	case IndexOrderAsc:
		return s.valueBeaconASC.GetManyFromOrderPosition(int(from), int(limit))
	case IndexOrderDesc:
		return s.valueBeaconDESC.GetManyFromOrderPosition(int(from), int(limit))
	default:
		return nil, errors.New("invalid order")
	}
}

// -- helper functions for beacons -----------------------------------------------------
// ------------------------------------------------------------------------------------
func (s *swamp) buildBeacon(beaconASC beacon.Beacon, beaconDESC beacon.Beacon, bc BeaconType) {

	// build the index only if it is not initialized
	if beaconASC.IsInitialized() && beaconASC.IsInitialized() {
		return
	}

	if !beaconASC.IsInitialized() {
		beaconASC.SetInitialized(true)
		beaconASC.PushManyFromMap(s.beaconKey.GetAll())
		var err error
		switch bc {
		case BeaconTypeCreationTime:
			err = beaconASC.SortByCreationTimeAsc()
		case BeaconTypeUpdateTime:
			err = beaconASC.SortByUpdateTimeAsc()
		case BeaconTypeExpirationTime:
			err = beaconASC.SortByExpirationTimeAsc()
		case BeaconTypeValueUint8:
			err = beaconASC.SortByValueUint8ASC()
		case BeaconTypeValueUint16:
			err = beaconASC.SortByValueUint16ASC()
		case BeaconTypeValueUint32:
			err = beaconASC.SortByValueUint32ASC()
		case BeaconTypeValueUint64:
			err = beaconASC.SortByValueUint64ASC()
		case BeaconTypeValueInt8:
			err = beaconASC.SortByValueInt8ASC()
		case BeaconTypeValueInt16:
			err = beaconASC.SortByValueInt16ASC()
		case BeaconTypeValueInt32:
			err = beaconASC.SortByValueInt32ASC()
		case BeaconTypeValueInt64:
			err = beaconASC.SortByValueInt64ASC()
		case BeaconTypeValueFloat32:
			err = beaconASC.SortByValueFloat32ASC()
		case BeaconTypeValueFloat64:
			err = beaconASC.SortByValueFloat64ASC()
		case BeaconTypeValueString:
			err = beaconASC.SortByValueStringASC()
		case BeaconTypeKey:
			err = beaconASC.SortByKeyAsc()
		default:
			err = beaconASC.SortByKeyAsc()
		}
		if err != nil {
			beaconASC.SetInitialized(false)
			slog.Error("failed to sort keyBeaconASC", "error", err)
		}
	}

	if !beaconDESC.IsInitialized() {
		beaconDESC.SetInitialized(true)
		beaconDESC.PushManyFromMap(s.beaconKey.GetAll())
		var err error
		switch bc {
		case BeaconTypeCreationTime:
			err = beaconDESC.SortByCreationTimeDesc()
		case BeaconTypeUpdateTime:
			err = beaconDESC.SortByUpdateTimeDesc()
		case BeaconTypeExpirationTime:
			err = beaconDESC.SortByExpirationTimeDesc()
		case BeaconTypeValueUint8:
			err = beaconDESC.SortByValueUint8DESC()
		case BeaconTypeValueUint16:
			err = beaconDESC.SortByValueUint16DESC()
		case BeaconTypeValueUint32:
			err = beaconDESC.SortByValueUint32DESC()
		case BeaconTypeValueUint64:
			err = beaconDESC.SortByValueUint64DESC()
		case BeaconTypeValueInt8:
			err = beaconDESC.SortByValueInt8DESC()
		case BeaconTypeValueInt16:
			err = beaconDESC.SortByValueInt16DESC()
		case BeaconTypeValueInt32:
			err = beaconDESC.SortByValueInt32DESC()
		case BeaconTypeValueInt64:
			err = beaconDESC.SortByValueInt64DESC()
		case BeaconTypeValueFloat32:
			err = beaconDESC.SortByValueFloat32DESC()
		case BeaconTypeValueFloat64:
			err = beaconDESC.SortByValueFloat64DESC()
		case BeaconTypeValueString:
			err = beaconDESC.SortByValueStringDESC()
		case BeaconTypeKey:
			err = beaconDESC.SortByKeyDesc()
		default:
			err = beaconDESC.SortByKeyDesc()
		}
		if err != nil {
			beaconDESC.SetInitialized(false)
			slog.Error("failed to sort keyBeaconDESC", "error", err)
		}
	}

}

func (s *swamp) addToKeyBeacon(treasureInterface treasure.Treasure) {
	// check if the index is already built
	// if not, then we don't need to add the treasures to the index
	if !s.keyBeaconASC.IsInitialized() {
		return
	}
	s.keyBeaconASC.Add(treasureInterface)
	err := s.keyBeaconASC.SortByKeyAsc()
	if err != nil {
		slog.Error("failed to sort keyBeaconASC", "error", err)
	}
	s.keyBeaconDESC.Add(treasureInterface)
	err = s.keyBeaconDESC.SortByKeyDesc()
	if err != nil {
		slog.Error("failed to sort keyBeaconDESC", "error", err)
	}
}

// addToCreationTimeBeacon - add the treasures to the creationTimeBeaconASC and creationTimeBeaconDESC slices if the treasure
// is not already in the slices
func (s *swamp) addToCreationTimeBeacon(treasureInterface treasure.Treasure) {
	// check if the index is already built
	// if not, then we don't need to add the treasures to the index
	if !s.creationTimeBeaconASC.IsInitialized() {
		return
	}
	s.creationTimeBeaconASC.Add(treasureInterface)
	err := s.creationTimeBeaconASC.SortByCreationTimeAsc()
	if err != nil {
		slog.Error("failed to sort creationTimeBeaconASC", "error", err)
	}
	s.creationTimeBeaconDESC.Add(treasureInterface)
	err = s.creationTimeBeaconDESC.SortByCreationTimeDesc()
	if err != nil {
		slog.Error("failed to sort creationTimeBeaconDESC", "error", err)
	}
}
func (s *swamp) addToUpdateTimeBeacon(treasureInterface treasure.Treasure) {
	// check if the index is already built
	// if not, then we don't need to add the treasures to the index
	if !s.updateTimeBeaconASC.IsInitialized() {
		return
	}
	s.updateTimeBeaconASC.Add(treasureInterface)
	err := s.updateTimeBeaconASC.SortByUpdateTimeAsc()
	if err != nil {
		slog.Error("failed to sort updateTimeBeaconASC", "error", err)
	}
	s.updateTimeBeaconDESC.Add(treasureInterface)
	err = s.updateTimeBeaconDESC.SortByUpdateTimeDesc()
	if err != nil {
		slog.Error("failed to sort updateTimeBeaconDESC", "error", err)
	}

}
func (s *swamp) addToExpirationTimeBeacon(treasureInterface treasure.Treasure) {
	// check if the index is already built
	// if not, then we don't need to add the treasures to the index
	if !s.expirationTimeBeaconASC.IsInitialized() {
		return
	}
	s.expirationTimeBeaconASC.Add(treasureInterface)
	err := s.expirationTimeBeaconASC.SortByExpirationTimeAsc()
	if err != nil {
		slog.Error("failed to sort expirationTimeBeaconASC", "error", err)
	}

	s.expirationTimeBeaconDESC.Add(treasureInterface)
	err = s.expirationTimeBeaconDESC.SortByExpirationTimeDesc()
	if err != nil {
		slog.Error("failed to sort expirationTimeBeaconDESC", "error", err)
	}

}
func (s *swamp) addToValueBeacon(treasureInterface treasure.Treasure) {
	// check if the index is already built
	// if not, then we don't need to add the treasures to the index
	if !s.valueBeaconASC.IsInitialized() {
		return
	}
	s.valueBeaconASC.Add(treasureInterface)
	err := s.valueBeaconASC.SortByValueInt64ASC()
	if err != nil {
		slog.Error("failed to sort valueIntBeaconASC", "error", err)
	}
	s.valueBeaconDESC.Add(treasureInterface)
	err = s.valueBeaconDESC.SortByValueInt64DESC()
	if err != nil {
		slog.Error("failed to sort valueIntBeaconDESC", "error", err)
	}
}

// sendEventToHydra sends the event to the ManagerInterface
func (s *swamp) sendEventToHydra(newTreasure, oldTreasure treasure.Treasure, status treasure.TreasureStatus) {

	if atomic.LoadInt32(&s.isEventSendingActive) == 0 {
		return
	}

	swampName := s.GetName()

	// create the event_channel_handler for the ManagerInterface database
	event := &Event{
		SwampName:       swampName,
		Treasure:        newTreasure,
		OldTreasure:     oldTreasure,
		DeletedTreasure: nil,
		EventTime:       time.Now().UTC().UnixNano(),
		StatusType:      status,
	}

	s.swampEventCallback(event)

}

func (s *swamp) startWriteListener() {

	s.mu.RLock()
	writeInterval := s.writeInterval
	s.mu.RUnlock()

	writeTicker := time.NewTicker(writeInterval)
	defer writeTicker.Stop()

	for {
		select {
		case <-s.goRoutineContext.Done():
			// return
			return
		case <-writeTicker.C:

			func() {

				// zárjuk a lockot, hogy a leállító művelet várakozzon a file kiírásának befejezésére és csak utána legyen képes
				// leállítani a swampot
				s.closeWriteMutex.Lock()
				defer s.closeWriteMutex.Unlock()

				if atomic.LoadInt32(&s.isFilesystemWritingActive) == 1 || atomic.LoadInt32(&s.closing) == 1 || s.treasuresWaitingForWriter.Count() == 0 {
					// nem kell kiírni a fileba, mert vagy már kiírás alatt van, vagy már le van állítva a swamp, vagy nincs mit kiírni így
					// egyből fel is szabadítjuk a lockot
					return
				}

				// kiírjuk a fileba a várakozó treasureket
				s.fileWriterHandler(false)

			}()

		}
	}

}

func (s *swamp) startCloseListener() {

	closeGapDuration := 1 * time.Second

	closeTicker := time.NewTicker(closeGapDuration)
	defer closeTicker.Stop()

	for {
		select {
		case <-s.goRoutineContext.Done():

			// return
			return

		case <-closeTicker.C:

			// closeMonitoring monitors the object and closes the swamp when the minimum open time is passed and if all
			// goroutines are finished their work
			currentTime := time.Now()
			lastInteractionTime := time.Unix(0, atomic.LoadInt64(&s.lastInteractionTime))

			func() {

				// lockolunk, hogy az ellenőrzés ideje alatt ne tudjon leállítani senki és írni se tudjon senki, de a fiepointer eventek se kerüljenek be,
				// mert azokat is ki kell írni a fileba.
				s.closeWriteMutex.Lock()
				defer s.closeWriteMutex.Unlock()

				// Ha ez egy in-memory swamp, akkor nem kell vizsgálni a lezárásnál, hogy a isFilesystemWritingActive 1-e, mert nincs
				// filerednszer szintű írás, csak a memóriában tároljuk a treasureket és csak azt kell ellenőrizni, hogy nincs-e aktív tranzakció
				// és nincs-e aktív vigília és az utolsó interakció óta eltelt idő nagyobb-e mint a closeAfterIdle
				// és ezt kjövetően már be is lehet zárni a swampot
				if atomic.LoadInt32(&s.inMemorySwamp) == 1 {
					if !s.Vigil.HasActiveVigils() && atomic.LoadInt32(&s.closing) == 0 && currentTime.After(lastInteractionTime.Add(s.closeAfterIdle+closeGapDuration)) {
						s.Close()
					}
				} else {
					if atomic.LoadInt32(&s.isFilesystemWritingActive) == 0 && !s.Vigil.HasActiveVigils() && atomic.LoadInt32(&s.closing) == 0 && currentTime.After(lastInteractionTime.Add(s.closeAfterIdle+closeGapDuration)) {
						// a swampot éppp nem írja senki, nincs aktív tranzakció, nem zárjuk éppen le és megfelelünk annak a követelménynek is, hogy
						// az utoljára történt interakció óta eltelt idő nagyobb legyen mint a closeAfterIdle, így a swamp leállítható biztonságosan
						s.Close()
					}
				}

			}()

		}
	}

}

// FilePointerCallbackFunction ez egy callback funkció, amit a chroniclerInterface hív meg, amikor egy filepointer
// event érkezik
func (s *swamp) FilePointerCallbackFunction(filePointerEvents []*chronicler.FileNameEvent) error {

	// ne tegyünk semmit, ha a swamp éppen leállítás alatt van
	if atomic.LoadInt32(&s.closing) == 1 {
		return errors.New("swamp is closing")
	}

	if filePointerEvents == nil {
		return nil
	}

	for _, e := range filePointerEvents {
		if s.beaconKey != nil && e != nil && e.TreasureKey != "" {

			func() {
				treasureObj := s.beaconKey.Get(e.TreasureKey)
				if treasureObj == nil {
					return
				}
				lockerID := treasureObj.StartTreasureGuard(true, guard.BodyAuthID)
				defer treasureObj.ReleaseTreasureGuard(lockerID)
				// set the loader pointer of the treasure
				treasureObj.BodySetFileName(lockerID, e.FileName)
			}()

		}
	}

	return nil

}
