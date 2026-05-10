// Package app defines the Bubbletea TUI application: model, update loop, and view rendering.
// It orchestrates the Ollama client, model manager, config, and conversation components
// into a cohesive terminal user interface.
package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/atotto/clipboard"

	"mini-wiki/internal/chart"
	"mini-wiki/internal/config"
	"mini-wiki/internal/conversation"
	"mini-wiki/internal/csvparser"
	"mini-wiki/internal/dataset"
	"mini-wiki/internal/export"
	"mini-wiki/internal/fileref"
	"mini-wiki/internal/filescanner"
	"mini-wiki/internal/memory"
	"mini-wiki/internal/modelmgr"
	"mini-wiki/internal/ollama"
	"mini-wiki/internal/projectkb"
	"mini-wiki/internal/rag"
	"mini-wiki/internal/ranking"
	"mini-wiki/internal/srs"
	"mini-wiki/internal/webfetch"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/xuri/excelize/v2"
)

// --- Message types for Bubbletea ---

type (
	// User messages
	UserSendMsg     struct{ Content string }
	UserSwitchModel struct{ Name string }

	// Streaming lifecycle
	StreamStarted struct {
		Stream          <-chan ollama.ChatStreamChunk
		Model           string
		Content         strings.Builder // accumulates full response
		CancelCtx       context.CancelFunc // call when stream fully consumed
	}
	StreamChunk struct {
		Text    string
		Content strings.Builder // shared reference for accumulation
	}
	StreamDone struct {
		FullText string
		Model    string
	}
	StreamError struct{ Err error }

	// Model / config events
	ModelsRefreshed struct{ Count int }
	ModelsFailed    struct{ Err error }
	PingComplete    struct{ OK bool }

	// Phase 2: File system & ingestion
	ScanRequested    struct{}
	ScanComplete     struct {
		Result *filescanner.ScanResult
	}
	ScanFailed       struct{ Err error }
	FilesListRequested struct{}
	IngestRequested  struct {
		Path     string
		Callback func(content string, size int64) // optional
	}
	IngestFailed     struct {
		Path string
		Err  error
	}
	IngestCompleteMsg struct{ Text string }
	RAGProgressMsg struct{ Text string }
	RAGDone struct {
		Path     string
		Chunks   int
		Error    string
		Progress []string
	}
	EmbedRequested struct{}

	// Phase 3: Web fetching
	FetchRequested  struct{ URL string }
	FetchComplete   struct {
		Result *webfetch.FetchResult
	}
	FetchFailed     struct{ Err error }

	// Phase 3: Export
	ExportRequested struct {
		Ranked bool
		Format string // "xlsx", "csv", "json"
		Output string
	}
	ExportComplete  struct {
		Result *export.ExportResult
	}
	ExportFailed    struct{ Err error }

	// Phase 4: Bookmarks & history & tool memory
	BookmarkAddRequested struct {
		Title       string
		Description string
		Source      string
		Tags        string
	}
	BookmarkListRequested struct{}
	HistoryListRequested  struct{}
	SkillsListRequested   struct{}
	FlawsListRequested    struct{}

	// Phase 5: SRS Pipeline
	SRSRequested struct{}
	SRSProgress  struct {
		Stage   int
		Message string
		Done    bool
	}
	SRSComplete struct {
		Content string
		RunID   string
	}
	SRSFailed struct{ Err error }

	// Tasks
	TaskAddRequested   struct{ Text string }
	TaskToggleRequested  struct{ Index int }
	TaskListRequested   struct{}

	// Phase 4: Ranking
	RankRequested struct{ Topic string }
	RankComplete struct {
		Result *ranking.RankResult
		Text   string
	}
	RankFailed      struct{ Err error }
	CompareRequested struct{ Topic string }
	DiscardRequested struct{ Threshold float64 }
	DiscardPreview  struct {
		Threshold float64
		Keep      int
		Discard   int
	}

	// Phase 5: Charts
	ChartRequested struct {
		Type    string
		ColumnX string
		ColumnY string
		Buckets int
	}
	ChartComplete struct{ Text string }
	ChartFailed   struct{ Err error }

	// Phase 7: Wizard
	WizardRequested struct{}
	WizardComplete  struct{ Text string }
)

// --- Layout ---

type taskItem struct {
	text   string
	done   bool
}

// fileNode represents one entry in the file tree (file or directory).
// suggestionItem represents an auto-complete suggestion with metadata.
type suggestionItem struct {
	text        string // value inserted on Tab
	description string // shown in hint bar
	category    string // "cmd", "file", "ref"
}

// --- Tokyo Night theme ---

var (
	// Core palette
	tokyoBg       = lipgloss.Color("#0d1117")
	tokyoSurface  = lipgloss.Color("#1f2335")
	tokyoOverlay  = lipgloss.Color("#24283b")
	tokyoFg       = lipgloss.Color("#c0caf5")
	tokyoComment  = lipgloss.Color("#565f89")
	tokyoBorder   = lipgloss.Color("#3b4261")
	tokyoBlue     = lipgloss.Color("#7aa2f7")
	tokyoCyan     = lipgloss.Color("#7dcfff")
	tokyoGreen    = lipgloss.Color("#9ece6a")
	tokyoOrange   = lipgloss.Color("#ff9e64")
	tokyoPurple   = lipgloss.Color("#bb9af7")
	tokyoRed      = lipgloss.Color("#f7768e")
	tokyoYellow   = lipgloss.Color("#e0af68")

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(tokyoPurple).
			Padding(0, 1)

	statusStyle = lipgloss.NewStyle().
			Foreground(tokyoComment).
			Padding(0, 1)

	userMsgStyle = lipgloss.NewStyle().
			Foreground(tokyoGreen).
			Background(tokyoSurface).
			Padding(0, 2)

	assistantHeaderStyle = lipgloss.NewStyle().
				Foreground(tokyoBlue).
				Bold(true).
				Background(tokyoOverlay).
				Padding(0, 2)

	assistantMsgStyle = lipgloss.NewStyle().
				Foreground(tokyoFg).
				Background(tokyoOverlay).
				Padding(0, 2)

	errorStyle = lipgloss.NewStyle().
			Foreground(tokyoRed)

	modelTagStyle = lipgloss.NewStyle().
			Foreground(tokyoPurple).
			Background(tokyoSurface).
			Padding(0, 1).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(tokyoComment)

	suggestionStyle = lipgloss.NewStyle().
			Foreground(tokyoFg).
			Background(tokyoOverlay).
			Padding(0, 1)

	hintStyle = lipgloss.NewStyle().
			Foreground(tokyoComment).
			Padding(0, 1)

	bottomBarStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tokyoBorder).
			Padding(0, 1)

	bottomRightStyle = lipgloss.NewStyle().
			Foreground(tokyoComment).
			Padding(0, 1)

	// Input box with subtle border
	inputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tokyoBorder).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Foreground(tokyoFg).
			Bold(true).
			Padding(0, 2)

	subHeaderStyle = lipgloss.NewStyle().
			Foreground(tokyoComment).
			Padding(0, 2)

	// Panel styles
	panelCenterStyle = lipgloss.NewStyle().
			Padding(0, 1)

	panelRightStyle = lipgloss.NewStyle().
			Background(tokyoSurface).
			Padding(1, 2)

	panelFocusStyle = lipgloss.NewStyle().
			Background(tokyoOverlay).
			Padding(1, 2)

	panelHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(tokyoBlue)

	infoLabelStyle = lipgloss.NewStyle().
			Foreground(tokyoComment)

	infoValueStyle = lipgloss.NewStyle().
			Foreground(tokyoFg)
)

// --- Application Model ---

// Application is the root Bubbletea model containing all state.
type Application struct {
	config  *config.Manager
	models  *modelmgr.Manager
	client  ollama.Client
	thread  *conversation.Thread
	scanner  filescanner.Scanner
	parser   csvparser.Parser
	fileref  fileref.Resolver
	fetcher   webfetch.Fetcher
	exporter   export.Exporter
	pkb       projectkb.DB
	ragClient     *rag.Client
	ragDir        string
	program       *tea.Program // for sending messages from goroutines
	currentRank  *ranking.RankResult // last /rank result in memory
	awaitingYn      bool                // waiting for y/n confirmation
	pendingYNMsg    string              // the question being asked
	pendingThreshold float64             // threshold being confirmed for /discard
	mem        memory.MemStore
	srsPipeline *srs.Pipeline

	// Right info panel visibility
	showInfoPanel bool // false = hidden, full width for chat

	// UI components
	input    textarea.Model
	viewport viewport.Model
	spinner  spinner.Model

	// UI state
	ready            bool
	streaming        bool
	retrying         bool // true when falling back to another model mid-stream
	currentStream    <-chan ollama.ChatStreamChunk
	streamContent    strings.Builder
	streamModel      string
	streamCancel     context.CancelFunc // cancels the stream's context when done
	width            int
	height           int
	statusMsg        string
	errMsg           string
	pongMsg          string
	viewportContent  string // tracks content since viewport.Model hides its content field

	// Phase 2: File system state
	fileIndex    *filescanner.ScanResult // last scan result (for @ completion)
	scanCfg      filescanner.ScannerConfig
	refCfg       fileref.ResolverConfig

	// Phase 3: Web fetch & export state
	fetchCfg     webfetch.FetcherConfig
	exportCfg    export.ExportConfig

	// Auto-completion
	suggestions  []suggestionItem
	tabIndex     int

	// Layout state
	estimatedTokens int
	showWelcome    bool // show welcome logo when chat is empty
	busy           bool // true when a background operation is running

	// Tasks / todo
	tasks         []taskItem
	actionHistory []string
}

// New creates a new Application with initialized components.
// ragWorkerDir is the directory containing the extracted Python RAG worker (empty if unavailable).
func New(cfg *config.Manager, client ollama.Client, mm *modelmgr.Manager, ragWorkerDir string) *Application {
	ti := textarea.New()
	ti.Placeholder = "Type a message... (/help for commands)"
	ti.Focus()
	ti.CharLimit = 4096
	ti.SetWidth(80)
	ti.MaxHeight = 8
	ti.ShowLineNumbers = false
	// Map Enter to submit (handled by Update loop), Shift+Enter = newline
	ti.KeyMap.InsertNewline.SetEnabled(false)
	ti.KeyMap.InsertNewline.SetKeys("shift+enter")
	ti.KeyMap.InsertNewline.SetEnabled(true)
	ti.KeyMap.InputEnd.SetKeys("ctrl+e")
	ti.KeyMap.InputBegin.SetKeys("ctrl+a")

	s := spinner.New()
	s.Style = lipgloss.NewStyle().Foreground(tokyoPurple)
	s.Spinner = spinner.Dot

	// Determine workspace root from CWD
	cwd, err := os.Getwd()
	rootDir := cwd
	if err == nil {
		rootDir = cwd
	}

	a := &Application{
		config:    cfg,
		client:    client,
		models:    mm,
		scanner:   filescanner.New(),
		parser:    csvparser.New(),
		fileref:   fileref.New(),
		fetcher:   webfetch.New(),
		exporter:   export.New(),
		pkb:        projectkb.New(),
		mem:        memory.New(),
		ragClient:  rag.New(),
		ragDir:     ragWorkerDir,
		thread:    conversation.NewThread("You are a helpful research assistant specializing in Software Engineering. Provide thorough, well-reasoned answers."),
		input:     ti,
		spinner:   s,
		statusMsg:   "Initializing...",
		showWelcome:   true,
		showInfoPanel: true,
		scanCfg: filescanner.ScannerConfig{
			RootDir: rootDir,
			MaxSize: 100 * 1024 * 1024,
		},
		refCfg: fileref.ResolverConfig{
			MaxRefs: 10,
			RootDir: rootDir,
		},
		fetchCfg:  webfetch.DefaultConfig(),
		exportCfg: export.DefaultConfig(),
		srsPipeline: srs.New(&srsLLMAdapter{client: client}, srs.PipelineConfig{
			ProjectName: "Research Project",
			Version:     "1.0",
			Model:       mm.Active(),
		}),
	}

	// Initialize tool memory (non-fatal on error)
	if err := a.mem.Init(); err != nil {
		a.statusMsg = fmt.Sprintf("Memory init: %v", err)
	} else {
		a.registerBuiltinSkills()
		a.updateSRSModelSkills()
	}

	// Initialize project KB (non-fatal on error)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.pkb.Open(ctx, rootDir); err != nil {
		a.statusMsg = fmt.Sprintf("Project KB: %v", err)
	}

	return a
}

// SetProgram stores the Bubbletea program reference for sending messages from goroutines.
func (a *Application) SetProgram(p *tea.Program) {
	a.program = p
}

// Init initializes the application by refreshing models and pinging Ollama.
func (a *Application) Init() tea.Cmd {
	return tea.Batch(
		a.spinner.Tick,
		func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := a.client.Ping(ctx); err != nil {
				return PingComplete{OK: false}
			}
			return PingComplete{OK: true}
		},
	)
}

// --- Update Loop ---

