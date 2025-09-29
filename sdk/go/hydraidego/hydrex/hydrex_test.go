package hydrex

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/client"
	"github.com/stretchr/testify/assert"
)

var hydraidegoInterface hydraidego.Hydraidego
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
	if err := clientInterface.Connect(true); err != nil {
		slog.Error("Failed to connect to Hydraide server", "error", err)
		os.Exit(1) // exit if the connection fails
	} else {
		slog.Info("Connected to Hydraide server successfully")
	}
	hydraidegoInterface = hydraidego.New(clientInterface) // creates a new hydraidego instance

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

// TestIndex tests the indexing functionality of the Hydrex interface.
// It saves initial test data, verifies core data and index data retrieval,
// modifies the data to test index updates, and checks the results accordingly.
func TestIndex(t *testing.T) {

	testIndexName := "categoryTestIndex"
	testDomain := "trendizz.com"

	// Initial test data to save in Hydrex
	testData := map[string]*CoreData{
		"category1": {
			Key:   "category1",
			Value: "test value",
		},
		"category2": {
			Key:   "category2",
			Value: "test value",
		},
		"category3": {
			Key:   "category3",
			Value: "test value",
		},
	}

	hydrexInterface := New(hydraidegoInterface)
	hydrexInterface.Save(context.Background(), testIndexName, testDomain, testData)
	// Destroy core data after the test
	defer hydrexInterface.Destroy(context.Background(), testIndexName, testDomain)

	// Retrieve and verify the core data
	coreData := hydrexInterface.GetCoreData(context.Background(), testIndexName, testDomain)
	assert.Equal(t, len(coreData), 3)

	for key, value := range coreData {
		fmt.Println(key, value.Key, value.Value, value.CreatedAt)
	}

	// Retrieve and verify the index data for each category key
	for i := 1; i <= 3; i++ {

		categoryName := fmt.Sprintf("category%d", i)

		elements := hydrexInterface.GetIndexData(context.Background(), testIndexName, categoryName)
		assert.Equal(t, 1, len(elements), "Element count is not equal")
		if len(elements) > 0 {
			assert.Equal(t, testDomain, elements[0].Domain, "Domain is not equal")
		}

	}

	// Modify the data to test index updates
	// New test data: category1 and category3 remain, category2 is removed, category4 is added
	modifiedTestData := map[string]*CoreData{
		"category1": { // still exists
			Key:   "category1",
			Value: "test value",
		},
		"category4": { // newly added, category2 is removed
			Key:   "category2",
			Value: "test value",
		},
		"category3": { // not changed
			Key:   "category3",
			Value: "test value",
		},
	}

	hydrexInterface.Save(context.Background(), testIndexName, testDomain, modifiedTestData)

	// Retrieve and verify the updated core data
	coreData = hydrexInterface.GetCoreData(context.Background(), testIndexName, testDomain)
	assert.Equal(t, len(coreData), 3)
	for key, value := range coreData {
		fmt.Println(key, value.Key, value.Value, value.CreatedAt)
	}

	// Verify index data for category1
	elements := hydrexInterface.GetIndexData(context.Background(), testIndexName, "category1")
	assert.Equal(t, 1, len(elements), "Element count is not equal")
	if len(elements) > 0 {
		assert.Equal(t, testDomain, elements[0].Domain, "Domain is not equal")
	}

	// Verify that category2 no longer exists
	elements = hydrexInterface.GetIndexData(context.Background(), testIndexName, "category2")
	assert.Equal(t, 0, len(elements), "Element count is not equal")

	// Verify index data for category3
	elements = hydrexInterface.GetIndexData(context.Background(), testIndexName, "category3")
	assert.Equal(t, 1, len(elements), "Element count is not equal")
	if len(elements) > 0 {
		assert.Equal(t, testDomain, elements[0].Domain, "Domain is not equal")
	}

	// Verify index data for newly added category4
	elements = hydrexInterface.GetIndexData(context.Background(), testIndexName, "category4")
	assert.Equal(t, 1, len(elements), "Element count is not equal")
	if len(elements) > 0 {
		assert.Equal(t, testDomain, elements[0].Domain, "Domain is not equal")
	}

}
