package jsonlparser

import (
	"context"
	"strings"
	"testing"
)

func TestParse_Basic(t *testing.T) {
	input := `{"name": "Alice", "role": "engineer"}
{"name": "Bob", "role": "designer"}
{"name": "Charlie", "role": "manager"}
`
	p := New()
	var rows []Row
	stats, err := p.Parse(context.Background(), strings.NewReader(input), 10, func(chunk Chunk) error {
		rows = append(rows, chunk.Rows...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalRows != 3 {
		t.Errorf("expected 3 rows, got %d", stats.TotalRows)
	}
	if len(rows) != 3 {
		t.Errorf("expected 3 rows, got %d", len(rows))
	}
	if rows[0].Data["name"] != "Alice" {
		t.Errorf("expected name=Alice, got %v", rows[0].Data["name"])
	}
}

func TestParse_EmptyLines(t *testing.T) {
	input := `{"id": 1}

{"id": 2}

{"id": 3}
`
	p := New()
	var rows []Row
	stats, err := p.Parse(context.Background(), strings.NewReader(input), 10, func(chunk Chunk) error {
		rows = append(rows, chunk.Rows...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalRows != 5 {
		t.Errorf("expected 5 total lines including empty, got %d", stats.TotalRows)
	}
	if len(rows) != 3 {
		t.Errorf("expected 3 valid rows, got %d", len(rows))
	}
}

func TestParse_Chunking(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 250; i++ {
		sb.WriteString(`{"val": "x"}`)
		sb.WriteString("\n")
	}

	p := New()
	chunkCount := 0
	_, err := p.Parse(context.Background(), strings.NewReader(sb.String()), 100, func(chunk Chunk) error {
		chunkCount++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if chunkCount != 3 {
		t.Errorf("expected 3 chunks (100+100+50), got %d", chunkCount)
	}
}

func TestParse_ContextCancellation(t *testing.T) {
	input := `{"a": 1}
{"a": 2}
{"a": 3}
`
	p := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.Parse(ctx, strings.NewReader(input), 10, func(chunk Chunk) error {
		return nil
	})
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestParse_MalformedLine(t *testing.T) {
	input := `{"valid": "yes"}
not json at all
{"valid": "also yes"}
`
	p := New()
	var rows []Row
	stats, err := p.Parse(context.Background(), strings.NewReader(input), 10, func(chunk Chunk) error {
		rows = append(rows, chunk.Rows...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalRows != 3 {
		t.Errorf("expected 3 total lines, got %d", stats.TotalRows)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 valid rows, got %d", len(rows))
	}
}

func TestExtractText(t *testing.T) {
	p := New()
	input := `{"name": "Alice", "age": 30, "skills": ["go", "python"], "active": true}`
	var capturedRows []Row
	stats, err := p.Parse(context.Background(), strings.NewReader(input), 10, func(chunk Chunk) error {
		capturedRows = append(capturedRows, chunk.Rows...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalRows != 1 {
		t.Fatalf("expected 1 row, got %d", stats.TotalRows)
	}
	if len(capturedRows) != 1 {
		t.Fatalf("expected 1 captured row, got %d", len(capturedRows))
	}

	// Test full text extraction from the actual row
	text := p.ExtractText(capturedRows[0], nil)
	if !strings.Contains(text, "Alice") {
		t.Errorf("expected text to contain 'Alice', got: %s", text)
	}
}

func TestListFields(t *testing.T) {
	input := `{"a": 1, "b": 2}
{"b": 3, "c": 4}
`
	p := New()
	var rows []Row
	_, err := p.Parse(context.Background(), strings.NewReader(input), 10, func(chunk Chunk) error {
		rows = append(rows, chunk.Rows...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	fields := ListFields(rows)
	if len(fields) != 3 {
		t.Errorf("expected 3 unique fields (a,b,c), got %d: %v", len(fields), fields)
	}
}

func TestBuildRowText_Nested(t *testing.T) {
	p := &parser{}
	data := map[string]any{
		"user": map[string]any{
			"name": "Alice",
			"contact": map[string]any{
				"email": "alice@test.com",
			},
		},
	}
	text := p.buildRowText(data)
	if !strings.Contains(text, "alice@test.com") {
		t.Error("expected nested email in extracted text")
	}
	if !strings.Contains(text, "user.contact.email") {
		t.Error("expected dot-notation key in extracted text")
	}
}
