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
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// fakeShopQuery は、手書きの usecase.ShopQuery の test double である。
// 未設定の振る舞いは panic するので、想定外の呼び出しに対してテストは
// fail-loud する。
type fakeShopQuery struct {
	listShops              func(ctx context.Context, vis domain.ShopVisibility, keyword string, limit, offset int32) ([]domain.Shop, bool, error)
	getShopWithCreator     func(ctx context.Context, id string) (domain.ShopDetail, error)
	listShopReviews        func(ctx context.Context, shopID string) ([]domain.ShopReview, error)
	listShopsForModeration func(ctx context.Context, status *domain.ShopStatus) ([]domain.ShopDetail, error)
}

func (f *fakeShopQuery) ListShops(ctx context.Context, vis domain.ShopVisibility, keyword string, limit, offset int32) ([]domain.Shop, bool, error) {
	if f.listShops == nil {
		panic("unexpected ListShops call")
	}
	return f.listShops(ctx, vis, keyword, limit, offset)
}

func (f *fakeShopQuery) GetShopWithCreator(ctx context.Context, id string) (domain.ShopDetail, error) {
	if f.getShopWithCreator == nil {
		panic("unexpected GetShopWithCreator call")
	}
	return f.getShopWithCreator(ctx, id)
}

func (f *fakeShopQuery) ListShopReviews(ctx context.Context, shopID string) ([]domain.ShopReview, error) {
	if f.listShopReviews == nil {
		panic("unexpected ListShopReviews call")
	}
	return f.listShopReviews(ctx, shopID)
}

func (f *fakeShopQuery) ListShopsForModeration(ctx context.Context, status *domain.ShopStatus) ([]domain.ShopDetail, error) {
	if f.listShopsForModeration == nil {
		panic("unexpected ListShopsForModeration call")
	}
	return f.listShopsForModeration(ctx, status)
}

// fakeShopRepo は、手書きの domain.ShopRepository（書き込み）の test double
// である。未設定の振る舞いは panic するので、想定外の呼び出しに対してテストは
// fail-loud する。
type fakeShopRepo struct {
	createShop       func(ctx context.Context, shop domain.Shop) (domain.Shop, error)
	updateShopName   func(ctx context.Context, id string, name string) (domain.Shop, error)
	updateShopStatus func(ctx context.Context, id string, status domain.ShopStatus, note *string) (domain.Shop, error)
}

func (f *fakeShopRepo) CreateShop(ctx context.Context, shop domain.Shop) (domain.Shop, error) {
	if f.createShop == nil {
		panic("unexpected CreateShop call")
	}
	return f.createShop(ctx, shop)
}

func (f *fakeShopRepo) UpdateShopName(ctx context.Context, id string, name string) (domain.Shop, error) {
	if f.updateShopName == nil {
		panic("unexpected UpdateShopName call")
	}
	return f.updateShopName(ctx, id, name)
}

func (f *fakeShopRepo) UpdateShopStatus(ctx context.Context, id string, status domain.ShopStatus, note *string) (domain.Shop, error) {
	if f.updateShopStatus == nil {
		panic("unexpected UpdateShopStatus call")
	}
	return f.updateShopStatus(ctx, id, status, note)
}

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
			query := &fakeShopQuery{
				listShops: func(_ context.Context, _ domain.ShopVisibility, _ string, limit, offset int32) ([]domain.Shop, bool, error) {
					gotLimit, gotOffset = limit, offset
					return []domain.Shop{}, false, nil
				},
			}
			if _, _, err := newShops(query, &fakeShopRepo{}).List(context.Background(), nil, "", tt.page, tt.perPage); err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			if gotLimit != tt.wantLimit || gotOffset != tt.wantOffset {
				t.Errorf("limit/offset = %d/%d, want %d/%d", gotLimit, gotOffset, tt.wantLimit, tt.wantOffset)
			}
		})
	}
}

