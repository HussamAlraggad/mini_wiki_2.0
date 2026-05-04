// Package rag provides a Go client for the Python RAG worker.
// It communicates with the worker via JSON over stdin/stdout pipes.
package rag

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Message types sent to the Python worker via stdin.
type Request struct {
	Cmd        string `json:"cmd"`
	Path       string `json:"path,omitempty"`
	Text       string `json:"text,omitempty"`
	TopK       int    `json:"top_k,omitempty"`
	EmbedModel string `json:"embed_model,omitempty"`
	LLMModel   string `json:"llm_model,omitempty"`
}

// Response types received from the Python worker via stdout.
type Response struct {
	Type        string            `json:"type"`
	Message     string            `json:"message,omitempty"`
	Path        string            `json:"path,omitempty"`
	Chunks      int               `json:"chunks,omitempty"`
	TotalChunks int               `json:"total_chunks,omitempty"`
	Answer      string            `json:"answer,omitempty"`
	Sources     []Source          `json:"sources,omitempty"`
	Model       string            `json:"model,omitempty"`
	EmbedModel  string            `json:"embed_model,omitempty"`
	RagDir      string            `json:"rag_dir,omitempty"`
	LLMModel    string            `json:"llm_model,omitempty"`
	Error       string            `json:"error,omitempty"`
	Traceback   string            `json:"traceback,omitempty"`
}

// Source represents a retrieved chunk with metadata.
type Source struct {
	File  string  `json:"file"`
	Score float64 `json:"score"`
	Text  string  `json:"text"`
}

// IngestResult holds the result of an ingestion operation.
type IngestResult struct {
	Path        string
	Chunks      int
	TotalChunks int
	Error       string
	Progress    []string // progress messages from the worker
}

// QueryResult holds the result of a RAG query.
type QueryResult struct {
	Answer  string
	Sources []Source
	Model   string
}

// Client manages the Python RAG worker process.
type Client struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Scanner
	mu        sync.Mutex
	startedBy bool
	lastError string
}

// LastError returns the last captured stderr output from the worker.
func (c *Client) LastError() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastError
}

// New creates a new RAG client without starting the worker.
func New() *Client {
	return &Client{}
}

// Start spawns the Python RAG worker and waits for it to be ready.
// pythonPath: path to Python interpreter
// workerPath: path to rag_worker/main.py
// wikiDir: project directory for .wiki/rag/ storage
// embedModel: embedding model to use
// llmModel: LLM model for answering
// ollamaURL: Ollama API base URL
func (c *Client) Start(pythonPath, workerPath, wikiDir, embedModel, llmModel, ollamaURL string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	cmd := exec.Command(pythonPath, workerPath)
	// Inherit PATH from parent so the Python process can find system tools.
	// The venv Python (pyvenv.cfg) handles site-packages automatically.
	parentPath := os.Getenv("PATH")
	if parentPath == "" {
		parentPath = "/usr/bin:/bin"
	}
	cmd.Env = []string{
		fmt.Sprintf("PATH=%s", parentPath),
		fmt.Sprintf("WIKI_DIR=%s", wikiDir),
		fmt.Sprintf("WIKI_EMBED_MODEL=%s", embedModel),
		fmt.Sprintf("WIKI_LLM_MODEL=%s", llmModel),
		fmt.Sprintf("WIKI_OLLAMA_URL=%s", ollamaURL),
		"PYTHONUNBUFFERED=1",
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start worker: %w", err)
	}

	c.cmd = cmd
	c.stdin = stdin
	c.stdout = bufio.NewScanner(stdout)

	// Read stderr in background and capture for error messages
	go func() {
		stderrData, _ := io.ReadAll(stderr)
		if len(stderrData) > 0 {
			c.lastError = string(stderrData)
		}
	}()

	// Wait for "ready" signal
	ready, err := c.readResponse()
	if err != nil {
		c.stop()
		return fmt.Errorf("read ready: %w", err)
	}
	if ready.Type != "ready" {
		c.stop()
		return fmt.Errorf("unexpected initial message: %s", ready.Type)
	}

	c.startedBy = true
	return nil
}

// Ingest sends a file path to the RAG worker for indexing.
// Reads progress messages until done/error.
func (c *Client) Ingest(path string, embedModel string) (*IngestResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	req := Request{Cmd: "ingest", Path: path, EmbedModel: embedModel}
	if err := c.sendRequest(req); err != nil {
		return nil, err
	}

	result := &IngestResult{Path: path}

	// Read responses until done/error, collecting progress
	timeout := time.After(10 * time.Minute)
	for {
		// Read with timeout
		type respResult struct {
			resp *Response
			err  error
		}
		resultCh := make(chan respResult, 1)
		go func() {
			resp, err := c.readResponse()
			resultCh <- respResult{resp, err}
		}()

		select {
		case r := <-resultCh:
			if r.err != nil {
				if c.lastError != "" {
					return result, fmt.Errorf("worker: %s", c.lastError)
				}
				return result, r.err
			}
			resp := r.resp

			switch resp.Type {
			case "progress":
				result.Progress = append(result.Progress, resp.Message)
				continue // read next response
			case "error":
				result.Error = resp.Message
				return result, nil
			case "done":
				result.Chunks = resp.Chunks
				result.TotalChunks = resp.TotalChunks
				return result, nil
			default:
				return result, fmt.Errorf("unexpected response: %s", resp.Type)
			}
		case <-timeout:
			return result, fmt.Errorf("ingest timed out after 30 minutes")
		}
	}
}

