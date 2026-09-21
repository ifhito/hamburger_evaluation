package oauthserver_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/oauthserver"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

func TestAuthorizationCodeFlow(t *testing.T) {
	t.Run("許可済みの利用者の認可の要求からトークンまで通すと、その利用者・範囲・宛先のトークンが発行される", func(t *testing.T) {
		r := newRig(t)
		grantID := r.approve(staticClientID, "Dev App", domain.OAuthScopeRead)
		verifier, challenge := pkcePair()

		redirect := r.issue(authParams(staticClientID, staticRedirect, challenge), grantID)
		if got := redirect.Scheme + "://" + redirect.Host + redirect.Path; got != staticRedirect {
			t.Errorf("戻り先 = %s, want %s", got, staticRedirect)
		}
		if redirect.Query().Get("state") != "state-1234567890" {
			t.Errorf("state = %q, want the requested state", redirect.Query().Get("state"))
		}
		if redirect.Query().Get("iss") != testIssuer {
			t.Errorf("iss = %q, want %q (発行者を付けて、別の認可サーバーの結果と取り違えないようにする)", redirect.Query().Get("iss"), testIssuer)
		}
		code := redirect.Query().Get("code")
		if code == "" {
			t.Fatal("認可コードがない")
		}

		resp := r.exchange(staticClientID, staticRedirect, code, verifier)
		if resp.Status != http.StatusOK {
			t.Fatalf("交換 status = %d, body = %v", resp.Status, resp.Body)
		}
		if resp.str("token_type") == "" || !strings.EqualFold(resp.str("token_type"), "bearer") {
			t.Errorf("token_type = %q, want bearer", resp.str("token_type"))
		}
		if resp.str("scope") != domain.OAuthScopeRead {
			t.Errorf("scope = %q, want %q", resp.str("scope"), domain.OAuthScopeRead)
		}
		if exp, _ := resp.Body["expires_in"].(float64); exp <= 0 || exp > domain.OAuthAccessTokenTTL.Seconds() {
			t.Errorf("expires_in = %v, want within (0, %v]", exp, domain.OAuthAccessTokenTTL.Seconds())
		}
		if resp.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("Cache-Control = %q, want no-store", resp.Header.Get("Cache-Control"))
		}
		if resp.str("refresh_token") == "" {
			t.Error("更新トークンが発行されなかった")
		}

		token, err := r.srv.IntrospectAccessToken(r.ctx, resp.str("access_token"))
		if err != nil {
			t.Fatalf("IntrospectAccessToken: %v", err)
		}
		if token.UserID != r.userID || token.ClientID != staticClientID {
			t.Errorf("token = %+v, want user %s client %s", token, r.userID, staticClientID)
		}
		if len(token.Scopes) != 1 || token.Scopes[0] != domain.OAuthScopeRead {
			t.Errorf("scopes = %v, want [read]", token.Scopes)
		}
		if len(token.Audience) != 1 || token.Audience[0] != testResource {
			t.Errorf("audience = %v, want [%s]", token.Audience, testResource)
		}
		if remaining := time.Until(token.ExpiresAt); remaining <= 0 || remaining > domain.OAuthAccessTokenTTL+time.Second {
			t.Errorf("有効期限まで %v、want within (0, %v] (秒に丸めるので、1 秒までの余裕を許す)", remaining, domain.OAuthAccessTokenTTL)
		}
	})

	t.Run("発行したトークンと認可コードの文字列そのものは、どこにも保存されず、署名だけが保存される", func(t *testing.T) {
		r := newRig(t)
		grantID := r.approve(staticClientID, "Dev App", domain.OAuthScopeRead)
		verifier, challenge := pkcePair()
		code := r.issueCode(authParams(staticClientID, staticRedirect, challenge), grantID)
		resp := r.exchange(staticClientID, staticRedirect, code, verifier)
		for name, secret := range map[string]string{"認可コード": code, "アクセストークン": resp.str("access_token"), "更新トークン": resp.str("refresh_token")} {
			if secret == "" {
				t.Fatalf("%s が空", name)
			}
			if n := r.count(`SELECT count(*) FROM oauth_token_sessions WHERE position($1 in row_to_json(oauth_token_sessions)::text) > 0`, secret); n != 0 {
				t.Errorf("%s の文字列が保存されている (%d 行)", name, n)
			}
		}
		if n := r.count(`SELECT count(*) FROM oauth_token_sessions WHERE kind = 'access_token'`); n != 1 {
			t.Errorf("アクセストークンの記録 = %d 件, want 1", n)
		}
	})

	t.Run("範囲を指定しない要求は、最も小さい範囲(読み取りだけ)で許可を求めたものとして扱われる", func(t *testing.T) {
		r := newRig(t)
		grantID := r.approve(staticClientID, "Dev App", domain.OAuthScopeRead)
		verifier, challenge := pkcePair()
		code := r.issueCode(authParams(staticClientID, staticRedirect, challenge, "scope", ""), grantID)
		resp := r.exchange(staticClientID, staticRedirect, code, verifier)
		if resp.str("scope") != domain.OAuthScopeRead {
			t.Errorf("scope = %q, want %q", resp.str("scope"), domain.OAuthScopeRead)
		}
	})

	t.Run("宛先(resource)を指定しない要求は、この認可サーバーの宛先のトークンになる", func(t *testing.T) {
		r := newRig(t)
		access, _, _ := func() (string, string, tokenResponse) {
			grantID := r.approve(staticClientID, "Dev App", domain.OAuthScopeRead)
			verifier, challenge := pkcePair()
			code := r.issueCode(authParams(staticClientID, staticRedirect, challenge, "resource", ""), grantID)
			resp := r.exchange(staticClientID, staticRedirect, code, verifier)
			return resp.str("access_token"), resp.str("refresh_token"), resp
		}()
		token, err := r.srv.IntrospectAccessToken(r.ctx, access)
		if err != nil || len(token.Audience) != 1 || token.Audience[0] != testResource {
			t.Errorf("token = %+v, err = %v, want audience [%s]", token, err, testResource)
		}
	})

	t.Run("読み取りと書き込みの両方を許可すると、両方の範囲を持つトークンになる", func(t *testing.T) {
		r := newRig(t)
		access, _, resp := r.tokens(domain.OAuthScopeRead, domain.OAuthScopeWrite)
		token, err := r.srv.IntrospectAccessToken(r.ctx, access)
		if err != nil {
			t.Fatal(err)
		}
		if err := token.Check(testResource, domain.OAuthScopeRead, domain.OAuthScopeWrite); err != nil {
			t.Errorf("Check = %v (scope %q)", err, resp.str("scope"))
		}
	})

	t.Run("ループバックの戻り先は、登録と同じでポート番号だけが違っても受け付ける", func(t *testing.T) {
		r := newRig(t)
		grantID := r.approve(staticClientID, "Dev App", domain.OAuthScopeRead)
		verifier, challenge := pkcePair()
		const requested = "http://127.0.0.1:53682/callback"
		code := r.issueCode(authParams(staticClientID, requested, challenge), grantID)
		if resp := r.exchange(staticClientID, requested, code, verifier); resp.Status != http.StatusOK {
			t.Errorf("交換 status = %d, body = %v", resp.Status, resp.Body)
		}
	})
}

