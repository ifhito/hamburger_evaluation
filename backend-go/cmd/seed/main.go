// Command seed populates the development database with idempotent fixture
// data (issue #17 R5): an admin user, three regular users, three approved
// shops plus one pending shop, two burgers per approved shop, and a fixed
// set of reviews. burger_stats is recalculated afterwards with the same
// domain calculator the app uses, so GET responses are coherent.
//
// All fixture passwords are "password123" — these are well-known dev
// fixtures, not secrets. Run it via docker compose:
//
//	docker compose run --rm migrate up
//	docker compose run --rm seed
//
// Re-running is safe: users are keyed by email, shops by name, burgers by
// (shop, name), and reviews by (user, burger), so nothing is duplicated.
// The whole seed runs in one transaction and fails loud (non-zero exit)
// on any error.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// devPassword is the shared password of every fixture user (dev-only).
const devPassword = "password123"

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatalf("seed: %v", err)
	}
	fmt.Println("seed: done")
}

func run(ctx context.Context) error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errors.New("DATABASE_URL is not set")
	}

	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(ctx)

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	if err := seed(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// seed inserts every fixture inside the given transaction. Reviews are
// assigned deterministically (unlike the Rails seeds' sampling) so repeat
// runs converge on the same state.
func seed(ctx context.Context, tx pgx.Tx) error {
	admin, err := seedUser(ctx, tx, "admin@example.com", "admin", true)
	if err != nil {
		return err
	}
	alice, err := seedUser(ctx, tx, "alice@example.com", "alice", false)
	if err != nil {
		return err
	}
	bob, err := seedUser(ctx, tx, "bob@example.com", "bob", false)
	if err != nil {
		return err
	}
	charlie, err := seedUser(ctx, tx, "charlie@example.com", "charlie", false)
	if err != nil {
		return err
	}

	// Shop status encoding mirrors db/migrations/000002: 0=pending, 1=active.
	shakeShack, err := seedShop(ctx, tx, "Shake Shack 渋谷", 1, admin)
	if err != nil {
		return err
	}
	jsBurgers, err := seedShop(ctx, tx, "J.S. BURGERS CAFE 新宿", 1, admin)
	if err != nil {
		return err
	}
	freshness, err := seedShop(ctx, tx, "フレッシュネスバーガー 原宿", 1, admin)
	if err != nil {
		return err
	}
	// One pending shop to exercise the moderation flow; it gets no burgers.
	if _, err := seedShop(ctx, tx, "バーガースタンド 下北沢（審査待ち）", 0, alice); err != nil {
		return err
	}

	type burgerSpec struct {
		shopID int64
		name   string
	}
	specs := []burgerSpec{
		{shakeShack, "クラシックバーガー"},
		{shakeShack, "チーズバーガー"},
		{jsBurgers, "クラシックバーガー"},
		{jsBurgers, "アボカドバーガー"},
		{freshness, "クラシックバーガー"},
		{freshness, "テリヤキバーガー"},
	}
	burgers := make([]int64, len(specs))
	for i, spec := range specs {
		id, err := seedBurger(ctx, tx, spec.shopID, spec.name)
		if err != nil {
			return err
		}
		burgers[i] = id
	}

	type reviewSpec struct {
		userID  int64
		burger  int // index into burgers
		rating  int16
		comment string
	}
	reviews := []reviewSpec{
		{alice, 0, 5, "肉汁たっぷりで最高でした！"},
		{alice, 2, 4, "バンズがふわふわで美味しかった。"},
		{alice, 5, 4, "チーズの量がちょうどよかった。"},
		{bob, 0, 4, "また絶対行きたいです。"},
		{bob, 1, 5, "ボリューム満点でコスパ良し。"},
		{bob, 3, 3, "ちょっとしょっぱかったけど美味しい。"},
		{charlie, 2, 5, "肉汁たっぷりで最高でした！"},
		{charlie, 4, 4, "バンズがふわふわで美味しかった。"},
		{charlie, 5, 5, "また絶対行きたいです。"},
	}
	for _, spec := range reviews {
		if err := seedReview(ctx, tx, spec.userID, burgers[spec.burger], spec.rating, spec.comment); err != nil {
			return err
		}
	}

	// Ascending burger id order, matching the multi-burger recalculation
	// convention in the review repository.
	for _, burgerID := range burgers {
		if err := recalculateBurgerStats(ctx, tx, burgerID); err != nil {
			return err
		}
	}
	return nil
}

// seedUser finds the user by email or creates it with the shared dev
// password hashed exactly like signup does (bcrypt at the default cost,
// see internal/adapter/infra/password.go).
func seedUser(ctx context.Context, tx pgx.Tx, email, username string, admin bool) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("find user %s: %w", email, err)
	}
	digest, err := bcrypt.GenerateFromPassword([]byte(devPassword), bcrypt.DefaultCost)
	if err != nil {
		return 0, fmt.Errorf("hash password for %s: %w", email, err)
	}
	err = tx.QueryRow(ctx,
		`INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, $3, $4) RETURNING id`,
		email, username, string(digest), admin,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert user %s: %w", email, err)
	}
	return id, nil
}

