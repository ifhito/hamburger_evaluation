package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// TestOAuthScopeWritesMark は、許可の画面の API と接続済みアプリの一覧が、範囲ごとに「書き込みを伴うか」の印(writes)を
// 返すこと(hamburger:write だけが true)を確かめる。
func TestOAuthScopeWritesMark(t *testing.T) {
	type scope struct {
		Name   string `json:"name"`
		Writes *bool  `json:"writes"`
	}
	marks := func(t *testing.T, scopes []scope) map[string]bool {
		t.Helper()
		out := map[string]bool{}
		for _, s := range scopes {
			if s.Writes == nil {
				t.Errorf("範囲 %s に writes がない", s.Name)
				continue
			}
			out[s.Name] = *s.Writes
		}
		return out
	}

	t.Run("読み取りと書き込みを求める要求は、書き込みの範囲にだけ印が付き、許可したあとの一覧でも同じである", func(t *testing.T) {
		k := newOAuthKit(t)
		_, challenge := pkce()
		q := authQuery(challenge, "scope", domain.OAuthScopeRead+" "+domain.OAuthScopeWrite)

		rec := k.describe(k.alice, q)
		var view struct {
			Scopes []scope `json:"scopes"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("describe = %d %s (%v)", rec.Code, rec.Body, err)
		}
		want := map[string]bool{domain.OAuthScopeRead: false, domain.OAuthScopeWrite: true}
		if got := marks(t, view.Scopes); !mapsEqual(got, want) {
			t.Errorf("許可の画面の writes = %v, want %v", got, want)
		}

		redirectTo(t, k.decide(k.alice, q, true))
		list := do(k.router, http.MethodGet, "/oauth/grants", "", k.bearer[k.alice])
		var grants []struct {
			Scopes []scope `json:"scopes"`
		}
		if err := json.Unmarshal(list.Body.Bytes(), &grants); err != nil || list.Code != http.StatusOK || len(grants) != 1 {
			t.Fatalf("grants = %d %s (%v)", list.Code, list.Body, err)
		}
		if got := marks(t, grants[0].Scopes); !mapsEqual(got, want) {
			t.Errorf("接続済みアプリの writes = %v, want %v", got, want)
		}
	})

	t.Run("読み取りだけの要求は、印が false になる", func(t *testing.T) {
		k := newOAuthKit(t)
		_, challenge := pkce()
		rec := k.describe(k.alice, authQuery(challenge))
		var view struct {
			Scopes []scope `json:"scopes"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("describe = %d %s (%v)", rec.Code, rec.Body, err)
		}
		if got := marks(t, view.Scopes); !mapsEqual(got, map[string]bool{domain.OAuthScopeRead: false}) {
			t.Errorf("writes = %v, want read だけで false", got)
		}
	})
}

func mapsEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if other, ok := b[key]; !ok || other != value {
			return false
		}
	}
	return true
}
