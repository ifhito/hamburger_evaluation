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

// TestUserBio は S28 の AC1〜AC4 を固定する：自己紹介文(bio)は本人だけが更新でき
// (PUT /users/{id}。送らなければ変更なし、空文字で消せる)、GET /users/{id} では
// 公開ビュー・本人ビューの両方に含まれる。email と admin は、これまでどおり本人だけ。
func TestUserBio(t *testing.T) {
	const aliceBio = "はじめまして。\n2 行目です。 <script>alert(1)</script> 🍔"

	put := func(router http.Handler, id, body, auth string) *httpResult {
		rec := do(router, http.MethodPut, "/users/"+id, body, auth)
		return &httpResult{code: rec.Code, body: rec.Body.String()}
	}
	get := func(router http.Handler, id, auth string) *httpResult {
		rec := do(router, http.MethodGet, "/users/"+id, "", auth)
		return &httpResult{code: rec.Code, body: rec.Body.String()}
	}

	t.Run("AC1 本人が bio を更新すると、PUT の応答と GET に反映される。HTML はそのまま文字として返る", func(t *testing.T) {
		_, router, token := newSeededUsersRouter(t)
		res := put(router, uid.N(1), `{"user":{"bio":`+jsonString(t, aliceBio)+`}}`, token(uid.N(1)))
		if res.code != http.StatusOK {
			t.Fatalf("PUT status = %d, want 200 (body %s)", res.code, res.body)
		}
		if got := decodeUserObject(t, []byte(res.body))["bio"]; got != aliceBio {
			t.Errorf("PUT の bio = %q, want %q", got, aliceBio)
		}
		if got := decodeUserObject(t, []byte(get(router, uid.N(1), token(uid.N(1))).body))["bio"]; got != aliceBio {
			t.Errorf("GET(本人)の bio = %q, want %q", got, aliceBio)
		}
	})

	t.Run("AC1 bio を送らない PUT は bio を変えず、空文字を送ると消せる", func(t *testing.T) {
		_, router, token := newSeededUsersRouter(t)
		auth := token(uid.N(1))
		put(router, uid.N(1), `{"user":{"bio":"kept"}}`, auth)
		if res := put(router, uid.N(1), `{"user":{"username":"alice2"}}`, auth); res.code != http.StatusOK {
			t.Fatalf("username だけの PUT: status = %d (body %s)", res.code, res.body)
		}
		if got := decodeUserObject(t, []byte(get(router, uid.N(1), "").body))["bio"]; got != "kept" {
			t.Errorf("bio を送らない PUT のあとの bio = %q, want kept", got)
		}
		if res := put(router, uid.N(1), `{"user":{"bio":""}}`, auth); res.code != http.StatusOK {
			t.Fatalf("空文字の bio の PUT: status = %d (body %s)", res.code, res.body)
		}
		if got := decodeUserObject(t, []byte(get(router, uid.N(1), "").body))["bio"]; got != "" {
			t.Errorf("空文字で消したあとの bio = %q, want 空", got)
		}
	})

	t.Run("AC2 上限ちょうどは 200、超えると 422 で変更されない。日本語・絵文字はコードポイント数で数える", func(t *testing.T) {
		repo, router, token := newSeededUsersRouter(t)
		auth := token(uid.N(1))
		for _, unit := range []string{"a", "あ", "🍔"} {
			exact := strings.Repeat(unit, domain.MaxBioChars)
			if res := put(router, uid.N(1), `{"user":{"bio":`+jsonString(t, exact)+`}}`, auth); res.code != http.StatusOK {
				t.Errorf("%q × %d: status = %d, want 200 (body %.200s)", unit, domain.MaxBioChars, res.code, res.body)
			}
		}
		before := repo.users[uid.N(1)].user
		over := tooLong("Bio", domain.MaxBioChars)
		for _, unit := range []string{"a", "あ", "🍔"} {
			res := put(router, uid.N(1), `{"user":{"bio":`+jsonString(t, strings.Repeat(unit, domain.MaxBioChars+1))+`}}`, auth)
			if res.code != http.StatusUnprocessableEntity || res.body != over {
				t.Errorf("%q × %d: status/body = %d %s, want 422 %s", unit, domain.MaxBioChars+1, res.code, res.body, over)
			}
		}
		if repo.users[uid.N(1)].user != before {
			t.Errorf("422 なのにユーザーが変わった: %+v → %+v", before, repo.users[uid.N(1)].user)
		}
	})

	t.Run("AC3 他人・匿名の GET は bio を含むが、email と admin は含まない", func(t *testing.T) {
		_, router, token := newSeededUsersRouter(t)
		put(router, uid.N(1), `{"user":{"bio":"hello"}}`, token(uid.N(1)))
		for name, auth := range map[string]string{"匿名": "", "他人(bob)": token(uid.N(3)), "admin(root)": token(uid.N(4))} {
			res := get(router, uid.N(1), auth)
			if res.code != http.StatusOK {
				t.Fatalf("%s: status = %d", name, res.code)
			}
			obj := decodeUserObject(t, []byte(res.body))
			if got := userKeySet(obj); got != publicProfileKeys {
				t.Errorf("%s: keys = [%s], want [%s]", name, got, publicProfileKeys)
			}
			if obj["bio"] != "hello" {
				t.Errorf("%s: bio = %v, want hello", name, obj["bio"])
			}
		}
	})

	t.Run("AC4 他人(admin を含む)は bio を変えられない(403)", func(t *testing.T) {
		repo, router, token := newSeededUsersRouter(t)
		before := repo.users[uid.N(1)].user
		for name, auth := range map[string]string{"bob": token(uid.N(3)), "admin(root)": token(uid.N(4))} {
			res := put(router, uid.N(1), `{"user":{"bio":"hacked"}}`, auth)
			if res.code != http.StatusForbidden {
				t.Errorf("%s: status = %d, want 403 (body %s)", name, res.code, res.body)
			}
		}
		if repo.users[uid.N(1)].user != before {
			t.Errorf("403 なのにユーザーが変わった: %+v → %+v", before, repo.users[uid.N(1)].user)
		}
	})

	t.Run("signup は bio を受け付けず、空のまま作られる", func(t *testing.T) {
		repo, router, _ := newSeededUsersRouter(t)
		body := `{"username":"carol","email":"carol@example.com","password":"Password123!","bio":"ignored"}`
		rec := do(router, http.MethodPost, "/signup", body, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("signup status = %d (body %s)", rec.Code, rec.Body)
		}
		for _, rec := range repo.users {
			if rec.user.Username == "carol" && rec.user.Bio != "" {
				t.Errorf("signup で bio が設定された: %q", rec.user.Bio)
			}
		}
	})
}

