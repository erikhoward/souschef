package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erikhoward/souschef/internal/httpapi"
	"github.com/erikhoward/souschef/internal/ideas"
	"github.com/erikhoward/souschef/internal/store"
)

// slowEnricher stands in for a Claude call still in flight when shutdown
// begins: it sleeps before returning a result.
type slowEnricher struct {
	delay time.Duration
}

func (e slowEnricher) Enrich(ctx context.Context, rawText string) (ideas.Metadata, error) {
	select {
	case <-time.After(e.delay):
	case <-ctx.Done():
		return ideas.Metadata{}, ctx.Err()
	}
	return ideas.Metadata{
		Title: "Drained", Difficulty: "easy", DurationClass: "quick",
		Treatment: "elevated", ContentType: "recipe", Cuisine: "Test",
		VisualPotential: "high", Seasonality: "all_year", ProductionEffort: "light",
	}, nil
}

func (e slowEnricher) Model() string { return "claude-stub-5" }

// TestDrainWaitsForInFlightEnrichmentBeforeStoreCloses proves the shutdown
// sequencing this task exists to fix: EnrichInBackground detaches a
// goroutine with no tracking of its own, so a naive shutdown can close the
// store while that goroutine is still mid-write, losing the result to
// "sql: database is closed" and leaving the idea stuck at
// enrichment_status = 'pending'. Drain must block until the write lands.
func TestDrainWaitsForInFlightEnrichmentBeforeStoreCloses(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()

	hub := httpapi.NewHub()
	defer hub.Close()

	svc := ideas.NewService(st)
	srv := httpapi.New(httpapi.Deps{
		Ideas:    svc,
		Enricher: slowEnricher{delay: 200 * time.Millisecond},
		Hub:      hub,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/ideas",
		strings.NewReader(`{"raw_text":"sheet pan shawarma","source":"web"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var created ideas.Idea
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode created idea: %v", err)
	}
	if created.Enrichment.Status != ideas.EnrichPending {
		t.Fatalf("Status = %q immediately after create, want pending", created.Enrichment.Status)
	}

	// This mirrors what run() must do at shutdown: drain in-flight
	// enrichment before the store closes, bounded by a grace period well
	// clear of the enricher's delay.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Drain(ctx)

	// The store is still open (defer st.Close() above hasn't run yet), so
	// a failure here would be enrichOnce losing a real race, not an
	// artifact of test cleanup ordering.
	got, err := svc.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("Get after drain: %v", err)
	}
	if got.Enrichment.Status != ideas.EnrichOK {
		t.Fatalf("Status after drain = %q, want ok — the write must complete before Drain returns", got.Enrichment.Status)
	}
	if got.Metadata.Cuisine != "Test" {
		t.Errorf("Cuisine = %q, enrichment result did not persist", got.Metadata.Cuisine)
	}
}

// TestDrainReturnsWhenGracePeriodExpires proves Drain is bounded: a hung
// enrichment call must not block shutdown forever.
func TestDrainReturnsWhenGracePeriodExpires(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()

	hub := httpapi.NewHub()
	defer hub.Close()

	svc := ideas.NewService(st)
	srv := httpapi.New(httpapi.Deps{
		Ideas:    svc,
		Enricher: slowEnricher{delay: 2 * time.Second},
		Hub:      hub,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/ideas",
		strings.NewReader(`{"raw_text":"slow one","source":"web"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", rec.Code, rec.Body.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	srv.Drain(ctx)
	if elapsed := time.Since(start); elapsed > 1*time.Second {
		t.Errorf("Drain took %v, want it bounded by the grace period, not the enricher's delay", elapsed)
	}
}
