package usecase_test

import (
	"context"
	"errors"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/photo"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// fakeReviewRepo は、手書きの usecase.ReviewRepository の test double である。
// 未設定の振る舞いは panic するので、想定外の呼び出し、特にテスト対象の
// フローの外での書き込みに対して、テストは fail-loud する。
type fakeReviewRepo struct {
	listReviews                func(ctx context.Context, filter usecase.ReviewListFilter, limit, offset int32) ([]domain.ReviewDetail, error)
	getReview                  func(ctx context.Context, id int64) (domain.ReviewDetail, error)
	getShop                    func(ctx context.Context, id int64) (domain.Shop, error)
	getShopBurger              func(ctx context.Context, shopID, burgerID int64) (domain.ShopReviewBurger, error)
	createReview               func(ctx context.Context, review domain.Review) (domain.Review, error)
	createReviewForNamedBurger func(ctx context.Context, shopID int64, burgerName string, review domain.Review) (domain.Review, domain.ShopReviewBurger, error)
	updateReviewContent        func(ctx context.Context, id int64, rating int, comment string) (domain.Review, error)
	updateReviewContentAndKey  func(ctx context.Context, id int64, rating int, comment string, photoKey *string) (domain.Review, error)
	discardReview              func(ctx context.Context, id int64) error
}

func (f *fakeReviewRepo) ListReviews(ctx context.Context, filter usecase.ReviewListFilter, limit, offset int32) ([]domain.ReviewDetail, error) {
	if f.listReviews == nil {
		panic("unexpected ListReviews call")
	}
	return f.listReviews(ctx, filter, limit, offset)
}

func (f *fakeReviewRepo) GetReview(ctx context.Context, id int64) (domain.ReviewDetail, error) {
	if f.getReview == nil {
		panic("unexpected GetReview call")
	}
	return f.getReview(ctx, id)
}

func (f *fakeReviewRepo) GetShop(ctx context.Context, id int64) (domain.Shop, error) {
	if f.getShop == nil {
		panic("unexpected GetShop call")
	}
	return f.getShop(ctx, id)
}

func (f *fakeReviewRepo) GetShopBurger(ctx context.Context, shopID, burgerID int64) (domain.ShopReviewBurger, error) {
	if f.getShopBurger == nil {
		panic("unexpected GetShopBurger call")
	}
	return f.getShopBurger(ctx, shopID, burgerID)
}

func (f *fakeReviewRepo) CreateReview(ctx context.Context, review domain.Review) (domain.Review, error) {
	if f.createReview == nil {
		panic("unexpected CreateReview call")
	}
	return f.createReview(ctx, review)
}

func (f *fakeReviewRepo) CreateReviewForNamedBurger(ctx context.Context, shopID int64, burgerName string, review domain.Review) (domain.Review, domain.ShopReviewBurger, error) {
	if f.createReviewForNamedBurger == nil {
		panic("unexpected CreateReviewForNamedBurger call")
	}
	return f.createReviewForNamedBurger(ctx, shopID, burgerName, review)
}

func (f *fakeReviewRepo) UpdateReviewContent(ctx context.Context, id int64, rating int, comment string) (domain.Review, error) {
	if f.updateReviewContent == nil {
		panic("unexpected UpdateReviewContent call")
	}
	return f.updateReviewContent(ctx, id, rating, comment)
}

func (f *fakeReviewRepo) UpdateReviewContentAndPhotoKey(ctx context.Context, id int64, rating int, comment string, photoKey *string) (domain.Review, error) {
	if f.updateReviewContentAndKey == nil {
		panic("unexpected UpdateReviewContentAndPhotoKey call")
	}
	return f.updateReviewContentAndKey(ctx, id, rating, comment, photoKey)
}

func (f *fakeReviewRepo) DiscardReview(ctx context.Context, id int64) error {
	if f.discardReview == nil {
		panic("unexpected DiscardReview call")
	}
	return f.discardReview(ctx, id)
}

// TestReviewsListPagination は Shops.List と同じフォールバック規則を固定する。
// page のデフォルトは 1、per_page のデフォルトは 20、per_page は 100 が上限で、
// 巨大な page はオーバーフローせずに offset を clamp する。
func TestReviewsListPagination(t *testing.T) {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotLimit, gotOffset int32
			repo := &fakeReviewRepo{
				listReviews: func(_ context.Context, _ usecase.ReviewListFilter, limit, offset int32) ([]domain.ReviewDetail, error) {
					gotLimit, gotOffset = limit, offset
					return []domain.ReviewDetail{}, nil
				},
			}
			if _, err := usecase.NewReviews(repo, &fakePhotoStorage{}).List(context.Background(), usecase.ReviewListFilter{}, tt.page, tt.perPage); err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			if gotLimit != tt.wantLimit || gotOffset != tt.wantOffset {
				t.Errorf("limit/offset = %d/%d, want %d/%d", gotLimit, gotOffset, tt.wantLimit, tt.wantOffset)
			}
		})
	}
}