func (a *Application) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// --- Window resize ---
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height

		if !a.ready {
			a.viewport = viewport.New(msg.Width-2, msg.Height-8)
			a.viewport.YPosition = 4
			a.viewport.Style = lipgloss.NewStyle().Padding(1)
			a.input.SetWidth(msg.Width - 4)
			a.ready = true
			return a, tea.Batch(
				refreshModelsCmd(a.client, a.models),
				scanCmd(a.scanner, a.scanCfg),
			)
		}

		a.viewport.Width = msg.Width - 2
		a.viewport.Height = msg.Height - 8
		a.input.SetWidth(msg.Width - 4)
		return a, nil

	// --- Ping result ---
	case PingComplete:
		if msg.OK {
			a.statusMsg = "Ollama connected "
			a.pongMsg = ""
		} else {
			a.statusMsg = "Ollama not reachable -- start Ollama and type /refresh"
			a.pongMsg = "Ollama is not running. Please start it with: ollama serve"
		}
		return a, nil

	// --- Model refresh ---
	case ModelsRefreshed:
		a.statusMsg = fmt.Sprintf("Loaded %d models. Active: %s", msg.Count, a.models.Active())
		a.errMsg = ""
		if a.models.Active() != "" {
			_ = a.config.SetDefaultModel(a.models.Active())
		}
		return a, nil

	case ModelsFailed:
		a.errMsg = fmt.Sprintf("Failed to load models: %v", msg.Err)
		a.statusMsg = "Model load failed"
		return a, nil

	// --- User sends a message ---
	case UserSendMsg:
		content := strings.TrimSpace(msg.Content)
		originalContent := content // save for display (before RAG injection)
		if content == "" {
			return a, nil
		}

		// Hide welcome logo on first real message
		if a.showWelcome {
			a.showWelcome = false
			a.viewportContent = ""
			a.viewport.SetContent("")
		}

		// Clear input for any message type
		a.input.SetValue("")

		// Handle slash commands
		if strings.HasPrefix(content, "/") {
			return a.handleCommand(content)
		}

		// Auto-resolve @file references (works even without /scan)
		refs := a.fileref.FindRefs(content)
		if len(refs) > 0 {
			if len(refs) > a.refCfg.MaxRefs {
				a.errMsg = fmt.Sprintf("Too many file references (%d, max %d)", len(refs), a.refCfg.MaxRefs)
				return a, nil
			}
			// If we have a file index, use it for resolution
			if a.fileIndex != nil {
				result, err := a.fileref.ResolveAll(context.Background(), content, a.fileIndex, a.refCfg)
				if err != nil {
					a.errMsg = fmt.Sprintf("Ref resolution error: %v", err)
					return a, nil
				}
				for _, re := range result.Errors {
				a.appendLine(errorStyle.Render(fmt.Sprintf("Could not resolve %s: %s", re.Raw, re.Reason)))
			}
			if result.TotalSize > 0 {
				content = a.fileref.Inject(content, result)
				a.appendLine(fmt.Sprintf("Attached %d files (%d bytes)", len(result.Contents), result.TotalSize))
			}
		} else {
			// No file index: resolve @ refs by checking the filesystem directly
			a.resolveAtRefsDirect(&content, refs)
		}
	}

		// Auto-save to project KB memory (non-blocking, errors logged)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = a.pkb.SaveHistory(ctx, &projectkb.HistoryEntry{
				Query: content,
				Model: a.models.Active(),
			})
		}()

	// Agentic Query + Auto-RAG: try to answer from data first, then enrich with RAG context.
	// Agentic: send schema + question to coder LLM, generate Pandas query, execute locally.
	// This works immediately without /embed, for any structured dataset question.
	agenticAnswer := ""
	if a.pkb != nil {
		if projectDir := a.pkb.ProjectDir(); projectDir != "" {
			if ds, err := ranking.LoadDataset(projectDir); err == nil && ds.SourceFile != "" {
				if errMsg := a.ensureRAGStarted(); errMsg == "" {
					if resp, err := a.ragClient.QueryAgentic(ds.SourceFile, content, "qwen2.5-coder:7b"); err == nil && resp.Type == "query_answer" {
						agenticAnswer = resp.Answer
					}
				}
			}
		}
	}
	if agenticAnswer != "" {
		content = fmt.Sprintf("[Data analysis result: %s]\n\nUser question: %s", agenticAnswer, content)
	}

	// Auto-RAG: search the vector database for relevant context (embedding optional)
	ragResult, _ := a.queryRAG(content, 3)
	if ragResult != nil && len(ragResult.Sources) > 0 {
		var ragParts []string
		ragParts = append(ragParts, "\n[Context from knowledge base:]")
		for _, src := range ragResult.Sources {
			ragParts = append(ragParts, fmt.Sprintf("Source %s (relevance: %.2f):", src.File, src.Score))
			ragParts = append(ragParts, src.Text)
		}
		content = strings.Join(ragParts, "\n") + "\n\nUser question: " + content
	}

	// Compact thread if context window is getting full
	a.compactThreadIfNeeded()

	// Add user message to thread
	a.thread.Add(conversation.Message{
		Role:    conversation.RoleUser,
		Content: content,
	})
	a.updateTokenCount()

	// Render user message in viewport (original text, not RAG-injected)
	a.appendLine(formatUserMsg(originalContent))

	// Clear input and start streaming
	a.input.SetValue("")
	a.errMsg = ""
	a.statusMsg = fmt.Sprintf("Thinking (%s)...", a.models.Active())

	return a, streamChatCmd(a.client, a.models, a.thread)

	// --- File scanning ---
	case ScanRequested:
		a.busy = true
		a.statusMsg = "Scanning files..."
		a.errMsg = ""
		return a, scanCmd(a.scanner, a.scanCfg)

	case ScanComplete:
		a.busy = false
		a.fileIndex = msg.Result
		summary := fmt.Sprintf("Scanned: %d files, %d dirs, %s total in %s",
			len(msg.Result.Files), msg.Result.Dirs, humanBytes(msg.Result.Total), msg.Result.Elapsed.Round(time.Millisecond))
		a.appendLine(helpStyle.Render(summary))
		if len(msg.Result.Skipped) > 0 {
			a.appendLine(helpStyle.Render(fmt.Sprintf("Skipped %d entries", len(msg.Result.Skipped))))
		}
		a.statusMsg = fmt.Sprintf("Ready -- %d files indexed", len(msg.Result.Files))
		return a, nil

	case ScanFailed:
		a.busy = false
		a.errMsg = fmt.Sprintf("Scan failed: %v", msg.Err)
		return a, nil

	case FilesListRequested:
		if a.fileIndex == nil {
			a.appendLine(errorStyle.Render("No files indexed. Run /scan first."))
			return a, nil
		}
		a.appendLine(formatFileTree(a.fileIndex))
		return a, nil

	// --- File ingestion (fast: parse + register, no embeddings) ---
	case IngestRequested:
		a.busy = true
		path := msg.Path
		a.statusMsg = fmt.Sprintf("Parsing %s...", filepath.Base(path))
		return a, ingestLocalCmd(a, path)

	case RAGDone:
		a.busy = false
		if msg.Error != "" {
			a.appendLine(fmt.Sprintf("[Error] %s", msg.Error))
			a.statusMsg = "Error"
		} else if msg.Chunks > 0 {
			a.appendLine(fmt.Sprintf("[Embed done] %d chunks indexed for RAG search\n", msg.Chunks))
			a.statusMsg = fmt.Sprintf("Embedded %d chunks", msg.Chunks)
			// Register as active dataset for ranking
			ext := strings.ToLower(filepath.Ext(msg.Path))
			format := "txt"
			if ext == ".csv" || ext == ".tsv" {
				format = "csv"
			} else if ext == ".jsonl" || ext == ".ndjson" || ext == ".json" {
				format = "jsonl"
			} else if ext == ".xlsx" {
				format = "xlsx"
			}
			_ = a.pkb.SetActiveDataset(context.Background(), msg.Path, format, msg.Chunks)
		}
		return a, nil

	case IngestFailed:
		a.errMsg = fmt.Sprintf("Ingest failed: %v", msg.Err)
		a.statusMsg = "Ingest failed"
		return a, nil

	case IngestCompleteMsg:
		a.busy = false
		a.appendLine(msg.Text)
		a.statusMsg = "Ingest complete"
		return a, nil

	case RAGProgressMsg:
		a.appendLine(fmt.Sprintf("  %s", msg.Text))
		a.statusMsg = msg.Text
		return a, nil

	case EmbedRequested:
		a.busy = true
		a.statusMsg = "Starting embedding (press Escape to cancel)..."
		a.appendLine("=== Starting embedding for RAG search (press Escape to cancel) ===\n")
		// Get active dataset path
		filePath, _, err := a.pkb.GetActiveDataset(context.Background())
		if err != nil {
			a.busy = false
			a.errMsg = "No dataset ingested. Run /ingest first."
			return a, nil
		}
		go a.embedStream(filePath)
		return a, nil

	// --- Ranking ---
	case RankRequested:
		a.busy = true
		a.statusMsg = "Ranking dataset..."
		return a, rankCmd(a, msg.Topic)

	case RankComplete:
		a.busy = false
		a.appendLine(msg.Text)
		if msg.Result != nil {
			a.currentRank = msg.Result
			_ = a.pkb.SaveRanking(context.Background(), &projectkb.RankingResult{
				Topic:  msg.Result.Topic,
				Scores: ranking.ResultsToJSON(msg.Result.Scores, msg.Result.Topic),
			})
		}
		a.statusMsg = "Ranking complete"
		return a, nil

	case RankFailed:
		a.busy = false
		a.errMsg = fmt.Sprintf("Ranking failed: %v", msg.Err)
		return a, nil

	case CompareRequested:
		if a.currentRank == nil {
			a.errMsg = "No ranking to compare. Run /rank first."
			return a, nil
		}
		if msg.Topic == "" {
			// No topic: show current ranking again
			table := ranking.FormatRankingTable(a.currentRank, 20)
			a.appendLine(table)
		} else {
			// New topic: rerank and show comparison
			a.busy = true
			a.statusMsg = "Re-ranking with new topic..."
			return a, compareCmd(a, msg.Topic)
		}
		a.statusMsg = "Compare"
		return a, nil

	case DiscardRequested:
		if a.currentRank == nil {
			a.errMsg = "No ranking to discard from. Run /rank first."
			return a, nil
		}
		threshold := msg.Threshold
		if threshold < 0 || threshold > 1 {
			a.errMsg = "Threshold must be between 0.0 and 1.0"
			return a, nil
		}

		// Count keep vs discard
		keep := 0
		discard := 0
		for _, s := range a.currentRank.Scores {
			if s >= threshold {
				keep++
			} else {
				discard++
			}
		}

		preview := fmt.Sprintf("Threshold: %.2f\n  Keep: %d rows (score >= %.2f)\n  Discard: %d rows (score < %.2f)",
			threshold, keep, threshold, discard, threshold)
		a.appendLine(preview)

		if discard == 0 {
			a.appendLine("No rows to discard at this threshold.")
			return a, nil
		}

		// Ask for confirmation (preview-only: --preview flag skips confirm)
		a.awaitingYn = true
		a.pendingYNMsg = fmt.Sprintf("Discard %d rows below score %.2f? (y/N)", discard, threshold)
		a.pendingThreshold = threshold
		a.appendLine(a.pendingYNMsg)
		a.statusMsg = "Waiting for confirmation..."
		return a, nil

	// --- Charts ---
	case ChartRequested:
		if a.currentRank == nil || a.currentRank.Dataset == nil {
			a.errMsg = "No data to chart. Ingest a dataset and run /rank first."
			return a, nil
		}
		a.busy = true
		return a, chartCmd(a, msg)

	case ChartComplete:
		a.busy = false
		a.appendLine(msg.Text)
		a.statusMsg = "Chart rendered"
		return a, nil

	case ChartFailed:
		a.busy = false
		a.errMsg = fmt.Sprintf("Chart error: %v", msg.Err)
		return a, nil

	// --- Wizard ---
	case WizardRequested:
		a.busy = true
		a.statusMsg = "Running system check..."
		return a, wizardCmd()

	case WizardComplete:
		a.busy = false
		a.appendLine(msg.Text)
		a.statusMsg = "Wizard complete"
		return a, nil

	// --- Web fetching ---
	case FetchRequested:
		a.statusMsg = fmt.Sprintf("Fetching %s...", msg.URL)
		a.errMsg = ""
		return a, fetchCmd(a.fetcher, msg.URL, a.fetchCfg)

	case FetchComplete:
		r := msg.Result
		summary := fmt.Sprintf("Fetched %s (%s, %d bytes, %s)",
			r.Meta.FinalURL, r.Meta.Title, r.Meta.ContentLength, r.Meta.Duration.Round(time.Millisecond))
		a.appendLine(helpStyle.Render(summary))
		// Append content to conversation context
		content := fmt.Sprintf("\n--- Web: %s ---\n%s\n", r.Meta.FinalURL, r.Content)
		a.appendLine(helpStyle.Render(content))
		a.statusMsg = "Fetch complete"
		return a, nil

	case FetchFailed:
		a.errMsg = fmt.Sprintf("Fetch failed: %v", msg.Err)
		a.statusMsg = "Fetch failed"
		return a, nil

	// --- Export ---
	case ExportRequested:
		a.busy = true
		a.statusMsg = "Exporting data..."
		if msg.Ranked && a.currentRank != nil {
			return a, exportRankedCmd(a, msg)
		}
		// Try exporting the dataset first; fall back to conversation
		return a, exportDatasetCmd(a, msg)

	case ExportComplete:
		a.busy = false
		r := msg.Result
		summary := fmt.Sprintf("Exported %d rows to %s (%s, %s)",
			r.Rows, r.FileName, humanBytes(r.Size), r.Duration.Round(time.Millisecond))
		a.appendLine(summary)
		a.statusMsg = "Export complete"
		return a, nil

	case ExportFailed:
		a.busy = false
		a.errMsg = fmt.Sprintf("Export failed: %v", msg.Err)
		a.statusMsg = "Export failed"
		return a, nil

	// --- Bookmarks ---
	case BookmarkAddRequested:
		bm := projectkb.Bookmark{
			Title:       msg.Title,
			Description: msg.Description,
			Source:      msg.Source,
			Tags:        msg.Tags,
		}
		if err := a.pkb.SaveBookmark(context.Background(), &bm); err != nil {
			a.errMsg = fmt.Sprintf("Save bookmark: %v", err)
		} else {
			a.appendLine(helpStyle.Render(fmt.Sprintf("Bookmarked: %s", msg.Title)))
			a.statusMsg = "Bookmark saved"
		}
		return a, nil

	case BookmarkListRequested:
		bookmarks, err := a.pkb.GetBookmarks(context.Background())
		if err != nil {
			a.errMsg = fmt.Sprintf("Load bookmarks: %v", err)
			return a, nil
		}
		if len(bookmarks) == 0 {
			a.appendLine(helpStyle.Render("No bookmarks yet. Use /bookmark <title> to save one."))
		} else {
			var b strings.Builder
			b.WriteString("Bookmarks:\n")
			for _, bm := range bookmarks {
				t := bm.CreatedAt.Format("2006-01-02 15:04")
				b.WriteString(fmt.Sprintf("  - %s [%s] %s\n", bm.Title, bm.Source, t))
				if bm.Tags != "" {
					b.WriteString(fmt.Sprintf("    Tags: %s\n", bm.Tags))
				}
			}
			a.appendLine(helpStyle.Render(b.String()))
		}
		a.statusMsg = "Bookmarks loaded"
		return a, nil

	// --- History ---
	case HistoryListRequested:
		entries, err := a.pkb.GetHistory(context.Background(), 20)
		if err != nil {
			a.errMsg = fmt.Sprintf("Load history: %v", err)
			return a, nil
		}
		if len(entries) == 0 {
			a.appendLine(helpStyle.Render("No history yet."))
		} else {
			var b strings.Builder
			b.WriteString("Recent queries:\n")
			for _, e := range entries {
				t := e.CreatedAt.Format("15:04")
				b.WriteString(fmt.Sprintf("  - [%s] %s\n", t, truncateText(e.Query, 80)))
			}
			a.appendLine(helpStyle.Render(b.String()))
		}
		a.statusMsg = "History loaded"
		return a, nil

	// --- Tool memory ---
	case SkillsListRequested:
		skills, err := a.mem.GetSkills("")
		if err != nil {
			a.errMsg = fmt.Sprintf("Load skills: %v", err)
			return a, nil
		}
		if len(skills) == 0 {
			a.appendLine(helpStyle.Render("No skills registered yet."))
		} else {
			var b strings.Builder
			b.WriteString("Tool skills:\n")
			for _, s := range skills {
				b.WriteString(fmt.Sprintf("  - %s (%s) — %s\n", s.Name, s.Category, s.Description))
				if s.Command != "" {
					b.WriteString(fmt.Sprintf("    Command: %s\n", s.Command))
				}
			}
			a.appendLine(helpStyle.Render(b.String()))
		}
		a.statusMsg = "Skills loaded"
		return a, nil

	case FlawsListRequested:
		flaws, err := a.mem.GetFlaws(nil)
		if err != nil {
			a.errMsg = fmt.Sprintf("Load flaws: %v", err)
			return a, nil
		}
		if len(flaws) == 0 {
			a.appendLine(helpStyle.Render("No known flaws. Good! Keep an eye out for issues."))
		} else {
			var b strings.Builder
			b.WriteString("Known flaws & solutions:\n")
			for _, f := range flaws {
				status := " "
				if f.Resolved {
					status = "x"
				}
				b.WriteString(fmt.Sprintf("  [%s] %s: %s\n", status, f.ID, f.Title))
				b.WriteString(fmt.Sprintf("    Symptom: %s\n", f.Symptom))
				b.WriteString(fmt.Sprintf("    Fix: %s\n", f.Solution))
			}
			a.appendLine(helpStyle.Render(b.String()))
		}
		a.statusMsg = "Flaws loaded"
		return a, nil

	// --- SRS Pipeline ---
	case SRSRequested:
		a.statusMsg = "Starting SRS pipeline..."
		a.errMsg = ""
		// Get data from conversation thread for processing
		var dataLines []string
		for _, msg := range a.thread.Messages {
			dataLines = append(dataLines, msg.Content)
		}
		data := strings.Join(dataLines, "\n")
		if data == "" {
			data = "No conversation data available. Run /scan and /ingest first, or type some requirements."
		}
		return a, srsPipelineCmd(a.srsPipeline, data)

	case SRSProgress:
		a.statusMsg = msg.Message
		if msg.Done {
			a.appendLine(helpStyle.Render(fmt.Sprintf("   Stage %d: %s", msg.Stage, msg.Message)))
		}
		return a, nil

	case SRSComplete:
		// Display the SRS document
		a.appendLine(helpStyle.Render("\n--- SRS Document Generated ---\n"))
		// Show first 20 lines as preview
		lines := strings.Split(msg.Content, "\n")
		preview := lines
		if len(preview) > 25 {
			preview = preview[:25]
			preview = append(preview, "... (full document saved to project KB)")
		}
		a.appendLine(strings.Join(preview, "\n"))
		a.statusMsg = fmt.Sprintf("SRS generated: %s", msg.RunID)
		// Save to project KB
		a.pkb.SaveSRSDocument(context.Background(), &projectkb.SRSDocument{
			ProjectName: "Research Project",
			Version:     "1.0",
			Standard:    "ieee_830",
			Content:     msg.Content,
			Format:      "markdown",
		})
		return a, nil

	case SRSFailed:
		a.errMsg = fmt.Sprintf("SRS pipeline failed: %v", msg.Err)
		a.statusMsg = "SRS failed"
		return a, nil

	// --- Streaming ---
	case StreamStarted:
		a.streaming = true
		a.currentStream = msg.Stream
		a.streamModel = msg.Model
		a.streamContent = msg.Content // shared builder
		a.streamCancel = msg.CancelCtx
		a.statusMsg = fmt.Sprintf("Receiving from %s...", a.streamModel)

		// Add assistant header only on first attempt, not on fallback retry
		if !a.retrying {
			a.appendToViewport(assistantHeaderStyle.Render(fmt.Sprintf("%s:", a.streamModel)))
		} else {
			a.retrying = false
		}
		return a, readNextChunkCmd(a.currentStream, a.streamModel, &a.streamContent)

	case StreamChunk:
		a.appendToViewport(msg.Text)
		return a, readNextChunkCmd(a.currentStream, a.streamModel, &a.streamContent)

	case StreamDone:
		a.streaming = false
		a.currentStream = nil
		if a.streamCancel != nil {
			a.streamCancel()
			a.streamCancel = nil
		}
		a.statusMsg = fmt.Sprintf("Ready -- %s", a.models.Active())

		// Store full assistant message in thread
		fullText := msg.FullText
		if fullText != "" {
			a.thread.Add(conversation.Message{
				Role:    conversation.RoleAssistant,
				Content: fullText,
				Metadata: conversation.Metadata{
					Model: msg.Model,
				},
			})
			// Auto-save assistant response to project KB memory
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = a.pkb.SaveHistory(ctx, &projectkb.HistoryEntry{
					Query:    "[AI response]",
					Response: fullText,
					Model:    msg.Model,
				})
			}()
		}

		a.input.Focus()
		a.streamContent.Reset()
		a.appendToViewport("\n") // blank line after AI response
		return a, nil

	case StreamError:
		// Clean up the failed stream's context
		if a.streamCancel != nil {
			a.streamCancel()
			a.streamCancel = nil
		}
		a.currentStream = nil

		// Attempt fallback if there are more models to try
		chain := a.models.ActiveChain()
		if len(chain) > 1 {
			a.retrying = true
			_ = a.models.SetActive(chain[0])
			a.statusMsg = fmt.Sprintf("Falling back to %s...", a.models.Active())
			return a, streamChatCmd(a.client, a.models, a.thread)
		}

		a.streaming = false
		a.streamContent.Reset()
		a.errMsg = fmt.Sprintf("Error: %v", msg.Err)
		a.statusMsg = "Failed"
		a.input.Focus()
		return a, nil

	// --- Model switching ---
	case UserSwitchModel:
		if err := a.models.SetActive(msg.Name); err != nil {
			a.errMsg = fmt.Sprintf("Cannot switch: %v", err)
		} else {
			a.statusMsg = fmt.Sprintf("Switched to %s", a.models.Active())
			a.errMsg = ""
		}
		return a, nil

	// --- Keyboard input ---
	case tea.KeyMsg:
		// Handle y/n confirmation prompt
		if a.awaitingYn {
			a.awaitingYn = false
			if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
				ch := msg.Runes[0]
				if ch == 'y' || ch == 'Y' {
					// Confirmed: execute the discard
					if a.currentRank != nil {
						a.discardRowsAtThreshold(a.currentRank, a.pendingThreshold)
					}
				} else {
					a.appendLine("Cancelled.")
				}
			} else {
				a.appendLine("Cancelled.")
			}
			a.statusMsg = "Ready"
			var cmd tea.Cmd
			a.input, cmd = a.input.Update(msg)
			return a, cmd
		}

		// Escape cancels EVERYTHING (universal cancel key)
		if msg.Type == tea.KeyEscape {
			a.clearSuggestions()
			// Cancel LLM streaming
			if a.streaming {
				a.streaming = false
				if a.streamCancel != nil {
					a.streamCancel()
					a.streamCancel = nil
				}
				a.currentStream = nil
				a.streamContent.Reset()
			}
			// Cancel RAG worker
			if a.busy && a.ragClient != nil && a.ragClient.IsRunning() {
				a.ragClient.Stop()
			}
			a.busy = false
			a.statusMsg = "Cancelled"
			a.input.Focus()
			var cmd tea.Cmd
			a.input, cmd = a.input.Update(msg)
			return a, cmd
		}

		// Ctrl+C during streaming: interrupt
		if a.streaming {
			if msg.Type == tea.KeyCtrlC {
				a.streaming = false
				if a.streamCancel != nil {
					a.streamCancel()
					a.streamCancel = nil
				}
				a.currentStream = nil
				a.streamContent.Reset()
				a.statusMsg = "Interrupted"
				a.input.Focus()
				a.clearSuggestions()
				return a, nil
			}
			return a, nil
		}

		// Handle Enter to submit
		if msg.Type == tea.KeyEnter {
			// Apply tab-completed suggestion if one is selected
			if a.tabIndex >= 0 && len(a.suggestions) > 0 && a.tabIndex < len(a.suggestions) {
				selected := a.suggestions[a.tabIndex].text
				a.input.SetValue(selected + " ")
				a.input.SetCursor(len(selected) + 1)
				a.clearSuggestions()
				return a, nil
			}
			content := a.input.Value()
			if content != "" {
				a.clearSuggestions()
				return a, func() tea.Msg { return UserSendMsg{Content: content} }
			}
			return a, nil
		}

		// Handle Tab for command / file completion
		if msg.Type == tea.KeyTab {
			val := a.input.Value()
			// Trigger completion for commands, @file refs, or path-like input
			if strings.HasPrefix(val, "/") || strings.Contains(val, "@") ||
				strings.HasPrefix(val, ".") || strings.Contains(val, "/") {
				if len(a.suggestions) > 0 && a.tabIndex >= 0 {
					a.cycleSuggestion(val)
					return a, nil
				}
				a.updateSuggestions()
				if len(a.suggestions) > 0 {
					a.tabIndex = 0
					a.applySuggestion(a.suggestions[0].text)
					return a, nil
				}
			}
			// No suggestions or not a completion context: forward Tab to input
			var cmd tea.Cmd
			a.input, cmd = a.input.Update(msg)
			_ = cmd
			return a, nil
		}

		// Handle Ctrl+C / Ctrl+D to quit (only when not streaming/busy)
		if msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyCtrlD {
			if a.streaming || a.busy {
				return a, nil // Escape handles cancellation
			}
			return a, tea.Quit
		}

		// Forward key events to text input, then update suggestions
		var cmd tea.Cmd
		a.input, cmd = a.input.Update(msg)
		a.updateSuggestions()
		return a, cmd

	// --- Mouse events (wheel only — leave clicks for native selection) ---
	case tea.MouseMsg:
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			a.viewport, _ = a.viewport.Update(msg)
		}
		return a, nil

	// --- Task messages ---
	case TaskAddRequested:
		if msg.Text != "" {
			a.tasks = append(a.tasks, taskItem{text: msg.Text, done: false})
			a.logAction("Task added: " + msg.Text)
			a.statusMsg = "Task added"
		}
		return a, nil

	case TaskToggleRequested:
		if msg.Index >= 0 && msg.Index < len(a.tasks) {
			a.tasks[msg.Index].done = !a.tasks[msg.Index].done
			status := "done"
			if !a.tasks[msg.Index].done {
				status = "pending"
			}
			a.logAction("Task " + status + ": " + a.tasks[msg.Index].text)
		}
		return a, nil

	case TaskListRequested:
		if len(a.tasks) == 0 {
			a.appendLine("No tasks. Use /task <description> to add one.")
		} else {
			var b strings.Builder
			b.WriteString("Tasks:\n")
			for _, t := range a.tasks {
				mark := " "
				if t.done {
					mark = "x"
				}
				b.WriteString(fmt.Sprintf("  [%s] %s\n", mark, t.text))
			}
			a.appendLine(b.String())
		}
		return a, nil

	// --- Spinner tick ---
	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spinner, cmd = a.spinner.Update(msg)
		return a, cmd

	// --- Window focus changes ---
	case tea.FocusMsg:
		a.input.Focus()
		return a, nil

	default:
		return a, nil
	}
}

