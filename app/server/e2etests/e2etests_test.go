package e2etests

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/server/server"
	"github.com/hydraide/hydraide/generated/hydraidepbgo"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/client"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var serverInterface server.Server
var clientInterface client.Client
var serverGlobalName name.Name

func TestMain(m *testing.M) {

	fmt.Println("Setting up test environment...")
	setup()
	code := m.Run()
	fmt.Println("Tearing down test environment...")
	teardown()
	os.Exit(code)
}

func setup() {

	serverGlobalName = name.New().Sanctuary("server").Realm("global").Swamp("name")

	slog.SetLogLoggerLevel(slog.LevelDebug)

	if os.Getenv("E2E_HYDRA_SERVER_CRT") == "" {
		slog.Error("E2E_HYDRA_SERVER_CRT environment variable is not set")
		panic("E2E_HYDRA_SERVER_CRT environment variable is not set")
	}
	if os.Getenv("E2E_HYDRA_SERVER_KEY") == "" {
		slog.Error("HYDRA_SERVER_KEY environment variable is not set")
		panic("HYDRA_SERVER_KEY environment variable is not set")
	}
	if os.Getenv("E2E_HYDRA_CA_CRT") == "" {
		slog.Error("E2E_HYDRA_CA_CRT environment variable is not set")
		panic("E2E_HYDRA_CA_CRT environment variable is not set")
	}

	if os.Getenv("E2E_HYDRA_CLIENT_CRT") == "" {
		slog.Error("E2E_HYDRA_CLIENT_CRT environment variable is not set")
		panic("E2E_HYDRA_CLIENT_CRT environment variable is not set")
	}

	if os.Getenv("E2E_HYDRA_CLIENT_KEY") == "" {
		slog.Error("E2E_HYDRA_CLIENT_KEY environment variable is not set")
		panic("E2E_HYDRA_CLIENT_KEY environment variable is not set")
	}

	if os.Getenv("HYDRA_E2E_GRPC_CONN_ANALYSIS") == "" {
		slog.Warn("HYDRA_E2E_GRPC_CONN_ANALYSIS environment variable is not set, using default value: false")
		if err := os.Setenv("HYDRA_E2E_GRPC_CONN_ANALYSIS", "false"); err != nil {
			slog.Error("error while setting HYDRA_E2E_GRPC_CONN_ANALYSIS environment variable", "error", err)
			panic(fmt.Sprintf("error while setting HYDRA_E2E_GRPC_CONN_ANALYSIS environment variable: %v", err))
		}
	}

	port := strings.Split(os.Getenv("E2E_HYDRA_TEST_SERVER"), ":")
	if len(port) != 2 {
		slog.Error("E2E_HYDRA_TEST_SERVER environment variable is not set or invalid")
		panic("E2E_HYDRA_TEST_SERVER environment variable is not set or invalid")
	}

	portAsNUmber, err := strconv.Atoi(port[1])
	if err != nil {
		slog.Error("E2E_HYDRA_TEST_SERVER port is not a valid number", "error", err)
		panic(fmt.Sprintf("E2E_HYDRA_TEST_SERVER port is not a valid number: %v", err))
	}

	// start the new Hydra server with V2 engine
	serverInterface = server.New(&server.Configuration{
		CertificateCrtFile:  os.Getenv("E2E_HYDRA_SERVER_CRT"),
		CertificateKeyFile:  os.Getenv("E2E_HYDRA_SERVER_KEY"),
		ClientCAFile:        os.Getenv("E2E_HYDRA_CA_CRT"), // this is the CA that signed the client certificates
		HydraServerPort:     portAsNUmber,
		HydraMaxMessageSize: 1024 * 1024 * 1024, // 1 GB
		UseV2Engine:         true,               // Use the new V2 append-only storage engine
	})

	if err := serverInterface.Start(); err != nil {
		slog.Error("error while starting the server", "error", err)
		panic(fmt.Sprintf("error while starting the server: %v", err))
	}

	createGrpcClient()

}

func teardown() {
	// stop the microservice and exit the program
	serverInterface.Stop()
	slog.Info("server stopped gracefully. Program is exiting...")
	// waiting for logs to be written to the file
	time.Sleep(1 * time.Second)
	// exit the program if the microservice is stopped gracefully
	os.Exit(0)
}

func createGrpcClient() {

	// create a new gRPC client object
	servers := []*client.Server{
		{
			Host:          os.Getenv("E2E_HYDRA_TEST_SERVER"),
			FromIsland:    0,
			ToIsland:      100,
			CACrtPath:     os.Getenv("E2E_HYDRA_CA_CRT"),
			ClientCrtPath: os.Getenv("E2E_HYDRA_CLIENT_CRT"),
			ClientKeyPath: os.Getenv("E2E_HYDRA_CLIENT_KEY"),
		},
	}

	// 100 folders and 2 gig message size
	clientInterface = client.New(servers, 100, 2147483648)
	if err := clientInterface.Connect(false); err != nil {
		slog.Error("error while connecting to the server", "error", err)
	}

}

func TestLockAndUnlock(t *testing.T) {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	lockKey := "myLockKey"
	maxTTL := 10 * time.Second

	lockResponse, err := clientInterface.GetServiceClient(serverGlobalName).Lock(ctx, &hydraidepbgo.LockRequest{
		Key: lockKey,
		TTL: maxTTL.Milliseconds(),
	})

	assert.NoError(t, err)
	assert.NotNil(t, lockResponse)

	unlockResponse, err := clientInterface.GetServiceClient(serverGlobalName).Unlock(ctx, &hydraidepbgo.UnlockRequest{
		Key:    lockKey,
		LockID: lockResponse.GetLockID(),
	})

	assert.NoError(t, err)
	assert.NotNil(t, unlockResponse)

}

func TestLockAndUnlockWithoutTTL(t *testing.T) {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	lockKey := "myLockKey"
	maxTTL := int64(0)

	lockResponse, err := clientInterface.GetServiceClient(serverGlobalName).Lock(ctx, &hydraidepbgo.LockRequest{
		Key: lockKey,
		TTL: maxTTL,
	})

	assert.NoError(t, err)
	assert.NotNil(t, lockResponse)

	unlockResponse, err := clientInterface.GetServiceClient(serverGlobalName).Unlock(ctx, &hydraidepbgo.UnlockRequest{
		Key:    lockKey,
		LockID: lockResponse.GetLockID(),
	})

	assert.NoError(t, err)
	assert.NotNil(t, unlockResponse)

}

func TestWaitingForAutoUnlock(t *testing.T) {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	lockKey := "myLockKey"
	maxTTL := int64(0)

	// the server will set this lock for 1 second
	_, err := clientInterface.GetServiceClient(serverGlobalName).Lock(ctx, &hydraidepbgo.LockRequest{
		Key: lockKey,
		TTL: maxTTL,
	})

	assert.NoError(t, err)

	time.Sleep(2 * time.Second) // wait for the lock to be auto-unlocked

	// try to lock it again with same key
	lockResponse, err := clientInterface.GetServiceClient(serverGlobalName).Lock(ctx, &hydraidepbgo.LockRequest{
		Key: lockKey,
		TTL: maxTTL,
	})

	assert.NoError(t, err)
	assert.NotNil(t, lockResponse)

}

func TestGateway_Set(t *testing.T) {

	writeInterval := int64(1)
	maxFileSize := int64(65536)

	swampPattern := name.New().Sanctuary("dizzlets").Realm("*").Swamp("*")
	selectedClient := clientInterface.GetServiceClient(swampPattern)
	_, err := selectedClient.RegisterSwamp(context.Background(), &hydraidepbgo.RegisterSwampRequest{
		SwampPattern:   swampPattern.Get(),
		CloseAfterIdle: int64(3600),
		WriteInterval:  &writeInterval,
		MaxFileSize:    &maxFileSize,
	})

	swampName := name.New().Sanctuary("dizzlets").Realm("testing").Swamp("set-and-get")
	swampClient := clientInterface.GetServiceClient(swampName)
	defer func() {
		_, err = swampClient.Destroy(context.Background(), &hydraidepbgo.DestroyRequest{
			SwampName: swampName.Get(),
		})
		assert.NoError(t, err)
	}()

	var keyValues []*hydraidepbgo.KeyValuePair
	for i := 0; i < 10; i++ {
		myVal := fmt.Sprintf("value-%d", i)
		createdBy := "trendizz"
		keyValues = append(keyValues, &hydraidepbgo.KeyValuePair{
			Key:       fmt.Sprintf("key-%d", i),
			StringVal: &myVal,
			CreatedAt: timestamppb.Now(),
			CreatedBy: &createdBy,
			UpdatedAt: timestamppb.Now(),
			UpdatedBy: &createdBy,
			ExpiredAt: timestamppb.Now(),
		})
	}

	// try to set a value to the swamp
	response, err := swampClient.Set(context.Background(), &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				SwampName:        swampName.Get(),
				CreateIfNotExist: true,
				Overwrite:        true,
				KeyValues:        keyValues,
			},
		}})

	assert.NoError(t, err)
	assert.NotNil(t, response)

	slog.Debug("response from the server", "response", response)

	assert.Equal(t, 1, len(response.GetSwamps()), "response should contain one swamp")
	assert.Equal(t, 10, len(response.GetSwamps()[0].GetKeysAndStatuses()), "the swamp should contain 10 keys")

	// try to get back all data from the swamp
	getResponse, err := swampClient.Get(context.Background(), &hydraidepbgo.GetRequest{
		Swamps: []*hydraidepbgo.GetSwamp{
			{
				SwampName: swampName.Get(),
				Keys:      []string{"key-0", "key-1", "key-2", "key-3", "key-4", "key-5", "key-6", "key-7", "key-8", "key-9", "key-10"},
			},
		},
	})

	assert.NoError(t, err)
	assert.NotNil(t, getResponse)

	// print the data to the console
	treasureExistCounter := 0
	treasureNotExistCounter := 0
	for _, getResponseValue := range getResponse.GetSwamps() {
		slog.Debug("swamp found", "swamp", getResponseValue.GetSwampName())
		for _, treasure := range getResponseValue.GetTreasures() {
			if treasure.IsExist {
				treasureExistCounter++
			} else {
				treasureNotExistCounter++
			}
		}
	}

	assert.Equal(t, 10, treasureExistCounter)
	assert.Equal(t, 1, treasureNotExistCounter)

	// destroy test swamps
	destroySwamp(clientInterface.GetServiceClient(swampName), swampName)

}

