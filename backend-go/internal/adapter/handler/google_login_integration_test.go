package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
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
	// 画面と同じサイト(画面の /api の転送)を通る、本番と同じ形の戻り先。サーバーが見る path には、/api がない。
	googleTestRedirect = "http://localhost:8080/api/auth/google/callback"
	// browserAPIPrefix は、ブラウザから見た API の path の接頭辞である(転送が取り除く)。
	browserAPIPrefix  = "/api"
	googleTestAppBase = "http://localhost:5173"
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

	// handoffs は、コールバックの応答が、手続きを終えたブラウザに設定した「結び付けの値」の cookie を、コードごとに
	// 覚えておく(そのブラウザが、あとで、そのコードを交換するときに、送るもの)。別のブラウザの交換は、これを使わない(exchangeWith)。
	mu       sync.Mutex
	handoffs map[string][]*http.Cookie
}

// kitOptions は、テストのために、部品を差し替えるための指定である。
type kitOptions struct {
	issuer func(usecase.TokenIssuer) usecase.TokenIssuer
	// txUsers は、トランザクションの中で、ユーザーを読む窓口を包む(DB の一時的なエラーを起こすため)。
	txUsers func(usecase.UserQuery) usecase.UserQuery
	// identityQuery は、結び付きを読む窓口を包む(並行する手続きの、読み取りと書き込みの間に割り込むため)。
	identityQuery func(usecase.IdentityQuery) usecase.IdentityQuery
	// maxConns は、DB の接続プールの上限である(0 なら、既定)。並行する処理が、接続を取り合って止まらないことを確かめるため、小さくする。
	maxConns int32
}

func withIdentityQuery(fn func(usecase.IdentityQuery) usecase.IdentityQuery) func(*kitOptions) {
	return func(o *kitOptions) { o.identityQuery = fn }
}

func withMaxConns(n int32) func(*kitOptions) {
	return func(o *kitOptions) { o.maxConns = n }
}

func withIssuer(fn func(usecase.TokenIssuer) usecase.TokenIssuer) func(*kitOptions) {
	return func(o *kitOptions) { o.issuer = fn }
}

func withTxUsers(fn func(usecase.UserQuery) usecase.UserQuery) func(*kitOptions) {
	return func(o *kitOptions) { o.txUsers = fn }
}

// flakyUnitOfWork は、UnitOfWork の中の、ユーザーを読む窓口を、包んだものに差し替える。
type flakyUnitOfWork struct {
	inner usecase.UnitOfWork
	wrap  func(usecase.UserQuery) usecase.UserQuery
}

func (f flakyUnitOfWork) Do(ctx context.Context, fn func(ctx context.Context, tx usecase.Tx) error) error {
	return f.inner.Do(ctx, func(ctx context.Context, tx usecase.Tx) error {
		tx.UserReads = f.wrap(tx.UserReads)
		return fn(ctx, tx)
	})
}

// flakyIssuer は、最初の failures 回だけ、トークンの発行に失敗する(一時的な失敗の代役)。
type flakyIssuer struct {
	inner    usecase.TokenIssuer
	failures atomic.Int32
}

func (f *flakyIssuer) Issue(userID string) (string, error) {
	if f.failures.Add(-1) >= 0 {
		return "", errors.New("temporary issuer failure")
	}
	return f.inner.Issue(userID)
}

// slowIssuer は、トークンの発行を、少し遅くする(並行する交換を、確実に重ならせる)。
type slowIssuer struct {
	inner usecase.TokenIssuer
	delay time.Duration
}

func (s slowIssuer) Issue(userID string) (string, error) {
	time.Sleep(s.delay)
	return s.inner.Issue(userID)
}

// flakyUsers は、最初の failures 回だけ、利用者の取得(ID)に失敗する(DB の一時的なエラーの代役)。
type flakyUsers struct {
	usecase.UserQuery
	failures atomic.Int32
}

func (f *flakyUsers) GetActiveUserByID(ctx context.Context, id string) (domain.User, error) {
	if f.failures.Add(-1) >= 0 {
		return domain.User{}, errors.New("temporary database failure")
	}
	return f.UserQuery.GetActiveUserByID(ctx, id)
}

// rendezvousIdentities は、結び付きを読む窓口を包んで、「結び付きがない」と返された手続きを、n 件が揃うまで待たせる。
// 並行する手続きが、どちらも「まだ結び付きがない」と見てから、書き込みに進むことを、確実に起こすためにある
// (揃ったあとの読み取りは、待たされない)。
type rendezvousIdentities struct {
	usecase.IdentityQuery
	n       int
	arrived atomic.Int32
	release chan struct{}
	once    sync.Once
}

func newRendezvousIdentities(n int) *rendezvousIdentities {
	return &rendezvousIdentities{n: n, release: make(chan struct{})}
}

func (r *rendezvousIdentities) GetIdentityByProviderUserID(ctx context.Context, provider, subject string) (domain.UserIdentity, error) {
	got, err := r.IdentityQuery.GetIdentityByProviderUserID(ctx, provider, subject)
	if errors.Is(err, domain.ErrIdentityNotFound) {
		if int(r.arrived.Add(1)) >= r.n {
			r.once.Do(func() { close(r.release) })
		}
		select {
		case <-r.release:
		case <-time.After(5 * time.Second):
		}
	}
	return got, err
}

