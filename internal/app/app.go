// Package app defines the Bubbletea TUI application: model, update loop, and view rendering.
// It orchestrates the Ollama client, model manager, config, and conversation components
// into a cohesive terminal user interface.
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mini-wiki/internal/config"
	"mini-wiki/internal/conversation"
	"mini-wiki/internal/csvparser"
	"mini-wiki/internal/export"
	"mini-wiki/internal/fileref"
	"mini-wiki/internal/filescanner"
	"mini-wiki/internal/kb"
	"mini-wiki/internal/memory"
	"mini-wiki/internal/modelmgr"
	"mini-wiki/internal/ollama"
	"mini-wiki/internal/projectkb"
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
	IngestComplete   struct {
		Path    string
		Content string
		Size    int64
	}
	IngestFailed     struct {
		Path string
		Err  error
	}

	// Phase 3: Web fetching
	FetchRequested  struct{ URL string }
	FetchComplete   struct {
		Result *webfetch.FetchResult
	}
	FetchFailed     struct{ Err error }

	// Phase 3: Export
	ExportRequested struct{}
	ExportComplete  struct {
		Result *export.ExportResult
	}
	ExportFailed    struct{ Err error }

	// Phase 3: Knowledge base
	KBStatusRequested struct{}
	KBStatusComplete  struct {
		Stats map[string]any
	}
	KBQueryRequested  struct{ Query string }
	KBQueryComplete   struct {
		Results []kb.SearchResult
		Query   string
	}

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
)

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
	fetcher  webfetch.Fetcher
	exporter export.Exporter
	kb       kb.DB
	pkb        projectkb.DB
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
	fileIndex    *filescanner.ScanResult // last scan result
	scanCfg      filescanner.ScannerConfig
	refCfg       fileref.ResolverConfig

	// Phase 3: Web fetch & export state
	fetchCfg     webfetch.FetcherConfig
	exportCfg    export.ExportConfig

	// Command auto-completion
	suggestions  []string // command suggestions matching current input
	tabIndex     int      // current tab-cycle position (-1 = no selection)
}

