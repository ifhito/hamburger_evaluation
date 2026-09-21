package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// TestUserBio は、自己紹介文(API の JSON のキーと DB の列名は bio。biography の略)の
// 公開範囲と更新できる人を固定する。自己紹介文は誰にでも見える公開情報で、更新できるのは
// 本人だけである(管理者でも、他人のものは更新できない)。メールアドレスと管理者かどうかは、
// 従来どおり本人にしか見えない。
func TestUserBio(t *testing.T) {
	// HTML のタグや改行、絵文字を含む値。加工されずに保存・返却されることを確かめるための入力
	const aliceBio = "はじめまして。\n2 行目です。 <script>alert(1)</script> 🍔"

	put := func(router http.Handler, id, body, auth string) *httpResult {
		rec := do(router, http.MethodPut, "/users/"+id, body, auth)
		return &httpResult{code: rec.Code, body: rec.Body.String()}
	}
	get := func(router http.Handler, id, auth string) *httpResult {
		rec := do(router, http.MethodGet, "/users/"+id, "", auth)
		return &httpResult{code: rec.Code, body: rec.Body.String()}
	}

	t.Run("本人が自己紹介文を更新すると、更新の応答にも、あとから取得したプロフィールにも反映される。HTML のタグは加工されずそのまま返る", func(t *testing.T) {
		_, router, token := newSeededUsersRouter(t)
		res := put(router, uid.N(1), `{"user":{"bio":`+jsonString(t, aliceBio)+`}}`, token(uid.N(1)))
		if res.code != http.StatusOK {
			t.Fatalf("PUT status = %d, want 200 (body %s)", res.code, res.body)
		}
		if got := decodeUserObject(t, []byte(res.body))["bio"]; got != aliceBio {
			t.Errorf("更新の応答の自己紹介文 = %q, want %q", got, aliceBio)
		}
		if got := decodeUserObject(t, []byte(get(router, uid.N(1), token(uid.N(1))).body))["bio"]; got != aliceBio {
			t.Errorf("あとから取得したプロフィールの自己紹介文 = %q, want %q", got, aliceBio)
		}
	})

	t.Run("自己紹介文を送らずに他の項目だけ更新しても自己紹介文は変わらず、空文字を送ると自己紹介文が消える", func(t *testing.T) {
		_, router, token := newSeededUsersRouter(t)
		auth := token(uid.N(1))
		put(router, uid.N(1), `{"user":{"bio":"kept"}}`, auth)
		if res := put(router, uid.N(1), `{"user":{"username":"alice2"}}`, auth); res.code != http.StatusOK {
			t.Fatalf("ユーザー名だけの PUT: status = %d (body %s)", res.code, res.body)
		}
		if got := decodeUserObject(t, []byte(get(router, uid.N(1), "").body))["bio"]; got != "kept" {
			t.Errorf("ユーザー名だけを更新したあとの自己紹介文 = %q, want kept(変わってはいけない)", got)
		}
		if res := put(router, uid.N(1), `{"user":{"bio":""}}`, auth); res.code != http.StatusOK {
			t.Fatalf("空文字の自己紹介文の PUT: status = %d (body %s)", res.code, res.body)
		}
		if got := decodeUserObject(t, []byte(get(router, uid.N(1), "").body))["bio"]; got != "" {
			t.Errorf("空文字を送ったあとの自己紹介文 = %q, want 空(消える)", got)
		}
	})

	t.Run("上限ちょうどの自己紹介文は保存でき、上限を 1 文字超えると 422 で拒否されて保存済みの内容も変わらない(日本語・絵文字も 1 文字と数える)", func(t *testing.T) {
		repo, router, token := newSeededUsersRouter(t)
		auth := token(uid.N(1))
		for _, unit := range []string{"a", "あ", "🍔"} {
			exact := strings.Repeat(unit, domain.MaxBioChars)
			if res := put(router, uid.N(1), `{"user":{"bio":`+jsonString(t, exact)+`}}`, auth); res.code != http.StatusOK {
				t.Errorf("%q を %d 文字: status = %d, want 200 (body %.200s)", unit, domain.MaxBioChars, res.code, res.body)
			}
		}
		before := repo.users[uid.N(1)].user
		over := tooLong("Bio", domain.MaxBioChars)
		for _, unit := range []string{"a", "あ", "🍔"} {
			res := put(router, uid.N(1), `{"user":{"bio":`+jsonString(t, strings.Repeat(unit, domain.MaxBioChars+1))+`}}`, auth)
			if res.code != http.StatusUnprocessableEntity || res.body != over {
				t.Errorf("%q を %d 文字: status/body = %d %s, want 422 %s", unit, domain.MaxBioChars+1, res.code, res.body, over)
			}
		}
		if repo.users[uid.N(1)].user != before {
			t.Errorf("422 で拒否したのにユーザーが変わった: %+v → %+v", before, repo.users[uid.N(1)].user)
		}
	})

	t.Run("他人や未ログインの閲覧者にも自己紹介文は見えるが、メールアドレスと管理者かどうかは見えない", func(t *testing.T) {
		_, router, token := newSeededUsersRouter(t)
		put(router, uid.N(1), `{"user":{"bio":"hello"}}`, token(uid.N(1)))
		for name, auth := range map[string]string{"未ログイン": "", "他人(bob)": token(uid.N(3)), "管理者(root)": token(uid.N(4))} {
			res := get(router, uid.N(1), auth)
			if res.code != http.StatusOK {
				t.Fatalf("%s: status = %d", name, res.code)
			}
			obj := decodeUserObject(t, []byte(res.body))
			if got := userKeySet(obj); got != publicProfileKeys {
				t.Errorf("%s: 返ったキー = [%s], want [%s]", name, got, publicProfileKeys)
			}
			if obj["bio"] != "hello" {
				t.Errorf("%s: 自己紹介文 = %v, want hello", name, obj["bio"])
			}
		}
	})

	t.Run("本人以外(管理者を含む)が自己紹介文を更新しようとすると 403 で拒否され、内容は変わらない", func(t *testing.T) {
		repo, router, token := newSeededUsersRouter(t)
		before := repo.users[uid.N(1)].user
		for name, auth := range map[string]string{"bob": token(uid.N(3)), "管理者(root)": token(uid.N(4))} {
			res := put(router, uid.N(1), `{"user":{"bio":"hacked"}}`, auth)
			if res.code != http.StatusForbidden {
				t.Errorf("%s: status = %d, want 403 (body %s)", name, res.code, res.body)
			}
		}
		if repo.users[uid.N(1)].user != before {
			t.Errorf("403 で拒否したのにユーザーが変わった: %+v → %+v", before, repo.users[uid.N(1)].user)
		}
	})
}

