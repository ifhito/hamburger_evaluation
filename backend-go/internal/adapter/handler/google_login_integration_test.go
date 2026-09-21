package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/googleauth"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/handler"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/infra"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/storage"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/uow"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/fakeoidc"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

const (
	googleTestClientID     = "google-test-client"
	googleTestClientSecret = "google-test-secret-value" // テスト用の使い捨ての値(本物の秘密ではない)
	googleTestRedirect     = "http://localhost:8080/auth/google/callback"
	googleTestAppBase      = "http://localhost:5173"
)

// googleKit は、本物の PostgreSQL と、OpenID Connect の提供元の代役をつないで、Google でのサインインを
// HTTP の入口から確かめるための一式である。
type googleKit struct {
	pool   *pgxpool.Pool
	conn   *pgx.Conn
	router http.Handler
	idp    *fakeoidc.Server
	codec  *infra.JWTCodec
	alice  string // パスワードでサインインできる利用者(alice@example.com)
	bob    string // パスワードでサインインできる利用者(bob@example.com)
	logs   *bytes.Buffer
}

func newGoogleKit(t *testing.T) *googleKit {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping DB-backed test in short mode")
	}
	ctx := context.Background()
	conn, dbURL := dbtest.New(t)
	alice := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('alice@example.com', 'alice', 'digest:Password123!') RETURNING id`)
	bob := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('bob@example.com', 'bob', 'digest:Password123!') RETURNING id`)
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	idp := fakeoidc.New(t, googleTestClientID, googleTestClientSecret)
	idp.SetUser(fakeoidc.User{Sub: "sub-carol", Email: "carol@gmail.example", EmailVerified: true, Name: "Carol Google"})

	codec := infra.NewJWTCodec(testJWTSecret, time.Hour)
	userQuery := query.NewUserQuery(pool)
	unitOfWork := uow.New(pool)
	logins := usecase.NewGoogleLogins(
		googleauth.New(googleauth.Config{ClientID: googleTestClientID, ClientSecret: googleTestClientSecret, RedirectURL: googleTestRedirect, Issuer: idp.URL}),
		query.NewUserIdentityQuery(pool), userQuery, unitOfWork,
		domain.NewLoginHandoffs(repository.NewLoginHandoffRepository(pool)),
		domain.NewUserIdentities(repository.NewUserIdentityRepository(pool)),
		codec,
	)
	google, err := handler.NewGoogleLogin(logins, handler.GoogleLoginConfig{AppBaseURL: googleTestAppBase, RedirectURL: googleTestRedirect, CookieSecret: testJWTSecret})
	if err != nil {
		t.Fatal(err)
	}
	recalc := usecase.NewBurgerStatsRecalculator(infra.SystemClock{})
	router := handler.NewRouter(pool, usecase.NewAuth(userQuery, hasherFake{}, codec, codec), unusedSignups(),
		usecase.NewShops(query.NewShopQuery(pool), domain.NewShops(repository.NewShopRepository(pool))),
		usecase.NewReviews(query.NewReviewQuery(pool), unitOfWork, recalc, storage.NewDisk(t.TempDir(), "/photos")),
		usecase.NewUsers(userQuery, domain.NewUsers(repository.NewUserRepository(pool)), unitOfWork, recalc, hasherFake{}),
		nil, nil, nil, handler.WithGoogleLogin(google))

	// ログの出力を捕まえて、秘密の値が出ていないことを確かめられるようにする。
	logs := &bytes.Buffer{}
	log.SetOutput(logs)
	t.Cleanup(func() { log.SetOutput(nil) })
	return &googleKit{pool: pool, conn: conn, router: router, idp: idp, codec: codec, alice: alice, bob: bob, logs: logs}
}

func (k *googleKit) bearer(t *testing.T, userID string) string {
	t.Helper()
	tok, err := k.codec.Issue(userID)
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + tok
}

// flow は、1 回の手続きの各段階の応答である。
type flow struct {
	start    *httptest.ResponseRecorder
	callback *httptest.ResponseRecorder
	// code は、コールバックが frontend へ渡した「画面へ渡すコード」である(なければ空)。
	code string
	// authorizeLocation は、代役の認可の画面が、利用者を戻した URL(コールバックの URL)である。
	authorizeLocation string
}