// TestReviewsListFilterPassThrough は、List が filter を変更せずに repository へ
// 渡すことを固定する。usecase は filter の値を正規化も validate もしない
// （指定の有無は handler が決め、一致の判定は SQL が行う）。
func TestReviewsListFilterPassThrough(t *testing.T) {
	rating := 4
	shopID := int64(7)
	want := usecase.ReviewListFilter{Rating: &rating, Keyword: "tasty", ShopID: &shopID}
	var got usecase.ReviewListFilter
	repo := &fakeReviewRepo{
		listReviews: func(_ context.Context, filter usecase.ReviewListFilter, _, _ int32) ([]domain.ReviewDetail, error) {
			got = filter
			return []domain.ReviewDetail{}, nil
		},
	}
	if _, err := usecase.NewReviews(repo, &fakePhotoStorage{}).List(context.Background(), want, 1, 20); err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filter = %+v, want %+v", got, want)
	}
}

// TestReviewsListFailure：repository の失敗は wrap されて伝播する。
func TestReviewsListFailure(t *testing.T) {
	repo := &fakeReviewRepo{
		listReviews: func(_ context.Context, _ usecase.ReviewListFilter, _, _ int32) ([]domain.ReviewDetail, error) {
			return nil, io.ErrUnexpectedEOF
		},
	}
	if _, err := usecase.NewReviews(repo, &fakePhotoStorage{}).List(context.Background(), usecase.ReviewListFilter{}, 1, 20); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("List error = %v, want %v", err, io.ErrUnexpectedEOF)
	}
}

