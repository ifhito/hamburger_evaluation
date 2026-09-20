package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/handler"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/infra"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/storage"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// userRepoFake の usecase.UsersRepository の半分（auth の半分は auth_test.go に
// ある）で、本物の repository のエラーの対応づけを再現している。

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
		// unique index は、users_email_key と同様に、discard 済みの
		// ユーザーにも及ぶ。
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

// newUsersRouter は、「同一の」in-memory のユーザー repository の上で auth と
// users を router に配線するので、profile の変更がログインから見える。
func newUsersRouter(t *testing.T) (*userRepoFake, http.Handler, func(int64) string) {
	t.Helper()
	repo, auth, codec := newAuthKit()
	router := handler.NewRouter(okPinger, auth, usecase.NewShops(&shopRepoFake{}),
		usecase.NewReviews(newReviewRepoFake(), storage.NewDisk(t.TempDir(), "/photos")),
		usecase.NewUsers(repo, hasherFake{}), nil)
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

// ユーザーの JSON のキー集合（ソート済み、カンマ区切り）。email と admin は
// 値が null や空であっても「キーが存在する」時点で不合格にしたいので、値ではなく
// キー集合で検証する。
const (
	publicUserKeys = "id,username"
	selfUserKeys   = "admin,email,id,username"
)

// userKeySet は JSON オブジェクトのキーをソートしてカンマで連結する。
func userKeySet(obj map[string]any) string {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

// decodeUserObject は body を単一の JSON オブジェクトとして読む。struct ではなく
// map に読むのは、キーが存在しないことを検証するためである。
func decodeUserObject(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		t.Fatalf("body %q is not a JSON object: %v", body, err)
	}
	return obj
}

// decodeUserList は body を JSON オブジェクトの配列として読む。
func decodeUserList(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var list []map[string]any
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("body %q is not a JSON array of objects: %v", body, err)
	}
	return list
}

// userObjectID は JSON オブジェクトの id を int64 で返す。
func userObjectID(t *testing.T, obj map[string]any) int64 {
	t.Helper()
	id, ok := obj["id"].(float64)
	if !ok {
		t.Fatalf("id = %v (%T), want a JSON number", obj["id"], obj["id"])
	}
	return int64(id)
}

// listUsers は GET /users を実行し、200 であることを確かめてから配列を返す。
func listUsers(t *testing.T, router http.Handler, path, auth string) []map[string]any {
	t.Helper()
	rec := do(router, http.MethodGet, path, "", auth)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want %d (body %s)", path, rec.Code, http.StatusOK, rec.Body)
	}
	return decodeUserList(t, rec.Body.Bytes())
}

// assertUserIDs は list の id が want と（順序も含めて）一致することを確かめる。
func assertUserIDs(t *testing.T, list []map[string]any, want ...int64) {
	t.Helper()
	got := make([]int64, 0, len(list))
	for _, obj := range list {
		got = append(got, userObjectID(t, obj))
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("ids = %v, want %v", got, want)
	}
}

// assertPublicUserList は、list のすべての要素が公開ビュー {id, username} の
// キーだけを持つことを確かめる。
func assertPublicUserList(t *testing.T, list []map[string]any) {
	t.Helper()
	for _, obj := range list {
		if got := userKeySet(obj); got != publicUserKeys {
			t.Errorf("user %v: keys = [%s], want [%s] (email/admin must be absent, not null)", obj["id"], got, publicUserKeys)
		}
	}
}

// newUsersListSetup は、alice(1)、discard 済みの ghost(2)、bob(3)、admin の
// root(4) を seed した router を返す。id に欠番があるので、discard 済みが id の
// 昇順や件数に紛れ込まないことも確かめられる。
func newUsersListSetup(t *testing.T) (*userRepoFake, http.Handler, func(int64) string) {
	t.Helper()
	repo, router, token := newUsersRouter(t)
	repo.seed("alice", "alice@example.com", "password123")
	ghost := repo.seed("ghost", "ghost@example.com", "password123")
	repo.users[ghost.ID].discarded = true
	repo.seed("bob", "bob@example.com", "password123")
	root := repo.seed("root", "root@example.com", "password123")
	repo.users[root.ID].user.Admin = true
	return repo, router, token
}

