package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"mini-wiki/internal/modelmgr"
	"mini-wiki/internal/ollama"
	"mini-wiki/internal/rag"
	"mini-wiki/internal/tool"
)

// Server is the HTTP/SSE API server that wraps the mini-wiki engine.
// It provides REST endpoints for the OpenTUI frontend to interact with
// the RAG pipeline, LLM, and dataset operations.
type Server struct {
	httpServer *http.Server
	sse        *SSEBroker

	// Dependencies
	rag          *rag.Client
	ollama       ollama.Client
	models       *modelmgr.Manager
	ragWorkerDir string
	projectDir   string
	port         int

	// Tool registry (for listing/autocomplete in the TUI)
	tools    *tool.Registry
	sessions *SessionStore
}

// Config holds the dependencies needed to create a Server.
type Config struct {
	RAG          *rag.Client
	Ollama       ollama.Client
	Models       *modelmgr.Manager
	RAGWorkerDir string
	ProjectDir   string
}

// New creates a new Server with the given dependencies.
func New(cfg Config) *Server {
	reg := tool.NewRegistry()
	tool.RegisterDefaultTools(reg)

	return &Server{
		sse:          newSSEBroker(),
		rag:          cfg.RAG,
		ollama:       cfg.Ollama,
		models:       cfg.Models,
		ragWorkerDir: cfg.RAGWorkerDir,
		projectDir:   cfg.ProjectDir,
		tools:        reg,
		sessions:     NewSessionStore(cfg.ProjectDir),
	}
}

// Start begins listening on the given port (0 for random) and returns
// the actual port bound. Blocks until the server is ready.
func (s *Server) Start(port int) (int, error) {
	mux := http.NewServeMux()

	// REST API endpoints
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/ingest", s.handleIngest)
	mux.HandleFunc("/api/query", s.handleQuery)
	mux.HandleFunc("/api/rank", s.handleRank)
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/model", s.handleSetModel)
	mux.HandleFunc("/api/models", s.handleListModels)

	// Configuration
	mux.HandleFunc("/api/config", s.handleGetConfig)

	// Sessions
	mux.HandleFunc("/api/sessions", s.handleListSessions)
	mux.HandleFunc("/api/sessions/", s.handleSessionByID)

	// Tool registry (for TUI autocomplete and command listing)
	mux.HandleFunc("/api/tools", s.handleListTools)
	mux.HandleFunc("/api/tools/", s.handleGetTool)

	// SSE event stream
	mux.HandleFunc("/api/events", s.SSEHandler)

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return 0, fmt.Errorf("listen: %w", err)
	}

	actualPort := listener.Addr().(*net.TCPAddr).Port
	s.port = actualPort

	s.httpServer = &http.Server{
		Handler:      withCORS(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // no write timeout for SSE
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("[server] listening on 127.0.0.1:%d", actualPort)
		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("[server] error: %v", err)
		}
	}()

	return actualPort, nil
}

// Port returns the port the server is listening on.
func (s *Server) Port() int {
	return s.port
}

// Stop gracefully shuts down the HTTP server.
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

// SSEBroker returns the SSE broker for publishing events.
func (s *Server) SSEBroker() *SSEBroker {
	return s.sse
}

// withCORS wraps a handler with permissive CORS headers for local development.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
