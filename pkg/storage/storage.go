package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/opan/secman/pkg/api"
)

// Storage interface defines the storage operations
type Storage interface {
	SaveAgent(agent *api.Agent) error
	GetAgent(id string) (*api.Agent, error)
	ListAgents() ([]*api.Agent, error)
	DeleteAgent(id string) error
}

// FileStorage implements Storage using file system
type FileStorage struct {
	basePath string
	mu       sync.RWMutex
}

// NewFileStorage creates a new file-based storage
func NewFileStorage(basePath string) (*FileStorage, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	agentsDir := filepath.Join(basePath, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create agents directory: %w", err)
	}

	return &FileStorage{
		basePath: basePath,
	}, nil
}

// SaveAgent saves an agent to storage
func (fs *FileStorage) SaveAgent(agent *api.Agent) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	agentPath := filepath.Join(fs.basePath, "agents", agent.ID+".json")

	data, err := json.MarshalIndent(agent, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal agent: %w", err)
	}

	if err := os.WriteFile(agentPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write agent file: %w", err)
	}

	return nil
}

// GetAgent retrieves an agent from storage
func (fs *FileStorage) GetAgent(id string) (*api.Agent, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	agentPath := filepath.Join(fs.basePath, "agents", id+".json")

	data, err := os.ReadFile(agentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("agent not found: %s", id)
		}
		return nil, fmt.Errorf("failed to read agent file: %w", err)
	}

	var agent api.Agent
	if err := json.Unmarshal(data, &agent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal agent: %w", err)
	}

	return &agent, nil
}

// ListAgents returns all agents from storage
func (fs *FileStorage) ListAgents() ([]*api.Agent, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	agentsDir := filepath.Join(fs.basePath, "agents")

	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read agents directory: %w", err)
	}

	var agents []*api.Agent
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			agentID := entry.Name()[:len(entry.Name())-5] // Remove .json extension
			agent, err := fs.GetAgent(agentID)
			if err != nil {
				continue // Skip corrupted files
			}
			agents = append(agents, agent)
		}
	}

	return agents, nil
}

// DeleteAgent removes an agent from storage
func (fs *FileStorage) DeleteAgent(id string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	agentPath := filepath.Join(fs.basePath, "agents", id+".json")

	if err := os.Remove(agentPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("agent not found: %s", id)
		}
		return fmt.Errorf("failed to delete agent file: %w", err)
	}

	return nil
}
