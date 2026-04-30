// Package ollama provides the client interface and HTTP implementation
// for communicating with a local Ollama instance.
package ollama

import "time"

// Model represents an Ollama model as returned by /api/tags.
type Model struct {
	Name       string    `json:"name"`
	ModifiedAt time.Time `json:"modified_at"`
	Size       int64     `json:"size"`
	Digest     string    `json:"digest"`
	Details    Details   `json:"details,omitempty"`
}

// Details holds model metadata.
type Details struct {
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

// Message represents a message in the Ollama chat API format.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the request body for POST /api/chat.
type ChatRequest struct {
	Model    string            `json:"model"`
	Messages []Message         `json:"messages"`
	Stream   bool              `json:"stream"`
	Options  map[string]any    `json:"options,omitempty"`
	Format   string            `json:"format,omitempty"` // "json" for structured output
}

// ChatResponse is the response body for POST /api/chat (non-streaming).
type ChatResponse struct {
	Model          string  `json:"model"`
	Message        Message `json:"message"`
	Done           bool    `json:"done"`
	TotalDuration  int64   `json:"total_duration,omitempty"`
	LoadDuration   int64   `json:"load_duration,omitempty"`
	PromptEvalCount int    `json:"prompt_eval_count,omitempty"`
	EvalCount      int     `json:"eval_count,omitempty"`
}

// ChatStreamChunk represents a single chunk from a streaming chat response.
type ChatStreamChunk struct {
	Message Message `json:"message"`
	Done    bool    `json:"done"`
	Err     error   // transport-level error inside stream (not from JSON)
}

// GenerateRequest is the request body for POST /api/generate.
type GenerateRequest struct {
	Model   string `json:"model"`
	Prompt  string `json:"prompt"`
	Stream  bool   `json:"stream"`
	System  string `json:"system,omitempty"`
	Options map[string]any `json:"options,omitempty"`
}

// GenerateResponse is the response for POST /api/generate (non-streaming).
type GenerateResponse struct {
	Model     string `json:"model"`
	Response  string `json:"response"`
	Done      bool   `json:"done"`
	Context   []int  `json:"context,omitempty"`
	TotalDuration int64 `json:"total_duration,omitempty"`
}

// ErrorResponse represents an error response from the Ollama API.
type ErrorResponse struct {
	Error string `json:"error"`
}

// ModelInfo represents detailed info from POST /api/show.
type ModelInfo struct {
	Modelfile  string         `json:"modelfile"`
	Parameters string         `json:"parameters"`
	Template   string         `json:"template"`
	Details    map[string]any `json:"details"`
}

// ListModelsResponse is the response from GET /api/tags.
type ListModelsResponse struct {
	Models []Model `json:"models"`
}
