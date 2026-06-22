# TUI Redesign Plan: OpenCode-Style Mini-Wiki

> Generated: 2026-06-21
> Status: Planning Phase
> Based on discussion in `history/discussion_point.txt` and opencode fork analysis in `history/opencode-compat-analysis.md`

---

## 1. Vision

Mini-wiki keeps its Go + Python RAG engine but adopts the **OpenTUI + SolidJS** terminal rendering stack — the same technology powering the opencode TUI. The result is a familiar, polished UI with mini-wiki's unique dataset-analysis workflow.

**Core principle:** One binary, two runtimes. The Go binary embeds both the RAG engine and the TUI. Run `wiki`, get the full experience.

---

## 2. Target Architecture

```
                        ┌──────────────────────────┐
                        │     wiki (Go binary)      │
                        │                           │
                        │  ┌─────────────────────┐  │
                        │  │ HTTP Server (net/http)│  │
                        │  │  /api/ingest         │  │
                        │  │  /api/query          │  │
                        │  │  /api/rank           │  │
                        │  │  /api/chart          │  │
                        │  │  /api/export         │  │
                        │  │  /api/session        │  │
                        │  │  /api/events (SSE)   │  │
                        │  └──────────┬──────────┘  │
                        │             │              │
                        │  ┌──────────▼──────────┐  │
                        │  │ RAG Engine           │  │
                        │  │  internal/rag/       │  │
                        │  │  └─ JSON/stdin/stdout │  │
                        │  │  └─ Python worker    │  │
                        │  └─────────────────────┘  │
                        │                           │
                        │  ┌─────────────────────┐  │
                        │  │ //go:embed tui.bin   │  │
                        │  └─────────────────────┘  │
                        └──────────┬───────────────┘
                                   │ spawns + passes port
                        ┌──────────▼───────────────┐
                        │  wiki-tui (OpenTUI native)│
                        │                           │
                        │  ┌─────────────────────┐  │
                        │  │ Session View         │  │
                        │  │  Chat messages       │  │
                        │  │  Streaming tokens    │  │
                        │  │  Tool call display   │  │
                        │  └─────────────────────┘  │
                        │                           │
                        │  ┌─────────────────────┐  │
                        │  │ Info Panel           │  │
                        │  │  Dataset metadata    │  │
                        │  │  RAG sources         │  │
                        │  │  Ranking results     │  │
                        │  │  Chart preview       │  │
                        │  └─────────────────────┘  │
                        │                           │
                        │  ┌─────────────────────┐  │
                        │  │ Input Bar            │  │
                        │  │  Command entry       │  │
                        │  │  Autocomplete        │  │
                        │  │  Multi-line support  │  │
                        │  └─────────────────────┘  │
                        │                           │
                        │  ┌─────────────────────┐  │
                        │  │ Status Bar           │  │
                        │  │  Model info          │  │
                        │  │  RAG state           │  │
                        │  │  Progress spinners   │  │
                        │  └─────────────────────┘  │
                        └──────────────────────────┘
```

### 2.1 Process Model

```
┌─ Startup ──────────────────────────────────────────┐
│ 1. Go binary starts                                 │
│ 2. Starts HTTP server on random available port      │
│ 3. Starts Python RAG worker (via JSON/stdin/stdout) │
│ 4. Starts embedded TUI binary, passes port as arg   │
│ 5. TUI connects to Go server via HTTP/SSE           │
│ 6. Ready                                              │
└──────────────────────────────────────────────────────┘

┌─ Shutdown ─────────────────────────────────────────┐
│ 1. TUI exits (Ctrl+C or /quit)                      │
│ 2. Go server receives SIGCHLD / detects TUI exit    │
│ 3. Sends shutdown to Python RAG worker              │
│ 4. Cleans up temp dirs                              │
│ 5. Exits                                            │
└──────────────────────────────────────────────────────┘
```

### 2.2 Data Flow

```
User types question in TUI
  → SolidJS sends POST /api/query {text: "..."}
  → Go server receives, starts processing
  → Go sends progress via SSE: "Searching...", "Reading...", etc.
  → TUI renders streaming status in real-time
  → Go finishes, sends final event: {answer: "...", sources: [...]}
  → TUI renders answer as formatted markdown + source panel
```

