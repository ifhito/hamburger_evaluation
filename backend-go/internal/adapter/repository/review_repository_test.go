package repository_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// reviewRow は、reviews の保存された行である（DB のカラムの値のまま）。
type reviewRow struct {
	Rating      int16
	Comment     *string
	UserID      string
	BurgerID    int64
	CreatedAt   time.Time
	DiscardedAt *time.Time
	PhotoKey    *string
}

// readReviewRow は reviews の行を直接読み取る（discard 済みの行も読める）。
func readReviewRow(ctx context.Context, t *testing.T, conn *pgx.Conn, id int64) reviewRow {
	t.Helper()
	var r reviewRow
	if err := conn.QueryRow(ctx,
		`SELECT rating, comment, user_id, burger_id, created_at, discarded_at, photo_key FROM reviews WHERE id = $1`, id,
	).Scan(&r.Rating, &r.Comment, &r.UserID, &r.BurgerID, &r.CreatedAt, &r.DiscardedAt, &r.PhotoKey); err != nil {
		t.Fatalf("select review %d: %v", id, err)
	}
	return r
}

// TestReviewRepository は、S6 の review の書き込み（作成・カラム単位の更新・soft delete）を、
// 共有の dbtest のスキャフォールドを通じて実際の PostgreSQL に対して検証する
// （TEST_DATABASE_URL がなければスキップする）。書き込みの結果は SQL で直接確かめる
// （フィード・詳細・filter などの読み取りは adapter/query のテストが担う）。
func TestReviewRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	repo := repository.NewReviewRepository(conn)

	insertUser := `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, 'x', $3) RETURNING id`
	alice := dbtest.InsertUserRow(ctx, t, conn, insertUser, "alice@example.com", "alice", false)
	carol := dbtest.InsertUserRow(ctx, t, conn, insertUser, "carol@example.com", "carol", false)

	cheese := dbtest.InsertRow(ctx, t, conn, `INSERT INTO burgers (name) VALUES ($1) RETURNING id`, "Cheese")

	insertReview := `INSERT INTO reviews (rating, comment, user_id, burger_id, discarded_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
	t1 := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 5, 2, 10, 0, 0, 0, time.UTC)
	rOld := dbtest.InsertRow(ctx, t, conn, insertReview, 5, "Tasty", alice, cheese, nil, t1)
	rDiscarded := dbtest.InsertRow(ctx, t, conn, insertReview, 1, "gone", alice, cheese, time.Now(), t2)

	t.Run("CreateReview は insert して保存された行を返す", func(t *testing.T) {
		review, err := domain.NewReview(4, "Fresh", carol, cheese)
		if err != nil {
			t.Fatalf("NewReview returned error: %v", err)
		}
		created, err := repo.CreateReview(ctx, review)
		if err != nil {
			t.Fatalf("CreateReview returned error: %v", err)
		}
		if created.ID == 0 || created.Rating != 4 || created.AuthorID != carol || created.BurgerID != cheese {
			t.Errorf("created = %+v, want generated id with the given fields", created)
		}
		if created.Comment == nil || *created.Comment != "Fresh" {
			t.Errorf("comment = %v, want Fresh", created.Comment)
		}
		if created.CreatedAt.IsZero() {
			t.Error("CreatedAt is zero, want the DB timestamp")
		}
		// 保存された行：渡した値で、kept（discarded_at なし）のまま。
		stored := readReviewRow(ctx, t, conn, created.ID)
		if stored.Rating != 4 || stored.UserID != carol || stored.BurgerID != cheese ||
			stored.Comment == nil || *stored.Comment != "Fresh" || stored.DiscardedAt != nil {
			t.Errorf("stored = %+v, want the created review (kept)", stored)
		}
	})

	t.Run("UpdateReviewContent は rating と comment だけを書き込む", func(t *testing.T) {
		updated, err := repo.UpdateReviewContent(ctx, rOld, 2, "Changed my mind")
		if err != nil {
			t.Fatalf("UpdateReviewContent returned error: %v", err)
		}
		if updated.Rating != 2 || updated.Comment == nil || *updated.Comment != "Changed my mind" {
			t.Errorf("updated = %+v, want rating 2 and the new comment", updated)
		}
		if !updated.CreatedAt.Equal(t1) {
			t.Errorf("CreatedAt = %v, want unchanged %v", updated.CreatedAt, t1)
		}
		// discarded_at には触れていない：review は依然として kept であり、保存された行に
		// rating と comment の変更だけが反映されている。
		stored := readReviewRow(ctx, t, conn, rOld)
		if stored.Rating != 2 || stored.Comment == nil || *stored.Comment != "Changed my mind" || stored.DiscardedAt != nil {
			t.Errorf("stored = %+v, want the new rating and comment, still kept", stored)
		}
		if !stored.CreatedAt.Equal(t1) || stored.UserID != alice || stored.BurgerID != cheese {
			t.Errorf("stored = %+v, want created_at, user and burger untouched", stored)
		}
	})

	t.Run("UpdateReviewContent に discard 済みまたは存在しない review を渡すと ErrReviewNotFound になる", func(t *testing.T) {
		for name, id := range map[string]int64{"discarded": rDiscarded, "unknown": 99999} {
			if _, err := repo.UpdateReviewContent(ctx, id, 3, "x"); !errors.Is(err, domain.ErrReviewNotFound) {
				t.Errorf("%s: error = %v, want %v", name, err, domain.ErrReviewNotFound)
			}
		}
	})

	t.Run("DiscardReview は soft delete をちょうど 1 回だけ行い、hard delete はしない", func(t *testing.T) {
		victim := dbtest.InsertRow(ctx, t, conn, insertReview, 3, "bye", carol, cheese, nil, t2)
		if err := repo.DiscardReview(ctx, victim); err != nil {
			t.Fatalf("DiscardReview returned error: %v", err)
		}
		// 行はまだ存在し（soft delete）、discarded_at に時刻が刻まれている。
		var discardedAt *time.Time
		if err := conn.QueryRow(ctx, `SELECT discarded_at FROM reviews WHERE id = $1`, victim).Scan(&discardedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				t.Fatal("review row was hard-deleted, want soft delete")
			}
			t.Fatalf("select discarded review: %v", err)
		}
		if discardedAt == nil {
			t.Error("discarded_at is NULL, want a timestamp")
		}
		// 2 回目の discard はどの行にも一致しない。
		if err := repo.DiscardReview(ctx, victim); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("second discard = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if err := repo.DiscardReview(ctx, 99999); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("unknown discard = %v, want %v", err, domain.ErrReviewNotFound)
		}
	})
}

// TestReviewRepositoryCreateReviewForNamedBurger は、burger_name による
// find-or-create の投稿経路（S6 P3-1）を検証する。名前の完全一致による
// shop 単位での再利用、未知の名前に対する burger と link の作成、shop ごとの
// 名前のスコープ（別の shop の同じ名前は別の burger 行になる）、および
// 単一トランザクションの保証（insert に失敗しても、孤立した burger や link が
// commit されない）である。
func TestReviewRepositoryCreateReviewForNamedBurger(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	repo := repository.NewReviewRepository(conn)

	insertUser := `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, 'x', $3) RETURNING id`
	alice := dbtest.InsertUserRow(ctx, t, conn, insertUser, "alice@example.com", "alice", false)

	insertShop := `INSERT INTO shops (name, status, moderation_note, creator_id) VALUES ($1, $2, $3, $4) RETURNING id`
	shopA := dbtest.InsertRow(ctx, t, conn, insertShop, "Shop A", 1, nil, alice)
	shopB := dbtest.InsertRow(ctx, t, conn, insertShop, "Shop B", 1, nil, nil)

	cheese := dbtest.InsertRow(ctx, t, conn, `INSERT INTO burgers (name) VALUES ($1) RETURNING id`, "Cheese")
	if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shopA, cheese); err != nil {
		t.Fatalf("link shop A cheese: %v", err)
	}
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

	mustNamedCreate := func(t *testing.T, shopID int64, name string) (domain.Review, domain.ShopReviewBurger) {
		t.Helper()
		review, err := domain.NewReview(4, "via name", alice, 0)
		if err != nil {
			t.Fatalf("NewReview returned error: %v", err)
		}
		created, burger, err := repo.CreateReviewForNamedBurger(ctx, shopID, name, review)
		if err != nil {
			t.Fatalf("CreateReviewForNamedBurger returned error: %v", err)
		}
		return created, burger
	}

	t.Run("shop 内に同名の burger があれば再利用し、戻り値の burger は insert 前の stats を持つ", func(t *testing.T) {
		created, burger := mustNamedCreate(t, shopA, "Cheese")
		if burger.ID != cheese {
			t.Fatalf("burger id = %d, want the existing Cheese %d", burger.ID, cheese)
		}
		want := domain.ShopReviewBurger{ID: cheese, Name: "Cheese", AverageRating: 4.0, ReviewCount: 2, WeightedScore: 3.9, Confidence: 0.7}
		if !reflect.DeepEqual(burger, want) {
			t.Errorf("burger = %+v, want the seeded pre-insert stats %+v", burger, want)
		}
		if created.ID == 0 || created.BurgerID != cheese || created.CreatedAt.IsZero() {
			t.Errorf("created = %+v, want a stored review for burger %d", created, cheese)
		}
		if got := burgersNamed(t, "Cheese"); got != 1 {
			t.Errorf("Cheese burger rows = %d, want no duplicate", got)
		}
		// insert が、同じトランザクション内で stats を再計算した。
		if stats := requireConsistentStats(ctx, t, conn, cheese); stats.ReviewCount != 1 {
			t.Errorf("stats = %+v, want the recalculated count 1 (only the new kept review)", stats)
		}
	})

	t.Run("未知の名前は burger とその shops_burgers の link を作成する", func(t *testing.T) {
		created, burger := mustNamedCreate(t, shopA, "Veggie")
		if burger.Name != "Veggie" || burger.ID == cheese {
			t.Fatalf("burger = %+v, want a new Veggie row", burger)
		}
		if zero := (domain.ShopReviewBurger{ID: burger.ID, Name: "Veggie"}); !reflect.DeepEqual(burger, zero) {
			t.Errorf("burger = %+v, want zero pre-insert stats", burger)
		}
		if created.BurgerID != burger.ID {
			t.Errorf("review burger = %d, want %d", created.BurgerID, burger.ID)
		}
		if got := countRows(t, `SELECT count(*) FROM shops_burgers WHERE shop_id = $1 AND burger_id = $2`, shopA, burger.ID); got != 1 {
			t.Errorf("link rows = %d, want 1", got)
		}
		if stats := requireConsistentStats(ctx, t, conn, burger.ID); stats.ReviewCount != 1 {
			t.Errorf("stats = %+v, want count 1", stats)
		}
	})

	t.Run("別の shop の同じ名前は別の burger 行になる", func(t *testing.T) {
		_, burger := mustNamedCreate(t, shopB, "Cheese")
		if burger.ID == cheese {
			t.Fatalf("burger id = %d, want a new row distinct from shop A's Cheese %d", burger.ID, cheese)
		}
		if got := burgersNamed(t, "Cheese"); got != 2 {
			t.Errorf("Cheese burger rows = %d, want 2 (one per shop)", got)
		}
		if got := countRows(t, `SELECT count(*) FROM shops_burgers WHERE shop_id = $1 AND burger_id = $2`, shopB, burger.ID); got != 1 {
			t.Errorf("shop B link rows = %d, want 1", got)
		}
		// Shop A の Cheese の link は、元の burger だけを指したままである。
		if got := countRows(t, `SELECT count(*) FROM shops_burgers WHERE shop_id = $1`, shopA); got != 2 {
			t.Errorf("shop A link rows = %d, want its original Cheese and Veggie", got)
		}
	})

	t.Run("insert に失敗しても孤立した burger や link は commit されない", func(t *testing.T) {
		// 未知の author は、burger と link の insert の後で reviews.user_id の
		// FK に違反する。トランザクション全体が rollback されなければならない。
		review := domain.Review{Rating: 4, AuthorID: uid.N(99999)}
		if _, _, err := repo.CreateReviewForNamedBurger(ctx, shopA, "Ghost", review); err == nil {
			t.Fatal("CreateReviewForNamedBurger returned nil error, want the FK failure")
		}
		if got := burgersNamed(t, "Ghost"); got != 0 {
			t.Errorf("Ghost burger rows = %d, want the rollback to leave none", got)
		}
		if got := countRows(t, `SELECT count(*) FROM shops_burgers sb JOIN burgers b ON b.id = sb.burger_id WHERE b.name = 'Ghost'`); got != 0 {
			t.Errorf("Ghost link rows = %d, want none", got)
		}
		if got := countRows(t, `SELECT count(*) FROM reviews WHERE user_id = $1`, uid.N(99999)); got != 0 {
			t.Errorf("review rows = %d, want none", got)
		}
	})
}

// storedBurgerStats は、テストで読み戻した burger_stats の行である。
type storedBurgerStats struct {
	ReviewCount   int64
	AverageRating float64
	WeightedScore float64
	Confidence    float64
	CalculatedAt  time.Time
}

// fetchBurgerStats は burger_stats の行を直接読み取る。行が存在しない場合、
// ok は false になる。
func fetchBurgerStats(ctx context.Context, t *testing.T, conn *pgx.Conn, burgerID int64) (storedBurgerStats, bool) {
	t.Helper()
	var s storedBurgerStats
	err := conn.QueryRow(ctx,
		`SELECT review_count, average_rating, weighted_score, confidence, calculated_at
		 FROM burger_stats WHERE burger_id = $1`, burgerID,
	).Scan(&s.ReviewCount, &s.AverageRating, &s.WeightedScore, &s.Confidence, &s.CalculatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return storedBurgerStats{}, false
	}
	if err != nil {
		t.Fatalf("fetch burger stats: %v", err)
	}
	return s, true
}

// keptReviewFacts は、burger の kept な review のうち kept な user のものを
// （repository が使うのと同じルールで）domain の fact として読み込み、
// 各 fact の author の、すべての burger にわたる kept な rating を
// reviewer の履歴として付ける。
func keptReviewFacts(ctx context.Context, t *testing.T, conn *pgx.Conn, burgerID int64) []domain.ReviewFact {
	t.Helper()
	rows, err := conn.Query(ctx,
		`SELECT r.rating, r.created_at, r.user_id
		 FROM reviews r JOIN users u ON u.id = r.user_id
		 WHERE r.burger_id = $1 AND r.discarded_at IS NULL AND u.discarded_at IS NULL
		 ORDER BY r.id`, burgerID)
	if err != nil {
		t.Fatalf("query review facts: %v", err)
	}
	type factRow struct {
		rating    int16
		createdAt time.Time
		userID    string
	}
	var factRows []factRow
	for rows.Next() {
		var fr factRow
		if err := rows.Scan(&fr.rating, &fr.createdAt, &fr.userID); err != nil {
			t.Fatalf("scan review fact: %v", err)
		}
		factRows = append(factRows, fr)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate review facts: %v", err)
	}
	facts := make([]domain.ReviewFact, 0, len(factRows))
	for _, fr := range factRows {
		facts = append(facts, domain.ReviewFact{
			Rating:          float64(fr.rating),
			CreatedAt:       fr.createdAt,
			ReviewerHistory: domain.ReviewerHistory{Ratings: keptRatingsOf(ctx, t, conn, fr.userID)},
		})
	}
	return facts
}

// keptRatingsOf は、user の、すべての burger にわたる kept な rating を id の
// 昇順で返す（reviewer-trust の履歴）。
func keptRatingsOf(ctx context.Context, t *testing.T, conn *pgx.Conn, userID string) []float64 {
	t.Helper()
	rows, err := conn.Query(ctx,
		`SELECT rating FROM reviews WHERE user_id = $1 AND discarded_at IS NULL ORDER BY id`, userID)
	if err != nil {
		t.Fatalf("query reviewer history: %v", err)
	}
	var ratings []float64
	for rows.Next() {
		var rating int16
		if err := rows.Scan(&rating); err != nil {
			t.Fatalf("scan reviewer rating: %v", err)
		}
		ratings = append(ratings, float64(rating))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate reviewer history: %v", err)
	}
	return ratings
}

// requireConsistentStats は、保存された burger_stats の行が存在し、保存された
// review の行と保存された calculated_at から domain の関数で再計算した結果と
// 完全に一致することをアサートし（repository が "now" を timestamptz の精度に
// 切り詰めるのは、まさにこれが往復しても一致するようにするためである）、その
// 行を返す。float は厳密に比較する：同じ入力を同じ純粋関数に通せば、同一の値に
// ならなければならない。
func requireConsistentStats(ctx context.Context, t *testing.T, conn *pgx.Conn, burgerID int64) storedBurgerStats {
	t.Helper()
	got, ok := fetchBurgerStats(ctx, t, conn, burgerID)
	if !ok {
		t.Fatalf("burger %d has no burger_stats row, want one", burgerID)
	}
	facts := keptReviewFacts(ctx, t, conn, burgerID)
	score := domain.CalculateBurgerScore(facts, got.CalculatedAt)
	want := storedBurgerStats{
		ReviewCount:   int64(len(facts)),
		AverageRating: domain.AverageRating(facts),
		WeightedScore: score.WeightedAverage,
		Confidence:    score.Confidence,
		CalculatedAt:  got.CalculatedAt,
	}
	if got != want {
		t.Fatalf("stored stats = %+v, want recomputed %+v", got, want)
	}
	return got
}

// mustCreateReview は repository を通して review を構築し永続化する。
func mustCreateReview(ctx context.Context, t *testing.T, repo *repository.ReviewRepository, rating int, comment string, authorID string, burgerID int64) domain.Review {
	t.Helper()
	review, err := domain.NewReview(rating, comment, authorID, burgerID)
	if err != nil {
		t.Fatalf("NewReview returned error: %v", err)
	}
	created, err := repo.CreateReview(ctx, review)
	if err != nil {
		t.Fatalf("CreateReview returned error: %v", err)
	}
	return created
}

// TestReviewRepositoryBurgerStats は、S7 の同一トランザクション内での
// burger_stats の再計算（issue #15）を検証する。review の書き込みのたびに、
// stats の行は、kept な user の kept な review に対する domain の calculator と
// 厳密に整合した状態になり、並行する書き込みが更新を失うことはなく、
// 失敗した書き込みは stats に手を付けない。
func TestReviewRepositoryBurgerStats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	conn, dbURL := dbtest.New(t)
	repo := repository.NewReviewRepository(conn)

	insertUser := `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, 'x', $3) RETURNING id`
	alice := dbtest.InsertUserRow(ctx, t, conn, insertUser, "alice@example.com", "alice", false)
	bob := dbtest.InsertUserRow(ctx, t, conn, insertUser, "bob@example.com", "bob", false)

	insertBurger := `INSERT INTO burgers (name) VALUES ($1) RETURNING id`
	burger := dbtest.InsertRow(ctx, t, conn, insertBurger, "Stats Burger")

	var aliceReview domain.Review

	t.Run("AC1 CreateReview は同一トランザクション内で stats を upsert する", func(t *testing.T) {
		aliceReview = mustCreateReview(ctx, t, repo, 5, "great", alice, burger)
		stats := requireConsistentStats(ctx, t, conn, burger)
		if stats.ReviewCount != 1 || stats.AverageRating != 5.0 {
			t.Errorf("stats after first review = %+v, want count 1 and average 5.0", stats)
		}

		mustCreateReview(ctx, t, repo, 4, "good", bob, burger)
		stats = requireConsistentStats(ctx, t, conn, burger)
		if stats.ReviewCount != 2 || stats.AverageRating != 4.5 {
			t.Errorf("stats after second review = %+v, want count 2 and average 4.5", stats)
		}
	})

	t.Run("UpdateReviewContent は stats を再計算する", func(t *testing.T) {
		before := requireConsistentStats(ctx, t, conn, burger)
		if _, err := repo.UpdateReviewContent(ctx, aliceReview.ID, 1, "changed my mind"); err != nil {
			t.Fatalf("UpdateReviewContent returned error: %v", err)
		}
		stats := requireConsistentStats(ctx, t, conn, burger)
		if stats.ReviewCount != 2 || stats.AverageRating != 2.5 {
			t.Errorf("stats after edit = %+v, want count 2 and average 2.5", stats)
		}
		if stats.WeightedScore == before.WeightedScore {
			t.Errorf("weighted score stayed %v after a 5→1 edit, want a change", stats.WeightedScore)
		}
	})

	t.Run("AC2 DiscardReview は discard した review を除いて再計算する", func(t *testing.T) {
		if err := repo.DiscardReview(ctx, aliceReview.ID); err != nil {
			t.Fatalf("DiscardReview returned error: %v", err)
		}
		stats := requireConsistentStats(ctx, t, conn, burger)
		if stats.ReviewCount != 1 || stats.AverageRating != 4.0 {
			t.Errorf("stats after discard = %+v, want only bob's rating 4 left", stats)
		}
	})

	t.Run("AC2 唯一の review を discard すると stats はゼロの行になる", func(t *testing.T) {
		var bobReviewID int64
		if err := conn.QueryRow(ctx,
			`SELECT id FROM reviews WHERE burger_id = $1 AND discarded_at IS NULL`, burger,
		).Scan(&bobReviewID); err != nil {
			t.Fatalf("find remaining review: %v", err)
		}
		if err := repo.DiscardReview(ctx, bobReviewID); err != nil {
			t.Fatalf("DiscardReview returned error: %v", err)
		}
		stats := requireConsistentStats(ctx, t, conn, burger)
		want := storedBurgerStats{ReviewCount: 0, AverageRating: 0.0, WeightedScore: 0.0, Confidence: 0.0, CalculatedAt: stats.CalculatedAt}
		if stats != want {
			t.Errorf("stats after last discard = %+v, want the zero row", stats)
		}
	})

	t.Run("AC4 discard 済みの user の review と履歴は除外される", func(t *testing.T) {
		ac4Burger := dbtest.InsertRow(ctx, t, conn, insertBurger, "AC4 Burger")
		carl := dbtest.InsertUserRow(ctx, t, conn, insertUser, "carl@example.com", "carl", false)
		aliceAC4 := mustCreateReview(ctx, t, repo, 5, "mine stays", alice, ac4Burger)
		mustCreateReview(ctx, t, repo, 2, "mine vanishes", carl, ac4Burger)
		if got := requireConsistentStats(ctx, t, conn, ac4Burger); got.ReviewCount != 2 {
			t.Fatalf("stats before user discard = %+v, want count 2", got)
		}

		if _, err := conn.Exec(ctx, `UPDATE users SET discarded_at = now() WHERE id = $1`, carl); err != nil {
			t.Fatalf("discard user: %v", err)
		}
		// kept な user の書き込みで再計算を発火させる。
		if _, err := repo.UpdateReviewContent(ctx, aliceAC4.ID, 4, "still here"); err != nil {
			t.Fatalf("UpdateReviewContent returned error: %v", err)
		}

		stats := requireConsistentStats(ctx, t, conn, ac4Burger)
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
			ReviewerHistory: domain.ReviewerHistory{Ratings: keptRatingsOf(ctx, t, conn, alice)},
		}}
		if want := domain.CalculateBurgerScore(aliceOnly, stats.CalculatedAt); stats.WeightedScore != want.WeightedAverage || stats.Confidence != want.Confidence {
			t.Errorf("stats = %+v, want score %+v from alice's fact and history alone", stats, want)
		}
	})

	t.Run("AC3 1 つの burger への並行する create で更新が失われない", func(t *testing.T) {
		pool, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			t.Fatalf("open pool: %v", err)
		}
		t.Cleanup(pool.Close)
		poolRepo := repository.NewReviewRepository(pool)

		dave := dbtest.InsertUserRow(ctx, t, conn, insertUser, "dave@example.com", "dave", false)
		erin := dbtest.InsertUserRow(ctx, t, conn, insertUser, "erin@example.com", "erin", false)

		// FOR UPDATE による直列化がなければ、2 つのトランザクションはどちらも
		// 相手の review が欠けたスナップショットを読み、後の upsert が
		// review_count 1 を書き込む（lost update）。新しい burger で繰り返す
		// のは、運よくうまく interleave しても race が隠れないようにするため
		// である。
		for i := 0; i < 5; i++ {
			raceBurger := dbtest.InsertRow(ctx, t, conn, insertBurger, fmt.Sprintf("Race Burger %d", i))
			daveReview, err := domain.NewReview(5, "race", dave, raceBurger)
			if err != nil {
				t.Fatalf("NewReview returned error: %v", err)
			}
			erinReview, err := domain.NewReview(3, "race", erin, raceBurger)
			if err != nil {
				t.Fatalf("NewReview returned error: %v", err)
			}
			start := make(chan struct{})
			errs := make(chan error, 2)
			for _, review := range []domain.Review{daveReview, erinReview} {
				review := review
				go func() {
					<-start
					_, err := poolRepo.CreateReview(ctx, review)
					errs <- err
				}()
			}
			close(start)
			for j := 0; j < 2; j++ {
				if err := <-errs; err != nil {
					t.Fatalf("iteration %d: concurrent CreateReview returned error: %v", i, err)
				}
			}
			stats := requireConsistentStats(ctx, t, conn, raceBurger)
			if stats.ReviewCount != 2 || stats.AverageRating != 4.0 {
				t.Fatalf("iteration %d: stats = %+v, want count 2 and average 4.0 (both writers)", i, stats)
			}
		}
	})

	t.Run("失敗した書き込みは burger_stats に手を付けない", func(t *testing.T) {
		errBurger := dbtest.InsertRow(ctx, t, conn, insertBurger, "Error Burger")
		mustCreateReview(ctx, t, repo, 5, "baseline", alice, errBurger)
		victim := mustCreateReview(ctx, t, repo, 3, "to discard", bob, errBurger)
		if err := repo.DiscardReview(ctx, victim.ID); err != nil {
			t.Fatalf("DiscardReview returned error: %v", err)
		}
		before := requireConsistentStats(ctx, t, conn, errBurger)

		if _, err := repo.UpdateReviewContent(ctx, victim.ID, 1, "x"); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("update discarded review = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if _, err := repo.UpdateReviewContent(ctx, 99999, 1, "x"); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("update unknown review = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if err := repo.DiscardReview(ctx, victim.ID); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("second discard = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if err := repo.DiscardReview(ctx, 99999); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("unknown discard = %v, want %v", err, domain.ErrReviewNotFound)
		}

		after, ok := fetchBurgerStats(ctx, t, conn, errBurger)
		if !ok {
			t.Fatal("burger_stats row disappeared")
		}
		if after != before || !after.CalculatedAt.Equal(before.CalculatedAt) {
			t.Errorf("stats after failed writes = %+v, want unchanged %+v", after, before)
		}
	})
}

// TestReviewRepositoryPhotoKey は、S10 の photo_key の永続化を検証する。
// CreateReview が key を保存し、UpdateReviewContentAndPhotoKey が、まだ kept な review の
// content と key を一緒に入れ替える。書き込みの結果は SQL で直接確かめる（結合された読み取り
// クエリが photo_key を返すことは、adapter/query のテストが担う）。
func TestReviewRepositoryPhotoKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	repo := repository.NewReviewRepository(conn)

	alice := dbtest.InsertUserRow(ctx, t, conn,
		`INSERT INTO users (email, username, password_digest) VALUES ($1, $2, 'x') RETURNING id`,
		"alice@example.com", "alice")
	burger := dbtest.InsertRow(ctx, t, conn,
		`INSERT INTO burgers (name) VALUES ($1) RETURNING id`, "Cheese")

	comment := "Tasty"
	created, err := repo.CreateReview(ctx, domain.Review{
		Rating: 4, Comment: &comment, AuthorID: alice, BurgerID: burger, PhotoKey: strPtr("reviews/abc.jpg"),
	})
	if err != nil {
		t.Fatalf("CreateReview returned error: %v", err)
	}
	if created.PhotoKey == nil || *created.PhotoKey != "reviews/abc.jpg" {
		t.Fatalf("created PhotoKey = %v, want reviews/abc.jpg", created.PhotoKey)
	}

	t.Run("CreateReview は photo_key を保存する", func(t *testing.T) {
		stored := readReviewRow(ctx, t, conn, created.ID)
		if stored.PhotoKey == nil || *stored.PhotoKey != "reviews/abc.jpg" {
			t.Errorf("stored photo_key = %v, want reviews/abc.jpg", stored.PhotoKey)
		}
	})

	t.Run("UpdateReviewContentAndPhotoKey は content と key を一緒に書き込む", func(t *testing.T) {
		updated, err := repo.UpdateReviewContentAndPhotoKey(ctx, created.ID, 5, "Even better", strPtr("reviews/both.png"))
		if err != nil {
			t.Fatalf("UpdateReviewContentAndPhotoKey returned error: %v", err)
		}
		if updated.Rating != 5 || updated.Comment == nil || *updated.Comment != "Even better" {
			t.Errorf("updated = %+v, want rating 5 and the new comment", updated)
		}
		if updated.PhotoKey == nil || *updated.PhotoKey != "reviews/both.png" {
			t.Errorf("updated PhotoKey = %v, want reviews/both.png", updated.PhotoKey)
		}
		// commit された行は content と key の「両方」を持つ（1 つの
		// トランザクションなので、key を伴わない content だけになることは
		// 決してない）。
		stored := readReviewRow(ctx, t, conn, created.ID)
		if stored.Rating != 5 || stored.Comment == nil || *stored.Comment != "Even better" ||
			stored.PhotoKey == nil || *stored.PhotoKey != "reviews/both.png" {
			t.Errorf("stored = rating %d, comment %v, key %v, want 5, Even better and reviews/both.png",
				stored.Rating, stored.Comment, stored.PhotoKey)
		}
	})

	t.Run("key なしの create は NULL のままになる", func(t *testing.T) {
		plain, err := repo.CreateReview(ctx, domain.Review{Rating: 3, Comment: &comment, AuthorID: alice, BurgerID: burger})
		if err != nil {
			t.Fatalf("CreateReview returned error: %v", err)
		}
		if plain.PhotoKey != nil {
			t.Errorf("PhotoKey = %v, want nil", plain.PhotoKey)
		}
	})

	t.Run("discard 済みの review への UpdateReviewContentAndPhotoKey は ErrReviewNotFound になり、変更は残らない", func(t *testing.T) {
		if err := repo.DiscardReview(ctx, created.ID); err != nil {
			t.Fatalf("DiscardReview returned error: %v", err)
		}
		// 結合した書き込みは rollback される：content の変更は何も残らない。
		if _, err := repo.UpdateReviewContentAndPhotoKey(ctx, created.ID, 1, "ghost", strPtr("reviews/ghost.jpg")); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Fatalf("UpdateReviewContentAndPhotoKey error = %v, want %v", err, domain.ErrReviewNotFound)
		}
		var rating int16
		var key *string
		if err := conn.QueryRow(ctx, `SELECT rating, photo_key FROM reviews WHERE id = $1`, created.ID).Scan(&rating, &key); err != nil {
			t.Fatalf("select discarded review: %v", err)
		}
		if rating == 1 || (key != nil && *key == "reviews/ghost.jpg") {
			t.Errorf("discarded review row = rating %d, key %v; the failed combined write leaked a change", rating, key)
		}
	})
}
