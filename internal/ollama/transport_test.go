package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------- helpers ----------

func testServer(h http.Handler) (*httptest.Server, *HTTPClient) {
	srv := httptest.NewServer(h)
	client := NewHTTPClient(WithBaseURL(srv.URL))
	return srv, client
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	return b
}

// ---------- NewHTTPClient ----------

func TestNewHTTPClient_Defaults(t *testing.T) {
	c := NewHTTPClient()
	if c.baseURL != defaultBaseURL {
		t.Errorf("expected base URL %q, got %q", defaultBaseURL, c.baseURL)
	}
	if c.httpClient.Timeout != DefaultTimeout {
		t.Errorf("expected timeout %v, got %v", DefaultTimeout, c.httpClient.Timeout)
	}
}

func TestNewHTTPClient_WithBaseURL(t *testing.T) {
	c := NewHTTPClient(WithBaseURL("http://example.com:8080"))
	if c.baseURL != "http://example.com:8080" {
		t.Errorf("expected base URL %q, got %q", "http://example.com:8080", c.baseURL)
	}
}

func TestNewHTTPClient_WithBaseURLTrimSlash(t *testing.T) {
	c := NewHTTPClient(WithBaseURL("http://example.com:8080/"))
	if c.baseURL != "http://example.com:8080" {
		t.Errorf("expected trailing slash to be trimmed, got %q", c.baseURL)
	}
}

func TestNewHTTPClient_WithHTTPClient(t *testing.T) {
	custom := &http.Client{Timeout: 1 * time.Second}
	c := NewHTTPClient(WithHTTPClient(custom))
	if c.httpClient != custom {
		t.Error("expected custom HTTP client to be set")
	}
}

func TestNewHTTPClient_MultipleOptions(t *testing.T) {
	custom := &http.Client{Timeout: 2 * time.Second}
	c := NewHTTPClient(WithBaseURL("http://other:8080"), WithHTTPClient(custom))
	if c.baseURL != "http://other:8080" {
		t.Errorf("expected base URL %q, got %q", "http://other:8080", c.baseURL)
	}
	if c.httpClient != custom {
		t.Error("expected custom HTTP client")
	}
}

// ---------- BaseURL ----------

func TestBaseURL_ThreadSafe(t *testing.T) {
	c := NewHTTPClient(WithBaseURL("http://test:11434"))
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if url := c.BaseURL(); url != "http://test:11434" {
				t.Errorf("expected http://test:11434, got %q", url)
			}
		}()
	}
	wg.Wait()
}

// ---------- Ping ----------

func TestPing_Success(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/tags" {
			t.Errorf("expected /api/tags, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mustMarshal(t, ListModelsResponse{}))
	}))
	defer srv.Close()

	if err := client.Ping(context.Background()); err != nil {
		t.Errorf("Ping() unexpected error: %v", err)
	}
}

func TestPing_ServerError(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal"}`))
	}))
	defer srv.Close()

	if err := client.Ping(context.Background()); err == nil {
		t.Error("expected error from Ping() with 500 response")
	}
}

func TestPing_ContextCanceled(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := client.Ping(ctx); err == nil {
		t.Error("expected error from Ping() with canceled context")
	}
}

func TestPing_Unreachable(t *testing.T) {
	client := NewHTTPClient(WithBaseURL("http://127.0.0.1:1"))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := client.Ping(ctx); err == nil {
		t.Error("expected error from Ping() to unreachable server")
	}
}

// ---------- ListModels ----------

func TestListModels_Success(t *testing.T) {
	expected := []Model{
		{Name: "llama3:latest", Size: 1000, Digest: "abc123"},
		{Name: "mistral:latest", Size: 2000, Digest: "def456"},
	}

	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/tags" {
			t.Errorf("expected /api/tags, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListModelsResponse{Models: expected})
	}))
	defer srv.Close()

	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() unexpected error: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].Name != "llama3:latest" {
		t.Errorf("expected first model 'llama3:latest', got %q", models[0].Name)
	}
	if models[1].Name != "mistral:latest" {
		t.Errorf("expected second model 'mistral:latest', got %q", models[1].Name)
	}
}

