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
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/storage"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/uow"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// userStoreFake は in-memory の usecase.UserQuery かつ domain.UserRepository で
// ある（型の宣言と読み取りのメソッドは auth_test.go にある）。この位置には書き込み
// 側の UpdateUserProfile / DiscardUser が定義されており、本物の repository の
// エラーの対応づけを再現している。

func (f *userStoreFake) UpdateUserProfile(_ context.Context, id string, changes domain.ProfileChanges) (domain.User, error) {
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

func (f *userStoreFake) DiscardUser(_ context.Context, id string) error {
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
func newUsersRouter(t *testing.T) (*userStoreFake, http.Handler, func(string) string) {
	t.Helper()
	repo, auth, codec := newAuthKit()
	reviewRepo := newReviewStoreFake()
	shopRepo := &shopStoreFake{}
	router := handler.NewRouter(okPinger, auth, usecase.NewShops(shopRepo, domain.NewShops(shopRepo)),
		reviewsUsecase(reviewRepo, storage.NewDisk(t.TempDir(), "/photos")),
		usersUsecase(repo, hasherFake{}), nil)
	token := func(id string) string {
		t.Helper()
		tok, err := codec.Issue(id)
		if err != nil {
			t.Fatalf("issue token for %s: %v", id, err)
		}
		return "Bearer " + tok
	}
	return repo, router, token
}

// ユーザーの JSON のキー集合（ソート済み、カンマ区切り）。email と admin は
// 値が null や空であっても「キーが存在する」時点で不合格にしたいので、値ではなく
// キー集合で検証する。
//
// publicUserKeys は、他のエンドポイントに埋め込まれる user の参照（{id, username}）の
// キー集合である。publicProfileKeys と selfUserKeys は GET /users/{id} のプロフィールで、
// viewer ごとの can_edit（編集・削除できるか）が常に付く。
const (
	publicUserKeys    = "id,username"
	publicProfileKeys = "can_edit,id,username"
	selfUserKeys      = "admin,can_edit,email,id,username"
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

// userObjectID は JSON オブジェクトの id(UUID の文字列)を返す。
func userObjectID(t *testing.T, obj map[string]any) string {
	t.Helper()
	id, ok := obj["id"].(string)
	if !ok {
		t.Fatalf("id = %v (%T), want a JSON string", obj["id"], obj["id"])
	}
	return id
}

// newSeededUsersRouter は、alice(1)、discard 済みの ghost(2)、bob(3)、admin の
// root(4) を seed した router を返す。discard 済みの id 2 が欠番になる。
func newSeededUsersRouter(t *testing.T) (*userStoreFake, http.Handler, func(string) string) {
	t.Helper()
	repo, router, token := newUsersRouter(t)
	repo.seed("alice", "alice@example.com", "Password123!")
	ghost := repo.seed("ghost", "ghost@example.com", "Password123!")
	repo.users[ghost.ID].discarded = true
	repo.seed("bob", "bob@example.com", "Password123!")
	root := repo.seed("root", "root@example.com", "Password123!")
	repo.users[root.ID].user.Admin = true
	return repo, router, token
}

// TestGetUser は GET /users/{id} を扱う：認証は任意で、匿名・他人・admin の他人には
// {id, username} だけ（email と admin はキーごと存在しない）、本人には
// {id, username, email, admin}。存在しない・退会済み・整数でない id は同一の 404
// になる。
func TestGetUser(t *testing.T) {
	get := func(router http.Handler, path, auth string) *httptest.ResponseRecorder {
		return do(router, http.MethodGet, path, "", auth)
	}

	t.Run("AC2 匿名は {id, username} のキーだけを返す", func(t *testing.T) {
		_, router, _ := newSeededUsersRouter(t)
		rec := get(router, "/users/"+uid.N(1), "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		obj := decodeUserObject(t, rec.Body.Bytes())
		if got := userKeySet(obj); got != publicProfileKeys {
			t.Errorf("keys = [%s], want [%s]", got, publicProfileKeys)
		}
		if obj["can_edit"] != false {
			t.Errorf("can_edit = %v, want false（匿名は編集できない）", obj["can_edit"])
		}
		if userObjectID(t, obj) != uid.N(1) || obj["username"] != "alice" {
			t.Errorf("body = %s, want id %s alice", rec.Body, uid.N(1))
		}
		if strings.Contains(rec.Body.String(), "alice@example.com") {
			t.Errorf("body = %s, want no email", rec.Body)
		}
	})

	t.Run("AC3 本人が閲覧すると {id, username, email, admin} を返す", func(t *testing.T) {
		tests := []struct {
			name      string
			id        string
			wantEmail string
			wantAdmin bool
		}{
			{name: "一般ユーザーは admin=false のキーを含む", id: uid.N(1), wantEmail: "alice@example.com", wantAdmin: false},
			{name: "admin は admin=true を含む", id: uid.N(4), wantEmail: "root@example.com", wantAdmin: true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, router, token := newSeededUsersRouter(t)
				rec := get(router, "/users/"+tt.id, token(tt.id))
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
				if obj["can_edit"] != true {
					t.Errorf("can_edit = %v, want true（本人は編集できる）", obj["can_edit"])
				}
			})
		}
	})

	t.Run("AC4 他人が閲覧すると公開ビューだけを返し email は body に現れない", func(t *testing.T) {
		tests := []struct {
			name      string
			viewerID  string
			targetID  string
			leakEmail string
		}{
			{name: "一般ユーザー alice から見た bob", viewerID: uid.N(1), targetID: uid.N(3), leakEmail: "bob@example.com"},
			{name: "一般ユーザー bob から見た alice", viewerID: uid.N(3), targetID: uid.N(1), leakEmail: "alice@example.com"},
			{name: "admin の root から見た一般ユーザー alice", viewerID: uid.N(4), targetID: uid.N(1), leakEmail: "alice@example.com"},
			{name: "一般ユーザー alice から見た admin の root", viewerID: uid.N(1), targetID: uid.N(4), leakEmail: "root@example.com"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, router, token := newSeededUsersRouter(t)
				rec := get(router, "/users/"+tt.targetID, token(tt.viewerID))
				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
				}
				obj := decodeUserObject(t, rec.Body.Bytes())
				if got := userKeySet(obj); got != publicProfileKeys {
					t.Errorf("keys = [%s], want [%s]", got, publicProfileKeys)
				}
				if userObjectID(t, obj) != tt.targetID {
					t.Errorf("id = %v, want %s", obj["id"], tt.targetID)
				}
				if obj["can_edit"] != false {
					t.Errorf("can_edit = %v, want false（他人は admin でも編集できない）", obj["can_edit"])
				}
				if body := rec.Body.String(); strings.Contains(body, tt.leakEmail) || strings.Contains(body, "@") {
					t.Errorf("body = %s, want no email", body)
				}
			})
		}
	})

	t.Run("認証できない Authorization では公開ビューだけを返す", func(t *testing.T) {
		_, router, token := newSeededUsersRouter(t)
		validAlice := strings.TrimPrefix(token(uid.N(1)), "Bearer ")
		tests := []struct {
			name string
			path string
			auth string
		}{
			{name: "不正な token で本人の id を見る", path: "/users/" + uid.N(1), auth: "Bearer garbage"},
			{name: "Bearer 以外の scheme で本人の id を見る", path: "/users/" + uid.N(1), auth: "Token " + validAlice},
			{name: "scheme のない生の token で本人の id を見る", path: "/users/" + uid.N(1), auth: validAlice},
			{name: "退会済みユーザーの token でも 401 にならず alice は公開ビューになる", path: "/users/" + uid.N(1), auth: token(uid.N(2))},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				rec := get(router, tt.path, tt.auth)
				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
				}
				if got := userKeySet(decodeUserObject(t, rec.Body.Bytes())); got != publicProfileKeys {
					t.Errorf("keys = [%s], want [%s]", got, publicProfileKeys)
				}
			})
		}
	})

	t.Run("AC5 存在しない・退会済み・UUID の正規形でない id は status/body/Content-Type がすべて同一の 404 になる", func(t *testing.T) {
		const wantBody = `{"error":"User not found"}`
		const wantContentType = "application/json; charset=utf-8"
		paths := []string{
			"/users/" + uid.N(999), // 存在しない
			"/users/" + uid.N(2),   // 退会済み (ghost)
			"/users/abc",           // UUID の正規形でない
			"/users/1",             // 旧形式の整数
			"/users/0B0E3A5C-8D54-4C1A-9F33-2A9D6F1C7E10", // 大文字の UUID(正規形ではない)
			"/users/0b0e3a5c8d544c1a9f332a9d6f1c7e10",     // ハイフンのない UUID(正規形ではない)
		}
		_, router, token := newSeededUsersRouter(t)
		for name, auth := range map[string]string{"匿名": "", "ログイン中の alice": token(uid.N(1))} {
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
		repo, router, token := newSeededUsersRouter(t)
		repo.err = fmt.Errorf("db down")
		for _, auth := range []string{"", token(uid.N(1))} {
			rec := get(router, "/users/"+uid.N(1), auth)
			if rec.Code != http.StatusInternalServerError || rec.Body.String() != `{"error":"internal server error"}` {
				t.Errorf("status/body = %d %s, want 500 internal server error", rec.Code, rec.Body)
			}
		}
	})
}

