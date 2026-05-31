# mini-wiki

A terminal-based AI research assistant for dataset analysis — fully local, no cloud APIs.

**Ingest** CSV / JSONL / XLSX / TSV → **Rank** by relevance via Agentic AI (seconds, not hours) →
**Visualize** with ASCII charts → **Export** to Excel/CSV/JSON. Everything runs offline on your GPU.

**Key Features:**
- **Natural Language Commands** — Say "find rows about X" instead of `/rank X`, "chart column Y" instead of `/chart bar column=Y`. The LLM detects your intent automatically.
- **Agentic Ranking** — LLM writes Pandas code to rank/filter datasets. One LLM call, not 92K.
- **Agentic Query** — Ask questions about your data in natural language. Gets exact Pandas-powered answers.
- **Deep RAG** — gemma4 reads each retrieved chunk like a human researcher for richer answers.
- **Proactive Assistant** — After ingesting data, automatically analyzes and summarizes your dataset.
- **Responsive TUI** — Adapts to terminal size (60-79 narrow, 80-119 medium, 120+ wide). Auto-hides panels on small screens.
- **Auto-expanding input** — Textarea grows as you type (proportional to terminal height). Alt+Enter for newlines.
- **5-container TUI** — Isolated panels: chat, input, right panel, header, footer. Independent scrolling.

---

## Quick Start

```bash
# One-time setup
bash setup.sh
go build -o wiki .
cp wiki ~/.local/bin/wiki

# Run from any dataset directory
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
# Python deps (install inside .venv/ or use system pip)
pip install chromadb ollama unstructured pypdf pandas

# Required models
ollama pull nomic-embed-text           # RAG embeddings (needed for /embed)
ollama pull gemma4:e4b                 # Default chat (131K context, thinking)

# Recommended for Agentic Ranking
ollama pull qwen2.5-coder:7b           # Code generation for /rank (fast)
ollama pull deepseek-r1:8b             # Deep reasoning

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
1. /ingest @dataset.csv         # Parse file (< 1 second) — ready for /rank
2. "find rows about X"          # NL intent → auto ranks by topic
3. Type a question              # Auto Agentic Query + Deep RAG answer
4. "chart the score column"     # NL intent → auto generates chart
5. /export --ranked              # Export to Excel with relevance scores
6. /embed [--deep]               # (Optional) Index for RAG chat with deep reading
```

---

## How Agentic Ranking Works

Traditional RAG scores every row individually via the LLM — **one API call per row** (hours for large datasets).

**mini-wiki uses Agentic RAG**:

1. Loads the dataset schema (column names + types) + 3 sample rows
2. Sends schema + your research topic to `qwen2.5-coder:7b`
3. The LLM writes a **Pandas `filter_data(df)`** function
4. The function runs **locally in a sandbox** (pandas/numpy/json only — no `os`/`sys`/`subprocess`)
5. Returns filtered rows with `relevance_score` (1-100)

**Result: 1 LLM call regardless of dataset size. Rows ranked in seconds.**

```json
{"cmd": "rank", "path": "dataset.csv", "topic": "research topic"}
       │
       ▼
Python worker: load CSV → extract schema → prompt qwen2.5-coder → execute Pandas → return JSON
       │
       ▼
Go TUI: display ranked table
```

---

## Models

### Recommended models

| Model | Size | Purpose | Command |
|---|---|---|---|
| **gemma4:e4b** | 9.6 GB | Default chat (thinking enabled, 131K ctx) | `/model gemma4:e4b` |
| **qwen2.5-coder:7b** | 4.7 GB | Code generation for Agentic Ranking | `/model qwen2.5-coder:7b` |
| **deepseek-r1:8b** | 5.2 GB | Deep reasoning (slower) | `/model deepseek-r1:8b` |
| **nomic-embed-text** | 274 MB | Required for `/embed` | *(auto-used)* |
| **all-minilm** | 45 MB | Lightweight embeddings (fallback) | *(auto-used)* |

### New model needed?

```bash
ollama pull <model-name>
```
Then inside the TUI: `/refresh` to see it, `/model <name>` to activate it.

---

## Text Selection & Scrolling

| Action | How |
|---|---|
| **Mouse wheel** | Scrolls chat or right panel (routes by cursor position) |
| **Left-click** | Disables mouse tracking → native terminal text selection works |
| **Type in input** | Re-enables mouse tracking → wheel scrolling works again |
| **`/clip`** | Copy entire viewport to clipboard |

Cycle: **scroll → click → select → type → scroll → ...**

## Input & Newlines

| Key | Action |
|---|---|
| **Enter** | Submit message |
| **Alt+Enter** | Insert newline in message (multi-line input) |
| **Input box** | Auto-expands vertically as you type (proportional to terminal height, max 25%) |

---

## Natural Language Commands (Phase 8)

Instead of memorizing slash commands, just talk naturally. The system detects your intent:

