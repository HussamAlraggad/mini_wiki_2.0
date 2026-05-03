// Package projectkb provides a per-directory SQLite knowledge base
// that lives inside the research project's working directory (.wiki/).
// It stores project-specific data: ingested rows, SRS outputs, query history,
// bookmarks, and filter configurations.
package projectkb

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

// Requirement represents an extracted FR or NFR requirement.
type Requirement struct {
	ID        string `json:"id"`
	Type      string `json:"type"` // "FR" or "NFR"
	Category  string `json:"category"`
	Statement string `json:"statement"`
	Source    string `json:"source"`
	Rationale string `json:"rationale"`
}

// MoscowEntry represents a MoSCoW-prioritized requirement.
type MoscowEntry struct {
	ID            string `json:"id"`
	Statement     string `json:"statement"`
	Moscow        string `json:"moscow"` // MUST, SHOULD, COULD, WON'T
	Justification string `json:"justification"`
}

// DFDComponent represents one DFD output at a time (entities, processes, stores, flows).
type DFDComponent struct {
	Type        string `json:"type"`        // "entity", "process", "data_store", "data_flow"
	Identifier  string `json:"identifier"`  // E1, P1, D1, etc.
	Name        string `json:"name"`
	Description string `json:"description"`
	Relations   string `json:"relations"` // JSON string for connections
}

// CSPECEntry represents a CSPEC activation or decision table row.
type CSPECEntry struct {
	ProcessID   string `json:"process_id"`
	ProcessName string `json:"process_name"`
	EntryType   string `json:"entry_type"` // "activation" or "decision"
	Data        string `json:"data"`       // JSON string
}

// SRSDocument represents a generated SRS document.
type SRSDocument struct {
	ID          int64     `json:"id"`
	ProjectName string    `json:"project_name"`
	Version     string    `json:"version"`
	Standard    string    `json:"standard"` // "ieee_830" or "ieee_29148"
	Content     string    `json:"content"`  // Full document text
	Format      string    `json:"format"`   // "markdown", "json", etc.
	CreatedAt   time.Time `json:"created_at"`
}

// HistoryEntry tracks a user query.
type HistoryEntry struct {
	ID        int64     `json:"id"`
	Query     string    `json:"query"`
	Response  string    `json:"response,omitempty"`
	Model     string    `json:"model"`
	FileRefs  string    `json:"file_refs,omitempty"` // JSON list of referenced files
	CreatedAt time.Time `json:"created_at"`
}

// Bookmark represents a saved finding.
type Bookmark struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	QueryRef    string    `json:"query_ref,omitempty"`
	Source      string    `json:"source,omitempty"` // "srs", "chat", "file"
	Tags        string    `json:"tags,omitempty"`   // comma-separated
	CreatedAt   time.Time `json:"created_at"`
}

// FilterState represents a saved filter configuration.
type FilterState struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Criteria  string    `json:"criteria"` // JSON filter criteria
	CreatedAt time.Time `json:"created_at"`
}

// --- Ranking types ---

// RankingResult stores the output of a /rank command.
type RankingResult struct {
	ID        int64     `json:"id"`
	Topic     string    `json:"topic"`
	Scores    string    `json:"scores"` // JSON blob of per-row scores
	CreatedAt time.Time `json:"created_at"`
}

// ComparisonSnapshot stores a /compare snapshot.
type ComparisonSnapshot struct {
	ID        int64     `json:"id"`
	RankingID int64     `json:"ranking_id"`
	Topic     string    `json:"topic"`
	Scores    string    `json:"scores"` // JSON blob
	CreatedAt time.Time `json:"created_at"`
}

// DiscardEntry records a /discard operation.
type DiscardEntry struct {
	ID            int64     `json:"id"`
	Threshold     float64   `json:"threshold"`
	RowsDiscarded int       `json:"rows_discarded"`
	CreatedAt     time.Time `json:"created_at"`
}