// TestUsersIndexIsNotFound は、ユーザー一覧が廃止されたことを固定する：
// "/users" は route として登録されていないので、匿名でも有効な token 付きでも、
// どの method でも、未知のルート（/nope）と status・body・Content-Type が同一の
// 404 になる（405 や Allow ヘッダーで存在を示さない）。
func TestUsersIndexIsNotFound(t *testing.T) {
	repo, router, token := newUsersRouter(t)
	alice := repo.seed("alice", "alice@example.com", "Password123!")

	unknown := do(router, http.MethodGet, "/nope", "", "")
	if unknown.Code != http.StatusNotFound || unknown.Body.String() != `{"error":"not found"}` {
		t.Fatalf("GET /nope = %d %s, want 404 not found (基準となる未知のルート)", unknown.Code, unknown.Body)
	}

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		auth   string
	}{
		{name: "匿名の GET", method: http.MethodGet, path: "/users"},
		{name: "有効な token 付きの GET", method: http.MethodGet, path: "/users", auth: token(alice.ID)},
		{name: "不正な token 付きの GET", method: http.MethodGet, path: "/users", auth: "Bearer garbage"},
		{name: "匿名の POST", method: http.MethodPost, path: "/users", body: `{"user":{"username":"x"}}`},
		{name: "有効な token 付きの POST", method: http.MethodPost, path: "/users", body: `{"user":{"username":"x"}}`, auth: token(alice.ID)},
		{name: "匿名の PUT", method: http.MethodPut, path: "/users", body: `{"user":{"username":"x"}}`},
		{name: "有効な token 付きの PUT", method: http.MethodPut, path: "/users", body: `{"user":{"username":"x"}}`, auth: token(alice.ID)},
		{name: "匿名の DELETE", method: http.MethodDelete, path: "/users"},
		{name: "有効な token 付きの DELETE", method: http.MethodDelete, path: "/users", auth: token(alice.ID)},
		{name: "匿名の GET (末尾スラッシュ /users/)", method: http.MethodGet, path: "/users/"},
		{name: "有効な token 付きの GET (末尾スラッシュ /users/)", method: http.MethodGet, path: "/users/", auth: token(alice.ID)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(router, tt.method, tt.path, tt.body, tt.auth)
			if rec.Code != unknown.Code {
				t.Errorf("status = %d, want %d (body %s)", rec.Code, unknown.Code, rec.Body)
			}
			if got, want := rec.Body.String(), unknown.Body.String(); got != want {
				t.Errorf("body = %s, want %s", got, want)
			}
			if got, want := rec.Header().Get("Content-Type"), unknown.Header().Get("Content-Type"); got != want {
				t.Errorf("Content-Type = %q, want %q", got, want)
			}
			if allow := rec.Header().Get("Allow"); allow != "" {
				t.Errorf("Allow = %q, want no Allow header (405 ではなく 404)", allow)
			}
		})
	}

	// 一覧を廃止しても GET /users/{id} は影響を受けない。
	if rec := do(router, http.MethodGet, "/users/"+alice.ID, "", ""); rec.Code != http.StatusOK {
		t.Errorf("GET /users/%s = %d, want 200 (詳細は維持される)", alice.ID, rec.Code)
	}
}

