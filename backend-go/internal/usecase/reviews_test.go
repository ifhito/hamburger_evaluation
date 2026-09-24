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
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// fakeReviewQuery は、手書きの usecase.ReviewQuery の test double である。
// 未設定の振る舞いは panic するので、想定外の呼び出しに対してテストは
// fail-loud する。
type fakeReviewQuery struct {
	listReviews     func(ctx context.Context, filter usecase.ReviewListFilter, limit, offset int32) ([]domain.ReviewDetail, bool, error)
	getReview       func(ctx context.Context, id string) (domain.ReviewDetail, error)
	getShop         func(ctx context.Context, id string) (domain.Shop, error)
	getShopBurger   func(ctx context.Context, shopID, burgerID string) (domain.ShopReviewBurger, error)
	listReviewShops func(ctx context.Context, reviewID string) ([]domain.Shop, error)
}

func (f *fakeReviewQuery) ListReviews(ctx context.Context, filter usecase.ReviewListFilter, limit, offset int32) ([]domain.ReviewDetail, bool, error) {
	if f.listReviews == nil {
		panic("unexpected ListReviews call")
	}
	return f.listReviews(ctx, filter, limit, offset)
}

func (f *fakeReviewQuery) GetReview(ctx context.Context, id string) (domain.ReviewDetail, error) {
	if f.getReview == nil {
		panic("unexpected GetReview call")
	}
	return f.getReview(ctx, id)
}

func (f *fakeReviewQuery) GetShop(ctx context.Context, id string) (domain.Shop, error) {
	if f.getShop == nil {
		panic("unexpected GetShop call")
	}
	return f.getShop(ctx, id)
}

// ListReviewShops は、未設定なら、ショップなし(空の一覧)を返す。
func (f *fakeReviewQuery) ListReviewShops(ctx context.Context, reviewID string) ([]domain.Shop, error) {
	if f.listReviewShops == nil {
		return nil, nil
	}
	return f.listReviewShops(ctx, reviewID)
}

func (f *fakeReviewQuery) GetShopBurger(ctx context.Context, shopID, burgerID string) (domain.ShopReviewBurger, error) {
	if f.getShopBurger == nil {
		panic("unexpected GetShopBurger call")
	}
	return f.getShopBurger(ctx, shopID, burgerID)
}

// fakeReviewRepo は、手書きの domain.ReviewRepository（書き込み）の test double
// である。未設定の振る舞いは panic するので、想定外の呼び出し、特にテスト対象の
// フローの外での書き込みに対して、テストは fail-loud する。
type fakeReviewRepo struct {
	createReview              func(ctx context.Context, review domain.Review) (domain.Review, error)
	createShopBurger          func(ctx context.Context, shopID string, burgerName string) (domain.ShopReviewBurger, error)
	updateReviewContent       func(ctx context.Context, id string, rating int, comment string) (domain.Review, error)
	updateReviewContentAndKey func(ctx context.Context, id string, rating int, comment string, photoKey *string) (domain.Review, error)
	discardReview             func(ctx context.Context, id string) error
}

func (f *fakeReviewRepo) CreateReview(ctx context.Context, review domain.Review) (domain.Review, error) {
	if f.createReview == nil {
		panic("unexpected CreateReview call")
	}
	return f.createReview(ctx, review)
}

func (f *fakeReviewRepo) CreateShopBurger(ctx context.Context, shopID string, burgerName string) (domain.ShopReviewBurger, error) {
	if f.createShopBurger == nil {
		panic("unexpected CreateShopBurger call")
	}
	return f.createShopBurger(ctx, shopID, burgerName)
}

func (f *fakeReviewRepo) UpdateReviewContent(ctx context.Context, id string, rating int, comment string) (domain.Review, error) {
	if f.updateReviewContent == nil {
		panic("unexpected UpdateReviewContent call")
	}
	return f.updateReviewContent(ctx, id, rating, comment)
}

func (f *fakeReviewRepo) UpdateReviewContentAndPhotoKey(ctx context.Context, id string, rating int, comment string, photoKey *string) (domain.Review, error) {
	if f.updateReviewContentAndKey == nil {
		panic("unexpected UpdateReviewContentAndPhotoKey call")
	}
	return f.updateReviewContentAndKey(ctx, id, rating, comment, photoKey)
}

func (f *fakeReviewRepo) DiscardReview(ctx context.Context, id string) error {
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
			query := &fakeReviewQuery{
				listReviews: func(_ context.Context, _ usecase.ReviewListFilter, limit, offset int32) ([]domain.ReviewDetail, bool, error) {
					gotLimit, gotOffset = limit, offset
					return []domain.ReviewDetail{}, false, nil
				},
			}
			if _, _, err := newReviews(query, &fakeReviewRepo{}, &fakePhotoStorage{}).List(context.Background(), nil, usecase.ReviewListFilter{}, tt.page, tt.perPage); err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			if gotLimit != tt.wantLimit || gotOffset != tt.wantOffset {
				t.Errorf("limit/offset = %d/%d, want %d/%d", gotLimit, gotOffset, tt.wantLimit, tt.wantOffset)
			}
		})
	}
}

