package usecase_test

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// fakeShopRepo is a hand-written usecase.ShopRepository test double.
// Unset behaviors panic so tests fail loudly on unexpected calls.
type fakeShopRepo struct {
	listShops          func(ctx context.Context, vis domain.ShopVisibility, keyword string, limit, offset int32) ([]domain.Shop, error)
	getShopWithCreator func(ctx context.Context, id int64) (domain.ShopDetail, error)
	listShopReviews    func(ctx context.Context, shopID int64) ([]domain.ShopReview, error)
}

func (f *fakeShopRepo) ListShops(ctx context.Context, vis domain.ShopVisibility, keyword string, limit, offset int32) ([]domain.Shop, error) {
	if f.listShops == nil {
		panic("unexpected ListShops call")
	}
	return f.listShops(ctx, vis, keyword, limit, offset)
}

func (f *fakeShopRepo) GetShopWithCreator(ctx context.Context, id int64) (domain.ShopDetail, error) {
	if f.getShopWithCreator == nil {
		panic("unexpected GetShopWithCreator call")
	}
	return f.getShopWithCreator(ctx, id)
}

func (f *fakeShopRepo) ListShopReviews(ctx context.Context, shopID int64) ([]domain.ShopReview, error) {
	if f.listShopReviews == nil {
		panic("unexpected ListShopReviews call")
	}
	return f.listShopReviews(ctx, shopID)
}

func int64Ptr(v int64) *int64 { return &v }

// TestShopsListPagination pins the fallback rules: page defaults to 1,
// per_page to 20, per_page is capped at 100, and none of them error.
func TestShopsListPagination(t *testing.T) {
	tests := []struct {
		name          string
		page, perPage int
		wantLimit     int32
		wantOffset    int32
	}{
		{name: "defaults", page: 0, perPage: 0, wantLimit: 20, wantOffset: 0},
		{name: "negative values fall back", page: -3, perPage: -1, wantLimit: 20, wantOffset: 0},
		{name: "explicit page and per_page", page: 3, perPage: 5, wantLimit: 5, wantOffset: 10},
		{name: "per_page above 100 is clamped", page: 1, perPage: 101, wantLimit: 100, wantOffset: 0},
		{name: "huge page clamps offset instead of overflowing", page: 1 << 40, perPage: 100, wantLimit: 100, wantOffset: 1<<31 - 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotLimit, gotOffset int32
			repo := &fakeShopRepo{
				listShops: func(_ context.Context, _ domain.ShopVisibility, _ string, limit, offset int32) ([]domain.Shop, error) {
					gotLimit, gotOffset = limit, offset
					return []domain.Shop{}, nil
				},
			}
			if _, err := usecase.NewShops(repo).List(context.Background(), nil, "", tt.page, tt.perPage); err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			if gotLimit != tt.wantLimit || gotOffset != tt.wantOffset {
				t.Errorf("limit/offset = %d/%d, want %d/%d", gotLimit, gotOffset, tt.wantLimit, tt.wantOffset)
			}
		})
	}
}

// TestShopsListVisibilityDescriptor asserts List derives the descriptor
// from the viewer and hands it to the repository unchanged.
func TestShopsListVisibilityDescriptor(t *testing.T) {
	admin := domain.User{ID: 5, Admin: true}
	var got domain.ShopVisibility
	repo := &fakeShopRepo{
		listShops: func(_ context.Context, vis domain.ShopVisibility, _ string, _, _ int32) ([]domain.Shop, error) {
			got = vis
			return nil, nil
		},
	}
	if _, err := usecase.NewShops(repo).List(context.Background(), &admin, "burger", 1, 20); err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if !got.ViewAll || got.ViewerID != nil {
		t.Errorf("descriptor = %+v, want ViewAll for admin", got)
	}
}

// TestShopsGet covers the detail use case: visible shops return the full
// detail with reviews; hidden shops and unknown ids both surface
// domain.ErrShopNotFound (AC4 at the usecase level).
func TestShopsGet(t *testing.T) {
	alice := domain.User{ID: 1, Username: "alice"}
	admin := domain.User{ID: 2, Admin: true}
	pending := domain.ShopDetail{
		Shop:    domain.Shop{ID: 10, Name: "Pending Shack", Status: domain.ShopStatusPending, CreatorID: int64Ptr(alice.ID)},
		Creator: &domain.UserRef{ID: alice.ID, Username: "alice"},
	}
	reviews := []domain.ShopReview{{ID: 3, Rating: 4, CreatedAt: time.Now()}}

	repo := &fakeShopRepo{
		getShopWithCreator: func(_ context.Context, id int64) (domain.ShopDetail, error) {
			if id == pending.ID {
				return pending, nil
			}
			return domain.ShopDetail{}, domain.ErrShopNotFound
		},
		listShopReviews: func(_ context.Context, shopID int64) ([]domain.ShopReview, error) {
			return reviews, nil
		},
	}
	shops := usecase.NewShops(repo)

	t.Run("creator sees own pending shop with reviews", func(t *testing.T) {
		got, err := shops.Get(context.Background(), &alice, pending.ID)
		if err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
		want := pending
		want.Reviews = reviews
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Get = %+v, want %+v", got, want)
		}
	})

	t.Run("admin sees pending shop", func(t *testing.T) {
		if _, err := shops.Get(context.Background(), &admin, pending.ID); err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
	})

	t.Run("anonymous viewer gets ErrShopNotFound for pending shop", func(t *testing.T) {
		if _, err := shops.Get(context.Background(), nil, pending.ID); !errors.Is(err, domain.ErrShopNotFound) {
			t.Fatalf("Get error = %v, want %v", err, domain.ErrShopNotFound)
		}
	})

	t.Run("unknown id yields ErrShopNotFound", func(t *testing.T) {
		if _, err := shops.Get(context.Background(), &admin, 999); !errors.Is(err, domain.ErrShopNotFound) {
			t.Fatalf("Get error = %v, want %v", err, domain.ErrShopNotFound)
		}
	})

	t.Run("review query failure propagates", func(t *testing.T) {
		failing := &fakeShopRepo{
			getShopWithCreator: repo.getShopWithCreator,
			listShopReviews: func(_ context.Context, _ int64) ([]domain.ShopReview, error) {
				return nil, io.ErrUnexpectedEOF
			},
		}
		if _, err := usecase.NewShops(failing).Get(context.Background(), &alice, pending.ID); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("Get error = %v, want %v", err, io.ErrUnexpectedEOF)
		}
	})
}
