package httpapi

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/erikhoward/souschef/web"
)

// WithStatic wraps the API handler with the embedded single-page app.
//
// Anything under /api or /events goes straight to the API. Everything else
// tries the embedded files, falling back to index.html so client-side routes
// like /ideas/<id> survive a hard reload — which is what Telegram's [Open]
// deep links rely on.
//
// The embedded files live in the web package (web/embed.go), not here:
// Go's embed directive cannot reference a path outside the package
// directory it is declared in, so //go:embed all:../../web/dist from this
// package fails at compile time with "pattern ...: invalid pattern syntax".
func WithStatic(api http.Handler) http.Handler {
	sub, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		// Only possible if the embed directive and this path disagree.
		panic("httpapi: embedded dist not found: " + err.Error())
	}
	files := http.FS(sub)
	fileServer := http.FileServer(files)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/events" {
			api.ServeHTTP(w, r)
			return
		}

		if f, err := files.Open(r.URL.Path); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		index, err := sub.Open("index.html")
		if err != nil {
			http.Error(w, "UI not built — run `make build`", http.StatusNotFound)
			return
		}
		defer index.Close()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		buf := make([]byte, 0, 4096)
		tmp := make([]byte, 4096)
		for {
			n, err := index.Read(tmp)
			buf = append(buf, tmp[:n]...)
			if err != nil {
				break
			}
		}
		w.Write(buf)
	})
}
