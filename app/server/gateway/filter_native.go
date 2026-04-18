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
		func(nf *hydrapb.NestedSliceWhereFilter) bool { return evaluateNativeNestedSliceWhereFilter(t, nf) },
	)
}

// evaluateNativeFilterGroupWithMeta evaluates a FilterGroup and collects metadata
// about which labeled filters matched and what vector scores were computed.
func evaluateNativeFilterGroupWithMeta(t treasure.Treasure, group *hydrapb.FilterGroup) (bool, *filterMatchMeta) {
	meta := &filterMatchMeta{}
	result := evaluateFilterGroupWithMeta(group,
		func(f *hydrapb.TreasureFilter) bool {
			matched := evaluateNativeSingleFilter(t, f)
			if matched && f.Label != nil && *f.Label != "" {
				meta.matchedLabels = append(meta.matchedLabels, *f.Label)
			}
			return matched
		},
		func(sg *hydrapb.FilterGroup) bool {
			matched, subMeta := evaluateNativeFilterGroupWithMeta(t, sg)
			if matched && subMeta != nil {
				meta.vectorScores = append(meta.vectorScores, subMeta.vectorScores...)
				meta.matchedLabels = append(meta.matchedLabels, subMeta.matchedLabels...)
			}
			return matched
		},
		func(pf *hydrapb.PhraseFilter) bool {
			matched := evaluateNativePhraseFilter(t, pf)
			if matched && pf.Label != nil && *pf.Label != "" {
				meta.matchedLabels = append(meta.matchedLabels, *pf.Label)
			}
			return matched
		},
		func(vf *hydrapb.VectorFilter) bool {
			matched, score := evaluateNativeVectorFilterWithScore(t, vf)
			if matched {
				meta.vectorScores = append(meta.vectorScores, score)
				if vf.Label != nil && *vf.Label != "" {
					meta.matchedLabels = append(meta.matchedLabels, *vf.Label)
				}
			}
			return matched
		},
		func(gf *hydrapb.GeoDistanceFilter) bool {
			matched := evaluateNativeGeoDistanceFilter(t, gf)
			if matched && gf.Label != nil && *gf.Label != "" {
				meta.matchedLabels = append(meta.matchedLabels, *gf.Label)
			}
			return matched
		},
		func(nf *hydrapb.NestedSliceWhereFilter) bool {
			matched := evaluateNativeNestedSliceWhereFilter(t, nf)
			if matched && nf.Label != nil && *nf.Label != "" {
				meta.matchedLabels = append(meta.matchedLabels, *nf.Label)
			}
			return matched
		},
	)
	return result, meta
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

	return evaluateBytesFieldFilterAgainstMap(decoded, filter)
}