// TestReviewsListFilterPassThrough は、List が filter を変更せずに query へ
// 渡すことを固定する。usecase は filter の値を正規化も validate もしない
// （指定の有無は handler が決め、一致の判定は SQL が行う）。
func TestReviewsListFilterPassThrough(t *testing.T) {
	rating := 4
	shopID := uid.N(7)
	userID := uid.N(42)
	want := usecase.ReviewListFilter{Rating: &rating, Keyword: "tasty", ShopID: &shopID, UserID: &userID}
	var got usecase.ReviewListFilter
	query := &fakeReviewQuery{
		listReviews: func(_ context.Context, filter usecase.ReviewListFilter, _, _ int32) ([]domain.ReviewDetail, bool, error) {
			got = filter
			return []domain.ReviewDetail{}, false, nil
		},
	}
	if _, _, err := newReviews(query, &fakeReviewRepo{}, &fakePhotoStorage{}).List(context.Background(), nil, want, 1, 20); err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filter = %+v, want %+v", got, want)
	}
}

// TestReviewsListFailure：query の失敗は wrap されて伝播する。
func TestReviewsListFailure(t *testing.T) {
	query := &fakeReviewQuery{
		listReviews: func(_ context.Context, _ usecase.ReviewListFilter, _, _ int32) ([]domain.ReviewDetail, bool, error) {
			return nil, false, io.ErrUnexpectedEOF
		},
	}
	if _, _, err := newReviews(query, &fakeReviewRepo{}, &fakePhotoStorage{}).List(context.Background(), nil, usecase.ReviewListFilter{}, 1, 20); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("List error = %v, want %v", err, io.ErrUnexpectedEOF)
	}
}

