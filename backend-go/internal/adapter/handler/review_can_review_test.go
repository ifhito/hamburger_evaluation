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

// TestGetReviewCanReview は、レビュー詳細の can_review が、そのレビューのショップの状態と閲覧者で決まる
// (ショップ詳細と同じ規則。匿名は false、active はログイン済みの全員、承認待ちは作成者と管理者、却下は全員 false)ことを、
// HTTP の応答で確かめる。
func TestGetReviewCanReview(t *testing.T) {
	repo := seedReviewWorld(uid.N(1)) // alice(id 1)が、承認待ちのショップの作成者
	rejectedOnlyBurger := uid.N(20)
	repo.burgers[rejectedOnlyBurger] = domain.ShopReviewBurger{ID: rejectedOnlyBurger, Name: "Grilled"}
	repo.links[rejectedShopID] = append(repo.links[rejectedShopID], rejectedOnlyBurger)

	seed := func(id, burgerID string) string {
		repo.reviews[id] = &fakeStoredReview{review: domain.Review{
			ID: id, Rating: 4, AuthorID: uid.N(2), BurgerID: burgerID, CreatedAt: reviewBaseTime,
		}}
		return "/reviews/" + id
	}
	activePath := seed(uid.N(101), cheeseBurgerID)       // Cheese は複数のショップにあり、id が最小の active なショップに属する
	pendingPath := seed(uid.N(102), plainBurgerID)       // 承認待ちのショップにだけある
	rejectedPath := seed(uid.N(103), rejectedOnlyBurger) // 却下されたショップにだけある
	router, aliceAuth, bobAuth, adminAuth := newReviewsRouter(t, repo)

	for _, tt := range []struct {
		name string
		path string
		auth string
		want bool
	}{
		{"匿名は、承認済みのショップのレビュー詳細で false", activePath, "", false},
		{"ログイン済みの利用者は、承認済みのショップのレビュー詳細で true", activePath, bobAuth, true},
		{"作成者は、自分の承認待ちのショップのレビュー詳細で true", pendingPath, aliceAuth, true},
		{"作成者以外は、承認待ちのショップのレビュー詳細で false", pendingPath, bobAuth, false},
		{"管理者は、承認待ちのショップのレビュー詳細で true", pendingPath, adminAuth, true},
		{"匿名は、承認待ちのショップのレビュー詳細で false", pendingPath, "", false},
		{"管理者も、却下されたショップのレビュー詳細で false", rejectedPath, adminAuth, false},
		{"ログイン済みの利用者は、却下されたショップのレビュー詳細で false", rejectedPath, bobAuth, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(router, http.MethodGet, tt.path, "", tt.auth)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
			}
			var body struct {
				CanReview *bool `json:"can_review"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.CanReview == nil || *body.CanReview != tt.want {
				t.Errorf("can_review = %v, want %v (body %s)", body.CanReview, tt.want, rec.Body)
			}
		})
	}

	t.Run("既存の項目は残り、can_review が 1 つ増えるだけである", func(t *testing.T) {
		rec := do(router, http.MethodGet, activePath, "", bobAuth)
		var object map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &object); err != nil {
			t.Fatalf("body = %s (%v)", rec.Body, err)
		}
		wantKeys := []string{"burger", "can_edit", "can_review", "comment", "created_at", "id", "photo_url", "rating", "user"}
		if got := slices.Sorted(maps.Keys(object)); !slices.Equal(got, wantKeys) {
			t.Errorf("詳細のキー = %v, want %v", got, wantKeys)
		}
	})

	t.Run("一覧は can_review を持たない(詳細だけの項目)", func(t *testing.T) {
		rec := do(router, http.MethodGet, "/reviews", "", bobAuth)
		if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "can_review") {
			t.Errorf("status = %d, body = %s, want 一覧に can_review がない", rec.Code, rec.Body)
		}
	})
}