func newGoogleKit(t *testing.T, opts ...func(*kitOptions)) *googleKit {
	t.Helper()
	var ko kitOptions
	for _, opt := range opts {
		opt(&ko)
	}
	if testing.Short() {
		t.Skip("skipping DB-backed test in short mode")
	}
	ctx := context.Background()
	conn, dbURL := dbtest.New(t)
	alice := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('alice@example.com', 'alice', 'digest:Password123!') RETURNING id`)
	bob := dbtest.InsertUserRow(ctx, t, conn, `INSERT INTO users (email, username, password_digest) VALUES ('bob@example.com', 'bob', 'digest:Password123!') RETURNING id`)
	poolCfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	if ko.maxConns > 0 {
		poolCfg.MaxConns = ko.maxConns
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	idp := fakeoidc.New(t, googleTestClientID, googleTestClientSecret)
	idp.SetUser(fakeoidc.User{Sub: "sub-carol", Email: "carol@gmail.example", EmailVerified: true, Name: "Carol Google"})

	codec := infra.NewJWTCodec(testJWTSecret, time.Hour)
	userQuery := query.NewUserQuery(pool)
	unitOfWork := uow.New(pool)
	var loginUoW usecase.UnitOfWork = unitOfWork
	if ko.txUsers != nil {
		loginUoW = flakyUnitOfWork{inner: unitOfWork, wrap: ko.txUsers}
	}
	var loginIssuer usecase.TokenIssuer = codec
	if ko.issuer != nil {
		loginIssuer = ko.issuer(codec)
	}
	var identityQuery usecase.IdentityQuery = query.NewUserIdentityQuery(pool)
	if ko.identityQuery != nil {
		identityQuery = ko.identityQuery(identityQuery)
	}
	logins := usecase.NewGoogleLogins(
		googleauth.New(googleauth.Config{ClientID: googleTestClientID, ClientSecret: googleTestClientSecret, RedirectURL: googleTestRedirect, Issuer: idp.URL}),
		identityQuery, userQuery, loginUoW,
		domain.NewLoginHandoffs(repository.NewLoginHandoffRepository(pool)),
		domain.NewUserIdentities(repository.NewUserIdentityRepository(pool)),
		loginIssuer,
	)
	google, err := handler.NewGoogleLogin(logins, handler.GoogleLoginConfig{AppBaseURL: googleTestAppBase, RedirectURL: googleTestRedirect, CookieSecret: testJWTSecret})
	if err != nil {
		t.Fatal(err)
	}
	recalc := usecase.NewBurgerStatsRecalculator(infra.SystemClock{})
	shopRecalc := usecase.NewShopStatsRecalculator(infra.SystemClock{})
	router := handler.NewRouter(pool, usecase.NewAuth(userQuery, hasherFake{}, codec, codec), unusedSignups(),
		shopsUsecase(query.NewShopQuery(pool), repository.NewShopRepository(pool)), nil,
		usecase.NewReviews(query.NewReviewQuery(pool), unitOfWork, recalc, shopRecalc, storage.NewDisk(t.TempDir(), "/photos"), infra.SystemClock{}),
		usecase.NewUsers(userQuery, domain.NewUsers(repository.NewUserRepository(pool)), unitOfWork, recalc, shopRecalc, hasherFake{}),
		nil, nil, nil, handler.WithGoogleLogin(google))

	// ログの出力を捕まえて、秘密の値が出ていないことを確かめられるようにする。
	// 終わったら、元の出力先に戻す(nil にすると、あとのテストの log 出力が panic する)。
	logs := &bytes.Buffer{}
	originalLogWriter := log.Writer()
	log.SetOutput(logs)
	t.Cleanup(func() { log.SetOutput(originalLogWriter) })
	return &googleKit{pool: pool, conn: conn, router: router, idp: idp, codec: codec, alice: alice, bob: bob, logs: logs, handoffs: map[string][]*http.Cookie{}}
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

// pendingCallback は、代役の認可の画面が自動で承認したあとの、コールバックの直前の状態である
// (1 つのブラウザの cookie と、Google が戻す URL)。
type pendingCallback struct {
	start       *httptest.ResponseRecorder
	cookies     []*http.Cookie
	callbackURL string
}

// cookieSentTo は、ブラウザが、この cookie を、サーバーが見る path(serverPath)への要求に付けるか(Path の一致。
// RFC 6265 の path-match)を返す。ブラウザから見た path には、転送の接頭辞(/api)が付く。**Path が合わない要求には、
// cookie は送られない**ので、たとえば、戻り先の path だけに絞った cookie は、開始の要求には付かない。
func cookieSentTo(c *http.Cookie, serverPath string) bool {
	requestPath := browserAPIPrefix + serverPath
	cookiePath := c.Path
	if cookiePath == "" {
		cookiePath = "/"
	}
	if requestPath == cookiePath {
		return true
	}
	if !strings.HasPrefix(requestPath, cookiePath) {
		return false
	}
	return strings.HasSuffix(cookiePath, "/") || requestPath[len(cookiePath)] == '/'
}

func sendable(cookies []*http.Cookie, serverPath string) []*http.Cookie {
	var out []*http.Cookie
	for _, c := range cookies {
		if cookieSentTo(c, serverPath) {
			out = append(out, c)
		}
	}
	return out
}

// authorize は、手続きを始めて、代役の認可の画面で承認し、コールバックの直前まで進める(開始が Google へ送らなかった
// ときは、callbackURL が空)。SetUser で選んだ利用者は、この時点で決まる。
func (k *googleKit) authorize(t *testing.T, startQuery string, cookiesToSend ...*http.Cookie) pendingCallback {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/auth/google/start"+startQuery, nil)
	for _, c := range sendable(cookiesToSend, "/auth/google/start") {
		req.AddCookie(c)
	}
	p := pendingCallback{start: httptest.NewRecorder()}
	k.router.ServeHTTP(p.start, req)
	if p.start.Code != http.StatusFound {
		return p // 始められなかった: frontend の結果の画面へ、コードなしで送られる。
	}
	p.cookies = p.start.Result().Cookies()
	resp, err := noRedirect().Get(p.start.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	back, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || back.Path != browserAPIPrefix+"/auth/google/callback" {
		t.Fatalf("代役の認可の画面が、コールバックへ戻さなかった: %q", resp.Header.Get("Location"))
	}
	p.callbackURL = back.String()
	return p
}

func plainRun() runOpts {
	return runOpts{cookie: func(c *http.Cookie) *http.Cookie { return c }, query: func(url.Values) {}}
}

const (
	flowCookiePrefix    = "google_login_flow_"
	handoffCookiePrefix = "google_login_handoff_"
)

// cookieFrom は、応答に設定された、手続きの cookie を返す(なければ nil)。
func cookieFrom(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if strings.HasPrefix(c.Name, flowCookiePrefix) && c.MaxAge >= 0 {
			return c
		}
	}
	return nil
}

// handoffCookiesFrom は、応答に設定された、「結び付けの値」の cookie(消すものを除く)を返す。
func handoffCookiesFrom(rec *httptest.ResponseRecorder) []*http.Cookie {
	var out []*http.Cookie
	for _, c := range rec.Result().Cookies() {
		if strings.HasPrefix(c.Name, handoffCookiePrefix) && c.MaxAge >= 0 {
			out = append(out, c)
		}
	}
	return out
}

// authorizeLink は、ログイン済みの利用者(userID)の、結び付けの手続きを、認証つきの POST で始め、返された Google の URL
// へ移動して承認し、コールバックの直前まで進める。cookie は、POST の応答で、このブラウザに設定されたものである。
func (k *googleKit) authorizeLink(t *testing.T, userID, body string, cookiesToSend ...*http.Cookie) pendingCallback {
	t.Helper()
	var reqBody io.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	}
	req := httptest.NewRequest(http.MethodPost, "/me/identities/google/link", reqBody)
	req.Header.Set("Authorization", k.bearer(t, userID))
	for _, c := range sendable(cookiesToSend, "/me/identities/google/link") {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	k.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("結び付けの開始 = %d: %s", rec.Code, rec.Body)
	}
	var start struct {
		RedirectURL string `json:"redirect_url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &start); err != nil || start.RedirectURL == "" {
		t.Fatalf("redirect_url がない: %s", rec.Body)
	}
	p := pendingCallback{start: rec, cookies: rec.Result().Cookies()}
	resp, err := noRedirect().Get(start.RedirectURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	p.callbackURL = resp.Header.Get("Location")
	return p
}

