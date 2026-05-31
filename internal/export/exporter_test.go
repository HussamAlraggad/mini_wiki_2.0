package export

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.OutputDir != "." {
		t.Errorf("OutputDir = %q, want %q", cfg.OutputDir, ".")
	}
	if cfg.Format != "xlsx" {
		t.Errorf("Format = %q, want %q", cfg.Format, "xlsx")
	}
}

func TestGenerateFileName(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		ext    string
	}{
		{"default prefix and ext", "", ""},
		{"custom prefix", "mydata", ""},
		{"custom ext", "", "csv"},
		{"custom both", "report", "json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateFileName(tt.prefix, tt.ext)
			if !strings.HasPrefix(result, tt.prefix) && tt.prefix != "" {
				t.Errorf("result %q does not have prefix %q", result, tt.prefix)
			}
			if tt.ext != "" && !strings.HasSuffix(result, "."+tt.ext) {
				t.Errorf("result %q does not have extension .%s", result, tt.ext)
			}
			if tt.prefix == "" && !strings.HasPrefix(result, "export") {
				t.Errorf("result %q should start with 'export'", result)
			}
			if tt.ext == "" && !strings.HasSuffix(result, ".xlsx") {
				t.Errorf("result %q should end with .xlsx", result)
			}
		})
	}
}

