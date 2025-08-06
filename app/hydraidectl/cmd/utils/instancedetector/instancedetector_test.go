package instancedetector

import (
	"context"
	"fmt"
	"testing"
)

// This test does not validate the output yet. To-do: Improve tests.
func TestListInstances(t *testing.T) {
	detector, err := NewDetector()
	if err != nil {
		t.Error(err)
	}

	instances, err := detector.ListInstances(context.TODO())
	if err != nil {
		t.Error(err)
	}

	for _, instance := range instances {
		fmt.Println("Name: ", instance.Name, "\nStatus: ", instance.Status)
	}
}