func noRedirect() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// run は、手続きを最後まで進める: 開始 → 代役の認可の画面(自動で承認) → コールバック。mutate は、コールバックの
// 要求の query を書き換える(state の改ざんなど)。cookie に nil を返す関数を渡すと、cookie なしで戻る。
func (k *googleKit) run(t *testing.T, startQuery string, opts ...func(*runOpts)) flow {
	t.Helper()
	o := runOpts{cookie: func(c *http.Cookie) *http.Cookie { return c }, query: func(q url.Values) {}}
	for _, opt := range opts {
		opt(&o)
	}
	var f flow
	f.start = do(k.router, http.MethodGet, "/auth/google/start"+startQuery, "", "")
	if f.start.Code != http.StatusFound {
		// 始められなかった: frontend の結果の画面へ、コードなしで送られる。
		return f
	}
	authURL := f.start.Header().Get("Location")
	resp, err := noRedirect().Get(authURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	back, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || back.Path != "/auth/google/callback" {
		t.Fatalf("代役の認可の画面が、コールバックへ戻さなかった: %q", resp.Header.Get("Location"))
	}
	q := back.Query()
	o.query(q)
	f.authorizeLocation = back.String()
	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?"+q.Encode(), nil)
	for _, c := range f.start.Result().Cookies() {
		if c = o.cookie(c); c != nil {
			req.AddCookie(c)
		}
	}
	f.callback = httptest.NewRecorder()
	k.router.ServeHTTP(f.callback, req)
	if f.callback.Code != http.StatusSeeOther {
		t.Fatalf("コールバックが 303 で frontend へ戻さなかった: %d", f.callback.Code)
	}
	dest, err := url.Parse(f.callback.Header().Get("Location"))
	if err != nil || !strings.HasPrefix(dest.String(), googleTestAppBase+"/auth/google/complete") {
		t.Fatalf("frontend の結果の画面へ戻していない: %q", f.callback.Header().Get("Location"))
	}
	f.code = dest.Query().Get("code")
	return f
}

type runOpts struct {
	cookie func(*http.Cookie) *http.Cookie
	query  func(url.Values)
}

func withQuery(fn func(url.Values)) func(*runOpts) { return func(o *runOpts) { o.query = fn } }
func withCookie(fn func(*http.Cookie) *http.Cookie) func(*runOpts) {
	return func(o *runOpts) { o.cookie = fn }
}

func (k *googleKit) exchange(code string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"code": code})
	return do(k.router, http.MethodPost, "/auth/google/exchange", string(body), "")
}

func (k *googleKit) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := k.conn.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

type exchangeBody struct {
	ID       string   `json:"id"`
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Admin    bool     `json:"admin"`
	Token    string   `json:"token"`
	ReturnTo string   `json:"return_to"`
	Linked   bool     `json:"linked"`
	Errors   []string `json:"errors"`
}

func decodeExchange(t *testing.T, rec *httptest.ResponseRecorder) exchangeBody {
	t.Helper()
	var b exchangeBody
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("exchange body %q: %v", rec.Body.String(), err)
	}
	return b
}

// requireNoSecrets は、ログと、手続きの各 URL に、秘密の値(クライアントの秘密・JWT・ID トークン・画面へ渡すコード)が
// 含まれていないことを確かめる。
func (k *googleKit) requireNoSecrets(t *testing.T, f flow, jwt string) {
	t.Helper()
	haystacks := map[string]string{
		"ログ": k.logs.String(), "開始の Location": f.start.Header().Get("Location"),
		"コールバックの Location": f.callback.Header().Get("Location"),
	}
	for where, text := range haystacks {
		for name, secret := range map[string]string{"クライアントの秘密": googleTestClientSecret, "JWT": jwt, "ID トークン": "eyJhbGciOi"} {
			if secret != "" && strings.Contains(text, secret) {
				t.Errorf("%s に%sが含まれている", where, name)
			}
		}
		if f.code != "" && where == "ログ" && strings.Contains(text, f.code) {
			t.Errorf("ログに「画面へ渡すコード」が含まれている")
		}
	}
}

