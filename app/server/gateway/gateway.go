package gateway

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/hydraide/hydraide/app/core/hydra/swamp"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/guard"
	"github.com/hydraide/hydraide/app/core/settings"
	"github.com/hydraide/hydraide/app/core/zeus"
	"github.com/hydraide/hydraide/app/name"
	"github.com/hydraide/hydraide/app/server/observer"
	hydrapb "github.com/hydraide/hydraide/generated/hydraidepbgo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log/slog"
	"runtime/debug"
	"strings"
	"time"
)

type Gateway struct {
	hydrapb.UnimplementedHydraideServiceServer
	ObserverInterface     observer.Observer
	SettingsInterface     settings.Settings
	ZeusInterface         zeus.Zeus
	DefaultCloseAfterIdle int64
	DefaultWriteInterval  int64
	DefaultFileSize       int64
}

func (g Gateway) Heartbeat(_ context.Context, in *hydrapb.HeartbeatRequest) (*hydrapb.HeartbeatResponse, error) {
	return &hydrapb.HeartbeatResponse{
		Pong: in.Ping,
	}, nil
}

func (g Gateway) Lock(ctx context.Context, in *hydrapb.LockRequest) (*hydrapb.LockResponse, error) {

	defer handlePanic()

	// try to summon the swamp
	lockerInterface := g.ZeusInterface.GetHydra().GetLocker()

	// create a new context
	ctxForLocker := context.WithoutCancel(ctx)

	// Set the TTL to the required minimum value if it is less than or equal to 1000 milliseconds
	if in.GetTTL() <= 1000 {
		// set the TTL to 1000 milliseconds to prevent too short TTLs
		in.TTL = 1000
	}

	// what is the lock key is an empty string
	if in.GetKey() == "" {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "Lock key cannot be empty")
	}

	// lock the system
	lockID, err := lockerInterface.Lock(ctxForLocker, in.GetKey(), time.Duration(in.GetTTL())*time.Millisecond)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.DeadlineExceeded, fmt.Sprintf("lock deadline exceeded: %s", err.Error()))
	}

	return &hydrapb.LockResponse{
		LockID: lockID,
	}, nil

}

func (g Gateway) Unlock(_ context.Context, in *hydrapb.UnlockRequest) (*hydrapb.UnlockResponse, error) {

	defer handlePanic()

	// wha it the lock key is empty
	if in.GetKey() == "" {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "Lock key cannot be empty")
	}

	// what if the lock ID is empty
	if in.GetLockID() == "" {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "Lock ID cannot be empty")
	}

	// try to summon the swamp
	lockerInterface := g.ZeusInterface.GetHydra().GetLocker()
	// unlock the system
	if err := lockerInterface.Unlock(in.GetKey(), in.GetLockID()); err != nil {
		// return with grpc error message
		return nil, status.Error(codes.NotFound, fmt.Sprintf("lock not found: %s", err.Error()))
	}
	return &hydrapb.UnlockResponse{}, nil

}

func (g Gateway) RegisterSwamp(_ context.Context, in *hydrapb.RegisterSwampRequest) (*hydrapb.RegisterSwampResponse, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	if in.SwampPattern == "" {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "SwampPattern cannot be empty")
	}

	// try to create the pattern from the input string
	swampPattern := name.Load(in.SwampPattern)

	closeAfterIdle := g.DefaultCloseAfterIdle

	if in.CloseAfterIdle > 0 {
		closeAfterIdle = in.CloseAfterIdle
	}

	var fss *settings.FileSystemSettings
	if !in.IsInMemorySwamp {

		fss = &settings.FileSystemSettings{}
		if in.WriteInterval != nil && *in.WriteInterval > 0 {
			fss.WriteIntervalSec = *in.WriteInterval
		} else {
			fss.WriteIntervalSec = g.DefaultWriteInterval
		}

		if in.MaxFileSize != nil && *in.MaxFileSize > 0 {
			fss.MaxFileSizeByte = *in.MaxFileSize
		} else {
			fss.MaxFileSizeByte = g.DefaultFileSize
		}

	}

	g.SettingsInterface.RegisterPattern(swampPattern, in.IsInMemorySwamp, closeAfterIdle, fss)

	return &hydrapb.RegisterSwampResponse{}, nil

}

func (g Gateway) DeRegisterSwamp(_ context.Context, in *hydrapb.DeRegisterSwampRequest) (*hydrapb.DeRegisterSwampResponse, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	if in.SwampPattern == "" {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "SwampPattern cannot be empty")
	}

	// try to create the pattern from the input string
	swampPattern := name.Load(in.SwampPattern)

	g.SettingsInterface.DeregisterPattern(swampPattern)

	return &hydrapb.DeRegisterSwampResponse{}, nil

}

func (g Gateway) Set(ctx context.Context, in *hydrapb.SetRequest) (*hydrapb.SetResponse, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	// validate all the requests
	for _, swampRequest := range in.GetSwamps() {
		// check if the swamp name is valid and exist or not
		// we don't need to check the existence of the swamp because we will create it if it does not exist
		if _, err := checkSwampName(g.ZeusInterface, swampRequest.GetIslandID(), swampRequest.SwampName, false); err != nil {
			return nil, err
		}
		if swampRequest.GetKeyValues() == nil {
			// return with grpc error message
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("KeyValues cannot be empty for the swamp: %s", swampRequest.GetSwampName()))
		}
	}

	// try to summon the swamp
	hydraInterface := g.ZeusInterface.GetHydra()

	var swampResponses []*hydrapb.SwampResponse

	for _, swampRequest := range in.GetSwamps() {

		swampResponse := &hydrapb.SwampResponse{
			SwampName: swampRequest.SwampName,
		}

		// don't need to check the error, because we already checked it in the previous loop
		swampName := name.Load(swampRequest.SwampName)

		var internalError error

		func() {

			// this is a meaningless setting
			if !swampRequest.GetCreateIfNotExist() && !swampRequest.GetOverwrite() {
				swampResponses = append(swampResponses, &hydrapb.SwampResponse{
					SwampName:       swampRequest.SwampName,
					KeysAndStatuses: []*hydrapb.KeyStatusPair{},
					ErrorCode:       hydrapb.SwampResponse_CanNotBeExecuted.Enum(),
				})
				return
			}

			// check if the swamp already exists if the
			if !swampRequest.GetCreateIfNotExist() {
				isExist, err := hydraInterface.IsExistSwamp(swampRequest.GetIslandID(), swampName)
				if err != nil || !isExist {
					swampResponses = append(swampResponses, &hydrapb.SwampResponse{
						SwampName:       swampRequest.SwampName,
						KeysAndStatuses: []*hydrapb.KeyStatusPair{},
						ErrorCode:       hydrapb.SwampResponse_SwampDoesNotExist.Enum(),
					})
					return
				}
			}

			swampInterface, err := hydraInterface.SummonSwamp(ctx, swampRequest.GetIslandID(), swampName)
			if err != nil {
				// return with grpc error message
				internalError = err
				return
			}

			// begin the vigil, to prevent the close of the swamp
			swampInterface.BeginVigil()
			defer swampInterface.CeaseVigil()

			response := make([]*hydrapb.KeyStatusPair, 0)

			for _, item := range swampRequest.GetKeyValues() {

				// if "create if not" exist is false and the treasure does not exist
				if !swampRequest.GetCreateIfNotExist() && !swampInterface.TreasureExists(item.Key) {
					response = append(response, &hydrapb.KeyStatusPair{
						Key:    item.Key,
						Status: hydrapb.Status_NOT_FOUND,
					})
					continue
				}

				if !swampRequest.Overwrite && swampInterface.TreasureExists(item.Key) {
					response = append(response, &hydrapb.KeyStatusPair{
						Key:    item.Key,
						Status: hydrapb.Status_NOTHING_CHANGED,
					})
					// check the treasure and skip if it exists
					continue
				}

				// anonymous function to handle the treasure
				func() {

					// create the treasure and start the guard
					treasureInterface := swampInterface.CreateTreasure(item.Key)
					guardID := treasureInterface.StartTreasureGuard(true)
					defer treasureInterface.ReleaseTreasureGuard(guardID)

					// set the content type and content
					keyValuesToTreasure(item, treasureInterface, guardID)

					treasureStatus := treasureInterface.Save(guardID)

					// save the treasure to the Hydra
					responseStatus := convertTreasureStatusToPbStatus(treasureStatus)

					// add the key and status to the response
					response = append(response, &hydrapb.KeyStatusPair{
						Key:    item.Key,
						Status: responseStatus,
					})

				}()

			}

			swampResponse.KeysAndStatuses = response

		}()

		if internalError != nil {
			// return with grpc error message
			return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", internalError.Error()))
		}

		swampResponses = append(swampResponses, swampResponse)

	}

	// return with all the keys and statuses that we saved
	return &hydrapb.SetResponse{
		Swamps: swampResponses,
	}, nil

}

