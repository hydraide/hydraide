package hydra

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/hydraide/hydraide/app/core/filesystem"
	"github.com/hydraide/hydraide/app/core/hydra/lock"
	"github.com/hydraide/hydraide/app/core/hydra/swamp"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/metadata"
	"github.com/hydraide/hydraide/app/core/safeops"
	"github.com/hydraide/hydraide/app/core/settings"
	"github.com/hydraide/hydraide/app/core/settings/setting"
	"github.com/hydraide/hydraide/app/name"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Hydra interface {

	// GetLocker returns the locker interface
	GetLocker() lock.Lock

	// SummonSwamp retrieves a single Swamp object based on its deterministic Island location,
	// and loads all associated Treasures from disk into memory.
	//
	// Parameters:
	// - `islandID`: the ID of the Island (i.e. top-level storage folder) where the Swamp physically resides.
	//               This ID is always provided by the client. It is deterministically computed from the full Swamp name
	//               (Sanctuary / Realm / Swamp) using a fixed hash function. It is not the HydrAIDE server's
	//               responsibility to calculate this value.
	// - `swampName`: the unique logical name of the Swamp.
	//
	// 🧭 Why does the IslandID come from the client?
	// - The routing logic is entirely client-driven: only the client computes the IslandID via hashing.
	// - This ensures that the HydrAIDE server remains stateless and does not make routing decisions.
	// - The same Swamp name will always map to the same IslandID, enabling deterministic access.
	//
	// 🏝️ Why is this important for scaling?
	// - Islands are the smallest physical storage units in HydrAIDE.
	// - They can be freely moved between servers (e.g. via rsync or ZFS) to balance load.
	// - Once moved, only the client’s IslandID → server mapping needs to change — Swamp names remain untouched.
	// - This allows orchestrator-free horizontal scaling and seamless server rebalancing.
	//
	// 💾 Behavior:
	// - If the Swamp is already in memory, the existing instance is returned.
	// - Otherwise, the function uses the provided IslandID and SwampName to construct the disk path,
	//   loads all associated Treasures, and returns the hydrated Swamp.
	//
	// Returns:
	// - A `swamp.Swamp` interface to interact with the loaded Swamp
	// - An error if the Swamp is missing or invalid
	//
	// Example:
	//
	//     island := swampName.GetIslandID(1000)
	//     mySwamp, err := myHydra.SummonSwamp(ctx, island, swampName)
	//     if err != nil {
	//         log.Fatal(err)
	//     }
	//
	//     // Interact with Swamp and its Treasures fully loaded into memory
	SummonSwamp(ctx context.Context, islandID uint64, swampName name.Name) (swampObj swamp.Swamp, err error)

	// IsExistSwamp checks whether the specified Swamp physically exists in the system.
	// This function does not load the Swamp into memory and does not create a new Swamp if it is missing —
	// it performs a non-intrusive existence check, either in-memory or on-disk.
	//
	// Parameters:
	// - `islandID`: the deterministic storage location (Island) where the Swamp is expected to reside.
	//               This must be computed on the client side using the full Swamp name.
	//               See the `SummonSwamp` function for detailed explanation on IslandID usage.
	// - `swampName`: the logical name of the Swamp to check.
	//
	// Returns:
	// - `true` if the Swamp exists (either in memory or on disk)
	// - `false` if the Swamp does not exist
	// - An error if an access or I/O issue occurs
	//
	// Example:
	//
	//     island := swampName.GetIslandID(1000)
	//     exists, err := myHydra.IsExistSwamp(island, swampName)
	//     if err != nil {
	//         log.Fatal(err)
	//     }
	//     if exists {
	//         // Swamp exists
	//     } else {
	//         // Swamp does not exist
	//     }
	IsExistSwamp(islandID uint64, swampName name.Name) (bool, error)

	// SubscribeToSwampEvents enables a Head to subscribe to events from a specific Swamp using a callback function,
	// allowing real-time monitoring or triggering business logic. This is a NON blocking function.
	//
	// This function takes three parameters:
	// - clientID of type uuid.UUID: Uniquely identifies the subscribing Head.
	// - swampName of type name.Name: Specifies the Swamp whose events the Head is interested in.
	// - subscriberEventCallbackFunction of type func(event *swamp.Event): A callback function that is invoked with each event related to the specified Swamp.
	//
	// Once subscribed, the provided callback function is executed in real-time whenever an event occurs in the specified Swamp.
	// This approach simplifies handling events by abstracting away channel management.
	//
	// Example:
	//
	//     myCallback := func(event *swamp.Event) {
	//         // Handle the received event
	//         ...
	//     }
	//
	//     // Subscribe to events of a specific Swamp
	//     err := myHydra.SubscribeToSwampEvents(uuid.New(), name.New("someSwamp"), myCallback)
	//     if err != nil {
	//         // Handle subscription error
	//         ...
	//     }
	//
	// Returns:
	// - An error value: Returns nil if the subscription was successful, otherwise returns an error detailing the issue.
	//
	// Use-cases:
	// 1. Real-time monitoring of a specific Swamp, enabling a Head to react quickly to changes.
	// 2. Triggering other business logic or actions based on events occurring within a Swamp.
	SubscribeToSwampEvents(clientID uuid.UUID, swampName name.Name, subscriberEventCallbackFunction func(event *swamp.Event)) (err error)

	// UnsubscribeFromSwampEvents allows a Head to unsubscribe from events of a specific Swamp, effectively stopping
	// real-time monitoring or triggering of business logic based on those events.
	//
	// This function takes two parameters:
	// - clientID of type uuid.UUID to uniquely identify the unsubscribing Head.
	// - swampName of type name.Name to specify which Swamp's events the Head wants to unsubscribe from.
	//
	// Once unsubscribed, the Head will no longer receive any events related to the specified Swamp. This is useful
	// when a Head no longer needs to monitor the Swamp, or when business logic tied to the Swamp's events is no longer relevant.
	//
	// Example:
	//
	//     // Unsubscribe from events of a specific Swamp
	//     myHydra.UnsubscribeFromSwampEvents(myExistingUUID, name.New("someSwamp"))
	//
	//     // The Head will no longer receive events from the specified Swamp
	//
	// Returns:
	// - No direct return value; the Head will stop receiving events related to the specified Swamp.
	//
	// Use-cases:
	// 1. Ending real-time monitoring of a specific Swamp when it's no longer needed.
	// 2. Preventing unnecessary triggering of business logic or actions that were previously dependent on the Swamp's events.
	//
	// It's in our interest to manage resources efficiently; unsubscribing when monitoring is no longer necessary helps in resource optimization.
	UnsubscribeFromSwampEvents(clientID uuid.UUID, swampName name.Name) (err error)

	// SubscribeToSwampInfo allows a Head to subscribe to specific updates about the state of a given Swamp, particularly
	// changes in the number of Treasures stored within.
	//
	// This function takes three parameters:
	// - clientID of type uuid.UUID: Uniquely identifies the subscribing Head.
	// - swampName of type name.Name: Specifies the Swamp whose state updates the Head wants to receive.
	// - subscriberInfoCallbackFunction of type func(info *swamp.Info): A callback function that is invoked whenever a significant state change occurs.
	//
	// The function provides updates only when the total number of Treasures in the Swamp decreases or becomes zero.
	// This approach ensures performance optimization by limiting notifications to significant changes.
	//
	// If a Swamp with 100 Treasures is destroyed, the last update will indicate that the total count of Treasures is now zero.
	// The subscriber will not be notified of each individual Treasure being destroyed.
	//
	// Example:
	//
	//     myCallback := func(info *swamp.Info) {
	//         // Process the new info update
	//         ...
	//     }
	//
	//     // Subscribe to information updates of a specific Swamp
	//     err := myHydra.SubscribeToSwampInfo(uuid.New(), name.New("someSwamp"), myCallback)
	//     if err != nil {
	//         // Handle subscription error
	//         ...
	//     }
	//
	// Returns:
	// - An error value: Returns nil if the subscription was successful, otherwise returns an error detailing the issue.
	//
	// Use-cases:
	// 1. Real-time analytics related to the Swamp.
	// 2. Dashboard updates to reflect changes in the Swamp's state.
	//
	// It's in our interest to ensure performance efficiency; limiting updates to only significant changes in the Swamp helps achieve this.
	SubscribeToSwampInfo(clientID uuid.UUID, swampName name.Name, subscriberInfoCallbackFunction func(info *swamp.Info)) (err error)

	// UnsubscribeFromSwampInfo allows a Head to unsubscribe from receiving updates about a specific Swamp's state.
	//
	// Parameters:
	// - clientID of type uuid.UUID: Uniquely identifies the unsubscribing Head.
	// - swampName of type name.Name: Specifies which Swamp the Head wants to stop receiving updates about.
	//
	// This function is particularly useful for performance optimization and resource management. Once a Head is unsubscribed,
	// it will no longer receive updates from the specified Swamp, freeing up computational and network resources.
	//
	// Example:
	//
	//     // Unsubscribe from updates of a specific Swamp
	//     myHydra.UnsubscribeFromSwampInfo(myExistingUUID, name.NewName("someSwamp"))
	//
	// Returns:
	// - No direct return value; stops updates from being sent to the Head for the specified Swamp.
	//
	// Use-cases:
	// 1. To stop receiving real-time analytics when no longer required.
	// 2. To manage resources effectively by unsubscribing from Swamps that are no longer of interest.
	//
	// Unsubscribing a Head when updates are no longer needed is in our interest to maintain efficient system performance.
	UnsubscribeFromSwampInfo(clientID uuid.UUID, swampName name.Name) (err error)

	// ListActiveSwamps retrieves and returns a list of currently active Swamps within the system.
	//
	// Parameters: None
	//
	// This function scans the internal data structures to identify Swamps that are currently in an active state,
	// meaning they are either engaged in data processing or are available for task assignment.
	//
	// Example:
	//
	//     // Retrieve the list of active swamps
	//     activeSwamps := myHydra.ListActiveSwamps()
	//
	// Returns:
	// - []string: An array of strings, where each string is the name or identifier of an active Swamp.
	//
	// Use-cases:
	// 1. To monitor the system's current state for debugging or analytical purposes.
	// 2. To identify which Swamps are available for immediate task assignments.
	// 3. To generate reports or dashboards that provide insights into system performance and utilization.
	//
	// Utilizing the ListActiveSwamps function is in our interest when we want to gain visibility into
	// the operational state of our Swamps, thereby enhancing our ability to manage resources and tasks effectively.
	ListActiveSwamps() []string

	// CountActiveSwamps calculates and returns the total number of currently active Swamps in the system.
	//
	// Parameters: None
	//
	// This function is responsible for iterating through the system's internal data structures to count
	// the number of Swamps that are currently active, meaning they are either processing tasks or are ready for new assignments.
	//
	// Example:
	//
	//     // Retrieve the count of active swamps
	//     activeSwampCount := myHydra.CountActiveSwamps()
	//
	// Returns:
	// - int: The total number of active Swamps as an integer.
	//
	// Use-cases:
	// 1. For real-time monitoring of system resource utilization.
	// 2. To help with load balancing decisions by gauging the system’s capacity.
	// 3. To quickly assess the need for scaling the system up or down based on active Swamps.
	//
	// Utilizing the CountActiveSwamps function is in our interest for effective resource allocation and system monitoring,
	// ensuring that we can respond swiftly to changing operational conditions.
	CountActiveSwamps() int

	// GracefulStop cleanly shuts down the server by finishing all ongoing processes and freeing up resources.
	//
	// Important: DO NOT CALL THIS FUNCTION DIRECTLY.
	//
	// This method is invoked automatically by the graceful stop package whenever the server is shutting down.
	// It ensures that all active connections are closed, ongoing tasks are completed, and resources are released,
	// thus enabling a graceful termination of the application.
	//
	// Use-cases:
	// 1. To ensure that no data is lost or corrupted during unplanned shutdowns.
	// 2. To maintain system integrity by safely concluding any in-flight transactions.
	// 3. To prevent abrupt termination that might cause issues in interconnected services or databases.
	//
	// Employing GracefulStop is in our interest as it allows us to maintain high availability and reliability
	// in our services, ensuring a seamless user experience even during maintenance periods.
	GracefulStop()
}