// handleCommand processes slash-prefixed commands.
func (a *Application) handleCommand(cmd string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return a, nil
	}

	switch parts[0] {
	case "/help":
		// /help [command] — man-page style
		subCmd := ""
		if len(parts) > 1 && parts[1] != "" {
			subCmd = strings.ToLower(strings.TrimPrefix(parts[1], "/"))
		}
		if subCmd == "" {
			a.appendLine(helpStyle.Render(helpSummary()))
		} else if man, ok := manPages[subCmd]; ok {
			a.appendLine(helpStyle.Render(man))
		} else {
			a.errMsg = fmt.Sprintf("No help for '%s'. Try /help for commands.", parts[1])
		}

	case "/model":
		if len(parts) < 2 {
			a.errMsg = "Usage: /model <name>"
		} else {
			return a, func() tea.Msg { return UserSwitchModel{Name: parts[1]} }
		}

	case "/models":
		names := a.models.AvailableNames()
		if len(names) == 0 {
			a.appendLine(errorStyle.Render("No models available. Try /refresh"))
		} else {
			var b strings.Builder
			b.WriteString("Available models:\n")
			for _, name := range names {
				marker := "  "
				if name == a.models.Active() {
					marker = "> "
				}
				b.WriteString(fmt.Sprintf("  %s%s\n", marker, name))
			}
			a.appendLine(helpStyle.Render(b.String()))
		}

	case "/refresh":
		a.statusMsg = "Refreshing models..."
		return a, refreshModelsCmd(a.client, a.models)

	case "/clear":
		a.thread = conversation.NewThread(a.thread.System)
		a.viewportContent = ""
		a.viewport.SetContent("")
		a.statusMsg = "Conversation cleared"
		a.errMsg = ""

	case "/system":
		if len(parts) < 2 {
			a.errMsg = "Usage: /system <prompt>"
		} else {
			a.thread.System = strings.Join(parts[1:], " ")
			a.statusMsg = "System prompt updated"
			a.errMsg = ""
		}

	case "/scan":
		return a, func() tea.Msg { return ScanRequested{} }

	case "/files":
		return a, func() tea.Msg { return FilesListRequested{} }

	case "/wizard":
		return a, func() tea.Msg { return WizardRequested{} }

	case "/ingest":
		if len(parts) < 2 {
			a.errMsg = "Usage: /ingest <filepath>"
			return a, nil
		}
		path := strings.TrimPrefix(parts[1], "@")
		return a, func() tea.Msg { return IngestRequested{Path: path} }

	case "/fetch":
		if len(parts) < 2 {
			a.errMsg = "Usage: /fetch <url>"
			return a, nil
		}
		return a, func() tea.Msg { return FetchRequested{URL: parts[1]} }

	case "/export":
		ranked := false
		format := "xlsx"
		output := ""
		for _, p := range parts {
			if p == "--ranked" {
				ranked = true
			} else if strings.HasPrefix(p, "--format=") {
				format = strings.TrimPrefix(p, "--format=")
			} else if strings.HasPrefix(p, "--output=") {
				output = strings.TrimPrefix(p, "--output=")
			}
		}
		return a, func() tea.Msg {
			return ExportRequested{Ranked: ranked, Format: format, Output: output}
		}

	case "/bookmark":
		if len(parts) < 2 {
			a.errMsg = "Usage: /bookmark <title>"
			return a, nil
		}
		title := strings.Join(parts[1:], " ")
		return a, func() tea.Msg {
			return BookmarkAddRequested{Title: title, Source: "chat"}
		}

	case "/bookmarks":
		return a, func() tea.Msg { return BookmarkListRequested{} }

	case "/history":
		return a, func() tea.Msg { return HistoryListRequested{} }

	case "/skills":
		return a, func() tea.Msg { return SkillsListRequested{} }

	case "/flaws":
		return a, func() tea.Msg { return FlawsListRequested{} }

	case "/task":
		if len(parts) < 2 {
			a.errMsg = "Usage: /task <description>"
			return a, nil
		}
		return a, func() tea.Msg { return TaskAddRequested{Text: strings.Join(parts[1:], " ")} }

	case "/tasks":
		return a, func() tea.Msg { return TaskListRequested{} }

	case "/srs":
		return a, func() tea.Msg { return SRSRequested{} }

	case "/rank":
		if len(parts) < 2 {
			a.errMsg = "Usage: /rank <topic>"
			return a, nil
		}
		topic := strings.Join(parts[1:], " ")
		return a, func() tea.Msg { return RankRequested{Topic: topic} }

	case "/compare":
		if len(parts) > 1 {
			topic := strings.Join(parts[1:], " ")
			return a, func() tea.Msg { return CompareRequested{Topic: topic} }
		}
		return a, func() tea.Msg { return CompareRequested{Topic: ""} }

	case "/discard":
		if len(parts) >= 2 && parts[1] == "--reset" {
			if a.currentRank != nil {
				a.currentRank.DiscardCount = 0
				a.appendLine("All previously discarded rows restored.")
			} else {
				a.errMsg = "No ranking to reset."
			}
			return a, nil
		}
		if len(parts) >= 2 && parts[1] == "--preview" {
			if len(parts) < 3 {
				a.errMsg = "Usage: /discard --preview <threshold>"
				return a, nil
			}
			threshold, err := strconv.ParseFloat(parts[2], 64)
			if err != nil || threshold < 0 || threshold > 1 {
				a.errMsg = "Threshold must be between 0.0 and 1.0"
				return a, nil
			}
			return a, func() tea.Msg { return DiscardRequested{Threshold: threshold} }
		}
		if len(parts) < 2 {
			a.errMsg = "Usage: /discard <threshold>"
			return a, nil
		}
		threshold, err := strconv.ParseFloat(parts[1], 64)
		if err != nil || threshold < 0 || threshold > 1 {
			a.errMsg = "Threshold must be a number between 0.0 and 1.0"
			return a, nil
		}
		return a, func() tea.Msg { return DiscardRequested{Threshold: threshold} }

	case "/infer":
		if len(parts) < 2 {
			a.errMsg = "Usage: /infer <filename>"
			return a, nil
		}
		filename := strings.TrimPrefix(parts[1], "@")
		format := dataset.AutoDetect(filename)
		if format == "" {
			a.appendLine(fmt.Sprintf("Cannot detect format for %s", filename))
		} else {
			a.appendLine(fmt.Sprintf("Detected: %s", dataset.DetectFormat(filename)))
			a.appendLine(fmt.Sprintf("Try: /ingest @%s", filename))
		}
		return a, nil

	case "/embed":
		return a, func() tea.Msg { return EmbedRequested{} }

	case "/chart":
		if len(parts) < 2 {
			a.errMsg = "Usage: /chart <type> [options]"
			return a, nil
		}
		chartType := strings.ToLower(parts[1])
		colX := ""
		colY := ""
		buckets := 10
		// Parse options: column=<name>, x=<col>, y=<col>, buckets=<n>
		for _, p := range parts[2:] {
			if strings.HasPrefix(p, "column=") {
				colX = strings.TrimPrefix(p, "column=")
			} else if strings.HasPrefix(p, "x=") {
				colX = strings.TrimPrefix(p, "x=")
			} else if strings.HasPrefix(p, "y=") {
				colY = strings.TrimPrefix(p, "y=")
			} else if strings.HasPrefix(p, "buckets=") {
				n, err := strconv.Atoi(strings.TrimPrefix(p, "buckets="))
				if err == nil && n > 0 {
					buckets = n
				}
			}
		}
		return a, func() tea.Msg {
			return ChartRequested{Type: chartType, ColumnX: colX, ColumnY: colY, Buckets: buckets}
		}

	case "/cancel":
		a.statusMsg = "Cancelling..."
		if a.ragClient != nil && a.ragClient.IsRunning() {
			a.ragClient.Stop()
			a.appendLine("[RAG operation cancelled]")
		}
		a.statusMsg = "Cancelled"
		return a, nil

	case "/clip":
		content := a.viewportContent
		if content == "" {
			a.appendLine("Nothing to copy.")
			return a, nil
		}
		if err := clipboard.WriteAll(content); err != nil {
			a.appendLine(fmt.Sprintf("Copy failed: %v", err))
			return a, nil
		}
		a.appendLine(fmt.Sprintf("Copied %d characters to clipboard.", len(content)))
		return a, nil

	case "/panel":
		a.showInfoPanel = !a.showInfoPanel
		if a.showInfoPanel {
			a.appendLine("Info panel shown.")
		} else {
			a.appendLine("Info panel hidden.")
		}
		return a, nil

	case "/exit":
		return a, tea.Quit

	default:
		a.errMsg = fmt.Sprintf("Unknown command: %s (try /help)", parts[0])
	}

	return a, nil
}

