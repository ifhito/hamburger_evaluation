package handler_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/handler"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/infra"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/oauthserver"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/storage"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/uow"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

const (
	oauthTestIssuer   = "http://localhost:8080"
	oauthTestResource = "http://localhost:8080/mcp"
	oauthTestClient   = "dev-app"
	oauthTestRedirect = "http://127.0.0.1/callback"
)

// oauthKit は、本物の PostgreSQL・認可サーバー・JWT のログインを通して、許可の画面の API から
// トークンの検証までを確かめるための一式である。
type oauthKit struct {
	pool   *pgxpool.Pool
	router http.Handler
	tokens *usecase.OAuthAccessTokens
	alice  string
	bob    string
	// bearer は、利用者ごとのログイン用の JWT("Bearer ..." の形)である。
	bearer map[string]string
}

func newOAuthKit(t *testing.T) *oauthKit {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping DB-backed test in short mode")
	}
	ctx := context.Background()
	conn, dbURL := dbtest.New(t)
	alice := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('alice@example.com', 'alice', 'd') RETURNING id`)
	bob := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('bob@example.com', 'bob', 'd') RETURNING id`)
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	codec := infra.NewJWTCodec(testJWTSecret, time.Hour)
	userQuery := query.NewUserQuery(pool)
	userWrites := domain.NewUsers(repository.NewUserRepository(pool))
	server, err := oauthserver.New(oauthserver.Config{
		Issuer: oauthTestIssuer, Resource: oauthTestResource, ConsentURL: "http://localhost:5173/oauth/authorize",
		Secret: []byte("0123456789abcdef0123456789abcdef"),
		StaticClients: []domain.OAuthClient{{
			ID: oauthTestClient, Name: "Dev App", RedirectURIs: []string{oauthTestRedirect},
		}},
	}, oauthserver.Deps{
		Sessions: uow.NewOAuthTokenSessionStore(pool),
		Grants:   query.NewOAuthGrantQuery(pool),
		Users:    userQuery,
		Fetcher:  oauthserver.NewHTTPMetadataFetcher(),
	})
	if err != nil {
		t.Fatalf("oauthserver.New: %v", err)
	}
	grantWrites := domain.NewOAuthGrants(repository.NewOAuthGrantRepository(pool))
	grantQuery := query.NewOAuthGrantQuery(pool)
	unitOfWork := uow.New(pool)
	recalc := usecase.NewBurgerStatsRecalculator(infra.SystemClock{})
	router := handler.NewRouter(pool, usecase.NewAuth(userQuery, hasherFake{}, codec, codec), unusedSignups(),
		usecase.NewShops(query.NewShopQuery(pool), domain.NewShops(repository.NewShopRepository(pool))),
		usecase.NewReviews(query.NewReviewQuery(pool), unitOfWork, recalc, storage.NewDisk(t.TempDir(), "/photos")),
		usecase.NewUsers(userQuery, userWrites, unitOfWork, recalc, hasherFake{}), nil,
		&handler.OAuth{
			Endpoints: server,
			Consents:  usecase.NewOAuthConsents(server, grantQuery, grantWrites),
			Apps:      usecase.NewConnectedApps(grantQuery, grantWrites),
		}, nil)

	bearer := map[string]string{}
	for _, id := range []string{alice, bob} {
		tok, err := codec.Issue(id)
		if err != nil {
			t.Fatal(err)
		}
		bearer[id] = "Bearer " + tok
	}
	return &oauthKit{
		pool: pool, router: router, alice: alice, bob: bob, bearer: bearer,
		tokens: usecase.NewOAuthAccessTokens(server, userQuery, oauthTestResource),
	}
}

