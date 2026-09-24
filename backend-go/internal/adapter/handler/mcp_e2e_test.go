package handler_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"

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

// このファイルは、本物の PostgreSQL・本物の認可サーバー(発行したトークン)・本物の /mcp を、1 つの
// HTTP サーバーにつないで確かめる。代役のテスト(mcp_test.go)が確かめない、つなぎ目(認可サーバーが
// 発行したトークンを /mcp が受け付けること、許可の取り消しや退会がすぐ効くこと)を見る。

const (
	e2eClientID = "e2e-app"
	e2eRedirect = "http://127.0.0.1/callback"
)

type mcpE2E struct {
	url, resource string
	oauth         *oauthserver.Server
	grants        *domain.OAuthGrants
	aliceID       string
	aliceJWT      string
}

func newMCPE2E(t *testing.T) *mcpE2E {
	t.Helper()
	ctx := context.Background()
	conn, dbURL := dbtest.New(t)
	aliceID := dbtest.InsertUserRow(ctx, t, conn,
		`INSERT INTO users (email, username, password_digest) VALUES ('alice@example.com', 'alice', 'digest') RETURNING id`)
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	srv := httptest.NewUnstartedServer(nil)
	e := &mcpE2E{url: "http://" + srv.Listener.Addr().String(), aliceID: aliceID}
	e.resource = e.url + "/mcp"

	userQuery := query.NewUserQuery(pool)
	hasher := infra.BcryptPasswordHasher{}
	codec := infra.NewJWTCodec(testJWTSecret, time.Hour)
	unitOfWork := uow.New(pool)
	recalc := usecase.NewBurgerStatsRecalculator(infra.SystemClock{})
	shopRecalc := usecase.NewShopStatsRecalculator(infra.SystemClock{})
	shops := shopsUsecase(query.NewShopQuery(pool), repository.NewShopRepository(pool))
	reviews := usecase.NewReviews(query.NewReviewQuery(pool), unitOfWork, recalc, shopRecalc, storage.NewDisk(t.TempDir(), "/photos"), infra.SystemClock{})
	users := usecase.NewUsers(userQuery, domain.NewUsers(repository.NewUserRepository(pool)), unitOfWork, recalc, shopRecalc, hasher)

	e.oauth, err = oauthserver.New(oauthserver.Config{
		Issuer:        e.url,
		Resource:      e.resource,
		ConsentURL:    e.url + "/oauth/authorize",
		Secret:        []byte("0123456789abcdef0123456789abcdef"),
		StaticClients: []domain.OAuthClient{{ID: e2eClientID, Name: "E2E App", RedirectURIs: []string{e2eRedirect}}},
	}, oauthserver.Deps{
		Sessions: uow.NewOAuthTokenSessionStore(pool),
		Grants:   query.NewOAuthGrantQuery(pool),
		Users:    userQuery,
		Fetcher:  oauthserver.NewHTTPMetadataFetcher(),
	})
	if err != nil {
		t.Fatalf("oauthserver.New: %v", err)
	}
	e.grants = domain.NewOAuthGrants(repository.NewOAuthGrantRepository(pool))
	mcpServer, err := handler.NewMCPServer(usecase.NewOAuthAccessTokens(e.oauth, userQuery, e.resource), shops, reviews, users,
		handler.MCPConfig{Resource: e.resource, Issuer: e.url, AllowedOrigins: []string{e.url}})
	if err != nil {
		t.Fatalf("new mcp server: %v", err)
	}
	auth := usecase.NewAuth(userQuery, hasher, codec, codec)
	srv.Config.Handler = handler.NewRouter(pool, auth, unusedSignups(), shops, nil, reviews, users, nil, &handler.OAuth{Endpoints: e.oauth}, mcpServer)
	srv.Start()
	t.Cleanup(srv.Close)

	if e.aliceJWT, err = codec.Issue(aliceID); err != nil {
		t.Fatalf("issue jwt: %v", err)
	}
	return e
}

type e2eTokens struct{ Access, Refresh string }