func (g Gateway) Get(ctx context.Context, in *hydrapb.GetRequest) (*hydrapb.GetResponse, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	// validate all the requests
	for _, swampRequest := range in.GetSwamps() {
		// check if the swamp name is valid and exist or not
		if _, err := checkSwampName(g.ZeusInterface, swampRequest.GetIslandID(), swampRequest.SwampName, true); err != nil {
			return nil, err
		}
		if swampRequest.GetKeys() == nil || swampRequest.GetKeys()[0] == "" {
			// return with grpc error message
			return nil, status.Error(codes.InvalidArgument, "Keys cannot be empty")
		}
	}

	// try to summon the swamp
	hydraInterface := g.ZeusInterface.GetHydra()

	var swamps []*hydrapb.GetSwampResponse

	// iterating over the requests
	for _, swampRequest := range in.GetSwamps() {

		// don't need to check the error, because we already checked it in the previous loop
		swampName := name.Load(swampRequest.SwampName)

		swampResponse := &hydrapb.GetSwampResponse{
			SwampName: swampRequest.SwampName,
		}

		var internalError error

		func() {

			// check if the swamp already exists
			// because we need to prevent the creation of a new swamp
			isExist, err := hydraInterface.IsExistSwamp(swampRequest.GetIslandID(), swampName)
			if err != nil || !isExist {
				// return with grpc error message
				swampResponse.IsExist = false // override the default value
				return
			}

			swampInterface, err := hydraInterface.SummonSwamp(ctx, swampRequest.GetIslandID(), swampName)
			if err != nil {
				// internal error
				internalError = err
				return
			}

			// begin the vigil, to prevent closing of the swamp
			swampInterface.BeginVigil()
			defer swampInterface.CeaseVigil()

			var response []*hydrapb.Treasure

			for _, key := range swampRequest.GetKeys() {

				t := &hydrapb.Treasure{
					Key:     key,
					IsExist: true, // default value
				}

				treasureInterface, err := swampInterface.GetTreasure(key)
				if err != nil {
					// the treasure does not exist
					t.IsExist = false // override the default value
				} else {
					// convert the treasure from the hydra to the protobuf format
					treasureToKeyValuePair(treasureInterface, t)
				}

				// add the treasure to the response
				response = append(response, t)

			}

			swampResponse.Treasures = response

		}()

		if internalError != nil {
			// return with grpc error message
			return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", internalError.Error()))
		}

		swamps = append(swamps, swampResponse)

	}

	// return with all the treasures that we found
	return &hydrapb.GetResponse{
		Swamps: swamps,
	}, nil

}

func (g Gateway) GetAll(ctx context.Context, in *hydrapb.GetAllRequest) (*hydrapb.GetAllResponse, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	// check if the swamp name is valid and exist or not
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, true)
	if err != nil {
		return nil, err
	}

	hydraInterface := g.ZeusInterface.GetHydra()

	// summon the swamp
	swampInterface, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}

	// begin the vigil, to prevent closing of the swamp
	swampInterface.BeginVigil()
	defer swampInterface.CeaseVigil()

	treasures := swampInterface.GetAll()

	var response []*hydrapb.Treasure
	for _, treasureInterface := range treasures {
		t := &hydrapb.Treasure{}
		treasureToKeyValuePair(treasureInterface, t)
		response = append(response, t)
	}

	return &hydrapb.GetAllResponse{
		Treasures: response,
	}, nil

}

func (g Gateway) GetByIndex(ctx context.Context, in *hydrapb.GetByIndexRequest) (*hydrapb.GetByIndexResponse, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, true)
	if err != nil {
		return nil, err
	}

	hydraInterface := g.ZeusInterface.GetHydra()

	// summon the swamp
	swampInterface, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}

	// begin the vigil, to prevent closing of the swamp
	swampInterface.BeginVigil()
	defer swampInterface.CeaseVigil()

	treasures, err := swampInterface.GetTreasuresByBeacon(inputIndexTypeToBeaconType(in.GetIndexType()),
		inputOrderTypeToBeaconOrderType(in.GetOrderType()), in.GetFrom(), in.GetLimit())

	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.Internal, fmt.Sprintf("hydra error: %s", err.Error()))
	}

	// convert all treasures to the protobuf format
	var response []*hydrapb.Treasure
	for _, treasureInterface := range treasures {
		// convert the treasure to the protobuf format
		t := &hydrapb.Treasure{}
		treasureToKeyValuePair(treasureInterface, t)
		response = append(response, t)
	}

	// get the treasures by the index
	return &hydrapb.GetByIndexResponse{
		Treasures: response,
	}, nil

}

func (g Gateway) ShiftExpiredTreasures(ctx context.Context, in *hydrapb.ShiftExpiredTreasuresRequest) (*hydrapb.ShiftExpiredTreasuresResponse, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, true)
	if err != nil {
		return nil, err
	}

	hydraInterface := g.ZeusInterface.GetHydra()

	// summon the swamp
	swampInterface, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}

	// begin the vigil, to prevent closing of the swamp
	swampInterface.BeginVigil()
	defer swampInterface.CeaseVigil()

	howMany := in.GetHowMany()
	if howMany == 0 {
		// set the howMany to the default value because it is zero and if the value is zero, the user wants ALL the
		// expired treasures
		howMany = 1000000000 // 1 billion, this is a very large number, so it will return all the expired treasures
	}

	// clone and delete the expired treasures from the swamp
	treasures, err := swampInterface.CloneAndDeleteExpiredTreasures(howMany)
	// there was an error
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.Internal, fmt.Sprintf("hydra error: %s", err.Error()))
	}

	// convert all treasures to the protobuf format
	var response []*hydrapb.Treasure
	for _, treasureInterface := range treasures {
		// convert the treasure to the protobuf format
		t := &hydrapb.Treasure{}
		treasureToKeyValuePair(treasureInterface, t)
		response = append(response, t)
	}

	// get the treasures by the index
	return &hydrapb.ShiftExpiredTreasuresResponse{
		Treasures: response,
	}, nil

}

