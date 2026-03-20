package gateway

import (
	"sort"

	hydrapb "github.com/hydraide/hydraide/generated/hydraidepbgo"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// evaluateNativeFilterGroup evaluates a FilterGroup directly against a treasure.Treasure
// interface, avoiding the cost of proto conversion for treasures that will be filtered out.
func evaluateNativeFilterGroup(t treasure.Treasure, group *hydrapb.FilterGroup) bool {
	return evaluateFilterGroupWith(group,
		func(f *hydrapb.TreasureFilter) bool { return evaluateNativeSingleFilter(t, f) },
		func(sg *hydrapb.FilterGroup) bool { return evaluateNativeFilterGroup(t, sg) },
		func(pf *hydrapb.PhraseFilter) bool { return evaluateNativePhraseFilter(t, pf) },
		func(vf *hydrapb.VectorFilter) bool { return evaluateNativeVectorFilter(t, vf) },
		func(gf *hydrapb.GeoDistanceFilter) bool { return evaluateNativeGeoDistanceFilter(t, gf) },
	)
}

// evaluateNativeSingleFilter evaluates one TreasureFilter against a treasure.Treasure interface.
func evaluateNativeSingleFilter(t treasure.Treasure, filter *hydrapb.TreasureFilter) bool {
	op := filter.GetOperator()

	// BytesFieldPath mode: extract from msgpack-encoded BytesVal
	if filter.BytesFieldPath != nil {
		return evaluateNativeBytesFieldFilter(t, filter)
	}

	// IS_EMPTY / IS_NOT_EMPTY
	if op == hydrapb.Relational_IS_EMPTY || op == hydrapb.Relational_IS_NOT_EMPTY {
		isEmpty := nativeFieldIsEmpty(t, filter)
		if op == hydrapb.Relational_IS_EMPTY {
			return isEmpty
		}
		return !isEmpty
	}

	ct := t.GetContentType()

	switch cv := filter.GetCompareValue().(type) {
	case *hydrapb.TreasureFilter_Int8Val:
		if ct != treasure.ContentTypeInt8 {
			return false
		}
		val, err := t.GetContentInt8()
		if err != nil {
			return false
		}
		return compareOrdered(int32(val), op, cv.Int8Val)

	case *hydrapb.TreasureFilter_Int16Val:
		if ct != treasure.ContentTypeInt16 {
			return false
		}
		val, err := t.GetContentInt16()
		if err != nil {
			return false
		}
		return compareOrdered(int32(val), op, cv.Int16Val)

	case *hydrapb.TreasureFilter_Int32Val:
		if ct != treasure.ContentTypeInt32 {
			return false
		}
		val, err := t.GetContentInt32()
		if err != nil {
			return false
		}
		return compareOrdered(val, op, cv.Int32Val)

	case *hydrapb.TreasureFilter_Int64Val:
		if ct != treasure.ContentTypeInt64 {
			return false
		}
		val, err := t.GetContentInt64()
		if err != nil {
			return false
		}
		return compareOrdered(val, op, cv.Int64Val)

	case *hydrapb.TreasureFilter_Uint8Val:
		if ct != treasure.ContentTypeUint8 {
			return false
		}
		val, err := t.GetContentUint8()
		if err != nil {
			return false
		}
		return compareOrdered(uint32(val), op, cv.Uint8Val)

	case *hydrapb.TreasureFilter_Uint16Val:
		if ct != treasure.ContentTypeUint16 {
			return false
		}
		val, err := t.GetContentUint16()
		if err != nil {
			return false
		}
		return compareOrdered(uint32(val), op, cv.Uint16Val)

	case *hydrapb.TreasureFilter_Uint32Val:
		if ct != treasure.ContentTypeUint32 {
			return false
		}
		val, err := t.GetContentUint32()
		if err != nil {
			return false
		}
		return compareOrdered(val, op, cv.Uint32Val)

	case *hydrapb.TreasureFilter_Uint64Val:
		if ct != treasure.ContentTypeUint64 {
			return false
		}
		val, err := t.GetContentUint64()
		if err != nil {
			return false
		}
		return compareOrdered(val, op, cv.Uint64Val)

	case *hydrapb.TreasureFilter_Float32Val:
		if ct != treasure.ContentTypeFloat32 {
			return false
		}
		val, err := t.GetContentFloat32()
		if err != nil {
			return false
		}
		return compareOrdered(val, op, cv.Float32Val)

	case *hydrapb.TreasureFilter_Float64Val:
		if ct != treasure.ContentTypeFloat64 {
			return false
		}
		val, err := t.GetContentFloat64()
		if err != nil {
			return false
		}
		return compareOrdered(val, op, cv.Float64Val)

	case *hydrapb.TreasureFilter_StringVal:
		if ct != treasure.ContentTypeString {
			return false
		}
		val, err := t.GetContentString()
		if err != nil {
			return false
		}
		return compareString(val, op, cv.StringVal)

	case *hydrapb.TreasureFilter_BoolVal:
		if ct != treasure.ContentTypeBoolean {
			return false
		}
		val, err := t.GetContentBool()
		if err != nil {
			return false
		}
		ref := cv.BoolVal == hydrapb.Boolean_TRUE
		return compareBoolRaw(val, op, ref)

	case *hydrapb.TreasureFilter_CreatedAtVal:
		return compareNativeTimestamp(t.GetCreatedAt(), op, cv.CreatedAtVal)

	case *hydrapb.TreasureFilter_UpdatedAtVal:
		return compareNativeTimestamp(t.GetModifiedAt(), op, cv.UpdatedAtVal)

	case *hydrapb.TreasureFilter_ExpiredAtVal:
		return compareNativeTimestamp(t.GetExpirationTime(), op, cv.ExpiredAtVal)

	default:
		return false
	}
}

// nativeFieldIsEmpty checks whether a treasure field is empty based on the filter's CompareValue type.
func nativeFieldIsEmpty(t treasure.Treasure, filter *hydrapb.TreasureFilter) bool {
	ct := t.GetContentType()
	switch filter.GetCompareValue().(type) {
	case *hydrapb.TreasureFilter_Int8Val:
		return ct != treasure.ContentTypeInt8
	case *hydrapb.TreasureFilter_Int16Val:
		return ct != treasure.ContentTypeInt16
	case *hydrapb.TreasureFilter_Int32Val:
		return ct != treasure.ContentTypeInt32
	case *hydrapb.TreasureFilter_Int64Val:
		return ct != treasure.ContentTypeInt64
	case *hydrapb.TreasureFilter_Uint8Val:
		return ct != treasure.ContentTypeUint8
	case *hydrapb.TreasureFilter_Uint16Val:
		return ct != treasure.ContentTypeUint16
	case *hydrapb.TreasureFilter_Uint32Val:
		return ct != treasure.ContentTypeUint32
	case *hydrapb.TreasureFilter_Uint64Val:
		return ct != treasure.ContentTypeUint64
	case *hydrapb.TreasureFilter_Float32Val:
		return ct != treasure.ContentTypeFloat32
	case *hydrapb.TreasureFilter_Float64Val:
		return ct != treasure.ContentTypeFloat64
	case *hydrapb.TreasureFilter_StringVal:
		if ct != treasure.ContentTypeString {
			return true
		}
		val, err := t.GetContentString()
		return err != nil || val == ""
	case *hydrapb.TreasureFilter_BoolVal:
		return ct != treasure.ContentTypeBoolean
	case *hydrapb.TreasureFilter_CreatedAtVal:
		return t.GetCreatedAt() == 0
	case *hydrapb.TreasureFilter_UpdatedAtVal:
		return t.GetModifiedAt() == 0
	case *hydrapb.TreasureFilter_ExpiredAtVal:
		return t.GetExpirationTime() == 0
	default:
		return true
	}
}

// compareNativeTimestamp compares a treasure's UnixNano timestamp against a proto Timestamp.
func compareNativeTimestamp(nanos int64, op hydrapb.Relational_Operator, ref *timestamppb.Timestamp) bool {
	if nanos == 0 || ref == nil {
		return false
	}
	return compareOrdered(nanos, op, ref.AsTime().UnixNano())
}

// evaluateNativeBytesFieldFilter extracts a field from the treasure's msgpack-encoded
// byte array content and applies the filter.
func evaluateNativeBytesFieldFilter(t treasure.Treasure, filter *hydrapb.TreasureFilter) bool {
	op := filter.GetOperator()

	if t.GetContentType() != treasure.ContentTypeByteArray {
		return op == hydrapb.Relational_IS_EMPTY
	}

	bytesVal, err := t.GetContentByteArray()
	if err != nil || bytesVal == nil || !isMsgpackEncoded(bytesVal) {
		return op == hydrapb.Relational_IS_EMPTY
	}

	decoded, err := decodeMsgpackToMap(unwrapMsgpack(bytesVal))
	if err != nil {
		return op == hydrapb.Relational_IS_EMPTY
	}

	fieldVal := extractFieldByPath(decoded, *filter.BytesFieldPath)

	// IS_EMPTY / IS_NOT_EMPTY
	if op == hydrapb.Relational_IS_EMPTY || op == hydrapb.Relational_IS_NOT_EMPTY {
		isEmpty := fieldVal == nil
		if !isEmpty {
			if s, ok := fieldVal.(string); ok {
				isEmpty = s == ""
			}
		}
		if op == hydrapb.Relational_IS_EMPTY {
			return isEmpty
		}
		return !isEmpty
	}

	// HAS_KEY / HAS_NOT_KEY
	if op == hydrapb.Relational_HAS_KEY || op == hydrapb.Relational_HAS_NOT_KEY {
		mapVal, ok := fieldVal.(map[string]interface{})
		if !ok {
			return op == hydrapb.Relational_HAS_NOT_KEY
		}
		cv, ok := filter.GetCompareValue().(*hydrapb.TreasureFilter_StringVal)
		if !ok {
			return false
		}
		_, exists := mapVal[cv.StringVal]
		if op == hydrapb.Relational_HAS_KEY {
			return exists
		}
		return !exists
	}

	if fieldVal == nil {
		return false
	}

	switch cv := filter.GetCompareValue().(type) {
	case *hydrapb.TreasureFilter_Int8Val:
		if v, ok := toInt64(fieldVal); ok {
			return compareOrdered(v, op, int64(cv.Int8Val))
		}
	case *hydrapb.TreasureFilter_Int16Val:
		if v, ok := toInt64(fieldVal); ok {
			return compareOrdered(v, op, int64(cv.Int16Val))
		}
	case *hydrapb.TreasureFilter_Int32Val:
		if v, ok := toInt64(fieldVal); ok {
			return compareOrdered(v, op, int64(cv.Int32Val))
		}
	case *hydrapb.TreasureFilter_Int64Val:
		if v, ok := toInt64(fieldVal); ok {
			return compareOrdered(v, op, cv.Int64Val)
		}
	case *hydrapb.TreasureFilter_Uint8Val:
		if v, ok := toUint64(fieldVal); ok {
			return compareOrdered(v, op, uint64(cv.Uint8Val))
		}
	case *hydrapb.TreasureFilter_Uint16Val:
		if v, ok := toUint64(fieldVal); ok {
			return compareOrdered(v, op, uint64(cv.Uint16Val))
		}
	case *hydrapb.TreasureFilter_Uint32Val:
		if v, ok := toUint64(fieldVal); ok {
			return compareOrdered(v, op, uint64(cv.Uint32Val))
		}
	case *hydrapb.TreasureFilter_Uint64Val:
		if v, ok := toUint64(fieldVal); ok {
			return compareOrdered(v, op, cv.Uint64Val)
		}
	case *hydrapb.TreasureFilter_Float32Val:
		if v, ok := toFloat64(fieldVal); ok {
			return compareOrdered(v, op, float64(cv.Float32Val))
		}
	case *hydrapb.TreasureFilter_Float64Val:
		if v, ok := toFloat64(fieldVal); ok {
			return compareOrdered(v, op, cv.Float64Val)
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

// evaluateNativePhraseFilter checks phrase matching against the treasure's byte array content.
func evaluateNativePhraseFilter(t treasure.Treasure, pf *hydrapb.PhraseFilter) bool {
	if pf == nil || len(pf.Words) == 0 {
		return true
	}

	if t.GetContentType() != treasure.ContentTypeByteArray {
		return pf.Negate
	}

	bytesVal, err := t.GetContentByteArray()
	if err != nil || bytesVal == nil || !isMsgpackEncoded(bytesVal) {
		return pf.Negate
	}

	decoded, err := decodeMsgpackToMap(unwrapMsgpack(bytesVal))
	if err != nil {
		return pf.Negate
	}

	fieldVal := extractFieldByPath(decoded, pf.BytesFieldPath)
	wordIndex, ok := fieldVal.(map[string]interface{})
	if !ok {
		return pf.Negate
	}

	wordPositions := make([][]int64, len(pf.Words))
	for i, word := range pf.Words {
		posVal, exists := wordIndex[word]
		if !exists {
			return pf.Negate
		}
		positions := toInt64Slice(posVal)
		if len(positions) == 0 {
			return pf.Negate
		}
		sort.Slice(positions, func(a, b int) bool { return positions[a] < positions[b] })
		wordPositions[i] = positions
	}

	found := hasConsecutivePositions(wordPositions)
	if pf.Negate {
		return !found
	}
	return found
}

// evaluateNativeVectorFilter computes vector similarity against the treasure's byte array content.
func evaluateNativeVectorFilter(t treasure.Treasure, vf *hydrapb.VectorFilter) bool {
	if vf == nil || len(vf.QueryVector) == 0 {
		return true
	}

	if t.GetContentType() != treasure.ContentTypeByteArray {
		return false
	}

	bytesVal, err := t.GetContentByteArray()
	if err != nil || bytesVal == nil || !isMsgpackEncoded(bytesVal) {
		return false
	}

	decoded, err := decodeMsgpackToMap(unwrapMsgpack(bytesVal))
	if err != nil {
		return false
	}

	fieldVal := extractFieldByPath(decoded, vf.BytesFieldPath)
	storedVector := toFloat32Slice(fieldVal)
	if len(storedVector) == 0 || len(storedVector) != len(vf.QueryVector) {
		return false
	}

	similarity := dotProduct(storedVector, vf.QueryVector)
	return similarity >= vf.MinSimilarity
}

// evaluateNativeProfileFilterGroup evaluates a FilterGroup against a map of native treasures.
// In profile mode, each struct field is stored as a separate Treasure keyed by field name.
func evaluateNativeProfileFilterGroup(treasures map[string]treasure.Treasure, group *hydrapb.FilterGroup) bool {
	return evaluateFilterGroupWith(group,
		func(f *hydrapb.TreasureFilter) bool { return evaluateNativeProfileSingleFilter(treasures, f) },
		func(sg *hydrapb.FilterGroup) bool { return evaluateNativeProfileFilterGroup(treasures, sg) },
		func(pf *hydrapb.PhraseFilter) bool { return evaluateNativeProfilePhraseFilter(treasures, pf) },
		func(vf *hydrapb.VectorFilter) bool { return evaluateNativeProfileVectorFilter(treasures, vf) },
		func(gf *hydrapb.GeoDistanceFilter) bool {
			return evaluateNativeProfileGeoDistanceFilter(treasures, gf)
		},
	)
}

func evaluateNativeProfileSingleFilter(treasures map[string]treasure.Treasure, filter *hydrapb.TreasureFilter) bool {
	if filter.TreasureKey == nil || *filter.TreasureKey == "" {
		return false
	}
	t, exists := treasures[*filter.TreasureKey]
	if !exists {
		return filter.GetOperator() == hydrapb.Relational_IS_EMPTY
	}
	return evaluateNativeSingleFilter(t, filter)
}

func evaluateNativeProfilePhraseFilter(treasures map[string]treasure.Treasure, pf *hydrapb.PhraseFilter) bool {
	if pf.TreasureKey == nil || *pf.TreasureKey == "" {
		return false
	}
	t, exists := treasures[*pf.TreasureKey]
	if !exists {
		return pf.Negate
	}
	return evaluateNativePhraseFilter(t, pf)
}

func evaluateNativeProfileVectorFilter(treasures map[string]treasure.Treasure, vf *hydrapb.VectorFilter) bool {
	if vf.TreasureKey == nil || *vf.TreasureKey == "" {
		return false
	}
	t, exists := treasures[*vf.TreasureKey]
	if !exists {
		return false
	}
	return evaluateNativeVectorFilter(t, vf)
}

// evaluateNativeGeoDistanceFilter computes geographic distance against the treasure's
// byte array content using the Haversine formula with bounding box pre-filtering.
func evaluateNativeGeoDistanceFilter(t treasure.Treasure, gf *hydrapb.GeoDistanceFilter) bool {
	if gf == nil {
		return true
	}

	if t.GetContentType() != treasure.ContentTypeByteArray {
		return false
	}

	bytesVal, err := t.GetContentByteArray()
	if err != nil || bytesVal == nil || !isMsgpackEncoded(bytesVal) {
		return false
	}

	decoded, err := decodeMsgpackToMap(unwrapMsgpack(bytesVal))
	if err != nil {
		return false
	}

	latVal := extractFieldByPath(decoded, gf.LatFieldPath)
	lngVal := extractFieldByPath(decoded, gf.LngFieldPath)
	if latVal == nil || lngVal == nil {
		return false
	}

	lat, latOk := toFloat64(latVal)
	lng, lngOk := toFloat64(lngVal)
	if !latOk || !lngOk {
		return false
	}

	// Null Island — missing data
	if lat == 0 && lng == 0 {
		return false
	}

	// Bounding box pre-filter
	bb := newGeoBoundingBox(gf.RefLatitude, gf.RefLongitude, gf.RadiusKm)
	insideBox := bb.contains(lat, lng)

	if gf.Mode == hydrapb.GeoDistanceMode_INSIDE && !insideBox {
		return false
	}
	if gf.Mode == hydrapb.GeoDistanceMode_OUTSIDE && !insideBox {
		return true
	}

	// Exact Haversine
	distance := haversineDistance(gf.RefLatitude, gf.RefLongitude, lat, lng)

	if gf.Mode == hydrapb.GeoDistanceMode_INSIDE {
		return distance <= gf.RadiusKm
	}
	return distance > gf.RadiusKm
}

func evaluateNativeProfileGeoDistanceFilter(treasures map[string]treasure.Treasure, gf *hydrapb.GeoDistanceFilter) bool {
	if gf == nil || gf.TreasureKey == nil || *gf.TreasureKey == "" {
		return true
	}
	t, exists := treasures[*gf.TreasureKey]
	if !exists {
		return false
	}
	return evaluateNativeGeoDistanceFilter(t, gf)
}
