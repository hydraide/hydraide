package gateway

import (
	"errors"

	hydrapb "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
	"github.com/vmihailenco/msgpack/v5"
)

// buildBodyCapPredicate validates a Cap message intended for an explicit-
// key Patch surface and returns a body-only predicate that operates on a
// pre-decoded msgpack map.
//
// Phase B restricts Cap on Patch surfaces to BytesField filters (and
// nested combinations thereof). Metadata filters (CreatedAt, UpdatedAt,
// ExpiredAt, value-typed filters) are rejected at this entry point —
// supporting them would require simulating the post-mutation metadata
// state for every per-key check, which is out of scope.
//
// Returns (nil, 0, nil) when Cap is absent (cap path is opt-in).
func buildBodyCapPredicate(cap *hydrapb.Cap) (func(decoded map[string]interface{}) bool, int32, error) {
	if cap == nil {
		return nil, 0, nil
	}
	if cap.GetMaxMatching() <= 0 {
		return nil, 0, errors.New("Cap.MaxMatching must be > 0")
	}
	filter := cap.GetFilter()
	if filter == nil {
		return nil, 0, errors.New("Cap.Filter is required when Cap is set")
	}
	if err := validateBodyOnlyFilterGroup(filter); err != nil {
		return nil, 0, err
	}
	predicate := func(decoded map[string]interface{}) bool {
		return evaluateFilterGroupAgainstMap(decoded, filter)
	}
	return predicate, cap.GetMaxMatching(), nil
}

// validateBodyOnlyFilterGroup walks the group recursively and rejects any
// filter that operates on metadata or typed values rather than a
// BytesField path. This guarantees the predicate can be evaluated against
// a decoded msgpack body alone — without consulting treasure metadata.
func validateBodyOnlyFilterGroup(group *hydrapb.FilterGroup) error {
	if group == nil {
		return nil
	}
	for _, f := range group.Filters {
		if f.GetBytesFieldPath() == "" {
			return errors.New("Cap.Filter for Patch surfaces must use BytesField filters only (metadata / typed-value filters are not supported)")
		}
	}
	if len(group.PhraseFilters) > 0 {
		return errors.New("Cap.Filter for Patch surfaces does not support PhraseFilter")
	}
	if len(group.VectorFilters) > 0 {
		return errors.New("Cap.Filter for Patch surfaces does not support VectorFilter")
	}
	if len(group.GeoDistanceFilters) > 0 {
		return errors.New("Cap.Filter for Patch surfaces does not support GeoDistanceFilter")
	}
	for _, sg := range group.SubGroups {
		if err := validateBodyOnlyFilterGroup(sg); err != nil {
			return err
		}
	}
	return nil
}

// decodeMsgpackMapForCap decodes a raw msgpack body (no magic prefix)
// into a map[string]interface{} suitable for evaluateFilterGroupAgainstMap.
// Returns (nil, err) on decode failure; the caller decides whether that
// turns into a per-key TYPE_MISMATCH or ENCODING_NOT_SUPPORTED status.
func decodeMsgpackMapForCap(body []byte) (map[string]interface{}, error) {
	if len(body) == 0 {
		return map[string]interface{}{}, nil
	}
	var decoded map[string]interface{}
	if err := msgpack.Unmarshal(body, &decoded); err != nil {
		return nil, err
	}
	if decoded == nil {
		decoded = map[string]interface{}{}
	}
	return decoded, nil
}