// --- View Rendering ---

func (a *Application) View() string {
	if !a.ready {
		return "\n  Initializing mini-wiki..."
	}

	w := a.width
	if w < 60 {
		w = 60
	}
	h := a.height
	if h < 20 {
		h = 20
	}

	// Status line text with spinner
	statusText := a.statusMsg
	if a.streaming || a.busy {
		statusText = fmt.Sprintf("%s %s", a.spinner.View(), statusText)
	}

	// Error text
	errText := ""
	if a.errMsg != "" {
		errText = "  ! " + a.errMsg
	} else if a.pongMsg != "" {
		errText = "  ! " + a.pongMsg
	}

	// Project dir for header (centered)
	projDir := ""
	if a.pkb != nil {
		projDir = a.pkb.ProjectDir()
	}
	shortDir := projDir
	if len(shortDir) > 40 {
		parts := strings.Split(shortDir, "/")
		if len(parts) > 3 {
			shortDir = parts[len(parts)-3] + "/" + parts[len(parts)-2] + "/" + parts[len(parts)-1]
		}
	}

	// --- Centered project path header ---
	padding := (w - len(shortDir)) / 2
	if padding < 0 {
		padding = 0
	}
	headerLine := headerStyle.Render(strings.Repeat(" ", padding) + shortDir)

	// --- Status sub-header ---
	subLine := ""
	if errText != "" {
		subLine = subHeaderStyle.Render(statusText + errText)
	} else {
		subLine = subHeaderStyle.Render(statusText + "  |  Tokens: " + fmt.Sprintf("%d", a.estimatedTokens))
	}

	// Suggestion overlay (pops up above input)
	overlayLines := 0
	suggestionText := ""
	if len(a.suggestions) > 0 {
		overlayLines = 1
		suggestionText = suggestionStyle.Render(formatSuggestions(a.suggestions, a.tabIndex))
	}

	// Bottom bar: model name left, token info right, with border
	leftInfo := fmt.Sprintf(" %s ", a.models.Active())
	rightInfo := fmt.Sprintf(" tokens: %d  |  models: %d ", a.estimatedTokens, len(a.models.Available()))

	// Lay out left and right within available width
	// Inner area = (w - 2) - 2 (border) - 2 (padding) = w - 6
	barContent := leftInfo
	rightLen := lipgloss.Width(rightInfo)
	leftLen := lipgloss.Width(leftInfo)
	innerW := w - 6
	padLen := innerW - leftLen - rightLen
	if padLen > 0 {
		barContent += strings.Repeat(" ", padLen) + rightInfo
	} else if innerW > leftLen {
		barContent += "  " + rightInfo
	}
	bottomBar := bottomBarStyle.Width(w - 2).Render(barContent)

	// Layout: header(2) + sub(2) + panels(panelH-2) + overlay(0/1) + \n(1) + input(3) + \n(1) + bar(1) = h
	//   = 4 + panelH - 2 + overlay + 1 + 3 + 1 + 1 = panelH + 8 + overlay
	//   panelH = h - 8 - overlayLines
	panelH := h - 8 - overlayLines
	if panelH < 3 {
		panelH = 3
	}

	// Two panels: center (80%) + right (20%), or full width if panel hidden
	rightW := 0
	if a.showInfoPanel {
		rightW = w * 20 / 100
		if rightW < 18 {
			rightW = 18
		}
	}
	centerW := w - rightW

	chatContent := a.renderChatPanel(centerW-2, panelH)
	centerSty := panelCenterStyle
	centerRendered := centerSty.Width(centerW).Height(panelH - 2).Render(chatContent)

	var panelsRow string
	if a.showInfoPanel {
		infoContent := a.renderInfoPanel(rightW)
		rightSty := panelRightStyle
		rightRendered := rightSty.Width(rightW).Height(panelH - 2).Render(infoContent)
		panelsRow = lipgloss.JoinHorizontal(lipgloss.Top, centerRendered, rightRendered)
	} else {
		panelsRow = centerRendered
	}

	// Input box (full width)
	var inputRendered string
	if a.streaming {
		inputContent := fmt.Sprintf(" %s %s", a.spinner.View(), a.statusMsg)
		inputRendered = inputBoxStyle.Width(w - 2).Render(inputContent)
	} else {
		a.input.SetWidth(w - 6)
		inputLine := a.input.View()
		inputRendered = inputBoxStyle.Width(w - 2).Render(inputLine)
	}

	// Assemble
	var b strings.Builder
	b.WriteString(headerLine)
	b.WriteString("\n")
	b.WriteString(subLine)
	b.WriteString("\n")
	b.WriteString(panelsRow)
	if suggestionText != "" {
		b.WriteString("\n")
		b.WriteString(suggestionText)
	}
	b.WriteString("\n")
	b.WriteString(inputRendered)
	b.WriteString("\n")
	b.WriteString(bottomBar)

	return b.String()
}

// renderFilePanel builds the left panel content (file explorer + bookmarks).
// renderChatPanel builds the center panel (viewport + input box).
// welcomeLogo is shown in the chat panel before the first message.
const welcomeLogo = ` _       _       _       _       _  
|_ _  _ |_ _  _ |_ _  _ |_ _  _ |_ 
  | ||_   _||_   _||_   _||_   _||  
  _|  _|  _|  _|  _|  _|  _|  _|   
                                    
       mini-wiki v2.0
   Your local AI research assistant
                                     
/rank - Agentic ranking (fast)
/scan - Index project files
/help - All commands`

func (a *Application) renderChatPanel(width, totalHeight int) string {
	var content strings.Builder

	vpHeight := totalHeight
	if vpHeight < 3 {
		vpHeight = 3
	}

	a.viewport.Width = width
	a.viewport.Height = vpHeight

	if a.showWelcome {
		logoLines := strings.Split(welcomeLogo, "\n")
		visible := logoLines
		if len(visible) > vpHeight {
			visible = visible[:vpHeight]
		}
		emptyBefore := (vpHeight - len(visible)) / 2
		for i := 0; i < emptyBefore; i++ {
			content.WriteString("\n")
		}
		for _, line := range visible {
			padding := (width - len(line)) / 2
			if padding < 0 {
				padding = 0
			}
			content.WriteString(strings.Repeat(" ", padding))
			content.WriteString(line)
			content.WriteString("\n")
		}
	} else {
		content.WriteString(a.viewport.View())
		content.WriteString("\n")
	}

	return content.String()
}

