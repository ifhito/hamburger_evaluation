package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// TestReviewsGetCanReview は、レビュー詳細の can_review が、ショップ詳細の can_review と同じ規則で
// 決まることを示す(同じショップ・同じ閲覧者で、2 つの use case が同じ答えを返す)。
func TestReviewsGetCanReview(t *testing.T) {
	alice := domain.User{ID: uid.N(1), Username: "alice"}
	bob := domain.User{ID: uid.N(2), Username: "bob"}
	admin := domain.User{ID: uid.N(3), Username: "root", Admin: true}
	shopsByID := map[string]domain.Shop{
		uid.N(10): {ID: uid.N(10), Status: domain.ShopStatusActive},
		uid.N(11): {ID: uid.N(11), Status: domain.ShopStatusPending, CreatorID: strPtr(alice.ID)},
		uid.N(12): {ID: uid.N(12), Status: domain.ShopStatusRejected, CreatorID: strPtr(alice.ID)},
	}
	reviews := newReviews(&fakeReviewQuery{
		getReview: func(_ context.Context, id string) (domain.ReviewDetail, error) {
			return domain.ReviewDetail{Review: domain.Review{ID: id, AuthorID: bob.ID}}, nil
		},
		getReviewShop: func(_ context.Context, reviewID string) (domain.Shop, error) {
			return shopsByID[reviewID], nil // テストでは、review の id に、属する shop の id を使う
		},
	}, &fakeReviewRepo{}, &fakePhotoStorage{})
	shops := usecase.NewShops(&fakeShopQuery{
		getShopWithCreator: func(_ context.Context, id string) (domain.ShopDetail, error) {
			return domain.ShopDetail{Shop: shopsByID[id]}, nil
		},
		listShopReviews: func(context.Context, string) ([]domain.ShopReview, error) { return nil, nil },
	}, domain.NewShops(&fakeShopRepo{}))

	for _, tt := range []struct {
		name   string
		viewer *domain.User
		shopID string
		want   bool
	}{
		{"匿名は、active なショップでも false", nil, uid.N(10), false},
		{"ログイン済みの利用者は、active なショップで true", &bob, uid.N(10), true},
		{"作成者は、自分の承認待ちのショップで true", &alice, uid.N(11), true},
		{"作成者以外は、承認待ちのショップで false", &bob, uid.N(11), false},
		{"管理者は、承認待ちのショップで true", &admin, uid.N(11), true},
		{"作成者は、却下されたショップで false", &alice, uid.N(12), false},
		{"管理者も、却下されたショップで false", &admin, uid.N(12), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			review, err := reviews.Get(context.Background(), tt.viewer, tt.shopID)
			if err != nil {
				t.Fatalf("Get returned error: %v", err)
			}
			if review.CanReview != tt.want {
				t.Errorf("レビュー詳細の CanReview = %v, want %v", review.CanReview, tt.want)
			}
			shop, err := shops.Get(context.Background(), tt.viewer, tt.shopID)
			switch {
			case errors.Is(err, domain.ErrShopNotFound):
				// 閲覧者に見えないショップ(他人の承認待ち・却下)は、詳細が返らない = レビューもできない
				if review.CanReview {
					t.Errorf("ショップ詳細が見えないのに、レビュー詳細の CanReview = true")
				}
			case err != nil:
				t.Fatalf("shops.Get returned error: %v", err)
			case shop.CanReview != review.CanReview:
				t.Errorf("ショップ詳細の CanReview = %v, レビュー詳細と食い違う(%v)", shop.CanReview, review.CanReview)
			}
		})
	}

	t.Run("レビューの属するショップが取れないときは、エラーを返す", func(t *testing.T) {
		boom := errors.New("boom")
		failing := newReviews(&fakeReviewQuery{
			getReview:     func(context.Context, string) (domain.ReviewDetail, error) { return domain.ReviewDetail{}, nil },
			getReviewShop: func(context.Context, string) (domain.Shop, error) { return domain.Shop{}, boom },
		}, &fakeReviewRepo{}, &fakePhotoStorage{})
		if _, err := failing.Get(context.Background(), &bob, uid.N(1)); !errors.Is(err, boom) {
			t.Errorf("err = %v, want boom", err)
		}
	})
}