// authorize は、利用者(alice)が scopes を許可したことにして、認可コード + PKCE の流れで、トークンを取得する。
func (e *mcpE2E) authorize(t *testing.T, scopes ...string) e2eTokens {
	t.Helper()
	ctx := context.Background()
	verifier := "e2e-verifier-0123456789-abcdefghijklmnopqrstuvwxyz"
	sum := sha256.Sum256([]byte(verifier))
	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {e2eClientID},
		"redirect_uri":          {e2eRedirect},
		"scope":                 {strings.Join(scopes, " ")},
		"state":                 {"state-1234567890"},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
		"resource":              {e.resource},
	}
	grantID, err := e.grants.Approve(ctx, e.aliceID, domain.OAuthClient{ID: e2eClientID, Name: "E2E App"}, scopes)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	redirect, err := e.oauth.IssueAuthorizationCode(ctx, params, e.aliceID, grantID)
	if err != nil {
		t.Fatalf("issue authorization code: %v", err)
	}
	loc, err := url.Parse(redirect)
	if err != nil || loc.Query().Get("code") == "" {
		t.Fatalf("redirect %q has no code (err %v)", redirect, err)
	}
	return e.token(t, url.Values{
		"grant_type": {"authorization_code"}, "code": {loc.Query().Get("code")}, "redirect_uri": {e2eRedirect},
		"client_id": {e2eClientID}, "code_verifier": {verifier}, "resource": {e.resource},
	})
}