// renderInfoPanel builds the right panel (session, model, tasks, history).
func (a *Application) renderInfoPanel(width int) string {
	var content strings.Builder

	content.WriteString(panelHeaderStyle.Render("SESSION"))
	content.WriteString("\n")
	content.WriteString(fmt.Sprintf("  %s %s\n", infoLabelStyle.Render("M:"), infoValueStyle.Render(a.models.Active())))
	chain := a.models.ActiveChain()
	if len(chain) > 1 {
		content.WriteString(fmt.Sprintf("  %s %s\n", infoLabelStyle.Render("F:"), infoValueStyle.Render(chain[1])))
	}
	ctxPct := "0%"
	if a.thread != nil && a.thread.MaxTokens > 0 {
		pct := a.thread.EstimatedTokens() * 100 / a.thread.MaxTokens
		ctxPct = fmt.Sprintf("%d%%", pct)
	}
	content.WriteString(fmt.Sprintf("  %s %s\n", infoLabelStyle.Render("Ctx:"), infoValueStyle.Render(ctxPct)))
	content.WriteString(fmt.Sprintf("  %s %s\n", infoLabelStyle.Render("Tok:"), infoValueStyle.Render(fmt.Sprintf("%d", a.estimatedTokens))))

	content.WriteString("\n")
	content.WriteString(panelHeaderStyle.Render("TASKS"))
	content.WriteString("\n")
	if len(a.tasks) == 0 {
		content.WriteString("  none\n")
	} else {
		maxShow := 4
		for i, t := range a.tasks {
			if i >= maxShow {
				break
			}
			mark := " "
			if t.done {
				mark = "x"
			}
			label := t.text
			if len(label) > width-6 {
				label = label[:width-9] + "..."
			}
			content.WriteString(fmt.Sprintf("  [%s] %s\n", mark, label))
		}
	}

	content.WriteString("\n")
	content.WriteString(panelHeaderStyle.Render("HX"))
	content.WriteString("\n")
	if len(a.actionHistory) == 0 {
		content.WriteString("  none\n")
	} else {
		maxShow := 4
		start := 0
		if len(a.actionHistory) > maxShow {
			start = len(a.actionHistory) - maxShow
		}
		for _, act := range a.actionHistory[start:] {
			label := act
			if len(label) > width-4 {
				label = label[:width-7] + "..."
			}
			content.WriteString(fmt.Sprintf("  %s\n", label))
		}
	}

	return content.String()
}

// shortName shortens a path for display within maxLen.
func shortName(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	// Try to show last segments
	parts := strings.Split(path, "/")
	if len(parts) <= 3 {
		return "..." + path[len(path)-maxLen+3:]
	}
	short := strings.Join(parts[len(parts)-3:], "/")
	if len(short) <= maxLen {
		return ".../" + short
	}
	return "..." + short[len(short)-maxLen+3:]
}

// --- View helpers ---

func (a *Application) appendToViewport(content string) {
	a.viewportContent += content
	a.viewport.SetContent(a.viewportContent)
	a.viewport.GotoBottom()
}

// appendLine appends a complete message, ensuring it ends with a newline
// so each message appears on its own line in the chat viewport.
func (a *Application) appendLine(content string) {
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	a.appendToViewport(content)
}

// highlightSelection modifies the viewport content to show visible
// highlighting on the lines being selected by the user's mouse drag.
func formatUserMsg(content string) string {
	return "\n" + userMsgStyle.Render("You: " + content) + "\n"
}

// formatAssistantMsg formats the assistant's response header with the model name.
func formatAssistantMsg(model string) string {
	return assistantHeaderStyle.Render(model + ":")
}

// --- Commands (tea.Cmd factories) ---

func refreshModelsCmd(client ollama.Client, mm *modelmgr.Manager) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := mm.Refresh(ctx); err != nil {
			return ModelsFailed{Err: err}
		}
		return ModelsRefreshed{Count: len(mm.Available())}
	}
}

func streamChatCmd(client ollama.Client, mm *modelmgr.Manager, thread *conversation.Thread) tea.Cmd {
	return func() tea.Msg {
		chain := mm.ActiveChain()
		if len(chain) == 0 {
			return StreamError{Err: fmt.Errorf("no models available — try /refresh")}
		}

		var lastErr error

		for _, model := range chain {
			req := ollama.ChatRequest{
				Model:    model,
				Messages: convertMessages(thread),
				Stream:   true,
				Options: map[string]any{
					"temperature": 0.7,
				},
			}

			// Context with 30min timeout. Cancel is passed to the reader
			// via StreamStarted.CancelCtx and called when the stream completes.
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)

			stream, err := client.ChatStream(ctx, req)
			if err != nil {
				cancel() // explicitly cancel failed attempts
				lastErr = err
				continue
			}

			// Switch to the working model
			if model != mm.Active() {
				_ = mm.SetActive(model)
			}

			var sb strings.Builder
			return StreamStarted{
				Stream:    stream,
				Model:     model,
				Content:   sb,
				CancelCtx: cancel, // reader will call this when stream ends
			}
		}

		return StreamError{Err: fmt.Errorf("all models failed: %v", lastErr)}
	}
}

func readNextChunkCmd(stream <-chan ollama.ChatStreamChunk, model string, sb *strings.Builder) tea.Cmd {
	return func() tea.Msg {
		chunk, ok := <-stream
		if !ok {
			return StreamDone{FullText: sb.String(), Model: model}
		}
		if chunk.Err != nil {
			return StreamError{Err: chunk.Err}
		}
		if chunk.Done {
			return StreamDone{FullText: sb.String(), Model: model}
		}
		sb.WriteString(chunk.Message.Content)
		return StreamChunk{Text: chunk.Message.Content}
	}
}

func convertMessages(thread *conversation.Thread) []ollama.Message {
	msgs := make([]ollama.Message, 0)

	if thread.System != "" {
		msgs = append(msgs, ollama.Message{
			Role:    "system",
			Content: thread.System,
		})
	}

	for _, msg := range thread.Messages {
		msgs = append(msgs, ollama.Message{
			Role:    string(msg.Role),
			Content: msg.Content,
		})
	}

	return msgs
}

// --- Phase 2: File system command factories ---

func wizardCmd() tea.Cmd {
	return func() tea.Msg {
		var b strings.Builder
		b.WriteString("=== System Check ===\n\n")

		// Go version
		b.WriteString("Go version: ")
		cmd := exec.Command("go", "version")
		out, err := cmd.Output()
		if err == nil {
			b.WriteString(strings.TrimSpace(string(out)))
		} else {
			b.WriteString("not found")
		}
		b.WriteString("\n")

		// Python version
		b.WriteString("Python: ")
		for _, name := range []string{"python3", "python"} {
			cmd := exec.Command(name, "--version")
			out, err := cmd.Output()
			if err == nil {
				b.WriteString(strings.TrimSpace(string(out)))
				break
			}
			b.WriteString("not found")
		}
		b.WriteString("\n")

		// Ollama
		b.WriteString("Ollama: ")
		cmd = exec.Command("ollama", "--version")
		out, err = cmd.Output()
		if err == nil {
			b.WriteString(strings.TrimSpace(string(out)))
			// Check if running
			cmd = exec.Command("sh", "-c", "curl -s http://127.0.0.1:11434/api/tags 2>/dev/null | head -c 1")
			if runOut, _ := cmd.Output(); len(runOut) > 0 {
				b.WriteString(" (running)")
			} else {
				b.WriteString(" (not running)")
			}
		} else {
			b.WriteString("not installed")
		}
		b.WriteString("\n")

		// Check key Python packages
		b.WriteString("\nPython packages:\n")
		for _, pkg := range []string{"chromadb", "ollama", "unstructured", "pypdf"} {
			cmd := exec.Command("sh", "-c", fmt.Sprintf("%s -c \"import %s\" 2>/dev/null && echo 'ok' || echo 'missing'", "python3", pkg))
			out, _ := cmd.Output()
			status := strings.TrimSpace(string(out))
			mark := " [x]"
			if status != "ok" {
				mark = " [ ]"
			}
			b.WriteString(fmt.Sprintf("  %s %s\n", mark, pkg))
		}

		// ChromaDB rag dir
		b.WriteString("\nRAG storage: ")
		home, _ := os.UserHomeDir()
		ragDir := filepath.Join(home, ".wiki", "rag")
		if _, err := os.Stat(ragDir); err == nil {
			b.WriteString(ragDir + " (exists)")
		} else {
			b.WriteString("no RAG index found (run /ingest first)")
		}
		b.WriteString("\n")

		b.WriteString("\n=== Recommendations ===\n")
		// Check if models are pulled
		b.WriteString("Recommended: ollama pull nomic-embed-text\n")
		b.WriteString("Recommended: ollama pull gemma4:e4b\n")
		b.WriteString("First time: bash setup.sh\n")

		return WizardComplete{Text: b.String()}
	}
}

func scanCmd(scanner filescanner.Scanner, cfg filescanner.ScannerConfig) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result, err := scanner.Scan(ctx, cfg)
		if err != nil {
			return ScanFailed{Err: err}
		}
		return ScanComplete{Result: result}
	}
}

// humanBytes returns a human-readable byte size string.
func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// formatFileTree returns a formatted list of scanned files, grouped by type.
func formatFileTree(result *filescanner.ScanResult) string {
	if result == nil || len(result.Files) == 0 {
		return "No files found."
	}

	// Group by type
	typeGroups := make(map[filescanner.FileType][]filescanner.FileInfo)
	for _, f := range result.Files {
		typeGroups[f.FileType] = append(typeGroups[f.FileType], f)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Workspace: %s\n", result.Root))
	b.WriteString(fmt.Sprintf("Total: %d files, %s\n\n", len(result.Files), humanBytes(result.Total)))

	// Show CSV files first (most relevant)
	if csvs, ok := typeGroups[filescanner.FileTypeCSV]; ok {
		b.WriteString("CSV Files:\n")
		for _, f := range csvs {
			rel, _ := filepath.Rel(result.Root, f.Path)
			b.WriteString(fmt.Sprintf("  - %s (%s)\n", rel, humanBytes(f.Size)))
		}
		b.WriteString("\n")
	}

	// Other text files
	ordered := []filescanner.FileType{
		filescanner.FileTypeMarkdown, filescanner.FileTypeJSON, filescanner.FileTypeYAML,
		filescanner.FileTypeXML, filescanner.FileTypeHTML, filescanner.FileTypeGo,
		filescanner.FileTypePython, filescanner.FileTypeJavaScript, filescanner.FileTypeTypeScript,
		filescanner.FileTypeShell, filescanner.FileTypeSQL, filescanner.FileTypeConfig,
		filescanner.FileTypeText,
	}
	for _, ft := range ordered {
		if files, ok := typeGroups[ft]; ok {
			b.WriteString(fileTypeName(ft) + ":\n")
			for _, f := range files {
				rel, _ := filepath.Rel(result.Root, f.Path)
				b.WriteString(fmt.Sprintf("  - %s (%s)\n", rel, humanBytes(f.Size)))
			}
			b.WriteString("\n")
		}
	}

	// Skipped summary
	if len(result.Skipped) > 0 {
		reasonCount := make(map[string]int)
		for _, s := range result.Skipped {
			reasonCount[s.Reason]++
		}
		b.WriteString("Skipped:\n")
		for reason, count := range reasonCount {
			b.WriteString(fmt.Sprintf("  - %d %s\n", count, reason))
		}
	}

	return b.String()
}

// fileTypeName returns a human-readable name for a FileType.
func fileTypeName(ft filescanner.FileType) string {
	switch ft {
	case filescanner.FileTypeText:
		return "Text"
	case filescanner.FileTypeMarkdown:
		return "Markdown"
	case filescanner.FileTypeCSV:
		return "CSV"
	case filescanner.FileTypeJSON:
		return "JSON"
	case filescanner.FileTypeYAML:
		return "YAML"
	case filescanner.FileTypeXML:
		return "XML"
	case filescanner.FileTypeHTML:
		return "HTML"
	case filescanner.FileTypeGo:
		return "Go"
	case filescanner.FileTypePython:
		return "Python"
	case filescanner.FileTypeJavaScript:
		return "JavaScript"
	case filescanner.FileTypeTypeScript:
		return "TypeScript"
	case filescanner.FileTypeShell:
		return "Shell"
	case filescanner.FileTypeSQL:
		return "SQL"
	case filescanner.FileTypeConfig:
		return "Config"
	case filescanner.FileTypeMakefile:
		return "Makefile"
	case filescanner.FileTypeDockerfile:
		return "Dockerfile"
	default:
		return "Other"
	}
}

// --- Phase 3: Command factories ---

func fetchCmd(f webfetch.Fetcher, rawURL string, cfg webfetch.FetcherConfig) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result, err := f.FetchText(ctx, rawURL, cfg)
		if err != nil {
			return FetchFailed{Err: err}
		}
		return FetchComplete{Result: result}
	}
}

// exportDatasetCmd exports the active dataset as-is (no ranking).
// Falls back to conversation export if no dataset is available.
func exportDatasetCmd(a *Application, msg ExportRequested) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Try to load the active dataset
		projectDir := a.pkb.ProjectDir()
		data, err := ranking.LoadDataset(projectDir)
		if err != nil || data == nil || data.RowCount == 0 {
			// Fall back to conversation export
			return exportCmd(a.exporter, a.thread, a.exportCfg, msg)()
		}

		cfg := a.exportCfg
		cfg.Format = msg.Format
		if msg.Output != "" {
			cfg.FileName = msg.Output
		}

		// Build export columns from dataset columns
		columns := make([]export.ColumnDef, len(data.Columns))
		for i, col := range data.Columns {
			colType := "text"
			switch col.Kind {
			case dataset.ColumnInteger:
				colType = "number"
			case dataset.ColumnFloat:
				colType = "number"
			case dataset.ColumnBoolean:
				colType = "boolean"
			}
			width := len(col.Name) + 5
			if width < 15 {
				width = 15
			}
			columns[i] = export.ColumnDef{Name: col.Name, Type: colType, Width: width}
		}

		// Build rows from dataset
		rows := make([]export.Row, len(data.Rows))
		for i, row := range data.Rows {
			r := make(export.Row, len(data.Columns))
			for j, col := range data.Columns {
				val := fmt.Sprintf("%v", row.Data[col.Name])
				r[j] = val
			}
			rows[i] = r
		}

		exportData := &export.ExportData{
			SheetName: data.Name,
			Columns:   columns,
			Rows:      rows,
		}

		result, err := a.exporter.Export(ctx, exportData, cfg)
		if err != nil {
			return ExportFailed{Err: err}
		}
		return ExportComplete{Result: result}
	}
}

