package filescanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectFileType(t *testing.T) {
	s := New()

	tests := []struct {
		name     string
		content  string
		ext      string
		wantType FileType
		wantBin  bool
	}{
		{"csv", "a,b,c\n1,2,3\n", ".csv", FileTypeCSV, false},
		{"json", `{"a":1}`, ".json", FileTypeJSON, false},
		{"go", "package main\nfunc main() {}", ".go", FileTypeGo, false},
		{"markdown", "# Title\nBody", ".md", FileTypeMarkdown, false},
		{"yaml", "key: value\n", ".yaml", FileTypeYAML, false},
		{"text", "hello world", ".txt", FileTypeText, false},
		{"shell", "#!/bin/bash\necho hi", ".sh", FileTypeShell, false},
		{"python", "print('hello')", ".py", FileTypePython, false},
		{"unknown_ext", "some content", ".xyz", FileTypeText, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "test"+tt.ext)
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}

			ft, isBin := s.DetectFileType(path)
			if ft != tt.wantType {
				t.Errorf("DetectFileType() type = %v, want %v", ft, tt.wantType)
			}
			if isBin != tt.wantBin {
				t.Errorf("DetectFileType() binary = %v, want %v", isBin, tt.wantBin)
			}
		})
	}
}

func TestIsSafeTextFile(t *testing.T) {
	s := New()
	dir := t.TempDir()

	// Text file should be safe
	textPath := filepath.Join(dir, "test.txt")
	os.WriteFile(textPath, []byte("hello world"), 0644)
	safe, err := s.IsSafeTextFile(textPath)
	if err != nil {
		t.Fatal(err)
	}
	if !safe {
		t.Error("text file should be safe")
	}

	// Binary file should not be safe
	// Create a minimal PNG-like header
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52}
	pngPath := filepath.Join(dir, "test.png")
	os.WriteFile(pngPath, pngHeader, 0644)
	safe, err = s.IsSafeTextFile(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	if safe {
		t.Error("PNG file should not be safe")
	}
}

func TestScan_Basic(t *testing.T) {
	dir := t.TempDir()

	// Create test files
	files := map[string]string{
		"data.csv":     "a,b\n1,2\n3,4\n",
		"notes.md":     "# Notes\nSome text",
		"config.json":  `{"key": "value"}`,
		"script.py":    "print('hello')",
		".hidden.txt":  "should be skipped",
		"nested/inner.txt": "inner content",
	}

	for path, content := range files {
		fullPath := filepath.Join(dir, path)
		os.MkdirAll(filepath.Dir(fullPath), 0755)
		os.WriteFile(fullPath, []byte(content), 0644)
	}

	// Also create a .git dir (should be skipped)
	gitPath := filepath.Join(dir, ".git", "config")
	os.MkdirAll(filepath.Dir(gitPath), 0755)
	os.WriteFile(gitPath, []byte("[core]\n"), 0644)

	s := New()
	cfg := DefaultConfig()
	cfg.RootDir = dir

	result, err := s.Scan(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Should find 5 files (hidden and .git excluded)
	if len(result.Files) != 5 {
		t.Errorf("expected 5 files, got %d", len(result.Files))
	}

	// Should have some skipped entries (hidden files)
	if len(result.Skipped) == 0 {
		t.Error("expected some skipped entries")
	}
}

func TestScan_ContextCancellation(t *testing.T) {
	dir := t.TempDir()

	// Create some files
	for i := 0; i < 10; i++ {
		path := filepath.Join(dir, fmt.Sprintf("file%d.txt", i))
		os.WriteFile(path, []byte("content"), 0644)
	}

	s := New()
	cfg := DefaultConfig()
	cfg.RootDir = dir

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancel

	_, err := s.Scan(ctx, cfg)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestScan_SizeLimit(t *testing.T) {
	dir := t.TempDir()

	// Create a file larger than max size
	largeData := make([]byte, 5*1024) // 5KB
	largePath := filepath.Join(dir, "large.txt")
	os.WriteFile(largePath, largeData, 0644)

	s := New()
	cfg := ScannerConfig{
		RootDir: dir,
		MaxSize: 1024, // 1KB max
	}

	result, err := s.Scan(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Files) != 0 {
		t.Errorf("expected 0 files (all over limit), got %d", len(result.Files))
	}

	if len(result.Skipped) == 0 {
		t.Error("expected skip for large file")
	}
}

func TestDetectFileType_Binary(t *testing.T) {
	s := New()
	dir := t.TempDir()

	// ELF binary header
	elfHeader := []byte{0x7F, 0x45, 0x4C, 0x46}
	path := filepath.Join(dir, "test.bin")
	os.WriteFile(path, elfHeader, 0644)

	ft, isBin := s.DetectFileType(path)
	if !isBin {
		t.Error("ELF file should be detected as binary")
	}
	if ft != FileTypeBinary {
		t.Errorf("type should be FileTypeBinary, got %v", ft)
	}
}

func TestScan_IncludeHidden(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=key"), 0644)
	os.WriteFile(filepath.Join(dir, "visible.txt"), []byte("hello"), 0644)

	s := New()

	// Without hidden
	cfg := ScannerConfig{RootDir: dir, MaxSize: 10 * 1024 * 1024}
	result, _ := s.Scan(context.Background(), cfg)
	if len(result.Files) != 1 {
		t.Errorf("expected 1 visible file, got %d", len(result.Files))
	}

	// With hidden
	cfg.IncludeHidden = true
	result, _ = s.Scan(context.Background(), cfg)
	if len(result.Files) != 2 {
		t.Errorf("expected 2 files (incl hidden), got %d", len(result.Files))
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxSize != 10*1024*1024 {
		t.Errorf("expected 10MB max size, got %d", cfg.MaxSize)
	}
	if cfg.IncludeHidden {
		t.Error("expected IncludeHidden to be false by default")
	}
}