func TestAuthorizeRequestErrors(t *testing.T) {
	_, challenge := pkcePair()

	// アプリへ戻せる不正は、エラーを付けてアプリの戻り先へ返す(認可コードは発行しない)。
	redirectable := []struct {
		name      string
		overrides []string
		wantError string
	}{
		{"PKCE の challenge がない要求は、アプリへエラーで戻す", []string{"code_challenge", "", "code_challenge_method", ""}, "invalid_request"},
		{"PKCE の方式が S256 ではなく plain の要求は、アプリへエラーで戻す", []string{"code_challenge_method", "plain"}, "invalid_request"},
		{"知らない範囲を求める要求は、アプリへエラーで戻す", []string{"scope", "admin"}, "invalid_scope"},
		{"認可コード以外の応答の形(token)を求める要求は、アプリへエラーで戻す", []string{"response_type", "token"}, ""},
		{"state がない要求は、アプリへエラーで戻す", []string{"state", ""}, "invalid_state"},
		{"別のサーバーの宛先を求める要求は、宛先の誤りとしてアプリへ戻す", []string{"resource", "https://other.example.com/mcp"}, "invalid_target"},
	}
	for _, tt := range redirectable {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t)
			grantID := r.approve(staticClientID, "Dev App", domain.OAuthScopeRead)
			params := authParams(staticClientID, staticRedirect, challenge, tt.overrides...)
			redirect := r.issue(params, grantID)
			q := redirect.Query()
			if q.Get("code") != "" {
				t.Error("不正な要求に認可コードが発行された")
			}
			if q.Get("error") == "" || (tt.wantError != "" && q.Get("error") != tt.wantError) {
				t.Errorf("error = %q, want %q (%s)", q.Get("error"), tt.wantError, redirect)
			}
			if redirect.Host != "127.0.0.1" {
				t.Errorf("戻り先 = %s, want the registered redirect", redirect)
			}
			if r.count(`SELECT count(*) FROM oauth_token_sessions`) != 0 {
				t.Error("不正な要求でトークンの記録が作られた")
			}
		})
	}

	// アプリへ戻せない不正は、戻り先が確かでないので、リダイレクトせずにエラーで返す。
	notRedirectable := []struct {
		name      string
		overrides []string
	}{
		{"登録されていない戻り先の要求は、リダイレクトせずにエラーで返す", []string{"redirect_uri", "https://evil.example.com/callback"}},
		{"ループバックでも、登録と経路が違う戻り先は、リダイレクトせずにエラーで返す", []string{"redirect_uri", "http://127.0.0.1:9999/other"}},
		{"登録されていないアプリの要求は、リダイレクトせずにエラーで返す", []string{"client_id", "unknown-app"}},
		{"アプリを指定しない要求は、リダイレクトせずにエラーで返す", []string{"client_id", ""}},
	}
	for _, tt := range notRedirectable {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t)
			grantID := r.approve(staticClientID, "Dev App", domain.OAuthScopeRead)
			params := authParams(staticClientID, staticRedirect, challenge, tt.overrides...)

			if _, err := r.srv.IssueAuthorizationCode(r.ctx, params, r.userID, grantID); !errors.Is(err, domain.ErrOAuthAuthorizeRequestInvalid) {
				t.Errorf("IssueAuthorizationCode err = %v, want ErrOAuthAuthorizeRequestInvalid", err)
			}
			if _, err := r.srv.DescribeAuthorizeRequest(r.ctx, params); !errors.Is(err, domain.ErrOAuthAuthorizeRequestInvalid) {
				t.Errorf("DescribeAuthorizeRequest err = %v, want ErrOAuthAuthorizeRequestInvalid", err)
			}

			rec := httptest.NewRecorder()
			r.srv.HandleAuthorize(rec, httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+params.Encode(), nil))
			if rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnauthorized {
				t.Errorf("HandleAuthorize status = %d, want 400 or 401", rec.Code)
			}
			if loc := rec.Header().Get("Location"); loc != "" {
				t.Errorf("HandleAuthorize が %q へリダイレクトした", loc)
			}
		})
	}

	t.Run("問題のない要求は、利用者に許可を尋ねる画面へ、同じ値のまま渡す", func(t *testing.T) {
		r := newRig(t)
		params := authParams(staticClientID, staticRedirect, challenge)
		rec := httptest.NewRecorder()
		r.srv.HandleAuthorize(rec, httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+params.Encode(), nil))
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303", rec.Code)
		}
		loc, err := url.Parse(rec.Header().Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		if got := loc.Scheme + "://" + loc.Host + loc.Path; got != testConsentURL {
			t.Errorf("渡し先 = %s, want %s", got, testConsentURL)
		}
		for _, k := range []string{"client_id", "redirect_uri", "state", "code_challenge", "code_challenge_method", "scope", "resource"} {
			if loc.Query().Get(k) != params.Get(k) {
				t.Errorf("%s = %q, want %q", k, loc.Query().Get(k), params.Get(k))
			}
		}
	})

	t.Run("アプリへ戻せる不正な要求を認可の URL に送ると、エラーを付けてアプリの戻り先へ返す", func(t *testing.T) {
		r := newRig(t)
		params := authParams(staticClientID, staticRedirect, challenge, "code_challenge", "", "code_challenge_method", "")
		rec := httptest.NewRecorder()
		r.srv.HandleAuthorize(rec, httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+params.Encode(), nil))
		loc, err := url.Parse(rec.Header().Get("Location"))
		if err != nil || loc.Host != "127.0.0.1" || loc.Query().Get("error") != "invalid_request" {
			t.Errorf("Location = %q, err = %v", rec.Header().Get("Location"), err)
		}
		if loc != nil && loc.Query().Get("iss") != testIssuer {
			t.Errorf("iss = %q, want %q", loc.Query().Get("iss"), testIssuer)
		}
	})

	t.Run("利用者が拒否すると、access_denied と state と発行者を付けてアプリの戻り先へ返し、トークンは作られない", func(t *testing.T) {
		r := newRig(t)
		loc, err := r.srv.DenyAuthorization(r.ctx, authParams(staticClientID, staticRedirect, challenge))
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(loc)
		q := u.Query()
		if q.Get("error") != "access_denied" || q.Get("state") != "state-1234567890" || q.Get("iss") != testIssuer || q.Get("code") != "" {
			t.Errorf("redirect = %s", loc)
		}
		if r.count(`SELECT count(*) FROM oauth_token_sessions`) != 0 {
			t.Error("拒否したのにトークンの記録が作られた")
		}
	})

	t.Run("戻り先が確かでない要求を拒否した場合は、アプリへ戻さずにエラーで返す", func(t *testing.T) {
		r := newRig(t)
		_, err := r.srv.DenyAuthorization(r.ctx, authParams(staticClientID, "https://evil.example.com/callback", challenge))
		if !errors.Is(err, domain.ErrOAuthAuthorizeRequestInvalid) {
			t.Errorf("err = %v, want ErrOAuthAuthorizeRequestInvalid", err)
		}
	})

	t.Run("利用者に見せる内容には、アプリの名前と、求められた範囲が入る", func(t *testing.T) {
		r := newRig(t)
		view, err := r.srv.DescribeAuthorizeRequest(r.ctx, authParams(staticClientID, staticRedirect, challenge, "scope", "hamburger:read hamburger:write"))
		if err != nil {
			t.Fatal(err)
		}
		want := usecase.AuthorizeRequestView{ClientID: staticClientID, ClientName: "Dev App", Scopes: []string{domain.OAuthScopeRead, domain.OAuthScopeWrite}}
		if view.ClientID != want.ClientID || view.ClientName != want.ClientName || strings.Join(view.Scopes, " ") != strings.Join(want.Scopes, " ") {
			t.Errorf("view = %+v, want %+v", view, want)
		}
	})
}

