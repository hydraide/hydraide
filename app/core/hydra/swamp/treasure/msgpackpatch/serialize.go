package msgpackpatch

import (
	"bytes"
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

// Serialize emits the skeleton as a msgpack blob, sourcing leaf bytes from
// orig (the original blob the skeleton was parsed from) unless a leaf has
// been mutated and carries its own RawBytes.
//
// Container headers (map / array length prefixes) are re-emitted based on
// the current child count, so insertions and deletions are reflected.
func (s *Skeleton) Serialize(orig []byte) ([]byte, error) {
	var buf bytes.Buffer
	// Pre-size: at minimum, output is similar in size to the input blob.
	buf.Grow(len(orig) + 16)
	enc := msgpack.NewEncoder(&buf)
	if err := writeNode(s, orig, &buf, enc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeNode(s *Skeleton, orig []byte, w *bytes.Buffer, enc *msgpack.Encoder) error {
	switch s.Kind {
	case KindLeaf:
		if s.RawBytes != nil {
			_, err := w.Write(s.RawBytes)
			return err
		}
		_, err := w.Write(orig[s.LeafStart:s.LeafEnd])
		return err
	case KindMap:
		if err := enc.EncodeMapLen(len(s.MapFields)); err != nil {
			return err
		}
		for _, f := range s.MapFields {
			if err := enc.EncodeString(f.Key); err != nil {
				return err
			}
			if err := writeNode(f.Value, orig, w, enc); err != nil {
				return err
			}
		}
		return nil
	case KindArray:
		if err := enc.EncodeArrayLen(len(s.ArrayItems)); err != nil {
			return err
		}
		for _, item := range s.ArrayItems {
			if err := writeNode(item, orig, w, enc); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("msgpackpatch: unknown skeleton kind %d", s.Kind)
}
