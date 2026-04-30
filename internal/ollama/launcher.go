package ollama

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// Launcher manages the lifecycle of a local Ollama server process.
// It starts Ollama on demand and shuts it down when the application exits,
// but only if the Launcher started it (not if it was already running).
type Launcher struct {
	client    Client
	cmd       *exec.Cmd
	startedBy bool // true if we launched the Ollama process
}

// NewLauncher creates a new Launcher that uses the given client for health checks.
func NewLauncher(client Client) *Launcher {
	return &Launcher{client: client}
}

// EnsureRunning checks if Ollama is responsive. If not, it attempts to start
// Ollama automatically. Returns true if Ollama was started by this call.
func (l *Launcher) EnsureRunning(ctx context.Context) (bool, error) {
	// Fast path: Ollama is already responding
	if err := l.client.Ping(ctx); err == nil {
		return false, nil // already running, not started by us
	}

	// Slow path: try to start Ollama
	if err := l.startOllama(ctx); err != nil {
		return false, fmt.Errorf("failed to start Ollama: %w", err)
	}

	l.startedBy = true

	// Wait for Ollama to become responsive
	if err := l.waitForReady(ctx); err != nil {
		// Clean up if it started but isn't becoming ready
		l.shutdown()
		return false, fmt.Errorf("Ollama started but not becoming ready: %w", err)
	}

	return true, nil
}

// Shutdown kills the Ollama process if the Launcher started it.
// It is safe to call multiple times.
func (l *Launcher) Shutdown() {
	if l.startedBy && l.cmd != nil {
		l.shutdown()
	}
}

// DidStart reports whether the Launcher started the Ollama process.
func (l *Launcher) DidStart() bool {
	return l.startedBy
}

// startOllama attempts to launch the ollama serve process.
func (l *Launcher) startOllama(ctx context.Context) error {
	// Verify the ollama binary exists in PATH
	ollamaPath, err := exec.LookPath("ollama")
	if err != nil {
		return fmt.Errorf("ollama binary not found in PATH: %w", err)
	}

	cmd := exec.CommandContext(ctx, ollamaPath, "serve")

	// Set process group so we can kill the entire group later
	cmd.SysProcAttr = sysProcAttrNewProcessGroup()

	// Discard stdout/stderr from the Ollama server to not pollute the TUI
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ollama serve: %w", err)
	}

	l.cmd = cmd

	// Detach — don't wait for the process to finish; it runs in background.
	// We'll kill it explicitly in Shutdown().
	go func() {
		_ = cmd.Wait() // reap the zombie when it exits
	}()

	return nil
}

// waitForReady polls the Ollama API until it responds or the context is cancelled.
func (l *Launcher) waitForReady(ctx context.Context) error {
	pollCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	deadline := time.Now().Add(30 * time.Second)

	for time.Now().Before(deadline) {
		select {
		case <-pollCtx.Done():
			return pollCtx.Err()
		case <-ticker.C:
			if err := l.client.Ping(pollCtx); err == nil {
				return nil // Ollama is ready
			}
		}
	}

	return fmt.Errorf("Ollama did not become ready within 30 seconds")
}

// shutdown kills the Ollama process. Caller must check startedBy.
func (l *Launcher) shutdown() {
	if l.cmd != nil && l.cmd.Process != nil {
		// Kill the process group (negative PID = process group)
		killProcessGroup(l.cmd)

		// Give it a moment, then force kill
		done := make(chan struct{}, 1)
		go func() {
			_, _ = l.cmd.Process.Wait()
			done <- struct{}{}
		}()

		select {
		case <-done:
		case <-time.After(3 * time.Second):
			// Force kill if graceful shutdown didn't work
			_ = l.cmd.Process.Kill()
		}

		l.cmd = nil
	}
}