const (
	ErrorHydraIsShuttingDown = "hydra is shutting down"
)

type hydra struct {
	mu           sync.RWMutex
	shuttingDown int32 // Hydra shutting down flag

	// maps that we need to protect by mutexes
	// swamps           map[string]swamp.Swamp
	swamps sync.Map
	// eventSubscribers map[string]map[uuid.UUID]chan *swamp.Event
	eventSubscribers sync.Map
	// infoSubscribers  map[string]map[uuid.UUID]chan *swamp.Info
	infoSubscribers sync.Map

	// summoningSwamps csak olyan swampokat tárol, amiket éppen summonolunk, hogy két rutin ne summonolhassa ugyanazt
	// a swampot, különben képesek lennének egyszerre létrehozni, ugyanazt a swampot. Így ha az egyik summonolja a swampot,
	// akkor meg kell várja a másik, hogy az első visszakapja azt.
	summoningSwamps sync.Map

	// interfaces
	elysiumInterface  safeops.Safeops
	settingsInterface settings.Settings

	// channels
	eventChannel      chan *swamp.Event
	closeEventChannel chan name.Name
	infoChannel       chan *swamp.Info
	// egyedi locker interface
	lockerInterface     lock.Lock
	filesystemInterface filesystem.Filesystem
}

// New creates a new hydra database
func New(settingsInterface settings.Settings, elysiumInterface safeops.Safeops,
	lockerInterface lock.Lock, filesystemInterface filesystem.Filesystem) Hydra {

	h := &hydra{
		// set interfaces
		settingsInterface: settingsInterface,
		elysiumInterface:  elysiumInterface,
		// set channels
		eventChannel:      make(chan *swamp.Event, 100000),
		closeEventChannel: make(chan name.Name, 100000),
		infoChannel:       make(chan *swamp.Info, 100000),

		// set locker interface
		lockerInterface:     lockerInterface,
		filesystemInterface: filesystemInterface,
	}

	return h

}

