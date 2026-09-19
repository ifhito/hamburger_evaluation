package handler

import (
	"net/http"
	"sort"
	"strings"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// route declares one path's handlers by HTTP method, plus optional
// middleware applied to every method of the path. methodMiddleware
// overrides that path-level middleware for individual methods, so one
// path can e.g. serve GET behind OptionalAuth and POST behind
// RequireAuth without duplicate ServeMux patterns. Routes are data so
// the 405 fallback (with its Allow header) is derived instead of
// hand-rolled.
type route struct {
	path             string
	methods          map[string]http.HandlerFunc
	middleware       func(http.Handler) http.Handler
	methodMiddleware map[string]func(http.Handler) http.Handler
}

// NewRouter builds the HTTP handler tree: stdlib Go 1.22 method-pattern
// mux wrapped in the global body-cap middleware. Unknown routes get 404
// and wrong methods 405, both in the JSON error shape. photoFiles, when
// non-nil (disk photo storage, S10), serves review photos under GET
// /photos/ — registered on the mux directly so the JSON 404 catch-all
// does not swallow it; in s3 mode it is nil and photo URLs point at the
// bucket's public domain instead.
func NewRouter(db Pinger, auth *usecase.Auth, shops *usecase.Shops, reviews *usecase.Reviews, users *usecase.Users, photoFiles http.Handler) http.Handler {
	mux := http.NewServeMux()
	if photoFiles != nil {
		mux.Handle("GET /photos/", http.StripPrefix("/photos/", photoFiles))
	}
	registerRoutes(mux, []route{
		{path: "/up", methods: map[string]http.HandlerFunc{http.MethodGet: handleHealth(db)}},
		{path: "/signup", methods: map[string]http.HandlerFunc{http.MethodPost: handleSignup(auth)}},
		{path: "/login", methods: map[string]http.HandlerFunc{http.MethodPost: handleLogin(auth)}},
		{path: "/logout", methods: map[string]http.HandlerFunc{http.MethodPost: handleLogout}, middleware: RequireAuth(auth)},
		// GET stays anonymous-friendly (OptionalAuth); only submitting a
		// shop requires a login, hence the per-method override.
		{
			path: "/shops",
			methods: map[string]http.HandlerFunc{
				http.MethodGet:  handleListShops(shops),
				http.MethodPost: handleCreateShop(shops),
			},
			middleware:       OptionalAuth(auth),
			methodMiddleware: map[string]func(http.Handler) http.Handler{http.MethodPost: RequireAuth(auth)},
		},
		{path: "/shops/{id}", methods: map[string]http.HandlerFunc{http.MethodGet: handleGetShop(shops)}, middleware: OptionalAuth(auth)},
		// The review feed and detail stay anonymous-friendly; only the
		// writes (POST/PUT/DELETE) require a login.
		{
			path: "/reviews",
			methods: map[string]http.HandlerFunc{
				http.MethodGet:  handleListReviews(reviews),
				http.MethodPost: handleCreateReview(reviews),
			},
			middleware:       OptionalAuth(auth),
			methodMiddleware: map[string]func(http.Handler) http.Handler{http.MethodPost: RequireAuth(auth)},
		},
		{
			path: "/reviews/{id}",
			methods: map[string]http.HandlerFunc{
				http.MethodGet:    handleGetReview(reviews),
				http.MethodPut:    handleUpdateReview(reviews),
				http.MethodDelete: handleDeleteReview(reviews),
			},
			middleware: OptionalAuth(auth),
			methodMiddleware: map[string]func(http.Handler) http.Handler{
				http.MethodPut:    RequireAuth(auth),
				http.MethodDelete: RequireAuth(auth),
			},
		},
		// The user index is public (Rails parity, no auth at all); editing
		// and deleting an account require a login, and the self-only rule
		// itself lives in the usecase (ErrForbidden).
		{path: "/users", methods: map[string]http.HandlerFunc{http.MethodGet: handleListUsers(users)}},
		{
			path: "/users/{id}",
			methods: map[string]http.HandlerFunc{
				http.MethodPut:    handleUpdateUser(users),
				http.MethodDelete: handleDeleteUser(users),
			},
			middleware: RequireAuth(auth),
		},
		// Moderation endpoints: RequireAuth only authenticates; the
		// admin decision itself lives in the usecase (ErrForbidden).
		{path: "/admin/shops", methods: map[string]http.HandlerFunc{http.MethodGet: handleAdminListShops(shops)}, middleware: RequireAuth(auth)},
		{path: "/admin/shops/{id}", methods: map[string]http.HandlerFunc{http.MethodPut: handleAdminUpdateShop(shops)}, middleware: RequireAuth(auth)},
		{path: "/admin/shops/{id}/approve", methods: map[string]http.HandlerFunc{http.MethodPost: handleApproveShop(shops)}, middleware: RequireAuth(auth)},
		{path: "/admin/shops/{id}/reject", methods: map[string]http.HandlerFunc{http.MethodPost: handleRejectShop(shops)}, middleware: RequireAuth(auth)},
	})
	return limitBody(mux)
}

// registerRoutes registers each route's "METHOD path" patterns (each
// wrapped in its per-method middleware when declared, else the path-level
// middleware), a per-path method-less fallback answering 405 with a
// comma-joined sorted Allow header of exactly the declared methods, and
// the catch-all JSON 404.
func registerRoutes(mux *http.ServeMux, routes []route) {
	for _, rt := range routes {
		allowed := make([]string, 0, len(rt.methods))
		for method, h := range rt.methods {
			allowed = append(allowed, method)
			var handler http.Handler = h
			mw := rt.middleware
			if perMethod, ok := rt.methodMiddleware[method]; ok {
				mw = perMethod
			}
			if mw != nil {
				handler = mw(handler)
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
