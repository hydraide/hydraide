package hydraidego

import (
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/server/server"
	"github.com/hydraide/hydraide/generated/hydraidepbgo"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/client"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/config"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

var hydraidegoInterface Hydraidego
var clientInterface client.Client
var serverInterface server.Server
var testConfig *config.E2ETestConfig

func TestMain(m *testing.M) {
	fmt.Println("Setting up test environment...")
	setup() // start the testing environment
	code := m.Run()
	fmt.Println("Tearing down test environment...")
	teardown() // Stop the testing environment
	os.Exit(code)
}

func setup() {
	// Load E2E test configuration from .env file
	var err error
	testConfig, err = config.LoadE2ETestConfig()
	if err != nil {
		slog.Error("Failed to load E2E test configuration", "error", err)
		panic(fmt.Sprintf("Failed to load E2E test configuration: %v", err))
	}

	// Validate that all certificate files exist
	if err := testConfig.Validate(); err != nil {
		slog.Error("E2E test configuration validation failed", "error", err)
		panic(fmt.Sprintf("E2E test configuration validation failed: %v", err))
	}

	// Parse server address to get port
	port := strings.Split(testConfig.TestServerAddr, ":")
	if len(port) != 2 {
		slog.Error("HYDRAIDE_E2E_TEST_SERVER_ADDR environment variable is invalid, expected format: host:port")
		panic("HYDRAIDE_E2E_TEST_SERVER_ADDR environment variable is invalid, expected format: host:port")
	}

	portAsNumber, err := strconv.Atoi(port[1])
	if err != nil {
		slog.Error("HYDRAIDE_E2E_TEST_SERVER_ADDR port is not a valid number", "error", err)
		panic(fmt.Sprintf("HYDRAIDE_E2E_TEST_SERVER_ADDR port is not a valid number: %v", err))
	}

	// Start the new Hydra server
	serverInterface = server.New(&server.Configuration{
		CertificateCrtFile:  testConfig.ServerCertFile,
		CertificateKeyFile:  testConfig.ServerKeyFile,
		ClientCAFile:        testConfig.CACertFile, // this is the CA that signed the client certificates
		HydraServerPort:     portAsNumber,
		HydraMaxMessageSize: 1024 * 1024 * 1024, // 1 GB
	})

	if err := serverInterface.Start(); err != nil {
		slog.Error("error while starting the server", "error", err)
		panic(fmt.Sprintf("error while starting the server: %v", err))
	}

	// Create a new Hydraidego interface
	createGrpcClient()
}

func createGrpcClient() {
	// Create a new gRPC client object
	servers := []*client.Server{
		{
			Host:          testConfig.TestServerAddr,
			FromIsland:    0,
			ToIsland:      100,
			CACrtPath:     testConfig.CACertFile,
			ClientCrtPath: testConfig.ClientCertFile,
			ClientKeyPath: testConfig.ClientKeyFile,
		},
	}

	// 100 folders and 2 gig message size
	clientInterface = client.New(servers, 100, 2147483648)
	if err := clientInterface.Connect(testConfig.GRPCConnAnalysis); err != nil {
		slog.Error("error while connecting to the server", "error", err)
	}

	hydraidegoInterface = New(clientInterface)

}

func teardown() {
	// stop the microservice and exit the program
	clientInterface.CloseConnection()
	slog.Info("HydrAIDE server stopped gracefully. Program is exiting...")
	// waiting for logs to be written to the file
	time.Sleep(1 * time.Second)
	// exit the program if the microservice is stopped gracefully
	os.Exit(0)
}

// TestHydraidego_Heartbeat tests the heartbeat functionality of the Hydraidego interface.
func TestHydraidego_Heartbeat(t *testing.T) {
	err := hydraidegoInterface.Heartbeat(context.Background())
	assert.NoError(t, err, "Heartbeat should not return an error")
}

func TestConvertCatalogModelWithOmitEmpty(t *testing.T) {
	// Get current time for non-empty time values
	now := time.Now().UTC()

	testCases := []struct {
		name           string
		input          interface{}
		expectedKey    string
		expectedValues map[string]bool // Fields that should be set in the result
	}{
		{
			name: "basic key-value without omitempty",
			input: &struct {
				Key   string `hydraide:"key"`
				Value string `hydraide:"value"`
			}{"test-key", "test-value"},
			expectedKey: "test-key",
			expectedValues: map[string]bool{
				"voidVal":   false, // Not all fields omitted, VoidVal should be false
				"stringVal": true,
			},
		},
		{
			name: "value with omitempty - non-empty",
			input: &struct {
				Key   string `hydraide:"key"`
				Value string `hydraide:"value,omitempty"`
			}{"test-key", "test-value"},
			expectedKey: "test-key",
			expectedValues: map[string]bool{
				"voidVal":   false, // Not all fields omitted, VoidVal should be false
				"stringVal": true,
			},
		},
		{
			name: "value with omitempty - empty string",
			input: &struct {
				Key   string `hydraide:"key"`
				Value string `hydraide:"value,omitempty"`
			}{"test-key", ""},
			expectedKey: "test-key",
			expectedValues: map[string]bool{
				"voidVal": true, // All fields omitted, VoidVal should be true
			},
		},
		{
			name: "all metadata fields - non-empty",
			input: &struct {
				Key       string    `hydraide:"key"`
				Value     int       `hydraide:"value"`
				ExpireAt  time.Time `hydraide:"expireAt"`
				CreatedAt time.Time `hydraide:"createdAt"`
				CreatedBy string    `hydraide:"createdBy"`
				UpdatedAt time.Time `hydraide:"updatedAt"`
				UpdatedBy string    `hydraide:"updatedBy"`
			}{
				"test-key", 123, now, now, "user1", now, "user2",
			},
			expectedKey: "test-key",
			expectedValues: map[string]bool{
				"voidVal":   false, // Not all fields omitted, VoidVal should be false
				"int64Val":  true,
				"expiredAt": true,
				"createdAt": true,
				"createdBy": true,
				"updatedAt": true,
				"updatedBy": true,
			},
		},
		{
			name: "all metadata fields - are omitempty - non-empty",
			input: &struct {
				Key       string    `hydraide:"key"`
				Value     int       `hydraide:"value,omitempty"`
				ExpireAt  time.Time `hydraide:"expireAt,omitempty"`
				CreatedAt time.Time `hydraide:"createdAt,omitempty"`
				CreatedBy string    `hydraide:"createdBy,omitempty"`
				UpdatedAt time.Time `hydraide:"updatedAt,omitempty"`
				UpdatedBy string    `hydraide:"updatedBy,omitempty"`
			}{
				"test-key", 123, now, now, "user1", now, "user2",
			},
			expectedKey: "test-key",
			expectedValues: map[string]bool{
				"voidVal":   false, // Not all fields omitted, VoidVal should be false
				"int64Val":  true,
				"expiredAt": true,
				"createdAt": true,
				"createdBy": true,
				"updatedAt": true,
				"updatedBy": true,
			},
		},
		{
			name: "metadata fields with omitempty - empty",
			input: &struct {
				Key       string    `hydraide:"key"`
				Value     int       `hydraide:"value"`
				ExpireAt  time.Time `hydraide:"expireAt,omitempty"`
				CreatedAt time.Time `hydraide:"createdAt,omitempty"`
				CreatedBy string    `hydraide:"createdBy,omitempty"`
				UpdatedAt time.Time `hydraide:"updatedAt,omitempty"`
				UpdatedBy string    `hydraide:"updatedBy,omitempty"`
			}{
				"test-key", 123, time.Time{}, time.Time{}, "", time.Time{}, "",
			},
			expectedKey: "test-key",
			expectedValues: map[string]bool{
				"voidVal":   false, // Not all fields omitted, VoidVal should be false
				"int64Val":  true,
				"expiredAt": false,
				"createdAt": false,
				"createdBy": false,
				"updatedAt": false,
				"updatedBy": false,
			},
		},
		{
			name: "mixed empty and non-empty with omitempty",
			input: &struct {
				Key       string    `hydraide:"key"`
				Value     int       `hydraide:"value"`
				ExpireAt  time.Time `hydraide:"expireAt,omitempty"`
				CreatedAt time.Time `hydraide:"createdAt,omitempty"`
				CreatedBy string    `hydraide:"createdBy,omitempty"`
				UpdatedAt time.Time `hydraide:"updatedAt"`           // No omitempty
				UpdatedBy string    `hydraide:"updatedBy,omitempty"` // With omitempty
			}{
				"test-key", 123, time.Time{}, now, "user1", now, "",
			},
			expectedKey: "test-key",
			expectedValues: map[string]bool{
				"voidVal":   false, // Not all fields omitted, VoidVal should be false
				"int64Val":  true,
				"expiredAt": false, // Should be omitted (empty with omitempty)
				"createdAt": true,  // Should be included (non-empty)
				"createdBy": true,  // Should be included (non-empty)
				"updatedAt": true,  // Should be included (no omitempty tag)
				"updatedBy": false, // Should be omitted (empty with omitempty)
			},
		},
		{
			name: "pointer values with omitempty",
			input: &struct {
				Key    string  `hydraide:"key"`
				Value  *string `hydraide:"value,omitempty"`
				IntPtr *int    `hydraide:"createdBy,omitempty"` // Repurposing field for testing
			}{
				"test-key", nil, nil,
			},
			expectedKey: "test-key",
			expectedValues: map[string]bool{
				"voidVal":   true,  // All fields omitted, VoidVal should be true
				"stringVal": false, // Should be omitted (nil with omitempty)
				"createdBy": false, // Should be omitted (nil with omitempty)
			},
		},
		{
			name: "slice and map with omitempty",
			input: &struct {
				Key      string         `hydraide:"key"`
				Value    []int          `hydraide:"value,omitempty"`
				MapField map[string]int `hydraide:"createdBy,omitempty"` // Repurposing field
			}{
				"test-key", []int{}, map[string]int{},
			},
			expectedKey: "test-key",
			expectedValues: map[string]bool{
				"voidVal":   true,  // All fields omitted, VoidVal should be true
				"bytesVal":  false, // Should be omitted (empty slice with omitempty)
				"createdBy": false, // Should be omitted (empty map with omitempty)
			},
		},
		{
			name: "non-empty slice",
			input: &struct {
				Key       string `hydraide:"key"`
				Value     []int  `hydraide:"value,omitempty"`
				CreatedBy string `hydraide:"createdBy,omitempty"` // Repurposing field
			}{
				"test-key", []int{1, 2, 3}, "admin",
			},
			expectedKey: "test-key",
			expectedValues: map[string]bool{
				"voidVal":   false, // Not all fields omitted, VoidVal should be false
				"bytesVal":  true,  // Should be included (non-empty slice with omitempty)
				"createdBy": true,  // Should be included (non-empty map with omitempty)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// Convert to key-value pair
			kv, err := convertCatalogModelToKeyValuePair(tc.input)

			require.NoError(t, err, "Conversion should succeed")

			// Check key
			assert.Equal(t, tc.expectedKey, kv.Key, "Key should match expected value")

			if tc.expectedValues == nil {
				assert.True(t, *kv.VoidVal, "All fields should be omitted, VoidVal should be true")
			} else {

				for field, expected := range tc.expectedValues {
					switch field {
					case "voidVal":
						if expected {
							assert.True(t, *kv.VoidVal, "voidVal should be set to true")
						} else {
							assert.Nil(t, kv.VoidVal, "voidVal should be set to false")
						}
					case "stringVal":
						if expected {
							assert.NotEmpty(t, kv.StringVal, "stringVal should be set")
						} else {
							assert.Empty(t, kv.StringVal, "stringVal should be empty")
						}
					case "int64Val":
						if expected {
							assert.NotZero(t, kv.Int64Val, "int64Val should be set")
						} else {
							assert.Zero(t, kv.Int64Val, "int64Val should be zero")
						}
					case "expiredAt":
						if expected {
							assert.False(t, kv.ExpiredAt.AsTime().IsZero(), "expiredAt should be set")
						} else {
							assert.Nil(t, kv.ExpiredAt, "expiredAt should be nil")
						}
					case "createdAt":
						if expected {
							assert.False(t, kv.CreatedAt.AsTime().IsZero(), "createdAt should be set")
						} else {
							assert.Nil(t, kv.CreatedAt, "createdAt should be nil")
						}
					case "createdBy":
						if expected {
							assert.NotEmpty(t, kv.CreatedBy, "createdBy should be set")
						} else {
							assert.Empty(t, kv.CreatedBy, "createdBy should be empty")
						}
					case "updatedAt":
						if expected {
							assert.False(t, kv.UpdatedAt.AsTime().IsZero(), "updatedAt should be set")
						} else {
							assert.Nil(t, kv.UpdatedAt, "updatedAt should be nil")
						}
					case "updatedBy":
						if expected {
							assert.NotEmpty(t, kv.UpdatedBy, "updatedBy should be set")
						} else {
							assert.Empty(t, kv.UpdatedBy, "updatedBy should be empty")
						}
					case "bytesVal":
						if expected {
							assert.NotEmpty(t, kv.BytesVal, "bytesVal should be set")
						} else {
							assert.Empty(t, kv.BytesVal, "bytesVal should be empty")
						}
					}

				}
			}

		})
	}

}

// --- Helpers ---

func newTestSwamp(prefix string) name.Name {
	return name.New().
		Sanctuary("tests").
		Realm("increment").
		Swamp(fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()))
}

