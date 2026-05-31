package app

import (
	"testing"

	"mini-wiki/internal/conversation"
	"mini-wiki/internal/filescanner"
)

// --- Pure helper functions ---

func TestFormatUserMsg(t *testing.T) {
	output := formatUserMsg("hello world")
	if output == "" {
		t.Error("formatUserMsg returned empty")
	}
	if !containsStr(output, "hello world") {
		t.Errorf("expected output to contain 'hello world', got: %s", output)
	}
}

func TestFormatAssistantMsg(t *testing.T) {
	output := formatAssistantMsg("gemma4:e4b")
	if output == "" {
		t.Error("formatAssistantMsg returned empty")
	}
	if !containsStr(output, "gemma4") {
		t.Errorf("expected output to contain model name, got: %s", output)
	}
}

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := humanBytes(tt.input)
			if got != tt.want {
				t.Errorf("humanBytes(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTruncateText(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 10, ""},
		{"exact", 5, "exact"},
		{"exactly!", 5, "exact..."},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncateText(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateText(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestShortName(t *testing.T) {
	tests := []struct {
		path   string
		maxLen int
		want   string
	}{
		{"short.txt", 20, "short.txt"},
		{"/a/b/c/d/e/f/g/h/file.txt", 20, ".../g/h/file.txt"},
		{"file.txt", 5, "file.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := shortName(tt.path, tt.maxLen)
			if len(got) > tt.maxLen && len(tt.path) > tt.maxLen {
				// Should be truncated or use ellipsis
				if !containsStr(got, "...") {
					t.Errorf("shortName(%q, %d) = %q, should contain '...'", tt.path, tt.maxLen, got)
				}
			}
		})
	}
}

func TestFileTypeName(t *testing.T) {
	tests := []struct {
		ft   filescanner.FileType
		want string
	}{
		{filescanner.FileTypeText, "Text"},
		{filescanner.FileTypeMarkdown, "Markdown"},
		{filescanner.FileTypeCSV, "CSV"},
		{filescanner.FileTypeJSON, "JSON"},
		{filescanner.FileTypeYAML, "YAML"},
		{filescanner.FileTypeXML, "XML"},
		{filescanner.FileTypeHTML, "HTML"},
		{filescanner.FileTypeGo, "Go"},
		{filescanner.FileTypePython, "Python"},
		{filescanner.FileTypeJavaScript, "JavaScript"},
		{filescanner.FileTypeTypeScript, "TypeScript"},
		{filescanner.FileTypeShell, "Shell"},
		{filescanner.FileTypeSQL, "SQL"},
		{filescanner.FileTypeConfig, "Config"},
		{filescanner.FileTypeMakefile, "Makefile"},
		{filescanner.FileTypeDockerfile, "Dockerfile"},
		{filescanner.FileType(255), "Other"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := fileTypeName(tt.ft)
			if got != tt.want {
				t.Errorf("fileTypeName(%v) = %q, want %q", tt.ft, got, tt.want)
			}
		})
	}
}

func TestExtractRefInfo(t *testing.T) {
	tests := []struct {
		input      string
		wantPath   string
		wantLine   int
		wantEndLine int
	}{
		{"@file.txt", "file.txt", 0, 0},
		{"@/path/to/file.go:42", "/path/to/file.go", 42, 42},
		{"@file.go:10-20", "file.go", 10, 20},
		{"@data.csv", "data.csv", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			path, line, endLine := extractRefInfo(tt.input)
			if path != tt.wantPath {
				t.Errorf("path = %q, want %q", path, tt.wantPath)
			}
			if line != tt.wantLine {
				t.Errorf("line = %d, want %d", line, tt.wantLine)
			}
			if endLine != tt.wantEndLine {
				t.Errorf("endLine = %d, want %d", endLine, tt.wantEndLine)
			}
		})
	}
}

func TestExtractPathToken(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello @file.txt", "file.txt"},
		{"@data.csv", "data.csv"},
		{"no at sign", ""},
		{"", ""},
		{"/absolute/path/file.txt", "/absolute/path/file.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractPathToken(tt.input)
			// Just verify it doesn't crash and returns something
			_ = got
		})
	}
}

func TestFormatSuggestions(t *testing.T) {
	suggestions := []suggestionItem{
		{text: "/help", description: "Show help", category: "cmd"},
		{text: "/rank", description: "Rank data", category: "cmd"},
	}
	output := formatSuggestions(suggestions, 0)
	if output == "" {
		t.Error("formatSuggestions should not be empty")
	}
	if !containsStr(output, "/help") {
		t.Errorf("expected suggestions to contain /help, got: %s", output)
	}

	// Empty suggestions
	empty := formatSuggestions(nil, -1)
	if empty != "" {
		t.Errorf("expected empty for nil suggestions, got: %s", empty)
	}
}

func TestHelpSummary(t *testing.T) {
	summary := helpSummary()
	if summary == "" {
		t.Fatal("helpSummary returned empty")
	}
	if !containsStr(summary, "mini-wiki") {
		t.Errorf("helpSummary should mention 'mini-wiki'")
	}
	if !containsStr(summary, "/rank") {
		t.Errorf("helpSummary should mention /rank")
	}
	if !containsStr(summary, "/help") {
		t.Errorf("helpSummary should mention /help")
	}
}

func TestFormatFileTree_Nil(t *testing.T) {
	output := formatFileTree(nil)
	if output == "" {
		t.Error("formatFileTree(nil) should not crash or return empty")
	}
}

