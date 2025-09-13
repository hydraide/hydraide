package buildmeta

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
)

const (
	// META_FILENAME is the name of the JSON file where instance metadata is stored.
	META_FILENAME = "metadata.json"
	// CONFIG_DIR is the name of the directory within the user's home directory
	// where HydrAIDE configuration is stored.
	CONFIG_DIR = ".hydraide"
)

// InstanceMetadata holds all configuration details for a single named instance.
// This struct can be expanded in the future to store more instance-specific data.
type InstanceMetadata struct {
	BasePath string `json:"base_path"`
	Version  string `json:"version,omitempty"`
}

// MetadataStore defines the public interface for all metadata operations.
// It provides a clear contract for creating, retrieving, updating, and deleting
// instance configurations.
type MetadataStore interface {
	// GetInstance retrieves the metadata for a single, named instance.
	// It returns an error if the instance name is not found.
	GetInstance(instanceName string) (InstanceMetadata, error)

	// GetAllInstances returns a map of all known instances and their metadata.
	GetAllInstances() (map[string]InstanceMetadata, error)

	// SaveInstance creates or updates the metadata for a named instance.
	SaveInstance(instanceName string, data InstanceMetadata) error

	// DeleteInstance removes a named instance and its metadata from the store.
	DeleteInstance(instanceName string) error

	// GetConfigFilePath returns the absolute path to the metadata.json file,
	// which is useful for logging and debugging.
	GetConfigFilePath() (string, error)
}

// storeImpl is the private implementation of the MetadataStore interface.
// It holds a reference to the filesystem utility and the path to the config file.
type storeImpl struct {
	fs             filesystem.FileSystem
	configFilePath string
}

// getHomeDir correctly determines the user's home directory, even when running
// under `sudo`. It checks for the `SUDO_USER` environment variable and falls back
// to the standard home directory if not found.
func getHomeDir() (string, error) {
	sudoUser := os.Getenv("SUDO_USER")
	if sudoUser != "" {
		// If running with sudo, look up the home directory of the original user.
		u, err := user.Lookup(sudoUser)
		if err != nil {
			return "", fmt.Errorf("failed to lookup sudo user '%s': %w", sudoUser, err)
		}
		return u.HomeDir, nil
	}
	// Otherwise, return the home directory of the current user.
	return os.UserHomeDir()
}

// New creates and initializes a new MetadataStore. It ensures the configuration
// directory and metadata file exist, creating them if necessary.
func New(fs filesystem.FileSystem) (MetadataStore, error) {

	ctx := context.Background()

	home, err := getHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not determine user home directory: %w", err)
	}

	configDir := filepath.Join(home, CONFIG_DIR)
	// check if the dir exists, if not create it
	info, err := fs.Stat(ctx, configDir)
	if err != nil {
		// there was an error but we don't know what
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to check config directory at %s: %w", configDir, err)
		}
		// there was an error, but this was a "not found" error, so we can create the directory
		if err := fs.CreateDir(ctx, configDir, 0750); err != nil {
			return nil, fmt.Errorf("failed to create config directory at %s: %w", configDir, err)
		}
	}

	configFilePath := filepath.Join(configDir, META_FILENAME)
	// the config dir exists, but the size was 0, meaning the file does not exist in it
	if info.Size() == 0 {
		// Write an empty JSON object to ensure the file is valid for parsing later.
		if err := fs.WriteFile(ctx, configFilePath, []byte("{}"), 0640); err != nil {
			return nil, fmt.Errorf("failed to create or initialize metadata file: %w", err)
		}
	}

	// if the metadata file does not exist, create it
	if _, err := fs.Stat(ctx, configFilePath); os.IsNotExist(err) {
		if err := fs.WriteFile(ctx, configFilePath, []byte("{}"), 0640); err != nil {
			return nil, fmt.Errorf("failed to create metadata file: %w", err)
		}
	}

	return &storeImpl{
		fs:             fs,
		configFilePath: configFilePath,
	}, nil

}

// load reads and unmarshals the entire metadata file into a map.
// This private method is used by all public-facing read operations.
func (s *storeImpl) load() (map[string]InstanceMetadata, error) {
	ctx := context.Background()
	data, err := s.fs.ReadFile(ctx, s.configFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata file: %w", err)
	}

	// An empty file is a valid state, representing no instances.
	if len(data) == 0 {
		return make(map[string]InstanceMetadata), nil
	}

	var instances map[string]InstanceMetadata
	if err := json.Unmarshal(data, &instances); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata from %s: %w", s.configFilePath, err)
	}

	return instances, nil
}

// save marshals a map of instances to JSON and writes it to the metadata file.
// This private method is used by all public-facing write operations.
func (s *storeImpl) save(instances map[string]InstanceMetadata) error {
	ctx := context.Background()

	// Marshal with indentation to make the file human-readable for debugging.
	data, err := json.MarshalIndent(instances, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	return s.fs.WriteFile(ctx, s.configFilePath, data, 0640)
}

// GetInstance implements the MetadataStore interface.
func (s *storeImpl) GetInstance(instanceName string) (InstanceMetadata, error) {
	instances, err := s.load()
	if err != nil {
		return InstanceMetadata{}, err
	}

	data, ok := instances[instanceName]
	if !ok {
		return InstanceMetadata{}, fmt.Errorf("instance '%s' not found in metadata", instanceName)
	}

	return data, nil
}

// GetAllInstances implements the MetadataStore interface.
func (s *storeImpl) GetAllInstances() (map[string]InstanceMetadata, error) {
	return s.load()
}

// SaveInstance implements the MetadataStore interface.
func (s *storeImpl) SaveInstance(instanceName string, data InstanceMetadata) error {
	instances, err := s.load()
	if err != nil {
		// Attempt to recover by creating a new map if loading failed but the file is empty/corrupt
		if instances == nil {
			instances = make(map[string]InstanceMetadata)
		} else {
			return err
		}
	}

	instances[instanceName] = data
	return s.save(instances)
}

// DeleteInstance implements the MetadataStore interface.
func (s *storeImpl) DeleteInstance(instanceName string) error {
	instances, err := s.load()
	if err != nil {
		return err
	}

	// Deleting a non-existent key is a no-op, which is safe.
	delete(instances, instanceName)
	return s.save(instances)
}

// GetConfigFilePath implements the MetadataStore interface.
func (s *storeImpl) GetConfigFilePath() (string, error) {
	return s.configFilePath, nil
}
