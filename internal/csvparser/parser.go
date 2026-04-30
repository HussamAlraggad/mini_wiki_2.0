// Package csvparser provides a streaming CSV parser with chunking, context
// cancellation, column type detection, and error tolerance for large files.
package csvparser

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// ColumnType represents the inferred type of a CSV column.
type ColumnType int

const (
	ColumnString  ColumnType = iota
	ColumnInteger
	ColumnFloat
	ColumnBoolean
	ColumnDate
	ColumnEmpty
)

// Column describes a single column in the CSV.
type Column struct {
	Index     int
	Name      string    // from header, or "col_N"
	Type      ColumnType
	NullCount int
}

// Row is a single parsed CSV record.
type Row struct {
	Number  int                 // 1-based row number (after header)
	Values  []string            // positional values
	Named   map[string]string   // header name → value (nil if no header)
	Columns int                 // field count
}

// Chunk is a batch of rows yielded by the streaming parser.
type Chunk struct {
	Rows     []Row
	StartRow int
	EndRow   int
	Columns  []Column
}

// ParseError records a row-level failure.
type ParseError struct {
	Row     int
	Field   int
	Message string
	Err     error
}

// CSVConfig configures parsing behaviour.
type CSVConfig struct {
	Delimiter       rune // default ','
	HasHeader       bool // default true
	ChunkSize       int  // rows per chunk, default 100
	MaxErrors       int  // stop after N bad rows (0 = unlimited)
	LazyQuotes      bool
	FieldsPerRecord int  // -1 = variable
	Comment         rune // 0 = none
}

// DefaultConfig returns a CSVConfig with sensible defaults.
func DefaultConfig() CSVConfig {
	return CSVConfig{
		Delimiter:       ',',
		HasHeader:       true,
		ChunkSize:       100,
		LazyQuotes:      true,
		FieldsPerRecord: -1,
	}
}

// Parser is the streaming CSV parser interface.
type Parser interface {
	// Parse reads the CSV incrementally, yielding Chunks via the callback.
	// It returns column metadata and any accumulated parse errors.
	Parse(ctx context.Context, r io.Reader, cfg CSVConfig, fn func(Chunk) error) ([]Column, []ParseError, error)

	// ParseHeaders reads only the header row and returns column info.
	ParseHeaders(ctx context.Context, r io.Reader, cfg CSVConfig) ([]Column, error)
}

// New creates a new Parser.
func New() Parser {
	return &parser{}
}

type parser struct{}

func (p *parser) Parse(ctx context.Context, r io.Reader, cfg CSVConfig, fn func(Chunk) error) ([]Column, []ParseError, error) {
	// Apply defaults
	if cfg.Delimiter == 0 {
		cfg.Delimiter = ','
	}
	if cfg.ChunkSize == 0 {
		cfg.ChunkSize = 100
	}

	br := bufio.NewReaderSize(r, 256*1024)
	cr := csv.NewReader(br)
	cr.Comma = cfg.Delimiter
	cr.LazyQuotes = cfg.LazyQuotes
	cr.FieldsPerRecord = cfg.FieldsPerRecord
	if cfg.Comment != 0 {
		cr.Comment = cfg.Comment
	}

	var columns []Column
	var errs []ParseError
	rowNum := 0

	// --- Read header ---
	if cfg.HasHeader {
		record, err := cr.Read()
		if err != nil {
			return nil, nil, fmt.Errorf("read header: %w", err)
		}
		columns = make([]Column, len(record))
		for i, name := range record {
			columns[i] = Column{
				Index: i,
				Name:  strings.TrimSpace(name),
				Type:  ColumnString,
			}
		}
		rowNum = 1 // data rows start at 1
	}

	// --- Streaming body ---
	batch := make([]Row, 0, cfg.ChunkSize)
	startRow := rowNum

	for {
		select {
		case <-ctx.Done():
			// Flush remaining batch before returning
			if len(batch) > 0 {
				chk := Chunk{
					Rows:     batch,
					StartRow: startRow,
					EndRow:   rowNum - 1,
					Columns:  columns,
				}
				if err := fn(chk); err != nil {
					return columns, errs, err
				}
			}
			return columns, errs, ctx.Err()
		default:
		}

		record, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			errs = append(errs, ParseError{Row: rowNum, Err: err})
			if cfg.MaxErrors > 0 && len(errs) >= cfg.MaxErrors {
				return columns, errs, fmt.Errorf("too many parse errors (%d)", len(errs))
			}
			rowNum++
			continue
		}

		// Initialize columns from first data row if no header
		if columns == nil {
			columns = make([]Column, len(record))
			for i := range record {
				columns[i] = Column{Index: i, Name: fmt.Sprintf("col_%d", i), Type: ColumnString}
			}
			rowNum = 1
			startRow = 1
		}

		// Widen columns for variable-width rows
		for len(columns) < len(record) {
			columns = append(columns, Column{
				Index: len(columns),
				Name:  fmt.Sprintf("col_%d", len(columns)),
				Type:  ColumnString,
			})
		}

		// Update column type detection
		updateColumnTypes(columns, record)

		row := Row{
			Number:  rowNum,
			Values:  record,
			Columns: len(record),
		}
		if len(columns) > 0 && columns[0].Name != "" {
			row.Named = make(map[string]string, len(record))
			for i, v := range record {
				if i < len(columns) {
					row.Named[columns[i].Name] = v
				}
			}
		}

		batch = append(batch, row)

		// Yield chunk when batch is full
		if len(batch) >= cfg.ChunkSize {
			chk := Chunk{
				Rows:     batch,
				StartRow: startRow,
				EndRow:   rowNum,
				Columns:  columns,
			}
			if err := fn(chk); err != nil {
				return columns, errs, err
			}
			startRow = rowNum + 1
			batch = batch[:0]
		}

		rowNum++
	}

	// Flush remaining rows
	if len(batch) > 0 {
		chk := Chunk{
			Rows:     batch,
			StartRow: startRow,
			EndRow:   rowNum - 1,
			Columns:  columns,
		}
		if err := fn(chk); err != nil {
			return columns, errs, err
		}
	}

	return columns, errs, nil
}