func TestRegisterSwamp(t *testing.T) {

	writeInterval := int64(1)
	maxFileSize := int64(65536)

	swampPattern := name.New().Sanctuary("dizzlets").Realm("*").Swamp("*")
	selectedClient := clientInterface.GetServiceClient(swampPattern)
	response, err := selectedClient.RegisterSwamp(context.Background(), &hydraidepbgo.RegisterSwampRequest{
		SwampPattern:    swampPattern.Get(),
		CloseAfterIdle:  int64(3600),
		IsInMemorySwamp: false,
		WriteInterval:   &writeInterval,
		MaxFileSize:     &maxFileSize,
	})

	assert.NoError(t, err, "error should be nil")
	assert.NotNil(t, response, "response should not be nil")

}

func TestGateway_SubscribeToEvent(t *testing.T) {

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	writeInterval := int64(1)
	maxFileSize := int64(65536)

	swampPattern := name.New().Sanctuary("subscribe").Realm("to").Swamp("event")

	destroySwamp(clientInterface.GetServiceClient(swampPattern), swampPattern)
	defer func() {
		destroySwamp(clientInterface.GetServiceClient(swampPattern), swampPattern)
		slog.Info("swamp destroyed at the end of the test", "swamp", swampPattern.Get())
	}()

	selectedClient := clientInterface.GetServiceClient(swampPattern)
	_, err := selectedClient.RegisterSwamp(context.Background(), &hydraidepbgo.RegisterSwampRequest{
		SwampPattern:   swampPattern.Get(),
		CloseAfterIdle: int64(3600),
		WriteInterval:  &writeInterval,
		MaxFileSize:    &maxFileSize,
	})

	assert.NoError(t, err)

	eventClient, err := selectedClient.SubscribeToEvents(ctx, &hydraidepbgo.SubscribeToEventsRequest{
		SwampName: swampPattern.Get(),
	})

	assert.NoError(t, err, "error should be nil")

	testTreasures := 5
	wg := &sync.WaitGroup{}
	wg.Add(testTreasures)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				event, err := eventClient.Recv()

				if err != nil || event == nil {
					continue
				}

				slog.Debug("event received", "treasure key", event.Treasure.GetKey(), "treasure value", event.Treasure.GetStringVal())

				wg.Done()
			}
		}
	}()

	var keyValues []*hydraidepbgo.KeyValuePair
	for i := 0; i < testTreasures; i++ {
		myVal := fmt.Sprintf("value-%d", i)
		keyValues = append(keyValues, &hydraidepbgo.KeyValuePair{
			Key:       fmt.Sprintf("key-%d", i),
			StringVal: &myVal,
		})
	}

	swampsRequest := []*hydraidepbgo.SwampRequest{
		{
			SwampName:        swampPattern.Get(),
			KeyValues:        keyValues,
			CreateIfNotExist: true,
			Overwrite:        true,
		},
	}

	// set the treasures to the swamp
	_, err = selectedClient.Set(context.Background(), &hydraidepbgo.SetRequest{
		Swamps: swampsRequest,
	})

	assert.NoError(t, err, "error should be nil")

	wg.Wait()

	slog.Info("all events received successfully")

}

func TestGateway_Increase(t *testing.T) {

	writeInterval := int64(1)
	maxFileSize := int64(65536)

	swampPattern := name.New().Sanctuary("dizzlets").Realm("*").Swamp("*")
	selectedClient := clientInterface.GetServiceClient(swampPattern)
	_, err := selectedClient.RegisterSwamp(context.Background(), &hydraidepbgo.RegisterSwampRequest{
		SwampPattern:   swampPattern.Get(),
		CloseAfterIdle: int64(3600),
		WriteInterval:  &writeInterval,
		MaxFileSize:    &maxFileSize,
	})

	swampName := name.New().Sanctuary("dizzlets").Realm("testing").Swamp("incrementInt16")
	swampClient := clientInterface.GetServiceClient(swampName)
	defer func() {
		_, err = swampClient.Destroy(context.Background(), &hydraidepbgo.DestroyRequest{
			SwampName: swampName.Get(),
		})
		assert.NoError(t, err)
	}()

	key := "increment-key"
	// try to set a value to the swamp
	response, err := swampClient.IncrementInt16(context.Background(), &hydraidepbgo.IncrementInt16Request{
		SwampName:   swampName.Get(),
		Key:         key,
		IncrementBy: 1,
		Condition:   nil,
		SetIfNotExist: &hydraidepbgo.IncrementRequestMetadata{
			ExpiredAt: timestamppb.New(time.Now().UTC().Add(1 * time.Hour)),
		},
	})

	assert.NoError(t, err)
	assert.NotNil(t, response)

	slog.Debug("response from the server", "response", response)

	assert.Equal(t, int32(1), response.GetValue(), "the value should be 1")
	assert.Greater(t, response.GetMetadata().GetExpiredAt().AsTime().Unix(), time.Now().UTC().Add(30*time.Minute).Unix(), "the expiration time should be in the future")

}

