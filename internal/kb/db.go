// Package kb provides a persistent knowledge base backed by SQLite.
// It supports file tracking, row-level storage with FTS5 full-text search,
// and session-persistent data ingestion across restarts.
package kb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// FileRecord tracks a single file in the knowledge base.
type FileRecord struct {
	Path        string
	Hash        string // SHA256 of file content
	Size        int64
	RowCount    int
	Status      string // "pending", "indexed", "stale", "failed"
	FirstSeen   time.Time
	LastIndexed time.Time
	ErrorMsg    string
}

// Row represents a single data row in the KB.
type Row struct {
	SourceFile  string
	RowIndex    int
	ColumnNames []string
	Values      []string
}

// InsertStats holds insertion results.
type InsertStats struct {
	RowsInserted int64
}

// SearchResult is a single FTS5 hit.
type SearchResult struct {
	SourceFile string
	RowIndex   int
	Content    string
	Snippet    string
	Score      float64
}

// DBConfig configures the knowledge base.
type DBConfig struct {
	Path string // path to SQLite file
}

// DB is the knowledge base interface.
type DB interface {
	// Open initializes the database connection.
	Open(ctx context.Context, cfg DBConfig) error

	// Close shuts down the database.
	Close() error

	// RegisterFile adds or updates a file in the registry.
	RegisterFile(ctx context.Context, rec FileRecord) error

	// GetFileRegistry returns tracking info for a file.
	GetFileRegistry(ctx context.Context, path string) (*FileRecord, error)

	// ListFiles returns all tracked files.
	ListFiles(ctx context.Context) ([]FileRecord, error)

	// DeleteFile removes a file and its rows.
	DeleteFile(ctx context.Context, path string) error

	// InsertRows stores rows into the KB with FTS indexing.
	InsertRows(ctx context.Context, rows []Row) (*InsertStats, error)

	// SearchRows performs FTS5 full-text search.
	SearchRows(ctx context.Context, query string, limit int) ([]SearchResult, error)

	// Stats returns KB metrics.
	Stats(ctx context.Context) (map[string]any, error)
}

// New creates a new KB.
func New() DB {
	return &sqliteDB{}
}

type sqliteDB struct {
	db   *sql.DB
	mu   sync.RWMutex
	path string
}

func (s *sqliteDB) Open(ctx context.Context, cfg DBConfig) error {
	// Ensure directory exists
	dir := filepath.Dir(cfg.Path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create kb dir: %w", err)
	}

	db, err := sql.Open("sqlite", cfg.Path+"?_journal_mode=WAL&_busy_timeout=5000&_cache_size=-64000&_secure_delete=ON")
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}

	// WAL mode + concurrency settings
	db.SetMaxOpenConns(1)

	s.db = db
	s.path = cfg.Path

	// Run migrations
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return fmt.Errorf("migrate: %w", err)
	}

	// Set file permissions
	os.Chmod(cfg.Path, 0600)

	return nil
}

func (s *sqliteDB) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *sqliteDB) migrate(ctx context.Context) error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS kb_meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`INSERT OR IGNORE INTO kb_meta (key, value) VALUES ('schema_version', '1')`,

		`CREATE TABLE IF NOT EXISTS file_registry (
			path          TEXT PRIMARY KEY,
			hash          TEXT NOT NULL DEFAULT '',
			size          INTEGER NOT NULL DEFAULT 0,
			row_count     INTEGER NOT NULL DEFAULT 0,
			status        TEXT NOT NULL DEFAULT 'pending',
			first_seen    TEXT NOT NULL DEFAULT (datetime('now')),
			last_indexed  TEXT NOT NULL DEFAULT (datetime('now')),
			error_msg     TEXT NOT NULL DEFAULT ''
		)`,

		`CREATE TABLE IF NOT EXISTS kb_rows (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			source_file TEXT NOT NULL,
			row_index   INTEGER NOT NULL,
			content     TEXT NOT NULL,
			created_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_kb_rows_source ON kb_rows(source_file)`,

		`CREATE VIRTUAL TABLE IF NOT EXISTS kb_fts USING fts5(
			content,
			source_file UNINDEXED,
			row_index UNINDEXED,
			tokenize='porter unicode61',
			content=kb_rows
		)`,

		// Triggers to keep FTS in sync
		`CREATE TRIGGER IF NOT EXISTS kb_rows_ai AFTER INSERT ON kb_rows BEGIN
			INSERT INTO kb_fts(rowid, content, source_file, row_index)
			VALUES (new.id, new.content, new.source_file, new.row_index);
		END`,
		`CREATE TRIGGER IF NOT EXISTS kb_rows_ad AFTER DELETE ON kb_rows BEGIN
			INSERT INTO kb_fts(kb_fts, rowid, content, source_file, row_index)
			VALUES ('delete', old.id, old.content, old.source_file, old.row_index);
		END`,
	}

	for _, m := range migrations {
		if _, err := s.db.ExecContext(ctx, m); err != nil {
			return fmt.Errorf("migration failed: %w\nSQL: %s", err, m)
		}
	}
	return nil
}