// TestUpdateUser は PUT /users/{id} を扱う：AC1（自分自身の更新が GET /users/{id} に
// 反映される）、AC2（存在する別のユーザーには 403、存在しない id には 404）、
// AC3（既に使われている email は 422）、そして Rails parity の validation の
// 境界ケース。
func TestUpdateUser(t *testing.T) {
	setup := func(t *testing.T) (*userStoreFake, http.Handler, string, string) {
		t.Helper()
		repo, router, token := newUsersRouter(t)
		alice := repo.seed("alice", "alice@example.com", "Password123!")
		bob := repo.seed("bob", "bob@example.com", "Password123!")
		return repo, router, token(alice.ID), token(bob.ID)
	}

	t.Run("AC1 自分自身の username 更新は 200 を返し GET /users/{id} に反映される", func(t *testing.T) {
		_, router, aliceAuth, _ := setup(t)
		rec := do(router, http.MethodPut, "/users/"+uid.N(1), `{"user":{"username":"alice2"}}`, aliceAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		want := `{"id":"` + uid.N(1) + `","username":"alice2","email":"alice@example.com","admin":false}`
		if got := rec.Body.String(); got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
		got := do(router, http.MethodGet, "/users/"+uid.N(1), "", "")
		if got.Code != http.StatusOK || got.Body.String() != `{"id":"`+uid.N(1)+`","username":"alice2","can_edit":false}` {
			t.Errorf("GET /users/1 = %d %s, want 200 with the new username reflected", got.Code, got.Body)
		}
	})

	t.Run("AC2 存在する別のユーザーの id は 403 を返す", func(t *testing.T) {
		_, router, _, bobAuth := setup(t)
		rec := do(router, http.MethodPut, "/users/"+uid.N(1), `{"user":{"username":"hacked"}}`, bobAuth)
		if rec.Code != http.StatusForbidden || rec.Body.String() != `{"error":"Forbidden"}` {
			t.Errorf("status/body = %d %s, want 403 Forbidden", rec.Code, rec.Body)
		}
	})

	t.Run("AC2 存在しない id と非数値の id は所有権に関わらず 404 を返す", func(t *testing.T) {
		_, router, aliceAuth, _ := setup(t)
		for _, path := range []string{"/users/" + uid.N(999), "/users/abc"} {
			rec := do(router, http.MethodPut, path, `{"user":{"username":"x"}}`, aliceAuth)
			if rec.Code != http.StatusNotFound || rec.Body.String() != `{"error":"User not found"}` {
				t.Errorf("PUT %s = %d %s, want 404 User not found", path, rec.Code, rec.Body)
			}
		}
	})

	t.Run("AC3 email を別のユーザーの email に変更すると 422 を返す", func(t *testing.T) {
		_, router, aliceAuth, _ := setup(t)
		rec := do(router, http.MethodPut, "/users/"+uid.N(1), `{"user":{"email":"bob@example.com"}}`, aliceAuth)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusUnprocessableEntity, rec.Body)
		}
		if got, want := rec.Body.String(), `{"errors":["Email has already been taken"]}`; got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})

	t.Run("空の user オブジェクトと user キーなしは変更なしで 200 を返す", func(t *testing.T) {
		_, router, aliceAuth, _ := setup(t)
		want := `{"id":"` + uid.N(1) + `","username":"alice","email":"alice@example.com","admin":false}`
		for _, body := range []string{`{"user":{}}`, `{}`} {
			rec := do(router, http.MethodPut, "/users/"+uid.N(1), body, aliceAuth)
			if rec.Code != http.StatusOK || rec.Body.String() != want {
				t.Errorf("body %s: status/body = %d %s, want 200 %s", body, rec.Code, rec.Body, want)
			}
		}
	})

	t.Run("空文字列の password は digest に影響しない", func(t *testing.T) {
		repo, router, aliceAuth, _ := setup(t)
		rec := do(router, http.MethodPut, "/users/"+uid.N(1), `{"user":{"password":""}}`, aliceAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		if got := repo.users[uid.N(1)].digest; got != "digest:Password123!" {
			t.Errorf("stored digest = %q, want the untouched %q", got, "digest:Password123!")
		}
	})

	t.Run("強度ルールを満たす password は 200 を返し、ハッシュ化した digest が保存される", func(t *testing.T) {
		repo, router, aliceAuth, _ := setup(t)
		rec := do(router, http.MethodPut, "/users/"+uid.N(1), `{"user":{"password":"NewPassw0rd!","password_confirmation":"NewPassw0rd!"}}`, aliceAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		if got := repo.users[uid.N(1)].digest; got != "digest:NewPassw0rd!" {
			t.Errorf("stored digest = %q, want %q", got, "digest:NewPassw0rd!")
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
				name:     "形式が不正な email は検証エラーになる",
				body:     `{"user":{"email":"abc"}}`,
				wantBody: `{"errors":["Email is invalid"]}`,
			},
			{
				name:     "確認用パスワードの不一致は検証エラーになる",
				body:     `{"user":{"password":"NewPassw0rd!","password_confirmation":"other"}}`,
				wantBody: `{"errors":["Password confirmation doesn't match Password"]}`,
			},
			{
				// 文字種は満たす 73 バイトにして、too long だけが出ることを見る。
				name:     "72 bytes を超える password は too long だけの検証エラーになる",
				body:     fmt.Sprintf(`{"user":{"password":%q}}`, "Aa1!"+strings.Repeat("x", 69)),
				wantBody: `{"errors":["Password is too long (maximum is 72 characters)"]}`,
			},
			{
				name:     "弱いパスワード（短く記号なし）は 2 件のメッセージ付きで検証エラーになる",
				body:     `{"user":{"password":"abc123"}}`,
				wantBody: `{"errors":["Password is too short (minimum is 8 characters)","Password must include letters, numbers and symbols"]}`,
			},
			{
				name:     "記号のない 8 バイトのパスワードは文字種のメッセージだけの検証エラーになる",
				body:     `{"user":{"password":"abcd1234"}}`,
				wantBody: `{"errors":["Password must include letters, numbers and symbols"]}`,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, router, aliceAuth, _ := setup(t)
				rec := do(router, http.MethodPut, "/users/"+uid.N(1), tt.body, aliceAuth)
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
			rec := do(router, http.MethodPut, "/users/"+uid.N(1), body, aliceAuth)
			if rec.Code != http.StatusBadRequest || rec.Body.String() != `{"error":"invalid JSON body"}` {
				t.Errorf("body %q: status/body = %d %s, want 400 invalid JSON body", body, rec.Code, rec.Body)
			}
		}
	})
}

// TestDeleteUser は DELETE /users/{id} を扱う：AC2 の 403/404 の使い分けと、
// AC4 の核（body なしの 204、無効になったトークン、GET /users/{id} の 404）。
func TestDeleteUser(t *testing.T) {
	setup := func(t *testing.T) (http.Handler, string, string) {
		t.Helper()
		repo, router, token := newUsersRouter(t)
		alice := repo.seed("alice", "alice@example.com", "Password123!")
		bob := repo.seed("bob", "bob@example.com", "Password123!")
		return router, token(alice.ID), token(bob.ID)
	}

	t.Run("AC2 存在する別のユーザーの id は 403 を返す", func(t *testing.T) {
		router, _, bobAuth := setup(t)
		rec := do(router, http.MethodDelete, "/users/"+uid.N(1), "", bobAuth)
		if rec.Code != http.StatusForbidden || rec.Body.String() != `{"error":"Forbidden"}` {
			t.Errorf("status/body = %d %s, want 403 Forbidden", rec.Code, rec.Body)
		}
	})

	t.Run("AC2 存在しない id と非数値の id は 404 を返す", func(t *testing.T) {
		router, aliceAuth, _ := setup(t)
		for _, path := range []string{"/users/" + uid.N(999), "/users/abc"} {
			rec := do(router, http.MethodDelete, path, "", aliceAuth)
			if rec.Code != http.StatusNotFound || rec.Body.String() != `{"error":"User not found"}` {
				t.Errorf("DELETE %s = %d %s, want 404 User not found", path, rec.Code, rec.Body)
			}
		}
	})

	t.Run("自分自身を削除すると 204 を返し、トークンが無効になり、GET /users/{id} が 404 になる", func(t *testing.T) {
		router, aliceAuth, _ := setup(t)
		rec := do(router, http.MethodDelete, "/users/"+uid.N(1), "", aliceAuth)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusNoContent, rec.Body)
		}
		if rec.Body.Len() != 0 {
			t.Errorf("body = %q, want empty", rec.Body.String())
		}
		if again := do(router, http.MethodPost, "/logout", "", aliceAuth); again.Code != http.StatusUnauthorized {
			t.Errorf("protected request after discard = %d, want 401", again.Code)
		}
		gone := do(router, http.MethodGet, "/users/"+uid.N(1), "", "")
		if gone.Code != http.StatusNotFound || gone.Body.String() != `{"error":"User not found"}` {
			t.Errorf("GET /users/1 = %d %s, want 404 User not found", gone.Code, gone.Body)
		}
	})
}