// TestReviewsGet は詳細の use case を扱う。見つかった review はそのまま通り、
// 存在しない、または discard 済みの review は domain.ErrReviewNotFound を
// 返す（usecase レベルでの issue #14 AC6）。
func TestReviewsGet(t *testing.T) {
	detail := domain.ReviewDetail{
		Review: domain.Review{ID: 9, Rating: 4, AuthorID: 1, BurgerID: 5, CreatedAt: time.Now()},
		User:   &domain.UserRef{ID: 1, Username: "alice"},
		Burger: &domain.ShopReviewBurger{ID: 5, Name: "Cheese"},
	}
	repo := &fakeReviewRepo{
		getReview: func(_ context.Context, id int64) (domain.ReviewDetail, error) {
			if id == detail.ID {
				return detail, nil
			}
			return domain.ReviewDetail{}, domain.ErrReviewNotFound
		},
	}
	reviews := usecase.NewReviews(repo, &fakePhotoStorage{})

	got, err := reviews.Get(context.Background(), detail.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if !reflect.DeepEqual(got, detail) {
		t.Errorf("Get = %+v, want %+v", got, detail)
	}

	if _, err := reviews.Get(context.Background(), 999); !errors.Is(err, domain.ErrReviewNotFound) {
		t.Fatalf("Get error = %v, want %v", err, domain.ErrReviewNotFound)
	}
}

// TestReviewsCreate は投稿のフローと、その厳密なチェック順序を扱う。
// shop 404 → reviewable 403 → burger 404 → validation 422 → insert である。
// fake の未設定の振る舞いにより、順序どおりでない呼び出しは panic になる。
func TestReviewsCreate(t *testing.T) {
	alice := domain.User{ID: 1, Username: "alice"}
	bob := domain.User{ID: 2, Username: "bob"}
	admin := domain.User{ID: 3, Username: "root", Admin: true}
	ctx := context.Background()

	activeShop := domain.Shop{ID: 10, Status: domain.ShopStatusActive}
	pendingShop := domain.Shop{ID: 11, Status: domain.ShopStatusPending, CreatorID: int64Ptr(alice.ID)}
	rejectedShop := domain.Shop{ID: 12, Status: domain.ShopStatusRejected, CreatorID: int64Ptr(alice.ID)}
	cheese := domain.ShopReviewBurger{ID: 5, Name: "Cheese", AverageRating: 4.5, ReviewCount: 2, WeightedScore: 4.1, Confidence: 0.8}

	getShop := func(_ context.Context, id int64) (domain.Shop, error) {
		for _, shop := range []domain.Shop{activeShop, pendingShop, rejectedShop} {
			if shop.ID == id {
				return shop, nil
			}
		}
		return domain.Shop{}, domain.ErrShopNotFound
	}
	getShopBurger := func(_ context.Context, shopID, burgerID int64) (domain.ShopReviewBurger, error) {
		if burgerID == cheese.ID {
			return cheese, nil
		}
		return domain.ShopReviewBurger{}, domain.ErrBurgerNotFound
	}

	t.Run("AC1 認証済みの投稿は active な shop に insert され payload が組み立てられる", func(t *testing.T) {
		var inserted domain.Review
		repo := &fakeReviewRepo{
			getShop:       getShop,
			getShopBurger: getShopBurger,
			createReview: func(_ context.Context, review domain.Review) (domain.Review, error) {
				inserted = review
				review.ID = 42
				review.CreatedAt = time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
				return review, nil
			},
		}
		got, err := usecase.NewReviews(repo, &fakePhotoStorage{}).Create(ctx, bob, activeShop.ID, cheese.ID, "", 4, "Tasty", nil)
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if inserted.AuthorID != bob.ID || inserted.BurgerID != cheese.ID || inserted.Rating != 4 {
			t.Errorf("inserted = %+v, want author %d, burger %d, rating 4", inserted, bob.ID, cheese.ID)
		}
		if got.ID != 42 || got.Rating != 4 || got.Comment == nil || *got.Comment != "Tasty" {
			t.Errorf("detail review = %+v, want id 42, rating 4, comment Tasty", got.Review)
		}
		if !reflect.DeepEqual(got.User, &domain.UserRef{ID: bob.ID, Username: "bob"}) {
			t.Errorf("user = %+v, want the viewer", got.User)
		}
		if !reflect.DeepEqual(got.Burger, &cheese) {
			t.Errorf("burger = %+v, want %+v", got.Burger, cheese)
		}
	})

	t.Run("未知の shop は他の何より先に ErrShopNotFound を返す", func(t *testing.T) {
		repo := &fakeReviewRepo{getShop: getShop}
		if _, err := usecase.NewReviews(repo, &fakePhotoStorage{}).Create(ctx, bob, 999, cheese.ID, "", 4, "ok", nil); !errors.Is(err, domain.ErrShopNotFound) {
			t.Fatalf("Create error = %v, want %v", err, domain.ErrShopNotFound)
		}
	})

	t.Run("AC2 rejected な shop は burger の lookup より先に ErrForbidden を返す", func(t *testing.T) {
		repo := &fakeReviewRepo{getShop: getShop} // getShopBurger は未設定：lookup があれば panic する
		for _, viewer := range []domain.User{alice, bob, admin} {
			if _, err := usecase.NewReviews(repo, &fakePhotoStorage{}).Create(ctx, viewer, rejectedShop.ID, cheese.ID, "", 4, "ok", nil); !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("viewer %s: error = %v, want %v", viewer.Username, err, domain.ErrForbidden)
			}
		}
	})

	t.Run("AC2 pending な shop には creator と admin は投稿でき、それ以外は ErrForbidden になる", func(t *testing.T) {
		repo := &fakeReviewRepo{
			getShop:       getShop,
			getShopBurger: getShopBurger,
			createReview: func(_ context.Context, review domain.Review) (domain.Review, error) {
				review.ID = 43
				return review, nil
			},
		}
		reviews := usecase.NewReviews(repo, &fakePhotoStorage{})
		if _, err := reviews.Create(ctx, alice, pendingShop.ID, cheese.ID, "", 4, "ok", nil); err != nil {
			t.Errorf("creator: Create returned error: %v", err)
		}
		if _, err := reviews.Create(ctx, admin, pendingShop.ID, cheese.ID, "", 4, "ok", nil); err != nil {
			t.Errorf("admin: Create returned error: %v", err)
		}
		if _, err := usecase.NewReviews(&fakeReviewRepo{getShop: getShop}, &fakePhotoStorage{}).Create(ctx, bob, pendingShop.ID, cheese.ID, "", 4, "ok", nil); !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("other user: error = %v, want %v", err, domain.ErrForbidden)
		}
	})

	t.Run("shop に紐付かない burger は insert せずに ErrBurgerNotFound を返す", func(t *testing.T) {
		repo := &fakeReviewRepo{getShop: getShop, getShopBurger: getShopBurger} // createReview は未設定
		if _, err := usecase.NewReviews(repo, &fakePhotoStorage{}).Create(ctx, bob, activeShop.ID, 999, "", 4, "ok", nil); !errors.Is(err, domain.ErrBurgerNotFound) {
			t.Fatalf("Create error = %v, want %v", err, domain.ErrBurgerNotFound)
		}
	})

	t.Run("AC4 不正な内容は insert せずに ValidationError を返す", func(t *testing.T) {
		repo := &fakeReviewRepo{getShop: getShop, getShopBurger: getShopBurger} // createReview は未設定
		_, err := usecase.NewReviews(repo, &fakePhotoStorage{}).Create(ctx, bob, activeShop.ID, cheese.ID, "", 0, " ", nil)
		var vErr *domain.ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("error = %v, want *domain.ValidationError", err)
		}
		want := []string{"Rating must be in 1..5", "Comment can't be blank"}
		if !reflect.DeepEqual(vErr.Messages, want) {
			t.Errorf("messages = %v, want %v", vErr.Messages, want)
		}
	})

	t.Run("burger_name 経由では repository 越しに find-or-create し payload を組み立てる", func(t *testing.T) {
		smash := domain.ShopReviewBurger{ID: 7, Name: " Smash "}
		var gotShopID int64
		var gotName string
		var gotReview domain.Review
		repo := &fakeReviewRepo{
			getShop: getShop, // getShopBurger と createReview は未設定：どの呼び出しも panic する
			createReviewForNamedBurger: func(_ context.Context, shopID int64, burgerName string, review domain.Review) (domain.Review, domain.ShopReviewBurger, error) {
				gotShopID, gotName, gotReview = shopID, burgerName, review
				review.ID = 44
				review.BurgerID = smash.ID
				return review, smash, nil
			},
		}
		// name は trim されないまま repository に届く（Rails は決して
		// trim しない）。
		got, err := usecase.NewReviews(repo, &fakePhotoStorage{}).Create(ctx, bob, activeShop.ID, 0, " Smash ", 4, "Juicy", nil)
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if gotShopID != activeShop.ID || gotName != " Smash " {
			t.Errorf("repo got shop %d name %q, want %d %q", gotShopID, gotName, activeShop.ID, " Smash ")
		}
		if gotReview.AuthorID != bob.ID || gotReview.Rating != 4 {
			t.Errorf("repo got review %+v, want author %d rating 4", gotReview, bob.ID)
		}
		if got.ID != 44 || got.BurgerID != smash.ID {
			t.Errorf("detail review = %+v, want id 44 for burger %d", got.Review, smash.ID)
		}
		if !reflect.DeepEqual(got.Burger, &smash) {
			t.Errorf("burger = %+v, want %+v", got.Burger, smash)
		}
	})

	t.Run("正の burger_id は burger_name より優先される", func(t *testing.T) {
		repo := &fakeReviewRepo{
			getShop:       getShop,
			getShopBurger: getShopBurger, // createReviewForNamedBurger は未設定：呼び出しは panic する
			createReview: func(_ context.Context, review domain.Review) (domain.Review, error) {
				review.ID = 45
				return review, nil
			},
		}
		got, err := usecase.NewReviews(repo, &fakePhotoStorage{}).Create(ctx, bob, activeShop.ID, cheese.ID, "Ignored", 4, "ok", nil)
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if got.BurgerID != cheese.ID || !reflect.DeepEqual(got.Burger, &cheese) {
			t.Errorf("burger = %+v, want the burger_id one %+v", got.Burger, cheese)
		}
	})

	t.Run("burger_id がなく burger_name も空か空白のみなら、書き込みせずに ValidationError を返す", func(t *testing.T) {
		for name, burgerName := range map[string]string{"burger_name が未指定": "", "burger_name が空白のみ": "  \t "} {
			t.Run(name, func(t *testing.T) {
				repo := &fakeReviewRepo{getShop: getShop} // すべての書き込みは未設定：呼び出しは panic する
				_, err := usecase.NewReviews(repo, &fakePhotoStorage{}).Create(ctx, bob, activeShop.ID, 0, burgerName, 4, "ok", nil)
				var vErr *domain.ValidationError
				if !errors.As(err, &vErr) {
					t.Fatalf("error = %v, want *domain.ValidationError", err)
				}
				if want := []string{"Burger name can't be blank"}; !reflect.DeepEqual(vErr.Messages, want) {
					t.Errorf("messages = %v, want %v", vErr.Messages, want)
				}
			})
		}
	})

	t.Run("burger_name 経由では書き込みの前に内容を validate する", func(t *testing.T) {
		repo := &fakeReviewRepo{getShop: getShop} // createReviewForNamedBurger は未設定：呼び出しは panic する
		_, err := usecase.NewReviews(repo, &fakePhotoStorage{}).Create(ctx, bob, activeShop.ID, 0, "Smash", 0, " ", nil)
		var vErr *domain.ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("error = %v, want *domain.ValidationError", err)
		}
		want := []string{"Rating must be in 1..5", "Comment can't be blank"}
		if !reflect.DeepEqual(vErr.Messages, want) {
			t.Errorf("messages = %v, want %v", vErr.Messages, want)
		}
	})
}