func TestUint32Slice(t *testing.T) {

	writeInterval := int64(1)
	maxFileSize := int64(65536)
	testKey := "my-uint32-slice"

	swampName := name.New().Sanctuary("uint32slice").Realm("test").Swamp("all")
	selectedClient := clientInterface.GetServiceClient(swampName)
	_, err := selectedClient.RegisterSwamp(context.Background(), &hydraidepbgo.RegisterSwampRequest{
		SwampPattern:   swampName.Get(),
		CloseAfterIdle: int64(3600),
		WriteInterval:  &writeInterval,
		MaxFileSize:    &maxFileSize,
	})
	assert.NoError(t, err)

	swampClient := clientInterface.GetServiceClient(swampName)
	defer func() {
		_, err = swampClient.Destroy(context.Background(), &hydraidepbgo.DestroyRequest{
			SwampName: swampName.Get(),
		})
		assert.NoError(t, err)
	}()

	ksp := &hydraidepbgo.KeySlicePair{
		Key:    testKey,
		Values: []uint32{1, 2, 3, 4, 5},
	}

	request := &hydraidepbgo.AddToUint32SlicePushRequest{
		SwampName: swampName.Get(),
		KeySlicePairs: []*hydraidepbgo.KeySlicePair{
			ksp,
		},
	}

	response, err := swampClient.Uint32SlicePush(context.Background(), request)
	assert.NoError(t, err, "error should be nil")
	assert.NotNil(t, response, "response should not be nil")

	// check if there is a key in the swamp
	getResponse, err := swampClient.Get(context.Background(), &hydraidepbgo.GetRequest{
		Swamps: []*hydraidepbgo.GetSwamp{
			{
				SwampName: swampName.Get(),
				Keys:      []string{ksp.GetKey()},
			},
		},
	})

	assert.NoError(t, err, "error should be nil")
	assert.NotNil(t, getResponse, "response should not be nil")
	assert.Equal(t, 1, len(getResponse.GetSwamps()), "response should contain one swamp")
	// check the treasure in the getResponse
	assert.Equal(t, 1, len(getResponse.GetSwamps()[0].GetTreasures()), "the swamp should contain one treasure")

	for _, treasure := range getResponse.GetSwamps()[0].GetTreasures() {
		assert.Equal(t, ksp.GetKey(), treasure.GetKey(), "the key should be the same")
		assert.Equal(t, ksp.GetValues(), treasure.GetUint32Slice(), "the values should be the same")
		fmt.Println("Key: ", treasure.GetKey())
		fmt.Println("Slice: ", treasure.GetUint32Slice())
	}

	// try to add new values to the slice
	kspNewValues := &hydraidepbgo.KeySlicePair{
		Key:    testKey,
		Values: []uint32{1, 2, 6, 7, 8, 9, 10},
	}

	request = &hydraidepbgo.AddToUint32SlicePushRequest{
		SwampName: swampName.Get(),
		KeySlicePairs: []*hydraidepbgo.KeySlicePair{
			kspNewValues,
		},
	}

	response, err = swampClient.Uint32SlicePush(context.Background(), request)
	assert.NoError(t, err, "error should be nil")
	assert.NotNil(t, response, "response should not be nil")

	// check if there is a key in the swamp
	getResponse, err = swampClient.Get(context.Background(), &hydraidepbgo.GetRequest{
		Swamps: []*hydraidepbgo.GetSwamp{
			{
				SwampName: swampName.Get(),
				Keys:      []string{kspNewValues.GetKey()},
			},
		},
	})

	assert.NoError(t, err, "error should be nil")
	assert.NotNil(t, getResponse, "response should not be nil")
	assert.Equal(t, 1, len(getResponse.GetSwamps()), "response should contain one swamp")

	// check the treasure in the getResponse
	assert.Equal(t, 1, len(getResponse.GetSwamps()[0].GetTreasures()), "the swamp should contain one treasure")
	for _, treasure := range getResponse.GetSwamps()[0].GetTreasures() {
		assert.Equal(t, kspNewValues.GetKey(), treasure.GetKey(), "the key should be the same")
		assert.Equal(t, []uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, treasure.GetUint32Slice(), "the values should be the same")
		fmt.Println("Key: ", treasure.GetKey())
		fmt.Println("Slice: ", treasure.GetUint32Slice())
	}

	// try top delete some values from the slice
	kspValuesToDelete := &hydraidepbgo.KeySlicePair{
		Key:    testKey,
		Values: []uint32{1, 2, 3},
	}

	requestDelete := &hydraidepbgo.Uint32SliceDeleteRequest{
		SwampName: swampName.Get(),
		KeySlicePairs: []*hydraidepbgo.KeySlicePair{
			kspValuesToDelete,
		},
	}

	_, err = swampClient.Uint32SliceDelete(context.Background(), requestDelete)
	assert.NoError(t, err, "error should be nil")

	// try to get the key again
	getResponse, err = swampClient.Get(context.Background(), &hydraidepbgo.GetRequest{
		Swamps: []*hydraidepbgo.GetSwamp{
			{
				SwampName: swampName.Get(),
				Keys:      []string{kspValuesToDelete.GetKey()},
			},
		},
	})

	assert.NoError(t, err, "error should be nil")
	assert.NotNil(t, getResponse, "response should not be nil")
	assert.Equal(t, 1, len(getResponse.GetSwamps()), "response should contain one swamp")

	// check the treasure in the getResponse
	assert.Equal(t, 1, len(getResponse.GetSwamps()[0].GetTreasures()), "the swamp should contain one treasure")
	for _, treasure := range getResponse.GetSwamps()[0].GetTreasures() {
		assert.Equal(t, kspValuesToDelete.GetKey(), treasure.GetKey(), "the key should be the same")
		assert.Equal(t, []uint32{4, 5, 6, 7, 8, 9, 10}, treasure.GetUint32Slice(), "the values should be the same")
		fmt.Println("Key: ", treasure.GetKey())
		fmt.Println("Slice: ", treasure.GetUint32Slice())
	}

	// check if the value is exist in the slice
	isValueExistResponse, err := swampClient.Uint32SliceIsValueExist(context.Background(), &hydraidepbgo.Uint32SliceIsValueExistRequest{
		SwampName: swampName.Get(),
		Key:       testKey,
		Value:     10,
	})
	assert.NoError(t, err, "error should be nil")
	assert.True(t, isValueExistResponse.IsExist, "value 10 should exist in the slice")

	// check if the value is not exist in the slice
	isValueExistResponse, err = swampClient.Uint32SliceIsValueExist(context.Background(), &hydraidepbgo.Uint32SliceIsValueExistRequest{
		SwampName: swampName.Get(),
		Key:       testKey,
		Value:     3,
	})
	assert.NoError(t, err, "error should be nil")
	assert.False(t, isValueExistResponse.IsExist, "value 3 should not exist in the slice")

	// check the length of the slice
	sliceLengthResponse, err := swampClient.Uint32SliceSize(context.Background(), &hydraidepbgo.Uint32SliceSizeRequest{
		SwampName: swampName.Get(),
		Key:       testKey,
	})
	assert.NoError(t, err, "error should be nil")
	assert.Equal(t, 7, int(sliceLengthResponse.Size), "the length of the slice should be 7")

}

func TestDeleteFromCatalog(t *testing.T) {

	writeInterval := int64(1)
	closeAfterIdle := int64(1)
	maxFileSize := int64(65536)

	swampName := name.New().Sanctuary("testDelete").Realm("test").Swamp("delete")
	selectedClient := clientInterface.GetServiceClient(swampName)
	_, err := selectedClient.RegisterSwamp(context.Background(), &hydraidepbgo.RegisterSwampRequest{
		SwampPattern:   swampName.Get(),
		CloseAfterIdle: closeAfterIdle,
		WriteInterval:  &writeInterval,
		MaxFileSize:    &maxFileSize,
	})

	assert.NoError(t, err, "there should be no error while registering the swamp")

	defer func() {
		_, err = selectedClient.Destroy(context.Background(), &hydraidepbgo.DestroyRequest{
			SwampName: swampName.Get(),
		})
		assert.NoError(t, err)
	}()

	stringVal := "myValue"

	// try to add 2 keys to the swamp
	keyValues := []*hydraidepbgo.KeyValuePair{
		{
			Key:       "key1",
			StringVal: &stringVal,
		},
		{
			Key:       "key2",
			StringVal: &stringVal,
		},
		{
			Key:       "key3",
			StringVal: &stringVal,
		},
	}

	_, err = selectedClient.Set(context.Background(), &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				SwampName:        swampName.Get(),
				KeyValues:        keyValues,
				CreateIfNotExist: true,
				Overwrite:        true,
			},
		},
	})
	assert.NoError(t, err)

	// wait a short time to let the swamp be closed due to idle time
	time.Sleep(3 * time.Second)

	// check how many keys are in the swamp with count
	countResponse, err := selectedClient.Count(context.Background(), &hydraidepbgo.CountRequest{
		Swamps: []*hydraidepbgo.CountRequest_SwampIdentifier{
			{
				SwampName: swampName.Get(),
			},
		},
	})

	assert.NoError(t, err)
	assert.Equal(t, int32(len(keyValues)), countResponse.Swamps[0].Count, "the swamp should contain 2 keys")

	// wait a short time to let the swamp be closed due to idle time
	time.Sleep(3 * time.Second)

	// try to delete 1 key from the swamp
	_, err = selectedClient.Delete(context.Background(), &hydraidepbgo.DeleteRequest{
		Swamps: []*hydraidepbgo.DeleteRequest_SwampKeys{
			{
				SwampName: swampName.Get(),
				Keys:      []string{"key1"},
			},
		},
	})

	assert.NoError(t, err, "there should be no error while deleting a key")

	// wait a short time to let the delete operation be processed and written to the catalog
	time.Sleep(3 * time.Second)

	// check if the delete operation was successful
	countResponse2, err2 := selectedClient.Count(context.Background(), &hydraidepbgo.CountRequest{
		Swamps: []*hydraidepbgo.CountRequest_SwampIdentifier{
			{
				SwampName: swampName.Get(),
			},
		},
	})

	assert.NoError(t, err2, "there should be no error while counting keys after deletion")
	assert.Equal(t, int32(2), countResponse2.Swamps[0].Count, "the swamp should contain 1 key after deletion")

}

func destroySwamp(selectedClient hydraidepbgo.HydraideServiceClient, swampName name.Name) {

	_, err := selectedClient.Destroy(context.Background(), &hydraidepbgo.DestroyRequest{
		SwampName: swampName.Get(),
	})

	if err != nil {
		slog.Error("error while destroying swamp", "swamp", swampName.Get(), "error", err)
		return
	}

}

