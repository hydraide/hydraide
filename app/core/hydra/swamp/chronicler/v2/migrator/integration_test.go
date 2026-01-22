package migrator

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/compressor"
	v2 "github.com/hydraide/hydraide/app/core/hydra/swamp/chronicler/v2"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type integrationTestDataDir struct {
	basePath     string
	dataPath     string
	settingsPath string
}

func setupIntegrationTestDataDir(t *testing.T) *integrationTestDataDir {
	t.Helper()
	basePath := filepath.Join(os.TempDir(), fmt.Sprintf("hydraide_integration_test_%d", time.Now().UnixNano()))
	td := &integrationTestDataDir{
		basePath:     basePath,
		dataPath:     filepath.Join(basePath, "data"),
		settingsPath: filepath.Join(basePath, "settings"),
	}
	require.NoError(t, os.MkdirAll(td.dataPath, 0755))
	require.NoError(t, os.MkdirAll(td.settingsPath, 0755))
	settings := map[string]interface{}{"engine": "V2"}
	settingsData, _ := json.MarshalIndent(settings, "", "  ")
	require.NoError(t, os.WriteFile(filepath.Join(td.settingsPath, "settings.json"), settingsData, 0644))
	return td
}

func (td *integrationTestDataDir) cleanup() {
	if td.basePath != "" {
		_ = os.RemoveAll(td.basePath)
	}
}

func (td *integrationTestDataDir) createV1Swamp(t *testing.T, swampName string, treasures map[string]string) string {
	t.Helper()
	hash := fmt.Sprintf("%x", time.Now().UnixNano())[:16]
	prefix := hash[:3]
	swampFolder := filepath.Join(td.dataPath, "0", prefix, hash)
	require.NoError(t, os.MkdirAll(swampFolder, 0755))
	metaPath := filepath.Join(swampFolder, "meta")
	metaFile, err := os.Create(metaPath)
	require.NoError(t, err)
	type MetaModel struct{ SwampName string }
	meta := MetaModel{SwampName: swampName}
	require.NoError(t, gob.NewEncoder(metaFile).Encode(&meta))
	metaFile.Close()
	if len(treasures) > 0 {
		dataFileName := fmt.Sprintf("%s-%s", hash[:8], "0001")
		dataPath := filepath.Join(swampFolder, dataFileName)
		var segments bytes.Buffer
		for key, value := range treasures {
			strVal := value
			model := treasure.Model{
				Key:       key,
				CreatedAt: time.Now().UnixNano(),
				Content:   &treasure.Content{String: &strVal},
			}
			var gobBuf bytes.Buffer
			require.NoError(t, gob.NewEncoder(&gobBuf).Encode(&model))
			gobData := gobBuf.Bytes()
			length := uint32(len(gobData))
			segments.WriteByte(byte(length))
			segments.WriteByte(byte(length >> 8))
			segments.WriteByte(byte(length >> 16))
			segments.WriteByte(byte(length >> 24))
			segments.Write(gobData)
		}
		comp := compressor.New(compressor.Snappy)
		compressed, err := comp.Compress(segments.Bytes())
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(dataPath, compressed, 0644))
	}
	return swampFolder
}

func (td *integrationTestDataDir) createV2Swamp(t *testing.T, swampName string, treasures map[string]string) string {
	t.Helper()
	hash := fmt.Sprintf("%x", time.Now().UnixNano())[:16]
	prefix := hash[:3]
	folderPath := filepath.Join(td.dataPath, "0", prefix)
	require.NoError(t, os.MkdirAll(folderPath, 0755))
	hydFilePath := filepath.Join(folderPath, hash+".hyd")
	writer, err := v2.NewFileWriter(hydFilePath, v2.DefaultMaxBlockSize)
	require.NoError(t, err)
	metaEntry := v2.Entry{Operation: v2.OpMetadata, Key: "__swamp_meta__", Data: []byte(swampName)}
	require.NoError(t, writer.WriteEntry(metaEntry))
	for key, value := range treasures {
		strVal := value
		model := treasure.Model{
			Key:       key,
			CreatedAt: time.Now().UnixNano(),
			Content:   &treasure.Content{String: &strVal},
		}
		var gobBuf bytes.Buffer
		require.NoError(t, gob.NewEncoder(&gobBuf).Encode(&model))
		entry := v2.Entry{Operation: v2.OpInsert, Key: key, Data: gobBuf.Bytes()}
		require.NoError(t, writer.WriteEntry(entry))
	}
	require.NoError(t, writer.Close())
	return hydFilePath
}

