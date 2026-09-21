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

// uowReviews は、テスト用の代役(フェイク)の UnitOfWork(ここからここまでをまとめて 1 つの
// トランザクションにする範囲を、usecase が指定する仕組み)を使った usecase.Reviews を組み立てる。
// 代役は、commit と rollback の回数と、統計に対する操作の順序を記録する。
func uowReviews(query usecase.ReviewQuery, repo domain.ReviewRepository, uow *uowtest.UoW) *usecase.Reviews {
	uow.Reviews = repo
	return usecase.NewReviews(query, uow, usecase.NewBurgerStatsRecalculator(uowtest.Clock{}), &fakePhotoStorage{})
}

// processedIf は、写真つきの編集を試すための、処理済みの画像を返す。withPhoto が false なら nil
// (写真なしの編集)を返す。
func processedIf(withPhoto bool) *photo.Processed {
	if !withPhoto {
		return nil
	}
	return &photo.Processed{Data: []byte("img"), ContentType: "image/jpeg", Ext: ".jpg"}
}

// TestReviewsWriteRecalculatesInOneTransaction は、レビューの投稿・編集・削除が、バーガーの統計の
// 再計算まで含めて 1 つのトランザクションで行われることを、データベースなしで確かめる。途中の
// どこで失敗しても全体が rollback され、統計だけが更新されて残ることはない。
//
// 「操作の順序」は、代役が記録した操作の並びである。各記録の意味は次のとおり。
// バーガーの ID は、テスト用に uid.N(n) で作った UUID の文字列である(n は番号)。
//   - "discard": レビューの論理削除
//   - "insert": レビューの登録
//   - "find-or-create": 名前で指定したバーガーの検索(なければ作成)
//   - "lock:<バーガーの ID>": そのバーガーの行のロック
//   - "facts:<バーガーの ID>": そのバーガーの統計の元になるレビューの読み取り
//   - "save:<バーガーの ID>": そのバーガーの統計の保存
func TestReviewsWriteRecalculatesInOneTransaction(t *testing.T) {
	ctx := context.Background()
	alice := domain.User{ID: uid.N(1), Username: "alice"}
	stored := reviewDetailFor(alice.ID) // バーガー uid.N(5) に付いた、uid.N(9) のレビュー
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

	t.Run("レビューを削除すると、そのバーガーだけを「ロック → 元データの読み取り → 保存」の順に再計算し、commit する", func(t *testing.T) {
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

	t.Run("削除しようとしたレビューが、別の削除で先に消えていた場合は、統計に触れずに rollback し、レビューが見つからないエラーを返す", func(t *testing.T) {
		stats := &uowtest.Stats{}
		uow := &uowtest.UoW{Stats: stats}
		repo := &fakeReviewRepo{discardReview: func(context.Context, string) error {
			return domain.ErrReviewNotFound // 並行する別の削除が、先にこのレビューを論理削除していた
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

	t.Run("統計の保存に失敗すると、レビューの削除ごと rollback し、commit しない", func(t *testing.T) {
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

	t.Run("バーガーのロックの取得に失敗すると、レビューの削除ごと rollback し、commit しない", func(t *testing.T) {
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

	t.Run("バーガー ID を指定して投稿すると、レビューを登録する前にそのバーガーをロックし、登録した後に統計を再計算して commit する", func(t *testing.T) {
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
		// レビューの登録は、外部キーの確認のために、バーガーの行へ弱い共有ロックをかける。その後で
		// 同じ行を更新用のロックに格上げしようとすると、同じバーガーへ同時に投稿した 2 人が互いを
		// 待ち合って止まる(デッドロック)。これを避けるため、登録の前に更新用のロックを取る。
		// 再計算の中でもう一度ロックを取るが、同じトランザクションが持っているロックの取り直しなので
		// 待たされない(操作の記録には 2 回現れる)。
		want := []string{"lock:" + uid.N(5), "insert", "lock:" + uid.N(5), "facts:" + uid.N(5), "save:" + uid.N(5)}
		if !reflect.DeepEqual(stats.Ops, want) {
			t.Errorf("操作の順序 = %v, want %v", stats.Ops, want)
		}
		if uow.Commits != 1 || uow.Rollbacks != 0 {
			t.Errorf("commit/rollback = %d/%d, want 1/0", uow.Commits, uow.Rollbacks)
		}
	})

	t.Run("バーガー名を指定して投稿すると、名前のバーガーを探して(なければ作って)からロックし、レビューを登録して、統計を再計算する", func(t *testing.T) {
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

	t.Run("バーガー名で新しく作ったバーガーへのレビューの登録に失敗すると、作ったバーガーごと rollback し、統計は再計算しない", func(t *testing.T) {
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
			t.Errorf("commit/rollback = %d/%d, want 0/1（レビューのない、作りかけのバーガーを commit しない）", uow.Commits, uow.Rollbacks)
		}
		for _, op := range stats.Ops {
			if op == "facts:"+uid.N(7) || op == "save:"+uid.N(7) {
				t.Errorf("登録の失敗のあとに統計を再計算した: %v", stats.Ops)
			}
		}
	})

	t.Run("レビューの評価とコメントを編集すると、編集したレビューのバーガーの統計を再計算する", func(t *testing.T) {
		for name, withPhoto := range map[string]bool{"写真を付け替えない編集": false, "写真を付け替える編集": true} {
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
				if _, err := reviews.Update(ctx, alice, stored.ID, 5, "Better", processedIf(withPhoto)); err != nil {
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

// TestBurgerStatsRecalculationUsesClockAndDomain は、統計の再計算が、現在時刻を Clock から受け取り、
// 保存される統計が「domain の計算(CalculateBurgerStat)を、その時刻で行った結果」とちょうど一致する
// ことを確かめる。時刻を固定できるので、保存された統計を後から検算できる。
func TestBurgerStatsRecalculationUsesClockAndDomain(t *testing.T) {
	ctx := context.Background()
	alice := domain.User{ID: uid.N(1), Username: "alice"}
	stored := reviewDetailFor(alice.ID)
	// データベースの時刻型(timestamptz)はマイクロ秒までしか持てないので、それより細かい部分は
	// 切り詰めて保存される。
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
		t.Errorf("計算時刻(calculated_at) = %v, want マイクロ秒に切り詰めた Clock の時刻", got)
	}
}

// TestUsersDeleteRecalculatesInAscendingOrder は、ユーザーの退会が、ユーザーの論理削除と、そのユーザーの
// レビューが付いているバーガーの統計の再計算を、1 つのトランザクションで、バーガー ID の昇順に行うことを
// 確かめる。昇順にそろえるのは、同時に退会する 2 人が同じバーガーを逆の順序でロックして、互いを
// 待ち合って止まる(デッドロック)のを避けるため。「操作の順序」の読み方は、上の
// TestReviewsWriteRecalculatesInOneTransaction と同じ(reviewed-by:ユーザー ID は、そのユーザーの
// レビューが付いたバーガーの一覧の読み取り)。
func TestUsersDeleteRecalculatesInAscendingOrder(t *testing.T) {
	ctx := context.Background()
	target := usersViewer
	query := &fakeUserQuery{getByID: activeUsersByID(target)}
	newUsersWithUoW := func(uow *uowtest.UoW, repo *fakeUserRepo) *usecase.Users {
		uow.Users = repo
		return usecase.NewUsers(query, domain.NewUsers(repo), uow, usecase.NewBurgerStatsRecalculator(uowtest.Clock{}), fakeHasher{})
	}

	t.Run("退会すると、そのユーザーがレビューしたバーガーを ID の昇順に 1 つずつ再計算して commit する(読み取りの結果が昇順でなくても)", func(t *testing.T) {
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

	t.Run("レビューを 1 件も書いていないユーザーが退会すると、統計を再計算もロックもせずに commit する", func(t *testing.T) {
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
				t.Errorf("レビューのないユーザーの退会でロックした: %v", stats.Ops)
			}
		}
	})

	t.Run("退会の対象のユーザーが存在せず、論理削除に失敗した場合は、統計に触れずに rollback し、ユーザーが見つからないエラーを返す", func(t *testing.T) {
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

	t.Run("複数のバーガーの再計算の途中で失敗すると、退会ごと rollback し、commit しない", func(t *testing.T) {
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
