// mini-wiki: OpenCode-style TUI RAG research assistant.
// Powered by local Ollama models. Fully offline.
//
// Build:
//   bash scripts/build-tui.sh     # Full build with embedded TUI
//
// Usage:
//   wiki                          # Launch OpenTUI TUI (default)
//   wiki --serve                  # HTTP server only (headless)
//   wiki --serve-port 8080        # Custom port
//   wiki --no-start               # Don't auto-start Ollama
//   wiki --ollama http://...      # Custom Ollama endpoint
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"mini-wiki/internal/config"
	"mini-wiki/internal/modelmgr"
	"mini-wiki/internal/ollama"
	"mini-wiki/internal/rag"
	"mini-wiki/internal/server"
)

//go:embed rag_worker/*.py rag_worker/*.txt
var ragWorkerFS embed.FS

func main() {
	// Command-line flags
	ollamaEndpoint := flag.String("ollama", "", "Ollama API endpoint (default: http://127.0.0.1:11434)")
	noAutoStart := flag.Bool("no-start", false, "Don't auto-start Ollama; fail if not running")
	serveMode := flag.Bool("serve", false, "Start HTTP server only (no TUI)")
	servePort := flag.Int("serve-port", 0, "HTTP server port (0 = random)")
	flag.Parse()

	// --- Initialize config ---
	cfg, err := config.New()
	if err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}

	endpoint := cfg.Endpoint()
	if *ollamaEndpoint != "" {
		endpoint = *ollamaEndpoint
	}

	// --- Initialize Ollama client ---
	client := ollama.NewHTTPClient(
		ollama.WithBaseURL(endpoint),
	)

	// --- Ensure Ollama is running ---
	launcher := ollama.NewLauncher(client)

	if *noAutoStart {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.Ping(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "Ollama is not running. Start it with: ollama serve")
			fmt.Fprintln(os.Stderr, "Or run without --no-start to auto-start it.")
			os.Exit(1)
		}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		started, err := launcher.EnsureRunning(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Could not connect to Ollama:", err)
			os.Exit(1)
		}
		if started {
			fmt.Println("Ollama started automatically")
		}
	}

	// --- Initialize model manager ---
	mm := modelmgr.New(client)

	// --- Extract embedded RAG worker ---
	ragDir, err := extractRAGWorker()
	if err != nil {
		log.Printf("Warning: RAG worker extraction failed: %v", err)
	}

	// --- Server-only mode ---
	if *serveMode {
		runServer(client, mm, ragDir, *servePort)
		return
	}

	// --- Default: Start server + embedded TUI ---
	runWithTUI(client, mm, ragDir, *servePort)
}

// runServer starts the HTTP/SSE API server without a TUI.
func runServer(client ollama.Client, mm *modelmgr.Manager, ragWorkerDir string, port int) {
	svr := newServer(client, mm, ragWorkerDir)

	actualPort, err := svr.Start(port)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	fmt.Printf("Server started on http://127.0.0.1:%d\n", actualPort)
	fmt.Println("Press Ctrl+C to stop.")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	fmt.Println("\nShutting down...")
	svr.Stop()
}

// runWithTUI starts the HTTP server and spawns the embedded OpenTUI TUI.
func runWithTUI(client ollama.Client, mm *modelmgr.Manager, ragWorkerDir string, port int) {
	svr := newServer(client, mm, ragWorkerDir)

	actualPort, err := svr.Start(port)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// Extract and launch embedded TUI binary
	tuiPath, err := extractTUIBinary()
	if err != nil {
		log.Fatalf("TUI binary not available.\nBuild with: bash scripts/build-tui.sh\nError: %v", err)
	}

	tuiCmd := exec.Command(tuiPath)
	tuiCmd.Env = append(os.Environ(), fmt.Sprintf("WIKI_PORT=%d", actualPort))
	tuiCmd.Stdin = os.Stdin
	tuiCmd.Stdout = os.Stdout
	tuiCmd.Stderr = os.Stderr

	if err := tuiCmd.Start(); err != nil {
		log.Fatalf("Failed to start TUI: %v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- tuiCmd.Wait()
	}()

	select {
	case <-sigCh:
		tuiCmd.Process.Kill()
		<-doneCh
	case <-doneCh:
	}

	svr.Stop()
	os.RemoveAll(filepath.Dir(tuiPath))
}

// newServer creates and starts the HTTP server with all dependencies.
func newServer(client ollama.Client, mm *modelmgr.Manager, ragWorkerDir string) *server.Server {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mm.Refresh(ctx); err != nil {
		log.Printf("Warning: model refresh failed: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get working directory: %v", err)
	}

	return server.New(server.Config{
		RAG:          rag.New(),
		Ollama:       client,
		Models:       mm,
		RAGWorkerDir: ragWorkerDir,
		ProjectDir:   cwd,
	})
}

// extractTUIBinary writes the embedded TUI binary to a temp file.
func extractTUIBinary() (string, error) {
	if len(tuiBinaryData) == 0 {
		return "", fmt.Errorf("TUI binary not embedded in this build")
	}

	tmpFile, err := os.CreateTemp("", "wiki-tui-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}

	if _, err := tmpFile.Write(tuiBinaryData); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("write TUI binary: %w", err)
	}

	if err := tmpFile.Chmod(0755); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("chmod TUI binary: %w", err)
	}

	tmpFile.Close()
	return tmpFile.Name(), nil
}

// extractRAGWorker extracts the embedded Python RAG worker to a temp directory.
func extractRAGWorker() (string, error) {
	tmpDir, err := os.MkdirTemp("", "mini-wiki-rag")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	entries, err := ragWorkerFS.ReadDir("rag_worker")
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("read embedded dir: %w", err)
	}

	ragDir := filepath.Join(tmpDir, "rag_worker")
	if err := os.MkdirAll(ragDir, 0755); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("create rag_worker dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := ragWorkerFS.ReadFile(filepath.Join("rag_worker", entry.Name()))
		if err != nil {
			os.RemoveAll(tmpDir)
			return "", fmt.Errorf("read %s: %w", entry.Name(), err)
		}
		dest := filepath.Join(ragDir, entry.Name())
		if err := os.WriteFile(dest, data, 0644); err != nil {
			os.RemoveAll(tmpDir)
			return "", fmt.Errorf("write %s: %w", entry.Name(), err)
		}
	}

	return ragDir, nil
}