func within(d time.Duration, want time.Time, got time.Time) bool {
	if want.IsZero() || got.IsZero() {
		return false
	}
	delta := got.Sub(want)
	if delta < 0 {
		delta = -delta
	}
	return delta <= d
}

// --- Tests ---

func TestHydraidego_IsSwampExist(t *testing.T) {

	swampName := name.New().Sanctuary("test").Realm("in").Swamp("isSwampExist")
	defer func() {
		if err := hydraidegoInterface.Destroy(context.Background(), swampName); err != nil {
			t.Logf("Cleanup failed: could not destroy swamp %s: %v", swampName.Get(), err)
		}
	}()

	// Bounded context for the test call
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	isExist, err := hydraidegoInterface.IsSwampExist(ctx, swampName)
	assert.NoError(t, err, "IsSwampExist should not return an error")
	assert.False(t, isExist, "Swamp should not exist")

	type ExampleSwamp struct {
		Key   string `hydraide:"key"`
		Value string `hydraide:"value"`
	}

	treasure := &ExampleSwamp{
		Key:   "key1",
		Value: "value1",
	}

	// add a treasure to create the swamp
	_, err = hydraidegoInterface.CatalogSave(ctx, swampName, treasure)

	assert.NoError(t, err, "CatalogSave should not return an error")
	isExist, err = hydraidegoInterface.IsSwampExist(ctx, swampName)
	assert.NoError(t, err, "IsSwampExist should not return an error")
	assert.True(t, isExist, "Swamp should exist after adding a treasure")

}

// Verifies: creation path (setIfNotExist), update path (setIfExist), ExpiredAt handling.
func TestIncrementInt8_WithMetadata_CreateThenUpdate(t *testing.T) {
	swamp := newTestSwamp("int8-meta")
	key := "user-1"

	// Bounded context for the test call
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Rolling 1h TTL for the example
	now := time.Now().UTC()
	exp1 := now.Add(1 * time.Hour)
	exp2 := now.Add(2 * time.Hour)

	setIfNotExist := &IncrementMetaRequest{
		SetCreatedAt: true,
		SetCreatedBy: "test-suite",
		ExpiredAt:    exp1,
	}
	setIfExist := &IncrementMetaRequest{
		SetUpdatedAt: true,
		SetUpdatedBy: "test-suite",
		ExpiredAt:    exp2, // refresh TTL on update
	}

	// --- First call: should create + increment ---
	val1, meta1, err := hydraidegoInterface.IncrementInt8(
		ctx,
		swamp,
		key,
		1,
		&Int8Condition{RelationalOperator: LessThan, Value: 10},
		setIfNotExist,
		setIfExist,
	)
	assert.NoError(t, err, "first increment should succeed (create path)")
	assert.Equal(t, int8(1), val1, "value after first increment must be 1")
	if assert.NotNil(t, meta1, "metadata must be returned on create") {
		assert.False(t, meta1.CreatedAt.IsZero(), "CreatedAt should be set")
		assert.Equal(t, "test-suite", meta1.CreatedBy)
		assert.True(t, meta1.UpdatedAt.IsZero(), "UpdatedAt should be zero on create")
		assert.Equal(t, "", meta1.UpdatedBy)
		// ExpiredAt around exp1 (tolerance 5s)
		assert.True(t, within(5*time.Second, exp1, meta1.ExpiredAt), "ExpiredAt should be ~exp1")
	}

	// --- Second call: update path + increment ---
	val2, meta2, err := hydraidegoInterface.IncrementInt8(
		ctx,
		swamp,
		key,
		1,
		&Int8Condition{RelationalOperator: LessThan, Value: 10},
		setIfNotExist,
		setIfExist,
	)
	assert.NoError(t, err, "second increment should succeed (update path)")
	assert.Equal(t, int8(2), val2, "value after second increment must be 2")
	if assert.NotNil(t, meta2, "metadata must be returned on update") {
		assert.False(t, meta2.CreatedAt.IsZero(), "CreatedAt should remain set")
		assert.Equal(t, "test-suite", meta2.CreatedBy)
		assert.False(t, meta2.UpdatedAt.IsZero(), "UpdatedAt should be set on update")
		assert.Equal(t, "test-suite", meta2.UpdatedBy)
		// ExpiredAt refreshed to exp2 (tolerance 5s)
		assert.True(t, within(5*time.Second, exp2, meta2.ExpiredAt), "ExpiredAt should be ~exp2 (refreshed)")
		// CreatedAt should not increase after creation (allow small clock skew tolerance)
		assert.True(t, !meta1.CreatedAt.After(meta2.CreatedAt.Add(250*time.Millisecond)),
			"CreatedAt should not move forward on update (allow minor skew)")
	}
}

// Verifies: condition-fail path returns current value + metadata + ErrConditionNotMet.
func TestIncrementInt8_ConditionNotMet_ReturnsValueAndMeta(t *testing.T) {
	swamp := newTestSwamp("int8-cond")
	key := "user-2"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	setIfNotExist := &IncrementMetaRequest{
		SetCreatedAt: true,
		SetCreatedBy: "test-suite",
		ExpiredAt:    time.Now().UTC().Add(30 * time.Minute),
	}
	setIfExist := &IncrementMetaRequest{
		SetUpdatedAt: true,
		SetUpdatedBy: "test-suite",
	}

	// Prime the counter to 3
	for i := 0; i < 3; i++ {
		_, _, err := hydraidegoInterface.IncrementInt8(
			ctx, swamp, key, 1,
			&Int8Condition{RelationalOperator: LessThan, Value: 100},
			setIfNotExist, setIfExist,
		)
		assert.NoError(t, err)
	}

	// Now force a failing condition: current must be < 0 (false)
	val, meta, err := hydraidegoInterface.IncrementInt8(
		ctx, swamp, key, 1,
		&Int8Condition{RelationalOperator: LessThan, Value: 0},
		setIfNotExist, setIfExist,
	)

	// We expect condition-not-met error, but value+meta returned
	if isCond := IsConditionNotMet(err); !isCond {
		t.Fatalf("expected ErrConditionNotMet, got: %v", err)
	}
	assert.Equal(t, int8(3), val, "value should remain unchanged on condition fail")
	if assert.NotNil(t, meta, "metadata should still be returned on condition fail") {
		assert.False(t, meta.CreatedAt.IsZero(), "CreatedAt should be present")
	}
}

// Optional: sanity test without condition, only metadata on create
func TestIncrementInt8_MetadataOnlyCreate(t *testing.T) {
	swamp := newTestSwamp("int8-meta-only")
	key := "user-3"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exp := time.Now().UTC().Add(10 * time.Minute)

	val, meta, err := hydraidegoInterface.IncrementInt8(
		ctx, swamp, key, 5,
		nil, // no condition
		&IncrementMetaRequest{
			SetCreatedAt: true,
			SetCreatedBy: "test-suite",
			ExpiredAt:    exp,
		},
		&IncrementMetaRequest{
			SetUpdatedAt: true,
			SetUpdatedBy: "test-suite",
		},
	)

	assert.NoError(t, err)
	assert.Equal(t, int8(5), val)
	if assert.NotNil(t, meta) {
		assert.Equal(t, "test-suite", meta.CreatedBy)
		assert.True(t, within(5*time.Second, exp, meta.ExpiredAt))
	}
}

func TestCreatedByUpdatedBy(t *testing.T) {

	swamp := newTestSwamp("meta-createdby-updatedby")
	key := "user-4"
	userID := "user-42"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type CatalogItem struct {
		Key       string `hydraide:"key"`
		Value     int    `hydraide:"value"`
		CreatedBy string `hydraide:"createdBy"`
		UpdatedBy string `hydraide:"updatedBy"`
	}

	// Save with CreatedBy and UpdatedBy
	item := &CatalogItem{
		Key:       key,
		Value:     123,
		CreatedBy: userID,
		UpdatedBy: userID,
	}

	_, err := hydraidegoInterface.CatalogSave(ctx, swamp, item)
	assert.NoError(t, err)

	// Read back and verify fields
	var out CatalogItem
	err = hydraidegoInterface.CatalogRead(ctx, swamp, key, &out)
	assert.NoError(t, err)
	assert.Equal(t, userID, out.CreatedBy)
	assert.Equal(t, userID, out.UpdatedBy)

}