func TestShiftByKeys(t *testing.T) {

	writeInterval := int64(1)
	maxFileSize := int64(65536)

	swampPattern := name.New().Sanctuary("shifttest").Realm("*").Swamp("*")
	selectedClient := clientInterface.GetServiceClient(swampPattern)
	_, err := selectedClient.RegisterSwamp(context.Background(), &hydraidepbgo.RegisterSwampRequest{
		SwampPattern:   swampPattern.Get(),
		CloseAfterIdle: int64(3600),
		WriteInterval:  &writeInterval,
		MaxFileSize:    &maxFileSize,
	})

	swampName := name.New().Sanctuary("shifttest").Realm("batch").Swamp("shift-by-keys")
	swampClient := clientInterface.GetServiceClient(swampName)
	defer func() {
		_, err = swampClient.Destroy(context.Background(), &hydraidepbgo.DestroyRequest{
			SwampName: swampName.Get(),
		})
		assert.NoError(t, err)
	}()

	// Create 10 treasures
	var keyValues []*hydraidepbgo.KeyValuePair
	for i := 0; i < 10; i++ {
		myVal := fmt.Sprintf("value-%d", i)
		createdBy := "test-user"
		keyValues = append(keyValues, &hydraidepbgo.KeyValuePair{
			Key:       fmt.Sprintf("item-%d", i),
			StringVal: &myVal,
			CreatedAt: timestamppb.Now(),
			CreatedBy: &createdBy,
			UpdatedAt: timestamppb.Now(),
			UpdatedBy: &createdBy,
		})
	}

	// Set the treasures
	setResponse, err := swampClient.Set(context.Background(), &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				SwampName:        swampName.Get(),
				CreateIfNotExist: true,
				Overwrite:        true,
				KeyValues:        keyValues,
			},
		}})

	assert.NoError(t, err)
	assert.NotNil(t, setResponse)
	assert.Equal(t, 1, len(setResponse.GetSwamps()), "response should contain one swamp")
	assert.Equal(t, 10, len(setResponse.GetSwamps()[0].GetKeysAndStatuses()), "the swamp should contain 10 keys")

	slog.Info("Set 10 treasures to swamp", "swamp", swampName.Get())

	// Now shift (clone and delete) specific keys
	keysToShift := []string{"item-1", "item-3", "item-5", "item-7", "item-9"}
	shiftResponse, err := swampClient.ShiftByKeys(context.Background(), &hydraidepbgo.ShiftByKeysRequest{
		SwampName: swampName.Get(),
		Keys:      keysToShift,
	})

	assert.NoError(t, err)
	assert.NotNil(t, shiftResponse)
	assert.Equal(t, 5, len(shiftResponse.GetTreasures()), "should return 5 shifted treasures")

	// Verify the shifted treasures have correct data
	shiftedKeys := make(map[string]bool)
	for _, treasure := range shiftResponse.GetTreasures() {
		shiftedKeys[treasure.GetKey()] = true
		assert.NotNil(t, treasure.GetStringVal(), "treasure should have string value")
		slog.Debug("Shifted treasure", "key", treasure.GetKey(), "value", treasure.GetStringVal())
	}

	// Verify all expected keys were shifted
	for _, key := range keysToShift {
		assert.True(t, shiftedKeys[key], "key %s should be in shifted treasures", key)
	}

	// Verify the shifted keys are deleted from the swamp
	getResponse, err := swampClient.Get(context.Background(), &hydraidepbgo.GetRequest{
		Swamps: []*hydraidepbgo.GetSwamp{
			{
				SwampName: swampName.Get(),
				Keys:      keysToShift,
			},
		},
	})

	assert.NoError(t, err)
	assert.NotNil(t, getResponse)

	// Count existing treasures in the response
	existingCount := 0
	for _, treasure := range getResponse.GetSwamps()[0].GetTreasures() {
		if treasure.IsExist {
			existingCount++
		}
	}
	assert.Equal(t, 0, existingCount, "shifted keys should be deleted from swamp")

	slog.Info("Verified shifted keys are deleted from swamp")

	// Verify the remaining keys still exist
	remainingKeys := []string{"item-0", "item-2", "item-4", "item-6", "item-8"}
	getResponse2, err := swampClient.Get(context.Background(), &hydraidepbgo.GetRequest{
		Swamps: []*hydraidepbgo.GetSwamp{
			{
				SwampName: swampName.Get(),
				Keys:      remainingKeys,
			},
		},
	})

	assert.NoError(t, err)
	assert.NotNil(t, getResponse2)

	// Count existing treasures in the response
	existingCount2 := 0
	for _, treasure := range getResponse2.GetSwamps()[0].GetTreasures() {
		if treasure.IsExist {
			existingCount2++
		}
	}
	assert.Equal(t, 5, existingCount2, "remaining keys should still exist")

	slog.Info("Verified remaining keys still exist in swamp", "count", existingCount2)

	// Test edge case: shift non-existent keys
	nonExistentKeys := []string{"item-99", "item-100"}
	shiftResponse2, err := swampClient.ShiftByKeys(context.Background(), &hydraidepbgo.ShiftByKeysRequest{
		SwampName: swampName.Get(),
		Keys:      nonExistentKeys,
	})

	assert.NoError(t, err)
	assert.NotNil(t, shiftResponse2)
	assert.Equal(t, 0, len(shiftResponse2.GetTreasures()), "should return empty array for non-existent keys")

	slog.Info("Verified non-existent keys return empty result")

	// Test edge case: empty keys array
	shiftResponse3, err := swampClient.ShiftByKeys(context.Background(), &hydraidepbgo.ShiftByKeysRequest{
		SwampName: swampName.Get(),
		Keys:      []string{},
	})

	assert.NoError(t, err)
	assert.NotNil(t, shiftResponse3)
	assert.Equal(t, 0, len(shiftResponse3.GetTreasures()), "should return empty array for empty keys")

	slog.Info("Test completed successfully")
}

// =============================================================================
// V2 ENGINE FILE PERSISTENCE TESTS
// =============================================================================
// These tests verify that the V2 append-only storage engine correctly:
// 1. Creates .hyd files on disk
// 2. Persists data through swamp close/reopen cycles
// 3. Handles insert, update, and delete operations with file persistence

// TestV2Engine_InsertAndReload tests that inserted data is persisted to disk
// and can be read back after the swamp is closed and reopened.
func TestV2Engine_InsertAndReload(t *testing.T) {
	writeInterval := int64(1)
	closeAfterIdle := int64(1)
	maxFileSize := int64(65536)

	swampName := name.New().Sanctuary("v2test").Realm("insert").Swamp("reload")
	selectedClient := clientInterface.GetServiceClient(swampName)

	// Register the swamp
	_, err := selectedClient.RegisterSwamp(context.Background(), &hydraidepbgo.RegisterSwampRequest{
		SwampPattern:   swampName.Get(),
		CloseAfterIdle: closeAfterIdle,
		WriteInterval:  &writeInterval,
		MaxFileSize:    &maxFileSize,
	})
	assert.NoError(t, err, "should register swamp without error")

	defer func() {
		_, _ = selectedClient.Destroy(context.Background(), &hydraidepbgo.DestroyRequest{
			SwampName: swampName.Get(),
		})
	}()

	// Step 1: Insert multiple keys with different value types
	slog.Info("Step 1: Inserting test data...")

	stringVal := "test-string-value"
	int64Val := int64(123456789)
	float64Val := float64(3.14159)
	boolVal := hydraidepbgo.Boolean_TRUE
	bytesVal := []byte("binary-data-content")

	keyValues := []*hydraidepbgo.KeyValuePair{
		{Key: "string-key", StringVal: &stringVal},
		{Key: "int64-key", Int64Val: &int64Val},
		{Key: "float64-key", Float64Val: &float64Val},
		{Key: "bool-key", BoolVal: &boolVal},
		{Key: "bytes-key", BytesVal: bytesVal},
	}

	_, err = selectedClient.Set(context.Background(), &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				SwampName:        swampName.Get(),
				KeyValues:        keyValues,
				CreateIfNotExist: true,
				Overwrite:        true,
			},
		},
	})
	assert.NoError(t, err, "should insert data without error")

	// Step 2: Wait for swamp to close (flush to disk) and memory to be freed
	slog.Info("Step 2: Waiting for swamp to close and flush to disk...")
	time.Sleep(4 * time.Second) // closeAfterIdle=1s + writeInterval=1s + buffer

	// Step 3: Reopen the swamp and verify data is loaded from .hyd file
	slog.Info("Step 3: Reopening swamp and verifying data from disk...")

	// Count should return 5 keys
	countResponse, err := selectedClient.Count(context.Background(), &hydraidepbgo.CountRequest{
		Swamps: []*hydraidepbgo.CountRequest_SwampIdentifier{
			{SwampName: swampName.Get()},
		},
	})
	assert.NoError(t, err, "should count without error")
	assert.Equal(t, int32(5), countResponse.Swamps[0].Count, "should have 5 keys after reload")

	// Step 4: Verify each value is correctly persisted
	slog.Info("Step 4: Verifying individual values...")

	getResponse, err := selectedClient.Get(context.Background(), &hydraidepbgo.GetRequest{
		Swamps: []*hydraidepbgo.GetSwamp{
			{
				SwampName: swampName.Get(),
				Keys:      []string{"string-key", "int64-key", "float64-key", "bool-key", "bytes-key"},
			},
		},
	})
	assert.NoError(t, err, "should get values without error")
	assert.Len(t, getResponse.Swamps, 1, "should have 1 swamp response")
	assert.Len(t, getResponse.Swamps[0].Treasures, 5, "should have 5 treasures")

	// Verify values
	treasureMap := make(map[string]*hydraidepbgo.Treasure)
	for _, treasure := range getResponse.Swamps[0].Treasures {
		treasureMap[treasure.Key] = treasure
	}

	assert.Equal(t, stringVal, treasureMap["string-key"].GetStringVal(), "string value should match")
	assert.Equal(t, int64Val, treasureMap["int64-key"].GetInt64Val(), "int64 value should match")
	assert.Equal(t, float64Val, treasureMap["float64-key"].GetFloat64Val(), "float64 value should match")
	assert.Equal(t, boolVal, treasureMap["bool-key"].GetBoolVal(), "bool value should match")
	assert.Equal(t, bytesVal, treasureMap["bytes-key"].GetBytesVal(), "bytes value should match")

	slog.Info("TestV2Engine_InsertAndReload completed successfully")
}