func TestTokenExchangeErrors(t *testing.T) {
	newCode := func(r *rig) (code, verifier string) {
		grantID := r.approve(staticClientID, "Dev App", domain.OAuthScopeRead)
		v, challenge := pkcePair()
		return r.issueCode(authParams(staticClientID, staticRedirect, challenge), grantID), v
	}
	wantError := func(t *testing.T, resp tokenResponse, status int, code string) {
		t.Helper()
		if resp.Status != status || resp.str("error") != code {
			t.Errorf("status = %d error = %q (%v), want %d %q", resp.Status, resp.str("error"), resp.Body, status, code)
		}
		if resp.str("access_token") != "" || resp.str("refresh_token") != "" {
			t.Error("エラー応答にトークンが含まれている")
		}
	}

	t.Run("PKCE の verifier が違うと、トークンを発行せずに invalid_grant で断る", func(t *testing.T) {
		r := newRig(t)
		code, _ := newCode(r)
		wantError(t, r.exchange(staticClientID, staticRedirect, code, "wrong-verifier-0123456789-abcdefghijklmnopqrstuvwxyz"), http.StatusBadRequest, "invalid_grant")
	})

	t.Run("PKCE の verifier を付けずに交換しようとすると、トークンを発行せずに断る", func(t *testing.T) {
		r := newRig(t)
		code, _ := newCode(r)
		resp := r.postToken(url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {staticRedirect}, "client_id": {staticClientID}})
		if resp.Status != http.StatusBadRequest || resp.str("access_token") != "" {
			t.Errorf("status = %d body = %v", resp.Status, resp.Body)
		}
	})

	t.Run("認可のときと違う戻り先で交換しようとすると、invalid_grant で断る", func(t *testing.T) {
		r := newRig(t)
		code, verifier := newCode(r)
		wantError(t, r.exchange(staticClientID, httpsRedirect, code, verifier), http.StatusBadRequest, "invalid_grant")
	})

	t.Run("別のアプリの client_id で交換しようとすると、invalid_grant で断る", func(t *testing.T) {
		r := newRig(t)
		code, verifier := newCode(r)
		r.fetcher.docs[metadataURL] = `{"client_id":"` + metadataURL + `","client_name":"Other","redirect_uris":["` + staticRedirect + `"]}`
		resp := r.exchange(metadataURL, staticRedirect, code, verifier)
		if resp.Status != http.StatusBadRequest || resp.str("access_token") != "" {
			t.Errorf("status = %d body = %v", resp.Status, resp.Body)
		}
	})

	t.Run("でたらめな認可コードで交換しようとすると、invalid_grant で断る", func(t *testing.T) {
		r := newRig(t)
		_, verifier := newCode(r)
		wantError(t, r.exchange(staticClientID, staticRedirect, "not-a-real-code.signature", verifier), http.StatusBadRequest, "invalid_grant")
	})

	t.Run("知らないアプリが交換しようとすると、invalid_client で断る", func(t *testing.T) {
		r := newRig(t)
		code, verifier := newCode(r)
		resp := r.exchange("unknown-app", staticRedirect, code, verifier)
		if resp.Status != http.StatusUnauthorized && resp.Status != http.StatusBadRequest || resp.str("error") != "invalid_client" {
			t.Errorf("status = %d error = %q", resp.Status, resp.str("error"))
		}
	})

	t.Run("別のサーバーの宛先を指定して交換しようとすると、invalid_target で断る", func(t *testing.T) {
		r := newRig(t)
		code, verifier := newCode(r)
		resp := r.postToken(url.Values{
			"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {staticRedirect},
			"client_id": {staticClientID}, "code_verifier": {verifier}, "resource": {"https://other.example.com/mcp"},
		})
		wantError(t, resp, http.StatusBadRequest, "invalid_target")
	})

	t.Run("有効期限を過ぎた認可コードで交換しようとすると、invalid_grant で断る", func(t *testing.T) {
		r := newRig(t)
		code, verifier := newCode(r)
		if _, err := r.pool.Exec(r.ctx, `UPDATE oauth_token_sessions SET request = jsonb_set(request, '{session,expires_at,authorize_code}', '"2000-01-01T00:00:00Z"') WHERE kind = 'authorization_code'`); err != nil {
			t.Fatal(err)
		}
		wantError(t, r.exchange(staticClientID, staticRedirect, code, verifier), http.StatusBadRequest, "invalid_grant")
	})

	for _, grant := range []string{"password", "client_credentials"} {
		t.Run("対応していない方式("+grant+")の要求は、トークンを発行せずに断る", func(t *testing.T) {
			r := newRig(t)
			resp := r.postToken(url.Values{"grant_type": {grant}, "client_id": {staticClientID}, "username": {"alice"}, "password": {"secret"}})
			if resp.Status < 400 || resp.Status >= 500 || resp.str("access_token") != "" {
				t.Errorf("status = %d body = %v", resp.Status, resp.Body)
			}
		})
	}
}

