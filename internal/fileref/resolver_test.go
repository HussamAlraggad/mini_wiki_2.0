package fileref

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mini-wiki/internal/filescanner"
)

func TestFindRefs(t *testing.T) {
	r := New()

	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"single ref", "check @data.csv", 1},
		{"multiple refs", "see @a.csv and @b.md", 2},
		{"no ref", "hello world", 0},
		{"ref with path", "look at @subdir/file.txt", 1},
		{"email not a ref", "email me@example.com", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := r.FindRefs(tt.input)
			if len(refs) != tt.want {
				t.Errorf("FindRefs() got %d refs, want %d", len(refs), tt.want)
			}
		})
	}
}

func TestFindRefs_LineNumbers(t *testing.T) {
	r := New()

	refs := r.FindRefs("see @file.go:42")
	if len(refs) != 1 {
		t.Fatal("expected 1 ref")
	}
	if refs[0].Line != 42 || refs[0].EndLine != 42 {
		t.Errorf("expected line 42, got %d-%d", refs[0].Line, refs[0].EndLine)
	}

	refs = r.FindRefs("see @file.go:10-20")
	if len(refs) != 1 {
		t.Fatal("expected 1 ref")
	}
	if refs[0].Line != 10 || refs[0].EndLine != 20 {
		t.Errorf("expected lines 10-20, got %d-%d", refs[0].Line, refs[0].EndLine)
	}
}

func TestSafeResolve(t *testing.T) {
	dir := t.TempDir()

	// Create a test file
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)
	os.MkdirAll(filepath.Join(dir, "subdir"), 0755)
	os.WriteFile(filepath.Join(dir, "subdir", "nested.txt"), []byte("nested"), 0644)

	// Valid resolution
	resolved, err := SafeResolve(dir, "test.txt")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != filepath.Join(dir, "test.txt") {
		t.Errorf("expected %s, got %s", filepath.Join(dir, "test.txt"), resolved)
	}

	// Nested resolution
	resolved, err = SafeResolve(dir, "subdir/nested.txt")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != filepath.Join(dir, "subdir", "nested.txt") {
		t.Errorf("expected nested path")
	}

	// Path traversal
	_, err = SafeResolve(dir, "../etc/passwd")
	if err == nil {
		t.Error("expected error for path traversal")
	}

	// Absolute path
	_, err = SafeResolve(dir, "/etc/passwd")
	if err == nil {
		t.Error("expected error for absolute path")
	}

	// Non-existent file
	_, err = SafeResolve(dir, "nonexistent.txt")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestResolve_Basic(t *testing.T) {
	dir := t.TempDir()
	content := "hello, world"
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0644)

	r := New()
	cfg := DefaultConfig()
	cfg.RootDir = dir

	ref := Reference{Raw: "@test.txt"}
	absPath, data, err := r.Resolve(context.Background(), ref, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Errorf("expected %q, got %q", content, string(data))
	}
	if absPath != filepath.Join(dir, "test.txt") {
		t.Errorf("unexpected path: %s", absPath)
	}
}

func TestResolve_LargeFile(t *testing.T) {
	dir := t.TempDir()
	// Create 100KB of text (not null bytes, so binary detection passes)
	largeData := []byte(strings.Repeat("hello world this is text data\n", 100*1024/32))
	os.WriteFile(filepath.Join(dir, "large.txt"), largeData, 0644)

	r := New()
	cfg := DefaultConfig()
	cfg.RootDir = dir

	ref := Reference{Raw: "@large.txt"}
	_, data, err := r.Resolve(context.Background(), ref, cfg)
	if err != nil {
		t.Fatalf("expected no error for large file, got: %v", err)
	}
	if len(data) < 90000 {
		t.Errorf("expected ~100KB data, got %d bytes", len(data))
	}
}

func TestResolveAll(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("file a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("file b"), 0644)

	r := New()
	cfg := DefaultConfig()
	cfg.RootDir = dir

	// Create a scan result
	scanResult := &filescanner.ScanResult{
		Root: dir,
	}

	result, err := r.ResolveAll(context.Background(), "check @a.txt and @b.txt", scanResult, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Contents) != 2 {
		t.Errorf("expected 2 resolved files, got %d", len(result.Contents))
	}
}

func TestInject(t *testing.T) {
	r := New()

	result := &ResolveResult{
		Contents: map[string][]byte{
			"/path/to/data.csv": []byte("a,b\n1,2"),
		},
		TotalSize: 8,
	}

	result.Refs = append(result.Refs, Reference{
		Raw:      "@data.csv",
		Resolved: "/path/to/data.csv",
	})

	text := "Analyze @data.csv"
	injected := r.Inject(text, result)

	if !strings.Contains(injected, "```") {
		t.Error("expected markdown code block in injected text")
	}
	if !strings.Contains(injected, "a,b") {
		t.Error("expected file content in injected text")
	}
}

func TestStripRefs(t *testing.T) {
	cleaned := StripRefs("see @data.csv and @notes.md")
	if strings.Contains(cleaned, "@data.csv") {
		t.Error("expected @data.csv to be stripped")
	}
	if strings.Contains(cleaned, "@notes.md") {
		t.Error("expected @notes.md to be stripped")
	}
}

func TestExtractRefInfo(t *testing.T) {
	path, line, endLine := extractRefInfo("@file.go:42")
	if path != "file.go" || line != 42 || endLine != 42 {
		t.Errorf("got path=%q line=%d endLine=%d", path, line, endLine)
	}

	path, line, endLine = extractRefInfo("@data.csv")
	if path != "data.csv" || line != 0 || endLine != 0 {
		t.Errorf("got path=%q line=%d endLine=%d", path, line, endLine)
	}

	path, line, endLine = extractRefInfo("@path/to/file.go:10-20")
	if path != "path/to/file.go" || line != 10 || endLine != 20 {
		t.Errorf("got path=%q line=%d endLine=%d", path, line, endLine)
	}
}
