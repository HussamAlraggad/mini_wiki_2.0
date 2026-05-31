// Package app provides intent detection and tool dispatch for natural language
// interactions with the Mini Wiki TUI. It defines the available tools the LLM
// can invoke, classifies user intent via a fast non-streaming LLM call, and
// dispatches to the appropriate command handler.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"mini-wiki/internal/ollama"

	tea "github.com/charmbracelet/bubbletea"
)

// ToolCall represents a tool that the LLM decided to invoke.
type ToolCall struct {
	Tool string                 `json:"tool"`
	Args map[string]interface{} `json:"args"`
}

// ArgSpec describes a single tool parameter.
type ArgSpec struct {
	Type        string `json:"type"`        // "string", "float", "int", "bool"
	Description string `json:"description"` // human-readable description for the LLM
}

// ToolSpec describes a tool available for natural language invocation.
type ToolSpec struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Args        map[string]ArgSpec `json:"args,omitempty"`
}

// availableTools defines all tools the LLM can invoke via natural language.
// Each tool maps to an existing slash-command handler.
var availableTools = []ToolSpec{
	{
		Name:        "rank",
		Description: "Rank or filter dataset rows by relevance to a topic. Use when the user asks to find relevant rows, filter by topic, rank items, or discover what matches a description.",
		Args: map[string]ArgSpec{
			"topic": {Type: "string", Description: "The topic, theme, or description to rank by"},
		},
	},
	{
		Name:        "chart",
		Description: "Generate a chart or graph from the dataset. Use when the user asks to visualize, plot, graph, chart, draw, or see a picture of the data.",
		Args: map[string]ArgSpec{
			"type":   {Type: "string", Description: "Chart type: bar, pie, trend, scatter, histogram"},
			"column": {Type: "string", Description: "The column name to chart"},
		},
	},
	{
		Name:        "export",
		Description: "Export or save the dataset to a file. Use when the user asks to save, export, download, or write the data to a file.",
		Args: map[string]ArgSpec{
			"format": {Type: "string", Description: "Export format: csv, xlsx, json"},
		},
	},
	{
		Name:        "discard",
		Description: "Remove or discard low-scoring rows from the working dataset. Use when the user asks to clean up, remove, discard, or filter out rows below a relevance score.",
		Args: map[string]ArgSpec{
			"threshold": {Type: "float", Description: "Score threshold between 0.0 and 1.0. Rows below this are removed."},
		},
	},
	{
		Name:        "dataset_info",
		Description: "Show information about the active dataset: columns, row count, schema, sample rows. Use when the user asks what data is loaded, what columns exist, or for a dataset summary.",
		Args:        map[string]ArgSpec{},
	},
	{
		Name:        "ingest",
		Description: "Load or import a data file into the tool. Use when the user asks to load, import, open, or ingest a file.",
		Args: map[string]ArgSpec{
			"path": {Type: "string", Description: "File path to load"},
		},
	},
}

// classifyIntentPrompt returns the system prompt for intent classification.
func classifyIntentPrompt(message string) string {
	var b strings.Builder
	b.WriteString("You are an intent classifier for a data analysis tool. Your job is to determine if the user wants to use a tool.\n\n")
	b.WriteString("Available tools:\n")
	for _, t := range availableTools {
		b.WriteString(fmt.Sprintf("- %s: %s\n", t.Name, t.Description))
		if len(t.Args) > 0 {
			b.WriteString("  Arguments:\n")
			for name, spec := range t.Args {
				b.WriteString(fmt.Sprintf("    %s (%s): %s\n", name, spec.Type, spec.Description))
			}
		}
	}
	b.WriteString("\n")
	b.WriteString("If the user wants to use a tool, respond with JSON only:\n")
	b.WriteString(`{"tool": "tool_name", "args": {"arg_name": "arg_value"}}` + "\n")
	b.WriteString("If no tool is needed (casual chat, general question, greeting), respond with:\n")
	b.WriteString(`{"tool": null}` + "\n")
	b.WriteString("\nExamples:\n")
	b.WriteString(`User: "find me rows about machine learning"` + "\n")
	b.WriteString(`Response: {"tool": "rank", "args": {"topic": "machine learning"}}` + "\n\n")
	b.WriteString(`User: "show me a pie chart of categories"` + "\n")
	b.WriteString(`Response: {"tool": "chart", "args": {"type": "pie", "column": "categories"}}` + "\n\n")
	b.WriteString(`User: "hello, how are you?"` + "\n")
	b.WriteString(`Response: {"tool": null}` + "\n\n")
	b.WriteString(`User: "what columns are in my data?"` + "\n")
	b.WriteString(`Response: {"tool": "dataset_info", "args": {}}` + "\n\n")
	b.WriteString(`User: "save these results as CSV"` + "\n")
	b.WriteString(`Response: {"tool": "export", "args": {"format": "csv"}}` + "\n\n")
	b.WriteString("User: " + message + "\n")
	b.WriteString("Response: ")
	return b.String()
}

