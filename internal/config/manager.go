// Package config manages application configuration persistence and session-level overrides.
// Configuration is stored at ~/.config/mini-wiki/config.yaml with restricted permissions (0600).
package config

import (
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config represents the persisted application configuration.
type Config struct {
	DefaultModel string `yaml:"default_model,omitempty"`
	Endpoint     string `yaml:"endpoint,omitempty"`
	Timeout      int    `yaml:"timeout_seconds,omitempty"` // 0 means use default
}

// Manager handles reading and writing application configuration with thread safety.
type Manager struct {
	mu      sync.RWMutex
	path    string
	config  Config
	session *Session
}

// Session holds ephemeral overrides that apply only for the current session.
type Session struct {
	mu           sync.RWMutex
	modelOverride string
}

// New creates a new ConfigManager, loading config from the default path.
// If the config file does not exist, it creates one with sensible defaults.
func New() (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configDir := filepath.Join(home, ".config", "mini-wiki")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, err
	}

	configPath := filepath.Join(configDir, "config.yaml")

	m := &Manager{
		path:    configPath,
		config:  defaultConfig(),
		session: &Session{},
	}

	if err := m.Load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return m, nil
}

func defaultConfig() Config {
	return Config{
		Endpoint: "http://127.0.0.1:11434",
		Timeout:  300,
	}
}

// Load reads configuration from disk.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.path)
	if err != nil {
		return err
	}

	return yaml.Unmarshal(data, &m.config)
}

// Save writes the current configuration to disk with restricted permissions.
func (m *Manager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, err := yaml.Marshal(&m.config)
	if err != nil {
		return err
	}

	return os.WriteFile(m.path, data, 0600)
}

// Get returns a copy of the current configuration.
func (m *Manager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// SetDefaultModel updates the default model and persists.
func (m *Manager) SetDefaultModel(name string) error {
	m.mu.Lock()
	m.config.DefaultModel = name
	m.mu.Unlock()
	return m.Save()
}

// Endpoint returns the configured Ollama endpoint.
func (m *Manager) Endpoint() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config.Endpoint
}

// Session returns the session override manager.
func (m *Manager) Session() *Session {
	return m.session
}

// ResolvedModel returns the effective model name: session override wins, then config default.
func (m *Manager) ResolvedModel() string {
	if m.session != nil {
		if override := m.session.ModelOverride(); override != "" {
			return override
		}
	}
	return m.Get().DefaultModel
}

// --- Session methods ---

func (s *Session) SetModel(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modelOverride = name
}

func (s *Session) ClearModel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modelOverride = ""
}

func (s *Session) ModelOverride() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.modelOverride
}
