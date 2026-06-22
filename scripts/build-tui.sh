#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
TUI_DIR="$ROOT_DIR/wiki-tui"

echo "=== Building wiki-tui binary ==="
cd "$TUI_DIR"

# Install deps if needed
if [ ! -d "node_modules" ]; then
  echo "Installing dependencies..."
  bun install
fi

# Build native binary
mkdir -p dist
bun build --compile --target=bun --outfile=dist/wiki-tui ./src/index.tsx
echo "=== TUI binary built: $(ls -lh dist/wiki-tui | awk '{print $5}') ==="

echo "=== Building Go binary (with embedded TUI) ==="
cd "$ROOT_DIR"
go build -tags tuibuild -o wiki .
echo "=== Installing globally to ~/.local/bin/wiki ==="
cp wiki "$HOME/.local/bin/wiki"
echo ""
echo "=== Done! ==="
echo ""
echo "Run from anywhere:"
echo "  wiki                Launch OpenTUI TUI (default)"
echo "  wiki --serve        HTTP server only (headless)"
echo "  wiki --no-start     Skip Ollama auto-start"