func TestIntegration_MigrateDryRun(t *testing.T) {
	td := setupIntegrationTestDataDir(t)
	defer td.cleanup()
	testTreasures := map[string]string{"key1": "value1", "key2": "value2"}
	v1SwampFolder := td.createV1Swamp(t, "test/migrate/dryrun", testTreasures)
	slog.Info("=== INTEGRATION MIGRATE DRY-RUN TEST ===")
	v2CountBefore := countHydFiles(t, td.dataPath)
	migrator, err := New(Config{DataPath: td.dataPath, DryRun: true})
	require.NoError(t, err)
	result, err := migrator.Run()
	require.NoError(t, err)
	slog.Info("Dry-run result", "processed", result.ProcessedSwamps)
	v2CountAfter := countHydFiles(t, td.dataPath)
	assert.Equal(t, v2CountBefore, v2CountAfter, "No V2 files should be created in dry-run")
	_, err = os.Stat(filepath.Join(v1SwampFolder, "meta"))
	assert.NoError(t, err, "V1 meta should still exist")
	slog.Info("TestIntegration_MigrateDryRun completed!")
}

func TestIntegration_Size(t *testing.T) {
	td := setupIntegrationTestDataDir(t)
	defer td.cleanup()
	slog.Info("=== INTEGRATION SIZE TEST ===")
	td.createV1Swamp(t, "test/size/v1", map[string]string{"k1": "v1"})
	time.Sleep(5 * time.Millisecond)
	td.createV2Swamp(t, "test/size/v2", map[string]string{"k2": "v2"})
	var totalSize int64
	var v1Files, v2Files int
	_ = filepath.Walk(td.dataPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		totalSize += info.Size()
		if strings.HasSuffix(path, ".hyd") {
			v2Files++
		} else if info.Name() != "meta" {
			v1Files++
		}
		return nil
	})
	slog.Info("Size results", "total", totalSize, "v1", v1Files, "v2", v2Files)
	assert.Greater(t, totalSize, int64(0))
	assert.Equal(t, 1, v1Files)
	assert.Equal(t, 1, v2Files)
	slog.Info("TestIntegration_Size completed!")
}

func TestIntegration_V2FileIntegrity(t *testing.T) {
	td := setupIntegrationTestDataDir(t)
	defer td.cleanup()
	slog.Info("=== INTEGRATION V2 FILE INTEGRITY TEST ===")
	td.createV2Swamp(t, "test/integrity/s1", map[string]string{"k1": "v1"})
	time.Sleep(5 * time.Millisecond)
	td.createV2Swamp(t, "test/integrity/s2", map[string]string{"k2": "v2"})
	hydFiles := findHydFiles(t, td.dataPath)
	assert.Equal(t, 2, len(hydFiles))
	for _, hydFile := range hydFiles {
		file, err := os.Open(hydFile)
		require.NoError(t, err)
		magic := make([]byte, 4)
		n, err := file.Read(magic)
		file.Close()
		require.NoError(t, err)
		require.Equal(t, 4, n)
		assert.Equal(t, []byte{'H', 'Y', 'D', 'R'}, magic)
		slog.Info("V2 file verified", "path", hydFile)
	}
	slog.Info("TestIntegration_V2FileIntegrity completed!")
}

func TestIntegration_FullMigration(t *testing.T) {
	td := setupIntegrationTestDataDir(t)
	defer td.cleanup()
	slog.Info("=== INTEGRATION FULL MIGRATION TEST ===")
	v1Folder1 := td.createV1Swamp(t, "test/mig/s1", map[string]string{"k1": "v1", "k2": "v2"})
	time.Sleep(5 * time.Millisecond)
	v1Folder2 := td.createV1Swamp(t, "test/mig/s2", map[string]string{"k3": "v3"})
	migrator, err := New(Config{DataPath: td.dataPath, DeleteOld: true, Verify: true})
	require.NoError(t, err)
	result, err := migrator.Run()
	require.NoError(t, err)
	slog.Info("Migration result", "processed", result.ProcessedSwamps, "ok", result.SuccessfulSwamps, "fail", len(result.FailedSwamps))
	assert.Equal(t, int64(2), result.ProcessedSwamps)
	assert.Equal(t, int64(2), result.SuccessfulSwamps)
	assert.Equal(t, 0, len(result.FailedSwamps))
	v2Count := countHydFiles(t, td.dataPath)
	assert.Equal(t, 2, v2Count)
	_, err = os.Stat(v1Folder1)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(v1Folder2)
	assert.True(t, os.IsNotExist(err))
	slog.Info("TestIntegration_FullMigration completed!")
}

func countHydFiles(t *testing.T, basePath string) int {
	t.Helper()
	count := 0
	_ = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err == nil && strings.HasSuffix(path, ".hyd") {
			count++
		}
		return nil
	})
	return count
}

func findHydFiles(t *testing.T, basePath string) []string {
	t.Helper()
	var files []string
	_ = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err == nil && strings.HasSuffix(path, ".hyd") {
			files = append(files, path)
		}
		return nil
	})
	return files
}
