package handler_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// TestGetReviewShopAndCanReview は、レビュー詳細の shop と can_review が、そのレビューのショップの状態と閲覧者で決まる
// (ショップ詳細と同じ規則。匿名は false、active はログイン済みの全員、承認待ちは作成者と管理者、却下は全員 false。
// 見えないショップは shop に出さない)ことを、HTTP の応答で確かめる。
func TestGetReviewShopAndCanReview(t *testing.T) {
	repo := seedReviewWorld(uid.N(1)) // alice(id 1)が、承認待ちのショップの作成者
	rejectedOnlyBurger := uid.N(20)
	repo.burgers[rejectedOnlyBurger] = domain.ShopReviewBurger{ID: rejectedOnlyBurger, Name: "Grilled"}
	repo.links[rejectedShopID] = append(repo.links[rejectedShopID], rejectedOnlyBurger)
	// 古い承認待ち(alice のもの。id が小さい方が古い)と、新しい承認済みのショップの、両方にあるバーガー。
	// id と名前は、ほかの値と重ならないものにする(応答に出ていないことを、文字列で確かめるため)。
	hiddenPendingID, newActiveID, sharedBurger, orphanBurger := uid.N(30), uid.N(31), uid.N(21), uid.N(22)
	repo.shops[hiddenPendingID] = domain.Shop{ID: hiddenPendingID, Name: "Hidden Pending Grill", Status: domain.ShopStatusPending, CreatorID: shopPtr(uid.N(1))}
	repo.shops[newActiveID] = domain.Shop{ID: newActiveID, Name: "New Active Diner", Status: domain.ShopStatusActive}
	repo.burgers[sharedBurger] = domain.ShopReviewBurger{ID: sharedBurger, Name: "Shared"}
	repo.links[hiddenPendingID] = []string{sharedBurger}
	repo.links[newActiveID] = []string{sharedBurger}
	repo.burgers[orphanBurger] = domain.ShopReviewBurger{ID: orphanBurger, Name: "Orphan"} // どのショップにもない

	seed := func(id, burgerID string) string {
		repo.reviews[id] = &fakeStoredReview{review: domain.Review{
			ID: id, Rating: 4, AuthorID: uid.N(2), BurgerID: burgerID, CreatedAt: reviewBaseTime,
		}}
		return "/reviews/" + id
	}
	activePath := seed(uid.N(101), cheeseBurgerID)       // Cheese は複数のショップ(承認済み・承認待ち・却下)にある
	pendingPath := seed(uid.N(102), plainBurgerID)       // 承認待ちのショップにだけある
	rejectedPath := seed(uid.N(103), rejectedOnlyBurger) // 却下されたショップにだけある
	sharedPath := seed(uid.N(104), sharedBurger)         // 古い承認待ち + 新しい承認済み
	orphanPath := seed(uid.N(105), orphanBurger)         // ショップに紐づかない
	router, aliceAuth, bobAuth, adminAuth := newReviewsRouter(t, repo)

	for _, tt := range []struct {
		name     string
		path     string
		auth     string
		wantShop string // ショップの id。"" は null
		want     bool
	}{
		{"匿名は、承認済みのショップのレビュー詳細で、そのショップが見え、can_review は false", activePath, "", uid.N(1), false},
		{"ログイン済みの利用者は、承認済みのショップのレビュー詳細で true", activePath, bobAuth, uid.N(1), true},
		{"作成者は、自分の承認待ちのショップのレビュー詳細で true", pendingPath, aliceAuth, uid.N(2), true},
		{"作成者以外は、承認待ちのショップのレビュー詳細で、ショップが見えず(null)、false", pendingPath, bobAuth, "", false},
		{"管理者は、承認待ちのショップのレビュー詳細で true", pendingPath, adminAuth, uid.N(2), true},
		{"匿名は、承認待ちのショップのレビュー詳細で、ショップが見えず(null)、false", pendingPath, "", "", false},
		{"管理者は、却下されたショップのレビュー詳細で、ショップは見えるが false", rejectedPath, adminAuth, uid.N(3), false},
		{"ログイン済みの利用者は、却下されたショップのレビュー詳細で、ショップが見えず(null)、false", rejectedPath, bobAuth, "", false},
		{"古い承認待ち(他人のもの)と新しい承認済みのショップがあるとき、見えるのは新しい承認済みのショップで、そこに書ける", sharedPath, bobAuth, newActiveID, true},
		{"同じとき、その承認待ちのショップの作成者は、書ける方(承認待ちの自分のショップ)が先になる", sharedPath, aliceAuth, hiddenPendingID, true},
		{"ショップに紐づかないバーガーのレビューでも、詳細は 200 で、shop は null・can_review は false", orphanPath, bobAuth, "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(router, http.MethodGet, tt.path, "", tt.auth)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
			}
			var body struct {
				CanReview *bool `json:"can_review"`
				Shop      *struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"shop"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.CanReview == nil || *body.CanReview != tt.want {
				t.Errorf("can_review = %v, want %v (body %s)", body.CanReview, tt.want, rec.Body)
			}
			switch {
			case tt.wantShop == "" && body.Shop != nil:
				t.Errorf("shop = %+v, want null(見えないショップは返さない)", body.Shop)
			case tt.wantShop != "" && (body.Shop == nil || body.Shop.ID != tt.wantShop || body.Shop.Name == ""):
				t.Errorf("shop = %+v, want id %s と名前", body.Shop, tt.wantShop)
			}
		})
	}

	t.Run("見えないショップの id も名前も、応答のどこにも出ない", func(t *testing.T) {
		rec := do(router, http.MethodGet, sharedPath, "", bobAuth)
		for _, hidden := range []string{hiddenPendingID, "Hidden Pending Grill"} {
			if strings.Contains(rec.Body.String(), hidden) {
				t.Errorf("body = %s, want 見えない承認待ちのショップの %q を含まない", rec.Body, hidden)
			}
		}
	})

	t.Run("既存の項目は残り、can_review と shop が増えるだけである", func(t *testing.T) {
		rec := do(router, http.MethodGet, activePath, "", bobAuth)
		var object map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &object); err != nil {
			t.Fatalf("body = %s (%v)", rec.Body, err)
		}
		wantKeys := []string{"burger", "can_edit", "can_review", "comment", "created_at", "id", "photo_url", "rating", "shop", "user", "visited_at"}
		if got := slices.Sorted(maps.Keys(object)); !slices.Equal(got, wantKeys) {
			t.Errorf("詳細のキー = %v, want %v", got, wantKeys)
		}
	})

	t.Run("一覧は can_review も shop も持たない(詳細だけの項目)", func(t *testing.T) {
		rec := do(router, http.MethodGet, "/reviews", "", bobAuth)
		if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "can_review") || strings.Contains(rec.Body.String(), `"shop"`) {
			t.Errorf("status = %d, body = %s, want 一覧に can_review・shop がない", rec.Code, rec.Body)
		}
	})
}
