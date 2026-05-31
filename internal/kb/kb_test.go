package kb

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestOpenClose(t *testing.T) {
	db := New()
	path := tempDBPath(t)
	defer os.Remove(path)

	ctx := context.Background()
	if err := db.Open(ctx, DBConfig{Path: path}); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestCloseIdempotent(t *testing.T) {
	db := New()
	path := tempDBPath(t)
	defer os.Remove(path)

	db.Open(context.Background(), DBConfig{Path: path})
	db.Close()
	// Second close should not panic
	db.Close()
}

func TestRegisterAndGetFile(t *testing.T) {
	db := newOpenDB(t)
	defer db.Close()
	defer cleanupDB(t, db)

	ctx := context.Background()
	rec := FileRecord{
		Path:        "/test/data.csv",
		Hash:        "abc123",
		Size:        1024,
		RowCount:    100,
		Status:      "indexed",
		FirstSeen:   time.Now(),
		LastIndexed: time.Now(),
	}

	if err := db.RegisterFile(ctx, rec); err != nil {
		t.Fatalf("RegisterFile failed: %v", err)
	}

	got, err := db.GetFileRegistry(ctx, "/test/data.csv")
	if err != nil {
		t.Fatalf("GetFileRegistry failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetFileRegistry returned nil")
	}
	if got.Path != rec.Path {
		t.Errorf("Path = %q, want %q", got.Path, rec.Path)
	}
	if got.Hash != rec.Hash {
		t.Errorf("Hash = %q, want %q", got.Hash, rec.Hash)
	}
	if got.Status != rec.Status {
		t.Errorf("Status = %q, want %q", got.Status, rec.Status)
	}
	if got.RowCount != rec.RowCount {
		t.Errorf("RowCount = %d, want %d", got.RowCount, rec.RowCount)
	}
}

func TestRegisterFileUpdate(t *testing.T) {
	db := newOpenDB(t)
	defer db.Close()
	defer cleanupDB(t, db)

	ctx := context.Background()
	rec1 := FileRecord{Path: "/test/file.csv", Hash: "v1", Status: "pending", RowCount: 10}
	rec2 := FileRecord{Path: "/test/file.csv", Hash: "v2", Status: "indexed", RowCount: 20}

	db.RegisterFile(ctx, rec1)
	db.RegisterFile(ctx, rec2)

	got, _ := db.GetFileRegistry(ctx, "/test/file.csv")
	if got.Hash != "v2" {
		t.Errorf("After update, Hash = %q, want %q", got.Hash, "v2")
	}
	if got.RowCount != 20 {
		t.Errorf("After update, RowCount = %d, want %d", got.RowCount, 20)
	}
}

func TestGetFileNotFound(t *testing.T) {
	db := newOpenDB(t)
	defer db.Close()
	defer cleanupDB(t, db)

	got, err := db.GetFileRegistry(context.Background(), "/nonexistent")
	if err != nil {
		t.Fatalf("GetFileRegistry for nonexistent: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent file, got %+v", got)
	}
}

func TestListFiles(t *testing.T) {
	db := newOpenDB(t)
	defer db.Close()
	defer cleanupDB(t, db)

	ctx := context.Background()
	db.RegisterFile(ctx, FileRecord{Path: "/a.csv", Status: "pending"})
	db.RegisterFile(ctx, FileRecord{Path: "/b.csv", Status: "indexed"})

	files, err := db.ListFiles(ctx)
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("ListFiles returned %d files, want 2", len(files))
	}
}

