package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/handler"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/infra"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// userRepoFake's usecase.UsersRepository half (the auth half lives in
// auth_test.go), mirroring the real repository's error mapping.

func (f *userRepoFake) ListActiveUsers(_ context.Context) ([]domain.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]domain.User, 0, len(f.users))
	for _, rec := range f.users {
		if !rec.discarded {
			out = append(out, rec.user)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *userRepoFake) UpdateUserProfile(_ context.Context, id int64, changes usecase.ProfileChanges) (domain.User, error) {
	if f.err != nil {
		return domain.User{}, f.err
	}
	rec, ok := f.users[id]
	if !ok || rec.discarded {
		return domain.User{}, domain.ErrUserNotFound
	}
	if changes.Email != nil {
		// The unique index spans discarded users too, like users_email_key.
		for otherID, other := range f.users {
			if otherID != id && other.user.Email == *changes.Email {
				return domain.User{}, domain.ErrEmailTaken
			}
		}
	}
	if changes.Username != nil {
		rec.user.Username = *changes.Username
	}
	if changes.Email != nil {
		rec.user.Email = *changes.Email
	}
	if changes.PasswordDigest != nil {
		rec.digest = *changes.PasswordDigest
	}
	return rec.user, nil
}

func (f *userRepoFake) DiscardUser(_ context.Context, id int64) error {
	if f.err != nil {
		return f.err
	}
	rec, ok := f.users[id]
	if !ok || rec.discarded {
		return domain.ErrUserNotFound
	}
	rec.discarded = true
	return nil
}

// newUsersRouter wires the router with auth and users over the SAME
// in-memory user repository, so profile changes are visible to logins.
func newUsersRouter(t *testing.T) (*userRepoFake, http.Handler, func(int64) string) {
	t.Helper()
	repo, auth, codec := newAuthKit()
	router := handler.NewRouter(okPinger, auth, usecase.NewShops(&shopRepoFake{}),
		usecase.NewReviews(newReviewRepoFake(), nil), usecase.NewUsers(repo, hasherFake{}), nil)
	token := func(id int64) string {
		t.Helper()
		tok, err := codec.Issue(id)
		if err != nil {
			t.Fatalf("issue token for %d: %v", id, err)
		}
		return "Bearer " + tok
	}
	return repo, router, token
}

// TestListUsers pins GET /users: public, kept users only, id ascending,
// as a plain top-level array of the token-less user shape.
func TestListUsers(t *testing.T) {
	t.Run("anonymous request returns kept users id ascending", func(t *testing.T) {
		repo, router, _ := newUsersRouter(t)
		repo.seed("alice", "alice@example.com", "password123")
		repo.seed("bob", "bob@example.com", "password123")
		ghost := repo.seed("ghost", "ghost@example.com", "password123")
		repo.users[ghost.ID].discarded = true

		rec := do(router, http.MethodGet, "/users", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		want := `[{"id":1,"username":"alice","email":"alice@example.com","admin":false},` +
			`{"id":2,"username":"bob","email":"bob@example.com","admin":false}]`
		if got := rec.Body.String(); got != want {
			t.Errorf("body = %s, want %s (ghost hidden, no token field)", got, want)
		}
	})

	t.Run("no users marshals as []", func(t *testing.T) {
		_, router, _ := newUsersRouter(t)
		rec := do(router, http.MethodGet, "/users", "", "")
		if rec.Code != http.StatusOK || rec.Body.String() != `[]` {
			t.Errorf("status/body = %d %s, want 200 []", rec.Code, rec.Body)
		}
	})

	t.Run("repository failure returns 500", func(t *testing.T) {
		repo, router, _ := newUsersRouter(t)
		repo.err = fmt.Errorf("db down")
		rec := do(router, http.MethodGet, "/users", "", "")
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusInternalServerError, rec.Body)
		}
	})
}

