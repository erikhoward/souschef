package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/erikhoward/souschef/internal/httpapi"
	"github.com/erikhoward/souschef/internal/ideas"
	"github.com/erikhoward/souschef/internal/store"
)

// stubEnricher lets the HTTP tests run with no API key. Enrichment behaviour
// itself is covered by the fixture tests in internal/enrich.
type stubEnricher struct {
	meta ideas.Metadata
	err  error
}

func (s stubEnricher) Enrich(context.Context, string) (ideas.Metadata, error) {
	return s.meta, s.err
}

func newTestServer(t *testing.T, e httpapi.Enricher) http.Handler {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	hub := httpapi.NewHub()
	t.Cleanup(hub.Close)

	return httpapi.New(httpapi.Deps{
		Ideas:    ideas.NewService(st),
		Enricher: e,
		Hub:      hub,
	})
}

func post(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeIdea(t *testing.T, rec *httptest.ResponseRecorder) ideas.Idea {
	t.Helper()
	var out ideas.Idea
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, rec.Body.String())
	}
	return out
}

func TestCreateIdeaReturnsImmediatelyAsPending(t *testing.T) {
	h := newTestServer(t, stubEnricher{})

	rec := post(t, h, "/api/ideas", map[string]string{
		"raw_text": "Sheet-pan shawarma with a lemony feta situation",
		"source":   "web",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}

	got := decodeIdea(t, rec)
	if got.ID == "" {
		t.Error("response must carry the new id")
	}
	if got.Enrichment.Status != ideas.EnrichPending {
		t.Errorf("Status = %q, want pending — the response must not wait on Claude",
			got.Enrichment.Status)
	}
	if got.Title == "" {
		t.Error("a provisional title must be present so the row is never blank")
	}
}

func TestCreateIdeaRejectsEmptyText(t *testing.T) {
	h := newTestServer(t, stubEnricher{})

	rec := post(t, h, "/api/ideas", map[string]string{"raw_text": "  ", "source": "web"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCreateIdeaRejectsUnknownSource(t *testing.T) {
	h := newTestServer(t, stubEnricher{})

	rec := post(t, h, "/api/ideas", map[string]string{"raw_text": "fine", "source": "carrier_pigeon"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an unknown source", rec.Code)
	}
}

func TestGetIdeaNotFoundIs404(t *testing.T) {
	h := newTestServer(t, stubEnricher{})

	req := httptest.NewRequest(http.MethodGet, "/api/ideas/nope", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestListIdeasReturnsArrayNotNull(t *testing.T) {
	h := newTestServer(t, stubEnricher{})

	req := httptest.NewRequest(http.MethodGet, "/api/ideas", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	// An empty backlog must serialise as [] — null would break .map() in the UI.
	if body := rec.Body.String(); !bytes.HasPrefix([]byte(body), []byte("[")) {
		t.Errorf("empty list should serialise as [], got %s", body)
	}
}

func TestPatchRecordsOverride(t *testing.T) {
	h := newTestServer(t, stubEnricher{})

	created := decodeIdea(t, post(t, h, "/api/ideas",
		map[string]string{"raw_text": "Chili eggs", "source": "web"}))

	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(map[string]any{"difficulty": "easy"})
	req := httptest.NewRequest(http.MethodPatch, "/api/ideas/"+created.ID, &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", rec.Code, rec.Body.String())
	}
	got := decodeIdea(t, rec)
	if got.Metadata.Difficulty != "easy" {
		t.Errorf("Difficulty = %q", got.Metadata.Difficulty)
	}
	if !got.HasOverride("difficulty") {
		t.Error("PATCH must record the field as overridden")
	}
}

func TestPatchRejectsUnknownField(t *testing.T) {
	h := newTestServer(t, stubEnricher{})

	created := decodeIdea(t, post(t, h, "/api/ideas",
		map[string]string{"raw_text": "Chili eggs", "source": "web"}))

	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(map[string]any{"enrichment_status": "ok"})
	req := httptest.NewRequest(http.MethodPatch, "/api/ideas/"+created.ID, &buf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 — enrichment_status is not user-correctable", rec.Code)
	}
}

func TestArchiveAndRestore(t *testing.T) {
	h := newTestServer(t, stubEnricher{})

	created := decodeIdea(t, post(t, h, "/api/ideas",
		map[string]string{"raw_text": "Archivable", "source": "web"}))

	if rec := post(t, h, "/api/ideas/"+created.ID+"/archive", nil); rec.Code != http.StatusOK {
		t.Fatalf("archive status = %d", rec.Code)
	}
	if got := decodeIdea(t, post(t, h, "/api/ideas/"+created.ID+"/restore", nil)); got.IsArchived() {
		t.Error("restore did not clear archived_at")
	}
}

func TestSearchEndpointUsesFTS(t *testing.T) {
	h := newTestServer(t, stubEnricher{})

	post(t, h, "/api/ideas", map[string]string{"raw_text": "sheet pan shawarma", "source": "web"})
	post(t, h, "/api/ideas", map[string]string{"raw_text": "cabbage soup", "source": "web"})

	req := httptest.NewRequest(http.MethodGet, "/api/ideas?q=shawarma", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var got []ideas.Idea
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("want 1 search result, got %d", len(got))
	}
}

func TestReenrichFailurePersistsVerbatimMessage(t *testing.T) {
	h := newTestServer(t, stubEnricher{err: errors.New("401 authentication_error: invalid x-api-key")})

	created := decodeIdea(t, post(t, h, "/api/ideas",
		map[string]string{"raw_text": "Will fail", "source": "web"}))

	// /reenrich runs synchronously so the failure is observable in the response.
	rec := post(t, h, "/api/ideas/"+created.ID+"/reenrich", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", rec.Code, rec.Body.String())
	}

	got := decodeIdea(t, rec)
	if got.Enrichment.Status != ideas.EnrichFailed {
		t.Errorf("Status = %q, want failed", got.Enrichment.Status)
	}
	if got.Enrichment.Error == "" {
		t.Fatal("a failure must leave a message on the row — never a silent stall")
	}
	if !bytes.Contains([]byte(got.Enrichment.Error), []byte("401")) {
		t.Errorf("Error = %q, want the provider message verbatim", got.Enrichment.Error)
	}
}

func TestReenrichSuccessAppliesMetadata(t *testing.T) {
	h := newTestServer(t, stubEnricher{meta: ideas.Metadata{
		Title: "Sheet-pan shawarma", Difficulty: "easy", DurationClass: "quick",
		Treatment: "elevated", ContentType: "recipe", Cuisine: "Middle Eastern",
		VisualPotential: "high", Seasonality: "all_year", ProductionEffort: "light",
	}})

	created := decodeIdea(t, post(t, h, "/api/ideas",
		map[string]string{"raw_text": "shawarma thing", "source": "web"}))

	got := decodeIdea(t, post(t, h, "/api/ideas/"+created.ID+"/reenrich", nil))
	if got.Enrichment.Status != ideas.EnrichOK {
		t.Errorf("Status = %q, want ok", got.Enrichment.Status)
	}
	if got.Metadata.Cuisine != "Middle Eastern" {
		t.Errorf("Cuisine = %q", got.Metadata.Cuisine)
	}
}