func exportCmd(e export.Exporter, thread *conversation.Thread, cfg export.ExportConfig, msg ExportRequested) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Apply request settings
		cfg.Format = msg.Format
		if msg.Output != "" {
			cfg.FileName = msg.Output
		}

		rows := make([]export.Row, 0, len(thread.Messages))
		for _, m := range thread.Messages {
			rows = append(rows, export.Row{
				string(m.Role),
				m.Content,
				m.Timestamp.Format(time.RFC3339),
			})
		}

		data := &export.ExportData{
			SheetName: "Conversation",
			Columns: []export.ColumnDef{
				{Name: "Role", Type: "text", Width: 12},
				{Name: "Content", Type: "text", Width: 60},
				{Name: "Timestamp", Type: "text", Width: 24},
			},
			Rows: rows,
		}

		result, err := e.Export(ctx, data, cfg)
		if err != nil {
			return ExportFailed{Err: err}
		}
		return ExportComplete{Result: result}
	}
}

func exportRankedCmd(a *Application, msg ExportRequested) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		cfg := a.exportCfg
		cfg.Format = msg.Format
		if msg.Output != "" {
			cfg.FileName = msg.Output
		}

		if a.currentRank == nil || a.currentRank.Dataset == nil {
			return ExportFailed{Err: fmt.Errorf("no ranked data available. Run /rank first.")}
		}

		ds := a.currentRank.Dataset
		cols := make([]export.ColumnDef, len(ds.Columns))
		for i, c := range ds.Columns {
			t := "text"
			if c.Kind == dataset.ColumnInteger || c.Kind == dataset.ColumnFloat {
				t = "number"
			}
			cols[i] = export.ColumnDef{Name: c.Name, Type: t, Width: 15}
		}

		rows := make([]export.Row, len(ds.Rows))
		for i, r := range ds.Rows {
			row := make(export.Row, len(ds.Columns))
			for j, c := range ds.Columns {
				val := fmt.Sprintf("%v", r.Data[c.Name])
				row[j] = val
			}
			rows[i] = row
		}

		data := &export.ExportData{
			SheetName: "RankedData",
			Columns:   cols,
			Rows:      rows,
		}

		result, err := a.exporter.Export(ctx, data, cfg)
		if err != nil {
			return ExportFailed{Err: err}
		}
		return ExportComplete{Result: result}
	}
}