// TestUpdateUser covers PUT /users/{id}: AC1 (self update reflected in
// the index), AC2 (403 for another existing user, 404 for a nonexistent
// id), AC3 (taken email 422), and the Rails-parity validation edges.
func TestUpdateUser(t *testing.T) {
	setup := func(t *testing.T) (*userRepoFake, http.Handler, string, string) {
		t.Helper()
		repo, router, token := newUsersRouter(t)
		alice := repo.seed("alice", "alice@example.com", "password123")
		bob := repo.seed("bob", "bob@example.com", "password123")
		return repo, router, token(alice.ID), token(bob.ID)
	}

	t.Run("AC1 self username update returns 200 and is reflected in GET /users", func(t *testing.T) {
		_, router, aliceAuth, _ := setup(t)
		rec := do(router, http.MethodPut, "/users/1", `{"user":{"username":"alice2"}}`, aliceAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		want := `{"id":1,"username":"alice2","email":"alice@example.com","admin":false}`
		if got := rec.Body.String(); got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
		list := do(router, http.MethodGet, "/users", "", "")
		if got := list.Body.String(); !strings.Contains(got, `"username":"alice2"`) || strings.Contains(got, `"username":"alice"`) {
			t.Errorf("GET /users = %s, want the new username reflected", got)
		}
	})

	t.Run("AC2 another existing user's id returns 403", func(t *testing.T) {
		_, router, _, bobAuth := setup(t)
		rec := do(router, http.MethodPut, "/users/1", `{"user":{"username":"hacked"}}`, bobAuth)
		if rec.Code != http.StatusForbidden || rec.Body.String() != `{"error":"Forbidden"}` {
			t.Errorf("status/body = %d %s, want 403 Forbidden", rec.Code, rec.Body)
		}
	})

	t.Run("AC2 nonexistent and non-numeric ids return 404 regardless of ownership", func(t *testing.T) {
		_, router, aliceAuth, _ := setup(t)
		for _, path := range []string{"/users/999", "/users/abc"} {
			rec := do(router, http.MethodPut, path, `{"user":{"username":"x"}}`, aliceAuth)
			if rec.Code != http.StatusNotFound || rec.Body.String() != `{"error":"User not found"}` {
				t.Errorf("PUT %s = %d %s, want 404 User not found", path, rec.Code, rec.Body)
			}
		}
	})

	t.Run("AC3 email changed to another user's email returns 422", func(t *testing.T) {
		_, router, aliceAuth, _ := setup(t)
		rec := do(router, http.MethodPut, "/users/1", `{"user":{"email":"bob@example.com"}}`, aliceAuth)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusUnprocessableEntity, rec.Body)
		}
		if got, want := rec.Body.String(), `{"errors":["Email has already been taken"]}`; got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})

	t.Run("empty user object and absent user key return 200 unchanged", func(t *testing.T) {
		_, router, aliceAuth, _ := setup(t)
		want := `{"id":1,"username":"alice","email":"alice@example.com","admin":false}`
		for _, body := range []string{`{"user":{}}`, `{}`} {
			rec := do(router, http.MethodPut, "/users/1", body, aliceAuth)
			if rec.Code != http.StatusOK || rec.Body.String() != want {
				t.Errorf("body %s: status/body = %d %s, want 200 %s", body, rec.Code, rec.Body, want)
			}
		}
	})

	t.Run("empty-string password is a no-op on the digest", func(t *testing.T) {
		repo, router, aliceAuth, _ := setup(t)
		rec := do(router, http.MethodPut, "/users/1", `{"user":{"password":""}}`, aliceAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		if got := repo.users[1].digest; got != "digest:password123" {
			t.Errorf("stored digest = %q, want the untouched %q", got, "digest:password123")
		}
	})

	t.Run("validation failures return 422 with the exact messages", func(t *testing.T) {
		tests := []struct {
			name     string
			body     string
			wantBody string
		}{
			{
				name:     "blank username",
				body:     `{"user":{"username":""}}`,
				wantBody: `{"errors":["Username can't be blank"]}`,
			},
			{
				name:     "blank email",
				body:     `{"user":{"email":""}}`,
				wantBody: `{"errors":["Email can't be blank"]}`,
			},
			{
				name:     "mismatched confirmation",
				body:     `{"user":{"password":"newpassword1","password_confirmation":"other"}}`,
				wantBody: `{"errors":["Password confirmation doesn't match Password"]}`,
			},
			{
				name:     "password over 72 bytes",
				body:     fmt.Sprintf(`{"user":{"password":%q}}`, strings.Repeat("a", 73)),
				wantBody: `{"errors":["Password is too long (maximum is 72 characters)"]}`,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, router, aliceAuth, _ := setup(t)
				rec := do(router, http.MethodPut, "/users/1", tt.body, aliceAuth)
				if rec.Code != http.StatusUnprocessableEntity {
					t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusUnprocessableEntity, rec.Body)
				}
				if got := rec.Body.String(); got != tt.wantBody {
					t.Errorf("body = %s, want %s", got, tt.wantBody)
				}
			})
		}
	})

	t.Run("malformed and empty JSON bodies return 400", func(t *testing.T) {
		_, router, aliceAuth, _ := setup(t)
		for _, body := range []string{`{"user":`, ""} {
			rec := do(router, http.MethodPut, "/users/1", body, aliceAuth)
			if rec.Code != http.StatusBadRequest || rec.Body.String() != `{"error":"invalid JSON body"}` {
				t.Errorf("body %q: status/body = %d %s, want 400 invalid JSON body", body, rec.Code, rec.Body)
			}
		}
	})
}

