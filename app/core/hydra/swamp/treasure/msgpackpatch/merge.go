package msgpackpatch

import (
	"bytes"
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

// applyMerge performs a shallow merge of a msgpack-encoded map (op.Value) into
// the target map at path. Top-level keys in op.Value override the same keys
// in the target; non-conflicting target keys retain their original encoding.
//
// Missing target with a missing intermediate path: auto-creates empty map
// chain, then materializes the merged map.
func applyMerge(skel *Skeleton, op Op, path *Path) error {
	if len(op.Value) == 0 {
		return fmt.Errorf("%w: MERGE requires Value", ErrInvalidOp)
	}
	patchFields, err := extractTopLevelFields(op.Value)
	if err != nil {
		return err
	}

	cur, err := path.Resolve(skel)
	if err != nil {
		return err
	}

	// Case 1: existing target — must be a map.
	if cur.Target != nil {
		if cur.Target.Kind != KindMap {
			return fmt.Errorf("%w: MERGE target is not a map", ErrTypeMismatch)
		}
		mergeFieldsInto(cur.Target, patchFields)
		return nil
	}

	// Case 2 / 3: missing target — auto-create intermediate maps and the
	// final map.
	finalSeg := path.Segments[len(path.Segments)-1]
	if finalSeg.Kind != SegField {
		return fmt.Errorf("%w: MERGE final segment must be a field name", ErrPathInvalid)
	}
	parent := cur.Parent
	for i := cur.MissingAt; i < len(path.Segments)-1; i++ {
		seg := path.Segments[i]
		if seg.Kind != SegField {
			return fmt.Errorf("%w: cannot auto-create non-field segment", ErrPathInvalid)
		}
		newMap := &Skeleton{Kind: KindMap}
		parent.MapFields = append(parent.MapFields, MapField{Key: seg.Field, Value: newMap})
		parent = newMap
	}
	if parent.Kind != KindMap {
		return fmt.Errorf("%w: MERGE parent is not a map", ErrTypeMismatch)
	}
	target := &Skeleton{Kind: KindMap}
	mergeFieldsInto(target, patchFields)
	parent.MapFields = append(parent.MapFields, MapField{Key: finalSeg.Field, Value: target})
	return nil
}

// mergeFieldsInto applies patchFields onto target: matching keys overwrite,
// new keys are appended. patchFields' bytes are copied so the caller's blob
// can be safely discarded.
func mergeFieldsInto(target *Skeleton, patchFields []rawField) {
	for _, pf := range patchFields {
		raw := append([]byte(nil), pf.value...)
		newSkel := newLeaf(raw)
		if idx := findField(target, pf.key); idx >= 0 {
			target.MapFields[idx].Value = newSkel
		} else {
			target.MapFields = append(target.MapFields, MapField{Key: pf.key, Value: newSkel})
		}
	}
}

// rawField is a key + raw msgpack value bytes pair extracted from a top-level
// map blob. The value bytes are a slice into the source blob.
type rawField struct {
	key   string
	value []byte
}

// extractTopLevelFields parses blob as a msgpack map and returns each entry
// as (key, raw value bytes). Non-map blobs return ErrTypeMismatch. The
// returned value slices reference blob; copy them if the caller plans to
// retain them past blob's lifetime.
func extractTopLevelFields(blob []byte) ([]rawField, error) {
	if len(blob) == 0 {
		return nil, fmt.Errorf("%w: empty MERGE value", ErrInvalidMsgpack)
	}
	if !isMapCode(blob[0]) {
		return nil, fmt.Errorf("%w: MERGE value is not a map", ErrTypeMismatch)
	}
	r := bytes.NewReader(blob)
	dec := msgpack.NewDecoder(r)
	total := len(blob)
	n, err := dec.DecodeMapLen()
	if err != nil {
		return nil, wrapInvalid(err)
	}
	out := make([]rawField, 0, n)
	for i := 0; i < n; i++ {
		code, err := dec.PeekCode()
		if err != nil {
			return nil, wrapInvalid(err)
		}
		if !isStringCode(code) {
			return nil, ErrNonStringKey
		}
		key, err := dec.DecodeString()
		if err != nil {
			return nil, wrapInvalid(err)
		}
		startPos := total - r.Len()
		if err := dec.Skip(); err != nil {
			return nil, wrapInvalid(err)
		}
		endPos := total - r.Len()
		out = append(out, rawField{key: key, value: blob[startPos:endPos]})
	}
	return out, nil
}
