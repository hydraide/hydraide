package gateway

import (
	"strings"

	hydrapb "github.com/hydraide/hydraide/generated/hydraidepbgo"
	"github.com/vmihailenco/msgpack/v5"
)

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

// evaluateFilterGroup recursively evaluates a FilterGroup against a Treasure.
//
// A FilterGroup combines leaf filters and nested sub-groups using AND or OR logic:
//   - AND (default): ALL leaf filters AND ALL sub-groups must evaluate to true
//   - OR: at least ONE leaf filter OR ONE sub-group must evaluate to true
//
// Returns true if the group is nil or empty (no filtering applied).
func evaluateFilterGroup(treasure *hydrapb.Treasure, group *hydrapb.FilterGroup) bool {
	if group == nil {
		return true
	}

	hasFilters := len(group.Filters) > 0
	hasSubGroups := len(group.SubGroups) > 0

	// Empty group = no filtering = pass
	if !hasFilters && !hasSubGroups {
		return true
	}

	if group.Logic == hydrapb.FilterLogic_OR {
		// OR: at least one leaf filter or sub-group must be true
		for _, f := range group.Filters {
			if evaluateSingleFilter(treasure, f) {
				return true
			}
		}
		for _, sg := range group.SubGroups {
			if evaluateFilterGroup(treasure, sg) {
				return true
			}
		}
		return false
	}

	// AND (default): all leaf filters and all sub-groups must be true
	for _, f := range group.Filters {
		if !evaluateSingleFilter(treasure, f) {
			return false
		}
	}
	for _, sg := range group.SubGroups {
		if !evaluateFilterGroup(treasure, sg) {
			return false
		}
	}
	return true
}

// evaluateSingleFilter evaluates one TreasureFilter against a Treasure.
// The oneof CompareValue determines which Treasure field to compare.
// If BytesFieldPath is set, the filter extracts from BytesVal instead.
func evaluateSingleFilter(treasure *hydrapb.Treasure, filter *hydrapb.TreasureFilter) bool {
	op := filter.GetOperator()

	// If BytesFieldPath is set, extract the value from MessagePack-encoded BytesVal
	if filter.BytesFieldPath != nil && *filter.BytesFieldPath != "" {
		return evaluateBytesFieldFilter(treasure, filter)
	}

	switch cv := filter.GetCompareValue().(type) {
	case *hydrapb.TreasureFilter_Int8Val:
		if treasure.Int8Val == nil {
			return false
		}
		return compareInt32(*treasure.Int8Val, op, cv.Int8Val)

	case *hydrapb.TreasureFilter_Int16Val:
		if treasure.Int16Val == nil {
			return false
		}
		return compareInt32(*treasure.Int16Val, op, cv.Int16Val)

	case *hydrapb.TreasureFilter_Int32Val:
		if treasure.Int32Val == nil {
			return false
		}
		return compareInt32(*treasure.Int32Val, op, cv.Int32Val)

	case *hydrapb.TreasureFilter_Int64Val:
		if treasure.Int64Val == nil {
			return false
		}
		return compareInt64(*treasure.Int64Val, op, cv.Int64Val)

	case *hydrapb.TreasureFilter_Uint8Val:
		if treasure.Uint8Val == nil {
			return false
		}
		return compareUint32(*treasure.Uint8Val, op, cv.Uint8Val)

	case *hydrapb.TreasureFilter_Uint16Val:
		if treasure.Uint16Val == nil {
			return false
		}
		return compareUint32(*treasure.Uint16Val, op, cv.Uint16Val)

	case *hydrapb.TreasureFilter_Uint32Val:
		if treasure.Uint32Val == nil {
			return false
		}
		return compareUint32(*treasure.Uint32Val, op, cv.Uint32Val)

	case *hydrapb.TreasureFilter_Uint64Val:
		if treasure.Uint64Val == nil {
			return false
		}
		return compareUint64(*treasure.Uint64Val, op, cv.Uint64Val)

	case *hydrapb.TreasureFilter_Float32Val:
		if treasure.Float32Val == nil {
			return false
		}
		return compareFloat32(*treasure.Float32Val, op, cv.Float32Val)

	case *hydrapb.TreasureFilter_Float64Val:
		if treasure.Float64Val == nil {
			return false
		}
		return compareFloat64(*treasure.Float64Val, op, cv.Float64Val)

	case *hydrapb.TreasureFilter_StringVal:
		if treasure.StringVal == nil {
			return false
		}
		return compareString(*treasure.StringVal, op, cv.StringVal)

	case *hydrapb.TreasureFilter_BoolVal:
		if treasure.BoolVal == nil {
			return false
		}
		return compareBool(*treasure.BoolVal, op, cv.BoolVal)

	default:
		// Unknown filter type — skip (don't match)
		return false
	}
}

