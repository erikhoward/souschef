package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/erikhoward/souschef/internal/ideas"
)

var ErrNotFound = errors.New("not found")

const ideaColumns = `
	id, title, raw_text, source, source_ref, stage, archived_at, merged_into_id,
	difficulty, duration_class, treatment, content_type, cuisine, primary_ingredient,
	equipment, visual_potential, seasonality, production_effort, field_overrides,
	enrichment_status, enrichment_error, enrichment_model, enriched_at,
	created_at, updated_at`

// encodeJSON marshals a string slice for storage. It takes []string rather
// than any so a nil or empty slice reliably encodes as "[]": a nil slice
// boxed into an any parameter is a non-nil interface value, which defeats a
// `v == nil` check and lets json.Marshal write the 4-byte string "null"
// instead. Callers must never see that — the frontend maps over these
// fields without a null guard.
func encodeJSON(v []string) (string, error) {
	if len(v) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func decodeStrings(raw sql.NullString) []string {
	if !raw.Valid || raw.String == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw.String), &out); err != nil {
		return nil
	}
	return out
}

func (s *Store) InsertIdea(ctx context.Context, i ideas.Idea) error {
	equipment, err := encodeJSON(i.Metadata.Equipment)
	if err != nil {
		return fmt.Errorf("encode equipment: %w", err)
	}
	overrides, err := encodeJSON(i.FieldOverrides)
	if err != nil {
		return fmt.Errorf("encode field_overrides: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO ideas (`+ideaColumns+`)
		VALUES (?,?,?,?,?,?,?,?, ?,?,?,?,?,?, ?,?,?,?,?, ?,?,?,?, ?,?)`,
		i.ID, i.Title, i.RawText, string(i.Source), nullString(i.SourceRef), string(i.Stage),
		i.ArchivedAt, i.MergedIntoID,
		nullString(i.Metadata.Difficulty), nullString(i.Metadata.DurationClass),
		nullString(i.Metadata.Treatment), nullString(i.Metadata.ContentType),
		nullString(i.Metadata.Cuisine), nullString(i.Metadata.PrimaryIngredient),
		equipment, nullString(i.Metadata.VisualPotential),
		nullString(i.Metadata.Seasonality), nullString(i.Metadata.ProductionEffort),
		overrides,
		string(i.Enrichment.Status), nullString(i.Enrichment.Error),
		nullString(i.Enrichment.Model), i.Enrichment.EnrichedAt,
		i.CreatedAt, i.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert idea %s: %w", i.ID, err)
	}
	return nil
}

func (s *Store) UpdateIdea(ctx context.Context, i ideas.Idea) error {
	equipment, err := encodeJSON(i.Metadata.Equipment)
	if err != nil {
		return fmt.Errorf("encode equipment: %w", err)
	}
	overrides, err := encodeJSON(i.FieldOverrides)
	if err != nil {
		return fmt.Errorf("encode field_overrides: %w", err)
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE ideas SET
			title = ?, raw_text = ?, source = ?, source_ref = ?, stage = ?,
			archived_at = ?, merged_into_id = ?,
			difficulty = ?, duration_class = ?, treatment = ?, content_type = ?,
			cuisine = ?, primary_ingredient = ?, equipment = ?, visual_potential = ?,
			seasonality = ?, production_effort = ?, field_overrides = ?,
			enrichment_status = ?, enrichment_error = ?, enrichment_model = ?, enriched_at = ?,
			updated_at = ?
		WHERE id = ?`,
		i.Title, i.RawText, string(i.Source), nullString(i.SourceRef), string(i.Stage),
		i.ArchivedAt, i.MergedIntoID,
		nullString(i.Metadata.Difficulty), nullString(i.Metadata.DurationClass),
		nullString(i.Metadata.Treatment), nullString(i.Metadata.ContentType),
		nullString(i.Metadata.Cuisine), nullString(i.Metadata.PrimaryIngredient),
		equipment, nullString(i.Metadata.VisualPotential),
		nullString(i.Metadata.Seasonality), nullString(i.Metadata.ProductionEffort),
		overrides,
		string(i.Enrichment.Status), nullString(i.Enrichment.Error),
		nullString(i.Enrichment.Model), i.Enrichment.EnrichedAt,
		time.Now().UTC(), i.ID,
	)
	if err != nil {
		return fmt.Errorf("update idea %s: %w", i.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetIdea(ctx context.Context, id string) (ideas.Idea, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+ideaColumns+` FROM ideas WHERE id = ?`, id)
	i, err := scanIdea(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ideas.Idea{}, ErrNotFound
	}
	return i, err
}

func (s *Store) DeleteIdea(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM ideas WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete idea %s: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// sortExpression maps a sort key to SQL. Difficulty and duration sort by
// their semantic order rather than alphabetically — "insane" must come after
// "moderate", which a plain ORDER BY on the text column would get wrong.
func sortExpression(sort string) string {
	switch sort {
	case "updated_at":
		return "updated_at"
	case "title":
		return "title COLLATE NOCASE"
	case "difficulty":
		return `CASE difficulty WHEN 'easy' THEN 1 WHEN 'moderate' THEN 2
		                        WHEN 'insane' THEN 3 ELSE 4 END`
	case "duration":
		return `CASE duration_class WHEN 'quick' THEN 1 WHEN 'average' THEN 2
		                            WHEN 'multi_day' THEN 3 ELSE 4 END`
	default:
		return "created_at"
	}
}

func defaultOrder(sort string) string {
	if sort == "" || sort == "created_at" || sort == "updated_at" {
		return "DESC"
	}
	return "ASC"
}

func (s *Store) ListIdeas(ctx context.Context, f ideas.ListFilter) ([]ideas.Idea, error) {
	var (
		where = []string{"merged_into_id IS NULL"}
		args  []any
	)

	switch f.Archived {
	case ideas.ArchivedOnly:
		where = append(where, "archived_at IS NOT NULL")
	case ideas.ArchivedAll:
		// no clause
	default:
		where = append(where, "archived_at IS NULL")
	}

	for col, val := range map[string]string{
		"stage":          f.Stage,
		"difficulty":     f.Difficulty,
		"duration_class": f.Duration,
		"treatment":      f.Treatment,
	} {
		if val != "" {
			where = append(where, col+" = ?")
			args = append(args, val)
		}
	}

	order := strings.ToUpper(f.Order)
	if order != "ASC" && order != "DESC" {
		order = defaultOrder(f.Sort)
	}

	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}

	query := `SELECT ` + ideaColumns + ` FROM ideas WHERE ` +
		strings.Join(where, " AND ") +
		` ORDER BY ` + sortExpression(f.Sort) + ` ` + order +
		` LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list ideas: %w", err)
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

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface{ Scan(dest ...any) error }

func scanIdea(sc scanner) (ideas.Idea, error) {
	var (
		i                                ideas.Idea
		source, stage, enrichStatus      string
		sourceRef, mergedInto            sql.NullString
		difficulty, duration, treatment  sql.NullString
		contentType, cuisine, ingredient sql.NullString
		equipment, overrides             sql.NullString
		visual, seasonality, effort      sql.NullString
		enrichErr, enrichModel           sql.NullString
		archivedAt, enrichedAt           sql.NullTime
	)

	err := sc.Scan(
		&i.ID, &i.Title, &i.RawText, &source, &sourceRef, &stage, &archivedAt, &mergedInto,
		&difficulty, &duration, &treatment, &contentType, &cuisine, &ingredient,
		&equipment, &visual, &seasonality, &effort, &overrides,
		&enrichStatus, &enrichErr, &enrichModel, &enrichedAt,
		&i.CreatedAt, &i.UpdatedAt,
	)
	if err != nil {
		return ideas.Idea{}, err
	}

	i.Source = ideas.Source(source)
	i.Stage = ideas.Stage(stage)
	i.SourceRef = sourceRef.String
	if archivedAt.Valid {
		t := archivedAt.Time
		i.ArchivedAt = &t
	}
	if mergedInto.Valid {
		v := mergedInto.String
		i.MergedIntoID = &v
	}

	i.Metadata = ideas.Metadata{
		Difficulty:        difficulty.String,
		DurationClass:     duration.String,
		Treatment:         treatment.String,
		ContentType:       contentType.String,
		Cuisine:           cuisine.String,
		PrimaryIngredient: ingredient.String,
		Equipment:         decodeStrings(equipment),
		VisualPotential:   visual.String,
		Seasonality:       seasonality.String,
		ProductionEffort:  effort.String,
	}
	if i.Metadata.Equipment == nil {
		i.Metadata.Equipment = []string{}
	}

	i.FieldOverrides = decodeStrings(overrides)
	if i.FieldOverrides == nil {
		i.FieldOverrides = []string{}
	}

	i.Enrichment = ideas.Enrichment{
		Status: ideas.EnrichmentStatus(enrichStatus),
		Error:  enrichErr.String,
		Model:  enrichModel.String,
	}
	if enrichedAt.Valid {
		t := enrichedAt.Time
		i.Enrichment.EnrichedAt = &t
	}

	i.Notes = []ideas.Note{}
	i.LinkedIDs = []string{}
	i.Metadata.Tags = []string{}
	return i, nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
