package gateway

import (
	"cmp"
	"math"
	"sort"
	"strings"

	hydrapb "github.com/hydraide/hydraide/generated/hydraidepbgo"
	"github.com/vmihailenco/msgpack/v5"
)

const earthRadiusKm = 6371.0

// haversineDistance computes the great-circle distance in km between two points
// specified in decimal degrees (WGS84).
func haversineDistance(lat1, lng1, lat2, lng2 float64) float64 {
	const degToRad = math.Pi / 180.0
	dLat := (lat2 - lat1) * degToRad
	dLng := (lng2 - lng1) * degToRad

	lat1Rad := lat1 * degToRad
	lat2Rad := lat2 * degToRad

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(dLng/2)*math.Sin(dLng/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusKm * c
}

// geoBoundingBox is a lat/lng rectangle for fast rejection before Haversine computation.
type geoBoundingBox struct {
	minLat, maxLat float64
	minLng, maxLng float64
}

func newGeoBoundingBox(lat, lng, radiusKm float64) geoBoundingBox {
	const degToRad = math.Pi / 180.0
	dLat := radiusKm / 111.32
	dLng := radiusKm / (111.32 * math.Cos(lat*degToRad))

	return geoBoundingBox{
		minLat: lat - dLat,
		maxLat: lat + dLat,
		minLng: lng - dLng,
		maxLng: lng + dLng,
	}
}

func (bb geoBoundingBox) contains(lat, lng float64) bool {
	return lat >= bb.minLat && lat <= bb.maxLat &&
		lng >= bb.minLng && lng <= bb.maxLng
}

// msgpack format detection constants (must match SDK values).
const (
	msgpackMagic0 byte = 0xC7
	msgpackMagic1 byte = 0x00
)

func isMsgpackEncoded(data []byte) bool {
	return len(data) >= 2 && data[0] == msgpackMagic0 && data[1] == msgpackMagic1
}

func unwrapMsgpack(data []byte) []byte {
	return data[2:]
}

// anyMatchSlice is a sentinel type returned by extractFieldByPath when the path
// contains a [*] wildcard. The evaluator iterates over values and returns true
// if ANY value matches the filter condition.
type anyMatchSlice struct {
	values []interface{}
}

// evaluateFilterGroupWith is the generic AND/OR evaluator for filter groups.
// The four callbacks determine how individual filters, sub-groups, phrase filters,
// and vector filters are evaluated.
func evaluateFilterGroupWith(
	group *hydrapb.FilterGroup,
	evalFilter func(*hydrapb.TreasureFilter) bool,
	evalSubGroup func(*hydrapb.FilterGroup) bool,
	evalPhrase func(*hydrapb.PhraseFilter) bool,
	evalVector func(*hydrapb.VectorFilter) bool,
	evalGeoDistance func(*hydrapb.GeoDistanceFilter) bool,
) bool {
	if group == nil {
		return true
	}

	hasFilters := len(group.Filters) > 0
	hasSubGroups := len(group.SubGroups) > 0
	hasPhraseFilters := len(group.PhraseFilters) > 0
	hasVectorFilters := len(group.VectorFilters) > 0
	hasGeoDistanceFilters := len(group.GeoDistanceFilters) > 0

	// Empty group = no filtering = pass
	if !hasFilters && !hasSubGroups && !hasPhraseFilters && !hasVectorFilters && !hasGeoDistanceFilters {
		return true
	}

	if group.Logic == hydrapb.FilterLogic_OR {
		for _, f := range group.Filters {
			if evalFilter(f) {
				return true
			}
		}
		for _, sg := range group.SubGroups {
			if evalSubGroup(sg) {
				return true
			}
		}
		for _, pf := range group.PhraseFilters {
			if evalPhrase(pf) {
				return true
			}
		}
		for _, vf := range group.VectorFilters {
			if evalVector(vf) {
				return true
			}
		}
		for _, gf := range group.GeoDistanceFilters {
			if evalGeoDistance(gf) {
				return true
			}
		}
		return false
	}

	// AND (default)
	for _, f := range group.Filters {
		if !evalFilter(f) {
			return false
		}
	}
	for _, sg := range group.SubGroups {
		if !evalSubGroup(sg) {
			return false
		}
	}
	for _, pf := range group.PhraseFilters {
		if !evalPhrase(pf) {
			return false
		}
	}
	for _, vf := range group.VectorFilters {
		if !evalVector(vf) {
			return false
		}
	}
	for _, gf := range group.GeoDistanceFilters {
		if !evalGeoDistance(gf) {
			return false
		}
	}
	return true
}

// decodeMsgpackToMap decodes MessagePack bytes into a map[string]interface{}.
func decodeMsgpackToMap(data []byte) (map[string]interface{}, error) {
	var m map[string]interface{}
	if err := msgpack.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// extractFieldByPath navigates a dot-separated path in a nested map.
// Example: "Address.City" extracts m["Address"].(map)["City"].
// An empty path returns the root map itself (used in profile mode where the entire BytesVal is the target).
//
// Special path segments:
//   - "#len" — returns the length of the current slice or map as int64
//   - "Field[*]" — iterates slice elements, extracts remaining path from each,
//     and returns an anyMatchSlice sentinel for any-match evaluation
func extractFieldByPath(m map[string]interface{}, path string) interface{} {
	if path == "" {
		return m
	}
	parts := strings.Split(path, ".")
	var current interface{} = m

	for i, part := range parts {
		// #len pseudo-field: return length of current value as int64
		if part == "#len" {
			switch v := current.(type) {
			case []interface{}:
				return int64(len(v))
			case map[string]interface{}:
				return int64(len(v))
			default:
				return nil
			}
		}

		// [*] wildcard: iterate slice, extract remaining path from each element
		if strings.HasSuffix(part, "[*]") {
			fieldName := strings.TrimSuffix(part, "[*]")
			if fieldName != "" {
				cm, ok := current.(map[string]interface{})
				if !ok {
					return nil
				}
				current = cm[fieldName]
			}
			arr, ok := current.([]interface{})
			if !ok {
				return nil
			}
			remainingPath := strings.Join(parts[i+1:], ".")
			values := make([]interface{}, 0, len(arr))
			for _, elem := range arr {
				if remainingPath == "" {
					values = append(values, elem)
				} else if em, ok := elem.(map[string]interface{}); ok {
					if v := extractFieldByPath(em, remainingPath); v != nil {
						values = append(values, v)
					}
				}
			}
			return anyMatchSlice{values: values}
		}

		// Normal navigation
		cm, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current, ok = cm[part]
		if !ok {
			return nil
		}
	}
	return current
}

// toInt64 attempts to convert a msgpack-decoded value to int64.
// MessagePack decodes integers as int8/int16/int32/int64/uint8/uint16/uint32/uint64.
func toInt64(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case int8:
		return int64(n), true
	case int16:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case uint8:
		return int64(n), true
	case uint16:
		return int64(n), true
	case uint32:
		return int64(n), true
	case uint64:
		const maxInt64 = uint64(1<<63 - 1)
		if n <= maxInt64 {
			return int64(n), true
		}
		return 0, false
	case float32:
		return int64(n), true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

// toUint64 attempts to convert a msgpack-decoded value to uint64.
func toUint64(v interface{}) (uint64, bool) {
	switch n := v.(type) {
	case uint8:
		return uint64(n), true
	case uint16:
		return uint64(n), true
	case uint32:
		return uint64(n), true
	case uint64:
		return n, true
	case int8:
		if n >= 0 {
			return uint64(n), true
		}
		return 0, false
	case int16:
		if n >= 0 {
			return uint64(n), true
		}
		return 0, false
	case int32:
		if n >= 0 {
			return uint64(n), true
		}
		return 0, false
	case int64:
		if n >= 0 {
			return uint64(n), true
		}
		return 0, false
	case float32:
		return uint64(n), true
	case float64:
		return uint64(n), true
	default:
		return 0, false
	}
}

// toFloat64 attempts to convert a msgpack-decoded value to float64.
func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	default:
		return 0, false
	}
}

// --- Typed comparison functions ---

// compareOrdered is a generic comparison function for all ordered types.
func compareOrdered[T cmp.Ordered](actual T, op hydrapb.Relational_Operator, ref T) bool {
	switch op {
	case hydrapb.Relational_EQUAL:
		return actual == ref
	case hydrapb.Relational_NOT_EQUAL:
		return actual != ref
	case hydrapb.Relational_GREATER_THAN:
		return actual > ref
	case hydrapb.Relational_GREATER_THAN_OR_EQUAL:
		return actual >= ref
	case hydrapb.Relational_LESS_THAN:
		return actual < ref
	case hydrapb.Relational_LESS_THAN_OR_EQUAL:
		return actual <= ref
	default:
		return false
	}
}

// compareString handles string comparison with additional string-specific operators.
func compareString(actual string, op hydrapb.Relational_Operator, ref string) bool {
	switch op {
	case hydrapb.Relational_CONTAINS:
		return strings.Contains(actual, ref)
	case hydrapb.Relational_NOT_CONTAINS:
		return !strings.Contains(actual, ref)
	case hydrapb.Relational_STARTS_WITH:
		return strings.HasPrefix(actual, ref)
	case hydrapb.Relational_ENDS_WITH:
		return strings.HasSuffix(actual, ref)
	default:
		return compareOrdered(actual, op, ref)
	}
}

// compareBoolRaw compares raw Go booleans.
func compareBoolRaw(actual bool, op hydrapb.Relational_Operator, ref bool) bool {
	switch op {
	case hydrapb.Relational_EQUAL:
		return actual == ref
	case hydrapb.Relational_NOT_EQUAL:
		return actual != ref
	default:
		return false
	}
}

// toInt64Slice converts a msgpack-decoded interface{} (expected []interface{}) to []int64.
func toInt64Slice(val interface{}) []int64 {
	arr, ok := val.([]interface{})
	if !ok {
		return nil
	}
	result := make([]int64, 0, len(arr))
	for _, item := range arr {
		if v, ok := toInt64(item); ok {
			result = append(result, v)
		}
	}
	return result
}

// hasConsecutivePositions checks if there exists a sequence of consecutive positions
// across the word position lists.
func hasConsecutivePositions(wordPositions [][]int64) bool {
	if len(wordPositions) == 0 {
		return true
	}
	if len(wordPositions) == 1 {
		return len(wordPositions[0]) > 0
	}
	for _, startPos := range wordPositions[0] {
		found := true
		for i := 1; i < len(wordPositions); i++ {
			target := startPos + int64(i)
			if !sortedContains(wordPositions[i], target) {
				found = false
				break
			}
		}
		if found {
			return true
		}
	}
	return false
}

// sortedContains checks if a sorted int64 slice contains the target value using binary search.
func sortedContains(sorted []int64, target int64) bool {
	i := sort.Search(len(sorted), func(j int) bool { return sorted[j] >= target })
	return i < len(sorted) && sorted[i] == target
}

// dotProduct computes the dot product of two float32 vectors.
// When both vectors are L2-normalized, this equals cosine similarity.
// Uses 4-wide loop unrolling for better CPU pipeline utilization.
func dotProduct(a, b []float32) float32 {
	n := len(a)
	var sum float32

	i := 0
	for ; i <= n-4; i += 4 {
		sum += a[i]*b[i] + a[i+1]*b[i+1] + a[i+2]*b[i+2] + a[i+3]*b[i+3]
	}

	for ; i < n; i++ {
		sum += a[i] * b[i]
	}

	return sum
}

// toFloat32Slice converts a msgpack-decoded interface{} (expected []interface{})
// to []float32. Returns nil if the value is not a numeric array.
func toFloat32Slice(val interface{}) []float32 {
	arr, ok := val.([]interface{})
	if !ok {
		return nil
	}
	result := make([]float32, 0, len(arr))
	for _, item := range arr {
		switch n := item.(type) {
		case float32:
			result = append(result, n)
		case float64:
			result = append(result, float32(n))
		case int8:
			result = append(result, float32(n))
		case int16:
			result = append(result, float32(n))
		case int32:
			result = append(result, float32(n))
		case int64:
			result = append(result, float32(n))
		case uint8:
			result = append(result, float32(n))
		case uint16:
			result = append(result, float32(n))
		case uint32:
			result = append(result, float32(n))
		case uint64:
			result = append(result, float32(n))
		default:
			return nil // Non-numeric element — invalid vector
		}
	}
	return result
}

// evaluateSliceContains checks if a slice ([]interface{}) contains (or not) the
// value specified in the filter's CompareValue. Handles exact match for int/string
// types and case-insensitive substring match for string slices.
func evaluateSliceContains(fieldVal interface{}, op hydrapb.Relational_Operator, filter *hydrapb.TreasureFilter) bool {
	arr, ok := fieldVal.([]interface{})
	if !ok {
		return op == hydrapb.Relational_SLICE_NOT_CONTAINS || op == hydrapb.Relational_SLICE_NOT_CONTAINS_SUBSTRING
	}

	isSubstring := op == hydrapb.Relational_SLICE_CONTAINS_SUBSTRING || op == hydrapb.Relational_SLICE_NOT_CONTAINS_SUBSTRING
	negate := op == hydrapb.Relational_SLICE_NOT_CONTAINS || op == hydrapb.Relational_SLICE_NOT_CONTAINS_SUBSTRING

	found := false
	if isSubstring {
		cv, ok := filter.GetCompareValue().(*hydrapb.TreasureFilter_StringVal)
		if !ok {
			return negate
		}
		lowerRef := strings.ToLower(cv.StringVal)
		for _, elem := range arr {
			if s, ok := elem.(string); ok && strings.Contains(strings.ToLower(s), lowerRef) {
				found = true
				break
			}
		}
	} else {
		switch cv := filter.GetCompareValue().(type) {
		case *hydrapb.TreasureFilter_Int8Val:
			ref := int64(cv.Int8Val)
			for _, elem := range arr {
				if v, ok := toInt64(elem); ok && v == ref {
					found = true
					break
				}
			}
		case *hydrapb.TreasureFilter_Int32Val:
			ref := int64(cv.Int32Val)
			for _, elem := range arr {
				if v, ok := toInt64(elem); ok && v == ref {
					found = true
					break
				}
			}
		case *hydrapb.TreasureFilter_Int64Val:
			ref := cv.Int64Val
			for _, elem := range arr {
				if v, ok := toInt64(elem); ok && v == ref {
					found = true
					break
				}
			}
		case *hydrapb.TreasureFilter_StringVal:
			for _, elem := range arr {
				if s, ok := elem.(string); ok && s == cv.StringVal {
					found = true
					break
				}
			}
		default:
			return negate
		}
	}

	if negate {
		return !found
	}
	return found
}

// evaluateAnyMatch returns true if ANY value in the anyMatchSlice satisfies
// the filter's operator and compare value.
func evaluateAnyMatch(ams anyMatchSlice, op hydrapb.Relational_Operator, filter *hydrapb.TreasureFilter) bool {
	if len(ams.values) == 0 {
		return op == hydrapb.Relational_IS_EMPTY
	}

	if op == hydrapb.Relational_IS_NOT_EMPTY {
		for _, v := range ams.values {
			if v != nil {
				if s, ok := v.(string); ok && s == "" {
					continue
				}
				return true
			}
		}
		return false
	}

	if op == hydrapb.Relational_IS_EMPTY {
		for _, v := range ams.values {
			if v != nil {
				if s, ok := v.(string); ok && s == "" {
					continue
				}
				return false
			}
		}
		return true
	}

	for _, v := range ams.values {
		if v == nil {
			continue
		}
		switch cv := filter.GetCompareValue().(type) {
		case *hydrapb.TreasureFilter_StringVal:
			if s, ok := v.(string); ok && compareString(s, op, cv.StringVal) {
				return true
			}
		case *hydrapb.TreasureFilter_Int8Val:
			if n, ok := toInt64(v); ok && compareOrdered(n, op, int64(cv.Int8Val)) {
				return true
			}
		case *hydrapb.TreasureFilter_Int32Val:
			if n, ok := toInt64(v); ok && compareOrdered(n, op, int64(cv.Int32Val)) {
				return true
			}
		case *hydrapb.TreasureFilter_Int64Val:
			if n, ok := toInt64(v); ok && compareOrdered(n, op, cv.Int64Val) {
				return true
			}
		case *hydrapb.TreasureFilter_BoolVal:
			if b, ok := v.(bool); ok {
				ref := cv.BoolVal == hydrapb.Boolean_TRUE
				if compareBoolRaw(b, op, ref) {
					return true
				}
			}
		case *hydrapb.TreasureFilter_Float64Val:
			if n, ok := toFloat64(v); ok && compareOrdered(n, op, cv.Float64Val) {
				return true
			}
		}
	}
	return false
}