---

## 3. Directory Structure (Post-Redesign)

```
mini-wiki/
├── main.go                         # Entry point
│   └── //go:embed wiki-tui/dist/*  # Embedded TUI binary
│
├── internal/
│   ├── server/                     # NEW: HTTP/SSE server
│   │   ├── server.go               #   net/http setup, routes
│   │   ├── sse.go                  #   SSE handler, event manager
│   │   └── handlers.go             #   Request handlers
│   │
│   ├── session/                    # NEW: Session management
│   │   ├── session.go              #   Create/resume sessions
│   │   ├── message.go              #   Message types + persistence
│   │   └── context.go              #   System context assembly
│   │
│   ├── tool/                       # NEW: Tool registry
│   │   ├── registry.go             #   Register, resolve, execute
│   │   ├── permission.go           #   Allow/ask/deny rules
│   │   └── tools/                  #   One file per tool
│   │       ├── bash.go
│   │       ├── read.go
│   │       ├── write.go
│   │       ├── ingest.go
│   │       ├── query.go
│   │       ├── rank.go
│   │       ├── chart.go
│   │       └── export.go
│   │
│   ├── app/                        # REMOVED (replaced by OpenTUI TUI)
│   │   └── [deleted]
│   │
│   ├── rag/                        # KEPT (enhanced)
│   │   ├── client.go               #   JSON/stdin/stdout protocol
│   │   ├── client_test.go
│   │   └── transport.go            #   NEW: abstract Transport for record/replay
│   │
│   ├── ranking/                    # KEPT
│   ├── dataset/                    # KEPT
│   ├── kb/                         # KEPT
│   ├── projectkb/                  # KEPT
│   ├── wiki/                       # KEPT (errors)
│   └── charts/                     # KEPT
│
├── wiki-tui/                       # NEW: OpenTUI + SolidJS TUI
│   ├── package.json
│   ├── tsconfig.json
│   ├── bunfig.toml
│   ├── src/
│   │   ├── index.tsx               # Entry point
│   │   ├── app.tsx                 # Root component
│   │   ├── client/
│   │   │   ├── api.ts              # HTTP client
│   │   │   └── sse.ts              # SSE event stream
│   │   ├── routes/
│   │   │   ├── session/
│   │   │   │   ├── index.tsx       # Chat view
│   │   │   │   ├── message.tsx     # Message component
│   │   │   │   └── input.tsx       # Input prompt
│   │   │   └── home.tsx            # Welcome/landing screen
│   │   ├── components/
│   │   │   ├── markdown.tsx        # Markdown renderer
│   │   │   ├── sources.tsx         # Source panel
│   │   │   ├── status.tsx          # Status bar
│   │   │   ├── spinner.tsx         # Loading spinner
│   │   │   └── dataset.tsx         # Dataset info card
│   │   ├── context/
│   │   │   ├── session.tsx         # Session state
│   │   │   └── theme.tsx           # Theming
│   │   └── styles/
│   │       └── theme.ts            # Color palette, spacing
│   └── build.ts                    # Build script
│
├── rag_worker/                     # KEPT (enhanced)
│   ├── main.py
│   ├── vectordb.py
│   ├── embedder.py
│   ├── ingester.py                 # MODIFIED: LangChain loaders
│   ├── chunker.py                  # KEPT or replaced by LangChain
│   ├── querier.py                  # MODIFIED: +query rewriting +reranker
│   ├── deep_reader.py              # KEPT
│   ├── agentic_query.py            # KEPT
│   ├── agentic_ranker.py           # KEPT
│   ├── ocr.py                      # NEW: Tesseract wrapper
│   ├── reranker.py                 # NEW: MiniLM cross-encoder
│   └── requirements.txt            # UPDATED
│
├── history/                        # Planning docs
│   ├── opencode-compat-analysis.md
│   ├── discussion_point.txt
│   └── tui-redesign-plan.md        # (this file)
│
└── .wiki/                          # State dir (kept)
```

---

## 4. OpenTUI + SolidJS — What You're Adopting

