# mini-wiki

A standalone Terminal User Interface (TUI) AI research assistant powered entirely by local LLMs via Ollama.
Designed for Software Requirements Engineering research: extract requirements, generate IEEE 830 SRS documents,
analyze datasets with a RAG pipeline — all from your terminal, fully offline.

---

## Quick Start

```bash
# One-command setup (installs everything)
bash setup.sh

# Build
go build -o wiki .

# Run from any research directory
cd ~/my_research_project
wiki
```

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Install (Automated)](#quick-install-automated)
- [Manual Installation](#manual-installation)
- [First Run](#first-run)
- [RAG Pipeline](#rag-pipeline)
- [Complete Command Reference](#complete-command-reference)
- [Feature Overview](#feature-overview)
- [Layout Guide](#layout-guide)
- [Troubleshooting](#troubleshooting)
- [Project Structure](#project-structure)

---

## Prerequisites

| Component | Required | Notes |
|---|---|---|
| Go | 1.22+ | `go version` |
| Ollama | 0.21+ | Local LLM runner |
| Python | 3.10+ | For RAG worker (chromadb, ollama, unstructured) |
| GPU | 8GB VRAM | RTX 4060 / RTX 5060 recommended |
| RAM | 16GB+ | For larger datasets |

---

## Quick Install (Automated)

```bash
# Clone the repo
git clone https://github.com/HussamAlraggad/mini_wiki_2.0.git
cd mini_wiki_2.0

# Run the setup script (installs Python deps, Ollama, embedding models)
bash setup.sh

# Build
go build -o wiki .

# (Optional) Install globally
cp wiki ~/.local/bin/wiki
```

The `setup.sh` script handles:
- Installing Python packages (chromadb, ollama, unstructured, pypdf)
- Installing Ollama (if missing)
- Starting Ollama
- Pulling embedding models (nomic-embed-text, all-minilm)

---

## Manual Installation

### 1. Install Ollama

```bash
# Linux
curl -fsSL https://ollama.com/install.sh | sh

# macOS
brew install ollama

# Verify
ollama --version
```

### 2. Install Python Dependencies

```bash
pip install chromadb ollama unstructured pypdf

# On Debian/Ubuntu (externally managed):
pip install --break-system-packages chromadb ollama unstructured pypdf
```

### 3. Pull Models

```bash
# Required for RAG embeddings (both recommended)
ollama pull nomic-embed-text
ollama pull all-minilm

# Required for chat and SRS generation
ollama pull qwen2.5-coder
```

### 4. Build wiki

```bash
go build -o wiki .
cp wiki ~/.local/bin/wiki   # optional: install globally
```

---

## First Run

```bash
cd ~/my_research_project
wiki
```

The tool will:
1. Auto-scan the directory for files
2. Auto-start Ollama if not running
3. Show the welcome screen

Type messages to chat, use `/` commands for actions, `@filename` to reference files.

---

## RAG Pipeline

The tool includes a full local RAG (Retrieval-Augmented Generation) pipeline:

```
File (PDF, CSV, JSONL, TXT, MD)
        │
        ▼  /ingest @file
  Python RAG Worker (spawned automatically)
        │
        ├── 1. Extract text (unstructured for PDF, direct for text)
        ├── 2. Chunk (800 chars, 100 overlap)
        ├── 3. Embed via Ollama /api/embed (nomic-embed-text)
        ├── 4. Store in ChromaDB (.wiki/rag/)
        │
        ▼
  User asks a question
        │
        ├── 5. Auto-RAG: embed query → search ChromaDB → retrieve top-k chunks
        ├── 6. Inject context → send to LLM
        └── 7. Answer with source citations
```

### Automatic Memory

- Every chat message is auto-saved to the project KB (`.wiki/kb.sqlite`)
- Every question searches past conversations for relevant context
- No explicit commands needed — works like ChatGPT memory

### Ingestion Progress

When you run `/ingest @file`, you'll see live progress:

```
  [RAG] Reading dataset.jsonl (51200 KB)...
  [RAG] Chunking 1048576 characters...
  [RAG] Generated 1500 chunks
  [RAG] Embedding 1500 chunks...
  [RAG] Stored 1500 chunks in database
  [RAG done] dataset.jsonl - 1500 chunks indexed
```

### Cancel Ingestion

Press **Escape** or run `/cancel` to abort any running operation.

---

## Complete Command Reference

### System Commands

| Command | Description |
|---|---|
| `/help` | Show all available commands |
| `/model <name>` | Switch the active LLM model |
| `/models` | List all available models from Ollama |
| `/refresh` | Reload model list from Ollama |
| `/clear` | Clear the conversation history |
| `/system <text>` | Set a custom system prompt |
| `/exit` | Quit the application |

### File & Data Operations

| Command | Description |
|---|---|
| `/scan` | Scan the current directory for files |
| `/files` | List scanned files |
| `/ingest <path>` | Ingest a file into the RAG knowledge base |
| `/fetch <url>` | Fetch a webpage and extract text |

### RAG & Export

| Command | Description |
|---|---|
| `/export` | Export conversation history to `.xlsx` |
| `/cancel` | Cancel the current RAG operation |

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

### File References (in chat)

| Syntax | Description |
|---|---|
| `@filename` | Reference a file (works without /scan) |
| `@path/to/file` | Reference by relative path |
| `@dir/` | Tab-completion lists directory contents like `ls` |

---

## Feature Overview

### Chat & Conversation
- Streaming responses from local LLM via Ollama
- Model switching mid-session
- Fallback chain (auto-retry with backup model)
- Auto-RAG (searches KB before every answer)
- Command auto-completion with descriptions
- Loading spinner for all operations
- Escape cancels everything

### File System & Data
- Auto-scan on startup (files ready immediately)
- File type detection (22 types)
- CSV and JSONL streaming parsers
- PDF/text/MD via RAG pipeline (unstructured)
- Binary detection (rejects non-text files)

### RAG Knowledge Base
- ChromaDB vector store per project (`.wiki/rag/`)
- Streaming ingestion (no memory limit for large files)
- Real-time progress during ingestion
- Auto-embeds every ingested file
- Auto-RAG on every chat question

### Web Fetching
- SSRF-safe (blocks private IPs)
- HTML-to-text extraction
- 5MB limit, 30s timeout, 5 redirects

### SRS Generation Pipeline
5-stage pipeline via local LLM:
1. FR/NFR Extraction
2. MoSCoW Prioritization
3. DFD Generation
4. CSPEC Logic
5. SRS Formatting (IEEE 830)

---

## Layout Guide

```
         ~/research/my_project
  Ready  |  Tokens: 123
+-----------------------------------+-------+
| CHAT                              | SESS  |
|                                   | M: .. |
|  (your conversation with the LLM) | Tok.. |
|                                   | TASKS |
|                                   | HX    |
+-----------------------------------+-------+
|  > _ Type a message...                   |
+------------------------------------------+
|  active: model | tokens: 0 | loaded: 3   |
+------------------------------------------+
```

- **Tab** while typing → cycles through completions
- **Escape** → cancels current operation
- **Mouse** scrolls the chat viewport

---

## Troubleshooting

### "Python error: ModuleNotFoundError: No module named 'chromadb'"
```bash
pip install --break-system-packages chromadb ollama unstructured pypdf
```

### "RAG worker not available"
Run `bash setup.sh` to install all dependencies. Or manually:
```bash
pip install chromadb ollama unstructured pypdf
```

### "Ollama is not reachable"
```bash
ollama serve
```

### "No models available"
```bash
ollama pull qwen2.5-coder
ollama pull nomic-embed-text
```

### "/ingest hangs or freezes"
Press **Escape** to cancel the operation. The RAG worker will be killed and restarted on next use.

### "Text selection doesn't work"
Run with `wiki --select` for inline mode (allows mouse text selection).

### Layout is broken
Resize your terminal to at least 80x24 characters.

---

## Project Structure

```
mini_wiki_2.0/
  main.go                       # Entry point (embeds rag_worker/)
  setup.sh                      # Automated dependency installer
  rag_worker/                   # Python RAG engine (embedded in Go binary)
    main.py                     # Stdin/stdout JSON protocol dispatcher
    chunker.py                  # Recursive text splitter
    embedder.py                 # Ollama /api/embed client
    vectordb.py                 # ChromaDB wrapper
    ingester.py                 # Document ingestion (PDF, CSV, JSONL, TXT)
    querier.py                  # RAG query pipeline
    requirements.txt            # Python dependencies
  internal/
    app/app.go                  # Bubbletea TUI (model, update, view)
    config/manager.go           # Config persistence (YAML)
    conversation/types.go       # Message & Thread data structures
    csvparser/parser.go         # Streaming CSV parser
    export/exporter.go          # .xlsx export
    fileref/resolver.go         # @file reference resolver
    filescanner/scanner.go      # Safe directory scanner
    jsonlparser/parser.go       # JSONL streaming parser
    memory/memory.go            # Tool memory (skills, flaws, session)
    modelmgr/manager.go         # Model lifecycle & fallback
    ollama/                     # Ollama HTTP client
    projectkb/projectkb.go      # Per-project SQLite KB
    rag/client.go               # Go RAG client (spawns Python worker)
    srs/                        # SRS generation pipeline (5 stages)
    webfetch/fetcher.go         # Web fetcher with SSRF protection
    wiki/errors.go              # Structured error types
```