func (g Gateway) Destroy(ctx context.Context, in *hydrapb.DestroyRequest) (*hydrapb.DestroyResponse, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	// check and validate the name of the swamp
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, false)
	if err != nil {
		return nil, err
	}

	// get the hydra interface
	hydraInterface := g.ZeusInterface.GetHydra()

	// summon the swamp
	swampInterface, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}

	// destroy the swamp
	swampInterface.Destroy()

	return &hydrapb.DestroyResponse{}, nil

}

func (g Gateway) Delete(ctx context.Context, in *hydrapb.DeleteRequest) (*hydrapb.DeleteResponse, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	responses := make([]*hydrapb.DeleteResponse_SwampDeleteResponse, 0, len(in.GetSwamps()))

	// validate all the requests
	for _, swampRequest := range in.GetSwamps() {

		// check if the swamp name is valid and exist or not
		// we don't need to check the existence of the swamp because we will create it if it does not exist
		swampNameObj, err := checkSwampName(g.ZeusInterface, swampRequest.GetIslandID(), swampRequest.SwampName, true)
		if err != nil {
			responses = append(responses, &hydrapb.DeleteResponse_SwampDeleteResponse{
				SwampName: swampRequest.SwampName,
				ErrorCode: hydrapb.DeleteResponse_SwampDeleteResponse_SwampDoesNotExist.Enum(),
			})
			continue
		}

		hydraInterface := g.ZeusInterface.GetHydra()

		// summon the swamp
		swampInterface, err := hydraInterface.SummonSwamp(ctx, swampRequest.GetIslandID(), swampNameObj)
		if err != nil {
			// return with grpc error message
			return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
		}

		func() {

			// begin the vigil, to prevent closing of the swamp
			swampInterface.BeginVigil()
			defer swampInterface.CeaseVigil()

			sr := &hydrapb.DeleteResponse_SwampDeleteResponse{
				SwampName: swampRequest.SwampName,
				ErrorCode: nil,
			}

			keyStatuses := make([]*hydrapb.KeyStatusPair, 0)

			for _, key := range swampRequest.GetKeys() {
				statusPair := &hydrapb.KeyStatusPair{
					Key: key,
				}
				if err := swampInterface.DeleteTreasure(key, false); err != nil {
					statusPair.Status = hydrapb.Status_NOT_FOUND

				} else {
					statusPair.Status = hydrapb.Status_DELETED
				}
				keyStatuses = append(keyStatuses, statusPair)
			}

			sr.KeyStatuses = keyStatuses
			responses = append(responses, sr)

		}()

	}

	return &hydrapb.DeleteResponse{
		Responses: responses,
	}, nil

}

func (g Gateway) Count(ctx context.Context, in *hydrapb.CountRequest) (*hydrapb.CountResponse, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	// try to count the treasures in all the swamps
	var response []*hydrapb.CountSwamp

	type SwampIdentifier struct {
		IslandID  uint64
		SwampName name.Name
	}

	var swamps []*SwampIdentifier

	for _, swampIdentifier := range in.GetSwamps() {

		swampNameObject, err := checkSwampName(g.ZeusInterface, swampIdentifier.GetIslandID(), swampIdentifier.GetSwampName(), true)
		if err != nil {
			// Ellenőrizzük, hogy a hiba állapota 'NotFound' kódú-e
			if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
				// this is not an error, just a swamp that does not exist
				response = append(response, &hydrapb.CountSwamp{
					SwampName: swampIdentifier.GetSwampName(),
					Count:     0,
					IsExist:   false,
				})
			} else {
				// return with grpc error message
				return nil, err
			}
		}

		swamps = append(swamps, &SwampIdentifier{
			IslandID:  swampIdentifier.GetIslandID(),
			SwampName: swampNameObject,
		})
	}

	hydraInterface := g.ZeusInterface.GetHydra()

	// iterating over only the existing swamps
	for _, swampIdentifier := range swamps {

		// summon the swamp
		swampInterface, err := hydraInterface.SummonSwamp(ctx, swampIdentifier.IslandID, swampIdentifier.SwampName)
		if err != nil {
			// return with grpc error message
			return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
		}
		if swampInterface == nil {
			// return with grpc error message
			return nil, status.Error(codes.Internal, "internal server error in hydra: swamp interface is nil")
		}

		func() {

			// begin the vigil, to prevent closing of the swamp
			swampInterface.BeginVigil()
			defer swampInterface.CeaseVigil()

			count := swampInterface.CountTreasures()
			response = append(response, &hydrapb.CountSwamp{
				SwampName: swampIdentifier.SwampName.Get(),
				Count:     int32(count),
				IsExist:   true,
			})

		}()

	}

	// return with the count of the treasures
	return &hydrapb.CountResponse{
		Swamps: response,
	}, nil

}

func (g Gateway) IsSwampExist(_ context.Context, in *hydrapb.IsSwampExistRequest) (*hydrapb.IsSwampExistResponse, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	_, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, true)
	if err != nil {
		return &hydrapb.IsSwampExistResponse{
			IsExist: false,
		}, err
	}

	return &hydrapb.IsSwampExistResponse{
		IsExist: true,
	}, nil

}

func (g Gateway) IsKeyExist(_ context.Context, in *hydrapb.IsKeyExistRequest) (*hydrapb.IsKeyExistResponse, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	// check if the swamp name is correct and exist
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, true)
	if err != nil {
		return nil, err
	}

	// summon the swamp
	hydraInterface := g.ZeusInterface.GetHydra()
	swampInterface, err := hydraInterface.SummonSwamp(context.Background(), in.GetIslandID(), swampName)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}

	// begin the vigil, to prevent closing of the swamp
	swampInterface.BeginVigil()
	defer swampInterface.CeaseVigil()

	isExist := swampInterface.TreasureExists(in.Key)

	return &hydrapb.IsKeyExistResponse{
		IsExist: isExist,
	}, nil

}