// GetLocker returns the locker interface
func (h *hydra) GetLocker() lock.Lock {
	return h.lockerInterface
}

// SwampWaiter struct-t használjuk a swampokra várakozásra
type SwampWaiter struct {
	cond  *sync.Cond
	ready bool
	count int32 // store the number of waiting goroutines
}

func newSwampWaiter() *SwampWaiter {
	return &SwampWaiter{
		cond:  sync.NewCond(&sync.Mutex{}),
		ready: false,
	}
}

// SummonSwamp creates a new swamp or returns the existing one
// mutexes: clean
func (h *hydra) SummonSwamp(ctx context.Context, islandID uint64, swampName name.Name) (swampObj swamp.Swamp, err error) {

	if atomic.LoadInt32(&h.shuttingDown) == 1 {
		return nil, errors.New(ErrorHydraIsShuttingDown)
	}

	// the swamp is actually summoning, so we need to wait for the other process to finish the summoning process
	// to prevent the same swamp to be summoned or created twice...
	// if the ok is true then the swamp is already summoning, so we need to wait for the other process to finish the summoning process
	// if the ok is false then the swamp is not summoning, so we can start the summoning process and store the swamp in the map
	// immediately
	result, _ := h.summoningSwamps.LoadOrStore(swampName.Get(), newSwampWaiter())
	waiter, _ := result.(*SwampWaiter)

	// lezárjuk a következő kódrészt, így csak egyetlen rutin futhatja egyszerre egy domain néven belül
	waiter.cond.L.Lock()
	for waiter.ready {
		select {
		case <-ctx.Done():
			// Ha a kontextus megszakad, jelezzük a többi várakozó goroutinnak, hogy ne várjanak tovább
			waiter.cond.Broadcast()
			waiter.cond.L.Unlock()
			return nil, ctx.Err() // Visszatérünk a kontextus hibaüzenetével
		default:
			atomic.AddInt32(&waiter.count, 1)
			waiter.cond.Wait()
		}
	}
	waiter.ready = true
	waiter.cond.L.Unlock()

	defer func() {
		// Swamp véglegesítése után
		waiter.cond.L.Lock()
		waiter.ready = false
		waiter.cond.Broadcast() // Értesítjük a többi várakozót
		waiter.cond.L.Unlock()
		// csökkentjük a várakozó goroutinok számát
		atomic.AddInt32(&waiter.count, -1)
		// ha nincs több várakozó goroutin, akkor töröljük a várakozó mapből a swampot
		if atomic.LoadInt32(&waiter.count) == 0 {
			h.summoningSwamps.Delete(swampName.Get())
		}
	}()

	var swampObject swamp.Swamp

	for {
		select {
		case <-ctx.Done():

			// we can not wait the summoning to finish, because the caller context is done
			// maybe this is a very long-running process, and the caller context is done meanwhile
			slog.Warn("the summoning context is done, summoning is cancelled", "swampName", swampName)

			return nil, errors.New("context is done")

		default:

			// get the info object of the swamp
			swampObject = h.getSwamp(swampName)

			// is the swamp is existing in the hydra
			if swampObject != nil {

				// existing, but it is closing now, so we need to wait for the closing
				// after the closing, the swamp object will be nil, and we will create a new swamp again
				// waiting for the swamp to be closed and deleted from the hydra's map
				// after the closing, the swamp object will be nil, and we will create a new swamp
				// Calling the IsClosing function – if it returns false, it prevents the swamp from closing immediately,
				// giving the caller time to set the BeginVigil instruction so that the swamp doesn't close
				// during the transaction.
				if swampObject.IsClosing() {

					var swampCloseError error

					// We wait until the swamp is removed from the hydra map, so that we can
					// summon it again later. This is a blocking function and will only release
					// once the swamp has been successfully closed.

					func() {

						// For safety reasons, we only wait a maximum of 30 seconds for the swamp to close.
						// If it doesn't close within this time, the swamp must be discarded, as it can't be safely shut down.
						waitingCtx, cancelFunc := context.WithTimeout(context.Background(), 30*time.Second)
						defer cancelFunc()

						// There are two cases where an error may occur:
						// 1. If the waitingCtx is done, which would indicate the swamp couldn't close within 30 seconds.
						// 2. If the swamp is not closing at all, for some reason.
						// In both cases, the swamp must be discarded, as it cannot be closed and the code cannot proceed.
						if closeErr := swampObject.WaitForGracefulClose(waitingCtx); closeErr != nil {
							slog.Error("the swamp can not be closed in 30 seconds, so we need to drop it", "swampName", swampName, "closeError", closeErr)
							swampCloseError = err
							return
						}

					}()

					// Return with an error, because closing took too long — this is a critical failure,
					// and this swamp cannot be summoned again.
					if swampCloseError != nil {
						return nil, swampCloseError
					}

					// No error occurred, the swamp has been successfully closed,
					// so we can move on and begin summoning it again.
					continue

				}

				return swampObject, nil

			}

			// The swamp does not exist in memory, so we need to create it.
			// During creation, other processes trying to access this swamp will still have to wait.
			swampObject = h.createNewSwamp(islandID, swampName)

			// Store the swamp in the hydra map, which is a sync.Map.
			h.swamps.Store(swampName.Get(), swampObject)

			// start sending events to the subscribers if there are any clients subscribed to the events
			if h.hasEventSubscriber(swampName) {
				swampObject.StartSendingEvents()
			}

			// start sending information to the subscribers if there are any clients subscribed to the information
			if h.hasInfoSubscriber(swampName) {
				swampObject.StartSendingInformation()
			}

			return swampObject, nil

		}

	}

}

