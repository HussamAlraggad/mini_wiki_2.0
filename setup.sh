#!/bin/bash
# setup.sh — Install all dependencies for mini-wiki
# Run this script once after cloning the repo.

set -e

echo "=== mini-wiki Setup ==="
echo ""

# --- Check Python ---
echo "[1/5] Checking Python..."
if command -v python3 &>/dev/null; then
    PYTHON=python3
elif command -v python &>/dev/null; then
    PYTHON=python
else
    echo "ERROR: Python 3 is not installed."
    echo "Install it with: sudo apt install python3 python3-pip"
    exit 1
fi
echo "  Found: $($PYTHON --version)"

# --- Install Python packages ---
echo ""
echo "[2/5] Installing Python packages (chromadb, ollama, unstructured, pypdf)..."
echo "  This may take a few minutes..."
# Try with --break-system-packages first (needed on Debian/Ubuntu)
if $PYTHON -m pip install chromadb ollama unstructured pypdf 2>&1 | grep -q "externally-managed"; then
    echo "  Detected externally-managed environment. Using --break-system-packages..."
    $PYTHON -m pip install --break-system-packages chromadb ollama unstructured pypdf
else
    $PYTHON -m pip install chromadb ollama unstructured pypdf
fi
echo "  Python packages installed successfully."

# --- Check Ollama ---
echo ""
echo "[3/5] Checking Ollama..."
if ! command -v ollama &>/dev/null; then
    echo "  Ollama not found. Installing..."
    curl -fsSL https://ollama.com/install.sh | sh
fi
echo "  Found: $(ollama --version)"

# --- Start Ollama (if not running) ---
echo ""
echo "[4/5] Ensuring Ollama is running..."
if curl -s http://127.0.0.1:11434/api/tags > /dev/null 2>&1; then
    echo "  Ollama is already running."
else
    echo "  Starting Ollama in background..."
    ollama serve &
    sleep 3
    echo "  Ollama started."
fi

# --- Pull embedding models ---
echo ""
echo "[5/5] Pulling embedding models..."
echo "  Pulling nomic-embed-text..."
ollama pull nomic-embed-text 2>&1 | tail -2
echo "  Pulling all-minilm..."
ollama pull all-minilm 2>&1 | tail -2
echo ""
echo "=== Setup Complete ==="
echo ""
echo "Next steps:"
echo "  1. Pull a chat model:  ollama pull qwen2.5-coder"
echo "  2. Build wiki:          go build -o wiki ."
echo "  3. Run:                ./wiki"
echo ""
