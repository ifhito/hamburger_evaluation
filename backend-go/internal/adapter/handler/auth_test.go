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

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/handler"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/infra"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

const testJWTSecret = "handler-test-secret"

// hasherFake is a fast stand-in for bcrypt with the same contract.
type hasherFake struct{}

func (hasherFake) Hash(password string) (string, error) { return "digest:" + password, nil }
func (hasherFake) Compare(digest, password string) error {
	if digest != "digest:"+password {
		return domain.ErrInvalidCredentials
	}
	return nil
}

// fakeRecord is a stored user inside userRepoFake.
type fakeRecord struct {
	user      domain.User
	digest    string
	discarded bool
}

// userRepoFake is an in-memory usecase.UserRepository. Setting err makes
// every operation fail with it (drives the 500 paths).
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

// seed stores an active user with the hasherFake digest for password.
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

// newAuthKit builds the real auth usecase over the in-memory fakes and a
// real JWT codec, ready for router-level tests.
func newAuthKit() (*userRepoFake, *usecase.Auth, *infra.JWTCodec) {
	repo := newUserRepoFake()
	codec := infra.NewJWTCodec(testJWTSecret, time.Hour)
	return repo, usecase.NewAuth(repo, hasherFake{}, codec, codec), codec
}

// newTestRouter is the default router for tests that only need db health
// or routing behavior.
func newTestRouter(p handler.Pinger) http.Handler {
	_, auth, _ := newAuthKit()
	return newTestRouterWith(p, auth)
}

// newTestRouterWith wires the router with the given auth and empty
// in-memory shop/review fakes, for tests that do not care about that data.
func newTestRouterWith(p handler.Pinger, auth *usecase.Auth) http.Handler {
	return handler.NewRouter(p, auth, usecase.NewShops(&shopRepoFake{}), usecase.NewReviews(newReviewRepoFake()))
}

// do runs one request through the router in-process and returns the
// recorder.
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

