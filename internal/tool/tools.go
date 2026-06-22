package tool

import "encoding/json"

// RegisterDefaultTools adds all built-in tools to the registry.
func RegisterDefaultTools(r *Registry) {
	r.Register(Tool{
		Name:        "ingest",
		Description: "Ingest a file into the RAG knowledge base for semantic search",
		Kind:        KindRAG,
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"path": {Type: "string", Description: "Path to the file to ingest"},
				"deep": {Type: "boolean", Description: "Enable deep reading (AI analyzes each chunk)", Default: false},
			},
			Required: []string{"path"},
		},
	})

	r.Register(Tool{
		Name:        "query",
		Description: "Query the RAG knowledge base for relevant information",
		Kind:        KindRAG,
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"text":  {Type: "string", Description: "Your question"},
				"top_k": {Type: "number", Description: "Number of chunks to retrieve", Default: 5},
				"deep":  {Type: "boolean", Description: "Enable deep reading of sources", Default: false},
			},
			Required: []string{"text"},
		},
	})

	r.Register(Tool{
		Name:        "rank",
		Description: "Rank dataset rows by relevance to a research topic",
		Kind:        KindData,
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"topic": {Type: "string", Description: "Research topic to rank by"},
			},
			Required: []string{"topic"},
		},
	})

	r.Register(Tool{
		Name:        "chart",
		Description: "Generate a chart from the active dataset",
		Kind:        KindData,
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"chart_type": {Type: "string", Description: "Type: bar, trend, pie, scatter"},
				"columns":    {Type: "array", Description: "Columns to chart (comma-separated)"},
			},
			Required: []string{"chart_type"},
		},
	})

	r.Register(Tool{
		Name:        "export",
		Description: "Export the active dataset to a file",
		Kind:        KindData,
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"format":  {Type: "string", Description: "Export format: csv, xlsx, json", Default: "csv"},
				"columns": {Type: "array", Description: "Columns to include"},
			},
		},
	})

	r.Register(Tool{
		Name:        "chat",
		Description: "Send a message to the AI model directly (no RAG context)",
		Kind:        KindChat,
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"text": {Type: "string", Description: "Your message"},
			},
			Required: []string{"text"},
		},
	})

	r.Register(Tool{
		Name:        "model",
		Description: "Switch the active AI model",
		Kind:        KindSystem,
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"name": {Type: "string", Description: "Model name (e.g. llama3.1:8b, gemma4:e4b)"},
			},
			Required: []string{"name"},
		},
	})

	r.Register(Tool{
		Name:        "models",
		Description: "List available AI models from Ollama",
		Kind:        KindSystem,
	})

	r.Register(Tool{
		Name:        "status",
		Description: "Show server and RAG knowledge base status",
		Kind:        KindSystem,
	})

	r.Register(Tool{
		Name:        "help",
		Description: "Show available tools and usage information",
		Kind:        KindSystem,
	})

	r.Register(Tool{
		Name:        "clear",
		Description: "Clear the current conversation",
		Kind:        KindSystem,
	})

	r.Register(Tool{
		Name:        "exit",
		Description: "Exit the application",
		Kind:        KindSystem,
	})

	_ = json.RawMessage{} // ensure encoding/json is used
}
