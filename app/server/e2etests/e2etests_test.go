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

	// start the new Hydra server
	serverInterface = server.New(&server.Configuration{
		CertificateCrtFile:  os.Getenv("E2E_HYDRA_SERVER_CRT"),
		CertificateKeyFile:  os.Getenv("E2E_HYDRA_SERVER_KEY"),
		ClientCAFile:        os.Getenv("E2E_HYDRA_CA_CRT"), // this is the CA that signed the client certificates
		HydraServerPort:     portAsNUmber,
		HydraMaxMessageSize: 1024 * 1024 * 1024, // 1 GB
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
	assert.NoError(t, err)

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

	slog.Info("Shifted 5 treasures from swamp", "swamp", swampName.Get(), "count", len(shiftResponse.GetTreasures()))

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
