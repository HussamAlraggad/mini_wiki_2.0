# OpenCode Source Fork → Mini-Wiki Compatibility Analysis

> Generated: 2026-06-21
> Source: `/home/halraggad/my_work/coding_stuff/opencode_source_fork`
> Purpose: Identify patterns, architecture, and tooling from the opencode fork that could guide a major mini-wiki revamp.

---

## 1. The Fork at a Glance

**OpenCode** is an open-source AI coding agent. The forked repo at `opencode_source_fork` is a monorepo with **25 packages**, written entirely in **TypeScript**, running on **Bun** (JavaScript runtime + package manager). Its TUI uses **SolidJS + OpenTUI** (a Zig-native terminal framework, not Bubbletea). The backend uses **Effect-TS** (functional effect system) for all async operations, dependency injection, and error handling.

### Key Stats

| Metric | Value |
|---|---|
| Monorepo packages | ~25 |
| Language | TypeScript 5.8 |
| Runtime | Bun 1.3.x |
| Package manager | Bun (workspaces) |
| Monorepo tool | Turborepo 2.8 |
| Terminal UI | OpenTUI (Zig) + SolidJS |
| CLI framework | Effect-TS CLI |
| LLM SDK | Vercel AI SDK (`ai` v6) |
| Testing | `bun test` + Playwright |
| Linter | Oxlint |
| Git branch | `dev` (default) |

---

## 2. Core Stack Divergence

| Layer | Mini-Wiki (current) | OpenCode Fork | Compatibility Notes |
|---|---|---|---|
| **Language** | Go 1.25 | TypeScript 5.8 (Bun) | **Incompatible** — different lang ecosystems |
| **TUI Framework** | Bubbletea + Bubbles + Lipgloss | OpenTUI (Zig) + SolidJS | **Incompatible** — different rendering engines |
| **Async / Effects** | Go goroutines + channels | Effect-TS (`effect` package) | **Conceptual match** — goroutines ~ Effect fibers |
| **Package Manager** | Go modules (`go.mod`) | Bun workspaces + Turborepo | N/A (different languages) |
| **LLM Integration** | Direct Ollama HTTP + Python subprocess | Vercel AI SDK + 18+ `@ai-sdk/*` providers | Mini-wiki is more local-first, no API keys |
| **Config** | Go structs + ad-hoc JSON | `opencode.json` with layered loading + Schema validation | Mini-wiki could borrow the **config layering pattern** |
| **Testing** | `go test` / `go vet` | `bun test` + Effect HTTP recorder + Playwright E2E | Both solid; no cross-pollination needed |
| **CLI Framework** | Bubbletea (implicit) | Effect-TS CLI (explicit command tree) | Mini-wiki's is simpler and sufficient |
| **IPC** | JSON-over-stdin/stdout (Python subprocess) | HTTP/SSE (daemon server ↔ TUI client) | Mini-wiki approach is lighter for local use |

---

## 3. Architecture Patterns Mini-Wiki Can Borrow

The languages differ, but these **architecture patterns are language-agnostic** and could guide the revamp:

### 3.1 Two-Process Architecture (Server ↔ Client)

**OpenCode pattern:** A background daemon server (`packages/server/`) handles all business logic. The TUI connects via HTTP/SSE. A headless API is always available.

**Mini-wiki currently:** Everything runs in a single process — Bubbletea loop, Python subprocess, SQLite, all in one.

**Opportunity:** Splitting into a lightweight daemon + client would enable:
- Headless / API mode (other tools query the daemon)
- Multi-client (TUI + Web UI + CLI commands)
- Session persistence across restarts
- Background operations without blocking the TUI

**Trade-off:** More complexity, IPC overhead. Worth it only if multi-client is desired.

### 3.2 Layered Configuration Loading

