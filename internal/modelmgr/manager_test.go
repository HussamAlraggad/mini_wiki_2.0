package modelmgr

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"mini-wiki/internal/ollama"
)

// ---------- mock client ----------

type mockClient struct {
	models []ollama.Model
	err    error
}

func (m *mockClient) Ping(ctx context.Context) error {
	return m.err
}

func (m *mockClient) ListModels(ctx context.Context) ([]ollama.Model, error) {
	return m.models, m.err
}

func (m *mockClient) Chat(ctx context.Context, req ollama.ChatRequest) (ollama.ChatResponse, error) {
	return ollama.ChatResponse{}, m.err
}

func (m *mockClient) ChatStream(ctx context.Context, req ollama.ChatRequest) (<-chan ollama.ChatStreamChunk, error) {
	return nil, m.err
}

func (m *mockClient) Generate(ctx context.Context, req ollama.GenerateRequest) (ollama.GenerateResponse, error) {
	return ollama.GenerateResponse{}, m.err
}

func (m *mockClient) ShowModel(ctx context.Context, name string) (ollama.ModelInfo, error) {
	return ollama.ModelInfo{}, m.err
}

func makeModels(names ...string) []ollama.Model {
	models := make([]ollama.Model, len(names))
	for i, name := range names {
		models[i] = ollama.Model{Name: name}
	}
	return models
}

// ---------- New ----------

func TestNew(t *testing.T) {
	client := &mockClient{}
	m := New(client)
	if m == nil {
		t.Fatal("expected non-nil Manager")
	}
	if m.Active() != "" {
		t.Errorf("expected empty active model, got %q", m.Active())
	}
	if m.Fallback() != "" {
		t.Errorf("expected empty fallback model, got %q", m.Fallback())
	}
	if len(m.Available()) != 0 {
		t.Errorf("expected empty available list, got %d", len(m.Available()))
	}
}

// ---------- Active / SetActive ----------

func TestActive_DefaultEmpty(t *testing.T) {
	m := New(&mockClient{})
	if m.Active() != "" {
		t.Errorf("expected empty, got %q", m.Active())
	}
}

func TestSetActive_Success(t *testing.T) {
	m := New(&mockClient{models: makeModels("llama3", "mistral")})
	_ = m.Refresh(context.Background())

	if err := m.SetActive("mistral"); err != nil {
		t.Fatalf("SetActive() unexpected error: %v", err)
	}
	if m.Active() != "mistral" {
		t.Errorf("expected 'mistral', got %q", m.Active())
	}
}

func TestSetActive_NoModelsLoaded(t *testing.T) {
	m := New(&mockClient{})
	err := m.SetActive("llama3")
	if err == nil {
		t.Fatal("expected error when no models loaded")
	}
	if err.Error() != "no models available; call Refresh first" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestSetActive_ModelNotFound(t *testing.T) {
	m := New(&mockClient{models: makeModels("llama3", "mistral")})
	_ = m.Refresh(context.Background())

	err := m.SetActive("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent model")
	}
	if err.Error() != "model \"nonexistent\" not found in available models" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestSetActive_CaseSensitive(t *testing.T) {
	m := New(&mockClient{models: makeModels("LLaMA3", "Mistral")})
	_ = m.Refresh(context.Background())

	err := m.SetActive("llama3")
	if err == nil {
		t.Fatal("expected error due to case mismatch")
	}
}

func TestActive_AfterRefreshAutoSelect(t *testing.T) {
	m := New(&mockClient{models: makeModels("llama3", "mistral")})
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() unexpected error: %v", err)
	}
	if m.Active() != "llama3" {
		t.Errorf("expected auto-selected 'llama3', got %q", m.Active())
	}
}

// ---------- Fallback / SetFallback ----------

func TestFallback_DefaultEmpty(t *testing.T) {
	m := New(&mockClient{})
	if m.Fallback() != "" {
		t.Errorf("expected empty fallback, got %q", m.Fallback())
	}
}

func TestSetFallback(t *testing.T) {
	m := New(&mockClient{})
	m.SetFallback("fallback-model")
	if m.Fallback() != "fallback-model" {
		t.Errorf("expected 'fallback-model', got %q", m.Fallback())
	}
}

func TestFallback_AutoSetOnRefresh(t *testing.T) {
	m := New(&mockClient{models: makeModels("llama3", "mistral")})
	_ = m.Refresh(context.Background())

	// fallback should be set to first model different from active
	if m.Fallback() != "mistral" {
		t.Errorf("expected fallback 'mistral', got %q", m.Fallback())
	}
}

