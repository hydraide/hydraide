package migrator

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"io"
)

// ByteReader is a simple byte reader for parsing V1 binary data
type ByteReader struct {
	data   []byte
	offset int
}

// NewByteReader creates a new ByteReader
func NewByteReader(data []byte) *ByteReader {
	return &ByteReader{data: data}
}

// ReadUint32 reads a big-endian uint32
func (r *ByteReader) ReadUint32() (uint32, error) {
	if r.offset+4 > len(r.data) {
		return 0, io.EOF
	}

	value := binary.BigEndian.Uint32(r.data[r.offset : r.offset+4])
	r.offset += 4
	return value, nil
}

// ReadBytes reads n bytes from the reader
func (r *ByteReader) ReadBytes(n int) ([]byte, error) {
	if r.offset+n > len(r.data) {
		return nil, io.ErrUnexpectedEOF
	}

	result := make([]byte, n)
	copy(result, r.data[r.offset:r.offset+n])
	r.offset += n
	return result, nil
}

// Remaining returns the number of bytes remaining
func (r *ByteReader) Remaining() int {
	return len(r.data) - r.offset
}

// Skip skips n bytes
func (r *ByteReader) Skip(n int) error {
	if r.offset+n > len(r.data) {
		return io.EOF
	}
	r.offset += n
	return nil
}

// GobDecoder wraps gob decoding for treasure data
type GobDecoder struct {
	decoder *gob.Decoder
}

// NewGobDecoder creates a new GobDecoder
func NewGobDecoder(data []byte) *GobDecoder {
	return &GobDecoder{
		decoder: gob.NewDecoder(bytes.NewReader(data)),
	}
}

// Decode decodes the gob data into the provided interface
func (d *GobDecoder) Decode(v interface{}) error {
	return d.decoder.Decode(v)
}

// TreasureKeyExtractor extracts just the key from a GOB-encoded treasure
// This is optimized to not decode the entire treasure
type TreasureKeyExtractor struct {
	data []byte
}

// NewTreasureKeyExtractor creates a new extractor
func NewTreasureKeyExtractor(data []byte) *TreasureKeyExtractor {
	return &TreasureKeyExtractor{data: data}
}

// ExtractKey extracts the key from the treasure data
func (e *TreasureKeyExtractor) ExtractKey() (string, error) {
	if len(e.data) == 0 {
		return "", errors.New("empty data")
	}

	// Simple model that matches treasure.Model structure for GOB decoding
	type SimpleTreasureModel struct {
		Key string
		// We don't need other fields for key extraction
	}

	var model SimpleTreasureModel
	decoder := gob.NewDecoder(bytes.NewReader(e.data))
	if err := decoder.Decode(&model); err != nil {
		return "", err
	}

	return model.Key, nil
}

// V1FileParser parses V1 format data files
type V1FileParser struct {
	data []byte
}

// NewV1FileParser creates a new parser
func NewV1FileParser(data []byte) *V1FileParser {
	return &V1FileParser{data: data}
}

// ParseSegments parses all segments from the V1 file
func (p *V1FileParser) ParseSegments() ([][]byte, error) {
	if len(p.data) == 0 {
		return nil, nil
	}

	var segments [][]byte
	reader := NewByteReader(p.data)

	for reader.Remaining() > 0 {
		// Try to read segment length
		length, err := reader.ReadUint32()
		if err != nil {
			if err == io.EOF {
				break
			}
			return segments, err
		}

		// Zero-length segment, skip
		if length == 0 {
			continue
		}

		// Sanity check on length
		if int(length) > reader.Remaining() {
			return segments, errors.New("segment length exceeds remaining data")
		}

		// Read segment data
		segment, err := reader.ReadBytes(int(length))
		if err != nil {
			return segments, err
		}

		segments = append(segments, segment)
	}

	return segments, nil
}
