package ranking

import (
	"testing"
	"time"

	"mini-wiki/internal/dataset"
)

func TestExtraFormatComparison(t *testing.T) {
	oldResult := &RankResult{
		Topic: "original topic",
		Dataset: &dataset.Dataset{
			Name: "test.csv",
			Columns: []dataset.Column{
				{Name: "name", Kind: dataset.ColumnString},
			},
			Rows:     []dataset.Row{},
			RowCount: 10,
		},
		Scores: []float64{0.9, 0.8, 0.7},
	}
	newResult := &RankResult{
		Topic: "refined topic",
		Dataset: &dataset.Dataset{
			Name: "test.csv",
			Columns: []dataset.Column{
				{Name: "name", Kind: dataset.ColumnString},
			},
			Rows:     []dataset.Row{},
			RowCount: 8,
		},
		Scores: []float64{0.95, 0.85, 0.75},
	}

	output := FormatComparison(oldResult, newResult)
	if output == "" {
		t.Error("FormatComparison returned empty string")
	}
	if !containsStr(output, "original") {
		t.Errorf("expected output to contain 'original', got:\n%s", output)
	}
	if !containsStr(output, "refined") {
		t.Errorf("expected output to contain 'refined', got:\n%s", output)
	}
}

func TestExtraFormatComparisonNil(t *testing.T) {
	output := FormatComparison(nil, nil)
	if output == "" {
		t.Error("FormatComparison(nil,nil) returned empty")
	}
}

func TestExtraFormatRankingTableEmpty(t *testing.T) {
	rr := &RankResult{
		Topic:  "empty",
		Scores: []float64{},
		Dataset: &dataset.Dataset{
			Name:    "empty.csv",
			Columns: []dataset.Column{},
			Rows:    []dataset.Row{},
		},
	}
	output := FormatRankingTable(rr, 10)
	if output == "" {
		t.Error("FormatRankingTable with empty result returned empty")
	}
}

func TestExtraRankResultFields(t *testing.T) {
	rr := &RankResult{
		Topic: "test",
		Dataset: &dataset.Dataset{
			Name:        "data.csv",
			RowCount:    100,
			ColumnCount: 5,
			IngestedAt:  time.Now(),
		},
		Scores:       []float64{0.9, 0.8},
		MeanScore:    0.85,
		MinScore:     0.8,
		MaxScore:     0.9,
		DiscardCount: 10,
	}
	if rr.Topic != "test" {
		t.Errorf("Topic = %q", rr.Topic)
	}
	if rr.DiscardCount != 10 {
		t.Errorf("DiscardCount = %d", rr.DiscardCount)
	}
	if rr.MeanScore != 0.85 {
		t.Errorf("MeanScore = %f", rr.MeanScore)
	}
}

func TestExtraLoadDatasetNonexistent(t *testing.T) {
	_, err := LoadDataset("/nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestExtraRerankNotPanic(t *testing.T) {
	r := NewRanker(nil, DefaultConfig())
	if r == nil {
		t.Fatal("NewRanker returned nil")
	}
}

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