func TestFallback_NotAutoSetWithSingleModel(t *testing.T) {
	m := New(&mockClient{models: makeModels("llama3")})
	_ = m.Refresh(context.Background())

	if m.Fallback() != "" {
		t.Errorf("expected empty fallback with single model, got %q", m.Fallback())
	}
}

func TestFallback_NotOverwrittenIfAlreadySet(t *testing.T) {
	m := New(&mockClient{models: makeModels("llama3", "mistral")})
	m.SetFallback("custom-fallback")
	_ = m.Refresh(context.Background())

	if m.Fallback() != "custom-fallback" {
		t.Errorf("expected custom 'custom-fallback' to be preserved, got %q", m.Fallback())
	}
}

// ---------- Available / AvailableNames ----------

func TestAvailable_Empty(t *testing.T) {
	m := New(&mockClient{})
	if len(m.Available()) != 0 {
		t.Errorf("expected empty, got %d", len(m.Available()))
	}
}

func TestAvailable_ReturnsCopy(t *testing.T) {
	client := &mockClient{models: makeModels("llama3")}
	m := New(client)
	_ = m.Refresh(context.Background())

	avail := m.Available()
	avail[0].Name = "hacked"

	if m.Available()[0].Name != "llama3" {
		t.Error("Available() should return a copy; modifying it should not affect internal state")
	}
}

func TestAvailable_AfterRefresh(t *testing.T) {
	client := &mockClient{models: makeModels("llama3", "mistral", "codellama")}
	m := New(client)
	_ = m.Refresh(context.Background())

	avail := m.Available()
	if len(avail) != 3 {
		t.Fatalf("expected 3 models, got %d", len(avail))
	}
	// Names should be sorted
	if avail[0].Name != "codellama" {
		t.Errorf("expected first sorted 'codellama', got %q", avail[0].Name)
	}
	if avail[1].Name != "llama3" {
		t.Errorf("expected second sorted 'llama3', got %q", avail[1].Name)
	}
	if avail[2].Name != "mistral" {
		t.Errorf("expected third sorted 'mistral', got %q", avail[2].Name)
	}
}

func TestAvailableNames(t *testing.T) {
	client := &mockClient{models: makeModels("llama3", "mistral")}
	m := New(client)
	_ = m.Refresh(context.Background())

	names := m.AvailableNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	if names[0] != "llama3" {
		t.Errorf("expected 'llama3', got %q", names[0])
	}
	if names[1] != "mistral" {
		t.Errorf("expected 'mistral', got %q", names[1])
	}
}

// ---------- Refresh ----------

func TestRefresh_Success(t *testing.T) {
	client := &mockClient{models: makeModels("llama3", "mistral", "codellama")}
	m := New(client)

	if err := m.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() unexpected error: %v", err)
	}

	if len(m.Available()) != 3 {
		t.Errorf("expected 3 available models, got %d", len(m.Available()))
	}
}

func TestRefresh_ClientError(t *testing.T) {
	client := &mockClient{err: fmt.Errorf("connection refused")}
	m := New(client)

	err := m.Refresh(context.Background())
	if err == nil {
		t.Fatal("expected error from Refresh()")
	}
	if err.Error() != "refresh models failed: connection refused" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestRefresh_EmptyModelList(t *testing.T) {
	client := &mockClient{models: []ollama.Model{}}
	m := New(client)

	if err := m.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() unexpected error: %v", err)
	}
	if len(m.Available()) != 0 {
		t.Errorf("expected 0 available models, got %d", len(m.Available()))
	}
	if m.Active() != "" {
		t.Errorf("expected empty active, got %q", m.Active())
	}
}

func TestRefresh_SortsModels(t *testing.T) {
	client := &mockClient{models: makeModels("z-model", "a-model", "m-model")}
	m := New(client)
	_ = m.Refresh(context.Background())

	names := m.AvailableNames()
	if names[0] != "a-model" {
		t.Errorf("expected 'a-model' first, got %q", names[0])
	}
	if names[1] != "m-model" {
		t.Errorf("expected 'm-model' second, got %q", names[1])
	}
	if names[2] != "z-model" {
		t.Errorf("expected 'z-model' third, got %q", names[2])
	}
}

func TestRefresh_ActivePreservedIfSet(t *testing.T) {
	client := &mockClient{models: makeModels("llama3", "mistral")}
	m := New(client)
	_ = m.Refresh(context.Background())
	_ = m.SetActive("mistral")

	// Refresh again
	client.models = makeModels("llama3", "mistral", "codellama")
	_ = m.Refresh(context.Background())

	if m.Active() != "mistral" {
		t.Errorf("expected active 'mistral' to be preserved, got %q", m.Active())
	}
}

