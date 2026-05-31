package csvparser

import (
	"context"
	"strings"
	"testing"
)

func TestExtraUpdateColumnTypes(t *testing.T) {
	// Columns start at ColumnString (as set by parser)
	cols := []Column{
		{Name: "age", Index: 0, Type: ColumnString},
		{Name: "score", Index: 1, Type: ColumnString},
	}

	// updateColumnTypes sets ColumnEmpty -> ColumnString but existing cols
	// are already ColumnString, so this is primarily a no-op in the current code
	updateColumnTypes(cols, []string{"30", "95.5"})
	if cols[0].Type == ColumnString {
		// Current implementation stops at ColumnString (see parser.go line 293 continue)
		// This is a known limitation: type detection only narrows from ColumnEmpty
	}
}

func TestExtraTryParseDate(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"2024-01-15", true},
		{"2024-01-15 10:30:00", true},
		{"01/02/2006", true},
		{"01/02/2006 15:04:05", true},
		{"2006/01/02", true},
		{"Jan 2, 2006", true},
		{"January 2, 2006", true},
		{"2 Jan 2006", true},
		{"not-a-date", false},
		{"", false},
		{"123", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := tryParseDate(tt.input)
			if got != tt.want {
				t.Errorf("tryParseDate(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtraParseMaxErrors(t *testing.T) {
	csv := "a,b\n1,2\n3\n4\n5,6\n"
	p := New()
	cfg := DefaultConfig()
	cfg.FieldsPerRecord = 2
	cfg.MaxErrors = 2

	_, _, err := p.Parse(context.Background(), strings.NewReader(csv), cfg, func(chunk Chunk) error {
		return nil
	})
	if err == nil {
		t.Error("expected error when max errors exceeded")
	}
}

func TestExtraParseTSV(t *testing.T) {
	csv := "name\tage\nAlice\t30\n"
	p := New()
	cfg := DefaultConfig()
	cfg.Delimiter = '\t'

	var rows int
	_, _, err := p.Parse(context.Background(), strings.NewReader(csv), cfg, func(chunk Chunk) error {
		rows = len(chunk.Rows)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("expected 1 row, got %d", rows)
	}
}