// IsExistSwamp checks if the swamp is existing in the hydras map or in the filesystem
// mutexes: clean
func (h *hydra) IsExistSwamp(islandID uint64, swampName name.Name) (bool, error) {

	if atomic.LoadInt32(&h.shuttingDown) == 1 {
		return false, errors.New(ErrorHydraIsShuttingDown)
	}

	if h.getSwamp(swampName) != nil {
		return true, nil
	}

	// Construct the full path to the swamp's directory.
	swampDataFolderPath := swampName.GetFullHashPath(h.settingsInterface.GetHydraAbsDataFolderPath(), islandID, h.settingsInterface.GetHashFolderDepth(), h.settingsInterface.GetMaxFoldersPerLevel())

	return h.filesystemInterface.IsFolderExists(swampDataFolderPath), nil

}

// ListActiveSwamps returns the list of opened and active swamps
// mutexes: clean
func (h *hydra) ListActiveSwamps() []string {
	var swampNames []string
	h.swamps.Range(func(key, value interface{}) bool {
		swampNames = append(swampNames, key.(string))
		return true
	})
	return swampNames
}

// CountActiveSwamps count the number of opened and active swamps
// mutexes: clean
func (h *hydra) CountActiveSwamps() int {
	elements := 0
	h.swamps.Range(func(key, value interface{}) bool {
		elements++
		return true
	})
	return elements
}