// reviewDetailFor は、edit/delete のテストが load する、保存済みの detail を
// 組み立てる。
func reviewDetailFor(authorID int64) domain.ReviewDetail {
	comment := "Old"
	return domain.ReviewDetail{
		Review: domain.Review{ID: 9, Rating: 2, Comment: &comment, AuthorID: authorID, BurgerID: 5,
			CreatedAt: time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)},
		User:   &domain.UserRef{ID: authorID, Username: "alice"},
		Burger: &domain.ShopReviewBurger{ID: 5, Name: "Cheese", AverageRating: 4.5, ReviewCount: 2},
	}
}

// TestReviewsUpdate は edit のフローを扱う。author のみ（AC3、admin でも
// 通らない）、書き込みの前の validation、カラム限定の書き込みそのもの
// （discardReview は未設定のままなので、どの discard も panic する）、
// そして 404 である。
func TestReviewsUpdate(t *testing.T) {
	alice := domain.User{ID: 1, Username: "alice"}
	bob := domain.User{ID: 2, Username: "bob"}
	admin := domain.User{ID: 3, Username: "root", Admin: true}
	ctx := context.Background()
	stored := reviewDetailFor(alice.ID)
	getReview := func(_ context.Context, id int64) (domain.ReviewDetail, error) {
		if id == stored.ID {
			return stored, nil
		}
		return domain.ReviewDetail{}, domain.ErrReviewNotFound
	}

	t.Run("AC3 author の edit は rating と comment だけを永続化する", func(t *testing.T) {
		var gotID int64
		var gotRating int
		var gotComment string
		repo := &fakeReviewRepo{
			getReview: getReview,
			updateReviewContent: func(_ context.Context, id int64, rating int, comment string) (domain.Review, error) {
				gotID, gotRating, gotComment = id, rating, comment
				updated := stored.Review
				updated.Rating = rating
				c := comment
				updated.Comment = &c
				return updated, nil
			},
		}
		got, err := usecase.NewReviews(repo, &fakePhotoStorage{}).Update(ctx, alice, stored.ID, 5, "Better", nil)
		if err != nil {
			t.Fatalf("Update returned error: %v", err)
		}
		if gotID != stored.ID || gotRating != 5 || gotComment != "Better" {
			t.Errorf("write = (%d, %d, %q), want (%d, 5, Better)", gotID, gotRating, gotComment, stored.ID)
		}
		if got.Rating != 5 || got.Comment == nil || *got.Comment != "Better" {
			t.Errorf("detail = %+v, want rating 5, comment Better", got.Review)
		}
		if !reflect.DeepEqual(got.User, stored.User) || !reflect.DeepEqual(got.Burger, stored.Burger) {
			t.Errorf("user/burger = %+v/%+v, want kept from the load", got.User, got.Burger)
		}
	})

	t.Run("AC3 author 以外は書き込みせずに ErrForbidden を返す", func(t *testing.T) {
		repo := &fakeReviewRepo{getReview: getReview}      // updateReviewContent は未設定
		for _, viewer := range []domain.User{bob, admin} { // admin でも通さない
			if _, err := usecase.NewReviews(repo, &fakePhotoStorage{}).Update(ctx, viewer, stored.ID, 5, "x", nil); !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("viewer %s: error = %v, want %v", viewer.Username, err, domain.ErrForbidden)
			}
		}
	})

	t.Run("AC4 不正な内容は書き込みせずに ValidationError を返す", func(t *testing.T) {
		repo := &fakeReviewRepo{getReview: getReview}
		_, err := usecase.NewReviews(repo, &fakePhotoStorage{}).Update(ctx, alice, stored.ID, 6, "ok", nil)
		var vErr *domain.ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("error = %v, want *domain.ValidationError", err)
		}
		if want := []string{"Rating must be in 1..5"}; !reflect.DeepEqual(vErr.Messages, want) {
			t.Errorf("messages = %v, want %v", vErr.Messages, want)
		}
	})

	t.Run("未知の id は ErrReviewNotFound を返す", func(t *testing.T) {
		repo := &fakeReviewRepo{getReview: getReview}
		if _, err := usecase.NewReviews(repo, &fakePhotoStorage{}).Update(ctx, alice, 999, 5, "x", nil); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Fatalf("Update error = %v, want %v", err, domain.ErrReviewNotFound)
		}
	})
}

