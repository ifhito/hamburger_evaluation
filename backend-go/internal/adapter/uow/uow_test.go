package uow_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/infra"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/storage"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/uow"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// このファイルは、usecase.UnitOfWork(ここからここまでをまとめて 1 つのトランザクションにする範囲を、
// usecase が指定する仕組み)の pgx による実装を、実際の PostgreSQL に対して、本物の usecase
// (usecase.Reviews / usecase.Users)・query・repository とつないで確かめる(環境変数 TEST_DATABASE_URL が
// なければスキップする)。レビューの書き込みとバーガーの統計の再計算が 1 つのトランザクションで行われ、
// 保存された統計が、保存されているレビューから domain の計算で求め直した値とちょうど一致することを確かめる。

const (
	insertUser   = `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, 'x', $3) RETURNING id`
	insertBurger = `INSERT INTO burgers (name) VALUES ($1) RETURNING id`
)

// world は、公開中のショップを 1 つ持つ、テスト用のデータベースの状態である。そのショップに結び付いた
// バーガーや、ユーザー、レビューを、必要に応じて作る。
type world struct {
	ctx     context.Context
	conn    *pgx.Conn
	dbURL   string
	shop    string
	unit    *uow.UnitOfWork
	recalc  *usecase.BurgerStatsRecalculator
	reviews *usecase.Reviews
	users   *usecase.Users
	queries struct {
		review *query.ReviewQuery
		shop   *query.ShopQuery
	}
}

func newWorld(t *testing.T) *world {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping DB-backed unit of work test in short mode")
	}
	ctx := context.Background()
	conn, dbURL := dbtest.New(t)
	w := &world{ctx: ctx, conn: conn, dbURL: dbURL}
	w.unit = uow.New(conn)
	w.recalc = usecase.NewBurgerStatsRecalculator(infra.SystemClock{})
	photos := storage.NewDisk(t.TempDir(), "/photos")
	w.queries.review = query.NewReviewQuery(conn)
	w.queries.shop = query.NewShopQuery(conn)
	w.reviews = usecase.NewReviews(w.queries.review, w.unit, w.recalc, photos)
	w.users = usecase.NewUsers(query.NewUserQuery(conn), domain.NewUsers(repository.NewUserRepository(conn)), w.unit, w.recalc, infra.BcryptPasswordHasher{})
	w.shop = dbtest.InsertUUIDRow(ctx, t, conn,
		`INSERT INTO shops (name, status, moderation_note, creator_id) VALUES ($1, $2, $3, $4) RETURNING id`,
		"Active One", 1, nil, nil)
	return w
}

// burger は、ショップに結び付いたバーガーを作って、その ID を返す。
func (w *world) burger(t *testing.T, name string) string {
	t.Helper()
	id := dbtest.InsertUUIDRow(w.ctx, t, w.conn, insertBurger, name)
	if _, err := w.conn.Exec(w.ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, w.shop, id); err != nil {
		t.Fatalf("ショップ %s とバーガー %s の結び付けに失敗した: %v", w.shop, id, err)
	}
	return id
}

// user はユーザーを作って、操作する人(viewer)として使える domain.User を返す。
func (w *world) user(t *testing.T, name string) domain.User {
	t.Helper()
	id := dbtest.InsertUserRow(w.ctx, t, w.conn, insertUser, name+"@example.com", name, false)
	return domain.User{ID: id, Username: name}
}

// review は、usecase を通してレビューを投稿する(統計の再計算まで、本番と同じトランザクションで行われる)。
func (w *world) review(t *testing.T, viewer domain.User, burgerID string, rating int, comment string) domain.ReviewDetail {
	t.Helper()
	detail, err := w.reviews.Create(w.ctx, viewer, w.shop, burgerID, "", rating, comment, nil)
	if err != nil {
		t.Fatalf("レビューの投稿に失敗した: %v", err)
	}
	return detail
}

