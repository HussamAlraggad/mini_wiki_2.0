package rag

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRequestJSON(t *testing.T) {
	tests := []struct {
		name string
		req  Request
		want string // substring to check
	}{
		{
			name: "ingest request",
			req:  Request{Cmd: "ingest", Path: "/data.csv", EmbedModel: "nomic-embed-text"},
			want: `"cmd":"ingest"`,
		},
		{
			name: "query request",
			req:  Request{Cmd: "query", Text: "find relevant rows", TopK: 3},
			want: `"text":"find relevant rows"`,
		},
		{
			name: "rank request",
			req:  Request{Cmd: "rank", Path: "/data.csv", Topic: "machine learning", LLMModel: "qwen2.5-coder:7b"},
			want: `"topic":"machine learning"`,
		},
		{
			name: "query_agentic request",
			req:  Request{Cmd: "query_agentic", Path: "/data.csv", Text: "summarize", LLMModel: "qwen2.5-coder:7b"},
			want: `"cmd":"query_agentic"`,
		},
		{
			name: "ping request",
			req:  Request{Cmd: "ping"},
			want: `"cmd":"ping"`,
		},
		{
			name: "status request",
			req:  Request{Cmd: "status"},
			want: `"cmd":"status"`,
		},
		{
			name: "deep query request",
			req:  Request{Cmd: "query", Text: "deep question", TopK: 5, Deep: true, DeepModel: "gemma4:e4b"},
			want: `"deep":true`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.req)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}
			if !strings.Contains(string(data), tt.want) {
				t.Errorf("JSON %s does not contain %q", string(data), tt.want)
			}
			// Verify it can be unmarshalled back
			var restored Request
			if err := json.Unmarshal(data, &restored); err != nil {
				t.Fatalf("json.Unmarshal failed: %v", err)
			}
			if restored.Cmd != tt.req.Cmd {
				t.Errorf("Cmd = %q, want %q", restored.Cmd, tt.req.Cmd)
			}
		})
	}
}

func TestRequestOmitEmpty(t *testing.T) {
	// Verify that zero-value fields are omitted in JSON
	req := Request{Cmd: "ping"}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	// Should not contain empty fields
	jsonStr := string(data)
	if strings.Contains(jsonStr, `"path"`) {
		t.Errorf("ping request should omit path field, got: %s", jsonStr)
	}
	if strings.Contains(jsonStr, `"text"`) {
		t.Errorf("ping request should omit text field, got: %s", jsonStr)
	}
}