// DB is the per-directory project knowledge base interface.
type DB interface {
	// Open initializes the database (creates .wiki/ if needed).
	Open(ctx context.Context, projectDir string) error

	// Close shuts down the database.
	Close() error

	// ProjectDir returns the project directory path.
	ProjectDir() string

	// --- SRS Storage ---
	SaveRequirements(ctx context.Context, reqs []Requirement, runID string) error
	GetRequirements(ctx context.Context, runID string) ([]Requirement, error)

	SaveMoscow(ctx context.Context, entries []MoscowEntry, runID string) error
	GetMoscow(ctx context.Context, runID string) ([]MoscowEntry, error)

	SaveDFDComponents(ctx context.Context, components []DFDComponent, runID string) error
	GetDFDComponents(ctx context.Context, runID string) ([]DFDComponent, error)

	SaveCSPEC(ctx context.Context, entries []CSPECEntry, runID string) error
	GetCSPEC(ctx context.Context, runID string) ([]CSPECEntry, error)

	SaveSRSDocument(ctx context.Context, doc *SRSDocument) error
	GetSRSDocuments(ctx context.Context) ([]SRSDocument, error)

	// --- History ---
	SaveHistory(ctx context.Context, entry *HistoryEntry) error
	GetHistory(ctx context.Context, limit int) ([]HistoryEntry, error)
	SearchHistory(ctx context.Context, query string, limit int) ([]HistoryEntry, error)

	// --- Bookmarks ---
	SaveBookmark(ctx context.Context, bm *Bookmark) error
	GetBookmarks(ctx context.Context) ([]Bookmark, error)
	DeleteBookmark(ctx context.Context, id int64) error

	// --- Filters ---
	SaveFilterState(ctx context.Context, fs *FilterState) error
	GetFilterStates(ctx context.Context) ([]FilterState, error)
	DeleteFilterState(ctx context.Context, id int64) error

	// --- Ranking ---
	SaveRanking(ctx context.Context, r *RankingResult) error
	GetRankings(ctx context.Context) ([]RankingResult, error)
	SaveComparisonSnapshot(ctx context.Context, s *ComparisonSnapshot) error
	GetComparisonSnapshots(ctx context.Context, rankingID int64) ([]ComparisonSnapshot, error)
	SaveDiscardEntry(ctx context.Context, d *DiscardEntry) error
	GetDiscardEntries(ctx context.Context) ([]DiscardEntry, error)

	// --- Active Dataset ---
	SetActiveDataset(ctx context.Context, filePath, fileFormat string, rowCount int) error
	GetActiveDataset(ctx context.Context) (string, string, error)
}

// New creates a new Project KB.
func New() DB {
	return &projectDB{}
}

type projectDB struct {
	db         *sql.DB
	mu         sync.RWMutex
	projectDir string
	wikiDir    string
}

func (p *projectDB) Open(ctx context.Context, projectDir string) error {
	p.projectDir = projectDir
	wikiDir := filepath.Join(projectDir, ".wiki")

	if err := os.MkdirAll(wikiDir, 0700); err != nil {
		return fmt.Errorf("create .wiki dir: %w", err)
	}
	p.wikiDir = wikiDir

	dbPath := filepath.Join(wikiDir, "kb.sqlite")
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_cache_size=-64000")
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	p.db = db

	if err := p.migrate(ctx); err != nil {
		db.Close()
		return fmt.Errorf("migrate: %w", err)
	}

	os.Chmod(dbPath, 0600)
	return nil
}

func (p *projectDB) Close() error {
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}

func (p *projectDB) ProjectDir() string { return p.projectDir }

