# mini-wiki

A terminal-based AI research assistant for dataset analysis — fully local, no cloud APIs.

Ingest CSV, JSONL, or text files → build a RAG knowledge base → chat with your data →
rank rows by relevance to a research topic → visualize with ASCII charts → export to Excel/CSV/JSON.

---

## Quick Start

```bash
# One-time setup (installs Python deps + Ollama models)
bash setup.sh

# Build
go build -o wiki .

# Run from any research directory
cd ~/my_dataset_folder
wiki
```

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Workflow](#workflow)
- [Complete Command Reference](#complete-command-reference)
- [Feature Overview](#feature-overview)
- [Project Structure](#project-structure)
- [Troubleshooting](#troubleshooting)

---

## Prerequisites

| Component | Required | Notes |
|---|---|---|
| Go | 1.22+ | `go version` |
| Ollama | 0.21+ | Local LLM runner (`ollama --version`) |
| Python | 3.10+ | For RAG worker (`python3 --version`) |
| GPU | 8GB VRAM | RTX 4060 / RTX 5060 recommended |
| RAM | 16GB+ | For larger datasets |

---

## Installation

### Automated (recommended)

```bash
git clone https://github.com/HussamAlraggad/mini_wiki_2.0.git
cd mini_wiki_2.0
bash setup.sh        # installs Python deps, Ollama, embedding models
go build -o wiki .
cp wiki ~/.local/bin/wiki
```

### Manual

```bash
# 1. Install Ollama
curl -fsSL https://ollama.com/install.sh | sh

# 2. Python dependencies
pip install chromadb ollama unstructured pypdf

# 3. Pull models
ollama pull qwen2.5-coder     # chat model
ollama pull nomic-embed-text   # embeddings
ollama pull all-minilm         # lightweight embeddings

# 4. Build
go build -o wiki .
cp wiki ~/.local/bin/wiki
```

---

## Workflow

```
1. /scan                    # List files in current directory
2. /ingest @dataset.csv     # Ingest into RAG knowledge base
3. Type questions            # Auto-RAG searches your data
4. /rank "research topic"   # Score rows by relevance to topic
5. /chart bar column=score  # Visualize the results
6. /export --ranked          # Export ranked data to Excel
```

---

## Complete Command Reference

### Data Ingestion

| Command | Description |
|---|---|
| `/scan` | Scan current directory for files |
| `/files` | List scanned files |
| `/ingest @file` | Ingest file into RAG knowledge base |
| `/infer @file` | Auto-detect file format |

### Chat & RAG

| Command | Description |
|---|---|
| *(type a question)* | Auto-RAG: searches KB, answers with sources |
| `/model <name>` | Switch active LLM model |
| `/models` | List available models from Ollama |
| `/refresh` | Reload model list |
| `/clear` | Clear conversation history |
| `/system <text>` | Set custom system prompt |
| `/cancel` | Cancel current operation |
| `@filename` | Reference a file in chat |

### Ranking

| Command | Description |
|---|---|
| `/rank <topic>` | Score every row against a research topic |
| `/compare [<topic>]` | Compare rankings side-by-side |
| `/discard <threshold>` | Preview and confirm row removal |
| `/discard --preview <t>` | Preview without confirmation |
| `/discard --reset` | Restore all discarded rows |

### Charts

| Command | Description |
|---|---|
| `/chart bar column=<col>` | Horizontal bar chart |
| `/chart trend column=<col>` | Line/trend chart |
| `/chart pie column=<col>` | Pie chart (proportional) |
| `/chart scatter x=<col> y=<col>` | Scatter plot |
| `/chart histogram column=<col> buckets=<n>` | Frequency histogram |
| `/chart box column=<col>` | Box plot with stats |
| `/chart heatmap x=<col> y=<col>` | Category heatmap |

### Export

| Command | Description |
|---|---|
| `/export` | Export conversation to Excel |
| `/export --ranked` | Export ranked dataset with scores |
| `/export --format=csv` | Export as CSV |
| `/export --format=json` | Export as JSON |
| `/export --format=md` | Export as Markdown table |
| `/export --output=<path>` | Specify output file |

### System

| Command | Description |
|---|---|
| `/wizard` | System check and setup assistant |
| `/help` | Show all commands |
| `/bookmark <title>` | Save current finding |
| `/bookmarks` | List bookmarks |
| `/history` | Recent query history |
| `/skills` | List tool capabilities |
| `/flaws` | Known issues and solutions |
| `/task <desc>` | Add a task |
| `/tasks` | List tasks |
| `/exit` | Quit |

---

## Feature Overview

### RAG Knowledge Base
- Automatic: ingest any file → chunked → embedded → searchable via ChromaDB
- Every question auto-searches the KB for relevant context
- Per-project storage in `.wiki/rag/`

### Relevance Ranking
- `/rank` scores every row against your research topic using the LLM
- Results sorted by relevance, stored for comparison
- `/compare` shows side-by-side with score deltas
- `/discard` removes low-scoring rows from the working set

### Data Visualization
- 7 chart types rendered as clean ASCII terminal graphics
- Auto-scales to terminal width
- Column auto-detection if not specified

### Smart Export
- Export to Excel (`.xlsx`), CSV, JSON, or Markdown
- `--ranked` flag includes relevance scores
- Type-aware formatting, formula injection protection

### Auto-Format Detection
- `/infer` detects file format by extension + magic bytes
- Supports: CSV, TSV, JSONL, NDJSON, JSON arrays, XLSX, ODS, TXT, MD, YAML, etc.

---

## Project Structure

```
mini_wiki_2.0/
  main.go                    # Entry point
  setup.sh                   # Automated dependency installer
  rag_worker/                # Python RAG engine (embedded in binary)
    main.py, chunker.py, embedder.py, vectordb.py, ingester.py, querier.py
  internal/
    app/app.go               # Bubbletea TUI (model, update, view)
    chart/chart.go            # 7 ASCII chart types
    config/manager.go         # YAML config
    conversation/types.go     # Message & thread types
    csvparser/parser.go       # Streaming CSV parser
    dataset/dataset.go        # Shared data types (Dataset, Row, Column)
    export/exporter.go        # XLSX/CSV/JSON/MD export
    fileref/resolver.go       # @file reference resolver
    filescanner/scanner.go    # Safe directory scanner
    jsonlparser/parser.go     # JSONL streaming parser
    memory/memory.go          # Tool memory (skills, flaws)
    modelmgr/manager.go       # Model lifecycle & fallback
    ollama/                   # Ollama HTTP client + launcher
    projectkb/projectkb.go    # Per-project SQLite KB
    rag/client.go             # Go RAG client (spawns Python worker)
    ranking/ranker.go         # Relevance ranking engine
    wiki/errors.go            # Structured error types
    webfetch/                 # Deprecated (kept for reference)
    srs/                      # Deprecated (kept for reference)
```

---

## Troubleshooting

| Problem | Solution |
|---|---|
| `Python error: ModuleNotFoundError` | `pip install chromadb ollama unstructured pypdf` |
| `Ollama is not reachable` | `ollama serve` |
| `No models available` | `ollama pull qwen2.5-coder` |
| `/rank says "no dataset ingested"` | Run `/ingest @file.csv` first |
| `/ingest hangs` | Press Escape to cancel |
| Layout broken | Resize terminal to at least 80x24 |
| Text selection doesn't work | `wiki --select` for inline mode |
