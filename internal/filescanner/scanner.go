// Package filescanner provides safe recursive directory scanning with file type
// detection, security checks (path traversal, symlink, binary detection), and
// configurable include/exclude patterns.
package filescanner

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileType categorizes a file based on its extension and content.
type FileType int

const (
	FileTypeUnknown   FileType = iota
	FileTypeText               // .txt
	FileTypeMarkdown           // .md, .markdown
	FileTypeCSV                // .csv
	FileTypeJSON               // .json
	FileTypeYAML               // .yaml, .yml
	FileTypeXML                // .xml
	FileTypeHTML               // .html, .htm
	FileTypeGo                 // .go
	FileTypePython             // .py
	FileTypeJavaScript         // .js
	FileTypeTypeScript         // .ts, .tsx
	FileTypeShell              // .sh, .bash
	FileTypeSQL                // .sql
	FileTypeConfig             // .env, .ini, .cfg, .toml
	FileTypeMakefile           // Makefile
	FileTypeDockerfile         // Dockerfile
	FileTypeBinary             // binary/magic-byte file (rejected from LLM context)
)

// FileInfo describes a single scanned file.
type FileInfo struct {
	Path      string    // relative path from scan root
	AbsPath   string    // absolute path on disk
	Size      int64
	FileType  FileType
	Extension string // ".go", ".md", etc.
	IsBinary  bool
	Modified  time.Time
}

// SkipReason records why a file/directory was excluded.
type SkipReason struct {
	Path   string
	Reason string // "dotfile", "hidden_dir", "binary", "symlink_outside_cwd", "too_large"
}

// ScanResult is returned after a completed scan.
type ScanResult struct {
	Root    string
	Files   []FileInfo
	Dirs    int
	Total   int64 // total bytes of all scanned files
	Skipped []SkipReason
	Elapsed time.Duration
}

// ScannerConfig controls scan behaviour.
type ScannerConfig struct {
	// RootDir is the directory to scan (defaults to CWD).
	RootDir string

	// MaxSize skips files larger than this (default 10MB).
	MaxSize int64

	// MaxDepth limits directory recursion depth (0 = unlimited).
	MaxDepth int

	// IncludeHidden, if true, includes dotfiles/dotdirs (default false).
	IncludeHidden bool
}

// DefaultConfig returns a ScannerConfig with safe defaults.
func DefaultConfig() ScannerConfig {
	return ScannerConfig{
		MaxSize:  10 * 1024 * 1024, // 10MB
		MaxDepth: 0,                // unlimited
	}
}

// Scanner is the file system scanner interface.
type Scanner interface {
	// Scan performs a full recursive scan with context cancellation.
	Scan(ctx context.Context, cfg ScannerConfig) (*ScanResult, error)

	// DetectFileType determines the type of a file by extension + magic bytes.
	DetectFileType(path string) (FileType, bool)

	// IsSafeTextFile checks that a file is safe to read for LLM context.
	IsSafeTextFile(path string) (bool, error)
}

// New creates a new Scanner with default settings.
func New() Scanner {
	return &scanner{}
}

type scanner struct{}

// --- Allowed MIME types for LLM context ---

var allowedTextMIMEs = map[string]bool{
	"text/plain":            true,
	"text/csv":              true,
	"text/tab-separated-values": true,
	"text/html":             true,
	"text/xml":              true,
	"text/markdown":         true,
	"application/json":      true,
	"application/xml":       true,
	"application/x-yaml":    true,
}

// extensionTypeMap maps file extensions to FileType.
var extensionTypeMap = map[string]FileType{
	".txt":       FileTypeText,
	".md":        FileTypeMarkdown,
	".markdown":  FileTypeMarkdown,
	".csv":       FileTypeCSV,
	".tsv":       FileTypeCSV,
	".json":      FileTypeJSON,
	".jsonl":     FileTypeJSON,
	".ndjson":    FileTypeJSON,
	".yaml":      FileTypeYAML,
	".yml":       FileTypeYAML,
	".xml":       FileTypeXML,
	".html":      FileTypeHTML,
	".htm":       FileTypeHTML,
	".go":        FileTypeGo,
	".py":        FileTypePython,
	".js":        FileTypeJavaScript,
	".ts":        FileTypeTypeScript,
	".tsx":       FileTypeTypeScript,
	".sh":        FileTypeShell,
	".bash":      FileTypeShell,
	".sql":       FileTypeSQL,
	".env":       FileTypeConfig,
	".ini":       FileTypeConfig,
	".cfg":       FileTypeConfig,
	".toml":      FileTypeConfig,
}

