package chronicler

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/beacon"
	v2 "github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler/v2"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
)

// writeRecoveryBatches writes `batches` batches of `per` treasures, closing
// the chronicler after each batch so every batch lands in its own block.
func writeRecoveryBatches(t *testing.T, swampPath, swampName string, start, batches, per int) {
	t.Helper()
	for b := 0; b < batches; b++ {
		chron := NewV2WithName(swampPath, 10, swampName)
		chron.CreateDirectoryIfNotExists()
		bc := beacon.New()
		chron.Load(bc)
		chron.RegisterLiveCountFunction(bc.Count)
		batch := make([]treasure.Treasure, 0, per)
		for i := 0; i < per; i++ {
			idx := start + b*per + i
			tr := makeTrendizzLikeTreasure(fmt.Sprintf("rec%05d.hu", idx), idx)
			batch = append(batch, tr)
			bc.Add(tr)
		}
		chron.Write(batch)
		if err := chron.Close(); err != nil {
			t.Fatalf("close failed: %v", err)
		}
	}
}

func spliceFile(t *testing.T, path string, offset int64, extra []byte) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if offset < 0 {
		offset = int64(len(data))
	}
	out := append(append(append([]byte{}, data[:offset]...), extra...), data[offset:]...)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func readerStats(t *testing.T, path string) (int, v2.RecoveryStats) {
	t.Helper()
	reader, err := v2.NewFileReader(path)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	defer reader.Close()
	index, _, err := reader.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex: %v", err)
	}
	return len(index), reader.RecoveryStats()
}

func loadAndCheck(t *testing.T, swampPath, swampName string, n int) {
	t.Helper()
	chron := NewV2WithName(swampPath, 10, swampName)
	chron.CreateDirectoryIfNotExists()
	bc := beacon.New()
	chron.Load(bc)
	if bc.Count() != n {
		t.Fatalf("expected %d treasures after load, got %d", n, bc.Count())
	}
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("rec%05d.hu", i)
		tr := bc.Get(key)
		if tr == nil {
			t.Fatalf("missing key %s", key)
		}
		want := makeTrendizzLikeTreasure(key, i)
		got, _ := tr.GetContentString()
		exp, _ := want.GetContentString()
		if got != exp {
			t.Fatalf("key %s: content mismatch", key)
		}
	}
	if err := chron.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

// TestRecovery_ChroniclerV2_TornWriteIsRepairedOnLoad reproduces the
// production shape: an orphan 16-byte block header in the middle of the file
// (a torn two-step block write followed by later sessions appending), plus a
// torn tail. Load must return every key from every valid block, rewrite the
// file clean, and keep working for later writes.
func TestRecovery_ChroniclerV2_TornWriteIsRepairedOnLoad(t *testing.T) {
	swampPath := filepath.Join(t.TempDir(), "recovery-swamp")
	swampName := "test/recovery/swamp"
	hydPath := swampPath + ".hyd"
	const batches, per = 6, 20
	writeRecoveryBatches(t, swampPath, swampName, 0, batches, per)

	// Block offsets of the clean file.
	reader, err := v2.NewFileReader(hydPath)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	blocks, err := reader.ReadAllBlocks()
	_ = reader.Close()
	if err != nil {
		t.Fatalf("read blocks: %v", err)
	}
	if len(blocks) != batches {
		t.Fatalf("expected %d blocks, got %d", batches, len(blocks))
	}

	// Orphan header of a block that never got its data written.
	lostHeader, lostData, err := v2.CompressEntries([]v2.Entry{{Operation: v2.OpInsert, Key: "lost", Data: []byte("lost")}})
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	orphan := lostHeader.Serialize()
	// Torn tail: header plus part of the data.
	spliceFile(t, hydPath, -1, append(append([]byte{}, orphan...), lostData[:len(lostData)/2]...))
	spliceFile(t, hydPath, blocks[3].Offset, orphan)

	if _, st := readerStats(t, hydPath); st.SkippedRegions != 1 || st.SkippedBytes != v2.BlockHeaderSize || st.TailBytes == 0 {
		t.Fatalf("corruption not in place as expected: %+v", st)
	}

	// Load recovers everything and rewrites the file.
	loadAndCheck(t, swampPath, swampName, batches*per)
	if n, st := readerStats(t, hydPath); st.Recovered() || n != batches*per {
		t.Fatalf("file not clean after load: keys=%d stats=%+v", n, st)
	}

	// Later write + close + load keeps old and new data, file stays clean.
	writeRecoveryBatches(t, swampPath, swampName, batches*per, 1, per)
	loadAndCheck(t, swampPath, swampName, (batches+1)*per)
	if n, st := readerStats(t, hydPath); st.Recovered() || n != (batches+1)*per {
		t.Fatalf("file not clean after append: keys=%d stats=%+v", n, st)
	}
}

// TestRecovery_ChroniclerV2_TornTailOnly covers a file whose only damage is a
// torn block at EOF: the rewrite drops it before any writer appends.
func TestRecovery_ChroniclerV2_TornTailOnly(t *testing.T) {
	swampPath := filepath.Join(t.TempDir(), "recovery-tail")
	swampName := "test/recovery/tail"
	hydPath := swampPath + ".hyd"
	writeRecoveryBatches(t, swampPath, swampName, 0, 3, 10)

	lostHeader, _, err := v2.CompressEntries([]v2.Entry{{Operation: v2.OpInsert, Key: "lost", Data: []byte("lost")}})
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	spliceFile(t, hydPath, -1, lostHeader.Serialize())

	loadAndCheck(t, swampPath, swampName, 30)
	if _, st := readerStats(t, hydPath); st.Recovered() {
		t.Fatalf("torn tail not removed: %+v", st)
	}
	writeRecoveryBatches(t, swampPath, swampName, 30, 1, 10)
	loadAndCheck(t, swampPath, swampName, 40)
	if _, st := readerStats(t, hydPath); st.Recovered() {
		t.Fatalf("file damaged after append: %+v", st)
	}
}