func truncateText(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// --- Man-page help system ---

// helpSummary returns a concise overview like 'command --help' in a terminal.
func helpSummary() string {
	return `NAME
    mini-wiki - Standalone TUI Dataset RAG Analysis Tool

USAGE
    wiki [options]

OPTIONS
    --ollama <url>  Custom Ollama endpoint (default: http://127.0.0.1:11434)
    --no-start      Fail if Ollama is not running (don't auto-start)
    --select        Inline mode (allows native text selection with mouse)

COMMANDS
    /help [command]  Show this summary or a man page for a specific command

  DATA
    /ingest <path>   Read a file into context and register as active dataset
    /export [opts]   Export dataset to xlsx/csv/json
    /infer <file>    Auto-detect a file's format

  RANKING
    /rank <topic>        Rank dataset by relevance (Agentic AI, fast)
    /compare [<topic>]   Compare rankings with a refined topic
    /discard <threshold> Remove rows below a relevance score

  VISUALIZATION
    /chart <type> <args>  Generate charts (bar, trend, pie, scatter, etc.)

  RAG
    /embed              Embed active dataset for semantic search (slow)

  SYSTEM
    /model <name>   Switch the active LLM model
    /models         List all available models
    /refresh        Reload the model list from Ollama
    /clear          Clear the conversation
    /system <text>  Set a new system prompt
    /wizard         Run an interactive system check / setup
    /panel          Toggle the right info panel

  UTILITY
    /bookmark <title>  Save the current finding
    /bookmarks         List all saved bookmarks
    /history           Show recent query history
    /skills            List all tool capabilities
    /flaws             Show known issues and solutions
    /task <desc>       Add a todo task
    /tasks             List all tasks
    /clip              Copy the viewport text to the system clipboard
    /exit              Quit the application

EXAMPLES
    wiki
    wiki --ollama http://192.168.1.100:11434
    wiki --no-start --select

SEE ALSO
    /help rank     — Full documentation for the /rank command
    /help chart    — Full documentation for the /chart command
    /help ingest   — Full documentation for the /ingest command
    /help export   — Full documentation for the /export command
    /help embed    — Full documentation for the /embed command`
}

// manPages contains detailed man-page documentation for each command.
var manPages = map[string]string{

	"rank": `NAME
    /rank — Rank dataset rows by relevance to a research topic

SYNOPSIS
    /rank <topic>

DESCRIPTION
    Uses Agentic RAG: instead of scoring every row via the LLM (slow),
    the tool sends the dataset schema + 3 sample rows to the coder model
    (qwen2.5-coder:7b). The LLM writes a Pandas filter_data(df) function.
    The function executes locally in a sandboxed environment, scoring all
    rows at once with vectorized operations.

    No row-by-row LLM calls. 92,000 rows ranked in seconds.

BEHAVIOR
    1. The active dataset is loaded from the project KB
    2. The Python RAG worker starts if not already running
    3. Worker loads the data with Pandas (auto-detects CSV/JSONL/XLSX)
    4. Worker extracts the schema (column names + types) + first 3 rows
    5. Worker prompts qwen2.5-coder:7b for a Pandas filter script
    6. The generated filter_data(df) function executes in a sandbox
       (only pandas, numpy, json available — no os/sys/subprocess)
    7. Worker returns filtered rows with a relevance_score column (1-100)
    8. Go TUI displays a ranked table with scores

DISPLAY
    Rank | Score | Row | Column1   | Column2   | ...
    1    | 0.95  | #42 | value     | value     | ...

EXAMPLE
    /rank studies about neural networks in medical imaging
    /rank failed CI commits in the last quarter
    /rank security vulnerabilities in authentication modules

SEE ALSO
    /compare, /discard, /export --ranked, /help ingest`,

	"compare": `NAME
    /compare — Re-rank with a refined topic and compare results

SYNOPSIS
    /compare
    /compare <refined topic>

DESCRIPTION
    After an initial /rank, you can refine your research topic and
    re-rank. The tool runs a new agentic ranking and displays a
    side-by-side comparison with score deltas.

BEHAVIOR
    Without arguments: shows the current ranking again.
    With a topic: runs agentic ranking with the new topic and displays:
      Previous ranking          |  New ranking
      1. neural imaging (0.95)  |  1. MRI preprocessing (0.97)
      2. deep learning (0.88)   |  2. CNN architecture (0.92)
                                  Delta: +0.05, -0.10

EXAMPLE
    /compare
    /compare transformer models for code completion
    /compare rust vs go security

SEE ALSO
    /rank, /discard`,

	"discard": `NAME
    /discard — Remove rows below a relevance threshold

SYNOPSIS
    /discard <threshold>
    /discard --preview <threshold>
    /discard --reset

DESCRIPTION
    After ranking, you can discard rows with a relevance_score below
    a threshold (0.0 to 1.0). A preview shows how many rows will be
    kept vs. discarded before asking for confirmation.

    Discarded rows are NOT deleted from the original file. They are
    only removed from the active working set in memory. Use --reset
    to restore all previously discarded rows.

BEHAVIOR
    /discard 0.3       — Preview + confirm discarding rows below 0.3
    /discard --preview — Show preview without asking confirmation
    /discard --reset   — Restore all previously discarded rows

EXAMPLE
    /discard 0.5
    /discard --preview 0.3
    /discard --reset

SEE ALSO
    /rank, /compare, /export --ranked`,

	"chart": `NAME
    /chart — Generate charts from the active dataset

SYNOPSIS
    /chart bar column=<col>
    /chart trend column=<col>
    /chart pie column=<col>
    /chart scatter x=<col> y=<col>
    /chart histogram column=<col> buckets=<n>
    /chart box column=<col>
    /chart heatmap x=<col> y=<col>

DESCRIPTION
    Renders a chart in the terminal (ASCII) and optionally exports
    a PNG/SVG file. Charts use the active (post-rank) dataset.

    When --export is added, saves as PNG/SVG to the current directory.

EXAMPLE
    /chart bar column=status
    /chart trend column=score
    /chart pie column=category
    /chart scatter x=age y=salary
    /chart histogram column=relevance buckets=10
    /chart trend column=relevance --export`,

	"ingest": `NAME
    /ingest — Read a file into context and register as the active dataset

SYNOPSIS
    /ingest @<file>

DESCRIPTION
    Parses the file, counts rows, and registers it as the active dataset
    for /rank, /chart, /export, and /embed commands. The file is NOT
    embedded for RAG search until /embed is called.

    Supports: CSV, TSV, JSONL, JSON, XLSX, ODS, and plain text.
    Format is auto-detected by file extension.

EXAMPLE
    /ingest @data.csv
    /ingest @datasets/SWE_Next_dataset.jsonl
    /ingest @../research/data.xlsx

SEE ALSO
    /rank, /chart, /export, /embed, /infer`,

	"export": `NAME
    /export — Export the active dataset to a file

SYNOPSIS
    /export [--format xlsx|csv|json] [--output <path>] [--ranked]

DESCRIPTION
    Exports the active (post-rank, post-discard) dataset to a file.
    Default format is xlsx (Excel). Column types are auto-detected
    and formatted correctly.

    Use --ranked to include the relevance_score column and sort by
    score descending. Requires a prior /rank.

EXAMPLE
    /export
    /export --format csv
    /export --format json
    /export --ranked
    /export --ranked --format csv --output ~/results.csv

SEE ALSO
    /rank, /ingest`,

	"embed": `NAME
    /embed — Embed the active dataset for RAG semantic search

SYNOPSIS
    /embed

DESCRIPTION
    Sends the dataset to the Python RAG worker for vector embedding.
    Once embedded, you can ask free-form questions about the data and
    the tool will retrieve relevant chunks via ChromaDB.

    For large datasets (1GB+), embedding can take hours. The progress
    is displayed in real-time. Press Escape to cancel at any time.

    NOTE: /embed is OPTIONAL. /rank, /chart, and /export do NOT
    require embedding — they work immediately after /ingest.

EXAMPLE
    /embed

SEE ALSO
    /ingest, /kbstatus, /kbquery`,

	"model": `NAME
    /model — Switch the active LLM model

SYNOPSIS
    /model <name>

DESCRIPTION
    Switches the active model used for chat and ranking code generation.
    The model name is matched case-insensitively (partial match).

    Models recommended:
      deepseek-r1:8b    — Research reasoning, deep dataset synthesis
      qwen2.5-coder:7b  — Code generation for Agentic Ranking
      gemma4:e4b        — Default chat model (131K context, thinking enabled)
      gemma4:e4b        — Logic-heavy tasks

EXAMPLE
    /model deepseek-r1:8b
    /model qwen2.5-coder:7b

SEE ALSO
    /models, /refresh, /help wizard`,

	"wizard": `NAME
    /wizard — Run system check and interactive setup

SYNOPSIS
    /wizard

DESCRIPTION
    Checks the system for: Python version, Ollama availability,
    installed models, pip dependencies. Helps you set up the
    environment if anything is missing.

EXAMPLE
    /wizard`,

	"panel": `NAME
    /panel — Toggle the right info panel

SYNOPSIS
    /panel

DESCRIPTION
    Hides or shows the right info panel (SESSION, TASKS, HX).
    When hidden, the chat panel uses the full terminal width.

EXAMPLE
    /panel`,

	"clip": `NAME
    /clip — Copy the chat viewport text to the system clipboard

SYNOPSIS
    /clip

DESCRIPTION
    Copies the entire viewport content to the system clipboard.
    You can also click-and-drag with the mouse to select specific
    lines — on release, the selected text is copied automatically.

EXAMPLE
    /clip`,

	"clear": `NAME
    /clear — Clear the conversation history

SYNOPSIS
    /clear

DESCRIPTION
    Removes all messages from the current conversation while keeping
    the system prompt. The conversation thread is preserved but empty.

EXAMPLE
    /clear`,

	"system": `NAME
    /system — Set a custom system prompt for the LLM

SYNOPSIS
    /system <text>

DESCRIPTION
    Replaces the current system prompt with the provided text.
    The system prompt sets the behavior and personality of the LLM.

EXAMPLE
    /system You are a data science researcher analyzing software repositories.
    /system You are a helpful assistant that speaks like a pirate.`,

	"infer": `NAME
    /infer — Auto-detect a file's format

SYNOPSIS
    /infer @<file>

DESCRIPTION
    Reads the file header and determines the format (CSV, JSONL,
    JSON, XLSX, etc.) without ingesting the data. Useful for
    checking if a file is compatible before running /ingest.

EXAMPLE
    /infer @unknown_data_file`,

	"bookmark": `NAME
    /bookmark — Save the current finding as a bookmark

SYNOPSIS
    /bookmark <title>

DESCRIPTION
    Saves a bookmark with the given title. Bookmarks persist
    across sessions in the project KB (.wiki/kb.sqlite).

EXAMPLE
    /bookmark Important finding about neural networks
    /bookmark Key insight from data analysis`,

	"bookmarks": `NAME
    /bookmarks — List all saved bookmarks

SYNOPSIS
    /bookmarks

DESCRIPTION
    Displays all bookmarks saved via /bookmark, showing title
    and timestamp for each.

EXAMPLE
    /bookmarks`,

	"history": `NAME
    /history — Show recent query history

SYNOPSIS
    /history

DESCRIPTION
    Displays the most recent queries from the current project,
    including timestamps and the model used.

EXAMPLE
    /history`,

	"skills": `NAME
    /skills — List all tool capabilities

SYNOPSIS
    /skills

DESCRIPTION
    Lists all registered tool capabilities and which models
    support them.

EXAMPLE
    /skills`,

	"flaws": `NAME
    /flaws — Show known issues and solutions

SYNOPSIS
    /flaws

DESCRIPTION
    Displays known issues, bugs, and their workarounds from
    the tool memory. Add entries via the memory system.

EXAMPLE
    /flaws`,

	"task": `NAME
    /task — Add a todo task

SYNOPSIS
    /task <description>

DESCRIPTION
    Adds a task to the in-session todo list. Tasks are displayed
    in the right info panel under TASKS.

EXAMPLE
    /task Review the ranked dataset for accuracy
    /task Run /compare with a different topic`,

	"tasks": `NAME
    /tasks — List all todo tasks

SYNOPSIS
    /tasks

DESCRIPTION
    Lists all tasks added via /task, showing completion status.

EXAMPLE
    /tasks`,

	"models": `NAME
    /models — List all available Ollama models

SYNOPSIS
    /models

DESCRIPTION
    Queries Ollama for all available models and displays them
    with sizes. The active model is marked.

EXAMPLE
    /models`,

	"refresh": `NAME
    /refresh — Reload the model list from Ollama

SYNOPSIS
    /refresh

DESCRIPTION
    Re-queries Ollama for the available model list. Useful after
    pulling or removing models while the tool is running.

EXAMPLE
    /refresh`,
}

// --- Command auto-completion ---

// commandList defines all available commands with descriptions for auto-complete hints.
var commandList = []suggestionItem{
	{text: "/help", description: "Show this help message", category: "cmd"},
	{text: "/model", description: "Switch the active LLM model", category: "cmd"},
	{text: "/models", description: "List all available models", category: "cmd"},
	{text: "/refresh", description: "Reload model list from Ollama", category: "cmd"},
	{text: "/clear", description: "Clear conversation history", category: "cmd"},
	{text: "/system", description: "Set a new system prompt", category: "cmd"},
	{text: "/scan", description: "Scan workspace for files", category: "cmd"},
	{text: "/files", description: "List scanned files", category: "cmd"},
	{text: "/ingest", description: "Read a file into context", category: "cmd"},
	{text: "/fetch", description: "Fetch a webpage and extract text", category: "cmd"},
	{text: "/export", description: "Export conversation to .xlsx", category: "cmd"},
	{text: "/bookmark", description: "Save current finding", category: "cmd"},
	{text: "/bookmarks", description: "List saved bookmarks", category: "cmd"},
	{text: "/history", description: "Show recent query history", category: "cmd"},
	{text: "/skills", description: "List tool capabilities", category: "cmd"},
	{text: "/flaws", description: "Show known issues and solutions", category: "cmd"},
	{text: "/task", description: "Add a todo task", category: "cmd"},
	{text: "/tasks", description: "List all tasks", category: "cmd"},
	{text: "/rank", description: "Rank dataset rows by relevance to topic", category: "cmd"},
	{text: "/compare", description: "Compare rankings iteratively", category: "cmd"},
	{text: "/discard", description: "Discard rows below relevance threshold", category: "cmd"},
	{text: "/chart", description: "Generate chart (bar, trend, pie, scatter, histogram, box, heatmap)", category: "cmd"},
	{text: "/infer", description: "Auto-detect file format", category: "cmd"},
	{text: "/embed", description: "Embed active dataset for RAG search (slow)", category: "cmd"},
	{text: "/wizard", description: "Run system check and setup assistant", category: "cmd"},
	{text: "/srs", description: "Run SRS generation pipeline", category: "cmd"},
	{text: "/clip", description: "Copy viewport text to clipboard", category: "cmd"},
	{text: "/panel", description: "Toggle right info panel", category: "cmd"},
	{text: "/cancel", description: "Cancel current RAG operation", category: "cmd"},
	{text: "/exit", description: "Quit the application", category: "cmd"},
}

func (a *Application) updateSuggestions() {
	val := a.input.Value()
	a.suggestions = nil
	a.tabIndex = -1

	// If input has @, do file completion (handles both bare @ and /cmd @path)
	if strings.Contains(val, "@") {
		a.suggestAtFiles(val)
		return
	}

	// If typing /, suggest commands
	if strings.HasPrefix(val, "/") {
		for _, cmd := range commandList {
			if strings.HasPrefix(cmd.text, val) {
				a.suggestions = append(a.suggestions, cmd)
			}
		}
		return
	}

	// If typing a path-like string, do file completion
	if strings.HasPrefix(val, ".") || strings.HasPrefix(val, "/") {
		a.suggestAtFiles(val)
	}
}

// suggestAtFiles lists files/dirs from the filesystem like ls, used for @ completion.
func (a *Application) suggestAtFiles(val string) {
	rootDir := a.scanCfg.RootDir
	if rootDir == "" {
		return
	}

	lastToken := extractPathToken(val)
	if lastToken == "" {
		// Bare @: list current directory like ls
		entries, err := os.ReadDir(rootDir)
		if err != nil {
			return
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() {
				a.suggestions = append(a.suggestions, suggestionItem{
					text:        "@" + name + "/",
					description: fmt.Sprintf("dir  %s", name),
					category:    "file",
				})
			} else {
				info, _ := e.Info()
				size := int64(0)
				if info != nil {
					size = info.Size()
				}
				a.suggestions = append(a.suggestions, suggestionItem{
					text:        "@" + name,
					description: fmt.Sprintf("file  %s  (%s)", name, humanBytes(size)),
					category:    "file",
				})
			}
			if len(a.suggestions) >= 20 {
				rem := "more..."
				a.suggestions = append(a.suggestions, suggestionItem{
					text:        "...",
					description: rem,
					category:    "file",
				})
				break
			}
		}
		return
	}

	// Has a token: list directory contents filtered by name (like ls | grep)
	// Split into directory path and filter
	dirPath := rootDir
	filter := lastToken

	if strings.Contains(lastToken, "/") {
		// e.g. "subdir/filter" or "subdir/"
		idx := strings.LastIndex(lastToken, "/")
		dirPart := lastToken[:idx]
		filter = lastToken[idx+1:]
		dirPath = filepath.Join(rootDir, dirPart)
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return
	}

	lowerFilter := strings.ToLower(filter)
	for _, e := range entries {
		name := e.Name()
		lowerName := strings.ToLower(name)

		// If filter is empty or name contains filter text
		if filter == "" || strings.Contains(lowerName, lowerFilter) {
			relPath := lastToken
			if strings.HasSuffix(relPath, "/") {
				relPath = relPath + name
			} else if filter != "" && !strings.HasSuffix(relPath, name) {
				// Replace the filter portion with the full name
				idx := strings.LastIndex(relPath, filter)
				if idx >= 0 {
					relPath = relPath[:idx] + name
				} else {
					relPath = lastToken[:strings.LastIndex(lastToken, "/")+1] + name
				}
			} else {
				relPath = name
			}

			if e.IsDir() {
				a.suggestions = append(a.suggestions, suggestionItem{
					text:        "@" + relPath + "/",
					description: fmt.Sprintf("dir  %s", relPath+"/"),
					category:    "file",
				})
			} else {
				info, _ := e.Info()
				size := int64(0)
				if info != nil {
					size = info.Size()
				}
				a.suggestions = append(a.suggestions, suggestionItem{
					text:        "@" + relPath,
					description: fmt.Sprintf("file  %s  (%s)", relPath, humanBytes(size)),
					category:    "file",
				})
			}
			if len(a.suggestions) >= 20 {
				break
			}
		}
	}
}

// extractPathToken pulls the last @reference or path-like token from input.
func extractPathToken(val string) string {
	// If there's an @, get everything after the last @
	if idx := strings.LastIndex(val, "@"); idx >= 0 {
		return val[idx+1:]
	}
	// If the input starts with ., /, or looks like a file path
	fields := strings.Fields(val)
	if len(fields) > 0 {
		last := fields[len(fields)-1]
		if strings.Contains(last, ".") || strings.Contains(last, "/") || strings.HasPrefix(last, ".") || strings.HasPrefix(last, "/") {
			return last
		}
	}
	return ""
}

func (a *Application) cycleSuggestion(val string) {
	// Rebuild suggestions if empty or input changed
	needsRebuild := false
	if len(a.suggestions) == 0 {
		needsRebuild = true
	} else if a.tabIndex >= 0 && a.tabIndex < len(a.suggestions) {
		if a.suggestions[a.tabIndex].text != val && !strings.HasPrefix(val, a.suggestions[a.tabIndex].text) {
			needsRebuild = true
		}
	} else {
		needsRebuild = true
	}

	if needsRebuild {
		a.updateSuggestions()
		if len(a.suggestions) == 0 {
			return
		}
		a.tabIndex = -1
	}

	if len(a.suggestions) == 0 {
		return
	}

	// Cycle to next suggestion
	a.tabIndex++
	if a.tabIndex >= len(a.suggestions) {
		a.tabIndex = 0
	}

	// Fill the input with the selected suggestion
	selected := a.suggestions[a.tabIndex]
	a.applySuggestion(selected.text)
}

// applySuggestion replaces the current input with the suggestion,
// preserving any command prefix before the @ sign.
func (a *Application) applySuggestion(suggestion string) {
	val := a.input.Value()

	// For @file completions: preserve everything before the @
	if strings.HasPrefix(suggestion, "@") {
		if idx := strings.LastIndex(val, "@"); idx >= 0 {
			prefix := val[:idx]
			a.input.SetValue(prefix + suggestion + " ")
			a.input.SetCursor(len(prefix) + len(suggestion) + 1)
			return
		}
	}

	// For command completions: preserve the leading /
	if strings.HasPrefix(suggestion, "/") && strings.HasPrefix(val, "/") {
		a.input.SetValue(suggestion + " ")
		a.input.SetCursor(len(suggestion) + 1)
		return
	}

	// Fallback: replace entire input
	a.input.SetValue(suggestion + " ")
	a.input.SetCursor(len(suggestion) + 1)
}

func (a *Application) clearSuggestions() {
	a.suggestions = nil
	a.tabIndex = -1
}

func formatSuggestions(suggestions []suggestionItem, selected int) string {
	if len(suggestions) == 0 {
		return ""
	}
	var b strings.Builder
	for i, s := range suggestions {
		if i > 0 {
			b.WriteString(" | ")
		}
		marker := "  "
		if i == selected {
			marker = ">"
		}
		if s.description != "" {
			b.WriteString(fmt.Sprintf("%s %s - %s", marker, s.text, s.description))
		} else {
			b.WriteString(fmt.Sprintf("%s %s", marker, s.text))
		}
	}
	return b.String()
}

// logAction records an action in the history ring buffer (max 20).
func (a *Application) logAction(action string) {
	a.actionHistory = append(a.actionHistory, action)
	if len(a.actionHistory) > 20 {
		a.actionHistory = a.actionHistory[len(a.actionHistory)-20:]
	}
}

// updateTokenCount estimates and stores the current token count from the thread.
func (a *Application) updateTokenCount() {
	if a.thread == nil {
		a.estimatedTokens = 0
		return
	}
	a.estimatedTokens = a.thread.EstimatedTokens()
}

// registerBuiltinSkills registers the tool's built-in capabilities in tool memory.
func (a *Application) registerBuiltinSkills() {
	skills := []memory.Skill{
		{Name: "Chat", Description: "Conversational AI assistant with local LLM", Command: "/chat", Category: "system", Models: []string{"qwen2.5-coder", "gemma4:e4b"}},
		{Name: "File Scanner", Description: "Scan workspace directory for files", Command: "/scan", Category: "data", Models: nil},
		{Name: "File Reference", Description: "Reference files with @filename in chat", Command: "@filename", Category: "data", Models: nil},
		{Name: "Web Fetch", Description: "Fetch webpage content and extract text", Command: "/fetch", Category: "data", Models: nil, Note: "SSRF-safe, blocks private IPs"},
		{Name: "Export", Description: "Export conversation to .xlsx", Command: "/export", Category: "export", Models: nil},
		{Name: "Bookmarks", Description: "Save and browse important findings", Command: "/bookmark", Category: "system", Models: nil},
		{Name: "History", Description: "Browse query history", Command: "/history", Category: "system", Models: nil},
		{Name: "SRS Pipeline", Description: "Full SRS generation: FR/NFR, MoSCoW, DFD, CSPEC, IEEE 830", Command: "/srs", Category: "srs", Models: []string{"qwen2.5-coder", "gemma4"}, Parameters: "temperature: 0.1"},
	}

	for _, skill := range skills {
		_ = a.mem.RegisterSkill(skill)
	}
}

// --- SRS Pipeline ---

// srsLLMAdapter adapts ollama.Client to srs.LLMClient.
type srsLLMAdapter struct {
	client ollama.Client
}

func (a *srsLLMAdapter) Generate(ctx context.Context, model, prompt string) (string, error) {
	resp, err := a.client.Generate(ctx, ollama.GenerateRequest{
		Model:  model,
		Prompt: prompt,
		Stream: false,
		Options: map[string]any{
			"temperature": 0.1,
		},
	})
	if err != nil {
		return "", err
	}
	return resp.Response, nil
}

func rankCmd(a *Application, topic string) tea.Cmd {
	return func() tea.Msg {
		projectDir := a.pkb.ProjectDir()

		// Load dataset metadata from project KB
		data, err := ranking.LoadDataset(projectDir)
		if err != nil {
			return RankFailed{Err: err}
		}

		// Ensure the RAG worker is running for agentic ranking
		if errMsg := a.ensureRAGStarted(); errMsg != "" {
			return RankFailed{Err: fmt.Errorf("RAG worker unavailable: %s. Try /embed first.", errMsg)}
		}

		// Use a 5-minute timeout for agentic ranking (LLM codegen + Pandas execution)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		ranker := ranking.NewRanker(a.ragClient, ranking.DefaultConfig())
		result, err := ranker.ScoreAll(ctx, data, topic)
		if err != nil {
			return RankFailed{Err: err}
		}

		text := ranking.FormatRankingTable(result, 20)
		return RankComplete{Result: result, Text: text}
	}
}

func compareCmd(a *Application, newTopic string) tea.Cmd {
	return func() tea.Msg {
		if a.currentRank == nil {
			return RankFailed{Err: fmt.Errorf("no previous ranking")}
		}

		// Ensure RAG worker is running
		if errMsg := a.ensureRAGStarted(); errMsg != "" {
			return RankFailed{Err: fmt.Errorf("RAG worker unavailable: %s. Try /embed first.", errMsg)}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		ranker := ranking.NewRanker(a.ragClient, ranking.DefaultConfig())
		newResult, err := ranker.Rerank(ctx, a.currentRank, newTopic)
		if err != nil {
			return RankFailed{Err: err}
		}

		// Build comparison display
		comparison := ranking.FormatComparison(a.currentRank, newResult)
		oldTable := ranking.FormatRankingTable(a.currentRank, 5)
		newTable := ranking.FormatRankingTable(newResult, 5)

		text := fmt.Sprintf("=== Comparison ===\nPrevious topic: %s\nNew topic: %s\n\n%s\n\nOld ranking:\n%s\n\nNew ranking:\n%s",
			a.currentRank.Topic, newTopic, comparison, oldTable, newTable)

		return RankComplete{Result: newResult, Text: text}
	}
}

// discardRowsAtThreshold removes rows below the threshold from the working set.
func (a *Application) discardRowsAtThreshold(result *ranking.RankResult, threshold float64) {
	if result == nil {
		return
	}
	kept := result.Dataset.Filter(func(r dataset.Row) bool {
		if s, ok := r.Data["relevance_score"].(float64); ok {
			return s >= threshold
		}
		return false
	})
	discarded := result.Dataset.RowCount - kept.RowCount
	result.Dataset = kept
	result.DiscardCount += discarded
	_ = a.pkb.SaveDiscardEntry(context.Background(), &projectkb.DiscardEntry{
		Threshold:     threshold,
		RowsDiscarded: discarded,
	})
	a.appendLine(fmt.Sprintf("Discarded %d rows. Remaining: %d rows.", discarded, kept.RowCount))
}

func chartCmd(a *Application, msg ChartRequested) tea.Cmd {
	return func() tea.Msg {
		ds := a.currentRank.Dataset
		cfg := chart.Config{
			Type:    chart.ChartType(msg.Type),
			ColumnX: msg.ColumnX,
			ColumnY: msg.ColumnY,
			Buckets: msg.Buckets,
			Width:   a.width - 10,
			Height:  16,
		}

		// Auto-detect columns if not specified
		if cfg.ColumnX == "" {
			for _, col := range ds.Columns {
				if col.Name != "relevance_score" {
					cfg.ColumnX = col.Name
					break
				}
			}
		}

		c, err := chart.Render(ds, cfg)
		if err != nil {
			return ChartFailed{Err: err}
		}
		return ChartComplete{Text: c.Terminal}
	}
}

func srsPipelineCmd(pipeline *srs.Pipeline, data string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		results, err := pipeline.Run(ctx, data, nil)
		if err != nil {
			return SRSFailed{Err: err}
		}

		return SRSComplete{
			Content: results.SRS,
			RunID:   string(results.RunID),
		}
	}
}

// resolveAtRefsDirect resolves @file references by checking the filesystem directly
// (used when no file index is available, i.e. before /scan).
func (a *Application) resolveAtRefsDirect(content *string, refs []fileref.Reference) {
	rootDir := a.scanCfg.RootDir
	if rootDir == "" {
		return
	}
	var attached []string
	for _, ref := range refs {
		pathOnly, _, _ := extractRefInfo(ref.Raw)
		absPath, err := fileref.SafeResolve(rootDir, pathOnly)
		if err != nil {
			continue
		}
		// Check if it's a text file (basic check: no null bytes in first 512 bytes)
		data, err := os.ReadFile(absPath)
		if err != nil || len(data) == 0 {
			continue
		}
		if len(data) > 10*1024*1024 {
			a.appendLine(fmt.Sprintf("[%s too large (%d MB), showing first 10MB]", pathOnly, len(data)/(1024*1024)))
			data = data[:10*1024*1024]
		}
		*content = *content + "\n\n```" + pathOnly + "\n" + string(data) + "\n```"
		attached = append(attached, pathOnly)
	}
	if len(attached) > 0 {
		a.appendLine(fmt.Sprintf("Attached %d files", len(attached)))
	}
}

// extractRefInfo is a local copy of fileref's internal function to extract path from @ref.
func extractRefInfo(raw string) (path string, line, endLine int) {
	path = strings.TrimPrefix(raw, "@")
	if colonIdx := strings.LastIndex(path, ":"); colonIdx > 0 {
		rest := path[colonIdx+1:]
		if n, err := fmt.Sscanf(rest, "%d-%d", &line, &endLine); err == nil && n >= 1 {
			path = path[:colonIdx]
		} else if n, err := fmt.Sscanf(rest, "%d", &line); err == nil && n == 1 {
			path = path[:colonIdx]
		}
		if endLine == 0 {
			endLine = line
		}
	}
	return path, line, endLine
}

// --- RAG helpers ---

// ensureRAGStarted starts the Python RAG worker if not already running.
// Returns error string if unavailable (empty string means ready).
func (a *Application) ensureRAGStarted() string {
	if a.ragDir == "" {
		return "RAG worker files not extracted"
	}
	if a.ragClient.IsRunning() {
		return ""
	}
	workerPath := filepath.Join(a.ragDir, "main.py")
	if _, err := os.Stat(workerPath); err != nil {
		return fmt.Sprintf("RAG worker not found at %s", workerPath)
	}
	// Build list of Python candidates:
	// 1. .venv/ in the CWD (project root)
	// 2. .venv/ alongside the wiki binary itself
	// 3. .venv/ in the global config directory (~/.config/mini-wiki/)
	// 4. system python3 / python
	rootDir := a.scanCfg.RootDir
	var pythonCandidates []string

	// CWD .venv
	if rootDir != "" {
		pythonCandidates = append(pythonCandidates,
			filepath.Join(rootDir, ".venv", "bin", "python3"),
			filepath.Join(rootDir, ".venv", "bin", "python"),
		)
	}

	// Binary directory .venv (covers case where binary is in project dir)
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		if exeDir != rootDir {
			pythonCandidates = append(pythonCandidates,
				filepath.Join(exeDir, ".venv", "bin", "python3"),
				filepath.Join(exeDir, ".venv", "bin", "python"),
			)
		}
	}

	// Global config directory .venv (covers running wiki from any directory)
	if home, err := os.UserHomeDir(); err == nil {
		configDir := filepath.Join(home, ".config", "mini-wiki")
		pythonCandidates = append(pythonCandidates,
			filepath.Join(configDir, ".venv", "bin", "python3"),
			filepath.Join(configDir, ".venv", "bin", "python"),
		)
	}

	// System fallbacks
	pythonCandidates = append(pythonCandidates, "python3", "python")

	var lastErr error
	var triedPaths []string
	for _, python := range pythonCandidates {
		triedPaths = append(triedPaths, python)
		if err := a.ragClient.Start(python, workerPath, a.pkb.ProjectDir(), "nomic-embed-text", a.models.Active(), "http://127.0.0.1:11434"); err != nil {
			lastErr = err
			continue
		}
		return ""
	}
	// Get detailed error from stderr if available
	details := fmt.Sprintf("(tried: %s)", strings.Join(triedPaths, ", "))
	if errMsg := a.ragClient.LastError(); errMsg != "" {
		// Extract the most useful part of the error
		lines := strings.Split(errMsg, "\n")
		for _, line := range lines {
			if strings.Contains(line, "ModuleNotFoundError") || strings.Contains(line, "Error") {
				return fmt.Sprintf("Python error: %s %s", strings.TrimSpace(line), details)
			}
		}
		if len(lines) > 0 {
			return fmt.Sprintf("Python: %s %s", strings.TrimSpace(lines[len(lines)-1]), details)
		}
	}
	if lastErr != nil {
		return fmt.Sprintf("RAG worker: %v %s", lastErr, details)
	}
	return fmt.Sprintf("RAG worker failed to start (unknown error) %s", details)
}