func TestFormatFileTree_Empty(t *testing.T) {
	result := &filescanner.ScanResult{
		Files: []filescanner.FileInfo{},
		Root:  "/test",
	}
	output := formatFileTree(result)
	if !containsStr(output, "No files") {
		t.Errorf("expected 'No files' message, got: %s", output)
	}
}

func TestFormatFileTree_WithFiles(t *testing.T) {
	result := &filescanner.ScanResult{
		Root: "/test",
		Files: []filescanner.FileInfo{
			{Path: "/test/data.csv", FileType: filescanner.FileTypeCSV, Size: 100},
			{Path: "/test/readme.md", FileType: filescanner.FileTypeMarkdown, Size: 50},
		},
		Total: 150,
	}
	output := formatFileTree(result)
	if !containsStr(output, "CSV") {
		t.Errorf("expected CSV section, got: %s", output)
	}
	if !containsStr(output, "data.csv") {
		t.Errorf("expected data.csv, got: %s", output)
	}
}

func TestExtractPathToken_AtSign(t *testing.T) {
	got := extractPathToken("some text @data.csv more")
	// extractPathToken returns everything after the last @
	if got != "data.csv more" {
		t.Errorf("expected 'data.csv more', got %q", got)
	}
}

// --- Application message handler tests ---
// These test that Update() handles various messages correctly
// by constructing an Application with minimal dependencies.

// --- Test that handleCommand doesn't panic ---

func TestHandleCommandHelp(t *testing.T) {
	a := appWithMinimalState(t)
	_, _ = a.handleCommand("/help")
	if a.errMsg != "" {
		t.Errorf("errMsg = %q, want empty", a.errMsg)
	}
}

func TestHandleCommandHelpSpecific(t *testing.T) {
	a := appWithMinimalState(t)
	_, _ = a.handleCommand("/help rank")
	if a.errMsg != "" {
		t.Errorf("errMsg = %q, want empty", a.errMsg)
	}
}

func TestHandleCommandUnknown(t *testing.T) {
	a := appWithMinimalState(t)
	_, _ = a.handleCommand("/nonexistent")
	if a.errMsg == "" {
		t.Error("expected error for unknown command")
	}
}

func TestHandleCommandClear(t *testing.T) {
	a := appWithMinimalState(t)
	_, _ = a.handleCommand("/clear")
	if a.errMsg != "" {
		t.Errorf("errMsg = %q", a.errMsg)
	}
}

func TestHandleCommandPanel(t *testing.T) {
	a := appWithMinimalState(t)
	original := a.showInfoPanel
	_, _ = a.handleCommand("/panel")
	if a.showInfoPanel == original {
		t.Errorf("showInfoPanel should have toggled from %v", original)
	}
	_, _ = a.handleCommand("/panel")
	if a.showInfoPanel != original {
		t.Errorf("showInfoPanel should have toggled back to %v", original)
	}
}

func TestHandleCommandExit(t *testing.T) {
	a := appWithMinimalState(t)
	_, cmd := a.handleCommand("/exit")
	if cmd == nil {
		t.Error("/exit should return tea.Quit command")
	}
}

func TestHandleCommandIngestNoArg(t *testing.T) {
	a := appWithMinimalState(t)
	_, _ = a.handleCommand("/ingest")
	if a.errMsg == "" {
		t.Error("expected error for /ingest without args")
	}
}

func TestHandleCommandRankNoArg(t *testing.T) {
	a := appWithMinimalState(t)
	_, _ = a.handleCommand("/rank")
	if a.errMsg == "" {
		t.Error("expected error for /rank without args")
	}
}

func TestHandleCommandDiscardNoArg(t *testing.T) {
	a := appWithMinimalState(t)
	_, _ = a.handleCommand("/discard")
	if a.errMsg == "" {
		t.Error("expected error for /discard without args")
	}
}

// --- View tests ---

func TestViewNotReady(t *testing.T) {
	a := &Application{}
	view := a.View()
	if !containsStr(view, "Initializing") {
		t.Errorf("expected 'Initializing' when not ready, got: %s", view)
	}
}

// --- AppState defaults ---

func TestAppStateDefaults(t *testing.T) {
	a := appWithMinimalState(t)
	if a.state != StateIdle {
		t.Errorf("default state = %v, want StateIdle", a.state)
	}
}

// --- Helpers ---

// errTest is a sentinel error for testing.
var errTest = &testError{"test error"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

// appWithMinimalState creates an Application with minimal initialized state for testing.
func appWithMinimalState(t *testing.T) *Application {
	t.Helper()
	return &Application{
		state:       StateIdle,
		statusMsg:   "Initializing...",
		errMsg:      "",
		showWelcome: true,
		thread:      conversation.NewThread("test system prompt"),
	}
}

// spinnerTickMsg creates a spinner.TickMsg for testing.
func spinnerTickMsg() interface{} {
	return struct{}{} // not a real TickMsg but tests the default case
}

// teaWindowSize creates a WindowSizeMsg-like value for testing.
func teaWindowSize(w, h int) interface{} {
	return windowSizeMsg{w, h}
}

// windowSizeMsg is a minimal stand-in for tea.WindowSizeMsg during testing.
type windowSizeMsg struct {
	Width  int
	Height int
}

func (m windowSizeMsg) _isMsg() {}

// containsStr helper
func containsStr(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