func pkce() (verifier, challenge string) {
	verifier = "test-verifier-0123456789-abcdefghijklmnopqrstuvwxyz"
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

// authQuery は、認可の URL の値(URL の ? のあとの文字列)を作る。
func authQuery(challenge string, overrides ...string) string {
	v := url.Values{
		"response_type": {"code"}, "client_id": {oauthTestClient}, "redirect_uri": {oauthTestRedirect},
		"scope": {domain.OAuthScopeRead}, "state": {"state-1234567890"},
		"code_challenge": {challenge}, "code_challenge_method": {"S256"}, "resource": {oauthTestResource},
	}
	for i := 0; i+1 < len(overrides); i += 2 {
		if overrides[i+1] == "" {
			v.Del(overrides[i])
		} else {
			v.Set(overrides[i], overrides[i+1])
		}
	}
	return v.Encode()
}

func (k *oauthKit) describe(userID, query string) *httptest.ResponseRecorder {
	return do(k.router, http.MethodGet, "/oauth/authorize/request?"+query, "", k.bearer[userID])
}

func (k *oauthKit) decide(userID, query string, approve any) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]any{"query": query, "approve": approve})
	return do(k.router, http.MethodPost, "/oauth/authorize/decision", string(body), k.bearer[userID])
}

// redirectTo は、許可・拒否の応答から、アプリへの戻り先を取り出す。
func redirectTo(t *testing.T, rec *httptest.ResponseRecorder) *url.URL {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		RedirectTo string `json:"redirect_to"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(body.RedirectTo)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func (k *oauthKit) exchange(t *testing.T, code, verifier string) map[string]any {
	t.Helper()
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {oauthTestRedirect}, "client_id": {oauthTestClient}, "code_verifier": {verifier}}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	k.router.ServeHTTP(rec, req)
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("token status = %d, body = %s", rec.Code, rec.Body.String())
	}
	return body
}

// grantScopeJSON は、許可の画面の API と接続済みアプリの一覧に出る、範囲 1 つである(Writes は、キーの有無を見分けるため、ポインタにしてある)。
type grantScopeJSON struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Writes      *bool  `json:"writes"`
}

type grantJSON struct {
	ID         string           `json:"id"`
	ClientID   string           `json:"client_id"`
	ClientName string           `json:"client_name"`
	Scopes     []grantScopeJSON `json:"scopes"`
}

func (k *oauthKit) grants(t *testing.T, userID string) []grantJSON {
	t.Helper()
	rec := do(k.router, http.MethodGet, "/oauth/grants", "", k.bearer[userID])
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /oauth/grants = %d %s", rec.Code, rec.Body.String())
	}
	var out []grantJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestOAuthConsentFlow(t *testing.T) {
	t.Run("許可の画面の API で許可し、アプリがトークンに交換し、そのトークンが使え、許可を取り消すと使えなくなる", func(t *testing.T) {
		k := newOAuthKit(t)
		verifier, challenge := pkce()
		q := authQuery(challenge)

		rec := k.describe(k.alice, q)
		if rec.Code != http.StatusOK {
			t.Fatalf("describe = %d %s", rec.Code, rec.Body.String())
		}
		var view struct {
			Client          struct{ ID, Name string } `json:"client"`
			Scopes          []struct{ Name, Description string }
			ConsentRequired bool `json:"consent_required"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		if view.Client.ID != oauthTestClient || view.Client.Name != "Dev App" || !view.ConsentRequired ||
			len(view.Scopes) != 1 || view.Scopes[0].Name != domain.OAuthScopeRead || view.Scopes[0].Description == "" {
			t.Errorf("view = %+v", view)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("Cache-Control = %q, want no-store", got)
		}

		back := redirectTo(t, k.decide(k.alice, q, true))
		if got := back.Scheme + "://" + back.Host + back.Path; got != oauthTestRedirect {
			t.Errorf("戻り先 = %s", got)
		}
		if back.Query().Get("state") != "state-1234567890" || back.Query().Get("iss") != oauthTestIssuer || back.Query().Get("code") == "" {
			t.Fatalf("戻り先の値 = %s", back)
		}

		tokens := k.exchange(t, back.Query().Get("code"), verifier)
		access, _ := tokens["access_token"].(string)
		user, token, err := k.tokens.Authenticate(context.Background(), access, domain.OAuthScopeRead)
		if err != nil || user.ID != k.alice || token.ClientID != oauthTestClient {
			t.Fatalf("Authenticate = %+v, %+v, %v", user, token, err)
		}
		if _, _, err := k.tokens.Authenticate(context.Background(), access, domain.OAuthScopeWrite); !errors.Is(err, domain.ErrOAuthInsufficientScope) {
			t.Errorf("書き込みの範囲 err = %v, want ErrOAuthInsufficientScope", err)
		}

		grants := k.grants(t, k.alice)
		if len(grants) != 1 || grants[0].ClientID != oauthTestClient || grants[0].ClientName != "Dev App" || len(grants[0].Scopes) != 1 || grants[0].Scopes[0].Description == "" {
			t.Fatalf("grants = %+v", grants)
		}
		if other := k.grants(t, k.bob); len(other) != 0 {
			t.Errorf("別の利用者に許可が見える: %+v", other)
		}

		if rec := do(k.router, http.MethodDelete, "/oauth/grants/"+grants[0].ID, "", k.bearer[k.alice]); rec.Code != http.StatusNoContent {
			t.Fatalf("DELETE = %d %s", rec.Code, rec.Body.String())
		}
		if _, _, err := k.tokens.Authenticate(context.Background(), access, domain.OAuthScopeRead); !errors.Is(err, domain.ErrOAuthInvalidToken) {
			t.Errorf("取り消し後のアクセストークン err = %v, want ErrOAuthInvalidToken", err)
		}
		if len(k.grants(t, k.alice)) != 0 {
			t.Error("取り消した許可が一覧に残っている")
		}
	})

	t.Run("すでに許可した範囲の要求は尋ねる必要がなく、範囲が広がった要求は、尋ね直す", func(t *testing.T) {
		k := newOAuthKit(t)
		_, challenge := pkce()
		redirectTo(t, k.decide(k.alice, authQuery(challenge), true))

		consent := func(q string) bool {
			rec := k.describe(k.alice, q)
			var v struct {
				ConsentRequired bool `json:"consent_required"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil || rec.Code != http.StatusOK {
				t.Fatalf("describe = %d %s", rec.Code, rec.Body.String())
			}
			return v.ConsentRequired
		}
		if consent(authQuery(challenge)) {
			t.Error("同じ範囲の要求で、尋ねる必要があると返った")
		}
		wider := authQuery(challenge, "scope", "hamburger:read hamburger:write")
		if !consent(wider) {
			t.Error("範囲が広がった要求で、尋ねる必要がないと返った")
		}
		redirectTo(t, k.decide(k.alice, wider, true))
		if grants := k.grants(t, k.alice); len(grants) != 1 || len(grants[0].Scopes) != 2 {
			t.Errorf("grants = %+v, want one grant with both scopes", grants)
		}
		if consent(wider) {
			t.Error("広げた範囲を許可したあとも、尋ねる必要があると返った")
		}
		if !func() bool { // 別の利用者には、許可済みでも関係なく、初めての要求になる
			rec := k.describe(k.bob, authQuery(challenge))
			return strings.Contains(rec.Body.String(), `"consent_required":true`)
		}() {
			t.Error("別の利用者が許可したことになっている")
		}
	})

	t.Run("拒否すると、access_denied を付けたアプリへの戻り先を返し、許可は記録されない", func(t *testing.T) {
		k := newOAuthKit(t)
		_, challenge := pkce()
		back := redirectTo(t, k.decide(k.alice, authQuery(challenge), false))
		if back.Query().Get("error") != "access_denied" || back.Query().Get("code") != "" || back.Query().Get("iss") != oauthTestIssuer {
			t.Errorf("戻り先 = %s", back)
		}
		if len(k.grants(t, k.alice)) != 0 {
			t.Error("拒否したのに許可が記録された")
		}
	})
}

func TestOAuthConsentErrors(t *testing.T) {
	_, challenge := pkce()

	t.Run("ログインしていない(JWT がない)利用者は、許可の画面の API・許可の一覧・取り消しのどれも使えない", func(t *testing.T) {
		k := newOAuthKit(t)
		for _, tt := range []struct{ method, path, body string }{
			{http.MethodGet, "/oauth/authorize/request?" + authQuery(challenge), ""},
			{http.MethodPost, "/oauth/authorize/decision", `{"query":"x","approve":true}`},
			{http.MethodGet, "/oauth/grants", ""},
			{http.MethodDelete, "/oauth/grants/0b0e3a5c-8d54-4c1a-9f33-2a9d6f1c7e10", ""},
		} {
			if rec := do(k.router, tt.method, tt.path, tt.body, ""); rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s = %d, want 401", tt.method, tt.path, rec.Code)
			}
		}
	})

	t.Run("OAuth のアクセストークンは、ログイン用の JWT ではないので、許可の画面の API では受け付けない", func(t *testing.T) {
		k := newOAuthKit(t)
		verifier, ch := pkce()
		code := redirectTo(t, k.decide(k.alice, authQuery(ch), true)).Query().Get("code")
		access, _ := k.exchange(t, code, verifier)["access_token"].(string)
		if rec := do(k.router, http.MethodGet, "/oauth/grants", "", "Bearer "+access); rec.Code != http.StatusUnauthorized {
			t.Errorf("GET /oauth/grants with an OAuth access token = %d, want 401", rec.Code)
		}
		if rec := do(k.router, http.MethodGet, "/me", "", "Bearer "+access); rec.Code != http.StatusUnauthorized {
			t.Errorf("GET /me with an OAuth access token = %d, want 401", rec.Code)
		}
	})

	invalid := []struct {
		name  string
		query string
	}{
		{"登録されていない戻り先の要求は、422 で理由を返す", authQuery(challenge, "redirect_uri", "https://evil.example.com/callback")},
		{"知らないアプリの要求は、422 で理由を返す", authQuery(challenge, "client_id", "unknown-app")},
		{"PKCE がない要求は、許可を尋ねる前に、422 で理由を返す", authQuery(challenge, "code_challenge", "", "code_challenge_method", "")},
		{"知らない範囲を求める要求は、422 で理由を返す", authQuery(challenge, "scope", "admin")},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			k := newOAuthKit(t)
			rec := k.describe(k.alice, tt.query)
			if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"error"`) {
				t.Errorf("describe = %d %s", rec.Code, rec.Body.String())
			}
			// 不正な要求への許可は、422 で断り、許可の記録も認可コードも作らない。
			if rec := k.decide(k.alice, tt.query, true); rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("decide approve = %d %s", rec.Code, rec.Body.String())
			}
			if len(k.grants(t, k.alice)) != 0 {
				t.Error("不正な要求への許可が記録された")
			}
		})
	}

	t.Run("許可の本文の不備(approve がない・JSON でない・query が解釈できない)は、断る", func(t *testing.T) {
		k := newOAuthKit(t)
		if rec := do(k.router, http.MethodPost, "/oauth/authorize/decision", `{"query":"`+authQuery(challenge)+`"}`, k.bearer[k.alice]); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("approve なし = %d, want 422", rec.Code)
		}
		if rec := do(k.router, http.MethodPost, "/oauth/authorize/decision", `not json`, k.bearer[k.alice]); rec.Code != http.StatusBadRequest {
			t.Errorf("JSON でない本文 = %d, want 400", rec.Code)
		}
		if rec := k.decide(k.alice, "a=%zz", true); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("query が解釈できない = %d, want 422", rec.Code)
		}
	})

	t.Run("別の利用者の許可・存在しない許可・正規の形でない id の取り消しは、区別できない同一の 404 になり、許可は残る", func(t *testing.T) {
		k := newOAuthKit(t)
		redirectTo(t, k.decide(k.alice, authQuery(challenge), true))
		aliceGrant := k.grants(t, k.alice)[0].ID
		for _, id := range []string{aliceGrant, "0b0e3a5c-8d54-4c1a-9f33-2a9d6f1c7e10", "not-a-uuid"} {
			// aliceGrant は bob が消そうとする。ほかの 2 つは、存在しない。
			rec := do(k.router, http.MethodDelete, "/oauth/grants/"+id, "", k.bearer[k.bob])
			if rec.Code != http.StatusNotFound || rec.Body.String() != `{"error":"not found"}` {
				t.Errorf("DELETE %s = %d %s, want 404", id, rec.Code, rec.Body.String())
			}
		}
		if len(k.grants(t, k.alice)) != 1 {
			t.Error("別の利用者の取り消しで、許可が消えた")
		}
	})
}