// TestShopsListVisibilityDescriptor は、List が viewer から記述子を導出し、
// それを変更せずに ShopQuery へ渡すことをアサートする。
func TestShopsListVisibilityDescriptor(t *testing.T) {
	admin := domain.User{ID: uid.N(5), Admin: true}
	var got domain.ShopVisibility
	query := &fakeShopQuery{
		listShops: func(_ context.Context, vis domain.ShopVisibility, _ string, _, _ int32) ([]domain.Shop, bool, error) {
			got = vis
			return nil, false, nil
		},
	}
	if _, _, err := newShops(query, &fakeShopRepo{}).List(context.Background(), &admin, "burger", 1, 20); err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if !got.ViewAll || got.ViewerID != nil {
		t.Errorf("descriptor = %+v, want ViewAll for admin", got)
	}
}

// TestShopsGet は詳細の use case を扱う。見える shop は review つきの完全な
// detail を返し、隠された shop と未知の id はどちらも
// domain.ErrShopNotFound を返す（usecase レベルでの AC4 と AC6）。
func TestShopsGet(t *testing.T) {
	alice := domain.User{ID: uid.N(1), Username: "alice"}
	admin := domain.User{ID: uid.N(2), Admin: true}
	pending := domain.ShopDetail{
		Shop:    domain.Shop{ID: uid.N(10), Name: "Pending Shack", Status: domain.ShopStatusPending, CreatorID: strPtr(alice.ID)},
		Creator: &domain.UserRef{ID: alice.ID, Username: "alice"},
	}
	reviews := []domain.ShopReview{{ID: 3, Rating: 4, CreatedAt: time.Now()}}

	query := &fakeShopQuery{
		getShopWithCreator: func(_ context.Context, id string) (domain.ShopDetail, error) {
			if id == pending.ID {
				return pending, nil
			}
			return domain.ShopDetail{}, domain.ErrShopNotFound
		},
		listShopReviews: func(_ context.Context, shopID string) ([]domain.ShopReview, error) {
			return reviews, nil
		},
	}
	shops := newShops(query, &fakeShopRepo{})

	t.Run("creator は自分の pending な shop を review つきで見られる", func(t *testing.T) {
		got, err := shops.Get(context.Background(), &alice, pending.ID)
		if err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
		want := pending
		want.Reviews = reviews
		want.CanReview = true // creator は自分の pending な shop に review できる
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
		if _, err := shops.Get(context.Background(), &admin, uid.N(999)); !errors.Is(err, domain.ErrShopNotFound) {
			t.Fatalf("Get error = %v, want %v", err, domain.ErrShopNotFound)
		}
	})

	t.Run("review の query が失敗したらそのエラーが伝播する", func(t *testing.T) {
		failing := &fakeShopQuery{
			getShopWithCreator: query.getShopWithCreator,
			listShopReviews: func(_ context.Context, _ string) ([]domain.ShopReview, error) {
				return nil, io.ErrUnexpectedEOF
			},
		}
		if _, err := newShops(failing, &fakeShopRepo{}).Get(context.Background(), &alice, pending.ID); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("Get error = %v, want %v", err, io.ErrUnexpectedEOF)
		}
	})
}

// TestShopsCreate は投稿の use case を扱う。有効な name は viewer を creator と
// する pending の shop を返し、空白の name は repository に触れずに validation
// に失敗する（fake は panic する）。
func TestShopsCreate(t *testing.T) {
	alice := domain.User{ID: uid.N(1), Username: "alice"}

	t.Run("viewer を creator とする pending の shop を作成する", func(t *testing.T) {
		repo := &fakeShopRepo{
			createShop: func(_ context.Context, shop domain.Shop) (domain.Shop, error) {
				shop.ID = uid.N(42)
				return shop, nil
			},
		}
		got, err := newShops(&fakeShopQuery{}, repo).Create(context.Background(), alice, "New Shack")
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		want := domain.ShopDetail{
			Shop: domain.Shop{
				ID: uid.N(42), Name: "New Shack", Status: domain.ShopStatusPending, CreatorID: strPtr(alice.ID),
			},
			Creator: &domain.UserRef{ID: alice.ID, Username: "alice"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Create = %+v, want %+v", got, want)
		}
	})

	t.Run("空白の name は repository を呼ばずに ValidationError を返す", func(t *testing.T) {
		_, err := newShops(&fakeShopQuery{}, &fakeShopRepo{}).Create(context.Background(), alice, "   ")
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
		if _, err := newShops(&fakeShopQuery{}, repo).Create(context.Background(), alice, "New Shack"); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("Create error = %v, want %v", err, io.ErrUnexpectedEOF)
		}
	})
}

