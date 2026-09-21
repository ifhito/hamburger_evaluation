package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// stubPhotoURLs は、写真のキーに固定の接頭辞をつけた URL を返す(写真の保存先の代役)。
type stubPhotoURLs struct{}

func (stubPhotoURLs) URL(key string) string { return "https://photos.test/" + key }

func floatPtr(f float64) *float64 { return &f }

func TestShopsListSummaries(t *testing.T) {
	shopA := domain.Shop{ID: uid.N(1), Name: "A", Status: domain.ShopStatusActive}
	shopB := domain.Shop{ID: uid.N(2), Name: "B", Status: domain.ShopStatusActive}
	summaryCalls := 0
	var gotIDs []string
	query := &fakeShopQuery{
		listShops: func(context.Context, domain.ShopVisibility, string, int32, int32) ([]domain.Shop, bool, error) {
			return []domain.Shop{shopA, shopB}, false, nil
		},
		listShopSummaries: func(_ context.Context, ids []string) (map[string]domain.ShopSummary, error) {
			summaryCalls++
			gotIDs = ids
			return map[string]domain.ShopSummary{
				shopA.ID: domain.NewShopSummary(3, floatPtr(4.0), strPtr("reviews/a.jpg")),
			}, nil
		},
	}
	shops := usecase.NewShops(query, domain.NewShops(&fakeShopRepo{}), usecase.WithPhotoURLs(stubPhotoURLs{}))

	t.Run("集計は、一覧の全ショップ分を 1 回の問い合わせで取り、写真のキーは公開 URL に直して添える", func(t *testing.T) {
		list, _, err := shops.List(context.Background(), nil, "", 1, 20)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if summaryCalls != 1 || len(gotIDs) != 2 || gotIDs[0] != shopA.ID || gotIDs[1] != shopB.ID {
			t.Errorf("集計の問い合わせ = %d 回 %v, want 1 回で [%s %s]", summaryCalls, gotIDs, shopA.ID, shopB.ID)
		}
		a := list[0].Summary
		if a.ReviewCount != 3 || a.AverageRating == nil || *a.AverageRating != 4 || a.PhotoURL == nil || *a.PhotoURL != "https://photos.test/reviews/a.jpg" {
			t.Errorf("A の集計 = %+v", a)
		}
	})

	t.Run("レビューのないショップは、件数 0・平均と写真なしの空の集計になる", func(t *testing.T) {
		list, _, err := shops.List(context.Background(), nil, "", 1, 20)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		b := list[1].Summary
		if b.ReviewCount != 0 || b.AverageRating != nil || b.PhotoURL != nil {
			t.Errorf("B の集計 = %+v, want 空", b)
		}
	})

	t.Run("集計の取得に失敗したら、一覧もエラーになる", func(t *testing.T) {
		boom := errors.New("boom")
		failing := &fakeShopQuery{
			listShops: query.listShops,
			listShopSummaries: func(context.Context, []string) (map[string]domain.ShopSummary, error) {
				return nil, boom
			},
		}
		_, _, err := usecase.NewShops(failing, domain.NewShops(&fakeShopRepo{})).List(context.Background(), nil, "", 1, 20)
		if !errors.Is(err, boom) {
			t.Errorf("err = %v, want boom", err)
		}
	})

	t.Run("写真の保存先を渡していないときは、写真のキーがあっても URL は nil になる", func(t *testing.T) {
		list, _, err := usecase.NewShops(query, domain.NewShops(&fakeShopRepo{})).List(context.Background(), nil, "", 1, 20)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if list[0].Summary.PhotoURL != nil {
			t.Errorf("PhotoURL = %v, want nil", *list[0].Summary.PhotoURL)
		}
	})
}

func TestShopsGetSummary(t *testing.T) {
	shop := domain.ShopDetail{Shop: domain.Shop{ID: uid.N(1), Name: "A", Status: domain.ShopStatusActive}}
	query := &fakeShopQuery{
		getShopWithCreator: func(context.Context, string) (domain.ShopDetail, error) { return shop, nil },
		listShopReviews:    func(context.Context, string) ([]domain.ShopReview, error) { return nil, nil },
		listShopSummaries: func(_ context.Context, ids []string) (map[string]domain.ShopSummary, error) {
			return map[string]domain.ShopSummary{ids[0]: domain.NewShopSummary(2, floatPtr(3.5), strPtr("reviews/x.jpg"))}, nil
		},
	}
	got, err := usecase.NewShops(query, domain.NewShops(&fakeShopRepo{}), usecase.WithPhotoURLs(stubPhotoURLs{})).
		Get(context.Background(), nil, shop.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.Summary.ReviewCount != 2 || *got.Summary.AverageRating != 3.5 || *got.Summary.PhotoURL != "https://photos.test/reviews/x.jpg" {
		t.Errorf("Summary = %+v", got.Summary)
	}
}
