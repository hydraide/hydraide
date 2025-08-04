package e2etests

import (
	"context"
	"fmt"
	"github.com/hydraide/hydraide/app/server/server"
	"github.com/hydraide/hydraide/generated/hydraidepbgo"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/client"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
)

var serverInterface server.Server
var clientInterface client.Client
var serverGlobalName name.Name

const (
	testPort = 4888
)

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

	if os.Getenv("HYDRA_SERVER_CRT") == "" {
		slog.Error("HYDRA_SERVER_CRT environment variable is not set")
		panic("HYDRA_SERVER_CRT environment variable is not set")
	}
	if os.Getenv("HYDRA_SERVER_KEY") == "" {
		slog.Error("HYDRA_SERVER_KEY environment variable is not set")
		panic("HYDRA_SERVER_KEY environment variable is not set")
	}
	if os.Getenv("HYDRA_CERT") == "" {
		slog.Error("HYDRA_CERT environment variable is not set")
		panic("HYDRA_CERT environment variable is not set")
	}

	// start the new Hydra server
	serverInterface = server.New(&server.Configuration{
		CertificateCrtFile:  os.Getenv("HYDRA_SERVER_CRT"),
		CertificateKeyFile:  os.Getenv("HYDRA_SERVER_KEY"),
		HydraServerPort:     testPort,
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
			Host:         fmt.Sprintf("localhost:%d", testPort),
			FromIsland:   0,
			ToIsland:     100,
			CertFilePath: os.Getenv("HYDRA_CERT"),
		},
	}

	// 100 folders and 2 gig message size
	clientInterface = client.New(servers, 100, 2147483648)
	if err := clientInterface.Connect(true); err != nil {
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
				fmt.Printf("Key: %s, Value: %s\n", treasure.GetKey(), treasure.GetStringVal())
				slog.Debug("treasure found", "key", treasure.GetKey(), "value", treasure.GetStringVal())
				treasureExistCounter++
			} else {
				slog.Debug("treasure not found")
				treasureNotExistCounter++
			}
		}
	}

	assert.Equal(t, 10, treasureExistCounter)
	assert.Equal(t, 1, treasureNotExistCounter)

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

func destroySwamp(selectedClient hydraidepbgo.HydraideServiceClient, swampName name.Name) {

	_, err := selectedClient.Destroy(context.Background(), &hydraidepbgo.DestroyRequest{
		SwampName: swampName.Get(),
	})

	if err != nil {
		slog.Error("error while destroying swamp", "swamp", swampName.Get(), "error", err)
		return
	}

}