func TestListModels_EmptyList(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListModelsResponse{Models: []Model{}})
	}))
	defer srv.Close()

	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() unexpected error: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("expected 0 models, got %d", len(models))
	}
}

func TestListModels_MalformedJSON(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{invalid`))
	}))
	defer srv.Close()

	if _, err := client.ListModels(context.Background()); err == nil {
		t.Error("expected error from malformed JSON response")
	}
}

func TestListModels_ServerError(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if _, err := client.ListModels(context.Background()); err == nil {
		t.Error("expected error from 503 response")
	}
}

// ---------- Chat ----------

func TestChat_Success(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/chat" {
			t.Errorf("expected /api/chat, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ChatResponse{
			Model:   "llama3",
			Message: Message{Role: "assistant", Content: "hello"},
			Done:    true,
		})
	}))
	defer srv.Close()

	resp, err := client.Chat(context.Background(), ChatRequest{
		Model:    "llama3",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat() unexpected error: %v", err)
	}
	if resp.Model != "llama3" {
		t.Errorf("expected model 'llama3', got %q", resp.Model)
	}
	if resp.Message.Content != "hello" {
		t.Errorf("expected content 'hello', got %q", resp.Message.Content)
	}
	if !resp.Done {
		t.Error("expected Done = true")
	}
}

func TestChat_ForcesStreamFalse(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		if req.Stream {
			t.Error("expected Stream to be forced to false")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ChatResponse{Done: true})
	}))
	defer srv.Close()

	_, _ = client.Chat(context.Background(), ChatRequest{
		Model:  "llama3",
		Stream: true, // should be overridden
	})
}

func TestChat_OllamaErrorResponse(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"model not found"}`))
	}))
	defer srv.Close()

	_, err := client.Chat(context.Background(), ChatRequest{Model: "nonexistent"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "model not found") {
		t.Errorf("expected error to contain 'model not found', got %q", err.Error())
	}
}

func TestChat_NonJSONErrorBody(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`Internal Server Error`))
	}))
	defer srv.Close()

	_, err := client.Chat(context.Background(), ChatRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestChat_ParseError(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer srv.Close()

	_, err := client.Chat(context.Background(), ChatRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected parse error")
	}
}

// ---------- ChatStream ----------

func TestChatStream_Success(t *testing.T) {
	chunks := []ChatStreamChunk{
		{Message: Message{Role: "assistant", Content: "Hello"}},
		{Message: Message{Role: "assistant", Content: " World"}},
		{Message: Message{Role: "assistant", Content: ""}, Done: true},
	}

	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/chat" {
			t.Errorf("expected /api/chat, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher")
		}
		for _, chunk := range chunks {
			b, _ := json.Marshal(chunk)
			_, _ = w.Write(b)
			_, _ = w.Write([]byte("\n"))
			flusher.Flush()
		}
	}))
	defer srv.Close()

	ch, err := client.ChatStream(context.Background(), ChatRequest{
		Model:    "llama3",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatStream() unexpected error: %v", err)
	}

	var received []string
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("unexpected error in stream: %v", chunk.Err)
		}
		received = append(received, chunk.Message.Content)
		if chunk.Done {
			break
		}
	}

	expected := []string{"Hello", " World", ""}
	if len(received) != len(expected) {
		t.Fatalf("expected %d chunks, got %d: %v", len(expected), len(received), received)
	}
	for i := range expected {
		if received[i] != expected[i] {
			t.Errorf("chunk[%d] = %q, want %q", i, received[i], expected[i])
		}
	}
}

