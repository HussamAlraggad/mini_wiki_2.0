package kb

const SchemaSQL = `
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA cache_size = -64000;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS file_registry (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    path       TEXT    UNIQUE NOT NULL,
    abs_path   TEXT    NOT NULL,
    size       INTEGER NOT NULL DEFAULT 0,
    file_type  TEXT    NOT NULL DEFAULT '',
    checksum   TEXT    NOT NULL DEFAULT '',
    row_count  INTEGER NOT NULL DEFAULT 0,
    indexed_at TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS kb_rows (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    source     TEXT    NOT NULL,
    source_id  TEXT    NOT NULL DEFAULT '',
    row_number INTEGER NOT NULL DEFAULT 0,
    content    TEXT    NOT NULL,
    metadata   TEXT    NOT NULL DEFAULT '{}',
    created_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_kb_rows_source ON kb_rows(source);
CREATE INDEX IF NOT EXISTS idx_kb_rows_created ON kb_rows(created_at);

CREATE VIRTUAL TABLE IF NOT EXISTS kb_fts USING fts5(
    content,
    source UNINDEXED,
    content_rowid='id',
    content='kb_rows',
    tokenize='porter unicode61'
);

CREATE TRIGGER IF NOT EXISTS kb_rows_ai AFTER INSERT ON kb_rows BEGIN
    INSERT INTO kb_fts(rowid, content, source)
    VALUES (new.id, new.content, new.source);
END;

CREATE TRIGGER IF NOT EXISTS kb_rows_ad AFTER DELETE ON kb_rows BEGIN
    INSERT INTO kb_fts(kb_fts, rowid, content, source)
    VALUES ('delete', old.id, old.content, old.source);
END;

CREATE TRIGGER IF NOT EXISTS kb_rows_au AFTER UPDATE ON kb_rows BEGIN
    INSERT INTO kb_fts(kb_fts, rowid, content, source)
    VALUES ('delete', old.id, old.content, old.source);
    INSERT INTO kb_fts(rowid, content, source)
    VALUES (new.id, new.content, new.source);
END;
`