// classifyIntent makes a fast non-streaming LLM call to classify user intent.
// Returns a ToolCall if a tool should be invoked, or nil for standard chat.
// Uses the active model (Option A per our design decision).
func (a *Application) classifyIntent(message string) *ToolCall {
	if a.client == nil {
		return nil
	}

	prompt := classifyIntentPrompt(message)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := a.client.Generate(ctx, ollama.GenerateRequest{
		Model:  a.models.Active(),
		Prompt: prompt,
		Stream: false,
		Options: map[string]any{
			"temperature": 0.1, // low temperature for deterministic classification
		},
	})
	if err != nil || resp.Response == "" {
		return nil // classify silently — fall through to standard chat
	}

	return parseToolCall(resp.Response)
}

// parseToolCall extracts a ToolCall from the LLM's JSON response.
// It handles common formatting issues like markdown code fences.
func parseToolCall(response string) *ToolCall {
	// Strip markdown code fences if present
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var tc ToolCall
	if err := json.Unmarshal([]byte(response), &tc); err != nil {
		return nil
	}

	// Validate: tool must be a known tool name or null
	if tc.Tool == "" || tc.Tool == "null" {
		return nil
	}

	// Validate: tool must be in our available tools list
	valid := false
	for _, t := range availableTools {
		if t.Name == tc.Tool {
			valid = true
			break
		}
	}
	if !valid {
		return nil
	}

	// Ensure args map exists
	if tc.Args == nil {
		tc.Args = make(map[string]interface{})
	}

	return &tc
}

// executeIntent dispatches a ToolCall to the appropriate command handler.
// It returns a tea.Cmd that will produce the tool result message.
func (a *Application) executeIntent(tc *ToolCall) tea.Cmd {
	if tc == nil {
		return nil
	}

	switch tc.Tool {
	case "rank":
		topic := getArgString(tc.Args, "topic")
		if topic == "" {
			return nil
		}
		// Set pending intent flag so RankComplete knows to wrap conversationally
		a.state = StateRanking
		return func() tea.Msg {
			return RankRequested{Topic: topic}
		}

	case "chart":
		chartType := getArgString(tc.Args, "type")
		column := getArgString(tc.Args, "column")
		if chartType == "" || column == "" {
			return nil
		}
		return func() tea.Msg {
			return ChartRequested{Type: chartType, ColumnX: column}
		}

	case "export":
		format := getArgString(tc.Args, "format")
		if format == "" {
			format = "xlsx"
		}
		return func() tea.Msg {
			return ExportRequested{Format: format}
		}

	case "discard":
		threshold := getArgFloat(tc.Args, "threshold")
		if threshold <= 0 || threshold > 1 {
			return nil
		}
		return func() tea.Msg {
			return DiscardRequested{Threshold: threshold}
		}

	case "dataset_info":
		// Show dataset info inline — no need for a full tool execution
		return func() tea.Msg {
			projectDir := a.pkb.ProjectDir()
			if projectDir == "" {
				return DataAnalysisResult{
					Question: "dataset info",
					Answer:   "No project directory found. Ingest a dataset first with /ingest.",
				}
			}
			// Get active dataset info from project KB
			filePath, format, err := a.pkb.GetActiveDataset(context.Background())
			if err != nil || filePath == "" {
				return DataAnalysisResult{
					Question: "dataset info",
					Answer:   "No dataset loaded. Use /ingest @<file> to load a dataset.",
				}
			}
			info := fmt.Sprintf("Active dataset: %s (format: %s)", filepath.Base(filePath), format)
			return DataAnalysisResult{
				Question: "dataset info",
				Answer:   info,
			}
		}

	case "ingest":
		path := getArgString(tc.Args, "path")
		if path == "" {
			return nil
		}
		return func() tea.Msg {
			return IngestRequested{Path: path}
		}

	default:
		return nil
	}
}

// getArgString safely extracts a string argument from a tool call's args map.
func getArgString(args map[string]interface{}, key string) string {
	if args == nil {
		return ""
	}
	v, ok := args[key]
	if !ok {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	default:
		return fmt.Sprintf("%v", s)
	}
}

// getArgFloat safely extracts a float argument from a tool call's args map.
func getArgFloat(args map[string]interface{}, key string) float64 {
	if args == nil {
		return 0
	}
	v, ok := args[key]
	if !ok {
		return 0
	}
	switch f := v.(type) {
	case float64:
		return f
	case int:
		return float64(f)
	case string:
		var val float64
		_, _ = fmt.Sscanf(f, "%f", &val)
		return val
	default:
		return 0
	}
}

// pendingIntent is set when a tool call is being executed from NL intent.
// It's checked in tool-complete handlers to wrap results conversationally.
// This is stored on the Application struct as a simple boolean flag alongside AppState.
