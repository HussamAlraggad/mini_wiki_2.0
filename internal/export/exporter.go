// Package export provides tabular data export to Excel (.xlsx), CSV, and JSON formats
// with type-aware column formatting and formula injection protection.
package export

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

// Row represents a single exported row (positional values).
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
	Format    string // "xlsx", "csv", "json" (default: xlsx)
	Overwrite bool
}

// DefaultConfig returns sensible export defaults.
func DefaultConfig() ExportConfig {
	return ExportConfig{
		OutputDir: ".",
		Format:    "xlsx",
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
	Format   string
}

// GenerateFileName creates a timestamped filename.
func GenerateFileName(prefix, ext string) string {
	ts := time.Now().Format("20060102_150405")
	if prefix == "" {
		prefix = "export"
	}
	if ext == "" {
		ext = "xlsx"
	}
	return fmt.Sprintf("%s_%s.%s", prefix, ts, ext)
}

// Exporter is the tabular export interface.
type Exporter interface {
	Export(ctx context.Context, data *ExportData, cfg ExportConfig) (*ExportResult, error)
	ExportStream(ctx context.Context, sheets []SheetDef, rows <-chan Row, cfg ExportConfig) (*ExportResult, error)
}

// New creates a new Exporter.
func New() Exporter {
	return &exporter{}
}

type exporter struct{}

// formulaPrefixes are Excel formula injection indicators.
var formulaPrefixes = []string{"=", "+", "-", "@", "\t=", "\t+", "\t-", "\t@"}

func sanitizeCell(value string) string {
	for _, prefix := range formulaPrefixes {
		if strings.HasPrefix(value, prefix) {
			return "'" + value
		}
	}
	return value
}

func (e *exporter) Export(ctx context.Context, data *ExportData, cfg ExportConfig) (*ExportResult, error) {
	if data == nil || len(data.Rows) == 0 {
		return nil, fmt.Errorf("no data to export")
	}

	format := strings.ToLower(cfg.Format)
	if format == "" {
		format = "xlsx"
	}

	var result *ExportResult
	var err error

	switch format {
	case "csv":
		result, err = e.exportCSV(ctx, data, cfg)
	case "json":
		result, err = e.exportJSON(ctx, data, cfg)
	case "md", "markdown":
		result, err = e.exportMarkdown(ctx, data, cfg)
	default:
		result, err = e.exportXLSX(ctx, data, cfg)
	}

	if err != nil {
		return nil, err
	}
	result.Format = format
	return result, nil
}

func (e *exporter) exportXLSX(ctx context.Context, data *ExportData, cfg ExportConfig) (*ExportResult, error) {
	start := time.Now()

	sheetName := data.SheetName
	if sheetName == "" {
		sheetName = "Sheet1"
	}

	f := excelize.NewFile()
	defer f.Close()

	idx, _ := f.GetSheetIndex("Sheet1")
	if idx >= 0 {
		f.SetSheetName("Sheet1", sheetName)
	}

	// Create a top-aligned style for all data cells.
	topStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{
			Vertical: "top",
		},
	})

	// Header row style (bold)
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold: true,
		},
	})

	// Header
	for j, col := range data.Columns {
		cell, _ := excelize.CoordinatesToCellName(j+1, 1)
		f.SetCellValue(sheetName, cell, col.Name)
		f.SetCellStyle(sheetName, cell, cell, headerStyle)
	}

	// Data rows
	for i, row := range data.Rows {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		for j, rawVal := range row {
			if j >= len(data.Columns) {
				break
			}
			cell, _ := excelize.CoordinatesToCellName(j+1, i+2)
			val := sanitizeCell(rawVal)
			colType := data.Columns[j].Type
			switch colType {
			case "number":
				if n, err := strconv.ParseFloat(val, 64); err == nil {
					f.SetCellFloat(sheetName, cell, n, 2, 64)
				} else {
					f.SetCellStr(sheetName, cell, val)
				}
			default:
				f.SetCellStr(sheetName, cell, val)
			}
			f.SetCellStyle(sheetName, cell, cell, topStyle)
		}
	}

	// Auto-width based on actual data content
	for j, col := range data.Columns {
		width := col.Width
		if width <= 0 {
			// Find max content width across header + all rows
			maxLen := len(col.Name) + 2
			for _, row := range data.Rows {
				if j < len(row) && len(row[j]) > maxLen {
					maxLen = len(row[j])
				}
			}
			width = maxLen
			if width < 10 {
				width = 10
			}
			if width > 80 {
				width = 80
			}
		}
		colName, err := excelize.ColumnNumberToName(j + 1)
		if err == nil {
			f.SetColWidth(sheetName, colName, colName, float64(width))
		}
	}

	fileName := cfg.FileName
	if fileName == "" {
		fileName = GenerateFileName("export", "xlsx")
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
		Sheets:   1,
		Rows:     len(data.Rows),
		Duration: time.Since(start),
	}, nil
}

