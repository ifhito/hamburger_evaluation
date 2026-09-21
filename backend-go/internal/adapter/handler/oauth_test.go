package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/handler"
)

// recordingOAuth は、どの窓口が呼ばれたかを記録する、テスト用の OAuth の窓口である。
type recordingOAuth struct{ called []string }

func (r *recordingOAuth) mark(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		r.called = append(r.called, name)
		w.WriteHeader(http.StatusNoContent)
	}
}
func (r *recordingOAuth) HandleMetadata(w http.ResponseWriter, req *http.Request) {
	r.mark("metadata")(w, req)
}
func (r *recordingOAuth) HandleAuthorize(w http.ResponseWriter, req *http.Request) {
	r.mark("authorize")(w, req)
}
func (r *recordingOAuth) HandleToken(w http.ResponseWriter, req *http.Request) {
	r.mark("token")(w, req)
}
func (r *recordingOAuth) HandleRevoke(w http.ResponseWriter, req *http.Request) {
	r.mark("revoke")(w, req)
}

func TestOAuthRoutes(t *testing.T) {
	newRouter := func(oauth handler.OAuthEndpoints) http.Handler {
		if oauth == nil {
			return handler.NewRouter(okPinger, nil, unusedSignups(), nil, nil, nil, nil, nil, nil)
		}
		return handler.NewRouter(okPinger, nil, unusedSignups(), nil, nil, nil, nil, &handler.OAuth{Endpoints: oauth}, nil)
	}
	do := func(h http.Handler, method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader("")))
		return rec
	}

	t.Run("OAuth の窓口を渡すと、認可サーバーの情報・認可・トークン・取り消しが、それぞれの URL と HTTP メソッドで届く", func(t *testing.T) {
		oauth := &recordingOAuth{}
		router := newRouter(oauth)
		for _, tt := range []struct{ method, path, want string }{
			{http.MethodGet, "/.well-known/oauth-authorization-server", "metadata"},
			{http.MethodGet, "/oauth/authorize", "authorize"},
			{http.MethodPost, "/oauth/token", "token"},
			{http.MethodPost, "/oauth/revoke", "revoke"},
		} {
			if rec := do(router, tt.method, tt.path); rec.Code != http.StatusNoContent {
				t.Errorf("%s %s = %d, want the endpoint to answer", tt.method, tt.path, rec.Code)
			}
			if got := oauth.called[len(oauth.called)-1]; got != tt.want {
				t.Errorf("%s %s reached %q, want %q", tt.method, tt.path, got, tt.want)
			}
		}
	})

	t.Run("認可の URL に POST、トークンの URL に GET を送ると、許可するメソッドを示して 405 になる", func(t *testing.T) {
		router := newRouter(&recordingOAuth{})
		for _, tt := range []struct{ method, path, allow string }{
			{http.MethodPost, "/oauth/authorize", "GET"},
			{http.MethodGet, "/oauth/token", "POST"},
			{http.MethodGet, "/oauth/revoke", "POST"},
			{http.MethodPost, "/.well-known/oauth-authorization-server", "GET"},
		} {
			rec := do(router, tt.method, tt.path)
			if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != tt.allow {
				t.Errorf("%s %s = %d Allow=%q, want 405 Allow=%q", tt.method, tt.path, rec.Code, rec.Header().Get("Allow"), tt.allow)
			}
		}
	})

	t.Run("OAuth の窓口を渡さない(認可サーバーが無効な)ときは、これらの URL は登録されず、404 になる", func(t *testing.T) {
		router := newRouter(nil)
		for _, path := range []string{"/.well-known/oauth-authorization-server", "/oauth/authorize", "/oauth/token", "/oauth/revoke"} {
			if rec := do(router, http.MethodGet, path); rec.Code != http.StatusNotFound {
				t.Errorf("GET %s = %d, want 404", path, rec.Code)
			}
		}
	})
}
