// Package fileref implements @file reference resolution with security checks.
// It handles parsing @filename, @path/to/file, @file.go:42, and @file.go:10-20
// syntax in user chat input, resolves them safely within the CWD, and injects
// file contents into the LLM context as annotated code blocks.
package fileref

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"mini-wiki/internal/filescanner"
	"mini-wiki/internal/wiki"
)

// Reference represents a single @file mention found in user input.
type Reference struct {
	Raw     string // original text like "@data.csv"
	Resolved string // absolute path after security resolution
	Line    int    // specific line (0 if not specified)
	EndLine int    // end line for ranges
	Start   int    // byte offset in original text
	End     int    // end byte offset
}

// RefError records why a reference could not be resolved.
type RefError struct {
	Raw    string
	Reason string
}

// ResolveResult is the outcome of resolving all @refs in one message.
type ResolveResult struct {
	Refs      []Reference
	Contents  map[string][]byte // resolved path → content
	Errors    []RefError
	TotalSize int64
}

// ResolverConfig controls resolution limits and security policies.
type ResolverConfig struct {
	// MaxFileSize is the per-file limit (default 1MB).
	MaxFileSize int64

	// MaxTotalSize is the cumulative limit across all refs (default 10MB).
	MaxTotalSize int64

	// MaxRefs is the max @ references per message (default 10).
	MaxRefs int

	// RootDir is the safe base directory for resolution.
	RootDir string
}

// DefaultConfig returns the default resolver settings.
func DefaultConfig() ResolverConfig {
	return ResolverConfig{
		MaxFileSize:  1 * 1024 * 1024,  // 1MB
		MaxTotalSize: 10 * 1024 * 1024, // 10MB
		MaxRefs:      10,
	}
}

// Resolver handles @file reference parsing and safe resolution.
type Resolver interface {
	// FindRefs extracts all @file references from text.
	FindRefs(text string) []Reference

	// Resolve converts a Reference to absolute path and reads content securely.
	Resolve(ctx context.Context, ref Reference, cfg ResolverConfig) (string, []byte, error)

	// ResolveAll processes all references in text against the scanned file index.
	ResolveAll(ctx context.Context, text string, index *filescanner.ScanResult, cfg ResolverConfig) (*ResolveResult, error)

	// Inject replaces @ references with annotated file content blocks.
	Inject(text string, result *ResolveResult) string
}

// New creates a new Resolver.
func New() Resolver {
	return &resolver{}
}

type resolver struct{}

// @ref regex: matches @filename, @path/to/file, @file.go:42, @file.go:10-20
// Must not be preceded by word characters (to avoid email-like false matches).
var refRegex = regexp.MustCompile(`(?:^|\s|,|\.\s)@([a-zA-Z0-9_./\\-]+(?:\.[a-zA-Z0-9]+)?(?::(\d+)(?:-(\d+))?)?)`)

// fileSafeExtractor strips the @ prefix and line number suffix for security checks.
func extractRefInfo(raw string) (path string, line, endLine int) {
	// Strip leading @
	path = strings.TrimPrefix(raw, "@")

	// Check for line/range suffix
	if colonIdx := strings.LastIndex(path, ":"); colonIdx > 0 {
		// Check if there's a number after the colon
		rest := path[colonIdx+1:]
		if n, err := fmt.Sscanf(rest, "%d-%d", &line, &endLine); err == nil && n >= 1 {
			path = path[:colonIdx]
		} else if n, err := fmt.Sscanf(rest, "%d", &line); err == nil && n == 1 {
			path = path[:colonIdx]
		}
		if endLine == 0 {
			endLine = line
		}
	}

	return path, line, endLine
}

func (r *resolver) FindRefs(text string) []Reference {
	matches := refRegex.FindAllStringSubmatchIndex(text, -1)
	var refs []Reference

	for _, match := range matches {
		if match[2] < 0 {
			continue
		}

		raw := text[match[2]-1 : match[3]] // include @

		_, line, endLine := extractRefInfo(raw)

		ref := Reference{
			Raw:     raw,
			Line:    line,
			EndLine: endLine,
			Start:   match[2] - 1,
			End:     match[3],
		}

		refs = append(refs, ref)
	}

	return refs
}

// SafeResolve resolves a user-supplied filename relative to rootDir and ensures
// it does not escape via path traversal or symlink attacks.
func SafeResolve(rootDir, refPath string) (string, error) {
	absRoot, err := filepath.EvalSymlinks(rootDir)
	if err != nil {
		return "", wiki.Wrap(wiki.KindFileSystem, "resolve root", err)
	}

	// Reject absolute references unless they're within root
	if filepath.IsAbs(refPath) {
		return "", wiki.New(wiki.KindPermission, fmt.Sprintf("absolute path denied: %s", refPath))
	}

	candidate := filepath.Join(absRoot, refPath)
	absRef, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", wiki.Wrap(wiki.KindNotFound, fmt.Sprintf("file not found: %s", refPath), err)
	}

	// Must still be inside root after symlink resolution
	if !strings.HasPrefix(absRef, absRoot+string(filepath.Separator)) && absRef != absRoot {
		return "", wiki.New(wiki.KindPermission, fmt.Sprintf("path traversal denied: %s escapes %s", refPath, rootDir))
	}

	return absRef, nil
}