// ---------- ActiveChain ----------

func TestActiveChain_Empty(t *testing.T) {
	m := New(&mockClient{})
	chain := m.ActiveChain()
	if len(chain) != 0 {
		t.Errorf("expected empty chain, got %v", chain)
	}
}

func TestActiveChain_ActiveOnly(t *testing.T) {
	m := New(&mockClient{models: makeModels("llama3")})
	_ = m.Refresh(context.Background())

	chain := m.ActiveChain()
	if len(chain) != 1 {
		t.Fatalf("expected 1 model in chain, got %d: %v", len(chain), chain)
	}
	if chain[0] != "llama3" {
		t.Errorf("expected 'llama3', got %q", chain[0])
	}
}

func TestActiveChain_ActiveAndFallback(t *testing.T) {
	client := &mockClient{models: makeModels("llama3", "mistral")}
	m := New(client)
	_ = m.Refresh(context.Background())

	chain := m.ActiveChain()
	if len(chain) != 2 {
		t.Fatalf("expected 2 models in chain, got %d: %v", len(chain), chain)
	}
	if chain[0] != "llama3" {
		t.Errorf("expected chain[0] 'llama3', got %q", chain[0])
	}
	if chain[1] != "mistral" {
		t.Errorf("expected chain[1] 'mistral', got %q", chain[1])
	}
}

func TestActiveChain_FullChain(t *testing.T) {
	client := &mockClient{models: makeModels("llama3", "mistral", "codellama", "phi3")}
	m := New(client)
	_ = m.Refresh(context.Background())
	m.SetFallback("phi3")

	chain := m.ActiveChain()
	// Active: llama3 (first alphabetically from sorted [codellama, llama3, mistral, phi3])
	// Wait - sorted order is: codellama, llama3, mistral, phi3
	// After refresh: active = codellama (first sorted), fallback = llama3 (first != active)
	// Then m.SetFallback("phi3") overrides
	// Expected: [codellama, phi3, llama3, mistral]
	expected := []string{"codellama", "phi3", "llama3", "mistral"}
	if len(chain) != len(expected) {
		t.Fatalf("expected %d models in chain, got %d: %v", len(expected), len(chain), chain)
	}
	for i := range expected {
		if chain[i] != expected[i] {
			t.Errorf("chain[%d] = %q, want %q", i, chain[i], expected[i])
		}
	}
}

func TestActiveChain_NoDuplicates(t *testing.T) {
	client := &mockClient{models: makeModels("llama3", "mistral")}
	m := New(client)
	_ = m.Refresh(context.Background())
	// Set fallback to same as active
	m.SetFallback("llama3")

	chain := m.ActiveChain()
	if len(chain) != 2 {
		t.Fatalf("expected 2 models (no duplicates), got %d: %v", len(chain), chain)
	}
	if chain[0] != "llama3" {
		t.Errorf("expected chain[0] 'llama3', got %q", chain[0])
	}
	if chain[1] != "mistral" {
		t.Errorf("expected chain[1] 'mistral', got %q", chain[1])
	}
}

func TestActiveChain_FallbackNotInAvailable(t *testing.T) {
	client := &mockClient{models: makeModels("llama3", "mistral")}
	m := New(client)
	_ = m.Refresh(context.Background())
	m.SetFallback("nonexistent")

	chain := m.ActiveChain()
	// 'nonexistent' is added to chain even if not in available (user-set fallback)
	if len(chain) != 3 {
		t.Fatalf("expected 3 models, got %d: %v", len(chain), chain)
	}
	if chain[0] != "llama3" {
		t.Errorf("expected chain[0] 'llama3', got %q", chain[0])
	}
	if chain[1] != "nonexistent" {
		t.Errorf("expected chain[1] 'nonexistent', got %q", chain[1])
	}
	if chain[2] != "mistral" {
		t.Errorf("expected chain[2] 'mistral', got %q", chain[2])
	}
}

