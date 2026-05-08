// mini-wiki: A standalone TUI AI Research Assistant powered by local LLMs.
//
// Usage:
//   wiki                           # Full-screen TUI (text selection via Shift+click)
//   wiki --ollama http://...       # Custom Ollama endpoint
//   wiki --no-start                # Don't auto-start Ollama
//   wiki --select                  # Inline mode (free text selection with mouse)
//
// Commands (inside the TUI):
//   /help          Show all commands
//   /model <name>  Switch model
//   /models        List available models
//   /refresh       Reload model list from Ollama
//   /clear         Clear conversation
//   /system <text> Set system prompt
//   /rank <topic>  Rank dataset by relevance (Agentic AI)
//   /chart <type>  Visualize data (bar, trend, pie, scatter)
//   /ingest <path> Read a file into context
//   /clip          Copy viewport text to system clipboard
//   /exit          Quit
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"mini-wiki/internal/app"
	"mini-wiki/internal/config"
	"mini-wiki/internal/modelmgr"
	"mini-wiki/internal/ollama"

	tea "github.com/charmbracelet/bubbletea"
)

//go:embed rag_worker/*.py rag_worker/*.txt
var ragWorkerFS embed.FS

func main() {
	// Command-line flags
	ollamaEndpoint := flag.String("ollama", "", "Ollama API endpoint (default: http://127.0.0.1:11434)")
	noAutoStart := flag.Bool("no-start", false, "Don't auto-start Ollama; fail if not running")
	inlineMode := flag.Bool("select", false, "Run inline (lets you select/copy text with mouse)")
	flag.Parse()

	// --- Initialize config ---
	cfg, err := config.New()
	if err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}

	// Apply CLI overrides
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
		// User opted out of auto-start — just check it's running
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := client.Ping(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "Ollama is not running.")
			fmt.Fprintln(os.Stderr, "Start it with: ollama serve")
			fmt.Fprintln(os.Stderr, "Or run without --no-start to auto-start it.")
			os.Exit(1)
		}
	} else {
		// Auto-start Ollama if not running
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()

		started, err := launcher.EnsureRunning(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Could not connect to Ollama:", err)
			fmt.Fprintln(os.Stderr, "Make sure Ollama is installed and available in PATH.")
			fmt.Fprintln(os.Stderr, "Install: curl -fsSL https://ollama.com/install.sh | sh")
			os.Exit(1)
		}
		if started {
			fmt.Println("✓ Ollama started automatically")
		}
	}

	// --- Initialize model manager ---
	mm := modelmgr.New(client)

	// --- Extract embedded RAG worker ---
	ragDir, err := extractRAGWorker()
	if err != nil {
		log.Printf("Warning: RAG worker extraction failed: %v", err)
	}

	// --- Initialize TUI application ---
	appModel := app.New(cfg, client, mm, ragDir)

	// --- Run Bubbletea program ---
	// Default: full-screen TUI (text selection via Shift+click).
	// Use --select for inline mode (free text selection with mouse).
	opts := []tea.ProgramOption{tea.WithAltScreen(), tea.WithMouseCellMotion()}
	if *inlineMode {
		opts = []tea.ProgramOption{tea.WithMouseCellMotion()}
	}
	p := tea.NewProgram(appModel, opts...)
	appModel.SetProgram(p) // allow streaming progress from goroutines

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		launcher.Shutdown()
		os.Exit(1)
	}

	// --- Cleanup ---
	launcher.Shutdown()
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