// TestV2Engine_UpdateAndReload tests that updated data is persisted to disk
// and the latest version is read back after the swamp is closed and reopened.
func TestV2Engine_UpdateAndReload(t *testing.T) {
	writeInterval := int64(1)
	closeAfterIdle := int64(1)
	maxFileSize := int64(65536)

	swampName := name.New().Sanctuary("v2test").Realm("update").Swamp("reload")
	selectedClient := clientInterface.GetServiceClient(swampName)

	// Register the swamp
	_, err := selectedClient.RegisterSwamp(context.Background(), &hydraidepbgo.RegisterSwampRequest{
		SwampPattern:   swampName.Get(),
		CloseAfterIdle: closeAfterIdle,
		WriteInterval:  &writeInterval,
		MaxFileSize:    &maxFileSize,
	})
	assert.NoError(t, err, "should register swamp without error")

	defer func() {
		_, _ = selectedClient.Destroy(context.Background(), &hydraidepbgo.DestroyRequest{
			SwampName: swampName.Get(),
		})
	}()

	// Step 1: Insert initial data
	slog.Info("Step 1: Inserting initial data...")

	initialValue := "initial-value"
	_, err = selectedClient.Set(context.Background(), &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				SwampName: swampName.Get(),
				KeyValues: []*hydraidepbgo.KeyValuePair{
					{Key: "update-key", StringVal: &initialValue},
				},
				CreateIfNotExist: true,
				Overwrite:        true,
			},
		},
	})
	assert.NoError(t, err, "should insert initial data without error")

	// Step 2: Wait for flush and swamp close
	slog.Info("Step 2: Waiting for first flush...")
	time.Sleep(4 * time.Second)

	// Step 3: Update the value
	slog.Info("Step 3: Updating the value...")

	updatedValue := "updated-value-v2"
	_, err = selectedClient.Set(context.Background(), &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				SwampName: swampName.Get(),
				KeyValues: []*hydraidepbgo.KeyValuePair{
					{Key: "update-key", StringVal: &updatedValue},
				},
				CreateIfNotExist: false,
				Overwrite:        true,
			},
		},
	})
	assert.NoError(t, err, "should update data without error")

	// Step 4: Wait for flush and swamp close again
	slog.Info("Step 4: Waiting for second flush...")
	time.Sleep(4 * time.Second)

	// Step 5: Reopen and verify the updated value is persisted
	slog.Info("Step 5: Reopening swamp and verifying updated data...")

	getResponse, err := selectedClient.Get(context.Background(), &hydraidepbgo.GetRequest{
		Swamps: []*hydraidepbgo.GetSwamp{
			{
				SwampName: swampName.Get(),
				Keys:      []string{"update-key"},
			},
		},
	})
	assert.NoError(t, err, "should get updated value without error")
	assert.Len(t, getResponse.Swamps, 1, "should have 1 swamp response")
	assert.Len(t, getResponse.Swamps[0].Treasures, 1, "should have 1 treasure")
	assert.Equal(t, updatedValue, getResponse.Swamps[0].Treasures[0].GetStringVal(),
		"should have the updated value, not the initial value")

	// Step 6: Update multiple times and verify only latest is returned
	slog.Info("Step 6: Multiple updates test...")

	for i := 1; i <= 5; i++ {
		updateVal := fmt.Sprintf("version-%d", i)
		_, err = selectedClient.Set(context.Background(), &hydraidepbgo.SetRequest{
			Swamps: []*hydraidepbgo.SwampRequest{
				{
					SwampName: swampName.Get(),
					KeyValues: []*hydraidepbgo.KeyValuePair{
						{Key: "multi-update-key", StringVal: &updateVal},
					},
					CreateIfNotExist: true,
					Overwrite:        true,
				},
			},
		})
		assert.NoError(t, err, "should update without error")
	}

	// Wait for flush
	time.Sleep(4 * time.Second)

	// Verify only the latest version is returned
	getResponse2, err := selectedClient.Get(context.Background(), &hydraidepbgo.GetRequest{
		Swamps: []*hydraidepbgo.GetSwamp{
			{
				SwampName: swampName.Get(),
				Keys:      []string{"multi-update-key"},
			},
		},
	})
	assert.NoError(t, err, "should get latest value without error")
	assert.Equal(t, "version-5", getResponse2.Swamps[0].Treasures[0].GetStringVal(),
		"should have the latest version (version-5)")

	slog.Info("TestV2Engine_UpdateAndReload completed successfully")
}

// TestV2Engine_DeleteAndReload tests that deleted data is properly removed
// from the .hyd file and not returned after swamp close/reopen.
func TestV2Engine_DeleteAndReload(t *testing.T) {
	writeInterval := int64(1)
	closeAfterIdle := int64(1)
	maxFileSize := int64(65536)

	swampName := name.New().Sanctuary("v2test").Realm("delete").Swamp("reload")
	selectedClient := clientInterface.GetServiceClient(swampName)

	// Register the swamp
	_, err := selectedClient.RegisterSwamp(context.Background(), &hydraidepbgo.RegisterSwampRequest{
		SwampPattern:   swampName.Get(),
		CloseAfterIdle: closeAfterIdle,
		WriteInterval:  &writeInterval,
		MaxFileSize:    &maxFileSize,
	})
	assert.NoError(t, err, "should register swamp without error")

	defer func() {
		_, _ = selectedClient.Destroy(context.Background(), &hydraidepbgo.DestroyRequest{
			SwampName: swampName.Get(),
		})
	}()

	// Step 1: Insert 5 keys
	slog.Info("Step 1: Inserting 5 test keys...")

	keyValues := make([]*hydraidepbgo.KeyValuePair, 5)
	for i := 0; i < 5; i++ {
		val := fmt.Sprintf("value-%d", i)
		keyValues[i] = &hydraidepbgo.KeyValuePair{
			Key:       fmt.Sprintf("key-%d", i),
			StringVal: &val,
		}
	}

	_, err = selectedClient.Set(context.Background(), &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				SwampName:        swampName.Get(),
				KeyValues:        keyValues,
				CreateIfNotExist: true,
				Overwrite:        true,
			},
		},
	})
	assert.NoError(t, err, "should insert data without error")

	// Step 2: Wait for flush
	slog.Info("Step 2: Waiting for flush...")
	time.Sleep(4 * time.Second)

	// Step 3: Verify swamp exists
	slog.Info("Step 3: Verifying swamp exists...")

	existResponse, err := selectedClient.IsSwampExist(context.Background(), &hydraidepbgo.IsSwampExistRequest{
		SwampName: swampName.Get(),
	})
	assert.NoError(t, err, "should check existence without error")
	assert.True(t, existResponse.IsExist, "swamp should exist after insert")

	countResponse, err := selectedClient.Count(context.Background(), &hydraidepbgo.CountRequest{
		Swamps: []*hydraidepbgo.CountRequest_SwampIdentifier{
			{SwampName: swampName.Get()},
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, int32(5), countResponse.Swamps[0].Count, "should have 5 keys")

	// Step 4: Delete 2 keys (key-1 and key-3)
	slog.Info("Step 4: Deleting key-1 and key-3...")

	_, err = selectedClient.Delete(context.Background(), &hydraidepbgo.DeleteRequest{
		Swamps: []*hydraidepbgo.DeleteRequest_SwampKeys{
			{
				SwampName: swampName.Get(),
				Keys:      []string{"key-1", "key-3"},
			},
		},
	})
	assert.NoError(t, err, "should delete without error")

	// Step 5: Wait for flush and swamp close
	slog.Info("Step 5: Waiting for delete to be flushed...")
	time.Sleep(4 * time.Second)

	// Step 6: Reopen and verify deleted keys are gone
	slog.Info("Step 6: Reopening swamp and verifying deletions...")

	countResponse2, err := selectedClient.Count(context.Background(), &hydraidepbgo.CountRequest{
		Swamps: []*hydraidepbgo.CountRequest_SwampIdentifier{
			{SwampName: swampName.Get()},
		},
	})
	assert.NoError(t, err, "should count after delete without error")
	assert.Equal(t, int32(3), countResponse2.Swamps[0].Count, "should have 3 keys after deleting 2")

	// Step 7: Verify the remaining keys are correct (key-0, key-2, key-4)
	slog.Info("Step 7: Verifying remaining keys...")

	getResponse, err := selectedClient.Get(context.Background(), &hydraidepbgo.GetRequest{
		Swamps: []*hydraidepbgo.GetSwamp{
			{
				SwampName: swampName.Get(),
				Keys:      []string{"key-0", "key-1", "key-2", "key-3", "key-4"},
			},
		},
	})
	assert.NoError(t, err, "should get remaining keys without error")

	// Count how many were found (only count existing treasures)
	foundKeys := make(map[string]bool)
	for _, treasure := range getResponse.Swamps[0].Treasures {
		if treasure.IsExist {
			foundKeys[treasure.Key] = true
		}
	}

	assert.True(t, foundKeys["key-0"], "key-0 should exist")
	assert.False(t, foundKeys["key-1"], "key-1 should be deleted")
	assert.True(t, foundKeys["key-2"], "key-2 should exist")
	assert.False(t, foundKeys["key-3"], "key-3 should be deleted")
	assert.True(t, foundKeys["key-4"], "key-4 should exist")

	// Step 8: Test insert-delete-insert cycle
	slog.Info("Step 8: Testing insert-delete-insert cycle...")

	// Insert a new key
	cycleVal1 := "cycle-value-1"
	_, err = selectedClient.Set(context.Background(), &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				SwampName: swampName.Get(),
				KeyValues: []*hydraidepbgo.KeyValuePair{
					{Key: "cycle-key", StringVal: &cycleVal1},
				},
				CreateIfNotExist: true,
				Overwrite:        true,
			},
		},
	})
	assert.NoError(t, err)
	time.Sleep(4 * time.Second)

	// Delete it
	_, err = selectedClient.Delete(context.Background(), &hydraidepbgo.DeleteRequest{
		Swamps: []*hydraidepbgo.DeleteRequest_SwampKeys{
			{
				SwampName: swampName.Get(),
				Keys:      []string{"cycle-key"},
			},
		},
	})
	assert.NoError(t, err)
	time.Sleep(4 * time.Second)

	// Re-insert with a different value
	cycleVal2 := "cycle-value-2-reinserted"
	_, err = selectedClient.Set(context.Background(), &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				SwampName: swampName.Get(),
				KeyValues: []*hydraidepbgo.KeyValuePair{
					{Key: "cycle-key", StringVal: &cycleVal2},
				},
				CreateIfNotExist: true,
				Overwrite:        true,
			},
		},
	})
	assert.NoError(t, err)
	time.Sleep(4 * time.Second)

	// Verify the re-inserted value
	getResponse2, err := selectedClient.Get(context.Background(), &hydraidepbgo.GetRequest{
		Swamps: []*hydraidepbgo.GetSwamp{
			{
				SwampName: swampName.Get(),
				Keys:      []string{"cycle-key"},
			},
		},
	})
	assert.NoError(t, err, "should get re-inserted key without error")
	assert.Len(t, getResponse2.Swamps[0].Treasures, 1, "should have 1 treasure")
	assert.Equal(t, cycleVal2, getResponse2.Swamps[0].Treasures[0].GetStringVal(),
		"should have the re-inserted value")

	slog.Info("TestV2Engine_DeleteAndReload completed successfully")
}