// TestUserBioIntegration は、本物の PostgreSQL・repository・router を通して、bio が保存・取得され、
// 上限が Go のコードポイント数と PostgreSQL の char_length で同じに数えられること(絵文字 500 個が
// 保存できる)と、422 のとき DB が変わらないことを固定する(S28 AC1・AC2)。
func TestUserBioIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed integration test in short mode")
	}
	ctx := context.Background()
	conn, router := newUsersIntegrationKit(t)
	id, auth := signupUser(t, router, "biouser", "biouser@example.com", "Password123!")
	otherID, otherAuth := signupUser(t, router, "other", "other@example.com", "Password123!")

	storedBio := func(userID string) (string, int) {
		t.Helper()
		var bio string
		var chars int
		if err := conn.QueryRow(ctx, "SELECT bio, char_length(bio) FROM users WHERE id = $1", userID).Scan(&bio, &chars); err != nil {
			t.Fatalf("read bio: %v", err)
		}
		return bio, chars
	}

	if bio, _ := storedBio(id); bio != "" {
		t.Fatalf("signup 直後の bio = %q, want 空(DEFAULT '')", bio)
	}

	const text = "1 行目\n<b>2 行目</b> 🍔"
	rec := do(router, http.MethodPut, "/users/"+id, `{"user":{"bio":`+jsonString(t, text)+`}}`, auth)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT bio: status = %d (body %s)", rec.Code, rec.Body)
	}
	if bio, _ := storedBio(id); bio != text {
		t.Errorf("保存された bio = %q, want %q", bio, text)
	}
	anon := do(router, http.MethodGet, "/users/"+id, "", "")
	if got := decodeUserObject(t, anon.Body.Bytes()); got["bio"] != text || got["email"] != nil {
		t.Errorf("匿名の GET = %s, want bio が見え、email が無い", anon.Body)
	}

	exact := strings.Repeat("🍔", domain.MaxBioChars)
	if rec := do(router, http.MethodPut, "/users/"+id, `{"user":{"bio":`+jsonString(t, exact)+`}}`, auth); rec.Code != http.StatusOK {
		t.Fatalf("絵文字 %d 個: status = %d (body %.200s)", domain.MaxBioChars, rec.Code, rec.Body)
	}
	if _, chars := storedBio(id); chars != domain.MaxBioChars {
		t.Errorf("保存された bio の char_length = %d, want %d", chars, domain.MaxBioChars)
	}

	over := tooLong("Bio", domain.MaxBioChars)
	rec = do(router, http.MethodPut, "/users/"+id, `{"user":{"bio":`+jsonString(t, exact+"🍔")+`}}`, auth)
	if rec.Code != http.StatusUnprocessableEntity || rec.Body.String() != over {
		t.Errorf("超過: status/body = %d %s, want 422 %s", rec.Code, rec.Body, over)
	}
	if bio, _ := storedBio(id); bio != exact {
		t.Errorf("422 なのに bio が変わった(%d 文字)", len([]rune(bio)))
	}

	if rec := do(router, http.MethodPut, "/users/"+id, fmt.Sprintf(`{"user":{"bio":%s}}`, jsonString(t, "hacked")), otherAuth); rec.Code != http.StatusForbidden {
		t.Errorf("他人の PUT: status = %d, want 403", rec.Code)
	}
	if bio, _ := storedBio(id); bio != exact {
		t.Errorf("403 なのに bio が変わった")
	}
	if bio, _ := storedBio(otherID); bio != "" {
		t.Errorf("他人の bio が変わった: %q", bio)
	}

	if rec := do(router, http.MethodPut, "/users/"+id, `{"user":{"bio":""}}`, auth); rec.Code != http.StatusOK {
		t.Fatalf("空文字で消す: status = %d", rec.Code)
	}
	if bio, _ := storedBio(id); bio != "" {
		t.Errorf("空文字で消したあとの bio = %q", bio)
	}
}

// httpResult は、テストの読みやすさのための、status と body だけの束である。
type httpResult struct {
	code int
	body string
}
