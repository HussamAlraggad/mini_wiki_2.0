package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

// setupManager creates a Manager with config in t.TempDir().
// It overrides os.UserHomeDir to use the temp dir for isolation.
func setupManager(t *testing.T) (*Manager, string) {
	t.Helper()
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".config", "mini-wiki")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	configPath := filepath.Join(configDir, "config.yaml")

	m := &Manager{
		path:   configPath,
		config: defaultConfig(),
		session: &Session{},
	}
	return m, homeDir
}

func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
}

// ---------- New ----------

func TestNew_CreatesDefaultConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	m, err := New()
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil manager")
	}

	cfg := m.Get()
	if cfg.Endpoint != "http://127.0.0.1:11434" {
		t.Errorf("expected default endpoint, got %q", cfg.Endpoint)
	}
	if cfg.Timeout != 300 {
		t.Errorf("expected default timeout 300, got %d", cfg.Timeout)
	}
	if cfg.DefaultModel != "" {
		t.Errorf("expected empty default model, got %q", cfg.DefaultModel)
	}

	// Verify config directory was created
	configDir := filepath.Join(homeDir, ".config", "mini-wiki")
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		t.Error("expected config directory to exist")
	}
}

func TestNew_LoadsExistingConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	configDir := filepath.Join(homeDir, ".config", "mini-wiki")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	configPath := filepath.Join(configDir, "config.yaml")
	writeConfig(t, configPath, "default_model: llama3\nendpoint: http://localhost:11434\n")

	m, err := New()
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	if m.Get().DefaultModel != "llama3" {
		t.Errorf("expected default_model 'llama3', got %q", m.Get().DefaultModel)
	}
	if m.Get().Endpoint != "http://localhost:11434" {
		t.Errorf("expected endpoint 'http://localhost:11434', got %q", m.Get().Endpoint)
	}
}

func TestNew_CorruptConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	configDir := filepath.Join(homeDir, ".config", "mini-wiki")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	configPath := filepath.Join(configDir, "config.yaml")
	writeConfig(t, configPath, "{invalid yaml: [\n")

	_, err := New()
	if err == nil {
		t.Error("expected error for corrupt config file")
	}
}

// ---------- defaultConfig ----------

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()
	if cfg.Endpoint != "http://127.0.0.1:11434" {
		t.Errorf("expected endpoint 'http://127.0.0.1:11434', got %q", cfg.Endpoint)
	}
	if cfg.Timeout != 300 {
		t.Errorf("expected timeout 300, got %d", cfg.Timeout)
	}
	if cfg.DefaultModel != "" {
		t.Errorf("expected empty DefaultModel, got %q", cfg.DefaultModel)
	}
}

// ---------- Load ----------

func TestLoad_Success(t *testing.T) {
	m, _ := setupManager(t)
	writeConfig(t, m.path, "default_model: mistral\nendpoint: http://other:11434\ntimeout_seconds: 60\n")

	if err := m.Load(); err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if m.config.DefaultModel != "mistral" {
		t.Errorf("expected DefaultModel 'mistral', got %q", m.config.DefaultModel)
	}
	if m.config.Endpoint != "http://other:11434" {
		t.Errorf("expected Endpoint 'http://other:11434', got %q", m.config.Endpoint)
	}
	if m.config.Timeout != 60 {
		t.Errorf("expected Timeout 60, got %d", m.config.Timeout)
	}
}

func TestLoad_FileNotExist(t *testing.T) {
	m, _ := setupManager(t)
	// Don't create the file
	err := m.Load()
	if err == nil {
		t.Error("expected error when file does not exist")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected IsNotExist error, got %v", err)
	}
}

func TestLoad_MalformedYAML(t *testing.T) {
	m, _ := setupManager(t)
	writeConfig(t, m.path, "default_model: [unclosed list")
	err := m.Load()
	if err == nil {
		t.Error("expected error for malformed YAML")
	}
}

// ---------- Save ----------