// TestReviewsGet は詳細の use case を扱う。見つかった review はそのまま通り、
// 存在しない、または discard 済みの review は domain.ErrReviewNotFound を
// 返す。
func TestReviewsGet(t *testing.T) {
	detail := domain.ReviewDetail{
		Review: domain.Review{ID: uid.N(9), Rating: 4, AuthorID: uid.N(1), BurgerID: uid.N(5), CreatedAt: time.Now()},
		User:   &domain.UserRef{ID: uid.N(1), Username: "alice"},
		Burger: &domain.ShopReviewBurger{ID: uid.N(5), Name: "Cheese"},
	}
	query := &fakeReviewQuery{
		getReview: func(_ context.Context, id string) (domain.ReviewDetail, error) {
			if id == detail.ID {
				return detail, nil
			}
			return domain.ReviewDetail{}, domain.ErrReviewNotFound
		},
	}
	reviews := newReviews(query, &fakeReviewRepo{}, &fakePhotoStorage{})

	got, err := reviews.Get(context.Background(), nil, detail.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if !reflect.DeepEqual(got, detail) {
		t.Errorf("Get = %+v, want %+v", got, detail)
	}

	if _, err := reviews.Get(context.Background(), nil, uid.N(999)); !errors.Is(err, domain.ErrReviewNotFound) {
		t.Fatalf("Get error = %v, want %v", err, domain.ErrReviewNotFound)
	}
}

// TestReviewsCreate は投稿のフローと、その厳密なチェック順序を扱う。
// shop 404 → reviewable 403 → burger 404 → validation 422 → insert である。
// fake の未設定の振る舞いにより、順序どおりでない呼び出しは panic になる。
func TestReviewsCreate(t *testing.T) {
	alice := domain.User{ID: uid.N(1), Username: "alice"}
	bob := domain.User{ID: uid.N(2), Username: "bob"}
	admin := domain.User{ID: uid.N(3), Username: "root", Admin: true}
	ctx := context.Background()

	activeShop := domain.Shop{ID: uid.N(10), Status: domain.ShopStatusActive}
	pendingShop := domain.Shop{ID: uid.N(11), Status: domain.ShopStatusPending, CreatorID: strPtr(alice.ID)}
	rejectedShop := domain.Shop{ID: uid.N(12), Status: domain.ShopStatusRejected, CreatorID: strPtr(alice.ID)}
	cheese := domain.ShopReviewBurger{ID: uid.N(5), Name: "Cheese", AverageRating: 4.5, ReviewCount: 2, WeightedScore: 4.1, Confidence: 0.8}

	getShop := func(_ context.Context, id string) (domain.Shop, error) {
		for _, shop := range []domain.Shop{activeShop, pendingShop, rejectedShop} {
			if shop.ID == id {
				return shop, nil
			}
		}
		return domain.Shop{}, domain.ErrShopNotFound
	}
	getShopBurger := func(_ context.Context, shopID, burgerID string) (domain.ShopReviewBurger, error) {
		if burgerID == cheese.ID {
			return cheese, nil
		}
		return domain.ShopReviewBurger{}, domain.ErrBurgerNotFound
	}

	t.Run("認証済みのユーザーが承認済みのショップに投稿すると、レビューが保存され、保存する内容が正しく組み立てられる", func(t *testing.T) {
		var inserted domain.Review
		query := &fakeReviewQuery{getShop: getShop, getShopBurger: getShopBurger}
		repo := &fakeReviewRepo{
			createReview: func(_ context.Context, review domain.Review) (domain.Review, error) {
				inserted = review
				review.ID = uid.N(42)
				review.CreatedAt = time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
				return review, nil
			},
		}
		got, err := newReviews(query, repo, &fakePhotoStorage{}).Create(ctx, bob, activeShop.ID, cheese.ID, "", 4, "Tasty", nil)
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if inserted.AuthorID != bob.ID || inserted.BurgerID != cheese.ID || inserted.Rating != 4 {
			t.Errorf("inserted = %+v, want author %s, burger %s, rating 4", inserted, bob.ID, cheese.ID)
		}
		if got.ID != uid.N(42) || got.Rating != 4 || got.Comment == nil || *got.Comment != "Tasty" {
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
		query := &fakeReviewQuery{getShop: getShop}
		if _, err := newReviews(query, &fakeReviewRepo{}, &fakePhotoStorage{}).Create(ctx, bob, uid.N(999), cheese.ID, "", 4, "ok", nil); !errors.Is(err, domain.ErrShopNotFound) {
			t.Fatalf("Create error = %v, want %v", err, domain.ErrShopNotFound)
		}
	})

	t.Run("却下済みのショップへの投稿は、バーガーを探す前に、誰であっても ErrForbidden になる", func(t *testing.T) {
		query := &fakeReviewQuery{getShop: getShop} // getShopBurger は未設定：lookup があれば panic する
		for _, viewer := range []domain.User{alice, bob, admin} {
			if _, err := newReviews(query, &fakeReviewRepo{}, &fakePhotoStorage{}).Create(ctx, viewer, rejectedShop.ID, cheese.ID, "", 4, "ok", nil); !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("viewer %s: error = %v, want %v", viewer.Username, err, domain.ErrForbidden)
			}
		}
	})

	t.Run("承認待ちのショップには、作成者と管理者だけが投稿でき、それ以外は ErrForbidden になる", func(t *testing.T) {
		query := &fakeReviewQuery{getShop: getShop, getShopBurger: getShopBurger}
		repo := &fakeReviewRepo{
			createReview: func(_ context.Context, review domain.Review) (domain.Review, error) {
				review.ID = uid.N(43)
				return review, nil
			},
		}
		reviews := newReviews(query, repo, &fakePhotoStorage{})
		if _, err := reviews.Create(ctx, alice, pendingShop.ID, cheese.ID, "", 4, "ok", nil); err != nil {
			t.Errorf("creator: Create returned error: %v", err)
		}
		if _, err := reviews.Create(ctx, admin, pendingShop.ID, cheese.ID, "", 4, "ok", nil); err != nil {
			t.Errorf("admin: Create returned error: %v", err)
		}
		if _, err := newReviews(&fakeReviewQuery{getShop: getShop}, &fakeReviewRepo{}, &fakePhotoStorage{}).Create(ctx, bob, pendingShop.ID, cheese.ID, "", 4, "ok", nil); !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("other user: error = %v, want %v", err, domain.ErrForbidden)
		}
	})

	t.Run("shop に紐付かない burger は insert せずに ErrBurgerNotFound を返す", func(t *testing.T) {
		query := &fakeReviewQuery{getShop: getShop, getShopBurger: getShopBurger}
		repo := &fakeReviewRepo{} // createReview は未設定
		if _, err := newReviews(query, repo, &fakePhotoStorage{}).Create(ctx, bob, activeShop.ID, uid.N(999), "", 4, "ok", nil); !errors.Is(err, domain.ErrBurgerNotFound) {
			t.Fatalf("Create error = %v, want %v", err, domain.ErrBurgerNotFound)
		}
	})

	t.Run("不正な内容(評価が範囲外、コメントが長すぎる)の投稿は、保存せずに ValidationError になる", func(t *testing.T) {
		query := &fakeReviewQuery{getShop: getShop, getShopBurger: getShopBurger}
		repo := &fakeReviewRepo{} // createReview は未設定
		tooLongComment := strings.Repeat("a", domain.MaxCommentChars+1)
		_, err := newReviews(query, repo, &fakePhotoStorage{}).Create(ctx, bob, activeShop.ID, cheese.ID, "", 0, tooLongComment, nil)
		var vErr *domain.ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("error = %v, want *domain.ValidationError", err)
		}
		want := []string{"Rating must be in 1..5", "Comment is too long (maximum is 2000 characters)"}
		if !reflect.DeepEqual(vErr.Texts(domain.LangEN), want) {
			t.Errorf("messages = %v, want %v", vErr.Texts(domain.LangEN), want)
		}
	})

	t.Run("バーガー名を指定して投稿すると、名前のバーガーを探して(なければ作って)からレビューを登録し、応答の内容を組み立てる", func(t *testing.T) {
		smash := domain.ShopReviewBurger{ID: uid.N(7), Name: " Smash "}
		var gotShopID string
		var gotName string
		var gotReview domain.Review
		query := &fakeReviewQuery{getShop: getShop} // getShopBurger は未設定：呼び出しは panic する
		repo := &fakeReviewRepo{
			createShopBurger: func(_ context.Context, shopID string, burgerName string) (domain.ShopReviewBurger, error) {
				gotShopID, gotName = shopID, burgerName
				return smash, nil
			},
			createReview: func(_ context.Context, review domain.Review) (domain.Review, error) {
				gotReview = review
				review.ID = uid.N(44)
				return review, nil
			},
		}
		// バーガー名は、前後の空白を取り除かれないまま、そのまま書き込み側に渡る。
		got, err := newReviews(query, repo, &fakePhotoStorage{}).Create(ctx, bob, activeShop.ID, "", " Smash ", 4, "Juicy", nil)
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if gotShopID != activeShop.ID || gotName != " Smash " {
			t.Errorf("repo got shop %s name %q, want %s %q", gotShopID, gotName, activeShop.ID, " Smash ")
		}
		if gotReview.AuthorID != bob.ID || gotReview.Rating != 4 || gotReview.BurgerID != smash.ID {
			t.Errorf("登録されたレビュー = %+v, want 投稿者 %s・評価 4・解決したバーガー %s", gotReview, bob.ID, smash.ID)
		}
		if got.ID != uid.N(44) || got.BurgerID != smash.ID {
			t.Errorf("detail review = %+v, want id 44 for burger %s", got.Review, smash.ID)
		}
		if !reflect.DeepEqual(got.Burger, &smash) {
			t.Errorf("burger = %+v, want %+v", got.Burger, smash)
		}
	})

	t.Run("バーガーの id(burger_id)が指定されていれば、バーガー名(burger_name)より優先される", func(t *testing.T) {
		query := &fakeReviewQuery{getShop: getShop, getShopBurger: getShopBurger}
		// バーガー名の解決(createShopBurger)は代役に設定していない。ID の指定が優先されるので、
		// 呼ばれると panic してテストが失敗する。
		repo := &fakeReviewRepo{
			createReview: func(_ context.Context, review domain.Review) (domain.Review, error) {
				review.ID = uid.N(45)
				return review, nil
			},
		}
		got, err := newReviews(query, repo, &fakePhotoStorage{}).Create(ctx, bob, activeShop.ID, cheese.ID, "Ignored", 4, "ok", nil)
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
				query := &fakeReviewQuery{getShop: getShop}
				repo := &fakeReviewRepo{} // すべての書き込みは未設定：呼び出しは panic する
				_, err := newReviews(query, repo, &fakePhotoStorage{}).Create(ctx, bob, activeShop.ID, "", burgerName, 4, "ok", nil)
				var vErr *domain.ValidationError
				if !errors.As(err, &vErr) {
					t.Fatalf("error = %v, want *domain.ValidationError", err)
				}
				if want := []string{"Burger name can't be blank"}; !reflect.DeepEqual(vErr.Texts(domain.LangEN), want) {
					t.Errorf("messages = %v, want %v", vErr.Texts(domain.LangEN), want)
				}
			})
		}
	})

	t.Run("burger_name 経由では書き込みの前に内容を validate する", func(t *testing.T) {
		query := &fakeReviewQuery{getShop: getShop}
		repo := &fakeReviewRepo{} // バーガー名の解決(createShopBurger)は代役に設定していない。書き込みの前に検証で失敗するので、呼ばれない
		tooLongComment := strings.Repeat("a", domain.MaxCommentChars+1)
		_, err := newReviews(query, repo, &fakePhotoStorage{}).Create(ctx, bob, activeShop.ID, "", "Smash", 0, tooLongComment, nil)
		var vErr *domain.ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("error = %v, want *domain.ValidationError", err)
		}
		want := []string{"Rating must be in 1..5", "Comment is too long (maximum is 2000 characters)"}
		if !reflect.DeepEqual(vErr.Texts(domain.LangEN), want) {
			t.Errorf("messages = %v, want %v", vErr.Texts(domain.LangEN), want)
		}
	})
}