// TestV2Engine_ShiftByKeysAndReload tests that ShiftByKeys correctly removes data
// and the changes are persisted after swamp close/reopen.
func TestV2Engine_ShiftByKeysAndReload(t *testing.T) {
	writeInterval := int64(1)
	closeAfterIdle := int64(1)
	maxFileSize := int64(65536)

	swampName := name.New().Sanctuary("v2test").Realm("shift").Swamp("bykeys")
	selectedClient := clientInterface.GetServiceClient(swampName)

	// Register the swamp
	_, err := selectedClient.RegisterSwamp(context.Background(), &hydraidepbgo.RegisterSwampRequest{
		SwampPattern:   swampName.Get(),
		CloseAfterIdle: closeAfterIdle,
		WriteInterval:  &writeInterval,
		MaxFileSize:    &maxFileSize,
	})
	assert.NoError(t, err, "should register swamp without error")

	defer func() {
		_, _ = selectedClient.Destroy(context.Background(), &hydraidepbgo.DestroyRequest{
			SwampName: swampName.Get(),
		})
	}()

	// Step 1: Insert 10 keys
	slog.Info("Step 1: Inserting 10 test keys...")

	keyValues := make([]*hydraidepbgo.KeyValuePair, 10)
	for i := 0; i < 10; i++ {
		val := fmt.Sprintf("value-%d", i)
		keyValues[i] = &hydraidepbgo.KeyValuePair{
			Key:       fmt.Sprintf("item-%d", i),
			StringVal: &val,
		}
	}

	_, err = selectedClient.Set(context.Background(), &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				SwampName:        swampName.Get(),
				KeyValues:        keyValues,
				CreateIfNotExist: true,
				Overwrite:        true,
			},
		},
	})
	assert.NoError(t, err, "should insert data without error")

	// Step 2: Wait for flush
	slog.Info("Step 2: Waiting for flush...")
	time.Sleep(4 * time.Second)

	// Step 3: Shift 5 keys (item-1, item-3, item-5, item-7, item-9)
	slog.Info("Step 3: Shifting 5 keys...")

	keysToShift := []string{"item-1", "item-3", "item-5", "item-7", "item-9"}
	shiftResponse, err := selectedClient.ShiftByKeys(context.Background(), &hydraidepbgo.ShiftByKeysRequest{
		SwampName: swampName.Get(),
		Keys:      keysToShift,
	})
	assert.NoError(t, err, "should shift without error")
	assert.Len(t, shiftResponse.Treasures, 5, "should return 5 shifted treasures")

	// Step 4: Wait for flush and swamp close
	slog.Info("Step 4: Waiting for shift to be flushed...")
	time.Sleep(4 * time.Second)

	// Step 5: Reopen and verify shifted keys are gone
	slog.Info("Step 5: Reopening swamp and verifying shifts persisted...")

	countResponse, err := selectedClient.Count(context.Background(), &hydraidepbgo.CountRequest{
		Swamps: []*hydraidepbgo.CountRequest_SwampIdentifier{
			{SwampName: swampName.Get()},
		},
	})
	assert.NoError(t, err, "should count after shift without error")
	assert.Equal(t, int32(5), countResponse.Swamps[0].Count, "should have 5 keys after shifting 5")

	// Step 6: Verify the remaining keys are correct (item-0, item-2, item-4, item-6, item-8)
	slog.Info("Step 6: Verifying remaining keys...")

	getResponse, err := selectedClient.Get(context.Background(), &hydraidepbgo.GetRequest{
		Swamps: []*hydraidepbgo.GetSwamp{
			{
				SwampName: swampName.Get(),
				Keys:      []string{"item-0", "item-2", "item-4", "item-6", "item-8"},
			},
		},
	})
	assert.NoError(t, err, "should get remaining keys without error")

	existCount := 0
	for _, treasure := range getResponse.Swamps[0].Treasures {
		if treasure.IsExist {
			existCount++
		}
	}
	assert.Equal(t, 5, existCount, "all 5 remaining keys should exist")

	slog.Info("TestV2Engine_ShiftByKeysAndReload completed successfully")
}

// TestV2Engine_ShiftExpiredAndReload tests that ShiftExpiredTreasures correctly removes
// expired data and the changes are persisted after swamp close/reopen.
func TestV2Engine_ShiftExpiredAndReload(t *testing.T) {
	writeInterval := int64(1)
	closeAfterIdle := int64(5) // Longer idle time to keep swamp open for expiration
	maxFileSize := int64(65536)

	swampName := name.New().Sanctuary("v2test").Realm("shift").Swamp("expired")
	selectedClient := clientInterface.GetServiceClient(swampName)

	// Register the swamp
	_, err := selectedClient.RegisterSwamp(context.Background(), &hydraidepbgo.RegisterSwampRequest{
		SwampPattern:   swampName.Get(),
		CloseAfterIdle: closeAfterIdle,
		WriteInterval:  &writeInterval,
		MaxFileSize:    &maxFileSize,
	})
	assert.NoError(t, err, "should register swamp without error")

	defer func() {
		_, _ = selectedClient.Destroy(context.Background(), &hydraidepbgo.DestroyRequest{
			SwampName: swampName.Get(),
		})
	}()

	// Step 1: Insert keys - some will expire soon, some won't
	slog.Info("Step 1: Inserting keys with different expiration times...")

	now := time.Now().UTC()
	expireSoon := now.Add(2 * time.Second) // Expires in 2 seconds
	expireLater := now.Add(1 * time.Hour)  // Expires in 1 hour

	keyValues := []*hydraidepbgo.KeyValuePair{}

	// 3 keys that expire soon
	for i := 0; i < 3; i++ {
		val := fmt.Sprintf("expire-soon-%d", i)
		keyValues = append(keyValues, &hydraidepbgo.KeyValuePair{
			Key:       fmt.Sprintf("soon-%d", i),
			StringVal: &val,
			ExpiredAt: timestamppb.New(expireSoon),
		})
	}

	// 3 keys that expire later
	for i := 0; i < 3; i++ {
		val := fmt.Sprintf("expire-later-%d", i)
		keyValues = append(keyValues, &hydraidepbgo.KeyValuePair{
			Key:       fmt.Sprintf("later-%d", i),
			StringVal: &val,
			ExpiredAt: timestamppb.New(expireLater),
		})
	}

	_, err = selectedClient.Set(context.Background(), &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				SwampName:        swampName.Get(),
				KeyValues:        keyValues,
				CreateIfNotExist: true,
				Overwrite:        true,
			},
		},
	})
	assert.NoError(t, err, "should insert data without error")

	// Step 2: Verify 6 keys exist
	slog.Info("Step 2: Verifying 6 keys exist...")

	countResponse, err := selectedClient.Count(context.Background(), &hydraidepbgo.CountRequest{
		Swamps: []*hydraidepbgo.CountRequest_SwampIdentifier{
			{SwampName: swampName.Get()},
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, int32(6), countResponse.Swamps[0].Count, "should have 6 keys initially")

	// Step 3: Wait for expiration
	slog.Info("Step 3: Waiting for keys to expire...")
	time.Sleep(3 * time.Second)

	// Step 4: Shift expired treasures
	slog.Info("Step 4: Shifting expired treasures...")

	shiftResponse, err := selectedClient.ShiftExpiredTreasures(context.Background(), &hydraidepbgo.ShiftExpiredTreasuresRequest{
		SwampName: swampName.Get(),
		HowMany:   10, // Get up to 10 expired
	})
	assert.NoError(t, err, "should shift expired without error")
	assert.Len(t, shiftResponse.Treasures, 3, "should return 3 expired treasures")

	// Step 5: Wait for flush and close
	slog.Info("Step 5: Waiting for flush...")
	time.Sleep(6 * time.Second)

	// Step 6: Reopen and verify only non-expired keys remain
	slog.Info("Step 6: Reopening and verifying persistence...")

	countResponse2, err := selectedClient.Count(context.Background(), &hydraidepbgo.CountRequest{
		Swamps: []*hydraidepbgo.CountRequest_SwampIdentifier{
			{SwampName: swampName.Get()},
		},
	})
	assert.NoError(t, err, "should count after shift without error")
	assert.Equal(t, int32(3), countResponse2.Swamps[0].Count, "should have 3 keys after shifting expired")

	slog.Info("TestV2Engine_ShiftExpiredAndReload completed successfully")
}

// TestV2Engine_BatchSetAndReload tests batch insert/update operations
// and verifies all data persists correctly after swamp close/reopen.
func TestV2Engine_BatchSetAndReload(t *testing.T) {
	writeInterval := int64(1)
	closeAfterIdle := int64(1)
	maxFileSize := int64(65536)

	swampName := name.New().Sanctuary("v2test").Realm("batch").Swamp("set")
	selectedClient := clientInterface.GetServiceClient(swampName)

	// Register the swamp
	_, err := selectedClient.RegisterSwamp(context.Background(), &hydraidepbgo.RegisterSwampRequest{
		SwampPattern:   swampName.Get(),
		CloseAfterIdle: closeAfterIdle,
		WriteInterval:  &writeInterval,
		MaxFileSize:    &maxFileSize,
	})
	assert.NoError(t, err, "should register swamp without error")

	defer func() {
		_, _ = selectedClient.Destroy(context.Background(), &hydraidepbgo.DestroyRequest{
			SwampName: swampName.Get(),
		})
	}()

	// Step 1: Batch insert 100 keys
	slog.Info("Step 1: Batch inserting 100 keys...")

	keyValues := make([]*hydraidepbgo.KeyValuePair, 100)
	for i := 0; i < 100; i++ {
		val := fmt.Sprintf("batch-value-%d", i)
		keyValues[i] = &hydraidepbgo.KeyValuePair{
			Key:       fmt.Sprintf("batch-key-%d", i),
			StringVal: &val,
		}
	}

	_, err = selectedClient.Set(context.Background(), &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				SwampName:        swampName.Get(),
				KeyValues:        keyValues,
				CreateIfNotExist: true,
				Overwrite:        true,
			},
		},
	})
	assert.NoError(t, err, "should batch insert without error")

	// Step 2: Wait for flush
	slog.Info("Step 2: Waiting for flush...")
	time.Sleep(4 * time.Second)

	// Step 3: Reopen and verify all 100 keys exist
	slog.Info("Step 3: Reopening and verifying all 100 keys...")

	countResponse, err := selectedClient.Count(context.Background(), &hydraidepbgo.CountRequest{
		Swamps: []*hydraidepbgo.CountRequest_SwampIdentifier{
			{SwampName: swampName.Get()},
		},
	})
	assert.NoError(t, err, "should count without error")
	assert.Equal(t, int32(100), countResponse.Swamps[0].Count, "should have 100 keys after reload")

	// Step 4: Batch update 50 keys
	slog.Info("Step 4: Batch updating 50 keys...")

	updateKeyValues := make([]*hydraidepbgo.KeyValuePair, 50)
	for i := 0; i < 50; i++ {
		val := fmt.Sprintf("updated-value-%d", i)
		updateKeyValues[i] = &hydraidepbgo.KeyValuePair{
			Key:       fmt.Sprintf("batch-key-%d", i), // Update first 50 keys
			StringVal: &val,
		}
	}

	_, err = selectedClient.Set(context.Background(), &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				SwampName:        swampName.Get(),
				KeyValues:        updateKeyValues,
				CreateIfNotExist: false,
				Overwrite:        true,
			},
		},
	})
	assert.NoError(t, err, "should batch update without error")

	// Step 5: Wait for flush
	slog.Info("Step 5: Waiting for update flush...")
	time.Sleep(4 * time.Second)

	// Step 6: Verify updated values persisted
	slog.Info("Step 6: Verifying updated values...")

	getResponse, err := selectedClient.Get(context.Background(), &hydraidepbgo.GetRequest{
		Swamps: []*hydraidepbgo.GetSwamp{
			{
				SwampName: swampName.Get(),
				Keys:      []string{"batch-key-0", "batch-key-25", "batch-key-49", "batch-key-50", "batch-key-99"},
			},
		},
	})
	assert.NoError(t, err, "should get values without error")

	treasureMap := make(map[string]*hydraidepbgo.Treasure)
	for _, treasure := range getResponse.Swamps[0].Treasures {
		if treasure.IsExist {
			treasureMap[treasure.Key] = treasure
		}
	}

	// First 50 should be updated
	assert.Equal(t, "updated-value-0", treasureMap["batch-key-0"].GetStringVal())
	assert.Equal(t, "updated-value-25", treasureMap["batch-key-25"].GetStringVal())
	assert.Equal(t, "updated-value-49", treasureMap["batch-key-49"].GetStringVal())
	// Last 50 should be original
	assert.Equal(t, "batch-value-50", treasureMap["batch-key-50"].GetStringVal())
	assert.Equal(t, "batch-value-99", treasureMap["batch-key-99"].GetStringVal())

	slog.Info("TestV2Engine_BatchSetAndReload completed successfully")
}