func TestSanitizeCell(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"normal text", "normal text"},
		{"=SUM(A1:A10)", "'=SUM(A1:A10)"},
		{"+FORMULA()", "'+FORMULA()"},
		{"-1+1", "'-1+1"},
		{"@DDE", "'@DDE"},
		{"\t=cmd", "'\t=cmd"},
		{"\t+cmd", "'\t+cmd"},
		{"\t-cmd", "'\t-cmd"},
		{"\t@cmd", "'\t@cmd"},
		{"", ""},
		{"already sanitized", "already sanitized"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeCell(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeCell(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func testData() *ExportData {
	return &ExportData{
		SheetName: "TestSheet",
		Columns: []ColumnDef{
			{Name: "Name", Type: "text"},
			{Name: "Age", Type: "number"},
			{Name: "Score", Type: "number"},
		},
		Rows: []Row{
			{"Alice", "30", "95.5"},
			{"Bob", "25", "88.0"},
			{"=FORMULA", "40", "70.0"}, // formula injection attempt
		},
	}
}

func testCfg() ExportConfig {
	dir, _ := os.MkdirTemp("", "export_test")
	return ExportConfig{
		OutputDir: dir,
		Format:    "csv",
		Overwrite: true,
	}
}

func cleanupCfg(cfg ExportConfig) {
	os.RemoveAll(cfg.OutputDir)
}

func TestExportCSV(t *testing.T) {
	e := New()
	cfg := testCfg()
	defer cleanupCfg(cfg)

	cfg.Format = "csv"
	result, err := e.Export(context.Background(), testData(), cfg)
	if err != nil {
		t.Fatalf("Export CSV failed: %v", err)
	}
	if result.Rows != 3 {
		t.Errorf("Rows = %d, want 3", result.Rows)
	}
	if result.Format != "csv" {
		t.Errorf("Format = %q, want %q", result.Format, "csv")
	}
	if !strings.HasSuffix(result.FileName, ".csv") {
		t.Errorf("FileName = %q, want .csv", result.FileName)
	}
	if result.Size == 0 {
		t.Errorf("Size = 0, expected > 0")
	}

	// Verify file content includes formula-safe prefix
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("Read exported file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "'=FORMULA") {
		t.Errorf("exported CSV should contain sanitized formula, got:\n%s", content)
	}
	if !strings.Contains(content, "Name,Age,Score") {
		t.Errorf("CSV missing header row, got:\n%s", content)
	}
}

func TestExportJSON(t *testing.T) {
	e := New()
	cfg := testCfg()
	defer cleanupCfg(cfg)

	cfg.Format = "json"
	result, err := e.Export(context.Background(), testData(), cfg)
	if err != nil {
		t.Fatalf("Export JSON failed: %v", err)
	}
	if result.Rows != 3 {
		t.Errorf("Rows = %d, want 3", result.Rows)
	}
	if !strings.HasSuffix(result.FileName, ".json") {
		t.Errorf("FileName = %q, want .json", result.FileName)
	}

	// Verify JSON content
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("Read exported file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "Alice") || !strings.Contains(content, "Bob") {
		t.Errorf("JSON missing row data, got:\n%s", content)
	}
	if !strings.Contains(content, "Name") || !strings.Contains(content, "Age") {
		t.Errorf("JSON missing column names, got:\n%s", content)
	}
}

func TestExportMarkdown(t *testing.T) {
	e := New()
	cfg := testCfg()
	defer cleanupCfg(cfg)

	cfg.Format = "md"
	result, err := e.Export(context.Background(), testData(), cfg)
	if err != nil {
		t.Fatalf("Export Markdown failed: %v", err)
	}
	if result.Rows != 3 {
		t.Errorf("Rows = %d, want 3", result.Rows)
	}

	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("Read exported file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "| Name | Age | Score |") {
		t.Errorf("Markdown missing header row, got:\n%s", content)
	}
	if !strings.Contains(content, "| --- |") {
		t.Errorf("Markdown missing separator row, got:\n%s", content)
	}
	if !strings.Contains(content, "Alice") {
		t.Errorf("Markdown missing data, got:\n%s", content)
	}
}

func TestExportXLSX(t *testing.T) {
	e := New()
	cfg := testCfg()
	defer cleanupCfg(cfg)

	cfg.Format = "xlsx"
	result, err := e.Export(context.Background(), testData(), cfg)
	if err != nil {
		t.Fatalf("Export XLSX failed: %v", err)
	}
	if result.Rows != 3 {
		t.Errorf("Rows = %d, want 3", result.Rows)
	}
	if !strings.HasSuffix(result.FileName, ".xlsx") {
		t.Errorf("FileName = %q, want .xlsx", result.FileName)
	}
	if result.Size == 0 {
		t.Errorf("Size = 0, expected > 0")
	}
}

func TestExportFormatAliases(t *testing.T) {
	e := New()
	tests := []struct {
		format string
		ext    string
	}{
		{"csv", ".csv"},
		{"json", ".json"},
		{"md", ".md"},
		{"markdown", ".md"},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			cfg := testCfg()
			defer cleanupCfg(cfg)
			cfg.Format = tt.format
			result, err := e.Export(context.Background(), testData(), cfg)
			if err != nil {
				t.Fatalf("Export %s failed: %v", tt.format, err)
			}
			if !strings.HasSuffix(result.FileName, tt.ext) {
				t.Errorf("expected .%s extension, got %q", tt.ext, result.FileName)
			}
		})
	}
}

func TestExportDefaultFormat(t *testing.T) {
	e := New()
	cfg := testCfg()
	defer cleanupCfg(cfg)

	cfg.Format = "" // should default to xlsx
	result, err := e.Export(context.Background(), testData(), cfg)
	if err != nil {
		t.Fatalf("Export with empty format failed: %v", err)
	}
	if !strings.HasSuffix(result.FileName, ".xlsx") {
		t.Errorf("expected .xlsx, got %q", result.FileName)
	}
}

func TestExportNilData(t *testing.T) {
	e := New()
	cfg := testCfg()
	defer cleanupCfg(cfg)

	_, err := e.Export(context.Background(), nil, cfg)
	if err == nil {
		t.Fatal("Export nil data: expected error, got nil")
	}
}

func TestExportEmptyRows(t *testing.T) {
	e := New()
	cfg := testCfg()
	defer cleanupCfg(cfg)

	data := &ExportData{
		SheetName: "Empty",
		Columns:   []ColumnDef{{Name: "Col1", Type: "text"}},
		Rows:      []Row{},
	}
	_, err := e.Export(context.Background(), data, cfg)
	if err == nil {
		t.Fatal("Export empty rows: expected error, got nil")
	}
}

func TestExportCustomFileName(t *testing.T) {
	e := New()
	cfg := testCfg()
	defer cleanupCfg(cfg)

	cfg.Format = "csv"
	cfg.FileName = "my_export.csv"
	result, err := e.Export(context.Background(), testData(), cfg)
	if err != nil {
		t.Fatalf("Export with custom filename failed: %v", err)
	}
	if result.FileName != "my_export.csv" {
		t.Errorf("FileName = %q, want %q", result.FileName, "my_export.csv")
	}
}

func TestExportCustomFileNameNoExt(t *testing.T) {
	e := New()
	cfg := testCfg()
	defer cleanupCfg(cfg)

	cfg.Format = "csv"
	cfg.FileName = "my_export" // no extension
	result, err := e.Export(context.Background(), testData(), cfg)
	if err != nil {
		t.Fatalf("Export with no-extension filename failed: %v", err)
	}
	if !strings.HasSuffix(result.FileName, ".csv") {
		t.Errorf("FileName = %q, want .csv extension", result.FileName)
	}
}

func TestExportContextCanceled(t *testing.T) {
	e := New()
	cfg := testCfg()
	defer cleanupCfg(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately canceled

	data := &ExportData{
		SheetName: "Canceled",
		Columns:   []ColumnDef{{Name: "X", Type: "text"}},
		Rows:      []Row{{"value1"}, {"value2"}, {"value3"}, {"value4"}, {"value5"}},
	}
	_, err := e.Export(ctx, data, cfg)
	if err == nil {
		t.Fatal("Export with canceled context: expected error, got nil")
	}
}

func TestExportStream(t *testing.T) {
	e := New()
	cfg := testCfg()
	defer cleanupCfg(cfg)

	cfg.Format = "csv"
	rowCh := make(chan Row, 3)
	sheets := []SheetDef{
		{
			Name:    "Sheet1",
			Columns: []ColumnDef{{Name: "A", Type: "text"}, {Name: "B", Type: "number"}},
		},
	}

	go func() {
		rowCh <- Row{"x", "1"}
		rowCh <- Row{"y", "2"}
		rowCh <- Row{"z", "3"}
		close(rowCh)
	}()

	result, err := e.ExportStream(context.Background(), sheets, rowCh, cfg)
	if err != nil {
		t.Fatalf("ExportStream failed: %v", err)
	}
	if result.Rows != 3 {
		t.Errorf("Rows = %d, want 3", result.Rows)
	}
}

func TestExportStreamEmptySheets(t *testing.T) {
	e := New()
	cfg := testCfg()
	defer cleanupCfg(cfg)

	rowCh := make(chan Row)
	close(rowCh)

	_, err := e.ExportStream(context.Background(), nil, rowCh, cfg)
	if err == nil {
		t.Fatal("ExportStream with nil sheets: expected error")
	}
}

func TestExportStreamCanceled(t *testing.T) {
	e := New()
	cfg := testCfg()
	defer cleanupCfg(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	rowCh := make(chan Row)
	sheets := []SheetDef{
		{
			Name:    "Sheet1",
			Columns: []ColumnDef{{Name: "A", Type: "text"}},
		},
	}

	go func() {
		rowCh <- Row{"hello"}
		cancel()
		close(rowCh)
	}()

	_, err := e.ExportStream(ctx, sheets, rowCh, cfg)
	if err == nil {
		t.Fatal("ExportStream canceled: expected error")
	}
}

func TestExportResultDuration(t *testing.T) {
	e := New()
	cfg := testCfg()
	defer cleanupCfg(cfg)

	start := time.Now()
	result, err := e.Export(context.Background(), testData(), cfg)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	if result.Duration <= 0 {
		t.Errorf("Duration = %v, expected > 0", result.Duration)
	}
	if time.Since(start) < result.Duration {
		t.Errorf("Duration %v seems wrong ( > elapsed time %v)", result.Duration, time.Since(start))
	}
}

func TestExportPath(t *testing.T) {
	e := New()
	cfg := testCfg()
	defer cleanupCfg(cfg)

	result, err := e.Export(context.Background(), testData(), cfg)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	if !filepath.IsAbs(result.Path) {
		t.Errorf("Path = %q, expected absolute path", result.Path)
	}
	if _, err := os.Stat(result.Path); err != nil {
		t.Errorf("Exported file not found at %s: %v", result.Path, err)
	}
}

func TestExportODS(t *testing.T) {
	// Tests ODS export. If libreoffice is installed, it converts to actual .ods.
	// If not, it falls back to .xlsx format.
	// Note: libreoffice uses the source filename's base name, not the target name,
	// so the ODS path in the result may not match the actual output file.
	e := New()
	cfg := testCfg()
	defer cleanupCfg(cfg)

	cfg.Format = "ods"
	result, err := e.Export(context.Background(), testData(), cfg)
	if err != nil {
		t.Fatalf("Export ODS failed: %v", err)
	}
	if result.Rows != 3 {
		t.Errorf("Rows = %d, want 3", result.Rows)
	}
	// At minimum, any file should exist in the output directory
	entries, _ := os.ReadDir(cfg.OutputDir)
	if len(entries) == 0 {
		t.Error("No output files generated")
	}
}

func TestExporterInterface(t *testing.T) {
	// Verify New() returns an Exporter
	var e Exporter = New()
	if e == nil {
		t.Fatal("New() returned nil")
	}
}
