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

// TestShopsListSummaries は、一覧が、保存された集計(query が返す)を、写真のキーだけ公開 URL に直して添えることを確かめる。
func TestShopsListSummaries(t *testing.T) {
	shopA := domain.Shop{ID: uid.N(1), Name: "A", Status: domain.ShopStatusActive}
	shopB := domain.Shop{ID: uid.N(2), Name: "B", Status: domain.ShopStatusActive}
	listCalls := 0
	query := &fakeShopQuery{
		listShops: func(context.Context, domain.ShopVisibility, string, int32, int32) ([]domain.ShopListing, bool, error) {
			listCalls++
			return []domain.ShopListing{
				{Shop: shopA, Summary: domain.ShopSummary{ReviewCount: 3, AverageRating: floatPtr(4.0), PhotoKey: strPtr("reviews/a.jpg")}},
				{Shop: shopB}, // まだ集計されていない(空の集計)
			}, false, nil
		},
	}
	shops := usecase.NewShops(query, domain.NewShops(&fakeShopRepo{}), stubPhotoURLs{})

	t.Run("一覧は、query の 1 回の問い合わせで、ショップと集計を取り、写真のキーは公開 URL に直して添える", func(t *testing.T) {
		listCalls = 0
		list, _, err := shops.List(context.Background(), nil, "", 1, 20)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if listCalls != 1 {
			t.Errorf("query の問い合わせ = %d 回, want 1 回", listCalls)
		}
		a := list[0].Summary
		if a.ReviewCount != 3 || a.AverageRating == nil || *a.AverageRating != 4 || a.PhotoURL == nil || *a.PhotoURL != "https://photos.test/reviews/a.jpg" {
			t.Errorf("A の集計 = %+v", a)
		}
	})

	t.Run("まだ集計されていないショップは、件数 0・平均と写真なしの空の集計のままである", func(t *testing.T) {
		list, _, err := shops.List(context.Background(), nil, "", 1, 20)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		b := list[1].Summary
		if b.ReviewCount != 0 || b.AverageRating != nil || b.PhotoKey != nil || b.PhotoURL != nil {
			t.Errorf("B の集計 = %+v, want 空", b)
		}
	})

	t.Run("写真の保存先が nil のときは、渡し忘れが黙って null になるのではなく、配線の時点で panic する", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("NewShops(nil の PhotoURLs) が panic しなかった")
			}
		}()
		usecase.NewShops(query, domain.NewShops(&fakeShopRepo{}), nil)
	})
}

func TestShopsGetSummary(t *testing.T) {
	shop := domain.ShopDetail{
		Shop:    domain.Shop{ID: uid.N(1), Name: "A", Status: domain.ShopStatusActive},
		Summary: domain.ShopSummary{ReviewCount: 2, AverageRating: floatPtr(3.5), PhotoKey: strPtr("reviews/x.jpg")},
	}
	query := &fakeShopQuery{
		getShopWithCreator: func(context.Context, string) (domain.ShopDetail, error) { return shop, nil },
		listShopReviews:    func(context.Context, string) ([]domain.ShopReview, error) { return nil, nil },
	}
	got, err := usecase.NewShops(query, domain.NewShops(&fakeShopRepo{}), stubPhotoURLs{}).
		Get(context.Background(), nil, shop.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.Summary.ReviewCount != 2 || *got.Summary.AverageRating != 3.5 || *got.Summary.PhotoURL != "https://photos.test/reviews/x.jpg" {
		t.Errorf("Summary = %+v", got.Summary)
	}

	t.Run("詳細の取得に失敗したら、エラーになる", func(t *testing.T) {
		boom := errors.New("boom")
		failing := &fakeShopQuery{getShopWithCreator: func(context.Context, string) (domain.ShopDetail, error) { return domain.ShopDetail{}, boom }}
		if _, err := usecase.NewShops(failing, domain.NewShops(&fakeShopRepo{}), stubPhotoURLs{}).Get(context.Background(), nil, shop.ID); !errors.Is(err, boom) {
			t.Errorf("err = %v, want boom", err)
		}
	})
}