func TestAuthorizationCodeReuse(t *testing.T) {
	t.Run("使用済みの認可コードをもう一度使うと断られ、そのコードから発行済みのトークンも使えなくなる", func(t *testing.T) {
		r := newRig(t)
		grantID := r.approve(staticClientID, "Dev App", domain.OAuthScopeRead)
		verifier, challenge := pkcePair()
		code := r.issueCode(authParams(staticClientID, staticRedirect, challenge), grantID)
		first := r.exchange(staticClientID, staticRedirect, code, verifier)
		if first.Status != http.StatusOK {
			t.Fatalf("1 回目 = %d %v", first.Status, first.Body)
		}
		if _, err := r.srv.IntrospectAccessToken(r.ctx, first.str("access_token")); err != nil {
			t.Fatalf("交換直後のアクセストークンが使えない: %v", err)
		}

		second := r.exchange(staticClientID, staticRedirect, code, verifier)
		if second.Status != http.StatusBadRequest || second.str("error") != "invalid_grant" || second.str("access_token") != "" {
			t.Errorf("2 回目 = %d %v, want 400 invalid_grant", second.Status, second.Body)
		}
		if _, err := r.srv.IntrospectAccessToken(r.ctx, first.str("access_token")); !errors.Is(err, domain.ErrOAuthInvalidToken) {
			t.Errorf("再利用のあと、発行済みのアクセストークンは err = %v, want ErrOAuthInvalidToken", err)
		}
		if resp := r.refresh(staticClientID, first.str("refresh_token")); resp.Status != http.StatusBadRequest {
			t.Errorf("再利用のあと、発行済みの更新トークンで取り直せた: %d %v", resp.Status, resp.Body)
		}
	})

	t.Run("同じ認可コードを並行して交換しても、トークンが発行されるのは 1 つだけで、残りは invalid_grant になる", func(t *testing.T) {
		r := newRig(t)
		grantID := r.approve(staticClientID, "Dev App", domain.OAuthScopeRead)
		verifier, challenge := pkcePair()
		code := r.issueCode(authParams(staticClientID, staticRedirect, challenge), grantID)
		const workers = 6
		results := make(chan tokenResponse, workers)
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results <- r.exchange(staticClientID, staticRedirect, code, verifier)
			}()
		}
		wg.Wait()
		close(results)
		issued := 0
		for resp := range results {
			switch {
			case resp.Status == http.StatusOK && resp.str("access_token") != "":
				issued++
			case resp.Status == http.StatusBadRequest && resp.str("error") == "invalid_grant":
			default:
				t.Errorf("想定外の応答: %d %v", resp.Status, resp.Body)
			}
		}
		if issued > 1 {
			t.Errorf("トークンが %d 回発行された、want at most 1", issued)
		}
	})
}

