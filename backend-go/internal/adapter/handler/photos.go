package handler

import (
	"net/http"
	"strings"
)

// PhotoFileServer serves the disk photo dir read-only: http.FileServer
// over http.Dir already confines requests to root (".." never escapes),
// and the wrapper turns directory requests into 404s instead of listings
// so stored keys are not enumerable. It expects to be mounted behind
// NewRouter's StripPrefix("/photos/") — the path it sees is relative to
// root. It lives here (not in cmd/api) so the router tests exercise
// exactly the wrapper production wires.
func PhotoFileServer(root string) http.Handler {
	files := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := r.URL.Path; p == "" || strings.HasSuffix(p, "/") {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
}
