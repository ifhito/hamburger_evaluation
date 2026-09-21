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

// このファイルは、usecase.UnitOfWork の pgx による実装を、実際の PostgreSQL に対して、本物の usecase
// （usecase.Reviews / usecase.Users）・query・repository とつないで検証する（TEST_DATABASE_URL がなければ
// dbtest の内部でスキップする）。review の書き込みと burger の統計の再計算が 1 つのトランザクションで
// 行われること（S7・S17）を、統計の行と domain の計算の一致で確かめる。

const (
	insertUser   = `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, 'x', $3) RETURNING id`
	insertBurger = `INSERT INTO burgers (name) VALUES ($1) RETURNING id`
)

// world は、active な shop 1 つと、それに紐づく burger を作れる、テスト用の DB の状態である。
type world struct {
	ctx     context.Context
	conn    *pgx.Conn
	dbURL   string
	shop    int64
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
	w.shop = dbtest.InsertRow(ctx, t, conn,
		`INSERT INTO shops (name, status, moderation_note, creator_id) VALUES ($1, $2, $3, $4) RETURNING id`,
		"Active One", 1, nil, nil)
	return w
}

// burger は、shop に紐づく burger を作って id を返す。
func (w *world) burger(t *testing.T, name string) int64 {
	t.Helper()
	id := dbtest.InsertRow(w.ctx, t, w.conn, insertBurger, name)
	if _, err := w.conn.Exec(w.ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, w.shop, id); err != nil {
		t.Fatalf("link shop %d burger %d: %v", w.shop, id, err)
	}
	return id
}

// user は user を作って、viewer として使える domain.User を返す。
func (w *world) user(t *testing.T, name string) domain.User {
	t.Helper()
	id := dbtest.InsertUserRow(w.ctx, t, w.conn, insertUser, name+"@example.com", name, false)
	return domain.User{ID: id, Username: name}
}

// review は usecase を通して（同一トランザクションで再計算しながら）review を投稿する。
func (w *world) review(t *testing.T, viewer domain.User, burgerID int64, rating int, comment string) domain.ReviewDetail {
	t.Helper()
	detail, err := w.reviews.Create(w.ctx, viewer, w.shop, burgerID, "", rating, comment, nil)
	if err != nil {
		t.Fatalf("Create review returned error: %v", err)
	}
	return detail
}

