package usecase_test

import (
	"context"
	"errors"
	"io"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// fakeShopRepo is a hand-written usecase.ShopRepository test double.
// Unset behaviors panic so tests fail loudly on unexpected calls.
type fakeShopRepo struct {
	listShops              func(ctx context.Context, vis domain.ShopVisibility, keyword string, limit, offset int32) ([]domain.Shop, error)
	getShopWithCreator     func(ctx context.Context, id int64) (domain.ShopDetail, error)
	listShopReviews        func(ctx context.Context, shopID int64) ([]domain.ShopReview, error)
	createShop             func(ctx context.Context, shop domain.Shop) (domain.Shop, error)
	listShopsForModeration func(ctx context.Context, status *domain.ShopStatus) ([]domain.ShopDetail, error)
	updateShop             func(ctx context.Context, shop domain.Shop) (domain.Shop, error)
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

func (f *fakeShopRepo) CreateShop(ctx context.Context, shop domain.Shop) (domain.Shop, error) {
	if f.createShop == nil {
		panic("unexpected CreateShop call")
	}
	return f.createShop(ctx, shop)
}

func (f *fakeShopRepo) ListShopsForModeration(ctx context.Context, status *domain.ShopStatus) ([]domain.ShopDetail, error) {
	if f.listShopsForModeration == nil {
		panic("unexpected ListShopsForModeration call")
	}
	return f.listShopsForModeration(ctx, status)
}

func (f *fakeShopRepo) UpdateShop(ctx context.Context, shop domain.Shop) (domain.Shop, error) {
	if f.updateShop == nil {
		panic("unexpected UpdateShop call")
	}
	return f.updateShop(ctx, shop)
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
		{name: "page MaxInt64 with default per_page clamps offset", page: math.MaxInt64, perPage: 0, wantLimit: 20, wantOffset: math.MaxInt32},
		{name: "page MaxInt64 with explicit per_page clamps offset", page: math.MaxInt64, perPage: 20, wantLimit: 20, wantOffset: math.MaxInt32},
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

// TestShopsCreate covers the submission use case: a valid name yields a
// pending shop with the viewer as creator; a blank name fails validation
// without touching the repository (the fake would panic).
func TestShopsCreate(t *testing.T) {
	alice := domain.User{ID: 1, Username: "alice"}

	t.Run("creates pending shop with viewer as creator", func(t *testing.T) {
		repo := &fakeShopRepo{
			createShop: func(_ context.Context, shop domain.Shop) (domain.Shop, error) {
				shop.ID = 42
				return shop, nil
			},
		}
		got, err := usecase.NewShops(repo).Create(context.Background(), alice, "New Shack")
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		want := domain.ShopDetail{
			Shop: domain.Shop{
				ID: 42, Name: "New Shack", Status: domain.ShopStatusPending, CreatorID: int64Ptr(alice.ID),
			},
			Creator: &domain.UserRef{ID: alice.ID, Username: "alice"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Create = %+v, want %+v", got, want)
		}
	})

	t.Run("blank name yields ValidationError without repository call", func(t *testing.T) {
		_, err := usecase.NewShops(&fakeShopRepo{}).Create(context.Background(), alice, "   ")
		var vErr *domain.ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("error = %v, want *domain.ValidationError", err)
		}
	})

	t.Run("repository failure propagates", func(t *testing.T) {
		repo := &fakeShopRepo{
			createShop: func(_ context.Context, _ domain.Shop) (domain.Shop, error) {
				return domain.Shop{}, io.ErrUnexpectedEOF
			},
		}
		if _, err := usecase.NewShops(repo).Create(context.Background(), alice, "New Shack"); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("Create error = %v, want %v", err, io.ErrUnexpectedEOF)
		}
	})
}

// TestShopsAdminForbidden pins the authorization boundary: every admin
// operation returns domain.ErrForbidden for non-admin viewers before any
// repository access (the zero fake panics on any call).
func TestShopsAdminForbidden(t *testing.T) {
	alice := domain.User{ID: 1, Username: "alice"} // authenticated, not admin
	shops := usecase.NewShops(&fakeShopRepo{})
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{name: "AdminList", call: func() error { _, err := shops.AdminList(ctx, alice, ""); return err }},
		{name: "AdminUpdateName", call: func() error { _, err := shops.AdminUpdateName(ctx, alice, 1, "x"); return err }},
		{name: "Approve", call: func() error { _, err := shops.Approve(ctx, alice, 1); return err }},
		{name: "Reject", call: func() error { _, err := shops.Reject(ctx, alice, 1, nil); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("%s error = %v, want %v", tt.name, err, domain.ErrForbidden)
			}
		})
	}
}

