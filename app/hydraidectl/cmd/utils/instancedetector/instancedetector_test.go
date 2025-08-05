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
	fmt.Println(instances)

	if err != nil {
		t.Error(err)
	}
}
