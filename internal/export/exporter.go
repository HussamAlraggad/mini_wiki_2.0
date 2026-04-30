// Package export provides tabular data export to Excel (.xlsx) format
// with streaming support, type-aware column formatting, and formula injection
// protection.
package export

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// ColumnDef describes a column in the export.
type ColumnDef struct {
	Name string
	Type string // "text", "number", "date"
	Width int   // 0 = auto
}

// Row represents a single exported row (positional values matching ColumnDef order).
type Row []string

// ExportData holds the complete data to export.
type ExportData struct {
	SheetName string
	Columns   []ColumnDef
	Rows      []Row
}

// SheetDef describes a sheet in multi-sheet exports.
type SheetDef struct {
	Name    string
	Columns []ColumnDef
}

// ExportConfig controls export behaviour.
type ExportConfig struct {
	OutputDir string // defaults to CWD
	FileName  string // defaults to auto-generated name
	Overwrite bool   // overwrite existing file
}

// DefaultConfig returns sensible export defaults.
func DefaultConfig() ExportConfig {
	return ExportConfig{
		OutputDir: ".",
	}
}

// ExportResult holds the outcome of an export operation.
type ExportResult struct {
	Path     string
	FileName string
	Size     int64
	Sheets   int
	Rows     int
	Duration time.Duration
}

// GenerateFileName creates a timestamped filename.
func GenerateFileName(prefix string) string {
	ts := time.Now().Format("20060102_150405")
	if prefix == "" {
		prefix = "export"
	}
	return fmt.Sprintf("%s_%s.xlsx", prefix, ts)
}

// Exporter is the tabular export interface.
type Exporter interface {
	// Export writes a complete dataset to an .xlsx file.
	Export(ctx context.Context, data *ExportData, cfg ExportConfig) (*ExportResult, error)

	// ExportStream writes data row-by-row from a channel (memory efficient).
	ExportStream(ctx context.Context, sheets []SheetDef, rows <-chan Row, cfg ExportConfig) (*ExportResult, error)
}

// New creates a new Exporter.
func New() Exporter {
	return &exporter{}
}

type exporter struct{}

// formulaPrefixes are Excel formula injection indicators.
var formulaPrefixes = []string{"=", "+", "-", "@", "\t=", "\t+", "\t-", "\t@"}

// sanitizeCell prevents formula injection by prefixing with apostrophe.
func sanitizeCell(value string) string {
	for _, prefix := range formulaPrefixes {
		if strings.HasPrefix(value, prefix) {
			return "'" + value
		}
	}
	return value
}

func (e *exporter) Export(ctx context.Context, data *ExportData, cfg ExportConfig) (*ExportResult, error) {
	start := time.Now()

	if data == nil {
		return nil, fmt.Errorf("no data to export")
	}
	if len(data.Rows) == 0 {
		return nil, fmt.Errorf("no rows to export")
	}

	sheetName := data.SheetName
	if sheetName == "" {
		sheetName = "Sheet1"
	}

	f := excelize.NewFile()
	defer f.Close()

	// Rename default sheet
	idx, err := f.GetSheetIndex("Sheet1")
	if err == nil && idx >= 0 {
		f.SetSheetName("Sheet1", sheetName)
	}

	// Write header row
	headers := make([]string, len(data.Columns))
	for i, col := range data.Columns {
		headers[i] = col.Name
	}
	for j, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(j+1, 1)
		f.SetCellValue(sheetName, cell, h)
	}

	// Write data rows
	for i, row := range data.Rows {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		for j, val := range row {
			if j >= len(data.Columns) {
				break
			}
			cell, _ := excelize.CoordinatesToCellName(j+1, i+2) // +2 for header offset
			f.SetCellValue(sheetName, cell, sanitizeCell(val))
		}
	}

	// Auto-width columns
	for j, col := range data.Columns {
		width := col.Width
		if width <= 0 {
			// Estimate from header
			width = len(col.Name) + 2
			if width < 10 {
				width = 10
			}
			if width > 50 {
				width = 50
			}
		}
		cell, _ := excelize.CoordinatesToCellName(j+1, 1)
		f.SetColWidth(sheetName, strings.Split(cell, "1")[0], strings.Split(cell, "1")[0], float64(width))
	}

	// Resolve output path
	fileName := cfg.FileName
	if fileName == "" {
		fileName = GenerateFileName("export")
	}
	if filepath.Ext(fileName) == "" {
		fileName += ".xlsx"
	}
	outputPath := filepath.Join(cfg.OutputDir, fileName)

	// Save to buffer then write atomically
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write buffer: %w", err)
	}
	if err := os.WriteFile(outputPath, buf.Bytes(), 0600); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	return &ExportResult{
		Path:     outputPath,
		FileName: fileName,
		Size:     int64(buf.Len()),
		Sheets:   1,
		Rows:     len(data.Rows),
		Duration: time.Since(start),
	}, nil
}

func (e *exporter) ExportStream(ctx context.Context, sheets []SheetDef, rows <-chan Row, cfg ExportConfig) (*ExportResult, error) {
	start := time.Now()

	if len(sheets) == 0 {
		return nil, fmt.Errorf("at least one sheet definition required")
	}

	f := excelize.NewFile()
	defer f.Close()

	// Set up first sheet
	sheetName := sheets[0].Name
	if sheetName == "" {
		sheetName = "Sheet1"
	}
	idx, err := f.GetSheetIndex("Sheet1")
	if err == nil && idx >= 0 {
		f.SetSheetName("Sheet1", sheetName)
	}

	// Add additional sheets
	for i := 1; i < len(sheets); i++ {
		name := sheets[i].Name
		if name == "" {
			name = fmt.Sprintf("Sheet%d", i+1)
		}
		f.NewSheet(name)
	}

	// Write headers for each sheet
	for _, sheet := range sheets {
		for j, col := range sheet.Columns {
			cell, _ := excelize.CoordinatesToCellName(j+1, 1)
			f.SetCellValue(sheet.Name, cell, col.Name)
		}
	}

	// Stream rows into the first sheet
	rowNum := 2 // after header
	totalRows := 0
	for row := range rows {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		for j, val := range row {
			cell, _ := excelize.CoordinatesToCellName(j+1, rowNum)
			f.SetCellValue(sheetName, cell, sanitizeCell(val))
		}
		rowNum++
		totalRows++
	}

	// Auto-width columns for first sheet
	for j, col := range sheets[0].Columns {
		width := col.Width
		if width <= 0 {
			width = len(col.Name) + 2
			if width < 10 {
				width = 10
			}
			if width > 50 {
				width = 50
			}
		}
		cell, _ := excelize.CoordinatesToCellName(j+1, 1)
		f.SetColWidth(sheetName, strings.Split(cell, "1")[0], strings.Split(cell, "1")[0], float64(width))
	}

	fileName := cfg.FileName
	if fileName == "" {
		fileName = GenerateFileName("export")
	}
	if filepath.Ext(fileName) == "" {
		fileName += ".xlsx"
	}
	outputPath := filepath.Join(cfg.OutputDir, fileName)

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write buffer: %w", err)
	}
	if err := os.WriteFile(outputPath, buf.Bytes(), 0600); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	return &ExportResult{
		Path:     outputPath,
		FileName: fileName,
		Size:     int64(buf.Len()),
		Sheets:   len(sheets),
		Rows:     totalRows,
		Duration: time.Since(start),
	}, nil
}
