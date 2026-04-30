package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// defaultBaseURL is the default Ollama API endpoint.
// Using 127.0.0.1 instead of localhost to avoid DNS rebinding and IPv6 issues.
const defaultBaseURL = "http://127.0.0.1:11434"

// DefaultTimeout is the default timeout for Ollama API requests.
const DefaultTimeout = 5 * time.Minute

// HTTPClient implements the Client interface using HTTP to communicate with Ollama.
type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
	mu         sync.RWMutex
}

// Option configures an HTTPClient.
type Option func(*HTTPClient)

// WithBaseURL sets a custom base URL for the Ollama server.
func WithBaseURL(url string) Option {
	return func(c *HTTPClient) {
		c.baseURL = strings.TrimRight(url, "/")
	}
}

// WithHTTPClient sets a custom http.Client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *HTTPClient) {
		c.httpClient = hc
	}
}

// NewHTTPClient creates a new Ollama HTTP client with sensible defaults and security hardening.
func NewHTTPClient(opts ...Option) *HTTPClient {
	c := &HTTPClient{
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   5 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          2,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// BaseURL returns the current base URL (thread-safe).
func (c *HTTPClient) BaseURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseURL
}

// Ping checks if Ollama is reachable by calling /api/tags.
func (c *HTTPClient) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := c.doRequest(ctx, http.MethodGet, "/api/tags", nil)
	if err != nil {
		return fmt.Errorf("ollama ping failed: %w", err)
	}
	return nil
}

// ListModels returns all available models.
func (c *HTTPClient) ListModels(ctx context.Context) ([]Model, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	body, err := c.doRequest(ctx, http.MethodGet, "/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("list models failed: %w", err)
	}

	var resp ListModelsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse models list failed: %w", err)
	}

	return resp.Models, nil
}

// Chat sends a non-streaming chat request.
func (c *HTTPClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	req.Stream = false
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	body, err := c.doRequest(ctx, http.MethodPost, "/api/chat", req)
	if err != nil {
		// Check for known Ollama error responses
		var errResp ErrorResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
			return ChatResponse{}, fmt.Errorf("ollama error: %s", errResp.Error)
		}
		return ChatResponse{}, fmt.Errorf("chat failed: %w", err)
	}

	var resp ChatResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return ChatResponse{}, fmt.Errorf("parse chat response failed: %w", err)
	}

	return resp, nil
}

// ChatStream sends a streaming chat request and returns a channel of chunks.
func (c *HTTPClient) ChatStream(ctx context.Context, req ChatRequest) (<-chan ChatStreamChunk, error) {
	req.Stream = true

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal chat request failed: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL()+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("chat stream request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(body))
	}

	// Buffered channel + ctx.Done() select prevents goroutine leaks
	// if the TUI stops reading the stream mid-way.
	ch := make(chan ChatStreamChunk, 1)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 1024*64), 1024*1024) // 1MB max line

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			var chunk ChatStreamChunk
			if err := json.Unmarshal([]byte(line), &chunk); err != nil {
				select {
				case ch <- ChatStreamChunk{Err: fmt.Errorf("decode chunk failed: %w", err)}:
				case <-ctx.Done():
				}
				return
			}

			select {
			case ch <- chunk:
			case <-ctx.Done():
				return
			}

			if chunk.Done {
				return
			}
		}

		if err := scanner.Err(); err != nil {
			select {
			case ch <- ChatStreamChunk{Err: fmt.Errorf("stream read error: %w", err)}:
			case <-ctx.Done():
			}
		}
	}()

	return ch, nil
}

// Generate sends a non-streaming generate request.
func (c *HTTPClient) Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error) {
	req.Stream = false
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	body, err := c.doRequest(ctx, http.MethodPost, "/api/generate", req)
	if err != nil {
		return GenerateResponse{}, fmt.Errorf("generate failed: %w", err)
	}

	var resp GenerateResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return GenerateResponse{}, fmt.Errorf("parse generate response failed: %w", err)
	}

	return resp, nil
}

// ShowModel returns detailed information about a specific model.
func (c *HTTPClient) ShowModel(ctx context.Context, name string) (ModelInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	body, err := c.doRequest(ctx, http.MethodPost, "/api/show", map[string]string{"name": name})
	if err != nil {
		return ModelInfo{}, fmt.Errorf("show model failed: %w", err)
	}

	var info ModelInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return ModelInfo{}, fmt.Errorf("parse model info failed: %w", err)
	}

	return info, nil
}

// doRequest performs an HTTP request and returns the response body.
func (c *HTTPClient) doRequest(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body failed: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	url := c.BaseURL() + path
	httpReq, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return respBody, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
