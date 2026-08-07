package ideas

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrEmptyText = errors.New("idea text must not be empty")
	ErrTooLong   = errors.New("idea text must be 5000 characters or fewer")
	ErrSelfLink  = errors.New("an idea cannot be linked to itself")
	ErrSelfMerge = errors.New("an idea cannot be merged into itself")
)

const maxRawTextLen = 5000

// Repo is the slice of the store the service needs. Declaring it here rather
// than importing a concrete type keeps the dependency pointing inward and lets
// tests substitute a fake if one is ever warranted.
type Repo interface {
	InsertIdea(ctx context.Context, i Idea) error
	GetIdea(ctx context.Context, id string) (Idea, error)
	UpdateIdea(ctx context.Context, i Idea) error
	DeleteIdea(ctx context.Context, id string) error
	ListIdeas(ctx context.Context, f ListFilter) ([]Idea, error)
	SearchIdeas(ctx context.Context, q string, limit int) ([]Idea, error)

	AddNote(ctx context.Context, id, ideaID, body string, at time.Time) error
	NotesFor(ctx context.Context, ideaID string) ([]Note, error)
	NotesForMany(ctx context.Context, ideaIDs []string) (map[string][]Note, error)
	AddLink(ctx context.Context, a, b string) error
	RemoveLink(ctx context.Context, a, b string) error
	LinkedIDs(ctx context.Context, id string) ([]string, error)
	LinkedIDsForMany(ctx context.Context, ideaIDs []string) (map[string][]string, error)
	SetTags(ctx context.Context, ideaID string, tags []string) error
	TagsFor(ctx context.Context, ideaID string) ([]string, error)
	TagsForMany(ctx context.Context, ideaIDs []string) (map[string][]string, error)
}

type Service struct{ repo Repo }

func NewService(repo Repo) *Service { return &Service{repo: repo} }

// DeriveTitle produces a provisional title from raw capture text so a row is
// never blank before enrichment lands. It prefers the first sentence, and
// falls back to a 60-character truncation on a word boundary.
func DeriveTitle(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "Untitled idea"
	}
	if idx := strings.IndexAny(s, ".!?\n"); idx > 0 && idx <= 60 {
		return strings.TrimSpace(s[:idx])
	}
	if len(s) <= 60 {
		return s
	}

	// "…" is 3 bytes in UTF-8, so the cut must reserve room for it. The cut
	// point itself must also land on a rune boundary: slicing at a raw byte
	// offset can split a multi-byte rune in half and produce invalid UTF-8.
	// Captured recipe text routinely contains multi-byte runes — café,
	// jalapeño, crème fraîche, sauté, purée — and Telegram voice transcripts
	// are machine-generated prose we don't control, so this isn't a
	// hypothetical edge case. Walking the string rune-by-rune and only ever
	// accepting a whole rune's bytes guarantees the cut never lands mid-rune.
	const ellipsis = "…"
	budget := 60 - len(ellipsis)
	cutBytes := 0
	for i, r := range s {
		end := i + utf8.RuneLen(r)
		if end > budget {
			break
		}
		cutBytes = end
	}

	// The word-boundary backup below operates on the []rune form rather than
	// a byte index into the string, so it can't reintroduce the same
	// mid-rune-split hazard the walk above exists to avoid.
	cut := []rune(s[:cutBytes])
	if sp := lastSpaceRune(cut); sp > 20 {
		cut = cut[:sp]
	}
	return strings.TrimSpace(string(cut)) + ellipsis
}

// lastSpaceRune returns the index of the last space rune in cut, or -1 if
// there is none.
func lastSpaceRune(cut []rune) int {
	for i := len(cut) - 1; i >= 0; i-- {
		if cut[i] == ' ' {
			return i
		}
	}
	return -1
}

