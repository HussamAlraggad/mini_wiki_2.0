# mini-wiki

A terminal-based AI research assistant for dataset analysis — fully local, no cloud APIs.

Ingest CSV, JSONL, XLSX, or TXT files → rank rows by relevance to a research topic →
visualize with ASCII charts → export to Excel/CSV/JSON. Everything runs offline on your GPU.

---

## Quick Start

```bash
# One-time setup
bash setup.sh
go build -o wiki .
cp wiki ~/.local/bin/wiki

# Run
cd ~/my_dataset_folder
wiki
```

---

## Installation

### Automated

```bash
git clone https://github.com/HussamAlraggad/mini_wiki_2.0.git
cd mini_wiki_2.0
bash setup.sh                          # installs Python deps + Ollama models
go build -o wiki .
cp wiki ~/.local/bin/wiki              # optional: global install
```

### Manual

```bash
# Python deps (required for RAG)
pip install chromadb ollama unstructured pypdf

# Recommended models
ollama pull nomic-embed-text            # embeddings (required for /embed)
ollama pull llama3.1:8b                 # chat + ranking (default)
ollama pull deepseek-coder              # best for analysis
ollama pull deepseek-r1:8b              # deep reasoning

# Build
go build -o wiki .
cp wiki ~/.local/bin/wiki
```

### Pulling updates from GitHub

```bash
cd mini_wiki_2.0
git pull origin main
go build -o wiki .
cp wiki ~/.local/bin/wiki
```

---

## Workflow

```
1. /ingest @dataset.csv     # Parse file (3 seconds) — ready for /rank
2. /rank "research topic"   # Score every row (GPU, stream progress)
3. /chart bar column=score  # Visualize results
4. /export --ranked          # Export to Excel with scores
5. /embed                    # (Optional) Index for RAG chat
6. Type questions            # Auto-RAG if embedded
```

---

## Models

### Recommended models

| Model | Size | When to use | Command |
|---|---|---|---|
| **llama3.1:8b** | 4.9 GB | Default — good all-rounder | `/model llama3.1:8b` |
| **deepseek-coder** | 776 MB | Best for analysis & ranking | `/model deepseek-coder` |
| **deepseek-r1:8b** | 5.2 GB | Deep reasoning (slower) | `/model deepseek-r1:8b` |
| **codeqwen** | 4.2 GB | Alternative analysis | `/model codeqwen` |
| **nomic-embed-text** | 274 MB | Required for `/embed` | *(auto-used)* |
| **all-minilm** | 45 MB | Lightweight embeddings | *(auto-used)* |

### Remove unused models to save disk

```bash
ollama rm deepseek-ocr:latest llama3:latest Nova:latest codellama:7b codellama:13b llama3.2:3b
```

---

## Text Selection

| Mode | Command | Selection |
|---|---|---|
| **Full-screen (default)** | `wiki` | Hold **Shift** + click-drag, or **Ctrl+Shift+C** to copy |
| **Inline** | `wiki --select` | Free click-drag selection with mouse |

---

## Complete Command Reference

### Data

| Command | Description |
|---|---|
| `/scan` | Scan current directory for files |
| `/files` | List scanned files |
| `/ingest @file` | Parse file and register as active dataset (seconds) |
| `/infer @file` | Auto-detect file format |
| `/embed` | Embed active dataset for RAG search (slow, background) |

### Chat & RAG

| Command | Description |
|---|---|
| *(type a question)* | Auto-RAG searches KB if embedded |
| `/model <name>` | Switch active LLM model |
| `/models` | List available models |
| `/clear` | Clear conversation |
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
| `/export --format=csv\|json\|md\|xlsx` | Choose format |
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

### Context Compaction
When the conversation exceeds ~70% of the model's context window, older messages are automatically summarized by the LLM and replaced with a compact context entry. The last 4 messages stay intact. This preserves conversation flow without hitting token limits.

### RAG Knowledge Base
- `/ingest` parses files locally (seconds) — no Python worker needed for `/rank`
- `/embed` indexes chunks for RAG search (optional, runs in background)
- Per-project storage in `.wiki/rag/`
- Retrieved chunks are formatted as clean readable text (no raw JSON in prompts)

### Relevance Ranking
- `/rank` scores every row against a research topic using the LLM
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
- Formula injection protection

### Auto-Format Detection
- `/infer` detects file format by extension + magic bytes
- Supports: CSV, TSV, JSONL, NDJSON, JSON arrays, XLSX, TXT, MD

---

## Performance Estimates (RTX 4060, 8GB VRAM)

| Operation | Time |
|---|---|
| `/ingest` (parse 1GB file) | ~3 seconds |
| `/rank` (score 92K rows) | ~3.8 hours |
| `/embed` (92K chunks) | ~6 minutes |
| Context compaction | ~30 seconds (LLM summarization) |

---

## Project Structure

```
mini_wiki_2.0/
  main.go                       # Entry point
  setup.sh                      # Automated dependency installer
  rag_worker/                   # Python RAG engine (embedded in binary)
    main.py, chunker.py, embedder.py, vectordb.py, ingester.py, querier.py
  internal/
    app/app.go                  # Bubbletea TUI (model, update, view)
    chart/chart.go              # 7 ASCII chart types
    config/manager.go           # YAML config
    conversation/types.go       # Message & thread types
    csvparser/parser.go         # Streaming CSV parser
    dataset/dataset.go          # Shared data types
    export/exporter.go          # XLSX/CSV/JSON/MD export
    fileref/resolver.go         # @file reference resolver
    filescanner/scanner.go      # Safe directory scanner
    jsonlparser/parser.go       # JSONL streaming parser
    memory/memory.go            # Tool memory (skills, flaws)
    modelmgr/manager.go         # Model lifecycle
    ollama/                     # Ollama HTTP client + launcher
    projectkb/projectkb.go      # Per-project SQLite KB
    rag/client.go               # Go RAG client (spawns Python worker)
    ranking/ranker.go           # Relevance ranking engine
    wiki/errors.go              # Structured error types
```

---

## Troubleshooting

| Problem | Solution |
|---|---|
| `Python error: ModuleNotFoundError` | `pip install chromadb ollama unstructured pypdf` |
| `Ollama is not reachable` | `ollama serve` |
| `/rank says "no dataset ingested"` | Run `/ingest @file.csv` first |
| `/ingest or /embed hangs` | Press **Escape** to cancel |
| Text selection in full-screen | Hold **Shift** + click-drag, or use `wiki --select` |
| Layout broken | Resize terminal to at least 80x24 |
| Slow responses | Try `/model deepseek-coder` or `/model codeqwen` |
| Context window limit | Auto-compaction kicks in at 70% — just keep chatting |