// TestListUsers は GET /users を固定する：認証は任意で、kept なユーザーのみを
// id の昇順で、viewer から見えるビューのトップレベルの単純な配列として返す。
// email と admin は viewer 本人の要素にだけ入り、他人・匿名には「キー自体が
// 存在しない」。
func TestListUsers(t *testing.T) {
	t.Run("AC1 匿名のリクエストは kept なユーザーを id の昇順で {id, username} だけで返す", func(t *testing.T) {
		_, router, _ := newUsersListSetup(t)

		rec := do(router, http.MethodGet, "/users", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		list := decodeUserList(t, rec.Body.Bytes())
		assertUserIDs(t, list, 1, 3, 4)
		assertPublicUserList(t, list)
		if got := []any{list[0]["username"], list[1]["username"], list[2]["username"]}; fmt.Sprint(got) != "[alice bob root]" {
			t.Errorf("usernames = %v, want [alice bob root]", got)
		}
		if body := rec.Body.String(); strings.Contains(body, "@") {
			t.Errorf("body = %s, want no email address anywhere", body)
		}
	})

	t.Run("AC2 ログイン中の viewer 自身の要素だけが本人ビューになる", func(t *testing.T) {
		tests := []struct {
			name      string
			viewerID  int64
			wantEmail string
			wantAdmin bool
		}{
			{name: "一般ユーザー alice は admin=false のキーを含む本人ビューを得る", viewerID: 1, wantEmail: "alice@example.com", wantAdmin: false},
			{name: "一般ユーザー bob は自分の要素だけが本人ビューになる", viewerID: 3, wantEmail: "bob@example.com", wantAdmin: false},
			{name: "admin の root でも他人の要素は公開ビューのままで自分の要素だけ本人ビューになる", viewerID: 4, wantEmail: "root@example.com", wantAdmin: true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, router, token := newUsersListSetup(t)
				list := listUsers(t, router, "/users", token(tt.viewerID))
				assertUserIDs(t, list, 1, 3, 4)
				selfSeen := 0
				for _, obj := range list {
					if userObjectID(t, obj) != tt.viewerID {
						if got := userKeySet(obj); got != publicUserKeys {
							t.Errorf("other user %v: keys = [%s], want [%s]", obj["id"], got, publicUserKeys)
						}
						continue
					}
					selfSeen++
					if got := userKeySet(obj); got != selfUserKeys {
						t.Errorf("self: keys = [%s], want [%s]", got, selfUserKeys)
					}
					if obj["email"] != tt.wantEmail {
						t.Errorf("self email = %v, want %q", obj["email"], tt.wantEmail)
					}
					if obj["admin"] != tt.wantAdmin {
						t.Errorf("self admin = %v (%T), want %v", obj["admin"], obj["admin"], tt.wantAdmin)
					}
				}
				if selfSeen != 1 {
					t.Errorf("self elements = %d, want exactly 1", selfSeen)
				}
			})
		}
	})

	t.Run("認証できない Authorization は 401 にならず匿名扱いで公開ビューだけを返す", func(t *testing.T) {
		expired, err := infra.NewJWTCodec(testJWTSecret, -time.Minute).Issue(1)
		if err != nil {
			t.Fatalf("issue expired token: %v", err)
		}
		wrongSecret, err := infra.NewJWTCodec("other-secret", time.Hour).Issue(1)
		if err != nil {
			t.Fatalf("issue wrong-secret token: %v", err)
		}
		_, router, token := newUsersListSetup(t)
		validAlice := strings.TrimPrefix(token(1), "Bearer ")

		tests := []struct {
			name string
			auth string
		}{
			{name: "不正な token", auth: "Bearer garbage"},
			{name: "空の Bearer token", auth: "Bearer "},
			{name: "期限切れの token", auth: "Bearer " + expired},
			{name: "別の secret で署名された token", auth: "Bearer " + wrongSecret},
			{name: "存在しないユーザーの token", auth: token(9999)},
			{name: "退会済みユーザーの token", auth: token(2)},
			{name: "Bearer 以外の scheme (Token)", auth: "Token " + validAlice},
			{name: "Bearer 以外の scheme (Basic)", auth: "Basic " + validAlice},
			{name: "scheme のない生の token", auth: validAlice},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				list := listUsers(t, router, "/users", tt.auth)
				assertUserIDs(t, list, 1, 3, 4)
				assertPublicUserList(t, list)
			})
		}
	})

	t.Run("ユーザーがいなければ [] として marshal される", func(t *testing.T) {
		_, router, _ := newUsersRouter(t)
		rec := do(router, http.MethodGet, "/users", "", "")
		if rec.Code != http.StatusOK || rec.Body.String() != `[]` {
			t.Errorf("status/body = %d %s, want 200 []", rec.Code, rec.Body)
		}
	})

	t.Run("repository の失敗は 500 を返す", func(t *testing.T) {
		repo, router, _ := newUsersRouter(t)
		repo.err = fmt.Errorf("db down")
		rec := do(router, http.MethodGet, "/users", "", "")
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusInternalServerError, rec.Body)
		}
	})
}

