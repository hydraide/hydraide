package hydrex

import (
	"context"
	"fmt"
	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/client"
	"github.com/stretchr/testify/assert"
	"log/slog"
	"os"
	"testing"
	"time"
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
		Host:         "",
		FromIsland:   0,
		ToIsland:     0,
		CertFilePath: "",
	}

	servers := []*client.Server{server}
	clientInterface = client.New(servers, 1000, 104857600)
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

func TestIndex(t *testing.T) {

	testIndexName := "categoryTestIndex"
	testDomain := "trendizz.com"

	// data to hydrex
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
	// destroy core data after the test
	defer hydrexInterface.Destroy(context.Background(), testIndexName, testDomain)

	// try to get the Core Data
	coreData := hydrexInterface.GetCoreData(context.Background(), testIndexName, testDomain)
	assert.Equal(t, len(coreData), 3)

	for key, value := range coreData {
		fmt.Println(key, value.Key, value.Value, value.CreatedAt)
	}

	// get the index data for the category keys
	for i := 1; i <= 3; i++ {

		categoryName := fmt.Sprintf("category%d", i)

		elements := hydrexInterface.GetIndexData(context.Background(), testIndexName, categoryName)
		assert.Equal(t, 1, len(elements), "Element count is not equal")
		if len(elements) > 0 {
			assert.Equal(t, testDomain, elements[0].Domain, "Domain is not equal")
		}

	}

	// módosítunk az adatokon, hogy tezsteljük az indexeket
	// data to hydrex
	modifiedTestData := map[string]*CoreData{
		"category1": { // still exists
			Key:   "category1",
			Value: "test value",
		},
		"category4": { // category 4 newly added category 2 is removed
			Key:   "category2",
			Value: "test value",
		},
		"category3": { // not changed
			Key:   "category3",
			Value: "test value",
		},
	}

	hydrexInterface.Save(context.Background(), testIndexName, testDomain, modifiedTestData)

	// get the core data
	coreData = hydrexInterface.GetCoreData(context.Background(), testIndexName, testDomain)
	assert.Equal(t, len(coreData), 3)
	for key, value := range coreData {
		fmt.Println(key, value.Key, value.Value, value.CreatedAt)
	}

	// get the index data for the category keys
	elements := hydrexInterface.GetIndexData(context.Background(), testIndexName, "category1")
	assert.Equal(t, 1, len(elements), "Element count is not equal")
	if len(elements) > 0 {
		assert.Equal(t, testDomain, elements[0].Domain, "Domain is not equal")
	}

	// category2 not exists anymore
	elements = hydrexInterface.GetIndexData(context.Background(), testIndexName, "category2")
	assert.Equal(t, 0, len(elements), "Element count is not equal")

	// category3 exists
	elements = hydrexInterface.GetIndexData(context.Background(), testIndexName, "category3")
	assert.Equal(t, 1, len(elements), "Element count is not equal")
	if len(elements) > 0 {
		assert.Equal(t, testDomain, elements[0].Domain, "Domain is not equal")
	}

	// category4 newly added
	elements = hydrexInterface.GetIndexData(context.Background(), testIndexName, "category4")
	assert.Equal(t, 1, len(elements), "Element count is not equal")
	if len(elements) > 0 {
		assert.Equal(t, testDomain, elements[0].Domain, "Domain is not equal")
	}

}
