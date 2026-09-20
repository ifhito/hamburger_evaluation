package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/handler"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/infra"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/storage"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

const testJWTSecret = "handler-test-secret"

// hasherFake は、同じ契約を持つ bcrypt の高速な代役である。
type hasherFake struct{}

func (hasherFake) Hash(password string) (string, error) { return "digest:" + password, nil }
func (hasherFake) Compare(digest, password string) error {
	if digest != "digest:"+password {
		return domain.ErrInvalidCredentials
	}
	return nil
}

// fakeRecord は userRepoFake の中に保存されたユーザーである。
type fakeRecord struct {
	user      domain.User
	digest    string
	discarded bool
}

// userRepoFake は in-memory の usecase.UserRepository である。err を設定すると
// すべての操作がその err で失敗する（500 の経路を駆動する）。
type userRepoFake struct {
	seq   int64
	users map[int64]*fakeRecord
	err   error
}

func newUserRepoFake() *userRepoFake { return &userRepoFake{users: map[int64]*fakeRecord{}} }

func (f *userRepoFake) CreateUser(_ context.Context, p usecase.CreateUserParams) (domain.User, error) {
	if f.err != nil {
		return domain.User{}, f.err
	}
	for _, rec := range f.users {
		if rec.user.Email == p.Email {
			return domain.User{}, domain.ErrEmailTaken
		}
	}
	f.seq++
	user := domain.User{ID: f.seq, Username: p.Username, Email: p.Email, Admin: p.Admin}
	f.users[user.ID] = &fakeRecord{user: user, digest: p.PasswordDigest}
	return user, nil
}

func (f *userRepoFake) GetActiveUserByEmail(_ context.Context, email string) (usecase.UserCredentials, error) {
	if f.err != nil {
		return usecase.UserCredentials{}, f.err
	}
	for _, rec := range f.users {
		if rec.user.Email == email && !rec.discarded {
			return usecase.UserCredentials{User: rec.user, PasswordDigest: rec.digest}, nil
		}
	}
	return usecase.UserCredentials{}, domain.ErrUserNotFound
}

func (f *userRepoFake) GetActiveUserByID(_ context.Context, id int64) (domain.User, error) {
	if f.err != nil {
		return domain.User{}, f.err
	}
	if rec, ok := f.users[id]; ok && !rec.discarded {
		return rec.user, nil
	}
	return domain.User{}, domain.ErrUserNotFound
}

// seed は、password に対する hasherFake の digest を持つ active なユーザーを
// 保存する。
func (f *userRepoFake) seed(username, email, password string) domain.User {
	user, err := f.CreateUser(context.Background(), usecase.CreateUserParams{
		Username:       username,
		Email:          email,
		PasswordDigest: "digest:" + password,
	})
	if err != nil {
		panic(err)
	}
	return user
}

// newAuthKit は、in-memory の fake と本物の JWT codec の上に本物の auth
// usecase を構築し、router レベルのテストに使える状態にする。
func newAuthKit() (*userRepoFake, *usecase.Auth, *infra.JWTCodec) {
	repo := newUserRepoFake()
	codec := infra.NewJWTCodec(testJWTSecret, time.Hour)
	return repo, usecase.NewAuth(repo, hasherFake{}, codec, codec), codec
}

// newTestRouter は、db の health か routing の挙動だけを必要とするテスト向けの
// 既定の router である。
func newTestRouter(t *testing.T, p handler.Pinger) http.Handler {
	t.Helper()
	_, auth, _ := newAuthKit()
	return newTestRouterWith(t, p, auth)
}

// newTestRouterWith は、与えられた auth、空の in-memory の shop/review の
// fake、新しい temp dir 内の本物の disk photo store（NewReviews は nil でない
// storage を要求する）で router を配線する。そのデータを気にしないテスト向け
// である。
func newTestRouterWith(t *testing.T, p handler.Pinger, auth *usecase.Auth) http.Handler {
	t.Helper()
	return handler.NewRouter(p, auth, usecase.NewShops(&shopRepoFake{}),
		usecase.NewReviews(newReviewRepoFake(), storage.NewDisk(t.TempDir(), "/photos")),
		usecase.NewUsers(newUserRepoFake(), hasherFake{}), nil)
}

