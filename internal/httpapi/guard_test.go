package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/erikhoward/souschef/internal/httpapi"
)

const guardPort = 8420

// guarded wraps a handler that records whether it was ever reached, so a test
// can tell "rejected with 403" apart from "reached the handler, which
// happened to 403".
func guarded() (http.Handler, *bool) {
	reached := new(bool)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
	})
	return httpapi.LocalOnly(inner, guardPort), reached
}

func TestLocalOnlyAllowsSameOriginWrite(t *testing.T) {
	h, reached := guarded()

	req := httptest.NewRequest(http.MethodPost, "/api/ideas", strings.NewReader(`{}`))
	req.Host = "127.0.0.1:8420"
	req.Header.Set("Origin", "http://127.0.0.1:8420")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !*reached {
		t.Fatalf("same-origin POST was blocked: status=%d reached=%v body=%s",
			rec.Code, *reached, rec.Body.String())
	}
}

// The exact shape the reviewer verified creating an idea: a CORS simple
// request, so there is no preflight to stop it, from a page the owner merely
// visited. Every such write also fires a paid Claude call.
func TestLocalOnlyRejectsCrossOriginWrite(t *testing.T) {
	for _, tc := range []struct {
		name    string
		headers map[string]string
	}{
		{"origin only", map[string]string{
			"Origin":       "https://evil.example.com",
			"Content-Type": "text/plain;charset=UTF-8",
		}},
		{"browser-reported cross-site", map[string]string{
			"Origin":         "https://evil.example.com",
			"Sec-Fetch-Site": "cross-site",
			"Content-Type":   "text/plain;charset=UTF-8",
		}},
		{"same-site but not same-origin", map[string]string{
			"Origin":         "http://evil.localhost:9999",
			"Sec-Fetch-Site": "same-site",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, reached := guarded()

			req := httptest.NewRequest(http.MethodPost, "/api/ideas",
				strings.NewReader(`{"raw_text":"pwned","source":"web"}`))
			req.Host = "127.0.0.1:8420"
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", rec.Code)
			}
			if *reached {
				t.Error("the request reached the API handler — the write was not prevented")
			}
		})
	}
}

// Unvalidated Host is what makes DNS rebinding work: the browser believes it
// is talking to evil.example.com, so the request is same-origin from its point
// of view and the whole backlog plus the /events stream becomes readable.
func TestLocalOnlyRejectsForeignHost(t *testing.T) {
	for _, host := range []string{
		"evil.example.com",
		"evil.example.com:8420",
		"127.0.0.1:9999", // right host, wrong port: not this process
		"souschef.internal:8420",
	} {
		t.Run(host, func(t *testing.T) {
			h, reached := guarded()

			req := httptest.NewRequest(http.MethodGet, "/api/ideas", nil)
			req.Host = host

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", rec.Code)
			}
			if *reached {
				t.Error("the request reached the API handler — the backlog was readable")
			}
		})
	}
}

// Ordinary navigation, a hard reload, and Telegram's [Open] deep link all
// arrive as a GET with no Origin. None of them may be blocked.
func TestLocalOnlyAllowsPlainGet(t *testing.T) {
	for _, host := range []string{"127.0.0.1:8420", "localhost:8420", "[::1]:8420"} {
		t.Run(host, func(t *testing.T) {
			h, reached := guarded()

			req := httptest.NewRequest(http.MethodGet, "/api/ideas", nil)
			req.Host = host

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK || !*reached {
				t.Fatalf("plain GET was blocked: status=%d reached=%v", rec.Code, *reached)
			}
		})
	}
}

func TestLocalOnlyAllowsEventStreamSameOrigin(t *testing.T) {
	h, reached := guarded()

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	req.Host = "127.0.0.1:8420"
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Sec-Fetch-Site", "same-origin")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !*reached {
		t.Fatalf("SSE stream was blocked: status=%d reached=%v", rec.Code, *reached)
	}
}

// `make dev` puts Vite on :5173 proxying to :8420 with changeOrigin, so the
// forwarded write carries this server's Host but the dev server's Origin. The
// browser's own Sec-Fetch-Site verdict is what keeps it working — if this
// test fails, `make dev` is broken.
func TestLocalOnlyAllowsViteDevProxyWrite(t *testing.T) {
	h, reached := guarded()

	req := httptest.NewRequest(http.MethodPost, "/api/ideas", strings.NewReader(`{}`))
	req.Host = "127.0.0.1:8420" // rewritten by the proxy's changeOrigin
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Sec-Fetch-Site", "same-origin") // the browser's verdict
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !*reached {
		t.Fatalf("the Vite dev proxy write was blocked: status=%d reached=%v", rec.Code, *reached)
	}
}

// A local CLI client sends neither header. It is inside the trust boundary the
// no-auth decision already accepts, and curl is how the API is debugged.
func TestLocalOnlyAllowsHeaderlessLocalClient(t *testing.T) {
	h, reached := guarded()

	req := httptest.NewRequest(http.MethodPost, "/api/ideas", strings.NewReader(`{}`))
	req.Host = "127.0.0.1:8420"

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !*reached {
		t.Fatalf("a local curl write was blocked: status=%d reached=%v", rec.Code, *reached)
	}
}

// The Host check must apply to the SPA shell too, not only /api — otherwise a
// rebound page still gets the app to run against a same-origin API.
func TestLocalOnlyGuardsStaticRoutesToo(t *testing.T) {
	h, reached := guarded()

	req := httptest.NewRequest(http.MethodGet, "/ideas/0191f0c2-1234-7890-abcd-ef0123456789", nil)
	req.Host = "evil.example.com"

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden || *reached {
		t.Fatalf("a rebound Host reached the SPA shell: status=%d reached=%v", rec.Code, *reached)
	}
}
