package app

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseToolCall_ValidJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantTool string
		wantArg  string
		wantNil  bool
	}{
		{
			name:     "rank tool",
			input:    `{"tool": "rank", "args": {"topic": "machine learning"}}`,
			wantTool: "rank",
			wantArg:  "machine learning",
		},
		{
			name:     "chart tool",
			input:    `{"tool": "chart", "args": {"type": "bar", "column": "category"}}`,
			wantTool: "chart",
			wantArg:  "bar",
		},
		{
			name:     "null tool (no intent)",
			input:    `{"tool": null}`,
			wantNil:  true,
		},
		{
			name:     "dataset info",
			input:    `{"tool": "dataset_info", "args": {}}`,
			wantTool: "dataset_info",
		},
		{
			name:     "export with format",
			input:    `{"tool": "export", "args": {"format": "csv"}}`,
			wantTool: "export",
			wantArg:  "csv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := parseToolCall(tt.input)
			if tt.wantNil {
				if tc != nil {
					t.Errorf("expected nil, got %+v", tc)
				}
				return
			}
			if tc == nil {
				t.Fatal("expected non-nil ToolCall")
			}
			if tc.Tool != tt.wantTool {
				t.Errorf("tool = %q, want %q", tc.Tool, tt.wantTool)
			}
		})
	}
}

func TestParseToolCall_MarkdownFences(t *testing.T) {
	input := "```json\n{\"tool\": \"rank\", \"args\": {\"topic\": \"test\"}}\n```"
	tc := parseToolCall(input)
	if tc == nil {
		t.Fatal("expected non-nil ToolCall")
	}
	if tc.Tool != "rank" {
		t.Errorf("tool = %q, want %q", tc.Tool, "rank")
	}
	arg := getArgString(tc.Args, "topic")
	if arg != "test" {
		t.Errorf("topic arg = %q, want %q", arg, "test")
	}
}

func TestParseToolCall_InvalidTool(t *testing.T) {
	// Unknown tool should be rejected
	input := `{"tool": "unknown_tool", "args": {}}`
	tc := parseToolCall(input)
	if tc != nil {
		t.Errorf("expected nil for unknown tool, got %+v", tc)
	}
}

func TestParseToolCall_InvalidJSON(t *testing.T) {
	inputs := []string{
		"not json at all",
		`{malformed`,
		"",
		`{"tool": 123}`, // tool must be string
	}
	for _, input := range inputs {
		t.Run("input_"+truncateStr(input, 20), func(t *testing.T) {
			tc := parseToolCall(input)
			if tc != nil {
				t.Errorf("expected nil for invalid input, got %+v", tc)
			}
		})
	}
}

func TestParseToolCall_NoNullTool(t *testing.T) {
	// "null" as a string value
	input := `{"tool": "null"}`
	tc := parseToolCall(input)
	if tc != nil {
		t.Errorf("expected nil for 'null' string, got %+v", tc)
	}

	// Empty tool
	input = `{"tool": "", "args": {}}`
	tc = parseToolCall(input)
	if tc != nil {
		t.Errorf("expected nil for empty tool, got %+v", tc)
	}
}

func TestGetArgString(t *testing.T) {
	args := map[string]interface{}{
		"topic":  "machine learning",
		"format": "csv",
		"count":  42,
	}
	if got := getArgString(args, "topic"); got != "machine learning" {
		t.Errorf("getArgString(topic) = %q, want %q", got, "machine learning")
	}
	if got := getArgString(args, "format"); got != "csv" {
		t.Errorf("getArgString(format) = %q, want %q", got, "csv")
	}
	// Non-string values should be stringified
	if got := getArgString(args, "count"); got != "42" {
		t.Errorf("getArgString(count) = %q, want %q", got, "42")
	}
	// Missing key
	if got := getArgString(args, "nonexistent"); got != "" {
		t.Errorf("getArgString(nonexistent) = %q, want ''", got)
	}
	// Nil map
	if got := getArgString(nil, "key"); got != "" {
		t.Errorf("getArgString(nil) = %q, want ''", got)
	}
}

func TestGetArgFloat(t *testing.T) {
	args := map[string]interface{}{
		"threshold": 0.75,
		"count":     10,
		"str_val":   "0.5",
	}
	if got := getArgFloat(args, "threshold"); got != 0.75 {
		t.Errorf("getArgFloat(threshold) = %f, want %f", got, 0.75)
	}
	if got := getArgFloat(args, "count"); got != 10.0 {
		t.Errorf("getArgFloat(count) = %f, want %f", got, 10.0)
	}
	if got := getArgFloat(args, "str_val"); got != 0.5 {
		t.Errorf("getArgFloat(str_val) = %f, want %f", got, 0.5)
	}
	// Missing key
	if got := getArgFloat(args, "nonexistent"); got != 0 {
		t.Errorf("getArgFloat(nonexistent) = %f, want 0", got)
	}
	// Nil map
	if got := getArgFloat(nil, "key"); got != 0 {
		t.Errorf("getArgFloat(nil) = %f, want 0", got)
	}
}

func TestClassifyIntentPrompt_ContainsToolNames(t *testing.T) {
	prompt := classifyIntentPrompt("test message")
	for _, tool := range availableTools {
		if !strings.Contains(prompt, tool.Name) {
			t.Errorf("prompt should mention tool %q", tool.Name)
		}
		if !strings.Contains(prompt, tool.Description) {
			t.Errorf("prompt should contain description of tool %q", tool.Name)
		}
	}
	// Should include the user message
	if !strings.Contains(prompt, "test message") {
		t.Errorf("prompt should contain the user message")
	}
}

func TestClassifyIntentPrompt_JSONFormat(t *testing.T) {
	prompt := classifyIntentPrompt("hello")
	// Should instruct JSON response format
	if !strings.Contains(prompt, "JSON only") && !strings.Contains(prompt, `"tool":`) {
		t.Errorf("prompt should instruct JSON response format")
	}
}

func TestAvailableTools_ValidJSON(t *testing.T) {
	// Verify each tool spec can be serialized to JSON and back without errors
	for _, tool := range availableTools {
		t.Run(tool.Name, func(t *testing.T) {
			data, err := json.Marshal(tool)
			if err != nil {
				t.Fatalf("failed to marshal tool %q: %v", tool.Name, err)
			}
			if len(data) == 0 {
				t.Errorf("tool %q serialized to empty JSON", tool.Name)
			}
			// Verify it unmarshals back
			var restored ToolSpec
			if err := json.Unmarshal(data, &restored); err != nil {
				t.Fatalf("failed to unmarshal tool %q: %v", tool.Name, err)
			}
			if restored.Name != tool.Name {
				t.Errorf("name mismatch after round-trip: %q vs %q", restored.Name, tool.Name)
			}
		})
	}
}

func TestAvailableTools_HaveNonEmptyDescriptions(t *testing.T) {
	for _, tool := range availableTools {
		if tool.Description == "" {
			t.Errorf("tool %q has empty description", tool.Name)
		}
	}
}

func TestAvailableTools_AllHaveNames(t *testing.T) {
	for _, tool := range availableTools {
		if tool.Name == "" {
			t.Errorf("tool with empty name found")
		}
	}
}

// truncateStr truncates a string for use in test names.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
