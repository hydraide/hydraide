// 01-quickstart — the smallest possible end-to-end demo against a live
// HydrAIDE instance.
//
//	connect → register pattern → save one record → read it back →
//	subscribe → trigger one update → exit
//
// Run from this directory:
//
//	go run .
//
// The connection is configured via HYDRA_HOST and HYDRA_CERT (see
// ../.env.example). With the in-tree docker compose stack running, no
// environment variables are required.
package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/internal/setup"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/utils/repo"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r, cleanup := setup.MustClient(ctx)
	defer cleanup()

	if err := RunQuickstart(ctx, r); err != nil {
		log.Fatalf("quickstart failed: %v", err)
	}
	fmt.Println("done.")
}

// User is the model the quickstart writes, reads, and watches.
type User struct {
	ID      string   `hydraide:"key"`
	Profile *Profile `hydraide:"value"`
}

// Profile is the msgpack-encoded payload that lives inside a User Treasure.
// Field-level patches in other recipes target paths inside this struct.
type Profile struct {
	Email string `msgpack:"email"`
	Score int64  `msgpack:"score"`
}

// QuickstartSwamp is the namespace this demo uses. The Sanctuary/Realm/Swamp
// trio is the addressing model — every recipe declares its own so they
// don't collide.
func QuickstartSwamp() name.Name {
	return name.New().Sanctuary("examples").Realm("quickstart").Swamp("users")
}

// RunQuickstart is the test-friendly entry point. The main function is a
// thin wrapper; the integration test calls this same function.
func RunQuickstart(ctx context.Context, r repo.Repo) error {
	swamp := QuickstartSwamp()

	if err := setup.Pattern(ctx, r, name.New().Sanctuary("examples").Realm("quickstart").Swamp("*")); err != nil {
		return fmt.Errorf("register pattern: %w", err)
	}

	h := r.GetHydraidego()

	// Step 1 — save a single user.
	first := &User{
		ID: "alice",
		Profile: &Profile{
			Email: "alice@example.com",
			Score: 10,
		},
	}
	status, err := h.CatalogSave(ctx, swamp, first)
	if err != nil {
		return fmt.Errorf("save: %w", err)
	}
	fmt.Printf("saved alice: %s\n", setup.EventStatusName(status))

	// Step 2 — read it back.
	got := &User{}
	if err := h.CatalogRead(ctx, swamp, "alice", got); err != nil {
		return fmt.Errorf("read: %w", err)
	}
	fmt.Printf("read alice: email=%s score=%d\n", got.Profile.Email, got.Profile.Score)

	// Step 3 — subscribe to changes, then trigger one update so the
	// subscription has something to print.
	subCtx, subCancel := context.WithCancel(ctx)
	defer subCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := h.Subscribe(subCtx, swamp, false, User{}, func(model any, eventStatus hydraidego.EventStatus, _ error) error {
			u, ok := model.(*User)
			if !ok {
				return nil
			}
			fmt.Printf("subscribe event: id=%s status=%s score=%d\n", u.ID, setup.EventStatusName(eventStatus), u.Profile.Score)
			if eventStatus == hydraidego.StatusModified {
				subCancel()
			}
			return nil
		})
		if err != nil && err != context.Canceled {
			log.Printf("subscribe stopped: %v", err)
		}
	}()

	// Give the subscription a moment to attach before we write.
	time.Sleep(200 * time.Millisecond)

	updated := &User{
		ID: "alice",
		Profile: &Profile{
			Email: "alice@example.com",
			Score: 11,
		},
	}
	if _, err := h.CatalogSave(ctx, swamp, updated); err != nil {
		return fmt.Errorf("update: %w", err)
	}
	fmt.Println("updated alice (score 10 → 11)")

	// Wait for the subscription goroutine to observe the modification and
	// exit. A short hard deadline guards against the unlikely case where
	// the event is lost.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		subCancel()
		return fmt.Errorf("subscribe did not observe the update in time")
	}

	// Step 4 — clean up so the demo can be re-run idempotently.
	if err := h.Destroy(ctx, swamp); err != nil {
		return fmt.Errorf("destroy: %w", err)
	}
	fmt.Println("destroyed quickstart swamp")

	return nil
}