// evaluateBytesFieldFilterAgainstMap evaluates a single TreasureFilter against
// a pre-decoded msgpack map. This allows reuse for both top-level evaluation
// and per-element evaluation in NestedSliceWhereFilter.
func evaluateBytesFieldFilterAgainstMap(decoded map[string]interface{}, filter *hydrapb.TreasureFilter) bool {
	op := filter.GetOperator()

	fieldVal := extractFieldByPath(decoded, *filter.BytesFieldPath)

	// [*] wildcard: any-match iteration over nested slice elements
	if ams, ok := fieldVal.(anyMatchSlice); ok {
		return evaluateAnyMatch(ams, op, filter)
	}

	// STRING_IN / INT32_IN / INT64_IN
	if op == hydrapb.Relational_STRING_IN {
		return evaluateStringIn(fieldVal, filter.StringInVals)
	}
	if op == hydrapb.Relational_INT32_IN {
		return evaluateInt32In(fieldVal, filter.Int32InVals)
	}
	if op == hydrapb.Relational_INT64_IN {
		return evaluateInt64In(fieldVal, filter.Int64InVals)
	}

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

	// SLICE_CONTAINS / SLICE_NOT_CONTAINS / SLICE_CONTAINS_SUBSTRING / SLICE_NOT_CONTAINS_SUBSTRING
	if op == hydrapb.Relational_SLICE_CONTAINS || op == hydrapb.Relational_SLICE_NOT_CONTAINS ||
		op == hydrapb.Relational_SLICE_CONTAINS_SUBSTRING || op == hydrapb.Relational_SLICE_NOT_CONTAINS_SUBSTRING {
		return evaluateSliceContains(fieldVal, op, filter)
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

// evaluateStringIn checks if the field value equals any of the given string values.
func evaluateStringIn(fieldVal interface{}, vals []string) bool {
	if fieldVal == nil || len(vals) == 0 {
		return false
	}
	s, ok := fieldVal.(string)
	if !ok {
		return false
	}
	for _, allowed := range vals {
		if s == allowed {
			return true
		}
	}
	return false
}

// evaluateInt32In checks if the field value equals any of the given int32 values.
func evaluateInt32In(fieldVal interface{}, vals []int32) bool {
	if fieldVal == nil || len(vals) == 0 {
		return false
	}
	v, ok := toInt64(fieldVal)
	if !ok {
		return false
	}
	for _, allowed := range vals {
		if v == int64(allowed) {
			return true
		}
	}
	return false
}

// evaluateInt64In checks if the field value equals any of the given int64 values.
func evaluateInt64In(fieldVal interface{}, vals []int64) bool {
	if fieldVal == nil || len(vals) == 0 {
		return false
	}
	v, ok := toInt64(fieldVal)
	if !ok {
		return false
	}
	for _, allowed := range vals {
		if v == allowed {
			return true
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

// evaluateNativeVectorFilterWithScore is like evaluateNativeVectorFilter but also
// returns the computed cosine similarity score.
func evaluateNativeVectorFilterWithScore(t treasure.Treasure, vf *hydrapb.VectorFilter) (bool, float32) {
	if vf == nil || len(vf.QueryVector) == 0 {
		return true, 0
	}

	if t.GetContentType() != treasure.ContentTypeByteArray {
		return false, 0
	}

	bytesVal, err := t.GetContentByteArray()
	if err != nil || bytesVal == nil || !isMsgpackEncoded(bytesVal) {
		return false, 0
	}

	decoded, err := decodeMsgpackToMap(unwrapMsgpack(bytesVal))
	if err != nil {
		return false, 0
	}

	fieldVal := extractFieldByPath(decoded, vf.BytesFieldPath)
	storedVector := toFloat32Slice(fieldVal)
	if len(storedVector) == 0 || len(storedVector) != len(vf.QueryVector) {
		return false, 0
	}

	similarity := dotProduct(storedVector, vf.QueryVector)
	return similarity >= vf.MinSimilarity, similarity
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
		func(nf *hydrapb.NestedSliceWhereFilter) bool {
			return evaluateNativeProfileNestedSliceWhereFilter(treasures, nf)
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

// evaluateNativeNestedSliceWhereFilter evaluates a NestedSliceWhereFilter against
// a treasure. It decodes the treasure's BytesVal, extracts the slice at SlicePath,
// and evaluates conditions per element according to the mode (ANY/ALL/NONE/COUNT).
func evaluateNativeNestedSliceWhereFilter(t treasure.Treasure, nf *hydrapb.NestedSliceWhereFilter) bool {
	if nf == nil {
		return true
	}

	if t.GetContentType() != treasure.ContentTypeByteArray {
		return nf.EvalMode == hydrapb.NestedSliceWhereFilter_ALL ||
			nf.EvalMode == hydrapb.NestedSliceWhereFilter_NONE ||
			(nf.EvalMode == hydrapb.NestedSliceWhereFilter_COUNT && evaluateCountResult(0, nf))
	}

	bytesVal, err := t.GetContentByteArray()
	if err != nil || bytesVal == nil || !isMsgpackEncoded(bytesVal) {
		return nf.EvalMode == hydrapb.NestedSliceWhereFilter_ALL ||
			nf.EvalMode == hydrapb.NestedSliceWhereFilter_NONE ||
			(nf.EvalMode == hydrapb.NestedSliceWhereFilter_COUNT && evaluateCountResult(0, nf))
	}

	decoded, err := decodeMsgpackToMap(unwrapMsgpack(bytesVal))
	if err != nil {
		return nf.EvalMode == hydrapb.NestedSliceWhereFilter_ALL ||
			nf.EvalMode == hydrapb.NestedSliceWhereFilter_NONE ||
			(nf.EvalMode == hydrapb.NestedSliceWhereFilter_COUNT && evaluateCountResult(0, nf))
	}

	return evaluateNestedSliceWhereAgainstMap(decoded, nf)
}

// evaluateNestedSliceWhereAgainstMap evaluates a NestedSliceWhereFilter against
// a pre-decoded msgpack map. Extracted for reuse in profile mode and nested evaluation.
func evaluateNestedSliceWhereAgainstMap(decoded map[string]interface{}, nf *hydrapb.NestedSliceWhereFilter) bool {
	sliceVal := extractFieldByPath(decoded, nf.SlicePath)
	arr, ok := sliceVal.([]interface{})
	if !ok || len(arr) == 0 {
		// No elements: ANY→false, ALL→true, NONE→true, COUNT→compare 0
		switch nf.EvalMode {
		case hydrapb.NestedSliceWhereFilter_ANY:
			return false
		case hydrapb.NestedSliceWhereFilter_ALL, hydrapb.NestedSliceWhereFilter_NONE:
			return true
		case hydrapb.NestedSliceWhereFilter_COUNT:
			return evaluateCountResult(0, nf)
		}
		return false
	}

	conditions := nf.Conditions
	if conditions == nil || (len(conditions.Filters) == 0 && len(conditions.SubGroups) == 0 &&
		len(conditions.NestedSliceWhereFilters) == 0) {
		// No conditions: all elements match
		switch nf.EvalMode {
		case hydrapb.NestedSliceWhereFilter_ANY:
			return true
		case hydrapb.NestedSliceWhereFilter_ALL:
			return true
		case hydrapb.NestedSliceWhereFilter_NONE:
			return false
		case hydrapb.NestedSliceWhereFilter_COUNT:
			return evaluateCountResult(int32(len(arr)), nf)
		}
		return true
	}

	var matchCount int32
	for _, elem := range arr {
		elemMap, ok := elem.(map[string]interface{})
		if !ok {
			// Non-map element: treated as not matching
			switch nf.EvalMode {
			case hydrapb.NestedSliceWhereFilter_ALL:
				return false
			}
			continue
		}

		matched := evaluateFilterGroupAgainstMap(elemMap, conditions)

		switch nf.EvalMode {
		case hydrapb.NestedSliceWhereFilter_ANY:
			if matched {
				return true
			}
		case hydrapb.NestedSliceWhereFilter_ALL:
			if !matched {
				return false
			}
		case hydrapb.NestedSliceWhereFilter_NONE:
			if matched {
				return false
			}
		case hydrapb.NestedSliceWhereFilter_COUNT:
			if matched {
				matchCount++
			}
		}
	}

	switch nf.EvalMode {
	case hydrapb.NestedSliceWhereFilter_ANY:
		return false
	case hydrapb.NestedSliceWhereFilter_ALL:
		return true
	case hydrapb.NestedSliceWhereFilter_NONE:
		return true
	case hydrapb.NestedSliceWhereFilter_COUNT:
		return evaluateCountResult(matchCount, nf)
	}
	return false
}

// evaluateCountResult compares a count of matching elements against the
// NestedSliceWhereFilter's CountOperator and CountValue.
func evaluateCountResult(count int32, nf *hydrapb.NestedSliceWhereFilter) bool {
	return compareOrdered(int64(count), nf.CountOperator, int64(nf.CountValue))
}

// evaluateFilterGroupAgainstMap evaluates a FilterGroup against a pre-decoded
// msgpack map. Used for per-element evaluation in NestedSliceWhereFilter.
// Only supports BytesField filters and nested NestedSliceWhereFilters within conditions.
func evaluateFilterGroupAgainstMap(decoded map[string]interface{}, group *hydrapb.FilterGroup) bool {
	if group == nil {
		return true
	}

	hasFilters := len(group.Filters) > 0
	hasSubGroups := len(group.SubGroups) > 0
	hasNestedSliceWhereFilters := len(group.NestedSliceWhereFilters) > 0

	if !hasFilters && !hasSubGroups && !hasNestedSliceWhereFilters {
		return true
	}

	if group.Logic == hydrapb.FilterLogic_OR {
		for _, f := range group.Filters {
			if f.BytesFieldPath != nil && evaluateBytesFieldFilterAgainstMap(decoded, f) {
				return true
			}
		}
		for _, sg := range group.SubGroups {
			if evaluateFilterGroupAgainstMap(decoded, sg) {
				return true
			}
		}
		for _, nf := range group.NestedSliceWhereFilters {
			if evaluateNestedSliceWhereAgainstMap(decoded, nf) {
				return true
			}
		}
		return false
	}

	// AND (default)
	for _, f := range group.Filters {
		if f.BytesFieldPath != nil && !evaluateBytesFieldFilterAgainstMap(decoded, f) {
			return false
		}
	}
	for _, sg := range group.SubGroups {
		if !evaluateFilterGroupAgainstMap(decoded, sg) {
			return false
		}
	}
	for _, nf := range group.NestedSliceWhereFilters {
		if !evaluateNestedSliceWhereAgainstMap(decoded, nf) {
			return false
		}
	}
	return true
}

// evaluateNativeProfileNestedSliceWhereFilter evaluates a NestedSliceWhereFilter
// against a map of native treasures in profile mode.
func evaluateNativeProfileNestedSliceWhereFilter(treasures map[string]treasure.Treasure, nf *hydrapb.NestedSliceWhereFilter) bool {
	if nf == nil {
		return true
	}
	if nf.TreasureKey == nil || *nf.TreasureKey == "" {
		return false
	}
	t, exists := treasures[*nf.TreasureKey]
	if !exists {
		return nf.EvalMode == hydrapb.NestedSliceWhereFilter_ALL ||
			nf.EvalMode == hydrapb.NestedSliceWhereFilter_NONE ||
			(nf.EvalMode == hydrapb.NestedSliceWhereFilter_COUNT && evaluateCountResult(0, nf))
	}
	return evaluateNativeNestedSliceWhereFilter(t, nf)
}