// SubscribeToSwampInfo subscribes to the information channel of the swamp
// mutexes: clean
func (h *hydra) SubscribeToSwampInfo(clientID uuid.UUID, swampName name.Name, subscriberInfoCallbackFunction func(info *swamp.Info)) error {

	if atomic.LoadInt32(&h.shuttingDown) == 1 {
		return errors.New(ErrorHydraIsShuttingDown)
	}

	canonicalForm := swampName.Get()

	defer func() {
		// starts sending information if the swamp exists in the hydra
		if swampObject, ok := h.swamps.Load(canonicalForm); ok {
			swampObject.(swamp.Swamp).StartSendingInformation()
		}
	}()

	if subscribers, ok := h.infoSubscribers.Load(canonicalForm); ok {
		// Always overwrite the subscriber, since the channel may have changed as well.
		subscribers.(*sync.Map).Store(clientID.String(), subscriberInfoCallbackFunction)
		return nil
	}

	// there is no subscribers to this swamp yet
	subscribers := &sync.Map{}
	subscribers.Store(clientID.String(), subscriberInfoCallbackFunction)
	h.infoSubscribers.Store(canonicalForm, subscribers)

	return nil

}

// UnsubscribeFromSwampInfo unsubscribes the user from the information channel of the swamp
// mutexes: clean
func (h *hydra) UnsubscribeFromSwampInfo(clientID uuid.UUID, swampName name.Name) error {

	if atomic.LoadInt32(&h.shuttingDown) == 1 {
		return errors.New(ErrorHydraIsShuttingDown)
	}

	canonicalForm := swampName.Get()

	if subscribers, ok := h.infoSubscribers.Load(canonicalForm); ok {
		subscribers.(*sync.Map).Delete(clientID.String())
	}

	allSubscribers := 0
	h.infoSubscribers.Range(func(key, value interface{}) bool {
		allSubscribers++
		return true
	})

	// stops sending information if the swamp exists and there are no subscribers to information
	if allSubscribers == 0 {
		if swampObject, ok := h.swamps.Load(canonicalForm); ok {
			swampObject.(swamp.Swamp).StopSendingInformation()
		}
	}

	return nil

}