func TestRefreshTokenRotation(t *testing.T) {
	t.Run("更新トークンを使うと、新しいトークンが発行され、古いアクセストークンと古い更新トークンは使えなくなる", func(t *testing.T) {
		r := newRig(t)
		access1, refresh1, _ := r.tokens(domain.OAuthScopeRead)

		resp := r.refresh(staticClientID, refresh1)
		if resp.Status != http.StatusOK {
			t.Fatalf("更新 = %d %v", resp.Status, resp.Body)
		}
		access2, refresh2 := resp.str("access_token"), resp.str("refresh_token")
		if access2 == "" || refresh2 == "" || access2 == access1 || refresh2 == refresh1 {
			t.Errorf("新しいトークンが古いものと同じか、空: access %v refresh %v", access2 == access1, refresh2 == refresh1)
		}
		if resp.str("scope") != domain.OAuthScopeRead {
			t.Errorf("scope = %q, want the granted scope", resp.str("scope"))
		}
		if _, err := r.srv.IntrospectAccessToken(r.ctx, access1); !errors.Is(err, domain.ErrOAuthInvalidToken) {
			t.Errorf("古いアクセストークン err = %v, want ErrOAuthInvalidToken", err)
		}
		token, err := r.srv.IntrospectAccessToken(r.ctx, access2)
		if err != nil || token.UserID != r.userID {
			t.Errorf("新しいアクセストークン = %+v, err = %v", token, err)
		}
	})

	t.Run("入れ替え済みの古い更新トークンを再び使うと断られ、その認可から発行されたトークンがすべて使えなくなる", func(t *testing.T) {
		r := newRig(t)
		_, refresh1, _ := r.tokens(domain.OAuthScopeRead)
		second := r.refresh(staticClientID, refresh1)
		if second.Status != http.StatusOK {
			t.Fatalf("更新 = %d %v", second.Status, second.Body)
		}

		reuse := r.refresh(staticClientID, refresh1)
		if reuse.Status != http.StatusBadRequest || reuse.str("error") != "invalid_grant" {
			t.Errorf("再利用 = %d %v, want 400 invalid_grant", reuse.Status, reuse.Body)
		}
		if _, err := r.srv.IntrospectAccessToken(r.ctx, second.str("access_token")); !errors.Is(err, domain.ErrOAuthInvalidToken) {
			t.Errorf("再利用のあと、新しいアクセストークン err = %v, want ErrOAuthInvalidToken", err)
		}
		if resp := r.refresh(staticClientID, second.str("refresh_token")); resp.Status != http.StatusBadRequest {
			t.Errorf("再利用のあと、新しい更新トークンで取り直せた: %d %v", resp.Status, resp.Body)
		}
	})

	t.Run("有効期限を過ぎた更新トークンでは取り直せず、invalid_grant で断る", func(t *testing.T) {
		r := newRig(t)
		_, refresh, _ := r.tokens(domain.OAuthScopeRead)
		if _, err := r.pool.Exec(r.ctx, `UPDATE oauth_token_sessions SET request = jsonb_set(request, '{session,expires_at,refresh_token}', '"2000-01-01T00:00:00Z"') WHERE kind = 'refresh_token'`); err != nil {
			t.Fatal(err)
		}
		if resp := r.refresh(staticClientID, refresh); resp.Status != http.StatusBadRequest || resp.str("error") != "invalid_grant" {
			t.Errorf("= %d %v, want 400 invalid_grant", resp.Status, resp.Body)
		}
	})

	t.Run("別のアプリの client_id で更新トークンを使おうとすると、断られる", func(t *testing.T) {
		r := newRig(t)
		_, refresh, _ := r.tokens(domain.OAuthScopeRead)
		r.fetcher.docs[metadataURL] = `{"client_id":"` + metadataURL + `","client_name":"Other","redirect_uris":["` + metadataRedirct + `"]}`
		if resp := r.refresh(metadataURL, refresh); resp.Status != http.StatusBadRequest || resp.str("access_token") != "" {
			t.Errorf("= %d %v", resp.Status, resp.Body)
		}
	})

	t.Run("同じ更新トークンを並行して使っても、新しいトークンが発行されるのは 1 つだけである", func(t *testing.T) {
		r := newRig(t)
		_, refresh, _ := r.tokens(domain.OAuthScopeRead)
		const workers = 6
		results := make(chan tokenResponse, workers)
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results <- r.refresh(staticClientID, refresh)
			}()
		}
		wg.Wait()
		close(results)
		issued := 0
		for resp := range results {
			if resp.Status == http.StatusOK {
				issued++
			}
		}
		if issued != 1 {
			t.Errorf("新しいトークンが %d 回発行された, want exactly 1", issued)
		}
	})
}

