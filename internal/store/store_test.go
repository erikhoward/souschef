package store

import (
	"path/filepath"
	"testing"
)

// newTestStore opens a store in a temp file. We deliberately do not use
// :memory: — FTS5 external-content triggers must survive a close and reopen,
// and an in-memory database would hide a broken trigger definition.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenAppliesMigrations(t *testing.T) {
	s := newTestStore(t)

	var count int
	err := s.DB().QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='ideas'`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Fatal("ideas table was not created")
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	s1.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopening an already-migrated database must succeed: %v", err)
	}
	defer s2.Close()

	var applied int
	if err := s2.DB().QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&applied); err != nil {
		t.Fatalf("query: %v", err)
	}
	if applied != 1 {
		t.Errorf("schema_migrations rows = %d, want 1 (migration applied twice?)", applied)
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	s := newTestStore(t)

	_, err := s.DB().Exec(`INSERT INTO notes (id, idea_id, body, created_at)
	                       VALUES ('n1', 'does-not-exist', 'orphan', datetime('now'))`)
	if err == nil {
		t.Fatal("expected foreign key violation for note with unknown idea_id")
	}
}

func TestFTSTableExists(t *testing.T) {
	s := newTestStore(t)

	var count int
	err := s.DB().QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE name='ideas_fts'`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if count == 0 {
		t.Fatal("ideas_fts virtual table was not created")
	}
}