func (s *Service) Create(ctx context.Context, rawText string, source Source, sourceRef string) (Idea, error) {
	text := strings.TrimSpace(rawText)
	if text == "" {
		return Idea{}, ErrEmptyText
	}
	if len(text) > maxRawTextLen {
		return Idea{}, ErrTooLong
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Idea{}, fmt.Errorf("generate id: %w", err)
	}

	now := time.Now().UTC()
	idea := Idea{
		ID:             id.String(),
		Title:          DeriveTitle(text),
		RawText:        text,
		Source:         source,
		SourceRef:      sourceRef,
		Stage:          StageIdea,
		FieldOverrides: []string{},
		Enrichment:     Enrichment{Status: EnrichPending},
		Notes:          []Note{},
		LinkedIDs:      []string{},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.repo.InsertIdea(ctx, idea); err != nil {
		return Idea{}, err
	}
	return idea, nil
}

// maxMergeHops caps how far Get follows a chain of merges. Merging B into A
// and later A into C is legitimate and should resolve; a chain longer than
// this is a data problem, not a usage pattern, and must not turn a fetch into
// an unbounded walk.
const maxMergeHops = 8

// Get returns an idea with its notes and links populated, resolving a merged
// tombstone to the idea it was merged into.
//
// Resolution is the entire reason Merge leaves a tombstone instead of
// deleting the duplicate (spec §4): an old Telegram message or an existing
// link must still resolve to something useful. Excluding tombstones from
// lists without resolving them on direct fetch delivers only half of that —
// the link opens, and shows a dead record.
func (s *Service) Get(ctx context.Context, id string) (Idea, error) {
	idea, err := s.repo.GetIdea(ctx, id)
	if err != nil {
		return Idea{}, err
	}

	// A cycle (A→B→A, or a self-reference) is not reachable through Merge,
	// which refuses to merge an idea into itself — but a corrupted row must
	// not hang a fetch, so the walk tracks what it has seen and stops on a
	// repeat. Both guards return the last idea that did resolve rather than
	// an error: a stale-but-real idea is more use to someone following an
	// old link than a failure.
	seen := map[string]bool{idea.ID: true}
	for hops := 0; idea.MergedIntoID != nil && hops < maxMergeHops; hops++ {
		next := *idea.MergedIntoID
		if seen[next] {
			break
		}
		primary, err := s.repo.GetIdea(ctx, next)
		if err != nil {
			// The primary is unreadable — a dangling pointer the schema's
			// ON DELETE SET NULL should make unreachable. Stop and return the
			// tombstone. This does not swallow a genuine store failure: the
			// hydrate call below queries the same store and surfaces it.
			break
		}
		seen[next] = true
		idea = primary
	}

	return s.hydrate(ctx, idea)
}

func (s *Service) hydrate(ctx context.Context, idea Idea) (Idea, error) {
	notes, err := s.repo.NotesFor(ctx, idea.ID)
	if err != nil {
		return Idea{}, err
	}
	linked, err := s.repo.LinkedIDs(ctx, idea.ID)
	if err != nil {
		return Idea{}, err
	}
	tags, err := s.repo.TagsFor(ctx, idea.ID)
	if err != nil {
		return Idea{}, err
	}
	idea.Notes = notes
	idea.LinkedIDs = linked
	idea.Metadata.Tags = tags
	return idea, nil
}

// List returns ideas with notes, links, and tags populated, same as Get.
// The inspector renders straight from this list (there is no separate
// per-idea fetch on selection), so any of the three going unhydrated here
// reads to a user as data loss — a note or tag they saved appears to
// vanish on the next page load.
func (s *Service) List(ctx context.Context, f ListFilter) ([]Idea, error) {
	var (
		out []Idea
		err error
	)
	if f.Query != "" {
		out, err = s.repo.SearchIdeas(ctx, f.Query, f.Limit)
	} else {
		out, err = s.repo.ListIdeas(ctx, f)
	}
	if err != nil {
		return nil, err
	}
	return s.hydrateMany(ctx, out)
}

// hydrateMany fills in notes, links, and tags for a whole list result in
// three queries total rather than three per idea — the per-idea Repo
// methods (NotesFor etc.) would mean up to 1500 queries for a full 500-idea
// page. Every idea comes back with non-nil Notes, LinkedIDs, and
// Metadata.Tags even when it has none, matching the invariant the rest of
// the API relies on: the frontend maps over these fields unguarded.
func (s *Service) hydrateMany(ctx context.Context, list []Idea) ([]Idea, error) {
	if len(list) == 0 {
		return list, nil
	}

	ids := make([]string, len(list))
	for i, idea := range list {
		ids[i] = idea.ID
	}

	notes, err := s.repo.NotesForMany(ctx, ids)
	if err != nil {
		return nil, err
	}
	linked, err := s.repo.LinkedIDsForMany(ctx, ids)
	if err != nil {
		return nil, err
	}
	tags, err := s.repo.TagsForMany(ctx, ids)
	if err != nil {
		return nil, err
	}

	for i := range list {
		list[i].Notes = notes[list[i].ID]
		if list[i].Notes == nil {
			list[i].Notes = []Note{}
		}
		list[i].LinkedIDs = linked[list[i].ID]
		if list[i].LinkedIDs == nil {
			list[i].LinkedIDs = []string{}
		}
		list[i].Metadata.Tags = tags[list[i].ID]
		if list[i].Metadata.Tags == nil {
			list[i].Metadata.Tags = []string{}
		}
	}
	return list, nil
}

func (s *Service) Save(ctx context.Context, i Idea) error {
	return s.repo.UpdateIdea(ctx, i)
}

// ApplyEnrichment writes inferred metadata, skipping every field a human has
// already corrected. This is the invariant that lets "allow correction" and
// "retry enrichment" coexist.
func (s *Service) ApplyEnrichment(ctx context.Context, id string, m Metadata, model string) (Idea, error) {
	idea, err := s.repo.GetIdea(ctx, id)
	if err != nil {
		return Idea{}, err
	}

	set := func(field string, dst *string, val string) {
		if val != "" && !idea.HasOverride(field) {
			*dst = val
		}
	}

	if m.Title != "" && !idea.HasOverride("title") {
		idea.Title = m.Title
	}
	set("difficulty", &idea.Metadata.Difficulty, m.Difficulty)
	set("duration_class", &idea.Metadata.DurationClass, m.DurationClass)
	set("treatment", &idea.Metadata.Treatment, m.Treatment)
	set("content_type", &idea.Metadata.ContentType, m.ContentType)
	set("cuisine", &idea.Metadata.Cuisine, m.Cuisine)
	set("primary_ingredient", &idea.Metadata.PrimaryIngredient, m.PrimaryIngredient)
	set("visual_potential", &idea.Metadata.VisualPotential, m.VisualPotential)
	set("seasonality", &idea.Metadata.Seasonality, m.Seasonality)
	set("production_effort", &idea.Metadata.ProductionEffort, m.ProductionEffort)

	if len(m.Equipment) > 0 && !idea.HasOverride("equipment") {
		idea.Metadata.Equipment = m.Equipment
	}

	now := time.Now().UTC()
	idea.Enrichment = Enrichment{Status: EnrichOK, Model: model, EnrichedAt: &now}
	idea.UpdatedAt = now

	if err := s.repo.UpdateIdea(ctx, idea); err != nil {
		return Idea{}, err
	}
	if len(m.Tags) > 0 && !idea.HasOverride("tags") {
		if err := s.repo.SetTags(ctx, idea.ID, m.Tags); err != nil {
			return Idea{}, err
		}
	}
	return s.hydrate(ctx, idea)
}

// RecordEnrichmentFailure stores the provider's message verbatim. The idea
// itself is untouched and remains fully usable.
func (s *Service) RecordEnrichmentFailure(ctx context.Context, id, errText string) (Idea, error) {
	idea, err := s.repo.GetIdea(ctx, id)
	if err != nil {
		return Idea{}, err
	}
	idea.Enrichment.Status = EnrichFailed
	idea.Enrichment.Error = errText
	idea.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateIdea(ctx, idea); err != nil {
		return Idea{}, err
	}
	return s.hydrate(ctx, idea)
}

// MarkPending resets an idea for a retry.
func (s *Service) MarkPending(ctx context.Context, id string) (Idea, error) {
	idea, err := s.repo.GetIdea(ctx, id)
	if err != nil {
		return Idea{}, err
	}
	idea.Enrichment = Enrichment{Status: EnrichPending}
	idea.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateIdea(ctx, idea); err != nil {
		return Idea{}, err
	}
	return s.hydrate(ctx, idea)
}

// correctable maps a JSON patch key to the field it writes.
var correctable = map[string]func(*Idea, string){
	"title":              func(i *Idea, v string) { i.Title = v },
	"difficulty":         func(i *Idea, v string) { i.Metadata.Difficulty = v },
	"duration_class":     func(i *Idea, v string) { i.Metadata.DurationClass = v },
	"treatment":          func(i *Idea, v string) { i.Metadata.Treatment = v },
	"content_type":       func(i *Idea, v string) { i.Metadata.ContentType = v },
	"cuisine":            func(i *Idea, v string) { i.Metadata.Cuisine = v },
	"primary_ingredient": func(i *Idea, v string) { i.Metadata.PrimaryIngredient = v },
	"visual_potential":   func(i *Idea, v string) { i.Metadata.VisualPotential = v },
	"seasonality":        func(i *Idea, v string) { i.Metadata.Seasonality = v },
	"production_effort":  func(i *Idea, v string) { i.Metadata.ProductionEffort = v },
}

// Correct applies a human edit and records the field as overridden so
// re-enrichment leaves it alone.
func (s *Service) Correct(ctx context.Context, id string, patch map[string]any) (Idea, error) {
	idea, err := s.repo.GetIdea(ctx, id)
	if err != nil {
		return Idea{}, err
	}

	overrides := map[string]bool{}
	for _, f := range idea.FieldOverrides {
		overrides[f] = true
	}

	for key, raw := range patch {
		switch key {
		case "equipment", "tags":
			vals, ok := toStringSlice(raw)
			if !ok {
				return Idea{}, fmt.Errorf("%s must be an array of strings", key)
			}
			if key == "equipment" {
				idea.Metadata.Equipment = vals
			} else {
				if err := s.repo.SetTags(ctx, idea.ID, vals); err != nil {
					return Idea{}, err
				}
			}
			overrides[key] = true
		default:
			apply, known := correctable[key]
			if !known {
				return Idea{}, fmt.Errorf("field %q is not correctable", key)
			}
			str, ok := raw.(string)
			if !ok {
				return Idea{}, fmt.Errorf("%s must be a string", key)
			}
			apply(&idea, str)
			overrides[key] = true
		}
	}

	idea.FieldOverrides = idea.FieldOverrides[:0]
	for f := range overrides {
		idea.FieldOverrides = append(idea.FieldOverrides, f)
	}
	idea.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateIdea(ctx, idea); err != nil {
		return Idea{}, err
	}
	return s.hydrate(ctx, idea)
}

func toStringSlice(raw any) ([]string, bool) {
	switch v := raw.(type) {
	case []string:
		return v, true
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}

// Archive sets archived_at and deliberately leaves Stage alone.
func (s *Service) Archive(ctx context.Context, id string) (Idea, error) {
	return s.setArchived(ctx, id, true)
}

func (s *Service) Restore(ctx context.Context, id string) (Idea, error) {
	return s.setArchived(ctx, id, false)
}

func (s *Service) setArchived(ctx context.Context, id string, archived bool) (Idea, error) {
	idea, err := s.repo.GetIdea(ctx, id)
	if err != nil {
		return Idea{}, err
	}
	if archived {
		now := time.Now().UTC()
		idea.ArchivedAt = &now
	} else {
		idea.ArchivedAt = nil
	}
	idea.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateIdea(ctx, idea); err != nil {
		return Idea{}, err
	}
	return s.hydrate(ctx, idea)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.DeleteIdea(ctx, id)
}

func (s *Service) Link(ctx context.Context, a, b string) error {
	if a == b {
		return ErrSelfLink
	}
	return s.repo.AddLink(ctx, a, b)
}

func (s *Service) Unlink(ctx context.Context, a, b string) error {
	return s.repo.RemoveLink(ctx, a, b)
}

func (s *Service) AddNote(ctx context.Context, ideaID, body string) (Note, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return Note{}, ErrEmptyText
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Note{}, fmt.Errorf("generate id: %w", err)
	}
	now := time.Now().UTC()
	if err := s.repo.AddNote(ctx, id.String(), ideaID, body, now); err != nil {
		return Note{}, err
	}
	return Note{ID: id.String(), Body: body, CreatedAt: now}, nil
}

// Merge folds duplicate into primary: notes and links move across, the
// duplicate becomes a tombstone pointing at the primary rather than being
// deleted, so old references still resolve.
func (s *Service) Merge(ctx context.Context, primaryID, duplicateID string) (Idea, error) {
	if primaryID == duplicateID {
		return Idea{}, ErrSelfMerge
	}

	primary, err := s.repo.GetIdea(ctx, primaryID)
	if err != nil {
		return Idea{}, err
	}
	duplicate, err := s.repo.GetIdea(ctx, duplicateID)
	if err != nil {
		return Idea{}, err
	}

	dupNotes, err := s.repo.NotesFor(ctx, duplicateID)
	if err != nil {
		return Idea{}, err
	}
	for _, n := range dupNotes {
		if err := s.repo.AddNote(ctx, uuid.NewString(), primaryID, n.Body, n.CreatedAt); err != nil {
			return Idea{}, err
		}
	}

	dupLinks, err := s.repo.LinkedIDs(ctx, duplicateID)
	if err != nil {
		return Idea{}, err
	}
	for _, other := range dupLinks {
		if other == primaryID {
			continue
		}
		if err := s.repo.AddLink(ctx, primaryID, other); err != nil {
			return Idea{}, err
		}
		if err := s.repo.RemoveLink(ctx, duplicateID, other); err != nil {
			return Idea{}, err
		}
	}

	dupTags, err := s.repo.TagsFor(ctx, duplicateID)
	if err != nil {
		return Idea{}, err
	}
	if len(dupTags) > 0 {
		primaryTags, err := s.repo.TagsFor(ctx, primaryID)
		if err != nil {
			return Idea{}, err
		}
		seen := map[string]bool{}
		union := []string{}
		for _, t := range append(primaryTags, dupTags...) {
			if !seen[t] {
				seen[t] = true
				union = append(union, t)
			}
		}
		if err := s.repo.SetTags(ctx, primaryID, union); err != nil {
			return Idea{}, err
		}
	}

	duplicate.MergedIntoID = &primaryID
	duplicate.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateIdea(ctx, duplicate); err != nil {
		return Idea{}, err
	}

	primary.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateIdea(ctx, primary); err != nil {
		return Idea{}, err
	}
	return s.hydrate(ctx, primary)
}