func TestAccessTokenVerification(t *testing.T) {
	t.Run("読み取りだけを許可したトークンは、書き込みの範囲が足りないと判断される", func(t *testing.T) {
		r := newRig(t)
		access, _, _ := r.tokens(domain.OAuthScopeRead)
		token, err := r.srv.IntrospectAccessToken(r.ctx, access)
		if err != nil {
			t.Fatal(err)
		}
		if err := token.Check(testResource, domain.OAuthScopeRead); err != nil {
			t.Errorf("読み取り: %v", err)
		}
		if err := token.Check(testResource, domain.OAuthScopeWrite); !errors.Is(err, domain.ErrOAuthInsufficientScope) {
			t.Errorf("書き込み err = %v, want ErrOAuthInsufficientScope", err)
		}
	})

	t.Run("トークンの宛先は、この認可サーバーの宛先で、別のサーバーへの使用は断られる", func(t *testing.T) {
		r := newRig(t)
		access, _, _ := r.tokens(domain.OAuthScopeRead)
		token, err := r.srv.IntrospectAccessToken(r.ctx, access)
		if err != nil {
			t.Fatal(err)
		}
		if err := token.Check("https://other.example.com/mcp", domain.OAuthScopeRead); !errors.Is(err, domain.ErrOAuthInvalidToken) {
			t.Errorf("err = %v, want ErrOAuthInvalidToken", err)
		}
	})

	t.Run("有効期限を過ぎたアクセストークンは、無効なトークンとして断られる", func(t *testing.T) {
		r := newRig(t)
		access, _, _ := r.tokens(domain.OAuthScopeRead)
		if _, err := r.pool.Exec(r.ctx, `UPDATE oauth_token_sessions SET request = jsonb_set(request, '{session,expires_at,access_token}', '"2000-01-01T00:00:00Z"') WHERE kind = 'access_token'`); err != nil {
			t.Fatal(err)
		}
		if _, err := r.srv.IntrospectAccessToken(r.ctx, access); !errors.Is(err, domain.ErrOAuthInvalidToken) {
			t.Errorf("err = %v, want ErrOAuthInvalidToken", err)
		}
	})

	t.Run("でたらめな文字列・空文字・更新トークンは、アクセストークンとして受け付けない", func(t *testing.T) {
		r := newRig(t)
		_, refresh, _ := r.tokens(domain.OAuthScopeRead)
		for name, raw := range map[string]string{
			"でたらめ": "garbage", "空文字": "", "署名がでたらめな形": "abc.def", "更新トークン": refresh,
		} {
			if _, err := r.srv.IntrospectAccessToken(r.ctx, raw); !errors.Is(err, domain.ErrOAuthInvalidToken) {
				t.Errorf("%s: err = %v, want ErrOAuthInvalidToken", name, err)
			}
		}
	})

	t.Run("トークンの一部を書き換えると、無効なトークンとして断られる", func(t *testing.T) {
		r := newRig(t)
		access, _, _ := r.tokens(domain.OAuthScopeRead)
		tampered := access[:len(access)-2] + "xx"
		if _, err := r.srv.IntrospectAccessToken(r.ctx, tampered); !errors.Is(err, domain.ErrOAuthInvalidToken) {
			t.Errorf("err = %v, want ErrOAuthInvalidToken", err)
		}
	})

	t.Run("データベースに接続できないときは、無効なトークンとは区別して、確かめられなかったエラーを返す", func(t *testing.T) {
		r := newRig(t)
		access, _, _ := r.tokens(domain.OAuthScopeRead)
		r.pool.Close()
		_, err := r.srv.IntrospectAccessToken(r.ctx, access)
		if err == nil || errors.Is(err, domain.ErrOAuthInvalidToken) {
			t.Errorf("err = %v, want a storage error (not ErrOAuthInvalidToken)", err)
		}
	})
}