// SubscribeToSwampEvents subscribes to the events channel of the swamp
// mutexes: clean
func (h *hydra) SubscribeToSwampEvents(clientID uuid.UUID, swampName name.Name, subscriberEventCallbackFunction func(event *swamp.Event)) error {

	if atomic.LoadInt32(&h.shuttingDown) == 1 {
		return errors.New(ErrorHydraIsShuttingDown)
	}

	canonicalForm := swampName.Get()

	defer func() {
		// starts the sending events if the swamp exists
		if swampObject, ok := h.swamps.Load(canonicalForm); ok {
			swampObject.(swamp.Swamp).StartSendingEvents()
		}
	}()

	if subscribers, ok := h.eventSubscribers.Load(canonicalForm); ok {
		// Always overwrite the subscriber, since the channel may have changed as well.
		subscribers.(*sync.Map).Store(clientID.String(), subscriberEventCallbackFunction)
		return nil
	}

	// there is no subscribers to this swamp yet
	subscribers := &sync.Map{}
	subscribers.Store(clientID.String(), subscriberEventCallbackFunction)
	h.eventSubscribers.Store(canonicalForm, subscribers)

	return nil

}

// UnsubscribeFromSwampEvents unsubscribes the user from the events channel of the swamp
// mutexes: clean
func (h *hydra) UnsubscribeFromSwampEvents(clientID uuid.UUID, swampName name.Name) error {

	if atomic.LoadInt32(&h.shuttingDown) == 1 {
		return errors.New(ErrorHydraIsShuttingDown)
	}

	canonicalForm := swampName.Get()

	if subscribers, ok := h.eventSubscribers.Load(canonicalForm); ok {
		// ha létezik a subscriber, akkor töröljük azt a mapből
		if _, ok := subscribers.(*sync.Map).Load(clientID.String()); ok {
			subscribers.(*sync.Map).Delete(clientID.String())
		}
	}

	allSubscribers := 0
	h.eventSubscribers.Range(func(key, value interface{}) bool {
		allSubscribers++
		return true
	})

	// stops sending events if the swamp exists and there are no subscribers to events
	if allSubscribers == 0 {
		if swampObject, ok := h.swamps.Load(canonicalForm); ok {
			swampObject.(swamp.Swamp).StopSendingEvents()
		}
	}

	return nil

}