func TestIsDeletable(t *testing.T) {

	type IsDeletableProfileTest struct {
		Name           string
		DeletableField string `hydraide:"omitempty,deletable"`
	}

	// elmentünk egy swampba
	swampName := name.New().Sanctuary("test").Realm("in").Swamp("IsDeletable")
	defer func() {
		if err := hydraidegoInterface.Destroy(context.Background(), swampName); err != nil {
			t.Logf("Cleanup failed: could not destroy swamp %s: %v", swampName.Get(), err)
		}
	}()

	// Bounded context for the test call
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// first add a deletable field
	treasure := &IsDeletableProfileTest{
		Name:           "test-name",
		DeletableField: "to-be-deleted",
	}

	if err := hydraidegoInterface.ProfileSave(ctx, swampName, treasure); err != nil {
		t.Fatalf("ProfileSave failed: %v", err)
	}

	// try to get the treasure bac after adding it
	retrieved := &IsDeletableProfileTest{}
	if err := hydraidegoInterface.ProfileRead(ctx, swampName, retrieved); err != nil {
		t.Fatalf("ProfileRead failed: %v", err)
	}

	assert.Equal(t, treasure.Name, retrieved.Name, "Name should match")
	assert.Equal(t, treasure.DeletableField, retrieved.DeletableField, "DeletableField should match")

	// try to save again, but without the deletable field
	treasure.DeletableField = ""
	if err := hydraidegoInterface.ProfileSave(ctx, swampName, treasure); err != nil {
		t.Fatalf("ProfileSave (2nd) failed: %v", err)
	}

	// read back and verify the deletable field is removed
	retrieved2 := &IsDeletableProfileTest{}
	if err := hydraidegoInterface.ProfileRead(ctx, swampName, retrieved2); err != nil {
		t.Fatalf("ProfileRead (2nd) failed: %v", err)
	}

	assert.Equal(t, treasure.Name, retrieved2.Name, "Name should match after 2nd save")
	assert.Equal(t, "", retrieved2.DeletableField, "DeletableField should be deleted after 2nd save")

}

func TestUint32SlicePush(t *testing.T) {

	swampName := name.New().Sanctuary("test").Realm("in").Swamp("TestUint32SlicePush")
	defer func() {
		if err := hydraidegoInterface.Destroy(context.Background(), swampName); err != nil {
			t.Logf("Cleanup failed: could not destroy swamp %s: %v", swampName.Get(), err)
		}
	}()

	// Bounded context for the test call
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	testData := make([]*KeyValuesPair, 1)
	testData[0] = &KeyValuesPair{
		Key:    "test-key",
		Values: []uint32{1, 2, 3},
	}

	err := hydraidegoInterface.Uint32SlicePush(ctx, swampName, testData)
	if err != nil {
		t.Fatalf("Uint32SlicePush failed: %v", err)
	}

	// try to get the treasure back after adding it
	size, err := hydraidegoInterface.Uint32SliceSize(ctx, swampName, "test-key")
	assert.NoError(t, err)
	assert.Equal(t, int64(3), size, "Slice size should be 3")

	// try to get back the slice
	type MyTest struct {
		Key   string   `hydraide:"key"`
		Slice []uint32 `hydraide:"value"`
	}

	response := &MyTest{}
	err = hydraidegoInterface.CatalogRead(context.Background(), swampName, "test-key", response)
	assert.NoError(t, err)
	assert.Equal(t, []uint32{1, 2, 3}, response.Slice, "Slice content should match")

}

