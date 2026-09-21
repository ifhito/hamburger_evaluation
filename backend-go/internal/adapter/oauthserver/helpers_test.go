package oauthserver_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/oauthserver"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/uow"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
)

const (
	testIssuer      = "http://localhost:8080"
	testResource    = "http://localhost:8080/mcp"
	testConsentURL  = "http://localhost:5173/oauth/authorize"
	staticClientID  = "dev-app"
	staticRedirect  = "http://127.0.0.1/callback"
	httpsRedirect   = "https://app.example.com/callback"
	metadataURL     = "https://client.example.com/oauth/client.json"
	metadataRedirct = "https://client.example.com/callback"
)

// fakeFetcher は、アプリの説明の文書を、あらかじめ決めた内容で返すテスト用の取得役である。
type fakeFetcher struct {
	mu    sync.Mutex
	docs  map[string]string
	err   error
	calls int
}

func (f *fakeFetcher) Fetch(_ context.Context, clientIDURL string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	doc, ok := f.docs[clientIDURL]
	if !ok {
		return nil, errors.New("not found")
	}
	return []byte(doc), nil
}

// rig は、実際の PostgreSQL に対する認可サーバーと、許可済みの利用者をまとめたテスト用の道具である。
type rig struct {
	t       *testing.T
	ctx     context.Context
	pool    *pgxpool.Pool
	conn    *pgx.Conn
	srv     *oauthserver.Server
	fetcher *fakeFetcher
	userID  string
	grants  *domain.OAuthGrants
}

func newRig(t *testing.T) *rig {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping DB-backed test in short mode")
	}
	ctx := context.Background()
	conn, url := dbtest.New(t)
	userID := dbtest.InsertUserRow(ctx, t, conn,
		`INSERT INTO users (email, username, password_digest) VALUES ('alice@example.com', 'alice', 'digest') RETURNING id`)
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	fetcher := &fakeFetcher{docs: map[string]string{}}
	srv, err := oauthserver.New(oauthserver.Config{
		Issuer:     testIssuer,
		Resource:   testResource,
		ConsentURL: testConsentURL,
		Secret:     []byte("0123456789abcdef0123456789abcdef"),
		StaticClients: []domain.OAuthClient{{
			ID: staticClientID, Name: "Dev App", RedirectURIs: []string{staticRedirect, httpsRedirect},
		}},
	}, oauthserver.Deps{
		Sessions: uow.NewOAuthTokenSessionStore(pool),
		Grants:   query.NewOAuthGrantQuery(pool),
		Users:    query.NewUserQuery(pool),
		Fetcher:  fetcher,
	})
	if err != nil {
		t.Fatalf("oauthserver.New: %v", err)
	}
	return &rig{
		t: t, ctx: ctx, pool: pool, conn: conn, srv: srv, fetcher: fetcher, userID: userID,
		grants: domain.NewOAuthGrants(repository.NewOAuthGrantRepository(pool)),
	}
}

// approve は、利用者が client に scopes を許可した記録を作り、その id を返す。
func (r *rig) approve(clientID, name string, scopes ...string) string {
	r.t.Helper()
	id, err := r.grants.Approve(r.ctx, r.userID, domain.OAuthClient{ID: clientID, Name: name}, scopes)
	if err != nil {
		r.t.Fatalf("approve: %v", err)
	}
	return id
}

// pkcePair は、PKCE の確認用の文字列(verifier)と、そこから作る、認可の要求に付ける値(challenge)を返す。
func pkcePair() (verifier, challenge string) {
	verifier = "test-verifier-0123456789-abcdefghijklmnopqrstuvwxyz"
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

// authParams は、認可の要求の値を作る。overrides の値で上書きし、値が空文字のキーは取り除く。
func authParams(clientID, redirect, challenge string, overrides ...string) url.Values {
	v := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirect},
		"scope":                 {domain.OAuthScopeRead},
		"state":                 {"state-1234567890"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"resource":              {testResource},
	}
	for i := 0; i+1 < len(overrides); i += 2 {
		if overrides[i+1] == "" {
			v.Del(overrides[i])
		} else {
			v.Set(overrides[i], overrides[i+1])
		}
	}
	return v
}

// issue は、許可済みの利用者の認可の要求に、認可コードを発行し、アプリへ戻す URL を返す。
func (r *rig) issue(params url.Values, grantID string) *url.URL {
	r.t.Helper()
	loc, err := r.srv.IssueAuthorizationCode(r.ctx, params, r.userID, grantID)
	if err != nil {
		r.t.Fatalf("IssueAuthorizationCode: %v", err)
	}
	u, err := url.Parse(loc)
	if err != nil {
		r.t.Fatalf("parse redirect %q: %v", loc, err)
	}
	return u
}

// issueCode は、認可コードを発行して、その値だけを返す。
func (r *rig) issueCode(params url.Values, grantID string) string {
	r.t.Helper()
	code := r.issue(params, grantID).Query().Get("code")
	if code == "" {
		r.t.Fatal("認可コードが発行されなかった")
	}
	return code
}

// tokenResponse は、トークンの窓口の応答である。
type tokenResponse struct {
	Status int
	Header http.Header
	Body   map[string]any
}

func (t tokenResponse) str(key string) string {
	s, _ := t.Body[key].(string)
	return s
}

func (r *rig) postToken(form url.Values) tokenResponse {
	r.t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	r.srv.HandleToken(rec, req)
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		r.t.Fatalf("token response is not JSON: %v (%s)", err, rec.Body.String())
	}
	return tokenResponse{Status: rec.Code, Header: rec.Header(), Body: body}
}

// exchange は、認可コードを、PKCE の verifier とともにトークンに交換する。
func (r *rig) exchange(clientID, redirect, code, verifier string) tokenResponse {
	r.t.Helper()
	return r.postToken(url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirect},
		"client_id": {clientID}, "code_verifier": {verifier},
	})
}

func (r *rig) refresh(clientID, refreshToken string) tokenResponse {
	r.t.Helper()
	return r.postToken(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refreshToken}, "client_id": {clientID}})
}

// tokens は、許可 → 認可コードの発行 → 交換までを行い、発行されたトークンを返す。
func (r *rig) tokens(scopes ...string) (access, refresh string, resp tokenResponse) {
	r.t.Helper()
	grantID := r.approve(staticClientID, "Dev App", scopes...)
	verifier, challenge := pkcePair()
	code := r.issueCode(authParams(staticClientID, staticRedirect, challenge, "scope", strings.Join(scopes, " ")), grantID)
	resp = r.exchange(staticClientID, staticRedirect, code, verifier)
	if resp.Status != http.StatusOK {
		r.t.Fatalf("exchange status = %d, body = %v", resp.Status, resp.Body)
	}
	return resp.str("access_token"), resp.str("refresh_token"), resp
}

func (r *rig) count(sql string, args ...any) int {
	r.t.Helper()
	var n int
	if err := r.pool.QueryRow(r.ctx, sql, args...).Scan(&n); err != nil {
		r.t.Fatalf("count %q: %v", sql, err)
	}
	return n
}
