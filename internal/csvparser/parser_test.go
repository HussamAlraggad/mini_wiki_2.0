package csvparser

import (
	"context"
	"strings"
	"testing"
)

func TestParse_Basic(t *testing.T) {
	csv := "name,age\nAlice,30\nBob,25\n"
	p := New()
	chunks := 0
	var lastChunk Chunk

	_, errs, err := p.Parse(context.Background(), strings.NewReader(csv), DefaultConfig(), func(chunk Chunk) error {
		chunks++
		lastChunk = chunk
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if chunks != 1 {
		t.Errorf("expected 1 chunk, got %d", chunks)
	}
	if len(lastChunk.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(lastChunk.Rows))
	}
	if len(lastChunk.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(lastChunk.Columns))
	}
	if lastChunk.Columns[0].Name != "name" {
		t.Errorf("expected column 0 name 'name', got %q", lastChunk.Columns[0].Name)
	}
}

func TestParse_NoHeader(t *testing.T) {
	csv := "Alice,30\nBob,25\n"
	p := New()
	cfg := DefaultConfig()
	cfg.HasHeader = false

	_, _, err := p.Parse(context.Background(), strings.NewReader(csv), cfg, func(chunk Chunk) error {
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestParse_Chunking(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("val\n")
	for i := 0; i < 250; i++ {
		sb.WriteString("x\n")
	}

	p := New()
	cfg := DefaultConfig()
	cfg.ChunkSize = 100

	chunkCount := 0
	_, _, err := p.Parse(context.Background(), strings.NewReader(sb.String()), cfg, func(chunk Chunk) error {
		chunkCount++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// 250 data rows with chunks of 100 = 3 chunks (100 + 100 + 50)
	if chunkCount != 3 {
		t.Errorf("expected 3 chunks, got %d", chunkCount)
	}
}

func TestParse_ContextCancellation(t *testing.T) {
	csv := "a,b\n1,2\n3,4\n5,6\n"
	p := New()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediate cancel

	_, _, err := p.Parse(ctx, strings.NewReader(csv), DefaultConfig(), func(chunk Chunk) error {
		return nil
	})
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestParse_MalformedRows(t *testing.T) {
	csv := "a,b\n1,2\n3\n4,5,6\n7,8\n"
	p := New()
	cfg := DefaultConfig()
	cfg.FieldsPerRecord = 2 // enforce exactly 2 fields per row
	cfg.MaxErrors = 5

	_, errs, err := p.Parse(context.Background(), strings.NewReader(csv), cfg, func(chunk Chunk) error {
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) == 0 {
		t.Error("expected parse errors for malformed rows")
	}
}

func TestParse_CustomDelimiter(t *testing.T) {
	csv := "name;age\nAlice;30\nBob;25\n"
	p := New()
	cfg := DefaultConfig()
	cfg.Delimiter = ';'

	var rows []Row
	_, _, err := p.Parse(context.Background(), strings.NewReader(csv), cfg, func(chunk Chunk) error {
		rows = append(rows, chunk.Rows...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rows))
	}
}

func TestParse_QuotedFields(t *testing.T) {
	csv := `name,description
Alice,"hello, world"
Bob,"line1
line2"
`
	p := New()
	var rows []Row
	_, _, err := p.Parse(context.Background(), strings.NewReader(csv), DefaultConfig(), func(chunk Chunk) error {
		rows = append(rows, chunk.Rows...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rows))
	}
}

func TestParseHeaders(t *testing.T) {
	csv := "name,age,city\nAlice,30,NYC\n"
	p := New()

	cols, err := p.ParseHeaders(context.Background(), strings.NewReader(csv), DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 3 {
		t.Errorf("expected 3 columns, got %d", len(cols))
	}
	if cols[0].Name != "name" || cols[1].Name != "age" || cols[2].Name != "city" {
		t.Errorf("unexpected column names: %v", cols)
	}
}

func TestParse_EmptyFile(t *testing.T) {
	p := New()
	_, _, err := p.Parse(context.Background(), strings.NewReader(""), DefaultConfig(), func(chunk Chunk) error {
		return nil
	})
	if err == nil {
		t.Error("expected error for empty file")
	}
}

func TestParse_LargeChunk(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("val\n")
	for i := 0; i < 1000; i++ {
		sb.WriteString("data\n")
	}

	p := New()
	cfg := DefaultConfig()
	cfg.ChunkSize = 10000 // one big chunk

	chunks := 0
	var rows int
	_, _, err := p.Parse(context.Background(), strings.NewReader(sb.String()), cfg, func(chunk Chunk) error {
		chunks++
		rows = len(chunk.Rows)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if chunks != 1 {
		t.Errorf("expected 1 chunk, got %d", chunks)
	}
	if rows != 1000 {
		t.Errorf("expected 1000 rows, got %d", rows)
	}
}
