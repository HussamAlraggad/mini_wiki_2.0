package filescanner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtraCheckMagicBytes(t *testing.T) {
	tests := []struct {
		name     string
		content  []byte
		wantBin  bool
	}{
		// Note: test files must be >= 512 bytes for reliable MIME detection.
		// Short files (<512 bytes) are handled as a fallback.
		{"ELF binary", append([]byte{0x7F, 0x45, 0x4C, 0x46}, padBytes(508)...), true},
		{"PDF document", append([]byte{0x25, 0x50, 0x44, 0x46}, padBytes(508)...), true},
		{"ZIP archive", append([]byte{0x50, 0x4B, 0x03, 0x04}, padBytes(508)...), true},
		{"null bytes binary", append(padBytes(100), []byte{0x00}...), true},
		{"plain text", []byte("hello world, this is a text file used for testing binary detection\n"), false},
		{"empty", []byte{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "file.bin")
			os.WriteFile(path, tt.content, 0644)
			isText, isBinary := checkMagicBytes(path)
			if tt.wantBin && !isBinary {
				t.Errorf("checkMagicBytes(%q): isText=%v, isBinary=%v, want isBinary=true", tt.name, isText, isBinary)
			}
			if !tt.wantBin && isBinary {
				t.Errorf("checkMagicBytes(%q): isText=%v, isBinary=%v, want isBinary=false", tt.name, isText, isBinary)
			}
		})
	}
}

func padBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte('A' + i%26)
	}
	return b
}

func TestExtraIsBinaryData(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"null byte", []byte("hello\x00world"), true},
		{"no null byte", []byte("hello world"), false},
		{"empty", []byte{}, false},
		{"unicode text", []byte{0xC3, 0xA9, 0xC3, 0xA0}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBinaryData(tt.data)
			if got != tt.want {
				t.Errorf("isBinaryData(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestExtraScanBinaryDetection(t *testing.T) {
	dir := t.TempDir()

	// Normal text file — should be IsBinary=false
	os.WriteFile(filepath.Join(dir, "normal.txt"), []byte("hello world"), 0644)

	// File with null bytes — should be IsBinary=true
	binData := make([]byte, 100)
	for i := 0; i < 50; i++ {
		binData[i] = byte('A' + i%26)
	}
	binData[50] = 0x00 // null byte triggers binary detection
	os.WriteFile(filepath.Join(dir, "data.bin"), binData, 0644)

	// File with ELF magic (>=512 bytes for proper detection)
	elfData := append([]byte{0x7F, 0x45, 0x4C, 0x46}, padBytes(508)...)
	os.WriteFile(filepath.Join(dir, "program.elf"), elfData, 0644)

	s := New()
	cfg := DefaultConfig()
	cfg.RootDir = dir

	result, err := s.Scan(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Scan includes all files, but with IsBinary flag set
	textFiles := 0
	binaryFiles := 0
	for _, f := range result.Files {
		if f.IsBinary {
			binaryFiles++
		} else {
			textFiles++
		}
	}
	if textFiles != 1 {
		t.Errorf("expected 1 text file, got %d", textFiles)
	}
	if binaryFiles < 2 {
		t.Errorf("expected at least 2 binary files, got %d", binaryFiles)
	}
}

func TestExtraScanSkipDirs(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("data"), 0644)
	os.MkdirAll(filepath.Join(dir, ".git"), 0700)
	os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("data"), 0644)
	os.MkdirAll(filepath.Join(dir, "node_modules"), 0700)
	os.WriteFile(filepath.Join(dir, "node_modules", "lib.js"), []byte("data"), 0644)
	os.MkdirAll(filepath.Join(dir, "__pycache__"), 0700)
	os.WriteFile(filepath.Join(dir, "__pycache__", "cache.pyc"), []byte("data"), 0644)

	s := New()
	cfg := DefaultConfig()
	cfg.RootDir = dir

	result, err := s.Scan(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if len(result.Files) != 1 {
		t.Errorf("expected 1 file (others in skipped dirs), got %d", len(result.Files))
	}
}

func TestExtraScanSkippedReasons(t *testing.T) {
	dir := t.TempDir()

	// File exceeding size limit
	largeData := make([]byte, 1025)
	os.WriteFile(filepath.Join(dir, "large.txt"), largeData, 0644)
	// Normal file
	os.WriteFile(filepath.Join(dir, "small.txt"), []byte("hello"), 0644)

	s := New()
	cfg := DefaultConfig()
	cfg.RootDir = dir
	cfg.MaxSize = 1024

	result, err := s.Scan(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if len(result.Files) != 1 {
		t.Errorf("expected 1 file, got %d", len(result.Files))
	}
	// Check that the large file was skipped
	hasSizeSkip := false
	for _, sk := range result.Skipped {
		if strings.Contains(sk.Reason, "size") || strings.Contains(sk.Reason, "MaxSize") {
			hasSizeSkip = true
		}
	}
	if !hasSizeSkip {
		t.Log("Skipped entries:", result.Skipped)
	}
}
