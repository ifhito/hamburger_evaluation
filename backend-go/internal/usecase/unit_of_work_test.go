package usecase_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/photo"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uowtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// uowReviews は、フェイクの UnitOfWork（commit / rollback と、統計の呼び出しの順序を記録する）を
// 使った usecase.Reviews と、その UnitOfWork を返す。
func uowReviews(query usecase.ReviewQuery, repo domain.ReviewRepository, uow *uowtest.UoW) *usecase.Reviews {
	uow.Reviews = repo
	return usecase.NewReviews(query, uow, usecase.NewBurgerStatsRecalculator(uowtest.Clock{}), &fakePhotoStorage{})
}

// processedIf は、写真つきの経路を試すための処理済みの upload（upload が false なら nil）を返す。
func processedIf(upload bool) *photo.Processed {
	if !upload {
		return nil
	}
	return &photo.Processed{Data: []byte("img"), ContentType: "image/jpeg", Ext: ".jpg"}
}

// TestReviewsWriteRegistersRecalcRequestInOneTransaction は、レビューの書き込みが、統計を計算せず、
// 同じトランザクション(UnitOfWork.Do)の中で、そのバーガーの再計算の依頼を登録するだけであることを、
// DB なしに固定する。書き込みが失敗したときは、依頼も登録されずに rollback される。
func TestReviewsWriteRegistersRecalcRequestInOneTransaction(t *testing.T) {
	ctx := context.Background()
	alice := domain.User{ID: uid.N(1), Username: "alice"}
	stored := reviewDetailFor(alice.ID) // burger 5 の review 9
	getReview := func(_ context.Context, id int64) (domain.ReviewDetail, error) {
		if id == stored.ID {
			return stored, nil
		}
		return domain.ReviewDetail{}, domain.ErrReviewNotFound
	}
	activeShop := domain.Shop{ID: uid.N(1), Name: "Active Diner", Status: domain.ShopStatusActive}
	cheese := domain.ShopReviewBurger{ID: uid.N(5), Name: "Cheese"}
	createQuery := &fakeReviewQuery{
		getShop:       func(context.Context, string) (domain.Shop, error) { return activeShop, nil },
		getShopBurger: func(context.Context, string, string) (domain.ShopReviewBurger, error) { return cheese, nil },
	}

	t.Run("削除は、該当のバーガーの再計算の依頼だけを登録し、統計の計算も burger のロックもせずに commit する", func(t *testing.T) {
		stats := &uowtest.Stats{}
		uow := &uowtest.UoW{Stats: stats}
		repo := &fakeReviewRepo{discardReview: func(context.Context, int64) error { stats.Note("discard"); return nil }}
		if err := uowReviews(&fakeReviewQuery{getReview: getReview}, repo, uow).Delete(ctx, alice, stored.ID); err != nil {
			t.Fatalf("Delete returned error: %v", err)
		}
		want := []string{"discard", "request:" + uid.N(5)}
		if !reflect.DeepEqual(stats.Ops, want) {
			t.Errorf("操作の順序 = %v, want %v", stats.Ops, want)
		}
		if uow.Commits != 1 || uow.Rollbacks != 0 {
			t.Errorf("commit/rollback = %d/%d, want 1/0", uow.Commits, uow.Rollbacks)
		}
	})

	t.Run("存在しないレビューの削除は、再計算の依頼を登録せずに rollback する", func(t *testing.T) {
		stats := &uowtest.Stats{}
		uow := &uowtest.UoW{Stats: stats}
		repo := &fakeReviewRepo{discardReview: func(context.Context, int64) error {
			return domain.ErrReviewNotFound // 並行して先に削除された
		}}
		err := uowReviews(&fakeReviewQuery{getReview: getReview}, repo, uow).Delete(ctx, alice, stored.ID)
		if !errors.Is(err, domain.ErrReviewNotFound) {
			t.Fatalf("Delete error = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if len(stats.Ops) != 0 {
			t.Errorf("統計に触れた: %v, want none", stats.Ops)
		}
		if uow.Commits != 0 || uow.Rollbacks != 1 {
			t.Errorf("commit/rollback = %d/%d, want 0/1", uow.Commits, uow.Rollbacks)
		}
	})

	t.Run("再計算の依頼の登録が失敗すると、全体が失敗して commit されない", func(t *testing.T) {
		requestErr := errors.New("boom")
		stats := &uowtest.Stats{RequestErr: requestErr}
		uow := &uowtest.UoW{Stats: stats}
		repo := &fakeReviewRepo{discardReview: func(context.Context, int64) error { return nil }}
		err := uowReviews(&fakeReviewQuery{getReview: getReview}, repo, uow).Delete(ctx, alice, stored.ID)
		if !errors.Is(err, requestErr) {
			t.Fatalf("Delete error = %v, want wrapped %v", err, requestErr)
		}
		if uow.Commits != 0 || uow.Rollbacks != 1 {
			t.Errorf("commit/rollback = %d/%d, want 0/1（依頼を登録できないなら、書き込みも取り消す）", uow.Commits, uow.Rollbacks)
		}
	})

	t.Run("投稿(バーガーの id 指定)は、insert のあとに再計算の依頼を登録し、burger のロックはしない", func(t *testing.T) {
		stats := &uowtest.Stats{}
		uow := &uowtest.UoW{Stats: stats}
		repo := &fakeReviewRepo{createReview: func(_ context.Context, review domain.Review) (domain.Review, error) {
			stats.Note("insert")
			review.ID = 44
			return review, nil
		}}
		if _, err := uowReviews(createQuery, repo, uow).Create(ctx, alice, activeShop.ID, cheese.ID, "", 4, "ok", nil); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		// 書き込みは burger の行をロックしない(ロックは、ワーカーが再計算するときだけ取る)。
		want := []string{"insert", "request:" + uid.N(5)}
		if !reflect.DeepEqual(stats.Ops, want) {
			t.Errorf("操作の順序 = %v, want %v", stats.Ops, want)
		}
		if uow.Commits != 1 || uow.Rollbacks != 0 {
			t.Errorf("commit/rollback = %d/%d, want 1/0", uow.Commits, uow.Rollbacks)
		}
	})

	t.Run("投稿(バーガー名指定)は、なければ作ったバーガーに対して、insert のあとに再計算の依頼を登録する", func(t *testing.T) {
		stats := &uowtest.Stats{}
		uow := &uowtest.UoW{Stats: stats}
		repo := &fakeReviewRepo{
			createShopBurger: func(context.Context, string, string) (domain.ShopReviewBurger, error) {
				stats.Note("find-or-create")
				return domain.ShopReviewBurger{ID: uid.N(7), Name: "Smash"}, nil
			},
			createReview: func(_ context.Context, review domain.Review) (domain.Review, error) {
				stats.Note("insert")
				review.ID = 45
				return review, nil
			},
		}
		query := &fakeReviewQuery{getShop: createQuery.getShop}
		if _, err := uowReviews(query, repo, uow).Create(ctx, alice, activeShop.ID, "", "Smash", 4, "ok", nil); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		want := []string{"find-or-create", "insert", "request:" + uid.N(7)}
		if !reflect.DeepEqual(stats.Ops, want) {
			t.Errorf("操作の順序 = %v, want %v", stats.Ops, want)
		}
	})

	t.Run("insert が失敗すると、なければ作ったバーガーも含めて rollback し、再計算の依頼は登録しない", func(t *testing.T) {
		insertErr := errors.New("fk violation")
		stats := &uowtest.Stats{}
		uow := &uowtest.UoW{Stats: stats}
		repo := &fakeReviewRepo{
			createShopBurger: func(context.Context, string, string) (domain.ShopReviewBurger, error) {
				return domain.ShopReviewBurger{ID: uid.N(7), Name: "Ghost"}, nil
			},
			createReview: func(context.Context, domain.Review) (domain.Review, error) { return domain.Review{}, insertErr },
		}
		query := &fakeReviewQuery{getShop: createQuery.getShop}
		if _, err := uowReviews(query, repo, uow).Create(ctx, alice, activeShop.ID, "", "Ghost", 4, "ok", nil); !errors.Is(err, insertErr) {
			t.Fatalf("Create error = %v, want wrapped %v", err, insertErr)
		}
		if uow.Commits != 0 || uow.Rollbacks != 1 {
			t.Errorf("commit/rollback = %d/%d, want 0/1（孤立したバーガーを commit しない）", uow.Commits, uow.Rollbacks)
		}
		for _, op := range stats.Ops {
			if op == "request:"+uid.N(7) {
				t.Errorf("insert の失敗のあとに再計算の依頼を登録した: %v", stats.Ops)
			}
		}
	})

	t.Run("編集は、更新したレビューのバーガーの再計算の依頼を登録する(本文だけの経路と、写真つきの経路)", func(t *testing.T) {
		for name, upload := range map[string]bool{"本文だけ": false, "写真つき": true} {
			t.Run(name, func(t *testing.T) {
				stats := &uowtest.Stats{}
				uow := &uowtest.UoW{Stats: stats}
				updated := func(review domain.Review, rating int, comment string) domain.Review {
					review.Rating, review.Comment = rating, &comment
					return review
				}
				repo := &fakeReviewRepo{
					updateReviewContent: func(_ context.Context, _ int64, rating int, comment string) (domain.Review, error) {
						return updated(stored.Review, rating, comment), nil
					},
					updateReviewContentAndKey: func(_ context.Context, _ int64, rating int, comment string, _ *string) (domain.Review, error) {
						return updated(stored.Review, rating, comment), nil
					},
				}
				reviews := uowReviews(&fakeReviewQuery{getReview: getReview}, repo, uow)
				if _, err := reviews.Update(ctx, alice, stored.ID, 5, "Better", processedIf(upload)); err != nil {
					t.Fatalf("Update returned error: %v", err)
				}
				want := []string{"request:" + uid.N(5)}
				if !reflect.DeepEqual(stats.Ops, want) {
					t.Errorf("操作の順序 = %v, want %v", stats.Ops, want)
				}
				if uow.Commits != 1 {
					t.Errorf("commits = %d, want 1", uow.Commits)
				}
			})
		}
	})
}

// TestUsersDeleteRegistersRecalcRequestsInAscendingOrder は、退会が、ユーザーの discard と、そのユーザーの
// レビューが付くバーガーの再計算の依頼の登録を、1 つのトランザクションで、バーガーの id の昇順に行うことを
// 固定する(複数の行を、いつも同じ順序で登録すれば、並行する退会が互いを逆順に待つデッドロックが起きない)。
func TestUsersDeleteRegistersRecalcRequestsInAscendingOrder(t *testing.T) {
	ctx := context.Background()
	target := usersViewer
	query := &fakeUserQuery{getByID: activeUsersByID(target)}
	newUsersWithUoW := func(uow *uowtest.UoW, repo *fakeUserRepo) *usecase.Users {
		uow.Users = repo
		return usecase.NewUsers(query, domain.NewUsers(repo), uow, usecase.NewBurgerStatsRecalculator(uowtest.Clock{}), fakeHasher{})
	}

	t.Run("バーガーの id の昇順に依頼を登録して commit する(読み取りが昇順でなくても)", func(t *testing.T) {
		stats := &uowtest.Stats{ReviewedBy: func(context.Context, string) ([]string, error) { return []string{uid.N(9), uid.N(3), uid.N(5)}, nil }}
		uow := &uowtest.UoW{Stats: stats}
		repo := &fakeUserRepo{discard: func(context.Context, string) error { stats.Note("discard"); return nil }}
		if err := newUsersWithUoW(uow, repo).Delete(ctx, target, target.ID); err != nil {
			t.Fatalf("Delete returned error: %v", err)
		}
		want := []string{
			"discard", "reviewed-by:" + target.ID,
			"request:" + uid.N(3), "request:" + uid.N(5), "request:" + uid.N(9),
		}
		if !reflect.DeepEqual(stats.Ops, want) {
			t.Errorf("操作の順序 = %v, want %v", stats.Ops, want)
		}
		if uow.Commits != 1 || uow.Rollbacks != 0 {
			t.Errorf("commit/rollback = %d/%d, want 1/0", uow.Commits, uow.Rollbacks)
		}
	})

	t.Run("レビューのないユーザーの退会は、再計算の依頼を登録せずに commit する", func(t *testing.T) {
		stats := &uowtest.Stats{}
		uow := &uowtest.UoW{Stats: stats}
		repo := &fakeUserRepo{discard: func(context.Context, string) error { return nil }}
		if err := newUsersWithUoW(uow, repo).Delete(ctx, target, target.ID); err != nil {
			t.Fatalf("Delete returned error: %v", err)
		}
		if uow.Commits != 1 {
			t.Errorf("commits = %d, want 1", uow.Commits)
		}
		for _, op := range stats.Ops {
			if strings.HasPrefix(op, "request:") {
				t.Errorf("レビューのないユーザーで依頼を登録した: %v", stats.Ops)
			}
		}
	})

	t.Run("退会に失敗した(存在しない)ときは、再計算の依頼に触れずに rollback する", func(t *testing.T) {
		stats := &uowtest.Stats{}
		uow := &uowtest.UoW{Stats: stats}
		repo := &fakeUserRepo{discard: func(context.Context, string) error { return domain.ErrUserNotFound }}
		if err := newUsersWithUoW(uow, repo).Delete(ctx, target, target.ID); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("Delete error = %v, want %v", err, domain.ErrUserNotFound)
		}
		if len(stats.Ops) != 0 {
			t.Errorf("統計に触れた: %v, want none", stats.Ops)
		}
		if uow.Commits != 0 || uow.Rollbacks != 1 {
			t.Errorf("commit/rollback = %d/%d, want 0/1", uow.Commits, uow.Rollbacks)
		}
	})

	t.Run("依頼の登録が失敗すると、退会も含めて全体が失敗して commit されない", func(t *testing.T) {
		boom := errors.New("boom")
		stats := &uowtest.Stats{
			ReviewedBy: func(context.Context, string) ([]string, error) { return []string{uid.N(3), uid.N(5)}, nil },
			RequestErr: boom,
		}
		uow := &uowtest.UoW{Stats: stats}
		repo := &fakeUserRepo{discard: func(context.Context, string) error { return nil }}
		if err := newUsersWithUoW(uow, repo).Delete(ctx, target, target.ID); !errors.Is(err, boom) {
			t.Fatalf("Delete error = %v, want wrapped %v", err, boom)
		}
		if uow.Commits != 0 || uow.Rollbacks != 1 {
			t.Errorf("commit/rollback = %d/%d, want 0/1", uow.Commits, uow.Rollbacks)
		}
	})
}
