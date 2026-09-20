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

// fakeShopRepo は、手書きの usecase.ShopRepository の test double である。
// 未設定の振る舞いは panic するので、想定外の呼び出しに対してテストは
// fail-loud する。
type fakeShopRepo struct {
	listShops              func(ctx context.Context, vis domain.ShopVisibility, keyword string, limit, offset int32) ([]domain.Shop, error)
	getShopWithCreator     func(ctx context.Context, id int64) (domain.ShopDetail, error)
	listShopReviews        func(ctx context.Context, shopID int64) ([]domain.ShopReview, error)
	createShop             func(ctx context.Context, shop domain.Shop) (domain.Shop, error)
	listShopsForModeration func(ctx context.Context, status *domain.ShopStatus) ([]domain.ShopDetail, error)
	updateShopName         func(ctx context.Context, id int64, name string) (domain.Shop, error)
	updateShopStatus       func(ctx context.Context, id int64, status domain.ShopStatus, note *string) (domain.Shop, error)
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

func (f *fakeShopRepo) UpdateShopName(ctx context.Context, id int64, name string) (domain.Shop, error) {
	if f.updateShopName == nil {
		panic("unexpected UpdateShopName call")
	}
	return f.updateShopName(ctx, id, name)
}

func (f *fakeShopRepo) UpdateShopStatus(ctx context.Context, id int64, status domain.ShopStatus, note *string) (domain.Shop, error) {
	if f.updateShopStatus == nil {
		panic("unexpected UpdateShopStatus call")
	}
	return f.updateShopStatus(ctx, id, status, note)
}

func int64Ptr(v int64) *int64 { return &v }

// TestShopsListPagination は、フォールバック規則を固定する。page の
// デフォルトは 1、per_page のデフォルトは 20、per_page は 100 が上限で、
// いずれもエラーにならない。
func TestShopsListPagination(t *testing.T) {
	tests := []struct {
		name          string
		page, perPage int
		wantLimit     int32
		wantOffset    int32
	}{
		{name: "page と per_page が 0 のときはデフォルト値になる", page: 0, perPage: 0, wantLimit: 20, wantOffset: 0},
		{name: "負の値はデフォルト値にフォールバックする", page: -3, perPage: -1, wantLimit: 20, wantOffset: 0},
		{name: "page と per_page を明示するとその値が使われる", page: 3, perPage: 5, wantLimit: 5, wantOffset: 10},
		{name: "100 を超える per_page は 100 に clamp される", page: 1, perPage: 101, wantLimit: 100, wantOffset: 0},
		{name: "巨大な page はオーバーフローせず offset を clamp する", page: 1 << 40, perPage: 100, wantLimit: 100, wantOffset: 1<<31 - 1},
		{name: "page が MaxInt64 で per_page がデフォルトのとき offset を clamp する", page: math.MaxInt64, perPage: 0, wantLimit: 20, wantOffset: math.MaxInt32},
		{name: "page が MaxInt64 で per_page が明示指定のとき offset を clamp する", page: math.MaxInt64, perPage: 20, wantLimit: 20, wantOffset: math.MaxInt32},
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

// TestShopsListVisibilityDescriptor は、List が viewer から記述子を導出し、
// それを変更せずに repository へ渡すことをアサートする。
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

// TestShopsGet は詳細の use case を扱う。見える shop は review つきの完全な
// detail を返し、隠された shop と未知の id はどちらも
// domain.ErrShopNotFound を返す（usecase レベルでの AC4）。
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

	t.Run("creator は自分の pending な shop を review つきで見られる", func(t *testing.T) {
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

	t.Run("admin は pending な shop を見られる", func(t *testing.T) {
		if _, err := shops.Get(context.Background(), &admin, pending.ID); err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
	})

	t.Run("匿名の viewer には pending な shop は ErrShopNotFound になる", func(t *testing.T) {
		if _, err := shops.Get(context.Background(), nil, pending.ID); !errors.Is(err, domain.ErrShopNotFound) {
			t.Fatalf("Get error = %v, want %v", err, domain.ErrShopNotFound)
		}
	})

	t.Run("未知の id は ErrShopNotFound を返す", func(t *testing.T) {
		if _, err := shops.Get(context.Background(), &admin, 999); !errors.Is(err, domain.ErrShopNotFound) {
			t.Fatalf("Get error = %v, want %v", err, domain.ErrShopNotFound)
		}
	})

	t.Run("review の query が失敗したらそのエラーが伝播する", func(t *testing.T) {
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

// TestShopsCreate は投稿の use case を扱う。有効な name は viewer を creator と
// する pending の shop を返し、空白の name は repository に触れずに validation
// に失敗する（fake は panic する）。
func TestShopsCreate(t *testing.T) {
	alice := domain.User{ID: 1, Username: "alice"}

	t.Run("viewer を creator とする pending の shop を作成する", func(t *testing.T) {
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

	t.Run("空白の name は repository を呼ばずに ValidationError を返す", func(t *testing.T) {
		_, err := usecase.NewShops(&fakeShopRepo{}).Create(context.Background(), alice, "   ")
		var vErr *domain.ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("error = %v, want *domain.ValidationError", err)
		}
	})

	t.Run("repository の失敗は伝播する", func(t *testing.T) {
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

// TestShopsAdminForbidden は認可の境界を固定する。admin のすべての操作は、
// admin でない viewer に対して、repository へのアクセスの前に
// domain.ErrForbidden を返す（ゼロ値の fake はどの呼び出しでも panic する）。
func TestShopsAdminForbidden(t *testing.T) {
	alice := domain.User{ID: 1, Username: "alice"} // 認証済みだが admin ではない
	shops := usecase.NewShops(&fakeShopRepo{})
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{name: "AdminList は admin でない viewer に ErrForbidden を返す", call: func() error { _, err := shops.AdminList(ctx, alice, ""); return err }},
		{name: "AdminUpdateName は admin でない viewer に ErrForbidden を返す", call: func() error { _, err := shops.AdminUpdateName(ctx, alice, 1, "x"); return err }},
		{name: "Approve は admin でない viewer に ErrForbidden を返す", call: func() error { _, err := shops.Approve(ctx, alice, 1); return err }},
		{name: "Reject は admin でない viewer に ErrForbidden を返す", call: func() error { _, err := shops.Reject(ctx, alice, 1, nil); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("%s error = %v, want %v", tt.name, err, domain.ErrForbidden)
			}
		})
	}
}

// TestShopsAdminList は moderation の一覧を扱う。既知の status は repository の
// フィルタになり、status なしはすべてを意味し、未知の status は repository を
// 呼ばずに空の結果へ short-circuit する。
func TestShopsAdminList(t *testing.T) {
	admin := domain.User{ID: 2, Admin: true}
	ctx := context.Background()

	t.Run("status のフィルタはそのまま repository に渡される", func(t *testing.T) {
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

	t.Run("status なしはフィルタなしを意味する", func(t *testing.T) {
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

	t.Run("未知の status は repository を呼ばずに空の一覧を返す", func(t *testing.T) {
		got, err := usecase.NewShops(&fakeShopRepo{}).AdminList(ctx, admin, "bogus")
		if err != nil {
			t.Fatalf("AdminList returned error: %v", err)
		}
		if got == nil || len(got) != 0 {
			t.Errorf("AdminList = %v, want empty non-nil slice", got)
		}
	})
}

// TestShopsModeration は admin の書き込み操作を扱う。それぞれ自分のカラム
// だけを永続化し（遷移では status/note、rename では name。fake はそれ以外の
// 書き込みで panic する）、未知の id は domain.ErrShopNotFound を返す。
func TestShopsModeration(t *testing.T) {
	admin := domain.User{ID: 2, Admin: true}
	ctx := context.Background()
	rejected := domain.ShopDetail{
		Shop:    domain.Shop{ID: 10, Name: "Shack", Status: domain.ShopStatusRejected, ModerationNote: strPtr("old note"), CreatorID: int64Ptr(1)},
		Creator: &domain.UserRef{ID: 1, Username: "alice"},
	}
	getRejected := func(_ context.Context, id int64) (domain.ShopDetail, error) {
		if id == rejected.ID {
			return rejected, nil
		}
		return domain.ShopDetail{}, domain.ErrShopNotFound
	}
	// statusWrite は 1 回の UpdateShopStatus の呼び出しの引数を記録する。
	type statusWrite struct {
		id     int64
		status domain.ShopStatus
		note   *string
	}
	// statusRepoFor は rejected の shop だけを返し、カラム限定の status の
	// 書き込みを記録する。updateShopName は未設定のままなので、name に触れる
	// status 遷移があればテストが panic する。
	statusRepoFor := func(got *statusWrite) *fakeShopRepo {
		return &fakeShopRepo{
			getShopWithCreator: getRejected,
			updateShopStatus: func(_ context.Context, id int64, status domain.ShopStatus, note *string) (domain.Shop, error) {
				*got = statusWrite{id: id, status: status, note: note}
				stored := rejected.Shop
				stored.Status = status
				stored.ModerationNote = note
				return stored, nil
			},
		}
	}

	t.Run("Approve は status と note だけを永続化する (rejected から active)", func(t *testing.T) {
		var write statusWrite
		got, err := usecase.NewShops(statusRepoFor(&write)).Approve(ctx, admin, rejected.ID)
		if err != nil {
			t.Fatalf("Approve returned error: %v", err)
		}
		if write.id != rejected.ID || write.status != domain.ShopStatusActive || write.note != nil {
			t.Errorf("status write = %+v, want id %d, active, nil note", write, rejected.ID)
		}
		if got.Status != domain.ShopStatusActive || got.Name != rejected.Name || !reflect.DeepEqual(got.Creator, rejected.Creator) {
			t.Errorf("detail = %+v, want active shop with unchanged name and creator", got)
		}
	})

	t.Run("Reject は status と note だけを永続化する", func(t *testing.T) {
		var write statusWrite
		note := strPtr("needs fixes")
		got, err := usecase.NewShops(statusRepoFor(&write)).Reject(ctx, admin, rejected.ID, note)
		if err != nil {
			t.Fatalf("Reject returned error: %v", err)
		}
		if write.id != rejected.ID || write.status != domain.ShopStatusRejected || write.note != note {
			t.Errorf("status write = %+v, want id %d, rejected, the note", write, rejected.ID)
		}
		if got.ModerationNote != note {
			t.Errorf("detail note = %v, want %v", got.ModerationNote, note)
		}
	})

	t.Run("AdminUpdateName は name だけを永続化する", func(t *testing.T) {
		var gotID int64
		var gotName string
		// updateShopStatus は未設定のままなので、status/note に触れる rename が
		// あればテストが panic する。
		repo := &fakeShopRepo{
			getShopWithCreator: getRejected,
			updateShopName: func(_ context.Context, id int64, name string) (domain.Shop, error) {
				gotID, gotName = id, name
				stored := rejected.Shop
				stored.Name = name
				return stored, nil
			},
		}
		got, err := usecase.NewShops(repo).AdminUpdateName(ctx, admin, rejected.ID, "Renamed")
		if err != nil {
			t.Fatalf("AdminUpdateName returned error: %v", err)
		}
		if gotID != rejected.ID || gotName != "Renamed" {
			t.Errorf("name write = (%d, %q), want (%d, Renamed)", gotID, gotName, rejected.ID)
		}
		if got.Name != "Renamed" || got.Status != rejected.Status {
			t.Errorf("detail = %+v, want renamed with status unchanged", got)
		}
	})

	t.Run("AdminUpdateName は lookup の前に空白の name を拒否する", func(t *testing.T) {
		var err error
		_, err = usecase.NewShops(&fakeShopRepo{}).AdminUpdateName(ctx, admin, rejected.ID, " ")
		var vErr *domain.ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("error = %v, want *domain.ValidationError", err)
		}
	})

	t.Run("未知の id は書き込みせずに ErrShopNotFound を返す", func(t *testing.T) {
		// 2 つの書き込みの振る舞いはどちらも未設定のままなので、lookup が
		// 失敗した後の書き込みはどれもテストを panic させる。
		shops := usecase.NewShops(&fakeShopRepo{getShopWithCreator: getRejected})
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