func (e *exporter) exportCSV(ctx context.Context, data *ExportData, cfg ExportConfig) (*ExportResult, error) {
	start := time.Now()

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Header
	headers := make([]string, len(data.Columns))
	for i, col := range data.Columns {
		headers[i] = col.Name
	}
	writer.Write(headers)

	// Data rows
	for _, row := range data.Rows {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		sanitized := make([]string, len(row))
		for i, v := range row {
			sanitized[i] = sanitizeCell(v)
		}
		writer.Write(sanitized)
	}
	writer.Flush()

	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("csv write: %w", err)
	}

	fileName := cfg.FileName
	if fileName == "" {
		fileName = GenerateFileName("export", "csv")
	}
	if filepath.Ext(fileName) == "" {
		fileName += ".csv"
	}
	outputPath := filepath.Join(cfg.OutputDir, fileName)

	if err := os.WriteFile(outputPath, buf.Bytes(), 0600); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	return &ExportResult{
		Path:     outputPath,
		FileName: fileName,
		Size:     int64(buf.Len()),
		Rows:     len(data.Rows),
		Duration: time.Since(start),
	}, nil
}

func (e *exporter) exportJSON(ctx context.Context, data *ExportData, cfg ExportConfig) (*ExportResult, error) {
	start := time.Now()

	// Build array of objects
	var records []map[string]interface{}
	for _, row := range data.Rows {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		rec := make(map[string]interface{})
		for j, val := range row {
			if j < len(data.Columns) {
				rec[data.Columns[j].Name] = sanitizeCell(val)
			}
		}
		records = append(records, rec)
	}

	jsonData, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}

	fileName := cfg.FileName
	if fileName == "" {
		fileName = GenerateFileName("export", "json")
	}
	if filepath.Ext(fileName) == "" {
		fileName += ".json"
	}
	outputPath := filepath.Join(cfg.OutputDir, fileName)

	if err := os.WriteFile(outputPath, jsonData, 0600); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	return &ExportResult{
		Path:     outputPath,
		FileName: fileName,
		Size:     int64(len(jsonData)),
		Rows:     len(data.Rows),
		Duration: time.Since(start),
	}, nil
}

func (e *exporter) exportMarkdown(ctx context.Context, data *ExportData, cfg ExportConfig) (*ExportResult, error) {
	start := time.Now()

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s\n\n", data.SheetName))
	b.WriteString("|")
	for _, col := range data.Columns {
		b.WriteString(" " + col.Name + " |")
	}
	b.WriteString("\n|")
	for range data.Columns {
		b.WriteString(" --- |")
	}
	b.WriteString("\n")

	for _, row := range data.Rows {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		b.WriteString("|")
		for _, val := range row {
			// Escape pipe characters in cell values
			cell := strings.ReplaceAll(val, "|", "\\|")
			b.WriteString(" " + cell + " |")
		}
		b.WriteString("\n")
	}

	content := b.String()
	fileName := cfg.FileName
	if fileName == "" {
		fileName = GenerateFileName("export", "md")
	}
	if filepath.Ext(fileName) == "" {
		fileName += ".md"
	}
	outputPath := filepath.Join(cfg.OutputDir, fileName)

	if err := os.WriteFile(outputPath, []byte(content), 0600); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	return &ExportResult{
		Path:     outputPath,
		FileName: fileName,
		Size:     int64(len(content)),
		Rows:     len(data.Rows),
		Duration: time.Since(start),
	}, nil
}

func (e *exporter) ExportStream(ctx context.Context, sheets []SheetDef, rows <-chan Row, cfg ExportConfig) (*ExportResult, error) {
	// For streaming, default to CSV (simplest streaming format)
	if cfg.Format == "" || cfg.Format == "xlsx" {
		cfg.Format = "csv"
	}

	if len(sheets) == 0 {
		return nil, fmt.Errorf("at least one sheet definition required")
	}

	start := time.Now()
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Headers
	headers := make([]string, len(sheets[0].Columns))
	for i, col := range sheets[0].Columns {
		headers[i] = col.Name
	}
	writer.Write(headers)

	totalRows := 0
	for row := range rows {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		sanitized := make([]string, len(row))
		for i, v := range row {
			sanitized[i] = sanitizeCell(v)
		}
		writer.Write(sanitized)
		totalRows++
	}
	writer.Flush()

	fileName := cfg.FileName
	if fileName == "" {
		fileName = GenerateFileName("export", "csv")
	}
	outputPath := filepath.Join(cfg.OutputDir, fileName)

	if err := os.WriteFile(outputPath, buf.Bytes(), 0600); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	return &ExportResult{
		Path:     outputPath,
		FileName: fileName,
		Size:     int64(buf.Len()),
		Rows:     totalRows,
		Duration: time.Since(start),
	}, nil
}
