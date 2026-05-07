// 02-recipes/atomic-patch — atomic field-level mutation on a msgpack
// Treasure using PatchTreasures.
//
// The classical alternative is read-modify-write: GET the row, change a
// field client-side, PUT the whole row back. That pattern races under
// concurrent writers and forces every update to ship the entire payload.
// PatchTreasures sends only the field path and the new value; the engine
// applies the SET op atomically under the per-key guard.
//
// This recipe walks through:
//
//  1. Save a User Treasure with a nested Profile.
//  2. Toggle a single boolean field with CatalogPatchField.
//  3. Patch several fields in one round-trip with CatalogPatchFields.
//  4. Read the result and confirm the rest of the payload is untouched.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hydraide/hydraide/docs/sdk/go/examples/internal/setup"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/utils/repo"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r, cleanup := setup.MustClient(ctx)
	defer cleanup()

	if err := RunAtomicPatch(ctx, r); err != nil {
		log.Fatalf("atomic-patch failed: %v", err)
	}
	fmt.Println("done.")
}

// User is the Treasure model; Profile is the patchable msgpack body.
type User struct {
	ID      string   `hydraide:"key"`
	Profile *Profile `hydraide:"value"`
}

type Profile struct {
	Email      string `msgpack:"email"`
	Score      int64  `msgpack:"score"`
	IsVerified bool   `msgpack:"isVerified"`
	LoginCount int64  `msgpack:"loginCount"`
}

// PatchSwamp is the namespace for this recipe.
func PatchSwamp() name.Name {
	return name.New().Sanctuary("examples").Realm("atomic-patch").Swamp("users")
}

// RunAtomicPatch is the integration-test-friendly entry point.
func RunAtomicPatch(ctx context.Context, r repo.Repo) error {
	swamp := PatchSwamp()

	if err := setup.Pattern(ctx, r, name.New().Sanctuary("examples").Realm("atomic-patch").Swamp("*")); err != nil {
		return fmt.Errorf("register pattern: %w", err)
	}

	h := r.GetHydraidego()
	_ = h.Destroy(ctx, swamp) // clean re-runs

	// Step 1 — save the initial Treasure.
	initial := &User{
		ID: "alice",
		Profile: &Profile{
			Email:      "alice@example.com",
			Score:      10,
			IsVerified: false,
			LoginCount: 0,
		},
	}
	if _, err := h.CatalogSave(ctx, swamp, initial); err != nil {
		return fmt.Errorf("save: %w", err)
	}
	fmt.Println("saved alice (verified=false, score=10, loginCount=0)")

	// Step 2 — flip a single field. No round-trip read.
	status, err := h.CatalogPatchField(ctx, swamp, "alice", "isVerified", true)
	if err != nil {
		return fmt.Errorf("patch isVerified: %w", err)
	}
	fmt.Printf("patched isVerified → true (status=%s)\n", status)

	// Step 3 — patch multiple fields atomically in one call. The map's
	// iteration order is non-deterministic, but the engine applies every
	// op under one per-key guard so observers never see a half-applied
	// state.
	status, err = h.CatalogPatchFields(ctx, swamp, "alice", map[string]any{
		"score":      int64(42),
		"loginCount": int64(1),
	})
	if err != nil {
		return fmt.Errorf("patch fields: %w", err)
	}
	fmt.Printf("patched score=42, loginCount=1 (status=%s)\n", status)

	// Step 4 — read back and confirm: the email is untouched, the patched
	// fields reflect the new values.
	got := &User{}
	if err := h.CatalogRead(ctx, swamp, "alice", got); err != nil {
		return fmt.Errorf("read: %w", err)
	}
	fmt.Printf("read alice: email=%s verified=%t score=%d loginCount=%d\n",
		got.Profile.Email, got.Profile.IsVerified, got.Profile.Score, got.Profile.LoginCount)

	if got.Profile.Email != "alice@example.com" {
		return fmt.Errorf("email was modified by patches: %q", got.Profile.Email)
	}
	if !got.Profile.IsVerified || got.Profile.Score != 42 || got.Profile.LoginCount != 1 {
		return fmt.Errorf("patched fields not reflected: %+v", got.Profile)
	}

	if err := h.Destroy(ctx, swamp); err != nil {
		return fmt.Errorf("destroy: %w", err)
	}
	fmt.Println("destroyed atomic-patch swamp")
	return nil
}