// reviewDetailFor は、edit/delete のテストが load する、保存済みの detail を
// 組み立てる。
func reviewDetailFor(authorID string) domain.ReviewDetail {
	comment := "Old"
	return domain.ReviewDetail{
		Review: domain.Review{ID: uid.N(9), Rating: 2, Comment: &comment, AuthorID: authorID, BurgerID: uid.N(5),
			CreatedAt: time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)},
		User:   &domain.UserRef{ID: authorID, Username: "alice"},
		Burger: &domain.ShopReviewBurger{ID: uid.N(5), Name: "Cheese", AverageRating: 4.5, ReviewCount: 2},
	}
}

// TestReviewsUpdate は編集のフローを扱う。投稿者のみ（管理者でも
// 通らない）、書き込みの前の validation、カラム限定の書き込みそのもの
// （discardReview は未設定のままなので、どの discard も panic する）、
// そして 404 である。
func TestReviewsUpdate(t *testing.T) {
	alice := domain.User{ID: uid.N(1), Username: "alice"}
	bob := domain.User{ID: uid.N(2), Username: "bob"}
	admin := domain.User{ID: uid.N(3), Username: "root", Admin: true}
	ctx := context.Background()
	stored := reviewDetailFor(alice.ID)
	getReview := func(_ context.Context, id string) (domain.ReviewDetail, error) {
		if id == stored.ID {
			return stored, nil
		}
		return domain.ReviewDetail{}, domain.ErrReviewNotFound
	}

	t.Run("投稿者による編集は、評価とコメントだけを保存する", func(t *testing.T) {
		var gotID string
		var gotRating int
		var gotComment string
		query := &fakeReviewQuery{getReview: getReview}
		repo := &fakeReviewRepo{
			updateReviewContent: func(_ context.Context, id string, rating int, comment string) (domain.Review, error) {
				gotID, gotRating, gotComment = id, rating, comment
				updated := stored.Review
				updated.Rating = rating
				c := comment
				updated.Comment = &c
				return updated, nil
			},
		}
		got, err := newReviews(query, repo, &fakePhotoStorage{}).Update(ctx, alice, stored.ID, 5, "Better", nil)
		if err != nil {
			t.Fatalf("Update returned error: %v", err)
		}
		if gotID != stored.ID || gotRating != 5 || gotComment != "Better" {
			t.Errorf("write = (%s, %d, %q), want (%s, 5, Better)", gotID, gotRating, gotComment, stored.ID)
		}
		if got.Rating != 5 || got.Comment == nil || *got.Comment != "Better" {
			t.Errorf("detail = %+v, want rating 5, comment Better", got.Review)
		}
		if !reflect.DeepEqual(got.User, stored.User) || !reflect.DeepEqual(got.Burger, stored.Burger) {
			t.Errorf("user/burger = %+v/%+v, want kept from the load", got.User, got.Burger)
		}
	})

	t.Run("投稿者以外(管理者を含む)の編集は、何も書き込まずに ErrForbidden になる", func(t *testing.T) {
		query := &fakeReviewQuery{getReview: getReview}
		repo := &fakeReviewRepo{}                          // updateReviewContent は未設定
		for _, viewer := range []domain.User{bob, admin} { // admin でも通さない
			if _, err := newReviews(query, repo, &fakePhotoStorage{}).Update(ctx, viewer, stored.ID, 5, "x", nil); !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("viewer %s: error = %v, want %v", viewer.Username, err, domain.ErrForbidden)
			}
		}
	})

	t.Run("不正な内容での編集は、何も書き込まずに ValidationError になる", func(t *testing.T) {
		query := &fakeReviewQuery{getReview: getReview}
		_, err := newReviews(query, &fakeReviewRepo{}, &fakePhotoStorage{}).Update(ctx, alice, stored.ID, 6, "ok", nil)
		var vErr *domain.ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("error = %v, want *domain.ValidationError", err)
		}
		if want := []string{"Rating must be in 1..5"}; !reflect.DeepEqual(vErr.Texts(domain.LangEN), want) {
			t.Errorf("messages = %v, want %v", vErr.Texts(domain.LangEN), want)
		}
	})

	t.Run("未知の id は ErrReviewNotFound を返す", func(t *testing.T) {
		query := &fakeReviewQuery{getReview: getReview}
		if _, err := newReviews(query, &fakeReviewRepo{}, &fakePhotoStorage{}).Update(ctx, alice, uid.N(999), 5, "x", nil); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Fatalf("Update error = %v, want %v", err, domain.ErrReviewNotFound)
		}
	})
}

