package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/erikhoward/souschef/internal/httpapi"
)

func TestStaticFallsThroughToAPI(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := httpapi.WithStatic(api)

	req := httptest.NewRequest(http.MethodGet, "/api/ideas", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Errorf("API routes must not be intercepted by the static handler, got %d", rec.Code)
	}
}

func TestStaticServesSPAFallbackForClientRoutes(t *testing.T) {
	h := httpapi.WithStatic(http.NotFoundHandler())

	// /ideas/<uuid> is a react-router path with no file behind it. It must
	// return index.html, not 404, or Telegram's deep links break on reload.
	req := httptest.NewRequest(http.MethodGet, "/ideas/0191f0c2-1234-7890-abcd-ef0123456789", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Error("client-side routes must fall back to index.html")
	}
}

func TestEventsRouteIsNotIntercepted(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := httpapi.WithStatic(api)

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Errorf("/events must reach the API handler, got %d", rec.Code)
	}
}
