// Package settings provides the ability to load and reload the setting from the config.yaml file.
package settings

import (
	"encoding/json"
	"fmt"
	"github.com/hydraide/hydraide/app/core/settings/setting"
	"github.com/hydraide/hydraide/app/name"
	"log/slog"
	"os"
	"path"
	"sync"
	"time"
)

// Settings is the interface for managing configuration settings
type Settings interface {
	// GetHashFolderDepth returns the depth of the hash folder
	GetHashFolderDepth() int
	// GetMaxFoldersPerLevel returns the maximum number of folders in a hash folder
	GetMaxFoldersPerLevel() int
	// GetHydraAbsDataFolderPath returns the absolute path of the hydra data folder
	GetHydraAbsDataFolderPath() string
	// GetBySwampName loads the settings for a specific swamp based on its name.
	// Real-world scenario: When initializing a new swamp, you can use this function to apply pre-configured settings
	// for that specific swamp.
	GetBySwampName(swampName name.Name) setting.Setting
	// RegisterPattern registers a pattern for a swamp to the settings
	// useful when the hydra register a new Head to the system with new swamp patterns
	RegisterPattern(pattern name.Name, inMemorySwamp bool, closeAfterIdleSec int64, filesystemSettings *FileSystemSettings)
	// DeregisterPattern deregister a pattern from the settings
	DeregisterPattern(pattern name.Name)
	// CallbackAtChanges wait a callback function and the settigns will call it when the settings changed
	CallbackAtChanges(func()) chan bool
}

const (
	hydraDataFolderPath     = "/hydraide/data"
	hydraSettingsFolderPath = "/hydraide/settings"
	fileName                = "settings.json"
	writetestFile           = "writetest"
)

type settings struct {
	mu                 sync.RWMutex
	modelMutex         sync.RWMutex
	model              *Model
	virtualNodesFrom   int
	virtualNodesTo     int
	defaultSetting     setting.Setting // nem kell kimenteni, mert a beállító fileban benne van mindig
	callbackFunctions  []func()        // nem kell kimenteni, mert újra feliratkozik akinek kell
	patterns           map[string]setting.Setting
	streamPath         string
	automoverPath      string
	pluginPath         string
	maxDepthOfFolders  int
	maxFoldersPerLevel int
}

type Model struct {
	Patterns      map[string]*PatternModel `json:"patterns,omitempty"`
	StreamPath    string                   `json:"streamPath,omitempty"`
	AutoMoverPath string                   `json:"autoMoverPath,omitempty"`
}

type PatternModel struct {
	NameCanonicalForm string `json:"nameCanonicalForm,omitempty"`
	InMemory          bool   `json:"inMemory,omitempty"`
	CloseAfterIdleSec int64  `json:"closeAfterIdleSec,omitempty"`
	WriteIntervalSec  int64  `json:"writeIntervalSec,omitempty"`
	MaxFileSizeByte   int64  `json:"maxFileSizeByte,omitempty"`
}

// New creates a new instance of the setting
func New(maxDepthOfFolders int, maxFoldersPerLevel int) Settings {

	// ellenőrizzük, hogy az alapvető mentési könyvtárak léteznek-e és írhatóak-e
	// ha nem léteznek, akkor létrehozzuk azokat írható formában
	checkFolder(hydraDataFolderPath)
	checkFolder(hydraSettingsFolderPath)

	t := &settings{
		patterns: make(map[string]setting.Setting),
		model: &Model{
			Patterns: make(map[string]*PatternModel),
		},
		maxDepthOfFolders:  maxDepthOfFolders,
		maxFoldersPerLevel: maxFoldersPerLevel,
	}

	// load the saved settings from the filesystem at the startup
	if err := t.loadSettingsFromFilesystem(); err != nil {
		slog.Error("failed to load settings from filesystem", "error", err)
	}

	return t

}

func (s *settings) GetHashFolderDepth() int {
	return s.maxDepthOfFolders
}

func (s *settings) GetMaxFoldersPerLevel() int {
	return s.maxFoldersPerLevel
}

