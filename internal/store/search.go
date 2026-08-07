package store

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/erikhoward/souschef/internal/ideas"
)

// sanitiseFTSQuery converts free-typed user input into a safe FTS5 MATCH
// expression. Users type things like `"unbalanced` or `NEAR/` without meaning
// FTS operators, and raw input would raise a syntax error rather than return
// nothing — so we strip everything non-alphanumeric, quote each remaining
// token, and append * for prefix matching.
func sanitiseFTSQuery(q string) string {
	fields := strings.FieldsFunc(q, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		if f == "" {
			continue
		}
		terms = append(terms, `"`+f+`"*`)
	}
	return strings.Join(terms, " ")
}

// SearchIdeas runs a ranked full-text search. Archived and merged ideas are
// excluded — search is for finding things you might still act on.
func (s *Store) SearchIdeas(ctx context.Context, query string, limit int) ([]ideas.Idea, error) {
	match := sanitiseFTSQuery(query)
	if match == "" {
		return []ideas.Idea{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT `+prefixed(ideaColumns, "i.")+`
		  FROM ideas_fts f
		  JOIN ideas i ON i.id = f.id
		 WHERE ideas_fts MATCH ?
		   AND i.archived_at IS NULL
		   AND i.merged_into_id IS NULL
		 ORDER BY rank
		 LIMIT ?`, match, limit)
	if err != nil {
		return nil, fmt.Errorf("search %q: %w", query, err)
	}
	defer rows.Close()

	out := []ideas.Idea{}
	for rows.Next() {
		i, err := scanIdea(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// ReindexTags refreshes the denormalised tag text in the FTS index. Tags live
// in a join table, so the AFTER UPDATE trigger on ideas cannot see them.
func (s *Store) ReindexTags(ctx context.Context, ideaID string, tags []string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE ideas_fts SET tags = ? WHERE id = ?`,
		strings.Join(tags, " "), ideaID)
	if err != nil {
		return fmt.Errorf("reindex tags for %s: %w", ideaID, err)
	}
	return nil
}

// prefixed qualifies each column in a comma-separated list with a table alias.
func prefixed(columns, alias string) string {
	parts := strings.Split(columns, ",")
	for idx, p := range parts {
		parts[idx] = alias + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}
