package fileref

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestExtraClamp(t *testing.T) {
	tests := []struct {
		val, min, max, want int
	}{
		{5, 0, 10, 5},
		{-1, 0, 10, 0},
		{15, 0, 10, 10},
		{0, 0, 0, 0},
	}
	for _, tt := range tests {
		got := clamp(tt.val, tt.min, tt.max)
		if got != tt.want {
			t.Errorf("clamp(%d,%d,%d) = %d, want %d", tt.val, tt.min, tt.max, got, tt.want)
		}
	}
}

func TestExtraResolveNotFound(t *testing.T) {
	r := New()
	cfg := DefaultConfig()
	cfg.RootDir = t.TempDir()

	ref := Reference{Raw: "@missing.txt"}
	_, _, err := r.Resolve(context.Background(), ref, cfg)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestExtraResolveBinary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.bin")
	os.WriteFile(path, []byte{0x00, 0x01, 0x02}, 0644)

	r := New()
	cfg := DefaultConfig()
	cfg.RootDir = dir

	ref := Reference{Raw: "@data.bin"}
	_, _, err := r.Resolve(context.Background(), ref, cfg)
	if err == nil {
		t.Error("expected error for binary file")
	}
}

func TestExtraInjectNoRefs(t *testing.T) {
	r := New()
	result := &ResolveResult{
		Contents:  map[string][]byte{},
		TotalSize: 0,
	}
	content := "no refs here"
	modified := r.Inject(content, result)
	if modified != content {
		t.Errorf("expected unchanged content, got %q", modified)
	}
}