func (g Gateway) SubscribeToEvents(in *hydrapb.SubscribeToEventsRequest, eventServer hydrapb.HydraideService_SubscribeToEventsServer) error {

	// do not use the g.ZeusInterface.GetSafeops().LockSystem() because if we use it, we can never stop the server because of the active subscribers
	// until the client closes the connection

	defer handlePanic()

	// check if the swamp name is correct
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, false)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	subscriberUUID := uuid.New()

	// the subscription is successful, now we can start to send the events to the client
	// Get the server context
	hydraInterface := g.ZeusInterface.GetHydra()

	eventCallbackFunction := func(event *swamp.Event) {

		if event == nil {
			return
		}

		// send the event to the client
		defer handlePanic()

		// get the canonical form of the swamp name
		eventSwampName := event.SwampName.Get()
		// convert the hydra treasure to the protobuf format
		convertedTreasure := &hydrapb.Treasure{}
		// convert the status type to the protobuf format
		convertedStatusType := convertTreasureStatusToPbStatus(event.StatusType)

		// convert the event time to the protobuf format
		convertedEventTime := timestamppb.New(time.Unix(event.EventTime, 0))
		convertedOldTreasure := &hydrapb.Treasure{}
		convertedDeletedTreasure := &hydrapb.Treasure{}

		switch event.StatusType {
		case treasure.StatusNew:

			if event.Treasure != nil {
				treasureToKeyValuePair(event.Treasure, convertedTreasure)
			}

		case treasure.StatusModified:

			if event.Treasure != nil {
				treasureToKeyValuePair(event.Treasure, convertedTreasure)
			}
			if event.OldTreasure != nil {
				treasureToKeyValuePair(event.OldTreasure, convertedOldTreasure)
			}

		case treasure.StatusDeleted:

			if event.DeletedTreasure != nil {
				treasureToKeyValuePair(event.DeletedTreasure, convertedDeletedTreasure)
			}

		default:

			return

		}

		// send the message to the client
		if sendErr := eventServer.SendMsg(&hydrapb.SubscribeToEventsResponse{
			SwampName:       eventSwampName,
			Treasure:        convertedTreasure,
			Status:          convertedStatusType,
			OldTreasure:     convertedOldTreasure,
			DeletedTreasure: convertedDeletedTreasure,
			EventTime:       convertedEventTime,
		}); sendErr != nil {
			slog.Error("failed to send the event to the client",
				"error", sendErr.Error(),
				"swamp_name", eventSwampName)
		}

	}

	if err := hydraInterface.SubscribeToSwampEvents(subscriberUUID, swampName, eventCallbackFunction); err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}

	for {
		select {
		// we are waiting for the client to close the connection
		case <-eventServer.Context().Done():

			err := eventServer.Context().Err()
			if err != nil && !errors.Is(err, context.Canceled) {
				slog.Warn("connection closed with an unexpected error",
					"uuid", subscriberUUID,
					"error", err.Error())
			} else {
				slog.Debug("the client closed the connection gracefully",
					"uuid", subscriberUUID)
			}

			// Unsubscribe logic
			if err := hydraInterface.UnsubscribeFromSwampEvents(subscriberUUID, swampName); err != nil {
				slog.Error("failed to unsubscribe the subscriber from the swamp",
					"uuid", subscriberUUID,
					"error", err.Error())
			}

			return nil

		}
	}

}

func (g Gateway) SubscribeToInfo(in *hydrapb.SubscribeToInfoRequest, infoServer hydrapb.HydraideService_SubscribeToInfoServer) error {

	// do not use the g.ZeusInterface.GetSafeops().LockSystem() because if we use it, we can never stop the server because of the active subscribers
	// until the client closes the connection
	defer handlePanic()

	// check if the swamp name is correct
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, false)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	// make the channel for the subscriber
	subscriberUUID := uuid.New()

	infoSubscriptionCallbackFunction := func(info *swamp.Info) {
		// send the event to the client
		func() {

			defer handlePanic()

			// get the canonical form of the swamp name
			infoSwampName := info.SwampName.Get()

			// send the info to the client
			if sendErr := infoServer.Send(&hydrapb.SubscribeToInfoResponse{
				SwampName:   infoSwampName,
				AllElements: info.AllElements,
			}); sendErr != nil {
				slog.Error("failed to send the info to the client",
					"error", sendErr.Error(),
					"swamp_name", infoSwampName)
			}

		}()
	}

	// subscribe to the swamp for information
	hydraInterface := g.ZeusInterface.GetHydra()
	if err := hydraInterface.SubscribeToSwampInfo(subscriberUUID, swampName, infoSubscriptionCallbackFunction); err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}

	defer func() {
		handlePanic()
		// remove the subscriber from the swamp when the client closes the connection
		if err := hydraInterface.UnsubscribeFromSwampInfo(subscriberUUID, swampName); err != nil {
			slog.Error("failed to unsubscribe the subscriber from the swamp",
				"uuid", subscriberUUID,
				"error", err.Error())
		}
		slog.Debug("the subscriber is removed from the swamp",
			"uuid", subscriberUUID,
		)
	}()

	for {
		select {
		// várunk arra, hogy a user bezárja a kapcsolatot
		case <-infoServer.Context().Done():
			// remove the subscriber from the swamp when the client closes the connection
			if err := hydraInterface.UnsubscribeFromSwampInfo(subscriberUUID, swampName); err != nil {
				slog.Error("failed to unsubscribe the subscriber from the swamp",
					"uuid", subscriberUUID,
					"error", err.Error(),
				)
			}
			return nil

		}
	}

}

func (g Gateway) Uint32SlicePush(ctx context.Context, in *hydrapb.AddToUint32SlicePushRequest) (*hydrapb.AddToUint32SlicePushResponse, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	if in.SwampName == "" {
		return nil, status.Error(codes.InvalidArgument, "SwampName cannot be empty")
	}

	// check the name of the swamp
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, false)
	if err != nil {
		return nil, err
	}

	// get the hydra interface
	hydraInterface := g.ZeusInterface.GetHydra()

	// summon the swamp
	swampObj, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}

	// begin the vigil, to prevent closing of the swamp
	swampObj.BeginVigil()
	defer swampObj.CeaseVigil()

	var errorsWhilePush []string

	for _, pair := range in.KeySlicePairs {

		func() {

			treasureObj := swampObj.CreateTreasure(pair.GetKey())

			guardID := treasureObj.StartTreasureGuard(true)
			defer treasureObj.ReleaseTreasureGuard(guardID)

			if err := treasureObj.Uint32SlicePush(pair.GetValues()); err != nil {
				errorsWhilePush = append(errorsWhilePush, err.Error())
			}

		}()

	}

	if len(errorsWhilePush) > 0 {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("the following errors occurred: %s", strings.Join(errorsWhilePush, ", ")))
	}

	return nil, nil

}

func (g Gateway) Uint32SliceDelete(ctx context.Context, in *hydrapb.Uint32SliceDeleteRequest) (*hydrapb.Uint32SliceDeleteResponse, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	if in.SwampName == "" {
		return nil, status.Error(codes.InvalidArgument, "SwampName cannot be empty")
	}

	// check the name of the swamp
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, false)
	if err != nil {
		return nil, err
	}

	// get the hydra interface
	hydraInterface := g.ZeusInterface.GetHydra()

	// summon the swamp
	swampObj, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}

	// begin the vigil, to prevent closing of the swamp
	swampObj.BeginVigil()
	defer swampObj.CeaseVigil()

	var errorsWhileDelete []string

	for _, pair := range in.KeySlicePairs {

		func() {

			// try to load the treasure
			treasureObj, err := swampObj.GetTreasure(pair.GetKey())
			// the treasure does not exist so we can't delete the slice from it
			if err != nil {
				return
			}

			guardID := treasureObj.StartTreasureGuard(true)
			defer treasureObj.ReleaseTreasureGuard(guardID)

			if err := treasureObj.Uint32SliceDelete(pair.GetValues()); err != nil {
				errorsWhileDelete = append(errorsWhileDelete, err.Error())
			}

			// check the length of the slice in the treasure
			// if the length is 0, we can delete the treasure
			size, err := treasureObj.Uint32SliceSize()
			if err != nil || size == 0 {
				// delete the treasure
				if err := swampObj.DeleteTreasure(pair.GetKey(), false); err != nil {
					errorsWhileDelete = append(errorsWhileDelete, err.Error())
				}
			}

		}()

	}

	if len(errorsWhileDelete) > 0 {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("the following errors occurred: %s", strings.Join(errorsWhileDelete, ", ")))
	}

	return nil, nil

}

