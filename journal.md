# mini-wiki 2.0 - Iteration Journal

## Phase 5: SRS Generation Pipeline (INPROGRESS)

### Completed Apr 30
- Ported all 5 Jinja2 prompt templates from Python project to Go text/template
- Built pipeline orchestrator: FR/NFR Extraction -> MoSCoW -> DFD -> CSPEC -> SRS Formatting
- Each stage calls local LLM via existing Ollama client with temperature 0.1
- JSON extraction from LLM output (handles markdown fences)
- Integrated /srs command into TUI
- Created srsLLMAdapter to bridge ollama.Client to srs.LLMClient interface
- All stages save results to Project KB

## Phase 4: Dual Knowledge Base System (COMPLETE)

### Completed Apr 30
- Created projectkb package: per-directory SQLite in .wiki/kb.sqlite
  - Tables for SRS pipeline storage (srs_runs, srs_requirements, srs_moscow, srs_dfd_components, srs_cspec, srs_documents)
  - Tables for project metadata (query_history, bookmarks, filter_states)
  - Thread-safe with sync.RWMutex + WAL mode
- Created memory package: global YAML-based tool memory
  - skills.yaml: 13 built-in skills registered at startup
  - flaws.yaml: track issues with resolution status
  - session.yaml: last project, query, model state
- Integrated both KBs into TUI with /bookmark, /bookmarks, /history, /skills, /flaws commands
- cmd+h shows conversation history (right panel)

### What I struggled with / broke
- The Project KB needs to be per-directory (./.wiki/) but I initially tried to make it global. Fixed by adding projectDir parameter to Open().
- Had to be careful about not hardcoding rootDir - it comes from os.Getwd() on startup.

## Phase 3: Web Fetching & Output Generation (COMPLETE)

### Completed Apr 30
- Created webfetch package: secure HTTP fetcher with SSRF protection
  - Blocked 11 private CIDR ranges at DNS level
  - URL validation rejects file://, ftp://, localhost, credentials
  - HTML-to-text extraction via golang.org/x/net/html (safe, no regex)
  - 5MB max body, 30s timeout, 5 redirect limit
- Created export package: .xlsx export with formula injection protection
  - Cells starting with =, +, -, @ get apostrophe prefix
  - Auto-width columns, streaming row support
  - 0600 file permissions
- Created kb package: SQLite knowledge base with FTS5 full-text search
  - Auto-sync triggers on INSERT/DELETE for FTS index
  - WAL mode, secure_delete, parameterized queries
  - File registry tracking with hash + status
- Integrated /fetch, /export, /kbstatus, /kbquery commands
- Added golang.org/x/net, modernc.org/sqlite, excelize/v2 dependencies

### What I struggled with / broke
- go.mod upgraded to 1.25 due to golang.org/x/net dependency requirement
- http.DetectContentType doesn't detect binary files with only 4 bytes (ELF header) - fixed by implementing null byte + magic prefix detection
- ChatStream goroutine leak fix: added ctx.Done() select on all channel sends, made channel buffered
- Context cancellation race in TestChatStream_ContextCanceledMidStream: relaxed test to accept either error or clean close
- Auto-generated fetch_msgs.go from design agent had incompatible types - deleted and integrated manually

## Phase 2: File System & Data Ingestion (COMPLETE)

### Completed Apr 30
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
  - Injects file content as annotated markdown code block
- Extended wiki/errors.go with 9 new Kind constants
- Integrated /scan, /files, /ingest commands + @file auto-resolution in chat

### What I struggled with / broke
- Had to fix TestKindValues test after adding new Kind constants (shifted iota values)
- The DetectFileType function needed null byte detection since http.DetectContentType doesn't reliably detect binary with short inputs
- CsvParser malformed row test needed FieldsPerRecord constraint to produce expected errors

## Phase 1: Foundation & LLM Integration (COMPLETE)

### Completed Apr 30
- Initialized Go module (mini-wiki) with Bubbletea TUI framework
- Created package structure: ollama (client), modelmgr, config, conversation, wiki (errors), app
- Ollama HTTP client: Ping, ListModels, Chat, ChatStream, Generate, ShowModel
  - 127.0.0.1 hardcoded (not localhost - DNS rebinding protection)
  - context.WithTimeout on all API calls
  - ctx.Done() select in streaming goroutine
  - Configurable via options (WithBaseURL, WithHTTPClient)
- Model manager: active/fallback tracking, Refresh, ActiveChain for fallback
- Config manager: YAML at ~/.config/mini-wiki/config.yaml (0600 perms)
- Conversation types: Thread, Message, Metadata, truncation, token estimation
- Error types: 17 Kind values with predicates (IsConnection, IsTimeout, etc.)
- Bubbletea TUI: chat interface with streaming responses, slash commands, model switching
- Ollama auto-start/stop: Launcher that spawns ollama serve if not running
  - Platform-specific process group management (Linux/macOS/other)
  - 30s startup timeout, graceful shutdown, only kills if we started it
- Command auto-completion: Tab cycling through available commands, /help hint

### What I struggled with / broke
- Initial keyboard input not working: forgot to forward tea.KeyMsg to textinput.Model - fixed
- Spinner not visible during streaming: added spinner.TickMsg handling
- Viewport scroll position reset on every chunk: added GotoBottom() tracking
- go vet complained about unused cancel func in streamChatCmd - fixed by passing CancelCtx through StreamStarted and calling it in StreamDone/StreamError handlers
- Bubbletea viewport.Model hides Content field in newer versions - had to track content separately
