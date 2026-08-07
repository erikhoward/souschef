package store

import (
	"context"
	"testing"

	"github.com/erikhoward/souschef/internal/ideas"
)

func seedSearchCorpus(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()

	shawarma := fixtureIdea("i1", "Sheet-pan shawarma")
	shawarma.RawText = "sheet pan shawarma with a lemony feta situation"
	shawarma.Metadata.Cuisine = "Middle Eastern"
	shawarma.Metadata.PrimaryIngredient = "Chicken"

	eggs := fixtureIdea("i2", "Crispy chili eggs")
	eggs.RawText = "chili crisp eggs with scallion oil, very fast"
	eggs.Metadata.Cuisine = "Chinese-inspired"
	eggs.Metadata.PrimaryIngredient = "Eggs"

	soup := fixtureIdea("i3", "Cabbage soup")
	soup.RawText = "humble cabbage soup, slow"

	for _, i := range []ideas.Idea{shawarma, eggs, soup} {
		if err := s.InsertIdea(ctx, i); err != nil {
			t.Fatal(err)
		}
		// Metadata set post-insert must reach the index.
		if err := s.UpdateIdea(ctx, i); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSearchMatchesTitle(t *testing.T) {
	s := newTestStore(t)
	seedSearchCorpus(t, s)

	got, err := s.SearchIdeas(context.Background(), "shawarma", 10)
	if err != nil {
		t.Fatalf("SearchIdeas: %v", err)
	}
	if len(got) != 1 || got[0].ID != "i1" {
		t.Fatalf("want just i1, got %d results", len(got))
	}
}

func TestSearchMatchesRawText(t *testing.T) {
	s := newTestStore(t)
	seedSearchCorpus(t, s)

	got, _ := s.SearchIdeas(context.Background(), "scallion", 10)
	if len(got) != 1 || got[0].ID != "i2" {
		t.Errorf("raw_text should be searchable, got %d results", len(got))
	}
}

func TestSearchMatchesCuisineAndIngredient(t *testing.T) {
	s := newTestStore(t)
	seedSearchCorpus(t, s)

	if got, _ := s.SearchIdeas(context.Background(), "Chinese", 10); len(got) != 1 {
		t.Errorf("cuisine should be searchable, got %d", len(got))
	}
	if got, _ := s.SearchIdeas(context.Background(), "chicken", 10); len(got) != 1 {
		t.Errorf("primary_ingredient should be searchable, got %d", len(got))
	}
}

// Prefix matching is what makes typing a partial word in Telegram useful.
func TestSearchPrefixMatches(t *testing.T) {
	s := newTestStore(t)
	seedSearchCorpus(t, s)

	got, err := s.SearchIdeas(context.Background(), "shawar", 10)
	if err != nil {
		t.Fatalf("SearchIdeas: %v", err)
	}
	if len(got) != 1 || got[0].ID != "i1" {
		t.Errorf("partial word should match via prefix, got %d results", len(got))
	}
}

func TestSearchExcludesArchivedAndMerged(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedSearchCorpus(t, s)

	soup, _ := s.GetIdea(ctx, "i3")
	at := soup.CreatedAt
	soup.ArchivedAt = &at
	if err := s.UpdateIdea(ctx, soup); err != nil {
		t.Fatal(err)
	}

	got, _ := s.SearchIdeas(ctx, "cabbage", 10)
	if len(got) != 0 {
		t.Errorf("archived ideas must not appear in search, got %d", len(got))
	}
}

func TestSearchHandlesFTSSyntaxWithoutError(t *testing.T) {
	s := newTestStore(t)
	seedSearchCorpus(t, s)

	// A user typing punctuation must not produce a syntax error from FTS5.
	for _, q := range []string{`"unbalanced`, `AND`, `*`, `foo NEAR/`, `()`} {
		if _, err := s.SearchIdeas(context.Background(), q, 10); err != nil {
			t.Errorf("query %q should be sanitised, not error: %v", q, err)
		}
	}
}

func TestSearchIsRankOrdered(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// "eggs" in the title should outrank "eggs" mentioned once in body text.
	strong := fixtureIdea("s1", "Eggs eggs eggs")
	strong.RawText = "eggs"
	weak := fixtureIdea("s2", "Something else")
	weak.RawText = "there are eggs in this one somewhere among many other words"

	for _, i := range []ideas.Idea{strong, weak} {
		if err := s.InsertIdea(ctx, i); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.SearchIdeas(ctx, "eggs", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 results, got %d", len(got))
	}
	if got[0].ID != "s1" {
		t.Errorf("results must be rank ordered; got %q first", got[0].ID)
	}
}

func TestReindexTagsMakesTagsSearchable(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.InsertIdea(ctx, fixtureIdea("i1", "Nondescript")); err != nil {
		t.Fatal(err)
	}
	if err := s.ReindexTags(ctx, "i1", []string{"weeknight", "charred"}); err != nil {
		t.Fatalf("ReindexTags: %v", err)
	}

	got, _ := s.SearchIdeas(ctx, "weeknight", 10)
	if len(got) != 1 || got[0].ID != "i1" {
		t.Errorf("tags should be searchable after reindex, got %d", len(got))
	}
}
