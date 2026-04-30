// Package modelmgr manages the lifecycle of AI models: listing, switching,
// and fallback chaining for reliable operation.
package modelmgr

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"mini-wiki/internal/ollama"
)

// Manager handles model selection, switching, and fallback.
type Manager struct {
	mu        sync.RWMutex
	client    ollama.Client
	active    string
	fallback  string
	available []ollama.Model
}

// New creates a ModelManager. The active model is set to the first available
// model when Refresh is called, or can be set explicitly.
func New(client ollama.Client) *Manager {
	return &Manager{
		client: client,
	}
}

// Active returns the currently active model name.
func (m *Manager) Active() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// SetActive sets the active model by name. Returns an error if the model
// is not in the available list (or if no models have been loaded yet).
func (m *Manager) SetActive(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.available) == 0 {
		return fmt.Errorf("no models available; call Refresh first")
	}

	for _, model := range m.available {
		if model.Name == name {
			m.active = name
			return nil
		}
	}
	return fmt.Errorf("model %q not found in available models", name)
}

// Fallback returns the fallback model name.
func (m *Manager) Fallback() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.fallback
}

// SetFallback sets the fallback model name.
func (m *Manager) SetFallback(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fallback = name
}

// Available returns a copy of the available models list.
func (m *Manager) Available() []ollama.Model {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]ollama.Model, len(m.available))
	copy(result, m.available)
	return result
}

// AvailableNames returns just the names of available models.
func (m *Manager) AvailableNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, len(m.available))
	for i, model := range m.available {
		names[i] = model.Name
	}
	return names
}

// Refresh fetches the latest model list from Ollama and updates the available list.
// If no active model is set, it picks the first available model.
func (m *Manager) Refresh(ctx context.Context) error {
	models, err := m.client.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("refresh models failed: %w", err)
	}

	// Sort by name for consistent ordering
	sort.Slice(models, func(i, j int) bool {
		return models[i].Name < models[j].Name
	})

	m.mu.Lock()
	defer m.mu.Unlock()

	m.available = models

	// Auto-select first available model if none active
	if m.active == "" && len(models) > 0 {
		m.active = models[0].Name
	}

	// Set fallback to first model different from active if not already set
	if m.fallback == "" && len(models) > 1 {
		for _, model := range models {
			if model.Name != m.active {
				m.fallback = model.Name
				break
			}
		}
	}

	return nil
}

// ActiveChain returns the ordered list of models to try: [active, fallback, ...available].
func (m *Manager) ActiveChain() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	chain := make([]string, 0, len(m.available))
	seen := make(map[string]bool)

	// Active first
	if m.active != "" {
		chain = append(chain, m.active)
		seen[m.active] = true
	}

	// Fallback second
	if m.fallback != "" && !seen[m.fallback] {
		chain = append(chain, m.fallback)
		seen[m.fallback] = true
	}

	// Remaining available models
	for _, model := range m.available {
		if !seen[model.Name] {
			chain = append(chain, model.Name)
			seen[model.Name] = true
		}
	}

	return chain
}
