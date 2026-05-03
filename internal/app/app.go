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

	"mini-wiki/internal/config"
	"mini-wiki/internal/conversation"
	"mini-wiki/internal/chart"
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
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	RAGDone struct {
		Path     string
		Chunks   int
		Error    string
		Progress []string
	}

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

// --- Styles ---

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7C3AED")).
			Padding(0, 1)

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280")).
			Padding(0, 1)

	userMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#10B981"))

	assistantHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#8B5CF6")).
				Bold(true)

	assistantMsgStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#E5E7EB"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444"))

	modelTagStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8B5CF6")).
			Background(lipgloss.Color("#1F2937")).
			Padding(0, 1).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4B5563"))

	suggestionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D1D5DB")).
			Background(lipgloss.Color("#1F2937")).
			Padding(0, 1)

	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4B5563")).
			Padding(0, 1)

	bottomRightStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280")).
			Padding(0, 1)

	// Input box with subtle border
	inputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#4B5563")).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E2E8F0")).
			Bold(true).
			Padding(0, 2)

	subHeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#94A3B8")).
			Padding(0, 2)

	// Panel styles (center + right only)
	panelCenterStyle = lipgloss.NewStyle().
			Padding(0, 1)

	panelRightStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#1A1A2E")).
			Padding(1, 2)

	panelFocusStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#1F2B47")).
			Padding(1, 2)

	panelHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#CBD5E1"))

	infoLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#64748B"))

	infoValueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#CBD5E1"))

	// Bottom bar styles
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
	ragClient    *rag.Client
	ragDir       string
	currentRank  *ranking.RankResult // last /rank result in memory
	awaitingYn      bool                // waiting for y/n confirmation
	pendingYNMsg    string              // the question being asked
	pendingThreshold float64             // threshold being confirmed for /discard
	mem        memory.MemStore
	srsPipeline *srs.Pipeline

	// UI components
	input    textinput.Model
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
	ti := textinput.New()
	ti.Placeholder = "Type a message... (/help for commands)"
	ti.Focus()
	ti.CharLimit = 4096
	ti.Width = 80

	s := spinner.New()
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED"))
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
		showWelcome: true,
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
			a.input.Width = msg.Width - 4
			a.ready = true
			return a, tea.Batch(
				refreshModelsCmd(a.client, a.models),
				scanCmd(a.scanner, a.scanCfg),
			)
		}

		a.viewport.Width = msg.Width - 2
		a.viewport.Height = msg.Height - 8
		a.input.Width = msg.Width - 4
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
					a.appendToViewport(errorStyle.Render(fmt.Sprintf("Could not resolve %s: %s", re.Raw, re.Reason)))
				}
				if result.TotalSize > 0 {
					content = a.fileref.Inject(content, result)
					a.appendToViewport(fmt.Sprintf("Attached %d files (%d bytes)", len(result.Contents), result.TotalSize))
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

		// Retrieve relevant memory from past conversations (non-blocking)
		memoryCtx, memoryCancel := context.WithTimeout(context.Background(), 3*time.Second)
		pastEntries, _ := a.pkb.SearchHistory(memoryCtx, content, 3)
		memoryCancel()
		if len(pastEntries) > 0 {
			var memParts []string
			memParts = append(memParts, "\n[Previous relevant context from past sessions:]")
			for _, e := range pastEntries {
				if e.Response != "" {
					memParts = append(memParts, "Q: "+e.Query)
					if len(e.Response) > 200 {
						memParts = append(memParts, "A: "+e.Response[:200]+"...")
					} else {
						memParts = append(memParts, "A: "+e.Response)
					}
				}
			}
			memParts = append(memParts, "[/End of past context]\n")
			content = strings.Join(memParts, "\n") + "\n\nCurrent question: " + content
	}

	// Auto-RAG: search the vector database for relevant context (non-blocking)
	ragResult, _ := a.queryRAG(content, 3)
	if ragResult != nil && len(ragResult.Sources) > 0 {
		var ragParts []string
		ragParts = append(ragParts, "\n[Retrieved from knowledge base:]")
		for _, src := range ragResult.Sources {
			ragParts = append(ragParts, fmt.Sprintf("  Source: %s (score: %.2f)", src.File, src.Score))
			ragParts = append(ragParts, "  "+src.Text)
		}
		ragParts = append(ragParts, "[/End of retrieved context]\n")
		content = strings.Join(ragParts, "\n") + "\n\nQuestion: " + content
		a.appendToViewport(fmt.Sprintf("[RAG: %d sources retrieved]", len(ragResult.Sources)))
	}

	// Add user message to thread
	a.thread.Add(conversation.Message{
		Role:    conversation.RoleUser,
		Content: content,
	})
	a.updateTokenCount()

	// Render user message in viewport
	a.appendToViewport(formatUserMsg(content))

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
		a.appendToViewport(helpStyle.Render(summary))
		if len(msg.Result.Skipped) > 0 {
			a.appendToViewport(helpStyle.Render(fmt.Sprintf("Skipped %d entries", len(msg.Result.Skipped))))
		}
		a.statusMsg = fmt.Sprintf("Ready -- %d files indexed", len(msg.Result.Files))
		return a, nil

	case ScanFailed:
		a.busy = false
		a.errMsg = fmt.Sprintf("Scan failed: %v", msg.Err)
		return a, nil

	case FilesListRequested:
		if a.fileIndex == nil {
			a.appendToViewport(errorStyle.Render("No files indexed. Run /scan first."))
			return a, nil
		}
		a.appendToViewport(formatFileTree(a.fileIndex))
		return a, nil

	// --- File ingestion ---
	case IngestRequested:
		a.busy = true
		path := msg.Path
		a.statusMsg = fmt.Sprintf("Indexing %s...", filepath.Base(path))
		a.appendToViewport(fmt.Sprintf("[Indexing %s with RAG worker - waiting for response]", filepath.Base(path)))
		return a, ingestRAGCmd(a, path)

	case RAGDone:
		a.busy = false
		if len(msg.Progress) > 0 {
			for _, p := range msg.Progress {
				a.appendToViewport(fmt.Sprintf("  [RAG] %s", p))
			}
		}
		if msg.Error != "" {
			a.appendToViewport(fmt.Sprintf("[RAG error] %s", msg.Error))
			a.statusMsg = "RAG error"
		} else {
			a.appendToViewport(fmt.Sprintf("[RAG done] %s - %d chunks indexed", filepath.Base(msg.Path), msg.Chunks))
			a.statusMsg = fmt.Sprintf("Indexed %d chunks", msg.Chunks)
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

	// --- Ranking ---
	case RankRequested:
		a.busy = true
		a.statusMsg = "Ranking dataset..."
		return a, rankCmd(a, msg.Topic)

	case RankComplete:
		a.busy = false
		a.appendToViewport(msg.Text)
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
			a.appendToViewport(table)
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
		a.appendToViewport(preview)

		if discard == 0 {
			a.appendToViewport("No rows to discard at this threshold.")
			return a, nil
		}

		// Ask for confirmation (preview-only: --preview flag skips confirm)
		a.awaitingYn = true
		a.pendingYNMsg = fmt.Sprintf("Discard %d rows below score %.2f? (y/N)", discard, threshold)
		a.pendingThreshold = threshold
		a.appendToViewport(a.pendingYNMsg)
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
		a.appendToViewport(msg.Text)
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
		a.appendToViewport(msg.Text)
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
		a.appendToViewport(helpStyle.Render(summary))
		// Append content to conversation context
		content := fmt.Sprintf("\n--- Web: %s ---\n%s\n", r.Meta.FinalURL, r.Content)
		a.appendToViewport(helpStyle.Render(content))
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
		return a, exportCmd(a.exporter, a.thread, a.exportCfg, msg)

	case ExportComplete:
		a.busy = false
		r := msg.Result
		summary := fmt.Sprintf("Exported %d rows to %s (%s, %s)",
			r.Rows, r.FileName, humanBytes(r.Size), r.Duration.Round(time.Millisecond))
		a.appendToViewport(summary)
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
			a.appendToViewport(helpStyle.Render(fmt.Sprintf("Bookmarked: %s", msg.Title)))
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
			a.appendToViewport(helpStyle.Render("No bookmarks yet. Use /bookmark <title> to save one."))
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
			a.appendToViewport(helpStyle.Render(b.String()))
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
			a.appendToViewport(helpStyle.Render("No history yet."))
		} else {
			var b strings.Builder
			b.WriteString("Recent queries:\n")
			for _, e := range entries {
				t := e.CreatedAt.Format("15:04")
				b.WriteString(fmt.Sprintf("  - [%s] %s\n", t, truncateText(e.Query, 80)))
			}
			a.appendToViewport(helpStyle.Render(b.String()))
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
			a.appendToViewport(helpStyle.Render("No skills registered yet."))
		} else {
			var b strings.Builder
			b.WriteString("Tool skills:\n")
			for _, s := range skills {
				b.WriteString(fmt.Sprintf("  - %s (%s) — %s\n", s.Name, s.Category, s.Description))
				if s.Command != "" {
					b.WriteString(fmt.Sprintf("    Command: %s\n", s.Command))
				}
			}
			a.appendToViewport(helpStyle.Render(b.String()))
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
			a.appendToViewport(helpStyle.Render("No known flaws. Good! Keep an eye out for issues."))
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
			a.appendToViewport(helpStyle.Render(b.String()))
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
			a.appendToViewport(helpStyle.Render(fmt.Sprintf("   Stage %d: %s", msg.Stage, msg.Message)))
		}
		return a, nil

	case SRSComplete:
		// Display the SRS document
		a.appendToViewport(helpStyle.Render("\n--- SRS Document Generated ---\n"))
		// Show first 20 lines as preview
		lines := strings.Split(msg.Content, "\n")
		preview := lines
		if len(preview) > 25 {
			preview = preview[:25]
			preview = append(preview, "... (full document saved to project KB)")
		}
		a.appendToViewport(strings.Join(preview, "\n"))
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
		a.appendToViewport("") // trailing newline
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
					a.appendToViewport("Cancelled.")
				}
			} else {
				a.appendToViewport("Cancelled.")
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
				// If we already have suggestions, cycle; otherwise compute
				if len(a.suggestions) > 0 && a.tabIndex >= 0 {
					a.cycleSuggestion(val)
				} else {
					a.updateSuggestions()
					if len(a.suggestions) > 0 {
						a.tabIndex = 0
						selected := a.suggestions[0].text
						a.input.SetValue(selected + " ")
						a.input.SetCursor(len(selected) + 1)
					}
				}
				return a, nil
			}
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

	// --- Mouse events (scrolling) ---
	case tea.MouseMsg:
		var cmd tea.Cmd
		a.viewport, cmd = a.viewport.Update(msg)
		return a, cmd

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
			a.appendToViewport("No tasks. Use /task <description> to add one.")
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
			a.appendToViewport(b.String())
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
		help := `Available commands:
  /help           Show this help message
  /model <name>   Switch to a different model
  /models         List available models
  /refresh        Refresh model list from Ollama
  /clear          Clear the conversation
  /system <text>  Set a new system prompt
  /scan           Scan workspace for files
  /files          List scanned files
  /ingest <path>  Read a file into context (auto-builds KB)
  /fetch <url>    Fetch a webpage and extract text
  /export         Export conversation data to .xlsx
  /bookmark <t>   Save current finding as a bookmark
  /bookmarks      List saved bookmarks
  /history        Show recent query history
  /skills         List tool capabilities
  /flaws          Show known issues and solutions
  /task <desc>    Add a task to the todo list
  /tasks          List all tasks
  /srs            Run full SRS generation pipeline (5 stages)
  /exit           Quit the application

Memory & RAG:
  Every message is auto-saved to the project KB (./.wiki/)
  /ingest automatically builds the knowledge base
  Chat searches past conversations for relevant context`
		a.appendToViewport(helpStyle.Render(help))

	case "/model":
		if len(parts) < 2 {
			a.errMsg = "Usage: /model <name>"
		} else {
			return a, func() tea.Msg { return UserSwitchModel{Name: parts[1]} }
		}

	case "/models":
		names := a.models.AvailableNames()
		if len(names) == 0 {
			a.appendToViewport(errorStyle.Render("No models available. Try /refresh"))
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
			a.appendToViewport(helpStyle.Render(b.String()))
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
				a.appendToViewport("All previously discarded rows restored.")
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
			a.appendToViewport(fmt.Sprintf("Cannot detect format for %s", filename))
		} else {
			a.appendToViewport(fmt.Sprintf("Detected: %s", dataset.DetectFormat(filename)))
			a.appendToViewport(fmt.Sprintf("Try: /ingest @%s", filename))
		}
		return a, nil

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
			a.appendToViewport("[RAG operation cancelled]")
		}
		a.statusMsg = "Cancelled"
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

	// Bottom bar: model info on the right
	modelInfo := fmt.Sprintf("  active: %s  |  tokens: %d  |  loaded: %d models",
		a.models.Active(), a.estimatedTokens, len(a.models.Available()))
	bottomBar := bottomRightStyle.Render(modelInfo)

	// Layout: header(2) + sub(2) + panels(panelH-2) + overlay(0/1) + \n(1) + input(3) + \n(1) + bar(1) = h
	//   = 4 + panelH - 2 + overlay + 1 + 3 + 1 + 1 = panelH + 8 + overlay
	//   panelH = h - 8 - overlayLines
	panelH := h - 8 - overlayLines
	if panelH < 3 {
		panelH = 3
	}

	// Two panels: center (80%) + right (20%)
	rightW := w * 20 / 100
	if rightW < 18 {
		rightW = 18
	}
	centerW := w - rightW

	chatContent := a.renderChatPanel(centerW-2, panelH)
	infoContent := a.renderInfoPanel(rightW)

	rightSty := panelRightStyle
	centerSty := panelCenterStyle

	rightRendered := rightSty.Width(rightW).Height(panelH - 2).Render(infoContent)
	centerRendered := centerSty.Width(centerW).Height(panelH - 2).Render(chatContent)

	panelsRow := lipgloss.JoinHorizontal(lipgloss.Top, centerRendered, rightRendered)

	// Input box (full width)
	var inputRendered string
	if a.streaming {
		inputContent := fmt.Sprintf(" %s %s", a.spinner.View(), a.statusMsg)
		inputRendered = inputBoxStyle.Width(w - 2).Render(inputContent)
	} else {
		inputLine := a.input.View()
		if len(inputLine) > w-6 {
			inputLine = inputLine[:w-6]
		}
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
                                    
/srs  - Generate IEEE 830 SRS docs
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

func formatUserMsg(content string) string {
	return userMsgStyle.Render("You: ") + content + "\n"
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

			// Context with 5min timeout. Cancel is passed to the reader
			// via StreamStarted.CancelCtx and called when the stream completes.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

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
		b.WriteString("Recommended: ollama pull qwen2.5-coder\n")
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
	{text: "/wizard", description: "Run system check and setup assistant", category: "cmd"},
	{text: "/srs", description: "Run SRS generation pipeline", category: "cmd"},
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
	a.input.SetValue(selected.text + " ")
	a.input.SetCursor(len(selected.text) + 1)
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
		{Name: "SRS Pipeline", Description: "Full SRS generation: FR/NFR, MoSCoW, DFD, CSPEC, IEEE 830", Command: "/srs", Category: "srs", Models: []string{"qwen2.5-coder", "llama3.1"}, Parameters: "temperature: 0.1"},
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
		// Load the dataset from the project KB
		// This is a placeholder -- Phase 4 needs LoadDataset to actually work
		projectDir := a.pkb.ProjectDir()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		ranker := ranking.NewRanker(&srsLLMAdapter{client: a.client}, ranking.DefaultConfig())
		data, err := ranking.LoadDataset(projectDir)
		if err != nil {
			return RankFailed{Err: err}
		}

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

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		ranker := ranking.NewRanker(&srsLLMAdapter{client: a.client}, ranking.DefaultConfig())
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
	a.appendToViewport(fmt.Sprintf("Discarded %d rows. Remaining: %d rows.", discarded, kept.RowCount))
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
			a.appendToViewport(fmt.Sprintf("[%s too large (%d MB), showing first 10MB]", pathOnly, len(data)/(1024*1024)))
			data = data[:10*1024*1024]
		}
		*content = *content + "\n\n```" + pathOnly + "\n" + string(data) + "\n```"
		attached = append(attached, pathOnly)
	}
	if len(attached) > 0 {
		a.appendToViewport(fmt.Sprintf("Attached %d files", len(attached)))
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
	// Check for project-level virtual environment first
	rootDir := a.scanCfg.RootDir
	var pythonCandidates []string
	if rootDir != "" {
		pythonCandidates = []string{
			filepath.Join(rootDir, ".venv", "bin", "python3"),
			filepath.Join(rootDir, ".venv", "bin", "python"),
		}
	}
	pythonCandidates = append(pythonCandidates, "python3", "python")

	var lastErr error
	for _, python := range pythonCandidates {
		if err := a.ragClient.Start(python, workerPath, a.pkb.ProjectDir(), "nomic-embed-text", a.models.Active(), "http://127.0.0.1:11434"); err != nil {
			lastErr = err
			continue
		}
		return ""
	}
	// Get detailed error from stderr if available
	if errMsg := a.ragClient.LastError(); errMsg != "" {
		// Extract the most useful part of the error
		lines := strings.Split(errMsg, "\n")
		for _, line := range lines {
			if strings.Contains(line, "ModuleNotFoundError") || strings.Contains(line, "Error") {
				return fmt.Sprintf("Python error: %s", strings.TrimSpace(line))
			}
		}
		if len(lines) > 0 {
			return fmt.Sprintf("Python: %s", strings.TrimSpace(lines[len(lines)-1]))
		}
	}
	if lastErr != nil {
		return fmt.Sprintf("RAG worker: %v", lastErr)
	}
	return "RAG worker failed to start (unknown error)"
}