// TestV2Engine_Uint32SliceAndReload tests Uint32Slice operations
// and verifies the slice data persists correctly after swamp close/reopen.
func TestV2Engine_Uint32SliceAndReload(t *testing.T) {
	writeInterval := int64(1)
	closeAfterIdle := int64(1)
	maxFileSize := int64(65536)

	swampName := name.New().Sanctuary("v2test").Realm("uint32slice").Swamp("reload")
	selectedClient := clientInterface.GetServiceClient(swampName)

	// Register the swamp
	_, err := selectedClient.RegisterSwamp(context.Background(), &hydraidepbgo.RegisterSwampRequest{
		SwampPattern:   swampName.Get(),
		CloseAfterIdle: closeAfterIdle,
		WriteInterval:  &writeInterval,
		MaxFileSize:    &maxFileSize,
	})
	assert.NoError(t, err, "should register swamp without error")

	defer func() {
		_, _ = selectedClient.Destroy(context.Background(), &hydraidepbgo.DestroyRequest{
			SwampName: swampName.Get(),
		})
	}()

	testKey := "slice-test-key"

	// Step 1: Push initial values to slice
	slog.Info("Step 1: Pushing initial values to uint32 slice...")

	_, err = selectedClient.Uint32SlicePush(context.Background(), &hydraidepbgo.AddToUint32SlicePushRequest{
		SwampName: swampName.Get(),
		KeySlicePairs: []*hydraidepbgo.KeySlicePair{
			{
				Key:    testKey,
				Values: []uint32{1, 2, 3, 4, 5},
			},
		},
	})
	assert.NoError(t, err, "should push without error")

	// Step 2: Wait for flush
	slog.Info("Step 2: Waiting for flush...")
	time.Sleep(4 * time.Second)

	// Step 3: Reopen and verify slice persisted
	slog.Info("Step 3: Reopening and verifying slice...")

	sizeResponse, err := selectedClient.Uint32SliceSize(context.Background(), &hydraidepbgo.Uint32SliceSizeRequest{
		SwampName: swampName.Get(),
		Key:       testKey,
	})
	assert.NoError(t, err, "should get size without error")
	assert.Equal(t, int64(5), sizeResponse.Size, "slice should have 5 elements after reload")

	// Step 4: Add more values
	slog.Info("Step 4: Adding more values to slice...")

	_, err = selectedClient.Uint32SlicePush(context.Background(), &hydraidepbgo.AddToUint32SlicePushRequest{
		SwampName: swampName.Get(),
		KeySlicePairs: []*hydraidepbgo.KeySlicePair{
			{
				Key:    testKey,
				Values: []uint32{6, 7, 8, 9, 10},
			},
		},
	})
	assert.NoError(t, err, "should push more values without error")

	// Step 5: Wait for flush
	slog.Info("Step 5: Waiting for flush...")
	time.Sleep(4 * time.Second)

	// Step 6: Verify extended slice
	slog.Info("Step 6: Verifying extended slice...")

	sizeResponse2, err := selectedClient.Uint32SliceSize(context.Background(), &hydraidepbgo.Uint32SliceSizeRequest{
		SwampName: swampName.Get(),
		Key:       testKey,
	})
	assert.NoError(t, err, "should get size without error")
	assert.Equal(t, int64(10), sizeResponse2.Size, "slice should have 10 elements after reload")

	// Step 7: Delete some values
	slog.Info("Step 7: Deleting values from slice...")

	_, err = selectedClient.Uint32SliceDelete(context.Background(), &hydraidepbgo.Uint32SliceDeleteRequest{
		SwampName: swampName.Get(),
		KeySlicePairs: []*hydraidepbgo.KeySlicePair{
			{
				Key:    testKey,
				Values: []uint32{1, 3, 5, 7, 9}, // Delete odd numbers
			},
		},
	})
	assert.NoError(t, err, "should delete values without error")

	// Step 8: Wait for flush
	slog.Info("Step 8: Waiting for flush...")
	time.Sleep(4 * time.Second)

	// Step 9: Verify deletions persisted
	slog.Info("Step 9: Verifying deletions persisted...")

	sizeResponse3, err := selectedClient.Uint32SliceSize(context.Background(), &hydraidepbgo.Uint32SliceSizeRequest{
		SwampName: swampName.Get(),
		Key:       testKey,
	})
	assert.NoError(t, err, "should get size without error")
	assert.Equal(t, int64(5), sizeResponse3.Size, "slice should have 5 elements after deletion")

	// Verify specific values exist
	existResponse, err := selectedClient.Uint32SliceIsValueExist(context.Background(), &hydraidepbgo.Uint32SliceIsValueExistRequest{
		SwampName: swampName.Get(),
		Key:       testKey,
		Value:     2, // Even number should exist
	})
	assert.NoError(t, err)
	assert.True(t, existResponse.IsExist, "value 2 should exist")

	existResponse2, err := selectedClient.Uint32SliceIsValueExist(context.Background(), &hydraidepbgo.Uint32SliceIsValueExistRequest{
		SwampName: swampName.Get(),
		Key:       testKey,
		Value:     1, // Odd number should be deleted
	})
	assert.NoError(t, err)
	assert.False(t, existResponse2.IsExist, "value 1 should be deleted")

	slog.Info("TestV2Engine_Uint32SliceAndReload completed successfully")
}