// queryRAG queries the RAG worker for relevant context, returning sources and answer.
func (a *Application) queryRAG(question string, topK int) (*rag.QueryResult, error) {
	if errMsg := a.ensureRAGStarted(); errMsg != "" {
		return nil, nil // RAG not available, silently skip
	}
	return a.ragClient.Query(question, topK)
}

// ingestLocalCmd parses a file locally and registers it as the active dataset (fast, no embeddings).
func ingestLocalCmd(a *Application, path string) tea.Cmd {
	return func() tea.Msg {
		rootDir := a.scanCfg.RootDir
		if rootDir == "" {
			rootDir, _ = os.Getwd()
		}
		absPath, err := fileref.SafeResolve(rootDir, path)
		if err != nil {
			return IngestFailed{Path: path, Err: err}
		}

		// Detect format by extension
		ext := strings.ToLower(filepath.Ext(absPath))
		format := "txt"
		if ext == ".csv" || ext == ".tsv" {
			format = "csv"
		} else if ext == ".jsonl" || ext == ".ndjson" || ext == ".json" {
			format = "jsonl"
		} else if ext == ".xlsx" {
			format = "xlsx"
		}

		// Count rows by reading through the file
		rowCount := countFileRows(absPath, format)
		_ = a.pkb.SetActiveDataset(context.Background(), absPath, format, rowCount)

		msg := fmt.Sprintf("Ingested %s — %d rows. Ready for /rank.\n  Run /embed to enable RAG search (optional, will take hours).",
			filepath.Base(absPath), rowCount)
		return IngestCompleteMsg{Text: msg}
	}
}

// countFileRows quickly counts the number of rows in a file without parsing fully.
// Uses a buffered reader to count newlines -- handles arbitrarily long lines.
func countFileRows(path, format string) int {
	switch format {
	case "jsonl", "csv", "txt":
		f, err := os.Open(path)
		if err != nil {
			return 0
		}
		defer f.Close()

		count := 0
		const bufSize = 64 * 1024 // 64KB read buffer (doesn't limit line length)
		buf := make([]byte, bufSize)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				for _, b := range buf[:n] {
					if b == '\n' {
						count++
					}
				}
			}
			if err != nil {
				break
			}
		}
		return count

	case "xlsx":
		return countXLSXRows(path)

	default:
		return 1
	}
}

// countXLSXRows counts rows in the first sheet of an Excel file.
func countXLSXRows(path string) int {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil {
		return 0
	}
	if len(rows) == 0 {
		return 1
	}
	return len(rows)
}

// embedStream runs the RAG embedder in a goroutine (for /embed command).
func (a *Application) embedStream(filePath string) {
	p := a.program
	if p == nil {
		return
	}

	if errMsg := a.ensureRAGStarted(); errMsg != "" {
		p.Send(RAGDone{Path: filePath, Error: errMsg})
		return
	}

	chunks, err := a.ragClient.IngestStream(filePath, "nomic-embed-text", func(msg string) {
		p.Send(RAGProgressMsg{Text: msg})
	})
	if err != nil {
		p.Send(RAGDone{Path: filePath, Error: err.Error()})
		return
	}
	p.Send(RAGDone{Path: filePath, Chunks: chunks})
}

// compactThreadIfNeeded summarizes old messages when the context window is >70% full.
// This preserves conversation flow without hitting token limits.
func (a *Application) compactThreadIfNeeded() {
	if a.thread == nil {
		return
	}

	// Check if we're over 70% of the context window
	maxTokens := a.thread.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	threshold := maxTokens * 70 / 100
	if a.thread.EstimatedTokens() < threshold {
		return // still plenty of room
	}

	// Keep the last 4 messages intact, summarize everything before that
	msgs := a.thread.Messages
	if len(msgs) <= 4 {
		return
	}

	oldMsgs := msgs[:len(msgs)-4]
	recentMsgs := msgs[len(msgs)-4:]

	// Build text to summarize
	var summaryText strings.Builder
	summaryText.WriteString("Summarize this conversation history. Keep: research topic, key findings, user goals, important numbers, and any decisions made.\n\nHistory:\n")
	for _, m := range oldMsgs {
		prefix := "User: "
		if m.Role == conversation.RoleAssistant {
			prefix = "Assistant: "
		}
		text := m.Content
		if len(text) > 500 {
			text = text[:500] + "..."
		}
		summaryText.WriteString(prefix + text + "\n")
	}

	// Call LLM for summarization
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	summary, err := (&srsLLMAdapter{client: a.client}).Generate(ctx, a.models.Active(), summaryText.String())
	if err != nil {
		// If summarization fails, just trim old messages silently
		msg := fmt.Sprintf("[Previous conversation compacted — %d messages summarized]", len(oldMsgs))
		a.thread.Messages = append([]conversation.Message{{
			Role:    conversation.RoleSystem,
			Content: msg,
		}}, recentMsgs...)
		return
	}

	if len(summary) > 800 {
		summary = summary[:800] + "..."
	}

	compacted := fmt.Sprintf("[Conversation summary from previous %d messages]: %s", len(oldMsgs), summary)
	a.thread.Messages = append([]conversation.Message{{
		Role:    conversation.RoleSystem,
		Content: compacted,
	}}, recentMsgs...)

	a.appendLine("  [Conversation compacted — old messages summarized]")
}

// Update the skills to mark SRS skills as implemented.
func (a *Application) updateSRSModelSkills() {
	skills := []memory.Skill{
		{Name: "FR/NFR Extraction", Description: "Extract functional and non-functional requirements from data", Command: "/srs", Category: "srs", Models: []string{"qwen2.5-coder", "gemma4"}, Parameters: "temperature: 0.1"},
		{Name: "MoSCoW Prioritization", Description: "Prioritize requirements using MoSCoW method", Command: "/srs", Category: "srs", Models: []string{"qwen2.5-coder"}, Parameters: "temperature: 0.1"},
		{Name: "DFD Generation", Description: "Identify Data Flow Diagram components", Command: "/srs", Category: "srs", Models: []string{"qwen2.5-coder"}},
		{Name: "CSPEC Logic", Description: "Create control specification tables", Command: "/srs", Category: "srs", Models: []string{"qwen2.5-coder"}},
		{Name: "SRS Formatting", Description: "Generate IEEE 830/29148 SRS document", Command: "/srs", Category: "srs", Models: []string{"qwen2.5-coder"}},
	}
	for _, skill := range skills {
		_ = a.mem.RegisterSkill(skill)
	}
}