### 4.1 The Stack (from the opencode fork)

| Layer | Library | Purpose |
|---|---|---|
| Terminal rendering | `@opentui/core` (Zig) | Raw terminal I/O, layout engine, mouse/keyboard |
| UI framework | `solid-js` (1.9.x) | Reactive components, signals, JSX |
| OpenTUI bindings | `@opentui/solid` | JSX intrinsic elements: `<box>`, `<text>`, `<scrollbox>` |
| Keybinding system | `@opentui/keymap` | Vim-like modes, composable bindings |
| Markdown rendering | Built into `@opentui/core` | `<markdown>`, `<code>`, `<diff>` components |

### 4.2 Key Components to Build

```
┌─ App Layout ───────────────────────────────────────────────┐
│ ┌─ Header ──────────────────────────────────────────────┐  │
│ │  [logo] mini-wiki  |  Model: llama3.1:8b  |  RAG: on │  │
│ └──────────────────────────────────────────────────────┘  │
│ ┌─ Main Content ─────────────────────────────────────────┐ │
│ │ ┌── Chat (70%) ─────────────────┐ ┌── Info (30%) ───┐ │ │
│ │ │  [User message]              │ │  Dataset:        │ │ │
│ │ │  [AI response with markdown] │ │  - sales_2024    │ │ │
│ │ │  [Tool: /rank result]        │ │  - 12 columns    │ │ │
│ │ │  [Tool: /chart output]       │ │  - 5,000 rows    │ │ │
│ │ │                              │ │                  │ │ │
│ │ │  [User message]              │ │  RAG Sources:    │ │ │
│ │ │  [Streaming AI response...]  │ │  - report.pdf    │ │ │
│ │ │                              │ │  - notes.txt     │ │ │
│ │ │                              │ │                  │ │ │
│ │ │                              │ │  Rank Results:   │ │ │
│ │ └──────────────────────────────┘ │  - #1 score 0.92 │ │ │
│ │                                  │  - #2 score 0.78 │ │ │
│ │                                  └──────────────────┘ │ │
│ └──────────────────────────────────────────────────────┘  │
│ ┌─ Input ───────────────────────────────────────────────┐  │
│ │ > What were the Q3 sales by region?                    │  │
│ │  [auto-expanding, Alt+Enter for newline]              │  │
│ └──────────────────────────────────────────────────────┘  │
│ ┌─ Status Bar ──────────────────────────────────────────┐  │
│ │  Ctrl+C quit  |  Tab focus  |  RAG active  |  Idle   │  │
│ └──────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────┘
```

**Minimal terminal: 60x16** (kept from current spec)
**Breakpoints:** <80, 80-119, >=120 cols (kept from current spec)
**Responsive:** At <80 cols, info panel collapses into a toggleable overlay

### 4.3 Component Mapping: Bubbletea → OpenTUI/SolidJS

| Current (Bubbletea) | New (OpenTUI + SolidJS) |
|---|---|
| `tea.Model` + `Update()` | SolidJS signals + components |
| `spinner.Model` | `<Spinner />` via `opentui-spinner` |
| `viewport.Model` | `<scrollbox>` virtual scrolling |
| `textarea.Model` | `<textarea>` component |
| `lipgloss.Style` | CSS-like props on `<box>`, `<text>` |
| `tea.Msg` pattern | HTTP/SSE event stream |
| `tea.WithMouseCellMotion` | Built-in OpenTUI mouse support |
| `renderInfoPanel()` | `<Sidebar>` component |

### 4.4 Theming

Colors keep mini-wiki's current muted palette (grays, muted blues/greens) — the same design rule from AGENTS.md — but expressed as SolidJS theme context:

```typescript
// wiki-tui/src/styles/theme.ts
export const theme = {
  bg: "#1a1a2e",
  bgLighter: "#16213e",
  accent: "#4a6fa5",
  accentLight: "#6b8fc9",
  text: "#e0e0e0",
  textMuted: "#8899aa",
  border: "#2a2a4e",
  success: "#4caf50",
  error: "#e53935",
  warning: "#ff9800",
}
```

---

## 5. RAG Enhancements (Parallel Track)

