package queue

import (
	"fmt"
	"github.com/hydraide/hydraide/example-applications/go/webapp/utils/repo"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/client"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"log/slog"
	"os"
	"testing"
	"time"
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
			// The Hydraidego SDK requires the server address where HydrAIDE is running
			Host: os.Getenv("HYDRA_HOST"),

			// FromIsland and ToIsland are required parameters for the Hydraidego SDK.
			// They define the island (folder range) this server is responsible for.
			// If you only have one server, you can use a default range of 100.
			FromIsland: 1,
			ToIsland:   100,

			// This is the path to the client certificate (typically ca.crt),
			// which is required by Hydraidego for establishing a secure connection.
			// To generate the certificate, refer to the install guide and use
			// the certificate generation script provided with the installation.
			CertFilePath: os.Getenv("HYDRA_CERT"),
		},
	}, 100, 10485760, true)

	s.queueName = "queueServiceTestQueue"
	mcu := &ModelCatalogQueue{}
	if err := mcu.DestroyQueue(s.repoInterface, s.queueName); err != nil {
		log.Fatal(err)
	}

}

func (s *TestQueueService) tearDownSuite() {

	mcu := &ModelCatalogQueue{}
	// destroy test queue after the tests
	if err := mcu.DestroyQueue(s.repoInterface, s.queueName); err != nil {
		log.Fatal(err)
	}

}

func (s *TestQueueService) TestQueueService() {

	qs := New(s.repoInterface)

	type Task struct {
		Command  string
		MaxToken string
		Message  string
	}

	// Létrehozunk 5 feladatot, 2 lejárt és 3 nem lejárt határidővel
	tasks := []*Task{
		{"first command", "1", "first message"},
		{"second command", "2", "second message"},
		{"third command", "3", "third message"},
	}

	// Feladatok mentése a queue-ba
	for _, task := range tasks {
		taskId, err := qs.Add(s.queueName, task, time.Now())
		assert.NotNil(s.T(), taskId)
		assert.Nil(s.T(), err)
	}

	time.Sleep(2000 * time.Millisecond)

	loadedTasks, err := qs.Get(s.queueName, Task{}, 2)
	assert.Equal(s.T(), 2, len(loadedTasks))
	assert.Nil(s.T(), err)

	for taskUUID, task := range loadedTasks {
		fmt.Println(taskUUID, task)
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
			"first message",
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

	log.WithFields(log.Fields{
		"queueSize": qs.GetSize(s.queueName),
	}).Info("added all tasks successfully")

	// try to ge all tasks from the queue
	loadedTasks, err := qs.Get(s.queueName, Task{}, allTasks)
	assert.Equal(s.T(), allTasks, len(loadedTasks))
	assert.Nil(s.T(), err)

	// count the tasks
	size := qs.GetSize(s.queueName)
	assert.Equal(s.T(), 0, size)

}

func TestQueueServiceFunc(t *testing.T) {
	suite.Run(t, new(TestQueueService))
}
