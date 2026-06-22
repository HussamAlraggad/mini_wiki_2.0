package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// ProjectConfig holds per-project configuration stored in .wiki/config.json.
// These values override the global config for the current project directory.
type ProjectConfig struct {
	Model     string `json:"model,omitempty"`
	Embed     string `json:"embed_model,omitempty"`
	OllamaURL string `json:"ollama_url,omitempty"`
}

// ProjectManager handles loading/saving project-level config.
type ProjectManager struct {
	mu       sync.RWMutex
	dir      string
	config   ProjectConfig
}

// NewProjectManager creates a ProjectManager for the given project directory.
func NewProjectManager(projectDir string) *ProjectManager {
	pm := &ProjectManager{
		dir:    projectDir,
		config: ProjectConfig{},
	}
	pm.load()
	return pm
}

func (pm *ProjectManager) path() string {
	return filepath.Join(pm.dir, ".wiki", "config.json")
}

func (pm *ProjectManager) load() {
	data, err := os.ReadFile(pm.path())
	if err != nil {
		return // file doesn't exist, use defaults
	}
	json.Unmarshal(data, &pm.config)
}

// Get returns the project config.
func (pm *ProjectManager) Get() ProjectConfig {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.config
}

// SetModel sets and persists the project model override.
func (pm *ProjectManager) SetModel(model string) error {
	pm.mu.Lock()
	pm.config.Model = model
	pm.mu.Unlock()
	return pm.save()
}

func (pm *ProjectManager) save() error {
	dir := filepath.Dir(pm.path())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(pm.config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(pm.path(), data, 0600)
}
