package ranking

import (
	"context"
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

func TestLoadDataset_NotImplemented(t *testing.T) {
	_, err := LoadDataset("/tmp/nonexistent")
	if err == nil {
		t.Error("expected error (not implemented)")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Model != "qwen2.5-coder" {
		t.Errorf("expected default model qwen2.5-coder, got %s", cfg.Model)
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
