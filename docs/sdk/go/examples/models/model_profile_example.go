//go:build ignore
// +build ignore

// This file provides an example of a complex Profile-style model used with ProfileSave(), ProfileLoad(), etc.
//
// In Profile mode, each field in the struct is stored as an independent Treasure
// (i.e. a key-value pair) within the same Swamp.
//
// 🧠 Use this pattern when you want to store logically grouped data
// — such as a user profile, a configuration object, or a settings page —
// where all fields belong together, and you typically load/save them as one unit.

package models

import "time"

// ProfileUser represents a user profile stored in a Swamp.
//
// ✅ Each struct field becomes its own key inside the Swamp
//
//	└── Key: derived from field name
//	└── Value: stored in the most optimal binary form
//
// ✅ All supported types are allowed:
//   - Primitives: string, bool, int8–64, uint8–64, float32, float64
//   - Structs (encoded with GOB)
//   - Pointers to struct
//
// ✅ If a field is tagged with `hydraide:"omitempty"`, it will be skipped during save if it's empty.
//
// ⚠️ Profile models are always saved and loaded as full units.
//   - Save() will write all non-empty fields to the Swamp
//   - Load() will populate all matching fields from the Swamp
//   - You cannot update or retrieve a single field independently
//
// ⚠️ DO NOT use `any` / `interface{}` types without a concrete underlying type!
//
//	HydrAIDE requires serializable, type-safe values. All values must have:
//	- A concrete Go type (e.g. `*MyStruct`, `map[string]int`)
//	- A known GOB encoding path (automatically handled for structs and pointers)
type ProfileUser struct {

	// UserUUID is typically used to construct the Swamp name.
	// For example: Sanctuary("users").Realm("profiles").Swamp(UserUUID)
	UserUUID string

	// Basic profile data — stored as individual Treasures
	Email string

	// Optional fields — stored only if non-empty
	Phone string `hydraide:"omitempty"`
	Age   uint8  `hydraide:"omitempty"` // Use the smallest integer types possible

	// Metadata for tracking lifecycle of the entire profile
	CreatedAt time.Time
	UpdatedAt time.Time
}