func (p *parser) ParseHeaders(ctx context.Context, r io.Reader, cfg CSVConfig) ([]Column, error) {
	if cfg.Delimiter == 0 {
		cfg.Delimiter = ','
	}

	br := bufio.NewReaderSize(r, 64*1024)
	cr := csv.NewReader(br)
	cr.Comma = cfg.Delimiter
	cr.LazyQuotes = cfg.LazyQuotes
	cr.FieldsPerRecord = cfg.FieldsPerRecord

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	record, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read headers: %w", err)
	}

	columns := make([]Column, len(record))
	for i, name := range record {
		columns[i] = Column{
			Index: i,
			Name:  strings.TrimSpace(name),
		}
	}
	return columns, nil
}

// updateColumnTypes samples values to infer column types.
func updateColumnTypes(columns []Column, record []string) {
	for i, val := range record {
		if i >= len(columns) {
			break
		}
		val = strings.TrimSpace(val)
		if val == "" {
			columns[i].NullCount++
			continue
		}
		if columns[i].Type == ColumnEmpty {
			columns[i].Type = ColumnString
		}
		// Only upgrade type if not already determined
		if columns[i].Type == ColumnString {
			continue
		}
		detectType(columns, i, val)
	}
}

// detectType attempts to determine the column type from a sample value.
func detectType(columns []Column, i int, val string) {
	// Try integer
	if _, err := strconv.ParseInt(val, 10, 64); err == nil {
		if columns[i].Type < ColumnInteger {
			columns[i].Type = ColumnInteger
		}
		return
	}

	// Try float
	if _, err := strconv.ParseFloat(val, 64); err == nil {
		if columns[i].Type < ColumnFloat {
			columns[i].Type = ColumnFloat
		}
		return
	}

	// Try boolean
	if lower := strings.ToLower(val); lower == "true" || lower == "false" || lower == "yes" || lower == "no" {
		if columns[i].Type < ColumnBoolean {
			columns[i].Type = ColumnBoolean
		}
		return
	}

	// Try date (common formats)
	if tryParseDate(val) {
		if columns[i].Type < ColumnDate {
			columns[i].Type = ColumnDate
		}
		return
	}

	// Fallback: string
	columns[i].Type = ColumnString
}

// tryParseDate attempts common date formats.
func tryParseDate(val string) bool {
	formats := []string{
		time.RFC3339,
		"2006-01-02",
		"2006-01-02 15:04:05",
		"01/02/2006",
		"01/02/2006 15:04:05",
		"2006/01/02",
		"Jan 2, 2006",
		"January 2, 2006",
		"2 Jan 2006",
	}
	for _, f := range formats {
		if _, err := time.Parse(f, val); err == nil {
			return true
		}
	}
	return false
}
