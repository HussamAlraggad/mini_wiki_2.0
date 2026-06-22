package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"mini-wiki/internal/ollama"
	"mini-wiki/internal/rag"
	"mini-wiki/internal/tool"
)

// --- GET /api/status ---

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	resp := StatusResponse{
		RAGRunning: s.rag != nil && s.rag.IsRunning(),
		Model:      s.models.Active(),
	}

	if s.rag != nil && s.rag.IsRunning() {
		status, err := s.rag.Status()
		if err == nil {
			resp.RAGStatus = status
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// --- POST /api/ingest ---

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	var req IngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request: " + err.Error()})
		return
	}
	if req.Path == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "path is required"})
		return
	}

	if errMsg := s.ensureRAG(); errMsg != "" {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: errMsg})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})

	go func() {
		s.sse.Publish(SSEEvent{Type: "progress", Data: ProgressEvent{Message: fmt.Sprintf("Ingesting %s...", req.Path)}})
		log.Printf("[ingest] starting: %s (deep=%v)", req.Path, req.Deep)

		var chunks int
		var err error
		if req.Deep {
			chunks, err = s.rag.IngestStreamDeep(req.Path, "nomic-embed-text", "gemma4:e4b", func(msg string) {
				s.sse.Publish(SSEEvent{Type: "progress", Data: ProgressEvent{Message: msg}})
			})
		} else {
			chunks, err = s.rag.IngestStream(req.Path, "nomic-embed-text", func(msg string) {
				s.sse.Publish(SSEEvent{Type: "progress", Data: ProgressEvent{Message: msg}})
			})
		}

		if err != nil {
			log.Printf("[ingest] error: %v", err)
			s.sse.Publish(SSEEvent{Type: "error", Data: ErrorEvent{Message: err.Error()}})
			return
		}
		log.Printf("[ingest] done: %d chunks", chunks)
		s.sse.Publish(SSEEvent{
			Type: "ingest_done",
			Data: IngestResultEvent{Path: req.Path, Chunks: chunks},
		})
	}()
}

// --- POST /api/query ---

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	var req QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request: " + err.Error()})
		return
	}
	if req.Text == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "text is required"})
		return
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}

	if errMsg := s.ensureRAG(); errMsg != "" {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: errMsg})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})

	go func() {
		log.Printf("[query] starting: %q", req.Text)
		s.sse.Publish(SSEEvent{Type: "progress", Data: ProgressEvent{Message: "Searching knowledge base..."}})

		qFn := s.rag.Query
		if req.Deep {
			qFn = func(text string, topK int) (*rag.QueryResult, error) {
				return s.rag.QueryDeep(text, topK, "gemma4:e4b")
			}
		}

		result, err := qFn(req.Text, req.TopK)
		if err != nil {
			log.Printf("[query] error: %v", err)
			s.sse.Publish(SSEEvent{Type: "error", Data: ErrorEvent{Message: err.Error()}})
			return
		}

		log.Printf("[query] done: %d chars, %d sources", len(result.Answer), len(result.Sources))
		s.sse.Publish(SSEEvent{
			Type: "query_result",
			Data: QueryResultEvent{
				Answer:  result.Answer,
				Sources: result.Sources,
				Model:   result.Model,
			},
		})
	}()
}

// --- POST /api/rank ---

func (s *Server) handleRank(w http.ResponseWriter, r *http.Request) {
	var req RankRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request: " + err.Error()})
		return
	}
	if req.Topic == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "topic is required"})
		return
	}

	// Rank requires a dataset path — use the project KB's dataset if available
	dsPath := s.getDatasetPath()
	if dsPath == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "no dataset ingested. Use /ingest first."})
		return
	}

	if errMsg := s.ensureRAG(); errMsg != "" {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: errMsg})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})

	go func() {
		log.Printf("[rank] starting topic=%q", req.Topic)
		s.sse.Publish(SSEEvent{Type: "progress", Data: ProgressEvent{Message: "Ranking dataset rows..."}})

		resp, err := s.rag.Rank(dsPath, req.Topic, "qwen2.5-coder:7b")
		if err != nil {
			log.Printf("[rank] error: %v", err)
			s.sse.Publish(SSEEvent{Type: "error", Data: ErrorEvent{Message: err.Error()}})
			return
		}

		log.Printf("[rank] done: %d/%d rows kept", resp.RowsKept, resp.TotalRows)
		s.sse.Publish(SSEEvent{
			Type: "rank_result",
			Data: RankResultEvent{
				RowsKept:  resp.RowsKept,
				TotalRows: resp.TotalRows,
				Data:      resp.Data,
				Message:   resp.Message,
			},
		})
	}()
}

