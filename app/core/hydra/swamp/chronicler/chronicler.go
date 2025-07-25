// Package chronicler A chronicler is responsible for the retrieval and reading of level files, as swamp as managing
// the file system, located in the swamp. The chronicler is able to write new data, modify existing ones, and
// permanently delete data marked for deletion from the file system. Additionally, it can create non-existent directory
// structures and completely delete a swamp object from the file system. It has direct access to the compressor,
// allowing it to compress data and decode compressed data as swamp.
package chronicler

import (
	"github.com/google/uuid"
	"github.com/hydraide/hydraide/app/core/compressor"
	"github.com/hydraide/hydraide/app/core/filesystem"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/beacon"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/metadata"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/guard"
	"github.com/hydraide/hydraide/app/name"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sync"
)

type Chronicler interface {
	Write(treasures []treasure.Treasure)
	Load(indexObj beacon.Beacon)
	CreateDirectoryIfNotExists()
	Destroy()
	GetSwampAbsPath() string
	IsFilesystemInitiated() bool
	RegisterSaveFunction(swampSaveFunction func(t treasure.Treasure, guardID guard.ID) treasure.TreasureStatus)
	DontSendFilePointer() // if we don't want to send the file pointer to the swamp, because it will be closed soon
	// RegisterFilePointerFunction egy filepointer callback funkciót regisztrálhat a swamp
	RegisterFilePointerFunction(filePointerFunction func(event []*FileNameEvent) error)
}

type FileNameEvent struct {
	TreasureKey string
	FileName    string
}

const (
	SnappyCompressionPercent = 0.36 // the compression rate of the snappy compression method
	ActualFileKeyInMeta      = "actual"
)

type chronicler struct {
	mu                          sync.RWMutex
	swampName                   name.Name
	swampDataFolderPath         string // absolute path, where the .actual file is located
	maxFileSize                 int
	modifiedTreasuresForWrite   map[string]map[string]treasure.Treasure
	newTreasuresForWrite        []treasure.Treasure
	compressionMethod           compressor.Type
	sanctuaryAbsPath            string // the path of the hydra, where the swamp is located
	filesystemInitiated         bool   // true if the directory is initiated
	swampSaveFunction           func(t treasure.Treasure, guardID guard.ID) treasure.TreasureStatus
	dontSendFilePointer         bool // true if we don't want to send the file pointer to the swamp, because it will be closed soon
	compressedFolderExists      bool // true if the compressed folder exists
	filePointerCallbackFunction func(event []*FileNameEvent) error
	filesystemInterface         filesystem.Filesystem
	compressorInterface         compressor.Compressor
	metadataInterface           metadata.Metadata
	maxDepth                    int
}

// New creates new filesystem for a swamp
func New(swampDataFolderPath string, maxFileSize int64, maxDepth int, filesystemInterface filesystem.Filesystem, metaInterface metadata.Metadata) Chronicler {

	fsObj := &chronicler{
		swampDataFolderPath:       swampDataFolderPath,
		maxFileSize:               calculateOverloadSize(maxFileSize),
		modifiedTreasuresForWrite: make(map[string]map[string]treasure.Treasure),
		compressionMethod:         compressor.Snappy, // we used the Snappy compression method by default
		filesystemInterface:       filesystemInterface,
		metadataInterface:         metaInterface,
		maxDepth:                  maxDepth,
	}

	fsObj.compressorInterface = compressor.New(fsObj.compressionMethod)

	return fsObj

}

func (c *chronicler) DontSendFilePointer() {
	c.mu.Lock()
	defer c.mu.Unlock()
	// close the file pointer channel
	c.dontSendFilePointer = true
}

func (c *chronicler) RegisterFilePointerFunction(filePointerFunction func(event []*FileNameEvent) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.filePointerCallbackFunction = filePointerFunction
}

