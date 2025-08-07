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
	// - Structs (encoded via GOB)
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
	//
	// ✅ This will work:
	//     Value *MyStruct `hydraide:"value"`         // ✅ pointer to struct
	//     Value MyStruct  `hydraide:"value"`         // ✅ struct value
	//     Value string     `hydraide:"value"`        // ✅ primitive
	//
	// 💡 If you need to store dynamic or unknown structure data:
	//    - Serialize it to JSON and store it as a string:
	//         Value string `hydraide:"value"`  // JSON string payload
	//    - Or encode it into a custom binary format and store it as []byte:
	//         Value []byte `hydraide:"value"`  // custom binary blob
	//
	// ❗ HydrAIDE does not support raw interface{} storage — values must always be strongly typed.
	Log struct {
		Amount   int16  // ✅ Small integer: better memory & disk usage than int
		Reason   string // Reason for the credit log (e.g. "bonus")
		Currency string // Currency ISO code (e.g. "HUF", "EUR")
	} `hydraide:"value"`

	// ⏳ OPTIONAL
	// The logical expiration timestamp of this Treasure.
	//
	// When set, this field indicates when the data is considered "expired"
	// and eligible for deletion or TTL-based operations like CatalogShiftExpired.
	//
	// ❗IMPORTANT:
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
	ExpireAt time.Time `hydraide:"expireAt"`

	// 🧾 OPTIONAL METADATA — useful for tracking/audit purposes
	// If omitted, these fields will not be included in the stored record.

	CreatedBy string    `hydraide:"createdBy"` // Who created the record
	CreatedAt time.Time `hydraide:"createdAt"` // When it was created
	UpdatedBy string    `hydraide:"updatedBy"` // Who last updated it
	UpdatedAt time.Time `hydraide:"updatedAt"` // When it was last updated
}