// New creates a new Application with initialized components.
func New(cfg *config.Manager, client ollama.Client, mm *modelmgr.Manager) *Application {
	ti := textinput.New()
	ti.Placeholder = "Ask a question... (/model <name> to switch, /help for commands)"
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
		exporter:  export.New(),
		kb:        kb.New(),
		pkb:       projectkb.New(),
		mem:       memory.New(),
		thread:    conversation.NewThread("You are a helpful research assistant specializing in Software Engineering. Provide thorough, well-reasoned answers."),
		input:     ti,
		spinner:   s,
		statusMsg: "Initializing...",
		scanCfg: filescanner.ScannerConfig{
			RootDir: rootDir,
			MaxSize: 10 * 1024 * 1024,
		},
		refCfg: fileref.ResolverConfig{
			MaxFileSize:  1 * 1024 * 1024,
			MaxTotalSize: 10 * 1024 * 1024,
			MaxRefs:      10,
			RootDir:      rootDir,
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
			return a, refreshModelsCmd(a.client, a.models)
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

		// Handle slash commands
		if strings.HasPrefix(content, "/") {
			return a.handleCommand(content)
		}

		// Auto-resolve @file references (if we have a file index)
		if a.fileIndex != nil {
			refs := a.fileref.FindRefs(content)
			if len(refs) > 0 {
				if len(refs) > a.refCfg.MaxRefs {
					a.errMsg = fmt.Sprintf("Too many file references (%d, max %d)", len(refs), a.refCfg.MaxRefs)
					return a, nil
				}
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
					a.appendToViewport(helpStyle.Render(fmt.Sprintf("Attached %d files (%d bytes)", len(result.Contents), result.TotalSize)))
				}
			}
		}

		// Add user message to thread
		a.thread.Add(conversation.Message{
			Role:    conversation.RoleUser,
			Content: content,
		})

		// Render user message in viewport
		a.appendToViewport(formatUserMsg(content))

		// Clear input and start streaming
		a.input.SetValue("")
		a.errMsg = ""
		a.statusMsg = fmt.Sprintf("Thinking (%s)...", a.models.Active())

		return a, streamChatCmd(a.client, a.models, a.thread)

	// --- File scanning ---
	case ScanRequested:
		a.statusMsg = "Scanning files..."
		a.errMsg = ""
		return a, scanCmd(a.scanner, a.scanCfg)

	case ScanComplete:
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
		a.statusMsg = fmt.Sprintf("Reading %s...", msg.Path)
		return a, ingestCmd(a.fileref, msg.Path, a.refCfg)

	case IngestComplete:
		a.statusMsg = fmt.Sprintf("Read %s (%s)", filepath.Base(msg.Path), humanBytes(msg.Size))
		header := fmt.Sprintf("\n--- %s ---\n", msg.Path)
		a.appendToViewport(helpStyle.Render(header + msg.Content))
		return a, nil

	case IngestFailed:
		a.errMsg = fmt.Sprintf("Ingest failed: %v", msg.Err)
		a.statusMsg = "Ingest failed"
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
		a.statusMsg = "Exporting data..."
		return a, exportCmd(a.exporter, a.thread, a.exportCfg)

	case ExportComplete:
		r := msg.Result
		summary := fmt.Sprintf("Exported %d rows to %s (%s, %s)",
			r.Rows, r.FileName, humanBytes(r.Size), r.Duration.Round(time.Millisecond))
		a.appendToViewport(helpStyle.Render(summary))
		a.statusMsg = "Export complete"
		return a, nil

	case ExportFailed:
		a.errMsg = fmt.Sprintf("Export failed: %v", msg.Err)
		a.statusMsg = "Export failed"
		return a, nil

	// --- Knowledge base ---
	case KBStatusRequested:
		a.statusMsg = "Querying KB stats..."
		return a, kbStatsCmd(a.kb)

	case KBStatusComplete:
		stats := msg.Stats
		var lines []string
		for k, v := range stats {
			lines = append(lines, fmt.Sprintf("  %s: %v", k, v))
		}
		a.appendToViewport(helpStyle.Render("Knowledge Base Stats:\n" + strings.Join(lines, "\n")))
		a.statusMsg = "KB stats ready"
		return a, nil

	case KBQueryRequested:
		a.statusMsg = fmt.Sprintf("Searching KB for: %s", msg.Query)
		return a, kbQueryCmd(a.kb, msg.Query)

	case KBQueryComplete:
		if len(msg.Results) == 0 {
			a.appendToViewport(helpStyle.Render(fmt.Sprintf("No results for: %s", msg.Query)))
		} else {
			var b strings.Builder
			b.WriteString(fmt.Sprintf("KB Results for '%s':\n", msg.Query))
			for _, r := range msg.Results {
				b.WriteString(fmt.Sprintf("  [%s] row %d: %s\n", r.SourceFile, r.RowIndex, truncateText(r.Content, 80)))
			}
			a.appendToViewport(helpStyle.Render(b.String()))
		}
		a.statusMsg = "KB query complete"
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
		// While streaming, only allow Ctrl+C to interrupt
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
				a.input.SetValue(a.suggestions[a.tabIndex] + " ")
				a.input.SetCursor(len(a.suggestions[a.tabIndex]) + 1)
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

		// Handle Tab for command completion
		if msg.Type == tea.KeyTab {
			val := a.input.Value()
			if strings.HasPrefix(val, "/") {
				a.cycleSuggestion(val)
				return a, nil
			}
			return a, nil
		}

		// Handle Escape to clear suggestions
		if msg.Type == tea.KeyEscape {
			a.clearSuggestions()
			var cmd tea.Cmd
			a.input, cmd = a.input.Update(msg)
			return a, cmd
		}

		// Handle Ctrl+C / Ctrl+D to quit
		if msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyCtrlD {
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

	// --- Spinner tick ---
	case spinner.TickMsg:
		if a.streaming {
			var cmd tea.Cmd
			a.spinner, cmd = a.spinner.Update(msg)
			return a, cmd
		}
		return a, nil

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
  /ingest <path>  Read a file into context
  /fetch <url>    Fetch a webpage and extract text
  /export         Export conversation data to .xlsx
  /kbstatus       Show knowledge base stats
  /kbquery <q>    Search the knowledge base
  /bookmark <t>   Save current finding as a bookmark
  /bookmarks      List saved bookmarks
  /history        Show recent query history
  /skills         List tool capabilities
  /flaws          Show known issues and solutions
  /srs            Run full SRS generation pipeline (5 stages)
  /exit           Quit the application

File references:
  @filename       Reference a file in the workspace (auto-attached)`
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

	case "/ingest":
		if len(parts) < 2 {
			a.errMsg = "Usage: /ingest <filepath>"
			return a, nil
		}
		return a, func() tea.Msg { return IngestRequested{Path: parts[1]} }

	case "/fetch":
		if len(parts) < 2 {
			a.errMsg = "Usage: /fetch <url>"
			return a, nil
		}
		return a, func() tea.Msg { return FetchRequested{URL: parts[1]} }

	case "/export":
		return a, func() tea.Msg { return ExportRequested{} }

	case "/kbstatus":
		return a, func() tea.Msg { return KBStatusRequested{} }

	case "/kbquery":
		if len(parts) < 2 {
			a.errMsg = "Usage: /kbquery <query>"
			return a, nil
		}
		query := strings.Join(parts[1:], " ")
		return a, func() tea.Msg { return KBQueryRequested{Query: query} }

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

	case "/srs":
		return a, func() tea.Msg { return SRSRequested{} }

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

	var b strings.Builder

	// Title bar
	b.WriteString(titleStyle.Render(" mini-wiki "))
	b.WriteString(modelTagStyle.Render(a.models.Active()))
	b.WriteString("\n")

	// Status line
	statusText := a.statusMsg
	if a.streaming {
		statusText = fmt.Sprintf("%s %s", a.spinner.View(), statusText)
	}
	b.WriteString(statusStyle.Render(statusText))
	b.WriteString("\n\n")

	// Error message
	if a.errMsg != "" {
		b.WriteString(errorStyle.Render("! " + a.errMsg))
		b.WriteString("\n")
	}

	// Ping message (shown if Ollama is down)
	if a.pongMsg != "" {
		b.WriteString(errorStyle.Render("! " + a.pongMsg))
		b.WriteString("\n")
	}

	// Conversation viewport
	b.WriteString(a.viewport.View())
	b.WriteString("\n")

	// Input area
	b.WriteString(a.input.View())
	b.WriteString("\n")

	// Suggestions / hints bar
	if len(a.suggestions) > 0 {
		b.WriteString(suggestionStyle.Render(formatSuggestions(a.suggestions, a.tabIndex)))
	} else {
		val := a.input.Value()
		if strings.HasPrefix(val, "/") {
			b.WriteString(hintStyle.Render("Tab to complete"))
		} else if val == "" {
			b.WriteString(hintStyle.Render("/help for commands"))
		}
	}

	return b.String()
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

func ingestCmd(r fileref.Resolver, path string, cfg fileref.ResolverConfig) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		ref := fileref.Reference{Raw: "@" + path}
		absPath, data, err := r.Resolve(ctx, ref, cfg)
		if err != nil {
			return IngestFailed{Path: path, Err: err}
		}
		return IngestComplete{Path: absPath, Content: string(data), Size: int64(len(data))}
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

func exportCmd(e export.Exporter, thread *conversation.Thread, cfg export.ExportConfig) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Build export data from conversation thread
		rows := make([]export.Row, 0, len(thread.Messages))
		for _, msg := range thread.Messages {
			rows = append(rows, export.Row{
				string(msg.Role),
				msg.Content,
				msg.Timestamp.Format(time.RFC3339),
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

func kbStatsCmd(kb kb.DB) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		stats, err := kb.Stats(ctx)
		if err != nil {
			return ExportFailed{Err: err}
		}
		return KBStatusComplete{Stats: stats}
	}
}

func kbQueryCmd(kb kb.DB, query string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		results, err := kb.SearchRows(ctx, query, 10)
		if err != nil {
			return ExportFailed{Err: err}
		}
		return KBQueryComplete{Results: results, Query: query}
	}
}

func truncateText(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// --- Command auto-completion ---

var availableCommands = []string{
	"/help", "/model", "/models", "/refresh", "/clear", "/system",
	"/scan", "/files", "/ingest", "/fetch", "/export",
	"/kbstatus", "/kbquery", "/bookmark", "/bookmarks",
	"/history", "/skills", "/flaws", "/srs", "/exit",
}

func (a *Application) updateSuggestions() {
	val := a.input.Value()
	a.suggestions = nil
	a.tabIndex = -1

	if !strings.HasPrefix(val, "/") {
		return
	}

	for _, cmd := range availableCommands {
		if strings.HasPrefix(cmd, val) {
			a.suggestions = append(a.suggestions, cmd)
		}
	}
}

func (a *Application) cycleSuggestion(val string) {
	// Rebuild suggestions if empty or input changed
	if len(a.suggestions) == 0 || (a.tabIndex >= 0 && a.suggestions[a.tabIndex] != val) {
		a.suggestions = nil
		for _, cmd := range availableCommands {
			if strings.HasPrefix(cmd, val) {
				a.suggestions = append(a.suggestions, cmd)
			}
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

	// Show the suggestion in the input (partial fill)
	a.input.SetValue(a.suggestions[a.tabIndex] + " ")
	a.input.SetCursor(len(a.suggestions[a.tabIndex]) + 1)
}

func (a *Application) clearSuggestions() {
	a.suggestions = nil
	a.tabIndex = -1
}

func formatSuggestions(suggestions []string, selected int) string {
	if len(suggestions) == 0 {
		return ""
	}
	var b strings.Builder
	for i, s := range suggestions {
		if i > 0 {
			b.WriteString("  ")
		}
		if i == selected {
			b.WriteString("> " + s)
		} else {
			b.WriteString("  " + s)
		}
	}
	return b.String()
}

// registerBuiltinSkills registers the tool's built-in capabilities in tool memory.
func (a *Application) registerBuiltinSkills() {
	skills := []memory.Skill{
		{Name: "Chat", Description: "Conversational AI assistant with local LLM", Command: "/chat", Category: "system", Models: []string{"qwen2.5-coder", "gemma4:e4b"}},
		{Name: "File Scanner", Description: "Scan workspace directory for files", Command: "/scan", Category: "data", Models: nil},
		{Name: "File Reference", Description: "Reference files with @filename in chat", Command: "@filename", Category: "data", Models: nil},
		{Name: "Web Fetch", Description: "Fetch webpage content and extract text", Command: "/fetch", Category: "data", Models: nil, Note: "SSRF-safe, blocks private IPs"},
		{Name: "Export", Description: "Export conversation to .xlsx", Command: "/export", Category: "export", Models: nil},
		{Name: "Knowledge Base", Description: "Search ingested data with FTS5", Command: "/kbquery", Category: "data", Models: nil},
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