func (c *chronicler) RegisterSaveFunction(swampSaveFunction func(t treasure.Treasure, guardID guard.ID) treasure.TreasureStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.swampSaveFunction = swampSaveFunction
}

// GetSwampAbsPath returns the absolute path of the swamp's directory
func (c *chronicler) GetSwampAbsPath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.swampDataFolderPath
}

// CreateDirectoryIfNotExists creates the directory for the swamp if it is not exists
// the create method separated, because the Hydra can call Destroy method separately, so the New method can not create
// the swamp directory itself
func (c *chronicler) CreateDirectoryIfNotExists() {

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.filesystemInterface.CreateFolder(c.swampDataFolderPath); err != nil {
		slog.Error("can not create the directory for the swamp", "error", err)
	}

	c.filesystemInitiated = true

}

// Destroy deletes the swamp directory with all contained files
// this is a dangerous and a possibly long-running operation
// The function will send panic if we can not delete the folder
func (c *chronicler) Destroy() {

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.filesystemInterface.DeleteAllFiles(c.swampDataFolderPath); err != nil {
		slog.Error("can not delete the swamp directory", "error", err)
		return
	}

	// delete all unnecessary folders from the filesystem
	if err := c.filesystemInterface.DeleteFolder(c.swampDataFolderPath, c.maxDepth); err != nil {
		slog.Error("can not delete the swamp directory with all empty folders", "error", err)
	}

}

func (c *chronicler) IsFilesystemInitiated() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.filesystemInitiated
}

// Load the whole swamp from the filesystem with all contents and return with it
func (c *chronicler) Load(indexObj beacon.Beacon) {

	c.mu.Lock()
	defer c.mu.Unlock()

	contents, err := c.filesystemInterface.GetAllFileContents(c.swampDataFolderPath, metadata.MetaFile)
	if err != nil {
		slog.Error("can not read the actual file", "error", err)
		return
	}

	// iterating over the contents
	treasures := make(map[string]treasure.Treasure)

	for fileName, byteTreasures := range contents {
		for _, byteTreasure := range byteTreasures {

			treasureInterface := treasure.New(c.swampSaveFunction)
			guardID := treasureInterface.StartTreasureGuard(true, guard.BodyAuthID)
			errFromByte := treasureInterface.LoadFromByte(guardID, byteTreasure, fileName)
			if errFromByte != nil {
				return
			}
			treasureInterface.ReleaseTreasureGuard(guardID)
			treasures[treasureInterface.GetKey()] = treasureInterface

		}
	}

	// add all treasures to the index object
	indexObj.PushManyFromMap(treasures)

}

// Write all Treasures to the filesystem
func (c *chronicler) Write(treasures []treasure.Treasure) {

	c.mu.Lock()
	defer c.mu.Unlock()

	defer func() {
		c.modifiedTreasuresForWrite = make(map[string]map[string]treasure.Treasure) // clear the map
		c.newTreasuresForWrite = nil                                                // clear the slice
	}()

	var modifiedTreasuresWaitingForWriter bool
	var newTreasuresWaitingForWriter bool

	for _, selectedTreasure := range treasures {

		fileName := selectedTreasure.GetFileName()

		if fileName != nil {
			// add the treasure to the modified treasures map if it is not exists
			if c.modifiedTreasuresForWrite[*fileName] == nil {
				c.modifiedTreasuresForWrite[*fileName] = make(map[string]treasure.Treasure)
			}
			// add the modified treasure to the map
			c.modifiedTreasuresForWrite[*fileName][selectedTreasure.GetKey()] = selectedTreasure
			modifiedTreasuresWaitingForWriter = true
		} else {
			// add new treasures to the slice
			c.newTreasuresForWrite = append(c.newTreasuresForWrite, selectedTreasure)
			newTreasuresWaitingForWriter = true
		}
	}

	// process the modified treasures in the filesystem FIRST
	// this is important, because there may be some modified treasures in the actual folder, and we need to modify them first,
	// before the new treasure writer starts the working...
	if modifiedTreasuresWaitingForWriter {
		c.modifyTreasuresInFilesystem()
	}

	// if there is at least 1 new treasure that waits for the writer
	if newTreasuresWaitingForWriter {
		// write new treasures to the filesystem
		c.writeNewTreasures(c.newTreasuresForWrite)
	}

}