// TestDeleteUser covers DELETE /users/{id}: AC2's 403/404 split and the
// AC4 kernel (204 without a body, dead token, gone from the index).
func TestDeleteUser(t *testing.T) {
	setup := func(t *testing.T) (http.Handler, string, string) {
		t.Helper()
		repo, router, token := newUsersRouter(t)
		alice := repo.seed("alice", "alice@example.com", "password123")
		bob := repo.seed("bob", "bob@example.com", "password123")
		return router, token(alice.ID), token(bob.ID)
	}

	t.Run("AC2 another existing user's id returns 403", func(t *testing.T) {
		router, _, bobAuth := setup(t)
		rec := do(router, http.MethodDelete, "/users/1", "", bobAuth)
		if rec.Code != http.StatusForbidden || rec.Body.String() != `{"error":"Forbidden"}` {
			t.Errorf("status/body = %d %s, want 403 Forbidden", rec.Code, rec.Body)
		}
	})

	t.Run("AC2 nonexistent and non-numeric ids return 404", func(t *testing.T) {
		router, aliceAuth, _ := setup(t)
		for _, path := range []string{"/users/999", "/users/abc"} {
			rec := do(router, http.MethodDelete, path, "", aliceAuth)
			if rec.Code != http.StatusNotFound || rec.Body.String() != `{"error":"User not found"}` {
				t.Errorf("DELETE %s = %d %s, want 404 User not found", path, rec.Code, rec.Body)
			}
		}
	})

	t.Run("self delete returns 204, kills the token, and leaves the index", func(t *testing.T) {
		router, aliceAuth, _ := setup(t)
		rec := do(router, http.MethodDelete, "/users/1", "", aliceAuth)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusNoContent, rec.Body)
		}
		if rec.Body.Len() != 0 {
			t.Errorf("body = %q, want empty", rec.Body.String())
		}
		if again := do(router, http.MethodPost, "/logout", "", aliceAuth); again.Code != http.StatusUnauthorized {
			t.Errorf("protected request after discard = %d, want 401", again.Code)
		}
		list := do(router, http.MethodGet, "/users", "", "")
		if got := list.Body.String(); strings.Contains(got, `"username":"alice"`) {
			t.Errorf("GET /users = %s, want alice gone", got)
		}
	})
}

// TestUsersRequireAuth pins the 401 boundary of the user routes: the
// writes reject anonymous requests before any repository access while the
// index stays open.
func TestUsersRequireAuth(t *testing.T) {
	_, router, _ := newUsersRouter(t)
	const unauthorized = `{"error":"Unauthorized"}`

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPut, path: "/users/1", body: `{"user":{"username":"x"}}`},
		{method: http.MethodDelete, path: "/users/1"},
	}
	for _, tt := range tests {
		rec := do(router, tt.method, tt.path, tt.body, "")
		if rec.Code != http.StatusUnauthorized || rec.Body.String() != unauthorized {
			t.Errorf("%s %s = %d %s, want 401 %s", tt.method, tt.path, rec.Code, rec.Body, unauthorized)
		}
	}

	if rec := do(router, http.MethodGet, "/users", "", ""); rec.Code != http.StatusOK {
		t.Errorf("GET /users anonymous = %d, want 200 (public index)", rec.Code)
	}
}

// newUsersIntegrationKit wires the full router over real repositories,
// real bcrypt, and real JWT on a fresh per-run database (skipped without
// TEST_DATABASE_URL, inside dbtest).
func newUsersIntegrationKit(t *testing.T) (*pgx.Conn, http.Handler) {
	t.Helper()
	conn, _ := dbtest.New(t)
	userRepo := repository.NewUserRepository(conn)
	hasher := infra.BcryptPasswordHasher{}
	codec := infra.NewJWTCodec(testJWTSecret, time.Hour)
	auth := usecase.NewAuth(userRepo, hasher, codec, codec)
	router := handler.NewRouter(conn, auth,
		usecase.NewShops(repository.NewShopRepository(conn)),
		usecase.NewReviews(repository.NewReviewRepository(conn), nil),
		usecase.NewUsers(userRepo, hasher), nil)
	return conn, router
}

