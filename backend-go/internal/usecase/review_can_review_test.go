package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// TestReviewsGetShopAndCanReview は、レビュー詳細の Shop と can_review が、ショップ詳細の can_review と同じ規則で
// 決まることを示す(同じショップ・同じ閲覧者で、2 つの use case が同じ答えを返す)。
func TestReviewsGetShopAndCanReview(t *testing.T) {
	alice := domain.User{ID: uid.N(1), Username: "alice"}
	bob := domain.User{ID: uid.N(2), Username: "bob"}
	admin := domain.User{ID: uid.N(3), Username: "root", Admin: true}
	shopsByID := map[string]domain.Shop{
		uid.N(10): {ID: uid.N(10), Name: "Active", Status: domain.ShopStatusActive},
		uid.N(11): {ID: uid.N(11), Name: "Pending", Status: domain.ShopStatusPending, CreatorID: strPtr(alice.ID)},
		uid.N(12): {ID: uid.N(12), Name: "Rejected", Status: domain.ShopStatusRejected, CreatorID: strPtr(alice.ID)},
	}
	reviews := newReviews(&fakeReviewQuery{
		getReview: func(_ context.Context, id string) (domain.ReviewDetail, error) {
			return domain.ReviewDetail{Review: domain.Review{ID: id, AuthorID: bob.ID}}, nil
		},
		listReviewShops: func(_ context.Context, reviewID string) ([]domain.Shop, error) {
			return []domain.Shop{shopsByID[reviewID]}, nil // テストでは、review の id に、属する shop の id を使う
		},
	}, &fakeReviewRepo{}, &fakePhotoStorage{})
	shops := usecase.NewShops(&fakeShopQuery{
		getShopWithCreator: func(_ context.Context, id string) (domain.ShopDetail, error) {
			return domain.ShopDetail{Shop: shopsByID[id]}, nil
		},
		listShopReviews: func(context.Context, string) ([]domain.ShopReview, error) { return nil, nil },
	}, domain.NewShops(&fakeShopRepo{}), stubPhotoURLs{})

	for _, tt := range []struct {
		name     string
		viewer   *domain.User
		shopID   string
		wantShop string // 見えないショップは ""(nil)
		want     bool
	}{
		{"匿名は、active なショップが見えるが、false", nil, uid.N(10), "Active", false},
		{"ログイン済みの利用者は、active なショップで true", &bob, uid.N(10), "Active", true},
		{"作成者は、自分の承認待ちのショップで true", &alice, uid.N(11), "Pending", true},
		{"作成者以外は、承認待ちのショップが見えず、false", &bob, uid.N(11), "", false},
		{"管理者は、承認待ちのショップで true", &admin, uid.N(11), "Pending", true},
		{"作成者は、却下されたショップが見えるが、false", &alice, uid.N(12), "Rejected", false},
		{"管理者も、却下されたショップが見えるが、false", &admin, uid.N(12), "Rejected", false},
		{"作成者以外は、却下されたショップが見えず、false", &bob, uid.N(12), "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			review, err := reviews.Get(context.Background(), tt.viewer, tt.shopID)
			if err != nil {
				t.Fatalf("Get returned error: %v", err)
			}
			if review.CanReview != tt.want {
				t.Errorf("レビュー詳細の CanReview = %v, want %v", review.CanReview, tt.want)
			}
			if tt.wantShop == "" && review.Shop != nil {
				t.Errorf("見えないショップが返った: %+v", review.Shop)
			}
			if tt.wantShop != "" && (review.Shop == nil || review.Shop.ID != tt.shopID || review.Shop.Name != tt.wantShop) {
				t.Errorf("Shop = %+v, want id %s name %s", review.Shop, tt.shopID, tt.wantShop)
			}
			shop, err := shops.Get(context.Background(), tt.viewer, tt.shopID)
			switch {
			case errors.Is(err, domain.ErrShopNotFound):
				// 閲覧者に見えないショップ(他人の承認待ち・却下)は、詳細が返らない = レビュー詳細にも出ず、レビューもできない
				if review.Shop != nil || review.CanReview {
					t.Errorf("ショップ詳細が見えないのに、レビュー詳細に Shop = %+v・CanReview = %v が出た", review.Shop, review.CanReview)
				}
			case err != nil:
				t.Fatalf("shops.Get returned error: %v", err)
			case shop.CanReview != review.CanReview:
				t.Errorf("ショップ詳細の CanReview = %v, レビュー詳細と食い違う(%v)", shop.CanReview, review.CanReview)
			}
		})
	}

	t.Run("バーガーが複数のショップにあるときは、古い方が却下されていても、書ける新しいショップを返す(ショップ詳細と食い違わない)", func(t *testing.T) {
		multi := newReviews(&fakeReviewQuery{
			getReview: func(context.Context, string) (domain.ReviewDetail, error) { return domain.ReviewDetail{}, nil },
			listReviewShops: func(context.Context, string) ([]domain.Shop, error) {
				return []domain.Shop{shopsByID[uid.N(12)], shopsByID[uid.N(10)]}, nil // 古い却下 → 新しい承認済み
			},
		}, &fakeReviewRepo{}, &fakePhotoStorage{})
		review, err := multi.Get(context.Background(), &bob, uid.N(1))
		if err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
		if review.Shop == nil || review.Shop.ID != uid.N(10) || !review.CanReview {
			t.Errorf("Shop = %+v, CanReview = %v, want 承認済みのショップと true", review.Shop, review.CanReview)
		}
	})

	t.Run("ショップに紐づかないバーガーのレビューでも、詳細は成功し、Shop は nil・CanReview は false になる", func(t *testing.T) {
		orphan := newReviews(&fakeReviewQuery{
			getReview:       func(context.Context, string) (domain.ReviewDetail, error) { return domain.ReviewDetail{}, nil },
			listReviewShops: func(context.Context, string) ([]domain.Shop, error) { return nil, nil },
		}, &fakeReviewRepo{}, &fakePhotoStorage{})
		review, err := orphan.Get(context.Background(), &bob, uid.N(1))
		if err != nil {
			t.Fatalf("Get returned error: %v(GetReview が成功したレビューの詳細は、成功のまま)", err)
		}
		if review.Shop != nil || review.CanReview {
			t.Errorf("Shop = %+v, CanReview = %v, want nil と false", review.Shop, review.CanReview)
		}
	})

	t.Run("レビューの属するショップが取れないときは、エラーを返す", func(t *testing.T) {
		boom := errors.New("boom")
		failing := newReviews(&fakeReviewQuery{
			getReview:       func(context.Context, string) (domain.ReviewDetail, error) { return domain.ReviewDetail{}, nil },
			listReviewShops: func(context.Context, string) ([]domain.Shop, error) { return nil, boom },
		}, &fakeReviewRepo{}, &fakePhotoStorage{})
		if _, err := failing.Get(context.Background(), &bob, uid.N(1)); !errors.Is(err, boom) {
			t.Errorf("err = %v, want boom", err)
		}
	})
}
