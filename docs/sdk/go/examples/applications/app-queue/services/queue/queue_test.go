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

type TestQueueService struct {
	suite.Suite
	repoInterface repo.Repo
	queueName     string
}

func (s *TestQueueService) SetupSuite() {

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

	// create a name for the queue
	s.queueName = "queueServiceTestQueue"
	// destroy the queue if it exists
	mcu := &ModelCatalogQueue{}
	if err := mcu.DestroyQueue(s.repoInterface, s.queueName); err != nil {
		slog.Error("Failed to destroy test queue", "error", err)
	}

}

func (s *TestQueueService) tearDownSuite() {

	mcu := &ModelCatalogQueue{}
	// destroy test queue after the tests
	if err := mcu.DestroyQueue(s.repoInterface, s.queueName); err != nil {
		slog.Error("Failed to destroy test queue", "error", err)
		panic(fmt.Sprintf("Failed to destroy test queue %s", s.queueName))
	}

}

func (s *TestQueueService) TestQueueService() {

	qs := New(s.repoInterface)

	type Task struct {
		Command  string
		MaxToken string
		Message  string
	}

	// create 3 tasks
	tasks := []*Task{
		{"first command", "1", "first message"},
		{"second command", "2", "second message"},
		{"third command", "3", "third message"},
	}

	// save tasks into the queue
	for _, task := range tasks {
		taskId, err := qs.Add(s.queueName, task, time.Now())
		assert.NotNil(s.T(), taskId)
		assert.Nil(s.T(), err)
	}

	// wait for the task expiration to 2 seconds
	// it means, there will be 2 expired tasks and 1 non-expired task
	time.Sleep(2 * time.Second)

	loadedTasks, err := qs.Get(s.queueName, Task{}, 2)
	assert.Equal(s.T(), 2, len(loadedTasks))
	assert.Nil(s.T(), err)

	for taskUUID, task := range loadedTasks {
		assert.NotEmpty(s.T(), taskUUID)
		assert.NotNil(s.T(), task)
		// check if the task type is *Task
		_, ok := task.(*Task)
		assert.True(s.T(), ok)
	}

}

func (s *TestQueueService) TestQueueServiceAddMany() {

	qs := New(s.repoInterface)

	allTasks := int32(20)

	type Task struct {
		Command  string
		MaxToken string
		Message  string
	}

	for i := int32(1); i <= allTasks; i++ {

		task := &Task{
			fmt.Sprintf("task %d", i),
			"1",
			fmt.Sprintf("message %d", i),
		}

		taskUUID, err := qs.Add(s.queueName, task, time.Now())

		assert.NotNil(s.T(), taskUUID)
		assert.Nil(s.T(), err)

		slog.Info("adding task",
			"iteration", i,
			"taskUUID", taskUUID,
			"queueSize", qs.GetSize(s.queueName),
		)

		time.Sleep(time.Second * 1)

	}

	slog.Info("added all tasks successfully", "queueSize", qs.GetSize(s.queueName))

	// try to ge all tasks from the queue
	loadedTasks, err := qs.Get(s.queueName, Task{}, allTasks)
	assert.Equal(s.T(), allTasks, int32(len(loadedTasks)))
	assert.Nil(s.T(), err)

	// count the tasks
	size := qs.GetSize(s.queueName)
	assert.Equal(s.T(), 0, size)

}

func TestQueueServiceFunc(t *testing.T) {
	suite.Run(t, new(TestQueueService))
}