**OpenCode pattern** (`packages/core/src/config.ts`):
1. Searches `config.json`, `opencode.json`, `opencode.jsonc`
2. Loads in order: global config → project-level (walk up from CWD) → `.opencode/` directory
3. Each layer merges over the last (higher priority wins)
4. Validated at runtime via `Schema.Codec` (Effect-TS Schema)
5. Environment variable substitution supported

**Mini-wiki currently:** Config is scattered — some from CLI flags, some from `.wiki/` state, some from Go structs.

**Opportunity:** Unified layered config:
```
~/.config/mini-wiki/config.json   (global defaults)
$CWD/.wiki/config.json            (project overrides)
CLI flags                         (runtime overrides)
```

**Recommendation:** Do this. It's low effort, high impact for UX.

### 3.3 Tool Registry with Permissions

**OpenCode pattern** (`packages/core/src/tool/`):
- `Tool.make()` factory — each tool defines typed input/output, description, permission key
- `ToolRegistry` materializes tools, filters by agent permissions
- Permission system: `allow / ask / deny` with wildcard rules
- Tools execute via `Effect` with scope management

**Mini-wiki currently:** Tool-like operations (ingest, rank, chart, export) are handled as Bubbletea messages and ad-hoc commands. No permission system, no typed interface.

**Opportunity:** A `internal/tool/` registry:
```go
type Tool struct {
    Name        string
    Description string
    Input       schema.Schema  // typed input validation
    Execute     func(ctx Context, input any) (Output, error)
    Permission  string         // permission key
}
```

This would enable:
- Safe MCP integration (external tools get same interface)
- Permission rules per agent (plan vs build)
- Consistent error handling and logging
- Plugin system hook points

### 3.4 Plugin / Skills System

**OpenCode pattern** (`packages/plugin/src/index.ts`):
- Plugin SDK with typed hooks: `tool`, `auth`, `provider`, `chat.message`, `chat.params`, `permission.ask`, `shell.env`, etc.
- Each hook is a function that returns a modified result
- Hooks are composed and executed in order

**Mini-wiki currently:** Skills are plain markdown files with frontmatter. There's no plugin system.

**Opportunity:** A hook-based plugin system for Go:
```go
type Plugin interface {
    Name() string
    Hooks() PluginHooks
}
type PluginHooks struct {
    ToolExecute   func(ctx, tool, input)  // before/after tool runs
    ConfigLoad    func(config) config      // modify config on load
    LLMRequest    func(req) req            // modify LLM request
    LLMResponse   func(resp) resp          // modify LLM response
}
```

This opens the door to third-party extensions without forking.

### 3.5 MCP (Model Context Protocol) Client

**OpenCode pattern** (`packages/opencode/src/mcp/`):
- Full MCP client supporting both `local` (stdio) and `remote` (HTTP + OAuth) transports
- Auto-discovers tools/resources from connected MCP servers
- Converts MCP tool definitions to internal tool format
- Handles OAuth flow for remote servers
- Cleans up child processes on shutdown

**Mini-wiki currently:** The only subprocess is a custom Python RAG worker. No standard protocol.

**Opportunity:** An `internal/mcp/` client would let mini-wiki connect to any MCP server:
- External vector databases (Pinecone, Weaviate)
- Web search tools
- Code execution sandboxes
- Any MCP-compatible data source

The existing Python RAG worker could also be wrapped as an MCP server, making it a standard component.

### 3.6 Session Management with Context Epochs

**OpenCode pattern** (`packages/core/src/session/`):
- `SessionV2` — durable conversation history stored in SQLite
- **Context Epoch** — the span during which one effective agent's system context is immutable
- **System Context** — structured facts presented to the model as initial instructions
- **Context Sources** — independently observed typed values with loaders and renderers
- **Context Snapshot** — tracks what was last sent to the model
- **Mid-Conversation System Messages** — update the model when context changes mid-session
- Compaction starts a new epoch, preserving conversation but refreshing context

**Mini-wiki currently:** Conversations are ephemeral (in-memory Bubbletea viewport). No persistence, no context management.