// signupUser registers a user through POST /signup and returns its id and
// Bearer header.
func signupUser(t *testing.T, router http.Handler, username, email, password string) (int64, string) {
	t.Helper()
	body := fmt.Sprintf(`{"username":%q,"email":%q,"password":%q}`, username, email, password)
	rec := do(router, http.MethodPost, "/signup", body, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("signup %s: status = %d (body %s)", username, rec.Code, rec.Body)
	}
	user := decodeAuthUser(t, rec.Body.Bytes())
	return user.ID, "Bearer " + user.Token
}

// TestUsersPasswordChangeIntegration covers AC6 end to end: after a
// password update through PUT /users/{id}, the old password no longer
// logs in (401) while the new one does (200) — real bcrypt, real JWT,
// real PostgreSQL.
func TestUsersPasswordChangeIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed integration test in short mode")
	}
	_, router := newUsersIntegrationKit(t)
	id, bearer := signupUser(t, router, "alice", "alice@example.com", "oldpassword1")

	body := `{"user":{"password":"newpassword1","password_confirmation":"newpassword1"}}`
	rec := do(router, http.MethodPut, fmt.Sprintf("/users/%d", id), body, bearer)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
	}
	want := fmt.Sprintf(`{"id":%d,"username":"alice","email":"alice@example.com","admin":false}`, id)
	if got := rec.Body.String(); got != want {
		t.Errorf("update body = %s, want %s", got, want)
	}

	old := do(router, http.MethodPost, "/login", `{"email":"alice@example.com","password":"oldpassword1"}`, "")
	if old.Code != http.StatusUnauthorized || old.Body.String() != `{"error":"Invalid email or password"}` {
		t.Errorf("old-password login = %d %s, want the Rails-parity 401", old.Code, old.Body)
	}
	fresh := do(router, http.MethodPost, "/login", `{"email":"alice@example.com","password":"newpassword1"}`, "")
	if fresh.Code != http.StatusOK {
		t.Errorf("new-password login = %d, want 200 (body %s)", fresh.Code, fresh.Body)
	}
}

// usersFeedItem is the slice of the review JSON these integration
// assertions care about.
type usersFeedItem struct {
	ID   int64 `json:"id"`
	User struct {
		ID int64 `json:"id"`
	} `json:"user"`
	Burger struct {
		ID          int64 `json:"id"`
		ReviewCount int64 `json:"review_count"`
	} `json:"burger"`
}