These are the RAG upgrades agreed upon in the discussion. They happen alongside or before the TUI redesign since they're mostly Python-side changes.

### 5.1 Dual Query Rewriting

**File:** `rag_worker/querier.py` (modified)

Flow:
```
User query → llama3.1:8b rewrites for retrieval → search with rewritten query
                                                          ↓
LLM receives: [original query + rewritten query + retrieved chunks]
```

The LLM sees both queries so it stays grounded in user intent while benefiting from better retrieval.

### 5.2 Built-in Reranker (MiniLM)

**File:** `rag_worker/reranker.py` (new)

```python
from sentence_transformers import CrossEncoder

class Reranker:
    def __init__(self):
        # 22M params, ~80MB, runs on CPU in ~50ms
        self.model = CrossEncoder("cross-encoder/ms-marco-MiniLM-L-6-v2")

    def rerank(self, query: str, chunks: list[dict], top_k: int = 5) -> list[dict]:
        pairs = [(query, chunk["text"]) for chunk in chunks]
        scores = self.model.predict(pairs)
        for i, chunk in enumerate(chunks):
            chunk["rerank_score"] = float(scores[i])
        chunks.sort(key=lambda c: c["rerank_score"], reverse=True)
        return chunks[:top_k]
```

Integrated into `querier.py`: retrieve 10, rerank to 5, then pass to LLM.

### 5.3 Tesseract OCR

**File:** `rag_worker/ocr.py` (new)

Fallback in `ingester.py`: if `unstructured` returns near-empty text, try OCR.

Installation: `apt install tesseract-ocr`, `pip install pytesseract`

### 5.4 LangChain for Document Loaders

**Dependency:** Add `langchain`, `langchain-community`, `langchain-ollama` to `requirements.txt`

**Scope:** Replace `unstructured`-based file loading in `ingester.py` with LangChain document loaders for PDFs, HTML, DOCX, websites, etc.

**Not replaced:** Chunking (keep custom), embedding (keep custom via LangChain `OllamaEmbeddings` or keep direct), query pipeline (keep custom — deep reading, agentic query, ranking are all custom).

---

## 6. Implementation Phases

### Phase 1: Foundation (Go Side)
*Estimated: 1-2 sessions*

1. Add `internal/server/` package with HTTP router + SSE handler
2. Create `/api/ingest`, `/api/query`, `/api/rank`, `/api/chart`, `/api/export`, `/api/status` endpoints
3. Wire existing RAG operations through HTTP handlers
4. Add `internal/session/` for basic session management (conversation history, state tracking)
5. Write tests for the HTTP handlers

**Go deps to add:** (minimal — standard library `net/http` is sufficient for the MVP)

### Phase 2: TUI Scaffolding (OpenTUI + SolidJS)
*Estimated: 2-3 sessions*

1. Set up `wiki-tui/` directory with `package.json`, `tsconfig.json`, build script
2. Create entry point (`index.tsx`) with `createCliRenderer()` + `render()`
3. Build the 4-panel layout: Chat, Info Panel, Input, Status Bar
4. Create HTTP client module + SSE stream consumer
5. Connect to Go server (hardcoded port for now)
6. Implement basic chat flow: type message → POST → receive SSE events → display
7. Test the full round-trip: TUI → Go → Python worker → Go → TUI

### Phase 3: TUI Polish
*Estimated: 2-3 sessions*

1. Markdown rendering for AI responses (`<markdown>` component)
2. Source panel display (file names, scores, chunk previews)
3. Streaming token display (incremental text updates via SSE deltas)
4. Input autocomplete for commands (/ingest, /rank, /chart, /export)
5. Status bar with model name, RAG state, spinner during operations
6. Dataset info card (schema preview, row count, column types)
7. Command palette / help view

### Phase 4: Embedding & One-Binary
*Estimated: 1 session*

1. Compile TUI to native binary: `bun build --compile ./wiki-tui/src/index.ts`
2. Embed binary in Go: `//go:embed wiki-tui/dist/wiki-tui-*`
3. Go starts TUI on a random port, passes port as CLI arg
4. Clean shutdown: SIGCHLD handler, graceful worker cleanup
5. Test: `go build -o wiki . && ./wiki` launches everything

