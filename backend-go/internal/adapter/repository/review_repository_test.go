package repository_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// reviewIDs は review の id を順序を保ったまま取り出す。
func reviewIDs(reviews []domain.ReviewDetail) []int64 {
	ids := make([]int64, 0, len(reviews))
	for _, r := range reviews {
		ids = append(ids, r.ID)
	}
	return ids
}

// TestReviewRepository は、S6 の review の永続化を、共有の dbtest の
// スキャフォールドを通じて実際の PostgreSQL に対して検証する
// （TEST_DATABASE_URL がなければスキップする）。対象は、active な shop に
// 対する EXISTS のフィード絞り込み（重複した link の dedup を含む）、
// soft delete の除外、結合された詳細、カラム単位の書き込みである。
func TestReviewRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	repo := repository.NewReviewRepository(conn)
	reviewQuery := query.NewReviewQuery(conn)

	insertUser := `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, 'x', $3) RETURNING id`
	alice := insertUserRow(ctx, t, conn, insertUser, "alice@example.com", "alice", false)
	carol := insertUserRow(ctx, t, conn, insertUser, "carol@example.com", "carol", false)

	insertShop := `INSERT INTO shops (name, status, moderation_note, creator_id) VALUES ($1, $2, $3, $4) RETURNING id`
	// status のコード：0=pending、1=active、2=rejected。
	active1 := insertRow(ctx, t, conn, insertShop, "Active One", 1, nil, alice)
	active2 := insertRow(ctx, t, conn, insertShop, "Active Two", 1, nil, nil)
	pending := insertRow(ctx, t, conn, insertShop, "Pending Shack", 0, nil, alice)
	rejected := insertRow(ctx, t, conn, insertShop, "Rejected Grill", 2, nil, nil)

	insertBurger := `INSERT INTO burgers (name) VALUES ($1) RETURNING id`
	cheese := insertRow(ctx, t, conn, insertBurger, "Cheese")   // active な shop の「両方」に link されている
	plain := insertRow(ctx, t, conn, insertBurger, "Plain")     // active1 だけに link されている。stats なし
	hidden := insertRow(ctx, t, conn, insertBurger, "Hidden")   // pending だけに link されている
	outcast := insertRow(ctx, t, conn, insertBurger, "Outcast") // rejected だけに link されている
	mustLink := func(shopID, burgerID int64) {
		t.Helper()
		if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shopID, burgerID); err != nil {
			t.Fatalf("link shop %d burger %d: %v", shopID, burgerID, err)
		}
	}
	mustLink(active1, cheese)
	mustLink(active2, cheese)
	mustLink(active1, plain)
	mustLink(pending, hidden)
	mustLink(rejected, outcast)

	// 読み取り系のサブテストは、cheese についてこのリテラルの stats を
	// アサートする。S7 の再計算は review の書き込みのたびにこの行を上書きする
	// ので、以下の書き込み系の各サブテストは、自身の後始末で、この upsert に
	// よってこの行を再度 seed する。
	seedCheeseStats := func(t *testing.T) {
		t.Helper()
		if _, err := conn.Exec(ctx,
			`INSERT INTO burger_stats (burger_id, review_count, average_rating, weighted_score, confidence, calculated_at)
			 VALUES ($1, 2, 4.0, 3.9, 0.7, now())
			 ON CONFLICT (burger_id) DO UPDATE SET
			   review_count = EXCLUDED.review_count,
			   average_rating = EXCLUDED.average_rating,
			   weighted_score = EXCLUDED.weighted_score,
			   confidence = EXCLUDED.confidence,
			   calculated_at = EXCLUDED.calculated_at`, cheese); err != nil {
			t.Fatalf("seed burger stats: %v", err)
		}
	}
	seedCheeseStats(t)

	insertReview := `INSERT INTO reviews (rating, comment, user_id, burger_id, discarded_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
	t1 := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 5, 2, 10, 0, 0, 0, time.UTC)
	rOld := insertRow(ctx, t, conn, insertReview, 5, "Tasty", alice, cheese, nil, t1)
	rTie1 := insertRow(ctx, t, conn, insertReview, 3, nil, carol, cheese, nil, t2)
	rTie2 := insertRow(ctx, t, conn, insertReview, 4, nil, alice, plain, nil, t2) // 同一時刻：id desc で同順位を解消する
	rDiscarded := insertRow(ctx, t, conn, insertReview, 1, "gone", alice, cheese, time.Now(), t2)
	insertRow(ctx, t, conn, insertReview, 2, "pending only", alice, hidden, nil, t2)
	insertRow(ctx, t, conn, insertReview, 2, "rejected only", alice, outcast, nil, t2)

	t.Run("ListReviews は active な shop の burger に絞り込み、重複なしで新しい順に返す", func(t *testing.T) {
		reviews, _, err := reviewQuery.ListReviews(ctx, usecase.ReviewListFilter{}, 100, 0)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		// cheese の review は、cheese が active な 2 つの shop に link されて
		// いても、ちょうど 1 回だけ現れなければならない（行を増殖させる JOIN
		// ではなく EXISTS）。pending だけ・rejected だけの burger の review と、
		// discard 済みの review は現れない（SQL レベルでの AC5/AC6）。
		if got, want := reviewIDs(reviews), []int64{rTie2, rTie1, rOld}; !reflect.DeepEqual(got, want) {
			t.Fatalf("ids = %v, want %v", got, want)
		}

		comment := "Tasty"
		wantOld := domain.ReviewDetail{
			Review: domain.Review{ID: rOld, Rating: 5, Comment: &comment, AuthorID: alice, BurgerID: cheese, CreatedAt: t1},
			User:   &domain.UserRef{ID: alice, Username: "alice"},
			Burger: &domain.ShopReviewBurger{ID: cheese, Name: "Cheese", AverageRating: 4.0, ReviewCount: 2, WeightedScore: 3.9, Confidence: 0.7},
		}
		got := reviews[2]
		if !got.CreatedAt.Equal(wantOld.CreatedAt) {
			t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, wantOld.CreatedAt)
		}
		got.CreatedAt = wantOld.CreatedAt
		if !reflect.DeepEqual(got, wantOld) {
			t.Errorf("review = %+v, want %+v", got, wantOld)
		}
		// stats のない burger はゼロになる。
		if want := (&domain.ShopReviewBurger{ID: plain, Name: "Plain"}); !reflect.DeepEqual(reviews[0].Burger, want) {
			t.Errorf("stats-less burger = %+v, want %+v", reviews[0].Burger, want)
		}
	})

	t.Run("ListReviews は順序付きフィードを pagination する", func(t *testing.T) {
		page1, more1, err := reviewQuery.ListReviews(ctx, usecase.ReviewListFilter{}, 2, 0)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if got, want := reviewIDs(page1), []int64{rTie2, rTie1}; !reflect.DeepEqual(got, want) {
			t.Errorf("page 1 = %v, want %v", got, want)
		}
		page2, more2, err := reviewQuery.ListReviews(ctx, usecase.ReviewListFilter{}, 2, 2)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if got, want := reviewIDs(page2), []int64{rOld}; !reflect.DeepEqual(got, want) {
			t.Errorf("page 2 = %v, want %v", got, want)
		}
		far, moreFar, err := reviewQuery.ListReviews(ctx, usecase.ReviewListFilter{}, 2, 100)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if len(far) != 0 {
			t.Errorf("far page = %v, want empty", far)
		}
		// has_more: 3 件を 2 件ずつ読むと、1 ページ目だけ続きがある。範囲外は空で false。
		if !more1 || more2 || moreFar {
			t.Errorf("hasMore = (page1 %v, page2 %v, far %v), want (true, false, false)", more1, more2, moreFar)
		}
	})

	t.Run("ListReviews の has_more は、件数ちょうどの limit では false になる（空のページを取りに行かせない）", func(t *testing.T) {
		// フィードは 3 件。limit=3 は件数ちょうど、limit=100 は件数より大きい。
		for _, limit := range []int32{3, 100} {
			all, hasMore, err := reviewQuery.ListReviews(ctx, usecase.ReviewListFilter{}, limit, 0)
			if err != nil {
				t.Fatalf("ListReviews(limit %d) returned error: %v", limit, err)
			}
			if len(all) != 3 || hasMore {
				t.Errorf("limit %d: len = %d, hasMore = %v, want 3, false", limit, len(all), hasMore)
			}
		}
	})

	t.Run("ListReviews はフィードのルールに加えて Rails ReviewQuery の filter を適用する", func(t *testing.T) {
		intp := func(n int) *int { return &n }
		int64p := func(n int64) *int64 { return &n }
		tests := []struct {
			name   string
			filter usecase.ReviewListFilter
			want   []int64
		}{
			{name: "rating の完全一致 (by_rating)", filter: usecase.ReviewListFilter{Rating: intp(4)}, want: []int64{rTie2}},
			// rating 2 の review は pending だけ・rejected だけの burger にしか
			// 存在しない：active な shop のフィードのルールが引き続き適用される。
			{name: "rating が非表示の review にしか一致しない場合は空になる", filter: usecase.ReviewListFilter{Rating: intp(2)}, want: []int64{}},
			{name: "keyword は大文字小文字を区別しない (keyword_search ILIKE)", filter: usecase.ReviewListFilter{Keyword: "tAsT"}, want: []int64{rOld}},
			// NULL の comment は決して一致しない。Rails の comment ILIKE と
			// 同様である。
			{name: "keyword は NULL の comment を対象にしない", filter: usecase.ReviewListFilter{Keyword: "a"}, want: []int64{rOld}},
			// エスケープしなければ、"%" は NULL でないすべての comment に
			// ILIKE で一致してしまう。
			{name: "keyword の LIKE メタ文字はリテラルとして一致する", filter: usecase.ReviewListFilter{Keyword: "%"}, want: []int64{}},
			{name: "keyword が非表示の review にしか一致しない場合は空になる", filter: usecase.ReviewListFilter{Keyword: "only"}, want: []int64{}},
			{name: "shop_id は shops_burgers の link をたどる", filter: usecase.ReviewListFilter{ShopID: int64p(active2)}, want: []int64{rTie1, rOld}},
			{name: "shop_id で絞り込んでも、その shop の burger の review はすべて残る", filter: usecase.ReviewListFilter{ShopID: int64p(active1)}, want: []int64{rTie2, rTie1, rOld}},
			{name: "存在しない shop_id は空になる", filter: usecase.ReviewListFilter{ShopID: int64p(99999)}, want: []int64{}},
			{name: "filter は AND で組み合わされる", filter: usecase.ReviewListFilter{Rating: intp(5), Keyword: "tast", ShopID: int64p(active2)}, want: []int64{rOld}},
			{name: "AND の組み合わせが一致しない場合は空になる", filter: usecase.ReviewListFilter{Rating: intp(3), Keyword: "tast"}, want: []int64{}},
			// 範囲外の rating は比較結果が false にならなければならず、
			// smallint カラムをオーバーフローさせて SQL エラーに
			// なってはならない。
			{name: "smallint を超える rating は空になり、エラーにならない", filter: usecase.ReviewListFilter{Rating: intp(1 << 40)}, want: []int64{}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				reviews, _, err := reviewQuery.ListReviews(ctx, tt.filter, 100, 0)
				if err != nil {
					t.Fatalf("ListReviews returned error: %v", err)
				}
				if got := reviewIDs(reviews); !reflect.DeepEqual(got, tt.want) {
					t.Errorf("ids = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("shop_id の filter は、指定された shop 自体が active であることを要求する", func(t *testing.T) {
		// active と pending の「両方」の shop に link された burger：その
		// review は（active な link 経由で）フィードに含まれるが、pending な
		// shop で絞り込むと何も返してはならない。これは Rails の、status を
		// 見ない shop の filter よりも厳格であり、フィードの active な shop の
		// ルールと整合する。
		mixedActive := insertRow(ctx, t, conn, insertShop, "Mixed Active", 1, nil, nil)
		mixed := insertRow(ctx, t, conn, insertBurger, "Mixed")
		mustLink(mixedActive, mixed)
		mustLink(pending, mixed)
		rMixed := insertRow(ctx, t, conn, insertReview, 4, "mixed", alice, mixed, nil, t2)
		t.Cleanup(func() {
			for _, del := range []struct {
				sql string
				id  int64
			}{
				{`DELETE FROM reviews WHERE id = $1`, rMixed},
				{`DELETE FROM shops_burgers WHERE burger_id = $1`, mixed},
				{`DELETE FROM burgers WHERE id = $1`, mixed},
				{`DELETE FROM shops WHERE id = $1`, mixedActive},
			} {
				if _, err := conn.Exec(ctx, del.sql, del.id); err != nil {
					t.Fatalf("cleanup %q: %v", del.sql, err)
				}
			}
		})

		byShop := func(id int64) usecase.ReviewListFilter { return usecase.ReviewListFilter{ShopID: &id} }
		got, _, err := reviewQuery.ListReviews(ctx, byShop(pending), 100, 0)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("pending shop filter = %v, want empty", reviewIDs(got))
		}
		got, _, err = reviewQuery.ListReviews(ctx, byShop(mixedActive), 100, 0)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if want := []int64{rMixed}; !reflect.DeepEqual(reviewIDs(got), want) {
			t.Errorf("active shop filter = %v, want %v", reviewIDs(got), want)
		}
	})

	t.Run("GetReview は author、burger、stats を join する", func(t *testing.T) {
		got, err := reviewQuery.GetReview(ctx, rTie1)
		if err != nil {
			t.Fatalf("GetReview returned error: %v", err)
		}
		want := domain.ReviewDetail{
			Review: domain.Review{ID: rTie1, Rating: 3, AuthorID: carol, BurgerID: cheese, CreatedAt: t2},
			User:   &domain.UserRef{ID: carol, Username: "carol"},
			Burger: &domain.ShopReviewBurger{ID: cheese, Name: "Cheese", AverageRating: 4.0, ReviewCount: 2, WeightedScore: 3.9, Confidence: 0.7},
		}
		if !got.CreatedAt.Equal(want.CreatedAt) {
			t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, want.CreatedAt)
		}
		got.CreatedAt = want.CreatedAt
		if !reflect.DeepEqual(got, want) {
			t.Errorf("detail = %+v, want %+v", got, want)
		}
	})

	t.Run("AC6 discard 済みの review と存在しない review は ErrReviewNotFound になる", func(t *testing.T) {
		for name, id := range map[string]int64{"discarded": rDiscarded, "unknown": 99999} {
			if _, err := reviewQuery.GetReview(ctx, id); !errors.Is(err, domain.ErrReviewNotFound) {
				t.Errorf("%s: error = %v, want %v", name, err, domain.ErrReviewNotFound)
			}
		}
	})

	t.Run("GetShop は shop 単体を返すか ErrShopNotFound を返す", func(t *testing.T) {
		shop, err := reviewQuery.GetShop(ctx, pending)
		if err != nil {
			t.Fatalf("GetShop returned error: %v", err)
		}
		if shop.ID != pending || shop.Status != domain.ShopStatusPending || shop.CreatorID == nil || *shop.CreatorID != alice {
			t.Errorf("shop = %+v, want pending shop created by alice", shop)
		}
		if _, err := reviewQuery.GetShop(ctx, 99999); !errors.Is(err, domain.ErrShopNotFound) {
			t.Fatalf("error = %v, want %v", err, domain.ErrShopNotFound)
		}
	})

	t.Run("GetShopBurger は shops_burgers の link を要求する", func(t *testing.T) {
		burger, err := reviewQuery.GetShopBurger(ctx, active1, cheese)
		if err != nil {
			t.Fatalf("GetShopBurger returned error: %v", err)
		}
		want := domain.ShopReviewBurger{ID: cheese, Name: "Cheese", AverageRating: 4.0, ReviewCount: 2, WeightedScore: 3.9, Confidence: 0.7}
		if !reflect.DeepEqual(burger, want) {
			t.Errorf("burger = %+v, want %+v", burger, want)
		}
		// stats 行がない場合：ゼロ。
		statless, err := reviewQuery.GetShopBurger(ctx, active1, plain)
		if err != nil {
			t.Fatalf("GetShopBurger returned error: %v", err)
		}
		if want := (domain.ShopReviewBurger{ID: plain, Name: "Plain"}); !reflect.DeepEqual(statless, want) {
			t.Errorf("stats-less burger = %+v, want %+v", statless, want)
		}
		// 別の shop に属する既存の burger と未知の burger は、区別できない。
		for name, burgerID := range map[string]int64{"unlinked": hidden, "unknown": 99999} {
			if _, err := reviewQuery.GetShopBurger(ctx, active1, burgerID); !errors.Is(err, domain.ErrBurgerNotFound) {
				t.Errorf("%s: error = %v, want %v", name, err, domain.ErrBurgerNotFound)
			}
		}
	})

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
		if _, err := reviewQuery.GetReview(ctx, created.ID); err != nil {
			t.Errorf("GetReview after create returned error: %v", err)
		}
		// 再度これを削除し、cheese の burger_stats を再 seed する（create が
		// 再計算した）。これにより、兄弟サブテストのフィードと stats の
		// アサーションが、実行順序にかかわらず厳密なままになる。
		if _, err := conn.Exec(ctx, `DELETE FROM reviews WHERE id = $1`, created.ID); err != nil {
			t.Fatalf("delete created review: %v", err)
		}
		seedCheeseStats(t)
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
		// discarded_at には触れていない：review は依然として kept であり、GetReview で取得できる。
		if _, err := reviewQuery.GetReview(ctx, rOld); err != nil {
			t.Errorf("GetReview after update returned error: %v", err)
		}
		// review を元に戻し、cheese の burger_stats を再 seed する（2 回の
		// update がどちらも再計算した）。兄弟サブテストのためである。
		if _, err := repo.UpdateReviewContent(ctx, rOld, 5, "Tasty"); err != nil {
			t.Fatalf("restore review: %v", err)
		}
		seedCheeseStats(t)
	})

	t.Run("UpdateReviewContent に discard 済みまたは存在しない review を渡すと ErrReviewNotFound になる", func(t *testing.T) {
		for name, id := range map[string]int64{"discarded": rDiscarded, "unknown": 99999} {
			if _, err := repo.UpdateReviewContent(ctx, id, 3, "x"); !errors.Is(err, domain.ErrReviewNotFound) {
				t.Errorf("%s: error = %v, want %v", name, err, domain.ErrReviewNotFound)
			}
		}
	})

	t.Run("DiscardReview は soft delete をちょうど 1 回だけ行い、hard delete はしない", func(t *testing.T) {
		victim := insertRow(ctx, t, conn, insertReview, 3, "bye", carol, cheese, nil, t2)
		if err := repo.DiscardReview(ctx, victim); err != nil {
			t.Fatalf("DiscardReview returned error: %v", err)
		}
		if _, err := reviewQuery.GetReview(ctx, victim); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("GetReview after discard = %v, want %v", err, domain.ErrReviewNotFound)
		}
		reviews, _, err := reviewQuery.ListReviews(ctx, usecase.ReviewListFilter{}, 100, 0)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		for _, r := range reviews {
			if r.ID == victim {
				t.Errorf("feed still contains discarded review %d", victim)
			}
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
		// 兄弟サブテストのための後始末：victim を削除し、cheese の
		// burger_stats を再 seed する（discard が再計算した）。
		if _, err := conn.Exec(ctx, `DELETE FROM reviews WHERE id = $1`, victim); err != nil {
			t.Fatalf("delete victim review: %v", err)
		}
		seedCheeseStats(t)
	})
}

// TestReviewRepositoryListByUser は、ListReviews の UserID フィルタを実際の
// PostgreSQL に対して検証する：その user の公開 review だけが新しい順に
// pagination されること、rating との AND、discard 済み・存在しない user が
// 空になること、そして「新しいフィルタが公開ルール（discard 済みの review、
// active な shop に紐づかない burger）を迂回しない」ことである。
func TestReviewRepositoryListByUser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	reviewQuery := query.NewReviewQuery(conn)

	insertUser := `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, 'x', $3) RETURNING id`
	alice := insertUserRow(ctx, t, conn, insertUser, "alice@example.com", "alice", false)
	bob := insertUserRow(ctx, t, conn, insertUser, "bob@example.com", "bob", false)
	carol := insertUserRow(ctx, t, conn, insertUser, "carol@example.com", "carol", false)
	dave := insertUserRow(ctx, t, conn, insertUser, "dave@example.com", "dave", false)
	erin := insertUserRow(ctx, t, conn, insertUser, "erin@example.com", "erin", false)

	insertShop := `INSERT INTO shops (name, status, moderation_note, creator_id) VALUES ($1, $2, $3, $4) RETURNING id`
	// status のコード：0=pending、1=active、2=rejected。
	activeShop := insertRow(ctx, t, conn, insertShop, "Active Shop", 1, nil, nil)
	pendingShop := insertRow(ctx, t, conn, insertShop, "Pending Shop", 0, nil, dave)

	insertBurger := `INSERT INTO burgers (name) VALUES ($1) RETURNING id`
	cheese := insertRow(ctx, t, conn, insertBurger, "Cheese") // active な shop に link されている
	secret := insertRow(ctx, t, conn, insertBurger, "Secret") // pending な shop だけに link されている
	for _, link := range [][2]int64{{activeShop, cheese}, {pendingShop, secret}} {
		if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, link[0], link[1]); err != nil {
			t.Fatalf("link shop %d burger %d: %v", link[0], link[1], err)
		}
	}

	insertReview := `INSERT INTO reviews (rating, comment, user_id, burger_id, discarded_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
	base := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)

	// alice の公開 review 25 件と、bob の公開 review 30 件。時刻は重なっており、
	// 一部は同一時刻（id desc で同順位を解消する）なので、フィルタがなければ
	// 2 人の review は混ざる。
	var aliceIDs, aliceRating5IDs []int64
	for i := 0; i < 25; i++ {
		rating := 1 + i%5
		id := insertRow(ctx, t, conn, insertReview, rating, fmt.Sprintf("alice %d", i), alice, cheese, nil, base.Add(time.Duration(i/2)*time.Minute))
		aliceIDs = append(aliceIDs, id)
		if rating == 5 {
			aliceRating5IDs = append(aliceRating5IDs, id)
		}
	}
	for i := 0; i < 30; i++ {
		insertRow(ctx, t, conn, insertReview, 1+i%5, fmt.Sprintf("bob %d", i), bob, cheese, nil, base.Add(time.Duration(i/3)*time.Minute))
	}
	// created_at は挿入順に単調非減少で id は単調増加なので、新しい順
	// （created_at desc、id desc）は挿入順の逆になる。
	slices.Reverse(aliceIDs)
	slices.Reverse(aliceRating5IDs)

	list := func(t *testing.T, filter usecase.ReviewListFilter, limit, offset int32) []int64 {
		t.Helper()
		reviews, _, err := reviewQuery.ListReviews(ctx, filter, limit, offset)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		return reviewIDs(reviews)
	}
	byUser := func(id string) usecase.ReviewListFilter { return usecase.ReviewListFilter{UserID: &id} }

	t.Run("AC1 UserID はその user の公開 review だけを新しい順に pagination して返す", func(t *testing.T) {
		if got, want := list(t, byUser(alice), 20, 0), aliceIDs[:20]; !reflect.DeepEqual(got, want) {
			t.Errorf("page 1 = %v, want %v", got, want)
		}
		if got, want := list(t, byUser(alice), 20, 20), aliceIDs[20:]; !reflect.DeepEqual(got, want) {
			t.Errorf("page 2 = %v, want %v", got, want)
		}
		if got := list(t, byUser(alice), 20, 40); len(got) != 0 {
			t.Errorf("page 3 = %v, want empty", got)
		}
		// 1 ページに収まる場合も、alice の 25 件だけがちょうど返る（bob の 30 件は
		// 混ざらない）。
		if got := list(t, byUser(alice), 100, 0); !reflect.DeepEqual(got, aliceIDs) {
			t.Errorf("all = %v, want %v", got, aliceIDs)
		}
		if got := list(t, usecase.ReviewListFilter{}, 100, 0); len(got) != 55 {
			t.Errorf("unfiltered feed has %d reviews, want 55 (alice 25 + bob 30)", len(got))
		}
	})

	t.Run("AC2 UserID は rating と AND で組み合わされる", func(t *testing.T) {
		// alice の rating 5 だけが残る。alice の他の rating と、bob の rating 5 は
		// 返らない。
		rating := 5
		filter := byUser(alice)
		filter.Rating = &rating
		if got := list(t, filter, 100, 0); !reflect.DeepEqual(got, aliceRating5IDs) {
			t.Errorf("alice rating 5 = %v, want %v", got, aliceRating5IDs)
		}
	})

	t.Run("AC4 discard 済みの user と存在しない user の id は空になる", func(t *testing.T) {
		carolReview := insertRow(ctx, t, conn, insertReview, 4, "carol was here", carol, cheese, nil, base)
		// discard する前は、carol の review は見える（このテストが空を検証する
		// 意味を持つための前提）。
		if got, want := list(t, byUser(carol), 100, 0), []int64{carolReview}; !reflect.DeepEqual(got, want) {
			t.Fatalf("active carol = %v, want %v", got, want)
		}
		if _, err := conn.Exec(ctx, `UPDATE users SET discarded_at = now() WHERE id = $1`, carol); err != nil {
			t.Fatalf("discard carol: %v", err)
		}
		if got := list(t, byUser(carol), 100, 0); !reflect.DeepEqual(got, []int64{}) {
			t.Errorf("discarded carol = %v, want empty", got)
		}
		if got := list(t, byUser(uid.N(999999)), 100, 0); !reflect.DeepEqual(got, []int64{}) {
			t.Errorf("unknown user = %v, want empty", got)
		}
	})

	t.Run("AC5 pending な shop の burger だけの review しかない user は空になり、公開ルールを迂回しない", func(t *testing.T) {
		daveA := insertRow(ctx, t, conn, insertReview, 5, "dave secret 1", dave, secret, nil, base)
		daveB := insertRow(ctx, t, conn, insertReview, 4, "dave secret 2", dave, secret, nil, base.Add(time.Minute))
		// UserID なしのフィードにも現れない（公開ルールの前提）。
		for _, id := range list(t, usecase.ReviewListFilter{}, 100, 0) {
			if id == daveA || id == daveB {
				t.Fatalf("review %d on a pending-only burger leaked into the public feed", id)
			}
		}
		if got := list(t, byUser(dave), 100, 0); !reflect.DeepEqual(got, []int64{}) {
			t.Errorf("dave (pending only) = %v, want empty", got)
		}
	})

	t.Run("AC5 UserID を指定しても discard 済みの review と非公開の burger の review は除外される", func(t *testing.T) {
		kept := insertRow(ctx, t, conn, insertReview, 4, "erin kept", erin, cheese, nil, base)
		insertRow(ctx, t, conn, insertReview, 1, "erin discarded", erin, cheese, time.Now(), base.Add(time.Minute))
		insertRow(ctx, t, conn, insertReview, 5, "erin on pending", erin, secret, nil, base.Add(2*time.Minute))
		if got, want := list(t, byUser(erin), 100, 0), []int64{kept}; !reflect.DeepEqual(got, want) {
			t.Errorf("erin = %v, want only the kept review on the active shop %v", got, want)
		}
	})
}

