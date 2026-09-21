package handler_test

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// TestOAuthScopeWritesMark は、許可の画面の API と接続済みアプリの一覧が、範囲ごとに「書き込みを伴うか」の印(writes)を
// 返すこと(hamburger:write だけが true)を確かめる。
func TestOAuthScopeWritesMark(t *testing.T) {
	marks := func(t *testing.T, scopes []grantScopeJSON) map[string]bool {
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
	describe := func(t *testing.T, k *oauthKit, query string) []grantScopeJSON {
		t.Helper()
		rec := k.describe(k.alice, query)
		var view struct {
			Scopes []grantScopeJSON `json:"scopes"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("describe = %d %s (%v)", rec.Code, rec.Body, err)
		}
		return view.Scopes
	}

	t.Run("読み取りと書き込みを求める要求は、書き込みの範囲にだけ印が付き、許可したあとの一覧でも同じである", func(t *testing.T) {
		k := newOAuthKit(t)
		_, challenge := pkce()
		q := authQuery(challenge, "scope", domain.OAuthScopeRead+" "+domain.OAuthScopeWrite)
		want := map[string]bool{domain.OAuthScopeRead: false, domain.OAuthScopeWrite: true}

		if got := marks(t, describe(t, k, q)); !maps.Equal(got, want) {
			t.Errorf("許可の画面の writes = %v, want %v", got, want)
		}

		redirectTo(t, k.decide(k.alice, q, true))
		grants := k.grants(t, k.alice)
		if len(grants) != 1 {
			t.Fatalf("grants = %+v, want 1 件", grants)
		}
		if got := marks(t, grants[0].Scopes); !maps.Equal(got, want) {
			t.Errorf("接続済みアプリの writes = %v, want %v", got, want)
		}
	})

	t.Run("読み取りだけの要求は、印が false になる", func(t *testing.T) {
		k := newOAuthKit(t)
		_, challenge := pkce()
		if got := marks(t, describe(t, k, authQuery(challenge))); !maps.Equal(got, map[string]bool{domain.OAuthScopeRead: false}) {
			t.Errorf("writes = %v, want read だけで false", got)
		}
	})

	t.Run("いまの定義にない範囲の名前が許可に残っていても、writes は false で返り、書き込みとは扱われない", func(t *testing.T) {
		k := newOAuthKit(t)
		if _, err := k.pool.Exec(context.Background(), `INSERT INTO oauth_grants (user_id, client_id, client_name, scopes)
			VALUES ($1, 'legacy-app', 'Legacy App', ARRAY['hamburger:read', 'legacy:removed'])`, k.alice); err != nil {
			t.Fatal(err)
		}
		grants := k.grants(t, k.alice)
		if len(grants) != 1 {
			t.Fatalf("grants = %+v, want 1 件", grants)
		}
		want := map[string]bool{domain.OAuthScopeRead: false, "legacy:removed": false}
		if got := marks(t, grants[0].Scopes); !maps.Equal(got, want) {
			t.Errorf("writes = %v, want %v", got, want)
		}
	})
}
