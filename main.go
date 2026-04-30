// mini-wiki: A standalone TUI AI Research Assistant powered by local LLMs.
//
// Usage:
//   wiki                    # Run from any directory (uses CWD as research root)
//   wiki --ollama http://... # Custom Ollama endpoint
//   wiki --no-start         # Don't auto-start Ollama, fail if not running
//
// Commands (inside the TUI):
//   /help          Show commands
//   /model <name>  Switch model
//   /models        List available models
//   /refresh       Reload model list from Ollama
//   /clear         Clear conversation
//   /system <text> Set system prompt
//   /exit          Quit
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"mini-wiki/internal/app"
	"mini-wiki/internal/config"
	"mini-wiki/internal/modelmgr"
	"mini-wiki/internal/ollama"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// Command-line flags
	ollamaEndpoint := flag.String("ollama", "", "Ollama API endpoint (default: http://127.0.0.1:11434)")
	noAutoStart := flag.Bool("no-start", false, "Don't auto-start Ollama; fail if not running")
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

	// --- Initialize TUI application ---
	appModel := app.New(cfg, client, mm)

	// --- Run Bubbletea program ---
	p := tea.NewProgram(
		appModel,
		tea.WithAltScreen(),       // Use alternate screen buffer
		tea.WithMouseCellMotion(), // Enable mouse support
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		launcher.Shutdown()
		os.Exit(1)
	}

	// --- Cleanup ---
	launcher.Shutdown()
}