### Phase 5: RAG Enhancements
*Estimated: 1-2 sessions (can overlap with Phase 2/3)*

1. Add `rag_worker/reranker.py` (MiniLM cross-encoder)
2. Modify `querier.py` for dual query rewriting
3. Add `rag_worker/ocr.py` (Tesseract fallback)
4. Add LangChain document loaders to `ingester.py`
5. Update `requirements.txt`
6. Update `setup.sh` to install tesseract-ocr
7. Test all enhancements

### Phase 6: Polish & Hardening
*Estimated: 1 session*

1. Responsive layout (info panel collapses at <80 cols)
2. Keyboard shortcuts (Ctrl+C quit, Tab focus, Alt+Enter newline)
3. Mouse support (dynamic toggle: click selects, typing scrolls)
4. Theming / color customization
5. Performance optimization (virtual scrolling for long chats)
6. Error handling (connection lost, worker crash recovery)

---

## 7. Key Technical Decisions

### 7.1 HTTP Port
The Go server binds to a **random available port** (port 0) and passes it to the TUI process as a CLI argument. No port conflicts, no hardcoding.

### 7.2 SSE Protocol
Events flow from Go → TUI as JSON over SSE:

```
event: status
data: {"type": "status", "state": "searching"}

event: progress
data: {"type": "progress", "message": "Reading chunk 3/5..."}

event: token
data: {"type": "token", "delta": "The answer is..."}

event: done
data: {"type": "done", "answer": "...", "sources": [...]}

event: error
data: {"type": "error", "message": "..."}
```

### 7.3 Session Persistence
- Session state (conversation history, RAG context) lives in Go memory
- Optional SQLite persistence via `internal/session/` for resume across restarts
- The TUI is stateless — if it crashes, the Go server continues, TUI reconnects

### 7.4 Build Tooling
- TUI built with `bun build --compile` (produces native binary for the target OS/arch)
- Go embeds the appropriate platform binary
- Add `build-tui.sh` script that: `cd wiki-tui && bun install && bun run build`
- Add `build-tui` target to `main.go` build tags (optional, default includes pre-built binary)

### 7.5 Dependencies (Go)
Keep Go dependencies minimal. The HTTP server uses only stdlib `net/http` initially. No need for chi/gin/echo — the API surface is small.

### 7.6 Dependencies (TUI)

Current opencode fork versions (June 2026):
- `solid-js` ^1.9.10
- `@opentui/core` ^0.4.1
- `@opentui/solid` ^0.4.1
- `@opentui/keymap` ^0.4.1
- `typescript` ^5.8.2
- `bun` ^1.3.x

These will be our starting versions.

---

## 8. Open Questions

1. **TUI update frequency during streaming** — SSE supports delta events for smooth token-by-token display. The TUI should batch updates at ~60fps (matching OpenTUI's `targetFps: 60`).

2. **Long-running operations** — Ingestion can take minutes. The TUI needs a persistent progress panel (or status bar) that shows chunk counts, file names, and estimated time remaining without blocking chat.

3. **/chart output** — Charts are currently rendered as text-based terminal graphics (ASCII bars, etc.). With OpenTUI we could render richer inline visuals. Worth exploring after MVP.

4. **Plugin/skill integration** — The opencode fork has a sophisticated plugin SDK. After the TUI redesign stabilizes, adding plugin support for custom tools/sources is a natural next step.

---

## 9. Success Criteria

The redesign is ready to ship when:

1. `go build -o wiki .` produces one binary
2. `./wiki` launches the OpenTUI TUI with the Go backend
3. All existing commands work: `/ingest`, `/query`, `/rank`, `/chart`, `/export`
4. Streaming responses display incrementally (not all at once)
5. Chat history scrolls with virtual scrolling
6. Info panel shows dataset metadata and RAG sources
7. Reranker runs automatically on every RAG query
8. Dual query rewriting works
9. Tesseract OCR processes scanned documents
10. All tests pass (`go test ./...` + `bun test` in `wiki-tui/`)
11. No emojis in the UI (per AGENTS.md rules)