// skipDirs lists directories whose entire tree is skipped by default.
var skipDirs = map[string]bool{
	".git":         true,
	".svn":         true,
	".hg":          true,
	"node_modules": true,
	"__pycache__":  true,
	".venv":        true,
	"venv":         true,
	"vendor":       true,
	".next":        true,
	"dist":         true,
	"build":        true,
	".cache":       true,
}

// scanMaxFiles limits the total number of files scanned to prevent DoS.
const scanMaxFiles = 10000

// skipMaxDepth is the maximum directory depth.
const skipMaxDepth = 50

func (s *scanner) Scan(ctx context.Context, cfg ScannerConfig) (*ScanResult, error) {
	if cfg.RootDir == "" {
		var err error
		cfg.RootDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getwd: %w", err)
		}
	}
	if cfg.MaxSize == 0 {
		cfg.MaxSize = 10 * 1024 * 1024
	}

	start := time.Now()

	absRoot, err := filepath.Abs(cfg.RootDir)
	if err != nil {
		return nil, fmt.Errorf("abs root: %w", err)
	}
	absRoot, err = filepath.EvalSymlinks(absRoot)
	if err != nil {
		return nil, fmt.Errorf("eval symlinks root: %w", err)
	}

	var files []FileInfo
	var skipped []SkipReason
	dirCount := 0
	var totalBytes int64
	fileCount := 0

	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Permission denied — skip the entry
			return filepath.SkipDir
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Resolve symlinks to check if they point outside CWD
		if d.Type()&os.ModeSymlink != 0 {
			target, err := filepath.EvalSymlinks(path)
			if err != nil {
				skipped = append(skipped, SkipReason{Path: path, Reason: "unresolvable_symlink"})
				return nil
			}
			if !strings.HasPrefix(target, absRoot+string(filepath.Separator)) && target != absRoot {
				skipped = append(skipped, SkipReason{Path: path, Reason: "symlink_outside_cwd"})
				return nil
			}
		}

		if d.IsDir() {
			name := d.Name()
			// Skip hidden dirs (unless included)
			if !cfg.IncludeHidden && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			// Skip known noisy dirs
			if skipDirs[name] {
				return filepath.SkipDir
			}
			// Check max depth
			rel, _ := filepath.Rel(absRoot, path)
			if rel != "." {
				depth := strings.Count(rel, string(filepath.Separator))
				if cfg.MaxDepth > 0 && depth >= cfg.MaxDepth {
					return filepath.SkipDir
				}
				if depth >= skipMaxDepth {
					return filepath.SkipDir
				}
			}
			dirCount++
			return nil
		}

		// --- File processing ---
		// Enforce max file count
		if fileCount >= scanMaxFiles {
			skipped = append(skipped, SkipReason{Path: path, Reason: "max_file_limit"})
			return filepath.SkipDir
		}

		// Skip hidden files
		if !cfg.IncludeHidden && strings.HasPrefix(d.Name(), ".") {
			skipped = append(skipped, SkipReason{Path: path, Reason: "dotfile"})
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		// Check size limit
		if info.Size() > cfg.MaxSize {
			skipped = append(skipped, SkipReason{Path: path, Reason: "too_large"})
			return nil
		}

		// Detect file type
		ext := strings.ToLower(filepath.Ext(path))
		ft, isBinary := s.DetectFileType(path)

		// Makefile and Dockerfile (no extension)
		if ext == "" {
			base := filepath.Base(path)
			if base == "Makefile" {
				ft = FileTypeMakefile
				isBinary = false
			} else if base == "Dockerfile" {
				ft = FileTypeDockerfile
				isBinary = false
			}
		}

		fileInfo := FileInfo{
			Path:      path,
			AbsPath:   path,
			Size:      info.Size(),
			FileType:  ft,
			Extension: ext,
			IsBinary:  isBinary,
			Modified:  info.ModTime(),
		}
		files = append(files, fileInfo)
		fileCount++
		totalBytes += info.Size()

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk error: %w", err)
	}

	return &ScanResult{
		Root:    absRoot,
		Files:   files,
		Dirs:    dirCount,
		Total:   totalBytes,
		Skipped: skipped,
		Elapsed: time.Since(start),
	}, nil
}