func TestGoogleSignIn(t *testing.T) {
	t.Run("初めての Google アカウントは、パスワードなしで新規登録され、コードの交換でサインインでき、GET /me が通る", func(t *testing.T) {
		k := newGoogleKit(t)
		f := k.run(t, "?return_to=/shops")
		if f.code == "" {
			t.Fatal("コードが渡されなかった")
		}
		rec := k.exchange(f.code)
		if rec.Code != http.StatusOK {
			t.Fatalf("交換 = %d: %s", rec.Code, rec.Body)
		}
		body := decodeExchange(t, rec)
		if body.Email != "carol@gmail.example" || body.Username != "Carol Google" || body.Admin || body.Token == "" || body.ReturnTo != "/shops" {
			t.Fatalf("body = %+v", body)
		}
		if me := do(k.router, http.MethodGet, "/me", "", "Bearer "+body.Token); me.Code != http.StatusOK {
			t.Fatalf("GET /me = %d", me.Code)
		}
		var digestIsNull bool
		if err := k.conn.QueryRow(context.Background(), `SELECT password_digest IS NULL FROM users WHERE email = 'carol@gmail.example'`).Scan(&digestIsNull); err != nil || !digestIsNull {
			t.Fatalf("パスワードなしで作られていない: %v %v", digestIsNull, err)
		}
		if n := k.count(t, "user_identities"); n != 1 {
			t.Fatalf("結び付きが %d 件", n)
		}
		k.requireNoSecrets(t, f, body.Token)
		if f.start.Header().Get("Cache-Control") != "no-store" || f.callback.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("秘密を含む応答が、キャッシュ・参照元の対策をしていない: %v", f.callback.Header())
		}
	})

	t.Run("2 回目以降は、同じ利用者としてサインインし、新しい利用者は作られない", func(t *testing.T) {
		k := newGoogleKit(t)
		first := decodeExchange(t, k.exchange(k.run(t, "").code))
		second := decodeExchange(t, k.exchange(k.run(t, "").code))
		if first.ID == "" || first.ID != second.ID {
			t.Fatalf("利用者が違う: %q %q", first.ID, second.ID)
		}
		if n := k.count(t, "users"); n != 3 { // alice・bob・carol
			t.Fatalf("利用者が %d 人", n)
		}
	})

	t.Run("同じメールの既存アカウントがあるときは、大文字小文字が違っても、連携もサインインもせず、案内のエラーを返す", func(t *testing.T) {
		for _, email := range []string{"alice@example.com", "ALICE@Example.COM"} {
			k := newGoogleKit(t)
			k.idp.SetUser(fakeoidc.User{Sub: "sub-attacker", Email: email, EmailVerified: true, Name: "Alice Impostor"})
			rec := k.exchange(k.run(t, "").code)
			body := decodeExchange(t, rec)
			if rec.Code != http.StatusConflict || body.Token != "" || len(body.Errors) != 1 || !strings.Contains(body.Errors[0], "already exists") {
				t.Fatalf("%s: %d %s", email, rec.Code, rec.Body)
			}
			if n := k.count(t, "user_identities"); n != 0 {
				t.Errorf("%s: 結び付きが作られた", email)
			}
			if n := k.count(t, "users"); n != 2 {
				t.Errorf("%s: 利用者が増えた(%d 人)", email, n)
			}
		}
	})

	t.Run("state が違う・cookie がない・cookie が改ざんされている・期限切れの手続きは、失敗になり、何も作られない", func(t *testing.T) {
		cases := map[string][]func(*runOpts){
			"state の改ざん": {withQuery(func(q url.Values) { q.Set("state", "forged-state") })},
			"state がない":  {withQuery(func(q url.Values) { q.Del("state") })},
			"cookie がない": {withCookie(func(*http.Cookie) *http.Cookie { return nil })},
			"cookie の改ざん": {withCookie(func(c *http.Cookie) *http.Cookie {
				c.Value = c.Value[:len(c.Value)-2] + "AA"
				return c
			})},
			"別の秘密で封じた cookie": {withCookie(func(c *http.Cookie) *http.Cookie { c.Value = "garbage"; return c })},
			"コードがない":          {withQuery(func(q url.Values) { q.Del("code") })},
		}
		for name, opts := range cases {
			t.Run(name, func(t *testing.T) {
				k := newGoogleKit(t)
				f := k.run(t, "", opts...)
				rec := k.exchange(f.code)
				if rec.Code != http.StatusBadRequest || decodeExchange(t, rec).Token != "" {
					t.Fatalf("%d %s", rec.Code, rec.Body)
				}
				if k.count(t, "users") != 2 || k.count(t, "user_identities") != 0 {
					t.Fatal("失敗したのに、利用者か結び付きが作られた")
				}
			})
		}
	})

	t.Run("ID トークンの検証に失敗する手続き(宛先・発行者・期限・署名・nonce・メール未確認)は、失敗になり、何も作られない", func(t *testing.T) {
		cases := map[string]func(*fakeoidc.Server){
			"宛先が別のクライアント": func(i *fakeoidc.Server) { i.SetTweaks(fakeoidc.Tweaks{Aud: "another-client"}) },
			"発行者が違う":      func(i *fakeoidc.Server) { i.SetTweaks(fakeoidc.Tweaks{Iss: "https://evil.example"}) },
			"期限切れ":        func(i *fakeoidc.Server) { i.SetTweaks(fakeoidc.Tweaks{ExpiredAgo: time.Hour}) },
			"別の鍵の署名":      func(i *fakeoidc.Server) { i.SetTweaks(fakeoidc.Tweaks{WrongKey: true}) },
			"nonce が違う":   func(i *fakeoidc.Server) { i.SetTweaks(fakeoidc.Tweaks{Nonce: "other-nonce"}) },
			"メールが未確認": func(i *fakeoidc.Server) {
				i.SetUser(fakeoidc.User{Sub: "sub-x", Email: "victim@example.com", EmailVerified: false, Name: "X"})
			},
			"利用者が認可を拒否": func(i *fakeoidc.Server) { i.SetTweaks(fakeoidc.Tweaks{DenyAuthorize: true}) },
			"交換がエラー":    func(i *fakeoidc.Server) { i.SetTweaks(fakeoidc.Tweaks{TokenStatus: http.StatusBadGateway}) },
		}
		for name, setup := range cases {
			t.Run(name, func(t *testing.T) {
				k := newGoogleKit(t)
				setup(k.idp)
				f := k.run(t, "")
				rec := k.exchange(f.code)
				body := decodeExchange(t, rec)
				if rec.Code != http.StatusBadRequest || body.Token != "" || len(body.Errors) != 1 || !strings.Contains(body.Errors[0], "failed") {
					t.Fatalf("%d %s", rec.Code, rec.Body)
				}
				if k.count(t, "users") != 2 || k.count(t, "user_identities") != 0 {
					t.Fatal("失敗したのに、利用者か結び付きが作られた")
				}
				k.requireNoSecrets(t, f, "")
			})
		}
	})

	t.Run("画面へ渡すコードは 1 回しか使えず、期限が過ぎると使えず、知らないコードも使えない", func(t *testing.T) {
		k := newGoogleKit(t)
		code := k.run(t, "").code
		if rec := k.exchange(code); rec.Code != http.StatusOK {
			t.Fatalf("1 回目 = %d", rec.Code)
		}
		if rec := k.exchange(code); rec.Code != http.StatusBadRequest {
			t.Fatalf("2 回目 = %d, want 400", rec.Code)
		}
		expiring := k.run(t, "").code
		if _, err := k.conn.Exec(context.Background(), `UPDATE login_handoffs SET expires_at = now() - interval '1 second'`); err != nil {
			t.Fatal(err)
		}
		if rec := k.exchange(expiring); rec.Code != http.StatusBadRequest {
			t.Fatalf("期限後 = %d, want 400", rec.Code)
		}
		for _, bad := range []string{"", "unknown-code"} {
			if rec := k.exchange(bad); rec.Code != http.StatusBadRequest {
				t.Fatalf("コード %q = %d, want 400", bad, rec.Code)
			}
		}
	})

	t.Run("同じコードを並行して交換しても、サインインできるのは 1 回だけである", func(t *testing.T) {
		k := newGoogleKit(t)
		code := k.run(t, "").code
		var ok, rejected atomic.Int32
		var wg sync.WaitGroup
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				switch k.exchange(code).Code {
				case http.StatusOK:
					ok.Add(1)
				case http.StatusBadRequest:
					rejected.Add(1)
				}
			}()
		}
		wg.Wait()
		if ok.Load() != 1 || rejected.Load() != 11 {
			t.Fatalf("成功 %d・拒否 %d, want 成功 1・拒否 11", ok.Load(), rejected.Load())
		}
	})

	t.Run("退会済みの利用者に結び付いた Google アカウントは、サインインできない", func(t *testing.T) {
		k := newGoogleKit(t)
		first := decodeExchange(t, k.exchange(k.run(t, "").code))
		if _, err := k.conn.Exec(context.Background(), `UPDATE users SET discarded_at = now() WHERE id = $1`, first.ID); err != nil {
			t.Fatal(err)
		}
		rec := k.exchange(k.run(t, "").code)
		if rec.Code != http.StatusBadRequest || decodeExchange(t, rec).Token != "" {
			t.Fatalf("退会済みなのにサインインできた: %d %s", rec.Code, rec.Body)
		}
		if k.count(t, "users") != 3 {
			t.Fatal("退会済みの利用者のために、新しい利用者が作られた")
		}
	})

	t.Run("パスワードなしのアカウントが、パスワードでサインインしようとすると、知らないメールと同じ失敗(文言・ステータス)になる", func(t *testing.T) {
		k := newGoogleKit(t)
		decodeExchange(t, k.exchange(k.run(t, "").code))
		passwordless := do(k.router, http.MethodPost, "/login", `{"email":"carol@gmail.example","password":"Password123!"}`, "")
		unknown := do(k.router, http.MethodPost, "/login", `{"email":"nobody@example.com","password":"Password123!"}`, "")
		if passwordless.Code != http.StatusUnauthorized || passwordless.Code != unknown.Code || passwordless.Body.String() != unknown.Body.String() {
			t.Fatalf("パスワードなし: %d %s / 知らないメール: %d %s", passwordless.Code, passwordless.Body, unknown.Code, unknown.Body)
		}
	})

	t.Run("戻り先は、アプリの中のパスだけが渡され、外部の URL などは既定(空)になる", func(t *testing.T) {
		cases := map[string]string{
			"/shops": "/shops", "/oauth/authorize?client_id=x&state=y": "/oauth/authorize?client_id=x&state=y",
			"https://evil.example": "", "//evil.example": "", `/\evil.example`: "", "javascript:alert(1)": "", "": "",
		}
		for returnTo, want := range cases {
			k := newGoogleKit(t)
			body := decodeExchange(t, k.exchange(k.run(t, "?return_to="+url.QueryEscape(returnTo)).code))
			if body.ReturnTo != want {
				t.Errorf("return_to %q → %q, want %q", returnTo, body.ReturnTo, want)
			}
		}
	})

	t.Run("手続きの cookie は、HttpOnly・SameSite=Lax・戻り先の path に限られ、値に平文の秘密を含まず、コールバックで消される", func(t *testing.T) {
		k := newGoogleKit(t)
		f := k.run(t, "")
		var flowCookie *http.Cookie
		for _, c := range f.start.Result().Cookies() {
			if c.Name == "google_login_flow" {
				flowCookie = c
			}
		}
		if flowCookie == nil || !flowCookie.HttpOnly || flowCookie.SameSite != http.SameSiteLaxMode || flowCookie.Path != "/auth/google/callback" || flowCookie.MaxAge != 600 || flowCookie.Secure {
			t.Fatalf("cookie = %+v", flowCookie)
		}
		authURL, _ := url.Parse(f.start.Header().Get("Location"))
		if state := authURL.Query().Get("state"); strings.Contains(flowCookie.Value, state) {
			t.Error("cookie の値に、平文の state が含まれている")
		}
		var cleared bool
		for _, c := range f.callback.Result().Cookies() {
			cleared = cleared || (c.Name == "google_login_flow" && c.MaxAge < 0)
		}
		if !cleared {
			t.Error("コールバックが cookie を消していない")
		}
	})
}

