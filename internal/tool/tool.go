// Package tool provides a registry for mini-wiki tools.
// Tools are operations that can be listed, described, and executed
// via the HTTP API or directly from Go code.
package tool

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Kind categorizes a tool by its functional area.
type Kind string

const (
	KindRAG    Kind = "rag"
	KindData   Kind = "data"
	KindSystem Kind = "system"
	KindFile   Kind = "file"
	KindChat   Kind = "chat"
)

// Tool defines an executable operation with metadata.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Kind        Kind        `json:"kind"`
	InputSchema InputSchema `json:"input_schema"`
	Execute     func(json.RawMessage) (Result, error) `json:"-"`
}

// InputSchema describes the expected input for a tool.
type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

// Property describes a single input field.
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Default     any    `json:"default,omitempty"`
}

// Result is the output of a tool execution.
type Result struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// OkResult returns a successful result with the given data.
func OkResult(data any) Result {
	raw, _ := json.Marshal(data)
	return Result{Success: true, Data: raw}
}

// ErrResult returns a failed result with the given error.
func ErrResult(err string) Result {
	return Result{Success: false, Error: err}
}

// Registry holds all registered tools.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry. Panics if name already exists.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[t.Name]; exists {
		panic(fmt.Sprintf("tool %q already registered", t.Name))
	}
	r.tools[t.Name] = t
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// List returns all registered tools.
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

// ListByKind returns tools of a specific kind.
func (r *Registry) ListByKind(kind Kind) []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []Tool
	for _, t := range r.tools {
		if t.Kind == kind {
			result = append(result, t)
		}
	}
	return result
}

// MustRegister panics if the tool can't be registered.
// Convenience for init() functions.
func MustRegister(r *Registry, t Tool) {
	r.Register(t)
}
