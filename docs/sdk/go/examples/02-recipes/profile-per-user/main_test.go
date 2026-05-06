//go:build integration

package main

import (
	"context"
	"testing"
	"time"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/internal/setup"
)

func TestProfilePerUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r, tracker := setup.NewTestClient(t)
	tracker.Track(UserSwamp("alice"))
	tracker.Track(UserSwamp("bob"))

	if err := RunProfilePerUser(ctx, r); err != nil {
		t.Fatalf("RunProfilePerUser: %v", err)
	}
}