// 退会した利用者と、退会がもたらす許可の取り消しの、許可の画面の API・接続済みアプリの一覧・取り消しへの影響。
func TestOAuthWithdrawnUsers(t *testing.T) {
	_, challenge := pkce()

	t.Run("退会した利用者は、退会前のログイン用の JWT でも、許可の画面の API・一覧・取り消しのどれも使えない", func(t *testing.T) {
		k := newOAuthKit(t)
		redirectTo(t, k.decide(k.alice, authQuery(challenge), true))
		grantID := k.grants(t, k.alice)[0].ID
		if _, err := k.pool.Exec(context.Background(), `UPDATE users SET discarded_at = now() WHERE id = $1`, k.alice); err != nil {
			t.Fatal(err)
		}
		for _, tt := range []struct{ method, path, body string }{
			{http.MethodGet, "/oauth/authorize/request?" + authQuery(challenge), ""},
			{http.MethodPost, "/oauth/authorize/decision", `{"query":"` + authQuery(challenge) + `","approve":true}`},
			{http.MethodGet, "/oauth/grants", ""},
			{http.MethodDelete, "/oauth/grants/" + grantID, ""},
		} {
			if rec := do(k.router, tt.method, tt.path, tt.body, k.bearer[k.alice]); rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s = %d, want 401", tt.method, tt.path, rec.Code)
			}
		}
	})

	t.Run("退会(DELETE /users/{id})すると、その利用者の許可とトークンの記録が消え、発行済みのアクセストークンは使えなくなり、別の利用者の許可は残る", func(t *testing.T) {
		k := newOAuthKit(t)
		verifier, ch := pkce()
		code := redirectTo(t, k.decide(k.alice, authQuery(ch), true)).Query().Get("code")
		access, _ := k.exchange(t, code, verifier)["access_token"].(string)
		redirectTo(t, k.decide(k.bob, authQuery(challenge), true))
		if _, _, err := k.tokens.Authenticate(context.Background(), access, domain.OAuthScopeRead); err != nil {
			t.Fatalf("退会前のアクセストークンが使えない: %v", err)
		}

		if rec := do(k.router, http.MethodDelete, "/users/"+k.alice, "", k.bearer[k.alice]); rec.Code != http.StatusNoContent {
			t.Fatalf("DELETE /users/{id} = %d %s", rec.Code, rec.Body.String())
		}

		count := func(sql string, args ...any) int {
			var n int
			if err := k.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
				t.Fatal(err)
			}
			return n
		}
		if n := count(`SELECT count(*) FROM oauth_grants WHERE user_id = $1`, k.alice); n != 0 {
			t.Errorf("退会した利用者の許可が %d 件残っている", n)
		}
		if n := count(`SELECT count(*) FROM oauth_token_sessions WHERE user_id = $1`, k.alice); n != 0 {
			t.Errorf("退会した利用者のトークンの記録が %d 件残っている", n)
		}
		if n := count(`SELECT count(*) FROM oauth_grants WHERE user_id = $1`, k.bob); n != 1 {
			t.Errorf("別の利用者の許可 = %d 件, want 1", n)
		}
		if _, _, err := k.tokens.Authenticate(context.Background(), access, domain.OAuthScopeRead); !errors.Is(err, domain.ErrOAuthInvalidToken) {
			t.Errorf("退会後のアクセストークン err = %v, want ErrOAuthInvalidToken", err)
		}
	})
}