| You say | What happens |
|---|---|
| "find rows about machine learning" | Auto-runs `/rank machine learning` |
| "show me a pie chart of categories" | Auto-runs `/chart pie column=categories` |
| "save these results as CSV" | Auto-runs `/export --format csv` |
| "what data do I have loaded?" | Shows dataset info |
| "load the data.csv file" | Auto-runs `/ingest @data.csv` |
| "remove rows below 0.5" | Auto-runs `/discard 0.5` |

All existing `/slash` commands still work. NL intent detection runs in parallel — if it detects a tool, it executes it and wraps the result conversationally.

---

## Complete Command Reference

### Data

| Command | Description |
|---|---|
| `/scan` | Scan current directory for files |
| `/files` | List scanned files |
| `/ingest @file` | Parse file and register as active dataset (< 1 second) |
| `/infer @file` | Auto-detect file format |
| `/embed` | Embed for RAG chat (optional, slow — see progress with ETA) |
| `/embed --deep` | Embed with AI deep reading (gemma4 reads each chunk like a human) |

### Agentic Ranking

| Command | Description |
|---|---|
| `/rank <topic>` | Rank dataset by relevance (sends schema to coder LLM, seconds) |
| `/compare [<topic>]` | Re-rank with refined topic, compare side-by-side |
| `/discard <threshold>` | Remove rows below relevance score |
| `/discard --preview <t>` | Preview without confirming |
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
| `/export` | Export dataset to Excel |
| `/export --ranked` | Include relevance scores, sorted descending |
| `/export --format=csv|json|xlsx` | Choose format |
| `/export --output=<path>` | Specify output file |

### System

| Command | Description |
|---|---|
| `/wizard` | System check and setup assistant |
| `/model <name>` | Switch active LLM model |
| `/models` | List available models |
| `/refresh` | Reload model list from Ollama |
| `/clear` | Clear conversation |
| `/system <text>` | Set custom system prompt |
| `/help [command]` | Show summary or man page for a command |
| `/panel` | Toggle right info panel |
| `/clip` | Copy viewport text to clipboard |
| `/cancel` | Cancel current operation |
| `/exit` | Quit |

### Bookmarks & History

| Command | Description |
|---|---|
| `/bookmark <title>` | Save current finding |
| `/bookmarks` | List bookmarks |
| `/history` | Recent query history |
| `/skills` | List tool capabilities |
| `/flaws` | Known issues and solutions |
| `/task <desc>` | Add a task |
| `/tasks` | List tasks |

### File References (in chat)

| Syntax | Description |
|---|---|
| `@filename` | Reference a file (works without /scan) |
| `@path/to/file` | Reference by relative path |

---

## Feature Overview

### Natural Language Intent Detection (Phase 8)
- **LLM-based intent classification**: Before every chat response, a fast non-streaming LLM call classifies your intent
- **Tool dispatch**: If intent is "rank", "chart", "export", etc., the tool runs automatically
- **Conversational wrapping**: Tool results are fed back to the LLM, which explains them conversationally
- **Fallback**: If no tool matches, standard RAG + Agentic Query flow runs unchanged

### AppState Management (Phase 8)
- Explicit Bubble Tea states: `StateIdle`, `StateStreaming`, `StateSearching`, `StateRanking`, `StateCharting`, `StateExporting`, `StateIngesting`, `StateConfirming`
- Replaces boolean flag soup for clear cognitive boundaries

### Responsive TUI Layout (Phase 8b)
- **Narrow mode** (< 80 cols): Single column, compact header `[+]`, no sub-header, model name only in footer, right panel auto-hidden
- **Medium mode** (80-119): Full header, abbreviated footer, right panel optional
- **Wide mode** (>= 120): Full header, detailed footer, full layout
- **Proportional input**: Max height = 25% of terminal height (clamped 3-12)
- **Compact welcome logo**: 4-line logo on narrow panels
- **Minimum terminal**: 60x16 (was 80x24)

### Agentic Ranking
- `/rank` sends the **schema only** (not the data) to `qwen2.5-coder:7b`
- LLM writes a **Pandas filter script** — executed locally, sandboxed
- **O(1) LLM calls** regardless of dataset size
- Datasets of any size ranked in **seconds** (not hours)
- `/compare` for iterative refinement with visual deltas
- `/discard` to remove low-relevance rows with preview

### Proactive Assistant
- After `/ingest`: auto-triggers Agentic Query to summarize the dataset
- After `/rank`: offering next steps conversationally

### RAG Knowledge Base
- `/ingest` parses files and registers them instantly — no Python worker needed for `/rank`
- `/embed` indexes chunks for semantic search (optional, runs in background with live progress)
- Chunk size: 4000 characters with 400 overlap
- ChromaDB per project (`.wiki/rag/`)

### Data Visualization
- 7 chart types rendered as ASCII terminal graphics
- Auto-scales to terminal width
- Export to PNG/SVG with `--export` flag

### Smart Export
- Excel (`.xlsx`), CSV, JSON, Markdown
- `--ranked` flag includes `relevance_score` column
- Formula injection protection
- Auto-column width and type formatting