func TestSave_Success(t *testing.T) {
	m, _ := setupManager(t)
	m.config = Config{
		DefaultModel: "llama3",
		Endpoint:     "http://custom:11434",
		Timeout:      120,
	}

	if err := m.Save(); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	data, err := os.ReadFile(m.path)
	if err != nil {
		t.Fatalf("read saved file failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("saved file is empty")
	}
}

func TestSave_RestrictedPermissions(t *testing.T) {
	m, _ := setupManager(t)
	if err := m.Save(); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}
	info, err := os.Stat(m.path)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("expected permissions 0600, got %o", perm)
	}
}

func TestSave_RoundTrip(t *testing.T) {
	m, _ := setupManager(t)
	m.config = Config{
		DefaultModel: "llama3",
		Endpoint:     "http://custom:11434",
		Timeout:      120,
	}
	if err := m.Save(); err != nil {
		t.Fatalf("first Save() failed: %v", err)
	}

	m2, _ := setupManager(t)
	writeConfig(t, m2.path, func() string {
		data, _ := os.ReadFile(m.path)
		return string(data)
	}())

	if err := m2.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if m2.config.DefaultModel != "llama3" {
		t.Errorf("expected DefaultModel 'llama3', got %q", m2.config.DefaultModel)
	}
	if m2.config.Endpoint != "http://custom:11434" {
		t.Errorf("expected Endpoint 'http://custom:11434', got %q", m2.config.Endpoint)
	}
	if m2.config.Timeout != 120 {
		t.Errorf("expected Timeout 120, got %d", m2.config.Timeout)
	}
}

// ---------- Get ----------

func TestGet_ReturnsCopy(t *testing.T) {
	m, _ := setupManager(t)
	m.config = Config{DefaultModel: "original", Endpoint: "http://original:11434", Timeout: 100}

	cfg := m.Get()
	cfg.DefaultModel = "modified"

	if m.config.DefaultModel != "original" {
		t.Error("Get() should return a copy; modifying it should not affect original")
	}
}

// ---------- SetDefaultModel ----------

func TestSetDefaultModel(t *testing.T) {
	m, _ := setupManager(t)
	if err := m.SetDefaultModel("llama3"); err != nil {
		t.Fatalf("SetDefaultModel() unexpected error: %v", err)
	}
	if m.config.DefaultModel != "llama3" {
		t.Errorf("expected DefaultModel 'llama3', got %q", m.config.DefaultModel)
	}

	// Reload and verify persisted
	m2, _ := setupManager(t)
	writeConfig(t, m2.path, func() string {
		data, _ := os.ReadFile(m.path)
		return string(data)
	}())
	if err := m2.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if m2.config.DefaultModel != "llama3" {
		t.Errorf("persisted DefaultModel should be 'llama3', got %q", m2.config.DefaultModel)
	}
}

func TestSetDefaultModel_Empty(t *testing.T) {
	m, _ := setupManager(t)
	if err := m.SetDefaultModel(""); err != nil {
		t.Fatalf("SetDefaultModel('') unexpected error: %v", err)
	}
	if m.config.DefaultModel != "" {
		t.Errorf("expected empty DefaultModel, got %q", m.config.DefaultModel)
	}
}

// ---------- Endpoint ----------

func TestEndpoint(t *testing.T) {
	m, _ := setupManager(t)
	m.config.Endpoint = "http://custom:11434"
	if ep := m.Endpoint(); ep != "http://custom:11434" {
		t.Errorf("expected 'http://custom:11434', got %q", ep)
	}
}

// ---------- Session ----------

func TestSession_SetModel(t *testing.T) {
	s := &Session{}
	if override := s.ModelOverride(); override != "" {
		t.Errorf("expected empty override initially, got %q", override)
	}

	s.SetModel("llama3")
	if override := s.ModelOverride(); override != "llama3" {
		t.Errorf("expected override 'llama3', got %q", override)
	}
}

func TestSession_ClearModel(t *testing.T) {
	s := &Session{}
	s.SetModel("llama3")
	s.ClearModel()
	if override := s.ModelOverride(); override != "" {
		t.Errorf("expected empty after clear, got %q", override)
	}
}

func TestSession_ModelOverride_EmptyInitially(t *testing.T) {
	s := &Session{}
	if s.ModelOverride() != "" {
		t.Errorf("expected empty, got %q", s.ModelOverride())
	}
}

