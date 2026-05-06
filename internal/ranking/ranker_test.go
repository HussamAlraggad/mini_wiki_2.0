package ranking

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mini-wiki/internal/dataset"
)

// mockLLM implements LLMClient for testing.
type mockLLM struct {
	responses map[string]string
}

func (m *mockLLM) Generate(ctx context.Context, model, prompt string) (string, error) {
	if m.responses != nil {
		for key, resp := range m.responses {
			if strings.Contains(prompt, key) {
				return resp, nil
			}
		}
	}
	return "0.5", nil
}

func TestParseScore(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"0.85", 0.85},
		{"0.5", 0.5},
		{"1.0", 1.0},
		{"0.0", 0.0},
		{" 0.75 ", 0.75},
		{"The score is 0.9", 0.9},
		{"-0.1", 0.0},   // clamped
		{"1.5", 1.0},    // clamped
		{"abc", 0.0},    // non-numeric
		{"", 0.0},       // empty
	}
	for _, tt := range tests {
		if got := parseScore(tt.input); got != tt.want {
			t.Errorf("parseScore(%q) = %f, want %f", tt.input, got, tt.want)
		}
	}
}

func TestScoreAll_Basic(t *testing.T) {
	mock := &mockLLM{responses: map[string]string{
		"Row #0": "0.9",
		"Row #1": "0.3",
	}}
	r := NewRanker(mock, DefaultConfig())

	data := &dataset.Dataset{
		Name: "test",
		Columns: []dataset.Column{
			{Name: "text", Kind: dataset.ColumnString},
		},
		Rows: []dataset.Row{
			{Index: 0, Data: map[string]interface{}{"text": "highly relevant"}},
			{Index: 1, Data: map[string]interface{}{"text": "not relevant"}},
		},
		RowCount: 2,
	}

	result, err := r.ScoreAll(context.Background(), data, "test topic")
	if err != nil {
		t.Fatal(err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if len(result.Scores) != 2 {
		t.Errorf("expected 2 scores, got %d", len(result.Scores))
	}

	// First row should be sorted highest first
	if result.Dataset.Rows[0].Data["text"] != "highly relevant" {
		t.Errorf("expected highly relevant first, got %v", result.Dataset.Rows[0].Data["text"])
	}
}

func TestScoreAll_EmptyData(t *testing.T) {
	mock := &mockLLM{}
	r := NewRanker(mock, DefaultConfig())
	_, err := r.ScoreAll(context.Background(), nil, "topic")
	if err == nil {
		t.Error("expected error for nil data")
	}
}

func TestScoreAll_ContextCancellation(t *testing.T) {
	mock := &mockLLM{}
	r := NewRanker(mock, DefaultConfig())

	data := &dataset.Dataset{
		Rows: []dataset.Row{
			{Index: 0, Data: map[string]interface{}{"text": "test"}},
		},
		RowCount: 1,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.ScoreAll(ctx, data, "topic")
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestFormatRankingTable(t *testing.T) {
	result := &RankResult{
		Topic:     "test topic",
		Scores:    []float64{0.9, 0.5},
		MeanScore: 0.7,
		MinScore:  0.5,
		MaxScore:  0.9,
		Dataset: &dataset.Dataset{
			Columns: []dataset.Column{
				{Name: "name", Kind: dataset.ColumnString},
				{Name: "relevance_score", Kind: dataset.ColumnFloat},
			},
			Rows: []dataset.Row{
				{Index: 0, Data: map[string]interface{}{"name": "Alice", "relevance_score": 0.9}},
				{Index: 1, Data: map[string]interface{}{"name": "Bob", "relevance_score": 0.5}},
			},
			RowCount: 2,
		},
	}

	table := FormatRankingTable(result, 5)
	if !strings.Contains(table, "Alice") {
		t.Error("table should contain row data")
	}
	if !strings.Contains(table, "0.90") {
		t.Error("table should contain scores")
	}
}

func TestResultsToJSON(t *testing.T) {
	json := ResultsToJSON([]float64{0.9, 0.5, 0.7}, "test")
	if !strings.Contains(json, "0.9") {
		t.Error("JSON should contain score values")
	}
	if !strings.Contains(json, "test") {
		t.Error("JSON should contain topic")
	}
}

func TestLoadDataset_NoProjectDir(t *testing.T) {
	_, err := LoadDataset("")
	if err == nil {
		t.Error("expected error for empty project dir")
	}
}

func TestParseCSV(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "test.csv")
	csvContent := "name,age\nAlice,30\nBob,25\n"
	if err := os.WriteFile(csvPath, []byte(csvContent), 0644); err != nil {
		t.Fatal(err)
	}

	ds, err := parseCSV(csvPath)
	if err != nil {
		t.Fatal(err)
	}

	if ds.RowCount != 2 {
		t.Errorf("expected 2 rows, got %d", ds.RowCount)
	}
	if ds.ColumnCount != 2 {
		t.Errorf("expected 2 columns, got %d", ds.ColumnCount)
	}
	if ds.Rows[0].Data["name"] != "Alice" {
		t.Errorf("expected Alice, got %v", ds.Rows[0].Data["name"])
	}
}

func TestParseJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	content := `{"name": "Alice", "age": 30}
{"name": "Bob", "age": 25}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	ds, err := parseJSONL(path)
	if err != nil {
		t.Fatal(err)
	}

	if ds.RowCount != 2 {
		t.Errorf("expected 2 rows, got %d", ds.RowCount)
	}
	if ds.ColumnCount != 2 {
		t.Errorf("expected 2 columns, got %d", ds.ColumnCount)
	}
}

func TestParseText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := "hello world"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	ds, err := parseText(path)
	if err != nil {
		t.Fatal(err)
	}

	if ds.RowCount != 1 {
		t.Errorf("expected 1 row, got %d", ds.RowCount)
	}
	if ds.ColumnCount != 1 {
		t.Errorf("expected 1 column, got %d", ds.ColumnCount)
	}
}

func TestDetectColumnTypes(t *testing.T) {
	ds := &dataset.Dataset{
		Columns: []dataset.Column{
			{Name: "name", Kind: dataset.ColumnString},
			{Name: "age", Kind: dataset.ColumnString},
			{Name: "score", Kind: dataset.ColumnString},
		},
		Rows: []dataset.Row{
			{Data: map[string]interface{}{"name": "Alice", "age": "30", "score": "95.5"}},
			{Data: map[string]interface{}{"name": "Bob", "age": "25", "score": "87.3"}},
		},
	}
	detectColumnTypes(ds)

	if ds.Columns[0].Kind != dataset.ColumnString {
		t.Errorf("name should remain string, got %v", ds.Columns[0].Kind)
	}
	if ds.Columns[1].Kind != dataset.ColumnInteger {
		t.Errorf("age should be integer, got %v", ds.Columns[1].Kind)
	}
	if ds.Columns[2].Kind != dataset.ColumnFloat {
		t.Errorf("score should be float, got %v", ds.Columns[2].Kind)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Model != "llama3.1:8b" {
		t.Errorf("expected default model llama3.1:8b, got %s", cfg.Model)
	}
	if cfg.MaxRows != 10000 {
		t.Errorf("expected MaxRows=10000, got %d", cfg.MaxRows)
	}
}

func TestNewRanker(t *testing.T) {
	mock := &mockLLM{}
	r := NewRanker(mock, DefaultConfig())
	if r == nil {
		t.Fatal("expected non-nil ranker")
	}
}