// TestReviewRepositoryCreateShopBurger は、burger_name による find-or-create の投稿経路
// （S6 P3-1）の burger の解決を検証する。名前の完全一致による shop 単位での再利用、未知の
// 名前に対する burger と link の作成、shop ごとの名前のスコープ（別の shop の同じ名前は別の
// burger 行になる）である。review の insert までを 1 つのトランザクションにする保証
// （insert に失敗しても、孤立した burger や link が commit されない）と、統計の再計算は、
// トランザクションを持つ usecase の UnitOfWork のテスト（adapter/uow）が扱う。
func TestReviewRepositoryCreateShopBurger(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	repo := repository.NewReviewRepository(conn)

	insertUser := `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, 'x', $3) RETURNING id`
	alice := insertUserRow(ctx, t, conn, insertUser, "alice@example.com", "alice", false)

	insertShop := `INSERT INTO shops (name, status, moderation_note, creator_id) VALUES ($1, $2, $3, $4) RETURNING id`
	shopA := insertRow(ctx, t, conn, insertShop, "Shop A", 1, nil, alice)
	shopB := insertRow(ctx, t, conn, insertShop, "Shop B", 1, nil, nil)

	cheese := insertRow(ctx, t, conn, `INSERT INTO burgers (name) VALUES ($1) RETURNING id`, "Cheese")
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

	mustShopBurger := func(t *testing.T, shopID int64, name string) domain.ShopReviewBurger {
		t.Helper()
		burger, err := repo.CreateShopBurger(ctx, shopID, name)
		if err != nil {
			t.Fatalf("CreateShopBurger returned error: %v", err)
		}
		return burger
	}

	t.Run("shop 内に同名の burger があれば再利用し、戻り値の burger は呼び出し前の stats を持つ", func(t *testing.T) {
		burger := mustShopBurger(t, shopA, "Cheese")
		if burger.ID != cheese {
			t.Fatalf("burger id = %d, want the existing Cheese %d", burger.ID, cheese)
		}
		want := domain.ShopReviewBurger{ID: cheese, Name: "Cheese", AverageRating: 4.0, ReviewCount: 2, WeightedScore: 3.9, Confidence: 0.7}
		if !reflect.DeepEqual(burger, want) {
			t.Errorf("burger = %+v, want the seeded pre-call stats %+v", burger, want)
		}
		if got := burgersNamed(t, "Cheese"); got != 1 {
			t.Errorf("Cheese burger rows = %d, want no duplicate", got)
		}
	})

	t.Run("未知の名前は burger とその shops_burgers の link を作成する", func(t *testing.T) {
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
			t.Errorf("second resolve = %d, want the same Veggie %d", again.ID, burger.ID)
		}
	})

	t.Run("別の shop の同じ名前は別の burger 行になる", func(t *testing.T) {
		burger := mustShopBurger(t, shopB, "Cheese")
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

// TestReviewRepositoryPhotoKey は、S10 の photo_key の永続化を検証する。
// CreateReview が key を保存し、結合された読み取りクエリがそれを返し、
// UpdateReviewContentAndPhotoKey が、まだ kept な review の content と key を
// 一緒に入れ替える。
func TestReviewRepositoryPhotoKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	repo := repository.NewReviewRepository(conn)
	reviewQuery := query.NewReviewQuery(conn)

	alice := insertUserRow(ctx, t, conn,
		`INSERT INTO users (email, username, password_digest) VALUES ($1, $2, 'x') RETURNING id`,
		"alice@example.com", "alice")
	shop := insertRow(ctx, t, conn,
		`INSERT INTO shops (name, status) VALUES ($1, 1) RETURNING id`, "Active One")
	burger := insertRow(ctx, t, conn,
		`INSERT INTO burgers (name) VALUES ($1) RETURNING id`, "Cheese")
	if _, err := conn.Exec(ctx, `INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2)`, shop, burger); err != nil {
		t.Fatalf("link shop and burger: %v", err)
	}

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

	t.Run("読み取りクエリは photo_key を返す", func(t *testing.T) {
		detail, err := reviewQuery.GetReview(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetReview returned error: %v", err)
		}
		if detail.PhotoKey == nil || *detail.PhotoKey != "reviews/abc.jpg" {
			t.Errorf("detail PhotoKey = %v, want reviews/abc.jpg", detail.PhotoKey)
		}
		list, _, err := reviewQuery.ListReviews(ctx, usecase.ReviewListFilter{}, 10, 0)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if len(list) != 1 || list[0].PhotoKey == nil || *list[0].PhotoKey != "reviews/abc.jpg" {
			t.Errorf("list = %+v, want one review with PhotoKey reviews/abc.jpg", list)
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
		detail, err := reviewQuery.GetReview(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetReview after combined update returned error: %v", err)
		}
		if detail.Rating != 5 || detail.PhotoKey == nil || *detail.PhotoKey != "reviews/both.png" {
			t.Errorf("stored = rating %d, key %v, want 5 and reviews/both.png", detail.Rating, detail.PhotoKey)
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