// GetHydraAbsDataFolderPath visszaadja a hydra alap adatmentési útvonalát
func (s *settings) GetHydraAbsDataFolderPath() string {
	// nem kell lockolni, mert ez egy konstans és az értéke nem változhat futásidő alatt,
	// így nem kell gátolni az egyidejű hozzáférést, ami lassítaná a rendszert
	return hydraDataFolderPath
}

// FileSystemSettings contains the settings for the filesystem-type swamps
type FileSystemSettings struct {
	// WriteIntervalSec is the time interval when the swamp will write the data to the filesystem
	WriteIntervalSec int64
	// MaxFileSizeByte is the maximum size of the file fragments of the swamp in bytes
	MaxFileSizeByte int64
}

// RegisterPattern registers a pattern for a swamp to the settings only if it is not exist
// inMemorySwamp is true if the swamp is in-memory type, otherwise it is false
// If the swamp is filesystem type, then the filesystemSettings must be set otherwise it is nil
func (s *settings) RegisterPattern(pattern name.Name, inMemorySwamp bool, closeAfterIdleSec int64, filesystemSettings *FileSystemSettings) {

	s.mu.Lock()
	defer s.mu.Unlock()

	swampSetting := &setting.SwampSetting{
		Pattern:           pattern,
		InMemory:          inMemorySwamp,
		CloseAfterIdleSec: time.Duration(closeAfterIdleSec) * time.Second,
	}

	// the swamp is filesystem type
	if !inMemorySwamp {

		// check if the pattern is already exist
		if _, ok := s.patterns[pattern.Get()]; ok {
			// check if the actual pattern setting is different from the new setting
			if s.patterns[pattern.Get()].GetCloseAfterIdle() == time.Duration(closeAfterIdleSec)*time.Second &&
				(filesystemSettings != nil &&
					(s.patterns[pattern.Get()].GetWriteInterval() == time.Duration(filesystemSettings.WriteIntervalSec)*time.Second &&
						s.patterns[pattern.Get()].GetMaxFileSizeByte() == filesystemSettings.MaxFileSizeByte)) {
				// do nothing, because the pattern is already exist and not changed
				// so, we don't need to save the settings to the filesystem
				return
			}
		}

		// create a new swamp setting
		s.patterns[pattern.Get()] = setting.New(swampSetting)
		if filesystemSettings != nil {
			swampSetting.WriteIntervalSec = time.Duration(filesystemSettings.WriteIntervalSec) * time.Second
			swampSetting.MaxFileSizeByte = filesystemSettings.MaxFileSizeByte
		}

	}

	// add the pattern to the Model
	func() {

		s.modelMutex.Lock()
		defer s.modelMutex.Unlock()

		pm := &PatternModel{
			NameCanonicalForm: pattern.Get(),
			InMemory:          inMemorySwamp,
		}

		if !inMemorySwamp {
			pm.WriteIntervalSec = filesystemSettings.WriteIntervalSec
			pm.MaxFileSizeByte = filesystemSettings.MaxFileSizeByte
		}

		pm.CloseAfterIdleSec = closeAfterIdleSec

		// set the pattern to the model
		s.model.Patterns[pattern.Get()] = pm

		if err := s.SaveSettingsToFilesystem(); err != nil {
			slog.Error("failed to save settings to filesystem", "error", err)
		}

	}()

	slog.Info("swamp pattern registered", "pattern", pattern.Get())

}

// DeregisterPattern deregister a pattern from the settings
func (s *settings) DeregisterPattern(pattern name.Name) {

	s.mu.Lock()
	defer s.mu.Unlock()

	func() {
		s.modelMutex.Lock()
		defer s.modelMutex.Unlock()
		delete(s.model.Patterns, pattern.Get())
		if err := s.SaveSettingsToFilesystem(); err != nil {
			slog.Error("failed to save settings to filesystem", "error", err)
		}
	}()

	delete(s.patterns, pattern.Get())

}