// GracefulStop close function for the graceful stop package
// DO NOT CALL THIS FUNCTION DIRECTLY
// the graceful stop package calls this function when the server is shutting down
// mutexes: clean
func (h *hydra) GracefulStop() {

	slog.Info("Graceful stop of the hydra executed")

	// set the shutting down flag to true and prevent the creation of new swamps
	// and all public functions will return error, because the hydra is shutting down
	atomic.StoreInt32(&h.shuttingDown, 1)

	// Remove all event and info subscribers to prevent them from waiting for new events or information
	// from Hydra. We close all subscriber channels, which notifies the subscribers accordingly.
	h.eventSubscribers = sync.Map{}
	h.infoSubscribers = sync.Map{}

	// start a new routine and close all swamps
	go h.tryToCloseAllSwamps()

	slog.Info("waiting for graceful stop")

	// wait until all swamps are closed then return
	iterationCounter := 0
	for {

		// check the opened swamps
		openedSwamps := h.CountActiveSwamps()

		slog.Info("opened swamps", "count", openedSwamps)

		// if there is no opened swamps and the there is no process that destroying swamps - kill the server
		if openedSwamps == 0 {
			slog.Info("all swamps are gracefully closed, hydra is shutting down")
			return
		}

		// if we can't close the swamp within 10 seconds....
		if iterationCounter >= 10 {

			slog.Error("can not close all swamps within 10 seconds, Force close all swamps", "activeSwamps", strings.Join(h.ListActiveSwamps(), ", "))

			go func() {
				// iterating over the swamps and close them
				h.swamps.Range(func(key, value interface{}) bool {

					s := value.(swamp.Swamp)

					s.StopSendingInformation()
					s.StopSendingEvents()

					// log the error, because we can't close the swamp
					slog.Error("the swamp still opened and try to write all treasures to the filesystem again",
						"swampName", s.GetName(),
						"swampIsClosing", s.IsClosing(),
						"allTreasures", s.CountTreasures(),
						"treasuresWaitingForWriter", s.CountTreasuresWaitingForWriter(),
						"isFileSystemInitiated", s.GetChronicler().IsFilesystemInitiated())

					// Write treasures to the filesystem
					s.WriteTreasuresToFilesystem()

					slog.Info("the swamp still opened and all treasures are written to the filesystem again, and try to close it again",
						"swampName", s.GetName(),
						"swampIsClosing", s.IsClosing(),
						"allTreasures", s.CountTreasures(),
						"treasuresWaitingForWriter", s.CountTreasuresWaitingForWriter(),
						"isFileSystemInitiated", s.GetChronicler().IsFilesystemInitiated())

					// try to close it again
					s.Close()

					slog.Info("the swamp is closed successfully",
						"swampName", s.GetName(),
						"swampIsClosing", s.IsClosing(),
						"allTreasures", s.CountTreasures(),
						"treasuresWaitingForWriter", s.CountTreasuresWaitingForWriter(),
						"isFileSystemInitiated", s.GetChronicler().IsFilesystemInitiated())

					return true

				})

			}()

			// waiting for 30 seconds then force close the server
			time.Sleep(30 * time.Second)

			return

		}

		iterationCounter++

		time.Sleep(1000 * time.Millisecond)

	}

}

func (h *hydra) tryToCloseAllSwamps() {

	slog.Info("try to closing all open swamps.....")

	// iterating over the swamps and close them
	h.swamps.Range(func(key, value interface{}) bool {

		// Stop it from sending any more information to the channel.
		// This is important in case there are still active subscribers.
		value.(swamp.Swamp).StopSendingInformation()
		value.(swamp.Swamp).StopSendingEvents()
		// close the swamp
		value.(swamp.Swamp).Close()

		return true

	})

}

// createNewSwamp creates a new swamp and adds it to the map
func (h *hydra) createNewSwamp(islandID uint64, swampName name.Name) swamp.Swamp {

	// get the setting of the swamp
	swampSettings := h.settingsInterface.GetBySwampName(swampName)

	swampDataFolderPath := swampName.GetFullHashPath(h.settingsInterface.GetHydraAbsDataFolderPath(), islandID, h.settingsInterface.GetHashFolderDepth(), h.settingsInterface.GetMaxFoldersPerLevel())

	// Instantiate the metadata based on the folder.
	metadataInterface := metadata.New(swampDataFolderPath)
	// Load the metadata from the file.
	metadataInterface.LoadFromFile()
	// Pass the swamp name to it.
	metadataInterface.SetSwampName(swampName)

	// create the new filesystem
	var fss *swamp.FilesystemSettings
	// init chronicler if the swamp is permanent-type
	if swampSettings.GetSwampType() == setting.PermanentSwamp {
		fss = &swamp.FilesystemSettings{}
		fss.ChroniclerInterface = h.loadChronicler(swampSettings, swampDataFolderPath, metadataInterface)
		fss.WriteInterval = swampSettings.GetWriteInterval()
	}

	// create the swamp with the filesystem
	return swamp.New(swampName, swampSettings.GetCloseAfterIdle(), fss, h.eventCallbackFunction, h.infoCallbackFunction, h.closeEventCallbackFunction, metadataInterface)

}

// loadChronicler loads the filesystem of the swamp or create a new one if it is not existing
func (h *hydra) loadChronicler(swampSettings setting.Setting, swampDataFolderPath string, metadataInterface metadata.Metadata) chronicler.Chronicler {

	// Construct the full path to the swamp's directory.
	maxFileSizeBytes := swampSettings.GetMaxFileSizeByte()

	// Create the file handler along with the metadata for the swamp.
	fs := chronicler.New(swampDataFolderPath, maxFileSizeBytes, h.settingsInterface.GetHashFolderDepth(), h.filesystemInterface, metadataInterface)
	fs.CreateDirectoryIfNotExists()

	return fs

}