// TestUsersRequireAuth はユーザー系のルートの 401 の境界を固定する：書き込み
// （PUT/DELETE）は repository へのアクセスより前に匿名の request を拒否し、
// 一方で GET /users/{id} は開かれたままである。/users/{id} は 1 つの route に
// 3 つの method を宣言しており、GET を OptionalAuth にしたことで PUT/DELETE の
// RequireAuth が緩んでいないことを確かめる。
func TestUsersRequireAuth(t *testing.T) {
	repo, router, _ := newUsersRouter(t)
	repo.seed("alice", "alice@example.com", "Password123!")
	const unauthorized = `{"error":"Unauthorized"}`

	tests := []struct {
		method string
		path   string
		body   string
		auth   string
	}{
		{method: http.MethodPut, path: "/users/" + uid.N(1), body: `{"user":{"username":"x"}}`},
		{method: http.MethodDelete, path: "/users/" + uid.N(1)},
		{method: http.MethodPut, path: "/users/" + uid.N(1), body: `{"user":{"username":"x"}}`, auth: "Bearer garbage"},
		{method: http.MethodDelete, path: "/users/" + uid.N(1), auth: "Bearer garbage"},
	}
	for _, tt := range tests {
		rec := do(router, tt.method, tt.path, tt.body, tt.auth)
		if rec.Code != http.StatusUnauthorized || rec.Body.String() != unauthorized {
			t.Errorf("%s %s (auth %q) = %d %s, want 401 %s", tt.method, tt.path, tt.auth, rec.Code, rec.Body, unauthorized)
		}
	}
	// 拒否された匿名の PUT/DELETE は何も変更していない。
	if rec := do(router, http.MethodGet, "/users/"+uid.N(1), "", ""); rec.Code != http.StatusOK ||
		decodeUserObject(t, rec.Body.Bytes())["username"] != "alice" {
		t.Errorf("GET /users/1 after rejected writes = %d %s, want 200 alice untouched", rec.Code, rec.Body)
	}

	if rec := do(router, http.MethodGet, "/users/"+uid.N(1), "", ""); rec.Code != http.StatusOK {
		t.Errorf("GET /users/1 anonymous = %d, want 200 (optional auth)", rec.Code)
	}
}