// do は router に対して 1 件の request を in-process で実行し、recorder を
// 返す。
func do(router http.Handler, method, path, body, authHeader string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeAuthUser(t *testing.T, body []byte) (resp struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Admin    bool   `json:"admin"`
	Token    string `json:"token"`
}) {
	t.Helper()
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("body %q is not valid JSON: %v", body, err)
	}
	return resp
}

// TestSignupThenLogout は AC1 を扱う：新規の signup は、完全な snake_case の
// body を伴う 201 と、保護された POST /logout ルートをただちに通過できる
// トークンを返す。
func TestSignupThenLogout(t *testing.T) {
	_, auth, _ := newAuthKit()
	router := newTestRouterWith(t, okPinger, auth)

	rec := do(router, http.MethodPost, "/signup",
		`{"username":"alice","email":"alice@example.com","password":"Password123!","password_confirmation":"Password123!"}`, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("signup status = %d, want %d (body %s)", rec.Code, http.StatusCreated, rec.Body)
	}
	user := decodeAuthUser(t, rec.Body.Bytes())
	if user.ID != 1 || user.Username != "alice" || user.Email != "alice@example.com" || user.Admin {
		t.Errorf("signup body = %+v, want id=1 alice alice@example.com admin=false", user)
	}
	if user.Token == "" {
		t.Fatal("signup token is empty")
	}

	rec = do(router, http.MethodPost, "/logout", "", "Bearer "+user.Token)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
	}
	if got := rec.Body.String(); got != `{"message":"Logged out successfully"}` {
		t.Errorf("logout body = %q, want the Rails-parity message", got)
	}
}

