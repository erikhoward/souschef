package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/erikhoward/souschef/internal/ideas"
)

func fixtureIdea(id, title string) ideas.Idea {
	now := time.Now().UTC().Truncate(time.Second)
	return ideas.Idea{
		ID:             id,
		Title:          title,
		RawText:        title + " — raw capture text",
		Source:         ideas.SourceWeb,
		Stage:          ideas.StageIdea,
		FieldOverrides: []string{},
		Enrichment:     ideas.Enrichment{Status: ideas.EnrichPending},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func TestInsertAndGetIdea(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()

	want := fixtureIdea("i1", "Crispy chili eggs")
	if err := s.InsertIdea(ctx, want); err != nil {
		t.Fatalf("InsertIdea: %v", err)
	}

	got, err := s.GetIdea(ctx, "i1")
	if err != nil {
		t.Fatalf("GetIdea: %v", err)
	}
	if got.Title != want.Title || got.RawText != want.RawText {
		t.Errorf("round trip mismatch: got %+v", got)
	}
	if got.Enrichment.Status != ideas.EnrichPending {
		t.Errorf("Enrichment.Status = %q, want pending", got.Enrichment.Status)
	}
	if got.Stage != ideas.StageIdea {
		t.Errorf("Stage = %q, want idea", got.Stage)
	}
}

func TestGetIdeaNotFound(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()

	_, err := s.GetIdea(ctx, "nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUpdateIdeaPersistsMetadata(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()

	idea := fixtureIdea("i1", "Sheet-pan shawarma")
	if err := s.InsertIdea(ctx, idea); err != nil {
		t.Fatal(err)
	}

	enriched := time.Now().UTC().Truncate(time.Second)
	idea.Metadata = ideas.Metadata{
		Difficulty:        "easy",
		DurationClass:     "quick",
		Treatment:         "elevated",
		ContentType:       "recipe",
		Cuisine:           "Middle Eastern",
		PrimaryIngredient: "Chicken",
		Equipment:         []string{"sheet pan", "oven"},
		VisualPotential:   "high",
		Seasonality:       "all_year",
		ProductionEffort:  "light",
	}
	idea.Enrichment = ideas.Enrichment{
		Status: ideas.EnrichOK, Model: "claude-sonnet-5", EnrichedAt: &enriched,
	}
	if err := s.UpdateIdea(ctx, idea); err != nil {
		t.Fatalf("UpdateIdea: %v", err)
	}

	got, err := s.GetIdea(ctx, "i1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Metadata.Cuisine != "Middle Eastern" {
		t.Errorf("Cuisine = %q", got.Metadata.Cuisine)
	}
	if len(got.Metadata.Equipment) != 2 || got.Metadata.Equipment[0] != "sheet pan" {
		t.Errorf("Equipment = %v, want JSON round trip", got.Metadata.Equipment)
	}
	if got.Enrichment.Status != ideas.EnrichOK {
		t.Errorf("Enrichment.Status = %q, want ok", got.Enrichment.Status)
	}
}

func TestUpdateIdeaPersistsFailureVerbatim(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()

	idea := fixtureIdea("i1", "Anything")
	if err := s.InsertIdea(ctx, idea); err != nil {
		t.Fatal(err)
	}

	const msg = "401 authentication_error: invalid x-api-key (request_id=req_abc)"
	idea.Enrichment = ideas.Enrichment{Status: ideas.EnrichFailed, Error: msg}
	if err := s.UpdateIdea(ctx, idea); err != nil {
		t.Fatal(err)
	}

	got, _ := s.GetIdea(ctx, "i1")
	if got.Enrichment.Error != msg {
		t.Errorf("error text must survive verbatim.\n got: %q\nwant: %q", got.Enrichment.Error, msg)
	}
}

func TestListExcludesArchivedByDefault(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()

	active := fixtureIdea("i1", "Active idea")
	archived := fixtureIdea("i2", "Archived idea")
	at := time.Now().UTC()
	archived.ArchivedAt = &at

	for _, i := range []ideas.Idea{active, archived} {
		if err := s.InsertIdea(ctx, i); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.ListIdeas(ctx, ideas.ListFilter{})
	if err != nil {
		t.Fatalf("ListIdeas: %v", err)
	}
	if len(got) != 1 || got[0].ID != "i1" {
		t.Fatalf("default listing should show only active ideas, got %d", len(got))
	}

	got, _ = s.ListIdeas(ctx, ideas.ListFilter{Archived: ideas.ArchivedOnly})
	if len(got) != 1 || got[0].ID != "i2" {
		t.Errorf("archived=true should show only archived, got %d", len(got))
	}

	got, _ = s.ListIdeas(ctx, ideas.ListFilter{Archived: ideas.ArchivedAll})
	if len(got) != 2 {
		t.Errorf("archived=all should show both, got %d", len(got))
	}
}

func TestListExcludesMergedTombstones(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()

	primary := fixtureIdea("i1", "Primary")
	dup := fixtureIdea("i2", "Duplicate")
	if err := s.InsertIdea(ctx, primary); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertIdea(ctx, dup); err != nil {
		t.Fatal(err)
	}

	target := "i1"
	dup.MergedIntoID = &target
	if err := s.UpdateIdea(ctx, dup); err != nil {
		t.Fatal(err)
	}

	got, _ := s.ListIdeas(ctx, ideas.ListFilter{Archived: ideas.ArchivedAll})
	if len(got) != 1 || got[0].ID != "i1" {
		t.Errorf("merged tombstones must be excluded from listings, got %d", len(got))
	}

	// but a direct fetch still resolves
	if _, err := s.GetIdea(ctx, "i2"); err != nil {
		t.Errorf("direct fetch of a tombstone should still work: %v", err)
	}
}

func TestListFiltersByMetadata(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()

	easy := fixtureIdea("i1", "Easy one")
	easy.Metadata.Difficulty = "easy"
	easy.Metadata.DurationClass = "quick"

	hard := fixtureIdea("i2", "Hard one")
	hard.Metadata.Difficulty = "insane"
	hard.Metadata.DurationClass = "multi_day"

	for _, i := range []ideas.Idea{easy, hard} {
		if err := s.InsertIdea(ctx, i); err != nil {
			t.Fatal(err)
		}
	}

	got, _ := s.ListIdeas(ctx, ideas.ListFilter{Difficulty: "easy"})
	if len(got) != 1 || got[0].ID != "i1" {
		t.Errorf("difficulty filter failed, got %d", len(got))
	}

	got, _ = s.ListIdeas(ctx, ideas.ListFilter{Duration: "multi_day"})
	if len(got) != 1 || got[0].ID != "i2" {
		t.Errorf("duration filter failed, got %d", len(got))
	}
}

func TestListSortsDifficultySemanticallyNotAlphabetically(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()

	// Alphabetically: easy, insane, moderate. Semantically: easy, moderate, insane.
	for id, d := range map[string]string{"i1": "insane", "i2": "easy", "i3": "moderate"} {
		idea := fixtureIdea(id, "Idea "+id)
		idea.Metadata.Difficulty = d
		if err := s.InsertIdea(ctx, idea); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.ListIdeas(ctx, ideas.ListFilter{Sort: "difficulty", Order: "asc"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"easy", "moderate", "insane"}
	for i, w := range want {
		if got[i].Metadata.Difficulty != w {
			t.Errorf("position %d = %q, want %q (semantic order, not alphabetical)",
				i, got[i].Metadata.Difficulty, w)
		}
	}
}

func TestDeleteIdea(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()

	if err := s.InsertIdea(ctx, fixtureIdea("i1", "Doomed")); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteIdea(ctx, "i1"); err != nil {
		t.Fatalf("DeleteIdea: %v", err)
	}
	if _, err := s.GetIdea(ctx, "i1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound after delete, got %v", err)
	}
}