// TestGetUser は GET /users/{id} を扱う：認証は任意で、匿名・他人・admin の他人には
// {id, username} だけ（email と admin はキーごと存在しない）、本人には
// {id, username, email, admin}。存在しない・退会済み・整数でない id は同一の 404
// になる。
func TestGetUser(t *testing.T) {
	get := func(router http.Handler, path, auth string) *httptest.ResponseRecorder {
		return do(router, http.MethodGet, path, "", auth)
	}

	t.Run("AC3 匿名は {id, username} のキーだけを返す", func(t *testing.T) {
		_, router, _ := newUsersListSetup(t)
		rec := get(router, "/users/1", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		obj := decodeUserObject(t, rec.Body.Bytes())
		if got := userKeySet(obj); got != publicUserKeys {
			t.Errorf("keys = [%s], want [%s]", got, publicUserKeys)
		}
		if userObjectID(t, obj) != 1 || obj["username"] != "alice" {
			t.Errorf("body = %s, want id 1 alice", rec.Body)
		}
		if strings.Contains(rec.Body.String(), "alice@example.com") {
			t.Errorf("body = %s, want no email", rec.Body)
		}
	})

	t.Run("AC4 本人が閲覧すると {id, username, email, admin} を返す", func(t *testing.T) {
		tests := []struct {
			name      string
			id        int64
			wantEmail string
			wantAdmin bool
		}{
			{name: "一般ユーザーは admin=false のキーを含む", id: 1, wantEmail: "alice@example.com", wantAdmin: false},
			{name: "admin は admin=true を含む", id: 4, wantEmail: "root@example.com", wantAdmin: true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, router, token := newUsersListSetup(t)
				rec := get(router, fmt.Sprintf("/users/%d", tt.id), token(tt.id))
				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
				}
				obj := decodeUserObject(t, rec.Body.Bytes())
				if got := userKeySet(obj); got != selfUserKeys {
					t.Errorf("keys = [%s], want [%s]", got, selfUserKeys)
				}
				if obj["email"] != tt.wantEmail || obj["admin"] != tt.wantAdmin {
					t.Errorf("body = %s, want email %q admin %v", rec.Body, tt.wantEmail, tt.wantAdmin)
				}
			})
		}
	})

	t.Run("AC5 他人が閲覧すると公開ビューだけを返し email は body に現れない", func(t *testing.T) {
		tests := []struct {
			name      string
			viewerID  int64
			targetID  int64
			leakEmail string
		}{
			{name: "一般ユーザー alice から見た bob", viewerID: 1, targetID: 3, leakEmail: "bob@example.com"},
			{name: "一般ユーザー bob から見た alice", viewerID: 3, targetID: 1, leakEmail: "alice@example.com"},
			{name: "admin の root から見た一般ユーザー alice", viewerID: 4, targetID: 1, leakEmail: "alice@example.com"},
			{name: "一般ユーザー alice から見た admin の root", viewerID: 1, targetID: 4, leakEmail: "root@example.com"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, router, token := newUsersListSetup(t)
				rec := get(router, fmt.Sprintf("/users/%d", tt.targetID), token(tt.viewerID))
				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
				}
				obj := decodeUserObject(t, rec.Body.Bytes())
				if got := userKeySet(obj); got != publicUserKeys {
					t.Errorf("keys = [%s], want [%s]", got, publicUserKeys)
				}
				if userObjectID(t, obj) != tt.targetID {
					t.Errorf("id = %v, want %d", obj["id"], tt.targetID)
				}
				if body := rec.Body.String(); strings.Contains(body, tt.leakEmail) || strings.Contains(body, "@") {
					t.Errorf("body = %s, want no email", body)
				}
			})
		}
	})

	t.Run("AC5 認証できない Authorization では本人の id でも公開ビューだけを返す", func(t *testing.T) {
		_, router, token := newUsersListSetup(t)
		validAlice := strings.TrimPrefix(token(1), "Bearer ")
		tests := []struct {
			name string
			path string
			auth string
		}{
			{name: "不正な token で本人の id を見る", path: "/users/1", auth: "Bearer garbage"},
			{name: "Bearer 以外の scheme で本人の id を見る", path: "/users/1", auth: "Token " + validAlice},
			{name: "scheme のない生の token で本人の id を見る", path: "/users/1", auth: validAlice},
			{name: "退会済みユーザーの token では alice の id も公開ビューになる", path: "/users/1", auth: token(2)},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				rec := get(router, tt.path, tt.auth)
				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
				}
				if got := userKeySet(decodeUserObject(t, rec.Body.Bytes())); got != publicUserKeys {
					t.Errorf("keys = [%s], want [%s]", got, publicUserKeys)
				}
			})
		}
	})

	t.Run("AC6 存在しない・退会済み・整数でない id は status/body/Content-Type がすべて同一の 404 になる", func(t *testing.T) {
		const wantBody = `{"error":"User not found"}`
		const wantContentType = "application/json; charset=utf-8"
		paths := []string{
			"/users/999",                  // 存在しない
			"/users/2",                    // 退会済み (ghost)
			"/users/abc",                  // 整数でない
			"/users/1.5",                  // 整数でない
			"/users/-1",                   // 整数だが存在しない
			"/users/99999999999999999999", // int64 に収まらない
		}
		_, router, token := newUsersListSetup(t)
		for name, auth := range map[string]string{"匿名": "", "ログイン中の alice": token(1)} {
			t.Run(name, func(t *testing.T) {
				for _, path := range paths {
					rec := get(router, path, auth)
					if rec.Code != http.StatusNotFound {
						t.Errorf("GET %s: status = %d, want %d", path, rec.Code, http.StatusNotFound)
					}
					if got := rec.Body.String(); got != wantBody {
						t.Errorf("GET %s: body = %s, want %s", path, got, wantBody)
					}
					if got := rec.Header().Get("Content-Type"); got != wantContentType {
						t.Errorf("GET %s: Content-Type = %q, want %q", path, got, wantContentType)
					}
				}
			})
		}
	})

	t.Run("repository の失敗は 500 を返す", func(t *testing.T) {
		repo, router, token := newUsersListSetup(t)
		repo.err = fmt.Errorf("db down")
		for _, auth := range []string{"", token(1)} {
			rec := get(router, "/users/1", auth)
			if rec.Code != http.StatusInternalServerError || rec.Body.String() != `{"error":"internal server error"}` {
				t.Errorf("status/body = %d %s, want 500 internal server error", rec.Code, rec.Body)
			}
		}
	})
}