// TestShopsAdminList covers the moderation list: known statuses become
// the repository filter, no status means all, and an unknown status
// short-circuits to an empty result without a repository call.
func TestShopsAdminList(t *testing.T) {
	admin := domain.User{ID: 2, Admin: true}
	ctx := context.Background()

	t.Run("status filter is passed through", func(t *testing.T) {
		var got *domain.ShopStatus
		repo := &fakeShopRepo{
			listShopsForModeration: func(_ context.Context, status *domain.ShopStatus) ([]domain.ShopDetail, error) {
				got = status
				return []domain.ShopDetail{}, nil
			},
		}
		if _, err := usecase.NewShops(repo).AdminList(ctx, admin, "pending"); err != nil {
			t.Fatalf("AdminList returned error: %v", err)
		}
		if got == nil || *got != domain.ShopStatusPending {
			t.Errorf("filter = %v, want pending", got)
		}
	})

	t.Run("absent status means no filter", func(t *testing.T) {
		called := false
		repo := &fakeShopRepo{
			listShopsForModeration: func(_ context.Context, status *domain.ShopStatus) ([]domain.ShopDetail, error) {
				called = true
				if status != nil {
					t.Errorf("filter = %v, want nil", *status)
				}
				return []domain.ShopDetail{}, nil
			},
		}
		if _, err := usecase.NewShops(repo).AdminList(ctx, admin, ""); err != nil {
			t.Fatalf("AdminList returned error: %v", err)
		}
		if !called {
			t.Error("repository was not called")
		}
	})

	t.Run("unknown status yields empty list without repository call", func(t *testing.T) {
		got, err := usecase.NewShops(&fakeShopRepo{}).AdminList(ctx, admin, "bogus")
		if err != nil {
			t.Fatalf("AdminList returned error: %v", err)
		}
		if got == nil || len(got) != 0 {
			t.Errorf("AdminList = %v, want empty non-nil slice", got)
		}
	})
}

// TestShopsModeration covers the admin read-modify-write operations:
// transitions and renames reach UpdateShop with the domain state applied,
// and unknown ids surface domain.ErrShopNotFound.
func TestShopsModeration(t *testing.T) {
	admin := domain.User{ID: 2, Admin: true}
	ctx := context.Background()
	rejected := domain.ShopDetail{
		Shop:    domain.Shop{ID: 10, Name: "Shack", Status: domain.ShopStatusRejected, ModerationNote: strPtr("old note"), CreatorID: int64Ptr(1)},
		Creator: &domain.UserRef{ID: 1, Username: "alice"},
	}
	// repoFor returns a fake serving only the rejected shop and recording
	// what UpdateShop receives.
	repoFor := func(updated *domain.Shop) *fakeShopRepo {
		return &fakeShopRepo{
			getShopWithCreator: func(_ context.Context, id int64) (domain.ShopDetail, error) {
				if id == rejected.ID {
					return rejected, nil
				}
				return domain.ShopDetail{}, domain.ErrShopNotFound
			},
			updateShop: func(_ context.Context, shop domain.Shop) (domain.Shop, error) {
				*updated = shop
				return shop, nil
			},
		}
	}

	t.Run("Approve activates and clears note (rejected to active)", func(t *testing.T) {
		var updated domain.Shop
		got, err := usecase.NewShops(repoFor(&updated)).Approve(ctx, admin, rejected.ID)
		if err != nil {
			t.Fatalf("Approve returned error: %v", err)
		}
		if updated.Status != domain.ShopStatusActive || updated.ModerationNote != nil {
			t.Errorf("persisted shop = %+v, want active with nil note", updated)
		}
		if got.Status != domain.ShopStatusActive || !reflect.DeepEqual(got.Creator, rejected.Creator) {
			t.Errorf("detail = %+v, want active shop with creator", got)
		}
	})

	t.Run("Reject sets status and note", func(t *testing.T) {
		var updated domain.Shop
		note := strPtr("needs fixes")
		got, err := usecase.NewShops(repoFor(&updated)).Reject(ctx, admin, rejected.ID, note)
		if err != nil {
			t.Fatalf("Reject returned error: %v", err)
		}
		if updated.Status != domain.ShopStatusRejected || updated.ModerationNote != note {
			t.Errorf("persisted shop = %+v, want rejected with the note", updated)
		}
		if got.ModerationNote != note {
			t.Errorf("detail note = %v, want %v", got.ModerationNote, note)
		}
	})

	t.Run("AdminUpdateName renames only", func(t *testing.T) {
		var updated domain.Shop
		got, err := usecase.NewShops(repoFor(&updated)).AdminUpdateName(ctx, admin, rejected.ID, "Renamed")
		if err != nil {
			t.Fatalf("AdminUpdateName returned error: %v", err)
		}
		if updated.Name != "Renamed" || updated.Status != rejected.Status {
			t.Errorf("persisted shop = %+v, want renamed with status unchanged", updated)
		}
		if got.Name != "Renamed" {
			t.Errorf("detail name = %q, want Renamed", got.Name)
		}
	})

	t.Run("AdminUpdateName rejects blank name before any lookup", func(t *testing.T) {
		var err error
		_, err = usecase.NewShops(&fakeShopRepo{}).AdminUpdateName(ctx, admin, rejected.ID, " ")
		var vErr *domain.ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("error = %v, want *domain.ValidationError", err)
		}
	})

	t.Run("unknown id yields ErrShopNotFound", func(t *testing.T) {
		var updated domain.Shop
		shops := usecase.NewShops(repoFor(&updated))
		for name, call := range map[string]func() error{
			"Approve":         func() error { _, err := shops.Approve(ctx, admin, 999); return err },
			"Reject":          func() error { _, err := shops.Reject(ctx, admin, 999, nil); return err },
			"AdminUpdateName": func() error { _, err := shops.AdminUpdateName(ctx, admin, 999, "x"); return err },
		} {
			if err := call(); !errors.Is(err, domain.ErrShopNotFound) {
				t.Errorf("%s error = %v, want %v", name, err, domain.ErrShopNotFound)
			}
		}
	})
}