// TestReviewsDelete は soft delete のフローを扱う。author のみ（AC3）、
// load した review そのものに対して discard が呼ばれること
// （updateReviewContent は未設定のままなので、どの content の書き込みも
// panic する）、そして 404 である。
func TestReviewsDelete(t *testing.T) {
	alice := domain.User{ID: 1, Username: "alice"}
	bob := domain.User{ID: 2, Username: "bob"}
	admin := domain.User{ID: 3, Username: "root", Admin: true}
	ctx := context.Background()
	stored := reviewDetailFor(alice.ID)
	getReview := func(_ context.Context, id int64) (domain.ReviewDetail, error) {
		if id == stored.ID {
			return stored, nil
		}
		return domain.ReviewDetail{}, domain.ErrReviewNotFound
	}

	t.Run("AC6 author の delete は review を discard する", func(t *testing.T) {
		var discarded int64
		repo := &fakeReviewRepo{
			getReview: getReview,
			discardReview: func(_ context.Context, id int64) error {
				discarded = id
				return nil
			},
		}
		if err := usecase.NewReviews(repo, &fakePhotoStorage{}).Delete(ctx, alice, stored.ID); err != nil {
			t.Fatalf("Delete returned error: %v", err)
		}
		if discarded != stored.ID {
			t.Errorf("discarded id = %d, want %d", discarded, stored.ID)
		}
	})

	t.Run("AC3 author 以外は書き込みせずに ErrForbidden を返す", func(t *testing.T) {
		repo := &fakeReviewRepo{getReview: getReview}      // discardReview は未設定
		for _, viewer := range []domain.User{bob, admin} { // admin でも通さない
			if err := usecase.NewReviews(repo, &fakePhotoStorage{}).Delete(ctx, viewer, stored.ID); !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("viewer %s: error = %v, want %v", viewer.Username, err, domain.ErrForbidden)
			}
		}
	})

	t.Run("未知の id は ErrReviewNotFound を返す", func(t *testing.T) {
		repo := &fakeReviewRepo{getReview: getReview}
		if err := usecase.NewReviews(repo, &fakePhotoStorage{}).Delete(ctx, alice, 999); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Fatalf("Delete error = %v, want %v", err, domain.ErrReviewNotFound)
		}
	})

	t.Run("discard の競合が起きたときは repository の ErrReviewNotFound が返る", func(t *testing.T) {
		repo := &fakeReviewRepo{
			getReview: getReview,
			discardReview: func(_ context.Context, _ int64) error {
				return domain.ErrReviewNotFound // load から書き込みまでの間に discard された
			},
		}
		if err := usecase.NewReviews(repo, &fakePhotoStorage{}).Delete(ctx, alice, stored.ID); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Fatalf("Delete error = %v, want %v", err, domain.ErrReviewNotFound)
		}
	})
}