// TestShopsAdminForbidden は認可の境界を固定する。admin のすべての操作は、
// admin でない viewer に対して、repository へのアクセスの前に
// domain.ErrForbidden を返す（ゼロ値の fake はどの呼び出しでも panic する）。
func TestShopsAdminForbidden(t *testing.T) {
	alice := domain.User{ID: uid.N(1), Username: "alice"} // 認証済みだが admin ではない
	shops := newShops(&fakeShopQuery{}, &fakeShopRepo{})
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{name: "AdminList は admin でない viewer に ErrForbidden を返す", call: func() error { _, err := shops.AdminList(ctx, alice, ""); return err }},
		{name: "AdminUpdateName は admin でない viewer に ErrForbidden を返す", call: func() error { _, err := shops.AdminUpdateName(ctx, alice, uid.N(1), "x"); return err }},
		{name: "Approve は admin でない viewer に ErrForbidden を返す", call: func() error { _, err := shops.Approve(ctx, alice, uid.N(1)); return err }},
		{name: "Reject は admin でない viewer に ErrForbidden を返す", call: func() error { _, err := shops.Reject(ctx, alice, uid.N(1), nil); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("%s error = %v, want %v", tt.name, err, domain.ErrForbidden)
			}
		})
	}
}

// TestShopsAdminList は moderation の一覧を扱う。既知の status は ShopQuery の
// フィルタになり、status なしはすべてを意味し、未知の status は ShopQuery を
// 呼ばずに空の結果へ short-circuit する。
func TestShopsAdminList(t *testing.T) {
	admin := domain.User{ID: uid.N(2), Admin: true}
	ctx := context.Background()

	t.Run("status のフィルタはそのまま repository に渡される", func(t *testing.T) {
		var got *domain.ShopStatus
		query := &fakeShopQuery{
			listShopsForModeration: func(_ context.Context, status *domain.ShopStatus) ([]domain.ShopDetail, error) {
				got = status
				return []domain.ShopDetail{}, nil
			},
		}
		if _, err := newShops(query, &fakeShopRepo{}).AdminList(ctx, admin, "pending"); err != nil {
			t.Fatalf("AdminList returned error: %v", err)
		}
		if got == nil || *got != domain.ShopStatusPending {
			t.Errorf("filter = %v, want pending", got)
		}
	})

	t.Run("status なしはフィルタなしを意味する", func(t *testing.T) {
		called := false
		query := &fakeShopQuery{
			listShopsForModeration: func(_ context.Context, status *domain.ShopStatus) ([]domain.ShopDetail, error) {
				called = true
				if status != nil {
					t.Errorf("filter = %v, want nil", *status)
				}
				return []domain.ShopDetail{}, nil
			},
		}
		if _, err := newShops(query, &fakeShopRepo{}).AdminList(ctx, admin, ""); err != nil {
			t.Fatalf("AdminList returned error: %v", err)
		}
		if !called {
			t.Error("repository was not called")
		}
	})

	t.Run("未知の status は repository を呼ばずに空の一覧を返す", func(t *testing.T) {
		got, err := newShops(&fakeShopQuery{}, &fakeShopRepo{}).AdminList(ctx, admin, "bogus")
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
	admin := domain.User{ID: uid.N(2), Admin: true}
	ctx := context.Background()
	rejected := domain.ShopDetail{
		Shop:    domain.Shop{ID: uid.N(10), Name: "Shack", Status: domain.ShopStatusRejected, ModerationNote: strPtr("old note"), CreatorID: strPtr(uid.N(1))},
		Creator: &domain.UserRef{ID: uid.N(1), Username: "alice"},
	}
	getRejected := func(_ context.Context, id string) (domain.ShopDetail, error) {
		if id == rejected.ID {
			return rejected, nil
		}
		return domain.ShopDetail{}, domain.ErrShopNotFound
	}
	// statusWrite は 1 回の UpdateShopStatus の呼び出しの引数を記録する。
	type statusWrite struct {
		id     string
		status domain.ShopStatus
		note   *string
	}
	// rejectedQuery は rejected の shop だけを返す（それ以外の読み取りは panic
	// する）。
	rejectedQuery := &fakeShopQuery{getShopWithCreator: getRejected}
	// statusRepoFor は、カラム限定の status の書き込みを記録する。
	// updateShopName は未設定のままなので、name に触れる status 遷移があれば
	// テストが panic する。
	statusRepoFor := func(got *statusWrite) *fakeShopRepo {
		return &fakeShopRepo{
			updateShopStatus: func(_ context.Context, id string, status domain.ShopStatus, note *string) (domain.Shop, error) {
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
		got, err := newShops(rejectedQuery, statusRepoFor(&write)).Approve(ctx, admin, rejected.ID)
		if err != nil {
			t.Fatalf("Approve returned error: %v", err)
		}
		if write.id != rejected.ID || write.status != domain.ShopStatusActive || write.note != nil {
			t.Errorf("status write = %+v, want id %s, active, nil note", write, rejected.ID)
		}
		if got.Status != domain.ShopStatusActive || got.Name != rejected.Name || !reflect.DeepEqual(got.Creator, rejected.Creator) {
			t.Errorf("detail = %+v, want active shop with unchanged name and creator", got)
		}
	})

	t.Run("Reject は status と note だけを永続化する", func(t *testing.T) {
		var write statusWrite
		note := strPtr("needs fixes")
		got, err := newShops(rejectedQuery, statusRepoFor(&write)).Reject(ctx, admin, rejected.ID, note)
		if err != nil {
			t.Fatalf("Reject returned error: %v", err)
		}
		if write.id != rejected.ID || write.status != domain.ShopStatusRejected || write.note != note {
			t.Errorf("status write = %+v, want id %s, rejected, the note", write, rejected.ID)
		}
		if got.ModerationNote != note {
			t.Errorf("detail note = %v, want %v", got.ModerationNote, note)
		}
	})

	t.Run("AdminUpdateName は name だけを永続化する", func(t *testing.T) {
		var gotID string
		var gotName string
		// updateShopStatus は未設定のままなので、status/note に触れる rename が
		// あればテストが panic する。
		repo := &fakeShopRepo{
			updateShopName: func(_ context.Context, id string, name string) (domain.Shop, error) {
				gotID, gotName = id, name
				stored := rejected.Shop
				stored.Name = name
				return stored, nil
			},
		}
		got, err := newShops(rejectedQuery, repo).AdminUpdateName(ctx, admin, rejected.ID, "Renamed")
		if err != nil {
			t.Fatalf("AdminUpdateName returned error: %v", err)
		}
		if gotID != rejected.ID || gotName != "Renamed" {
			t.Errorf("name write = (%s, %q), want (%s, Renamed)", gotID, gotName, rejected.ID)
		}
		if got.Name != "Renamed" || got.Status != rejected.Status {
			t.Errorf("detail = %+v, want renamed with status unchanged", got)
		}
	})

	t.Run("AdminUpdateName は lookup の前に空白の name を拒否する", func(t *testing.T) {
		var err error
		_, err = newShops(&fakeShopQuery{}, &fakeShopRepo{}).AdminUpdateName(ctx, admin, rejected.ID, " ")
		var vErr *domain.ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("error = %v, want *domain.ValidationError", err)
		}
	})

	t.Run("未知の id は書き込みせずに ErrShopNotFound を返す", func(t *testing.T) {
		// 2 つの書き込みの振る舞いはどちらも未設定のままなので、lookup が
		// 失敗した後の書き込みはどれもテストを panic させる。
		shops := newShops(rejectedQuery, &fakeShopRepo{})
		for name, call := range map[string]func() error{
			"Approve":         func() error { _, err := shops.Approve(ctx, admin, uid.N(999)); return err },
			"Reject":          func() error { _, err := shops.Reject(ctx, admin, uid.N(999), nil); return err },
			"AdminUpdateName": func() error { _, err := shops.AdminUpdateName(ctx, admin, uid.N(999), "x"); return err },
		} {
			if err := call(); !errors.Is(err, domain.ErrShopNotFound) {
				t.Errorf("%s error = %v, want %v", name, err, domain.ErrShopNotFound)
			}
		}
	})
}

// TestShopsGetCanReview は、詳細の CanReview が domain の reviewable ルール
// （匿名は false）どおりに設定されることを固定する。
func TestShopsGetCanReview(t *testing.T) {
	alice := domain.User{ID: uid.N(1), Username: "alice"}
	bob := domain.User{ID: uid.N(2), Username: "bob"}
	admin := domain.User{ID: uid.N(3), Username: "root", Admin: true}
	byStatus := map[string]domain.ShopDetail{
		uid.N(10): {Shop: domain.Shop{ID: uid.N(10), Status: domain.ShopStatusActive}},
		uid.N(11): {Shop: domain.Shop{ID: uid.N(11), Status: domain.ShopStatusPending, CreatorID: strPtr(alice.ID)}},
		uid.N(12): {Shop: domain.Shop{ID: uid.N(12), Status: domain.ShopStatusRejected, CreatorID: strPtr(alice.ID)}},
	}
	query := &fakeShopQuery{
		getShopWithCreator: func(_ context.Context, id string) (domain.ShopDetail, error) { return byStatus[id], nil },
		listShopReviews:    func(context.Context, string) ([]domain.ShopReview, error) { return nil, nil },
	}
	shops := newShops(query, &fakeShopRepo{})

	tests := []struct {
		name   string
		viewer *domain.User
		id     string
		want   bool
	}{
		{name: "匿名は active な shop でも false", viewer: nil, id: uid.N(10), want: false},
		{name: "ログイン済みの一般ユーザーは active な shop で true", viewer: &bob, id: uid.N(10), want: true},
		{name: "creator は自分の pending な shop で true", viewer: &alice, id: uid.N(11), want: true},
		{name: "admin は pending な shop で true", viewer: &admin, id: uid.N(11), want: true},
		{name: "creator は rejected な shop でも false", viewer: &alice, id: uid.N(12), want: false},
		{name: "admin は rejected な shop でも false", viewer: &admin, id: uid.N(12), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := shops.Get(context.Background(), tt.viewer, tt.id)
			if err != nil {
				t.Fatalf("Get returned error: %v", err)
			}
			if got.CanReview != tt.want {
				t.Errorf("CanReview = %v, want %v", got.CanReview, tt.want)
			}
		})
	}
}

// TestShopsListHasMore は、一覧の次のページの有無（has_more）が query の判定のまま
// 返ること、query の失敗では false とエラーが返ることを固定する。
func TestShopsListHasMore(t *testing.T) {
	for _, want := range []bool{true, false} {
		query := &fakeShopQuery{
			listShops: func(context.Context, domain.ShopVisibility, string, int32, int32) ([]domain.Shop, bool, error) {
				return []domain.Shop{{ID: uid.N(1)}}, want, nil
			},
		}
		_, got, err := newShops(query, &fakeShopRepo{}).List(context.Background(), nil, "", 1, 20)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if got != want {
			t.Errorf("hasMore = %v, want %v", got, want)
		}
	}

	failing := &fakeShopQuery{
		listShops: func(context.Context, domain.ShopVisibility, string, int32, int32) ([]domain.Shop, bool, error) {
			return nil, true, io.ErrUnexpectedEOF
		},
	}
	if _, hasMore, err := newShops(failing, &fakeShopRepo{}).List(context.Background(), nil, "", 1, 20); !errors.Is(err, io.ErrUnexpectedEOF) || hasMore {
		t.Errorf("List = (hasMore %v, err %v), want (false, %v)", hasMore, err, io.ErrUnexpectedEOF)
	}
}
