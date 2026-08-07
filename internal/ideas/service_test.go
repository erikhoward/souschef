package ideas_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/erikhoward/souschef/internal/ideas"
	"github.com/erikhoward/souschef/internal/store"
)

func newService(t *testing.T) (*ideas.Service, context.Context) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return ideas.NewService(s), context.Background()
}

func TestCreateDerivesTitleAndStaysPending(t *testing.T) {
	svc, ctx := newService(t)

	got, err := svc.Create(ctx, "Sheet-pan shawarma with a lemony feta situation. Maybe halloumi too.",
		ideas.SourceWeb, "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID == "" {
		t.Error("Create must assign an id")
	}
	if got.Title != "Sheet-pan shawarma with a lemony feta situation" {
		t.Errorf("Title = %q, want the first sentence", got.Title)
	}
	if got.Enrichment.Status != ideas.EnrichPending {
		t.Errorf("Status = %q, want pending — capture must never block on Claude", got.Enrichment.Status)
	}
	if got.Stage != ideas.StageIdea {
		t.Errorf("Stage = %q, want idea", got.Stage)
	}
}

func TestDeriveTitleTruncatesLongSingleSentence(t *testing.T) {
	long := strings.Repeat("a", 200)
	got := ideas.DeriveTitle(long)
	if len(got) > 60 {
		t.Errorf("len = %d, want <= 60", len(got))
	}
}

// Captured recipe text is routinely multi-byte: café, jalapeño, crème
// fraîche, sauté, purée, and Telegram voice transcripts we don't control
// (which may include emoji). A byte-offset cut can split a rune mid-way and
// silently corrupt the title — SQLite doesn't validate UTF-8 and
// encoding/json substitutes U+FFFD rather than erroring, so this fails
// quietly rather than loudly.
func TestDeriveTitleTruncatesOnRuneBoundaries(t *testing.T) {
	cases := map[string]string{
		"emoji":    strings.Repeat("😀", 200),
		"accented": strings.Repeat("é", 200),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			got := ideas.DeriveTitle(raw)
			if !utf8.ValidString(got) {
				t.Errorf("DeriveTitle produced invalid UTF-8: %q", got)
			}
			if n := utf8.RuneCountInString(got); n > 60 {
				t.Errorf("rune count = %d, want <= 60", n)
			}
			if got == "" {
				t.Error("DeriveTitle returned an empty string")
			}
		})
	}
}

func TestCreateRejectsEmptyAndOverlongText(t *testing.T) {
	svc, ctx := newService(t)

	if _, err := svc.Create(ctx, "   ", ideas.SourceWeb, ""); !errors.Is(err, ideas.ErrEmptyText) {
		t.Errorf("want ErrEmptyText, got %v", err)
	}
	if _, err := svc.Create(ctx, strings.Repeat("x", 5001), ideas.SourceWeb, ""); !errors.Is(err, ideas.ErrTooLong) {
		t.Errorf("want ErrTooLong, got %v", err)
	}
}

