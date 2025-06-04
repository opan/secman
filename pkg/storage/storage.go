package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/opan/secman/pkg/api"
)

// Storage defines the interface for persistent storage operations
type Storage interface {
	// Agent operations
	SaveAgent(agent api.Agent) error
	GetAgent(id string) (*api.Agent, error)
	ListAgents() ([]api.Agent, error)
	DeleteAgent(id string) error

	// Config operations
	SaveConfig(key string, data interface{}) error
	GetConfig(key string, data interface{}) error
}

// FileStorage is a file-based implementation of Storage
type FileStorage struct {
	basePath string
	mutex    sync.RWMutex
}

// NewFileStorage creates a new file storage
func NewFileStorage(basePath string) (*FileStorage, error) {
	// Create base directories if they don't exist
	if err := os.MkdirAll(filepath.Join(basePath, "agents"), 0755); err != nil {
		return nil, fmt.Errorf("failed to create agents directory: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(basePath, "config"), 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	return &FileStorage{
		basePath: basePath,
	}, nil
}

// SaveAgent saves an agent to storage
func (s *FileStorage) SaveAgent(agent api.Agent) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	filePath := filepath.Join(s.basePath, "agents", agent.ID+".json")
	return s.saveJSON(filePath, agent)
}

// GetAgent retrieves an agent from storage
func (s *FileStorage) GetAgent(id string) (*api.Agent, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	filePath := filepath.Join(s.basePath, "agents", id+".json")

	var agent api.Agent
	err := s.loadJSON(filePath, &agent)
	if err != nil {
		return nil, err
	}

	return &agent, nil
}

// ListAgents returns all agents from storage
func (s *FileStorage) ListAgents() ([]api.Agent, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	dirPath := filepath.Join(s.basePath, "agents")
	files, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read agents directory: %w", err)
	}

	var agents []api.Agent
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}

		filePath := filepath.Join(dirPath, file.Name())
		var agent api.Agent
		if err := s.loadJSON(filePath, &agent); err != nil {
			return nil, err
		}

		agents = append(agents, agent)
	}

	return agents, nil
}

// DeleteAgent removes an agent from storage
func (s *FileStorage) DeleteAgent(id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	filePath := filepath.Join(s.basePath, "agents", id+".json")
	err := os.Remove(filePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete agent: %w", err)
	}

	return nil
}

// SaveConfig saves configuration data
func (s *FileStorage) SaveConfig(key string, data interface{}) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	filePath := filepath.Join(s.basePath, "config", key+".json")
	return s.saveJSON(filePath, data)
}

// GetConfig retrieves configuration data
func (s *FileStorage) GetConfig(key string, data interface{}) error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	filePath := filepath.Join(s.basePath, "config", key+".json")
	return s.loadJSON(filePath, data)
}

// saveJSON serializes and saves data to a JSON file
func (s *FileStorage) saveJSON(filePath string, data interface{}) error {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	if err := os.WriteFile(filePath, bytes, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// loadJSON loads and deserializes data from a JSON file
func (s *FileStorage) loadJSON(filePath string, data interface{}) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	bytes, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	if err := json.Unmarshal(bytes, data); err != nil {
		return fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return nil
}