func (e *mcpE2E) token(t *testing.T, form url.Values) e2eTokens {
	t.Helper()
	resp, err := http.PostForm(e.url+"/oauth/token", form)
	if err != nil {
		t.Fatalf("post /oauth/token: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if resp.StatusCode != http.StatusOK || json.Unmarshal(body, &out) != nil || out.AccessToken == "" {
		t.Fatalf("token endpoint = %d %s, want an access token", resp.StatusCode, body)
	}
	return e2eTokens{Access: out.AccessToken, Refresh: out.RefreshToken}
}

func (e *mcpE2E) status(t *testing.T, access string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, e.resource, strings.NewReader(rpcToolsList))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+access)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post /mcp: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func (e *mcpE2E) session(t *testing.T, access string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "e2e", Version: "0"}, nil)
	s, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: e.resource, HTTPClient: &http.Client{Transport: bearerTransport{token: access}},
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestMCPWithARealAuthorizationServer(t *testing.T) {
	t.Run("認可サーバーが発行したトークンで、ショップの申請・レビューの投稿・編集・削除まで通り、範囲が足りないときは断られ、広げれば通る", func(t *testing.T) {
		e := newMCPE2E(t)
		readOnly := e.authorize(t, domain.OAuthScopeRead)

		// 読み取りだけの許可: 読み取りは通り、書き込みのツールは 403 で、足りない範囲が示される。
		rs := e.session(t, readOnly.Access)
		if text, isErr := call(t, rs, "get_meta", nil); isErr || !strings.Contains(text, `"rating"`) {
			t.Fatalf("get_meta with a read grant = %q (isError=%v)", text, isErr)
		}
		req, _ := http.NewRequest(http.MethodPost, e.resource, strings.NewReader(rpcToolCall("submit_shop", `{"name":"通らない店"}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+readOnly.Access)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden || !strings.Contains(resp.Header.Get("WWW-Authenticate"), `scope="hamburger:write"`) {
			t.Fatalf("write with a read grant = %d %q, want 403 asking for hamburger:write", resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
		}

		// UUID の形でない ID は、データベースへ渡さず、存在しないものとして答える(渡すと、データベースが
		// 形式のエラーを返し、利用者には内部エラーに見えてしまう)。
		for tool, arg := range map[string]string{"get_shop": "shop_id", "get_review": "review_id", "get_user": "user_id"} {
			if text, isErr := call(t, rs, tool, map[string]any{arg: "not-a-uuid"}); !isErr || !strings.Contains(text, "not found") {
				t.Errorf("%s(%s=not-a-uuid) = %q (isError=%v), want a not-found failure", tool, arg, text, isErr)
			}
		}

		// 書き込みも許可し直す(範囲を広げる)と、同じアプリが書き込める。
		ws := e.session(t, e.authorize(t, domain.OAuthScopeRead, domain.OAuthScopeWrite).Access)
		text, isErr := call(t, ws, "submit_shop", map[string]any{"name": "申請した店"})
		if isErr {
			t.Fatalf("submit_shop = %q", text)
		}
		var shop struct{ ID string }
		mustJSON(t, text, &shop)

		text, isErr = call(t, ws, "create_review", map[string]any{"shop_id": shop.ID, "burger_name": "テリヤキ", "rating": 4, "comment": "たれが濃くておいしい", "visited_at": "2024-05-01"})
		if isErr {
			t.Fatalf("create_review = %q", text)
		}
		if !strings.Contains(text, `"visited_at":"2024-05-01"`) {
			t.Errorf("create_review with a visited_at = %q, want it to include \"visited_at\":\"2024-05-01\" (same shape as REST)", text)
		}
		var review struct{ ID string }
		mustJSON(t, text, &review)

		// 実食日が YYYY-MM-DD の形式で読めないときは、REST と同じ文言のツールの失敗になり、レビューは
		// 変わらない(usecase を呼ぶ前に、ツールの入口で弾く)。
		if text, isErr = call(t, ws, "update_review", map[string]any{"review_id": review.ID, "rating": 5, "comment": "不正な日付", "visited_at": "not-a-date"}); !isErr || !strings.Contains(text, "Visited at must be in YYYY-MM-DD format") {
			t.Errorf("update_review with a malformed visited_at = %q (isError=%v), want a visited_at format failure", text, isErr)
		}

		if text, isErr = call(t, ws, "update_review", map[string]any{"review_id": review.ID, "rating": 5, "comment": "二回目に食べて、もっと好きになった", "visited_at": "2024-06-15"}); isErr || !strings.Contains(text, "もっと好きになった") || !strings.Contains(text, `"visited_at":"2024-06-15"`) {
			t.Errorf("update_review = %q (isError=%v)", text, isErr)
		}
		if text, isErr = call(t, ws, "get_review", map[string]any{"review_id": review.ID}); isErr || !strings.Contains(text, `"rating":5`) || !strings.Contains(text, `"can_edit":true`) || !strings.Contains(text, `"visited_at":"2024-06-15"`) {
			t.Errorf("get_review = %q (isError=%v)", text, isErr)
		}
		if text, isErr = call(t, ws, "delete_review", map[string]any{"review_id": review.ID}); isErr {
			t.Errorf("delete_review = %q", text)
		}
		if text, isErr = call(t, ws, "get_review", map[string]any{"review_id": review.ID}); !isErr || text != "Review not found" {
			t.Errorf("get_review after delete = %q (isError=%v), want Review not found", text, isErr)
		}
	})

	t.Run("更新トークンで取り直したアクセストークンも /mcp で使える", func(t *testing.T) {
		e := newMCPE2E(t)
		first := e.authorize(t, domain.OAuthScopeRead)
		second := e.token(t, url.Values{
			"grant_type": {"refresh_token"}, "refresh_token": {first.Refresh}, "client_id": {e2eClientID}, "resource": {e.resource},
		})
		if got := e.status(t, second.Access); got != http.StatusOK {
			t.Errorf("the refreshed access token: status = %d, want 200", got)
		}
	})

	t.Run("トークンを取り消す(RFC 7009)と、次の呼び出しから 401 になる", func(t *testing.T) {
		e := newMCPE2E(t)
		tokens := e.authorize(t, domain.OAuthScopeRead)
		if got := e.status(t, tokens.Access); got != http.StatusOK {
			t.Fatalf("before revoke: status = %d, want 200", got)
		}
		resp, err := http.PostForm(e.url+"/oauth/revoke", url.Values{"token": {tokens.Access}, "client_id": {e2eClientID}})
		if err != nil {
			t.Fatalf("post /oauth/revoke: %v", err)
		}
		resp.Body.Close()
		if got := e.status(t, tokens.Access); got != http.StatusUnauthorized {
			t.Errorf("after revoke: status = %d, want 401", got)
		}
	})

	t.Run("許可したアプリを取り消す(プロフィールの接続済みアプリ)と、そのアプリのトークンは、すぐ 401 になる", func(t *testing.T) {
		e := newMCPE2E(t)
		tokens := e.authorize(t, domain.OAuthScopeRead)
		if err := e.grants.RevokeAll(context.Background(), e.aliceID); err != nil {
			t.Fatalf("revoke grants: %v", err)
		}
		if got := e.status(t, tokens.Access); got != http.StatusUnauthorized {
			t.Errorf("after the grant was revoked: status = %d, want 401", got)
		}
	})

	t.Run("持ち主が退会すると、そのトークンは 401 になり、更新トークンでも取り直せない", func(t *testing.T) {
		e := newMCPE2E(t)
		tokens := e.authorize(t, domain.OAuthScopeRead)
		req, _ := http.NewRequest(http.MethodDelete, e.url+"/users/"+e.aliceID, nil)
		req.Header.Set("Authorization", "Bearer "+e.aliceJWT)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("delete user: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("DELETE /users/{id} = %d, want 204", resp.StatusCode)
		}
		if got := e.status(t, tokens.Access); got != http.StatusUnauthorized {
			t.Errorf("after the owner left: status = %d, want 401", got)
		}
		refresh, err := http.PostForm(e.url+"/oauth/token", url.Values{
			"grant_type": {"refresh_token"}, "refresh_token": {tokens.Refresh}, "client_id": {e2eClientID}, "resource": {e.resource},
		})
		if err != nil {
			t.Fatalf("refresh: %v", err)
		}
		refresh.Body.Close()
		if refresh.StatusCode == http.StatusOK {
			t.Errorf("refresh after the owner left = %d, want it refused", refresh.StatusCode)
		}
	})
}