func TestActiveChain_ExplicitActiveSet(t *testing.T) {
	client := &mockClient{models: makeModels("llama3", "mistral", "codellama")}
	m := New(client)
	_ = m.Refresh(context.Background())
	// After refresh: active = "codellama" (first sorted), fallback = "llama3" (first != "codellama")
	_ = m.SetActive("mistral")

	chain := m.ActiveChain()
	if len(chain) != 3 {
		t.Fatalf("expected 3 models, got %d: %v", len(chain), chain)
	}
	if chain[0] != "mistral" {
		t.Errorf("expected chain[0] 'mistral', got %q", chain[0])
	}
	// fallback was auto-set to "llama3" during refresh
	if chain[1] != "llama3" {
		t.Errorf("expected chain[1] 'llama3', got %q", chain[1])
	}
	// remaining: codellama
	if chain[2] != "codellama" {
		t.Errorf("expected chain[2] 'codellama', got %q", chain[2])
	}
}

// ---------- Thread safety ----------

func TestManager_ConcurrentReads(t *testing.T) {
	client := &mockClient{models: makeModels("llama3", "mistral")}
	m := New(client)
	_ = m.Refresh(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.Active()
			_ = m.Fallback()
			_ = m.Available()
			_ = m.AvailableNames()
			_ = m.ActiveChain()
		}()
	}
	wg.Wait()
}

func TestManager_ConcurrentReadWrite(t *testing.T) {
	client := &mockClient{models: makeModels("llama3", "mistral", "codellama")}
	m := New(client)
	_ = m.Refresh(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.SetActive("llama3")
			_ = m.Active()
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.ActiveChain()
			_ = m.Available()
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.SetFallback("fallback")
			_ = m.Fallback()
		}()
	}
	wg.Wait()
}

func TestManager_ConcurrentRefresh(t *testing.T) {
	client := &mockClient{models: makeModels("llama3", "mistral")}
	m := New(client)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.Refresh(context.Background())
		}()
	}
	wg.Wait()

	// After concurrent refreshes, state should be consistent
	_ = m.Active()
	_ = m.Fallback()
	_ = m.Available()
}

func TestManager_ConcurrentSetActiveAndRefresh(t *testing.T) {
	client := &mockClient{models: makeModels("llama3", "mistral")}
	m := New(client)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.Refresh(context.Background())
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.SetActive("llama3")
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.ActiveChain()
		}()
	}
	wg.Wait()
}

// ---------- Edge cases ----------

func TestRefresh_AfterSetActive_ModelRemoved(t *testing.T) {
	client := &mockClient{models: makeModels("llama3", "mistral")}
	m := New(client)
	_ = m.Refresh(context.Background())
	_ = m.SetActive("mistral")

	// Refresh with models that no longer include mistral
	client.models = makeModels("llama3", "codellama")
	_ = m.Refresh(context.Background())

	// Active should be preserved even if model no longer available
	if m.Active() != "mistral" {
		t.Errorf("expected active to be preserved as 'mistral', got %q", m.Active())
	}

	// Available should have the new list
	if len(m.Available()) != 2 {
		t.Fatalf("expected 2 available models, got %d", len(m.Available()))
	}
}

func TestRefresh_MultipleRefreshCalls(t *testing.T) {
	client := &mockClient{models: makeModels("llama3")}
	m := New(client)

	_ = m.Refresh(context.Background())
	if m.Active() != "llama3" {
		t.Errorf("expected 'llama3', got %q", m.Active())
	}

	// Second refresh with same data
	_ = m.Refresh(context.Background())
	if m.Active() != "llama3" {
		t.Errorf("expected 'llama3', got %q", m.Active())
	}
}

func TestAvailableNames_Sorted(t *testing.T) {
	client := &mockClient{models: makeModels("zebra", "apple", "mango")}
	m := New(client)
	_ = m.Refresh(context.Background())

	names := m.AvailableNames()
	if names[0] != "apple" {
		t.Errorf("expected 'apple' first, got %q", names[0])
	}
	if names[1] != "mango" {
		t.Errorf("expected 'mango' second, got %q", names[1])
	}
	if names[2] != "zebra" {
		t.Errorf("expected 'zebra' third, got %q", names[2])
	}
}

func TestNew_NilClient(t *testing.T) {
	m := New(nil)
	if m == nil {
		t.Fatal("New(nil) should return non-nil manager")
	}
}

func TestActiveChain_ActiveNotInAvailable(t *testing.T) {
	client := &mockClient{models: makeModels("llama3", "mistral")}
	m := New(client)
	_ = m.Refresh(context.Background())

	// Directly set active to a model not in available
	m.mu.Lock()
	m.active = "ghost-model"
	m.mu.Unlock()

	chain := m.ActiveChain()
	// 'ghost-model' should still appear in chain since it's explicitly active
	if len(chain) < 1 || chain[0] != "ghost-model" {
		t.Errorf("expected 'ghost-model' as first in chain, got %v", chain)
	}
}