// 許可の決定の API は、許可の記録を、要求された範囲だけで作る(利用者が求めていない範囲は、記録も、認可コードにも入らない)。
// すでにある、広い範囲の許可を、狭い要求で縮めることも、そこから広げることもない。
func TestOAuthDecisionRecordsOnlyTheRequestedScopes(t *testing.T) {
	k := newOAuthKit(t)
	verifier, challenge := pkce()

	wide := authQuery(challenge, "scope", "hamburger:read hamburger:write")
	redirectTo(t, k.decide(k.alice, wide, true))

	// 狭い(読み取りだけの)要求で許可しても、記録された範囲は減らず、発行される認可コードは、要求した範囲だけになる。
	code := redirectTo(t, k.decide(k.alice, authQuery(challenge), true)).Query().Get("code")
	tokens := k.exchange(t, code, verifier)
	if tokens["scope"] != domain.OAuthScopeRead {
		t.Errorf("狭い要求のトークンの scope = %v, want %s だけ", tokens["scope"], domain.OAuthScopeRead)
	}
	grants := k.grants(t, k.alice)
	if len(grants) != 1 || len(grants[0].Scopes) != 2 {
		t.Errorf("記録された許可 = %+v, want 1 件で読み取りと書き込みの両方", grants)
	}

	// 初めての利用者が、読み取りだけの要求を許可すると、記録される範囲も、読み取りだけになる。
	redirectTo(t, k.decide(k.bob, authQuery(challenge), true))
	if bobs := k.grants(t, k.bob); len(bobs) != 1 || len(bobs[0].Scopes) != 1 || bobs[0].Scopes[0].Name != domain.OAuthScopeRead {
		t.Errorf("bob の許可 = %+v, want 読み取りだけ", bobs)
	}
}

