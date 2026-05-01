// Package jsonlparser provides streaming parsing for JSONL (JSON Lines) files.
// Each line in a JSONL file is a separate JSON object.
package jsonlparser

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Row represents a single parsed JSONL row as a flat key-value map.
type Row struct {
	Number int               // 1-based line number
	Data   map[string]any    // parsed JSON fields
	Text   string            // all text values concatenated for LLM context
	Size   int
}

// Chunk is a batch of rows yielded by the streaming parser.
type Chunk struct {
	Rows     []Row
	StartRow int
	EndRow   int
	Fields   []string // discovered field names
}

// ParseStats holds metadata about a parsed JSONL file.
type ParseStats struct {
	TotalRows  int
	TotalBytes int64
	Fields     []string
	Sample     map[string]any // first row as sample
}

// Parser is the streaming JSONL parser interface.
type Parser interface {
	// Parse reads JSONL incrementally, yielding Chunks via callback.
	Parse(ctx context.Context, r io.Reader, chunkSize int, fn func(Chunk) error) (*ParseStats, error)

	// ExtractText converts a Row to a single text string for LLM consumption.
	ExtractText(row Row, includeFields []string) string
}

// New creates a new JSONL parser.
func New() Parser {
	return &parser{}
}

type parser struct{}

func (p *parser) Parse(ctx context.Context, r io.Reader, chunkSize int, fn func(Chunk) error) (*ParseStats, error) {
	if chunkSize <= 0 {
		chunkSize = 100
	}

	scanner := bufio.NewScanner(r)
	// Increase scan buffer for long JSON lines (up to 10MB per line)
	scanner.Buffer(make([]byte, 0, 1024*64), 10*1024*1024)

	var fields []string
	fieldSet := make(map[string]bool)
	var sample map[string]any
	var totalBytes int64
	rowNum := 0
	batch := make([]Row, 0, chunkSize)
	startRow := 1

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			rowNum++
			continue
		}

		totalBytes += int64(len(line))
		rowNum++

		var data map[string]any
		if err := json.Unmarshal([]byte(line), &data); err != nil {
			// Skip malformed lines
			continue
		}

		// Collect field names
		if len(fieldSet) == 0 {
			sample = data
		}
		for k := range data {
			if !fieldSet[k] {
				fieldSet[k] = true
				fields = append(fields, k)
			}
		}

		// Build text content
		text := p.buildRowText(data)

		row := Row{
			Number: rowNum,
			Data:   data,
			Text:   text,
			Size:   len(line),
		}

		batch = append(batch, row)

		if len(batch) >= chunkSize {
			chk := Chunk{
				Rows:     batch,
				StartRow: startRow,
				EndRow:   rowNum,
				Fields:   fields,
			}
			if err := fn(chk); err != nil {
				return nil, err
			}
			startRow = rowNum + 1
			batch = batch[:0]
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}

	// Flush remaining
	if len(batch) > 0 {
		chk := Chunk{
			Rows:     batch,
			StartRow: startRow,
			EndRow:   rowNum,
			Fields:   fields,
		}
		if err := fn(chk); err != nil {
			return nil, err
		}
	}

	return &ParseStats{
		TotalRows:  rowNum,
		TotalBytes: totalBytes,
		Fields:     fields,
		Sample:     sample,
	}, nil
}

func (p *parser) ExtractText(row Row, includeFields []string) string {
	if len(includeFields) == 0 {
		return row.Text
	}

	var parts []string
	for _, field := range includeFields {
		if val, ok := row.Data[field]; ok {
			if s, ok := val.(string); ok {
				parts = append(parts, s)
			}
		}
	}
	return strings.Join(parts, "\n")
}

// buildRowText concatenates all string field values from a JSON object.
func (p *parser) buildRowText(data map[string]any) string {
	if len(data) == 0 {
		return ""
	}
	var parts []string
	for k, v := range data {
		switch val := v.(type) {
		case string:
			if strings.TrimSpace(val) != "" {
				parts = append(parts, fmt.Sprintf("%s: %s", k, val))
			}
		case float64:
			parts = append(parts, fmt.Sprintf("%s: %v", k, val))
		case bool:
			parts = append(parts, fmt.Sprintf("%s: %v", k, val))
		case map[string]any:
			// Flatten nested objects
			if nested := flattenMap(val, k); nested != "" {
				parts = append(parts, nested)
			}
		case []any:
			if len(val) > 0 {
				var items []string
				for _, item := range val {
					if s, ok := item.(string); ok {
						items = append(items, s)
					}
				}
				if len(items) > 0 {
					parts = append(parts, fmt.Sprintf("%s: %s", k, strings.Join(items, ", ")))
				}
			}
		}
	}
	return strings.Join(parts, "\n")
}

// flattenMap recursively flattens nested maps into dot-notation.
func flattenMap(data map[string]any, prefix string) string {
	var parts []string
	for k, v := range data {
		key := prefix + "." + k
		switch val := v.(type) {
		case string:
			if strings.TrimSpace(val) != "" {
				parts = append(parts, fmt.Sprintf("%s: %s", key, val))
			}
		case map[string]any:
			if nested := flattenMap(val, key); nested != "" {
				parts = append(parts, nested)
			}
		default:
			parts = append(parts, fmt.Sprintf("%s: %v", key, val))
		}
	}
	return strings.Join(parts, "\n")
}

// ListFields returns all unique field names across a set of rows.
func ListFields(rows []Row) []string {
	seen := make(map[string]bool)
	var fields []string
	for _, row := range rows {
		for k := range row.Data {
			if !seen[k] {
				seen[k] = true
				fields = append(fields, k)
			}
		}
	}
	return fields
}
