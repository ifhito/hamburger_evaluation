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
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// reviewIDs extracts the review ids preserving order.
func reviewIDs(reviews []domain.ReviewDetail) []int64 {
	ids := make([]int64, 0, len(reviews))
	for _, r := range reviews {
		ids = append(ids, r.ID)
	}
	return ids
}

// TestReviewRepository exercises the S6 review persistence against a real
// PostgreSQL via the shared dbtest scaffold (skips without
// TEST_DATABASE_URL): the EXISTS active-shop feed filter (with the
// duplicate-link dedup), the soft-delete exclusion, the joined detail,
// and the column-scoped writes.
func TestReviewRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	repo := repository.NewReviewRepository(conn)

	insertUser := `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, 'x', $3) RETURNING id`
	alice := insertRow(ctx, t, conn, insertUser, "alice@example.com", "alice", false)
	carol := insertRow(ctx, t, conn, insertUser, "carol@example.com", "carol", false)

	insertShop := `INSERT INTO shops (name, status, moderation_note, creator_id) VALUES ($1, $2, $3, $4) RETURNING id`
	// status codes: 0=pending, 1=active, 2=rejected.
	active1 := insertRow(ctx, t, conn, insertShop, "Active One", 1, nil, alice)
	active2 := insertRow(ctx, t, conn, insertShop, "Active Two", 1, nil, nil)
	pending := insertRow(ctx, t, conn, insertShop, "Pending Shack", 0, nil, alice)
	rejected := insertRow(ctx, t, conn, insertShop, "Rejected Grill", 2, nil, nil)

	insertBurger := `INSERT INTO burgers (name) VALUES ($1) RETURNING id`
	cheese := insertRow(ctx, t, conn, insertBurger, "Cheese")   // linked to BOTH active shops
	plain := insertRow(ctx, t, conn, insertBurger, "Plain")     // linked to active1 only, no stats
	hidden := insertRow(ctx, t, conn, insertBurger, "Hidden")   // linked to pending only
	outcast := insertRow(ctx, t, conn, insertBurger, "Outcast") // linked to rejected only
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

	// The read subtests assert these literal stats for cheese. The S7
	// recalculation overwrites this row on every review write, so each
	// write subtest below re-seeds it in its cleanup via this upsert.
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
	rTie2 := insertRow(ctx, t, conn, insertReview, 4, nil, alice, plain, nil, t2) // same instant: id desc breaks the tie
	rDiscarded := insertRow(ctx, t, conn, insertReview, 1, "gone", alice, cheese, time.Now(), t2)
	insertRow(ctx, t, conn, insertReview, 2, "pending only", alice, hidden, nil, t2)
	insertRow(ctx, t, conn, insertReview, 2, "rejected only", alice, outcast, nil, t2)

	t.Run("ListReviews filters to active-shop burgers, no dupes, newest first", func(t *testing.T) {
		reviews, err := repo.ListReviews(ctx, usecase.ReviewListFilter{}, 100, 0)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		// The cheese reviews must appear exactly once despite cheese being
		// linked to two active shops (EXISTS, not a multiplying JOIN); the
		// pending-only and rejected-only burgers' reviews and the discarded
		// review are absent (AC5/AC6 at the SQL level).
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
		// The stats-less burger maps to zeros.
		if want := (&domain.ShopReviewBurger{ID: plain, Name: "Plain"}); !reflect.DeepEqual(reviews[0].Burger, want) {
			t.Errorf("stats-less burger = %+v, want %+v", reviews[0].Burger, want)
		}
	})

	t.Run("ListReviews paginates the ordered feed", func(t *testing.T) {
		page1, err := repo.ListReviews(ctx, usecase.ReviewListFilter{}, 2, 0)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if got, want := reviewIDs(page1), []int64{rTie2, rTie1}; !reflect.DeepEqual(got, want) {
			t.Errorf("page 1 = %v, want %v", got, want)
		}
		page2, err := repo.ListReviews(ctx, usecase.ReviewListFilter{}, 2, 2)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if got, want := reviewIDs(page2), []int64{rOld}; !reflect.DeepEqual(got, want) {
			t.Errorf("page 2 = %v, want %v", got, want)
		}
		far, err := repo.ListReviews(ctx, usecase.ReviewListFilter{}, 2, 100)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if len(far) != 0 {
			t.Errorf("far page = %v, want empty", far)
		}
	})

	t.Run("ListReviews applies the Rails ReviewQuery filters on top of the feed rules", func(t *testing.T) {
		intp := func(n int) *int { return &n }
		int64p := func(n int64) *int64 { return &n }
		tests := []struct {
			name   string
			filter usecase.ReviewListFilter
			want   []int64
		}{
			{name: "rating exact match (by_rating)", filter: usecase.ReviewListFilter{Rating: intp(4)}, want: []int64{rTie2}},
			// Rating-2 reviews exist only on the pending/rejected-only
			// burgers: the active-shop feed rule still applies.
			{name: "rating matching only hidden reviews is empty", filter: usecase.ReviewListFilter{Rating: intp(2)}, want: []int64{}},
			{name: "keyword is case-insensitive (keyword_search ILIKE)", filter: usecase.ReviewListFilter{Keyword: "tAsT"}, want: []int64{rOld}},
			// NULL comments never match, like Rails' comment ILIKE.
			{name: "keyword skips NULL comments", filter: usecase.ReviewListFilter{Keyword: "a"}, want: []int64{rOld}},
			// Unescaped, "%" would ILIKE-match every non-NULL comment.
			{name: "keyword LIKE metacharacters match literally", filter: usecase.ReviewListFilter{Keyword: "%"}, want: []int64{}},
			{name: "keyword matching only hidden reviews is empty", filter: usecase.ReviewListFilter{Keyword: "only"}, want: []int64{}},
			{name: "shop_id follows the shops_burgers link", filter: usecase.ReviewListFilter{ShopID: int64p(active2)}, want: []int64{rTie1, rOld}},
			{name: "shop_id keeps all burgers of the shop", filter: usecase.ReviewListFilter{ShopID: int64p(active1)}, want: []int64{rTie2, rTie1, rOld}},
			{name: "unknown shop_id is empty", filter: usecase.ReviewListFilter{ShopID: int64p(99999)}, want: []int64{}},
			{name: "filters combine with AND", filter: usecase.ReviewListFilter{Rating: intp(5), Keyword: "tast", ShopID: int64p(active2)}, want: []int64{rOld}},
			{name: "AND combination with no match is empty", filter: usecase.ReviewListFilter{Rating: intp(3), Keyword: "tast"}, want: []int64{}},
			// Out-of-range ratings must compare false, not overflow the
			// smallint column into a SQL error.
			{name: "rating beyond smallint is empty, not an error", filter: usecase.ReviewListFilter{Rating: intp(1 << 40)}, want: []int64{}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				reviews, err := repo.ListReviews(ctx, tt.filter, 100, 0)
				if err != nil {
					t.Fatalf("ListReviews returned error: %v", err)
				}
				if got := reviewIDs(reviews); !reflect.DeepEqual(got, tt.want) {
					t.Errorf("ids = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("shop_id filter requires the filter shop itself to be active", func(t *testing.T) {
		// A burger linked to BOTH an active and a pending shop: its review
		// is in the feed (via the active link), but filtering by the pending
		// shop must return nothing — stricter than Rails' status-blind shop
		// filter, consistent with the feed's active-shop rule.
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
		got, err := repo.ListReviews(ctx, byShop(pending), 100, 0)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("pending shop filter = %v, want empty", reviewIDs(got))
		}
		got, err = repo.ListReviews(ctx, byShop(mixedActive), 100, 0)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if want := []int64{rMixed}; !reflect.DeepEqual(reviewIDs(got), want) {
			t.Errorf("active shop filter = %v, want %v", reviewIDs(got), want)
		}
	})

	t.Run("GetReview joins author, burger, and stats", func(t *testing.T) {
		got, err := repo.GetReview(ctx, rTie1)
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

	t.Run("AC6 discarded and unknown reviews yield ErrReviewNotFound", func(t *testing.T) {
		for name, id := range map[string]int64{"discarded": rDiscarded, "unknown": 99999} {
			if _, err := repo.GetReview(ctx, id); !errors.Is(err, domain.ErrReviewNotFound) {
				t.Errorf("%s: error = %v, want %v", name, err, domain.ErrReviewNotFound)
			}
		}
	})

	t.Run("GetShop returns the bare shop or ErrShopNotFound", func(t *testing.T) {
		shop, err := repo.GetShop(ctx, pending)
		if err != nil {
			t.Fatalf("GetShop returned error: %v", err)
		}
		if shop.ID != pending || shop.Status != domain.ShopStatusPending || shop.CreatorID == nil || *shop.CreatorID != alice {
			t.Errorf("shop = %+v, want pending shop created by alice", shop)
		}
		if _, err := repo.GetShop(ctx, 99999); !errors.Is(err, domain.ErrShopNotFound) {
			t.Fatalf("error = %v, want %v", err, domain.ErrShopNotFound)
		}
	})

	t.Run("GetShopBurger requires the shops_burgers link", func(t *testing.T) {
		burger, err := repo.GetShopBurger(ctx, active1, cheese)
		if err != nil {
			t.Fatalf("GetShopBurger returned error: %v", err)
		}
		want := domain.ShopReviewBurger{ID: cheese, Name: "Cheese", AverageRating: 4.0, ReviewCount: 2, WeightedScore: 3.9, Confidence: 0.7}
		if !reflect.DeepEqual(burger, want) {
			t.Errorf("burger = %+v, want %+v", burger, want)
		}
		// No stats row: zeros.
		statless, err := repo.GetShopBurger(ctx, active1, plain)
		if err != nil {
			t.Fatalf("GetShopBurger returned error: %v", err)
		}
		if want := (domain.ShopReviewBurger{ID: plain, Name: "Plain"}); !reflect.DeepEqual(statless, want) {
			t.Errorf("stats-less burger = %+v, want %+v", statless, want)
		}
		// Existing burger of another shop and unknown burger are identical.
		for name, burgerID := range map[string]int64{"unlinked": hidden, "unknown": 99999} {
			if _, err := repo.GetShopBurger(ctx, active1, burgerID); !errors.Is(err, domain.ErrBurgerNotFound) {
				t.Errorf("%s: error = %v, want %v", name, err, domain.ErrBurgerNotFound)
			}
		}
	})

	t.Run("CreateReview inserts and returns the stored row", func(t *testing.T) {
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
		if _, err := repo.GetReview(ctx, created.ID); err != nil {
			t.Errorf("GetReview after create returned error: %v", err)
		}
		// Remove it again and re-seed cheese's burger_stats (the create
		// recalculated them) so the feed and stats assertions of sibling
		// subtests stay exact regardless of execution order.
		if _, err := conn.Exec(ctx, `DELETE FROM reviews WHERE id = $1`, created.ID); err != nil {
			t.Fatalf("delete created review: %v", err)
		}
		seedCheeseStats(t)
	})

	t.Run("UpdateReviewContent writes only rating and comment", func(t *testing.T) {
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
		// discarded_at was not touched: the review is still in the feed.
		if _, err := repo.GetReview(ctx, rOld); err != nil {
			t.Errorf("GetReview after update returned error: %v", err)
		}
		// Restore the review and re-seed cheese's burger_stats (both updates
		// recalculated them) for sibling subtests.
		if _, err := repo.UpdateReviewContent(ctx, rOld, 5, "Tasty"); err != nil {
			t.Fatalf("restore review: %v", err)
		}
		seedCheeseStats(t)
	})

	t.Run("UpdateReviewContent on discarded or unknown reviews yields ErrReviewNotFound", func(t *testing.T) {
		for name, id := range map[string]int64{"discarded": rDiscarded, "unknown": 99999} {
			if _, err := repo.UpdateReviewContent(ctx, id, 3, "x"); !errors.Is(err, domain.ErrReviewNotFound) {
				t.Errorf("%s: error = %v, want %v", name, err, domain.ErrReviewNotFound)
			}
		}
	})

	t.Run("DiscardReview soft-deletes exactly once and never hard-deletes", func(t *testing.T) {
		victim := insertRow(ctx, t, conn, insertReview, 3, "bye", carol, cheese, nil, t2)
		if err := repo.DiscardReview(ctx, victim); err != nil {
			t.Fatalf("DiscardReview returned error: %v", err)
		}
		if _, err := repo.GetReview(ctx, victim); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("GetReview after discard = %v, want %v", err, domain.ErrReviewNotFound)
		}
		reviews, err := repo.ListReviews(ctx, usecase.ReviewListFilter{}, 100, 0)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		for _, r := range reviews {
			if r.ID == victim {
				t.Errorf("feed still contains discarded review %d", victim)
			}
		}
		// The row still exists (soft delete), with discarded_at stamped.
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
		// A second discard matches no row.
		if err := repo.DiscardReview(ctx, victim); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("second discard = %v, want %v", err, domain.ErrReviewNotFound)
		}
		if err := repo.DiscardReview(ctx, 99999); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Errorf("unknown discard = %v, want %v", err, domain.ErrReviewNotFound)
		}
		// Clean up for sibling subtests: remove the victim and re-seed
		// cheese's burger_stats (the discard recalculated them).
		if _, err := conn.Exec(ctx, `DELETE FROM reviews WHERE id = $1`, victim); err != nil {
			t.Fatalf("delete victim review: %v", err)
		}
		seedCheeseStats(t)
	})
}

// TestReviewRepositoryCreateReviewForNamedBurger exercises the burger_name
// find-or-create submission path (S6 P3-1): shop-scoped reuse by exact
// name, burger + link creation for unknown names, per-shop name scoping
// (the same name at another shop is a distinct burger row), and the
// single-transaction guarantee (a failed insert commits no orphan burger
// or link).
func TestReviewRepositoryCreateReviewForNamedBurger(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	repo := repository.NewReviewRepository(conn)

	insertUser := `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, 'x', $3) RETURNING id`
	alice := insertRow(ctx, t, conn, insertUser, "alice@example.com", "alice", false)

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

	t.Run("existing name in the shop is reused with its pre-insert stats", func(t *testing.T) {
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
		// The insert recalculated the stats in the same transaction.
		if stats := requireConsistentStats(ctx, t, conn, cheese); stats.ReviewCount != 1 {
			t.Errorf("stats = %+v, want the recalculated count 1 (only the new kept review)", stats)
		}
	})

	t.Run("unknown name creates the burger and its shops_burgers link", func(t *testing.T) {
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

	t.Run("the same name at another shop is a distinct burger row", func(t *testing.T) {
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
		// Shop A's Cheese link still points at the original burger only.
		if got := countRows(t, `SELECT count(*) FROM shops_burgers WHERE shop_id = $1`, shopA); got != 2 {
			t.Errorf("shop A link rows = %d, want its original Cheese and Veggie", got)
		}
	})

	t.Run("a failed insert commits no orphan burger or link", func(t *testing.T) {
		// The unknown author violates the reviews.user_id FK after the
		// burger and link inserts — the whole transaction must roll back.
		review := domain.Review{Rating: 4, AuthorID: 99999}
		if _, _, err := repo.CreateReviewForNamedBurger(ctx, shopA, "Ghost", review); err == nil {
			t.Fatal("CreateReviewForNamedBurger returned nil error, want the FK failure")
		}
		if got := burgersNamed(t, "Ghost"); got != 0 {
			t.Errorf("Ghost burger rows = %d, want the rollback to leave none", got)
		}
		if got := countRows(t, `SELECT count(*) FROM shops_burgers sb JOIN burgers b ON b.id = sb.burger_id WHERE b.name = 'Ghost'`); got != 0 {
			t.Errorf("Ghost link rows = %d, want none", got)
		}
		if got := countRows(t, `SELECT count(*) FROM reviews WHERE user_id = 99999`); got != 0 {
			t.Errorf("review rows = %d, want none", got)
		}
	})
}

// storedBurgerStats is the burger_stats row as read back in tests.
type storedBurgerStats struct {
	ReviewCount   int64
	AverageRating float64
	WeightedScore float64
	Confidence    float64
	CalculatedAt  time.Time
}

// fetchBurgerStats reads the burger_stats row directly; ok is false when
// no row exists.
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

// keptReviewFacts loads the burger's kept reviews of kept users (the same
// rule the repository uses) as domain facts, with each fact author's kept
// ratings across all burgers as reviewer history.
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
		userID    int64
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

// keptRatingsOf returns the user's kept ratings across all burgers, id
// ascending (the reviewer-trust history).
func keptRatingsOf(ctx context.Context, t *testing.T, conn *pgx.Conn, userID int64) []float64 {
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

// requireConsistentStats asserts the stored burger_stats row exists and
// exactly equals a recomputation via the domain functions from the stored
// review rows and the stored calculated_at (the repository truncates its
// "now" to the timestamptz resolution precisely so this round-trips), then
// returns the row. Floats are compared exactly: same inputs through the
// same pure functions must yield identical values.
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

// mustCreateReview builds and persists a review through the repository.
func mustCreateReview(ctx context.Context, t *testing.T, repo *repository.ReviewRepository, rating int, comment string, authorID, burgerID int64) domain.Review {
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

// TestReviewRepositoryBurgerStats exercises the S7 same-transaction
// burger_stats recalculation (issue #15): every review write leaves the
// stats row exactly consistent with the domain calculator over the kept
// reviews of kept users, concurrent writers never lose an update, and
// failed writes leave the stats untouched.
func TestReviewRepositoryBurgerStats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	conn, dbURL := dbtest.New(t)
	repo := repository.NewReviewRepository(conn)

	insertUser := `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, 'x', $3) RETURNING id`
	alice := insertRow(ctx, t, conn, insertUser, "alice@example.com", "alice", false)
	bob := insertRow(ctx, t, conn, insertUser, "bob@example.com", "bob", false)

	insertBurger := `INSERT INTO burgers (name) VALUES ($1) RETURNING id`
	burger := insertRow(ctx, t, conn, insertBurger, "Stats Burger")

	var aliceReview domain.Review

	t.Run("AC1 CreateReview upserts stats in the same transaction", func(t *testing.T) {
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

	t.Run("UpdateReviewContent recalculates stats", func(t *testing.T) {
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

	t.Run("AC2 DiscardReview recalculates without the discarded review", func(t *testing.T) {
		if err := repo.DiscardReview(ctx, aliceReview.ID); err != nil {
			t.Fatalf("DiscardReview returned error: %v", err)
		}
		stats := requireConsistentStats(ctx, t, conn, burger)
		if stats.ReviewCount != 1 || stats.AverageRating != 4.0 {
			t.Errorf("stats after discard = %+v, want only bob's rating 4 left", stats)
		}
	})

	t.Run("AC2 discarding the only review leaves the zero row", func(t *testing.T) {
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

	t.Run("AC4 reviews and histories of discarded users are excluded", func(t *testing.T) {
		ac4Burger := insertRow(ctx, t, conn, insertBurger, "AC4 Burger")
		carl := insertRow(ctx, t, conn, insertUser, "carl@example.com", "carl", false)
		aliceAC4 := mustCreateReview(ctx, t, repo, 5, "mine stays", alice, ac4Burger)
		mustCreateReview(ctx, t, repo, 2, "mine vanishes", carl, ac4Burger)
		if got := requireConsistentStats(ctx, t, conn, ac4Burger); got.ReviewCount != 2 {
			t.Fatalf("stats before user discard = %+v, want count 2", got)
		}

		if _, err := conn.Exec(ctx, `UPDATE users SET discarded_at = now() WHERE id = $1`, carl); err != nil {
			t.Fatalf("discard user: %v", err)
		}
		// Trigger recalculation via a kept user's write.
		if _, err := repo.UpdateReviewContent(ctx, aliceAC4.ID, 4, "still here"); err != nil {
			t.Fatalf("UpdateReviewContent returned error: %v", err)
		}

		stats := requireConsistentStats(ctx, t, conn, ac4Burger)
		if stats.ReviewCount != 1 || stats.AverageRating != 4.0 {
			t.Errorf("stats after user discard = %+v, want only alice's kept review", stats)
		}
		// Carl's ratings feed neither the facts nor any reviewer history:
		// the stored score equals one computed from alice's fact and
		// alice's own kept ratings alone.
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

	t.Run("AC3 concurrent creates on one burger never lose an update", func(t *testing.T) {
		pool, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			t.Fatalf("open pool: %v", err)
		}
		t.Cleanup(pool.Close)
		poolRepo := repository.NewReviewRepository(pool)

		dave := insertRow(ctx, t, conn, insertUser, "dave@example.com", "dave", false)
		erin := insertRow(ctx, t, conn, insertUser, "erin@example.com", "erin", false)

		// Without the FOR UPDATE serialization both transactions read a
		// snapshot missing the other's review and the later upsert writes
		// review_count 1 (lost update). Repeat with fresh burgers so a
		// lucky interleaving cannot mask the race.
		for i := 0; i < 5; i++ {
			raceBurger := insertRow(ctx, t, conn, insertBurger, fmt.Sprintf("Race Burger %d", i))
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

	t.Run("failed writes leave burger_stats untouched", func(t *testing.T) {
		errBurger := insertRow(ctx, t, conn, insertBurger, "Error Burger")
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

// TestReviewRepositoryPhotoKey covers the S10 photo_key persistence:
// CreateReview stores the key, the joined read queries return it, and
// UpdateReviewPhotoKey swaps only the key on still-kept reviews.
func TestReviewRepositoryPhotoKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	repo := repository.NewReviewRepository(conn)

	alice := insertRow(ctx, t, conn,
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

	t.Run("read queries carry photo_key", func(t *testing.T) {
		detail, err := repo.GetReview(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetReview returned error: %v", err)
		}
		if detail.PhotoKey == nil || *detail.PhotoKey != "reviews/abc.jpg" {
			t.Errorf("detail PhotoKey = %v, want reviews/abc.jpg", detail.PhotoKey)
		}
		list, err := repo.ListReviews(ctx, usecase.ReviewListFilter{}, 10, 0)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if len(list) != 1 || list[0].PhotoKey == nil || *list[0].PhotoKey != "reviews/abc.jpg" {
			t.Errorf("list = %+v, want one review with PhotoKey reviews/abc.jpg", list)
		}
	})

	t.Run("UpdateReviewPhotoKey swaps only the key", func(t *testing.T) {
		updated, err := repo.UpdateReviewPhotoKey(ctx, created.ID, strPtr("reviews/def.png"))
		if err != nil {
			t.Fatalf("UpdateReviewPhotoKey returned error: %v", err)
		}
		if updated.PhotoKey == nil || *updated.PhotoKey != "reviews/def.png" {
			t.Errorf("updated PhotoKey = %v, want reviews/def.png", updated.PhotoKey)
		}
		if updated.Rating != 4 || updated.Comment == nil || *updated.Comment != "Tasty" {
			t.Errorf("updated review = %+v, want rating and comment untouched", updated)
		}
	})

	t.Run("UpdateReviewContentAndPhotoKey writes content and key together", func(t *testing.T) {
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
		// The committed row carries BOTH the content and the key (one
		// transaction — never content without the key).
		detail, err := repo.GetReview(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetReview after combined update returned error: %v", err)
		}
		if detail.Rating != 5 || detail.PhotoKey == nil || *detail.PhotoKey != "reviews/both.png" {
			t.Errorf("stored = rating %d, key %v, want 5 and reviews/both.png", detail.Rating, detail.PhotoKey)
		}
	})

	t.Run("keyless create stays NULL", func(t *testing.T) {
		plain, err := repo.CreateReview(ctx, domain.Review{Rating: 3, Comment: &comment, AuthorID: alice, BurgerID: burger})
		if err != nil {
			t.Fatalf("CreateReview returned error: %v", err)
		}
		if plain.PhotoKey != nil {
			t.Errorf("PhotoKey = %v, want nil", plain.PhotoKey)
		}
	})

	t.Run("discarded review yields ErrReviewNotFound", func(t *testing.T) {
		if err := repo.DiscardReview(ctx, created.ID); err != nil {
			t.Fatalf("DiscardReview returned error: %v", err)
		}
		if _, err := repo.UpdateReviewPhotoKey(ctx, created.ID, nil); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Fatalf("UpdateReviewPhotoKey error = %v, want %v", err, domain.ErrReviewNotFound)
		}
		// The combined write rolls back too: no content change survives.
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
