package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS positions (
    document_id    TEXT NOT NULL,
    mode           TEXT NOT NULL,
    line_offset    INTEGER NOT NULL,
    progress       REAL NOT NULL DEFAULT 0,
    source_offset  INTEGER NOT NULL DEFAULT 0,
    block_fraction REAL NOT NULL DEFAULT 0,
    anchor_valid   INTEGER NOT NULL DEFAULT 0,
    updated_at     INTEGER NOT NULL DEFAULT (unixepoch()),
    PRIMARY KEY (document_id, mode)
);
CREATE TABLE IF NOT EXISTS bookmarks (
    document_id    TEXT NOT NULL,
    source_offset  INTEGER NOT NULL,
    block_fraction REAL NOT NULL,
    label          TEXT NOT NULL,
    created_at     INTEGER NOT NULL DEFAULT (unixepoch()),
    PRIMARY KEY (document_id, source_offset, block_fraction)
);`

// Position combines a semantic anchor with legacy restoration fallbacks.
type Position struct {
	LineOffset    int
	Progress      float64
	SourceOffset  int
	BlockFraction float64
	AnchorValid   bool
}

// Bookmark is a persisted semantic location with a display label.
type Bookmark struct {
	SourceOffset  int
	BlockFraction float64
	Label         string
}

type Store struct{ db *sql.DB }

func Open() (*Store, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("find user config directory: %w", err)
	}
	directory := filepath.Join(configDir, "termleaf")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create storage directory: %w", err)
	}
	return open(filepath.Join(directory, "positions.db"))
}

func open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open position database: %w", err)
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	if _, err := db.Exec(schema); err != nil {
		return nil, errors.Join(fmt.Errorf("initialize position database: %w", err), store.Close())
	}
	if err := store.migratePositionColumns(); err != nil {
		return nil, errors.Join(err, store.Close())
	}
	return store, nil
}

func (s *Store) migratePositionColumns() error {
	rows, err := s.db.Query(`PRAGMA table_info(positions)`)
	if err != nil {
		return fmt.Errorf("inspect position schema: %w", err)
	}
	columns := make(map[string]bool)
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return errors.Join(fmt.Errorf("read position schema: %w", err), rows.Close())
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return errors.Join(fmt.Errorf("iterate position schema: %w", err), rows.Close())
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close position schema rows: %w", err)
	}

	migrations := []struct{ name, sql string }{
		{"progress", `ALTER TABLE positions ADD COLUMN progress REAL NOT NULL DEFAULT 0`},
		{"source_offset", `ALTER TABLE positions ADD COLUMN source_offset INTEGER NOT NULL DEFAULT 0`},
		{"block_fraction", `ALTER TABLE positions ADD COLUMN block_fraction REAL NOT NULL DEFAULT 0`},
		{"anchor_valid", `ALTER TABLE positions ADD COLUMN anchor_valid INTEGER NOT NULL DEFAULT 0`},
	}
	for _, migration := range migrations {
		if columns[migration.name] {
			continue
		}
		if _, err := s.db.Exec(migration.sql); err != nil {
			return fmt.Errorf("migrate %s column: %w", migration.name, err)
		}
	}
	return nil
}

func (s *Store) Load(ctx context.Context, documentID, mode string) (Position, error) {
	var position Position
	var anchorValid int
	err := s.db.QueryRowContext(ctx, `
SELECT line_offset, progress, source_offset, block_fraction, anchor_valid
FROM positions WHERE document_id = ? AND mode = ?`, documentID, mode).Scan(
		&position.LineOffset, &position.Progress, &position.SourceOffset,
		&position.BlockFraction, &anchorValid,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Position{}, nil
	}
	if err != nil {
		return Position{}, fmt.Errorf("load reading position: %w", err)
	}
	position.Progress = finiteFraction(position.Progress)
	if anchorValid != 1 || position.SourceOffset < 0 || math.IsNaN(position.BlockFraction) || math.IsInf(position.BlockFraction, 0) || position.BlockFraction < 0 || position.BlockFraction > 1 {
		position.SourceOffset = 0
		position.BlockFraction = 0
		position.AnchorValid = false
		return position, nil
	}
	position.AnchorValid = true
	return position, nil
}

func (s *Store) Save(ctx context.Context, documentID, mode string, position Position) error {
	position.LineOffset = max(0, position.LineOffset)
	position.Progress = finiteFraction(position.Progress)
	position.SourceOffset = max(0, position.SourceOffset)
	position.BlockFraction = finiteFraction(position.BlockFraction)
	anchorValid := 0
	if position.AnchorValid {
		anchorValid = 1
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO positions (
    document_id, mode, line_offset, progress, source_offset, block_fraction, anchor_valid, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, unixepoch())
ON CONFLICT(document_id, mode) DO UPDATE SET
    line_offset = excluded.line_offset,
    progress = excluded.progress,
    source_offset = excluded.source_offset,
    block_fraction = excluded.block_fraction,
    anchor_valid = excluded.anchor_valid,
    updated_at = excluded.updated_at`, documentID, mode, position.LineOffset, position.Progress,
		position.SourceOffset, position.BlockFraction, anchorValid)
	if err != nil {
		return fmt.Errorf("save reading position: %w", err)
	}
	return nil
}

// LoadBookmarks returns bookmarks ordered by document position.
func (s *Store) LoadBookmarks(ctx context.Context, documentID string) ([]Bookmark, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT source_offset, block_fraction, label
FROM bookmarks WHERE document_id = ?
ORDER BY source_offset, block_fraction`, documentID)
	if err != nil {
		return nil, fmt.Errorf("load bookmarks: %w", err)
	}
	defer rows.Close()

	bookmarks := make([]Bookmark, 0)
	for rows.Next() {
		var bookmark Bookmark
		if err := rows.Scan(&bookmark.SourceOffset, &bookmark.BlockFraction, &bookmark.Label); err != nil {
			return nil, fmt.Errorf("read bookmark: %w", err)
		}
		if bookmark.SourceOffset < 0 || math.IsNaN(bookmark.BlockFraction) || math.IsInf(bookmark.BlockFraction, 0) || bookmark.BlockFraction < 0 || bookmark.BlockFraction > 1 {
			continue
		}
		bookmark.Label = sanitizeStoredLabel(bookmark.Label)
		bookmarks = append(bookmarks, bookmark)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bookmarks: %w", err)
	}
	return bookmarks, nil
}

// ReplaceBookmarks atomically replaces a document's bookmark set.
func (s *Store) ReplaceBookmarks(ctx context.Context, documentID string, bookmarks []Bookmark) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin bookmark update: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, tx.Rollback())
		}
	}()
	if _, err = tx.ExecContext(ctx, `DELETE FROM bookmarks WHERE document_id = ?`, documentID); err != nil {
		return fmt.Errorf("clear bookmarks: %w", err)
	}
	for _, bookmark := range bookmarks {
		if bookmark.SourceOffset < 0 {
			continue
		}
		if _, err = tx.ExecContext(ctx, `
INSERT INTO bookmarks (document_id, source_offset, block_fraction, label)
VALUES (?, ?, ?, ?)`, documentID, bookmark.SourceOffset, finiteFraction(bookmark.BlockFraction), bookmark.Label); err != nil {
			return fmt.Errorf("save bookmark: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit bookmarks: %w", err)
	}
	return nil
}

func sanitizeStoredLabel(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	value = strings.TrimSpace(value)
	if len([]rune(value)) > 80 {
		value = string([]rune(value)[:80])
	}
	return value
}

func finiteFraction(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return max(0, min(1, value))
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close position database: %w", err)
	}
	return nil
}
