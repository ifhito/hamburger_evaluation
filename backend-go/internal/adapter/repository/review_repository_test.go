package repository_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/rowmap"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// reviewRow は、reviews の保存された行である（DB のカラムの値のまま）。
type reviewRow struct {
	Rating      int16
	Comment     *string
	UserID      string
	BurgerID    string
	CreatedAt   time.Time
	DiscardedAt *time.Time
	PhotoKey    *string
	VisitedAt   *time.Time
}

// readReviewRow は reviews の行を直接読み取る（discard 済みの行も読める）。visited_at は
// date 列なので、他の adapter と同じく pgtype.Date で受けてから rowmap.VisitedAt で変換する
// （pgx は date を *time.Time に直接 Scan できない）。
func readReviewRow(ctx context.Context, t *testing.T, conn *pgx.Conn, id string) reviewRow {
	t.Helper()
	var r reviewRow
	var visitedAt pgtype.Date
	if err := conn.QueryRow(ctx,
		`SELECT rating, comment, user_id, burger_id, created_at, discarded_at, photo_key, visited_at FROM reviews WHERE id = $1`, id,
	).Scan(&r.Rating, &r.Comment, &r.UserID, &r.BurgerID, &r.CreatedAt, &r.DiscardedAt, &r.PhotoKey, &visitedAt); err != nil {
		t.Fatalf("select review %s: %v", id, err)
	}
	r.VisitedAt = rowmap.VisitedAt(visitedAt)
	return r
}