// TestReviewsDelete は論理削除（soft delete）のフローを扱う。投稿者のみ、
// load した review そのものに対して discard が呼ばれること
// （updateReviewContent は未設定のままなので、どの content の書き込みも
// panic する）、そして 404 である。
func TestReviewsDelete(t *testing.T) {
	alice := domain.User{ID: uid.N(1), Username: "alice"}
	bob := domain.User{ID: uid.N(2), Username: "bob"}
	admin := domain.User{ID: uid.N(3), Username: "root", Admin: true}
	ctx := context.Background()
	stored := reviewDetailFor(alice.ID)
	getReview := func(_ context.Context, id string) (domain.ReviewDetail, error) {
		if id == stored.ID {
			return stored, nil
		}
		return domain.ReviewDetail{}, domain.ErrReviewNotFound
	}

	t.Run("投稿者による削除は、そのレビューを論理削除(discard)する", func(t *testing.T) {
		var discarded string
		query := &fakeReviewQuery{getReview: getReview}
		repo := &fakeReviewRepo{
			discardReview: func(_ context.Context, id string) error {
				discarded = id
				return nil
			},
		}
		if err := newReviews(query, repo, &fakePhotoStorage{}).Delete(ctx, alice, stored.ID); err != nil {
			t.Fatalf("Delete returned error: %v", err)
		}
		if discarded != stored.ID {
			t.Errorf("discarded id = %s, want %s", discarded, stored.ID)
		}
	})

	t.Run("投稿者以外(管理者を含む)の削除は、何も書き込まずに ErrForbidden になる", func(t *testing.T) {
		query := &fakeReviewQuery{getReview: getReview}
		repo := &fakeReviewRepo{}                          // discardReview は未設定
		for _, viewer := range []domain.User{bob, admin} { // admin でも通さない
			if err := newReviews(query, repo, &fakePhotoStorage{}).Delete(ctx, viewer, stored.ID); !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("viewer %s: error = %v, want %v", viewer.Username, err, domain.ErrForbidden)
			}
		}
	})

	t.Run("未知の id は ErrReviewNotFound を返す", func(t *testing.T) {
		query := &fakeReviewQuery{getReview: getReview}
		if err := newReviews(query, &fakeReviewRepo{}, &fakePhotoStorage{}).Delete(ctx, alice, uid.N(999)); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Fatalf("Delete error = %v, want %v", err, domain.ErrReviewNotFound)
		}
	})

	t.Run("discard の競合が起きたときは repository の ErrReviewNotFound が返る", func(t *testing.T) {
		query := &fakeReviewQuery{getReview: getReview}
		repo := &fakeReviewRepo{
			discardReview: func(_ context.Context, _ string) error {
				return domain.ErrReviewNotFound // load から書き込みまでの間に discard された
			},
		}
		if err := newReviews(query, repo, &fakePhotoStorage{}).Delete(ctx, alice, stored.ID); !errors.Is(err, domain.ErrReviewNotFound) {
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
	newReviews(&fakeReviewQuery{}, &fakeReviewRepo{}, nil)
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

// TestReviewsCreatePhoto は Create の写真の流れを扱う。写真の実体（blob）は insert の
// 前にランダムな reviews/<hex><ext> の key で保存され、その key は review に
// 永続化され、insert が失敗した場合はアップロードしたばかりの blob を
// best-effort で削除するので、孤立ファイルは残らない。
func TestReviewsCreatePhoto(t *testing.T) {
	ctx := context.Background()
	bob := domain.User{ID: uid.N(2), Username: "bob"}
	activeShop := domain.Shop{ID: uid.N(10), Status: domain.ShopStatusActive}
	cheese := domain.ShopReviewBurger{ID: uid.N(5), Name: "Cheese"}
	upload := &photo.Processed{Data: []byte("img"), ContentType: "image/jpeg", Ext: ".jpg"}
	getShop := func(_ context.Context, id string) (domain.Shop, error) { return activeShop, nil }
	getShopBurger := func(_ context.Context, _, _ string) (domain.ShopReviewBurger, error) { return cheese, nil }

	t.Run("blob を保存し、その key を永続化する", func(t *testing.T) {
		photos := &fakePhotoStorage{}
		var inserted domain.Review
		query := &fakeReviewQuery{getShop: getShop, getShopBurger: getShopBurger}
		repo := &fakeReviewRepo{
			createReview: func(_ context.Context, review domain.Review) (domain.Review, error) {
				inserted = review
				review.ID = uid.N(42)
				return review, nil
			},
		}
		got, err := newReviews(query, repo, photos).Create(ctx, bob, activeShop.ID, cheese.ID, "", 4, "Tasty", upload)
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
		query := &fakeReviewQuery{getShop: getShop, getShopBurger: getShopBurger}
		repo := &fakeReviewRepo{
			createReview: func(_ context.Context, _ domain.Review) (domain.Review, error) {
				return domain.Review{}, io.ErrUnexpectedEOF
			},
		}
		_, err := newReviews(query, repo, photos).Create(ctx, bob, activeShop.ID, cheese.ID, "", 4, "Tasty", upload)
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("Create error = %v, want the DB error", err)
		}
		if len(photos.puts) != 1 || !reflect.DeepEqual(photos.deletes, photos.puts) {
			t.Errorf("puts/deletes = %v/%v, want the uploaded key deleted", photos.puts, photos.deletes)
		}
	})

	t.Run("Put が失敗したら insert せずにそのエラーを返す", func(t *testing.T) {
		photos := &fakePhotoStorage{putErr: io.ErrUnexpectedEOF}
		query := &fakeReviewQuery{getShop: getShop, getShopBurger: getShopBurger}
		repo := &fakeReviewRepo{} // createReview は未設定：insert があれば panic する
		if _, err := newReviews(query, repo, photos).Create(ctx, bob, activeShop.ID, cheese.ID, "", 4, "Tasty", upload); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("Create error = %v, want the storage error", err)
		}
	})
}

// TestReviewsUpdatePhoto は Update の写真の置き換えの流れを扱う。新しい blob が
// 先に保存され、次に content と photo_key が「1 つの」atomic な repository の
// 書き込みで切り替わり、そのあとではじめて「古い」blob（load のスナップ
// ショットのもの）が best-effort で削除される。upload がなければ content
// だけの書き込みが走り、写真の書き込みは決して起こらない。
func TestReviewsUpdatePhoto(t *testing.T) {
	ctx := context.Background()
	alice := domain.User{ID: uid.N(1), Username: "alice"}
	oldKey := "reviews/old.jpg"
	stored := domain.ReviewDetail{
		Review: domain.Review{ID: uid.N(9), Rating: 4, AuthorID: alice.ID, BurgerID: uid.N(5), PhotoKey: &oldKey},
		User:   &domain.UserRef{ID: alice.ID, Username: "alice"},
		Burger: &domain.ShopReviewBurger{ID: uid.N(5), Name: "Cheese"},
	}
	getReview := func(_ context.Context, id string) (domain.ReviewDetail, error) { return stored, nil }
	upload := &photo.Processed{Data: []byte("img"), ContentType: "image/png", Ext: ".png"}

	t.Run("content と key を atomic に置き換え、DB 成功後に古い blob を削除する", func(t *testing.T) {
		photos := &fakePhotoStorage{}
		var gotRating int
		var gotComment string
		query := &fakeReviewQuery{getReview: getReview}
		repo := &fakeReviewRepo{
			// updateReviewContent は未設定のままである。写真の経路で
			// content だけの別の文を実行すれば panic する（書き込みは
			// 単一の atomic な repository の呼び出しでなければならない）。
			updateReviewContentAndKey: func(_ context.Context, id string, rating int, comment string, photoKey *string) (domain.Review, error) {
				gotRating, gotComment = rating, comment
				review := stored.Review
				review.Rating = rating
				c := comment
				review.Comment = &c
				review.PhotoKey = photoKey
				return review, nil
			},
		}
		got, err := newReviews(query, repo, photos).Update(ctx, alice, stored.ID, 5, "Better", upload)
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
		query := &fakeReviewQuery{getReview: getReview}
		repo := &fakeReviewRepo{
			// updateReviewContent は未設定のままである。atomic な書き込みが
			// 失敗したということは content の文が「一切」実行されていない
			// ので、あとから content の変更が見えることはありえない。
			// 紛れ込んだ content だけの呼び出しは panic する。
			updateReviewContentAndKey: func(_ context.Context, _ string, _ int, _ string, _ *string) (domain.Review, error) {
				return domain.Review{}, domain.ErrReviewNotFound // load から書き込みまでの間に discard された
			},
		}
		_, err := newReviews(query, repo, photos).Update(ctx, alice, stored.ID, 5, "Better", upload)
		if !errors.Is(err, domain.ErrReviewNotFound) {
			t.Fatalf("Update error = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if !reflect.DeepEqual(photos.deletes, photos.puts) {
			t.Errorf("puts/deletes = %v/%v, want the new blob deleted and the old kept", photos.puts, photos.deletes)
		}
	})

	t.Run("upload がなければ content だけの書き込みになり、photo storage には触れない", func(t *testing.T) {
		photos := &fakePhotoStorage{}
		query := &fakeReviewQuery{getReview: getReview}
		repo := &fakeReviewRepo{
			// updateReviewContentAndKey は未設定：photo_key のどんな書き込みも
			// panic する。
			updateReviewContent: func(_ context.Context, _ string, _ int, _ string) (domain.Review, error) {
				return stored.Review, nil
			},
		}
		got, err := newReviews(query, repo, photos).Update(ctx, alice, stored.ID, 5, "Better", nil)
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

// TestReviewsDeletePhoto は Delete の、写真を削除する部分を扱う。discard が成功
// した後に、blob は best-effort で削除される。
func TestReviewsDeletePhoto(t *testing.T) {
	ctx := context.Background()
	alice := domain.User{ID: uid.N(1), Username: "alice"}
	key := "reviews/gone.jpg"
	photos := &fakePhotoStorage{}
	query := &fakeReviewQuery{
		getReview: func(_ context.Context, id string) (domain.ReviewDetail, error) {
			return domain.ReviewDetail{Review: domain.Review{ID: id, AuthorID: alice.ID, PhotoKey: &key}}, nil
		},
	}
	repo := &fakeReviewRepo{
		discardReview: func(_ context.Context, _ string) error { return nil },
	}
	if err := newReviews(query, repo, photos).Delete(ctx, alice, uid.N(9)); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if !reflect.DeepEqual(photos.deletes, []string{key}) {
		t.Errorf("deletes = %v, want %q", photos.deletes, key)
	}
}

// TestReviewsGetCanEdit は、詳細の CanEdit が domain の所有権ルール（author だけ。
// admin にも例外なし。匿名は false）どおりに設定されることを固定する。
func TestReviewsGetCanEdit(t *testing.T) {
	author := domain.User{ID: uid.N(1), Username: "alice"}
	other := domain.User{ID: uid.N(2), Username: "bob"}
	admin := domain.User{ID: uid.N(3), Username: "root", Admin: true}
	detail := domain.ReviewDetail{Review: domain.Review{ID: uid.N(9), Rating: 4, AuthorID: author.ID, BurgerID: uid.N(5)}}
	query := &fakeReviewQuery{
		getReview: func(context.Context, string) (domain.ReviewDetail, error) { return detail, nil },
	}
	reviews := newReviews(query, &fakeReviewRepo{}, &fakePhotoStorage{})

	tests := []struct {
		name   string
		viewer *domain.User
		want   bool
	}{
		{name: "匿名は false", viewer: nil, want: false},
		{name: "author は true", viewer: &author, want: true},
		{name: "他のユーザーは false", viewer: &other, want: false},
		{name: "admin でも他人の review は false", viewer: &admin, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reviews.Get(context.Background(), tt.viewer, detail.ID)
			if err != nil {
				t.Fatalf("Get returned error: %v", err)
			}
			if got.CanEdit != tt.want {
				t.Errorf("CanEdit = %v, want %v", got.CanEdit, tt.want)
			}
		})
	}
}

// TestReviewsListCanEditAndHasMore は、一覧の各 review の CanEdit が viewer ごとに
// 設定されること（author の review だけ true）と、次のページの有無（has_more）が
// query の判定のまま返ることを固定する。
func TestReviewsListCanEditAndHasMore(t *testing.T) {
	author := domain.User{ID: uid.N(1), Username: "alice"}
	feed := []domain.ReviewDetail{
		{Review: domain.Review{ID: uid.N(3), AuthorID: uid.N(2)}},
		{Review: domain.Review{ID: uid.N(2), AuthorID: author.ID}},
	}
	for _, hasMore := range []bool{true, false} {
		query := &fakeReviewQuery{
			listReviews: func(context.Context, usecase.ReviewListFilter, int32, int32) ([]domain.ReviewDetail, bool, error) {
				return append([]domain.ReviewDetail(nil), feed...), hasMore, nil
			},
		}
		reviews := newReviews(query, &fakeReviewRepo{}, &fakePhotoStorage{})

		got, gotHasMore, err := reviews.List(context.Background(), &author, usecase.ReviewListFilter{}, 1, 20)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if gotHasMore != hasMore {
			t.Errorf("hasMore = %v, want %v", gotHasMore, hasMore)
		}
		if len(got) != 2 || got[0].CanEdit || !got[1].CanEdit {
			t.Errorf("CanEdit = [%v %v], want [false true]", got[0].CanEdit, got[1].CanEdit)
		}

		anon, _, err := reviews.List(context.Background(), nil, usecase.ReviewListFilter{}, 1, 20)
		if err != nil {
			t.Fatalf("List (anonymous) returned error: %v", err)
		}
		if anon[0].CanEdit || anon[1].CanEdit {
			t.Errorf("anonymous CanEdit = [%v %v], want [false false]", anon[0].CanEdit, anon[1].CanEdit)
		}
	}
}