func (g Gateway) Uint32SliceSize(ctx context.Context, in *hydrapb.Uint32SliceSizeRequest) (*hydrapb.Uint32SliceSizeResponse, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	if in.SwampName == "" {
		return nil, status.Error(codes.InvalidArgument, "SwampName cannot be empty")
	}

	// check the name of the swamp
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, false)
	if err != nil {
		return nil, err
	}

	// get the hydra interface
	hydraInterface := g.ZeusInterface.GetHydra()

	// summon the swamp
	swampObj, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}

	// begin the vigil, to prevent closing of the swamp
	swampObj.BeginVigil()
	defer swampObj.CeaseVigil()

	treasureObj, err := swampObj.GetTreasure(in.GetKey())
	if err != nil {
		return &hydrapb.Uint32SliceSizeResponse{Size: 0}, status.Error(codes.InvalidArgument, fmt.Sprintf("the key does not exist: %s", err.Error()))
	}

	size, err := treasureObj.Uint32SliceSize()
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, fmt.Sprintf("the treasure type is not slice. err: %s", err.Error()))
	}

	return &hydrapb.Uint32SliceSizeResponse{Size: int64(size)}, nil

}

func (g Gateway) Uint32SliceIsValueExist(ctx context.Context, in *hydrapb.Uint32SliceIsValueExistRequest) (*hydrapb.Uint32SliceIsValueExistResponse, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	if in.SwampName == "" {
		return nil, status.Error(codes.InvalidArgument, "SwampName cannot be empty")
	}

	// check the name of the swamp
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, false)
	if err != nil {
		return nil, err
	}

	// get the hydra interface
	hydraInterface := g.ZeusInterface.GetHydra()

	// summon the swamp
	swampObj, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}

	// begin the vigil, to prevent closing of the swamp
	swampObj.BeginVigil()
	defer swampObj.CeaseVigil()

	treasureObj, err := swampObj.GetTreasure(in.GetKey())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("the key does not exist: %s", err.Error()))
	}

	sl, err := treasureObj.Uint32SliceGetAll()
	for _, value := range sl {
		if value == in.GetValue() {
			return &hydrapb.Uint32SliceIsValueExistResponse{IsExist: true}, nil
		}
	}

	return &hydrapb.Uint32SliceIsValueExistResponse{IsExist: false}, nil
}

func (g Gateway) IncrementInt8(ctx context.Context, in *hydrapb.IncrementInt8Request) (*hydrapb.IncrementInt8Response, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	if in.SwampName == "" {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "SwampName cannot be empty")
	}
	if in.IncrementBy == 0 {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "IncrementBy cannot be zero")
	}

	// check the name of the swamp
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, false)
	if err != nil {
		return nil, err
	}

	// get the hydra interface
	hydraInterface := g.ZeusInterface.GetHydra()

	// summon the swamp
	swampObj, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}

	// begin the vigil, to prevent closing of the swamp
	swampObj.BeginVigil()
	defer swampObj.CeaseVigil()

	// create the condition if it is not nil
	var condition *swamp.IncrementInt8Condition
	if in.GetCondition() != nil {
		condition = &swamp.IncrementInt8Condition{
			RelationalOperator: relationalOperatorToSwampRelationalOperator(in.GetCondition().GetRelationalOperator()),
			Value:              int8(in.GetCondition().GetValue()),
		}
	}

	// increment the value with the condition
	newValue, isIncremented, err := swampObj.IncrementInt8(in.Key, int8(in.IncrementBy), condition)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("the value of the key is not an integer: %s", err.Error()))
	}

	// return with the new value and the status of the increment
	return &hydrapb.IncrementInt8Response{
		Value:         int32(newValue),
		IsIncremented: isIncremented,
	}, nil

}

func (g Gateway) IncrementInt16(ctx context.Context, in *hydrapb.IncrementInt16Request) (*hydrapb.IncrementInt16Response, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	if in.SwampName == "" {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "SwampName cannot be empty")
	}
	if in.IncrementBy == 0 {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "IncrementBy cannot be zero")
	}

	// check the name of the swamp
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, false)
	if err != nil {
		return nil, err
	}

	// get the hydra interface
	hydraInterface := g.ZeusInterface.GetHydra()

	// summon the swamp
	swampObj, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}

	// begin the vigil, to prevent closing of the swamp
	swampObj.BeginVigil()
	defer swampObj.CeaseVigil()

	// create the condition if it is not nil
	var condition *swamp.IncrementInt16Condition
	if in.GetCondition() != nil {
		condition = &swamp.IncrementInt16Condition{
			RelationalOperator: relationalOperatorToSwampRelationalOperator(in.GetCondition().GetRelationalOperator()),
			Value:              int16(in.GetCondition().GetValue()),
		}
	}

	// increment the value with the condition
	newValue, isIncremented, err := swampObj.IncrementInt16(in.Key, int16(in.IncrementBy), condition)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("the value of the key is not an integer: %s", err.Error()))
	}

	// return with the new value and the status of the increment
	return &hydrapb.IncrementInt16Response{
		Value:         int32(newValue),
		IsIncremented: isIncremented,
	}, nil

}

func (g Gateway) IncrementInt32(ctx context.Context, in *hydrapb.IncrementInt32Request) (*hydrapb.IncrementInt32Response, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	if in.SwampName == "" {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "SwampName cannot be empty")
	}
	if in.IncrementBy == 0 {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "IncrementBy cannot be zero")
	}

	// check the name of the swamp
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, false)
	if err != nil {
		return nil, err
	}

	// get the hydra interface
	hydraInterface := g.ZeusInterface.GetHydra()

	// summon the swamp
	swampObj, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}

	// begin the vigil, to prevent closing of the swamp
	swampObj.BeginVigil()
	defer swampObj.CeaseVigil()

	// create the condition if it is not nil
	var condition *swamp.IncrementInt32Condition
	if in.GetCondition() != nil {
		condition = &swamp.IncrementInt32Condition{
			RelationalOperator: relationalOperatorToSwampRelationalOperator(in.GetCondition().GetRelationalOperator()),
			Value:              in.GetCondition().GetValue(),
		}
	}

	// increment the value with the condition
	newValue, isIncremented, err := swampObj.IncrementInt32(in.Key, in.IncrementBy, condition)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("the value of the key is not an integer: %s", err.Error()))
	}

	// return with the new value and the status of the increment
	return &hydrapb.IncrementInt32Response{
		Value:         newValue,
		IsIncremented: isIncremented,
	}, nil

}

func (g Gateway) IncrementInt64(ctx context.Context, in *hydrapb.IncrementInt64Request) (*hydrapb.IncrementInt64Response, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	if in.SwampName == "" {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "SwampName cannot be empty")
	}
	if in.IncrementBy == 0 {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "IncrementBy cannot be zero")
	}

	// check the name of the swamp
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, false)
	if err != nil {
		return nil, err
	}

	// get the hydra interface
	hydraInterface := g.ZeusInterface.GetHydra()

	// summon the swamp
	swampObj, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}

	// begin the vigil, to prevent closing of the swamp
	swampObj.BeginVigil()
	defer swampObj.CeaseVigil()

	// create the condition if it is not nil
	var condition *swamp.IncrementInt64Condition
	if in.GetCondition() != nil {
		condition = &swamp.IncrementInt64Condition{
			RelationalOperator: relationalOperatorToSwampRelationalOperator(in.GetCondition().GetRelationalOperator()),
			Value:              in.GetCondition().GetValue(),
		}
	}

	// increment the value with the condition
	newValue, isIncremented, err := swampObj.IncrementInt64(in.Key, in.IncrementBy, condition)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("the value of the key is not an integer: %s", err.Error()))
	}

	// return with the new value and the status of the increment
	return &hydrapb.IncrementInt64Response{
		Value:         newValue,
		IsIncremented: isIncremented,
	}, nil

}