// TestSignupErrors は AC2（既に使われている email -> 422）に加え、POST /signup
// の decode 経路と失敗経路を扱う。
func TestSignupErrors(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(repo *userRepoFake)
		body       string
		wantStatus int
		wantBody   string // 完全一致させる body。空なら status のみを検証する
	}{
		{
			name:       "AC2 使用済みの email は 422 を返す",
			setup:      func(repo *userRepoFake) { repo.seed("bob", "bob@example.com", "Password123!") },
			body:       `{"username":"bob2","email":"bob@example.com","password":"Password123!"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `{"errors":["Email has already been taken"]}`,
		},
		{
			name:       "空のフィールドはすべてのメッセージ付きで 422 を返す",
			body:       `{"username":"","email":"","password":""}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `{"errors":["Username can't be blank","Email can't be blank","Password can't be blank"]}`,
		},
		{
			name:       "確認用パスワードが一致しないと 422 を返す",
			body:       `{"username":"eve","email":"eve@example.com","password":"Password123!","password_confirmation":"other"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `{"errors":["Password confirmation doesn't match Password"]}`,
		},
		{
			name:       "弱いパスワード（短く記号なし）は 2 件のメッセージ付きで 422 を返す",
			body:       `{"username":"eve","email":"eve@example.com","password":"abc123"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `{"errors":["Password is too short (minimum is 8 characters)","Password must include letters, numbers and symbols"]}`,
		},
		{
			name:       "パスワードが空だと blank のメッセージだけで 422 を返す",
			body:       `{"username":"eve","email":"eve@example.com","password":""}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `{"errors":["Password can't be blank"]}`,
		},
		{
			name:       "記号のない 8 バイトのパスワードは文字種のメッセージだけで 422 を返す",
			body:       `{"username":"eve","email":"eve@example.com","password":"abcd1234"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `{"errors":["Password must include letters, numbers and symbols"]}`,
		},
		{
			name:       "規則を満たす強いパスワードは 201 を返す",
			body:       `{"username":"eve","email":"eve@example.com","password":"Abcdef1!"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "未知の余分なフィールドは無視される",
			body:       `{"username":"carol","email":"carol@example.com","password":"Password123!","future_field":true}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "不正な JSON は 400 を返す",
			body:       `{"username":`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"invalid JSON body"}`,
		},
		{
			name:       "空の body は 400 を返す",
			body:       "",
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"invalid JSON body"}`,
		},
		{
			name:       "repository の失敗は 500 を返す",
			setup:      func(repo *userRepoFake) { repo.err = io.ErrUnexpectedEOF },
			body:       `{"username":"dan","email":"dan@example.com","password":"Password123!"}`,
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"error":"internal server error"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, auth, _ := newAuthKit()
			if tt.setup != nil {
				tt.setup(repo)
			}
			rec := do(newTestRouterWith(t, okPinger, auth), http.MethodPost, "/signup", tt.body, "")
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

// TestLogin は AC3 を扱う：正しい認証情報はトークンを伴う 200 を返し、誤った
// パスワードと未知の email はどちらも Rails parity の 401 を返す。
func TestLogin(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(repo *userRepoFake)
		body       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "AC3 正しい認証情報は token 付きで 200 を返す",
			body:       `{"email":"alice@example.com","password":"Password123!"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "AC3 誤ったパスワードは 401 を返す",
			body:       `{"email":"alice@example.com","password":"wrong"}`,
			wantStatus: http.StatusUnauthorized,
			wantBody:   `{"error":"Invalid email or password"}`,
		},
		{
			name:       "未知の email は同じ 401 を返す",
			body:       `{"email":"nobody@example.com","password":"Password123!"}`,
			wantStatus: http.StatusUnauthorized,
			wantBody:   `{"error":"Invalid email or password"}`,
		},
		{
			name:       "不正な JSON は 400 を返す",
			body:       `not json`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"invalid JSON body"}`,
		},
		{
			name:       "JSON の後ろに余分な文字列が続くと 400 を返す",
			body:       `{"email":"a@x","password":"p"}garbage`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"invalid JSON body"}`,
		},
		{
			name:       "repository の失敗は 500 を返す",
			setup:      func(repo *userRepoFake) { repo.err = io.ErrUnexpectedEOF },
			body:       `{"email":"alice@example.com","password":"Password123!"}`,
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"error":"internal server error"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, auth, _ := newAuthKit()
			seeded := repo.seed("alice", "alice@example.com", "Password123!")
			if tt.setup != nil {
				tt.setup(repo)
			}
			rec := do(newTestRouterWith(t, okPinger, auth), http.MethodPost, "/login", tt.body, "")
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantBody != "" {
				if rec.Body.String() != tt.wantBody {
					t.Errorf("body = %q, want %q", rec.Body.String(), tt.wantBody)
				}
				return
			}
			user := decodeAuthUser(t, rec.Body.Bytes())
			if user.ID != seeded.ID || user.Username != "alice" || user.Email != "alice@example.com" || user.Admin {
				t.Errorf("login body = %+v, want the seeded user", user)
			}
			if user.Token == "" {
				t.Error("login token is empty")
			}
		})
	}
}

// TestLoginLegacyWeakPassword は、旧ルールで作られたアカウントを締め出さない
// ことを固定する。login は強度を検証しない（Story #38 AC9 / R4）ので、
// 現行の規則を満たさない弱いパスワードのユーザーでも、digest が一致すれば
// token 付きで 200 を返す。
func TestLoginLegacyWeakPassword(t *testing.T) {
	for _, weak := range []string{"weakpw", "a"} {
		t.Run(weak, func(t *testing.T) {
			repo, auth, _ := newAuthKit()
			seeded := repo.seed("legacy", "legacy@example.com", weak)
			body := fmt.Sprintf(`{"email":"legacy@example.com","password":%q}`, weak)
			rec := do(newTestRouterWith(t, okPinger, auth), http.MethodPost, "/login", body, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
			}
			user := decodeAuthUser(t, rec.Body.Bytes())
			if user.ID != seeded.ID || user.Token == "" {
				t.Errorf("login body = %+v, want the seeded user with a token", user)
			}
		})
	}
}

// TestRequireAuth は、保護された POST /logout ルートに対する AC4 と AC5 を
// 扱う：トークンの欠落、Bearer 以外、改ざん、secret 違い、期限切れのトークン、
// 未知のユーザーのトークン、discard 済みのユーザーのトークンは、いずれも
// 正確な Rails parity の 401 body を返し、有効なトークンは通る。
func TestRequireAuth(t *testing.T) {
	repo, auth, codec := newAuthKit()
	alice := repo.seed("alice", "alice@example.com", "Password123!")
	discarded := repo.seed("gone", "gone@example.com", "Password123!")
	repo.users[discarded.ID].discarded = true

	validToken, err := codec.Issue(alice.ID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	expiredToken, err := infra.NewJWTCodec(testJWTSecret, -time.Minute).Issue(alice.ID)
	if err != nil {
		t.Fatalf("issue expired token: %v", err)
	}
	wrongSecretToken, err := infra.NewJWTCodec("other-secret", time.Hour).Issue(alice.ID)
	if err != nil {
		t.Fatalf("issue wrong-secret token: %v", err)
	}
	discardedToken, err := codec.Issue(discarded.ID)
	if err != nil {
		t.Fatalf("issue discarded-user token: %v", err)
	}
	unknownToken, err := codec.Issue(9999)
	if err != nil {
		t.Fatalf("issue unknown-user token: %v", err)
	}

	router := newTestRouterWith(t, okPinger, auth)
	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{name: "有効なトークンは通る", authHeader: "Bearer " + validToken, wantStatus: http.StatusOK},
		{name: "小文字の bearer スキームは通る (RFC 6750)", authHeader: "bearer " + validToken, wantStatus: http.StatusOK},
		{name: "大文字の BEARER スキームは通る (RFC 6750)", authHeader: "BEARER " + validToken, wantStatus: http.StatusOK},
		{name: "スキームの後ろに余分な空白があっても通る (RFC 6750)", authHeader: "Bearer  " + validToken, wantStatus: http.StatusOK},
		{name: "AC4 トークンなしは拒否される", authHeader: "", wantStatus: http.StatusUnauthorized},
		{name: "AC4 Bearer 以外のスキームは拒否される", authHeader: "Token " + validToken, wantStatus: http.StatusUnauthorized},
		{name: "Basic スキームは拒否される", authHeader: "Basic " + validToken, wantStatus: http.StatusUnauthorized},
		{name: "スキームなしの生のトークンは拒否される", authHeader: validToken, wantStatus: http.StatusUnauthorized},
		{name: "AC4 空の Bearer トークンは拒否される", authHeader: "Bearer ", wantStatus: http.StatusUnauthorized},
		{name: "AC4 改ざんされたトークンは拒否される", authHeader: "Bearer " + validToken + "x", wantStatus: http.StatusUnauthorized},
		{name: "AC4 別の secret で署名されたトークンは拒否される", authHeader: "Bearer " + wrongSecretToken, wantStatus: http.StatusUnauthorized},
		{name: "AC4 期限切れのトークンは拒否される", authHeader: "Bearer " + expiredToken, wantStatus: http.StatusUnauthorized},
		{name: "未知のユーザーのトークンは拒否される", authHeader: "Bearer " + unknownToken, wantStatus: http.StatusUnauthorized},
		{name: "AC5 discard 済みのユーザーのトークンは拒否される", authHeader: "Bearer " + discardedToken, wantStatus: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(router, http.MethodPost, "/logout", "", tt.authHeader)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantStatus == http.StatusUnauthorized {
				if got := rec.Body.String(); got != `{"error":"Unauthorized"}` {
					t.Errorf("body = %q, want the exact Rails-parity 401 body", got)
				}
			}
		})
	}
}

// TestRequireAuthInfraFailure：構文上有効なトークンを解決する際の repository の
// 失敗は、認証の判断ではなくサーバー側の障害であり、RequireAuth は 401 ではなく
// 500 を返す。
func TestRequireAuthInfraFailure(t *testing.T) {
	repo, auth, codec := newAuthKit()
	alice := repo.seed("alice", "alice@example.com", "Password123!")
	token, err := codec.Issue(alice.ID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	repo.err = io.ErrUnexpectedEOF

	rec := do(newTestRouterWith(t, okPinger, auth), http.MethodPost, "/logout", "", "Bearer "+token)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusInternalServerError, rec.Body)
	}
	if got := rec.Body.String(); got != `{"error":"internal server error"}` {
		t.Errorf("body = %q, want the 500 JSON error", got)
	}
}

// TestOptionalAuth は、probe handler を直接ラップして AC6 を扱う：トークンが
// ない、または無効な場合は 401 にならず匿名のまま続行し、有効なトークンは
// viewer を request context に入れる。
func TestOptionalAuth(t *testing.T) {
	repo, auth, codec := newAuthKit()
	alice := repo.seed("alice", "alice@example.com", "Password123!")
	token, err := codec.Issue(alice.ID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	tests := []struct {
		name       string
		authHeader string
		wantViewer bool
	}{
		{name: "AC6 トークンがなければ匿名のまま実行される", authHeader: "", wantViewer: false},
		{name: "AC6 無効なトークンなら匿名のまま実行される", authHeader: "Bearer not-a-token", wantViewer: false},
		{name: "AC6 有効なトークンなら viewer が得られる", authHeader: "Bearer " + token, wantViewer: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var viewer domain.User
			var ok bool
			probe := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				viewer, ok = handler.ViewerFrom(r.Context())
				w.WriteHeader(http.StatusOK)
			})

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/probe", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			handler.OptionalAuth(auth)(probe).ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (downstream must always run)", rec.Code, http.StatusOK)
			}
			if ok != tt.wantViewer {
				t.Fatalf("viewer present = %v, want %v", ok, tt.wantViewer)
			}
			if tt.wantViewer && viewer.ID != alice.ID {
				t.Errorf("viewer.ID = %d, want %d", viewer.ID, alice.ID)
			}
		})
	}
}

// TestOptionalAuthInfraFailure：OptionalAuth は repository の失敗時でも決して
// 拒否せず、下流の handler は匿名のまま実行される。
func TestOptionalAuthInfraFailure(t *testing.T) {
	repo, auth, codec := newAuthKit()
	alice := repo.seed("alice", "alice@example.com", "Password123!")
	token, err := codec.Issue(alice.ID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	repo.err = io.ErrUnexpectedEOF

	var viewerPresent bool
	probe := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, viewerPresent = handler.ViewerFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.OptionalAuth(auth)(probe).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (downstream must always run)", rec.Code, http.StatusOK)
	}
	if viewerPresent {
		t.Error("viewer present on repository failure, want anonymous")
	}
}

// TestSignupBodyTooLarge は decodeJSON の 413 経路を実行する：1 MiB を超える
// chunked body（Content-Length なし）は、decoder が読み込む間に
// http.MaxBytesReader を作動させ、JSON のエラー形式で 413 を返す。
func TestSignupBodyTooLarge(t *testing.T) {
	// httptest.NewRequest が Content-Length を設定できないように reader を
	// ラップし、事前の limitBody のチェックを迂回する。
	body := struct{ io.Reader }{strings.NewReader(`{"username":"` + strings.Repeat("a", 2<<20))}
	req := httptest.NewRequest(http.MethodPost, "/signup", body)
	rec := httptest.NewRecorder()
	newTestRouter(t, okPinger).ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusRequestEntityTooLarge, rec.Body)
	}
	if got := rec.Body.String(); got != `{"error":"request body too large"}` {
		t.Errorf("body = %q, want the 413 JSON error", got)
	}
}
