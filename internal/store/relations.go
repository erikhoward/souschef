package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/erikhoward/souschef/internal/ideas"
)

// placeholders builds a "?,?,?" clause and the matching args slice for an
// IN (...) over ids. Callers must check len(ids) > 0 themselves — SQLite
// rejects "IN ()" as a syntax error, so an empty id list is never sent here.
func placeholders(ids []string) (string, []any) {
	marks := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		marks[i] = "?"
		args[i] = id
	}
	return strings.Join(marks, ","), args
}

func (s *Store) AddNote(ctx context.Context, id, ideaID, body string, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notes (id, idea_id, body, created_at) VALUES (?,?,?,?)`,
		id, ideaID, body, at)
	if err != nil {
		return fmt.Errorf("add note to %s: %w", ideaID, err)
	}
	return nil
}

func (s *Store) DeleteNote(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM notes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete note %s: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) NotesFor(ctx context.Context, ideaID string) ([]ideas.Note, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, body, created_at FROM notes WHERE idea_id = ? ORDER BY created_at`, ideaID)
	if err != nil {
		return nil, fmt.Errorf("notes for %s: %w", ideaID, err)
	}
	defer rows.Close()

	out := []ideas.Note{}
	for rows.Next() {
		var n ideas.Note
		if err := rows.Scan(&n.ID, &n.Body, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// NotesForMany is the batched form of NotesFor, for hydrating a whole list
// result in three queries total instead of one per idea. The returned map
// only has entries for ideas that have at least one note — callers must
// treat a missing key as "no notes", not an error.
func (s *Store) NotesForMany(ctx context.Context, ideaIDs []string) (map[string][]ideas.Note, error) {
	out := map[string][]ideas.Note{}
	if len(ideaIDs) == 0 {
		return out, nil
	}
	marks, args := placeholders(ideaIDs)
	rows, err := s.db.QueryContext(ctx,
		`SELECT idea_id, id, body, created_at FROM notes
		  WHERE idea_id IN (`+marks+`) ORDER BY idea_id, created_at`, args...)
	if err != nil {
		return nil, fmt.Errorf("notes for many: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ideaID string
		var n ideas.Note
		if err := rows.Scan(&ideaID, &n.ID, &n.Body, &n.CreatedAt); err != nil {
			return nil, err
		}
		out[ideaID] = append(out[ideaID], n)
	}
	return out, rows.Err()
}

// orderPair returns the two ids in canonical order. The idea_links CHECK
// constraint requires a < b, which is how symmetry is enforced by the schema
// rather than by convention.
func orderPair(a, b string) (string, string) {
	if a < b {
		return a, b
	}
	return b, a
}

func (s *Store) AddLink(ctx context.Context, a, b string) error {
	lo, hi := orderPair(a, b)
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO idea_links (idea_a_id, idea_b_id) VALUES (?,?)`, lo, hi)
	if err != nil {
		return fmt.Errorf("link %s<->%s: %w", a, b, err)
	}
	return nil
}

func (s *Store) RemoveLink(ctx context.Context, a, b string) error {
	lo, hi := orderPair(a, b)
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM idea_links WHERE idea_a_id = ? AND idea_b_id = ?`, lo, hi)
	if err != nil {
		return fmt.Errorf("unlink %s<->%s: %w", a, b, err)
	}
	return nil
}

// LinkedIDs returns every idea linked to id, in either direction.
func (s *Store) LinkedIDs(ctx context.Context, id string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT idea_b_id FROM idea_links WHERE idea_a_id = ?
		UNION
		SELECT idea_a_id FROM idea_links WHERE idea_b_id = ?`, id, id)
	if err != nil {
		return nil, fmt.Errorf("linked ids for %s: %w", id, err)
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var other string
		if err := rows.Scan(&other); err != nil {
			return nil, err
		}
		out = append(out, other)
	}
	return out, rows.Err()
}

// LinkedIDsForMany is the batched form of LinkedIDs. A link row can name two
// ideas that are both in ideaIDs (a link within the current page) or just
// one (the other end is off-page) — each row is fanned out to whichever of
// its two ends is actually in ideaIDs, which may be one side, both, or
// (defensively) neither.
func (s *Store) LinkedIDsForMany(ctx context.Context, ideaIDs []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(ideaIDs) == 0 {
		return out, nil
	}
	inSet := make(map[string]bool, len(ideaIDs))
	for _, id := range ideaIDs {
		inSet[id] = true
	}

	marksA, argsA := placeholders(ideaIDs)
	marksB, argsB := placeholders(ideaIDs)
	query := `SELECT idea_a_id, idea_b_id FROM idea_links
	           WHERE idea_a_id IN (` + marksA + `) OR idea_b_id IN (` + marksB + `)`
	rows, err := s.db.QueryContext(ctx, query, append(argsA, argsB...)...)
	if err != nil {
		return nil, fmt.Errorf("linked ids for many: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		if inSet[a] {
			out[a] = append(out[a], b)
		}
		if inSet[b] {
			out[b] = append(out[b], a)
		}
	}
	return out, rows.Err()
}

func (s *Store) SetTags(ctx context.Context, ideaID string, tags []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM idea_tags WHERE idea_id = ?`, ideaID); err != nil {
		return fmt.Errorf("clear tags for %s: %w", ideaID, err)
	}
	for _, name := range tags {
		if name == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO tags (id, name) VALUES (lower(hex(randomblob(8))), ?)`, name); err != nil {
			return fmt.Errorf("upsert tag %q: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO idea_tags (idea_id, tag_id)
			SELECT ?, id FROM tags WHERE name = ?`, ideaID, name); err != nil {
			return fmt.Errorf("attach tag %q: %w", name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.ReindexTags(ctx, ideaID, tags)
}

func (s *Store) TagsFor(ctx context.Context, ideaID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.name FROM tags t
		  JOIN idea_tags it ON it.tag_id = t.id
		 WHERE it.idea_id = ? ORDER BY t.name`, ideaID)
	if err != nil {
		return nil, fmt.Errorf("tags for %s: %w", ideaID, err)
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// TagsForMany is the batched form of TagsFor.
func (s *Store) TagsForMany(ctx context.Context, ideaIDs []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(ideaIDs) == 0 {
		return out, nil
	}
	marks, args := placeholders(ideaIDs)
	rows, err := s.db.QueryContext(ctx, `
		SELECT it.idea_id, t.name
		  FROM tags t
		  JOIN idea_tags it ON it.tag_id = t.id
		 WHERE it.idea_id IN (`+marks+`)
		 ORDER BY it.idea_id, t.name`, args...)
	if err != nil {
		return nil, fmt.Errorf("tags for many: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ideaID, name string
		if err := rows.Scan(&ideaID, &name); err != nil {
			return nil, err
		}
		out[ideaID] = append(out[ideaID], name)
	}
	return out, rows.Err()
}