// TestUpdateUser は PUT /users/{id} を扱う：AC1（自分自身の更新が index に反映
// される）、AC2（存在する別のユーザーには 403、存在しない id には 404）、
// AC3（既に使われている email は 422）、そして Rails parity の validation の
// 境界ケース。
func TestUpdateUser(t *testing.T) {
	setup := func(t *testing.T) (*userRepoFake, http.Handler, string, string) {
		t.Helper()
		repo, router, token := newUsersRouter(t)
		alice := repo.seed("alice", "alice@example.com", "password123")
		bob := repo.seed("bob", "bob@example.com", "password123")
		return repo, router, token(alice.ID), token(bob.ID)
	}

	t.Run("AC1 自分自身の username 更新は 200 を返し GET /users に反映される", func(t *testing.T) {
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

	t.Run("AC2 存在する別のユーザーの id は 403 を返す", func(t *testing.T) {
		_, router, _, bobAuth := setup(t)
		rec := do(router, http.MethodPut, "/users/1", `{"user":{"username":"hacked"}}`, bobAuth)
		if rec.Code != http.StatusForbidden || rec.Body.String() != `{"error":"Forbidden"}` {
			t.Errorf("status/body = %d %s, want 403 Forbidden", rec.Code, rec.Body)
		}
	})

	t.Run("AC2 存在しない id と非数値の id は所有権に関わらず 404 を返す", func(t *testing.T) {
		_, router, aliceAuth, _ := setup(t)
		for _, path := range []string{"/users/999", "/users/abc"} {
			rec := do(router, http.MethodPut, path, `{"user":{"username":"x"}}`, aliceAuth)
			if rec.Code != http.StatusNotFound || rec.Body.String() != `{"error":"User not found"}` {
				t.Errorf("PUT %s = %d %s, want 404 User not found", path, rec.Code, rec.Body)
			}
		}
	})

	t.Run("AC3 email を別のユーザーの email に変更すると 422 を返す", func(t *testing.T) {
		_, router, aliceAuth, _ := setup(t)
		rec := do(router, http.MethodPut, "/users/1", `{"user":{"email":"bob@example.com"}}`, aliceAuth)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusUnprocessableEntity, rec.Body)
		}
		if got, want := rec.Body.String(), `{"errors":["Email has already been taken"]}`; got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})

	t.Run("空の user オブジェクトと user キーなしは変更なしで 200 を返す", func(t *testing.T) {
		_, router, aliceAuth, _ := setup(t)
		want := `{"id":1,"username":"alice","email":"alice@example.com","admin":false}`
		for _, body := range []string{`{"user":{}}`, `{}`} {
			rec := do(router, http.MethodPut, "/users/1", body, aliceAuth)
			if rec.Code != http.StatusOK || rec.Body.String() != want {
				t.Errorf("body %s: status/body = %d %s, want 200 %s", body, rec.Code, rec.Body, want)
			}
		}
	})

	t.Run("空文字列の password は digest に影響しない", func(t *testing.T) {
		repo, router, aliceAuth, _ := setup(t)
		rec := do(router, http.MethodPut, "/users/1", `{"user":{"password":""}}`, aliceAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		if got := repo.users[1].digest; got != "digest:password123" {
			t.Errorf("stored digest = %q, want the untouched %q", got, "digest:password123")
		}
	})

	t.Run("検証エラーは正確なメッセージ付きで 422 を返す", func(t *testing.T) {
		tests := []struct {
			name     string
			body     string
			wantBody string
		}{
			{
				name:     "空の username は検証エラーになる",
				body:     `{"user":{"username":""}}`,
				wantBody: `{"errors":["Username can't be blank"]}`,
			},
			{
				name:     "空の email は検証エラーになる",
				body:     `{"user":{"email":""}}`,
				wantBody: `{"errors":["Email can't be blank"]}`,
			},
			{
				name:     "確認用パスワードの不一致は検証エラーになる",
				body:     `{"user":{"password":"newpassword1","password_confirmation":"other"}}`,
				wantBody: `{"errors":["Password confirmation doesn't match Password"]}`,
			},
			{
				name:     "72 bytes を超える password は検証エラーになる",
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

	t.Run("不正な JSON の body と空の body は 400 を返す", func(t *testing.T) {
		_, router, aliceAuth, _ := setup(t)
		for _, body := range []string{`{"user":`, ""} {
			rec := do(router, http.MethodPut, "/users/1", body, aliceAuth)
			if rec.Code != http.StatusBadRequest || rec.Body.String() != `{"error":"invalid JSON body"}` {
				t.Errorf("body %q: status/body = %d %s, want 400 invalid JSON body", body, rec.Code, rec.Body)
			}
		}
	})
}

// TestDeleteUser は DELETE /users/{id} を扱う：AC2 の 403/404 の使い分けと、
// AC4 の核（body なしの 204、無効になったトークン、index からの消失）。
func TestDeleteUser(t *testing.T) {
	setup := func(t *testing.T) (http.Handler, string, string) {
		t.Helper()
		repo, router, token := newUsersRouter(t)
		alice := repo.seed("alice", "alice@example.com", "password123")
		bob := repo.seed("bob", "bob@example.com", "password123")
		return router, token(alice.ID), token(bob.ID)
	}

	t.Run("AC2 存在する別のユーザーの id は 403 を返す", func(t *testing.T) {
		router, _, bobAuth := setup(t)
		rec := do(router, http.MethodDelete, "/users/1", "", bobAuth)
		if rec.Code != http.StatusForbidden || rec.Body.String() != `{"error":"Forbidden"}` {
			t.Errorf("status/body = %d %s, want 403 Forbidden", rec.Code, rec.Body)
		}
	})

	t.Run("AC2 存在しない id と非数値の id は 404 を返す", func(t *testing.T) {
		router, aliceAuth, _ := setup(t)
		for _, path := range []string{"/users/999", "/users/abc"} {
			rec := do(router, http.MethodDelete, path, "", aliceAuth)
			if rec.Code != http.StatusNotFound || rec.Body.String() != `{"error":"User not found"}` {
				t.Errorf("DELETE %s = %d %s, want 404 User not found", path, rec.Code, rec.Body)
			}
		}
	})

	t.Run("自分自身を削除すると 204 を返し、トークンが無効になり、index から消える", func(t *testing.T) {
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

// TestUsersRequireAuth はユーザー系のルートの 401 の境界を固定する：書き込み
// （PUT/DELETE）は repository へのアクセスより前に匿名の request を拒否し、
// 一方で GET は（一覧も詳細も）開かれたままである。/users/{id} は 1 つの route に
// 3 つの method を宣言しており、GET を OptionalAuth にしたことで PUT/DELETE の
// RequireAuth が緩んでいないことを確かめる。
func TestUsersRequireAuth(t *testing.T) {
	repo, router, _ := newUsersRouter(t)
	repo.seed("alice", "alice@example.com", "password123")
	const unauthorized = `{"error":"Unauthorized"}`

	tests := []struct {
		method string
		path   string
		body   string
		auth   string
	}{
		{method: http.MethodPut, path: "/users/1", body: `{"user":{"username":"x"}}`},
		{method: http.MethodDelete, path: "/users/1"},
		{method: http.MethodPut, path: "/users/1", body: `{"user":{"username":"x"}}`, auth: "Bearer garbage"},
		{method: http.MethodDelete, path: "/users/1", auth: "Bearer garbage"},
	}
	for _, tt := range tests {
		rec := do(router, tt.method, tt.path, tt.body, tt.auth)
		if rec.Code != http.StatusUnauthorized || rec.Body.String() != unauthorized {
			t.Errorf("%s %s (auth %q) = %d %s, want 401 %s", tt.method, tt.path, tt.auth, rec.Code, rec.Body, unauthorized)
		}
	}
	// 拒否された匿名の PUT/DELETE は何も変更していない。
	if rec := do(router, http.MethodGet, "/users/1", "", ""); rec.Code != http.StatusOK ||
		decodeUserObject(t, rec.Body.Bytes())["username"] != "alice" {
		t.Errorf("GET /users/1 after rejected writes = %d %s, want 200 alice untouched", rec.Code, rec.Body)
	}

	for _, path := range []string{"/users", "/users/1"} {
		if rec := do(router, http.MethodGet, path, "", ""); rec.Code != http.StatusOK {
			t.Errorf("GET %s anonymous = %d, want 200 (optional auth)", path, rec.Code)
		}
	}
}

// newUsersIntegrationKit は、実行ごとに新しく作られる database 上で、本物の
// repository、本物の bcrypt、本物の JWT を使って router 全体を配線する
// （TEST_DATABASE_URL がなければ dbtest の内部で skip される）。
func newUsersIntegrationKit(t *testing.T) (*pgx.Conn, http.Handler) {
	t.Helper()
	conn, _ := dbtest.New(t)
	userRepo := repository.NewUserRepository(conn)
	hasher := infra.BcryptPasswordHasher{}
	codec := infra.NewJWTCodec(testJWTSecret, time.Hour)
	auth := usecase.NewAuth(userRepo, hasher, codec, codec)
	router := handler.NewRouter(conn, auth,
		usecase.NewShops(repository.NewShopRepository(conn)),
		usecase.NewReviews(repository.NewReviewRepository(conn), storage.NewDisk(t.TempDir(), "/photos")),
		usecase.NewUsers(userRepo, hasher), nil)
	return conn, router
}

// signupUser は POST /signup を通じてユーザーを登録し、その id と Bearer
// ヘッダーを返す。
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

// TestUsersPasswordChangeIntegration は AC6 をエンドツーエンドで扱う：
// PUT /users/{id} によるパスワードの更新後は、古いパスワードではもう
// ログインできず（401）、新しいパスワードではログインできる（200）。
// 本物の bcrypt、本物の JWT、本物の PostgreSQL を使う。
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

// TestUsersProfileViewsIntegration は、本物の PostgreSQL・repository・JWT を通して、
// SQL から domain、JSON までの一連で、公開ビューと本人ビューのキー集合が保たれる
// ことを確かめる（AC1〜AC6）。fixture は signup した 3 人を共有し、最後に 1 人を
// 退会させる。
func TestUsersProfileViewsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed integration test in short mode")
	}
	ctx := context.Background()
	conn, router := newUsersIntegrationKit(t)

	aliceID, aliceAuth := signupUser(t, router, "alice", "alice@example.com", "password123")
	bobID, bobAuth := signupUser(t, router, "bob", "bob@example.com", "password123")
	rootID, rootAuth := signupUser(t, router, "root", "root@example.com", "password123")
	if _, err := conn.Exec(ctx, `UPDATE users SET admin = true WHERE id = $1`, rootID); err != nil {
		t.Fatalf("promote root: %v", err)
	}
	const total = 3

	// assertViews は list の各要素が、viewerID（0 = 匿名）に対して正しいビューであること
	// を確かめる：viewerID 自身の要素だけが {id, username, email, admin}、他は公開ビュー。
	assertViews := func(t *testing.T, list []map[string]any, viewerID int64, wantEmail string, wantAdmin bool) {
		t.Helper()
		selfSeen := 0
		for _, obj := range list {
			if viewerID == 0 || userObjectID(t, obj) != viewerID {
				if got := userKeySet(obj); got != publicUserKeys {
					t.Errorf("user %v: keys = [%s], want [%s]", obj["id"], got, publicUserKeys)
				}
				continue
			}
			selfSeen++
			if got := userKeySet(obj); got != selfUserKeys {
				t.Errorf("self: keys = [%s], want [%s]", got, selfUserKeys)
			}
			if obj["email"] != wantEmail || obj["admin"] != wantAdmin {
				t.Errorf("self = %v, want email %q admin %v", obj, wantEmail, wantAdmin)
			}
		}
		if viewerID != 0 && selfSeen != 1 {
			t.Errorf("self elements = %d, want exactly 1", selfSeen)
		}
	}

	t.Run("匿名の一覧と詳細は {id, username} だけを返す", func(t *testing.T) {
		rec := do(router, http.MethodGet, "/users", "", "")
		list := decodeUserList(t, rec.Body.Bytes())
		if rec.Code != http.StatusOK || len(list) != total {
			t.Fatalf("status/len = %d/%d, want 200/%d (body %s)", rec.Code, len(list), total, rec.Body)
		}
		assertViews(t, list, 0, "", false)
		if strings.Contains(rec.Body.String(), "@") {
			t.Errorf("anonymous list contains an email address: %s", rec.Body)
		}

		one := do(router, http.MethodGet, fmt.Sprintf("/users/%d", aliceID), "", "")
		if got := userKeySet(decodeUserObject(t, one.Body.Bytes())); one.Code != http.StatusOK || got != publicUserKeys {
			t.Errorf("anonymous detail = %d keys [%s], want 200 [%s]", one.Code, got, publicUserKeys)
		}
	})

	t.Run("alice の token では alice の要素だけが本人ビューになる", func(t *testing.T) {
		list := listUsers(t, router, "/users", aliceAuth)
		if len(list) != total {
			t.Fatalf("len = %d, want %d", len(list), total)
		}
		assertViews(t, list, aliceID, "alice@example.com", false)

		self := do(router, http.MethodGet, fmt.Sprintf("/users/%d", aliceID), "", aliceAuth)
		if got := userKeySet(decodeUserObject(t, self.Body.Bytes())); self.Code != http.StatusOK || got != selfUserKeys {
			t.Errorf("own detail = %d keys [%s], want 200 [%s]", self.Code, got, selfUserKeys)
		}
		other := do(router, http.MethodGet, fmt.Sprintf("/users/%d", bobID), "", aliceAuth)
		if got := userKeySet(decodeUserObject(t, other.Body.Bytes())); other.Code != http.StatusOK || got != publicUserKeys {
			t.Errorf("other's detail = %d keys [%s], want 200 [%s]", other.Code, got, publicUserKeys)
		}
		if strings.Contains(other.Body.String(), "bob@example.com") {
			t.Errorf("other's detail leaks the email: %s", other.Body)
		}
	})

	t.Run("admin の root でも他人の要素は公開ビューのままで自分の要素だけ本人ビューになる", func(t *testing.T) {
		list := listUsers(t, router, "/users", rootAuth)
		assertViews(t, list, rootID, "root@example.com", true)

		other := do(router, http.MethodGet, fmt.Sprintf("/users/%d", aliceID), "", rootAuth)
		if got := userKeySet(decodeUserObject(t, other.Body.Bytes())); other.Code != http.StatusOK || got != publicUserKeys {
			t.Errorf("admin viewing other = %d keys [%s], want 200 [%s]", other.Code, got, publicUserKeys)
		}
	})

	t.Run("退会したユーザーは一覧から消え、詳細は存在しない id と同一の 404 になる", func(t *testing.T) {
		if rec := do(router, http.MethodDelete, fmt.Sprintf("/users/%d", bobID), "", bobAuth); rec.Code != http.StatusNoContent {
			t.Fatalf("delete bob = %d, want 204 (body %s)", rec.Code, rec.Body)
		}
		gone := do(router, http.MethodGet, fmt.Sprintf("/users/%d", bobID), "", "")
		never := do(router, http.MethodGet, "/users/999999", "", "")
		if gone.Code != http.StatusNotFound || gone.Body.String() != `{"error":"User not found"}` {
			t.Errorf("discarded detail = %d %s, want 404 User not found", gone.Code, gone.Body)
		}
		if gone.Code != never.Code || gone.Body.String() != never.Body.String() ||
			gone.Header().Get("Content-Type") != never.Header().Get("Content-Type") {
			t.Errorf("discarded (%d %s) differs from never-existed (%d %s)", gone.Code, gone.Body, never.Code, never.Body)
		}

		assertUserIDs(t, listUsers(t, router, "/users", ""), aliceID, rootID)
	})
}

// usersFeedItem は、これらの統合テストの assertion が関心を持つ review の
// JSON の一部分である。
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

// TestUsersDiscardPropagationIntegration は、本物の database に対して
// HTTP 越しに行う AC4+AC5 退会の波及のシナリオである：ユーザー A は共有の
// burger（B も review している）と単独の burger を review し、account を
// 削除する。すると下流のすべて（トークン、index、feed、detail、shop の
// review、burger の統計）が A を忘れ、B はそのまま残る。
func TestUsersDiscardPropagationIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed integration test in short mode")
	}
	ctx := context.Background()
	conn, router := newUsersIntegrationKit(t)

	aliceID, aliceAuth := signupUser(t, router, "alice", "alice@example.com", "password123")
	bobID, bobAuth := signupUser(t, router, "bob", "bob@example.com", "password123")

	// 承認済みの（active な）shop 1 件が 2 つの burger を提供し、直接 seed
	// している。moderation はこのシナリオの範囲外である。
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

	// AC4：A は自分自身を discard する。空の body を伴う 204 になる。
	rec := do(router, http.MethodDelete, fmt.Sprintf("/users/%d", aliceID), "", aliceAuth)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d (body %s)", rec.Code, http.StatusNoContent, rec.Body)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("delete body = %q, want empty", rec.Body.String())
	}

	// A の、まだ期限切れになっていないトークンは、もう保護された
	// エンドポイントを通さない。
	if rec := do(router, http.MethodPost, "/logout", "", aliceAuth); rec.Code != http.StatusUnauthorized {
		t.Errorf("discarded user's token = %d, want 401 (body %s)", rec.Code, rec.Body)
	}

	// GET /users は A を忘れ、B を残す。
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

	// AC5：feed は A の review を隠し、B の review を残し、共有の burger に
	// 表示される review_count（1）は feed 上のその burger の review の件数と
	// 等しくなる。
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

	// A の review は detail エンドポイントから消えており、
	// もともと存在しなかった review と区別できない。
	for _, id := range []int64{aliceShared, aliceSolo} {
		rec := do(router, http.MethodGet, fmt.Sprintf("/reviews/%d", id), "", "")
		if rec.Code != http.StatusNotFound || rec.Body.String() != `{"error":"Review not found"}` {
			t.Errorf("GET /reviews/%d = %d %s, want 404 Review not found", id, rec.Code, rec.Body)
		}
	}

	// shop の detail は B の review だけを列挙する。
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

	// 保存された統計も一致する：shared は B の rating だけを保ち、solo は
	// ゼロの行に落ちる（discard の transaction 内で再計算される）。
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