func TestGrantRevocation(t *testing.T) {
	t.Run("許可を取り消すと、そのアプリのアクセストークンも更新トークンも、すぐに使えなくなる", func(t *testing.T) {
		r := newRig(t)
		grantID := r.approve(staticClientID, "Dev App", domain.OAuthScopeRead)
		verifier, challenge := pkcePair()
		code := r.issueCode(authParams(staticClientID, staticRedirect, challenge), grantID)
		resp := r.exchange(staticClientID, staticRedirect, code, verifier)
		if _, err := r.srv.IntrospectAccessToken(r.ctx, resp.str("access_token")); err != nil {
			t.Fatalf("取り消し前: %v", err)
		}

		if err := r.grants.Revoke(r.ctx, r.userID, grantID); err != nil {
			t.Fatal(err)
		}
		if _, err := r.srv.IntrospectAccessToken(r.ctx, resp.str("access_token")); !errors.Is(err, domain.ErrOAuthInvalidToken) {
			t.Errorf("取り消し後のアクセストークン err = %v, want ErrOAuthInvalidToken", err)
		}
		if r := r.refresh(staticClientID, resp.str("refresh_token")); r.Status != http.StatusBadRequest {
			t.Errorf("取り消し後の更新トークンで取り直せた: %d %v", r.Status, r.Body)
		}
	})

	t.Run("許可を取り消すと、まだ交換していない認可コードも使えなくなる", func(t *testing.T) {
		r := newRig(t)
		grantID := r.approve(staticClientID, "Dev App", domain.OAuthScopeRead)
		verifier, challenge := pkcePair()
		code := r.issueCode(authParams(staticClientID, staticRedirect, challenge), grantID)
		if err := r.grants.Revoke(r.ctx, r.userID, grantID); err != nil {
			t.Fatal(err)
		}
		if resp := r.exchange(staticClientID, staticRedirect, code, verifier); resp.Status != http.StatusBadRequest {
			t.Errorf("= %d %v, want 400", resp.Status, resp.Body)
		}
	})

	t.Run("あるアプリの許可を取り消しても、別のアプリのトークンは使える", func(t *testing.T) {
		r := newRig(t)
		accessA, _, _ := r.tokens(domain.OAuthScopeRead)
		r.fetcher.docs[metadataURL] = `{"client_id":"` + metadataURL + `","client_name":"Other App","redirect_uris":["` + metadataRedirct + `"]}`
		otherGrant := r.approve(metadataURL, "Other App", domain.OAuthScopeRead)
		verifier, challenge := pkcePair()
		code := r.issueCode(authParams(metadataURL, metadataRedirct, challenge), otherGrant)
		otherResp := r.exchange(metadataURL, metadataRedirct, code, verifier)
		if otherResp.Status != http.StatusOK {
			t.Fatalf("別のアプリの交換 = %d %v", otherResp.Status, otherResp.Body)
		}
		if err := r.grants.Revoke(r.ctx, r.userID, otherGrant); err != nil {
			t.Fatal(err)
		}
		if _, err := r.srv.IntrospectAccessToken(r.ctx, accessA); err != nil {
			t.Errorf("取り消していないアプリのトークンが使えない: %v", err)
		}
		if _, err := r.srv.IntrospectAccessToken(r.ctx, otherResp.str("access_token")); !errors.Is(err, domain.ErrOAuthInvalidToken) {
			t.Errorf("取り消したアプリのトークン err = %v, want ErrOAuthInvalidToken", err)
		}
	})
}

func TestRevocationEndpoint(t *testing.T) {
	revoke := func(r *rig, form url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/oauth/revoke", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		r.srv.HandleRevoke(rec, req)
		return rec
	}

	t.Run("更新トークンを取り消すと、その認可から発行されたトークンがすべて使えなくなる", func(t *testing.T) {
		r := newRig(t)
		access, refresh, _ := r.tokens(domain.OAuthScopeRead)
		rec := revoke(r, url.Values{"token": {refresh}, "token_type_hint": {"refresh_token"}, "client_id": {staticClientID}})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
		}
		if _, err := r.srv.IntrospectAccessToken(r.ctx, access); !errors.Is(err, domain.ErrOAuthInvalidToken) {
			t.Errorf("アクセストークン err = %v, want ErrOAuthInvalidToken", err)
		}
		if resp := r.refresh(staticClientID, refresh); resp.Status != http.StatusBadRequest {
			t.Errorf("取り消した更新トークンで取り直せた: %d %v", resp.Status, resp.Body)
		}
	})

	t.Run("存在しないトークンの取り消しは、エラーにせず成功として扱う", func(t *testing.T) {
		r := newRig(t)
		rec := revoke(r, url.Values{"token": {"nothing.here"}, "client_id": {staticClientID}})
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d body = %s", rec.Code, rec.Body.String())
		}
	})
}

func TestMetadata(t *testing.T) {
	r := newRig(t)
	rec := httptest.NewRecorder()
	r.srv.HandleMetadata(rec, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status = %d, content-type = %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	body := r.srv.Metadata()
	t.Run("認可サーバーの情報に、発行者と、認可・トークン・取り消しの窓口の URL が入る", func(t *testing.T) {
		for k, want := range map[string]string{
			"issuer":                 testIssuer,
			"authorization_endpoint": testIssuer + "/oauth/authorize",
			"token_endpoint":         testIssuer + "/oauth/token",
			"revocation_endpoint":    testIssuer + "/oauth/revoke",
		} {
			if body[k] != want {
				t.Errorf("%s = %v, want %s", k, body[k], want)
			}
		}
	})
	t.Run("対応する方式は、認可コード・更新・S256 の PKCE・秘密の鍵なしのアプリだけである", func(t *testing.T) {
		for k, want := range map[string][]string{
			"response_types_supported":              {"code"},
			"grant_types_supported":                 {"authorization_code", "refresh_token"},
			"code_challenge_methods_supported":      {"S256"},
			"token_endpoint_auth_methods_supported": {"none"},
			"scopes_supported":                      {domain.OAuthScopeRead, domain.OAuthScopeWrite},
		} {
			got, _ := body[k].([]string)
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("%s = %v, want %v", k, got, want)
			}
		}
	})
	t.Run("アプリが自分の説明を URL で公開する方式と、戻り先への発行者の付与に対応していると伝える", func(t *testing.T) {
		if body["client_id_metadata_document_supported"] != true || body["authorization_response_iss_parameter_supported"] != true {
			t.Errorf("metadata = %v", body)
		}
	})
}

