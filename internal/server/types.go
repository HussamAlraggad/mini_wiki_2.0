// Package server provides an HTTP/SSE API that wraps the mini-wiki engine,
// enabling the OpenTUI/SolidJS frontend (and any other client) to interact
// with the RAG pipeline, dataset operations, and model management.
package server

import "mini-wiki/internal/rag"

// --- API Request Payloads ---

type IngestRequest struct {
	Path      string `json:"path"`
	Deep      bool   `json:"deep,omitempty"`
	DeepModel string `json:"deep_model,omitempty"`
}

type QueryRequest struct {
	Text  string `json:"text"`
	TopK  int    `json:"top_k,omitempty"`
	Deep  bool   `json:"deep,omitempty"`
	Topic string `json:"topic,omitempty"` // triggers agentic query if set
}

type RankRequest struct {
	Topic string `json:"topic"`
}

type ChartRequest struct {
	ChartType string   `json:"chart_type"`
	Columns   []string `json:"columns,omitempty"`
}

type ExportRequest struct {
	Format   string   `json:"format,omitempty"` // csv, xlsx, json
	Columns  []string `json:"columns,omitempty"`
}

type SetModelRequest struct {
	Model string `json:"model"`
}

// --- API Response Payloads ---

type StatusResponse struct {
	RAGRunning bool            `json:"rag_running"`
	Model      string          `json:"model"`
	Dataset    *DatasetInfo    `json:"dataset,omitempty"`
	RAGStatus  map[string]interface{} `json:"rag_status,omitempty"`
}

type DatasetInfo struct {
	Name        string `json:"name"`
	SourceFile  string `json:"source_file"`
	RowCount    int    `json:"row_count"`
	ColumnCount int    `json:"column_count"`
	Columns     []string `json:"columns"`
	IngestedAt  string `json:"ingested_at,omitempty"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

// --- SSE Event Types ---

type SSEEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// Standard SSE event payloads
type ProgressEvent struct {
	Message string `json:"message"`
}

type QueryResultEvent struct {
	Answer  string       `json:"answer"`
	Sources []rag.Source `json:"sources"`
	Model   string       `json:"model"`
}

type IngestResultEvent struct {
	Path        string `json:"path"`
	Chunks      int    `json:"chunks"`
	TotalChunks int    `json:"total_chunks"`
}

type ErrorEvent struct {
	Message   string `json:"message"`
	Traceback string `json:"traceback,omitempty"`
}

type RankResultEvent struct {
	RowsKept  int                      `json:"rows_kept"`
	TotalRows int                      `json:"total_rows"`
	Data      []map[string]interface{} `json:"data,omitempty"`
	Message   string                   `json:"message"`
}