func TestResponseJSON(t *testing.T) {
	tests := []struct {
		name string
		resp Response
		want string
	}{
		{
			name: "ready response",
			resp: Response{Type: "ready"},
			want: `"type":"ready"`,
		},
		{
			name: "answer with sources",
			resp: Response{Type: "answer", Answer: "42 rows found", Sources: []Source{
				{File: "data.csv", Score: 0.95, Text: "relevant chunk"},
			}},
			want: `"score":0.95`,
		},
		{
			name: "error response",
			resp: Response{Type: "error", Message: "worker failed"},
			want: `"message":"worker failed"`,
		},
		{
			name: "progress response",
			resp: Response{Type: "progress", Message: "embedding chunk 42/100"},
			want: `"progress"`,
		},
		{
			name: "done response",
			resp: Response{Type: "done", Chunks: 100, TotalChunks: 100},
			want: `"chunks":100`,
		},
		{
			name: "pong response",
			resp: Response{Type: "pong"},
			want: `"pong"`,
		},
		{
			name: "query_answer response",
			resp: Response{Type: "query_answer", Answer: "data summary", Model: "gemma4"},
			want: `"query_answer"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.resp)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}
			if !strings.Contains(string(data), tt.want) {
				t.Errorf("JSON %s does not contain %q", string(data), tt.want)
			}
			// Verify round-trip
			var restored Response
			if err := json.Unmarshal(data, &restored); err != nil {
				t.Fatalf("json.Unmarshal failed: %v", err)
			}
			if restored.Type != tt.resp.Type {
				t.Errorf("Type = %q, want %q", restored.Type, tt.resp.Type)
			}
		})
	}
}

func TestResponseWithData(t *testing.T) {
	// Test response with Data field (used by agentic ranking)
	resp := Response{
		Type:      "answer",
		RowsKept:  42,
		TotalRows: 1000,
		Data: []map[string]interface{}{
			{"name": "Alice", "score": 0.95},
			{"name": "Bob", "score": 0.88},
		},
		Message: "Ranked: 1000 -> 42 (4.2%)",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var restored Response
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if restored.RowsKept != 42 {
		t.Errorf("RowsKept = %d, want 42", restored.RowsKept)
	}
	if restored.TotalRows != 1000 {
		t.Errorf("TotalRows = %d, want 1000", restored.TotalRows)
	}
	if len(restored.Data) != 2 {
		t.Errorf("Data length = %d, want 2", len(restored.Data))
	}
}

func TestSourceRoundTrip(t *testing.T) {
	src := Source{
		File:  "/path/to/data.csv",
		Score: 0.9234,
		Text:  "This is a relevant chunk of text from the dataset.",
	}

	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var restored Source
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if restored.File != src.File {
		t.Errorf("File = %q, want %q", restored.File, src.File)
	}
	if restored.Score != src.Score {
		t.Errorf("Score = %f, want %f", restored.Score, src.Score)
	}
	if restored.Text != src.Text {
		t.Errorf("Text = %q, want %q", restored.Text, src.Text)
	}
}

func TestNewClient(t *testing.T) {
	c := New()
	if c == nil {
		t.Fatal("New() returned nil")
	}
	if c.cmd != nil {
		t.Errorf("new client should have nil cmd")
	}
	if c.stdin != nil {
		t.Errorf("new client should have nil stdin")
	}
}

func TestIsRunningBeforeStart(t *testing.T) {
	c := New()
	if c.IsRunning() {
		t.Error("IsRunning() should be false before Start()")
	}
}

func TestLastErrorEmpty(t *testing.T) {
	c := New()
	if c.LastError() != "" {
		t.Errorf("LastError() = %q, want empty", c.LastError())
	}
}

func TestStopBeforeStart(t *testing.T) {
	// Stop() should not panic when called before Start()
	c := New()
	c.Stop()
}

func TestIngestResult(t *testing.T) {
	r := &IngestResult{
		Path:        "/data.csv",
		Chunks:      100,
		TotalChunks: 100,
		Error:       "",
		Progress:    []string{"starting", "processing", "done"},
	}
	if r.Chunks != 100 {
		t.Errorf("Chunks = %d, want 100", r.Chunks)
	}
	if len(r.Progress) != 3 {
		t.Errorf("Progress length = %d, want 3", len(r.Progress))
	}
}

func TestQueryResult(t *testing.T) {
	r := &QueryResult{
		Answer: "42 relevant rows found",
		Sources: []Source{
			{File: "data.csv", Score: 0.95, Text: "relevant content"},
		},
		Model: "gemma4:e4b",
	}
	if r.Answer != "42 relevant rows found" {
		t.Errorf("Answer = %q", r.Answer)
	}
	if len(r.Sources) != 1 {
		t.Errorf("Sources length = %d, want 1", len(r.Sources))
	}
	if r.Model != "gemma4:e4b" {
		t.Errorf("Model = %q", r.Model)
	}
}

func TestResponseWithTraceback(t *testing.T) {
	resp := Response{
		Type:      "error",
		Error:     "ModuleNotFoundError",
		Traceback: "Traceback (most recent call last):\n  File \"main.py\", line 42, in <module>\n    import chromadb\nModuleNotFoundError: No module named 'chromadb'",
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var restored Response
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if restored.Traceback == "" {
		t.Error("Traceback should not be empty after round-trip")
	}
	if !strings.Contains(restored.Traceback, "ModuleNotFoundError") {
		t.Errorf("Traceback should contain the error message")
	}
}

func TestResponseOmitEmpty(t *testing.T) {
	// Verify zero values are omitted
	req := Request{Cmd: "ping"}
	data, _ := json.Marshal(req)
	jsonStr := string(data)

	if strings.Contains(jsonStr, `"path"`) {
		t.Errorf("ping request should omit empty path, got: %s", jsonStr)
	}
}

func TestRequestFullRoundTrip(t *testing.T) {
	// Test a realistic request used by QueryAgentic
	req := Request{
		Cmd:      "query_agentic",
		Path:     "/home/user/data.csv",
		Text:     "What are the key findings?",
		LLMModel: "qwen2.5-coder:7b",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var restored Request
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if restored.Cmd != "query_agentic" {
		t.Errorf("Cmd = %q", restored.Cmd)
	}
	if restored.Path != "/home/user/data.csv" {
		t.Errorf("Path = %q", restored.Path)
	}
	if restored.Text != "What are the key findings?" {
		t.Errorf("Text = %q", restored.Text)
	}
}

func TestSourceSliceEmpty(t *testing.T) {
	// Source should marshal to an empty array, not null
	resp := Response{
		Type:    "answer",
		Sources: []Source{},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// Empty slice should be [] not null or omitted
	if strings.Contains(string(data), `"sources":null`) {
		t.Errorf("empty Sources should be [] not null: %s", data)
	}
}