// TestUnitOfWorkBurgerStats は、S7 の同一トランザクション内での burger_stats の再計算（issue #15）を、
// usecase が UnitOfWork.Do の中で組み立てる形（S17）で検証する。review の書き込みのたびに、stats の行は、
// kept な user の kept な review に対する domain の calculator と厳密に整合した状態になり、並行する書き込みが
// 更新を失うことはなく、失敗した書き込みは stats に手を付けない。
func TestUnitOfWorkBurgerStats(t *testing.T) {
	w := newWorld(t)
	ctx, conn := w.ctx, w.conn
	alice := w.user(t, "alice")
	bob := w.user(t, "bob")
	burger := w.burger(t, "Stats Burger")

	var aliceReview, bobReview domain.ReviewDetail

	t.Run("AC1 create は同一トランザクション内で stats を upsert する", func(t *testing.T) {
		aliceReview = w.review(t, alice, burger, 5, "great")
		stats := dbtest.RequireConsistentStats(ctx, t, conn, burger)
		if stats.ReviewCount != 1 || stats.AverageRating != 5.0 {
			t.Errorf("stats after first review = %+v, want count 1 and average 5.0", stats)
		}

		bobReview = w.review(t, bob, burger, 4, "good")
		stats = dbtest.RequireConsistentStats(ctx, t, conn, burger)
		if stats.ReviewCount != 2 || stats.AverageRating != 4.5 {
			t.Errorf("stats after second review = %+v, want count 2 and average 4.5", stats)
		}
	})

	t.Run("update は stats を再計算する", func(t *testing.T) {
		before := dbtest.RequireConsistentStats(ctx, t, conn, burger)
		if _, err := w.reviews.Update(ctx, alice, aliceReview.ID, 1, "changed my mind", nil); err != nil {
			t.Fatalf("Update returned error: %v", err)
		}
		stats := dbtest.RequireConsistentStats(ctx, t, conn, burger)
		if stats.ReviewCount != 2 || stats.AverageRating != 2.5 {
			t.Errorf("stats after edit = %+v, want count 2 and average 2.5", stats)
		}
		if stats.WeightedScore == before.WeightedScore {
			t.Errorf("weighted score stayed %v after a 5→1 edit, want a change", stats.WeightedScore)
		}
	})

	t.Run("AC2 delete は discard した review を除いて再計算する", func(t *testing.T) {
		if err := w.reviews.Delete(ctx, alice, aliceReview.ID); err != nil {
			t.Fatalf("Delete returned error: %v", err)
		}
		stats := dbtest.RequireConsistentStats(ctx, t, conn, burger)
		if stats.ReviewCount != 1 || stats.AverageRating != 4.0 {
			t.Errorf("stats after discard = %+v, want only bob's rating 4 left", stats)
		}
	})

	t.Run("AC2 唯一の review を discard すると stats はゼロの行になる", func(t *testing.T) {
		if err := w.reviews.Delete(ctx, bob, bobReview.ID); err != nil {
			t.Fatalf("Delete returned error: %v", err)
		}
		stats := dbtest.RequireConsistentStats(ctx, t, conn, burger)
		want := dbtest.StoredBurgerStats{ReviewCount: 0, AverageRating: 0.0, WeightedScore: 0.0, Confidence: 0.0, CalculatedAt: stats.CalculatedAt}
		if stats != want {
			t.Errorf("stats after last discard = %+v, want the zero row", stats)
		}
	})

	t.Run("AC4 discard 済みの user の review と履歴は除外される", func(t *testing.T) {
		ac4Burger := w.burger(t, "AC4 Burger")
		carl := w.user(t, "carl")
		aliceAC4 := w.review(t, alice, ac4Burger, 5, "mine stays")
		w.review(t, carl, ac4Burger, 2, "mine vanishes")
		if got := dbtest.RequireConsistentStats(ctx, t, conn, ac4Burger); got.ReviewCount != 2 {
			t.Fatalf("stats before user discard = %+v, want count 2", got)
		}

		if _, err := conn.Exec(ctx, `UPDATE users SET discarded_at = now() WHERE id = $1`, carl.ID); err != nil {
			t.Fatalf("discard user: %v", err)
		}
		// kept な user の書き込みで再計算を発火させる。
		if _, err := w.reviews.Update(ctx, alice, aliceAC4.ID, 4, "still here", nil); err != nil {
			t.Fatalf("Update returned error: %v", err)
		}

		stats := dbtest.RequireConsistentStats(ctx, t, conn, ac4Burger)
		if stats.ReviewCount != 1 || stats.AverageRating != 4.0 {
			t.Errorf("stats after user discard = %+v, want only alice's kept review", stats)
		}
		// Carl の rating は、fact にも、どの reviewer の履歴にも反映されない：
		// 保存されたスコアは、alice の fact と alice 自身の kept な rating
		// だけから計算したスコアに等しい。
		var createdAt time.Time
		if err := conn.QueryRow(ctx, `SELECT created_at FROM reviews WHERE id = $1`, aliceAC4.ID).Scan(&createdAt); err != nil {
			t.Fatalf("select review created_at: %v", err)
		}
		aliceOnly := []domain.ReviewFact{{
			Rating:          4,
			CreatedAt:       createdAt,
			ReviewerHistory: domain.ReviewerHistory{Ratings: dbtest.KeptRatingsOf(ctx, t, conn, alice.ID)},
		}}
		if want := domain.CalculateBurgerScore(aliceOnly, stats.CalculatedAt); stats.WeightedScore != want.WeightedAverage || stats.Confidence != want.Confidence {
			t.Errorf("stats = %+v, want score %+v from alice's fact and history alone", stats, want)
		}
	})

	t.Run("AC3 1 つの burger への並行する create で更新が失われない", func(t *testing.T) {
		pool, err := pgxpool.New(ctx, w.dbURL)
		if err != nil {
			t.Fatalf("open pool: %v", err)
		}
		t.Cleanup(pool.Close)
		poolReviews := usecase.NewReviews(query.NewReviewQuery(pool), uow.New(pool), w.recalc, storage.NewDisk(t.TempDir(), "/photos"))

		dave := w.user(t, "dave")
		erin := w.user(t, "erin")

		// FOR UPDATE による直列化がなければ、2 つのトランザクションはどちらも
		// 相手の review が欠けたスナップショットを読み、後の upsert が
		// review_count 1 を書き込む（lost update）。新しい burger で繰り返す
		// のは、運よくうまく interleave しても race が隠れないようにするため
		// である。
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
					t.Fatalf("iteration %d: concurrent Create returned error: %v", i, err)
				}
			}
			stats := dbtest.RequireConsistentStats(ctx, t, conn, raceBurger)
			if stats.ReviewCount != 2 || stats.AverageRating != 4.0 {
				t.Fatalf("iteration %d: stats = %+v, want count 2 and average 4.0 (both writers)", i, stats)
			}
		}
	})

	t.Run("失敗した書き込みは burger_stats に手を付けない", func(t *testing.T) {
		errBurger := w.burger(t, "Error Burger")
		w.review(t, alice, errBurger, 5, "baseline")
		victim := w.review(t, bob, errBurger, 3, "to discard")
		if err := w.reviews.Delete(ctx, bob, victim.ID); err != nil {
			t.Fatalf("Delete returned error: %v", err)
		}
		before := dbtest.RequireConsistentStats(ctx, t, conn, errBurger)

		// 書き込みが失敗する手順は、UnitOfWork.Do を直接使って確かめる（usecase は、書き込みの前に
		// query で review を load するので、存在しない review は書き込みまで届かない）。
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
			t.Errorf("update discarded review = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if err := fail(func(tx usecase.Tx) error {
			_, err := tx.Reviews.UpdateContent(ctx, 99999, 1, "x")
			return err
		}); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("update unknown review = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if err := fail(func(tx usecase.Tx) error { return tx.Reviews.Discard(ctx, victim.ID) }); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("second discard = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if err := fail(func(tx usecase.Tx) error { return tx.Reviews.Discard(ctx, 99999) }); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("unknown discard = %v, want %v", err, domain.ErrReviewNotFound)
		}

		after, ok := dbtest.FetchBurgerStats(ctx, t, conn, errBurger)
		if !ok {
			t.Fatal("burger_stats row disappeared")
		}
		if after != before || !after.CalculatedAt.Equal(before.CalculatedAt) {
			t.Errorf("stats after failed writes = %+v, want unchanged %+v", after, before)
		}
	})

	t.Run("書き込みの後の失敗は、書き込みも統計も rollback する（同一トランザクション）", func(t *testing.T) {
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
			// 再計算まで済ませたあとで失敗する。トランザクション内の読み取りには、この書き込みが見える。
			facts, err := tx.Stats.ListBurgerReviewFacts(ctx, rbBurger)
			if err != nil || len(facts) != 2 {
				t.Errorf("同一トランザクションの facts = %d 件 (err %v), want 未コミットの書き込みが見える 2 件", len(facts), err)
			}
			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatalf("Do error = %v, want %v", err, boom)
		}
		var reviews int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM reviews WHERE burger_id = $1`, rbBurger).Scan(&reviews); err != nil {
			t.Fatalf("count reviews: %v", err)
		}
		if reviews != 1 {
			t.Errorf("review rows = %d, want the doomed write rolled back (1 baseline)", reviews)
		}
		after, _ := dbtest.FetchBurgerStats(ctx, t, conn, rbBurger)
		if after != before {
			t.Errorf("stats = %+v, want the rolled-back recalculation to leave %+v", after, before)
		}
	})
}

// TestUnitOfWorkNamedBurger は、burger_name による find-or-create の投稿経路（S6 P3-1）を、UnitOfWork の
// 中で検証する。burger の再利用と作成に加えて、review の insert が失敗しても、find-or-create した burger や
// link が commit されないこと（単一トランザクションの保証）と、投稿と同じトランザクションで統計が再計算される
// ことを確かめる。
func TestUnitOfWorkNamedBurger(t *testing.T) {
	w := newWorld(t)
	ctx, conn := w.ctx, w.conn
	alice := w.user(t, "alice")
	shopB := dbtest.InsertRow(ctx, t, conn,
		`INSERT INTO shops (name, status, moderation_note, creator_id) VALUES ($1, $2, $3, $4) RETURNING id`,
		"Shop B", 1, nil, nil)

	cheese := w.burger(t, "Cheese")
	if _, err := conn.Exec(ctx,
		`INSERT INTO burger_stats (burger_id, review_count, average_rating, weighted_score, confidence, calculated_at)
		 VALUES ($1, 2, 4.0, 3.9, 0.7, now())`, cheese); err != nil {
		t.Fatalf("seed cheese stats: %v", err)
	}

	countRows := func(t *testing.T, query string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := conn.QueryRow(ctx, query, args...).Scan(&n); err != nil {
			t.Fatalf("count (%s): %v", query, err)
		}
		return n
	}
	burgersNamed := func(t *testing.T, name string) int64 {
		return countRows(t, `SELECT count(*) FROM burgers WHERE name = $1`, name)
	}
	named := func(t *testing.T, viewer domain.User, shopID int64, name string) domain.ReviewDetail {
		t.Helper()
		detail, err := w.reviews.Create(ctx, viewer, shopID, 0, name, 4, "via name", nil)
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		return detail
	}

	t.Run("shop 内に同名の burger があれば再利用し、戻り値の burger は insert 前の stats を持つ", func(t *testing.T) {
		created := named(t, alice, w.shop, "Cheese")
		want := domain.ShopReviewBurger{ID: cheese, Name: "Cheese", AverageRating: 4.0, ReviewCount: 2, WeightedScore: 3.9, Confidence: 0.7}
		if created.Burger == nil || !reflect.DeepEqual(*created.Burger, want) {
			t.Errorf("burger = %+v, want the seeded pre-insert stats %+v", created.Burger, want)
		}
		if created.ID == 0 || created.BurgerID != cheese || created.CreatedAt.IsZero() {
			t.Errorf("created = %+v, want a stored review for burger %d", created.Review, cheese)
		}
		if got := burgersNamed(t, "Cheese"); got != 1 {
			t.Errorf("Cheese burger rows = %d, want no duplicate", got)
		}
		// insert が、同じトランザクション内で stats を再計算した。
		if stats := dbtest.RequireConsistentStats(ctx, t, conn, cheese); stats.ReviewCount != 1 {
			t.Errorf("stats = %+v, want the recalculated count 1 (only the new kept review)", stats)
		}
	})

	t.Run("未知の名前は burger とその shops_burgers の link を作成する", func(t *testing.T) {
		created := named(t, alice, w.shop, "Veggie")
		burger := created.Burger
		if burger == nil || burger.Name != "Veggie" || burger.ID == cheese {
			t.Fatalf("burger = %+v, want a new Veggie row", burger)
		}
		if zero := (domain.ShopReviewBurger{ID: burger.ID, Name: "Veggie"}); !reflect.DeepEqual(*burger, zero) {
			t.Errorf("burger = %+v, want zero pre-insert stats", *burger)
		}
		if created.BurgerID != burger.ID {
			t.Errorf("review burger = %d, want %d", created.BurgerID, burger.ID)
		}
		if got := countRows(t, `SELECT count(*) FROM shops_burgers WHERE shop_id = $1 AND burger_id = $2`, w.shop, burger.ID); got != 1 {
			t.Errorf("link rows = %d, want 1", got)
		}
		if stats := dbtest.RequireConsistentStats(ctx, t, conn, burger.ID); stats.ReviewCount != 1 {
			t.Errorf("stats = %+v, want count 1", stats)
		}
	})

	t.Run("別の shop の同じ名前は別の burger 行になる", func(t *testing.T) {
		burger := named(t, alice, shopB, "Cheese").Burger
		if burger == nil || burger.ID == cheese {
			t.Fatalf("burger = %+v, want a new row distinct from shop A's Cheese %d", burger, cheese)
		}
		if got := burgersNamed(t, "Cheese"); got != 2 {
			t.Errorf("Cheese burger rows = %d, want 2 (one per shop)", got)
		}
		if got := countRows(t, `SELECT count(*) FROM shops_burgers WHERE shop_id = $1 AND burger_id = $2`, shopB, burger.ID); got != 1 {
			t.Errorf("shop B link rows = %d, want 1", got)
		}
		// Shop A の Cheese の link は、元の burger だけを指したままである。
		if got := countRows(t, `SELECT count(*) FROM shops_burgers WHERE shop_id = $1`, w.shop); got != 2 {
			t.Errorf("shop A link rows = %d, want its original Cheese and Veggie", got)
		}
	})

	t.Run("insert に失敗しても孤立した burger や link は commit されない", func(t *testing.T) {
		// 未知の author は、burger と link の insert の後で reviews.user_id の
		// FK に違反する。トランザクション全体が rollback されなければならない。
		ghost := domain.User{ID: uid.N(99999), Username: "ghost"}
		if _, err := w.reviews.Create(ctx, ghost, w.shop, 0, "Ghost", 4, "ok", nil); err == nil {
			t.Fatal("Create returned nil error, want the FK failure")
		}
		if got := burgersNamed(t, "Ghost"); got != 0 {
			t.Errorf("Ghost burger rows = %d, want the rollback to leave none", got)
		}
		if got := countRows(t, `SELECT count(*) FROM shops_burgers sb JOIN burgers b ON b.id = sb.burger_id WHERE b.name = 'Ghost'`); got != 0 {
			t.Errorf("Ghost link rows = %d, want none", got)
		}
		if got := countRows(t, `SELECT count(*) FROM reviews WHERE user_id = $1`, ghost.ID); got != 0 {
			t.Errorf("review rows = %d, want none", got)
		}
	})
}

// TestUnitOfWorkDiscardUser は、S8 の退会（user の discard）と、その user の review が付く burger の統計の
// 再計算を、usecase が UnitOfWork.Do の中で組み立てる形（S17）で検証する。読み取り経路が、discard 済みの
// user の kept な review を隠すことと、並行する退会がデッドロックしないこと（burger_id の昇順のロック）も
// 確かめる。
func TestUnitOfWorkDiscardUser(t *testing.T) {
	w := newWorld(t)
	ctx, conn := w.ctx, w.conn
	alice := w.user(t, "alice")
	victim := w.user(t, "victim")
	shared := w.burger(t, "Shared")
	solo := w.burger(t, "Solo")
	// "shared" は victim と alice の両方が review し、"solo" は victim だけが review した。victim の
	// discard 後、shared は alice の review だけに減り、solo は stats がゼロの行にならなければならない。
	victimShared := w.review(t, victim, shared, 2, "meh")
	aliceShared := w.review(t, alice, shared, 4, "good")
	victimSolo := w.review(t, victim, solo, 5, "only mine")

	t.Run("退会は user に discard 時刻を刻み、その user が review した burger の stats を再計算する", func(t *testing.T) {
		if got := dbtest.RequireConsistentStats(ctx, t, conn, shared); got.ReviewCount != 2 {
			t.Fatalf("shared stats before discard = %+v, want count 2", got)
		}
		if err := w.users.Delete(ctx, victim, victim.ID); err != nil {
			t.Fatalf("Delete returned error: %v", err)
		}
		// soft delete：行はまだ存在し、discarded_at に時刻が刻まれている。
		var discardedAt *time.Time
		if err := conn.QueryRow(ctx, `SELECT discarded_at FROM users WHERE id = $1`, victim.ID).Scan(&discardedAt); err != nil {
			t.Fatalf("select discarded user: %v", err)
		}
		if discardedAt == nil {
			t.Fatal("users.discarded_at is NULL, want a timestamp")
		}
		// Rails parity：victim の review 自体は kept のままである。非表示化は
		// 純粋に読み取り側で行われる。
		var keptReviews int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM reviews WHERE user_id = $1 AND discarded_at IS NULL`, victim.ID).Scan(&keptReviews); err != nil {
			t.Fatalf("count victim reviews: %v", err)
		}
		if keptReviews != 2 {
			t.Errorf("victim kept reviews = %d, want 2 (reviews must not be discarded)", keptReviews)
		}
		// shared は alice の review だけに減る。alice の review は影響を受けない。
		sharedStats := dbtest.RequireConsistentStats(ctx, t, conn, shared)
		if sharedStats.ReviewCount != 1 || sharedStats.AverageRating != 4.0 {
			t.Errorf("shared stats after discard = %+v, want only alice's rating 4", sharedStats)
		}
		// solo は、victim だけが review したので、ゼロの行になる。
		soloStats := dbtest.RequireConsistentStats(ctx, t, conn, solo)
		want := dbtest.StoredBurgerStats{ReviewCount: 0, AverageRating: 0.0, WeightedScore: 0.0, Confidence: 0.0, CalculatedAt: soloStats.CalculatedAt}
		if soloStats != want {
			t.Errorf("solo stats after discard = %+v, want the zero row", soloStats)
		}
	})

	t.Run("2 回目の退会と存在しない id の退会は ErrUserNotFound になる", func(t *testing.T) {
		if err := w.users.Delete(ctx, victim, victim.ID); !errors.Is(err, domain.ErrUserNotFound) {
			t.Errorf("second delete = %v, want %v", err, domain.ErrUserNotFound)
		}
		unknown := domain.User{ID: uid.N(99999)}
		if err := w.users.Delete(ctx, unknown, unknown.ID); !errors.Is(err, domain.ErrUserNotFound) {
			t.Errorf("unknown delete = %v, want %v", err, domain.ErrUserNotFound)
		}
	})

	t.Run("読み取り経路は discard 済みの user の kept な review を隠す", func(t *testing.T) {
		// フィード：alice の review だけが残り、表示される stats は再計算された
		// burger_stats の行（count 1）と一致する。
		feed, _, err := w.queries.review.ListReviews(ctx, usecase.ReviewListFilter{}, 100, 0)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		ids := make([]int64, 0, len(feed))
		for _, r := range feed {
			ids = append(ids, r.ID)
		}
		if want := []int64{aliceShared.ID}; !reflect.DeepEqual(ids, want) {
			t.Fatalf("feed ids = %v, want %v (victim's reviews hidden)", ids, want)
		}
		sharedStats := dbtest.RequireConsistentStats(ctx, t, conn, shared)
		if feed[0].Burger.ReviewCount != sharedStats.ReviewCount || feed[0].Burger.ReviewCount != int64(len(feed)) {
			t.Errorf("displayed review count = %d, want burger_stats %d = %d displayed reviews",
				feed[0].Burger.ReviewCount, sharedStats.ReviewCount, len(feed))
		}
		// 詳細：discard 済みの author の review は、存在しない review と
		// 区別がつかない。alice の review には引き続き到達できる。
		if _, err := w.queries.review.GetReview(ctx, victimShared.ID); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("GetReview(victim shared) = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if _, err := w.queries.review.GetReview(ctx, victimSolo.ID); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("GetReview(victim solo) = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if _, err := w.queries.review.GetReview(ctx, aliceShared.ID); err != nil {
			t.Errorf("GetReview(alice) returned error: %v", err)
		}
		// shop の review：alice の review だけが一覧に載る。
		shopReviews, err := w.queries.shop.ListShopReviews(ctx, w.shop)
		if err != nil {
			t.Fatalf("ListShopReviews returned error: %v", err)
		}
		shopIDs := make([]int64, 0, len(shopReviews))
		for _, r := range shopReviews {
			shopIDs = append(shopIDs, r.ID)
		}
		if want := []int64{aliceShared.ID}; !reflect.DeepEqual(shopIDs, want) {
			t.Errorf("shop review ids = %v, want %v (victim's review hidden)", shopIDs, want)
		}
	})

	t.Run("burger の集合が重なる 2 人の並行する退会は、デッドロックせずに、どちらも成功する", func(t *testing.T) {
		pool, err := pgxpool.New(ctx, w.dbURL)
		if err != nil {
			t.Fatalf("open pool: %v", err)
		}
		t.Cleanup(pool.Close)
		poolUsers := usecase.NewUsers(query.NewUserQuery(pool), domain.NewUsers(repository.NewUserRepository(pool)), uow.New(pool), w.recalc, infra.BcryptPasswordHasher{})

		// 2 人が、同じ 4 つの burger に review する。退会の再計算は、burger_id の昇順にロックするので、
		// 互いに逆順にロックして待ち合う（デッドロック）ことがない。繰り返すのは、運よく interleave
		// しても race が隠れないようにするため。
		for i := 0; i < 5; i++ {
			burgers := make([]int64, 4)
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
					t.Fatalf("iteration %d: concurrent Delete returned error: %v", i, err)
				}
			}
			for _, id := range burgers {
				if stats := dbtest.RequireConsistentStats(ctx, t, conn, id); stats.ReviewCount != 0 {
					t.Fatalf("iteration %d burger %d: stats = %+v, want both users' reviews excluded", i, id, stats)
				}
			}
		}
	})
}