// GetBySwampName loads the setting of the swamp by the name of the swamp
func (s *settings) GetBySwampName(swampName name.Name) setting.Setting {

	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.patterns) > 0 {
		for _, pi := range s.patterns {
			// compare if the pattern is math with the swamp name
			if swampName.ComparePattern(pi.GetPattern()) {
				return pi
			}
		}
	}

	// ha nem találunk olyan beállítást, ami a megadott mintához tartozik, akkor visszaadjuk az alapértelmezett beállítást
	// ebben az esetben is a mentés helye nem változik, csak a beállítások lesznek alapértelmezettek
	return setting.New(&setting.SwampSetting{
		Pattern:           swampName,
		CloseAfterIdleSec: time.Duration(5) * time.Second,
		WriteIntervalSec:  time.Duration(1) * time.Second,
		MaxFileSizeByte:   65536, // 64KB
	})

}

func (s *settings) CallbackAtChanges(f func()) chan bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callbackFunctions = append(s.callbackFunctions, f)
	return nil
}

func (s *settings) SaveSettingsToFilesystem() error {

	data, err := json.MarshalIndent(s.model, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	filePath := path.Join(hydraSettingsFolderPath, fileName)

	err = os.MkdirAll(path.Join(hydraSettingsFolderPath), 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory path: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write to file system: %w", err)
	}

	return nil

}

func (s *settings) loadSettingsFromFilesystem() error {

	s.modelMutex.Lock()
	defer s.modelMutex.Unlock()

	filePath := path.Join(hydraSettingsFolderPath, fileName)
	data, err := os.ReadFile(filePath)

	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read file: %w", err)
	}

	if err := json.Unmarshal(data, &s.model); err != nil {
		return fmt.Errorf("failed to unmarshal settings: %w", err)
	}

	// Format the LoadedSettings as indented JSON
	formattedSettings, err := json.MarshalIndent(s.model, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings for logging: %w", err)
	}

	// Debug: Print the raw JSON data read from the file
	slog.Info("main settings loaded from filesystem successfully", "mainSettings", string(formattedSettings))

	// visszatöltjük a beállításokat a memóriába
	if s.model != nil {
		if s.model.Patterns != nil {
			for _, pattern := range s.model.Patterns {

				patternNameObj := name.Load(pattern.NameCanonicalForm)

				s.patterns[pattern.NameCanonicalForm] = setting.New(&setting.SwampSetting{
					Pattern:           patternNameObj,
					CloseAfterIdleSec: time.Duration(pattern.CloseAfterIdleSec) * time.Second,
					WriteIntervalSec:  time.Duration(pattern.WriteIntervalSec) * time.Second,
					MaxFileSizeByte:   pattern.MaxFileSizeByte,
				})

			}
		}
		if s.model.StreamPath != "" {
			s.streamPath = s.model.StreamPath
		}
		if s.model.AutoMoverPath != "" {
			s.automoverPath = s.model.AutoMoverPath
		}
	}

	return nil

}

// checkDataFolder ellenőrzi, hogy a hydra adatmentési könyvtár létezik-e és írható-e
func checkFolder(folderPath string) {
	// ellenőrizzük, hogy a folder létezik-e és írható-e
	if _, err := os.Stat(folderPath); os.IsNotExist(err) {
		slog.Info("Hydra folder does not exist", "folder", folderPath)
		// létrehozzuk a foldert minden subfolderrel együtt
		if err := os.MkdirAll(folderPath, 0755); err != nil {
			slog.Error("failed to create Hydra folder", "folder", folderPath, "error", err)
			panic("failed to create Hydra folder")
		}
	}
	// ellenőrizzük, hogy a folder írható-e
	if err := os.WriteFile(path.Join(folderPath, writetestFile), []byte("test"), 0644); err != nil {
		slog.Error("Hydraide folder is not writable", "folder", folderPath, "error", err)
		panic("Hydraide folder is not writable")
	}
	// töröljük a teszt fájlt
	if err := os.Remove(path.Join(folderPath, writetestFile)); err != nil {
		slog.Error("failed to remove test file", "error", err, "folder", folderPath)
		panic("failed to remove test file")
	}
}