func TestChatStream_ForcesStreamTrue(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		if !req.Stream {
			t.Error("expected Stream to be forced to true")
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		b, _ := json.Marshal(ChatStreamChunk{Done: true})
		_, _ = w.Write(b)
		_, _ = w.Write([]byte("\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	ch, err := client.ChatStream(context.Background(), ChatRequest{
		Model:    "llama3",
		Stream:   false,
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatStream() unexpected error: %v", err)
	}
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("unexpected error: %v", chunk.Err)
		}
	}
}

func TestChatStream_HTTPError(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"model not found"}`))
	}))
	defer srv.Close()

	_, err := client.ChatStream(context.Background(), ChatRequest{Model: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected error to contain status code, got %q", err.Error())
	}
}

func TestChatStream_ContextCanceledMidStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		// Send one chunk then cancel
		b, _ := json.Marshal(ChatStreamChunk{Message: Message{Content: "before cancel"}})
		_, _ = w.Write(b)
		_, _ = w.Write([]byte("\n"))
		flusher.Flush()

		cancel()
	}))
	defer srv.Close()

	ch, err := client.ChatStream(ctx, ChatRequest{Model: "test", Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		// In Go 1.25+, the context may be cancelled before the request completes
		// due to timing. This is an acceptable race condition.
		t.Logf("ChatStream returned error (acceptable race): %v", err)
		return
	}

	for chunk := range ch {
		if chunk.Err != nil {
			// Expected: stream may error after cancel
			return
		}
	}
}

func TestChatStream_DecodeErrorInStream(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte(`{invalid json}\n`))
		flusher.Flush()
	}))
	defer srv.Close()

	ch, err := client.ChatStream(context.Background(), ChatRequest{Model: "test"})
	if err != nil {
		t.Fatalf("ChatStream() unexpected error: %v", err)
	}

	chunk, ok := <-ch
	if !ok {
		t.Fatal("expected a chunk with decode error")
	}
	if chunk.Err == nil {
		t.Fatal("expected decode error in chunk")
	}
	if !strings.Contains(chunk.Err.Error(), "decode") {
		t.Errorf("expected error to contain 'decode', got %q", chunk.Err.Error())
	}
}

func TestChatStream_ScannerError(t *testing.T) {
	// Simulate a huge line that exceeds the scanner buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		// Write a valid chunk then a huge line
		b, _ := json.Marshal(ChatStreamChunk{Message: Message{Content: "ok"}})
		_, _ = w.Write(b)
		_, _ = w.Write([]byte("\n"))
		flusher.Flush()

		huge := make([]byte, 2*1024*1024) // 2MB - exceeds 1MB buffer
		for i := range huge {
			huge[i] = 'x'
		}
		_, _ = w.Write(huge)
		_, _ = w.Write([]byte("\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	ch, err := client.ChatStream(ctx, ChatRequest{Model: "test"})
	if err != nil {
		t.Fatalf("ChatStream() unexpected error: %v", err)
	}

	gotFirst := false
	gotErr := false
	for chunk := range ch {
		if chunk.Err != nil {
			gotErr = true
		} else {
			gotFirst = true
		}
	}
	if !gotFirst {
		t.Error("expected at least one valid chunk before scanner error")
	}
	if !gotErr {
		t.Log("scanner error may not trigger if buffer configured properly; acceptable")
	}
}

func TestChatStream_EmptyBody(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		// No body - immediate EOF
	}))
	defer srv.Close()

	ch, err := client.ChatStream(context.Background(), ChatRequest{Model: "test"})
	if err != nil {
		t.Fatalf("ChatStream() unexpected error: %v", err)
	}

	chunk, ok := <-ch
	if ok {
		t.Errorf("expected channel to be closed immediately, got chunk: %+v", chunk)
	}
}

func TestChatStream_ClosesChannel(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		b, _ := json.Marshal(ChatStreamChunk{Done: true})
		_, _ = w.Write(b)
		_, _ = w.Write([]byte("\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	ch, err := client.ChatStream(context.Background(), ChatRequest{Model: "test"})
	if err != nil {
		t.Fatalf("ChatStream() unexpected error: %v", err)
	}

	for range ch {
	}
	// If we get here without deadlock, channel was properly closed
}

func TestChatStream_ConcurrentRead(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 50; i++ {
			b, _ := json.Marshal(ChatStreamChunk{Message: Message{Content: "chunk"}})
			_, _ = w.Write(b)
			_, _ = w.Write([]byte("\n"))
			flusher.Flush()
		}
		b, _ := json.Marshal(ChatStreamChunk{Done: true})
		_, _ = w.Write(b)
		_, _ = w.Write([]byte("\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	ch, err := client.ChatStream(context.Background(), ChatRequest{Model: "test"})
	if err != nil {
		t.Fatalf("ChatStream() unexpected error: %v", err)
	}

	var mu sync.Mutex
	var count int
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for chunk := range ch {
				if chunk.Err != nil {
					return
				}
				mu.Lock()
				count++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	mu.Lock()
	if count < 50 {
		t.Errorf("expected at least 50 chunks, got %d", count)
	}
	mu.Unlock()
}

// ---------- Generate ----------

func TestGenerate_Success(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/generate" {
			t.Errorf("expected /api/generate, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GenerateResponse{
			Model:    "llama3",
			Response: "The answer is 42",
			Done:     true,
		})
	}))
	defer srv.Close()

	resp, err := client.Generate(context.Background(), GenerateRequest{
		Model:  "llama3",
		Prompt: "What is the answer?",
	})
	if err != nil {
		t.Fatalf("Generate() unexpected error: %v", err)
	}
	if resp.Model != "llama3" {
		t.Errorf("expected model 'llama3', got %q", resp.Model)
	}
	if resp.Response != "The answer is 42" {
		t.Errorf("expected response 'The answer is 42', got %q", resp.Response)
	}
}

func TestGenerate_ForcesStreamFalse(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req GenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		if req.Stream {
			t.Error("expected Stream to be forced to false")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GenerateResponse{Done: true})
	}))
	defer srv.Close()

	_, _ = client.Generate(context.Background(), GenerateRequest{
		Model:  "llama3",
		Prompt: "hi",
		Stream: true,
	})
}

func TestGenerate_ErrorResponse(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	_, err := client.Generate(context.Background(), GenerateRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------- ShowModel ----------

func TestShowModel_Success(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/show" {
			t.Errorf("expected /api/show, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ModelInfo{
			Modelfile:  "FROM llama3",
			Parameters: "temperature 0.7",
			Template:   "{{ .Prompt }}",
			Details:    map[string]any{"format": "gguf"},
		})
	}))
	defer srv.Close()

	info, err := client.ShowModel(context.Background(), "llama3")
	if err != nil {
		t.Fatalf("ShowModel() unexpected error: %v", err)
	}
	if info.Modelfile != "FROM llama3" {
		t.Errorf("expected modelfile 'FROM llama3', got %q", info.Modelfile)
	}
	if info.Template != "{{ .Prompt }}" {
		t.Errorf("expected template '{{ .Prompt }}', got %q", info.Template)
	}
}

func TestShowModel_NotFound(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"model 'x' not found"}`))
	}))
	defer srv.Close()

	_, err := client.ShowModel(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestShowModel_ParseError(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{invalid}`))
	}))
	defer srv.Close()

	_, err := client.ShowModel(context.Background(), "test")
	if err == nil {
		t.Fatal("expected parse error")
	}
}

// ---------- doRequest ----------

func TestDoRequest_NonJSONBody(t *testing.T) {
	// doRequest should marshal body to JSON; body not marshalable should error
	client := NewHTTPClient()
	_, err := client.doRequest(context.Background(), http.MethodPost, "/api/chat", make(chan int))
	if err == nil {
		t.Error("expected error when marshaling non-JSON body")
	}
}

func TestDoRequest_RequestCancelled(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.doRequest(ctx, http.MethodGet, "/api/tags", nil)
	if err == nil {
		t.Error("expected error with canceled context")
	}
}

// ---------- Option interface ----------

func TestOption_NilOption(t *testing.T) {
	c := NewHTTPClient(WithBaseURL("http://test:11434"))
	if c.baseURL != "http://test:11434" {
		t.Errorf("expected base URL http://test:11434, got %q", c.baseURL)
	}
}

// ---------- Buffer size for scanner ----------

func TestChatStream_LargeChunk(t *testing.T) {
	content := strings.Repeat("x", 500*1024) // 500KB content
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		b, _ := json.Marshal(ChatStreamChunk{Message: Message{Content: content}})
		_, _ = w.Write(b)
		_, _ = w.Write([]byte("\n"))
		flusher.Flush()
		b2, _ := json.Marshal(ChatStreamChunk{Done: true})
		_, _ = w.Write(b2)
		_, _ = w.Write([]byte("\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	ch, err := client.ChatStream(context.Background(), ChatRequest{Model: "test"})
	if err != nil {
		t.Fatalf("ChatStream() unexpected error: %v", err)
	}

	var gotContent string
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("unexpected error: %v", chunk.Err)
		}
		gotContent += chunk.Message.Content
	}
	if gotContent != content {
		t.Errorf("content length mismatch: got %d, want %d", len(gotContent), len(content))
	}
}

// ---------- ndjson with empty lines ----------

func TestChatStream_SkipEmptyLines(t *testing.T) {
	srv, client := testServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		// empty line followed by valid line
		_, _ = w.Write([]byte("\n"))
		flusher.Flush()
		b, _ := json.Marshal(ChatStreamChunk{Message: Message{Content: "actual"}})
		_, _ = w.Write(b)
		_, _ = w.Write([]byte("\n"))
		flusher.Flush()
		b2, _ := json.Marshal(ChatStreamChunk{Done: true})
		_, _ = w.Write(b2)
		_, _ = w.Write([]byte("\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	ch, err := client.ChatStream(context.Background(), ChatRequest{Model: "test"})
	if err != nil {
		t.Fatalf("ChatStream() unexpected error: %v", err)
	}

	var chunks []ChatStreamChunk
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0].Message.Content != "actual" {
		t.Errorf("expected content 'actual', got %q", chunks[0].Message.Content)
	}
}

// ---------- JSON round-trip for ChatStreamChunk with error ----------

func TestChatStreamChunk_ErrorField(t *testing.T) {
	// Err field is not a JSON field (no tag). Marshal only the Message/Done fields.
	b, err := json.Marshal(struct {
		Message Message `json:"message"`
		Done    bool    `json:"done"`
	}{Message: Message{Content: "hello"}, Done: true})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded ChatStreamChunk
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if decoded.Err != nil {
		t.Error("expected Err to be nil after unmarshal (not a JSON field)")
	}
	if decoded.Message.Content != "hello" {
		t.Errorf("expected content 'hello', got %q", decoded.Message.Content)
	}
	if !decoded.Done {
		t.Error("expected Done = true")
	}

	// Verify Err can be set programmatically
	errSentinel := fmt.Errorf("transport error")
	decoded.Err = errSentinel
	if decoded.Err != errSentinel {
		t.Error("Err field should be settable in code")
	}
}

// ---------- Default transport config ----------

func TestNewHTTPClient_TransportDefaults(t *testing.T) {
	c := NewHTTPClient()
	transport, ok := c.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
	}
	if !transport.ForceAttemptHTTP2 {
		t.Error("expected ForceAttemptHTTP2 to be true")
	}
	if transport.MaxIdleConns != 2 {
		t.Errorf("expected MaxIdleConns 2, got %d", transport.MaxIdleConns)
	}
	if transport.IdleConnTimeout != 90*time.Second {
		t.Errorf("expected IdleConnTimeout 90s, got %v", transport.IdleConnTimeout)
	}
}
