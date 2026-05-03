package dataset

import (
	"testing"
	"time"
)

func TestColumnKind_String(t *testing.T) {
	tests := []struct {
		kind ColumnKind
		want string
	}{
		{ColumnString, "string"},
		{ColumnInteger, "integer"},
		{ColumnFloat, "float"},
		{ColumnBoolean, "boolean"},
		{ColumnDate, "date"},
		{ColumnKind(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Errorf("ColumnKind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestDataset_Filter(t *testing.T) {
	ds := &Dataset{
		Name: "test",
		Columns: []Column{
			{Name: "name", Kind: ColumnString},
			{Name: "score", Kind: ColumnFloat},
		},
		Rows: []Row{
			{Index: 0, Data: map[string]interface{}{"name": "Alice", "score": 0.9}},
			{Index: 1, Data: map[string]interface{}{"name": "Bob", "score": 0.5}},
			{Index: 2, Data: map[string]interface{}{"name": "Charlie", "score": 0.7}},
		},
		RowCount: 3,
		IngestedAt: time.Now(),
	}

	filtered := ds.Filter(func(r Row) bool {
		if s, ok := r.Data["score"].(float64); ok {
			return s >= 0.7
		}
		return false
	})

	if filtered.RowCount != 2 {
		t.Errorf("expected 2 filtered rows, got %d", filtered.RowCount)
	}
}

func TestDataset_Sort(t *testing.T) {
	ds := &Dataset{
		Name: "test",
		Columns: []Column{
			{Name: "name", Kind: ColumnString},
			{Name: "score", Kind: ColumnFloat},
		},
		Rows: []Row{
			{Index: 0, Data: map[string]interface{}{"name": "Alice", "score": 0.9}},
			{Index: 1, Data: map[string]interface{}{"name": "Bob", "score": 0.5}},
			{Index: 2, Data: map[string]interface{}{"name": "Charlie", "score": 0.7}},
		},
		RowCount: 3,
	}

	sorted := ds.Sort("score", true)
	if sorted.Rows[0].Data["name"] != "Alice" {
		t.Errorf("expected Alice first (highest score), got %v", sorted.Rows[0].Data["name"])
	}
}

func TestDataset_Select(t *testing.T) {
	ds := &Dataset{
		Columns: []Column{
			{Name: "a", Kind: ColumnString},
			{Name: "b", Kind: ColumnString},
			{Name: "c", Kind: ColumnString},
		},
		Rows: []Row{
			{Data: map[string]interface{}{"a": "1", "b": "2", "c": "3"}},
		},
		ColumnCount: 3,
		RowCount:    1,
	}

	selected := ds.Select("a", "c")
	if selected.ColumnCount != 2 {
		t.Errorf("expected 2 columns, got %d", selected.ColumnCount)
	}
	if selected.Rows[0].Data["a"] != "1" {
		t.Errorf("expected a=1, got %v", selected.Rows[0].Data["a"])
	}
	if _, ok := selected.Rows[0].Data["b"]; ok {
		t.Error("column b should not be in selected dataset")
	}
}

func TestDataset_Head(t *testing.T) {
	ds := &Dataset{
		RowCount: 10,
		Rows:     make([]Row, 10),
	}
	head := ds.Head(3)
	if head.RowCount != 3 {
		t.Errorf("expected 3 rows, got %d", head.RowCount)
	}
}

func TestDataset_String(t *testing.T) {
	ds := &Dataset{
		Name: "test",
		Columns: []Column{
			{Name: "id", Kind: ColumnInteger},
			{Name: "value", Kind: ColumnString},
		},
		Rows: []Row{
			{Index: 0, Data: map[string]interface{}{"id": 1, "value": "hello"}},
		},
		RowCount: 1,
		ColumnCount: 2,
	}
	s := ds.String()
	if len(s) == 0 {
		t.Error("String() returned empty")
	}
	if !contains(s, "hello") {
		t.Error("String() should contain row data")
	}
}

func TestAutoDetect(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"data.csv", "csv"},
		{"data.tsv", "csv"},
		{"data.jsonl", "jsonl"},
		{"data.ndjson", "jsonl"},
		{"data.xlsx", "xlsx"},
		{"data.ods", "ods"},
		{"data.txt", "txt"},
		{"file.md", "txt"},
		{"data.unknown", ""},
	}
	for _, tt := range tests {
		if got := AutoDetect(tt.path); got != tt.want {
			t.Errorf("AutoDetect(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