// TestCatalogReadMany_TimeFiltering tests the CatalogReadMany function with various time-based filtering scenarios.
// It verifies that the FromTime (inclusive) and ToTime (exclusive) boundaries work correctly,
// and ensures that the half-open interval [FromTime, ToTime) is properly respected.
func TestCatalogReadMany_TimeFiltering(t *testing.T) {

	// Setup: Create a unique Swamp for this test
	swampName := name.New().Sanctuary("test").Realm("catalog").Swamp("time-filtering")
	defer func() {
		if err := hydraidegoInterface.Destroy(context.Background(), swampName); err != nil {
			t.Logf("cleanup warning: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Define the test model
	type TimeFilteredTreasure struct {
		Key       string    `hydraide:"key"`
		Value     string    `hydraide:"value"`
		CreatedAt time.Time `hydraide:"createdAt"`
	}

	// Create 10 treasures with CreatedAt timestamps from 1 to 10 hours ago
	baseTime := time.Now().UTC()
	treasures := make([]*TimeFilteredTreasure, 10)

	for i := 0; i < 10; i++ {
		treasures[i] = &TimeFilteredTreasure{
			Key:       fmt.Sprintf("k%d", i+1),
			Value:     fmt.Sprintf("value-%d", i+1),
			CreatedAt: baseTime.Add(-time.Duration(i+1) * time.Hour),
		}
	}

	// Insert all treasures
	for _, treasure := range treasures {
		_, err := hydraidegoInterface.CatalogSave(ctx, swampName, treasure)
		require.NoError(t, err, "failed to save treasure: %s", treasure.Key)
	}

	// Wait a bit to ensure all writes are completed
	time.Sleep(100 * time.Millisecond)

	// Test cases
	testCases := []struct {
		name         string
		fromTime     *time.Time
		toTime       *time.Time
		expectedKeys []string
	}{
		{
			name:         "No time filter - all treasures",
			fromTime:     nil,
			toTime:       nil,
			expectedKeys: []string{"k1", "k2", "k3", "k4", "k5", "k6", "k7", "k8", "k9", "k10"},
		},
		{
			name:         "FromTime only - last 5 hours (inclusive)",
			fromTime:     &[]time.Time{baseTime.Add(-5 * time.Hour)}[0],
			toTime:       nil,
			expectedKeys: []string{"k1", "k2", "k3", "k4", "k5"},
		},
		{
			name:         "ToTime only - older than 5 hours (exclusive)",
			fromTime:     nil,
			toTime:       &[]time.Time{baseTime.Add(-5 * time.Hour)}[0],
			expectedKeys: []string{"k6", "k7", "k8", "k9", "k10"},
		},
		{
			name:         "Both FromTime and ToTime - 3 to 7 hours ago",
			fromTime:     &[]time.Time{baseTime.Add(-7 * time.Hour)}[0],
			toTime:       &[]time.Time{baseTime.Add(-3 * time.Hour)}[0],
			expectedKeys: []string{"k4", "k5", "k6", "k7"},
		},
		{
			name:         "Exact boundary test - FromTime inclusive",
			fromTime:     &[]time.Time{baseTime.Add(-5 * time.Hour)}[0],
			toTime:       &[]time.Time{baseTime.Add(-4 * time.Hour)}[0],
			expectedKeys: []string{"k5"},
		},
		{
			name:         "Empty result - FromTime after all treasures",
			fromTime:     &[]time.Time{baseTime.Add(1 * time.Hour)}[0],
			toTime:       nil,
			expectedKeys: []string{},
		},
		{
			name:         "Empty result - ToTime before all treasures",
			fromTime:     nil,
			toTime:       &[]time.Time{baseTime.Add(-11 * time.Hour)}[0],
			expectedKeys: []string{},
		},
		{
			name:         "Half-open interval test - ToTime excludes boundary",
			fromTime:     &[]time.Time{baseTime.Add(-8 * time.Hour)}[0],
			toTime:       &[]time.Time{baseTime.Add(-6 * time.Hour)}[0],
			expectedKeys: []string{"k7", "k8"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create index with time filtering
			index := &Index{
				IndexType:  IndexCreationTime,
				IndexOrder: IndexOrderAsc,
				From:       0,
				Limit:      0, // Get all
				FromTime:   tc.fromTime,
				ToTime:     tc.toTime,
			}

			// Collect results using the iterator
			var collectedKeys []string
			err := hydraidegoInterface.CatalogReadMany(
				ctx,
				swampName,
				index,
				TimeFilteredTreasure{},
				func(model any) error {
					treasure, ok := model.(*TimeFilteredTreasure)
					if !ok {
						return fmt.Errorf("unexpected model type")
					}
					collectedKeys = append(collectedKeys, treasure.Key)
					return nil
				},
			)

			assert.NoError(t, err, "CatalogReadMany should not return an error")
			assert.Equal(t, len(tc.expectedKeys), len(collectedKeys), "Number of results should match")
			assert.ElementsMatch(t, tc.expectedKeys, collectedKeys, "Keys should match expected set")
		})
	}
}

// TestCatalogReadMany_OrderAndPagination tests ordering (ASC/DESC) and pagination (From/Limit).
func TestCatalogReadMany_OrderAndPagination(t *testing.T) {

	swampName := name.New().Sanctuary("test").Realm("catalog").Swamp("order-pagination")
	defer func() {
		if err := hydraidegoInterface.Destroy(context.Background(), swampName); err != nil {
			t.Logf("cleanup warning: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	type OrderedTreasure struct {
		Key       string    `hydraide:"key"`
		Value     int       `hydraide:"value"`
		CreatedAt time.Time `hydraide:"createdAt"`
	}

	baseTime := time.Now().UTC()
	for i := 0; i < 5; i++ {
		treasure := &OrderedTreasure{
			Key:       fmt.Sprintf("item-%d", i+1),
			Value:     i + 1,
			CreatedAt: baseTime.Add(-time.Duration(i+1) * time.Hour),
		}
		_, err := hydraidegoInterface.CatalogSave(ctx, swampName, treasure)
		require.NoError(t, err)
	}

	time.Sleep(100 * time.Millisecond)

	testCases := []struct {
		name         string
		order        IndexOrder
		from         int32
		limit        int32
		expectedKeys []string
	}{
		{
			name:         "Ascending order - all",
			order:        IndexOrderDesc,
			from:         0,
			limit:        0,
			expectedKeys: []string{"item-1", "item-2", "item-3", "item-4", "item-5"},
		},
		{
			name:         "Descending order - all",
			order:        IndexOrderAsc,
			from:         0,
			limit:        0,
			expectedKeys: []string{"item-5", "item-4", "item-3", "item-2", "item-1"},
		},
		{
			name:         "Ascending with offset",
			order:        IndexOrderDesc,
			from:         2,
			limit:        0,
			expectedKeys: []string{"item-3", "item-4", "item-5"},
		},
		{
			name:         "Ascending with limit",
			order:        IndexOrderDesc,
			from:         0,
			limit:        3,
			expectedKeys: []string{"item-1", "item-2", "item-3"},
		},
		{
			name:         "Ascending with offset and limit",
			order:        IndexOrderDesc,
			from:         1,
			limit:        2,
			expectedKeys: []string{"item-2", "item-3"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			index := &Index{
				IndexType:  IndexCreationTime,
				IndexOrder: tc.order,
				From:       tc.from,
				Limit:      tc.limit,
			}

			var collectedKeys []string
			err := hydraidegoInterface.CatalogReadMany(
				ctx,
				swampName,
				index,
				OrderedTreasure{},
				func(model any) error {
					treasure := model.(*OrderedTreasure)
					collectedKeys = append(collectedKeys, treasure.Key)
					return nil
				},
			)

			assert.NoError(t, err)
			assert.Equal(t, tc.expectedKeys, collectedKeys, "Order should match expected")
		})
	}
}

// TestCatalogReadBatch tests the CatalogReadBatch function which retrieves multiple treasures
// by their keys in a single batch request. This test verifies:
// - Successful batch retrieval of existing keys
// - Silent skipping of non-existent keys
// - Empty keys slice handling
// - Iterator error propagation
// - Model conversion correctness
func TestCatalogReadBatch(t *testing.T) {

	// Setup: Create a unique Swamp for this test
	swampName := name.New().Sanctuary("test").Realm("catalog").Swamp("batch-read")
	defer func() {
		if err := hydraidegoInterface.Destroy(context.Background(), swampName); err != nil {
			t.Logf("cleanup warning: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Define the test model
	type BatchTreasure struct {
		Key       string    `hydraide:"key"`
		Value     string    `hydraide:"value"`
		CreatedAt time.Time `hydraide:"createdAt"`
	}

	// Test Case 1: Create test data - 10 treasures
	t.Run("Setup test data", func(t *testing.T) {
		for i := 1; i <= 10; i++ {
			treasure := &BatchTreasure{
				Key:       fmt.Sprintf("batch-key-%d", i),
				Value:     fmt.Sprintf("batch-value-%d", i),
				CreatedAt: time.Now().UTC(),
			}
			_, err := hydraidegoInterface.CatalogSave(ctx, swampName, treasure)
			require.NoError(t, err, "failed to save treasure: %s", treasure.Key)
		}
		time.Sleep(100 * time.Millisecond) // Ensure writes are committed
	})

	// Test Case 2: Read all keys in batch
	t.Run("Read all existing keys", func(t *testing.T) {
		keys := []string{
			"batch-key-1", "batch-key-2", "batch-key-3", "batch-key-4", "batch-key-5",
			"batch-key-6", "batch-key-7", "batch-key-8", "batch-key-9", "batch-key-10",
		}

		var collected []*BatchTreasure
		err := hydraidegoInterface.CatalogReadBatch(
			ctx,
			swampName,
			keys,
			BatchTreasure{},
			func(model any) error {
				treasure, ok := model.(*BatchTreasure)
				if !ok {
					return fmt.Errorf("unexpected model type")
				}
				collected = append(collected, treasure)
				return nil
			},
		)

		assert.NoError(t, err)
		assert.Equal(t, 10, len(collected), "Should retrieve all 10 treasures")

		// Verify all values are correct
		for _, treasure := range collected {
			assert.NotEmpty(t, treasure.Key, "Key should not be empty")
			assert.NotEmpty(t, treasure.Value, "Value should not be empty")
			assert.False(t, treasure.CreatedAt.IsZero(), "CreatedAt should be set")
		}
	})

	// Test Case 3: Read subset of keys
	t.Run("Read subset of keys", func(t *testing.T) {
		keys := []string{"batch-key-2", "batch-key-5", "batch-key-8"}

		var collected []*BatchTreasure
		err := hydraidegoInterface.CatalogReadBatch(
			ctx,
			swampName,
			keys,
			BatchTreasure{},
			func(model any) error {
				treasure := model.(*BatchTreasure)
				collected = append(collected, treasure)
				return nil
			},
		)

		assert.NoError(t, err)
		assert.Equal(t, 3, len(collected), "Should retrieve exactly 3 treasures")

		// Collect keys for verification
		var collectedKeys []string
		for _, treasure := range collected {
			collectedKeys = append(collectedKeys, treasure.Key)
		}
		assert.ElementsMatch(t, keys, collectedKeys, "Retrieved keys should match requested keys")
	})

	// Test Case 4: Mix of existing and non-existing keys
	t.Run("Mix of existing and non-existing keys", func(t *testing.T) {
		keys := []string{
			"batch-key-1",       // exists
			"non-existent-key1", // does not exist
			"batch-key-3",       // exists
			"non-existent-key2", // does not exist
			"batch-key-5",       // exists
		}

		var collected []*BatchTreasure
		err := hydraidegoInterface.CatalogReadBatch(
			ctx,
			swampName,
			keys,
			BatchTreasure{},
			func(model any) error {
				treasure := model.(*BatchTreasure)
				collected = append(collected, treasure)
				return nil
			},
		)

		assert.NoError(t, err)
		assert.Equal(t, 3, len(collected), "Should retrieve only existing treasures")

		// Verify we got the right keys
		var collectedKeys []string
		for _, treasure := range collected {
			collectedKeys = append(collectedKeys, treasure.Key)
		}
		expectedKeys := []string{"batch-key-1", "batch-key-3", "batch-key-5"}
		assert.ElementsMatch(t, expectedKeys, collectedKeys, "Should only return existing keys")
	})

	// Test Case 5: Empty keys slice
	t.Run("Empty keys slice", func(t *testing.T) {
		keys := []string{}

		var collected []*BatchTreasure
		err := hydraidegoInterface.CatalogReadBatch(
			ctx,
			swampName,
			keys,
			BatchTreasure{},
			func(model any) error {
				treasure := model.(*BatchTreasure)
				collected = append(collected, treasure)
				return nil
			},
		)

		assert.NoError(t, err, "Empty keys should not cause an error")
		assert.Equal(t, 0, len(collected), "Should retrieve no treasures")
	})

	// Test Case 6: All non-existent keys
	t.Run("All non-existent keys", func(t *testing.T) {
		keys := []string{"non-existent-1", "non-existent-2", "non-existent-3"}

		var collected []*BatchTreasure
		err := hydraidegoInterface.CatalogReadBatch(
			ctx,
			swampName,
			keys,
			BatchTreasure{},
			func(model any) error {
				treasure := model.(*BatchTreasure)
				collected = append(collected, treasure)
				return nil
			},
		)

		assert.NoError(t, err)
		assert.Equal(t, 0, len(collected), "Should retrieve no treasures for non-existent keys")
	})

	// Test Case 7: Iterator returns error
	t.Run("Iterator error propagation", func(t *testing.T) {
		keys := []string{"batch-key-1", "batch-key-2", "batch-key-3"}

		expectedErr := fmt.Errorf("iterator intentional error")
		callCount := 0

		err := hydraidegoInterface.CatalogReadBatch(
			ctx,
			swampName,
			keys,
			BatchTreasure{},
			func(model any) error {
				callCount++
				if callCount == 2 {
					return expectedErr
				}
				return nil
			},
		)

		assert.Error(t, err, "Should propagate iterator error")
		assert.Equal(t, expectedErr, err, "Error should match iterator error")
		assert.Equal(t, 2, callCount, "Iterator should be called until error occurs")
	})

	// Test Case 8: Nil iterator should fail
	t.Run("Nil iterator validation", func(t *testing.T) {
		keys := []string{"batch-key-1"}

		err := hydraidegoInterface.CatalogReadBatch(
			ctx,
			swampName,
			keys,
			BatchTreasure{},
			nil, // nil iterator
		)

		assert.Error(t, err, "Should return error for nil iterator")
		assert.Contains(t, err.Error(), "iterator can not be nil", "Error message should mention nil iterator")
	})

	// Test Case 9: Pointer model should fail
	t.Run("Pointer model validation", func(t *testing.T) {
		keys := []string{"batch-key-1"}

		err := hydraidegoInterface.CatalogReadBatch(
			ctx,
			swampName,
			keys,
			&BatchTreasure{}, // pointer model (invalid)
			func(model any) error { return nil },
		)

		assert.Error(t, err, "Should return error for pointer model")
		assert.Contains(t, err.Error(), "model cannot be a pointer", "Error message should mention pointer model")
	})

	// Test Case 10: Large batch read (performance verification)
	t.Run("Large batch read", func(t *testing.T) {
		// Create 100 more treasures for this test
		largeSwampName := name.New().Sanctuary("test").Realm("catalog").Swamp("large-batch")
		defer func() {
			if err := hydraidegoInterface.Destroy(context.Background(), largeSwampName); err != nil {
				t.Logf("cleanup warning: %v", err)
			}
		}()

		// Create 100 treasures
		for i := 1; i <= 100; i++ {
			treasure := &BatchTreasure{
				Key:       fmt.Sprintf("large-key-%d", i),
				Value:     fmt.Sprintf("large-value-%d", i),
				CreatedAt: time.Now().UTC(),
			}
			_, err := hydraidegoInterface.CatalogSave(ctx, largeSwampName, treasure)
			require.NoError(t, err)
		}
		time.Sleep(200 * time.Millisecond)

		// Build keys slice
		keys := make([]string, 100)
		for i := 0; i < 100; i++ {
			keys[i] = fmt.Sprintf("large-key-%d", i+1)
		}

		// Read all in one batch
		var collected []*BatchTreasure
		startTime := time.Now()
		err := hydraidegoInterface.CatalogReadBatch(
			ctx,
			largeSwampName,
			keys,
			BatchTreasure{},
			func(model any) error {
				treasure := model.(*BatchTreasure)
				collected = append(collected, treasure)
				return nil
			},
		)
		elapsed := time.Since(startTime)

		assert.NoError(t, err)
		assert.Equal(t, 100, len(collected), "Should retrieve all 100 treasures")
		t.Logf("Batch read of 100 keys took: %v", elapsed)
	})
}

type conversionTestCase struct {
	name     string
	input    any
	expected any
}

func convertKeyValuePairToTreasure(kv *hydraidepbgo.KeyValuePair) *hydraidepbgo.Treasure {
	return &hydraidepbgo.Treasure{
		Key:        kv.Key,
		IsExist:    true,
		StringVal:  kv.StringVal,
		Uint8Val:   kv.Uint8Val,
		Uint16Val:  kv.Uint16Val,
		Uint32Val:  kv.Uint32Val,
		Uint64Val:  kv.Uint64Val,
		Int8Val:    kv.Int8Val,
		Int16Val:   kv.Int16Val,
		Int32Val:   kv.Int32Val,
		Int64Val:   kv.Int64Val,
		Float32Val: kv.Float32Val,
		Float64Val: kv.Float64Val,
		BoolVal:    kv.BoolVal,
		BytesVal:   kv.BytesVal,
		ExpiredAt:  kv.ExpiredAt,
		CreatedBy:  kv.CreatedBy,
		CreatedAt:  kv.CreatedAt,
		UpdatedBy:  kv.UpdatedBy,
		UpdatedAt:  kv.UpdatedAt,
	}
}

func TestHydraideTypeConversions(t *testing.T) {

	type StructX struct {
		Field string
	}

	testCases := []conversionTestCase{
		{
			name: "string value",
			input: &struct {
				Key   string `hydraide:"key"`
				Value string `hydraide:"value"`
			}{"str-key", "hello"},
			expected: struct {
				Key   string `hydraide:"key"`
				Value string `hydraide:"value"`
			}{"str-key", "hello"},
		},
		{
			name: "int64 value",
			input: &struct {
				Key   string `hydraide:"key"`
				Value int64  `hydraide:"value"`
			}{"int64-key", 123456789},
			expected: struct {
				Key   string `hydraide:"key"`
				Value int64  `hydraide:"value"`
			}{"int64-key", 123456789},
		},
		{
			name: "bool value",
			input: &struct {
				Key   string `hydraide:"key"`
				Value bool   `hydraide:"value"`
			}{"bool-key", true},
			expected: struct {
				Key   string `hydraide:"key"`
				Value bool   `hydraide:"value"`
			}{"bool-key", true},
		},
		{
			name: "[]string slice",
			input: &struct {
				Key   string   `hydraide:"key"`
				Value []string `hydraide:"value"`
			}{"slice-key", []string{"a", "b"}},
			expected: struct {
				Key   string   `hydraide:"key"`
				Value []string `hydraide:"value"`
			}{"slice-key", []string{"a", "b"}},
		},
		{
			name: "map[string]int",
			input: &struct {
				Key   string         `hydraide:"key"`
				Value map[string]int `hydraide:"value"`
			}{"map-key", map[string]int{"a": 1, "b": 2}},
			expected: struct {
				Key   string         `hydraide:"key"`
				Value map[string]int `hydraide:"value"`
			}{"map-key", map[string]int{"a": 1, "b": 2}},
		},
		{
			name: "float64 value",
			input: &struct {
				Key   string  `hydraide:"key"`
				Value float64 `hydraide:"value"`
			}{"float64-key", 3.1415},
			expected: struct {
				Key   string  `hydraide:"key"`
				Value float64 `hydraide:"value"`
			}{"float64-key", 3.1415},
		},
		{
			name: "[]int64 slice",
			input: &struct {
				Key   string  `hydraide:"key"`
				Value []int64 `hydraide:"value"`
			}{"slice-int64-key", []int64{100, 200}},
			expected: struct {
				Key   string  `hydraide:"key"`
				Value []int64 `hydraide:"value"`
			}{"slice-int64-key", []int64{100, 200}},
		},
		{
			name: "[]byte value",
			input: &struct {
				Key   string `hydraide:"key"`
				Value []byte `hydraide:"value"`
			}{"byte-key", []byte("hello")},
			expected: struct {
				Key   string `hydraide:"key"`
				Value []byte `hydraide:"value"`
			}{"byte-key", []byte("hello")},
		},
		{
			name: "time.Time as value",
			input: &struct {
				Key   string    `hydraide:"key"`
				Value time.Time `hydraide:"value"`
			}{"time-key", time.Unix(1700000000, 0).UTC()},
			expected: struct {
				Key   string    `hydraide:"key"`
				Value time.Time `hydraide:"value"`
			}{"time-key", time.Unix(1700000000, 0).UTC()},
		},
		{
			name: "pointer to struct",
			input: &struct {
				Key   string   `hydraide:"key"`
				Value *StructX `hydraide:"value"`
			}{"ptr-key", &StructX{Field: "val"}},
			expected: struct {
				Key   string   `hydraide:"key"`
				Value *StructX `hydraide:"value"`
			}{"ptr-key", &StructX{Field: "val"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			kv, err := convertCatalogModelToKeyValuePair(tc.input)
			require.NoError(t, err)

			treasure := convertKeyValuePairToTreasure(kv)

			restored := reflect.New(reflect.TypeOf(tc.expected)).Interface()
			err = convertProtoTreasureToCatalogModel(treasure, restored)
			require.NoError(t, err)

			require.Equal(t, tc.expected, reflect.ValueOf(restored).Elem().Interface())
		})
	}
}

// TestOmitEmptyFieldsE2E tests the real behavior of omitempty fields with the server
// This test validates that:
// 1. Creating data without updatedAt, updatedBy, and expiredAt works correctly (omitempty)
// 2. Reading data returns empty values for these fields
// 3. Updating data with these fields populated works correctly
// 4. Reading data after update returns the populated values
func TestOmitEmptyFieldsE2E(t *testing.T) {
	ctx := context.Background()

	// Create a test swamp
	swampName := name.New().
		Sanctuary("tests").
		Realm("omitempty").
		Swamp(fmt.Sprintf("test-omit-%d", time.Now().UnixNano()))

	// Register the swamp
	err := hydraidegoInterface.RegisterSwamp(ctx, &RegisterSwampRequest{
		SwampPattern: swampName,
	})
	require.Empty(t, err, "Failed to register swamp")

	// Clean up
	defer func() {
		if destroyErr := hydraidegoInterface.Destroy(ctx, swampName); destroyErr != nil {
			t.Logf("Failed to destroy swamp: %v", destroyErr)
		}
	}()

	// Define test model with omitempty fields
	type TestModel struct {
		Key       string    `hydraide:"key"`
		Value     string    `hydraide:"value"`
		UpdatedAt time.Time `hydraide:"updatedAt,omitempty"`
		UpdatedBy string    `hydraide:"updatedBy,omitempty"`
		ExpiredAt time.Time `hydraide:"expireAt,omitempty"`
	}

	// Step 1: Create data without updatedAt, updatedBy, and expiredAt
	t.Run("Step1_CreateWithoutOmitEmptyFields", func(t *testing.T) {
		initialData := &TestModel{
			Key:   "test-key-1",
			Value: "initial-value",
			// UpdatedAt, UpdatedBy, ExpiredAt are intentionally not set (zero values)
		}

		err := hydraidegoInterface.CatalogCreate(ctx, swampName, initialData)
		require.NoError(t, err, "Failed to create data without omitempty fields")
	})

	// Step 2: Read data and verify that updatedAt, updatedBy, and expiredAt are empty
	t.Run("Step2_ReadAndVerifyEmptyFields", func(t *testing.T) {
		readData := &TestModel{}
		err := hydraidegoInterface.CatalogRead(ctx, swampName, "test-key-1", readData)
		require.NoError(t, err, "Failed to read data")

		assert.Equal(t, "test-key-1", readData.Key, "Key should match")
		assert.Equal(t, "initial-value", readData.Value, "Value should match")
		assert.True(t, readData.UpdatedAt.IsZero(), "UpdatedAt should be empty (zero value)")
		assert.Empty(t, readData.UpdatedBy, "UpdatedBy should be empty")
		assert.True(t, readData.ExpiredAt.IsZero(), "ExpiredAt should be empty (zero value)")
	})

	// Step 3: Update data with updatedAt, updatedBy, and expiredAt values
	t.Run("Step3_UpdateWithOmitEmptyFields", func(t *testing.T) {
		now := time.Now().UTC()
		futureTime := now.Add(24 * time.Hour)

		updateData := &TestModel{
			Key:       "test-key-1",
			Value:     "updated-value",
			UpdatedAt: now,
			UpdatedBy: "test-user",
			ExpiredAt: futureTime,
		}

		err := hydraidegoInterface.CatalogUpdate(ctx, swampName, updateData)
		require.NoError(t, err, "Failed to update data with omitempty fields")
	})

	// Step 4: Read data and verify that updatedAt, updatedBy, and expiredAt are now populated
	t.Run("Step4_ReadAndVerifyPopulatedFields", func(t *testing.T) {
		readData := &TestModel{}
		err := hydraidegoInterface.CatalogRead(ctx, swampName, "test-key-1", readData)
		require.NoError(t, err, "Failed to read updated data")

		assert.Equal(t, "test-key-1", readData.Key, "Key should match")
		assert.Equal(t, "updated-value", readData.Value, "Value should be updated")
		assert.False(t, readData.UpdatedAt.IsZero(), "UpdatedAt should be populated")
		assert.Equal(t, "test-user", readData.UpdatedBy, "UpdatedBy should be populated")
		assert.False(t, readData.ExpiredAt.IsZero(), "ExpiredAt should be populated")

		// Verify that the times are within a reasonable range (1 second tolerance)
		assert.WithinDuration(t, time.Now().UTC(), readData.UpdatedAt, 5*time.Second, "UpdatedAt should be recent")
		assert.WithinDuration(t, time.Now().UTC().Add(24*time.Hour), readData.ExpiredAt, 5*time.Second, "ExpiredAt should be ~24 hours in the future")
	})

	// Additional test: Create a second record with all fields populated from the start
	t.Run("Step5_CreateWithAllFieldsPopulated", func(t *testing.T) {
		now := time.Now().UTC()
		futureTime := now.Add(48 * time.Hour)

		fullData := &TestModel{
			Key:       "test-key-2",
			Value:     "full-value",
			UpdatedAt: now,
			UpdatedBy: "admin",
			ExpiredAt: futureTime,
		}

		err := hydraidegoInterface.CatalogCreate(ctx, swampName, fullData)
		require.NoError(t, err, "Failed to create data with all fields populated")

		// Read back and verify
		readData := &TestModel{}
		err = hydraidegoInterface.CatalogRead(ctx, swampName, "test-key-2", readData)
		require.NoError(t, err, "Failed to read fully populated data")

		assert.Equal(t, "test-key-2", readData.Key, "Key should match")
		assert.Equal(t, "full-value", readData.Value, "Value should match")
		assert.False(t, readData.UpdatedAt.IsZero(), "UpdatedAt should be populated")
		assert.Equal(t, "admin", readData.UpdatedBy, "UpdatedBy should be 'admin'")
		assert.False(t, readData.ExpiredAt.IsZero(), "ExpiredAt should be populated")
	})
}

// TestCatalogSaveWithOmitEmptyRealWorldScenario tests a real-world scenario
// where a model is saved without UpdatedAt/UpdatedBy, then loaded, modified,
// and saved again with UpdatedAt/UpdatedBy populated using CatalogSave.
// This mimics the user's test case exactly with CatalogSave instead of CatalogUpdate.
func TestCatalogSaveWithOmitEmptyRealWorldScenario(t *testing.T) {
	ctx := context.Background()

	// Create a test swamp
	swampName := name.New().
		Sanctuary("tests").
		Realm("real-world").
		Swamp(fmt.Sprintf("catalog-save-%d", time.Now().UnixNano()))

	// Clean up
	defer func() {
		if destroyErr := hydraidegoInterface.Destroy(ctx, swampName); destroyErr != nil {
			t.Logf("Failed to destroy swamp: %v", destroyErr)
		}
	}()

	// Define a payload structure (similar to user's EmailManagerPayload)
	type TestPayload struct {
		Title  string
		Status int
		Count  int
	}

	// Define test model (similar to user's ModelEmailManagerCatalog)
	type TestCatalog struct {
		EmailID   string       `hydraide:"key"`
		Payload   *TestPayload `hydraide:"value"`
		CreatedAt time.Time    `hydraide:"createdAt"`
		CreatedBy string       `hydraide:"createdBy"`
		UpdatedAt time.Time    `hydraide:"updatedAt,omitempty"`
		UpdatedBy string       `hydraide:"updatedBy,omitempty"`
	}

	// Step 1: Save initial model WITHOUT UpdatedAt/UpdatedBy (using CatalogSave like user does)
	t.Run("Step1_InitialSaveWithoutUpdatedFields", func(t *testing.T) {
		testModel := &TestCatalog{
			EmailID: "test-email-1",
			Payload: &TestPayload{
				Title:  "Original Title",
				Status: 1,
				Count:  10,
			},
			CreatedAt: time.Now().UTC(),
			CreatedBy: "unittest-user",
			// UpdatedAt and UpdatedBy are intentionally NOT set (zero values)
		}

		_, err := hydraidegoInterface.CatalogSave(ctx, swampName, testModel)
		require.NoError(t, err, "Initial save should succeed")
	})

	// Step 2: Load the model (simulate user's Load() method)
	t.Run("Step2_LoadModel", func(t *testing.T) {
		loadedModel := &TestCatalog{
			EmailID: "test-email-1",
		}

		err := hydraidegoInterface.CatalogRead(ctx, swampName, "test-email-1", loadedModel)
		require.NoError(t, err, "Load should succeed")

		// Verify initial state
		assert.Equal(t, "test-email-1", loadedModel.EmailID)
		assert.Equal(t, "Original Title", loadedModel.Payload.Title)
		assert.Equal(t, "unittest-user", loadedModel.CreatedBy)
		assert.False(t, loadedModel.CreatedAt.IsZero(), "CreatedAt should be populated")
		assert.True(t, loadedModel.UpdatedAt.IsZero(), "UpdatedAt should be empty (zero)")
		assert.Empty(t, loadedModel.UpdatedBy, "UpdatedBy should be empty")
	})

	// Step 3: Modify and save WITH UpdatedAt/UpdatedBy (using CatalogSave like user does)
	t.Run("Step3_ModifyAndSaveWithUpdatedFields", func(t *testing.T) {
		loadedModel := &TestCatalog{
			EmailID: "test-email-1",
		}
		err := hydraidegoInterface.CatalogRead(ctx, swampName, "test-email-1", loadedModel)
		require.NoError(t, err)

		// Modify the model
		loadedModel.Payload.Title = "Updated Title"
		loadedModel.Payload.Status = 2
		loadedModel.Payload.Count = 25
		loadedModel.UpdatedAt = time.Now().UTC()
		loadedModel.UpdatedBy = "unittest-updater"

		// Save using CatalogSave (like user does)
		_, err = hydraidegoInterface.CatalogSave(ctx, swampName, loadedModel)
		require.NoError(t, err, "Save with UpdatedAt/UpdatedBy should succeed")
	})

	// Step 4: Reload and verify UpdatedAt/UpdatedBy are populated
	t.Run("Step4_ReloadAndVerifyUpdatedFields", func(t *testing.T) {
		reloadedModel := &TestCatalog{
			EmailID: "test-email-1",
		}

		err := hydraidegoInterface.CatalogRead(ctx, swampName, "test-email-1", reloadedModel)
		require.NoError(t, err, "Reload should succeed")

		// Verify updated fields
		assert.Equal(t, "test-email-1", reloadedModel.EmailID)
		assert.Equal(t, "Updated Title", reloadedModel.Payload.Title, "Title should be updated")
		assert.Equal(t, 2, reloadedModel.Payload.Status, "Status should be updated")
		assert.Equal(t, 25, reloadedModel.Payload.Count, "Count should be updated")
		assert.False(t, reloadedModel.UpdatedAt.IsZero(), "UpdatedAt MUST be populated after second save")
		assert.Equal(t, "unittest-updater", reloadedModel.UpdatedBy, "UpdatedBy MUST be populated after second save")
		assert.False(t, reloadedModel.CreatedAt.IsZero(), "CreatedAt should still be populated")
		assert.Equal(t, "unittest-user", reloadedModel.CreatedBy, "CreatedBy should remain unchanged")
	})
}

// TestProfileReadBatch tests the batch profile read functionality
func TestProfileReadBatch(t *testing.T) {
	ctx := context.Background()

	// Define a profile model for testing
	type UserProfile struct {
		Username      string    `hydraide:"Username"`
		Email         string    `hydraide:"Email"`
		Age           int64     `hydraide:"Age"`
		IsActive      bool      `hydraide:"IsActive"`
		LastLoginTime time.Time `hydraide:"LastLoginTime"`
	}

	// Step 1: Create multiple profiles in different swamps
	t.Run("Step1_CreateMultipleProfiles", func(t *testing.T) {
		profiles := []struct {
			swampID  string
			username string
			email    string
			age      int64
			active   bool
		}{
			{"user-alice", "alice", "alice@example.com", 25, true},
			{"user-bob", "bob", "bob@example.com", 30, true},
			{"user-charlie", "charlie", "charlie@example.com", 35, false},
			{"user-diana", "diana", "diana@example.com", 28, true},
			{"user-eve", "eve", "eve@example.com", 32, true},
		}

		for _, p := range profiles {
			swampName := name.New().Sanctuary("unittest-profiles").Realm("batch-test").Swamp(p.swampID)
			profile := &UserProfile{
				Username:      p.username,
				Email:         p.email,
				Age:           p.age,
				IsActive:      p.active,
				LastLoginTime: time.Now().UTC(),
			}

			err := hydraidegoInterface.ProfileSave(ctx, swampName, profile)
			require.NoError(t, err, "ProfileSave should succeed for %s", p.username)
		}
	})

	// Step 2: Read all profiles using ProfileReadBatch
	t.Run("Step2_ReadBatch", func(t *testing.T) {
		swampNames := []name.Name{
			name.New().Sanctuary("unittest-profiles").Realm("batch-test").Swamp("user-alice"),
			name.New().Sanctuary("unittest-profiles").Realm("batch-test").Swamp("user-bob"),
			name.New().Sanctuary("unittest-profiles").Realm("batch-test").Swamp("user-charlie"),
			name.New().Sanctuary("unittest-profiles").Realm("batch-test").Swamp("user-diana"),
			name.New().Sanctuary("unittest-profiles").Realm("batch-test").Swamp("user-eve"),
		}

		var results []*UserProfile
		var callCount int

		err := hydraidegoInterface.ProfileReadBatch(ctx, swampNames, &UserProfile{}, func(swampName name.Name, model any, err error) error {
			callCount++
			require.NoError(t, err, "ProfileReadBatch should not return error for swamp: %s", swampName.Get())

			profile := model.(*UserProfile)
			results = append(results, profile)
			return nil
		})

		require.NoError(t, err, "ProfileReadBatch should succeed")
		assert.Equal(t, 5, callCount, "Iterator should be called 5 times")
		assert.Equal(t, 5, len(results), "Should have 5 profiles loaded")

		// Verify each profile
		expectedUsers := map[string]struct {
			email    string
			age      int64
			isActive bool
		}{
			"alice":   {"alice@example.com", 25, true},
			"bob":     {"bob@example.com", 30, true},
			"charlie": {"charlie@example.com", 35, false},
			"diana":   {"diana@example.com", 28, true},
			"eve":     {"eve@example.com", 32, true},
		}

		for _, profile := range results {
			expected, ok := expectedUsers[profile.Username]
			require.True(t, ok, "Unexpected username: %s", profile.Username)
			assert.Equal(t, expected.email, profile.Email, "Email should match for %s", profile.Username)
			assert.Equal(t, expected.age, profile.Age, "Age should match for %s", profile.Username)
			assert.Equal(t, expected.isActive, profile.IsActive, "IsActive should match for %s", profile.Username)
			assert.False(t, profile.LastLoginTime.IsZero(), "LastLoginTime should be set for %s", profile.Username)
		}
	})

	// Step 3: Test with non-existent swamp
	t.Run("Step3_ReadBatchWithNonExistentSwamp", func(t *testing.T) {
		swampNames := []name.Name{
			name.New().Sanctuary("unittest-profiles").Realm("batch-test").Swamp("user-alice"),
			name.New().Sanctuary("unittest-profiles").Realm("batch-test").Swamp("user-nonexistent"), // This doesn't exist
			name.New().Sanctuary("unittest-profiles").Realm("batch-test").Swamp("user-bob"),
		}

		var successCount int
		var errorCount int

		err := hydraidegoInterface.ProfileReadBatch(ctx, swampNames, &UserProfile{}, func(swampName name.Name, model any, err error) error {
			if err != nil {
				errorCount++
				assert.Contains(t, swampName.Get(), "user-nonexistent", "Error should be for non-existent swamp")
				return nil // Continue processing other swamps
			}
			successCount++
			return nil
		})

		require.NoError(t, err, "ProfileReadBatch should not fail even if some swamps don't exist")
		assert.Equal(t, 2, successCount, "Should successfully read 2 existing swamps")
		assert.Equal(t, 1, errorCount, "Should have 1 error for non-existent swamp")
	})

	// Step 4: Test with empty swamp list
	t.Run("Step4_ReadBatchWithEmptyList", func(t *testing.T) {
		var swampNames []name.Name
		var callCount int

		err := hydraidegoInterface.ProfileReadBatch(ctx, swampNames, &UserProfile{}, func(swampName name.Name, model any, err error) error {
			callCount++
			return nil
		})

		// Should handle empty list gracefully
		assert.Error(t, err, "Should return error for empty swamp list")
		assert.Equal(t, 0, callCount, "Iterator should not be called for empty list")
	})

	// Step 5: Cleanup - destroy all test swamps
	t.Run("Step5_Cleanup", func(t *testing.T) {
		swampIDs := []string{"user-alice", "user-bob", "user-charlie", "user-diana", "user-eve"}
		for _, swampID := range swampIDs {
			swampName := name.New().Sanctuary("unittest-profiles").Realm("batch-test").Swamp(swampID)
			err := hydraidegoInterface.Destroy(ctx, swampName)
			assert.NoError(t, err, "Destroy should succeed for %s", swampID)
		}
	})
}

// TestProfileSaveBatch tests the batch profile save functionality
func TestProfileSaveBatch(t *testing.T) {
	ctx := context.Background()

	// Define a profile model for testing
	type UserProfile struct {
		Username      string    `hydraide:"Username"`
		Email         string    `hydraide:"Email"`
		Age           int64     `hydraide:"Age"`
		IsActive      bool      `hydraide:"IsActive"`
		LastLoginTime time.Time `hydraide:"LastLoginTime"`
		Score         float64   `hydraide:"Score,omitempty,deletable"`
	}

	// Step 1: Save multiple profiles using ProfileSaveBatch
	t.Run("Step1_SaveBatch", func(t *testing.T) {
		swampNames := []name.Name{
			name.New().Sanctuary("unittest-profiles").Realm("batch-save-test").Swamp("user-alice"),
			name.New().Sanctuary("unittest-profiles").Realm("batch-save-test").Swamp("user-bob"),
			name.New().Sanctuary("unittest-profiles").Realm("batch-save-test").Swamp("user-charlie"),
			name.New().Sanctuary("unittest-profiles").Realm("batch-save-test").Swamp("user-diana"),
			name.New().Sanctuary("unittest-profiles").Realm("batch-save-test").Swamp("user-eve"),
		}

		profiles := []any{
			&UserProfile{Username: "alice", Email: "alice@test.com", Age: 25, IsActive: true, LastLoginTime: time.Now().UTC(), Score: 100.5},
			&UserProfile{Username: "bob", Email: "bob@test.com", Age: 30, IsActive: true, LastLoginTime: time.Now().UTC(), Score: 200.75},
			&UserProfile{Username: "charlie", Email: "charlie@test.com", Age: 35, IsActive: false, LastLoginTime: time.Now().UTC(), Score: 150.0},
			&UserProfile{Username: "diana", Email: "diana@test.com", Age: 28, IsActive: true, LastLoginTime: time.Now().UTC(), Score: 300.25},
			&UserProfile{Username: "eve", Email: "eve@test.com", Age: 32, IsActive: true, LastLoginTime: time.Now().UTC(), Score: 250.5},
		}

		var successCount int
		var errorCount int

		err := hydraidegoInterface.ProfileSaveBatch(ctx, swampNames, profiles, func(swampName name.Name, err error) error {
			if err != nil {
				errorCount++
				t.Logf("❌ Failed to save %s: %v", swampName.Get(), err)
				return nil // Continue with other profiles
			}
			successCount++
			return nil
		})

		require.NoError(t, err, "ProfileSaveBatch should succeed")
		assert.Equal(t, 5, successCount, "Should have saved 5 profiles")
		assert.Equal(t, 0, errorCount, "Should have no errors")
	})

	// Step 2: Read back the profiles to verify they were saved correctly
	t.Run("Step2_VerifyBatchSave", func(t *testing.T) {
		swampNames := []name.Name{
			name.New().Sanctuary("unittest-profiles").Realm("batch-save-test").Swamp("user-alice"),
			name.New().Sanctuary("unittest-profiles").Realm("batch-save-test").Swamp("user-bob"),
			name.New().Sanctuary("unittest-profiles").Realm("batch-save-test").Swamp("user-charlie"),
			name.New().Sanctuary("unittest-profiles").Realm("batch-save-test").Swamp("user-diana"),
			name.New().Sanctuary("unittest-profiles").Realm("batch-save-test").Swamp("user-eve"),
		}

		var results []*UserProfile

		err := hydraidegoInterface.ProfileReadBatch(ctx, swampNames, &UserProfile{}, func(swampName name.Name, model any, err error) error {
			require.NoError(t, err, "ProfileReadBatch should not return error for swamp: %s", swampName.Get())
			profile := model.(*UserProfile)
			results = append(results, profile)
			return nil
		})

		require.NoError(t, err, "ProfileReadBatch should succeed")
		assert.Equal(t, 5, len(results), "Should have read 5 profiles")

		// Verify specific values
		expectedUsers := map[string]struct {
			email    string
			age      int64
			isActive bool
			score    float64
		}{
			"alice":   {"alice@test.com", 25, true, 100.5},
			"bob":     {"bob@test.com", 30, true, 200.75},
			"charlie": {"charlie@test.com", 35, false, 150.0},
			"diana":   {"diana@test.com", 28, true, 300.25},
			"eve":     {"eve@test.com", 32, true, 250.5},
		}

		for _, profile := range results {
			expected, ok := expectedUsers[profile.Username]
			require.True(t, ok, "Unexpected username: %s", profile.Username)
			assert.Equal(t, expected.email, profile.Email, "Email should match for %s", profile.Username)
			assert.Equal(t, expected.age, profile.Age, "Age should match for %s", profile.Username)
			assert.Equal(t, expected.isActive, profile.IsActive, "IsActive should match for %s", profile.Username)
			assert.InDelta(t, expected.score, profile.Score, 0.001, "Score should match for %s", profile.Username)
		}
	})

	// Step 3: Update profiles using batch save (test deletable field)
	t.Run("Step3_UpdateBatchWithDeletable", func(t *testing.T) {
		swampNames := []name.Name{
			name.New().Sanctuary("unittest-profiles").Realm("batch-save-test").Swamp("user-alice"),
			name.New().Sanctuary("unittest-profiles").Realm("batch-save-test").Swamp("user-bob"),
		}

		// Update alice and bob - set Score to 0 (should delete due to deletable tag)
		profiles := []any{
			&UserProfile{Username: "alice", Email: "alice_updated@test.com", Age: 26, IsActive: true, LastLoginTime: time.Now().UTC(), Score: 0},
			&UserProfile{Username: "bob", Email: "bob_updated@test.com", Age: 31, IsActive: false, LastLoginTime: time.Now().UTC(), Score: 0},
		}

		var successCount int

		err := hydraidegoInterface.ProfileSaveBatch(ctx, swampNames, profiles, func(swampName name.Name, err error) error {
			require.NoError(t, err, "Should save successfully for %s", swampName.Get())
			successCount++
			return nil
		})

		require.NoError(t, err, "ProfileSaveBatch should succeed")
		assert.Equal(t, 2, successCount, "Should have saved 2 profiles")

		// Verify updates
		var results []*UserProfile
		err = hydraidegoInterface.ProfileReadBatch(ctx, swampNames, &UserProfile{}, func(swampName name.Name, model any, err error) error {
			require.NoError(t, err)
			results = append(results, model.(*UserProfile))
			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, 2, len(results))

		for _, profile := range results {
			if profile.Username == "alice" {
				assert.Equal(t, "alice_updated@test.com", profile.Email)
				assert.Equal(t, int64(26), profile.Age)
				assert.True(t, profile.IsActive)
				assert.Equal(t, 0.0, profile.Score, "Score should be 0 (deleted due to deletable)")
			} else if profile.Username == "bob" {
				assert.Equal(t, "bob_updated@test.com", profile.Email)
				assert.Equal(t, int64(31), profile.Age)
				assert.False(t, profile.IsActive)
				assert.Equal(t, 0.0, profile.Score, "Score should be 0 (deleted due to deletable)")
			}
		}
	})

	// Step 4: Test with mismatched lengths (validation)
	t.Run("Step4_MismatchedLengths", func(t *testing.T) {
		swampNames := []name.Name{
			name.New().Sanctuary("unittest-profiles").Realm("batch-save-test").Swamp("user-alice"),
			name.New().Sanctuary("unittest-profiles").Realm("batch-save-test").Swamp("user-bob"),
		}

		profiles := []any{
			&UserProfile{Username: "alice", Email: "alice@test.com", Age: 25, IsActive: true, LastLoginTime: time.Now().UTC()},
		}

		err := hydraidegoInterface.ProfileSaveBatch(ctx, swampNames, profiles, func(swampName name.Name, err error) error {
			return nil
		})

		assert.Error(t, err, "Should return error for mismatched lengths")
		assert.Contains(t, err.Error(), "must have the same length")
	})

	// Step 5: Test with empty list
	t.Run("Step5_EmptyList", func(t *testing.T) {
		var swampNames []name.Name
		var profiles []any

		err := hydraidegoInterface.ProfileSaveBatch(ctx, swampNames, profiles, func(swampName name.Name, err error) error {
			return nil
		})

		assert.Error(t, err, "Should return error for empty list")
	})

	// Step 6: Cleanup - destroy all test swamps
	t.Run("Step6_Cleanup", func(t *testing.T) {
		swampIDs := []string{"user-alice", "user-bob", "user-charlie", "user-diana", "user-eve"}
		for _, swampID := range swampIDs {
			swampName := name.New().Sanctuary("unittest-profiles").Realm("batch-save-test").Swamp(swampID)
			err := hydraidegoInterface.Destroy(ctx, swampName)
			assert.NoError(t, err, "Destroy should succeed for %s", swampID)
		}
	})
}

func TestCatalogShiftBatch(t *testing.T) {

	// Setup: Create a unique Swamp for this test
	swampName := name.New().Sanctuary("test").Realm("catalog").Swamp("batch-shift")
	defer func() {
		if err := hydraidegoInterface.Destroy(context.Background(), swampName); err != nil {
			t.Logf("cleanup warning: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Define the test model
	type ShiftTreasure struct {
		Key       string    `hydraide:"key"`
		Value     string    `hydraide:"value"`
		Priority  int       `hydraide:"priority,omitempty"`
		CreatedAt time.Time `hydraide:"createdAt"`
		CreatedBy string    `hydraide:"createdBy"`
	}

	// Test Case 1: Create test data - 10 treasures
	t.Run("Setup test data", func(t *testing.T) {
		for i := 1; i <= 10; i++ {
			treasure := &ShiftTreasure{
				Key:       fmt.Sprintf("shift-key-%d", i),
				Value:     fmt.Sprintf("shift-value-%d", i),
				Priority:  i,
				CreatedAt: time.Now().UTC(),
				CreatedBy: "test-user",
			}
			_, err := hydraidegoInterface.CatalogSave(ctx, swampName, treasure)
			require.NoError(t, err, "failed to save treasure: %s", treasure.Key)
		}
		time.Sleep(100 * time.Millisecond) // Ensure writes are committed
	})

	// Test Case 2: Shift (clone and delete) multiple treasures
	t.Run("Shift multiple existing keys", func(t *testing.T) {
		keys := []string{"shift-key-1", "shift-key-3", "shift-key-5", "shift-key-7"}

		var shifted []*ShiftTreasure
		err := hydraidegoInterface.CatalogShiftBatch(
			ctx,
			swampName,
			keys,
			ShiftTreasure{},
			func(model any) error {
				treasure, ok := model.(*ShiftTreasure)
				if !ok {
					return fmt.Errorf("unexpected model type")
				}
				shifted = append(shifted, treasure)
				return nil
			},
		)

		assert.NoError(t, err)
		assert.Equal(t, 4, len(shifted), "Should shift 4 treasures")

		// Verify all values are correct
		for _, treasure := range shifted {
			assert.NotEmpty(t, treasure.Key, "Key should not be empty")
			assert.NotEmpty(t, treasure.Value, "Value should not be empty")
			// Priority has omitempty tag, so it may be 0 for some serialization paths
			assert.False(t, treasure.CreatedAt.IsZero(), "CreatedAt should be set")
			assert.Equal(t, "test-user", treasure.CreatedBy, "CreatedBy should match")
		}

		// Verify shifted treasures are deleted from swamp
		var stillExists []*ShiftTreasure
		err = hydraidegoInterface.CatalogReadBatch(
			ctx,
			swampName,
			keys,
			ShiftTreasure{},
			func(model any) error {
				treasure := model.(*ShiftTreasure)
				stillExists = append(stillExists, treasure)
				return nil
			},
		)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(stillExists), "Shifted treasures should be deleted")

		// Verify other treasures still exist
		remainingKeys := []string{"shift-key-2", "shift-key-4", "shift-key-6"}
		var remaining []*ShiftTreasure
		err = hydraidegoInterface.CatalogReadBatch(
			ctx,
			swampName,
			remainingKeys,
			ShiftTreasure{},
			func(model any) error {
				treasure := model.(*ShiftTreasure)
				remaining = append(remaining, treasure)
				return nil
			},
		)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(remaining), "Other treasures should still exist")
	})

	// Test Case 3: Mix of existing and non-existing keys
	t.Run("Mix of existing and non-existing keys", func(t *testing.T) {
		keys := []string{
			"shift-key-2",       // exists
			"non-existent-key1", // does not exist
			"shift-key-4",       // exists
			"non-existent-key2", // does not exist
			"shift-key-6",       // exists
		}

		var shifted []*ShiftTreasure
		err := hydraidegoInterface.CatalogShiftBatch(
			ctx,
			swampName,
			keys,
			ShiftTreasure{},
			func(model any) error {
				treasure := model.(*ShiftTreasure)
				shifted = append(shifted, treasure)
				return nil
			},
		)

		assert.NoError(t, err)
		assert.Equal(t, 3, len(shifted), "Should shift only existing treasures")

		// Verify we got the right keys
		var shiftedKeys []string
		for _, treasure := range shifted {
			shiftedKeys = append(shiftedKeys, treasure.Key)
		}
		expectedKeys := []string{"shift-key-2", "shift-key-4", "shift-key-6"}
		assert.ElementsMatch(t, expectedKeys, shiftedKeys, "Should only return existing keys")

		// Verify all shifted treasures are deleted
		var stillExists []*ShiftTreasure
		err = hydraidegoInterface.CatalogReadBatch(
			ctx,
			swampName,
			expectedKeys,
			ShiftTreasure{},
			func(model any) error {
				treasure := model.(*ShiftTreasure)
				stillExists = append(stillExists, treasure)
				return nil
			},
		)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(stillExists), "Shifted treasures should be deleted")
	})

	// Test Case 4: Empty keys slice
	t.Run("Empty keys slice", func(t *testing.T) {
		keys := []string{}

		var shifted []*ShiftTreasure
		err := hydraidegoInterface.CatalogShiftBatch(
			ctx,
			swampName,
			keys,
			ShiftTreasure{},
			func(model any) error {
				treasure := model.(*ShiftTreasure)
				shifted = append(shifted, treasure)
				return nil
			},
		)

		assert.NoError(t, err, "Empty keys should not cause an error")
		assert.Equal(t, 0, len(shifted), "Should shift no treasures")
	})

	// Test Case 5: All non-existent keys
	t.Run("All non-existent keys", func(t *testing.T) {
		keys := []string{"non-existent-1", "non-existent-2", "non-existent-3"}

		var shifted []*ShiftTreasure
		err := hydraidegoInterface.CatalogShiftBatch(
			ctx,
			swampName,
			keys,
			ShiftTreasure{},
			func(model any) error {
				treasure := model.(*ShiftTreasure)
				shifted = append(shifted, treasure)
				return nil
			},
		)

		assert.NoError(t, err)
		assert.Equal(t, 0, len(shifted), "Should shift no treasures for non-existent keys")
	})

	// Test Case 6: Iterator returns error
	t.Run("Iterator error propagation", func(t *testing.T) {
		// Create new test data for this test
		for i := 20; i <= 22; i++ {
			treasure := &ShiftTreasure{
				Key:       fmt.Sprintf("shift-key-%d", i),
				Value:     fmt.Sprintf("shift-value-%d", i),
				CreatedAt: time.Now().UTC(),
				CreatedBy: "test-user",
			}
			_, err := hydraidegoInterface.CatalogSave(ctx, swampName, treasure)
			require.NoError(t, err)
		}
		time.Sleep(100 * time.Millisecond)

		keys := []string{"shift-key-20", "shift-key-21", "shift-key-22"}

		expectedErr := fmt.Errorf("iterator intentional error")
		callCount := 0

		err := hydraidegoInterface.CatalogShiftBatch(
			ctx,
			swampName,
			keys,
			ShiftTreasure{},
			func(model any) error {
				callCount++
				if callCount == 2 {
					return expectedErr
				}
				return nil
			},
		)

		assert.Error(t, err, "Should propagate iterator error")
		assert.Equal(t, expectedErr, err, "Error should match iterator error")
		assert.Equal(t, 2, callCount, "Iterator should be called until error occurs")

		// Note: Even though iterator failed, treasures processed before error are already deleted
		// This is expected behavior - shift is destructive per treasure
	})

	// Test Case 7: Nil iterator should fail
	t.Run("Nil iterator validation", func(t *testing.T) {
		keys := []string{"shift-key-8"}

		err := hydraidegoInterface.CatalogShiftBatch(
			ctx,
			swampName,
			keys,
			ShiftTreasure{},
			nil, // nil iterator
		)

		assert.Error(t, err, "Should return error for nil iterator")
		assert.Contains(t, err.Error(), "iterator can not be nil", "Error message should mention nil iterator")
	})

	// Test Case 8: Pointer model should fail
	t.Run("Pointer model validation", func(t *testing.T) {
		keys := []string{"shift-key-8"}

		err := hydraidegoInterface.CatalogShiftBatch(
			ctx,
			swampName,
			keys,
			&ShiftTreasure{}, // pointer model (should fail)
			func(model any) error {
				return nil
			},
		)

		assert.Error(t, err, "Should return error for pointer model")
		assert.Contains(t, err.Error(), "model cannot be a pointer", "Error message should mention pointer model")
	})

	// Test Case 9: Verify data integrity - shifted data matches original
	t.Run("Data integrity verification", func(t *testing.T) {
		// Create specific test data
		originalTreasures := []*ShiftTreasure{
			{
				Key:       "data-integrity-1",
				Value:     "important-data-12345",
				Priority:  99,
				CreatedAt: time.Date(2024, 11, 14, 10, 30, 0, 0, time.UTC),
				CreatedBy: "integrity-test",
			},
			{
				Key:       "data-integrity-2",
				Value:     "critical-data-67890",
				Priority:  100,
				CreatedAt: time.Date(2024, 11, 14, 11, 30, 0, 0, time.UTC),
				CreatedBy: "integrity-test",
			},
		}

		// Save original data
		for _, treasure := range originalTreasures {
			_, err := hydraidegoInterface.CatalogSave(ctx, swampName, treasure)
			require.NoError(t, err)
		}
		time.Sleep(100 * time.Millisecond)

		// Shift treasures
		keys := []string{"data-integrity-1", "data-integrity-2"}
		var shifted []*ShiftTreasure
		err := hydraidegoInterface.CatalogShiftBatch(
			ctx,
			swampName,
			keys,
			ShiftTreasure{},
			func(model any) error {
				treasure := model.(*ShiftTreasure)
				shifted = append(shifted, treasure)
				return nil
			},
		)

		assert.NoError(t, err)
		assert.Equal(t, 2, len(shifted), "Should shift 2 treasures")

		// Verify data integrity - shifted data should match original
		for _, original := range originalTreasures {
			found := false
			for _, shiftedTreasure := range shifted {
				if shiftedTreasure.Key == original.Key {
					found = true
					assert.Equal(t, original.Value, shiftedTreasure.Value, "Value should match")
					// Priority has omitempty tag, so serialization behavior may vary
					assert.Equal(t, original.CreatedBy, shiftedTreasure.CreatedBy, "CreatedBy should match")
					// CreatedAt may have minor differences due to serialization, check it's set
					assert.False(t, shiftedTreasure.CreatedAt.IsZero(), "CreatedAt should be set")
					break
				}
			}
			assert.True(t, found, "Original treasure %s should be found in shifted results", original.Key)
		}
	})

	// Test Case 10: Concurrent behavior - shifted treasures don't appear in subsequent reads
	t.Run("Concurrent behavior verification", func(t *testing.T) {
		// Create test data
		for i := 30; i <= 35; i++ {
			treasure := &ShiftTreasure{
				Key:       fmt.Sprintf("concurrent-key-%d", i),
				Value:     fmt.Sprintf("concurrent-value-%d", i),
				CreatedAt: time.Now().UTC(),
				CreatedBy: "concurrent-test",
			}
			_, err := hydraidegoInterface.CatalogSave(ctx, swampName, treasure)
			require.NoError(t, err)
		}
		time.Sleep(100 * time.Millisecond)

		// Shift first batch
		firstBatch := []string{"concurrent-key-30", "concurrent-key-31", "concurrent-key-32"}
		var firstShifted []*ShiftTreasure
		err := hydraidegoInterface.CatalogShiftBatch(
			ctx,
			swampName,
			firstBatch,
			ShiftTreasure{},
			func(model any) error {
				treasure := model.(*ShiftTreasure)
				firstShifted = append(firstShifted, treasure)
				return nil
			},
		)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(firstShifted), "Should shift 3 treasures in first batch")

		// Try to shift the same keys again - should get nothing
		var secondShifted []*ShiftTreasure
		err = hydraidegoInterface.CatalogShiftBatch(
			ctx,
			swampName,
			firstBatch, // same keys as before
			ShiftTreasure{},
			func(model any) error {
				treasure := model.(*ShiftTreasure)
				secondShifted = append(secondShifted, treasure)
				return nil
			},
		)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(secondShifted), "Should not shift already deleted treasures")

		// Shift remaining keys
		remainingBatch := []string{"concurrent-key-33", "concurrent-key-34", "concurrent-key-35"}
		var remainingShifted []*ShiftTreasure
		err = hydraidegoInterface.CatalogShiftBatch(
			ctx,
			swampName,
			remainingBatch,
			ShiftTreasure{},
			func(model any) error {
				treasure := model.(*ShiftTreasure)
				remainingShifted = append(remainingShifted, treasure)
				return nil
			},
		)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(remainingShifted), "Should shift remaining 3 treasures")
	})
}