**Opportunity:** A `internal/session/` package for durable, structured sessions:
```go
type Session struct {
    ID          string
    Messages    []Message
    Context     SystemContext       // current RAG context, model config, etc.
    Epoch       int                 // context epoch counter
    CreatedAt   time.Time
    LastActive  time.Time
}
```

This enables:
- Session persistence across restarts
- Conversation history browsing
- Context-aware compaction (trim old messages, retain RAG context)
- Better token management

### 3.7 Deterministic Testing via Traffic Recording

**OpenCode pattern** (`packages/http-recorder/`):
- Records HTTP traffic as JSON files
- Replays recorded responses in tests
- Enables deterministic testing of LLM integrations without live API calls

**Mini-wiki currently:** Tests that exercise the RAG pipeline need Ollama running. This makes CI brittle.

**Opportunity:** Record Ollama API responses and replay them in tests. The `internal/rag/client.go` already abstracts the RPC layer — add a recorder mode.

```go
type Transport interface {
    Send(request) (response, error)
}
// Real: spawns Python, pipes stdin/stdout
// Record: records all traffic to JSON
// Replay: reads recorded traffic, no subprocess needed
```

---

## 4. What Mini-Wiki Should Keep (Strengths)

| Strength | Why |
|---|---|
| **Single binary** | `go build` produces one static binary. No runtime dependency (Bun/Node). |
| **Local-first** | No cloud APIs, no telemetry, no API keys. Fully offline. |
| **Python RAG ecosystem** | ChromaDB, Pandas, NumPy, unstructured — mature, well-suited for tabular RAG. |
| **Fast startup** | Go binary launches instantly vs. Bun cold start (~200ms+). |
| **IPC simplicity** | JSON-over-stdin/stdout is simpler than HTTP/SSE for a local subprocess. |
| **Strong typing** | Go's compile-time safety catches errors the TS ecosystem needs runtime tests for. |
| **Embedded assets** | `//go:embed` bundles Python worker into the binary — elegant deployment. |

---

## 5. Recommended Hybrid Architecture (Revamp Target)

```
mini-wiki/
├── main.go                      # Entry point (stays Go, embeds RAG worker)
│
├── internal/
│   ├── app/                     # Bubbletea TUI (keep, modernize layout)
│   ├── config/                  # NEW: Layered config loader
│   │   ├── loader.go            #   global → project → CLI flags
│   │   └── schema.go            #   Validated schema (from opencode's pattern)
│   │
│   ├── session/                 # NEW: Durable session management
│   │   ├── session.go           #   Persist conversation history
│   │   └── context.go           #   System context assembly + epochs
│   │
│   ├── tool/                    # NEW: Tool registry
│   │   ├── registry.go          #   Register, materialize, settle tools
│   │   ├── permission.go        #   Allow/ask/deny permission rules
│   │   └── tools/               #   One file per built-in tool
│   │       ├── bash.go
│   │       ├── read.go
│   │       ├── write.go
│   │       ├── edit.go
│   │       ├── glob.go
│   │       ├── grep.go
│   │       ├── webfetch.go
│   │       ├── ingest.go
│   │       ├── rank.go
│   │       ├── chart.go
│   │       └── export.go
│   │
│   ├── plugin/                  # NEW: Plugin SDK
│   │   └── hooks.go             #   Hook interface + registry
│   │
│   ├── mcp/                     # NEW: MCP client
│   │   └── client.go            #   Connect to stdio/HTTP MCP servers
│   │
│   ├── rag/                     # RAG client (keep, enhance)
│   │   ├── client.go            #   Keep JSON/stdin/stdout protocol
│   │   ├── client_test.go
│   │   └── transport.go         #   NEW: abstract Transport for record/replay
│   │
│   ├── ranking/                 # Ranking (keep)
│   ├── dataset/                 # Shared data types (keep)
│   ├── kb/                      # SQLite KB (keep)
│   ├── projectkb/               # Project state (keep)
│   ├── wiki/                    # Shared errors (keep)
│   └── charts/                  # Chart generation (keep)
│
├── rag_worker/                  # Python RAG engine (keep)
│   ├── main.py
│   ├── vectordb.py              # ChromaDB (keep)
│   ├── embedder.py              # Ollama embed (keep)
│   ├── ingester.py              # Ingestion (keep, add MCP wrapping)
│   ├── chunker.py               # Chunking (keep)
│   ├── querier.py               # RAG query (keep)
│   ├── deep_reader.py           # Deep RAG (keep)
│   ├── agentic_query.py         # Agentic Query (keep)
│   └── agentic_ranker.py        # Agentic Ranking (keep)
│
├── history/                     # Planning docs
│   ├── opencode-compat-analysis.md
│   └── ... 
│
├── plugins/                     # NEW: User-installable plugins dir
│
└── .wiki/                       # Project state dir (keep)
    ├── rag/                     # ChromaDB vector store
    ├── kb.sqlite                # Knowledge base
    └── config.json              # NEW: Per-project config
```