// modifyTreasuresInFilesystem modifies the treasures in the filesystem
func (c *chronicler) modifyTreasuresInFilesystem() {
	// write existing treasures
	for fileName, treasures := range c.modifiedTreasuresForWrite {
		// replace all treasures in the folder
		c.writeModifiedTreasures(fileName, treasures)
	}
}

// writeNewTreasures recursively writes the new treasures to the filesystem
func (c *chronicler) writeNewTreasures(newTreasures []treasure.Treasure) {

	workingFile := c.getActualFile()

	fileSize, _ := c.filesystemInterface.GetFileSize(workingFile)
	byteContent := make([][]byte, 0)

	// get the actual size of the file
	actualSizeInBytes := int(fileSize)
	// count how many newTreasures can be written to the file
	countTreasures := len(newTreasures)

	filePointerEvents := make([]*FileNameEvent, 0, countTreasures)

	// iterating over the newTreasures
	for k, t := range newTreasures {

		// convert the treasure to the binary data
		guardID := t.StartTreasureGuard(true, guard.BodyAuthID)
		b, convertErr := t.ConvertToByte(guardID)
		if convertErr != nil {
			t.ReleaseTreasureGuard(guardID)
			continue
		}
		t.ReleaseTreasureGuard(guardID)

		byteContent = append(byteContent, b)

		actualSizeInBytes += int(float64(len(b)) * SnappyCompressionPercent)

		// if the size of the folder is bigger than the max folder size we need to create a new folder and write the rest of the newTreasures
		if actualSizeInBytes > c.maxFileSize {
			// if there are more than one treasure that wait for the next folder
			if k+1 < countTreasures {
				// create new actual file
				c.createActualFile()
				// send the rest of the newTreasures to the next folder
				c.writeNewTreasures(newTreasures[k+1:])
				break
			}
		}

		// collect file pointer events
		filePointerEvents = append(filePointerEvents, &FileNameEvent{
			TreasureKey: t.GetKey(),
			FileName:    filepath.Base(workingFile),
		})

	}

	// write the data to filesystem
	if err := c.filesystemInterface.SaveFile(workingFile, byteContent, true); err != nil {
		slog.Error("can not write the new treasures to the filesystem", "error", err)
		return
	}

	// send file pointer events
	c.sendFilePointerEvents(filePointerEvents)

}

func (c *chronicler) sendFilePointerEvents(filePointerEvents []*FileNameEvent) {

	if c.dontSendFilePointer || len(filePointerEvents) == 0 {
		return
	}

	// send events if there is callback function registered
	if c.filePointerCallbackFunction != nil {
		if err := c.filePointerCallbackFunction(filePointerEvents); err != nil {
			// if the swamp is closed, but this is not an error
		}
	}

}

