// 02-recipes/profile-per-user — Swamp-per-user profile storage.
//
// HydrAIDE has two natural shapes for typed records:
//
//   - **Catalog**: many Treasures inside one Swamp. Use when listing,
//     filtering or paginating across records is the primary operation.
//   - **Profile**: one record per Swamp, each struct field stored as its
//     own Treasure inside that Swamp. Use when each record is an
//     independent unit with its own lifecycle, lock domain and disk file.
//
// Per-user profiles fit Profile cleanly: one Swamp per user means each
// user's data evicts independently, locks independently, and disappears
// completely when the user is deleted (no orphan rows in a shared table).
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/internal/setup"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r, cleanup := setup.MustClient(ctx)
	defer cleanup()

	if err := RunProfilePerUser(ctx, r); err != nil {
		log.Fatalf("profile-per-user failed: %v", err)
	}
	fmt.Println("done.")
}

// UserProfile is the profile model. Each field becomes its own Treasure
// inside the user's Swamp.
type UserProfile struct {
	DisplayName string `hydraide:"DisplayName"`
	Email       string `hydraide:"Email"`
	Score       int64  `hydraide:"Score"`
	IsBanned    bool   `hydraide:"IsBanned"`
}

// UserSwamp computes the per-user Swamp name. Pattern registration
// happens against `users/profiles/*` so any user ID slots in.
func UserSwamp(userID string) name.Name {
	return name.New().Sanctuary("examples").Realm("profiles").Swamp(userID)
}

// RunProfilePerUser saves two users into separate Swamps, reads one back,
// updates a single field, and destroys both Swamps.
func RunProfilePerUser(ctx context.Context, r repo.Repo) error {
	if err := setup.Pattern(ctx, r,
		name.New().Sanctuary("examples").Realm("profiles").Swamp("*")); err != nil {
		return fmt.Errorf("register pattern: %w", err)
	}

	h := r.GetHydraidego()

	users := []*UserProfile{
		{DisplayName: "Alice", Email: "alice@example.com", Score: 10, IsBanned: false},
		{DisplayName: "Bob", Email: "bob@example.com", Score: 20, IsBanned: false},
	}
	swamps := []name.Name{UserSwamp("alice"), UserSwamp("bob")}

	defer func() {
		for _, s := range swamps {
			_ = h.Destroy(ctx, s)
		}
	}()

	for i, u := range users {
		if err := h.ProfileSave(ctx, swamps[i], u); err != nil {
			return fmt.Errorf("save %s: %w", swamps[i].Get(), err)
		}
		fmt.Printf("saved profile %s\n", swamps[i].Get())
	}

	got := &UserProfile{}
	if err := h.ProfileRead(ctx, swamps[0], got); err != nil {
		return fmt.Errorf("read alice: %w", err)
	}
	fmt.Printf("read alice: name=%s email=%s score=%d banned=%t\n",
		got.DisplayName, got.Email, got.Score, got.IsBanned)

	// Update a single field. ProfileSave is idempotent: only changed
	// Treasures inside the Swamp actually move on disk.
	got.Score = 99
	if err := h.ProfileSave(ctx, swamps[0], got); err != nil {
		return fmt.Errorf("update alice: %w", err)
	}
	fmt.Printf("updated alice score: 10 → %d\n", got.Score)

	return nil
}