---

## 6. Priority Matrix

| Priority | Feature | Effort | Impact | Depends On |
|---|---|---|---|---|
| **P1** | Layered config loading | Low | Cleaner UX, extensibility | Nothing |
| **P2** | Tool registry + permissions | Medium | Safety, extensibility, MCP-ready | P1 (config) |
| **P3** | Durable session management | Medium | Persistent conversations, compaction | P1 (config for paths) |
| **P4** | MCP client | High | Connect to any MCP server | P2 (tool registry) |
| **P5** | Plugin SDK | High | Third-party extensions | P2 (hooks into tools) |
| **P6** | Server/Client split | Very High | Web UI, headless, multi-client | P3 + P4 |
| **P7** | Transport record/replay for tests | Low | Deterministic RAG tests | Nothing |

---

## 7. Key OpenCode Files to Reference

These are the most relevant files from the fork for reference during implementation:

| Pattern | Key File(s) in Fork |
|---|---|
| Config layering | `packages/core/src/config.ts` |
| Tool registry | `packages/core/src/tool/` (tool.ts, registry.ts, builtins.ts) |
| Permissions | `packages/core/src/permission.ts`, `packages/core/src/permission/schema.ts` |
| Plugin hooks | `packages/core/src/plugin.ts`, `packages/plugin/src/index.ts` |
| MCP client | `packages/opencode/src/mcp/index.ts` |
| Session/Context | `packages/core/src/session.ts`, `packages/core/src/session/execution/` |
| Context Sources | `packages/core/src/system-context/` |
| AI SDK bridge | `packages/opencode/src/session/llm/ai-sdk.ts` |
| Agent definitions | `packages/opencode/src/agent/agent.ts` |
| CLI framework | `packages/cli/src/framework/spec.ts`, `packages/cli/src/commands/commands.ts` |
| Configuration schema | `packages/core/src/config/provider.ts`, `packages/core/src/config/agent.ts` |

---

## 8. Key Differences in Philosophy

| Dimension | OpenCode | Mini-Wiki |
|---|---|---|
| **Target user** | Developer, coding agent | Researcher, data analyst |
| **Primary data** | Code files | Tabular datasets |
| **AI models** | Cloud + local (many providers) | Local-only (Ollama) |
| **Deployment** | npm package, desktop app | Single Go binary |
| **Extensibility** | Plugin SDK + MCP | Scripts + skills |
| **Networking** | Cloud-connected (telemetry, auth, web) | Fully offline |
| **Concurrency model** | Effect-TS (structured concurrency) | Goroutines (CSP) |

The revamp should respect mini-wiki's identity: **local-first, tabular-focused, single-binary, researcher-oriented**. Not every opencode pattern applies — choose selectively.
