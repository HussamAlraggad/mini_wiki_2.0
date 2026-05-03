# Project Plan: Standalone TUI Dataset RAG Analysis Tool

> This document is the **single source of truth**. Every agent working on this project
> MUST read this file first and follow it precisely. Do not make assumptions. Do not
> hallucinate features. If a detail is ambiguous, stop and ask.

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Agent Ground Rules](#2-agent-ground-rules)
3. [Core Architecture & Tech Stack](#3-core-architecture--tech-stack)
4. [Design & UI/UX Rules (STRICT)](#4-design--uiux-rules-strict)
5. [Local AI & LLM Engine](#5-local-ai--llm-engine)
6. [Dataset Ingestion & Format Detection](#6-dataset-ingestion--format-detection)
7. [Conversational & Chat Functionality](#7-conversational--chat-functionality)
8. [Relevance Ranking & Iterative Comparison](#8-relevance-ranking--iterative-comparison)
9. [Data Visualization](#9-data-visualization)
10. [Smart Export & Output Generation](#10-smart-export--output-generation)
11. [Development Roadmap: Phase Details](#11-development-roadmap-phase-details)
12. [Phase Interop Contracts](#12-phase-interop-contracts)
13. [Complete Command Reference](#13-complete-command-reference)
14. [Project File Structure](#14-project-file-structure)
15. [Error Handling & Logging Conventions](#15-error-handling--logging-conventions)
16. [Testing Requirements](#16-testing-requirements)

---

## 1. Project Overview

### Objective
Develop a robust, standalone Terminal User Interface (TUI) tool powered entirely by
**local** AI models (no cloud APIs). The tool:

1. Ingests tabular datasets (CSV, XLSX, ODS, JSONL, JSON, etc.)
2. Builds a local vector knowledge base using a local LLM
3. Enables conversational querying about the dataset (RAG-based chat)
4. Ranks dataset rows by relevance to a user-provided research topic (iteratively)
5. Generates charts/graphs in the terminal and as exportable image files
6. Exports cleaned, ranked, and formatted datasets to Excel/CSV/JSON

### Execution Strategy
Build entirely from scratch in this repository. No legacy code. No copy-paste from
previous projects. Every line must be written fresh with lessons learned applied.

---

## 2. Agent Ground Rules

These rules apply to **every agent** working on this project:

### 2.1 Before Making Changes
1. Read `plan.md` (this file) in full.
2. Read `journal.md` to understand what's been done, what broke, and current state.
3. Read `AGENTS.md` for operational rules.
4. If any instruction in this file conflicts with another file, this file wins.
5. Keep the `README.m` file up to date.

### 2.2 While Working
1. NEVER make assumptions about features not explicitly listed here.
2. NEVER add features, commands, or UI elements that are not documented here.
3. NEVER use emojis, icons, or "playful" naming in code, comments, or UI strings.
4. NEVER add bright/neon colors. Use muted terminal colors only (grays, muted blues/greens).
5. ALWAYS follow the exact command syntax documented in section 13.
6. ALWAYS update `journal.md` after every session, noting what was done, what broke, and why.
7. ALWAYS run `go test ./...` before marking any phase as COMPLETE.

### 2.3 When in Doubt
ASK. Do not guess. Do not assume. Do not hallucinate feature behavior.

---

## 3. Core Architecture & Tech Stack

### 3.1 Primary Codebase
- **Language:** Go (Golang)
- **Minimum Go version:** 1.22 (current: 1.25)
- **Reasoning:** High performance, excellent concurrency, single-binary compilation.

### 3.2 Interface
- **Terminal User Interface (TUI)** via [Bubbletea](https://github.com/charmbracelet/bubbletea)
- **UI Components:** Bubbletea v1.x + Bubbles v0.20.x + Lipgloss v0.13.x
- **No web interface.** No GUI. Terminal only.

### 3.3 Execution Environment
- **Directory-agnostic:** The binary must work from any directory.
- **CWD as root:** `os.Getwd()` at startup determines the project root.
- **Per-project state:** Stored in `$CWD/.wiki/` directory.
- **Global state:** Stored in `~/.config/mini-wiki/` directory.

### 3.4 Hardware Constraints
- **GPU VRAM:** 8GB (RTX 4060 / RTX 5060 class)
- **RAM:** 16GB+ recommended
- **Models capped at:** 10B parameters or fewer
- **Quantization:** Q4_K_M or similar (4-bit) to fit 8B models in 8GB VRAM

### 3.5 Key Dependencies (already in go.mod)
| Package | Purpose |
|---|---|
| `github.com/charmbracelet/bubbletea` | TUI framework |
| `github.com/charmbracelet/bubbles` | TUI widgets (viewport, textinput, spinner, table) |
| `github.com/charmbracelet/lipgloss` | Terminal styling |
| `modernc.org/sqlite` | SQLite (pure Go, no CGO) |
| `github.com/xuri/excelize/v2` | Excel (.xlsx) read/write |
| `gopkg.in/yaml.v3` | YAML config parsing |
| `golang.org/x/net` | HTML parsing |

### 3.6 Python Dependencies (for RAG worker, in `.venv/`)
| Package | Purpose |
|---|---|
| `chromadb` | Vector store |
| `ollama` | Python client for Ollama embeddings |
| `unstructured` | Document parsing (PDF, etc.) |
| `pypdf` | PDF support |

The RAG worker is spawned as a subprocess by the Go binary. The worker uses a
JSON-over-stdin/stdout protocol. The Go code checks `.venv/bin/python3` first,
then falls back to system `python3`, then `python`.

---

## 4. Design & UI/UX Rules (STRICT)

These rules are **mandatory**. Violations will be rejected.

### 4.1 Aesthetic
- **Theme:** Terminal/minimal. Clean lines. No glossy or "modern web" styling.
- **Colors:** Muted palette only.
  - Allowed: grays (`#64748B`, `#CBD5E1`, `#334155`, `#1E293B`), muted accent blue (`#3B82F6`), muted green (`#22C55E` for success), muted red (`#EF4444` for errors).
  - FORBIDDEN: bright neon, pastel, rainbow, gradient backgrounds.
- **No borders with rounded corners** in terminal -- use straight lines (`│`, `─`, `┌`, `┐`, etc.).

### 4.2 Visuals & Icons
- **NO emojis.** EVER. Not in UI, not in messages, not in comments, not in commits.
- **NO icons.** No Unicode pictograms. No ASCII art except the ASCII art logo on welcome screen.
- **NO loading spinners with emoji.** Only use the Bubbletea `spinner.Dot` model.

### 4.3 Animations
- **ONLY loading animations:** The Bubbletea spinner (`.Dot` style) during:
  - LLM streaming responses
  - File ingestion
  - Any operation > 1 second
- **No decorative animations.** No transitions. No bounce effects.

### 4.4 Layout
- Fixed layout: chat panel (left, ~70%) + session panel (right, ~30%)
- Bottom input bar with text input
- Bottom status bar showing: active model, token count, loaded files
- Minimum terminal size: 80x24 characters
- If terminal is too small, show a centered warning message

### 4.5 Naming Convention (Code & UI)
- Use descriptive, boring names. No "clever" or "fun" names.
- No "shiny", "playful", or "magical" terminology.
- Examples of BAD names: `magicSort`, `smartFilter`, `wizardEngine`, `sparkle`
- Examples of GOOD names: `rankByRelevance`, `filterByThreshold`, `comparisonEngine`

---

## 5. Local AI & LLM Engine

### 5.1 Strictly Local Operation
- **NO cloud API calls.** Not OpenAI, not Anthropic, not any external LLM provider.
- **Only Ollama** as the LLM backend. Ollama must be installed on the machine.
- If Ollama is not running, the tool **auto-starts** it (`ollama serve` subprocess).
- The user can disable auto-start with `--no-start` flag.

### 5.2 Model Constraints
- **Max 10B parameters.** Models larger than this will OOM on 8GB VRAM.
- **Q4_K_M quantization** recommended for 8B models on 8GB VRAM.

### 5.3 Model Manager (`internal/modelmgr/`)
- Tracks: `active` model (current), `fallback` model (auto-retry on failure).
- Commands:
  - `/model <name>` -- switch active model (case-insensitive partial match).
  - `/models` -- list all available models from Ollama, mark the active one.
  - `/refresh` -- re-query Ollama for available models.
- Fallback chain: If the active model fails (timeout, error, empty response),
  automatically retry with the fallback model. Show a status message: `retrying with <fallback>`.

### 5.4 Ollama Client (`internal/ollama/`)
- Hardcoded to `127.0.0.1:11434` (NOT `localhost` -- DNS rebinding protection).
- Context timeouts on all API calls (30s default for chat, 5s for ping).
- Streaming support for chat (server-sent events via `ndjson`).
- Launcher sub-package: spawns `ollama serve` if not running, tracks PID,
  kills only if we started it (graceful shutdown on tool exit).

---

## 6. Dataset Ingestion & Format Detection

### 6.1 Auto-Format Detection
When a file is provided via `/ingest @<file>`, the tool must:

1. Check file extension first:
   - `.csv` -> CSV parser
   - `.xlsx` -> Excel parser (excelize)
   - `.ods` -> Libre Sheets parser
   - `.json` -> JSON parser (expects array of objects)
   - `.jsonl` / `.jsonlines` / `.ndjson` -> JSONL parser (one JSON object per line)
   - `.tsv` -> CSV parser with tab delimiter
2. If extension is unknown or missing, read the first 512 bytes and detect:
   - Looks like CSV (commas, newlines, consistent columns) -> CSV parser
   - Looks like JSON array -> JSON parser
   - Looks like JSONL -> JSONL parser
   - Looks like Excel magic bytes (`PK\x03\x04`) -> Excel parser
3. If detection fails, return error: `cannot detect format for <file>`

### 6.2 Streaming Ingestion
- All parsers must support **streaming** (read in chunks, not the whole file at once).
- Context cancellation: check `ctx.Done()` between every chunk.
- Column type detection: for each column, sample first 100 rows and detect:
  `string`, `integer`, `float64`, `boolean`, `date` (ISO 8601, US, EU formats).
- Error tolerance: configurable `MaxErrors` limit (default 10). Beyond that, abort.

### 6.3 Supported Formats
| Format | Extension | Parser Location | Status |
|---|---|---|---|
| CSV | `.csv` | `internal/csvparser/parser.go` | COMPLETE |
| TSV | `.tsv` | `internal/csvparser/parser.go` (tab delim) | COMPLETE |
| JSONL | `.jsonl`, `.jsonlines`, `.ndjson` | `internal/jsonlparser/parser.go` | COMPLETE |
| JSON | `.json` | `internal/jsonlparser/parser.go` (array mode) | PLANNED |
| Excel | `.xlsx` | New: `internal/xlsxparser/` | PLANNED |
| ODS | `.ods` | New: `internal/odsparser/` | PLANNED |

### 6.4 File Scanner (`internal/filescanner/`)
- Recursive scan from CWD.
- Skip: dotfiles/dotdirs, `.git/`, `node_modules/`, `__pycache__/`, `.wiki/`, `.venv/`.
- Max: 10MB per file, depth 50, 10,000 files total.
- Binary detection: null byte check + 22 known binary magic prefixes.

### 6.5 Knowledge Base Storage
- **Vector store:** ChromaDB (managed by Python RAG worker), stored in `.wiki/rag/`.
- **Metadata store:** SQLite (`.wiki/kb.sqlite`) with FTS5 full-text search.
- **Per-project:** Everything under `$CWD/.wiki/`.
- **Persistence:** Once ingested, data persists across sessions. No re-processing needed.

---

## 7. Conversational & Chat Functionality

### 7.1 Chat Interface
- Standard chat: user types message, AI responds with streaming output.
- Messages are displayed in the left panel (chat history).
- Auto-scroll to bottom on new messages.
- User can scroll up to read history (mouse wheel or PageUp/PageDown).

### 7.2 RAG-Enhanced Answers
- Every user message automatically triggers a RAG search:
  1. Embed the user's question using the embedding model
  2. Search ChromaDB for top-5 most relevant chunks
  3. Inject chunks as context into the LLM prompt
  4. LLM generates answer with source references
- If the knowledge base is empty (no ingested files), RAG is skipped silently.

### 7.3 Conversation Management
- `/clear` -- clear the current conversation (keep the thread, remove all messages).
- `/system <text>` -- set a custom system prompt for the LLM.
- `/cancel` -- cancel the current streaming response or ingestion operation.
- Conversation history is auto-saved to the project KB (`.wiki/kb.sqlite`).

### 7.4 File References (@)
- Users can reference files in chat with `@filename` syntax.
- The `@file` resolver (`internal/fileref/`) resolves and injects file content.
- Security: path traversal protection, symlink checks, binary detection.
- Max 10 refs per message.

---

## 8. Relevance Ranking & Iterative Comparison

> **Status: PLANNED. Not yet implemented.**

### 8.1 Purpose
Score every row in the ingested dataset against a user-provided research topic,
display them ranked by relevance, and allow iterative refinement with optional
threshold-based discarding.

### 8.2 Command: `/rank`

**Syntax:**
```
/rank <topic description>
```

**Behavior:**
1. User provides a research topic as a string (e.g., `/rank studies about neural networks in medical imaging`).
2. The tool reads every row from the ingested dataset.
3. For each row, the tool asks the local LLM: *"On a scale of 0.0 to 1.0, how relevant is this row to the topic: <topic>? Respond with only a number."*
4. The tool collects all scores, ranks rows in descending order.
5. Displays results in a scrollable table:

```
 Rank | Score | Row  | Column1         | Column2     | ...
------+-------+------+-----------------+-------------+-----
 1    | 0.95  | #42  | neural imaging  | 2024        | ...
 2    | 0.88  | #17  | deep learning   | 2023        | ...
...
```

**Display requirements:**
- Show top 20 rows by default.
- Show total row count and score distribution (min, max, mean).
- "Press any key to return to chat" at the bottom.
- Store the ranking in memory for subsequent `/compare` and `/discard` commands.

**Edge cases:**
- If no dataset has been ingested, show: `No dataset ingested. Use /ingest first.`
- If topic is empty, show: `Provide a topic: /rank <topic>`
- If LLM returns non-numeric response, default score to `0.0` and log a warning.
- For very large datasets (>10,000 rows), only score the first 10,000 rows and show a warning: `Dataset truncated to 10,000 rows for ranking.`

### 8.3 Command: `/compare`

**Syntax:**
```
/compare
/compare <refined topic>
```

**Behavior:**
1. If called without arguments after a `/rank`, shows the current ranking again.
2. If called with a refined topic, re-runs the ranking with the new topic.
3. Displays the new ranking alongside the previous ranking (side-by-side or sequential):

```
 Previous ranking          |  New ranking
---------------------------+-------------------------
 1. neural imaging (0.95)  |  1. MRI preprocessing (0.97)
 2. deep learning (0.88)   |  2. CNN architecture (0.92)
...
```

4. Shows score delta for each row: `+0.05`, `-0.10`, etc.

**Iterative refinement loop:**
- User can call `/compare <new topic>` repeatedly.
- Each call records the ranking as a "comparison snapshot".
- User can cycle through snapshots with `/compare --prev` and `/compare --next`.
- Maximum 10 snapshots stored per session (oldest evicted).

### 8.4 Command: `/discard`

**Syntax:**
```
/discard <threshold>
/discard 0.3
/discard --preview 0.3
```

**Behavior:**
1. `/discard 0.3` -- marks all rows with score < 0.3 for removal.
2. Before discarding, show a preview:
   - Number of rows that will be kept (score >= threshold)
   - Number of rows that will be discarded (score < threshold)
   - Summary statistics for kept vs discarded
3. Ask for confirmation: `Discard 42 rows below score 0.3? (y/N)`
4. On confirmation, remove the rows from the working dataset in memory.
5. The discarded rows are NOT deleted from the original file or KB, only from the active working set.
6. `/discard --preview 0.3` shows the preview without asking for confirmation.
7. `/discard --reset` restores all previously discarded rows.

**Edge cases:**
- Threshold must be between 0.0 and 1.0. If outside range, show: `Threshold must be between 0.0 and 1.0.`
- If no ranking has been done yet: `Run /rank first to score the dataset.`
- If all rows would be discarded: `Threshold 0.95 would discard all 100 rows. Continue? (y/N)`

### 8.5 Storage
- `/rank` results stored in Project KB table: `ranking_results`
  - Columns: `id`, `topic`, `timestamp`, `scores` (JSON blob of per-row scores)
- `/compare` snapshots stored in Project KB table: `comparison_snapshots`
  - Columns: `id`, `ranking_id`, `topic`, `timestamp`, `scores` (JSON blob)
- `/discard` history stored in Project KB table: `discard_history`
  - Columns: `id`, `threshold`, `rows_discarded`, `timestamp`

---

## 9. Data Visualization

> **Status: PLANNED. Not yet implemented.**

### 9.1 Command: `/chart`

**Syntax:**
```
/chart <type> <arguments>
/chart bar column=<column>
/chart trend column=<column>
/chart pie column=<column>
/chart scatter x=<col> y=<col>
/chart histogram column=<column> buckets=<n>
```

**Behavior:**
1. User provides chart type and column specification.
2. Tool reads the active (post-rank/post-discard) dataset.
3. Generates the chart and displays it in the terminal.
4. If `--export` flag is given, saves as PNG/SVG to the current directory.

### 9.2 Terminal Rendering (ASCII)
- Bar chart: horizontal bars using block characters (`█`, `▓`, `▒`, `░`)
- Line/trend chart: character grid with `*`, `+`, line drawing
- Pie chart: ASCII pie with labels and percentages
- All charts must include: title, axis labels (if applicable), legend (if multiple series)

**Minimum terminal width:** Charts should auto-scale to terminal width (use `lipgloss.Width`).
If terminal is too narrow (< 40 cols), show: `Terminal too narrow for chart display.`

### 9.3 File Export (PNG/SVG)
- Use Go libraries for chart rendering to PNG/SVG:
  - `github.com/wcharczuk/go-chart` OR equivalent
  - Must be a pure Go library (no CGO)
- File naming: `<column>_<chart_type>_<timestamp>.png`
- Save to CWD by default, or to path specified with `--output` flag.
- If `--output` specifies a directory, save there with auto-generated name.

### 9.4 Chart Types
| Type | Command | Data Required | Status |
|---|---|---|---|
| Bar | `/chart bar column=<col>` | Single categorical/numeric column | PLANNED |
| Line / Trend | `/chart trend column=<col>` | Ordered numeric column | PLANNED |
| Pie | `/chart pie column=<col>` | Categorical column with counts | PLANNED |
| Scatter | `/chart scatter x=<col> y=<col>` | Two numeric columns | PLANNED |
| Histogram | `/chart histogram column=<col> buckets=<n>` | Single numeric column | PLANNED |
| Box Plot | `/chart box column=<col>` | Single numeric column | PLANNED |
| Heatmap | `/chart heatmap x=<col> y=<col>` | Two categorical columns | PLANNED |

### 9.5 Error Handling
- Column not found: `Column "<name>" not found in dataset. Available columns: <list>`
- Column type mismatch (e.g., bar chart on text column is OK, but trend on non-numeric is not):
  `Column "<name>" is not numeric. Chart type <type> requires numeric data.`
- No data: `No data to chart. Ingest a dataset first.`

---

## 10. Smart Export & Output Generation

### 10.1 Command: `/export`

**Syntax:**
```
/export
/export --format xlsx
/export --format csv
/export --format json
/export --ranked          # export with relevance scores
/export --output <path>   # specify output file
```

**Behavior:**
1. If no dataset is loaded: `No dataset loaded. Use /ingest first.`
2. Default format: xlsx (Excel).
3. Detect column types and format them correctly in the output:
   - `integer` -> number column (no decimals)
   - `float64` -> number column (2 decimal places)
   - `boolean` -> checkbox or "TRUE"/"FALSE"
   - `date` -> date-formatted column
   - `string` -> text column
4. Auto-set column widths to fit content (max 50 chars).
5. Header row: bold, with column names from the dataset.
6. If `--ranked` flag is used AND a ranking exists, append a `relevance_score` column
   and sort rows descending by score.
7. If `--output` is not specified:
   - Default name: `export_<timestamp>.<ext>`
   - Save to CWD.

### 10.2 Formula Injection Protection
- Any cell value starting with `=`, `+`, `-`, `@` MUST be prefixed with `'` (apostrophe).
- This prevents Excel formula injection attacks.

### 10.3 Supported Export Formats
| Format | Extension | Notes | Status |
|---|---|---|---|
| Excel | `.xlsx` | Via excelize | COMPLETE (basic) |
| CSV | `.csv` | Via csv.Writer | COMPLETE (basic) |
| JSON | `.json` | Via json.Marshal | PLANNED |
| Markdown | `.md` | Table format | PLANNED |
| PDF | `.pdf` | Via go library | PLANNED |

---

## 11. Development Roadmap: Phase Details

### Phase 1: Foundation & LLM Integration (COMPLETE)

**Package:** `internal/ollama/`, `internal/modelmgr/`, `internal/config/`,
`internal/conversation/`, `internal/wiki/`, `internal/app/`

**Deliverables:**
- [x] Go module initialized with Bubbletea TUI
- [x] Ollama HTTP client (`internal/ollama/client.go`) with:
  - `Ping()` -- check if Ollama is reachable
  - `Chat()` -- non-streaming chat completion
  - `ChatStream()` -- streaming chat via ndjson
  - `ListModels()` -- list available models
- [x] Model manager (`internal/modelmgr/`) with active/fallback tracking
- [x] Config manager (`internal/config/`) -- YAML at `~/.config/mini-wiki/config.yaml`
- [x] Conversation thread with message truncation
- [x] Structured error types (`internal/wiki/errors.go`) -- 26 Kind constants + predicates
- [x] Ollama launcher (`internal/ollama/launcher.go`) -- auto-start/stop
- [x] Command auto-completion (Tab cycling)
- [x] TUI layout: chat panel + session panel + input bar + status bar

### Phase 2: File System & Data Ingestion (COMPLETE)

**Package:** `internal/filescanner/`, `internal/csvparser/`, `internal/jsonlparser/`,
`internal/fileref/`

**Deliverables:**
- [x] Safe directory scanner (skip dotfiles, symlinks, binaries)
- [x] File type detection (22 types)
- [x] Streaming CSV parser (with column type detection)
- [x] Streaming JSONL parser
- [x] @file reference resolver with security checks
- [x] TUI commands: `/scan`, `/files`, `/ingest`

### Phase 3: RAG Knowledge Base & Conversational Engine (COMPLETE)

**Package:** `internal/kb/`, `internal/rag/`, `internal/projectkb/`, `internal/memory/`

**Deliverables:**
- [x] SQLite knowledge base with FTS5
- [x] Python RAG worker (`rag_worker/`) with ChromaDB
- [x] Auto-RAG on every chat question
- [x] Per-project KB (`.wiki/kb.sqlite`)
- [x] Global tool memory (`~/.config/mini-wiki/memory/`)
- [x] TUI commands: `/bookmark`, `/bookmarks`, `/history`, `/skills`, `/flaws`

### Phase 4: Relevance Ranking & Iterative Comparison (COMPLETE)

**Package:** New: `internal/ranking/`

**Deliverables:**
- [ ] `/rank` command (score every row against topic)
- [ ] `/compare` command (iterative refinement with snapshots)
- [ ] `/discard` command (threshold-based removal with preview)
- [ ] Ranking results stored in Project KB
- [ ] Comparison snapshots stored in Project KB
- [ ] Discard history stored in Project KB

**Detailed spec:** See [section 8](#8-relevance-ranking--iterative-comparison).

### Phase 5: Data Visualization (COMPLETE)

**Package:** New: `internal/chart/`

**Deliverables:**
- [ ] `/chart bar` command
- [ ] `/chart trend` command
- [ ] `/chart pie` command
- [ ] `/chart scatter` command
- [ ] `/chart histogram` command
- [ ] `/chart box` command
- [ ] `/chart heatmap` command
- [ ] ASCII terminal rendering
- [ ] PNG/SVG file export

**Detailed spec:** See [section 9](#9-data-visualization).

### Phase 6: Smart Export & Multi-Format Support (COMPLETE)

**Package:** Existing: `internal/export/`, New: `internal/xlsxparser/`, `internal/odsparser/`

**Deliverables:**
- [ ] XLSX ingestion (read)
- [ ] ODS ingestion (read)
- [ ] JSON array ingestion (read)
- [ ] Smart Excel export (auto-detect types, auto-width, header formatting)
- [ ] Relevance-sorted export (`/export --ranked`)
- [ ] JSON export
- [ ] `/infer` command (auto-detect unknown format)

**Detailed spec:** See [section 10](#10-smart-export--output-generation).

### Phase 7: Remaining Features (COMPLETE)

**Deliverables:**
- [ ] `/wizard` command (interactive setup: checks OS, installs deps, pulls models)
- [ ] Enhanced export: PDF, Markdown
- [ ] OS detection + automatic dependency installer
- [ ] `/infer` -- auto-detect dataset format and suggest appropriate ingestion

---

## 12. Phase Interop Contracts

> This section defines the **exact Go types and interfaces** that PLANNED phases must
> implement and consume. An agent implementing Phase N must write code that satisfies
> the contracts that Phase N+1 will consume. This prevents refactoring when later
> phases are built.

### 12.1 Core Shared Types

These types live in a shared package (`internal/dataset/`) that ALL phases import.
No phase defines its own "row" or "column" type -- they all use these.

```go
package dataset

// ColumnKind identifies the detected type of a column.
type ColumnKind int

const (
    ColumnString  ColumnKind = iota
    ColumnInteger
    ColumnFloat
    ColumnBoolean
    ColumnDate
)

// Column describes a single column in the dataset.
type Column struct {
    Name string
    Kind ColumnKind
}

// Row represents a single row of data with column-name indexing.
type Row struct {
    Index int                      // original row number in source file
    Data  map[string]interface{}   // column name -> value
}

// Dataset is the in-memory representation that all phases operate on.
// It is produced by ingestion (Phase 2/3) and consumed by ranking (Phase 4),
// charting (Phase 5), and export (Phase 6).
type Dataset struct {
    Name        string        // source filename (without path)
    SourceFile  string        // original file path
    Columns     []Column
    Rows        []Row
    ColumnCount int
    RowCount    int
    IngestedAt  time.Time
}

// Filter returns a new Dataset containing only rows where the predicate is true.
func (d *Dataset) Filter(predicate func(Row) bool) *Dataset { ... }

// Sort returns a new Dataset sorted by the given column and direction.
func (d *Dataset) Sort(column string, descending bool) *Dataset { ... }

// Select returns a new Dataset with only the given columns.
func (d *Dataset Select(columns ...string) *Dataset { ... }
```

**Agent implementing Phase 4, 5, or 6:**
- Import `"mini-wiki/internal/dataset"`.
- Use `*dataset.Dataset` as your input type.
- Do NOT define your own Row/Column types.

### 12.2 Phase 3 -> Phase 4 Contract (Ingestion -> Ranking)

Phase 3 (COMPLETE) ingests files and stores them in ChromaDB + SQLite KB.
Phase 4 (PLANNED) needs to load row data into memory for scoring.

The bridge: Phase 4 must load data from the KB and return a `*dataset.Dataset`.

```go
// The ranking package MUST expose this loader function.
// It reads the ingested dataset from the project KB.
// This is the Phase 3 -> Phase 4 boundary.
package ranking

// LoadDataset reads the currently active ingested dataset from the project KB.
// It returns the full dataset in memory for ranking.
// projectDir is the CWD (where .wiki/ lives).
func LoadDataset(projectDir string) (*dataset.Dataset, error) { ... }
```

**Contract rules:**
- `LoadDataset` must be implemented in `internal/ranking/load.go` (Phase 4).
- Phase 3 does NOT implement this -- Phase 4 does, reading from the KB tables that Phase 3 created.
- If no dataset has been ingested, return `nil, errors.New("no dataset ingested")`.

### 12.3 Phase 4 -> Phase 5 Contract (Ranking -> Charts)

Phase 4 produces scored datasets. Phase 5 needs them as `*dataset.Dataset` with
an extra `relevance_score` column.

```go
package ranking

// RankResult is what /rank produces and /chart consumes.
type RankResult struct {
    Dataset      *dataset.Dataset    // original data + "relevance_score" column appended
    Topic        string              // the topic used for scoring
    Scores       []float64           // one score per row (same order as Dataset.Rows)
    MeanScore    float64
    MinScore     float64
    MaxScore     float64
    DiscardCount int                 // number of rows discarded (if /discard was run)
}

// Ranker performs relevance scoring against a topic.
type Ranker interface {
    // ScoreAll scores every row in the dataset against the topic.
    // It calls the LLM once per row (or in batches) and returns scores.
    ScoreAll(ctx context.Context, data *dataset.Dataset, topic string) (*RankResult, error)

    // Rerank scores against a refined topic, preserving the original scores for comparison.
    Rerank(ctx context.Context, original *RankResult, newTopic string) (*RankResult, error)
}
```

**Contract rules:**
- Phase 4 exposes `RankResult` and `Ranker` interface.
- Phase 5 imports `"mini-wiki/internal/ranking"` and calls `Ranker.ScoreAll()` or takes a `*RankResult`.
- The `relevance_score` column is appended to `Dataset.Columns` and populated in `Dataset.Rows[].Data`.

### 12.4 Phase 4 -> Phase 6 Contract (Ranking -> Export)

Phase 6 needs the ranked dataset for export with `--ranked` flag.

```go
// The export package consumes RankResult when --ranked is specified.
package export

// ExportConfig controls export behavior.
type ExportConfig struct {
    Format      string              // "xlsx", "csv", "json"
    OutputPath  string              // "" means auto-generate
    Ranked      bool                // if true, requires a *ranking.RankResult
    RankData    *ranking.RankResult // set when --ranked is passed
    ProjectDir  string              // CWD for default paths
}
```

**Contract rules:**
- Phase 6's `ExportConfig` has a `RankData` field of type `*ranking.RankResult`.
- If `Ranked` is true and `RankData` is nil, return error: `no ranking data available`.
- The export adds the `relevance_score` column and sorts descending by it.

### 12.5 Phase 2 -> Phase 6 Contract (Parsers -> Format Detection)

Phase 6 adds new parsers (XLSX, ODS). They must conform to the same interface
that the existing CSV and JSONL parsers use.

```go
package dataset

// Parser is the interface that ALL format parsers must implement.
// This includes existing parsers (CSV, JSONL) and planned ones (XLSX, ODS).
type Parser interface {
    // Parse reads from the given path and returns a Dataset.
    // It must check ctx.Done() between chunks for cancellation.
    Parse(ctx context.Context, path string) (*Dataset, error)

    // Format returns the format name this parser handles (e.g. "csv", "xlsx").
    Format() string
}

// AutoDetect reads the file header and returns the matching parser.
// If unknown, returns nil.
func AutoDetect(path string) Parser { ... }
```

**Contract rules:**
- All parsers (existing and new) implement `dataset.Parser`.
- `AutoDetect` is in `internal/dataset/detect.go` and is called by `/ingest`.
- New parsers register themselves in `AutoDetect` by file extension + magic bytes.

---

## 13. Complete Command Reference

### 12.1 Existing Commands (DO NOT MODIFY without explicit request)

| Command | Syntax | Description | Package |
|---|---|---|---|
| `/help` | `/help` | Show all available commands | `internal/app/` |
| `/model` | `/model <name>` | Switch active LLM model | `internal/modelmgr/` |
| `/models` | `/models` | List available models from Ollama | `internal/modelmgr/` |
| `/refresh` | `/refresh` | Reload model list from Ollama | `internal/modelmgr/` |
| `/clear` | `/clear` | Clear conversation history | `internal/app/` |
| `/system` | `/system <text>` | Set custom system prompt | `internal/app/` |
| `/exit` | `/exit` | Quit the application | `internal/app/` |
| `/scan` | `/scan` | Scan CWD for files | `internal/filescanner/` |
| `/files` | `/files` | List scanned files | `internal/filescanner/` |
| `/ingest` | `/ingest @<file>` | Ingest file into RAG knowledge base | `internal/rag/` |
| `/export` | `/export [--format xlsx] [--output <path>]` | Export dataset | `internal/export/` |
| `/cancel` | `/cancel` | Cancel current operation | `internal/app/` |
| `/bookmark` | `/bookmark <title>` | Save current finding as bookmark | `internal/projectkb/` |
| `/bookmarks` | `/bookmarks` | List all bookmarks | `internal/projectkb/` |
| `/history` | `/history` | Show recent query history | `internal/projectkb/` |
| `/skills` | `/skills` | List tool capabilities | `internal/memory/` |
| `/flaws` | `/flaws` | Show known issues and solutions | `internal/memory/` |
| `/kbstatus` | `/kbstatus` | Show knowledge base status | `internal/kb/` |
| `/kbquery` | `/kbquery <query>` | Query the knowledge base | `internal/kb/` |

### 12.2 Planned Commands (to be implemented)

| Command | Syntax | Description | Package | Phase |
|---|---|---|---|---|
| `/rank` | `/rank <topic>` | Rank dataset rows by relevance to topic | `internal/ranking/` | 4 |
| `/compare` | `/compare [<topic>]` | Iterative comparison with snapshots | `internal/ranking/` | 4 |
| `/discard` | `/discard <threshold>` | Discard rows below relevance threshold | `internal/ranking/` | 4 |
| `/chart` | `/chart <type> <args>` | Generate chart (bar, trend, pie, etc.) | `internal/chart/` | 5 |
| `/infer` | `/infer @<file>` | Auto-detect format and suggest ingestion | `internal/app/` | 6 |
| `/wizard` | `/wizard` | Interactive setup wizard | `internal/app/` | 7 |

### 12.3 Removed Commands (do NOT re-implement)

| Command | Reason for Removal |
|---|---|
| `/fetch` | Web fetching removed from scope |
| `/srs` | SRS generation removed from scope |

---

## 14. Project File Structure

```
mini_wiki_2.0/
  main.go                          # Entry point
  setup.sh                         # Automated dependency installer
  .venv/                           # Python virtual env (gitignored)
  rag_worker/                      # Python RAG engine (embedded in Go binary)
    main.py                        # Stdin/stdout JSON protocol dispatcher
    chunker.py                     # Recursive text splitter
    embedder.py                    # Ollama /api/embed client
    vectordb.py                    # ChromaDB wrapper
    ingester.py                    # Document ingestion (PDF, CSV, JSONL, TXT)
    querier.py                     # RAG query pipeline
    requirements.txt               # Python dependencies
  internal/
    app/
      app.go                       # Bubbletea TUI (main model, update, view)
    config/
      manager.go                   # Config persistence (YAML)
      manager_test.go
    conversation/
      types.go                     # Message & Thread data structures
      types_test.go
    csvparser/
      parser.go                    # Streaming CSV parser
      parser_test.go
    export/
      exporter.go                  # .xlsx / CSV export
    fileref/
      resolver.go                  # @file reference resolver
      resolver_test.go
    filescanner/
      scanner.go                   # Safe directory scanner
      scanner_test.go
    jsonlparser/
      parser.go                    # JSONL streaming parser
      parser_test.go
    kb/
      db.go                        # SQLite knowledge base
      schema.go
    memory/
      memory.go                    # Tool memory (skills, flaws, session)
    modelmgr/
      manager.go                   # Model lifecycle & fallback
      manager_test.go
    ollama/
      client.go                    # Ollama HTTP client
      launcher.go                  # Ollama process lifecycle
      process_darwin.go            # macOS process management
      process_linux.go             # Linux process management
      process_other.go             # Other OS process management
      transport.go                 # HTTP transport with SSRF protection
      transport_test.go
      types.go                     # Ollama API types
    projectkb/
      projectkb.go                 # Per-project SQLite KB
    rag/
      client.go                    # Go RAG client (spawns Python worker)
    ranking/                       # Phase 4: NEW
      ranker.go                    # Relevance scoring engine
      ranker_test.go
    chart/                         # Phase 5: NEW
      chart.go                     # Chart engine (terminal + file)
      chart_test.go
    xlsxparser/                    # Phase 6: NEW
      parser.go                    # Excel ingestion
    odsparser/                     # Phase 6: NEW
      parser.go                    # ODS ingestion
    webfetch/
      fetcher.go                   # DEPRECATED -- kept for reference, not used
    wiki/
      errors.go                    # Structured error types
```

### When adding new packages:
1. Create the directory under `internal/`.
2. Create the main `.go` file with the same name as the package.
3. Create a corresponding `_test.go` file.
4. Import via `"mini-wiki/internal/<packagename>"`.
5. Wire commands into `internal/app/app.go` in the `update()` function.

---

## 15. Error Handling & Logging Conventions

### 15.1 Error Types
All errors must use the `internal/wiki/errors.go` system:
- Use `wiki.New(kind, msg)` for new errors.
- Use `wiki.Wrap(kind, msg, cause)` for wrapping.
- Use `wiki.IsKind(err, kind)` for checking.
- Use the predicate functions (`wiki.IsConnection(err)`, etc.) when possible.

### 15.2 Error Display in TUI
- Errors appear in the chat panel as styled messages:
  - User errors (bad input): muted red foreground, no background.
  - System errors (Ollama down, file not found): same style, with brief hint.
- Format: `error: <message>`
- Keep messages under 120 characters. If longer, truncate with `...`.

### 15.3 Logging
- No log files. No stdout logs (they corrupt the TUI).
- Debug information: stored in `~/.config/mini-wiki/memory/flaws.yaml` via `/flaws`.
- Panic recovery: Bubbletea middleware catches panics and shows them in the TUI.

---

## 16. Testing Requirements

### 16.1 Running Tests
```bash
go test ./...          # all tests
go test -v ./...       # verbose
go test -race ./...    # race detection
```

### 16.2 Coverage Requirements
- New packages: minimum 60% coverage.
- Bug fixes: add a regression test that covers the fixed bug.
- Edge cases: test empty inputs, nil contexts, canceled contexts, timeout errors.

### 16.3 Test Patterns (mandatory for new code)
1. **Table-driven tests** for functions with multiple input/output cases.
2. **Context cancellation tests** for any function that accepts `context.Context`.
3. **Error path tests** -- test that expected errors are returned, not just success paths.
4. **No network calls** in unit tests. Use interfaces/mocks for Ollama, filesystem, etc.

### 16.4 Before Marking a Phase COMPLETE
1. Run `go build -o wiki .` -- must succeed.
2. Run `go test ./...` -- all tests must pass.
3. Run `go vet ./...` -- no warnings.
4. Update `journal.md` with what was done and any issues encountered.

---

*End of plan.md. This is the single source of truth for the mini-wiki project.*
