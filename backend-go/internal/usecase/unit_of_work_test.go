package usecase_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

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

// TestReviewsWriteRecalculatesInOneTransaction は、S17 AC2・AC4 を扱う。review の書き込みと burger の
// 統計の再計算が、1 つの UnitOfWork.Do（同一トランザクション）の中で組み立てられ、書き込みが失敗した
// ときは、再計算に触れずに rollback されることを、DB なしに固定する。
func TestReviewsWriteRecalculatesInOneTransaction(t *testing.T) {
	ctx := context.Background()
	alice := domain.User{ID: uid.N(1), Username: "alice"}
	stored := reviewDetailFor(alice.ID) // burger 5 の review 9
	getReview := func(_ context.Context, id string) (domain.ReviewDetail, error) {
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

	t.Run("delete は、該当の burger だけをロック → facts → 保存の順に再計算し、commit する", func(t *testing.T) {
		stats := &uowtest.Stats{}
		uow := &uowtest.UoW{Stats: stats}
		repo := &fakeReviewRepo{discardReview: func(context.Context, string) error { stats.Note("discard"); return nil }}
		if err := uowReviews(&fakeReviewQuery{getReview: getReview}, repo, uow).Delete(ctx, alice, stored.ID); err != nil {
			t.Fatalf("Delete returned error: %v", err)
		}
		want := []string{"discard", "lock:" + uid.N(5), "facts:" + uid.N(5), "save:" + uid.N(5)}
		if !reflect.DeepEqual(stats.Ops, want) {
			t.Errorf("操作の順序 = %v, want %v", stats.Ops, want)
		}
		if uow.Commits != 1 || uow.Rollbacks != 0 {
			t.Errorf("commit/rollback = %d/%d, want 1/0", uow.Commits, uow.Rollbacks)
		}
	})

	t.Run("存在しない review の delete は、再計算に触れずに rollback する", func(t *testing.T) {
		stats := &uowtest.Stats{}
		uow := &uowtest.UoW{Stats: stats}
		repo := &fakeReviewRepo{discardReview: func(context.Context, string) error {
			return domain.ErrReviewNotFound // 並行して先に discard された
		}}
		err := uowReviews(&fakeReviewQuery{getReview: getReview}, repo, uow).Delete(ctx, alice, stored.ID)
		if !errors.Is(err, domain.ErrReviewNotFound) {
			t.Fatalf("Delete error = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if len(stats.Ops) != 0 {
			t.Errorf("再計算に触れた: %v, want none", stats.Ops)
		}
		if uow.Commits != 0 || uow.Rollbacks != 1 {
			t.Errorf("commit/rollback = %d/%d, want 0/1", uow.Commits, uow.Rollbacks)
		}
	})

	t.Run("再計算の保存が失敗すると、全体が失敗して commit されない", func(t *testing.T) {
		saveErr := errors.New("boom")
		stats := &uowtest.Stats{SaveErr: saveErr}
		uow := &uowtest.UoW{Stats: stats}
		repo := &fakeReviewRepo{discardReview: func(context.Context, string) error { return nil }}
		err := uowReviews(&fakeReviewQuery{getReview: getReview}, repo, uow).Delete(ctx, alice, stored.ID)
		if !errors.Is(err, saveErr) {
			t.Fatalf("Delete error = %v, want wrapped %v", err, saveErr)
		}
		if uow.Commits != 0 || uow.Rollbacks != 1 {
			t.Errorf("commit/rollback = %d/%d, want 0/1", uow.Commits, uow.Rollbacks)
		}
	})

	t.Run("ロックの失敗も全体を失敗させ、review の書き込みは rollback される", func(t *testing.T) {
		lockErr := errors.New("lock timeout")
		stats := &uowtest.Stats{LockErr: lockErr}
		uow := &uowtest.UoW{Stats: stats}
		repo := &fakeReviewRepo{discardReview: func(context.Context, string) error { return nil }}
		if err := uowReviews(&fakeReviewQuery{getReview: getReview}, repo, uow).Delete(ctx, alice, stored.ID); !errors.Is(err, lockErr) {
			t.Fatalf("Delete error = %v, want wrapped %v", err, lockErr)
		}
		if uow.Commits != 0 || uow.Rollbacks != 1 {
			t.Errorf("commit/rollback = %d/%d, want 0/1", uow.Commits, uow.Rollbacks)
		}
	})

	t.Run("create(burger_id)は、insert の前に burger をロックし、insert の後に再計算する", func(t *testing.T) {
		stats := &uowtest.Stats{}
		uow := &uowtest.UoW{Stats: stats}
		repo := &fakeReviewRepo{createReview: func(_ context.Context, review domain.Review) (domain.Review, error) {
			stats.Note("insert")
			review.ID = uid.N(44)
			return review, nil
		}}
		if _, err := uowReviews(createQuery, repo, uow).Create(ctx, alice, activeShop.ID, cheese.ID, "", 4, "ok", nil); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		// insert の FK チェックが取る KEY SHARE を FOR UPDATE に昇格させると、並行する creator が
		// デッドロックしうるので、ロックは insert の前に取る。再計算の中のロックは再取得になる。
		want := []string{"lock:" + uid.N(5), "insert", "lock:" + uid.N(5), "facts:" + uid.N(5), "save:" + uid.N(5)}
		if !reflect.DeepEqual(stats.Ops, want) {
			t.Errorf("操作の順序 = %v, want %v", stats.Ops, want)
		}
		if uow.Commits != 1 || uow.Rollbacks != 0 {
			t.Errorf("commit/rollback = %d/%d, want 1/0", uow.Commits, uow.Rollbacks)
		}
	})

	t.Run("create(burger_name)は、burger の find-or-create の後に、その burger をロックしてから insert する", func(t *testing.T) {
		stats := &uowtest.Stats{}
		uow := &uowtest.UoW{Stats: stats}
		repo := &fakeReviewRepo{
			createShopBurger: func(context.Context, string, string) (domain.ShopReviewBurger, error) {
				stats.Note("find-or-create")
				return domain.ShopReviewBurger{ID: uid.N(7), Name: "Smash"}, nil
			},
			createReview: func(_ context.Context, review domain.Review) (domain.Review, error) {
				stats.Note("insert")
				review.ID = uid.N(45)
				return review, nil
			},
		}
		query := &fakeReviewQuery{getShop: createQuery.getShop}
		if _, err := uowReviews(query, repo, uow).Create(ctx, alice, activeShop.ID, "", "Smash", 4, "ok", nil); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		want := []string{"find-or-create", "lock:" + uid.N(7), "insert", "lock:" + uid.N(7), "facts:" + uid.N(7), "save:" + uid.N(7)}
		if !reflect.DeepEqual(stats.Ops, want) {
			t.Errorf("操作の順序 = %v, want %v", stats.Ops, want)
		}
	})

	t.Run("insert が失敗すると、find-or-create した burger も含めて rollback し、再計算はしない", func(t *testing.T) {
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
			t.Errorf("commit/rollback = %d/%d, want 0/1（孤立した burger を commit しない）", uow.Commits, uow.Rollbacks)
		}
		for _, op := range stats.Ops {
			if op == "facts:"+uid.N(7) || op == "save:"+uid.N(7) {
				t.Errorf("insert の失敗のあとに再計算した: %v", stats.Ops)
			}
		}
	})

	t.Run("update は、更新した review の burger を再計算する(content だけの経路と、写真つきの経路)", func(t *testing.T) {
		for name, upload := range map[string]bool{"content だけ": false, "写真つき": true} {
			t.Run(name, func(t *testing.T) {
				stats := &uowtest.Stats{}
				uow := &uowtest.UoW{Stats: stats}
				updated := func(review domain.Review, rating int, comment string) domain.Review {
					review.Rating, review.Comment = rating, &comment
					return review
				}
				repo := &fakeReviewRepo{
					updateReviewContent: func(_ context.Context, _ string, rating int, comment string) (domain.Review, error) {
						return updated(stored.Review, rating, comment), nil
					},
					updateReviewContentAndKey: func(_ context.Context, _ string, rating int, comment string, _ *string) (domain.Review, error) {
						return updated(stored.Review, rating, comment), nil
					},
				}
				reviews := uowReviews(&fakeReviewQuery{getReview: getReview}, repo, uow)
				if _, err := reviews.Update(ctx, alice, stored.ID, 5, "Better", processedIf(upload)); err != nil {
					t.Fatalf("Update returned error: %v", err)
				}
				want := []string{"lock:" + uid.N(5), "facts:" + uid.N(5), "save:" + uid.N(5)}
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

// TestBurgerStatsRecalculationUsesClockAndDomain は、S17 AC5 を扱う。再計算に使う時刻は Clock から得て、
// 保存される統計が、domain の計算(CalculateBurgerStat)を、その時刻で行った結果と一致する。
func TestBurgerStatsRecalculationUsesClockAndDomain(t *testing.T) {
	ctx := context.Background()
	alice := domain.User{ID: uid.N(1), Username: "alice"}
	stored := reviewDetailFor(alice.ID)
	// マイクロ秒より細かい部分は切り詰められる（timestamptz の精度）。
	clockTime := time.Date(2024, 6, 1, 12, 0, 0, 123456789, time.UTC)
	facts := []domain.ReviewFact{
		{Rating: 5, CreatedAt: time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC), ReviewerHistory: domain.ReviewerHistory{Ratings: []float64{5, 3, 4}}},
		{Rating: 3, CreatedAt: time.Date(2024, 5, 20, 0, 0, 0, 0, time.UTC), ReviewerHistory: domain.ReviewerHistory{Ratings: []float64{3}}},
	}
	stats := &uowtest.Stats{Facts: func(context.Context, string) ([]domain.ReviewFact, error) { return facts, nil }}
	uow := &uowtest.UoW{Stats: stats, Reviews: &fakeReviewRepo{discardReview: func(context.Context, string) error { return nil }}}
	reviews := usecase.NewReviews(
		&fakeReviewQuery{getReview: func(context.Context, string) (domain.ReviewDetail, error) { return stored, nil }},
		uow, usecase.NewBurgerStatsRecalculator(uowtest.Clock{T: clockTime}), &fakePhotoStorage{})
	if err := reviews.Delete(ctx, alice, stored.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if len(stats.Saved) != 1 {
		t.Fatalf("保存された統計 = %d 件, want 1", len(stats.Saved))
	}
	want := domain.CalculateBurgerStat(stored.BurgerID, facts, clockTime.Truncate(time.Microsecond))
	if got := stats.Saved[0]; got != want {
		t.Errorf("保存された統計 = %+v, want %+v", got, want)
	}
	if got := stats.Saved[0].CalculatedAt; !got.Equal(clockTime.Truncate(time.Microsecond)) {
		t.Errorf("calculated_at = %v, want the clock time truncated to microseconds", got)
	}
}

// TestUsersDeleteRecalculatesInAscendingOrder は、S17 AC4 を扱う。退会は、ユーザーの discard と、その
// ユーザーの review が付く burger の再計算を、1 つのトランザクションで、burger_id の昇順に行う(並行する
// 退会がデッドロックしないよう、ロックの順序を固定する)。
func TestUsersDeleteRecalculatesInAscendingOrder(t *testing.T) {
	ctx := context.Background()
	target := usersViewer
	query := &fakeUserQuery{getByID: activeUsersByID(target)}
	newUsersWithUoW := func(uow *uowtest.UoW, repo *fakeUserRepo) *usecase.Users {
		uow.Users = repo
		return usecase.NewUsers(query, domain.NewUsers(repo), uow, usecase.NewBurgerStatsRecalculator(uowtest.Clock{}), fakeHasher{})
	}

	t.Run("burger_id の昇順に再計算して commit する(読み取りが昇順でなくても)", func(t *testing.T) {
		stats := &uowtest.Stats{ReviewedBy: func(context.Context, string) ([]string, error) { return []string{uid.N(9), uid.N(3), uid.N(5)}, nil }}
		uow := &uowtest.UoW{Stats: stats}
		repo := &fakeUserRepo{discard: func(context.Context, string) error { stats.Note("discard"); return nil }}
		if err := newUsersWithUoW(uow, repo).Delete(ctx, target, target.ID); err != nil {
			t.Fatalf("Delete returned error: %v", err)
		}
		want := []string{
			"discard", "reviewed-by:" + target.ID,
			"lock:" + uid.N(3), "facts:" + uid.N(3), "save:" + uid.N(3),
			"lock:" + uid.N(5), "facts:" + uid.N(5), "save:" + uid.N(5),
			"lock:" + uid.N(9), "facts:" + uid.N(9), "save:" + uid.N(9),
		}
		if !reflect.DeepEqual(stats.Ops, want) {
			t.Errorf("操作の順序 = %v, want %v", stats.Ops, want)
		}
		if uow.Commits != 1 || uow.Rollbacks != 0 {
			t.Errorf("commit/rollback = %d/%d, want 1/0", uow.Commits, uow.Rollbacks)
		}
	})

	t.Run("review のないユーザーの退会は、再計算せずに commit する", func(t *testing.T) {
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
			if op[:4] == "lock" {
				t.Errorf("review のないユーザーでロックした: %v", stats.Ops)
			}
		}
	})

	t.Run("discard が失敗(存在しない)なら、再計算に触れずに rollback する", func(t *testing.T) {
		stats := &uowtest.Stats{}
		uow := &uowtest.UoW{Stats: stats}
		repo := &fakeUserRepo{discard: func(context.Context, string) error { return domain.ErrUserNotFound }}
		if err := newUsersWithUoW(uow, repo).Delete(ctx, target, target.ID); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("Delete error = %v, want %v", err, domain.ErrUserNotFound)
		}
		if len(stats.Ops) != 0 {
			t.Errorf("再計算に触れた: %v, want none", stats.Ops)
		}
		if uow.Commits != 0 || uow.Rollbacks != 1 {
			t.Errorf("commit/rollback = %d/%d, want 0/1", uow.Commits, uow.Rollbacks)
		}
	})

	t.Run("途中の burger の再計算が失敗すると、全体が失敗して commit されない", func(t *testing.T) {
		boom := errors.New("boom")
		stats := &uowtest.Stats{
			ReviewedBy: func(context.Context, string) ([]string, error) { return []string{uid.N(3), uid.N(5)}, nil },
			Facts: func(_ context.Context, burgerID string) ([]domain.ReviewFact, error) {
				if burgerID == uid.N(5) {
					return nil, boom
				}
				return nil, nil
			},
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
