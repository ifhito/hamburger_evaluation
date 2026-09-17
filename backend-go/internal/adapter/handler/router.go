package handler

import "net/http"

// NewRouter builds the HTTP handler tree: stdlib Go 1.22 method-pattern
// mux wrapped in the global body-cap middleware. Unknown routes get 404
// and wrong methods 405, both in the JSON error shape.
func NewRouter(db Pinger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /up", handleHealth(db))
	mux.HandleFunc("/up", methodNotAllowed(http.MethodGet))

	// Catch-all for unmatched paths, replacing the stdlib plain-text 404.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})

	return limitBody(mux)
}

// methodNotAllowed serves 405 in the JSON error shape. Registered on the
// method-less pattern of each route, it catches the methods the specific
// method patterns do not.
func methodNotAllowed(allowed ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for _, m := range allowed {
			w.Header().Add("Allow", m)
		}
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