func (g Gateway) IncrementUint8(ctx context.Context, in *hydrapb.IncrementUint8Request) (*hydrapb.IncrementUint8Response, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	if in.SwampName == "" {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "SwampName cannot be empty")
	}
	if in.IncrementBy == 0 {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "IncrementBy cannot be zero")
	}

	// check the name of the swamp
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, false)
	if err != nil {
		return nil, err
	}

	// get the hydra interface
	hydraInterface := g.ZeusInterface.GetHydra()

	// summon the swamp
	swampObj, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}

	// begin the vigil, to prevent closing of the swamp
	swampObj.BeginVigil()
	defer swampObj.CeaseVigil()

	// create the condition if it is not nil
	var condition *swamp.IncrementUInt8Condition
	if in.GetCondition() != nil {
		condition = &swamp.IncrementUInt8Condition{
			RelationalOperator: relationalOperatorToSwampRelationalOperator(in.GetCondition().GetRelationalOperator()),
			Value:              uint8(in.GetCondition().GetValue()),
		}
	}

	// increment the value with the condition
	newValue, isIncremented, err := swampObj.IncrementUint8(in.Key, uint8(in.IncrementBy), condition)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("the value of the key is not an integer: %s", err.Error()))
	}

	// return with the new value and the status of the increment
	return &hydrapb.IncrementUint8Response{
		Value:         uint32(newValue),
		IsIncremented: isIncremented,
	}, nil

}

func (g Gateway) IncrementUint16(ctx context.Context, in *hydrapb.IncrementUint16Request) (*hydrapb.IncrementUint16Response, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	if in.SwampName == "" {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "SwampName cannot be empty")
	}
	if in.IncrementBy == 0 {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "IncrementBy cannot be zero")
	}

	// check the name of the swamp
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, false)
	if err != nil {
		return nil, err
	}

	// get the hydra interface
	hydraInterface := g.ZeusInterface.GetHydra()

	// summon the swamp
	swampObj, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}

	// begin the vigil, to prevent closing of the swamp
	swampObj.BeginVigil()
	defer swampObj.CeaseVigil()

	// create the condition if it is not nil
	var condition *swamp.IncrementUInt16Condition
	if in.GetCondition() != nil {
		condition = &swamp.IncrementUInt16Condition{
			RelationalOperator: relationalOperatorToSwampRelationalOperator(in.GetCondition().GetRelationalOperator()),
			Value:              uint16(in.GetCondition().GetValue()),
		}
	}

	// increment the value with the condition
	newValue, isIncremented, err := swampObj.IncrementUint16(in.Key, uint16(in.IncrementBy), condition)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("the value of the key is not an integer: %s", err.Error()))
	}

	// return with the new value and the status of the increment
	return &hydrapb.IncrementUint16Response{
		Value:         uint32(newValue),
		IsIncremented: isIncremented,
	}, nil

}

func (g Gateway) IncrementUint32(ctx context.Context, in *hydrapb.IncrementUint32Request) (*hydrapb.IncrementUint32Response, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	if in.SwampName == "" {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "SwampName cannot be empty")
	}
	if in.IncrementBy == 0 {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "IncrementBy cannot be zero")
	}

	// check the name of the swamp
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, false)
	if err != nil {
		return nil, err
	}

	// get the hydra interface
	hydraInterface := g.ZeusInterface.GetHydra()

	// summon the swamp
	swampObj, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}

	// begin the vigil, to prevent closing of the swamp
	swampObj.BeginVigil()
	defer swampObj.CeaseVigil()

	// create the condition if it is not nil
	var condition *swamp.IncrementUInt32Condition
	if in.GetCondition() != nil {
		condition = &swamp.IncrementUInt32Condition{
			RelationalOperator: relationalOperatorToSwampRelationalOperator(in.GetCondition().GetRelationalOperator()),
			Value:              in.GetCondition().GetValue(),
		}
	}

	// increment the value with the condition
	newValue, isIncremented, err := swampObj.IncrementUint32(in.Key, in.IncrementBy, condition)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("the value of the key is not an integer: %s", err.Error()))
	}

	// return with the new value and the status of the increment
	return &hydrapb.IncrementUint32Response{
		Value:         newValue,
		IsIncremented: isIncremented,
	}, nil

}

func (g Gateway) IncrementUint64(ctx context.Context, in *hydrapb.IncrementUint64Request) (*hydrapb.IncrementUint64Response, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	if in.SwampName == "" {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "SwampName cannot be empty")
	}
	if in.IncrementBy == 0 {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "IncrementBy cannot be zero")
	}

	// check the name of the swamp
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, false)
	if err != nil {
		return nil, err
	}

	// get the hydra interface
	hydraInterface := g.ZeusInterface.GetHydra()

	// summon the swamp
	swampObj, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}

	// begin the vigil, to prevent closing of the swamp
	swampObj.BeginVigil()
	defer swampObj.CeaseVigil()

	// create the condition if it is not nil
	var condition *swamp.IncrementUInt64Condition
	if in.GetCondition() != nil {
		condition = &swamp.IncrementUInt64Condition{
			RelationalOperator: relationalOperatorToSwampRelationalOperator(in.GetCondition().GetRelationalOperator()),
			Value:              in.GetCondition().GetValue(),
		}
	}

	// increment the value with the condition
	newValue, isIncremented, err := swampObj.IncrementUint64(in.Key, in.IncrementBy, condition)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("the value of the key is not an integer: %s", err.Error()))
	}

	// return with the new value and the status of the increment
	return &hydrapb.IncrementUint64Response{
		Value:         newValue,
		IsIncremented: isIncremented,
	}, nil

}

func (g Gateway) IncrementFloat32(ctx context.Context, in *hydrapb.IncrementFloat32Request) (*hydrapb.IncrementFloat32Response, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	if in.SwampName == "" {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "SwampName cannot be empty")
	}
	if in.IncrementBy == 0 {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "IncrementBy cannot be zero")
	}

	// check the name of the swamp
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, false)
	if err != nil {
		return nil, err
	}

	// get the hydra interface
	hydraInterface := g.ZeusInterface.GetHydra()

	// summon the swamp
	swampObj, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}

	// begin the vigil, to prevent closing of the swamp
	swampObj.BeginVigil()
	defer swampObj.CeaseVigil()

	// create the condition if it is not nil
	var condition *swamp.IncrementFloat32Condition
	if in.GetCondition() != nil {
		condition = &swamp.IncrementFloat32Condition{
			RelationalOperator: relationalOperatorToSwampRelationalOperator(in.GetCondition().GetRelationalOperator()),
			Value:              in.GetCondition().GetValue(),
		}
	}

	// increment the value with the condition
	newValue, isIncremented, err := swampObj.IncrementFloat32(in.Key, in.IncrementBy, condition)

	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("the value of the key is not a float: %s", err.Error()))
	}

	// return with the new value and the status of the increment
	return &hydrapb.IncrementFloat32Response{
		Value:         newValue,
		IsIncremented: isIncremented,
	}, nil

}