### Auto-Format Detection
- `/infer` detects format by extension + magic bytes
- Supports: CSV, TSV, JSONL, NDJSON, JSON arrays, XLSX, TXT, MD

### Text Selection
- Click-drag anywhere in the TUI → native terminal selection
- `/clip` command copies entire viewport

### Man-Page Help System
- `/help` shows a summary (like `command --help`)
- `/help <command>` shows full man page for that command
- Every man page includes: NAME, SYNOPSIS, DESCRIPTION, BEHAVIOR, EXAMPLE, SEE ALSO

---

## Test Coverage

18 test suites, 340+ tests, ~50% overall coverage.

| Package | Coverage | Status |
|---------|----------|--------|
| modelmgr | **100%** | Full |
| wiki (errors) | **100%** | Full |
| conversation | **94.4%** | Near-full |
| dataset | **92.9%** | Near-full |
| config | **91.7%** | Near-full |
| memory | **90.4%** | Near-full |
| export | **88.9%** | Near-full |
| kb | **86.6%** | Near-full |
| projectkb | **86.1%** | Near-full |
| jsonlparser | **86.8%** | Near-full |
| chart | **82.8%** | Good |
| ranking | **77.5%** | Good |
| fileref | **75.2%** | Good |
| filescanner | **72.8%** | Good |
| csvparser | **71.7%** | Good |
| ollama | **65.9%** | Moderate |
| app (TUI) | **11.3%** | Helpers + commands |
| rag (protocol) | **7.7%** | Types only |

---

## Performance Estimates (RTX 4060, 8GB VRAM)

| Operation | Time | Notes |
|---|---|---|
| `/ingest` (parse 1GB file) | < 1 second | Counts newlines in chunks |
| `/rank` (any dataset) | **seconds** | Agentic RAG — 1 LLM call |
| `/embed` (large file) | ~1-2 hours | 4000-char chunks, live ETA shown |
| `/embed` (small file) | ~1-5 minutes | Depends on file size |

---

## Project Structure

```
mini_wiki_2.0/
  main.go                          # Entry point
  setup.sh                         # Automated dependency installer
  .venv/                           # Python virtual env (gitignored)
  rag_worker/                      # Python engine (embedded in Go binary)
    main.py                        # Stdin/stdout JSON protocol dispatcher
    agentic_ranker.py              # Agentic RAG: LLM writes Pandas code, executes locally
    chunker.py                     # Recursive text splitter (4000-char chunks)
    embedder.py                    # Ollama /api/embed client
    vectordb.py                    # ChromaDB wrapper
    ingester.py                    # Document ingestion (CSV, JSONL, TXT)
    querier.py                     # RAG query pipeline + call_llm()
    requirements.txt
  internal/
    app/
      app.go                       # Bubbletea TUI (model, update, view)
      intent.go                    # Phase 8: Tool definitions, intent classification, dispatch
      intent_test.go               # Phase 8: 14 tests for intent detection
    chart/chart.go                 # 7 ASCII chart types + PNG export
    config/manager.go              # YAML config (~/.config/mini-wiki/)
    conversation/types.go          # Message & thread types
    csvparser/parser.go            # Streaming CSV parser
    dataset/dataset.go             # Shared data types (Dataset, Row, Column)
    export/exporter.go             # XLSX/CSV/JSON/MD export
    fileref/resolver.go            # @file reference resolver
    filescanner/scanner.go         # Safe directory scanner
    jsonlparser/parser.go          # JSONL streaming parser
    kb/db.go                       # SQLite knowledge base (FTS5)
    memory/memory.go               # Tool memory (skills, flaws, session)
    modelmgr/manager.go            # Model lifecycle & fallback
    ollama/                        # Ollama HTTP client + process launcher
    projectkb/projectkb.go         # Per-project SQLite KB
    rag/client.go                  # Go RAG client (spawns Python worker)
    ranking/ranker.go              # Agentic ranking engine
    wiki/errors.go                 # Structured error types (26 kinds)
```

---

## Troubleshooting

| Problem | Solution |
|---|---|
| `Python error: ModuleNotFoundError` | `.venv/bin/pip install chromadb ollama unstructured pypdf pandas` |
| `Ollama is not reachable` | `ollama serve` |
| `/rank says "no dataset ingested"` | Run `/ingest @dataset.csv` first |
| `/rank says "RAG worker unavailable"` | Try `/embed` first (needed for the Python worker) |
| `/ingest or /embed hangs` | Press **Escape** to cancel |
| Text selection in full-screen | Click-drag to select, release to auto-copy. Or `/clip` |
| Layout broken | Resize terminal to at least 60x16 |
| Model not listed in `/models` | Run `/refresh` |
| Slow /embed | Increase chunk size or use a smaller dataset. Press Escape to cancel |
| NL intent not triggering tools | Make sure your active chat model supports function-calling-like behavior |

---

## How to Get Help

Inside the TUI:
- `/help` — command summary
- `/help rank` — full man page for ranking
- `/help chart` — full man page for charts
- `/help ingest` — full man page for ingestion
- `/help <any command>` — man page for that command