// callback は、コールバックの要求を送り、frontend の結果の画面へ戻された結果(コード)を返す。
func (k *googleKit) callback(t *testing.T, p pendingCallback, cookies []*http.Cookie, o runOpts) flow {
	t.Helper()
	f := flow{start: p.start, authorizeLocation: p.callbackURL}
	back, _ := url.Parse(p.callbackURL)
	q := back.Query()
	o.query(q)
	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?"+q.Encode(), nil) // 転送が /api を取り除いた path
	for _, c := range sendable(cookies, "/auth/google/callback") {
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
	if f.code != "" {
		k.mu.Lock()
		k.handoffs[f.code] = handoffCookiesFrom(f.callback)
		k.mu.Unlock()
	}
	return f
}

// run は、手続きを最後まで進める: 開始 → 代役の認可の画面(自動で承認) → コールバック。mutate は、コールバックの
// 要求の query を書き換える(state の改ざんなど)。cookie に nil を返す関数を渡すと、cookie なしで戻る。
func (k *googleKit) run(t *testing.T, startQuery string, opts ...func(*runOpts)) flow {
	t.Helper()
	o := runOpts{cookie: func(c *http.Cookie) *http.Cookie { return c }, query: func(q url.Values) {}}
	for _, opt := range opts {
		opt(&o)
	}
	p := k.authorize(t, startQuery)
	if p.callbackURL == "" {
		return flow{start: p.start}
	}
	return k.callback(t, p, p.cookies, o)
}

// browserJar は、1 つのブラウザが持つ cookie の代役である。ブラウザと同じく、名前と Path が同じ cookie を、Set-Cookie で
// 置き換え、MaxAge が負の Set-Cookie で(名前と Path が同じものだけを)消す。
type browserJar struct{ store map[string]*http.Cookie }

func (j *browserJar) cookies() []*http.Cookie {
	out := make([]*http.Cookie, 0, len(j.store))
	for _, c := range j.store {
		out = append(out, c)
	}
	return out
}

func (j *browserJar) update(rec *httptest.ResponseRecorder) {
	if j.store == nil {
		j.store = map[string]*http.Cookie{}
	}
	for _, c := range rec.Result().Cookies() {
		key := c.Name + "|" + c.Path
		if c.MaxAge < 0 {
			delete(j.store, key)
		} else {
			j.store[key] = c
		}
	}
}

type runOpts struct {
	cookie func(*http.Cookie) *http.Cookie
	query  func(url.Values)
}

func withQuery(fn func(url.Values)) func(*runOpts) { return func(o *runOpts) { o.query = fn } }
func withCookie(fn func(*http.Cookie) *http.Cookie) func(*runOpts) {
	return func(o *runOpts) { o.cookie = fn }
}

// exchange は、コードを、そのコードの手続きを終えたブラウザ(コールバックの応答が「結び付けの値」の cookie を
// 設定したブラウザ)が交換する要求である。
func (k *googleKit) exchange(code string) *httptest.ResponseRecorder {
	return k.exchangeIn(code, "")
}

// exchangeIn は、exchange と同じだが、Accept-Language ヘッダーを付ける(空なら付けない)。
func (k *googleKit) exchangeIn(code, acceptLanguage string) *httptest.ResponseRecorder {
	k.mu.Lock()
	cookies := k.handoffs[code]
	k.mu.Unlock()
	return k.exchangeAs(code, acceptLanguage, cookies...)
}

// exchangeWith は、コードを、指定した cookie を持つブラウザが交換する要求である(cookie を渡さなければ、別のブラウザ)。
// cookie は、ブラウザと同じく、Path が合うものだけが送られる。
func (k *googleKit) exchangeWith(code string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return k.exchangeAs(code, "", cookies...)
}

// exchangeAs は、exchangeWith に、Accept-Language(空なら付けない)を足したものである。
func (k *googleKit) exchangeAs(code, acceptLanguage string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"code": code})
	req := httptest.NewRequest(http.MethodPost, "/auth/google/exchange", strings.NewReader(string(body)))
	for _, c := range sendable(cookies, "/auth/google/exchange") {
		req.AddCookie(c)
	}
	if acceptLanguage != "" {
		req.Header.Set("Accept-Language", acceptLanguage)
	}
	rec := httptest.NewRecorder()
	k.router.ServeHTTP(rec, req)
	return rec
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
	// Reason は、失敗の理由を示す、言語によらない識別子(messages.go の key* 定数と同じ文字列)。
	Reason string `json:"reason"`
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

	t.Run("表示名(name)がない Google アカウントでも、公開のユーザー名に、メールの一部(@ より前)を使わない", func(t *testing.T) {
		k := newGoogleKit(t)
		k.idp.SetUser(fakeoidc.User{Sub: "sub-noname", Email: "john.smith1985@gmail.example", EmailVerified: true, Name: ""})
		body := decodeExchange(t, k.exchange(k.run(t, "").code))
		if body.Token == "" {
			t.Fatalf("サインインできなかった: %+v", body)
		}
		if strings.Contains(strings.ToLower(body.Username), "john") || strings.Contains(body.Username, "1985") || strings.Contains(body.Username, "@") {
			t.Fatalf("ユーザー名 %q に、メールの一部が含まれている(email は本人にしか見せない)", body.Username)
		}
		var publicName string
		if err := k.conn.QueryRow(context.Background(), `SELECT username FROM users WHERE email = 'john.smith1985@gmail.example'`).Scan(&publicName); err != nil || publicName != body.Username {
			t.Fatalf("保存されたユーザー名 = %q(%v), 応答 = %q", publicName, err, body.Username)
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
			if body.Reason != "google.account_exists" {
				t.Errorf("%s: reason = %q, want google.account_exists", email, body.Reason)
			}
			if n := k.count(t, "user_identities"); n != 0 {
				t.Errorf("%s: 結び付きが作られた", email)
			}
			if n := k.count(t, "users"); n != 2 {
				t.Errorf("%s: 利用者が増えた(%d 人)", email, n)
			}
		}
	})

	t.Run("大文字小文字だけが違うメールの、2 つの新しい Google アカウントが並行して新規登録しても、利用者は 1 人だけ作られ、負けた側は案内のエラーになる", func(t *testing.T) {
		k := newGoogleKit(t)
		k.idp.SetUser(fakeoidc.User{Sub: "sub-race-1", Email: "Racer@Gmail.example", EmailVerified: true, Name: "Racer One"})
		first := k.authorize(t, "")
		k.idp.SetUser(fakeoidc.User{Sub: "sub-race-2", Email: "racer@gmail.example", EmailVerified: true, Name: "Racer Two"})
		second := k.authorize(t, "")

		codes := make([]string, 2)
		var wg sync.WaitGroup
		for i, p := range []pendingCallback{first, second} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				codes[i] = k.callback(t, p, p.cookies, runOpts{cookie: func(c *http.Cookie) *http.Cookie { return c }, query: func(url.Values) {}}).code
			}()
		}
		wg.Wait()

		var created, exists int
		for _, code := range codes {
			switch rec := k.exchange(code); rec.Code {
			case http.StatusOK:
				created++
			case http.StatusConflict:
				exists++
			default:
				t.Fatalf("交換 = %d %s", rec.Code, rec.Body)
			}
		}
		if created != 1 || exists != 1 {
			t.Fatalf("新規登録 %d 件・案内のエラー %d 件, want 1 件ずつ", created, exists)
		}
		var n int
		if err := k.conn.QueryRow(context.Background(), `SELECT count(*) FROM users WHERE lower(email) = 'racer@gmail.example'`).Scan(&n); err != nil || n != 1 {
			t.Fatalf("大文字小文字を無視して同じメールの利用者が %d 人(err %v), want 1", n, err)
		}
	})

	t.Run("同じ Google アカウントの初回のサインインが並行しても、両方が同じ利用者としてサインインでき、「パスワードでサインインしてください」とは案内されない", func(t *testing.T) {
		rendezvous := newRendezvousIdentities(2)
		k := newGoogleKit(t, withIdentityQuery(func(inner usecase.IdentityQuery) usecase.IdentityQuery {
			rendezvous.IdentityQuery = inner
			return rendezvous
		}))
		first := k.authorize(t, "")
		second := k.authorize(t, "")

		codes := make([]string, 2)
		var wg sync.WaitGroup
		for i, p := range []pendingCallback{first, second} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				codes[i] = k.callback(t, p, p.cookies, plainRun()).code
			}()
		}
		wg.Wait()

		ids := map[string]bool{}
		for i, code := range codes {
			rec := k.exchange(code)
			body := decodeExchange(t, rec)
			if rec.Code != http.StatusOK || body.Token == "" {
				t.Fatalf("%d 番目の手続き = %d %s, want サインインの成功(負けた側が、誤った案内になっていないか)", i+1, rec.Code, rec.Body)
			}
			ids[body.ID] = true
		}
		if len(ids) != 1 {
			t.Fatalf("同じ Google アカウントなのに、別の利用者としてサインインした: %v", ids)
		}
		if k.count(t, "users") != 3 || k.count(t, "user_identities") != 1 {
			t.Fatalf("利用者 %d 人・結び付き %d 件, want 3 人・1 件", k.count(t, "users"), k.count(t, "user_identities"))
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

	t.Run("手続きの cookie がない(または、state が合わない)コールバックでは、DB に何も書かず、コードなしで frontend の結果の画面へ戻す", func(t *testing.T) {
		cases := map[string]func(*runOpts){
			"cookie がない":      func(o *runOpts) { o.cookie = func(*http.Cookie) *http.Cookie { return nil } },
			"state の改ざん":      func(o *runOpts) { o.query = func(q url.Values) { q.Set("state", "forged-state") } },
			"state がない":       func(o *runOpts) { o.query = func(q url.Values) { q.Del("state") } },
			"別の秘密で封じた cookie": func(o *runOpts) { o.cookie = func(c *http.Cookie) *http.Cookie { c.Value = "garbage"; return c } },
		}
		for name, mutate := range cases {
			t.Run(name, func(t *testing.T) {
				k := newGoogleKit(t)
				before := k.count(t, "login_handoffs")
				f := k.run(t, "", func(o *runOpts) { mutate(o) })
				if f.code != "" {
					t.Fatalf("コードが渡された(手続きを始めたブラウザが確かめられないのに、結果を作った): %q", f.code)
				}
				if after := k.count(t, "login_handoffs"); after != before {
					t.Fatalf("login_handoffs が %d 件から %d 件に増えた(手続きの cookie がない要求で、DB に書いている)", before, after)
				}
			})
		}
		t.Run("パラメーターなしの要求", func(t *testing.T) {
			k := newGoogleKit(t)
			rec := do(k.router, http.MethodGet, "/auth/google/callback", "", "")
			if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "code=") || k.count(t, "login_handoffs") != 0 {
				t.Fatalf("%d %q, login_handoffs %d 件", rec.Code, rec.Header().Get("Location"), k.count(t, "login_handoffs"))
			}
		})
	})

	t.Run("失敗の応答(409・400)にも、検証済みの戻り先(return_to)を含める(AI アプリの許可の画面から来た利用者が、元の要求へ戻れるように)", func(t *testing.T) {
		const consent = "/oauth/authorize?client_id=app-1&state=xyz"
		cases := map[string]struct {
			setup      func(*googleKit)
			wantStatus int
		}{
			"同じメールのアカウントがある(409)": {func(k *googleKit) {
				k.idp.SetUser(fakeoidc.User{Sub: "sub-x", Email: "alice@example.com", EmailVerified: true, Name: "X"})
			}, http.StatusConflict},
			"利用者が認可を拒否した(400)":     {func(k *googleKit) { k.idp.SetTweaks(fakeoidc.Tweaks{DenyAuthorize: true}) }, http.StatusBadRequest},
			"ID トークンの検証に失敗した(400)": {func(k *googleKit) { k.idp.SetTweaks(fakeoidc.Tweaks{Aud: "another-client"}) }, http.StatusBadRequest},
		}
		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				k := newGoogleKit(t)
				tc.setup(k)
				rec := k.exchange(k.run(t, "?return_to="+url.QueryEscape(consent)).code)
				if rec.Code != tc.wantStatus {
					t.Fatalf("交換 = %d %s, want %d", rec.Code, rec.Body, tc.wantStatus)
				}
				if got := decodeExchange(t, rec).ReturnTo; got != consent {
					t.Fatalf("失敗の応答の return_to = %q, want %q", got, consent)
				}
			})
		}
		t.Run("アプリの外を指す戻り先は、失敗の応答でも、既定(空)になる", func(t *testing.T) {
			k := newGoogleKit(t)
			k.idp.SetTweaks(fakeoidc.Tweaks{DenyAuthorize: true})
			rec := k.exchange(k.run(t, "?return_to="+url.QueryEscape("https://evil.example/x")).code)
			if rec.Code != http.StatusBadRequest || decodeExchange(t, rec).ReturnTo != "" {
				t.Fatalf("%d %s", rec.Code, rec.Body)
			}
		})
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
				if body.Reason != "google.sign_in_failed" {
					t.Errorf("reason = %q, want google.sign_in_failed", body.Reason)
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
		rec := k.exchange(code)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("2 回目 = %d, want 400", rec.Code)
		}
		if got := decodeExchange(t, rec).Reason; got != "google.code_invalid" {
			t.Errorf("2 回目の reason = %q, want google.code_invalid", got)
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

	t.Run("交換の途中で、一時的な失敗(トークンの発行・利用者の取得)が起きても、コードは消えず、同じコードでの再試行が成功する", func(t *testing.T) {
		cases := map[string]func() []func(*kitOptions){
			"トークンの発行の失敗": func() []func(*kitOptions) {
				f := &flakyIssuer{}
				f.failures.Store(1)
				return []func(*kitOptions){withIssuer(func(inner usecase.TokenIssuer) usecase.TokenIssuer { f.inner = inner; return f })}
			},
			"利用者の取得(DB)の失敗": func() []func(*kitOptions) {
				f := &flakyUsers{}
				f.failures.Store(1)
				return []func(*kitOptions){withTxUsers(func(inner usecase.UserQuery) usecase.UserQuery { f.UserQuery = inner; return f })}
			},
		}
		for name, mk := range cases {
			t.Run(name, func(t *testing.T) {
				k := newGoogleKit(t, mk()...)
				code := k.run(t, "").code
				if first := k.exchange(code); first.Code != http.StatusInternalServerError {
					t.Fatalf("失敗させた 1 回目 = %d %s, want 500", first.Code, first.Body)
				}
				retry := k.exchange(code)
				if retry.Code != http.StatusOK || decodeExchange(t, retry).Token == "" {
					t.Fatalf("同じコードでの再試行 = %d %s, want 200(コードは、後続の処理が成功するまで消えない)", retry.Code, retry.Body)
				}
				if again := k.exchange(code); again.Code != http.StatusBadRequest {
					t.Fatalf("成功したあとの 3 回目 = %d, want 400(1 回だけ使える)", again.Code)
				}
			})
		}
	})

	t.Run("2 つのタブで、続けて Google でのサインインを始めても、両方の手続きが成功する(後発が先発の cookie を上書きしない)", func(t *testing.T) {
		k := newGoogleKit(t)
		jar := &browserJar{}
		tab1 := k.authorize(t, "?return_to=/shops", jar.cookies()...)
		jar.update(tab1.start)
		tab2 := k.authorize(t, "?return_to=/reviews", jar.cookies()...)
		jar.update(tab2.start)
		plain := runOpts{cookie: func(c *http.Cookie) *http.Cookie { return c }, query: func(url.Values) {}}

		f1 := k.callback(t, tab1, jar.cookies(), plain)
		jar.update(f1.callback)
		f2 := k.callback(t, tab2, jar.cookies(), plain)
		jar.update(f2.callback)

		if b1 := decodeExchange(t, k.exchange(f1.code)); b1.Token == "" || b1.ReturnTo != "/shops" {
			t.Fatalf("先発のタブ: %+v", b1)
		}
		if b2 := decodeExchange(t, k.exchange(f2.code)); b2.Token == "" || b2.ReturnTo != "/reviews" {
			t.Fatalf("後発のタブ: %+v", b2)
		}
		jar.update(k.exchange(f1.code)) // (交換の応答も、ブラウザの cookie を消す)
		jar.update(k.exchange(f2.code))
		if left := jar.cookies(); len(left) != 0 {
			t.Fatalf("両方の手続きが終わったのに、cookie が %d 件残っている: %+v", len(left), left)
		}
	})

	t.Run("同じコードを、接続プールの上限より多く並行して交換しても、止まらず、サインインできるのは 1 回だけである", func(t *testing.T) {
		// 接続を 2 つしか持たないプールで、12 件を同時に交換する。トランザクションの中で、プールからもう 1 つ接続を
		// 取ろうとすると、待っている処理が接続を使い切り、先頭の処理が進めなくなって、全体が止まる(デッドロック)。
		// 発行を遅くして、並行する交換が、確実に重なるようにする(コードのロックがなければ、複数が成功してしまう)。
		k := newGoogleKit(t, withMaxConns(2), withIssuer(func(inner usecase.TokenIssuer) usecase.TokenIssuer {
			return slowIssuer{inner: inner, delay: 150 * time.Millisecond}
		}))
		code := k.run(t, "").code
		var ok, rejected atomic.Int32
		done := make(chan struct{})
		go func() {
			defer close(done)
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
		}()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Fatal("並行する交換が止まった(接続プールの取り合いによるデッドロックの疑い)")
		}
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

	t.Run("手続きの cookie は、手続きごとに 1 つで、HttpOnly・SameSite=Lax・戻りの要求の path だけに限られ、名前にも値にも平文の秘密を含まず、コールバックで消される", func(t *testing.T) {
		k := newGoogleKit(t)
		f := k.run(t, "")
		flowCookie := cookieFrom(f.start)
		if flowCookie == nil || !strings.HasPrefix(flowCookie.Name, flowCookiePrefix) || !flowCookie.HttpOnly || flowCookie.SameSite != http.SameSiteLaxMode ||
			flowCookie.Path != browserAPIPrefix+"/auth/google/callback" || flowCookie.MaxAge != 600 || flowCookie.Secure {
			t.Fatalf("cookie = %+v", flowCookie)
		}
		authURL, _ := url.Parse(f.start.Header().Get("Location"))
		if state := authURL.Query().Get("state"); strings.Contains(flowCookie.Value, state) || strings.Contains(flowCookie.Name, state) {
			t.Error("cookie の名前か値に、平文の state が含まれている")
		}
		var cleared *http.Cookie
		for _, c := range f.callback.Result().Cookies() {
			if c.Name == flowCookie.Name && c.MaxAge < 0 {
				cleared = c
			}
		}
		if cleared == nil || cleared.Path != flowCookie.Path {
			t.Errorf("コールバックが、同じ名前・Path の cookie を消していない: %+v", cleared)
		}

		// コールバックは、「結び付けの値」の cookie を、結果との交換の要求だけに送られる Path で設定する。
		binders := handoffCookiesFrom(f.callback)
		if len(binders) != 1 {
			t.Fatalf("結び付けの値の cookie = %d 件, want 1", len(binders))
		}
		b := binders[0]
		if !strings.HasPrefix(b.Name, handoffCookiePrefix) || !b.HttpOnly || b.SameSite != http.SameSiteLaxMode || b.Path != browserAPIPrefix+"/auth/google/exchange" ||
			b.MaxAge != int(domain.LoginHandoffTTL.Seconds()) || b.Secure {
			t.Fatalf("結び付けの値の cookie = %+v", b)
		}
		if strings.Contains(b.Name, f.code) || b.Value == f.code || strings.Contains(f.callback.Header().Get("Location"), b.Value) {
			t.Error("結び付けの値が、コードと同じか、URL に含まれているか、名前にコードが含まれている")
		}
	})

	t.Run("手続きを終えたブラウザに、cookie は残らない(手続きの cookie はコールバックで、結び付けの値の cookie は交換で、消える)。古い Path の同名の cookie が読まれ続けることも、ない", func(t *testing.T) {
		k := newGoogleKit(t)
		jar := &browserJar{}
		p := k.authorize(t, "")
		jar.update(p.start)
		if len(jar.cookies()) != 1 {
			t.Fatalf("開始のあとの cookie = %d 件, want 1", len(jar.cookies()))
		}

		f := k.callback(t, p, jar.cookies(), plainRun())
		jar.update(f.callback)
		if names := len(jar.cookies()); names != 1 { // 手続きの cookie は消え、結び付けの値の cookie だけが残る
			t.Fatalf("コールバックのあとの cookie = %d 件, want 1(結び付けの値だけ)", names)
		}

		jar.update(k.exchange(f.code))
		if left := jar.cookies(); len(left) != 0 {
			t.Fatalf("交換のあとも、cookie が %d 件残っている: %+v", len(left), left)
		}
	})
}

func TestGoogleLinking(t *testing.T) {
	// linkFlow は、ログイン済みの利用者(userID)の、結び付けの手続きを、同じブラウザで最後まで進める:
	// 認証つきの開始(POST。cookie が、このブラウザに設定される)→ Google の URL へ移動 → 承認 → コールバック。
	linkFlow := func(t *testing.T, k *googleKit, userID string) flow {
		t.Helper()
		p := k.authorizeLink(t, userID, `{"return_to":"/profile"}`)
		return k.callback(t, p, p.cookies, plainRun())
	}

	t.Run("ログイン済みの利用者が結び付けると、その Google アカウントでサインインでき、一覧に出る", func(t *testing.T) {
		k := newGoogleKit(t)
		f := linkFlow(t, k, k.alice)
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

	t.Run("2 つのタブで、続けて結び付けを始めても、両方の手続きが成功する(先発の手続きの cookie が、後発の開始の要求にも送られ、上書きされない)", func(t *testing.T) {
		k := newGoogleKit(t)
		jar := &browserJar{}
		tab1 := k.authorizeLink(t, k.alice, `{"return_to":"/profile"}`, jar.cookies()...)
		jar.update(tab1.start)
		tab2 := k.authorizeLink(t, k.alice, `{"return_to":"/shops"}`, jar.cookies()...)
		jar.update(tab2.start)

		f1 := k.callback(t, tab1, jar.cookies(), plainRun())
		jar.update(f1.callback)
		f2 := k.callback(t, tab2, jar.cookies(), plainRun())
		jar.update(f2.callback)
		for i, f := range []flow{f1, f2} {
			if b := decodeExchange(t, k.exchange(f.code)); !b.Linked {
				t.Fatalf("%d 番目のタブの手続きが失敗した: %+v", i+1, b)
			}
		}
	})

	t.Run("手続きを終えたブラウザとは別のブラウザが、「画面へ渡すコード」だけを持ち込んで交換しても、サインインも結び付けもできず、本来のブラウザのコードも使えなくならない(ログイン CSRF)", func(t *testing.T) {
		k := newGoogleKit(t)
		// 攻撃者が、自分の Google で手続きを最後まで進め、画面へ戻る URL(/auth/google/complete?code=C)で止める。
		attacker := k.run(t, "")
		if attacker.code == "" {
			t.Fatal("攻撃者の手続きが、コードを得られなかった")
		}
		// 被害者が、その URL を(60 秒以内に)踏む。被害者のブラウザは、結び付けの値の cookie を持たない。
		rec := k.exchangeWith(attacker.code)
		body := decodeExchange(t, rec)
		if rec.Code != http.StatusBadRequest || body.Token != "" || len(body.Errors) != 1 {
			t.Fatalf("別のブラウザが、コードだけで交換できた(被害者が、攻撃者のアカウントでログインした状態になる): %d %s", rec.Code, rec.Body)
		}
		if n := k.count(t, "users"); n != 3 { // alice・bob と、攻撃者自身の Google のアカウント(コールバックで作られた)
			t.Fatalf("利用者が %d 人, want 3 人(別のブラウザの交換で、増えていないこと)", n)
		}

		// 無効なコードと、区別できない(応答が同じ)。
		unknown := k.exchangeWith("unknown-code")
		if unknown.Code != rec.Code || unknown.Body.String() != rec.Body.String() {
			t.Fatalf("結び付けの値がない交換 = %d %s / 無効なコード = %d %s, want 同じ応答", rec.Code, rec.Body, unknown.Code, unknown.Body)
		}

		// 被害者の試み(または、攻撃者の推測)で、本来のブラウザのコードは、消費されない。
		if legit := k.exchange(attacker.code); legit.Code != http.StatusOK || decodeExchange(t, legit).Token == "" {
			t.Fatalf("別のブラウザの試みのあと、本来のブラウザが交換できない: %d %s", legit.Code, legit.Body)
		}
		if again := k.exchange(attacker.code); again.Code != http.StatusBadRequest {
			t.Fatalf("2 回目 = %d, want 400(1 回だけ使える)", again.Code)
		}
	})

	t.Run("結び付けの値が合わない(改ざん・別のコードのもの・コードそのもの)交換は、無効なコードと同じ応答で断られ、コードは消費されない", func(t *testing.T) {
		k := newGoogleKit(t)
		first, second := k.run(t, ""), k.run(t, "")
		firstBinder, secondBinder := k.handoffs[first.code][0], k.handoffs[second.code][0]

		wrongValue := *firstBinder
		wrongValue.Value = secondBinder.Value // 別のコードの結び付けの値
		tampered := *firstBinder
		tampered.Value = firstBinder.Value[:len(firstBinder.Value)-2] + "AA"
		asCode := *firstBinder
		asCode.Value = first.code
		movedName := *secondBinder // 別のコードの名前の cookie に、値だけを移し替えたもの(名前が合わないので、送られても読まれない)
		movedName.Value = firstBinder.Value
		for name, c := range map[string]*http.Cookie{"別のコードの結び付けの値": &wrongValue, "改ざん": &tampered, "コードそのもの": &asCode, "名前の違う cookie": &movedName} {
			if rec := k.exchangeWith(first.code, c); rec.Code != http.StatusBadRequest || decodeExchange(t, rec).Token != "" {
				t.Errorf("%s で交換できた: %d %s", name, rec.Code, rec.Body)
			}
		}
		if ok := k.exchange(first.code); ok.Code != http.StatusOK {
			t.Fatalf("断られた試みのあと、正しい結び付けの値での交換 = %d %s", ok.Code, ok.Body)
		}
	})

	t.Run("結び付けを始めたブラウザとは別のブラウザで、Google の URL(と、古い link_code つきの開始の URL)を開かせても、被害者の Google は、攻撃者のアカウントに結び付けられない", func(t *testing.T) {
		k := newGoogleKit(t)
		// 攻撃者(alice)が、自分のブラウザで、結び付けを始める(cookie は、攻撃者のブラウザにだけ設定される)。
		rec := do(k.router, http.MethodPost, "/me/identities/google/link", "", k.bearer(t, k.alice))
		if rec.Code != http.StatusOK {
			t.Fatalf("結び付けの開始 = %d %s", rec.Code, rec.Body)
		}
		var start struct {
			RedirectURL string `json:"redirect_url"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &start)
		if strings.Contains(rec.Body.String(), "link_code") || strings.Contains(rec.Body.String(), k.alice) {
			t.Fatalf("応答に、持ち運べる開始のコードや利用者の ID が含まれている: %s", rec.Body)
		}
		// 被害者(victim)が、その URL を、自分のブラウザ(手続きの cookie を持たない)で開き、Google で承認する。
		k.idp.SetUser(fakeoidc.User{Sub: "sub-victim", Email: "victim@gmail.example", EmailVerified: true, Name: "Victim"})
		resp, err := noRedirect().Get(start.RedirectURL)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		victimCallback := pendingCallback{callbackURL: resp.Header.Get("Location")}
		f := k.callback(t, victimCallback, nil, plainRun()) // 被害者のブラウザには、手続きの cookie がない
		if b := decodeExchange(t, k.exchange(f.code)); b.Linked || b.Token != "" {
			t.Fatalf("別のブラウザで開いた URL で、被害者の Google が結び付けられた(またはサインインした): %+v", b)
		}

		// 古い形の URL(link_code に、攻撃者の利用者の ID を付けた開始)は、もう結び付けの手続きではなく、ただのサインインとして扱われる。
		f2 := k.run(t, "?link_code="+url.QueryEscape(k.alice)+"&link_user="+url.QueryEscape(k.alice)+"&user_id="+url.QueryEscape(k.alice))
		if b := decodeExchange(t, k.exchange(f2.code)); b.Linked {
			t.Fatalf("link_code つきの開始が、結び付けとして扱われた: %+v", b)
		}
		var n int
		if err := k.conn.QueryRow(context.Background(), `SELECT count(*) FROM user_identities WHERE user_id = $1`, k.alice).Scan(&n); err != nil || n != 0 {
			t.Fatalf("alice に結び付いた外部のアカウントが %d 件(err %v), want 0", n, err)
		}
	})

	t.Run("結び付けの開始は、Google の認可の URL を返し、手続きの cookie を、要求を出したブラウザに設定する(結び付ける利用者は、cookie の中にだけある)", func(t *testing.T) {
		k := newGoogleKit(t)
		rec := do(k.router, http.MethodPost, "/me/identities/google/link", `{"return_to":"/profile"}`, k.bearer(t, k.alice))
		var start struct {
			RedirectURL string `json:"redirect_url"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &start)
		c := cookieFrom(rec)
		if rec.Code != http.StatusOK || !strings.HasPrefix(start.RedirectURL, k.idp.URL+"/authorize?") || c == nil || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
			t.Fatalf("%d %s / cookie %+v", rec.Code, rec.Body, c)
		}
		if strings.Contains(c.Value, k.alice) || strings.Contains(start.RedirectURL, k.alice) {
			t.Fatal("利用者の ID が、平文で、cookie か URL に含まれている")
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
		}
		// body なし(return_to を省略)でも始められる。
		if plain := do(k.router, http.MethodPost, "/me/identities/google/link", "", k.bearer(t, k.alice)); plain.Code != http.StatusOK {
			t.Fatalf("body なし = %d", plain.Code)
		}
	})

	t.Run("別の利用者に結び付け済みの Google アカウントは、結び付けられず、すでに Google と結び付いた利用者も、別の Google を足せない", func(t *testing.T) {
		k := newGoogleKit(t)
		if body := decodeExchange(t, k.exchange(linkFlow(t, k, k.bob).code)); !body.Linked {
			t.Fatal("bob の結び付けに失敗した")
		}
		taken := k.exchange(linkFlow(t, k, k.alice).code)
		if taken.Code != http.StatusConflict || !strings.Contains(taken.Body.String(), "another account") {
			t.Fatalf("別の利用者の Google = %d %s", taken.Code, taken.Body)
		}
		if got := decodeExchange(t, taken).Reason; got != "google.identity_taken" {
			t.Errorf("別の利用者の Google の reason = %q, want google.identity_taken", got)
		}
		k.idp.SetUser(fakeoidc.User{Sub: "sub-other", Email: "other@gmail.example", EmailVerified: true, Name: "Other"})
		already := k.exchange(linkFlow(t, k, k.bob).code)
		if already.Code != http.StatusConflict || !strings.Contains(already.Body.String(), "already connected") {
			t.Fatalf("すでに結び付いた利用者 = %d %s", already.Code, already.Body)
		}
		if got := decodeExchange(t, already).Reason; got != "google.already_linked" {
			t.Errorf("すでに結び付いた利用者の reason = %q, want google.already_linked", got)
		}
		if n := k.count(t, "user_identities"); n != 1 {
			t.Fatalf("結び付きが %d 件", n)
		}
	})

	t.Run("同じ Google アカウントを、同じ利用者が結び付け直しても、成功し、増えない(繰り返しの操作)", func(t *testing.T) {
		k := newGoogleKit(t)
		k.exchange(linkFlow(t, k, k.alice).code)
		again := k.exchange(linkFlow(t, k, k.alice).code)
		if again.Code != http.StatusOK || !decodeExchange(t, again).Linked || k.count(t, "user_identities") != 1 {
			t.Fatalf("%d %s", again.Code, again.Body)
		}
	})

	t.Run("パスワードのある利用者は解除でき、そのあともパスワードでサインインできる", func(t *testing.T) {
		k := newGoogleKit(t)
		k.exchange(linkFlow(t, k, k.alice).code)
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

	t.Run("Google と結び付けた利用者が退会すると、結び付きも消え、その Google アカウントを、別の利用者が結び付けられる", func(t *testing.T) {
		k := newGoogleKit(t)
		if body := decodeExchange(t, k.exchange(linkFlow(t, k, k.alice).code)); !body.Linked {
			t.Fatal("alice の結び付けに失敗した")
		}

		if del := do(k.router, http.MethodDelete, "/users/"+k.alice, "", k.bearer(t, k.alice)); del.Code != http.StatusNoContent {
			t.Fatalf("退会 = %d %s", del.Code, del.Body)
		}

		if n := k.count(t, "user_identities"); n != 0 {
			t.Fatalf("退会したのに、結び付きが %d 件残っている", n)
		}
		linked := k.exchange(linkFlow(t, k, k.bob).code)
		if linked.Code != http.StatusOK || !decodeExchange(t, linked).Linked {
			t.Fatalf("退会した利用者の Google を、別の利用者(bob)が結び付けられない: %d %s", linked.Code, linked.Body)
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

func TestGoogleLoginDisabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed test in short mode")
	}
	router := handler.NewRouter(okPinger, nil, unusedSignups(), nil, nil, nil, nil, nil, nil, nil)
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

// テストの部品(googleKit)は、ログの出力先を差し替えて、秘密が出ていないことを確かめるが、終わったら、元の出力先へ
// 戻さなければならない(nil にすると、あとのテストの log 出力が panic して、テストの実行順に依存してしまう)。
func TestGoogleKitRestoresTheLogWriter(t *testing.T) {
	before := log.Writer()
	t.Run("キットを使うテスト", func(t *testing.T) {
		_ = newGoogleKit(t)
	})
	if after := log.Writer(); after != before {
		t.Fatalf("ログの出力先が戻っていない: before = %v, after = %v", before, after)
	}
	log.Print("出力先が戻っていれば、panic しない")
}