// TestNewReviewsNilPhotoStorage は、fail-loud なコンストラクタのガードを
// 固定する。nil の PhotoStorage は、リクエストの途中ではなく、配線時に
// panic しなければならない。
func TestNewReviewsNilPhotoStorage(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewReviews with a nil PhotoStorage did not panic")
		}
	}()
	usecase.NewReviews(&fakeReviewRepo{}, nil)
}

// fakePhotoStorage は Put/Delete の key を順に記録し、putErr は Put を
// 失敗させる。
type fakePhotoStorage struct {
	putErr  error
	puts    []string
	deletes []string
}

func (f *fakePhotoStorage) Put(_ context.Context, key, _ string, _ io.Reader) error {
	if f.putErr != nil {
		return f.putErr
	}
	f.puts = append(f.puts, key)
	return nil
}

func (f *fakePhotoStorage) Delete(_ context.Context, key string) error {
	f.deletes = append(f.deletes, key)
	return nil
}

func (f *fakePhotoStorage) URL(key string) string { return "/photos/" + key }

// TestReviewsCreatePhoto は Create の S10 の写真フローを扱う。blob は insert の
// 前にランダムな reviews/<hex><ext> の key で保存され、その key は review に
// 永続化され、insert が失敗した場合はアップロードしたばかりの blob を
// best-effort で削除するので、孤立ファイルは残らない。
func TestReviewsCreatePhoto(t *testing.T) {
	ctx := context.Background()
	bob := domain.User{ID: 2, Username: "bob"}
	activeShop := domain.Shop{ID: 10, Status: domain.ShopStatusActive}
	cheese := domain.ShopReviewBurger{ID: 5, Name: "Cheese"}
	upload := &photo.Processed{Data: []byte("img"), ContentType: "image/jpeg", Ext: ".jpg"}
	getShop := func(_ context.Context, id int64) (domain.Shop, error) { return activeShop, nil }
	getShopBurger := func(_ context.Context, _, _ int64) (domain.ShopReviewBurger, error) { return cheese, nil }

	t.Run("blob を保存し、その key を永続化する", func(t *testing.T) {
		photos := &fakePhotoStorage{}
		var inserted domain.Review
		repo := &fakeReviewRepo{
			getShop:       getShop,
			getShopBurger: getShopBurger,
			createReview: func(_ context.Context, review domain.Review) (domain.Review, error) {
				inserted = review
				review.ID = 42
				return review, nil
			},
		}
		got, err := usecase.NewReviews(repo, photos).Create(ctx, bob, activeShop.ID, cheese.ID, "", 4, "Tasty", upload)
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if len(photos.puts) != 1 || len(photos.deletes) != 0 {
			t.Fatalf("puts/deletes = %v/%v, want one put, no delete", photos.puts, photos.deletes)
		}
		key := photos.puts[0]
		if !strings.HasPrefix(key, "reviews/") || !strings.HasSuffix(key, ".jpg") || len(key) != len("reviews/")+32+len(".jpg") {
			t.Errorf("key = %q, want reviews/<32 hex>.jpg", key)
		}
		if inserted.PhotoKey == nil || *inserted.PhotoKey != key {
			t.Errorf("inserted PhotoKey = %v, want %q", inserted.PhotoKey, key)
		}
		if got.PhotoURL == nil || *got.PhotoURL != "/photos/"+key {
			t.Errorf("PhotoURL = %v, want %q", got.PhotoURL, "/photos/"+key)
		}
	})

	t.Run("insert が失敗したらアップロードした blob を best-effort で削除する", func(t *testing.T) {
		photos := &fakePhotoStorage{}
		repo := &fakeReviewRepo{
			getShop:       getShop,
			getShopBurger: getShopBurger,
			createReview: func(_ context.Context, _ domain.Review) (domain.Review, error) {
				return domain.Review{}, io.ErrUnexpectedEOF
			},
		}
		_, err := usecase.NewReviews(repo, photos).Create(ctx, bob, activeShop.ID, cheese.ID, "", 4, "Tasty", upload)
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("Create error = %v, want the DB error", err)
		}
		if len(photos.puts) != 1 || !reflect.DeepEqual(photos.deletes, photos.puts) {
			t.Errorf("puts/deletes = %v/%v, want the uploaded key deleted", photos.puts, photos.deletes)
		}
	})

	t.Run("Put が失敗したら insert せずにそのエラーを返す", func(t *testing.T) {
		photos := &fakePhotoStorage{putErr: io.ErrUnexpectedEOF}
		repo := &fakeReviewRepo{getShop: getShop, getShopBurger: getShopBurger} // createReview は未設定：insert があれば panic する
		if _, err := usecase.NewReviews(repo, photos).Create(ctx, bob, activeShop.ID, cheese.ID, "", 4, "Tasty", upload); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("Create error = %v, want the storage error", err)
		}
	})
}

