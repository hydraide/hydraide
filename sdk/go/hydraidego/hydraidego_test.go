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
		Host:         os.Getenv("HYDRA_HOST"),
		FromIsland:   0,
		ToIsland:     1000,
		CertFilePath: os.Getenv("HYDRA_CERT"),
	}

	servers := []*client.Server{server}
	clientInterface = client.New(servers, 1000, 104857600)
	if err := clientInterface.Connect(true); err != nil {
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
