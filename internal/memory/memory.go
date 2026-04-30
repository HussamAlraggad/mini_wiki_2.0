// Package memory provides the tool's global self-memory system.
// It stores skills registry, flaws/mistakes log, session state, and preferences
// across all projects. Data is stored as YAML files in ~/.config/mini-wiki/memory/.
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Skill represents a capability the tool has.
type Skill struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Command     string   `yaml:"command"`    // TUI command to invoke
	Category    string   `yaml:"category"`   // "srs", "data", "export", "system"
	Models      []string `yaml:"models"`     // Recommended models
	Parameters  string   `yaml:"parameters"` // Key parameters for best results
	Note        string   `yaml:"note"`       // Usage tip
}

// FlawEntry records a known issue and its solution.
type FlawEntry struct {
	ID          string    `yaml:"id"`
	Title       string    `yaml:"title"`
	Description string    `yaml:"description"`
	Symptom     string    `yaml:"symptom"`  // How user experiences it
	Cause       string    `yaml:"cause"`    // Root cause
	Solution    string    `yaml:"solution"` // How to fix or workaround
	Affects     []string  `yaml:"affects"`  // Affected commands/skills
	Workaround  string    `yaml:"workaround"` // Temporary fix
	Resolved    bool      `yaml:"resolved"`
	CreatedAt   time.Time `yaml:"created_at"`
	ResolvedAt  time.Time `yaml:"resolved_at,omitempty"`
}

// SessionState stores the current session's context.
type SessionState struct {
	LastProject  string `yaml:"last_project"`
	LastQuery    string `yaml:"last_query"`
	ActiveModel  string `yaml:"active_model"`
	ActiveTab    string `yaml:"active_tab"`
	LastRunID    string `yaml:"last_run_id"` // Last SRS pipeline run ID
}

// MemStore is the tool's global memory interface.
type MemStore interface {
	// --- Skills ---
	RegisterSkill(skill Skill) error
	GetSkills(category string) ([]Skill, error)
	GetSkill(name string) (*Skill, error)
	ListCategories() ([]string, error)

	// --- Flaws ---
	LogFlaw(entry FlawEntry) error
	GetFlaws(resolved *bool) ([]FlawEntry, error)
	GetFlaw(id string) (*FlawEntry, error)
	ResolveFlaw(id string) error

	// --- Session ---
	SaveSession(state SessionState) error
	LoadSession() (*SessionState, error)

	// --- Convenience ---
	Init() error
}

// New creates a new tool memory store.
func New() MemStore {
	return &memStore{
		baseDir: "", // set in Init()
	}
}

type memStore struct {
	baseDir string
	mu      sync.RWMutex
}

func (m *memStore) Init() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	m.baseDir = filepath.Join(home, ".config", "mini-wiki", "memory")
	if err := os.MkdirAll(m.baseDir, 0700); err != nil {
		return fmt.Errorf("create memory dir: %w", err)
	}
	return nil
}

func (m *memStore) skillsPath() string  { return filepath.Join(m.baseDir, "skills.yaml") }
func (m *memStore) flawsPath() string   { return filepath.Join(m.baseDir, "flaws.yaml") }
func (m *memStore) sessionPath() string { return filepath.Join(m.baseDir, "session.yaml") }

func (m *memStore) readYAML(path string, out interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // empty = no data
		}
		return err
	}
	return yaml.Unmarshal(data, out)
}

func (m *memStore) writeYAML(path string, in interface{}) error {
	data, err := yaml.Marshal(in)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// --- Skills ---

type skillsFile struct {
	Skills []Skill `yaml:"skills"`
}

func (m *memStore) RegisterSkill(skill Skill) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var sf skillsFile
	m.readYAML(m.skillsPath(), &sf)

	// Update if exists
	for i, s := range sf.Skills {
		if s.Name == skill.Name {
			sf.Skills[i] = skill
			return m.writeYAML(m.skillsPath(), &sf)
		}
	}

	sf.Skills = append(sf.Skills, skill)
	return m.writeYAML(m.skillsPath(), &sf)
}

func (m *memStore) GetSkills(category string) ([]Skill, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var sf skillsFile
	if err := m.readYAML(m.skillsPath(), &sf); err != nil {
		return nil, err
	}

	if category == "" {
		return sf.Skills, nil
	}

	var filtered []Skill
	for _, s := range sf.Skills {
		if s.Category == category {
			filtered = append(filtered, s)
		}
	}
	return filtered, nil
}

func (m *memStore) GetSkill(name string) (*Skill, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var sf skillsFile
	if err := m.readYAML(m.skillsPath(), &sf); err != nil {
		return nil, err
	}

	for _, s := range sf.Skills {
		if s.Name == name {
			return &s, nil
		}
	}
	return nil, nil
}

func (m *memStore) ListCategories() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var sf skillsFile
	if err := m.readYAML(m.skillsPath(), &sf); err != nil {
		return nil, err
	}

	catSet := make(map[string]bool)
	for _, s := range sf.Skills {
		catSet[s.Category] = true
	}
	var cats []string
	for c := range catSet {
		cats = append(cats, c)
	}
	return cats, nil
}

// --- Flaws ---

type flawsFile struct {
	Flaws []FlawEntry `yaml:"flaws"`
}

func (m *memStore) LogFlaw(entry FlawEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var ff flawsFile
	m.readYAML(m.flawsPath(), &ff)

	if entry.ID == "" {
		entry.ID = fmt.Sprintf("FLAW-%03d", len(ff.Flaws)+1)
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	ff.Flaws = append(ff.Flaws, entry)
	return m.writeYAML(m.flawsPath(), &ff)
}

func (m *memStore) GetFlaws(resolved *bool) ([]FlawEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var ff flawsFile
	if err := m.readYAML(m.flawsPath(), &ff); err != nil {
		return nil, err
	}

	if resolved == nil {
		return ff.Flaws, nil
	}

	var filtered []FlawEntry
	for _, f := range ff.Flaws {
		if f.Resolved == *resolved {
			filtered = append(filtered, f)
		}
	}
	return filtered, nil
}

func (m *memStore) GetFlaw(id string) (*FlawEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var ff flawsFile
	if err := m.readYAML(m.flawsPath(), &ff); err != nil {
		return nil, err
	}

	for _, f := range ff.Flaws {
		if f.ID == id {
			return &f, nil
		}
	}
	return nil, nil
}

func (m *memStore) ResolveFlaw(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var ff flawsFile
	if err := m.readYAML(m.flawsPath(), &ff); err != nil {
		return err
	}

	for i, f := range ff.Flaws {
		if f.ID == id {
			ff.Flaws[i].Resolved = true
			ff.Flaws[i].ResolvedAt = time.Now()
			return m.writeYAML(m.flawsPath(), &ff)
		}
	}
	return fmt.Errorf("flaw not found: %s", id)
}

// --- Session ---

func (m *memStore) SaveSession(state SessionState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.writeYAML(m.sessionPath(), &state)
}

func (m *memStore) LoadSession() (*SessionState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var state SessionState
	if err := m.readYAML(m.sessionPath(), &state); err != nil {
		return nil, err
	}
	return &state, nil
}