// newUsersIntegrationKit は、実行ごとに新しく作られる database 上で、本物の
// repository、本物の bcrypt、本物の JWT を使って router 全体を配線する
// （TEST_DATABASE_URL がなければ dbtest の内部で skip される）。
func newUsersIntegrationKit(t *testing.T) (*pgx.Conn, http.Handler) {
	t.Helper()
	conn, _ := dbtest.New(t)
	userQuery := query.NewUserQuery(conn)
	userRepo := repository.NewUserRepository(conn)
	hasher := infra.BcryptPasswordHasher{}
	codec := infra.NewJWTCodec(testJWTSecret, time.Hour)
	auth := usecase.NewAuth(userQuery, domain.NewUsers(userRepo), hasher, codec, codec)
	unitOfWork := uow.New(conn)
	recalc := usecase.NewBurgerStatsRecalculator(infra.SystemClock{})
	router := handler.NewRouter(conn, auth,
		usecase.NewShops(query.NewShopQuery(conn), domain.NewShops(repository.NewShopRepository(conn))),
		usecase.NewReviews(query.NewReviewQuery(conn), unitOfWork, recalc, storage.NewDisk(t.TempDir(), "/photos")),
		usecase.NewUsers(userQuery, domain.NewUsers(userRepo), unitOfWork, recalc, hasher), nil)
	return conn, router
}

