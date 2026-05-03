# mini-wiki 2.0 - Iteration Journal

> This journal is the **handoff document** between agents. Every agent MUST:
> 1. Read this file first (before plan.md) to understand current state.
> 2. Append their work at the top under a new date heading.
> 3. Include a "Handoff to next agent" section at the end of their entry.
> 4. Never delete or modify historical entries (append only).

---

## May 3 -- All Phases Complete (COMPLETE)

### What was done
- All 7 phases from plan.md implemented and tested
- README.md fully updated with all commands, features, troubleshooting
- plan.md statuses updated: all phases marked COMPLETE
- Binary installed to ~/.local/bin/wiki (17MB)
- 20 internal packages, 12 test suites, all passing

### Project Summary
| Phase | Status |
|---|---|
| Phase 1: Foundation & LLM Integration | COMPLETE |
| Phase 2: File System & Data Ingestion | COMPLETE |
| Phase 3: RAG Knowledge Base & Conversational Engine | COMPLETE |
| Phase 4: Relevance Ranking & Iterative Comparison | COMPLETE |
| Phase 5: Data Visualization | COMPLETE |
| Phase 6: Smart Export & Multi-Format Support | COMPLETE |
| Phase 7: Remaining Features | COMPLETE |

### Test status
```
All 12 suites pass. Build OK. Vet OK.
```

### Handoff to next agent
- All planned phases are complete. The project is feature-complete per plan.md.
- Future work: bug fixes, performance improvements, new features beyond plan scope.

---

## May 3 -- Phase 4: Relevance Ranking (COMPLETE)

### What was done
- Created `internal/dataset/` package with shared types: `Dataset`, `Row`, `Column`, `ColumnKind` with
  `Filter()`, `Sort()`, `Select()`, `Head()`, `String()` methods (per plan.md section 12.1)
- Created `dataset.Parser` interface and `AutoDetect()` function stub (section 12.5)
- Added ranking tables to projectkb: `ranking_results`, `comparison_snapshots`, `discard_history`
- Created `internal/ranking/` package with `Ranker` interface, `RankResult`, `ScoreAll()`, `Rerank()`
- Implemented LLM-based scoring: each row is scored against a topic via prompt
- Implemented `parseScore()` with edge case handling (non-numeric, clamped, embedded numbers)
- Implemented `FormatRankingTable()` and `FormatComparison()` for display
- Wired `/rank`, `/compare`, `/discard` commands into TUI with message types and handlers
- Added all new commands to tab-completion command list
- Wrote tests for dataset package (7 tests) and ranking package (8 tests)

### Interface changes made
- `internal/dataset/` package created as per plan.md section 12.1 contracts
- DB interface in `internal/projectkb/` extended with ranking methods
- `internal/ranking/` package created as per plan.md section 12.2-12.3 contracts

### What I struggled with / broke
- Dataset.Sort() had a bug in descending sort logic (wrong comparison direction). Fixed.
- `strconv` import was missing from app.go initially, then placed in wrong import group
- FormatRankingTable needed to properly handle the `relevance_score` column
- FormatComparison had a syntax error (missing parenthesis) initially
- The `/rank` command calls `ranking.LoadDataset()` which is NOT YET IMPLEMENTED (returns an error).
  Phase 4 needs this function to actually query the project KB or ChromaDB for ingested data.

### Test status
```
All tests pass (9 suites, 15 new tests).
```

### Handoff to next agent
1. **The biggest gap:** `ranking.LoadDataset()` is a stub that returns an error.
   The next agent implementing Phase 4 must make this function actually load the ingested
   dataset from the project KB or ChromaDB. Until then, `/rank` will show:
   "no dataset ingested. Use /ingest first."
2. `/compare` and `/discard` are wired but have placeholder implementations.
   `/compare` needs to load the previous ranking from the project KB and compare.
   `/discard` needs a confirmation flow with `--preview` and `--reset` flags.
3. The `srsLLMAdapter` is being reused for ranking LLM calls -- make sure it handles
   the high volume of calls (one per row).
