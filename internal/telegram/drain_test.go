package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

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

func (e slowEnricher) Model() string { return "test-model" }

// fakeTelegram answers every Bot API method with a plausible success envelope,
// so a bot under test never touches api.telegram.org.
func fakeTelegram(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"result":{"message_id":42,"chat":{"id":1}}}`))
	}))
	t.Cleanup(srv.Close)

	c := NewClient("test-token")
	c.baseURL = srv.URL
	return c
}

func newTestBot(t *testing.T, svc *ideas.Service, e Enricher) *Bot {
	t.Helper()
	bot, err := New(Deps{
		Client:   fakeTelegram(t),
		Ideas:    svc,
		Enricher: e,
		ChatID:   1,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return bot
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	return st
}

// TestDrainWaitsForInFlightEnrichmentBeforeStoreCloses proves the shutdown
// sequencing this fix exists to establish on the Telegram surface. capture()
// detaches enrichment from the update loop, so a naive shutdown can close the
// store while that goroutine is still mid-write, losing the result to
// "sql: database is closed" and leaving the idea stuck at
// enrichment_status = 'pending'. That is worse in Telegram than on the web:
// RenderIdeaCard only offers Retry for a 'failed' idea, so the chat message
// stays at "Reading it now…" with no in-chat recovery whatsoever.
func TestDrainWaitsForInFlightEnrichmentBeforeStoreCloses(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	svc := ideas.NewService(st)
	bot := newTestBot(t, svc, slowEnricher{delay: 200 * time.Millisecond})

	if err := bot.capture(context.Background(), "chili crisp eggs", ideas.SourceTelegramText, "2"); err != nil {
		t.Fatalf("capture: %v", err)
	}

	// This mirrors what run() must do at shutdown: drain in-flight enrichment
	// before the store closes, bounded by a grace period well clear of the
	// enricher's delay.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	bot.Drain(ctx)

	// The store is still open (defer st.Close() above hasn't run yet), so a
	// failure here is the enrichment write losing a real race, not an
	// artifact of test cleanup ordering.
	list, err := svc.List(context.Background(), ideas.ListFilter{})
	if err != nil {
		t.Fatalf("List after drain: %v", err)
	}

	var found bool
	for _, idea := range list {
		if idea.RawText != "chili crisp eggs" {
			continue
		}
		found = true
		if idea.Enrichment.Status != ideas.EnrichOK {
			t.Fatalf("Status after drain = %q, want ok — the write must complete before Drain returns",
				idea.Enrichment.Status)
		}
		if idea.Metadata.Cuisine != "Test" {
			t.Errorf("Cuisine = %q, enrichment result did not persist", idea.Metadata.Cuisine)
		}
	}
	if !found {
		t.Fatal("the captured idea is missing from the store")
	}
}

// TestDrainReturnsWhenGracePeriodExpires proves Drain is bounded: a hung
// enrichment call must not block shutdown forever.
func TestDrainReturnsWhenGracePeriodExpires(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	svc := ideas.NewService(st)
	bot := newTestBot(t, svc, slowEnricher{delay: 2 * time.Second})

	if err := bot.capture(context.Background(), "slow one", ideas.SourceTelegramText, "1"); err != nil {
		t.Fatalf("capture: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	bot.Drain(ctx)
	if elapsed := time.Since(start); elapsed > 1*time.Second {
		t.Errorf("Drain took %v, want it bounded by the grace period, not the enricher's delay", elapsed)
	}
}

// TestDrainCoversVoiceAndRetryPaths pins the invariant that makes Drain
// trustworthy: every path that enriches in the background must be tracked.
// A bare `go b.enrichAndEdit(...)` anywhere would be invisible to shutdown,
// so this exercises the retry callback path too.
func TestDrainCoversRetryCallbackPath(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	svc := ideas.NewService(st)
	bot := newTestBot(t, svc, slowEnricher{delay: 150 * time.Millisecond})

	idea, err := svc.Create(context.Background(), "retry me", ideas.SourceTelegramText, "1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	update := Update{CallbackQuery: &CallbackQuery{
		ID:      "cb1",
		Data:    "retry:" + idea.ID,
		Message: &Message{MessageID: 7, Chat: Chat{ID: 1}},
	}}
	if err := bot.handleCallback(context.Background(), update); err != nil {
		t.Fatalf("handleCallback: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	bot.Drain(ctx)

	got, err := svc.Get(context.Background(), idea.ID)
	if err != nil {
		t.Fatalf("Get after drain: %v", err)
	}
	if got.Enrichment.Status != ideas.EnrichOK {
		t.Errorf("Status after drain = %q, want ok — the retry path must be tracked by Drain too",
			got.Enrichment.Status)
	}
}