// signupUser は POST /signup を通じてユーザーを登録し、その id と Bearer
// ヘッダーを返す。
func signupUser(t *testing.T, router http.Handler, username, email, password string) (string, string) {
	t.Helper()
	body := fmt.Sprintf(`{"username":%q,"email":%q,"password":%q}`, username, email, password)
	rec := do(router, http.MethodPost, "/signup", body, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("signup %s: status = %d (body %s)", username, rec.Code, rec.Body)
	}
	user := decodeAuthUser(t, rec.Body.Bytes())
	return user.ID, "Bearer " + user.Token
}

// loginAs は POST /login を email と password で実行する。
func loginAs(router http.Handler, email, password string) *httptest.ResponseRecorder {
	return do(router, http.MethodPost, "/login", fmt.Sprintf(`{"email":%q,"password":%q}`, email, password), "")
}

// TestUsersPasswordChangeIntegration は AC6 をエンドツーエンドで扱う：
// 弱いパスワードへの PUT /users/{id} は 422 で拒否され、旧パスワードのまま
// ログインできる。規則を満たすパスワードへの更新後は、古いパスワードでは
// もうログインできず（401）、新しいパスワードではログインできる（200）。
// 本物の bcrypt、本物の JWT、本物の PostgreSQL を使う。
func TestUsersPasswordChangeIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed integration test in short mode")
	}
	const (
		oldPassword = "OldPassw0rd!"
		newPassword = "NewPassw0rd!"
	)
	_, router := newUsersIntegrationKit(t)
	id, bearer := signupUser(t, router, "alice", "alice@example.com", oldPassword)
	path := "/users/" + id

	weak := do(router, http.MethodPut, path, `{"user":{"password":"abc123","password_confirmation":"abc123"}}`, bearer)
	wantWeak := `{"errors":["Password is too short (minimum is 8 characters)","Password must include letters, numbers and symbols"]}`
	if weak.Code != http.StatusUnprocessableEntity || weak.Body.String() != wantWeak {
		t.Fatalf("weak update = %d %s, want 422 %s", weak.Code, weak.Body, wantWeak)
	}
	if kept := loginAs(router, "alice@example.com", oldPassword); kept.Code != http.StatusOK {
		t.Errorf("old-password login after rejected update = %d, want 200 (body %s)", kept.Code, kept.Body)
	}

	body := fmt.Sprintf(`{"user":{"password":%q,"password_confirmation":%q}}`, newPassword, newPassword)
	rec := do(router, http.MethodPut, path, body, bearer)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
	}
	want := fmt.Sprintf(`{"id":%q,"username":"alice","email":"alice@example.com","admin":false}`, id)
	if got := rec.Body.String(); got != want {
		t.Errorf("update body = %s, want %s", got, want)
	}

	old := loginAs(router, "alice@example.com", oldPassword)
	if old.Code != http.StatusUnauthorized || old.Body.String() != `{"error":"Invalid email or password"}` {
		t.Errorf("old-password login = %d %s, want the Rails-parity 401", old.Code, old.Body)
	}
	fresh := loginAs(router, "alice@example.com", newPassword)
	if fresh.Code != http.StatusOK {
		t.Errorf("new-password login = %d, want 200 (body %s)", fresh.Code, fresh.Body)
	}
}

