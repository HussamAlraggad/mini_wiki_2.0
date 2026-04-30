package ollama

import "context"

// Client defines the interface for communicating with an Ollama server.
// Implementations must be safe for concurrent use.
type Client interface {
	// Ping checks if the Ollama server is reachable.
	Ping(ctx context.Context) error

	// ListModels returns all available models from Ollama.
	ListModels(ctx context.Context) ([]Model, error)

	// Chat sends a non-streaming chat request and returns the full response.
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)

	// ChatStream sends a streaming chat request and returns a channel of chunks.
	// The caller must read from the channel until it is closed.
	ChatStream(ctx context.Context, req ChatRequest) (<-chan ChatStreamChunk, error)

	// Generate sends a non-streaming generate request.
	Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error)

	// ShowModel returns detailed information about a specific model.
	ShowModel(ctx context.Context, name string) (ModelInfo, error)
}