func (r *resolver) Resolve(ctx context.Context, ref Reference, cfg ResolverConfig) (string, []byte, error) {
	pathOnly, _, _ := extractRefInfo(ref.Raw)

	rootDir := cfg.RootDir
	if rootDir == "" {
		var err error
		rootDir, err = os.Getwd()
		if err != nil {
			return "", nil, wiki.Wrap(wiki.KindFileSystem, "getwd", err)
		}
	}

	// Security: resolve and validate path
	absPath, err := SafeResolve(rootDir, pathOnly)
	if err != nil {
		return "", nil, err
	}

	// Security: stat the open file (TOCTOU safe)
	f, err := os.Open(absPath)
	if err != nil {
		return "", nil, wiki.Wrap(wiki.KindNotFound, fmt.Sprintf("cannot open: %s", absPath), err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", nil, wiki.Wrap(wiki.KindFileSystem, "stat", err)
	}

	// Size limit
	if info.Size() > cfg.MaxFileSize {
		return "", nil, wiki.New(wiki.KindFileTooLarge, fmt.Sprintf("file too large: %d bytes (max %d)", info.Size(), cfg.MaxFileSize))
	}

	// Binary detection
	safe, err := filescanner.New().IsSafeTextFile(absPath)
	if err != nil {
		return "", nil, wiki.Wrap(wiki.KindFileSystem, "detect file type", err)
	}
	if !safe {
		return "", nil, wiki.New(wiki.KindBinaryFile, fmt.Sprintf("binary file not allowed: %s", absPath))
	}

	// Read content
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", nil, wiki.Wrap(wiki.KindFileSystem, "read file", err)
	}

	// Line-based slicing
	if ref.Line > 0 {
		lines := strings.Split(string(data), "\n")
		start := clamp(ref.Line-1, 0, len(lines))
		end := clamp(ref.EndLine, 0, len(lines))
		if start > end {
			start = end
		}
		data = []byte(strings.Join(lines[start:end], "\n"))
	}

	return absPath, data, nil
}

func (r *resolver) ResolveAll(ctx context.Context, text string, index *filescanner.ScanResult, cfg ResolverConfig) (*ResolveResult, error) {
	refs := r.FindRefs(text)

	if len(refs) > cfg.MaxRefs {
		return nil, wiki.New(wiki.KindValidation, fmt.Sprintf("too many file references: %d (max %d)", len(refs), cfg.MaxRefs))
	}

	result := &ResolveResult{
		Contents: make(map[string][]byte),
	}

	for _, ref := range refs {
		absPath, data, err := r.Resolve(ctx, ref, cfg)
		if err != nil {
			result.Errors = append(result.Errors, RefError{
				Raw:    ref.Raw,
				Reason: err.Error(),
			})
			continue
		}

		result.Refs = append(result.Refs, ref)
		result.Contents[absPath] = data
		result.TotalSize += int64(len(data))

		if result.TotalSize > cfg.MaxTotalSize {
			result.Errors = append(result.Errors, RefError{
				Raw:    ref.Raw,
				Reason: "total size limit exceeded",
			})
			delete(result.Contents, absPath)
			result.TotalSize -= int64(len(data))
			break
		}
	}

	return result, nil
}

func (r *resolver) Inject(text string, result *ResolveResult) string {
	if result == nil || len(result.Contents) == 0 {
		return text
	}

	var sb strings.Builder
	lastEnd := 0

	// Build a map from raw ref to resolved path
	refToResolved := make(map[string]string)
	for _, ref := range result.Refs {
		refToResolved[ref.Raw] = ref.Resolved
	}

	for _, ref := range result.Refs {
		// Append text before this ref
		if ref.Start > lastEnd {
			sb.WriteString(text[lastEnd:ref.Start])
		}

		// Check if resolution succeeded
		resolvedPath, ok := refToResolved[ref.Raw]
		if !ok {
			sb.WriteString(ref.Raw)
			lastEnd = ref.End
			continue
		}

		data, hasContent := result.Contents[resolvedPath]
		if !hasContent {
			sb.WriteString(ref.Raw)
			lastEnd = ref.End
			continue
		}

		// Replace @ref with annotated markdown code block
		ext := filepath.Ext(resolvedPath)
		lang := strings.TrimPrefix(ext, ".")
		if lang == "" {
			lang = "text"
		}

		relPath := resolvedPath
		if cfg := getRootDir(); cfg != "" {
			if rel, err := filepath.Rel(cfg, resolvedPath); err == nil {
				relPath = rel
			}
		}

		sb.WriteString(fmt.Sprintf("\n```%s file=%s\n%s\n```\n", lang, relPath, string(data)))
		lastEnd = ref.End
	}

	// Append remaining text
	if lastEnd < len(text) {
		sb.WriteString(text[lastEnd:])
	}

	return sb.String()
}

// getRootDir returns the configured root dir or empty string.
var getRootDir = func() string { return "" }

func clamp(n, min, max int) int {
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

// StripRefs removes all @file references from text (for display purposes).
func StripRefs(text string) string {
	return refRegex.ReplaceAllString(text, "")
}