func TestGoogleLinking(t *testing.T) {
	linkFlow := func(t *testing.T, k *googleKit, userID string, opts ...func(*runOpts)) (flow, string) {
		t.Helper()
		rec := do(k.router, http.MethodPost, "/me/identities/google/link", "", k.bearer(t, userID))
		if rec.Code != http.StatusOK {
			t.Fatalf("結び付けの開始 = %d: %s", rec.Code, rec.Body)
		}
		var body struct {
			LinkCode string `json:"link_code"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return k.run(t, "?return_to=/profile&link_code="+url.QueryEscape(body.LinkCode), opts...), body.LinkCode
	}

	t.Run("ログイン済みの利用者が結び付けると、その Google アカウントでサインインでき、一覧に出る", func(t *testing.T) {
		k := newGoogleKit(t)
		f, _ := linkFlow(t, k, k.alice)
		rec := k.exchange(f.code)
		body := decodeExchange(t, rec)
		if rec.Code != http.StatusOK || !body.Linked || body.ReturnTo != "/profile" || body.Token != "" {
			t.Fatalf("交換 = %d %s", rec.Code, rec.Body)
		}
		list := do(k.router, http.MethodGet, "/me/identities", "", k.bearer(t, k.alice))
		var got struct {
			Identities []struct {
				Provider  string `json:"provider"`
				Email     string `json:"email"`
				CanUnlink bool   `json:"can_unlink"`
			} `json:"identities"`
		}
		_ = json.Unmarshal(list.Body.Bytes(), &got)
		if len(got.Identities) != 1 || got.Identities[0].Provider != "google" || got.Identities[0].Email != "carol@gmail.example" || !got.Identities[0].CanUnlink {
			t.Fatalf("一覧 = %s", list.Body)
		}
		signIn := decodeExchange(t, k.exchange(k.run(t, "").code))
		if signIn.ID != k.alice || signIn.Token == "" {
			t.Fatalf("結び付けた Google で、alice にサインインできない: %+v", signIn)
		}
	})

	t.Run("結び付けの開始のコードは、1 回しか使えず、サインインの結果としては交換できない", func(t *testing.T) {
		k := newGoogleKit(t)
		rec := do(k.router, http.MethodPost, "/me/identities/google/link", "", k.bearer(t, k.alice))
		var body struct {
			LinkCode string `json:"link_code"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if ex := k.exchange(body.LinkCode); ex.Code != http.StatusBadRequest || decodeExchange(t, ex).Token != "" {
			t.Fatalf("開始のコードを、サインインの結果として交換できた: %d %s", ex.Code, ex.Body)
		}
		// 上の交換で、コードは消えている(用途が違っても、使った時点で消える)。
		if start := do(k.router, http.MethodGet, "/auth/google/start?link_code="+url.QueryEscape(body.LinkCode), "", ""); start.Code != http.StatusSeeOther {
			t.Fatalf("使用済みの開始のコードで、手続きが始まった: %d", start.Code)
		}
	})

	t.Run("サインインの結果のコードは、結び付けの開始のコードとしては使えない", func(t *testing.T) {
		k := newGoogleKit(t)
		f := k.run(t, "")
		if start := do(k.router, http.MethodGet, "/auth/google/start?link_code="+url.QueryEscape(f.code), "", ""); start.Code != http.StatusSeeOther {
			t.Fatalf("サインインの結果のコードで、結び付けの手続きが始まった: %d", start.Code)
		}
	})

	t.Run("別の利用者に結び付け済みの Google アカウントは、結び付けられず、すでに Google と結び付いた利用者も、別の Google を足せない", func(t *testing.T) {
		k := newGoogleKit(t)
		if body := decodeExchange(t, k.exchange(mustCode(linkFlow(t, k, k.bob)))); !body.Linked {
			t.Fatal("bob の結び付けに失敗した")
		}
		taken := k.exchange(mustCode(linkFlow(t, k, k.alice)))
		if taken.Code != http.StatusConflict || !strings.Contains(taken.Body.String(), "another account") {
			t.Fatalf("別の利用者の Google = %d %s", taken.Code, taken.Body)
		}
		k.idp.SetUser(fakeoidc.User{Sub: "sub-other", Email: "other@gmail.example", EmailVerified: true, Name: "Other"})
		already := k.exchange(mustCode(linkFlow(t, k, k.bob)))
		if already.Code != http.StatusConflict || !strings.Contains(already.Body.String(), "already connected") {
			t.Fatalf("すでに結び付いた利用者 = %d %s", already.Code, already.Body)
		}
		if n := k.count(t, "user_identities"); n != 1 {
			t.Fatalf("結び付きが %d 件", n)
		}
	})

	t.Run("同じ Google アカウントを、同じ利用者が結び付け直しても、成功し、増えない(繰り返しの操作)", func(t *testing.T) {
		k := newGoogleKit(t)
		k.exchange(mustCode(linkFlow(t, k, k.alice)))
		again := k.exchange(mustCode(linkFlow(t, k, k.alice)))
		if again.Code != http.StatusOK || !decodeExchange(t, again).Linked || k.count(t, "user_identities") != 1 {
			t.Fatalf("%d %s", again.Code, again.Body)
		}
	})

	t.Run("パスワードのある利用者は解除でき、そのあともパスワードでサインインできる", func(t *testing.T) {
		k := newGoogleKit(t)
		k.exchange(mustCode(linkFlow(t, k, k.alice)))
		if del := do(k.router, http.MethodDelete, "/me/identities/google", "", k.bearer(t, k.alice)); del.Code != http.StatusNoContent {
			t.Fatalf("解除 = %d %s", del.Code, del.Body)
		}
		if n := k.count(t, "user_identities"); n != 0 {
			t.Fatalf("結び付きが %d 件残っている", n)
		}
		if login := do(k.router, http.MethodPost, "/login", `{"email":"alice@example.com","password":"Password123!"}`, ""); login.Code != http.StatusOK {
			t.Fatalf("解除後のパスワードでのサインイン = %d", login.Code)
		}
		if del := do(k.router, http.MethodDelete, "/me/identities/google", "", k.bearer(t, k.alice)); del.Code != http.StatusNotFound {
			t.Fatalf("結び付きがないときの解除 = %d, want 404", del.Code)
		}
	})

	t.Run("パスワードなしのアカウントは、サインインする方法がなくなるので、解除できず、一覧でも解除できないと示される", func(t *testing.T) {
		k := newGoogleKit(t)
		carol := decodeExchange(t, k.exchange(k.run(t, "").code))
		auth := "Bearer " + carol.Token
		del := do(k.router, http.MethodDelete, "/me/identities/google", "", auth)
		if del.Code != http.StatusUnprocessableEntity || !strings.Contains(del.Body.String(), "only way to sign in") {
			t.Fatalf("解除 = %d %s", del.Code, del.Body)
		}
		if n := k.count(t, "user_identities"); n != 1 {
			t.Fatalf("拒否したのに、結び付きが %d 件になった", n)
		}
		list := do(k.router, http.MethodGet, "/me/identities", "", auth)
		if !strings.Contains(list.Body.String(), `"can_unlink":false`) {
			t.Fatalf("一覧 = %s", list.Body)
		}
	})

	t.Run("結び付けの API は、ログインが要る(なければ 401)", func(t *testing.T) {
		k := newGoogleKit(t)
		for _, tc := range []struct{ method, path string }{
			{http.MethodPost, "/me/identities/google/link"}, {http.MethodGet, "/me/identities"}, {http.MethodDelete, "/me/identities/google"},
		} {
			if rec := do(k.router, tc.method, tc.path, "", ""); rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s = %d, want 401", tc.method, tc.path, rec.Code)
			}
		}
	})
}