// TestLoginValidationIntegration は、認証情報の規則の判定が本物の bcrypt・PostgreSQL・
// router を通しても効くことを固定する（Story #61）。規則を満たす認証情報は 200、
// 規則を満たさない入力は 422、規則を満たしたうえで誤っている入力は 401 になる。
func TestLoginValidationIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed integration test in short mode")
	}
	const password = "Password123!"
	conn, router := newUsersIntegrationKit(t)
	digest, err := infra.BcryptPasswordHasher{}.Hash(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := conn.Exec(context.Background(),
		`INSERT INTO users (email, username, password_digest) VALUES ($1, $2, $3)`,
		"alice@example.com", "alice", digest); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	rec := loginAs(router, "alice@example.com", password)
	if rec.Code != http.StatusOK {
		t.Fatalf("login = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	if user := decodeAuthUser(t, rec.Body.Bytes()); user.Username != "alice" || user.Token == "" {
		t.Errorf("login body = %+v, want alice with a token", user)
	}
	weak := loginAs(router, "alice@example.com", "weakpassword")
	if weak.Code != http.StatusUnprocessableEntity || weak.Body.String() != `{"errors":["Password must include letters, numbers and symbols"]}` {
		t.Errorf("weak-password login = %d %s, want 422 with the password message", weak.Code, weak.Body)
	}
	if wrong := loginAs(router, "alice@example.com", "Wrongpass1!"); wrong.Code != http.StatusUnauthorized {
		t.Errorf("wrong-password login = %d, want 401", wrong.Code)
	}
}

// TestUsersProfileViewsIntegration は、本物の PostgreSQL・repository・JWT を通して、
// SQL から domain、JSON までの一連で、GET /users/{id} の公開ビューと本人ビューの
// キー集合が保たれることを確かめる（AC2〜AC5）。fixture は signup した 3 人を共有し、
// 最後に 1 人を退会させる。
func TestUsersProfileViewsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed integration test in short mode")
	}
	ctx := context.Background()
	conn, router := newUsersIntegrationKit(t)

	aliceID, aliceAuth := signupUser(t, router, "alice", "alice@example.com", "Password123!")
	bobID, bobAuth := signupUser(t, router, "bob", "bob@example.com", "Password123!")
	rootID, rootAuth := signupUser(t, router, "root", "root@example.com", "Password123!")
	if _, err := conn.Exec(ctx, `UPDATE users SET admin = true WHERE id = $1`, rootID); err != nil {
		t.Fatalf("promote root: %v", err)
	}

	// assertView は GET /users/{id} を auth（"" = 匿名）で実行し、200 で、wantEmail が
	// 空なら公開ビュー {id, username}（body に email の "@" も現れない）、空でなければ
	// 本人ビュー {id, username, email, admin} であることを確かめる。
	assertView := func(t *testing.T, id string, auth, wantEmail string, wantAdmin bool) {
		t.Helper()
		rec := do(router, http.MethodGet, "/users/"+id, "", auth)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /users/%s = %d, want 200 (body %s)", id, rec.Code, rec.Body)
		}
		obj := decodeUserObject(t, rec.Body.Bytes())
		if wantEmail == "" {
			if got := userKeySet(obj); got != publicProfileKeys {
				t.Errorf("user %s: keys = [%s], want [%s]", id, got, publicProfileKeys)
			}
			if strings.Contains(rec.Body.String(), "@") {
				t.Errorf("user %s: body leaks an email address: %s", id, rec.Body)
			}
			return
		}
		if got := userKeySet(obj); got != selfUserKeys {
			t.Errorf("self %s: keys = [%s], want [%s]", id, got, selfUserKeys)
		}
		if obj["email"] != wantEmail || obj["admin"] != wantAdmin {
			t.Errorf("self %s = %v, want email %q admin %v", id, obj, wantEmail, wantAdmin)
		}
	}

	t.Run("匿名の詳細は {id, username} だけを返す", func(t *testing.T) {
		for _, id := range []string{aliceID, bobID, rootID} {
			assertView(t, id, "", "", false)
		}
	})

	t.Run("alice の token では alice 自身の詳細だけが本人ビューになる", func(t *testing.T) {
		assertView(t, aliceID, aliceAuth, "alice@example.com", false)
		assertView(t, bobID, aliceAuth, "", false)
	})

	t.Run("admin の root でも他人の詳細は公開ビューのままで自分の詳細だけ本人ビューになる", func(t *testing.T) {
		assertView(t, rootID, rootAuth, "root@example.com", true)
		assertView(t, aliceID, rootAuth, "", false)
	})

	t.Run("退会したユーザーの詳細は存在しない id と同一の 404 になる", func(t *testing.T) {
		if rec := do(router, http.MethodDelete, "/users/"+bobID, "", bobAuth); rec.Code != http.StatusNoContent {
			t.Fatalf("delete bob = %d, want 204 (body %s)", rec.Code, rec.Body)
		}
		gone := do(router, http.MethodGet, "/users/"+bobID, "", "")
		never := do(router, http.MethodGet, "/users/"+uid.N(999999), "", "")
		if gone.Code != http.StatusNotFound || gone.Body.String() != `{"error":"User not found"}` {
			t.Errorf("discarded detail = %d %s, want 404 User not found", gone.Code, gone.Body)
		}
		if gone.Code != never.Code || gone.Body.String() != never.Body.String() ||
			gone.Header().Get("Content-Type") != never.Header().Get("Content-Type") {
			t.Errorf("discarded (%d %s) differs from never-existed (%d %s)", gone.Code, gone.Body, never.Code, never.Body)
		}

		assertView(t, aliceID, "", "", false)
		assertView(t, rootID, "", "", false)
	})
}

