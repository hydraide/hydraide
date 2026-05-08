package hydraidego

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
	"github.com/vmihailenco/msgpack/v5"
)

// reservedHydraideTagNames is the set of tag values that map to first-class
// Treasure slots (key, expireAt, the createdAt/By + updatedAt/By metadata,
// and the single-value `value` wrapper). Everything else is a body field.
var reservedHydraideTagNames = map[string]struct{}{
	tagKey:       {},
	tagValue:     {},
	tagExpireAt:  {},
	tagCreatedAt: {},
	tagCreatedBy: {},
	tagUpdatedAt: {},
	tagUpdatedBy: {},
}

// mapBodyField describes one field that participates in the msgpack-map body
// of a map-body Catalog. Name is the wire key (the `hydraide:"FieldName"`
// tag value), Index is the struct field index, OmitEmpty mirrors the tag.
type mapBodyField struct {
	Name      string
	Index     int
	OmitEmpty bool
}

// catalogShape classifies a Catalog model based on which `hydraide` tags
// appear on its fields. The three shapes are mutually exclusive.
type catalogShape int

const (
	catalogShapeKeyOnly   catalogShape = iota // no value, no body fields
	catalogShapeSingleVal                     // hydraide:"value" wrapper
	catalogShapeMapBody                       // one or more hydraide:"FieldName" body fields
)

// inspectCatalogModel walks a struct type once and returns its shape +
// the list of map-body fields (only populated for catalogShapeMapBody).
//
// Mixing shapes (a `value` tag together with non-reserved body tags) is a
// modelling error and is reported as an error.
func inspectCatalogModel(t reflect.Type) (catalogShape, []mapBodyField, error) {
	if t.Kind() != reflect.Struct {
		return catalogShapeKeyOnly, nil, errors.New("model must be a struct")
	}

	hasValue := false
	var bodyFields []mapBodyField

	for i := 0; i < t.NumField(); i++ {
		raw, ok := t.Field(i).Tag.Lookup(tagHydrAIDE)
		if !ok {
			continue
		}
		parts := strings.Split(raw, ",")
		head := parts[0]
		if head == "" {
			continue
		}
		if head == tagValue {
			hasValue = true
			continue
		}
		if _, reserved := reservedHydraideTagNames[head]; reserved {
			continue
		}
		// Non-reserved head → map-body field. Tag value is the wire key.
		omitempty := false
		for _, p := range parts[1:] {
			if strings.TrimSpace(p) == tagOmitempty {
				omitempty = true
			}
		}
		bodyFields = append(bodyFields, mapBodyField{
			Name:      head,
			Index:     i,
			OmitEmpty: omitempty,
		})
	}

	if hasValue && len(bodyFields) > 0 {
		return catalogShapeKeyOnly, nil, errors.New(`model mixes hydraide:"value" with map-body fields; pick one shape`)
	}
	switch {
	case hasValue:
		return catalogShapeSingleVal, nil, nil
	case len(bodyFields) > 0:
		return catalogShapeMapBody, bodyFields, nil
	default:
		return catalogShapeKeyOnly, nil, nil
	}
}

// encodeMapBody marshals the named body fields of v into a wrapped msgpack
// map blob (with HydrAIDE's 2-byte magic prefix). Empty fields tagged with
// omitempty are skipped. Returns nil when every field was skipped (caller
// can leave BytesVal unset and rely on VoidVal).
func encodeMapBody(v reflect.Value, fields []mapBodyField) ([]byte, error) {
	if len(fields) == 0 {
		return nil, nil
	}
	body := make(map[string]any, len(fields))
	for _, f := range fields {
		fv := v.Field(f.Index)
		if f.OmitEmpty && isFieldEmpty(fv) {
			continue
		}
		body[f.Name] = fv.Interface()
	}
	if len(body) == 0 {
		return nil, nil
	}
	encoded, err := msgpack.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode map-body: %w", err)
	}
	return wrapMsgpack(encoded), nil
}

// decodeMapBodyInto unwraps and msgpack-decodes blob into the named body
// fields of v. blob may be wrapped (magic prefix) or raw msgpack — both are
// accepted, since CatalogReadX returns wrapped BytesVal but the patch flow
// returns the unwrapped NewMsgpack.
//
// Field assignment uses the tag value as the map key. Missing keys leave
// the field at its zero value. Type mismatches surface as an error.
func decodeMapBodyInto(blob []byte, v reflect.Value, fields []mapBodyField) error {
	if len(blob) == 0 || len(fields) == 0 {
		return nil
	}
	body := blob
	if isMsgpackEncoded(body) {
		body = unwrapMsgpack(body)
	}
	if len(body) == 0 {
		return nil
	}
	raws := make(map[string]msgpack.RawMessage)
	if err := msgpack.Unmarshal(body, &raws); err != nil {
		return fmt.Errorf("decode map-body: %w", err)
	}
	for _, f := range fields {
		raw, ok := raws[f.Name]
		if !ok {
			continue
		}
		fv := v.Field(f.Index)
		if !fv.CanAddr() {
			continue
		}
		if err := msgpack.Unmarshal(raw, fv.Addr().Interface()); err != nil {
			return fmt.Errorf("decode map-body field %q: %w", f.Name, err)
		}
	}
	return nil
}

// applyMapBodyToKvPair encodes the model's map-body (if any) into kvPair
// as a wrapped msgpack BytesVal. It also clears VoidVal that an earlier
// pass may have set when no `value` field was present.
//
// The encoding format is fixed (msgpack + magic prefix) regardless of the
// swamp's EncodingFormat: the patch flow only operates on msgpack bodies,
// and the whole point of map-body Catalogs is round-tripping with patches.
func applyMapBodyToKvPair(kvPair *hydraidepbgo.KeyValuePair, v reflect.Value, fields []mapBodyField) error {
	body, err := encodeMapBody(v, fields)
	if err != nil {
		return err
	}
	if body == nil {
		return nil
	}
	kvPair.BytesVal = body
	kvPair.VoidVal = nil
	return nil
}