// TestSignupThenLogout covers AC1: a fresh signup returns 201 with the
// full snake_case body and a token that immediately opens the protected
// POST /logout route.
func TestSignupThenLogout(t *testing.T) {
	_, auth, _ := newAuthKit()
	router := newTestRouterWith(okPinger, auth)

	rec := do(router, http.MethodPost, "/signup",
		`{"username":"alice","email":"alice@example.com","password":"password123","password_confirmation":"password123"}`, "")
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

// TestSignupErrors covers AC2 (taken email -> 422) plus the decode and
// failure paths of POST /signup.
func TestSignupErrors(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(repo *userRepoFake)
		body       string
		wantStatus int
		wantBody   string // exact body; empty means only status is asserted
	}{
		{
			name:       "AC2 taken email returns 422",
			setup:      func(repo *userRepoFake) { repo.seed("bob", "bob@example.com", "password123") },
			body:       `{"username":"bob2","email":"bob@example.com","password":"password123"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `{"errors":["Email has already been taken"]}`,
		},
		{
			name:       "blank fields return 422 with all messages",
			body:       `{"username":"","email":"","password":""}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `{"errors":["Username can't be blank","Email can't be blank","Password can't be blank"]}`,
		},
		{
			name:       "mismatched confirmation returns 422",
			body:       `{"username":"eve","email":"eve@example.com","password":"password123","password_confirmation":"other"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `{"errors":["Password confirmation doesn't match Password"]}`,
		},
		{
			name:       "unknown extra fields are ignored",
			body:       `{"username":"carol","email":"carol@example.com","password":"password123","future_field":true}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "malformed JSON returns 400",
			body:       `{"username":`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"invalid JSON body"}`,
		},
		{
			name:       "empty body returns 400",
			body:       "",
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"invalid JSON body"}`,
		},
		{
			name:       "repository failure returns 500",
			setup:      func(repo *userRepoFake) { repo.err = io.ErrUnexpectedEOF },
			body:       `{"username":"dan","email":"dan@example.com","password":"password123"}`,
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
			rec := do(newTestRouterWith(okPinger, auth), http.MethodPost, "/signup", tt.body, "")
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

// TestLogin covers AC3: correct credentials return 200 with a token,
// wrong password and unknown email both return the Rails-parity 401.
func TestLogin(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(repo *userRepoFake)
		body       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "AC3 correct credentials return 200 with token",
			body:       `{"email":"alice@example.com","password":"password123"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "AC3 wrong password returns 401",
			body:       `{"email":"alice@example.com","password":"wrong"}`,
			wantStatus: http.StatusUnauthorized,
			wantBody:   `{"error":"Invalid email or password"}`,
		},
		{
			name:       "unknown email returns the same 401",
			body:       `{"email":"nobody@example.com","password":"password123"}`,
			wantStatus: http.StatusUnauthorized,
			wantBody:   `{"error":"Invalid email or password"}`,
		},
		{
			name:       "malformed JSON returns 400",
			body:       `not json`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"invalid JSON body"}`,
		},
		{
			name:       "trailing garbage after JSON returns 400",
			body:       `{"email":"a@x","password":"p"}garbage`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"invalid JSON body"}`,
		},
		{
			name:       "repository failure returns 500",
			setup:      func(repo *userRepoFake) { repo.err = io.ErrUnexpectedEOF },
			body:       `{"email":"alice@example.com","password":"password123"}`,
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"error":"internal server error"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, auth, _ := newAuthKit()
			seeded := repo.seed("alice", "alice@example.com", "password123")
			if tt.setup != nil {
				tt.setup(repo)
			}
			rec := do(newTestRouterWith(okPinger, auth), http.MethodPost, "/login", tt.body, "")
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

// TestRequireAuth covers AC4 and AC5 on the protected POST /logout route:
// missing, non-Bearer, tampered, wrong-secret, and expired tokens, tokens
// of unknown users, and tokens of discarded users all return the exact
// Rails-parity 401 body; a valid token passes.
func TestRequireAuth(t *testing.T) {
	repo, auth, codec := newAuthKit()
	alice := repo.seed("alice", "alice@example.com", "password123")
	discarded := repo.seed("gone", "gone@example.com", "password123")
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

	router := newTestRouterWith(okPinger, auth)
	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{name: "valid token passes", authHeader: "Bearer " + validToken, wantStatus: http.StatusOK},
		{name: "lowercase bearer scheme passes (RFC 6750)", authHeader: "bearer " + validToken, wantStatus: http.StatusOK},
		{name: "uppercase BEARER scheme passes (RFC 6750)", authHeader: "BEARER " + validToken, wantStatus: http.StatusOK},
		{name: "extra whitespace after scheme passes (RFC 6750)", authHeader: "Bearer  " + validToken, wantStatus: http.StatusOK},
		{name: "AC4 no token", authHeader: "", wantStatus: http.StatusUnauthorized},
		{name: "AC4 non-Bearer scheme", authHeader: "Token " + validToken, wantStatus: http.StatusUnauthorized},
		{name: "Basic scheme is rejected", authHeader: "Basic " + validToken, wantStatus: http.StatusUnauthorized},
		{name: "bare token without scheme is rejected", authHeader: validToken, wantStatus: http.StatusUnauthorized},
		{name: "AC4 empty bearer token", authHeader: "Bearer ", wantStatus: http.StatusUnauthorized},
		{name: "AC4 tampered token", authHeader: "Bearer " + validToken + "x", wantStatus: http.StatusUnauthorized},
		{name: "AC4 token signed with another secret", authHeader: "Bearer " + wrongSecretToken, wantStatus: http.StatusUnauthorized},
		{name: "AC4 expired token", authHeader: "Bearer " + expiredToken, wantStatus: http.StatusUnauthorized},
		{name: "token of unknown user", authHeader: "Bearer " + unknownToken, wantStatus: http.StatusUnauthorized},
		{name: "AC5 token of discarded user", authHeader: "Bearer " + discardedToken, wantStatus: http.StatusUnauthorized},
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

// TestRequireAuthInfraFailure: a repository failure while resolving a
// syntactically valid token is a server fault, not an authentication
// decision — RequireAuth answers 500, not 401.
func TestRequireAuthInfraFailure(t *testing.T) {
	repo, auth, codec := newAuthKit()
	alice := repo.seed("alice", "alice@example.com", "password123")
	token, err := codec.Issue(alice.ID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	repo.err = io.ErrUnexpectedEOF

	rec := do(newTestRouterWith(okPinger, auth), http.MethodPost, "/logout", "", "Bearer "+token)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusInternalServerError, rec.Body)
	}
	if got := rec.Body.String(); got != `{"error":"internal server error"}` {
		t.Errorf("body = %q, want the 500 JSON error", got)
	}
}

// TestOptionalAuth covers AC6 by wrapping a probe handler directly: no or
// invalid tokens continue anonymously without a 401, a valid token puts
// the viewer into the request context.
func TestOptionalAuth(t *testing.T) {
	repo, auth, codec := newAuthKit()
	alice := repo.seed("alice", "alice@example.com", "password123")
	token, err := codec.Issue(alice.ID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	tests := []struct {
		name       string
		authHeader string
		wantViewer bool
	}{
		{name: "AC6 no token runs anonymously", authHeader: "", wantViewer: false},
		{name: "AC6 invalid token runs anonymously", authHeader: "Bearer not-a-token", wantViewer: false},
		{name: "AC6 valid token yields viewer", authHeader: "Bearer " + token, wantViewer: true},
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

// TestOptionalAuthInfraFailure: OptionalAuth never rejects, even on a
// repository failure — the downstream handler still runs anonymously.
func TestOptionalAuthInfraFailure(t *testing.T) {
	repo, auth, codec := newAuthKit()
	alice := repo.seed("alice", "alice@example.com", "password123")
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

// TestSignupBodyTooLarge exercises the decodeJSON 413 path: a chunked
// body (no Content-Length) over 1 MiB trips http.MaxBytesReader while the
// decoder reads, yielding 413 with the JSON error shape.
func TestSignupBodyTooLarge(t *testing.T) {
	// Wrap the reader so httptest.NewRequest cannot set Content-Length,
	// bypassing the up-front limitBody check.
	body := struct{ io.Reader }{strings.NewReader(`{"username":"` + strings.Repeat("a", 2<<20))}
	req := httptest.NewRequest(http.MethodPost, "/signup", body)
	rec := httptest.NewRecorder()
	newTestRouter(okPinger).ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusRequestEntityTooLarge, rec.Body)
	}
	if got := rec.Body.String(); got != `{"error":"request body too large"}` {
		t.Errorf("body = %q, want the 413 JSON error", got)
	}
}