// The headline invariant from the spec.
func TestArchiveRestorePreservesStage(t *testing.T) {
	svc, ctx := newService(t)

	created, _ := svc.Create(ctx, "An idea that got somewhere", ideas.SourceWeb, "")
	created.Stage = ideas.StageBriefReady
	if err := svc.Save(ctx, created); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Archive(ctx, created.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	archived, _ := svc.Get(ctx, created.ID)
	if !archived.IsArchived() {
		t.Fatal("Archive did not set archived_at")
	}
	if archived.Stage != ideas.StageBriefReady {
		t.Errorf("archiving destroyed stage: got %q, want brief_ready", archived.Stage)
	}

	if _, err := svc.Restore(ctx, created.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	restored, _ := svc.Get(ctx, created.ID)
	if restored.IsArchived() {
		t.Error("Restore did not clear archived_at")
	}
	if restored.Stage != ideas.StageBriefReady {
		t.Errorf("restore reset stage to %q, want brief_ready preserved", restored.Stage)
	}
}

func TestCorrectRecordsOverrideAndSurvivesReenrichment(t *testing.T) {
	svc, ctx := newService(t)

	created, _ := svc.Create(ctx, "Chili eggs", ideas.SourceWeb, "")

	// First enrichment lands.
	_, err := svc.ApplyEnrichment(ctx, created.ID, ideas.Metadata{
		Title: "Chili eggs", Difficulty: "moderate", Cuisine: "Unclear",
	}, "claude-sonnet-5")
	if err != nil {
		t.Fatalf("ApplyEnrichment: %v", err)
	}

	// Human corrects difficulty.
	corrected, err := svc.Correct(ctx, created.ID, map[string]any{"difficulty": "easy"})
	if err != nil {
		t.Fatalf("Correct: %v", err)
	}
	if corrected.Metadata.Difficulty != "easy" {
		t.Errorf("Difficulty = %q, want easy", corrected.Metadata.Difficulty)
	}
	if !corrected.HasOverride("difficulty") {
		t.Fatal("Correct must record the field in FieldOverrides")
	}

	// Re-enrichment tries to set it back and must be refused for that field only.
	after, err := svc.ApplyEnrichment(ctx, created.ID, ideas.Metadata{
		Title: "Chili eggs", Difficulty: "insane", Cuisine: "Chinese-inspired",
	}, "claude-sonnet-5")
	if err != nil {
		t.Fatalf("ApplyEnrichment: %v", err)
	}
	if after.Metadata.Difficulty != "easy" {
		t.Errorf("re-enrichment clobbered a corrected field: got %q, want easy", after.Metadata.Difficulty)
	}
	if after.Metadata.Cuisine != "Chinese-inspired" {
		t.Errorf("re-enrichment should still update uncorrected fields, got %q", after.Metadata.Cuisine)
	}
}

func TestCorrectingTitleProtectsIt(t *testing.T) {
	svc, ctx := newService(t)

	created, _ := svc.Create(ctx, "some rambling capture", ideas.SourceWeb, "")
	if _, err := svc.Correct(ctx, created.ID, map[string]any{"title": "My Title"}); err != nil {
		t.Fatal(err)
	}

	after, _ := svc.ApplyEnrichment(ctx, created.ID,
		ideas.Metadata{Title: "Model Title", Difficulty: "easy"}, "claude-sonnet-5")
	if after.Title != "My Title" {
		t.Errorf("Title = %q, want the human's title preserved", after.Title)
	}
}

func TestRecordEnrichmentFailureKeepsIdeaUsable(t *testing.T) {
	svc, ctx := newService(t)

	created, _ := svc.Create(ctx, "Something worth keeping", ideas.SourceWeb, "")
	const msg = "401 authentication_error: invalid x-api-key"

	got, err := svc.RecordEnrichmentFailure(ctx, created.ID, msg)
	if err != nil {
		t.Fatalf("RecordEnrichmentFailure: %v", err)
	}
	if got.Enrichment.Status != ideas.EnrichFailed {
		t.Errorf("Status = %q, want failed", got.Enrichment.Status)
	}
	if got.Enrichment.Error != msg {
		t.Errorf("Error = %q, want verbatim provider message", got.Enrichment.Error)
	}
	if got.RawText != "Something worth keeping" {
		t.Error("the captured text must survive an enrichment failure intact")
	}
}

func TestLinkIsSymmetricAndRejectsSelf(t *testing.T) {
	svc, ctx := newService(t)

	a, _ := svc.Create(ctx, "Idea A", ideas.SourceWeb, "")
	b, _ := svc.Create(ctx, "Idea B", ideas.SourceWeb, "")

	if err := svc.Link(ctx, a.ID, b.ID); err != nil {
		t.Fatalf("Link: %v", err)
	}

	gotA, _ := svc.Get(ctx, a.ID)
	gotB, _ := svc.Get(ctx, b.ID)
	if len(gotA.LinkedIDs) != 1 || gotA.LinkedIDs[0] != b.ID {
		t.Errorf("A should link to B, got %v", gotA.LinkedIDs)
	}
	if len(gotB.LinkedIDs) != 1 || gotB.LinkedIDs[0] != a.ID {
		t.Errorf("link must be symmetric; B sees %v", gotB.LinkedIDs)
	}

	// Linking again in the other direction must not create a duplicate.
	if err := svc.Link(ctx, b.ID, a.ID); err != nil {
		t.Fatalf("re-link: %v", err)
	}
	gotA, _ = svc.Get(ctx, a.ID)
	if len(gotA.LinkedIDs) != 1 {
		t.Errorf("reverse link created a duplicate: %v", gotA.LinkedIDs)
	}

	if err := svc.Link(ctx, a.ID, a.ID); !errors.Is(err, ideas.ErrSelfLink) {
		t.Errorf("want ErrSelfLink, got %v", err)
	}
}

// List renders straight into the inspector with no separate per-idea
// fetch on selection — so if List doesn't hydrate the same way Get does,
// a saved note/link/tag correctly persists in the database but appears to
// have vanished the next time the list loads. This must fail against the
// pre-fix List (which just returned s.repo.ListIdeas(ctx, f) verbatim).
func TestListHydratesNotesLinksAndTags(t *testing.T) {
	svc, ctx := newService(t)

	a, err := svc.Create(ctx, "Idea with relations", ideas.SourceWeb, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Create(ctx, "Idea B", ideas.SourceWeb, "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.AddNote(ctx, a.ID, "a note worth keeping"); err != nil {
		t.Fatalf("AddNote: %v", err)
	}
	if err := svc.Link(ctx, a.ID, b.ID); err != nil {
		t.Fatalf("Link: %v", err)
	}
	if _, err := svc.Correct(ctx, a.ID, map[string]any{"tags": []string{"weeknight", "spicy"}}); err != nil {
		t.Fatalf("Correct tags: %v", err)
	}

	list, err := svc.List(ctx, ideas.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	var got *ideas.Idea
	for i := range list {
		if list[i].ID == a.ID {
			got = &list[i]
		}
	}
	if got == nil {
		t.Fatal("List did not return idea A at all")
	}

	if len(got.Notes) != 1 || got.Notes[0].Body != "a note worth keeping" {
		t.Errorf("List did not hydrate notes, got %#v", got.Notes)
	}
	if len(got.LinkedIDs) != 1 || got.LinkedIDs[0] != b.ID {
		t.Errorf("List did not hydrate links, got %#v", got.LinkedIDs)
	}
	if len(got.Metadata.Tags) != 2 {
		t.Errorf("List did not hydrate tags, got %#v", got.Metadata.Tags)
	}
}

// The main risk in batching relations for a whole list is a fan-out bug:
// row N's relations attached to idea M instead. Three ideas, each with its
// own distinct note, its own distinct tag, and a link only between two of
// them, is enough that any cross-wiring in the batched queries produces a
// visible, specific mismatch rather than an accidental pass.
func TestListDoesNotCrossWireRelationsBetweenIdeas(t *testing.T) {
	svc, ctx := newService(t)

	x, err := svc.Create(ctx, "Idea X", ideas.SourceWeb, "")
	if err != nil {
		t.Fatal(err)
	}
	y, err := svc.Create(ctx, "Idea Y", ideas.SourceWeb, "")
	if err != nil {
		t.Fatal(err)
	}
	z, err := svc.Create(ctx, "Idea Z", ideas.SourceWeb, "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.AddNote(ctx, x.ID, "note for x"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddNote(ctx, y.ID, "note for y"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddNote(ctx, z.ID, "note for z"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Link(ctx, x.ID, y.ID); err != nil {
		t.Fatal(err) // z is deliberately left unlinked
	}
	if _, err := svc.Correct(ctx, x.ID, map[string]any{"tags": []string{"tag-x"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Correct(ctx, y.ID, map[string]any{"tags": []string{"tag-y"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Correct(ctx, z.ID, map[string]any{"tags": []string{"tag-z"}}); err != nil {
		t.Fatal(err)
	}

	list, err := svc.List(ctx, ideas.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	byID := map[string]ideas.Idea{}
	for _, i := range list {
		byID[i.ID] = i
	}

	cases := []struct {
		id       string
		wantNote string
		wantTag  string
		wantLink []string
	}{
		{x.ID, "note for x", "tag-x", []string{y.ID}},
		{y.ID, "note for y", "tag-y", []string{x.ID}},
		{z.ID, "note for z", "tag-z", nil},
	}
	for _, c := range cases {
		got := byID[c.id]
		if len(got.Notes) != 1 || got.Notes[0].Body != c.wantNote {
			t.Errorf("idea %s: Notes = %#v, want exactly [%q]", c.id, got.Notes, c.wantNote)
		}
		if len(got.Metadata.Tags) != 1 || got.Metadata.Tags[0] != c.wantTag {
			t.Errorf("idea %s: Tags = %#v, want exactly [%q]", c.id, got.Metadata.Tags, c.wantTag)
		}
		if len(got.LinkedIDs) != len(c.wantLink) {
			t.Errorf("idea %s: LinkedIDs = %#v, want %#v", c.id, got.LinkedIDs, c.wantLink)
			continue
		}
		for _, want := range c.wantLink {
			found := false
			for _, l := range got.LinkedIDs {
				if l == want {
					found = true
				}
			}
			if !found {
				t.Errorf("idea %s: LinkedIDs = %#v, want to contain %q", c.id, got.LinkedIDs, want)
			}
		}
	}
}

// SQLite rejects "IN ()" outright, so an empty result set is the trap case
// for the batching fix — this must not error.
func TestListOnEmptyDatabaseReturnsEmptySliceWithoutError(t *testing.T) {
	svc, ctx := newService(t)

	got, err := svc.List(ctx, ideas.ListFilter{})
	if err != nil {
		t.Fatalf("List on empty db must not error, got %v", err)
	}
	if got == nil {
		t.Error("List on empty db should return an empty slice, not nil")
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

// Every idea List returns must have non-nil relation fields even when it
// has none of them — the frontend maps over Notes/LinkedIDs/Metadata.Tags
// without a null guard, same invariant already enforced for Equipment and
// FieldOverrides.
func TestListNeverReturnsNilRelationFields(t *testing.T) {
	svc, ctx := newService(t)

	if _, err := svc.Create(ctx, "Bare idea, no relations at all", ideas.SourceWeb, ""); err != nil {
		t.Fatal(err)
	}

	list, err := svc.List(ctx, ideas.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected exactly 1 idea, got %d", len(list))
	}
	got := list[0]
	if got.Notes == nil {
		t.Error("Notes is nil, want non-nil empty slice")
	}
	if got.LinkedIDs == nil {
		t.Error("LinkedIDs is nil, want non-nil empty slice")
	}
	if got.Metadata.Tags == nil {
		t.Error("Metadata.Tags is nil, want non-nil empty slice")
	}
}

func TestMergeUnionsAndTombstones(t *testing.T) {
	svc, ctx := newService(t)

	primary, _ := svc.Create(ctx, "Primary idea", ideas.SourceWeb, "")
	dup, _ := svc.Create(ctx, "Duplicate idea", ideas.SourceWeb, "")
	other, _ := svc.Create(ctx, "Unrelated", ideas.SourceWeb, "")

	if _, err := svc.AddNote(ctx, primary.ID, "note on primary"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddNote(ctx, dup.ID, "note on duplicate"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Link(ctx, dup.ID, other.ID); err != nil {
		t.Fatal(err)
	}

	merged, err := svc.Merge(ctx, primary.ID, dup.ID)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(merged.Notes) != 2 {
		t.Errorf("merge should union notes, got %d", len(merged.Notes))
	}
	if len(merged.LinkedIDs) != 1 || merged.LinkedIDs[0] != other.ID {
		t.Errorf("merge should inherit the duplicate's links, got %v", merged.LinkedIDs)
	}

	// The duplicate's id must still resolve — and resolve to the primary,
	// not to the dead record. That is what the tombstone exists for.
	tomb, err := svc.Get(ctx, dup.ID)
	if err != nil {
		t.Fatalf("tombstone must still resolve: %v", err)
	}
	if tomb.ID != primary.ID {
		t.Errorf("Get(duplicate).ID = %q, want the primary %q", tomb.ID, primary.ID)
	}

	if _, err := svc.Merge(ctx, primary.ID, primary.ID); err == nil {
		t.Error("merging an idea into itself must fail")
	}
}

// tombstone points a at b directly, bypassing Merge — which refuses the
// degenerate shapes these tests need to construct.
func tombstone(t *testing.T, svc *ideas.Service, ctx context.Context, a ideas.Idea, targetID string) {
	t.Helper()
	a.MergedIntoID = &targetID
	if err := svc.Save(ctx, a); err != nil {
		t.Fatalf("Save tombstone: %v", err)
	}
}

// Spec §6 promises merged tombstones are both excluded from lists and
// resolved on direct fetch. Exclusion alone is not enough: an old Telegram
// [Open] link, or a link from another idea, points at the duplicate's id and
// must land on the surviving idea.
func TestGetResolvesAMergedTombstoneToItsPrimary(t *testing.T) {
	svc, ctx := newService(t)

	primary, _ := svc.Create(ctx, "Surviving idea", ideas.SourceWeb, "")
	dup, _ := svc.Create(ctx, "Duplicate idea", ideas.SourceWeb, "")

	if _, err := svc.Merge(ctx, primary.ID, dup.ID); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	got, err := svc.Get(ctx, dup.ID)
	if err != nil {
		t.Fatalf("Get(duplicate): %v", err)
	}
	if got.ID != primary.ID {
		t.Errorf("Get(duplicate).ID = %q, want the primary %q", got.ID, primary.ID)
	}
	if got.Title != primary.Title {
		t.Errorf("Title = %q, want the primary's %q", got.Title, primary.Title)
	}
	if got.MergedIntoID != nil {
		t.Error("the resolved idea must be the primary itself, not another tombstone")
	}
}

// Merging B into A and later A into C is legitimate, and the oldest link must
// still land on the idea that is actually alive.
func TestGetFollowsAChainOfMerges(t *testing.T) {
	svc, ctx := newService(t)

	first, _ := svc.Create(ctx, "First", ideas.SourceWeb, "")
	second, _ := svc.Create(ctx, "Second", ideas.SourceWeb, "")
	third, _ := svc.Create(ctx, "Third", ideas.SourceWeb, "")

	if _, err := svc.Merge(ctx, second.ID, first.ID); err != nil {
		t.Fatalf("Merge first into second: %v", err)
	}
	if _, err := svc.Merge(ctx, third.ID, second.ID); err != nil {
		t.Fatalf("Merge second into third: %v", err)
	}

	got, err := svc.Get(ctx, first.ID)
	if err != nil {
		t.Fatalf("Get(first): %v", err)
	}
	if got.ID != third.ID {
		t.Errorf("Get(first).ID = %q, want the surviving idea %q", got.ID, third.ID)
	}
}

// A cycle is unreachable through Merge, but a corrupted row must never hang a
// fetch. Get must terminate and return a real idea.
func TestGetSurvivesCyclicAndSelfReferentialTombstones(t *testing.T) {
	t.Run("self-referential", func(t *testing.T) {
		svc, ctx := newService(t)
		idea, _ := svc.Create(ctx, "Points at itself", ideas.SourceWeb, "")
		tombstone(t, svc, ctx, idea, idea.ID)

		done := make(chan struct{})
		var got ideas.Idea
		var err error
		go func() {
			got, err = svc.Get(ctx, idea.ID)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Get did not terminate on a self-referential tombstone")
		}

		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.ID != idea.ID {
			t.Errorf("ID = %q, want the idea itself %q", got.ID, idea.ID)
		}
	})

	t.Run("two-idea cycle", func(t *testing.T) {
		svc, ctx := newService(t)
		a, _ := svc.Create(ctx, "A", ideas.SourceWeb, "")
		b, _ := svc.Create(ctx, "B", ideas.SourceWeb, "")
		tombstone(t, svc, ctx, a, b.ID)
		tombstone(t, svc, ctx, b, a.ID)

		done := make(chan struct{})
		var got ideas.Idea
		var err error
		go func() {
			got, err = svc.Get(ctx, a.ID)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Get did not terminate on a cyclic tombstone chain")
		}

		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		// Either end of the cycle is a defensible answer; the contract is
		// that Get returns a real idea instead of looping forever.
		if got.ID != a.ID && got.ID != b.ID {
			t.Errorf("ID = %q, want one of the two ideas in the cycle", got.ID)
		}
	})
}