func TestClientMetadataDocuments(t *testing.T) {
	_, challenge := pkcePair()
	doc := func(redirect string) string {
		return `{"client_id":"` + metadataURL + `","client_name":"Docs App","redirect_uris":["` + redirect + `"],"token_endpoint_auth_method":"none"}`
	}

	t.Run("説明を公開しているアプリは、文書の戻り先で認可を進められ、同意画面に文書の名前が出る", func(t *testing.T) {
		r := newRig(t)
		r.fetcher.docs[metadataURL] = doc(metadataRedirct)
		view, err := r.srv.DescribeAuthorizeRequest(r.ctx, authParams(metadataURL, metadataRedirct, challenge))
		if err != nil || view.ClientName != "Docs App" || view.ClientID != metadataURL {
			t.Fatalf("view = %+v, err = %v", view, err)
		}
		grantID := r.approve(metadataURL, view.ClientName, domain.OAuthScopeRead)
		if redirect := r.issue(authParams(metadataURL, metadataRedirct, challenge), grantID); redirect.Query().Get("code") == "" {
			t.Errorf("認可コードがない: %s", redirect)
		}
	})

	t.Run("文書に書かれていない戻り先の要求は、リダイレクトせずにエラーで返す", func(t *testing.T) {
		r := newRig(t)
		r.fetcher.docs[metadataURL] = doc(metadataRedirct)
		_, err := r.srv.DescribeAuthorizeRequest(r.ctx, authParams(metadataURL, "https://evil.example.com/callback", challenge))
		if !errors.Is(err, domain.ErrOAuthAuthorizeRequestInvalid) {
			t.Errorf("err = %v, want ErrOAuthAuthorizeRequestInvalid", err)
		}
	})

	t.Run("文書を取得できないアプリは、リダイレクトせずにエラーで返す", func(t *testing.T) {
		r := newRig(t)
		r.fetcher.err = errors.New("connection refused")
		_, err := r.srv.DescribeAuthorizeRequest(r.ctx, authParams(metadataURL, metadataRedirct, challenge))
		if !errors.Is(err, domain.ErrOAuthAuthorizeRequestInvalid) {
			t.Errorf("err = %v, want ErrOAuthAuthorizeRequestInvalid", err)
		}
	})

	t.Run("文書の client_id が URL と違うアプリは、なりすましなので断る", func(t *testing.T) {
		r := newRig(t)
		r.fetcher.docs[metadataURL] = `{"client_id":"https://other.example.com/client.json","client_name":"x","redirect_uris":["` + metadataRedirct + `"]}`
		if _, err := r.srv.DescribeAuthorizeRequest(r.ctx, authParams(metadataURL, metadataRedirct, challenge)); !errors.Is(err, domain.ErrOAuthAuthorizeRequestInvalid) {
			t.Errorf("err = %v, want ErrOAuthAuthorizeRequestInvalid", err)
		}
	})

	t.Run("取得した文書は一定時間覚えていて、続けて使っても取りに行くのは 1 回だけである", func(t *testing.T) {
		r := newRig(t)
		r.fetcher.docs[metadataURL] = doc(metadataRedirct)
		for i := 0; i < 3; i++ {
			if _, err := r.srv.DescribeAuthorizeRequest(r.ctx, authParams(metadataURL, metadataRedirct, challenge)); err != nil {
				t.Fatal(err)
			}
		}
		if r.fetcher.calls != 1 {
			t.Errorf("取得の回数 = %d, want 1", r.fetcher.calls)
		}
	})

	t.Run("固定で登録したアプリは、文書を取りに行かない", func(t *testing.T) {
		r := newRig(t)
		if _, err := r.srv.DescribeAuthorizeRequest(r.ctx, authParams(staticClientID, staticRedirect, challenge)); err != nil {
			t.Fatal(err)
		}
		if r.fetcher.calls != 0 {
			t.Errorf("取得の回数 = %d, want 0", r.fetcher.calls)
		}
	})

	t.Run("文書に書かれた範囲や宛先の指定にかかわらず、許可できる範囲は認可サーバーが決めた範囲だけである", func(t *testing.T) {
		r := newRig(t)
		r.fetcher.docs[metadataURL] = `{"client_id":"` + metadataURL + `","client_name":"Greedy","redirect_uris":["` + metadataRedirct + `"],"scope":"admin","audience":["https://other.example.com"]}`
		params := authParams(metadataURL, metadataRedirct, challenge, "scope", "admin")
		grantID := r.approve(metadataURL, "Greedy", domain.OAuthScopeRead)
		redirect := r.issue(params, grantID)
		if redirect.Query().Get("error") != "invalid_scope" || redirect.Query().Get("code") != "" {
			t.Errorf("redirect = %s, want invalid_scope", redirect)
		}
	})
}

func TestNew(t *testing.T) {
	valid := oauthserver.Config{
		Issuer: testIssuer, Resource: testResource, ConsentURL: testConsentURL,
		Secret: []byte("0123456789abcdef0123456789abcdef"),
	}
	tests := []struct {
		name   string
		mutate func(c *oauthserver.Config)
		ok     bool
	}{
		{"必要な設定がそろっていれば、組み立てられる", func(*oauthserver.Config) {}, true},
		{"署名の秘密の鍵が 32 バイトより短いと、組み立てに失敗する", func(c *oauthserver.Config) { c.Secret = []byte("short") }, false},
		{"発行者が URL でないと、組み立てに失敗する", func(c *oauthserver.Config) { c.Issuer = "not a url" }, false},
		{"宛先が URL でないと、組み立てに失敗する", func(c *oauthserver.Config) { c.Resource = "" }, false},
		{"許可を尋ねる画面の URL がないと、組み立てに失敗する", func(c *oauthserver.Config) { c.ConsentURL = "" }, false},
		{"規則を満たさない固定のアプリ(戻り先がない)があると、組み立てに失敗する", func(c *oauthserver.Config) {
			c.StaticClients = []domain.OAuthClient{{ID: "x", Name: "x"}}
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			tt.mutate(&cfg)
			_, err := oauthserver.New(cfg, nil, nil, nil)
			if (err == nil) != tt.ok {
				t.Errorf("err = %v, want ok = %v", err, tt.ok)
			}
		})
	}
}
