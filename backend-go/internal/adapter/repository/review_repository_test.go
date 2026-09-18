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
