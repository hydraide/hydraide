package hydraidego

import (
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// convertCatalogModelToKeyValuePair converts a Go struct (passed as pointer) into a HydrAIDE-compatible KeyValuePair message.
//
// 🧠 This is an **internal serialization helper** used by the Go SDK to translate user-defined models
// into the binary format that HydrAIDE expects when inserting or updating Treasures.
//
// ✅ Supported field tags:
// - `hydraide:"key"`       → Marks the string field to use as the Treasure key (must be non-empty).
// - `hydraide:"value"`     → Marks the value field (can be any supported primitive or complex type).
// - `hydraide:"expireAt"`  → Optional `time.Time`, marks the logical expiry time of the Treasure.
// - `hydraide:"createdAt"` / `createdBy` / `updatedAt` / `updatedBy` → Optional metadata fields.
// - `hydraide:"omitempty"` → Skips the field during encoding if it's zero, nil, or empty.
//
// ✅ Supported value types:
// - Primitives: string, bool, int, uint, float (various widths)
// - time.Time (as int64 UNIX timestamp)
// - Slices and maps (serialized as GOB or MessagePack binary blobs depending on encoding setting)
// - Structs and pointers (also GOB or MessagePack encoded)
// - `nil` / empty values are optionally excluded if marked with `omitempty`
//
// ⚠️ Requirements:
// - The input **must be a pointer to a struct**, otherwise the function returns an error.
// - The struct **must contain a field marked as `hydrun:"key"`** with a non-empty string.
// - The value can be a primitive or complex field marked with `hydrun:"value"`.
// - If no value is provided, the resulting KeyValuePair will include a `VoidVal=true` marker.
//
// 🧬 Why this matters:
// HydrAIDE works with protocol-level binary messages.
// Every Treasure must be sent as a KeyValuePair with a valid key and (optionally) a value.
// This function bridges Go structs and HydrAIDE’s native format, abstracting encoding logic.
//
// ✨ This is how arbitrary business models (e.g. `UserProfile`, `InvoiceItem`) are safely,
// efficiently and correctly transformed into Treasure representations.
//
// 📌 If you're building a new SDK (e.g. for Python, Rust, Node.js), your implementation
// should follow the same principles:
// - Tag-driven key/value separation
// - Support for void values and expiration
// - Metadata injection
// - Optional field skipping (e.g. omitempty)
// - Consistent type coercion for known value types
func convertCatalogModelToKeyValuePair(model any, encoding EncodingFormat) (*hydraidepbgo.KeyValuePair, error) {

	// Get the reflection value of the input model
	v := reflect.ValueOf(model)

	// 🧪 Validate the input: it must be a pointer to a struct.
	// This is required because we'll be using reflection to iterate over the fields
	// and extract tags and values dynamically. Non-pointer or non-struct inputs are invalid.
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return nil, errors.New("input must be a pointer to a struct")
	}

	// Initialize the KeyValuePair that will hold the final encoded output
	kvPair := &hydraidepbgo.KeyValuePair{}

	// Get the actual struct (dereferenced value) and its type
	v = v.Elem()
	t := v.Type()

	// Classify the Catalog shape (single-value vs map-body vs key-only) so
	// we can encode the body correctly and reject mixed-shape models early.
	shape, mapBodyFields, shapeErr := inspectCatalogModel(t)
	if shapeErr != nil {
		return nil, shapeErr
	}

	for i := 0; i < t.NumField(); i++ {

		field := t.Field(i)

		// Check if the current field is marked as the `key` field (via `hydraide:"key"` tag)
		if key, ok := field.Tag.Lookup(tagHydrAIDE); ok && key == tagKey {

			value := v.Field(i)

			// Validate that the field is a non-empty string — required for all HydrAIDE Treasures.
			// Keys must always be explicit and unique within a Swamp.
			if value.Kind() == reflect.String && value.String() != "" {
				// Found the key — assign it to the KeyValuePair
				kvPair.Key = value.String()
				continue
			}

			// If the key field is missing or empty, this is an invalid model
			return nil, errors.New("key field must be a non-empty string")
		}

		// Check if the current field is tagged as the `value` field (via `hydraide:"value"`)
		// This field holds the actual value of the Treasure.
		// We detect its type using reflection and populate the corresponding proto field in KeyValuePair.
		if key, ok := field.Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagValue) {

			value := v.Field(i)
			isEmpty := isFieldEmpty(value)
			if isEmpty {
				// This flag tracks whether any value has been set.
				// If no value is provided (only key or metadata), we'll later set VoidVal = true.
				valueVoid := true
				kvPair.VoidVal = &valueVoid
			}
			if strings.Contains(key, tagOmitempty) && isEmpty {
				// If omitempty is set and the field is empty, skip setting the value
				continue
			}

			// convert the value to KeyValuePair
			if err := convertFieldToKvPair(value, kvPair, encoding); err != nil {
				return nil, err
			}

		}

		// Process the `expireAt` field (tagged with `hydraide:"expireAt"`).
		// This defines the logical expiration time of the Treasure.
		// Once the given timestamp is reached, HydrAIDE will treat the record as expired.
		// - Must be of type `time.Time`
		// - If omitempty is set, zero values are skipped without error
		// - Otherwise must be non-zero
		// - Automatically converted to a `timestamppb.Timestamp` for protobuf
		if key, ok := field.Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagExpireAt) {

			value := v.Field(i)
			hasOmitempty := strings.Contains(key, tagOmitempty)

			if hasOmitempty && isFieldEmpty(value) {
				// If omitempty is set and the field is empty, skip setting expireAt
				continue
			}

			if value.Kind() != reflect.Struct || value.Type() != reflect.TypeOf(time.Time{}) {
				return nil, errors.New("expireAt field must be a time.Time")
			}
			expireAt := value.Interface().(time.Time).UTC()

			// Only validate non-zero if omitempty is NOT set
			if !hasOmitempty && expireAt.IsZero() {
				return nil, errors.New("expireAt field must be a non-zero time.Time")
			}

			// If omitempty is set and we got here, the value is non-zero, so we can set it
			if !expireAt.IsZero() {
				kvPair.ExpiredAt = timestamppb.New(expireAt)
			}
			continue

		}

		// Process the `createdBy` field (tagged with `hydraide:"createdBy"`).
		// Optional metadata indicating who or what created the Treasure.
		// - Must be of type `string`
		// - Empty values are ignored
		if key, ok := field.Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagCreatedBy) {

			value := v.Field(i)

			if strings.Contains(key, tagOmitempty) && isFieldEmpty(value) {
				// If omitempty is set and the field is empty, skip setting createdBy
				continue
			}

			if value.Kind() != reflect.String {
				return nil, errors.New("createdBy field must be a string")
			}

			if value.String() != "" {
				createdBy := value.String()
				kvPair.CreatedBy = &createdBy
			}

			continue
		}

		// Process the `createdAt` field (tagged with `hydraide:"createdAt"`).
		// Optional metadata representing when the Treasure was created.
		// - Must be of type `time.Time`
		// - If omitempty is set, zero values are skipped without error
		// - Otherwise must be non-zero
		// - Converted to protobuf-compatible timestamp
		if key, ok := field.Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagCreatedAt) {

			value := v.Field(i)
			hasOmitempty := strings.Contains(key, tagOmitempty)

			if hasOmitempty && isFieldEmpty(value) {
				continue
			}

			if value.Kind() != reflect.Struct || value.Type() != reflect.TypeOf(time.Time{}) {
				return nil, errors.New("createdAt field must be a time.Time")
			}
			createdAt := value.Interface().(time.Time).UTC()

			// Only validate non-zero if omitempty is NOT set
			if !hasOmitempty && createdAt.IsZero() {
				return nil, errors.New("createdAt field must be a non-zero time.Time")
			}

			// If omitempty is set and we got here, the value is non-zero, so we can set it
			if !createdAt.IsZero() {
				kvPair.CreatedAt = timestamppb.New(createdAt)
			}
			continue
		}

		// Process the `updatedBy` field (tagged with `hydraide:"updatedBy"`).
		// Optional metadata indicating who or what last updated the Treasure.
		// - Must be of type `string`
		// - If omitempty is set, empty values are skipped
		// - Otherwise empty values are still allowed but not set
		if key, ok := field.Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagUpdatedBy) {

			value := v.Field(i)
			hasOmitempty := strings.Contains(key, tagOmitempty)

			if hasOmitempty && isFieldEmpty(value) {
				// If omitempty is set and the field is empty, skip setting updatedBy
				continue
			}

			if value.Kind() != reflect.String {
				return nil, errors.New("updatedBy field must be a string")
			}

			// Only set if the value is non-empty
			if value.String() != "" {
				updatedBy := value.String()
				kvPair.UpdatedBy = &updatedBy
			}
			continue
		}

		// Process the `updatedAt` field (tagged with `hydraide:"updatedAt"`).
		// Optional metadata representing the last modification time of the Treasure.
		// - Must be of type `time.Time`
		// - If omitempty is set, zero values are skipped without error
		// - Otherwise must be non-zero
		// - Automatically converted to a `timestamppb.Timestamp` for protobuf transmission
		if key, ok := field.Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagUpdatedAt) {

			value := v.Field(i)
			hasOmitempty := strings.Contains(key, tagOmitempty)

			if hasOmitempty && isFieldEmpty(value) {
				// If omitempty is set and the field is empty, skip setting updatedAt
				continue
			}

			if value.Kind() != reflect.Struct || value.Type() != reflect.TypeOf(time.Time{}) {
				return nil, errors.New("updatedAt field must be a time.Time")
			}
			updatedAt := value.Interface().(time.Time).UTC()

			// Only validate non-zero if omitempty is NOT set
			if !hasOmitempty && updatedAt.IsZero() {
				return nil, errors.New("updatedAt field must be a non-zero time.Time")
			}

			// If omitempty is set and we got here, the value is non-zero, so we can set it
			if !updatedAt.IsZero() {
				kvPair.UpdatedAt = timestamppb.New(updatedAt)
			}
			continue
		}

	}

	// Map-body Catalog shape: collect non-reserved tagged fields into a
	// msgpack map and store it (wrapped with the magic prefix) in BytesVal.
	// This produces a body that is symmetric with the patch flow — Save and
	// later Patch operate on the same on-disk msgpack map.
	if shape == catalogShapeMapBody {
		if err := applyMapBodyToKvPair(kvPair, v, mapBodyFields); err != nil {
			return nil, err
		}
	}

	// Final validation: the key must be present and non-empty.
	// This is a hard requirement — all Treasures in HydrAIDE must have a key.
	if kvPair.Key == "" {
		return nil, errors.New("key field not found")
	}

	// Return the fully constructed KeyValuePair for insertion into the system.
	return kvPair, nil

}

