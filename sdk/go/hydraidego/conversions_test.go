package hydraidego

import (
	"reflect"
	"testing"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// convertKeyValuePairToTreasure clones the fields of a KeyValuePair into a
// Treasure. It is used by the conversion round-trip tests below to simulate
// what the server returns to the client.
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

type conversionTestCase struct {
	name     string
	input    any
	expected any
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
			kv, err := convertCatalogModelToKeyValuePair(tc.input, EncodingGOB)

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
			kv, err := convertCatalogModelToKeyValuePair(tc.input, EncodingGOB)
			require.NoError(t, err)

			treasure := convertKeyValuePairToTreasure(kv)

			restored := reflect.New(reflect.TypeOf(tc.expected)).Interface()
			err = convertProtoTreasureToCatalogModel(treasure, restored)
			require.NoError(t, err)

			require.Equal(t, tc.expected, reflect.ValueOf(restored).Elem().Interface())
		})
	}
}