func (s *sqliteDB) RegisterFile(ctx context.Context, rec FileRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO file_registry (path, hash, size, row_count, status, first_seen, last_indexed, error_msg)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(path) DO UPDATE SET
			hash=excluded.hash, size=excluded.size, row_count=excluded.row_count,
			status=excluded.status, last_indexed=excluded.last_indexed, error_msg=excluded.error_msg`,
		rec.Path, rec.Hash, rec.Size, rec.RowCount, rec.Status,
		rec.FirstSeen.Format(time.RFC3339), rec.LastIndexed.Format(time.RFC3339), rec.ErrorMsg,
	)
	return err
}

func (s *sqliteDB) GetFileRegistry(ctx context.Context, path string) (*FileRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRowContext(ctx, `SELECT path, hash, size, row_count, status, first_seen, last_indexed, error_msg FROM file_registry WHERE path = ?`, path)

	var rec FileRecord
	var firstSeen, lastIndexed string
	if err := row.Scan(&rec.Path, &rec.Hash, &rec.Size, &rec.RowCount, &rec.Status, &firstSeen, &lastIndexed, &rec.ErrorMsg); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	rec.FirstSeen, _ = time.Parse(time.RFC3339, firstSeen)
	rec.LastIndexed, _ = time.Parse(time.RFC3339, lastIndexed)
	return &rec, nil
}

func (s *sqliteDB) ListFiles(ctx context.Context) ([]FileRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, `SELECT path, hash, size, row_count, status, first_seen, last_indexed, error_msg FROM file_registry ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []FileRecord
	for rows.Next() {
		var rec FileRecord
		var firstSeen, lastIndexed string
		if err := rows.Scan(&rec.Path, &rec.Hash, &rec.Size, &rec.RowCount, &rec.Status, &firstSeen, &lastIndexed, &rec.ErrorMsg); err != nil {
			return nil, err
		}
		rec.FirstSeen, _ = time.Parse(time.RFC3339, firstSeen)
		rec.LastIndexed, _ = time.Parse(time.RFC3339, lastIndexed)
		records = append(records, rec)
	}
	return records, rows.Err()
}

func (s *sqliteDB) DeleteFile(ctx context.Context, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete rows for this file
	if _, err := tx.ExecContext(ctx, `DELETE FROM kb_rows WHERE source_file = ?`, path); err != nil {
		return err
	}
	// Delete registry entry
	if _, err := tx.ExecContext(ctx, `DELETE FROM file_registry WHERE path = ?`, path); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *sqliteDB) InsertRows(ctx context.Context, rows []Row) (*InsertStats, error) {
	if len(rows) == 0 {
		return &InsertStats{}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO kb_rows (source_file, row_index, content) VALUES (?, ?, ?)`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	inserted := int64(0)
	for _, row := range rows {
		// Build content string from all values
		var contentBuilder string
		for i, val := range row.Values {
			if i > 0 {
				contentBuilder += " "
			}
			if i < len(row.ColumnNames) {
				contentBuilder += row.ColumnNames[i] + ": "
			}
			contentBuilder += val
		}

		if _, err := stmt.ExecContext(ctx, row.SourceFile, row.RowIndex, contentBuilder); err != nil {
			return nil, err
		}
		inserted++
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &InsertStats{RowsInserted: inserted}, nil
}

func (s *sqliteDB) SearchRows(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	// Use FTS5 with snippet highlighting
	q := `SELECT
		k.source_file, k.row_index, k.content,
		snippet(kb_fts, 1, '<b>', '</b>', '...', 32) AS snippet,
		rank
	FROM kb_fts
	JOIN kb_rows k ON kb_fts.rowid = k.id
	WHERE kb_fts MATCH ?
	ORDER BY rank
	LIMIT ?`

	rows, err := s.db.QueryContext(ctx, q, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.SourceFile, &r.RowIndex, &r.Content, &r.Snippet, &r.Score); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *sqliteDB) Stats(ctx context.Context) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make(map[string]any)

	var rowCount int64
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM kb_rows`).Scan(&rowCount)
	stats["total_rows"] = rowCount

	var sourceCount int64
	s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT source_file) FROM kb_rows`).Scan(&sourceCount)
	stats["total_sources"] = sourceCount

	var fileCount int64
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM file_registry`).Scan(&fileCount)
	stats["tracked_files"] = fileCount

	var dbSize int64
	s.db.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&dbSize)
	var pageSize int64
	s.db.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&pageSize)
	stats["disk_size_bytes"] = dbSize * pageSize

	return stats, nil
}
