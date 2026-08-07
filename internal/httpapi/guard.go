package httpapi

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
)

// localHostnames are the names that legitimately resolve to this process.
var localHostnames = map[string]bool{
	"127.0.0.1": true,
	"localhost": true,
	"::1":       true,
}

// LocalOnly rejects requests that a browser on another site could have caused.
//
// The spec's decision that this app needs no authentication is correct and is
// not relitigated here: it is localhost, single user. But the accepted risk
// was "anyone with local shell access can read it", not "any website the
// owner visits can write to it". Two holes made the second true:
//
//   - A cross-origin POST with Content-Type: text/plain is a CORS *simple
//     request*, so there is no preflight to block it. Any page could
//     silently create ideas, and every write fires a paid Claude call.
//     Verified live before this fix: POST /api/ideas with
//     Origin: https://evil.example.com returned 201 and created the idea.
//   - Host went unvalidated, so DNS rebinding let a visited page read the
//     entire backlog and the /events stream. Verified: GET /api/ideas with
//     Host: evil.example.com returned 200.
//
// The two checks below close exactly those, and nothing more:
//
//  1. Host must name this process — 127.0.0.1, localhost, or [::1] on the
//     configured port. This is what defeats DNS rebinding, and it applies to
//     every method, including reads.
//  2. A request that can change state must prove it came from our own page.
//
// Safe methods (GET/HEAD) skip check 2 deliberately. A cross-origin read is
// already useless to an attacker: we send no Access-Control-Allow-Origin, so
// the browser refuses to hand the response body to the calling page. Requiring
// an Origin on GET would break ordinary navigation and Telegram's [Open] deep
// links, which arrive with no Origin at all.
func LocalOnly(next http.Handler, port int) http.Handler {
	expectedPort := strconv.Itoa(port)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLocalHost(r.Host, expectedPort) {
			http.Error(w,
				"Forbidden: Sous Chef only answers to localhost. "+
					"This request arrived with Host "+r.Host+".",
				http.StatusForbidden)
			return
		}

		if !isSafeMethod(r.Method) && !isSameOriginRequest(r, expectedPort) {
			http.Error(w,
				"Forbidden: cross-origin write rejected. Sous Chef has no "+
					"authentication because it is local and single-user, which "+
					"means another site must never be able to write to it.",
				http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

// isLocalHost reports whether a Host header names this process.
func isLocalHost(host, expectedPort string) bool {
	hostname, hostPort, err := net.SplitHostPort(host)
	if err != nil {
		// No port in the header. Browsers omit it only for the scheme's
		// default port, so this can be legitimate when we are bound to 80.
		hostname, hostPort = host, "80"
	}
	return localHostnames[hostname] && hostPort == expectedPort
}

// isSameOriginRequest decides whether a state-changing request came from a
// page this server served.
//
// Sec-Fetch-Site is preferred over Origin because it is both stricter and
// more accurate. It is a forbidden header name, so page JavaScript cannot
// set or spoof it — only the browser writes it, and it describes the
// relationship the browser itself computed. It is also what makes `make dev`
// work: under the Vite proxy the browser talks to localhost:5173 and Vite
// forwards to 127.0.0.1:8420 with changeOrigin, so the forwarded request
// carries Host: 127.0.0.1:8420 (passing check 1) but Origin:
// http://localhost:5173, which is not literally same-origin with this
// server. The browser's own verdict on that request is "same-origin", and
// that is the truthful answer to the question being asked.
//
// A non-browser client could of course forge either header — but a
// non-browser client on this machine is the risk the no-auth decision
// already accepts. CSRF is specifically about requests a browser makes on
// the owner's behalf, and a browser cannot lie here.
func isSameOriginRequest(r *http.Request, expectedPort string) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		// "none" is a user-initiated request with no initiator page at all
		// (typed URL, bookmark), which no other site can cause.
		return site == "same-origin" || site == "none"
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		// No Origin and no Sec-Fetch-Site: not a browser-initiated
		// cross-site request. Every browser in use sends at least one of
		// them on a write, so this is a local CLI client (curl, a script),
		// which is inside the accepted trust boundary.
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return isLocalHost(parsed.Host, expectedPort)
}
