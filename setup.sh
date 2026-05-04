#!/bin/bash
# setup.sh — Install all dependencies for mini-wiki
# Run this script once after cloning the repo.
# Handles systems without pip by creating a virtual environment.

set -e

echo "=== mini-wiki Setup ==="
echo ""

# --- Check Python ---
echo "[1/5] Checking Python..."
PYTHON=""
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

# Check if pip is available
if $PYTHON -m pip --version &>/dev/null; then
    PIP="$PYTHON -m pip"
else
    echo "  pip not found. Creating virtual environment..."
    # Check if venv module is available
    if $PYTHON -m venv --help &>/dev/null; then
        # Try creating venv (may need --without-pip if ensurepip is missing)
        if $PYTHON -m venv .venv &>/dev/null; then
            echo "  Virtual environment created at .venv/"
        else
            echo "  ensurepip not available. Creating venv without pip..."
            $PYTHON -m venv --without-pip .venv
            echo "  Bootstrapping pip..."
            curl -sS https://bootstrap.pypa.io/get-pip.py -o /tmp/get-pip.py
            .venv/bin/python3 /tmp/get-pip.py &>/dev/null
            echo "  pip installed in virtual environment."
        fi
    else
        echo "  venv module not available. Installing pip via get-pip.py..."
        curl -sS https://bootstrap.pypa.io/get-pip.py -o /tmp/get-pip.py
        $PYTHON /tmp/get-pip.py --user &>/dev/null
        echo "  pip installed (user-level)."
    fi
fi

# Determine which pip to use
if [ -f ".venv/bin/pip" ]; then
    PIP=".venv/bin/pip"
    echo "  Using virtual environment pip: $PIP"
elif $PYTHON -m pip --version &>/dev/null; then
    PIP="$PYTHON -m pip"
else
    echo "ERROR: Could not set up pip."
    echo "Try: sudo apt install python3-pip python3-venv"
    exit 1
fi

# Install the packages
echo "  Installing chromadb, ollama, unstructured, pypdf..."
if $PIP install chromadb ollama unstructured pypdf 2>&1 | grep -q "externally-managed"; then
    echo "  Detected externally-managed environment. Using --break-system-packages..."
    $PIP install --break-system-packages chromadb ollama unstructured pypdf
fi
echo "  Python packages installed successfully."

# Create a global symlink so the tool can find .venv from any directory
echo ""
echo "  Linking .venv to ~/.config/mini-wiki/.venv for global access..."
CONFIG_DIR="$HOME/.config/mini-wiki"
mkdir -p "$CONFIG_DIR"
if [ -d "$CONFIG_DIR/.venv" ]; then
    echo "  Global .venv already exists at $CONFIG_DIR/.venv"
elif [ -d ".venv" ]; then
    ln -sf "$(pwd)/.venv" "$CONFIG_DIR/.venv"
    echo "  Linked .venv -> $CONFIG_DIR/.venv"
fi

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
echo "  1. Build wiki:  go build -o wiki ."
echo "  2. Run:         ./wiki"
echo ""