func (g Gateway) IncrementFloat64(ctx context.Context, in *hydrapb.IncrementFloat64Request) (*hydrapb.IncrementFloat64Response, error) {

	g.ZeusInterface.GetSafeops().LockSystem()
	defer g.ZeusInterface.GetSafeops().UnlockSystem()

	defer handlePanic()

	if in.SwampName == "" {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "SwampName cannot be empty")
	}
	if in.IncrementBy == 0 {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "IncrementBy cannot be zero")
	}

	// check the name of the swamp
	swampName, err := checkSwampName(g.ZeusInterface, in.GetIslandID(), in.SwampName, false)
	if err != nil {
		return nil, err
	}

	// get the hydra interface
	hydraInterface := g.ZeusInterface.GetHydra()

	// summon the swamp
	swampObj, err := hydraInterface.SummonSwamp(ctx, in.GetIslandID(), swampName)
	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.Internal, fmt.Sprintf("internal server error in hydra: %s", err.Error()))
	}

	// begin the vigil, to prevent closing of the swamp
	swampObj.BeginVigil()
	defer swampObj.CeaseVigil()

	// create the condition if it is not nil
	var condition *swamp.IncrementFloat64Condition
	if in.GetCondition() != nil {
		condition = &swamp.IncrementFloat64Condition{
			RelationalOperator: relationalOperatorToSwampRelationalOperator(in.GetCondition().GetRelationalOperator()),
			Value:              in.GetCondition().GetValue(),
		}
	}

	// increment the value with the condition
	newValue, isIncremented, err := swampObj.IncrementFloat64(in.Key, in.IncrementBy, condition)

	if err != nil {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("the value of the key is not a float: %s", err.Error()))
	}

	// return with the new value and the status of the increment
	return &hydrapb.IncrementFloat64Response{
		Value:         newValue,
		IsIncremented: isIncremented,
	}, nil

}

// keyValuesToTreasure converts the key value pairs to the treasure content
// this function not save the treasure, only set its content
func keyValuesToTreasure(keyValuePair *hydrapb.KeyValuePair, treasureInterface treasure.Treasure, guardID guard.ID) {

	// Ensure keyValuePair is not nil to avoid panic
	if keyValuePair == nil {
		treasureInterface.SetContentVoid(guardID)
		return
	}

	// convert interface to the correct type and set the content
	switch {
	case keyValuePair.Int8Val != nil:
		treasureInterface.SetContentInt8(guardID, int8(*keyValuePair.Int8Val))
	case keyValuePair.Int16Val != nil:
		treasureInterface.SetContentInt16(guardID, int16(*keyValuePair.Int16Val))
	case keyValuePair.Int32Val != nil:
		treasureInterface.SetContentInt32(guardID, *keyValuePair.Int32Val)
	case keyValuePair.Int64Val != nil:
		treasureInterface.SetContentInt64(guardID, *keyValuePair.Int64Val)
	case keyValuePair.Uint8Val != nil:
		treasureInterface.SetContentUint8(guardID, uint8(*keyValuePair.Uint8Val))
	case keyValuePair.Uint16Val != nil:
		treasureInterface.SetContentUint16(guardID, uint16(*keyValuePair.Uint16Val))
	case keyValuePair.Uint32Val != nil:
		treasureInterface.SetContentUint32(guardID, *keyValuePair.Uint32Val)
	case keyValuePair.Uint64Val != nil:
		treasureInterface.SetContentUint64(guardID, *keyValuePair.Uint64Val)
	case keyValuePair.Float32Val != nil:
		treasureInterface.SetContentFloat32(guardID, *keyValuePair.Float32Val)
	case keyValuePair.Float64Val != nil:
		treasureInterface.SetContentFloat64(guardID, *keyValuePair.Float64Val)
	case keyValuePair.StringVal != nil:
		treasureInterface.SetContentString(guardID, *keyValuePair.StringVal)
	case keyValuePair.BoolVal != nil:
		var booleanValue bool
		switch *keyValuePair.BoolVal {
		case hydrapb.Boolean_TRUE:
			booleanValue = true
		case hydrapb.Boolean_FALSE:
			booleanValue = false
		default:
			booleanValue = false
		}
		treasureInterface.SetContentBool(guardID, booleanValue)
	case keyValuePair.BytesVal != nil:
		treasureInterface.SetContentByteArray(guardID, keyValuePair.BytesVal)
	case keyValuePair.Uint32Slice != nil:

		if err := treasureInterface.Uint32SlicePush(keyValuePair.Uint32Slice); err != nil {
			slog.Error("failed to push the uint32 slice to the treasure", "error", err.Error())
		}
	case keyValuePair.VoidVal != nil && *keyValuePair.VoidVal:
		// set the content to void
		treasureInterface.SetContentVoid(guardID)
	default:
		treasureInterface.SetContentVoid(guardID)
	}

	// set other values if they are not empty
	if isValidTimestamp(keyValuePair.GetCreatedAt()) {
		treasureInterface.SetCreatedAt(guardID, keyValuePair.GetCreatedAt().AsTime())
	}
	if keyValuePair.GetCreatedBy() != "" {
		treasureInterface.SetCreatedBy(guardID, keyValuePair.GetCreatedBy())
	}
	if isValidTimestamp(keyValuePair.GetUpdatedAt()) {
		treasureInterface.SetModifiedAt(guardID, keyValuePair.GetUpdatedAt().AsTime())
	}
	if keyValuePair.GetUpdatedBy() != "" {
		treasureInterface.SetModifiedBy(guardID, keyValuePair.GetUpdatedBy())
	}
	if isValidTimestamp(keyValuePair.GetExpiredAt()) {
		treasureInterface.SetExpirationTime(guardID, keyValuePair.GetExpiredAt().AsTime())
	}
}

