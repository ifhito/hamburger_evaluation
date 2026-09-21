package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// decodeJSONObject は body を単一の JSON オブジェクトとして読む。
func decodeJSONObject(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		t.Fatalf("body %q is not a JSON object: %v", body, err)
	}
	return obj
}

// decodeJSONArray は body を JSON の配列として読む。
func decodeJSONArray(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var arr []map[string]any
	if err := json.Unmarshal(body, &arr); err != nil {
		t.Fatalf("body %q is not a JSON array: %v", body, err)
	}
	return arr
}

// TestGetShopCanReview は GET /shops/{id} の can_review が、domain の reviewable ルール
// （匿名は false）どおりに返ることを固定する（S24 AC3）。
func TestGetShopCanReview(t *testing.T) {
	repo := seedShops(uid.N(1)) // alice(1) は pending な shop 2 の creator。admin は id 2
	repo.shops = append(repo.shops, domain.ShopDetail{
		Shop: domain.Shop{ID: uid.N(5), Name: "Alice Rejected", Status: domain.ShopStatusRejected, CreatorID: shopPtr(uid.N(1))},
	})
	router, aliceAuth, adminAuth, _ := newShopsRouter(t, repo)

	tests := []struct {
		name string
		path string
		auth string
		want bool
	}{
		{name: "匿名は active な shop でも false", path: "/shops/" + uid.N(1), auth: "", want: false},
		{name: "ログイン済みのユーザーは active な shop で true", path: "/shops/" + uid.N(1), auth: aliceAuth, want: true},
		{name: "creator は自分の pending な shop で true", path: "/shops/" + uid.N(2), auth: aliceAuth, want: true},
		{name: "admin は pending な shop で true", path: "/shops/" + uid.N(2), auth: adminAuth, want: true},
		{name: "creator は自分の rejected な shop でも false（見えるが review できない）", path: "/shops/" + uid.N(5), auth: aliceAuth, want: false},
		{name: "admin は active な shop で true", path: "/shops/" + uid.N(1), auth: adminAuth, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(router, http.MethodGet, tt.path, "", tt.auth)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
			}
			if got := decodeJSONObject(t, rec.Body.Bytes())["can_review"]; got != tt.want {
				t.Errorf("can_review = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestReviewCanEdit は /reviews 系の can_edit が、domain の所有権ルール（author だけ。
// admin にも例外なし。匿名は false）どおりに、詳細・一覧・作成・更新のレスポンスに
// 返ることを固定する（S24 AC1）。
func TestReviewCanEdit(t *testing.T) {
	repo := seedReviewWorld(uid.N(1))
	router, aliceAuth, bobAuth, adminAuth := newReviewsRouter(t, repo)
	cheeseReviewID, _ := seedFeed(t, router, aliceAuth) // alice の review が feed に 1 件ある

	viewers := []struct {
		name string
		auth string
		want bool
	}{
		{name: "匿名", auth: "", want: false},
		{name: "author の alice", auth: aliceAuth, want: true},
		{name: "他人の bob", auth: bobAuth, want: false},
		{name: "admin でも他人の review は編集できない", auth: adminAuth, want: false},
	}
	for _, v := range viewers {
		t.Run("詳細: "+v.name, func(t *testing.T) {
			rec := do(router, http.MethodGet, fmt.Sprintf("/reviews/%d", cheeseReviewID), "", v.auth)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
			}
			if got := decodeJSONObject(t, rec.Body.Bytes())["can_edit"]; got != v.want {
				t.Errorf("can_edit = %v, want %v", got, v.want)
			}
		})
		t.Run("一覧: "+v.name, func(t *testing.T) {
			rec := do(router, http.MethodGet, "/reviews", "", v.auth)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
			}
			list := decodeJSONArray(t, rec.Body.Bytes())
			if len(list) != 1 {
				t.Fatalf("len(list) = %d, want 1 (body %s)", len(list), rec.Body)
			}
			if got := list[0]["can_edit"]; got != v.want {
				t.Errorf("can_edit = %v, want %v", got, v.want)
			}
		})
	}

	t.Run("作成のレスポンスは投稿者本人なので true", func(t *testing.T) {
		body := fmt.Sprintf(`{"review":{"rating":3,"comment":"mine","shop_id":%q,"burger_id":%q}}`, activeShopID, cheeseBurgerID)
		rec := do(router, http.MethodPost, "/reviews", body, bobAuth)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (body %s)", rec.Code, rec.Body)
		}
		if got := decodeJSONObject(t, rec.Body.Bytes())["can_edit"]; got != true {
			t.Errorf("can_edit = %v, want true", got)
		}
	})

	t.Run("更新のレスポンスは author 本人なので true", func(t *testing.T) {
		rec := do(router, http.MethodPut, fmt.Sprintf("/reviews/%d", cheeseReviewID), `{"review":{"rating":5,"comment":"edited"}}`, aliceAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
		}
		if got := decodeJSONObject(t, rec.Body.Bytes())["can_edit"]; got != true {
			t.Errorf("can_edit = %v, want true", got)
		}
	})

	t.Run("shop 詳細に埋め込まれる review には can_edit を含めない", func(t *testing.T) {
		// shop 詳細の review は閲覧者ごとの値を持たない（別の型）。frontend が誤って使えないようにする。
		shopRepo := seedShops(uid.N(1))
		shopRepo.reviews[uid.N(1)] = []domain.ShopReview{{ID: 9, Rating: 4, User: &domain.UserRef{ID: uid.N(3), Username: "bob"}}}
		shopRouter, _, _, _ := newShopsRouter(t, shopRepo)
		rec := do(shopRouter, http.MethodGet, "/shops/"+uid.N(1), "", "")
		reviews, ok := decodeJSONObject(t, rec.Body.Bytes())["reviews"].([]any)
		if !ok || len(reviews) != 1 {
			t.Fatalf("reviews = %v, want 1 element (body %s)", reviews, rec.Body)
		}
		if _, has := reviews[0].(map[string]any)["can_edit"]; has {
			t.Errorf("embedded review has can_edit, want none (body %s)", rec.Body)
		}
	})
}

// hasMoreCase は、次のページの有無（X-Has-More）の期待を 1 件ぶん表す。
type hasMoreCase struct {
	name     string
	total    int    // 見える項目の総数
	query    string // page / per_page の query
	wantLen  int
	wantMore string
}

var hasMoreCases = []hasMoreCase{
	{name: "0 件は false", total: 0, query: "", wantLen: 0, wantMore: "false"},
	{name: "ちょうど 1 ページぶん（20 件）なら false（空のページを取りに行かせない）", total: 20, query: "", wantLen: 20, wantMore: "false"},
	{name: "21 件の 1 ページ目は true", total: 21, query: "", wantLen: 20, wantMore: "true"},
	{name: "21 件の 2 ページ目は 1 件で false", total: 21, query: "?page=2", wantLen: 1, wantMore: "false"},
	{name: "40 件（20 の倍数）の 1 ページ目は true", total: 40, query: "?page=1", wantLen: 20, wantMore: "true"},
	{name: "40 件（20 の倍数）の最終ページは false", total: 40, query: "?page=2", wantLen: 20, wantMore: "false"},
	{name: "範囲外のページは空で false", total: 5, query: "?page=9", wantLen: 0, wantMore: "false"},
	{name: "per_page を指定した場合の途中のページは true", total: 10, query: "?page=1&per_page=5", wantLen: 5, wantMore: "true"},
	{name: "per_page を指定した場合の最終ページは false", total: 10, query: "?page=2&per_page=5", wantLen: 5, wantMore: "false"},
	{name: "per_page がちょうど総数なら false", total: 10, query: "?per_page=10", wantLen: 10, wantMore: "false"},
}

// TestListShopsHasMore は GET /shops が次のページの有無をレスポンスヘッダー X-Has-More で
// 返すことを固定する（S24 AC4）。本文は従来どおりの配列である。
func TestListShopsHasMore(t *testing.T) {
	for _, tt := range hasMoreCases {
		t.Run(tt.name, func(t *testing.T) {
			repo := seedShops(uid.N(1))
			repo.shops = nil
			for i := 1; i <= tt.total; i++ {
				repo.shops = append(repo.shops, domain.ShopDetail{Shop: domain.Shop{
					ID: uid.N(i), Name: fmt.Sprintf("Shop %03d", i), Status: domain.ShopStatusActive,
				}})
			}
			router, _, _, _ := newShopsRouter(t, repo)

			rec := do(router, http.MethodGet, "/shops"+tt.query, "", "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
			}
			if got := len(decodeJSONArray(t, rec.Body.Bytes())); got != tt.wantLen {
				t.Errorf("len(body) = %d, want %d", got, tt.wantLen)
			}
			if got := rec.Header().Get("X-Has-More"); got != tt.wantMore {
				t.Errorf("X-Has-More = %q, want %q", got, tt.wantMore)
			}
		})
	}
}

// TestListReviewsHasMore は GET /reviews が次のページの有無をレスポンスヘッダー X-Has-More で
// 返すことを固定する（S24 AC4）。本文は従来どおりの配列である。
func TestListReviewsHasMore(t *testing.T) {
	for _, tt := range hasMoreCases {
		t.Run(tt.name, func(t *testing.T) {
			repo := seedReviewWorld(uid.N(1))
			for i := 1; i <= tt.total; i++ {
				id := int64(i)
				repo.reviews[id] = &fakeStoredReview{review: domain.Review{
					ID: id, Rating: 3, AuthorID: uid.N(1), BurgerID: cheeseBurgerID,
					CreatedAt: reviewBaseTime.Add(0),
				}}
			}
			router, _, _, _ := newReviewsRouter(t, repo)

			rec := do(router, http.MethodGet, "/reviews"+tt.query, "", "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
			}
			if got := len(decodeJSONArray(t, rec.Body.Bytes())); got != tt.wantLen {
				t.Errorf("len(body) = %d, want %d", got, tt.wantLen)
			}
			if got := rec.Header().Get("X-Has-More"); got != tt.wantMore {
				t.Errorf("X-Has-More = %q, want %q", got, tt.wantMore)
			}
		})
	}
}

// TestListHasMoreNotSetOnError は、page / per_page が不正で 422 になる場合は、
// X-Has-More を付けないことを固定する（本文の契約にない値を返さない）。
func TestListHasMoreNotSetOnError(t *testing.T) {
	shopRouter, _, _, _ := newShopsRouter(t, seedShops(uid.N(1)))
	reviewRouter, _, _, _ := newReviewsRouter(t, seedReviewWorld(uid.N(1)))
	for name, router := range map[string]http.Handler{"/shops": shopRouter, "/reviews": reviewRouter} {
		t.Run(name, func(t *testing.T) {
			rec := do(router, http.MethodGet, name+"?page=abc", "", "")
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422", rec.Code)
			}
			if got := rec.Header().Get("X-Has-More"); got != "" {
				t.Errorf("X-Has-More = %q, want empty", got)
			}
		})
	}
}