// TestUserIDsAreUUIDsIntegration は S27 の AC1〜AC3・AC6 を、本物の PostgreSQL・bcrypt・JWT・
// router で固定する：signup と login が返す id は UUID の正規形で、同じユーザーは同じ id になり、
// トークンの user_id もその id である。GET /users/{uuid} は 200、旧形式の整数や UUID の
// 正規形でない id は存在しない id と同じ 404 になる。
func TestUserIDsAreUUIDsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed integration test in short mode")
	}
	_, router := newUsersIntegrationKit(t)
	id, bearer := signupUser(t, router, "alice", "alice@example.com", "Password123!")
	if !domain.IsUUID(id) {
		t.Fatalf("signup id = %q, want a UUID in the canonical form", id)
	}

	login := loginAs(router, "alice@example.com", "Password123!")
	if login.Code != http.StatusOK {
		t.Fatalf("login = %d, want 200 (body %s)", login.Code, login.Body)
	}
	if got := decodeAuthUser(t, login.Body.Bytes()).ID; got != id {
		t.Errorf("login id = %q, want the signup id %q", got, id)
	}

	codec := infra.NewJWTCodec(testJWTSecret, time.Hour)
	if got, err := codec.Verify(strings.TrimPrefix(bearer, "Bearer ")); err != nil || got != id {
		t.Errorf("token user_id = %q (err %v), want %q", got, err, id)
	}

	if rec := do(router, http.MethodGet, "/users/"+id, "", ""); rec.Code != http.StatusOK {
		t.Errorf("GET /users/%s = %d, want 200 (body %s)", id, rec.Code, rec.Body)
	}
	const wantNotFound = `{"error":"User not found"}`
	for _, path := range []string{"/users/1", "/users/abc", "/users/" + strings.ToUpper(id), "/users/" + uid.N(999999)} {
		rec := do(router, http.MethodGet, path, "", "")
		if rec.Code != http.StatusNotFound || rec.Body.String() != wantNotFound {
			t.Errorf("GET %s = %d %s, want 404 %s", path, rec.Code, rec.Body, wantNotFound)
		}
	}
	if rec := do(router, http.MethodPut, "/users/1", `{"user":{"username":"x"}}`, bearer); rec.Code != http.StatusNotFound {
		t.Errorf("PUT /users/1 = %d, want 404 (旧形式の整数の id)", rec.Code)
	}
}

// usersFeedItem は、これらの統合テストの assertion が関心を持つ review の
// JSON の一部分である。
type usersFeedItem struct {
	ID   int64 `json:"id"`
	User struct {
		ID string `json:"id"`
	} `json:"user"`
	Burger struct {
		ID          int64 `json:"id"`
		ReviewCount int64 `json:"review_count"`
	} `json:"burger"`
}

// TestUsersDiscardPropagationIntegration は、本物の database に対して
// HTTP 越しに行う AC4+AC5 退会の波及のシナリオである：ユーザー A は共有の
// burger（B も review している）と単独の burger を review し、account を
// 削除する。すると下流のすべて（トークン、user の detail、feed、detail、shop の
// review、burger の統計）が A を忘れ、B はそのまま残る。
func TestUsersDiscardPropagationIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed integration test in short mode")
	}
	ctx := context.Background()
	conn, router := newUsersIntegrationKit(t)

	aliceID, aliceAuth := signupUser(t, router, "alice", "alice@example.com", "Password123!")
	bobID, bobAuth := signupUser(t, router, "bob", "bob@example.com", "Password123!")

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
	rec := do(router, http.MethodDelete, "/users/"+aliceID, "", aliceAuth)
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

	// GET /users/{id} は A を忘れ（存在しない id と同じ 404）、B を残す。
	rec = do(router, http.MethodGet, "/users/"+aliceID, "", "")
	if rec.Code != http.StatusNotFound || rec.Body.String() != `{"error":"User not found"}` {
		t.Errorf("GET /users/%s = %d %s, want 404 User not found", aliceID, rec.Code, rec.Body)
	}
	rec = do(router, http.MethodGet, "/users/"+bobID, "", "")
	if want := fmt.Sprintf(`{"id":%q,"username":"bob","can_edit":false}`, bobID); rec.Code != http.StatusOK || rec.Body.String() != want {
		t.Errorf("GET /users/%s = %d %s, want 200 %s", bobID, rec.Code, rec.Body, want)
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
