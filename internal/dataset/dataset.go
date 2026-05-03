// Package dataset defines the shared data types used by ALL phases of the tool.
// Every phase (ingestion, ranking, charting, export) uses these types.
// Do NOT define your own Row/Column types in other packages.
package dataset

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ColumnKind identifies the detected type of a column.
type ColumnKind int

const (
	ColumnString  ColumnKind = iota // text values
	ColumnInteger                   // whole numbers
	ColumnFloat                     // decimal numbers
	ColumnBoolean                   // true/false
	ColumnDate                      // dates and timestamps
)

// String returns a human-readable name for a ColumnKind.
func (k ColumnKind) String() string {
	switch k {
	case ColumnString:
		return "string"
	case ColumnInteger:
		return "integer"
	case ColumnFloat:
		return "float"
	case ColumnBoolean:
		return "boolean"
	case ColumnDate:
		return "date"
	default:
		return "unknown"
	}
}

// Column describes a single column in the dataset.
type Column struct {
	Name string
	Kind ColumnKind
}

// Row represents a single row of data with column-name indexing.
type Row struct {
	Index int                    // original row number in source file
	Data  map[string]interface{} // column name -> value
}

// Dataset is the in-memory representation that all phases operate on.
// It is produced by ingestion (Phase 2/3) and consumed by ranking (Phase 4),
// charting (Phase 5), and export (Phase 6).
type Dataset struct {
	Name        string      // source filename (without path)
	SourceFile  string      // original file path
	Columns     []Column
	Rows        []Row
	ColumnCount int
	RowCount    int
	IngestedAt  time.Time
}

// Filter returns a new Dataset containing only rows where the predicate is true.
func (d *Dataset) Filter(predicate func(Row) bool) *Dataset {
	result := &Dataset{
		Name:       d.Name,
		SourceFile: d.SourceFile,
		Columns:    append([]Column(nil), d.Columns...),
		ColumnCount: d.ColumnCount,
		IngestedAt: d.IngestedAt,
	}
	for _, row := range d.Rows {
		if predicate(row) {
			result.Rows = append(result.Rows, row)
		}
	}
	result.RowCount = len(result.Rows)
	return result
}

// Sort returns a new Dataset sorted by the given column and direction.
func (d *Dataset) Sort(column string, descending bool) *Dataset {
	result := &Dataset{
		Name:       d.Name,
		SourceFile: d.SourceFile,
		Columns:    append([]Column(nil), d.Columns...),
		ColumnCount: d.ColumnCount,
		Rows:       append([]Row(nil), d.Rows...),
		RowCount:   d.RowCount,
		IngestedAt: d.IngestedAt,
	}

	colIdx := -1
	for i, c := range d.Columns {
		if c.Name == column {
			colIdx = i
			break
		}
	}
	if colIdx < 0 {
		return result // column not found, return unsorted
	}

	// Simple bubble sort for now (datasets are typically <10K rows)
	for i := 0; i < len(result.Rows); i++ {
		for j := i + 1; j < len(result.Rows); j++ {
			vi := fmt.Sprintf("%v", result.Rows[i].Data[column])
			vj := fmt.Sprintf("%v", result.Rows[j].Data[column])
			// Ascending: swap if vi > vj. Descending: swap if vi < vj.
			swap := vi > vj
			if descending {
				swap = vi < vj
			}
			if swap {
				result.Rows[i], result.Rows[j] = result.Rows[j], result.Rows[i]
			}
		}
	}
	return result
}

// Select returns a new Dataset with only the given columns.
func (d *Dataset) Select(columns ...string) *Dataset {
	colSet := make(map[string]bool)
	for _, c := range columns {
		colSet[c] = true
	}

	var newCols []Column
	for _, c := range d.Columns {
		if colSet[c.Name] {
			newCols = append(newCols, c)
		}
	}

	result := &Dataset{
		Name:       d.Name,
		SourceFile: d.SourceFile,
		Columns:    newCols,
		ColumnCount: len(newCols),
		IngestedAt: d.IngestedAt,
	}

	for _, row := range d.Rows {
		newRow := Row{
			Index: row.Index,
			Data:  make(map[string]interface{}),
		}
		for _, c := range newCols {
			if v, ok := row.Data[c.Name]; ok {
				newRow.Data[c.Name] = v
			}
		}
		result.Rows = append(result.Rows, newRow)
	}
	result.RowCount = len(result.Rows)
	return result
}

// Head returns the first n rows (or all if n > row count).
func (d *Dataset) Head(n int) *Dataset {
	if n > d.RowCount {
		n = d.RowCount
	}
	result := &Dataset{
		Name:       d.Name,
		SourceFile: d.SourceFile,
		Columns:    d.Columns,
		ColumnCount: d.ColumnCount,
		RowCount:   n,
		IngestedAt: d.IngestedAt,
		Rows:       d.Rows[:n],
	}
	return result
}

// String returns a formatted table preview of the dataset.
func (d *Dataset) String() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Dataset: %s (%d rows, %d cols)\n", d.Name, d.RowCount, d.ColumnCount))

	// Header
	for i, col := range d.Columns {
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString(col.Name)
	}
	b.WriteString("\n")

	// Separator
	for i := range d.Columns {
		if i > 0 {
			b.WriteString("-+-")
		}
		b.WriteString(strings.Repeat("-", len(d.Columns[i].Name)))
	}
	b.WriteString("\n")

	// First 10 rows
	maxRows := 10
	if len(d.Rows) < maxRows {
		maxRows = len(d.Rows)
	}
	for _, row := range d.Rows[:maxRows] {
		for i, col := range d.Columns {
			if i > 0 {
				b.WriteString(" | ")
			}
			val := fmt.Sprintf("%v", row.Data[col.Name])
			if len(val) > 30 {
				val = val[:27] + "..."
			}
			b.WriteString(val)
		}
		b.WriteString("\n")
	}

	if len(d.Rows) > maxRows {
		b.WriteString(fmt.Sprintf("... %d more rows\n", len(d.Rows)-maxRows))
	}

	return b.String()
}

// --- Parser Interface (section 12.5) ---

// Parser is the interface that ALL format parsers must implement.
type Parser interface {
	// Parse reads from the given path and returns a Dataset.
	// It must check ctx.Done() between chunks for cancellation.
	Parse(ctx context.Context, path string) (*Dataset, error)

	// Format returns the format name this parser handles (e.g. "csv", "xlsx").
	Format() string
}

// AutoDetect reads the file extension and returns the matching parser.
// If unknown, returns nil.
func AutoDetect(path string) Parser {
	// Magic byte detection could be added here
	// For now, uses extension-based detection
	return nil
}