func TestSession_SetModel_Replace(t *testing.T) {
	s := &Session{}
	s.SetModel("first")
	s.SetModel("second")
	if s.ModelOverride() != "second" {
		t.Errorf("expected 'second', got %q", s.ModelOverride())
	}
}

// ---------- ResolvedModel ----------

func TestResolvedModel_NoOverride(t *testing.T) {
	m, _ := setupManager(t)
	m.config.DefaultModel = "default-model"

	if model := m.ResolvedModel(); model != "default-model" {
		t.Errorf("expected 'default-model', got %q", model)
	}
}

func TestResolvedModel_SessionOverrideWins(t *testing.T) {
	m, _ := setupManager(t)
	m.config.DefaultModel = "default-model"
	m.session.SetModel("override-model")

	if model := m.ResolvedModel(); model != "override-model" {
		t.Errorf("expected 'override-model', got %q", model)
	}
}

func TestResolvedModel_NoDefaultNoOverride(t *testing.T) {
	m, _ := setupManager(t)
	m.config.DefaultModel = ""

	if model := m.ResolvedModel(); model != "" {
		t.Errorf("expected empty, got %q", model)
	}
}

func TestResolvedModel_ClearedOverrideFallsBack(t *testing.T) {
	m, _ := setupManager(t)
	m.config.DefaultModel = "default-model"
	m.session.SetModel("override")
	m.session.ClearModel()

	if model := m.ResolvedModel(); model != "default-model" {
		t.Errorf("expected fallback to 'default-model', got %q", model)
	}
}

// ---------- Thread safety ----------

func TestManager_ConcurrentReadWrite(t *testing.T) {
	m, _ := setupManager(t)
	var wg sync.WaitGroup

	// Concurrent reads
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.Get()
			_ = m.Endpoint()
			_ = m.ResolvedModel()
		}()
	}

	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = m.SetDefaultModel("model")
			_ = m.Save()
			_ = m.Load()
		}(i)
	}

	wg.Wait()
}

func TestSession_ConcurrentAccess(t *testing.T) {
	s := &Session{}
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.SetModel("model-a")
			s.ClearModel()
			_ = s.ModelOverride()
			s.SetModel("model-b")
		}(i)
	}
	wg.Wait()
}

func TestManager_ConcurrentSaveAndLoad(t *testing.T) {
	m, _ := setupManager(t)
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.SetDefaultModel("model")
		}()
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.Load()
		}()
	}

	wg.Wait()
}

// ---------- End-to-end persistence ----------

func TestManager_FullRoundTrip(t *testing.T) {
	m, _ := setupManager(t)

	// Set values
	m.config = Config{
		DefaultModel: "llama3:70b",
		Endpoint:     "http://ollama.local:11434",
		Timeout:      600,
	}
	if err := m.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load into fresh manager
	m2, _ := setupManager(t)
	writeConfig(t, m2.path, func() string {
		data, _ := os.ReadFile(m.path)
		return string(data)
	}())
	if err := m2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	cfg := m2.Get()
	if cfg.DefaultModel != "llama3:70b" {
		t.Errorf("DefaultModel: got %q, want 'llama3:70b'", cfg.DefaultModel)
	}
	if cfg.Endpoint != "http://ollama.local:11434" {
		t.Errorf("Endpoint: got %q, want 'http://ollama.local:11434'", cfg.Endpoint)
	}
	if cfg.Timeout != 600 {
		t.Errorf("Timeout: got %d, want 600", cfg.Timeout)
	}
}

// ---------- Edge cases ----------

func TestManager_NilSession(t *testing.T) {
	m, _ := setupManager(t)
	m.session = nil
	// Should not panic
	model := m.ResolvedModel()
	if model != "" {
		t.Errorf("expected empty, got %q", model)
	}
}

func TestConfigYAMLTags(t *testing.T) {
	cfg := Config{
		DefaultModel: "llama3",
		Endpoint:     "http://test:11434",
		Timeout:      42,
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var restored Config
	if err := yaml.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if restored.DefaultModel != cfg.DefaultModel {
		t.Errorf("DefaultModel mismatch")
	}
	if restored.Endpoint != cfg.Endpoint {
		t.Errorf("Endpoint mismatch")
	}
	if restored.Timeout != cfg.Timeout {
		t.Errorf("Timeout mismatch")
	}
}
