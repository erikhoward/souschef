package ideas_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

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

	tomb, err := svc.Get(ctx, dup.ID)
	if err != nil {
		t.Fatalf("tombstone must still resolve: %v", err)
	}
	if tomb.MergedIntoID == nil || *tomb.MergedIntoID != primary.ID {
		t.Error("duplicate must be tombstoned pointing at the primary")
	}

	if _, err := svc.Merge(ctx, primary.ID, primary.ID); err == nil {
		t.Error("merging an idea into itself must fail")
	}
}
