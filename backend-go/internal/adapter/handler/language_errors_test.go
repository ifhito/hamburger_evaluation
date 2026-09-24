package handler_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// TestClientErrorsFollowAcceptLanguage は、検証の失敗(422 の domain の文言。language_test.go)以外の、利用者に
// 見える 4xx(見つからない・権限・認証・リクエストの形・写真・クエリの誤り)が、Accept-Language の言語で
// 返ることを、handler が返す種類ごとに確かめる。指定がない・英語・対応しない言語は、いまと同じ英語である。
func TestClientErrorsFollowAcceptLanguage(t *testing.T) {
	type errCase struct {
		name   string
		run    func(t *testing.T, acceptLanguage string) *httptest.ResponseRecorder
		status int
		en, ja string
	}
	reviewFields := map[string]string{"rating": "4", "comment": "Tasty", "shop_id": activeShopID, "burger_id": cheeseBurgerID}
	users := func(method, path, body, as string) func(*testing.T, string) *httptest.ResponseRecorder {
		return func(t *testing.T, lang string) *httptest.ResponseRecorder {
			repo, router, token := newUsersRouter(t)
			alice := repo.seed("alice", "alice@example.com", "Password123!")
			bob := repo.seed("bob", "bob@example.com", "Password123!")
			auth := map[string]string{"": "", "alice": token(alice.ID), "bob": token(bob.ID)}[as]
			return doWithLanguage(router, method, path, body, auth, lang)
		}
	}
	shops := func(method, path string) func(*testing.T, string) *httptest.ResponseRecorder {
		return func(t *testing.T, lang string) *httptest.ResponseRecorder {
			router, _, _, _ := newShopsRouter(t, seedShops(uid.N(1)))
			return doWithLanguage(router, method, path, "", "", lang)
		}
	}
	adminShops := func(method, path, body string, asAdmin bool) func(*testing.T, string) *httptest.ResponseRecorder {
		return func(t *testing.T, lang string) *httptest.ResponseRecorder {
			router, alice, admin, _ := newShopsRouter(t, seedShops(uid.N(1)))
			auth := alice
			if asAdmin {
				auth = admin
			}
			return doWithLanguage(router, method, path, body, auth, lang)
		}
	}
	reviews := func(method, path, body string, signedIn bool) func(*testing.T, string) *httptest.ResponseRecorder {
		return func(t *testing.T, lang string) *httptest.ResponseRecorder {
			router, alice, _, _ := newReviewsRouter(t, seedReviewWorld(uid.N(1)))
			if !signedIn {
				alice = ""
			}
			return doWithLanguage(router, method, path, body, alice, lang)
		}
	}
	multipart := func(fields map[string]string, photos ...[]byte) func(*testing.T, string) *httptest.ResponseRecorder {
		return func(t *testing.T, lang string) *httptest.ResponseRecorder {
			router, _, alice, _, _ := newPhotoReviewsRouter(t, seedReviewWorld(uid.N(1)))
			body, contentType := multipartBody(t, fields, photos...)
			req := httptest.NewRequest(http.MethodPost, "/reviews", body)
			req.Header.Set("Content-Type", contentType)
			req.Header.Set("Authorization", alice)
			if lang != "" {
				req.Header.Set("Accept-Language", lang)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			return rec
		}
	}
	single := func(en, ja string) (string, string) {
		return fmt.Sprintf(`{"error":%q}`, en), fmt.Sprintf(`{"error":%q}`, ja)
	}
	list := func(en, ja []string) (string, string) {
		quote := func(items []string) string {
			quoted := make([]string, len(items))
			for i, item := range items {
				quoted[i] = fmt.Sprintf("%q", item)
			}
			return `{"errors":[` + strings.Join(quoted, ",") + `]}`
		}
		return quote(en), quote(ja)
	}

	var cases []errCase
	add := func(name string, run func(*testing.T, string) *httptest.ResponseRecorder, status int, en, ja string) {
		cases = append(cases, errCase{name, run, status, en, ja})
	}
	{
		en, ja := single("User not found", "ユーザーが見つかりません")
		add("見つからない: ユーザー", users(http.MethodGet, "/users/"+uid.N(999), "", ""), http.StatusNotFound, en, ja)
	}
	{
		en, ja := single("Shop not found", "ショップが見つかりません")
		add("見つからない: ショップ", shops(http.MethodGet, "/shops/"+uid.N(999)), http.StatusNotFound, en, ja)
	}
	{
		en, ja := single("Review not found", "レビューが見つかりません")
		add("見つからない: レビュー", reviews(http.MethodGet, "/reviews/"+uid.N(999), "", false), http.StatusNotFound, en, ja)
	}
	{
		en, ja := single("Burger not found", "バーガーが見つかりません")
		body := fmt.Sprintf(`{"review":{"rating":4,"comment":"x","shop_id":%q,"burger_id":%q}}`, activeShopID, plainBurgerID)
		add("見つからない: ショップが出していないバーガー", reviews(http.MethodPost, "/reviews", body, true), http.StatusNotFound, en, ja)
	}
	{
		en, ja := single("not found", "見つかりません")
		add("見つからない: 存在しないパス", reviews(http.MethodGet, "/no/such/path", "", false), http.StatusNotFound, en, ja)
	}
	{
		en, ja := single("method not allowed", "このメソッドは使えません")
		add("許可されないメソッド", reviews(http.MethodGet, "/login", "", false), http.StatusMethodNotAllowed, en, ja)
	}
	{
		en, ja := single("Forbidden", "この操作をする権限がありません")
		add("権限がない: 他人のプロフィールの更新", users(http.MethodPut, "/users/"+uid.N(1), `{"user":{"username":"x"}}`, "bob"), http.StatusForbidden, en, ja)
	}
	{
		en, ja := single("Forbidden", "この操作をする権限がありません")
		add("権限がない: 管理者でない利用者のショップの変更", adminShops(http.MethodPut, "/admin/shops/"+uid.N(2), `{"shop":{"name":"x"}}`, false), http.StatusForbidden, en, ja)
	}
	{
		en, ja := single("Shop not found", "ショップが見つかりません")
		add("見つからない: 管理者の承認の対象のショップ", adminShops(http.MethodPost, "/admin/shops/"+uid.N(999)+"/approve", "", true), http.StatusNotFound, en, ja)
	}
	{
		en, ja := single("Unauthorized", "サインインが必要です")
		add("認証がない: サインインなしのレビューの投稿", reviews(http.MethodPost, "/reviews", `{}`, false), http.StatusUnauthorized, en, ja)
	}
	{
		en, ja := single("Invalid email or password", "メールアドレスかパスワードが違います")
		add("認証の失敗: パスワードの誤り", users(http.MethodPost, "/login", `{"email":"alice@example.com","password":"Wrong123!x"}`, ""), http.StatusUnauthorized, en, ja)
	}
	{
		en, ja := single("Confirmation token is invalid or has expired", "確認のリンクが、無効か期限切れです")
		add("確認のトークンが無効", func(t *testing.T, lang string) *httptest.ResponseRecorder {
			return doWithLanguage(newSignupKit(t).router, http.MethodPost, "/signup/confirm", confirmBody("nope"), "", lang)
		}, http.StatusBadRequest, en, ja)
	}
	{
		en, ja := single("invalid JSON body", "送られた内容が、JSON として読めません")
		add("リクエストの形: JSON が壊れている", users(http.MethodPost, "/login", `{`, ""), http.StatusBadRequest, en, ja)
	}
	{
		en, ja := single("request body too large", "送られた内容が大きすぎます")
		add("リクエストの形: 本文が大きすぎる", func(t *testing.T, lang string) *httptest.ResponseRecorder {
			// Content-Length を隠して、事前の検査を迂回し、読み取りの上限を作動させる。
			body := struct{ io.Reader }{strings.NewReader(`{"username":"` + strings.Repeat("a", (10<<20)+1))}
			req := httptest.NewRequest(http.MethodPost, "/signup", body)
			if lang != "" {
				req.Header.Set("Accept-Language", lang)
			}
			rec := httptest.NewRecorder()
			newTestRouter(t, okPinger).ServeHTTP(rec, req)
			return rec
		}, http.StatusRequestEntityTooLarge, en, ja)
	}
	{
		en, ja := single("invalid multipart body", "送られた内容(multipart)が読めません")
		add("リクエストの形: multipart の境界がない", func(t *testing.T, lang string) *httptest.ResponseRecorder {
			router, _, alice, _, _ := newPhotoReviewsRouter(t, seedReviewWorld(uid.N(1)))
			req := httptest.NewRequest(http.MethodPost, "/reviews", strings.NewReader("garbage"))
			req.Header.Set("Content-Type", "multipart/form-data")
			req.Header.Set("Authorization", alice)
			if lang != "" {
				req.Header.Set("Accept-Language", lang)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			return rec
		}, http.StatusBadRequest, en, ja)
	}
	{
		en, ja := single("duplicate photo field", "写真は 1 枚だけ送ってください")
		add("リクエストの形: 写真が 2 枚", func(t *testing.T, lang string) *httptest.ResponseRecorder {
			return multipart(reviewFields, pngBytes(t), pngBytes(t))(t, lang)
		}, http.StatusBadRequest, en, ja)
	}
	{
		en, ja := single("multipart field too large", "送られた項目が大きすぎます")
		fields := map[string]string{"comment": strings.Repeat("a", 70<<10)}
		add("リクエストの形: テキストの項目が大きすぎる", multipart(fields), http.StatusBadRequest, en, ja)
	}
	{
		en, ja := list([]string{"Photo is too large (max 5MB)"}, []string{"写真が大きすぎます(最大 5 MB)"})
		add("写真: ファイルが大きすぎる", func(t *testing.T, lang string) *httptest.ResponseRecorder {
			return multipart(reviewFields, jpegBytes(t, 6_000_000))(t, lang)
		}, http.StatusUnprocessableEntity, en, ja)
	}
	{
		en, ja := list([]string{"Photo dimensions are too large (max 10000px per side and 24 megapixels)"}, []string{"写真の縦横が大きすぎます(1 辺が最大 10000 px、全体で最大 24 メガピクセル)"})
		add("写真: 縦横が大きすぎる", func(t *testing.T, lang string) *httptest.ResponseRecorder {
			return multipart(reviewFields, jpegWithDeclaredSize(t, 12_000, 100))(t, lang)
		}, http.StatusUnprocessableEntity, en, ja)
	}
	{
		en, ja := list([]string{"Photo must be a JPEG, PNG, or WebP image (HEIC/HEIF is not supported)"}, []string{"写真は、JPEG・PNG・WebP の画像にしてください(HEIC・HEIF は使えません)"})
		add("写真: HEIC", multipart(reviewFields, fileWithBrand("heic")), http.StatusUnprocessableEntity, en, ja)
	}
	{
		en, ja := list([]string{"Photo must be a JPEG, PNG, or WebP image"}, []string{"写真は、JPEG・PNG・WebP の画像にしてください"})
		add("写真: 画像でないファイル", multipart(reviewFields, []byte("just some text, definitely not an image")), http.StatusUnprocessableEntity, en, ja)
	}
	{
		en, ja := list([]string{"Rating must be an integer"}, []string{"評価は、整数で指定してください"})
		add("クエリ: 評価が整数でない", reviews(http.MethodGet, "/reviews?rating=abc", "", false), http.StatusUnprocessableEntity, en, ja)
	}
	{
		en, ja := list([]string{"Shop id must be a valid UUID"}, []string{"ショップの ID の形式が正しくありません"})
		add("クエリ: ショップの ID の形式", reviews(http.MethodGet, "/reviews?shop_id=x", "", false), http.StatusUnprocessableEntity, en, ja)
		add("投稿: ショップの ID の形式(JSON)", reviews(http.MethodPost, "/reviews", `{"review":{"rating":4,"comment":"x","shop_id":"x"}}`, true), http.StatusUnprocessableEntity, en, ja)
		add("投稿: ショップの ID の形式(multipart)", multipart(map[string]string{"rating": "4", "comment": "x", "shop_id": "x"}), http.StatusUnprocessableEntity, en, ja)
	}
	{
		en, ja := list([]string{"User id must be a valid UUID"}, []string{"ユーザーの ID の形式が正しくありません"})
		add("クエリ: ユーザーの ID の形式", reviews(http.MethodGet, "/reviews?user_id=x", "", false), http.StatusUnprocessableEntity, en, ja)
	}
	{
		en, ja := list([]string{"Burger id must be a valid UUID"}, []string{"バーガーの ID の形式が正しくありません"})
		body := fmt.Sprintf(`{"review":{"rating":4,"comment":"x","shop_id":%q,"burger_id":"x"}}`, activeShopID)
		add("投稿: バーガーの ID の形式", reviews(http.MethodPost, "/reviews", body, true), http.StatusUnprocessableEntity, en, ja)
	}
	{
		en, ja := list([]string{"Page must be an integer", "Per page must be an integer"}, []string{"ページは、整数で指定してください", "1 ページの件数は、整数で指定してください"})
		add("クエリ: ページと件数が整数でない", shops(http.MethodGet, "/shops?page=a&per_page=b"), http.StatusUnprocessableEntity, en, ja)
	}

	// 言語の選び方そのものは lang_test.go が持つので、ここでは、英語(指定なし)と日本語の 2 通りだけを見る。
	languages := []struct {
		name, header string
		ja           bool
	}{
		{"指定なし", "", false},
		{"ja", "ja", true},
		{"ja-JP,ja;q=0.9,en;q=0.8", "ja-JP,ja;q=0.9,en;q=0.8", true},
	}
	for _, c := range cases {
		for _, l := range languages {
			t.Run(c.name+" / "+l.name, func(t *testing.T) {
				rec := c.run(t, l.header)
				if rec.Code != c.status {
					t.Fatalf("status = %d, want %d (body %s)", rec.Code, c.status, rec.Body)
				}
				want := c.en
				if l.ja {
					want = c.ja
				}
				if rec.Body.String() != want {
					t.Errorf("body = %s, want %s", rec.Body, want)
				}
				if !slices.Contains(rec.Header().Values("Vary"), "Accept-Language") {
					t.Errorf("Vary = %q, want Accept-Language(言語で本文が変わるので、キャッシュに伝える)", rec.Header().Values("Vary"))
				}
			})
		}
	}
}