// IngestStream sends a file to the RAG worker for indexing with real-time progress.
// The onProgress callback is called for each progress message as it arrives.
// On timeout or error, the Python worker process is killed.
// Returns the number of chunks indexed on success.
func (c *Client) IngestStream(path string, embedModel string, onProgress func(msg string)) (int, error) {
	c.mu.Lock()

	req := Request{Cmd: "ingest", Path: path, EmbedModel: embedModel}
	if err := c.sendRequest(req); err != nil {
		c.mu.Unlock()
		return 0, err
	}

	// Release mutex during the blocking wait, so Stop() can be called from
	// another goroutine (e.g. Escape key). Re-acquire only when we're done.
	c.mu.Unlock()

	// Channel to signal process kill when we need to abort
	done := make(chan struct{}, 1)
	defer func() { close(done) }()

	type respResult struct {
		resp *Response
		err  error
	}

	// Use idle-progress timeout: if no progress message is received for
	// 5 minutes, assume the process is hung and kill it. This lets large
	// files take as long as they need as long as they're making progress.
	const idleTimeout = 5 * time.Minute
	lastProgress := time.Now()
	for {
		// Set per-iteration timeout to idleTimeout from last progress
		deadline := time.After(time.Until(lastProgress.Add(idleTimeout)))

		resultCh := make(chan respResult, 1)
		go func() {
			resp, err := c.readResponse()
			select {
			case resultCh <- respResult{resp, err}:
			case <-done:
			}
		}()

		select {
		case r := <-resultCh:
			if r.err != nil {
				c.killProcess()
				if c.lastError != "" {
					return 0, fmt.Errorf("worker: %s", c.lastError)
				}
				return 0, r.err
			}
			resp := r.resp
			switch resp.Type {
			case "progress":
				lastProgress = time.Now()
				if onProgress != nil {
					onProgress(resp.Message)
				}
				continue
			case "error":
				c.killProcess()
				return 0, fmt.Errorf("%s", resp.Message)
			case "done":
				return resp.Chunks, nil
			default:
				c.killProcess()
				return 0, fmt.Errorf("unexpected response: %s", resp.Type)
			}
		case <-deadline:
			c.killProcess()
			elapsed := time.Since(lastProgress)
			return 0, fmt.Errorf("ingest stalled: no progress for %v (worker killed). Press Escape and try again.", elapsed.Round(time.Second))
		}
	}
}

// killProcess kills the Python worker process without acquiring the mutex.
// Safe to call from any goroutine.
func (c *Client) killProcess() {
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}
	if c.stdin != nil {
		c.stdin.Close()
	}
}

// Query sends a question to the RAG worker and returns the answer with sources.
func (c *Client) Query(text string, topK int) (*QueryResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	req := Request{Cmd: "query", Text: text, TopK: topK}
	if err := c.sendRequest(req); err != nil {
		return nil, err
	}

	resp, err := c.readResponse()
	if err != nil {
		return nil, err
	}

	if resp.Type == "error" {
		return &QueryResult{Answer: resp.Message, Model: ""}, nil
	}

	if resp.Type != "answer" {
		return nil, fmt.Errorf("unexpected response: %s", resp.Type)
	}

	if resp.Sources == nil {
		resp.Sources = []Source{}
	}

	return &QueryResult{
		Answer:  resp.Answer,
		Sources: resp.Sources,
		Model:   resp.Model,
	}, nil
}

// Status retrieves the current index statistics from the worker.
func (c *Client) Status() (map[string]interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	req := Request{Cmd: "status"}
	if err := c.sendRequest(req); err != nil {
		return nil, err
	}

	resp, err := c.readResponse()
	if err != nil {
		return nil, err
	}

	if resp.Type == "error" {
		return nil, fmt.Errorf("%s", resp.Message)
	}

	return map[string]interface{}{
		"total_chunks": resp.TotalChunks,
		"sources":      resp.Sources,
		"embed_model":  resp.EmbedModel,
	}, nil
}

// Ping checks if the worker is alive.
func (c *Client) Ping() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	req := Request{Cmd: "ping"}
	if err := c.sendRequest(req); err != nil {
		return err
	}

	resp, err := c.readResponse()
	if err != nil {
		return err
	}

	if resp.Type != "pong" {
		return fmt.Errorf("unexpected ping response: %s", resp.Type)
	}
	return nil
}

// Stop sends the shutdown command and waits for the process to exit.
func (c *Client) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stop()
}

// IsRunning checks if the worker process is still active.
func (c *Client) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cmd != nil && c.cmd.Process != nil && c.cmd.ProcessState == nil
}

// sendRequest writes a JSON request to the worker's stdin.
func (c *Client) sendRequest(req Request) error {
	if c.stdin == nil {
		return fmt.Errorf("worker not started")
	}
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	if _, err := c.stdin.Write(data); err != nil {
		return fmt.Errorf("write request: %w", err)
	}
	if _, err := c.stdin.Write([]byte("\n")); err != nil {
		return fmt.Errorf("write newline: %w", err)
	}
	return nil
}

// readResponse reads the next JSON response from the worker's stdout.
func (c *Client) readResponse() (*Response, error) {
	if c.stdout == nil {
		return nil, fmt.Errorf("worker not started")
	}

	if !c.stdout.Scan() {
		if err := c.stdout.Err(); err != nil {
			return nil, fmt.Errorf("read stdout: %w", err)
		}
		return nil, fmt.Errorf("worker closed stdout (unexpected exit)")
	}

	line := strings.TrimSpace(c.stdout.Text())
	var resp Response
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w (line: %s)", err, line)
	}

	return &resp, nil
}

// stop kills the process immediately (does not wait for graceful shutdown).
func (c *Client) stop() {
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait() // Wait will complete immediately after Kill
	}
	if c.stdin != nil {
		c.stdin.Close()
	}
	c.cmd = nil
	c.stdin = nil
	c.stdout = nil
	c.startedBy = false
}
