package models

import (
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// UserSanctuaryName and UserRealmName define the naming convention for user Profile Swamps.
const (
	UserSanctuaryName = "users"
	UserRealmName     = "profiles"
)

// UserProfile – Example: Complex Profile Swamp Model
//
// This example demonstrates how to define and work with a complex HydrAIDE Profile Swamp
// using typed Go structs, nested pointers, timestamps, and intelligent field-level tagging.
//
// 🔹 What is a Profile Swamp?
// A Profile Swamp is a one-to-one data container. It stores all structured data for a single entity —
// such as a user — under a single Swamp name (not a key). This means:
//
//	→ You don't need to define `hydraide:"key"` fields like in Catalogs.
//	→ The Swamp's name (usually the UserID) acts as the identity.
//
// In this example, `UserProfile` includes:
// - Strings, booleans, and typed primitives (e.g. `int32`, `float64`, `uint8`)
// - Timestamp fields for registration, login, and data tracking
// - Pointer-based nested structs: `*UserImage`, `*SecuritySettings`, `*Preferences`
// - Optional fields using `hydraide:"omitempty"` — which are only stored if non-empty
//
// Why `omitempty` matters:
// Fields tagged with `hydraide:"omitempty"` are omitted from binary storage
// if their value is zero, empty, or nil — reducing data size and boosting performance.
//
// ----------------------------------------------------
// 📥 How to Load a User's Profile:
//
//	user := &UserProfile{UserID: "12345"}
//	err := user.Load(repo) // repo implements the repo.Repo interface
//
// This will hydrate the full profile from HydrAIDE using the Swamp name "12345".
//
// ----------------------------------------------------
// 💾 How to Save or Update the Profile:
//
//	user.Email = "hello@lipsum.com"
//	user.Username = "lipsum"
//	...
//	err := user.Save(repo)
//
// Any modified field will be persisted automatically. You don’t need to diff or merge manually.
//
// ----------------------------------------------------
// 🚀 Notes:
// - Use `RegisterPattern()` ONLY ONCE, to define persistence settings for all user profiles.
// - The Swamp name is generated using the `createName()` helper.
//
// This is a great example of how to model user-facing, multi-field state cleanly
// while keeping full control over serialization, performance, and structure.
type UserProfile struct {
	UserID   string
	Email    string
	Username string

	FirstName string `hydraide:"omitempty"`
	LastName  string `hydraide:"omitempty"`

	PasswordHash string
	RegisteredAt time.Time
	LastLoginAt  *time.Time `hydraide:"omitempty"`
	UploadedAt   time.Time  `hydraide:"omitempty"`

	Age        uint8 `hydraide:"omitempty"`
	IsVerified bool
	LoginCount int32
	Rating     float64 `hydraide:"omitempty"`

	// Nested pointer structs
	Avatar      *UserImage        `hydraide:"omitempty"`
	Security    *SecuritySettings `hydraide:"omitempty"`
	Preferences *Preferences      `hydraide:"omitempty"`
}
type UserImage struct {
	URL        string
	Width      int16
	Height     int16
	UploadedAt time.Time
}

type SecuritySettings struct {
	TwoFactorEnabled   bool
	LastPasswordChange time.Time
	BackupCodesUsed    int32      `hydraide:"omitempty"`
	BlockedUntil       *time.Time `hydraide:"omitempty"`
}

type Preferences struct {
	Language             string `hydraide:"omitempty"`
	Timezone             string `hydraide:"omitempty"`
	DarkMode             bool
	NotificationsEnabled bool
}

// Load loads the full user profile from HydrAIDE into the current struct instance.
//
// This method uses the HydrAIDE ProfileRead() call to hydrate a complete Swamp
// based on the given `UserID`. In Profile Swamps, the Swamp name acts as the key —
// so we don’t query by field, but by Swamp name.
//
// 🧠 This is the standard way to retrieve a user’s full state in one call.
//
// Example:
//
//	user := &UserProfile{UserID: "abc123"}
//	err := user.Load(repo)
//
// Internally, this method:
//
// - Creates a timeout-safe context
// - Builds the Swamp name using the `UserID` and naming convention
// - Delegates the load to HydrAIDE’s ProfileRead(), which populates all fields
//
// The current instance (`m`) will be mutated in-place with all loaded values.
//
// This method is non-blocking and works with both memory-based and file-based Swamps.
// Make sure the Swamp was previously registered with `RegisterPattern()`.
func (m *UserProfile) Load(r repo.Repo) error {
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	h := r.GetHydraidego()
	return h.ProfileRead(ctx, m.createName(m.UserID), m)
}

// Save persists the current user profile into HydrAIDE.
//
// This method stores all fields of the `UserProfile` struct into a Profile Swamp,
// using the `UserID` as the Swamp name. HydrAIDE handles full serialization of the
// entire struct — including nested pointers and any `hydraide:"omitempty"` logic.
//
// Example:
//
//	user.Email = "hello@lipsum.com"
//	user.LoginCount++
//	err := user.Save(repo)
//
// 💡 Concurrency Tip:
// If you want to ensure that only one instance can write to this user's profile
// at a time (to avoid race conditions in distributed environments),
// you can use HydrAIDE’s key-based locking system.
// See: `docs/sdk/go/examples/models/basics_lock_unlock.go`
//
// Internally, this method:
//
// - Creates a timeout-bound context
// - Resolves the Swamp name using `UserID`
// - Calls `ProfileSave()` to write all data atomically into the Swamp
//
// The write is atomic from HydrAIDE’s perspective, and will overwrite the full state.
// Fields marked with `hydraide:"omitempty"` will be excluded from storage if empty.
func (m *UserProfile) Save(r repo.Repo) error {
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	h := r.GetHydraidego()
	return h.ProfileSave(ctx, m.createName(m.UserID), m)
}

// IsUserExists checks whether the user’s Profile Swamp already exists in HydrAIDE.
//
// This is useful for scenarios where you want to validate if a user has been registered,
// or to prevent duplicate Swamp creation for the same user ID.
//
// Example use case:
//
//	user := &UserProfile{UserID: "petergebri"}
//	exists, err := user.IsUserExists(repo)
//	if exists {
//	    log.Println("User already registered.")
//	}
//
// 🧠 Why this works:
// In HydrAIDE, each Profile is stored as a uniquely named Swamp.
// This function simply queries whether that Swamp (named after the UserID) exists in the system.
//
// 🔐 This check is read-only and does not load or hydrate any data.
// It’s ideal for signup flows, admin panels, or Swamp lifecycle audits.
func (m *UserProfile) IsUserExists(r repo.Repo) (bool, error) {
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	h := r.GetHydraidego()
	return h.IsSwampExist(ctx, m.createName(m.UserID))
}

// Destroy permanently deletes the user’s Profile Swamp from HydrAIDE.
//
// ⚠️ This operation is **destructive** and should typically only be used
// for testing, cleanup, or reset purposes — not in production flows.
//
// Once executed, all data associated with this user’s Swamp is lost and
// cannot be recovered unless backed up externally.
//
// Example usage:
//
//	user := &UserProfile{UserID: "abc123"}
//	err := user.Destroy(repo)
//
// Internally, this method:
//
// - Builds a Swamp name from the `UserID`
// - Sends a `Destroy()` call to HydrAIDE to fully delete the Swamp and its contents
//
// 🧪 Use case: Unit tests, CI/CD setup teardown, temporary data cleanup,
// or development tools that require a reset-to-zero operation.
func (m *UserProfile) Destroy(r repo.Repo) error {
	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	h := r.GetHydraidego()
	return h.Destroy(ctx, m.createName(m.UserID))
}

// RegisterPattern registers the Profile Swamp pattern for all users.
//
// This method should be called **once** during application startup
// to configure how HydrAIDE should manage all user profile Swamps.
//
// In this example, we use a wildcard pattern (`Swamp("*")`) to apply
// the same settings to every user, such as:
// - Auto-closing after 5 seconds of inactivity
// - Immediate write to disk (`WriteInterval: 1s`)
// - Max file size of 1MB (suitable for rarely-changing full profiles)
//
// 🧠 Tip: You can tune these settings per environment (e.g. dev vs prod),
// but one pattern registration is enough per Swamp type.
func (m *UserProfile) RegisterPattern(repo repo.Repo) error {
	h := repo.GetHydraidego()

	ctx, cancelFunc := hydraidehelper.CreateHydraContext()
	defer cancelFunc()

	// RegisterSwamp always returns a []error.
	// Each error (if any) represents a failure during Swamp registration on a HydrAIDE server.
	//
	// ⚠️ Even when only a single Swamp pattern is registered, HydrAIDE may attempt to replicate or validate
	// the pattern across multiple server nodes (depending on your cluster).
	//
	// ➕ Return behavior:
	// - If all servers succeeded → returns nil
	// - If one or more servers failed → returns a non-nil []error
	//
	// 🧠 To convert this into a single `error`, you can use the helper:
	//     hydraidehelper.ConcatErrors(errorResponses)
	errorResponses := h.RegisterSwamp(ctx, &hydraidego.RegisterSwampRequest{
		SwampPattern:    name.New().Sanctuary(UserSanctuaryName).Realm(UserRealmName).Swamp("*"),
		CloseAfterIdle:  time.Second * 5,
		IsInMemorySwamp: false,
		FilesystemSettings: &hydraidego.SwampFilesystemSettings{
			WriteInterval: time.Second * 1,
			MaxFileSize:   1048576,
		},
	})

	if errorResponses != nil {
		return hydraidehelper.ConcatErrors(errorResponses)
	}
	return nil
}

// createName generates a fully qualified Swamp name for a given user.
//
// This method ensures that every `UserProfile` is consistently mapped
// to its own unique Profile Swamp using the naming hierarchy:
//   - Sanctuary: global service group (e.g. "user")
//   - Realm:     subdomain or functional context (e.g. "profiles")
//   - Swamp:     the actual user ID or username
//
// By standardizing name generation, this ensures compatibility across
// save/load/delete operations and makes subscription or analytics easier to scope.
func (m *UserProfile) createName(username string) name.Name {
	return name.New().Sanctuary(UserSanctuaryName).Realm(UserRealmName).Swamp(username)
}