// seedShop finds the shop by name (the seed's natural key — the schema has
// no unique constraint on it) or creates it with the given status.
func seedShop(ctx context.Context, tx pgx.Tx, name string, status int16, creatorID int64) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM shops WHERE name = $1`, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("find shop %s: %w", name, err)
	}
	err = tx.QueryRow(ctx,
		`INSERT INTO shops (name, status, creator_id) VALUES ($1, $2, $3) RETURNING id`,
		name, status, creatorID,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert shop %s: %w", name, err)
	}
	return id, nil
}

// seedBurger finds the shop's burger by name via shops_burgers (the same
// per-shop natural key CreateReviewForNamedBurger uses) or creates the
// burger and its link.
func seedBurger(ctx context.Context, tx pgx.Tx, shopID int64, name string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx,
		`SELECT b.id FROM burgers b
		 JOIN shops_burgers sb ON sb.burger_id = b.id
		 WHERE sb.shop_id = $1 AND b.name = $2`,
		shopID, name,
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("find burger %s of shop %d: %w", name, shopID, err)
	}
	err = tx.QueryRow(ctx, `INSERT INTO burgers (name) VALUES ($1) RETURNING id`, name).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert burger %s: %w", name, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		shopID, id,
	); err != nil {
		return 0, fmt.Errorf("link burger %d to shop %d: %w", id, shopID, err)
	}
	return id, nil
}

// seedReview inserts a review unless the user already has a kept review of
// the burger (the seed's natural key; the app itself allows several).
func seedReview(ctx context.Context, tx pgx.Tx, userID, burgerID int64, rating int16, comment string) error {
	var exists bool
	err := tx.QueryRow(ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM reviews
		   WHERE user_id = $1 AND burger_id = $2 AND discarded_at IS NULL
		 )`,
		userID, burgerID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("find review (user %d, burger %d): %w", userID, burgerID, err)
	}
	if exists {
		return nil
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO reviews (rating, comment, user_id, burger_id) VALUES ($1, $2, $3, $4)`,
		rating, comment, userID, burgerID,
	); err != nil {
		return fmt.Errorf("insert review (user %d, burger %d): %w", userID, burgerID, err)
	}
	return nil
}

// recalculateBurgerStats recomputes and upserts one burger's stats row from
// its kept reviews, mirroring the repository's recalculateBurgerStats: the
// same SQL as db/queries/burger_stats.sql and the same pure domain
// calculator, so seeded stats match what the app would store. No FOR UPDATE
// lock: the seed is a one-shot tool with no concurrent writers.
func recalculateBurgerStats(ctx context.Context, tx pgx.Tx, burgerID int64) error {
	rows, err := tx.Query(ctx,
		`SELECT r.rating, r.created_at, r.user_id
		 FROM reviews r
		 JOIN users u ON u.id = r.user_id
		 WHERE r.burger_id = $1 AND r.discarded_at IS NULL AND u.discarded_at IS NULL
		 ORDER BY r.id`,
		burgerID,
	)
	if err != nil {
		return fmt.Errorf("list facts for burger %d: %w", burgerID, err)
	}
	type factRow struct {
		rating    int16
		createdAt time.Time
		userID    int64
	}
	var factRows []factRow
	for rows.Next() {
		var row factRow
		if err := rows.Scan(&row.rating, &row.createdAt, &row.userID); err != nil {
			rows.Close()
			return fmt.Errorf("scan fact for burger %d: %w", burgerID, err)
		}
		factRows = append(factRows, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list facts for burger %d: %w", burgerID, err)
	}

	// Reviewer-trust histories: each fact author's kept ratings across all
	// burgers, grouped by user.
	historyByUser := make(map[int64][]float64, len(factRows))
	userIDs := make([]int64, 0, len(factRows))
	for _, row := range factRows {
		if _, seen := historyByUser[row.userID]; !seen {
			historyByUser[row.userID] = nil
			userIDs = append(userIDs, row.userID)
		}
	}
	if len(userIDs) > 0 {
		ratingRows, err := tx.Query(ctx,
			`SELECT r.user_id, r.rating FROM reviews r
			 WHERE r.user_id = ANY($1::bigint[]) AND r.discarded_at IS NULL
			 ORDER BY r.id`,
			userIDs,
		)
		if err != nil {
			return fmt.Errorf("list reviewer ratings: %w", err)
		}
		for ratingRows.Next() {
			var userID int64
			var rating int16
			if err := ratingRows.Scan(&userID, &rating); err != nil {
				ratingRows.Close()
				return fmt.Errorf("scan reviewer rating: %w", err)
			}
			historyByUser[userID] = append(historyByUser[userID], float64(rating))
		}
		ratingRows.Close()
		if err := ratingRows.Err(); err != nil {
			return fmt.Errorf("list reviewer ratings: %w", err)
		}
	}

	facts := make([]domain.ReviewFact, 0, len(factRows))
	for _, row := range factRows {
		facts = append(facts, domain.ReviewFact{
			Rating:          float64(row.rating),
			CreatedAt:       row.createdAt,
			ReviewerHistory: domain.ReviewerHistory{Ratings: historyByUser[row.userID]},
		})
	}
	now := time.Now().Truncate(time.Microsecond)
	score := domain.CalculateBurgerScore(facts, now)
	if _, err := tx.Exec(ctx,
		`INSERT INTO burger_stats (burger_id, review_count, average_rating, weighted_score, confidence, calculated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (burger_id) DO UPDATE
		 SET review_count = EXCLUDED.review_count,
		     average_rating = EXCLUDED.average_rating,
		     weighted_score = EXCLUDED.weighted_score,
		     confidence = EXCLUDED.confidence,
		     calculated_at = EXCLUDED.calculated_at`,
		burgerID, int64(len(facts)), domain.AverageRating(facts), score.WeightedAverage, score.Confidence, now,
	); err != nil {
		return fmt.Errorf("upsert stats for burger %d: %w", burgerID, err)
	}
	return nil
}