// TestV2Engine_DeleteAllKeysAndFileCleanup tests that when all keys are deleted from a swamp,
// the .hyd file and its parent folder are properly cleaned up.
func TestV2Engine_DeleteAllKeysAndFileCleanup(t *testing.T) {
	writeInterval := int64(1)
	closeAfterIdle := int64(1)
	maxFileSize := int64(65536)

	swampName := name.New().Sanctuary("v2test").Realm("deleteall").Swamp("cleanup")
	selectedClient := clientInterface.GetServiceClient(swampName)

	// Register the swamp
	_, err := selectedClient.RegisterSwamp(context.Background(), &hydraidepbgo.RegisterSwampRequest{
		SwampPattern:   swampName.Get(),
		CloseAfterIdle: closeAfterIdle,
		WriteInterval:  &writeInterval,
		MaxFileSize:    &maxFileSize,
	})
	assert.NoError(t, err, "should register swamp without error")

	// Step 1: Insert 5 keys
	slog.Info("Step 1: Inserting 5 test keys...")

	keyValues := make([]*hydraidepbgo.KeyValuePair, 5)
	for i := 0; i < 5; i++ {
		val := fmt.Sprintf("value-%d", i)
		keyValues[i] = &hydraidepbgo.KeyValuePair{
			Key:       fmt.Sprintf("key-%d", i),
			StringVal: &val,
		}
	}

	_, err = selectedClient.Set(context.Background(), &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				SwampName:        swampName.Get(),
				KeyValues:        keyValues,
				CreateIfNotExist: true,
				Overwrite:        true,
			},
		},
	})
	assert.NoError(t, err, "should insert data without error")

	// Step 2: Wait for flush
	slog.Info("Step 2: Waiting for flush...")
	time.Sleep(4 * time.Second)

	// Step 3: Verify swamp exists
	slog.Info("Step 3: Verifying swamp exists...")

	existResponse, err := selectedClient.IsSwampExist(context.Background(), &hydraidepbgo.IsSwampExistRequest{
		SwampName: swampName.Get(),
	})
	assert.NoError(t, err, "should check existence without error")
	assert.True(t, existResponse.IsExist, "swamp should exist after insert")

	countResponse, err := selectedClient.Count(context.Background(), &hydraidepbgo.CountRequest{
		Swamps: []*hydraidepbgo.CountRequest_SwampIdentifier{
			{SwampName: swampName.Get()},
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, int32(5), countResponse.Swamps[0].Count, "should have 5 keys")

	// Step 4: Delete all keys one by one
	slog.Info("Step 4: Deleting all 5 keys...")

	_, err = selectedClient.Delete(context.Background(), &hydraidepbgo.DeleteRequest{
		Swamps: []*hydraidepbgo.DeleteRequest_SwampKeys{
			{
				SwampName: swampName.Get(),
				Keys:      []string{"key-0", "key-1", "key-2", "key-3", "key-4"},
			},
		},
	})
	assert.NoError(t, err, "should delete all keys without error")

	// Step 5: Wait for flush and swamp close
	slog.Info("Step 5: Waiting for delete to be flushed...")
	time.Sleep(4 * time.Second)

	// Step 6: Verify swamp no longer exists (should be deleted when empty)
	slog.Info("Step 6: Verifying swamp is deleted after all keys removed...")

	existResponse2, err := selectedClient.IsSwampExist(context.Background(), &hydraidepbgo.IsSwampExistRequest{
		SwampName: swampName.Get(),
	})
	assert.NoError(t, err, "should check existence without error")
	assert.False(t, existResponse2.IsExist, "swamp should NOT exist after all keys deleted - .hyd file should be removed")

	// Step 7: Verify we can recreate the swamp (folder was cleaned up properly)
	slog.Info("Step 7: Verifying we can recreate the swamp...")

	newVal := "new-value-after-cleanup"
	_, err = selectedClient.Set(context.Background(), &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				SwampName: swampName.Get(),
				KeyValues: []*hydraidepbgo.KeyValuePair{
					{Key: "new-key", StringVal: &newVal},
				},
				CreateIfNotExist: true,
				Overwrite:        true,
			},
		},
	})
	assert.NoError(t, err, "should be able to recreate swamp after cleanup")

	// Wait for flush
	time.Sleep(4 * time.Second)

	// Verify new swamp exists
	existResponse3, err := selectedClient.IsSwampExist(context.Background(), &hydraidepbgo.IsSwampExistRequest{
		SwampName: swampName.Get(),
	})
	assert.NoError(t, err)
	assert.True(t, existResponse3.IsExist, "recreated swamp should exist")

	// Verify the new key
	getResponse, err := selectedClient.Get(context.Background(), &hydraidepbgo.GetRequest{
		Swamps: []*hydraidepbgo.GetSwamp{
			{
				SwampName: swampName.Get(),
				Keys:      []string{"new-key"},
			},
		},
	})
	assert.NoError(t, err)
	assert.True(t, getResponse.Swamps[0].Treasures[0].IsExist, "new key should exist")
	assert.Equal(t, newVal, getResponse.Swamps[0].Treasures[0].GetStringVal(), "new value should match")

	// Cleanup
	_, _ = selectedClient.Destroy(context.Background(), &hydraidepbgo.DestroyRequest{
		SwampName: swampName.Get(),
	})

	slog.Info("TestV2Engine_DeleteAllKeysAndFileCleanup completed successfully")
}

// TestV2Engine_DestroyAndFileCleanup tests that Destroy properly removes
// the .hyd file and cleans up the folder structure.
func TestV2Engine_DestroyAndFileCleanup(t *testing.T) {
	writeInterval := int64(1)
	closeAfterIdle := int64(1)
	maxFileSize := int64(65536)

	swampName := name.New().Sanctuary("v2test").Realm("destroy").Swamp("cleanup")
	selectedClient := clientInterface.GetServiceClient(swampName)

	// Register the swamp
	_, err := selectedClient.RegisterSwamp(context.Background(), &hydraidepbgo.RegisterSwampRequest{
		SwampPattern:   swampName.Get(),
		CloseAfterIdle: closeAfterIdle,
		WriteInterval:  &writeInterval,
		MaxFileSize:    &maxFileSize,
	})
	assert.NoError(t, err, "should register swamp without error")

	// Step 1: Insert data
	slog.Info("Step 1: Inserting test data...")

	keyValues := make([]*hydraidepbgo.KeyValuePair, 10)
	for i := 0; i < 10; i++ {
		val := fmt.Sprintf("value-%d", i)
		keyValues[i] = &hydraidepbgo.KeyValuePair{
			Key:       fmt.Sprintf("key-%d", i),
			StringVal: &val,
		}
	}

	_, err = selectedClient.Set(context.Background(), &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				SwampName:        swampName.Get(),
				KeyValues:        keyValues,
				CreateIfNotExist: true,
				Overwrite:        true,
			},
		},
	})
	assert.NoError(t, err, "should insert data without error")

	// Step 2: Wait for flush
	slog.Info("Step 2: Waiting for flush...")
	time.Sleep(4 * time.Second)

	// Step 3: Verify swamp exists
	slog.Info("Step 3: Verifying swamp exists...")

	existResponse, err := selectedClient.IsSwampExist(context.Background(), &hydraidepbgo.IsSwampExistRequest{
		SwampName: swampName.Get(),
	})
	assert.NoError(t, err)
	assert.True(t, existResponse.IsExist, "swamp should exist before destroy")

	// Step 4: Destroy the swamp
	slog.Info("Step 4: Destroying swamp...")

	_, err = selectedClient.Destroy(context.Background(), &hydraidepbgo.DestroyRequest{
		SwampName: swampName.Get(),
	})
	assert.NoError(t, err, "should destroy without error")

	// Step 5: Wait a moment for cleanup
	time.Sleep(2 * time.Second)

	// Step 6: Verify swamp no longer exists
	slog.Info("Step 6: Verifying swamp is destroyed...")

	existResponse2, err := selectedClient.IsSwampExist(context.Background(), &hydraidepbgo.IsSwampExistRequest{
		SwampName: swampName.Get(),
	})
	assert.NoError(t, err)
	assert.False(t, existResponse2.IsExist, "swamp should NOT exist after destroy")

	// Step 7: Verify we can recreate with same name
	slog.Info("Step 7: Verifying we can recreate swamp with same name...")

	newVal := "recreated-value"
	_, err = selectedClient.Set(context.Background(), &hydraidepbgo.SetRequest{
		Swamps: []*hydraidepbgo.SwampRequest{
			{
				SwampName: swampName.Get(),
				KeyValues: []*hydraidepbgo.KeyValuePair{
					{Key: "recreated-key", StringVal: &newVal},
				},
				CreateIfNotExist: true,
				Overwrite:        true,
			},
		},
	})
	assert.NoError(t, err, "should recreate swamp without error")

	time.Sleep(4 * time.Second)

	// Verify recreated swamp
	getResponse, err := selectedClient.Get(context.Background(), &hydraidepbgo.GetRequest{
		Swamps: []*hydraidepbgo.GetSwamp{
			{
				SwampName: swampName.Get(),
				Keys:      []string{"recreated-key"},
			},
		},
	})
	assert.NoError(t, err)
	assert.True(t, getResponse.Swamps[0].Treasures[0].IsExist, "recreated key should exist")
	assert.Equal(t, newVal, getResponse.Swamps[0].Treasures[0].GetStringVal())

	// Final cleanup
	_, _ = selectedClient.Destroy(context.Background(), &hydraidepbgo.DestroyRequest{
		SwampName: swampName.Get(),
	})

	slog.Info("TestV2Engine_DestroyAndFileCleanup completed successfully")
}
