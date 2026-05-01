# mini-wiki

A standalone Terminal User Interface (TUI) AI research assistant that runs entirely on local LLMs via Ollama. Designed for Software Requirements Engineering research: extract requirements, generate IEEE 830 SRS documents, analyze datasets, and more — all from your terminal, fully offline.

## Quick Start

```bash
# 1. Install Ollama (if not already)
curl -fsSL https://ollama.com/install.sh | sh

# 2. Pull a model
ollama pull qwen2.5-coder

# 3. Build wiki
git clone https://github.com/HussamAlraggad/mini_wiki_2.0.git
cd mini_wiki_2.0
go build -o wiki .

# 4. Install globally (optional)
cp wiki /home/$USER/.local/bin/wiki

# 5. Run from any research directory
cd ~/my_research_project
wiki
```

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Installation Guide](#installation-guide)
  - [1. Install Ollama](#1-install-ollama)
  - [2. Pull an LLM Model](#2-pull-an-llm-model)
  - [3. Build wiki](#3-build-wiki)
  - [4. Install System-Wide (Optional)](#4-install-system-wide-optional)
- [First Run](#first-run)
- [Complete Command Reference](#complete-command-reference)
- [Feature Overview](#feature-overview)
  - [Chat & Conversation](#chat--conversation)
  - [File System & Data](#file-system--data)
  - [Web Fetching](#web-fetching)
  - [Knowledge Base](#knowledge-base)
  - [SRS Generation Pipeline](#srs-generation-pipeline)
  - [Project Management](#project-management)
- [Layout Guide](#layout-guide)
- [Configuration](#configuration)
- [Project Structure](#project-structure)
- [Troubleshooting](#troubleshooting)
- [Building from Source](#building-from-source)

---

## Prerequisites

| Component | Required | Notes |
|---|---|---|
| Go | 1.22+ | For building from source (`go version`) |
| Ollama | 0.21+ | Local LLM runner (`ollama --version`) |
| LLM Model | Any 4B-10B | Pull at least one (`ollama pull qwen2.5-coder`) |
| GPU | 8GB VRAM | RTX 4060 / RTX 5060 recommended |
| RAM | 16GB+ | For larger models and datasets |
| OS | Linux / macOS | Windows untested but may work |

---

## Installation Guide

### 1. Install Ollama

```bash
# Linux
curl -fsSL https://ollama.com/install.sh | sh

# macOS
brew install ollama

# Verify
ollama --version
```

### 2. Pull an LLM Model

Recommended models (choose at least one):

```bash
# Best for coding and SRS tasks (recommended)
ollama pull qwen2.5-coder

# General purpose
ollama pull llama3.1:8b

# Lightweight, fast
ollama pull gemma4:e4b

# List installed models
ollama list
```

### 3. Build wiki

```bash
git clone https://github.com/HussamAlraggad/mini_wiki_2.0.git
cd mini_wiki_2.0
go build -o wiki .
```

This produces a single `wiki` binary (~16MB). No runtime dependencies.

### 4. Install System-Wide (Optional)

```bash
# Option A: Copy to user-local bin (recommended)
cp wiki ~/.local/bin/wiki

# Option B: Copy to system bin
sudo cp wiki /usr/local/bin/wiki

# Now you can run 'wiki' from any directory
```

---

## First Run

```bash
# 1. Make sure Ollama is running
ollama serve

# 2. Navigate to your research directory
mkdir -p ~/my_research
cd ~/my_research

# 3. Launch wiki
wiki
```

If Ollama is not running, wiki will auto-start it (it spawns `ollama serve` in the background).

### What you'll see

```
              ~/my_research
  Ready  |  Tokens: 0
+------------------------------------+----------+
|                                    | SESSION  |
|     _       _       _       _      | M: model |
|    |_ _  _ |_ _  _ |_ _  _ |_ _   | Tok: 0   |
|      | ||_   _||_   _||_   _||    | TASKS    |
|      _|  _|  _|  _|  _|  _|  _|   | HX       |
|                                    |          |
|         mini-wiki v2.0             |          |
|    Your local AI research assistant|          |
|                                    |          |
|   /srs  - Generate IEEE 830 SRS    |          |
|   /scan - Index project files      |          |
|   /help - All commands             |          |
+------------------------------------+----------+
|  > _  Type a message...                       |
|  active: qwen2.5-coder  |  tokens: 0  |  loaded: 3  |
```

Type your first message or a command like `/help`.

---

## Complete Command Reference

### System Commands

| Command | Description |
|---|---|
| `/help` | Show all available commands |
| `/model <name>` | Switch the active LLM model (e.g., `/model llama3.1`) |
| `/models` | List all available models from Ollama |
| `/refresh` | Reload model list from Ollama |
| `/clear` | Clear the conversation history |
| `/system <text>` | Set a custom system prompt |
| `/exit` | Quit the application |

### File Operations

| Command | Description |
|---|---|
| `/scan` | Scan the current directory for relevant files |
| `/files` | List all scanned files |
| `/ingest <path>` | Read a file and display its content |
| `/jsonl <file>` | Parse and display a JSONL dataset |

### Data & Web

| Command | Description |
|---|---|
| `/fetch <url>` | Fetch a webpage, extract text content |
| `/export` | Export conversation history to `.xlsx` |
| `/kbstatus` | Show knowledge base statistics |
| `/kbquery <query>` | Full-text search across ingested data |

### Project Management

| Command | Description |
|---|---|
| `/bookmark <title>` | Save the current finding as a bookmark |
| `/bookmarks` | List all bookmarks |
| `/history` | Show recent query history |
| `/task <description>` | Add a task to the todo list |
| `/tasks` | List all tasks |

### SRS Generation

| Command | Description |
|---|---|
| `/srs` | Run the 5-stage SRS generation pipeline |

### Skills & Memory

| Command | Description |
|---|---|
| `/skills` | List all tool capabilities |
| `/flaws` | Show known issues and their solutions |

### File References

| Syntax | Description |
|---|---|
| `@filename` | Reference a file in chat (auto-attaches content) |
| `@path/to/file` | Reference a file by relative path |
| `@file.go:42` | Reference a specific line number |
| `@file.go:10-20` | Reference a line range |

---

## Feature Overview

### Chat & Conversation

- **Streaming responses** from local LLM via Ollama
- **Model switching** mid-session without losing context
- **Fallback chain** — if the primary model fails, it tries a backup
- **Welcome logo** shown before the first message, then replaced by chat
- **Command auto-completion** with Tab key (shows descriptions)
- **File reference** via `@filename` — content auto-injected into context

### File System & Data

- **Safe directory scanner**: skips dotfiles, hidden dirs (`.git`, `node_modules`), binary files, symlinks outside CWD
- **File type detection**: 22 types (code, markup, data, config)
- **CSV parsing**: streaming parser with chunking, column type detection, error tolerance
- **JSONL parsing**: streaming line-by-line JSON parser, extracts nested fields, flattens objects
- **Binary detection**: null-byte check + 22 known binary magic prefixes (ELF, PDF, ZIP, PNG, etc.)

### Web Fetching

- **SSRF-safe**: blocks 11 private CIDR ranges at DNS level
- **URL validation**: rejects `file://`, `ftp://`, localhost, credentials in URL
- **HTML-to-text**: safe parsing via `golang.org/x/net/html` (no regex)
- **Limits**: 5MB max body, 30s timeout, 5 redirect limit

### Knowledge Base

- **SQLite-backed** with FTS5 full-text search
- **Per-project isolation**: each directory gets `./.wiki/kb.sqlite`
- **Tables**: file registry, row storage, SRS pipeline results, query history, bookmarks, filter states

### SRS Generation Pipeline

Runs 5 stages sequentially via local LLM:

| Stage | What It Does |
|---|---|
| **1. FR/NFR Extraction** | Extracts functional and non-functional requirements from data |
| **2. MoSCoW Prioritization** | Categorizes requirements: MUST / SHOULD / COULD / WON'T |
| **3. DFD Generation** | Identifies external entities, processes, data stores, data flows |
| **4. CSPEC Logic** | Creates activation tables and decision tables |
| **5. SRS Formatting** | Generates a complete IEEE 830 SRS document |

All results saved to the project KB for cross-session reference.

### Project Management

- **Tool Memory**: global YAML at `~/.config/mini-wiki/memory/`
  - `skills.yaml` — registry of tool capabilities
  - `flaws.yaml` — log of known issues and solutions
  - `session.yaml` — last project, last query, active model
- **Bookmarks**: save important findings per project
- **Tasks**: todo list visible in the right panel
- **Action history**: last 20 actions tracked

---

## Layout Guide

The TUI has two panels plus a bottom bar:

```
         ~/research/my_project              <- centered project path
  Ready  |  Tokens: 123                     <- status + token count
+-----------------------------------+-------+
| CHAT                              | SESS  |  <- chat (80%) + info (20%)
|                                   | M: .. |
|  (your conversation with the LLM) | Tok.. |
|                                   | TASKS |
|                                   | HX    |
+-----------------------------------+-------+
|  /model - Switch  |  /models - List     |  <- suggestion overlay (Tab)
+------------------------------------------+
|  > _ Type a message...                   |  <- input box
+------------------------------------------+
|  active: model | tokens: 0 | loaded: 3   |  <- model info bar
+------------------------------------------+
```

- **Tab** while typing a command → cycles through completions
- **Mouse** scrolls the chat viewport
- **Click** on the right panel content focuses it

---

## Configuration

Config file: `~/.config/mini-wiki/config.yaml`

```yaml
default_model: "qwen2.5-coder"     # default LLM model
endpoint: "http://127.0.0.1:11434"  # Ollama API endpoint
timeout_seconds: 300                # API timeout
```

Command-line flags:

| Flag | Description |
|---|---|
| `--ollama http://...` | Custom Ollama endpoint |
| `--no-start` | Don't auto-start Ollama; fail if not running |
| `--select` | Run inline (allows mouse text selection) |

---

## Project Structure

```
mini_wiki_2.0/
  main.go                       # Entry point
  internal/
    app/app.go                  # Bubbletea TUI (model, update, view)
    config/manager.go           # Config persistence (YAML)
    conversation/types.go       # Message & Thread data structures
    csvparser/parser.go         # Streaming CSV parser
    export/exporter.go          # .xlsx export
    fileref/resolver.go         # @file reference resolver
    filescanner/scanner.go      # Safe directory scanner
    jsonlparser/parser.go       # JSONL streaming parser
    kb/db.go                    # Global knowledge base (SQLite + FTS5)
    memory/memory.go            # Tool memory (skills, flaws, session)
    modelmgr/manager.go         # Model lifecycle & fallback
    ollama/                     # Ollama HTTP client
    projectkb/projectkb.go      # Per-project KB (SQLite)
    srs/                        # SRS generation pipeline (5 stages)
    webfetch/fetcher.go         # Web fetcher with SSRF protection
    wiki/errors.go              # Structured error types
```

---

## Troubleshooting

### "Ollama is not reachable"

```bash
# Check if Ollama is running
ollama list

# Start Ollama if not running
ollama serve

# Or let wiki auto-start it (default behavior)
wiki
```

### "No models available"

```bash
# Pull a model
ollama pull qwen2.5-coder
ollama pull llama3.1:8b

# Then run /refresh in wiki or restart
```

### "Permission denied" when copying to /usr/local/bin

```bash
# Use user-local bin instead
cp wiki ~/.local/bin/wiki

# Make sure it's in PATH
echo $PATH  # should include /home/$USER/.local/bin
```

### "Text file busy" when updating binary

```bash
# Kill running instances first
pkill -f wiki

# Then copy
cp wiki /home/$USER/.local/bin/wiki
```

### "Layout is broken / overflowing"

The tool calculates layout based on your terminal size. Try:
- Resizing your terminal to at least 80x24
- Running with `--select` flag (inline mode)
- Checking if your terminal supports 256 colors (`echo $TERM`)

### "SRS pipeline is slow"

Each of the 5 stages calls the local LLM sequentially. The whole pipeline can take
several minutes depending on your model. Tips:
- Use a faster model like `gemma4:e4b`
- Reduce the amount of data in the conversation before running `/srs`
- Temperature is set to 0.1 for deterministic output

### "Text selection doesn't work"

By default, wiki uses the alternate screen buffer (full-screen TUI).
To select and copy text:
- **Most terminals**: hold Shift while clicking/dragging
- **Alacritty/Kitty**: Ctrl+Shift+click
- **Alternative**: run with `wiki --select` for inline mode

---

## Building from Source

```bash
git clone https://github.com/HussamAlraggad/mini_wiki_2.0.git
cd mini_wiki_2.0

# Build with debug symbols (larger, but better stack traces)
go build -o wiki .

# Build stripped (smaller binary)
go build -ldflags="-s -w" -o wiki .

# Cross-compile for different architectures
GOOS=linux GOARCH=amd64 go build -o wiki-linux-amd64 .
GOOS=darwin GOARCH=amd64 go build -o wiki-darwin-amd64 .
```