func (p *projectDB) migrate(ctx context.Context) error {
	migrations := []string{
		// SRS pipeline storage
		`CREATE TABLE IF NOT EXISTS srs_runs (
			run_id      TEXT PRIMARY KEY,
			created_at  TEXT NOT NULL DEFAULT (datetime('now')),
			status      TEXT NOT NULL DEFAULT 'in_progress'
		)`,
		`CREATE TABLE IF NOT EXISTS srs_requirements (
			id          TEXT NOT NULL,
			run_id      TEXT NOT NULL REFERENCES srs_runs(run_id),
			type        TEXT NOT NULL,
			category    TEXT NOT NULL,
			statement   TEXT NOT NULL,
			source      TEXT NOT NULL DEFAULT '',
			rationale   TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (id, run_id)
		)`,
		`CREATE TABLE IF NOT EXISTS srs_moscow (
			id            TEXT NOT NULL,
			run_id        TEXT NOT NULL REFERENCES srs_runs(run_id),
			statement     TEXT NOT NULL,
			moscow        TEXT NOT NULL,
			justification TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (id, run_id)
		)`,
		`CREATE TABLE IF NOT EXISTS srs_dfd_components (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id      TEXT NOT NULL REFERENCES srs_runs(run_id),
			comp_type   TEXT NOT NULL,
			identifier  TEXT NOT NULL,
			name        TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			relations   TEXT NOT NULL DEFAULT '{}'
		)`,
		`CREATE TABLE IF NOT EXISTS srs_cspec (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id      TEXT NOT NULL REFERENCES srs_runs(run_id),
			process_id  TEXT NOT NULL,
			process_name TEXT NOT NULL,
			entry_type  TEXT NOT NULL,
			data        TEXT NOT NULL DEFAULT '{}'
		)`,
		`CREATE TABLE IF NOT EXISTS srs_documents (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id      TEXT NOT NULL REFERENCES srs_runs(run_id),
			project_name TEXT NOT NULL,
			version     TEXT NOT NULL DEFAULT '1.0',
			standard    TEXT NOT NULL DEFAULT 'ieee_830',
			content     TEXT NOT NULL,
			format      TEXT NOT NULL DEFAULT 'markdown',
			created_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)`,

		// Query history
		`CREATE TABLE IF NOT EXISTS query_history (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			query       TEXT NOT NULL,
			response    TEXT NOT NULL DEFAULT '',
			model       TEXT NOT NULL DEFAULT '',
			file_refs   TEXT NOT NULL DEFAULT '[]',
			created_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_history_created ON query_history(created_at DESC)`,

		// Bookmarks
		`CREATE TABLE IF NOT EXISTS bookmarks (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			title       TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			query_ref   TEXT NOT NULL DEFAULT '',
			source      TEXT NOT NULL DEFAULT '',
			tags        TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_bookmarks_tags ON bookmarks(tags)`,

		// Filter states
		`CREATE TABLE IF NOT EXISTS filter_states (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			name        TEXT NOT NULL,
			criteria    TEXT NOT NULL DEFAULT '{}',
			created_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		// Phase 4: Ranking tables
		`CREATE TABLE IF NOT EXISTS ranking_results (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			topic       TEXT NOT NULL,
			scores      TEXT NOT NULL DEFAULT '[]',
			created_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS comparison_snapshots (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			ranking_id  INTEGER NOT NULL REFERENCES ranking_results(id),
			topic       TEXT NOT NULL,
			scores      TEXT NOT NULL DEFAULT '[]',
			created_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS discard_history (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			threshold       REAL NOT NULL,
			rows_discarded  INTEGER NOT NULL DEFAULT 0,
			created_at      TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		// Active ingested dataset tracking
		`CREATE TABLE IF NOT EXISTS active_dataset (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			file_path   TEXT NOT NULL,
			file_format TEXT NOT NULL DEFAULT 'csv',
			row_count   INTEGER NOT NULL DEFAULT 0,
			created_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
	}

	for _, m := range migrations {
		if _, err := p.db.ExecContext(ctx, m); err != nil {
			return fmt.Errorf("migration: %w\nSQL: %s", err, m)
		}
	}
	return nil
}

// --- SRS Storage ---

func (p *projectDB) SaveRequirements(ctx context.Context, reqs []Requirement, runID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Ensure run exists
	tx.ExecContext(ctx, `INSERT OR IGNORE INTO srs_runs (run_id) VALUES (?)`, runID)

	for _, r := range reqs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO srs_requirements (id, run_id, type, category, statement, source, rationale)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			r.ID, runID, r.Type, r.Category, r.Statement, r.Source, r.Rationale,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (p *projectDB) GetRequirements(ctx context.Context, runID string) ([]Requirement, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	rows, err := p.db.QueryContext(ctx,
		`SELECT id, type, category, statement, source, rationale FROM srs_requirements WHERE run_id = ? ORDER BY id`,
		runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reqs []Requirement
	for rows.Next() {
		var r Requirement
		if err := rows.Scan(&r.ID, &r.Type, &r.Category, &r.Statement, &r.Source, &r.Rationale); err != nil {
			return nil, err
		}
		reqs = append(reqs, r)
	}
	return reqs, rows.Err()
}

func (p *projectDB) SaveMoscow(ctx context.Context, entries []MoscowEntry, runID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tx.ExecContext(ctx, `INSERT OR IGNORE INTO srs_runs (run_id) VALUES (?)`, runID)

	for _, e := range entries {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO srs_moscow (id, run_id, statement, moscow, justification) VALUES (?, ?, ?, ?, ?)`,
			e.ID, runID, e.Statement, e.Moscow, e.Justification,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (p *projectDB) GetMoscow(ctx context.Context, runID string) ([]MoscowEntry, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	rows, err := p.db.QueryContext(ctx,
		`SELECT id, statement, moscow, justification FROM srs_moscow WHERE run_id = ? ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []MoscowEntry
	for rows.Next() {
		var e MoscowEntry
		if err := rows.Scan(&e.ID, &e.Statement, &e.Moscow, &e.Justification); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (p *projectDB) SaveDFDComponents(ctx context.Context, components []DFDComponent, runID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tx.ExecContext(ctx, `INSERT OR IGNORE INTO srs_runs (run_id) VALUES (?)`, runID)

	for _, c := range components {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO srs_dfd_components (run_id, comp_type, identifier, name, description, relations) VALUES (?, ?, ?, ?, ?, ?)`,
			runID, c.Type, c.Identifier, c.Name, c.Description, c.Relations,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (p *projectDB) GetDFDComponents(ctx context.Context, runID string) ([]DFDComponent, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	rows, err := p.db.QueryContext(ctx,
		`SELECT comp_type, identifier, name, description, relations FROM srs_dfd_components WHERE run_id = ? ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comps []DFDComponent
	for rows.Next() {
		var c DFDComponent
		if err := rows.Scan(&c.Type, &c.Identifier, &c.Name, &c.Description, &c.Relations); err != nil {
			return nil, err
		}
		comps = append(comps, c)
	}
	return comps, rows.Err()
}

func (p *projectDB) SaveCSPEC(ctx context.Context, entries []CSPECEntry, runID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tx.ExecContext(ctx, `INSERT OR IGNORE INTO srs_runs (run_id) VALUES (?)`, runID)

	for _, e := range entries {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO srs_cspec (run_id, process_id, process_name, entry_type, data) VALUES (?, ?, ?, ?, ?)`,
			runID, e.ProcessID, e.ProcessName, e.EntryType, e.Data,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (p *projectDB) GetCSPEC(ctx context.Context, runID string) ([]CSPECEntry, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	rows, err := p.db.QueryContext(ctx,
		`SELECT process_id, process_name, entry_type, data FROM srs_cspec WHERE run_id = ? ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []CSPECEntry
	for rows.Next() {
		var e CSPECEntry
		if err := rows.Scan(&e.ProcessID, &e.ProcessName, &e.EntryType, &e.Data); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (p *projectDB) SaveSRSDocument(ctx context.Context, doc *SRSDocument) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, err := p.db.ExecContext(ctx,
		`INSERT INTO srs_documents (run_id, project_name, version, standard, content, format) VALUES (?, ?, ?, ?, ?, ?)`,
		"", doc.ProjectName, doc.Version, doc.Standard, doc.Content, doc.Format)
	return err
}

func (p *projectDB) GetSRSDocuments(ctx context.Context) ([]SRSDocument, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	rows, err := p.db.QueryContext(ctx,
		`SELECT id, project_name, version, standard, content, format, created_at FROM srs_documents ORDER BY created_at DESC LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []SRSDocument
	for rows.Next() {
		var d SRSDocument
		var createdAt string
		if err := rows.Scan(&d.ID, &d.ProjectName, &d.Version, &d.Standard, &d.Content, &d.Format, &createdAt); err != nil {
			return nil, err
		}
		d.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// --- History ---

func (p *projectDB) SaveHistory(ctx context.Context, entry *HistoryEntry) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, err := p.db.ExecContext(ctx,
		`INSERT INTO query_history (query, response, model, file_refs) VALUES (?, ?, ?, ?)`,
		entry.Query, entry.Response, entry.Model, entry.FileRefs)
	return err
}

func (p *projectDB) GetHistory(ctx context.Context, limit int) ([]HistoryEntry, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, query, response, model, file_refs, created_at FROM query_history ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		var createdAt string
		if err := rows.Scan(&e.ID, &e.Query, &e.Response, &e.Model, &e.FileRefs, &createdAt); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (p *projectDB) SearchHistory(ctx context.Context, query string, limit int) ([]HistoryEntry, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, query, response, model, file_refs, created_at FROM query_history
		 WHERE query LIKE ? OR response LIKE ? ORDER BY created_at DESC LIMIT ?`,
		"%"+query+"%", "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		var createdAt string
		if err := rows.Scan(&e.ID, &e.Query, &e.Response, &e.Model, &e.FileRefs, &createdAt); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// --- Bookmarks ---

func (p *projectDB) SaveBookmark(ctx context.Context, bm *Bookmark) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, err := p.db.ExecContext(ctx,
		`INSERT INTO bookmarks (title, description, query_ref, source, tags) VALUES (?, ?, ?, ?, ?)`,
		bm.Title, bm.Description, bm.QueryRef, bm.Source, bm.Tags)
	return err
}

func (p *projectDB) GetBookmarks(ctx context.Context) ([]Bookmark, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	rows, err := p.db.QueryContext(ctx,
		`SELECT id, title, description, query_ref, source, tags, created_at FROM bookmarks ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookmarks []Bookmark
	for rows.Next() {
		var b Bookmark
		var createdAt string
		if err := rows.Scan(&b.ID, &b.Title, &b.Description, &b.QueryRef, &b.Source, &b.Tags, &createdAt); err != nil {
			return nil, err
		}
		b.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		bookmarks = append(bookmarks, b)
	}
	return bookmarks, rows.Err()
}

func (p *projectDB) DeleteBookmark(ctx context.Context, id int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err := p.db.ExecContext(ctx, `DELETE FROM bookmarks WHERE id = ?`, id)
	return err
}

// --- Filter States ---

func (p *projectDB) SaveFilterState(ctx context.Context, fs *FilterState) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, err := p.db.ExecContext(ctx,
		`INSERT INTO filter_states (name, criteria) VALUES (?, ?)`,
		fs.Name, fs.Criteria)
	return err
}

func (p *projectDB) GetFilterStates(ctx context.Context) ([]FilterState, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	rows, err := p.db.QueryContext(ctx,
		`SELECT id, name, criteria, created_at FROM filter_states ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var states []FilterState
	for rows.Next() {
		var fs FilterState
		var createdAt string
		if err := rows.Scan(&fs.ID, &fs.Name, &fs.Criteria, &createdAt); err != nil {
			return nil, err
		}
		fs.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		states = append(states, fs)
	}
	return states, rows.Err()
}

func (p *projectDB) DeleteFilterState(ctx context.Context, id int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err := p.db.ExecContext(ctx, `DELETE FROM filter_states WHERE id = ?`, id)
	return err
}

// --- Ranking implementations ---

func (p *projectDB) SaveRanking(ctx context.Context, r *RankingResult) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	res, err := p.db.ExecContext(ctx,
		`INSERT INTO ranking_results (topic, scores) VALUES (?, ?)`,
		r.Topic, r.Scores)
	if err != nil {
		return err
	}
	r.ID, _ = res.LastInsertId()
	return nil
}

func (p *projectDB) GetRankings(ctx context.Context) ([]RankingResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, topic, scores, created_at FROM ranking_results ORDER BY created_at DESC LIMIT 10`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []RankingResult
	for rows.Next() {
		var r RankingResult
		var createdAt string
		if err := rows.Scan(&r.ID, &r.Topic, &r.Scores, &createdAt); err != nil {
			return nil, err
		}
		r.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		results = append(results, r)
	}
	return results, rows.Err()
}

func (p *projectDB) SaveComparisonSnapshot(ctx context.Context, s *ComparisonSnapshot) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	res, err := p.db.ExecContext(ctx,
		`INSERT INTO comparison_snapshots (ranking_id, topic, scores) VALUES (?, ?, ?)`,
		s.RankingID, s.Topic, s.Scores)
	if err != nil {
		return err
	}
	s.ID, _ = res.LastInsertId()
	return nil
}

func (p *projectDB) GetComparisonSnapshots(ctx context.Context, rankingID int64) ([]ComparisonSnapshot, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, ranking_id, topic, scores, created_at FROM comparison_snapshots WHERE ranking_id = ? ORDER BY created_at DESC`,
		rankingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snaps []ComparisonSnapshot
	for rows.Next() {
		var s ComparisonSnapshot
		var createdAt string
		if err := rows.Scan(&s.ID, &s.RankingID, &s.Topic, &s.Scores, &createdAt); err != nil {
			return nil, err
		}
		s.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		snaps = append(snaps, s)
	}
	return snaps, rows.Err()
}

func (p *projectDB) SaveDiscardEntry(ctx context.Context, d *DiscardEntry) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO discard_history (threshold, rows_discarded) VALUES (?, ?)`,
		d.Threshold, d.RowsDiscarded)
	return err
}

func (p *projectDB) GetDiscardEntries(ctx context.Context) ([]DiscardEntry, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, threshold, rows_discarded, created_at FROM discard_history ORDER BY created_at DESC LIMIT 10`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []DiscardEntry
	for rows.Next() {
		var d DiscardEntry
		var createdAt string
		if err := rows.Scan(&d.ID, &d.Threshold, &d.RowsDiscarded, &createdAt); err != nil {
			return nil, err
		}
		d.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		entries = append(entries, d)
	}
	return entries, rows.Err()
}

// --- Active Dataset ---

func (p *projectDB) SetActiveDataset(ctx context.Context, filePath, fileFormat string, rowCount int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err := p.db.ExecContext(ctx, `DELETE FROM active_dataset`)
	if err != nil {
		return err
	}
	_, err = p.db.ExecContext(ctx,
		`INSERT INTO active_dataset (file_path, file_format, row_count) VALUES (?, ?, ?)`,
		filePath, fileFormat, rowCount)
	return err
}

func (p *projectDB) GetActiveDataset(ctx context.Context) (string, string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var filePath, fileFormat string
	err := p.db.QueryRowContext(ctx,
		`SELECT file_path, file_format FROM active_dataset ORDER BY id DESC LIMIT 1`).Scan(&filePath, &fileFormat)
	if err != nil {
		return "", "", err
	}
	return filePath, fileFormat, nil
}
