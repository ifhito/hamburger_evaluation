package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/handler"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/infra"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/storage"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uowtest"
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

// fakeRecord は userStoreFake の中に保存されたユーザーである。
type fakeRecord struct {
	user      domain.User
	digest    string
	discarded bool
}

// userStoreFake は in-memory の usecase.UserQuery かつ domain.UserRepository
// である。in-memory の fake は共有 DB の代役なので、読み書きで状態を共有する
// よう 1 つの型に保つ（読み書きの分離は、usecase の Query の引数型と domain の
// 書き込みオブジェクトの引数型がコンパイル時に保証する）。err を設定するとすべての操作がその err で失敗する（500 の経路を
// 駆動する）。
type userStoreFake struct {
	seq   int
	users map[string]*fakeRecord
	err   error
}

var (
	_ usecase.UserQuery     = (*userStoreFake)(nil)
	_ domain.UserRepository = (*userStoreFake)(nil)
)

func newUserStoreFake() *userStoreFake { return &userStoreFake{users: map[string]*fakeRecord{}} }

func (f *userStoreFake) CreateUser(_ context.Context, p domain.CreateUserParams) (domain.User, error) {
	if f.err != nil {
		return domain.User{}, f.err
	}
	for _, rec := range f.users {
		if rec.user.Email == p.Email {
			return domain.User{}, domain.ErrEmailTaken
		}
	}
	f.seq++
	user := domain.User{ID: uid.N(f.seq), Username: p.Username, Email: p.Email, Admin: p.Admin}
	f.users[user.ID] = &fakeRecord{user: user, digest: p.PasswordDigest}
	return user, nil
}

