package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRegisterRoutesAllow は、導出された 405 の fallback が、宣言された
// メソッドをちょうど列挙し、ソートしてカンマで連結した 1 つの Allow ヘッダー
// になっていることを検証する。
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

// TestRegisterRoutesPerMethodMiddleware は、1 つのパスが、ServeMux の
// pattern の panic を起こさずに、メソッドごとに異なる middleware を登録できる
// ことを検証する：各メソッドはそれ自身の methodMiddleware のエントリで
// ラップされ、エントリのないメソッドはパスレベルの middleware に fallback し、
// 導出された 405 は宣言されたすべてのメソッドを引き続き列挙する。
func TestRegisterRoutesPerMethodMiddleware(t *testing.T) {
	tag := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Middleware", name)
				next.ServeHTTP(w, r)
			})
		}
	}
	ok := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }
	mux := http.NewServeMux()
	registerRoutes(mux, []route{
		{
			path: "/mixed",
			methods: map[string]http.HandlerFunc{
				http.MethodGet:  ok,
				http.MethodPost: ok,
			},
			methodMiddleware: map[string]func(http.Handler) http.Handler{
				http.MethodGet:  tag("get"),
				http.MethodPost: tag("post"),
			},
		},
		{
			path: "/override",
			methods: map[string]http.HandlerFunc{
				http.MethodGet:  ok,
				http.MethodPost: ok,
			},
			middleware: tag("path"),
			methodMiddleware: map[string]func(http.Handler) http.Handler{
				http.MethodPost: tag("post"),
			},
		},
	})

	tests := []struct {
		method string
		path   string
		want   string
	}{
		{http.MethodGet, "/mixed", "get"},
		{http.MethodPost, "/mixed", "post"},
		{http.MethodGet, "/override", "path"},
		{http.MethodPost, "/override", "post"},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s %s: status = %d, want %d", tt.method, tt.path, rec.Code, http.StatusNoContent)
		}
		if got := rec.Header().Get("X-Middleware"); got != tt.want {
			t.Errorf("%s %s: middleware = %q, want %q", tt.method, tt.path, got, tt.want)
		}
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/mixed", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if allow := rec.Header().Get("Allow"); allow != "GET, POST" {
		t.Errorf("Allow = %q, want %q", allow, "GET, POST")
	}
}
