//go:build integration

package main

import (
	"context"
	"testing"
	"time"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/internal/setup"
)

func TestBusinessLock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r, _ := setup.NewTestClient(t)

	if err := RunBusinessLock(ctx, r); err != nil {
		t.Fatalf("RunBusinessLock: %v", err)
	}
}