// replaceLineInFile replaces the selected lines in the folder
func (c *chronicler) writeModifiedTreasures(fileName string, treasures map[string]treasure.Treasure) {

	// the file path
	fp := filepath.Join(c.swampDataFolderPath, fileName)

	byteTreasures, err := c.filesystemInterface.GetFile(fp)
	if err != nil {
		slog.Error("can not read the file", "file absolute path", fp, "error", err)
		return
	}

	modifiedTreasures := make([][]byte, 0)

	for _, treasureData := range byteTreasures {

		treasureObject := treasure.New(c.swampSaveFunction)
		lockerID := treasureObject.StartTreasureGuard(true, guard.BodyAuthID)
		loadErr := treasureObject.LoadFromByte(lockerID, treasureData, fileName)
		treasureObject.ReleaseTreasureGuard(lockerID)
		if loadErr != nil {
			slog.Error("can not load the treasure from the binary data", "error", loadErr)
			continue
		}

		// 3. check the treasure key
		treasureKey := treasureObject.GetKey()
		if modifiedTreasure, exist := treasures[treasureKey]; exist {

			// treasure exists in the modified treasures, so we need to modify it
			treasureGuardID := modifiedTreasure.StartTreasureGuard(true, guard.BodyAuthID)
			modifiedBytes, convertErr := modifiedTreasure.ConvertToByte(treasureGuardID)
			modifiedTreasure.ReleaseTreasureGuard(treasureGuardID)
			if convertErr != nil {
				slog.Error("can not convert the modified treasure to byte", "error", convertErr)
				continue
			}

			// The treasure was permanently deleted, not just shadowDeleted,
			// so we also need to remove it from the filesystem.
			// Therefore, we do NOT write this treasure back to the file.
			if modifiedTreasure.GetDeletedAt() != 0 && modifiedTreasure.GetDeletedBy() != "" && !modifiedTreasure.GetShadowDelete() {
				continue
			}

			// Write back the modified treasure.
			modifiedTreasures = append(modifiedTreasures, modifiedBytes)

		} else {

			// The original treasure was not modified, so we write back the original data.
			modifiedTreasures = append(modifiedTreasures, treasureData)

		}

	}

	// All data has been deleted from the file, so we remove the file itself.
	if len(modifiedTreasures) == 0 {
		c.deleteFile(fp)
		return
	}

	// Write the modified treasures back to the file.
	if err := c.filesystemInterface.SaveFile(fp, modifiedTreasures, false); err != nil {
		slog.Error("can not write the modified treasures to the filesystem", "error", err, "file absolute path", fp)
		return
	}

}

func (c *chronicler) deleteFile(filePath string) {

	if err := c.filesystemInterface.DeleteFile(filePath); err != nil {
		slog.Error("can not delete the file", "error", err, "file absolute path", filePath)
	}

}

func (c *chronicler) decompressFile(fp string) []byte {

	compressedByteContent, err := os.ReadFile(fp)
	if err != nil {
		slog.Error("can not read the compressed file", "error", err, "file absolute path", fp)
		return nil
	}

	if len(compressedByteContent) == 0 {
		return nil // empty file
	}

	// decompress file
	compressorInterface := compressor.New(c.compressionMethod)
	byteContent, err := compressorInterface.Decompress(compressedByteContent)

	if err != nil {
		slog.Error("can not decompress the compressed file", "error", err, "file absolute path", fp)
		return nil
	}

	return byteContent

}

func (c *chronicler) getActualFile() (actualFilePath string) {
	actualFilePath = c.metadataInterface.GetKey(ActualFileKeyInMeta)
	if actualFilePath != "" {
		return filepath.Join(c.swampDataFolderPath, actualFilePath)
	}
	return c.createActualFile()
}

// createActualFile creates the actual folder in the filesystem
func (c *chronicler) createActualFile() (filePath string) {

	// create an empty file with new uuid
	actualFileUUID := uuid.NewString()
	filePath = filepath.Join(c.swampDataFolderPath, actualFileUUID)

	if err := c.filesystemInterface.SaveFile(filePath, nil, false); err != nil {
		slog.Error("can not create the actual file", "error", err, "file absolute path", filePath)
		return ""
	}

	// create the actual key in the metadata
	c.metadataInterface.SetKey(ActualFileKeyInMeta, actualFileUUID)

	return filePath

}

// calculateOverloadSize calculates the size of the overloaded folder because the folder will be compressed
// and this is the
func calculateOverloadSize(maxFileSizeBytes int64) int {
	overloadedSize := float64(maxFileSizeBytes) / SnappyCompressionPercent
	return int(math.Floor(overloadedSize*1) / 1)
}