func TestListFilesEmpty(t *testing.T) {
	db := newOpenDB(t)
	defer db.Close()
	defer cleanupDB(t, db)

	files, err := db.ListFiles(context.Background())
	if err != nil {
		t.Fatalf("ListFiles on empty db: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestDeleteFile(t *testing.T) {
	db := newOpenDB(t)
	defer db.Close()
	defer cleanupDB(t, db)

	ctx := context.Background()
	db.RegisterFile(ctx, FileRecord{Path: "/delete_me.csv", Status: "indexed"})

	// Insert a row for this file
	db.InsertRows(ctx, []Row{
		{SourceFile: "/delete_me.csv", RowIndex: 0, ColumnNames: []string{"col1"}, Values: []string{"val1"}},
	})

	if err := db.DeleteFile(ctx, "/delete_me.csv"); err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}

	// Verify file is gone
	got, _ := db.GetFileRegistry(ctx, "/delete_me.csv")
	if got != nil {
		t.Errorf("file should be deleted")
	}

	// Verify rows are gone (stats should show 0)
	stats, _ := db.Stats(ctx)
	if stats["total_rows"].(int64) != 0 {
		t.Errorf("expected 0 rows after delete, got %d", stats["total_rows"])
	}
}

func TestInsertRows(t *testing.T) {
	db := newOpenDB(t)
	defer db.Close()
	defer cleanupDB(t, db)

	ctx := context.Background()
	rows := []Row{
		{SourceFile: "/data.csv", RowIndex: 0, ColumnNames: []string{"name", "age"}, Values: []string{"Alice", "30"}},
		{SourceFile: "/data.csv", RowIndex: 1, ColumnNames: []string{"name", "age"}, Values: []string{"Bob", "25"}},
		{SourceFile: "/data.csv", RowIndex: 2, ColumnNames: []string{"name", "age"}, Values: []string{"Charlie", "35"}},
	}

	stats, err := db.InsertRows(ctx, rows)
	if err != nil {
		t.Fatalf("InsertRows failed: %v", err)
	}
	if stats.RowsInserted != 3 {
		t.Errorf("RowsInserted = %d, want 3", stats.RowsInserted)
	}

	// Verify via stats
	kbStats, _ := db.Stats(ctx)
	if kbStats["total_rows"].(int64) != 3 {
		t.Errorf("total_rows = %d, want 3", kbStats["total_rows"])
	}
}

func TestInsertRowsEmpty(t *testing.T) {
	db := newOpenDB(t)
	defer db.Close()
	defer cleanupDB(t, db)

	stats, err := db.InsertRows(context.Background(), []Row{})
	if err != nil {
		t.Fatalf("InsertRows empty failed: %v", err)
	}
	if stats.RowsInserted != 0 {
		t.Errorf("RowsInserted = %d, want 0", stats.RowsInserted)
	}
}

func TestSearchRows(t *testing.T) {
	db := newOpenDB(t)
	defer db.Close()
	defer cleanupDB(t, db)

	ctx := context.Background()
	db.InsertRows(ctx, []Row{
		{SourceFile: "/data.csv", RowIndex: 0, Values: []string{"machine learning is fun"}},
		{SourceFile: "/data.csv", RowIndex: 1, Values: []string{"deep learning with neural networks"}},
		{SourceFile: "/data.csv", RowIndex: 2, Values: []string{"classic statistics and probability"}},
	})

	results, err := db.SearchRows(ctx, "learning", 10)
	if err != nil {
		t.Fatalf("SearchRows failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("SearchRows returned 0 results")
	}
}

func TestSearchRowsLimit(t *testing.T) {
	db := newOpenDB(t)
	defer db.Close()
	defer cleanupDB(t, db)

	ctx := context.Background()
	var rows []Row
	for i := 0; i < 20; i++ {
		rows = append(rows, Row{SourceFile: "/data.csv", RowIndex: i, Values: []string{"searchable content"}})
	}
	db.InsertRows(ctx, rows)

	results, err := db.SearchRows(ctx, "searchable", 5)
	if err != nil {
		t.Fatalf("SearchRows failed: %v", err)
	}
	if len(results) > 5 {
		t.Errorf("SearchRows returned %d results, want <= 5", len(results))
	}
}

func TestSearchRowsDefaultLimit(t *testing.T) {
	db := newOpenDB(t)
	defer db.Close()
	defer cleanupDB(t, db)

	ctx := context.Background()
	var rows []Row
	for i := 0; i < 20; i++ {
		rows = append(rows, Row{SourceFile: "/data.csv", RowIndex: i, Values: []string{"content for search"}})
	}
	db.InsertRows(ctx, rows)

	// limit <= 0 should default to 10
	results, err := db.SearchRows(ctx, "content", 0)
	if err != nil {
		t.Fatalf("SearchRows failed: %v", err)
	}
	if len(results) != 10 {
		t.Errorf("SearchRows with limit=0 returned %d, want 10", len(results))
	}
}

func TestSearchRowsNoMatch(t *testing.T) {
	db := newOpenDB(t)
	defer db.Close()
	defer cleanupDB(t, db)

	ctx := context.Background()
	db.InsertRows(ctx, []Row{
		{SourceFile: "/data.csv", RowIndex: 0, Values: []string{"nothing here"}},
	})

	results, err := db.SearchRows(ctx, "zzzzz_nonexistent", 10)
	if err != nil {
		t.Fatalf("SearchRows failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-matching query, got %d", len(results))
	}
}

func TestStats(t *testing.T) {
	db := newOpenDB(t)
	defer db.Close()
	defer cleanupDB(t, db)

	ctx := context.Background()

	// Empty db
	stats, err := db.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if stats["total_rows"].(int64) != 0 {
		t.Errorf("total_rows = %d, want 0", stats["total_rows"])
	}
	if stats["disk_size_bytes"].(int64) <= 0 {
		t.Errorf("disk_size_bytes should be > 0, got %d", stats["disk_size_bytes"])
	}

	// After insert
	db.RegisterFile(ctx, FileRecord{Path: "/test.csv", Status: "indexed"})
	db.InsertRows(ctx, []Row{
		{SourceFile: "/test.csv", RowIndex: 0, Values: []string{"data"}},
	})

	stats, _ = db.Stats(ctx)
	if stats["total_rows"].(int64) != 1 {
		t.Errorf("total_rows = %d, want 1", stats["total_rows"])
	}
	if stats["tracked_files"].(int64) != 1 {
		t.Errorf("tracked_files = %d, want 1", stats["tracked_files"])
	}
}

func TestDBInterface(t *testing.T) {
	var db DB = New()
	if db == nil {
		t.Fatal("New() returned nil")
	}
}

func TestOpenInvalidPath(t *testing.T) {
	db := New()
	// Try to open in a non-writable location (root dir)
	err := db.Open(context.Background(), DBConfig{Path: "/no-such-dir/kb.sqlite"})
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
}

func TestInsertThenDeleteThenInsert(t *testing.T) {
	db := newOpenDB(t)
	defer db.Close()
	defer cleanupDB(t, db)

	ctx := context.Background()
	db.InsertRows(ctx, []Row{
		{SourceFile: "/f.csv", RowIndex: 0, Values: []string{"row one"}},
		{SourceFile: "/f.csv", RowIndex: 1, Values: []string{"row two"}},
	})
	db.DeleteFile(ctx, "/f.csv")
	stats, _ := db.InsertRows(ctx, []Row{
		{SourceFile: "/f.csv", RowIndex: 0, Values: []string{"row three"}},
	})
	if stats.RowsInserted != 1 {
		t.Errorf("RowsInserted after re-insert = %d, want 1", stats.RowsInserted)
	}
}

// --- Helpers ---

func tempDBPath(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("", "kb_test_*.sqlite")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	f.Close()
	return f.Name()
}

func newOpenDB(t *testing.T) *sqliteDB {
	t.Helper()
	db := New().(*sqliteDB)
	path := tempDBPath(t)
	if err := db.Open(context.Background(), DBConfig{Path: path}); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	return db
}

func cleanupDB(t *testing.T, db *sqliteDB) {
	t.Helper()
	os.Remove(db.path)
}

// Ensure sqliteDB implements DB at compile time
var _ DB = (*sqliteDB)(nil)