// TestUnitOfWorkBurgerStats は、レビューの投稿・編集・削除のたびに、バーガーの統計が、同じ
// トランザクションの中で正しく計算し直されることを確かめる。確かめるのは次の 4 点。
//   - 統計の行が、削除されていないレビュー(削除されたユーザーのものを除く)から求め直した値と
//     ちょうど一致し続けること
//   - 同じバーガーへ同時に投稿しても、片方の追加分を取りこぼさないこと
//   - 書き込みに失敗したとき、統計に手を付けないこと
//   - 統計の計算のあとで失敗したとき、レビューの書き込みも統計も巻き戻ること
func TestUnitOfWorkBurgerStats(t *testing.T) {
	w := newWorld(t)
	ctx, conn := w.ctx, w.conn
	alice := w.user(t, "alice")
	bob := w.user(t, "bob")
	burger := w.burger(t, "Stats Burger")

	var aliceReview, bobReview domain.ReviewDetail

	t.Run("レビューを投稿するたびに、そのバーガーの統計(件数と平均)が、投稿を含めた値に更新される", func(t *testing.T) {
		aliceReview = w.review(t, alice, burger, 5, "great")
		stats := dbtest.RequireConsistentStats(ctx, t, conn, burger)
		if stats.ReviewCount != 1 || stats.AverageRating != 5.0 {
			t.Errorf("1 件目の投稿後の統計 = %+v, want 件数 1・平均 5.0", stats)
		}

		bobReview = w.review(t, bob, burger, 4, "good")
		stats = dbtest.RequireConsistentStats(ctx, t, conn, burger)
		if stats.ReviewCount != 2 || stats.AverageRating != 4.5 {
			t.Errorf("2 件目の投稿後の統計 = %+v, want 件数 2・平均 4.5", stats)
		}
	})

	t.Run("レビューの評価を 5 から 1 に編集すると、そのバーガーの統計(平均と加重スコア)が編集後の値に更新される", func(t *testing.T) {
		before := dbtest.RequireConsistentStats(ctx, t, conn, burger)
		if _, err := w.reviews.Update(ctx, alice, aliceReview.ID, 1, "changed my mind", nil); err != nil {
			t.Fatalf("レビューの編集に失敗した: %v", err)
		}
		stats := dbtest.RequireConsistentStats(ctx, t, conn, burger)
		if stats.ReviewCount != 2 || stats.AverageRating != 2.5 {
			t.Errorf("編集後の統計 = %+v, want 件数 2・平均 2.5", stats)
		}
		if stats.WeightedScore == before.WeightedScore {
			t.Errorf("評価を 5 から 1 に編集したのに、加重スコアが %v のまま変わっていない", stats.WeightedScore)
		}
	})

	t.Run("レビューを削除すると、そのバーガーの統計が、削除したレビューを除いた値に更新される", func(t *testing.T) {
		if err := w.reviews.Delete(ctx, alice, aliceReview.ID); err != nil {
			t.Fatalf("削除に失敗した: %v", err)
		}
		stats := dbtest.RequireConsistentStats(ctx, t, conn, burger)
		if stats.ReviewCount != 1 || stats.AverageRating != 4.0 {
			t.Errorf("削除後の統計 = %+v, want bob の評価 4 だけが残った値", stats)
		}
	})

	t.Run("バーガーの最後の 1 件のレビューを削除すると、統計の行はなくならず、件数 0・平均 0 の行になる", func(t *testing.T) {
		if err := w.reviews.Delete(ctx, bob, bobReview.ID); err != nil {
			t.Fatalf("削除に失敗した: %v", err)
		}
		stats := dbtest.RequireConsistentStats(ctx, t, conn, burger)
		want := dbtest.StoredBurgerStats{ReviewCount: 0, AverageRating: 0.0, WeightedScore: 0.0, Confidence: 0.0, CalculatedAt: stats.CalculatedAt}
		if stats != want {
			t.Errorf("最後のレビューを削除した後の統計 = %+v, want 件数 0 の行", stats)
		}
	})

	t.Run("ユーザーが削除されると、そのユーザーのレビューは、統計の元データにも、他の投稿者の信頼度の履歴にも含まれなくなる", func(t *testing.T) {
		deletedUserBurger := w.burger(t, "Deleted User Burger")
		carl := w.user(t, "carl")
		aliceKept := w.review(t, alice, deletedUserBurger, 5, "mine stays")
		w.review(t, carl, deletedUserBurger, 2, "mine vanishes")
		if got := dbtest.RequireConsistentStats(ctx, t, conn, deletedUserBurger); got.ReviewCount != 2 {
			t.Fatalf("ユーザー削除前の統計 = %+v, want 件数 2", got)
		}

		if _, err := conn.Exec(ctx, `UPDATE users SET discarded_at = now() WHERE id = $1`, carl.ID); err != nil {
			t.Fatalf("ユーザーの論理削除に失敗した: %v", err)
		}
		// 削除されていないユーザーの書き込み(編集)で、統計の再計算を起こす。
		if _, err := w.reviews.Update(ctx, alice, aliceKept.ID, 4, "still here", nil); err != nil {
			t.Fatalf("レビューの編集に失敗した: %v", err)
		}

		stats := dbtest.RequireConsistentStats(ctx, t, conn, deletedUserBurger)
		if stats.ReviewCount != 1 || stats.AverageRating != 4.0 {
			t.Errorf("ユーザー削除後の統計 = %+v, want alice のレビューだけが残った値", stats)
		}
		// 削除された carl の評価は、統計の元データにも、他の投稿者の信頼度の履歴にも含まれない。
		// したがって、保存されたスコアは、alice のレビューと、alice 自身の評価の履歴だけから
		// 計算したスコアと等しくなる。
		var createdAt time.Time
		if err := conn.QueryRow(ctx, `SELECT created_at FROM reviews WHERE id = $1`, aliceKept.ID).Scan(&createdAt); err != nil {
			t.Fatalf("レビューの作成日時の取得に失敗した: %v", err)
		}
		aliceOnly := []domain.ReviewFact{{
			Rating:          4,
			CreatedAt:       createdAt,
			ReviewerHistory: domain.ReviewerHistory{Ratings: dbtest.KeptRatingsOf(ctx, t, conn, alice.ID)},
		}}
		if want := domain.CalculateBurgerScore(aliceOnly, stats.CalculatedAt); stats.WeightedScore != want.WeightedAverage || stats.Confidence != want.Confidence {
			t.Errorf("統計 = %+v, want alice のレビューと履歴だけから計算したスコア %+v", stats, want)
		}
	})

	t.Run("同じバーガーに 2 人が同時に投稿しても、どちらの投稿も統計に反映され、更新を取りこぼさない", func(t *testing.T) {
		pool, err := pgxpool.New(ctx, w.dbURL)
		if err != nil {
			t.Fatalf("接続プールを開けなかった: %v", err)
		}
		t.Cleanup(pool.Close)
		poolReviews := usecase.NewReviews(query.NewReviewQuery(pool), uow.New(pool), w.recalc, storage.NewDisk(t.TempDir(), "/photos"))

		dave := w.user(t, "dave")
		erin := w.user(t, "erin")

		// バーガーの行を FOR UPDATE でロックして順番に処理しないと、2 つのトランザクションが、
		// どちらも相手の投稿を知らないまま「読んでから上書き」して、後から書いた側が件数 1 で
		// 上書きしてしまう(更新の取りこぼし)。たまたま順番が合って失敗を見逃さないよう、新しい
		// バーガーで 5 回繰り返す。
		for i := 0; i < 5; i++ {
			raceBurger := w.burger(t, fmt.Sprintf("Race Burger %d", i))
			start := make(chan struct{})
			errs := make(chan error, 2)
			for _, c := range []struct {
				viewer domain.User
				rating int
			}{{dave, 5}, {erin, 3}} {
				c := c
				go func() {
					<-start
					_, err := poolReviews.Create(ctx, c.viewer, w.shop, raceBurger, "", c.rating, "race", nil)
					errs <- err
				}()
			}
			close(start)
			for j := 0; j < 2; j++ {
				if err := <-errs; err != nil {
					t.Fatalf("%d 回目: 同時の投稿が失敗した: %v", i, err)
				}
			}
			stats := dbtest.RequireConsistentStats(ctx, t, conn, raceBurger)
			if stats.ReviewCount != 2 || stats.AverageRating != 4.0 {
				t.Fatalf("%d 回目: 統計 = %+v, want 2 人の投稿が両方反映された件数 2・平均 4.0", i, stats)
			}
		}
	})

	t.Run("存在しない・削除済みのレビューの編集や削除に失敗したとき、バーガーの統計は書き換えられない", func(t *testing.T) {
		errBurger := w.burger(t, "Error Burger")
		w.review(t, alice, errBurger, 5, "baseline")
		victim := w.review(t, bob, errBurger, 3, "to discard")
		if err := w.reviews.Delete(ctx, bob, victim.ID); err != nil {
			t.Fatalf("削除に失敗した: %v", err)
		}
		before := dbtest.RequireConsistentStats(ctx, t, conn, errBurger)

		// 書き込みに失敗する場面は、UnitOfWork.Do(トランザクションの範囲を指定して実行する関数)を
		// 直接使って作る。usecase は、書き込みの前に
		// 読み取りでレビューの存在を確かめるので、存在しないレビューでは書き込みまで進まないため。
		fail := func(write func(tx usecase.Tx) error) error {
			return w.unit.Do(ctx, func(ctx context.Context, tx usecase.Tx) error {
				if err := write(tx); err != nil {
					return err
				}
				return w.recalc.Recalculate(ctx, tx, errBurger)
			})
		}
		if err := fail(func(tx usecase.Tx) error {
			_, err := tx.Reviews.UpdateContent(ctx, victim.ID, 1, "x")
			return err
		}); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("削除済みのレビューの編集 = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if err := fail(func(tx usecase.Tx) error {
			_, err := tx.Reviews.UpdateContent(ctx, uid.N(99999), 1, "x")
			return err
		}); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("存在しないレビューの編集 = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if err := fail(func(tx usecase.Tx) error { return tx.Reviews.Discard(ctx, victim.ID) }); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("削除済みのレビューの削除 = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if err := fail(func(tx usecase.Tx) error { return tx.Reviews.Discard(ctx, uid.N(99999)) }); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("存在しないレビューの削除 = %v, want %v", err, domain.ErrReviewNotFound)
		}

		after, ok := dbtest.FetchBurgerStats(ctx, t, conn, errBurger)
		if !ok {
			t.Fatal("統計の行(burger_stats)がなくなった")
		}
		if after != before || !after.CalculatedAt.Equal(before.CalculatedAt) {
			t.Errorf("失敗した書き込みの後の統計 = %+v, want 変化なし %+v", after, before)
		}
	})

	t.Run("レビューの登録と統計の再計算まで終えたあとで失敗すると、登録したレビューも、計算し直した統計も巻き戻る", func(t *testing.T) {
		rbBurger := w.burger(t, "Rollback Burger")
		w.review(t, alice, rbBurger, 5, "baseline")
		before := dbtest.RequireConsistentStats(ctx, t, conn, rbBurger)

		boom := errors.New("boom")
		err := w.unit.Do(ctx, func(ctx context.Context, tx usecase.Tx) error {
			review, err := domain.NewReview(1, "doomed", bob.ID, rbBurger)
			if err != nil {
				return err
			}
			if err := tx.BurgerStats.Lock(ctx, rbBurger); err != nil {
				return err
			}
			if _, err := tx.Reviews.Create(ctx, review); err != nil {
				return err
			}
			if err := w.recalc.Recalculate(ctx, tx, rbBurger); err != nil {
				return err
			}
			// 再計算まで済ませたあとで失敗させる。同じトランザクションの読み取りには、まだ確定して
			// いないこの書き込みが見えていることも、あわせて確かめる。
			facts, err := tx.Stats.ListBurgerReviewFacts(ctx, rbBurger)
			if err != nil || len(facts) != 2 {
				t.Errorf("同じトランザクションで読んだ統計の元データ = %d 件 (エラー %v), want 未確定の書き込みが見える 2 件", len(facts), err)
			}
			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatalf("Do のエラー = %v, want %v", err, boom)
		}
		var reviews int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM reviews WHERE burger_id = $1`, rbBurger).Scan(&reviews); err != nil {
			t.Fatalf("レビューの件数の取得に失敗した: %v", err)
		}
		if reviews != 1 {
			t.Errorf("レビューの行数 = %d, want 失敗した登録が巻き戻って、最初の 1 件だけ", reviews)
		}
		after, _ := dbtest.FetchBurgerStats(ctx, t, conn, rbBurger)
		if after != before {
			t.Errorf("統計 = %+v, want 巻き戻った再計算の前の値 %+v", after, before)
		}
	})
}

// TestUnitOfWorkNamedBurger は、バーガー名を指定して投稿する経路を、UnitOfWork(まとめて 1 つの
// トランザクションにする範囲)の中で確かめる。
// 同名のバーガーの再利用と、新しいバーガーの作成に加えて、レビューの登録に失敗したときに、作りかけの
// バーガーやショップとの結び付けが確定しないこと(バーガーの作成からレビューの登録までが 1 つの
// トランザクションであること)と、投稿と同じトランザクションで統計が計算し直されることを確かめる。
func TestUnitOfWorkNamedBurger(t *testing.T) {
	w := newWorld(t)
	ctx, conn := w.ctx, w.conn
	alice := w.user(t, "alice")
	shopB := dbtest.InsertUUIDRow(ctx, t, conn,
		`INSERT INTO shops (name, status, moderation_note, creator_id) VALUES ($1, $2, $3, $4) RETURNING id`,
		"Shop B", 1, nil, nil)

	cheese := w.burger(t, "Cheese")
	if _, err := conn.Exec(ctx,
		`INSERT INTO burger_stats (burger_id, review_count, average_rating, weighted_score, confidence, calculated_at)
		 VALUES ($1, 2, 4.0, 3.9, 0.7, now())`, cheese); err != nil {
		t.Fatalf("Cheese の統計の準備に失敗した: %v", err)
	}

	countRows := func(t *testing.T, query string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := conn.QueryRow(ctx, query, args...).Scan(&n); err != nil {
			t.Fatalf("件数の取得(%s)に失敗した: %v", query, err)
		}
		return n
	}
	burgersNamed := func(t *testing.T, name string) int64 {
		return countRows(t, `SELECT count(*) FROM burgers WHERE name = $1`, name)
	}
	named := func(t *testing.T, viewer domain.User, shopID string, name string) domain.ReviewDetail {
		t.Helper()
		detail, err := w.reviews.Create(ctx, viewer, shopID, "", name, 4, "via name", nil)
		if err != nil {
			t.Fatalf("投稿に失敗した: %v", err)
		}
		return detail
	}

	t.Run("同じショップに同名のバーガーがあれば、それにレビューを付け、応答のバーガーの統計は投稿前の値になる", func(t *testing.T) {
		created := named(t, alice, w.shop, "Cheese")
		want := domain.ShopReviewBurger{ID: cheese, Name: "Cheese", AverageRating: 4.0, ReviewCount: 2, WeightedScore: 3.9, Confidence: 0.7}
		if created.Burger == nil || !reflect.DeepEqual(*created.Burger, want) {
			t.Errorf("応答のバーガー = %+v, want 投稿前の統計を持つバーガー %+v", created.Burger, want)
		}
		if created.ID == "" || created.BurgerID != cheese || created.CreatedAt.IsZero() {
			t.Errorf("作成されたレビュー = %+v, want バーガー %s に付いた、保存済みのレビュー", created.Review, cheese)
		}
		if got := burgersNamed(t, "Cheese"); got != 1 {
			t.Errorf("Cheese のバーガーの行数 = %d, want 重複なしの 1", got)
		}
		// 投稿と同じトランザクションの中で、統計が計算し直されている。
		if stats := dbtest.RequireConsistentStats(ctx, t, conn, cheese); stats.ReviewCount != 1 {
			t.Errorf("統計 = %+v, want 計算し直された件数 1(新しいレビューだけ)", stats)
		}
	})

	t.Run("そのショップにない名前で投稿すると、バーガーを新しく作ってショップと結び付け、そのバーガーの統計は件数 1 になる", func(t *testing.T) {
		created := named(t, alice, w.shop, "Veggie")
		burger := created.Burger
		if burger == nil || burger.Name != "Veggie" || burger.ID == cheese {
			t.Fatalf("バーガー = %+v, want 新しく作られた Veggie", burger)
		}
		if zero := (domain.ShopReviewBurger{ID: burger.ID, Name: "Veggie"}); !reflect.DeepEqual(*burger, zero) {
			t.Errorf("バーガー = %+v, want 投稿前の統計が 0", *burger)
		}
		if created.BurgerID != burger.ID {
			t.Errorf("レビューのバーガー = %s, want %s", created.BurgerID, burger.ID)
		}
		if got := countRows(t, `SELECT count(*) FROM shops_burgers WHERE shop_id = $1 AND burger_id = $2`, w.shop, burger.ID); got != 1 {
			t.Errorf("ショップとの結び付けの行数 = %d, want 1", got)
		}
		if stats := dbtest.RequireConsistentStats(ctx, t, conn, burger.ID); stats.ReviewCount != 1 {
			t.Errorf("統計 = %+v, want 件数 1", stats)
		}
	})

	t.Run("別のショップに同じ名前のバーガーがあっても、投稿したショップ用に別のバーガーを作る", func(t *testing.T) {
		burger := named(t, alice, shopB, "Cheese").Burger
		if burger == nil || burger.ID == cheese {
			t.Fatalf("バーガー = %+v, want 最初のショップの Cheese(%s)とは別に、新しく作られた行", burger, cheese)
		}
		if got := burgersNamed(t, "Cheese"); got != 2 {
			t.Errorf("Cheese のバーガーの行数 = %d, want ショップごとに 1 つずつの 2", got)
		}
		if got := countRows(t, `SELECT count(*) FROM shops_burgers WHERE shop_id = $1 AND burger_id = $2`, shopB, burger.ID); got != 1 {
			t.Errorf("2 つ目のショップとの結び付けの行数 = %d, want 1", got)
		}
		// 最初のショップの Cheese の結び付けは、元のバーガーだけを指したままである。
		if got := countRows(t, `SELECT count(*) FROM shops_burgers WHERE shop_id = $1`, w.shop); got != 2 {
			t.Errorf("最初のショップとの結び付けの行数 = %d, want 元の Cheese と Veggie の 2", got)
		}
	})

	t.Run("存在しないユーザーで新しい名前のバーガーに投稿して登録に失敗すると、作りかけのバーガーやショップとの結び付けは残らない", func(t *testing.T) {
		// 存在しないユーザーの投稿は、バーガーとショップとの結び付けを作ったあと、レビューの登録で
		// 外部キー(reviews.user_id)に違反して失敗する。そのとき、作ったバーガーも巻き戻る必要がある。
		ghost := domain.User{ID: uid.N(99999), Username: "ghost"}
		if _, err := w.reviews.Create(ctx, ghost, w.shop, "", "Ghost", 4, "ok", nil); err == nil {
			t.Fatal("投稿が成功した。外部キー違反で失敗するはず")
		}
		if got := burgersNamed(t, "Ghost"); got != 0 {
			t.Errorf("Ghost のバーガーの行数 = %d, want 巻き戻って 0", got)
		}
		if got := countRows(t, `SELECT count(*) FROM shops_burgers sb JOIN burgers b ON b.id = sb.burger_id WHERE b.name = 'Ghost'`); got != 0 {
			t.Errorf("Ghost とショップとの結び付けの行数 = %d, want 0", got)
		}
		if got := countRows(t, `SELECT count(*) FROM reviews WHERE user_id = $1`, ghost.ID); got != 0 {
			t.Errorf("レビューの行数 = %d, want 0", got)
		}
	})
}

// TestUnitOfWorkDiscardUser は、ユーザーの退会(論理削除)と、そのユーザーのレビューが付いている
// バーガーの統計の再計算が、1 つのトランザクションで行われることを確かめる。あわせて、退会した
// ユーザーのレビューが読み取りから隠れること、同時に退会する 2 人がデッドロックしないこと
// (バーガー ID の昇順にロックするため)も確かめる。
func TestUnitOfWorkDiscardUser(t *testing.T) {
	w := newWorld(t)
	ctx, conn := w.ctx, w.conn
	alice := w.user(t, "alice")
	victim := w.user(t, "victim")
	shared := w.burger(t, "Shared")
	solo := w.burger(t, "Solo")
	// "shared" は victim と alice の両方が、"solo" は victim だけがレビューしたバーガーである。
	// victim が退会すると、shared の統計は alice のレビューだけになり、solo の統計は件数 0 の行になる。
	victimShared := w.review(t, victim, shared, 2, "meh")
	aliceShared := w.review(t, alice, shared, 4, "good")
	victimSolo := w.review(t, victim, solo, 5, "only mine")

	t.Run("ユーザーが退会すると、削除日時が記録され、そのユーザーがレビューしたバーガーの統計が、そのレビューを除いた値に更新される", func(t *testing.T) {
		if got := dbtest.RequireConsistentStats(ctx, t, conn, shared); got.ReviewCount != 2 {
			t.Fatalf("退会前の shared の統計 = %+v, want 件数 2", got)
		}
		if err := w.users.Delete(ctx, victim, victim.ID); err != nil {
			t.Fatalf("削除に失敗した: %v", err)
		}
		// 論理削除なので、行は残り、削除日時(discarded_at)が記録されている。
		var discardedAt *time.Time
		if err := conn.QueryRow(ctx, `SELECT discarded_at FROM users WHERE id = $1`, victim.ID).Scan(&discardedAt); err != nil {
			t.Fatalf("退会したユーザーの取得に失敗した: %v", err)
		}
		if discardedAt == nil {
			t.Fatal("削除日時(users.discarded_at)が記録されていない")
		}
		// 退会したユーザーのレビュー自体は、削除されない。画面から隠すのは、読み取りの側で
		// 削除済みのユーザーのレビューを除いて行う。
		var keptReviews int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM reviews WHERE user_id = $1 AND discarded_at IS NULL`, victim.ID).Scan(&keptReviews); err != nil {
			t.Fatalf("victim のレビュー数の取得に失敗した: %v", err)
		}
		if keptReviews != 2 {
			t.Errorf("victim の削除されていないレビュー数 = %d, want 2(レビューは削除されない)", keptReviews)
		}
		// shared の統計は、alice のレビューだけになる。alice のレビューは影響を受けない。
		sharedStats := dbtest.RequireConsistentStats(ctx, t, conn, shared)
		if sharedStats.ReviewCount != 1 || sharedStats.AverageRating != 4.0 {
			t.Errorf("退会後の shared の統計 = %+v, want alice の評価 4 だけ", sharedStats)
		}
		// solo は victim だけがレビューしたので、件数 0 の行になる。
		soloStats := dbtest.RequireConsistentStats(ctx, t, conn, solo)
		want := dbtest.StoredBurgerStats{ReviewCount: 0, AverageRating: 0.0, WeightedScore: 0.0, Confidence: 0.0, CalculatedAt: soloStats.CalculatedAt}
		if soloStats != want {
			t.Errorf("退会後の solo の統計 = %+v, want 件数 0 の行", soloStats)
		}
	})

	t.Run("すでに退会したユーザーや存在しないユーザーの退会は、ユーザーが見つからないエラーになる", func(t *testing.T) {
		if err := w.users.Delete(ctx, victim, victim.ID); !errors.Is(err, domain.ErrUserNotFound) {
			t.Errorf("すでに退会したユーザーの退会 = %v, want %v", err, domain.ErrUserNotFound)
		}
		unknown := domain.User{ID: uid.N(99999)}
		if err := w.users.Delete(ctx, unknown, unknown.ID); !errors.Is(err, domain.ErrUserNotFound) {
			t.Errorf("存在しないユーザーの退会 = %v, want %v", err, domain.ErrUserNotFound)
		}
	})

	t.Run("退会したユーザーのレビューは、一覧・詳細・ショップのレビュー一覧から隠れ、他のユーザーのレビューは見え続ける", func(t *testing.T) {
		// 一覧: alice のレビューだけが残り、一覧に表示されるバーガーの統計は、計算し直された
		// 統計の行(件数 1)と一致する。
		feed, _, err := w.queries.review.ListReviews(ctx, usecase.ReviewListFilter{}, 100, 0)
		if err != nil {
			t.Fatalf("レビュー一覧の取得に失敗した: %v", err)
		}
		ids := make([]string, 0, len(feed))
		for _, r := range feed {
			ids = append(ids, r.ID)
		}
		if want := []string{aliceShared.ID}; !reflect.DeepEqual(ids, want) {
			t.Fatalf("一覧のレビュー ID = %v, want %v(victim のレビューは隠れる)", ids, want)
		}
		sharedStats := dbtest.RequireConsistentStats(ctx, t, conn, shared)
		if feed[0].Burger.ReviewCount != sharedStats.ReviewCount || feed[0].Burger.ReviewCount != int64(len(feed)) {
			t.Errorf("一覧に表示されるレビュー数(バーガーの統計) = %d, want 統計の行の件数 %d = 表示されるレビューの数 %d",
				feed[0].Burger.ReviewCount, sharedStats.ReviewCount, len(feed))
		}
		// 詳細: 削除済みのユーザーのレビューは、存在しないレビューと区別がつかない(見つからない
		// エラーになる)。alice のレビューは、引き続き取得できる。
		if _, err := w.queries.review.GetReview(ctx, victimShared.ID); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("victim が shared に書いたレビューの詳細 = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if _, err := w.queries.review.GetReview(ctx, victimSolo.ID); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("victim が solo に書いたレビューの詳細 = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if _, err := w.queries.review.GetReview(ctx, aliceShared.ID); err != nil {
			t.Errorf("alice のレビューの詳細の取得に失敗した: %v", err)
		}
		// ショップのレビュー一覧: alice のレビューだけが載る。
		shopReviews, err := w.queries.shop.ListShopReviews(ctx, w.shop)
		if err != nil {
			t.Fatalf("ショップのレビュー一覧の取得に失敗した: %v", err)
		}
		shopIDs := make([]string, 0, len(shopReviews))
		for _, r := range shopReviews {
			shopIDs = append(shopIDs, r.ID)
		}
		if want := []string{aliceShared.ID}; !reflect.DeepEqual(shopIDs, want) {
			t.Errorf("ショップのレビュー ID = %v, want %v(victim のレビューは隠れる)", shopIDs, want)
		}
	})

	t.Run("同じバーガーにレビューした 2 人が同時に退会しても、デッドロックせず、どちらの退会も成功する", func(t *testing.T) {
		pool, err := pgxpool.New(ctx, w.dbURL)
		if err != nil {
			t.Fatalf("接続プールを開けなかった: %v", err)
		}
		t.Cleanup(pool.Close)
		poolUsers := usecase.NewUsers(query.NewUserQuery(pool), domain.NewUsers(repository.NewUserRepository(pool)), uow.New(pool), w.recalc, infra.BcryptPasswordHasher{})

		// 2 人が、同じ 4 つのバーガーにレビューする。退会に伴う再計算は、バーガー ID の昇順に
		// ロックするので、2 人が互いに逆の順序でロックして待ち合う(デッドロック)ことがない。
		// たまたま順番が合って失敗を見逃さないよう、5 回繰り返す。
		for i := 0; i < 5; i++ {
			burgers := make([]string, 4)
			for j := range burgers {
				burgers[j] = w.burger(t, fmt.Sprintf("Overlap %d-%d", i, j))
			}
			left := w.user(t, fmt.Sprintf("left%d", i))
			right := w.user(t, fmt.Sprintf("right%d", i))
			for _, id := range burgers {
				w.review(t, left, id, 5, "l")
				w.review(t, right, id, 3, "r")
			}
			start := make(chan struct{})
			errs := make(chan error, 2)
			for _, u := range []domain.User{left, right} {
				u := u
				go func() {
					<-start
					errs <- poolUsers.Delete(ctx, u, u.ID)
				}()
			}
			close(start)
			for j := 0; j < 2; j++ {
				if err := <-errs; err != nil {
					t.Fatalf("%d 回目: 同時の退会が失敗した: %v", i, err)
				}
			}
			for _, id := range burgers {
				if stats := dbtest.RequireConsistentStats(ctx, t, conn, id); stats.ReviewCount != 0 {
					t.Fatalf("%d 回目のバーガー %s: 統計 = %+v, want 2 人のレビューがどちらも除かれた件数 0", i, id, stats)
				}
			}
		}
	})
}