// getSwamp returns the swamp itself from the map
func (h *hydra) getSwamp(swampName name.Name) (swampObject swamp.Swamp) {
	if swampObj, ok := h.swamps.Load(swampName.Get()); ok {
		s := swampObj.(swamp.Swamp)
		return s
	}
	return nil
}

// hasEventSubscriber checks if the swamp has any event subscribers
func (h *hydra) hasEventSubscriber(swampName name.Name) bool {

	canonicalForm := swampName.Get()

	if subscribers, ok := h.eventSubscribers.Load(canonicalForm); ok {
		eventSubscribers := 0
		subscribers.(*sync.Map).Range(func(key, value interface{}) bool {
			eventSubscribers++
			return true
		})
		if eventSubscribers > 0 {
			return true
		}
	}

	return false

}

// hasEventSubscriber checks if the swamp has any event subscribers
func (h *hydra) hasInfoSubscriber(swampName name.Name) bool {

	canonicalForm := swampName.Get()
	if subscribers, ok := h.infoSubscribers.Load(canonicalForm); ok {
		infoSubscribers := 0
		subscribers.(*sync.Map).Range(func(key, value interface{}) bool {
			infoSubscribers++
			return true
		})
		if infoSubscribers > 0 {
			return true
		}
	}

	return false
}

// eventCallbackFunction send the eventChannelHandler to all subscribed clients
func (h *hydra) eventCallbackFunction(event *swamp.Event) {

	swampName := event.SwampName

	if subscribers, ok := h.eventSubscribers.Load(swampName.Get()); ok {

		treasureID := ""
		if event.Treasure != nil {
			treasureID = event.Treasure.GetKey()
		}
		oldTreasureID := ""
		if event.OldTreasure != nil {
			oldTreasureID = event.OldTreasure.GetKey()
		}

		subscribers.(*sync.Map).Range(func(key, value interface{}) bool {

			if value == nil {

				slog.Error("the callback function is nil, this is a bug, the callback function should not be nil",
					"swampName", swampName,
					"subscriberID", key,
					"eventTreasureKey", treasureID,
					"eventOldTreasureKey", oldTreasureID)

				return true
			}

			// let's try to send the event to the client
			// select for the case when the channel is full or closed
			if function, ok := value.(func(event *swamp.Event)); ok {

				function(event) // Csak akkor hívjuk, ha a type assertion sikeres.

			} else {

				clientUUID, err := uuid.Parse(key.(string))
				if err != nil {

					slog.Error("can not parse the subscriberID to UUID",
						"swampName", swampName,
						"subscriberID", key)

					return true
				}

				// unsubscribe the client from the event channel, because the channel is full or closed
				if err := h.UnsubscribeFromSwampEvents(clientUUID, event.SwampName); err != nil {

					slog.Error("can not unsubscribe the client from the event channel",
						"swampName", swampName,
						"subscriberID", key)

					return true
				}

				slog.Warn("can not send event to the subscribed user because the callback function is not a func(event *swamp.Event) type. The client forced to unsubscribe from the event channel.",
					"swampName", swampName,
					"subscriberID", key)

				return true

			}

			return true

		})
	}

}

func (h *hydra) infoCallbackFunction(si *swamp.Info) {

	if subscribers, ok := h.infoSubscribers.Load(si.SwampName.Get()); ok {

		subscribers.(*sync.Map).Range(func(key, value interface{}) bool {

			if value == nil {

				slog.Error("the callback function is nil, this is a bug, the callback function should not be nil",
					"swampName", si.SwampName,
					"subscriberID", key,
					"info", si.AllElements)

				return true
			}

			if function, ok := value.(func(info *swamp.Info)); ok {
				function(si)
			} else {

				// unsubscribe the client from the event channel if the swamp is closing
				clientUUID, err := uuid.Parse(key.(string))
				if err != nil {

					slog.Error("can not parse the subscriberID to UUID",
						"swampName", si.SwampName,
						"subscriberID", key)

					return true
				}
				// unsubscribe the client from the event channel, because the channel is full or closed
				if err := h.UnsubscribeFromSwampInfo(clientUUID, si.SwampName); err != nil {

					slog.Error("can not unsubscribe the client from the swamp information",
						"swampName", si.SwampName,
						"subscriberID", key)

					return true
				}

				slog.Warn("can not send info to the subscribed user because the callback function is not a func(info *swamp.Info) type. The client forced to unsubscribe from the swamp informations.",
					"swampName", si.SwampName)

				return true

			}

			return true

		})
	}

}

// closeEventCallbackFunction removes the swamp from the opened swamps map
func (h *hydra) closeEventCallbackFunction(swampName name.Name) {
	h.swamps.Delete(swampName.Get())
}
