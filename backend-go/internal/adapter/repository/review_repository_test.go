package repository_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
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

	if _, err := conn.Exec(ctx,
		`INSERT INTO burger_stats (burger_id, review_count, average_rating, weighted_score, confidence, calculated_at)
		 VALUES ($1, 2, 4.0, 3.9, 0.7, now())`, cheese); err != nil {
		t.Fatalf("insert burger stats: %v", err)
	}

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
		reviews, err := repo.ListReviews(ctx, 100, 0)
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
		page1, err := repo.ListReviews(ctx, 2, 0)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if got, want := reviewIDs(page1), []int64{rTie2, rTie1}; !reflect.DeepEqual(got, want) {
			t.Errorf("page 1 = %v, want %v", got, want)
		}
		page2, err := repo.ListReviews(ctx, 2, 2)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if got, want := reviewIDs(page2), []int64{rOld}; !reflect.DeepEqual(got, want) {
			t.Errorf("page 2 = %v, want %v", got, want)
		}
		far, err := repo.ListReviews(ctx, 2, 100)
		if err != nil {
			t.Fatalf("ListReviews returned error: %v", err)
		}
		if len(far) != 0 {
			t.Errorf("far page = %v, want empty", far)
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
		// Remove it again so the feed assertions of sibling subtests stay
		// exact regardless of execution order.
		if _, err := conn.Exec(ctx, `DELETE FROM reviews WHERE id = $1`, created.ID); err != nil {
			t.Fatalf("delete created review: %v", err)
		}
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
		// Restore for sibling subtests.
		if _, err := repo.UpdateReviewContent(ctx, rOld, 5, "Tasty"); err != nil {
			t.Fatalf("restore review: %v", err)
		}
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
		reviews, err := repo.ListReviews(ctx, 100, 0)
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
		// Clean up for sibling subtests.
		if _, err := conn.Exec(ctx, `DELETE FROM reviews WHERE id = $1`, victim); err != nil {
			t.Fatalf("delete victim review: %v", err)
		}
	})
}