// DetectFileType determines the type by extension and validates with magic bytes.
func (s *scanner) DetectFileType(path string) (FileType, bool) {
	ext := strings.ToLower(filepath.Ext(path))

	// Check by extension first
	ft, ok := extensionTypeMap[ext]
	if !ok {
		ft = FileTypeText
	}

	// Validate with magic bytes
	ok, isBinary := checkMagicBytes(path)
	if !ok {
		return FileTypeBinary, true
	}

	return ft, isBinary
}

// IsSafeTextFile checks that a file contains only safe text content.
func (s *scanner) IsSafeTextFile(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return false, err
	}
	buf = buf[:n]

	// Check for binary indicators (null bytes, known binary magic)
	if isBinaryData(buf) {
		return false, nil
	}

	// Fallback: use Go's mime detection
	mime := http.DetectContentType(buf)
	if strings.HasPrefix(mime, "text/") {
		return true, nil
	}
	if allowedTextMIMEs[mime] {
		return true, nil
	}
	return false, nil
}

// Known binary magic byte prefixes (longest first for prefix matching).
var binaryMagicPrefixes = [][]byte{
	{0x7F, 0x45, 0x4C, 0x46},             // ELF
	{0x25, 0x50, 0x44, 0x46},             // PDF
	{0x50, 0x4B, 0x03, 0x04},             // ZIP (incl. .docx, .xlsx, .jar)
	{0x50, 0x4B, 0x05, 0x06},             // ZIP (empty archive)
	{0x50, 0x4B, 0x07, 0x08},             // ZIP (spanned)
	{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, // PNG
	{0xFF, 0xD8, 0xFF},                   // JPEG
	{0x47, 0x49, 0x46, 0x38},             // GIF
	{0x42, 0x4D},                         // BMP
	{0x25, 0x21},                         // PostScript
	{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, // OLE2 (doc, xls)
	{0x4D, 0x5A},                         // PE (Windows exe, dll)
	{0xFE, 0xED, 0xFA, 0xCE},            // Mach-O (32-bit)
	{0xFE, 0xED, 0xFA, 0xCF},            // Mach-O (64-bit)
	{0xCE, 0xFA, 0xED, 0xFE},            // Mach-O (reverse 32)
	{0xCF, 0xFA, 0xED, 0xFE},            // Mach-O (reverse 64)
	{0x1F, 0x8B},                         // GZIP
	{0x42, 0x5A, 0x68},                   // BZIP2
	{0xFD, 0x37, 0x7A, 0x58, 0x5A, 0x00}, // XZ
	{0x52, 0x61, 0x72, 0x21, 0x1A, 0x07}, // RAR
	{0x00, 0x00, 0x01, 0x00},             // ICO
}

// isBinaryData checks if the given bytes indicate binary content.
func isBinaryData(buf []byte) bool {
	if len(buf) == 0 {
		return false
	}

	// Check for null bytes (strong binary indicator in first 512 bytes)
	for _, b := range buf {
		if b == 0 {
			return true
		}
	}

	// Check for known binary magic prefixes
	for _, magic := range binaryMagicPrefixes {
		if len(buf) >= len(magic) {
			match := true
			for i, m := range magic {
				if buf[i] != m {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}

	return false
}

// checkMagicBytes reads the first bytes and checks if the file is binary.
func checkMagicBytes(path string) (isText bool, isBinary bool) {
	f, err := os.Open(path)
	if err != nil {
		return true, false // assume text on open error
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return true, false
	}
	buf = buf[:n]

	// First: check for definitive binary indicators
	if isBinaryData(buf) {
		return false, true
	}

	// Second: use Go's content-type detection as a fallback
	mime := http.DetectContentType(buf)
	if strings.HasPrefix(mime, "text/") {
		return true, false
	}
	if allowedTextMIMEs[mime] {
		return true, false
	}
	return false, true
}
