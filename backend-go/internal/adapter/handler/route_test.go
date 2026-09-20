package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRegisterRoutesAllow asserts the derived 405 fallback lists exactly
// the declared methods, sorted and comma-joined in a single Allow header.
func TestRegisterRoutesAllow(t *testing.T) {
	ok := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }
	mux := http.NewServeMux()
	registerRoutes(mux, []route{
		{path: "/multi", methods: map[string]http.HandlerFunc{
			http.MethodPost: ok,
			http.MethodGet:  ok,
		}},
	})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/multi", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if allow := rec.Header().Get("Allow"); allow != "GET, POST" {
		t.Errorf("Allow = %q, want %q", allow, "GET, POST")
	}
	if got, want := rec.Header().Values("Allow"), 1; len(got) != want {
		t.Errorf("Allow header count = %d, want %d", len(got), want)
	}
}
