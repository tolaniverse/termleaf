package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	store, err := open(filepath.Join(t.TempDir(), "positions.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})

	ctx := context.Background()
	want := Position{LineOffset: 42, Progress: 0.5, SourceOffset: 120, BlockFraction: 0.25, AnchorValid: true}
	if err := store.Save(ctx, "document", "page", want); err != nil {
		t.Fatalf("save position: %v", err)
	}

	got, err := store.Load(ctx, "document", "page")
	if err != nil {
		t.Fatalf("load position: %v", err)
	}
	if got != want {
		t.Fatalf("loaded position = %+v, want %+v", got, want)
	}
}

func TestStoreMigratesLegacySchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "positions.db")
	legacy, err := openLegacy(path)
	if err != nil {
		t.Fatalf("create legacy store: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy store: %v", err)
	}

	store, err := open(path)
	if err != nil {
		t.Fatalf("migrate legacy store: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Errorf("close migrated store: %v", err)
		}
	}()

	position, err := store.Load(context.Background(), "missing", "scroll")
	if err != nil {
		t.Fatalf("load from migrated store: %v", err)
	}
	if position.AnchorValid {
		t.Fatal("legacy position unexpectedly has a semantic anchor")
	}
}

func openLegacy(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE positions (
		document_id TEXT NOT NULL,
		mode TEXT NOT NULL,
		line_offset INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		PRIMARY KEY (document_id, mode)
	)`)
	return db, err
}

func TestBookmarksRoundTrip(t *testing.T) {
	store, err := open(filepath.Join(t.TempDir(), "positions.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	want := []Bookmark{
		{SourceOffset: 10, BlockFraction: 0.25, Label: "Introduction"},
		{SourceOffset: 80, BlockFraction: 0.5, Label: "Architecture"},
	}
	if err := store.ReplaceBookmarks(context.Background(), "doc", want); err != nil {
		t.Fatalf("replace bookmarks: %v", err)
	}
	got, err := store.LoadBookmarks(context.Background(), "doc")
	if err != nil {
		t.Fatalf("load bookmarks: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bookmarks = %+v, want %+v", got, want)
	}
}

func TestBookmarkLabelsAreSanitizedOnLoad(t *testing.T) {
	store, err := open(filepath.Join(t.TempDir(), "positions.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	_, err = store.db.Exec(`INSERT INTO bookmarks (document_id, source_offset, block_fraction, label) VALUES (?, ?, ?, ?)`, "doc", 1, 0.5, "safe\x1b]52;c;payload\a label")
	if err != nil {
		t.Fatalf("insert bookmark: %v", err)
	}
	bookmarks, err := store.LoadBookmarks(context.Background(), "doc")
	if err != nil {
		t.Fatalf("load bookmarks: %v", err)
	}
	if len(bookmarks) != 1 || strings.ContainsAny(bookmarks[0].Label, "\x1b\a") {
		t.Fatalf("unsafe bookmarks = %+v", bookmarks)
	}
}

func TestLoadRejectsCorruptSemanticAnchor(t *testing.T) {
	store, err := open(filepath.Join(t.TempDir(), "positions.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	_, err = store.db.Exec(`INSERT INTO positions (
		document_id, mode, line_offset, progress, source_offset, block_fraction, anchor_valid
	) VALUES ('doc', 'page', 10, 0.5, -20, 2.5, 7)`)
	if err != nil {
		t.Fatalf("insert corrupt anchor: %v", err)
	}
	position, err := store.Load(context.Background(), "doc", "page")
	if err != nil {
		t.Fatalf("load corrupt anchor: %v", err)
	}
	if position.AnchorValid || position.SourceOffset != 0 || position.BlockFraction != 0 {
		t.Fatalf("corrupt anchor survived validation: %+v", position)
	}
	if position.LineOffset != 10 || position.Progress != 0.5 {
		t.Fatalf("legacy fallbacks were lost: %+v", position)
	}
}

func TestStoreMissingPositionStartsAtZero(t *testing.T) {
	store, err := open(filepath.Join(t.TempDir(), "positions.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})

	got, err := store.Load(context.Background(), "missing", "scroll")
	if err != nil {
		t.Fatalf("load missing position: %v", err)
	}
	if got != (Position{}) {
		t.Fatalf("missing position = %+v, want zero value", got)
	}
}