// TestUsersDiscardPropagationIntegration is the AC4+AC5 退会の波及
// scenario over HTTP against a real database: user A reviews a shared
// burger (also reviewed by B) and a solo burger, deletes their account,
// and everything downstream — token, index, feed, detail, shop reviews,
// burger stats — forgets A while keeping B intact.
func TestUsersDiscardPropagationIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed integration test in short mode")
	}
	ctx := context.Background()
	conn, router := newUsersIntegrationKit(t)

	aliceID, aliceAuth := signupUser(t, router, "alice", "alice@example.com", "password123")
	bobID, bobAuth := signupUser(t, router, "bob", "bob@example.com", "password123")

	// One approved (active) shop serving two burgers, seeded directly —
	// moderation is out of this scenario's scope.
	var shopID, sharedID, soloID int64
	if err := conn.QueryRow(ctx, `INSERT INTO shops (name, status) VALUES ('Active One', 1) RETURNING id`).Scan(&shopID); err != nil {
		t.Fatalf("insert shop: %v", err)
	}
	for name, dst := range map[string]*int64{"Shared": &sharedID, "Solo": &soloID} {
		if err := conn.QueryRow(ctx, `INSERT INTO burgers (name) VALUES ($1) RETURNING id`, name).Scan(dst); err != nil {
			t.Fatalf("insert burger %s: %v", name, err)
		}
	}
	for _, burgerID := range []int64{sharedID, soloID} {
		if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shopID, burgerID); err != nil {
			t.Fatalf("link burger %d: %v", burgerID, err)
		}
	}

	postReview := func(auth string, burgerID int64, rating int, comment string) int64 {
		t.Helper()
		body := fmt.Sprintf(`{"review":{"rating":%d,"comment":%q,"shop_id":%d,"burger_id":%d}}`, rating, comment, shopID, burgerID)
		rec := do(router, http.MethodPost, "/reviews", body, auth)
		if rec.Code != http.StatusCreated {
			t.Fatalf("post review: status = %d (body %s)", rec.Code, rec.Body)
		}
		var resp struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode review: %v", err)
		}
		return resp.ID
	}
	aliceShared := postReview(aliceAuth, sharedID, 2, "meh")
	aliceSolo := postReview(aliceAuth, soloID, 5, "only mine")
	bobShared := postReview(bobAuth, sharedID, 4, "good")

	// AC4: A discards themself — 204 with an empty body.
	rec := do(router, http.MethodDelete, fmt.Sprintf("/users/%d", aliceID), "", aliceAuth)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d (body %s)", rec.Code, http.StatusNoContent, rec.Body)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("delete body = %q, want empty", rec.Body.String())
	}

	// A's still-unexpired token no longer opens protected endpoints.
	if rec := do(router, http.MethodPost, "/logout", "", aliceAuth); rec.Code != http.StatusUnauthorized {
		t.Errorf("discarded user's token = %d, want 401 (body %s)", rec.Code, rec.Body)
	}

	// GET /users forgets A and keeps B.
	rec = do(router, http.MethodGet, "/users", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list users status = %d (body %s)", rec.Code, rec.Body)
	}
	var listed []struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode users: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != bobID {
		t.Errorf("GET /users = %s, want only bob (id %d)", rec.Body, bobID)
	}

	// AC5: the feed hides A's reviews, keeps B's, and the shared burger's
	// displayed review_count (1) equals the number of its feed reviews.
	rec = do(router, http.MethodGet, "/reviews", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("feed status = %d (body %s)", rec.Code, rec.Body)
	}
	var feed []usersFeedItem
	if err := json.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("decode feed: %v", err)
	}
	if len(feed) != 1 || feed[0].ID != bobShared || feed[0].User.ID != bobID {
		t.Fatalf("feed = %s, want exactly bob's review %d", rec.Body, bobShared)
	}
	sharedInFeed := 0
	for _, item := range feed {
		if item.Burger.ID == sharedID {
			sharedInFeed++
		}
	}
	if feed[0].Burger.ReviewCount != 1 || int(feed[0].Burger.ReviewCount) != sharedInFeed {
		t.Errorf("shared burger review_count = %d with %d feed reviews, want both 1",
			feed[0].Burger.ReviewCount, sharedInFeed)
	}

	// A's reviews are gone from the detail endpoint, indistinguishable
	// from never-existing ones.
	for _, id := range []int64{aliceShared, aliceSolo} {
		rec := do(router, http.MethodGet, fmt.Sprintf("/reviews/%d", id), "", "")
		if rec.Code != http.StatusNotFound || rec.Body.String() != `{"error":"Review not found"}` {
			t.Errorf("GET /reviews/%d = %d %s, want 404 Review not found", id, rec.Code, rec.Body)
		}
	}

	// The shop detail lists only B's review.
	rec = do(router, http.MethodGet, fmt.Sprintf("/shops/%d", shopID), "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("shop detail status = %d (body %s)", rec.Code, rec.Body)
	}
	var shopDetail struct {
		Reviews []usersFeedItem `json:"reviews"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &shopDetail); err != nil {
		t.Fatalf("decode shop detail: %v", err)
	}
	if len(shopDetail.Reviews) != 1 || shopDetail.Reviews[0].ID != bobShared {
		t.Fatalf("shop reviews = %s, want exactly bob's review %d", rec.Body, bobShared)
	}
	if got := shopDetail.Reviews[0].Burger.ReviewCount; got != 1 {
		t.Errorf("shop detail shared review_count = %d, want 1", got)
	}

	// The stored stats agree: shared keeps only B's rating, solo drops to
	// the zero row (recalculated in the discard's transaction).
	assertStats := func(burgerID, wantCount int64, wantAvg float64) {
		t.Helper()
		var count int64
		var avg float64
		if err := conn.QueryRow(ctx, `SELECT review_count, average_rating FROM burger_stats WHERE burger_id = $1`, burgerID).Scan(&count, &avg); err != nil {
			t.Fatalf("select stats for burger %d: %v", burgerID, err)
		}
		if count != wantCount || avg != wantAvg {
			t.Errorf("burger %d stats = (count %d, avg %v), want (count %d, avg %v)", burgerID, count, avg, wantCount, wantAvg)
		}
	}
	assertStats(sharedID, 1, 4.0)
	assertStats(soloID, 0, 0.0)
}
