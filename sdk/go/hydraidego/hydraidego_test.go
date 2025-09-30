package hydraidego

import (
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/hydraide/hydraide/generated/hydraidepbgo"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/client"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

var hydraidegoInterface Hydraidego
var clientInterface client.Client

func TestMain(m *testing.M) {
	fmt.Println("Setting up test environment...")
	setup() // start the testing environment
	code := m.Run()
	fmt.Println("Tearing down test environment...")
	teardown() // Stop the testing environment
	os.Exit(code)
}

func setup() {

	server := &client.Server{
		Host:          os.Getenv("HYDRAIDE_TEST_SERVER"),
		FromIsland:    0,
		ToIsland:      1000,
		CACrtPath:     os.Getenv("HYDRAIDE_CA_CRT"),
		ClientCrtPath: os.Getenv("HYDRAIDE_CLIENT_CRT"),
		ClientKeyPath: os.Getenv("HYDRAIDE_CLIENT_KEY"),
	}

	servers := []*client.Server{server}
	clientInterface = client.New(servers, 1000, 104857600)
	if err := clientInterface.Connect(false); err != nil {
		slog.Error("Failed to connect to Hydraide server", "error", err)
		os.Exit(1) // exit if the connection fails
	} else {
		slog.Info("Connected to Hydraide server successfully")
	}
	hydraidegoInterface = New(clientInterface) // creates a new hydraidego instance

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
