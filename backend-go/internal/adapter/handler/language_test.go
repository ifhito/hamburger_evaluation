package handler_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// doWithLanguage は do と同じだが、Accept-Language ヘッダーを付ける(空なら付けない)。
func doWithLanguage(router http.Handler, method, path, body, authHeader, acceptLanguage string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if acceptLanguage != "" {
		req.Header.Set("Accept-Language", acceptLanguage)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestValidationErrorsFollowAcceptLanguage は、検証の失敗(422)の文言が、Accept-Language の言語で
// 返ることを、検証の失敗を返す主な経路(ログイン・登録・ショップ・レビュー・プロフィール)で確かめる。
// 指定がない・英語・対応しない言語は、いまと同じ英語である。
func TestValidationErrorsFollowAcceptLanguage(t *testing.T) {
	type call struct {
		name   string
		route  func(t *testing.T) (router http.Handler, auth string)
		method string
		path   string
		body   string
		en, ja string
	}
	authKit := func(t *testing.T) (http.Handler, string) {
		_, auth, _ := newAuthKit()
		return newTestRouterWith(t, okPinger, auth), ""
	}
	shopsKit := func(t *testing.T) (http.Handler, string) {
		router, alice, _, _ := newShopsRouter(t, &shopStoreFake{})
		return router, alice
	}
	reviewsKit := func(t *testing.T) (http.Handler, string) {
		router, alice, _, _ := newReviewsRouter(t, seedReviewWorld(uid.N(1)))
		return router, alice
	}
	reviewBody := fmt.Sprintf(`{"review":{"rating":0,"comment":"","shop_id":%q,"burger_id":%q}}`, activeShopID, cheeseBurgerID)
	calls := []call{
		{"ログイン: メールとパスワードが空", authKit, http.MethodPost, "/login", `{"email":"","password":""}`,
			`{"errors":["Email can't be blank","Password can't be blank"]}`,
			`{"errors":["メールアドレスを入力してください","パスワードを入力してください"]}`},
		{"ログイン: メールの形式が不正", authKit, http.MethodPost, "/login", `{"email":"abc","password":"Password123!"}`,
			`{"errors":["Email is invalid"]}`,
			`{"errors":["メールアドレスの形式が正しくありません"]}`},
		{"ログイン: 文字種の足りないパスワード", authKit, http.MethodPost, "/login", `{"email":"alice@example.com","password":"weakpassword"}`,
			`{"errors":["Password must include letters, numbers and symbols"]}`,
			`{"errors":["パスワードには、半角の英字・数字・記号を、それぞれ 1 文字以上含めてください"]}`},
		{"登録: 確認欄が一致しない", authKit, http.MethodPost, "/signup",
			`{"username":"eve","email":"eve@example.com","password":"Password123!","password_confirmation":"other"}`,
			`{"errors":["Password confirmation doesn't match Password"]}`,
			`{"errors":["パスワード(確認)が、パスワードと一致しません"]}`},
		{"登録: ユーザー名が空", authKit, http.MethodPost, "/signup",
			`{"username":"","email":"eve@example.com","password":"Password123!"}`,
			`{"errors":["Username can't be blank"]}`,
			`{"errors":["ユーザー名を入力してください"]}`},
		{"ショップの追加: 名前が空", shopsKit, http.MethodPost, "/shops", `{"shop":{"name":"   "}}`,
			`{"errors":["Name can't be blank"]}`,
			`{"errors":["ショップ名を入力してください"]}`},
		{"レビューの投稿: 評価が範囲外で、コメントが空(評価の文言が先)", reviewsKit, http.MethodPost, "/reviews", reviewBody,
			`{"errors":["Rating must be in 1..5","Comment can't be blank"]}`,
			`{"errors":["評価は 1〜5 の整数で入力してください","コメントを入力してください"]}`},
	}
	langs := []struct {
		name   string
		header string
		ja     bool
	}{
		{"指定なし", "", false},
		{"en", "en", false},
		{"en-US,en;q=0.9", "en-US,en;q=0.9", false},
		{"対応しない言語(fr)", "fr-FR", false},
		{"ja", "ja", true},
		{"ja-JP,ja;q=0.9,en;q=0.8", "ja-JP,ja;q=0.9,en;q=0.8", true},
	}
	for _, c := range calls {
		for _, l := range langs {
			t.Run(c.name+" / "+l.name, func(t *testing.T) {
				router, auth := c.route(t)
				rec := doWithLanguage(router, c.method, c.path, c.body, auth, l.header)
				if rec.Code != http.StatusUnprocessableEntity {
					t.Fatalf("status = %d, want 422 (body %s)", rec.Code, rec.Body)
				}
				want := c.en
				if l.ja {
					want = c.ja
				}
				if rec.Body.String() != want {
					t.Errorf("body = %s, want %s", rec.Body, want)
				}
			})
		}
	}
}