func (f *userStoreFake) GetActiveUserByEmail(_ context.Context, email string) (usecase.UserCredentials, error) {
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

func (f *userStoreFake) GetActiveUserByID(_ context.Context, id string) (domain.User, error) {
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
func (f *userStoreFake) seed(username, email, password string) domain.User {
	user, err := f.CreateUser(context.Background(), domain.CreateUserParams{
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
func newAuthKit() (*userStoreFake, *usecase.Auth, *infra.JWTCodec) {
	repo := newUserStoreFake()
	codec := infra.NewJWTCodec(testJWTSecret, time.Hour)
	return repo, usecase.NewAuth(repo, hasherFake{}, codec, codec), codec
}

// unusedSignups は、signup を使わないテストの router に渡す Signups である。保存先もメールの
// 送り先も、その場で捨てる fake になっている。
func unusedSignups() *usecase.Signups {
	codec := infra.NewJWTCodec(testJWTSecret, time.Hour)
	users := newUserStoreFake()
	store := newSignupStoreFake(users)
	return usecase.NewSignups(users, domain.NewSignupVerifications(store),
		&uowtest.UoW{Users: users, SignupVerifications: store, PendingSignups: store},
		hasherFake{}, &mailRecorder{}, codec, testSignupConfig)
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
	reviewRepo := newReviewStoreFake()
	users := newUserStoreFake()
	shopRepo := &shopStoreFake{}
	return handler.NewRouter(p, auth, unusedSignups(), usecase.NewShops(shopRepo, domain.NewShops(shopRepo)),
		reviewsUsecase(reviewRepo, storage.NewDisk(t.TempDir(), "/photos")),
		usersUsecase(users, hasherFake{}), nil, nil, nil)
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
	ID       string `json:"id"`
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

// TestSignupErrors は、POST /signup の検証エラー（422）・decode 経路・失敗経路を扱う。
// 登録の有無に依存する応答（登録済みの email）は signup_test.go が扱う。
func TestSignupErrors(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(repo *userStoreFake)
		body       string
		wantStatus int
		wantBody   string // 完全一致させる body。空なら status のみを検証する
	}{
		{
			name:       "空のフィールドはすべてのメッセージ付きで 422 を返す",
			body:       `{"username":"","email":"","password":""}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `{"errors":["Username can't be blank","Email can't be blank","Password can't be blank"]}`,
		},
		{
			name:       "形式が不正な email は Email is invalid で 422 を返す",
			body:       `{"username":"eve","email":"abc","password":"Password123!"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `{"errors":["Email is invalid"]}`,
		},
		{
			name:       "表示名つきの email も Email is invalid で 422 を返す",
			body:       `{"username":"eve","email":"Eve <eve@example.com>","password":"Password123!"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `{"errors":["Email is invalid"]}`,
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
			name:       "規則を満たす強いパスワードは 202 を返す",
			body:       `{"username":"eve","email":"eve@example.com","password":"Abcdef1!"}`,
			wantStatus: http.StatusAccepted,
			wantBody:   `{"message":"Confirmation email sent"}`,
		},
		{
			name:       "未知の余分なフィールドは無視される",
			body:       `{"username":"carol","email":"carol@example.com","password":"Password123!","future_field":true}`,
			wantStatus: http.StatusAccepted,
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
			name:       "email の検索の失敗は 500 を返す",
			setup:      func(repo *userStoreFake) { repo.err = io.ErrUnexpectedEOF },
			body:       `{"username":"dan","email":"dan@example.com","password":"Password123!"}`,
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"error":"internal server error"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kit := newSignupKit(t)
			if tt.setup != nil {
				tt.setup(kit.users)
			}
			rec := do(kit.router, http.MethodPost, "/signup", tt.body, "")
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

// TestLogin はログインの結果を扱う：正しい認証情報はトークンを伴う 200 を返し、誤った
// パスワードと未知の email はどちらも Rails parity の 401 を返す。
func TestLogin(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(repo *userStoreFake)
		body       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "正しい認証情報でログインすると、トークンつきで 200 が返る",
			body:       `{"email":"alice@example.com","password":"Password123!"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "誤ったパスワードでログインすると 401 になる",
			body:       `{"email":"alice@example.com","password":"Wrongpass1!"}`,
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
			setup:      func(repo *userStoreFake) { repo.err = io.ErrUnexpectedEOF },
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

// TestLoginValidation は、認証情報が signup と同じ規則を満たさないとき、login が
// 401 ではなく 422 {"errors":[...]}（signup と同じ形・同じメッセージ）を返し、
// 規則を満たしたうえで誤っている場合だけ 401 になることを固定する。
func TestLoginValidation(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{"email と password が空なら 422 で blank を返す", `{"email":"","password":""}`, http.StatusUnprocessableEntity,
			`{"errors":["Email can't be blank","Password can't be blank"]}`},
		{"フィールドが無い body も空として 422 を返す", `{}`, http.StatusUnprocessableEntity,
			`{"errors":["Email can't be blank","Password can't be blank"]}`},
		{"形式の合わない email は 422 を返す", `{"email":"abc","password":"Password123!"}`, http.StatusUnprocessableEntity,
			`{"errors":["Email is invalid"]}`},
		{"強度を満たさない password は 422 を返す", `{"email":"alice@example.com","password":"weakpassword"}`, http.StatusUnprocessableEntity,
			`{"errors":["Password must include letters, numbers and symbols"]}`},
		{"規則を満たす誤った password は 401 を返す", `{"email":"alice@example.com","password":"Wrongpass1!"}`, http.StatusUnauthorized,
			`{"error":"Invalid email or password"}`},
		{"規則を満たす未知の email は 401 を返す(アカウントの有無で応答が変わらない)", `{"email":"nobody@example.com","password":"Wrongpass1!"}`, http.StatusUnauthorized,
			`{"error":"Invalid email or password"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, auth, _ := newAuthKit()
			repo.seed("alice", "alice@example.com", "Password123!")
			rec := do(newTestRouterWith(t, okPinger, auth), http.MethodPost, "/login", tt.body, "")
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if rec.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

// TestRequireAuth は、保護された POST /logout ルートの認証を
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
	unknownToken, err := codec.Issue(uid.N(9999))
	if err != nil {
		t.Fatalf("issue unknown-user token: %v", err)
	}

	// user_id が数値だった旧形式のトークン(ID を UUID にする前の発行形式)は、署名と期限が正しくても無効になる。
	legacyToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.MapClaims{"user_id": 1, "exp": time.Now().Add(time.Hour).Unix()}).SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("issue legacy numeric token: %v", err)
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
		{name: "トークンがないリクエストは拒否される", authHeader: "", wantStatus: http.StatusUnauthorized},
		{name: "Bearer 以外の認証方式は拒否される", authHeader: "Token " + validToken, wantStatus: http.StatusUnauthorized},
		{name: "Basic スキームは拒否される", authHeader: "Basic " + validToken, wantStatus: http.StatusUnauthorized},
		{name: "スキームなしの生のトークンは拒否される", authHeader: validToken, wantStatus: http.StatusUnauthorized},
		{name: "空の Bearer トークンは拒否される", authHeader: "Bearer ", wantStatus: http.StatusUnauthorized},
		{name: "改ざんされたトークンは拒否される", authHeader: "Bearer " + validToken + "x", wantStatus: http.StatusUnauthorized},
		{name: "別の秘密鍵で署名されたトークンは拒否される", authHeader: "Bearer " + wrongSecretToken, wantStatus: http.StatusUnauthorized},
		{name: "期限切れのトークンは拒否される", authHeader: "Bearer " + expiredToken, wantStatus: http.StatusUnauthorized},
		{name: "未知のユーザーのトークンは拒否される", authHeader: "Bearer " + unknownToken, wantStatus: http.StatusUnauthorized},
		{name: "ユーザー ID が数値の旧形式のトークンは拒否される", authHeader: "Bearer " + legacyToken, wantStatus: http.StatusUnauthorized},
		{name: "退会済み(discard 済み)のユーザーのトークンは拒否される", authHeader: "Bearer " + discardedToken, wantStatus: http.StatusUnauthorized},
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

// TestOptionalAuth は、probe handler を直接ラップして扱う：トークンが
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
		{name: "トークンがなければ、匿名のまま実行される", authHeader: "", wantViewer: false},
		{name: "無効なトークンでも、匿名のまま実行される", authHeader: "Bearer not-a-token", wantViewer: false},
		{name: "有効なトークンなら、閲覧者(viewer)が得られる", authHeader: "Bearer " + token, wantViewer: true},
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
				t.Errorf("viewer.ID = %s, want %s", viewer.ID, alice.ID)
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
