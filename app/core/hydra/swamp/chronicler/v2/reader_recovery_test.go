package v2

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// writeBlocksFile writes n blocks with entriesPerBlock insert entries each and
// returns the path plus the start offset of every block.
func writeBlocksFile(t *testing.T, n, entriesPerBlock int) (string, []int64) {
	t.Helper()
	filePath := filepath.Join(t.TempDir(), "recovery.hyd")
	writer, err := NewFileWriterWithName(filePath, DefaultMaxBlockSize, "test/recovery/swamp")
	if err != nil {
		t.Fatalf("create writer: %v", err)
	}
	for b := 0; b < n; b++ {
		for e := 0; e < entriesPerBlock; e++ {
			key := fmt.Sprintf("k-%03d-%03d", b, e)
			if err := writer.WriteEntry(Entry{Operation: OpInsert, Key: key, Data: []byte("value-" + key)}); err != nil {
				t.Fatalf("write entry: %v", err)
			}
		}
		if err := writer.Flush(); err != nil {
			t.Fatalf("flush: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	defer reader.Close()
	blocks, err := reader.ReadAllBlocks()
	if err != nil {
		t.Fatalf("read blocks: %v", err)
	}
	if len(blocks) != n {
		t.Fatalf("expected %d blocks, got %d", n, len(blocks))
	}
	offsets := make([]int64, n)
	for i, b := range blocks {
		offsets[i] = b.Offset
	}
	return filePath, offsets
}

// insertBytes splices extra bytes into the file at offset.
func insertBytes(t *testing.T, filePath string, offset int64, extra []byte) {
	t.Helper()
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	out := make([]byte, 0, len(data)+len(extra))
	out = append(out, data[:offset]...)
	out = append(out, extra...)
	out = append(out, data[offset:]...)
	if err := os.WriteFile(filePath, out, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

// appendBytes appends extra bytes at the end of the file.
func appendBytes(t *testing.T, filePath string, extra []byte) {
	t.Helper()
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	insertBytes(t, filePath, info.Size(), extra)
}

// orphanBlock returns the header and data of a real block that was never
// written into the file, the exact shape of a torn two-step write.
func orphanBlock(t *testing.T) ([]byte, []byte) {
	t.Helper()
	var entries []Entry
	for i := 0; i < 20; i++ {
		entries = append(entries, Entry{Operation: OpInsert, Key: fmt.Sprintf("lost-%d", i), Data: []byte(fmt.Sprintf("lost-value-%d", i))})
	}
	header, compressed, err := CompressEntries(entries)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	return header.Serialize(), compressed
}

// deterministicGarbage returns n pseudo-random bytes.
func deterministicGarbage(n int) []byte {
	out := make([]byte, n)
	x := uint32(2463534242)
	for i := range out {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		out[i] = byte(x)
	}
	return out
}

func loadIndexWithStats(t *testing.T, filePath string) (map[string][]byte, string, RecoveryStats) {
	t.Helper()
	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	defer reader.Close()
	index, name, err := reader.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}
	return index, name, reader.RecoveryStats()
}

func assertAllKeys(t *testing.T, index map[string][]byte, n, entriesPerBlock int) {
	t.Helper()
	if len(index) != n*entriesPerBlock {
		t.Fatalf("expected %d keys, got %d", n*entriesPerBlock, len(index))
	}
	for b := 0; b < n; b++ {
		for e := 0; e < entriesPerBlock; e++ {
			key := fmt.Sprintf("k-%03d-%03d", b, e)
			if string(index[key]) != "value-"+key {
				t.Fatalf("key %s: got %q", key, index[key])
			}
		}
	}
}

func TestRecovery_CleanFileHasZeroStats(t *testing.T) {
	filePath, _ := writeBlocksFile(t, 10, 5)
	index, name, stats := loadIndexWithStats(t, filePath)
	assertAllKeys(t, index, 10, 5)
	if name != "test/recovery/swamp" {
		t.Fatalf("unexpected swamp name %q", name)
	}
	if stats.Recovered() || stats.SkippedRegions != 0 || stats.SkippedBytes != 0 || stats.TailBytes != 0 {
		t.Fatalf("expected zero stats, got %+v", stats)
	}
	info, _ := os.Stat(filePath)
	if stats.ValidEnd != info.Size() {
		t.Fatalf("ValidEnd %d != file size %d", stats.ValidEnd, info.Size())
	}
}

func TestRecovery_OrphanHeaderInTheMiddle(t *testing.T) {
	const n, per = 12, 5
	for _, at := range []int{0, 1, 6, n - 1} {
		t.Run(fmt.Sprintf("before_block_%d", at), func(t *testing.T) {
			filePath, offsets := writeBlocksFile(t, n, per)
			orphanHeader, _ := orphanBlock(t)
			insertBytes(t, filePath, offsets[at], orphanHeader)

			index, _, stats := loadIndexWithStats(t, filePath)
			assertAllKeys(t, index, n, per)
			if stats.SkippedRegions != 1 || stats.SkippedBytes != BlockHeaderSize || stats.TailBytes != 0 {
				t.Fatalf("expected 1 region of 16 bytes, got %+v", stats)
			}
			info, _ := os.Stat(filePath)
			if stats.ValidEnd != info.Size() {
				t.Fatalf("ValidEnd %d != file size %d", stats.ValidEnd, info.Size())
			}
		})
	}
}

func TestRecovery_GarbageInTheMiddle(t *testing.T) {
	const n, per = 8, 4
	filePath, offsets := writeBlocksFile(t, n, per)
	garbage := deterministicGarbage(37)
	insertBytes(t, filePath, offsets[4], garbage)

	index, _, stats := loadIndexWithStats(t, filePath)
	assertAllKeys(t, index, n, per)
	if stats.SkippedRegions != 1 || stats.SkippedBytes != int64(len(garbage)) || stats.TailBytes != 0 {
		t.Fatalf("expected 1 region of %d bytes, got %+v", len(garbage), stats)
	}
}

func TestRecovery_MultipleGaps(t *testing.T) {
	const n, per = 10, 3
	filePath, offsets := writeBlocksFile(t, n, per)
	orphanHeader, _ := orphanBlock(t)
	garbage := deterministicGarbage(100)
	// Insert the later one first so the earlier offset stays valid.
	insertBytes(t, filePath, offsets[7], garbage)
	insertBytes(t, filePath, offsets[2], orphanHeader)

	index, _, stats := loadIndexWithStats(t, filePath)
	assertAllKeys(t, index, n, per)
	if stats.SkippedRegions != 2 || stats.SkippedBytes != int64(BlockHeaderSize+len(garbage)) {
		t.Fatalf("expected 2 regions of %d bytes, got %+v", BlockHeaderSize+len(garbage), stats)
	}
}

func TestRecovery_TornTailHeaderOnly(t *testing.T) {
	const n, per = 6, 5
	filePath, _ := writeBlocksFile(t, n, per)
	info, _ := os.Stat(filePath)
	cleanSize := info.Size()
	orphanHeader, _ := orphanBlock(t)
	appendBytes(t, filePath, orphanHeader)

	index, _, stats := loadIndexWithStats(t, filePath)
	assertAllKeys(t, index, n, per)
	if stats.SkippedRegions != 0 || stats.TailBytes != BlockHeaderSize || !stats.Recovered() {
		t.Fatalf("expected a 16 byte torn tail, got %+v", stats)
	}
	if stats.ValidEnd != cleanSize {
		t.Fatalf("ValidEnd %d != clean size %d", stats.ValidEnd, cleanSize)
	}
}

func TestRecovery_TornTailHeaderAndPartialData(t *testing.T) {
	const n, per = 6, 5
	filePath, _ := writeBlocksFile(t, n, per)
	info, _ := os.Stat(filePath)
	cleanSize := info.Size()
	orphanHeader, data := orphanBlock(t)
	partial := append(append([]byte{}, orphanHeader...), data[:len(data)/2]...)
	appendBytes(t, filePath, partial)

	index, _, stats := loadIndexWithStats(t, filePath)
	assertAllKeys(t, index, n, per)
	if stats.SkippedRegions != 0 || stats.TailBytes != int64(len(partial)) {
		t.Fatalf("expected a %d byte torn tail, got %+v", len(partial), stats)
	}
	if stats.ValidEnd != cleanSize {
		t.Fatalf("ValidEnd %d != clean size %d", stats.ValidEnd, cleanSize)
	}
}

func TestRecovery_TornTailShorterThanHeader(t *testing.T) {
	const n, per = 3, 2
	filePath, _ := writeBlocksFile(t, n, per)
	appendBytes(t, filePath, []byte{1, 2, 3, 4, 5})

	index, _, stats := loadIndexWithStats(t, filePath)
	assertAllKeys(t, index, n, per)
	if stats.TailBytes != 5 || stats.SkippedRegions != 0 {
		t.Fatalf("expected a 5 byte torn tail, got %+v", stats)
	}
}

func TestRecovery_ScanBlockHeadersAndFragmentation(t *testing.T) {
	const n, per = 12, 5
	filePath, offsets := writeBlocksFile(t, n, per)
	orphanHeader, _ := orphanBlock(t)
	insertBytes(t, filePath, offsets[5], orphanHeader)

	reader, err := NewFileReader(filePath)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	defer reader.Close()

	scan, err := reader.ScanBlockHeaders()
	if err != nil {
		t.Fatalf("ScanBlockHeaders: %v", err)
	}
	if scan.BlockCount != n || scan.TotalEntryCount != n*per {
		t.Fatalf("expected %d blocks / %d entries, got %+v", n, n*per, scan)
	}
	if st := reader.RecoveryStats(); st.SkippedRegions != 1 || st.SkippedBytes != BlockHeaderSize {
		t.Fatalf("scan stats: %+v", st)
	}

	frag, live, total, err := reader.CalculateFragmentation()
	if err != nil {
		t.Fatalf("CalculateFragmentation: %v", err)
	}
	if live != n*per || total != n*per || frag != 0 {
		t.Fatalf("unexpected fragmentation result: frag=%v live=%d total=%d", frag, live, total)
	}
}