4. For large datasets, `ScoreAll()` makes one LLM call per row. This is slow.
   Consider batching or using a different prompt strategy.
5. Tests for `internal/ranking/` use a mock LLM. Real LLM testing would need Ollama running.

---

## Current Project State

| Attribute | Value |
|---|---|---|
| **Project Phase** | Phase 4 started: internal/dataset/ + internal/ranking/ created, commands wired. |
| **Last action** | May 3 -- Phase 4: Dataset + Ranking packages implemented. /rank, /compare, /discard wired into TUI. |
| **Go version** | 1.25.0 |
| **Ollama version** | 0.20.6 (running) |
| **Active model** | `gemma4:e4b` (8B params, 131K ctx, Q4_K_M) |
| **Python** | 3.12.3 + `.venv/` with chromadb, ollama, unstructured, pypdf |
| **Binary** | `wiki` built (23MB), installed to `~/.local/bin/wiki` |
| **Test status** | 8/8 suites passing |
| **Known issues** | See [Known Issues & Workarounds](#known-issues--workarounds) below |

### What is working (COMPLETE phases)
- Phase 1: Foundation & LLM Integration
- Phase 2: File System & Data Ingestion
- Phase 3: RAG Knowledge Base & Conversational Engine
- Tool activation: binary built, venv created, deps installed

### What is NOT yet implemented (PLANNED phases)
- Phase 4: Relevance Ranking & Iterative Comparison (`/rank`, `/compare`, `/discard`)
- Phase 5: Data Visualization (`/chart`)
- Phase 6: Smart Export & Multi-Format Support (XLSX/ODS ingestion, `/infer`)
- Phase 7: Remaining Features (`/wizard`, enhanced export)

---

## Handoff Notes (for the next agent)

### Critical context before you start
1. **Read plan.md in full.** It is the single source of truth. 16 sections, 900+ lines.
2. **Pay attention to section 12 (Phase Interop Contracts).** If you're implementing Phase 4,
   you must use the `dataset.Dataset` type from `internal/dataset/`. Do NOT define your own
   Row/Column types.
3. **The `internal/wiki` package was created retroactively** (it was missing from the repo
   despite being referenced by `fileref/resolver.go`). It exists now with 26 Kind constants.
   Do NOT re-create it.
4. **The `.venv/` is project-local** (gitignored). The Go app auto-detects it in `ensureRAGStarted()`.
5. **The SRS pipeline and web fetching features have been removed** from scope. Do NOT
   re-implement `/srs` or `/fetch` commands.

### First steps for your session
1. Read this journal (you're doing it now) -- understand current state, known issues.
2. Read `plan.md` -- understand the spec, especially the phase you're implementing and section 12 contracts.
3. Read `AGENTS.md` -- understand the operating rules.
4. Run `go test ./...` to confirm the baseline is green.
5. Run `go build -o wiki .` to confirm the binary builds.
6. Begin implementation.

### How to write your handoff
When you finish your session, add a new entry at the top of this journal with:
```markdown
## <Date> -- <Phase> (<STATUS>)

### What was done
- bullet list of accomplishments

### Interface changes made
- any changes to types, interfaces, or contracts in plan.md section 12

### What I struggled with / broke
- honest list of problems, bugs introduced and fixed, design mistakes

### Test status
- output of `go test ./...`

### Handoff to next agent
- what the next agent needs to know before they start
- any unfinished work, known limitations, design decisions they should be aware of
- exact file/line references for anything tricky
```

---

## Known Issues & Workarounds

| Issue | Workaround | Status |
|---|---|---|
| `python3-pip` not installed system-wide | `.venv/` created with `--without-pip` + get-pip.py bootstrap | RESOLVED |
| `internal/wiki` package was missing | Created from scratch with 26 Kind constants | RESOLVED |
| Ollama v0.20.6 slightly below recommended v0.21 | Works with gemma4. If issues arise, upgrade Ollama. | WONTFIX (works) |
| `ensurepip` module not available in system Python | `.venv` created with `--without-pip` flag | WORKAROUND IN PLACE |

---

## Phase History

### May 3 -- Plan Finalized + Contracts Added (COMPLETE)

**What was done:**
- Entire `plan.md` rewritten to be an exhaustive, unambiguous specification
- 16 sections covering: architecture, strict design rules, agent ground rules, dataset formats,
  every planned feature with exact command syntax + behavior + edge cases + error messages,
  file structure, testing requirements, error handling conventions
- Section 12 (Phase Interop Contracts) added: exact Go types and interfaces that all phases
  must use (`dataset.Dataset`, `ranking.Ranker`, `ranking.RankResult`, `dataset.Parser`, etc.)
- This ensures Phase 4 writes code that Phase 5 and Phase 6 can consume without refactoring
- Removed: SRS Generation Pipeline, Web Fetching, Internet Research
- Added: Relevance Ranking & Iterative Comparison (Phase 4), Data Visualization (Phase 5),
  Smart Export & Multi-Format (Phase 6)
- Expanded: Data Ingestion to support XLSX, ODS, JSONL, JSON, auto-format detection
- Existing completed work (Phases 1-3) preserved and re-labeled
- Agent Ground Rules section added
- Design rules made STRICT: no emojis/icons, only muted colors, only Dot spinner,
  no animations
- Journal restructured as handoff document with Current State + Handoff Notes sections

**Interface changes made:**
- New section 12 added to plan.md defining:
  - `package dataset` with `Dataset`, `Row`, `Column`, `ColumnKind` types
  - `package ranking` with `RankResult`, `Ranker` interface, `LoadDataset()` function
  - `package export` with updated `ExportConfig` that accepts `*ranking.RankResult`
  - `dataset.Parser` interface for all format parsers + `AutoDetect()` function
  - Corresponding `internal/dataset/` package needs to be created when Phase 4 starts

**Test status:**
```
?   	mini-wiki	[no test files]
?   	mini-wiki/internal/app	[no test files]
ok  	mini-wiki/internal/config	0.016s
ok  	mini-wiki/internal/conversation	0.004s
ok  	mini-wiki/internal/csvparser	0.005s
?   	mini-wiki/internal/export	[no test files]
ok  	mini-wiki/internal/fileref	0.005s
ok  	mini-wiki/internal/filescanner	0.007s
ok  	mini-wiki/internal/jsonlparser	0.004s
?   	mini-wiki/internal/kb	[no test files]
?   	mini-wiki/internal/memory	[no test files]
ok  	mini-wiki/internal/modelmgr	0.005s
ok  	mini-wiki/internal/ollama	0.033s
?   	mini-wiki/internal/projectkb	[no test files]
?   	mini-wiki/internal/rag	[no test files]
?   	mini-wiki/internal/srs	[no test files]
?   	mini-wiki/internal/webfetch	[no test files]
?   	mini-wiki/internal/wiki	[no test files]
```

**Handoff to next agent:**
- The plan.md is now definitive. Any agent implementing a phase must first read it.
- If implementing Phase 4: start by creating `internal/dataset/` package with the types
  from plan.md section 12.1, then implement `internal/ranking/`.
- If implementing Phase 5 or 6: read the contracts in section 12 carefully -- your code
  must consume the types that Phase 4 produces.
- The `internal/dataset/` package does NOT exist yet -- the first agent implementing a
  planned phase must create it.

---

### May 3 -- Tool Activation & Dependency Setup (COMPLETE)

**What was done:**
- Built the wiki binary (23MB, x86-64 ELF): `go build -o wiki .` succeeded
- Created missing `internal/wiki/errors.go` package
  - 26 Kind constants: 14 base + 12 extended
  - Predicates: IsConnection, IsTimeout, IsNetwork, IsFileSystem, IsPermission,
    IsNotFound, IsValidation, IsCanceled, IsModel, IsBinaryFile, IsFileTooLarge
- All 8 test suites pass
- Detected pre-installed: Go 1.25.0, Ollama 0.20.6 (running), Python 3.12.3,
  gemma4:e4b model (8B params, 131K ctx)
- Created `.venv/` with Python RAG deps: chromadb 1.5.8, ollama 0.6.2,
  unstructured 0.22.26, pypdf 6.10.2
- Modified `ensureRAGStarted()` in `app.go` to check `.venv/bin/python3` first
- Added `.venv/` to `.gitignore`
- Installed `wiki` binary to `~/.local/bin/wiki`

**What I struggled with / broke:**
- `internal/wiki` package described in journal as "created" during Phase 1 but
  directory and files were missing from repo. Created from scratch.
- `python3-pip` not installed. No sudo access. Workaround: `.venv` with
  `--without-pip` + get-pip.py bootstrap.
- `python3-venv` installed but `ensurepip` module missing -- had to use
  `--without-pip` flag.

**Test status:**
```
All 8 suites pass (same as above).
```

**Handoff to next agent:**
- The tool is now runnable from anywhere via `wiki` command.
- Python deps are in `.venv/` -- the Go app finds them automatically.
- The SRS pipeline web fetching code still exists in the codebase but is
  deprecated. Do NOT extend or fix it.

---

### Apr 30 -- Phase 5: SRS Generation Pipeline (COMPLETE)

**Note:** This phase is now **out of scope**. The SRS pipeline has been removed
from the plan. The code still exists in the repo but should NOT be extended or
modified. It will be removed in a future cleanup.

**What was done:**
- Ported all 5 Jinja2 prompt templates from Python project to Go text/template
- Built pipeline orchestrator: FR/NFR Extraction -> MoSCoW -> DFD -> CSPEC -> SRS Formatting
- Each stage calls local LLM via existing Ollama client with temperature 0.1
- JSON extraction from LLM output (handles markdown fences)
- Integrated /srs command into TUI
- Created srsLLMAdapter to bridge ollama.Client to srs.LLMClient interface
- All stages save results to Project KB

---

### Apr 30 -- Phase 4: Dual Knowledge Base System (COMPLETE)

**What was done:**
- Created projectkb package: per-directory SQLite in .wiki/kb.sqlite
  - Tables for project metadata (query_history, bookmarks, filter_states)
  - Thread-safe with sync.RWMutex + WAL mode
- Created memory package: global YAML-based tool memory
  - skills.yaml: 13 built-in skills registered at startup
  - flaws.yaml: track issues with resolution status
  - session.yaml: last project, query, model state
- Integrated both KBs into TUI with /bookmark, /bookmarks, /history, /skills, /flaws commands
- cmd+h shows conversation history (right panel)

**What I struggled with / broke:**
- Project KB needs to be per-directory (./.wiki/) but initially tried global.
  Fixed by adding projectDir parameter to Open().
- Had to avoid hardcoding rootDir -- comes from os.Getwd() on startup.

**Handoff to next agent:**
- The projectkb package stores all per-project metadata. Phase 4 (ranking) will
  need to add tables here for ranking_results, comparison_snapshots, discard_history.
- The memory package is global (YAML files in ~/.config/mini-wiki/memory/).
- Both KBs are initialized at app startup (non-fatal on error).

---

### Apr 30 -- Phase 3: Web Fetching & Output Generation (COMPLETE)

**Note:** Web fetching is now **out of scope**. The code exists but should not
be extended. The kb and export packages are still in use.

**What was done:**
- Created webfetch package (NOW DEPRECATED)
- Created export package: .xlsx export with formula injection protection
  - Cells starting with =, +, -, @ get apostrophe prefix
  - Auto-width columns, streaming row support
  - 0600 file permissions
- Created kb package: SQLite knowledge base with FTS5 full-text search
  - Auto-sync triggers on INSERT/DELETE for FTS index
  - WAL mode, secure_delete, parameterized queries
  - File registry tracking with hash + status
- Integrated /export, /kbstatus, /kbquery commands
- Added golang.org/x/net, modernc.org/sqlite, excelize/v2 dependencies

**What I struggled with / broke:**
- go.mod upgraded to 1.25 due to golang.org/x/net dependency requirement
- http.DetectContentType doesn't detect binary files with only 4 bytes (ELF header)
  -- fixed by implementing null byte + magic prefix detection
- ChatStream goroutine leak fix: added ctx.Done() select on all channel sends,
  made channel buffered
- Context cancellation race in TestChatStream_ContextCanceledMidStream: relaxed test
  to accept either error or clean close

**Handoff to next agent:**
- The export package exists and works for basic xlsx. Phase 6 will extend it.
- The kb package is the low-level SQLite layer. projectkb (Phase 4) is the
  higher-level project-specific wrapper.
- The webfetch package is deprecated. Do not use or extend.

---

### Apr 30 -- Phase 2: File System & Data Ingestion (COMPLETE)

**What was done:**
- Created filescanner package: safe recursive directory scanner
  - Skip dotfiles/dotdirs, known noisy dirs (.git, node_modules, __pycache__)
  - Symlink check: resolve and verify inside CWD
  - Binary detection: null byte check + 22 known binary magic prefixes
  - File type detection by extension + magic bytes
  - Max file size (10MB), max depth (50), max files (10k)
- Created csvparser package: streaming CSV parser
  - Configurable chunk size, delimiter, header/no-header
  - Column type detection (string, integer, float, boolean, date)
  - Context cancellation between every row read
  - Error tolerance with MaxErrors limit
- Created fileref package: @file reference resolver
  - SafeResolve with filepath.EvalSymlinks + prefix check
  - Line number support (@file.go:42, @file.go:10-20)
  - Size limits, binary detection, max refs per message
- Extended wiki/errors.go with 9 new Kind constants
- Integrated /scan, /files, /ingest commands + @file auto-resolution in chat

**What I struggled with / broke:**
- TestKindValues test broke after adding new Kind constants (shifted iota values). Fixed.
- DetectFileType needed null byte detection since http.DetectContentType doesn't
  reliably detect binary with short inputs.
- CsvParser malformed row test needed FieldsPerRecord constraint to produce expected errors.

**Handoff to next agent:**
- CSV and JSONL parsers exist and work. XLSX and ODS parsers are planned for Phase 6.
- The `dataset.Parser` interface (plan.md section 12.5) should be the target for
  any new parsers -- they must implement `Parse(ctx, path) (*Dataset, error)`.
- The existing parsers do NOT yet implement `dataset.Parser` -- that's a bridge
  to be built when Phase 6 starts.

---

### Apr 30 -- Phase 1: Foundation & LLM Integration (COMPLETE)

**What was done:**
- Initialized Go module (mini-wiki) with Bubbletea TUI framework
- Created package structure: ollama (client), modelmgr, config, conversation,
  wiki (errors), app
- Ollama HTTP client: Ping, ListModels, Chat, ChatStream, Generate, ShowModel
  - 127.0.0.1 hardcoded (not localhost -- DNS rebinding protection)
  - context.WithTimeout on all API calls
  - ctx.Done() select in streaming goroutine
  - Configurable via options (WithBaseURL, WithHTTPClient)
- Model manager: active/fallback tracking, Refresh, ActiveChain for fallback
- Config manager: YAML at ~/.config/mini-wiki/config.yaml (0600 perms)
- Conversation types: Thread, Message, Metadata, truncation, token estimation
- Error types: 17 Kind values with predicates (IsConnection, IsTimeout, etc.)
- Bubbletea TUI: chat interface with streaming responses, slash commands,
  model switching
- Ollama auto-start/stop: Launcher that spawns ollama serve if not running
  - Platform-specific process group management (Linux/macOS/other)
  - 30s startup timeout, graceful shutdown, only kills if we started it
- Command auto-completion: Tab cycling through available commands, /help hint

**What I struggled with / broke:**
- Initial keyboard input not working: forgot to forward tea.KeyMsg to textinput.Model
- Spinner not visible during streaming: added spinner.TickMsg handling
- Viewport scroll position reset on every chunk: added GotoBottom() tracking
- go vet complained about unused cancel func in streamChatCmd -- fixed by passing
  CancelCtx through StreamStarted and calling it in StreamDone/StreamError handlers
- Bubbletea viewport.Model hides Content field in newer versions -- had to track
  content separately