// evaluateBytesFieldFilter extracts a field from MessagePack-encoded BytesVal
// and applies the filter to the extracted value.
// Returns false if BytesVal is nil, not MessagePack-encoded, or the field path doesn't exist.
func evaluateBytesFieldFilter(treasure *hydrapb.Treasure, filter *hydrapb.TreasureFilter) bool {
	if treasure.BytesVal == nil {
		return false
	}
	if !isMsgpackEncoded(treasure.BytesVal) {
		// GOB-encoded data cannot be inspected — filter does not match
		return false
	}

	decoded, err := decodeMsgpackToMap(unwrapMsgpack(treasure.BytesVal))
	if err != nil {
		return false
	}

	fieldVal := extractFieldByPath(decoded, *filter.BytesFieldPath)
	if fieldVal == nil {
		return false
	}

	op := filter.GetOperator()

	// Match the extracted value against the filter's CompareValue
	switch cv := filter.GetCompareValue().(type) {
	case *hydrapb.TreasureFilter_Int8Val:
		if v, ok := toInt64(fieldVal); ok {
			return compareInt64(v, op, int64(cv.Int8Val))
		}
	case *hydrapb.TreasureFilter_Int16Val:
		if v, ok := toInt64(fieldVal); ok {
			return compareInt64(v, op, int64(cv.Int16Val))
		}
	case *hydrapb.TreasureFilter_Int32Val:
		if v, ok := toInt64(fieldVal); ok {
			return compareInt64(v, op, int64(cv.Int32Val))
		}
	case *hydrapb.TreasureFilter_Int64Val:
		if v, ok := toInt64(fieldVal); ok {
			return compareInt64(v, op, cv.Int64Val)
		}
	case *hydrapb.TreasureFilter_Uint8Val:
		if v, ok := toUint64(fieldVal); ok {
			return compareUint64(v, op, uint64(cv.Uint8Val))
		}
	case *hydrapb.TreasureFilter_Uint16Val:
		if v, ok := toUint64(fieldVal); ok {
			return compareUint64(v, op, uint64(cv.Uint16Val))
		}
	case *hydrapb.TreasureFilter_Uint32Val:
		if v, ok := toUint64(fieldVal); ok {
			return compareUint64(v, op, uint64(cv.Uint32Val))
		}
	case *hydrapb.TreasureFilter_Uint64Val:
		if v, ok := toUint64(fieldVal); ok {
			return compareUint64(v, op, cv.Uint64Val)
		}
	case *hydrapb.TreasureFilter_Float32Val:
		if v, ok := toFloat64(fieldVal); ok {
			return compareFloat64(v, op, float64(cv.Float32Val))
		}
	case *hydrapb.TreasureFilter_Float64Val:
		if v, ok := toFloat64(fieldVal); ok {
			return compareFloat64(v, op, cv.Float64Val)
		}
	case *hydrapb.TreasureFilter_StringVal:
		if v, ok := fieldVal.(string); ok {
			return compareString(v, op, cv.StringVal)
		}
	case *hydrapb.TreasureFilter_BoolVal:
		if v, ok := fieldVal.(bool); ok {
			ref := cv.BoolVal == hydrapb.Boolean_TRUE
			return compareBoolRaw(v, op, ref)
		}
	}

	return false
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
func extractFieldByPath(m map[string]interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	var current interface{} = m
	for _, part := range parts {
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
// All use Relational.Operator (EQUAL, NOT_EQUAL, GREATER_THAN, etc.)

func compareInt32(actual int32, op hydrapb.Relational_Operator, ref int32) bool {
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

func compareInt64(actual int64, op hydrapb.Relational_Operator, ref int64) bool {
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

func compareUint32(actual uint32, op hydrapb.Relational_Operator, ref uint32) bool {
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

func compareUint64(actual uint64, op hydrapb.Relational_Operator, ref uint64) bool {
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

func compareFloat32(actual float32, op hydrapb.Relational_Operator, ref float32) bool {
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

func compareFloat64(actual float64, op hydrapb.Relational_Operator, ref float64) bool {
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

func compareString(actual string, op hydrapb.Relational_Operator, ref string) bool {
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
	case hydrapb.Relational_CONTAINS:
		return strings.Contains(actual, ref)
	case hydrapb.Relational_NOT_CONTAINS:
		return !strings.Contains(actual, ref)
	case hydrapb.Relational_STARTS_WITH:
		return strings.HasPrefix(actual, ref)
	case hydrapb.Relational_ENDS_WITH:
		return strings.HasSuffix(actual, ref)
	default:
		return false
	}
}

func compareBool(actual hydrapb.Boolean_Type, op hydrapb.Relational_Operator, ref hydrapb.Boolean_Type) bool {
	switch op {
	case hydrapb.Relational_EQUAL:
		return actual == ref
	case hydrapb.Relational_NOT_EQUAL:
		return actual != ref
	default:
		// GT/LT/GTE/LTE don't make sense for booleans
		return false
	}
}

// compareBoolRaw compares raw Go booleans (used for BytesField extraction).
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