// TestUserBioIntegration は、本物の PostgreSQL・repository・router を通して、次のことを固定する。
//   - 自己紹介文が保存され、取得できる(新規登録の要求に含まれていても無視され、空で始まる)
//   - 文字数の上限が、Go のコードポイント数と PostgreSQL の char_length で同じに数えられる
//     (絵文字で上限ちょうどの文字数を保存できる)
//   - 拒否された更新(上限超過・他人の更新)では、データベースの内容が変わらない
func TestUserBioIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed integration test in short mode")
	}
	ctx := context.Background()
	conn, router := newUsersIntegrationKit(t)

	storedBio := func(userID string) (string, int) {
		t.Helper()
		var bio string
		var chars int
		if err := conn.QueryRow(ctx, "SELECT bio, char_length(bio) FROM users WHERE id = $1", userID).Scan(&bio, &chars); err != nil {
			t.Fatalf("自己紹介文の読み取り: %v", err)
		}
		return bio, chars
	}

	t.Run("新規登録の要求に自己紹介文が含まれていても無視され、確認後に作られたユーザーの自己紹介文は空になる", func(t *testing.T) {
		kit, ok := router.(*mailedRouter)
		if !ok {
			t.Fatalf("router は newUsersIntegrationKit のものでなければならない: %T", router)
		}
		body := `{"username":"carol","email":"carol@example.com","password":"Password123!","bio":"ignored"}`
		if rec := do(router, http.MethodPost, "/signup", body, ""); rec.Code != http.StatusAccepted {
			t.Fatalf("新規登録: status = %d, want 202 (body %s)", rec.Code, rec.Body)
		}
		rec := do(router, http.MethodPost, "/signup/confirm", confirmBody(kit.mailer.lastToken(t)), "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("確認: status = %d, want 201 (body %s)", rec.Code, rec.Body)
		}
		if bio, _ := storedBio(decodeAuthUser(t, rec.Body.Bytes()).ID); bio != "" {
			t.Errorf("新規登録で作られたユーザーの自己紹介文 = %q, want 空(新規登録では設定できない)", bio)
		}
	})

	id, auth := signupUser(t, router, "biouser", "biouser@example.com", "Password123!")
	otherID, otherAuth := signupUser(t, router, "other", "other@example.com", "Password123!")

	// 更新前は空(列の既定値)。以降の「変わっていない」の比較の起点になる
	if bio, _ := storedBio(id); bio != "" {
		t.Fatalf("新規登録直後の自己紹介文 = %q, want 空(列の既定値)", bio)
	}

	const text = "1 行目\n<b>2 行目</b> 🍔"
	rec := do(router, http.MethodPut, "/users/"+id, `{"user":{"bio":`+jsonString(t, text)+`}}`, auth)
	if rec.Code != http.StatusOK {
		t.Fatalf("自己紹介文の更新: status = %d (body %s)", rec.Code, rec.Body)
	}
	if bio, _ := storedBio(id); bio != text {
		t.Errorf("保存された自己紹介文 = %q, want %q", bio, text)
	}
	anon := do(router, http.MethodGet, "/users/"+id, "", "")
	if got := decodeUserObject(t, anon.Body.Bytes()); got["bio"] != text || got["email"] != nil {
		t.Errorf("未ログインで取得したプロフィール = %s, want 自己紹介文が見えて、メールアドレスは無い", anon.Body)
	}

	exact := strings.Repeat("🍔", domain.MaxBioChars)
	if rec := do(router, http.MethodPut, "/users/"+id, `{"user":{"bio":`+jsonString(t, exact)+`}}`, auth); rec.Code != http.StatusOK {
		t.Fatalf("絵文字 %d 個の更新: status = %d (body %.200s)", domain.MaxBioChars, rec.Code, rec.Body)
	}
	if _, chars := storedBio(id); chars != domain.MaxBioChars {
		t.Errorf("保存された自己紹介文の char_length = %d, want %d", chars, domain.MaxBioChars)
	}

	over := tooLong("Bio", domain.MaxBioChars)
	rec = do(router, http.MethodPut, "/users/"+id, `{"user":{"bio":`+jsonString(t, exact+"🍔")+`}}`, auth)
	if rec.Code != http.StatusUnprocessableEntity || rec.Body.String() != over {
		t.Errorf("上限超過の更新: status/body = %d %s, want 422 %s", rec.Code, rec.Body, over)
	}
	if bio, _ := storedBio(id); bio != exact {
		t.Errorf("422 で拒否したのに自己紹介文が変わった(%d 文字)", len([]rune(bio)))
	}

	if rec := do(router, http.MethodPut, "/users/"+id, fmt.Sprintf(`{"user":{"bio":%s}}`, jsonString(t, "hacked")), otherAuth); rec.Code != http.StatusForbidden {
		t.Errorf("他人による更新: status = %d, want 403", rec.Code)
	}
	if bio, _ := storedBio(id); bio != exact {
		t.Errorf("403 で拒否したのに自己紹介文が変わった")
	}
	if bio, _ := storedBio(otherID); bio != "" {
		t.Errorf("更新を試みた側の自己紹介文が変わった: %q", bio)
	}

	if rec := do(router, http.MethodPut, "/users/"+id, `{"user":{"bio":""}}`, auth); rec.Code != http.StatusOK {
		t.Fatalf("空文字で消す更新: status = %d", rec.Code)
	}
	if bio, _ := storedBio(id); bio != "" {
		t.Errorf("空文字を送ったあとの自己紹介文 = %q, want 空(消える)", bio)
	}
}

// httpResult は、テストの読みやすさのための、ステータスコードと本文だけの束である。
type httpResult struct {
	code int
	body string
}