// --- POST /api/chat (direct LLM chat without RAG) ---

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text    string `json:"text"`
		Context string `json:"context,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request: " + err.Error()})
		return
	}
	if req.Text == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "text is required"})
		return
	}

	prompt := req.Text
	if req.Context != "" {
		prompt = req.Context + "\n\n---\n\n" + req.Text
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})

	go func() {
		log.Printf("[chat] starting: %q", truncate(req.Text, 80))
		s.sse.Publish(SSEEvent{Type: "progress", Data: ProgressEvent{Message: "Thinking..."}})

		chatReq := ollama.ChatRequest{
			Model: s.models.Active(),
			Messages: []ollama.Message{
				{Role: "user", Content: prompt},
			},
		}
		ctx := context.Background()

		stream, err := s.ollama.ChatStream(ctx, chatReq)
		if err != nil {
			log.Printf("[chat] error: %v", err)
			s.sse.Publish(SSEEvent{Type: "error", Data: ErrorEvent{Message: err.Error()}})
			return
		}

		var fullText strings.Builder
		for chunk := range stream {
			if chunk.Err != nil {
				log.Printf("[chat] stream error: %v", chunk.Err)
				break
			}
			fullText.WriteString(chunk.Message.Content)
			s.sse.Publish(SSEEvent{
				Type: "token",
				Data: map[string]string{"delta": chunk.Message.Content},
			})
		}

		log.Printf("[chat] done: %d chars", fullText.Len())
		s.sse.Publish(SSEEvent{
			Type: "chat_result",
			Data: map[string]interface{}{
				"answer": fullText.String(),
				"model":  s.models.Active(),
			},
		})
	}()
}

// --- POST /api/model ---

func (s *Server) handleSetModel(w http.ResponseWriter, r *http.Request) {
	var req SetModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request: " + err.Error()})
		return
	}
	if req.Model == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "model name is required"})
		return
	}

	if err := s.models.SetActive(req.Model); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	log.Printf("[model] switched to %s", req.Model)
	writeJSON(w, http.StatusOK, map[string]string{"model": req.Model})
}

// --- GET /api/tools ---

func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	tools := s.tools.List()
	type toolView struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Kind        string            `json:"kind"`
		InputSchema tool.InputSchema  `json:"input_schema"`
	}
	views := make([]toolView, 0, len(tools))
	for _, t := range tools {
		views = append(views, toolView{
			Name:        t.Name,
			Description: t.Description,
			Kind:        string(t.Kind),
			InputSchema: t.InputSchema,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tools": views,
	})
}

// --- GET /api/tools/:name ---

func (s *Server) handleGetTool(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path[len("/api/tools/"):]
	if name == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "tool name required"})
		return
	}

	t, ok := s.tools.Get(name)
	if !ok {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: fmt.Sprintf("tool %q not found", name)})
		return
	}

	writeJSON(w, http.StatusOK, t)
}

// --- GET /api/config ---

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"model": s.models.Active(),
		"models": s.models.AvailableNames(),
		"rag_chunks": s.getRAGChunkCount(),
	})
}

func (s *Server) getRAGChunkCount() int {
	if s.rag == nil || !s.rag.IsRunning() {
		return 0
	}
	status, err := s.rag.Status()
	if err != nil {
		return 0
	}
	if chunks, ok := status["total_chunks"].(float64); ok {
		return int(chunks)
	}
	return 0
}

// --- GET /api/models ---

func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	models := s.models.AvailableNames()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"models": models,
		"active": s.models.Active(),
	})
}

// --- Session endpoints ---

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// Create new session
		var req struct {
			Title string `json:"title"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Title == "" {
			req.Title = "Session " + time.Now().Format("Jan 2 15:04")
		}
		session := s.sessions.Create(req.Title)
		writeJSON(w, http.StatusCreated, session)
		return
	}

	// List sessions
	sessions := s.sessions.List()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sessions": sessions,
	})
}

func (s *Server) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/sessions/"):]
	if id == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "session ID required"})
		return
	}

	session, ok := s.sessions.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "session not found"})
		return
	}

	if r.Method == http.MethodPost {
		var msg struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&msg); err == nil && msg.Content != "" {
			s.sessions.AppendMessage(id, msg.Role, msg.Content)
		}
	}

	writeJSON(w, http.StatusOK, session)
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// ensureRAG starts the RAG worker if not running.
func (s *Server) ensureRAG() string {
	if s.rag == nil {
		return "RAG client not configured"
	}
	if s.rag.IsRunning() {
		return ""
	}

	pythonPath := s.findPython()
	workerPath := s.ragWorkerDir + "/main.py"

	if _, err := os.Stat(workerPath); err != nil {
		return fmt.Sprintf("RAG worker not found at %s", workerPath)
	}

	err := s.rag.Start(pythonPath, workerPath, s.projectDir, "nomic-embed-text", s.models.Active(), "http://127.0.0.1:11434")
	if err != nil {
		details := s.rag.LastError()
		if details != "" {
			return fmt.Sprintf("RAG start failed: %v (stderr: %s)", err, details)
		}
		return fmt.Sprintf("RAG start failed: %v", err)
	}
	return ""
}

// findPython locates a Python interpreter.
func (s *Server) findPython() string {
	// Check project .venv first
	candidates := []string{
		s.projectDir + "/.venv/bin/python3",
		s.projectDir + "/.venv/bin/python",
	}
	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return "python3" // fallback to system
}

// getDatasetPath checks if a dataset has been ingested and returns its path.
func (s *Server) getDatasetPath() string {
	// The project KB tracks the active dataset path.
	// For now, return empty (the TUI can provide it in rank requests later).
	// In a full implementation, this reads from projectkb.
	return ""
}
