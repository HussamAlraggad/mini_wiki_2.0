package dataset

import (
	"os"
	"testing"
)

func TestExtraDetectFormat(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"data.csv", "CSV (comma-separated values)"},
		{"data.json", "JSONL (JSON lines)"}, // AutoDetect treats .json as jsonl by default
		{"data.jsonl", "JSONL (JSON lines)"},
		{"data.xlsx", "Excel (.xlsx)"},
		{"data.ods", "LibreOffice Calc (.ods)"},
		{"data.txt", "Text file"},
		{"data.unknown", "Unknown format"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := DetectFormat(tt.path)
			if got != tt.want {
				t.Errorf("DetectFormat(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestExtraIsJSONArrayWithFile(t *testing.T) {
	dir := t.TempDir()

	// JSON array should return true
	arrayPath := dir + "/array.json"
	os.WriteFile(arrayPath, []byte("[1, 2, 3]"), 0644)
	if !isJSONArray(arrayPath) {
		t.Errorf("isJSONArray should be true for JSON array file")
	}

	// JSON object should return false
	objPath := dir + "/obj.json"
	os.WriteFile(objPath, []byte(`{"key": "value"}`), 0644)
	if isJSONArray(objPath) {
		t.Errorf("isJSONArray should be false for JSON object file")
	}

	// Empty file should return false
	emptyPath := dir + "/empty.json"
	os.WriteFile(emptyPath, []byte(""), 0644)
	if isJSONArray(emptyPath) {
		t.Errorf("isJSONArray should be false for empty file")
	}

	// Nonexistent file should return false
	if isJSONArray(dir + "/nonexistent.json") {
		t.Errorf("isJSONArray should be false for nonexistent file")
	}
}

func TestExtraAutoExt(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"data.csv", ".csv"},
		{"data.tsv", ".tsv"},
		{"data.json", ".json"},
		{"data.jsonl", ".jsonl"},
		{"data.xlsx", ".xlsx"},
		{"data.ods", ".ods"},
		{"data.txt", ".txt"},
		{"data.md", ".md"},
		{"data", ""},
		{"/path/to/file.go", ".go"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := AutoExt(tt.path)
			if got != tt.want {
				t.Errorf("AutoExt(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