// queryRAG queries the RAG worker for relevant context, returning sources and answer.
func (a *Application) queryRAG(question string, topK int) (*rag.QueryResult, error) {
	if errMsg := a.ensureRAGStarted(); errMsg != "" {
		return nil, nil // RAG not available, silently skip
	}
	return a.ragClient.Query(question, topK)
}

// ingestRAGCmd returns a command that sends a file to the RAG worker and returns a RAGDone message.
func ingestRAGCmd(a *Application, path string) tea.Cmd {
	return func() tea.Msg {
		rootDir := a.scanCfg.RootDir
		if rootDir == "" {
			rootDir, _ = os.Getwd()
		}
		absPath, err := fileref.SafeResolve(rootDir, path)
		if err != nil {
			return RAGDone{Path: path, Error: err.Error()}
		}
		if errMsg := a.ensureRAGStarted(); errMsg != "" {
			return RAGDone{Path: path, Error: errMsg}
		}
		result, err := a.ragClient.Ingest(absPath, "nomic-embed-text")
		if err != nil {
			prog := []string{}
			if result != nil {
				prog = result.Progress
			}
			return RAGDone{Path: path, Error: err.Error(), Progress: prog}
		}
		if result != nil && result.Error != "" {
			return RAGDone{Path: path, Error: result.Error, Progress: result.Progress}
		}
		chunks := 0
		progress := []string{}
		if result != nil {
			chunks = result.Chunks
			progress = result.Progress
		}
		return RAGDone{Path: absPath, Chunks: chunks, Progress: progress}
	}
}

// Update the skills to mark SRS skills as implemented.
func (a *Application) updateSRSModelSkills() {
	skills := []memory.Skill{
		{Name: "FR/NFR Extraction", Description: "Extract functional and non-functional requirements from data", Command: "/srs", Category: "srs", Models: []string{"qwen2.5-coder", "llama3.1"}, Parameters: "temperature: 0.1"},
		{Name: "MoSCoW Prioritization", Description: "Prioritize requirements using MoSCoW method", Command: "/srs", Category: "srs", Models: []string{"qwen2.5-coder"}, Parameters: "temperature: 0.1"},
		{Name: "DFD Generation", Description: "Identify Data Flow Diagram components", Command: "/srs", Category: "srs", Models: []string{"qwen2.5-coder"}},
		{Name: "CSPEC Logic", Description: "Create control specification tables", Command: "/srs", Category: "srs", Models: []string{"qwen2.5-coder"}},
		{Name: "SRS Formatting", Description: "Generate IEEE 830/29148 SRS document", Command: "/srs", Category: "srs", Models: []string{"qwen2.5-coder"}},
	}
	for _, skill := range skills {
		_ = a.mem.RegisterSkill(skill)
	}
}
