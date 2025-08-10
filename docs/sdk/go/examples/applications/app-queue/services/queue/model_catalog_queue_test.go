package queue

import (
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego/client"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type TestModelCatalogQueue struct {
	suite.Suite
	repoInterface repo.Repo
}

func (s *TestModelCatalogQueue) SetupSuite() {

	// Connect to the actual Hydra test database
	// Set the "connection analysis" parameter to true during testing (at least for the first time)
	// to verify whether the connection is properly established.
	s.repoInterface = repo.New([]*client.Server{
		{
			// The Hydraidego SDK requires the server address where HydrAIDE is running.
			// This can be a hostname or IP address with port, e.g., "localhost:5444".
			// If you're using Docker and mapped the internal 4444 port to 5444 on the host,
			// then use "localhost:5444" or "remote-ip:5444" as the HYDRA_HOST value.
			Host: os.Getenv("HYDRA_HOST"),

			// FromIsland and ToIsland are required parameters for the Hydraidego SDK.
			// They define the island (folder range) this server is responsible for.
			// If you only have one server, you can use a default range of 100.
			FromIsland: 1,
			ToIsland:   100,

			// This is the path to the client certificate file (typically ca.crt),
			// which is required by Hydraidego to establish a secure TLS connection.
			// The value of HYDRA_CERT must include the full filename and extension, e.g.:
			// "/etc/hydraide/certs/ca.crt"
			// For generating the certificate, refer to the install guide and use
			// the official script provided during HydrAIDE install-scripts.
			CertFilePath: os.Getenv("HYDRA_CERT"),
		},
	}, 100, 10485760, false)

	// Register the model in the HydraIDE test database that we want to test
	mcq := &ModelCatalogQueue{}
	err := mcq.RegisterPattern(s.repoInterface)
	assert.Nil(s.T(), err)

	// Ensure a clean test state by deleting the queue named "testQueue" if it exists
	if err := mcq.DestroyQueue(s.repoInterface, "testQueue"); err != nil {
		slog.Error("failed to destroy queue",
			"error", err,
		)
	}
}

func (s *TestModelCatalogQueue) TearDownSuite() {
	// Destroy the test Swamp at the end of the test suite
	nonExpiredTask := &ModelCatalogQueue{}
	// Delete the entire queue named "testQueue"
	err := nonExpiredTask.DestroyQueue(s.repoInterface, "testQueue")
	assert.Nil(s.T(), err)
}

func (s *TestModelCatalogQueue) TestQueueOperations() {

	// creates 5 new tasks. 2 of them are expired, 3 of them are not.
	tasks := []*ModelCatalogQueue{
		{TaskUUID: "task1", TaskData: []byte(`{"info": "data1"}`), ExpireAt: time.Now().Add(-time.Minute * 2)},
		{TaskUUID: "task2", TaskData: []byte(`{"info": "data2"}`), ExpireAt: time.Now().Add(-time.Minute * 1)},
		{TaskUUID: "task3", TaskData: []byte(`{"info": "data3"}`), ExpireAt: time.Now().Add(time.Hour)},
		{TaskUUID: "task4", TaskData: []byte(`{"info": "data4"}`), ExpireAt: time.Now().Add(2 * time.Hour)},
		{TaskUUID: "task5", TaskData: []byte(`{"info": "data5"}`), ExpireAt: time.Now().Add(3 * time.Hour)},
	}

	queueName := "testQueue"

	// ssave tasks to the queue
	for _, task := range tasks {
		err := task.Save(s.repoInterface, queueName)
		assert.Nil(s.T(), err)
	}

	counter := &ModelCatalogQueue{}
	// Count the number of tasks in the queue
	count := counter.Count(s.repoInterface, queueName)
	assert.Equal(s.T(), 5, count)

	// load expired tasks from the queue
	loadedTask := &ModelCatalogQueue{}
	tasks, err := loadedTask.LoadExpired(s.repoInterface, queueName, 0)

	for _, task := range tasks {
		slog.Info("task loaded",
			"taskUUID", task.TaskUUID,
			"taskData", task.TaskData,
		)
	}

	assert.Equal(s.T(), 2, len(tasks))
	assert.Nil(s.T(), err)

}

func (s *TestModelCatalogQueue) TestManyInserts() {

	allTests := 20

	// create many tasks
	tasks := make([]*ModelCatalogQueue, 0)
	for i := 0; i < allTests; i++ {

		task := &ModelCatalogQueue{
			TaskUUID: fmt.Sprintf("task-%d", i),
			TaskData: []byte(`{"info": "data` + fmt.Sprintf("%d", i) + `"}`),
			ExpireAt: time.Now(),
		}

		err := task.Save(s.repoInterface, "testQueue")

		assert.Nil(s.T(), err)

		slog.Info("task created",
			"taskUUID", task.TaskUUID,
			"taskData", task.TaskData,
			"expireAt", task.ExpireAt,
			"allTasks", task.Count(s.repoInterface, "testQueue"),
		)

		time.Sleep(time.Second * 1)

	}

	// load all the expired tasks from the queue
	loadedTask := &ModelCatalogQueue{}
	tasks, err := loadedTask.LoadExpired(s.repoInterface, "testQueue", 0)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), allTests, len(tasks))

	slog.Info("all tasks loaded successfully", "allTasksInDatabase", len(tasks))

}

func TestModelCatalogQueueSuite(t *testing.T) {
	suite.Run(t, new(TestModelCatalogQueue))
}
