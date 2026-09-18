package handler

import (
	"net/http"
	"sort"
	"strings"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// route declares one path's handlers by HTTP method, plus optional
// middleware applied to every method of the path. Routes are data so the
// 405 fallback (with its Allow header) is derived instead of hand-rolled.
type route struct {
	path       string
	methods    map[string]http.HandlerFunc
	middleware func(http.Handler) http.Handler
}

// NewRouter builds the HTTP handler tree: stdlib Go 1.22 method-pattern
// mux wrapped in the global body-cap middleware. Unknown routes get 404
// and wrong methods 405, both in the JSON error shape.
func NewRouter(db Pinger, auth *usecase.Auth) http.Handler {
	mux := http.NewServeMux()
	registerRoutes(mux, []route{
		{path: "/up", methods: map[string]http.HandlerFunc{http.MethodGet: handleHealth(db)}},
		{path: "/signup", methods: map[string]http.HandlerFunc{http.MethodPost: handleSignup(auth)}},
		{path: "/login", methods: map[string]http.HandlerFunc{http.MethodPost: handleLogin(auth)}},
		{path: "/logout", methods: map[string]http.HandlerFunc{http.MethodPost: handleLogout}, middleware: RequireAuth(auth)},
	})
	return limitBody(mux)
}

// registerRoutes registers each route's "METHOD path" patterns, a per-path
// method-less fallback answering 405 with a comma-joined sorted Allow
// header of exactly the declared methods, and the catch-all JSON 404.
func registerRoutes(mux *http.ServeMux, routes []route) {
	for _, rt := range routes {
		allowed := make([]string, 0, len(rt.methods))
		for method, h := range rt.methods {
			allowed = append(allowed, method)
			var handler http.Handler = h
			if rt.middleware != nil {
				handler = rt.middleware(handler)
			}
			mux.Handle(method+" "+rt.path, handler)
		}
		sort.Strings(allowed)
		allow := strings.Join(allowed, ", ")
		mux.HandleFunc(rt.path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Allow", allow)
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		})
	}

	// Catch-all for unmatched paths, replacing the stdlib plain-text 404.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
}
