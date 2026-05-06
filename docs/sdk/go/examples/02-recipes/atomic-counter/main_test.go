//go:build integration

package main

import (
	"context"
	"testing"
	"time"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/internal/setup"
)

func TestAtomicCounter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r, tracker := setup.NewTestClient(t)
	tracker.Track(CounterSwamp())

	if err := RunAtomicCounter(ctx, r); err != nil {
		t.Fatalf("RunAtomicCounter: %v", err)
	}
}
