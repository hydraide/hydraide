// This file provides a detailed example of a catalog-style model used with CatalogCreate().
// It explains required fields, supported types, optional metadata, and best practices.

package models

import "time"

// Example: CatalogCreditLog — a catalog model for logging credit operations per user.
//
// This struct demonstrates how to define a model for CatalogCreate.
// Each field uses `hydraide` tags to indicate its role within the KeyValuePair.
// All values will be transformed into HydrAIDE-compatible binary format at runtime.

type CatalogCreditLog struct {
	// 🔑 REQUIRED
	// This will be used as the Treasure key.
	// Must be a non-empty string.
	UserUUID string `hydraide:"key"`

	// 📦 OPTIONAL — The value of the Treasure.
	// Can be:
	// - Primitive types: string, bool, int8–64, uint8–64, float32, float64
	// - Pointer to struct (also GOB-encoded)
	//
	// ⚠️ Use the SMALLEST type possible for space efficiency.
	//
	// ⚠️ DO NOT use `any` / `interface{}` types without a concrete underlying type!
	//    HydrAIDE requires serializable, type-safe values. All values must have:
	//    - A concrete Go type (e.g. `*MyStruct`, `map[string]int`)
	//    - A known GOB encoding path (automatically handled for structs and pointers)
	//
	// ❌ This will NOT work:
	//     Value any `hydraide:"value"`               // ❌ rejected: type unknown at runtime
	//	   Value MyStruct  `hydraide:"value"`         // ❌ rejected: simple struct value without pointer type
	//
	// ✅ This will work:
	//     Value *MyStruct `hydraide:"value"`         // ✅ pointer to struct
	//     Value string     `hydraide:"value"`        // ✅ primitive
	//
	// 💡 If you need to store dynamic or unknown structure data:
	//    - Serialize it to JSON and store it as a string:
	//         Value string `hydraide:"value"`  // JSON string payload
	//    - Or encode it into a custom binary format and store it as []byte:
	//         Value []byte `hydraide:"value"`  // custom binary blob
	//
	// ❗ HydrAIDE does not support raw interface{} storage — values must always be strongly typed.
	Log *Log `hydraide:"value"`

	// ⏳ OPTIONAL
	// The logical expiration timestamp of this Treasure.
	//
	// When set, this field indicates when the data is considered "expired"
	// and can be queried or extracted using expiration-based operations.
	//
	// ❗IMPORTANT — HydrAIDE DOES NOT auto-delete expired data!
	//   - HydrAIDE does NOT automatically remove Treasures when ExpireAt is reached
	//   - Instead, it provides tools to query or extract expired data on-demand:
	//     * Query expired data separately using filter operations
	//     * Use Shift operations (e.g., CatalogShiftExpired) to atomically query AND remove expired items
	//   - This makes ExpireAt a powerful, unique metadata field for:
	//     * Task scheduling and time-based workflows
	//     * Deferred data processing
	//     * Manual or batch-based cleanup strategies
	//
	// ⏰ Timezone handling:
	//   - Must be a valid, non-zero `time.Time`
	//   - Strongly recommended to set it in **UTC**, e.g., using `time.Now().UTC()`
	//   - HydrAIDE internally compares expiration using `time.Now().UTC()`
	//   - If the given value is in a different timezone, HydrAIDE will automatically convert it to UTC,
	//     but relying on implicit conversion is discouraged to avoid logic errors or timezone drift
	//
	// ✅ Example:
	//   ExpireAt: time.Now().UTC().Add(10 * time.Minute)
	//
	// If omitted or zero, this Treasure is considered non-expirable.
	ExpireAt time.Time `hydraide:"expireAt,omitempty"`

	// 🧾 OPTIONAL METADATA — useful for tracking/audit purposes
	// If omitted (with `omitempty` tag), these fields will not be included in the stored record.
	//
	// ❗IMPORTANT — Non-nullable constraint:
	//   - If you remove `omitempty` from any of these fields (CreatedAt, UpdatedAt, ExpireAt),
	//     you MUST provide a valid value at creation time
	//   - Once set, these fields CANNOT be nullified or zeroed in subsequent updates
	//   - This ensures data integrity and prevents accidental loss of audit information
	//
	// ✅ Example:
	//   With `omitempty`:    field is optional, can be omitted
	//   Without `omitempty`: field is REQUIRED, must always have a valid value

	CreatedBy string    `hydraide:"createdBy,omitempty"` // Who created the record
	CreatedAt time.Time `hydraide:"createdAt,omitempty"` // When it was created
	UpdatedBy string    `hydraide:"updatedBy,omitempty"` // Who last updated it
	UpdatedAt time.Time `hydraide:"updatedAt,omitempty"` // When it was last updated
}

type Log struct {
	Amount   int16  // ✅ Small integer: better memory & disk usage than int
	Reason   string // Reason for the credit log (e.g. "bonus")
	Currency string // Currency ISO code (e.g. "HUF", "EUR")
}