// convertProtoTreasureToCatalogModel maps a hydraidepbgo.Treasure protobuf object back into a Go struct.
//
// The target model must be a pointer to a struct. Fields are matched using `hydraide` struct tags:
// - `key`: assigns Treasure.Key to the struct's key field.
// - `value`: maps the appropriate typed value from Treasure into the struct's value field.
// - `expireAt`, `createdBy`, `createdAt`, `updatedBy`, `updatedAt`: optional metadata fields.
//
// Supported value conversions include:
// - Primitive types: string, bool, intX, uintX, floatX
// - time.Time (from int64 UNIX timestamp)
// - []byte (raw bytes)
// - All other slices, maps, and pointers (GOB-encoded in BytesVal)
//
// If the field type does not match the Treasure value type, it is silently skipped.
// If decoding fails (e.g. from GOB), an error is returned.
func convertProtoTreasureToCatalogModel(treasure *hydraidepbgo.Treasure, model any) error {

	v := reflect.ValueOf(model)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return errors.New("input must be a pointer to a struct at convertProtoTreasureToCatalogModel")
	}

	t := v.Elem().Type()

	// For map-body Catalogs, decode the wrapped msgpack BytesVal into the
	// non-reserved tagged fields. This is the symmetric counterpart of the
	// encoder's map-body branch and keeps Save/Read round-trippable with
	// the patch flow.
	if shape, mapBodyFields, shapeErr := inspectCatalogModel(t); shapeErr == nil && shape == catalogShapeMapBody {
		if err := decodeMapBodyInto(treasure.GetBytesVal(), v.Elem(), mapBodyFields); err != nil {
			return err
		}
	}

	for i := 0; i < t.NumField(); i++ {

		if key, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagKey) {
			v.Elem().Field(i).SetString(treasure.GetKey())
			continue
		}

		if key, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagValue) {

			field := v.Elem().Field(i)

			// set proto treasure to model
			if err := setProtoTreasureToModel(treasure, field); err != nil {
				return err
			}

			continue

		}

		if key, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagExpireAt) {
			if treasure.ExpiredAt != nil {
				v.Elem().Field(i).Set(reflect.ValueOf(treasure.ExpiredAt.AsTime()))
			}
			continue
		}

		if key, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagCreatedBy) {
			if treasure.CreatedBy != nil {
				v.Elem().Field(i).SetString(*treasure.CreatedBy)
			}
			continue
		}

		if key, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagCreatedAt) {
			if treasure.CreatedAt != nil {
				v.Elem().Field(i).Set(reflect.ValueOf(treasure.CreatedAt.AsTime()))
			}
			continue
		}

		if key, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagUpdatedBy) {
			if treasure.UpdatedBy != nil {
				v.Elem().Field(i).SetString(*treasure.UpdatedBy)
			}
			continue
		}

		if key, ok := t.Field(i).Tag.Lookup(tagHydrAIDE); ok && strings.Contains(key, tagUpdatedAt) {
			if treasure.UpdatedAt != nil {
				v.Elem().Field(i).Set(reflect.ValueOf(treasure.UpdatedAt.AsTime()))
			}
			continue
		}

	}

	return nil

}