// 接続済みアプリの一覧は、既存の一覧(GET /shops・GET /reviews)と同じ契約でページ送りされる:
// page / per_page(整数でなければ 422、範囲外は補正)、1 ページの件数は backend が決める(既定 20、上限 100)、
// 続きがあるかはレスポンスヘッダー X-Has-More で返す。
func TestOAuthGrantsListIsPaginated(t *testing.T) {
	k := newOAuthKit(t)
	ctx := context.Background()
	// alice が 45 個のアプリを許可している(更新の新しい順に app-1, app-2, ...)。bob も 3 個許可している。
	for user, n := range map[string]int{k.alice: 45, k.bob: 3} {
		if _, err := k.pool.Exec(ctx, `INSERT INTO oauth_grants (user_id, client_id, client_name, scopes, created_at, updated_at)
			SELECT $1::uuid, 'app-' || g, 'App ' || g, ARRAY['hamburger:read'], now() - (g || ' minutes')::interval, now() - (g || ' minutes')::interval
			FROM generate_series(1, $2::int) AS g`, user, n); err != nil {
			t.Fatal(err)
		}
	}

	list := func(query string) (items []grantJSON, hasMore string, status int, body string) {
		t.Helper()
		rec := do(k.router, http.MethodGet, "/oauth/grants"+query, "", k.bearer[k.alice])
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
				t.Fatalf("decode %q: %v", rec.Body.String(), err)
			}
		}
		return items, rec.Header().Get("X-Has-More"), rec.Code, rec.Body.String()
	}

	t.Run("何も指定しなければ、1 ページ目の 20 件だけを返し、続きがあることを X-Has-More で伝える", func(t *testing.T) {
		items, hasMore, status, _ := list("")
		if status != http.StatusOK || len(items) != 20 || hasMore != "true" {
			t.Fatalf("status = %d, %d 件, X-Has-More = %q, want 200 で 20 件、true", status, len(items), hasMore)
		}
		if items[0].ClientID != "app-1" || items[19].ClientID != "app-20" {
			t.Errorf("並び = %s … %s, want 更新の新しい順(app-1 … app-20)", items[0].ClientID, items[19].ClientID)
		}
	})

	t.Run("ページを進めると残りが取れ、最後のページの X-Has-More は false で、全ページを合わせると重複も欠落もない", func(t *testing.T) {
		seen := map[string]bool{}
		for page, want := range map[int]struct {
			count   int
			hasMore string
		}{1: {20, "true"}, 2: {20, "true"}, 3: {5, "false"}, 4: {0, "false"}} {
			items, hasMore, status, _ := list("?page=" + strconv.Itoa(page))
			if status != http.StatusOK || len(items) != want.count || hasMore != want.hasMore {
				t.Errorf("page %d: status = %d, %d 件, X-Has-More = %q, want %d 件、%s", page, status, len(items), hasMore, want.count, want.hasMore)
			}
			for _, it := range items {
				if seen[it.ID] {
					t.Errorf("page %d: %s が重複している", page, it.ID)
				}
				seen[it.ID] = true
			}
		}
		if len(seen) != 45 {
			t.Errorf("全ページの合計 = %d 件, want 45(bob の分は含まない)", len(seen))
		}
	})

	t.Run("範囲を超えたページは、null ではなく空の配列と X-Has-More: false を返す", func(t *testing.T) {
		_, hasMore, status, body := list("?page=999")
		if status != http.StatusOK || hasMore != "false" || strings.TrimSpace(body) != "[]" {
			t.Errorf("status = %d, X-Has-More = %q, body = %q", status, hasMore, body)
		}
	})

	t.Run("1 ページの件数(per_page)は、上限(100)まで指定でき、範囲外の値は補正される", func(t *testing.T) {
		for _, tt := range []struct {
			query   string
			count   int
			hasMore string
		}{
			{"?per_page=100", 45, "false"},
			{"?per_page=101", 45, "false"}, // 上限 100 に補正されるので、45 件は 1 ページに収まる
			{"?per_page=5&page=9", 5, "false"},
			{"?per_page=5&page=1", 5, "true"},
			{"?per_page=0", 20, "true"}, // 0 以下は既定の 20 件
			{"?per_page=-3", 20, "true"},
			{"?page=0", 20, "true"}, // 1 未満のページは 1 ページ目
		} {
			items, hasMore, status, _ := list(tt.query)
			if status != http.StatusOK || len(items) != tt.count || hasMore != tt.hasMore {
				t.Errorf("%s: status = %d, %d 件, X-Has-More = %q, want %d 件、%s", tt.query, status, len(items), hasMore, tt.count, tt.hasMore)
			}
		}
	})

	t.Run("page・per_page が整数でなければ、既存の一覧と同じ 422 になる", func(t *testing.T) {
		_, _, status, body := list("?page=abc&per_page=x")
		if status != http.StatusUnprocessableEntity || body != `{"errors":["Page must be an integer","Per page must be an integer"]}` {
			t.Errorf("status = %d, body = %s", status, body)
		}
	})

	t.Run("取り消した許可は、どのページにも現れず、続きのページが 1 件ぶん詰まる", func(t *testing.T) {
		items, _, _, _ := list("")
		revoked := items[0].ID
		if rec := do(k.router, http.MethodDelete, "/oauth/grants/"+revoked, "", k.bearer[k.alice]); rec.Code != http.StatusNoContent {
			t.Fatalf("DELETE = %d %s", rec.Code, rec.Body.String())
		}
		total := 0
		for page := 1; page <= 3; page++ {
			got, hasMore, _, _ := list("?page=" + strconv.Itoa(page))
			for _, it := range got {
				if it.ID == revoked {
					t.Errorf("取り消した許可が page %d に残っている", page)
				}
			}
			total += len(got)
			if want := page < 3; (hasMore == "true") != want {
				t.Errorf("page %d の X-Has-More = %q", page, hasMore)
			}
		}
		if total != 44 {
			t.Errorf("取り消し後の合計 = %d 件, want 44", total)
		}
	})
}
