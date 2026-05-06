//go:build integration

package main

import (
	"context"
	"testing"
	"time"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/internal/setup"
)

func TestTTLQueue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r, tracker := setup.NewTestClient(t)
	tracker.Track(QueueSwamp())

	if err := RunTTLQueue(ctx, r); err != nil {
		t.Fatalf("RunTTLQueue: %v", err)
	}
}