func mustCode(f flow, _ string) string { return f.code }

func TestGoogleLoginDisabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed test in short mode")
	}
	router := handler.NewRouter(okPinger, nil, unusedSignups(), nil, nil, nil, nil, nil, nil)
	for _, path := range []string{"/auth/google/start", "/auth/google/callback", "/auth/google/exchange", "/me/identities"} {
		if rec := do(router, http.MethodGet, path, "", ""); rec.Code != http.StatusNotFound {
			t.Errorf("Google が無効なとき、%s = %d, want 404", path, rec.Code)
		}
	}
	meta := do(router, http.MethodGet, "/meta", "", "")
	var body struct {
		LoginProviders []string `json:"login_providers"`
	}
	_ = json.Unmarshal(meta.Body.Bytes(), &body)
	if body.LoginProviders == nil || len(body.LoginProviders) != 0 {
		t.Fatalf("無効なときの login_providers = %v, want 空の配列", body.LoginProviders)
	}
}

func TestGoogleLoginProvidersInMeta(t *testing.T) {
	k := newGoogleKit(t)
	meta := do(k.router, http.MethodGet, "/meta", "", "")
	var body struct {
		LoginProviders []string `json:"login_providers"`
	}
	_ = json.Unmarshal(meta.Body.Bytes(), &body)
	if len(body.LoginProviders) != 1 || body.LoginProviders[0] != "google" {
		t.Fatalf("有効なときの login_providers = %v", body.LoginProviders)
	}
}