// treasureToKeyValuePair converts the treasure content from the hydra to the protobuf format
func treasureToKeyValuePair(treasureInterface treasure.Treasure, t *hydrapb.Treasure) {

	// Set the key of the treasure
	t.Key = treasureInterface.GetKey()
	t.IsExist = true

	switch treasureInterface.GetContentType() {
	case treasure.ContentTypeInt8:
		val, contentErr := treasureInterface.GetContentInt8()
		if contentErr == nil {
			v := int32(val)
			t.Int8Val = &v
		}
	case treasure.ContentTypeInt16:
		val, contentErr := treasureInterface.GetContentInt16()
		if contentErr == nil {
			v := int32(val)
			t.Int16Val = &v
		}
	case treasure.ContentTypeInt32:
		val, contentErr := treasureInterface.GetContentInt32()
		if contentErr == nil {
			t.Int32Val = &val
		}
	case treasure.ContentTypeInt64:
		val, contentErr := treasureInterface.GetContentInt64()
		if contentErr == nil {
			t.Int64Val = &val
		}
	case treasure.ContentTypeUint8:
		val, contentErr := treasureInterface.GetContentUint8()
		if contentErr == nil {
			v := uint32(val)
			t.Uint8Val = &v
		}
	case treasure.ContentTypeUint16:
		val, contentErr := treasureInterface.GetContentUint16()
		if contentErr == nil {
			v := uint32(val)
			t.Uint16Val = &v
		}
	case treasure.ContentTypeUint32:
		val, contentErr := treasureInterface.GetContentUint32()
		if contentErr == nil {
			t.Uint32Val = &val
		}
	case treasure.ContentTypeUint64:
		val, contentErr := treasureInterface.GetContentUint64()
		if contentErr == nil {
			t.Uint64Val = &val
		}
	case treasure.ContentTypeFloat32:
		val, contentErr := treasureInterface.GetContentFloat32()
		if contentErr == nil {
			t.Float32Val = &val
		}
	case treasure.ContentTypeFloat64:
		val, contentErr := treasureInterface.GetContentFloat64()
		if contentErr == nil {
			t.Float64Val = &val
		}
	case treasure.ContentTypeString:
		val, contentErr := treasureInterface.GetContentString()
		if contentErr == nil {
			t.StringVal = &val
		}
	case treasure.ContentTypeBoolean:
		val, contentErr := treasureInterface.GetContentBool()
		if contentErr == nil {
			// convert the boolean to the protobuf type
			if val {
				t.BoolVal = hydrapb.Boolean_TRUE.Enum()
			} else {
				t.BoolVal = hydrapb.Boolean_FALSE.Enum()
			}
		}
	case treasure.ContentTypeByteArray:
		val, contentErr := treasureInterface.GetContentByteArray()
		if contentErr == nil {
			t.BytesVal = val
		}
	case treasure.ContentTypeUint32Slice:
		val, contentErr := treasureInterface.Uint32SliceGetAll()
		if contentErr == nil {
			t.Uint32Slice = val
		}
	default:
		// do nothing
	}

	if treasureInterface.GetCreatedAt() > 0 {
		t.CreatedAt = timestamppb.New(time.Unix(0, treasureInterface.GetCreatedAt()))
	}
	if treasureInterface.GetCreatedBy() != "" {
		createdBy := t.GetCreatedBy()
		t.CreatedBy = &createdBy
	}
	if treasureInterface.GetModifiedAt() > 0 {
		t.UpdatedAt = timestamppb.New(time.Unix(0, treasureInterface.GetModifiedAt()))
	}
	if t.GetUpdatedBy() != "" {
		updatedBy := t.GetUpdatedBy()
		t.UpdatedBy = &updatedBy
	}
	if treasureInterface.GetExpirationTime() > 0 {
		t.ExpiredAt = timestamppb.New(time.Unix(0, treasureInterface.GetExpirationTime()))
	}

}

// isValidTimestamp checks if the timestamp is valid
func isValidTimestamp(ts *timestamppb.Timestamp) bool {
	if ts == nil {
		return false
	}
	return ts.GetSeconds() > 0 || ts.GetNanos() > 0
}

// convertTreasureStatusToPbStatus converts the treasure status from the hydra to the protobuf status
func convertTreasureStatusToPbStatus(treasureStatus treasure.TreasureStatus) hydrapb.Status_Code {
	switch treasureStatus {
	case treasure.StatusNew:
		return hydrapb.Status_NEW
	case treasure.StatusModified:
		return hydrapb.Status_UPDATED
	case treasure.StatusSame:
		return hydrapb.Status_NOTHING_CHANGED
	case treasure.StatusDeleted:
		return hydrapb.Status_DELETED
	default:
		return hydrapb.Status_NOT_FOUND
	}
}

// handle all the panics in the gateway
func handlePanic() {
	if r := recover(); r != nil {
		// get the stack trace
		stackTrace := debug.Stack()
		// log the panic with the error and stack trace
		slog.Error("grpc gateway panic", "error", r, "stack", string(stackTrace))
	}
}

// checkSwampName check if the swamp name is valid and exist or not.
// The function will return a grpc error message if the swamp name is invalid or does not exist.
func checkSwampName(zeusInterface zeus.Zeus, islandID uint64, inputSwampName string, checkExist bool) (name.Name, error) {

	// check the input
	if inputSwampName == "" {
		// return with grpc error message
		return nil, status.Error(codes.InvalidArgument, "SwampName cannot be empty")
	}
	swampName := name.Load(inputSwampName)

	// check the existence of the swamp only if it is needed
	// because the Set method does not need to check the existence of the swamp
	if checkExist {
		// check if the swamp is exist or not
		hydraInterface := zeusInterface.GetHydra()
		isExist, err := hydraInterface.IsExistSwamp(islandID, swampName)
		if err != nil || !isExist {
			// return with grpc error message
			return nil, status.Error(codes.FailedPrecondition, "Swamp does not exist")
		}
	}

	return swampName, nil

}

func inputIndexTypeToBeaconType(inputIndexType hydrapb.IndexType_Type) swamp.BeaconType {
	switch inputIndexType {
	case hydrapb.IndexType_EXPIRATION_TIME:
		return swamp.BeaconTypeExpirationTime
	case hydrapb.IndexType_CREATION_TIME:
		return swamp.BeaconTypeCreationTime
	case hydrapb.IndexType_UPDATE_TIME:
		return swamp.BeaconTypeUpdateTime
	case hydrapb.IndexType_VALUE_INT8:
		return swamp.BeaconTypeValueInt8
	case hydrapb.IndexType_VALUE_INT16:
		return swamp.BeaconTypeValueInt16
	case hydrapb.IndexType_VALUE_INT32:
		return swamp.BeaconTypeValueInt32
	case hydrapb.IndexType_VALUE_INT64:
		return swamp.BeaconTypeValueInt64
	case hydrapb.IndexType_VALUE_UINT8:
		return swamp.BeaconTypeValueUint8
	case hydrapb.IndexType_VALUE_UINT16:
		return swamp.BeaconTypeValueUint16
	case hydrapb.IndexType_VALUE_UINT32:
		return swamp.BeaconTypeValueUint32
	case hydrapb.IndexType_VALUE_UINT64:
		return swamp.BeaconTypeValueUint64
	case hydrapb.IndexType_VALUE_FLOAT32:
		return swamp.BeaconTypeValueFloat32
	case hydrapb.IndexType_VALUE_FLOAT64:
		return swamp.BeaconTypeValueFloat64
	case hydrapb.IndexType_VALUE_STRING:
		return swamp.BeaconTypeValueString
	default:
		return swamp.BeaconTypeValueString
	}
}

func inputOrderTypeToBeaconOrderType(inputOrderType hydrapb.OrderType_Type) swamp.BeaconOrder {
	switch inputOrderType {
	case hydrapb.OrderType_ASC:
		return swamp.IndexOrderAsc
	case hydrapb.OrderType_DESC:
		return swamp.IndexOrderDesc
	default:
		return swamp.IndexOrderAsc
	}
}

func relationalOperatorToSwampRelationalOperator(operator hydrapb.Relational_Operator) swamp.RelationalOperator {
	switch operator {
	case hydrapb.Relational_EQUAL:
		return swamp.RelationalOperatorEqual
	case hydrapb.Relational_LESS_THAN:
		return swamp.RelationalOperatorLessThan
	case hydrapb.Relational_LESS_THAN_OR_EQUAL:
		return swamp.RelationalOperatorLessThanOrEqual
	case hydrapb.Relational_GREATER_THAN:
		return swamp.RelationalOperatorGreaterThan
	case hydrapb.Relational_GREATER_THAN_OR_EQUAL:
		return swamp.RelationalOperatorGreaterThanOrEqual
	case hydrapb.Relational_NOT_EQUAL:
		return swamp.RelationalOperatorNotEqual
	default:
		return swamp.RelationalOperatorEqual
	}
}