// TestReviewsUpdatePhoto は Update の S10 の置き換えフローを扱う。新しい blob が
// 先に保存され、次に content と photo_key が「1 つの」atomic な repository の
// 書き込みで切り替わり、そのあとではじめて「古い」blob（load のスナップ
// ショットのもの）が best-effort で削除される。upload がなければ content
// だけの書き込みが走り、写真の書き込みは決して起こらない。
func TestReviewsUpdatePhoto(t *testing.T) {
	ctx := context.Background()
	alice := domain.User{ID: 1, Username: "alice"}
	oldKey := "reviews/old.jpg"
	stored := domain.ReviewDetail{
		Review: domain.Review{ID: 9, Rating: 4, AuthorID: alice.ID, BurgerID: 5, PhotoKey: &oldKey},
		User:   &domain.UserRef{ID: alice.ID, Username: "alice"},
		Burger: &domain.ShopReviewBurger{ID: 5, Name: "Cheese"},
	}
	getReview := func(_ context.Context, id int64) (domain.ReviewDetail, error) { return stored, nil }
	upload := &photo.Processed{Data: []byte("img"), ContentType: "image/png", Ext: ".png"}

	t.Run("content と key を atomic に置き換え、DB 成功後に古い blob を削除する", func(t *testing.T) {
		photos := &fakePhotoStorage{}
		var gotRating int
		var gotComment string
		repo := &fakeReviewRepo{
			getReview: getReview,
			// updateReviewContent は未設定のままである。写真の経路で
			// content だけの別の文を実行すれば panic する（書き込みは
			// 単一の atomic な repository の呼び出しでなければならない）。
			updateReviewContentAndKey: func(_ context.Context, id int64, rating int, comment string, photoKey *string) (domain.Review, error) {
				gotRating, gotComment = rating, comment
				review := stored.Review
				review.Rating = rating
				c := comment
				review.Comment = &c
				review.PhotoKey = photoKey
				return review, nil
			},
		}
		got, err := usecase.NewReviews(repo, photos).Update(ctx, alice, stored.ID, 5, "Better", upload)
		if err != nil {
			t.Fatalf("Update returned error: %v", err)
		}
		if gotRating != 5 || gotComment != "Better" {
			t.Errorf("write = (%d, %q), want (5, Better)", gotRating, gotComment)
		}
		if len(photos.puts) != 1 || !strings.HasSuffix(photos.puts[0], ".png") {
			t.Fatalf("puts = %v, want one .png key", photos.puts)
		}
		if !reflect.DeepEqual(photos.deletes, []string{oldKey}) {
			t.Errorf("deletes = %v, want only the old key %q", photos.deletes, oldKey)
		}
		if got.PhotoURL == nil || *got.PhotoURL != "/photos/"+photos.puts[0] {
			t.Errorf("PhotoURL = %v, want the new key's URL", got.PhotoURL)
		}
	})

	t.Run("更新中に discard されると atomic な書き込みが失敗し、新しい blob を削除して古い blob は残す", func(t *testing.T) {
		photos := &fakePhotoStorage{}
		repo := &fakeReviewRepo{
			getReview: getReview,
			// updateReviewContent は未設定のままである。atomic な書き込みが
			// 失敗したということは content の文が「一切」実行されていない
			// ので、あとから content の変更が見えることはありえない。
			// 紛れ込んだ content だけの呼び出しは panic する。
			updateReviewContentAndKey: func(_ context.Context, _ int64, _ int, _ string, _ *string) (domain.Review, error) {
				return domain.Review{}, domain.ErrReviewNotFound // load から書き込みまでの間に discard された
			},
		}
		_, err := usecase.NewReviews(repo, photos).Update(ctx, alice, stored.ID, 5, "Better", upload)
		if !errors.Is(err, domain.ErrReviewNotFound) {
			t.Fatalf("Update error = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if !reflect.DeepEqual(photos.deletes, photos.puts) {
			t.Errorf("puts/deletes = %v/%v, want the new blob deleted and the old kept", photos.puts, photos.deletes)
		}
	})

	t.Run("upload がなければ content だけの書き込みになり、photo storage には触れない", func(t *testing.T) {
		photos := &fakePhotoStorage{}
		repo := &fakeReviewRepo{
			getReview: getReview,
			// updateReviewContentAndKey は未設定：photo_key のどんな書き込みも
			// panic する。
			updateReviewContent: func(_ context.Context, _ int64, _ int, _ string) (domain.Review, error) {
				return stored.Review, nil
			},
		}
		got, err := usecase.NewReviews(repo, photos).Update(ctx, alice, stored.ID, 5, "Better", nil)
		if err != nil {
			t.Fatalf("Update returned error: %v", err)
		}
		if len(photos.puts) != 0 || len(photos.deletes) != 0 {
			t.Errorf("puts/deletes = %v/%v, want none", photos.puts, photos.deletes)
		}
		if got.PhotoURL == nil || *got.PhotoURL != "/photos/"+oldKey {
			t.Errorf("PhotoURL = %v, want the kept old key's URL", got.PhotoURL)
		}
	})
}

// TestReviewsDeletePhoto は Delete の S10 の末尾部分を扱う。discard が成功
// した後に、blob は best-effort で削除される。
func TestReviewsDeletePhoto(t *testing.T) {
	ctx := context.Background()
	alice := domain.User{ID: 1, Username: "alice"}
	key := "reviews/gone.jpg"
	photos := &fakePhotoStorage{}
	repo := &fakeReviewRepo{
		getReview: func(_ context.Context, id int64) (domain.ReviewDetail, error) {
			return domain.ReviewDetail{Review: domain.Review{ID: id, AuthorID: alice.ID, PhotoKey: &key}}, nil
		},
		discardReview: func(_ context.Context, _ int64) error { return nil },
	}
	if err := usecase.NewReviews(repo, photos).Delete(ctx, alice, 9); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if !reflect.DeepEqual(photos.deletes, []string{key}) {
		t.Errorf("deletes = %v, want %q", photos.deletes, key)
	}
}