// TestReviewRepository は、review の書き込み（作成・カラム単位の更新・soft delete）を、
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

	cheese := dbtest.InsertUUIDRow(ctx, t, conn, `INSERT INTO burgers (name) VALUES ($1) RETURNING id`, "Cheese")

	insertReview := `INSERT INTO reviews (rating, comment, user_id, burger_id, discarded_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
	t1 := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 5, 2, 10, 0, 0, 0, time.UTC)
	rOld := dbtest.InsertUUIDRow(ctx, t, conn, insertReview, 5, "Tasty", alice, cheese, nil, t1)
	rDiscarded := dbtest.InsertUUIDRow(ctx, t, conn, insertReview, 1, "gone", alice, cheese, time.Now(), t2)

	t.Run("CreateReview は insert して保存された行を返す（実食日つき）", func(t *testing.T) {
		visitedAt := time.Date(2024, 4, 20, 0, 0, 0, 0, time.UTC)
		review, err := domain.NewReview(4, "Fresh", carol, cheese, &visitedAt, time.Now())
		if err != nil {
			t.Fatalf("NewReview returned error: %v", err)
		}
		created, err := repo.CreateReview(ctx, review)
		if err != nil {
			t.Fatalf("CreateReview returned error: %v", err)
		}
		if created.ID == "" || created.Rating != 4 || created.AuthorID != carol || created.BurgerID != cheese {
			t.Errorf("created = %+v, want generated id with the given fields", created)
		}
		if created.Comment == nil || *created.Comment != "Fresh" {
			t.Errorf("comment = %v, want Fresh", created.Comment)
		}
		if created.CreatedAt.IsZero() {
			t.Error("CreatedAt is zero, want the DB timestamp")
		}
		if created.VisitedAt == nil || !created.VisitedAt.Equal(visitedAt) {
			t.Errorf("created VisitedAt = %v, want %v", created.VisitedAt, visitedAt)
		}
		// 保存された行：渡した値で、kept（discarded_at なし）のまま。visited_at も、
		// SQL で直接読み取った行が、渡したとおりの日付を持つ（date 列の丸め・時差ずれがないこと）。
		stored := readReviewRow(ctx, t, conn, created.ID)
		if stored.Rating != 4 || stored.UserID != carol || stored.BurgerID != cheese ||
			stored.Comment == nil || *stored.Comment != "Fresh" || stored.DiscardedAt != nil {
			t.Errorf("stored = %+v, want the created review (kept)", stored)
		}
		if stored.VisitedAt == nil || !stored.VisitedAt.Equal(visitedAt) {
			t.Errorf("stored VisitedAt = %v, want %v", stored.VisitedAt, visitedAt)
		}
	})

	t.Run("UpdateReviewContent は rating・comment・実食日を書き込むが、discarded_at には触れない", func(t *testing.T) {
		visitedAt := time.Date(2024, 5, 10, 0, 0, 0, 0, time.UTC)
		updated, err := repo.UpdateReviewContent(ctx, rOld, 2, "Changed my mind", &visitedAt)
		if err != nil {
			t.Fatalf("UpdateReviewContent returned error: %v", err)
		}
		if updated.Rating != 2 || updated.Comment == nil || *updated.Comment != "Changed my mind" {
			t.Errorf("updated = %+v, want rating 2 and the new comment", updated)
		}
		if updated.VisitedAt == nil || !updated.VisitedAt.Equal(visitedAt) {
			t.Errorf("updated VisitedAt = %v, want %v", updated.VisitedAt, visitedAt)
		}
		if !updated.CreatedAt.Equal(t1) {
			t.Errorf("CreatedAt = %v, want unchanged %v", updated.CreatedAt, t1)
		}
		// discarded_at には触れていない：review は依然として kept であり、保存された行に
		// rating・comment・visited_at の変更だけが反映されている。
		stored := readReviewRow(ctx, t, conn, rOld)
		if stored.Rating != 2 || stored.Comment == nil || *stored.Comment != "Changed my mind" || stored.DiscardedAt != nil {
			t.Errorf("stored = %+v, want the new rating and comment, still kept", stored)
		}
		if stored.VisitedAt == nil || !stored.VisitedAt.Equal(visitedAt) {
			t.Errorf("stored VisitedAt = %v, want %v", stored.VisitedAt, visitedAt)
		}
		if !stored.CreatedAt.Equal(t1) || stored.UserID != alice || stored.BurgerID != cheese {
			t.Errorf("stored = %+v, want created_at, user and burger untouched", stored)
		}
	})

	t.Run("UpdateReviewContent に visitedAt として nil を渡すと、実食日は NULL に戻る（部分更新ではなく全置換）", func(t *testing.T) {
		// 直前の subtest で rOld の visited_at は非 NULL になっている。ここで nil を渡すと
		// review_repository.go の doc comment どおり「full replace」として NULL に戻ることを確かめる。
		updated, err := repo.UpdateReviewContent(ctx, rOld, 2, "Changed my mind", nil)
		if err != nil {
			t.Fatalf("UpdateReviewContent returned error: %v", err)
		}
		if updated.VisitedAt != nil {
			t.Errorf("updated VisitedAt = %v, want nil (cleared)", updated.VisitedAt)
		}
		stored := readReviewRow(ctx, t, conn, rOld)
		if stored.VisitedAt != nil {
			t.Errorf("stored VisitedAt = %v, want nil (cleared)", stored.VisitedAt)
		}
	})

	t.Run("UpdateReviewContent に discard 済みまたは存在しない review を渡すと ErrReviewNotFound になる", func(t *testing.T) {
		for name, id := range map[string]string{"discarded": rDiscarded, "unknown": uid.N(99999)} {
			if _, err := repo.UpdateReviewContent(ctx, id, 3, "x", nil); !errors.Is(err, domain.ErrReviewNotFound) {
				t.Errorf("%s: error = %v, want %v", name, err, domain.ErrReviewNotFound)
			}
		}
	})

	t.Run("DiscardReview は soft delete をちょうど 1 回だけ行い、hard delete はしない", func(t *testing.T) {
		victim := dbtest.InsertUUIDRow(ctx, t, conn, insertReview, 3, "bye", carol, cheese, nil, t2)
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
		if err := repo.DiscardReview(ctx, uid.N(99999)); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("unknown discard = %v, want %v", err, domain.ErrReviewNotFound)
		}
	})
}

// TestReviewRepositoryCreateShopBurger は、バーガー名を指定して投稿するときの、バーガーの解決
// (名前で探し、なければ作る)を確かめる。同じショップに同名のバーガーがあれば再利用すること、
// なければバーガーとショップとの結び付けを作ること、別のショップの同じ名前は別のバーガーに
// なることを確かめる。
//
// レビューの登録まで含めた 1 つのトランザクション(登録に失敗したら、作ったバーガーも巻き戻る)と、
// 統計の再計算は、トランザクションを持つ usecase を通して、adapter/uow のテストが確かめている。
func TestReviewRepositoryCreateShopBurger(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	repo := repository.NewReviewRepository(conn)

	insertUser := `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, 'x', $3) RETURNING id`
	alice := dbtest.InsertUserRow(ctx, t, conn, insertUser, "alice@example.com", "alice", false)

	insertShop := `INSERT INTO shops (name, status, moderation_note, creator_id) VALUES ($1, $2, $3, $4) RETURNING id`
	shopA := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Shop A", 1, nil, alice)
	shopB := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Shop B", 1, nil, nil)

	cheese := dbtest.InsertUUIDRow(ctx, t, conn, `INSERT INTO burgers (name) VALUES ($1) RETURNING id`, "Cheese")
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

	mustShopBurger := func(t *testing.T, shopID string, name string) domain.ShopReviewBurger {
		t.Helper()
		burger, err := repo.CreateShopBurger(ctx, shopID, name)
		if err != nil {
			t.Fatalf("CreateShopBurger returned error: %v", err)
		}
		return burger
	}

	t.Run("同じショップに同名のバーガーがあれば、新しく作らずにそれを返し、統計は呼び出し前の値のままである", func(t *testing.T) {
		burger := mustShopBurger(t, shopA, "Cheese")
		if burger.ID != cheese {
			t.Fatalf("burger id = %s, want the existing Cheese %s", burger.ID, cheese)
		}
		want := domain.ShopReviewBurger{ID: cheese, Name: "Cheese", AverageRating: 4.0, ReviewCount: 2, WeightedScore: 3.9, Confidence: 0.7}
		if !reflect.DeepEqual(burger, want) {
			t.Errorf("burger = %+v, want the seeded pre-call stats %+v", burger, want)
		}
		if got := burgersNamed(t, "Cheese"); got != 1 {
			t.Errorf("Cheese burger rows = %d, want no duplicate", got)
		}
	})

	t.Run("そのショップにない名前を指定すると、バーガーを新しく作って、ショップと結び付け(shops_burgers)、統計は 0 で返す", func(t *testing.T) {
		burger := mustShopBurger(t, shopA, "Veggie")
		if burger.Name != "Veggie" || burger.ID == cheese {
			t.Fatalf("burger = %+v, want a new Veggie row", burger)
		}
		if zero := (domain.ShopReviewBurger{ID: burger.ID, Name: "Veggie"}); !reflect.DeepEqual(burger, zero) {
			t.Errorf("burger = %+v, want zero pre-call stats", burger)
		}
		if got := countRows(t, `SELECT count(*) FROM shops_burgers WHERE shop_id = $1 AND burger_id = $2`, shopA, burger.ID); got != 1 {
			t.Errorf("link rows = %d, want 1", got)
		}
		// 同じ名前をもう一度解決すると、いま作った burger を再利用する。
		if again := mustShopBurger(t, shopA, "Veggie"); again.ID != burger.ID {
			t.Errorf("second resolve = %s, want the same Veggie %s", again.ID, burger.ID)
		}
	})

	t.Run("別のショップに同じ名前のバーガーがあっても、指定したショップ用に別のバーガーを作る", func(t *testing.T) {
		burger := mustShopBurger(t, shopB, "Cheese")
		if burger.ID == cheese {
			t.Fatalf("burger id = %s, want a new row distinct from shop A's Cheese %s", burger.ID, cheese)
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

	t.Run("ショップの中に同じ名前のバーガーが複数あるとき、作成が最も古いものが選ばれ、作成が同時刻なら id の小さいものが選ばれる", func(t *testing.T) {
		// id は UUID なので、id の大小は作成の順序を表さない。作成の新しい方を先に insert して、
		// 選び方が id の生成の順序に依存していないことを確かめる。
		shopC := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Shop C", 1, nil, nil)
		insertTwin := `INSERT INTO burgers (name, created_at) VALUES ($1, $2) RETURNING id`
		link := func(burgerID string) {
			t.Helper()
			if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shopC, burgerID); err != nil {
				t.Fatalf("link shop C burger %s: %v", burgerID, err)
			}
		}
		newer := dbtest.InsertUUIDRow(ctx, t, conn, insertTwin, "Twin", time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC))
		older := dbtest.InsertUUIDRow(ctx, t, conn, insertTwin, "Twin", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		link(newer)
		link(older)
		if burger := mustShopBurger(t, shopC, "Twin"); burger.ID != older {
			t.Errorf("burger id = %s, want the earliest created %s (newer %s)", burger.ID, older, newer)
		}

		sameTime := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
		tieA := dbtest.InsertUUIDRow(ctx, t, conn, insertTwin, "Tie", sameTime)
		tieB := dbtest.InsertUUIDRow(ctx, t, conn, insertTwin, "Tie", sameTime)
		link(tieA)
		link(tieB)
		smaller := tieA
		if tieB < tieA {
			smaller = tieB
		}
		if burger := mustShopBurger(t, shopC, "Tie"); burger.ID != smaller {
			t.Errorf("burger id = %s, want the smaller id %s of the same-time burgers", burger.ID, smaller)
		}
	})
}

// mustCreateReview は repository を通して review を構築し永続化する。
func mustCreateReview(ctx context.Context, t *testing.T, repo *repository.ReviewRepository, rating int, comment string, authorID string, burgerID string) domain.Review {
	t.Helper()
	review, err := domain.NewReview(rating, comment, authorID, burgerID, nil, time.Now())
	if err != nil {
		t.Fatalf("NewReview returned error: %v", err)
	}
	created, err := repo.CreateReview(ctx, review)
	if err != nil {
		t.Fatalf("CreateReview returned error: %v", err)
	}
	return created
}

// TestReviewRepositoryPhotoKey は、写真の key(photo_key)の永続化を検証する。
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
	burger := dbtest.InsertUUIDRow(ctx, t, conn,
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

	t.Run("UpdateReviewContentAndPhotoKey は content・実食日・key を一緒に書き込む", func(t *testing.T) {
		visitedAt := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
		updated, err := repo.UpdateReviewContentAndPhotoKey(ctx, created.ID, 5, "Even better", &visitedAt, strPtr("reviews/both.png"))
		if err != nil {
			t.Fatalf("UpdateReviewContentAndPhotoKey returned error: %v", err)
		}
		if updated.Rating != 5 || updated.Comment == nil || *updated.Comment != "Even better" {
			t.Errorf("updated = %+v, want rating 5 and the new comment", updated)
		}
		if updated.VisitedAt == nil || !updated.VisitedAt.Equal(visitedAt) {
			t.Errorf("updated VisitedAt = %v, want %v", updated.VisitedAt, visitedAt)
		}
		if updated.PhotoKey == nil || *updated.PhotoKey != "reviews/both.png" {
			t.Errorf("updated PhotoKey = %v, want reviews/both.png", updated.PhotoKey)
		}
		// commit された行は content・実食日・key の「すべて」を持つ（1 つの
		// トランザクションなので、key や visited_at を伴わない content だけになることは
		// 決してない）。
		stored := readReviewRow(ctx, t, conn, created.ID)
		if stored.Rating != 5 || stored.Comment == nil || *stored.Comment != "Even better" ||
			stored.PhotoKey == nil || *stored.PhotoKey != "reviews/both.png" {
			t.Errorf("stored = rating %d, comment %v, key %v, want 5, Even better and reviews/both.png",
				stored.Rating, stored.Comment, stored.PhotoKey)
		}
		if stored.VisitedAt == nil || !stored.VisitedAt.Equal(visitedAt) {
			t.Errorf("stored VisitedAt = %v, want %v", stored.VisitedAt, visitedAt)
		}
	})

	t.Run("key と実食日なしの create は両方 NULL のままになる", func(t *testing.T) {
		// domain.Review{} のゼロ値どおり VisitedAt を指定しない create。
		plain, err := repo.CreateReview(ctx, domain.Review{Rating: 3, Comment: &comment, AuthorID: alice, BurgerID: burger})
		if err != nil {
			t.Fatalf("CreateReview returned error: %v", err)
		}
		if plain.PhotoKey != nil {
			t.Errorf("PhotoKey = %v, want nil", plain.PhotoKey)
		}
		if plain.VisitedAt != nil {
			t.Errorf("VisitedAt = %v, want nil", plain.VisitedAt)
		}
		// SQL で直接読み取った行も、visited_at が NULL のままである。
		stored := readReviewRow(ctx, t, conn, plain.ID)
		if stored.PhotoKey != nil || stored.VisitedAt != nil {
			t.Errorf("stored = %+v, want photo_key and visited_at both NULL", stored)
		}
	})

	t.Run("discard 済みの review への UpdateReviewContentAndPhotoKey は ErrReviewNotFound になり、変更は残らない", func(t *testing.T) {
		if err := repo.DiscardReview(ctx, created.ID); err != nil {
			t.Fatalf("DiscardReview returned error: %v", err)
		}
		// 結合した書き込みは rollback される：content の変更は何も残らない。
		if _, err := repo.UpdateReviewContentAndPhotoKey(ctx, created.ID, 1, "ghost", nil, strPtr("reviews/ghost.jpg")); !errors.Is(err, domain.ErrReviewNotFound) {
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
